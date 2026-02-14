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

func TestRepositoryCreateAndList(t *testing.T) {
	st := newHandlerTestStorage()
	app := newTestApp(t, st)

	body := []byte(`{"name":"repo-one","path":"/tmp/repo1"}`)
	rec := httptest.NewRecorder()
	app.Router().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/repository", bytes.NewReader(body)))

	require.Equal(t, http.StatusCreated, rec.Code)

	var repo storage.Repository
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &repo))
	require.Equal(t, "repo-one", repo.Name)
	require.Equal(t, "/tmp/repo1", repo.Path)
	require.NotEmpty(t, repo.ID)
	require.WithinDuration(t, time.Now(), repo.CreatedAt, time.Second*2)

	// list
	rec = httptest.NewRecorder()
	app.Router().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/repository", nil))
	require.Equal(t, http.StatusOK, rec.Code)

	var list []storage.Repository
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &list))
	require.Len(t, list, 1)
	require.Equal(t, repo.ID, list[0].ID)
}

func TestRepositoryCreateRejectsInvalidPath(t *testing.T) {
	app := newTestApp(t, newHandlerTestStorage())

	rec := httptest.NewRecorder()
	app.Router().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/repository", bytes.NewBufferString(`{"name":"bad","path":"relative/path"}`)))

	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestRepositoryCreateConflictOnDuplicate(t *testing.T) {
	st := newHandlerTestStorage()
	now := time.Now().UTC()
	st.repos["repo-1"] = storage.Repository{ID: "repo-1", Name: "existing", Path: "/tmp/existing", CreatedAt: now, UpdatedAt: now}
	app := newTestApp(t, st)

	rec := httptest.NewRecorder()
	app.Router().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/repository", bytes.NewBufferString(`{"name":"existing","path":"/tmp/new"}`)))

	require.Equal(t, http.StatusConflict, rec.Code)
}

func TestRepositoryGetAndUpdate(t *testing.T) {
	st := newHandlerTestStorage()
	now := time.Now().UTC()
	st.repos["repo-1"] = storage.Repository{ID: "repo-1", Name: "one", Path: "/tmp/one", CreatedAt: now, UpdatedAt: now}
	app := newTestApp(t, st)

	rec := httptest.NewRecorder()
	app.Router().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/repository/repo-1", nil))
	require.Equal(t, http.StatusOK, rec.Code)

	var repo storage.Repository
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &repo))
	require.Equal(t, "one", repo.Name)

	rec = httptest.NewRecorder()
	app.Router().ServeHTTP(rec, httptest.NewRequest(http.MethodPut, "/repository/repo-1", bytes.NewBufferString(`{"name":"two","path":"/tmp/two"}`)))
	require.Equal(t, http.StatusOK, rec.Code)

	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &repo))
	require.Equal(t, "two", repo.Name)
	require.Equal(t, "/tmp/two", repo.Path)
}

func TestRepositoryUpdateConflict(t *testing.T) {
	st := newHandlerTestStorage()
	now := time.Now().UTC()
	st.repos["repo-1"] = storage.Repository{ID: "repo-1", Name: "one", Path: "/tmp/one", CreatedAt: now, UpdatedAt: now}
	st.repos["repo-2"] = storage.Repository{ID: "repo-2", Name: "two", Path: "/tmp/two", CreatedAt: now, UpdatedAt: now}
	app := newTestApp(t, st)

	rec := httptest.NewRecorder()
	app.Router().ServeHTTP(rec, httptest.NewRequest(http.MethodPut, "/repository/repo-1", bytes.NewBufferString(`{"name":"two","path":"/tmp/three"}`)))

	require.Equal(t, http.StatusConflict, rec.Code)
}

func TestRepositoryDeleteBlocksActiveBatches(t *testing.T) {
	st := newHandlerTestStorage()
	now := time.Now().UTC()
	repoID := "repo-1"
	st.repos[repoID] = storage.Repository{ID: repoID, Name: "one", Path: "/tmp/one", CreatedAt: now, UpdatedAt: now}
	st.batches["b1"] = storage.Batch{ID: "b1", RepositoryID: repoID, Status: storage.BatchStatusPending}
	app := newTestApp(t, st)

	rec := httptest.NewRecorder()
	app.Router().ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/repository/repo-1", nil))

	require.Equal(t, http.StatusConflict, rec.Code)
}

func TestRepositoryDeleteAllowedWhenIdle(t *testing.T) {
	st := newHandlerTestStorage()
	now := time.Now().UTC()
	repoID := "repo-1"
	st.repos[repoID] = storage.Repository{ID: repoID, Name: "one", Path: "/tmp/one", CreatedAt: now, UpdatedAt: now}
	app := newTestApp(t, st)

	rec := httptest.NewRecorder()
	app.Router().ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/repository/repo-1", nil))

	require.Equal(t, http.StatusNoContent, rec.Code)
	_, ok := st.repos[repoID]
	require.False(t, ok)
}

func TestRepositoryGetNotFound(t *testing.T) {
	app := newTestApp(t, newHandlerTestStorage())

	rec := httptest.NewRecorder()
	app.Router().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/repository/missing", nil))

	require.Equal(t, http.StatusNotFound, rec.Code)
}
