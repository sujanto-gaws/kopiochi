package sender_test

import (
	"bufio"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/sujanto-gaws/kopiochi/internal/platform/secret"
	domain "github.com/sujanto-gaws/kopiochi/modules/notification/domain"
	"github.com/sujanto-gaws/kopiochi/modules/notification/infrastructure/sender"
)

// These tests speak SMTP to a real server on loopback rather than mocking
// net/smtp. The claims worth making here — that a 550 dead-letters and a 451
// retries, that STARTTLS is attempted when advertised and the credential is
// not sent if it fails, that the bytes on the wire are a well-formed
// multipart/alternative — are all claims about the conversation. A mocked
// client would assert that the code called the functions the test expected.
//
// net/smtp's PlainAuth refuses to send a credential over an unencrypted
// connection unless the server is loopback, which is why these servers listen
// on 127.0.0.1 and why the sender does not re-implement that policy.

const (
	testFrom     = "no-reply@example.test"
	testPassword = "the-smtp-credential"
	testTo       = "ada@example.test"
)

// fakeSMTP is a scriptable SMTP server. It answers the commands net/smtp
// sends, records what it was given, and returns a scripted reply for any
// command prefix a test overrides.
type fakeSMTP struct {
	t *testing.T

	// replies maps a command verb ("RCPT", "AUTH", "DATA-END") to the reply
	// line the server sends instead of its default.
	replies map[string]string

	// advertiseSTARTTLS puts STARTTLS in the EHLO response.
	advertiseSTARTTLS bool

	listener net.Listener

	mu       sync.Mutex
	auth     string
	mailFrom string
	rcptTo   string
	data     string
	commands []string
}

func startFakeSMTP(t *testing.T, replies map[string]string, advertiseSTARTTLS bool) *fakeSMTP {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	s := &fakeSMTP{t: t, replies: replies, advertiseSTARTTLS: advertiseSTARTTLS, listener: ln}
	t.Cleanup(func() { _ = ln.Close() })

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go s.serve(conn)
		}
	}()

	return s
}

func (s *fakeSMTP) hostPort() (string, int) {
	addr := s.listener.Addr().(*net.TCPAddr)
	return "127.0.0.1", addr.Port
}

func (s *fakeSMTP) reply(verb, def string) string {
	if r, ok := s.replies[verb]; ok {
		return r
	}
	return def
}

func (s *fakeSMTP) serve(conn net.Conn) {
	defer func() { _ = conn.Close() }()

	w := bufio.NewWriter(conn)
	r := bufio.NewReader(conn)

	say := func(line string) bool {
		if _, err := w.WriteString(line + "\r\n"); err != nil {
			return false
		}
		return w.Flush() == nil
	}

	if !say("220 fake.example.test ESMTP") {
		return
	}

	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimRight(line, "\r\n")

		s.mu.Lock()
		s.commands = append(s.commands, line)
		s.mu.Unlock()

		verb, rest, _ := strings.Cut(line, " ")
		switch strings.ToUpper(verb) {
		case "EHLO":
			// A multiline reply: every line but the last uses "250-".
			if !say("250-fake.example.test") {
				return
			}
			if s.advertiseSTARTTLS && !say("250-STARTTLS") {
				return
			}
			if !say("250 AUTH PLAIN") {
				return
			}

		case "STARTTLS":
			// Never 220 here: a test that got that far would need a real
			// handshake. The scripted failure is what proves the sender
			// attempts the upgrade and stops when it cannot have it.
			if !say(s.reply("STARTTLS", "454 4.7.0 TLS not available right now")) {
				return
			}

		case "AUTH":
			s.mu.Lock()
			s.auth = rest
			s.mu.Unlock()
			if !say(s.reply("AUTH", "235 2.7.0 Authentication successful")) {
				return
			}

		case "MAIL":
			s.mu.Lock()
			s.mailFrom = rest
			s.mu.Unlock()
			if !say(s.reply("MAIL", "250 2.1.0 Ok")) {
				return
			}

		case "RCPT":
			s.mu.Lock()
			s.rcptTo = rest
			s.mu.Unlock()
			if !say(s.reply("RCPT", "250 2.1.5 Ok")) {
				return
			}

		case "DATA":
			if reply, ok := s.replies["DATA"]; ok {
				if !say(reply) {
					return
				}
				continue
			}
			if !say("354 End data with <CR><LF>.<CR><LF>") {
				return
			}

			var body strings.Builder
			for {
				dataLine, err := r.ReadString('\n')
				if err != nil {
					return
				}
				if dataLine == ".\r\n" || dataLine == ".\n" {
					break
				}
				body.WriteString(dataLine)
			}
			s.mu.Lock()
			s.data = body.String()
			s.mu.Unlock()

			if !say(s.reply("DATA-END", "250 2.0.0 Ok: queued as ABC123")) {
				return
			}

		case "QUIT":
			say("221 2.0.0 Bye")
			return

		default:
			if !say("500 5.5.2 Unrecognized command") {
				return
			}
		}
	}
}

