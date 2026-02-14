package app

import (
	"net/http"
	"strconv"
)

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

	if err := a.scheduler.WaitForIdle(r.Context()); err != nil {
		writeError(w, http.StatusRequestTimeout, "drain wait canceled or timed out")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status":      "drained",
		"active_runs": a.scheduler.ActiveCount(),
	})
}
