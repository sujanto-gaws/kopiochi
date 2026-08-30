package notification

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/sujanto-gaws/kopiochi/internal/platform/secret"
	domain "github.com/sujanto-gaws/kopiochi/modules/notification/domain"
)

// Config is the notification module's settings, as the composition root
// supplies them.
//
// It mirrors the notification section of internal/config field for field, the
// same way identity.Config mirrors config.Auth. The duplication is deliberate:
// this struct is the module's contract with whoever builds it, and a module
// that read the host's config type directly would make every other host —
// a test, a second binary — carry the whole application config to construct
// one module.
//
// The mapstructure tags document the YAML keys each field comes from. Nothing
// unmarshals into this type today; the composition root maps the fields
// explicitly, which is what keeps a rename in the host config a compile error
// here rather than a silently-zeroed field.
type Config struct {
	// Enabled gates the entire module. False means no dispatcher, no senders,
	// and no routes — see New.
	Enabled bool `mapstructure:"enabled"`

	Dispatcher DispatcherConfig `mapstructure:"dispatcher"`
	Email      EmailConfig      `mapstructure:"email"`
	LogSender  LogSenderConfig  `mapstructure:"log_sender"`
}

// DispatcherConfig tunes the background worker that drains the outbox.
type DispatcherConfig struct {
	// PollInterval is how often each worker looks for claimable rows, and how
	// often the stalled-row sweep runs.
	PollInterval time.Duration `mapstructure:"poll_interval"`

	// BatchSize is how many rows one claim takes. A cycle that claims a full
	// batch immediately claims again rather than waiting for the next tick, so
	// this bounds a single database round trip, not the drain rate.
	BatchSize int `mapstructure:"batch_size"`

	// Workers is how many claim-render-send loops run concurrently. Claiming
	// is exclusive in the database (FOR UPDATE SKIP LOCKED), so this is a
	// throughput knob and not a correctness one.
	Workers int `mapstructure:"workers"`

	// MaxAttempts is the retry budget. A row that has failed this many times
	// is dead-lettered instead of rescheduled.
	MaxAttempts int `mapstructure:"max_attempts"`

	// BackoffBase is the wait after the first failure; BackoffCap bounds the
	// doubling. Jitter is applied inside the cap, so a retry never waits
	// longer than BackoffCap.
	BackoffBase time.Duration `mapstructure:"backoff_base"`
	BackoffCap  time.Duration `mapstructure:"backoff_cap"`

	// StalledAfter is how long a row may sit in sending before the sweep
	// treats it as abandoned and recovers it.
	//
	// It is a timeout on a whole delivery attempt, so it must be comfortably
	// longer than the slowest sender: recovering a row that is merely slow
	// costs it an attempt and can dead-letter it. Five minutes is the default
	// because an SMTP conversation that has not finished in five minutes is
	// not going to.
	StalledAfter time.Duration `mapstructure:"stalled_after"`

	// DrainTimeout bounds shutdown. Stop stops claiming immediately and then
	// waits this long for in-flight deliveries to settle their rows.
	//
	// It exists because module.Module.Close takes no context and therefore
	// cannot inherit the process's shutdown deadline (see New). Without a
	// bound of its own, one wedged send would hang the whole shutdown until
	// the operator's second SIGINT.
	DrainTimeout time.Duration `mapstructure:"drain_timeout"`
}

// EmailConfig describes SMTP delivery.
//
// Enabled is what says whether this deployment can send email at all. When it
// is false no email sender is registered, so an enqueue for the email channel
// is refused at the producer with application.ErrChannelNotRoutable rather than
// accepted and quietly dropped — the distinction E13 settled.
type EmailConfig struct {
	Enabled  bool   `mapstructure:"enabled"`
	SMTPHost string `mapstructure:"smtp_host"`
	SMTPPort int    `mapstructure:"smtp_port"`

	// From is the envelope and header sender address.
	From string `mapstructure:"from"`

	// Password is env-only: APP_NOTIFICATION_EMAIL_PASSWORD. It never appears
	// in a YAML file, and secret.String keeps it out of logs and JSON — it
	// redacts itself in every formatting verb, so the value is readable only
	// through Reveal, which the SMTP sender calls at dial time and nowhere
	// else.
	Password secret.String `mapstructure:"password"`
}

// LogSenderConfig wires the development sender, which writes a rendered
// message to the log instead of delivering it.
//
// It is off by default and names its channel explicitly, because "log it
// instead" must be something an operator asks for by name. A fallback — "no
// SMTP configured, so log the mail" — would settle rows as sent while nothing
// was sent, which is the silent drop wearing the costume of a durable outbox
// that E13 removed from Enqueue.
type LogSenderConfig struct {
	Enabled bool `mapstructure:"enabled"`

	// Channel is the channel the log sender claims, e.g. "email". It must not
	// be a channel that already has a real sender; Validate refuses the
	// overlap rather than letting registration order decide.
	Channel string `mapstructure:"channel"`
}

