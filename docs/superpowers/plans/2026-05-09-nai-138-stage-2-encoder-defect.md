# NAI-138 Stage 2 — Probe-first encoder-defect investigation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Bind which of three hypotheses (A sequencing / B tick timing / C encoder-byte) is the actual defect behind the run-toggle button not visually de-toggling at energy=0, then ship the fix at the bound layer.

**Architecture:** Stage 1 §6.3 routed to §7.4 (Goscape encoder defect) via the synthesis matrix, but a goscape-side pre-flight refuted an encoder-byte defect. Stage 2 follows the handoff's probe-first cadence: instrument three NodeDebug-gated `s.log.Info` gateways spanning the run-varp emit pathway (Bundle β.1), hand off to user smoke for paired energy=0 + click-toggle runs, synthesize the log shape (Bundle β.2), then author the fix at the bound hypothesis (Bundle β.3) with TDD red→green→commit. Smoke handoff #2 verifies visual button de-toggle.

**Tech Stack:** Go 1.26+; `pkg/io/packet` (RS2 binary buffer); `pkg/io/protocol/game/server` (opcode constants); `modules/world` (Player, Server, writeVarp, updateEnergy); `pkg/script` (handlePRun, ScriptState); `log/slog` (NodeDebug-gated probes per `nodedebug_gateway_probe_pattern` memory).

**Predecessor:** NAI-138 Stage 1 closed at `d33bd78` (handoff commit). Spec §6.3 + handoff `docs/superpowers/handoffs/2026-05-09-nai-138-stage-1-binding.md`.

---

## File Structure

**Modify:**
- `pkg/script/handlers_player.go:635-644` — add G1 probe to `handlePRun` (script-side P_RUN dispatch).
- `modules/world/player_run.go:43-46` — add G2 probe to `(*Player).updateEnergy` energy=0 branch.
- `modules/world/player_varp.go:12-26` — add G3 probe to `(*Player).writeVarp` (universal wire-byte capture).

**Create (tests):**
- `pkg/script/handlers_player_run_probe_test.go` — focused unit tests for G1 emit shape and gating.
- `modules/world/player_run_probe_test.go` — focused unit tests for G2 emit shape and gating.
- `modules/world/player_varp_probe_test.go` — focused unit tests for G3 emit shape and gating.

**Bundle β.3 fix layer (one of):**
- Hypothesis A fix: `modules/world/player_run.go` and/or `modules/world/player_script.go` (sequencing fix at the call-chain that mints `varps[173]=1` on click).
- Hypothesis B fix: `modules/world/tick.go` (move `processEnergy` call relative to the tick-end flush).
- Hypothesis C fix: `pkg/io/packet/packet.go` (P1/P2 encoder primitives) and/or `modules/world/player_varp.go` (emit ordering inside `writeVarp`).

---

## Bundle β.1 — Probe instrumentation

Three gateways. All NodeDebug-gated; permanent diagnostic per `nodedebug_gateway_probe_pattern`. Unique log key prefix `nai138.` for grep. Each task is a self-contained TDD red→green→commit cycle.

### Task β.1.G1: Probe `handlePRun` (P_RUN dispatch site)

**Files:**
- Modify: `pkg/script/handlers_player.go:635-644`
- Test: `pkg/script/handlers_player_run_probe_test.go` (CREATE)

Captures the click-pathway script-side dispatch. Pattern mirrors `handleNpcFindHero` (`pkg/script/handlers_npc.go:1113`) — `s.NodeDebug && s.Log != nil` gate, `s.Log.Info("nai138.p_run", …)` emit. Captures the run-mode value being written, the resolved varp id, the pre-write `varps[id]` value (read via `s.Self.Varp`), and `s.World.CurrentTick()` if `s.World != nil`.

- [ ] **Step 1: Write the failing tests**

Create `pkg/script/handlers_player_run_probe_test.go`:

```go
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run "TestPRun_Probe_" -v`
Expected: FAIL — emit not present, record count = 0.

- [ ] **Step 3: Add the G1 probe to handlePRun**