func (s *fakeSMTP) captured() (mailFrom, rcptTo, data, auth string, commands []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.mailFrom, s.rcptTo, s.data, s.auth, append([]string(nil), s.commands...)
}

// staticResolver answers with one address, or one error.
type staticResolver struct {
	address string
	err     error
}

func (r staticResolver) ResolveEmail(context.Context, uuid.UUID) (string, error) {
	return r.address, r.err
}

type fixedClock struct{ now time.Time }

func (c fixedClock) Now() time.Time { return c.now }

var testClock = fixedClock{now: time.Date(2026, 8, 30, 9, 14, 0, 0, time.UTC)}

func smtpConfig(host string, port int) sender.SMTPConfig {
	return sender.SMTPConfig{
		Host:     host,
		Port:     port,
		Password: secret.String(testPassword),
		From:     testFrom,
		Timeout:  5 * time.Second,
	}
}

func newSMTP(t *testing.T, cfg sender.SMTPConfig, resolver sender.AddressResolver) *sender.SMTP {
	t.Helper()

	s, err := sender.NewSMTP(sender.SMTPDeps{Resolver: resolver, Clock: testClock}, cfg)
	if err != nil {
		t.Fatalf("NewSMTP: %v", err)
	}
	return s
}

func emailMessage() domain.RenderedMessage {
	return domain.RenderedMessage{
		NotificationID: uuid.MustParse("33333333-3333-3333-3333-333333333333"),
		RecipientID:    uuid.MustParse("44444444-4444-4444-4444-444444444444"),
		Channel:        domain.ChannelEmail,
		Subject:        "Your password was changed",
		Body:           "Your account password was changed on 30 August 2026 at 09:14 UTC.",
	}
}

