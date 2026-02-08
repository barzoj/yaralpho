package mongo

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.uber.org/zap"
)

const defaultTimeout = 5 * time.Second

// Client implements the storage.Storage interface using MongoDB.
type Client struct {
	client        *mongo.Client
	db            *mongo.Database
	batches       *mongo.Collection
	taskRuns      *mongo.Collection
	sessionEvents *mongo.Collection
	logger        *zap.Logger
	timeout       time.Duration
}

// New creates a Mongo-backed storage client and ensures required indexes.
func New(ctx context.Context, uri, dbName string, logger *zap.Logger) (*Client, error) {
	if logger == nil {
		logger = zap.NewNop()
	}

	if uri == "" {
		return nil, fmt.Errorf("mongo uri is required")
	}
	if dbName == "" {
		return nil, fmt.Errorf("mongo database name is required")
	}

	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		logger.Error("connect mongo", zap.Error(err))
		return nil, fmt.Errorf("connect mongo: %w", err)
	}

	db := client.Database(dbName)
	storageClient := &Client{
		client:        client,
		db:            db,
		batches:       db.Collection(batchesCollection),
		taskRuns:      db.Collection(taskRunsCollection),
		sessionEvents: db.Collection(sessionEventsCollection),
		logger:        logger,
		timeout:       defaultTimeout,
	}

	if err := storageClient.ensureIndexes(context.Background()); err != nil {
		return nil, err
	}

	return storageClient, nil
}

// Close disconnects the underlying Mongo client.
func (c *Client) Close(ctx context.Context) error {
	if c == nil || c.client == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	if err := c.client.Disconnect(ctx); err != nil && !errors.Is(err, mongo.ErrClientDisconnected) {
		c.logger.Error("disconnect mongo", zap.Error(err))
		return fmt.Errorf("disconnect mongo: %w", err)
	}
	return nil
}

func (c *Client) withTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, c.timeout)
}

// ensureIndexes creates non-unique indexes needed by the storage interface.
func (c *Client) ensureIndexes(ctx context.Context) error {
	ctx, cancel := c.withTimeout(ctx)
	defer cancel()

	definitions := []struct {
		name   string
		coll   *mongo.Collection
		models []mongo.IndexModel
	}{
		{
			name: batchesCollection,
			coll: c.batches,
			models: []mongo.IndexModel{
				{Keys: bson.D{{Key: "batch_id", Value: 1}}},
			},
		},
		{
			name: taskRunsCollection,
			coll: c.taskRuns,
			models: []mongo.IndexModel{
				{Keys: bson.D{{Key: "run_id", Value: 1}}},
				{Keys: bson.D{{Key: "batch_id", Value: 1}}},
				{Keys: bson.D{{Key: "batch_id", Value: 1}, {Key: "status", Value: 1}}},
			},
		},
		{
			name: sessionEventsCollection,
			coll: c.sessionEvents,
			models: []mongo.IndexModel{
				{Keys: bson.D{{Key: "session_id", Value: 1}, {Key: "ingested_at", Value: 1}}},
				{Keys: bson.D{{Key: "run_id", Value: 1}}},
			},
		},
	}

	for _, def := range definitions {
		if def.coll == nil {
			return fmt.Errorf("collection %s is not initialized", def.name)
		}

		if len(def.models) == 0 {
			continue
		}

		_, err := def.coll.Indexes().CreateMany(ctx, def.models)
		if err != nil {
			c.logger.Error("create indexes", zap.String("collection", def.name), zap.Error(err))
			return fmt.Errorf("create indexes for %s: %w", def.name, err)
		}
	}

	return nil
}
