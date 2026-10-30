package match

import (
	"context"
	"errors"
	"slices"
	"sync"
	"time"

	"github.com/jholhewres/draftmind/api/internal/budget"
	"github.com/jholhewres/draftmind/api/internal/decision"
	"github.com/jholhewres/draftmind/api/internal/draft"
	"github.com/jholhewres/draftmind/api/internal/items"
	"github.com/jholhewres/draftmind/api/internal/persona"
	"github.com/jholhewres/draftmind/api/internal/writer"
)

// The live coach (ADR-0014): every tick the browser sends one frame of the
// shared game window that changed and is in a match. The code gates first
// (finished match, tick spacing), Jev answers one closed question, and the
// code applies cooldown, dedupe and a per-minute cap. A spoken event is a
// template the UI words from the event type and the item plan: no writer
// call by default. Every failure is silent.

// Live event types, the closed set Jev picks from.
const (
	EventSilent     = "silent"
	EventItemTiming = "item_timing" // the player can afford the next planned item
	EventDeaths     = "deaths"      // lost a fight or died repeatedly
	EventObjective  = "objective"   // a tower, Roshan or a rune is up for grabs
	EventPhase      = "phase"       // the game moved into a new phase
	EventMap        = "map"         // something on the minimap the player should see
	// EventBuyback comes only from Game State Integration's code triggers.
	EventBuyback = "buyback"
)

// LiveEvents are the events a tick can speak, in the order the UI lists them.
var LiveEvents = []string{EventItemTiming, EventDeaths, EventObjective, EventPhase, EventMap}

// Why a tick stayed silent.
const (
	LiveReasonSilent   = "silent"   // Jev chose silent
	LiveReasonUnsure   = "unsure"   // Jev's pick was under LiveMinConfidence
	LiveReasonError    = "error"    // the call failed or was late
	LiveReasonBudget   = "budget"   // the model spend cap is used up
	LiveReasonCooldown = "cooldown" // the same event spoke within LiveCooldown
	LiveReasonCap      = "cap"      // an event already spoke in the last minute
	LiveReasonTooSoon  = "too_soon" // the last tick was under LiveMinInterval ago
	LiveReasonEnded    = "ended"    // the match has a result
	LiveReasonNoItem   = "no_item"  // an item event with no item left in the plan
	// LiveReasonTemplate prefixes the reason of a spoken event the UI words:
	// live:<event>.
	LiveReasonTemplate = "live"
)

// SourceLive is the decision source of a live event.
const SourceLive = "live"

// Limits the code applies, whatever Jev says.
const (
	LiveMinConfidence = 0.6
	LiveCooldown      = 4 * time.Minute // per event type
	LivePerMinute     = 1               // spoken events in any minute
	LiveMinInterval   = 10 * time.Second
	LiveTimeout       = 8 * time.Second
)

// LiveFrame is one frame of the shared window, never stored. With Game
// State Integration on, Facts carries the game's own state and Data may be
// empty: Jev then reads the facts instead of the HUD.
type LiveFrame struct {
	Format string // jpg or png
	Data   []byte
	Facts  any
}

// LiveDecision is one tick's outcome. Reply is set only when Speak.
type LiveDecision struct {
	Speak      bool     `json:"speak"`
	Event      string   `json:"event,omitempty"`
	Confidence float64  `json:"confidence,omitempty"`
	Reason     string   `json:"reason,omitempty"`
	Cost       *float64 `json:"cost,omitempty"`
	Reply      Reply    `json:"-"`
}

// Live keeps each session's last tick and spoken events in memory: a
// restart forgets them, which costs at most one repeated line.
type Live struct {
	Jev Asker
	// Write, when set, words a spoken event (opt-in, LIVE_WRITER); Facts
	// gives it the names to check the text against. Nil: the UI's template.
	Write Answerer
	Facts Facts
	mu    sync.Mutex
	per   map[string]*liveState
}

type liveState struct {
	lastTick time.Time
	spoken   map[string]time.Time
	recent   []time.Time
}

