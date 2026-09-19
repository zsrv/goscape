// Package admin provides the optional supervisor HTTP listener: a tiny server
// that answers exactly two routes, meant to be wired as a container's
// Kubernetes probes.
//
//	GET /healthz — liveness. 200 as soon as the process is up. It deliberately
//	               checks nothing: a process that is merely not ready must be
//	               taken out of Service endpoints, never restart-looped.
//	GET /readyz  — readiness. 200 only when the whole process is ready, else
//	               503 with a short reason and the state of every module.
//
// Bind it to a cluster-internal or loopback address inside the pod
// (admin.listen: ":PORT") and probe it from the kubelet. Never publish it
// through a Service, Ingress or gateway: /readyz names the modules this
// process runs and the state each one is in, which is operational detail no
// player-facing port should carry.
//
// The design follows Grafana Loki's internal_server, which serves the same
// purpose for a dskit process. goscape needs its own because its modules do
// not share one HTTP server the way Loki's do — each module owns its listener,
// so a host running only --target=login has no HTTP surface at all, and
// Kubernetes could previously do no better than a tcpSocket probe. This
// listener is owned by the app root, outside the module graph, so it answers
// during module init (a slow cache load) and for targets that serve no HTTP.
//
// It is off by default: an empty Listen yields a server whose Start, Addr and
// Shutdown are all no-ops.
package admin

import (
	"context"
	"encoding/json"
	"flag"
	"net"
	"net/http"
	"time"
)

// readHeaderTimeout bounds how long a client may take to send its request
// headers, so a stuck or slow connection cannot hold a goroutine forever.
const readHeaderTimeout = 5 * time.Second

// Config configures the admin HTTP listener. An empty Listen disables it.
type Config struct {
	Listen string `yaml:"listen"`
}

// RegisterFlagsAndApplyDefaults registers --admin.listen. The default is
// empty: nothing binds unless an operator asks for it.
func (c *Config) RegisterFlagsAndApplyDefaults(f *flag.FlagSet) {
	f.StringVar(&c.Listen, "admin.listen", "",
		"Address for the admin HTTP listener serving GET /healthz (liveness) and GET /readyz (readiness). Empty disables it. Bind it inside the pod only — /readyz reports module names and states.")
}

// ReadyFunc reports process readiness, a short human reason when not ready
// ("" when ready), and the per-module state map carried in the /readyz body.
// cmd/goscape/app.App.Ready implements it.
type ReadyFunc func(ctx context.Context) (ready bool, reason string, modules map[string]string)

// Server is the admin listener. Build it with NewServer.
type Server struct {
	addr  string
	ready ReadyFunc

	ln  net.Listener
	srv *http.Server
}

// NewServer builds the server. An empty addr yields a disabled server whose
// Start, Addr and Shutdown are all no-ops.
func NewServer(addr string, ready ReadyFunc) *Server {
	return &Server{addr: addr, ready: ready}
}

// readyzResponse is the GET /readyz body. Reason is omitted when the process
// is ready; Modules is always an object, never null.
type readyzResponse struct {
	Status  string            `json:"status"`
	Reason  string            `json:"reason,omitempty"`
	Modules map[string]string `json:"modules"`
}

// Handler returns the route mux. Exported so callers and tests can drive the
// routes through httptest without binding a port.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		// A server built without a ReadyFunc fails closed rather than
		// claiming readiness it cannot verify.
		ready, reason, states := false, "no readiness check wired", map[string]string(nil)
		if s.ready != nil {
			ready, reason, states = s.ready(r.Context())
		}
		if states == nil {
			states = map[string]string{}
		}
		body := readyzResponse{Status: "ready", Modules: states}
		code := http.StatusOK
		if !ready {
			body.Status = "not ready"
			body.Reason = reason
			code = http.StatusServiceUnavailable
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		_ = json.NewEncoder(w).Encode(body)
	})
	return mux
}

// Start binds the listener synchronously — a port clash must fail the boot,
// not disappear into a goroutine — and then serves in the background. It is a
// no-op when the listener is disabled.
func (s *Server) Start() error {
	if s.addr == "" {
		return nil
	}
	ln, err := net.Listen("tcp", s.addr)
	if err != nil {
		return err
	}
	s.ln = ln
	s.srv = &http.Server{
		Handler:           s.Handler(),
		ReadHeaderTimeout: readHeaderTimeout,
	}
	// Serve always ends in an error: http.ErrServerClosed on the Shutdown
	// path, and on any other path the bind has already succeeded, so the
	// failure is the listener dying under a process that is on its way down
	// anyway. Server carries no logger by design (this package stays
	// module-agnostic), so there is nowhere useful to report it — the
	// boot-blocking failure mode is the bind above, which Start returns
	// synchronously.
	go func() { _ = s.srv.Serve(ln) }()
	return nil
}

// Addr returns the bound address (resolved, so a :0 port is concrete), or ""
// when the listener is disabled or has not been started.
func (s *Server) Addr() string {
	if s.ln == nil {
		return ""
	}
	return s.ln.Addr().String()
}

// Shutdown gracefully stops the listener. It is a no-op when the listener is
// disabled or was never started.
func (s *Server) Shutdown(ctx context.Context) error {
	if s.srv == nil {
		return nil
	}
	return s.srv.Shutdown(ctx)
}
