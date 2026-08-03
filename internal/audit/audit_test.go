package audit

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/rs/zerolog"
)

func decode(t *testing.T, buf *bytes.Buffer) map[string]any {
	t.Helper()

	var rec map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &rec); err != nil {
		t.Fatalf("audit line is not JSON: %v (%q)", err, buf.String())
	}
	return rec
}

// TestEmit_MarksEveryRecord: a log pipeline routes audit records to a
// longer-retention store on this field alone. Without it they are
// indistinguishable from request logs and expire with them.
func TestEmit_MarksEveryRecord(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	New(zerolog.New(&buf)).Emit(context.Background(), Event{
		Action: ActionLoginSucceeded, Outcome: OutcomeSuccess, ActorID: "u1",
	})

	if got := decode(t, &buf)["audit"]; got != true {
		t.Errorf("audit marker = %v, want true", got)
	}
}

func TestEmit_WritesTheFields(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	New(zerolog.New(&buf)).Emit(context.Background(), Event{
		Action:   ActionLoginFailed,
		Outcome:  OutcomeFailure,
		Subject:  "alice",
		Reason:   "bad_password",
		TargetID: "t1",
		Fields:   map[string]any{"attempts": 3},
	})

	rec := decode(t, &buf)
	for field, want := range map[string]any{
		"action":    string(ActionLoginFailed),
		"outcome":   string(OutcomeFailure),
		"subject":   "alice",
		"reason":    "bad_password",
		"target_id": "t1",
		"attempts":  float64(3),
	} {
		if rec[field] != want {
			t.Errorf("%s = %v, want %v", field, rec[field], want)
		}
	}
}

// TestEmit_OmitsEmptyFields keeps records greppable: a stream where half the
// keys are empty strings is one where `actor_id != ""` stops meaning
// "authenticated".
func TestEmit_OmitsEmptyFields(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	New(zerolog.New(&buf)).Emit(context.Background(), Event{
		Action: ActionLogout, Outcome: OutcomeSuccess, ActorID: "u1",
	})

	rec := decode(t, &buf)
	for _, absent := range []string{"subject", "reason", "target_id"} {
		if _, ok := rec[absent]; ok {
			t.Errorf("%q is present but empty; it should be omitted", absent)
		}
	}
}

// TestEmit_NeverBelowWarn is the property that keeps an audit trail from being
// silently filtered away. A deployment running at warn or error to cut request
// log volume must still get every security event.
func TestEmit_NeverBelowWarn(t *testing.T) {
	t.Parallel()

	actions := []Action{
		ActionLoginSucceeded, ActionLoginFailed, ActionAccountLocked,
		ActionLogout, ActionMFAEnrolled, ActionMFAFailed,
		ActionRefreshReuseDetected,
	}

	for _, action := range actions {
		t.Run(string(action), func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer
			New(zerolog.New(&buf)).Emit(context.Background(), Event{Action: action})

			level, _ := decode(t, &buf)["level"].(string)
			if level != "warn" && level != "error" {
				t.Errorf("level = %q, want warn or error — anything lower can be filtered out", level)
			}
		})
	}
}

// TestEmit_ReuseDetectionIsAnError: it is the one event here that means an
// attacker held a live credential, and it must outrank the ordinary failures
// so an alert can key on error level alone.
func TestEmit_ReuseDetectionIsAnError(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	New(zerolog.New(&buf)).Emit(context.Background(), Event{Action: ActionRefreshReuseDetected})

	if got := decode(t, &buf)["level"]; got != "error" {
		t.Errorf("level = %v, want error", got)
	}
}

// TestEmit_AtWarnLevelStillWrites is the same property from the other side:
// configure the logger to drop info and the audit record must survive.
func TestEmit_AtWarnLevelStillWrites(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	log := zerolog.New(&buf).Level(zerolog.WarnLevel)

	New(log).Emit(context.Background(), Event{Action: ActionLoginSucceeded, ActorID: "u1"})

	if buf.Len() == 0 {
		t.Fatal("a successful login produced no audit record at warn level")
	}
}

// TestEmit_CarriesTheRequestID makes "what else was this client doing"
// answerable: the audit record and the access-log line share an id.
func TestEmit_CarriesTheRequestID(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	logger := New(zerolog.New(&buf))

	h := chimw.RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logger.Emit(r.Context(), Event{Action: ActionLoginSucceeded, ActorID: "u1"})
	}))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))

	if got, _ := decode(t, &buf)["request_id"].(string); got == "" {
		t.Error("no request_id; the audit record cannot be tied to the request that produced it")
	}
}

// TestEmit_WithoutARequestContextStillWrites: audit events are also emitted
// from background work, where there is no request id to attach.
func TestEmit_WithoutARequestContextStillWrites(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	New(zerolog.New(&buf)).Emit(context.Background(), Event{Action: ActionLogout, ActorID: "u1"})

	rec := decode(t, &buf)
	if _, ok := rec["request_id"]; ok {
		t.Error("request_id was emitted with no request in scope")
	}
	if rec["action"] != string(ActionLogout) {
		t.Errorf("the event was not written: %v", rec)
	}
}

// TestEmit_NilLoggerIsSafe: a nil *Logger must not panic. Panicking here would
// turn a missing wiring into a 500 at precisely the moment a security event
// needed recording.
func TestEmit_NilLoggerIsSafe(t *testing.T) {
	t.Parallel()

	var l *Logger
	l.Emit(context.Background(), Event{Action: ActionLoginFailed})
}
