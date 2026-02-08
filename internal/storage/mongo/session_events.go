package mongo

import (
	"context"
	"fmt"

	"github.com/barzoj/yaralpho/internal/storage"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.uber.org/zap"
)

func (c *Client) InsertSessionEvent(ctx context.Context, event *storage.SessionEvent) error {
	if event == nil {
		return fmt.Errorf("event is nil")
	}

	ctx, cancel := c.withTimeout(ctx)
	defer cancel()

	if _, err := c.sessionEvents.InsertOne(ctx, event); err != nil {
		c.logger.Error("insert session event", zap.Error(err), zap.String("session_id", event.SessionID), zap.String("run_id", event.RunID))
		return fmt.Errorf("insert session event: %w", err)
	}
	return nil
}

func (c *Client) ListSessionEvents(ctx context.Context, sessionID string) ([]storage.SessionEvent, error) {
	ctx, cancel := c.withTimeout(ctx)
	defer cancel()

	cur, err := c.sessionEvents.Find(
		ctx,
		bson.M{"session_id": sessionID},
		options.Find().SetSort(bson.D{{Key: "ingested_at", Value: 1}}),
	)
	if err != nil {
		c.logger.Error("list session events", zap.Error(err), zap.String("session_id", sessionID))
		return nil, fmt.Errorf("list session events: %w", err)
	}
	defer cur.Close(context.Background())

	events := make([]storage.SessionEvent, 0)
	for cur.Next(ctx) {
		var evt storage.SessionEvent
		if err := cur.Decode(&evt); err != nil {
			return nil, fmt.Errorf("decode session event: %w", err)
		}
		events = append(events, evt)
	}
	if err := cur.Err(); err != nil {
		return nil, fmt.Errorf("iterate session events: %w", err)
	}
	return events, nil
}
