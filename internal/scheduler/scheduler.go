package scheduler

import (
	"context"
	"fmt"
	"sync/atomic"

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

const defaultMaxRetries = 5

// Scheduler drives periodic selection of batch items for execution.
type Scheduler struct {
	store      Storage
	worker     Worker
	logger     *zap.Logger
	draining   atomic.Bool
	maxRetries int
}

// New constructs a Scheduler with the provided dependencies.
func New(store Storage, worker Worker, logger *zap.Logger, maxRetries int) *Scheduler {
	if logger == nil {
		logger = zap.NewNop()
	}
	if maxRetries <= 0 {
		maxRetries = defaultMaxRetries
	}
	return &Scheduler{store: store, worker: worker, logger: logger, maxRetries: maxRetries}
}

// SetDraining toggles the draining flag. When draining, Tick does nothing.
func (s *Scheduler) SetDraining(draining bool) {
	s.draining.Store(draining)
}

// Tick selects the next pending item across batches and dispatches it to the
// worker. It enforces per-batch sequential execution, skips paused batches, and
// requires an idle agent before scheduling work.
func (s *Scheduler) Tick(ctx context.Context) error {
	if s == nil {
		return nil
	}
	if s.draining.Load() {
		s.logger.Debug("scheduler draining; skipping tick")
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
		s.logger.Debug("no idle agents available; skipping tick")
		return nil
	}

	for _, batch := range batches {
		switch batch.Status {
		case storage.BatchStatusPaused, storage.BatchStatusFailed, storage.BatchStatusDone:
			continue
		}
		pendingIdx, hasInProgress := nextPendingIndex(batch.Items)
		if hasInProgress || pendingIdx == -1 {
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

		work := consumer.WorkItem{BatchID: batch.ID, TaskRef: batch.Items[pendingIdx].Input}
		workerErr := s.worker.Process(ctx, work)

		pendingItem.Attempts++
		if workerErr == nil {
			pendingItem.Status = storage.ItemStatusDone
			if allItemsDone(batch.Items) {
				batch.Status = storage.BatchStatusDone
			} else {
				batch.Status = storage.BatchStatusPending
			}
		} else {
			s.logger.Warn("worker failed", zap.Error(workerErr), zap.String("batch_id", batch.ID), zap.String("agent_id", claimedAgent.ID), zap.Int("attempt", pendingItem.Attempts), zap.Int("max_retries", s.maxRetries))
			if pendingItem.Attempts >= s.maxRetries {
				pendingItem.Status = storage.ItemStatusFailed
				batch.Status = storage.BatchStatusFailed
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
