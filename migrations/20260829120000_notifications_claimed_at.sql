-- +goose Up
-- claimed_at records when the dispatcher took the row, and exists so a stalled
-- send can be told apart from a healthy in-flight one (E9b).
--
-- Why a new column rather than reusing next_attempt_at. ClaimBatch sets only
-- status, so a claimed row keeps its PRE-claim next_attempt_at and is
-- indistinguishable from one that stalled. The cheap-looking alternative was to
-- have ClaimBatch write next_attempt_at = now() on claim, and it was rejected
-- twice over: it writes to the very column the claim predicate reads, so a
-- mistake there changes which rows are deliverable rather than just which are
-- reported; and it gives one column two meanings selected by another column —
-- "not before this" while pending, "claimed at this" while sending.
--
-- It is also the timestamp dispatch latency is measured from: sent_at minus
-- claimed_at, durable and queryable rather than an in-process timer (E12).
-- Overwriting next_attempt_at on every retry could never have provided that.
--
-- Nullable with no default and no backfill: NULL means "never claimed", which
-- is the truth for every existing row. A default of now() would have claimed
-- the entire outbox at migration time.
ALTER TABLE notifications ADD COLUMN claimed_at timestamptz;

-- The stalled sweep's predicate, mirroring idx_notifications_claim's shape for
-- the pending side. Partial, because only sending rows are ever swept.
CREATE INDEX IF NOT EXISTS idx_notifications_stalled
    ON notifications (claimed_at)
    WHERE status = 'sending';

-- +goose Down
DROP INDEX IF EXISTS idx_notifications_stalled;
ALTER TABLE notifications DROP COLUMN claimed_at;
