-- name: ListExportBooks :many
SELECT id,
       isbn,
       title,
       author,
       level,
       page_count,
       description,
       cover_url,
       is_lost,
       lost_at,
       notes,
       created_at
FROM books
ORDER BY lower(title), title, id;

-- name: ListExportUsers :many
SELECT id,
       display_name,
       COALESCE(
           CASE
               WHEN code IS NOT NULL THEN 'code'
               ELSE oauth_provider
           END,
           ''
       )::text AS login_method,
       code,
       status,
       is_admin,
       created_at
FROM users
ORDER BY created_at, id;

-- name: ListExportLoans :many
SELECT l.id,
       l.book_id,
       b.title AS book_title,
       l.user_id,
       u.display_name AS user_display_name,
       l.taken_at,
       l.due_at,
       l.returned_at,
       l.return_reason,
       l.shelf_scan_ok,
       l.book_scan_ok,
       l.created_by_admin,
       l.scan_reviewed_at
FROM loans l
JOIN books b ON b.id = l.book_id
JOIN users u ON u.id = l.user_id
ORDER BY l.taken_at, l.id;
