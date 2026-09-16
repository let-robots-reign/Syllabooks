package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"syllabooks/db/gen"
)

const defaultLoanDays int32 = 21

type borrowInput struct {
	ISBN string `json:"isbn"`
}

type borrowResponse struct {
	ID      string       `json:"id"`
	TakenAt time.Time    `json:"taken_at"`
	DueAt   time.Time    `json:"due_at"`
	Book    bookResponse `json:"book"`
}

type borrowErrorResponse struct {
	Error        string        `json:"error"`
	Code         string        `json:"code"`
	Book         *bookResponse `json:"book,omitempty"`
	BorrowerName string        `json:"borrower_name,omitempty"`
	DueAt        *time.Time    `json:"due_at,omitempty"`
}

func (s *Server) borrowBook(w http.ResponseWriter, r *http.Request, user gen.User) {
	var input borrowInput
	if !decodeJSON(w, r, &input) {
		return
	}

	isbn, err := normalizeISBN(input.ISBN)
	if err != nil || isbn == nil {
		writeBorrowError(w, http.StatusBadRequest, "invalid_isbn",
			"Введи корректный ISBN из 13 цифр.", nil, "", nil)
		return
	}

	queries := gen.New(s.Pool)
	book, err := queries.GetBookByISBN(r.Context(), isbn)
	if errors.Is(err, pgx.ErrNoRows) {
		writeBorrowError(w, http.StatusNotFound, "book_not_found",
			"Такого ISBN нет в каталоге.", nil, "", nil)
		return
	}
	if err != nil {
		serverError(w, r, fmt.Errorf("find book by ISBN: %w", err))
		return
	}
	if book.IsLost {
		presented := presentBook(book)
		writeBorrowError(w, http.StatusConflict, "book_lost",
			"Эта книга отмечена как потерянная. Обратись к учителю.", &presented, "", nil)
		return
	}

	// This early lookup provides a useful conflict response. The database
	// indexes remain the authority when two requests race after this check.
	openLoan, err := queries.GetOpenLoanForUser(r.Context(), user.ID)
	if err == nil {
		if openLoan.BookID == book.ID {
			writeBorrowSuccess(w, http.StatusOK, openLoan, book)
			return
		}
		s.writeLoanLimit(w, r, queries, openLoan)
		return
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		serverError(w, r, fmt.Errorf("find user's open loan: %w", err))
		return
	}

	loanDays := s.LoanDays
	if loanDays <= 0 {
		loanDays = defaultLoanDays
	}
	loan, err := queries.CreateLoan(r.Context(), gen.CreateLoanParams{
		BookID: book.ID, UserID: user.ID, LoanDays: loanDays, CreatedByAdmin: false,
	})
	if err == nil {
		writeBorrowSuccess(w, http.StatusCreated, loan, book)
		return
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		switch pgErr.ConstraintName {
		case "loans_one_open_per_book":
			s.writeBookUnavailable(w, r, queries, book, user)
			return
		case "loans_one_open_per_user":
			current, lookupErr := queries.GetOpenLoanForUser(r.Context(), user.ID)
			if lookupErr != nil {
				serverError(w, r, fmt.Errorf("resolve open-loan conflict: %w", lookupErr))
				return
			}
			if current.BookID == book.ID {
				writeBorrowSuccess(w, http.StatusOK, current, book)
				return
			}
			s.writeLoanLimit(w, r, queries, current)
			return
		}
	}

	serverError(w, r, fmt.Errorf("create loan: %w", err))
}

func (s *Server) writeBookUnavailable(
	w http.ResponseWriter,
	r *http.Request,
	queries *gen.Queries,
	book gen.Book,
	user gen.User,
) {
	current, err := queries.GetOpenLoanForBook(r.Context(), book.ID)
	if err != nil {
		serverError(w, r, fmt.Errorf("resolve book-loan conflict: %w", err))
		return
	}
	if current.UserID == user.ID {
		writeBorrowSuccess(w, http.StatusOK, gen.Loan{
			ID: current.ID, BookID: current.BookID, UserID: current.UserID,
			TakenAt: current.TakenAt, DueAt: current.DueAt,
			ReturnedAt: current.ReturnedAt, ReturnReason: current.ReturnReason,
			ShelfScanOk: current.ShelfScanOk, BookScanOk: current.BookScanOk,
			CreatedByAdmin: current.CreatedByAdmin,
		}, book)
		return
	}
	presented := presentBook(book)
	writeBorrowError(w, http.StatusConflict, "book_unavailable",
		"Эту книгу уже взяли.", &presented, current.BorrowerName, &current.DueAt)
}

func (s *Server) writeLoanLimit(
	w http.ResponseWriter,
	r *http.Request,
	queries *gen.Queries,
	loan gen.Loan,
) {
	book, err := queries.GetBook(r.Context(), loan.BookID)
	if err != nil {
		serverError(w, r, fmt.Errorf("load user's open-loan book: %w", err))
		return
	}
	presented := presentBook(book)
	writeBorrowError(w, http.StatusConflict, "loan_limit",
		"Сначала верни книгу, которая у тебя на руках.", &presented, "", &loan.DueAt)
}

func writeBorrowSuccess(w http.ResponseWriter, status int, loan gen.Loan, book gen.Book) {
	writeJSON(w, status, borrowResponse{
		ID: loan.ID.String(), TakenAt: loan.TakenAt, DueAt: loan.DueAt, Book: presentBook(book),
	})
}

func writeBorrowError(
	w http.ResponseWriter,
	status int,
	code string,
	message string,
	book *bookResponse,
	borrowerName string,
	dueAt *time.Time,
) {
	writeJSON(w, status, borrowErrorResponse{
		Error: message, Code: code, Book: book, BorrowerName: borrowerName, DueAt: dueAt,
	})
}
