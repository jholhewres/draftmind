package gamesync

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// NoteKind is what a patch note is about.
type NoteKind string

// Note kinds as stored in patch_note.kind.
const (
	NoteHero    NoteKind = "hero"
	NoteAbility NoteKind = "ability"
	NoteTalent  NoteKind = "talent"
	NoteItem    NoteKind = "item"
	NoteGeneral NoteKind = "general"
)

// PatchNote is one line of a patch's notes.
type PatchNote struct {
	Kind     NoteKind
	HeroID   int // hero, ability and talent notes
	TargetID int // the ability of an ability note, the item of an item note
	Indent   int
	Text     string
}

// PatchNotes are one patch's notes in one language, in feed order.
type PatchNotes struct {
	Name       string
	ReleasedAt time.Time
	Notes      []PatchNote
}

// Release is a patch of the feed's patch list.
type Release struct {
	Name       string
	ReleasedAt time.Time
}

type rawNote struct {
	Note   string `json:"note"`
	Indent int    `json:"indent_level"`
}

type rawNoteGroup struct {
	AbilityID     int       `json:"ability_id"`
	Title         string    `json:"title"`
	IsGeneralNote bool      `json:"is_general_note"`
	Notes         []rawNote `json:"ability_notes"`
}

// ParsePatchList reads a patchnoteslist response, oldest patch first.
func ParsePatchList(b []byte) ([]Release, error) {
	var raw struct {
		Success bool `json:"success"`
		Patches []struct {
			Name      string `json:"patch_name"`
			Timestamp int64  `json:"patch_timestamp"`
		} `json:"patches"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, fmt.Errorf("parse patchnoteslist: %w", err)
	}
	if !raw.Success || len(raw.Patches) == 0 {
		return nil, errors.New("parse patchnoteslist: no patches")
	}
	out := make([]Release, 0, len(raw.Patches))
	seen := map[string]bool{}
	for _, p := range raw.Patches {
		if p.Name == "" || p.Timestamp <= 0 || seen[p.Name] {
			return nil, fmt.Errorf("parse patchnoteslist: patch without name or date, or twice: %q", p.Name)
		}
		seen[p.Name] = true
		out = append(out, Release{Name: p.Name, ReleasedAt: time.Unix(p.Timestamp, 0).UTC()})
	}
	slices.SortStableFunc(out, func(a, b Release) int { return a.ReleasedAt.Compare(b.ReleasedAt) })
	return out, nil
}

// ParsePatchNotes reads a patchnotes response for patch. Markup is
// stripped to text and lines left empty are dropped.
func ParsePatchNotes(b []byte, patch string) (PatchNotes, error) {
	var raw struct {
		Success   bool   `json:"success"`
		Name      string `json:"patch_name"`
		Timestamp int64  `json:"patch_timestamp"`
		General   []struct {
			Title   string    `json:"title"`
			Generic []rawNote `json:"generic"`
		} `json:"general_notes"`
		Heroes []struct {
			HeroID    int            `json:"hero_id"`
			Notes     []rawNote      `json:"hero_notes"`
			Abilities []rawNoteGroup `json:"abilities"`
			Talents   []rawNote      `json:"talent_notes"`
		} `json:"heroes"`
		Items        []rawNoteGroup `json:"items"`
		NeutralItems []rawNoteGroup `json:"neutral_items"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return PatchNotes{}, fmt.Errorf("parse patchnotes %s: %w", patch, err)
	}
	if !raw.Success || raw.Name != patch {
		return PatchNotes{}, fmt.Errorf("parse patchnotes: asked for %s, got %q", patch, raw.Name)
	}
	out := PatchNotes{Name: raw.Name}
	if raw.Timestamp > 0 {
		out.ReleasedAt = time.Unix(raw.Timestamp, 0).UTC()
	}
	add := func(n PatchNote, notes []rawNote) {
		for _, r := range notes {
			if text := plainText(r.Note); text != "" {
				n.Indent, n.Text = r.Indent, text
				out.Notes = append(out.Notes, n)
			}
		}
	}
	for _, g := range raw.General {
		if title := plainText(g.Title); title != "" {
			out.Notes = append(out.Notes, PatchNote{Kind: NoteGeneral, Text: title})
		}
		add(PatchNote{Kind: NoteGeneral}, g.Generic)
	}
	for _, h := range raw.Heroes {
		if h.HeroID <= 0 {
			return PatchNotes{}, fmt.Errorf("parse patchnotes %s: a hero without id", patch)
		}
		add(PatchNote{Kind: NoteHero, HeroID: h.HeroID}, h.Notes)
		for _, a := range h.Abilities {
			if a.AbilityID <= 0 {
				return PatchNotes{}, fmt.Errorf("parse patchnotes %s: hero %d has an ability without id", patch, h.HeroID)
			}
			add(PatchNote{Kind: NoteAbility, HeroID: h.HeroID, TargetID: a.AbilityID}, a.Notes)
		}
		add(PatchNote{Kind: NoteTalent, HeroID: h.HeroID}, h.Talents)
	}
	for _, it := range append(append([]rawNoteGroup{}, raw.Items...), raw.NeutralItems...) {
		if it.IsGeneralNote || it.AbilityID < 0 {
			continue // a section heading, not an item
		}
		if it.AbilityID == 0 {
			return PatchNotes{}, fmt.Errorf("parse patchnotes %s: an item without id", patch)
		}
		add(PatchNote{Kind: NoteItem, TargetID: it.AbilityID}, it.Notes)
	}
	return out, nil
}

