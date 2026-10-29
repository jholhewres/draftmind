package httpapi

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/jholhewres/draftmind/api/internal/auth"
	"github.com/jholhewres/draftmind/api/internal/gsi"
	"github.com/jholhewres/draftmind/api/internal/items"
	"github.com/jholhewres/draftmind/api/internal/match"
)

// GSIStore keeps the accounts' Game State Integration tokens and states.
type GSIStore interface {
	Issue(ctx context.Context, userID int64) (string, error)
	Revoke(ctx context.Context, userID int64) error
	Authenticate(ctx context.Context, token string, now time.Time) (int64, error)
	Status(ctx context.Context, userID int64) (gsi.Status, error)
	Last(ctx context.Context, userID int64) (gsi.State, time.Time, bool, error)
	Save(ctx context.Context, userID int64, st gsi.State, now time.Time) error
}

// OpenMatches finds a player's match in progress.
type OpenMatches interface {
	OpenFor(ctx context.Context, userID int64) (string, error)
}

// maxGSIBody caps a post: the game's full state is a few kilobytes.
const maxGSIBody = 64 << 10

// GSIFresh is how old the last state may be to count as the match's state.
const GSIFresh = 60 * time.Second

// DefaultGSILimit allows the game's posts (about one a second while
// playing, throttled in the cfg) with room to spare, per client IP.
var DefaultGSILimit = Limit{Burst: 20, Every: 500 * time.Millisecond}

// WithGSI turns Game State Integration on (ADR-0015).
func (s *Server) WithGSI(st GSIStore, open OpenMatches) *Server {
	s.gsi, s.openMatches = st, open
	return s
}

// triggers is the live coach the code triggers speak through: the
// configured one, else a code-only one, since triggers make no model call.
func (s *Server) triggers() *match.Live {
	if s.live != nil {
		return s.live
	}
	s.gsiOnce.Do(func() { s.gsiLive = match.NewLive(nil) })
	return s.gsiLive
}

