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

	err = slack.NotifyTaskFinished(ctx, "b1", "r1", "task-1", "Task One", "done", "abc123")
	require.NoError(t, err)

	require.Contains(t, body, "\"batch_id\":\"b1\"")
	require.Contains(t, body, "\"run_id\":\"r1\"")
	require.Contains(t, body, "\"task_ref\":\"task-1\"")
	require.Contains(t, body, "\"task_name\":\"Task One\"")
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

	require.NoError(t, slack.NotifyError(context.TODO(), "b1", "", "t1", errors.New("boom")))
}

func TestNewSlackNilConfigError(t *testing.T) {
	_, err := NewSlack(nil, zap.NewNop())
	require.Error(t, err)
}

func TestSlackTextForEventFormatsLifecycle(t *testing.T) {
	slack := &Slack{}

	cases := []struct {
		name  string
		event Event
		want  string
	}{
		{
			name: "task received",
			event: Event{Type: "task_received", BatchID: "b1", TaskRef: "task-1", TaskName: "Task One", Status: "pending"},
			want: "[task_received] batch=b1 | task=task-1 (Task One) | status=pending",
		},
		{
			name: "task started",
			event: Event{Type: "task_started", BatchID: "b1", TaskRef: "task-1", TaskName: "Task One"},
			want: "[task_started] batch=b1 | start batch for task task-1 (Task One)",
		},
		{
			name: "attempt started",
			event: Event{Type: "attempt_started", BatchID: "b1", TaskRef: "task-1", TaskName: "Task One", Status: "in_progress", Attempt: 1},
			want: "[attempt_started] batch=b1 | task=task-1 (Task One) | status=in_progress | attempt=1",
		},
		{
			name: "verification succeeded",
			event: Event{Type: "verification_succeeded", BatchID: "b1", TaskRef: "task-1", TaskName: "Task One", Status: "succeeded", Attempt: 1, Details: "Reason: looks good\nDetails: all checks passed"},
			want: "[verification_succeeded] batch=b1 | task=task-1 (Task One) | status=succeeded | attempt=1\nReason: looks good\nDetails: all checks passed",
		},
		{
			name: "task finished",
			event: Event{Type: "task_finished", BatchID: "b1", TaskRef: "task-1", TaskName: "Task One", Status: "succeeded", CommitHash: "abc123"},
			want: "[task_finished] batch=b1 | task=task-1 (Task One) | status=succeeded | commit=abc123",
		},
		{
			name: "batch idle",
			event: Event{Type: "batch_idle", BatchID: "b1", Status: "idle"},
			want: "[batch_idle] batch=b1 | status=idle",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, slack.textForEvent(tc.event))
		})
	}
}
