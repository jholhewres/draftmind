package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jholhewres/draftmind/api/internal/catalog"
	"github.com/jholhewres/draftmind/api/internal/dbtest"
	"github.com/jholhewres/draftmind/api/internal/decision"
	"github.com/jholhewres/draftmind/api/internal/draft"
)

// seedDraft makes 7.41 current with heroes 1 (Carry), 2 (Initiator),
// 5 (Support), 7 (Initiator) and 8 (Carry); hero 9 is only on 7.40. Hero 8
// beats 2 60-40 over 100 games, hero 1 loses 40-60.
func seedDraft(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool := dbtest.New(t)
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO patch (name, is_current) VALUES ('7.40', false), ('7.41', true);
		INSERT INTO hero (id, slug, name, primary_attr, attack_type, roles, img, icon) VALUES
			(1, 'antimage', 'Anti-Mage', 'agi', 'Melee', '{Carry}', '/am.png', '/am_i.png'),
			(2, 'axe', 'Axe', 'str', 'Melee', '{Initiator}', '/axe.png', '/axe_i.png'),
			(5, 'crystal_maiden', 'Crystal Maiden', 'int', 'Ranged', '{Support}', '/cm.png', '/cm_i.png'),
			(7, 'earthshaker', 'Earthshaker', 'str', 'Melee', '{Initiator}', '/es.png', '/es_i.png'),
			(8, 'juggernaut', 'Juggernaut', 'agi', 'Melee', '{Carry}', '/jugg.png', '/jugg_i.png'),
			(9, 'mirana', 'Mirana', 'all', 'Ranged', '{Carry}', '/mir.png', '/mir_i.png');
		INSERT INTO hero_patch (patch_id, hero_id) SELECT p.id, h.id FROM patch p CROSS JOIN hero h
			WHERE p.name = '7.41' AND h.id <> 9 OR p.name = '7.40' AND h.id = 9;
		INSERT INTO hero_matchup (patch_id, hero_id, opponent_id, games, wins)
			SELECT id, 8, 2, 100, 60 FROM patch WHERE is_current UNION ALL
			SELECT id, 1, 2, 100, 40 FROM patch WHERE is_current;
		INSERT INTO hero_stat (patch_id, hero_id, bracket, picks, wins)
			SELECT id, 5, '3', 9900, 5445 FROM patch WHERE is_current UNION ALL
			SELECT id, 5, 'pro', 100, 100 FROM patch WHERE is_current`); err != nil {
		t.Fatal(err)
	}
	return pool
}

type suggestBody struct {
	Patch       string             `json:"patch"`
	Source      string             `json:"source"`
	Reason      string             `json:"reason"`
	Confidence  *float64           `json:"confidence"`
	Suggestions []draft.Suggestion `json:"suggestions"`
	Error       string             `json:"error"`
}

func suggest(t *testing.T, pool *pgxpool.Pool, body string) (int, suggestBody) {
	t.Helper()
	return suggestWith(t, pool, nil, body)
}

func suggestWith(t *testing.T, pool *pgxpool.Pool, jev draft.Asker, body string) (int, suggestBody) {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/draft/suggest", strings.NewReader(body))
	New(fakeDB{}, catalog.NewStore(pool), quietLog()).WithDecider(jev).Handler().ServeHTTP(rec, req)
	var out suggestBody
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("status %d, body %q: %v", rec.Code, rec.Body, err)
	}
	return rec.Code, out
}

func TestDraftSuggestRanksTheLegalHeroesWithTheDeltaPerEnemy(t *testing.T) {
	pool := seedDraft(t)
	code, body := suggest(t, pool, `{"allies": [5], "enemies": [2], "bans": [7], "role": 1}`)
	if code != http.StatusOK || body.Patch != "7.41" {
		t.Fatalf("status %d, patch %q: %+v", code, body.Patch, body)
	}
	// 8: +5.0 vs Axe and fits pos 1; 1: -5.0 and fits pos 1. 5, 2 and 7 are taken, 9 is off the patch.
	if len(body.Suggestions) != 2 || body.Suggestions[0].HeroID != 8 || body.Suggestions[1].HeroID != 1 {
		t.Fatalf("suggestions %+v, want 8 then 1", body.Suggestions)
	}
	if e := body.Suggestions[0].Enemies; len(e) != 1 || e[0] != (draft.EnemyDelta{HeroID: 2, Delta: 5, Games: 100}) {
		t.Fatalf("Juggernaut vs enemies %+v, want +5 vs Axe over 100 games", e)
	}
}

func TestDraftSuggestReadsThePatchWinRateFromThePubBrackets(t *testing.T) {
	pool := seedDraft(t)
	_, body := suggest(t, pool, `{"role": 5}`)
	for _, s := range body.Suggestions {
		// Crystal Maiden: 5445 wins in 9900 pub picks, (5445+50)/10000; pro games left out.
		if s.HeroID == 5 && math.Abs(s.WinRate-54.95) > 1e-9 {
			t.Fatalf("Crystal Maiden win rate %v, want 54.95", s.WinRate)
		}
	}
}

func TestDraftSuggestReturnsEightByDefaultOrTheLimit(t *testing.T) {
	pool := seedDraft(t)
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO hero (id, slug, name, primary_attr, attack_type, roles, img, icon)
			SELECT g, 'h' || g, 'Hero ' || g, 'str', 'Melee', '{}', '/x.png', '/x_i.png' FROM generate_series(20, 29) g;
		INSERT INTO hero_patch (patch_id, hero_id) SELECT p.id, g FROM patch p, generate_series(20, 29) g WHERE p.is_current`); err != nil {
		t.Fatal(err)
	}
	if _, body := suggest(t, pool, `{"role": 2}`); len(body.Suggestions) != 8 {
		t.Fatalf("%d suggestions, want 8 by default", len(body.Suggestions))
	}
	if _, body := suggest(t, pool, `{"role": 2, "limit": 3}`); len(body.Suggestions) != 3 {
		t.Fatalf("%d suggestions, want the limit of 3", len(body.Suggestions))
	}
}

