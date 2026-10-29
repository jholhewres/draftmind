// Package gsi reads the game's own Game State Integration posts (ADR-0015):
// the player's clock, gold, items, level, KDA and the end of the match, and
// turns changes in them into coach triggers in code, before any model call.
package gsi

import (
	"encoding/json"
	"errors"
	"slices"
	"strings"
)

// Phases a state is in.
const (
	PhaseDraft = "draft" // hero selection, strategy time, pre-game
	PhaseGame  = "game"  // the match is in progress
	PhaseEnded = "ended" // post game, with a winner
	PhaseOther = "other" // menus, spectating, anything else
)

// State is the player's own state from one post. Nothing about the enemy:
// the game does not send it to the player, and Draftmind does not track it.
type State struct {
	MatchID        string   `json:"matchId,omitempty"`
	Phase          string   `json:"phase"`
	Clock          int      `json:"clock"` // seconds on the game clock; negative before the horn
	DurationS      int      `json:"durationS,omitempty"`
	Mode           string   `json:"mode,omitempty"` // a draft.Mode when the post names one
	Winner         string   `json:"winner,omitempty"`
	Team           string   `json:"team,omitempty"`
	Gold           int      `json:"gold"`
	GPM            int      `json:"gpm,omitempty"`
	Level          int      `json:"level,omitempty"`
	Kills          int      `json:"kills"`
	Deaths         int      `json:"deaths"`
	Assists        int      `json:"assists"`
	Alive          bool     `json:"alive"`
	BuybackCost    int      `json:"buybackCost,omitempty"`
	BuybackReady   bool     `json:"buybackReady,omitempty"`
	Items          []string `json:"items"` // inventory item keys, without "item_"
	HeroID         int      `json:"heroId,omitempty"`
	LastDeathClock int      `json:"lastDeathClock,omitempty"` // kept across posts by the caller
}

type wire struct {
	Map *struct {
		MatchID   string `json:"matchid"`
		ClockTime int    `json:"clock_time"`
		GameState string `json:"game_state"`
		WinTeam   string `json:"win_team"`
		GameMode  string `json:"game_mode"`
	} `json:"map"`
	Player *struct {
		TeamName string `json:"team_name"`
		Gold     int    `json:"gold"`
		GPM      int    `json:"gpm"`
		Kills    int    `json:"kills"`
		Deaths   int    `json:"deaths"`
		Assists  int    `json:"assists"`
	} `json:"player"`
	Hero *struct {
		ID              int  `json:"id"`
		Level           int  `json:"level"`
		Alive           bool `json:"alive"`
		BuybackCost     int  `json:"buyback_cost"`
		BuybackCooldown int  `json:"buyback_cooldown"`
	} `json:"hero"`
	Items map[string]struct {
		Name string `json:"name"`
	} `json:"items"`
	Auth *struct {
		Token string `json:"token"`
	} `json:"auth"`
}

// ErrNoToken means the post carries no auth token.
var ErrNoToken = errors.New("gsi: the post has no auth token")

// modes maps the game's mode names to Draftmind's (ADR-0012).
var modes = map[string]string{
	"DOTA_GAMEMODE_AP": "all_pick", "DOTA_GAMEMODE_ALL_DRAFT": "all_pick_ranked", "DOTA_GAMEMODE_TURBO": "turbo",
	"DOTA_GAMEMODE_CM": "captains_mode", "DOTA_GAMEMODE_CD": "captains_draft", "DOTA_GAMEMODE_SD": "single_draft",
	"DOTA_GAMEMODE_RD": "random_draft", "DOTA_GAMEMODE_AR": "all_random",
}

