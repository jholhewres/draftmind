package photo

import (
	"context"
	"errors"
	"image"
	"testing"
	"time"

	"github.com/jholhewres/draftmind/api/internal/catalog"
)

var testHeroes = []catalog.Hero{
	{ID: 1, Name: "Anti-Mage", Img: "/heroes/antimage.png"},
	{ID: 2, Name: "Axe", Img: "/heroes/axe.png"},
	{ID: 53, Name: "Nature's Prophet", Img: "/heroes/furion.png"},
	{ID: 8, Name: "Juggernaut", Img: "/heroes/juggernaut.png"},
}

func guesses(radiant, dire []Guess) [Slots]Guess {
	var g [Slots]Guess
	copy(g[:], radiant)
	copy(g[PerSide:], dire)
	return g
}

func TestResolveFlagsUnknownAndLowConfidenceSlotsAndNeverGuessesThem(t *testing.T) {
	g := guesses(
		[]Guess{
			{Name: "Anti-Mage", Confidence: 0.9},       // read
			{Name: "natures prophet", Confidence: 0.8}, // read, spelt loosely
			{Name: "Axe", Confidence: 0.3},             // too unsure: flagged
			{Name: "Pudge", Confidence: 0.95},          // not on the current patch: flagged
			{Empty: true, Confidence: 0.9},             // nobody picked yet
		},
		[]Guess{
			{Name: "Juggernaut", Confidence: 0.9},
			{Empty: true, Confidence: 0.2}, // an unsure empty slot is flagged too
			{Name: "", Confidence: 0.9},    // no answer at all
		},
	)
	r := Resolve(g, testHeroes, 0.5)
	want := []Slot{
		{HeroID: 1, Status: StatusHero, Confidence: 0.9},
		{HeroID: 53, Status: StatusHero, Confidence: 0.8},
		{Status: StatusUnknown, Confidence: 0.3},
		{Status: StatusUnknown, Confidence: 0.95},
		{Status: StatusEmpty, Confidence: 0.9},
		{HeroID: 8, Status: StatusHero, Confidence: 0.9},
		{Status: StatusUnknown, Confidence: 0.2},
		{Status: StatusUnknown, Confidence: 0.9},
		{Status: StatusUnknown},
		{Status: StatusUnknown},
	}
	got := append(append([]Slot{}, r.Radiant...), r.Dire...)
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("slot %d: %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestResolveFlagsAHeroReadTwice(t *testing.T) {
	g := guesses([]Guess{{Name: "Axe", Confidence: 0.9}}, []Guess{{Name: "Axe", Confidence: 0.8}})
	r := Resolve(g, testHeroes, 0.5)
	if r.Radiant[0].Status != StatusUnknown || r.Dire[0].Status != StatusUnknown || r.Radiant[0].HeroID != 0 {
		t.Fatalf("a hero read in two slots was kept: %+v %+v", r.Radiant[0], r.Dire[0])
	}
}

// stubReader answers after delay with read, or fails with err.
type stubReader struct {
	name  string
	delay time.Duration
	read  Read
	err   error
	calls int
}

func (s *stubReader) Name() string { return s.name }

func (s *stubReader) Read(ctx context.Context, _ image.Image, _ []catalog.Hero) (Read, error) {
	s.calls++
	select {
	case <-time.After(s.delay):
	case <-ctx.Done():
		return Read{}, ctx.Err()
	}
	if s.err != nil {
		return Read{}, s.err
	}
	r := s.read
	r.Reader = s.name
	return r, nil
}

func oneHero(id int) Read {
	r := Resolve(guesses(nil, nil), testHeroes, 0.5)
	r.Radiant[0] = Slot{HeroID: id, Status: StatusHero, Confidence: 0.9}
	return r
}

func TestChainFallsBackToTheOtherReaderOnAnError(t *testing.T) {
	first := &stubReader{name: "jev", err: errors.New("HTTP 500")}
	second := &stubReader{name: "vision", read: oneHero(2)}
	c := Chain{Primary: first, Secondary: second, PrimaryDeadline: time.Second, SecondaryDeadline: time.Second}
	r, err := c.Read(context.Background(), nil, testHeroes)
	if err != nil {
		t.Fatal(err)
	}
	if r.Reader != "vision" || r.Fallback != FallbackError || r.Radiant[0].HeroID != 2 {
		t.Fatalf("read %+v", r)
	}
}

func TestChainFallsBackToTheOtherReaderAtTheDeadline(t *testing.T) {
	first := &stubReader{name: "jev", delay: time.Second, read: oneHero(1)}
	second := &stubReader{name: "vision", read: oneHero(2)}
	c := Chain{Primary: first, Secondary: second, PrimaryDeadline: 30 * time.Millisecond, SecondaryDeadline: time.Second}
	start := time.Now()
	r, err := c.Read(context.Background(), nil, testHeroes)
	if err != nil {
		t.Fatal(err)
	}
	if r.Reader != "vision" || r.Fallback != FallbackTimeout || r.Radiant[0].HeroID != 2 {
		t.Fatalf("read %+v", r)
	}
	if time.Since(start) > 500*time.Millisecond {
		t.Fatalf("waited %v for the late reader", time.Since(start))
	}
}

func TestChainKeepsThePrimaryReadWhenItAnswers(t *testing.T) {
	first := &stubReader{name: "jev", read: oneHero(1)}
	second := &stubReader{name: "vision", read: oneHero(2)}
	c := Chain{Primary: first, Secondary: second, PrimaryDeadline: time.Second, SecondaryDeadline: time.Second}
	r, err := c.Read(context.Background(), nil, testHeroes)
	if err != nil {
		t.Fatal(err)
	}
	if r.Reader != "jev" || r.Fallback != "" || second.calls != 0 {
		t.Fatalf("read %+v, secondary calls %d", r, second.calls)
	}
}

func TestChainFailsWhenBothReadersFail(t *testing.T) {
	c := Chain{
		Primary:         &stubReader{name: "jev", err: errors.New("down")},
		Secondary:       &stubReader{name: "vision", err: errors.New("also down")},
		PrimaryDeadline: time.Second, SecondaryDeadline: time.Second,
	}
	if _, err := c.Read(context.Background(), nil, testHeroes); err == nil {
		t.Fatal("no error when both readers failed")
	}
}
