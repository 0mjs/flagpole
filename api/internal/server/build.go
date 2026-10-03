package server

import (
	"context"
	"log/slog"

	"github.com/redis/go-redis/v9"
	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/0mjs/zinc"

	"github.com/0mjs/flagpole/api/internal/evaluation"
	"github.com/0mjs/flagpole/api/internal/flags"
	"github.com/0mjs/flagpole/api/internal/identity"
	"github.com/0mjs/flagpole/api/internal/insights"
	"github.com/0mjs/flagpole/api/internal/platform/config"
	"github.com/0mjs/flagpole/api/internal/platform/kafka"
	"github.com/0mjs/flagpole/api/internal/platform/postgres"
	"github.com/0mjs/flagpole/api/internal/platform/redisx"
	"github.com/0mjs/flagpole/api/internal/projects"
)

// System is the app with the connections it holds.
type System struct {
	App       *zinc.App
	DB        *postgres.DB
	Redis     *redis.Client
	Producer  *kgo.Client
	Snapshots *evaluation.Snapshots
	Identity  *identity.Service
	Projects  *projects.Service
	Flags     *flags.Service
}

// Build connects to Postgres, Redis and Kafka, migrates if configured, and
// assembles the app. Close releases the connections.
func Build(ctx context.Context, cfg config.Config, log *slog.Logger, mailer identity.Mailer) (*System, error) {
	s := &System{}
	if cfg.AutoMigrate {
		if err := postgres.Migrate(ctx, cfg.DatabaseURL, "up"); err != nil {
			return nil, err
		}
	}
	var err error
	if s.DB, err = postgres.Open(ctx, cfg.DatabaseURL); err != nil {
		return nil, err
	}
	if s.Redis, err = redisx.Open(ctx, cfg.RedisURL); err != nil {
		s.Close()
		return nil, err
	}
	if s.Producer, err = kafka.NewClient(cfg.KafkaBrokers); err != nil {
		s.Close()
		return nil, err
	}
	if err := kafka.EnsureTopics(ctx, s.Producer); err != nil {
		s.Close()
		return nil, err
	}

	s.Snapshots = evaluation.NewSnapshots(s.DB, s.Redis)
	s.Identity = identity.NewService(s.DB, s.Redis, mailer, cfg.PublicURL, cfg.SessionLifetime)
	projectSvc := projects.NewService(s.DB)
	s.Projects = projectSvc
	s.Flags = flags.NewService(s.DB, projectSvc, s.Snapshots)
	projectSvc.OnKeyRevoked = func(ctx context.Context, hash []byte) {
		s.Redis.Del(ctx, evaluation.SDKKeyCacheKey(hash))
	}
	s.App = New(Deps{
		Config:   cfg,
		DB:       s.DB,
		Redis:    s.Redis,
		Log:      log,
		Identity: s.Identity,
		Projects: projectSvc,
		Flags:    s.Flags,
		Insights: insights.NewService(s.DB, projectSvc),
		SDK:      evaluation.NewAPI(s.DB, s.Redis, s.Snapshots, s.Producer, log),
	})
	return s, nil
}

func (s *System) Close() {
	if s.Producer != nil {
		s.Producer.Close()
	}
	if s.Redis != nil {
		s.Redis.Close()
	}
	if s.DB != nil {
		s.DB.Close()
	}
}
