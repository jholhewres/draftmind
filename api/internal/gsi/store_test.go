package gsi

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jholhewres/draftmind/api/internal/dbtest"
)

func TestATokenAuthenticatesOnlyItsAccountUntilRevokedOrRotated(t *testing.T) {
	pool := dbtest.New(t)
	ctx := context.Background()
	var alice, bob int64
	if err := pool.QueryRow(ctx, `INSERT INTO app_user (email) VALUES ('a@example.com') RETURNING id`).Scan(&alice); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO app_user (email) VALUES ('b@example.com') RETURNING id`).Scan(&bob); err != nil {
		t.Fatal(err)
	}
	s := NewStore(pool)
	tok, err := s.Issue(ctx, alice)
	if err != nil || len(tok) < 32 {
		t.Fatalf("issue: %q %v", tok, err)
	}
	// Stored as a hash, never as sent.
	var n int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM gsi_token WHERE token_hash = convert_to($1, 'UTF8')`, tok).Scan(&n)
	if n != 0 {
		t.Fatal("the token is stored as sent")
	}
	now := time.Unix(1_800_000_000, 0)
	if id, err := s.Authenticate(ctx, tok, now); err != nil || id != alice {
		t.Fatalf("authenticate: %d %v", id, err)
	}
	if _, err := s.Authenticate(ctx, "not-a-token", now); !errors.Is(err, ErrBadToken) {
		t.Fatalf("a wrong token: %v", err)
	}
	if st, err := s.Status(ctx, alice); err != nil || !st.Configured || st.LastPostAt == nil || !st.LastPostAt.Equal(now) {
		t.Fatalf("status after a post: %+v %v", st, err)
	}
	if st, _ := s.Status(ctx, bob); st.Configured {
		t.Fatal("bob has a token he never made")
	}
	// Rotating replaces the old token.
	tok2, _ := s.Issue(ctx, alice)
	if _, err := s.Authenticate(ctx, tok, now); !errors.Is(err, ErrBadToken) {
		t.Fatal("the rotated-out token still works")
	}
	if id, err := s.Authenticate(ctx, tok2, now); err != nil || id != alice {
		t.Fatalf("the new token: %d %v", id, err)
	}
	// Revoked: nothing authenticates.
	if err := s.Revoke(ctx, alice); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Authenticate(ctx, tok2, now); !errors.Is(err, ErrBadToken) {
		t.Fatal("a revoked token still works")
	}
}
