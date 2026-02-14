package scheduler

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/barzoj/yaralpho/internal/consumer"
	"github.com/barzoj/yaralpho/internal/storage"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestTick_NoBatches(t *testing.T) {
	st := &fakeStorage{}
	sched := New(st, &fakeWorker{}, zap.NewNop(), defaultMaxRetries)

	require.NoError(t, sched.Tick(context.Background()))
	require.Zero(t, st.updateBatchCalls)
}

func TestTick_DrainingSkips(t *testing.T) {
	st := &fakeStorage{batches: []storage.Batch{{ID: "b1"}}, agents: []storage.Agent{{ID: "a1", Status: storage.AgentStatusIdle}}}
	worker := &fakeWorker{}
	sched := New(st, worker, zap.NewNop(), defaultMaxRetries)
	sched.SetDraining(true)

	require.NoError(t, sched.Tick(context.Background()))
	require.False(t, worker.called)
}

func TestWaitForIdleTracksActiveWork(t *testing.T) {
	st := &fakeStorage{
		batches: []storage.Batch{{
			ID:     "b1",
			Status: storage.BatchStatusPending,
			Items:  []storage.BatchItem{{Input: "task-1", Status: storage.ItemStatusPending}},
		}},
		agents: []storage.Agent{{ID: "agent-1", Status: storage.AgentStatusIdle}},
	}

	started := make(chan struct{})
	release := make(chan struct{})
	worker := &blockingWorker{started: started, release: release}
	sched := New(st, worker, zap.NewNop(), defaultMaxRetries)

	go func() {
		require.NoError(t, sched.Tick(context.Background()))
	}()

	<-started
	require.Equal(t, 1, sched.ActiveCount())

	waitDone := make(chan struct{})
	go func() {
		require.NoError(t, sched.WaitForIdle(context.Background()))
		close(waitDone)
	}()

	select {
	case <-waitDone:
		t.Fatalf("WaitForIdle returned before work completed")
	case <-time.After(20 * time.Millisecond):
	}

	close(release)

	select {
	case <-waitDone:
	case <-time.After(time.Second):
		t.Fatalf("WaitForIdle did not return after work completion")
	}

	require.Equal(t, 0, sched.ActiveCount())
}

func TestTick_NoIdleAgents(t *testing.T) {
	st := &fakeStorage{
		batches: []storage.Batch{{ID: "b1", Items: []storage.BatchItem{{Input: "t1", Status: storage.ItemStatusPending}}, Status: storage.BatchStatusPending}},
		agents:  []storage.Agent{{ID: "a1", Status: storage.AgentStatusBusy}},
	}
	sched := New(st, &fakeWorker{}, zap.NewNop(), defaultMaxRetries)

	require.NoError(t, sched.Tick(context.Background()))
	require.Equal(t, storage.BatchStatusPending, st.batches[0].Status)
}

func TestTick_SkipsPausedAndInProgressBatches(t *testing.T) {
	st := &fakeStorage{
		batches: []storage.Batch{
			{ID: "paused", Status: storage.BatchStatusPaused, Items: []storage.BatchItem{{Input: "t0", Status: storage.ItemStatusPending}}},
			{ID: "active", Status: storage.BatchStatusPending, Items: []storage.BatchItem{{Input: "t1", Status: storage.ItemStatusInProgress}, {Input: "t2", Status: storage.ItemStatusPending}}},
		},
		agents: []storage.Agent{{ID: "a1", Status: storage.AgentStatusIdle}},
	}
	worker := &fakeWorker{}
	sched := New(st, worker, zap.NewNop(), defaultMaxRetries)

	require.NoError(t, sched.Tick(context.Background()))
	require.False(t, worker.called, "no work should be dispatched when batches are ineligible")
}

