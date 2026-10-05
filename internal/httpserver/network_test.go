package httpserver

import (
	"bufio"
	"bytes"
	"fmt"
	"image"
	"image/png"
	"io"
	"math/rand"
	"mime/multipart"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// Small socket buffers make backpressure observable without relying on a sleep.
type smallBufferListener struct{ net.Listener }

func (l smallBufferListener) Accept() (net.Conn, error) {
	conn, err := l.Listener.Accept()
	if err == nil {
		if tcp, ok := conn.(*net.TCPConn); ok {
			if err = tcp.SetWriteBuffer(4096); err != nil {
				conn.Close()
				return nil, err
			}
		}
	}
	return conn, err
}
func networkServer(t *testing.T, h http.Handler) *httptest.Server {
	t.Helper()
	server := httptest.NewUnstartedServer(h)
	server.Listener = smallBufferListener{server.Listener}
	server.Config.ReadHeaderTimeout = 5 * time.Second
	server.Config.ReadTimeout = 10 * time.Second
	server.Config.WriteTimeout = 10 * time.Second
	server.Start()
	t.Cleanup(server.Close)
	return server
}
func socket(t *testing.T, server *httptest.Server) net.Conn {
	t.Helper()
	conn, err := net.DialTimeout("tcp", server.Listener.Addr().String(), 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	if err := conn.SetDeadline(time.Now().Add(10 * time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := conn.(*net.TCPConn).SetReadBuffer(4096); err != nil {
		t.Fatal(err)
	}
	return conn
}
func sendHTTP(t *testing.T, server *httptest.Server, request *http.Request) *http.Response {
	t.Helper()
	request.URL.Scheme = "http"
	request.URL.Host = server.Listener.Addr().String()
	request.RequestURI = ""
	response, err := (&http.Client{Timeout: 5 * time.Second}).Do(request)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { response.Body.Close() })
	return response
}
func largeImageRequest(t *testing.T) *http.Request {
	t.Helper()
	raw := bytes.ReplaceAll(dailySource(t), []byte(`"background": {`), []byte(`"background": {"image":"tile.png","fit":"cover",`))
	img := image.NewNRGBA(image.Rect(0, 0, 1024, 1024))
	rng := rand.New(rand.NewSource(7))
	if _, err := rng.Read(img.Pix); err != nil {
		t.Fatal(err)
	}
	var encoded, body bytes.Buffer
	if err := png.Encode(&encoded, img); err != nil {
		t.Fatal(err)
	}
	writer := multipart.NewWriter(&body)
	for _, part := range []struct {
		name, file string
		data       []byte
	}{{"content", "content.json", raw}, {"assets", "tile.png", encoded.Bytes()}} {
		p, err := writer.CreateFormFile(part.name, part.file)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := p.Write(part.data); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest("PUT", "/content", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	return request
}
func TestSocketBackpressureAndDisconnectedReaders(t *testing.T) {
	h, _ := testAPI(t)
	finished := make(chan struct{}, 4)
	server := networkServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/display" {
			defer func() { finished <- struct{}{} }()
		}
		h.ServeHTTP(w, r)
	}))
	upload := sendHTTP(t, server, largeImageRequest(t))
	if upload.StatusCode != 200 {
		t.Fatalf("upload: %d", upload.StatusCode)
	}
	upload.Body.Close()
	var readers []net.Conn
	var oldTag string
	for i := 0; i < 2; i++ {
		conn := socket(t, server)
		readers = append(readers, conn)
		if _, err := fmt.Fprint(conn, "GET /display HTTP/1.1\r\nHost: localhost\r\nConnection: close\r\n\r\n"); err != nil {
			t.Fatal(err)
		}
		response, err := http.ReadResponse(bufio.NewReader(conn), nil)
		if err != nil {
			t.Fatal(err)
		}
		if response.StatusCode != 200 {
			t.Fatalf("slow response: %d", response.StatusCode)
		}
		oldTag = response.Header.Get("ETag")
		// Read one byte, then leave megabytes unread in a socket with a small receive window.
		var first [1]byte
		if _, err := io.ReadFull(response.Body, first[:]); err != nil {
			t.Fatal(err)
		}
	}
	busy := sendHTTP(t, server, httptest.NewRequest("GET", "/display", nil))
	if busy.StatusCode != 503 || busy.Header.Get("Retry-After") == "" {
		t.Fatalf("slow readers did not retain admission slots: %d", busy.StatusCode)
	}
	busy.Body.Close()
	waitFor(t, finished)
	replacement := sendHTTP(t, server, uploadRequest(t, dailySource(t), false))
	if replacement.StatusCode != 200 {
		t.Fatalf("backpressure blocked publication: %d", replacement.StatusCode)
	}
	replacement.Body.Close()
	for _, conn := range readers {
		conn.Close()
	}
	waitFor(t, finished)
	waitFor(t, finished)
	current := sendHTTP(t, server, httptest.NewRequest("GET", "/display", nil))
	if current.StatusCode != 200 || current.Header.Get("ETag") == oldTag {
		t.Fatal("disconnected readers did not release slots or expose replacement")
	}
	if _, err := io.Copy(io.Discard, current.Body); err != nil {
		t.Fatal(err)
	}
	current.Body.Close()
	waitFor(t, finished)
}

type observedBody struct {
	io.ReadCloser
	once sync.Once
	read chan struct{}
}

func (b *observedBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	if n > 0 {
		b.once.Do(func() { close(b.read) })
	}
	return n, err
}
func TestSocketInterruptedUpload(t *testing.T) {
	h, _ := testAPI(t)
	read, finished := make(chan struct{}), make(chan struct{})
	server := networkServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Test-Slow") == "1" {
			r.Body = &observedBody{ReadCloser: r.Body, read: read}
			defer close(finished)
		}
		h.ServeHTTP(w, r)
	}))
	initial := sendHTTP(t, server, uploadRequest(t, dailySource(t), false))
	if initial.StatusCode != 200 {
		t.Fatal(initial.StatusCode)
	}
	initial.Body.Close()
	before := sendHTTP(t, server, httptest.NewRequest("GET", "/display", nil))
	tag := before.Header.Get("ETag")
	before.Body.Close()
	conn := socket(t, server)
	partial := "--slow\r\nContent-Disposition: form-data; name=\"content\"\r\n\r\n{" + strings.Repeat(" ", 1024)
	if _, err := fmt.Fprintf(conn, "PUT /content HTTP/1.1\r\nHost: localhost\r\nX-Test-Slow: 1\r\nContent-Type: multipart/form-data; boundary=slow\r\nContent-Length: 100000\r\n\r\n%s", partial); err != nil {
		t.Fatal(err)
	}
	waitFor(t, read)
	busy := sendHTTP(t, server, uploadRequest(t, dailySource(t), false))
	if busy.StatusCode != 503 {
		t.Fatalf("slow upload did not retain admission: %d", busy.StatusCode)
	}
	busy.Body.Close()
	conditional := httptest.NewRequest("GET", "/display", nil)
	conditional.Header.Set("If-None-Match", tag)
	current := sendHTTP(t, server, conditional)
	if current.StatusCode != 304 {
		t.Fatal("slow upload changed or blocked display")
	}
	current.Body.Close()
	conn.Close()
	waitFor(t, finished)
	conditional = httptest.NewRequest("GET", "/display", nil)
	conditional.Header.Set("If-None-Match", tag)
	current = sendHTTP(t, server, conditional)
	if current.StatusCode != 304 {
		t.Fatal("disconnected upload changed current content")
	}
	current.Body.Close()
	retry := sendHTTP(t, server, uploadRequest(t, dailySource(t), false))
	if retry.StatusCode != 200 {
		t.Fatalf("upload slot not released: %d", retry.StatusCode)
	}
	retry.Body.Close()
}
