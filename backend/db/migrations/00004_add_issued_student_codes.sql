-- +goose Up

-- Codes are usernames, but a teacher can rotate one when a paper copy is
-- lost. Keep every value ever issued so an old code can never belong to a
-- different student later.
CREATE TABLE issued_student_codes (
    code      text PRIMARY KEY CHECK (code = upper(code)),
    issued_at timestamptz NOT NULL DEFAULT now()
);

INSERT INTO issued_student_codes (code, issued_at)
SELECT code, created_at
FROM users
WHERE code IS NOT NULL;

-- +goose Down

DROP TABLE issued_student_codes;
