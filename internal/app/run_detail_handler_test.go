package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/barzoj/yaralpho/internal/storage"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/require"
)

func TestRunDetailHandlerIncludesRepositoryContext(t *testing.T) {
	st := newHandlerTestStorage()
	q := &handlerTestQueue{}
	app := newTestApp(t, st, q)

	runID := "run-1"
	repoID := "repo-123"
	batchID := "batch-9"
	sessionID := "session-abc"

	st.runs[runID] = storage.TaskRun{
		ID:           runID,
		BatchID:      batchID,
		RepositoryID: repoID,
		SessionID:    sessionID,
		StartedAt:    time.Now().Add(-1 * time.Minute).UTC(),
		Status:       storage.TaskRunStatusRunning,
	}
	st.events[sessionID] = []storage.SessionEvent{
		{BatchID: batchID, RunID: runID, SessionID: sessionID, Event: map[string]any{"type": "log", "data": "a"}, IngestedAt: time.Now().Add(-30 * time.Second).UTC()},
		{BatchID: batchID, RunID: runID, SessionID: sessionID, Event: map[string]any{"type": "log", "data": "b"}, IngestedAt: time.Now().UTC()},
	}

	req := httptest.NewRequest(http.MethodGet, "/runs/"+runID+"?event_limit=5", nil)
	req = mux.SetURLVars(req, map[string]string{"id": runID})
	w := httptest.NewRecorder()

	app.runDetailHandler(w, req)

	resp := w.Result()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var body map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	require.Equal(t, repoID, body["repository_id"])
	require.Equal(t, batchID, body["batch_id"])
	require.Equal(t, sessionID, body["session_id"])

	events, ok := body["events"].([]any)
	require.True(t, ok)
	require.Len(t, events, 2)

	// Ensure the run payload contains the repository_id and omits epic fields.
	runPayload, ok := body["run"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, repoID, runPayload["repository_id"])
	require.Nil(t, runPayload["epic_ref"])
}
