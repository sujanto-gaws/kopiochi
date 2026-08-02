-- +goose Up
-- +goose StatementBegin
-- gen_random_uuid() is built into Postgres core as of v13, but pgcrypto is
-- enabled defensively so this migration also works unmodified on older
-- servers where the function only exists via the extension.
CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS auth_users (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    username TEXT NOT NULL,
    email TEXT NOT NULL,
    name TEXT,
    roles TEXT[] NOT NULL DEFAULT '{}',
    permissions TEXT[] NOT NULL DEFAULT '{}',
    password_hash TEXT,
    mfa_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    mfa_secret TEXT,
    failed_login_attempts INTEGER NOT NULL DEFAULT 0,
    locked_until TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Repository lookups (FindByEmail, Save's upsert conflict target considerations)
-- compare email with a plain "=" — no lower()/citext normalization anywhere in
-- the code — so a case-sensitive unique index matches actual query behavior.
CREATE UNIQUE INDEX IF NOT EXISTS idx_auth_users_email ON auth_users(email);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_auth_users_email;
DROP TABLE IF EXISTS auth_users;
-- +goose StatementEnd
