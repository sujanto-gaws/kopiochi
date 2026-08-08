# Notification Module — Concept & Blueprint

Target repo: `sujanto-gaws/kopiochi`
Proposed location: `modules/notification/` (+ this doc under `docs/architectures/`)

---

## 1. Concept

A self-contained **notification capability** that other modules trigger indirectly and users
consume directly. It owns three things:

1. **An outbox** — `Enqueue` writes a `notifications` row; nothing is sent inside a request.
2. **A dispatcher** — a background worker owned by the module that claims pending rows and
   pushes them through pluggable channel senders (email, in-app, webhook).
3. **A read model** — HTTP endpoints for a user's in-app notifications and per-channel
   preferences.

Why an outbox instead of direct sending:

- Request latency never depends on SMTP or a third-party webhook.
- A crash between "business action" and "notification" loses nothing — the row is already
  committed.
- Retry with backoff is a natural state machine on the row, not ad-hoc goroutine logic.
- `FOR UPDATE SKIP LOCKED` claiming is safe when you run two replicas — same reasoning the
  README applies to migrations.

## 2. Position in the architecture

All existing rules hold with zero exceptions:

```
cmd/api (BuildApp)  ──builds──►  modules/notification
        │                              │ imports internal/** (allowed)
        │                              │ never imports modules/** (enforced)
        └──adapts──► identity.SecurityNotifier ◄──declared by── modules/identity
```

**Cross-module trigger pattern** (the `user`-takes-auth-middleware pattern, reused):

- `modules/identity` declares, in its *own* package, the narrowest interface it needs:

  ```go
  // modules/identity/application/notifier.go
  type SecurityNotifier interface {
      PasswordChanged(ctx context.Context, userID string) error
      NewLoginDetected(ctx context.Context, userID string, meta LoginMeta) error
  }
  ```

- `BuildApp` satisfies it with a tiny adapter over notification's `Enqueue` use case:

  ```go
  // cmd/api/container.go
  notifMod, notifSvc, err := notification.New(deps, cfg.Notification)
  idMod, err := identity.New(deps, identity.Config{
      Notifier: notificationadapter.ForIdentity(notifSvc), // adapter lives in cmd/api
      ...
  })
  ```

- Neither module imports the other; `tools/archtest` and `depguard` stay green.
- A `NoopNotifier` default keeps identity constructible in tests and when the module is
  disabled.

## 3. Directory blueprint

```
modules/notification/
├── module.go                     # New(deps module.Deps, cfg Config) (*module.Module, *application.Service, error)
├── config.go                     # typed Config + Validate(); fails closed
├── domain/
│   ├── notification.go           # entity, Status/Channel/Category value types, transitions
│   ├── preference.go             # per-user, per-channel, per-category opt-in/out
│   ├── errors.go                 # ErrNotFound, ErrInvalidTransition, ErrDuplicateIdempotencyKey
│   └── repository.go             # NotificationRepository, PreferenceRepository interfaces
├── application/
│   ├── service.go                # Enqueue, ListForUser, MarkRead, MarkAllRead, GetPreferences, UpdatePreferences
│   ├── dispatch.go               # DispatchBatch use case (claim → render → send → settle)
│   ├── ports.go                  # ChannelSender, TemplateRenderer, Clock interfaces
│   └── dto.go                    # EnqueueRequest, NotificationView, PreferenceView
├── infrastructure/
│   ├── persistence/
│   │   ├── model.go              # bun models (schemacheck-compatible)
│   │   ├── notification_repo.go  # incl. ClaimBatch with FOR UPDATE SKIP LOCKED
│   │   └── preference_repo.go
│   ├── sender/
│   │   ├── email_smtp.go         # net/smtp or go-mail; password via platform/secret
│   │   ├── inapp.go              # marks row delivered; the row *is* the notification
│   │   ├── webhook.go            # http.Client, hard timeout, no redirects
│   │   └── log.go                # dev/test sender: writes to zerolog, always succeeds
│   ├── template/
│   │   ├── renderer.go           # html/template + text/template over embed.FS
│   │   └── templates/            # *.subject.tmpl, *.html.tmpl, *.text.tmpl
│   └── dispatcher/
│       └── dispatcher.go         # ticker loop, batch claim, jittered backoff, Stop()
└── transport/
    ├── handler.go
    ├── routes.go                 # Routes(r chi.Router) — wrapped in injected auth middleware
    └── dto.go                    # request/response JSON shapes + validation
```

