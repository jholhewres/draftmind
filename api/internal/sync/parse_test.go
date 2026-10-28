package sync

import (
	"os"
	"testing"
	"time"
)

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestParseHeroesReadsDotaconstantsFixture(t *testing.T) {
	heroes, err := ParseHeroes(readFixture(t, "heroes.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(heroes) != 3 {
		t.Fatalf("got %d heroes, want 3", len(heroes))
	}
	cm := heroes[2]
	if cm.ID != 5 || cm.Slug != "crystal_maiden" || cm.Name != "Crystal Maiden" ||
		cm.PrimaryAttr != "int" || cm.AttackType != "Ranged" || len(cm.Roles) != 3 ||
		cm.Img != "/apps/dota2/images/dota_react/heroes/crystal_maiden.png" {
		t.Fatalf("unexpected hero: %+v", cm)
	}
}

func TestParseHeroesRejectsHeroWithoutName(t *testing.T) {
	if _, err := ParseHeroes([]byte(`{"1": {"id": 1, "name": "npc_dota_hero_x"}}`)); err == nil {
		t.Fatal("want error for a hero without localized_name")
	}
}

func TestParsePatchesReadsFixture(t *testing.T) {
	patches, err := ParsePatches(readFixture(t, "patch.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(patches) != 2 || patches[1].Name != "7.41" ||
		!patches[1].ReleasedAt.Equal(time.Date(2026, 3, 24, 0, 50, 59, 580e6, time.UTC)) {
		t.Fatalf("unexpected patches: %+v", patches)
	}
}
