package middleware

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"

	"github.com/rs/zerolog"
)

// TestRealIP_IgnoresForwardedHeadersWithoutTrustedProxies is the direct
// regression test for the proxy-header defect in
// docs/architectures/04-security/middleware-hardening.md: with no trusted
// proxies configured (the default), forwarded headers must never override
// the socket address, no matter what a client sends.
func TestRealIP_IgnoresForwardedHeadersWithoutTrustedProxies(t *testing.T) {
	var got string
	h := RealIP(nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = ClientIP(r.Context())
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "203.0.113.9:54321"
	req.Header.Set("X-Forwarded-For", "1.2.3.4")
	req.Header.Set("X-Real-Ip", "5.6.7.8")

	h.ServeHTTP(httptest.NewRecorder(), req)

	if got != "203.0.113.9" {
		t.Fatalf("ClientIP = %q, want raw peer %q (headers must be ignored: no trusted proxies configured)", got, "203.0.113.9")
	}
}

func TestRealIP_TrustsForwardedHeaderOnlyFromTrustedPeer(t *testing.T) {
	trusted := []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")}

	var got string
	h := RealIP(trusted)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = ClientIP(r.Context())
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.5:443" // trusted proxy hop
	req.Header.Set("X-Forwarded-For", "203.0.113.9, 10.0.0.5")

	h.ServeHTTP(httptest.NewRecorder(), req)

	if got != "203.0.113.9" {
		t.Fatalf("ClientIP = %q, want resolved client %q", got, "203.0.113.9")
	}
}

func TestRealIP_IgnoresForwardedHeaderFromUntrustedPeerEvenIfProxiesConfigured(t *testing.T) {
	trusted := []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")}

	var got string
	h := RealIP(trusted)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = ClientIP(r.Context())
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "203.0.113.9:54321" // NOT inside the trusted CIDR
	req.Header.Set("X-Forwarded-For", "6.6.6.6")

	h.ServeHTTP(httptest.NewRecorder(), req)

	if got != "203.0.113.9" {
		t.Fatalf("ClientIP = %q, want raw peer %q: peer is not a trusted proxy", got, "203.0.113.9")
	}
}

func TestRealIP_WalksMultipleTrustedHopsToFindRealClient(t *testing.T) {
	trusted := []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")}

	var got string
	h := RealIP(trusted)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = ClientIP(r.Context())
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.5:443"
	// Two trusted hops (10.0.0.1 added a load balancer hop, 10.0.0.5 is the
	// immediate peer) in front of the real client.
	req.Header.Set("X-Forwarded-For", "198.51.100.7, 10.0.0.1, 10.0.0.5")

	h.ServeHTTP(httptest.NewRecorder(), req)

	if got != "198.51.100.7" {
		t.Fatalf("ClientIP = %q, want %q", got, "198.51.100.7")
	}
}

func TestParseTrustedProxies_SkipsInvalidEntries(t *testing.T) {
	got := ParseTrustedProxies([]string{"10.0.0.0/8", "not-a-cidr", "192.168.1.0/24"}, zerolog.Nop())
	if len(got) != 2 {
		t.Fatalf("len(ParseTrustedProxies(...)) = %d, want 2 (invalid entry skipped)", len(got))
	}
}

// TestParseTrustedProxies_WarnsAboutSkippedEntries: a dropped entry silently
// narrows who may set X-Forwarded-For, so the real client IP starts resolving
// to the proxy's address and every rate limit and audit line is wrong. That is
// a security-relevant config change and must not pass unremarked — this is the
// assertion the package-global logger made impossible to write.
func TestParseTrustedProxies_WarnsAboutSkippedEntries(t *testing.T) {
	var buf bytes.Buffer

	ParseTrustedProxies([]string{"10.0.0.0/8", "192.168.1.0/99"}, zerolog.New(&buf))

	out := buf.String()
	if !strings.Contains(out, "192.168.1.0/99") {
		t.Errorf("the skipped CIDR is not named in the log: %s", out)
	}
	if !strings.Contains(out, `"level":"warn"`) {
		t.Errorf("skipping a trusted-proxy entry was not logged at warn level: %s", out)
	}
}

func TestParseTrustedProxies_EmptyMeansTrustNothing(t *testing.T) {
	if got := ParseTrustedProxies(nil, zerolog.Nop()); len(got) != 0 {
		t.Fatalf("ParseTrustedProxies(nil, zerolog.Nop()) = %v, want empty", got)
	}
	if got := ParseTrustedProxies([]string{}, zerolog.Nop()); len(got) != 0 {
		t.Fatalf("ParseTrustedProxies([]) = %v, want empty", got)
	}
}
