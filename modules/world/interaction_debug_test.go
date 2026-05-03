package world

import (
	"context"
	"log/slog"
	"sync"
	"testing"

	entitypkg "github.com/zsrv/goscape/pkg/entity"
)

func TestChebDist(t *testing.T) {
	tests := []struct {
		name                   string
		ax, az, bx, bz, expect int
	}{
		{"same tile", 5, 5, 5, 5, 0},
		{"adjacent N", 5, 5, 5, 4, 1},
		{"diagonal", 5, 5, 6, 6, 1},
		{"two tiles E", 5, 5, 7, 5, 2},
		{"asymmetric 3x1", 5, 5, 8, 6, 3},
		{"negative direction", 10, 10, 7, 7, 3},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := chebDist(tc.ax, tc.az, tc.bx, tc.bz)
			if got != tc.expect {
				t.Errorf("chebDist(%d,%d,%d,%d) = %d, want %d",
					tc.ax, tc.az, tc.bx, tc.bz, got, tc.expect)
			}
		})
	}
}

func TestTargetKindString(t *testing.T) {
	loc := entitypkg.NewLoc(0, 1, 1, 1, 1, entitypkg.LifecycleForever, 0, 0, 0)
	obj := entitypkg.NewObj(0, 1, 1, entitypkg.LifecycleForever, 0, 1)
	npc := &Npc{}
	plr := &Player{}

	tests := []struct {
		name   string
		target entity
		expect string
	}{
		{"loc", loc, "Loc"},
		{"obj", obj, "Obj"},
		{"npc", npc, "Npc"},
		{"player", plr, "Player"},
		{"nil", nil, "unknown"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := targetKindString(tc.target)
			if got != tc.expect {
				t.Errorf("targetKindString(%T) = %q, want %q",
					tc.target, got, tc.expect)
			}
		})
	}
}

// capturingHandler is a slog.Handler that retains every Record passed to
// Handle so tests can assert on emitted frames.
type capturingHandler struct {
	mu      sync.Mutex
	records []slog.Record
}

func (h *capturingHandler) Enabled(_ context.Context, _ slog.Level) bool { return true }

func (h *capturingHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.records = append(h.records, r.Clone())
	return nil
}

func (h *capturingHandler) WithAttrs(_ []slog.Attr) slog.Handler { return h }
func (h *capturingHandler) WithGroup(_ string) slog.Handler      { return h }

func (h *capturingHandler) snapshot() []slog.Record {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]slog.Record, len(h.records))
	copy(out, h.records)
	return out
}

// newCapturingLogger returns a logger and the handler so tests can pull
// records back out. The logger emits at Debug level (matching production
// usage of s.log.Debug for instrumentation).
func newCapturingLogger() (*slog.Logger, *capturingHandler) {
	h := &capturingHandler{}
	return slog.New(h), h
}

// findRecord returns the first record with the given message, or nil.
func findRecord(records []slog.Record, msg string) *slog.Record {
	for i := range records {
		if records[i].Message == msg {
			return &records[i]
		}
	}
	return nil
}

// attrValue extracts the value of attribute `key` from `r`. Returns
// (slog.Value{}, false) if not found.
func attrValue(r slog.Record, key string) (slog.Value, bool) {
	var found slog.Value
	var ok bool
	r.Attrs(func(a slog.Attr) bool {
		if a.Key == key {
			found = a.Value
			ok = true
			return false
		}
		return true
	})
	return found, ok
}

// requireAttr fails the test if `key` is missing from `r` or its value
// (compared via String()) doesn't match `want`.
func requireAttr(t *testing.T, r slog.Record, key, want string) {
	t.Helper()
	v, ok := attrValue(r, key)
	if !ok {
		t.Fatalf("record %q missing attr %q", r.Message, key)
	}
	if got := v.String(); got != want {
		t.Errorf("record %q attr %q = %q, want %q", r.Message, key, got, want)
	}
}
