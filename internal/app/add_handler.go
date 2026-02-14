package app

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/barzoj/yaralpho/internal/consumer"
	"github.com/barzoj/yaralpho/internal/queue"
	"github.com/barzoj/yaralpho/internal/storage"
	"go.uber.org/zap"
)

// addHandler enqueues a new batch of tasks described by ?items= (comma
// separated). An optional session_name labels the batch. Responds with the
// created batch_id.
func (a *App) addHandler(w http.ResponseWriter, r *http.Request) {
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
	if sessionName == "" {
		sessionName = "default"
	}

	now := time.Now().UTC()
	batchID := "batch-" + strconv.FormatInt(now.UnixNano(), 10)

	batch := storage.Batch{
		ID:          batchID,
		CreatedAt:   now,
		Items:       make([]storage.BatchItem, 0, len(items)),
		Status:      storage.BatchStatusCreated,
		SessionName: sessionName,
	}
	for _, it := range items {
		batch.Items = append(batch.Items, storage.BatchItem{Input: it, Status: string(storage.BatchStatusCreated), Attempts: 0})
	}

	if err := a.storage.CreateBatch(r.Context(), &batch); err != nil {
		writeStorageError(a.logger, w, err)
		return
	}

	for _, item := range items {
		payload, err := consumer.EncodeQueueItem(consumer.QueueItem{BatchID: batchID, TaskRef: item})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to enqueue item")
			a.logger.Error("encode queue item", zap.Error(err))
			return
		}
		if err := a.queue.Enqueue(payload); err != nil {
			if errors.Is(err, queue.ErrClosed) {
				writeError(w, http.StatusServiceUnavailable, "queue closed")
			} else {
				writeError(w, http.StatusInternalServerError, "failed to enqueue item")
			}
			a.logger.Error("enqueue", zap.Error(err))
			return
		}
	}

	writeJSON(w, http.StatusAccepted, map[string]string{"batch_id": batchID})
}
