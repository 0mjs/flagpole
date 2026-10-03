// Command api serves the Flagpole HTTP API.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/0mjs/flagpole/api/internal/platform/config"
	"github.com/0mjs/flagpole/api/internal/platform/mail"
	"github.com/0mjs/flagpole/api/internal/server"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(log); err != nil {
		log.Error("api stopped", "err", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	sys, err := server.Build(ctx, cfg, log, mail.New(cfg.SMTPAddr, cfg.MailFrom))
	if err != nil {
		return err
	}
	defer sys.Close()
	// Catch spec and route mistakes before taking traffic.
	if err := sys.App.Validate(); err != nil {
		return err
	}
	log.Info("api listening", "addr", cfg.Addr, "env", cfg.Env)
	return sys.App.ListenContext(ctx, cfg.Addr)
}