// NotesFeed reads the data feed's patch notes.
type NotesFeed interface {
	PatchList(ctx context.Context) ([]byte, error)
	PatchNotes(ctx context.Context, patch, language string) ([]byte, error)
}

const patchNotesSource = "datafeed:patchnotes"

// RefetchRecent is how many of the newest patches each sync fetches again,
// since notes can be amended after release. Older patches already stored
// in every language are not fetched again (ADR-0007: cached by patch).
const RefetchRecent = 2

// PatchNotes syncs the notes of the last patches of the feed's list (all
// of them when last <= 0), one patch per transaction. A patch that fails is
// skipped and named in the returned error; the others are still stored.
func (s *Syncer) PatchNotes(ctx context.Context, feed NotesFeed, last int) (Result, error) {
	var runID int64
	if err := s.pool.QueryRow(ctx,
		`INSERT INTO sync_run (source, status) VALUES ($1, 'running') RETURNING id`, patchNotesSource).Scan(&runID); err != nil {
		return Result{}, fmt.Errorf("start sync run: %w", err)
	}
	res, err := s.patchNotes(ctx, feed, last)
	s.finish(runID, res, err)
	if err != nil {
		return res, err
	}
	s.log.Info("patch notes synced", "newest", res.Revision, "inserted", res.Inserted, "updated", res.Updated,
		"deleted", res.Deleted, "unchanged", res.Unchanged)
	return res, nil
}

func (s *Syncer) patchNotes(ctx context.Context, feed NotesFeed, last int) (Result, error) {
	var res Result
	b, err := feed.PatchList(ctx)
	if err != nil {
		return res, fmt.Errorf("patch list: %w", err)
	}
	releases, err := ParsePatchList(b)
	if err != nil {
		return res, err
	}
	if last > 0 && last < len(releases) {
		releases = releases[len(releases)-last:]
	}
	res.Revision = releases[len(releases)-1].Name

	stored, err := s.storedPatches(ctx)
	if err != nil {
		return res, err
	}
	var failed []string
	for i, r := range releases {
		if err := ctx.Err(); err != nil {
			return res, err
		}
		recent := i >= len(releases)-RefetchRecent
		if !recent && stored[r.Name] {
			continue
		}
		if err := s.patchRelease(ctx, feed, r, &res); err != nil {
			s.log.Warn("patch notes failed", "patch", r.Name, "err", err)
			failed = append(failed, fmt.Sprintf("patch %s: %v", r.Name, err))
		}
	}
	if len(failed) > 0 {
		return res, fmt.Errorf("%d of %d patches failed: %s", len(failed), len(releases), strings.Join(failed, "; "))
	}
	return res, nil
}