Edit `pkg/script/handlers_player.go:635-644`. Replace the entire `handlePRun` body so the probe runs BEFORE `SetVarp` (so `varp_pre` reads the pre-write value):

```go
func handlePRun(s *ScriptState) error {
	if err := requireProtectedActivePlayer(s, "P_RUN"); err != nil {
		return err
	}
	v := s.PopInt()
	s.Self.SetRun(v)
	varpID := s.Self.RunVarpID()
	if s.NodeDebug && s.Log != nil {
		var (
			scriptName string
			tick       int
			varpPre    int32
		)
		if s.Script != nil {
			scriptName = s.Script.Name
		}
		if s.World != nil {
			tick = s.World.CurrentTick()
		}
		varpPre = s.Self.Varp(varpID)
		s.Log.Info("nai138.p_run",
			"script_name", scriptName,
			"script_pc", s.PC,
			"tick", tick,
			"value", v,
			"varp_id", varpID,
			"varp_pre", varpPre,
		)
	}
	// todo: better way to sync engine varp (mirrored from TS PlayerOps.ts:1207)
	s.Self.SetVarp(varpID, int32(v))
	return nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run "TestPRun_Probe_" -v`
Expected: PASS (3 tests).

- [ ] **Step 5: Run full pkg/script suite to verify no regression**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add pkg/script/handlers_player.go pkg/script/handlers_player_run_probe_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(nai-138): G1 probe — handlePRun emit shape

NodeDebug-gated nai138.p_run gateway captures the script-side
P_RUN dispatch context: value being written, resolved varp_id,
pre-write varp value, tick. Stage 2 Bundle β.1 of three probes
spanning the run-varp emit pathway. Per nodedebug_gateway_probe_pattern
permanent diagnostic.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task β.1.G2: Probe `(*Player).updateEnergy` energy=0 branch

**Files:**
- Modify: `modules/world/player_run.go:29-50`
- Test: `modules/world/player_run_probe_test.go` (CREATE)

Captures the energy=0 emit pathway (Pathway A from spec §6.1). Pattern mirrors `(*Npc) damage` at `modules/world/npc_masks.go:178` — `n.server != nil && n.server.cfg.NodeDebug && n.server.log != nil` gate. Player-side equivalent uses `p.client != nil && p.client.server != nil && p.client.server.cfg.NodeDebug && p.client.server.log != nil`. Emits with the resolved varp id, pre-write `p.varps[id]`, run-state pre, and `currentTick`.

- [ ] **Step 1: Write the failing tests**

Create `modules/world/player_run_probe_test.go`:

```go
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run "TestUpdateEnergy_Probe_" -v`
Expected: FAIL — `nai138.update_energy.zero record absent`.

- [ ] **Step 3: Add the G2 probe to updateEnergy**

Edit `modules/world/player_run.go`. Replace the energy=0 branch so the probe fires before `SetVarp`:

```go
func (p *Player) updateEnergy() {
	if p.delayed {
		return
	}
	if p.stepsTaken < 2 {
		agility := int(p.baseLevels[objtype.PlayerStatAgility])
		recovered := agility/9 + 8
		p.runenergy = min(p.runenergy+recovered, 10000)
	} else {
		weightKg := p.runweight / 1000
		clampWeight := max(min(weightKg, 64), 0)
		loss := 67 + 67*clampWeight/64
		p.runenergy = max(p.runenergy-loss, 0)
	}
	if p.runenergy == 0 {
		varpID := p.RunVarpID()
		if p.client != nil && p.client.server != nil &&
			p.client.server.cfg.NodeDebug && p.client.server.log != nil {
			var varpPre int32
			if varpID >= 0 && varpID < len(p.varps) {
				varpPre = p.varps[varpID]
			}
			p.client.server.log.Info("nai138.update_energy.zero",
				"tick", p.client.server.currentTick,
				"player_uid", p.uid,
				"varp_id", varpID,
				"varp_pre", varpPre,
				"run_pre", p.run,
			)
		}
		p.run = 0
		p.SetVarp(varpID, 0)
	}
	if p.runenergy < 100 {
		p.tempRun = 0
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run "TestUpdateEnergy_" -v`
Expected: PASS (all existing TestUpdateEnergy_* tests + 3 new TestUpdateEnergy_Probe_*).

