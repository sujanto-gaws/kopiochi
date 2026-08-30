package notification_test

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/sujanto-gaws/kopiochi/internal/platform/secret"
	"github.com/sujanto-gaws/kopiochi/modules/notification"
)

// stubResolver stands in for the composition root's adapter over the identity
// module's user table. Config only cares that one is present.
type stubResolver struct{}

func (stubResolver) ResolveEmail(context.Context, uuid.UUID) (string, error) {
	return "someone@example.test", nil
}

// stubAuth stands in for the composition root's access-token middleware.
// Config only cares that one is present; what it does is transport's business
// and modules/notification/transport tests it with the real shape.
func stubAuth(next http.Handler) http.Handler { return next }

// validConfig is the shipped default set, as config/default.yaml spells it,
// plus the two values that file cannot carry: the SMTP password is env-only,
// and the auth middleware is a dependency rather than a setting.
// Every case below is this minus one thing, so a test says exactly which
// setting it is about.
func validConfig() notification.Config {
	return notification.Config{
		Enabled: true,
		Auth:    stubAuth,
		Dispatcher: notification.DispatcherConfig{
			PollInterval: 5 * time.Second,
			BatchSize:    50,
			Workers:      2,
			MaxAttempts:  6,
			BackoffBase:  30 * time.Second,
			BackoffCap:   time.Hour,
			StalledAfter: 5 * time.Minute,
			DrainTimeout: 30 * time.Second,
		},
	}
}

func withEmail(c notification.Config) notification.Config {
	c.Email = notification.EmailConfig{
		Enabled:  true,
		SMTPHost: "smtp.example.test",
		SMTPPort: 587,
		From:     "no-reply@example.test",
		Password: secret.String("a-real-credential"),
		Timeout:  10 * time.Second,
	}
	c.EmailAddressResolver = stubResolver{}
	return c
}

