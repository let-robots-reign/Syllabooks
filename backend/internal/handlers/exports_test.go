package handlers

import (
	"bytes"
	"context"
	"encoding/csv"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestAdminCSVExports(t *testing.T) {
	env := newTestEnv(t, Server{})
	adminID, adminCode := env.newCodeUser(t)
	env.exec(t, "UPDATE users SET is_admin = true, display_name = 'Учитель' WHERE id = $1", adminID)
	adminToken := env.login(t, adminCode, "учитель")
	_, studentCode := env.newCodeUser(t)
	studentToken := env.login(t, studentCode, "ученик1")

	for _, path := range []string{
		"/api/admin/exports/books.csv",
		"/api/admin/exports/users.csv",
		"/api/admin/exports/loans.csv",
	} {
		for _, token := range []string{"", studentToken} {
			resp, _ := exportRequest(t, env, path, token)
			if resp.StatusCode != http.StatusNotFound {
				t.Fatalf("GET %s as non-admin = %d, want 404", path, resp.StatusCode)
			}
		}
	}

	createdAt := time.Date(2026, time.September, 17, 8, 9, 10, 0, time.UTC)
	isbn := randomTestISBN()
	var bookID uuid.UUID
	err := env.pool.QueryRow(t.Context(), `
		INSERT INTO books (
			isbn, title, author, level, page_count, description, cover_url, notes, created_at
		) VALUES ($1, '=SUM(1,1)', '  +Автор', 'yellow', 123, '-Описание', NULL, '@Заметка', $2)
		RETURNING id`, isbn, createdAt).Scan(&bookID)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := env.pool.Exec(context.Background(), "DELETE FROM books WHERE id = $1", bookID); err != nil {
			t.Errorf("delete export book: %v", err)
		}
	})

	var oauthUserID uuid.UUID
	err = env.pool.QueryRow(t.Context(), `
		INSERT INTO users (
			oauth_provider, oauth_subject, display_name, email, status, created_at
		) VALUES ('yandex', 'secret-oauth-subject', '  @Ученик', 'secret@example.test', 'pending', $1)
		RETURNING id`, createdAt.Add(time.Minute)).Scan(&oauthUserID)
	if err != nil {
		t.Fatal(err)
	}
	env.deleteUserAfter(t, oauthUserID)

	returnedAt := createdAt.Add(48 * time.Hour)
	var loanID uuid.UUID
	err = env.pool.QueryRow(t.Context(), `
		INSERT INTO loans (
			book_id, user_id, taken_at, due_at, returned_at, return_reason,
			shelf_scan_ok, book_scan_ok, created_by_admin, scan_reviewed_at
		) VALUES ($1, $2, $3, $4, $5, 'finished', true, false, true, $5)
		RETURNING id`, bookID, oauthUserID, createdAt.Add(time.Hour), createdAt.Add(22*24*time.Hour), returnedAt).Scan(&loanID)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := env.pool.Exec(context.Background(), "DELETE FROM loans WHERE id = $1", loanID); err != nil {
			t.Errorf("delete export loan: %v", err)
		}
	})

	bookResponse, bookRows := exportRequest(t, env, "/api/admin/exports/books.csv", adminToken)
	assertExportHeaders(t, bookResponse, "books")
	assertHeader(t, bookRows, bookExportHeader)
	bookRow := rowByID(t, bookRows, bookID.String())
	if want := []string{
		bookID.String(), isbn, "'=SUM(1,1)", "'  +Автор", "yellow", "123",
		"'-Описание", "", "false", "", "'@Заметка", csvTime(createdAt),
	}; !slices.Equal(bookRow, want) {
		t.Fatalf("book row = %#v, want %#v", bookRow, want)
	}

	userResponse, userRows := exportRequest(t, env, "/api/admin/exports/users.csv", adminToken)
	assertExportHeaders(t, userResponse, "users")
	assertHeader(t, userRows, userExportHeader)
	userRow := rowByID(t, userRows, oauthUserID.String())
	if want := []string{
		oauthUserID.String(), "'  @Ученик", "yandex", "", "pending", "false", csvTime(createdAt.Add(time.Minute)),
	}; !slices.Equal(userRow, want) {
		t.Fatalf("user row = %#v, want %#v", userRow, want)
	}
	userBody := rowsText(userRows)
	for _, secret := range []string{"password_hash", "oauth_subject", "email", "secret-oauth-subject", "secret@example.test"} {
		if strings.Contains(userBody, secret) {
			t.Errorf("users export contains excluded value %q", secret)
		}
	}

	loanResponse, loanRows := exportRequest(t, env, "/api/admin/exports/loans.csv", adminToken)
	assertExportHeaders(t, loanResponse, "loans")
	assertHeader(t, loanRows, loanExportHeader)
	loanRow := rowByID(t, loanRows, loanID.String())
	if want := []string{
		loanID.String(), bookID.String(), "'=SUM(1,1)", oauthUserID.String(), "'  @Ученик",
		csvTime(createdAt.Add(time.Hour)), csvTime(createdAt.Add(22 * 24 * time.Hour)), csvTime(returnedAt),
		"finished", "true", "false", "true", csvTime(returnedAt),
	}; !slices.Equal(loanRow, want) {
		t.Fatalf("loan row = %#v, want %#v", loanRow, want)
	}
}

