package httpapi

import (
	"net"
	"net/http"
	"net/netip"
	"slices"
	"strings"
)

// WithTrustedProxies names the reverse proxies in front of the server. A
// request from one of them is the client named in its X-Real-IP, else the
// right-most X-Forwarded-For entry, for the rate limits and anything that
// logs the address. Headers from any other address are ignored.
func (s *Server) WithTrustedProxies(addrs ...netip.Addr) *Server {
	s.trusted = addrs
	return s
}

// realIP rewrites RemoteAddr to the forwarded client when the connection
// comes from a trusted proxy.
func (s *Server) realIP(next http.Handler) http.Handler {
	if len(s.trusted) == 0 {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, port, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			next.ServeHTTP(w, r)
			return
		}
		peer, err := netip.ParseAddr(host)
		if err != nil || !slices.Contains(s.trusted, peer.Unmap()) {
			next.ServeHTTP(w, r)
			return
		}
		if client, ok := forwardedClient(r.Header); ok {
			r = r.WithContext(r.Context())
			r.RemoteAddr = net.JoinHostPort(client.String(), port)
		}
		next.ServeHTTP(w, r)
	})
}

// forwardedClient is the address the proxy saw: X-Real-IP, else the last
// X-Forwarded-For hop (the earlier ones are whatever the client sent).
func forwardedClient(h http.Header) (netip.Addr, bool) {
	if a, err := netip.ParseAddr(strings.TrimSpace(h.Get("X-Real-IP"))); err == nil {
		return a.Unmap(), true
	}
	xff := h.Values("X-Forwarded-For")
	if len(xff) == 0 {
		return netip.Addr{}, false
	}
	hops := strings.Split(xff[len(xff)-1], ",")
	if a, err := netip.ParseAddr(strings.TrimSpace(hops[len(hops)-1])); err == nil {
		return a.Unmap(), true
	}
	return netip.Addr{}, false
}
