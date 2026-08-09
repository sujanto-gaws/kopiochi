package domain

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

var testUser = uuid.MustParse("33333333-3333-4333-8333-333333333333")

// The default that would invert silently during a refactor, pinned: a user with
// no stored rows receives everything. The opposite reading — absence means
// disabled — mutes every existing user the day the feature ships, and nothing
// in the system would report it.
func TestAllowedDefaultsToEnabledWhenNoRecordExists(t *testing.T) {
	for _, prefs := range [][]Preference{nil, {}} {
		for _, ch := range Channels() {
			for _, cat := range Categories() {
				if !Allowed(prefs, ch, cat) {
					t.Errorf("Allowed(%v, %q, %q) = false, want true: absence means enabled", prefs, ch, cat)
				}
			}
		}
	}
}

func TestAllowedHonoursAStoredRow(t *testing.T) {
	prefs := []Preference{
		{UserID: testUser, Channel: ChannelInApp, Category: CategorySystem, Enabled: false},
		{UserID: testUser, Channel: ChannelEmail, Category: CategoryAccount, Enabled: false},
		{UserID: testUser, Channel: ChannelWebhook, Category: CategorySystem, Enabled: true},
	}

	if Allowed(prefs, ChannelInApp, CategorySystem) {
		t.Error("a stored disabled row must filter the enqueue")
	}
	if Allowed(prefs, ChannelEmail, CategoryAccount) {
		t.Error("a stored disabled row must filter the enqueue")
	}
	if !Allowed(prefs, ChannelWebhook, CategorySystem) {
		t.Error("a stored enabled row must allow the enqueue")
	}
	// A row for one pair says nothing about another pair.
	if !Allowed(prefs, ChannelInApp, CategoryAccount) {
		t.Error("an unrelated stored row must not disable a different pair")
	}
}

// The rule with teeth. Preferences filter at enqueue time, so a disabled pair
// means the row is never written — and the absence of "your password was
// changed" is exactly what an attacker wants.
func TestSecurityEmailCannotBeDisabled(t *testing.T) {
	t.Run("the constructor refuses it", func(t *testing.T) {
		p, err := NewPreference(testUser, ChannelEmail, CategorySecurity, false, testNow)
		if !errors.Is(err, ErrPreferenceProtected) {
			t.Fatalf("err = %v, want ErrPreferenceProtected", err)
		}
		if p != (Preference{}) {
			t.Fatalf("preference = %+v, want the zero value on error", p)
		}
		if !strings.Contains(err.Error(), string(ChannelEmail)) || !strings.Contains(err.Error(), string(CategorySecurity)) {
			t.Fatalf("error %q does not name the rejected pair", err)
		}
	})

	t.Run("SetEnabled refuses it", func(t *testing.T) {
		p, err := NewPreference(testUser, ChannelEmail, CategorySecurity, true, testNow)
		if err != nil {
			t.Fatalf("NewPreference: %v", err)
		}

		later := testNow.Add(time.Hour)
		if err := p.SetEnabled(false, later); !errors.Is(err, ErrPreferenceProtected) {
			t.Fatalf("err = %v, want ErrPreferenceProtected", err)
		}
		if !p.Enabled {
			t.Error("Enabled = false after a refused change")
		}
		if !p.UpdatedAt.Equal(testNow) {
			t.Errorf("UpdatedAt = %v, want untouched %v", p.UpdatedAt, testNow)
		}
	})

	t.Run("resolution overrides a row that should not exist", func(t *testing.T) {
		// Written before this rule existed, or by hand in psql. It still must
		// not suppress the mail.
		rogue := []Preference{{
			UserID:   testUser,
			Channel:  ChannelEmail,
			Category: CategorySecurity,
			Enabled:  false,
		}}

		if !Allowed(rogue, ChannelEmail, CategorySecurity) {
			t.Fatal("a stored disabled row must not suppress security email")
		}
	})
}

