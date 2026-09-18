-- +goose Up

-- NULL means this account still needs to see onboarding. Do not backfill:
-- the app has not launched yet, and existing development accounts should be
-- able to exercise the first-login experience too.
ALTER TABLE users ADD COLUMN onboarding_completed_at timestamptz;

-- +goose Down

ALTER TABLE users DROP COLUMN onboarding_completed_at;
