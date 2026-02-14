package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRestartHandler_SchedulerMissing(t *testing.T) {
	app := newTestApp(t, newHandlerTestStorage())

	req := httptest.NewRequest(http.MethodPost, "/restart", nil)
	rec := httptest.NewRecorder()

	app.Router().ServeHTTP(rec, req)

	require.Equal(t, http.StatusServiceUnavailable, rec.Code)
}

func TestRestartHandler_NoWaitReturnsAccepted(t *testing.T) {
	app := newTestApp(t, newHandlerTestStorage())
	sched := &fakeScheduler{active: 2}
	app.SetScheduler(sched)

	req := httptest.NewRequest(http.MethodPost, "/restart", nil)
	rec := httptest.NewRecorder()

	app.Router().ServeHTTP(rec, req)

	require.Equal(t, http.StatusAccepted, rec.Code)
	require.True(t, sched.draining)
	require.Contains(t, rec.Body.String(), `"status":"draining"`)
	require.Contains(t, rec.Body.String(), `"active_runs":2`)
	require.False(t, sched.waitCalled)
}

func TestRestartHandler_WaitBlocksUntilIdle(t *testing.T) {
	app := newTestApp(t, newHandlerTestStorage())
	sched := &fakeScheduler{active: 1, waitCh: make(chan struct{})}
	app.SetScheduler(sched)

	req := httptest.NewRequest(http.MethodPost, "/restart?wait=true", nil)
	rec := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		app.Router().ServeHTTP(rec, req)
		close(done)
	}()

	select {
	case <-done:
		t.Fatalf("handler returned before scheduler drained")
	case <-time.After(20 * time.Millisecond):
	}

	sched.active = 0
	close(sched.waitCh)

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatalf("handler did not return after drain")
	}

	require.True(t, sched.waitCalled)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"status":"drained"`)
	require.Contains(t, rec.Body.String(), `"active_runs":0`)
}

type fakeScheduler struct {
	draining   bool
	waitCalled bool
	waitCh     chan struct{}
	waitErr    error
	active     int
}

func (f *fakeScheduler) SetDraining(draining bool) { f.draining = draining }
func (f *fakeScheduler) Draining() bool            { return f.draining }
func (f *fakeScheduler) ActiveCount() int          { return f.active }
func (f *fakeScheduler) WaitForIdle(_ context.Context) error {
	f.waitCalled = true
	if f.waitCh != nil {
		<-f.waitCh
	}
	return f.waitErr
}
