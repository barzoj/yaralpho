package scheduler

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/barzoj/yaralpho/internal/consumer"
	"github.com/barzoj/yaralpho/internal/storage"
	"go.uber.org/zap"
)

// Storage exposes the minimal persistence contract the scheduler needs for
// agent and batch coordination. It is satisfied by internal/storage.Storage.
type Storage interface {
	ListBatches(ctx context.Context, limit int64) ([]storage.Batch, error)
	UpdateBatch(ctx context.Context, batch *storage.Batch) error
	ListAgents(ctx context.Context) ([]storage.Agent, error)
	UpdateAgent(ctx context.Context, agent *storage.Agent) error
}

// Worker executes a single work item selected by the scheduler.
type Worker interface {
	Process(ctx context.Context, item consumer.WorkItem) error
}

const (
	defaultMaxRetries = 5
	defaultInterval   = 10 * time.Second
)

// Scheduler drives periodic selection of batch items for execution.
type Scheduler struct {
	store      Storage
	worker     Worker
	logger     *zap.Logger
	interval   time.Duration
	draining   atomic.Bool
	activeWG   sync.WaitGroup
	activeRuns atomic.Int64
	maxRetries int
}

// Options configures Scheduler construction.
type Options struct {
	// Interval controls how often Start will invoke Tick. A zero or negative value defaults to 10s.
	Interval time.Duration
	// Draining sets the initial draining state; when true, no new work is scheduled until cleared.
	Draining bool
	// MaxRetries caps attempts per item; zero or negative values fall back to defaultMaxRetries.
	MaxRetries int
}

// New constructs a Scheduler with the provided dependencies and options.
func New(store Storage, worker Worker, logger *zap.Logger, opts Options) *Scheduler {
	if logger == nil {
		logger = zap.NewNop()
	}
	if opts.MaxRetries <= 0 {
		opts.MaxRetries = defaultMaxRetries
	}
	if opts.Interval <= 0 {
		opts.Interval = defaultInterval
	}
	s := &Scheduler{store: store, worker: worker, logger: logger, maxRetries: opts.MaxRetries, interval: opts.Interval}
	s.draining.Store(opts.Draining)
	return s
}

// SetDraining toggles the draining flag. When draining, Tick does nothing.
func (s *Scheduler) SetDraining(draining bool) {
	s.draining.Store(draining)
}

// Draining reports whether the scheduler is currently in draining mode.
func (s *Scheduler) Draining() bool {
	if s == nil {
		return false
	}
	return s.draining.Load()
}

// ActiveCount returns the number of in-flight work items.
func (s *Scheduler) ActiveCount() int {
	if s == nil {
		return 0
	}
	return int(s.activeRuns.Load())
}

// WaitForIdle blocks until no work items are active or the context is canceled.
func (s *Scheduler) WaitForIdle(ctx context.Context) error {
	if s == nil {
		return nil
	}

	done := make(chan struct{})
	go func() {
		s.activeWG.Wait()
		close(done)
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-done:
		return nil
	}
}

// Start begins periodic ticking at the configured interval until the context is
// canceled or Stop is called. Stub implementation; Tick must be invoked
// manually until periodic scheduling is implemented.
func (s *Scheduler) Start(ctx context.Context) error {
	_ = ctx
	return nil
}

// Stop requests a graceful shutdown of periodic ticking and prevents new work
// from starting by enabling draining. Stub implementation; callers should
// invoke WaitForIdle to block until in-flight work completes.
func (s *Scheduler) Stop(ctx context.Context) error {
	_ = ctx
	s.SetDraining(true)
	return nil
}

