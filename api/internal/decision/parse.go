package decision

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
)

// validQuestions rejects a question the endpoint could not answer in a
// form parse can check.
func validQuestions(qs map[string]Question) error {
	if len(qs) == 0 {
		return errors.New("no questions")
	}
	for name, q := range qs {
		switch q.Type {
		case "noul":
		case "choice":
			if opts, ok := q.Criteria.(map[string]any); !ok || len(opts) == 0 {
				return fmt.Errorf("choice %q needs options", name)
			}
		case "score":
			if levels, ok := q.Criteria.([]any); !ok || len(levels) == 0 {
				return fmt.Errorf("score %q needs levels", name)
			}
		default:
			return fmt.Errorf("question %q has unknown type %q", name, q.Type)
		}
	}
	return nil
}

type wireResponse struct {
	Model   string                     `json:"model"`
	Answers map[string]json.RawMessage `json:"answers"`
	Usage   json.RawMessage            `json:"usage"`
}

// wireAnswer has pointers so that a missing field is told from a zero.
type wireAnswer struct {
	Type          *string             `json:"type"`
	Choice        *string             `json:"choice"`
	Noul          *float64            `json:"noul"`
	Score         *float64            `json:"score"`
	Confidence    *float64            `json:"confidence"`
	Probabilities map[string]*float64 `json:"probabilities"` // a null is not a 0
}

// parse reads a 200 answer: every question asked must come back with its
// own type and every field that type requires, else the whole call fails.
// Usage is best effort: a missing or odd usage never fails a call.
func parse(raw []byte, qs map[string]Question) (Result, error) {
	var w wireResponse
	if err := json.Unmarshal(raw, &w); err != nil {
		return Result{}, fmt.Errorf("decision: answer is not JSON: %w", err)
	}
	res := Result{Model: w.Model, Answers: make(map[string]Answer, len(qs)), Usage: parseUsage(w.Usage)}
	for name, q := range qs {
		rawA, ok := w.Answers[name]
		if !ok {
			return Result{}, fmt.Errorf("decision: no answer to %q", name)
		}
		a, err := parseAnswer(rawA, q)
		if err != nil {
			return Result{}, fmt.Errorf("decision: answer to %q: %w", name, err)
		}
		res.Answers[name] = a
	}
	return res, nil
}

func parseAnswer(raw json.RawMessage, q Question) (Answer, error) {
	var w wireAnswer
	if err := json.Unmarshal(raw, &w); err != nil {
		return Answer{}, err
	}
	if w.Type == nil || *w.Type != q.Type {
		return Answer{}, fmt.Errorf("type %v, want %q", deref(w.Type), q.Type)
	}
	a := Answer{Type: q.Type}
	switch q.Type {
	case "noul":
		if w.Noul == nil || !unit(*w.Noul) {
			return Answer{}, errors.New("noul missing or outside 0..1")
		}
		a.Noul = *w.Noul
		return a, nil
	case "choice":
		opts := q.Criteria.(map[string]any)
		if w.Choice == nil {
			return Answer{}, errors.New("choice missing")
		}
		if _, ok := opts[*w.Choice]; !ok {
			return Answer{}, fmt.Errorf("choice %q is not an option", *w.Choice)
		}
		a.Choice = *w.Choice
		keys := make([]string, 0, len(opts))
		for k := range opts {
			keys = append(keys, k)
		}
		return a, probabilities(&a, w, keys)
	case "score":
		if w.Score == nil || math.IsNaN(*w.Score) || math.IsInf(*w.Score, 0) {
			return Answer{}, errors.New("score missing")
		}
		a.Score = *w.Score
		levels := q.Criteria.([]any)
		keys := make([]string, len(levels))
		for i := range levels {
			keys[i] = strconv.Itoa(i)
		}
		return a, probabilities(&a, w, keys)
	}
	return Answer{}, fmt.Errorf("unknown type %q", q.Type)
}

// probabilities checks the confidence and that each key, and only those,
// has a probability in 0..1.
func probabilities(a *Answer, w wireAnswer, keys []string) error {
	if w.Confidence == nil || !unit(*w.Confidence) {
		return errors.New("confidence missing or outside 0..1")
	}
	if len(w.Probabilities) != len(keys) {
		return fmt.Errorf("%d probabilities for %d options", len(w.Probabilities), len(keys))
	}
	a.Probabilities = make(map[string]float64, len(keys))
	for _, k := range keys {
		p := w.Probabilities[k]
		if p == nil || !unit(*p) {
			return fmt.Errorf("probability of %q missing or outside 0..1", k)
		}
		a.Probabilities[k] = *p
	}
	a.Confidence = *w.Confidence
	return nil
}

func unit(x float64) bool { return x >= 0 && x <= 1 }

func deref(s *string) any {
	if s == nil {
		return nil
	}
	return *s
}

// parseUsage reads TypeSafe's input/output tokens or OpenRouter's
// prompt/completion tokens, and OpenRouter's cost in USD when present.
func parseUsage(raw json.RawMessage) Usage {
	var w struct {
		InputTokens      *int     `json:"input_tokens"`
		OutputTokens     *int     `json:"output_tokens"`
		PromptTokens     *int     `json:"prompt_tokens"`
		CompletionTokens *int     `json:"completion_tokens"`
		Cost             *float64 `json:"cost"`
	}
	if len(raw) == 0 || json.Unmarshal(raw, &w) != nil {
		return Usage{}
	}
	u := Usage{Cost: w.Cost}
	for _, p := range []struct {
		dst  *int
		srcs []*int
	}{{&u.InputTokens, []*int{w.InputTokens, w.PromptTokens}}, {&u.OutputTokens, []*int{w.OutputTokens, w.CompletionTokens}}} {
		for _, s := range p.srcs {
			if s != nil {
				*p.dst = *s
				break
			}
		}
	}
	return u
}
