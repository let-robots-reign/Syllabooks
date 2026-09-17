package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"syllabooks/db/gen"
)

type loanDetailResponse struct {
	ID           uuid.UUID         `json:"id"`
	TakenAt      time.Time         `json:"taken_at"`
	DueAt        time.Time         `json:"due_at"`
	ReturnedAt   *time.Time        `json:"returned_at"`
	ReturnReason *gen.ReturnReason `json:"return_reason"`
	ShelfScanOK  *bool             `json:"shelf_scan_ok"`
	BookScanOK   *bool             `json:"book_scan_ok"`
	Book         bookResponse      `json:"book"`
}

type returnEvidenceInput struct {
	Method string `json:"method"`
	Value  string `json:"value"`
}

type returnInput struct {
	Shelf returnEvidenceInput `json:"shelf"`
	Book  returnEvidenceInput `json:"book"`
}

type shelfCodeInput struct {
	Code string `json:"code"`
}

type returnReasonInput struct {
	Reason gen.ReturnReason `json:"reason"`
}

type finishCelebrationResponse struct {
	ClassFinishedCount int64 `json:"class_finished_count"`
	IsFirstBook        bool  `json:"is_first_book"`
}

type returnReasonResponse struct {
	Loan        loanDetailResponse         `json:"loan"`
	Celebration *finishCelebrationResponse `json:"celebration"`
}

func (s *Server) currentLoan(w http.ResponseWriter, r *http.Request, user gen.User) {
	loan, err := gen.New(s.Pool).GetCurrentLoanWithBook(r.Context(), user.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		writeReturnError(w, http.StatusNotFound, "no_open_loan", "У тебя сейчас нет книги на руках.")
		return
	}
	if err != nil {
		serverError(w, r, fmt.Errorf("get current loan: %w", err))
		return
	}
	writeJSON(w, http.StatusOK, presentCurrentLoan(loan))
}

