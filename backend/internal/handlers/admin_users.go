package handlers

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"syllabooks/db/gen"
)

const (
	studentCodeAlphabet = "2346789ABCDEFGHJKMNPQRTUVWXYZ"
	studentCodeLength   = 6
	studentCodeAttempts = 64
)

// The alphabet is Latin-only, so Russian words are represented by the common
// transliterations a student would actually read from a paper code.
var blockedStudentCodeParts = []string{
	"FUCK", "FUK", "FCK", "DICK", "DCK", "CUNT", "HUY", "HUI", "XYI",
	"PZD", "PIZD", "BLYA", "EBAT", "EBAN", "YEB", "PEDR", "PIDR",
}

type adminUserSummaryResponse struct {
	FinishedBooks int32 `json:"finished_books"`
	ReadingNow    int32 `json:"reading_now"`
	WithoutBook   int32 `json:"without_book"`
	NeverBorrowed int32 `json:"never_borrowed"`
}

type adminUserBookResponse struct {
	ID    uuid.UUID     `json:"id"`
	Title string        `json:"title"`
	Level gen.BookLevel `json:"level"`
}

type adminUserLoanResponse struct {
	ID    uuid.UUID             `json:"id"`
	DueAt time.Time             `json:"due_at"`
	Book  adminUserBookResponse `json:"book"`
}

type adminUserResponse struct {
	ID             uuid.UUID              `json:"id"`
	DisplayName    string                 `json:"display_name"`
	LoginMethod    string                 `json:"login_method"`
	Code           *string                `json:"code"`
	HasPassword    bool                   `json:"has_password"`
	Status         gen.UserStatus         `json:"status"`
	CreatedAt      time.Time              `json:"created_at"`
	CurrentLoan    *adminUserLoanResponse `json:"current_loan"`
	FinishedCount  int32                  `json:"finished_count"`
	AbandonedCount int32                  `json:"abandoned_count"`
}

type adminUsersResponse struct {
	AsOf    time.Time                `json:"as_of"`
	Summary adminUserSummaryResponse `json:"summary"`
	Users   []adminUserResponse      `json:"users"`
}

type adminUserNameRequest struct {
	DisplayName string `json:"display_name"`
}

type adminUserNameResponse struct {
	ID          uuid.UUID `json:"id"`
	DisplayName string    `json:"display_name"`
}

type issuedCodeResponse struct {
	ID          uuid.UUID `json:"id"`
	DisplayName string    `json:"display_name,omitempty"`
	Code        string    `json:"code"`
}

func (s *Server) listAdminUsers(w http.ResponseWriter, r *http.Request, _ gen.User) {
	queries := gen.New(s.Pool)
	summary, err := queries.GetAdminUserSummary(r.Context())
	if err != nil {
		serverError(w, r, fmt.Errorf("get admin user summary: %w", err))
		return
	}
	rows, err := queries.ListAdminUsers(r.Context())
	if err != nil {
		serverError(w, r, fmt.Errorf("list admin users: %w", err))
		return
	}

	response := adminUsersResponse{
		AsOf: time.Now().UTC(),
		Summary: adminUserSummaryResponse{
			FinishedBooks: summary.FinishedBooks,
			ReadingNow:    summary.ReadingNow,
			WithoutBook:   summary.WithoutBook,
			NeverBorrowed: summary.NeverBorrowed,
		},
		Users: make([]adminUserResponse, 0, len(rows)),
	}
	for _, row := range rows {
		response.Users = append(response.Users, presentAdminUser(row))
	}
	writeJSON(w, http.StatusOK, response)
}

