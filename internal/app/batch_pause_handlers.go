package app

import (
	"net/http"
	"time"

	"github.com/barzoj/yaralpho/internal/storage"
	"github.com/gorilla/mux"
)

// pauseBatchHandler marks a batch as paused to stop new work from starting.
func (a *App) pauseBatchHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	repoID := vars["repoid"]
	batchID := vars["batchid"]
	if repoID == "" || batchID == "" {
		writeError(w, http.StatusBadRequest, "repository id and batch id are required")
		return
	}

	if _, err := a.storage.GetRepository(r.Context(), repoID); err != nil {
		writeStorageError(a.logger, w, err)
		return
	}

	batch, err := a.storage.GetBatch(r.Context(), batchID)
	if err != nil {
		writeStorageError(a.logger, w, err)
		return
	}
	if batch.RepositoryID != repoID {
		writeError(w, http.StatusNotFound, "batch not found for repository")
		return
	}
	if batch.Status == storage.BatchStatusDone || batch.Status == storage.BatchStatusFailed {
		writeError(w, http.StatusConflict, "batch cannot be paused in current state")
		return
	}
	if batch.Status == storage.BatchStatusPaused {
		writeError(w, http.StatusConflict, "batch already paused")
		return
	}

	batch.Status = storage.BatchStatusPaused
	batch.UpdatedAt = time.Now().UTC()

	if err := a.storage.UpdateBatch(r.Context(), batch); err != nil {
		writeStorageError(a.logger, w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"batch_id":      batch.ID,
		"status":        batch.Status,
		"repository_id": batch.RepositoryID,
	})
}

// resumeBatchHandler returns a paused batch to pending so scheduler can pick it up.
func (a *App) resumeBatchHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	repoID := vars["repoid"]
	batchID := vars["batchid"]
	if repoID == "" || batchID == "" {
		writeError(w, http.StatusBadRequest, "repository id and batch id are required")
		return
	}

	if _, err := a.storage.GetRepository(r.Context(), repoID); err != nil {
		writeStorageError(a.logger, w, err)
		return
	}

	batch, err := a.storage.GetBatch(r.Context(), batchID)
	if err != nil {
		writeStorageError(a.logger, w, err)
		return
	}
	if batch.RepositoryID != repoID {
		writeError(w, http.StatusNotFound, "batch not found for repository")
		return
	}
	if batch.Status != storage.BatchStatusPaused {
		writeError(w, http.StatusConflict, "batch is not paused")
		return
	}

	batch.Status = storage.BatchStatusPending
	batch.UpdatedAt = time.Now().UTC()

	if err := a.storage.UpdateBatch(r.Context(), batch); err != nil {
		writeStorageError(a.logger, w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"batch_id":      batch.ID,
		"status":        batch.Status,
		"repository_id": batch.RepositoryID,
	})
}
