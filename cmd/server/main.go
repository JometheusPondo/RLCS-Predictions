// Package main is the entrypoint for the RLCS Predictions server.
//
// Loads config, opens SQLite, runs migrations, seeds the active tournament,
// constructs the configured match source (Liquipedia or Sheet) and starts
// the syncer poller in the background, then serves the HTTP API plus the
// embedded frontend.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	app "github.com/jometheuspondo/rlcs-predictions"
	"github.com/jometheuspondo/rlcs-predictions/internal/api"
	"github.com/jometheuspondo/rlcs-predictions/internal/config"
	"github.com/jometheuspondo/rlcs-predictions/internal/db"
	"github.com/jometheuspondo/rlcs-predictions/internal/liquipedia"
	"github.com/jometheuspondo/rlcs-predictions/internal/matchsource"
	"github.com/jometheuspondo/rlcs-predictions/internal/models"
	"github.com/jometheuspondo/rlcs-predictions/internal/syncer"
)

func main() {
	// Bootstrap logger so config errors get emitted before the real logger is set up.
	bootstrap := slog.New(slog.NewTextHandler(os.Stderr, nil))

	cfg, err := config.Load()
	if err != nil {
		bootstrap.Error("config load failed", "err", err)
		os.Exit(1)
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel}))
	slog.SetDefault(logger)

	if err := ensureDBDir(cfg.DatabasePath); err != nil {
		logger.Error("create db directory", "path", cfg.DatabasePath, "err", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	database, err := db.Open(cfg.DatabasePath)
	if err != nil {
		logger.Error("db open failed", "path", cfg.DatabasePath, "err", err)
		os.Exit(1)
	}
	defer func() { _ = database.Close() }()

	if err := database.Migrate(ctx, logger); err != nil {
		logger.Error("db migration failed", "err", err)
		os.Exit(1)
	}

	tournament, err := database.GetOrCreateActiveTournament(ctx, cfg.LiquipediaPage, deriveTournamentName(cfg.LiquipediaPage))
	if err != nil {
		logger.Error("seed tournament failed", "err", err)
		os.Exit(1)
	}
	logger.Info("active tournament ready",
		"id", tournament.ID,
		"page", tournament.LiquipediaPage,
		"name", tournament.Name,
	)

	source, err := buildMatchSource(cfg, tournament, logger)
	if err != nil {
		logger.Error("match source init failed", "source", cfg.MatchSource, "err", err)
		os.Exit(1)
	}
	logger.Info("match source ready",
		"source", cfg.MatchSource,
		"interval", cfg.ActivePollInterval(),
	)

	poller := syncer.NewPoller(
		source,
		database,
		tournament,
		cfg.ActivePollInterval(),
		logger.With("component", "poller"),
	)
	go poller.Run(ctx)

	if cfg.DevMode {
		logger.Warn("dev mode enabled: POST /api/sync/now is registered")
	}

	// The embedded frontend. fs.Sub never fails on a valid embed.FS, but the
	// error is surfaced rather than ignored on principle.
	distFS, err := app.DistFS()
	if err != nil {
		logger.Error("embedded frontend init failed", "err", err)
		os.Exit(1)
	}

	handler := api.NewRouter(api.Deps{
		DB:         database,
		Tournament: tournament,
		Poller:     poller,
		Logger:     logger.With("component", "api"),
		DevMode:    cfg.DevMode,
		DistFS:     distFS,
	})

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	serverErr := make(chan error, 1)
	go func() {
		logger.Info("server starting", "port", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	select {
	case <-ctx.Done():
		logger.Info("shutdown signal received")
	case err := <-serverErr:
		logger.Error("server error", "err", err)
		os.Exit(1)
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Warn("graceful shutdown failed", "err", err)
	}
	logger.Info("server stopped")
}

// buildMatchSource constructs the configured match source. Returns an error
// if the source-specific configuration is invalid (e.g., placeholder
// Liquipedia user-agent in production, missing Google Sheets IDs).
func buildMatchSource(cfg *config.Config, tournament *models.Tournament, logger *slog.Logger) (matchsource.MatchSource, error) {
	switch cfg.MatchSource {
	case config.MatchSourceLiquipedia:
		if cfg.HasPlaceholderUserAgent() {
			logger.Warn("LIQUIPEDIA_USER_AGENT still contains placeholder contact info; set a real email before any production sync")
		}
		lpClient, err := liquipedia.NewClient(liquipedia.ClientOptions{
			UserAgent:        cfg.LiquipediaUserAgent,
			SaveResponsePath: "tmp/last_response.html",
			Logger:           logger.With("component", "liquipedia-client"),
		})
		if err != nil {
			return nil, fmt.Errorf("liquipedia client: %w", err)
		}
		return matchsource.NewLiquipediaSource(lpClient, cfg.LiquipediaPage, tournament.ID), nil

	case config.MatchSourceSheet:
		return matchsource.NewSheetSource(matchsource.SheetSourceOptions{
			SpreadsheetID: cfg.SheetSpreadsheetID,
			GroupsGID:     cfg.SheetGroupsGID,
			BracketGID:    cfg.SheetBracketGID,
			ScheduleGID:   cfg.SheetScheduleGID,
			TournamentID:  tournament.ID,
			Logger:        logger.With("component", "sheet-source"),
		})

	default:
		return nil, fmt.Errorf("unknown match source %q", cfg.MatchSource)
	}
}

// ensureDBDir creates the parent directory of the SQLite file if missing.
// sqlite won't auto-create directories — this is the difference between a
// useful error message and "unable to open database file" on a fresh checkout.
func ensureDBDir(path string) error {
	dir := filepath.Dir(path)
	if dir == "." || dir == "" {
		return nil
	}
	return os.MkdirAll(dir, 0o755)
}

// deriveTournamentName turns a Liquipedia page slug into a readable name.
// "Rocket_League_Championship_Series/2026/Paris_Major"
//   → "Rocket League Championship Series 2026 Paris Major"
func deriveTournamentName(page string) string {
	n := strings.ReplaceAll(page, "/", " ")
	return strings.ReplaceAll(n, "_", " ")
}
