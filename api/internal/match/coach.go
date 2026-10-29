package match

import (
	"context"
	"errors"
	"math"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/jholhewres/draftmind/api/internal/catalog"
	"github.com/jholhewres/draftmind/api/internal/decision"
	"github.com/jholhewres/draftmind/api/internal/draft"
	"github.com/jholhewres/draftmind/api/internal/items"
	"github.com/jholhewres/draftmind/api/internal/persona"
	"github.com/jholhewres/draftmind/api/internal/writer"
)

// Situation is a message as the coach read it: the parsed category and
// summary, and the mentioned names the database resolved.
type Situation struct {
	Category  string   `json:"category"`
	Summary   string   `json:"summary"`
	Heroes    []int    `json:"heroes"`
	Abilities []string `json:"abilities"`
	Items     []string `json:"items"`
	// Unknown are mentioned names the current patch lacks; an answer naming
	// one, in any case, fails the check.
	Unknown []string `json:"unknown"`
}

// Priorities are Jev's levels for how urgent the advice is.
var Priorities = []string{"low", "medium", "high"}

// Where the category and priority came from.
const (
	SourceJev    = "jev"
	SourceParser = "parser" // Jev failed, was late or unsure: the parse's category at medium priority
)

// MinConfidence is the least confidence Jev's category needs to stand. The
// priority is a score over ordered levels, whose confidence runs lower by
// nature; its most likely level stands.
const MinConfidence = 0.4

// Parser reads a message into a situation (the writer model).
type Parser interface {
	Situation(ctx context.Context, text string) (writer.Situation, error)
}

// Answerer writes the answer from the facts (the writer model).
type Answerer interface {
	Answer(ctx context.Context, b writer.AnswerBrief) writer.Answer
}

// Asker asks the decision model.
type Asker interface {
	Ask(ctx context.Context, req decision.Request) (decision.Result, error)
}

// Facts reads the synced game data.
type Facts interface {
	Heroes(ctx context.Context) (patch string, heroes []catalog.Hero, err error)
	HeroDetail(ctx context.Context, slug, lang string) (catalog.HeroDetail, error)
	DraftDataset(ctx context.Context, enemies []int) (patch string, ds draft.Dataset, err error)
	Names(ctx context.Context) (catalog.Names, error)
	// HeroBuilds are the hero's stored builds on the current patch (ADR-0011).
	HeroBuilds(ctx context.Context, hero int) ([]catalog.Build, error)
}

// Coach answers a player's messages during a match.
type Coach struct {
	Parse Parser
	Jev   Asker // nil: the parse's category stands
	Write Answerer
	Facts Facts
	Note  Noter // nil: no end-of-match notes
}

// Reply is the coach's turn: an answer, or a follow-up question when the
// message could not be read.
type Reply struct {
	Kind      string     `json:"kind"` // answer or followup
	Situation *Situation `json:"situation,omitempty"`
	Category  string     `json:"category,omitempty"`
	Priority  string     `json:"priority,omitempty"`
	Source    string     `json:"source,omitempty"`
	Text      string     `json:"text,omitempty"`
	// TextSource is writer, or template when the UI words the answer.
	TextSource string `json:"textSource,omitempty"`
	Reason     string `json:"reason,omitempty"`
	// Items are the current-patch items the writer's text names, in order.
	Items []items.Card `json:"items,omitempty"`
}

// MaxMessage is the longest player message, in characters.
const MaxMessage = 500

// Emit sends one stream event.
type Emit func(name string, v any)

