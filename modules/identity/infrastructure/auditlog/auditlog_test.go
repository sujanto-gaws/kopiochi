package auditlog_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/rs/zerolog"

	"github.com/sujanto-gaws/kopiochi/internal/audit"
	"github.com/sujanto-gaws/kopiochi/modules/identity/infrastructure/auditlog"
)

// This adapter is eight one-line methods, and the thing worth testing is not
// that Emit was called — it is the MAPPING. Every method chooses an action, an
// outcome, and which field the identifier goes in, and each of those choices is
// load-bearing for whoever reads the audit stream afterwards. A swapped
// ActorID/Subject or a success outcome on a failure event is invisible in
// review and wrong forever in the log.

// emitted runs fn against a fresh auditor and decodes the single record it
// wrote.
func emitted(t *testing.T, fn func(*auditlog.Auditor, context.Context)) map[string]any {
	t.Helper()

	var buf bytes.Buffer
	fn(auditlog.New(audit.New(zerolog.New(&buf))), context.Background())

	line := strings.TrimSpace(buf.String())
	if line == "" {
		t.Fatal("nothing was emitted")
	}
	if strings.Count(line, "\n") > 0 {
		t.Fatalf("expected exactly one record, got:\n%s", line)
	}

	var rec map[string]any
	if err := json.Unmarshal([]byte(line), &rec); err != nil {
		t.Fatalf("audit line is not JSON: %v (%q)", err, line)
	}
	return rec
}

func TestAuditor_MapsEachEventToItsActionAndOutcome(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		call    func(*auditlog.Auditor, context.Context)
		action  string
		outcome string
	}{
		{"login succeeded", func(a *auditlog.Auditor, ctx context.Context) { a.LoginSucceeded(ctx, "u1") },
			string(audit.ActionLoginSucceeded), string(audit.OutcomeSuccess)},
		{"account locked", func(a *auditlog.Auditor, ctx context.Context) { a.AccountLocked(ctx, "u1") },
			string(audit.ActionAccountLocked), string(audit.OutcomeFailure)},
		{"logout", func(a *auditlog.Auditor, ctx context.Context) { a.LogoutSucceeded(ctx, "u1") },
			string(audit.ActionLogout), string(audit.OutcomeSuccess)},
		{"mfa enrolled", func(a *auditlog.Auditor, ctx context.Context) { a.MFAEnrolled(ctx, "u1") },
			string(audit.ActionMFAEnrolled), string(audit.OutcomeSuccess)},
		{"mfa failed", func(a *auditlog.Auditor, ctx context.Context) { a.MFAFailed(ctx, "u1", "bad code") },
			string(audit.ActionMFAFailed), string(audit.OutcomeFailure)},
		{"login failed", func(a *auditlog.Auditor, ctx context.Context) { a.LoginFailed(ctx, "alice", "bad password") },
			string(audit.ActionLoginFailed), string(audit.OutcomeFailure)},
		{"refresh reuse", func(a *auditlog.Auditor, ctx context.Context) { a.RefreshReuseDetected(ctx, "u1", "fam", 2, nil) },
			string(audit.ActionRefreshReuseDetected), string(audit.OutcomeFailure)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			rec := emitted(t, tc.call)
			if got := rec["action"]; got != tc.action {
				t.Errorf("action = %v, want %v", got, tc.action)
			}
			if got := rec["outcome"]; got != tc.outcome {
				t.Errorf("outcome = %v, want %v", got, tc.outcome)
			}
		})
	}
}

// TestAuditor_AFailedLoginIsNotAnActor is the adapter's one genuinely
// security-relevant decision, and it is stated in its doc comment: nobody
// authenticated, so the attempted username goes in Subject. Putting it in
// ActorID would make every failed login read, downstream, as a confirmed
// identity doing something — including the ones typed by an attacker
// enumerating usernames.
func TestAuditor_AFailedLoginIsNotAnActor(t *testing.T) {
	t.Parallel()

	rec := emitted(t, func(a *auditlog.Auditor, ctx context.Context) {
		a.LoginFailed(ctx, "alice", "bad password")
	})

	if got := rec["subject"]; got != "alice" {
		t.Errorf("subject = %v, want the attempted username", got)
	}
	if _, ok := rec["actor_id"]; ok {
		t.Errorf("a failed login named an actor_id: %v", rec["actor_id"])
	}
	if got := rec["reason"]; got != "bad password" {
		t.Errorf("reason = %v, want it carried through", got)
	}
}

