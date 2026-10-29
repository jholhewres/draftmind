package gsi

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrBadToken means no account has that token.
var ErrBadToken = errors.New("gsi: unknown or revoked token")

// Store keeps each account's GSI token, as its hash, and its last state.
type Store struct{ pool *pgxpool.Pool }

// NewStore builds a store.
func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

func hash(token string) []byte {
	h := sha256.Sum256([]byte(token))
	return h[:]
}

// Issue makes a new token for the account, replacing any old one. The token
// is returned once and kept only as its hash.
func (s *Store) Issue(ctx context.Context, userID int64) (string, error) {
	b := make([]byte, 32)
	_, _ = rand.Read(b) // never fails (crypto/rand)
	token := base64.RawURLEncoding.EncodeToString(b)
	_, err := s.pool.Exec(ctx, `
		INSERT INTO gsi_token (user_id, token_hash) VALUES ($1, $2)
		ON CONFLICT (user_id) DO UPDATE SET token_hash = $2, created_at = now(), last_post_at = NULL`, userID, hash(token))
	if err != nil {
		return "", fmt.Errorf("issue gsi token: %w", err)
	}
	return token, nil
}

// Revoke deletes the account's token and its last state.
func (s *Store) Revoke(ctx context.Context, userID int64) error {
	if _, err := s.pool.Exec(ctx, `DELETE FROM gsi_token WHERE user_id = $1`, userID); err != nil {
		return fmt.Errorf("revoke gsi token: %w", err)
	}
	_, err := s.pool.Exec(ctx, `DELETE FROM gsi_state WHERE user_id = $1`, userID)
	return err
}

// Authenticate returns the account a token belongs to and records the post time.
func (s *Store) Authenticate(ctx context.Context, token string, now time.Time) (int64, error) {
	var id int64
	err := s.pool.QueryRow(ctx, `UPDATE gsi_token SET last_post_at = $2 WHERE token_hash = $1 RETURNING user_id`, hash(token), now).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrBadToken
	}
	if err != nil {
		return 0, fmt.Errorf("gsi token: %w", err)
	}
	return id, nil
}

// Status is what the settings screen shows.
type Status struct {
	Configured bool       `json:"configured"`
	CreatedAt  *time.Time `json:"createdAt,omitempty"`
	LastPostAt *time.Time `json:"lastPostAt,omitempty"`
}

// Status reads whether the account has a token and when the game last posted.
func (s *Store) Status(ctx context.Context, userID int64) (Status, error) {
	var st Status
	var created time.Time
	err := s.pool.QueryRow(ctx, `SELECT created_at, last_post_at FROM gsi_token WHERE user_id = $1`, userID).Scan(&created, &st.LastPostAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return st, nil
	}
	if err != nil {
		return st, fmt.Errorf("gsi status: %w", err)
	}
	st.Configured, st.CreatedAt = true, &created
	return st, nil
}

// Last is the account's last state and when it came, or ok false.
func (s *Store) Last(ctx context.Context, userID int64) (State, time.Time, bool, error) {
	var raw []byte
	var at time.Time
	err := s.pool.QueryRow(ctx, `SELECT state, received_at FROM gsi_state WHERE user_id = $1`, userID).Scan(&raw, &at)
	if errors.Is(err, pgx.ErrNoRows) {
		return State{}, at, false, nil
	}
	if err != nil {
		return State{}, at, false, fmt.Errorf("gsi state: %w", err)
	}
	var st State
	if err := json.Unmarshal(raw, &st); err != nil {
		return State{}, at, false, err
	}
	return st, at, true, nil
}

// Save keeps the account's latest state.
func (s *Store) Save(ctx context.Context, userID int64, st State, now time.Time) error {
	raw, err := json.Marshal(st)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `
		INSERT INTO gsi_state (user_id, state, received_at) VALUES ($1, $2, $3)
		ON CONFLICT (user_id) DO UPDATE SET state = $2, received_at = $3`, userID, raw, now)
	return err
}
