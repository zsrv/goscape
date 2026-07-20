package account

import (
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/zsrv/goscape/pkg/dskit/services"
	"github.com/zsrv/goscape/pkg/gamedb"
)

func freePort(t *testing.T) int {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer lis.Close()
	return lis.Addr().(*net.TCPAddr).Port
}

func TestAccountModule_StartServeStop(t *testing.T) {
	dir := t.TempDir()
	var dbCfg gamedb.Config
	dbCfg.Backend = gamedb.BackendSQLite
	dbCfg.SQLite.DSN = filepath.Join(dir, "test.db")

	// The database module migrates before dependents start; mirror that.
	db, err := gamedb.Open(dbCfg, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Migrate(t.Context()); err != nil {
		t.Fatal(err)
	}
	db.Close()

	cfg := defaultConfig(t)
	cfg.Enable = true
	cfg.HTTPListenPort = freePort(t)
	cfg.GRPCListenPort = freePort(t)
	cfg.PublicURL = fmt.Sprintf("http://127.0.0.1:%d", cfg.HTTPListenPort)

	a, err := New(cfg, dbCfg, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	// StartAndAwaitRunning/StopAndAwaitTerminated are the dskit service
	// helpers; check pkg/dskit/services for the exact names in this port
	// before writing (grep 'func StartAndAwaitRunning').
	if err := services.StartAndAwaitRunning(t.Context(), a); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer func() {
		if err := services.StopAndAwaitTerminated(t.Context(), a); err != nil {
			t.Fatalf("stop: %v", err)
		}
	}()

	// Portal answers.
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/healthz", cfg.HTTPListenPort))
	if err != nil {
		t.Fatalf("healthz: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("healthz status = %d", resp.StatusCode)
	}
}
