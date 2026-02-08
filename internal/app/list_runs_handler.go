package app

import "net/http"

// listRunsHandler returns runs optionally filtered by batch_id and capped by
// limit.
func (a *App) listRunsHandler(w http.ResponseWriter, r *http.Request) {
	limit := parseLimit(r.URL.Query().Get("limit"), defaultListLimit, maxListLimit)
	batchID := r.URL.Query().Get("batch_id")

	runs, err := a.storage.ListTaskRuns(r.Context(), batchID)
	if err != nil {
		writeStorageError(a.logger, w, err)
		return
	}
	if len(runs) > limit {
		runs = runs[:limit]
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"runs":  runs,
		"count": len(runs),
	})
}
