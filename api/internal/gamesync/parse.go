// Package gamesync loads game data from dotaconstants and OpenDota into Postgres.
package gamesync

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

// Hero is one hero as dotaconstants describes it.
type Hero struct {
	ID          int
	Slug        string
	Name        string
	PrimaryAttr string
	AttackType  string
	Roles       []string
	Img         string
	Icon        string
}

type rawHero struct {
	ID            int      `json:"id"`
	Name          string   `json:"name"`
	LocalizedName string   `json:"localized_name"`
	PrimaryAttr   string   `json:"primary_attr"`
	AttackType    string   `json:"attack_type"`
	Roles         []string `json:"roles"`
	Img           string   `json:"img"`
	Icon          string   `json:"icon"`
}

// ParseHeroes reads dotaconstants heroes.json, sorted by id.
func ParseHeroes(b []byte) ([]Hero, error) {
	var raw map[string]rawHero
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, fmt.Errorf("parse heroes: %w", err)
	}
	heroes := make([]Hero, 0, len(raw))
	for key, r := range raw {
		if r.ID <= 0 || r.LocalizedName == "" || !strings.HasPrefix(r.Name, heroPrefix) {
			return nil, fmt.Errorf("parse heroes: entry %q lacks id, name or localized_name", key)
		}
		roles := r.Roles
		if roles == nil {
			roles = []string{}
		}
		heroes = append(heroes, Hero{
			ID:          r.ID,
			Slug:        strings.TrimPrefix(r.Name, heroPrefix),
			Name:        r.LocalizedName,
			PrimaryAttr: r.PrimaryAttr,
			AttackType:  r.AttackType,
			Roles:       roles,
			Img:         strings.TrimSuffix(r.Img, "?"),
			Icon:        strings.TrimSuffix(r.Icon, "?"),
		})
	}
	sort.Slice(heroes, func(i, j int) bool { return heroes[i].ID < heroes[j].ID })
	return heroes, nil
}

const heroPrefix = "npc_dota_hero_"

// Patch is one game patch from dotaconstants patch.json.
type Patch struct {
	Name       string
	ReleasedAt time.Time
}

// ParsePatches reads dotaconstants patch.json, in release order.
func ParsePatches(b []byte) ([]Patch, error) {
	var raw []struct {
		Name string    `json:"name"`
		Date time.Time `json:"date"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, fmt.Errorf("parse patches: %w", err)
	}
	patches := make([]Patch, 0, len(raw))
	for _, r := range raw {
		if r.Name == "" {
			return nil, fmt.Errorf("parse patches: entry without name")
		}
		patches = append(patches, Patch{Name: r.Name, ReleasedAt: r.Date.UTC()})
	}
	sort.SliceStable(patches, func(i, j int) bool { return patches[i].ReleasedAt.Before(patches[j].ReleasedAt) })
	return patches, nil
}