func (s *Server) checkShelfCode(w http.ResponseWriter, r *http.Request, user gen.User) {
	id, ok := loanID(w, r)
	if !ok {
		return
	}
	var input shelfCodeInput
	if !decodeJSON(w, r, &input) {
		return
	}

	loan, err := gen.New(s.Pool).GetLoanWithBookForUser(r.Context(), gen.GetLoanWithBookForUserParams{
		ID: id, UserID: user.ID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		serverError(w, r, fmt.Errorf("get loan for shelf check: %w", err))
		return
	}
	if loan.ReturnedAt != nil {
		writeReturnError(w, http.StatusConflict, "already_returned", "Эта книга уже возвращена.")
		return
	}
	if !s.validShelfCode(input.Code) {
		writeReturnError(w, http.StatusUnprocessableEntity, "invalid_shelf_code", "Это не код полки. Попробуй ещё раз.")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) returnBook(w http.ResponseWriter, r *http.Request, user gen.User) {
	id, ok := loanID(w, r)
	if !ok {
		return
	}
	var input returnInput
	if !decodeJSON(w, r, &input) {
		return
	}

	queries := gen.New(s.Pool)
	loan, err := queries.GetLoanWithBookForUser(r.Context(), gen.GetLoanWithBookForUserParams{
		ID: id, UserID: user.ID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		serverError(w, r, fmt.Errorf("get loan for return: %w", err))
		return
	}
	if loan.ReturnedAt != nil {
		writeJSON(w, http.StatusOK, presentOwnedLoan(loan))
		return
	}

	shelfOK, validation := s.validateShelfEvidence(input.Shelf)
	if validation != nil {
		writeReturnError(w, validation.status, validation.code, validation.message)
		return
	}
	bookOK, validation := validateBookEvidence(input.Book, loan.BookIsbn)
	if validation != nil {
		writeReturnError(w, validation.status, validation.code, validation.message)
		return
	}

	returned, err := queries.CompleteReturn(r.Context(), gen.CompleteReturnParams{
		ID: id, UserID: user.ID, ShelfScanOk: &shelfOK, BookScanOk: &bookOK,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		// Another identical request can win between the read and update. Reloading
		// gives the caller the completed return instead of a misleading failure.
		loan, err = queries.GetLoanWithBookForUser(r.Context(), gen.GetLoanWithBookForUserParams{
			ID: id, UserID: user.ID,
		})
		if err == nil && loan.ReturnedAt != nil {
			writeJSON(w, http.StatusOK, presentOwnedLoan(loan))
			return
		}
	}
	if err != nil {
		serverError(w, r, fmt.Errorf("complete return: %w", err))
		return
	}

	response := presentOwnedLoan(loan)
	response.ReturnedAt = returned.ReturnedAt
	response.ReturnReason = returned.ReturnReason
	response.ShelfScanOK = returned.ShelfScanOk
	response.BookScanOK = returned.BookScanOk
	writeJSON(w, http.StatusOK, response)
}

func (s *Server) setReturnReason(w http.ResponseWriter, r *http.Request, user gen.User) {
	id, ok := loanID(w, r)
	if !ok {
		return
	}
	var input returnReasonInput
	if !decodeJSON(w, r, &input) {
		return
	}
	if !input.Reason.Valid() {
		writeReturnError(w, http.StatusUnprocessableEntity, "invalid_return_reason", "Выбери причину возврата.")
		return
	}

	queries := gen.New(s.Pool)
	_, err := queries.SetReturnReason(r.Context(), gen.SetReturnReasonParams{
		ID: id, UserID: user.ID, ReturnReason: &input.Reason,
	})
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		serverError(w, r, fmt.Errorf("set return reason: %w", err))
		return
	}

	loan, lookupErr := queries.GetLoanWithBookForUser(r.Context(), gen.GetLoanWithBookForUserParams{
		ID: id, UserID: user.ID,
	})
	if errors.Is(lookupErr, pgx.ErrNoRows) {
		http.NotFound(w, r)
		return
	}
	if lookupErr != nil {
		serverError(w, r, fmt.Errorf("resolve return reason conflict: %w", lookupErr))
		return
	}
	if loan.ReturnedAt == nil {
		writeReturnError(w, http.StatusConflict, "loan_not_returned", "Сначала верни книгу.")
		return
	}
	if errors.Is(err, pgx.ErrNoRows) && (loan.ReturnReason == nil || *loan.ReturnReason != input.Reason) {
		writeReturnError(w, http.StatusConflict, "return_reason_set", "Причина возврата уже сохранена.")
		return
	}

	response := returnReasonResponse{Loan: presentOwnedLoan(loan)}
	if input.Reason == gen.ReturnReasonFinished {
		classFinished, countErr := queries.CountFinishedBooks(r.Context())
		if countErr != nil {
			serverError(w, r, fmt.Errorf("count class finished books: %w", countErr))
			return
		}
		userFinished, countErr := queries.CountFinishedBooksForUser(r.Context(), user.ID)
		if countErr != nil {
			serverError(w, r, fmt.Errorf("count user's finished books: %w", countErr))
			return
		}
		response.Celebration = &finishCelebrationResponse{
			ClassFinishedCount: classFinished,
			IsFirstBook:        userFinished == 1,
		}
	}
	writeJSON(w, http.StatusOK, response)
}

type evidenceError struct {
	status  int
	code    string
	message string
}

func (s *Server) validateShelfEvidence(input returnEvidenceInput) (bool, *evidenceError) {
	switch input.Method {
	case "scan", "manual":
		if !s.validShelfCode(input.Value) {
			return false, &evidenceError{http.StatusUnprocessableEntity, "invalid_shelf_code", "Это не код полки. Попробуй ещё раз."}
		}
		return input.Method == "scan", nil
	case "skipped":
		return false, nil
	default:
		return false, &evidenceError{http.StatusBadRequest, "invalid_return_evidence", "Некорректный способ возврата."}
	}
}

func validateBookEvidence(input returnEvidenceInput, expected *string) (bool, *evidenceError) {
	switch input.Method {
	case "scan", "manual":
		isbn, err := normalizeISBN(input.Value)
		if err != nil || isbn == nil || expected == nil || *isbn != *expected {
			return false, &evidenceError{http.StatusUnprocessableEntity, "wrong_book", "Это штрих-код другой книги."}
		}
		return input.Method == "scan", nil
	case "skipped":
		return false, nil
	default:
		return false, &evidenceError{http.StatusBadRequest, "invalid_return_evidence", "Некорректный способ возврата."}
	}
}

func (s *Server) validShelfCode(value string) bool {
	return strings.TrimSpace(value) != "" && strings.TrimSpace(value) == strings.TrimSpace(s.ShelfCode)
}

func presentCurrentLoan(loan gen.GetCurrentLoanWithBookRow) loanDetailResponse {
	return loanDetailResponse{
		ID: loan.ID, TakenAt: loan.TakenAt, DueAt: loan.DueAt,
		ReturnedAt: loan.ReturnedAt, ReturnReason: loan.ReturnReason,
		ShelfScanOK: loan.ShelfScanOk, BookScanOK: loan.BookScanOk,
		Book: presentLoanBook(loan.BookID, loan.BookIsbn, loan.BookTitle, loan.BookAuthor,
			loan.BookLevel, loan.BookPageCount, loan.BookDescription, loan.BookCoverUrl),
	}
}

func presentOwnedLoan(loan gen.GetLoanWithBookForUserRow) loanDetailResponse {
	return loanDetailResponse{
		ID: loan.ID, TakenAt: loan.TakenAt, DueAt: loan.DueAt,
		ReturnedAt: loan.ReturnedAt, ReturnReason: loan.ReturnReason,
		ShelfScanOK: loan.ShelfScanOk, BookScanOK: loan.BookScanOk,
		Book: presentLoanBook(loan.BookID, loan.BookIsbn, loan.BookTitle, loan.BookAuthor,
			loan.BookLevel, loan.BookPageCount, loan.BookDescription, loan.BookCoverUrl),
	}
}

func presentLoanBook(
	id uuid.UUID,
	isbn *string,
	title, author string,
	level gen.BookLevel,
	pageCount int32,
	description, coverURL *string,
) bookResponse {
	return bookResponse{
		ID: id, ISBN: isbn, Title: title, Author: author, Level: level,
		PageCount: pageCount, Description: description, CoverURL: coverURL,
	}
}

func loanID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.NotFound(w, r)
		return uuid.Nil, false
	}
	return id, true
}

func writeReturnError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]string{"error": message, "code": code})
}
