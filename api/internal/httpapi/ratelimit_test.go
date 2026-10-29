package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// fakeClock is a time the test moves by hand.
type fakeClock struct{ t time.Time }

func (c *fakeClock) now() time.Time { return c.t }

func hit(h http.Handler, ip string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/x", nil)
	req.RemoteAddr = ip + ":51234"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
}

func TestRateLimitAllowsABurstThenRefusesWithRetryAfter(t *testing.T) {
	clock := &fakeClock{time.Unix(1_800_000_000, 0)}
	h := newLimiter(Limit{Burst: 3, Every: 10 * time.Second}, clock.now).wrap(okHandler())
	for i := 0; i < 3; i++ {
		if rec := hit(h, "203.0.113.7"); rec.Code != http.StatusOK {
			t.Fatalf("request %d: status %d", i+1, rec.Code)
		}
	}
	rec := hit(h, "203.0.113.7")
	if rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") != "10" {
		t.Fatalf("4th request: status %d, Retry-After %q", rec.Code, rec.Header().Get("Retry-After"))
	}
	if !strings.Contains(rec.Body.String(), "error") {
		t.Fatalf("body %q", rec.Body)
	}
}

func TestRateLimitRefillsOneRequestPerInterval(t *testing.T) {
	clock := &fakeClock{time.Unix(1_800_000_000, 0)}
	h := newLimiter(Limit{Burst: 1, Every: 10 * time.Second}, clock.now).wrap(okHandler())
	hit(h, "203.0.113.7")
	clock.t = clock.t.Add(9 * time.Second)
	if rec := hit(h, "203.0.113.7"); rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") != "1" {
		t.Fatalf("after 9 s: status %d, Retry-After %q", rec.Code, rec.Header().Get("Retry-After"))
	}
	clock.t = clock.t.Add(time.Second)
	if rec := hit(h, "203.0.113.7"); rec.Code != http.StatusOK {
		t.Fatalf("after 10 s: status %d", rec.Code)
	}
}

func TestRateLimitKeepsEachClientApart(t *testing.T) {
	clock := &fakeClock{time.Unix(1_800_000_000, 0)}
	h := newLimiter(Limit{Burst: 1, Every: time.Minute}, clock.now).wrap(okHandler())
	hit(h, "203.0.113.7")
	if rec := hit(h, "198.51.100.2"); rec.Code != http.StatusOK {
		t.Fatalf("another client was limited: %d", rec.Code)
	}
	if rec := hit(h, "203.0.113.7"); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("the first client was not limited: %d", rec.Code)
	}
}

func TestRateLimitForgetsIdleClients(t *testing.T) {
	clock := &fakeClock{time.Unix(1_800_000_000, 0)}
	l := newLimiter(Limit{Burst: 2, Every: time.Second}, clock.now)
	h := l.wrap(okHandler())
	for i := 0; i < 50; i++ {
		hit(h, "203.0.113."+string(rune('0'+i%10))+string(rune('0'+i/10)))
	}
	clock.t = clock.t.Add(time.Hour)
	hit(h, "198.51.100.2")
	if n := l.clients(); n != 1 {
		t.Fatalf("%d clients kept after an hour idle", n)
	}
}

func TestSuggestAndReadAreRateLimitedPerClient(t *testing.T) {
	srv := New(fakeDB{}, nil, quietLog()).WithLimits(Limit{Burst: 1, Every: time.Minute}, Limit{Burst: 1, Every: time.Minute})
	h := srv.Handler()
	for _, path := range []string{"/api/draft/suggest", "/api/draft/read"} {
		first := httptest.NewRequest(http.MethodPost, path, strings.NewReader("{}"))
		first.RemoteAddr = "203.0.113.7:1000"
		h.ServeHTTP(httptest.NewRecorder(), first)
		again := httptest.NewRequest(http.MethodPost, path, strings.NewReader("{}"))
		again.RemoteAddr = "203.0.113.7:1001"
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, again)
		if rec.Code != http.StatusTooManyRequests {
			t.Errorf("%s: second request status %d", path, rec.Code)
		}
	}
	health := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	health.RemoteAddr = "203.0.113.7:1002"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, health)
	if rec.Code == http.StatusTooManyRequests {
		t.Fatal("health is rate limited")
	}
}
