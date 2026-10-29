package photo

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"math"
	"slices"
	"sync"

	"github.com/jholhewres/draftmind/api/internal/catalog"
	"github.com/jholhewres/draftmind/api/internal/decision"
)

// Asker asks Jev; *decision.Client is one.
type Asker interface {
	Ask(ctx context.Context, req decision.Request) (decision.Result, error)
}

// The top bar's shape in slot heights, from the clock panel between the
// teams, calibrated on the game's own screenshots and the synthetic set
// (api/testdata/photos/README.md). Both put a slot every 1.88 slot heights.
const (
	slotAspect = 118.0 / 66 // a portrait's width over its height
	slotGap    = 6.0 / 66   // between two portraits, so a slot step is aspect plus gap
	// From the clock's centre to the centre of the slot nearest it: 240 px
	// for 66 px slots on the game's 1920x1080 screen, 177 px on the synthetic
	// set's invented bar. The two are half a slot step apart, so the slots
	// alone cannot tell them: the clock does (gameClock).
	nearGame      = 240.0 / 66
	nearSynthetic = 177.0 / 66
	slotPerClockH = 66.0 / 64 // a portrait's height over the clock box's
	slotTopShift  = 4.0 / 64  // the portraits' top below the clock box's, in clock heights
	// A portrait that fits this well fixes the size and height of the next slot's guess.
	anchorScore = 0.8
	// The least fit Jev's pick must have in the code's search to stand.
	agreeScore = 0.75
	// The least share of its portrait's colour a locked hero's slot shows:
	// the game greys a hovered portrait to a tenth or a fifth of it, and the
	// synthetic photos keep more than half.
	lockedColour = 0.3
	// The longest side of the photo the clock is looked for in.
	locateMaxSide = 1024
)

// Slot crop options besides the shortlisted heroes.
const (
	optionEmpty = "empty"
	optionOther = "other"
)

// Jev reads the draft with Jev images (ADR-0006): a Box question finds the
// clock between the teams, the code finds each portrait around the slot the
// layout puts it in and shortlists the heroes whose portraits fit best, and
// one choice question per slot picks among their portraits.
type Jev struct {
	Ask       Asker
	Portraits *Portraits
	// Shortlist is how many heroes each slot's choice offers.
	Shortlist int
	// MinConfidence is the least probability a slot's answer needs to stand.
	MinConfidence float64
	// Trace, when set, receives the clock found and each slot's place, fit
	// and shortlist, for the bench.
	Trace func(slot int, clock, at image.Rectangle, fit float32, shortlist []int)
}

// Name names the reader in reads and logs.
func (j *Jev) Name() string { return "jev" }

// Read reads img's draft.
func (j *Jev) Read(ctx context.Context, img image.Image, heroes []catalog.Hero) (Read, error) {
	ports, err := j.Portraits.byID(ctx, heroes)
	if err != nil {
		return Read{}, err
	}
	photo := toRGB(img)
	var cost costSum
	clock, err := j.locate(ctx, photo, &cost)
	if err != nil {
		return Read{}, err
	}
	slots := layout(photo, clock, ports)
	var g [Slots]Guess
	var wg sync.WaitGroup
	errs := make([]error, Slots)
	for i, f := range slots {
		short := shortlist(f, j.Shortlist)
		if j.Trace != nil {
			j.Trace(i, clock, f.Rect, f.Best, short)
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			g[i], errs[i] = j.slot(ctx, photo, i, f, short, ports, &cost)
		}()
	}
	wg.Wait()
	failed := 0
	for _, e := range errs {
		if e != nil {
			failed++
		}
	}
	if failed == Slots {
		return Read{}, fmt.Errorf("photo: jev: every slot failed: %w", errors.Join(errs...))
	}
	r := Resolve(g, heroes, j.MinConfidence)
	r.Reader, r.Cost = j.Name(), cost.total()
	return r, nil
}