// gsiPost serves POST /api/gsi: one state the game posts, with the
// account's token in auth.token. Code triggers run on it against the
// player's open match, with no model call; a finished match records its
// result. The game ignores the answer, so every refusal is just a status.
func (s *Server) gsiPost(w http.ResponseWriter, r *http.Request) {
	if s.gsi == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxGSIBody))
	if err != nil {
		w.WriteHeader(http.StatusRequestEntityTooLarge)
		return
	}
	st, token, err := gsi.Parse(body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	now := s.now()
	user, err := s.gsi.Authenticate(r.Context(), token, now)
	if errors.Is(err, gsi.ErrBadToken) {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	if err != nil {
		s.log.Error("gsi auth", "err", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	prev, _, hadPrev, err := s.gsi.Last(r.Context(), user)
	if err != nil {
		s.log.Error("gsi last", "err", err)
	}
	if hadPrev && prev.MatchID == st.MatchID {
		// The last death's clock is ours to keep: the game sends only the count.
		st.LastDeathClock = prev.LastDeathClock
		if st.Deaths > prev.Deaths {
			st.LastDeathClock = st.Clock
		}
	} else {
		hadPrev = false
	}
	if err := s.gsi.Save(r.Context(), user, st, now); err != nil {
		s.log.Error("gsi save", "err", err)
	}
	s.gsiAct(r.Context(), user, prevOf(hadPrev, prev), st, now)
	w.WriteHeader(http.StatusOK)
}

func prevOf(ok bool, p gsi.State) *gsi.State {
	if !ok {
		return nil
	}
	return &p
}

// gsiAct runs the code triggers on the player's open match and records
// its result once the game reports the winner.
func (s *Server) gsiAct(ctx context.Context, user int64, prev *gsi.State, cur gsi.State, now time.Time) {
	if s.openMatches == nil || s.matches == nil {
		return
	}
	id, err := s.openMatches.OpenFor(ctx, user)
	if err != nil || id == "" {
		return
	}
	sess, err := s.matches.Get(ctx, id)
	if err != nil || sess.Result != nil {
		return
	}
	// The game's own mode wins over the one picked at the draft (ADR-0012).
	if cur.Mode != "" {
		sess.Mode = cur.Mode
	} else {
		cur.Mode = sess.Mode
	}
	if out, ok := cur.Outcome(); ok {
		r := match.Result{Outcome: out, Kills: &cur.Kills, Deaths: &cur.Deaths, Assists: &cur.Assists, Source: "gsi"}
		if cur.DurationS > 0 && cur.DurationS <= match.MaxDuration {
			r.DurationS = &cur.DurationS
		}
		if _, err := s.matches.SaveResult(ctx, id, &user, r); err != nil {
			s.log.Error("gsi result", "err", err)
		} else {
			s.triggers().Forget(id)
		}
		return
	}
	plan, cards := gsiPlan(sess)
	for _, tr := range gsi.Triggers(prev, cur, plan) {
		var card *items.Card
		if tr.Item != nil {
			card = cards[tr.Item.Key]
		}
		d := s.triggers().Trigger(sess, tr.Event, card, now)
		if !d.Speak {
			continue
		}
		rp := d.Reply
		if _, err := s.matches.Add(ctx, id, match.Message{Author: "coach", Kind: "answer", Text: rp.Text,
			Category: rp.Category, Priority: rp.Priority, Source: rp.Source, Reason: rp.Reason, Items: rp.Items}); err != nil {
			s.log.Error("gsi live event", "err", err)
		}
		s.log.Info("gsi trigger", "session", id, "event", tr.Event)
	}
}

// gsiPlan is the session's plan items in buying order, with their cards.
func gsiPlan(sess match.Session) (gsi.Plan, map[string]*items.Card) {
	var plan gsi.Plan
	cards := map[string]*items.Card{}
	for _, c := range match.PlanCards(sess) {
		plan.Next = append(plan.Next, gsi.PlanItem{Key: c.Key, Name: c.Name, Cost: c.Cost})
		cards[c.Key] = &c
	}
	return plan, cards
}

// gsiFacts is the account's fresh game state for a live tick, or nil.
func (s *Server) gsiFacts(ctx context.Context, user *int64) any {
	if s.gsi == nil || user == nil {
		return nil
	}
	st, at, ok, err := s.gsi.Last(ctx, *user)
	if err != nil || !ok || s.now().Sub(at) > GSIFresh || st.Phase != gsi.PhaseGame {
		return nil
	}
	return st
}

type gsiStatus struct {
	gsi.Status
	// Receiving is true when the game posted in the last GSIFresh.
	Receiving bool `json:"receiving"`
}

// myGSI serves GET /api/me/gsi: whether the account has a token and
// whether the game is posting.
func (s *Server) myGSI(w http.ResponseWriter, r *http.Request) {
	u, ok := auth.UserFrom(r.Context())
	if !ok || s.gsi == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "game integration is not configured"})
		return
	}
	st, err := s.gsi.Status(r.Context(), u.ID)
	if err != nil {
		s.log.Error("gsi status", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not load the integration"})
		return
	}
	writeJSON(w, http.StatusOK, gsiStatus{Status: st, Receiving: st.LastPostAt != nil && s.now().Sub(*st.LastPostAt) <= GSIFresh})
}

// issueGSI serves POST /api/me/gsi/token: a new token, replacing any old
// one, and the config file the game reads, shown once.
func (s *Server) issueGSI(w http.ResponseWriter, r *http.Request) {
	u, ok := auth.UserFrom(r.Context())
	if !ok || s.gsi == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "game integration is not configured"})
		return
	}
	token, err := s.gsi.Issue(r.Context(), u.ID)
	if err != nil {
		s.log.Error("gsi issue", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not create the token"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"filename": GSIConfigName, "config": GSIConfig(s.publicURL(r), token)})
}

// revokeGSI serves DELETE /api/me/gsi/token.
func (s *Server) revokeGSI(w http.ResponseWriter, r *http.Request) {
	u, ok := auth.UserFrom(r.Context())
	if !ok || s.gsi == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "game integration is not configured"})
		return
	}
	if err := s.gsi.Revoke(r.Context(), u.ID); err != nil {
		s.log.Error("gsi revoke", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not revoke the token"})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GSIConfigName is the file the game reads from cfg/gamestate_integration/.
const GSIConfigName = "gamestate_integration_draftmind.cfg"

// GSIConfig is the game's config file for the account's token: the
// player's own state, throttled, posted to base's /api/gsi.
func GSIConfig(base, token string) string {
	q := strconv.Quote
	var b strings.Builder
	b.WriteString("\"Draftmind\"\n{\n")
	b.WriteString("\t\"uri\" " + q(strings.TrimRight(base, "/")+"/api/gsi") + "\n")
	b.WriteString("\t\"timeout\" \"5.0\"\n\t\"buffer\" \"0.5\"\n\t\"throttle\" \"1.0\"\n\t\"heartbeat\" \"30.0\"\n")
	b.WriteString("\t\"data\"\n\t{\n")
	for _, k := range []string{"provider", "map", "player", "hero", "items"} {
		b.WriteString("\t\t" + q(k) + " \"1\"\n")
	}
	b.WriteString("\t}\n\t\"auth\"\n\t{\n\t\t\"token\" " + q(token) + "\n\t}\n}\n")
	return b.String()
}

// publicURL is the server's address as the player reaches it.
func (s *Server) publicURL(r *http.Request) string {
	if s.public != "" {
		return s.public
	}
	u := url.URL{Scheme: "https", Host: r.Host}
	if r.TLS == nil && strings.HasPrefix(r.Host, "localhost") {
		u.Scheme = "http"
	}
	return u.String()
}

// WithPublicURL sets the address written into the game's config file.
func (s *Server) WithPublicURL(u string) *Server {
	s.public = u
	return s
}
