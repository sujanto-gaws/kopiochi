package application

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	domain "github.com/sujanto-gaws/kopiochi/modules/notification/domain"
)

// Service is the notification module's use cases. One type rather than one per
// use case, because they share the same four dependencies and the read model is
// three lines of orchestration each.
type Service struct {
	notifications domain.NotificationRepository
	preferences   domain.PreferenceRepository
	renderer      TemplateRenderer
	senders       map[domain.Channel]ChannelSender
	clock         Clock
	jitter        domain.JitterSource
	cfg           DispatchConfig
}

// NewService assembles the service, refusing a dependency set that would fail
// later instead of now.
//
// senders is a slice and not a map: a ChannelSender already declares its own
// channel, so building the routing table here from ChannelSender.Channel()
// makes a registry key that disagrees with the sender under it unexpressible.
// Two senders claiming one channel is a wiring bug with no correct resolution,
// so it is refused. An empty slice is legal — a deployment with email switched
// off has no email sender, and rows for that channel dead-letter with a
// descriptive reason, which is exactly the intended behaviour.
//
// jitter may be nil, which means an unjittered retry schedule; see
// domain.BackoffWithJitter. Everything else is required. A nil clock or
// repository would otherwise surface as a nil dereference inside a background
// worker hours after boot, which is the worst place to discover that the
// composition root missed an argument.
func NewService(
	notifications domain.NotificationRepository,
	preferences domain.PreferenceRepository,
	renderer TemplateRenderer,
	senders []ChannelSender,
	clock Clock,
	jitter domain.JitterSource,
	cfg DispatchConfig,
) (*Service, error) {
	if notifications == nil {
		return nil, errors.New("notification: notification repository is required")
	}
	if preferences == nil {
		return nil, errors.New("notification: preference repository is required")
	}
	if renderer == nil {
		return nil, errors.New("notification: template renderer is required")
	}
	if clock == nil {
		return nil, errors.New("notification: clock is required")
	}

	registry := make(map[domain.Channel]ChannelSender, len(senders))
	for i, sender := range senders {
		if sender == nil {
			return nil, fmt.Errorf("notification: sender %d is nil", i)
		}
		ch := sender.Channel()
		if !ch.Valid() {
			return nil, fmt.Errorf("notification: sender %d claims unknown channel %q", i, ch)
		}
		if _, dup := registry[ch]; dup {
			return nil, fmt.Errorf("notification: two senders registered for channel %q", ch)
		}
		registry[ch] = sender
	}

	return &Service{
		notifications: notifications,
		preferences:   preferences,
		renderer:      renderer,
		senders:       registry,
		clock:         clock,
		jitter:        jitter,
		cfg:           cfg,
	}, nil
}

// ErrChannelNotRoutable is returned by Enqueue for a channel that is valid in
// the domain but has no sender wired into this Service.
//
// It is a configuration or contract error, never a user-facing one, and the two
// causes want different responses from a caller that maps it: a producer asking
// for a channel this deployment does not implement (webhook is a v1 non-goal)
// is the producer's bug, while a channel that should be wired and is not is the
// operator's. Both are worth failing loudly; neither is worth a dead row.
var ErrChannelNotRoutable = errors.New("notification: no sender registered for channel")

// Enqueue writes an intent to notify into the outbox. Nothing is sent here; the
// dispatcher drains the row later.
//
// Two outcomes are deliberately reported as success with nothing written:
//
//   - The user has this (channel, category) pair switched off. They asked not
//     to receive it, and the caller did nothing wrong.
//   - The idempotency key is already in the outbox. "This notification was
//     already queued" is exactly the outcome the caller asked for, and the
//     caller cannot distinguish the first enqueue from the second — which is
//     the point of the key.
//
// An unroutable channel is NOT one of them: see ErrChannelNotRoutable.
//
// The preference gate goes through domain.Allowed rather than reading
// Preference.Enabled here. Allowed applies the protected-pair override before
// it looks at any stored row, so a rogue "security email: off" row cannot
// suppress a password-changed mail. A scan of the slice in this package would
// silently reintroduce that hole and nothing would report it.
func (s *Service) Enqueue(ctx context.Context, req EnqueueRequest) error {
	// Validate before touching the database: an unknown channel is the
	// caller's bug and does not deserve a preference lookup.
	n, err := domain.NewNotification(domain.NewNotificationParams{
		// The row id is generated here rather than taken from the caller. The
		// domain takes it as a parameter so that construction is deterministic,
		// and this is the layer that decides: an outbox id is an internal
		// handle, and the reproducible identity of an enqueue is its
		// idempotency key, not its uuid.
		ID:             uuid.New(),
		RecipientID:    req.RecipientID,
		Channel:        req.Channel,
		Category:       req.Category,
		TemplateKey:    req.TemplateKey,
		Payload:        req.Payload,
		IdempotencyKey: req.IdempotencyKey,
	}, s.clock.Now())
	if err != nil {
		return err
	}

	// A channel nobody can deliver is refused HERE, at the producer, rather
	// than accepted and killed later at the dispatch gate.
	//
	// domain.Channel accepts email, inapp and webhook, but only the channels
	// with a registered sender can actually be delivered: deliver() treats an
	// unrouted channel as non-retryable, so such a row goes pending -> dead
	// having never been attempted, and the only trace is a LastError nobody is
	// watching. That is a silent drop wearing the costume of a durable outbox.
	// See E13 — it was found as "every in-app notification dies at the D6
	// gate", but the shape is general and would have recurred verbatim for
	// webhook.
	//
	// Checked before the preference lookup on purpose: an unroutable channel is
	// a broken contract between this service and whoever wired it, and it stays
	// broken whether or not this particular user wanted the message.
	if _, routable := s.senders[n.Channel]; !routable {
		return fmt.Errorf("%w: %q", ErrChannelNotRoutable, n.Channel)
	}

	prefs, err := s.preferences.ListForUser(ctx, n.RecipientID)
	if err != nil {
		return fmt.Errorf("load preferences: %w", err)
	}
	if !domain.Allowed(prefs, n.Channel, n.Category) {
		return nil
	}

	if err := s.notifications.Enqueue(ctx, n); err != nil {
		if errors.Is(err, domain.ErrDuplicateIdempotencyKey) {
			return nil
		}
		return fmt.Errorf("enqueue notification: %w", err)
	}
	return nil
}

