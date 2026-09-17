package handlers

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestStudentCatalog(t *testing.T) {
	env := newTestEnv(t, Server{})
	viewerID, viewerCode := env.newCodeUser(t)
	token := env.login(t, viewerCode, "читатель")
	borrowerID, _ := env.newCodeUser(t)
	env.exec(t, "UPDATE users SET display_name = 'Аня' WHERE id = $1", borrowerID)

	availableID := uuid.New()
	borrowedID := uuid.New()
	lostID := uuid.New()
	for _, book := range []struct {
		id    uuid.UUID
		title string
		level string
		pages int
		lost  bool
		notes string
		descr string
	}{
		{availableID, "Catalog Available", "green", 21, false, "private available note", "Short description"},
		{borrowedID, "Catalog Borrowed", "yellow", 42, false, "private borrowed note", ""},
		{lostID, "Catalog Lost", "red", 63, true, "private lost note", ""},
	} {
		env.exec(t, `INSERT INTO books (id, title, author, level, page_count, is_lost, lost_at, notes, description)
			VALUES ($1, $2, 'Test Author', $3, $4, $5, CASE WHEN $5 THEN now() ELSE NULL END, $6, NULLIF($7, ''))`,
			book.id, book.title, book.level, book.pages, book.lost, book.notes, book.descr)
	}
	t.Cleanup(func() {
		ids := []uuid.UUID{availableID, borrowedID, lostID}
		if _, err := env.pool.Exec(context.Background(), "DELETE FROM loans WHERE book_id = ANY($1)", ids); err != nil {
			t.Errorf("delete catalogue test loans: %v", err)
		}
		if _, err := env.pool.Exec(context.Background(), "DELETE FROM books WHERE id = ANY($1)", ids); err != nil {
			t.Errorf("delete catalogue test books: %v", err)
		}
	})

	var baseline int64
	if err := env.pool.QueryRow(t.Context(), "SELECT count(*) FROM loans WHERE return_reason = 'finished'").Scan(&baseline); err != nil {
		t.Fatal(err)
	}
	dueAt := time.Now().UTC().Add(21 * 24 * time.Hour).Truncate(time.Microsecond)
	env.exec(t, `INSERT INTO loans (book_id, user_id, due_at)
		VALUES ($1, $2, $3)`, borrowedID, borrowerID, dueAt)
	env.exec(t, `INSERT INTO loans (book_id, user_id, due_at, returned_at, return_reason, shelf_scan_ok, book_scan_ok)
		VALUES ($1, $2, now() + interval '21 days', now(), 'finished', true, true)`, availableID, viewerID)
	env.exec(t, `INSERT INTO loans (book_id, user_id, due_at, returned_at, return_reason, shelf_scan_ok, book_scan_ok)
		VALUES ($1, $2, now() + interval '21 days', now(), 'skipped', true, true)`, lostID, viewerID)

	status, _ := bookRequest[map[string]any](t, env, http.MethodGet, "/api/books", "", nil)
	if status != http.StatusUnauthorized {
		t.Fatalf("anonymous catalogue status = %d, want 401", status)
	}
	status, _ = bookRequest[map[string]any](t, env, http.MethodGet, "/api/books/"+availableID.String(), "", nil)
	if status != http.StatusUnauthorized {
		t.Fatalf("anonymous book detail status = %d, want 401", status)
	}

	type payload struct {
		FinishedCount int64            `json:"finished_count"`
		Books         []map[string]any `json:"books"`
	}
	status, catalogue := bookRequest[payload](t, env, http.MethodGet, "/api/books", token, nil)
	if status != http.StatusOK {
		t.Fatalf("catalogue status = %d, want 200", status)
	}
	if catalogue.FinishedCount != baseline+1 {
		t.Fatalf("finished_count = %d, want %d", catalogue.FinishedCount, baseline+1)
	}

	available := responseBook(catalogue.Books, availableID)
	if available == nil || available["current_loan"] != nil {
		t.Fatalf("available book = %#v", available)
	}
	if available["description"] != "Short description" || available["page_count"] != float64(21) {
		t.Fatalf("available book fields = %#v", available)
	}
	if _, exposed := available["notes"]; exposed {
		t.Fatalf("catalogue exposed private notes: %#v", available)
	}

	borrowed := responseBook(catalogue.Books, borrowedID)
	loan, ok := borrowed["current_loan"].(map[string]any)
	if borrowed == nil || !ok || loan["borrower_name"] != "Аня" {
		t.Fatalf("borrowed book = %#v", borrowed)
	}
	gotDue, err := time.Parse(time.RFC3339Nano, loan["due_at"].(string))
	if err != nil || !gotDue.Equal(dueAt) {
		t.Fatalf("due_at = %v (%v), want %v", loan["due_at"], err, dueAt)
	}
	if responseBook(catalogue.Books, lostID) != nil {
		t.Fatal("lost book appeared in catalogue")
	}

	status, detail := bookRequest[map[string]any](t, env, http.MethodGet, "/api/books/"+borrowedID.String(), token, nil)
	if status != http.StatusOK || detail["title"] != "Catalog Borrowed" {
		t.Fatalf("book detail: status=%d body=%#v", status, detail)
	}
	if _, exposed := detail["notes"]; exposed {
		t.Fatalf("book detail exposed private notes: %#v", detail)
	}

	for _, id := range []uuid.UUID{lostID, uuid.New()} {
		status, _ = bookRequest[map[string]any](t, env, http.MethodGet, "/api/books/"+id.String(), token, nil)
		if status != http.StatusNotFound {
			t.Errorf("GET /api/books/%s status = %d, want 404", id, status)
		}
	}
}

func responseBook(books []map[string]any, id uuid.UUID) map[string]any {
	for _, book := range books {
		if book["id"] == id.String() {
			return book
		}
	}
	return nil
}
