package photo

import (
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/jholhewres/draftmind/api/internal/catalog"
	"github.com/jholhewres/draftmind/api/internal/decision"
)

// noiseHeroes are n heroes whose portraits, served by the returned CDN, are
// distinct random pictures.
func noiseHeroes(t *testing.T, n int) ([]catalog.Hero, *httptest.Server) {
	t.Helper()
	heroes := make([]catalog.Hero, n)
	for i := range heroes {
		heroes[i] = catalog.Hero{ID: i + 1, Name: fmt.Sprintf("Hero %d", i+1), Img: fmt.Sprintf("/heroes/h%d.png", i+1)}
	}
	cdn := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var id uint64
		if _, err := fmt.Sscanf(r.URL.Path, "/heroes/h%d.png", &id); err != nil {
			http.NotFound(w, r)
			return
		}
		_ = png.Encode(w, noise(id))
	}))
	t.Cleanup(cdn.Close)
	return heroes, cdn
}

// pickScreen draws a 1568x882 photo of the synthetic set's bar: the clock
// panel at clock and each slot's hero portrait (0 leaves the slot empty).
func pickScreen(clock image.Rectangle, slots [Slots]int) *image.RGBA {
	return drawBar(clock, slots, false, nil)
}

// drawBar draws the top bar. The synthetic set's centres the slot nearest
// the clock 177 px from the clock's centre for 66 px tall slots, under a
// dim clock panel; the game's centres it 240 px from it, under the clock's
// big white digits. A greyed slot shows its hero hovered, not locked:
// desaturated and dim.
func drawBar(clock image.Rectangle, slots [Slots]int, game bool, greyed map[int]bool) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, 1568, 882))
	draw.Draw(img, img.Bounds(), image.NewUniform(color.RGBA{14, 16, 20, 255}), image.Point{}, draw.Src)
	near := 177.0 / 66
	if game {
		near = 240.0 / 66
		// "0:29": four white strokes across the middle of the box.
		dw, dh := clock.Dx()/9, clock.Dy()/2
		for k := range 4 {
			x := clock.Min.X + clock.Dx()/9 + 2*k*dw
			draw.Draw(img, image.Rect(x, clock.Min.Y+dh/2, x+dw/2, clock.Min.Y+dh/2+dh), image.NewUniform(color.White), image.Point{}, draw.Src)
		}
	} else {
		draw.Draw(img, clock, image.NewUniform(color.RGBA{30, 32, 38, 255}), image.Point{}, draw.Src)
	}
	h := float64(clock.Dy()) * slotPerClockH
	w, gap := h*slotAspect, h*slotGap
	top := float64(clock.Min.Y) + slotTopShift*float64(clock.Dy())
	cx := float64(clock.Min.X+clock.Max.X) / 2
	for i, id := range slots {
		// k counts slots outward from the clock.
		dir, k := 1.0, float64(i-PerSide)
		if i < PerSide {
			dir, k = -1, float64(PerSide-1-i)
		}
		mid := cx + dir*(near*h+k*(w+gap))
		r := image.Rect(int(mid-w/2), int(top), int(mid+w/2), int(top+h))
		if id == 0 {
			draw.Draw(img, r, image.NewUniform(color.RGBA{24, 27, 32, 255}), image.Point{}, draw.Src)
			continue
		}
		pic := toRGB(noise(uint64(id))).resize(r.Dx(), r.Dy())
		if greyed[i] {
			for p := 0; p < len(pic.px); p += 3 {
				// As the game greys a hover: a fifth of the colour and a
				// quarter of the contrast left.
				l := (pic.px[p] + pic.px[p+1] + pic.px[p+2]) / 3
				for c := p; c < p+3; c++ {
					pic.px[c] = 50 + 0.25*(l+0.2*(pic.px[c]-l))
				}
			}
		}
		draw.Draw(img, r, pic.image(), image.Point{}, draw.Src)
	}
	return img
}

// fakeJev answers the clock's Box with clock and each slot's choice with
// the hero placed there, keyed "radiant 1" to "dire 5", sure of it.
type fakeJev struct {
	t       *testing.T
	clock   decision.Edges
	placed  map[string]string
	mu      sync.Mutex
	options map[string][]string // each slot's options
}

