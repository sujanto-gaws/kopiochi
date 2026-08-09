package domain

import (
	"math"
	"testing"
)

// Limit has one job — bound how much of the table a single request can read —
// and it only does that job at the edges, which is where callers and D4 will
// actually hit it.
func TestListFilterNormalizeClampsLimit(t *testing.T) {
	tests := []struct {
		name string
		in   int
		want int
	}{
		{"zero is unspecified, not an empty page", 0, DefaultListLimit},
		{"negative is unspecified too", -1, DefaultListLimit},
		{"the most negative int is still just unspecified", math.MinInt, DefaultListLimit},
		{"one is a legitimate request and survives", 1, 1},
		{"one below the default is left alone", DefaultListLimit - 1, DefaultListLimit - 1},
		{"the default asked for explicitly is left alone", DefaultListLimit, DefaultListLimit},
		{"one below the ceiling is left alone", MaxListLimit - 1, MaxListLimit - 1},
		{"exactly the ceiling is allowed, not capped to one less", MaxListLimit, MaxListLimit},
		{"one above the ceiling is capped", MaxListLimit + 1, MaxListLimit},
		{"a large value is capped", 1_000_000, MaxListLimit},
		{"the largest int is capped", math.MaxInt, MaxListLimit},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ListFilter{Limit: tt.in}.Normalize()
			if got.Limit != tt.want {
				t.Errorf("ListFilter{Limit: %d}.Normalize().Limit = %d, want %d", tt.in, got.Limit, tt.want)
			}
		})
	}
}

// The bounds themselves, asserted rather than assumed: the table above is
// written in terms of the constants, so it would still pass if the ceiling were
// below the default — a configuration in which Normalize can never return the
// default it documents.
func TestListLimitBoundsAreCoherent(t *testing.T) {
	if DefaultListLimit < 1 {
		t.Errorf("DefaultListLimit = %d, want at least 1: a non-positive default normalizes to an empty page", DefaultListLimit)
	}
	if MaxListLimit < DefaultListLimit {
		t.Errorf("MaxListLimit = %d < DefaultListLimit = %d: the default would itself be capped", MaxListLimit, DefaultListLimit)
	}
}

// Normalize is the value the repository is supposed to trust, so it must not
// quietly drop the rest of the filter on the way through.
func TestListFilterNormalizePreservesEveryOtherField(t *testing.T) {
	before := &Cursor{CreatedAt: testNow, ID: testID}
	f := ListFilter{UnreadOnly: true, Limit: 0, Before: before}

	got := f.Normalize()

	if !got.UnreadOnly {
		t.Error("UnreadOnly was dropped: an unread-only page would silently include read rows")
	}
	if got.Before != before {
		t.Errorf("Before = %v, want the same cursor %v: losing it restarts the caller at page one", got.Before, before)
	}
}

// A value receiver returning a copy. A caller that normalizes for a bounds check
// and then passes the original along must not find it rewritten, and a
// repository that normalizes a filter its caller already normalized must get the
// same answer.
func TestListFilterNormalizeDoesNotMutateAndIsIdempotent(t *testing.T) {
	f := ListFilter{Limit: 1_000_000}

	once := f.Normalize()
	if f.Limit != 1_000_000 {
		t.Errorf("receiver Limit = %d after Normalize, want it untouched at 1000000", f.Limit)
	}

	twice := once.Normalize()
	if twice != once {
		t.Errorf("Normalize(Normalize(f)) = %+v, want %+v", twice, once)
	}
}

// The whole reason the cursor is a pair. Two rows written by one fan-out share
// CreatedAt to the microsecond, so a cursor on the timestamp alone cannot name
// which of them the caller has already seen; the id is what makes the position
// unambiguous.
func TestCursorDistinguishesRowsSharingATimestamp(t *testing.T) {
	a := Cursor{CreatedAt: testNow, ID: testID}
	b := Cursor{CreatedAt: testNow, ID: testRecipient}

	if a.CreatedAt != b.CreatedAt {
		t.Fatal("the fixtures must share a timestamp for this test to mean anything")
	}
	if a == b {
		t.Fatal("two cursors at the same instant compared equal: the id is not discriminating, and a page boundary would drop or repeat rows")
	}
}
