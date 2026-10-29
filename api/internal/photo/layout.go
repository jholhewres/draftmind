package photo

import "slices"

// The pick-screen layouts a read is for (ADR-0012).
const (
	LayoutCalibrated   = "all_pick"     // the All Pick and Turbo top bar the readers were tuned on
	LayoutUncalibrated = "uncalibrated" // every slot is flagged for the player
)

// calibrated are the modes whose pick screen is the All Pick top bar.
var calibrated = []string{"", "all_pick_ranked", "all_pick", "turbo"}

// FlagUncalibrated keeps a read on a layout the readers were tuned on and,
// for any other mode, flags every slot so the player picks each hero: a
// reader never guesses on a screen it was not calibrated for.
func FlagUncalibrated(r Read, mode string) Read {
	if slices.Contains(calibrated, mode) {
		r.Layout = LayoutCalibrated
		return r
	}
	flag := func(in []Slot) []Slot {
		out := make([]Slot, len(in))
		for i := range in {
			out[i] = Slot{Status: StatusUnknown}
		}
		return out
	}
	r.Radiant, r.Dire, r.Layout = flag(r.Radiant), flag(r.Dire), LayoutUncalibrated
	return r
}
