package ondemand

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// readyFunc builds the verdict func RegisterHealthRoutes takes. The real one
// is the adapter in cmd/goscape/app/modules.go, which asks
// world.Server.CheckReady and translates world.ErrStarting to nil (the legacy
// "starting is a 200" behaviour this route has always had).
func readyFunc(err error, hasWorld bool) func(context.Context) (error, bool) {
	return func(context.Context) (error, bool) { return err, hasWorld }
}

func alwaysSnap(s HealthSnapshot, hasWorld bool) func() (HealthSnapshot, bool) {
	return func() (HealthSnapshot, bool) { return s, hasWorld }
}

// assertHealthz pins both halves of the response contract: the status code and
// the exact body bytes. The bodies are load-bearing — operators and the Helm
// chart match on them — so they must survive the move of the decision itself
// into modules/world.
func assertHealthz(t *testing.T, ready func(context.Context) (error, bool), wantCode int, wantBody string) {
	t.Helper()
	mux := http.NewServeMux()
	RegisterHealthRoutes(mux, ready, alwaysSnap(HealthSnapshot{}, false), true)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest("GET", "/healthz", nil))
	if rr.Code != wantCode {
		t.Fatalf("/healthz code = %d, want %d", rr.Code, wantCode)
	}
	if got := rr.Body.String(); got != wantBody {
		t.Fatalf("/healthz body = %q, want %q", got, wantBody)
	}
}

func TestHealthzFreshTick(t *testing.T) {
	assertHealthz(t, readyFunc(nil, true), http.StatusOK, "")
}

func TestHealthzStaleTick(t *testing.T) {
	assertHealthz(t, readyFunc(errors.New("tick stale"), true), http.StatusServiceUnavailable, "tick stale\n")
}

// TestHealthzNoFirstTick pins the other wedged verdict's body.
func TestHealthzNoFirstTick(t *testing.T) {
	assertHealthz(t, readyFunc(errors.New("no first tick"), true), http.StatusServiceUnavailable, "no first tick\n")
}

func TestHealthzNoWorld(t *testing.T) {
	// hasWorld=false short-circuits to a plain process-up 200 even if the
	// verdict func were to report an error.
	assertHealthz(t, readyFunc(errors.New("tick stale"), false), http.StatusOK, "")
}

// TestHealthzBootGraceBeforeFirstTick pins the boot-grace decision
// (arch-29.6) at the HTTP level: a world that is wired but hasn't completed
// its first tick yet must read as "still starting up" (200), not "wedged"
// (503) — otherwise every world takes a guaranteed 503 between process start
// and its first completed tick. The grace itself now lives in
// modules/world (world.ErrStarting); the app adapter translates it to a nil
// verdict before it reaches this route, which is what this test injects.
func TestHealthzBootGraceBeforeFirstTick(t *testing.T) {
	assertHealthz(t, readyFunc(nil, true), http.StatusOK, "")
}

// TestHealthzUsesRequestContext confirms the verdict func is called with the
// request's context, so a client that goes away can cancel the check.
func TestHealthzUsesRequestContext(t *testing.T) {
	type ctxKey struct{}
	var got bool
	mux := http.NewServeMux()
	RegisterHealthRoutes(mux, func(ctx context.Context) (error, bool) {
		got = ctx.Value(ctxKey{}) == "yes"
		return nil, true
	}, alwaysSnap(HealthSnapshot{}, false), false)

	req := httptest.NewRequest("GET", "/healthz", nil)
	req = req.WithContext(context.WithValue(req.Context(), ctxKey{}, "yes"))
	mux.ServeHTTP(httptest.NewRecorder(), req)

	if !got {
		t.Fatal("ready func was not called with the request context")
	}
}

func TestDebugStatusJSON(t *testing.T) {
	mux := http.NewServeMux()
	RegisterHealthRoutes(mux, readyFunc(nil, true), alwaysSnap(
		HealthSnapshot{LastTick: time.Now(), CurrentTick: 7, PlayersOnline: 2, LastCycleMillis: 9}, true), true)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest("GET", "/debug/status", nil))
	var got map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("bad JSON: %v", err)
	}
	if got["players_online"].(float64) != 2 || got["current_tick"].(float64) != 7 {
		t.Fatalf("unexpected payload: %v", got)
	}
	if got["ticking"] != true {
		t.Fatalf("ticking: got %v, want true", got["ticking"])
	}
}

// SEC1 M-12: /debug/status is off unless explicitly enabled; /healthz
// is unaffected.
func TestDebugStatusDisabledByDefault(t *testing.T) {
	mux := http.NewServeMux()
	RegisterHealthRoutes(mux, readyFunc(nil, true), alwaysSnap(
		HealthSnapshot{LastTick: time.Now()}, true), false)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest("GET", "/debug/status", nil))
	if rr.Code != http.StatusNotFound {
		t.Fatalf("/debug/status when disabled: got %d, want 404", rr.Code)
	}
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest("GET", "/healthz", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("/healthz: got %d, want 200", rr.Code)
	}
}
