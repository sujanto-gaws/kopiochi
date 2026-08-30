package domain

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

// The validation tests that used to fill this file are gone with the fields
// they guarded. A profile has no name and no email (E20) and no field a caller
// supplies, so Validate, isValidEmail, ErrInvalidName and ErrInvalidEmail all
// had nothing left to do.
//
// Deleted rather than kept as tests of an empty Validate: a test that asserts
// nothing can fail is not coverage, it is a green light with no bulb behind it.
//
// What is worth pinning is the property the reshape exists to create.

// TestProfileIsKeyedByAnIdentity is E16 stated as a type.
//
// The IDOR was not a missing ownership check. It was that `users.id` was a
// BIGSERIAL unrelated to any identity, so there was no value a handler COULD
// compare a caller against. The fix is that the profile's id is the identity's
// uuid — which makes ownership expressible, and makes "somebody else's profile"
// a thing this package cannot represent, since the only id it holds is the one
// it was constructed with.
func TestProfileIsKeyedByAnIdentity(t *testing.T) {
	identity := uuid.New()
	u := User{ID: identity, CreatedAt: time.Now(), UpdatedAt: time.Now()}

	if u.ID != identity {
		t.Fatalf("ID = %v, want the identity uuid %v", u.ID, identity)
	}
	if u.ID == uuid.Nil {
		t.Error("a profile keyed by the nil uuid is a profile belonging to nobody")
	}
}

// TestToUserResponseCarriesNoIdentityData: the response is id and timestamps.
// If a name or an email ever reappears here it is a second copy of data the
// identity owns, which is the staleness hazard E15 refused and E20 removed.
func TestToUserResponseCarriesNoIdentityData(t *testing.T) {
	id := uuid.New()
	now := time.Now()

	got := ToUserResponse(&User{ID: id, CreatedAt: now, UpdatedAt: now})
	if got == nil {
		t.Fatal("ToUserResponse returned nil for a real profile")
	}
	if got.ID != id {
		t.Errorf("ID = %v, want %v", got.ID, id)
	}
	if !got.CreatedAt.Equal(now) || !got.UpdatedAt.Equal(now) {
		t.Error("timestamps did not survive the mapping")
	}
}

// TestToUserResponseOfNilIsNil: the service maps whatever the repository
// returned, and a nil profile must not become a response object full of zero
// values that a client would read as a real, empty account.
func TestToUserResponseOfNilIsNil(t *testing.T) {
	if got := ToUserResponse(nil); got != nil {
		t.Errorf("ToUserResponse(nil) = %+v, want nil", got)
	}
}