// Validate rejects a configuration the module cannot honestly run under. New
// calls it, so a bad value fails the boot that reads it rather than the first
// notification that needs it.
//
// A disabled module validates trivially. Nothing downstream is constructed —
// no dispatcher, no senders, no repositories — so there is nothing for these
// values to be wrong about, and refusing to start over the tuning of a
// component that does not exist would be a failure with no fault behind it.
func (c Config) Validate() error {
	if !c.Enabled {
		return nil
	}

	var errs []error
	errs = append(errs, c.Dispatcher.validate()...)
	errs = append(errs, c.Email.validate()...)
	errs = append(errs, c.validateLogSender()...)

	return errors.Join(errs...)
}

func (d DispatcherConfig) validate() []error {
	var errs []error

	if d.PollInterval <= 0 {
		errs = append(errs, errors.New("notification: dispatcher.poll_interval must be positive"))
	}
	if d.BatchSize <= 0 {
		errs = append(errs, errors.New("notification: dispatcher.batch_size must be positive"))
	}
	if d.Workers <= 0 {
		errs = append(errs, errors.New("notification: dispatcher.workers must be positive"))
	}
	// Below one, the first failure is fatal — which the domain honours
	// literally and which no operator means to configure.
	if d.MaxAttempts <= 0 {
		errs = append(errs, errors.New("notification: dispatcher.max_attempts must be positive"))
	}
	if d.BackoffBase <= 0 {
		errs = append(errs, errors.New("notification: dispatcher.backoff_base must be positive"))
	}
	if d.BackoffCap <= 0 {
		errs = append(errs, errors.New("notification: dispatcher.backoff_cap must be positive"))
	}
	// A cap below the base is not a slower schedule, it is a shorter one: the
	// clamp fires on the very first retry and the exponential never happens,
	// so the tuning an operator wrote is silently not the tuning they get.
	if d.BackoffBase > 0 && d.BackoffCap > 0 && d.BackoffBase > d.BackoffCap {
		errs = append(errs, fmt.Errorf(
			"notification: dispatcher.backoff_base (%s) must not exceed dispatcher.backoff_cap (%s)",
			d.BackoffBase, d.BackoffCap))
	}
	if d.StalledAfter <= 0 {
		errs = append(errs, errors.New("notification: dispatcher.stalled_after must be positive"))
	}
	// A stall window shorter than the poll interval declares rows abandoned
	// faster than a worker can plausibly finish them, which turns the recovery
	// sweep into a machine for burning attempts on healthy deliveries.
	if d.StalledAfter > 0 && d.PollInterval > 0 && d.StalledAfter < d.PollInterval {
		errs = append(errs, fmt.Errorf(
			"notification: dispatcher.stalled_after (%s) must be at least dispatcher.poll_interval (%s)",
			d.StalledAfter, d.PollInterval))
	}
	if d.DrainTimeout <= 0 {
		errs = append(errs, errors.New("notification: dispatcher.drain_timeout must be positive"))
	}

	return errs
}

func (e EmailConfig) validate() []error {
	if !e.Enabled {
		return nil
	}

	var errs []error

	// Fail closed, all three of them. Email that is switched on with nowhere
	// to connect, no sender address, or no credential does not degrade into
	// "no email" — it degrades into a dispatcher that dead-letters every
	// security notification at 3am, which is precisely when nobody is reading
	// the log that would say why.
	if strings.TrimSpace(e.SMTPHost) == "" {
		errs = append(errs, errors.New("notification: email.smtp_host is required when email.enabled is true"))
	}
	if e.SMTPPort <= 0 || e.SMTPPort > 65535 {
		errs = append(errs, fmt.Errorf("notification: email.smtp_port (%d) must be between 1 and 65535", e.SMTPPort))
	}
	if strings.TrimSpace(e.From) == "" {
		errs = append(errs, errors.New("notification: email.from is required when email.enabled is true"))
	} else if !strings.Contains(e.From, "@") {
		// Not an address validator — that argument has no winner. It catches
		// the one mistake worth catching at boot: a display name or a
		// hostname pasted where an address belongs, which every SMTP server
		// rejects at MAIL FROM, one notification at a time.
		errs = append(errs, fmt.Errorf("notification: email.from (%q) must be an email address", e.From))
	}
	if e.Password.IsEmpty() {
		errs = append(errs, errors.New("notification: email password is required when email.enabled is true; set APP_NOTIFICATION_EMAIL_PASSWORD"))
	}

	return errs
}

func (c Config) validateLogSender() []error {
	if !c.LogSender.Enabled {
		return nil
	}

	channel := domain.Channel(c.LogSender.Channel)
	if !channel.Valid() {
		return []error{fmt.Errorf("notification: log_sender.channel (%q) is not a known channel", c.LogSender.Channel)}
	}

	// Two senders for one channel is a wiring bug with no correct resolution,
	// and application.NewService refuses it. Catching it here names the
	// setting that caused it instead of reporting an anonymous duplicate from
	// deep inside construction.
	switch {
	case channel == domain.ChannelInApp:
		return []error{errors.New("notification: log_sender.channel cannot be \"inapp\"; the in-app row is the notification and always has its own sender")}
	case channel == domain.ChannelEmail && c.Email.Enabled:
		return []error{errors.New("notification: log_sender.channel is \"email\" while email.enabled is true; one channel cannot have two senders")}
	}

	return nil
}
