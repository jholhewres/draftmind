// Package photo reads the draft from a photo of the pick screen (ADR-0006):
// two Readers, Jev images and a vision model, one the fallback of the
// other. Every slot resolves to a hero of the current patch or is flagged;
// a reader never guesses. Images are read in memory and never stored.
package photo

import (
	"context"
	"errors"
	"fmt"
	"image"
	"strings"
	"time"
	"unicode"

	"github.com/jholhewres/draftmind/api/internal/catalog"
)

// The pick screen's top bar: five radiant slots, then five dire.
const (
	PerSide = 5
	Slots   = 2 * PerSide
)

// What a slot holds once resolved.
const (
	StatusHero    = "hero"    // a hero of the current patch, read with confidence
	StatusEmpty   = "empty"   // nobody has picked yet
	StatusUnknown = "unknown" // unread, unsure or not a current hero: the player picks it
)

// Why the default reader's read was not used.
const (
	FallbackError   = "error"
	FallbackTimeout = "timeout"
)

// Slot is one resolved slot; HeroID is set only for StatusHero.
type Slot struct {
	HeroID     int     `json:"heroId,omitempty"`
	Status     string  `json:"status"`
	Confidence float64 `json:"confidence"`
}

// Read is a photo's draft, the reader that read it and, when the default
// reader failed, why.
type Read struct {
	Radiant  []Slot `json:"radiant"`
	Dire     []Slot `json:"dire"`
	Reader   string `json:"reader"`
	Fallback string `json:"fallback,omitempty"`
	// Cost in USD as the providers report it, for the bench; nil when none did.
	Cost *float64 `json:"-"`
}

// Guess is what a reader made of one slot, before it is resolved.
type Guess struct {
	Name       string // a hero name, as the reader gave it
	Empty      bool
	Confidence float64
}

// Reader is ADR-0006's PhotoReader: it reads the draft in img; heroes are
// the current patch's.
type Reader interface {
	Name() string
	Read(ctx context.Context, img image.Image, heroes []catalog.Hero) (Read, error)
}

// Resolve turns a reader's guesses into slots: a name becomes a hero of
// the current patch only when it matches one and the reader is at least
// minConfidence sure; anything else, and a hero read in two slots, is
// flagged unknown with no hero.
func Resolve(g [Slots]Guess, heroes []catalog.Hero, minConfidence float64) Read {
	byName := make(map[string]int, len(heroes))
	for _, h := range heroes {
		byName[normName(h.Name)] = h.ID
	}
	slots := make([]Slot, Slots)
	seen := map[int][]int{}
	for i, guess := range g {
		s := Slot{Status: StatusUnknown, Confidence: guess.Confidence}
		sure := guess.Confidence >= minConfidence
		switch id, ok := byName[normName(guess.Name)]; {
		case guess.Empty && sure:
			s.Status = StatusEmpty
		case !guess.Empty && ok && guess.Name != "" && sure:
			s.Status, s.HeroID = StatusHero, id
			seen[id] = append(seen[id], i)
		}
		slots[i] = s
	}
	for _, at := range seen {
		if len(at) > 1 { // one hero cannot be picked twice: none of them is trusted
			for _, i := range at {
				slots[i].Status, slots[i].HeroID = StatusUnknown, 0
			}
		}
	}
	return Read{Radiant: slots[:PerSide], Dire: slots[PerSide:]}
}

// normName compares names by their letters and digits only, ignoring case.
func normName(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// Chain reads with Primary and, when it fails or misses its deadline, with
// Secondary.
type Chain struct {
	Primary, Secondary                 Reader
	PrimaryDeadline, SecondaryDeadline time.Duration
}

// Name is the primary reader's.
func (c Chain) Name() string { return c.Primary.Name() }

// Read returns the primary read, else the secondary's with the reason the
// primary's was not used.
func (c Chain) Read(ctx context.Context, img image.Image, heroes []catalog.Hero) (Read, error) {
	pctx, cancel := context.WithTimeout(ctx, c.PrimaryDeadline)
	r, err := c.Primary.Read(pctx, img, heroes)
	late := pctx.Err() != nil
	cancel()
	if err == nil {
		return r, nil
	}
	if ctx.Err() != nil { // the caller gave up, not the reader
		return Read{}, ctx.Err()
	}
	reason := FallbackError
	if late || errors.Is(err, context.DeadlineExceeded) {
		reason = FallbackTimeout
	}
	sctx, cancel := context.WithTimeout(ctx, c.SecondaryDeadline)
	defer cancel()
	r2, err2 := c.Secondary.Read(sctx, img, heroes)
	if err2 != nil {
		return Read{}, fmt.Errorf("photo: %s: %w; %s: %w", c.Primary.Name(), err, c.Secondary.Name(), err2)
	}
	r2.Fallback = reason
	return r2, nil
}
