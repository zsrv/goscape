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
	}, true)
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
	}, true)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest("GET", "/healthz", nil))
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("stale tick: got %d, want 503", rr.Code)
	}
}

func TestHealthzNoWorld(t *testing.T) {
	mux := http.NewServeMux()
	RegisterHealthRoutes(mux, func() (HealthSnapshot, bool) { return HealthSnapshot{}, false }, true)
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
	}, true)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest("GET", "/healthz", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("pre-first-tick boot grace: got %d, want 200", rr.Code)
	}
}

// TestHealthzStatus exercises the pure decision helper directly, so the
// boot-deadline and staleness branches can be pinned without a real clock.
func TestHealthzStatus(t *testing.T) {
	tests := []struct {
		name      string
		snap      HealthSnapshot
		hasWorld  bool
		sinceBoot time.Duration
		wantCode  int
	}{
		{"no world is process-up", HealthSnapshot{}, false, time.Hour, http.StatusOK},
		{"before first tick within boot grace", HealthSnapshot{LastTick: time.Unix(0, 0)}, true, 5 * time.Second, http.StatusOK},
		{"before first tick past boot grace", HealthSnapshot{LastTick: time.Unix(0, 0)}, true, healthzBootGrace + time.Second, http.StatusServiceUnavailable},
		{"zero time.Time past boot grace", HealthSnapshot{}, true, healthzBootGrace + time.Second, http.StatusServiceUnavailable},
		{"fresh tick", HealthSnapshot{LastTick: time.Now()}, true, time.Hour, http.StatusOK},
		{"stale tick", HealthSnapshot{LastTick: time.Now().Add(-time.Minute)}, true, time.Hour, http.StatusServiceUnavailable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if code, _ := healthzStatus(tt.snap, tt.hasWorld, tt.sinceBoot); code != tt.wantCode {
				t.Fatalf("healthzStatus = %d, want %d", code, tt.wantCode)
			}
		})
	}
}

func TestDebugStatusJSON(t *testing.T) {
	mux := http.NewServeMux()
	RegisterHealthRoutes(mux, func() (HealthSnapshot, bool) {
		return HealthSnapshot{LastTick: time.Now(), CurrentTick: 7, PlayersOnline: 2, LastCycleMillis: 9}, true
	}, true)
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
	RegisterHealthRoutes(mux, func() (HealthSnapshot, bool) {
		return HealthSnapshot{LastTick: time.Now()}, true
	}, false)
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
