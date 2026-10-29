// Package writer turns a decided, structured draft result into short text
// (ADR-0003 step 4). The model only words the facts it is given; Check holds
// every text to the names in those facts, and anything that fails, is late or
// errors becomes a template the UI fills from the same facts.
package writer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/jholhewres/draftmind/api/internal/budget"
)

// MaxTip is the longest tip, in characters, that stands.
const MaxTip = 160

// DefaultTimeout bounds one tip; the code ranking is already on screen.
const DefaultTimeout = 4 * time.Second

// Where a tip's text came from, and why a template took its place.
const (
	SourceWriter   = "writer"
	SourceTemplate = "template"

	ReasonRejected    = "rejected"    // the text failed Check
	ReasonTimeout     = "timeout"     // the writer did not answer in time
	ReasonError       = "error"       // the writer failed or answered badly
	ReasonUnavailable = "unavailable" // no writer is configured
	ReasonBudget      = "budget"      // the model spend cap is used up
)

// Outcomes of a call, as logged.
const (
	OutcomeOK       = "ok"
	OutcomeRejected = "rejected"
	OutcomeTimeout  = "timeout"
	OutcomeError    = "error"
)

// Enemy is the hero's win-rate delta against one enemy, in points.
type Enemy struct {
	Name  string  `json:"enemy"`
	Delta float64 `json:"delta"`
	Games int     `json:"games"`
}

// Brief is one hero's structured reasons, the language to write in, and
// the names a tip may use.
type Brief struct {
	HeroID       int
	Hero         string
	Lang         string // BCP 47 tag, e.g. en or pt-BR
	Position     int
	PositionName string
	Roles        []string
	RoleFit      float64
	WinRate      float64
	Enemies      []Enemy
	// Allowed are the names in the reasons: the hero's and its enemies', in
	// every language. Known are every hero, ability and item name the
	// database has; a tip naming one outside Allowed, in any case, fails.
	Allowed, Known []string
}

// Tip is a hero's tip, or a template the UI fills from the reasons.
type Tip struct {
	HeroID int    `json:"heroId"`
	Text   string `json:"text,omitempty"`
	Source string `json:"source"`
	Reason string `json:"reason,omitempty"`
}

// Template is the tip that stands in when no written one does.
func Template(heroID int, reason string) Tip {
	return Tip{HeroID: heroID, Source: SourceTemplate, Reason: reason}
}

// Call is one writer call, as logged in model_call.
type Call struct {
	Purpose       string
	Model         string
	AnsweredModel string
	Outcome       string
	Error         string
	Latency       time.Duration
	InputTokens   int
	OutputTokens  int
	Cost          *float64
	Output        string
}

// Logger records calls.
type Logger interface {
	Record(ctx context.Context, c Call)
}

// Writer writes tips with a chat model on OpenRouter.
type Writer struct {
	BaseURL, APIKey, Model string
	HTTP                   *http.Client
	Timeout                time.Duration
	Log                    Logger // nil logs nothing
}

// New builds a writer; log may be nil.
func New(baseURL, apiKey, model string, log Logger) *Writer {
	return &Writer{BaseURL: baseURL, APIKey: apiKey, Model: model, HTTP: http.DefaultClient, Timeout: DefaultTimeout, Log: log}
}

const tipInstructions = "You write one short tip for a player choosing a hero in a 5v5 hero draft, from the facts given " +
	"and nothing else. Write in the language whose BCP 47 tag is %s. One sentence of at most 140 characters that starts " +
	"with the hero's name; plain text, no quotes, no markdown. Name only the heroes in the facts, exactly as written there. Never name an item, an " +
	"ability or any other hero. Say why the hero fits: its win-rate deltas in points against the enemies, with the games " +
	"behind them, its patch win rate and how its roles fit the position."

// Tip writes b's tip. It never fails: a text that fails Check, a late
// answer and an error all give a template.
func (w *Writer) Tip(ctx context.Context, b Brief) Tip {
	timeout := w.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	start := time.Now()
	call := Call{Purpose: "draft.tip", Model: w.Model}
	text, err := w.complete(ctx, b, &call)
	call.Latency = time.Since(start)
	tip := Tip{HeroID: b.HeroID, Text: text, Source: SourceWriter}
	switch {
	case errors.Is(err, budget.ErrOverCap):
		return Template(b.HeroID, ReasonBudget) // no call made, nothing to log
	case errors.Is(err, context.DeadlineExceeded):
		call.Outcome, call.Error, tip = OutcomeTimeout, err.Error(), Template(b.HeroID, ReasonTimeout)
	case err != nil:
		call.Outcome, call.Error, tip = OutcomeError, err.Error(), Template(b.HeroID, ReasonError)
	default:
		call.Output = text
		if err := Check(text, b.Allowed, b.Known); err != nil {
			call.Outcome, call.Error, tip = OutcomeRejected, err.Error(), Template(b.HeroID, ReasonRejected)
		} else {
			call.Outcome = OutcomeOK
		}
	}
	// The tip is ready; a slow database must not hold the stream long.
	w.record(ctx, call)
	return tip
}

func (w *Writer) complete(ctx context.Context, b Brief, call *Call) (string, error) {
	facts, err := json.Marshal(map[string]any{
		"hero":            b.Hero,
		"position":        map[string]any{"number": b.Position, "name": b.PositionName},
		"roles":           b.Roles,
		"role_fit":        b.RoleFit,
		"patch_win_rate":  round1(b.WinRate),
		"against_enemies": roundEnemies(b.Enemies),
	})
	if err != nil {
		return "", err
	}
	return w.chat(ctx, fmt.Sprintf(tipInstructions, b.Lang), string(facts), 200, false, call)
}

