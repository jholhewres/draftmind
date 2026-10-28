package gamesync

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	neturl "net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// AbilityKind tells a hero's regular abilities, its innate one and its talents apart.
type AbilityKind string

// Ability kinds as stored in ability.kind.
const (
	KindAbility AbilityKind = "ability"
	KindInnate  AbilityKind = "innate"
	KindTalent  AbilityKind = "talent"
)

// HeroStats are a hero's base stats on one patch.
type HeroStats struct {
	StrBase, AgiBase, IntBase                           int
	StrGain, AgiGain, IntGain                           float64
	Armor, AttackRate, TurnRate, HealthRegen, ManaRegen float64
	MagicResistance, DamageMin, DamageMax, AttackRange  int
	ProjectileSpeed, MovementSpeed, MaxHealth, MaxMana  int
	SightRangeDay, SightRangeNight                      int
}

// AbilityText is an ability's or talent's text in one language.
type AbilityText struct {
	Name, Description, Lore string
}

// Ability is one ability or talent of a hero.
type Ability struct {
	ID        int
	Name      string // internal name, e.g. puck_illusory_orb
	Kind      AbilityKind
	Cooldowns []float64 // per level; empty when it has none
	ManaCosts []float64
	Text      AbilityText
}

// HeroDetail is one herodata response: the hero in one language.
type HeroDetail struct {
	ID         int
	Name       string
	Hype, Bio  string
	Complexity int
	Stats      HeroStats
	Abilities  []Ability
	Talents    []Ability // in feed order: two per level from 10 to 25
}

type rawSpecialValue struct {
	Name    string    `json:"name"`
	Values  []float64 `json:"values_float"`
	Bonuses []struct {
		Name  string  `json:"name"`
		Value float64 `json:"value"`
	} `json:"bonuses"`
}

type rawAbility struct {
	ID            int               `json:"id"`
	Name          string            `json:"name"`
	NameLoc       string            `json:"name_loc"`
	DescLoc       string            `json:"desc_loc"`
	LoreLoc       string            `json:"lore_loc"`
	Cooldowns     []float64         `json:"cooldowns"`
	ManaCosts     []float64         `json:"mana_costs"`
	Durations     []float64         `json:"durations"`
	ChannelTimes  []float64         `json:"channel_times"`
	CastPoints    []float64         `json:"cast_points"`
	SpecialValues []rawSpecialValue `json:"special_values"`
	IsInnate      bool              `json:"ability_is_innate"`
}

type rawHeroData struct {
	ID              int          `json:"id"`
	NameLoc         string       `json:"name_loc"`
	BioLoc          string       `json:"bio_loc"`
	HypeLoc         string       `json:"hype_loc"`
	Complexity      int          `json:"complexity"`
	StrBase         int          `json:"str_base"`
	StrGain         float64      `json:"str_gain"`
	AgiBase         int          `json:"agi_base"`
	AgiGain         float64      `json:"agi_gain"`
	IntBase         int          `json:"int_base"`
	IntGain         float64      `json:"int_gain"`
	DamageMin       int          `json:"damage_min"`
	DamageMax       int          `json:"damage_max"`
	AttackRate      float64      `json:"attack_rate"`
	AttackRange     int          `json:"attack_range"`
	ProjectileSpeed int          `json:"projectile_speed"`
	Armor           float64      `json:"armor"`
	MagicResistance int          `json:"magic_resistance"`
	MovementSpeed   int          `json:"movement_speed"`
	TurnRate        float64      `json:"turn_rate"`
	SightRangeDay   int          `json:"sight_range_day"`
	SightRangeNight int          `json:"sight_range_night"`
	MaxHealth       int          `json:"max_health"`
	HealthRegen     float64      `json:"health_regen"`
	MaxMana         int          `json:"max_mana"`
	ManaRegen       float64      `json:"mana_regen"`
	Abilities       []rawAbility `json:"abilities"`
	Talents         []rawAbility `json:"talents"`
}