// headersOf splits a captured message into its header block and body.
func headersOf(t *testing.T, raw string) (mail.Header, string) {
	t.Helper()

	msg, err := mail.ReadMessage(strings.NewReader(raw))
	if err != nil {
		t.Fatalf("the sender produced something that is not a message: %v\n%s", err, raw)
	}
	body, err := io.ReadAll(msg.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return msg.Header, string(body)
}

func decodeQP(t *testing.T, s string) string {
	t.Helper()

	out, err := io.ReadAll(quotedprintable.NewReader(strings.NewReader(s)))
	if err != nil {
		t.Fatalf("decode quoted-printable: %v", err)
	}
	return string(out)
}

func TestSMTPSendsAPlainTextMessage(t *testing.T) {
	t.Parallel()

	server := startFakeSMTP(t, nil, false)
	host, port := server.hostPort()

	msg := emailMessage()
	if err := newSMTP(t, smtpConfig(host, port), staticResolver{address: testTo}).Send(context.Background(), msg); err != nil {
		t.Fatalf("Send: %v", err)
	}

	mailFrom, rcptTo, raw, _, _ := server.captured()

	// The envelope, which is what actually routes the mail — a From header
	// that disagrees with MAIL FROM is how a message gets delivered to nobody.
	if want := "FROM:<" + testFrom + ">"; mailFrom != want {
		t.Errorf("MAIL %s, want %s", mailFrom, want)
	}
	if want := "TO:<" + testTo + ">"; rcptTo != want {
		t.Errorf("RCPT %s, want %s", rcptTo, want)
	}

	header, body := headersOf(t, raw)
	for field, want := range map[string]string{
		"From":           "<" + testFrom + ">",
		"To":             "<" + testTo + ">",
		"Subject":        msg.Subject,
		"Mime-Version":   "1.0",
		"Auto-Submitted": "auto-generated",
		"Content-Type":   `text/plain; charset="utf-8"`,
		// quoted-printable rather than 8bit: SMTP forbids a line over 998
		// octets and a rendered notification can carry a long URL.
		"Content-Transfer-Encoding": "quoted-printable",
	} {
		if got := header.Get(field); got != want {
			t.Errorf("%s = %q, want %q", field, got, want)
		}
	}

	// Stable across retries and derived from the row, so a receiver that gets
	// an at-least-once duplicate can collapse the copies.
	if want := fmt.Sprintf("<%s@example.test>", msg.NotificationID); header.Get("Message-Id") != want {
		t.Errorf("Message-ID = %q, want %q", header.Get("Message-Id"), want)
	}
	if _, err := header.Date(); err != nil {
		t.Errorf("Date is not a date: %v", err)
	}

	if got := decodeQP(t, body); strings.TrimSpace(got) != msg.Body {
		t.Errorf("body = %q, want %q", got, msg.Body)
	}
}

func TestSMTPSendsMultipartAlternativeWhenThereIsHTML(t *testing.T) {
	t.Parallel()

	server := startFakeSMTP(t, nil, false)
	host, port := server.hostPort()

	msg := emailMessage()
	msg.HTMLBody = "<html><body><p>Your account password was changed.</p></body></html>"

	if err := newSMTP(t, smtpConfig(host, port), staticResolver{address: testTo}).Send(context.Background(), msg); err != nil {
		t.Fatalf("Send: %v", err)
	}

	_, _, raw, _, _ := server.captured()
	header, body := headersOf(t, raw)

	mediaType, params, err := mime.ParseMediaType(header.Get("Content-Type"))
	if err != nil {
		t.Fatalf("Content-Type: %v", err)
	}
	if mediaType != "multipart/alternative" {
		t.Fatalf("Content-Type = %q, want multipart/alternative", mediaType)
	}

	mr := multipart.NewReader(strings.NewReader(body), params["boundary"])

	var types, bodies []string
	for {
		part, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("read part: %v", err)
		}
		raw, err := io.ReadAll(part)
		if err != nil {
			t.Fatalf("read part body: %v", err)
		}
		types = append(types, part.Header.Get("Content-Type"))
		bodies = append(bodies, strings.TrimSpace(decodeQP(t, string(raw))))
	}

	// Least-preferred first (RFC 2046): a client takes the last part it can
	// display, so text before HTML is what makes the HTML win where it can and
	// the text survive where it cannot.
	if len(types) != 2 {
		t.Fatalf("parts = %v, want a text part and an html part", types)
	}
	if !strings.HasPrefix(types[0], "text/plain") {
		t.Errorf("first part is %q, want text/plain", types[0])
	}
	if !strings.HasPrefix(types[1], "text/html") {
		t.Errorf("second part is %q, want text/html", types[1])
	}
	if bodies[0] != msg.Body {
		t.Errorf("text part = %q, want %q", bodies[0], msg.Body)
	}
	if bodies[1] != msg.HTMLBody {
		t.Errorf("html part = %q, want %q", bodies[1], msg.HTMLBody)
	}
}

