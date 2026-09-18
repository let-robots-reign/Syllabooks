-- name: CreateCodeUser :one
-- A code-based student, created by the teacher before their first login. The
-- password stays NULL until the student sets one.
INSERT INTO users (display_name, code)
VALUES (@display_name, sqlc.arg(code)::text)
RETURNING *;

-- name: ReserveStudentCode :execrows
-- This table includes active and retired codes. ON CONFLICT lets the handler
-- retry a randomly generated collision without aborting its transaction.
INSERT INTO issued_student_codes (code)
VALUES (sqlc.arg(code)::text)
ON CONFLICT DO NOTHING;

-- name: ListAdminUsers :many
SELECT u.id,
       u.display_name,
       u.oauth_provider,
       u.code,
       (u.password_hash IS NOT NULL)::boolean AS has_password,
       u.status,
       u.created_at,
       COALESCE(current_loan.loan_id, '00000000-0000-0000-0000-000000000000'::uuid) AS loan_id,
       COALESCE(current_loan.due_at, 'epoch'::timestamptz) AS due_at,
       COALESCE(current_loan.book_id, '00000000-0000-0000-0000-000000000000'::uuid) AS book_id,
       COALESCE(current_loan.book_title, '')::text AS book_title,
       COALESCE(current_loan.book_level, '')::text AS book_level,
       (SELECT count(*) FROM loans l
        WHERE l.user_id = u.id AND l.return_reason = 'finished')::integer AS finished_count,
       (SELECT count(*) FROM loans l
        WHERE l.user_id = u.id AND l.return_reason IN ('too_hard', 'boring'))::integer AS abandoned_count
FROM users u
LEFT JOIN LATERAL (
    SELECT l.id AS loan_id,
           l.due_at,
           b.id AS book_id,
           b.title AS book_title,
           b.level::text AS book_level
    FROM loans l
    JOIN books b ON b.id = l.book_id
    WHERE l.user_id = u.id AND l.returned_at IS NULL
    LIMIT 1
) current_loan ON true
WHERE NOT u.is_admin
ORDER BY u.created_at DESC, u.id;

-- name: GetAdminUserSummary :one
SELECT (SELECT count(*) FROM loans WHERE return_reason = 'finished')::integer AS finished_books,
       (SELECT count(*) FROM loans l JOIN users u ON u.id = l.user_id
        WHERE l.returned_at IS NULL AND NOT u.is_admin)::integer AS reading_now,
       (SELECT count(*) FROM users u
        WHERE NOT u.is_admin
          AND NOT EXISTS (SELECT 1 FROM loans l WHERE l.user_id = u.id AND l.returned_at IS NULL))::integer AS without_book,
       (SELECT count(*) FROM users u
        WHERE NOT u.is_admin
          AND NOT EXISTS (SELECT 1 FROM loans l WHERE l.user_id = u.id))::integer AS never_borrowed;

-- name: GetAdminStudentForUpdate :one
SELECT id, code, status
FROM users
WHERE id = @id AND NOT is_admin
FOR UPDATE;

-- name: UpdateAdminStudentName :one
UPDATE users
SET display_name = @display_name
WHERE id = @id AND NOT is_admin
RETURNING id, display_name;

-- name: ReissueStudentCode :exec
UPDATE users
SET code = sqlc.arg(code)::text
WHERE id = @id AND NOT is_admin;

-- name: BanAdminStudent :execrows
UPDATE users
SET status = 'banned'
WHERE id = @id AND NOT is_admin;

-- name: GetUserByCode :one
-- The caller uppercases the code, which makes login case-insensitive.
SELECT * FROM users WHERE code = sqlc.arg(code)::text;

-- name: SetPasswordIfUnset :execrows
-- First login of a code user, or the first after a reset. The IS NULL check
-- stops two devices racing to set a password from overwriting each other:
-- the loser updates 0 rows.
UPDATE users SET password_hash = sqlc.arg(password_hash)::text
WHERE id = @id AND password_hash IS NULL;

-- name: ResetPassword :execrows
-- The teacher's reset. The student's next code entry lands in the "set a
-- password" branch. Only code users have a password, so 0 rows means there
-- is no such code user.
UPDATE users SET password_hash = NULL
WHERE id = @id AND code IS NOT NULL;

-- name: UpsertOAuthUser :one
-- Finds or creates the user for an OAuth identity. A repeat login changes
-- nothing, because display_name belongs to the teacher once the row exists;
-- the no-op SET is only there so RETURNING yields the existing row.
INSERT INTO users (oauth_provider, oauth_subject, display_name)
VALUES (sqlc.arg(oauth_provider)::text, sqlc.arg(oauth_subject)::text, @display_name)
ON CONFLICT (oauth_provider, oauth_subject) DO UPDATE SET oauth_provider = excluded.oauth_provider
RETURNING *;

-- name: CompleteOnboarding :exec
-- Idempotent so retries and two open tabs cannot change the first completion
-- time or turn a successful dismissal into an error.
UPDATE users
SET onboarding_completed_at = COALESCE(onboarding_completed_at, now())
WHERE id = @id;
