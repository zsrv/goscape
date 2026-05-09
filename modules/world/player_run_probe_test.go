package world

import (
	"context"
	"log/slog"
	"testing"

	"github.com/zsrv/goscape/pkg/objtype"
)

// nai138WorldHandler captures slog records for assertion. Mirrors the
// pkg/script-side capturingHandler used by NAI-128 tests.
type nai138WorldHandler struct {
	records []slog.Record
}

func (h *nai138WorldHandler) Enabled(_ context.Context, _ slog.Level) bool { return true }
func (h *nai138WorldHandler) Handle(_ context.Context, r slog.Record) error {
	h.records = append(h.records, r)
	return nil
}
func (h *nai138WorldHandler) WithAttrs(_ []slog.Attr) slog.Handler { return h }
func (h *nai138WorldHandler) WithGroup(_ string) slog.Handler      { return h }

func recordAttrs138(r slog.Record) map[string]any {
	m := map[string]any{}
	r.Attrs(func(a slog.Attr) bool {
		m[a.Key] = a.Value.Any()
		return true
	})
	return m
}

func TestUpdateEnergy_Probe_EmittedAtEnergyZeroWhenNodeDebugTrue(t *testing.T) {
	p, _ := newTestPlayer(t)
	s := newTestServer(t)
	s.varpTypes = &objtype.VarpTypeConfigs{
		Configs: make([]*objtype.VarPlayerType, 174),
		RunID:   173,
	}
	s.cfg.NodeDebug = true
	rec := &nai138WorldHandler{}
	s.log = slog.New(rec)
	s.currentTick = 42
	p.client.server = s

	p.varps = make([]int32, 174)
	p.varps[173] = 1
	p.run = 1
	p.stepsTaken = 2
	p.runweight = 0
	p.runenergy = 10 // drains to 0

	p.updateEnergy()

	var probe *slog.Record
	for i := range rec.records {
		if rec.records[i].Message == "nai138.update_energy.zero" {
			probe = &rec.records[i]
			break
		}
	}
	if probe == nil {
		t.Fatalf("nai138.update_energy.zero record absent; got %d records", len(rec.records))
	}
	got := recordAttrs138(*probe)
	if got["tick"] != int64(42) {
		t.Errorf(`attr "tick": got %v, want 42`, got["tick"])
	}
	if got["varp_id"] != int64(173) {
		t.Errorf(`attr "varp_id": got %v, want 173`, got["varp_id"])
	}
	if got["varp_pre"] != int64(1) {
		t.Errorf(`attr "varp_pre": got %v, want 1`, got["varp_pre"])
	}
	if got["run_pre"] != int64(1) {
		t.Errorf(`attr "run_pre": got %v, want 1`, got["run_pre"])
	}
}

func TestUpdateEnergy_Probe_SuppressedWhenNodeDebugFalse(t *testing.T) {
	p, _ := newTestPlayer(t)
	s := newTestServer(t)
	s.varpTypes = &objtype.VarpTypeConfigs{
		Configs: make([]*objtype.VarPlayerType, 174),
		RunID:   173,
	}
	s.cfg.NodeDebug = false
	rec := &nai138WorldHandler{}
	s.log = slog.New(rec)
	p.client.server = s

	p.varps = make([]int32, 174)
	p.varps[173] = 1
	p.run = 1
	p.stepsTaken = 2
	p.runweight = 0
	p.runenergy = 10

	p.updateEnergy()

	for _, r := range rec.records {
		if r.Message == "nai138.update_energy.zero" {
			t.Errorf("probe emitted under NodeDebug=false")
			return
		}
	}
}

func TestUpdateEnergy_Probe_NotEmittedWhenEnergyNonZero(t *testing.T) {
	p, _ := newTestPlayer(t)
	s := newTestServer(t)
	s.varpTypes = &objtype.VarpTypeConfigs{
		Configs: make([]*objtype.VarPlayerType, 174),
		RunID:   173,
	}
	s.cfg.NodeDebug = true
	rec := &nai138WorldHandler{}
	s.log = slog.New(rec)
	p.client.server = s

	p.varps = make([]int32, 174)
	p.run = 1
	p.stepsTaken = 2
	p.runweight = 0
	p.runenergy = 10000 // drain by 67 → 9933, not zero

	p.updateEnergy()

	for _, r := range rec.records {
		if r.Message == "nai138.update_energy.zero" {
			t.Errorf("probe emitted when energy != 0 (post-tick energy=%d)", p.runenergy)
			return
		}
	}
}
