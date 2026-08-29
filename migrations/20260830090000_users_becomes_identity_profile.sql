-- +goose Up
-- Reshape `users` into the profile OF an identity, keyed by auth_users.id.
-- E16-ARCH, E20, E22.
--
-- Before this, `users.id` was a BIGSERIAL unrelated to any identity, and there
-- was no value a handler could compare a caller against — which is why the IDOR
-- (E16) was not a missing `if` but a missing column. Afterwards the profile's
-- primary key IS the caller's subject, so ownership is expressible at all.
--
-- name and email are dropped (E20). Every column of `users` already existed on
-- auth_users, and the profile's copies had exactly one consumer: the CRUD JSON
-- echoing back what was posted.
--
-- 00001 is NOT edited. ADR-010: a mistake is corrected by a new migration.

-- The guard, and the reason this migration is correct whether or not any
-- environment holds rows (E22).
--
-- On an empty table this is a no-op and the result is indistinguishable from a
-- clean break. On a populated one it refuses to continue rather than silently
-- discard a profile it cannot attach to an identity — the same choice 00007
-- made about colliding identifiers, for the same reason: there is no correct
-- automatic answer, so the migration stops and says which rows are the problem.
--
-- The join is on lower(email), and BOTH sides carry a UNIQUE index on it
-- (idx_auth_users_email_lower and idx_users_email_lower, 00007), so a match is
-- at most 1:1 in each direction and an unmatched row is the only failure mode.
--
-- This is NOT B2 being overturned. B2 refused lower(email) as a PER-REQUEST
-- authorization decision against a mutable natural key, and that refusal
-- stands. This is a one-time, supervised data migration that fails closed.
-- +goose StatementBegin
DO $$
DECLARE
    orphans text;
BEGIN
    SELECT string_agg(quote_literal(u.email), ', ' ORDER BY u.email)
      INTO orphans
      FROM users u
      LEFT JOIN auth_users a ON lower(a.email) = lower(u.email)
     WHERE a.id IS NULL;

    IF orphans IS NOT NULL THEN
        RAISE EXCEPTION
            'cannot reshape users into an identity profile: no auth_users row matches %. '
            'Resolve these before migrating: create the identities, or delete the profiles.',
            orphans;
    END IF;
END
$$;
-- +goose StatementEnd

ALTER TABLE users ADD COLUMN identity_id uuid;

UPDATE users u
   SET identity_id = a.id
  FROM auth_users a
 WHERE lower(a.email) = lower(u.email);

ALTER TABLE users DROP CONSTRAINT users_pkey;
ALTER TABLE users DROP COLUMN id;
ALTER TABLE users RENAME COLUMN identity_id TO id;
ALTER TABLE users ALTER COLUMN id SET NOT NULL;
ALTER TABLE users ADD CONSTRAINT users_pkey PRIMARY KEY (id);

-- ON DELETE CASCADE, so deleting an identity takes its profile with it. The
-- profile is derived from the identity and cannot outlive it; a row here whose
-- auth_users row is gone would be unreachable, since the only way to address a
-- profile is as the authenticated caller.
ALTER TABLE users
    ADD CONSTRAINT users_identity_fk FOREIGN KEY (id) REFERENCES auth_users (id) ON DELETE CASCADE;

-- Dropping these also drops idx_users_email_lower, which 00007 created on
-- users(lower(email)). The Down below recreates it.
ALTER TABLE users DROP COLUMN name;
ALTER TABLE users DROP COLUMN email;

-- +goose Down
-- Restores the pre-reshape shape. name and email are recovered from auth_users,
-- which is where the reshape established they came from — so this is reversible
-- for the rows that exist, rather than reversible only on an empty table.
-- COALESCE because auth_users.name is nullable and users.name was NOT NULL.
ALTER TABLE users ADD COLUMN name  VARCHAR(255);
ALTER TABLE users ADD COLUMN email VARCHAR(255);

UPDATE users u
   SET name  = COALESCE(a.name, a.username),
       email = a.email
  FROM auth_users a
 WHERE a.id = u.id;

ALTER TABLE users ALTER COLUMN name  SET NOT NULL;
ALTER TABLE users ALTER COLUMN email SET NOT NULL;

ALTER TABLE users DROP CONSTRAINT users_identity_fk;
ALTER TABLE users DROP CONSTRAINT users_pkey;
ALTER TABLE users RENAME COLUMN id TO identity_id;
ALTER TABLE users ADD COLUMN id BIGSERIAL;
ALTER TABLE users ADD CONSTRAINT users_pkey PRIMARY KEY (id);
ALTER TABLE users DROP COLUMN identity_id;

CREATE UNIQUE INDEX IF NOT EXISTS idx_users_email_lower ON users (lower(email));
