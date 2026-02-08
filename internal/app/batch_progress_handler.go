package app

import (
	"net/http"

	"github.com/gorilla/mux"
)

// batchProgressHandler returns counts for a batch.
func (a *App) batchProgressHandler(w http.ResponseWriter, r *http.Request) {
	batchID := mux.Vars(r)["id"]
	if batchID == "" {
		writeError(w, http.StatusBadRequest, "batch id is required")
		return
	}

	progress, err := a.storage.GetBatchProgress(r.Context(), batchID)
	if err != nil {
		writeStorageError(a.logger, w, err)
		return
	}

	writeJSON(w, http.StatusOK, progress)
}