// Tick selects the next pending item across batches and dispatches it to the
// worker. It enforces per-batch sequential execution, skips paused batches, and
// requires an idle agent before scheduling work. Tick must only be driven by a
// single process at a time; when draining is true, Tick should avoid starting
// new work.
func (s *Scheduler) Tick(ctx context.Context) error {
	if s == nil {
		return nil
	}
	if s.draining.Load() {
		s.logger.Debug("scheduler draining; skipping tick", zap.Bool("draining", true))
		return nil
	}
	if s.store == nil || s.worker == nil {
		return fmt.Errorf("scheduler not initialized")
	}

	batches, err := s.store.ListBatches(ctx, 0)
	if err != nil {
		return fmt.Errorf("list batches: %w", err)
	}
	agents, err := s.store.ListAgents(ctx)
	if err != nil {
		return fmt.Errorf("list agents: %w", err)
	}

	idleAgent := firstIdleAgent(agents)
	if idleAgent == nil {
		s.logger.Debug("no idle agents available; skipping tick", zap.Int("agents_total", len(agents)))
		return nil
	}

	for _, batch := range batches {
		batchFields := withBatchFields(batch, -1)
		switch batch.Status {
		case storage.BatchStatusPaused:
			s.logger.Debug("batch paused; skipping", batchFields...)
			continue
		case storage.BatchStatusFailed, storage.BatchStatusDone:
			continue
		}
		pendingIdx, hasInProgress := nextPendingIndex(batch.Items)
		if hasInProgress {
			s.logger.Debug("batch already in progress; skipping", batchFields...)
			continue
		}
		if pendingIdx == -1 {
			s.logger.Debug("batch has no pending items; skipping", batchFields...)
			continue
		}

		// Claim agent and batch item before dispatching work.
		batch.Status = storage.BatchStatusInProgress
		pendingItem := &batch.Items[pendingIdx]
		pendingItem.Status = storage.ItemStatusInProgress
		if err := s.store.UpdateBatch(ctx, &batch); err != nil {
			return fmt.Errorf("update batch %s: %w", batch.ID, err)
		}

		claimedAgent := *idleAgent
		claimedAgent.Status = storage.AgentStatusBusy
		if err := s.store.UpdateAgent(ctx, &claimedAgent); err != nil {
			// best-effort rollback to keep batch selectable next tick
			batch.Items[pendingIdx].Status = storage.ItemStatusPending
			batch.Status = storage.BatchStatusPending
			_ = s.store.UpdateBatch(ctx, &batch)
			return fmt.Errorf("update agent %s: %w", idleAgent.ID, err)
		}

		work := consumer.WorkItem{
			BatchID: batch.ID,
			TaskRef: batch.Items[pendingIdx].Input,
			AgentID: claimedAgent.ID,
			Runtime: claimedAgent.Runtime,
		}
		claimFields := append(withBatchFields(batch, pendingIdx), withAgentFields(&claimedAgent)...)
		s.logger.Info("claiming work item", append(claimFields, zap.String("task_ref", work.TaskRef))...)
		s.activeWG.Add(1)
		s.activeRuns.Add(1)
		defer func() {
			s.activeRuns.Add(-1)
			s.activeWG.Done()
		}()
		workerErr := s.worker.Process(ctx, work)

		pendingItem.Attempts++
		if workerErr == nil {
			pendingItem.Status = storage.ItemStatusDone
			if allItemsDone(batch.Items) {
				batch.Status = storage.BatchStatusDone
			} else {
				batch.Status = storage.BatchStatusPending
			}
			s.logger.Info("work item succeeded", claimFields...)
		} else {
			attemptFields := append(claimFields, withAttemptFields(pendingItem.Attempts, s.maxRetries)...)
			s.logger.Warn("worker failed", append(attemptFields, zap.Error(workerErr))...)
			if pendingItem.Attempts >= s.maxRetries {
				pendingItem.Status = storage.ItemStatusFailed
				batch.Status = storage.BatchStatusFailed
				s.logger.Error("work item failed", append(attemptFields, zap.Error(workerErr))...)
			} else {
				pendingItem.Status = storage.ItemStatusPending
				batch.Status = storage.BatchStatusPending
			}
		}

		updateBatchErr := s.store.UpdateBatch(ctx, &batch)

		claimedAgent.Status = storage.AgentStatusIdle
		updateAgentErr := s.store.UpdateAgent(ctx, &claimedAgent)
		if updateAgentErr != nil {
			s.logger.Warn("update agent idle", zap.Error(updateAgentErr), zap.String("agent_id", claimedAgent.ID))
		}

		if updateBatchErr != nil {
			return fmt.Errorf("update batch %s after completion: %w", batch.ID, updateBatchErr)
		}
		if updateAgentErr != nil {
			return fmt.Errorf("set agent %s idle: %w", claimedAgent.ID, updateAgentErr)
		}
		if workerErr != nil {
			return workerErr
		}
		return nil
	}

	s.logger.Debug("no eligible batches found; skipping tick")
	return nil
}

func firstIdleAgent(agents []storage.Agent) *storage.Agent {
	for i := range agents {
		if agents[i].Status == storage.AgentStatusIdle {
			return &agents[i]
		}
	}
	return nil
}

// nextPendingIndex returns the first pending item index and whether the batch
// already has an in-progress item (which enforces sequential execution).
func nextPendingIndex(items []storage.BatchItem) (pendingIdx int, hasInProgress bool) {
	pendingIdx = -1
	for i := range items {
		switch items[i].Status {
		case storage.ItemStatusInProgress:
			hasInProgress = true
		case storage.ItemStatusPending:
			if pendingIdx == -1 {
				pendingIdx = i
			}
		}
	}
	return pendingIdx, hasInProgress
}

func allItemsDone(items []storage.BatchItem) bool {
	for _, item := range items {
		if item.Status != storage.ItemStatusDone {
			return false
		}
	}
	return true
}

// withBatchFields returns standard batch zap fields, including optional item index.
func withBatchFields(batch storage.Batch, itemIndex int) []zap.Field {
	fields := []zap.Field{
		zap.String("batch_id", batch.ID),
		zap.String("repository_id", batch.RepositoryID),
	}
	if itemIndex >= 0 {
		fields = append(fields, zap.Int("item_index", itemIndex))
	}
	return fields
}

// withAgentFields returns the agent identifier field when present.
func withAgentFields(agent *storage.Agent) []zap.Field {
	if agent == nil {
		return nil
	}
	return []zap.Field{zap.String("agent_id", agent.ID)}
}

// withAttemptFields captures retry attempt metadata.
func withAttemptFields(attempt, maxRetries int) []zap.Field {
	fields := []zap.Field{}
	if attempt > 0 {
		fields = append(fields, zap.Int("attempt", attempt))
	}
	if maxRetries > 0 {
		fields = append(fields, zap.Int("max_retries", maxRetries))
	}
	return fields
}
