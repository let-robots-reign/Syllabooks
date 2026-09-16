package handlers

import (
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestCurrentLoanAndReturnValidation(t *testing.T) {
	env := newTestEnv(t, Server{ShelfCode: "SHELF-403"})
	userID, code := env.newCodeUser(t)
	token := env.login(t, code, "читатель")
	_, otherCode := env.newCodeUser(t)
	otherToken := env.login(t, otherCode, "другой читатель")
	book := newBorrowTestBook(t, env, randomTestISBN(), "Coraline")

	status, created := bookRequest[borrowResponse](t, env, http.MethodPost, "/api/loans", token, map[string]string{"isbn": *book.Isbn})
	if status != http.StatusCreated {
		t.Fatalf("borrow status = %d, want 201", status)
	}

	status, _ = bookRequest[map[string]any](t, env, http.MethodGet, "/api/loans/current", "", nil)
	if status != http.StatusUnauthorized {
		t.Fatalf("anonymous current loan status = %d, want 401", status)
	}
	status, current := bookRequest[loanDetailResponse](t, env, http.MethodGet, "/api/loans/current", token, nil)
	if status != http.StatusOK || current.ID.String() != created.ID || current.Book.Title != "Coraline" {
		t.Fatalf("current loan: status=%d body=%+v", status, current)
	}
	status, noLoan := bookRequest[map[string]any](t, env, http.MethodGet, "/api/loans/current", otherToken, nil)
	if status != http.StatusNotFound || noLoan["code"] != "no_open_loan" {
		t.Fatalf("other current loan: status=%d body=%v", status, noLoan)
	}

	loanPath := "/api/loans/" + created.ID
	status, _ = bookRequest[map[string]any](t, env, http.MethodPost, loanPath+"/shelf-check", otherToken, map[string]string{"code": "SHELF-403"})
	if status != http.StatusNotFound {
		t.Fatalf("other user's shelf check status = %d, want 404", status)
	}
	status, invalidShelf := bookRequest[map[string]any](t, env, http.MethodPost, loanPath+"/shelf-check", token, map[string]string{"code": "WRONG"})
	if status != http.StatusUnprocessableEntity || invalidShelf["code"] != "invalid_shelf_code" {
		t.Fatalf("invalid shelf: status=%d body=%v", status, invalidShelf)
	}
	status, _ = bookRequest[map[string]any](t, env, http.MethodPost, loanPath+"/shelf-check", token, map[string]string{"code": "  SHELF-403  "})
	if status != http.StatusNoContent {
		t.Fatalf("valid shelf status = %d, want 204", status)
	}
	status, invalidReturnShelf := bookRequest[map[string]any](t, env, http.MethodPost, loanPath+"/return", token, returnBody(
		"scan", "WRONG", "scan", *book.Isbn,
	))
	if status != http.StatusUnprocessableEntity || invalidReturnShelf["code"] != "invalid_shelf_code" {
		t.Fatalf("invalid return shelf: status=%d body=%v", status, invalidReturnShelf)
	}

	status, wrongBook := bookRequest[map[string]any](t, env, http.MethodPost, loanPath+"/return", token, returnBody(
		"scan", "SHELF-403", "scan", randomTestISBN(),
	))
	if status != http.StatusUnprocessableEntity || wrongBook["code"] != "wrong_book" {
		t.Fatalf("wrong book: status=%d body=%v", status, wrongBook)
	}
	var returnedAt *time.Time
	if err := env.pool.QueryRow(t.Context(), "SELECT returned_at FROM loans WHERE id = $1", created.ID).Scan(&returnedAt); err != nil {
		t.Fatal(err)
	}
	if returnedAt != nil {
		t.Fatal("invalid evidence closed the loan")
	}

	status, _ = bookRequest[map[string]any](t, env, http.MethodPost, loanPath+"/return", otherToken, returnBody(
		"scan", "SHELF-403", "scan", *book.Isbn,
	))
	if status != http.StatusNotFound {
		t.Fatalf("other user's return status = %d, want 404", status)
	}

	var catalogue struct {
		MyLoan *loanDetailResponse `json:"my_loan"`
	}
	status, catalogue = bookRequest[struct {
		MyLoan *loanDetailResponse `json:"my_loan"`
	}](t, env, http.MethodGet, "/api/books", token, nil)
	if status != http.StatusOK || catalogue.MyLoan == nil || catalogue.MyLoan.ID != uuid.MustParse(created.ID) {
		t.Fatalf("catalogue my_loan: status=%d body=%+v user=%s", status, catalogue, userID)
	}
}

func TestReturnAuditFlagsAndIdempotency(t *testing.T) {
	tests := []struct {
		name                        string
		shelfMethod, bookMethod     string
		wantShelfScan, wantBookScan bool
	}{
		{"both scanned", "scan", "scan", true, true},
		{"both skipped", "skipped", "skipped", false, false},
		{"shelf scanned book skipped", "scan", "skipped", true, false},
		{"both manually entered", "manual", "manual", false, false},
		{"shelf manual book scanned", "manual", "scan", false, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			env := newTestEnv(t, Server{ShelfCode: "SHELF-403"})
			_, code := env.newCodeUser(t)
			token := env.login(t, code, "читатель")
			book := newBorrowTestBook(t, env, randomTestISBN(), "Return Flags")
			status, created := bookRequest[borrowResponse](t, env, http.MethodPost, "/api/loans", token, map[string]string{"isbn": *book.Isbn})
			if status != http.StatusCreated {
				t.Fatalf("borrow status = %d", status)
			}

			status, returned := bookRequest[loanDetailResponse](t, env, http.MethodPost, "/api/loans/"+created.ID+"/return", token, returnBody(
				tc.shelfMethod, evidenceValue(tc.shelfMethod, "SHELF-403"),
				tc.bookMethod, evidenceValue(tc.bookMethod, *book.Isbn),
			))
			if status != http.StatusOK || returned.ReturnedAt == nil || returned.ShelfScanOK == nil || returned.BookScanOK == nil {
				t.Fatalf("return: status=%d body=%+v", status, returned)
			}
			if *returned.ShelfScanOK != tc.wantShelfScan || *returned.BookScanOK != tc.wantBookScan {
				t.Fatalf("flags = %v/%v, want %v/%v", *returned.ShelfScanOK, *returned.BookScanOK, tc.wantShelfScan, tc.wantBookScan)
			}

			status, retried := bookRequest[loanDetailResponse](t, env, http.MethodPost, "/api/loans/"+created.ID+"/return", token, returnBody(
				"skipped", "", "skipped", "",
			))
			if status != http.StatusOK || retried.ReturnedAt == nil || !retried.ReturnedAt.Equal(*returned.ReturnedAt) ||
				*retried.ShelfScanOK != tc.wantShelfScan || *retried.BookScanOK != tc.wantBookScan {
				t.Fatalf("idempotent retry changed return: status=%d first=%+v retry=%+v", status, returned, retried)
			}

			status, _ = bookRequest[map[string]any](t, env, http.MethodGet, "/api/loans/current", token, nil)
			if status != http.StatusNotFound {
				t.Fatalf("current loan after return status = %d, want 404", status)
			}
			status, borrowedAgain := bookRequest[borrowResponse](t, env, http.MethodPost, "/api/loans", token, map[string]string{"isbn": *book.Isbn})
			if status != http.StatusCreated || borrowedAgain.Book.ID != book.ID {
				t.Fatalf("borrow after return: status=%d body=%+v", status, borrowedAgain)
			}
		})
	}
}

