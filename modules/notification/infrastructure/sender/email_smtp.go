package sender

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"net/smtp"
	"net/textproto"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/sujanto-gaws/kopiochi/internal/platform/secret"
	domain "github.com/sujanto-gaws/kopiochi/modules/notification/domain"
)

// AddressResolver turns a recipient id into an email address.
//
// It is declared HERE, by the consumer, and satisfied at the composition root
// by an adapter over the identity module's user repository (E15, R2). The
// notification module holds no foreign key into another module's tables and
// learns nothing about what a user is; it asks a question and receives a
// string.
//
// This is the mirror image of E11's ruling and not a contradiction of it.
// RenderedMessage lives in domain because a sender must NAME a type it
// implements against; this interface is one a sender CONSUMES, so the narrow
// interface belongs to the consumer and domain stays clean.
type AddressResolver interface {
	// ResolveEmail returns the address to deliver to.
	//
	// The error decides the row's fate, so implementations owe exactly one
	// distinction: a recipient who does not exist, or has no address, returns
	// an error wrapping ErrNoAddress, and everything else — a dropped
	// connection, a timeout — returns a plain error and is retried. An
	// implementation that flattens the two either destroys a security mail on
	// a blip, or retries a deleted user until its budget is gone.
	ResolveEmail(ctx context.Context, recipientID uuid.UUID) (string, error)
}

// ErrNoAddress reports that the recipient has no address to deliver to: the id
// names nobody, or names somebody with no email on file.
//
// It wraps domain.ErrNonRetryable because no number of attempts creates a user.
// It is a sentinel of this port rather than a storage error because the
// resolver is an adapter over anything at all, and this package must not have
// to know what internal/db calls a missing row.
var ErrNoAddress = fmt.Errorf("%w: recipient has no email address", domain.ErrNonRetryable)

// Clock reports the current time. It exists so the Date header of a built
// message is a literal in tests rather than whatever time.Now said.
type Clock interface{ Now() time.Time }

// SMTPConfig is everything the sender needs to reach a mail server.
type SMTPConfig struct {
	Host string
	Port int

	// Username is the SMTP AUTH identity. It is separate from From because
	// relays disagree: a mailbox provider authenticates as the mailbox, while
	// SES, SendGrid and Mailgun issue a credential that is not an address at
	// all. Empty means "authenticate as From", which is the mailbox-provider
	// case and the one most people configure.
	Username string
	Password secret.String

	// From is the envelope sender and the From header. It is parsed at
	// construction, so a malformed address fails the boot rather than every
	// delivery.
	From string

	// Timeout bounds the whole SMTP conversation — dial, handshake, and the
	// commands after it.
	//
	// net/smtp is not context-aware, so this becomes a deadline on the
	// connection. Without it an unreachable host blocks a dispatcher worker
	// until the operating system gives up on the TCP handshake, which is
	// minutes, and the stalled-row sweep exists to recover from crashes rather
	// than to paper over a sender with no timeout.
	Timeout time.Duration

	// ImplicitTLS starts the connection with a TLS handshake instead of a
	// plaintext greeting ("SMTPS"). Use PortUsesImplicitTLS to derive it.
	//
	// It is a field here rather than a port comparison inside deliver so that
	// the branch is reachable from a test: 465 is a privileged port and no
	// test can listen on it, and an untested TLS path in the one sender that
	// carries a credential is not a trade worth making.
	ImplicitTLS bool

	// TLSConfig overrides the defaults used for STARTTLS and for implicit TLS.
	// nil means a secure default whose ServerName is Host. It exists for a
	// deployment with a private CA — and for the tests here, which point it at
	// a server on loopback.
	TLSConfig *tls.Config
}

// smtpsPort is the registered port for implicit TLS ("SMTPS").
const smtpsPort = 465

// PortUsesImplicitTLS reports whether a connection to port starts with a TLS
// handshake rather than a plaintext greeting.
//
// The transport is inferred from the port rather than configured, because that
// is what every mail client does and because the alternative is a third
// setting whose only correct value is a function of the second. A deployment
// needing implicit TLS on some other port is the case that would justify an
// explicit setting; nobody has one.
func PortUsesImplicitTLS(port int) bool { return port == smtpsPort }

// SMTPDeps are the collaborators the sender cannot build for itself.
type SMTPDeps struct {
	// Resolver is required: a message with no address cannot be sent.
	Resolver AddressResolver

	// Clock is optional; nil means the system clock.
	Clock Clock
}

