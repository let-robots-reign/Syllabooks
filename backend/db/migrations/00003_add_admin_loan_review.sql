-- +goose Up

-- A lost book needs a date for the teacher's registry. Keep the timestamp and
-- the existing boolean in lockstep so catalogue filtering cannot drift.
ALTER TABLE books ADD COLUMN lost_at timestamptz;
UPDATE books SET lost_at = now() WHERE is_lost;
ALTER TABLE books
    ADD CONSTRAINT books_lost_state
    CHECK (is_lost = (lost_at IS NOT NULL));

-- Failed return scans stay in the teacher's reconciliation queue until they
-- have been checked. Clean returns can never be marked as reviewed.
ALTER TABLE loans ADD COLUMN scan_reviewed_at timestamptz;
ALTER TABLE loans
    ADD CONSTRAINT loans_scan_review_only_on_flagged_return
    CHECK (
        scan_reviewed_at IS NULL
        OR (
            returned_at IS NOT NULL
            AND (shelf_scan_ok IS FALSE OR book_scan_ok IS FALSE)
        )
    );

-- +goose Down

ALTER TABLE loans DROP CONSTRAINT loans_scan_review_only_on_flagged_return;
ALTER TABLE loans DROP COLUMN scan_reviewed_at;
ALTER TABLE books DROP CONSTRAINT books_lost_state;
ALTER TABLE books DROP COLUMN lost_at;
