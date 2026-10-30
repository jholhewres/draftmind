package httpapi

import (
	"net/http"
	"net/netip"
	"strconv"

	"github.com/jholhewres/draftmind/api/internal/budget"
)

// WithBudget caps model spend per match session and per client (a signed-in
// account, else the client's IP); 0 leaves that cap off.
func (s *Server) WithBudget(sessionUSD, clientUSD float64) *Server {
	s.meter, s.sessionBudget, s.clientBudget = budget.New(), sessionUSD, clientUSD
	return s
}

// metered charges the request's model calls to its match session, when the
// route names one, and to its client.
func (s *Server) metered(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.meter == nil {
			next.ServeHTTP(w, r)
			return
		}
		ctx := r.Context()
		if id := r.PathValue("id"); id != "" {
			ctx = s.meter.With(ctx, "match:"+id, s.sessionBudget)
		}
		// A signed-in client spends from its account and its address both, so
		// new accounts on one address do not add caps.
		ctx = s.meter.With(ctx, "ip:"+spenderOf(clientIP(r)), s.clientBudget)
		if u := s.userID(r); u != nil {
			ctx = s.meter.With(ctx, "user:"+strconv.FormatInt(*u, 10), s.clientBudget)
		}
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// spenderOf is an address as one spender: an IPv6 client's whole /64, since
// one host usually holds all of it.
func spenderOf(ip string) string {
	a, err := netip.ParseAddr(ip)
	if err != nil || !a.Unmap().Is6() {
		return ip
	}
	p, _ := a.Prefix(64)
	return p.String()
}

// budgetUse is how much of its cap a session spent, for the live bar.
type budgetUse struct {
	Spent float64 `json:"spent"`
	Cap   float64 `json:"cap"`
}

// errOverCap is the clear message a capped request answers with.
const errOverCap = "the model spend cap is used up for now; the built-in ranking and answers still work"

// writeOverCap answers 429 marked as the spend cap, so the client tells it
// from a rate limit.
func writeOverCap(w http.ResponseWriter) {
	w.Header().Set("X-Spend-Cap", "used")
	writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": errOverCap})
}
