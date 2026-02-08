package app

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"go.mongodb.org/mongo-driver/mongo"
	"go.uber.org/zap"
)

const (
	defaultListLimit   = 50
	maxListLimit       = 200
	defaultEventsLimit = 50
	maxEventsLimit     = 200
)

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func writeStorageError(logger *zap.Logger, w http.ResponseWriter, err error) {
	if errors.Is(err, mongo.ErrNoDocuments) {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if logger != nil {
		logger.Error("storage error", zap.Error(err))
	}
	writeError(w, http.StatusInternalServerError, "storage error")
}

func parseLimit(raw string, def, max int) int {
	if raw == "" {
		return def
	}
	val, err := strconv.Atoi(raw)
	if err != nil || val <= 0 {
		return def
	}
	if val > max {
		return max
	}
	return val
}
