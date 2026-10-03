// Command flagpole runs maintenance tasks: migrations, creating the first
// admin, and loading demo data.
//
//	flagpole migrate up|down|reset|status
//	flagpole admin create -email ada@example.com -name Ada -password ...
//	flagpole seed [-demo-env ../demo/.env.local]
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/0mjs/flagpole/api/internal/identity"
	"github.com/0mjs/flagpole/api/internal/platform/config"
	"github.com/0mjs/flagpole/api/internal/platform/mail"
	"github.com/0mjs/flagpole/api/internal/platform/postgres"
	"github.com/0mjs/flagpole/api/internal/platform/redisx"
	"github.com/0mjs/flagpole/api/internal/seed"
	"github.com/0mjs/flagpole/api/internal/server"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "flagpole:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	ctx := context.Background()
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if len(args) >= 1 && args[0] == "migrate" {
		direction := "up"
		if len(args) > 1 {
			direction = args[1]
		}
		return postgres.Migrate(ctx, cfg.DatabaseURL, direction)
	}
	if len(args) >= 2 && args[0] == "admin" && args[1] == "create" {
		fs := flag.NewFlagSet("admin create", flag.ContinueOnError)
		email := fs.String("email", "", "the admin's email")
		name := fs.String("name", "", "the admin's name")
		password := fs.String("password", "", "at least 12 characters")
		if err := fs.Parse(args[2:]); err != nil {
			return err
		}
		if *email == "" || *name == "" || len(*password) < 12 {
			return fmt.Errorf("admin create needs -email, -name and a -password of at least 12 characters")
		}
		if err := postgres.Migrate(ctx, cfg.DatabaseURL, "up"); err != nil {
			return err
		}
		db, err := postgres.Open(ctx, cfg.DatabaseURL)
		if err != nil {
			return err
		}
		defer db.Close()
		rdb, err := redisx.Open(ctx, cfg.RedisURL)
		if err != nil {
			return err
		}
		defer rdb.Close()
		svc := identity.NewService(db, rdb, mail.New(cfg.SMTPAddr, cfg.MailFrom), cfg.PublicURL, cfg.SessionLifetime)
		u, err := svc.CreateAdmin(ctx, *email, *name, *password)
		if err != nil {
			return err
		}
		fmt.Printf("created admin %s (%s)\n", u.Email, u.ID)
		return nil
	}
	if len(args) >= 1 && args[0] == "seed" {
		fs := flag.NewFlagSet("seed", flag.ContinueOnError)
		demoEnv := fs.String("demo-env", "", "write the demo shop's settings to this file")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		sys, err := server.Build(ctx, withMigrations(cfg), slog.New(slog.NewTextHandler(io.Discard, nil)), mail.New(cfg.SMTPAddr, cfg.MailFrom))
		if err != nil {
			return err
		}
		defer sys.Close()
		res, err := seed.Run(ctx, sys.DB, sys.Identity, sys.Projects, sys.Flags)
		if err != nil {
			return err
		}
		fmt.Println("seeded. Sign in at", cfg.PublicURL, "with password", seed.Password, "as:")
		for _, u := range res.Users {
			fmt.Printf("  %-22s %s\n", u.Email, u.Role)
		}
		fmt.Println("SDK key for web-app/production:", res.SDKKey)
		fmt.Println("SDK key for web-app/development (demo shop):", res.DemoKey)
		if *demoEnv != "" {
			env := fmt.Sprintf("VITE_FLAGPOLE_URL=http://localhost%s\nVITE_FLAGPOLE_KEY=%s\n", cfg.Addr, res.DemoKey)
			if err := os.WriteFile(*demoEnv, []byte(env), 0o600); err != nil {
				return err
			}
			fmt.Println("wrote", *demoEnv)
		}
		return nil
	}
	return fmt.Errorf("usage: flagpole migrate up|down|reset|status | flagpole admin create -email E -name N -password P | flagpole seed [-demo-env FILE]")
}

func withMigrations(cfg config.Config) config.Config {
	cfg.AutoMigrate = true
	return cfg
}
