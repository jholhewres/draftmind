package writer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// Categories a player's message can be about; other is anything else the
// coach can still answer.
var Categories = []string{"lane", "teamfight", "items", "objective", "other"}

// Situation is a player's message as the writer model reads it.
type Situation struct {
	// Clear is false when the message says too little to advise on.
	Clear    bool     `json:"clear"`
	Category string   `json:"category"`
	Entities []string `json:"entities"`
	// Summary restates the message in a short English sentence.
	Summary string `json:"summary"`
}

// ErrUnclear means the message could not be read into a situation.
var ErrUnclear = errors.New("the message could not be read into a situation")

// CoachTimeout bounds one parse or answer call during a match.
const CoachTimeout = 8 * time.Second

// MaxAnswer is the longest coach answer, in characters, that stands.
const MaxAnswer = 360

const parseInstructions = "You read one message a player sends their coach during a 5v5 hero match. " +
	"Answer with one JSON object and nothing else: " +
	`{"clear": bool, "category": "lane"|"teamfight"|"items"|"objective"|"other", "entities": [string], "summary": string}. ` +
	"clear is false when the message gives too little to advise on (a greeting, a single vague word, noise). " +
	"entities are the hero, ability and item names the message mentions, spelled as in the game. " +
	"summary restates the situation in one short English sentence."

// Situation reads text into a typed situation. An answer that is not the
// JSON asked for, or a message the model marks unclear, returns an error
// wrapping ErrUnclear; a model error or a late answer returns the cause.
func (w *Writer) Situation(ctx context.Context, text string) (Situation, error) {
	ctx, cancel := context.WithTimeout(ctx, CoachTimeout)
	defer cancel()
	start := time.Now()
	call := Call{Purpose: "coach.parse", Model: w.Model}
	raw, err := w.chat(ctx, parseInstructions, text, 300, true, &call)
	call.Latency = time.Since(start)
	call.Output = raw
	var sit Situation
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		call.Outcome, call.Error = OutcomeTimeout, err.Error()
	case err != nil:
		call.Outcome, call.Error = OutcomeError, err.Error()
	default:
		if perr := parseSituation(raw, &sit); perr != nil {
			err = perr
			call.Outcome, call.Error = OutcomeRejected, perr.Error()
		} else {
			call.Outcome = OutcomeOK
		}
	}
	w.record(ctx, call)
	return sit, err
}

// parseSituation holds the model's JSON to the shape asked for.
func parseSituation(raw string, sit *Situation) error {
	raw = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(raw), "```json"), "```"))
	dec := json.NewDecoder(strings.NewReader(raw))
	dec.DisallowUnknownFields()
	var s struct {
		Clear    *bool    `json:"clear"`
		Category string   `json:"category"`
		Entities []string `json:"entities"`
		Summary  string   `json:"summary"`
	}
	if err := dec.Decode(&s); err != nil {
		return fmt.Errorf("%w: %w", ErrUnclear, err)
	}
	if s.Clear == nil || !slices.Contains(Categories, s.Category) {
		return fmt.Errorf("%w: no clear flag or an unknown category %q", ErrUnclear, s.Category)
	}
	if !*s.Clear {
		return ErrUnclear
	}
	*sit = Situation{Clear: true, Category: s.Category, Entities: s.Entities, Summary: s.Summary}
	if sit.Entities == nil {
		sit.Entities = []string{}
	}
	return nil
}

// AnswerBrief is what a coach answer is written from: the facts, the
// player's message, the decided category and priority, the language, and
// the names the answer may use.
type AnswerBrief struct {
	Lang     string
	Message  string
	Category string
	Priority string
	Facts    any
	// Allowed are the names in the facts; Known every hero, ability and item
	// name the database has. An answer naming one outside Allowed fails.
	Allowed, Known []string
}

// Answer is the coach's text, or a template the UI words when the text
// failed the check, was late or errored.
type Answer struct {
	Text   string `json:"text,omitempty"`
	Source string `json:"source"`
	Reason string `json:"reason,omitempty"`
}

const answerInstructions = "You are a concise coach for a player in a live 5v5 hero match. Answer their message from " +
	"the facts given and nothing else, in the language whose BCP 47 tag is %s. At most three short sentences, under 300 " +
	"characters, plain text, no markdown, no lists. Focus on the decided category; do not restate the category or the " +
	"priority. Name only the heroes, abilities and items in the facts, exactly as written there, and never anything else. " +
	"Start each sentence with a capital letter and write every other word that is not one of those names in lower case."

// Answer writes the coach's answer. It never fails: a text that fails
// CheckAnswer, a late answer and an error give a template.
func (w *Writer) Answer(ctx context.Context, b AnswerBrief) Answer {
	ctx, cancel := context.WithTimeout(ctx, CoachTimeout)
	defer cancel()
	start := time.Now()
	call := Call{Purpose: "coach.answer", Model: w.Model}
	facts, err := json.Marshal(map[string]any{
		"player_message": b.Message,
		"decided":        map[string]any{"category": b.Category, "priority": b.Priority},
		"facts":          b.Facts,
	})
	text := ""
	if err == nil {
		text, err = w.chat(ctx, fmt.Sprintf(answerInstructions, b.Lang), string(facts), 400, false, &call)
	}
	call.Latency = time.Since(start)
	a := Answer{Text: text, Source: SourceWriter}
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		call.Outcome, call.Error, a = OutcomeTimeout, err.Error(), Answer{Source: SourceTemplate, Reason: ReasonTimeout}
	case err != nil:
		call.Outcome, call.Error, a = OutcomeError, err.Error(), Answer{Source: SourceTemplate, Reason: ReasonError}
	default:
		call.Output = text
		if err := CheckAnswer(text, b.Allowed, b.Known); err != nil {
			call.Outcome, call.Error, a = OutcomeRejected, err.Error(), Answer{Source: SourceTemplate, Reason: ReasonRejected}
		} else {
			call.Outcome = OutcomeOK
		}
	}
	w.record(ctx, call)
	return a
}

func (w *Writer) record(ctx context.Context, c Call) {
	if w.Log == nil {
		return
	}
	logCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer cancel()
	w.Log.Record(logCtx, c)
}

// CheckAnswer holds a coach answer to its facts as Check holds a tip, with
// room for a few sentences: a capital is allowed only where a sentence
// starts, so any other capitalised word is a name the facts lack.
func CheckAnswer(text string, allowed, known []string) error {
	return checkSentences(text, MaxAnswer, allowed, known)
}

// checkSentences holds text of at most limit characters to the allowed
// names, with capitals only where a sentence starts.
func checkSentences(text string, limit int, allowed, known []string) error {
	text = strings.TrimSpace(text)
	switch n := utf8.RuneCountInString(text); {
	case n == 0:
		return errors.New("no text")
	case n > limit:
		return fmt.Errorf("%d characters, over %d", n, limit)
	}
	masked, err := maskAndMatch(text, allowed, known)
	if err != nil {
		return err
	}
	start := true
	for _, word := range strings.Fields(masked) {
		core := strings.TrimLeftFunc(word, isNotWord)
		if r, _ := utf8.DecodeRuneInString(core); unicode.IsUpper(r) && !start {
			return fmt.Errorf("names %q, which is not in the facts", strings.TrimRightFunc(core, isNotWord))
		}
		last, _ := utf8.DecodeLastRuneInString(strings.TrimRight(word, `"'”’)»`))
		start = last == '.' || last == '!' || last == '?'
	}
	return nil
}
