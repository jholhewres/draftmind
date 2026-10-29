package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// signIn opens a session through a magic link and returns its cookie.
func (h *harness) signIn(email string) *http.Cookie {
	h.t.Helper()
	rec := h.openLink(h.requestLink(email))
	c := sessionCookie(rec)
	if rec.Code != http.StatusOK || c == nil {
		h.t.Fatalf("sign in: status %d: %s", rec.Code, rec.Body)
	}
	return c
}

func TestEndpointsThatNeedASessionAnswer401WithoutOne(t *testing.T) {
	h := newHarness(t)
	reached := false
	saved := h.svc.Require(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		if _, ok := UserFrom(r.Context()); !ok {
			t.Error("Require let a request through without its user")
		}
		reached = true
	}))
	save := func(cookies ...*http.Cookie) int {
		req := httptest.NewRequest(http.MethodPost, "/api/history", nil)
		for _, c := range cookies {
			req.AddCookie(c)
		}
		rec := httptest.NewRecorder()
		saved.ServeHTTP(rec, req)
		return rec.Code
	}

	expiring := h.signIn("expiring@example.com")
	h.advance(SessionTTL - time.Second)
	live := h.signIn("live@example.com")
	signedOut := h.signIn("gone@example.com")
	if rec := h.do(http.MethodPost, "/api/auth/signout", "", signedOut); rec.Code != http.StatusNoContent {
		t.Fatalf("sign out: status %d: %s", rec.Code, rec.Body)
	}
	h.advance(time.Second)

	cases := []struct {
		name    string
		cookies []*http.Cookie
	}{
		{"no cookie", nil},
		{"unknown session", []*http.Cookie{{Name: CookieName, Value: "not-a-session"}}},
		{"expired session", []*http.Cookie{expiring}},
		{"signed out", []*http.Cookie{signedOut}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if rec := h.do(http.MethodGet, "/api/me", "", tc.cookies...); rec.Code != http.StatusUnauthorized {
				t.Fatalf("GET /api/me: status %d: %s", rec.Code, rec.Body)
			}
			if code := save(tc.cookies...); code != http.StatusUnauthorized || reached {
				t.Fatalf("a Require'd handler: status %d, reached %t", code, reached)
			}
		})
	}
	if code := save(live); code != http.StatusOK || !reached {
		t.Fatalf("with a live session: status %d, reached %t", code, reached)
	}
}

// checkCookie fails unless c is the session cookie with the attributes ADR-0008 asks for.
func checkCookie(t *testing.T, what string, c *http.Cookie) {
	t.Helper()
	if c == nil || c.Name != CookieName || !c.HttpOnly || !c.Secure || c.SameSite != http.SameSiteLaxMode || c.Path != "/" {
		t.Fatalf("%s: cookie %+v, want HttpOnly, Secure, SameSite=Lax on /", what, c)
	}
}

func TestSessionCookieIsHttpOnlySecureAndLax(t *testing.T) {
	h := newHarness(t)
	rec := h.openLink(h.requestLink("player@example.com"))
	c := sessionCookie(rec)
	checkCookie(t, "magic link", c)
	if c.MaxAge != int(SessionTTL.Seconds()) || len(c.Value) < 43 {
		t.Fatalf("magic link cookie: max age %d, value of %d chars", c.MaxAge, len(c.Value))
	}

	out := h.do(http.MethodPost, "/api/auth/signout", "", c)
	var cleared *http.Cookie
	for _, k := range out.Result().Cookies() {
		if k.Name == CookieName {
			cleared = k
		}
	}
	checkCookie(t, "sign out", cleared)
	if cleared.MaxAge >= 0 || cleared.Value != "" {
		t.Fatalf("sign out keeps the cookie: %+v", cleared)
	}

	// Development on this machine serves plain HTTP: Secure would drop the cookie.
	h.svc.dev = true
	dev := sessionCookie(h.openLink(h.requestLink("dev@example.com")))
	if dev == nil || dev.Secure || !dev.HttpOnly || dev.SameSite != http.SameSiteLaxMode {
		t.Fatalf("development cookie %+v, want HttpOnly and Lax, not Secure", dev)
	}
}
