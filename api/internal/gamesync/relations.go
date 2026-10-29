package gamesync

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/jholhewres/draftmind/api/internal/draft"
	"github.com/jholhewres/draftmind/api/internal/items"
)

// Counters is the curated counter file, api/internal/curated/item_counters.json (ADR-0011).
type Counters struct {
	Version int `json:"version"`
	// Items are, per tag, the item keys that answer it.
	Items map[string][]string `json:"items"`
	// Heroes are, per hero slug, the tags the hero brings.
	Heroes map[string][]string `json:"heroes"`
	// Roles are, per dotaconstants role, the tags every hero with it brings.
	Roles map[string][]string `json:"roles"`
}

// ParseCounters reads the counter file; tags are lower-case words.
func ParseCounters(b []byte) (Counters, error) {
	var c Counters
	if err := json.Unmarshal(b, &c); err != nil {
		return c, fmt.Errorf("parse item counters: %w", err)
	}
	if c.Version <= 0 {
		return c, errors.New("item counters: version is required")
	}
	check := func(tag string) error {
		if tag == "" || strings.Trim(tag, "abcdefghijklmnopqrstuvwxyz_") != "" {
			return fmt.Errorf("item counters: tag %q is not a lower-case word", tag)
		}
		return nil
	}
	for tag := range c.Items {
		if err := check(tag); err != nil {
			return c, err
		}
	}
	for _, m := range []map[string][]string{c.Heroes, c.Roles} {
		for _, tags := range m {
			for _, tag := range tags {
				if err := check(tag); err != nil {
					return c, err
				}
			}
		}
	}
	return c, nil
}

const relationsSource = "draftmind:relations"

// Relations syncs the current patch's item components, counter tags and
// builds per hero and position in one transaction (ADR-0011), from the
// synced items, roles, stats and item popularity and the curated counters.
// Rows the inputs no longer give are removed.
func (s *Syncer) Relations(ctx context.Context, counters []byte) (Result, error) {
	c, err := ParseCounters(counters)
	if err != nil {
		return Result{}, err
	}
	var runID int64
	if err := s.pool.QueryRow(ctx,
		`INSERT INTO sync_run (source, status) VALUES ($1, 'running') RETURNING id`, relationsSource).Scan(&runID); err != nil {
		return Result{}, fmt.Errorf("start sync run: %w", err)
	}
	var res Result
	err = s.inTx(ctx, &res, func(tx pgx.Tx, r *Result) error { return s.relations(ctx, tx, c, r) })
	s.finish(runID, res, err)
	if err != nil {
		return res, err
	}
	s.log.Info("relations synced", "counters_version", c.Version, "inserted", res.Inserted, "updated", res.Updated,
		"deleted", res.Deleted, "unchanged", res.Unchanged)
	return res, nil
}

