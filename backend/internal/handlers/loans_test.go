package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"syllabooks/db/gen"
)

func TestBorrowBookValidationAndAuthentication(t *testing.T) {
	env := newTestEnv(t, Server{})
	userID, code := env.newCodeUser(t)
	token := env.login(t, code, "читатель")

	missingISBN := randomTestISBN()
	status, _ := bookRequest[map[string]any](t, env, http.MethodPost, "/api/loans", "", map[string]string{"isbn": missingISBN})
	if status != http.StatusUnauthorized {
		t.Fatalf("anonymous borrow status = %d, want 401", status)
	}

	status, invalid := bookRequest[borrowErrorResponse](t, env, http.MethodPost, "/api/loans", token, map[string]string{"isbn": missingISBN[:12] + string('0'+(missingISBN[12]-'0'+1)%10)})
	if status != http.StatusBadRequest || invalid.Code != "invalid_isbn" {
		t.Fatalf("invalid ISBN: status=%d body=%+v", status, invalid)
	}

	status, missing := bookRequest[borrowErrorResponse](t, env, http.MethodPost, "/api/loans", token, map[string]string{"isbn": missingISBN})
	if status != http.StatusNotFound || missing.Code != "book_not_found" {
		t.Fatalf("missing ISBN: status=%d body=%+v", status, missing)
	}

	env.exec(t, "UPDATE users SET status = 'banned' WHERE id = $1", userID)
	status, _ = bookRequest[map[string]any](t, env, http.MethodPost, "/api/loans", token, map[string]string{"isbn": missingISBN})
	if status != http.StatusUnauthorized {
		t.Fatalf("banned borrow status = %d, want 401", status)
	}
}

func TestBorrowBookRejectsLostBook(t *testing.T) {
	env := newTestEnv(t, Server{})
	_, code := env.newCodeUser(t)
	token := env.login(t, code, "читатель")
	book := newBorrowTestBook(t, env, randomTestISBN(), "Lost Book")
	env.exec(t, "UPDATE books SET is_lost = true, lost_at = now() WHERE id = $1", book.ID)

	status, result := bookRequest[borrowErrorResponse](t, env, http.MethodPost, "/api/loans", token, map[string]string{"isbn": *book.Isbn})
	if status != http.StatusConflict || result.Code != "book_lost" || result.Book == nil || result.Book.ID != book.ID {
		t.Fatalf("lost book: status=%d body=%+v", status, result)
	}
}

func TestBorrowBookCreatesLoanAndRetryIsIdempotent(t *testing.T) {
	env := newTestEnv(t, Server{LoanDays: 14})
	userID, code := env.newCodeUser(t)
	env.exec(t, "UPDATE users SET display_name = 'Аня' WHERE id = $1", userID)
	token := env.login(t, code, "читатель")
	book := newBorrowTestBook(t, env, randomTestISBN(), "Matilda")

	status, created := bookRequest[borrowResponse](t, env, http.MethodPost, "/api/loans", token, map[string]string{"isbn": *book.Isbn})
	if status != http.StatusCreated || created.Book.ID != book.ID {
		t.Fatalf("create loan: status=%d body=%+v", status, created)
	}
	if got := created.DueAt.Sub(created.TakenAt); got != 14*24*time.Hour {
		t.Fatalf("loan period = %v, want 14 days", got)
	}

	status, retried := bookRequest[borrowResponse](t, env, http.MethodPost, "/api/loans", token, map[string]string{"isbn": *book.Isbn})
	if status != http.StatusOK || retried.ID != created.ID {
		t.Fatalf("retry: status=%d body=%+v, want original loan %s", status, retried, created.ID)
	}

	type catalogPayload struct {
		Books []catalogBookResponse `json:"books"`
	}
	status, catalog := bookRequest[catalogPayload](t, env, http.MethodGet, "/api/books", token, nil)
	if status != http.StatusOK {
		t.Fatalf("catalogue status = %d", status)
	}
	var found *catalogBookResponse
	for index := range catalog.Books {
		if catalog.Books[index].ID == book.ID {
			found = &catalog.Books[index]
			break
		}
	}
	if found == nil || found.CurrentLoan == nil || found.CurrentLoan.BorrowerName != "Аня" || !found.CurrentLoan.DueAt.Equal(created.DueAt) {
		t.Fatalf("catalogue loan = %+v, want borrower and due date", found)
	}
}

