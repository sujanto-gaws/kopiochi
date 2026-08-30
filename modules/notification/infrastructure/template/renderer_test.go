package template_test

import (
	"errors"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/google/uuid"

	domain "github.com/sujanto-gaws/kopiochi/modules/notification/domain"
	"github.com/sujanto-gaws/kopiochi/modules/notification/infrastructure/template"
)

// rendererPort is application.TemplateRenderer, copied rather than imported.
//
// Infrastructure may not import application (R1, E11), and tools/archtest loads
// test files too — so naming the real interface here would fail the build for
// the exact reason this package exists to demonstrate. Go satisfies interfaces
// structurally, so a copy proves the same thing: if this assertion compiles,
// the dispatcher can hold a *Renderer.
type rendererPort interface {
	Render(key string, channel domain.Channel, payload map[string]any) (domain.RenderedMessage, error)
}

var _ rendererPort = (*template.Renderer)(nil)

// changedAt is the one payload field the shipped family names.
const changedAt = "30 August 2026 at 09:14 UTC"

func passwordChanged() map[string]any { return map[string]any{"ChangedAt": changedAt} }

func newShipped(t *testing.T) *template.Renderer {
	t.Helper()
	r, err := template.New()
	if err != nil {
		// A failure here is a shipped template that does not parse or a family
		// missing a required part, which is a boot failure in production.
		t.Fatalf("New over the embedded templates: %v", err)
	}
	return r
}

func TestRenderEmailFamilyProducesBothBodies(t *testing.T) {
	t.Parallel()

	msg, err := newShipped(t).Render("security.password_changed", domain.ChannelEmail, passwordChanged())
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	if msg.Subject == "" {
		t.Error("subject is empty")
	}
	if strings.ContainsAny(msg.Subject, "\r\n") {
		t.Errorf("subject is not one line: %q", msg.Subject)
	}
	if !strings.Contains(msg.Body, changedAt) {
		t.Errorf("plain body does not carry the payload: %q", msg.Body)
	}
	if strings.Contains(msg.Body, "<p>") {
		t.Errorf("plain body carries markup: %q", msg.Body)
	}

	// The asymmetry E14 settled: a family may add HTML, and the plain part is
	// still there beside it.
	if !strings.Contains(msg.HTMLBody, changedAt) {
		t.Errorf("html body does not carry the payload: %q", msg.HTMLBody)
	}
	if !strings.Contains(msg.HTMLBody, "<html") {
		t.Errorf("html body is a fragment, not a document: %q", msg.HTMLBody)
	}
}

// A renderer that stamped routing would let a template misaddress a message.
// The dispatch cycle overwrites these three after calling Render, so a value
// here would be silently discarded in production and this is the only place the
// contract can be asserted.
func TestRenderLeavesRoutingFieldsUnset(t *testing.T) {
	t.Parallel()

	msg, err := newShipped(t).Render("security.password_changed", domain.ChannelEmail, passwordChanged())
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if msg.NotificationID != uuid.Nil || msg.RecipientID != uuid.Nil || msg.Channel != "" {
		t.Errorf("renderer set a routing field: %+v", msg)
	}
}

// The in-app family ships no html part, which is the normal state and not a
// failure — and is why HTMLBody's emptiness has to mean "none" rather than
// "not rendered yet".
func TestRenderInAppFamilyHasNoHTMLBody(t *testing.T) {
	t.Parallel()

	msg, err := newShipped(t).Render("security.password_changed", domain.ChannelInApp, passwordChanged())
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if msg.Body == "" {
		t.Error("plain body is empty; it is the mandatory part")
	}
	if msg.HTMLBody != "" {
		t.Errorf("html body should be empty for in-app, got %q", msg.HTMLBody)
	}
}