- [ ] **Step 5: Commit**

```bash
git add modules/world/player_run.go modules/world/player_run_probe_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(nai-138): G2 probe — updateEnergy energy=0 emit shape

NodeDebug-gated nai138.update_energy.zero gateway captures the
energy=0 reset emit context: resolved varp_id, pre-write varp
value, run-state pre, tick, player_uid. Pathway A binding for
the Stage 2 hypothesis matrix (A sequencing / B tick timing /
C encoder bytes).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task β.1.G3: Probe `(*Player).writeVarp` (universal wire-byte capture)

**Files:**
- Modify: `modules/world/player_varp.go:12-26`
- Test: `modules/world/player_varp_probe_test.go` (CREATE)

Captures actual on-wire bytes for every varp emit, regardless of caller. Bind for Hypothesis C (encoder-byte divergence). Logs `id`, `value`, `opcode` (the gameserver Op constant numeric value), `payload_hex` (the `buf.Bytes()` rendered as hex string), `payload_len`. Probe fires AFTER the buffer is built but BEFORE `writeOut`, so a panic in `writeOut` doesn't suppress the diagnostic.

- [ ] **Step 1: Write the failing tests**

Create `modules/world/player_varp_probe_test.go`:

```go
package world

import (
	"encoding/hex"
	"log/slog"
	"testing"

	gameserver "github.com/zsrv/goscape/pkg/io/protocol/game/server"
	"github.com/zsrv/goscape/pkg/objtype"
)

func TestWriteVarp_Probe_SmallValueShape(t *testing.T) {
	p, _ := newTestPlayer(t)
	s := newTestServer(t)
	s.varpTypes = &objtype.VarpTypeConfigs{
		Configs: make([]*objtype.VarPlayerType, 174),
		RunID:   173,
	}
	s.varpTypes.Configs[173] = &objtype.VarPlayerType{Transmit: true}
	s.cfg.NodeDebug = true
	rec := &nai138WorldHandler{}
	s.log = slog.New(rec)
	p.client.server = s

	p.writeVarp(173, 0)

	var probe *slog.Record
	for i := range rec.records {
		if rec.records[i].Message == "nai138.write_varp" {
			probe = &rec.records[i]
			break
		}
	}
	if probe == nil {
		t.Fatalf("nai138.write_varp record absent; got %d records", len(rec.records))
	}
	got := recordAttrs138(*probe)
	if got["id"] != int64(173) {
		t.Errorf(`attr "id": got %v, want 173`, got["id"])
	}
	if got["value"] != int64(0) {
		t.Errorf(`attr "value": got %v, want 0`, got["value"])
	}
	if got["opcode"] != int64(gameserver.OpVarpSmall.Opcode) {
		t.Errorf(`attr "opcode": got %v, want %d (OpVarpSmall)`,
			got["opcode"], gameserver.OpVarpSmall.Opcode)
	}
	// Payload: P2(173) + P1(0) = 0x00, 0xAD, 0x00 (3 bytes)
	wantHex := hex.EncodeToString([]byte{0x00, 0xAD, 0x00})
	if got["payload_hex"] != wantHex {
		t.Errorf(`attr "payload_hex": got %q, want %q`, got["payload_hex"], wantHex)
	}
	if got["payload_len"] != int64(3) {
		t.Errorf(`attr "payload_len": got %v, want 3`, got["payload_len"])
	}
}

