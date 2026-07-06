package gamedb

import (
	"path/filepath"
	"testing"

	"github.com/zsrv/goscape/pkg/dskit/services"
)

func TestMigratorService_MigratesThenIdles(t *testing.T) {
	cfg := defaultConfig()
	cfg.SQLite.DSN = filepath.Join(t.TempDir(), "goscape.db")

	svc := NewMigratorService(cfg, noopLogger())
	if err := services.StartAndAwaitRunning(t.Context(), svc); err != nil {
		t.Fatalf("StartAndAwaitRunning: %v", err)
	}

	// Schema must exist for an independent client by the time the
	// service reports Running.
	db, err := Open(cfg, noopLogger())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='friendlist'`).Scan(&n); err != nil {
		t.Fatalf("probe: %v", err)
	}
	if n != 1 {
		t.Error("friendlist table missing after migrator Running")
	}

	svc.StopAsync()
	if err := svc.AwaitTerminated(t.Context()); err != nil {
		t.Fatalf("AwaitTerminated: %v", err)
	}
}

func TestMigratorService_FailsOnUnknownBackend(t *testing.T) {
	cfg := defaultConfig()
	cfg.Backend = "bogus"
	svc := NewMigratorService(cfg, noopLogger())
	if err := services.StartAndAwaitRunning(t.Context(), svc); err == nil {
		t.Fatal("StartAndAwaitRunning: got nil error, want failure (unknown backend)")
	}
}
