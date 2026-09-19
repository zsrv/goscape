package admin

import (
	"context"
	"encoding/json"
	"flag"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type readyzBody struct {
	Status  string            `json:"status"`
	Reason  string            `json:"reason"`
	Modules map[string]string `json:"modules"`
}

func readyFunc(ready bool, reason string, mods map[string]string) ReadyFunc {
	return func(context.Context) (bool, string, map[string]string) { return ready, reason, mods }
}

func TestConfigFlagDefaultIsEmpty(t *testing.T) {
	var c Config
	fs := flag.NewFlagSet("", flag.PanicOnError)
	c.RegisterFlagsAndApplyDefaults(fs)
	if c.Listen != "" {
		t.Fatalf("default admin.listen = %q, want empty (disabled)", c.Listen)
	}
	f := fs.Lookup("admin.listen")
	if f == nil {
		t.Fatal("flag admin.listen is not registered")
	}
	if f.DefValue != "" {
		t.Fatalf("flag default = %q, want empty", f.DefValue)
	}
	if err := fs.Parse([]string{"--admin.listen=127.0.0.1:1"}); err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if c.Listen != "127.0.0.1:1" {
		t.Fatalf("after parse, Listen = %q", c.Listen)
	}
}

func TestHealthzIsAlwaysOK(t *testing.T) {
	// /healthz is liveness: it deliberately checks nothing, so it answers
	// 200 even while the process reports itself not ready.
	a := NewServer("", readyFunc(false, "modules not running", nil))
	srv := httptest.NewServer(a.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/healthz")
	if err != nil {
		t.Fatalf("GET /healthz: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("/healthz status = %d, want 200", resp.StatusCode)
	}
	if got := resp.Header.Get("Content-Type"); got != "text/plain; charset=utf-8" {
		t.Errorf("/healthz Content-Type = %q", got)
	}
	buf := make([]byte, 16)
	n, _ := resp.Body.Read(buf)
	if string(buf[:n]) != "ok\n" {
		t.Errorf("/healthz body = %q, want %q", string(buf[:n]), "ok\n")
	}
}

func TestReadyzReports503WithReasonAndModuleStates(t *testing.T) {
	states := map[string]string{"login": "Running", "world": "Starting"}
	a := NewServer("", readyFunc(false, "world: no first tick", states))
	srv := httptest.NewServer(a.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/readyz")
	if err != nil {
		t.Fatalf("GET /readyz: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("/readyz status = %d, want 503", resp.StatusCode)
	}
	var body readyzBody
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Status != "not ready" {
		t.Errorf("status = %q, want %q", body.Status, "not ready")
	}
	if body.Reason != "world: no first tick" {
		t.Errorf("reason = %q, want the verdict to round-trip", body.Reason)
	}
	if body.Modules["world"] != "Starting" {
		t.Errorf("modules = %v, want world=Starting", body.Modules)
	}
}

func TestReadyz200WhenReady(t *testing.T) {
	states := map[string]string{"login": "Running"}
	a := NewServer("", readyFunc(true, "", states))
	srv := httptest.NewServer(a.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/readyz")
	if err != nil {
		t.Fatalf("GET /readyz: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("/readyz status = %d, want 200", resp.StatusCode)
	}
	var raw map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if raw["status"] != "ready" {
		t.Errorf("status = %v, want %q", raw["status"], "ready")
	}
	if _, ok := raw["reason"]; ok {
		t.Errorf("reason must be omitted when empty, got %v", raw["reason"])
	}
}

// TestReadyzModulesIsNeverNull pins that the modules object is always a JSON
// object: probes and dashboards parsing it must not have to handle null.
func TestReadyzModulesIsNeverNull(t *testing.T) {
	for name, a := range map[string]*Server{
		"nil map":       NewServer("", readyFunc(true, "", nil)),
		"nil ReadyFunc": NewServer("", nil),
	} {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(a.Handler())
			defer srv.Close()
			resp, err := http.Get(srv.URL + "/readyz")
			if err != nil {
				t.Fatalf("GET /readyz: %v", err)
			}
			defer resp.Body.Close()
			var raw map[string]json.RawMessage
			if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if string(raw["modules"]) != "{}" {
				t.Fatalf("modules = %s, want {}", raw["modules"])
			}
		})
	}
}

// TestReadyzNilReadyFuncIsNotReady: a server built without a verdict func
// must fail closed, not report itself ready.
func TestReadyzNilReadyFuncIsNotReady(t *testing.T) {
	a := NewServer("", nil)
	srv := httptest.NewServer(a.Handler())
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/readyz")
	if err != nil {
		t.Fatalf("GET /readyz: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", resp.StatusCode)
	}
}

// TestReadyzUsesRequestContext confirms the verdict func gets the request's
// context, so a client that goes away can cancel the check.
func TestReadyzUsesRequestContext(t *testing.T) {
	type ctxKey struct{}
	got := make(chan bool, 1)
	a := NewServer("", func(ctx context.Context) (bool, string, map[string]string) {
		got <- ctx.Value(ctxKey{}) == "yes"
		return true, "", nil
	})
	req := httptest.NewRequest("GET", "/readyz", nil)
	req = req.WithContext(context.WithValue(req.Context(), ctxKey{}, "yes"))
	a.Handler().ServeHTTP(httptest.NewRecorder(), req)
	if !<-got {
		t.Fatal("ReadyFunc was not called with the request context")
	}
}

func TestStartBindsAndShutsDown(t *testing.T) {
	a := NewServer("127.0.0.1:0", readyFunc(true, "", nil))
	if err := a.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if a.Addr() == "" {
		t.Fatal("Addr must report the bound address")
	}
	resp, err := http.Get("http://" + a.Addr() + "/healthz")
	if err != nil {
		t.Fatalf("GET /healthz on the bound listener: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("/healthz status = %d, want 200", resp.StatusCode)
	}

	addr := a.Addr()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	if err := a.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	if _, err := http.Get("http://" + addr + "/healthz"); err == nil {
		t.Fatal("the listener is still accepting after Shutdown")
	}
}

// TestStartOnOccupiedPortFails pins the reason Start binds synchronously: a
// port clash must fail the boot rather than disappear into a goroutine.
func TestStartOnOccupiedPortFails(t *testing.T) {
	held, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer held.Close()

	a := NewServer(held.Addr().String(), readyFunc(true, "", nil))
	if err := a.Start(); err == nil {
		_ = a.Shutdown(t.Context())
		t.Fatal("Start on an occupied port returned nil, want an error")
	}
	if a.Addr() != "" {
		t.Errorf("Addr after a failed Start = %q, want empty", a.Addr())
	}
}

func TestEmptyAddrIsDisabled(t *testing.T) {
	a := NewServer("", nil)
	if err := a.Start(); err != nil {
		t.Fatalf("Start on a disabled server: %v", err)
	}
	if a.Addr() != "" {
		t.Fatalf("Addr = %q, want empty", a.Addr())
	}
	if err := a.Shutdown(t.Context()); err != nil {
		t.Fatalf("Shutdown on a disabled server: %v", err)
	}
}

// TestUnknownRouteIs404 keeps the surface to exactly the two documented
// endpoints — this listener carries module names, so it must not grow
// incidental routes.
func TestUnknownRouteIs404(t *testing.T) {
	a := NewServer("", readyFunc(true, "", nil))
	for _, target := range []string{"/", "/metrics", "/debug/pprof/"} {
		rr := httptest.NewRecorder()
		a.Handler().ServeHTTP(rr, httptest.NewRequest("GET", target, nil))
		if rr.Code != http.StatusNotFound {
			t.Errorf("GET %s = %d, want 404", target, rr.Code)
		}
	}
	for _, method := range []string{"POST", "PUT", "DELETE"} {
		rr := httptest.NewRecorder()
		a.Handler().ServeHTTP(rr, httptest.NewRequest(method, "/readyz", nil))
		if rr.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s /readyz = %d, want 405", method, rr.Code)
		}
	}
}
