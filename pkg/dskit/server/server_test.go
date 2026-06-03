package server

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zsrv/goscape/pkg/dskit/middleware"
	"github.com/zsrv/goscape/pkg/dskit/services"
)

// discardLogger returns a logger that discards output.
func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// newTestConfig returns a Config that listens on a kernel-assigned port
// (`:0`), uses an ignoreSignalHandler so Run() does not register OS signal
// handlers, and discards logs.
func newTestConfig() Config {
	cfg := Config{
		HTTPListenAddress:             "127.0.0.1",
		HTTPListenPort:                0,
		HTTPListenNetwork:             "",
		ServerGracefulShutdownTimeout: 5 * time.Second,
		Log:                           discardLogger(),
	}
	DisableSignalHandling(&cfg)
	return cfg
}

// TestServerNew_Defaults confirms New constructs a Server with a default
// network ("tcp") when HTTPListenNetwork is empty, plus a default mux when
// Router is unset.
// COV-1 (Arc 18).
func TestServerNew_Defaults(t *testing.T) {
	cfg := newTestConfig()
	s, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer s.Shutdown()
	defer s.httpListener.Close()

	if s.HTTP == nil {
		t.Error("HTTP mux is nil")
	}
	if s.HTTPServer == nil {
		t.Error("HTTPServer is nil")
	}
	if s.Log == nil {
		t.Error("Log is nil")
	}
	// The kernel should have assigned a non-zero port.
	tcpAddr, ok := s.httpListener.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatalf("listener addr type = %T, want *net.TCPAddr", s.httpListener.Addr())
	}
	if tcpAddr.Port == 0 {
		t.Errorf("listener port = 0, want kernel-assigned non-zero")
	}
}

// TestServerNew_InvalidPort confirms a port outside [0, 65535] surfaces a
// net.Listen error.
// COV-1 (Arc 18).
func TestServerNew_InvalidPort(t *testing.T) {
	cfg := newTestConfig()
	cfg.HTTPListenPort = -1
	if _, err := New(cfg); err == nil {
		t.Fatal("New() = nil, want net.Listen error for negative port")
	}
}

// TestServerNew_InvalidLogSourceIPsRegex confirms BuildHTTPMiddleware errors
// propagate from New (wrapping "error building http middleware").
// COV-1 (Arc 18).
func TestServerNew_InvalidLogSourceIPsRegex(t *testing.T) {
	cfg := newTestConfig()
	cfg.LogSourceIPsHeader = "X-Forwarded-For"
	cfg.LogSourceIPsRegex = "[invalid"
	_, err := New(cfg)
	if err == nil {
		t.Fatal("New() = nil, want middleware build error")
	}
	if !strings.Contains(err.Error(), "http middleware") {
		t.Errorf("err = %v, want middleware error", err)
	}
}

// TestServerNew_CustomRouter confirms New honors a caller-supplied
// http.ServeMux instead of allocating a fresh one.
// COV-1 (Arc 18).
func TestServerNew_CustomRouter(t *testing.T) {
	cfg := newTestConfig()
	mux := http.NewServeMux()
	cfg.Router = mux
	s, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer s.Shutdown()
	defer s.httpListener.Close()
	if s.HTTP != mux {
		t.Errorf("s.HTTP = %p, want %p (custom router)", s.HTTP, mux)
	}
}

