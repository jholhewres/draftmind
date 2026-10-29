package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/jholhewres/draftmind/api/internal/catalog"
	"github.com/jholhewres/draftmind/api/internal/photo"
)

// stubReader reads every photo as Anti-Mage in the first radiant slot and
// keeps what it was given.
type stubReader struct {
	err    error
	size   image.Point
	heroes []int
}

func (s *stubReader) Name() string { return "stub" }

func (s *stubReader) Read(_ context.Context, img image.Image, heroes []catalog.Hero) (photo.Read, error) {
	s.size = img.Bounds().Size()
	for _, h := range heroes {
		s.heroes = append(s.heroes, h.ID)
	}
	if s.err != nil {
		return photo.Read{}, s.err
	}
	r := photo.Resolve([photo.Slots]photo.Guess{{Name: "Anti-Mage", Confidence: 0.9}}, heroes, 0.5)
	r.Reader, r.Fallback = "vision", photo.FallbackTimeout
	return r, nil
}

func jpegOf(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := range img.Pix {
		img.Pix[i] = byte(i * 7)
	}
	img.Set(0, 0, color.White)
	var b bytes.Buffer
	if err := jpeg.Encode(&b, img, &jpeg.Options{Quality: 100}); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// upload posts file as the multipart field name.
func upload(t *testing.T, srv *Server, name string, file []byte) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	part, err := mw.CreateFormFile(name, "pick.jpg")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = part.Write(file)
	_ = mw.Close()
	req := httptest.NewRequest(http.MethodPost, "/api/draft/read", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	return rec
}

type readBody struct {
	Patch    string       `json:"patch"`
	Reader   string       `json:"reader"`
	Fallback string       `json:"fallback"`
	Radiant  []photo.Slot `json:"radiant"`
	Dire     []photo.Slot `json:"dire"`
	Error    string       `json:"error"`
}

func TestDraftReadReturnsEverySlotResolvedOnTheCurrentPatch(t *testing.T) {
	pool := seedDraft(t)
	reader := &stubReader{}
	srv := New(fakeDB{}, catalog.NewStore(pool), quietLog()).WithPhotoReader(reader)
	rec := upload(t, srv, "image", jpegOf(t, 1600, 900))
	var body readBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || rec.Code != http.StatusOK {
		t.Fatalf("status %d, body %s", rec.Code, rec.Body)
	}
	if reader.size != (image.Point{1600, 900}) {
		t.Fatalf("reader got a %v image", reader.size)
	}
	// Mirana (9) is only on the old patch.
	if len(reader.heroes) != 5 || reader.heroes[0] == 9 {
		t.Fatalf("reader got heroes %v, want the 5 of 7.41", reader.heroes)
	}
	if body.Patch != "7.41" || body.Reader != "vision" || body.Fallback != "timeout" {
		t.Fatalf("body %+v", body)
	}
	if len(body.Radiant) != 5 || len(body.Dire) != 5 || body.Radiant[0] != (photo.Slot{HeroID: 1, Status: photo.StatusHero, Confidence: 0.9}) ||
		body.Radiant[1].Status != photo.StatusUnknown || body.Radiant[1].HeroID != 0 {
		t.Fatalf("slots %+v %+v", body.Radiant, body.Dire)
	}
}

func TestDraftReadRefusesAPhotoOver5MB(t *testing.T) {
	srv := New(fakeDB{}, nil, quietLog()).WithPhotoReader(&stubReader{})
	rec := upload(t, srv, "image", make([]byte, 5<<20+1))
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
}

func TestDraftReadRefusesWhatIsNotAJPEGOrPNG(t *testing.T) {
	srv := New(fakeDB{}, nil, quietLog()).WithPhotoReader(&stubReader{})
	for name, file := range map[string][]byte{"text": []byte("not an image"), "gif": []byte("GIF89a\x01\x00\x01\x00")} {
		if rec := upload(t, srv, "image", file); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status %d: %s", name, rec.Code, rec.Body)
		}
	}
	if rec := upload(t, srv, "photo", jpegOf(t, 10, 10)); rec.Code != http.StatusBadRequest {
		t.Errorf("wrong field: status %d", rec.Code)
	}
}

