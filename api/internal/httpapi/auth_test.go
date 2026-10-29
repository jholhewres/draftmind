package httpapi

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/jholhewres/draftmind/api/internal/auth"
)

// devLink is the sign-in link a development server writes to its log.
var devLink = regexp.MustCompile(`link=http://localhost:5173/#/signin/link/([A-Za-z0-9_-]+)`)

func postJSON(h http.Handler, path, body string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestASignedInPlayersMatchIsTheirsAndAVisitorsIsNobodys(t *testing.T) {
	srv, pool := buildPatchesPool(t)
	logs := &bytes.Buffer{}
	a, err := auth.New(pool, "http://localhost:5173", slog.New(slog.NewTextHandler(logs, nil)))
	if err != nil {
		t.Fatal(err)
	}
	if err := a.WithLogMailer(); err != nil {
		t.Fatal(err)
	}
	h := srv.WithAuth(a).Handler()

	if rec := postJSON(h, "/api/auth/email", `{"email": "player@example.com"}`); rec.Code != http.StatusAccepted {
		t.Fatalf("ask for a link: %d %s", rec.Code, rec.Body)
	}
	m := devLink.FindStringSubmatch(logs.String())
	if m == nil || strings.Contains(logs.String(), "player@example.com") {
		t.Fatalf("the development log should hold the link and nothing else of the e-mail: %s", logs)
	}
	var session *http.Cookie
	for _, c := range postJSON(h, "/api/auth/email/verify", `{"token": "`+m[1]+`"}`).Result().Cookies() {
		if c.Name == auth.CookieName {
			session = c
		}
	}
	if session == nil {
		t.Fatal("the logged link signed nobody in")
	}

	start := `{"hero":1,"role":2,"allies":[],"enemies":[2],"bans":[3]}`
	if rec := postJSON(h, "/api/match", start, session); rec.Code != http.StatusOK {
		t.Fatalf("signed-in start: %d %s", rec.Code, rec.Body)
	}
	if rec := postJSON(h, "/api/match", start); rec.Code != http.StatusOK {
		t.Fatalf("visitor start: %d %s", rec.Code, rec.Body)
	}
	var mine, visitors int
	if err := pool.QueryRow(context.Background(), `
		SELECT count(*) FILTER (WHERE m.user_id = u.id), count(*) FILTER (WHERE m.user_id IS NULL)
		FROM match_session m CROSS JOIN app_user u WHERE u.email = 'player@example.com'`).Scan(&mine, &visitors); err != nil {
		t.Fatal(err)
	}
	if mine != 1 || visitors != 1 {
		t.Fatalf("matches: %d of the player, %d of visitors; want 1 and 1", mine, visitors)
	}

	prod, err := auth.New(pool, "https://draftmind.example", quietLog())
	if err != nil {
		t.Fatal(err)
	}
	if prod.WithLogMailer() == nil {
		t.Fatal("production accepted writing sign-in links to its log")
	}
}

func TestSignInLinksAreLimitedPerIPAndPerAddress(t *testing.T) {
	srv, pool := buildPatchesPool(t)
	a, err := auth.New(pool, "http://localhost:5173", quietLog())
	if err != nil {
		t.Fatal(err)
	}
	if err := a.WithLogMailer(); err != nil {
		t.Fatal(err)
	}
	h := srv.WithAuth(a).Handler()
	ask := func(ip, email string) int {
		req := httptest.NewRequest(http.MethodPost, "/api/auth/email", strings.NewReader(`{"email": "`+email+`"}`))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = ip + ":40000"
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}

	// One address, from many IPs: refused after its burst.
	for i := range DefaultMailLimit.Burst {
		if code := ask("10.0.0."+string(rune('1'+i)), "inbox@example.com"); code != http.StatusAccepted {
			t.Fatalf("link %d to the address: %d", i+1, code)
		}
	}
	if code := ask("10.0.1.1", "INBOX@example.com"); code != http.StatusTooManyRequests {
		t.Fatalf("one link too many to the address: %d, want 429", code)
	}

	// One IP, many addresses: refused after its burst.
	for i := range DefaultMailIPLimit.Burst {
		if code := ask("10.0.2.1", "p"+string(rune('a'+i))+"@example.com"); code != http.StatusAccepted {
			t.Fatalf("link %d from the IP: %d", i+1, code)
		}
	}
	if code := ask("10.0.2.1", "fresh@example.com"); code != http.StatusTooManyRequests {
		t.Fatalf("one link too many from the IP: %d, want 429", code)
	}
}

func TestPasswordSignInIsLimitedPerIPAndPerAddress(t *testing.T) {
	srv, pool := buildPatchesPool(t)
	a, err := auth.New(pool, "https://draftmind.example", quietLog())
	if err != nil {
		t.Fatal(err)
	}
	h := srv.WithAuth(a).Handler()
	login := func(ip, email string) int {
		req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"email": "`+email+`", "password": "12345678"}`))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = ip + ":40000"
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}

	// One address, from many IPs: refused after its burst.
	for i := range DefaultPasswordLimit.Burst {
		if code := login("10.0.0."+string(rune('1'+i)), "player@example.com"); code != http.StatusUnauthorized {
			t.Fatalf("attempt %d on the address: %d", i+1, code)
		}
	}
	if code := login("10.0.1.1", " PLAYER@example.com"); code != http.StatusTooManyRequests {
		t.Fatalf("one attempt too many on the address: %d, want 429", code)
	}

	// One IP, many addresses: refused after its burst.
	for i := range DefaultPasswordIPLimit.Burst {
		if code := login("10.0.2.1", "p"+string(rune('a'+i))+"@example.com"); code != http.StatusUnauthorized {
			t.Fatalf("attempt %d from the IP: %d", i+1, code)
		}
	}
	if code := login("10.0.2.1", "fresh@example.com"); code != http.StatusTooManyRequests {
		t.Fatalf("one attempt too many from the IP: %d, want 429", code)
	}
}

func TestAccountRoutesAnswer401WithoutAuth(t *testing.T) {
	rec := httptest.NewRecorder()
	New(fakeDB{}, nil, quietLog()).Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/me", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("GET /api/me without sign-in set up: %d", rec.Code)
	}
}
