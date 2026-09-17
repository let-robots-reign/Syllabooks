package handlers

import (
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestAdminLoansLifecycle(t *testing.T) {
	env := newTestEnv(t, Server{})
	adminID, adminCode := env.newCodeUser(t)
	env.exec(t, "UPDATE users SET is_admin = true, display_name = 'Учитель' WHERE id = $1", adminID)
	adminToken := env.login(t, adminCode, "учитель")

	returnUserID, returnCode := env.newCodeUser(t)
	env.exec(t, "UPDATE users SET display_name = 'Аня' WHERE id = $1", returnUserID)
	returnToken := env.login(t, returnCode, "читатель")
	reviewUserID, _ := env.newCodeUser(t)
	env.exec(t, "UPDATE users SET display_name = 'Борис' WHERE id = $1", reviewUserID)
	lostUserID, lostCode := env.newCodeUser(t)
	env.exec(t, "UPDATE users SET display_name = 'Лиза' WHERE id = $1", lostUserID)
	lostToken := env.login(t, lostCode, "читатель")

	returnBook := newBorrowTestBook(t, env, randomTestISBN(), "Admin Return")
	reviewBook := newBorrowTestBook(t, env, randomTestISBN(), "Needs Review")
	lostBook := newBorrowTestBook(t, env, randomTestISBN(), "Lost Book")
	orphanLostBook := newBorrowTestBook(t, env, randomTestISBN(), "Lost Without History")
	afterReturnBook := newBorrowTestBook(t, env, randomTestISBN(), "After Return")
	afterLostBook := newBorrowTestBook(t, env, randomTestISBN(), "After Lost")

	overdueDue := time.Now().UTC().Add(-48 * time.Hour).Truncate(time.Microsecond)
	returnLoanID := insertAdminTestLoan(t, env, returnBook.ID, returnUserID, overdueDue)
	lostDue := time.Now().UTC().Add(5 * 24 * time.Hour).Truncate(time.Microsecond)
	lostLoanID := insertAdminTestLoan(t, env, lostBook.ID, lostUserID, lostDue)

	returnedAt := time.Now().UTC().Add(-2 * time.Hour).Truncate(time.Microsecond)
	reviewLoanID := uuid.New()
	env.exec(t, `INSERT INTO loans (
	    id, book_id, user_id, taken_at, due_at, returned_at, shelf_scan_ok, book_scan_ok,
	    created_by_admin
	) VALUES ($1, $2, $3, $4, $5, $6, false, true, true)`,
		reviewLoanID, reviewBook.ID, reviewUserID, returnedAt.Add(-22*24*time.Hour),
		returnedAt.Add(-24*time.Hour), returnedAt)
	env.exec(t, "UPDATE books SET is_lost = true, lost_at = $2 WHERE id = $1",
		orphanLostBook.ID, returnedAt)

	for _, token := range []string{"", returnToken} {
		status, _ := bookRequest[map[string]any](t, env, http.MethodGet, "/api/admin/loans", token, nil)
		if status != http.StatusNotFound {
			t.Fatalf("non-admin loans status = %d, want 404", status)
		}
	}
	status, _ := bookRequest[map[string]any](t, env, http.MethodPost, "/api/admin/loans/not-a-uuid/extend", adminToken, nil)
	if status != http.StatusNotFound {
		t.Fatalf("invalid loan id status = %d, want 404", status)
	}

	status, stats := bookRequest[adminStatsResponse](t, env, http.MethodGet, "/api/admin/stats", adminToken, nil)
	if status != http.StatusOK || stats.OpenLoans < 2 || stats.Books < 5 {
		t.Fatalf("admin stats: status=%d body=%+v", status, stats)
	}

	status, loans := bookRequest[adminLoansResponse](t, env, http.MethodGet, "/api/admin/loans", adminToken, nil)
	returnIndex, lostIndex := openLoanIndex(loans.OpenLoans, returnLoanID), openLoanIndex(loans.OpenLoans, lostLoanID)
	if status != http.StatusOK || returnIndex < 0 || lostIndex < 0 || returnIndex >= lostIndex {
		t.Fatalf("open loans are not overdue-first: status=%d body=%+v", status, loans.OpenLoans)
	}
	review := scanReview(loans.ReviewQueue, reviewLoanID)
	if review == nil || review.ShelfScanOK || !review.BookScanOK {
		t.Fatalf("review queue = %+v", loans.ReviewQueue)
	}

	status, extended := bookRequest[map[string]time.Time](t, env, http.MethodPost,
		"/api/admin/loans/"+returnLoanID.String()+"/extend", adminToken, nil)
	if status != http.StatusOK || !extended["due_at"].Equal(overdueDue.Add(7*24*time.Hour)) {
		t.Fatalf("extend: status=%d due=%v want=%v", status, extended["due_at"], overdueDue.Add(7*24*time.Hour))
	}

	status, _ = bookRequest[map[string]any](t, env, http.MethodPost,
		"/api/admin/loans/"+returnLoanID.String()+"/return", adminToken, nil)
	if status != http.StatusNoContent {
		t.Fatalf("admin return status = %d, want 204", status)
	}
	var returned *time.Time
	var shelfOK, bookOK *bool
	var byAdmin bool
	var reviewed *time.Time
	if err := env.pool.QueryRow(t.Context(), `SELECT returned_at, shelf_scan_ok, book_scan_ok,
        created_by_admin, scan_reviewed_at FROM loans WHERE id = $1`, returnLoanID).
		Scan(&returned, &shelfOK, &bookOK, &byAdmin, &reviewed); err != nil {
		t.Fatal(err)
	}
	if returned == nil || shelfOK == nil || *shelfOK || bookOK == nil || *bookOK || !byAdmin || reviewed == nil {
		t.Fatalf("admin return audit = returned:%v shelf:%v book:%v admin:%v reviewed:%v",
			returned, shelfOK, bookOK, byAdmin, reviewed)
	}
	status, borrowed := bookRequest[borrowResponse](t, env, http.MethodPost, "/api/loans", returnToken,
		map[string]string{"isbn": *afterReturnBook.Isbn})
	if status != http.StatusCreated || borrowed.Book.ID != afterReturnBook.ID {
		t.Fatalf("borrow after admin return: status=%d body=%+v", status, borrowed)
	}
	status, _ = bookRequest[map[string]any](t, env, http.MethodPost,
		"/api/admin/loans/"+returnLoanID.String()+"/extend", adminToken, nil)
	if status != http.StatusConflict {
		t.Fatalf("extend closed loan status = %d, want 409", status)
	}

	status, _ = bookRequest[map[string]any](t, env, http.MethodPost,
		"/api/admin/loans/"+reviewLoanID.String()+"/review", adminToken, nil)
	if status != http.StatusNoContent {
		t.Fatalf("review status = %d, want 204", status)
	}
	// A retry after a lost response is safe.
	status, _ = bookRequest[map[string]any](t, env, http.MethodPost,
		"/api/admin/loans/"+reviewLoanID.String()+"/review", adminToken, nil)
	if status != http.StatusNoContent {
		t.Fatalf("review retry status = %d, want 204", status)
	}
	status, loans = bookRequest[adminLoansResponse](t, env, http.MethodGet, "/api/admin/loans", adminToken, nil)
	if status != http.StatusOK || scanReview(loans.ReviewQueue, reviewLoanID) != nil {
		t.Fatalf("reviewed return remained queued: status=%d queue=%+v", status, loans.ReviewQueue)
	}

	status, _ = bookRequest[map[string]any](t, env, http.MethodPost,
		"/api/admin/loans/"+lostLoanID.String()+"/lost", adminToken, nil)
	if status != http.StatusNoContent {
		t.Fatalf("mark lost status = %d, want 204", status)
	}
	var isLost bool
	var lostAt *time.Time
	if err := env.pool.QueryRow(t.Context(), "SELECT is_lost, lost_at FROM books WHERE id = $1", lostBook.ID).
		Scan(&isLost, &lostAt); err != nil {
		t.Fatal(err)
	}
	if !isLost || lostAt == nil {
		t.Fatalf("lost state = %v/%v", isLost, lostAt)
	}
	status, _ = bookRequest[bookResponse](t, env, http.MethodGet,
		"/api/books/"+lostBook.ID.String(), lostToken, nil)
	if status != http.StatusNotFound {
		t.Fatalf("lost book catalogue status = %d, want 404", status)
	}
	status, lost := bookRequest[adminLostBooksResponse](t, env, http.MethodGet, "/api/admin/lost", adminToken, nil)
	if status != http.StatusOK || !containsLostBook(lost.Books, lostBook.ID, "Лиза") {
		t.Fatalf("lost registry: status=%d body=%+v", status, lost)
	}
	orphan := lostBookByID(lost.Books, orphanLostBook.ID)
	if orphan == nil || orphan.LastBorrowerName != "" || orphan.LastTakenAt != nil {
		t.Fatalf("lost book without loan history = %+v, want no borrower or taken date", orphan)
	}
	status, borrowed = bookRequest[borrowResponse](t, env, http.MethodPost, "/api/loans", lostToken,
		map[string]string{"isbn": *afterLostBook.Isbn})
	if status != http.StatusCreated || borrowed.Book.ID != afterLostBook.ID {
		t.Fatalf("borrow after lost closure: status=%d body=%+v", status, borrowed)
	}

	status, _ = bookRequest[map[string]any](t, env, http.MethodPost,
		"/api/admin/books/"+lostBook.ID.String()+"/found", adminToken, nil)
	if status != http.StatusNoContent {
		t.Fatalf("found status = %d, want 204", status)
	}
	status, _ = bookRequest[map[string]any](t, env, http.MethodPost,
		"/api/admin/books/"+lostBook.ID.String()+"/found", adminToken, nil)
	if status != http.StatusNoContent {
		t.Fatalf("found retry status = %d, want 204", status)
	}
	status, detail := bookRequest[bookResponse](t, env, http.MethodGet,
		"/api/books/"+lostBook.ID.String(), lostToken, nil)
	if status != http.StatusOK || detail.ID != lostBook.ID {
		t.Fatalf("found book did not return to catalogue: status=%d body=%+v", status, detail)
	}
}

func insertAdminTestLoan(t *testing.T, env *testEnv, bookID, userID uuid.UUID, dueAt time.Time) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := env.pool.QueryRow(t.Context(),
		"INSERT INTO loans (book_id, user_id, taken_at, due_at) VALUES ($1, $2, $3, $4) RETURNING id",
		bookID, userID, dueAt.Add(-21*24*time.Hour), dueAt).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func containsLostBook(books []adminLostBookResponse, id uuid.UUID, borrower string) bool {
	for _, book := range books {
		if book.Book.ID == id && book.LastBorrowerName == borrower {
			return true
		}
	}
	return false
}

func lostBookByID(books []adminLostBookResponse, id uuid.UUID) *adminLostBookResponse {
	for index := range books {
		if books[index].Book.ID == id {
			return &books[index]
		}
	}
	return nil
}

func openLoanIndex(loans []adminOpenLoanResponse, id uuid.UUID) int {
	for index, loan := range loans {
		if loan.ID == id {
			return index
		}
	}
	return -1
}

func scanReview(loans []adminScanReviewResponse, id uuid.UUID) *adminScanReviewResponse {
	for index := range loans {
		if loans[index].ID == id {
			return &loans[index]
		}
	}
	return nil
}
