package middleware

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
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
	got := ParseTrustedProxies([]string{"10.0.0.0/8", "not-a-cidr", "192.168.1.0/24"})
	if len(got) != 2 {
		t.Fatalf("len(ParseTrustedProxies(...)) = %d, want 2 (invalid entry skipped)", len(got))
	}
}

func TestParseTrustedProxies_EmptyMeansTrustNothing(t *testing.T) {
	if got := ParseTrustedProxies(nil); len(got) != 0 {
		t.Fatalf("ParseTrustedProxies(nil) = %v, want empty", got)
	}
	if got := ParseTrustedProxies([]string{}); len(got) != 0 {
		t.Fatalf("ParseTrustedProxies([]) = %v, want empty", got)
	}
}
