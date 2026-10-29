// Package items builds a hero's item build from the synced item popularity
// (ADR-0003): code builds the core items and the situational candidates,
// Jev picks the situational items, their urgency and the enemy each one
// answers, and the code order stands when Jev cannot.
package items

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"

	"github.com/jholhewres/draftmind/api/internal/decision"
)

// Phases in game order, as hero_item_popularity names them.
var Phases = []string{"start", "early", "mid", "late"}

// Sizes of the build.
const (
	CoreStart       = 6 // starting items shown
	CorePerPhase    = 3 // early, mid and late items shown
	Candidates      = 6 // situational candidates Jev chooses among
	Shown           = 4 // situational items kept
	CountersPerFoe  = 5 // heroes that beat an enemy whose items count as answers to it
	minCandidateUse = 0.02
)

// Item is an item of the current patch.
type Item struct {
	ID   int    `json:"id"`
	Key  string `json:"key"`
	Name string `json:"name"`
	Cost int    `json:"cost"`
	Qual string `json:"-"`
	// Components are the keys of the items it is built from.
	Components []string `json:"-"`
}

// Count is how many of a hero's games had an item in one phase.
type Count struct {
	ItemID int
	Count  int
}

// Dataset is the current patch's data a build reads.
type Dataset struct {
	Items map[int]Item
	// Popularity is each hero's item counts by phase.
	Popularity map[int]map[string][]Count
	// Counters are, for each enemy, the heroes that beat it, strongest first.
	Counters map[int][]int
	// Names are hero names, for Jev.
	Names map[int]string
}

// PhaseItems is one phase of the core build.
type PhaseItems struct {
	Phase string      `json:"phase"`
	Items []ItemCount `json:"items"`
}

// ItemCount is an item with the games it was bought in.
type ItemCount struct {
	Item
	Count int `json:"count"`
}

// Situational is an item for this lineup, the enemy it answers and, when
// Jev chose it, how urgent it is.
type Situational struct {
	Item    Item    `json:"item"`
	EnemyID int     `json:"enemyId"`
	Urgency string  `json:"urgency,omitempty"` // one of Urgencies; empty in the code order
	Score   float64 `json:"score"`             // the code's evidence, in share points
}

// Urgencies are Jev's levels, least urgent first; "skip" drops the item.
var Urgencies = []string{"skip", "later", "soon", "now"}

// Where the situational order came from, and why the code order stands.
const (
	SourceJev     = "jev"
	SourceRanking = "ranking"

	ReasonTimeout       = "timeout"
	ReasonError         = "error"
	ReasonUnavailable   = "unavailable"
	ReasonFewCandidates = "few_candidates"
)

// MinConfidence is the least confidence Jev's enemy choice needs to replace
// the code's enemy for an item (ADR-0003: shadow below it). Urgency is a
// score over four ordered levels, whose confidence runs lower by nature (0.1
// to 0.4 on the live 7.41 calls), so its most likely level always stands.
const MinConfidence = 0.4

// Build is a hero's build against a lineup.
type Build struct {
	HeroID      int           `json:"heroId"`
	Core        []PhaseItems  `json:"core"`
	Situational []Situational `json:"situational"`
	Source      string        `json:"source"`
	Reason      string        `json:"reason,omitempty"`
}

// Asker asks the decision model.
type Asker interface {
	Ask(ctx context.Context, req decision.Request) (decision.Result, error)
}

// Core is the hero's most bought items by phase, from the dataset's
// current-patch items only; an item stays in the first phase it tops.
func Core(ds Dataset, hero int) []PhaseItems {
	seen := map[int]bool{}
	out := make([]PhaseItems, 0, len(Phases))
	for _, ph := range Phases {
		size := CorePerPhase
		if ph == "start" {
			size = CoreStart
		}
		p := PhaseItems{Phase: ph, Items: []ItemCount{}}
		for _, c := range sorted(ds.Popularity[hero][ph]) {
			it, ok := ds.Items[c.ItemID]
			if !ok || seen[c.ItemID] || len(p.Items) == size {
				continue
			}
			seen[c.ItemID] = true
			p.Items = append(p.Items, ItemCount{Item: it, Count: c.Count})
		}
		out = append(out, p)
	}
	return out
}

func sorted(cs []Count) []Count {
	out := slices.Clone(cs)
	slices.SortFunc(out, func(a, b Count) int { return cmp.Or(cmp.Compare(b.Count, a.Count), cmp.Compare(a.ItemID, b.ItemID)) })
	return out
}

