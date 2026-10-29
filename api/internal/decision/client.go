// Package decision is a client for TypeSafe System One models (Jev) served
// through OpenRouter (ADR-0003): typed questions in, strictly parsed answers
// out, with a deadline, retries, a hedged request and a log of every call.
package decision

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jholhewres/draftmind/api/internal/budget"
)

// Outcomes of a call, as logged.
const (
	OutcomeOK      = "ok"
	OutcomeTimeout = "timeout"
	OutcomeError   = "error"
)

// Question is one typed question: a choice, a noul or a score.
type Question struct {
	Type         string `json:"type"`
	Instructions any    `json:"instructions,omitempty"`
	Criteria     any    `json:"criteria,omitempty"`
}

// Choice selects one of options, each named by its key and described by its value.
func Choice(instructions string, options map[string]any) Question {
	return Question{Type: "choice", Instructions: instructions, Criteria: options}
}

// Noul asks for the probability that a yes/no question is yes.
func Noul(instructions string) Question {
	return Question{Type: "noul", Instructions: instructions}
}

// Score rates against ordered levels, the first being 0.
func Score(instructions string, levels ...any) Question {
	return Question{Type: "score", Instructions: instructions, Criteria: levels}
}

// Box asks where something is in an image (Jev 1.14 and later).
func Box(instructions string) Question {
	return Question{Type: "box", Instructions: instructions}
}

// Asset is an image the state, instructions or criteria refer to by its key
// in Request.Assets.
type Asset struct {
	Format string // png or jpg
	Data   []byte
}

// Request is the state and the named questions about it. Purpose names the
// caller in the log; it is not sent.
type Request struct {
	Purpose   string
	State     any
	Assets    map[string]Asset
	Questions map[string]Question
}

// Edges is a Box answer: fractions of the image's width and height from its
// top-left corner.
type Edges struct {
	Top    float64 `json:"top"`
	Bottom float64 `json:"bottom"`
	Left   float64 `json:"left"`
	Right  float64 `json:"right"`
}

// Answer is one question's answer; the fields set depend on its type.
type Answer struct {
	Type          string             `json:"type"`
	Choice        string             `json:"choice,omitempty"`
	Noul          float64            `json:"noul,omitempty"`
	Score         float64            `json:"score,omitempty"`
	Confidence    float64            `json:"confidence,omitempty"`
	Probabilities map[string]float64 `json:"probabilities,omitempty"`
	Box           *Edges             `json:"box,omitempty"`
}

// Usage is what a call consumed; Cost is in USD when the endpoint reports it.
type Usage struct {
	InputTokens  int
	OutputTokens int
	Cost         *float64
}

// Result is a call's answers keyed by question name, the model that
// answered and the usage.
type Result struct {
	Model   string
	Answers map[string]Answer
	Usage   Usage
}

// Entry is one call as decision_log keeps it.
type Entry struct {
	Purpose       string
	Model         string
	AnsweredModel string
	Outcome       string
	Error         string
	Latency       time.Duration
	Attempts      int
	Hedged        bool
	InputTokens   int
	OutputTokens  int
	Cost          *float64
	Request       json.RawMessage
	Answer        json.RawMessage
}

// Logger records calls.
type Logger interface {
	Record(ctx context.Context, e Entry)
}

// StatusError is a non-200 answer from the endpoint, its body cut short.
type StatusError struct {
	Code       int
	Body       string
	retryAfter time.Duration
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("decision: HTTP %d: %s", e.Code, e.Body)
}

// retryable says whether another attempt may succeed: a rate limit, a server
// error or a broken connection, never a bad request or a bad answer.
func retryable(err error) bool {
	var se *StatusError
	if errors.As(err, &se) {
		return se.Code == http.StatusTooManyRequests || se.Code >= 500
	}
	var ne net.Error
	return errors.As(err, &ne) && !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled)
}

// Default timings.
const (
	DefaultDeadline    = 1500 * time.Millisecond
	DefaultHedgeAfter  = 600 * time.Millisecond
	DefaultBackoff     = 100 * time.Millisecond
	DefaultMaxAttempts = 3
)

// Client calls {BaseURL}/v1/systemone with the provider pinned, no
// fallbacks and zero data retention.
type Client struct {
	BaseURL, APIKey, Model, Provider string
	HTTP                             *http.Client
	Log                              Logger // nil logs nothing
	// Deadline bounds a whole call, retries and hedge included.
	Deadline time.Duration
	// HedgeAfter starts a second request while the first is unanswered; < 0 never.
	HedgeAfter time.Duration
	// Backoff is the first retry's wait, doubled on each retry, unless the
	// answer has a Retry-After.
	Backoff time.Duration
	// MaxAttempts bounds the requests of each of the two racing lanes.
	MaxAttempts int
}

