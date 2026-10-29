package photo

import "testing"

func TestAReadOnALayoutTheReaderIsNotCalibratedForFlagsEverySlot(t *testing.T) {
	read := Read{Radiant: []Slot{{HeroID: 1, Status: StatusHero, Confidence: 0.9}, {Status: StatusEmpty, Confidence: 0.9}},
		Dire: []Slot{{HeroID: 2, Status: StatusHero, Confidence: 0.9}}}
	for _, mode := range []string{"all_pick_ranked", "all_pick", "turbo", ""} {
		if got := FlagUncalibrated(read, mode); got.Layout != LayoutCalibrated || got.Radiant[0].Status != StatusHero {
			t.Errorf("%q: %+v, want the read kept on the calibrated layout", mode, got)
		}
	}
	for _, mode := range []string{"captains_mode", "captains_draft", "single_draft", "random_draft", "all_random"} {
		got := FlagUncalibrated(read, mode)
		if got.Layout != LayoutUncalibrated {
			t.Errorf("%q: layout %q, want uncalibrated", mode, got.Layout)
		}
		for _, s := range append(got.Radiant, got.Dire...) {
			if s.Status != StatusUnknown || s.HeroID != 0 {
				t.Errorf("%q: slot %+v, want flagged for the player", mode, s)
			}
		}
	}
	// The input is not changed.
	if read.Radiant[0].Status != StatusHero {
		t.Fatal("FlagUncalibrated changed its input")
	}
}
