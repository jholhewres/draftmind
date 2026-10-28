package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jholhewres/draftmind/api/internal/catalog"
	"github.com/jholhewres/draftmind/api/internal/dbtest"
	"github.com/jholhewres/draftmind/api/internal/gamesync"
)

// testdataFeed serves the gamesync herodata fixtures.
type testdataFeed struct{}

func (testdataFeed) HeroData(_ context.Context, heroID int, language string) ([]byte, error) {
	return os.ReadFile(fmt.Sprintf("../gamesync/testdata/herodata_%d_%s.json", heroID, language))
}

const puck = `"13": {"id": 13, "name": "npc_dota_hero_puck", "localized_name": "Puck", "primary_attr": "int", "attack_type": "Ranged", "roles": ["Initiator", "Escape"], "img": "/puck.png?", "icon": "/puck_icon.png?"}`

// syncedHeroDetails syncs 7.41 with Axe and Puck and their details.
func syncedHeroDetails(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool := dbtest.New(t)
	ctx := context.Background()
	src := memSource{"patch.json": twoPatches, "heroes.json": "{" + axe + "," + puck + "}"}
	s := gamesync.NewSyncer(pool, src, quietLog())
	if _, err := s.Heroes(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Details(ctx, testdataFeed{}); err != nil {
		t.Fatal(err)
	}
	return pool
}

func getHero(t *testing.T, pool *pgxpool.Pool, target, acceptLanguage string) (*httptest.ResponseRecorder, catalog.HeroDetail) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, target, nil)
	if acceptLanguage != "" {
		req.Header.Set("Accept-Language", acceptLanguage)
	}
	rec := httptest.NewRecorder()
	New(fakeDB{}, catalog.NewStore(pool), quietLog()).Handler().ServeHTTP(rec, req)
	var body catalog.HeroDetail
	if rec.Code == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
	}
	return rec, body
}

func TestHeroDetailReturnsTheCurrentPatchStatsAndAbilities(t *testing.T) {
	pool := syncedHeroDetails(t)
	rec, body := getHero(t, pool, "/api/heroes/puck?lang=en", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	if body.Patch != "7.41" || body.Lang != "en" || body.Slug != "puck" || body.Name != "Puck" || body.AttackType != "Ranged" {
		t.Fatalf("unexpected hero: %+v", body)
	}
	if body.Stats == nil || body.Stats.MaxHealth != 494 || body.Stats.AttackRange != 550 || body.Stats.IntBase != 23 {
		t.Fatalf("unexpected stats: %+v", body.Stats)
	}
	if len(body.Abilities) != 5 || body.Abilities[0].Name != "Illusory Orb" || body.Abilities[3].Kind != "innate" ||
		len(body.Abilities[4].Cooldowns) != 1 || body.Abilities[4].Cooldowns[0] != 75 {
		t.Fatalf("unexpected abilities: %+v", body.Abilities)
	}
	if len(body.Talents) != 8 || body.Talents[0].Level != 10 || body.Talents[7].Level != 25 ||
		body.Talents[6].Name != "-30s Dream Coil Cooldown" {
		t.Fatalf("unexpected talents: %+v", body.Talents)
	}
}

func TestHeroDetailIsInTheRequestedLanguage(t *testing.T) {
	pool := syncedHeroDetails(t)
	for _, tc := range []struct{ name, target, accept string }{
		{"query", "/api/heroes/puck?lang=pt-BR", ""},
		{"accept-language", "/api/heroes/puck", "pt-BR,pt;q=0.9,en;q=0.8"},
		{"accept-language primary tag", "/api/heroes/puck", "pt-PT"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec, body := getHero(t, pool, tc.target, tc.accept)
			if rec.Code != http.StatusOK {
				t.Fatalf("status %d: %s", rec.Code, rec.Body)
			}
			if body.Lang != "pt-BR" || body.Abilities[4].Name != "Espiral dos Sonhos" ||
				body.Talents[0].Name != "+35 de dano para Orbe Ilusório" {
				t.Fatalf("lang %s, abilities %q, talent %q", body.Lang, body.Abilities[4].Name, body.Talents[0].Name)
			}
		})
	}
}

func TestHeroDetailFallsBackToEnglish(t *testing.T) {
	pool := syncedHeroDetails(t)
	rec, body := getHero(t, pool, "/api/heroes/axe?lang=xx", "de-DE")
	if rec.Code != http.StatusOK || body.Lang != "en" || body.Abilities[0].Name != "Berserker's Call" {
		t.Fatalf("status %d lang %q first ability %+v", rec.Code, body.Lang, body.Abilities)
	}
}

func TestHeroDetailIs404ForAHeroNotInTheCurrentPatch(t *testing.T) {
	pool := syncedHeroDetails(t)
	// Crystal Maiden is only on the old patch.
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO hero (id, slug, name, primary_attr, attack_type, img, icon) VALUES (5, 'crystal_maiden', 'Crystal Maiden', 'int', 'Ranged', '/cm.png', '/cm_i.png');
		INSERT INTO hero_patch (patch_id, hero_id) SELECT id, 5 FROM patch WHERE name = '7.40'`); err != nil {
		t.Fatal(err)
	}
	for _, slug := range []string{"crystal_maiden", "nobody"} {
		if rec, _ := getHero(t, pool, "/api/heroes/"+slug, ""); rec.Code != http.StatusNotFound {
			t.Fatalf("%s: status %d, want 404", slug, rec.Code)
		}
	}
}
