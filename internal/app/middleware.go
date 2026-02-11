package app

import (
	"bufio"
	"fmt"
	"net"
	"net/http"
	"runtime/debug"
	"strconv"
	"sync/atomic"

	"go.uber.org/zap"
)

const requestIDHeader = "X-Request-ID"

// requestIDMiddleware ensures every request has a request ID for logging. If
// the header is already present, it is reused; otherwise an incrementing
// counter generates a lightweight ID.
func (a *App) requestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqID := r.Header.Get(requestIDHeader)
		if reqID == "" {
			id := atomic.AddUint64(&a.reqCounter, 1)
			reqID = strconv.FormatUint(id, 10)
			r.Header.Set(requestIDHeader, reqID)
		}
		w.Header().Set(requestIDHeader, reqID)
		next.ServeHTTP(w, r)
	})
}

// loggingMiddleware logs method, path, status, and request ID after the handler
// completes.
func (a *App) loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rw := &responseWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rw, r)
		a.logger.Info("http request",
			zap.String("method", r.Method),
			zap.String("path", r.URL.Path),
			zap.Int("status", rw.status),
			zap.String("request_id", r.Header.Get(requestIDHeader)),
		)
	})
}

// recoveryMiddleware catches panics, logs them, and returns a 500 JSON error
// without crashing the server.
func (a *App) recoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				a.logger.Error("panic recovered", zap.Any("error", rec), zap.ByteString("stack", debug.Stack()))
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// responseWriter captures the status code for logging.
type responseWriter struct {
	http.ResponseWriter
	status int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if h, ok := rw.ResponseWriter.(http.Hijacker); ok {
		return h.Hijack()
	}
	return nil, nil, fmt.Errorf("hijacker not supported")
}
