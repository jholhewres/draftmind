package httpapi

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jholhewres/draftmind/api/internal/auth"
	"github.com/jholhewres/draftmind/api/internal/gsi"
	"github.com/jholhewres/draftmind/api/internal/match"
)

// fixture reads a recorded GSI post with the token put in.
func gsiFixture(t *testing.T, name, token string, edit func(map[string]any)) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "gsi", "testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	var v map[string]any
	_ = json.Unmarshal(b, &v)
	v["auth"] = map[string]any{"token": token}
	if edit != nil {
		edit(v)
	}
	out, _ := json.Marshal(v)
	return string(out)
}

func TestTheGamePostsWithTheAccountTokenAndTheMatchResultIsRecordedFromTheWinner(t *testing.T) {
	srv, pool := buildPatchesPool(t)
	logs := &bytes.Buffer{}
	a, err := auth.New(pool, "http://localhost:5173", slog.New(slog.NewTextHandler(logs, nil)))
	if err != nil {
		t.Fatal(err)
	}
	if err := a.WithLogMailer(); err != nil {
		t.Fatal(err)
	}
	matches := match.NewStore(pool)
	now := time.Unix(1_800_000_000, 0)
	srv.now = func() time.Time { return now }
	h := srv.WithAuth(a).WithMatches(matches).WithGSI(gsi.NewStore(pool), matches).WithPublicURL("https://draftmind.example").Handler()
	send(h, http.MethodPost, "/api/auth/email", `{"email":"player@example.com"}`)
	var me *http.Cookie
	for _, c := range send(h, http.MethodPost, "/api/auth/email/verify", `{"token":"`+devLink.FindStringSubmatch(logs.String())[1]+`"}`).Result().Cookies() {
		if c.Name == auth.CookieName {
			me = c
		}
	}

	// Settings: a token and the config file that carries it.
	rec := send(h, http.MethodPost, "/api/me/gsi/token", "", me)
	var issued struct{ Filename, Config string }
	_ = json.Unmarshal(rec.Body.Bytes(), &issued)
	if rec.Code != http.StatusOK || issued.Filename != "gamestate_integration_draftmind.cfg" ||
		!strings.Contains(issued.Config, `"uri" "https://draftmind.example/api/gsi"`) {
		t.Fatalf("issue: %d %s", rec.Code, rec.Body)
	}
	token := issued.Config[strings.Index(issued.Config, `"token" "`)+9:]
	token = token[:strings.Index(token, `"`)]
	if rec := send(h, http.MethodPost, "/api/me/gsi/token", ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("a visitor got a token: %d", rec.Code)
	}

	// A wrong token, a garbage body or an oversized body is refused.
	if rec := send(h, http.MethodPost, "/api/gsi", gsiFixture(t, "in_progress.json", "wrong", nil)); rec.Code != http.StatusUnauthorized {
		t.Fatalf("wrong token: %d", rec.Code)
	}
	if rec := send(h, http.MethodPost, "/api/gsi", "{"); rec.Code != http.StatusBadRequest {
		t.Fatalf("garbage: %d", rec.Code)
	}
	if rec := send(h, http.MethodPost, "/api/gsi", `{"pad":"`+strings.Repeat("x", maxGSIBody)+`"}`); rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized: %d", rec.Code)
	}

	// A match in progress: the post lands and the settings say "receiving".
	var sess match.Session
	_ = json.Unmarshal(send(h, http.MethodPost, "/api/match", `{"hero":1,"role":2,"enemies":[2]}`, me).Body.Bytes(), &sess)
	if rec := send(h, http.MethodPost, "/api/gsi", gsiFixture(t, "in_progress.json", token, nil)); rec.Code != http.StatusOK {
		t.Fatalf("post: %d", rec.Code)
	}
	var st gsiStatus
	_ = json.Unmarshal(send(h, http.MethodGet, "/api/me/gsi", "", me).Body.Bytes(), &st)
	if !st.Configured || !st.Receiving {
		t.Fatalf("status %+v, want receiving", st)
	}

	// The gold for the plan's next item: a code trigger posts a live line,
	// with no model call (the server has no Jev).
	now = now.Add(time.Second)
	send(h, http.MethodPost, "/api/gsi", gsiFixture(t, "in_progress.json", token, func(v map[string]any) {
		v["player"].(map[string]any)["gold"] = 5000
	}))
	got, _ := matches.Get(t.Context(), sess.ID)
	first := match.PlanCards(got)[0].Name
	if len(got.Messages) != 1 || got.Messages[0].Source != match.SourceLive || got.Messages[0].Reason != "live:item_timing" ||
		len(got.Messages[0].Items) != 1 || got.Messages[0].Items[0].Name != first {
		t.Fatalf("messages %+v, want one live item line for the plan's next item %s", got.Messages, first)
	}

	// Post game: radiant won, the player is radiant, the result is recorded.
	send(h, http.MethodPost, "/api/gsi", gsiFixture(t, "post_game.json", token, nil))
	got, _ = matches.Get(t.Context(), sess.ID)
	if got.Result == nil || got.Result.Outcome != "win" || got.Result.Source != "gsi" || *got.Result.Kills != 12 ||
		*got.Result.Deaths != 4 || *got.Result.Assists != 17 || *got.Result.DurationS != 2410 {
		t.Fatalf("result %+v, want a win 12/4/17 in 2410 s from gsi", got.Result)
	}

	// Revoked: the game's posts stop counting.
	if rec := send(h, http.MethodDelete, "/api/me/gsi/token", "", me); rec.Code != http.StatusNoContent {
		t.Fatalf("revoke: %d", rec.Code)
	}
	if rec := send(h, http.MethodPost, "/api/gsi", gsiFixture(t, "in_progress.json", token, nil)); rec.Code != http.StatusUnauthorized {
		t.Fatalf("revoked token: %d", rec.Code)
	}
}
