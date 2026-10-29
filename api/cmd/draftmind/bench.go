package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"io/fs"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jholhewres/draftmind/api/internal/catalog"
	"github.com/jholhewres/draftmind/api/internal/config"
	"github.com/jholhewres/draftmind/api/internal/decision"
	"github.com/jholhewres/draftmind/api/internal/httpapi"
	"github.com/jholhewres/draftmind/api/internal/photo"
)

// fixture is one photo of the bench set and the draft it shows.
type fixture struct {
	name    string
	data    []byte
	img     image.Image
	expect  [photo.Slots]int // 0: an empty slot
	picked  int              // slots with a hero
	isSynth bool
}

// runBench runs `bench photo [dir]`: every reader live on every photo of
// the set (ADR-0006). This is the only place photo reads call the models.
func runBench(ctx context.Context, cfg config.Config, pool *pgxpool.Pool, log *slog.Logger) error {
	if len(os.Args) < 3 || os.Args[2] != "photo" {
		return fmt.Errorf("unknown bench; %s", usage)
	}
	if cfg.OpenRouterAPIKey == "" {
		return errors.New("bench photo calls the models: set OPENROUTER_API_KEY in .env")
	}
	dir := "testdata/photos"
	if len(os.Args) >= 4 {
		dir = os.Args[3]
	}
	set, skipped, err := loadFixtures(dir)
	if err != nil {
		return err
	}
	for _, name := range skipped {
		fmt.Printf("skipped %s: its photo is not here (real photos stay out of git)\n", name)
	}
	cat := catalog.NewStore(pool)
	_, heroes, err := cat.Heroes(ctx)
	if err != nil {
		return err
	}
	// Bench calls stay out of decision_log; BENCH_VERBOSE=1 prints each Jev call.
	var calls decision.Logger
	if os.Getenv("BENCH_VERBOSE") == "1" {
		calls = printCalls{}
	}
	readers := newPhotoReaders(cfg, calls)
	// Scored on the names it gives; as served, the fallback flags them all.
	readers.vision.TrustNames = true
	if err := readers.jev.Portraits.Load(ctx, heroes); err != nil {
		return err
	}
	fmt.Printf("bench photo: %d photos in %s (%d synthetic), %d heroes on the current patch\n",
		len(set), dir, countSynth(set), len(heroes))
	fmt.Printf("models: jev %s, vision %s; slot confidence floor %.2f\n\n", cfg.PhotoJevModel, cfg.VisionModel, photoMinConfidence)

	var inShort, shortTotal [2]int // by group: synthetic, real
	group := 0
	expectByShortlist := map[int]int{} // slot -> expected hero for the photo being read
	verbose := os.Getenv("BENCH_VERBOSE") == "1"
	readers.jev.Trace = func(slot int, clock, at image.Rectangle, fit float32, short []int) {
		if verbose {
			if slot == 0 {
				fmt.Printf("    clock %v\n", clock)
			}
			fmt.Printf("    slot %d at %v fit %.2f shortlist %v, want %d\n", slot, at, fit, short, expectByShortlist[slot])
		}
		if want := expectByShortlist[slot]; want != 0 {
			shortTotal[group]++
			if slices.Contains(short, want) {
				inShort[group]++
			}
		}
	}
	jev := benchReader(ctx, "jev", readers.jev, set, heroes, func(f fixture) {
		group = groupOf(f)
		for i, id := range f.expect {
			expectByShortlist[i] = id
		}
	})
	vision := benchReader(ctx, "vision", readers.vision, set, heroes, func(fixture) {})
	for g, label := range groupLabels {
		if jev[g].photos == 0 {
			continue
		}
		fmt.Printf("%s photos:\n\n", label)
		fmt.Println("| reader | slots right | wrong hero | flagged | errors | p50 | p95 | cost / photo |")
		fmt.Println("|---|---|---|---|---|---|---|---|")
		jev[g].print()
		vision[g].print()
		if shortTotal[g] > 0 {
			fmt.Printf("\njev shortlist: the right hero was among the 5 candidates in %d of %d picked slots (%.0f %%)\n",
				inShort[g], shortTotal[g], 100*float64(inShort[g])/float64(shortTotal[g]))
		}
		fmt.Println()
	}

	// Photo to first suggestion, through the API as the browser calls it.
	readers.jev.Trace = nil
	readers.vision.TrustNames = false
	srv := httpapi.New(pool, cat, log).
		WithDecider(decision.New(cfg.OpenRouterBaseURL, cfg.OpenRouterAPIKey, cfg.DecisionModel, cfg.DecisionProvider, nil)).
		WithPhotoReader(readers.chain(cfg.PhotoReader)).
		WithLimits(httpapi.Limit{Burst: 1000, Every: time.Millisecond}, httpapi.Limit{Burst: 1000, Every: time.Millisecond})
	api := httptest.NewServer(srv.Handler())
	defer api.Close()
	var total []time.Duration
	failed := 0
	for _, f := range set {
		d, err := photoToSuggestion(ctx, api.URL, f)
		if err != nil {
			failed++
			fmt.Printf("  %s: %v\n", f.name, err)
			continue
		}
		total = append(total, d)
	}
	fmt.Printf("\nphoto to first suggestion (POST /api/draft/read, then /api/draft/suggest; default reader %s): p50 %s, p95 %s, %d of %d failed\n",
		cfg.PhotoReader, pct(total, 50), pct(total, 95), failed, len(set))
	// The brief's target; a failed photo counts as missing it.
	if failed > 0 || len(total) == 0 || median(total) > photoTargetP50 {
		return fmt.Errorf("photo to first suggestion misses the %v p50 target", photoTargetP50)
	}
	fmt.Printf("target: p50 at most %v, met\n", photoTargetP50)
	return nil
}

