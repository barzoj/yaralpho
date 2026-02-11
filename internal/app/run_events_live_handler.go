package app

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sort"
	"time"

	"github.com/barzoj/yaralpho/internal/storage"
	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

const (
	defaultWSReadTimeout  = 60 * time.Second
	defaultWSWriteTimeout = 10 * time.Second
	// Heartbeats (and pings) are sent on this cadence to detect stalled clients.
	defaultWSHeartbeatInterval = 15 * time.Second
)

var (
	wsReadTimeout       = defaultWSReadTimeout
	wsWriteTimeout      = defaultWSWriteTimeout
	wsHeartbeatInterval = defaultWSHeartbeatInterval

	wsWriteJSON = func(conn *websocket.Conn, v any) error {
		return conn.WriteJSON(v)
	}
	wsWritePingControl = func(conn *websocket.Conn, deadline time.Time) error {
		return conn.WriteControl(websocket.PingMessage, nil, deadline)
	}
)

// eventEnvelope encodes WebSocket frames for live events using a typed schema:
// event/error/heartbeat with optional cursor for ordering.
type eventEnvelope struct {
	Type   string                `json:"type"`
	Cursor string                `json:"cursor,omitempty"`
	Event  *storage.SessionEvent `json:"event,omitempty"`
	Error  string                `json:"error,omitempty"`
}

const (
	envelopeTypeEvent     = "event"
	envelopeTypeError     = "error"
	envelopeTypeHeartbeat = "heartbeat"
)

// runEventsLiveHandler upgrades to websocket and streams session events for a run.
// It validates parameters before upgrade, subscribes to the shared event bus, and
// cleans up on close or context cancellation.
func (a *App) runEventsLiveHandler(w http.ResponseWriter, r *http.Request) {
	if a.eventBus == nil {
		writeError(w, http.StatusServiceUnavailable, "event bus unavailable")
		return
	}

	runID := mux.Vars(r)["id"]
	if runID == "" {
		writeError(w, http.StatusBadRequest, "run id is required")
		return
	}

	run, err := a.storage.GetTaskRun(r.Context(), runID)
	if err != nil {
		writeStorageError(a.logger, w, err)
		return
	}
	if run.SessionID == "" {
		writeError(w, http.StatusNotFound, "session not found for run")
		return
	}

	cursor, err := parseCursor(r.URL.Query().Get("last_ingested"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		if a.logger != nil {
			a.logger.Warn(
				"websocket upgrade failed",
				zap.Error(err),
				zap.String("run_id", run.ID),
				zap.String("batch_id", run.BatchID),
				zap.String("session_id", run.SessionID),
			)
		}
		return
	}
	defer conn.Close()

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	conn.SetCloseHandler(func(code int, text string) error {
		if a.logger != nil {
			a.logger.Info(
				"websocket closed by client",
				zap.Int("code", code),
				zap.String("reason", text),
				zap.String("run_id", run.ID),
				zap.String("batch_id", run.BatchID),
				zap.String("session_id", run.SessionID),
			)
		}
		cancel()
		return nil
	})
	conn.SetPongHandler(func(string) error {
		_ = conn.SetReadDeadline(time.Now().Add(wsReadTimeout))
		return nil
	})
	_ = conn.SetReadDeadline(time.Now().Add(wsReadTimeout))

	sub, err := a.eventBus.Subscribe(ctx, run.SessionID)
	if err != nil {
		if a.logger != nil {
			a.logger.Warn(
				"event bus subscribe failed",
				zap.Error(err),
				zap.String("run_id", run.ID),
				zap.String("batch_id", run.BatchID),
				zap.String("session_id", run.SessionID),
			)
		}
		_ = writeErrorEnvelope(conn, "subscribe failed")
		writeClose(conn, websocket.CloseInternalServerErr, "subscribe failed", a.logger, run)
		return
	}
	defer sub.Close()

	events, err := a.storage.ListSessionEvents(ctx, run.SessionID)
	if err != nil {
		if a.logger != nil {
			a.logger.Warn(
				"list session events failed",
				zap.Error(err),
				zap.String("run_id", run.ID),
				zap.String("batch_id", run.BatchID),
				zap.String("session_id", run.SessionID),
			)
		}
		_ = writeErrorEnvelope(conn, "list session events failed")
		writeClose(conn, websocket.CloseInternalServerErr, "list session events failed", a.logger, run)
		return
	}

	sort.Slice(events, func(i, j int) bool {
		return events[i].IngestedAt.Before(events[j].IngestedAt)
	})

	seen := make(map[string]struct{}, len(events))
	for _, evt := range events {
		if !evt.IngestedAt.After(cursor) {
			continue
		}

		if err := writeEventEnvelope(conn, evt); err != nil {
			if a.logger != nil {
				a.logger.Info(
					"websocket write failed during backfill",
					zap.Error(err),
					zap.String("run_id", run.ID),
					zap.String("batch_id", run.BatchID),
					zap.String("session_id", run.SessionID),
				)
			}
			cancel()
			writeClose(conn, websocket.CloseGoingAway, "write failed during backfill", a.logger, run)
			return
		}
		seen[eventKey(evt)] = struct{}{}
		cursor = evt.IngestedAt
	}

	// Drain control frames to detect client disconnects.
	readErrCh := make(chan error, 1)
	go func() {
		for {
			if _, _, err := conn.NextReader(); err != nil {
				select {
				case readErrCh <- err:
				default:
				}
				cancel()
				return
			}
		}
	}()

	heartbeatTicker := time.NewTicker(wsHeartbeatInterval)
	defer heartbeatTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			writeClose(conn, websocket.CloseNormalClosure, "context done", a.logger, run)
			return
		case readErr := <-readErrCh:
			code, reason := closeReason(readErr)
			if a.logger != nil {
				a.logger.Info(
					"websocket read failed",
					zap.Error(readErr),
					zap.String("run_id", run.ID),
					zap.String("batch_id", run.BatchID),
					zap.String("session_id", run.SessionID),
				)
			}
			writeClose(conn, code, reason, a.logger, run)
			return
		case <-heartbeatTicker.C:
			if err := sendHeartbeat(conn, cursor); err != nil {
				if a.logger != nil {
					a.logger.Info(
						"websocket heartbeat failed",
						zap.Error(err),
						zap.String("run_id", run.ID),
						zap.String("batch_id", run.BatchID),
						zap.String("session_id", run.SessionID),
					)
				}
				cancel()
				writeClose(conn, websocket.CloseGoingAway, "heartbeat send failed", a.logger, run)
				return
			}
		case evt, ok := <-sub.Events:
			if !ok {
				writeClose(conn, websocket.CloseNormalClosure, "subscription closed", a.logger, run)
				return
			}

			if !evt.IngestedAt.After(cursor) {
				continue
			}
			key := eventKey(evt)
			if _, ok := seen[key]; ok {
				continue
			}

			if err := writeEventEnvelope(conn, evt); err != nil {
				if a.logger != nil {
					a.logger.Info(
						"websocket write failed",
						zap.Error(err),
						zap.String("run_id", run.ID),
						zap.String("batch_id", run.BatchID),
						zap.String("session_id", run.SessionID),
					)
				}
				cancel()
				writeClose(conn, websocket.CloseGoingAway, "write failed", a.logger, run)
				return
			}
			seen[key] = struct{}{}
			cursor = evt.IngestedAt
		}
	}
}

