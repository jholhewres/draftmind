package items

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jholhewres/draftmind/api/internal/decision"
)

// lineup: the player's hero 10 against enemies 20 and 21. The heroes that
// beat 20 (hero 30) buy Eul's Scepter far more than heroes overall; the
// heroes that beat 21 (hero 31) buy Aghanim's Scepter.
func lineup() Dataset {
	items := map[int]Item{}
	for _, it := range []Item{
		{ID: 1, Key: "blink", Name: "Blink Dagger", Cost: 2250}, {ID: 44, Key: "tango", Name: "Tango", Cost: 90, Qual: "consumable"},
		{ID: 48, Key: "travel_boots", Name: "Boots of Travel", Cost: 2500}, {ID: 50, Key: "phase_boots", Name: "Phase Boots", Cost: 1500},
		{ID: 63, Key: "power_treads", Name: "Power Treads", Cost: 1400}, {ID: 96, Key: "sheepstick", Name: "Scythe of Vyse", Cost: 5200},
		{ID: 100, Key: "cyclone", Name: "Eul's Scepter of Divinity", Cost: 2625}, {ID: 108, Key: "ultimate_scepter", Name: "Aghanim's Scepter", Cost: 4200},
		{ID: 116, Key: "black_king_bar", Name: "Black King Bar", Cost: 4050},
	} {
		items[it.ID] = it
	}
	return Dataset{
		Items: items,
		Popularity: map[int]map[string][]Count{
			10: {
				"start": {{44, 900}},
				"mid":   {{50, 1000}, {1, 900}, {63, 800}, {108, 100}},
				"late":  {{116, 600}, {48, 500}, {96, 400}, {100, 30}, {999, 300}}, // 999 is not on the patch
			},
			30: {"mid": {{100, 900}, {50, 100}}, "late": {{116, 100}}},
			31: {"mid": {{108, 900}, {50, 100}}, "late": {{116, 100}}},
		},
		Counters: map[int][]int{20: {30}, 21: {31}},
		Names:    map[int]string{10: "Puck", 20: "Lion", 21: "Viper", 30: "Lina", 31: "Sniper"},
	}
}

// fakeJev answers every question: each item's enemy and urgency by item name.
type fakeJev struct {
	enemy   map[string]string // item key → enemy name
	urgency map[string]int    // item key → level
	err     error
	asked   decision.Request
}

func (f *fakeJev) Ask(_ context.Context, req decision.Request) (decision.Result, error) {
	f.asked = req
	if f.err != nil {
		return decision.Result{}, f.err
	}
	res := decision.Result{Answers: map[string]decision.Answer{}}
	for name, q := range req.Questions {
		item, kind, _ := strings.Cut(strings.TrimPrefix(name, "item_"), "_")
		switch kind {
		case "enemy":
			res.Answers[name] = decision.Answer{Type: "choice", Choice: f.enemy[item], Confidence: 0.8}
		case "urgency":
			probs := map[string]float64{}
			for i := range q.Criteria.([]any) {
				probs[string(rune('0'+i))] = 0.1
			}
			probs[string(rune('0'+f.urgency[item]))] = 0.7
			res.Answers[name] = decision.Answer{Type: "score", Probabilities: probs, Confidence: 0.8}
		}
	}
	return res, nil
}

func cited(b Build) map[string]int {
	out := map[string]int{}
	for _, s := range b.Situational {
		out[s.Item.Key] = s.EnemyID
	}
	return out
}

func TestEachSituationalItemCitesTheEnemyHeroThatTriggersIt(t *testing.T) {
	ds := lineup()
	// Jev moves Eul's to Viper, rates it urgent, and skips Aghanim's.
	jev := &fakeJev{enemy: map[string]string{"100": "Viper", "108": "Lion"}, urgency: map[string]int{"100": 3, "108": 0}}
	b := Decide(context.Background(), jev, ds, 10, []int{20, 21})
	if b.Source != SourceJev || len(b.Situational) != 1 || b.Situational[0].Item.Key != "cyclone" ||
		b.Situational[0].EnemyID != 21 || b.Situational[0].Urgency != "now" {
		t.Fatalf("build %+v, want Eul's Scepter now against Viper (21) from Jev", b)
	}

	// Jev fails: the code order stands, each item citing the enemy whose
	// beaters buy it most.
	b = Decide(context.Background(), &fakeJev{err: errors.New("HTTP 500")}, ds, 10, []int{20, 21})
	if b.Source != SourceRanking || b.Reason != ReasonError {
		t.Fatalf("source %s/%s, want ranking/error", b.Source, b.Reason)
	}
	if got := cited(b); len(got) != 2 || got["cyclone"] != 20 || got["ultimate_scepter"] != 21 {
		t.Fatalf("situational cites %v, want cyclone → 20 and ultimate_scepter → 21", got)
	}
	for _, s := range b.Situational {
		if s.Urgency != "" {
			t.Fatalf("%s has urgency %q without Jev", s.Item.Key, s.Urgency)
		}
	}
}