func (f *fakeJev) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("TypeSafe-Danger-Zone-Images") != "true" {
		http.Error(w, "images are not turned on", http.StatusBadRequest)
		return
	}
	var req struct {
		State     map[string]any `json:"state"`
		Assets    map[string]any `json:"assets"`
		Questions map[string]struct {
			Type     string         `json:"type"`
			Criteria map[string]any `json:"criteria"`
		} `json:"questions"`
	}
	body, _ := io.ReadAll(r.Body)
	if err := json.Unmarshal(body, &req); err != nil {
		f.t.Errorf("request: %v", err)
	}
	if q, ok := req.Questions["clock"]; ok && q.Type == "box" {
		b, _ := json.Marshal(f.clock)
		_, _ = fmt.Fprintf(w, `{"model": "typesafe/jev-1.14-20261014", "answers": {"clock": {"type": "box", "box": %s, "confidence": 0.85}},
			"usage": {"input_tokens": 3931, "output_tokens": 36, "cost": 0.000165}}`, b)
		return
	}
	key := fmt.Sprintf("%v %v", req.State["team"], req.State["position"])
	var opts []string
	for name, v := range req.Questions["hero"].Criteria {
		opts = append(opts, name)
		if ref, ok := v.(string); ok && strings.HasPrefix(ref, "img-") && req.Assets[ref] == nil {
			f.t.Errorf("option %s refers to %s, which is not an asset", name, ref)
		}
	}
	f.mu.Lock()
	f.options[key] = opts
	f.mu.Unlock()
	answer := choose(f.placed[key], opts, 0.9)
	_, _ = fmt.Fprintf(w, `{"model": "typesafe/jev-1.14-20261014", "answers": {"hero": %s}, "usage": {"input_tokens": 2400, "output_tokens": 30, "cost": 0.0001}}`, answer)
}

// choose answers pick with probability p, the rest shared among the others.
func choose(pick string, opts []string, p float64) string {
	probs := map[string]float64{}
	for _, o := range opts {
		probs[o] = (1 - p) / float64(len(opts)-1)
	}
	probs[pick] = p
	b, _ := json.Marshal(probs)
	return fmt.Sprintf(`{"type": "choice", "choice": %q, "confidence": 0.7, "probabilities": %s}`, pick, b)
}

func newJev(t *testing.T, cdn string, jev http.Handler) *Jev {
	t.Helper()
	srv := httptest.NewServer(jev)
	t.Cleanup(srv.Close)
	c := decision.New(srv.URL, "test-key", "typesafe/jev-1.14", "typesafe", nil)
	c.HTTP = srv.Client()
	return &Jev{Ask: c, Portraits: NewPortraits(t.TempDir(), cdn, http.DefaultClient), Shortlist: 5, MinConfidence: 0.5}
}

func TestJevReadsEachSlotAmongItsShortlistAndFlagsWhatItCannotRead(t *testing.T) {
	heroes, cdn := noiseHeroes(t, 30)
	clock := image.Rect(720, 60, 848, 124)
	// Radiant: heroes 3, 7, 12, 20 and an empty slot; dire: 5, 9, 14, 25, 30.
	slots := [Slots]int{3, 7, 12, 20, 0, 5, 9, 14, 25, 30}
	placed := map[string]string{}
	for i, id := range slots {
		name := optionEmpty
		if id != 0 {
			name = heroes[id-1].Name
		}
		placed[fmt.Sprintf("%s %d", teamOf(i), i%PerSide+1)] = name
	}
	fake := &fakeJev{
		t: t, placed: placed, options: map[string][]string{},
		clock: decision.Edges{Left: 720.0 / 1568, Right: 848.0 / 1568, Top: 60.0 / 882, Bottom: 124.0 / 882},
	}
	j := newJev(t, cdn.URL, fake)
	// Jev is unsure of dire 3, names none of the options for dire 4 and, sure
	// of it, names for radiant 2 a hero whose portrait does not fit the slot.
	j.Ask = askOverride{j.Ask, map[string]func(opts []string) string{
		"dire 3": func(opts []string) string { return choose("Hero 14", opts, 0.3) },
		"dire 4": func(opts []string) string { return choose(optionOther, opts, 0.8) },
		"radiant 2": func(opts []string) string {
			for _, o := range opts {
				if o != "Hero 7" && o != optionEmpty && o != optionOther {
					return choose(o, opts, 0.9)
				}
			}
			return ""
		},
	}}
	r, err := j.Read(context.Background(), pickScreen(clock, slots), heroes)
	if err != nil {
		t.Fatal(err)
	}
	want := []Slot{
		{HeroID: 3, Status: StatusHero}, {Status: StatusUnknown}, {HeroID: 12, Status: StatusHero},
		{HeroID: 20, Status: StatusHero}, {Status: StatusEmpty},
		{HeroID: 5, Status: StatusHero}, {HeroID: 9, Status: StatusHero}, {Status: StatusUnknown},
		{Status: StatusUnknown}, {HeroID: 30, Status: StatusHero},
	}
	got := append(append([]Slot{}, r.Radiant...), r.Dire...)
	for i := range want {
		if got[i].HeroID != want[i].HeroID || got[i].Status != want[i].Status {
			t.Errorf("slot %d: %+v, want %+v", i, got[i], want[i])
		}
	}
	if len(fake.options) != Slots-3 {
		t.Errorf("%d slots asked of the fake, want %d", len(fake.options), Slots-3)
	}
	for key, opts := range fake.options {
		if len(opts) != 5+2 {
			t.Errorf("%s offered %d options: %v", key, len(opts), opts)
		}
	}
	if r.Reader != "jev" || r.Cost == nil || *r.Cost < 0.0011 {
		t.Fatalf("reader %q, cost %v", r.Reader, r.Cost)
	}
}

