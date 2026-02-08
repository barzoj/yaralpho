package app

import (
	"net/http"

	"github.com/gorilla/mux"
)

// batchDetailHandler returns a batch and its runs.
func (a *App) batchDetailHandler(w http.ResponseWriter, r *http.Request) {
	batchID := mux.Vars(r)["id"]
	if batchID == "" {
		writeError(w, http.StatusBadRequest, "batch id is required")
		return
	}

	batch, err := a.storage.GetBatch(r.Context(), batchID)
	if err != nil {
		writeStorageError(a.logger, w, err)
		return
	}

	runs, err := a.storage.ListTaskRuns(r.Context(), batchID)
	if err != nil {
		writeStorageError(a.logger, w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"batch": batch,
		"runs":  runs,
	})
}
