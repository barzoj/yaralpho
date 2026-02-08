package app

import "net/http"

// registerRoutes attaches middleware and HTTP handlers to the mux router.
// It is split into its own file for clarity and to keep app.go focused on
// construction and lifecycle concerns.
func (a *App) registerRoutes() {
	// Middleware order: request ID -> logging -> panic recovery.
	a.router.Use(a.requestIDMiddleware)
	a.router.Use(a.loggingMiddleware)
	a.router.Use(a.recoveryMiddleware)

	a.router.HandleFunc("/health", a.healthHandler).Methods(http.MethodGet)

	a.router.HandleFunc("/add", a.addHandler).Methods(http.MethodPost)
	a.router.HandleFunc("/batches", a.listBatchesHandler).Methods(http.MethodGet)
	a.router.HandleFunc("/batches/{id}", a.batchDetailHandler).Methods(http.MethodGet)
	a.router.HandleFunc("/batches/{id}/progress", a.batchProgressHandler).Methods(http.MethodGet)
	a.router.HandleFunc("/runs", a.listRunsHandler).Methods(http.MethodGet)
	a.router.HandleFunc("/runs/{id}", a.runDetailHandler).Methods(http.MethodGet)
}
