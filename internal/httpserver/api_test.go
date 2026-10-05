package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/kuny/glypha/internal/compiler"
	"github.com/kuny/glypha/internal/content"
	"github.com/kuny/glypha/internal/store"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

type testClock struct{}

func (testClock) Now() time.Time { return time.Date(2026, 10, 5, 5, 0, 0, 0, time.UTC) }
func testAPI(t *testing.T) (http.Handler, *store.Store) {
	t.Helper()
	c, err := compiler.New("../../renderer/public/fonts/noto-sans-jp/NotoSansJP.ttf", content.Canvas{Width: 1920, Height: 1080})
	if err != nil {
		t.Fatal(err)
	}
	s, err := store.Open(filepath.Join(t.TempDir(), "test.db"), c, testClock{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return New(fstest.MapFS{}, NewAPI(c, s)), s
}
func uploadRequest(t *testing.T, source []byte, extra bool) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, _ := writer.CreateFormFile("content", "content.json")
	_, _ = part.Write(source)
	if extra {
		part, _ = writer.CreateFormFile("assets", "unused.png")
		_, _ = part.Write([]byte("invalid"))
	}
	writer.Close()
	req := httptest.NewRequest("PUT", "/content", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return req
}
func TestUploadRetrieveAndReject(t *testing.T) {
	handler, s := testAPI(t)
	raw, err := os.ReadFile("../../examples/daily/content.json")
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, uploadRequest(t, raw, false))
	if w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	get := httptest.NewRecorder()
	handler.ServeHTTP(get, httptest.NewRequest("GET", "/display", nil))
	if get.Code != 200 {
		t.Fatal(get.Code)
	}
	var envelope compiler.Envelope
	if err = json.Unmarshal(get.Body.Bytes(), &envelope); err != nil || envelope.Scene != "afternoon" {
		t.Fatal(envelope, err)
	}
	etag := get.Header().Get("ETag")
	if etag == "" {
		t.Fatal("missing ETag")
	}
	request := httptest.NewRequest("GET", "/display", nil)
	request.Header.Set("If-None-Match", "W/"+etag)
	same := httptest.NewRecorder()
	handler.ServeHTTP(same, request)
	if same.Code != 304 || same.Body.Len() != 0 {
		t.Fatal(same.Code)
	}
	for _, tc := range []struct {
		source []byte
		extra  bool
		status int
	}{
		{[]byte(`{"format":1,"format":1}`), false, 400},
		{raw, true, 422},
		{[]byte(strings.Replace(string(raw), "Welcome", strings.Repeat("W", 500), 1)), false, 422},
	} {
		w = httptest.NewRecorder()
		handler.ServeHTTP(w, uploadRequest(t, tc.source, tc.extra))
		if w.Code != tc.status {
			t.Fatalf("%d %s", w.Code, w.Body)
		}
		snapshot, err := s.Snapshot(context.Background())
		if err != nil || snapshot.ETag != etag {
			t.Fatal("rejection modified content", err)
		}
	}
}
func TestMultipartAndLimits(t *testing.T) {
	handler, _ := testAPI(t)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest("PUT", "/content", strings.NewReader("x")))
	if w.Code != 415 {
		t.Fatal(w.Code)
	}
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, uploadRequest(t, bytes.Repeat([]byte("x"), content.MaxDocumentBytes+1), false))
	if w.Code != 413 {
		t.Fatal(w.Code)
	}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for i := 0; i < 2; i++ {
		p, _ := writer.CreateFormFile("content", "content.json")
		p.Write([]byte("{}"))
	}
	writer.Close()
	req := httptest.NewRequest("PUT", "/content", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != 400 {
		t.Fatal(w.Code)
	}
}
