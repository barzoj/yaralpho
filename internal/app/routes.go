package app

import (
	"net/http"

	"github.com/barzoj/yaralpho/internal/app/ui"
)

// registerRoutes attaches middleware and HTTP handlers to the mux router.
// It is split into its own file for clarity and to keep app.go focused on
// construction and lifecycle concerns.
func (a *App) registerRoutes() {
	// Middleware order: request ID -> logging -> panic recovery.
	a.router.Use(a.requestIDMiddleware)
	a.router.Use(a.loggingMiddleware)
	a.router.Use(a.recoveryMiddleware)

	a.router.HandleFunc("/health", a.healthHandler).Methods(http.MethodGet)
	a.router.HandleFunc("/version", a.versionHandler).Methods(http.MethodGet)

	a.router.HandleFunc("/repository/{repoid}/add", a.addBatchHandler).Methods(http.MethodPost)
	a.router.HandleFunc("/repository/{repoid}/batch/{batchid}/restart", a.restartBatchHandler).Methods(http.MethodPut)
	a.router.HandleFunc("/batches", a.listBatchesHandler).Methods(http.MethodGet)
	a.router.HandleFunc("/batches/{id}", a.batchDetailHandler).Methods(http.MethodGet)
	a.router.HandleFunc("/batches/{id}/progress", a.batchProgressHandler).Methods(http.MethodGet)
	a.router.HandleFunc("/agent", a.createAgentHandler).Methods(http.MethodPost)
	a.router.HandleFunc("/agent", a.listAgentsHandler).Methods(http.MethodGet)
	a.router.HandleFunc("/agent/{id}", a.getAgentHandler).Methods(http.MethodGet)
	a.router.HandleFunc("/agent/{id}", a.updateAgentHandler).Methods(http.MethodPut)
	a.router.HandleFunc("/agent/{id}", a.deleteAgentHandler).Methods(http.MethodDelete)
	a.router.HandleFunc("/repository/{repoid}/batch/{batchid}/runs", a.listRunsHandler).Methods(http.MethodGet)
	a.router.HandleFunc("/runs/{id}/events", a.runEventsHandler).Methods(http.MethodGet)
	a.router.HandleFunc("/runs/{id}/events/live", a.runEventsLiveHandler).Methods(http.MethodGet)
	a.router.HandleFunc("/runs/{id}", a.runDetailHandler).Methods(http.MethodGet)
	a.router.Handle("/app", ui.IndexHandler()).Methods(http.MethodGet)
	a.router.Handle("/app/", ui.IndexHandler()).Methods(http.MethodGet)
	a.router.PathPrefix("/app/static/").Handler(http.StripPrefix("/app/static/", ui.StaticHandler())).Methods(http.MethodGet)
}