// Reply runs the pipeline on text for session s, emitting situation,
// decision and then answer, or a single followup when the message cannot
// be parsed. lang is the player's UI language, which the decision's facts
// are read in; p, the player's persona, shapes only the writer: its
// wording and the language it answers in.
func (c *Coach) Reply(ctx context.Context, s Session, text, lang string, p persona.Persona, emit Emit) (Reply, error) {
	parsed, err := c.Parse.Situation(ctx, text)
	if errors.Is(err, writer.ErrUnclear) {
		// A guess on an unread message is worse than a question.
		r := Reply{Kind: "followup", Reason: "unclear"}
		emit("followup", r)
		return r, nil
	}
	if err != nil {
		// The model failed or was late: the message was not unclear.
		r := Reply{Kind: "answer", TextSource: writer.SourceTemplate, Reason: writer.ReasonError}
		if errors.Is(err, context.DeadlineExceeded) {
			r.Reason = writer.ReasonTimeout
		}
		emit("answer", r)
		return r, nil
	}
	_, heroes, err := c.Facts.Heroes(ctx)
	if err != nil {
		return Reply{}, err
	}
	names, err := c.Facts.Names(ctx)
	if err != nil {
		return Reply{}, err
	}
	sit := resolve(parsed, heroes, names)
	emit("situation", sit)

	f, err := c.facts(ctx, s, sit, heroes, names, lang)
	if err != nil {
		return Reply{}, err
	}
	r := Reply{Kind: "answer", Situation: &sit}
	r.Category, r.Priority, r.Source = c.decide(ctx, sit, f.data)
	emit("decision", map[string]string{"category": r.Category, "priority": r.Priority, "source": r.Source})

	// The writer reads the facts in the persona's language; Jev never does.
	wf := f
	if wlang := p.LangFor(lang); wlang != lang {
		if wf, err = c.facts(ctx, s, sit, heroes, names, wlang); err != nil {
			return Reply{}, err
		}
	}
	a := c.Write.Answer(ctx, writer.AnswerBrief{
		Lang: p.LangFor(lang), Persona: p, Message: text, Category: r.Category, Priority: r.Priority, Facts: wf.data,
		Allowed: wf.allowed, Known: wf.known,
	})
	r.Text, r.TextSource, r.Reason = a.Text, a.Source, a.Reason
	if r.TextSource == writer.SourceWriter {
		r.Items = cardsIn(r.Text, names.ItemCards)
	}
	emit("answer", r)
	return r, nil
}

// cardsIn returns the cards of the items text names, in the order it names
// them, once each. Only the current patch's items have cards, so a name the
// database lacks never gets one. Longer names win, so "Witch Blade" is not
// also read as a shorter name inside it.
func cardsIn(text string, cards []items.Card) []items.Card {
	byLen := slices.Clone(cards)
	slices.SortStableFunc(byLen, func(a, b items.Card) int { return utf8.RuneCountInString(b.Name) - utf8.RuneCountInString(a.Name) })
	lower := strings.ToLower(text)
	type hit struct {
		at   int
		card items.Card
	}
	var hits []hit
	for _, c := range byLen {
		name := strings.ToLower(c.Name)
		if name == "" {
			continue
		}
		for from := 0; ; {
			i := strings.Index(lower[from:], name)
			if i < 0 {
				break
			}
			i += from
			end := i + len(name)
			from = end
			if !wordEdge(lower, i-1) || !wordEdge(lower, end) {
				continue
			}
			hits = append(hits, hit{i, c})
			// Mask it so a shorter name inside it is not matched again.
			lower = lower[:i] + strings.Repeat(" ", len(name)) + lower[end:]
			break
		}
	}
	slices.SortFunc(hits, func(a, b hit) int { return a.at - b.at })
	out := make([]items.Card, 0, len(hits))
	for _, h := range hits {
		out = append(out, h.card)
	}
	return out
}

// wordEdge says whether s has no letter or digit at byte i.
func wordEdge(s string, i int) bool {
	if i < 0 || i >= len(s) {
		return true
	}
	r, _ := utf8.DecodeRuneInString(s[i:])
	if r == utf8.RuneError {
		// Inside a multi-byte rune: look back for its start.
		r, _ = utf8.DecodeLastRuneInString(s[:i+1])
	}
	return !unicode.IsLetter(r) && !unicode.IsDigit(r)
}

// resolve keeps the mentioned names the current patch has.
func resolve(p writer.Situation, heroes []catalog.Hero, names catalog.Names) Situation {
	sit := Situation{Category: p.Category, Summary: p.Summary, Heroes: []int{}, Abilities: []string{}, Items: []string{}, Unknown: []string{}}
	byName := map[string]int{}
	for _, h := range heroes {
		byName[strings.ToLower(h.Name)] = h.ID
		for _, n := range names.Heroes[h.ID] {
			byName[strings.ToLower(n)] = h.ID
		}
	}
	find := func(list []string, name string) (string, bool) {
		for _, n := range list {
			if strings.EqualFold(n, name) {
				return n, true
			}
		}
		return "", false
	}
	for _, e := range p.Entities {
		e = strings.TrimSpace(e)
		if e == "" || utf8.RuneCountInString(e) > 60 {
			continue
		}
		if id, ok := byName[strings.ToLower(e)]; ok {
			if !slices.Contains(sit.Heroes, id) {
				sit.Heroes = append(sit.Heroes, id)
			}
		} else if n, ok := find(names.Abilities, e); ok {
			sit.Abilities = append(sit.Abilities, n)
		} else if n, ok := find(names.Items, e); ok {
			sit.Items = append(sit.Items, n)
		} else {
			sit.Unknown = append(sit.Unknown, e)
		}
	}
	return sit
}

