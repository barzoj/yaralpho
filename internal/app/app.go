package app

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/barzoj/yaralpho/internal/bus"
	"github.com/barzoj/yaralpho/internal/config"
	"github.com/barzoj/yaralpho/internal/consumer"
	"github.com/barzoj/yaralpho/internal/copilot"
	"github.com/barzoj/yaralpho/internal/notify"
	"github.com/barzoj/yaralpho/internal/scheduler"
	"github.com/barzoj/yaralpho/internal/storage"
	mongostorage "github.com/barzoj/yaralpho/internal/storage/mongo"
	"github.com/barzoj/yaralpho/internal/tracker"
	"github.com/gorilla/mux"
	"go.uber.org/zap"
)

// BuildOptions is kept for compatibility; no options are currently supported.
type BuildOptions struct{}

type schedulerController interface {
	SetDraining(bool)
	Draining() bool
	ActiveCount() int
	WaitForIdle(ctx context.Context) error
	Tick(ctx context.Context) error
	Stop(ctx context.Context) error
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
	newScheduler func(st scheduler.Storage, worker scheduler.Worker, logger *zap.Logger, opts scheduler.Options) schedulerController = func(st scheduler.Storage, worker scheduler.Worker, logger *zap.Logger, opts scheduler.Options) schedulerController {
		return scheduler.New(st, worker, logger, opts)
	}
)

// App wires the Ralph Runner dependencies, HTTP router, and background
// consumer. Run starts the HTTP server and consumer and blocks until context
// cancellation or a fatal error.
type App struct {
	cfg          config.Config
	logger       *zap.Logger
	storage      storage.Storage
	tracker      tracker.Tracker
	notifier     notify.Notifier
	copilot      copilot.Client
	scheduler    schedulerController
	router       *mux.Router
	server       *http.Server
	closers      []func(context.Context) error
	started      uint32
	reqCounter   uint64
	wg           sync.WaitGroup
	eventBus     bus.Bus
	runCancel    context.CancelFunc
	shutdownOnce sync.Once
}

// Build constructs the production App using concrete implementations. All
// dependencies are derived from the provided config and logger.
func Build(ctx context.Context, logger *zap.Logger, cfg config.Config) (*App, error) {
	return BuildWithOptions(ctx, logger, cfg, BuildOptions{})
}

// BuildWithOptions constructs the production App using concrete
// implementations. Provider selection is now per-agent runtime.
func BuildWithOptions(ctx context.Context, logger *zap.Logger, cfg config.Config, _ BuildOptions) (*App, error) {
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
	st, err := newStorage(ctx, mongoURI, mongoDB, logger)
	if err != nil {
		return nil, fmt.Errorf("init mongo storage: %w", err)
	}

	tr, err := newTracker(cfg, logger)
	if err != nil {
		return nil, fmt.Errorf("init tracker: %w", err)
	}

	nt, err := newNotifier(cfg, logger)
	if err != nil {
		return nil, fmt.Errorf("init notifier: %w", err)
	}

	cp := copilot.NewCodex(logger)
	application, err := New(logger, cfg, st, tr, nt, cp)
	if err != nil {
		return nil, err
	}

	worker := consumer.NewWorker(tr, cp, st, nt, cfg, logger)
	schedOpts := schedulerOptionsFromConfig(cfg, logger)
	application.SetScheduler(newScheduler(st, worker, logger, schedOpts))

	return application, nil
}

// New assembles an App from already-constructed interfaces. It is primarily
// used by tests to supply fakes without touching external systems.
func New(logger *zap.Logger, cfg config.Config, st storage.Storage, tr tracker.Tracker, nt notify.Notifier, cp copilot.Client) (*App, error) {
	if logger == nil {
		logger = zap.NewNop()
	}

	switch {
	case cfg == nil:
		return nil, fmt.Errorf("config is required")
	case st == nil:
		return nil, fmt.Errorf("storage is required")
	case tr == nil:
		return nil, fmt.Errorf("tracker is required")
	case nt == nil:
		return nil, fmt.Errorf("notifier is required")
	case cp == nil:
		return nil, fmt.Errorf("copilot client is required")
	}

	router := mux.NewRouter()
	app := &App{
		cfg:      cfg,
		logger:   logger,
		storage:  st,
		tracker:  tr,
		notifier: nt,
		copilot:  cp,
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

// SetScheduler wires the scheduler controller used by restartHandler. Tests can
// supply a fake while production wiring may inject a real scheduler.
func (a *App) SetScheduler(s schedulerController) {
	a.scheduler = s
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
	a.shutdownOnce = sync.Once{}
	a.runCancel = cancel
	defer cancel()

	errCh := make(chan error, 2)

	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		if err := a.server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	a.logConfigValues()

	if a.scheduler != nil {
		a.wg.Add(1)
		go func() {
			defer a.wg.Done()
			a.runScheduler(runCtx)
		}()
	}

	select {
	case <-runCtx.Done():
	case err := <-errCh:
		if err != nil {
			cancel()
			a.logger.Error("fatal runtime error", zap.Error(err))
		}
	}

	if a.scheduler != nil {
		_ = a.scheduler.Stop(context.Background())
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

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

func (a *App) requestShutdown() {
	if a == nil {
		return
	}
	a.shutdownOnce.Do(func() {
		if a.scheduler != nil {
			_ = a.scheduler.Stop(context.Background())
		}
		if a.runCancel != nil {
			a.runCancel()
		}
	})
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

func (a *App) runScheduler(ctx context.Context) {
	if a == nil || a.scheduler == nil {
		return
	}

	opts := schedulerOptionsFromConfig(a.cfg, a.logger)
	interval := opts.Interval
	if interval <= 0 {
		interval = defaultSchedulerIntervalDuration
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	a.logger.Info("scheduler loop started", zap.Duration("interval", interval))
	defer a.logger.Info("scheduler loop stopped")

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := a.scheduler.Tick(ctx); err != nil {
				a.logger.Warn("scheduler tick failed", zap.Error(err))
			}
		}
	}
}

// Router exposes the underlying mux router for tests.
func (a *App) Router() *mux.Router {
	return a.router
}

func (a *App) healthHandler(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (a *App) versionHandler(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"version": Version})
}
