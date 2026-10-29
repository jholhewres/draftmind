package httpapi

import (
	"context"
	"net/http"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jholhewres/draftmind/api/internal/catalog"
	"github.com/jholhewres/draftmind/api/internal/dbtest"
)

// counterPatches has heroes 1 to 8 on 7.40 and on 7.41, the current patch.
// On 7.41 Anti-Mage (1) has these records; on 7.40 one record that would
// top both lists if it were read.
func counterPatches(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool := dbtest.New(t)
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO patch (name, is_current) VALUES ('7.40', false), ('7.41', true);
		INSERT INTO hero (id, slug, name, primary_attr, attack_type, img, icon)
		SELECT i, 'hero' || i, 'Hero ' || i, 'str', 'Melee', '/h' || i || '.png', '/h' || i || '_icon.png' FROM generate_series(1, 8) i;
		UPDATE hero SET slug = 'antimage', name = 'Anti-Mage' WHERE id = 1;
		INSERT INTO hero_patch (patch_id, hero_id) SELECT p.id, h.id FROM patch p CROSS JOIN hero h;
		INSERT INTO hero_matchup (patch_id, hero_id, opponent_id, games, wins)
		SELECT p.id, 1, m.opp, m.games, m.wins FROM patch p, (VALUES
			(2, 300, 180), -- +7.5
			(3, 100, 60),  -- +5
			(4, 50, 40),   -- +10, under 100 games
			(5, 200, 100), -- 0
			(6, 300, 120), -- -7.5
			(7, 100, 40),  -- -5
			(8, 20, 0)     -- -8.33, under 100 games
		) AS m(opp, games, wins) WHERE p.name = '7.41';
		INSERT INTO hero_matchup (patch_id, hero_id, opponent_id, games, wins)
		SELECT id, 1, 5, 1000, 900 FROM patch WHERE name = '7.40';
		INSERT INTO hero_matchup (patch_id, hero_id, opponent_id, games, wins)
		SELECT id, 2, 1, 1000, 100 FROM patch WHERE name = '7.40'`); err != nil {
		t.Fatal(err)
	}
	return pool
}

type sideIDs struct {
	ids    []int
	deltas []float64
	low    []bool
}

func side(cs []catalog.Counter) sideIDs {
	var s sideIDs
	for _, c := range cs {
		s.ids = append(s.ids, c.ID)
		s.deltas = append(s.deltas, c.Delta)
		s.low = append(s.low, c.LowSample)
	}
	return s
}

func TestCountersComeFromTheCurrentPatchMatchups(t *testing.T) {
	pool := counterPatches(t)
	code, c := get[catalog.Counters](t, pool, "/api/heroes/antimage/counters")
	if code != http.StatusOK {
		t.Fatalf("status %d", code)
	}
	good, bad := side(c.GoodAgainst), side(c.BadAgainst)
	if c.Patch != "7.41" || !slices.Equal(good.ids, []int{4, 2, 3}) || !slices.Equal(bad.ids, []int{8, 6, 7}) {
		t.Fatalf("patch %s, good against %v, bad against %v; want 7.41, [4 2 3], [8 6 7]", c.Patch, good.ids, bad.ids)
	}
	if good.deltas[0] != 10 || good.deltas[1] != 7.5 || bad.deltas[1] != -7.5 || c.GoodAgainst[1].Games != 300 {
		t.Fatalf("deltas %v %v", good.deltas, bad.deltas)
	}
	if c.GoodAgainst[0].Slug != "hero4" || c.GoodAgainst[0].Img != "/h4.png" {
		t.Fatalf("counter hero %+v", c.GoodAgainst[0].Hero)
	}
}

func TestCountersMarkLowSampleMatchups(t *testing.T) {
	pool := counterPatches(t)
	_, c := get[catalog.Counters](t, pool, "/api/heroes/antimage/counters")
	good, bad := side(c.GoodAgainst), side(c.BadAgainst)
	// Under 100 games (draft.ShrinkGames) a record is weaker.
	if !slices.Equal(good.low, []bool{true, false, false}) || !slices.Equal(bad.low, []bool{true, false, false}) {
		t.Fatalf("low sample good %v bad %v", good.low, bad.low)
	}
}

func TestCountersReadNoNumberFromAnOlderPatch(t *testing.T) {
	pool := counterPatches(t)
	// Hero 2 has a record only on 7.40.
	code, c := get[catalog.Counters](t, pool, "/api/heroes/hero2/counters")
	if code != http.StatusOK || c.Patch != "7.41" || len(c.GoodAgainst) != 0 || len(c.BadAgainst) != 0 {
		t.Fatalf("status %d, counters %+v; want 7.41 with none", code, c)
	}
	if code, _ := get[catalog.Counters](t, pool, "/api/heroes/nobody/counters"); code != http.StatusNotFound {
		t.Fatalf("unknown hero: status %d, want 404", code)
	}
}
