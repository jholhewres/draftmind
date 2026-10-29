package auth

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
)

// fakeSteam answers check_authentication like Steam's OpenID provider.
type fakeSteam struct {
	*httptest.Server
	valid  bool
	mu     sync.Mutex
	checks []url.Values
}

func newFakeSteam(t *testing.T, h *harness) *fakeSteam {
	f := &fakeSteam{valid: true}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.ParseForm() != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		f.mu.Lock()
		f.checks = append(f.checks, r.PostForm)
		f.mu.Unlock()
		valid := "false"
		if f.valid {
			valid = "true"
		}
		_, _ = w.Write([]byte("ns:http://specs.openid.net/auth/2.0\nis_valid:" + valid + "\n"))
	}))
	t.Cleanup(f.Close)
	h.svc.steam = steamProvider{endpoint: f.endpoint(), client: f.Client()}
	return f
}

func (f *fakeSteam) endpoint() string { return f.URL + "/openid/login" }

func (f *fakeSteam) checked() []url.Values {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.checks
}

// assertion is what Steam appends to return_to once the player signed in.
func assertion(endpoint, returnTo, steamID string) url.Values {
	id := "https://steamcommunity.com/openid/id/" + steamID
	return url.Values{
		"openid.ns":             {"http://specs.openid.net/auth/2.0"},
		"openid.mode":           {"id_res"},
		"openid.op_endpoint":    {endpoint},
		"openid.claimed_id":     {id},
		"openid.identity":       {id},
		"openid.return_to":      {returnTo},
		"openid.response_nonce": {"2026-10-28T12:00:00Zq1w2e3"},
		"openid.assoc_handle":   {"1234567890"},
		"openid.signed":         {"signed,op_endpoint,claimed_id,identity,return_to,response_nonce,assoc_handle"},
		"openid.sig":            {"c2lnbmF0dXJl"},
	}
}

// startSteam follows GET /api/auth/steam: the return URL it sends to Steam
// and the state cookie it sets.
func (h *harness) startSteam(f *fakeSteam) (string, *http.Cookie) {
	h.t.Helper()
	rec := h.do(http.MethodGet, "/api/auth/steam", "")
	if rec.Code != http.StatusFound {
		h.t.Fatalf("start: status %d: %s", rec.Code, rec.Body)
	}
	loc, err := url.Parse(rec.Header().Get("Location"))
	if err != nil || !strings.HasPrefix(loc.String(), f.endpoint()+"?") {
		h.t.Fatalf("start redirects to %q, not to Steam", rec.Header().Get("Location"))
	}
	q := loc.Query()
	selectID := "http://specs.openid.net/auth/2.0/identifier_select"
	if q.Get("openid.ns") != "http://specs.openid.net/auth/2.0" || q.Get("openid.mode") != "checkid_setup" ||
		q.Get("openid.identity") != selectID || q.Get("openid.claimed_id") != selectID || q.Get("openid.realm") != publicURL+"/" ||
		!strings.HasPrefix(q.Get("openid.return_to"), publicURL+"/api/auth/steam/callback?state=") {
		h.t.Fatalf("start asks Steam for %v", q)
	}
	var state *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == steamStateCookie {
			state = c
		}
	}
	if state == nil || !state.HttpOnly || !state.Secure || state.SameSite != http.SameSiteLaxMode {
		h.t.Fatalf("state cookie %+v", state)
	}
	return q.Get("openid.return_to"), state
}

// steamCallback is Steam's redirect back to returnTo with the assertion.
func (h *harness) steamCallback(returnTo string, a url.Values, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	u, err := url.Parse(returnTo)
	if err != nil {
		h.t.Fatal(err)
	}
	return h.do(http.MethodGet, u.Path+"?"+u.RawQuery+"&"+a.Encode(), "", cookies...)
}

func TestSteamSignInVerifiesTheAssertionWithSteam(t *testing.T) {
	h := newHarness(t)
	steam := newFakeSteam(t, h)
	returnTo, state := h.startSteam(steam)

	sent := assertion(steam.endpoint(), returnTo, "76561198000000001")
	rec := h.steamCallback(returnTo, sent, state)
	if rec.Code != http.StatusFound || rec.Header().Get("Location") != publicURL+"/#/signin" {
		t.Fatalf("callback: status %d to %q: %s", rec.Code, rec.Header().Get("Location"), rec.Body)
	}
	cookie := sessionCookie(rec)
	checkCookie(t, "steam", cookie)

	checks := steam.checked()
	if len(checks) != 1 {
		t.Fatalf("asked Steam %d times, want once", len(checks))
	}
	got := checks[0]
	if got.Get("openid.mode") != "check_authentication" {
		t.Fatalf("asked Steam with mode %q", got.Get("openid.mode"))
	}
	for k := range sent {
		if k != "openid.mode" && got.Get(k) != sent.Get(k) {
			t.Fatalf("asked Steam with %s=%q, Steam sent %q", k, got.Get(k), sent.Get(k))
		}
	}
	if _, ok := got["state"]; ok {
		t.Fatal("sent our state to Steam")
	}
	if me := h.do(http.MethodGet, "/api/me", "", cookie); me.Code != http.StatusOK || !strings.Contains(me.Body.String(), `"steamId":"76561198000000001"`) {
		t.Fatalf("me: status %d: %s", me.Code, me.Body)
	}
}

func TestSteamSignInRefusesAnAssertionSteamDoesNotVerify(t *testing.T) {
	cases := []struct {
		name       string
		edit       func(a url.Values, f *fakeSteam)
		noCookie   bool
		askedSteam int
	}{
		{name: "Steam says it is not valid", edit: func(_ url.Values, f *fakeSteam) { f.valid = false }, askedSteam: 1},
		{name: "claimed id from another site", edit: func(a url.Values, _ *fakeSteam) {
			a.Set("openid.claimed_id", "https://evil.example/openid/id/76561198000000001")
			a.Set("openid.identity", a.Get("openid.claimed_id"))
		}},
		{name: "another provider", edit: func(a url.Values, _ *fakeSteam) { a.Set("openid.op_endpoint", "https://evil.example/openid/login") }},
		{name: "another return URL", edit: func(a url.Values, _ *fakeSteam) {
			a.Set("openid.return_to", publicURL+"/api/auth/steam/callback?state=someone-else")
		}},
		{name: "claimed id not signed", edit: func(a url.Values, _ *fakeSteam) {
			a.Set("openid.signed", "signed,op_endpoint,identity,return_to,response_nonce,assoc_handle")
		}},
		{name: "not a positive assertion", edit: func(a url.Values, _ *fakeSteam) { a.Set("openid.mode", "cancel") }},
		{name: "no state cookie", edit: func(url.Values, *fakeSteam) {}, noCookie: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t)
			steam := newFakeSteam(t, h)
			returnTo, state := h.startSteam(steam)
			a := assertion(steam.endpoint(), returnTo, "76561198000000001")
			tc.edit(a, steam)
			var cookies []*http.Cookie
			if !tc.noCookie {
				cookies = append(cookies, state)
			}
			rec := h.steamCallback(returnTo, a, cookies...)
			if rec.Code != http.StatusFound || rec.Header().Get("Location") != publicURL+"/#/signin/failed" {
				t.Fatalf("callback: status %d to %q", rec.Code, rec.Header().Get("Location"))
			}
			if c := sessionCookie(rec); c != nil {
				t.Fatalf("signed in with %+v", c)
			}
			if n := len(steam.checked()); n != tc.askedSteam {
				t.Fatalf("asked Steam %d times, want %d", n, tc.askedSteam)
			}
		})
	}
}
