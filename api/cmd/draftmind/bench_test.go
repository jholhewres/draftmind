package main

import (
	"testing"
	"time"
)

func TestBenchPhotoLoadsEveryPhotoOfTheSetWithItsExpectedDraft(t *testing.T) {
	set, err := loadFixtures("../../testdata/photos")
	if err != nil {
		t.Fatal(err)
	}
	if len(set) < 6 || len(set) > 10 {
		t.Fatalf("%d photos, want 6 to 10", len(set))
	}
	for _, f := range set {
		if !f.isSynth || f.img == nil || f.picked == 0 {
			t.Errorf("%s: synthetic %t, image %t, %d picked slots", f.name, f.isSynth, f.img != nil, f.picked)
		}
	}
	// synthetic-02: radiant Techies, Anti-Mage, Puck, Sand King and an empty slot.
	if set[1].name != "synthetic-02" || set[1].expect[0] != 105 || set[1].expect[1] != 1 || set[1].expect[4] != 0 {
		t.Fatalf("%s expects %v", set[1].name, set[1].expect)
	}
}

func TestBenchPercentilesAreNearestRank(t *testing.T) {
	d := []time.Duration{4 * time.Second, 1 * time.Second, 3 * time.Second, 2 * time.Second}
	if pct(d, 50) != "2s" || pct(d, 95) != "4s" || pct(nil, 50) != "n/a" {
		t.Fatalf("p50 %s, p95 %s", pct(d, 50), pct(d, 95))
	}
}