func TestDraftSuggestRejectsABadDraft(t *testing.T) {
	pool := seedDraft(t)
	cases := map[string]string{
		`{"enemies": [999], "role": 1}`:              "unknown hero 999",
		`{"bans": [9], "role": 1}`:                   "unknown hero 9",
		`{"enemies": [2147483648], "role": 1}`:       "unknown hero 2147483648",
		`{"enemies": [4294967296], "role": 1}`:       "unknown hero 4294967296",
		`{"allies": [-1], "role": 1}`:                "unknown hero -1",
		`{"enemies": [2, 2], "role": 1}`:             "hero 2 is in the draft twice",
		`{"allies": [5], "bans": [5], "role": 1}`:    "hero 5 is in the draft twice",
		`{"role": 0}`:                                "role must be 1 to 5",
		`{"role": 6}`:                                "role must be 1 to 5",
		`{"allies": [1, 2, 5, 7, 8], "role": 1}`:     "at most 4 allies",
		`{"enemies": [1, 2, 5, 7, 8, 1], "role": 1}`: "at most 5 enemies",
		`{"role": 1, "limit": 0}`:                    "limit must be 1 to 50",
		`{"role": 1, "picks": []}`:                   "unknown field",
		`{"role": 1`:                                 "invalid JSON",
		`{"role": 1} {}`:                             "invalid JSON",
	}
	for in, want := range cases {
		code, body := suggest(t, pool, in)
		if code != http.StatusBadRequest || !strings.Contains(body.Error, want) {
			t.Errorf("%s: status %d error %q, want 400 with %q", in, code, body.Error, want)
		}
	}
}

