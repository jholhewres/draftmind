package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jholhewres/draftmind/api/internal/catalog"
	"github.com/jholhewres/draftmind/api/internal/dbtest"
	"github.com/jholhewres/draftmind/api/internal/match"
)

// buildPatches has heroes 1 to 3 on 7.40 and 7.41, the current patch. Item
// 200 is on 7.40 only, where hero 1 bought it in every game; on 7.41 hero 1
// buys item 116, and hero 3, who beats hero 2, buys item 300.
func buildPatches(t *testing.T) *Server {
	t.Helper()
	pool := dbtest.New(t)
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO patch (name, is_current) VALUES ('7.40', false), ('7.41', true);
		INSERT INTO hero (id, slug, name, primary_attr, attack_type, img, icon)
		SELECT i, 'hero' || i, 'Hero ' || i, 'str', 'Melee', '/h' || i || '.png', '/h' || i || '_icon.png' FROM generate_series(1, 3) i;
		INSERT INTO hero_patch (patch_id, hero_id) SELECT p.id, h.id FROM patch p CROSS JOIN hero h;
		INSERT INTO item (patch_id, id, key, dname, cost) SELECT id, 116, 'black_king_bar', 'Black King Bar', 4050 FROM patch;
		INSERT INTO item (patch_id, id, key, dname, cost) SELECT id, 200, 'old_item', 'Old Item', 1000 FROM patch WHERE name = '7.40';
		INSERT INTO item (patch_id, id, key, dname, cost) SELECT id, 300, 'new_item', 'New Item', 3000 FROM patch WHERE name = '7.41';
		INSERT INTO hero_item_popularity (patch_id, hero_id, phase, item_id, count)
		SELECT id, 1, 'mid', 200, 99999 FROM patch WHERE name = '7.40';
		INSERT INTO hero_item_popularity (patch_id, hero_id, phase, item_id, count)
		SELECT p.id, v.hero, v.phase, v.item, v.n FROM patch p, (VALUES
			(1, 'mid', 116, 500), (3, 'mid', 300, 900), (3, 'late', 116, 10)
		) AS v(hero, phase, item, n) WHERE p.name = '7.41';
		INSERT INTO hero_matchup (patch_id, hero_id, opponent_id, games, wins)
		SELECT id, 3, 2, 1000, 700 FROM patch WHERE name = '7.41'`); err != nil {
		t.Fatal(err)
	}
	return New(fakeDB{}, catalog.NewStore(pool), quietLog()).WithMatches(match.NewStore(pool))
}

func TestBuildSuggestsOnlyItemsOfTheCurrentPatch(t *testing.T) {
	srv := buildPatches(t)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/draft/build", strings.NewReader(`{"hero":1,"enemies":[2]}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	var b struct {
		Patch string
		Core  []struct {
			Phase string
			Items []struct{ ID int }
		}
		Situational []struct {
			Item    struct{ ID int }
			EnemyID int
		}
		Source string
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &b); err != nil {
		t.Fatal(err)
	}
	var core []int
	for _, p := range b.Core {
		for _, it := range p.Items {
			core = append(core, it.ID)
		}
	}
	// No decision model: the code order, with New Item answering hero 2.
	if b.Patch != "7.41" || len(core) != 1 || core[0] != 116 || len(b.Situational) != 1 ||
		b.Situational[0].Item.ID != 300 || b.Situational[0].EnemyID != 2 || b.Source != "ranking" {
		t.Fatalf("build %s", rec.Body)
	}

	code, hb := getFrom[catalog.HeroBuild](t, srv, "/api/heroes/hero1/build")
	if code != http.StatusOK || hb.Patch != "7.41" || hb.Source != catalog.BuildSource {
		t.Fatalf("hero build %d %+v", code, hb)
	}
	var ids []int
	for _, p := range hb.Core {
		for _, it := range p.Items {
			ids = append(ids, it.ID)
		}
	}
	if len(ids) != 1 || ids[0] != 116 {
		t.Fatalf("hero build items %v, want [116] from 7.41 only", ids)
	}
}

func getFrom[T any](t *testing.T, srv *Server, target string) (int, T) {
	t.Helper()
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
	var body T
	if rec.Code == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
	}
	return rec.Code, body
}
