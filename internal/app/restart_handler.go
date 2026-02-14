package app

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/barzoj/yaralpho/internal/config"
	"go.uber.org/zap"
)

const defaultRestartWaitTimeout = 10 * 60 * time.Second

// restartHandler sets the scheduler into draining mode. When wait=true, the
// request blocks until all active runs finish; otherwise it returns 202
// immediately while draining continues.
func (a *App) restartHandler(w http.ResponseWriter, r *http.Request) {
	if a.scheduler == nil {
		writeError(w, http.StatusServiceUnavailable, "scheduler not configured")
		return
	}

	waitParam := r.URL.Query().Get("wait")
	wait := false
	if waitParam != "" {
		parsed, err := strconv.ParseBool(waitParam)
		if err != nil {
			writeError(w, http.StatusBadRequest, "wait must be boolean")
			return
		}
		wait = parsed
	}

	a.scheduler.SetDraining(true)

	if !wait {
		writeJSON(w, http.StatusAccepted, map[string]any{
			"status":      "draining",
			"active_runs": a.scheduler.ActiveCount(),
		})
		return
	}

	waitCtx, cancel := context.WithTimeout(r.Context(), a.restartWaitTimeout())
	defer cancel()

	if err := a.scheduler.WaitForIdle(waitCtx); err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			writeError(w, http.StatusRequestTimeout, "drain wait timed out")
			return
		}
		writeError(w, http.StatusRequestTimeout, "drain wait canceled or timed out")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status":      "drained",
		"active_runs": a.scheduler.ActiveCount(),
	})
}

func (a *App) restartWaitTimeout() time.Duration {
	if a == nil || a.cfg == nil {
		return defaultRestartWaitTimeout
	}
	value, err := a.cfg.Get(config.RestartWaitTimeoutKey)
	if err != nil || strings.TrimSpace(value) == "" {
		return defaultRestartWaitTimeout
	}
	dur, err := time.ParseDuration(value)
	if err != nil || dur <= 0 {
		a.logger.Warn("invalid restart wait timeout; using default", zap.String("value", value), zap.Error(err))
		return defaultRestartWaitTimeout
	}
	return dur
}
