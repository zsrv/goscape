package ondemand

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHealthzFreshTick(t *testing.T) {
	mux := http.NewServeMux()
	RegisterHealthRoutes(mux, func() (HealthSnapshot, bool) {
		return HealthSnapshot{LastTick: time.Now(), CurrentTick: 42, PlayersOnline: 3, LastCycleMillis: 12}, true
	})
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest("GET", "/healthz", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("fresh tick: got %d, want 200", rr.Code)
	}
}

func TestHealthzStaleTick(t *testing.T) {
	mux := http.NewServeMux()
	RegisterHealthRoutes(mux, func() (HealthSnapshot, bool) {
		return HealthSnapshot{LastTick: time.Now().Add(-time.Minute)}, true
	})
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest("GET", "/healthz", nil))
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("stale tick: got %d, want 503", rr.Code)
	}
}

func TestHealthzNoWorld(t *testing.T) {
	mux := http.NewServeMux()
	RegisterHealthRoutes(mux, func() (HealthSnapshot, bool) { return HealthSnapshot{}, false })
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest("GET", "/healthz", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("standalone ondemand: got %d, want 200 (process-up)", rr.Code)
	}
}

// TestHealthzBootGraceBeforeFirstTick pins the boot-grace decision
// (arch-29.6): a world that is wired but hasn't completed its first tick
// yet reports LastTick as time.Unix(0, 0) (world.Server.HealthSnapshot's
// atomic zero value), not a real timestamp. That must read as "still
// starting up" (200), not "wedged" (503) — otherwise every world takes a
// guaranteed 503 between process start and its first completed tick.
func TestHealthzBootGraceBeforeFirstTick(t *testing.T) {
	mux := http.NewServeMux()
	RegisterHealthRoutes(mux, func() (HealthSnapshot, bool) {
		return HealthSnapshot{LastTick: time.Unix(0, 0)}, true
	})
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest("GET", "/healthz", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("pre-first-tick boot grace: got %d, want 200", rr.Code)
	}
}

func TestDebugStatusJSON(t *testing.T) {
	mux := http.NewServeMux()
	RegisterHealthRoutes(mux, func() (HealthSnapshot, bool) {
		return HealthSnapshot{LastTick: time.Now(), CurrentTick: 7, PlayersOnline: 2, LastCycleMillis: 9}, true
	})
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
