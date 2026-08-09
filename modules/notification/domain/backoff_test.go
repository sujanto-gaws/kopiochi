package domain

import (
	"math"
	"testing"
	"time"
)

// fixedJitter is the injected random source. Two lines of hand-written fake are
// the whole reason the source is a parameter: the schedule is asserted on exact
// durations, not on a range.
type fixedJitter struct {
	values []float64
	i      int
}

func (f *fixedJitter) Float64() float64 {
	v := f.values[f.i%len(f.values)]
	f.i++
	return v
}

func TestBackoffDoublesUntilTheCap(t *testing.T) {
	const (
		base = 30 * time.Second
		max  = time.Hour
	)

	tests := []struct {
		attempt int
		want    time.Duration
	}{
		{0, 30 * time.Second},
		{1, time.Minute},
		{2, 2 * time.Minute},
		{3, 4 * time.Minute},
		{4, 8 * time.Minute},
		{5, 16 * time.Minute},
		{6, 32 * time.Minute},
		{7, time.Hour}, // 64m would exceed the cap
		{8, time.Hour},
		{40, time.Hour},
	}

	for _, tt := range tests {
		if got := Backoff(tt.attempt, base, max); got != tt.want {
			t.Errorf("Backoff(%d, 30s, 1h) = %v, want %v", tt.attempt, got, tt.want)
		}
	}
}

// The saturation boundary. base << attempt overflows int64 nanoseconds at
// attempt values a stuck row reaches within days; a wrapped value is negative,
// which would put NextAttemptAt in the past and turn the backoff into a hot
// loop against a downstream that is already failing.
func TestBackoffSaturatesInsteadOfOverflowing(t *testing.T) {
	const maxDur = time.Duration(math.MaxInt64)

	tests := []struct {
		name    string
		attempt int
		base    time.Duration
		cap     time.Duration
		want    time.Duration
	}{
		{"shift past the width of an int64", 63, time.Second, maxDur, maxDur},
		{"far past it", 1000, time.Second, maxDur, maxDur},
		{"product overflows, shift does not", 62, time.Second, maxDur, maxDur},
		{"saturated value still clamps to the cap", 62, time.Second, time.Hour, time.Hour},
		{"largest non-overflowing shift is exact", 1, maxDur / 2, maxDur, (maxDur / 2) << 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Backoff(tt.attempt, tt.base, tt.cap)
			if got != tt.want {
				t.Fatalf("Backoff(%d, %v, %v) = %v, want %v", tt.attempt, tt.base, tt.cap, got, tt.want)
			}
			if got < 0 {
				t.Fatalf("Backoff returned a negative delay (%v): retries would be scheduled in the past", got)
			}
		})
	}
}

// No value of any argument produces a negative delay.
func TestBackoffIsNeverNegative(t *testing.T) {
	bases := []time.Duration{-time.Hour, 0, time.Nanosecond, time.Second, time.Duration(math.MaxInt64)}
	caps := []time.Duration{-time.Hour, 0, time.Second, time.Hour, time.Duration(math.MaxInt64)}
	attempts := []int{-5, 0, 1, 7, 62, 63, 1 << 20}

	for _, b := range bases {
		for _, c := range caps {
			for _, a := range attempts {
				if got := Backoff(a, b, c); got < 0 {
					t.Fatalf("Backoff(%d, %v, %v) = %v, want >= 0", a, b, c, got)
				}
			}
		}
	}
}

func TestBackoffDegenerateInputs(t *testing.T) {
	tests := []struct {
		name    string
		attempt int
		base    time.Duration
		cap     time.Duration
		want    time.Duration
	}{
		// A negative attempt is treated as the first one rather than as a
		// fractional delay.
		{"negative attempt", -3, 30 * time.Second, time.Hour, 30 * time.Second},
		{"zero base", 4, 0, time.Hour, 0},
		{"negative base", 4, -time.Second, time.Hour, 0},
		// A zeroed cap caps at zero. There is no "unlimited" reading, because
		// that turns an unset config field into an unbounded wait.
		{"zero cap", 4, 30 * time.Second, 0, 0},
		{"negative cap", 4, 30 * time.Second, -time.Hour, 0},
		{"cap below base", 0, time.Hour, time.Second, time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Backoff(tt.attempt, tt.base, tt.cap); got != tt.want {
				t.Fatalf("Backoff(%d, %v, %v) = %v, want %v", tt.attempt, tt.base, tt.cap, got, tt.want)
			}
		})
	}
}

