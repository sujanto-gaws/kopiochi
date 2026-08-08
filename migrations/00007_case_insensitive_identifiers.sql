-- Case-insensitive login identifiers.
--
-- Both user tables previously enforced uniqueness on the raw column, and both
-- repositories looked rows up with a plain "=". That let Alice@example.com and
-- alice@example.com exist as two separate accounts, and made a login fail for
-- anyone who typed their address with different capitalisation than they
-- registered with. auth_users.username had no uniqueness at all, so
-- FindByUsername could match more than one row.
--
-- The fix is a functional unique index on lower(col), paired with
-- "WHERE lower(col) = lower(?)" in the repositories. That form is preferred
-- over normalising to lowercase on write because the original capitalisation
-- stays intact for display, no existing row has to be rewritten, and the
-- planner still uses the index — the indexed expression matches the predicate
-- exactly.
--
-- Down restores the raw-column uniqueness. It is not a lossless inverse in the
-- data sense: rows that could only be created while the case-insensitive index
-- was in place are still there afterwards, they just stop being rejected.

-- +goose Up

-- Fail loudly and specifically if the data already contains pairs that differ
-- only by case. CREATE UNIQUE INDEX would fail here anyway, but with a generic
-- "could not create unique index" that names neither the column nor the rows
-- to reconcile. Merging those accounts is a decision for whoever owns the
-- data, not something a migration should make silently.
-- +goose StatementBegin
DO $$
DECLARE
    conflict text;
BEGIN
    SELECT string_agg(format('%s.%s = %L (%s rows)', t, c, v, n), '; ')
      INTO conflict
      FROM (
        SELECT 'auth_users' AS t, 'email' AS c, lower(email) AS v, count(*) AS n
          FROM auth_users GROUP BY lower(email) HAVING count(*) > 1
        UNION ALL
        SELECT 'auth_users', 'username', lower(username), count(*)
          FROM auth_users GROUP BY lower(username) HAVING count(*) > 1
        UNION ALL
        SELECT 'users', 'email', lower(email), count(*)
          FROM users GROUP BY lower(email) HAVING count(*) > 1
      ) dupes;

    IF conflict IS NOT NULL THEN
        RAISE EXCEPTION
            'cannot enforce case-insensitive identifiers: existing rows collide once case is ignored: %',
            conflict
        USING HINT = 'merge or rename the colliding accounts, then re-run this migration';
    END IF;
END
$$;
-- +goose StatementEnd

-- auth_users: replace the case-sensitive unique index from 00003, and add the
-- username uniqueness FindByUsername has always assumed but never had.
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_auth_users_email;

CREATE UNIQUE INDEX IF NOT EXISTS idx_auth_users_email_lower
    ON auth_users (lower(email));

CREATE UNIQUE INDEX IF NOT EXISTS idx_auth_users_username_lower
    ON auth_users (lower(username));
-- +goose StatementEnd

-- users: the uniqueness here came from an inline UNIQUE on the column in
-- 00001, which Postgres named users_email_key. idx_users_email was a separate
-- non-unique lookup index over the same column; the functional index replaces
-- both, since it serves the lookup and the constraint at once.
-- +goose StatementBegin
ALTER TABLE users DROP CONSTRAINT IF EXISTS users_email_key;

DROP INDEX IF EXISTS idx_users_email;

CREATE UNIQUE INDEX IF NOT EXISTS idx_users_email_lower
    ON users (lower(email));
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_users_email_lower;

ALTER TABLE users ADD CONSTRAINT users_email_key UNIQUE (email);

CREATE INDEX IF NOT EXISTS idx_users_email ON users (email);
-- +goose StatementEnd

-- +goose StatementBegin
DROP INDEX IF EXISTS idx_auth_users_username_lower;

DROP INDEX IF EXISTS idx_auth_users_email_lower;

CREATE UNIQUE INDEX IF NOT EXISTS idx_auth_users_email ON auth_users (email);
-- +goose StatementEnd
