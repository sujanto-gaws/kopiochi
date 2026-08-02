// Package secret provides a string wrapper that keeps sensitive
// configuration values (DB passwords, JWT/HMAC secrets, ...) out of logs and
// error messages by construction, rather than relying on every call site to
// remember not to print them.
package secret

import "encoding/json"

// redacted is what every default formatting/serialization path produces
// instead of the real value.
const redacted = "[REDACTED]"

// String is a string that redacts itself everywhere except through the
// explicit Reveal method. It implements fmt.Stringer and fmt.GoStringer, so
// "%v", "%s", and "%#v" formatting all print "[REDACTED]" instead of the
// underlying value, and it implements json.Marshaler so it never appears in
// clear text in a JSON-encoded config dump.
//
// mapstructure decodes plain strings into String without a custom decode
// hook: its underlying kind is string, so reflect's SetString path handles
// it directly (verified in config_test.go).
type String string

// String implements fmt.Stringer.
func (s String) String() string { return redacted }

// GoString implements fmt.GoStringer (used for the "%#v" verb).
func (s String) GoString() string { return redacted }

// MarshalJSON implements json.Marshaler. It intentionally never emits the
// underlying value.
func (s String) MarshalJSON() ([]byte, error) {
	return json.Marshal(redacted)
}

// UnmarshalJSON implements json.Unmarshaler so a String field can still be
// populated from a legitimate JSON source (e.g. a request body); only
// MarshalJSON is one-way.
func (s *String) UnmarshalJSON(data []byte) error {
	var v string
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	*s = String(v)
	return nil
}

// Reveal returns the underlying value. Every call site is a place the
// secret legitimately needs to leave this type — grep for "Reveal(" in
// review.
func (s String) Reveal() string { return string(s) }

// IsEmpty reports whether the underlying value is the empty string.
func (s String) IsEmpty() bool { return s == "" }
