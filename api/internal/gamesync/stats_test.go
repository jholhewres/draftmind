package gamesync

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jholhewres/draftmind/api/internal/dbtest"
)

// fixtureOpenDota serves testdata/opendota_*.json; failMatchups makes one
// hero's matchups fail like an unreachable API.
type fixtureOpenDota struct{ failMatchups int }

func (f fixtureOpenDota) HeroStats(context.Context) ([]byte, error) {
	return readFixtureFile("opendota_herostats.json")
}

func (f fixtureOpenDota) Matchups(_ context.Context, heroID int) ([]byte, error) {
	if heroID == f.failMatchups {
		return nil, errors.New("GET /heroes/13/matchups: HTTP 503")
	}
	b, err := readFixtureFile(fmt.Sprintf("opendota_matchups_%d.json", heroID))
	if err != nil {
		return nil, fmt.Errorf("no matchups fixture for hero %d", heroID)
	}
	return b, nil
}

type matchupRow struct{ hero, opponent, games, wins int }

func matchupRows(t *testing.T, pool *pgxpool.Pool) []matchupRow {
	t.Helper()
	rows, err := pool.Query(context.Background(), `
		SELECT hero_id, opponent_id, games, wins FROM hero_matchup m JOIN patch p ON p.id = m.patch_id AND p.is_current
		ORDER BY hero_id, opponent_id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []matchupRow
	for rows.Next() {
		var r matchupRow
		if err := rows.Scan(&r.hero, &r.opponent, &r.games, &r.wins); err != nil {
			t.Fatal(err)
		}
		out = append(out, r)
	}
	return out
}

func syncRun(t *testing.T, pool *pgxpool.Pool) (status string, inserted int, errText *string) {
	t.Helper()
	if err := pool.QueryRow(context.Background(), `
		SELECT status, inserted, error FROM sync_run WHERE source = 'opendota:stats' ORDER BY id DESC LIMIT 1`).
		Scan(&status, &inserted, &errText); err != nil {
		t.Fatal(err)
	}
	return status, inserted, errText
}

func TestMatchupSyncStoresMatchupsAndStatsForTheCurrentPatch(t *testing.T) {
	pool := dbtest.New(t)
	syncPuckAndAxe(t, pool)
	ctx := context.Background()
	res, err := NewSyncer(pool, nil, quietLog()).Stats(ctx, fixtureOpenDota{})
	if err != nil {
		t.Fatal(err)
	}
	// Only Axe and Puck are on the patch: one matchup each way, 9 brackets each.
	want := []matchupRow{{2, 13, 141, 81}, {13, 2, 141, 60}}
	if got := matchupRows(t, pool); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("matchups %v, want %v", got, want)
	}
	var stats, picks, wins int
	if err := pool.QueryRow(ctx, `SELECT count(*), sum(picks) FILTER (WHERE hero_id = 2 AND bracket = 'pro'),
		sum(wins) FILTER (WHERE hero_id = 2 AND bracket = 'pro') FROM hero_stat`).Scan(&stats, &picks, &wins); err != nil {
		t.Fatal(err)
	}
	if stats != 18 || picks != 11 || wins != 4 {
		t.Fatalf("%d stat rows, Axe pro %d/%d, want 18 rows and 11 picks, 4 wins", stats, picks, wins)
	}
	if status, inserted, _ := syncRun(t, pool); status != "succeeded" || inserted != 20 || res.Inserted != 20 {
		t.Fatalf("run %s with %d inserted (result %+v), want succeeded with 20", status, inserted, res)
	}
}

func TestMatchupSyncSecondRunChangesZeroRows(t *testing.T) {
	pool := dbtest.New(t)
	syncPuckAndAxe(t, pool)
	ctx := context.Background()
	s := NewSyncer(pool, nil, quietLog())
	if _, err := s.Stats(ctx, fixtureOpenDota{}); err != nil {
		t.Fatal(err)
	}
	second, err := s.Stats(ctx, fixtureOpenDota{})
	if err != nil {
		t.Fatal(err)
	}
	if second.Changed() != 0 || second.Unchanged != 20 {
		t.Fatalf("second run %+v, want 0 changed and 20 unchanged", second)
	}
}

func TestMatchupSyncFailureKeepsSavedRowsAndMarksTheRunFailed(t *testing.T) {
	pool := dbtest.New(t)
	syncPuckAndAxe(t, pool)
	ctx := context.Background()
	_, err := NewSyncer(pool, nil, quietLog()).Stats(ctx, fixtureOpenDota{failMatchups: 13})
	if err == nil || !strings.Contains(err.Error(), "hero 13") {
		t.Fatalf("err = %v, want one naming hero 13", err)
	}
	// Axe's matchups came before Puck's failed and stay saved.
	if got := matchupRows(t, pool); fmt.Sprint(got) != fmt.Sprint([]matchupRow{{2, 13, 141, 81}}) {
		t.Fatalf("matchups %v, want Axe's row kept", got)
	}
	status, inserted, errText := syncRun(t, pool)
	if status != "failed" || inserted != 19 || errText == nil || !strings.Contains(*errText, "HTTP 503") {
		t.Fatalf("run %s with %d inserted, error %v; want failed with the 19 saved rows and the cause", status, inserted, errText)
	}
	// A later run picks up where it stopped.
	res, err := NewSyncer(pool, nil, quietLog()).Stats(ctx, fixtureOpenDota{})
	if err != nil || res.Inserted != 1 || res.Unchanged != 19 {
		t.Fatalf("resumed run %+v, %v; want 1 inserted and 19 unchanged", res, err)
	}
}

func TestMatchupSyncDropsAnOpponentNoLongerInTheResponse(t *testing.T) {
	pool := dbtest.New(t)
	syncPuckAndAxe(t, pool)
	ctx := context.Background()
	s := NewSyncer(pool, nil, quietLog())
	if _, err := s.Stats(ctx, fixtureOpenDota{}); err != nil {
		t.Fatal(err)
	}
	res, err := s.Stats(ctx, emptyMatchups{fixtureOpenDota{}})
	if err != nil || res.Deleted != 2 {
		t.Fatalf("run %+v, %v; want both matchups deleted", res, err)
	}
	if got := matchupRows(t, pool); len(got) != 0 {
		t.Fatalf("matchups %v, want none", got)
	}
}

type emptyMatchups struct{ fixtureOpenDota }

func (emptyMatchups) Matchups(context.Context, int) ([]byte, error) { return []byte("[]"), nil }
