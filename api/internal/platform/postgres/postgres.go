// Package postgres opens the connection pool, runs migrations, and wraps
// transactions around the generated queries.
package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" database/sql driver for goose
	"github.com/pressly/goose/v3"

	"github.com/0mjs/flagpole/api/db"
	"github.com/0mjs/flagpole/api/internal/store"
)

// DB is the pool and the queries that run on it outside a transaction.
type DB struct {
	Pool *pgxpool.Pool
	*store.Queries
}

func Open(ctx context.Context, url string) (*DB, error) {
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("postgres: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("postgres: %w", err)
	}
	return &DB{Pool: pool, Queries: store.New(pool)}, nil
}

func (db *DB) Close() { db.Pool.Close() }

// Tx runs fn in a transaction, committing when it returns nil.
func (db *DB) Tx(ctx context.Context, fn func(q *store.Queries) error) error {
	return pgx.BeginFunc(ctx, db.Pool, func(tx pgx.Tx) error {
		return fn(db.Queries.WithTx(tx))
	})
}

// Migrate applies the embedded migrations. direction is "up", "down" or
// "status".
func Migrate(ctx context.Context, url, direction string) error {
	conn, err := sql.Open("pgx", url)
	if err != nil {
		return err
	}
	defer conn.Close()
	goose.SetBaseFS(db.Migrations)
	goose.SetLogger(goose.NopLogger())
	if err := goose.SetDialect("postgres"); err != nil {
		return err
	}
	switch direction {
	case "up":
		return goose.UpContext(ctx, conn, "migrations")
	case "down":
		return goose.DownContext(ctx, conn, "migrations")
	case "reset":
		// Every migration down, then up again: an empty database.
		if err := goose.ResetContext(ctx, conn, "migrations"); err != nil {
			return err
		}
		return goose.UpContext(ctx, conn, "migrations")
	case "status":
		goose.SetLogger(log.Default())
		return goose.StatusContext(ctx, conn, "migrations")
	}
	return fmt.Errorf("postgres: unknown migration direction %q", direction)
}

// IsNotFound reports whether err is a query that found no row.
func IsNotFound(err error) bool { return errors.Is(err, pgx.ErrNoRows) }

// IsUniqueViolation reports whether err broke a unique constraint.
func IsUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// IsCheckViolation reports whether err broke a CHECK constraint or domain.
func IsCheckViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23514"
}
