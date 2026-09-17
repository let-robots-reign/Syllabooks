package handlers

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"syllabooks/db/gen"
)

func TestStudentCodeGeneration(t *testing.T) {
	for range 100 {
		code, err := newStudentCode()
		if err != nil {
			t.Fatal(err)
		}
		if len(code) != studentCodeLength {
			t.Fatalf("code length = %d, want %d", len(code), studentCodeLength)
		}
		for _, char := range code {
			if !strings.ContainsRune(studentCodeAlphabet, char) {
				t.Fatalf("code %q contains disallowed character %q", code, char)
			}
		}
	}

	for _, code := range []string{"FUK234", "23HUY4", "23467O", "SHORT"} {
		if studentCodeIsClean(code) {
			t.Errorf("studentCodeIsClean(%q) = true, want false", code)
		}
	}
}

func TestAdminUsersLifecycle(t *testing.T) {
	codes := []string{"234678", "234679", "23467A"}
	codeIndex := 0
	env := newTestEnv(t, Server{StudentCodeGenerator: func() (string, error) {
		if codeIndex >= len(codes) {
			return "", errors.New("test code sequence exhausted")
		}
		code := codes[codeIndex]
		codeIndex++
		return code, nil
	}})

	adminID, adminCode := env.newCodeUser(t)
	env.exec(t, "UPDATE users SET is_admin = true, display_name = 'Учитель' WHERE id = $1", adminID)
	admin := env.login(t, adminCode, "учитель")
	studentID, studentCode := env.newCodeUser(t)
	studentSession := env.login(t, studentCode, "читатель")

	for _, token := range []string{"", studentSession} {
		status, _ := bookRequest[map[string]any](t, env, http.MethodGet, "/api/admin/users", token, nil)
		if status != http.StatusNotFound {
			t.Fatalf("non-admin users status = %d, want 404", status)
		}
	}

	// The first generated value is already reserved, so creation must retry.
	env.exec(t, "INSERT INTO issued_student_codes (code) VALUES ($1) ON CONFLICT DO NOTHING", codes[0])
	t.Cleanup(func() {
		for _, code := range codes {
			if _, err := env.pool.Exec(context.Background(), "DELETE FROM issued_student_codes WHERE code = $1", code); err != nil {
				t.Errorf("delete issued code %s: %v", code, err)
			}
		}
	})
	status, created := bookRequest[issuedCodeResponse](t, env, http.MethodPost, "/api/admin/users", admin,
		map[string]string{"display_name": "  Аня Королёва  "})
	if status != http.StatusCreated || created.Code != codes[1] || created.DisplayName != "Аня Королёва" {
		t.Fatalf("create admin user: status=%d body=%+v", status, created)
	}
	env.deleteUserAfter(t, created.ID)
	var passwordHash *string
	if err := env.pool.QueryRow(t.Context(), "SELECT password_hash FROM users WHERE id = $1", created.ID).Scan(&passwordHash); err != nil {
		t.Fatal(err)
	}
	if passwordHash != nil {
		t.Fatal("new code user has a password")
	}

	createdSession := env.login(t, created.Code, "кошка1")
	book := newBorrowTestBook(t, env, randomTestISBN(), "Current Admin User Book")
	dueAt := time.Now().UTC().Add(-24 * time.Hour).Truncate(time.Microsecond)
	env.exec(t, "INSERT INTO loans (book_id, user_id, taken_at, due_at) VALUES ($1, $2, $3, $4)",
		book.ID, created.ID, dueAt.Add(-21*24*time.Hour), dueAt)
	finishedBook := newBorrowTestBook(t, env, randomTestISBN(), "Finished Admin User Book")
	abandonedBook := newBorrowTestBook(t, env, randomTestISBN(), "Abandoned Admin User Book")
	returnedAt := time.Now().UTC().Add(-48 * time.Hour)
	env.exec(t, `INSERT INTO loans (book_id, user_id, taken_at, due_at, returned_at, return_reason, shelf_scan_ok, book_scan_ok)
		VALUES ($1, $2, $3, $4, $5, 'finished', true, true)`, finishedBook.ID, created.ID,
		returnedAt.Add(-21*24*time.Hour), returnedAt.Add(-time.Hour), returnedAt)
	env.exec(t, `INSERT INTO loans (book_id, user_id, taken_at, due_at, returned_at, return_reason, shelf_scan_ok, book_scan_ok)
		VALUES ($1, $2, $3, $4, $5, 'too_hard', true, true)`, abandonedBook.ID, created.ID,
		returnedAt.Add(-21*24*time.Hour), returnedAt.Add(-time.Hour), returnedAt)

	status, listed := bookRequest[adminUsersResponse](t, env, http.MethodGet, "/api/admin/users", admin, nil)
	user := adminUserByID(listed.Users, created.ID)
	if status != http.StatusOK || user == nil {
		t.Fatalf("list admin users: status=%d user=%+v", status, user)
	}
	if adminUserByID(listed.Users, adminID) != nil {
		t.Fatal("admin account appeared in students list")
	}
	if listed.Users[0].ID != created.ID {
		t.Fatalf("first user = %s, want newest %s", listed.Users[0].ID, created.ID)
	}
	if user.CurrentLoan == nil || user.CurrentLoan.Book.ID != book.ID || !user.CurrentLoan.DueAt.Equal(dueAt) ||
		user.FinishedCount != 1 || user.AbandonedCount != 1 {
		t.Fatalf("listed user = %+v", user)
	}
	if listed.Summary.ReadingNow < 1 || listed.Summary.FinishedBooks < 1 {
		t.Fatalf("summary = %+v", listed.Summary)
	}

	status, renamed := bookRequest[adminUserNameResponse](t, env, http.MethodPatch,
		"/api/admin/users/"+created.ID.String(), admin, map[string]string{"display_name": "  Анна К.  "})
	if status != http.StatusOK || renamed.DisplayName != "Анна К." {
		t.Fatalf("rename: status=%d body=%+v", status, renamed)
	}

	status, reissued := bookRequest[issuedCodeResponse](t, env, http.MethodPost,
		"/api/admin/users/"+created.ID.String()+"/reissue-code", admin, nil)
	if status != http.StatusOK || reissued.Code != codes[2] {
		t.Fatalf("reissue: status=%d body=%+v", status, reissued)
	}
	env.request(t, http.MethodPost, "/api/auth/code/check", "", map[string]string{"code": created.Code}, http.StatusNotFound)
	env.request(t, http.MethodGet, "/api/me", createdSession, nil, http.StatusOK)
	secondSession := env.login(t, reissued.Code, "кошка1")

	resetPath := "/api/admin/users/" + created.ID.String() + "/reset-password"
	env.request(t, http.MethodPost, resetPath, admin, nil, http.StatusNoContent)
	env.request(t, http.MethodGet, "/api/me", createdSession, nil, http.StatusUnauthorized)
	env.request(t, http.MethodGet, "/api/me", secondSession, nil, http.StatusUnauthorized)
	check := env.request(t, http.MethodPost, "/api/auth/code/check", "", map[string]string{"code": reissued.Code}, http.StatusOK)
	if check["has_password"] != false {
		t.Fatalf("has_password after reset = %v", check["has_password"])
	}

	bannedSession := env.login(t, reissued.Code, "собака1")
	env.request(t, http.MethodPost, "/api/admin/users/"+created.ID.String()+"/ban", admin, nil, http.StatusNoContent)
	env.request(t, http.MethodGet, "/api/me", bannedSession, nil, http.StatusUnauthorized)
	env.request(t, http.MethodPost, "/api/auth/code/login", "",
		map[string]string{"code": reissued.Code, "password": "собака1"}, http.StatusForbidden)
	var loanCount int
	if err := env.pool.QueryRow(t.Context(), "SELECT count(*) FROM loans WHERE user_id = $1", created.ID).Scan(&loanCount); err != nil {
		t.Fatal(err)
	}
	if loanCount != 3 {
		t.Fatalf("loans after ban = %d, want 3", loanCount)
	}

	oauth, err := gen.New(env.pool).UpsertOAuthUser(t.Context(), gen.UpsertOAuthUserParams{
		OauthProvider: "yandex", OauthSubject: uuid.NewString(), DisplayName: "OAuth Student",
	})
	if err != nil {
		t.Fatal(err)
	}
	env.deleteUserAfter(t, oauth.ID)
	for _, suffix := range []string{"reissue-code", "reset-password"} {
		status, _ := bookRequest[map[string]any](t, env, http.MethodPost,
			"/api/admin/users/"+oauth.ID.String()+"/"+suffix, admin, nil)
		if status != http.StatusConflict {
			t.Fatalf("OAuth %s status = %d, want 409", suffix, status)
		}
	}

	for _, id := range []string{"not-a-uuid", uuid.NewString()} {
		status, _ := bookRequest[map[string]any](t, env, http.MethodPost,
			"/api/admin/users/"+id+"/ban", admin, nil)
		if status != http.StatusNotFound {
			t.Fatalf("ban %s status = %d, want 404", id, status)
		}
	}
	status, _ = bookRequest[map[string]any](t, env, http.MethodPatch,
		"/api/admin/users/"+studentID.String(), admin, map[string]string{"display_name": " "})
	if status != http.StatusBadRequest {
		t.Fatalf("empty name status = %d, want 400", status)
	}
}

func adminUserByID(users []adminUserResponse, id uuid.UUID) *adminUserResponse {
	for index := range users {
		if users[index].ID == id {
			return &users[index]
		}
	}
	return nil
}
