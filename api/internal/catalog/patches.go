package catalog

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// Note is one line of a patch's notes as the API serves it.
type Note struct {
	Kind    string `json:"kind"`              // hero, ability, talent, item or general
	Ability string `json:"ability,omitempty"` // the ability's name, when the current patch has it
	Indent  int    `json:"indent"`
	Text    string `json:"text"`
}

// PatchHistory is one patch's notes about a hero.
type PatchHistory struct {
	Name       string    `json:"name"`
	ReleasedAt time.Time `json:"releasedAt"`
	Notes      []Note    `json:"notes"`
}

// HeroHistory is a hero's notes across patches, newest patch first.
type HeroHistory struct {
	Slug    string         `json:"slug"`
	Lang    string         `json:"lang"`
	Patches []PatchHistory `json:"patches"`
}

// PatchSummary is a patch of the feed's list and how much it changed.
type PatchSummary struct {
	Name       string    `json:"name"`
	ReleasedAt time.Time `json:"releasedAt"`
	Heroes     int       `json:"heroes"`
	Items      int       `json:"items"`
}

// HeroChanges are a patch's notes about one hero.
type HeroChanges struct {
	Hero
	Notes []Note `json:"notes"`
}

// ItemChanges are a patch's notes about one item.
type ItemChanges struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Notes []Note `json:"notes"`
}

// PatchChanges is what one patch changed, in feed order.
type PatchChanges struct {
	Name       string        `json:"name"`
	ReleasedAt time.Time     `json:"releasedAt"`
	Lang       string        `json:"lang"`
	General    []Note        `json:"general"`
	Heroes     []HeroChanges `json:"heroes"`
	Items      []ItemChanges `json:"items"`
}

// noteLang picks, per patch, lang when the patch has notes in it, else FallbackLang.
const noteLang = `
	SELECT patch_name, CASE WHEN bool_or(lang = $2) THEN $2 ELSE $3 END AS lang
	FROM patch_note GROUP BY patch_name`

// abilityName names an ability note's ability from the current patch, in lang or FallbackLang.
const abilityName = `
	LEFT JOIN LATERAL (
		SELECT t.name FROM ability_text t
		WHERE n.kind = 'ability' AND t.patch_id = (SELECT id FROM patch WHERE is_current)
			AND t.hero_id = n.hero_id AND t.ability_id = n.target_id AND t.lang IN ($2, $3)
		ORDER BY t.lang = $2 DESC LIMIT 1) a ON true`

// HeroHistory returns the notes of the hero with slug, newest patch first.
func (s *Store) HeroHistory(ctx context.Context, slug, lang string) (HeroHistory, error) {
	out := HeroHistory{Slug: slug, Lang: lang, Patches: []PatchHistory{}}
	var heroID int
	err := s.pool.QueryRow(ctx, `SELECT id FROM hero WHERE slug = $1`, slug).Scan(&heroID)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, ErrNotFound
	}
	if err != nil {
		return out, fmt.Errorf("hero %s: %w", slug, err)
	}
	rows, err := s.pool.Query(ctx, `
		WITH l AS (`+noteLang+`)
		SELECT r.name, r.released_at, n.kind, coalesce(a.name, ''), n.indent, n.text
		FROM patch_note n
		JOIN l ON l.patch_name = n.patch_name AND l.lang = n.lang
		JOIN patch_release r ON r.name = n.patch_name`+abilityName+`
		WHERE n.hero_id = $1
		ORDER BY r.released_at DESC, n.seq`, heroID, lang, FallbackLang)
	if err != nil {
		return out, fmt.Errorf("history of %s: %w", slug, err)
	}
	defer rows.Close()
	for rows.Next() {
		var p PatchHistory
		var n Note
		if err := rows.Scan(&p.Name, &p.ReleasedAt, &n.Kind, &n.Ability, &n.Indent, &n.Text); err != nil {
			return out, fmt.Errorf("history of %s: %w", slug, err)
		}
		if last := len(out.Patches) - 1; last >= 0 && out.Patches[last].Name == p.Name {
			out.Patches[last].Notes = append(out.Patches[last].Notes, n)
			continue
		}
		p.Notes = []Note{n}
		out.Patches = append(out.Patches, p)
	}
	if err := rows.Err(); err != nil {
		return out, fmt.Errorf("history of %s: %w", slug, err)
	}
	return out, nil
}

