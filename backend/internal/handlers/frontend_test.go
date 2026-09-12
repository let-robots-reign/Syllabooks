package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
)

// The frontend tests need no database: they never reach a query.

func TestFrontend(t *testing.T) {
	const index = "<!doctype html><title>Syllabooks</title>"
	built := fstest.MapFS{
		"index.html":          {Data: []byte(index)},
		"favicon.svg":         {Data: []byte("<svg/>")},
		"assets/index-abc.js": {Data: []byte("console.log(1)")},
		".gitkeep":            {},
	}
	routes := (&Server{Frontend: built}).Routes()

	for _, tc := range []struct {
		method, path string
		status       int
		body, cache  string // checked only for a 200
	}{
		{"GET", "/", http.StatusOK, index, "no-cache"},
		// Client-side routes load the app, which then shows the right screen.
		{"GET", "/profile", http.StatusOK, index, "no-cache"},
		{"GET", "/books/0b6e2a", http.StatusOK, index, "no-cache"},
		{"GET", "/favicon.svg", http.StatusOK, "<svg/>", ""},
		{"GET", "/assets/index-abc.js", http.StatusOK, "console.log(1)", "public, max-age=31536000, immutable"},
		{"HEAD", "/", http.StatusOK, "", "no-cache"},
		{"GET", "/assets/index-old.js", http.StatusNotFound, "", ""},
		{"GET", "/.gitkeep", http.StatusNotFound, "", ""},
		{"GET", "/api/nope", http.StatusNotFound, "", ""},
		{"POST", "/profile", http.StatusMethodNotAllowed, "", ""},
	} {
		rec := httptest.NewRecorder()
		routes.ServeHTTP(rec, httptest.NewRequest(tc.method, tc.path, nil))
		if rec.Code != tc.status {
			t.Errorf("%s %s: status %d, want %d", tc.method, tc.path, rec.Code, tc.status)
			continue
		}
		body := rec.Body.String()
		if tc.status != http.StatusOK {
			if body == index {
				t.Errorf("%s %s: answered with index.html", tc.method, tc.path)
			}
			continue
		}
		if tc.method == "GET" && body != tc.body {
			t.Errorf("%s %s: body %q, want %q", tc.method, tc.path, body, tc.body)
		}
		if got := rec.Header().Get("Cache-Control"); got != tc.cache {
			t.Errorf("%s %s: Cache-Control %q, want %q", tc.method, tc.path, got, tc.cache)
		}
	}

	// A binary built without the frontend serves only the API.
	rec := httptest.NewRecorder()
	(&Server{Frontend: fstest.MapFS{".gitkeep": {}}}).Routes().ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("GET / without a frontend: status %d, want %d", rec.Code, http.StatusNotFound)
	}
}