// jevOn is a decision client on an httptest server that runs handle,
// logging every call to the test database.
func jevOn(t *testing.T, pool *pgxpool.Pool, handle http.HandlerFunc) *decision.Client {
	t.Helper()
	srv := httptest.NewServer(handle)
	t.Cleanup(srv.Close)
	c := decision.New(srv.URL, "test-key", "typesafe/jev-1.13", "typesafe", decision.NewPGLog(pool, quietLog()))
	c.HTTP = srv.Client()
	return c
}

func TestDraftSuggestWithoutADecisionModelLabelsTheCodeRanking(t *testing.T) {
	pool := seedDraft(t)
	_, body := suggest(t, pool, `{"enemies": [2], "role": 1}`)
	if body.Source != "ranking" || body.Reason != "unavailable" || body.Confidence != nil {
		t.Fatalf("source %q reason %q, want the ranking labelled unavailable", body.Source, body.Reason)
	}
}

func TestDraftSuggestReturnsJevsOrderAndLogsTheCall(t *testing.T) {
	pool := seedDraft(t)
	var asked map[string]any
	jev := jevOn(t, pool, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&asked)
		_, _ = io.WriteString(w, `{"model": "typesafe/jev-1.13.0", "answers": {"pick": {"type": "choice",
			"choice": "Anti-Mage", "confidence": 0.5, "probabilities": {"Anti-Mage": 0.7, "Juggernaut": 0.3}}},
			"usage": {"input_tokens": 512, "output_tokens": 24, "cost": 0.0003}}`)
	})
	code, body := suggestWith(t, pool, jev, `{"enemies": [2], "bans": [5, 7], "role": 1}`)
	if code != http.StatusOK || body.Source != "jev" || body.Confidence == nil || *body.Confidence != 0.5 {
		t.Fatalf("status %d source %q confidence %v, want Jev's answer", code, body.Source, body.Confidence)
	}
	// The code ranks Juggernaut (+5 vs Axe) first; Jev prefers Anti-Mage.
	if len(body.Suggestions) != 2 || body.Suggestions[0].HeroID != 1 || body.Suggestions[1].HeroID != 8 {
		t.Fatalf("suggestions %+v, want Anti-Mage then Juggernaut", body.Suggestions)
	}
	state, _ := json.Marshal(asked["state"])
	if !strings.Contains(string(state), `"enemies":["Axe"]`) {
		t.Fatalf("state %s, want the enemies by name from the database", state)
	}
	var model, outcome string
	var latency int
	var cost float64
	if err := pool.QueryRow(context.Background(),
		`SELECT model, outcome, latency_ms, cost_usd::float8 FROM decision_log`).Scan(&model, &outcome, &latency, &cost); err != nil {
		t.Fatalf("decision_log: %v", err)
	}
	if model != "typesafe/jev-1.13" || outcome != "ok" || latency < 0 || cost != 0.0003 {
		t.Fatalf("logged model %q outcome %q latency %d cost %v", model, outcome, latency, cost)
	}
}

func TestDraftSuggestFallsBackToTheCodeRankingWhenJevIsLate(t *testing.T) {
	pool := seedDraft(t)
	jev := jevOn(t, pool, func(_ http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		select {
		case <-r.Context().Done():
		case <-time.After(2 * time.Second):
		}
	})
	jev.Deadline, jev.HedgeAfter = 50*time.Millisecond, 20*time.Millisecond
	_, body := suggestWith(t, pool, jev, `{"enemies": [2], "bans": [5, 7], "role": 1}`)
	if body.Source != "ranking" || body.Reason != "timeout" || len(body.Suggestions) != 2 || body.Suggestions[0].HeroID != 8 {
		t.Fatalf("source %q reason %q suggestions %+v, want the code ranking labelled timeout", body.Source, body.Reason, body.Suggestions)
	}
	var outcome string
	var hedged bool
	if err := pool.QueryRow(context.Background(), `SELECT outcome, hedged FROM decision_log`).Scan(&outcome, &hedged); err != nil {
		t.Fatalf("decision_log: %v", err)
	}
	if outcome != "timeout" || !hedged {
		t.Fatalf("logged outcome %q hedged %t, want a hedged timeout", outcome, hedged)
	}
}