// TestAuditor_SucceededEventsNameTheActor is the mirror image: these did
// authenticate, so they belong in ActorID and must not land in Subject.
func TestAuditor_SucceededEventsNameTheActor(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		call func(*auditlog.Auditor, context.Context)
	}{
		{"login", func(a *auditlog.Auditor, ctx context.Context) { a.LoginSucceeded(ctx, "u1") }},
		{"logout", func(a *auditlog.Auditor, ctx context.Context) { a.LogoutSucceeded(ctx, "u1") }},
		{"locked", func(a *auditlog.Auditor, ctx context.Context) { a.AccountLocked(ctx, "u1") }},
		{"mfa enrolled", func(a *auditlog.Auditor, ctx context.Context) { a.MFAEnrolled(ctx, "u1") }},
		{"mfa failed", func(a *auditlog.Auditor, ctx context.Context) { a.MFAFailed(ctx, "u1", "bad code") }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			rec := emitted(t, tc.call)
			if got := rec["actor_id"]; got != "u1" {
				t.Errorf("actor_id = %v, want u1", got)
			}
			if _, ok := rec["subject"]; ok {
				t.Errorf("an authenticated event used subject: %v", rec["subject"])
			}
		})
	}
}

// TestAuditor_ReuseDetectionCarriesTheFamilyNeverTheToken: the doc comment
// commits to reporting the family id and NOT the token or its hash, because an
// audit stream is retained longer and read by more people than a request log.
// A credential leaked into it is worse, not better.
func TestAuditor_ReuseDetectionCarriesTheFamilyNeverTheToken(t *testing.T) {
	t.Parallel()

	rec := emitted(t, func(a *auditlog.Auditor, ctx context.Context) {
		a.RefreshReuseDetected(ctx, "u1", "fam-42", 4, nil)
	})

	if got := rec["family_id"]; got != "fam-42" {
		t.Errorf("family_id = %v, want fam-42", got)
	}
	if got := rec["tokens_revoked"]; got != float64(4) {
		t.Errorf("tokens_revoked = %v, want 4", got)
	}
	for _, forbidden := range []string{"token", "token_hash", "hash"} {
		if _, ok := rec[forbidden]; ok {
			t.Errorf("the reuse record leaked %q into the audit stream", forbidden)
		}
	}
}

// TestAuditor_AFailedRevocationIsDistinguishable: a detection whose revocation
// did not land is the one case where the attacker still holds a working
// session. Its doc comment says that must not be indistinguishable from a clean
// one, so the fields are asserted in both directions.
func TestAuditor_AFailedRevocationIsDistinguishable(t *testing.T) {
	t.Parallel()

	clean := emitted(t, func(a *auditlog.Auditor, ctx context.Context) {
		a.RefreshReuseDetected(ctx, "u1", "fam", 3, nil)
	})
	if _, ok := clean["revocation_failed"]; ok {
		t.Errorf("a successful revocation was flagged as failed")
	}
	if _, ok := clean["revocation_error"]; ok {
		t.Errorf("a successful revocation carried an error")
	}

	broken := emitted(t, func(a *auditlog.Auditor, ctx context.Context) {
		a.RefreshReuseDetected(ctx, "u1", "fam", 0, errors.New("connection reset"))
	})
	if got := broken["revocation_failed"]; got != true {
		t.Errorf("revocation_failed = %v, want true", got)
	}
	if got := broken["revocation_error"]; got != "connection reset" {
		t.Errorf("revocation_error = %v, want the cause", got)
	}
}

// TestAuditor_ToleratesANilLogger: New is called from the composition root, and
// audit.Logger.Emit is nil-safe by design. If that ever stopped holding, the
// first symptom would be a panic on a security event — the worst possible
// moment to discover it.
func TestAuditor_ToleratesANilLogger(t *testing.T) {
	t.Parallel()

	a := auditlog.New(nil)
	ctx := context.Background()

	a.LoginSucceeded(ctx, "u1")
	a.LoginFailed(ctx, "alice", "bad password")
	a.AccountLocked(ctx, "u1")
	a.LogoutSucceeded(ctx, "u1")
	a.MFAEnrolled(ctx, "u1")
	a.MFAFailed(ctx, "u1", "bad code")
	a.RefreshReuseDetected(ctx, "u1", "fam", 0, errors.New("boom"))
}