type benchRow struct {
	name                         string
	slots, right, wrong, flagged int
	errors                       int
	latency                      []time.Duration
	cost                         float64
	costed, photos               int
}

func (r benchRow) print() {
	perPhoto := "n/a"
	if r.costed > 0 {
		perPhoto = fmt.Sprintf("$%.5f", r.cost/float64(r.costed))
	}
	acc := 0.0
	if r.slots > 0 {
		acc = 100 * float64(r.right) / float64(r.slots)
	}
	fmt.Printf("| %s | %d/%d (%.1f %%) | %d | %d | %d | %s | %s | %s |\n",
		r.name, r.right, r.slots, acc, r.wrong, r.flagged, r.errors, pct(r.latency, 50), pct(r.latency, 95), perPhoto)
}

// The bench reports the synthetic and the real photos apart.
var groupLabels = [2]string{"synthetic", "real"}

func groupOf(f fixture) int {
	if f.isSynth {
		return 0
	}
	return 1
}

// benchReader reads every photo of the set with rd: one row per group.
func benchReader(ctx context.Context, name string, rd photo.Reader, set []fixture, heroes []catalog.Hero, before func(fixture)) [2]benchRow {
	rows := [2]benchRow{{name: name}, {name: name}}
	for _, f := range set {
		before(f)
		row := &rows[groupOf(f)]
		rctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		start := time.Now()
		read, err := rd.Read(rctx, f.img, heroes)
		elapsed := time.Since(start)
		cancel()
		row.photos++
		row.slots += photo.Slots
		if err != nil {
			row.errors++
			fmt.Printf("  %s %s: %v\n", name, f.name, err)
			continue
		}
		row.latency = append(row.latency, elapsed)
		if read.Cost != nil {
			row.cost += *read.Cost
			row.costed++
		}
		var marks []string
		for i, s := range append(append([]photo.Slot{}, read.Radiant...), read.Dire...) {
			want := f.expect[i]
			switch {
			case s.Status == photo.StatusUnknown:
				row.flagged++
				marks = append(marks, "?")
			case s.Status == photo.StatusEmpty && want == 0, s.Status == photo.StatusHero && s.HeroID == want:
				row.right++
				marks = append(marks, ".")
			default:
				row.wrong++
				marks = append(marks, "x")
			}
		}
		fmt.Printf("  %s %s: %s %s in %s\n", name, f.name, strings.Join(marks[:photo.PerSide], ""),
			strings.Join(marks[photo.PerSide:], ""), elapsed.Round(time.Millisecond))
	}
	fmt.Println()
	return rows
}

