package auth

import (
	"context"
	"encoding/json"
	"errors"
	"mime"
	"net/http"
	"time"
)

// SessionTTL is how long a session lasts from sign-in.
const SessionTTL = 30 * 24 * time.Hour

type userKey struct{}

// UserFrom is the user Require put on the request's context.
func UserFrom(ctx context.Context) (User, bool) {
	u, ok := ctx.Value(userKey{}).(User)
	return u, ok
}

// Require answers 401 unless the request carries a live session; next
// reads the user with UserFrom.
func (s *Service) Require(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(CookieName)
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "sign in first"})
			return
		}
		user, err := s.sessionUser(r.Context(), c.Value, s.now())
		if errors.Is(err, errNoSession) {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "sign in first"})
			return
		}
		if err != nil {
			s.log.Error("auth: read session", "err", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not read the session"})
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userKey{}, user)))
	})
}

func (s *Service) me(w http.ResponseWriter, r *http.Request) {
	u, _ := UserFrom(r.Context())
	writeJSON(w, http.StatusOK, u)
}

// signOut ends the request's session, if any, and clears the cookie.
func (s *Service) signOut(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(CookieName); err == nil {
		if err := s.deleteSession(r.Context(), c.Value); err != nil {
			s.log.Error("auth: delete session", "err", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not sign out"})
			return
		}
	}
	s.setCookie(w, "/", CookieName, "", -1)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Service) setSessionCookie(w http.ResponseWriter, token string) {
	s.setCookie(w, "/", CookieName, token, int(SessionTTL.Seconds()))
}

// setCookie sets an HttpOnly, SameSite=Lax cookie, Secure unless the web
// app is served on this machine over plain HTTP.
func (s *Service) setCookie(w http.ResponseWriter, path, name, value string, maxAge int) {
	// Secure is off only for http://localhost (New refuses plain HTTP elsewhere).
	http.SetCookie(w, &http.Cookie{Name: name, Value: value, Path: path, MaxAge: maxAge, //nolint:gosec // G124: see above
		HttpOnly: true, Secure: !s.dev, SameSite: http.SameSiteLaxMode})
}

// decodeJSON reads a small JSON body. A form a foreign page posts cannot
// set this content type without a CORS preflight.
func decodeJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	if mt, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type")); mt != "application/json" {
		writeJSON(w, http.StatusUnsupportedMediaType, map[string]string{"error": "send application/json"})
		return false
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(v); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "malformed JSON"})
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
