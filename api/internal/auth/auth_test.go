package auth

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jholhewres/draftmind/api/internal/dbtest"
)

const publicURL = "https://draftmind.test"

type fakeMailer struct {
	mu   sync.Mutex
	sent []Message
	err  error
}

func (f *fakeMailer) Send(_ context.Context, m Message) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, m)
	return f.err
}

func (f *fakeMailer) last(t *testing.T) Message {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.sent) == 0 {
		t.Fatal("no e-mail sent")
	}
	return f.sent[len(f.sent)-1]
}

// harness is the service on a fresh database, its clock stopped at now.
type harness struct {
	t    *testing.T
	svc  *Service
	h    http.Handler
	mail *fakeMailer
	logs *bytes.Buffer
	now  time.Time
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	logs := &bytes.Buffer{}
	svc, err := New(dbtest.New(t), publicURL, slog.New(slog.NewTextHandler(logs, nil)))
	if err != nil {
		t.Fatal(err)
	}
	h := &harness{t: t, svc: svc, mail: &fakeMailer{}, logs: logs, now: time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)}
	svc.WithMailer(h.mail)
	svc.now = func() time.Time { return h.now }
	mux := http.NewServeMux()
	svc.Mount(mux, nil, nil)
	h.h = mux
	return h
}

func (h *harness) advance(d time.Duration) { h.now = h.now.Add(d) }

func (h *harness) do(method, path, body string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	var r io.Reader
	if body != "" {
		r = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, r)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	h.h.ServeHTTP(rec, req)
	return rec
}

var linkToken = regexp.MustCompile(regexp.QuoteMeta(publicURL) + `/#/signin/link/([A-Za-z0-9_-]+)`)

// requestLink asks for a link to email and returns the token the e-mail carries.
func (h *harness) requestLink(email string) string {
	h.t.Helper()
	if rec := h.do(http.MethodPost, "/api/auth/email", `{"email": "`+email+`", "lang": "en"}`); rec.Code != http.StatusAccepted {
		h.t.Fatalf("request link: status %d: %s", rec.Code, rec.Body)
	}
	m := linkToken.FindStringSubmatch(h.mail.last(h.t).Body)
	if m == nil {
		h.t.Fatalf("no sign-in link in the e-mail: %q", h.mail.last(h.t).Body)
	}
	return m[1]
}

func (h *harness) openLink(token string) *httptest.ResponseRecorder {
	return h.do(http.MethodPost, "/api/auth/email/verify", `{"token": "`+token+`"}`)
}

// sessionCookie is the session cookie rec sets, or nil.
func sessionCookie(rec *httptest.ResponseRecorder) *http.Cookie {
	for _, c := range rec.Result().Cookies() {
		if c.Name == CookieName && c.Value != "" {
			return c
		}
	}
	return nil
}

func TestMagicLinkWorksOnceAndExpiresAfter15Minutes(t *testing.T) {
	h := newHarness(t)
	token := h.requestLink("Player@Example.com")
	if to := h.mail.last(t).To; to != "player@example.com" {
		t.Fatalf("link sent to %q", to)
	}
	late := h.requestLink("late@example.com")

	h.advance(15*time.Minute - time.Second)
	first := h.openLink(token)
	if first.Code != http.StatusOK {
		t.Fatalf("first open at 14m59s: status %d: %s", first.Code, first.Body)
	}
	cookie := sessionCookie(first)
	if cookie == nil {
		t.Fatal("first open set no session cookie")
	}
	if me := h.do(http.MethodGet, "/api/me", "", cookie); me.Code != http.StatusOK || !strings.Contains(me.Body.String(), `"email":"player@example.com"`) {
		t.Fatalf("me: status %d: %s", me.Code, me.Body)
	}
	if again := h.openLink(token); again.Code != http.StatusUnauthorized || sessionCookie(again) != nil {
		t.Fatalf("second open: status %d, cookie %v", again.Code, sessionCookie(again))
	}

	h.advance(time.Second)
	if rec := h.openLink(late); rec.Code != http.StatusUnauthorized || sessionCookie(rec) != nil {
		t.Fatalf("opened at 15m: status %d: %s", rec.Code, rec.Body)
	}
}

func TestRequestLinkRefusesWhatItCannotSend(t *testing.T) {
	h := newHarness(t)
	for _, tc := range []struct {
		name, contentType, body string
		want                    int
	}{
		{"not an address", "application/json", `{"email": "player"}`, http.StatusBadRequest},
		{"an address with a name", "application/json", `{"email": "Player <player@example.com>"}`, http.StatusBadRequest},
		{"a form post", "application/x-www-form-urlencoded", `email=player@example.com`, http.StatusUnsupportedMediaType},
	} {
		req := httptest.NewRequest(http.MethodPost, "/api/auth/email", strings.NewReader(tc.body))
		req.Header.Set("Content-Type", tc.contentType)
		rec := httptest.NewRecorder()
		h.h.ServeHTTP(rec, req)
		if rec.Code != tc.want {
			t.Errorf("%s: status %d, want %d", tc.name, rec.Code, tc.want)
		}
	}
	if len(h.mail.sent) != 0 {
		t.Fatalf("sent %d e-mails", len(h.mail.sent))
	}

	h.mail.err = errors.New("smtp: 451 try later")
	if rec := h.do(http.MethodPost, "/api/auth/email", `{"email": "player@example.com"}`); rec.Code != http.StatusBadGateway {
		t.Fatalf("mailer down: status %d", rec.Code)
	}
	if m := linkToken.FindStringSubmatch(h.mail.last(t).Body); m == nil || strings.Contains(h.logs.String(), m[1]) {
		t.Fatalf("the log holds the link: %s", h.logs)
	}

	h.svc.WithMailer(nil)
	if rec := h.do(http.MethodPost, "/api/auth/email", `{"email": "player@example.com"}`); rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("no mailer: status %d", rec.Code)
	}
}
