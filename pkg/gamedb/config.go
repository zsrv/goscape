// Package gamedb is the central-database client library. Every service
// that needs persistent state (login, friends, future consumers) opens
// its OWN pool through this package — services are independent clients
// of one central database, mirroring the historical model of a
// standalone account database that login servers, the website, and the
// friend server each connected to directly. There is no handle sharing,
// even when modules are co-resident in one process.
//
// The package owns all dialect knowledge: backend selection
// (sqlite | postgres), per-dialect pool posture, placeholder rebinding
// (Rebind), and the unified schema migration lineage (migrations/).
//
// Spec: docs/superpowers/specs/2026-07-05-central-db-consolidation-postgres-design.md
package gamedb

import (
	"flag"
	"fmt"
)

const (
	BackendSQLite   = "sqlite"
	BackendPostgres = "postgres"
)

// Config selects and configures the central-database backend. It is a
// top-level config section (database:) shared by every DB-using module,
// analogous to TS Environment.DB_BACKEND (src/db/query.ts:12-28
// @e1dea19f, sqlite | mysql there; goscape chooses postgres as its
// second backend instead of mysql — explicit user decision).
type Config struct {
	Backend  string         `yaml:"backend"`
	SQLite   SQLiteConfig   `yaml:"sqlite"`
	Postgres PostgresConfig `yaml:"postgres"`
}

type SQLiteConfig struct {
	DSN string `yaml:"dsn"`
}

type PostgresConfig struct {
	DSN          string `yaml:"dsn"`
	MaxOpenConns int    `yaml:"max_open_conns"`
}

func (c *Config) RegisterFlagsAndApplyDefaults(f *flag.FlagSet) {
	f.StringVar(&c.Backend, "database.backend", BackendSQLite, "Central database backend. Valid values: [sqlite, postgres].")
	f.StringVar(&c.SQLite.DSN, "database.sqlite-dsn", "data/goscape.db", "Central database SQLite DSN (file path).")
	f.StringVar(&c.Postgres.DSN, "database.postgres-dsn", "", "Central database PostgreSQL DSN, e.g. postgres://user:pass@host:5432/goscape?sslmode=disable. Required when database.backend=postgres.")
	f.IntVar(&c.Postgres.MaxOpenConns, "database.postgres-max-open-conns", 8, "Max open connections per service pool (postgres backend only; sqlite is always 1).")
}

// Validate enforces backend invariants. Errors self-prefix "database: "
// (matching the login/friends module convention consumed by
// cmd/goscape/app Config.Validate).
func (c *Config) Validate() error {
	switch c.Backend {
	case BackendSQLite:
		if c.SQLite.DSN == "" {
			return fmt.Errorf("database: sqlite.dsn must be non-empty when database.backend=sqlite")
		}
	case BackendPostgres:
		if c.Postgres.DSN == "" {
			return fmt.Errorf("database: postgres.dsn must be non-empty when database.backend=postgres")
		}
		if c.Postgres.MaxOpenConns < 1 {
			return fmt.Errorf("database: postgres.max_open_conns must be >= 1, got %d", c.Postgres.MaxOpenConns)
		}
	default:
		return fmt.Errorf("database: backend must be one of [sqlite, postgres], got %q", c.Backend)
	}
	return nil
}
