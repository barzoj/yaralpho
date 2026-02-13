package app

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/barzoj/yaralpho/internal/bus"
	"github.com/barzoj/yaralpho/internal/config"
	"github.com/barzoj/yaralpho/internal/consumer"
	"github.com/barzoj/yaralpho/internal/copilot"
	"github.com/barzoj/yaralpho/internal/notify"
	"github.com/barzoj/yaralpho/internal/queue"
	"github.com/barzoj/yaralpho/internal/storage"
	mongostorage "github.com/barzoj/yaralpho/internal/storage/mongo"
	"github.com/barzoj/yaralpho/internal/tracker"
	"github.com/gorilla/mux"
	"go.uber.org/zap"
)

// Consumer abstracts the worker loop so tests can provide a stub and the
// application can remain interface-driven.
type Consumer interface {
	Run(ctx context.Context) error
}

// BuildOptions controls provider selection at composition time.
type BuildOptions struct {
	Agent string
}

var (
	newStorage func(ctx context.Context, uri, dbName string, logger *zap.Logger) (storage.Storage, error) = func(ctx context.Context, uri, dbName string, logger *zap.Logger) (storage.Storage, error) {
		return mongostorage.New(ctx, uri, dbName, logger)
	}
	newTracker func(cfg config.Config, logger *zap.Logger) (tracker.Tracker, error) = func(cfg config.Config, logger *zap.Logger) (tracker.Tracker, error) {
		return tracker.NewBeads(cfg, logger)
	}
	newNotifier func(cfg config.Config, logger *zap.Logger) (notify.Notifier, error) = func(cfg config.Config, logger *zap.Logger) (notify.Notifier, error) {
		return notify.NewSlack(cfg, logger)
	}
	newGitHubClient = func(logger *zap.Logger) copilot.Client {
		return copilot.NewGitHub(logger)
	}
	newCodexClient = func(logger *zap.Logger) copilot.Client {
		return copilot.NewCodex(logger)
	}
	newWorker = func(q queue.Queue, tr tracker.Tracker, cp copilot.Client, st storage.Storage, nt notify.Notifier, cfg config.Config, repoPath string, logger *zap.Logger) Consumer {
		return consumer.NewWorker(q, tr, cp, st, nt, cfg, repoPath, logger)
	}
)

// App wires the Ralph Runner dependencies, HTTP router, and background
// consumer. Run starts the HTTP server and consumer and blocks until context
// cancellation or a fatal error.
type App struct {
	cfg        config.Config
	logger     *zap.Logger
	storage    storage.Storage
	queue      queue.Queue
	tracker    tracker.Tracker
	notifier   notify.Notifier
	copilot    copilot.Client
	consumer   Consumer
	router     *mux.Router
	server     *http.Server
	closers    []func(context.Context) error
	started    uint32
	reqCounter uint64
	startOnce  sync.Once
	wg         sync.WaitGroup
	eventBus   bus.Bus
}

// Build constructs the production App using concrete implementations. All
// dependencies are derived from the provided config and logger.
func Build(ctx context.Context, logger *zap.Logger, cfg config.Config) (*App, error) {
	return BuildWithOptions(ctx, logger, cfg, BuildOptions{})
}

// BuildWithOptions constructs the production App using concrete
// implementations and provider-selection options.
func BuildWithOptions(ctx context.Context, logger *zap.Logger, cfg config.Config, opts BuildOptions) (*App, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if logger == nil {
		logger = zap.NewNop()
	}
	if cfg == nil {
		return nil, fmt.Errorf("config is required")
	}

	mongoURI, err := cfg.Get(config.MongoURIKey)
	if err != nil {
		return nil, fmt.Errorf("get mongo uri: %w", err)
	}
	mongoDB, err := cfg.Get(config.MongoDBKey)
	if err != nil {
		return nil, fmt.Errorf("get mongo db: %w", err)
	}
	repoPath, err := cfg.Get(config.RepoPathKey)
	if err != nil {
		return nil, fmt.Errorf("get repo path: %w", err)
	}

	st, err := newStorage(ctx, mongoURI, mongoDB, logger)
	if err != nil {
		return nil, fmt.Errorf("init mongo storage: %w", err)
	}

	q := queue.NewMemoryQueue(logger)

	tr, err := newTracker(cfg, logger)
	if err != nil {
		return nil, fmt.Errorf("init tracker: %w", err)
	}

	nt, err := newNotifier(cfg, logger)
	if err != nil {
		return nil, fmt.Errorf("init notifier: %w", err)
	}

	cp, err := copilotClientForAgent(logger, opts.Agent)
	if err != nil {
		return nil, err
	}

	cons := newWorker(q, tr, cp, st, nt, cfg, repoPath, logger)

	return New(logger, cfg, st, q, tr, nt, cp, cons)
}

