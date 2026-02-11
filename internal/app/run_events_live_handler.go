package app

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

const (
	wsReadTimeout  = 60 * time.Second
	wsWriteTimeout = 10 * time.Second
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

	rawCursor := r.URL.Query().Get("last_ingested")
	if rawCursor != "" {
		if _, err := time.Parse(time.RFC3339Nano, rawCursor); err != nil {
			writeError(w, http.StatusBadRequest, "invalid last_ingested")
			return
		}
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
		_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseInternalServerErr, "subscribe failed"), time.Now().Add(wsWriteTimeout))
		return
	}
	defer sub.Close()

	// Drain control frames to detect client disconnects.
	go func() {
		for {
			if _, _, err := conn.NextReader(); err != nil {
				cancel()
				return
			}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""), time.Now().Add(wsWriteTimeout))
			return
		case evt, ok := <-sub.Events:
			if !ok {
				_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""), time.Now().Add(wsWriteTimeout))
				return
			}

			payload, err := json.Marshal(evt)
			if err != nil {
				if a.logger != nil {
					a.logger.Warn(
						"marshal session event failed",
						zap.Error(err),
						zap.String("run_id", run.ID),
						zap.String("batch_id", run.BatchID),
						zap.String("session_id", run.SessionID),
					)
				}
				continue
			}

			_ = conn.SetWriteDeadline(time.Now().Add(wsWriteTimeout))
			if err := conn.WriteMessage(websocket.TextMessage, payload); err != nil {
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
				return
			}
		}
	}
}
