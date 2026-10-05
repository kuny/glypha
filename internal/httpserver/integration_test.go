package httpserver

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/kuny/glypha/internal/compiler"
	"github.com/kuny/glypha/internal/content"
	"github.com/kuny/glypha/internal/store"
)

// Gates stop I/O at known boundaries instead of relying on sleeps or socket buffers.
type gatedWriter struct {
	*httptest.ResponseRecorder
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (w *gatedWriter) Write(b []byte) (int, error) {
	w.once.Do(func() { close(w.entered) })
	<-w.release
	return w.ResponseRecorder.Write(b)
}
func waitFor(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("operation did not complete")
	}
}
func serveAsync(h http.Handler, w http.ResponseWriter, r *http.Request) <-chan struct{} {
	done := make(chan struct{})
	go func() { defer close(done); h.ServeHTTP(w, r) }()
	return done
}
func dailySource(t *testing.T) []byte {
	t.Helper()
	raw, err := os.ReadFile("../../examples/daily/content.json")
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
func getDisplay(h http.Handler, etag string) *httptest.ResponseRecorder {
	r := httptest.NewRequest("GET", "/display", nil)
	if etag != "" {
		r.Header.Set("If-None-Match", etag)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}
func publish(t *testing.T, h http.Handler, r *http.Request) store.Publication {
	t.Helper()
	w := httptest.NewRecorder()
	waitFor(t, serveAsync(h, w, r))
	if w.Code != 200 {
		t.Fatalf("upload: %d %s", w.Code, w.Body)
	}
	var p store.Publication
	if err := json.Unmarshal(w.Body.Bytes(), &p); err != nil {
		t.Fatal(err)
	}
	return p
}
func imageRequest(t *testing.T, value uint8) *http.Request {
	t.Helper()
	var d content.Document
	if err := json.Unmarshal(dailySource(t), &d); err != nil {
		t.Fatal(err)
	}
	s := d.Scenes["afternoon"]
	s.Elements = []content.Element{{Type: "image", X: 100, Y: 100, Width: 200, Height: 200, Asset: "tile.png", Fit: "contain"}}
	d.Scenes["afternoon"] = s
	raw, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	img := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	for y := 0; y < 2; y++ {
		for x := 0; x < 2; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: value, A: 255})
		}
	}
	var asset, body bytes.Buffer
	if err := png.Encode(&asset, img); err != nil {
		t.Fatal(err)
	}
	writer := multipart.NewWriter(&body)
	for _, part := range []struct {
		name, filename string
		data           []byte
	}{{"content", "content.json", raw}, {"assets", "tile.png", asset.Bytes()}} {
		p, err := writer.CreateFormFile(part.name, part.filename)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = p.Write(part.data); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("PUT", "/content", &body)
	r.Header.Set("Content-Type", writer.FormDataContentType())
	return r
}
func assertImageEnvelope(t *testing.T, w *httptest.ResponseRecorder, generation string, value uint8) {
	t.Helper()
	if w.Code != 200 {
		t.Fatalf("display: %d %s", w.Code, w.Body)
	}
	var e compiler.Envelope
	if err := json.Unmarshal(w.Body.Bytes(), &e); err != nil {
		t.Fatal(err)
	}
	if e.Generation != generation || e.Scene != "afternoon" || len(e.Assets) != 1 || len(e.AST.Elements) != 1 {
		t.Fatalf("mixed envelope: %+v", e)
	}
	asset, ok := e.Assets[e.AST.Elements[0].Asset]
	if !ok {
		t.Fatal("dangling asset reference")
	}
	if asset.SHA256 != fmt.Sprintf("%x", sha256.Sum256(asset.Data)) {
		t.Fatal("asset hash mismatch")
	}
	img, err := png.Decode(bytes.NewReader(asset.Data))
	if err != nil {
		t.Fatal(err)
	}
	r, _, _, _ := img.At(0, 0).RGBA()
	if r != uint32(value)*257 {
		t.Fatal("asset from another generation")
	}
	if w.Header().Get("ETag") != fmt.Sprintf(`"%x"`, sha256.Sum256(w.Body.Bytes())) {
		t.Fatal("ETag does not describe response bytes")
	}
}
func TestSlowTransfersDoNotBlockPublication(t *testing.T) {
	h, _ := testAPI(t)
	old := publish(t, h, imageRequest(t, 20))
	release := make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	defer unblock()
	writers := make([]*gatedWriter, 2)
	done := make([]<-chan struct{}, 2)
	for i := range writers {
		writers[i] = &gatedWriter{ResponseRecorder: httptest.NewRecorder(), entered: make(chan struct{}), release: release}
		done[i] = serveAsync(h, writers[i], httptest.NewRequest("GET", "/display", nil))
		waitFor(t, writers[i].entered)
	}
	busy := getDisplay(h, "")
	if busy.Code != 503 || busy.Header().Get("Retry-After") == "" {
		t.Fatalf("transfer admission: %d", busy.Code)
	}
	// Both old responses are stopped during transmission while a replacement commits.
	next := publish(t, h, imageRequest(t, 220))
	if next.Generation == old.Generation {
		t.Fatal("generation reused")
	}
	unblock()
	for i := range writers {
		waitFor(t, done[i])
		assertImageEnvelope(t, writers[i].ResponseRecorder, old.Generation, 20)
	}
	assertImageEnvelope(t, getDisplay(h, ""), next.Generation, 220)
}

