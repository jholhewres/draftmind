package decision

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// photoRequest asks where the clock is in one image and which of two
// pictured heroes a crop shows.
func photoRequest() Request {
	return Request{
		Purpose: "test.photo",
		State:   map[string]any{"photo": "ref-photo"},
		Assets: map[string]Asset{
			"ref-photo": {Format: "jpg", Data: []byte("photo-bytes")},
			"ref-axe":   {Format: "png", Data: []byte("axe-bytes")},
		},
		Questions: map[string]Question{
			"clock": Box("the clock in `photo`"),
			"hero":  Choice("Which hero is in `photo`?", map[string]any{"Axe": "ref-axe", "other": "none of these"}),
		},
	}
}

const photoBody = `{
	"model": "typesafe/jev-1.14-20261014",
	"answers": {
		"clock": {"type": "box", "box": {"top": 0.1, "bottom": 0.19, "left": 0.42, "right": 0.53}, "confidence": 0.85},
		"hero": {"type": "choice", "choice": "Axe", "confidence": 0.7, "probabilities": {"Axe": 0.9, "other": 0.1}}
	},
	"usage": {"input_tokens": 3931, "output_tokens": 36, "cost": 0.000165}
}`

func TestDecisionClientSendsImagesAsAssetsWithTheImagesHeader(t *testing.T) {
	var raw map[string]any
	var header string
	c, _ := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		header = r.Header.Get("TypeSafe-Danger-Zone-Images")
		_ = json.NewDecoder(r.Body).Decode(&raw)
		reply(http.StatusOK, photoBody)(w, r)
	})
	res, err := c.Ask(context.Background(), photoRequest())
	if err != nil {
		t.Fatal(err)
	}
	if header != "true" {
		t.Fatalf("images header %q, want true", header)
	}
	assets, _ := raw["assets"].(map[string]any)
	axe, _ := assets["ref-axe"].(map[string]any)
	if len(assets) != 2 || axe["type"] != "image" || axe["format"] != "png" ||
		axe["data_base64"] != base64.StdEncoding.EncodeToString([]byte("axe-bytes")) {
		t.Fatalf("assets %v", assets)
	}
	box := res.Answers["clock"].Box
	if box == nil || box.Top != 0.1 || box.Bottom != 0.19 || box.Left != 0.42 || box.Right != 0.53 || res.Answers["clock"].Confidence != 0.85 {
		t.Fatalf("box answer %+v", res.Answers["clock"])
	}
	if res.Answers["hero"].Choice != "Axe" {
		t.Fatalf("choice answer %+v", res.Answers["hero"])
	}
}

func TestDecisionClientSendsNoImagesHeaderWithoutImages(t *testing.T) {
	header := "unset"
	c, _ := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		header = r.Header.Get("TypeSafe-Danger-Zone-Images")
		reply(http.StatusOK, okBody)(w, r)
	})
	if _, err := c.Ask(context.Background(), pickRequest()); err != nil {
		t.Fatal(err)
	}
	if header != "" {
		t.Fatalf("images header %q on a text request", header)
	}
}

func TestDecisionClientNeverLogsTheImages(t *testing.T) {
	c, log := newClient(t, reply(http.StatusOK, photoBody))
	if _, err := c.Ask(context.Background(), photoRequest()); err != nil {
		t.Fatal(err)
	}
	e := log.all()
	if len(e) != 1 {
		t.Fatalf("%d entries", len(e))
	}
	enc := base64.StdEncoding.EncodeToString([]byte("photo-bytes"))
	if bytes.Contains(e[0].Request, []byte(enc)) || bytes.Contains(e[0].Request, []byte("photo-bytes")) {
		t.Fatalf("logged request holds the image: %s", e[0].Request)
	}
	if !strings.Contains(string(e[0].Request), `"ref-photo":{"type":"image","format":"jpg","bytes":11}`) {
		t.Fatalf("logged request does not describe the image: %s", e[0].Request)
	}
}

func TestDecisionClientFailsOnABadBoxAnswer(t *testing.T) {
	cases := map[string]string{
		"no box":            `{"type": "box", "confidence": 0.8}`,
		"no confidence":     `{"type": "box", "box": {"top": 0.1, "bottom": 0.2, "left": 0.1, "right": 0.2}}`,
		"edge missing":      `{"type": "box", "box": {"top": 0.1, "bottom": 0.2, "left": 0.1}, "confidence": 0.8}`,
		"outside the image": `{"type": "box", "box": {"top": 0.1, "bottom": 1.2, "left": 0.1, "right": 0.2}, "confidence": 0.8}`,
		"inverted":          `{"type": "box", "box": {"top": 0.3, "bottom": 0.2, "left": 0.1, "right": 0.2}, "confidence": 0.8}`,
		"mistyped":          `{"type": "choice", "choice": "x", "confidence": 0.8}`,
	}
	for name, answer := range cases {
		t.Run(name, func(t *testing.T) {
			body := `{"model": "m", "answers": {"clock": ` + answer + `}}`
			c, _ := newClient(t, reply(http.StatusOK, body))
			req := Request{State: map[string]any{"photo": "ref"}, Assets: map[string]Asset{"ref": {Format: "jpg", Data: []byte{1}}},
				Questions: map[string]Question{"clock": Box("the clock in `photo`")}}
			if _, err := c.Ask(context.Background(), req); err == nil {
				t.Fatal("bad box answer accepted")
			}
		})
	}
}
