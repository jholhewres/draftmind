// Package match keeps a match session (the draft, the player's hero and
// role, the item plan and the chat) and runs the coach on each message
// (ADR-0003): the writer model parses it, code gathers the facts, Jev
// decides the advice category and priority, and the writer answers.
package match

import (
	"cmp"
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
	ID      string `json:"id"`
	UserID  *int64 `json:"-"` // the signed-in player's account; nil for a visitor
	Patch   string `json:"patch"`
	HeroID  int    `json:"heroId"`
	Role    int    `json:"role"`
	Allies  []int  `json:"allies"`
	Enemies []int  `json:"enemies"`
	Bans    []int  `json:"bans"`
	// Mode is the game mode (ADR-0012).
	Mode      string      `json:"mode"`
	ItemPlan  items.Build `json:"itemPlan"`
	CreatedAt time.Time   `json:"createdAt"`
	Messages  []Message   `json:"messages"`
	Result    *Result     `json:"result,omitempty"` // set once the player confirmed how it ended
}

// Message is one chat line.
type Message struct {
	ID        int64      `json:"id"`
	Author    string     `json:"author"` // player or coach
	Kind      string     `json:"kind"`   // text or voice (player), answer or followup (coach)
	Text      string     `json:"text"`
	Situation *Situation `json:"situation,omitempty"`
	Category  string     `json:"category,omitempty"`
	Priority  string     `json:"priority,omitempty"`
	Source    string     `json:"source,omitempty"`
	Reason    string     `json:"reason,omitempty"`
	// Items are the cards of the items a coach answer names.
	Items     []items.Card `json:"items,omitempty"`
	CreatedAt time.Time    `json:"createdAt"`
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
		INSERT INTO match_session (patch_id, hero_id, role, allies, enemies, bans, item_plan, user_id, game_mode)
		SELECT id, $1, $2, $3, $4, $5, $6, $7, $8 FROM patch WHERE is_current
		RETURNING id::text, (SELECT name FROM patch WHERE is_current), created_at`,
		in.HeroID, in.Role, in.Allies, in.Enemies, in.Bans, plan, in.UserID, cmp.Or(in.Mode, "all_pick_ranked")).Scan(&in.ID, &in.Patch, &in.CreatedAt)
	in.Mode = cmp.Or(in.Mode, "all_pick_ranked")
	if errors.Is(err, pgx.ErrNoRows) {
		return in, errors.New("no current patch")
	}
	if err != nil {
		return in, fmt.Errorf("create match session: %w", err)
	}
	in.Messages = []Message{}
	return in, nil
}

// MaxMessages is how many of a session's latest messages Get returns.
const MaxMessages = 200

// Get returns the session with its latest MaxMessages messages, oldest first.
func (s *Store) Get(ctx context.Context, id string) (Session, error) {
	var out Session
	if !ValidID(id) {
		return out, ErrNotFound
	}
	var plan []byte
	var res struct {
		outcome, source *string
		notes           []string
		endedAt         *time.Time
	}
	r := &Result{}
	err := s.pool.QueryRow(ctx, `
		SELECT m.id::text, m.user_id, p.name, m.hero_id, m.role, m.allies, m.enemies, m.bans, m.game_mode, m.item_plan, m.created_at,
			r.result, r.duration_s, r.kills, r.deaths, r.assists, r.source, r.notes, m.ended_at
		FROM match_session m JOIN patch p ON p.id = m.patch_id
		LEFT JOIN match_result r ON r.session_id = m.id WHERE m.id = $1`, id).
		Scan(&out.ID, &out.UserID, &out.Patch, &out.HeroID, &out.Role, &out.Allies, &out.Enemies, &out.Bans, &out.Mode, &plan, &out.CreatedAt,
			&res.outcome, &r.DurationS, &r.Kills, &r.Deaths, &r.Assists, &res.source, &res.notes, &res.endedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, ErrNotFound
	}
	if err != nil {
		return out, fmt.Errorf("match session: %w", err)
	}
	if err := json.Unmarshal(plan, &out.ItemPlan); err != nil {
		return out, fmt.Errorf("match session item plan: %w", err)
	}
	if res.outcome != nil {
		r.Outcome, r.Source, r.Notes = *res.outcome, *res.source, res.notes
		if res.endedAt != nil {
			r.EndedAt = *res.endedAt
		}
		out.Result = r
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, author, kind, text, situation, coalesce(category, ''), coalesce(priority, ''), coalesce(source, ''),
			coalesce(reason, ''), items, created_at
		FROM (SELECT * FROM match_message WHERE session_id = $1 ORDER BY id DESC LIMIT $2) m ORDER BY id`, id, MaxMessages)
	if err != nil {
		return out, fmt.Errorf("match messages: %w", err)
	}
	out.Messages, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (Message, error) {
		var m Message
		err := row.Scan(&m.ID, &m.Author, &m.Kind, &m.Text, &m.Situation, &m.Category, &m.Priority, &m.Source, &m.Reason, &m.Items, &m.CreatedAt)
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
	var cards []items.Card
	if len(m.Items) > 0 {
		cards = m.Items
	}
	err := s.pool.QueryRow(ctx, `
		INSERT INTO match_message (session_id, author, kind, text, situation, category, priority, source, reason, items)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10) RETURNING id, created_at`,
		sessionID, m.Author, m.Kind, m.Text, m.Situation, null(m.Category), null(m.Priority), null(m.Source), null(m.Reason), cards).
		Scan(&m.ID, &m.CreatedAt)
	if err != nil {
		return m, fmt.Errorf("add match message: %w", err)
	}
	return m, nil
}
