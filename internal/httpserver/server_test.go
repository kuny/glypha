package httpserver

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func TestBootstrapRoutes(t *testing.T) {
	handler := New(fstest.MapFS{
		"index.html":    {Data: []byte("<title>Glypha</title>")},
		"assets/app.js": {Data: []byte("export {};")},
	})
	for _, tc := range []struct {
		method, path string
		status       int
		body         string
	}{
		{"GET", "/healthz", 200, `"status":"ok"`},
		{"GET", "/display", 204, ""},
		{"PUT", "/content", 501, `"code":"not_implemented"`},
		{"GET", "/", 200, "<title>Glypha</title>"},
		{"GET", "/assets/app.js", 200, "export {};"},
		{"GET", "/assets/", 404, ""},
		{"GET", "/unknown", 404, ""},
		{"POST", "/display", 405, ""},
	} {
		t.Run(tc.method+tc.path, func(t *testing.T) {
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(tc.method, tc.path, nil))
			if response.Code != tc.status || !strings.Contains(response.Body.String(), tc.body) {
				t.Fatalf("got %d %s", response.Code, response.Body.String())
			}
			if tc.status == http.StatusNoContent && response.Body.Len() != 0 {
				t.Fatal("204 must not contain a body")
			}
		})
	}
}
