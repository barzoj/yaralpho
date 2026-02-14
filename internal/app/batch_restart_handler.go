package app

import (
	"net/http"
	"time"

	"github.com/barzoj/yaralpho/internal/storage"
	"github.com/gorilla/mux"
	"go.uber.org/zap"
)

// restartBatchHandler resets the failed item in a batch so it can be retried.
// It is only valid when the batch is in a failed state and belongs to the
// referenced repository.
func (a *App) restartBatchHandler(w http.ResponseWriter, r *http.Request) {
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
		a.logger.Warn("batch restart repository mismatch",
			zap.String("batch_id", batchID),
			zap.String("repository_id", batch.RepositoryID),
			zap.String("requested_repository_id", repoID),
		)
		writeError(w, http.StatusNotFound, "batch not found for repository")
		return
	}
	if batch.Status != storage.BatchStatusFailed {
		writeError(w, http.StatusConflict, "batch is not in failed state")
		return
	}

	failedIdx := -1
	for i := range batch.Items {
		if batch.Items[i].Status == storage.ItemStatusFailed {
			failedIdx = i
			break
		}
	}
	if failedIdx == -1 {
		writeError(w, http.StatusBadRequest, "batch has no failed item to restart")
		return
	}

	batch.Items[failedIdx].Status = storage.ItemStatusPending
	batch.Items[failedIdx].Attempts = 0
	batch.Status = storage.BatchStatusPending
	batch.UpdatedAt = time.Now().UTC()

	if err := a.storage.UpdateBatch(r.Context(), batch); err != nil {
		writeStorageError(a.logger, w, err)
		return
	}

	a.logger.Info("batch restart scheduled",
		zap.String("batch_id", batch.ID),
		zap.String("repository_id", batch.RepositoryID),
		zap.Int("failed_index", failedIdx),
	)

	writeJSON(w, http.StatusOK, map[string]any{
		"batch_id":      batch.ID,
		"status":        batch.Status,
		"repository_id": batch.RepositoryID,
	})
}