// askOverride answers the named slots itself, from the options asked.
type askOverride struct {
	next  Asker
	slots map[string]func(opts []string) string
}

func (a askOverride) Ask(ctx context.Context, req decision.Request) (decision.Result, error) {
	st, _ := req.State.(map[string]any)
	key := fmt.Sprintf("%v %v", st["team"], st["position"])
	answer, ok := a.slots[key]
	if !ok {
		return a.next.Ask(ctx, req)
	}
	var opts []string
	for name := range req.Questions["hero"].Criteria.(map[string]any) {
		opts = append(opts, name)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprintf(w, `{"model": "m", "answers": {"hero": %s}, "usage": {"cost": 0.0001}}`, answer(opts))
	}))
	defer srv.Close()
	c := decision.New(srv.URL, "k", "m", "typesafe", nil)
	return c.Ask(ctx, req)
}

func TestJevFailsWhenItCannotFindTheClock(t *testing.T) {
	heroes, cdn := noiseHeroes(t, 5)
	j := newJev(t, cdn.URL, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"error": "bad image"}`, http.StatusBadRequest)
	}))
	if _, err := j.Read(context.Background(), pickScreen(image.Rect(720, 60, 848, 124), [Slots]int{}), heroes); err == nil {
		t.Fatal("no error when the Box question failed")
	}
}

func TestJevFindsTheSlotsWhenTheClockBoxIsLoose(t *testing.T) {
	heroes, cdn := noiseHeroes(t, 30)
	clock := image.Rect(740, 70, 828, 114) // a small, far-away top bar
	slots := [Slots]int{3, 7, 12, 20, 21, 5, 9, 14, 25, 30}
	placed := map[string]string{}
	for i, id := range slots {
		placed[fmt.Sprintf("%s %d", teamOf(i), i%PerSide+1)] = heroes[id-1].Name
	}
	// The Box answer is a hundredth-rounded box, too tall and off to the right.
	loose := decision.Edges{Left: 0.48, Right: 0.54, Top: 0.07, Bottom: 0.14}
	fake := &fakeJev{t: t, placed: placed, options: map[string][]string{}, clock: loose}
	r, err := newJev(t, cdn.URL, fake).Read(context.Background(), pickScreen(clock, slots), heroes)
	if err != nil {
		t.Fatal(err)
	}
	for i, s := range append(append([]Slot{}, r.Radiant...), r.Dire...) {
		if s.HeroID != slots[i] {
			t.Errorf("slot %d: %+v, want hero %d", i, s, slots[i])
		}
	}
}

// placedNames keys each slot's hero name, or empty, as fakeJev does.
func placedNames(heroes []catalog.Hero, slots [Slots]int) map[string]string {
	placed := map[string]string{}
	for i, id := range slots {
		name := optionEmpty
		if id != 0 {
			name = heroes[id-1].Name
		}
		placed[fmt.Sprintf("%s %d", teamOf(i), i%PerSide+1)] = name
	}
	return placed
}

func TestJevNumbersTheSlotsOfTheGameScreenLeftToRight(t *testing.T) {
	heroes, cdn := noiseHeroes(t, 30)
	clock := image.Rect(736, 60, 832, 116)
	// As on the real pick screen (api/testdata/photos/real/pick-phase): the
	// outermost radiant slot and two dire slots still empty, and the slot
	// nearest the clock centred 240 px from it with 66 px tall slots.
	slots := [Slots]int{0, 7, 12, 20, 21, 5, 0, 14, 0, 30}
	fake := &fakeJev{
		t: t, placed: placedNames(heroes, slots), options: map[string][]string{},
		clock: gameBox,
	}
	r, err := newJev(t, cdn.URL, fake).Read(context.Background(), drawBar(clock, slots, true, nil), heroes)
	if err != nil {
		t.Fatal(err)
	}
	for i, s := range append(append([]Slot{}, r.Radiant...), r.Dire...) {
		want := StatusHero
		if slots[i] == 0 {
			want = StatusEmpty
		}
		if s.HeroID != slots[i] || s.Status != want {
			t.Errorf("slot %d: %+v, want hero %d (%s)", i, s, slots[i], want)
		}
	}
}

func TestJevNeverReadsAHoveredPortraitAsAHero(t *testing.T) {
	heroes, cdn := noiseHeroes(t, 30)
	clock := image.Rect(736, 60, 832, 116)
	slots := [Slots]int{0, 7, 12, 20, 21, 5, 0, 14, 0, 30}
	// Radiant 3 is hovered, not locked: Jev, sure of it, still names the hero.
	fake := &fakeJev{
		t: t, placed: placedNames(heroes, slots), options: map[string][]string{},
		clock: gameBox,
	}
	img := drawBar(clock, slots, true, map[int]bool{2: true})
	r, err := newJev(t, cdn.URL, fake).Read(context.Background(), img, heroes)
	if err != nil {
		t.Fatal(err)
	}
	if s := r.Radiant[2]; s.Status == StatusHero {
		t.Errorf("hovered radiant 3 read as hero %d", s.HeroID)
	}
	if s := r.Radiant[3]; s.Status != StatusHero || s.HeroID != 20 {
		t.Errorf("locked radiant 4: %+v, want hero 20", s)
	}
}

func TestJevNeverReadsALockedPortraitAsEmpty(t *testing.T) {
	heroes, cdn := noiseHeroes(t, 30)
	clock := image.Rect(736, 60, 832, 116)
	slots := [Slots]int{0, 7, 12, 20, 21, 5, 0, 14, 0, 30}
	fake := &fakeJev{
		t: t, placed: placedNames(heroes, slots), options: map[string][]string{},
		clock: gameBox,
	}
	j := newJev(t, cdn.URL, fake)
	// Jev, sure of it, calls dire 5's locked portrait an empty slot.
	j.Ask = askOverride{j.Ask, map[string]func(opts []string) string{
		"dire 5": func(opts []string) string { return choose(optionEmpty, opts, 0.96) },
	}}
	r, err := j.Read(context.Background(), drawBar(clock, slots, true, nil), heroes)
	if err != nil {
		t.Fatal(err)
	}
	if s := r.Dire[4]; s.Status != StatusUnknown {
		t.Errorf("locked dire 5 read as %+v, want it flagged", s)
	}
	if s := r.Dire[3]; s.Status != StatusEmpty {
		t.Errorf("empty dire 4 read as %+v", s)
	}
}

func TestJevNumbersTheSyntheticBarWhenTheClockBoxIsOffCentre(t *testing.T) {
	heroes, cdn := noiseHeroes(t, 30)
	clock := image.Rect(720, 60, 848, 124)
	// The two slots nearest the clock empty, as in synthetic-04.
	slots := [Slots]int{3, 7, 12, 0, 0, 5, 9, 14, 25, 0}
	// Jev's box sits 40 px left of the panel, as it did on synthetic-04.
	off := decision.Edges{Left: 680.0 / 1568, Right: 808.0 / 1568, Top: 60.0 / 882, Bottom: 124.0 / 882}
	fake := &fakeJev{t: t, placed: placedNames(heroes, slots), options: map[string][]string{}, clock: off}
	r, err := newJev(t, cdn.URL, fake).Read(context.Background(), pickScreen(clock, slots), heroes)
	if err != nil {
		t.Fatal(err)
	}
	for i, s := range append(append([]Slot{}, r.Radiant...), r.Dire...) {
		if s.HeroID != slots[i] {
			t.Errorf("slot %d: %+v, want hero %d", i, s, slots[i])
		}
	}
}

// gameBox is Jev's Box answer for the game's clock drawn at 736,60-832,116:
// 9 px right of it, as on the real pick-phase screenshot.
var gameBox = decision.Edges{Left: 745.0 / 1568, Right: 841.0 / 1568, Top: 60.0 / 882, Bottom: 116.0 / 882}
