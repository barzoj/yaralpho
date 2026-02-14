package app

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/barzoj/yaralpho/internal/storage"
	"github.com/gorilla/mux"
	"go.mongodb.org/mongo-driver/mongo"
)

type addBatchRequest struct {
	Items       []string `json:"items"`
	SessionName string   `json:"session_name"`
}

// addBatchHandler creates a batch under a repository without enqueuing work.
// Items are provided via JSON body with the shape:
// { "items": ["item1", "item2"], "session_name": "label" }.
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

	var req addBatchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}

	items := make([]string, 0, len(req.Items))
	for _, it := range req.Items {
		if trimmed := strings.TrimSpace(it); trimmed != "" {
			items = append(items, trimmed)
		}
	}
	if len(items) == 0 {
		writeError(w, http.StatusBadRequest, "items is required")
		return
	}

	sessionName := strings.TrimSpace(req.SessionName)
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
