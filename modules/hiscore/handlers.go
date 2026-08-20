package hiscore

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/zsrv/goscape/pkg/dskit/middleware"
)

// Error codes. These are part of the public contract.
const (
	codeNotFound       = "not_found"
	codeInvalidRequest = "invalid_request"
	codeRateLimited    = "rate_limited"
	codeInternal       = "internal"
)

type errorEnvelope struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// api holds the HTTP surface. now is injectable so tests can freeze the
// clock that drives the visibility filter and the limiter. It must
// always yield UTC: modernc.org/sqlite serializes time.Time via
// t.String() with no zone conversion, so a non-UTC clock would corrupt
// every comparison against stored (UTC) timestamps.
type api struct {
	cfg       Config
	store     *Store
	sourceIPs *middleware.SourceIPExtractor
	limiter   *backstop
	now       func() time.Time
	log       *slog.Logger
}

func newAPI(cfg Config, store *Store, log *slog.Logger) (*api, error) {
	// Always construct the extractor: with header and regex both blank
	// it uses dskit's built-in Forwarded / X-Real-IP / X-Forwarded-For
	// chain, which is the default and is what a gateway populates.
	// NewSourceIPs only errors when exactly one of the pair is set.
	sourceIPs, err := middleware.NewSourceIPs(
		cfg.Server.LogSourceIPsHeader, cfg.Server.LogSourceIPsRegex, cfg.Server.LogSourceIPsFull)
	if err != nil {
		return nil, fmt.Errorf("hiscore: source IP extractor: %w", err)
	}
	return &api{
		cfg:       cfg,
		store:     store,
		sourceIPs: sourceIPs,
		limiter:   newBackstop(cfg.BackstopRate),
		now:       func() time.Time { return time.Now().UTC() },
		log:       log,
	}, nil
}

// register wires the routes. Every route goes through guard, which
// applies the backstop limiter.
func (a *api) register(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/skills", a.guard(a.handleSkills))
	// A catch-all so unmatched paths get the JSON error shape rather
	// than net/http's text 404.
	mux.HandleFunc("/", a.guard(func(w http.ResponseWriter, r *http.Request) {
		a.writeError(w, http.StatusNotFound, codeNotFound, "no such resource")
	}))
}

// identify resolves the caller for logging and limiter keying.
func (a *api) identify(r *http.Request) caller {
	c := consumerFromHeaders(r, a.cfg.TrustGatewayHeaders)
	c.IP = a.clientIP(r)
	return c
}

// clientIP prefers the configured proxy header (dskit's extractor,
// which is how the real client IP is recovered from behind a gateway)
// and falls back to the socket peer.
func (a *api) clientIP(r *http.Request) string {
	if a.sourceIPs != nil {
		if ip := a.sourceIPs.Get(r); ip != "" {
			return ip
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// guard applies the backstop limiter to a handler.
func (a *api) guard(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c := a.identify(r)
		if !a.limiter.allow(c.limiterKey(), a.now()) {
			w.Header().Set("Retry-After", "60")
			a.writeError(w, http.StatusTooManyRequests, codeRateLimited,
				"too many requests; retry after 60s")
			return
		}
		h(w, r)
	}
}

func (a *api) handleSkills(w http.ResponseWriter, r *http.Request) {
	a.writeJSON(w, r, struct {
		Skills []Skill `json:"skills"`
	}{Skills: Skills()}, time.Time{})
}

// writeJSON encodes v, sets caching headers, and honours If-None-Match.
// lastMod is written as Last-Modified when non-zero; build-static
// responses pass the zero time and carry an ETag only.
func (a *api) writeJSON(w http.ResponseWriter, r *http.Request, v any, lastMod time.Time) {
	body, err := json.Marshal(v)
	if err != nil {
		a.log.Error("hiscore: encoding response", slog.Any("err", err))
		a.writeError(w, http.StatusInternalServerError, codeInternal, "internal error")
		return
	}

	sum := sha256.Sum256(body)
	etag := `"` + base64.RawURLEncoding.EncodeToString(sum[:16]) + `"`

	h := w.Header()
	h.Set("Content-Type", "application/json; charset=utf-8")
	h.Set("ETag", etag)
	h.Set("Cache-Control", "public, max-age="+strconv.Itoa(int(a.cfg.CacheMaxAge.Seconds())))
	if !lastMod.IsZero() {
		h.Set("Last-Modified", lastMod.UTC().Format(http.TimeFormat))
	}

	if match := r.Header.Get("If-None-Match"); match != "" && match == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(body); err != nil {
		a.log.Debug("hiscore: writing response", slog.Any("err", err))
	}
}

func (a *api) writeError(w http.ResponseWriter, status int, code, msg string) {
	var env errorEnvelope
	env.Error.Code = code
	env.Error.Message = msg

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	// Errors are per-caller and must not be cached at the edge.
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(env); err != nil {
		a.log.Debug("hiscore: writing error response", slog.Any("err", err))
	}
}