func TestBorrowBookConflicts(t *testing.T) {
	t.Run("user already has another book", func(t *testing.T) {
		env := newTestEnv(t, Server{})
		userID, code := env.newCodeUser(t)
		token := env.login(t, code, "читатель")
		first := newBorrowTestBook(t, env, randomTestISBN(), "First Book")
		second := newBorrowTestBook(t, env, randomTestISBN(), "Second Book")
		env.exec(t, `INSERT INTO loans (book_id, user_id, due_at) VALUES ($1, $2, now() + interval '21 days')`, first.ID, userID)

		status, result := bookRequest[borrowErrorResponse](t, env, http.MethodPost, "/api/loans", token, map[string]string{"isbn": *second.Isbn})
		if status != http.StatusConflict || result.Code != "loan_limit" || result.Book == nil || result.Book.ID != first.ID || result.DueAt == nil {
			t.Fatalf("loan limit: status=%d body=%+v", status, result)
		}
	})

	t.Run("book already belongs to another user", func(t *testing.T) {
		env := newTestEnv(t, Server{})
		borrowerID, _ := env.newCodeUser(t)
		_, viewerCode := env.newCodeUser(t)
		env.exec(t, "UPDATE users SET display_name = 'Борис' WHERE id = $1", borrowerID)
		viewerToken := env.login(t, viewerCode, "читатель")
		book := newBorrowTestBook(t, env, randomTestISBN(), "Shared Book")
		dueAt := time.Now().UTC().Add(21 * 24 * time.Hour).Truncate(time.Microsecond)
		env.exec(t, `INSERT INTO loans (book_id, user_id, due_at) VALUES ($1, $2, $3)`, book.ID, borrowerID, dueAt)

		status, result := bookRequest[borrowErrorResponse](t, env, http.MethodPost, "/api/loans", viewerToken, map[string]string{"isbn": *book.Isbn})
		if status != http.StatusConflict || result.Code != "book_unavailable" || result.Book == nil || result.Book.ID != book.ID || result.BorrowerName != "Борис" || result.DueAt == nil || !result.DueAt.Equal(dueAt) {
			t.Fatalf("book unavailable: status=%d body=%+v", status, result)
		}
	})
}

func TestConcurrentBorrowHasOneWinner(t *testing.T) {
	env := newTestEnv(t, Server{})
	_, firstCode := env.newCodeUser(t)
	_, secondCode := env.newCodeUser(t)
	firstToken := env.login(t, firstCode, "читатель1")
	secondToken := env.login(t, secondCode, "читатель2")
	book := newBorrowTestBook(t, env, randomTestISBN(), "Race Book")

	start := make(chan struct{})
	results := make(chan concurrentBorrowResult, 2)
	var group sync.WaitGroup
	for _, token := range []string{firstToken, secondToken} {
		group.Add(1)
		go func(token string) {
			defer group.Done()
			<-start
			results <- sendConcurrentBorrow(env, token, *book.Isbn)
		}(token)
	}
	close(start)
	group.Wait()
	close(results)

	created, conflicted := 0, 0
	for result := range results {
		if result.err != nil {
			t.Fatal(result.err)
		}
		switch result.status {
		case http.StatusCreated:
			created++
		case http.StatusConflict:
			if result.body["code"] != "book_unavailable" {
				t.Fatalf("conflict body = %#v", result.body)
			}
			conflicted++
		default:
			t.Fatalf("concurrent borrow: status=%d body=%#v", result.status, result.body)
		}
	}
	if created != 1 || conflicted != 1 {
		t.Fatalf("created=%d conflicted=%d, want one of each", created, conflicted)
	}
}

