package main

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestBenchPhotoLoadsEveryPhotoOfTheSetWithItsExpectedDraft(t *testing.T) {
	set, _, err := loadFixtures("../../testdata/photos")
	if err != nil {
		t.Fatal(err)
	}
	var synth []fixture
	for _, f := range set {
		if f.isSynth {
			synth = append(synth, f)
		}
	}
	set = synth
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

func TestBenchPhotoLoadsTheRealPhotosApartAndSkipsTheOnesNotHere(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"synthetic-01.jpg", "synthetic-01.json"} {
		b, err := os.ReadFile(filepath.Join("../../testdata/photos", name))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), b, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(dir, "real"), 0o750); err != nil {
		t.Fatal(err)
	}
	// The committed expected drafts of the real set; only one of their
	// images is here, as on a fresh checkout with one photo copied in.
	for _, name := range []string{"pick-phase.json", "ban-phase.json"} {
		b, err := os.ReadFile(filepath.Join("../../testdata/photos/real", name))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "real", name), b, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	f, err := os.Create(filepath.Join(dir, "real", "pick-phase.png"))
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, image.NewRGBA(image.Rect(0, 0, 32, 18))); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()

	set, skipped, err := loadFixtures(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(set) != 2 || groupOf(set[0]) != 0 || groupOf(set[1]) != 1 || set[1].name != "real/pick-phase" {
		t.Fatalf("set %v", set)
	}
	if len(skipped) != 1 || skipped[0] != "real/ban-phase" {
		t.Fatalf("skipped %v, want real/ban-phase", skipped)
	}
	// pick-phase: radiant an empty slot, Enigma, a hover, Faceless Void and
	// Rubick; dire Puck, empty, Crystal Maiden, empty, Beastmaster.
	if set[1].expect != [10]int{0, 33, 0, 41, 86, 13, 0, 5, 0, 38} {
		t.Fatalf("pick-phase expects %v", set[1].expect)
	}
}