// facts is what the answer is written from, and the names it may use.
type facts struct {
	data           map[string]any
	allowed, known []string
}

// abilityBrief is an ability as the writer sees it.
type abilityBrief struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

func (c *Coach) facts(ctx context.Context, s Session, sit Situation, heroes []catalog.Hero, names catalog.Names, lang string) (facts, error) {
	byID := make(map[int]catalog.Hero, len(heroes))
	for _, h := range heroes {
		byID[h.ID] = h
	}
	f := facts{}
	allow := func(ns ...string) { f.allowed = append(f.allowed, ns...) }
	heroName := func(id int) string {
		allow(byID[id].Name)
		allow(names.Heroes[id]...)
		return byID[id].Name
	}
	list := func(ids []int) []string {
		out := []string{}
		for _, id := range ids {
			out = append(out, heroName(id))
		}
		return out
	}
	// The player's hero, and every hero they named, with its abilities.
	abilitiesOf := func(id int) ([]abilityBrief, error) {
		d, err := c.Facts.HeroDetail(ctx, byID[id].Slug, lang)
		if errors.Is(err, catalog.ErrNotFound) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		out := []abilityBrief{}
		for _, a := range d.Abilities {
			allow(a.Name)
			b := abilityBrief{Name: a.Name}
			if slices.Contains(sit.Abilities, a.Name) || slices.Contains(sit.Heroes, id) {
				b.Description = cut(a.Description, 220)
			}
			out = append(out, b)
		}
		return out, nil
	}
	you, err := abilitiesOf(s.HeroID)
	if err != nil {
		return f, err
	}
	_, ds, err := c.Facts.DraftDataset(ctx, s.Enemies)
	if err != nil {
		return f, err
	}
	matchups := []map[string]any{}
	for _, e := range s.Enemies {
		rec := ds.Matchups[draft.Pair{Hero: s.HeroID, Opponent: e}]
		matchups = append(matchups, map[string]any{
			"enemy": heroName(e), "your_win_rate_delta_points": round1(draft.Delta(rec.Wins, rec.Games)), "games": rec.Games,
		})
	}
	mentioned := []map[string]any{}
	for _, id := range sit.Heroes {
		if id == s.HeroID {
			continue
		}
		ab, err := abilitiesOf(id)
		if err != nil {
			return f, err
		}
		mentioned = append(mentioned, map[string]any{"hero": heroName(id), "abilities": ab})
	}
	plan := map[string]any{}
	for _, p := range s.ItemPlan.Core {
		if p.Phase == "start" {
			continue
		}
		var ns []string
		for _, it := range p.Items {
			ns = append(ns, it.Name)
		}
		allow(ns...)
		plan[p.Phase] = ns
	}
	situational := []map[string]any{}
	for _, it := range s.ItemPlan.Situational {
		allow(it.Item.Name)
		x := map[string]any{"item": it.Item.Name, "against": heroName(it.EnemyID)}
		if it.Urgency != "" {
			x["urgency"] = it.Urgency
		}
		situational = append(situational, x)
	}
	plan["situational"] = situational
	// The stored builds for the player's position: the only builds the
	// writer may cite besides the session's own plan.
	builds, err := c.Facts.HeroBuilds(ctx, s.HeroID)
	if err != nil {
		return f, err
	}
	stored := []map[string]any{}
	for _, b := range builds {
		if b.Position != s.Role {
			continue
		}
		byPhase := map[string][]string{}
		for _, it := range b.Items {
			allow(it.Name)
			byPhase[it.Phase] = append(byPhase[it.Phase], it.Name)
		}
		stored = append(stored, map[string]any{"source": b.Source, "items": byPhase})
	}
	allow(sit.Items...)
	allow(sit.Abilities...)
	f.data = map[string]any{
		"you":                 map[string]any{"hero": heroName(s.HeroID), "position": draft.PositionName(s.Role), "abilities": you},
		"allies":              list(s.Allies),
		"enemies":             list(s.Enemies),
		"matchups":            matchups,
		"item_plan":           plan,
		"stored_builds":       stored,
		"situation":           map[string]any{"category": sit.Category, "summary": sit.Summary},
		"mentioned_heroes":    mentioned,
		"mentioned_items":     sit.Items,
		"mentioned_abilities": sit.Abilities,
	}
	for _, ns := range names.Heroes {
		f.known = append(f.known, ns...)
	}
	f.known = slices.Concat(f.known, names.Abilities, names.Items, names.OffPatch, sit.Unknown)
	return f, nil
}