// The rule E14 settled, enforced at the far end of the pipe as well as at the
// renderer: an HTML-only mail is penalised by spam filters and unreadable to a
// text client, and the mail most likely to be filtered is the security mail.
func TestSMTPRefusesAMessageWithNoPlainTextBody(t *testing.T) {
	t.Parallel()

	server := startFakeSMTP(t, nil, false)
	host, port := server.hostPort()

	msg := emailMessage()
	msg.Body = ""
	msg.HTMLBody = "<html><body><p>rich only</p></body></html>"

	err := newSMTP(t, smtpConfig(host, port), staticResolver{address: testTo}).Send(context.Background(), msg)
	if err == nil {
		t.Fatal("an HTML-only message was accepted")
	}
	if !errors.Is(err, domain.ErrNonRetryable) {
		t.Errorf("not non-retryable: %v", err)
	}
	if _, _, data, _, _ := server.captured(); data != "" {
		t.Errorf("something was sent anyway:\n%s", data)
	}
}

func TestSMTPEncodesANonASCIISubject(t *testing.T) {
	t.Parallel()

	server := startFakeSMTP(t, nil, false)
	host, port := server.hostPort()

	msg := emailMessage()
	msg.Subject = "Passwort geändert für Ada"

	if err := newSMTP(t, smtpConfig(host, port), staticResolver{address: testTo}).Send(context.Background(), msg); err != nil {
		t.Fatalf("Send: %v", err)
	}

	_, _, raw, _, _ := server.captured()
	header, _ := headersOf(t, raw)

	// Encoded on the wire — a raw UTF-8 header is not legal in a message
	// header — and decodable back to what the template rendered.
	if strings.Contains(rawHeaderLine(t, raw, "Subject"), "ä") {
		t.Error("the subject went out as raw UTF-8")
	}
	// net/mail does not decode encoded-words; a reader has to ask. That it
	// round-trips through the standard decoder is the claim worth making.
	decoded, err := new(mime.WordDecoder).DecodeHeader(header.Get("Subject"))
	if err != nil {
		t.Fatalf("the subject is not a decodable encoded-word: %v", err)
	}
	if decoded != msg.Subject {
		t.Errorf("decoded subject = %q, want %q", decoded, msg.Subject)
	}
}

// Defence in depth against header injection. The renderer already collapses a
// subject to one line (D5), and it is not the only thing that can produce a
// RenderedMessage.
func TestSMTPKeepsAnInjectedSubjectOnOneLine(t *testing.T) {
	t.Parallel()

	server := startFakeSMTP(t, nil, false)
	host, port := server.hostPort()

	msg := emailMessage()
	msg.Subject = "Password changed\r\nBcc: attacker@example.test"

	if err := newSMTP(t, smtpConfig(host, port), staticResolver{address: testTo}).Send(context.Background(), msg); err != nil {
		t.Fatalf("Send: %v", err)
	}

	_, _, raw, _, _ := server.captured()
	header, _ := headersOf(t, raw)

	if got := header.Get("Bcc"); got != "" {
		t.Fatalf("a Bcc header was injected: %q", got)
	}
	if !strings.Contains(header.Get("Subject"), "Bcc: attacker@example.test") {
		t.Errorf("the injected text should survive as subject text, got %q", header.Get("Subject"))
	}
}

// rawHeaderLine returns the header line as it appears on the wire, before
// net/mail decodes it.
func rawHeaderLine(t *testing.T, raw, name string) string {
	t.Helper()

	for _, line := range strings.Split(raw, "\r\n") {
		if strings.HasPrefix(line, name+":") {
			return line
		}
	}
	t.Fatalf("no %s header in:\n%s", name, raw)
	return ""
}

// Every line ending in a message is CRLF. A bare LF is what makes a message
// arrive truncated at one server and intact at the next.
func TestSMTPWritesCRLFOnly(t *testing.T) {
	t.Parallel()

	server := startFakeSMTP(t, nil, false)
	host, port := server.hostPort()

	msg := emailMessage()
	msg.Body = "line one\nline two\nline three"
	msg.HTMLBody = "<html><body>\n<p>one</p>\n<p>two</p>\n</body></html>"

	if err := newSMTP(t, smtpConfig(host, port), staticResolver{address: testTo}).Send(context.Background(), msg); err != nil {
		t.Fatalf("Send: %v", err)
	}

	_, _, raw, _, _ := server.captured()
	if strings.Contains(strings.ReplaceAll(raw, "\r\n", ""), "\n") {
		t.Errorf("the message contains a bare LF:\n%q", raw)
	}
}