// lateShares is each item's share of a hero's mid and late game purchases.
func lateShares(ds Dataset, hero int) map[int]float64 {
	total := 0
	for _, ph := range []string{"mid", "late"} {
		for _, c := range ds.Popularity[hero][ph] {
			total += c.Count
		}
	}
	out := map[int]float64{}
	if total == 0 {
		return out
	}
	for _, ph := range []string{"mid", "late"} {
		for _, c := range ds.Popularity[hero][ph] {
			out[c.ItemID] += float64(c.Count) / float64(total)
		}
	}
	return out
}

// Candidate is a situational candidate with its evidence against each enemy.
type Candidate struct {
	Item Item
	// Affinity is the item's share of this hero's mid and late purchases.
	Affinity float64
	// Lift is, per enemy, how much more the heroes that beat it buy the
	// item than heroes overall, in share points; only positive lifts.
	Lift map[int]float64
	// Enemy is the enemy with the largest lift.
	Enemy int
	Score float64
}

// SituationalCandidates are the situational candidates, strongest first: mid and
// late items, not consumables and not in the core, that this hero buys or
// that the heroes beating an enemy buy more than heroes overall, and not a
// component of a core item or of another candidate. Every candidate answers
// an enemy: one with no lift against any is dropped.
func SituationalCandidates(ds Dataset, hero int, enemies []int, core []PhaseItems) []Candidate {
	inCore := map[int]bool{}
	for _, p := range core {
		for _, it := range p.Items {
			inCore[it.ID] = true
		}
	}
	// Baseline: the mean share across every hero with data.
	base := map[int]float64{}
	heroes := 0
	for _, h := range slices.Sorted(maps.Keys(ds.Popularity)) {
		sh := lateShares(ds, h)
		if len(sh) == 0 {
			continue
		}
		heroes++
		for id, s := range sh {
			base[id] += s
		}
	}
	for id := range base {
		base[id] /= float64(max(heroes, 1))
	}
	// Per enemy, the mean share among the heroes that beat it.
	byEnemy := map[int]map[int]float64{}
	for _, e := range enemies {
		cs := ds.Counters[e][:min(CountersPerFoe, len(ds.Counters[e]))]
		m := map[int]float64{}
		for _, c := range cs {
			for id, s := range lateShares(ds, c) {
				m[id] += s / float64(len(cs))
			}
		}
		byEnemy[e] = m
	}
	own := lateShares(ds, hero)
	pool := map[int]bool{}
	for id, s := range own {
		if s >= minCandidateUse {
			pool[id] = true
		}
	}
	for _, m := range byEnemy {
		for id, s := range m {
			if s >= minCandidateUse {
				pool[id] = true
			}
		}
	}
	partOf := map[string]bool{}
	for id := range pool {
		for _, k := range ds.Items[id].Components {
			partOf[k] = true
		}
	}
	for _, p := range core {
		for _, it := range p.Items {
			for _, k := range ds.Items[it.ID].Components {
				partOf[k] = true
			}
		}
	}
	var out []Candidate
	for _, id := range slices.Sorted(maps.Keys(pool)) {
		it, ok := ds.Items[id]
		if !ok || inCore[id] || it.Qual == "consumable" || partOf[it.Key] {
			continue
		}
		c := Candidate{Item: it, Affinity: own[id], Lift: map[int]float64{}}
		best := 0.0
		for _, e := range enemies {
			if l := byEnemy[e][id] - base[id]; l > 0 {
				c.Lift[e] = l
				if l > best {
					best, c.Enemy = l, e
				}
			}
		}
		if c.Enemy == 0 {
			continue
		}
		c.Score = c.Affinity + best
		out = append(out, c)
	}
	slices.SortFunc(out, func(a, b Candidate) int {
		return cmp.Or(cmp.Compare(b.Score, a.Score), cmp.Compare(a.Item.ID, b.Item.ID))
	})
	return out[:min(Candidates, len(out))]
}

const urgencyInstructions = "How urgently should the player buy %s in this match, given the enemy lineup? " +
	"The evidence is how much of their mid and late game purchases it is for this hero, and how much more the heroes " +
	"that beat each enemy buy it than heroes overall, in percentage points."

const enemyInstructions = "Which enemy hero does %s answer best? Each option is an enemy whose beaters buy it more " +
	"than heroes overall, with how much more, in percentage points."

