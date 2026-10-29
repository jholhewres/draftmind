package photo

import (
	"context"
	"encoding/json"
	"fmt"
	"image"
	"strconv"
	"strings"
)

// EndRead is what a photo of the end-of-match screen tells, for the player
// to confirm: the result for the player's team, the duration and the
// player's KDA. A field the photo did not tell is left out.
type EndRead struct {
	Result    string   `json:"result,omitempty"` // win or loss
	DurationS *int     `json:"durationS,omitempty"`
	Kills     *int     `json:"kills,omitempty"`
	Deaths    *int     `json:"deaths,omitempty"`
	Assists   *int     `json:"assists,omitempty"`
	Cost      *float64 `json:"-"`
}

// EndReader reads the end-of-match screen; hero is the player's hero name.
type EndReader interface {
	ReadEnd(ctx context.Context, img image.Image, hero string) (EndRead, error)
}

// MaxStat bounds a kill, death or assist count read off a photo.
const MaxStat = 999

const endPrompt = "This is a photo or screenshot of a game's end-of-match screen for a 5v5 hero match between the radiant " +
	"and the dire. Read which team won (winner: radiant, dire, or unknown when the screen does not say), the match " +
	"duration as shown, e.g. 38:12 or 1:02:05 (empty when not shown), and the row of the hero named %q: found is false " +
	"when no row shows that hero; team is the team whose table holds the row; kills, deaths and assists are the K, D " +
	"and A numbers of that row. Read only what the screen shows."

var endSchema = map[string]any{
	"type": "object", "additionalProperties": false, "required": []string{"winner", "duration", "player"},
	"properties": map[string]any{
		"winner":   map[string]any{"type": "string", "enum": []string{"radiant", "dire", "unknown"}},
		"duration": map[string]any{"type": "string"},
		"player": map[string]any{
			"type": "object", "additionalProperties": false,
			"required": []string{"found", "team", "kills", "deaths", "assists"},
			"properties": map[string]any{
				"found":   map[string]any{"type": "boolean"},
				"team":    map[string]any{"type": "string", "enum": []string{"radiant", "dire", "unknown"}},
				"kills":   map[string]any{"type": "integer"},
				"deaths":  map[string]any{"type": "integer"},
				"assists": map[string]any{"type": "integer"},
			},
		},
	},
}

// ReadEnd reads the end screen with the vision model (ADR-0006, amended in
// B4): the model reads the text, and code turns the winner and the
// player's team into the result.
func (v *Vision) ReadEnd(ctx context.Context, img image.Image, hero string) (EndRead, error) {
	raw, err := v.ask(ctx, img, fmt.Sprintf(endPrompt, hero), "end_screen", endSchema, 300)
	if err != nil {
		return EndRead{}, err
	}
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
		return EndRead{}, fmt.Errorf("photo: end screen: no answer: %s", v.snippet(raw))
	}
	var a struct {
		Winner   string `json:"winner"`
		Duration string `json:"duration"`
		Player   struct {
			Found                  bool   `json:"found"`
			Team                   string `json:"team"`
			Kills, Deaths, Assists int
		} `json:"player"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(w.Choices[0].Message.Content)), &a); err != nil {
		return EndRead{}, fmt.Errorf("photo: end screen: answer is not the schema: %w", err)
	}
	out := EndRead{Cost: w.Usage.Cost, DurationS: parseClock(a.Duration)}
	p := a.Player
	known := func(team string) bool { return team == "radiant" || team == "dire" }
	if p.Found && known(p.Team) && known(a.Winner) {
		out.Result = "loss"
		if p.Team == a.Winner {
			out.Result = "win"
		}
	}
	stat := func(n int) bool { return n >= 0 && n <= MaxStat }
	if p.Found && stat(p.Kills) && stat(p.Deaths) && stat(p.Assists) {
		out.Kills, out.Deaths, out.Assists = &p.Kills, &p.Deaths, &p.Assists
	}
	return out, nil
}

// parseClock reads m:ss or h:mm:ss into seconds; nil when it is neither or
// over 4 hours.
func parseClock(s string) *int {
	parts := strings.Split(strings.TrimSpace(s), ":")
	if len(parts) < 2 || len(parts) > 3 {
		return nil
	}
	total := 0
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 || (i > 0 && (n > 59 || len(p) != 2)) {
			return nil
		}
		total = total*60 + n
	}
	if total == 0 || total > 4*3600 {
		return nil
	}
	return &total
}
