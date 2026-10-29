package photo

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image/color"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/jholhewres/draftmind/api/internal/catalog"
)

// visionHeroes are the heroes the recorded answer names, on the current patch
// except Clockwerk.
var visionHeroes = []catalog.Hero{
	{ID: 1, Name: "Anti-Mage"}, {ID: 13, Name: "Puck"}, {ID: 16, Name: "Sand King"},
	{ID: 87, Name: "Disruptor"}, {ID: 27, Name: "Shadow Shaman"}, {ID: 91, Name: "Io"},
	{ID: 67, Name: "Spectre"}, {ID: 106, Name: "Ember Spirit"},
}

func newVision(t *testing.T, h http.HandlerFunc) *Vision {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return &Vision{BaseURL: srv.URL, APIKey: "sk-or-test-key-not-real", Model: "google/gemini-3.1-flash-lite", HTTP: srv.Client(), MinConfidence: 0.5}
}

func TestVisionSendsThePhotoWithAJSONSchemaAndResolvesTheNames(t *testing.T) {
	recorded, err := os.ReadFile("testdata/openrouter-vision.json")
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Model    string `json:"model"`
		Messages []struct {
			Content []struct {
				Type     string `json:"type"`
				Text     string `json:"text"`
				ImageURL struct {
					URL string `json:"url"`
				} `json:"image_url"`
			} `json:"content"`
		} `json:"messages"`
		ResponseFormat struct {
			Type       string `json:"type"`
			JSONSchema struct {
				Strict bool `json:"strict"`
			} `json:"json_schema"`
		} `json:"response_format"`
	}
	var path, auth string
	v := newVision(t, func(w http.ResponseWriter, r *http.Request) {
		path, auth = r.URL.Path, r.Header.Get("Authorization")
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &got)
		_, _ = w.Write(recorded)
	})
	// Clockwerk is not on this patch, so the slot read as Clockwerk is flagged.
	r, err := v.Read(context.Background(), solid(1600, 900, color.RGBA{20, 20, 20, 255}), visionHeroes)
	if err != nil {
		t.Fatal(err)
	}
	if path != "/v1/chat/completions" || auth != "Bearer sk-or-test-key-not-real" {
		t.Fatalf("path %q, auth set %t", path, auth != "")
	}
	if got.Model != "google/gemini-3.1-flash-lite" || got.ResponseFormat.Type != "json_schema" || !got.ResponseFormat.JSONSchema.Strict {
		t.Fatalf("request model %q, format %+v", got.Model, got.ResponseFormat)
	}
	parts := got.Messages[len(got.Messages)-1].Content
	var text, image string
	for _, p := range parts {
		text += p.Text
		image += p.ImageURL.URL
	}
	if !strings.Contains(text, "Shadow Shaman") || !strings.HasPrefix(image, "data:image/jpeg;base64,") {
		t.Fatalf("prompt lists no heroes or sends no JPEG: %.80q %.40q", text, image)
	}
	jpg, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(image, "data:image/jpeg;base64,"))
	if err != nil || len(jpg) == 0 {
		t.Fatalf("image is not base64: %v", err)
	}
	want := []Slot{
		{Status: StatusUnknown, Confidence: 0.95}, // Clockwerk: not a hero of this patch
		{HeroID: 1, Status: StatusHero, Confidence: 0.95}, {HeroID: 13, Status: StatusHero, Confidence: 0.95},
		{HeroID: 16, Status: StatusHero, Confidence: 0.95},
		{Status: StatusUnknown, Confidence: 0}, // empty, but with no confidence: flagged
		{HeroID: 87, Status: StatusHero, Confidence: 0.95}, {HeroID: 27, Status: StatusHero, Confidence: 0.95},
		{HeroID: 91, Status: StatusHero, Confidence: 0.95}, {HeroID: 67, Status: StatusHero, Confidence: 0.95},
		{HeroID: 106, Status: StatusHero, Confidence: 0.95},
	}
	slots := append(append([]Slot{}, r.Radiant...), r.Dire...)
	for i := range want {
		if slots[i] != want[i] {
			t.Errorf("slot %d: %+v, want %+v", i, slots[i], want[i])
		}
	}
	if r.Reader != "vision" || r.Cost == nil || *r.Cost != 0.00099 {
		t.Fatalf("reader %q, cost %v", r.Reader, r.Cost)
	}
}

func TestVisionFailsOnAnAnswerThatIsNotTheDraft(t *testing.T) {
	cases := map[string]string{
		"four radiant slots": `{"radiant": [{"hero": "Axe", "empty": false, "confidence": 0.9}], "dire": []}`,
		"not JSON":           `the radiant team has Axe`,
		"confidence above 1": strings.Replace(fullDraft, `"confidence": 0.9`, `"confidence": 9`, 1),
	}
	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			v := newVision(t, func(w http.ResponseWriter, _ *http.Request) {
				b, _ := json.Marshal(content)
				_, _ = fmt.Fprintf(w, `{"choices": [{"message": {"role": "assistant", "content": %s}}], "usage": {"cost": 0.001}}`, b)
			})
			if _, err := v.Read(context.Background(), solid(400, 300, color.RGBA{}), visionHeroes); err == nil {
				t.Fatal("bad answer accepted")
			}
		})
	}
}

func TestVisionNeverPutsTheKeyInAnError(t *testing.T) {
	v := newVision(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error": {"message": "bad key `+strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")+`"}}`, http.StatusUnauthorized)
	})
	_, err := v.Read(context.Background(), solid(400, 300, color.RGBA{}), visionHeroes)
	if err == nil || strings.Contains(err.Error(), "sk-or-test-key-not-real") {
		t.Fatalf("error %v", err)
	}
}

var fullDraft = `{"radiant": [` + strings.Repeat(`{"hero": "Axe", "empty": false, "confidence": 0.9},`, 4) +
	`{"hero": "", "empty": true, "confidence": 0.9}], "dire": [` + strings.Repeat(`{"hero": "", "empty": true, "confidence": 0.9},`, 4) +
	`{"hero": "", "empty": true, "confidence": 0.9}]}`
