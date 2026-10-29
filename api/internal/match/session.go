// Package match keeps a match session (the draft, the player's hero and
// role, the item plan and the chat) and runs the coach on each message
// (ADR-0003): the writer model parses it, code gathers the facts, Jev
// decides the advice category and priority, and the writer answers.
package match

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jholhewres/draftmind/api/internal/items"
)

// Session is one match being coached.
type Session struct {
	ID        string      `json:"id"`
	Patch     string      `json:"patch"`
	HeroID    int         `json:"heroId"`
	Role      int         `json:"role"`
	Allies    []int       `json:"allies"`
	Enemies   []int       `json:"enemies"`
	Bans      []int       `json:"bans"`
	ItemPlan  items.Build `json:"itemPlan"`
	CreatedAt time.Time   `json:"createdAt"`
	Messages  []Message   `json:"messages"`
}

// Message is one chat line.
type Message struct {
	ID        int64      `json:"id"`
	Author    string     `json:"author"` // player or coach
	Kind      string     `json:"kind"`   // text (player), answer or followup (coach)
	Text      string     `json:"text"`
	Situation *Situation `json:"situation,omitempty"`
	Category  string     `json:"category,omitempty"`
	Priority  string     `json:"priority,omitempty"`
	Source    string     `json:"source,omitempty"`
	Reason    string     `json:"reason,omitempty"`
	CreatedAt time.Time  `json:"createdAt"`
}

// ErrNotFound means no session has that id.
var ErrNotFound = errors.New("no such match session")

// uuidRe is a session id as Postgres writes it.
var uuidRe = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// ValidID says whether id can be a session id.
func ValidID(id string) bool { return uuidRe.MatchString(id) }

// Store keeps sessions in Postgres.
type Store struct{ pool *pgxpool.Pool }

// NewStore builds a store.
func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

// Create stores a new session on the current patch and returns it with its id.
func (s *Store) Create(ctx context.Context, in Session) (Session, error) {
	plan, err := json.Marshal(in.ItemPlan)
	if err != nil {
		return in, err
	}
	err = s.pool.QueryRow(ctx, `
		INSERT INTO match_session (patch_id, hero_id, role, allies, enemies, bans, item_plan)
		SELECT id, $1, $2, $3, $4, $5, $6 FROM patch WHERE is_current
		RETURNING id::text, (SELECT name FROM patch WHERE is_current), created_at`,
		in.HeroID, in.Role, in.Allies, in.Enemies, in.Bans, plan).Scan(&in.ID, &in.Patch, &in.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return in, errors.New("no current patch")
	}
	if err != nil {
		return in, fmt.Errorf("create match session: %w", err)
	}
	in.Messages = []Message{}
	return in, nil
}

// Get returns the session with its messages, oldest first.
func (s *Store) Get(ctx context.Context, id string) (Session, error) {
	var out Session
	if !ValidID(id) {
		return out, ErrNotFound
	}
	var plan []byte
	err := s.pool.QueryRow(ctx, `
		SELECT m.id::text, p.name, m.hero_id, m.role, m.allies, m.enemies, m.bans, m.item_plan, m.created_at
		FROM match_session m JOIN patch p ON p.id = m.patch_id WHERE m.id = $1`, id).
		Scan(&out.ID, &out.Patch, &out.HeroID, &out.Role, &out.Allies, &out.Enemies, &out.Bans, &plan, &out.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, ErrNotFound
	}
	if err != nil {
		return out, fmt.Errorf("match session: %w", err)
	}
	if err := json.Unmarshal(plan, &out.ItemPlan); err != nil {
		return out, fmt.Errorf("match session item plan: %w", err)
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, author, kind, text, situation, coalesce(category, ''), coalesce(priority, ''), coalesce(source, ''),
			coalesce(reason, ''), created_at
		FROM match_message WHERE session_id = $1 ORDER BY id`, id)
	if err != nil {
		return out, fmt.Errorf("match messages: %w", err)
	}
	out.Messages, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (Message, error) {
		var m Message
		err := row.Scan(&m.ID, &m.Author, &m.Kind, &m.Text, &m.Situation, &m.Category, &m.Priority, &m.Source, &m.Reason, &m.CreatedAt)
		return m, err
	})
	if err != nil {
		return out, fmt.Errorf("match messages: %w", err)
	}
	return out, nil
}

// Add stores a message of the session and returns it with its id.
func (s *Store) Add(ctx context.Context, sessionID string, m Message) (Message, error) {
	null := func(s string) *string {
		if s == "" {
			return nil
		}
		return &s
	}
	err := s.pool.QueryRow(ctx, `
		INSERT INTO match_message (session_id, author, kind, text, situation, category, priority, source, reason)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9) RETURNING id, created_at`,
		sessionID, m.Author, m.Kind, m.Text, m.Situation, null(m.Category), null(m.Priority), null(m.Source), null(m.Reason)).
		Scan(&m.ID, &m.CreatedAt)
	if err != nil {
		return m, fmt.Errorf("add match message: %w", err)
	}
	return m, nil
}
