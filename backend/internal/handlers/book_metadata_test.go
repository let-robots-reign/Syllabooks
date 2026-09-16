package handlers

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func TestNormalizeISBN(t *testing.T) {
	normalized, err := normalizeISBN("978-0-380-80734-5")
	if err != nil || normalized == nil || *normalized != "9780380807345" {
		t.Fatalf("normalize valid ISBN: got %v, %v", normalized, err)
	}
	if _, err := normalizeISBN("978-0-380-80734-6"); err == nil {
		t.Fatal("invalid checksum was accepted")
	}
	if normalized, err := normalizeISBN("  "); err != nil || normalized != nil {
		t.Fatalf("empty ISBN: got %v, %v", normalized, err)
	}
}

func TestLookupMetadataMergesGoogleFallback(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		var body string
		switch {
		case r.URL.Host == "open.test" && r.URL.Path == "/api/books":
			body = `{"ISBN:9780380807345":{"title":"Coraline","number_of_pages":162,"authors":[{"name":"Neil Gaiman"}]}}`
		case r.URL.Host == "open.test" && r.URL.Path == "/isbn/9780380807345.json":
			body = `{"description":{"value":"  A door <b>behind</b> the wallpaper.  "}}`
		case r.URL.Host == "google.test" && r.URL.Path == "/volumes":
			body = `{"items":[{"volumeInfo":{"title":"Wrong fallback title","authors":["Other"],"pageCount":999,"description":"Fallback","imageLinks":{"large":"https://images.test/cover.jpg"}}}]}`
		default:
			t.Fatalf("unexpected metadata request: %s", r.URL)
		}
		return jsonResponse(body), nil
	})}
	s := Server{
		HTTPClient:         client,
		OpenLibraryBaseURL: "https://open.test",
		GoogleBooksBaseURL: "https://google.test",
	}
	metadata, found, err := s.lookupMetadata(context.Background(), "9780380807345")
	if err != nil || !found {
		t.Fatalf("lookup: found=%v err=%v", found, err)
	}
	if metadata.Title != "Coraline" || metadata.Author != "Neil Gaiman" || metadata.PageCount != 162 {
		t.Fatalf("Open Library values were overwritten: %+v", metadata)
	}
	if metadata.Description != "A door behind the wallpaper." {
		t.Errorf("description = %q", metadata.Description)
	}
	if metadata.CoverURL != "https://images.test/cover.jpg" || metadata.Source != "openlibrary+google" {
		t.Errorf("fallback cover/source: %+v", metadata)
	}
}

func TestMissingMetadataFields(t *testing.T) {
	missing := missingMetadataFields(bookMetadata{Title: "Coraline", Author: "Neil Gaiman"})
	want := []string{"page_count", "description", "cover"}
	if strings.Join(missing, ",") != strings.Join(want, ",") {
		t.Fatalf("missing fields = %v, want %v", missing, want)
	}
}

func TestLookupMetadataFallsBackWhenOpenLibraryHasNoBook(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host == "open.test" {
			return jsonResponse(`{}`), nil
		}
		return jsonResponse(`{"items":[{"volumeInfo":{"title":"Matilda","authors":["Roald Dahl"],"pageCount":240}}]}`), nil
	})}
	s := Server{HTTPClient: client, OpenLibraryBaseURL: "https://open.test", GoogleBooksBaseURL: "https://google.test"}
	metadata, found, err := s.lookupMetadata(context.Background(), "9780142410370")
	if err != nil || !found || metadata.Source != "google" || metadata.Title != "Matilda" {
		t.Fatalf("Google fallback: found=%v metadata=%+v err=%v", found, metadata, err)
	}
}

func TestLookupMetadataReportsProviderFailure(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("provider unavailable")
	})}
	s := Server{HTTPClient: client, OpenLibraryBaseURL: "https://open.test", GoogleBooksBaseURL: "https://google.test"}
	_, found, err := s.lookupMetadata(context.Background(), "9780142410370")
	if found || err == nil {
		t.Fatalf("provider failure: found=%v err=%v", found, err)
	}
	message := metadataLookupErrorMessage(err)
	if !strings.Contains(message, "Open Library — ошибка подключения") ||
		!strings.Contains(message, "Google Books — ошибка подключения") {
		t.Fatalf("provider failure message = %q", message)
	}
}

func TestMetadataLookupErrorMessageExplainsProviderFailures(t *testing.T) {
	err := errors.Join(
		&metadataProviderError{Provider: "Open Library", Err: context.DeadlineExceeded},
		&metadataProviderError{Provider: "Google Books", Err: &metadataHTTPError{StatusCode: http.StatusForbidden}},
	)
	message := metadataLookupErrorMessage(err)
	if !strings.Contains(message, "Open Library — превышено время ожидания") ||
		!strings.Contains(message, "Google Books — сервис отказал в доступе (HTTP 403)") {
		t.Fatalf("failure message = %q", message)
	}
}

func TestLookupMetadataTreatsProviderNotFoundAsMissingBook(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusNotFound,
			Status:     "404 Not Found",
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{}`)),
		}, nil
	})}
	s := Server{HTTPClient: client, OpenLibraryBaseURL: "https://open.test", GoogleBooksBaseURL: "https://google.test"}
	_, found, err := s.lookupMetadata(context.Background(), "9780142410370")
	if found || err != nil {
		t.Fatalf("missing book: found=%v err=%v", found, err)
	}
}

func TestValidateBook(t *testing.T) {
	valid := bookInput{
		ISBN: "9780380807345", Title: "Coraline", Author: "Neil Gaiman",
		Level: "yellow", PageCount: 162,
	}
	for _, test := range []struct {
		name  string
		alter func(*bookInput)
	}{
		{"missing title", func(book *bookInput) { book.Title = "" }},
		{"bad level", func(book *bookInput) { book.Level = "blue" }},
		{"zero pages", func(book *bookInput) { book.PageCount = 0 }},
		{"long description", func(book *bookInput) { book.Description = strings.Repeat("я", 241) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			input := valid
			test.alter(&input)
			response := httptest.NewRecorder()
			if _, ok := validateBook(response, input); ok || response.Code != http.StatusBadRequest {
				t.Fatalf("validate = %v, status = %d", ok, response.Code)
			}
		})
	}
}

func TestShortenDescription(t *testing.T) {
	long := strings.Repeat("слово ", 60)
	short := shortenDescription(long)
	if len([]rune(short)) > 240 || !strings.HasSuffix(short, "…") {
		t.Fatalf("description was not shortened cleanly: %d %q", len([]rune(short)), short)
	}
}

func TestStoreAndRemoveCover(t *testing.T) {
	dir := t.TempDir()
	s := Server{BookCoversDir: dir}
	// DetectContentType only needs the PNG signature for this storage-level test.
	data := append([]byte("\x89PNG\r\n\x1a\n"), bytes.Repeat([]byte{0}, 32)...)
	coverURL, err := s.storeCover(data, "image/png")
	if err != nil {
		t.Fatalf("store cover: %v", err)
	}
	filename := strings.TrimPrefix(coverURL, coverURLPrefix)
	path := filepath.Join(dir, filename)
	if stored, err := os.ReadFile(path); err != nil || !bytes.Equal(stored, data) {
		t.Fatalf("stored cover differs: %v", err)
	}
	s.removeStoredCover(&coverURL)
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("cover still exists after removal: %v", err)
	}
}

func jsonResponse(body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Status:     "200 OK",
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}