// Decide builds the hero's build: the core from popularity, then Jev picks
// the situational items among the candidates, with their urgency and the
// enemy each one answers. Any failure keeps the code order, each item
// citing the enemy with the largest lift.
func Decide(ctx context.Context, jev Asker, ds Dataset, hero int, enemies []int) Build {
	core := Core(ds, hero)
	cands := SituationalCandidates(ds, hero, enemies, core)
	b := Build{HeroID: hero, Core: core, Situational: []Situational{}}
	fallback := func(reason string) Build {
		b.Source, b.Reason = SourceRanking, reason
		for _, c := range cands[:min(Shown, len(cands))] {
			b.Situational = append(b.Situational, Situational{Item: c.Item, EnemyID: c.Enemy, Score: pct(c.Score)})
		}
		return b
	}
	if len(cands) == 0 {
		return fallback(ReasonFewCandidates)
	}
	if jev == nil {
		return fallback(ReasonUnavailable)
	}
	name := func(id int) string {
		if n := ds.Names[id]; n != "" {
			return n
		}
		return "#" + strconv.Itoa(id)
	}
	byName := map[string]int{}
	for _, e := range enemies {
		byName[name(e)] = e
	}
	levels := make([]any, len(Urgencies))
	for i, u := range Urgencies {
		levels[i] = u
	}
	qs := map[string]decision.Question{}
	evidence := make([]map[string]any, len(cands))
	for i, c := range cands {
		options := map[string]any{}
		lifts := map[string]float64{}
		// Only the enemies the data ties the item to are options.
		for _, e := range enemies {
			if l, ok := c.Lift[e]; ok {
				options[name(e)] = map[string]any{"lift_points": pct(l)}
				lifts[name(e)] = pct(l)
			}
		}
		key := "item_" + strconv.Itoa(c.Item.ID)
		qs[key+"_urgency"] = decision.Score(fmt.Sprintf(urgencyInstructions, c.Item.Name), levels...)
		qs[key+"_enemy"] = decision.Choice(fmt.Sprintf(enemyInstructions, c.Item.Name), options)
		evidence[i] = map[string]any{"item": c.Item.Name, "cost": c.Item.Cost, "share_for_hero_points": pct(c.Affinity),
			"lift_against_enemies": lifts}
	}
	coreNames := []string{}
	for _, p := range core {
		for _, it := range p.Items {
			coreNames = append(coreNames, it.Name)
		}
	}
	enemyNames := make([]string, len(enemies))
	for i, e := range enemies {
		enemyNames[i] = name(e)
	}
	res, err := jev.Ask(ctx, decision.Request{
		Purpose: "items.build",
		State: map[string]any{
			"task":       "Pick the situational items a hero should buy against this enemy lineup, and the enemy each answers.",
			"hero":       name(hero),
			"enemies":    enemyNames,
			"core_build": coreNames,
			"candidates": evidence,
		},
		Questions: qs,
	})
	if errors.Is(err, context.DeadlineExceeded) {
		return fallback(ReasonTimeout)
	}
	if err != nil {
		return fallback(ReasonError)
	}
	type picked struct {
		Situational
		level, order int
	}
	var kept []picked
	for i, c := range cands {
		key := "item_" + strconv.Itoa(c.Item.ID)
		level := argmax(res.Answers[key+"_urgency"].Probabilities, len(Urgencies))
		if level <= 0 {
			continue
		}
		choice := res.Answers[key+"_enemy"]
		enemy, ok := byName[choice.Choice]
		if !ok {
			return fallback(ReasonError)
		}
		if choice.Confidence < MinConfidence {
			enemy = c.Enemy
		}
		kept = append(kept, picked{Situational{Item: c.Item, EnemyID: enemy, Urgency: Urgencies[level], Score: pct(c.Score)}, level, i})
	}
	slices.SortStableFunc(kept, func(a, b picked) int { return cmp.Or(cmp.Compare(b.level, a.level), cmp.Compare(a.order, b.order)) })
	b.Source = SourceJev
	for _, k := range kept[:min(Shown, len(kept))] {
		b.Situational = append(b.Situational, k.Situational)
	}
	return b
}

// argmax is the level with the largest probability; ties go to the lower.
func argmax(probs map[string]float64, n int) int {
	best, bestP := 0, -1.0
	for i := range n {
		if p := probs[strconv.Itoa(i)]; p > bestP {
			best, bestP = i, p
		}
	}
	return best
}

// pct turns a share into points, one decimal.
func pct(x float64) float64 { return float64(int(x*1000+0.5)) / 10 }