// ParseHeroData reads a herodata response. It rejects a hero inPatch lacks.
func ParseHeroData(b []byte, inPatch map[int]bool) (HeroDetail, error) {
	var raw struct {
		Result struct {
			Data struct {
				Heroes []rawHeroData `json:"heroes"`
			} `json:"data"`
		} `json:"result"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return HeroDetail{}, fmt.Errorf("parse herodata: %w", err)
	}
	if len(raw.Result.Data.Heroes) != 1 {
		return HeroDetail{}, fmt.Errorf("parse herodata: want 1 hero, got %d", len(raw.Result.Data.Heroes))
	}
	h := raw.Result.Data.Heroes[0]
	if !inPatch[h.ID] {
		return HeroDetail{}, fmt.Errorf("parse herodata: hero %d is not in the current patch", h.ID)
	}
	if h.NameLoc == "" {
		return HeroDetail{}, fmt.Errorf("parse herodata: hero %d has no name", h.ID)
	}
	d := HeroDetail{
		ID:         h.ID,
		Name:       h.NameLoc,
		Hype:       plainText(h.HypeLoc),
		Bio:        plainText(h.BioLoc),
		Complexity: h.Complexity,
		Stats: HeroStats{
			StrBase: h.StrBase, StrGain: h.StrGain, AgiBase: h.AgiBase, AgiGain: h.AgiGain,
			IntBase: h.IntBase, IntGain: h.IntGain, Armor: h.Armor, MagicResistance: h.MagicResistance,
			DamageMin: h.DamageMin, DamageMax: h.DamageMax, AttackRate: h.AttackRate, AttackRange: h.AttackRange,
			ProjectileSpeed: h.ProjectileSpeed, MovementSpeed: h.MovementSpeed, TurnRate: h.TurnRate,
			MaxHealth: h.MaxHealth, HealthRegen: h.HealthRegen, MaxMana: h.MaxMana, ManaRegen: h.ManaRegen,
			SightRangeDay: h.SightRangeDay, SightRangeNight: h.SightRangeNight,
		},
	}
	seen := map[int]bool{}
	for _, a := range h.Abilities {
		if a.ID <= 0 || a.Name == "" || a.NameLoc == "" || seen[a.ID] {
			return HeroDetail{}, fmt.Errorf("parse herodata: hero %d has an ability without id or name, or twice: %d %q", h.ID, a.ID, a.Name)
		}
		seen[a.ID] = true
		kind := KindAbility
		if a.IsInnate {
			kind = KindInnate
		}
		d.Abilities = append(d.Abilities, Ability{
			ID: a.ID, Name: a.Name, Kind: kind,
			Cooldowns: nonZero(a.Cooldowns), ManaCosts: nonZero(a.ManaCosts),
			Text: AbilityText{
				Name:        a.NameLoc,
				Description: plainText(fillValues(a)),
				Lore:        plainText(a.LoreLoc),
			},
		})
	}
	for _, t := range h.Talents {
		if t.ID <= 0 || t.Name == "" || t.NameLoc == "" || seen[t.ID] {
			return HeroDetail{}, fmt.Errorf("parse herodata: hero %d has a talent without id or name, or twice: %d %q", h.ID, t.ID, t.Name)
		}
		seen[t.ID] = true
		d.Talents = append(d.Talents, Ability{
			ID: t.ID, Name: t.Name, Kind: KindTalent,
			Text: AbilityText{Name: plainText(talentName(t, h.Abilities))},
		})
	}
	return d, nil
}

// nonZero returns nil when every value is zero: the feed's way of saying none.
func nonZero(vs []float64) []float64 {
	for _, v := range vs {
		if v != 0 {
			return vs
		}
	}
	return nil
}

// %name% is an ability value; %% is a literal percent sign.
var valueRef = regexp.MustCompile(`%([A-Za-z0-9_]*)%`)

// fillValues replaces each %name% in the ability's description with the
// values of special value name, matched without case, joined by " / ".
// Duration, channel time and cast point fall back to the ability's own
// fields. Unknown names stay as written.
func fillValues(a rawAbility) string {
	own := map[string][]float64{
		"abilityduration": a.Durations, "abilitychanneltime": a.ChannelTimes, "abilitycastpoint": a.CastPoints,
	}
	return valueRef.ReplaceAllStringFunc(a.DescLoc, func(m string) string {
		name := m[1 : len(m)-1]
		if name == "" {
			return "%"
		}
		for _, v := range a.SpecialValues {
			if strings.EqualFold(v.Name, name) {
				return joinValues(v.Values)
			}
		}
		if vs := own[strings.ToLower(name)]; len(vs) > 0 {
			return joinValues(vs)
		}
		return m
	})
}

// {s:name} in a talent's name: its own value, or its bonus on an ability's
// value when name is bonus_<value>.
var talentRef = regexp.MustCompile(`\{s:([A-Za-z0-9_]+)\}`)

func talentName(t rawAbility, abilities []rawAbility) string {
	return talentRef.ReplaceAllStringFunc(t.NameLoc, func(m string) string {
		name := talentRef.FindStringSubmatch(m)[1]
		for _, v := range t.SpecialValues {
			if v.Name == name {
				return joinValues(v.Values)
			}
		}
		target, ok := strings.CutPrefix(name, "bonus_")
		if !ok {
			return m
		}
		for _, a := range abilities {
			for _, v := range a.SpecialValues {
				if v.Name != target {
					continue
				}
				for _, b := range v.Bonuses {
					if b.Name == t.Name {
						return formatValue(b.Value)
					}
				}
			}
		}
		return m
	})
}

func joinValues(vs []float64) string {
	parts := make([]string, 0, len(vs))
	same := true
	for i, v := range vs {
		parts = append(parts, formatValue(v))
		same = same && (i == 0 || v == vs[0])
	}
	if same && len(parts) > 1 {
		parts = parts[:1]
	}
	return strings.Join(parts, " / ")
}

func formatValue(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }

var (
	lineBreak = regexp.MustCompile(`(?i)<br\s*/?>`)
	markup    = regexp.MustCompile(`<[^>]*>`)
)

// plainText drops the feed's markup, keeping line breaks.
func plainText(s string) string {
	s = lineBreak.ReplaceAllString(s, "\n")
	s = markup.ReplaceAllString(s, "")
	return strings.TrimSpace(html.UnescapeString(s))
}

// DefaultDatafeedBase serves the game's public data feed (ADR-0007).
const DefaultDatafeedBase = "https://www.dota2.com"

// DefaultDatafeedInterval keeps the sync at about 2 requests a second.
const DefaultDatafeedInterval = 500 * time.Millisecond

// HTTPFeed reads the data feed, one request per interval at most. Not safe
// for concurrent use.
type HTTPFeed struct {
	baseURL  string
	interval time.Duration
	client   *http.Client
	last     time.Time
}

// NewHTTPFeed builds a feed client.
func NewHTTPFeed(baseURL string, interval time.Duration, client *http.Client) *HTTPFeed {
	return &HTTPFeed{baseURL: strings.TrimRight(baseURL, "/"), interval: interval, client: client}
}

// HeroData returns the herodata response for a hero in a feed language.
func (f *HTTPFeed) HeroData(ctx context.Context, heroID int, language string) ([]byte, error) {
	if err := f.wait(ctx); err != nil {
		return nil, err
	}
	defer func() { f.last = time.Now() }()
	q := neturl.Values{"hero_id": {strconv.Itoa(heroID)}, "language": {language}}
	url := f.baseURL + "/datafeed/herodata?" + q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := f.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: HTTP %d", url, resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 8<<20))
}

// wait holds the next request until interval has passed since the last one ended.
func (f *HTTPFeed) wait(ctx context.Context) error {
	if d := time.Until(f.last.Add(f.interval)); !f.last.IsZero() && d > 0 {
		t := time.NewTimer(d)
		defer t.Stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
		}
	}
	return nil
}
