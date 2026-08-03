package secret

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/go-viper/mapstructure/v2"
)

// TestString_RedactsEverywhere is the core guarantee of this type: no
// formatting or serialization path leaks the underlying value.
func TestString_RedactsEverywhere(t *testing.T) {
	s := String("super-secret-value")

	if got := s.String(); got != redacted {
		t.Errorf("String() = %q, want %q", got, redacted)
	}
	if got := fmt.Sprintf("%v", s); got != redacted {
		t.Errorf("%%v formatting = %q, want %q", got, redacted)
	}
	// staticcheck's S1025 suggests s.String() here. Taking that advice would
	// delete the assertion: the point is that the %s *verb* redacts, which is
	// the path any log line or error message actually takes. Calling String()
	// directly is already covered three lines up.
	//nolint:staticcheck // S1025: exercising the %s verb is the test
	if got := fmt.Sprintf("%s", s); got != redacted {
		t.Errorf("%%s formatting = %q, want %q", got, redacted)
	}
	if got := fmt.Sprintf("%#v", s); got != redacted {
		t.Errorf("%%#v formatting = %q, want %q", got, redacted)
	}

	b, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	if string(b) != `"[REDACTED]"` {
		t.Errorf("json.Marshal = %s, want %q", b, `"[REDACTED]"`)
	}

	// Reveal is the one and only escape hatch.
	if got := s.Reveal(); got != "super-secret-value" {
		t.Errorf("Reveal() = %q, want %q", got, "super-secret-value")
	}
}

// TestString_RedactsInsideAStruct guards against the realistic leak path:
// logging or dumping a config struct that embeds a String field.
func TestString_RedactsInsideAStruct(t *testing.T) {
	type cfg struct {
		Password String
	}
	c := cfg{Password: "hunter2"}

	if got := fmt.Sprintf("%v", c); got != "{[REDACTED]}" {
		t.Errorf("%%v of struct = %q, want it to redact the Password field", got)
	}

	b, err := json.Marshal(c)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	if string(b) != `{"Password":"[REDACTED]"}` {
		t.Errorf("json.Marshal(struct) = %s, want the Password field redacted", b)
	}
}

// TestString_MapstructureDecode proves String decodes correctly from a
// plain string source without a custom mapstructure decode hook: its
// underlying Kind is string, so mapstructure's reflect-based string
// assignment handles it directly. This is exactly the decode path Viper's
// Unmarshal uses for internal/config.Config.
func TestString_MapstructureDecode(t *testing.T) {
	type target struct {
		Password String `mapstructure:"password"`
	}

	input := map[string]interface{}{"password": "from-map"}

	var out target
	if err := mapstructure.Decode(input, &out); err != nil {
		t.Fatalf("mapstructure.Decode: %v", err)
	}
	if got := out.Password.Reveal(); got != "from-map" {
		t.Fatalf("Password.Reveal() = %q, want %q (no decode hook should be necessary)", got, "from-map")
	}
}

// TestString_IsEmpty is a small sanity check for the helper used by
// config validation to detect an unset secret.
func TestString_IsEmpty(t *testing.T) {
	if !String("").IsEmpty() {
		t.Error(`String("").IsEmpty() = false, want true`)
	}
	if String("x").IsEmpty() {
		t.Error(`String("x").IsEmpty() = true, want false`)
	}
}
