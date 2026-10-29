package httpapi

import (
	"net/http"
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
		client := "ip:" + clientIP(r)
		if u := s.userID(r); u != nil {
			client = "user:" + strconv.FormatInt(*u, 10)
		}
		next.ServeHTTP(w, r.WithContext(s.meter.With(ctx, client, s.clientBudget)))
	})
}

// budgetUse is how much of its cap a session spent, for the live bar.
type budgetUse struct {
	Spent float64 `json:"spent"`
	Cap   float64 `json:"cap"`
}

// errOverCap is the clear message a capped request answers with.
const errOverCap = "the model spend cap is used up for now; the built-in ranking and answers still work"