// SMTP delivers over the email channel through an SMTP server.
//
// It never decides a row's fate by prose. Every failure is classified by the
// SMTP reply code or by a sentinel — see classify — so that "the mailbox does
// not exist" dead-letters immediately and "the server is busy" is retried, and
// neither depends on a message string a server operator can change.
type SMTP struct {
	cfg  SMTPConfig
	deps SMTPDeps

	// from is cfg.From parsed once. Keeping the parsed form is what lets the
	// header and the envelope agree without re-parsing per message.
	from *mail.Address

	// messageIDDomain is the domain half of from, used to make the Message-ID
	// a valid address. See build.
	messageIDDomain string
}

// NewSMTP validates the configuration and returns a sender.
//
// Everything that can be wrong before a message exists is refused here: no
// host, no credential, an unparsable From, no resolver. The alternative is a
// deployment that boots, looks healthy, and dead-letters its first security
// mail.
func NewSMTP(deps SMTPDeps, cfg SMTPConfig) (*SMTP, error) {
	var errs []error

	if deps.Resolver == nil {
		errs = append(errs, errors.New("smtp sender: an address resolver is required"))
	}
	if strings.TrimSpace(cfg.Host) == "" {
		errs = append(errs, errors.New("smtp sender: host is required"))
	}
	if cfg.Port <= 0 || cfg.Port > 65535 {
		errs = append(errs, fmt.Errorf("smtp sender: port (%d) must be between 1 and 65535", cfg.Port))
	}
	if cfg.Password.IsEmpty() {
		errs = append(errs, errors.New("smtp sender: password is required"))
	}
	if cfg.Timeout <= 0 {
		errs = append(errs, errors.New("smtp sender: timeout must be positive"))
	}

	from, err := mail.ParseAddress(cfg.From)
	if err != nil {
		errs = append(errs, fmt.Errorf("smtp sender: from address %q: %w", cfg.From, err))
	}
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}

	_, domainPart, _ := strings.Cut(from.Address, "@")

	return &SMTP{cfg: cfg, deps: deps, from: from, messageIDDomain: domainPart}, nil
}

// Channel reports domain.ChannelEmail.
func (s *SMTP) Channel() domain.Channel { return domain.ChannelEmail }

// Send resolves the recipient, builds the message and delivers it.
//
// The three steps fail differently on purpose. Resolution reports whether the
// recipient exists; building fails only on data no retry can repair; delivery
// is classified by the server's reply code.
func (s *SMTP) Send(ctx context.Context, msg domain.RenderedMessage) error {
	to, err := s.resolve(ctx, msg.RecipientID)
	if err != nil {
		return err
	}

	raw, err := s.build(msg, to)
	if err != nil {
		return err
	}

	return s.deliver(ctx, to.Address, raw)
}

// resolve asks the resolver for an address and parses it.
//
// The parse is not a formality. The address comes from another module's table,
// and a value containing a newline would end the To header and let whatever
// follows be read as another one — a Bcc, a second Reply-To. net/mail refuses
// it, and so does every other malformed address, non-retryably: no retry
// repairs a stored value.
func (s *SMTP) resolve(ctx context.Context, recipientID uuid.UUID) (*mail.Address, error) {
	raw, err := s.deps.Resolver.ResolveEmail(ctx, recipientID)
	if err != nil {
		// Passed through unchanged: the resolver already classified it, and
		// re-deciding here would overrule the only component that knows
		// whether the lookup failed or the recipient is gone.
		return nil, fmt.Errorf("resolve recipient %s: %w", recipientID, err)
	}
	if strings.TrimSpace(raw) == "" {
		return nil, fmt.Errorf("%w: resolver returned an empty address for %s", ErrNoAddress, recipientID)
	}

	addr, err := mail.ParseAddress(raw)
	if err != nil {
		// The address itself is not repeated into the error: it is another
		// user's personal data and this string ends up in LastError, which
		// operators read.
		return nil, fmt.Errorf("%w: recipient %s has an unusable address: %v", domain.ErrNonRetryable, recipientID, err)
	}
	return addr, nil
}