// photoToSuggestion posts the photo, then the read draft, and times both.
func photoToSuggestion(ctx context.Context, base string, f fixture) (time.Duration, error) {
	start := time.Now()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	part, _ := mw.CreateFormFile("image", f.name)
	_, _ = part.Write(f.data)
	_ = mw.Close()
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, base+"/api/draft/read", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	var read struct {
		Radiant, Dire []photo.Slot
	}
	if err := doJSON(req, &read); err != nil {
		return 0, fmt.Errorf("read: %w", err)
	}
	draft := map[string]any{"allies": heroIDs(read.Radiant, 4), "enemies": heroIDs(read.Dire, 5), "bans": []int{}, "role": 2}
	b, _ := json.Marshal(draft)
	req, _ = http.NewRequestWithContext(ctx, http.MethodPost, base+"/api/draft/suggest", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	var sug struct {
		Suggestions []json.RawMessage
	}
	if err := doJSON(req, &sug); err != nil {
		return 0, fmt.Errorf("suggest: %w", err)
	}
	if len(sug.Suggestions) == 0 {
		return 0, errors.New("no suggestion")
	}
	return time.Since(start), nil
}

func doJSON(req *http.Request, out any) error {
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, raw)
	}
	return json.Unmarshal(raw, out)
}

func heroIDs(slots []photo.Slot, limit int) []int {
	ids := []int{}
	for _, s := range slots {
		if s.Status == photo.StatusHero && len(ids) < limit {
			ids = append(ids, s.HeroID)
		}
	}
	return ids
}

// realDir holds the real screenshots and photos under the set's dir: their
// expected drafts are committed, the images are not.
const realDir = "real"

// loadFixtures reads every <name>.json and the image of the same name in
// dir, then in dir/real, where it skips an expected draft whose image is
// missing and names it in skipped.
func loadFixtures(dir string) (set []fixture, skipped []string, err error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = root.Close() }()
	specs, err := fs.Glob(root.FS(), "*.json")
	if err != nil || len(specs) == 0 {
		return nil, nil, fmt.Errorf("no expected drafts (*.json) in %s", dir)
	}
	realSpecs, _ := fs.Glob(root.FS(), realDir+"/*.json")
	for _, spec := range append(specs, realSpecs...) {
		var e struct {
			Synthetic     bool
			Radiant, Dire []*int
		}
		raw, err := root.ReadFile(spec)
		if err == nil {
			err = json.Unmarshal(raw, &e)
		}
		if err != nil {
			return nil, nil, fmt.Errorf("%s: %w", spec, err)
		}
		if len(e.Radiant) != photo.PerSide || len(e.Dire) != photo.PerSide {
			return nil, nil, fmt.Errorf("%s: want radiant and dire of %d hero ids or null", spec, photo.PerSide)
		}
		f := fixture{name: strings.TrimSuffix(spec, ".json"), isSynth: e.Synthetic}
		for i, id := range append(e.Radiant, e.Dire...) {
			if id != nil {
				f.expect[i] = *id
				f.picked++
			}
		}
		for _, ext := range []string{".jpg", ".jpeg", ".png"} {
			if f.data, err = root.ReadFile(f.name + ext); err == nil {
				break
			}
		}
		if f.data == nil && strings.HasPrefix(spec, realDir+"/") {
			skipped = append(skipped, f.name)
			continue
		}
		if f.data == nil {
			return nil, nil, fmt.Errorf("%s: no .jpg, .jpeg or .png next to it", spec)
		}
		if f.img, _, err = image.Decode(bytes.NewReader(f.data)); err != nil {
			return nil, nil, fmt.Errorf("%s: %w", f.name, err)
		}
		set = append(set, f)
	}
	return set, skipped, nil
}

func countSynth(set []fixture) int {
	n := 0
	for _, f := range set {
		if f.isSynth {
			n++
		}
	}
	return n
}

// photoTargetP50 is the brief's photo-to-first-suggestion target.
const photoTargetP50 = 5 * time.Second

func median(d []time.Duration) time.Duration {
	s := slices.Clone(d)
	slices.Sort(s)
	return s[(len(s)+1)/2-1]
}

// pct is the nearest-rank percentile.
func pct(d []time.Duration, p int) string {
	if len(d) == 0 {
		return "n/a"
	}
	s := slices.Clone(d)
	slices.Sort(s)
	i := (p*len(s)+99)/100 - 1
	return s[max(0, i)].Round(10 * time.Millisecond).String()
}

// printCalls prints each Jev call of the bench, without its request.
type printCalls struct{}

func (printCalls) Record(_ context.Context, e decision.Entry) {
	fmt.Printf("    jev %s: %s in %s, %d attempts, hedged %t, %d tokens %s\n",
		e.Purpose, e.Outcome, e.Latency.Round(time.Millisecond), e.Attempts, e.Hedged, e.InputTokens, e.Error)
}