Layer import rules are unchanged: `domain` → stdlib + `internal/platform` only;
`application` → own domain; `infrastructure` → domain + `internal/**`;
`transport` → application + domain.

## 4. Domain model

### 4.1 Notification entity

| Field             | Type        | Notes                                              |
| ----------------- | ----------- | -------------------------------------------------- |
| `ID`              | uuid        |                                                    |
| `RecipientID`     | uuid        | user id; no FK into user module's table semantics — just an id |
| `Channel`         | enum        | `email` \| `inapp` \| `webhook`                    |
| `Category`        | enum        | `security` \| `account` \| `system` (extensible)   |
| `TemplateKey`     | string      | e.g. `security.password_changed`                   |
| `Payload`         | jsonb       | template data; never rendered content              |
| `Status`          | enum        | `pending` → `sending` → `sent` \| `failed` \| `dead`; `failed` → `pending` on retry |
| `Attempts`        | int         |                                                    |
| `NextAttemptAt`   | timestamptz | dispatcher claim predicate                         |
| `IdempotencyKey`  | text, null  | unique when present; enqueue is upsert-do-nothing  |
| `ReadAt`          | timestamptz | in-app only                                        |
| `LastError`       | text        | truncated; for ops, never shown to users           |
| `CreatedAt/SentAt`| timestamptz |                                                    |

**Invariants (enforced in the entity, ≥90% coverage floor applies):**

- Transitions only along the arrows above; anything else returns `ErrInvalidTransition`.
- `failed` with `Attempts >= MaxAttempts` transitions to `dead`, terminal.
- `Backoff(n) = min(base * 2^n, cap) + jitter` — pure function, unit-testable.
- Payload is data-only. Rendering happens at dispatch time so template fixes apply to
  queued rows.

### 4.2 Preference entity

`(UserID, Channel, Category) → Enabled`, defaulting to enabled when no row exists.
One rule with teeth: **`security` category cannot be disabled for `email`** — preferences
are a filter at enqueue time, and password-changed mails must not be filterable.

## 5. Ports (application layer)

```go
type ChannelSender interface {
    Channel() domain.Channel
    Send(ctx context.Context, msg RenderedMessage) error
}

type TemplateRenderer interface {
    Render(key string, channel domain.Channel, payload map[string]any) (RenderedMessage, error)
}

type Clock interface { Now() time.Time }   // deterministic backoff tests
```

`DispatchBatch` is the whole worker logic as a plain, testable use case:

```
claim N rows (status=pending, next_attempt_at <= now, FOR UPDATE SKIP LOCKED, mark sending)
for each: render → sender[channel].Send → settle(sent | failed+backoff | dead)
```

Unknown template key or missing sender for a channel ⇒ row goes straight to `dead` with a
descriptive `LastError` — retrying a config bug wastes attempts.

## 6. Dispatcher (infrastructure)

- Started inside `notification.New`; stopped by `Module.Close` (the existing contract —
  the composition root already registers `Close` on the lifecycle stack).
- `poll_interval` ticker, `workers` concurrent batches, context-aware shutdown: stop
  claiming, let in-flight sends finish, then return.
- Emits `internal/metrics` counters: `notification_sent_total{channel}`,
  `notification_failed_total{channel}`, `notification_dead_total{channel}`, and a
  dispatch-latency histogram.
- Delivery failures at `dead` also emit an `internal/audit` event for `security`-category
  notifications — a user not receiving a password-changed mail is a security signal.

## 7. Transport — route table

All routes require the injected access-token middleware; there is deliberately **no public
send endpoint** — enqueue is an internal capability reached only through adapters.