func TestConfigValidate(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		cfg  notification.Config
		// want is a substring of the expected error; empty means the config
		// must be accepted.
		want string
	}{
		{name: "the shipped defaults", cfg: validConfig()},
		{name: "the shipped defaults with email configured", cfg: withEmail(validConfig())},

		// A disabled module constructs nothing, so there is nothing for these
		// values to be wrong about. Refusing to boot over the tuning of a
		// component that does not exist would be a failure with no fault
		// behind it.
		{
			name: "disabled, and every value nonsense",
			cfg: notification.Config{
				Enabled:    false,
				Dispatcher: notification.DispatcherConfig{PollInterval: -1, BatchSize: -1, Workers: -1},
				Email:      notification.EmailConfig{Enabled: true},
				LogSender:  notification.LogSenderConfig{Enabled: true, Channel: "carrier-pigeon"},
			},
		},

		// The one requirement that is not about tuning. Routes that mount
		// without a middleware are routes served to anonymous callers, so an
		// enabled module with no Auth is refused for the same reason user.New
		// refuses one — and refused at boot, where a human is reading.
		{
			name: "enabled with no auth middleware",
			cfg:  mutate(func(c *notification.Config) { c.Auth = nil }),
			want: "auth middleware is required",
		},

		{
			name: "zero poll interval",
			cfg:  mutate(func(c *notification.Config) { c.Dispatcher.PollInterval = 0 }),
			want: "poll_interval must be positive",
		},
		{
			name: "zero batch size claims nothing, forever",
			cfg:  mutate(func(c *notification.Config) { c.Dispatcher.BatchSize = 0 }),
			want: "batch_size must be positive",
		},
		{
			name: "zero workers",
			cfg:  mutate(func(c *notification.Config) { c.Dispatcher.Workers = 0 }),
			want: "workers must be positive",
		},
		{
			name: "max attempts below one makes the first failure fatal",
			cfg:  mutate(func(c *notification.Config) { c.Dispatcher.MaxAttempts = 0 }),
			want: "max_attempts must be positive",
		},
		{
			name: "zero backoff base",
			cfg:  mutate(func(c *notification.Config) { c.Dispatcher.BackoffBase = 0 }),
			want: "backoff_base must be positive",
		},
		{
			name: "zero backoff cap",
			cfg:  mutate(func(c *notification.Config) { c.Dispatcher.BackoffCap = 0 }),
			want: "backoff_cap must be positive",
		},
		{
			// The clamp fires on the first retry and the exponential never
			// happens, so the schedule an operator wrote is not the one they
			// get.
			name: "backoff cap below the base",
			cfg: mutate(func(c *notification.Config) {
				c.Dispatcher.BackoffBase = time.Hour
				c.Dispatcher.BackoffCap = time.Minute
			}),
			want: "must not exceed dispatcher.backoff_cap",
		},
		{
			name: "zero stall window",
			cfg:  mutate(func(c *notification.Config) { c.Dispatcher.StalledAfter = 0 }),
			want: "stalled_after must be positive",
		},
		{
			// Declaring rows abandoned faster than a worker can finish them
			// turns the recovery sweep into a machine for burning attempts on
			// healthy deliveries.
			name: "stall window shorter than one poll",
			cfg: mutate(func(c *notification.Config) {
				c.Dispatcher.PollInterval = time.Minute
				c.Dispatcher.StalledAfter = time.Second
			}),
			want: "must be at least dispatcher.poll_interval",
		},
		{
			name: "zero drain timeout",
			cfg:  mutate(func(c *notification.Config) { c.Dispatcher.DrainTimeout = 0 }),
			want: "drain_timeout must be positive",
		},

		// Email fails closed on all four. Email switched on with nowhere to
		// connect does not degrade into "no email" — it degrades into a
		// dispatcher that dead-letters every security notification.
		{
			name: "email enabled with no host",
			cfg:  mutateEmail(func(e *notification.EmailConfig) { e.SMTPHost = "" }),
			want: "email.smtp_host is required",
		},
		{
			name: "email enabled with a whitespace host",
			cfg:  mutateEmail(func(e *notification.EmailConfig) { e.SMTPHost = "   " }),
			want: "email.smtp_host is required",
		},
		{
			name: "email enabled with no port",
			cfg:  mutateEmail(func(e *notification.EmailConfig) { e.SMTPPort = 0 }),
			want: "email.smtp_port",
		},
		{
			name: "email enabled with an out-of-range port",
			cfg:  mutateEmail(func(e *notification.EmailConfig) { e.SMTPPort = 70000 }),
			want: "email.smtp_port",
		},
		{
			name: "email enabled with no from address",
			cfg:  mutateEmail(func(e *notification.EmailConfig) { e.From = "" }),
			want: "email.from is required",
		},
		{
			name: "email enabled with a from address that is not one",
			cfg:  mutateEmail(func(e *notification.EmailConfig) { e.From = "No Reply" }),
			want: "must be an email address",
		},
		{
			name: "email enabled with no password",
			cfg:  mutateEmail(func(e *notification.EmailConfig) { e.Password = "" }),
			want: "APP_NOTIFICATION_EMAIL_PASSWORD",
		},
		{
			// A sender with no timeout parks a dispatcher worker on a TCP
			// handshake until the kernel gives up, which is minutes.
			name: "email enabled with no timeout",
			cfg:  mutateEmail(func(e *notification.EmailConfig) { e.Timeout = 0 }),
			want: "email.timeout must be positive",
		},
		{
			// Without it every email notification dead-letters on its first
			// attempt: the sender has a mail server and no way to address a
			// message.
			name: "email enabled with no address resolver",
			cfg: func() notification.Config {
				c := withEmail(validConfig())
				c.EmailAddressResolver = nil
				return c
			}(),
			want: "address resolver is required",
		},
		{
			// The resolver is a dependency of the email sender and nothing
			// else, so a deployment that does not send email owes none.
			name: "email disabled and no resolver",
			cfg:  validConfig(),
		},
		{
			// The whole email block is inert when it is off, including the
			// credential it does not have.
			name: "email disabled and unconfigured",
			cfg:  validConfig(),
		},

		{
			name: "log sender on an unknown channel",
			cfg: mutate(func(c *notification.Config) {
				c.LogSender = notification.LogSenderConfig{Enabled: true, Channel: "carrier-pigeon"}
			}),
			want: "is not a known channel",
		},
		{
			// The in-app row is the notification; its sender is unconditional,
			// and two senders for one channel is a wiring bug with no correct
			// resolution.
			name: "log sender claiming the in-app channel",
			cfg: mutate(func(c *notification.Config) {
				c.LogSender = notification.LogSenderConfig{Enabled: true, Channel: "inapp"}
			}),
			want: "cannot be \"inapp\"",
		},
		{
			name: "log sender claiming email while SMTP is on",
			cfg: func() notification.Config {
				c := withEmail(validConfig())
				c.LogSender = notification.LogSenderConfig{Enabled: true, Channel: "email"}
				return c
			}(),
			want: "one channel cannot have two senders",
		},
		{
			name: "log sender claiming email while SMTP is off",
			cfg: mutate(func(c *notification.Config) {
				c.LogSender = notification.LogSenderConfig{Enabled: true, Channel: "email"}
			}),
		},
		{
			name: "log sender disabled with a nonsense channel",
			cfg: mutate(func(c *notification.Config) {
				c.LogSender = notification.LogSenderConfig{Enabled: false, Channel: "carrier-pigeon"}
			}),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := tc.cfg.Validate()
			switch {
			case tc.want == "" && err != nil:
				t.Fatalf("Validate() = %v, want nil", err)
			case tc.want != "" && err == nil:
				t.Fatalf("Validate() = nil, want an error mentioning %q", tc.want)
			case tc.want != "" && !strings.Contains(err.Error(), tc.want):
				t.Fatalf("Validate() = %v, want it to mention %q", err, tc.want)
			}
		})
	}
}

// Every fault is reported, not just the first. An operator fixing a config one
// boot at a time is an operator who reboots four times.
func TestConfigValidateReportsEveryFaultAtOnce(t *testing.T) {
	t.Parallel()

	cfg := validConfig()
	cfg.Dispatcher.Workers = 0
	cfg.Dispatcher.BatchSize = 0
	cfg.Email = notification.EmailConfig{Enabled: true, SMTPPort: 587, Timeout: time.Second}
	cfg.EmailAddressResolver = stubResolver{}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("Validate() = nil")
	}
	for _, want := range []string{"workers", "batch_size", "smtp_host", "email.from", "APP_NOTIFICATION_EMAIL_PASSWORD"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error does not mention %q:\n%v", want, err)
		}
	}
}

func mutate(fn func(*notification.Config)) notification.Config {
	c := validConfig()
	fn(&c)
	return c
}

func mutateEmail(fn func(*notification.EmailConfig)) notification.Config {
	c := withEmail(validConfig())
	fn(&c.Email)
	return c
}
