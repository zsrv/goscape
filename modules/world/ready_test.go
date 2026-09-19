package world

import (
	"errors"
	"path/filepath"
	"testing"
	"time"
)

// TestReadyStatus exercises the pure decision helper directly, so the
// boot-deadline and staleness branches can be pinned without a real clock.
// Moved here from modules/ondemand's TestHealthzStatus when the tick-loop
// readiness check became the world's own (the ondemand /healthz route keeps
// its HTTP-level table tests).
func TestReadyStatus(t *testing.T) {
	tests := []struct {
		name      string
		snap      HealthSnapshot
		sinceBoot time.Duration
		wantErr   error
	}{
		{"before first tick within boot grace", HealthSnapshot{LastTick: time.Unix(0, 0)}, 5 * time.Second, ErrStarting},
		{"before first tick past boot grace", HealthSnapshot{LastTick: time.Unix(0, 0)}, readyBootGrace + time.Second, errNoFirstTick},
		{"zero time.Time past boot grace", HealthSnapshot{}, readyBootGrace + time.Second, errNoFirstTick},
		{"fresh tick", HealthSnapshot{LastTick: time.Now()}, time.Hour, nil},
		{"stale tick", HealthSnapshot{LastTick: time.Now().Add(-time.Minute)}, time.Hour, errTickStale},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := readyStatus(tt.snap, tt.sinceBoot)
			if !errors.Is(got, tt.wantErr) {
				t.Fatalf("readyStatus = %v, want %v", got, tt.wantErr)
			}
		})
	}
}

// TestReadyStatusReasonStrings pins the reason text: these strings are the
// ondemand /healthz response bodies operators and the Helm chart already
// match on, and the "world: <err>" reasons /readyz reports.
func TestReadyStatusReasonStrings(t *testing.T) {
	for want, err := range map[string]error{
		"starting":      ErrStarting,
		"no first tick": errNoFirstTick,
		"tick stale":    errTickStale,
	} {
		if err.Error() != want {
			t.Errorf("%v.Error() = %q, want %q", err, err.Error(), want)
		}
	}
}

// TestCheckReadyUsesServerBootTime confirms CheckReady reads the server's own
// construction time as the boot-grace reference, not some caller-supplied
// clock: a freshly stamped bootTime yields ErrStarting before the first tick,
// and an old one yields the wedged verdict.
func TestCheckReadyUsesServerBootTime(t *testing.T) {
	s := newTestServer(t)

	s.bootTime = time.Now()
	if err := s.CheckReady(t.Context()); !errors.Is(err, ErrStarting) {
		t.Fatalf("fresh server before first tick: CheckReady = %v, want ErrStarting", err)
	}

	s.bootTime = time.Now().Add(-readyBootGrace - time.Second)
	if err := s.CheckReady(t.Context()); !errors.Is(err, errNoFirstTick) {
		t.Fatalf("past boot grace with no tick: CheckReady = %v, want %v", err, errNoFirstTick)
	}

	s.lastTickNano.Store(time.Now().UnixNano())
	if err := s.CheckReady(t.Context()); err != nil {
		t.Fatalf("ticking world: CheckReady = %v, want nil", err)
	}

	s.lastTickNano.Store(time.Now().Add(-time.Minute).UnixNano())
	if err := s.CheckReady(t.Context()); !errors.Is(err, errTickStale) {
		t.Fatalf("stale tick: CheckReady = %v, want %v", err, errTickStale)
	}
}

// TestNewServerStampsBootTime pins that the boot-grace reference is set at
// construction — a zero bootTime would make every world read as "past the
// grace" the instant it starts, so a real world would answer "no first tick"
// for the whole of its startup instead of "starting".
//
// NewServer performs full cache loading, so this shares the Server244-ref
// fixture the other NewServer tests use (server_lifecycle_test.go) and skips
// with them when it is unavailable.
func TestNewServerStampsBootTime(t *testing.T) {
	cachePath := ref244CacheDir(t)
	// cfg.WordEncPath is resolved by encfilter.Load relative to cwd — switch
	// to the repo root so the committed data/raw/wordenc jagfile is reachable.
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	t.Chdir(repoRoot)

	cfg := Config{
		CachePath:   cachePath,
		WordEncPath: filepath.Join("data", "raw", "wordenc"), // matches world.wordenc-path default
	}
	before := time.Now()
	s, err := NewServer(cfg, nil, nil, discardLogger(), nil)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	if s.bootTime.Before(before) || s.bootTime.After(time.Now()) {
		t.Fatalf("bootTime = %v, want a stamp taken during NewServer", s.bootTime)
	}
	if err := s.CheckReady(t.Context()); !errors.Is(err, ErrStarting) {
		t.Fatalf("a freshly constructed world: CheckReady = %v, want ErrStarting", err)
	}
}
