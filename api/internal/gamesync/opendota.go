package gamesync

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	neturl "net/url"
	"strconv"
	"strings"
	"time"
)

// Matchup is a hero's record against one opponent.
type Matchup struct{ OpponentID, Games, Wins int }

// HeroStat is a hero's picks and wins in one bracket: "1".."8" by rank, or "pro".
type HeroStat struct {
	HeroID      int
	Bracket     string
	Picks, Wins int
}

// StatBrackets are the brackets stored from /heroStats, as <bracket>_pick and <bracket>_win.
var StatBrackets = []string{"1", "2", "3", "4", "5", "6", "7", "8", "pro"}

// ParseMatchups reads a /heroes/{id}/matchups response. Opponents the patch
// lacks are skipped.
func ParseMatchups(b []byte, inPatch map[int]bool) ([]Matchup, error) {
	var raw []struct {
		HeroID      int `json:"hero_id"`
		GamesPlayed int `json:"games_played"`
		Wins        int `json:"wins"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, fmt.Errorf("parse matchups: %w", err)
	}
	out := make([]Matchup, 0, len(raw))
	seen := map[int]bool{}
	for _, r := range raw {
		if r.GamesPlayed < 0 || r.Wins < 0 || r.Wins > r.GamesPlayed {
			return nil, fmt.Errorf("parse matchups: opponent %d has %d wins in %d games", r.HeroID, r.Wins, r.GamesPlayed)
		}
		if seen[r.HeroID] {
			return nil, fmt.Errorf("parse matchups: opponent %d twice", r.HeroID)
		}
		seen[r.HeroID] = true
		if inPatch[r.HeroID] {
			out = append(out, Matchup{OpponentID: r.HeroID, Games: r.GamesPlayed, Wins: r.Wins})
		}
	}
	return out, nil
}

// ParseHeroStats reads a /heroStats response, one row per hero on the patch
// and bracket. A null count reads as zero.
func ParseHeroStats(b []byte, inPatch map[int]bool) ([]HeroStat, error) {
	var raw []map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, fmt.Errorf("parse heroStats: %w", err)
	}
	var out []HeroStat
	for _, h := range raw {
		var id int
		if err := json.Unmarshal(h["id"], &id); err != nil || id <= 0 {
			return nil, fmt.Errorf("parse heroStats: a hero has no id")
		}
		if !inPatch[id] {
			continue
		}
		for _, br := range StatBrackets {
			pick, win := h[br+"_pick"], h[br+"_win"]
			if pick == nil || win == nil {
				return nil, fmt.Errorf("parse heroStats: hero %d lacks bracket %s", id, br)
			}
			var picks, wins *int
			if json.Unmarshal(pick, &picks) != nil || json.Unmarshal(win, &wins) != nil {
				return nil, fmt.Errorf("parse heroStats: hero %d bracket %s is not a count", id, br)
			}
			s := HeroStat{HeroID: id, Bracket: br, Picks: deref(picks), Wins: deref(wins)}
			if s.Picks < 0 || s.Wins < 0 || s.Wins > s.Picks {
				return nil, fmt.Errorf("parse heroStats: hero %d has %d wins in %d picks in bracket %s", id, s.Wins, s.Picks, br)
			}
			out = append(out, s)
		}
	}
	return out, nil
}

func deref(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}

// DefaultOpenDotaBase is the OpenDota API.
const DefaultOpenDotaBase = "https://api.opendota.com/api"

// OpenDotaInterval keeps a keyless sync at one request per second, the
// free tier's limit; a key allows OpenDotaKeyInterval.
const (
	OpenDotaInterval    = time.Second
	OpenDotaKeyInterval = 100 * time.Millisecond
)

// openDotaRetries is how often a 429 or 5xx answer is retried.
const openDotaRetries = 3

// HTTPOpenDota reads the OpenDota API, one request per interval at most.
// Not safe for concurrent use.
type HTTPOpenDota struct {
	baseURL, key string
	interval     time.Duration
	client       *http.Client
	next         time.Time // the earliest start of the next request
}

// NewHTTPOpenDota builds a client; key may be empty.
func NewHTTPOpenDota(baseURL, key string, client *http.Client) *HTTPOpenDota {
	interval := OpenDotaInterval
	if key != "" {
		interval = OpenDotaKeyInterval
	}
	return &HTTPOpenDota{baseURL: strings.TrimRight(baseURL, "/"), key: key, interval: interval, client: client}
}

// HeroStats returns the /heroStats response.
func (o *HTTPOpenDota) HeroStats(ctx context.Context) ([]byte, error) {
	return o.get(ctx, "/heroStats")
}

// Matchups returns the /heroes/{id}/matchups response.
func (o *HTTPOpenDota) Matchups(ctx context.Context, heroID int) ([]byte, error) {
	return o.get(ctx, "/heroes/"+strconv.Itoa(heroID)+"/matchups")
}

// get fetches path, retrying a 429 or 5xx after Retry-After or a doubling
// delay. Errors name the path only, never the key.
func (o *HTTPOpenDota) get(ctx context.Context, path string) ([]byte, error) {
	url := o.baseURL + path
	if o.key != "" {
		url += "?" + neturl.Values{"api_key": {o.key}}.Encode()
	}
	for attempt := 0; ; attempt++ {
		if err := sleepUntil(ctx, o.next); err != nil {
			return nil, err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, fmt.Errorf("GET %s: bad request", path)
		}
		resp, err := o.client.Do(req)
		if err != nil {
			o.next = time.Now().Add(o.interval)
			return nil, fmt.Errorf("GET %s: %w", path, redactURL(err))
		}
		b, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
		_ = resp.Body.Close()
		o.next = time.Now().Add(o.interval)
		retry := resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500
		if retry && attempt < openDotaRetries {
			delay := o.interval << (attempt + 1)
			if s, err := strconv.Atoi(resp.Header.Get("Retry-After")); err == nil && s >= 0 {
				delay = max(o.interval, min(time.Duration(s)*time.Second, time.Minute))
			}
			o.next = time.Now().Add(delay)
			continue
		}
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("GET %s: HTTP %d", path, resp.StatusCode)
		}
		if err != nil {
			return nil, fmt.Errorf("GET %s: %w", path, redactURL(err))
		}
		return b, nil
	}
}

// redactURL drops the URL, and the key in its query, from a client error.
func redactURL(err error) error {
	var uerr *neturl.Error
	if errors.As(err, &uerr) {
		return uerr.Err
	}
	return err
}

func sleepUntil(ctx context.Context, t time.Time) error {
	d := time.Until(t)
	if d <= 0 {
		return ctx.Err()
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
