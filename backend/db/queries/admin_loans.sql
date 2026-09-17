-- name: GetAdminStats :one
SELECT (SELECT count(*) FROM loans WHERE returned_at IS NULL)::integer AS open_loans,
       (SELECT count(*) FROM books)::integer AS books,
       (SELECT count(*) FROM books WHERE is_lost)::integer AS lost_books,
       (SELECT count(*) FROM users WHERE NOT is_admin)::integer AS users;

-- name: ListAdminOpenLoans :many
SELECT l.id,
       l.taken_at,
       l.due_at,
       b.id AS book_id,
       b.isbn AS book_isbn,
       b.title AS book_title,
       b.level AS book_level,
       b.page_count AS book_page_count,
       u.id AS student_id,
       u.display_name AS student_name
FROM loans l
JOIN books b ON b.id = l.book_id
JOIN users u ON u.id = l.user_id
WHERE l.returned_at IS NULL
ORDER BY (l.due_at < now()) DESC,
         l.due_at,
         lower(u.display_name),
         lower(b.title),
         l.id;

-- name: ListAdminScanReviews :many
SELECT l.id,
       l.taken_at,
       l.due_at,
       l.returned_at,
       l.shelf_scan_ok,
       l.book_scan_ok,
       b.id AS book_id,
       b.isbn AS book_isbn,
       b.title AS book_title,
       b.level AS book_level,
       b.page_count AS book_page_count,
       u.id AS student_id,
       u.display_name AS student_name
FROM loans l
JOIN books b ON b.id = l.book_id
JOIN users u ON u.id = l.user_id
WHERE l.returned_at IS NOT NULL
  AND l.scan_reviewed_at IS NULL
  AND (l.shelf_scan_ok IS FALSE OR l.book_scan_ok IS FALSE)
ORDER BY l.returned_at DESC, l.id;

-- name: ListAdminLostBooks :many
SELECT b.id,
       b.isbn,
       b.title,
       b.level,
       b.page_count,
       b.lost_at,
       COALESCE(last_loan.borrower_name, '')::text AS last_borrower_name,
       COALESCE(last_loan.taken_at, b.lost_at) AS last_taken_at,
       COALESCE(last_loan.loan_id, '00000000-0000-0000-0000-000000000000'::uuid) AS last_loan_id
FROM books b
LEFT JOIN LATERAL (
    SELECT l.id AS loan_id, u.display_name AS borrower_name, l.taken_at
    FROM loans l
    JOIN users u ON u.id = l.user_id
    WHERE l.book_id = b.id
    ORDER BY COALESCE(l.returned_at, l.taken_at) DESC, l.id DESC
    LIMIT 1
) last_loan ON true
WHERE b.is_lost
ORDER BY b.lost_at DESC, lower(b.title), b.id;

-- name: ExtendAdminLoan :one
UPDATE loans
SET due_at = due_at + interval '7 days'
WHERE id = @id AND returned_at IS NULL
RETURNING due_at;

-- name: CompleteAdminReturn :one
UPDATE loans
SET returned_at = now(),
    shelf_scan_ok = false,
    book_scan_ok = false,
    created_by_admin = true,
    scan_reviewed_at = now()
WHERE id = @id AND returned_at IS NULL
RETURNING *;

-- name: GetAdminLoanState :one
SELECT book_id, returned_at
FROM loans
WHERE id = @id;

-- name: ReviewAdminLoanScan :one
UPDATE loans
SET scan_reviewed_at = COALESCE(scan_reviewed_at, now())
WHERE id = @id
  AND returned_at IS NOT NULL
  AND (shelf_scan_ok IS FALSE OR book_scan_ok IS FALSE)
RETURNING id;

-- name: MarkAdminBookLost :one
UPDATE books
SET is_lost = true,
    lost_at = now()
WHERE id = @id AND NOT is_lost
RETURNING id;

-- name: MarkAdminBookFound :one
UPDATE books
SET is_lost = false,
    lost_at = NULL
WHERE id = @id
RETURNING id;
