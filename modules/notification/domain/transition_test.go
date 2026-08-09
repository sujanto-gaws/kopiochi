package domain

import (
	"errors"
	"strings"
	"testing"
)

// TestTransitionMatrix asserts every ordered (from, to) pair over the five
// statuses — 25 of them — not only the seven legal moves.
//
// The forbidden pairs are the point. A legal-moves-only test passes just as
// happily against a transitionTo that accepts everything, and the failure mode
// it misses is the expensive one: a row that goes sent -> pending is delivered
// twice, and a row that goes dead -> sending is retried after the operator
// declared it undeliverable. Both stay forbidden here.
//
// sending -> pending is the seventh arrow: a worker that crashed mid-send left
// the row claimed forever, and stall recovery puts it back in the queue. The
// arrow is in the table, but transitionTo is unexported precisely so that
// travelling it costs an attempt — RecoverStalled is the only way in from
// outside the package.
func TestTransitionMatrix(t *testing.T) {
	allowed := map[transition]bool{
		{StatusPending, StatusSending}: true,
		{StatusSending, StatusSent}:    true,
		{StatusSending, StatusFailed}:  true,
		{StatusSending, StatusDead}:    true,
		{StatusSending, StatusPending}: true,
		{StatusFailed, StatusPending}:  true,
		{StatusFailed, StatusDead}:     true,
	}

	statuses := []Status{StatusPending, StatusSending, StatusSent, StatusFailed, StatusDead}
	if len(statuses) != 5 {
		t.Fatalf("the matrix must cover every status; got %d", len(statuses))
	}

	var pairs, permitted, refused int
	for _, from := range statuses {
		for _, to := range statuses {
			pairs++
			if allowed[transition{from, to}] {
				permitted++
			} else {
				refused++
			}

			t.Run(string(from)+"->"+string(to), func(t *testing.T) {
				n := &Notification{Status: from}
				err := n.transitionTo(to)

				if allowed[transition{from, to}] {
					if err != nil {
						t.Fatalf("%s -> %s must be allowed, got %v", from, to, err)
					}
					if n.Status != to {
						t.Fatalf("status = %q, want %q", n.Status, to)
					}
					return
				}

				if !errors.Is(err, ErrInvalidTransition) {
					t.Fatalf("%s -> %s must be rejected with ErrInvalidTransition, got %v", from, to, err)
				}
				if n.Status != from {
					t.Fatalf("a rejected transition must not mutate the entity: status = %q, want %q", n.Status, from)
				}
			})
		}
	}

	if pairs != 25 {
		t.Fatalf("covered %d pairs, want 25", pairs)
	}
	if permitted != 7 || refused != 18 {
		t.Fatalf("matrix split = %d allowed / %d forbidden, want 7 / 18", permitted, refused)
	}
}

// TestTransitionRejectsUnknownStatus pins the behaviour for a status that is
// not one of the five — a hand-edited database row, or a column value written
// by a future version. It is not a legal source or destination, so it is
// rejected rather than treated as a fresh start.
func TestTransitionRejectsUnknownStatus(t *testing.T) {
	const bogus Status = "queued"

	n := &Notification{Status: bogus}
	if err := n.transitionTo(StatusSending); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("from unknown status: got %v, want ErrInvalidTransition", err)
	}

	n = &Notification{Status: StatusPending}
	if err := n.transitionTo(bogus); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("to unknown status: got %v, want ErrInvalidTransition", err)
	}
	if n.Status != StatusPending {
		t.Fatalf("status = %q, want unchanged %q", n.Status, StatusPending)
	}
}

// TestTransitionErrorNamesBothEnds keeps the diagnostic in the error. An
// operator reading a log line needs to know which move was refused; errors.Is
// still matches, which is what callers branch on.
func TestTransitionErrorNamesBothEnds(t *testing.T) {
	n := &Notification{Status: StatusSent}
	err := n.transitionTo(StatusPending)
	if err == nil {
		t.Fatal("want an error")
	}
	msg := err.Error()
	for _, want := range []string{string(StatusSent), string(StatusPending)} {
		if !strings.Contains(msg, want) {
			t.Fatalf("error %q does not name %q", msg, want)
		}
	}
}