func TestWriteVarp_Probe_LargeValueShape(t *testing.T) {
	p, _ := newTestPlayer(t)
	s := newTestServer(t)
	s.varpTypes = &objtype.VarpTypeConfigs{
		Configs: make([]*objtype.VarPlayerType, 174),
		RunID:   173,
	}
	s.varpTypes.Configs[100] = &objtype.VarPlayerType{Transmit: true}
	s.cfg.NodeDebug = true
	rec := &nai138WorldHandler{}
	s.log = slog.New(rec)
	p.client.server = s

	p.writeVarp(100, 200) // 200 > 127 → VARP_LARGE

	var probe *slog.Record
	for i := range rec.records {
		if rec.records[i].Message == "nai138.write_varp" {
			probe = &rec.records[i]
			break
		}
	}
	if probe == nil {
		t.Fatalf("nai138.write_varp record absent; got %d records", len(rec.records))
	}
	got := recordAttrs138(*probe)
	if got["opcode"] != int64(gameserver.OpVarpLarge.Opcode) {
		t.Errorf(`attr "opcode": got %v, want %d (OpVarpLarge)`,
			got["opcode"], gameserver.OpVarpLarge.Opcode)
	}
	if got["payload_len"] != int64(6) {
		t.Errorf(`attr "payload_len": got %v, want 6 (P2 id + P4 value)`, got["payload_len"])
	}
}

func TestWriteVarp_Probe_SuppressedWhenNodeDebugFalse(t *testing.T) {
	p, _ := newTestPlayer(t)
	s := newTestServer(t)
	s.varpTypes = &objtype.VarpTypeConfigs{
		Configs: make([]*objtype.VarPlayerType, 174),
		RunID:   173,
	}
	s.varpTypes.Configs[173] = &objtype.VarPlayerType{Transmit: true}
	s.cfg.NodeDebug = false
	rec := &nai138WorldHandler{}
	s.log = slog.New(rec)
	p.client.server = s

	p.writeVarp(173, 0)

	for _, r := range rec.records {
		if r.Message == "nai138.write_varp" {
			t.Errorf("probe emitted under NodeDebug=false")
			return
		}
	}
}

