// Package config reads the runtime settings from the environment.
package config

import (
	"errors"
	"fmt"
	"math"
	"net/netip"
	"os"
	"strconv"
	"strings"
)

// Config holds the settings every subcommand may need.
type Config struct {
	HTTPAddr          string
	DatabaseURL       string
	OpenRouterBaseURL string
	OpenRouterAPIKey  string
	DecisionModel     string
	DecisionProvider  string
	VisionModel       string
	PhotoReader       string // the default photo reader, jev or vision; the other is its fallback
	PhotoJevModel     string // Jev model for photo reads; Box questions need 1.14
	DataDir           string // local data outside git, e.g. the portrait cache
	WriterModel       string // writes the tips (ADR-0003 step 4)
	STTModel          string // transcribes push-to-talk clips (ADR-0005)
	OpenDotaAPIKey    string
	DotaconstantsRef  string
	SyncSchedule      bool   // SYNC_SCHEDULE=on: serve keeps the synced data current
	LiveCoach         bool   // LIVE_COACH=on: the live coach ticks (ADR-0014); off until benchmarked
	LiveWriter        bool   // LIVE_WRITER=on: the writer words live events; off, templates do
	PublicURL         string // where the web app is served; sign-in links and Steam return there
	SMTPHost          string // empty: links go to the log in development, else e-mail sign-in is off
	SMTPPort          string
	SMTPUser          string
	SMTPPass          string
	MailFrom          string
	WebDir            string       // the web build served at /; empty serves the API alone
	TrustedProxies    []netip.Addr // TRUSTED_PROXY: proxies whose X-Real-IP / X-Forwarded-For is believed
	SessionBudgetUSD  float64      // MODEL_BUDGET_SESSION_USD: model spend per match session per day; 0 is off
	ClientBudgetUSD   float64      // MODEL_BUDGET_CLIENT_USD: model spend per client (account or IP) per day; 0 is off
}

// Load reads the environment, applying defaults where a value is optional.
func Load() (Config, error) {
	return load(os.Getenv)
}

func load(getenv func(string) string) (Config, error) {
	get := func(key, def string) string {
		if v := strings.TrimSpace(getenv(key)); v != "" {
			return v
		}
		return def
	}
	c := Config{
		HTTPAddr:          get("HTTP_ADDR", "127.0.0.1:8080"),
		DatabaseURL:       get("DATABASE_URL", ""),
		OpenRouterBaseURL: get("OPENROUTER_BASE_URL", "https://openrouter.ai/api"),
		OpenRouterAPIKey:  get("OPENROUTER_API_KEY", ""),
		DecisionModel:     get("DECISION_MODEL", "typesafe/jev-1.13"),
		DecisionProvider:  get("DECISION_PROVIDER", "typesafe"),
		VisionModel:       get("VISION_MODEL", "google/gemini-3.1-flash-lite"),
		PhotoReader:       get("PHOTO_READER", "jev"),
		PhotoJevModel:     get("PHOTO_JEV_MODEL", "typesafe/jev-1.14"),
		DataDir:           get("DATA_DIR", "data"),
		WriterModel:       get("WRITER_MODEL", "openai/gpt-6-luna"),
		STTModel:          get("STT_MODEL", "google/gemini-3.1-flash-lite"),
		OpenDotaAPIKey:    get("OPENDOTA_API_KEY", ""),
		DotaconstantsRef:  get("DOTACONSTANTS_REF", "master"),
		PublicURL:         get("PUBLIC_URL", "http://localhost:5173"),
		SMTPHost:          get("SMTP_HOST", ""),
		SMTPPort:          get("SMTP_PORT", "587"),
		SMTPUser:          get("SMTP_USER", ""),
		SMTPPass:          get("SMTP_PASS", ""),
		MailFrom:          get("MAIL_FROM", ""),
		WebDir:            get("WEB_DIR", ""),
	}
	for _, p := range strings.Split(get("TRUSTED_PROXY", ""), ",") {
		if p = strings.TrimSpace(p); p == "" {
			continue
		}
		a, err := netip.ParseAddr(p)
		if err != nil {
			return c, fmt.Errorf("TRUSTED_PROXY must be IP addresses, not %q", p)
		}
		c.TrustedProxies = append(c.TrustedProxies, a)
	}
	switch v := get("SYNC_SCHEDULE", "off"); v {
	case "on", "off":
		c.SyncSchedule = v == "on"
	default:
		return c, fmt.Errorf("SYNC_SCHEDULE must be on or off, not %q", v)
	}
	for _, f := range []struct {
		key string
		dst *bool
	}{{"LIVE_COACH", &c.LiveCoach}, {"LIVE_WRITER", &c.LiveWriter}} {
		switch v := get(f.key, "off"); v {
		case "on", "off":
			*f.dst = v == "on"
		default:
			return c, fmt.Errorf("%s must be on or off, not %q", f.key, v)
		}
	}
	for _, f := range []struct {
		key, def string
		dst      *float64
	}{{"MODEL_BUDGET_SESSION_USD", "0.25", &c.SessionBudgetUSD}, {"MODEL_BUDGET_CLIENT_USD", "1", &c.ClientBudgetUSD}} {
		v, err := strconv.ParseFloat(get(f.key, f.def), 64)
		if err != nil || v < 0 || math.IsNaN(v) || math.IsInf(v, 0) {
			return c, fmt.Errorf("%s must be a number of USD, 0 or more", f.key)
		}
		*f.dst = v
	}
	if c.DatabaseURL == "" {
		return c, errors.New("DATABASE_URL is required")
	}
	if c.PhotoReader != "jev" && c.PhotoReader != "vision" {
		return c, fmt.Errorf("PHOTO_READER must be jev or vision, not %q", c.PhotoReader)
	}
	if c.SMTPHost != "" && c.MailFrom == "" {
		return c, errors.New("MAIL_FROM is required with SMTP_HOST")
	}
	return c, nil
}

// String describes the config without secrets.
func (c Config) String() string {
	return fmt.Sprintf("addr=%s public_url=%s decision_model=%s openrouter_key_set=%t smtp_set=%t sync_schedule=%t web_dir=%q trusted_proxies=%v budget_session_usd=%v budget_client_usd=%v",
		c.HTTPAddr, c.PublicURL, c.DecisionModel, c.OpenRouterAPIKey != "", c.SMTPHost != "", c.SyncSchedule, c.WebDir, c.TrustedProxies, c.SessionBudgetUSD, c.ClientBudgetUSD)
}
