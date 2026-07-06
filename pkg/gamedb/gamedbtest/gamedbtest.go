// Package gamedbtest provides shared Postgres schema-isolation test
// support for goscape's central-database test suites. It is deliberately
// a separate package from pkg/gamedb (rather than a non-_test.go file
// inside gamedb itself): a _test.go file cannot be imported by other
// packages' tests, but a plain .go file that imports "testing" becomes
// part of every consumer's build graph — including the production
// goscape binary, since modules/login and modules/friends import
// pkg/gamedb. Carving the testing.T-shaped helper into its own package
// keeps "testing" out of that production import graph entirely; only
// test binaries (pkg/gamedb, modules/login, modules/friends _test.go
// files) import gamedbtest.
//
// No production code may import this package.
package gamedbtest

import (
	"context"
	"crypto/sha256"
	"flag"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/zsrv/goscape/pkg/gamedb"
)

// OpenTestSchema opens the given Postgres DSN with a unique throwaway
// schema derived from name (search_path isolation), creates the schema,
// registers cleanup (DROP SCHEMA ... CASCADE, then pool Close) on t, and
// applies the migration lineage. Test support for goscape's
// Postgres-backed suites (pkg/gamedb, modules/login, modules/friends);
// no production caller. Callers are responsible for deciding whether to
// invoke this at all (e.g. skip vs. fall back to sqlite) based on
// whether a DSN is configured — this helper always dials postgres.
func OpenTestSchema(t *testing.T, dsn, name string, logger *slog.Logger) *gamedb.DB {
	t.Helper()
	schema := fmt.Sprintf("t_%x", sha256.Sum256([]byte(name)))[:32]

	var cfg gamedb.Config
	fs := flag.NewFlagSet("", flag.PanicOnError)
	cfg.RegisterFlagsAndApplyDefaults(fs)
	cfg.Backend = gamedb.BackendPostgres
	sep := "?"
	if strings.Contains(dsn, "?") {
		sep = "&"
	}
	cfg.Postgres.DSN = dsn + sep + "search_path=" + schema

	db, err := gamedb.Open(cfg, logger)
	if err != nil {
		t.Fatalf("gamedbtest.OpenTestSchema: open: %v", err)
	}
	if _, err := db.ExecContext(t.Context(), `CREATE SCHEMA IF NOT EXISTS `+schema); err != nil {
		t.Fatalf("gamedbtest.OpenTestSchema: create schema: %v", err)
	}
	t.Cleanup(func() {
		// Two traps meet here. (1) t.Context() is canceled BEFORE
		// cleanup functions run, so the drop needs its own context or
		// it silently no-ops and the schema leaks across runs
		// (identically-named tests on sibling branches then collide on
		// seed rows). (2) The drop must not ride the test's own pool:
		// tests that force insert errors by closing the pool would make
		// the drop fail and leak anyway. So: close the test pool, then
		// drop through a dedicated short-lived connection, and surface
		// failures — a leaked schema is a real defect, not noise.
		db.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		dropper, err := gamedb.Open(cfg, logger)
		if err != nil {
			t.Errorf("gamedbtest.OpenTestSchema: open dropper: %v", err)
			return
		}
		defer dropper.Close()
		if _, err := dropper.ExecContext(ctx, `DROP SCHEMA IF EXISTS `+schema+` CASCADE`); err != nil {
			t.Errorf("gamedbtest.OpenTestSchema: drop schema %s: %v", schema, err)
		}
	})
	if err := db.Migrate(t.Context()); err != nil {
		t.Fatalf("gamedbtest.OpenTestSchema: migrate: %v", err)
	}
	return db
}
