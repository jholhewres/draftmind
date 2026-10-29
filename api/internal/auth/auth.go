// Package auth signs people in with a password, an e-mail link or Steam and keeps
// their server-side sessions (ADR-0008, ADR-0010).
package auth

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// CookieName names the session cookie.
const CookieName = "draftmind_session"

// Message is one e-mail. Link is the sign-in link its body carries.
type Message struct {
	To, Subject, Body, Link string
}

// Mailer sends e-mail.
type Mailer interface {
	Send(ctx context.Context, m Message) error
}

// Service holds the sign-in routes and the sessions.
type Service struct {
	pool   *pgxpool.Pool
	public *url.URL // where the web app is served; links and Steam return there
	dev    bool     // the web app is served on this machine: plain HTTP cookies, links may go to the log
	mailer Mailer   // nil turns e-mail sign-in off
	steam  steamProvider
	now    func() time.Time
	log    *slog.Logger
}

// New builds the service; publicURL is the web app's address, e.g.
// https://draftmind.example. A loopback host (localhost, 127.0.0.1, ::1)
// is development.
func New(pool *pgxpool.Pool, publicURL string, log *slog.Logger) (*Service, error) {
	u, err := url.Parse(publicURL)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return nil, fmt.Errorf("public URL %q must be an absolute http(s) URL", publicURL)
	}
	dev := isLoopback(u.Hostname())
	if !dev && u.Scheme != "https" {
		return nil, fmt.Errorf("public URL %q must be https outside this machine", publicURL)
	}
	steam := steamProvider{endpoint: SteamEndpoint, client: &http.Client{Timeout: 10 * time.Second}}
	return &Service{pool: pool, public: u, dev: dev, steam: steam, now: time.Now, log: log}, nil
}

func isLoopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// Dev says whether the web app is served on this machine.
func (s *Service) Dev() bool { return s.dev }

// WithMailer turns e-mail sign-in on.
func (s *Service) WithMailer(m Mailer) *Service {
	s.mailer = m
	return s
}

// Limits are the rate limits Mount puts on the sign-in routes. Each IP
// field wraps routes per client IP; each Email field, given the address,
// says how long until it may try again (0: now). Any may be nil.
type Limits struct {
	LinkIP        func(http.Handler) http.Handler // asking for a magic link
	LinkEmail     func(email string) time.Duration
	PasswordIP    func(http.Handler) http.Handler // register and password sign-in
	PasswordEmail func(email string) time.Duration
}

// Mount adds the sign-in routes to mux.
func (s *Service) Mount(mux *http.ServeMux, l Limits) {
	limitIP := func(wrap func(http.Handler) http.Handler, h http.Handler) http.Handler {
		if wrap == nil {
			return h
		}
		return wrap(h)
	}
	mux.Handle("POST /api/auth/email", limitIP(l.LinkIP, s.requestLink(l.LinkEmail)))
	mux.HandleFunc("POST /api/auth/email/verify", s.verifyLink)
	mux.Handle("POST /api/auth/register", limitIP(l.PasswordIP, s.register(l.PasswordEmail)))
	mux.Handle("POST /api/auth/login", limitIP(l.PasswordIP, s.passwordLogin(l.PasswordEmail)))
	mux.HandleFunc("GET /api/auth/methods", s.methods)
	mux.HandleFunc("GET /api/auth/steam", s.steamStart)
	mux.HandleFunc("GET /api/auth/steam/callback", s.steamCallback)
	mux.HandleFunc("POST /api/auth/signout", s.signOut)
	mux.Handle("GET /api/me", s.Require(http.HandlerFunc(s.me)))
}

// UserOf is the request's signed-in user, if its session is live. Errors
// read as a visitor.
func (s *Service) UserOf(r *http.Request) (User, bool) {
	c, err := r.Cookie(CookieName)
	if err != nil {
		return User{}, false
	}
	u, err := s.sessionUser(r.Context(), c.Value, s.now())
	if err != nil {
		return User{}, false
	}
	return u, true
}
