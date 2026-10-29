package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"mime"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"sync"

	"github.com/jholhewres/draftmind/api/internal/draft"
	"github.com/jholhewres/draftmind/api/internal/photo"
	"github.com/jholhewres/draftmind/api/internal/writer"
)

// TipHeroes is how many of the top heroes get a written tip.
const TipHeroes = 3

// langTag is a BCP 47 language tag as the UI sends it (en, pt-BR).
var langTag = regexp.MustCompile(`^[A-Za-z]{2,3}(-[A-Za-z0-9]{2,8}){0,3}$`)

// seatedDraft is the draft a photo's read gives, as the stream ranked it.
type seatedDraft struct {
	Allies  []int `json:"allies"`
	Enemies []int `json:"enemies"`
	Bans    []int `json:"bans"`
	Role    int   `json:"role"`
}

type readEvent struct {
	Patch string `json:"patch"`
	photo.Read
	Draft seatedDraft `json:"draft"`
	// Cost of the read in USD as the providers report it, so the live read
	// can show what a session spent; omitted when none reported one.
	Cost *float64 `json:"cost,omitempty"`
}

type rankingEvent struct {
	Patch       string             `json:"patch"`
	Suggestions []draft.Suggestion `json:"suggestions"`
	modeInfo
}

// stream serves POST /api/draft/stream: the suggestions as Server-Sent
// Events, the code ranking first and the models' work after it (docs/ARCHITECTURE.md):
//
//	read      the draft read from the photo, with the draft it seats (photo only)
//	ranking   the code ranking, before any model call returns
//	decision  Jev's order, or the ranking with the reason it stands
//	tip       one per top hero: the writer's text, or a template
//	done      the stream is over
//
// The body is a JSON draft, or a multipart photo in the "image" field with
// role and team (radiant or dire) in the query. lang (default en) is the
// tips' language. A request refused before the stream starts gets a JSON
// error, as POST /api/draft/suggest does.
func (s *Server) stream(readLim *limiter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		lang := q.Get("lang")
		if lang == "" {
			lang = "en"
		}
		if len(lang) > 35 || !langTag.MatchString(lang) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "lang must be a language tag such as en or pt-BR"})
			return
		}
		var req suggestRequest
		var read *readEvent
		if mt, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type")); mt == "multipart/form-data" {
			var status int
			var err error
			read, status, err = s.streamRead(w, r, readLim)
			if err != nil {
				writeJSON(w, status, map[string]string{"error": err.Error()})
				return
			}
			d := read.Draft
			req = suggestRequest{Allies: d.Allies, Enemies: d.Enemies, Bans: d.Bans, Role: d.Role, Mode: q.Get("mode")}
			if req.draft().Mode.Pooled() {
				// The offered heroes are not on a photo of the top bar: the
				// player taps them on the board.
				req.Mode = ""
			}
			if err := req.validate(); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
		} else {
			var err error
			if req, err = decodeDraft(w, r); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
		}
		patch, ds, status, err := s.loadDraft(r.Context(), req)
		if err != nil {
			writeJSON(w, status, map[string]string{"error": err.Error()})
			return
		}

		out := newEventStream(w, s)
		if read != nil {
			read.Patch = patch
			out.send("read", read)
		}
		limit, d := req.limit(), req.draft()
		ranked := draft.Rank(ds, d, max(limit, draft.JevCandidates))
		out.send("ranking", rankingEvent{Patch: patch, Suggestions: ranked[:min(limit, len(ranked))], modeInfo: modeOf(ds, d)})

		// The ranking's top heroes are written while Jev decides; any hero
		// Jev moves into the top is written once it has.
		tips := s.newTipRun(r.Context(), out, ds, d, lang)
		tips.start(ranked[:min(TipHeroes, len(ranked))])
		decided := draft.Decide(r.Context(), s.jev, ds, d, ranked)
		s.logDecided(decided)
		decided.Suggestions = decided.Suggestions[:min(limit, len(decided.Suggestions))]
		out.send("decision", decided)
		tips.start(decided.Suggestions[:min(TipHeroes, len(decided.Suggestions))])
		tips.wait()
		out.send("done", struct{}{})
	}
}

// streamRead reads the photo upload and seats the player in its draft.
func (s *Server) streamRead(w http.ResponseWriter, r *http.Request, readLim *limiter) (*readEvent, int, error) {
	if s.reader == nil {
		return nil, http.StatusServiceUnavailable, errors.New("no photo reader is configured")
	}
	q := r.URL.Query()
	role, err := strconv.Atoi(q.Get("role"))
	if err != nil || role < 1 || role > 5 {
		return nil, http.StatusBadRequest, errors.New("role must be 1 to 5")
	}
	team := q.Get("team")
	if team == "" {
		team = "radiant"
	}
	if team != "radiant" && team != "dire" {
		return nil, http.StatusBadRequest, errors.New("team must be radiant or dire")
	}
	// A photo spends the read budget as well as the suggest one.
	if wait := readLim.take(clientIP(r)); wait > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(int(math.Ceil(wait.Seconds()))))
		return nil, http.StatusTooManyRequests, errors.New("too many requests; try again shortly")
	}
	data, status, err := photoUpload(w, r)
	if err != nil {
		return nil, status, err
	}
	patch, read, status, err := s.readPhoto(r.Context(), w, data)
	if err != nil {
		return nil, status, err
	}
	read = photo.FlagUncalibrated(read, q.Get("mode"))
	return &readEvent{Patch: patch, Read: read, Draft: seat(read, team, role), Cost: read.Cost}, http.StatusOK, nil
}

