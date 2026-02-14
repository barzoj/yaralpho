package app

import (
	"net/http"
	"strings"

	"github.com/barzoj/yaralpho/internal/storage"
	"github.com/gorilla/mux"
	"go.mongodb.org/mongo-driver/mongo"
)

// listRepositoryBatchesHandler returns batches scoped to a repository with optional status filtering.
func (a *App) listRepositoryBatchesHandler(w http.ResponseWriter, r *http.Request) {
	repoID := mux.Vars(r)["repoid"]
	if repoID == "" {
		writeError(w, http.StatusBadRequest, "repository id is required")
		return
	}

	if _, err := a.storage.GetRepository(r.Context(), repoID); err != nil {
		if err == mongo.ErrNoDocuments {
			writeError(w, http.StatusNotFound, "repository not found")
			return
		}
		writeStorageError(a.logger, w, err)
		return
	}

	rawStatus := strings.TrimSpace(r.URL.Query().Get("status"))
	status, ok := parseBatchStatus(rawStatus)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid status")
		return
	}

	limit := parseLimit(r.URL.Query().Get("limit"), defaultListLimit, maxListLimit)

	batches, err := a.storage.ListBatchesByRepository(r.Context(), repoID, status, int64(limit))
	if err != nil {
		writeStorageError(a.logger, w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"batches": batches,
		"count":   len(batches),
	})
}

func parseBatchStatus(raw string) (storage.BatchStatus, bool) {
	if raw == "" {
		return "", true
	}

	switch strings.ToLower(raw) {
	case "pending":
		return storage.BatchStatusPending, true
	case "in-progress", "in_progress":
		return storage.BatchStatusInProgress, true
	case "paused":
		return storage.BatchStatusPaused, true
	case "done":
		return storage.BatchStatusDone, true
	case "failed":
		return storage.BatchStatusFailed, true
	default:
		return "", false
	}
}
