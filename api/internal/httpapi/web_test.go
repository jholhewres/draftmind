package httpapi

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWebServesTheBuildWithSPAFallbackAndLeavesAPIAlone(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{"index.html": "<!doctype html>app", "assets/index-abc123.js": "console.log(1)", "favicon.svg": "<svg/>"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	h := New(fakeDB{}, nil, quietLog()).WithWeb(dir).Handler()
	get := func(path string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		return rec
	}

	asset := get("/assets/index-abc123.js")
	if asset.Code != http.StatusOK || asset.Body.String() != "console.log(1)" ||
		asset.Header().Get("Cache-Control") != "public, max-age=31536000, immutable" {
		t.Fatalf("hashed asset: %d %q cache %q", asset.Code, asset.Body, asset.Header().Get("Cache-Control"))
	}
	for _, path := range []string{"/", "/heroes/axe", "/match/42"} {
		page := get(path)
		if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), "app") || page.Header().Get("Cache-Control") != "no-cache" {
			t.Fatalf("%s: %d %q cache %q", path, page.Code, page.Body, page.Header().Get("Cache-Control"))
		}
	}
	if rec := get("/assets/missing-000.js"); rec.Code != http.StatusNotFound {
		t.Fatalf("missing asset: %d, want 404 not the app", rec.Code)
	}
	if rec := get("/api/nope"); rec.Code != http.StatusNotFound || strings.Contains(rec.Body.String(), "app") {
		t.Fatalf("unknown API route: %d %q", rec.Code, rec.Body)
	}
	if rec := get("/api/health"); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"ok"`) {
		t.Fatalf("health: %d %q", rec.Code, rec.Body)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/health", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST /api/health: %d, want 405", rec.Code)
	}
}
