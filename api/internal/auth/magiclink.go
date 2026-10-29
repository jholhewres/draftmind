package auth

import (
	"errors"
	"math"
	"net/http"
	"net/mail"
	"strconv"
	"strings"
	"time"
)

// LinkTTL is how long a magic link works; it works once.
const LinkTTL = 15 * time.Minute

// requestLink e-mails a sign-in link to the address. The link opens the
// web app, which posts its token to verifyLink: a mail scanner that
// fetches the link does not spend it. limitEmail, when set, refuses an
// address that asked too often.
func (s *Service) requestLink(limitEmail func(string) time.Duration) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.mailer == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "e-mail sign-in is not set up"})
			return
		}
		var in struct {
			Email string `json:"email"`
			Lang  string `json:"lang"`
		}
		if !decodeJSON(w, r, &in) {
			return
		}
		email, ok := normalizeEmail(in.Email)
		if !ok {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "not an e-mail address"})
			return
		}
		if limitEmail != nil {
			if wait := limitEmail(email); wait > 0 {
				w.Header().Set("Retry-After", strconv.Itoa(int(math.Ceil(wait.Seconds()))))
				writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "too many links asked for this address; try again later"})
				return
			}
		}
		token, err := s.createLoginToken(r.Context(), email, s.now())
		if err != nil {
			s.log.Error("auth: store login token", "err", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not send the link"})
			return
		}
		link := strings.TrimSuffix(s.public.String(), "/") + "/#/signin/link/" + token
		if err := s.mailer.Send(r.Context(), linkMessage(in.Lang, email, link)); err != nil {
			s.log.Error("auth: send link", "err", err) // never the link: it signs in
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "could not send the e-mail"})
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]any{"status": "sent", "minutes": int(LinkTTL.Minutes())})
	})
}

// verifyLink spends the link's token and opens a session.
func (s *Service) verifyLink(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Token string `json:"token"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	now := s.now()
	user, session, err := s.useLoginToken(r.Context(), in.Token, now)
	if errors.Is(err, errLinkUsed) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": err.Error()})
		return
	}
	if err != nil {
		s.log.Error("auth: use login token", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not sign in"})
		return
	}
	s.setSessionCookie(w, session)
	writeJSON(w, http.StatusOK, user)
}

// normalizeEmail accepts a bare address and lower-cases it.
func normalizeEmail(s string) (string, bool) {
	s = strings.TrimSpace(s)
	a, err := mail.ParseAddress(s)
	if err != nil || a.Name != "" || a.Address != s || len(s) > 254 {
		return "", false
	}
	return strings.ToLower(s), true
}