// chat sends one system and one user message and returns the text answer;
// user is a string or a list of content parts. asJSON asks for a JSON object.
func (w *Writer) chat(ctx context.Context, system string, user any, maxTokens int, asJSON bool, call *Call) (string, error) {
	if err := budget.Check(ctx); err != nil {
		return "", err
	}
	payload := map[string]any{
		"model": w.Model,
		"messages": []any{
			map[string]any{"role": "system", "content": system},
			map[string]any{"role": "user", "content": user},
		},
		"max_tokens": maxTokens,
		"reasoning":  map[string]any{"effort": "minimal"},
		"provider":   map[string]any{"data_collection": "deny", "zdr": true},
		"usage":      map[string]any{"include": true},
	}
	if asJSON {
		payload["response_format"] = map[string]any{"type": "json_object"}
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(w.BaseURL, "/")+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+w.APIKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := w.HTTP.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return "", fmt.Errorf("writer: %w", ctx.Err())
		}
		return "", fmt.Errorf("writer: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		if ctx.Err() != nil {
			return "", fmt.Errorf("writer: %w", ctx.Err())
		}
		return "", fmt.Errorf("writer: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("writer: HTTP %d: %s", resp.StatusCode, snippet(w.redact(raw)))
	}
	var out struct {
		Model   string `json:"model"`
		Choices []struct {
			Message struct {
				Content *string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int      `json:"prompt_tokens"`
			CompletionTokens int      `json:"completion_tokens"`
			Cost             *float64 `json:"cost"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", fmt.Errorf("writer: answer is not JSON: %s", snippet(w.redact(raw)))
	}
	call.AnsweredModel, call.InputTokens, call.OutputTokens, call.Cost = out.Model, out.Usage.PromptTokens, out.Usage.CompletionTokens, out.Usage.Cost
	budget.Add(ctx, out.Usage.Cost)
	if len(out.Choices) == 0 || out.Choices[0].Message.Content == nil {
		return "", fmt.Errorf("writer: no text: %s", snippet(w.redact(raw)))
	}
	return strings.Trim(strings.TrimSpace(*out.Choices[0].Message.Content), `"“”`), nil
}

// Check holds a tip to the facts it was written from: it must have text,
// fit in MaxTip, name no known hero or ability outside allowed (in any
// case), and hold no other capitalised word. The writer is asked for one
// sentence that starts with the hero's name, so a capital anywhere else is
// a name the database may lack (an item, an old hero) and fails the tip.
func Check(text string, allowed, known []string) error {
	text = strings.TrimSpace(text)
	switch n := utf8.RuneCountInString(text); {
	case n == 0:
		return errors.New("no text")
	case n > MaxTip:
		return fmt.Errorf("%d characters, over %d", n, MaxTip)
	}
	masked, err := maskAndMatch(text, allowed, known)
	if err != nil {
		return err
	}
	for _, word := range strings.Fields(masked) {
		core := strings.TrimLeftFunc(word, isNotWord)
		if r, _ := utf8.DecodeRuneInString(core); unicode.IsUpper(r) {
			return fmt.Errorf("names %q, which is not in the result", strings.TrimRightFunc(core, isNotWord))
		}
	}
	return nil
}

// maskAndMatch masks the allowed names in text and fails when it still
// names a known one, in any case.
func maskAndMatch(text string, allowed, known []string) (string, error) {
	// Longest first, so a name inside a longer one is masked with it.
	names := slices.Clone(allowed)
	slices.SortFunc(names, func(a, b string) int { return len(b) - len(a) })
	masked := text
	for _, name := range names {
		masked = replaceWord(masked, name, mask)
	}
	lower := strings.ToLower(masked)
	for _, name := range known {
		if name != "" && indexWord(lower, strings.ToLower(name)) >= 0 {
			return "", fmt.Errorf("names %q, which is not in the result", name)
		}
	}
	return masked, nil
}

// mask stands in for an allowed name; it is not a letter or a digit.
const mask = "\u2063"

func isNotWord(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }

// indexWord finds name in s as a whole word: no letter or digit touches it.
func indexWord(s, name string) int {
	for from := 0; from <= len(s); {
		i := strings.Index(s[from:], name)
		if i < 0 {
			return -1
		}
		i += from
		before, _ := utf8.DecodeLastRuneInString(s[:i])
		after, _ := utf8.DecodeRuneInString(s[i+len(name):])
		if (i == 0 || isNotWord(before)) && (i+len(name) == len(s) || isNotWord(after)) {
			return i
		}
		from = i + 1
	}
	return -1
}

func replaceWord(s, name, with string) string {
	if name == "" || strings.Contains(name, with) {
		return s
	}
	for i := indexWord(s, name); i >= 0; i = indexWord(s, name) {
		s = s[:i] + with + s[i+len(name):]
	}
	return s
}

// redact takes the API key out of a provider's answer before it is logged.
func (w *Writer) redact(b []byte) []byte {
	if w.APIKey == "" {
		return b
	}
	return bytes.ReplaceAll(b, []byte(w.APIKey), []byte("<REDACTED>"))
}

func roundEnemies(es []Enemy) []Enemy {
	out := make([]Enemy, len(es))
	for i, e := range es {
		out[i] = Enemy{Name: e.Name, Delta: round1(e.Delta), Games: e.Games}
	}
	return out
}

func round1(x float64) float64 { return math.Round(x*10) / 10 }

// snippet cuts a provider's answer short for errors and logs, on a rune boundary.
func snippet(b []byte) string {
	const limit = 300
	s := strings.ToValidUTF8(string(b), "")
	if len(s) <= limit {
		return s
	}
	cut := limit
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + "…"
}
