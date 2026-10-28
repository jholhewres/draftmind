package gamesync

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"
)

// Feed reads the game's data feed.
type Feed interface {
	// HeroData returns the herodata response for a hero in a feed language.
	HeroData(ctx context.Context, heroID int, language string) ([]byte, error)
}

// FeedLanguage maps a stored language tag to the feed's language name.
type FeedLanguage struct{ Tag, Feed string }

// FeedLanguages are the languages synced; the first one also gives the
// stats and values, which do not change with the language.
var FeedLanguages = []FeedLanguage{{Tag: "en", Feed: "english"}, {Tag: "pt-BR", Feed: "brazilian"}}

const detailsSource = "datafeed:herodata"

// Details syncs the stats, abilities and talents of the current patch's
// heroes, one hero per transaction. A hero that fails is skipped and named
// in the returned error; the others are still stored.
func (s *Syncer) Details(ctx context.Context, feed Feed) (Result, error) {
	var runID int64
	if err := s.pool.QueryRow(ctx,
		`INSERT INTO sync_run (source, status) VALUES ($1, 'running') RETURNING id`, detailsSource).Scan(&runID); err != nil {
		return Result{}, fmt.Errorf("start sync run: %w", err)
	}
	res, err := s.details(ctx, feed)
	s.finish(runID, res, err)
	if err != nil {
		return res, err
	}
	s.log.Info("hero details synced", "inserted", res.Inserted, "updated", res.Updated, "deleted", res.Deleted,
		"unchanged", res.Unchanged)
	return res, nil
}

func (s *Syncer) details(ctx context.Context, feed Feed) (Result, error) {
	var res Result
	var patchID int
	if err := s.pool.QueryRow(ctx, `SELECT id FROM patch WHERE is_current`).Scan(&patchID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return res, errors.New("no current patch: run sync heroes first")
		}
		return res, fmt.Errorf("current patch: %w", err)
	}
	rows, err := s.pool.Query(ctx, `SELECT hero_id FROM hero_patch WHERE patch_id = $1 ORDER BY hero_id`, patchID)
	if err != nil {
		return res, fmt.Errorf("current patch heroes: %w", err)
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[int])
	if err != nil {
		return res, fmt.Errorf("current patch heroes: %w", err)
	}
	inPatch := make(map[int]bool, len(ids))
	for _, id := range ids {
		inPatch[id] = true
	}

	var failed []string
	for _, id := range ids {
		if err := ctx.Err(); err != nil {
			return res, err
		}
		if err := s.heroDetails(ctx, feed, patchID, id, inPatch, &res); err != nil {
			s.log.Warn("hero details failed", "hero", id, "err", err)
			failed = append(failed, fmt.Sprintf("hero %d: %v", id, err))
		}
	}
	if len(failed) > 0 {
		return res, fmt.Errorf("%d of %d heroes failed: %s", len(failed), len(ids), strings.Join(failed, "; "))
	}
	return res, nil
}

// heroDetails fetches one hero in every language and stores it in one transaction.
func (s *Syncer) heroDetails(ctx context.Context, feed Feed, patchID, heroID int, inPatch map[int]bool, res *Result) error {
	byLang := make([]HeroDetail, len(FeedLanguages))
	for i, lang := range FeedLanguages {
		b, err := feed.HeroData(ctx, heroID, lang.Feed)
		if err != nil {
			return err
		}
		d, err := ParseHeroData(b, inPatch)
		if err != nil {
			return err
		}
		if d.ID != heroID {
			return fmt.Errorf("asked for hero %d in %s, got hero %d", heroID, lang.Feed, d.ID)
		}
		byLang[i] = d
	}
	var counts Result
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		counts = Result{}
		return storeHeroDetails(ctx, tx, patchID, byLang, &counts)
	})
	if err != nil {
		return err
	}
	res.Inserted += counts.Inserted
	res.Updated += counts.Updated
	res.Deleted += counts.Deleted
	res.Unchanged += counts.Unchanged
	return nil
}

