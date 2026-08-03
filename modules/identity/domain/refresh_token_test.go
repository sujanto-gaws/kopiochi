package auth

import (
	"strings"
	"testing"
)

// HashToken is what keeps refresh tokens out of the database in plaintext: a
// dump of auth_refresh_tokens must not yield usable credentials.

func TestHashToken_IsDeterministic(t *testing.T) {
	t.Parallel()

	const plain = "a-refresh-token"

	// Via variables rather than inline: staticcheck's SA4000 reads two
	// identical call expressions as a tautology, and it is not wrong to — the
	// point here is specifically that the function is pure.
	first := HashToken(plain)
	second := HashToken(plain)

	if first != second {
		t.Errorf("HashToken is not deterministic (%q vs %q); a stored token could never be looked up again",
			first, second)
	}
}

func TestHashToken_DiffersPerInput(t *testing.T) {
	t.Parallel()

	if HashToken("token-a") == HashToken("token-b") {
		t.Error("two different tokens hash identically")
	}
}

// TestHashToken_DoesNotContainThePlaintext is the property that matters: the
// stored value must be useless to anyone who reads the table.
func TestHashToken_DoesNotContainThePlaintext(t *testing.T) {
	t.Parallel()

	const plain = "super-secret-refresh-token"
	got := HashToken(plain)

	if strings.Contains(got, plain) {
		t.Errorf("hash %q contains the plaintext", got)
	}
	if got == plain {
		t.Error("HashToken returned its input unchanged")
	}
}

// TestHashToken_IsSHA256Hex pins the encoding. Changing it silently would
// invalidate every refresh token already stored, logging out every session
// with no error anywhere — the lookup simply stops matching.
func TestHashToken_IsSHA256Hex(t *testing.T) {
	t.Parallel()

	got := HashToken("anything")
	if len(got) != 64 {
		t.Errorf("len(HashToken(...)) = %d, want 64 hex chars for SHA-256", len(got))
	}
	if strings.TrimLeft(got, "0123456789abcdef") != "" {
		t.Errorf("HashToken(...) = %q, want lowercase hex", got)
	}
}

func TestHashToken_EmptyInputStillHashes(t *testing.T) {
	t.Parallel()

	if got := HashToken(""); len(got) != 64 {
		t.Errorf("HashToken(\"\") = %q, want a 64-char hash rather than an empty string", got)
	}
}
