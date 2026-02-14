package app

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/barzoj/yaralpho/internal/storage"
	"github.com/stretchr/testify/require"
)

func TestAgentCreateDefaultsIdle(t *testing.T) {
	st := newHandlerTestStorage()
	app := newTestApp(t, st, &handlerTestQueue{})

	body := []byte(`{"name":"worker1","runtime":"codex"}`)
	req := httptest.NewRequest(http.MethodPost, "/agent", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	app.Router().ServeHTTP(rec, req)

	require.Equal(t, http.StatusCreated, rec.Code)

	var resp storage.Agent
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, "worker1", resp.Name)
	require.Equal(t, "codex", resp.Runtime)
	require.Equal(t, storage.AgentStatusIdle, resp.Status)
	require.NotEmpty(t, resp.ID)
	require.WithinDuration(t, time.Now(), resp.CreatedAt, time.Second*2)
}

func TestAgentCreateRejectsInvalidType(t *testing.T) {
	app := newTestApp(t, newHandlerTestStorage(), &handlerTestQueue{})

	req := httptest.NewRequest(http.MethodPost, "/agent", bytes.NewBufferString(`{"name":"w","runtime":"invalid"}`))
	rec := httptest.NewRecorder()
	app.Router().ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "invalid agent type")
}

func TestAgentListAndGet(t *testing.T) {
	st := newHandlerTestStorage()
	now := time.Now().UTC()
	st.agents["a1"] = storage.Agent{ID: "a1", Name: "one", Runtime: "codex", Status: storage.AgentStatusIdle, CreatedAt: now, UpdatedAt: now}
	st.agents["a2"] = storage.Agent{ID: "a2", Name: "two", Runtime: "copilot", Status: storage.AgentStatusBusy, CreatedAt: now, UpdatedAt: now}

	app := newTestApp(t, st, &handlerTestQueue{})

	rec := httptest.NewRecorder()
	app.Router().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/agent", nil))
	require.Equal(t, http.StatusOK, rec.Code)

	var list []storage.Agent
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &list))
	require.Len(t, list, 2)

	rec = httptest.NewRecorder()
	app.Router().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/agent/a2", nil))
	require.Equal(t, http.StatusOK, rec.Code)

	var agent storage.Agent
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &agent))
	require.Equal(t, "a2", agent.ID)
	require.Equal(t, "copilot", agent.Runtime)
}

func TestAgentUpdateBlocksWhenBusy(t *testing.T) {
	st := newHandlerTestStorage()
	now := time.Now().UTC()
	st.agents["a1"] = storage.Agent{ID: "a1", Name: "busy", Runtime: "codex", Status: storage.AgentStatusBusy, CreatedAt: now, UpdatedAt: now}
	app := newTestApp(t, st, &handlerTestQueue{})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/agent/a1", bytes.NewBufferString(`{"name":"new","runtime":"codex"}`))
	app.Router().ServeHTTP(rec, req)

	require.Equal(t, http.StatusConflict, rec.Code)
}

func TestAgentDeleteBlocksWhenBusy(t *testing.T) {
	st := newHandlerTestStorage()
	now := time.Now().UTC()
	st.agents["a1"] = storage.Agent{ID: "a1", Name: "busy", Runtime: "codex", Status: storage.AgentStatusBusy, CreatedAt: now, UpdatedAt: now}
	app := newTestApp(t, st, &handlerTestQueue{})

	rec := httptest.NewRecorder()
	app.Router().ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/agent/a1", nil))

	require.Equal(t, http.StatusConflict, rec.Code)
}

func TestAgentUpdateAllowsIdle(t *testing.T) {
	st := newHandlerTestStorage()
	now := time.Now().UTC()
	st.agents["a1"] = storage.Agent{ID: "a1", Name: "idle", Runtime: "codex", Status: storage.AgentStatusIdle, CreatedAt: now, UpdatedAt: now}
	app := newTestApp(t, st, &handlerTestQueue{})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/agent/a1", bytes.NewBufferString(`{"name":"updated","runtime":"copilot"}`))
	app.Router().ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var agent storage.Agent
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &agent))
	require.Equal(t, "updated", agent.Name)
	require.Equal(t, "copilot", agent.Runtime)
	require.Equal(t, storage.AgentStatusIdle, agent.Status)
}
