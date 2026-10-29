package photo

import (
	"bytes"
	"context"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
)

func pngBytes(t *testing.T, c color.RGBA) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, solid(256, 144, c)); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestPortraitsDownloadOnceIntoTheDataDir(t *testing.T) {
	var requests atomic.Int32
	body := pngBytes(t, color.RGBA{200, 30, 30, 255})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		_, _ = w.Write(body)
	}))
	defer srv.Close()
	dir := t.TempDir()
	p := NewPortraits(dir, srv.URL, srv.Client())
	got, err := p.byID(context.Background(), testHeroes)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(testHeroes) || int(requests.Load()) != len(testHeroes) {
		t.Fatalf("%d portraits, %d requests", len(got), requests.Load())
	}
	if _, err := os.Stat(filepath.Join(dir, "portraits", "furion.png")); err != nil {
		t.Fatalf("portrait not cached on disk: %v", err)
	}
	// A new process finds them on disk.
	again, err := NewPortraits(dir, srv.URL, srv.Client()).byID(context.Background(), testHeroes)
	if err != nil || len(again) != len(testHeroes) || int(requests.Load()) != len(testHeroes) {
		t.Fatalf("reload: %d portraits, %d requests, err %v", len(again), requests.Load(), err)
	}
}

func TestPortraitsFailOnAMissingImage(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	if err := NewPortraits(t.TempDir(), srv.URL, srv.Client()).Load(context.Background(), testHeroes); err == nil {
		t.Fatal("no error for a 404 portrait")
	}
}
