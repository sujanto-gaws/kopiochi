-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS auth_refresh_tokens (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL REFERENCES auth_users(id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    revoked BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- RefreshTokenStore.FindValid looks up by token_hash (plus revoked/expires_at
-- filters); RevokeAllForUser looks up by user_id. Both are hot paths on every
-- refresh/login/logout request.
CREATE INDEX IF NOT EXISTS idx_auth_refresh_tokens_token_hash ON auth_refresh_tokens(token_hash);
CREATE INDEX IF NOT EXISTS idx_auth_refresh_tokens_user_id ON auth_refresh_tokens(user_id);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_auth_refresh_tokens_user_id;
DROP INDEX IF EXISTS idx_auth_refresh_tokens_token_hash;
DROP TABLE IF EXISTS auth_refresh_tokens;
-- +goose StatementEnd
