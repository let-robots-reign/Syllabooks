package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"syllabooks/db/gen"
)

type adminStatsResponse struct {
	OpenLoans int32 `json:"open_loans"`
	Books     int32 `json:"books"`
	LostBooks int32 `json:"lost_books"`
}

type adminLoanBookResponse struct {
	ID        uuid.UUID     `json:"id"`
	ISBN      *string       `json:"isbn"`
	Title     string        `json:"title"`
	Level     gen.BookLevel `json:"level"`
	PageCount int32         `json:"page_count"`
}

type adminLoanStudentResponse struct {
	ID          uuid.UUID `json:"id"`
	DisplayName string    `json:"display_name"`
}

type adminOpenLoanResponse struct {
	ID      uuid.UUID                `json:"id"`
	TakenAt time.Time                `json:"taken_at"`
	DueAt   time.Time                `json:"due_at"`
	Book    adminLoanBookResponse    `json:"book"`
	Student adminLoanStudentResponse `json:"student"`
}

type adminScanReviewResponse struct {
	ID          uuid.UUID                `json:"id"`
	TakenAt     time.Time                `json:"taken_at"`
	DueAt       time.Time                `json:"due_at"`
	ReturnedAt  time.Time                `json:"returned_at"`
	ShelfScanOK bool                     `json:"shelf_scan_ok"`
	BookScanOK  bool                     `json:"book_scan_ok"`
	Book        adminLoanBookResponse    `json:"book"`
	Student     adminLoanStudentResponse `json:"student"`
}

type adminLoansResponse struct {
	AsOf        time.Time                 `json:"as_of"`
	OpenLoans   []adminOpenLoanResponse   `json:"open_loans"`
	ReviewQueue []adminScanReviewResponse `json:"review_queue"`
}

type adminLostBookResponse struct {
	Book             adminLoanBookResponse `json:"book"`
	LostAt           time.Time             `json:"lost_at"`
	LastBorrowerName string                `json:"last_borrower_name"`
	LastTakenAt      *time.Time            `json:"last_taken_at"`
}

type adminLostBooksResponse struct {
	Books []adminLostBookResponse `json:"books"`
}

func (s *Server) adminStats(w http.ResponseWriter, r *http.Request, _ gen.User) {
	stats, err := gen.New(s.Pool).GetAdminStats(r.Context())
	if err != nil {
		serverError(w, r, fmt.Errorf("get admin stats: %w", err))
		return
	}
	writeJSON(w, http.StatusOK, adminStatsResponse{
		OpenLoans: stats.OpenLoans,
		Books:     stats.Books,
		LostBooks: stats.LostBooks,
	})
}

func (s *Server) listAdminLoans(w http.ResponseWriter, r *http.Request, _ gen.User) {
	queries := gen.New(s.Pool)
	open, err := queries.ListAdminOpenLoans(r.Context())
	if err != nil {
		serverError(w, r, fmt.Errorf("list admin open loans: %w", err))
		return
	}
	reviews, err := queries.ListAdminScanReviews(r.Context())
	if err != nil {
		serverError(w, r, fmt.Errorf("list admin scan reviews: %w", err))
		return
	}

	response := adminLoansResponse{
		AsOf:        time.Now().UTC(),
		OpenLoans:   make([]adminOpenLoanResponse, 0, len(open)),
		ReviewQueue: make([]adminScanReviewResponse, 0, len(reviews)),
	}
	for _, loan := range open {
		response.OpenLoans = append(response.OpenLoans, presentAdminOpenLoan(loan))
	}
	for _, loan := range reviews {
		if loan.ReturnedAt == nil || loan.ShelfScanOk == nil || loan.BookScanOk == nil {
			serverError(w, r, fmt.Errorf("scan review %s has incomplete return data", loan.ID))
			return
		}
		response.ReviewQueue = append(response.ReviewQueue, adminScanReviewResponse{
			ID: loan.ID, TakenAt: loan.TakenAt, DueAt: loan.DueAt,
			ReturnedAt: *loan.ReturnedAt, ShelfScanOK: *loan.ShelfScanOk, BookScanOK: *loan.BookScanOk,
			Book:    presentAdminLoanBook(loan.BookID, loan.BookIsbn, loan.BookTitle, loan.BookLevel, loan.BookPageCount),
			Student: adminLoanStudentResponse{ID: loan.StudentID, DisplayName: loan.StudentName},
		})
	}
	writeJSON(w, http.StatusOK, response)
}

