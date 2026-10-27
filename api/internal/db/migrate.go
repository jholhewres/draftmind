package db

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

// Migration is one embedded SQL file, applied once and in name order.
type Migration struct {
	Name string
	SQL  string
}

// Migrations lists the embedded migrations sorted by name.
func Migrations() ([]Migration, error) {
	return readMigrations(migrationFS, "migrations")
}

func readMigrations(fsys fs.FS, dir string) ([]Migration, error) {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return nil, err
	}
	var out []Migration
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		b, err := fs.ReadFile(fsys, dir+"/"+e.Name())
		if err != nil {
			return nil, err
		}
		out = append(out, Migration{Name: e.Name(), SQL: string(b)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// Migrate applies every pending migration, each in its own transaction.
// It returns the names it applied.
func Migrate(ctx context.Context, pool *pgxpool.Pool) ([]string, error) {
	if _, err := pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		name text PRIMARY KEY,
		applied_at timestamptz NOT NULL DEFAULT now())`); err != nil {
		return nil, fmt.Errorf("create schema_migrations: %w", err)
	}
	migs, err := Migrations()
	if err != nil {
		return nil, err
	}
	var applied []string
	for _, m := range migs {
		done, err := apply(ctx, pool, m)
		if err != nil {
			return applied, fmt.Errorf("migration %s: %w", m.Name, err)
		}
		if done {
			applied = append(applied, m.Name)
		}
	}
	return applied, nil
}

func apply(ctx context.Context, pool *pgxpool.Pool, m Migration) (bool, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	// Serialises concurrent runners on the same database.
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(7240531)`); err != nil {
		return false, err
	}
	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE name = $1)`, m.Name).Scan(&exists); err != nil {
		return false, err
	}
	if exists {
		return false, nil
	}
	if _, err := tx.Exec(ctx, m.SQL); err != nil {
		return false, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO schema_migrations (name) VALUES ($1)`, m.Name); err != nil {
		return false, err
	}
	return true, tx.Commit(ctx)
}