| Method | Endpoint                               | Description                                  |
| ------ | -------------------------------------- | -------------------------------------------- |
| GET    | `/api/v1/notifications`                | List caller's in-app notifications; `?unread=true`, cursor pagination |
| POST   | `/api/v1/notifications/{id}/read`      | Mark one read (404 if not caller's)          |
| POST   | `/api/v1/notifications/read-all`       | Mark all read                                |
| GET    | `/api/v1/notifications/preferences`    | Effective preference matrix                  |
| PUT    | `/api/v1/notifications/preferences`    | Update; rejects disabling protected combos with a problem+json 422 |

Errors use the existing problem+json helpers in `internal/httpx`.

## 8. Config

```yaml
notification:
  enabled: true
  dispatcher:
    poll_interval: "5s"
    batch_size: 50
    workers: 2
    max_attempts: 6
    backoff_base: "30s"
    backoff_cap: "1h"
  email:
    enabled: false            # off by default, like CORS/rate-limit
    smtp_host: ""
    smtp_port: 587
    from: ""
    # password: APP_NOTIFICATION_EMAIL_PASSWORD (env only, secret.String — never in YAML)
  webhook:
    enabled: false
    timeout: "10s"
```

`Validate()` fails closed at boot:

- `email.enabled` with empty host/from/password ⇒ error.
- `backoff_base > backoff_cap`, `batch_size <= 0`, `workers <= 0` ⇒ error.
- `notification.New` returns an error if routes would mount without auth middleware
  (mirrors `user.New`).
- `enabled: false` ⇒ `New` returns a module with no routes and no dispatcher, and
  `BuildApp` wires `NoopNotifier` into identity.

Env overrides follow the existing convention: `APP_NOTIFICATION_DISPATCHER_BATCH_SIZE`, etc.

## 9. Migrations (Goose)

`migrations/<ts>_create_notifications.sql`:

```sql
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
```

CI's migrations job (up → full down → up) and `schemacheck` cover drift automatically once
the bun models exist.

## 10. Wiring into BuildApp

```go
// cmd/api/container.go
notifMod, notifSvc, err := notification.New(module.Deps{DB: db, Logger: log}, cfg.Notification)
if err != nil { return nil, fmt.Errorf("notification module: %w", err) }

idMod, err := identity.New(deps, identity.Config{
    Notifier: adaptNotifier(notifSvc), // or identity.NoopNotifier{} when disabled
    // ...existing config
})

app.Modules = append(app.Modules, idMod, userMod, notifMod)
```

`adaptNotifier` is ~15 lines in `cmd/api` translating identity's intent into
`application.EnqueueRequest` (template key, category, idempotency key like
`pwchange:<userID>:<eventID>`). `cmd/**` is the only place allowed to see both sides.

Note: `New` returning `(*module.Module, *application.Service, error)` is a deliberate,
minor extension of the current one-return convention — the service handle is what adapters
in `cmd/api` consume. Alternative: export a `notification.Service` interface and keep the
module constructor uniform.

## 11. Testing plan

| Target | Approach | Floor |
| --- | --- | --- |
| `domain` | Pure unit tests: transitions, backoff math, preference defaults | 90% |
| `application` | Fake repos/senders/clock; DispatchBatch happy path, retry, dead-lettering, idempotent enqueue | 80% |
| `infrastructure/persistence` | `testsupport.ScratchPostgres`; **two concurrent ClaimBatch calls must not double-claim** (the SKIP LOCKED test — run under `-race`, so effectively CI-only) | — |
| `transport` | httptest against real router slice + fake service; 401 without token, 404 cross-user access | — |
| archtest | Nothing to add — the walker picks the module up automatically; run `make arch` (`-count=1`) | — |

Add the new packages to `tools/coverage/policy.json` with reasons, per existing policy.

## 12. Implementation order

1. Migration + bun models + `schemacheck` green.
2. `domain/` complete with tests (transitions, backoff, preferences).
3. `application/` Enqueue + DispatchBatch against fakes.
4. `infrastructure/persistence` with the ClaimBatch concurrency test.
5. `sender/log.go` + `template/` — end-to-end deliverable in dev with zero external deps.
6. `dispatcher/` + `module.go` + Config validation; wire into `BuildApp`.
7. `transport/` routes + `TestRouteTable` update in `cmd/api/routes_test.go`.
8. `sender/email_smtp.go`, then identity's `SecurityNotifier` + adapter.
9. Metrics + audit hooks; coverage policy entries; Swagger annotations.

Steps 1–7 ship a working in-app notification system with no external infrastructure;
email and cross-module triggers layer on top.

## 13. Deliberate non-goals (v1)

- **No push/SMS** — add as new `ChannelSender` implementations later; the port is the
  extension point.
- **No user-facing send API** — prevents the module becoming an open relay.
- **No Kafka/queue dependency** — Postgres outbox + SKIP LOCKED covers this scale; the
  application layer wouldn't change if a queue replaced the poller later.
- **No WebSocket/SSE delivery** — clients poll `GET /notifications`; real-time is a
  separate transport concern that can wrap the same read model.
