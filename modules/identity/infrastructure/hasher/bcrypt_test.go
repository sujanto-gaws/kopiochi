package hasher

import (
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

const password = "correct horse battery staple"

func TestBcryptHasher_VerifiesItsOwnHash(t *testing.T) {
	t.Parallel()

	h := BcryptHasher{}

	hashed, err := h.Hash(password)
	if err != nil {
		t.Fatalf("Hash() error = %v", err)
	}
	if !h.Verify(password, hashed) {
		t.Error("Verify() rejected the password its own Hash() produced")
	}
}

func TestBcryptHasher_RejectsAWrongPassword(t *testing.T) {
	t.Parallel()

	h := BcryptHasher{}
	hashed, err := h.Hash(password)
	if err != nil {
		t.Fatalf("Hash() error = %v", err)
	}

	for _, wrong := range []string{"", "wrong", password + " ", strings.ToUpper(password)} {
		if h.Verify(wrong, hashed) {
			t.Errorf("Verify(%q, ...) = true, want false", wrong)
		}
	}
}

// TestBcryptHasher_IsSalted: two hashes of the same password must differ, or
// identical passwords are visible as identical rows and the table becomes a
// ready-made rainbow-table target.
func TestBcryptHasher_IsSalted(t *testing.T) {
	t.Parallel()

	h := BcryptHasher{}

	first, err := h.Hash(password)
	if err != nil {
		t.Fatalf("Hash() error = %v", err)
	}
	second, err := h.Hash(password)
	if err != nil {
		t.Fatalf("Hash() error = %v", err)
	}

	if first == second {
		t.Error("hashing the same password twice produced the same value; the hash is unsalted")
	}
	if !h.Verify(password, first) || !h.Verify(password, second) {
		t.Error("both salted hashes must still verify")
	}
}

func TestBcryptHasher_HashDoesNotContainThePlaintext(t *testing.T) {
	t.Parallel()

	hashed, err := BcryptHasher{}.Hash(password)
	if err != nil {
		t.Fatalf("Hash() error = %v", err)
	}
	if strings.Contains(hashed, password) {
		t.Errorf("hash %q contains the plaintext", hashed)
	}
}

// TestBcryptHasher_UsesAtLeastTheDefaultCost pins the work factor. Cost is the
// entire defence against offline cracking, and it is a single constant away
// from being lowered to nothing without any test noticing.
func TestBcryptHasher_UsesAtLeastTheDefaultCost(t *testing.T) {
	t.Parallel()

	hashed, err := BcryptHasher{}.Hash(password)
	if err != nil {
		t.Fatalf("Hash() error = %v", err)
	}

	cost, err := bcrypt.Cost([]byte(hashed))
	if err != nil {
		t.Fatalf("bcrypt.Cost() error = %v", err)
	}
	if cost < bcrypt.DefaultCost {
		t.Errorf("bcrypt cost = %d, want at least %d", cost, bcrypt.DefaultCost)
	}
}

// TestBcryptHasher_VerifyRejectsGarbage: Verify must return false for a value
// that is not a bcrypt hash at all, not panic and not somehow succeed. An
// empty PasswordHash column is the realistic case.
func TestBcryptHasher_VerifyRejectsGarbage(t *testing.T) {
	t.Parallel()

	h := BcryptHasher{}
	for _, hashed := range []string{"", "not-a-hash", "$2a$", password} {
		if h.Verify(password, hashed) {
			t.Errorf("Verify(_, %q) = true, want false", hashed)
		}
	}
}

// TestBcryptHasher_RejectsOverlongPasswords records real bcrypt behaviour:
// the algorithm takes at most 72 bytes, and x/crypto returns an error rather
// than silently truncating. Hash must surface that error — a caller that
// ignored it would store an empty string as the hash.
func TestBcryptHasher_RejectsOverlongPasswords(t *testing.T) {
	t.Parallel()

	long := strings.Repeat("a", 73)

	if _, err := (BcryptHasher{}).Hash(long); err == nil {
		t.Error("Hash() of a 73-byte password returned no error; check whether it truncated instead")
	}
}