func TestBackoffWithJitterIsDeterministicGivenTheSource(t *testing.T) {
	const (
		base = 30 * time.Second
		max  = time.Hour
	)

	tests := []struct {
		name    string
		attempt int
		f       float64
		want    time.Duration
	}{
		// d = 30s: floor is d/2 = 15s, spread is the remaining 15s.
		{"floor of the range", 0, 0, 15 * time.Second},
		{"midpoint", 0, 0.5, 15*time.Second + 7500*time.Millisecond},
		{"three quarters of the spread", 0, 0.75, 26*time.Second + 250*time.Millisecond},
		// d = 1h at the cap.
		{"at the cap, floor", 9, 0, 30 * time.Minute},
		{"at the cap, midpoint", 9, 0.5, 45 * time.Minute},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := &fixedJitter{values: []float64{tt.f}}
			got := BackoffWithJitter(tt.attempt, base, max, src)
			if got != tt.want {
				t.Fatalf("BackoffWithJitter(%d, 30s, 1h, %v) = %v, want %v", tt.attempt, tt.f, got, tt.want)
			}
		})
	}
}

// The whole point of applying jitter inside the cap: the configured maximum
// wait is a maximum, so no draw may push a retry past it.
func TestBackoffWithJitterNeverExceedsTheCapOrDropsBelowHalf(t *testing.T) {
	const (
		base = 30 * time.Second
		max  = time.Hour
	)

	draws := []float64{0, 0.000001, 0.25, 0.5, 0.75, 0.999999}
	src := &fixedJitter{values: draws}

	for attempt := 0; attempt < 20; attempt++ {
		for range draws {
			d := Backoff(attempt, base, max)
			got := BackoffWithJitter(attempt, base, max, src)

			if got > max {
				t.Fatalf("attempt %d: %v exceeds the cap %v", attempt, got, max)
			}
			if got >= d && d > 0 {
				t.Fatalf("attempt %d: %v is not below the un-jittered %v", attempt, got, d)
			}
			if got < d/2 {
				t.Fatalf("attempt %d: %v is below the floor %v — a near-immediate retry against a failing host", attempt, got, d/2)
			}
		}
	}
}

// A source outside its documented [0, 1) contract must not push the result out
// of range either.
func TestBackoffWithJitterClampsAMisbehavingSource(t *testing.T) {
	const (
		base = 30 * time.Second
		max  = time.Hour
	)

	if got := BackoffWithJitter(0, base, max, &fixedJitter{values: []float64{-2}}); got != 15*time.Second {
		t.Errorf("negative draw: got %v, want the floor 15s", got)
	}
	got := BackoffWithJitter(0, base, max, &fixedJitter{values: []float64{1}})
	if got >= 30*time.Second {
		t.Errorf("draw of 1.0: got %v, want strictly below 30s", got)
	}
	if got < 15*time.Second {
		t.Errorf("draw of 1.0: got %v, want at least the floor 15s", got)
	}
}

// A nil source degrades to the un-jittered schedule. A dispatcher that panics
// loses the whole batch; a dispatcher with correlated retries does not.
func TestBackoffWithJitterNilSource(t *testing.T) {
	if got := BackoffWithJitter(2, 30*time.Second, time.Hour, nil); got != 2*time.Minute {
		t.Fatalf("got %v, want the un-jittered 2m", got)
	}
}

// Nothing to spread when the delay is zero, and no division producing a
// negative floor.
func TestBackoffWithJitterZeroDelay(t *testing.T) {
	src := &fixedJitter{values: []float64{0.5}}
	if got := BackoffWithJitter(3, 0, time.Hour, src); got != 0 {
		t.Fatalf("zero base: got %v, want 0", got)
	}
	if got := BackoffWithJitter(3, time.Second, 0, src); got != 0 {
		t.Fatalf("zero cap: got %v, want 0", got)
	}
}

// The scheduling contract the dispatcher relies on: RecordFailure stamps
// now + the delay Backoff computed from the attempt count *before* the
// increment, so the first failure waits base rather than base*2.
func TestBackoffComposesWithRecordFailure(t *testing.T) {
	const (
		base = 30 * time.Second
		max  = time.Hour
	)

	n := &Notification{Status: StatusSending}
	now := testNow

	for _, want := range []time.Duration{30 * time.Second, time.Minute, 2 * time.Minute} {
		delay := Backoff(n.Attempts, base, max)
		if delay != want {
			t.Fatalf("attempt %d: Backoff = %v, want %v", n.Attempts, delay, want)
		}
		if err := n.RecordFailure(now, delay, 10, "boom"); err != nil {
			t.Fatalf("RecordFailure: %v", err)
		}
		if !n.NextAttemptAt.Equal(now.Add(want)) {
			t.Fatalf("NextAttemptAt = %v, want %v", n.NextAttemptAt, now.Add(want))
		}

		now = n.NextAttemptAt
		if err := n.transitionTo(StatusSending); err != nil {
			t.Fatalf("re-claim: %v", err)
		}
	}
}
