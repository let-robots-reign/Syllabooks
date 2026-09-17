package handlers

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"syllabooks/db/gen"
)

var (
	bookExportHeader = []string{
		"id", "isbn", "title", "author", "level", "page_count",
		"description", "cover_url", "is_lost", "lost_at", "notes", "created_at",
	}
	userExportHeader = []string{
		"id", "display_name", "login_method", "code", "status", "is_admin", "created_at",
	}
	loanExportHeader = []string{
		"id", "book_id", "book_title", "user_id", "user_display_name",
		"taken_at", "due_at", "returned_at", "return_reason", "shelf_scan_ok",
		"book_scan_ok", "created_by_admin", "scan_reviewed_at",
	}
)

func (s *Server) exportBooksCSV(w http.ResponseWriter, r *http.Request, _ gen.User) {
	books, err := gen.New(s.Pool).ListExportBooks(r.Context())
	if err != nil {
		serverError(w, r, fmt.Errorf("list books for export: %w", err))
		return
	}

	rows := make([][]string, 0, len(books))
	for _, book := range books {
		rows = append(rows, []string{
			book.ID.String(),
			spreadsheetText(optionalString(book.Isbn)),
			spreadsheetText(book.Title),
			spreadsheetText(book.Author),
			string(book.Level),
			fmt.Sprint(book.PageCount),
			spreadsheetText(optionalString(book.Description)),
			spreadsheetText(optionalString(book.CoverUrl)),
			fmt.Sprint(book.IsLost),
			optionalTime(book.LostAt),
			spreadsheetText(optionalString(book.Notes)),
			csvTime(book.CreatedAt),
		})
	}
	writeCSVDownload(w, r, "books", bookExportHeader, rows)
}

func (s *Server) exportUsersCSV(w http.ResponseWriter, r *http.Request, _ gen.User) {
	users, err := gen.New(s.Pool).ListExportUsers(r.Context())
	if err != nil {
		serverError(w, r, fmt.Errorf("list users for export: %w", err))
		return
	}

	rows := make([][]string, 0, len(users))
	for _, user := range users {
		rows = append(rows, []string{
			user.ID.String(),
			spreadsheetText(user.DisplayName),
			user.LoginMethod,
			spreadsheetText(optionalString(user.Code)),
			string(user.Status),
			fmt.Sprint(user.IsAdmin),
			csvTime(user.CreatedAt),
		})
	}
	writeCSVDownload(w, r, "users", userExportHeader, rows)
}

func (s *Server) exportLoansCSV(w http.ResponseWriter, r *http.Request, _ gen.User) {
	loans, err := gen.New(s.Pool).ListExportLoans(r.Context())
	if err != nil {
		serverError(w, r, fmt.Errorf("list loans for export: %w", err))
		return
	}

	rows := make([][]string, 0, len(loans))
	for _, loan := range loans {
		rows = append(rows, []string{
			loan.ID.String(),
			loan.BookID.String(),
			spreadsheetText(loan.BookTitle),
			loan.UserID.String(),
			spreadsheetText(loan.UserDisplayName),
			csvTime(loan.TakenAt),
			csvTime(loan.DueAt),
			optionalTime(loan.ReturnedAt),
			optionalReturnReason(loan.ReturnReason),
			optionalBool(loan.ShelfScanOk),
			optionalBool(loan.BookScanOk),
			fmt.Sprint(loan.CreatedByAdmin),
			optionalTime(loan.ScanReviewedAt),
		})
	}
	writeCSVDownload(w, r, "loans", loanExportHeader, rows)
}

func writeCSVDownload(w http.ResponseWriter, r *http.Request, name string, header []string, rows [][]string) {
	var body bytes.Buffer
	body.WriteString("\uFEFF")
	writer := csv.NewWriter(&body)
	writer.UseCRLF = true
	_ = writer.Write(header)
	writer.WriteAll(rows)
	writer.Flush()
	if err := writer.Error(); err != nil {
		serverError(w, r, fmt.Errorf("encode %s export: %w", name, err))
		return
	}

	filename := fmt.Sprintf("syllabooks-%s-%s.csv", name, time.Now().UTC().Format(time.DateOnly))
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if _, err := w.Write(body.Bytes()); err != nil {
		log.Printf("write %s export: %v", name, err)
	}
}

func spreadsheetText(value string) string {
	trimmed := strings.TrimLeft(value, " \t\r\n")
	if trimmed == "" {
		return value
	}
	switch trimmed[0] {
	case '=', '+', '-', '@':
		return "'" + value
	default:
		return value
	}
}

func optionalString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func csvTime(value time.Time) string {
	return value.UTC().Format(time.RFC3339)
}

func optionalTime(value *time.Time) string {
	if value == nil {
		return ""
	}
	return csvTime(*value)
}

func optionalBool(value *bool) string {
	if value == nil {
		return ""
	}
	return fmt.Sprint(*value)
}

func optionalReturnReason(value *gen.ReturnReason) string {
	if value == nil {
		return ""
	}
	return string(*value)
}