func TestRenderMissingTemplateIsTypedAndNonRetryable(t *testing.T) {
	t.Parallel()

	r := newShipped(t)
	cases := []struct {
		name    string
		key     string
		channel domain.Channel
	}{
		{"unknown key", "security.nothing_like_this", domain.ChannelEmail},
		{"known key, channel with no family", "security.password_changed", domain.ChannelWebhook},
		{"empty key", "", domain.ChannelEmail},
		{"key that is a filename", "security.password_changed.email.text.tmpl", domain.ChannelEmail},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			msg, err := r.Render(tc.key, tc.channel, passwordChanged())
			if err == nil {
				t.Fatalf("expected an error, got %+v", msg)
			}
			if !errors.Is(err, template.ErrTemplateNotFound) {
				t.Errorf("not typed as not-found: %v", err)
			}
			// The half the dispatcher acts on: a missing template is a
			// deployment bug, so the row dies now instead of spending its
			// retry budget re-discovering the deployment is unchanged.
			if !errors.Is(err, domain.ErrNonRetryable) {
				t.Errorf("not classified as non-retryable: %v", err)
			}
			if errors.Is(err, template.ErrTemplateInvalid) {
				t.Errorf("a missing template must not read as an unusable one: %v", err)
			}
			if msg != (domain.RenderedMessage{}) {
				t.Errorf("a failed render returned a message: %+v", msg)
			}
		})
	}
}

// missingkey=error is what turns a producer's mistake into a dead row an
// operator can see. Without it these all render "<no value>" into a user's
// mailbox and succeed.
func TestRenderMalformedPayloadIsTypedAndNonRetryable(t *testing.T) {
	t.Parallel()

	r := newShipped(t)
	cases := []struct {
		name    string
		payload map[string]any
	}{
		{"nil payload", nil},
		{"empty payload", map[string]any{}},
		{"misspelled key", map[string]any{"changedAt": changedAt}},
		{"unrelated keys only", map[string]any{"UserID": "u1"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			msg, err := r.Render("security.password_changed", domain.ChannelEmail, tc.payload)
			if err == nil {
				t.Fatalf("expected an error, got %+v", msg)
			}
			if !errors.Is(err, template.ErrTemplateInvalid) {
				t.Errorf("not typed as invalid: %v", err)
			}
			if !errors.Is(err, domain.ErrNonRetryable) {
				t.Errorf("not classified as non-retryable: %v", err)
			}
			if errors.Is(err, template.ErrTemplateNotFound) {
				t.Errorf("a bad payload must not read as a missing template: %v", err)
			}
			if msg != (domain.RenderedMessage{}) {
				t.Errorf("a failed render returned a message: %+v", msg)
			}
		})
	}
}

// A malformed payload for the HTML part alone still fails the whole render:
// there is no half-rendered message, and a mail with a body and no HTML it was
// supposed to have is a message nobody wrote.
func TestRenderFailsWhenOnlyTheHTMLPartCannotExecute(t *testing.T) {
	t.Parallel()

	r, err := template.NewFromFS(fstest.MapFS{
		"k.email.subject.tmpl": &fstest.MapFile{Data: []byte("Subject")},
		"k.email.text.tmpl":    &fstest.MapFile{Data: []byte("plain")},
		"k.email.html.tmpl":    &fstest.MapFile{Data: []byte("<p>{{.OnlyHere}}</p>")},
	})
	if err != nil {
		t.Fatalf("NewFromFS: %v", err)
	}

	if _, err := r.Render("k", domain.ChannelEmail, map[string]any{}); !errors.Is(err, template.ErrTemplateInvalid) {
		t.Fatalf("want ErrTemplateInvalid, got %v", err)
	}
}

