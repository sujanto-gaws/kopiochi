package auth

import (
	"testing"
	"time"
)

// The lockout rules are the only brute-force defence on the login path, and
// they live entirely in these three methods. They need no database and no
// HTTP — if a mock were required here, the boundary would be in the wrong
// place (docs/architectures/06-quality/testing-strategy.md).

func TestUser_IsLocked(t *testing.T) {
	t.Parallel()

	future := time.Now().Add(time.Hour)
	past := time.Now().Add(-time.Hour)

	tests := []struct {
		name        string
		lockedUntil *time.Time
		want        bool
	}{
		{"never locked", nil, false},
		{"lock still in force", &future, true},
		{"lock has expired", &past, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			u := &User{LockedUntil: tc.lockedUntil}
			if got := u.IsLocked(); got != tc.want {
				t.Errorf("IsLocked() = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestUser_RecordFailedLogin_LocksAtTheThreshold pins the boundary. Off by one
// in either direction is a real defect: locking a attempt early denies a user
// their last legitimate try, and locking a attempt late grants an attacker a
// free guess on every account.
func TestUser_RecordFailedLogin_LocksAtTheThreshold(t *testing.T) {
	t.Parallel()

	const maxAttempts = 3
	u := &User{}

	for i := 1; i < maxAttempts; i++ {
		u.RecordFailedLogin(maxAttempts, time.Hour)
		if u.IsLocked() {
			t.Fatalf("locked after %d of %d attempts; must not lock before the threshold", i, maxAttempts)
		}
	}

	u.RecordFailedLogin(maxAttempts, time.Hour)
	if !u.IsLocked() {
		t.Fatalf("not locked after %d of %d attempts; the threshold does not lock", maxAttempts, maxAttempts)
	}
	if u.FailedLoginAttempts != maxAttempts {
		t.Errorf("FailedLoginAttempts = %d, want %d", u.FailedLoginAttempts, maxAttempts)
	}
}

func TestUser_RecordFailedLogin_UsesTheConfiguredDuration(t *testing.T) {
	t.Parallel()

	u := &User{}
	before := time.Now()
	u.RecordFailedLogin(1, 30*time.Minute)

	if u.LockedUntil == nil {
		t.Fatal("LockedUntil is nil after locking")
	}
	got := u.LockedUntil.Sub(before)
	if got < 29*time.Minute || got > 31*time.Minute {
		t.Errorf("lock duration = %v, want ~30m", got)
	}
}

// TestUser_RecordFailedLogin_KeepsExtendingTheLock: attempts made while
// already locked push the expiry out. A lock that stopped moving would let an
// attacker keep guessing at a fixed rate forever, timed to each expiry.
func TestUser_RecordFailedLogin_KeepsExtendingTheLock(t *testing.T) {
	t.Parallel()

	u := &User{}
	u.RecordFailedLogin(1, time.Hour)
	first := *u.LockedUntil

	u.RecordFailedLogin(1, 2*time.Hour)
	if !u.LockedUntil.After(first) {
		t.Errorf("LockedUntil = %v, want it pushed past %v by a further attempt", *u.LockedUntil, first)
	}
}

// TestUser_ResetFailedLogins_ClearsBoth: clearing the counter but leaving
// LockedUntil set would leave a user locked out with no attempts recorded and
// nothing to expire it early.
func TestUser_ResetFailedLogins_ClearsBoth(t *testing.T) {
	t.Parallel()

	u := &User{}
	u.RecordFailedLogin(1, time.Hour)
	if !u.IsLocked() {
		t.Fatal("precondition failed: user is not locked")
	}

	u.ResetFailedLogins()

	if u.FailedLoginAttempts != 0 {
		t.Errorf("FailedLoginAttempts = %d, want 0", u.FailedLoginAttempts)
	}
	if u.LockedUntil != nil {
		t.Errorf("LockedUntil = %v, want nil", *u.LockedUntil)
	}
	if u.IsLocked() {
		t.Error("still locked after ResetFailedLogins")
	}
}
