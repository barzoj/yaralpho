package app

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/barzoj/yaralpho/internal/bus"
	"github.com/barzoj/yaralpho/internal/storage"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func skipIfNoTCP(t *testing.T) {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("tcp listen not permitted in sandbox: %v", err)
	}
	_ = l.Close()
}

func TestRunEventsLiveHandlerRejectsInvalidCursor(t *testing.T) {
	skipIfNoTCP(t)

	st := newHandlerTestStorage()
	app := newTestApp(t, st)
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

func TestRunEventsLiveHandlerRejectsFutureCursor(t *testing.T) {
	skipIfNoTCP(t)

	st := newHandlerTestStorage()
	app := newTestApp(t, st)
	app.logger = zap.NewExample()

	st.runs["run-1"] = storage.TaskRun{
		ID:        "run-1",
		BatchID:   "batch-1",
		SessionID: "session-1",
	}

	server := httptest.NewServer(app.Router())
	t.Cleanup(server.Close)

	rawCursor := time.Now().Add(5 * time.Minute).UTC().Format(time.RFC3339Nano)
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/runs/run-1/events/live?last_ingested=" + rawCursor
	_, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.Error(t, err)
	require.NotNil(t, resp)
	require.Equal(t, 400, resp.StatusCode)
}

func TestRunEventsLiveHandlerStreamsEvents(t *testing.T) {
	skipIfNoTCP(t)

	st := newHandlerTestStorage()
	app := newTestApp(t, st)

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

	var got eventEnvelope
	require.NoError(t, json.Unmarshal(msg, &got))
	require.Equal(t, envelopeTypeEvent, got.Type)
	require.NotNil(t, got.Event)
	require.Equal(t, event.RunID, got.Event.RunID)
	require.Equal(t, event.BatchID, got.Event.BatchID)
	require.Equal(t, event.SessionID, got.Event.SessionID)
}

func TestRunEventsLiveHandlerBackfillsFromCursor(t *testing.T) {
	skipIfNoTCP(t)

	st := newHandlerTestStorage()
	app := newTestApp(t, st)

	st.runs["run-1"] = storage.TaskRun{
		ID:        "run-1",
		BatchID:   "batch-1",
		SessionID: "session-1",
	}

	t1 := time.Now().Add(-2 * time.Minute).UTC()
	t2 := t1.Add(30 * time.Second)
	t3 := t2.Add(30 * time.Second)

	st.events["session-1"] = []storage.SessionEvent{
		{
			BatchID:    "batch-1",
			RunID:      "run-1",
			SessionID:  "session-1",
			Event:      map[string]any{"type": "log", "data": "newer"},
			IngestedAt: t2,
		},
		{
			BatchID:    "batch-1",
			RunID:      "run-1",
			SessionID:  "session-1",
			Event:      map[string]any{"type": "log", "data": "oldest"},
			IngestedAt: t1,
		},
		{
			BatchID:    "batch-1",
			RunID:      "run-1",
			SessionID:  "session-1",
			Event:      map[string]any{"type": "log", "data": "dup-newer"},
			IngestedAt: t2,
		},
	}

	server := httptest.NewServer(app.Router())
	t.Cleanup(server.Close)

	cursor := t1.UTC().Format(time.RFC3339Nano)
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/runs/run-1/events/live?last_ingested=" + cursor
	conn, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		require.Failf(t, "handshake failed", "status=%v body=%s err=%v", statusFromResponse(resp), bodyFromResponse(resp), err)
	}
	defer conn.Close()

	// Backfill should send only the event after the cursor.
	_, msg, err := conn.ReadMessage()
	require.NoError(t, err)

	var backfill eventEnvelope
	require.NoError(t, json.Unmarshal(msg, &backfill))
	require.Equal(t, envelopeTypeEvent, backfill.Type)
	require.NotNil(t, backfill.Event)
	require.Equal(t, t2.Format(time.RFC3339Nano), backfill.Cursor)
	require.Equal(t, t2, backfill.Event.IngestedAt)

	// Publish a later event; it should stream once and not duplicate backfill.
	liveEvent := storage.SessionEvent{
		BatchID:    "batch-1",
		RunID:      "run-1",
		SessionID:  "session-1",
		Event:      map[string]any{"type": "log", "data": "latest"},
		IngestedAt: t3,
	}
	require.NoError(t, app.eventBus.Publish(context.Background(), liveEvent.SessionID, liveEvent))

	_, msg, err = conn.ReadMessage()
	require.NoError(t, err)

	var live eventEnvelope
	require.NoError(t, json.Unmarshal(msg, &live))
	require.Equal(t, t3.Format(time.RFC3339Nano), live.Cursor)
	require.Equal(t, t3, live.Event.IngestedAt)
	require.Equal(t, t3, live.Event.IngestedAt)

	// Attempt to publish the backfilled event again; it should be skipped.
	require.NoError(t, app.eventBus.Publish(context.Background(), liveEvent.SessionID, st.events["session-1"][1]))

	conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	_, _, err = conn.ReadMessage()
	require.Error(t, err)
}