func (s *Server) createAdminUser(w http.ResponseWriter, r *http.Request, _ gen.User) {
	var req adminUserNameRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	displayName, ok := validAdminDisplayName(w, req.DisplayName)
	if !ok {
		return
	}

	tx, err := s.Pool.Begin(r.Context())
	if err != nil {
		serverError(w, r, fmt.Errorf("begin create user transaction: %w", err))
		return
	}
	defer tx.Rollback(r.Context())

	queries := gen.New(tx)
	code, err := s.reserveStudentCode(r.Context(), queries)
	if err != nil {
		serverError(w, r, fmt.Errorf("reserve student code: %w", err))
		return
	}
	user, err := queries.CreateCodeUser(r.Context(), gen.CreateCodeUserParams{
		DisplayName: displayName,
		Code:        code,
	})
	if err != nil {
		serverError(w, r, fmt.Errorf("create code user: %w", err))
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		serverError(w, r, fmt.Errorf("commit create user: %w", err))
		return
	}
	writeJSON(w, http.StatusCreated, issuedCodeResponse{
		ID: user.ID, DisplayName: user.DisplayName, Code: code,
	})
}

func (s *Server) updateAdminUser(w http.ResponseWriter, r *http.Request, _ gen.User) {
	id, ok := adminUserID(w, r)
	if !ok {
		return
	}
	var req adminUserNameRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	displayName, ok := validAdminDisplayName(w, req.DisplayName)
	if !ok {
		return
	}
	user, err := gen.New(s.Pool).UpdateAdminStudentName(r.Context(), gen.UpdateAdminStudentNameParams{
		ID: id, DisplayName: displayName,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		serverError(w, r, fmt.Errorf("update admin user: %w", err))
		return
	}
	writeJSON(w, http.StatusOK, adminUserNameResponse(user))
}

func (s *Server) reissueCode(w http.ResponseWriter, r *http.Request, _ gen.User) {
	id, ok := adminUserID(w, r)
	if !ok {
		return
	}
	tx, err := s.Pool.Begin(r.Context())
	if err != nil {
		serverError(w, r, fmt.Errorf("begin reissue code transaction: %w", err))
		return
	}
	defer tx.Rollback(r.Context())

	queries := gen.New(tx)
	student, err := queries.GetAdminStudentForUpdate(r.Context(), id)
	if errors.Is(err, pgx.ErrNoRows) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		serverError(w, r, fmt.Errorf("get student for code reissue: %w", err))
		return
	}
	if student.Code == nil {
		writeError(w, http.StatusConflict, "У этого ученика нет входа по коду.")
		return
	}
	code, err := s.reserveStudentCode(r.Context(), queries)
	if err != nil {
		serverError(w, r, fmt.Errorf("reserve replacement code: %w", err))
		return
	}
	if err := queries.ReissueStudentCode(r.Context(), gen.ReissueStudentCodeParams{ID: id, Code: code}); err != nil {
		serverError(w, r, fmt.Errorf("reissue student code: %w", err))
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		serverError(w, r, fmt.Errorf("commit reissue code: %w", err))
		return
	}
	writeJSON(w, http.StatusOK, issuedCodeResponse{ID: id, Code: code})
}

