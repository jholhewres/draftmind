package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jholhewres/draftmind/api/internal/match"
	"github.com/jholhewres/draftmind/api/internal/writer"
)

// Matches keeps match sessions.
type Matches interface {
	Create(ctx context.Context, s match.Session) (match.Session, error)
	Get(ctx context.Context, id string) (match.Session, error)
	Add(ctx context.Context, sessionID string, m match.Message) (match.Message, error)
	SaveResult(ctx context.Context, sessionID string, r match.Result) (match.Result, error)
	History(ctx context.Context, userID int64) ([]match.Past, error)
}

// Replier runs the coach on a message.
type Replier interface {
	Reply(ctx context.Context, s match.Session, text, lang string, emit match.Emit) (match.Reply, error)
}

// WithMatches stores match sessions; without it the match routes answer 503.
func (s *Server) WithMatches(m Matches) *Server {
	s.matches = m
	return s
}

// WithCoach answers the player's messages; nil answers with a template.
func (s *Server) WithCoach(c Replier) *Server {
	s.coach = c
	return s
}

type startRequest struct {
	Hero    int   `json:"hero"`
	Role    int   `json:"role"`
	Allies  []int `json:"allies"`
	Enemies []int `json:"enemies"`
	Bans    []int `json:"bans"`
}

// startMatch serves POST /api/match: a session for the locked hero with its
// draft and the item plan decided for the enemies (B1).
func (s *Server) startMatch(w http.ResponseWriter, r *http.Request) {
	if s.matches == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "match sessions are not configured"})
		return
	}
	var req startRequest
	if err := decodeOne(w, r, &req, 16<<10); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	draftReq := suggestRequest{Allies: nonNil(req.Allies), Enemies: nonNil(req.Enemies), Bans: nonNil(req.Bans), Role: req.Role}
	if err := draftReq.validate(); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if req.Hero <= 0 || slices.Contains(slices.Concat(draftReq.Allies, draftReq.Enemies, draftReq.Bans), req.Hero) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "hero must be a hero the draft has not taken"})
		return
	}
	if _, _, status, err := s.loadDraft(r.Context(), draftReq); err != nil {
		writeJSON(w, status, map[string]string{"error": err.Error()})
		return
	}
	_, plan, status, err := s.decideBuild(r, buildRequest{Hero: req.Hero, Enemies: draftReq.Enemies})
	if err != nil {
		writeJSON(w, status, map[string]string{"error": err.Error()})
		return
	}
	sess, err := s.matches.Create(r.Context(), match.Session{
		UserID: s.userID(r), HeroID: req.Hero, Role: req.Role, Allies: draftReq.Allies, Enemies: draftReq.Enemies, Bans: draftReq.Bans, ItemPlan: plan,
	})
	if err != nil {
		s.log.Error("start match", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not start the match"})
		return
	}
	writeJSON(w, http.StatusOK, sess)
}

// getMatch serves GET /api/match/{id}: the session and its chat, so a
// reloaded page picks up where it was.
func (s *Server) getMatch(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.loadMatch(w, r)
	if ok {
		writeJSON(w, http.StatusOK, sess)
	}
}

func (s *Server) loadMatch(w http.ResponseWriter, r *http.Request) (match.Session, bool) {
	if s.matches == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "match sessions are not configured"})
		return match.Session{}, false
	}
	sess, err := s.matches.Get(r.Context(), r.PathValue("id"))
	if errors.Is(err, match.ErrNotFound) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "no such match"})
		return sess, false
	}
	if err != nil {
		s.log.Error("load match", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not load the match"})
		return sess, false
	}
	return sess, true
}

// matchMessage serves POST /api/match/{id}/messages: the player's message is
// stored, then the coach's turn streams as Server-Sent Events:
//
//	message    the stored player message
//	situation  the message as read: category, summary, resolved names
//	decision   the advice category and priority, and who decided them
//	answer     the coach's text, or a template the UI words
//	followup   instead of the three above, when the message could not be read
//	done       the stream is over
func (s *Server) matchMessage(w http.ResponseWriter, r *http.Request) {
	lang, ok := queryLang(w, r)
	if !ok {
		return
	}
	var req struct {
		Text string `json:"text"`
	}
	if err := decodeOne(w, r, &req, 8<<10); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	req.Text = strings.TrimSpace(req.Text)
	if req.Text == "" || utf8.RuneCountInString(req.Text) > match.MaxMessage {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("text must be 1 to %d characters", match.MaxMessage)})
		return
	}
	sess, ok := s.loadMatch(w, r)
	if !ok {
		return
	}
	s.coachTurn(w, r, sess, match.Message{Author: "player", Kind: "text", Text: req.Text}, lang)
}

// queryLang reads the ?lang= tag, en when absent; a bad one answers 400.
func queryLang(w http.ResponseWriter, r *http.Request) (string, bool) {
	lang := r.URL.Query().Get("lang")
	if lang == "" {
		lang = "en"
	}
	if len(lang) > 35 || !langTag.MatchString(lang) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "lang must be a language tag such as en or pt-BR"})
		return "", false
	}
	return lang, true
}

// coachTurn stores the player's message, then streams the coach's turn on
// its text and stores the reply.
func (s *Server) coachTurn(w http.ResponseWriter, r *http.Request, sess match.Session, msg match.Message, lang string) {
	mine, err := s.matches.Add(r.Context(), sess.ID, msg)
	if err != nil {
		s.log.Error("match message", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not store the message"})
		return
	}
	out := newEventStream(w, s)
	out.send("message", mine)
	reply := match.Reply{Kind: "answer", TextSource: writer.SourceTemplate, Reason: writer.ReasonUnavailable}
	if s.coach == nil {
		out.send("answer", reply)
	} else if reply, err = s.coach.Reply(r.Context(), sess, msg.Text, lang, out.send); err != nil {
		s.log.Error("coach reply", "err", err)
		reply = match.Reply{Kind: "answer", TextSource: writer.SourceTemplate, Reason: writer.ReasonError}
		out.send("answer", reply)
	}
	// The turn is kept even when the player has left.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 5*time.Second)
	defer cancel()
	m := match.Message{Author: "coach", Kind: reply.Kind, Text: reply.Text, Situation: reply.Situation,
		Category: reply.Category, Priority: reply.Priority, Source: reply.Source, Reason: reply.Reason}
	if _, err := s.matches.Add(ctx, sess.ID, m); err != nil {
		s.log.Error("store coach reply", "err", err)
	}
	out.send("done", struct{}{})
}

// decodeOne reads exactly one JSON object of at most limit bytes into v.
func decodeOne(w http.ResponseWriter, r *http.Request, v any, limit int64) error {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, limit))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return errors.New("invalid JSON: " + err.Error())
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return errors.New("invalid JSON: one object expected")
	}
	return nil
}

func nonNil(ids []int) []int {
	if ids == nil {
		return []int{}
	}
	return ids
}