// NewLive builds the live coach on jev.
func NewLive(jev Asker) *Live { return &Live{Jev: jev, per: map[string]*liveState{}} }

// eventCategory is the chat category each event is tagged with.
var eventCategory = map[string]string{
	EventItemTiming: "items", EventDeaths: "teamfight", EventObjective: "objective", EventPhase: "other", EventMap: "lane",
	EventBuyback: "teamfight",
}

// Tick decides on one frame of session s at now.
func (l *Live) Tick(ctx context.Context, s Session, f LiveFrame, now time.Time) LiveDecision {
	if s.Result != nil {
		return LiveDecision{Reason: LiveReasonEnded}
	}
	l.mu.Lock()
	st := l.per[s.ID]
	if st == nil {
		st = &liveState{spoken: map[string]time.Time{}}
		l.per[s.ID] = st
	}
	if !st.lastTick.IsZero() && now.Sub(st.lastTick) < LiveMinInterval {
		l.mu.Unlock()
		return LiveDecision{Reason: LiveReasonTooSoon}
	}
	st.lastTick = now
	l.mu.Unlock()

	if l.Jev == nil {
		return LiveDecision{Reason: LiveReasonError}
	}
	ctx, cancel := context.WithTimeout(ctx, LiveTimeout)
	defer cancel()
	res, err := l.Jev.Ask(ctx, liveRequest(s, f))
	d := LiveDecision{Cost: res.Usage.Cost}
	if errors.Is(err, budget.ErrOverCap) {
		d.Reason = LiveReasonBudget
		return d
	}
	if err != nil {
		d.Reason = LiveReasonError
		return d
	}
	a := res.Answers["event"]
	if a.Choice == EventSilent || !slices.Contains(LiveEvents, a.Choice) {
		d.Reason = LiveReasonSilent
		return d
	}
	conf := a.Probabilities[a.Choice]
	if conf == 0 {
		conf = a.Confidence
	}
	if conf < LiveMinConfidence {
		d.Reason = LiveReasonUnsure
		return d
	}
	reply, ok := liveReply(s, a.Choice)
	if !ok {
		d.Reason = LiveReasonNoItem
		return d
	}
	return l.admit(s, st, d, a.Choice, conf, reply, now)
}

// Trigger speaks a code trigger (from Game State Integration) with no model
// call, through the same cooldown and cap as Jev's events; item, when set,
// is the card of the item the trigger is about.
func (l *Live) Trigger(s Session, event string, item *items.Card, now time.Time) LiveDecision {
	if s.Result != nil {
		return LiveDecision{Reason: LiveReasonEnded}
	}
	reply, ok := liveReply(s, event)
	if !ok && item == nil {
		return LiveDecision{Reason: LiveReasonNoItem}
	}
	if item != nil {
		reply.Items = []items.Card{*item}
	}
	l.mu.Lock()
	st := l.per[s.ID]
	if st == nil {
		st = &liveState{spoken: map[string]time.Time{}}
		l.per[s.ID] = st
	}
	l.mu.Unlock()
	return l.admit(s, st, LiveDecision{}, event, 1, reply, now)
}

// admit applies cooldown, dedupe and the per-minute cap to a spoken event.
func (l *Live) admit(s Session, st *liveState, d LiveDecision, event string, conf float64, reply Reply, now time.Time) LiveDecision {
	l.mu.Lock()
	defer l.mu.Unlock()
	// Turbo's clock runs about twice as fast, so its cooldown is shorter (ADR-0012).
	cooldown := time.Duration(float64(LiveCooldown) * draft.TimeScale(draft.Mode(s.Mode)))
	if at, ok := st.spoken[event]; ok && now.Sub(at) < cooldown {
		d.Reason = LiveReasonCooldown
		return d
	}
	st.recent = slices.DeleteFunc(st.recent, func(t time.Time) bool { return now.Sub(t) >= time.Minute })
	if len(st.recent) >= LivePerMinute {
		d.Reason = LiveReasonCap
		return d
	}
	st.spoken[event] = now
	st.recent = append(st.recent, now)
	d.Speak, d.Event, d.Confidence, d.Reply = true, event, conf, reply
	return d
}