func TestSMTPAuthenticatesAsTheConfiguredIdentity(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		username string
		want     string
	}{
		// The mailbox-provider case, and the default.
		{"empty username authenticates as from", "", testFrom},
		// The relay case: SES, SendGrid and Mailgun issue a credential that is
		// not an address, and without this field they are unusable.
		{"an explicit username is used verbatim", "AKIAEXAMPLE", "AKIAEXAMPLE"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			server := startFakeSMTP(t, nil, false)
			host, port := server.hostPort()

			cfg := smtpConfig(host, port)
			cfg.Username = tc.username

			if err := newSMTP(t, cfg, staticResolver{address: testTo}).Send(context.Background(), emailMessage()); err != nil {
				t.Fatalf("Send: %v", err)
			}

			_, _, _, auth, _ := server.captured()
			mechanism, encoded, _ := strings.Cut(auth, " ")
			if mechanism != "PLAIN" {
				t.Fatalf("auth mechanism = %q, want PLAIN", mechanism)
			}

			decoded, err := base64.StdEncoding.DecodeString(encoded)
			if err != nil {
				t.Fatalf("auth payload is not base64: %v", err)
			}
			// identity \x00 username \x00 password
			fields := strings.Split(string(decoded), "\x00")
			if len(fields) != 3 {
				t.Fatalf("auth payload = %q", decoded)
			}
			if fields[1] != tc.want {
				t.Errorf("authenticated as %q, want %q", fields[1], tc.want)
			}
			if fields[2] != testPassword {
				t.Errorf("the password on the wire was %q", fields[2])
			}
		})
	}
}

// The security property: when the server offers STARTTLS and the upgrade
// fails, the conversation stops. It must not fall back to handing the
// credential to a plaintext connection.
func TestSMTPStopsWhenSTARTTLSFails(t *testing.T) {
	t.Parallel()

	server := startFakeSMTP(t, nil, true)
	host, port := server.hostPort()

	err := newSMTP(t, smtpConfig(host, port), staticResolver{address: testTo}).Send(context.Background(), emailMessage())
	if err == nil {
		t.Fatal("Send succeeded despite a failed TLS upgrade")
	}
	// 454 is transient: the server said "not right now", and next time it may
	// be.
	if errors.Is(err, domain.ErrNonRetryable) {
		t.Errorf("a 4xx STARTTLS refusal must be retryable: %v", err)
	}

	_, _, _, auth, commands := server.captured()
	if auth != "" {
		t.Fatalf("the credential was sent over a plaintext connection: %q", auth)
	}
	if !containsPrefix(commands, "STARTTLS") {
		t.Errorf("the sender never attempted STARTTLS although it was advertised: %v", commands)
	}
}

func containsPrefix(lines []string, prefix string) bool {
	for _, l := range lines {
		if strings.HasPrefix(strings.ToUpper(l), prefix) {
			return true
		}
	}
	return false
}

