// Package template renders a notification's template key and payload into the
// subject and bodies a sender transmits. It is the module's TemplateRenderer
// port, implemented over templates embedded in the binary — there is no
// template directory to deploy and no file a running process can be missing.
//
// # Naming convention
//
// One file per (key, channel, part):
//
//	templates/<key>.<channel>.<part>.tmpl
//	templates/security.password_changed.email.text.tmpl
//
// <key> is the notification's TemplateKey and may itself contain dots;
// <channel> is a domain.Channel ("email", "inapp", "webhook"); <part> is
// "subject", "text" or "html". The files sharing a (key, channel) prefix are a
// FAMILY, and a family is what Render resolves.
//
// The three-segment name is the union of two documents that each described one
// axis. The plan's <key>.<channel>.tmpl carries the channel axis: an email and
// an in-app card are different messages, written separately. The blueprint's
// *.subject/*.text/*.html carries the MIME axis: the text part and the HTML
// part of ONE email are two representations of a single message and cannot be
// two notifications. Both axes are real, so both are in the name (see E14).
//
// # Required and optional parts
//
//   - subject — REQUIRED.
//   - text    — REQUIRED. Becomes domain.RenderedMessage.Body.
//   - html    — OPTIONAL. Becomes HTMLBody; absent means the message has none.
//
// Requiring the plain part and not the rich one is the invariant E14 settled,
// and it is enforced here rather than trusted: New refuses a family that ships
// html without text, so no template can produce an HTML-only message even by
// omission. An HTML-only mail is penalised by spam filters and unreadable to a
// text client, and the mail most likely to be filtered is the security mail —
// the one that must arrive.
//
// The html part is a complete document, not a fragment. A sender chooses a MIME
// structure; it must not have to know enough HTML to wrap one.
//
// Subject is required for the same reason in miniature: both channels this
// module ships display a heading, and a mail with a blank Subject line is a
// deliverability problem of exactly the same kind. A family that omits it does
// not build.
//
// A filename that does not parse — unknown part, unknown channel, too few
// segments — is a construction error and not a skipped file. A typo'd name that
// were merely ignored would be a template that silently never renders, found
// months later by a user who did not get their mail.
//
// # Payload contract
//
// Templates execute with missingkey=error, so a payload that omits a key the
// template names fails the render instead of writing "<no value>" into a user's
// mailbox. The dispatch cycle turns that into a dead-lettered row with the
// reason in LastError, which an operator can see and fix; a message delivered
// with a hole in it is unfixable after the fact and invisible in every test.
//
// The payload for the family shipped here, security.password_changed, is:
//
//	{"ChangedAt": string}   // already formatted for display
//
// Producers of that key — the identity adapter in cmd/api — must supply it.
//
// # What this package does not do
//
// It never sets NotificationID, RecipientID or Channel on the message it
// returns. Routing is the outbox row's property, and the dispatch cycle stamps
// those fields after calling Render precisely so that a template cannot
// misaddress a message.
package template

import (
	"bytes"
	"embed"
	"fmt"
	htmltemplate "html/template"
	"io/fs"
	"strings"
	texttemplate "text/template"

	domain "github.com/sujanto-gaws/kopiochi/modules/notification/domain"
)

// templates holds every shipped template family. Embedding is what makes the
// renderer's contents a property of the binary rather than of the filesystem it
// runs on: New cannot half-succeed against a deployment that forgot to copy a
// directory.
//
//go:embed templates/*.tmpl
var templates embed.FS

// The renderer's error contract.
//
// Both sentinels wrap domain.ErrNonRetryable, which settles the row as dead on
// the first attempt. That is redundant today — the dispatch cycle classifies
// EVERY renderer error as non-retryable by construction (see
// application/ports.go) — and it is kept anyway, because the classification is
// a property of the failure and not of the one caller that currently sees it:
// an unknown key is a deployment bug and a payload the template cannot execute
// is a producer bug, and no amount of waiting fixes either. The cost is that
// LastError carries the sentence twice when dispatch adds its own wrap. That is
// noise in a log field; the alternative is a second consumer retrying a
// misconfiguration until the row's budget is gone.
//
// The two are distinct because they name different repairs. Not-found is fixed
// by shipping a template or correcting the key; invalid is fixed by the
// producer sending the payload the template asks for.
var (
	// ErrTemplateNotFound reports that no family exists for a (key, channel)
	// pair.
	ErrTemplateNotFound = fmt.Errorf("%w: no template for key and channel", domain.ErrNonRetryable)

	// ErrTemplateInvalid reports a family that cannot produce a message: it
	// did not parse, it is missing a required part, or it did not execute
	// against this payload.
	ErrTemplateInvalid = fmt.Errorf("%w: template is unusable", domain.ErrNonRetryable)
)

