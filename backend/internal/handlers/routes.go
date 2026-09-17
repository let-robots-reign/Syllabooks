package handlers

import (
	"fmt"
	"io/fs"
	"log"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Server holds what the handlers share. Handlers call sqlc queries directly.
type Server struct {
	Pool *pgxpool.Pool
	// LoanDays is the borrowing period. Values <= 0 use the 21-day default.
	LoanDays int32
	// ShelfCode is the exact payload encoded in the QR beside the shelf.
	ShelfCode string
	// OAuth providers, each nil when not configured.
	Yandex, VK *Provider
	// SecureCookies marks cookies Secure; true when the app is served over HTTPS.
	SecureCookies bool
	// Frontend is the built single-page app, served for every path outside
	// /api/. Without an index.html only the API is served.
	Frontend fs.FS
	// BookCoversDir is the persistent directory behind /api/book-covers/.
	BookCoversDir string
	// HTTPClient and the base URLs are configurable so metadata lookup can be
	// tested without contacting the real providers.
	HTTPClient         *http.Client
	OpenLibraryBaseURL string
	GoogleBooksBaseURL string
	GoogleBooksAPIKey  string
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", s.health)
	mux.HandleFunc("GET /api/book-covers/{filename}", s.bookCover)

	mux.HandleFunc("GET /api/auth/yandex", s.oauthStart(s.Yandex))
	mux.HandleFunc("GET /api/auth/callback/yandex", s.oauthCallback(s.Yandex))
	mux.HandleFunc("GET /api/auth/vk", s.oauthStart(s.VK))
	mux.HandleFunc("GET /api/auth/callback/vk", s.oauthCallback(s.VK))
	mux.HandleFunc("POST /api/auth/code/check", s.codeCheck)
	mux.HandleFunc("POST /api/auth/code/login", s.codeLogin)
	mux.HandleFunc("POST /api/auth/logout", s.logout)
	mux.HandleFunc("GET /api/me", s.requireUser(s.me))
	mux.HandleFunc("GET /api/books", s.requireUser(s.listCatalog))
	mux.HandleFunc("GET /api/books/{id}", s.requireUser(s.getCatalogBook))
	mux.HandleFunc("POST /api/loans", s.requireUser(s.borrowBook))
	mux.HandleFunc("GET /api/loans/current", s.requireUser(s.currentLoan))
	mux.HandleFunc("POST /api/loans/{id}/shelf-check", s.requireUser(s.checkShelfCode))
	mux.HandleFunc("POST /api/loans/{id}/return", s.requireUser(s.returnBook))
	mux.HandleFunc("POST /api/loans/{id}/return-reason", s.requireUser(s.setReturnReason))

	mux.HandleFunc("GET /api/admin/books", s.requireAdmin(s.listBooks))
	mux.HandleFunc("POST /api/admin/books", s.requireAdmin(s.createBook))
	mux.HandleFunc("POST /api/admin/books/lookup", s.requireAdmin(s.lookupBookMetadata))
	mux.HandleFunc("GET /api/admin/books/{id}", s.requireAdmin(s.getBook))
	mux.HandleFunc("PUT /api/admin/books/{id}", s.requireAdmin(s.updateBook))
	mux.HandleFunc("DELETE /api/admin/books/{id}", s.requireAdmin(s.deleteBook))
	mux.HandleFunc("PUT /api/admin/books/{id}/cover", s.requireAdmin(s.uploadBookCover))
	mux.HandleFunc("DELETE /api/admin/books/{id}/cover", s.requireAdmin(s.deleteBookCover))
	mux.HandleFunc("POST /api/admin/books/{id}/found", s.requireAdmin(s.markAdminBookFound))
	mux.HandleFunc("GET /api/admin/stats", s.requireAdmin(s.adminStats))
	mux.HandleFunc("GET /api/admin/loans", s.requireAdmin(s.listAdminLoans))
	mux.HandleFunc("POST /api/admin/loans/{id}/extend", s.requireAdmin(s.extendAdminLoan))
	mux.HandleFunc("POST /api/admin/loans/{id}/return", s.requireAdmin(s.completeAdminReturn))
	mux.HandleFunc("POST /api/admin/loans/{id}/lost", s.requireAdmin(s.markAdminLoanLost))
	mux.HandleFunc("POST /api/admin/loans/{id}/review", s.requireAdmin(s.reviewAdminLoanScan))
	mux.HandleFunc("GET /api/admin/lost", s.requireAdmin(s.listAdminLostBooks))
	mux.HandleFunc("POST /api/admin/users/{id}/reset-password", s.requireAdmin(s.resetPassword))

	// An unknown API path is a plain 404, never the app's index.html.
	mux.HandleFunc("/api/", http.NotFound)
	mux.HandleFunc("/", s.frontend)
	return mux
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	if err := s.Pool.Ping(r.Context()); err != nil {
		log.Printf("health: %v", err)
		http.Error(w, "database unavailable", http.StatusServiceUnavailable)
		return
	}
	fmt.Fprintln(w, "ok")
}
