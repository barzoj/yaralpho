package app

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/barzoj/yaralpho/internal/storage"
	"github.com/gorilla/mux"
	"go.mongodb.org/mongo-driver/mongo"
)

type addBatchRequest struct {
	Items       []string
	SessionName string
}

// addBatchHandler creates a batch under a repository without enqueuing work.
// Items are provided via the query param `items` as a comma-separated list.
// Optional `session_name` labels the batch for display.
func (a *App) addBatchHandler(w http.ResponseWriter, r *http.Request) {
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

	itemsParam := strings.TrimSpace(r.URL.Query().Get("items"))
	if itemsParam == "" {
		writeError(w, http.StatusBadRequest, "items is required")
		return
	}

	rawItems := strings.Split(itemsParam, ",")
	items := make([]string, 0, len(rawItems))
	for _, it := range rawItems {
		if trimmed := strings.TrimSpace(it); trimmed != "" {
			items = append(items, trimmed)
		}
	}
	if len(items) == 0 {
		writeError(w, http.StatusBadRequest, "no valid items provided")
		return
	}

	sessionName := strings.TrimSpace(r.URL.Query().Get("session_name"))
	now := time.Now().UTC()
	batchID := "batch-" + strconv.FormatInt(now.UnixNano(), 10)

	batch := storage.Batch{
		ID:           batchID,
		RepositoryID: repoID,
		CreatedAt:    now,
		UpdatedAt:    now,
		Items:        make([]storage.BatchItem, 0, len(items)),
		Status:       storage.BatchStatusPending,
		SessionName:  sessionName,
	}
	for _, it := range items {
		batch.Items = append(batch.Items, storage.BatchItem{
			Input:    it,
			Status:   storage.ItemStatusPending,
			Attempts: 0,
		})
	}

	if err := a.storage.CreateBatch(r.Context(), &batch); err != nil {
		writeStorageError(a.logger, w, err)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"batch_id":      batch.ID,
		"status":        batch.Status,
		"repository_id": batch.RepositoryID,
	})
}
