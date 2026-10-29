package photo

import (
	"context"
	"encoding/json"
	"image"
	_ "image/jpeg"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
)

func endScreenFixture(t *testing.T) image.Image {
	t.Helper()
	f, err := os.Open("testdata/endscreen.jpg")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	img, _, err := image.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	return img
}

// answerWith is a chat completion whose content is v as JSON.
func answerWith(t *testing.T, w http.ResponseWriter, v any) {
	t.Helper()
	content, _ := json.Marshal(v)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"choices": []any{map[string]any{"message": map[string]any{"content": string(content)}}},
		"usage":   map[string]any{"cost": 0.0009},
	})
}

func TestEndScreenReadFillsResultDurationAndKDAOfThePlayersHero(t *testing.T) {
	var prompt string
	var sentImage bool
	v := newVision(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		prompt = string(body)
		sentImage = strings.Contains(prompt, `data:image/jpeg;base64,`)
		// What the model reads off testdata/endscreen.jpg: radiant won in 38:12,
		// Juggernaut is on radiant at 9/3/14.
		answerWith(t, w, map[string]any{
			"winner": "radiant", "duration": "38:12",
			"player": map[string]any{"found": true, "team": "radiant", "kills": 9, "deaths": 3, "assists": 14},
		})
	})

	got, err := v.ReadEnd(context.Background(), endScreenFixture(t), "Juggernaut")
	if err != nil {
		t.Fatal(err)
	}
	if !sentImage || !strings.Contains(prompt, "Juggernaut") || !strings.Contains(prompt, `"json_schema"`) {
		t.Fatalf("request lacks the photo, the hero or the schema")
	}
	want := EndRead{Result: "win", DurationS: ptr(2292), Kills: ptr(9), Deaths: ptr(3), Assists: ptr(14)}
	if got.Result != want.Result || !same(got.DurationS, want.DurationS) || !same(got.Kills, want.Kills) ||
		!same(got.Deaths, want.Deaths) || !same(got.Assists, want.Assists) {
		t.Fatalf("read %+v, want %+v", got, want)
	}
}

func TestEndScreenReadLeavesBlankWhatItCannotTell(t *testing.T) {
	for _, c := range []struct {
		name   string
		answer map[string]any
		want   EndRead
	}{
		{"the player's team lost", map[string]any{"winner": "dire", "duration": "1:02:05",
			"player": map[string]any{"found": true, "team": "radiant", "kills": 1, "deaths": 12, "assists": 4}},
			EndRead{Result: "loss", DurationS: ptr(3725), Kills: ptr(1), Deaths: ptr(12), Assists: ptr(4)}},
		{"the hero is not on the screen", map[string]any{"winner": "radiant", "duration": "nonsense",
			"player": map[string]any{"found": false, "team": "unknown", "kills": 0, "deaths": 0, "assists": 0}},
			EndRead{}},
		{"numbers out of range", map[string]any{"winner": "unknown", "duration": "",
			"player": map[string]any{"found": true, "team": "dire", "kills": -1, "deaths": 5000, "assists": 3}},
			EndRead{}},
	} {
		v := newVision(t, func(w http.ResponseWriter, _ *http.Request) { answerWith(t, w, c.answer) })
		got, err := v.ReadEnd(context.Background(), endScreenFixture(t), "Juggernaut")
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if got.Result != c.want.Result || !same(got.DurationS, c.want.DurationS) || !same(got.Kills, c.want.Kills) ||
			!same(got.Deaths, c.want.Deaths) || !same(got.Assists, c.want.Assists) {
			t.Errorf("%s: read %+v, want %+v", c.name, got, c.want)
		}
	}
}

func ptr(n int) *int { return &n }

func same(a, b *int) bool { return (a == nil) == (b == nil) && (a == nil || *a == *b) }
