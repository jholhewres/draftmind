// Package config reads the runtime settings from the environment.
package config

import (
	"errors"
	"fmt"
	"os"
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
	PublicURL         string // where the web app is served; sign-in links and Steam return there
	SMTPHost          string // empty: links go to the log in development, else e-mail sign-in is off
	SMTPPort          string
	SMTPUser          string
	SMTPPass          string
	MailFrom          string
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
		SyncSchedule:      get("SYNC_SCHEDULE", "off") == "on",
	}
	if c.DatabaseURL == "" {
		return c, errors.New("DATABASE_URL is required")
	}
	if c.PhotoReader != "jev" && c.PhotoReader != "vision" {
		return c, fmt.Errorf("PHOTO_READER must be jev or vision, not %q", c.PhotoReader)
	}
	if v := get("SYNC_SCHEDULE", "off"); v != "on" && v != "off" {
		return c, fmt.Errorf("SYNC_SCHEDULE must be on or off, not %q", v)
	}
	if c.SMTPHost != "" && c.MailFrom == "" {
		return c, errors.New("MAIL_FROM is required with SMTP_HOST")
	}
	return c, nil
}

// String describes the config without secrets.
func (c Config) String() string {
	return fmt.Sprintf("addr=%s public_url=%s decision_model=%s openrouter_key_set=%t smtp_set=%t sync_schedule=%t",
		c.HTTPAddr, c.PublicURL, c.DecisionModel, c.OpenRouterAPIKey != "", c.SMTPHost != "", c.SyncSchedule)
}
