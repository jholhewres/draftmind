package auth

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/argon2"
)

// Password length bounds, in characters.
const (
	MinPasswordLen = 8
	MaxPasswordLen = 200
)

// argon2id parameters (OWASP's minimum: 19 MiB, 2 passes, 1 lane). They
// go into each hash, so raising them later still checks the old hashes.
const (
	argonMemory  = 19 * 1024 // KiB
	argonTime    = 2
	argonThreads = 1
	argonSaltLen = 16
	argonKeyLen  = 32
)

// errBadLogin is the one answer to a wrong password, an unknown address
// or an account without a password.
var errBadLogin = errors.New("wrong e-mail or password")

// hashSlots caps the hashes running at once: each takes argonMemory.
var hashSlots = make(chan struct{}, 4)

// hashPassword is password's argon2id hash in PHC form:
// $argon2id$v=19$m=…,t=…,p=…$<salt>$<key>.
func hashPassword(password string) (string, error) {
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key := argonKey(password, salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	b64 := base64.RawStdEncoding
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s", argon2.Version, argonMemory, argonTime, argonThreads,
		b64.EncodeToString(salt), b64.EncodeToString(key)), nil
}

func argonKey(password string, salt []byte, t, m uint32, p uint8, n uint32) []byte {
	hashSlots <- struct{}{}
	defer func() { <-hashSlots }()
	return argon2.IDKey([]byte(password), salt, t, m, p, n)
}

// checkPassword says whether password matches the PHC hash. A malformed
// hash, or one with parameters out of bounds, matches nothing.
func checkPassword(hash, password string) bool {
	parts := strings.Split(hash, "$")
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" || parts[2] != "v="+strconv.Itoa(argon2.Version) {
		return false
	}
	var m, t, p uint32
	if n, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &m, &t, &p); err != nil || n != 3 ||
		m < 8 || m > 256*1024 || t < 1 || t > 10 || p < 1 || p > 16 {
		return false
	}
	b64 := base64.RawStdEncoding
	salt, err := b64.DecodeString(parts[4])
	if err != nil || len(salt) < 8 {
		return false
	}
	want, err := b64.DecodeString(parts[5])
	if err != nil || len(want) < 16 || len(want) > 64 {
		return false
	}
	got := argonKey(password, salt, t, m, uint8(p), uint32(len(want))) //nolint:gosec // G115: p is 1..16
	return subtle.ConstantTimeCompare(got, want) == 1
}

// dummyHash is compared against when there is no hash to compare, so an
// unknown address takes as long as a wrong password.
var dummyHash = sync.OnceValue(func() string {
	h, err := hashPassword(base64.RawStdEncoding.EncodeToString(make([]byte, 16)))
	if err != nil {
		panic(err) // crypto/rand does not fail
	}
	return h
})

type credentials struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func passwordLenOK(p string) bool {
	n := utf8.RuneCountInString(p)
	return n >= MinPasswordLen && n <= MaxPasswordLen
}

// emailKey is the address as the per-address limit counts it, valid or not.
func emailKey(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

// limited answers 429 and returns true when limitEmail refuses the address.
func limited(w http.ResponseWriter, limitEmail func(string) time.Duration, email string) bool {
	if limitEmail == nil {
		return false
	}
	wait := limitEmail(emailKey(email))
	if wait <= 0 {
		return false
	}
	w.Header().Set("Retry-After", strconv.Itoa(int(math.Ceil(wait.Seconds()))))
	writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "too many attempts for this address; try again later"})
	return true
}

// register creates an account with a password. A taken address gets the
// same answer and keeps its account untouched, so the route does not tell
// which addresses have one; the client signs in next.
func (s *Service) register(limitEmail func(string) time.Duration) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var in credentials
		if !decodeJSON(w, r, &in) {
			return
		}
		email, ok := normalizeEmail(in.Email)
		if !ok {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "not an e-mail address"})
			return
		}
		if !passwordLenOK(in.Password) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("the password must have %d to %d characters", MinPasswordLen, MaxPasswordLen)})
			return
		}
		if limited(w, limitEmail, email) {
			return
		}
		hash, err := hashPassword(in.Password)
		if err == nil {
			_, err = s.pool.Exec(r.Context(), `INSERT INTO app_user (email, password_hash) VALUES ($1, $2) ON CONFLICT (email) DO NOTHING`, email, hash)
		}
		if err != nil {
			s.log.Error("auth: register", "err", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not create the account"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
}

// passwordLogin opens a session for the address when the password matches.
func (s *Service) passwordLogin(limitEmail func(string) time.Duration) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var in credentials
		if !decodeJSON(w, r, &in) {
			return
		}
		if limited(w, limitEmail, in.Email) {
			return
		}
		user, session, err := s.passwordSession(r.Context(), in, s.now())
		if errors.Is(err, errBadLogin) {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": errBadLogin.Error()})
			return
		}
		if err != nil {
			s.log.Error("auth: password sign-in", "err", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not sign in"})
			return
		}
		s.setSessionCookie(w, session)
		writeJSON(w, http.StatusOK, user)
	})
}

// passwordSession checks the credentials and opens a session. Every
// refusal runs one hash compare, against the dummy hash when there is no
// account or no password.
func (s *Service) passwordSession(ctx context.Context, in credentials, now time.Time) (User, string, error) {
	email, ok := normalizeEmail(in.Email)
	ok = ok && passwordLenOK(in.Password)
	var u User
	var hash *string
	if ok {
		err := s.pool.QueryRow(ctx, `SELECT id, email, coalesce(steam_id, ''), password_hash FROM app_user WHERE email = $1`, email).
			Scan(&u.ID, &u.Email, &u.SteamID, &hash)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return User{}, "", err
		}
	}
	if hash == nil {
		checkPassword(dummyHash(), in.Password)
		return User{}, "", errBadLogin
	}
	if !checkPassword(*hash, in.Password) {
		return User{}, "", errBadLogin
	}
	session, err := createSession(ctx, s.pool, u.ID, now)
	return u, session, err
}

// methods says which ways in are on: e-mail links only with a mailer.
func (s *Service) methods(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, struct {
		Password  bool `json:"password"`
		Steam     bool `json:"steam"`
		EmailLink bool `json:"emailLink"`
	}{true, true, s.mailer != nil})
}
