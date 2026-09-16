-- name: CreateLoan :one
-- Fails with a unique violation on loans_one_open_per_book or
-- loans_one_open_per_user. due_at uses the same now() as taken_at's default.
INSERT INTO loans (book_id, user_id, due_at, created_by_admin)
VALUES (@book_id, @user_id, now() + sqlc.arg(loan_days)::integer * interval '1 day', @created_by_admin)
RETURNING *;

-- name: GetBookByISBN :one
SELECT *
FROM books
WHERE isbn = @isbn;

-- name: GetOpenLoanForUser :one
SELECT *
FROM loans
WHERE user_id = @user_id AND returned_at IS NULL;

-- name: GetOpenLoanForBook :one
SELECT l.id,
       l.book_id,
       l.user_id,
       l.taken_at,
       l.due_at,
       l.returned_at,
       l.return_reason,
       l.shelf_scan_ok,
       l.book_scan_ok,
       l.created_by_admin,
       u.display_name AS borrower_name
FROM loans l
JOIN users u ON u.id = l.user_id
WHERE l.book_id = @book_id AND l.returned_at IS NULL;

-- name: GetCurrentLoanWithBook :one
SELECT l.id,
       l.book_id,
       l.user_id,
       l.taken_at,
       l.due_at,
       l.returned_at,
       l.return_reason,
       l.shelf_scan_ok,
       l.book_scan_ok,
       l.created_by_admin,
       b.isbn AS book_isbn,
       b.title AS book_title,
       b.author AS book_author,
       b.level AS book_level,
       b.page_count AS book_page_count,
       b.description AS book_description,
       b.cover_url AS book_cover_url
FROM loans l
JOIN books b ON b.id = l.book_id
WHERE l.user_id = @user_id AND l.returned_at IS NULL;

-- name: GetLoanWithBookForUser :one
SELECT l.id,
       l.book_id,
       l.user_id,
       l.taken_at,
       l.due_at,
       l.returned_at,
       l.return_reason,
       l.shelf_scan_ok,
       l.book_scan_ok,
       l.created_by_admin,
       b.isbn AS book_isbn,
       b.title AS book_title,
       b.author AS book_author,
       b.level AS book_level,
       b.page_count AS book_page_count,
       b.description AS book_description,
       b.cover_url AS book_cover_url
FROM loans l
JOIN books b ON b.id = l.book_id
WHERE l.id = @id AND l.user_id = @user_id;

-- name: CompleteReturn :one
UPDATE loans
SET returned_at = now(),
    shelf_scan_ok = @shelf_scan_ok,
    book_scan_ok = @book_scan_ok
WHERE id = @id AND user_id = @user_id AND returned_at IS NULL
RETURNING *;

-- name: SetReturnReason :one
UPDATE loans
SET return_reason = @return_reason
WHERE id = @id
  AND user_id = @user_id
  AND returned_at IS NOT NULL
  AND return_reason IS NULL
RETURNING *;
