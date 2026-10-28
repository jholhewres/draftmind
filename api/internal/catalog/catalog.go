// Package catalog reads the synced game data.
package catalog

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Hero is a hero as the API serves it.
type Hero struct {
	ID          int      `json:"id"`
	Slug        string   `json:"slug"`
	Name        string   `json:"name"`
	PrimaryAttr string   `json:"primaryAttr"`
	AttackType  string   `json:"attackType"`
	Roles       []string `json:"roles"`
	Img         string   `json:"img"`
	Icon        string   `json:"icon"`
}

// Store reads the catalog from Postgres.
type Store struct{ pool *pgxpool.Pool }

// NewStore builds a store.
func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

// Heroes returns the current patch name and its heroes, sorted by name.
// The patch is empty until the first sync.
func (s *Store) Heroes(ctx context.Context) (string, []Hero, error) {
	var patch string
	err := s.pool.QueryRow(ctx, `SELECT name FROM patch WHERE is_current`).Scan(&patch)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return "", nil, fmt.Errorf("current patch: %w", err)
	}
	rows, err := s.pool.Query(ctx, `
		SELECT h.id, h.slug, h.name, h.primary_attr, h.attack_type, h.roles, h.img, h.icon
		FROM hero h
		JOIN hero_patch hp ON hp.hero_id = h.id
		JOIN patch p ON p.id = hp.patch_id AND p.is_current
		ORDER BY h.name`)
	if err != nil {
		return "", nil, fmt.Errorf("list heroes: %w", err)
	}
	heroes, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Hero, error) {
		var h Hero
		err := row.Scan(&h.ID, &h.Slug, &h.Name, &h.PrimaryAttr, &h.AttackType, &h.Roles, &h.Img, &h.Icon)
		return h, err
	})
	if err != nil {
		return "", nil, fmt.Errorf("list heroes: %w", err)
	}
	return patch, heroes, nil
}

// FallbackLang is the language text falls back to when lang lacks it.
const FallbackLang = "en"

// ErrNotFound means the current patch has no such hero.
var ErrNotFound = errors.New("not found")

// Stats are a hero's base stats on the current patch.
type Stats struct {
	StrBase         int     `json:"strBase"`
	StrGain         float64 `json:"strGain"`
	AgiBase         int     `json:"agiBase"`
	AgiGain         float64 `json:"agiGain"`
	IntBase         int     `json:"intBase"`
	IntGain         float64 `json:"intGain"`
	Armor           float64 `json:"armor"`
	MagicResistance int     `json:"magicResistance"`
	DamageMin       int     `json:"damageMin"`
	DamageMax       int     `json:"damageMax"`
	AttackRate      float64 `json:"attackRate"`
	AttackRange     int     `json:"attackRange"`
	MovementSpeed   int     `json:"movementSpeed"`
	MaxHealth       int     `json:"maxHealth"`
	HealthRegen     float64 `json:"healthRegen"`
	MaxMana         int     `json:"maxMana"`
	ManaRegen       float64 `json:"manaRegen"`
}

// Ability is an ability as the hero page shows it.
type Ability struct {
	ID          int       `json:"id"`
	Kind        string    `json:"kind"` // ability or innate
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Lore        string    `json:"lore"`
	Cooldowns   []float64 `json:"cooldowns"`
	ManaCosts   []float64 `json:"manaCosts"`
}

// Talent is one talent and the hero level it unlocks at.
type Talent struct {
	ID    int    `json:"id"`
	Level int    `json:"level"`
	Name  string `json:"name"`
}

// HeroDetail is a hero's page on the current patch in one language.
type HeroDetail struct {
	Hero
	Patch      string    `json:"patch"`
	Lang       string    `json:"lang"`
	Hype       string    `json:"hype"`
	Complexity int       `json:"complexity"`
	Stats      *Stats    `json:"stats"` // nil until the details sync reaches the hero
	Abilities  []Ability `json:"abilities"`
	Talents    []Talent  `json:"talents"`
}

