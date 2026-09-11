package db_test

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"syllabooks/db/gen"
)

// The loan rules are enforced by two partial unique indexes. These tests prove
// that the indexes fire, and that they only count loans that are still open.
//
// They need a migrated database in DATABASE_URL and leave nothing behind:
// each test runs inside a transaction that is rolled back.

func TestSecondOpenLoanOnBookFails(t *testing.T) {
	ctx, tx := begin(t)
	q := gen.New(tx)
	book := newBook(t, ctx, q, "0000000000001")
	anna, boris := newUser(t, ctx, q, "TEST0A"), newUser(t, ctx, q, "TEST0B")

	borrow(t, ctx, q, book, anna)
	_, err := q.CreateLoan(ctx, gen.CreateLoanParams{BookID: book, UserID: boris, LoanDays: 21})
	wantUniqueViolation(t, err, "loans_one_open_per_book")
}

func TestSecondOpenLoanForUserFails(t *testing.T) {
	ctx, tx := begin(t)
	q := gen.New(tx)
	first, second := newBook(t, ctx, q, "0000000000001"), newBook(t, ctx, q, "0000000000002")
	anna := newUser(t, ctx, q, "TEST0A")

	borrow(t, ctx, q, first, anna)
	_, err := q.CreateLoan(ctx, gen.CreateLoanParams{BookID: second, UserID: anna, LoanDays: 21})
	wantUniqueViolation(t, err, "loans_one_open_per_user")
}

func TestReturnedLoansDoNotBlock(t *testing.T) {
	ctx, tx := begin(t)
	q := gen.New(tx)
	first, second := newBook(t, ctx, q, "0000000000001"), newBook(t, ctx, q, "0000000000002")
	anna, boris := newUser(t, ctx, q, "TEST0A"), newUser(t, ctx, q, "TEST0B")

	loan := borrow(t, ctx, q, first, anna)
	_, err := tx.Exec(ctx,
		`UPDATE loans SET returned_at = now(), shelf_scan_ok = true, book_scan_ok = true WHERE id = $1`, loan)
	if err != nil {
		t.Fatalf("return: %v", err)
	}

	borrow(t, ctx, q, first, boris) // the book is free again
	borrow(t, ctx, q, second, anna) // and so is Anna
}

func begin(t *testing.T) (context.Context, pgx.Tx) {
	t.Helper()
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_URL is not set")
	}
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() {
		if err := conn.Close(ctx); err != nil {
			t.Errorf("close: %v", err)
		}
	})
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	t.Cleanup(func() {
		if err := tx.Rollback(ctx); err != nil {
			t.Errorf("rollback: %v", err)
		}
	})
	return ctx, tx
}

// newBook uses ISBNs that fail the EAN-13 checksum, so they can never collide
// with a real book's barcode.
func newBook(t *testing.T, ctx context.Context, q *gen.Queries, isbn string) uuid.UUID {
	t.Helper()
	b, err := q.CreateBook(ctx, gen.CreateBookParams{
		Isbn: &isbn, Title: "Test book " + isbn, Author: "Test", Level: gen.BookLevelGreen, PageCount: 100,
	})
	if err != nil {
		t.Fatalf("create book: %v", err)
	}
	return b.ID
}

// newUser uses codes containing 0, which real codes never do.
func newUser(t *testing.T, ctx context.Context, q *gen.Queries, code string) uuid.UUID {
	t.Helper()
	u, err := q.CreateCodeUser(ctx, gen.CreateCodeUserParams{DisplayName: "Test " + code, Code: code})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	return u.ID
}

func borrow(t *testing.T, ctx context.Context, q *gen.Queries, book, user uuid.UUID) uuid.UUID {
	t.Helper()
	loan, err := q.CreateLoan(ctx, gen.CreateLoanParams{BookID: book, UserID: user, LoanDays: 21})
	if err != nil {
		t.Fatalf("borrow: %v", err)
	}
	return loan.ID
}

func wantUniqueViolation(t *testing.T, err error, constraint string) {
	t.Helper()
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23505" || pgErr.ConstraintName != constraint {
		t.Fatalf("got %v, want unique violation on %s", err, constraint)
	}
}
