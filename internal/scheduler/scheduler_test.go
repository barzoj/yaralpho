package scheduler

import (
	"context"
	"errors"
	"testing"

	"github.com/barzoj/yaralpho/internal/consumer"
	"github.com/barzoj/yaralpho/internal/storage"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestLogFieldHelpers(t *testing.T) {
	fields := withBatchFields(storage.Batch{ID: "b1", RepositoryID: "r1"}, 2)
	require.Len(t, fields, 3)
	assertField(t, fields, "batch_id", "b1")
	assertField(t, fields, "repository_id", "r1")
	assertField(t, fields, "item_index", 2)

	agentFields := withAgentFields(&storage.Agent{ID: "a1"})
	require.Len(t, agentFields, 1)
	assertField(t, agentFields, "agent_id", "a1")

	attemptFields := withAttemptFields(3, 5)
	require.Len(t, attemptFields, 2)
	assertField(t, attemptFields, "attempt", 3)
	assertField(t, attemptFields, "max_retries", 5)
}

func TestSchedulerTick_LogsClaimSuccess(t *testing.T) {
	ctx := context.Background()
	core, logs := observer.New(zap.DebugLevel)
	logger := zap.New(core)

	st := &fakeStorage{
		batches: []storage.Batch{{
			ID:           "batch-1",
			RepositoryID: "repo-1",
			Status:       storage.BatchStatusPending,
			Items: []storage.BatchItem{{
				Input:    "task-1",
				Status:   storage.ItemStatusPending,
				Attempts: 0,
			}},
		}},
		agents: []storage.Agent{{
			ID:     "agent-1",
			Status: storage.AgentStatusIdle,
		}},
	}

	w := &fakeWorker{}
	s := New(st, w, logger, Options{MaxRetries: 3})

	err := s.Tick(ctx)
	require.NoError(t, err)

	require.True(t, logs.FilterMessage("claiming work item").Len() == 1)
	require.True(t, logs.FilterMessage("work item succeeded").Len() == 1)

	entry := logs.FilterMessage("claiming work item").All()[0]
	require.Equal(t, zap.InfoLevel, entry.Level)
	assertContextField(t, entry, "batch_id", "batch-1")
	assertContextField(t, entry, "agent_id", "agent-1")
	assertContextField(t, entry, "repository_id", "repo-1")
	assertContextField(t, entry, "item_index", 0)
}

func TestSchedulerTick_LogsRetryExhaustion(t *testing.T) {
	ctx := context.Background()
	core, logs := observer.New(zap.DebugLevel)
	logger := zap.New(core)

	st := &fakeStorage{
		batches: []storage.Batch{{
			ID:           "batch-2",
			RepositoryID: "repo-2",
			Status:       storage.BatchStatusPending,
			Items: []storage.BatchItem{{
				Input:    "task-err",
				Status:   storage.ItemStatusPending,
				Attempts: 0,
			}},
		}},
		agents: []storage.Agent{{
			ID:     "agent-2",
			Status: storage.AgentStatusIdle,
		}},
	}

	w := &fakeWorker{err: errors.New("boom")}
	s := New(st, w, logger, Options{MaxRetries: 1})

	err := s.Tick(ctx)
	require.Error(t, err)

	require.True(t, logs.FilterMessage("worker failed").Len() == 1)
	require.True(t, logs.FilterMessage("work item failed").Len() == 1)

	failEntry := logs.FilterMessage("work item failed").All()[0]
	require.Equal(t, zap.ErrorLevel, failEntry.Level)
	assertContextField(t, failEntry, "batch_id", "batch-2")
	assertContextField(t, failEntry, "agent_id", "agent-2")
	assertContextField(t, failEntry, "attempt", 1)
	assertContextField(t, failEntry, "max_retries", 1)
}

func TestSchedulerTick_TimeoutCleansUp(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	st := &cancelAwareStorage{
		batches: []storage.Batch{{
			ID:           "batch-timeout",
			RepositoryID: "repo-timeout",
			Status:       storage.BatchStatusPending,
			Items: []storage.BatchItem{{
				Input:    "task-timeout",
				Status:   storage.ItemStatusPending,
				Attempts: 0,
			}},
		}},
		agents: []storage.Agent{{
			ID:     "agent-timeout",
			Status: storage.AgentStatusIdle,
		}},
	}

	worker := &cancelingWorker{cancel: cancel}
	s := New(st, worker, zap.NewNop(), Options{MaxRetries: 1})

	err := s.Tick(ctx)
	require.ErrorIs(t, err, context.DeadlineExceeded)

	batch := st.batches[0]
	require.Equal(t, 1, batch.Items[0].Attempts)
	require.Equal(t, storage.ItemStatusFailed, batch.Items[0].Status)
	require.Equal(t, storage.BatchStatusFailed, batch.Status)
	require.Equal(t, storage.AgentStatusIdle, st.agents[0].Status)
}

func TestSchedulerStopEnablesDraining(t *testing.T) {
	s := New(nil, nil, zap.NewNop(), Options{})
	require.False(t, s.Draining())

	err := s.Stop(context.Background())
	require.NoError(t, err)
	require.True(t, s.Draining())

	err = s.Tick(context.Background())
	require.NoError(t, err)
}

func assertField(t *testing.T, fields []zap.Field, key string, value any) {
	t.Helper()
	for _, f := range fields {
		if f.Key == key {
			switch f.Type {
			case zapcore.StringType:
				require.Equal(t, value, f.String)
			case zapcore.Int64Type, zapcore.Int32Type, zapcore.Int16Type, zapcore.Int8Type:
				require.EqualValues(t, value, f.Integer)
			default:
				require.EqualValues(t, value, f.Interface)
			}
			return
		}
	}
	t.Fatalf("field %s not found", key)
}

func assertContextField(t *testing.T, entry observer.LoggedEntry, key string, value any) {
	t.Helper()
	for _, f := range entry.Context {
		if f.Key == key {
			switch f.Type {
			case zapcore.StringType:
				require.Equal(t, value, f.String)
			case zapcore.Int64Type, zapcore.Int32Type, zapcore.Int16Type, zapcore.Int8Type:
				require.EqualValues(t, value, f.Integer)
			default:
				require.EqualValues(t, value, f.Interface)
			}
			return
		}
	}
	t.Fatalf("field %s not found", key)
}

type fakeStorage struct {
	batches []storage.Batch
	agents  []storage.Agent
}

func (f *fakeStorage) ListBatches(ctx context.Context, limit int64) ([]storage.Batch, error) {
	return f.batches, nil
}

func (f *fakeStorage) UpdateBatch(ctx context.Context, batch *storage.Batch) error {
	for i := range f.batches {
		if f.batches[i].ID == batch.ID {
			f.batches[i] = *batch
			return nil
		}
	}
	return nil
}

func (f *fakeStorage) ListAgents(ctx context.Context) ([]storage.Agent, error) {
	return f.agents, nil
}

func (f *fakeStorage) UpdateAgent(ctx context.Context, agent *storage.Agent) error {
	for i := range f.agents {
		if f.agents[i].ID == agent.ID {
			f.agents[i] = *agent
			return nil
		}
	}
	return nil
}

type fakeWorker struct {
	err error
}

func (f *fakeWorker) Process(ctx context.Context, item consumer.WorkItem) error {
	return f.err
}

type cancelingWorker struct {
	cancel func()
}

func (f *cancelingWorker) Process(ctx context.Context, item consumer.WorkItem) error {
	if f.cancel != nil {
		f.cancel()
	}
	return context.DeadlineExceeded
}

type cancelAwareStorage struct {
	batches []storage.Batch
	agents  []storage.Agent
}

func (f *cancelAwareStorage) ListBatches(ctx context.Context, limit int64) ([]storage.Batch, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return f.batches, nil
}

func (f *cancelAwareStorage) UpdateBatch(ctx context.Context, batch *storage.Batch) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	for i := range f.batches {
		if f.batches[i].ID == batch.ID {
			f.batches[i] = *batch
			return nil
		}
	}
	return nil
}

func (f *cancelAwareStorage) ListAgents(ctx context.Context) ([]storage.Agent, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return f.agents, nil
}

func (f *cancelAwareStorage) UpdateAgent(ctx context.Context, agent *storage.Agent) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	for i := range f.agents {
		if f.agents[i].ID == agent.ID {
			f.agents[i] = *agent
			return nil
		}
	}
	return nil
}