// The parts of a family. Values are the <part> segment of a filename.
const (
	partSubject = "subject"
	partText    = "text"
	partHTML    = "html"
)

// fileSuffix is the extension every template file carries, and the only thing
// New looks at when deciding whether a file in the directory is its business.
const fileSuffix = ".tmpl"

// familyID identifies one template family. It is a struct key rather than a
// joined string so that a key containing the separator cannot collide with a
// different (key, channel) pair.
type familyID struct {
	key     string
	channel domain.Channel
}

// family is one (key, channel)'s parsed templates. subject and text are always
// non-nil; html is nil when the family ships no rich part.
type family struct {
	subject *texttemplate.Template
	text    *texttemplate.Template
	html    *htmltemplate.Template
}

// Renderer resolves a (key, channel) to a family and executes it.
//
// It is immutable after New and holds no state per render, so a single
// instance is shared by every dispatcher worker.
type Renderer struct {
	families map[familyID]*family
}

// New returns a Renderer over the templates embedded in this package.
//
// Every file is parsed and every family checked here, at construction, so a
// malformed template is a boot failure rather than a notification that
// dead-letters at 3am. This is the constructor the composition root uses.
func New() (*Renderer, error) {
	dir, err := fs.Sub(templates, "templates")
	if err != nil {
		return nil, fmt.Errorf("open embedded templates: %w", err)
	}
	return NewFromFS(dir)
}

// NewFromFS returns a Renderer over the *.tmpl files at the root of fsys.
//
// It exists so the parsing and validation rules can be tested against families
// that must never be shipped — an html part with no text part, an unparsable
// action, a filename naming a channel that does not exist. Those cases cannot
// be expressed in templates/ without breaking New for everyone.
func NewFromFS(fsys fs.FS) (*Renderer, error) {
	names, err := fs.Glob(fsys, "*"+fileSuffix)
	if err != nil {
		return nil, fmt.Errorf("list templates: %w", err)
	}

	families := make(map[familyID]*family, len(names))
	for _, name := range names {
		id, part, err := parseName(name)
		if err != nil {
			return nil, err
		}

		content, err := fs.ReadFile(fsys, name)
		if err != nil {
			return nil, fmt.Errorf("read template %s: %w", name, err)
		}

		f := families[id]
		if f == nil {
			f = &family{}
			families[id] = f
		}
		if err := f.parse(name, part, string(content)); err != nil {
			return nil, err
		}
	}

	for id, f := range families {
		if err := f.validate(id); err != nil {
			return nil, err
		}
	}

	return &Renderer{families: families}, nil
}

// parse compiles one part into f.
//
// A name that does not describe a part this package knows is an error rather
// than a file quietly skipped — see the package comment on typo'd filenames.
func (f *family) parse(name, part, content string) error {
	switch part {
	case partSubject:
		t, err := parseText(name, content)
		if err != nil {
			return err
		}
		f.subject = t

	case partText:
		t, err := parseText(name, content)
		if err != nil {
			return err
		}
		f.text = t

	case partHTML:
		// html/template and not text/template, and this is the whole reason
		// the two engines are both imported: a payload value reaching the HTML
		// part is contextually escaped, so a display name containing a <script>
		// tag renders as text instead of executing in a mail client that runs
		// scripts. The text part needs no such treatment and must not have it,
		// because escaping a plain-text body corrupts it.
		t, err := htmltemplate.New(name).Option("missingkey=error").Parse(content)
		if err != nil {
			return fmt.Errorf("%w: %s: %w", ErrTemplateInvalid, name, err)
		}
		f.html = t

	default:
		return fmt.Errorf("%w: %s: unknown part %q, want %s, %s or %s",
			ErrTemplateInvalid, name, part, partSubject, partText, partHTML)
	}
	return nil
}

// parseText compiles a plain-text part.
func parseText(name, content string) (*texttemplate.Template, error) {
	t, err := texttemplate.New(name).Option("missingkey=error").Parse(content)
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %w", ErrTemplateInvalid, name, err)
	}
	return t, nil
}