type gatedBody struct {
	entered, release chan struct{}
	once             sync.Once
}

func (b *gatedBody) Read([]byte) (int, error) {
	b.once.Do(func() { close(b.entered) })
	<-b.release
	return 0, io.ErrUnexpectedEOF
}
func (b *gatedBody) Close() error { return nil }
func TestInterruptedUploadRetainsCurrentAndReleasesAdmission(t *testing.T) {
	h, _ := testAPI(t)
	raw := dailySource(t)
	publish(t, h, uploadRequest(t, raw, false))
	before := getDisplay(h, "")
	release := make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	defer unblock()
	body := &gatedBody{entered: make(chan struct{}), release: release}
	request := httptest.NewRequest("PUT", "/content", body)
	request.Header.Set("Content-Type", "multipart/form-data; boundary=interrupted")
	response := httptest.NewRecorder()
	done := serveAsync(h, response, request)
	waitFor(t, body.entered)
	second := httptest.NewRecorder()
	h.ServeHTTP(second, uploadRequest(t, raw, false))
	if second.Code != 503 || second.Header().Get("Retry-After") != "1" {
		t.Fatalf("upload admission: %d", second.Code)
	}
	current := getDisplay(h, before.Header().Get("ETag"))
	if current.Code != 304 {
		t.Fatalf("upload blocked or changed display: %d", current.Code)
	}
	unblock()
	waitFor(t, done)
	if response.Code != 400 {
		t.Fatalf("interrupted upload: %d", response.Code)
	}
	if after := getDisplay(h, ""); !bytes.Equal(after.Body.Bytes(), before.Body.Bytes()) {
		t.Fatal("interrupted upload changed snapshot")
	}
	publish(t, h, uploadRequest(t, raw, false))
}

type mutableClock struct{ now time.Time }

func (c *mutableClock) Now() time.Time { return c.now }
func TestConditionalSchedulingPersistsBefore304(t *testing.T) {
	c, err := compiler.New("../../renderer/public/fonts/noto-sans-jp/NotoSansJP.ttf", content.Canvas{Width: 1920, Height: 1080})
	if err != nil {
		t.Fatal(err)
	}
	clock := &mutableClock{}
	set := func(value string) {
		t.Helper()
		var err error
		clock.now, err = time.Parse(time.RFC3339, value)
		if err != nil {
			t.Fatal(err)
		}
	}
	path := filepath.Join(t.TempDir(), "schedule.db")
	var s *store.Store
	open := func() http.Handler {
		t.Helper()
		var err error
		s, err = store.Open(path, c, clock)
		if err != nil {
			t.Fatal(err)
		}
		return New(fstest.MapFS{}, NewAPI(c, s))
	}
	set("2026-10-05T09:00:00+09:00")
	h := open()
	defer func() { s.Close() }()
	publish(t, h, uploadRequest(t, dailySource(t), false))
	first := getDisplay(h, "")
	etag := first.Header().Get("ETag")
	// '*' always permits 304, even though the due transition must first be committed.
	set("2026-10-05T13:00:00+09:00")
	due := getDisplay(h, "*")
	if due.Code != 304 || due.Header().Get("ETag") == etag {
		t.Fatal("conditional request did not advance")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	set("2026-10-05T11:00:00+09:00")
	h = open()
	retained := getDisplay(h, "")
	var e compiler.Envelope
	if err := json.Unmarshal(retained.Body.Bytes(), &e); err != nil {
		t.Fatal(err)
	}
	if e.Scene != "afternoon" || retained.Header().Get("ETag") != due.Header().Get("ETag") {
		t.Fatal("restart replayed a consumed transition")
	}
	set("2026-10-06T00:00:00+09:00")
	midnight := getDisplay(h, retained.Header().Get("ETag"))
	if midnight.Code != 200 {
		t.Fatalf("midnight did not reset default: %d", midnight.Code)
	}
	if err := json.Unmarshal(midnight.Body.Bytes(), &e); err != nil || e.Scene != "closed" {
		t.Fatal("wrong midnight scene", err)
	}
}