func (s *Server) extendAdminLoan(w http.ResponseWriter, r *http.Request, _ gen.User) {
	id, ok := loanID(w, r)
	if !ok {
		return
	}
	dueAt, err := gen.New(s.Pool).ExtendAdminLoan(r.Context(), id)
	if errors.Is(err, pgx.ErrNoRows) {
		s.writeAdminLoanClosedOrMissing(w, r, id)
		return
	}
	if err != nil {
		serverError(w, r, fmt.Errorf("extend admin loan: %w", err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]time.Time{"due_at": dueAt})
}

func (s *Server) completeAdminReturn(w http.ResponseWriter, r *http.Request, _ gen.User) {
	id, ok := loanID(w, r)
	if !ok {
		return
	}
	_, err := gen.New(s.Pool).CompleteAdminReturn(r.Context(), id)
	if errors.Is(err, pgx.ErrNoRows) {
		s.writeAdminLoanClosedOrMissing(w, r, id)
		return
	}
	if err != nil {
		serverError(w, r, fmt.Errorf("complete admin return: %w", err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) markAdminLoanLost(w http.ResponseWriter, r *http.Request, _ gen.User) {
	id, ok := loanID(w, r)
	if !ok {
		return
	}
	tx, err := s.Pool.Begin(r.Context())
	if err != nil {
		serverError(w, r, fmt.Errorf("begin mark-lost transaction: %w", err))
		return
	}
	defer tx.Rollback(r.Context())

	queries := gen.New(tx)
	loan, err := queries.CompleteAdminReturn(r.Context(), id)
	if errors.Is(err, pgx.ErrNoRows) {
		s.writeAdminLoanClosedOrMissingWithQueries(w, r, queries, id)
		return
	}
	if err != nil {
		serverError(w, r, fmt.Errorf("close lost loan: %w", err))
		return
	}
	if _, err := queries.MarkAdminBookLost(r.Context(), loan.BookID); errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusConflict, "Книга уже отмечена как утерянная.")
		return
	} else if err != nil {
		serverError(w, r, fmt.Errorf("mark admin book lost: %w", err))
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		serverError(w, r, fmt.Errorf("commit mark-lost transaction: %w", err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) reviewAdminLoanScan(w http.ResponseWriter, r *http.Request, _ gen.User) {
	id, ok := loanID(w, r)
	if !ok {
		return
	}
	if _, err := gen.New(s.Pool).ReviewAdminLoanScan(r.Context(), id); errors.Is(err, pgx.ErrNoRows) {
		http.NotFound(w, r)
		return
	} else if err != nil {
		serverError(w, r, fmt.Errorf("review admin loan scan: %w", err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) listAdminLostBooks(w http.ResponseWriter, r *http.Request, _ gen.User) {
	books, err := gen.New(s.Pool).ListAdminLostBooks(r.Context())
	if err != nil {
		serverError(w, r, fmt.Errorf("list admin lost books: %w", err))
		return
	}
	response := adminLostBooksResponse{Books: make([]adminLostBookResponse, 0, len(books))}
	for _, book := range books {
		if book.LostAt == nil {
			serverError(w, r, fmt.Errorf("lost book %s has no lost date", book.ID))
			return
		}
		var lastTakenAt *time.Time
		if book.LastLoanID != uuid.Nil {
			lastTakenAt = &book.LastTakenAt
		}
		response.Books = append(response.Books, adminLostBookResponse{
			Book:   presentAdminLoanBook(book.ID, book.Isbn, book.Title, book.Level, book.PageCount),
			LostAt: *book.LostAt, LastBorrowerName: book.LastBorrowerName, LastTakenAt: lastTakenAt,
		})
	}
	writeJSON(w, http.StatusOK, response)
}

func (s *Server) markAdminBookFound(w http.ResponseWriter, r *http.Request, _ gen.User) {
	id, ok := bookID(w, r)
	if !ok {
		return
	}
	if _, err := gen.New(s.Pool).MarkAdminBookFound(r.Context(), id); errors.Is(err, pgx.ErrNoRows) {
		http.NotFound(w, r)
		return
	} else if err != nil {
		serverError(w, r, fmt.Errorf("mark admin book found: %w", err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) writeAdminLoanClosedOrMissing(w http.ResponseWriter, r *http.Request, id uuid.UUID) {
	s.writeAdminLoanClosedOrMissingWithQueries(w, r, gen.New(s.Pool), id)
}

func (s *Server) writeAdminLoanClosedOrMissingWithQueries(
	w http.ResponseWriter,
	r *http.Request,
	queries *gen.Queries,
	id uuid.UUID,
) {
	_, err := queries.GetAdminLoanState(r.Context(), id)
	if errors.Is(err, pgx.ErrNoRows) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		serverError(w, r, fmt.Errorf("resolve admin loan state: %w", err))
		return
	}
	writeError(w, http.StatusConflict, "Выдача уже закрыта.")
}

func presentAdminOpenLoan(loan gen.ListAdminOpenLoansRow) adminOpenLoanResponse {
	return adminOpenLoanResponse{
		ID: loan.ID, TakenAt: loan.TakenAt, DueAt: loan.DueAt,
		Book:    presentAdminLoanBook(loan.BookID, loan.BookIsbn, loan.BookTitle, loan.BookLevel, loan.BookPageCount),
		Student: adminLoanStudentResponse{ID: loan.StudentID, DisplayName: loan.StudentName},
	}
}

func presentAdminLoanBook(
	id uuid.UUID,
	isbn *string,
	title string,
	level gen.BookLevel,
	pageCount int32,
) adminLoanBookResponse {
	return adminLoanBookResponse{ID: id, ISBN: isbn, Title: title, Level: level, PageCount: pageCount}
}