const categoryInstructions = "Which kind of advice does the player need most right now? The parsed situation " +
	"names its own guess; choose by the facts and the message summary."

const priorityInstructions = "How urgent is acting on this advice for the player right now?"

// categoryOptions describe each category for Jev.
var categoryOptions = map[string]string{
	"lane":      "laning: trading, last hits, the lane matchup, rotations into the lane",
	"teamfight": "fights: positioning, targets, when to engage or disengage",
	"items":     "what to buy next and why, against this lineup",
	"objective": "map objectives: towers, the boss, vision, farm and timings",
	"other":     "anything else about the match",
}

// decide asks Jev for the category and priority; on any failure or low
// confidence the parse's category stands at medium priority.
func (c *Coach) decide(ctx context.Context, sit Situation, data map[string]any) (string, string, string) {
	fallback := func() (string, string, string) { return sit.Category, "medium", SourceParser }
	if c.Jev == nil {
		return fallback()
	}
	options := map[string]any{}
	for _, cat := range writer.Categories {
		opt := map[string]any{"means": categoryOptions[cat]}
		if cat == sit.Category {
			opt["parsed_guess"] = true
		}
		options[cat] = opt
	}
	levels := make([]any, len(Priorities))
	for i, p := range Priorities {
		levels[i] = p
	}
	res, err := c.Jev.Ask(ctx, decision.Request{
		Purpose: "coach.advice",
		State:   data,
		Questions: map[string]decision.Question{
			"category": decision.Choice(categoryInstructions, options),
			"priority": decision.Score(priorityInstructions, levels...),
		},
	})
	if err != nil {
		return fallback()
	}
	cat, pri := res.Answers["category"], res.Answers["priority"]
	if cat.Confidence < MinConfidence {
		return fallback()
	}
	best, bestP := 1, -1.0
	for i := range Priorities {
		if p := pri.Probabilities[string(rune('0'+i))]; p > bestP {
			best, bestP = i, p
		}
	}
	return cat.Choice, Priorities[best], SourceJev
}

func cut(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return strings.TrimSpace(string(r[:n])) + "…"
}

func round1(x float64) float64 { return math.Round(x*10) / 10 }

// ReasonQuick marks a template answer to a quick action, with no model call.
const ReasonQuick = "quick"

// QuickItems answers the "items now?" quick action from the session's item
// plan alone, with no Jev or writer call (ADR-0011): the situational items
// most urgent first (later ones last), then the mid and late core items. The UI words
// the template; the cards are the answer.
func QuickItems(s Session, emit Emit) Reply {
	r := Reply{Kind: "answer", Category: "items", Priority: "medium", Source: SourceQuick, TextSource: writer.SourceTemplate, Reason: ReasonQuick}
	rank := map[string]int{"now": 0, "soon": 1, "": 2, "later": 3}
	sit := slices.Clone(s.ItemPlan.Situational)
	slices.SortStableFunc(sit, func(a, b items.Situational) int { return rank[a.Urgency] - rank[b.Urgency] })
	seen := map[int]bool{}
	add := func(it items.Item) {
		if !seen[it.ID] && len(r.Items) < QuickCards {
			seen[it.ID] = true
			r.Items = append(r.Items, items.Card{ID: it.ID, Key: it.Key, Name: it.Name, Cost: it.Cost, Img: it.Img})
		}
	}
	for _, it := range sit {
		if it.Urgency != "later" {
			add(it.Item)
		}
	}
	for _, p := range s.ItemPlan.Core {
		if p.Phase == "mid" || p.Phase == "late" {
			for _, it := range p.Items {
				add(it.Item)
			}
		}
	}
	for _, it := range sit {
		add(it.Item)
	}
	if len(sit) > 0 && sit[0].Urgency == "now" {
		r.Priority = "high"
	}
	emit("decision", map[string]string{"category": r.Category, "priority": r.Priority, "source": r.Source})
	emit("answer", r)
	return r
}

// QuickCards is how many items a quick answer shows.
const QuickCards = 4

// SourceQuick is the decision source of a quick action: the code alone.
const SourceQuick = "code"
