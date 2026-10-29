// Command draftmind runs the Draftmind API and its maintenance tasks.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jholhewres/draftmind/api/internal/auth"
	"github.com/jholhewres/draftmind/api/internal/catalog"
	"github.com/jholhewres/draftmind/api/internal/config"
	"github.com/jholhewres/draftmind/api/internal/db"
	"github.com/jholhewres/draftmind/api/internal/decision"
	"github.com/jholhewres/draftmind/api/internal/gamesync"
	"github.com/jholhewres/draftmind/api/internal/httpapi"
	"github.com/jholhewres/draftmind/api/internal/match"
	"github.com/jholhewres/draftmind/api/internal/persona"
	"github.com/jholhewres/draftmind/api/internal/writer"
)

// version is set at build time: -ldflags "-X main.version=<v>".
var version = "dev"

const usage = "usage: draftmind <version|serve|migrate|sync heroes|sync details|sync patchnotes [N]|sync stats|sync builds|bench photo [dir]>"

func main() {
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, usage)
		os.Exit(2)
	}
	if os.Args[1] == "version" {
		fmt.Println(version)
		return
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1], log); err != nil {
		log.Error("draftmind failed", "cmd", os.Args[1], "err", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, cmd string, log *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	pool, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	switch cmd {
	case "migrate":
		applied, err := db.Migrate(ctx, pool)
		log.Info("migrations applied", "count", len(applied), "names", applied)
		return err
	case "sync":
		return runSync(ctx, cfg, pool, log)
	case "serve":
		cat := catalog.NewStore(pool)
		srv := httpapi.New(pool, cat, log).WithMatches(match.NewStore(pool)).WithPersonas(persona.NewStore(pool)).
			WithWeb(cfg.WebDir).WithTrustedProxies(cfg.TrustedProxies...)
		if cfg.OpenRouterAPIKey != "" {
			jev := decision.New(cfg.OpenRouterBaseURL, cfg.OpenRouterAPIKey, cfg.DecisionModel,
				cfg.DecisionProvider, decision.NewPGLog(pool, log))
			srv.WithDecider(jev)
			readers := newPhotoReaders(cfg, decision.NewPGLog(pool, log))
			srv.WithPhotoReader(readers.chain(cfg.PhotoReader))
			w := writer.New(cfg.OpenRouterBaseURL, cfg.OpenRouterAPIKey, cfg.WriterModel, writer.NewPGLog(pool, log))
			srv.WithWriter(w)
			coach := &match.Coach{Parse: w, Jev: jev, Write: w, Facts: cat, Note: w}
			srv.WithCoach(coach).WithNoter(coach).WithResultParser(w).WithEndReader(readers.vision)
			srv.WithTranscriber(writer.New(cfg.OpenRouterBaseURL, cfg.OpenRouterAPIKey, cfg.STTModel, writer.NewPGLog(pool, log)))
			go warmPortraits(ctx, cat, readers.jev.Portraits, log)
		} else {
			log.Warn("OPENROUTER_API_KEY is not set: suggestions keep the code ranking, tips and coach answers are templates, photos cannot be read and voice is off")
		}
		accounts, err := newAccounts(cfg, pool, log)
		if err != nil {
			return err
		}
		srv.WithAuth(accounts)
		if cfg.SyncSchedule {
			go newScheduler(cfg, pool, log).Run(ctx)
		}
		return serve(ctx, cfg, srv.Handler(), log)
	case "bench":
		return runBench(ctx, cfg, pool, log)
	default:
		return fmt.Errorf("unknown command %q; %s", cmd, usage)
	}
}

// newAccounts sets up sign-in (ADR-0008): links by SMTP, else to the log
// in development, else e-mail sign-in is off and Steam still works.
func newAccounts(cfg config.Config, pool *pgxpool.Pool, log *slog.Logger) (*auth.Service, error) {
	accounts, err := auth.New(pool, cfg.PublicURL, log)
	if err != nil {
		return nil, err
	}
	switch {
	case cfg.SMTPHost != "":
		accounts.WithMailer(auth.SMTP{Addr: net.JoinHostPort(cfg.SMTPHost, cfg.SMTPPort), Username: cfg.SMTPUser,
			Password: cfg.SMTPPass, From: cfg.MailFrom})
	case accounts.Dev():
		if err := accounts.WithLogMailer(); err != nil {
			return nil, err
		}
		log.Info("SMTP_HOST is not set: sign-in links are written to this log (development)")
	default:
		log.Warn("SMTP_HOST is not set: sign-in by e-mail link is off, password and Steam sign-in still work")
	}
	return accounts, nil
}

// newScheduler keeps the synced data current from inside serve
// (SYNC_SCHEDULE=on): a new patch is synced from every source and promoted
// only when all succeeded; stats and builds are refreshed daily.
func newScheduler(cfg config.Config, pool *pgxpool.Pool, log *slog.Logger) *gamesync.Scheduler {
	client := &http.Client{Timeout: 30 * time.Second}
	source := func() gamesync.PatchSource {
		return gamesync.NewHTTPSource(gamesync.DefaultGitHubAPI, gamesync.DefaultDotaconstantsBase, cfg.DotaconstantsRef, client)
	}
	feed := gamesync.NewHTTPFeed(gamesync.DefaultDatafeedBase, gamesync.DefaultDatafeedInterval, client)
	od := gamesync.NewHTTPOpenDota(gamesync.DefaultOpenDotaBase, cfg.OpenDotaAPIKey, client)
	return gamesync.NewScheduler(pool, source, feed, od, log.With("component", "sync"))
}

func runSync(ctx context.Context, cfg config.Config, pool *pgxpool.Pool, log *slog.Logger) error {
	client := &http.Client{Timeout: 30 * time.Second}
	target := ""
	if len(os.Args) >= 3 {
		target = os.Args[2]
	}
	switch target {
	case "heroes":
		src := gamesync.NewHTTPSource(gamesync.DefaultGitHubAPI, gamesync.DefaultDotaconstantsBase, cfg.DotaconstantsRef, client)
		syncer := gamesync.NewSyncer(pool, src, log)
		if _, err := syncer.Heroes(ctx); err != nil {
			return err
		}
		// Same source, so the items come from the revision the heroes pinned.
		_, err := syncer.Items(ctx)
		return err
	case "details":
		feed := gamesync.NewHTTPFeed(gamesync.DefaultDatafeedBase, gamesync.DefaultDatafeedInterval, client)
		_, err := gamesync.NewSyncer(pool, nil, log).Details(ctx, feed)
		return err
	case "patchnotes":
		// Every patch of the list by default; a number keeps the last N.
		last := 0
		if len(os.Args) >= 4 {
			n, err := strconv.Atoi(os.Args[3])
			if err != nil || n < 0 {
				return fmt.Errorf("sync patchnotes takes a patch count; %s", usage)
			}
			last = n
		}
		feed := gamesync.NewHTTPFeed(gamesync.DefaultDatafeedBase, gamesync.DefaultDatafeedInterval, client)
		_, err := gamesync.NewSyncer(pool, nil, log).PatchNotes(ctx, feed, last)
		return err
	case "stats":
		od := gamesync.NewHTTPOpenDota(gamesync.DefaultOpenDotaBase, cfg.OpenDotaAPIKey, client)
		_, err := gamesync.NewSyncer(pool, nil, log).Stats(ctx, od)
		return err
	case "builds":
		// Each hero's item popularity by phase, for the item build (B1).
		od := gamesync.NewHTTPOpenDota(gamesync.DefaultOpenDotaBase, cfg.OpenDotaAPIKey, client)
		_, err := gamesync.NewSyncer(pool, nil, log).ItemPopularity(ctx, od)
		return err
	default:
		return fmt.Errorf("unknown sync target; %s", usage)
	}
}

func serve(ctx context.Context, cfg config.Config, h http.Handler, log *slog.Logger) error {
	srv := &http.Server{Addr: cfg.HTTPAddr, Handler: h, ReadHeaderTimeout: 10 * time.Second}
	errc := make(chan error, 1)
	go func() { errc <- srv.ListenAndServe() }()
	log.Info("listening", "version", version, "config", cfg.String())
	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}
	shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdown); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