func TestPause_PausedBatchNotScheduled(t *testing.T) {
	st := &fakeStorage{
		batches: []storage.Batch{
			{ID: "paused", Status: storage.BatchStatusPaused, Items: []storage.BatchItem{{Input: "t0", Status: storage.ItemStatusPending}}},
		},
		agents: []storage.Agent{{ID: "a1", Status: storage.AgentStatusIdle}},
	}
	worker := &fakeWorker{}
	sched := New(st, worker, zap.NewNop(), defaultMaxRetries)

	require.NoError(t, sched.Tick(context.Background()))
	require.False(t, worker.called, "paused batch must not dispatch work")

	b := st.batches[0]
	require.Equal(t, storage.BatchStatusPaused, b.Status)
	require.Equal(t, storage.ItemStatusPending, b.Items[0].Status)
}

func TestPause_ResumeAllowsScheduling(t *testing.T) {
	st := &fakeStorage{
		batches: []storage.Batch{
			{ID: "paused", Status: storage.BatchStatusPaused, Items: []storage.BatchItem{{Input: "t0", Status: storage.ItemStatusPending}}},
		},
		agents: []storage.Agent{{ID: "a1", Status: storage.AgentStatusIdle}},
	}
	worker := &fakeWorker{}
	sched := New(st, worker, zap.NewNop(), defaultMaxRetries)

	require.NoError(t, sched.Tick(context.Background()))
	require.False(t, worker.called, "paused batch must not dispatch work")

	st.batches[0].Status = storage.BatchStatusPending
	worker.called = false

	require.NoError(t, sched.Tick(context.Background()))
	require.True(t, worker.called, "resumed batch should dispatch work")

	b := st.batches[0]
	require.Equal(t, storage.BatchStatusDone, b.Status)
	require.Equal(t, storage.ItemStatusDone, b.Items[0].Status)
	require.Equal(t, 1, b.Items[0].Attempts)
}

func TestTick_HappyPathClaimsAgentAndItem(t *testing.T) {
	st := &fakeStorage{
		batches: []storage.Batch{{
			ID:     "b1",
			Status: storage.BatchStatusPending,
			Items: []storage.BatchItem{
				{Input: "task-1", Status: storage.ItemStatusPending},
			},
		}},
		agents: []storage.Agent{{ID: "agent-1", Status: storage.AgentStatusIdle}},
	}
	worker := &fakeWorker{}
	sched := New(st, worker, zap.NewNop(), defaultMaxRetries)

	require.NoError(t, sched.Tick(context.Background()))

	require.True(t, worker.called)
	require.Equal(t, consumer.WorkItem{BatchID: "b1", TaskRef: "task-1"}, worker.item)

	b := st.batches[0]
	require.Equal(t, storage.BatchStatusDone, b.Status)
	require.Equal(t, storage.ItemStatusDone, b.Items[0].Status)
	require.Equal(t, 1, b.Items[0].Attempts)

	a := st.agents[0]
	require.Equal(t, storage.AgentStatusIdle, a.Status)
}

func TestTick_RollsBackOnWorkerError(t *testing.T) {
	st := &fakeStorage{
		batches: []storage.Batch{{ID: "b1", Status: storage.BatchStatusPending, Items: []storage.BatchItem{{Input: "task-1", Status: storage.ItemStatusPending}}}},
		agents:  []storage.Agent{{ID: "agent-1", Status: storage.AgentStatusIdle}},
	}
	worker := &fakeWorker{err: errors.New("boom")}
	sched := New(st, worker, zap.NewNop(), 2)

	err := sched.Tick(context.Background())
	require.Error(t, err)

	b := st.batches[0]
	require.Equal(t, storage.BatchStatusPending, b.Status)
	require.Equal(t, storage.ItemStatusPending, b.Items[0].Status)
	require.Equal(t, 1, b.Items[0].Attempts)
	require.Equal(t, storage.AgentStatusIdle, st.agents[0].Status)
}

