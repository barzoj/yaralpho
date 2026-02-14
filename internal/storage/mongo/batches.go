package mongo

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/barzoj/yaralpho/internal/storage"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.uber.org/zap"
)

func (c *Client) CreateBatch(ctx context.Context, batch *storage.Batch) error {
	if batch == nil {
		return fmt.Errorf("batch is nil")
	}

	now := time.Now().UTC()
	if batch.CreatedAt.IsZero() {
		batch.CreatedAt = now
	}
	batch.UpdatedAt = now

	if batch.Status == "" {
		batch.Status = storage.BatchStatusPending
	}

	for i := range batch.Items {
		if batch.Items[i].Status == "" {
			batch.Items[i].Status = storage.ItemStatusPending
		}
		if batch.Items[i].Attempts < 0 {
			batch.Items[i].Attempts = 0
		}
	}

	ctx, cancel := c.withTimeout(ctx)
	defer cancel()

	if _, err := c.batches.InsertOne(ctx, batch); err != nil {
		c.logger.Error("insert batch", zap.Error(err), zap.String("batch_id", batch.ID))
		return fmt.Errorf("insert batch: %w", err)
	}
	return nil
}

func (c *Client) UpdateBatch(ctx context.Context, batch *storage.Batch) error {
	if batch == nil {
		return fmt.Errorf("batch is nil")
	}

	batch.UpdatedAt = time.Now().UTC()

	ctx, cancel := c.withTimeout(ctx)
	defer cancel()

	res, err := c.batches.UpdateOne(ctx, bson.M{"batch_id": batch.ID}, bson.M{"$set": batch})
	if err != nil {
		c.logger.Error("update batch", zap.Error(err), zap.String("batch_id", batch.ID))
		return fmt.Errorf("update batch: %w", err)
	}
	if res.MatchedCount == 0 {
		return mongo.ErrNoDocuments
	}
	return nil
}

func (c *Client) GetBatch(ctx context.Context, batchID string) (*storage.Batch, error) {
	ctx, cancel := c.withTimeout(ctx)
	defer cancel()

	var batch storage.Batch
	if err := c.batches.FindOne(ctx, bson.M{"batch_id": batchID}).Decode(&batch); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, err
		}
		c.logger.Error("get batch", zap.Error(err), zap.String("batch_id", batchID))
		return nil, fmt.Errorf("get batch: %w", err)
	}
	return &batch, nil
}

func (c *Client) ListBatches(ctx context.Context, limit int64) ([]storage.Batch, error) {
	ctx, cancel := c.withTimeout(ctx)
	defer cancel()

	findOpts := options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}})
	if limit > 0 {
		findOpts.SetLimit(limit)
	}

	cur, err := c.batches.Find(ctx, bson.M{}, findOpts)
	if err != nil {
		c.logger.Error("list batches", zap.Error(err))
		return nil, fmt.Errorf("list batches: %w", err)
	}
	defer cur.Close(context.Background())

	batches := make([]storage.Batch, 0)
	for cur.Next(ctx) {
		var batch storage.Batch
		if err := cur.Decode(&batch); err != nil {
			return nil, fmt.Errorf("decode batch: %w", err)
		}
		batches = append(batches, batch)
	}
	if err := cur.Err(); err != nil {
		return nil, fmt.Errorf("iterate batches: %w", err)
	}
	return batches, nil
}
