package httpapi

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jholhewres/draftmind/api/internal/match"
	"github.com/jholhewres/draftmind/api/internal/writer"
)

// wavClip is ms milliseconds of 16 kHz mono 16-bit silence, as the browser sends.
func wavClip(ms int) []byte {
	data := make([]byte, 16*2*ms)
	var b bytes.Buffer
	w := func(v any) { _ = binary.Write(&b, binary.LittleEndian, v) }
	b.WriteString("RIFF")
	w(uint32(36 + len(data)))
	b.WriteString("WAVEfmt ")
	w(uint32(16))
	w(uint16(1))
	w(uint16(1))
	w(uint32(16000))
	w(uint32(32000))
	w(uint16(2))
	w(uint16(16))
	b.WriteString("data")
	w(uint32(len(data)))
	b.Write(data)
	return b.Bytes()
}

type fakeTranscriber struct {
	text string
	err  error
	got  [][]byte
	lang string
}

func (f *fakeTranscriber) Transcribe(_ context.Context, wav []byte, lang string) (string, error) {
	f.got, f.lang = append(f.got, wav), lang
	return f.text, f.err
}

// recordingCoach answers every message and keeps the texts it was given.
type recordingCoach struct{ texts []string }

func (c *recordingCoach) Reply(_ context.Context, _ match.Session, text, _ string, emit match.Emit) (match.Reply, error) {
	c.texts = append(c.texts, text)
	r := match.Reply{Kind: "answer", Category: "lane", Priority: "high", Source: "jev", Text: "Stay behind your creeps.", TextSource: writer.SourceWriter}
	emit("answer", r)
	return r, nil
}

func startTestMatch(t *testing.T, h http.Handler) string {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/match", strings.NewReader(`{"hero":1,"role":2,"enemies":[2]}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("start: %d %s", rec.Code, rec.Body)
	}
	var s match.Session
	if err := json.Unmarshal(rec.Body.Bytes(), &s); err != nil {
		t.Fatal(err)
	}
	return s.ID
}

func TestVoiceIsTranscribedAndAnsweredThroughTheCoachPipeline(t *testing.T) {
	stt := &fakeTranscriber{text: "  pudge keeps hooking me mid  "}
	coach := &recordingCoach{}
	srv := buildPatches(t).WithCoach(coach).WithTranscriber(stt)
	h := srv.Handler()
	id := startTestMatch(t, h)
	clip := wavClip(500)

	req := httptest.NewRequest(http.MethodPost, "/api/match/"+id+"/voice?lang=pt-BR", bytes.NewReader(clip))
	req.Header.Set("Content-Type", "audio/wav")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("voice: %d %s", rec.Code, rec.Body)
	}
	if len(stt.got) != 1 || !bytes.Equal(stt.got[0], clip) || stt.lang != "pt-BR" {
		t.Fatalf("transcriber got %d clips in %q", len(stt.got), stt.lang)
	}
	if len(coach.texts) != 1 || coach.texts[0] != "pudge keeps hooking me mid" {
		t.Fatalf("coach got %q, want the transcript", coach.texts)
	}
	body := rec.Body.String()
	for _, want := range []string{`"kind":"voice"`, `"text":"pudge keeps hooking me mid"`, "event: answer", "Stay behind your creeps.", "event: done"} {
		if !strings.Contains(body, want) {
			t.Fatalf("stream lacks %q:\n%s", want, body)
		}
	}

	// The transcript stays in the chat next to the answer.
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/match/"+id, nil))
	var got match.Session
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Messages) != 2 || got.Messages[0].Kind != "voice" || got.Messages[0].Text != "pudge keeps hooking me mid" ||
		got.Messages[1].Kind != "answer" || got.Messages[1].Text != "Stay behind your creeps." {
		t.Fatalf("messages %+v", got.Messages)
	}
}

func TestVoiceRefusesWhatIsNotASpokenClip(t *testing.T) {
	stt := &fakeTranscriber{err: writer.ErrNoSpeech}
	srv := buildPatches(t).WithCoach(&recordingCoach{}).WithTranscriber(stt)
	h := srv.Handler()
	id := startTestMatch(t, h)
	for _, c := range []struct {
		name string
		body []byte
		want int
	}{
		{"not a WAV file", []byte("OggS\x00\x02 not wav at all, just bytes"), http.StatusBadRequest},
		{"longer than the limit", wavClip(int(match.MaxClip/time.Millisecond) + 1000), http.StatusRequestEntityTooLarge},
		{"no speech in it", wavClip(300), http.StatusUnprocessableEntity},
	} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/match/"+id+"/voice", bytes.NewReader(c.body)))
		if rec.Code != c.want {
			t.Errorf("%s: %d %s, want %d", c.name, rec.Code, rec.Body, c.want)
		}
	}
	if len(stt.got) != 1 {
		t.Fatalf("transcriber called %d times, want only for the WAV clip", len(stt.got))
	}
}

func TestVoiceIsRateLimitedPerClient(t *testing.T) {
	srv := New(fakeDB{}, nil, quietLog()).WithVoiceLimit(Limit{Burst: 1, Every: time.Minute})
	h := srv.Handler()
	codes := []int{}
	for i := range 2 {
		req := httptest.NewRequest(http.MethodPost, "/api/match/00000000-0000-4000-8000-000000000000/voice", bytes.NewReader(wavClip(100)))
		req.RemoteAddr = "203.0.113.7:100" + string(rune('0'+i))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		codes = append(codes, rec.Code)
	}
	if codes[0] == http.StatusTooManyRequests || codes[1] != http.StatusTooManyRequests {
		t.Fatalf("statuses %v, want the second refused", codes)
	}
}