// locate asks where the clock panel between the teams is, in pixels.
func (j *Jev) locate(ctx context.Context, photo rgb, cost *costSum) (image.Rectangle, error) {
	// The Box answer is in hundredths of the image: a smaller copy finds the
	// clock as well and uploads faster.
	scaled := photo.fit(locateMaxSide)
	ref := newRef("photo")
	res, err := j.Ask.Ask(ctx, decision.Request{
		Purpose: "photo.locate",
		State:   map[string]any{"photo": ref},
		Assets:  map[string]decision.Asset{ref: {Format: "jpg", Data: encodeJPEG(scaled, qualitySmall)}},
		Questions: map[string]decision.Question{
			"clock": decision.Box("the small dark clock panel at the top centre of the game screen in `photo`, " +
				"between the two rows of hero portraits"),
		},
	})
	cost.add(res.Usage.Cost)
	if err != nil {
		return image.Rectangle{}, fmt.Errorf("photo: jev: locate: %w", err)
	}
	e := res.Answers["clock"].Box
	// The answer is in fractions, so it holds for the full-size photo too.
	return image.Rect(int(e.Left*float64(photo.w)), int(e.Top*float64(photo.h)),
		int(e.Right*float64(photo.w)), int(e.Bottom*float64(photo.h))), nil
}

// layout finds the ten portraits, radiant then dire, left to right. A walk
// outward from the clock finds the portraits it can: each one found sets
// the size and height of the next guess, which follows the bar's tilt. The
// walk counts slots blindly past empty ones, so the portraits found then
// place slot 0 of each side (slotZero) and are numbered by their distance
// from it in slot steps; every slot without one is looked for a slot step
// from its neighbour.
func layout(photo rgb, clock image.Rectangle, ports map[int]*portrait) [Slots]found {
	cx := float64(clock.Min.X+clock.Max.X) / 2
	h := float64(clock.Dy()) * slotPerClockH
	top := float64(clock.Min.Y) + slotTopShift*float64(clock.Dy())
	near := nearSynthetic
	if gameClock(photo.sub(clock)) {
		near = nearGame
	}
	walked := walk(photo, cx, h, top, near, ports)

	// The portraits found, radiant's then dire's.
	var anchors [2][]found
	for _, f := range walked {
		if !anchored(photo, f, ports) {
			continue
		}
		side := 1
		if mid(f.Rect) < cx {
			side = 0
		}
		anchors[side] = append(anchors[side], f)
	}
	anchors[0], anchors[1] = apart(anchors[0]), apart(anchors[1])
	var steps [2]float64
	for side := range 2 {
		steps[side] = sideStep(anchors[side])
	}
	for side := range 2 {
		if steps[side] == 0 {
			steps[side] = steps[1-side] // a screen keeps one scale across the bar
		}
		if steps[side] == 0 && len(anchors[side]) > 0 {
			steps[side] = float64(anchors[side][0].Rect.Dy()) * (slotAspect + slotGap)
		}
	}
	x0 := slotZero(anchors, steps, cx, near)
	var out [Slots]found
	var have [Slots]bool
	for side, dir := range sides {
		for _, f := range anchors[side] {
			n := int(math.Round(dir * (mid(f.Rect) - x0[side]) / steps[side]))
			if n < 0 || n >= PerSide {
				continue
			}
			if i := slotIndex(dir, n); !have[i] || f.Best > out[i].Best {
				out[i], have[i] = f, true
			}
		}
	}
	for side, dir := range sides {
		fill(photo, &out, &have, dir, slotRect(cx, h, top, near, dir, 0), steps[side], ports)
	}
	return out
}

// The game's clock is big white digits on black; the synthetic set's is a
// dim panel with small grey ones. Measured on the photo sets, in the middle
// three fifths of the box: at least 7 % of it white digits on the game's
// screen, at most 0.1 % on synthetic ones, also with the box 40 px astray.
const (
	digitAbove  = 100  // how much brighter than the box's median a digit's pixel is
	digitChroma = 40   // how far a white digit's channels spread at most
	gameDigits  = 0.02 // the least share of the box in digits on the game's screen
)

// gameClock tells whether box, the clock the Box answer found, is the
// game's own: enough of its middle white digits.
func gameClock(box rgb) bool {
	box = box.sub(image.Rect(box.w/5, 0, box.w-box.w/5, box.h))
	n := box.w * box.h
	if n == 0 {
		return false
	}
	var hist [256]int
	for i := range n {
		hist[int(box.px[3*i]+box.px[3*i+1]+box.px[3*i+2])/3]++
	}
	median, seen := 0, 0
	for ; median < 255 && seen+hist[median] <= n/2; median++ {
		seen += hist[median]
	}
	digits := 0
	for i := range n {
		r, g, b := box.px[3*i], box.px[3*i+1], box.px[3*i+2]
		if int(r+g+b)/3 > median+digitAbove && max(r, g, b)-min(r, g, b) < digitChroma {
			digits++
		}
	}
	return float64(digits) >= gameDigits*float64(n)
}