func copilotClientForAgent(logger *zap.Logger, agent string) (copilot.Client, error) {
	switch strings.ToLower(strings.TrimSpace(agent)) {
	case "", "codex":
		return newCodexClient(logger), nil
	case "github":
		return newGitHubClient(logger), nil
	default:
		return nil, fmt.Errorf("unknown agent %q (allowed: codex, github)", agent)
	}
}

// New assembles an App from already-constructed interfaces. It is primarily
// used by tests to supply fakes without touching external systems.
func New(logger *zap.Logger, cfg config.Config, st storage.Storage, q queue.Queue, tr tracker.Tracker, nt notify.Notifier, cp copilot.Client, cons Consumer) (*App, error) {
	if logger == nil {
		logger = zap.NewNop()
	}

	switch {
	case cfg == nil:
		return nil, fmt.Errorf("config is required")
	case st == nil:
		return nil, fmt.Errorf("storage is required")
	case q == nil:
		return nil, fmt.Errorf("queue is required")
	case tr == nil:
		return nil, fmt.Errorf("tracker is required")
	case nt == nil:
		return nil, fmt.Errorf("notifier is required")
	case cp == nil:
		return nil, fmt.Errorf("copilot client is required")
	case cons == nil:
		return nil, fmt.Errorf("consumer is required")
	}

	router := mux.NewRouter()
	app := &App{
		cfg:      cfg,
		logger:   logger,
		storage:  st,
		queue:    q,
		tracker:  tr,
		notifier: nt,
		copilot:  cp,
		consumer: cons,
		router:   router,
	}

	app.eventBus = consumer.SessionEventBus()
	if app.eventBus == nil {
		app.eventBus = bus.NewMemoryBus(bus.Config{Logger: logger})
		consumer.SetSessionEventBus(app.eventBus)
	}

	app.registerRoutes()

	port, err := cfg.Get(config.PortKey)
	if err != nil || port == "" {
		port = "8080"
	}

	app.server = &http.Server{
		Addr:              fmt.Sprintf(":%s", port),
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
	}

	if closer, ok := st.(interface{ Close(context.Context) error }); ok {
		app.closers = append(app.closers, func(ctx context.Context) error {
			return closer.Close(ctx)
		})
	}

	return app, nil
}

// Run starts the HTTP server and consumer once. It blocks until the provided
// context is cancelled or a fatal error occurs, then shuts down gracefully.
func (a *App) Run(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}

	if !atomic.CompareAndSwapUint32(&a.started, 0, 1) {
		return fmt.Errorf("app already running")
	}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	errCh := make(chan error, 2)

	a.startOnce.Do(func() {
		a.wg.Add(1)
		go func() {
			defer a.wg.Done()
			if err := a.consumer.Run(runCtx); err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, queue.ErrClosed) {
				errCh <- err
			}
		}()
	})

	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		if err := a.server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	a.logConfigValues()

	select {
	case <-runCtx.Done():
	case err := <-errCh:
		if err != nil {
			cancel()
			a.logger.Error("fatal runtime error", zap.Error(err))
		}
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	a.queue.Close()
	_ = a.server.Shutdown(shutdownCtx)
	for _, closer := range a.closers {
		if err := closer(shutdownCtx); err != nil {
			a.logger.Warn("resource close failed", zap.Error(err))
		}
	}

	a.wg.Wait()

	select {
	case err := <-errCh:
		if err != nil {
			return err
		}
	default:
	}

	return nil
}

func (a *App) logConfigValues() {
	fields := make([]zap.Field, 0, len(config.LoggableKeys()))
	for _, key := range config.LoggableKeys() {
		value, err := a.cfg.Get(key)
		if err != nil {
			fields = append(fields, zap.String(key, "<missing>"))
			continue
		}
		fields = append(fields, zap.String(key, value))
	}

	a.logger.Info("config values", fields...)
}

// Router exposes the underlying mux router for tests.
func (a *App) Router() *mux.Router {
	return a.router
}

func (a *App) healthHandler(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
