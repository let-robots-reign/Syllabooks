package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestAdminBookCRUDAndCovers(t *testing.T) {
	emptyMetadata := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return jsonResponse(`{}`), nil
	})}
	coverDir := t.TempDir()
	env := newTestEnv(t, Server{
		BookCoversDir:      coverDir,
		HTTPClient:         emptyMetadata,
		OpenLibraryBaseURL: "https://open.test",
		GoogleBooksBaseURL: "https://google.test",
	})
	adminID, adminCode := env.newCodeUser(t)
	env.exec(t, "UPDATE users SET is_admin = true WHERE id = $1", adminID)
	adminToken := env.login(t, adminCode, "учитель")
	_, studentCode := env.newCodeUser(t)
	studentToken := env.login(t, studentCode, "ученик1")

	status, _ := bookRequest[map[string]any](t, env, http.MethodGet, "/api/admin/books", studentToken, nil)
	if status != http.StatusNotFound {
		t.Fatalf("non-admin catalogue status = %d, want 404", status)
	}

	input := bookInput{
		ISBN: "978-0-380-80734-5", Title: "Coraline", Author: "Neil Gaiman",
		Level: "yellow", PageCount: 162, Description: "A small door in an old house.",
	}
	status, created := bookRequest[bookResponse](t, env, http.MethodPost, "/api/admin/books", adminToken, input)
	if status != http.StatusCreated || created.ISBN == nil || *created.ISBN != "9780380807345" {
		t.Fatalf("create: status=%d book=%+v", status, created)
	}
	t.Cleanup(func() {
		env.pool.Exec(context.Background(), "DELETE FROM loans WHERE book_id = $1", created.ID)
		env.pool.Exec(context.Background(), "DELETE FROM books WHERE id = $1", created.ID)
	})

	status, _ = bookRequest[map[string]any](t, env, http.MethodPost, "/api/admin/books", adminToken, input)
	if status != http.StatusConflict {
		t.Fatalf("duplicate ISBN status = %d, want 409", status)
	}

	input.Title = "Coraline — special edition"
	status, updated := bookRequest[bookResponse](t, env, http.MethodPut, "/api/admin/books/"+created.ID.String(), adminToken, input)
	if status != http.StatusOK || updated.Title != input.Title {
		t.Fatalf("update: status=%d book=%+v", status, updated)
	}

	status, books := bookRequest[[]bookResponse](t, env, http.MethodGet, "/api/admin/books", adminToken, nil)
	if status != http.StatusOK || !containsBook(books, created.ID) {
		t.Fatalf("list: status=%d books=%+v", status, books)
	}

	status, _ = bookRequest[bookResponse](t, env, http.MethodGet, "/api/admin/books/"+uuid.NewString(), adminToken, nil)
	if status != http.StatusNotFound {
		t.Fatalf("unknown book status = %d, want 404", status)
	}

	var lastCoverURL string
	for _, cover := range []struct {
		name string
		data []byte
	}{
		{"cover.png", append([]byte("\x89PNG\r\n\x1a\n"), bytes.Repeat([]byte{0}, 32)...)},
		{"cover.jpg", append([]byte("\xff\xd8\xff\xdb"), bytes.Repeat([]byte{0}, 32)...)},
		{"cover.webp", append([]byte("RIFF\x10\x00\x00\x00WEBPVP8 "), bytes.Repeat([]byte{0}, 32)...)},
	} {
		status, covered := uploadCover(t, env, created.ID, adminToken, cover.name, cover.data)
		if status != http.StatusOK || covered.CoverURL == nil {
			t.Fatalf("upload %s: status=%d book=%+v", cover.name, status, covered)
		}
		lastCoverURL = *covered.CoverURL
		stored := filepath.Join(coverDir, filepath.Base(*covered.CoverURL))
		if _, err := os.Stat(stored); err != nil {
			t.Fatalf("uploaded %s is not stored: %v", cover.name, err)
		}
	}
	resp, err := env.client.Get(env.srv.URL + lastCoverURL)
	if err != nil {
		t.Fatalf("get stored cover: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || resp.Header.Get("Cache-Control") == "" {
		t.Fatalf("public cover response: status=%d cache=%q", resp.StatusCode, resp.Header.Get("Cache-Control"))
	}
	status, _ = uploadCover(t, env, created.ID, adminToken, "cover.txt", []byte("not an image"))
	if status != http.StatusUnsupportedMediaType {
		t.Fatalf("text cover status = %d, want 415", status)
	}
	status, _ = uploadCover(t, env, created.ID, adminToken, "huge.png", bytes.Repeat([]byte{0}, maxCoverBytes+1))
	if status != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized cover status = %d, want 413", status)
	}

	status, withoutCover := bookRequest[bookResponse](t, env, http.MethodDelete, "/api/admin/books/"+created.ID.String()+"/cover", adminToken, nil)
	if status != http.StatusOK || withoutCover.CoverURL != nil {
		t.Fatalf("remove cover: status=%d book=%+v", status, withoutCover)
	}

	env.exec(t, `INSERT INTO loans (book_id, user_id, due_at) VALUES ($1, $2, $3)`, created.ID, adminID, time.Now().Add(21*24*time.Hour))
	status, _ = bookRequest[map[string]any](t, env, http.MethodDelete, "/api/admin/books/"+created.ID.String(), adminToken, nil)
	if status != http.StatusConflict {
		t.Fatalf("delete book with history status = %d, want 409", status)
	}
	env.exec(t, "DELETE FROM loans WHERE book_id = $1", created.ID)
	status, _ = bookRequest[map[string]any](t, env, http.MethodDelete, "/api/admin/books/"+created.ID.String(), adminToken, nil)
	if status != http.StatusNoContent {
		t.Fatalf("delete book status = %d, want 204", status)
	}
}