// build renders msg as an RFC 5322 message.
//
// Two forms, and only two: multipart/alternative when the message has an HTML
// part, and text/plain when it does not. There is deliberately no third form
// with HTML alone — an HTML-only mail is penalised by spam filters and
// unreadable to a text client, and the mail most likely to be filtered is the
// security mail. The renderer already makes such a family unbuildable (E14);
// this is the same rule at the other end of the pipe, because the rule matters
// more than the layer that happens to enforce it.
func (s *SMTP) build(msg domain.RenderedMessage, to *mail.Address) ([]byte, error) {
	if strings.TrimSpace(msg.Body) == "" {
		return nil, fmt.Errorf("%w: message %s has no plain-text body", domain.ErrNonRetryable, msg.NotificationID)
	}

	var buf bytes.Buffer
	headers := []header{
		{"From", s.from.String()},
		{"To", to.String()},
		{"Subject", mime.QEncoding.Encode("utf-8", oneLineSubject(msg.Subject))},
		{"Date", s.now().Format(time.RFC1123Z)},
		// Stable across retries, and derived from the notification id rather
		// than random: at-least-once delivery means a receiver may see this
		// message twice, and an identical Message-ID is what lets a mail
		// client collapse the copies instead of showing two.
		{"Message-ID", fmt.Sprintf("<%s@%s>", msg.NotificationID, s.messageIDDomain)},
		// RFC 3834. Tells vacation responders and ticket systems not to reply
		// to a machine, which is how a password-changed mail becomes a support
		// ticket loop.
		{"Auto-Submitted", "auto-generated"},
		{"MIME-Version", "1.0"},
	}

	if msg.HTMLBody == "" {
		headers = append(headers,
			header{"Content-Type", `text/plain; charset="utf-8"`},
			header{"Content-Transfer-Encoding", "quoted-printable"},
		)
		writeHeaders(&buf, headers)
		if err := writeQuotedPrintable(&buf, msg.Body); err != nil {
			return nil, err
		}
		return buf.Bytes(), nil
	}

	// The parts go least-preferred first, which RFC 2046 defines as the
	// ordering a client reads: it takes the last part it can display. Plain
	// text before HTML is therefore what makes the HTML win where it can and
	// the text survive where it cannot.
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)

	if err := writePart(mw, `text/plain; charset="utf-8"`, msg.Body); err != nil {
		return nil, err
	}
	if err := writePart(mw, `text/html; charset="utf-8"`, msg.HTMLBody); err != nil {
		return nil, err
	}
	if err := mw.Close(); err != nil {
		return nil, fmt.Errorf("close multipart message: %w", err)
	}

	headers = append(headers, header{"Content-Type", `multipart/alternative; boundary="` + mw.Boundary() + `"`})
	writeHeaders(&buf, headers)
	buf.Write(body.Bytes())

	return buf.Bytes(), nil
}

// deliver runs the SMTP conversation.
//
// net/smtp takes no context, so the budget is applied as a deadline on the
// connection: one deadline for the whole exchange, set before the greeting is
// read. ctx's own deadline wins when it is the shorter of the two, which is how
// a shutdown that is already past its drain timeout stops waiting on a mail
// server.
func (s *SMTP) deliver(ctx context.Context, to string, raw []byte) error {
	addr := net.JoinHostPort(s.cfg.Host, fmt.Sprint(s.cfg.Port))

	// time.Now, deliberately, and not the injected clock. A deadline is an
	// instruction to the network stack about real elapsed time; an injected
	// clock exists so the Date header is a literal in a test, and one fixed in
	// the past would expire this connection before its first read.
	deadline := time.Now().Add(s.cfg.Timeout)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}

	dialer := net.Dialer{Timeout: s.cfg.Timeout}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		// Retryable, and this is the case the default exists for: an
		// unreachable mail server is the most ordinary transient failure there
		// is, and dead-lettering on it would empty the outbox during an
		// outage.
		return fmt.Errorf("dial %s: %w", addr, err)
	}
	defer func() { _ = conn.Close() }()

	if err := conn.SetDeadline(deadline); err != nil {
		return fmt.Errorf("set deadline: %w", err)
	}

	if s.cfg.ImplicitTLS {
		tlsConn := tls.Client(conn, s.tlsConfig())
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			return fmt.Errorf("tls handshake with %s: %w", addr, err)
		}
		conn = tlsConn
	}

	c, err := smtp.NewClient(conn, s.cfg.Host)
	if err != nil {
		return classify(fmt.Errorf("smtp greeting from %s: %w", addr, err))
	}
	defer func() { _ = c.Close() }()

	if !s.cfg.ImplicitTLS {
		// STARTTLS when the server offers it. When it does not, the
		// conversation continues in the clear and net/smtp's PlainAuth refuses
		// to hand over the credential unless the server is loopback — which is
		// the correct policy and the reason it is not re-implemented here.
		if ok, _ := c.Extension("STARTTLS"); ok {
			if err := c.StartTLS(s.tlsConfig()); err != nil {
				return fmt.Errorf("starttls with %s: %w", addr, err)
			}
		}
	}

	username := s.cfg.Username
	if username == "" {
		username = s.from.Address
	}
	// Reveal, here and nowhere else in this module: the credential is a
	// secret.String everywhere it is stored, carried and logged, and becomes a
	// string only as an argument to the authentication that needs it.
	if err := c.Auth(smtp.PlainAuth("", username, s.cfg.Password.Reveal(), s.cfg.Host)); err != nil {
		return classify(fmt.Errorf("smtp auth as %s: %w", username, err))
	}

	if err := c.Mail(s.from.Address); err != nil {
		return classify(fmt.Errorf("smtp mail from: %w", err))
	}
	if err := c.Rcpt(to); err != nil {
		return classify(fmt.Errorf("smtp rcpt to: %w", err))
	}

	w, err := c.Data()
	if err != nil {
		return classify(fmt.Errorf("smtp data: %w", err))
	}
	if _, err := w.Write(raw); err != nil {
		return fmt.Errorf("write message: %w", err)
	}
	// The close is what sends the terminating dot and reads the server's
	// verdict, so its error is the delivery's, not a cleanup detail.
	if err := w.Close(); err != nil {
		return classify(fmt.Errorf("send message: %w", err))
	}

	// Quit failing after the server has accepted the message is not a failed
	// delivery — the mail is sent, and reporting a failure here would deliver
	// it a second time on the retry.
	_ = c.Quit()
	return nil
}