func TestWriteVarp_Probe_NotEmittedForNonTransmitVarp(t *testing.T) {
	p, _ := newTestPlayer(t)
	s := newTestServer(t)
	s.varpTypes = &objtype.VarpTypeConfigs{
		Configs: make([]*objtype.VarPlayerType, 174),
		RunID:   173,
	}
	s.varpTypes.Configs[173] = &objtype.VarPlayerType{Transmit: false}
	s.cfg.NodeDebug = true
	rec := &nai138WorldHandler{}
	s.log = slog.New(rec)
	p.client.server = s

	p.writeVarp(173, 0)

	for _, r := range rec.records {
		if r.Message == "nai138.write_varp" {
			t.Errorf("probe emitted for non-transmit varp (writeVarp early-returned, " +
				"so probe must not fire)")
			return
		}
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run "TestWriteVarp_Probe_" -v`
Expected: FAIL — `nai138.write_varp record absent`.

- [ ] **Step 3: Add the G3 probe to writeVarp**

Edit `modules/world/player_varp.go`. Replace the entire `writeVarp` so the probe fires after the buffer is built but before `writeOut`:

```go
func (p *Player) writeVarp(id int, value int32) {
	cfg := p.varpTypeConfig(id)
	if cfg == nil || !cfg.Transmit {
		return
	}
	buf := packet.NewPacket(nil)
	buf.P2(uint16(id))
	var op gameserver.Op
	if value >= -128 && value <= 127 {
		buf.P1(uint8(int8(value)))
		op = gameserver.OpVarpSmall
	} else {
		buf.P4(uint32(value))
		op = gameserver.OpVarpLarge
	}
	payload := buf.Bytes()
	if p.client != nil && p.client.server != nil &&
		p.client.server.cfg.NodeDebug && p.client.server.log != nil {
		p.client.server.log.Info("nai138.write_varp",
			"tick", p.client.server.currentTick,
			"player_uid", p.uid,
			"id", id,
			"value", value,
			"opcode", int(op.Opcode),
			"payload_hex", hex.EncodeToString(payload),
			"payload_len", len(payload),
		)
	}
	p.writeOut(op, payload)
}
```

Also add `"encoding/hex"` to the import block at the top of the file.

- [ ] **Step 4: Run tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run "TestWriteVarp_" -v`
Expected: PASS (4 new TestWriteVarp_Probe_* tests).

- [ ] **Step 5: Run full modules/world suite to verify no regression**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add modules/world/player_varp.go modules/world/player_varp_probe_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(nai-138): G3 probe — writeVarp universal wire-byte capture

NodeDebug-gated nai138.write_varp gateway captures actual on-wire
bytes for every transmit-eligible varp emit, regardless of caller.
Logs id, value, opcode (small vs large), payload_hex, payload_len.
Bind for Hypothesis C (encoder-byte divergence) — covers both the
energy=0 (Pathway A) and click-toggle (Pathway B) emit paths.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task β.1.SMOKE: User smoke handoff #1 — paired runs

**No code change.** Controller produces a smoke-handoff doc with explicit instructions for the user, then stops. Per `smoke_test_server_handoff` memory.

- [ ] **Step 1: Compose handoff doc**

Create `docs/superpowers/handoffs/2026-05-09-nai-138-stage-2-probe-smoke.md` with:

1. Server build + run command:
   ```bash
   GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache CGO_ENABLED=0 go build -trimpath -o /tmp/goscape-nai138 ./cmd/goscape
   /tmp/goscape-nai138 --config.file config.yaml 2>&1 | tee /tmp/nai138-probe.log
   ```
   (NodeDebug defaults to `true` per `modules/world/config.go:76` `f.BoolVar(&c.NodeDebug, "world.node-debug", true, …)` — no extra flag needed.)

2. Paired smoke runs:
   - **Run A — energy=0 path:** Log in, ensure run-mode is ON (toggle if needed). Walk continuously until run-energy depletes to 0 (the orb shows 0%). Observe whether the run-mode button visually de-toggles. Note the symptom (stays-on / de-toggles).
   - **Run B — click-toggle path (control):** Log in (or use same session, fresh login preferred), full energy. Click the run-mode button to toggle from ON→OFF. Observe button de-toggles correctly. This is the working-baseline reference.

3. Capture logs:
   ```bash
   grep -E "nai138\.(p_run|update_energy\.zero|write_varp)" /tmp/nai138-probe.log > /tmp/nai138-probe-filtered.log
   ```

4. Hand back the filtered log + visual symptom observation.

- [ ] **Step 2: Pause the controller — wait for user to run smoke and return logs.**

The Bundle β.2 synthesis cannot proceed without the smoke output. Stop here and wait.

---

## Bundle β.2 — Synthesis (controller-only)

**No code change. No commits.** Controller reads the filtered log + smoke observation and binds Hypothesis A / B / C per the decision table below. Result is appended to the spec doc as §6.4 (or to a new `nai138-stage-2-binding.md` handoff if §6 grows unwieldy).

### Decision table

| Energy=0 G2 record | G3 record(s) | Binding |
|---|---|---|
| present, `varp_pre = 0` | present (energy=0 path) | **Hypothesis A — Sequencing.** Client `varps[173]` was already 0 server-side at the moment of the energy=0 emit. The redundant emit hits the client-side `if (varps[var26] != var52)` short-circuit at `Client-Java/deob/client.java:9367`. Server-side run-state didn't transition through 1 even though the client UI thought it did. Fix layer: ensure varp[173] correctly reaches 1 server-side at the moment p.run is set to 1 (audit the click-toggle path's server-side state writes). |
| present, `varp_pre = 1` | present, `payload_hex` matches expected `00 AD 00` | **Hypothesis A.b — Client-side desync** (server is correct, client never received the prior varp[173]=1). Same fix family as A: audit click-path. Possibly a missed `writeVarp` on the click→on transition. |
| present, `varp_pre = 1` | present, `payload_hex` divergent from `00 AD 00` | **Hypothesis C — Encoder-byte divergence.** Compare hex against TS-emitted bytes (Bundle 3 Template α empirical TS smoke if needed). Fix layer: `pkg/io/packet` P1/P2 primitives or `(*Player).writeVarp` byte ordering. |
| present, `varp_pre = 1` | present, hex matches, **but** G3 records show ordering relative to other tick-end packets that differs from click path | **Hypothesis B — Tick timing.** The energy=0 emit is flushed in a different position relative to other server messages than the click-toggle emit, and the client only triggers `redrawSidebar` evaluation at certain frame boundaries. Fix layer: `modules/world/tick.go` ordering of `processEnergy` relative to `processOut` flush. |
| G2 absent | — | Test fixture wired wrong, OR `cfg.NodeDebug` defaulted off in the smoke run. Re-handoff the smoke with `--world.node-debug` explicitly. |

### Pre-Bundle β.3 verification step

Per `audit_subagent_fabrication` and the spec §5 pattern: before dispatching β.3, controller independently re-grep + Read each cited file:line in the binding to confirm the citation still matches HEAD. Specifically:

- `Client-Java/deob/client.java:9367` short-circuit (cite for Hyp A).
- `modules/world/tick.go` `processEnergy` invocation site (cite for Hyp B).
- `pkg/io/packet/packet.go` P1/P2 implementations (cite for Hyp C).

Document the verification in the handoff alongside the binding.

---

## Bundle β.3 — Fix at bound hypothesis

**One of the three template tasks below runs**, selected by the Bundle β.2 binding. The two unrun templates remain in this plan as documentation (do not delete on close — provides reverse-mapping for future smoke regressions).

Each template follows TDD red→green→commit per `test-driven-development`. Each commit body cites the bound hypothesis and the smoke evidence path.

### Template β.3.A — Hypothesis A (Sequencing) fix

**Symptom from probe:** Energy=0 G2 record shows `varp_pre = 0` even though the client UI displayed run-mode ON before the depletion. Server-side `p.varps[173]` never transitioned to 1 during the click-on session, OR transitioned momentarily but was reset by an unrelated path before the energy=0 tick.

**Investigation step (controller, before TDD):** grep all writers of `p.varps[173]` (or general `RunVarpID()`):

```bash
rg -n "RunVarpID|varps\[.*173|SetVarp\(.*173|p\.run\b" modules/world/ pkg/script/
```

Identify which writer should land `1` on click but doesn't, or which writer resets it between click and energy=0.

**Likely root causes:**
- `(*Player).RunVarpID()` returns 0 instead of 173 at toggle-time (cache-resolution race or shim-default leak).
- `handlePRun` calls `s.Self.SetVarp(s.Self.RunVarpID(), int32(v))` but `s.Self` resolves to a stale Player handle (audit ScriptState wiring at click-path entry).
- An interfering tick-init step (e.g., the `tick.go:181-189` varps-init loop that runs at login) hits varps[173] mid-session.

**TDD Files:**
- Test: `modules/world/player_run_sequencing_test.go` (CREATE)
- Modify: site identified by investigation (likely `modules/world/player_run.go` or `modules/world/player_script.go` `SetVarp`).

- [ ] **Step 1: Write a failing test that pins `varps[173] == 1` after the failing flow.** Test exercises the exact server-side sequence the probe surfaced (e.g., click→walk-tick chain). Use existing `newTestPlayer` + `newTestServer` patterns from `modules/world/player_run_test.go:252-282`.

- [ ] **Step 2: Run** `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run "TestRunVarp_Sequencing" -v` — Expected: FAIL.

- [ ] **Step 3: Author the fix.** Code shape determined by investigation; commit body cites the specific writer site.

- [ ] **Step 4: Run** `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run "TestRunVarp_Sequencing" -v` — Expected: PASS.

- [ ] **Step 5: Run full goscape suite** — `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...` — Expected: PASS.

- [ ] **Step 6: Commit** with message `fix(nai-138): hypothesis A — <one-line summary>` and body citing probe-log evidence + TS-source comparison if any. **No deviation tag** — this is a fidelity catch-up.

---

### Template β.3.B — Hypothesis B (Tick timing) fix

**Symptom from probe:** Energy=0 G2 + G3 records present, varp_pre=1, payload bytes correct, but the G3 records show the energy=0 emit in a different position relative to other tick-end packets compared to the click-toggle path.

**Investigation step (controller, before TDD):** locate the call site of `processEnergy` in `tick.go:57` and trace what runs before/after relative to `processOut` (the per-tick flush). Compare against where the click-driven `handlePRun` emit lands in the same tick.

**TDD Files:**
- Test: `modules/world/tick_run_varp_timing_test.go` (CREATE)
- Modify: `modules/world/tick.go` (move the `processEnergy` call, OR add an explicit flush trigger after the energy=0 emit).

- [ ] **Step 1: Write a failing test that pins the post-fix tick ordering.** Capture wire bytes via `flushWrite()` + `clientConn` read (pattern from `modules/world/handlers_game_test.go:397` `drainAfterTele`). Assert energy=0 emit lands within the same flush boundary as a sibling reference packet.

- [ ] **Step 2: Run** the test — Expected: FAIL.

- [ ] **Step 3: Author the fix** in `tick.go`. Likely a 1–3 line move of `s.processEnergy()` relative to `s.processOut()`. Document the TS counterpart at `Engine-TS/src/engine/World.ts:731` and the call ordering there.

- [ ] **Step 4: Run** the test — Expected: PASS.

- [ ] **Step 5: Run full suite.**

- [ ] **Step 6: Commit** `fix(nai-138): hypothesis B — reorder processEnergy in tick loop`. Cite TS World.ts counterpart.

---

### Template β.3.C — Hypothesis C (Encoder-byte divergence) fix

**Symptom from probe:** G3 record's `payload_hex` diverges from the expected `00 AD 00` (for VarpSmall(173, 0)) or whatever sequence TS emits.

**Investigation step (controller, before TDD):** identify the divergent byte position. Cross-reference TS `Engine-TS/src/engine/io/Packet.ts` `p1`/`p2` implementations and the `Engine-TS/src/engine/network/outgoing/codec/VarpSmallEncoder.ts` (or equivalent) byte ordering.

**TDD Files:**
- Test: `modules/world/player_varp_encoder_test.go` (CREATE) AND/OR `pkg/io/packet/packet_test.go` (modify).
- Modify: `pkg/io/packet/packet.go` (P1/P2/P4 primitives) AND/OR `modules/world/player_varp.go` (byte order).

- [ ] **Step 1: Write a failing roundtrip test.** Per `rsbuf_roundtrip_tests` memory: assert byte-for-byte against the expected wire sequence using a Java-client-reader-order decode. Use `packet.NewPacket(buf.Bytes())` to read back via `G2`/`G1` and confirm round-trip semantics.

- [ ] **Step 2: Run** `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/io/packet/... ./modules/world/ -run "TestVarp" -v` — Expected: FAIL on the divergent byte assertion.

- [ ] **Step 3: Author the fix.** Per `dispatch_order_audit_blind_spot`: also compare the dispatch ORDER not just byte-level (e.g., is `P2` emitting big-endian when client expects little-endian).

- [ ] **Step 4: Run** the tests — Expected: PASS.

- [ ] **Step 5: Run full suite.**

- [ ] **Step 6: Commit** `fix(nai-138): hypothesis C — VarpSmall encoder byte order` with TS source citation + roundtrip pin.

---

## Smoke handoff #2 — Stage 2 close gate

**No code change.** Controller produces handoff after Bundle β.3 commit per `smoke_test_server_handoff`.

- [ ] **Step 1: Compose handoff** `docs/superpowers/handoffs/2026-05-09-nai-138-stage-2-fix-smoke.md`:

Same server build + run command as smoke #1. User repeats Run A (deplete energy to 0). Decision tree per spec §8:

| Outcome | Close action |
|---|---|
| Button visually de-toggles at energy=0 | PRIMARY met → close NAI-138 |
| Button stays stuck-on, click toggles still work | Re-open Bundle β.1 with sharper probes scoped to the next hypothesis (per spec §10 R2 — the binding may have been wrong) |
| New regression (click toggles broken, etc.) | Revert Bundle β.3 commit; open NAI-138 stretch with regression-pin test |

- [ ] **Step 2: Pause and wait for user smoke output.**

---

## Memory routing at Stage 2 close

Per the handoff doc + spec §11. Write all four entries on close commit; cite via `Closes memory:` trailer per `close_commit_memory_trailer`:

- `cs1_re_eval_triggers.md` — already exists from Stage 1; verify content matches. If not, update.
- `runescript_self_write_semantics.md` — already exists from Stage 1; verify.
- `lostcity_no_varp_trigger.md` — already exists from Stage 1; verify.
- NEW per Stage 2 fix: `nai138_<bound-hypothesis>.md` — root-cause finding + fix shape + smoke evidence reference. E.g., `nai138_run_varp_sequencing.md` for Hyp A, `nai138_run_varp_tick_timing.md` for Hyp B, `nai138_run_varp_encoder.md` for Hyp C.
- Update `nai_followups.md` line 6432 (NAI-137 carryover): replace open NAI-138+ candidate with closed NAI-138 entry, include final fix-layer + post-fix smoke evidence.

If Bundle β.3 was Template β.3.B (tick-timing) or Template β.3.C (encoder), additionally add to `nai_followups.md`: "audit other bare-`setVar`-at-tick-end callers for the same defect family" (per `latent_bug_at_migration_boundary` memory pattern — fixing one symptom may surface siblings).

---

## Self-review notes

**Spec coverage:** Bundles β.1 / β.2 / β.3 + two smoke handoffs map directly to the user brief and spec §6.3 routing. Memory routing covers spec §11.

**Probe-pattern verification:**
- `s.NodeDebug && s.Log != nil` for pkg/script (matches `handlers_npc.go:1113` `nai128.npc.findhero` precedent).
- `p.client != nil && p.client.server != nil && p.client.server.cfg.NodeDebug && p.client.server.log != nil` for modules/world (matches `npc_masks.go:178` `nai128.npc.damage` precedent).
- Unique key prefix `nai138.` mirrors `nai128.` reference impl from the gateway-probe pattern memory.

**Citation freshness (verified at plan-write 2026-05-09):**
- `pkg/script/handlers_player.go:635-644` — `handlePRun` body confirmed at HEAD.
- `modules/world/player_run.go:43-46` — energy=0 branch confirmed at HEAD.
- `modules/world/player_varp.go:12-26` — `writeVarp` body confirmed at HEAD.
- `pkg/script/state.go:229-235` — `NodeDebug bool` + `Log *slog.Logger` fields confirmed.
- `modules/world/script.go:47-48` — `state.NodeDebug = s.cfg.NodeDebug` + `state.Log = s.log` wiring confirmed.
- `modules/world/config.go:76` — `world.node-debug` defaults to `true`, so smoke needs no extra flag.
- `pkg/script/runner_test.go:402-413` — `mockPlayer.Varp` / `SetVarp` / `RunVarpID` test surface confirmed.
- `modules/world/server_test.go:311-326` — `newTestServer` confirmed; `s.cfg`, `s.log`, `s.currentTick` all assignable post-construction.

**Sibling-site guard audit (per `plan_sibling_site_guard_audit`):** modules/world probe sites use the four-clause guard chain (`p.client != nil && p.client.server != nil && cfg.NodeDebug && log != nil`) matching the reference impl exactly. No simplification.

**Plan-author gotchas avoided:**
- G1 in `handlePRun` reads `varp_pre` via `s.Self.Varp(varpID)` not direct field access — works for both production `(*Player)` and test `mockPlayer` per the `ActivePlayer.Varp` interface (state.go:55, runner_test.go:402-407).
- G2 reads `varp_pre` from `p.varps[varpID]` directly with bounds check; this is server-side state at the moment of the energy=0 reset, which is exactly what Hypothesis A's binding requires.
- G3 builds the payload first, captures it, then calls `writeOut` — so a panic in `writeOut` doesn't suppress the diagnostic, AND the captured bytes match exactly what hits the wire.
- Test fixtures populate `s.varpTypes.Configs[id] = &objtype.VarPlayerType{Transmit: true}` — without this the `writeVarp` early-return at line 13-15 (`cfg == nil || !cfg.Transmit`) hides the probe.
