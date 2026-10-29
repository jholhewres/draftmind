// Package persona is the player's own coach (B5): tone, language,
// verbosity and focus. A persona shapes only the writer's prompt; the code
// ranking, Jev's decisions and the item plan never read it (ADR-0003).
package persona

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Persona is how the coach talks.
type Persona struct {
	Tone      string `json:"tone"`
	Language  string `json:"language"` // auto follows the UI language
	Verbosity string `json:"verbosity"`
	Focus     string `json:"focus"`
}

// The values each setting takes.
var (
	Tones       = []string{"direct", "encouraging", "analytic"}
	Languages   = []string{"auto", "en", "pt-BR"}
	Verbosities = []string{"short", "normal"}
	Focuses     = []string{"laning", "teamfights", "items", "macro"}
	// UILanguages are the app's catalogs, for the player's UI language.
	UILanguages = []string{"en", "pt-BR"}
)

// Default is the coach before the player changes anything.
func Default() Persona {
	return Persona{Tone: "direct", Language: "auto", Verbosity: "normal", Focus: "laning"}
}

// Validate holds each setting to its values.
func (p Persona) Validate() error {
	for _, f := range []struct {
		name, v string
		in      []string
	}{{"tone", p.Tone, Tones}, {"language", p.Language, Languages}, {"verbosity", p.Verbosity, Verbosities}, {"focus", p.Focus, Focuses}} {
		if !slices.Contains(f.in, f.v) {
			return fmt.Errorf("%s must be one of %v", f.name, f.in)
		}
	}
	return nil
}

// LangFor is the language the coach answers in: the persona's, or ui when
// it is auto.
func (p Persona) LangFor(ui string) string {
	if p.Language == "" || p.Language == "auto" {
		return ui
	}
	return p.Language
}

var (
	toneText = map[string]string{
		"direct":      "Be direct: say what to do, with no padding or praise.",
		"encouraging": "Be encouraging: keep the player's spirits up while you say what to do.",
		"analytic":    "Be analytic: give the reason from the facts, such as the win-rate numbers, behind what to do.",
	}
	focusText = map[string]string{
		"laning":     "laning",
		"teamfights": "teamfights",
		"items":      "items",
		"macro":      "map play: objectives, farm and timings",
	}
)

// Instructions is the persona as writer instructions for an answer, in
// English: its voice and its length. The language is given apart.
func (p Persona) Instructions() string {
	if p.Validate() != nil {
		p = Default()
	}
	length := "At most three short sentences."
	if p.Verbosity == "short" {
		length = "One or two short sentences."
	}
	return length + " " + p.Voice()
}

// Voice is the persona's tone and focus as writer instructions, for text
// whose length is set elsewhere.
func (p Persona) Voice() string {
	if p.Validate() != nil {
		p = Default()
	}
	return toneText[p.Tone] + " The player cares most about " + focusText[p.Focus] +
		": where the facts allow, word the advice from that angle, but never change what was decided."
}

// Store keeps each signed-in player's persona.
type Store struct{ pool *pgxpool.Pool }

// NewStore builds a store.
func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

// Get returns the user's persona, or Default when they never saved one.
func (s *Store) Get(ctx context.Context, userID int64) (Persona, error) {
	var p Persona
	err := s.pool.QueryRow(ctx, `SELECT tone, language, verbosity, focus FROM coach_persona WHERE user_id = $1`, userID).
		Scan(&p.Tone, &p.Language, &p.Verbosity, &p.Focus)
	if errors.Is(err, pgx.ErrNoRows) {
		return Default(), nil
	}
	if err != nil {
		return p, fmt.Errorf("coach persona: %w", err)
	}
	return p, nil
}

// Save stores the user's persona.
func (s *Store) Save(ctx context.Context, userID int64, p Persona) error {
	if err := p.Validate(); err != nil {
		return err
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO coach_persona (user_id, tone, language, verbosity, focus) VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (user_id) DO UPDATE SET tone = $2, language = $3, verbosity = $4, focus = $5, updated_at = now()`,
		userID, p.Tone, p.Language, p.Verbosity, p.Focus)
	if err != nil {
		return fmt.Errorf("save coach persona: %w", err)
	}
	return nil
}

// UILanguage is the UI language the user picked, or "" before they pick one.
func (s *Store) UILanguage(ctx context.Context, userID int64) (string, error) {
	var lang string
	err := s.pool.QueryRow(ctx, `SELECT coalesce(ui_language, '') FROM app_user WHERE id = $1`, userID).Scan(&lang)
	if err != nil {
		return "", fmt.Errorf("ui language: %w", err)
	}
	return lang, nil
}

// SaveUILanguage stores the user's UI language, one of UILanguages.
func (s *Store) SaveUILanguage(ctx context.Context, userID int64, lang string) error {
	if !slices.Contains(UILanguages, lang) {
		return fmt.Errorf("language must be one of %v", UILanguages)
	}
	if _, err := s.pool.Exec(ctx, `UPDATE app_user SET ui_language = $2 WHERE id = $1`, userID, lang); err != nil {
		return fmt.Errorf("save ui language: %w", err)
	}
	return nil
}

// GameMode is the game mode the user last coached a match in, or "".
func (s *Store) GameMode(ctx context.Context, userID int64) (string, error) {
	var mode string
	if err := s.pool.QueryRow(ctx, `SELECT coalesce(game_mode, '') FROM app_user WHERE id = $1`, userID).Scan(&mode); err != nil {
		return "", fmt.Errorf("game mode: %w", err)
	}
	return mode, nil
}

// SaveGameMode remembers the user's game mode; the column checks it.
func (s *Store) SaveGameMode(ctx context.Context, userID int64, mode string) error {
	if _, err := s.pool.Exec(ctx, `UPDATE app_user SET game_mode = $2 WHERE id = $1`, userID, mode); err != nil {
		return fmt.Errorf("save game mode: %w", err)
	}
	return nil
}
