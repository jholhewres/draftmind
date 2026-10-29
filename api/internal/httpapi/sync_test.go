package httpapi

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/jholhewres/draftmind/api/internal/catalog"
)

func TestSyncStatusShowsTheCurrentPatchAndTheLastSyncPerSource(t *testing.T) {
	pool := syncedPatchNotes(t)
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO sync_run (source, status, started_at, finished_at, error)
		VALUES ('opendota:stats', 'failed', '2026-10-28T10:00:00Z', '2026-10-28T10:05:00Z', 'GET /heroStats: HTTP 502')`); err != nil {
		t.Fatal(err)
	}
	code, got := get[catalog.SyncStatus](t, pool, "/api/sync/status")
	if code != http.StatusOK {
		t.Fatalf("status %d", code)
	}
	last := map[string]time.Time{}
	for _, s := range got.Sources {
		last[s.Source] = s.LastSuccess
	}
	if got.Patch != "7.41" || last["dotaconstants:heroes"].IsZero() || last["datafeed:patchnotes"].IsZero() ||
		got.UpdatedAt == nil || !got.UpdatedAt.Equal(last["datafeed:patchnotes"]) {
		t.Fatalf("status %+v, want 7.41 with heroes and notes synced, updated at the notes' sync", got)
	}
	if _, ok := last["opendota:stats"]; ok {
		t.Fatalf("a source that only failed has a last success: %+v", got.Sources)
	}
	if f := got.LastFailure; f == nil || f.Source != "opendota:stats" || !f.At.Equal(time.Date(2026, 10, 28, 10, 5, 0, 0, time.UTC)) {
		t.Fatalf("last failure %+v, want opendota:stats at 10:05", got.LastFailure)
	}
}
