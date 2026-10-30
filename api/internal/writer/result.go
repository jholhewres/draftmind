package writer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jholhewres/draftmind/api/internal/budget"
	"github.com/jholhewres/draftmind/api/internal/persona"
)

// Results a match can end with.
var Results = []string{"win", "loss"}

const resultInstructions = "You read one message a player sends at the end of a 5v5 hero match, in any language, saying " +
	`how it ended. Answer with one JSON object and nothing else: {"result": "win"|"loss"|"unclear"}. ` +
	"win when the player's team won, loss when it lost, unclear when the message does not say."

// ParseResult reads a message such as "we won" or "perdemos" into win or
// loss. A message that does not say, or an answer that is not the JSON
// asked for, returns an error wrapping ErrUnclear; a model error or a late
// answer returns the cause.
func (w *Writer) ParseResult(ctx context.Context, text string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, CoachTimeout)
	defer cancel()
	start := time.Now()
	call := Call{Purpose: "match.result", Model: w.Model}
	raw, err := w.chat(ctx, resultInstructions, text, 60, true, &call)
	call.Latency, call.Output = time.Since(start), raw
	result := ""
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		call.Outcome, call.Error = OutcomeTimeout, err.Error()
	case err != nil:
		call.Outcome, call.Error = OutcomeError, err.Error()
	default:
		var a struct {
			Result string `json:"result"`
		}
		if jerr := json.Unmarshal([]byte(stripFence(raw)), &a); jerr != nil || (a.Result != "win" && a.Result != "loss") {
			err = fmt.Errorf("%w: %q", ErrUnclear, cut(raw, 80))
			call.Outcome, call.Error = OutcomeRejected, err.Error()
		} else {
			result, call.Outcome = a.Result, OutcomeOK
		}
	}
	w.record(ctx, call)
	return result, err
}

// NotesBrief is what the coach's end-of-match notes are written from: the
// result, the session's chat and facts, the language, and the names the
// notes may use.
type NotesBrief struct {
	Lang     string
	Persona  persona.Persona // shapes the wording only
	Result   string
	Messages []string // "player: …" and "coach: …" lines, oldest first
	Facts    any
	// Allowed are the names in the facts; Known every name the database has.
	Allowed, Known []string
}

// Notes are the coach's notes on a match; with none written, Source is
// template and Reason says why.
type Notes struct {
	Items  []string `json:"items"`
	Source string   `json:"source"`
	Reason string   `json:"reason,omitempty"`
}

// MaxNotes and MaxNote bound the notes kept.
const (
	MaxNotes = 3
	MaxNote  = 180
)

const notesInstructions = "You are a concise coach. A 5v5 hero match just ended. From the facts and the chat given, and " +
	"nothing else, write two or three short notes for the player to remember, in the language whose BCP 47 tag is %s: " +
	"what went well or badly and one thing to try next time. Each note is one plain sentence under 160 characters, no " +
	`markdown. Answer with one JSON object and nothing else: {"notes": [string]}. Name only the heroes, abilities and ` +
	"items in the facts, exactly as written there, and never anything else. Start each sentence with a capital letter " +
	"and write every other word that is not one of those names in lower case. %s"

// Notes writes two or three notes on the match. It never fails: each note
// is held to the facts like an answer and dropped when it fails; with none
// left, or on an error or a late answer, it returns no notes.
func (w *Writer) Notes(ctx context.Context, b NotesBrief) Notes {
	ctx, cancel := context.WithTimeout(ctx, CoachTimeout)
	defer cancel()
	start := time.Now()
	call := Call{Purpose: "match.notes", Model: w.Model}
	input, err := json.Marshal(map[string]any{"result": b.Result, "chat": b.Messages, "facts": b.Facts})
	raw := ""
	if err == nil {
		raw, err = w.chat(ctx, fmt.Sprintf(notesInstructions, b.Lang, b.Persona.Voice()), string(input), 500, true, &call)
	}
	call.Latency, call.Output = time.Since(start), raw
	out := Notes{Items: []string{}, Source: SourceTemplate}
	switch {
	case errors.Is(err, budget.ErrOverCap):
		out.Reason = ReasonBudget
		return out
	case errors.Is(err, context.DeadlineExceeded):
		call.Outcome, call.Error, out.Reason = OutcomeTimeout, err.Error(), ReasonTimeout
	case err != nil:
		call.Outcome, call.Error, out.Reason = OutcomeError, err.Error(), ReasonError
	default:
		var a struct {
			Notes []string `json:"notes"`
		}
		if jerr := json.Unmarshal([]byte(stripFence(raw)), &a); jerr != nil {
			call.Outcome, call.Error, out.Reason = OutcomeError, jerr.Error(), ReasonError
			break
		}
		var failed []string
		for _, n := range a.Notes {
			n = strings.TrimSpace(n)
			if err := checkSentences(n, MaxNote, b.Allowed, b.Known); err != nil {
				failed = append(failed, err.Error())
				continue
			}
			if len(out.Items) < MaxNotes {
				out.Items = append(out.Items, n)
			}
		}
		if len(out.Items) == 0 {
			call.Outcome, call.Error, out.Reason = OutcomeRejected, strings.Join(failed, "; "), ReasonRejected
		} else {
			call.Outcome, out.Source = OutcomeOK, SourceWriter
			if len(failed) > 0 {
				call.Error = "dropped: " + strings.Join(failed, "; ")
			}
		}
	}
	w.record(ctx, call)
	return out
}

// stripFence takes a ```json fence off a model's JSON answer.
func stripFence(raw string) string {
	return strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(raw), "```json"), "```"))
}

func cut(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
