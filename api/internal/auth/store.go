package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

var (
	errLinkUsed  = errors.New("the link expired or was already used")
	errNoSession = errors.New("no live session")
)

// User is a signed-in person: an e-mail address, a SteamID or, later, both.
type User struct {
	ID      int64  `json:"-"`
	Email   string `json:"email,omitempty"`
	SteamID string `json:"steamId,omitempty"`
}

// newSecret is a random token to send and the hash to keep of it.
func newSecret() (token string, hash []byte) {
	b := make([]byte, 32)
	_, _ = rand.Read(b) // never fails (crypto/rand)
	token = base64.RawURLEncoding.EncodeToString(b)
	return token, hashOf(token)
}

func hashOf(token string) []byte {
	h := sha256.Sum256([]byte(token))
	return h[:]
}

func (s *Service) createLoginToken(ctx context.Context, email string, now time.Time) (string, error) {
	token, hash := newSecret()
	_, err := s.pool.Exec(ctx, `INSERT INTO login_token (token_hash, email, created_at, expires_at) VALUES ($1, $2, $3, $4)`,
		hash, email, now, now.Add(LinkTTL))
	return token, err
}

// useLoginToken spends the link and opens a session for its address,
// creating the account on its first sign-in.
func (s *Service) useLoginToken(ctx context.Context, token string, now time.Time) (User, string, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return User{}, "", err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var email string
	err = tx.QueryRow(ctx, `
		UPDATE login_token SET used_at = $2
		WHERE token_hash = $1 AND used_at IS NULL AND expires_at > $2
		RETURNING email`, hashOf(token), now).Scan(&email)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, "", errLinkUsed
	}
	if err != nil {
		return User{}, "", err
	}
	// A password set before the address was proved is not the owner's:
	// the link drops it and ends the sessions it opened (ADR-0010).
	if _, err := tx.Exec(ctx, `
		WITH squatted AS (
			UPDATE app_user SET password_hash = NULL
			WHERE email = $1 AND email_verified_at IS NULL AND password_hash IS NOT NULL
			RETURNING id)
		DELETE FROM user_session WHERE user_id IN (SELECT id FROM squatted)`, email); err != nil {
		return User{}, "", err
	}
	u := User{Email: email}
	if err := tx.QueryRow(ctx, `
		INSERT INTO app_user (email, email_verified_at) VALUES ($1, $2)
		ON CONFLICT (email) DO UPDATE SET email_verified_at = coalesce(app_user.email_verified_at, EXCLUDED.email_verified_at)
		RETURNING id, coalesce(steam_id, '')`, email, now).Scan(&u.ID, &u.SteamID); err != nil {
		return User{}, "", err
	}
	session, err := createSession(ctx, tx, u.ID, now)
	if err != nil {
		return User{}, "", err
	}
	return u, session, tx.Commit(ctx)
}

// steamSession opens a session for the SteamID, creating the account on
// its first sign-in.
func (s *Service) steamSession(ctx context.Context, steamID string, now time.Time) (string, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var id int64
	if err := tx.QueryRow(ctx, `
		INSERT INTO app_user (steam_id) VALUES ($1)
		ON CONFLICT (steam_id) DO UPDATE SET steam_id = EXCLUDED.steam_id
		RETURNING id`, steamID).Scan(&id); err != nil {
		return "", err
	}
	session, err := createSession(ctx, tx, id, now)
	if err != nil {
		return "", err
	}
	return session, tx.Commit(ctx)
}

// execer is a pool or a transaction.
type execer interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

func createSession(ctx context.Context, db execer, userID int64, now time.Time) (string, error) {
	token, hash := newSecret()
	_, err := db.Exec(ctx, `INSERT INTO user_session (id_hash, user_id, created_at, expires_at) VALUES ($1, $2, $3, $4)`,
		hash, userID, now, now.Add(SessionTTL))
	return token, err
}

func (s *Service) sessionUser(ctx context.Context, token string, now time.Time) (User, error) {
	var u User
	err := s.pool.QueryRow(ctx, `
		SELECT u.id, coalesce(u.email, ''), coalesce(u.steam_id, '')
		FROM user_session s JOIN app_user u ON u.id = s.user_id
		WHERE s.id_hash = $1 AND s.expires_at > $2`, hashOf(token), now).Scan(&u.ID, &u.Email, &u.SteamID)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, errNoSession
	}
	return u, err
}

func (s *Service) deleteSession(ctx context.Context, token string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM user_session WHERE id_hash = $1`, hashOf(token))
	return err
}
