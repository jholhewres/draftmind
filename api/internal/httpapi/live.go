package httpapi

import (
	"bytes"
	"context"
	"image"
	"net/http"
	"time"

	"github.com/jholhewres/draftmind/api/internal/budget"
	"github.com/jholhewres/draftmind/api/internal/match"
)

// DefaultLiveLimit allows a tick every 10 s per client, with a small burst:
// the browser ticks every 30 s.
var DefaultLiveLimit = Limit{Burst: 3, Every: 10 * time.Second}

// WithLive turns the live coach on (ADR-0014); without it live ticks answer 503.
func (s *Server) WithLive(l *match.Live) *Server {
	s.live = l
	return s
}

type liveResponse struct {
	match.LiveDecision
	// Message is the stored coach message when the tick spoke.
	Message *match.Message `json:"message,omitempty"`
	// Budget is how much of the session's model spend cap is used.
	Budget *budgetUse `json:"budget,omitempty"`
}

// liveTick serves POST /api/match/{id}/live: one frame of the shared game
// window, multipart "image". The answer is the tick's decision; a spoken
// event is stored as a live coach message and returned. The frame is never
// stored. Every failure past validation is a silent tick.
func (s *Server) liveTick(w http.ResponseWriter, r *http.Request) {
	if s.live == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "the live coach is not configured"})
		return
	}
	lang, ok := queryLang(w, r)
	if !ok {
		return
	}
	p, ok := s.personaOf(w, r)
	if !ok {
		return
	}
	sess, ok := s.loadMatch(w, r)
	if !ok {
		return
	}
	data, status, err := photoUpload(w, r)
	if err != nil {
		writeJSON(w, status, map[string]string{"error": err.Error()})
		return
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || (format != "jpeg" && format != "png") || cfg.Width > maxPhotoSide || cfg.Height > maxPhotoSide {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "the frame must be a JPEG or PNG of at most 2048 px a side"})
		return
	}
	f := match.LiveFrame{Format: "png", Data: data, Facts: s.gsiFacts(r.Context(), s.userID(r))}
	if f.Facts != nil {
		// The game's own state replaces reading the HUD (ADR-0015).
		f.Data = nil
	}
	if format == "jpeg" {
		f.Format = "jpg"
	}
	d := s.live.Tick(r.Context(), sess, f, s.now())
	if d.Reason == match.LiveReasonEnded {
		s.live.Forget(sess.ID)
	}
	out := liveResponse{LiveDecision: d}
	if d.Speak {
		s.live.Word(r.Context(), &d, lang, p)
		ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 5*time.Second)
		defer cancel()
		rp := d.Reply
		m, err := s.matches.Add(ctx, sess.ID, match.Message{Author: "coach", Kind: "answer", Text: rp.Text,
			Category: rp.Category, Priority: rp.Priority, Source: rp.Source, Reason: rp.Reason, Items: rp.Items})
		if err != nil {
			// Fail silent: a line the chat cannot keep is not shown.
			s.log.Error("store live event", "err", err)
			out.LiveDecision = match.LiveDecision{Reason: match.LiveReasonError, Cost: d.Cost}
		} else {
			out.Message = &m
		}
	}
	if spent, capUSD, ok := budget.Used(r.Context()); ok {
		out.Budget = &budgetUse{Spent: spent, Cap: capUSD}
	}
	attrs := []any{"session", sess.ID, "speak", d.Speak, "event", d.Event, "reason", d.Reason}
	if d.Cost != nil {
		attrs = append(attrs, "cost_usd", *d.Cost)
	}
	s.log.Info("live tick", attrs...)
	writeJSON(w, http.StatusOK, out)
}