// The HTML part is contextually escaped and the text part is not, which is the
// only reason two template engines are involved. Escaping the plain body would
// corrupt it; not escaping the HTML one puts a payload value into a document a
// mail client renders.
func TestHTMLPartEscapesPayloadAndPlainPartDoesNot(t *testing.T) {
	t.Parallel()

	r, err := template.NewFromFS(fstest.MapFS{
		"k.email.subject.tmpl": &fstest.MapFile{Data: []byte("Subject")},
		"k.email.text.tmpl":    &fstest.MapFile{Data: []byte("hello {{.Name}}")},
		"k.email.html.tmpl":    &fstest.MapFile{Data: []byte("<p>hello {{.Name}}</p>")},
	})
	if err != nil {
		t.Fatalf("NewFromFS: %v", err)
	}

	const hostile = `<script>alert(1)</script>`
	msg, err := r.Render("k", domain.ChannelEmail, map[string]any{"Name": hostile})
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	if !strings.Contains(msg.Body, hostile) {
		t.Errorf("plain body was escaped: %q", msg.Body)
	}
	if strings.Contains(msg.HTMLBody, "<script>") {
		t.Errorf("html body was not escaped: %q", msg.HTMLBody)
	}
	if !strings.Contains(msg.HTMLBody, "&lt;script&gt;") {
		t.Errorf("html body does not carry the escaped value: %q", msg.HTMLBody)
	}
}