// Parse reads one post: the state and the auth token it carries.
func Parse(b []byte) (State, string, error) {
	var w wire
	if err := json.Unmarshal(b, &w); err != nil {
		return State{}, "", err
	}
	if w.Auth == nil || w.Auth.Token == "" {
		return State{}, "", ErrNoToken
	}
	s := State{Phase: PhaseOther, Items: []string{}}
	if m := w.Map; m != nil {
		s.MatchID, s.Clock, s.Mode = m.MatchID, m.ClockTime, modes[m.GameMode]
		switch m.GameState {
		case "DOTA_GAMERULES_STATE_GAME_IN_PROGRESS":
			s.Phase = PhaseGame
		case "DOTA_GAMERULES_STATE_POST_GAME":
			s.Phase = PhaseEnded
			s.DurationS = m.ClockTime
			if m.WinTeam == "radiant" || m.WinTeam == "dire" {
				s.Winner = m.WinTeam
			}
		case "DOTA_GAMERULES_STATE_HERO_SELECTION", "DOTA_GAMERULES_STATE_STRATEGY_TIME",
			"DOTA_GAMERULES_STATE_PRE_GAME", "DOTA_GAMERULES_STATE_WAIT_FOR_MAP_TO_LOAD", "DOTA_GAMERULES_STATE_TEAM_SHOWCASE":
			s.Phase = PhaseDraft
		}
	}
	if p := w.Player; p != nil {
		s.Team, s.Gold, s.GPM, s.Kills, s.Deaths, s.Assists = p.TeamName, p.Gold, p.GPM, p.Kills, p.Deaths, p.Assists
	}
	if h := w.Hero; h != nil {
		s.HeroID, s.Level, s.Alive, s.BuybackCost = h.ID, h.Level, h.Alive, h.BuybackCost
		s.BuybackReady = h.BuybackCooldown == 0 && h.BuybackCost > 0
	}
	for i := range 6 {
		it, ok := w.Items["slot"+string(rune('0'+i))]
		if ok && it.Name != "" && it.Name != "empty" {
			s.Items = append(s.Items, strings.TrimPrefix(it.Name, "item_"))
		}
	}
	return s, w.Auth.Token, nil
}

// Outcome is the player's result once the match ended with a winner.
func (s State) Outcome() (string, bool) {
	if s.Phase != PhaseEnded || s.Winner == "" || s.Team == "" {
		return "", false
	}
	if s.Winner == s.Team {
		return "win", true
	}
	return "loss", true
}

// Trigger events, code only.
const (
	EventItem    = "item_timing" // gold reached for the next planned item
	EventDeaths  = "deaths"      // died again within DeathWindow
	EventPhase   = "phase"       // the laning stage ended
	EventBuyback = "buyback"     // dead, buyback ready and affordable
)

// Thresholds, in seconds of game clock; Turbo halves them (ADR-0012).
const (
	DeathWindow = 180
	LaningEnd   = 600
)

// PlanItem is an item of the session's plan with its cost.
type PlanItem struct {
	Key  string `json:"key"`
	Name string `json:"name"`
	Cost int    `json:"cost"`
}

// Plan is what the triggers read from the session: the items to buy next, in order.
type Plan struct{ Next []PlanItem }

// Trigger is one code trigger; Item is set for an item event.
type Trigger struct {
	Event string    `json:"event"`
	Item  *PlanItem `json:"item,omitempty"`
}

func scale(s State, secs int) int {
	if s.Mode == "turbo" {
		return secs / 2
	}
	return secs
}

// Triggers compares the last state with the new one during a match. prev is
// nil on the first post. Only changes fire, so a state that stays the same
// fires nothing.
func Triggers(prev *State, cur State, plan Plan) []Trigger {
	if cur.Phase != PhaseGame {
		return nil
	}
	var out []Trigger
	if next := nextItem(cur, plan); next != nil && cur.Gold >= next.Cost {
		was := prev != nil && prev.Phase == PhaseGame && prev.Gold >= next.Cost && nextItem(*prev, plan) != nil && nextItem(*prev, plan).Key == next.Key
		if prev != nil && !was {
			out = append(out, Trigger{Event: EventItem, Item: next})
		}
	}
	if prev != nil && cur.Deaths > prev.Deaths {
		last := prev.LastDeathClock
		if prev.Deaths > 0 && last == 0 {
			last = prev.Clock
		}
		if prev.Deaths > 0 && cur.Clock-last <= scale(cur, DeathWindow) {
			out = append(out, Trigger{Event: EventDeaths})
		}
		if !cur.Alive && cur.BuybackReady && cur.Gold >= cur.BuybackCost {
			out = append(out, Trigger{Event: EventBuyback})
		}
	}
	if prev != nil && prev.Clock < scale(cur, LaningEnd) && cur.Clock >= scale(cur, LaningEnd) {
		out = append(out, Trigger{Event: EventPhase})
	}
	return out
}

// nextItem is the first planned item the player does not own yet.
func nextItem(s State, plan Plan) *PlanItem {
	for i, it := range plan.Next {
		if !slices.Contains(s.Items, it.Key) {
			return &plan.Next[i]
		}
	}
	return nil
}