// seat places the player in a read draft as the photo screen does: their
// team's first empty slot is theirs, else its first unread one; the other
// four are allies, and the other team's heroes are the enemies. Unread
// slots count as nobody until the player fixes them.
func seat(read photo.Read, team string, role int) seatedDraft {
	own, other := read.Radiant, read.Dire
	if team == "dire" {
		own, other = other, own
	}
	you := -1
	for _, want := range []string{photo.StatusEmpty, photo.StatusUnknown} {
		for i, sl := range own {
			if you < 0 && sl.Status == want {
				you = i
			}
		}
	}
	d := seatedDraft{Allies: []int{}, Enemies: []int{}, Bans: []int{}, Role: role}
	seats := 0
	for i, sl := range own {
		if i == you || seats == maxAllies {
			continue
		}
		seats++
		if sl.Status == photo.StatusHero {
			d.Allies = append(d.Allies, sl.HeroID)
		}
	}
	for _, sl := range other {
		if sl.Status == photo.StatusHero {
			d.Enemies = append(d.Enemies, sl.HeroID)
		}
	}
	return d
}

// eventStream writes Server-Sent Events, flushing each; safe for concurrent use.
type eventStream struct {
	mu  sync.Mutex
	w   http.ResponseWriter
	rc  *http.ResponseController
	srv *Server
}

func newEventStream(w http.ResponseWriter, srv *Server) *eventStream {
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("X-Accel-Buffering", "no") // a proxy in front must not hold the events
	w.WriteHeader(http.StatusOK)
	e := &eventStream{w: w, rc: http.NewResponseController(w), srv: srv}
	_ = e.rc.Flush()
	return e
}

func (e *eventStream) send(name string, v any) {
	data, err := json.Marshal(v)
	if err != nil {
		e.srv.log.Error("draft stream: encode event", "event", name, "err", err)
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	// A client that left is not an error; its request context ends the work.
	if _, err := fmt.Fprintf(e.w, "event: %s\ndata: %s\n\n", name, data); err == nil {
		_ = e.rc.Flush()
	}
}

// tipRun writes each top hero's tip once, sending each as it is ready.
type tipRun struct {
	srv    *Server
	ctx    context.Context
	out    *eventStream
	d      draft.Draft
	lang   string
	names  func() (vocabulary, error) // loaded once, on the first tip
	heroes map[int]draft.Hero

	started map[int]bool
	wg      sync.WaitGroup
}

func (s *Server) newTipRun(ctx context.Context, out *eventStream, ds draft.Dataset, d draft.Draft, lang string) *tipRun {
	heroes := make(map[int]draft.Hero, len(ds.Heroes))
	for _, h := range ds.Heroes {
		heroes[h.ID] = h
	}
	return &tipRun{
		srv: s, ctx: ctx, out: out, d: d, lang: lang, heroes: heroes, started: map[int]bool{},
		names: sync.OnceValues(func() (vocabulary, error) {
			n, err := s.catalog.Names(ctx)
			if err != nil {
				return vocabulary{}, err
			}
			v := vocabulary{heroes: n.Heroes}
			for _, names := range n.Heroes {
				v.known = append(v.known, names...)
			}
			v.known = slices.Concat(v.known, n.Abilities, n.Items, n.OffPatch)
			return v, nil
		}),
	}
}

// vocabulary is what a tip is checked against: each hero's names by id,
// and every hero and ability name on the patch.
type vocabulary struct {
	heroes map[int][]string
	known  []string
}

// start writes the tips of the heroes in top that have none yet. Called
// from the handler's goroutine only.
func (t *tipRun) start(top []draft.Suggestion) {
	for _, sg := range top {
		if t.started[sg.HeroID] {
			continue
		}
		t.started[sg.HeroID] = true
		if t.srv.tips == nil {
			t.out.send("tip", writer.Template(sg.HeroID, writer.ReasonUnavailable))
			continue
		}
		t.wg.Add(1)
		go func() {
			defer t.wg.Done()
			t.out.send("tip", t.write(sg))
		}()
	}
}

func (t *tipRun) wait() { t.wg.Wait() }

// write asks for sg's tip from its reasons only.
func (t *tipRun) write(sg draft.Suggestion) writer.Tip {
	v, err := t.names()
	if err != nil {
		t.srv.log.Error("draft stream: entity names", "err", err)
		return writer.Template(sg.HeroID, writer.ReasonError)
	}
	b := writer.Brief{
		HeroID: sg.HeroID, Hero: t.heroes[sg.HeroID].Name, Lang: t.lang,
		Position: t.d.Role, PositionName: draft.PositionName(t.d.Role),
		Roles: t.heroes[sg.HeroID].Roles, RoleFit: sg.RoleFit, WinRate: sg.WinRate,
		Allowed: append([]string{t.heroes[sg.HeroID].Name}, v.heroes[sg.HeroID]...),
		Known:   v.known,
	}
	for _, e := range sg.Enemies {
		b.Enemies = append(b.Enemies, writer.Enemy{Name: t.heroes[e.HeroID].Name, Delta: e.Delta, Games: e.Games})
		b.Allowed = append(b.Allowed, t.heroes[e.HeroID].Name)
		b.Allowed = append(b.Allowed, v.heroes[e.HeroID]...)
	}
	return t.srv.tips.Tip(t.ctx, b)
}
