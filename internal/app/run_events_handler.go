package app

import (
	"net/http"

	"github.com/gorilla/mux"
)

// runEventsHandler returns session events for a run with optional limit.
func (a *App) runEventsHandler(w http.ResponseWriter, r *http.Request) {
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

	limit := parseLimit(r.URL.Query().Get("limit"), defaultEventsLimit, maxEventsLimit)
	events, err := a.storage.ListSessionEvents(r.Context(), run.SessionID)
	if err != nil {
		writeStorageError(a.logger, w, err)
		return
	}

	truncated := false
	if len(events) > limit {
		events = events[:limit]
		truncated = true
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"run":              run,
		"repository_id":    run.RepositoryID,
		"batch_id":         run.BatchID,
		"session_id":       run.SessionID,
		"events":           events,
		"count":            len(events),
		"events_truncated": truncated,
		"event_limit_used": limit,
	})
}