// New builds a client with the default timings.
func New(baseURL, apiKey, model, provider string, log Logger) *Client {
	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"), APIKey: apiKey, Model: model, Provider: provider,
		HTTP: http.DefaultClient, Log: log,
		Deadline: DefaultDeadline, HedgeAfter: DefaultHedgeAfter, Backoff: DefaultBackoff, MaxAttempts: DefaultMaxAttempts,
	}
}

type providerPrefs struct {
	AllowFallbacks bool     `json:"allow_fallbacks"`
	DataCollection string   `json:"data_collection"`
	Only           []string `json:"only"`
	ZDR            bool     `json:"zdr"`
}

type wireRequest struct {
	Model     string              `json:"model"`
	State     any                 `json:"state"`
	Assets    map[string]any      `json:"assets,omitempty"`
	Questions map[string]Question `json:"questions"`
	Provider  providerPrefs       `json:"provider"`
}

type wireAsset struct {
	Type       string `json:"type"`
	Format     string `json:"format"`
	DataBase64 string `json:"data_base64,omitempty"`
	Bytes      int    `json:"bytes,omitempty"` // in the log, in place of the data
}

// wireAssets encodes the images for the endpoint, or, for the log, only
// their format and size: images are never stored.
func wireAssets(assets map[string]Asset, forLog bool) map[string]any {
	if len(assets) == 0 {
		return nil
	}
	out := make(map[string]any, len(assets))
	for ref, a := range assets {
		w := wireAsset{Type: "image", Format: a.Format}
		if forLog {
			w.Bytes = len(a.Data)
		} else {
			w.DataBase64 = base64.StdEncoding.EncodeToString(a.Data)
		}
		out[ref] = w
	}
	return out
}

// needsImages says whether a request must turn on images: it sends an image
// or asks a Box question.
func needsImages(req Request) bool {
	if len(req.Assets) > 0 {
		return true
	}
	for _, q := range req.Questions {
		if q.Type == "box" {
			return true
		}
	}
	return false
}

// outcome is what one lane of the race ended with.
type outcome struct {
	res      Result
	raw      []byte
	err      error
	attempts int
}

// Ask sends the questions and returns every answer, or an error when any
// answer is missing or mistyped, the endpoint fails or the deadline passes.
// Each call is logged once, whatever its outcome.
func (c *Client) Ask(ctx context.Context, req Request) (Result, error) {
	if err := budget.Check(ctx); err != nil {
		return Result{}, err // no call made, nothing to log
	}
	start := time.Now()
	wire := wireRequest{
		Model: c.Model, State: req.State, Questions: req.Questions,
		Provider: providerPrefs{DataCollection: "deny", Only: []string{c.Provider}, ZDR: true},
	}
	wire.Assets = wireAssets(req.Assets, true)
	logged, err := json.Marshal(wire)
	var body []byte
	if err == nil {
		wire.Assets = wireAssets(req.Assets, false)
		body, err = json.Marshal(wire)
	}
	if err == nil {
		err = validQuestions(req.Questions)
	}
	if err != nil {
		err = fmt.Errorf("decision: request: %w", err)
		c.record(ctx, req, logged, start, outcome{err: err}, false)
		return Result{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, c.Deadline)
	defer cancel()
	o, hedged := c.race(ctx, body, needsImages(req), req.Questions)
	if o.err != nil && ctx.Err() != nil {
		o.err = fmt.Errorf("decision: no answer within %v: %w", c.Deadline, ctx.Err())
	}
	c.record(ctx, req, logged, start, o, hedged)
	budget.Add(ctx, o.res.Usage.Cost)
	return o.res, o.err
}

// race runs a lane and, while it is unanswered after HedgeAfter, a second
// one; the first answer wins and cancels the other.
func (c *Client) race(ctx context.Context, body []byte, images bool, qs map[string]Question) (outcome, bool) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan outcome, 2)
	run := func() { done <- c.lane(ctx, body, images, qs) }
	go run()
	running, hedged := 1, false
	var hedge <-chan time.Time
	if c.HedgeAfter >= 0 {
		t := time.NewTimer(c.HedgeAfter)
		defer t.Stop()
		hedge = t.C
	}
	attempts := 0
	for {
		select {
		case <-hedge:
			hedge, hedged = nil, true
			running++
			go run()
		case o := <-done:
			running--
			attempts += o.attempts
			// A success wins; a bad request or a bad answer will not get
			// better; a lane out of retries waits for the other.
			if o.err == nil || !retryable(o.err) || running == 0 {
				o.attempts = attempts
				return o, hedged
			}
		}
	}
}