func (s *Syncer) relations(ctx context.Context, tx pgx.Tx, c Counters, res *Result) error {
	patchID, err := s.targetPatch(ctx, tx)
	if err != nil {
		return err
	}
	byKey := map[string]items.Item{}
	ds := items.Dataset{Items: map[int]items.Item{}, Popularity: map[int]map[string][]items.Count{}}
	rows, err := tx.Query(ctx, `SELECT id, key, dname, cost, qual, components FROM item WHERE patch_id = $1`, patchID)
	if err != nil {
		return fmt.Errorf("items: %w", err)
	}
	var it items.Item
	if _, err := pgx.ForEachRow(rows, []any{&it.ID, &it.Key, &it.Name, &it.Cost, &it.Qual, &it.Components}, func() error {
		ds.Items[it.ID], byKey[it.Key] = it, it
		it.Components = nil
		return nil
	}); err != nil {
		return fmt.Errorf("items: %w", err)
	}
	if len(ds.Items) == 0 {
		return errors.New("the current patch has no items: run sync heroes first")
	}

	// Components, by id; a component the patch lacks is skipped.
	var comps [][2]int
	for _, id := range slices.Sorted(maps.Keys(ds.Items)) {
		for _, k := range ds.Items[id].Components {
			if c, ok := byKey[k]; ok && c.ID != id && !slices.Contains(comps, [2]int{id, c.ID}) {
				comps = append(comps, [2]int{id, c.ID})
			}
		}
	}
	keys := make([]string, len(comps))
	for i, p := range comps {
		keys[i] = strconv.Itoa(p[0]) + ":" + strconv.Itoa(p[1])
		if err := upsert(ctx, tx, res, `INSERT INTO item_component (patch_id, item_id, component_id) VALUES ($1, $2, $3)
			ON CONFLICT DO NOTHING RETURNING xmax = 0`, patchID, p[0], p[1]); err != nil {
			return fmt.Errorf("item component: %w", err)
		}
	}
	if err := prune(ctx, tx, res, `DELETE FROM item_component WHERE patch_id = $1 AND item_id || ':' || component_id <> ALL($2::text[])`,
		patchID, keys); err != nil {
		return err
	}

	// Item tags.
	keys = keys[:0]
	for _, tag := range slices.Sorted(maps.Keys(c.Items)) {
		for _, k := range c.Items[tag] {
			item, ok := byKey[k]
			if !ok {
				continue
			}
			keys = append(keys, strconv.Itoa(item.ID)+":"+tag)
			if err := upsert(ctx, tx, res, `INSERT INTO item_tag (patch_id, item_id, tag, source) VALUES ($1, $2, $3, 'curated')
				ON CONFLICT DO NOTHING RETURNING xmax = 0`, patchID, item.ID, tag); err != nil {
				return fmt.Errorf("item tag: %w", err)
			}
		}
	}
	if err := prune(ctx, tx, res, `DELETE FROM item_tag WHERE patch_id = $1 AND item_id || ':' || tag <> ALL($2::text[])`,
		patchID, keys); err != nil {
		return err
	}

	// Heroes on the patch, their roles and pub record.
	type hero struct {
		id          int
		slug        string
		roles       []string
		games, wins int
	}
	rows, err = tx.Query(ctx, `
		SELECT h.id, h.slug, h.roles, coalesce(sum(st.picks), 0), coalesce(sum(st.wins), 0)
		FROM hero_patch hp JOIN hero h ON h.id = hp.hero_id
		LEFT JOIN hero_stat st ON st.patch_id = hp.patch_id AND st.hero_id = hp.hero_id AND st.bracket <> 'pro'
		WHERE hp.patch_id = $1 GROUP BY h.id ORDER BY h.id`, patchID)
	if err != nil {
		return fmt.Errorf("heroes: %w", err)
	}
	var heroes []hero
	var h hero
	if _, err := pgx.ForEachRow(rows, []any{&h.id, &h.slug, &h.roles, &h.games, &h.wins}, func() error {
		heroes = append(heroes, h)
		return nil
	}); err != nil {
		return fmt.Errorf("heroes: %w", err)
	}

	// Hero tags: the curated ones win over those from roles.
	keys = keys[:0]
	for _, h := range heroes {
		tags := map[string]string{}
		for _, r := range h.roles {
			for _, tag := range c.Roles[r] {
				tags[tag] = "roles"
			}
		}
		for _, tag := range c.Heroes[h.slug] {
			tags[tag] = "curated"
		}
		for _, tag := range slices.Sorted(maps.Keys(tags)) {
			keys = append(keys, strconv.Itoa(h.id)+":"+tag)
			if err := upsert(ctx, tx, res, `INSERT INTO hero_tag (patch_id, hero_id, tag, source) VALUES ($1, $2, $3, $4)
				ON CONFLICT (patch_id, hero_id, tag) DO UPDATE SET source = EXCLUDED.source
				WHERE hero_tag.source IS DISTINCT FROM EXCLUDED.source RETURNING xmax = 0`, patchID, h.id, tag, tags[tag]); err != nil {
				return fmt.Errorf("hero tag: %w", err)
			}
		}
	}
	if err := prune(ctx, tx, res, `DELETE FROM hero_tag WHERE patch_id = $1 AND hero_id || ':' || tag <> ALL($2::text[])`,
		patchID, keys); err != nil {
		return err
	}

	// Builds: the popularity core at each position the hero's roles fit best.
	var heroID int
	var phase string
	rows, err = tx.Query(ctx, `SELECT hero_id, phase, item_id, count FROM hero_item_popularity WHERE patch_id = $1`, patchID)
	if err != nil {
		return fmt.Errorf("item popularity: %w", err)
	}
	var cnt items.Count
	if _, err := pgx.ForEachRow(rows, []any{&heroID, &phase, &cnt.ItemID, &cnt.Count}, func() error {
		if ds.Popularity[heroID] == nil {
			ds.Popularity[heroID] = map[string][]items.Count{}
		}
		ds.Popularity[heroID][phase] = append(ds.Popularity[heroID][phase], cnt)
		return nil
	}); err != nil {
		return fmt.Errorf("item popularity: %w", err)
	}
	builds := []string{}
	itemKeys := []string{}
	for _, h := range heroes {
		core := items.Core(ds, h.id)
		if !slices.ContainsFunc(core, func(p items.PhaseItems) bool { return len(p.Items) > 0 }) {
			continue
		}
		var games *int
		var rate *float64
		if h.games > 0 {
			r := float64(h.wins) / float64(h.games)
			games, rate = &h.games, &r
		}
		for _, pos := range BuildPositions(h.roles) {
			builds = append(builds, strconv.Itoa(h.id)+":"+strconv.Itoa(pos))
			if err := upsert(ctx, tx, res, `
				INSERT INTO hero_build (patch_id, hero_id, position, source, name, games, win_rate) VALUES ($1, $2, $3, 'opendota', $4, $5, $6)
				ON CONFLICT (patch_id, hero_id, position, source) DO UPDATE SET name = EXCLUDED.name, games = EXCLUDED.games, win_rate = EXCLUDED.win_rate
				WHERE (hero_build.name, hero_build.games, hero_build.win_rate) IS DISTINCT FROM (EXCLUDED.name, EXCLUDED.games, EXCLUDED.win_rate)
				RETURNING xmax = 0`, patchID, h.id, pos, "popular", games, rate); err != nil {
				return fmt.Errorf("hero %d build: %w", h.id, err)
			}
			ord := 0
			for _, p := range core {
				for _, ic := range p.Items {
					itemKeys = append(itemKeys, fmt.Sprintf("%d:%d:%d", h.id, pos, ord))
					if err := upsert(ctx, tx, res, `
						INSERT INTO hero_build_item (patch_id, hero_id, position, source, ord, phase, item_id) VALUES ($1, $2, $3, 'opendota', $4, $5, $6)
						ON CONFLICT (patch_id, hero_id, position, source, ord) DO UPDATE SET phase = EXCLUDED.phase, item_id = EXCLUDED.item_id
						WHERE (hero_build_item.phase, hero_build_item.item_id) IS DISTINCT FROM (EXCLUDED.phase, EXCLUDED.item_id)
						RETURNING xmax = 0`, patchID, h.id, pos, ord, p.Phase, ic.ID); err != nil {
						return fmt.Errorf("hero %d build item: %w", h.id, err)
					}
					ord++
				}
			}
		}
	}
	if err := prune(ctx, tx, res, `DELETE FROM hero_build_item WHERE patch_id = $1 AND source = 'opendota'
		AND hero_id || ':' || position || ':' || ord <> ALL($2::text[])`, patchID, itemKeys); err != nil {
		return err
	}
	return prune(ctx, tx, res, `DELETE FROM hero_build WHERE patch_id = $1 AND source = 'opendota'
		AND hero_id || ':' || position <> ALL($2::text[])`, patchID, builds)
}

// BuildPositions are the positions (1-5) the roles fit best (ADR-0009's
// role fit), or none when they fit no position.
func BuildPositions(roles []string) []int {
	best := 0.0
	for pos := 1; pos <= 5; pos++ {
		best = max(best, draft.RoleFit(pos, roles))
	}
	var out []int
	for pos := 1; pos <= 5 && best > 0; pos++ {
		if draft.RoleFit(pos, roles) == best {
			out = append(out, pos)
		}
	}
	return out
}

// upsert runs an upsert that returns xmax = 0 only when it wrote a row.
func upsert(ctx context.Context, tx pgx.Tx, res *Result, sql string, args ...any) error {
	inserted, written, err := upsertReturning(ctx, tx, sql, args...)
	if err != nil {
		return err
	}
	res.add(inserted, written)
	return nil
}

// prune deletes the rows sql selects and counts them.
func prune(ctx context.Context, tx pgx.Tx, res *Result, sql string, args ...any) error {
	tag, err := tx.Exec(ctx, sql, args...)
	if err != nil {
		return err
	}
	res.Deleted += int(tag.RowsAffected())
	return nil
}
