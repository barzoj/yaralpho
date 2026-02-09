package ui

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed index.html app.js
var assets embed.FS

var fileServer http.Handler

func init() {
	sub, err := fs.Sub(assets, ".")
	if err != nil {
		panic(err)
	}
	fileServer = http.FileServer(http.FS(sub))
}

// IndexHandler serves the embedded index HTML for /app.
func IndexHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		data, err := assets.ReadFile("index.html")
		if err != nil {
			http.Error(w, "app UI unavailable", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(data)
	})
}

// StaticHandler serves embedded static assets under /app/static/.
func StaticHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fileServer.ServeHTTP(w, r)
	})
}
