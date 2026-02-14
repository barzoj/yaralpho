package mongo

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/barzoj/yaralpho/internal/storage"
	"go.uber.org/zap"
)

func mongoConfigOrSkip(t *testing.T) (string, string) {
	t.Helper()
	uri := os.Getenv("YARALPHO_MONGODB_URI")
	if uri == "" {
		t.Skip("YARALPHO_MONGODB_URI not set; skipping integration tests")
	}
	db := os.Getenv("YARALPHO_MONGODB_DB")
	if db == "" {
		db = "yaralpho_test"
	}
	return uri, db + "_mongo_tests"
}

func TestMongoStorageCRUD(t *testing.T) {
	uri, db := mongoConfigOrSkip(t)
	logger := zap.NewExample()

	ctx := context.Background()
	client, err := New(ctx, uri, db, logger)
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	defer func() {
		_ = client.db.Drop(ctx)
		_ = client.Close(ctx)
	}()

	now := time.Now().UTC()
	batch := &storage.Batch{
		ID:           "batch-1",
		RepositoryID: "repo-1",
		CreatedAt:    now,
		UpdatedAt:    now,
		Items:        []storage.BatchItem{{Input: "task-1", Status: string(storage.BatchStatusCreated), Attempts: 0}},
		Status:       storage.BatchStatusCreated,
	}
	if err := client.CreateBatch(ctx, batch); err != nil {
		t.Fatalf("CreateBatch: %v", err)
	}

	fetchedBatch, err := client.GetBatch(ctx, batch.ID)
	if err != nil {
		t.Fatalf("GetBatch: %v", err)
	}
	if fetchedBatch.Status != storage.BatchStatusCreated {
		t.Fatalf("unexpected batch status: %s", fetchedBatch.Status)
	}

	batch.Status = storage.BatchStatusRunning
	if err := client.UpdateBatch(ctx, batch); err != nil {
		t.Fatalf("UpdateBatch: %v", err)
	}

	batches, err := client.ListBatches(ctx, 10)
	if err != nil {
		t.Fatalf("ListBatches: %v", err)
	}
	if len(batches) == 0 {
		t.Fatalf("expected batches, got none")
	}

	run := &storage.TaskRun{
		ID:        "run-1",
		BatchID:   batch.ID,
		TaskRef:   "TASK-1",
		SessionID: "session-1",
		StartedAt: now,
		Status:    storage.TaskRunStatusRunning,
	}
	if err := client.CreateTaskRun(ctx, run); err != nil {
		t.Fatalf("CreateTaskRun: %v", err)
	}

	finished := now.Add(time.Minute)
	run.FinishedAt = &finished
	run.Status = storage.TaskRunStatusSucceeded
	if err := client.UpdateTaskRun(ctx, run); err != nil {
		t.Fatalf("UpdateTaskRun: %v", err)
	}

	gotRun, err := client.GetTaskRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetTaskRun: %v", err)
	}
	if gotRun.Status != storage.TaskRunStatusSucceeded {
		t.Fatalf("unexpected run status: %s", gotRun.Status)
	}

	runs, err := client.ListTaskRuns(ctx, batch.ID)
	if err != nil {
		t.Fatalf("ListTaskRuns: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("expected 1 run, got %d", len(runs))
	}
	if runs[0].TotalEvents != 0 {
		t.Fatalf("expected 0 events, got %d", runs[0].TotalEvents)
	}

	event := &storage.SessionEvent{
		BatchID:    batch.ID,
		RunID:      run.ID,
		SessionID:  run.SessionID,
		Event:      map[string]any{"raw": true},
		IngestedAt: now,
	}
	if err := client.InsertSessionEvent(ctx, event); err != nil {
		t.Fatalf("InsertSessionEvent: %v", err)
	}

	events, err := client.ListSessionEvents(ctx, run.SessionID)
	if err != nil {
		t.Fatalf("ListSessionEvents: %v", err)
	}
	if len(events) != 1 || events[0].Event["raw"] != true {
		t.Fatalf("unexpected session events: %+v", events)
	}

	runsWithCounts, err := client.ListTaskRuns(ctx, batch.ID)
	if err != nil {
		t.Fatalf("ListTaskRuns after events: %v", err)
	}
	if runsWithCounts[0].TotalEvents != 1 {
		t.Fatalf("expected 1 event, got %d", runsWithCounts[0].TotalEvents)
	}

	progress, err := client.GetBatchProgress(ctx, batch.ID)
	if err != nil {
		t.Fatalf("GetBatchProgress: %v", err)
	}
	if progress.Total != 1 || progress.Succeeded != 1 {
		t.Fatalf("unexpected progress: %+v", progress)
	}
}
