package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"

	"syllabooks/db/gen"
)

type profileStatsResponse struct {
	FinishedBooks int32 `json:"finished_books"`
	PagesRead     int64 `json:"pages_read"`
}

type profileResponse struct {
	JoinedAt    time.Time            `json:"joined_at"`
	CurrentLoan *loanDetailResponse  `json:"current_loan"`
	History     []loanDetailResponse `json:"history"`
	Stats       profileStatsResponse `json:"stats"`
}

func (s *Server) profile(w http.ResponseWriter, r *http.Request, user gen.User) {
	queries := gen.New(s.Pool)
	response := profileResponse{
		JoinedAt: user.CreatedAt,
		History:  make([]loanDetailResponse, 0),
	}

	current, err := queries.GetCurrentLoanWithBook(r.Context(), user.ID)
	if err == nil {
		presented := presentCurrentLoan(current)
		response.CurrentLoan = &presented
	} else if !errors.Is(err, pgx.ErrNoRows) {
		serverError(w, r, fmt.Errorf("get profile current loan: %w", err))
		return
	}

	history, err := queries.ListProfileHistory(r.Context(), user.ID)
	if err != nil {
		serverError(w, r, fmt.Errorf("list profile history: %w", err))
		return
	}
	for _, loan := range history {
		response.History = append(response.History, presentProfileLoan(loan))
	}

	stats, err := queries.GetProfileStats(r.Context(), user.ID)
	if err != nil {
		serverError(w, r, fmt.Errorf("get profile stats: %w", err))
		return
	}
	response.Stats = profileStatsResponse{
		FinishedBooks: stats.FinishedBooks,
		PagesRead:     stats.PagesRead,
	}

	writeJSON(w, http.StatusOK, response)
}

func presentProfileLoan(loan gen.ListProfileHistoryRow) loanDetailResponse {
	return loanDetailResponse{
		ID: loan.ID, TakenAt: loan.TakenAt, DueAt: loan.DueAt,
		ReturnedAt: loan.ReturnedAt, ReturnReason: loan.ReturnReason,
		ShelfScanOK: loan.ShelfScanOk, BookScanOK: loan.BookScanOk,
		Book: presentLoanBook(loan.BookID, loan.BookIsbn, loan.BookTitle, loan.BookAuthor,
			loan.BookLevel, loan.BookPageCount, loan.BookDescription, loan.BookCoverUrl),
	}
}