// classify decides whether another attempt could plausibly succeed.
//
// The rule is the reply code and only the reply code: 5xx is permanent by
// definition (RFC 5321), so a rejected recipient (550) and a refused
// credential (535) dead-letter at once, while 4xx and everything without a
// code — a dropped connection, a TLS failure, a timeout — are retried.
//
// Never the message text. Server operators write those, they differ between
// implementations, and a substring match is a classification that changes
// when somebody edits a string in a mail server.
func classify(err error) error {
	var protoErr *textproto.Error
	if errors.As(err, &protoErr) && protoErr.Code >= 500 {
		return fmt.Errorf("%w: %w", domain.ErrNonRetryable, err)
	}
	return err
}

// tlsConfig returns the configured TLS settings, or a default pinned to the
// configured host. Never an empty config: an empty one has no ServerName, and
// verification would fail or, worse, be skipped.
func (s *SMTP) tlsConfig() *tls.Config {
	if s.cfg.TLSConfig != nil {
		return s.cfg.TLSConfig
	}
	return &tls.Config{ServerName: s.cfg.Host, MinVersion: tls.VersionTLS12}
}

func (s *SMTP) now() time.Time {
	if s.deps.Clock != nil {
		return s.deps.Clock.Now()
	}
	return time.Now()
}

type header struct{ name, value string }

// writeHeaders writes the header block and the blank line that ends it, with
// CRLF endings — the only line ending RFC 5322 allows.
func writeHeaders(buf *bytes.Buffer, headers []header) {
	for _, h := range headers {
		fmt.Fprintf(buf, "%s: %s\r\n", h.name, h.value)
	}
	buf.WriteString("\r\n")
}

// writePart writes one multipart/alternative part.
func writePart(mw *multipart.Writer, contentType, body string) error {
	h := textproto.MIMEHeader{}
	h.Set("Content-Type", contentType)
	h.Set("Content-Transfer-Encoding", "quoted-printable")

	w, err := mw.CreatePart(h)
	if err != nil {
		return fmt.Errorf("create %s part: %w", contentType, err)
	}
	return writeQuotedPrintable(w, body)
}

// writeQuotedPrintable encodes body.
//
// quoted-printable rather than 8bit for two reasons that both bite silently:
// SMTP allows no line longer than 998 octets, and a rendered notification can
// contain a long URL; and a server that has not advertised 8BITMIME may mangle
// non-ASCII, which is every accented name in a greeting.
func writeQuotedPrintable(w interface{ Write([]byte) (int, error) }, body string) error {
	qp := quotedprintable.NewWriter(w)
	if _, err := qp.Write([]byte(body)); err != nil {
		return fmt.Errorf("encode body: %w", err)
	}
	return qp.Close()
}

// oneLineSubject removes the line breaks a header cannot carry.
//
// The renderer already collapses subjects (D5), and this does it again on
// purpose: header injection is the failure this prevents, the renderer is not
// the only thing that can produce a RenderedMessage, and the check costs
// nothing.
func oneLineSubject(s string) string { return strings.Join(strings.Fields(s), " ") }
