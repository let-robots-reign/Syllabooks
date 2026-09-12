package handlers

import (
	"net/http"

	"github.com/google/uuid"

	"syllabooks/db/gen"
)

// resetPassword is the teacher's "Сбросить пароль" (PRD §8): the student's next
// code entry lands on "Придумай пароль". Their sessions are deleted in the same
// transaction, so any device still signed in is locked out too.
func (s *Server) resetPassword(w http.ResponseWriter, r *http.Request, _ gen.User) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	tx, err := s.Pool.Begin(r.Context())
	if err != nil {
		serverError(w, r, err)
		return
	}
	defer tx.Rollback(r.Context()) // a no-op once committed

	q := gen.New(tx)
	n, err := q.ResetPassword(r.Context(), id)
	if err != nil {
		serverError(w, r, err)
		return
	}
	if n == 0 {
		writeError(w, http.StatusNotFound, "Нет ученика с кодом и таким id.")
		return
	}
	if err := q.DeleteUserSessions(r.Context(), id); err != nil {
		serverError(w, r, err)
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		serverError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
