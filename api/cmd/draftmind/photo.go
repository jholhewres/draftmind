package main

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/jholhewres/draftmind/api/internal/catalog"
	"github.com/jholhewres/draftmind/api/internal/config"
	"github.com/jholhewres/draftmind/api/internal/decision"
	"github.com/jholhewres/draftmind/api/internal/photo"
)

// heroImageBase is the CDN the synced hero image paths are relative to.
const heroImageBase = "https://cdn.cloudflare.steamstatic.com"

// Photo read timings (ADR-0006): the default reader has the SPEC's 3.5 s
// read budget; the fallback gets a little more, since it is the last try.
const (
	photoPrimaryDeadline   = 3500 * time.Millisecond
	photoSecondaryDeadline = 5 * time.Second
	// A slot's portrait must be at least this likely for the slot to stand.
	photoMinConfidence = 0.5
)

type photoReaders struct {
	jev    *photo.Jev
	vision *photo.Vision
}

func newPhotoReaders(cfg config.Config, log decision.Logger) photoReaders {
	jevClient := decision.New(cfg.OpenRouterBaseURL, cfg.OpenRouterAPIKey, cfg.PhotoJevModel, cfg.DecisionProvider, log)
	// A photo call carries images: slower than a text decision. No hedge:
	// a second copy of every request doubles the upload and slows them all.
	jevClient.Deadline, jevClient.HedgeAfter, jevClient.MaxAttempts = 3*time.Second, -1, 2
	jevClient.HTTP = &http.Client{Transport: http1Transport()}
	return photoReaders{
		jev: &photo.Jev{
			Ask:           jevClient,
			Portraits:     photo.NewPortraits(cfg.DataDir, heroImageBase, &http.Client{Timeout: 15 * time.Second}),
			Shortlist:     5,
			MinConfidence: photoMinConfidence,
		},
		vision: &photo.Vision{
			BaseURL: cfg.OpenRouterBaseURL, APIKey: cfg.OpenRouterAPIKey, Model: cfg.VisionModel,
			HTTP: &http.Client{}, MinConfidence: photoMinConfidence,
		},
	}
}

// http1Transport opens one connection per concurrent request. Over HTTP/2
// a read's ten slot requests share one connection and its flow-control
// window, and their uploads queue behind each other.
func http1Transport() *http.Transport {
	// A new transport: a clone of the default one keeps its TLS config,
	// which already offers h2.
	t := &http.Transport{
		Proxy: http.ProxyFromEnvironment, TLSHandshakeTimeout: 10 * time.Second,
		IdleConnTimeout: 90 * time.Second, MaxIdleConnsPerHost: 16,
		Protocols: new(http.Protocols),
	}
	t.Protocols.SetHTTP1(true)
	return t
}

// chain reads with the named default reader and falls back to the other.
func (r photoReaders) chain(primary string) photo.Chain {
	c := photo.Chain{Primary: r.jev, Secondary: r.vision, PrimaryDeadline: photoPrimaryDeadline, SecondaryDeadline: photoSecondaryDeadline}
	if primary == "vision" {
		c.Primary, c.Secondary = r.vision, r.jev
	}
	return c
}

// warmPortraits fetches the current heroes' portraits at start, so the first
// photo is not slowed by the CDN.
func warmPortraits(ctx context.Context, cat *catalog.Store, p *photo.Portraits, log *slog.Logger) {
	_, heroes, err := cat.Heroes(ctx)
	if err == nil {
		err = p.Load(ctx, heroes)
	}
	if err != nil {
		log.Warn("portraits not cached; the Jev photo reader will retry on its first read", "err", err)
	}
}