// The error contract, end to end. The rule is the reply code and nothing else:
// 5xx is permanent by definition, 4xx is not, and no branch here reads a
// message string — server operators write those.
func TestSMTPClassifiesFailuresByReplyCode(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name         string
		replies      map[string]string
		nonRetryable bool
	}{
		{
			name:         "a refused credential is permanent",
			replies:      map[string]string{"AUTH": "535 5.7.8 Authentication credentials invalid"},
			nonRetryable: true,
		},
		{
			name:         "a temporary authentication failure is not",
			replies:      map[string]string{"AUTH": "454 4.7.0 Temporary authentication failure"},
			nonRetryable: false,
		},
		{
			name:         "an unknown mailbox is permanent",
			replies:      map[string]string{"RCPT": "550 5.1.1 No such user here"},
			nonRetryable: true,
		},
		{
			name:         "a mailbox that is busy is not",
			replies:      map[string]string{"RCPT": "451 4.3.0 Mailbox busy, try again"},
			nonRetryable: false,
		},
		{
			name:         "a rejected sender is permanent",
			replies:      map[string]string{"MAIL": "553 5.7.1 Sender address rejected"},
			nonRetryable: true,
		},
		{
			name:         "a full disk is not",
			replies:      map[string]string{"DATA": "452 4.3.1 Insufficient system storage"},
			nonRetryable: false,
		},
		{
			name:         "content rejected after the dot is permanent",
			replies:      map[string]string{"DATA-END": "554 5.7.1 Message rejected as spam"},
			nonRetryable: true,
		},
		{
			name:         "greylisting after the dot is not",
			replies:      map[string]string{"DATA-END": "451 4.7.1 Greylisted, try again later"},
			nonRetryable: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			server := startFakeSMTP(t, tc.replies, false)
			host, port := server.hostPort()

			err := newSMTP(t, smtpConfig(host, port), staticResolver{address: testTo}).Send(context.Background(), emailMessage())
			if err == nil {
				t.Fatal("Send reported success against a server that refused")
			}
			if got := errors.Is(err, domain.ErrNonRetryable); got != tc.nonRetryable {
				t.Errorf("non-retryable = %v, want %v: %v", got, tc.nonRetryable, err)
			}
		})
	}
}

// The default, and the case it exists for: an unreachable mail server is the
// most ordinary transient failure there is, and dead-lettering on it would
// empty the outbox during an outage.
func TestSMTPRetriesAnUnreachableServer(t *testing.T) {
	t.Parallel()

	// A listener opened and immediately closed yields a port nothing is on.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()

	err = newSMTP(t, smtpConfig("127.0.0.1", port), staticResolver{address: testTo}).Send(context.Background(), emailMessage())
	if err == nil {
		t.Fatal("Send succeeded against a closed port")
	}
	if errors.Is(err, domain.ErrNonRetryable) {
		t.Errorf("an unreachable server must be retryable: %v", err)
	}
}

// Implicit TLS is not a fallback: on an SMTPS connection the sender must open
// with a handshake, not with plaintext SMTP. Asserted against a plain server,
// which is the only kind a test can bind — 465 is privileged.
func TestSMTPImplicitTLSDoesNotSpeakPlaintext(t *testing.T) {
	t.Parallel()

	server := startFakeSMTP(t, nil, false)
	host, port := server.hostPort()

	cfg := smtpConfig(host, port)
	cfg.ImplicitTLS = true

	err := newSMTP(t, cfg, staticResolver{address: testTo}).Send(context.Background(), emailMessage())
	if err == nil {
		t.Fatal("an implicit-TLS connection completed against a plaintext server")
	}
	if !strings.Contains(err.Error(), "tls handshake") {
		t.Errorf("failed somewhere other than the handshake: %v", err)
	}

	// Retryable: a handshake failure carries no reply code, and the default is
	// to try again.
	if errors.Is(err, domain.ErrNonRetryable) {
		t.Errorf("a handshake failure must be retryable: %v", err)
	}
	if _, _, _, auth, _ := server.captured(); auth != "" {
		t.Error("the credential was sent to a server that failed the handshake")
	}
}

func TestPortUsesImplicitTLS(t *testing.T) {
	t.Parallel()

	for port, want := range map[int]bool{465: true, 587: false, 25: false, 2525: false, 0: false} {
		if got := sender.PortUsesImplicitTLS(port); got != want {
			t.Errorf("PortUsesImplicitTLS(%d) = %v, want %v", port, got, want)
		}
	}
}