// ListForUser returns one page of userID's in-app mailbox, newest first.
//
// f is passed through untouched: normalising the limit is the repository's
// documented job, and doing it in both places means two bounds that can drift
// apart.
func (s *Service) ListForUser(ctx context.Context, userID uuid.UUID, f domain.ListFilter) ([]NotificationView, error) {
	rows, err := s.notifications.ListForRecipient(ctx, userID, f)
	if err != nil {
		return nil, fmt.Errorf("list notifications: %w", err)
	}

	views := make([]NotificationView, 0, len(rows))
	for _, n := range rows {
		views = append(views, NotificationView{
			ID:          n.ID,
			Category:    n.Category,
			TemplateKey: n.TemplateKey,
			Payload:     n.Payload,
			CreatedAt:   n.CreatedAt,
			ReadAt:      n.ReadAt,
		})
	}
	return views, nil
}

// MarkRead marks one of userID's notifications read. It returns
// domain.ErrNotFound when the id does not exist or belongs to someone else —
// the two are indistinguishable on purpose, so the endpoint cannot be used to
// probe for ids.
func (s *Service) MarkRead(ctx context.Context, userID, id uuid.UUID) error {
	if err := s.notifications.MarkRead(ctx, userID, id, s.clock.Now()); err != nil {
		return fmt.Errorf("mark notification read: %w", err)
	}
	return nil
}

// MarkAllRead marks every unread in-app notification of userID read and reports
// how many rows changed.
func (s *Service) MarkAllRead(ctx context.Context, userID uuid.UUID) (int, error) {
	n, err := s.notifications.MarkAllRead(ctx, userID, s.clock.Now())
	if err != nil {
		return 0, fmt.Errorf("mark all notifications read: %w", err)
	}
	return n, nil
}

// GetPreferences returns the effective preference matrix: every (channel,
// category) pair with the answer the system will actually give at enqueue time.
//
// Every cell is resolved through domain.Allowed, over the full cross product of
// domain.Channels() and domain.Categories(). Two consequences, both wanted.
// Pairs with no stored row appear as enabled rather than missing, so the caller
// never has to know what absence means. And a rogue stored row disabling a
// protected pair is reported as enabled, because that is what will happen —
// a matrix built by reading Enabled off the stored rows would tell a user
// "security email: off" while the system correctly kept sending.
func (s *Service) GetPreferences(ctx context.Context, userID uuid.UUID) ([]PreferenceView, error) {
	prefs, err := s.preferences.ListForUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("load preferences: %w", err)
	}

	channels := domain.Channels()
	categories := domain.Categories()

	views := make([]PreferenceView, 0, len(channels)*len(categories))
	for _, ch := range channels {
		for _, cat := range categories {
			views = append(views, PreferenceView{
				Channel:  ch,
				Category: cat,
				Enabled:  domain.Allowed(prefs, ch, cat),
			})
		}
	}
	return views, nil
}

// UpdatePreferences applies updates and returns the resulting effective matrix.
//
// Every update is validated before any of them is written. The repository
// writes the slice as one unit, so a request that is half-legal must be
// refused whole rather than land half-applied — a user who disabled three
// categories and tried to disable a protected fourth would otherwise have to
// guess which three took effect.
//
// Refusals come from the domain constructor: an unknown channel or category is
// domain.ErrInvalidPreference, and disabling the protected pair is
// domain.ErrPreferenceProtected, which transport turns into a 422.
func (s *Service) UpdatePreferences(ctx context.Context, userID uuid.UUID, updates []PreferenceUpdate) ([]PreferenceView, error) {
	now := s.clock.Now()

	type pair struct {
		ch  domain.Channel
		cat domain.Category
	}
	seen := make(map[pair]struct{}, len(updates))

	prefs := make([]domain.Preference, 0, len(updates))
	for _, u := range updates {
		// One request naming the same pair twice has no single correct
		// outcome, and an upsert asked to write one row twice in one statement
		// is an error at the database rather than a last-one-wins. Refused
		// here, where it is still the caller's mistake and not a 500.
		key := pair{u.Channel, u.Category}
		if _, dup := seen[key]; dup {
			return nil, fmt.Errorf("%w: %s/%s named twice", domain.ErrInvalidPreference, u.Channel, u.Category)
		}
		seen[key] = struct{}{}

		p, err := domain.NewPreference(userID, u.Channel, u.Category, u.Enabled, now)
		if err != nil {
			return nil, err
		}
		prefs = append(prefs, p)
	}

	if err := s.preferences.Upsert(ctx, prefs); err != nil {
		return nil, fmt.Errorf("save preferences: %w", err)
	}

	// Re-read rather than compute from updates: the answer the caller renders
	// must be the effective matrix, including the pairs this request did not
	// touch and the protected override.
	return s.GetPreferences(ctx, userID)
}
