package match

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/jholhewres/draftmind/api/internal/writer"
)

// Result is how a match ended, as the player confirmed it, with the
// coach's notes on it.
type Result struct {
	Outcome   string    `json:"result"` // win or loss
	DurationS *int      `json:"durationS,omitempty"`
	Kills     *int      `json:"kills,omitempty"`
	Deaths    *int      `json:"deaths,omitempty"`
	Assists   *int      `json:"assists,omitempty"`
	Source    string    `json:"source"` // tap, message or photo
	Notes     []string  `json:"notes"`
	EndedAt   time.Time `json:"endedAt"`
}

// Sources a result can come from.
var Sources = []string{"tap", "message", "photo"}

// MaxDuration is the longest match a result can record.
const MaxDuration = 4 * 3600

// Validate holds a result to what the database keeps.
func (r Result) Validate() error {
	if !slices.Contains(writer.Results, r.Outcome) {
		return errors.New("result must be win or loss")
	}
	if !slices.Contains(Sources, r.Source) {
		return errors.New("source must be tap, message or photo")
	}
	if r.DurationS != nil && (*r.DurationS < 1 || *r.DurationS > MaxDuration) {
		return fmt.Errorf("durationS must be 1 to %d", MaxDuration)
	}
	for _, n := range []*int{r.Kills, r.Deaths, r.Assists} {
		if n != nil && (*n < 0 || *n > 999) {
			return errors.New("kills, deaths and assists must be 0 to 999")
		}
	}
	return nil
}

// SaveResult stores the session's result, replacing any earlier one, and
// marks the session ended. A visitor's session is claimed by userID, when
// set: a player who signs in before ending the match keeps it.
func (s *Store) SaveResult(ctx context.Context, sessionID string, userID *int64, r Result) (Result, error) {
	if r.Notes == nil {
		r.Notes = []string{}
	}
	notes, err := json.Marshal(r.Notes)
	if err != nil {
		return r, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return r, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	err = tx.QueryRow(ctx, `
		UPDATE match_session SET ended_at = now(), user_id = coalesce(user_id, $2) WHERE id = $1 RETURNING ended_at`,
		sessionID, userID).Scan(&r.EndedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return r, ErrNotFound
	}
	if err != nil {
		return r, fmt.Errorf("end match session: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO match_result (session_id, result, duration_s, kills, deaths, assists, source, notes)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (session_id) DO UPDATE SET result = $2, duration_s = $3, kills = $4, deaths = $5, assists = $6,
			source = $7, notes = $8, created_at = now()`,
		sessionID, r.Outcome, r.DurationS, r.Kills, r.Deaths, r.Assists, r.Source, notes); err != nil {
		return r, fmt.Errorf("save match result: %w", err)
	}
	return r, tx.Commit(ctx)
}

// Past is one ended match in a player's history.
type Past struct {
	ID        string    `json:"id"`
	HeroID    int       `json:"heroId"`
	Role      int       `json:"role"`
	Result    string    `json:"result"`
	DurationS *int      `json:"durationS,omitempty"`
	Kills     *int      `json:"kills,omitempty"`
	Deaths    *int      `json:"deaths,omitempty"`
	Assists   *int      `json:"assists,omitempty"`
	EndedAt   time.Time `json:"endedAt"`
}

// MaxHistory is how many of a player's latest matches History returns.
const MaxHistory = 100

// History returns the user's ended matches, newest first.
func (s *Store) History(ctx context.Context, userID int64) ([]Past, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT m.id::text, m.hero_id, m.role, r.result, r.duration_s, r.kills, r.deaths, r.assists, m.ended_at
		FROM match_session m JOIN match_result r ON r.session_id = m.id
		WHERE m.user_id = $1 AND m.ended_at IS NOT NULL
		ORDER BY m.ended_at DESC, m.id LIMIT $2`, userID, MaxHistory)
	if err != nil {
		return nil, fmt.Errorf("match history: %w", err)
	}
	out, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Past, error) {
		var p Past
		err := row.Scan(&p.ID, &p.HeroID, &p.Role, &p.Result, &p.DurationS, &p.Kills, &p.Deaths, &p.Assists, &p.EndedAt)
		return p, err
	})
	if err != nil {
		return nil, fmt.Errorf("match history: %w", err)
	}
	return out, nil
}

// Noter writes the coach's end-of-match notes (the writer model).
type Noter interface {
	Notes(ctx context.Context, b writer.NotesBrief) writer.Notes
}

// MaxNoteMessages is how many of the chat's latest lines the notes read.
const MaxNoteMessages = 40

// Notes writes two or three notes on the ended match from the session's
// chat and facts only, held to the session's names like an answer.
func (c *Coach) Notes(ctx context.Context, s Session, r Result, lang string) writer.Notes {
	none := writer.Notes{Items: []string{}, Source: writer.SourceTemplate, Reason: writer.ReasonUnavailable}
	if c.Note == nil {
		return none
	}
	_, heroes, err := c.Facts.Heroes(ctx)
	if err != nil {
		none.Reason = writer.ReasonError
		return none
	}
	names, err := c.Facts.Names(ctx)
	if err != nil {
		none.Reason = writer.ReasonError
		return none
	}
	f, err := c.facts(ctx, s, Situation{Heroes: []int{}, Abilities: []string{}, Items: []string{}, Unknown: []string{}}, heroes, names, lang)
	if err != nil {
		none.Reason = writer.ReasonError
		return none
	}
	delete(f.data, "situation")
	result := map[string]any{"result": r.Outcome}
	if r.DurationS != nil {
		result["duration_minutes"] = *r.DurationS / 60
	}
	if r.Kills != nil {
		result["kills"], result["deaths"], result["assists"] = *r.Kills, *r.Deaths, *r.Assists
	}
	f.data["result"] = result
	msgs := s.Messages[max(0, len(s.Messages)-MaxNoteMessages):]
	lines := make([]string, 0, len(msgs))
	for _, m := range msgs {
		if m.Text != "" {
			lines = append(lines, m.Author+": "+m.Text)
		}
	}
	return c.Note.Notes(ctx, writer.NotesBrief{
		Lang: lang, Result: r.Outcome, Messages: lines, Facts: f.data, Allowed: f.allowed, Known: f.known,
	})
}