func TestRunEventsLiveHandlerClosesOnSubscribeError(t *testing.T) {
	skipIfNoTCP(t)

	st := newHandlerTestStorage()
	app := newTestApp(t, st)

	app.eventBus = errBus{}
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

	_, msg, readErr := conn.ReadMessage()
	require.NoError(t, readErr)

	var env eventEnvelope
	require.NoError(t, json.Unmarshal(msg, &env))
	require.Equal(t, envelopeTypeError, env.Type)
	require.Contains(t, env.Error, "subscribe failed")

	_, _, readErr = conn.ReadMessage()
	var closeErr *websocket.CloseError
	require.ErrorAs(t, readErr, &closeErr)
	require.Equal(t, websocket.CloseInternalServerErr, closeErr.Code)
	require.Contains(t, closeErr.Text, "subscribe failed")
}

func TestRunEventsLiveHandlerClosesOnHeartbeatTimeout(t *testing.T) {
	skipIfNoTCP(t)

	oldReadTimeout := wsReadTimeout
	oldHeartbeat := wsHeartbeatInterval
	wsReadTimeout = 100 * time.Millisecond
	wsHeartbeatInterval = 20 * time.Millisecond
	t.Cleanup(func() {
		wsReadTimeout = oldReadTimeout
		wsHeartbeatInterval = oldHeartbeat
	})

	st := newHandlerTestStorage()
	app := newTestApp(t, st)
	app.logger = zap.NewExample()

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

	// Drop pongs to trigger timeout.
	conn.SetPingHandler(func(string) error { return nil })
	conn.SetPongHandler(func(string) error { return nil })
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))

	for {
		_, _, err = conn.ReadMessage()
		if err == nil {
			continue
		}
		var closeErr *websocket.CloseError
		require.ErrorAs(t, err, &closeErr)
		require.Equal(t, websocket.CloseGoingAway, closeErr.Code)
		require.Contains(t, closeErr.Text, "pong timeout")
		return
	}
}

func TestRunEventsLiveHandlerCleansUpOnWriteFailure(t *testing.T) {
	skipIfNoTCP(t)

	oldWriteJSON := wsWriteJSON
	wsWriteJSON = func(*websocket.Conn, any) error {
		return errors.New("forced write failure")
	}
	t.Cleanup(func() { wsWriteJSON = oldWriteJSON })

	st := newHandlerTestStorage()
	app := newTestApp(t, st)
	app.logger = zap.NewExample()
	recBus := newRecordingBus()
	app.eventBus = recBus

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
	require.NoError(t, recBus.Publish(context.Background(), event.SessionID, event))

	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, _, err = conn.ReadMessage()
	var closeErr *websocket.CloseError
	require.ErrorAs(t, err, &closeErr)
	require.Equal(t, websocket.CloseGoingAway, closeErr.Code)
	require.Contains(t, closeErr.Text, "write failed")

	require.Eventually(t, func() bool {
		return recBus.closed()
	}, time.Second, 10*time.Millisecond)
}

type errBus struct{}

func (errBus) Publish(ctx context.Context, sessionID string, evt storage.SessionEvent) error {
	return nil
}
func (errBus) Subscribe(ctx context.Context, sessionID string) (bus.Subscription, error) {
	return bus.Subscription{}, errors.New("subscribe failed")
}

type recordingBus struct {
	bus bus.Bus

	mu      sync.Mutex
	lastSub bus.Subscription
}

func newRecordingBus() *recordingBus {
	return &recordingBus{bus: bus.NewMemoryBus(bus.Config{})}
}

func (b *recordingBus) Publish(ctx context.Context, sessionID string, evt storage.SessionEvent) error {
	return b.bus.Publish(ctx, sessionID, evt)
}

func (b *recordingBus) Subscribe(ctx context.Context, sessionID string) (bus.Subscription, error) {
	sub, err := b.bus.Subscribe(ctx, sessionID)
	if err == nil {
		b.mu.Lock()
		b.lastSub = sub
		b.mu.Unlock()
	}
	return sub, err
}

func (b *recordingBus) closed() bool {
	b.mu.Lock()
	sub := b.lastSub
	b.mu.Unlock()
	if sub.Done() == nil {
		return false
	}
	select {
	case <-sub.Done():
		return true
	default:
		return false
	}
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
