package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jholhewres/draftmind/api/internal/auth"
	"github.com/jholhewres/draftmind/api/internal/match"
	"github.com/jholhewres/draftmind/api/internal/photo"
	"github.com/jholhewres/draftmind/api/internal/writer"
)

// ResultParser reads a message such as "we won" into win or loss.
type ResultParser interface {
	ParseResult(ctx context.Context, text string) (string, error)
}

// Noter writes the coach's notes on an ended match.
type Noter interface {
	Notes(ctx context.Context, s match.Session, r match.Result, lang string) writer.Notes
}

// WithResultParser reads end-of-match messages; without it that route answers 503.
func (s *Server) WithResultParser(p ResultParser) *Server {
	s.resultParser = p
	return s
}

// WithEndReader reads end-of-match photos; without it that route answers 503.
func (s *Server) WithEndReader(r photo.EndReader) *Server {
	s.endReader = r
	return s
}

// WithNoter writes the notes on a confirmed result; without it a result has none.
func (s *Server) WithNoter(n Noter) *Server {
	s.noter = n
	return s
}

// ownMatch loads the session the way loadMatch does, answering 404 when a
// signed-in player's session is asked for by anyone else.
func (s *Server) ownMatch(w http.ResponseWriter, r *http.Request) (match.Session, bool) {
	sess, ok := s.loadMatch(w, r)
	if !ok {
		return sess, false
	}
	if sess.UserID != nil {
		if me := s.userID(r); me == nil || *me != *sess.UserID {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "no such match"})
			return sess, false
		}
	}
	return sess, true
}

// parseResult serves POST /api/match/{id}/end/parse: a message read into
// {"result": "win"|"loss"} for the player to confirm, or 422 when it does
// not say. Nothing is saved.
func (s *Server) parseResult(w http.ResponseWriter, r *http.Request) {
	if s.resultParser == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "messages cannot be read"})
		return
	}
	var req struct {
		Text string `json:"text"`
	}
	if err := decodeOne(w, r, &req, 4<<10); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	req.Text = strings.TrimSpace(req.Text)
	if req.Text == "" || utf8.RuneCountInString(req.Text) > match.MaxMessage {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "text must be 1 to 500 characters"})
		return
	}
	if _, ok := s.ownMatch(w, r); !ok {
		return
	}
	result, err := s.resultParser.ParseResult(r.Context(), req.Text)
	switch {
	case errors.Is(err, writer.ErrUnclear):
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "the message does not say how the match ended"})
	case err != nil:
		s.log.Warn("parse result", "err", err)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "the message could not be read; tap the result instead"})
	default:
		writeJSON(w, http.StatusOK, map[string]string{"result": result})
	}
}

// endReadTimeout bounds one end-screen read: the match is over, so it can
// take longer than a pick-screen read.
const endReadTimeout = 10 * time.Second

// readEnd serves POST /api/match/{id}/end/read: a photo of the end screen
// (multipart field "image") read into the result, duration and the
// player's KDA for the player to confirm. Nothing is saved, the photo
// included.
func (s *Server) readEnd(w http.ResponseWriter, r *http.Request) {
	if s.endReader == nil || s.catalog == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "end screens cannot be read"})
		return
	}
	data, status, err := photoUpload(w, r)
	if err != nil {
		writeJSON(w, status, map[string]string{"error": err.Error()})
		return
	}
	sess, ok := s.ownMatch(w, r)
	if !ok {
		return
	}
	release, ok := s.takeReadSlot(w)
	if !ok {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": errBusy.Error()})
		return
	}
	defer release()
	img, status, err := decodePhoto(data)
	if err != nil {
		writeJSON(w, status, map[string]string{"error": err.Error()})
		return
	}
	_, heroes, err := s.catalog.Heroes(r.Context())
	if err != nil {
		s.log.Error("end read: heroes", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not load the heroes"})
		return
	}
	hero := ""
	for _, h := range heroes {
		if h.ID == sess.HeroID {
			hero = h.Name
		}
	}
	ctx, cancel := context.WithTimeout(r.Context(), endReadTimeout)
	defer cancel()
	start := time.Now()
	read, err := s.endReader.ReadEnd(ctx, img, hero)
	if err != nil {
		s.log.Warn("end read failed", "err", err, "latency", time.Since(start))
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "the photo could not be read; tap the result instead"})
		return
	}
	attrs := []any{"latency", time.Since(start)}
	if read.Cost != nil {
		attrs = append(attrs, "cost_usd", *read.Cost)
	}
	s.log.Info("end read", attrs...)
	writeJSON(w, http.StatusOK, read)
}

// saveResult serves POST /api/match/{id}/result: the result the player
// confirmed, stored with the coach's notes on the match, which it returns.
func (s *Server) saveResult(w http.ResponseWriter, r *http.Request) {
	lang, ok := queryLang(w, r)
	if !ok {
		return
	}
	if s.matches == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "match sessions are not configured"})
		return
	}
	var req struct {
		Result    string `json:"result"`
		Source    string `json:"source"`
		DurationS *int   `json:"durationS"`
		Kills     *int   `json:"kills"`
		Deaths    *int   `json:"deaths"`
		Assists   *int   `json:"assists"`
	}
	if err := decodeOne(w, r, &req, 4<<10); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	res := match.Result{Outcome: req.Result, Source: req.Source, DurationS: req.DurationS, Kills: req.Kills, Deaths: req.Deaths, Assists: req.Assists}
	if (res.Kills == nil) != (res.Deaths == nil) || (res.Kills == nil) != (res.Assists == nil) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "send kills, deaths and assists together"})
		return
	}
	if err := res.Validate(); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	sess, ok := s.ownMatch(w, r)
	if !ok {
		return
	}
	res.Notes = []string{}
	if s.noter != nil {
		res.Notes = s.noter.Notes(r.Context(), sess, res, lang).Items
	}
	// The result is kept even when the player has left.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 5*time.Second)
	defer cancel()
	saved, err := s.matches.SaveResult(ctx, sess.ID, res)
	if err != nil {
		s.log.Error("save result", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not save the result"})
		return
	}
	writeJSON(w, http.StatusOK, saved)
}

// myMatches serves GET /api/me/matches: the signed-in player's ended
// matches, newest first.
func (s *Server) myMatches(w http.ResponseWriter, r *http.Request) {
	u, ok := auth.UserFrom(r.Context())
	if !ok || s.matches == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "sign in first"})
		return
	}
	past, err := s.matches.History(r.Context(), u.ID)
	if err != nil {
		s.log.Error("match history", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not load the history"})
		return
	}
	if past == nil {
		past = []match.Past{}
	}
	writeJSON(w, http.StatusOK, past)
}