// Patches lists the synced patches, newest first, with how many heroes and
// items each changed.
func (s *Store) Patches(ctx context.Context) ([]PatchSummary, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT r.name, r.released_at, count(DISTINCT n.hero_id), count(DISTINCT n.target_id) FILTER (WHERE n.kind = 'item')
		FROM patch_release r
		LEFT JOIN patch_note n ON n.patch_name = r.name AND n.lang = $1
		GROUP BY r.name, r.released_at
		ORDER BY r.released_at DESC`, FallbackLang)
	if err != nil {
		return nil, fmt.Errorf("list patches: %w", err)
	}
	out, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (PatchSummary, error) {
		var p PatchSummary
		err := row.Scan(&p.Name, &p.ReleasedAt, &p.Heroes, &p.Items)
		return p, err
	})
	if err != nil {
		return nil, fmt.Errorf("list patches: %w", err)
	}
	return out, nil
}

// PatchChanges returns the notes of the patch named name, in lang when the
// patch has it, else in FallbackLang. Heroes and items the database does
// not know are left out.
func (s *Store) PatchChanges(ctx context.Context, name, lang string) (PatchChanges, error) {
	out := PatchChanges{Name: name, General: []Note{}, Heroes: []HeroChanges{}, Items: []ItemChanges{}}
	err := s.pool.QueryRow(ctx, `
		SELECT r.released_at, coalesce(l.lang, $3)
		FROM patch_release r LEFT JOIN (`+noteLang+`) l ON l.patch_name = r.name
		WHERE r.name = $1`, name, lang, FallbackLang).Scan(&out.ReleasedAt, &out.Lang)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, ErrNotFound
	}
	if err != nil {
		return out, fmt.Errorf("patch %s: %w", name, err)
	}

	rows, err := s.pool.Query(ctx, `
		SELECT n.kind, coalesce(a.name, ''), n.indent, n.text, coalesce(n.hero_id, 0), coalesce(n.target_id, 0),
			coalesce(h.slug, ''), coalesce(ht.name, h.name, ''), coalesce(h.primary_attr, ''), coalesce(h.attack_type, ''),
			coalesce(h.roles, '{}'), coalesce(h.img, ''), coalesce(h.icon, ''), coalesce(it.dname, '')
		FROM patch_note n
		LEFT JOIN hero h ON h.id = n.hero_id
		LEFT JOIN LATERAL (
			SELECT t.name FROM hero_text t
			WHERE t.patch_id = (SELECT id FROM patch WHERE is_current) AND t.hero_id = n.hero_id AND t.lang IN ($2, $3)
			ORDER BY t.lang = $2 DESC LIMIT 1) ht ON true
		LEFT JOIN LATERAL (
			SELECT i.dname FROM item i WHERE n.kind = 'item' AND i.id = n.target_id
			ORDER BY i.patch_id DESC LIMIT 1) it ON true`+abilityName+`
		WHERE n.patch_name = $1 AND n.lang = $4
		ORDER BY n.seq`, name, lang, FallbackLang, out.Lang)
	if err != nil {
		return out, fmt.Errorf("notes of %s: %w", name, err)
	}
	defer rows.Close()
	heroAt, itemAt := map[int]int{}, map[int]int{}
	for rows.Next() {
		var n Note
		var heroID, targetID int
		var h Hero
		var itemName string
		if err := rows.Scan(&n.Kind, &n.Ability, &n.Indent, &n.Text, &heroID, &targetID,
			&h.Slug, &h.Name, &h.PrimaryAttr, &h.AttackType, &h.Roles, &h.Img, &h.Icon, &itemName); err != nil {
			return out, fmt.Errorf("notes of %s: %w", name, err)
		}
		switch {
		case n.Kind == "general":
			out.General = append(out.General, n)
		case n.Kind == "item" && itemName != "":
			i, ok := itemAt[targetID]
			if !ok {
				i = len(out.Items)
				itemAt[targetID] = i
				out.Items = append(out.Items, ItemChanges{ID: targetID, Name: itemName})
			}
			out.Items[i].Notes = append(out.Items[i].Notes, n)
		case heroID != 0 && h.Slug != "":
			i, ok := heroAt[heroID]
			if !ok {
				h.ID = heroID
				i = len(out.Heroes)
				heroAt[heroID] = i
				out.Heroes = append(out.Heroes, HeroChanges{Hero: h})
			}
			out.Heroes[i].Notes = append(out.Heroes[i].Notes, n)
		}
	}
	if err := rows.Err(); err != nil {
		return out, fmt.Errorf("notes of %s: %w", name, err)
	}
	return out, nil
}