func TestCSVExportWithNoRows(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/admin/exports/books.csv", nil)
	writeCSVDownload(recorder, request, "books", bookExportHeader, nil)

	response := recorder.Result()
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(body, []byte{0xEF, 0xBB, 0xBF}) {
		t.Fatal("CSV does not start with a UTF-8 BOM")
	}
	if !bytes.Contains(body, []byte("\r\n")) {
		t.Fatal("CSV does not use CRLF rows")
	}
	reader := csv.NewReader(bytes.NewReader(bytes.TrimPrefix(body, []byte{0xEF, 0xBB, 0xBF})))
	records, err := reader.ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || !slices.Equal(records[0], bookExportHeader) {
		t.Fatalf("empty export records = %#v", records)
	}
}

func TestSpreadsheetText(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{input: "plain", want: "plain"},
		{input: "", want: ""},
		{input: "=1+1", want: "'=1+1"},
		{input: "  +1", want: "'  +1"},
		{input: "@name", want: "'@name"},
		{input: "-value", want: "'-value"},
		{input: "123", want: "123"},
	}
	for _, test := range tests {
		if got := spreadsheetText(test.input); got != test.want {
			t.Errorf("spreadsheetText(%q) = %q, want %q", test.input, got, test.want)
		}
	}
}

func exportRequest(t *testing.T, env *testEnv, path, token string) (*http.Response, [][]string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, env.srv.URL+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	if token != "" {
		req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: token})
	}
	resp, err := env.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		return resp, nil
	}
	if !bytes.HasPrefix(body, []byte{0xEF, 0xBB, 0xBF}) {
		t.Fatalf("GET %s has no UTF-8 BOM", path)
	}
	if !bytes.Contains(body, []byte("\r\n")) {
		t.Fatalf("GET %s does not use CRLF rows", path)
	}
	reader := csv.NewReader(bytes.NewReader(bytes.TrimPrefix(body, []byte{0xEF, 0xBB, 0xBF})))
	records, err := reader.ReadAll()
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	return resp, records
}

func assertExportHeaders(t *testing.T, response *http.Response, name string) {
	t.Helper()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("%s export status = %d, want 200", name, response.StatusCode)
	}
	if got := response.Header.Get("Content-Type"); got != "text/csv; charset=utf-8" {
		t.Errorf("Content-Type = %q", got)
	}
	wantDisposition := `attachment; filename="syllabooks-` + name + `-` + time.Now().UTC().Format(time.DateOnly) + `.csv"`
	if got := response.Header.Get("Content-Disposition"); got != wantDisposition {
		t.Errorf("Content-Disposition = %q, want %q", got, wantDisposition)
	}
	if got := response.Header.Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q", got)
	}
}

func assertHeader(t *testing.T, rows [][]string, want []string) {
	t.Helper()
	if len(rows) == 0 || !slices.Equal(rows[0], want) {
		t.Fatalf("CSV header = %#v, want %#v", rows, want)
	}
}

func rowByID(t *testing.T, rows [][]string, id string) []string {
	t.Helper()
	for _, row := range rows[1:] {
		if len(row) > 0 && row[0] == id {
			return row
		}
	}
	t.Fatalf("CSV row %s not found", id)
	return nil
}

func rowsText(rows [][]string) string {
	var values []string
	for _, row := range rows {
		values = append(values, row...)
	}
	return strings.Join(values, "\n")
}