// lane sends the request, retrying a retryable failure while the deadline
// leaves room for the wait.
func (c *Client) lane(ctx context.Context, body []byte, images bool, qs map[string]Question) outcome {
	wait := c.Backoff
	for attempt := 1; ; attempt++ {
		res, raw, err := c.once(ctx, body, images, qs)
		if err == nil || !retryable(err) || attempt >= c.MaxAttempts {
			return outcome{res: res, raw: raw, err: err, attempts: attempt}
		}
		d := wait
		if se := (*StatusError)(nil); errors.As(err, &se) && se.retryAfter > 0 {
			d = se.retryAfter
		}
		wait *= 2
		if dl, ok := ctx.Deadline(); ok && time.Until(dl) < d {
			return outcome{res: res, raw: raw, err: err, attempts: attempt}
		}
		t := time.NewTimer(d)
		select {
		case <-t.C:
		case <-ctx.Done():
			t.Stop()
			return outcome{err: ctx.Err(), attempts: attempt}
		}
	}
}

// once makes one request and parses its answer strictly.
func (c *Client) once(ctx context.Context, body []byte, images bool, qs map[string]Question) (Result, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/v1/systemone", bytes.NewReader(body))
	if err != nil {
		return Result{}, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	req.Header.Set("Content-Type", "application/json")
	if images {
		req.Header.Set("TypeSafe-Danger-Zone-Images", "true") // beta: images and Box (ADR-0006)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return Result{}, nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return Result{}, nil, err
	}
	raw = c.redact(raw)
	if resp.StatusCode != http.StatusOK {
		return Result{}, raw, &StatusError{Code: resp.StatusCode, Body: snippet(raw), retryAfter: retryAfter(resp.Header)}
	}
	res, err := parse(raw, qs)
	return res, raw, err
}

// maxBody bounds the answer read; Jev's answers are a few kilobytes.
const maxBody = 1 << 20

// redact takes the API key out of anything the endpoint sent back.
func (c *Client) redact(b []byte) []byte {
	if c.APIKey == "" {
		return b
	}
	return bytes.ReplaceAll(b, []byte(c.APIKey), []byte("<REDACTED>"))
}

func snippet(b []byte) string {
	const n = 300
	s := strings.ToValidUTF8(strings.TrimSpace(string(b)), "\uFFFD")
	if len(s) > n {
		cut := n
		for cut > 0 && !utf8.RuneStart(s[cut]) {
			cut-- // never split a character
		}
		s = s[:cut] + "…"
	}
	return s
}

// retryAfter reads Retry-After in seconds; an HTTP date is ignored.
func retryAfter(h http.Header) time.Duration {
	secs, err := strconv.Atoi(strings.TrimSpace(h.Get("Retry-After")))
	if err != nil || secs < 0 {
		return 0
	}
	return time.Duration(secs) * time.Second
}

func (c *Client) record(ctx context.Context, req Request, body []byte, start time.Time, o outcome, hedged bool) {
	if c.Log == nil {
		return
	}
	e := Entry{
		Purpose: req.Purpose, Model: c.Model, AnsweredModel: o.res.Model,
		Outcome: OutcomeOK, Latency: time.Since(start), Attempts: o.attempts, Hedged: hedged,
		InputTokens: o.res.Usage.InputTokens, OutputTokens: o.res.Usage.OutputTokens, Cost: o.res.Usage.Cost,
		Request: json.RawMessage(body),
	}
	if len(o.raw) > 0 {
		if json.Valid(o.raw) {
			e.Answer = json.RawMessage(o.raw)
		} else {
			e.Answer, _ = json.Marshal(snippet(o.raw))
		}
	}
	if o.err != nil {
		e.Outcome, e.Error = OutcomeError, string(c.redact([]byte(o.err.Error())))
		if errors.Is(o.err, context.DeadlineExceeded) {
			e.Outcome = OutcomeTimeout
		}
	}
	// The call's own context may be past its deadline; the log is not.
	lctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer cancel()
	c.Log.Record(lctx, e)
}
