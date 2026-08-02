package middleware

import (
	"context"
	"net"
	"net/http"
	"net/netip"
	"strings"

	"github.com/rs/zerolog/log"
)

// clientIPKey is the context key RealIP uses to store the resolved client
// address. It is unexported so ClientIP is the only way to read it.
type clientIPKey struct{}

// ClientIP returns the client IP resolved by RealIP for this request, or ""
// if RealIP has not run against it. Callers that need the client's address
// (rate limiting, request logging, auditing) must read it from here rather
// than re-parsing X-Forwarded-For / X-Real-Ip themselves -- those headers
// are attacker-controlled and only RealIP knows whether the immediate peer
// was trusted enough to honor them.
func ClientIP(ctx context.Context) string {
	ip, _ := ctx.Value(clientIPKey{}).(string)
	return ip
}

// ParseTrustedProxies converts configured CIDR strings into netip.Prefix
// values for RealIP. Invalid entries are logged and skipped rather than
// aborting startup -- full config validation is a later phase -- but note
// that an empty, nil, or all-invalid input all mean the same thing to
// RealIP: trust nothing, always use the socket address.
func ParseTrustedProxies(cidrs []string) []netip.Prefix {
	prefixes := make([]netip.Prefix, 0, len(cidrs))
	for _, c := range cidrs {
		p, err := netip.ParsePrefix(strings.TrimSpace(c))
		if err != nil {
			log.Warn().Str("cidr", c).Err(err).Msg("ignoring invalid trusted_proxies entry")
			continue
		}
		prefixes = append(prefixes, p)
	}
	return prefixes
}

// RealIP resolves the request's client IP and stores it in the request
// context for downstream consumers (request logging, rate limiting, ...).
//
// Forwarded headers (X-Forwarded-For, X-Real-Ip) are honored ONLY when the
// immediate TCP peer's address falls inside one of the given trusted CIDR
// ranges -- i.e. the peer is a proxy we operate or otherwise trust to set
// those headers honestly. An empty trusted list, which is the default,
// means the headers are never consulted and the socket address is always
// used: safe when the service is reachable directly, and safe by default
// even before a deployment behind a proxy has been configured correctly.
//
// This replaces chi's middleware.RealIP, which trusts forwarded headers
// from any peer unconditionally.
func RealIP(trusted []netip.Prefix) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := resolveClientIP(r, trusted)
			ctx := context.WithValue(r.Context(), clientIPKey{}, ip)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func resolveClientIP(r *http.Request, trusted []netip.Prefix) string {
	peer := peerAddr(r.RemoteAddr)

	if peer == "" || len(trusted) == 0 || !isTrusted(peer, trusted) {
		// No identifiable peer, nothing configured to trust, or the
		// immediate hop is not a trusted proxy: never consult forwarded
		// headers, they are attacker-controlled input.
		return peer
	}

	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		// Walk right-to-left: each hop prepends itself, so the entries
		// closest to us are the most recently added and, if trusted, are
		// proxies we operate. The first entry (scanning from the right)
		// that is NOT itself a trusted proxy is the real client.
		for i := len(parts) - 1; i >= 0; i-- {
			candidate := strings.TrimSpace(parts[i])
			addr, err := netip.ParseAddr(candidate)
			if err != nil {
				continue
			}
			if !isTrusted(candidate, trusted) {
				return addr.String()
			}
		}
	}

	if realIP := strings.TrimSpace(r.Header.Get("X-Real-Ip")); realIP != "" {
		if addr, err := netip.ParseAddr(realIP); err == nil {
			return addr.String()
		}
	}

	return peer
}

// peerAddr extracts and normalizes the host portion of a RemoteAddr
// (host:port). Falls back to the raw value if it can't be split or parsed,
// rather than returning empty and silently losing the address entirely.
func peerAddr(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	addr, err := netip.ParseAddr(host)
	if err != nil {
		return host
	}
	return addr.String()
}

func isTrusted(ip string, trusted []netip.Prefix) bool {
	addr, err := netip.ParseAddr(ip)
	if err != nil {
		return false
	}
	for _, prefix := range trusted {
		if prefix.Contains(addr) {
			return true
		}
	}
	return false
}
