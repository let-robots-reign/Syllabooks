-- +goose Up

-- An inactive book stays in the admin catalogue with its loan history but is
-- hidden from students and cannot be borrowed. Deleting is for mistakes.
ALTER TABLE books ADD COLUMN is_active boolean NOT NULL DEFAULT true;

-- +goose Down

ALTER TABLE books DROP COLUMN is_active;