// The distinction the resolver owes its caller, and the reason #55 converted
// identity's repository to db.ErrNotFound.
func TestSMTPHonoursTheResolversClassification(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name         string
		resolver     staticResolver
		nonRetryable bool
	}{
		{
			name:         "a recipient who does not exist is permanent",
			resolver:     staticResolver{err: fmt.Errorf("%w: no user", sender.ErrNoAddress)},
			nonRetryable: true,
		},
		{
			name:         "a failed lookup is not",
			resolver:     staticResolver{err: errors.New("connection reset by peer")},
			nonRetryable: false,
		},
		{
			// A resolver that reports success with nothing to deliver to is a
			// bug in the resolver, and no retry fixes it.
			name:         "an empty address is permanent",
			resolver:     staticResolver{address: "   "},
			nonRetryable: true,
		},
		{
			// The address comes out of another module's table. A value with a
			// newline in it would end the To header and let what follows be
			// read as another one.
			name:         "an unusable address is permanent",
			resolver:     staticResolver{address: "ada@example.test\r\nBcc: attacker@example.test"},
			nonRetryable: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			server := startFakeSMTP(t, nil, false)
			host, port := server.hostPort()

			err := newSMTP(t, smtpConfig(host, port), tc.resolver).Send(context.Background(), emailMessage())
			if err == nil {
				t.Fatal("Send succeeded with no usable address")
			}
			if got := errors.Is(err, domain.ErrNonRetryable); got != tc.nonRetryable {
				t.Errorf("non-retryable = %v, want %v: %v", got, tc.nonRetryable, err)
			}
			if _, _, _, _, commands := server.captured(); len(commands) != 0 {
				t.Errorf("the server was contacted before the address was known: %v", commands)
			}
		})
	}
}

// An operator reads LastError. It must say what failed without publishing
// another user's address into a log the whole team can see.
func TestSMTPDoesNotRepeatTheRecipientAddressIntoAnUnusableAddressError(t *testing.T) {
	t.Parallel()

	const leaked = "ada.private@example.test"

	err := newSMTP(t, smtpConfig("127.0.0.1", 1), staticResolver{address: leaked + " <not an address"}).
		Send(context.Background(), emailMessage())
	if err == nil {
		t.Fatal("expected an error")
	}
	if strings.Contains(err.Error(), leaked) {
		t.Errorf("the recipient's address is in the error text: %v", err)
	}
}

func TestNewSMTPRefusesAnIncompleteConfiguration(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		mutate   func(*sender.SMTPConfig)
		resolver sender.AddressResolver
	}{
		{"no resolver", nil, nil},
		{"no host", func(c *sender.SMTPConfig) { c.Host = "" }, staticResolver{}},
		{"no port", func(c *sender.SMTPConfig) { c.Port = 0 }, staticResolver{}},
		{"port out of range", func(c *sender.SMTPConfig) { c.Port = 70000 }, staticResolver{}},
		{"no password", func(c *sender.SMTPConfig) { c.Password = "" }, staticResolver{}},
		{"no timeout", func(c *sender.SMTPConfig) { c.Timeout = 0 }, staticResolver{}},
		{"no from", func(c *sender.SMTPConfig) { c.From = "" }, staticResolver{}},
		{"from is not an address", func(c *sender.SMTPConfig) { c.From = "No Reply" }, staticResolver{}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cfg := smtpConfig("smtp.example.test", 587)
			if tc.mutate != nil {
				tc.mutate(&cfg)
			}

			s, err := sender.NewSMTP(sender.SMTPDeps{Resolver: tc.resolver}, cfg)
			if err == nil {
				t.Fatalf("NewSMTP accepted it: %+v", s)
			}
		})
	}
}

func TestSMTPClaimsTheEmailChannel(t *testing.T) {
	t.Parallel()

	s := newSMTP(t, smtpConfig("smtp.example.test", 587), staticResolver{address: testTo})
	if got := s.Channel(); got != domain.ChannelEmail {
		t.Errorf("Channel() = %q, want %q", got, domain.ChannelEmail)
	}

	// The port the service registers it under, proved the same way the other
	// senders are.
	var _ channelSender = s
}