// A subject becomes an SMTP header. A payload value carrying CRLF would end the
// Subject field and let what follows be read as another header.
func TestSubjectIsCollapsedToOneLine(t *testing.T) {
	t.Parallel()

	r, err := template.NewFromFS(fstest.MapFS{
		"k.email.subject.tmpl": &fstest.MapFile{Data: []byte("Hello {{.Name}}\n")},
		"k.email.text.tmpl":    &fstest.MapFile{Data: []byte("body")},
	})
	if err != nil {
		t.Fatalf("NewFromFS: %v", err)
	}

	msg, err := r.Render("k", domain.ChannelEmail, map[string]any{
		"Name": "Ada\r\nBcc: someone@example.test",
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	if strings.ContainsAny(msg.Subject, "\r\n") {
		t.Fatalf("subject still spans lines: %q", msg.Subject)
	}
	if want := "Hello Ada Bcc: someone@example.test"; msg.Subject != want {
		t.Errorf("subject = %q, want %q", msg.Subject, want)
	}
}

func TestRenderRejectsAnEmptySubject(t *testing.T) {
	t.Parallel()

	r, err := template.NewFromFS(fstest.MapFS{
		"k.email.subject.tmpl": &fstest.MapFile{Data: []byte("{{.Heading}}\n")},
		"k.email.text.tmpl":    &fstest.MapFile{Data: []byte("body")},
	})
	if err != nil {
		t.Fatalf("NewFromFS: %v", err)
	}

	if _, err := r.Render("k", domain.ChannelEmail, map[string]any{"Heading": "   "}); !errors.Is(err, template.ErrTemplateInvalid) {
		t.Fatalf("want ErrTemplateInvalid, got %v", err)
	}
}

// Everything here is refused at construction, which is a boot failure — the
// alternative is a notification that dead-letters at 3am for a mistake visible
// in the filename.
func TestNewFromFSRefusesAnUnusableSet(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		files fstest.MapFS
	}{
		{"html without text: the HTML-only message E14 forbids", fstest.MapFS{
			"k.email.subject.tmpl": &fstest.MapFile{Data: []byte("s")},
			"k.email.html.tmpl":    &fstest.MapFile{Data: []byte("<p>b</p>")},
		}},
		{"text without subject", fstest.MapFS{
			"k.email.text.tmpl": &fstest.MapFile{Data: []byte("b")},
		}},
		{"unparsable text part", fstest.MapFS{
			"k.email.subject.tmpl": &fstest.MapFile{Data: []byte("s")},
			"k.email.text.tmpl":    &fstest.MapFile{Data: []byte("{{.Unclosed")},
		}},
		{"unparsable html part", fstest.MapFS{
			"k.email.subject.tmpl": &fstest.MapFile{Data: []byte("s")},
			"k.email.text.tmpl":    &fstest.MapFile{Data: []byte("b")},
			"k.email.html.tmpl":    &fstest.MapFile{Data: []byte("<p>{{.Unclosed</p>")},
		}},
		{"typo'd part is not a silently ignored file", fstest.MapFS{
			"k.email.subject.tmpl": &fstest.MapFile{Data: []byte("s")},
			"k.email.text.tmpl":    &fstest.MapFile{Data: []byte("b")},
			"k.email.htlm.tmpl":    &fstest.MapFile{Data: []byte("<p>b</p>")},
		}},
		{"unknown channel", fstest.MapFS{
			"k.telegram.subject.tmpl": &fstest.MapFile{Data: []byte("s")},
			"k.telegram.text.tmpl":    &fstest.MapFile{Data: []byte("b")},
		}},
		{"no channel segment", fstest.MapFS{
			"k.subject.tmpl": &fstest.MapFile{Data: []byte("s")},
		}},
		{"no key segment", fstest.MapFS{
			"email.subject.tmpl": &fstest.MapFile{Data: []byte("s")},
		}},
		{"one segment", fstest.MapFS{
			"subject.tmpl": &fstest.MapFile{Data: []byte("s")},
		}},
		{"leading dot leaves an empty key", fstest.MapFS{
			".email.subject.tmpl": &fstest.MapFile{Data: []byte("s")},
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			r, err := template.NewFromFS(tc.files)
			if err == nil {
				t.Fatalf("expected a construction error, got a renderer: %+v", r)
			}
			if !errors.Is(err, template.ErrTemplateInvalid) {
				t.Errorf("not typed as invalid: %v", err)
			}
		})
	}
}

// Only *.tmpl is this package's business. A README beside the templates is not
// a broken template.
func TestNewFromFSIgnoresFilesThatAreNotTemplates(t *testing.T) {
	t.Parallel()

	r, err := template.NewFromFS(fstest.MapFS{
		"README.md":            &fstest.MapFile{Data: []byte("# templates")},
		"k.email.subject.tmpl": &fstest.MapFile{Data: []byte("s")},
		"k.email.text.tmpl":    &fstest.MapFile{Data: []byte("b")},
	})
	if err != nil {
		t.Fatalf("NewFromFS: %v", err)
	}
	if _, err := r.Render("k", domain.ChannelEmail, nil); err != nil {
		t.Fatalf("render: %v", err)
	}
}

// An empty set is legal — every render then reports not-found, which is the
// honest answer and not a crash at boot. It matters because a deployment that
// wires the module with no templates should fail per notification, visibly,
// rather than refuse to start with no clue why.
func TestNewFromFSAcceptsAnEmptySet(t *testing.T) {
	t.Parallel()

	r, err := template.NewFromFS(fstest.MapFS{})
	if err != nil {
		t.Fatalf("NewFromFS: %v", err)
	}
	if _, err := r.Render("k", domain.ChannelEmail, nil); !errors.Is(err, template.ErrTemplateNotFound) {
		t.Fatalf("want ErrTemplateNotFound, got %v", err)
	}
}

// The key is the one segment allowed to contain dots, so the filename must be
// read right to left. Reading left to right resolves "security.password_changed"
// to key "security", channel "password_changed" — which is not a channel, so
// every shipped template would fail to load.
func TestKeysMayContainDots(t *testing.T) {
	t.Parallel()

	r, err := template.NewFromFS(fstest.MapFS{
		"a.b.c.inapp.subject.tmpl": &fstest.MapFile{Data: []byte("s")},
		"a.b.c.inapp.text.tmpl":    &fstest.MapFile{Data: []byte("b")},
	})
	if err != nil {
		t.Fatalf("NewFromFS: %v", err)
	}
	if _, err := r.Render("a.b.c", domain.ChannelInApp, nil); err != nil {
		t.Fatalf("render: %v", err)
	}
}
