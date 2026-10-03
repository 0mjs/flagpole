// Package db holds the schema migrations and the SQL queries sqlc compiles
// into internal/store.
package db

import "embed"

// Migrations are applied in order by goose.
//
//go:embed migrations/*.sql
var Migrations embed.FS
