package app

import (
	"context"
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health/grpc_health_v1"

	"github.com/zsrv/goscape/pkg/dskit/modules"
	"github.com/zsrv/goscape/pkg/dskit/services"
)

// newHealthApp builds an App from the default config with mutate applied, and
// injects the fake signal handler so Run terminates on the test's command.
func newHealthApp(t *testing.T, mutate func(*Config)) (*App, *fakeSignalHandler) {
	t.Helper()
	cfg := *NewDefaultConfig()
	if mutate != nil {
		mutate(&cfg)
	}
	a, err := New(discardLogger(), cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	fh := newFakeSignalHandler()
	a.newSignalHandler = func(*slog.Logger) signalHandler { return fh }
	return a, fh
}

// withIdleModule swaps in a scoped ModuleManager carrying one module that
// starts, runs and stops cleanly — the cheapest runnable target (see
// TestApp_Run_GracefulStop for why the disabled production modules cannot
// stand in for one).
func withIdleModule(t *testing.T, a *App, name string) {
	t.Helper()
	mm := modules.NewManager(discardLogger())
	mm.RegisterModule(name, func() (services.Service, error) {
		return services.NewIdleService(nil, nil), nil
	})
	if err := mm.AddDependency(name); err != nil {
		t.Fatalf("AddDependency: %v", err)
	}
	a.ModuleManager = mm
}

type readyzBody struct {
	Status  string            `json:"status"`
	Reason  string            `json:"reason"`
	Modules map[string]string `json:"modules"`
}

// getReadyz fetches /readyz from the app's admin listener.
func getReadyz(t *testing.T, addr string) (int, readyzBody) {
	t.Helper()
	resp, err := http.Get("http://" + addr + "/readyz")
	if err != nil {
		t.Fatalf("GET /readyz: %v", err)
	}
	defer resp.Body.Close()
	var body readyzBody
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode /readyz: %v", err)
	}
	return resp.StatusCode, body
}

