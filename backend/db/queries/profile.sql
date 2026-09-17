-- name: ListProfileHistory :many
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
WHERE l.user_id = @user_id
  AND l.returned_at IS NOT NULL
ORDER BY l.returned_at DESC, l.id DESC;

-- name: GetProfileStats :one
SELECT count(*) FILTER (WHERE l.return_reason = 'finished')::integer AS finished_books,
       COALESCE(sum(b.page_count) FILTER (WHERE l.return_reason = 'finished'), 0)::bigint AS pages_read
FROM loans l
JOIN books b ON b.id = l.book_id
WHERE l.user_id = @user_id;
