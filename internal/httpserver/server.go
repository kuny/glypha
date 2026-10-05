// Package httpserver exposes content publication, snapshots, and renderer assets.
package httpserver

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"strings"
)

func New(assets fs.FS, api *API) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		if api != nil && !api.store.Healthy() {
			fail(w, 503, "storage_unavailable", "Store recovery is required.")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	mux.HandleFunc("GET /display", func(w http.ResponseWriter, r *http.Request) {
		if api != nil {
			api.display(w, r)
			return
		}
		// A nil API is useful for the static bootstrap and handler tests.
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("PUT /content", func(w http.ResponseWriter, r *http.Request) {
		if api != nil {
			api.upload(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotImplemented)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{
			"code": "not_implemented", "message": "Content publication is not implemented yet.",
		}})
	})
	files := http.FileServerFS(assets)
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/")
		if name == "" {
			name = "index.html"
		}
		info, err := fs.Stat(assets, name)
		if err != nil || info.IsDir() {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-cache")
		files.ServeHTTP(w, r)
	})
	return mux
}
