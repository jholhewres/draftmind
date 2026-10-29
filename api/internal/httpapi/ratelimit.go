package httpapi

import (
	"math"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// Limit is a token bucket: Burst requests at once, then one per Every.
type Limit struct {
	Burst int
	Every time.Duration
}

// Default limits for the endpoints that spend money and need no login:
// suggest runs on every draft edit, read on every photo.
var (
	DefaultSuggestLimit = Limit{Burst: 30, Every: 2 * time.Second}
	DefaultReadLimit    = Limit{Burst: 6, Every: 20 * time.Second}
	// Voice clips: each one is a transcription and a coach turn.
	DefaultVoiceLimit = Limit{Burst: 10, Every: 15 * time.Second}
	// Sign-in links: per client IP, and per e-mail address so one inbox is
	// not flooded from many addresses.
	DefaultMailIPLimit = Limit{Burst: 5, Every: time.Minute}
	DefaultMailLimit   = Limit{Burst: 3, Every: 5 * time.Minute}
)

// limiter keeps one token bucket per key (a client IP, an e-mail address), in memory.
type limiter struct {
	limit     Limit
	now       func() time.Time
	mu        sync.Mutex
	buckets   map[string]*bucket
	lastSweep time.Time
}

type bucket struct {
	tokens float64
	at     time.Time
}

func newLimiter(l Limit, now func() time.Time) *limiter {
	return &limiter{limit: l, now: now, buckets: map[string]*bucket{}, lastSweep: now()}
}

// wrap refuses a client's request with 429 and Retry-After once its bucket
// is empty.
func (l *limiter) wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if wait := l.take(clientIP(r)); wait > 0 {
			w.Header().Set("Retry-After", strconv.Itoa(int(math.Ceil(wait.Seconds()))))
			writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "too many requests; try again shortly"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

// take spends one of key's tokens, or says how long until one is back.
func (l *limiter) take(key string) time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	l.sweep(now)
	b, ok := l.buckets[key]
	if !ok {
		b = &bucket{tokens: float64(l.limit.Burst), at: now}
		l.buckets[key] = b
	}
	b.tokens = min(float64(l.limit.Burst), b.tokens+now.Sub(b.at).Seconds()/l.limit.Every.Seconds())
	b.at = now
	if b.tokens >= 1 {
		b.tokens--
		return 0
	}
	return time.Duration((1 - b.tokens) * float64(l.limit.Every))
}

// sweep drops, at most once a minute, the buckets that have refilled: a
// client seen again starts full anyway.
func (l *limiter) sweep(now time.Time) {
	if now.Sub(l.lastSweep) < time.Minute {
		return
	}
	l.lastSweep = now
	full := time.Duration(l.limit.Burst) * l.limit.Every
	for ip, b := range l.buckets {
		if now.Sub(b.at) >= full {
			delete(l.buckets, ip)
		}
	}
}

func (l *limiter) clients() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.buckets)
}

// clientIP is the connection's address; behind a trusted proxy realIP has
// already set it to the forwarded client.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// WithLimits sets the per-client rate limits of suggest and read.
func (s *Server) WithLimits(suggest, read Limit) *Server {
	s.suggestLimit, s.readLimit = suggest, read
	return s
}
