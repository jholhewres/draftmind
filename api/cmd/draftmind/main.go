// Command draftmind runs the Draftmind API and its maintenance tasks.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jholhewres/draftmind/api/internal/catalog"
	"github.com/jholhewres/draftmind/api/internal/config"
	"github.com/jholhewres/draftmind/api/internal/db"
	"github.com/jholhewres/draftmind/api/internal/gamesync"
	"github.com/jholhewres/draftmind/api/internal/httpapi"
)

const usage = "usage: draftmind <serve|migrate|sync heroes|sync details|sync stats>"

func main() {
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, usage)
		os.Exit(2)
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
		return serve(ctx, cfg, httpapi.New(pool, catalog.NewStore(pool), log).Handler(), log)
	default:
		return fmt.Errorf("unknown command %q; %s", cmd, usage)
	}
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
		_, err := gamesync.NewSyncer(pool, src, log).Heroes(ctx)
		return err
	case "details":
		feed := gamesync.NewHTTPFeed(gamesync.DefaultDatafeedBase, gamesync.DefaultDatafeedInterval, client)
		_, err := gamesync.NewSyncer(pool, nil, log).Details(ctx, feed)
		return err
	case "stats":
		od := gamesync.NewHTTPOpenDota(gamesync.DefaultOpenDotaBase, cfg.OpenDotaAPIKey, client)
		_, err := gamesync.NewSyncer(pool, nil, log).Stats(ctx, od)
		return err
	default:
		return fmt.Errorf("unknown sync target; %s", usage)
	}
}

func serve(ctx context.Context, cfg config.Config, h http.Handler, log *slog.Logger) error {
	srv := &http.Server{Addr: cfg.HTTPAddr, Handler: h, ReadHeaderTimeout: 10 * time.Second}
	errc := make(chan error, 1)
	go func() { errc <- srv.ListenAndServe() }()
	log.Info("listening", "config", cfg.String())
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