func TestTick_RetryThenSuccess(t *testing.T) {
	st := &fakeStorage{
		batches: []storage.Batch{{
			ID:     "b1",
			Status: storage.BatchStatusPending,
			Items:  []storage.BatchItem{{Input: "task-1", Status: storage.ItemStatusPending}},
		}},
		agents: []storage.Agent{{ID: "agent-1", Status: storage.AgentStatusIdle}},
	}
	worker := &fakeWorker{responses: []error{errors.New("first"), nil}}
	sched := New(st, worker, zap.NewNop(), 2)

	err := sched.Tick(context.Background())
	require.Error(t, err)

	b := st.batches[0]
	require.Equal(t, storage.BatchStatusPending, b.Status)
	require.Equal(t, storage.ItemStatusPending, b.Items[0].Status)
	require.Equal(t, 1, b.Items[0].Attempts)
	require.Equal(t, storage.AgentStatusIdle, st.agents[0].Status)

	worker.called = false

	require.NoError(t, sched.Tick(context.Background()))
	require.Equal(t, 2, worker.callCount)

	b = st.batches[0]
	require.Equal(t, storage.BatchStatusDone, b.Status)
	require.Equal(t, storage.ItemStatusDone, b.Items[0].Status)
	require.Equal(t, 2, b.Items[0].Attempts)
	require.Equal(t, storage.AgentStatusIdle, st.agents[0].Status)
}

func TestTick_RetryExhausted(t *testing.T) {
	st := &fakeStorage{
		batches: []storage.Batch{{
			ID:     "b1",
			Status: storage.BatchStatusPending,
			Items:  []storage.BatchItem{{Input: "task-1", Status: storage.ItemStatusPending}},
		}},
		agents: []storage.Agent{{ID: "agent-1", Status: storage.AgentStatusIdle}},
	}
	worker := &fakeWorker{err: errors.New("always fails")}
	sched := New(st, worker, zap.NewNop(), 2)

	require.Error(t, sched.Tick(context.Background()))

	b := st.batches[0]
	require.Equal(t, storage.BatchStatusPending, b.Status)
	require.Equal(t, storage.ItemStatusPending, b.Items[0].Status)
	require.Equal(t, 1, b.Items[0].Attempts)

	require.Error(t, sched.Tick(context.Background()))

	b = st.batches[0]
	require.Equal(t, storage.BatchStatusFailed, b.Status)
	require.Equal(t, storage.ItemStatusFailed, b.Items[0].Status)
	require.Equal(t, 2, b.Items[0].Attempts)
	require.Equal(t, storage.AgentStatusIdle, st.agents[0].Status)
}

// --- fakes ---

type fakeStorage struct {
	mu               sync.Mutex
	batches          []storage.Batch
	agents           []storage.Agent
	updateBatchCalls int
}

func (f *fakeStorage) ListBatches(_ context.Context, _ int64) ([]storage.Batch, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]storage.Batch, len(f.batches))
	copy(out, f.batches)
	return out, nil
}

func (f *fakeStorage) UpdateBatch(_ context.Context, batch *storage.Batch) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := range f.batches {
		if f.batches[i].ID == batch.ID {
			f.batches[i] = *batch
			f.updateBatchCalls++
			return nil
		}
	}
	return fmt.Errorf("batch %s not found", batch.ID)
}

func (f *fakeStorage) ListAgents(_ context.Context) ([]storage.Agent, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]storage.Agent, len(f.agents))
	copy(out, f.agents)
	return out, nil
}

func (f *fakeStorage) UpdateAgent(_ context.Context, agent *storage.Agent) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := range f.agents {
		if f.agents[i].ID == agent.ID {
			f.agents[i] = *agent
			return nil
		}
	}
	return fmt.Errorf("agent %s not found", agent.ID)
}

// satisfy unused methods of storage.Storage for tests
var _ Storage = (*fakeStorage)(nil)

type fakeWorker struct {
	item      consumer.WorkItem
	called    bool
	err       error
	responses []error
	callCount int
}

func (f *fakeWorker) Process(_ context.Context, item consumer.WorkItem) error {
	f.called = true
	f.item = item
	resp := f.err
	if len(f.responses) > 0 {
		idx := f.callCount
		if idx >= len(f.responses) {
			idx = len(f.responses) - 1
		}
		resp = f.responses[idx]
	}
	f.callCount++
	return resp
}

type blockingWorker struct {
	started chan struct{}
	release chan struct{}
}

func (b *blockingWorker) Process(_ context.Context, item consumer.WorkItem) error {
	if b.started != nil {
		close(b.started)
	}
	if b.release != nil {
		<-b.release
	}
	return nil
}
