package app

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/barzoj/yaralpho/internal/storage"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestRunEventsLiveHandlerRejectsInvalidCursor(t *testing.T) {
	st := newHandlerTestStorage()
	q := &handlerTestQueue{}
	app := newTestApp(t, st, q)
	app.logger = zap.NewExample()

	st.runs["run-1"] = storage.TaskRun{
		ID:        "run-1",
		BatchID:   "batch-1",
		SessionID: "session-1",
	}

	server := httptest.NewServer(app.Router())
	t.Cleanup(server.Close)

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/runs/run-1/events/live?last_ingested=not-a-time"
	_, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.Error(t, err)
	require.NotNil(t, resp)
	require.Equal(t, 400, resp.StatusCode)
}

func TestRunEventsLiveHandlerStreamsEvents(t *testing.T) {
	st := newHandlerTestStorage()
	q := &handlerTestQueue{}
	app := newTestApp(t, st, q)

	st.runs["run-1"] = storage.TaskRun{
		ID:        "run-1",
		BatchID:   "batch-1",
		SessionID: "session-1",
	}

	server := httptest.NewServer(app.Router())
	t.Cleanup(server.Close)

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/runs/run-1/events/live"
	conn, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		require.Failf(t, "handshake failed", "status=%v body=%s err=%v", statusFromResponse(resp), bodyFromResponse(resp), err)
	}
	defer conn.Close()

	event := storage.SessionEvent{
		BatchID:    "batch-1",
		RunID:      "run-1",
		SessionID:  "session-1",
		Event:      map[string]any{"type": "log", "data": "hello"},
		IngestedAt: time.Now().UTC(),
	}
	require.NoError(t, app.eventBus.Publish(context.Background(), event.SessionID, event))

	_, msg, err := conn.ReadMessage()
	require.NoError(t, err)

	var got storage.SessionEvent
	require.NoError(t, json.Unmarshal(msg, &got))
	require.Equal(t, event.RunID, got.RunID)
	require.Equal(t, event.BatchID, got.BatchID)
	require.Equal(t, event.SessionID, got.SessionID)
}

func statusFromResponse(resp *http.Response) any {
	if resp == nil {
		return nil
	}
	return resp.StatusCode
}

func bodyFromResponse(resp *http.Response) string {
	if resp == nil || resp.Body == nil {
		return ""
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return string(b)
}