// Forget drops a session's live state, once it ends.
func (l *Live) Forget(id string) {
	l.mu.Lock()
	delete(l.per, id)
	l.mu.Unlock()
}

// liveRequest is Jev's one closed question about the frame, with the
// match as the code knows it. No free text comes back.
func liveRequest(s Session, f LiveFrame) decision.Request {
	options := map[string]any{
		EventSilent:     "nothing worth interrupting the player for right now, or the frame is not the player's own match",
		EventItemTiming: "the gold shown is enough for the next item of the plan and the player has not bought it",
		EventDeaths:     "the player just died or lost a fight, or has died several times in a short time",
		EventObjective:  "an objective is up for grabs now: a tower under push, Roshan, a power rune",
		EventPhase:      "the game just moved into a new phase: laning over, teams grouping, late game",
		EventMap:        "the minimap shows something the player should react to now",
	}
	var next []string
	for _, c := range planCards(s) {
		next = append(next, c.Name)
	}
	state := map[string]any{"hero": s.HeroID, "position": s.Role, "mode": s.Mode, "next_items": next}
	req := decision.Request{Purpose: "live.tick", State: state}
	if f.Facts != nil {
		// The game's own state: exact gold, items and clock, no image.
		state["game"] = f.Facts
		req.Questions = map[string]decision.Question{"event": decision.Choice("`game` is the player's own state as the game "+
			"reports it during a match. Pick the one event the player most needs to hear about now; silent unless one clearly applies.", options)}
		return req
	}
	state["frame"] = "frame"
	req.Assets = map[string]decision.Asset{"frame": {Format: f.Format, Data: f.Data}}
	req.Questions = map[string]decision.Question{"event": decision.Choice("`frame` is the player's own game screen during a match. "+
		"Pick the one event the player most needs to hear about now, judged only from what the screen shows; silent unless one clearly applies.", options)}
	return req
}

// liveReply is the spoken event as a template: the UI words it from the
// event type; an item event carries the next item of the plan as its card.
func liveReply(s Session, event string) (Reply, bool) {
	r := Reply{Kind: "answer", Category: eventCategory[event], Priority: "high", Source: SourceLive,
		TextSource: "template", Reason: LiveReasonTemplate + ":" + event}
	if event == EventItemTiming {
		cards := planCards(s)
		if len(cards) == 0 {
			return r, false
		}
		r.Items = cards[:1]
	}
	return r, true
}

// planCards are the plan's items in the order the player should buy them:
// urgent situational items, then the mid and late core.
func planCards(s Session) []items.Card { return PlanCards(s) }

// PlanCards are the session's plan items in buying order.
func PlanCards(s Session) []items.Card {
	r := QuickItems(s, func(string, any) {})
	return r.Items
}

// Word asks the writer for the spoken event's line when Write is set, in
// the persona's language, from the event and its item only; a text naming
// anything else, or any failure, keeps the template.
func (l *Live) Word(ctx context.Context, d *LiveDecision, lang string, p persona.Persona) {
	if l.Write == nil || l.Facts == nil || !d.Speak {
		return
	}
	names, err := l.Facts.Names(ctx)
	if err != nil {
		return
	}
	known := slices.Concat(names.Items, names.Abilities, names.OffPatch)
	for _, n := range names.Heroes {
		known = append(known, n...)
	}
	facts := map[string]any{"event": d.Event}
	var allowed []string
	for _, c := range d.Reply.Items {
		facts["item"] = map[string]any{"name": c.Name, "cost": c.Cost}
		allowed = append(allowed, c.Name)
	}
	a := l.Write.Answer(ctx, writer.AnswerBrief{Lang: p.LangFor(lang), Persona: p, Message: "(live event: " + d.Event + ")",
		Category: d.Reply.Category, Priority: d.Reply.Priority, Facts: facts, Allowed: allowed, Known: known})
	if a.Source == writer.SourceWriter {
		d.Reply.Text, d.Reply.TextSource, d.Reply.Reason = a.Text, a.Source, ""
	}
}
