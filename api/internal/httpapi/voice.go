package httpapi

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/jholhewres/draftmind/api/internal/budget"
	"github.com/jholhewres/draftmind/api/internal/match"
	"github.com/jholhewres/draftmind/api/internal/writer"
)

// WithTranscriber turns voice clips into text; without it the voice route answers 503.
func (s *Server) WithTranscriber(t match.Transcriber) *Server {
	s.stt = t
	return s
}

// WithVoiceLimit sets the per-client rate limit of voice clips.
func (s *Server) WithVoiceLimit(l Limit) *Server {
	s.voiceLimit = l
	return s
}

// matchVoice serves POST /api/match/{id}/voice: the body is a WAV clip
// (ADR-0005). It is transcribed, and the transcript runs the same coach
// turn as a typed message, its stored message of kind voice. A clip that
// is not WAV answers 400, one over match.MaxClip 413, and one with no
// speech 422; a failed transcription answers 502.
func (s *Server) matchVoice(w http.ResponseWriter, r *http.Request) {
	lang, ok := queryLang(w, r)
	if !ok {
		return
	}
	p, ok := s.personaOf(w, r)
	if !ok {
		return
	}
	if s.stt == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "voice is not configured"})
		return
	}
	clip, err := io.ReadAll(http.MaxBytesReader(w, r.Body, match.MaxClipBytes))
	var tooBig *http.MaxBytesError
	switch {
	case errors.As(err, &tooBig):
		writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": match.ErrClipTooLong.Error()})
		return
	case err != nil:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "could not read the clip"})
		return
	}
	if err := match.CheckClip(clip); errors.Is(err, match.ErrClipTooLong) {
		writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": err.Error()})
		return
	} else if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	sess, ok := s.loadMatch(w, r)
	if !ok {
		return
	}
	text, err := s.stt.Transcribe(r.Context(), clip, lang)
	text = strings.TrimSpace(text)
	switch {
	case errors.Is(err, writer.ErrNoSpeech) || (err == nil && text == ""):
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "no speech in the clip"})
		return
	case errors.Is(err, budget.ErrOverCap):
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": errOverCap})
		return
	case err != nil:
		if !errors.Is(err, context.Canceled) {
			s.log.Error("transcribe", "err", err)
		}
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "could not transcribe the clip"})
		return
	}
	if runes := []rune(text); len(runes) > match.MaxMessage {
		text = string(runes[:match.MaxMessage])
	}
	s.coachTurn(w, r, sess, match.Message{Author: "player", Kind: "voice", Text: text}, lang, p)
}
