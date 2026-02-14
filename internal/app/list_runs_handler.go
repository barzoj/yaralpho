package app

import (
	"net/http"

	"github.com/gorilla/mux"
)

// listRunsHandler returns runs for a batch within a repository, capped by limit.
func (a *App) listRunsHandler(w http.ResponseWriter, r *http.Request) {
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

	limit := parseLimit(r.URL.Query().Get("limit"), defaultListLimit, maxListLimit)

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