func storeHeroDetails(ctx context.Context, tx pgx.Tx, patchID int, byLang []HeroDetail, res *Result) error {
	base := byLang[0]
	st := base.Stats
	tag, err := tx.Exec(ctx, `
		UPDATE hero_patch SET complexity = $3, str_base = $4, str_gain = $5, agi_base = $6, agi_gain = $7,
			int_base = $8, int_gain = $9, armor = $10, magic_resistance = $11, damage_min = $12, damage_max = $13,
			attack_rate = $14, attack_range = $15, projectile_speed = $16, movement_speed = $17, turn_rate = $18,
			max_health = $19, health_regen = $20, max_mana = $21, mana_regen = $22, sight_range_day = $23,
			sight_range_night = $24
		WHERE patch_id = $1 AND hero_id = $2
			AND (complexity, str_base, str_gain, agi_base, agi_gain, int_base, int_gain, armor, magic_resistance,
				damage_min, damage_max, attack_rate, attack_range, projectile_speed, movement_speed, turn_rate,
				max_health, health_regen, max_mana, mana_regen, sight_range_day, sight_range_night)
			IS DISTINCT FROM ($3::smallint, $4::integer, $5::double precision, $6::integer, $7::double precision,
				$8::integer, $9::double precision, $10::double precision, $11::integer, $12::integer, $13::integer,
				$14::double precision, $15::integer, $16::integer, $17::integer, $18::double precision, $19::integer,
				$20::double precision, $21::integer, $22::double precision, $23::integer, $24::integer)`,
		patchID, base.ID, base.Complexity, st.StrBase, st.StrGain, st.AgiBase, st.AgiGain, st.IntBase, st.IntGain,
		st.Armor, st.MagicResistance, st.DamageMin, st.DamageMax, st.AttackRate, st.AttackRange, st.ProjectileSpeed,
		st.MovementSpeed, st.TurnRate, st.MaxHealth, st.HealthRegen, st.MaxMana, st.ManaRegen, st.SightRangeDay,
		st.SightRangeNight)
	if err != nil {
		return fmt.Errorf("store stats: %w", err)
	}
	if tag.RowsAffected() == 1 {
		res.Updated++
	} else {
		res.Unchanged++
	}

	for i, d := range byLang {
		inserted, written, err := upsertReturning(ctx, tx, `
			INSERT INTO hero_text (patch_id, hero_id, lang, name, hype, bio) VALUES ($1, $2, $3, $4, $5, $6)
			ON CONFLICT (patch_id, hero_id, lang) DO UPDATE SET name = EXCLUDED.name, hype = EXCLUDED.hype, bio = EXCLUDED.bio
			WHERE (hero_text.name, hero_text.hype, hero_text.bio) IS DISTINCT FROM (EXCLUDED.name, EXCLUDED.hype, EXCLUDED.bio)
			RETURNING xmax = 0`, patchID, d.ID, FeedLanguages[i].Tag, d.Name, d.Hype, d.Bio)
		if err != nil {
			return fmt.Errorf("store hero text: %w", err)
		}
		res.add(inserted, written)
	}

	all := append(append([]Ability{}, base.Abilities...), base.Talents...)
	ids := make([]int, len(all))
	for slot, a := range all {
		ids[slot] = a.ID
		inserted, written, err := upsertReturning(ctx, tx, `
			INSERT INTO ability (patch_id, hero_id, id, slot, kind, name, cooldowns, mana_costs)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			ON CONFLICT (patch_id, hero_id, id) DO UPDATE SET slot = EXCLUDED.slot, kind = EXCLUDED.kind,
				name = EXCLUDED.name, cooldowns = EXCLUDED.cooldowns, mana_costs = EXCLUDED.mana_costs
			WHERE (ability.slot, ability.kind, ability.name, ability.cooldowns, ability.mana_costs)
				IS DISTINCT FROM (EXCLUDED.slot, EXCLUDED.kind, EXCLUDED.name, EXCLUDED.cooldowns, EXCLUDED.mana_costs)
			RETURNING xmax = 0`, patchID, base.ID, a.ID, slot, string(a.Kind), a.Name, orEmpty(a.Cooldowns), orEmpty(a.ManaCosts))
		if err != nil {
			return fmt.Errorf("store ability %s: %w", a.Name, err)
		}
		res.add(inserted, written)
	}
	del, err := tx.Exec(ctx, `DELETE FROM ability WHERE patch_id = $1 AND hero_id = $2 AND id <> ALL($3::integer[])`,
		patchID, base.ID, ids)
	if err != nil {
		return fmt.Errorf("remove old abilities: %w", err)
	}
	res.Deleted += int(del.RowsAffected())

	for i, d := range byLang {
		for _, a := range append(append([]Ability{}, d.Abilities...), d.Talents...) {
			if !slices.Contains(ids, a.ID) {
				continue // only in this language: no row to hang its text on
			}
			inserted, written, err := upsertReturning(ctx, tx, `
				INSERT INTO ability_text (patch_id, hero_id, ability_id, lang, name, description, lore)
				VALUES ($1, $2, $3, $4, $5, $6, $7)
				ON CONFLICT (patch_id, hero_id, ability_id, lang) DO UPDATE SET name = EXCLUDED.name,
					description = EXCLUDED.description, lore = EXCLUDED.lore
				WHERE (ability_text.name, ability_text.description, ability_text.lore)
					IS DISTINCT FROM (EXCLUDED.name, EXCLUDED.description, EXCLUDED.lore)
				RETURNING xmax = 0`, patchID, base.ID, a.ID, FeedLanguages[i].Tag, a.Text.Name, a.Text.Description, a.Text.Lore)
			if err != nil {
				return fmt.Errorf("store ability text %s: %w", a.Name, err)
			}
			res.add(inserted, written)
		}
	}
	return nil
}

func orEmpty(vs []float64) []float64 {
	if vs == nil {
		return []float64{}
	}
	return vs
}
