package mongo

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/barzoj/yaralpho/internal/storage"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.uber.org/zap"
)

func (c *Client) CreateRepository(ctx context.Context, repo *storage.Repository) error {
	if repo == nil {
		return fmt.Errorf("repository is nil")
	}

	repo.CreatedAt = time.Now().UTC()
	repo.UpdatedAt = repo.CreatedAt

	ctx, cancel := c.withTimeout(ctx)
	defer cancel()

	if _, err := c.repositories.InsertOne(ctx, repo); err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return storage.ErrConflict
		}
		c.logger.Error("insert repository", zap.Error(err), zap.String("repository_id", repo.ID))
		return fmt.Errorf("insert repository: %w", err)
	}
	return nil
}

func (c *Client) UpdateRepository(ctx context.Context, repo *storage.Repository) error {
	if repo == nil {
		return fmt.Errorf("repository is nil")
	}
	repo.UpdatedAt = time.Now().UTC()

	ctx, cancel := c.withTimeout(ctx)
	defer cancel()

	res, err := c.repositories.UpdateOne(ctx, bson.M{"repository_id": repo.ID}, bson.M{"$set": repo})
	if err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return storage.ErrConflict
		}
		c.logger.Error("update repository", zap.Error(err), zap.String("repository_id", repo.ID))
		return fmt.Errorf("update repository: %w", err)
	}
	if res.MatchedCount == 0 {
		return mongo.ErrNoDocuments
	}
	return nil
}

func (c *Client) GetRepository(ctx context.Context, id string) (*storage.Repository, error) {
	ctx, cancel := c.withTimeout(ctx)
	defer cancel()

	var repo storage.Repository
	if err := c.repositories.FindOne(ctx, bson.M{"repository_id": id}).Decode(&repo); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, err
		}
		c.logger.Error("get repository", zap.Error(err), zap.String("repository_id", id))
		return nil, fmt.Errorf("get repository: %w", err)
	}
	return &repo, nil
}

func (c *Client) ListRepositories(ctx context.Context) ([]storage.Repository, error) {
	ctx, cancel := c.withTimeout(ctx)
	defer cancel()

	cur, err := c.repositories.Find(ctx, bson.M{}, optionsFindNewest)
	if err != nil {
		c.logger.Error("list repositories", zap.Error(err))
		return nil, fmt.Errorf("list repositories: %w", err)
	}
	defer cur.Close(context.Background())

	var repos []storage.Repository
	for cur.Next(ctx) {
		var repo storage.Repository
		if err := cur.Decode(&repo); err != nil {
			return nil, fmt.Errorf("decode repository: %w", err)
		}
		repos = append(repos, repo)
	}
	if err := cur.Err(); err != nil {
		return nil, fmt.Errorf("iterate repositories: %w", err)
	}
	return repos, nil
}

func (c *Client) DeleteRepository(ctx context.Context, id string) error {
	ctx, cancel := c.withTimeout(ctx)
	defer cancel()

	res, err := c.repositories.DeleteOne(ctx, bson.M{"repository_id": id})
	if err != nil {
		c.logger.Error("delete repository", zap.Error(err), zap.String("repository_id", id))
		return fmt.Errorf("delete repository: %w", err)
	}
	if res.DeletedCount == 0 {
		return mongo.ErrNoDocuments
	}
	return nil
}

func (c *Client) RepositoryHasActiveBatches(ctx context.Context, id string) (bool, error) {
	ctx, cancel := c.withTimeout(ctx)
	defer cancel()

	filter := bson.M{
		"repository_id": id,
		"status": bson.M{"$in": []storage.BatchStatus{
			storage.BatchStatusPending,
			storage.BatchStatusInProgress,
			storage.BatchStatusPaused,
			storage.BatchStatusCreated,
			storage.BatchStatusRunning,
			storage.BatchStatusIdle,
		}},
	}
	count, err := c.batches.CountDocuments(ctx, filter)
	if err != nil {
		return false, fmt.Errorf("count active batches: %w", err)
	}
	return count > 0, nil
}

var optionsFindNewest = optionsFindByCreatedDesc()