// anchored tells whether f is a portrait found for sure: it fits well and
// shows its hero's colours, where an empty slot or a greyed hover can fit a
// dull portrait's shapes.
func anchored(photo rgb, f found, ports map[int]*portrait) bool {
	return len(f.Ranks) > 0 && f.Best >= anchorScore &&
		colourSpread(photo.sub(inset(f.Rect, 10))) >= lockedColour*ports[f.Ranks[0].ID].colour
}

// sides are the directions from the clock to radiant's slots and dire's.
var sides = [2]float64{-1, 1}

// How far, in slot steps, the bar's centre may sit from the Box answer's
// (up to 0.43 on the synthetic set) and slot 0 from the geometry's near
// distance. A portrait found off the bar costs as much as a centre off by
// twice its slack: one stray fit cannot move the bar alone.
const (
	centreSlack = 0.25
	nearSlack   = 0.1
	offBar      = 4
)

// slotZero is where the centre of each side's slot nearest the clock is,
// steps the side's slot steps. A side alone puts it the geometry's near
// distance from the clock's box. With portraits found on both sides the bar's
// symmetry places it better than the box can: of the numberings the
// portraits allow, the one whose bar is centred nearest the box and whose
// slot 0 sits nearest that distance.
func slotZero(anchors [2][]found, steps [2]float64, cx, near float64) [2]float64 {
	var x0 [2]float64
	for side, dir := range sides {
		x0[side] = cx + dir*near*steps[side]/(slotAspect+slotGap)
	}
	var cands [2][]float64
	for side, dir := range sides {
		if len(anchors[side]) == 0 {
			return x0
		}
		ref := anchors[side][0]
		for _, f := range anchors[side] {
			if math.Abs(mid(f.Rect)-cx) < math.Abs(mid(ref.Rect)-cx) {
				ref = f
			}
		}
		for k := range PerSide {
			cands[side] = append(cands[side], mid(ref.Rect)-dir*float64(k)*steps[side])
		}
	}
	step := (steps[0] + steps[1]) / 2
	best := math.Inf(1)
	out := x0
	for _, r := range cands[0] {
		for _, d := range cands[1] {
			off := ((d-r)/2 - near*step/(slotAspect+slotGap)) / (nearSlack * step)
			drift := ((r+d)/2 - cx) / (centreSlack * step)
			c := off*off + drift*drift + offBar*float64(outside(anchors[0], r, -1, steps[0])+outside(anchors[1], d, 1, steps[1]))
			if c < best {
				best, out = c, [2]float64{r, d}
			}
		}
	}
	return out
}

// outside counts the portraits found on side dir that land on no slot of
// it, its slot 0 centred at x0.
func outside(fs []found, x0, dir, step float64) int {
	out := 0
	for _, f := range fs {
		if n := math.Round(dir * (mid(f.Rect) - x0) / step); n < 0 || n >= PerSide {
			out++
		}
	}
	return out
}

// sideStep is the slot step on one side of the bar from its portraits
// found: the median over each pair of their distance over the whole number
// of steps it spans. A portrait's own height misjudges it on the game's
// screen, whose portraits are cut narrower than the image CDN's. 0 with
// fewer than two.
func sideStep(fs []found) float64 {
	var steps []float64
	for i, a := range fs {
		for _, b := range fs[i+1:] {
			dx := math.Abs(mid(b.Rect) - mid(a.Rect))
			guess := float64(a.Rect.Dy()+b.Rect.Dy()) / 2 * (slotAspect + slotGap)
			if n := math.Round(dx / guess); n >= 1 {
				steps = append(steps, dx/n)
			}
		}
	}
	if len(steps) == 0 {
		return 0
	}
	slices.Sort(steps)
	return steps[len(steps)/2]
}

