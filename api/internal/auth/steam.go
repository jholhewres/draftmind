package auth

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"
)

// SteamEndpoint is Steam's OpenID 2.0 provider.
const SteamEndpoint = "https://steamcommunity.com/openid/login"

const (
	openIDNS         = "http://specs.openid.net/auth/2.0"
	selectIdentity   = "http://specs.openid.net/auth/2.0/identifier_select"
	steamStateCookie = "draftmind_steam_state"
	steamStateTTL    = 10 * time.Minute
)

// A Steam claimed id ends in the 64-bit SteamID.
var steamClaimedID = regexp.MustCompile(`^https://steamcommunity\.com/openid/id/([0-9]{17})$`)

// steamProvider is Steam's OpenID 2.0 provider.
type steamProvider struct {
	endpoint string
	client   *http.Client
}

// authURL sends the player to Steam, which returns them to returnTo.
func (p steamProvider) authURL(returnTo, realm string) string {
	q := url.Values{
		"openid.ns":         {openIDNS},
		"openid.mode":       {"checkid_setup"},
		"openid.return_to":  {returnTo},
		"openid.realm":      {realm},
		"openid.identity":   {selectIdentity},
		"openid.claimed_id": {selectIdentity},
	}
	return p.endpoint + "?" + q.Encode()
}

// verify checks the assertion Steam sent to returnTo and asks Steam to
// confirm its signature; it returns the SteamID.
func (p steamProvider) verify(ctx context.Context, q url.Values, returnTo string) (string, error) {
	// A repeated field would be checked here by its first copy and by
	// Steam by another one.
	for k, v := range q {
		if strings.HasPrefix(k, "openid.") && len(v) != 1 {
			return "", fmt.Errorf("%s is repeated", k)
		}
	}
	if q.Get("openid.ns") != openIDNS || q.Get("openid.mode") != "id_res" {
		return "", errors.New("not a positive assertion")
	}
	if q.Get("openid.op_endpoint") != p.endpoint {
		return "", fmt.Errorf("assertion from %q", q.Get("openid.op_endpoint"))
	}
	if q.Get("openid.return_to") != returnTo {
		return "", errors.New("assertion for another return URL")
	}
	claimed := q.Get("openid.claimed_id")
	m := steamClaimedID.FindStringSubmatch(claimed)
	if m == nil || q.Get("openid.identity") != claimed {
		return "", fmt.Errorf("claimed id %q is not a Steam account", claimed)
	}
	signed := strings.Split(q.Get("openid.signed"), ",")
	for _, f := range []string{"op_endpoint", "claimed_id", "identity", "return_to", "response_nonce", "assoc_handle"} {
		if !slices.Contains(signed, f) {
			return "", fmt.Errorf("%s is not signed", f)
		}
	}

	// Steam checks the signature and that the nonce was not used before.
	form := url.Values{}
	for k, v := range q {
		if strings.HasPrefix(k, "openid.") {
			form[k] = v
		}
	}
	form.Set("openid.mode", "check_authentication")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := p.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("ask Steam: %w", err)
	}
	defer func() { _ = res.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(res.Body, 4<<10))
	if err != nil {
		return "", fmt.Errorf("read Steam's answer: %w", err)
	}
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("steam answered HTTP %d", res.StatusCode)
	}
	for line := range strings.SplitSeq(string(body), "\n") {
		if k, v, _ := strings.Cut(strings.TrimSpace(line), ":"); k == "is_valid" && v == "true" {
			return m[1], nil
		}
	}
	return "", errors.New("steam did not confirm the assertion")
}

// steamStart sends the player to Steam with a state the callback checks:
// an assertion meant for another browser does not sign this one in.
func (s *Service) steamStart(w http.ResponseWriter, r *http.Request) {
	state, _ := newSecret()
	s.setCookie(w, "/api/auth/steam", steamStateCookie, state, int(steamStateTTL.Seconds()))
	http.Redirect(w, r, s.steam.authURL(s.steamReturnTo(state), s.public.Scheme+"://"+s.public.Host+"/"), http.StatusFound)
}

// steamCallback signs the player in with the SteamID Steam vouched for,
// then returns them to the web app.
func (s *Service) steamCallback(w http.ResponseWriter, r *http.Request) {
	s.setCookie(w, "/api/auth/steam", steamStateCookie, "", -1)
	app := strings.TrimSuffix(s.public.String(), "/") + "/#/signin"
	q := r.URL.Query()
	c, err := r.Cookie(steamStateCookie)
	if err != nil || c.Value == "" || subtle.ConstantTimeCompare([]byte(c.Value), []byte(q.Get("state"))) != 1 {
		s.log.Warn("auth: steam callback without its state")
		http.Redirect(w, r, app+"/failed", http.StatusFound)
		return
	}
	steamID, err := s.steam.verify(r.Context(), q, s.steamReturnTo(c.Value))
	if err != nil {
		s.log.Warn("auth: steam assertion refused", "err", err)
		http.Redirect(w, r, app+"/failed", http.StatusFound)
		return
	}
	session, err := s.steamSession(r.Context(), steamID, s.now())
	if err != nil {
		s.log.Error("auth: steam session", "err", err)
		http.Redirect(w, r, app+"/failed", http.StatusFound)
		return
	}
	s.setSessionCookie(w, session)
	http.Redirect(w, r, app, http.StatusFound)
}

func (s *Service) steamReturnTo(state string) string {
	return strings.TrimSuffix(s.public.String(), "/") + "/api/auth/steam/callback?state=" + url.QueryEscape(state)
}