// resetPassword is the teacher's "Сбросить пароль" (PRD §8): the student's next
// code entry lands on "Придумай пароль". Their sessions are deleted in the same
// transaction, so any device still signed in is locked out too.
func (s *Server) resetPassword(w http.ResponseWriter, r *http.Request, _ gen.User) {
	id, ok := adminUserID(w, r)
	if !ok {
		return
	}
	tx, err := s.Pool.Begin(r.Context())
	if err != nil {
		serverError(w, r, fmt.Errorf("begin reset password transaction: %w", err))
		return
	}
	defer tx.Rollback(r.Context())

	queries := gen.New(tx)
	student, err := queries.GetAdminStudentForUpdate(r.Context(), id)
	if errors.Is(err, pgx.ErrNoRows) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		serverError(w, r, fmt.Errorf("get student for password reset: %w", err))
		return
	}
	if student.Code == nil {
		writeError(w, http.StatusConflict, "У этого ученика нет входа по коду.")
		return
	}
	if _, err := queries.ResetPassword(r.Context(), id); err != nil {
		serverError(w, r, fmt.Errorf("reset password: %w", err))
		return
	}
	if err := queries.DeleteUserSessions(r.Context(), id); err != nil {
		serverError(w, r, fmt.Errorf("delete reset user sessions: %w", err))
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		serverError(w, r, fmt.Errorf("commit reset password: %w", err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) banAdminUser(w http.ResponseWriter, r *http.Request, _ gen.User) {
	id, ok := adminUserID(w, r)
	if !ok {
		return
	}
	tx, err := s.Pool.Begin(r.Context())
	if err != nil {
		serverError(w, r, fmt.Errorf("begin ban user transaction: %w", err))
		return
	}
	defer tx.Rollback(r.Context())

	queries := gen.New(tx)
	n, err := queries.BanAdminStudent(r.Context(), id)
	if err != nil {
		serverError(w, r, fmt.Errorf("ban admin user: %w", err))
		return
	}
	if n == 0 {
		http.NotFound(w, r)
		return
	}
	if err := queries.DeleteUserSessions(r.Context(), id); err != nil {
		serverError(w, r, fmt.Errorf("delete banned user sessions: %w", err))
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		serverError(w, r, fmt.Errorf("commit ban user: %w", err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) reserveStudentCode(ctx context.Context, queries *gen.Queries) (string, error) {
	for range studentCodeAttempts {
		code, err := s.nextStudentCode()
		if err != nil {
			return "", err
		}
		if !studentCodeIsClean(code) {
			continue
		}
		n, err := queries.ReserveStudentCode(ctx, code)
		if err != nil {
			return "", err
		}
		if n == 1 {
			return code, nil
		}
	}
	return "", errors.New("could not generate a unique student code")
}

func (s *Server) nextStudentCode() (string, error) {
	if s.StudentCodeGenerator != nil {
		return s.StudentCodeGenerator()
	}
	return newStudentCode()
}

func newStudentCode() (string, error) {
	code := make([]byte, studentCodeLength)
	limit := big.NewInt(int64(len(studentCodeAlphabet)))
	for index := range code {
		value, err := rand.Int(rand.Reader, limit)
		if err != nil {
			return "", fmt.Errorf("read random code: %w", err)
		}
		code[index] = studentCodeAlphabet[value.Int64()]
	}
	return string(code), nil
}

func studentCodeIsClean(code string) bool {
	if len(code) != studentCodeLength {
		return false
	}
	for _, char := range code {
		if !strings.ContainsRune(studentCodeAlphabet, char) {
			return false
		}
	}
	for _, blocked := range blockedStudentCodeParts {
		if strings.Contains(code, blocked) {
			return false
		}
	}
	return true
}

func validAdminDisplayName(w http.ResponseWriter, raw string) (string, bool) {
	name := strings.TrimSpace(raw)
	if name == "" {
		writeError(w, http.StatusBadRequest, "Укажи имя ученика.")
		return "", false
	}
	if utf8.RuneCountInString(name) > 120 {
		writeError(w, http.StatusBadRequest, "Имя слишком длинное.")
		return "", false
	}
	return name, true
}

func adminUserID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.NotFound(w, r)
		return uuid.Nil, false
	}
	return id, true
}

func presentAdminUser(row gen.ListAdminUsersRow) adminUserResponse {
	loginMethod := "code"
	if row.OauthProvider != nil {
		loginMethod = *row.OauthProvider
	}
	user := adminUserResponse{
		ID: row.ID, DisplayName: row.DisplayName, LoginMethod: loginMethod,
		Code: row.Code, HasPassword: row.HasPassword, Status: row.Status,
		CreatedAt: row.CreatedAt, FinishedCount: row.FinishedCount,
		AbandonedCount: row.AbandonedCount,
	}
	if row.LoanID != uuid.Nil {
		user.CurrentLoan = &adminUserLoanResponse{
			ID: row.LoanID, DueAt: row.DueAt,
			Book: adminUserBookResponse{
				ID: row.BookID, Title: row.BookTitle, Level: gen.BookLevel(row.BookLevel),
			},
		}
	}
	return user
}