// apart drops each portrait found over half of a better one: the walk can
// find one portrait twice, or a stray fit astride two.
func apart(fs []found) []found {
	var out []found
	for _, f := range fs {
		keep := true
		for _, g := range fs {
			ov := f.Rect.Intersect(g.Rect).Dx()
			if g.Best > f.Best && 2*ov > f.Rect.Dx() {
				keep = false
				break
			}
		}
		if keep {
			out = append(out, f)
		}
	}
	return out
}

func mid(r image.Rectangle) float64 { return float64(r.Min.X+r.Max.X) / 2 }

// fill looks for each slot of side dir still without a portrait one slot
// step from a neighbour that has one, nearest first; a side with none starts
// from first, the slot next to the clock as the Box puts it. step is the
// side's slot step, 0 when unknown.
func fill(photo rgb, out *[Slots]found, have *[Slots]bool, dir float64, first image.Rectangle, step float64, ports map[int]*portrait) {
	known := map[int]bool{}
	for k := range PerSide {
		if have[slotIndex(dir, k)] {
			known[k] = true
		}
	}
	if len(known) == 0 {
		f := search(photo, first, ports, walkScales, 0.45)
		if !anchored(photo, f, ports) {
			f.Rect = first
		}
		out[slotIndex(dir, 0)], known[0] = f, true
	}
	for {
		// The unknown slot next to a known one.
		k, from := -1, -1
		for c := range PerSide {
			if known[c] {
				continue
			}
			if c > 0 && known[c-1] {
				k, from = c, c-1
				break
			}
			if c < PerSide-1 && known[c+1] {
				k, from = c, c+1
				break
			}
		}
		if k < 0 {
			return
		}
		n := out[slotIndex(dir, from)].Rect
		st := step
		if st == 0 {
			st = float64(n.Dy()) * (slotAspect + slotGap)
		}
		guess := n.Add(image.Pt(int(math.Round(dir*float64(k-from)*st)), 0))
		f := search(photo, guess, ports, []float64{0.9, 1, 1.1}, 0.3)
		if !anchored(photo, f, ports) {
			f.Rect = guess
		}
		out[slotIndex(dir, k)], known[k] = f, true
	}
}

func slotIndex(dir float64, k int) int {
	if dir < 0 {
		return PerSide - 1 - k
	}
	return PerSide + k
}

// walkScales are the sizes, relative to the guess, a portrait is looked for
// at while the slot size is still the Box's rough one.
var walkScales = []float64{0.65, 0.75, 0.85, 0.95, 1.05, 1.2, 1.35}

// slotRect is where the layout puts slot k of side dir, the slot nearest
// the clock centred near slot heights from it.
func slotRect(cx, h, top, near, dir float64, k int) image.Rectangle {
	w := h * slotAspect
	mid := cx + dir*(near*h+float64(k)*(w+slotGap*h))
	return image.Rect(int(mid-w/2), int(top), int(mid+w/2), int(top+h))
}

// walk is the first pass: from the clock outward, the size and height of
// each portrait found set the next guess. A side where it finds none, its
// guesses having drifted off the slots, is walked again with a wider search.
func walk(photo rgb, cx, h0, top0, near float64, ports map[int]*portrait) [Slots]found {
	var out [Slots]found
	for _, dir := range sides {
		side, ok := walkSide(photo, cx, h0, top0, near, dir, 0.45, ports)
		if !ok {
			side, _ = walkSide(photo, cx, h0, top0, near, dir, 0.7, ports)
		}
		for k, f := range side {
			out[slotIndex(dir, k)] = f
		}
	}
	return out
}

// walkSide walks side dir, its slots by their distance from the clock,
// looking slack slot sizes around each guess; ok when it found a portrait.
func walkSide(photo rgb, cx, h, top, near, dir, slack float64, ports map[int]*portrait) (side [PerSide]found, ok bool) {
	inner := cx + dir*(near-slotAspect/2)*h
	for k := range PerSide {
		w := h * slotAspect
		x0 := inner
		if dir < 0 {
			x0 = inner - w
		}
		guess := image.Rect(int(x0), int(top), int(x0+w), int(top+h))
		f := search(photo, guess, ports, walkScales, slack)
		if anchored(photo, f, ports) {
			ok = true
			h, top = float64(f.Rect.Dy()), float64(f.Rect.Min.Y)
			inner = float64(f.Rect.Max.X)
			if dir < 0 {
				inner = float64(f.Rect.Min.X)
			}
		} else {
			f.Rect = guess
			inner += dir * w
		}
		inner += dir * slotGap * h
		side[k] = f
	}
	return side, ok
}

