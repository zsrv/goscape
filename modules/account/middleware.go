package account

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

const sessionCookieName = "goscape_session"

type ctxKey int

const ctxKeyAccount ctxKey = 0

// ctxAccount returns the logged-in account attached by the middleware,
// or nil.
func ctxAccount(r *http.Request) *PortalAccount {
	a, _ := r.Context().Value(ctxKeyAccount).(*PortalAccount)
	return a
}

// csrfToken derives the per-session CSRF token from the RAW session
// cookie value: hex(sha256("goscape-csrf|" + raw)). Stateless and
// session-bound — the server never stores it, and it is worthless
// without the (HttpOnly) session cookie it is derived from.
func csrfToken(rawSessionToken string) string {
	sum := sha256.Sum256([]byte("goscape-csrf|" + rawSessionToken))
	return hex.EncodeToString(sum[:])
}

// public loads the session account (if any) into the request context.
func (p *portal) public(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if c, err := r.Cookie(sessionCookieName); err == nil {
			acct, err := p.store.SessionAccount(r.Context(), HashToken(c.Value), p.cfg.Session)
			switch {
			case err == nil && acct.Status == StatusActive:
				r = r.WithContext(context.WithValue(r.Context(), ctxKeyAccount, acct))
			case err == nil:
				// Disabled account with a live cookie: kill the session.
				_ = p.store.DeleteSession(r.Context(), HashToken(c.Value))
				p.clearSessionCookie(w)
			case errors.Is(err, ErrNotFound):
				p.clearSessionCookie(w)
			default:
				p.log.Error("session load failed", slog.Any("err", err))
			}
		}
		h(w, r)
	}
}

// requireCSRF checks the CSRF token on state-changing methods, writing
// a 403 and returning false if it is missing or wrong. GET/HEAD are
// exempt (and pass).
func requireCSRF(w http.ResponseWriter, r *http.Request) bool {
	if r.Method == http.MethodGet || r.Method == http.MethodHead {
		return true
	}
	c, err := r.Cookie(sessionCookieName)
	if err != nil || subtle.ConstantTimeCompare([]byte(r.FormValue("csrf")), []byte(csrfToken(c.Value))) != 1 {
		http.Error(w, "invalid csrf token", http.StatusForbidden)
		return false
	}
	return true
}

// authed requires a logged-in account and enforces CSRF on
// state-changing methods.
func (p *portal) authed(h http.HandlerFunc) http.HandlerFunc {
	return p.public(func(w http.ResponseWriter, r *http.Request) {
		if ctxAccount(r) == nil {
			http.Redirect(w, r, "/login?msg=Please+log+in", http.StatusFound)
			return
		}
		if !requireCSRF(w, r) {
			return
		}
		h(w, r)
	})
}

// admin is admin-group membership + CSRF. Anyone who isn't an admin —
// including anonymous visitors — gets 404 rather than the 302 that
// authed() would give a logged-out user: the admin surface itself is
// not advertised. NOTE: this deliberately does not wrap authed(),
// whose login-redirect would leak the existence of an admin-only route
// to anonymous requests before the group check ever runs.
func (p *portal) admin(h http.HandlerFunc) http.HandlerFunc {
	return p.public(func(w http.ResponseWriter, r *http.Request) {
		acct := ctxAccount(r)
		isAdmin := false
		if acct != nil {
			ok, err := p.store.IsGroupMember(r.Context(), GroupAdmin, acct.ID)
			if err != nil {
				p.log.Error("admin check failed", slog.Any("err", err))
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
			isAdmin = ok
		}
		if !isAdmin {
			http.NotFound(w, r)
			return
		}
		if !requireCSRF(w, r) {
			return
		}
		h(w, r)
	})
}

func (p *portal) setSessionCookie(w http.ResponseWriter, raw string) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    raw,
		Path:     "/",
		MaxAge:   int(p.cfg.Session.AbsoluteTTL / time.Second),
		HttpOnly: true,
		Secure:   strings.HasPrefix(p.cfg.PublicURL, "https://"),
		SameSite: http.SameSiteLaxMode,
	})
}

func (p *portal) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookieName, Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, Secure: strings.HasPrefix(p.cfg.PublicURL, "https://"), SameSite: http.SameSiteLaxMode,
	})
}

// clientIP extracts the remote IP for rate-limit keys and audit rows.
func clientIP(r *http.Request) string {
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}

// rateLimiter is a fixed-window in-memory counter: cheap, per-process,
// good enough for portal abuse control (spec: in-memory token buckets).
type rateLimiter struct {
	mu      sync.Mutex
	windows map[string]*rlWindow
}

type rlWindow struct {
	start  time.Time
	count  int
	window time.Duration // the window duration this entry was created with, for eviction
}

// rlEvictionThreshold bounds rateLimiter.windows: keys come from
// attacker-controlled input (IP/email on anonymous endpoints like
// /login and /register), so without a bound a sustained flood of
// distinct keys grows the map without limit. Once the map reaches this
// size, allow() sweeps every entry whose own window has already
// expired before proceeding — cheap relative to the flood that would
// be needed to keep the map above the threshold.
const rlEvictionThreshold = 4096

func newRateLimiter() *rateLimiter {
	return &rateLimiter{windows: make(map[string]*rlWindow)}
}

func (rl *rateLimiter) allow(key string, limit int, window time.Duration) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	now := time.Now()
	if len(rl.windows) >= rlEvictionThreshold {
		for k, w := range rl.windows {
			if now.Sub(w.start) >= w.window {
				delete(rl.windows, k)
			}
		}
	}
	w, ok := rl.windows[key]
	if !ok || now.Sub(w.start) >= window {
		rl.windows[key] = &rlWindow{start: now, count: 1, window: window}
		return true
	}
	w.count++
	return w.count <= limit
}
