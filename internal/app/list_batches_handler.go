package app

import (
	"net/http"
)

// listBatchesHandler returns recent batches with optional limit pagination.
func (a *App) listBatchesHandler(w http.ResponseWriter, r *http.Request) {
	limit := parseLimit(r.URL.Query().Get("limit"), defaultListLimit, maxListLimit)

	batches, err := a.storage.ListBatches(r.Context(), int64(limit))
	if err != nil {
		writeStorageError(a.logger, w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"batches": batches,
		"count":   len(batches),
	})
}
