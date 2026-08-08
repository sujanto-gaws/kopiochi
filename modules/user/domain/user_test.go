package domain

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestUser_Validate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		user    User
		wantErr error
	}{
		{"valid", User{Name: "Alice", Email: "alice@example.com"}, nil},
		{"empty name", User{Name: "", Email: "alice@example.com"}, ErrInvalidName},
		{"whitespace name", User{Name: "   ", Email: "alice@example.com"}, ErrInvalidName},
		{"empty email", User{Name: "Alice", Email: ""}, ErrInvalidEmail},
		{"whitespace email", User{Name: "Alice", Email: "   "}, ErrInvalidEmail},
		{"no at sign", User{Name: "Alice", Email: "alice.example.com"}, ErrInvalidEmail},
		{"two at signs", User{Name: "Alice", Email: "a@b@example.com"}, ErrInvalidEmail},
		{"empty local part", User{Name: "Alice", Email: "@example.com"}, ErrInvalidEmail},
		{"domain without a dot", User{Name: "Alice", Email: "alice@localhost"}, ErrInvalidEmail},
		{"too short", User{Name: "Alice", Email: "a@b"}, ErrInvalidEmail},
		{"name is checked before email", User{Name: "", Email: "nonsense"}, ErrInvalidName},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := tc.user.Validate()
			if !errors.Is(err, tc.wantErr) {
				t.Errorf("Validate() = %v, want %v", err, tc.wantErr)
			}
		})
	}
}

// TestUser_Validate_AcceptsRealisticAddresses guards against tightening the
// rule into something that rejects valid mail: plus-addressing and subdomains
// are ordinary and a validator that refuses them locks real users out.
func TestUser_Validate_AcceptsRealisticAddresses(t *testing.T) {
	t.Parallel()

	addresses := []string{
		"alice+tag@example.com",
		"alice.smith@mail.example.co.uk",
		"a@b.co",
		"ALICE@EXAMPLE.COM",
	}

	for _, addr := range addresses {
		t.Run(addr, func(t *testing.T) {
			t.Parallel()

			u := User{Name: "Alice", Email: addr}
			if err := u.Validate(); err != nil {
				t.Errorf("Validate() rejected %q: %v", addr, err)
			}
		})
	}
}

// TestUser_Validate_DoesNotTrimIntoValidity: the name check trims before
// testing for empty, so a name of only spaces is rejected — but the stored
// value is whatever was supplied. This records that Validate does not mutate.
func TestUser_Validate_DoesNotMutate(t *testing.T) {
	t.Parallel()

	u := User{Name: "  Alice  ", Email: "alice@example.com"}
	if err := u.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if u.Name != "  Alice  " {
		t.Errorf("Validate() rewrote Name to %q; it is a check, not a normaliser", u.Name)
	}
}

func TestToUserResponse_NilIsNil(t *testing.T) {
	t.Parallel()

	if got := ToUserResponse(nil); got != nil {
		t.Errorf("ToUserResponse(nil) = %+v, want nil", got)
	}
}

func TestToUserResponse_CopiesEveryField(t *testing.T) {
	t.Parallel()

	now := time.Now()
	u := &User{ID: 7, Name: "Alice", Email: "alice@example.com", CreatedAt: now, UpdatedAt: now}

	got := ToUserResponse(u)
	if got.ID != u.ID || got.Name != u.Name || got.Email != u.Email {
		t.Errorf("ToUserResponse() = %+v, want it to carry id/name/email from %+v", got, u)
	}
	if !got.CreatedAt.Equal(now) || !got.UpdatedAt.Equal(now) {
		t.Errorf("timestamps not carried through: %+v", got)
	}
}

// TestToUserResponses_EmptySliceIsNotNil keeps the JSON stable: a nil slice
// marshals to null and an empty one to [], and a client iterating the result
// breaks on the former.
func TestToUserResponses_EmptySliceIsNotNil(t *testing.T) {
	t.Parallel()

	got := ToUserResponses(nil)
	if got == nil {
		t.Fatal("ToUserResponses(nil) = nil, want an empty slice so it marshals to [] not null")
	}
	if len(got) != 0 {
		t.Errorf("len = %d, want 0", len(got))
	}
}

func TestToUserResponses_PreservesOrder(t *testing.T) {
	t.Parallel()

	users := []*User{{ID: 1, Name: "A"}, {ID: 2, Name: "B"}, {ID: 3, Name: "C"}}

	got := ToUserResponses(users)
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3", len(got))
	}
	for i, want := range []int64{1, 2, 3} {
		if got[i].ID != want {
			t.Errorf("got[%d].ID = %d, want %d", i, got[i].ID, want)
		}
	}
}

func TestCreateUserRequest_ToDomain(t *testing.T) {
	t.Parallel()

	req := &CreateUserRequest{Name: "Alice", Email: "alice@example.com"}

	u := req.ToDomain()
	if u.Name != req.Name || u.Email != req.Email {
		t.Errorf("ToDomain() = %+v, want name/email from %+v", u, req)
	}
	// A client-supplied id would let a caller overwrite an arbitrary row.
	if u.ID != 0 {
		t.Errorf("ToDomain() set ID = %d; the id must come from the database, not the request", u.ID)
	}
}

func TestUpdateUserRequest_ToDomain(t *testing.T) {
	t.Parallel()

	req := &UpdateUserRequest{Name: "Alice", Email: "alice@example.com"}

	u := req.ToDomain()
	if u.Name != req.Name || u.Email != req.Email {
		t.Errorf("ToDomain() = %+v, want name/email from %+v", u, req)
	}
	if u.ID != 0 {
		t.Errorf("ToDomain() set ID = %d; the id comes from the route, not the body", u.ID)
	}
}

// TestErrors_AreDistinct: the transport layer maps these to different status
// codes, so two of them being the same value would collapse those responses.
func TestErrors_AreDistinct(t *testing.T) {
	t.Parallel()

	all := []error{ErrInvalidName, ErrInvalidEmail, ErrUserNotFound}
	for i := range all {
		for j := range all {
			if i != j && errors.Is(all[i], all[j]) {
				t.Errorf("%v and %v are the same error value", all[i], all[j])
			}
		}
	}
	for _, err := range all {
		if strings.TrimSpace(err.Error()) == "" {
			t.Errorf("error %v has an empty message", err)
		}
	}
}
