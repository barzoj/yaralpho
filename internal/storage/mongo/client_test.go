package mongo

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/barzoj/yaralpho/internal/storage"
	"go.mongodb.org/mongo-driver/bson"
	mongodriver "go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
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

	// Repositories
	repo := &storage.Repository{ID: "repo-1", Name: "Repo One", Path: "/tmp/repo1"}
	if err := client.CreateRepository(ctx, repo); err != nil {
		t.Fatalf("CreateRepository: %v", err)
	}
	gotRepo, err := client.GetRepository(ctx, repo.ID)
	if err != nil {
		t.Fatalf("GetRepository: %v", err)
	}
	if gotRepo.Path != repo.Path {
		t.Fatalf("repository path mismatch: %s vs %s", gotRepo.Path, repo.Path)
	}
	// uniqueness on name
	if err := client.CreateRepository(ctx, &storage.Repository{ID: "repo-dup", Name: repo.Name, Path: "/tmp/other"}); !errors.Is(err, storage.ErrConflict) {
		t.Fatalf("expected conflict on duplicate repo name, got %v", err)
	}

	now := time.Now().UTC()
	batch := &storage.Batch{
		ID:           "batch-1",
		RepositoryID: "repo-1",
		CreatedAt:    now,
		UpdatedAt:    now,
		Items:        []storage.BatchItem{{Input: "task-1", Status: storage.ItemStatusPending, Attempts: 0}},
		Status:       storage.BatchStatusPending,
	}
	if err := client.CreateBatch(ctx, batch); err != nil {
		t.Fatalf("CreateBatch: %v", err)
	}

	fetchedBatch, err := client.GetBatch(ctx, batch.ID)
	if err != nil {
		t.Fatalf("GetBatch: %v", err)
	}
	if fetchedBatch.Status != storage.BatchStatusPending {
		t.Fatalf("unexpected batch status: %s", fetchedBatch.Status)
	}
	if len(fetchedBatch.Items) != 1 || fetchedBatch.Items[0].Attempts != 0 {
		t.Fatalf("unexpected batch items: %+v", fetchedBatch.Items)
	}

	batch.Status = storage.BatchStatusInProgress
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

	// Agents
	agent := &storage.Agent{ID: "agent-1", Name: "Agent One", Runtime: "codex"}
	if err := client.CreateAgent(ctx, agent); err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}
	if err := client.CreateAgent(ctx, &storage.Agent{ID: "agent-dup", Name: agent.Name, Runtime: "codex"}); !errors.Is(err, storage.ErrConflict) {
		t.Fatalf("expected conflict on duplicate agent name, got %v", err)
	}
	agents, err := client.ListAgents(ctx)
	if err != nil || len(agents) == 0 {
		t.Fatalf("ListAgents: %v len=%d", err, len(agents))
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
	if run.RepositoryID == "" {
		t.Fatalf("expected repository_id backfilled on create")
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

func TestEnsureIndexesRecoversFromSpecConflicts(t *testing.T) {
	uri, db := mongoConfigOrSkip(t)
	db = db + "_index_conflict"

	ctx := context.Background()
	driverClient, err := mongodriver.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		t.Fatalf("connect mongo: %v", err)
	}
	defer func() { _ = driverClient.Disconnect(ctx) }()

	// Start from a clean slate.
	if err := driverClient.Database(db).Drop(ctx); err != nil {
		t.Fatalf("drop test db: %v", err)
	}

	// Seed a non-unique index with the same name to mirror pre-upgrade state.
	batches := driverClient.Database(db).Collection(batchesCollection)
	if _, err := batches.Indexes().CreateOne(ctx, mongodriver.IndexModel{Keys: bson.D{{Key: "batch_id", Value: 1}}}); err != nil {
		t.Fatalf("seed index: %v", err)
	}

	client, err := New(ctx, uri, db, zap.NewExample())
	if err != nil {
		t.Fatalf("New with conflicting index: %v", err)
	}
	defer func() {
		_ = client.db.Drop(ctx)
		_ = client.Close(ctx)
	}()

	cursor, err := client.batches.Indexes().List(ctx)
	if err != nil {
		t.Fatalf("list indexes: %v", err)
	}
	var indexes []bson.M
	if err := cursor.All(ctx, &indexes); err != nil {
		t.Fatalf("read indexes: %v", err)
	}

	var batchIndex bson.M
	for _, idx := range indexes {
		if name, ok := idx["name"].(string); ok && name == "batch_id_1" {
			batchIndex = idx
			break
		}
	}
	if batchIndex == nil {
		t.Fatalf("batch_id_1 index not found")
	}
	if unique, ok := batchIndex["unique"].(bool); !ok || !unique {
		t.Fatalf("expected batch_id_1 unique index, got %v", batchIndex["unique"])
	}
}
