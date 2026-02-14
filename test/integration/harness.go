package integration

import (
	"context"
	"fmt"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/barzoj/yaralpho/internal/app"
	"github.com/barzoj/yaralpho/internal/config"
	"github.com/barzoj/yaralpho/internal/scheduler"
	"go.uber.org/zap"
)

type mapConfig struct {
	values map[string]string
}

func (m mapConfig) Get(key string) (string, error) {
	if v, ok := m.values[key]; ok {
		return v, nil
	}
	return "", fmt.Errorf("config key %s not set", key)
}

type harness struct {
	t        *testing.T
	app      *app.App
	storage  *fakeStorage
	tracker  *fakeTracker
	notifier *fakeNotifier
	copilot  *fakeCopilot
	server   *httptest.Server
	sched    *scheduler.Scheduler
}

type harnessOptions struct {
	Interval   time.Duration
	MaxRetries int
	WorkerFail bool
}

// newHarness builds an App wired with fakes and returns helpers for tests.
func newHarness(t *testing.T, opts harnessOptions) *harness {
	t.Helper()

	logger := zap.NewNop()
	st := newFakeStorage()
	tr := newFakeTracker()
	nt := &fakeNotifier{}
	cp := &fakeCopilot{}
	fw := newFakeWorker(st, opts.WorkerFail)

	cfg := mapConfig{values: map[string]string{
		config.MongoURIKey:          "mongodb://fake",
		config.MongoDBKey:           "yaralpho-test",
		config.RepoPathKey:          "/tmp/yaralpho-repo",
		config.PortKey:              "0",
		config.SchedulerIntervalKey: opts.Interval.String(),
		config.MaxRetriesKey:        intToString(opts.MaxRetries),
	}}

	hApp, err := app.New(logger, cfg, st, tr, nt, cp)
	if err != nil {
		t.Fatalf("build app: %v", err)
	}

	var worker scheduler.Worker = fw
	sched := scheduler.New(st, worker, logger, scheduler.Options{Interval: opts.Interval, MaxRetries: opts.MaxRetries})
	hApp.SetScheduler(sched)

	server := httptest.NewServer(hApp.Router())
	t.Cleanup(server.Close)

	return &harness{t: t, app: hApp, storage: st, tracker: tr, notifier: nt, copilot: cp, server: server, sched: sched}
}

// tick runs scheduler.Tick and returns any error for the caller to assert.
func (h *harness) tick() error {
	return h.sched.Tick(context.Background())
}

// tickN runs Tick n times.
func (h *harness) tickN(n int) error {
	for i := 0; i < n; i++ {
		if err := h.tick(); err != nil {
			return err
		}
	}
	return nil
}

func intToString(v int) string {
	return fmt.Sprintf("%d", v)
}
