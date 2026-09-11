-- name: CreateLoan :one
-- Fails with a unique violation on loans_one_open_per_book or
-- loans_one_open_per_user. due_at uses the same now() as taken_at's default.
INSERT INTO loans (book_id, user_id, due_at, created_by_admin)
VALUES (@book_id, @user_id, now() + sqlc.arg(loan_days)::integer * interval '1 day', @created_by_admin)
RETURNING *;
