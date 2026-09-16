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

type currentLoanResponse struct {
	BorrowerName string    `json:"borrower_name"`
	DueAt        time.Time `json:"due_at"`
}

type catalogBookResponse struct {
	ID          uuid.UUID            `json:"id"`
	ISBN        *string              `json:"isbn"`
	Title       string               `json:"title"`
	Author      string               `json:"author"`
	Level       gen.BookLevel        `json:"level"`
	PageCount   int32                `json:"page_count"`
	Description *string              `json:"description"`
	CoverURL    *string              `json:"cover_url"`
	CurrentLoan *currentLoanResponse `json:"current_loan"`
}

type catalogBookData struct {
	ID           uuid.UUID
	ISBN         *string
	Title        string
	Author       string
	Level        gen.BookLevel
	PageCount    int32
	Description  *string
	CoverURL     *string
	BorrowerName *string
	DueAt        *time.Time
}

func presentCatalogBook(book catalogBookData) catalogBookResponse {
	response := catalogBookResponse{
		ID: book.ID, ISBN: book.ISBN, Title: book.Title, Author: book.Author,
		Level: book.Level, PageCount: book.PageCount,
		Description: book.Description, CoverURL: book.CoverURL,
	}
	if book.BorrowerName != nil && book.DueAt != nil {
		response.CurrentLoan = &currentLoanResponse{
			BorrowerName: *book.BorrowerName,
			DueAt:        *book.DueAt,
		}
	}
	return response
}

func (s *Server) listCatalog(w http.ResponseWriter, r *http.Request, user gen.User) {
	queries := gen.New(s.Pool)
	books, err := queries.ListCatalogBooks(r.Context())
	if err != nil {
		serverError(w, r, fmt.Errorf("list catalogue: %w", err))
		return
	}
	finishedCount, err := queries.CountFinishedBooks(r.Context())
	if err != nil {
		serverError(w, r, fmt.Errorf("count finished books: %w", err))
		return
	}
	var myLoan *loanDetailResponse
	current, err := queries.GetCurrentLoanWithBook(r.Context(), user.ID)
	if err == nil {
		presented := presentCurrentLoan(current)
		myLoan = &presented
	} else if !errors.Is(err, pgx.ErrNoRows) {
		serverError(w, r, fmt.Errorf("get reader's current loan: %w", err))
		return
	}

	result := make([]catalogBookResponse, 0, len(books))
	for _, book := range books {
		result = append(result, presentCatalogBook(catalogBookData{
			ID: book.ID, ISBN: book.Isbn, Title: book.Title, Author: book.Author,
			Level: book.Level, PageCount: book.PageCount,
			Description: book.Description, CoverURL: book.CoverUrl,
			BorrowerName: book.BorrowerName, DueAt: book.DueAt,
		}))
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"finished_count": finishedCount,
		"books":          result,
		"my_loan":        myLoan,
	})
}

func (s *Server) getCatalogBook(w http.ResponseWriter, r *http.Request, _ gen.User) {
	id, ok := bookID(w, r)
	if !ok {
		return
	}
	book, err := gen.New(s.Pool).GetCatalogBook(r.Context(), id)
	if errors.Is(err, pgx.ErrNoRows) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		serverError(w, r, fmt.Errorf("get catalogue book: %w", err))
		return
	}
	writeJSON(w, http.StatusOK, presentCatalogBook(catalogBookData{
		ID: book.ID, ISBN: book.Isbn, Title: book.Title, Author: book.Author,
		Level: book.Level, PageCount: book.PageCount,
		Description: book.Description, CoverURL: book.CoverUrl,
		BorrowerName: book.BorrowerName, DueAt: book.DueAt,
	}))
}
