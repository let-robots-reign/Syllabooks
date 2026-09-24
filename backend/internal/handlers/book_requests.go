package handlers

import (
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"syllabooks/db/gen"
)

const maxBookRequestTitle = 200

type bookRequestResponse struct {
	ID          uuid.UUID                `json:"id"`
	Title       string                   `json:"title"`
	Student     adminLoanStudentResponse `json:"student"`
	CreatedAt   time.Time                `json:"created_at"`
	PurchasedAt *time.Time               `json:"purchased_at"`
}

// createBookRequest records a title a student would like the club to buy.
// Students never see requests afterwards, their own included.
func (s *Server) createBookRequest(w http.ResponseWriter, r *http.Request, user gen.User) {
	var input struct {
		Title string `json:"title"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	title := strings.TrimSpace(input.Title)
	if title == "" {
		writeError(w, http.StatusBadRequest, "Напиши название книги.")
		return
	}
	if utf8.RuneCountInString(title) > maxBookRequestTitle {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("Название длиннее %d символов.", maxBookRequestTitle))
		return
	}
	if err := gen.New(s.Pool).CreateBookRequest(r.Context(), gen.CreateBookRequestParams{
		UserID: user.ID,
		Title:  title,
	}); err != nil {
		serverError(w, r, fmt.Errorf("create book request: %w", err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) listAdminBookRequests(w http.ResponseWriter, r *http.Request, _ gen.User) {
	rows, err := gen.New(s.Pool).ListBookRequests(r.Context())
	if err != nil {
		serverError(w, r, fmt.Errorf("list book requests: %w", err))
		return
	}
	requests := make([]bookRequestResponse, 0, len(rows))
	for _, row := range rows {
		requests = append(requests, bookRequestResponse{
			ID:          row.ID,
			Title:       row.Title,
			Student:     adminLoanStudentResponse{ID: row.StudentID, DisplayName: row.StudentName},
			CreatedAt:   row.CreatedAt,
			PurchasedAt: row.PurchasedAt,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"requests": requests})
}

func (s *Server) setAdminBookRequestPurchased(w http.ResponseWriter, r *http.Request, _ gen.User) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	var input struct {
		Purchased bool `json:"purchased"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	n, err := gen.New(s.Pool).SetBookRequestPurchased(r.Context(), gen.SetBookRequestPurchasedParams{
		Purchased: input.Purchased,
		ID:        id,
	})
	if err != nil {
		serverError(w, r, fmt.Errorf("set book request purchased: %w", err))
		return
	}
	if n == 0 {
		http.NotFound(w, r)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) deleteAdminBookRequest(w http.ResponseWriter, r *http.Request, _ gen.User) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	n, err := gen.New(s.Pool).DeleteBookRequest(r.Context(), id)
	if err != nil {
		serverError(w, r, fmt.Errorf("delete book request: %w", err))
		return
	}
	if n == 0 {
		http.NotFound(w, r)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