func shortlist(f found, n int) []int {
	out := make([]int, 0, n)
	for _, c := range f.Ranks[:min(n, len(f.Ranks))] {
		out = append(out, c.ID)
	}
	return out
}

const slotInstructions = "Which option is the hero portrait in `slot`? The slot is cut from a phone photo of a " +
	"monitor, so it may be blurred, tilted, washed out by glare or striped by moire. Choose empty when the slot " +
	"has no hero picture, and other when it shows a hero that is none of the pictured options."

// slot asks which of the shortlisted heroes the portrait at r shows.
func (j *Jev) slot(ctx context.Context, photo rgb, i int, f found, short []int, ports map[int]*portrait, cost *costSum) (Guess, error) {
	r := f.Rect
	// A little margin, so a portrait the search placed a bit off is whole.
	padX, padY := r.Dx()/10, r.Dy()/10
	crop := photo.sub(image.Rect(r.Min.X-padX, r.Min.Y-padY, r.Max.X+padX, r.Max.Y+padY))
	if crop.w < 2 || crop.h < 2 {
		return Guess{}, errors.New("slot outside the photo")
	}
	slotRef := newRef("slot")
	assets := map[string]decision.Asset{slotRef: {Format: "jpg", Data: encodeJPEG(crop.forJev(), qualitySmall)}}
	options := map[string]any{
		optionEmpty: "An empty slot: a dark panel with no hero picture.",
		optionOther: "A hero portrait that matches none of the pictured options.",
	}
	for _, id := range short {
		ref := newRef(fmt.Sprintf("hero%d", id))
		assets[ref] = decision.Asset{Format: "jpg", Data: ports[id].jpeg}
		options[ports[id].hero.Name] = ref
	}
	res, err := j.Ask.Ask(ctx, decision.Request{
		Purpose:   "photo.slot",
		State:     map[string]any{"slot": slotRef, "team": teamOf(i), "position": i%PerSide + 1},
		Assets:    assets,
		Questions: map[string]decision.Question{"hero": decision.Choice(slotInstructions, options)},
	})
	cost.add(res.Usage.Cost)
	if err != nil {
		return Guess{}, fmt.Errorf("photo: jev: slot: %w", err)
	}
	a := res.Answers["hero"]
	p := a.Probabilities[a.Choice]
	colour := colourSpread(photo.sub(inset(r, 10)))
	switch a.Choice {
	case optionEmpty:
		// A portrait in full colour that fits the slot is no empty slot.
		if len(f.Ranks) > 0 && f.Best >= anchorScore && colour >= lockedColour*ports[f.Ranks[0].ID].colour {
			return Guess{Confidence: p}, nil
		}
		return Guess{Empty: true, Confidence: p}, nil
	case optionOther:
		return Guess{Confidence: p}, nil // a hero, but not one we can name: flagged
	}
	// Two views must agree: Jev's pick must also be a portrait that fits the
	// slot by the code's measure, in its colours, else the slot is flagged.
	// A portrait greyed out is hovered, not locked: never a hero.
	for _, c := range f.Ranks {
		if ports[c.ID].hero.Name != a.Choice {
			continue
		}
		if c.Score < agreeScore || colour < lockedColour*ports[c.ID].colour {
			return Guess{Confidence: p}, nil
		}
	}
	return Guess{Name: a.Choice, Confidence: p}, nil
}

func teamOf(i int) string {
	if i < PerSide {
		return "radiant"
	}
	return "dire"
}

// newRef is an asset reference no instruction text can contain by chance.
func newRef(label string) string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return "img-" + hex.EncodeToString(b) + "-" + label
}

// costSum adds the costs of concurrent calls; nil until one reports a cost.
type costSum struct {
	mu  sync.Mutex
	sum *float64
}

func (c *costSum) add(v *float64) {
	if v == nil || math.IsNaN(*v) {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.sum == nil {
		c.sum = new(float64)
	}
	*c.sum += *v
}

func (c *costSum) total() *float64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.sum
}
