package catalog

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// SyncStatus is how fresh the synced data is.
type SyncStatus struct {
	Patch       string         `json:"patch"` // the current patch; empty before the first sync
	UpdatedAt   *time.Time     `json:"updatedAt"`
	Sources     []SourceStatus `json:"sources"`
	LastFailure *SyncFailure   `json:"lastFailure"`
}

// SourceStatus is a source's last successful sync.
type SourceStatus struct {
	Source      string    `json:"source"`
	LastSuccess time.Time `json:"lastSuccess"`
}

// SyncFailure is the newest failed sync run.
type SyncFailure struct {
	Source string    `json:"source"`
	At     time.Time `json:"at"`
}

// SyncStatus reads the current patch and the sync runs. A failure's error
// text stays in sync_run: it is for operators, not the public.
func (s *Store) SyncStatus(ctx context.Context) (SyncStatus, error) {
	out := SyncStatus{Sources: []SourceStatus{}}
	err := s.pool.QueryRow(ctx, `SELECT name FROM patch WHERE is_current`).Scan(&out.Patch)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return out, fmt.Errorf("current patch: %w", err)
	}
	rows, err := s.pool.Query(ctx, `
		SELECT source, max(finished_at) FROM sync_run WHERE status = 'succeeded' AND finished_at IS NOT NULL
		GROUP BY source ORDER BY source`)
	if err != nil {
		return out, fmt.Errorf("last syncs: %w", err)
	}
	out.Sources, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (SourceStatus, error) {
		var st SourceStatus
		err := row.Scan(&st.Source, &st.LastSuccess)
		return st, err
	})
	if err != nil {
		return out, fmt.Errorf("last syncs: %w", err)
	}
	for _, st := range out.Sources {
		if out.UpdatedAt == nil || st.LastSuccess.After(*out.UpdatedAt) {
			at := st.LastSuccess
			out.UpdatedAt = &at
		}
	}
	var f SyncFailure
	err = s.pool.QueryRow(ctx, `
		SELECT source, finished_at FROM sync_run WHERE status = 'failed' AND finished_at IS NOT NULL
		ORDER BY finished_at DESC LIMIT 1`).Scan(&f.Source, &f.At)
	switch {
	case err == nil:
		out.LastFailure = &f
	case !errors.Is(err, pgx.ErrNoRows):
		return out, fmt.Errorf("last failure: %w", err)
	}
	return out, nil
}
