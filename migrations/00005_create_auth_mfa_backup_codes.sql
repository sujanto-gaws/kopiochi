-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS auth_mfa_backup_codes (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL REFERENCES auth_users(id) ON DELETE CASCADE,
    code_hash TEXT NOT NULL,
    used BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- MFAStore.FindAndUseBackupCode fetches all unused codes for a user
-- (WHERE user_id = ? AND used = false), so user_id needs an index the same
-- way the FK-referencing tables above do.
CREATE INDEX IF NOT EXISTS idx_auth_mfa_backup_codes_user_id ON auth_mfa_backup_codes(user_id);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_auth_mfa_backup_codes_user_id;
DROP TABLE IF EXISTS auth_mfa_backup_codes;
-- +goose StatementEnd
