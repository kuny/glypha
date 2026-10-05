package httpserver

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"strings"

	"github.com/kuny/glypha/internal/compiler"
	"github.com/kuny/glypha/internal/content"
	"github.com/kuny/glypha/internal/store"
)

type API struct {
	compiler           *compiler.Compiler
	store              *store.Store
	uploads, transfers chan struct{}
}

func NewAPI(c *compiler.Compiler, s *store.Store) *API {
	return &API{c, s, make(chan struct{}, 1), make(chan struct{}, 2)}
}
func acquire(ch chan struct{}) bool {
	select {
	case ch <- struct{}{}:
		return true
	default:
		return false
	}
}
func fail(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if status == 503 {
		w.Header().Set("Retry-After", "1")
	}
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"code": code, "message": message}})
}
func (a *API) upload(w http.ResponseWriter, r *http.Request) {
	if !acquire(a.uploads) {
		fail(w, 503, "busy", "An upload is already in progress.")
		return
	}
	defer func() { <-a.uploads }()
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "multipart/form-data" {
		fail(w, 415, "unsupported_media_type", "Expected multipart/form-data.")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, content.MaxPackageBytes)
	reader, err := r.MultipartReader()
	if err != nil {
		fail(w, 400, "invalid_multipart", "Malformed multipart request.")
		return
	}
	var source []byte
	hasSource := false
	assets := map[string][]byte{}
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			requestError(w, err)
			return
		}
		_, params, err := mime.ParseMediaType(part.Header.Get("Content-Disposition"))
		if err != nil {
			fail(w, 400, "invalid_part", "Malformed part header.")
			return
		}
		name := params["name"]
		limit := int64(content.MaxPackageBytes)
		if name == "content" {
			if hasSource {
				fail(w, 400, "duplicate_content", "Only one content document is allowed.")
				return
			}
			limit = content.MaxDocumentBytes
		}
		if name != "content" && name != "assets" {
			fail(w, 400, "unknown_part", "Unknown multipart part.")
			return
		}
		id := params["filename"]
		if name == "assets" {
			if id == "" || strings.ContainsAny(id, "/\\") {
				fail(w, 400, "invalid_filename", "Asset filename must be a local ID.")
				return
			}
			if _, ok := assets[id]; ok {
				fail(w, 400, "duplicate_asset", "Asset filenames must be unique.")
				return
			}
		}
		data, err := io.ReadAll(io.LimitReader(part, limit+1))
		if err != nil {
			requestError(w, err)
			return
		}
		if int64(len(data)) > limit {
			fail(w, 413, "resource_limit", "Part exceeds the configured size limit.")
			return
		}
		if name == "content" {
			source = data
			hasSource = true
		} else {
			assets[id] = data
		}
	}
	if !hasSource {
		fail(w, 400, "missing_content", "A content document is required.")
		return
	}
	candidate, issues := a.compiler.Compile(source, assets)
	if len(issues) > 0 {
		status := 422
		for _, i := range issues {
			switch i.Code {
			case "invalid_json", "invalid_type", "unknown_field", "required":
				if status != 413 {
					status = 400
				}
			case "resource_limit":
				status = 413
			}
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": "validation_failed", "issues": issues}})
		return
	}
	publication, err := a.store.Publish(r.Context(), candidate)
	if err != nil {
		slog.Error("publication failed", "error", err)
		fail(w, 503, "storage_unavailable", "Publication could not be completed safely.")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(publication)
}
func requestError(w http.ResponseWriter, err error) {
	var large *http.MaxBytesError
	if errors.As(err, &large) {
		fail(w, 413, "resource_limit", "Request exceeds 32 MiB.")
	} else {
		fail(w, 400, "invalid_multipart", "Malformed or incomplete multipart request.")
	}
}
func (a *API) display(w http.ResponseWriter, r *http.Request) {
	if !acquire(a.transfers) {
		fail(w, 503, "busy", "Too many display transfers.")
		return
	}
	defer func() { <-a.transfers }()
	snapshot, err := a.store.Snapshot(r.Context())
	if err != nil {
		slog.Error("snapshot failed", "error", err)
		fail(w, 503, "storage_unavailable", "Display content is temporarily unavailable.")
		return
	}
	w.Header().Set("Cache-Control", "no-cache")
	if len(snapshot.Body) == 0 {
		w.WriteHeader(204)
		return
	}
	w.Header().Set("ETag", snapshot.ETag)
	for _, tag := range strings.Split(r.Header.Get("If-None-Match"), ",") {
		tag = strings.TrimSpace(tag)
		if tag == "*" || strings.TrimPrefix(tag, "W/") == snapshot.ETag {
			w.WriteHeader(304)
			return
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(snapshot.Body)
}
