package photo

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"io"
	"net/http"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jholhewres/draftmind/api/internal/budget"
	"github.com/jholhewres/draftmind/api/internal/catalog"
)

// Vision reads the draft with a vision model on OpenRouter's chat
// completions API (ADR-0006), its answer held to a JSON schema.
type Vision struct {
	BaseURL, APIKey, Model string
	HTTP                   *http.Client
	// MinConfidence is the least confidence a slot's answer needs to stand.
	MinConfidence float64
	// TrustNames keeps the heroes the model names. Off, every hero it names is
	// flagged for the player: on the bench it gave a wrong hero 0.95, the same
	// as a right one (ADR-0006), so its confidence cannot gate a guess.
	TrustNames bool
}

// Name names the reader in reads and logs.
func (v *Vision) Name() string { return "vision" }

const visionPrompt = "This is a phone photo of a monitor showing a game's hero pick screen. Along the top of the screen " +
	"is a row of hero portraits: five for the radiant team left of the clock, five for the dire team right of it. " +
	"For each slot, left to right, give the hero's name exactly as in the list below, or set empty when the slot has " +
	"no hero yet. Confidence, from 0 to 1, is how sure you are of what the slot holds, a hero or empty. When you are " +
	"not sure which hero it is, leave hero blank and give a low confidence rather than guess. Heroes: "

var visionSchema = func() map[string]any {
	slot := map[string]any{
		"type": "object", "additionalProperties": false, "required": []string{"hero", "empty", "confidence"},
		"properties": map[string]any{
			"hero": map[string]any{"type": "string"}, "empty": map[string]any{"type": "boolean"},
			"confidence": map[string]any{"type": "number"},
		},
	}
	team := map[string]any{"type": "array", "minItems": PerSide, "maxItems": PerSide, "items": slot}
	return map[string]any{
		"type": "object", "additionalProperties": false, "required": []string{"radiant", "dire"},
		"properties": map[string]any{"radiant": team, "dire": team},
	}
}()

// Read reads img's draft.
func (v *Vision) Read(ctx context.Context, img image.Image, heroes []catalog.Hero) (Read, error) {
	names := make([]string, len(heroes))
	for i, h := range heroes {
		names[i] = h.Name
	}
	slices.Sort(names)
	raw, err := v.ask(ctx, img, visionPrompt+strings.Join(names, ", "), "draft", visionSchema, 800)
	if err != nil {
		return Read{}, err
	}
	return v.parse(raw, heroes)
}

// ask sends img with the prompt and returns the provider's body, the
// answer held to schema.
func (v *Vision) ask(ctx context.Context, img image.Image, prompt, name string, schema map[string]any, maxTokens int) ([]byte, error) {
	if err := budget.Check(ctx); err != nil {
		return nil, err
	}
	photo := encodeJPEG(toRGB(img).forJev(), qualityPhoto) // the same size limits suit the vision model
	body, err := json.Marshal(map[string]any{
		"model": v.Model,
		"messages": []any{map[string]any{"role": "user", "content": []any{
			map[string]any{"type": "text", "text": prompt},
			map[string]any{"type": "image_url", "image_url": map[string]string{"url": "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(photo)}},
		}}},
		"response_format": map[string]any{"type": "json_schema", "json_schema": map[string]any{"name": name, "strict": true, "schema": schema}},
		"temperature":     0,
		"max_tokens":      maxTokens,
		"reasoning":       map[string]any{"effort": "minimal"},
		"provider":        map[string]any{"data_collection": "deny", "require_parameters": true, "zdr": true},
		"usage":           map[string]any{"include": true},
	})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(v.BaseURL, "/")+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+v.APIKey)
	req.Header.Set("Content-Type", "application/json")
	start := time.Now()
	resp, err := v.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("photo: vision: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("photo: vision: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("photo: vision: HTTP %d after %v: %s", resp.StatusCode, time.Since(start).Round(time.Millisecond), v.snippet(raw))
	}
	var usage struct {
		Usage struct {
			Cost *float64 `json:"cost"`
		} `json:"usage"`
	}
	if json.Unmarshal(raw, &usage) == nil {
		budget.Add(ctx, usage.Usage.Cost)
	}
	return raw, nil
}

func (v *Vision) parse(raw []byte, heroes []catalog.Hero) (Read, error) {
	var w struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			Cost *float64 `json:"cost"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(raw, &w); err != nil || len(w.Choices) == 0 {
		return Read{}, fmt.Errorf("photo: vision: no answer: %s", v.snippet(raw))
	}
	type slot struct {
		Hero       *string  `json:"hero"`
		Empty      *bool    `json:"empty"`
		Confidence *float64 `json:"confidence"`
	}
	var d struct {
		Radiant []slot `json:"radiant"`
		Dire    []slot `json:"dire"`
	}
	content := strings.TrimSpace(w.Choices[0].Message.Content)
	if err := json.Unmarshal([]byte(content), &d); err != nil {
		return Read{}, fmt.Errorf("photo: vision: answer is not the draft: %w", err)
	}
	if len(d.Radiant) != PerSide || len(d.Dire) != PerSide {
		return Read{}, fmt.Errorf("photo: vision: %d radiant and %d dire slots", len(d.Radiant), len(d.Dire))
	}
	var g [Slots]Guess
	for i, s := range append(d.Radiant, d.Dire...) {
		if s.Hero == nil || s.Empty == nil || s.Confidence == nil || !unit(*s.Confidence) {
			return Read{}, fmt.Errorf("photo: vision: slot %d is incomplete or its confidence is outside 0..1", i)
		}
		g[i] = Guess{Name: *s.Hero, Empty: *s.Empty, Confidence: *s.Confidence}
		if !v.TrustNames && !g[i].Empty {
			g[i].Name = ""
		}
	}
	r := Resolve(g, heroes, v.MinConfidence)
	r.Reader, r.Cost = v.Name(), w.Usage.Cost
	return r, nil
}

func unit(x float64) bool { return x >= 0 && x <= 1 }

// snippet is the start of a provider's body, without the API key.
func (v *Vision) snippet(b []byte) string {
	s := strings.ToValidUTF8(string(b), "�")
	if v.APIKey != "" {
		s = strings.ReplaceAll(s, v.APIKey, "<REDACTED>")
	}
	if len(s) > 300 {
		cut := 300
		for cut > 0 && !utf8.RuneStart(s[cut]) {
			cut--
		}
		s = s[:cut] + "…"
	}
	return s
}
