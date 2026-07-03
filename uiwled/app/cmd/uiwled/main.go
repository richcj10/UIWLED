// Command uiwled runs the UIWLED daemon inside the Home Assistant addon
// container.
//
// It wires the four internal packages together:
//
//   - config     — reads /data/options.json (the HA addon options)
//   - switches   — one supervise-goroutine per switch, owns the SSH connection
//   - engine     — a ticker that renders effect frames and pushes them via switches
//   - api        — HTTP server for the built-in web UI and REST endpoints
//
// The process exits cleanly on SIGINT/SIGTERM, which cascades a context
// cancellation through every component.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/richcj10/uiwled/internal/api"
	"github.com/richcj10/uiwled/internal/config"
	"github.com/richcj10/uiwled/internal/engine"
	"github.com/richcj10/uiwled/internal/switches"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	cfg, err := config.Load(os.Getenv("UIWLED_CONFIG"))
	if err != nil {
		slog.Error("failed to load config", "err", err)
		os.Exit(1)
	}
	config.ApplyLogLevel(cfg.LogLevel)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mgr := switches.NewManager(cfg.Switches)
	if err := mgr.Start(ctx); err != nil {
		slog.Error("failed to start switch manager", "err", err)
		os.Exit(1)
	}

	eng := engine.New(mgr, cfg.TargetFPS)
	eng.Start(ctx)

	srv := api.NewServer(cfg.ListenPort, mgr, eng)
	go func() {
		if err := srv.Run(ctx); err != nil {
			slog.Error("http server exited", "err", err)
			cancel()
		}
	}()

	slog.Info("uiwled started", "port", cfg.ListenPort, "switches", len(cfg.Switches), "fps", cfg.TargetFPS)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	slog.Info("shutting down")
	cancel()
	mgr.Stop()
}