func TestCreateBookDownloadsProviderCover(t *testing.T) {
	coverData := append([]byte("\xff\xd8\xff\xdb"), bytes.Repeat([]byte{0}, 32)...)
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		switch {
		case r.URL.Host == "open.test" && r.URL.Path == "/api/books":
			return jsonResponse(`{"ISBN:9780142410370":{"title":"Matilda","number_of_pages":240,"authors":[{"name":"Roald Dahl"}],"cover":{"large":"http://images.test/matilda.jpg"}}}`), nil
		case r.URL.Host == "open.test" && r.URL.Path == "/isbn/9780142410370.json":
			return jsonResponse(`{}`), nil
		case r.URL.Host == "google.test":
			return jsonResponse(`{}`), nil
		case r.URL.Host == "images.test":
			return &http.Response{StatusCode: http.StatusOK, Status: "200 OK", Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(coverData))}, nil
		default:
			t.Fatalf("unexpected request: %s", r.URL)
			return nil, nil
		}
	})}
	coverDir := t.TempDir()
	env := newTestEnv(t, Server{
		BookCoversDir: coverDir, HTTPClient: client,
		OpenLibraryBaseURL: "https://open.test", GoogleBooksBaseURL: "https://google.test",
	})
	adminID, adminCode := env.newCodeUser(t)
	env.exec(t, "UPDATE users SET is_admin = true WHERE id = $1", adminID)
	token := env.login(t, adminCode, "учитель")

	status, created := bookRequest[bookResponse](t, env, http.MethodPost, "/api/admin/books", token, bookInput{
		ISBN: "9780142410370", Level: "green",
	})
	if status != http.StatusCreated || created.CoverURL == nil || created.Title != "Matilda" {
		t.Fatalf("create with metadata: status=%d book=%+v", status, created)
	}
	path := filepath.Join(coverDir, filepath.Base(*created.CoverURL))
	stored, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(stored, coverData) {
		t.Fatalf("downloaded cover: %v", err)
	}
	t.Cleanup(func() {
		env.pool.Exec(context.Background(), "DELETE FROM books WHERE id = $1", created.ID)
		os.Remove(path)
	})
}

func TestAdminCanReorderBooksForAdminAndPublicCatalogues(t *testing.T) {
	env := newTestEnv(t, Server{})
	adminID, adminCode := env.newCodeUser(t)
	env.exec(t, "UPDATE users SET is_admin = true WHERE id = $1", adminID)
	adminToken := env.login(t, adminCode, "учитель")
	_, studentCode := env.newCodeUser(t)
	studentToken := env.login(t, studentCode, "ученик")

	firstID, secondID := uuid.New(), uuid.New()
	env.exec(t, `INSERT INTO books (id, title, author, level, page_count)
		VALUES ($1, 'Order First', 'Test Author', 'green', 10),
		       ($2, 'Order Second', 'Test Author', 'green', 10)`, firstID, secondID)
	t.Cleanup(func() {
		env.pool.Exec(context.Background(), "DELETE FROM books WHERE id = ANY($1)", []uuid.UUID{firstID, secondID})
	})

	status, listed := bookRequest[[]bookResponse](t, env, http.MethodGet, "/api/admin/books", adminToken, nil)
	if status != http.StatusOK {
		t.Fatalf("list before reorder: status=%d", status)
	}
	orderedIDs := make([]uuid.UUID, len(listed))
	for index, book := range listed {
		orderedIDs[len(listed)-1-index] = book.ID
	}
	status, _ = bookRequest[map[string]any](t, env, http.MethodPut, "/api/admin/books/order", adminToken, map[string]any{"ids": orderedIDs})
	if status != http.StatusNoContent {
		t.Fatalf("reorder: status=%d", status)
	}

	status, reordered := bookRequest[[]bookResponse](t, env, http.MethodGet, "/api/admin/books", adminToken, nil)
	if status != http.StatusOK || len(reordered) != len(orderedIDs) {
		t.Fatalf("list after reorder: status=%d books=%+v", status, reordered)
	}
	for index, id := range orderedIDs {
		if reordered[index].ID != id || reordered[index].Ord != int32(index+1) {
			t.Fatalf("book %d after reorder = %+v, want id=%s ord=%d", index, reordered[index], id, index+1)
		}
	}

	type catalogPayload struct {
		Books []bookResponse `json:"books"`
	}
	status, catalogue := bookRequest[catalogPayload](t, env, http.MethodGet, "/api/books", studentToken, nil)
	if status != http.StatusOK || bookIndex(catalogue.Books, secondID) >= bookIndex(catalogue.Books, firstID) {
		t.Fatalf("public catalogue did not preserve reordered books: status=%d books=%+v", status, catalogue.Books)
	}
}

