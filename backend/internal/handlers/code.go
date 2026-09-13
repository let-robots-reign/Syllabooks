package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"syllabooks/db/gen"
)

// Code login (PRD §8). The teacher gives a student a permanent code. One
// screen serves the first login and every login after it: the student enters
// the code, and whether the account has a password decides whether the next
// field says "Придумай пароль" or "Введи пароль".

type codeRequest struct {
	Code     string `json:"code"`
	Password string `json:"password"`
}

const codeNotFound = "Такого кода нет. Проверь, как он написан на листке."

// Codes are stored uppercase, so uppercasing the input makes login
// case-insensitive.
func normalizeCode(code string) string {
	return strings.ToUpper(strings.TrimSpace(code))
}

// codeCheck tells the login screen which password field to show.
func (s *Server) codeCheck(w http.ResponseWriter, r *http.Request) {
	var req codeRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	user, err := gen.New(s.Pool).GetUserByCode(r.Context(), normalizeCode(req.Code))
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, codeNotFound)
		return
	}
	if err != nil {
		serverError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"has_password": user.PasswordHash != nil})
}

// codeLogin signs a code user in. When the account has no password yet (first
// login, or after a teacher reset) the password given becomes its password.
// The server picks the branch from the account's state, not from the client.
func (s *Server) codeLogin(w http.ResponseWriter, r *http.Request) {
	var req codeRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	q := gen.New(s.Pool)
	user, err := q.GetUserByCode(r.Context(), normalizeCode(req.Code))
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, codeNotFound)
		return
	}
	if err != nil {
		serverError(w, r, err)
		return
	}
	if user.Status == gen.UserStatusBanned {
		writeError(w, http.StatusForbidden, "Аккаунт заблокирован. Обратись к учителю.")
		return
	}

	if user.PasswordHash == nil {
		if utf8.RuneCountInString(req.Password) < minPasswordLength {
			writeError(w, http.StatusBadRequest,
				fmt.Sprintf("Пароль должен быть не короче %d символов.", minPasswordLength))
			return
		}
		n, err := q.SetPasswordIfUnset(r.Context(), gen.SetPasswordIfUnsetParams{
			ID:           user.ID,
			PasswordHash: hashPassword(req.Password),
		})
		if err != nil {
			serverError(w, r, err)
			return
		}
		if n == 0 {
			// Another device set a password since the lookup above.
			writeError(w, http.StatusConflict, "Пароль уже задан с другого устройства. Введи его, чтобы войти.")
			return
		}
	} else {
		ok, err := checkPassword(*user.PasswordHash, req.Password)
		if err != nil {
			serverError(w, r, err)
			return
		}
		if !ok {
			writeError(w, http.StatusUnauthorized, "Неверный пароль. Если не помнишь его, попроси учителя сбросить пароль.")
			return
		}
	}

	token, err := s.startSession(r.Context(), user.ID)
	if err != nil {
		serverError(w, r, err)
		return
	}
	s.setSessionCookie(w, token)
	w.WriteHeader(http.StatusNoContent)
}
