package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/barzoj/yaralpho/internal/storage"
	"github.com/stretchr/testify/require"
)

func TestIntegration_CreateBatchStartsPending(t *testing.T) {
	h := newHarness(t, harnessOptions{Interval: 5 * time.Millisecond, MaxRetries: 2})

	repoID := createRepository(t, h, "repo-one", "/tmp/repo-one")
	batchID := addBatch(t, h, repoID, []string{"task-1", "  task-2  "})

	batch := getBatch(t, h, batchID)
	require.Equal(t, storage.BatchStatusPending, batch.Status)
	require.Len(t, batch.Items, 2)
	require.Equal(t, storage.ItemStatusPending, batch.Items[0].Status)
	require.Equal(t, storage.ItemStatusPending, batch.Items[1].Status)
	require.Equal(t, "task-1", batch.Items[0].Input)
	require.Equal(t, "task-2", batch.Items[1].Input)
	require.Zero(t, batch.Items[0].Attempts)
	require.Zero(t, batch.Items[1].Attempts)
}

func TestIntegration_ProcessBatchSucceeds(t *testing.T) {
	h := newHarness(t, harnessOptions{Interval: 5 * time.Millisecond, MaxRetries: 2})

	repoID := createRepository(t, h, "repo-two", "/tmp/repo-two")
	addAgent(t, h, "agent-one")
	batchID := addBatch(t, h, repoID, []string{"task-happy"})

	require.NoError(t, h.tickN(1))

	batch := getBatch(t, h, batchID)
	require.Equal(t, storage.BatchStatusDone, batch.Status)
	require.Equal(t, storage.ItemStatusDone, batch.Items[0].Status)

	agent := getAgent(t, h)
	require.Equal(t, storage.AgentStatusIdle, agent.Status)
}

func TestIntegration_RetryExhaustionFailsBatch(t *testing.T) {
	h := newHarness(t, harnessOptions{Interval: 5 * time.Millisecond, MaxRetries: 2, WorkerFail: true})

	repoID := createRepository(t, h, "repo-three", "/tmp/repo-three")
	addAgent(t, h, "agent-two")
	batchID := addBatch(t, h, repoID, []string{"task-fail"})

	for i := 0; i < 3; i++ {
		_ = h.tick()
	}

	batch := getBatch(t, h, batchID)
	require.Equal(t, storage.BatchStatusFailed, batch.Status)
	require.Equal(t, storage.ItemStatusFailed, batch.Items[0].Status)
	require.Equal(t, 2, batch.Items[0].Attempts)

	agent := getAgent(t, h)
	require.Equal(t, storage.AgentStatusIdle, agent.Status)
}

func TestIntegration_PauseThenResumeBatch(t *testing.T) {
	h := newHarness(t, harnessOptions{Interval: 5 * time.Millisecond, MaxRetries: 2})

	repoID := createRepository(t, h, "repo-four", "/tmp/repo-four")
	addAgent(t, h, "agent-three")
	batchID := addBatch(t, h, repoID, []string{"task-pause"})

	pauseBatch(t, h, repoID, batchID)

	require.NoError(t, h.tickN(2)) // paused; no progress expected

	batch := getBatch(t, h, batchID)
	require.Equal(t, storage.BatchStatusPaused, batch.Status)
	require.Equal(t, storage.ItemStatusPending, batch.Items[0].Status)
	require.Zero(t, batch.Items[0].Attempts)

	resumeBatch(t, h, repoID, batchID)
	require.NoError(t, h.tickN(1))

	batch = getBatch(t, h, batchID)
	require.Equal(t, storage.BatchStatusDone, batch.Status)
	require.Equal(t, storage.ItemStatusDone, batch.Items[0].Status)
}

// Helpers

func createRepository(t *testing.T, h *harness, name, path string) string {
	t.Helper()
	body := map[string]string{"name": name, "path": path}
	status, data := postJSON(t, h.server.URL+"/repository", body)
	require.Equal(t, http.StatusCreated, status)
	var resp storage.Repository
	require.NoError(t, json.Unmarshal(data, &resp))
	return resp.ID
}

func addAgent(t *testing.T, h *harness, name string) string {
	t.Helper()
	body := map[string]string{"name": name, "runtime": "codex"}
	status, data := postJSON(t, h.server.URL+"/agent", body)
	require.Equal(t, http.StatusCreated, status)
	var resp storage.Agent
	require.NoError(t, json.Unmarshal(data, &resp))
	return resp.ID
}

func addBatch(t *testing.T, h *harness, repoID string, items []string) string {
	t.Helper()
	body := map[string]any{"items": items}
	status, data := postJSON(t, fmt.Sprintf("%s/repository/%s/add", h.server.URL, repoID), body)
	require.Equal(t, http.StatusCreated, status)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(data, &resp))
	id, ok := resp["batch_id"].(string)
	require.True(t, ok)
	return id
}

func getBatch(t *testing.T, h *harness, batchID string) storage.Batch {
	t.Helper()
	resp, err := http.Get(fmt.Sprintf("%s/batches/%s", h.server.URL, batchID))
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	data, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	var payload struct {
		Batch storage.Batch `json:"batch"`
		Runs  any           `json:"runs"`
	}
	require.NoError(t, json.Unmarshal(data, &payload))
	return payload.Batch
}

func getAgent(t *testing.T, h *harness) storage.Agent {
	t.Helper()
	resp, err := http.Get(h.server.URL + "/agent")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	data, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	var agents []storage.Agent
	require.NoError(t, json.Unmarshal(data, &agents))
	require.NotEmpty(t, agents)
	return agents[0]
}

func pauseBatch(t *testing.T, h *harness, repoID, batchID string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPut, fmt.Sprintf("%s/repository/%s/batch/%s/pause", h.server.URL, repoID, batchID), nil)
	require.NoError(t, err)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
}

func resumeBatch(t *testing.T, h *harness, repoID, batchID string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPut, fmt.Sprintf("%s/repository/%s/batch/%s/resume", h.server.URL, repoID, batchID), nil)
	require.NoError(t, err)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
}

func postJSON(t *testing.T, url string, body any) (int, []byte) {
	t.Helper()
	data, err := json.Marshal(body)
	require.NoError(t, err)
	resp, err := http.Post(url, "application/json", bytes.NewReader(data))
	require.NoError(t, err)
	defer resp.Body.Close()
	respData, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return resp.StatusCode, respData
}