func bookIndex(books []bookResponse, id uuid.UUID) int {
	for index, book := range books {
		if book.ID == id {
			return index
		}
	}
	return len(books)
}

func bookRequest[T any](t *testing.T, env *testEnv, method, path, token string, body any) (int, T) {
	t.Helper()
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequest(method, env.srv.URL+path, reader)
	if err != nil {
		t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: token})
	}
	resp, err := env.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var result T
	if resp.StatusCode != http.StatusNoContent {
		_ = json.NewDecoder(resp.Body).Decode(&result)
	}
	return resp.StatusCode, result
}

func uploadCover(t *testing.T, env *testEnv, id uuid.UUID, token, filename string, data []byte) (int, bookResponse) {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("cover", filename)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPut, env.srv.URL+"/api/admin/books/"+id.String()+"/cover", &body)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: token})
	resp, err := env.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var result bookResponse
	_ = json.NewDecoder(resp.Body).Decode(&result)
	return resp.StatusCode, result
}

func containsBook(books []bookResponse, id uuid.UUID) bool {
	for _, book := range books {
		if book.ID == id {
			return true
		}
	}
	return false
}

func TestAdminCanDeactivateBook(t *testing.T) {
	env := newTestEnv(t, Server{})
	adminID, adminCode := env.newCodeUser(t)
	env.exec(t, "UPDATE users SET is_admin = true WHERE id = $1", adminID)
	adminToken := env.login(t, adminCode, "учитель")
	_, studentCode := env.newCodeUser(t)
	studentToken := env.login(t, studentCode, "ученик")
	book := newBorrowTestBook(t, env, randomTestISBN(), "Resting Book")
	path := "/api/admin/books/" + book.ID.String() + "/active"

	status, _ := bookRequest[map[string]any](t, env, http.MethodPut, path, studentToken, map[string]bool{"is_active": false})
	if status != http.StatusNotFound {
		t.Fatalf("student deactivate status = %d, want 404", status)
	}

	status, updated := bookRequest[bookResponse](t, env, http.MethodPut, path, adminToken, map[string]bool{"is_active": false})
	if status != http.StatusOK || updated.IsActive {
		t.Fatalf("deactivate: status=%d body=%+v", status, updated)
	}

	status, adminList := bookRequest[[]bookResponse](t, env, http.MethodGet, "/api/admin/books", adminToken, nil)
	if status != http.StatusOK || !containsBook(adminList, book.ID) {
		t.Fatalf("admin list should keep inactive book: status=%d", status)
	}
	type catalogPayload struct {
		Books []bookResponse `json:"books"`
	}
	status, catalogue := bookRequest[catalogPayload](t, env, http.MethodGet, "/api/books", studentToken, nil)
	if status != http.StatusOK || containsBook(catalogue.Books, book.ID) {
		t.Fatalf("public catalogue should hide inactive book: status=%d", status)
	}
	status, _ = bookRequest[map[string]any](t, env, http.MethodGet, "/api/books/"+book.ID.String(), studentToken, nil)
	if status != http.StatusNotFound {
		t.Fatalf("inactive book detail status = %d, want 404", status)
	}
	status, borrow := bookRequest[borrowErrorResponse](t, env, http.MethodPost, "/api/loans", studentToken, map[string]string{"isbn": *book.Isbn})
	if status != http.StatusConflict || borrow.Code != "book_inactive" {
		t.Fatalf("borrow inactive book: status=%d body=%+v", status, borrow)
	}

	status, updated = bookRequest[bookResponse](t, env, http.MethodPut, path, adminToken, map[string]bool{"is_active": true})
	if status != http.StatusOK || !updated.IsActive {
		t.Fatalf("reactivate: status=%d body=%+v", status, updated)
	}
	status, catalogue = bookRequest[catalogPayload](t, env, http.MethodGet, "/api/books", studentToken, nil)
	if status != http.StatusOK || !containsBook(catalogue.Books, book.ID) {
		t.Fatalf("public catalogue should show reactivated book: status=%d", status)
	}
}