func TestConcurrentBorrowEnforcesUserLimit(t *testing.T) {
	env := newTestEnv(t, Server{})
	_, code := env.newCodeUser(t)
	token := env.login(t, code, "читатель")
	first := newBorrowTestBook(t, env, randomTestISBN(), "First Race Book")
	second := newBorrowTestBook(t, env, randomTestISBN(), "Second Race Book")

	start := make(chan struct{})
	results := make(chan concurrentBorrowResult, 2)
	var group sync.WaitGroup
	for _, isbn := range []string{*first.Isbn, *second.Isbn} {
		group.Add(1)
		go func(isbn string) {
			defer group.Done()
			<-start
			results <- sendConcurrentBorrow(env, token, isbn)
		}(isbn)
	}
	close(start)
	group.Wait()
	close(results)

	created, conflicted := 0, 0
	for result := range results {
		if result.err != nil {
			t.Fatal(result.err)
		}
		switch result.status {
		case http.StatusCreated:
			created++
		case http.StatusConflict:
			if result.body["code"] != "loan_limit" {
				t.Fatalf("conflict body = %#v", result.body)
			}
			conflicted++
		default:
			t.Fatalf("concurrent borrow: status=%d body=%#v", result.status, result.body)
		}
	}
	if created != 1 || conflicted != 1 {
		t.Fatalf("created=%d conflicted=%d, want one of each", created, conflicted)
	}
}

func newBorrowTestBook(t *testing.T, env *testEnv, isbn, title string) gen.Book {
	t.Helper()
	book, err := gen.New(env.pool).CreateBook(t.Context(), gen.CreateBookParams{
		Isbn: &isbn, Title: title, Author: "Test Author", Level: gen.BookLevelGreen,
		PageCount: 100,
	})
	if err != nil {
		t.Fatalf("create test book: %v", err)
	}
	t.Cleanup(func() {
		if _, err := env.pool.Exec(context.Background(), "DELETE FROM loans WHERE book_id = $1", book.ID); err != nil {
			t.Errorf("delete test loans: %v", err)
		}
		if _, err := env.pool.Exec(context.Background(), "DELETE FROM books WHERE id = $1", book.ID); err != nil {
			t.Errorf("delete test book: %v", err)
		}
	})
	return book
}

type concurrentBorrowResult struct {
	status int
	body   map[string]any
	err    error
}

func sendConcurrentBorrow(env *testEnv, token, isbn string) concurrentBorrowResult {
	body, err := json.Marshal(map[string]string{"isbn": isbn})
	if err != nil {
		return concurrentBorrowResult{err: err}
	}
	req, err := http.NewRequest(http.MethodPost, env.srv.URL+"/api/loans", bytes.NewReader(body))
	if err != nil {
		return concurrentBorrowResult{err: err}
	}
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: token})
	response, err := env.client.Do(req)
	if err != nil {
		return concurrentBorrowResult{err: err}
	}
	defer response.Body.Close()
	var result map[string]any
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		return concurrentBorrowResult{err: fmt.Errorf("decode status %d: %w", response.StatusCode, err)}
	}
	return concurrentBorrowResult{status: response.StatusCode, body: result}
}

func TestDefaultLoanPeriod(t *testing.T) {
	env := newTestEnv(t, Server{})
	_, code := env.newCodeUser(t)
	token := env.login(t, code, "читатель")
	book := newBorrowTestBook(t, env, randomTestISBN(), "Default Period")

	status, created := bookRequest[borrowResponse](t, env, http.MethodPost, "/api/loans", token, map[string]string{"isbn": *book.Isbn})
	if status != http.StatusCreated || created.DueAt.Sub(created.TakenAt) != 21*24*time.Hour {
		t.Fatalf("default loan: status=%d period=%v", status, created.DueAt.Sub(created.TakenAt))
	}
}

func TestBorrowInputRejectsUnknownFields(t *testing.T) {
	env := newTestEnv(t, Server{})
	_, code := env.newCodeUser(t)
	token := env.login(t, code, "читатель")
	status, _ := bookRequest[map[string]any](t, env, http.MethodPost, "/api/loans", token, map[string]string{
		"isbn": randomTestISBN(), "book_id": uuid.NewString(),
	})
	if status != http.StatusBadRequest {
		t.Fatalf("unknown field status = %d, want 400", status)
	}
}

func randomTestISBN() string {
	random := uuid.New()
	prefix := "979"
	for index := 0; index < 9; index++ {
		prefix += string('0' + random[index]%10)
	}
	sum := 0
	for index := range 12 {
		digit := int(prefix[index] - '0')
		if index%2 == 1 {
			digit *= 3
		}
		sum += digit
	}
	return prefix + string('0'+byte((10-sum%10)%10))
}
