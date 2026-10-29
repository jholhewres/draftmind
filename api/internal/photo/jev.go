package photo

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"math"
	"sync"

	"github.com/jholhewres/draftmind/api/internal/catalog"
	"github.com/jholhewres/draftmind/api/internal/decision"
)

// Asker asks Jev; *decision.Client is one.
type Asker interface {
	Ask(ctx context.Context, req decision.Request) (decision.Result, error)
}

// The top bar's shape in slot heights, from the clock panel between the
// teams. Calibrated on the synthetic set (api/testdata/photos/README.md);
// recheck it when real photos land.
const (
	slotAspect    = 118.0 / 66 // a portrait's width over its height
	slotGap       = 6.0 / 66   // between two portraits
	innerOffset   = 118.0 / 66 // from the clock's centre to the nearest portrait
	slotPerClockH = 66.0 / 64  // a portrait's height over the clock box's
	slotTopShift  = 4.0 / 64   // the portraits' top below the clock box's, in clock heights
	// A portrait that fits this well fixes the size and height of the next slot's guess.
	anchorScore = 0.8
	// The least fit Jev's pick must have in the code's search to stand.
	agreeScore = 0.75
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
// walk counts slots blindly past empty ones, so each portrait found is then
// numbered by its own distance from the clock in its own slot widths, and
// every slot without one is looked for a slot step from its neighbour.
func layout(photo rgb, clock image.Rectangle, ports map[int]*portrait) [Slots]found {
	cx := float64(clock.Min.X+clock.Max.X) / 2
	h := float64(clock.Dy()) * slotPerClockH
	top := float64(clock.Min.Y) + slotTopShift*float64(clock.Dy())
	walked := walk(photo, cx, h, top, ports)

	var out [Slots]found
	var have [Slots]bool
	for _, f := range walked {
		if f.Best < anchorScore {
			continue
		}
		dir := 1.0
		inner := float64(f.Rect.Min.X)
		if float64(f.Rect.Min.X+f.Rect.Max.X)/2 < cx {
			dir, inner = -1, float64(f.Rect.Max.X)
		}
		fh := float64(f.Rect.Dy())
		n := int(math.Round((dir*(inner-cx) - innerOffset*fh) / (fh * (slotAspect + slotGap))))
		if n < 0 || n >= PerSide {
			continue
		}
		if i := slotIndex(dir, n); !have[i] || f.Best > out[i].Best {
			out[i], have[i] = f, true
		}
	}
	for _, dir := range []float64{-1, 1} {
		fill(photo, &out, &have, dir, slotRect(cx, h, top, dir, 0), ports)
	}
	return out
}

// fill looks for each slot of side dir still without a portrait one slot
// step from a neighbour that has one, nearest first; a side with none starts
// from first, the slot next to the clock as the Box puts it.
func fill(photo rgb, out *[Slots]found, have *[Slots]bool, dir float64, first image.Rectangle, ports map[int]*portrait) {
	known := map[int]bool{}
	for k := range PerSide {
		if have[slotIndex(dir, k)] {
			known[k] = true
		}
	}
	if len(known) == 0 {
		f := search(photo, first, ports, walkScales, 0.45)
		if f.Best < anchorScore {
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
		step := float64(n.Dy()) * (slotAspect + slotGap)
		guess := n.Add(image.Pt(int(math.Round(dir*float64(k-from)*step)), 0))
		f := search(photo, guess, ports, []float64{0.9, 1, 1.1}, 0.3)
		if f.Best < anchorScore {
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

// slotRect is where the layout puts slot k of side dir.
func slotRect(cx, h, top, dir float64, k int) image.Rectangle {
	w := h * slotAspect
	inner := cx + dir*(innerOffset*h+float64(k)*(w+slotGap*h))
	x0 := inner
	if dir < 0 {
		x0 = inner - w
	}
	return image.Rect(int(x0), int(top), int(x0+w), int(top+h))
}

// walk is the first pass: from the clock outward, the size and height of
// each portrait found set the next guess.
func walk(photo rgb, cx, h0, top0 float64, ports map[int]*portrait) [Slots]found {
	var out [Slots]found
	scales := walkScales
	for _, dir := range []float64{-1, 1} {
		h, top := h0, top0
		inner := cx + dir*innerOffset*h
		for k := 0; k < PerSide; k++ {
			w := h * slotAspect
			x0 := inner
			if dir < 0 {
				x0 = inner - w
			}
			guess := image.Rect(int(x0), int(top), int(x0+w), int(top+h))
			f := search(photo, guess, ports, scales, 0.45)
			if f.Best >= anchorScore {
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
			out[slotIndex(dir, k)] = f
		}
	}
	return out
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
	switch a.Choice {
	case optionEmpty:
		return Guess{Empty: true, Confidence: p}, nil
	case optionOther:
		return Guess{Confidence: p}, nil // a hero, but not one we can name: flagged
	}
	// Two views must agree: Jev's pick must also be a portrait that fits the
	// slot by the code's measure, else the slot is flagged.
	for _, c := range f.Ranks {
		if ports[c.ID].hero.Name == a.Choice && c.Score < agreeScore {
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
