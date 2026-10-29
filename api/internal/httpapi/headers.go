package httpapi

import (
	_ "embed"
	"net/http"
	"strings"
)

// csp lets the app load only its own scripts, styles, fonts and API, and
// images from the game's image CDN (hero portraits, item icons,
// backgrounds). Nothing may frame it. The web build's preview server sends
// the same file, so the browser tests run under this policy.
//
//go:embed csp.txt
var csp string

var contentSecurityPolicy = strings.TrimSpace(csp)

// securityHeaders sets the browser's security headers on every response.
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", contentSecurityPolicy)
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		h.Set("X-Frame-Options", "DENY") // frame-ancestors for browsers without CSP 2
		next.ServeHTTP(w, r)
	})
}
