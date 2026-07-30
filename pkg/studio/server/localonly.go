package server

import (
	"net"
	"net/http"
	"net/url"
	"strings"
)

// localOnly wraps next with a Host/Origin guard so the Studio API (which has
// no auth of its own) is only reachable from the local machine. It rejects
// any request whose Host header does not name a loopback address — defeating
// DNS-rebinding, where an attacker's page resolves a hostname to 127.0.0.1
// only after the browser has already committed to sending that hostname as
// Host — and any request that carries a non-loopback Origin header, which
// blocks cross-site browser requests (e.g. a POST from another site's page).
// Requests with no Origin header (curl, same-origin navigations) pass the
// Origin check.
func localOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isLoopbackHost(r.Host) {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden: non-loopback host"})
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" {
			u, err := url.Parse(origin)
			if err != nil || !isLoopbackHost(u.Host) {
				writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden: non-loopback origin"})
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// isLoopbackHost reports whether hostport — a hostname with an optional
// ":port" (e.g. "127.0.0.1:8787", "localhost", "[::1]:8787") — names the
// local machine: 127.0.0.1, localhost, or ::1.
func isLoopbackHost(hostport string) bool {
	host, _, err := net.SplitHostPort(hostport)
	if err != nil {
		// No port present (e.g. bare "localhost" or "::1"); use as-is, minus
		// any IPv6 brackets.
		host = strings.Trim(hostport, "[]")
	}
	switch host {
	case "127.0.0.1", "localhost", "::1":
		return true
	default:
		return false
	}
}
