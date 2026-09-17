package handlers

import (
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"

	"syllabooks/db/gen"
)

func TestProfileRequiresAuthenticationAndHasEmptyState(t *testing.T) {
	env := newTestEnv(t, Server{})
	userID, code := env.newCodeUser(t)
	token := env.login(t, code, "читатель")

	status, _ := bookRequest[map[string]any](t, env, http.MethodGet, "/api/profile", "", nil)
	if status != http.StatusUnauthorized {
		t.Fatalf("anonymous profile status = %d, want 401", status)
	}

	status, profile := bookRequest[profileResponse](t, env, http.MethodGet, "/api/profile", token, nil)
	if status != http.StatusOK {
		t.Fatalf("empty profile status = %d, want 200", status)
	}
	if profile.CurrentLoan != nil || profile.History == nil || len(profile.History) != 0 {
		t.Fatalf("empty profile = %+v", profile)
	}
	if profile.Stats.FinishedBooks != 0 || profile.Stats.PagesRead != 0 {
		t.Fatalf("empty stats = %+v", profile.Stats)
	}
	var joinedAt time.Time
	if err := env.pool.QueryRow(t.Context(), "SELECT created_at FROM users WHERE id = $1", userID).Scan(&joinedAt); err != nil {
		t.Fatal(err)
	}
	if !profile.JoinedAt.Equal(joinedAt) {
		t.Fatalf("joined_at = %v, want %v", profile.JoinedAt, joinedAt)
	}
}

func TestProfileReturnsCurrentLoanOrderedPrivateHistoryAndFinishedStats(t *testing.T) {
	env := newTestEnv(t, Server{})
	userID, code := env.newCodeUser(t)
	token := env.login(t, code, "читатель")
	otherID, otherCode := env.newCodeUser(t)
	otherToken := env.login(t, otherCode, "другой читатель")

	currentBook := newBorrowTestBook(t, env, randomTestISBN(), "Current Profile Book")
	now := time.Now().UTC().Truncate(time.Microsecond)
	dueAt := now.Add(7 * 24 * time.Hour)
	env.exec(t, `INSERT INTO loans (book_id, user_id, taken_at, due_at)
		VALUES ($1, $2, $3, $4)`, currentBook.ID, userID, now.Add(-14*24*time.Hour), dueAt)

	type historyCase struct {
		title  string
		reason *gen.ReturnReason
	}
	reason := func(value gen.ReturnReason) *gen.ReturnReason { return &value }
	history := []historyCase{
		{"Finished One", reason(gen.ReturnReasonFinished)},
		{"Too Hard", reason(gen.ReturnReasonTooHard)},
		{"Boring", reason(gen.ReturnReasonBoring)},
		{"Skipped", reason(gen.ReturnReasonSkipped)},
		{"Pending Reason", nil},
		{"Finished Two", reason(gen.ReturnReasonFinished)},
	}
	returnedBase := now.Add(-10 * 24 * time.Hour)
	for index, item := range history {
		book := newBorrowTestBook(t, env, randomTestISBN(), item.title)
		returnedAt := returnedBase.Add(time.Duration(index) * time.Hour)
		env.exec(t, `INSERT INTO loans
			(book_id, user_id, taken_at, due_at, returned_at, return_reason, shelf_scan_ok, book_scan_ok)
			VALUES ($1, $2, $3, $4, $5, $6, true, true)`,
			book.ID, userID, returnedAt.Add(-14*24*time.Hour), returnedAt.Add(7*24*time.Hour), returnedAt, item.reason)
	}

	privateBook := newBorrowTestBook(t, env, randomTestISBN(), "Other Student Secret")
	otherReturnedAt := now.Add(-time.Hour)
	env.exec(t, `INSERT INTO loans
		(book_id, user_id, taken_at, due_at, returned_at, return_reason, shelf_scan_ok, book_scan_ok)
		VALUES ($1, $2, $3, $4, $5, 'finished', true, true)`,
		privateBook.ID, otherID, otherReturnedAt.Add(-7*24*time.Hour), otherReturnedAt.Add(14*24*time.Hour), otherReturnedAt)

	status, profile := bookRequest[profileResponse](t, env, http.MethodGet, "/api/profile", token, nil)
	if status != http.StatusOK || profile.CurrentLoan == nil {
		t.Fatalf("profile: status=%d body=%+v", status, profile)
	}
	if profile.CurrentLoan.Book.ID != currentBook.ID || !profile.CurrentLoan.DueAt.Equal(dueAt) {
		t.Fatalf("current loan = %+v", profile.CurrentLoan)
	}
	if len(profile.History) != len(history) {
		t.Fatalf("history length = %d, want %d", len(profile.History), len(history))
	}
	for index, loan := range profile.History {
		want := history[len(history)-1-index]
		if loan.Book.Title != want.title {
			t.Errorf("history[%d] title = %q, want %q", index, loan.Book.Title, want.title)
		}
		if (loan.ReturnReason == nil) != (want.reason == nil) ||
			(loan.ReturnReason != nil && *loan.ReturnReason != *want.reason) {
			t.Errorf("history[%d] reason = %v, want %v", index, loan.ReturnReason, want.reason)
		}
		if loan.Book.Title == privateBook.Title {
			t.Fatal("another student's loan appeared in profile history")
		}
	}
	if profile.Stats.FinishedBooks != 2 || profile.Stats.PagesRead != 200 {
		t.Fatalf("stats = %+v, want 2 books and 200 pages", profile.Stats)
	}

	status, otherProfile := bookRequest[profileResponse](t, env, http.MethodGet, "/api/profile", otherToken, nil)
	if status != http.StatusOK || len(otherProfile.History) != 1 ||
		otherProfile.History[0].Book.Title != privateBook.Title ||
		otherProfile.Stats.FinishedBooks != 1 || otherProfile.Stats.PagesRead != 100 {
		t.Fatalf("other profile: status=%d body=%+v", status, otherProfile)
	}
}

func TestProfileHistoryUsesLoanIDAsStableTieBreaker(t *testing.T) {
	env := newTestEnv(t, Server{})
	userID, code := env.newCodeUser(t)
	token := env.login(t, code, "читатель")
	returnedAt := time.Now().UTC().Truncate(time.Microsecond)
	firstID := uuid.MustParse("10000000-0000-0000-0000-000000000000")
	secondID := uuid.MustParse("20000000-0000-0000-0000-000000000000")

	for _, item := range []struct {
		id    uuid.UUID
		title string
	}{
		{firstID, "First Stable History Book"},
		{secondID, "Second Stable History Book"},
	} {
		book := newBorrowTestBook(t, env, randomTestISBN(), item.title)
		env.exec(t, `INSERT INTO loans
			(id, book_id, user_id, taken_at, due_at, returned_at, return_reason, shelf_scan_ok, book_scan_ok)
			VALUES ($1, $2, $3, $4, $5, $6, 'skipped', true, true)`,
			item.id, book.ID, userID, returnedAt.Add(-time.Hour), returnedAt.Add(time.Hour), returnedAt)
	}

	status, profile := bookRequest[profileResponse](t, env, http.MethodGet, "/api/profile", token, nil)
	if status != http.StatusOK || len(profile.History) != 2 ||
		profile.History[0].ID != secondID || profile.History[1].ID != firstID {
		t.Fatalf("stable history order: status=%d history=%+v", status, profile.History)
	}
}