// The rule is about one pair only. Security in the in-app mailbox, and every
// other category by email, stay the user's to silence — a rule that protected
// more would be a rule users route around by filtering the sender.
func TestOnlySecurityEmailIsProtected(t *testing.T) {
	for _, ch := range Channels() {
		for _, cat := range Categories() {
			if ch == ChannelEmail && cat == CategorySecurity {
				continue
			}

			p, err := NewPreference(testUser, ch, cat, false, testNow)
			if err != nil {
				t.Errorf("NewPreference(%q, %q, disabled) = %v, want it to be allowed", ch, cat, err)
				continue
			}
			if p.Enabled {
				t.Errorf("%q/%q: Enabled = true, want false", ch, cat)
			}
			if Allowed([]Preference{p}, ch, cat) {
				t.Errorf("%q/%q: Allowed = true, want the stored disabled row honoured", ch, cat)
			}
		}
	}
}

func TestNewPreferenceValidation(t *testing.T) {
	tests := []struct {
		name    string
		userID  uuid.UUID
		channel Channel
		cat     Category
		wantMsg string
	}{
		{"missing user", uuid.Nil, ChannelInApp, CategorySystem, "user id is required"},
		{"unknown channel", testUser, "carrier-pigeon", CategorySystem, "unknown channel"},
		{"empty channel", testUser, "", CategorySystem, "unknown channel"},
		{"unknown category", testUser, ChannelInApp, "marketing", "unknown category"},
		{"empty category", testUser, ChannelInApp, "", "unknown category"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Both values of enabled: validation must not be reachable only on
			// the path that also hits the protected-pair check.
			for _, enabled := range []bool{true, false} {
				p, err := NewPreference(tt.userID, tt.channel, tt.cat, enabled, testNow)
				if !errors.Is(err, ErrInvalidPreference) {
					t.Fatalf("enabled=%v: err = %v, want ErrInvalidPreference", enabled, err)
				}
				if p != (Preference{}) {
					t.Fatalf("preference = %+v, want the zero value on error", p)
				}
				if !strings.Contains(err.Error(), tt.wantMsg) {
					t.Fatalf("error %q does not mention %q", err, tt.wantMsg)
				}
			}
		})
	}
}

func TestNewPreferenceCarriesEveryField(t *testing.T) {
	p, err := NewPreference(testUser, ChannelInApp, CategoryAccount, false, testNow)
	if err != nil {
		t.Fatalf("NewPreference: %v", err)
	}

	want := Preference{
		UserID:    testUser,
		Channel:   ChannelInApp,
		Category:  CategoryAccount,
		Enabled:   false,
		UpdatedAt: testNow,
	}
	if p != want {
		t.Fatalf("preference = %+v, want %+v", p, want)
	}
}

func TestSetEnabled(t *testing.T) {
	p, err := NewPreference(testUser, ChannelInApp, CategorySecurity, true, testNow)
	if err != nil {
		t.Fatalf("NewPreference: %v", err)
	}

	later := testNow.Add(2 * time.Hour)
	if err := p.SetEnabled(false, later); err != nil {
		t.Fatalf("SetEnabled(false): %v", err)
	}
	if p.Enabled {
		t.Error("Enabled = true, want false")
	}
	if !p.UpdatedAt.Equal(later) {
		t.Errorf("UpdatedAt = %v, want %v", p.UpdatedAt, later)
	}

	if err := p.SetEnabled(true, later); err != nil {
		t.Fatalf("SetEnabled(true): %v", err)
	}
	if !p.Enabled {
		t.Error("Enabled = false, want true")
	}
}

// Turning a protected pair back on must succeed, so that an idempotent
// "enable everything" request does not fail on the one pair that was never off.
func TestSetEnabledTrueOnProtectedPairIsAllowed(t *testing.T) {
	p, err := NewPreference(testUser, ChannelEmail, CategorySecurity, true, testNow)
	if err != nil {
		t.Fatalf("NewPreference: %v", err)
	}

	later := testNow.Add(time.Hour)
	if err := p.SetEnabled(true, later); err != nil {
		t.Fatalf("SetEnabled(true) on the protected pair: %v", err)
	}
	if !p.Enabled || !p.UpdatedAt.Equal(later) {
		t.Fatalf("preference = %+v", p)
	}
}
