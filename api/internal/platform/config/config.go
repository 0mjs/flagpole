// Package config reads Flagpole's settings from the environment. Every
// setting has a default that works with docker-compose.yml.
package config

import (
	"errors"
	"os"
	"strings"
	"time"
)

type Config struct {
	Env             string // "development" or "production"
	Addr            string
	DatabaseURL     string
	RedisURL        string
	KafkaBrokers    []string
	SessionSecret   []byte
	SessionLifetime time.Duration
	CookieSecure    bool
	// PublicURL is where the web app lives: invite and reset links point here,
	// and it's the origin CSRF protection trusts.
	PublicURL   string
	SMTPAddr    string
	MailFrom    string
	AutoMigrate bool
}

const devSecret = "development-only-session-secret-change-me-in-production"

func Load() (Config, error) {
	cfg := Config{
		Env:             get("FLAGPOLE_ENV", "development"),
		Addr:            get("FLAGPOLE_ADDR", ":8080"),
		DatabaseURL:     get("DATABASE_URL", "postgres://flagpole:flagpole@localhost:5432/flagpole?sslmode=disable"),
		RedisURL:        get("REDIS_URL", "redis://localhost:6379/0"),
		KafkaBrokers:    strings.Split(get("KAFKA_BROKERS", "localhost:19092"), ","),
		SessionSecret:   []byte(get("SESSION_SECRET", devSecret)),
		SessionLifetime: 30 * 24 * time.Hour,
		PublicURL:       strings.TrimSuffix(get("PUBLIC_URL", "http://localhost:5173"), "/"),
		SMTPAddr:        get("SMTP_ADDR", "localhost:1025"),
		MailFrom:        get("MAIL_FROM", "Flagpole <flagpole@localhost>"),
	}
	cfg.CookieSecure = get("COOKIE_SECURE", boolString(cfg.Production())) == "true"
	cfg.AutoMigrate = get("AUTO_MIGRATE", boolString(!cfg.Production())) == "true"
	if len(cfg.SessionSecret) < 32 {
		return cfg, errors.New("config: SESSION_SECRET must be at least 32 bytes")
	}
	if cfg.Production() && string(cfg.SessionSecret) == devSecret {
		return cfg, errors.New("config: set SESSION_SECRET in production")
	}
	return cfg, nil
}

func (c Config) Production() bool { return c.Env == "production" }

func get(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}

func boolString(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