func TestReturnReasonIsValidatedAndIdempotent(t *testing.T) {
	env := newTestEnv(t, Server{ShelfCode: "SHELF-403"})
	_, code := env.newCodeUser(t)
	token := env.login(t, code, "читатель")
	book := newBorrowTestBook(t, env, randomTestISBN(), "Finished Book")
	status, created := bookRequest[borrowResponse](t, env, http.MethodPost, "/api/loans", token, map[string]string{"isbn": *book.Isbn})
	if status != http.StatusCreated {
		t.Fatalf("borrow status = %d", status)
	}
	path := "/api/loans/" + created.ID

	status, beforeReturn := bookRequest[map[string]any](t, env, http.MethodPost, path+"/return-reason", token, map[string]string{"reason": "finished"})
	if status != http.StatusConflict || beforeReturn["code"] != "loan_not_returned" {
		t.Fatalf("reason before return: status=%d body=%v", status, beforeReturn)
	}
	status, invalid := bookRequest[map[string]any](t, env, http.MethodPost, path+"/return-reason", token, map[string]string{"reason": "other"})
	if status != http.StatusUnprocessableEntity || invalid["code"] != "invalid_return_reason" {
		t.Fatalf("invalid reason: status=%d body=%v", status, invalid)
	}

	status, _ = bookRequest[loanDetailResponse](t, env, http.MethodPost, path+"/return", token, returnBody("scan", "SHELF-403", "scan", *book.Isbn))
	if status != http.StatusOK {
		t.Fatalf("return status = %d", status)
	}
	status, _ = bookRequest[map[string]any](t, env, http.MethodPost, path+"/return-reason", token, map[string]string{"reason": "finished"})
	if status != http.StatusNoContent {
		t.Fatalf("set reason status = %d, want 204", status)
	}
	status, _ = bookRequest[map[string]any](t, env, http.MethodPost, path+"/return-reason", token, map[string]string{"reason": "finished"})
	if status != http.StatusNoContent {
		t.Fatalf("same reason retry status = %d, want 204", status)
	}
	status, conflict := bookRequest[map[string]any](t, env, http.MethodPost, path+"/return-reason", token, map[string]string{"reason": "boring"})
	if status != http.StatusConflict || conflict["code"] != "return_reason_set" {
		t.Fatalf("different reason: status=%d body=%v", status, conflict)
	}

	var count int64
	if err := env.pool.QueryRow(t.Context(), "SELECT count(*) FROM loans WHERE id = $1 AND return_reason = 'finished'", created.ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("finished count for loan = %d, want 1", count)
	}
}

func returnBody(shelfMethod, shelfValue, bookMethod, bookValue string) map[string]any {
	return map[string]any{
		"shelf": map[string]string{"method": shelfMethod, "value": shelfValue},
		"book":  map[string]string{"method": bookMethod, "value": bookValue},
	}
}

func evidenceValue(method, value string) string {
	if method == "skipped" {
		return ""
	}
	return value
}
