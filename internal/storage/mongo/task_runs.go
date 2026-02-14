package mongo

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/barzoj/yaralpho/internal/storage"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.uber.org/zap"
)

func (c *Client) CreateTaskRun(ctx context.Context, run *storage.TaskRun) error {
	if run == nil {
		return fmt.Errorf("run is nil")
	}

	if run.StartedAt.IsZero() {
		run.StartedAt = time.Now().UTC()
	}

	// Backfill repository_id from batch when omitted to keep documents aligned
	// with repository-scoped queries and indexes.
	if run.RepositoryID == "" && run.BatchID != "" {
		batch, err := c.GetBatch(ctx, run.BatchID)
		if err == nil && batch != nil {
			run.RepositoryID = batch.RepositoryID
		}
	}

	ctx, cancel := c.withTimeout(ctx)
	defer cancel()

	if _, err := c.taskRuns.InsertOne(ctx, run); err != nil {
		c.logger.Error("insert task run", zap.Error(err), zap.String("run_id", run.ID))
		return fmt.Errorf("insert task run: %w", err)
	}
	return nil
}

func (c *Client) UpdateTaskRun(ctx context.Context, run *storage.TaskRun) error {
	if run == nil {
		return fmt.Errorf("run is nil")
	}

	if run.FinishedAt == nil && (run.Status == storage.TaskRunStatusSucceeded || run.Status == storage.TaskRunStatusFailed || run.Status == storage.TaskRunStatusStopped) {
		now := time.Now().UTC()
		run.FinishedAt = &now
	}

	if run.RepositoryID == "" && run.BatchID != "" {
		batch, err := c.GetBatch(ctx, run.BatchID)
		if err == nil && batch != nil {
			run.RepositoryID = batch.RepositoryID
		}
	}

	ctx, cancel := c.withTimeout(ctx)
	defer cancel()

	res, err := c.taskRuns.UpdateOne(ctx, bson.M{"run_id": run.ID}, bson.M{"$set": run})
	if err != nil {
		c.logger.Error("update task run", zap.Error(err), zap.String("run_id", run.ID))
		return fmt.Errorf("update task run: %w", err)
	}
	if res.MatchedCount == 0 {
		return mongo.ErrNoDocuments
	}
	return nil
}

func (c *Client) GetTaskRun(ctx context.Context, runID string) (*storage.TaskRun, error) {
	ctx, cancel := c.withTimeout(ctx)
	defer cancel()

	var run storage.TaskRun
	if err := c.taskRuns.FindOne(ctx, bson.M{"run_id": runID}).Decode(&run); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, err
		}
		c.logger.Error("get task run", zap.Error(err), zap.String("run_id", runID))
		return nil, fmt.Errorf("get task run: %w", err)
	}
	return &run, nil
}

func (c *Client) ListTaskRuns(ctx context.Context, batchID string) ([]storage.TaskRunSummary, error) {
	ctx, cancel := c.withTimeout(ctx)
	defer cancel()

	filter := bson.M{}
	if strings.TrimSpace(batchID) != "" {
		filter["batch_id"] = batchID
	}

	cur, err := c.taskRuns.Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "started_at", Value: -1}}))
	if err != nil {
		c.logger.Error("list task runs", zap.Error(err), zap.String("batch_id", batchID))
		return nil, fmt.Errorf("list task runs: %w", err)
	}
	defer cur.Close(context.Background())

	runs := make([]storage.TaskRun, 0)
	for cur.Next(ctx) {
		var run storage.TaskRun
		if err := cur.Decode(&run); err != nil {
			return nil, fmt.Errorf("decode task run: %w", err)
		}
		runs = append(runs, run)
	}
	if err := cur.Err(); err != nil {
		return nil, fmt.Errorf("iterate task runs: %w", err)
	}

	if len(runs) == 0 {
		return []storage.TaskRunSummary{}, nil
	}

	runIDs := make([]string, 0, len(runs))
	for _, run := range runs {
		runIDs = append(runIDs, run.ID)
	}

	counts, err := c.fetchEventCounts(ctx, runIDs)
	if err != nil {
		return nil, err
	}

	summaries := make([]storage.TaskRunSummary, 0, len(runs))
	for _, run := range runs {
		summaries = append(summaries, storage.TaskRunSummary{
			TaskRun:     run,
			TotalEvents: counts[run.ID],
		})
	}

	return summaries, nil
}

func (c *Client) ListTaskRunsByRepository(ctx context.Context, repositoryID string) ([]storage.TaskRunSummary, error) {
	ctx, cancel := c.withTimeout(ctx)
	defer cancel()

	filter := bson.M{"repository_id": repositoryID}
	cur, err := c.taskRuns.Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "started_at", Value: -1}}))
	if err != nil {
		c.logger.Error("list task runs by repo", zap.Error(err), zap.String("repository_id", repositoryID))
		return nil, fmt.Errorf("list task runs by repo: %w", err)
	}
	defer cur.Close(context.Background())

	runs := make([]storage.TaskRun, 0)
	for cur.Next(ctx) {
		var run storage.TaskRun
		if err := cur.Decode(&run); err != nil {
			return nil, fmt.Errorf("decode task run: %w", err)
		}
		runs = append(runs, run)
	}
	if err := cur.Err(); err != nil {
		return nil, fmt.Errorf("iterate task runs: %w", err)
	}

	runIDs := make([]string, 0, len(runs))
	for _, run := range runs {
		runIDs = append(runIDs, run.ID)
	}

	counts, err := c.fetchEventCounts(ctx, runIDs)
	if err != nil {
		return nil, err
	}

	summaries := make([]storage.TaskRunSummary, 0, len(runs))
	for _, run := range runs {
		summaries = append(summaries, storage.TaskRunSummary{
			TaskRun:     run,
			TotalEvents: counts[run.ID],
		})
	}
	return summaries, nil
}

func (c *Client) fetchEventCounts(ctx context.Context, runIDs []string) (map[string]int64, error) {
	if len(runIDs) == 0 {
		return map[string]int64{}, nil
	}

	ctx, cancel := c.withTimeout(ctx)
	defer cancel()

	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.M{"run_id": bson.M{"$in": runIDs}}}},
		{{Key: "$group", Value: bson.M{"_id": "$run_id", "total": bson.M{"$sum": 1}}}},
	}

	cur, err := c.sessionEvents.Aggregate(ctx, pipeline)
	if err != nil {
		c.logger.Error("aggregate session events for run counts", zap.Error(err))
		return nil, fmt.Errorf("aggregate session events: %w", err)
	}
	defer cur.Close(context.Background())

	counts := make(map[string]int64, len(runIDs))
	for cur.Next(ctx) {
		var doc struct {
			ID    string `bson:"_id"`
			Total int64  `bson:"total"`
		}
		if err := cur.Decode(&doc); err != nil {
			return nil, fmt.Errorf("decode session events aggregate: %w", err)
		}
		counts[doc.ID] = doc.Total
	}
	if err := cur.Err(); err != nil {
		return nil, fmt.Errorf("iterate session events aggregate: %w", err)
	}

	// ensure zero default for runs with no events
	for _, id := range runIDs {
		if _, ok := counts[id]; !ok {
			counts[id] = 0
		}
	}

	return counts, nil
}
