// Command worker publishes the outbox to Kafka and consumes Kafka to keep
// SDK snapshots fresh and count exposures.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/0mjs/flagpole/api/internal/evaluation"
	"github.com/0mjs/flagpole/api/internal/platform/config"
	"github.com/0mjs/flagpole/api/internal/platform/kafka"
	"github.com/0mjs/flagpole/api/internal/platform/postgres"
	"github.com/0mjs/flagpole/api/internal/platform/redisx"
	"github.com/0mjs/flagpole/api/internal/worker"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(log); err != nil {
		log.Error("worker stopped", "err", err)
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
	producer, err := kafka.NewClient(cfg.KafkaBrokers)
	if err != nil {
		return err
	}
	defer producer.Close()
	if err := kafka.EnsureTopics(ctx, producer); err != nil {
		return err
	}
	consumer, err := kafka.NewClient(cfg.KafkaBrokers,
		kgo.ConsumerGroup("flagpole-worker"),
		kgo.ConsumeTopics(kafka.TopicFlagChanges, kafka.TopicExposures),
		kgo.DisableAutoCommit(),
	)
	if err != nil {
		return err
	}
	defer consumer.Close()

	w := &worker.Worker{DB: db, Snapshots: evaluation.NewSnapshots(db, rdb), Producer: producer, Consumer: consumer, Log: log}
	log.Info("worker started")
	var wg sync.WaitGroup
	wg.Go(func() { w.RelayOutbox(ctx, 500*time.Millisecond) })
	wg.Go(func() { w.Consume(ctx) })
	wg.Wait()
	return nil
}
