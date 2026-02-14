package mongo

import (
	"context"
	"errors"
	"fmt"
	"strings"
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
	repositories  *mongo.Collection
	agents        *mongo.Collection
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
		repositories:  db.Collection(repositoriesCollection),
		agents:        db.Collection(agentsCollection),
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

func optionsFindByCreatedDesc() *options.FindOptions {
	return options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}})
}

// normalizeIndexModel ensures every index has a stable, explicit name so we can
// manage it idempotently.
func normalizeIndexModel(model mongo.IndexModel) (mongo.IndexModel, error) {
	name, err := indexModelName(model)
	if err != nil {
		return mongo.IndexModel{}, err
	}

	if model.Options == nil {
		model.Options = options.Index()
	}
	model.Options.SetName(name)
	return model, nil
}

func indexModelName(model mongo.IndexModel) (string, error) {
	if model.Options != nil && model.Options.Name != nil && *model.Options.Name != "" {
		return *model.Options.Name, nil
	}

	keysDoc, ok := model.Keys.(bson.D)
	if !ok {
		return "", fmt.Errorf("index keys must be bson.D to derive name: %T", model.Keys)
	}
	if len(keysDoc) == 0 {
		return "", errors.New("index keys cannot be empty")
	}

	parts := make([]string, len(keysDoc))
	for i, kv := range keysDoc {
		parts[i] = fmt.Sprintf("%s_%v", kv.Key, kv.Value)
	}
	return strings.Join(parts, "_"), nil
}

func (c *Client) createIndexes(ctx context.Context, collectionName string, coll *mongo.Collection, models []mongo.IndexModel) error {
	_, err := coll.Indexes().CreateMany(ctx, models)
	if err == nil {
		return nil
	}

	var cmdErr mongo.CommandError
	if errors.As(err, &cmdErr) && cmdErr.Name == "IndexKeySpecsConflict" {
		for _, model := range models {
			if model.Options == nil || model.Options.Name == nil || *model.Options.Name == "" {
				continue
			}
			name := *model.Options.Name
			if _, dropErr := coll.Indexes().DropOne(ctx, name); dropErr != nil {
				var dropCmdErr mongo.CommandError
				if errors.As(dropErr, &dropCmdErr) && dropCmdErr.Name == "IndexNotFound" {
					continue
				}
				return fmt.Errorf("drop conflicting index %s on %s: %w", name, collectionName, dropErr)
			}
			c.logger.Warn("dropped conflicting index", zap.String("collection", collectionName), zap.String("index", name))
		}

		_, err = coll.Indexes().CreateMany(ctx, models)
	}

	if err != nil {
		c.logger.Error("create indexes", zap.String("collection", collectionName), zap.Error(err))
		return fmt.Errorf("create indexes for %s: %w", collectionName, err)
	}

	return nil
}

// ensureIndexes creates required indexes needed by the storage interface.
func (c *Client) ensureIndexes(ctx context.Context) error {
	ctx, cancel := c.withTimeout(ctx)
	defer cancel()

	definitions := []struct {
		name   string
		coll   *mongo.Collection
		models []mongo.IndexModel
	}{
		{
			name: repositoriesCollection,
			coll: c.repositories,
			models: []mongo.IndexModel{
				{Keys: bson.D{{Key: "repository_id", Value: 1}}, Options: options.Index().SetUnique(true)},
				{Keys: bson.D{{Key: "name", Value: 1}}, Options: options.Index().SetUnique(true)},
				{Keys: bson.D{{Key: "path", Value: 1}}, Options: options.Index().SetUnique(true)},
			},
		},
		{
			name: agentsCollection,
			coll: c.agents,
			models: []mongo.IndexModel{
				{Keys: bson.D{{Key: "agent_id", Value: 1}}, Options: options.Index().SetUnique(true)},
				{Keys: bson.D{{Key: "name", Value: 1}}, Options: options.Index().SetUnique(true)},
				{Keys: bson.D{{Key: "runtime", Value: 1}}},
			},
		},
		{
			name: batchesCollection,
			coll: c.batches,
			models: []mongo.IndexModel{
				{Keys: bson.D{{Key: "batch_id", Value: 1}}, Options: options.Index().SetUnique(true)},
				{Keys: bson.D{{Key: "repository_id", Value: 1}}},
				{Keys: bson.D{{Key: "repository_id", Value: 1}, {Key: "status", Value: 1}}},
				{Keys: bson.D{{Key: "repository_id", Value: 1}, {Key: "created_at", Value: -1}}},
			},
		},
		{
			name: taskRunsCollection,
			coll: c.taskRuns,
			models: []mongo.IndexModel{
				{Keys: bson.D{{Key: "run_id", Value: 1}}, Options: options.Index().SetUnique(true)},
				{Keys: bson.D{{Key: "batch_id", Value: 1}}},
				{Keys: bson.D{{Key: "batch_id", Value: 1}, {Key: "status", Value: 1}}},
				{Keys: bson.D{{Key: "repository_id", Value: 1}}},
				{Keys: bson.D{{Key: "repository_id", Value: 1}, {Key: "started_at", Value: -1}}},
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

		models := make([]mongo.IndexModel, 0, len(def.models))
		for _, model := range def.models {
			normalized, err := normalizeIndexModel(model)
			if err != nil {
				return fmt.Errorf("normalize index for %s: %w", def.name, err)
			}
			models = append(models, normalized)
		}

		if err := c.createIndexes(ctx, def.name, def.coll, models); err != nil {
			return err
		}
	}

	return nil
}
