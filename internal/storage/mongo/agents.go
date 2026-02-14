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

func (c *Client) CreateAgent(ctx context.Context, agent *storage.Agent) error {
	if agent == nil {
		return fmt.Errorf("agent is nil")
	}

	now := time.Now().UTC()
	if agent.CreatedAt.IsZero() {
		agent.CreatedAt = now
	}
	agent.UpdatedAt = now
	if agent.Status == "" {
		agent.Status = storage.AgentStatusIdle
	}

	ctx, cancel := c.withTimeout(ctx)
	defer cancel()

	if _, err := c.agents.InsertOne(ctx, agent); err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return storage.ErrConflict
		}
		c.logger.Error("insert agent", zap.Error(err), zap.String("agent_id", agent.ID))
		return fmt.Errorf("insert agent: %w", err)
	}
	return nil
}

func (c *Client) UpdateAgent(ctx context.Context, agent *storage.Agent) error {
	if agent == nil {
		return fmt.Errorf("agent is nil")
	}

	agent.UpdatedAt = time.Now().UTC()

	ctx, cancel := c.withTimeout(ctx)
	defer cancel()

	res, err := c.agents.UpdateOne(ctx, bson.M{"agent_id": agent.ID}, bson.M{"$set": agent})
	if err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return storage.ErrConflict
		}
		c.logger.Error("update agent", zap.Error(err), zap.String("agent_id", agent.ID))
		return fmt.Errorf("update agent: %w", err)
	}
	if res.MatchedCount == 0 {
		return mongo.ErrNoDocuments
	}
	return nil
}

func (c *Client) GetAgent(ctx context.Context, id string) (*storage.Agent, error) {
	ctx, cancel := c.withTimeout(ctx)
	defer cancel()

	var agent storage.Agent
	if err := c.agents.FindOne(ctx, bson.M{"agent_id": id}).Decode(&agent); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, err
		}
		c.logger.Error("get agent", zap.Error(err), zap.String("agent_id", id))
		return nil, fmt.Errorf("get agent: %w", err)
	}
	return &agent, nil
}

func (c *Client) ListAgents(ctx context.Context) ([]storage.Agent, error) {
	ctx, cancel := c.withTimeout(ctx)
	defer cancel()

	cur, err := c.agents.Find(ctx, bson.M{}, optionsFindNewest)
	if err != nil {
		c.logger.Error("list agents", zap.Error(err))
		return nil, fmt.Errorf("list agents: %w", err)
	}
	defer cur.Close(context.Background())

	var agents []storage.Agent
	for cur.Next(ctx) {
		var agent storage.Agent
		if err := cur.Decode(&agent); err != nil {
			return nil, fmt.Errorf("decode agent: %w", err)
		}
		agents = append(agents, agent)
	}
	if err := cur.Err(); err != nil {
		return nil, fmt.Errorf("iterate agents: %w", err)
	}
	return agents, nil
}

func (c *Client) DeleteAgent(ctx context.Context, id string) error {
	ctx, cancel := c.withTimeout(ctx)
	defer cancel()

	res, err := c.agents.DeleteOne(ctx, bson.M{"agent_id": id})
	if err != nil {
		c.logger.Error("delete agent", zap.Error(err), zap.String("agent_id", id))
		return fmt.Errorf("delete agent: %w", err)
	}
	if res.DeletedCount == 0 {
		return mongo.ErrNoDocuments
	}
	return nil
}