// HeroDetail returns the hero with slug on the current patch, its text in lang.
// Text missing in lang falls back to FallbackLang.
func (s *Store) HeroDetail(ctx context.Context, slug, lang string) (HeroDetail, error) {
	d := HeroDetail{Lang: lang}
	var patchID int
	var complexity, strBase, agiBase, intBase, magicRes, dmgMin, dmgMax, attackRange, moveSpeed, maxHealth, maxMana *int
	var strGain, agiGain, intGain, armor, attackRate, healthRegen, manaRegen *float64
	var hype *string
	err := s.pool.QueryRow(ctx, `
		SELECT p.id, p.name, h.id, h.slug, coalesce(t.name, h.name), h.primary_attr, h.attack_type, h.roles, h.img, h.icon,
			t.hype, hp.complexity, hp.str_base, hp.str_gain, hp.agi_base, hp.agi_gain, hp.int_base, hp.int_gain,
			hp.armor, hp.magic_resistance, hp.damage_min, hp.damage_max, hp.attack_rate, hp.attack_range,
			hp.movement_speed, hp.max_health, hp.health_regen, hp.max_mana, hp.mana_regen
		FROM hero h
		JOIN hero_patch hp ON hp.hero_id = h.id
		JOIN patch p ON p.id = hp.patch_id AND p.is_current
		LEFT JOIN LATERAL (
			SELECT name, hype FROM hero_text t
			WHERE t.patch_id = hp.patch_id AND t.hero_id = hp.hero_id AND t.lang IN ($2, $3)
			ORDER BY t.lang = $2 DESC LIMIT 1) t ON true
		WHERE h.slug = $1`, slug, lang, FallbackLang).Scan(
		&patchID, &d.Patch, &d.ID, &d.Slug, &d.Name, &d.PrimaryAttr, &d.AttackType, &d.Roles, &d.Img, &d.Icon,
		&hype, &complexity, &strBase, &strGain, &agiBase, &agiGain, &intBase, &intGain,
		&armor, &magicRes, &dmgMin, &dmgMax, &attackRate, &attackRange, &moveSpeed, &maxHealth, &healthRegen, &maxMana, &manaRegen)
	if errors.Is(err, pgx.ErrNoRows) {
		return d, ErrNotFound
	}
	if err != nil {
		return d, fmt.Errorf("hero %s: %w", slug, err)
	}
	if hype != nil {
		d.Hype = *hype
	}
	if strBase != nil { // the details sync reached the hero
		d.Complexity = deref(complexity)
		d.Stats = &Stats{
			StrBase: *strBase, StrGain: deref(strGain), AgiBase: deref(agiBase), AgiGain: deref(agiGain),
			IntBase: deref(intBase), IntGain: deref(intGain), Armor: deref(armor), MagicResistance: deref(magicRes),
			DamageMin: deref(dmgMin), DamageMax: deref(dmgMax), AttackRate: deref(attackRate), AttackRange: deref(attackRange),
			MovementSpeed: deref(moveSpeed), MaxHealth: deref(maxHealth), HealthRegen: deref(healthRegen),
			MaxMana: deref(maxMana), ManaRegen: deref(manaRegen),
		}
	}

	rows, err := s.pool.Query(ctx, `
		SELECT a.id, a.kind, a.slot, a.cooldowns, a.mana_costs, coalesce(t.name, a.name), coalesce(t.description, ''), coalesce(t.lore, '')
		FROM ability a
		LEFT JOIN LATERAL (
			SELECT name, description, lore FROM ability_text t
			WHERE t.patch_id = a.patch_id AND t.hero_id = a.hero_id AND t.ability_id = a.id AND t.lang IN ($3, $4)
			ORDER BY t.lang = $3 DESC LIMIT 1) t ON true
		WHERE a.patch_id = $1 AND a.hero_id = $2
		ORDER BY a.slot`, patchID, d.ID, lang, FallbackLang)
	if err != nil {
		return d, fmt.Errorf("abilities of %s: %w", slug, err)
	}
	defer rows.Close()
	d.Abilities, d.Talents = []Ability{}, []Talent{}
	for rows.Next() {
		var a Ability
		var slot int
		if err := rows.Scan(&a.ID, &a.Kind, &slot, &a.Cooldowns, &a.ManaCosts, &a.Name, &a.Description, &a.Lore); err != nil {
			return d, fmt.Errorf("abilities of %s: %w", slug, err)
		}
		if a.Kind == "talent" {
			// Two talents per level, from 10 to 25, in feed order.
			d.Talents = append(d.Talents, Talent{ID: a.ID, Level: 10 + 5*(len(d.Talents)/2), Name: a.Name})
			continue
		}
		d.Abilities = append(d.Abilities, a)
	}
	if err := rows.Err(); err != nil {
		return d, fmt.Errorf("abilities of %s: %w", slug, err)
	}
	return d, nil
}

func deref[T any](p *T) T {
	var zero T
	if p == nil {
		return zero
	}
	return *p
}
