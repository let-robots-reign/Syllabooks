package handlers

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"syllabooks/db/gen"
)

// Sessions are opaque random tokens rather than JWTs, so deleting a row (on
// ban, on password reset, or by hand) locks the client out on its very next
// request. The client sends the token as "Authorization: Bearer <token>";
// only its SHA-256 is stored.

// sessionLifetime is long on purpose (PRD §7): a student who has to log in
// again every week stops using the app.
const sessionLifetime = 365 * 24 * time.Hour

func newToken() string {
	b := make([]byte, 32)
	rand.Read(b) // never returns an error since Go 1.24
	return base64.RawURLEncoding.EncodeToString(b)
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// startSession creates a session for the user and returns its token.
func (s *Server) startSession(ctx context.Context, userID uuid.UUID) (string, error) {
	token := newToken()
	err := gen.New(s.Pool).CreateSession(ctx, gen.CreateSessionParams{
		TokenHash: hashToken(token),
		UserID:    userID,
		ExpiresAt: time.Now().Add(sessionLifetime),
	})
	if err != nil {
		return "", fmt.Errorf("create session: %w", err)
	}
	return token, nil
}

func bearerToken(r *http.Request) (string, bool) {
	token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	return token, ok && token != ""
}

// authenticate resolves the request's bearer token to a user. ok is false
// when there is no token, the session is unknown or expired, or the user is
// banned: a banned user is simply not signed in.
func (s *Server) authenticate(r *http.Request) (user gen.User, ok bool, err error) {
	token, ok := bearerToken(r)
	if !ok {
		return gen.User{}, false, nil
	}
	user, err = gen.New(s.Pool).GetSessionUser(r.Context(), hashToken(token))
	if errors.Is(err, pgx.ErrNoRows) {
		return gen.User{}, false, nil
	}
	if err != nil {
		return gen.User{}, false, fmt.Errorf("look up session: %w", err)
	}
	return user, user.Status != gen.UserStatusBanned, nil
}

type userHandler func(w http.ResponseWriter, r *http.Request, user gen.User)

// requireUser passes the signed-in user to h, or answers 401.
func (s *Server) requireUser(h userHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok, err := s.authenticate(r)
		if err != nil {
			serverError(w, r, err)
			return
		}
		if !ok {
			writeError(w, http.StatusUnauthorized, "Войди заново.")
			return
		}
		h(w, r, user)
	}
}

// requireAdmin passes a signed-in admin to h. Everyone else, signed in or
// not, gets the same 404 as an unknown route (PRD §8).
func (s *Server) requireAdmin(h userHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok, err := s.authenticate(r)
		if err != nil {
			serverError(w, r, err)
			return
		}
		if !ok || !user.IsAdmin {
			http.NotFound(w, r)
			return
		}
		h(w, r, user)
	}
}

func (s *Server) me(w http.ResponseWriter, r *http.Request, user gen.User) {
	writeJSON(w, http.StatusOK, map[string]any{
		"id":           user.ID,
		"display_name": user.DisplayName,
		"is_admin":     user.IsAdmin,
	})
}

// logout ends the session behind the request's token. It needs no valid
// session: signing out always succeeds.
func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if token, ok := bearerToken(r); ok {
		if err := gen.New(s.Pool).DeleteSession(r.Context(), hashToken(token)); err != nil {
			serverError(w, r, err)
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}
