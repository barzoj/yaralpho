package mongo

import (
	"context"
	"errors"
	"fmt"
	"strings"

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

func (c *Client) ListTaskRuns(ctx context.Context, batchID string) ([]storage.TaskRun, error) {
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
	return runs, nil
}