// TestServerRun_ServesHandler starts the server, sends a request through a
// real net.Dial-backed http.Client, and confirms the registered handler
// returns the expected body.
// COV-1 (Arc 18).
func TestServerRun_ServesHandler(t *testing.T) {
	cfg := newTestConfig()
	s, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	s.HTTP.HandleFunc("GET /hello", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("hello world"))
	})

	runDone := make(chan error, 1)
	go func() { runDone <- s.Run() }()

	// Issue the request.
	addr := s.httpListener.Addr().String()
	resp, err := http.Get("http://" + addr + "/hello")
	if err != nil {
		s.Stop()
		<-runDone
		t.Fatalf("GET /hello: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if got := string(body); got != "hello world" {
		t.Errorf("body = %q, want %q", got, "hello world")
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}

	s.Stop()
	select {
	case err := <-runDone:
		if err != nil {
			t.Errorf("Run returned %v, want nil", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return within 5s after Stop")
	}
	s.Shutdown()
}

// TestServerRun_Default404 confirms an unregistered route returns 404.
// COV-1 (Arc 18).
func TestServerRun_Default404(t *testing.T) {
	cfg := newTestConfig()
	s, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	runDone := make(chan error, 1)
	go func() { runDone <- s.Run() }()

	resp, err := http.Get("http://" + s.httpListener.Addr().String() + "/nonexistent")
	if err != nil {
		s.Stop()
		<-runDone
		t.Fatalf("GET /nonexistent: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}

	s.Stop()
	<-runDone
	s.Shutdown()
}

// TestServerShutdown_Idempotent confirms calling Shutdown multiple times
// does not panic. Production code defers Shutdown after New, and the
// service.go stoppingFn also calls Shutdown — both paths run on graceful
// termination, so idempotency is load-bearing.
// COV-1 (Arc 18).
func TestServerShutdown_Idempotent(t *testing.T) {
	cfg := newTestConfig()
	s, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	// First Shutdown — closes the underlying HTTP server.
	s.Shutdown()
	// Second Shutdown — must not panic even though the server is closed.
	s.Shutdown()
}

// TestServerStop_UnblocksRun confirms Stop unblocks Run by closing the
// configured SignalHandler. Without Stop, Run would block forever on the
// signal-handler goroutine.
// COV-1 (Arc 18).
func TestServerStop_UnblocksRun(t *testing.T) {
	cfg := newTestConfig()
	s, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	runDone := make(chan error, 1)
	go func() { runDone <- s.Run() }()

	// Give Run a beat to set up its goroutines.
	time.Sleep(10 * time.Millisecond)
	s.Stop()

	select {
	case err := <-runDone:
		if err != nil {
			t.Errorf("Run returned %v, want nil", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return within 5s after Stop")
	}
	s.Shutdown()
}

// TestServerRun_InFlightRequestCompletesOnShutdown confirms that a request
// already in flight when Shutdown is called runs to completion. This is
// the http.Server.Shutdown contract; pinning it here keeps the dskit
// wrapper honest.
// COV-1 (Arc 18).
func TestServerRun_InFlightRequestCompletesOnShutdown(t *testing.T) {
	cfg := newTestConfig()
	s, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	started := make(chan struct{})
	finish := make(chan struct{})
	var completed atomic.Bool

	s.HTTP.HandleFunc("GET /slow", func(w http.ResponseWriter, _ *http.Request) {
		close(started)
		<-finish
		_, _ = w.Write([]byte("done"))
		completed.Store(true)
	})

	runDone := make(chan error, 1)
	go func() { runDone <- s.Run() }()

	// Fire the slow request from one goroutine.
	addr := s.httpListener.Addr().String()
	var wg sync.WaitGroup
	wg.Add(1)
	var body string
	var statusCode int
	go func() {
		defer wg.Done()
		resp, err := http.Get("http://" + addr + "/slow")
		if err != nil {
			return
		}
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		body = string(b)
		statusCode = resp.StatusCode
	}()

	// Wait for the handler to start, then trigger Shutdown.
	<-started
	go func() {
		// Shutdown should wait for the in-flight handler to finish.
		s.Shutdown()
	}()

	// Let Shutdown register, then release the handler.
	time.Sleep(20 * time.Millisecond)
	close(finish)

	wg.Wait()
	if !completed.Load() {
		t.Error("in-flight handler did not run to completion before Shutdown returned")
	}
	if statusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", statusCode)
	}
	if body != "done" {
		t.Errorf("body = %q, want %q", body, "done")
	}

	s.Stop()
	<-runDone
}

// TestBuildHTTPMiddleware_DefaultsIncludeRouteInjectorTracerAndLog confirms
// the default middleware stack contains exactly the expected three
// middlewares in order.
// COV-1 (Arc 18).
func TestBuildHTTPMiddleware_DefaultsIncludeRouteInjectorTracerAndLog(t *testing.T) {
	cfg := Config{
		LogRequestExcludeHeadersList: "Authorization",
	}
	mws, err := BuildHTTPMiddleware(cfg, discardLogger())
	if err != nil {
		t.Fatalf("BuildHTTPMiddleware: %v", err)
	}
	if len(mws) != 3 {
		t.Fatalf("len(mws) = %d, want 3", len(mws))
	}
	if _, ok := mws[0].(middleware.RouteInjector); !ok {
		t.Errorf("mws[0] type = %T, want middleware.RouteInjector", mws[0])
	}
	if _, ok := mws[1].(middleware.Tracer); !ok {
		t.Errorf("mws[1] type = %T, want middleware.Tracer", mws[1])
	}
	if _, ok := mws[2].(middleware.Log); !ok {
		t.Errorf("mws[2] type = %T, want middleware.Log", mws[2])
	}
}

// TestBuildHTTPMiddleware_DoNotAddDefault confirms that when
// DoNotAddDefaultHTTPMiddleware is true, only the caller-supplied
// middlewares appear and the defaults are skipped.
// COV-1 (Arc 18).
func TestBuildHTTPMiddleware_DoNotAddDefault(t *testing.T) {
	custom := middleware.Func(func(h http.Handler) http.Handler { return h })
	cfg := Config{
		DoNotAddDefaultHTTPMiddleware: true,
		HTTPMiddleware:                []middleware.Interface{custom},
	}
	mws, err := BuildHTTPMiddleware(cfg, discardLogger())
	if err != nil {
		t.Fatalf("BuildHTTPMiddleware: %v", err)
	}
	if len(mws) != 1 {
		t.Fatalf("len(mws) = %d, want 1", len(mws))
	}
}

// TestBuildHTTPMiddleware_AppendsExtras confirms that under the default
// path, caller-supplied middlewares are appended after the defaults.
// COV-1 (Arc 18).
func TestBuildHTTPMiddleware_AppendsExtras(t *testing.T) {
	custom := middleware.Func(func(h http.Handler) http.Handler { return h })
	cfg := Config{
		HTTPMiddleware: []middleware.Interface{custom},
	}
	mws, err := BuildHTTPMiddleware(cfg, discardLogger())
	if err != nil {
		t.Fatalf("BuildHTTPMiddleware: %v", err)
	}
	if len(mws) != 4 {
		t.Fatalf("len(mws) = %d, want 4 (3 defaults + 1 extra)", len(mws))
	}
}

// TestBuildHTTPMiddleware_LogSourceIPsRegexError confirms an invalid regex
// surfaces from BuildHTTPMiddleware via NewSourceIPs.
// COV-1 (Arc 18).
func TestBuildHTTPMiddleware_LogSourceIPsRegexError(t *testing.T) {
	cfg := Config{
		LogSourceIPsHeader: "X-Forwarded-For",
		LogSourceIPsRegex:  "((((",
	}
	_, err := BuildHTTPMiddleware(cfg, discardLogger())
	if err == nil {
		t.Fatal("BuildHTTPMiddleware() = nil, want regex error")
	}
}

// TestMiddlewareCompositionOrder builds the default stack and feeds it
// through a recording wrapper to verify Merge applies middlewares in
// declaration order (outermost first).
// COV-1 (Arc 18).
func TestMiddlewareCompositionOrder(t *testing.T) {
	var order []string
	mw := func(name string) middleware.Interface {
		return middleware.Func(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				order = append(order, name)
				next.ServeHTTP(w, r)
			})
		})
	}

	stack := middleware.Merge(mw("a"), mw("b"), mw("c"))
	final := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		order = append(order, "handler")
		w.WriteHeader(http.StatusOK)
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	stack.Wrap(final).ServeHTTP(rec, req)

	want := []string{"a", "b", "c", "handler"}
	if fmt.Sprintf("%v", order) != fmt.Sprintf("%v", want) {
		t.Errorf("order = %v, want %v", order, want)
	}
}

// TestServerNew_TCPV4Network confirms HTTPListenNetwork = "tcp4" is
// accepted (Config exposes both DefaultNetwork="tcp" and NetworkTCPV4).
// COV-1 (Arc 18).
func TestServerNew_TCPV4Network(t *testing.T) {
	cfg := newTestConfig()
	cfg.HTTPListenNetwork = NetworkTCPV4
	s, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer s.Shutdown()
	defer s.httpListener.Close()
	if s.httpListener == nil {
		t.Error("listener is nil")
	}
}

// TestDisableSignalHandling_StopUnblocksLoop confirms the dummy signal
// handler installed by DisableSignalHandling blocks Loop until Stop closes
// the channel.
// COV-1 (Arc 18).
func TestDisableSignalHandling_StopUnblocksLoop(t *testing.T) {
	cfg := Config{}
	DisableSignalHandling(&cfg)
	if cfg.SignalHandler == nil {
		t.Fatal("SignalHandler not set by DisableSignalHandling")
	}
	done := make(chan struct{})
	go func() {
		cfg.SignalHandler.Loop()
		close(done)
	}()
	// Loop should block.
	select {
	case <-done:
		t.Fatal("Loop returned before Stop")
	case <-time.After(20 * time.Millisecond):
	}
	cfg.SignalHandler.Stop()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Loop did not return after Stop")
	}
}

// TestServerServiceLifecycle drives NewServerService through start →
// AwaitRunning → StopAsync → AwaitTerminated, confirming the dskit wrapper
// stops the server cleanly on Stop.
// COV-1 (Arc 18) — boosts service.go coverage alongside server.go.
func TestServerServiceLifecycle(t *testing.T) {
	cfg := newTestConfig()
	s, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	svc := NewServerService(s, func() []services.Service { return nil })
	if err := svc.StartAsync(context.Background()); err != nil {
		t.Fatalf("StartAsync: %v", err)
	}
	if err := svc.AwaitRunning(context.Background()); err != nil {
		t.Fatalf("AwaitRunning: %v", err)
	}
	svc.StopAsync()
	if err := svc.AwaitTerminated(context.Background()); err != nil {
		t.Errorf("AwaitTerminated: %v", err)
	}
}
