-- Refresh-token families, for rotation with reuse detection (Phase 5.6).
--
-- Rotation alone is not enough. If a refresh token is stolen, both the thief
-- and the legitimate user hold a valid token; whoever refreshes second
-- presents one that has already been used. Without a family there is nothing
-- to revoke at that point except that single token, so the attacker simply
-- continues with the one they were issued.
--
-- A family is the chain of tokens descending from one login. Detecting reuse
-- anywhere in the chain revokes the whole chain, which logs out both parties
-- and forces a real re-authentication.

-- +goose Up
-- +goose StatementBegin

-- Existing rows each become their own single-token family. gen_random_uuid()
-- as the default means the backfill is implicit and every historical token
-- gets a distinct family rather than sharing one — sharing would make the
-- first reuse detection revoke every session issued before this migration.
ALTER TABLE auth_refresh_tokens
    ADD COLUMN IF NOT EXISTS family_id uuid NOT NULL DEFAULT gen_random_uuid();

-- used_at records when a token was exchanged. It is what distinguishes "this
-- token was rotated away normally" from "this token was never presented", and
-- the reuse check needs that distinction: presenting a token that already has
-- a used_at is the theft signal.
ALTER TABLE auth_refresh_tokens
    ADD COLUMN IF NOT EXISTS used_at TIMESTAMPTZ;

-- Set when a family is revoked because reuse was detected, as opposed to an
-- ordinary logout. Kept separate from `revoked` so an operator can tell a
-- security event from a user action after the fact.
ALTER TABLE auth_refresh_tokens
    ADD COLUMN IF NOT EXISTS reuse_detected_at TIMESTAMPTZ;

-- The reuse path revokes by family, so this index is on the hot path of every
-- detection — and detection happens while an attacker is active.
CREATE INDEX IF NOT EXISTS idx_auth_refresh_tokens_family_id
    ON auth_refresh_tokens(family_id);

-- token_hash must be unique: two rows with the same hash would make "which
-- token was presented" ambiguous, and the reuse check would then depend on
-- which row the planner happened to return. The existing non-unique index is
-- replaced rather than supplemented.
DROP INDEX IF EXISTS idx_auth_refresh_tokens_token_hash;
CREATE UNIQUE INDEX IF NOT EXISTS idx_auth_refresh_tokens_token_hash
    ON auth_refresh_tokens(token_hash);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

-- Restore the non-unique index this replaced, so Down genuinely returns the
-- schema to its previous shape rather than approximately.
DROP INDEX IF EXISTS idx_auth_refresh_tokens_token_hash;
CREATE INDEX IF NOT EXISTS idx_auth_refresh_tokens_token_hash
    ON auth_refresh_tokens(token_hash);

DROP INDEX IF EXISTS idx_auth_refresh_tokens_family_id;

ALTER TABLE auth_refresh_tokens DROP COLUMN IF EXISTS reuse_detected_at;
ALTER TABLE auth_refresh_tokens DROP COLUMN IF EXISTS used_at;
ALTER TABLE auth_refresh_tokens DROP COLUMN IF EXISTS family_id;

-- +goose StatementEnd
