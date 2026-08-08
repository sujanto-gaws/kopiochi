-- +goose Up
CREATE TABLE notifications (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    recipient_id     uuid        NOT NULL,
    channel          text        NOT NULL,
    category         text        NOT NULL,
    template_key     text        NOT NULL,
    payload          jsonb       NOT NULL DEFAULT '{}',
    status           text        NOT NULL DEFAULT 'pending',
    attempts         int         NOT NULL DEFAULT 0,
    next_attempt_at  timestamptz NOT NULL DEFAULT now(),
    idempotency_key  text,
    read_at          timestamptz,
    last_error       text,
    created_at       timestamptz NOT NULL DEFAULT now(),
    sent_at          timestamptz,
    CONSTRAINT notifications_status_chk
        CHECK (status IN ('pending','sending','sent','failed','dead'))
);

-- dispatcher claim path: small partial index, exactly the claim predicate
CREATE INDEX idx_notifications_claim
    ON notifications (next_attempt_at)
    WHERE status = 'pending';

-- user-facing list path
CREATE INDEX idx_notifications_recipient
    ON notifications (recipient_id, created_at DESC)
    WHERE channel = 'inapp';

CREATE UNIQUE INDEX idx_notifications_idem
    ON notifications (idempotency_key)
    WHERE idempotency_key IS NOT NULL;

CREATE TABLE notification_preferences (
    user_id    uuid NOT NULL,
    channel    text NOT NULL,
    category   text NOT NULL,
    enabled    boolean NOT NULL DEFAULT true,
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, channel, category)
);

-- +goose Down
DROP TABLE notification_preferences;
DROP TABLE notifications;
