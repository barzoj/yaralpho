package app

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/barzoj/yaralpho/internal/storage"
	"github.com/gorilla/mux"
	"go.mongodb.org/mongo-driver/mongo"
)

type repositoryRequest struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

func (a *App) listRepositoriesHandler(w http.ResponseWriter, r *http.Request) {
	repos, err := a.storage.ListRepositories(r.Context())
	if err != nil {
		writeStorageError(a.logger, w, err)
		return
	}
	writeJSON(w, http.StatusOK, repos)
}

func (a *App) createRepositoryHandler(w http.ResponseWriter, r *http.Request) {
	var req repositoryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.Path = strings.TrimSpace(req.Path)

	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if !isValidRepoPath(req.Path) {
		writeError(w, http.StatusBadRequest, "invalid path")
		return
	}

	now := time.Now().UTC()
	repo := storage.Repository{
		ID:        "repo-" + strconv.FormatInt(now.UnixNano(), 10),
		Name:      req.Name,
		Path:      req.Path,
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := a.storage.CreateRepository(r.Context(), &repo); err != nil {
		if err == storage.ErrConflict {
			writeError(w, http.StatusConflict, "repository already exists")
			return
		}
		writeStorageError(a.logger, w, err)
		return
	}

	writeJSON(w, http.StatusCreated, repo)
}

func (a *App) getRepositoryHandler(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	repo, err := a.storage.GetRepository(r.Context(), id)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		writeStorageError(a.logger, w, err)
		return
	}
	writeJSON(w, http.StatusOK, repo)
}

func (a *App) updateRepositoryHandler(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	existing, err := a.storage.GetRepository(r.Context(), id)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		writeStorageError(a.logger, w, err)
		return
	}

	var req repositoryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.Path = strings.TrimSpace(req.Path)

	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if !isValidRepoPath(req.Path) {
		writeError(w, http.StatusBadRequest, "invalid path")
		return
	}

	existing.Name = req.Name
	existing.Path = req.Path
	existing.UpdatedAt = time.Now().UTC()

	if err := a.storage.UpdateRepository(r.Context(), existing); err != nil {
		if err == storage.ErrConflict {
			writeError(w, http.StatusConflict, "repository already exists")
			return
		}
		writeStorageError(a.logger, w, err)
		return
	}

	writeJSON(w, http.StatusOK, existing)
}

func (a *App) deleteRepositoryHandler(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	if _, err := a.storage.GetRepository(r.Context(), id); err != nil {
		if err == mongo.ErrNoDocuments {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		writeStorageError(a.logger, w, err)
		return
	}

	hasActive, err := a.storage.RepositoryHasActiveBatches(r.Context(), id)
	if err != nil {
		writeStorageError(a.logger, w, err)
		return
	}
	if hasActive {
		writeError(w, http.StatusConflict, "repository has active batches")
		return
	}

	if err := a.storage.DeleteRepository(r.Context(), id); err != nil {
		writeStorageError(a.logger, w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func isValidRepoPath(p string) bool {
	return p != "" && filepath.IsAbs(p)
}
