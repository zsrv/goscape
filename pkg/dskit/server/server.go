package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/zsrv/goscape/pkg/dskit/middleware"
	"github.com/zsrv/goscape/pkg/dskit/signals"
)

const (
	DefaultNetwork = "tcp"
	NetworkTCPV4   = "tcp4"
)

// SignalHandler used by Server.
type SignalHandler interface {
	// Loop starts the signals handler. This method is blocking, and returns
	// only after signal is received, or Stop is called.
	Loop()

	// Stop blocked Loop method.
	Stop()
}

// Config for a Server.
type Config struct {
	HTTPListenNetwork string `yaml:"http_listen_network"`
	HTTPListenAddress string `yaml:"http_listen_address"`
	HTTPListenPort    int    `yaml:"http_listen_port"`

	ExcludeRequestInLog      bool `yaml:"-"`
	DisableRequestSuccessLog bool `yaml:"-"`

	ServerGracefulShutdownTimeout time.Duration `yaml:"graceful_shutdown_timeout"`
	HTTPServerReadTimeout         time.Duration `yaml:"http_server_read_timeout"`
	HTTPServerReadHeaderTimeout   time.Duration `yaml:"http_server_read_header_timeout"`
	HTTPServerWriteTimeout        time.Duration `yaml:"http_server_write_timeout"`
	HTTPServerIdleTimeout         time.Duration `yaml:"http_server_idle_timeout"`

	HTTPLogClosedConnectionsWithoutResponse bool `yaml:"http_log_closed_connections_without_response_enabled"`

	HTTPMiddleware                []middleware.Interface `yaml:"-"`
	Router                        *http.ServeMux         `yaml:"-"`
	DoNotAddDefaultHTTPMiddleware bool                   `yaml:"-"`

	LogFormat                    string       `yaml:"log_format"`
	LogLevel                     *slog.Level  `yaml:"log_level"`
	Log                          *slog.Logger `yaml:"-"`
	LogSourceIPs                 bool         `yaml:"log_source_ips_enabled"`
	LogSourceIPsFull             bool         `yaml:"log_source_ips_full"`
	LogSourceIPsHeader           string       `yaml:"log_source_ips_header"`
	LogSourceIPsRegex            string       `yaml:"log_source_ips_regex"`
	LogRequestHeaders            bool         `yaml:"log_request_headers"`
	LogRequestAtInfoLevel        bool         `yaml:"log_request_at_info_level_enabled"`
	LogRequestExcludeHeadersList string       `yaml:"log_request_exclude_headers_list"`

	// If not set, default signal handler is used.
	SignalHandler SignalHandler `yaml:"-"`

	PathPrefix string `yaml:"http_path_prefix"`
}

// Server wraps an HTTP server, and some common initialization.
type Server struct {
	cfg          Config
	handler      SignalHandler
	httpListener net.Listener

	HTTP       *http.ServeMux
	HTTPServer *http.Server
	Log        *slog.Logger
}

// New makes a new Server.
func New(cfg Config) (*Server, error) {
	return newServer(cfg)
}

func newServer(cfg Config) (*Server, error) {
	logger := cfg.Log

	network := cfg.HTTPListenNetwork
	if network == "" {
		network = DefaultNetwork
	}

	// Set up listeners first, so we can fail early if the port is in use
	httpListener, err := net.Listen(network, net.JoinHostPort(cfg.HTTPListenAddress, strconv.Itoa(cfg.HTTPListenPort)))
	if err != nil {
		return nil, err
	}

	logger.Info("server listening", "http", httpListener.Addr())

	// Set up HTTP server
	var router *http.ServeMux
	if cfg.Router != nil {
		router = cfg.Router
	} else {
		router = http.NewServeMux()
	}

	httpMiddleware, err := BuildHTTPMiddleware(cfg, logger)
	if err != nil {
		// arch-29.8: httpListener is already bound (see "Set up listeners
		// first" above) — close it on this error path so a middleware
		// build failure doesn't leak the socket.
		_ = httpListener.Close()
		return nil, fmt.Errorf("error building http middleware: %w", err)
	}

	httpServer := &http.Server{
		ReadTimeout:       cfg.HTTPServerReadTimeout,
		ReadHeaderTimeout: cfg.HTTPServerReadHeaderTimeout,
		WriteTimeout:      cfg.HTTPServerWriteTimeout,
		IdleTimeout:       cfg.HTTPServerIdleTimeout,
		Handler:           middleware.Merge(httpMiddleware...).Wrap(router),
	}

	handler := cfg.SignalHandler
	if handler == nil {
		handler = signals.NewHandler(logger)
	}

	return &Server{
		cfg:          cfg,
		httpListener: httpListener,
		handler:      handler,

		HTTP:       router,
		HTTPServer: httpServer,
		Log:        logger,
	}, nil
}

// Run the server; blocks until SIGTERM (if signal handling is enabled), an error is received, or Stop() is called.
func (s *Server) Run() error {
	errChan := make(chan error, 1)

	// Wait for a signal
	go func() {
		s.handler.Loop()
		select {
		case errChan <- nil:
		default:
		}
	}()

	// TODO: TLS support
	go func() {
		err := s.HTTPServer.Serve(s.httpListener)
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}

		select {
		case errChan <- err:
		default:
		}
	}()

	return <-errChan
}

// Stop unblocks Run().
func (s *Server) Stop() {
	s.handler.Stop()
}

// Shutdown gracefully shuts the server down. Should be deferred after New().
func (s *Server) Shutdown() {
	ctx, cancel := context.WithTimeout(context.Background(), s.cfg.ServerGracefulShutdownTimeout)
	defer cancel() // releases resources if httpServer.Shutdown completes before timeout elapses

	_ = s.HTTPServer.Shutdown(ctx)
}

func BuildHTTPMiddleware(cfg Config, logger *slog.Logger) ([]middleware.Interface, error) {
	sourceIPs, err := middleware.NewSourceIPs(cfg.LogSourceIPsHeader, cfg.LogSourceIPsRegex, cfg.LogSourceIPsFull)
	if err != nil {
		return nil, fmt.Errorf("error setting up source IP extraction: %w", err)
	}
	logSourceIPs := sourceIPs
	if !cfg.LogSourceIPs {
		// We always include the source IPs for traces,
		// but only want to log them in the middleware if that is enabled.
		logSourceIPs = nil
	}

	defaultLogMiddleware := middleware.NewLogMiddleware(
		logger,
		cfg.LogRequestHeaders,
		cfg.LogRequestAtInfoLevel,
		logSourceIPs,
		strings.Split(cfg.LogRequestExcludeHeadersList, ","),
	)
	defaultLogMiddleware.DisableRequestSuccessLog = cfg.DisableRequestSuccessLog

	defaultHTTPMiddleware := []middleware.Interface{
		middleware.RouteInjector{},
		middleware.Tracer{
			SourceIPs: sourceIPs,
		},
		defaultLogMiddleware,
	}
	var httpMiddleware []middleware.Interface
	if cfg.DoNotAddDefaultHTTPMiddleware {
		httpMiddleware = cfg.HTTPMiddleware
	} else {
		httpMiddleware = append(defaultHTTPMiddleware, cfg.HTTPMiddleware...)
	}

	return httpMiddleware, nil
}
