package consumer

import (
	"context"
	"testing"
	"time"

	"github.com/barzoj/yaralpho/internal/copilot"
	"github.com/barzoj/yaralpho/internal/notify"
	"github.com/barzoj/yaralpho/internal/storage"
	"github.com/barzoj/yaralpho/internal/tracker"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/mongo"
	"go.uber.org/zap"
)

type stubTracker struct {
	title string
}

func (s stubTracker) IsEpic(ctx context.Context, ref string) (bool, error) { return false, nil }
func (s stubTracker) ListChildren(ctx context.Context, ref string) ([]string, error) {
	return nil, nil
}
func (s stubTracker) AddComment(ctx context.Context, ref string, text string) error { return nil }
func (s stubTracker) FetchComments(ctx context.Context, ref string) ([]tracker.Comment, error) {
	return nil, nil
}
func (s stubTracker) GetTitle(ctx context.Context, ref string) (string, error) { return s.title, nil }

type taskNotifier struct {
	finished []taskFinished
}

type taskFinished struct {
	batchID    string
	runID      string
	taskRef    string
	taskName   string
	status     string
	commitHash string
}

func (t *taskNotifier) NotifyEvent(ctx context.Context, event notify.Event) error { return nil }
func (t *taskNotifier) NotifyTaskFinished(ctx context.Context, batchID, runID, taskRef, taskName, status, commitHash string) error {
	t.finished = append(t.finished, taskFinished{batchID, runID, taskRef, taskName, status, commitHash})
	return nil
}
func (t *taskNotifier) NotifyBatchIdle(ctx context.Context, batchID string) error { return nil }
func (t *taskNotifier) NotifyError(ctx context.Context, batchID, runID, taskRef string, err error) error {
	return nil
}

type stubStorage struct {
	runs map[string]storage.TaskRun
}

func newStubStorage() *stubStorage {
	return &stubStorage{runs: make(map[string]storage.TaskRun)}
}

func (s *stubStorage) CreateRepository(ctx context.Context, repo *storage.Repository) error {
	return nil
}
func (s *stubStorage) UpdateRepository(ctx context.Context, repo *storage.Repository) error {
	return nil
}
func (s *stubStorage) GetRepository(ctx context.Context, id string) (*storage.Repository, error) {
	return nil, mongo.ErrNoDocuments
}
func (s *stubStorage) ListRepositories(ctx context.Context) ([]storage.Repository, error) {
	return nil, nil
}
func (s *stubStorage) DeleteRepository(ctx context.Context, id string) error { return nil }
func (s *stubStorage) RepositoryHasActiveBatches(ctx context.Context, id string) (bool, error) {
	return false, nil
}

func (s *stubStorage) CreateAgent(ctx context.Context, agent *storage.Agent) error { return nil }
func (s *stubStorage) UpdateAgent(ctx context.Context, agent *storage.Agent) error { return nil }
func (s *stubStorage) GetAgent(ctx context.Context, id string) (*storage.Agent, error) {
	return nil, mongo.ErrNoDocuments
}
func (s *stubStorage) ListAgents(ctx context.Context) ([]storage.Agent, error) { return nil, nil }
func (s *stubStorage) DeleteAgent(ctx context.Context, id string) error        { return nil }

func (s *stubStorage) CreateBatch(ctx context.Context, batch *storage.Batch) error { return nil }
func (s *stubStorage) UpdateBatch(ctx context.Context, batch *storage.Batch) error { return nil }
func (s *stubStorage) GetBatch(ctx context.Context, batchID string) (*storage.Batch, error) {
	return &storage.Batch{ID: batchID}, nil
}
func (s *stubStorage) ListBatches(ctx context.Context, limit int64) ([]storage.Batch, error) {
	return nil, nil
}
func (s *stubStorage) ListBatchesByRepository(ctx context.Context, repositoryID string, status storage.BatchStatus, limit int64) ([]storage.Batch, error) {
	return nil, nil
}
func (s *stubStorage) CreateTaskRun(ctx context.Context, run *storage.TaskRun) error {
	s.runs[run.ID] = *run
	return nil
}
func (s *stubStorage) UpdateTaskRun(ctx context.Context, run *storage.TaskRun) error {
	s.runs[run.ID] = *run
	return nil
}
func (s *stubStorage) GetTaskRun(ctx context.Context, runID string) (*storage.TaskRun, error) {
	run := s.runs[runID]
	return &run, nil
}
func (s *stubStorage) ListTaskRuns(ctx context.Context, batchID string) ([]storage.TaskRunSummary, error) {
	return nil, nil
}
func (s *stubStorage) ListTaskRunsByRepository(ctx context.Context, repositoryID string) ([]storage.TaskRunSummary, error) {
	return nil, nil
}
func (s *stubStorage) InsertSessionEvent(ctx context.Context, event *storage.SessionEvent) error {
	return nil
}
func (s *stubStorage) ListSessionEvents(ctx context.Context, sessionID string) ([]storage.SessionEvent, error) {
	return nil, nil
}
func (s *stubStorage) GetBatchProgress(ctx context.Context, batchID string) (storage.BatchProgress, error) {
	return storage.BatchProgress{}, nil
}

type stubCopilot struct{}

func (stubCopilot) StartSession(ctx context.Context, prompt, repoPath string) (string, <-chan copilot.RawEvent, func(), error) {
	ch := make(chan copilot.RawEvent)
	close(ch)
	return "session-1", ch, func() {}, nil
}

func TestExecuteTaskNotifiesTaskFinishedWithTitle(t *testing.T) {
	st := newStubStorage()
	nt := &taskNotifier{}
	tr := stubTracker{title: "Task One"}
	batch := &storage.Batch{ID: "batch-1"}

	status, err := executeTask(
		context.Background(),
		stubCopilot{},
		st,
		tr,
		nt,
		zap.NewNop(),
		"/repo",
		func() string { return "run-1" },
		func() time.Time { return time.Date(2026, 2, 12, 0, 0, 0, 0, time.UTC) },
		batch,
		"task-1",
		"",
		"prompt",
	)

	require.NoError(t, err)
	require.Equal(t, storage.TaskRunStatusSucceeded, status)
	require.Len(t, nt.finished, 1)
	require.Equal(t, taskFinished{batchID: "batch-1", runID: "run-1", taskRef: "task-1", taskName: "Task One", status: "succeeded"}, nt.finished[0])
}