func TestDraftReadNeverWritesThePhotoToDisk(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp) // where a parsed multipart form would spill a large file
	pool := seedDraft(t)
	srv := New(fakeDB{}, catalog.NewStore(pool), quietLog()).WithPhotoReader(&stubReader{})
	big := append(jpegOf(t, 1600, 1200), make([]byte, 4<<20)...) // trailing bytes after the JPEG are ignored
	if rec := upload(t, srv, "image", big); rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	if entries, _ := os.ReadDir(tmp); len(entries) != 0 {
		t.Fatalf("the upload left %d files in the temp dir", len(entries))
	}
}

func TestDraftReadWithoutAReaderIsUnavailable(t *testing.T) {
	srv := New(fakeDB{}, catalog.NewStore(seedDraft(t)), quietLog())
	if rec := upload(t, srv, "image", jpegOf(t, 10, 10)); rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status %d", rec.Code)
	}
}

func TestDraftReadReportsReadersThatFailed(t *testing.T) {
	srv := New(fakeDB{}, catalog.NewStore(seedDraft(t)), quietLog()).WithPhotoReader(&stubReader{err: errors.New("both down")})
	rec := upload(t, srv, "image", jpegOf(t, 300, 300))
	var body readBody
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if rec.Code != http.StatusBadGateway || body.Error == "" {
		t.Fatalf("status %d, body %s", rec.Code, rec.Body)
	}
}

func pngOf(t *testing.T, w, h int) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewGray(image.Rect(0, 0, w, h))); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestDraftReadRefusesAPhotoLargerThanTheBrowserSends(t *testing.T) {
	reader := &stubReader{}
	srv := New(fakeDB{}, nil, quietLog()).WithPhotoReader(reader)
	// A flat 4096 px image is a few KB, but decodes to 64 MiB.
	rec := upload(t, srv, "image", pngOf(t, 4096, 4096))
	if rec.Code != http.StatusRequestEntityTooLarge || reader.size != (image.Point{}) {
		t.Fatalf("status %d, reader got %v", rec.Code, reader.size)
	}
}

// blockingReader holds every read until released, or until the test ends.
type blockingReader struct {
	started chan struct{}
	release chan struct{}
}

func (b *blockingReader) Name() string { return "blocking" }

func (b *blockingReader) Read(ctx context.Context, _ image.Image, heroes []catalog.Hero) (photo.Read, error) {
	b.started <- struct{}{}
	select {
	case <-b.release:
	case <-ctx.Done():
		return photo.Read{}, ctx.Err()
	}
	return photo.Resolve([photo.Slots]photo.Guess{}, heroes, 0.5), nil
}

func TestDraftReadRunsAFewReadsAtOnceAndTurnsAwayTheRest(t *testing.T) {
	pool := seedDraft(t)
	reader := &blockingReader{started: make(chan struct{}, 10), release: make(chan struct{})}
	srv := New(fakeDB{}, catalog.NewStore(pool), quietLog()).WithPhotoReader(reader)
	file := jpegOf(t, 300, 300)
	codes := make(chan int, maxConcurrentReads)
	for range maxConcurrentReads {
		go func() { codes <- upload(t, srv, "image", file).Code }()
	}
	for range maxConcurrentReads {
		select {
		case <-reader.started:
		case <-time.After(5 * time.Second):
			t.Fatal("the reads did not start")
		}
	}
	if rec := upload(t, srv, "image", file); rec.Code != http.StatusServiceUnavailable || rec.Header().Get("Retry-After") == "" {
		t.Fatalf("a read past the limit: status %d, Retry-After %q", rec.Code, rec.Header().Get("Retry-After"))
	}
	close(reader.release)
	for range maxConcurrentReads {
		if code := <-codes; code != http.StatusOK {
			t.Fatalf("a read within the limit: status %d", code)
		}
	}
}
