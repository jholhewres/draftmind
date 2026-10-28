package gamesync

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHTTPSourceFetchesBuildFileAtRef(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	b, err := NewHTTPSource(srv.URL, "abc123", srv.Client()).Fetch(context.Background(), "patch.json")
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/odota/dotaconstants/abc123/build/patch.json" || string(b) != "[]" {
		t.Fatalf("path %q body %q", gotPath, b)
	}
}

func TestHTTPSourceFailsOnNon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "rate limited", http.StatusTooManyRequests)
	}))
	defer srv.Close()

	_, err := NewHTTPSource(srv.URL, "master", srv.Client()).Fetch(context.Background(), "heroes.json")
	if err == nil || !strings.Contains(err.Error(), "429") {
		t.Fatalf("want a 429 error, got %v", err)
	}
}
