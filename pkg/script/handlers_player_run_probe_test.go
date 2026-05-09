package script

import (
	"context"
	"log/slog"
	"testing"
)

// captureLogger returns a recording handler + slog.Logger pair so tests
// can assert the exact slog records emitted by the gateway probe.
func captureLogger() (*nai138Handler, *slog.Logger) {
	h := &nai138Handler{}
	return h, slog.New(h)
}

type nai138Handler struct {
	records []slog.Record
}

func (h *nai138Handler) Enabled(_ context.Context, _ slog.Level) bool { return true }
func (h *nai138Handler) Handle(_ context.Context, r slog.Record) error {
	h.records = append(h.records, r)
	return nil
}
func (h *nai138Handler) WithAttrs(_ []slog.Attr) slog.Handler { return h }
func (h *nai138Handler) WithGroup(_ string) slog.Handler      { return h }

// recordAttrs collects a slog.Record's attrs into a map for assertion.
func recordAttrs(r slog.Record) map[string]any {
	m := map[string]any{}
	r.Attrs(func(a slog.Attr) bool {
		m[a.Key] = a.Value.Any()
		return true
	})
	return m
}

func TestPRun_Probe_EmittedWhenNodeDebugTrue(t *testing.T) {
	rec, lg := captureLogger()
	mp := &mockPlayer{
		runVarpID: 173,
		varps:     map[int]int32{173: 0},
	}
	s := &ScriptState{
		Script:    &ScriptFile{Name: "p_run_probe"},
		IntStack:  make([]int, StackCapacity),
		Self:      mp,
		Pointers:  PtrProtectedActivePlayer | PtrActivePlayer,
		NodeDebug: true,
		Log:       lg,
	}
	s.PushInt(1) // run-mode value to write

	if err := handlePRun(s); err != nil {
		t.Fatalf("handlePRun: %v", err)
	}
	if len(rec.records) != 1 {
		t.Fatalf("nai138.p_run records: got %d, want 1", len(rec.records))
	}
	r := rec.records[0]
	if r.Message != "nai138.p_run" {
		t.Errorf("message: got %q, want %q", r.Message, "nai138.p_run")
	}
	got := recordAttrs(r)
	if got["value"] != int64(1) {
		t.Errorf(`attr "value": got %v, want 1`, got["value"])
	}
	if got["varp_id"] != int64(173) {
		t.Errorf(`attr "varp_id": got %v, want 173`, got["varp_id"])
	}
	if got["varp_pre"] != int64(0) {
		t.Errorf(`attr "varp_pre": got %v, want 0`, got["varp_pre"])
	}
}

func TestPRun_Probe_SuppressedWhenNodeDebugFalse(t *testing.T) {
	rec, lg := captureLogger()
	mp := &mockPlayer{runVarpID: 173}
	s := &ScriptState{
		Script:   &ScriptFile{Name: "p_run_silent"},
		IntStack: make([]int, StackCapacity),
		Self:     mp,
		Pointers: PtrProtectedActivePlayer | PtrActivePlayer,
		// NodeDebug zero-value = false
		Log: lg,
	}
	s.PushInt(0)

	if err := handlePRun(s); err != nil {
		t.Fatalf("handlePRun: %v", err)
	}
	if len(rec.records) != 0 {
		t.Errorf("records under NodeDebug=false: got %d, want 0", len(rec.records))
	}
}

func TestPRun_Probe_NilLogSafe(t *testing.T) {
	mp := &mockPlayer{runVarpID: 173}
	s := &ScriptState{
		Script:    &ScriptFile{Name: "p_run_nil_log"},
		IntStack:  make([]int, StackCapacity),
		Self:      mp,
		Pointers:  PtrProtectedActivePlayer | PtrActivePlayer,
		NodeDebug: true,
		// Log nil
	}
	s.PushInt(1)

	if err := handlePRun(s); err != nil {
		t.Fatalf("handlePRun nil-Log: %v", err)
	}
}