// validate refuses a family that cannot produce a complete message.
//
// This is where "plain text is mandatory, HTML is optional" stops being a
// convention and becomes a build failure.
func (f *family) validate(id familyID) error {
	if f.subject == nil {
		return fmt.Errorf("%w: %s.%s is missing its %s part", ErrTemplateInvalid, id.key, id.channel, partSubject)
	}
	if f.text == nil {
		return fmt.Errorf("%w: %s.%s is missing its %s part; a family may add %s but never replace %s with it",
			ErrTemplateInvalid, id.key, id.channel, partText, partHTML, partText)
	}
	return nil
}

// parseName splits a template filename into the family it belongs to and the
// part it supplies.
//
// It reads right to left — part, then channel, then whatever remains is the key
// — because the key is the one segment allowed to contain dots
// ("security.password_changed"), and reading from the left cannot tell where it
// ends.
func parseName(name string) (familyID, string, error) {
	base := strings.TrimSuffix(name, fileSuffix)

	partAt := strings.LastIndex(base, ".")
	if partAt <= 0 {
		return familyID{}, "", fmt.Errorf("%w: %s: want <key>.<channel>.<part>%s", ErrTemplateInvalid, name, fileSuffix)
	}
	part := base[partAt+1:]

	rest := base[:partAt]
	channelAt := strings.LastIndex(rest, ".")
	if channelAt <= 0 {
		return familyID{}, "", fmt.Errorf("%w: %s: want <key>.<channel>.<part>%s", ErrTemplateInvalid, name, fileSuffix)
	}

	channel := domain.Channel(rest[channelAt+1:])
	if !channel.Valid() {
		return familyID{}, "", fmt.Errorf("%w: %s: unknown channel %q", ErrTemplateInvalid, name, channel)
	}

	return familyID{key: rest[:channelAt], channel: channel}, part, nil
}

// Render executes the family for key and channel against payload.
//
// The returned message carries Subject, Body and — when the family ships one —
// HTMLBody, and nothing else: see the package comment on routing fields.
func (r *Renderer) Render(key string, channel domain.Channel, payload map[string]any) (domain.RenderedMessage, error) {
	f, ok := r.families[familyID{key: key, channel: channel}]
	if !ok {
		return domain.RenderedMessage{}, fmt.Errorf("%w: key %q, channel %q", ErrTemplateNotFound, key, channel)
	}

	subject, err := executeText(f.subject, payload)
	if err != nil {
		return domain.RenderedMessage{}, err
	}
	body, err := executeText(f.text, payload)
	if err != nil {
		return domain.RenderedMessage{}, err
	}

	msg := domain.RenderedMessage{
		Subject: oneLine(subject),
		Body:    strings.TrimSpace(body),
	}

	if f.html != nil {
		var buf bytes.Buffer
		if err := f.html.Execute(&buf, payload); err != nil {
			return domain.RenderedMessage{}, fmt.Errorf("%w: execute %s: %w", ErrTemplateInvalid, f.html.Name(), err)
		}
		msg.HTMLBody = strings.TrimSpace(buf.String())
	}

	// Refused here rather than at construction: a template can only be checked
	// for an empty subject once its payload is known, and an empty Subject
	// header is the deliverability problem the required subject part exists to
	// prevent. It is ErrTemplateInvalid because the repair is the same as any
	// other unusable family — fix the template, or send the data it names.
	if msg.Subject == "" {
		return domain.RenderedMessage{}, fmt.Errorf("%w: %s.%s rendered an empty subject", ErrTemplateInvalid, key, channel)
	}

	return msg, nil
}

// executeText runs one plain-text part.
func executeText(t *texttemplate.Template, payload map[string]any) (string, error) {
	var buf bytes.Buffer
	if err := t.Execute(&buf, payload); err != nil {
		return "", fmt.Errorf("%w: execute %s: %w", ErrTemplateInvalid, t.Name(), err)
	}
	return buf.String(), nil
}

// oneLine collapses every run of whitespace in s — including the newline the
// file ends with — into a single space, and trims the ends.
//
// A subject becomes an SMTP header, and a header is one line by definition. A
// payload value carrying a CRLF would otherwise end the Subject field and let
// whatever follows it be read as another header — a Bcc, a Reply-To. The sender
// is not excused from validating what it writes, but the injection is easiest to
// kill here, where the value first becomes a subject, and a template author
// cannot forget to do it.
//
// Collapsing rather than rejecting: a display name with a stray newline in it
// must not be the reason a password-changed mail never arrives.
func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }
