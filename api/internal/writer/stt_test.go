package writer

import (
	"context"
	"encoding/base64"
	"errors"
	"testing"
	"time"
)

func TestTranscribeSendsTheClipAsAudioInputAndReturnsTheTranscript(t *testing.T) {
	or := &openRouter{}
	srv := or.serve(t, "  the enemy pudge keeps hooking me mid \n", 0)
	log := &callLog{}
	w := New(srv.URL, "test-key", "google/gemini-3.1-flash-lite", log)
	w.HTTP = srv.Client()
	clip := []byte("RIFF\x24\x00\x00\x00WAVEfmt ")

	got, err := w.Transcribe(context.Background(), clip, "pt-BR")
	if err != nil {
		t.Fatal(err)
	}
	if got != "the enemy pudge keeps hooking me mid" {
		t.Fatalf("transcript %q", got)
	}
	if len(or.bodies) != 1 {
		t.Fatalf("%d calls", len(or.bodies))
	}
	msgs := or.bodies[0]["messages"].([]any)
	parts := msgs[len(msgs)-1].(map[string]any)["content"].([]any)
	var audio map[string]any
	for _, p := range parts {
		if p := p.(map[string]any); p["type"] == "input_audio" {
			audio = p["input_audio"].(map[string]any)
		}
	}
	if audio == nil || audio["format"] != "wav" || audio["data"] != base64.StdEncoding.EncodeToString(clip) {
		t.Fatalf("audio part %v, want the clip as base64 wav", audio)
	}
	if len(log.calls) != 1 || log.calls[0].Purpose != "coach.stt" || log.calls[0].Outcome != OutcomeOK {
		t.Fatalf("logged %+v", log.calls)
	}
}

func TestTranscribeSaysWhenTheClipHasNoSpeech(t *testing.T) {
	or := &openRouter{}
	srv := or.serve(t, "", 0)
	w := New(srv.URL, "test-key", "google/gemini-3.1-flash-lite", nil)
	w.HTTP = srv.Client()
	if _, err := w.Transcribe(context.Background(), []byte("RIFF"), "en"); !errors.Is(err, ErrNoSpeech) {
		t.Fatalf("err %v, want ErrNoSpeech", err)
	}
}

func TestTranscribeGivesUpWhenTheModelIsLate(t *testing.T) {
	or := &openRouter{}
	srv := or.serve(t, "late", time.Second)
	w := New(srv.URL, "test-key", "google/gemini-3.1-flash-lite", nil)
	w.HTTP = srv.Client()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := w.Transcribe(ctx, []byte("RIFF"), "en"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err %v, want a deadline", err)
	}
}
