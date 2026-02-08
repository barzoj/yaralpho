package notify

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/barzoj/yaralpho/internal/config"
)

type stubConfig map[string]string

func (c stubConfig) Get(key string) (string, error) {
	val, ok := c[key]
	if !ok {
		return "", errors.New("not found")
	}
	return val, nil
}

func TestNewSlackNoWebhookReturnsNoop(t *testing.T) {
	n, err := NewSlack(stubConfig{}, zap.NewNop())
	require.NoError(t, err)

	_, ok := n.(Noop)
	require.True(t, ok, "expected Noop when webhook missing")
}

func TestPostPayloadAndStatus(t *testing.T) {
	var body string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(data)
		body = string(data)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	n, err := NewSlack(stubConfig{config.SlackWebhookKey: server.URL}, zap.NewNop())
	require.NoError(t, err)

	slack, ok := n.(*Slack)
	require.True(t, ok)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	err = slack.NotifyTaskFinished(ctx, "b1", "r1", "task-1", "done", "abc123")
	require.NoError(t, err)

	require.Contains(t, body, "\"batch_id\":\"b1\"")
	require.Contains(t, body, "\"run_id\":\"r1\"")
	require.Contains(t, body, "\"task_ref\":\"task-1\"")
	require.Contains(t, body, "\"status\":\"done\"")
	require.Contains(t, body, "\"commit_hash\":\"abc123\"")
}

func TestSlackErrorSurfaces(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	t.Cleanup(server.Close)

	n, err := NewSlack(stubConfig{config.SlackWebhookKey: server.URL}, zap.NewExample())
	require.NoError(t, err)

	slack := n.(*Slack)

	err = slack.NotifyBatchIdle(context.Background(), "batch-xyz")
	require.Error(t, err)
}

func TestSlackNilCtxIsHandled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	n, err := NewSlack(stubConfig{config.SlackWebhookKey: server.URL}, zap.NewNop())
	require.NoError(t, err)

	slack := n.(*Slack)

	require.NoError(t, slack.NotifyError(nil, "b1", "", "t1", errors.New("boom")))
}

func TestNewSlackNilConfigError(t *testing.T) {
	_, err := NewSlack(nil, zap.NewNop())
	require.Error(t, err)
}