// awaitAdminAddr polls until Run has bound the admin listener.
func awaitAdminAddr(t *testing.T, a *App) string {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if addr := a.adminAddress(); addr != "" {
			return addr
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("admin listener did not bind within 5s")
	return ""
}

// TestDefaultsAreInert pins the promise that nothing new binds or changes
// behaviour with a stock config.
func TestDefaultsAreInert(t *testing.T) {
	cfg := NewDefaultConfig()
	if cfg.Admin.Listen != "" {
		t.Errorf("default admin.listen = %q, want empty", cfg.Admin.Listen)
	}
	if cfg.ShutdownDelay != 0 {
		t.Errorf("default shutdown_delay = %v, want 0", cfg.ShutdownDelay)
	}
}

// TestReadyBeforeRun: an App that has not run yet is not ready, and says so
// with an empty (but non-nil) module map.
func TestReadyBeforeRun(t *testing.T) {
	a, _ := newHealthApp(t, nil)
	ready, reason, mods := a.Ready(t.Context())
	if ready {
		t.Error("a fresh App reports ready")
	}
	if reason != "modules not running" {
		t.Errorf("reason = %q, want %q", reason, "modules not running")
	}
	if mods == nil {
		t.Fatal("modules map is nil; the /readyz body must never carry null")
	}
	if len(mods) != 0 {
		t.Errorf("modules = %v, want empty", mods)
	}
}

// TestReadyzLifecycle drives the admin listener through a whole Run.
func TestReadyzLifecycle(t *testing.T) {
	a, _ := newHealthApp(t, func(c *Config) {
		c.Target = "idle"
		c.Admin.Listen = "127.0.0.1:0"
	})
	withIdleModule(t, a, "idle")

	runDone := make(chan error, 1)
	go func() { runDone <- a.Run() }()

	addr := awaitAdminAddr(t, a)

	// /healthz is liveness: up immediately, and it stays 200 regardless.
	resp, err := http.Get("http://" + addr + "/healthz")
	if err != nil {
		t.Fatalf("GET /healthz: %v", err)
	}
	buf := make([]byte, 8)
	n, _ := resp.Body.Read(buf)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || string(buf[:n]) != "ok\n" {
		t.Fatalf("/healthz = %d %q, want 200 %q", resp.StatusCode, string(buf[:n]), "ok\n")
	}

	// /readyz goes 200 once every module is Running.
	var body readyzBody
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var code int
		code, body = getReadyz(t, addr)
		if code == http.StatusOK {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if body.Status != "ready" {
		t.Fatalf("/readyz never became ready: %+v", body)
	}
	if len(body.Modules) == 0 {
		t.Fatal("/readyz reported no modules")
	}
	for name, state := range body.Modules {
		if state != "Running" {
			t.Errorf("module %s = %q, want Running", name, state)
		}
	}

	a.Stop()
	select {
	case err := <-runDone:
		if err != nil {
			t.Errorf("Run returned %v, want nil", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return within 5s")
	}

	// Run's deferred Shutdown must have closed the admin port.
	if _, err := http.Get("http://" + addr + "/healthz"); err == nil {
		t.Error("the admin listener is still accepting after Run returned")
	}
}

// TestShutdownDelayReportsNotReady pins the point of --shutdown-delay: the
// window between SIGTERM and the actual stop, during which probes must see
// not-ready so the pod leaves Service endpoints before it stops serving.
func TestShutdownDelayReportsNotReady(t *testing.T) {
	a, _ := newHealthApp(t, func(c *Config) {
		c.Target = "idle"
		c.Admin.Listen = "127.0.0.1:0"
		c.ShutdownDelay = 300 * time.Millisecond
	})
	withIdleModule(t, a, "idle")

	runDone := make(chan error, 1)
	go func() { runDone <- a.Run() }()

	addr := awaitAdminAddr(t, a)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if code, _ := getReadyz(t, addr); code == http.StatusOK {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	a.Stop()

	// DURING the delay the listener is still up and answers 503.
	var (
		sawNotReady bool
		gotReason   string
	)
	notReadyDeadline := time.Now().Add(250 * time.Millisecond)
	for time.Now().Before(notReadyDeadline) {
		code, body := getReadyz(t, addr)
		if code == http.StatusServiceUnavailable {
			sawNotReady = true
			gotReason = body.Reason
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if !sawNotReady {
		t.Fatal("/readyz never reported 503 during the shutdown delay")
	}
	if gotReason != "shutting down" {
		t.Errorf("reason = %q, want %q", gotReason, "shutting down")
	}

	select {
	case err := <-runDone:
		if err != nil {
			t.Errorf("Run returned %v, want nil", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after the shutdown delay")
	}
}

// TestAdminBindFailureFailsRun: the listener binds synchronously, before any
// module is initialised, so a port clash is a boot error and leaves nothing
// running.
func TestAdminBindFailureFailsRun(t *testing.T) {
	held, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer held.Close()

	a, _ := newHealthApp(t, func(c *Config) {
		c.Target = "idle"
		c.Admin.Listen = held.Addr().String()
	})
	withIdleModule(t, a, "idle")

	err = a.Run()
	if err == nil {
		t.Fatal("Run returned nil with the admin port occupied")
	}
	if !strings.Contains(err.Error(), "admin listener") {
		t.Errorf("Run err = %v, want it to name the admin listener", err)
	}
	if len(a.serviceMap) != 0 {
		t.Errorf("modules were initialised despite the bind failure: %v", a.serviceMap)
	}
}

// TestGRPCHealthOnLogin boots the login module against a throwaway sqlite DB
// on an injected loopback listener and asks its gRPC server for the health of
// the whole process — the surface a service mesh or grpc_health_probe uses on
// a host that serves no HTTP at all.
func TestGRPCHealthOnLogin(t *testing.T) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := lis.Addr().String()

	dir := t.TempDir()
	a, _ := newHealthApp(t, func(c *Config) {
		c.Target = Login
		c.Login.Enable = true
		c.Login.Listener = lis
		c.Login.SavePath = filepath.Join(dir, "players")
		c.Login.BCryptCost = 4
		c.Database.SQLite.DSN = filepath.Join(dir, "goscape.db")
		c.ShutdownDelay = 2 * time.Second
	})

	runDone := make(chan error, 1)
	go func() { runDone <- a.Run() }()
	t.Cleanup(func() {
		select {
		case <-runDone:
		case <-time.After(10 * time.Second):
			t.Error("Run did not return")
		}
	})

	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer conn.Close()
	client := grpc_health_v1.NewHealthClient(conn)

	check := func() grpc_health_v1.HealthCheckResponse_ServingStatus {
		ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
		defer cancel()
		resp, err := client.Check(ctx, &grpc_health_v1.HealthCheckRequest{})
		if err != nil {
			return grpc_health_v1.HealthCheckResponse_UNKNOWN
		}
		return resp.Status
	}

	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if check() == grpc_health_v1.HealthCheckResponse_SERVING {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if got := check(); got != grpc_health_v1.HealthCheckResponse_SERVING {
		t.Fatalf("health before shutdown = %v, want SERVING", got)
	}

	// The shutdown delay keeps the server up while it reports NOT_SERVING.
	a.Stop()
	deadline = time.Now().Add(2 * time.Second)
	var got grpc_health_v1.HealthCheckResponse_ServingStatus
	for time.Now().Before(deadline) {
		got = check()
		if got == grpc_health_v1.HealthCheckResponse_NOT_SERVING {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if got != grpc_health_v1.HealthCheckResponse_NOT_SERVING {
		t.Fatalf("health during shutdown = %v, want NOT_SERVING", got)
	}
}
