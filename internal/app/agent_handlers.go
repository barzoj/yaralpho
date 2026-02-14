package app

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/barzoj/yaralpho/internal/storage"
	"github.com/gorilla/mux"
	"go.mongodb.org/mongo-driver/mongo"
	"strconv"
)

type agentRequest struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

func (a *App) listAgentsHandler(w http.ResponseWriter, r *http.Request) {
	agents, err := a.storage.ListAgents(r.Context())
	if err != nil {
		writeStorageError(a.logger, w, err)
		return
	}
	writeJSON(w, http.StatusOK, agents)
}

func (a *App) createAgentHandler(w http.ResponseWriter, r *http.Request) {
	var req agentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.Type = strings.ToLower(strings.TrimSpace(req.Type))

	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if !isValidAgentType(req.Type) {
		writeError(w, http.StatusBadRequest, "invalid agent type")
		return
	}

	now := time.Now().UTC()
	id := "agent-" + strconv.FormatInt(now.UnixNano(), 10)
	agent := storage.Agent{
		ID:        id,
		Name:      req.Name,
		Type:      req.Type,
		Status:    storage.AgentStatusIdle,
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := a.storage.CreateAgent(r.Context(), &agent); err != nil {
		if err == storage.ErrConflict {
			writeError(w, http.StatusConflict, "agent already exists")
			return
		}
		writeStorageError(a.logger, w, err)
		return
	}

	writeJSON(w, http.StatusCreated, agent)
}

func (a *App) getAgentHandler(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	agent, err := a.storage.GetAgent(r.Context(), id)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		writeStorageError(a.logger, w, err)
		return
	}
	writeJSON(w, http.StatusOK, agent)
}

func (a *App) updateAgentHandler(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	existing, err := a.storage.GetAgent(r.Context(), id)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		writeStorageError(a.logger, w, err)
		return
	}
	if existing.Status == storage.AgentStatusBusy {
		writeError(w, http.StatusConflict, "agent is busy")
		return
	}

	var req agentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.Type = strings.ToLower(strings.TrimSpace(req.Type))

	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if !isValidAgentType(req.Type) {
		writeError(w, http.StatusBadRequest, "invalid agent type")
		return
	}

	existing.Name = req.Name
	existing.Type = req.Type
	existing.UpdatedAt = time.Now().UTC()

	if err := a.storage.UpdateAgent(r.Context(), existing); err != nil {
		if err == storage.ErrConflict {
			writeError(w, http.StatusConflict, "agent already exists")
			return
		}
		writeStorageError(a.logger, w, err)
		return
	}

	writeJSON(w, http.StatusOK, existing)
}

func (a *App) deleteAgentHandler(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	agent, err := a.storage.GetAgent(r.Context(), id)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		writeStorageError(a.logger, w, err)
		return
	}
	if agent.Status == storage.AgentStatusBusy {
		writeError(w, http.StatusConflict, "agent is busy")
		return
	}

	if err := a.storage.DeleteAgent(r.Context(), id); err != nil {
		writeStorageError(a.logger, w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func isValidAgentType(t string) bool {
	switch t {
	case "codex", "copilot":
		return true
	default:
		return false
	}
}
