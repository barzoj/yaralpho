package mongo

import (
	"context"
	"fmt"

	"github.com/barzoj/yaralpho/internal/storage"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

func (c *Client) GetBatchProgress(ctx context.Context, batchID string) (storage.BatchProgress, error) {
	ctx, cancel := c.withTimeout(ctx)
	defer cancel()

	pipeline := mongo.Pipeline{
		bson.D{{Key: "$match", Value: bson.D{{Key: "batch_id", Value: batchID}}}},
		bson.D{{Key: "$group", Value: bson.D{
			{Key: "_id", Value: "$status"},
			{Key: "count", Value: bson.D{{Key: "$sum", Value: 1}}},
		}}},
	}

	cur, err := c.taskRuns.Aggregate(ctx, pipeline)
	if err != nil {
		return storage.BatchProgress{}, fmt.Errorf("aggregate progress: %w", err)
	}
	defer cur.Close(context.Background())

	progress := storage.BatchProgress{}
	for cur.Next(ctx) {
		var row struct {
			Status storage.TaskRunStatus `bson:"_id"`
			Count  int                   `bson:"count"`
		}
		if err := cur.Decode(&row); err != nil {
			return storage.BatchProgress{}, fmt.Errorf("decode progress: %w", err)
		}
		progress.Total += row.Count
		switch row.Status {
		case storage.TaskRunStatusRunning:
			progress.Running = row.Count
		case storage.TaskRunStatusSucceeded:
			progress.Succeeded = row.Count
		case storage.TaskRunStatusFailed:
			progress.Failed = row.Count
		case storage.TaskRunStatusStopped:
			progress.Stopped = row.Count
		default:
			progress.Pending += row.Count
		}
	}
	if err := cur.Err(); err != nil {
		return storage.BatchProgress{}, fmt.Errorf("iterate progress: %w", err)
	}

	// Pending = total - other counted
	counted := progress.Running + progress.Succeeded + progress.Failed + progress.Stopped
	if progress.Pending == 0 {
		progress.Pending = progress.Total - counted
	}

	return progress, nil
}