func parseCursor(raw string) (time.Time, error) {
	if raw == "" {
		return time.Time{}, nil
	}

	parsed, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid last_ingested")
	}
	if parsed.After(time.Now().UTC()) {
		return time.Time{}, fmt.Errorf("invalid last_ingested")
	}
	return parsed, nil
}

func writeEventEnvelope(conn *websocket.Conn, evt storage.SessionEvent) error {
	payload := eventEnvelope{
		Type:   envelopeTypeEvent,
		Cursor: evt.IngestedAt.UTC().Format(time.RFC3339Nano),
		Event:  &evt,
	}
	_ = conn.SetWriteDeadline(time.Now().Add(wsWriteTimeout))
	return wsWriteJSON(conn, payload)
}

func writeErrorEnvelope(conn *websocket.Conn, msg string) error {
	if msg == "" {
		msg = "unknown error"
	}
	payload := eventEnvelope{
		Type:  envelopeTypeError,
		Error: msg,
	}
	_ = conn.SetWriteDeadline(time.Now().Add(wsWriteTimeout))
	return wsWriteJSON(conn, payload)
}

func writeHeartbeatEnvelope(conn *websocket.Conn, cursor time.Time) error {
	payload := eventEnvelope{Type: envelopeTypeHeartbeat}
	if !cursor.IsZero() {
		payload.Cursor = cursor.UTC().Format(time.RFC3339Nano)
	}
	_ = conn.SetWriteDeadline(time.Now().Add(wsWriteTimeout))
	return wsWriteJSON(conn, payload)
}

func sendHeartbeat(conn *websocket.Conn, cursor time.Time) error {
	if err := writeHeartbeatEnvelope(conn, cursor); err != nil {
		return err
	}
	_ = conn.SetWriteDeadline(time.Now().Add(wsWriteTimeout))
	return wsWritePingControl(conn, time.Now().Add(wsWriteTimeout))
}

func eventKey(evt storage.SessionEvent) string {
	return fmt.Sprintf("%s|%s|%s|%s", evt.SessionID, evt.RunID, evt.BatchID, evt.IngestedAt.UTC().Format(time.RFC3339Nano))
}

func writeClose(conn *websocket.Conn, code int, reason string, logger *zap.Logger, run *storage.TaskRun) {
	if conn == nil {
		return
	}

	if reason != "" && len(reason) > 123 {
		reason = reason[:123]
	}

	_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(code, reason), time.Now().Add(wsWriteTimeout))

	if logger != nil && run != nil {
		logger.Info(
			"websocket closing",
			zap.Int("code", code),
			zap.String("reason", reason),
			zap.String("run_id", run.ID),
			zap.String("batch_id", run.BatchID),
			zap.String("session_id", run.SessionID),
		)
	}
}

func closeReason(err error) (int, string) {
	if err == nil {
		return websocket.CloseGoingAway, "client disconnected"
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return websocket.CloseGoingAway, "pong timeout"
	}
	var closeErr *websocket.CloseError
	if errors.As(err, &closeErr) {
		return closeErr.Code, closeErr.Text
	}
	return websocket.CloseGoingAway, "client read failed"
}
