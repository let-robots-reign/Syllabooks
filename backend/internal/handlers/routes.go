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
	// OAuth providers, each nil when not configured.
	Yandex, VK *Provider
	// SecureCookies marks cookies Secure; true when the app is served over HTTPS.
	SecureCookies bool
	// Frontend is the built single-page app, served for every path outside
	// /api/. Without an index.html only the API is served.
	Frontend fs.FS
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", s.health)
	mux.HandleFunc("/api/books", BooksHandler)

	mux.HandleFunc("GET /api/auth/yandex", s.oauthStart(s.Yandex))
	mux.HandleFunc("GET /api/auth/callback/yandex", s.oauthCallback(s.Yandex))
	mux.HandleFunc("GET /api/auth/vk", s.oauthStart(s.VK))
	mux.HandleFunc("GET /api/auth/callback/vk", s.oauthCallback(s.VK))
	mux.HandleFunc("POST /api/auth/code/check", s.codeCheck)
	mux.HandleFunc("POST /api/auth/code/login", s.codeLogin)
	mux.HandleFunc("POST /api/auth/logout", s.logout)
	mux.HandleFunc("GET /api/me", s.requireUser(s.me))

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