// storedPatches are the patches with notes in every synced language.
func (s *Syncer) storedPatches(ctx context.Context) (map[string]bool, error) {
	rows, err := s.pool.Query(ctx, `SELECT patch_name FROM patch_note GROUP BY patch_name HAVING count(DISTINCT lang) >= $1`,
		len(FeedLanguages))
	if err != nil {
		return nil, fmt.Errorf("stored patches: %w", err)
	}
	names, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, fmt.Errorf("stored patches: %w", err)
	}
	out := make(map[string]bool, len(names))
	for _, n := range names {
		out[n] = true
	}
	return out, nil
}

// patchRelease fetches one patch in every language and stores it in one transaction.
func (s *Syncer) patchRelease(ctx context.Context, feed NotesFeed, r Release, res *Result) error {
	byLang := make([]PatchNotes, len(FeedLanguages))
	for i, lang := range FeedLanguages {
		b, err := feed.PatchNotes(ctx, r.Name, lang.Feed)
		if err != nil {
			return err
		}
		if byLang[i], err = ParsePatchNotes(b, r.Name); err != nil {
			return err
		}
	}
	return s.inTx(ctx, res, func(tx pgx.Tx, c *Result) error { return storePatchNotes(ctx, tx, r, byLang, c) })
}

// storePatchNotes makes the patch's rows exactly the notes of each language.
func storePatchNotes(ctx context.Context, tx pgx.Tx, r Release, byLang []PatchNotes, res *Result) error {
	inserted, written, err := upsertReturning(ctx, tx, `
		INSERT INTO patch_release (name, released_at) VALUES ($1, $2)
		ON CONFLICT (name) DO UPDATE SET released_at = EXCLUDED.released_at
		WHERE patch_release.released_at IS DISTINCT FROM EXCLUDED.released_at
		RETURNING xmax = 0`, r.Name, r.ReleasedAt)
	if err != nil {
		return fmt.Errorf("store patch: %w", err)
	}
	res.add(inserted, written)
	for i, p := range byLang {
		tag := FeedLanguages[i].Tag
		for seq, n := range p.Notes {
			inserted, written, err := upsertReturning(ctx, tx, `
				INSERT INTO patch_note (patch_name, lang, seq, kind, hero_id, target_id, indent, text)
				VALUES ($1, $2, $3, $4, NULLIF($5, 0), NULLIF($6, 0), $7, $8)
				ON CONFLICT (patch_name, lang, seq) DO UPDATE SET kind = EXCLUDED.kind, hero_id = EXCLUDED.hero_id,
					target_id = EXCLUDED.target_id, indent = EXCLUDED.indent, text = EXCLUDED.text
				WHERE (patch_note.kind, patch_note.hero_id, patch_note.target_id, patch_note.indent, patch_note.text)
					IS DISTINCT FROM (EXCLUDED.kind, EXCLUDED.hero_id, EXCLUDED.target_id, EXCLUDED.indent, EXCLUDED.text)
				RETURNING xmax = 0`, r.Name, tag, seq, string(n.Kind), n.HeroID, n.TargetID, n.Indent, n.Text)
			if err != nil {
				return fmt.Errorf("store %s note %d: %w", tag, seq, err)
			}
			res.add(inserted, written)
		}
		del, err := tx.Exec(ctx, `DELETE FROM patch_note WHERE patch_name = $1 AND lang = $2 AND seq >= $3`,
			r.Name, tag, len(p.Notes))
		if err != nil {
			return fmt.Errorf("remove old %s notes: %w", tag, err)
		}
		res.Deleted += int(del.RowsAffected())
	}
	return nil
}
