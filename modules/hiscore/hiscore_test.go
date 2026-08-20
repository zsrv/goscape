package hiscore

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/zsrv/goscape/pkg/dskit/server"
	"github.com/zsrv/goscape/pkg/dskit/services"
)

// testServer builds a dskit server bound to an ephemeral port. Port 0
// avoids collisions when tests run in parallel; the tests drive
// serv.HTTP directly rather than dialing, because Server exposes no
// accessor for the bound listener address.
func testServer(t *testing.T, cfg *Config) *server.Server {
	t.Helper()
	cfg.Server.HTTPListenAddress = "127.0.0.1"
	cfg.Server.HTTPListenPort = 0
	cfg.Server.Log = noopLogger()
	server.DisableSignalHandling(&cfg.Server)

	serv, err := server.New(cfg.Server)
	if err != nil {
		t.Fatalf("server.New: %v", err)
	}
	t.Cleanup(func() { _ = serv.Close() })
	return serv
}

func TestHiscore_StartsAndRegistersRoutes(t *testing.T) {
	dbCfg := testGameDBConfig(t)

	cfg := defaultConfig(t)
	cfg.Enable = true
	serv := testServer(t, &cfg)

	h, err := New(cfg, dbCfg, noopLogger(), serv)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if err := services.StartAndAwaitRunning(t.Context(), h); err != nil {
		t.Fatalf("StartAndAwaitRunning: %v", err)
	}
	t.Cleanup(func() {
		// Own context: t.Context() is already canceled by cleanup time.
		stopCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := services.StopAndAwaitTerminated(stopCtx, h); err != nil {
			t.Errorf("StopAndAwaitTerminated: %v", err)
		}
	})

	// starting() registered the routes on the server's mux.
	rec := httptest.NewRecorder()
	serv.HTTP.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/skills", nil))
	if rec.Code != http.StatusOK {
		t.Errorf("GET /v1/skills after start: status %d, want 200; body %s", rec.Code, rec.Body.String())
	}
}

func TestHiscore_RejectsInvalidConfig(t *testing.T) {
	cfg := defaultConfig(t)
	cfg.Enable = true
	cfg.Profile = ""
	serv := testServer(t, &cfg)

	if _, err := New(cfg, testGameDBConfig(t), noopLogger(), serv); err == nil {
		t.Fatal("New: got nil error for an invalid config, want validation failure")
	}
}
