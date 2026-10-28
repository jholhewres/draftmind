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
