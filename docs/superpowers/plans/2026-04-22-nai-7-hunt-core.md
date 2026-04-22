# NAI-7 NPC Hunt Core — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add the hunt-skeleton to `*Npc`: 4 hunt fields (huntMode/huntRange/huntClock/huntTarget), `processNpcHunt` tick-pass helper, `huntAll` dispatcher with stubbed variant functions, `OpNpcSetHunt` / `OpNpcSetHuntMode` opcode wiring, and extension of NAI-5's `revertType` to reset hunt fields.

**Architecture:** Three tasks. (1) Fields + NewNpc seeds + revertType extension + SetHuntRange/SetHuntMode methods + ActiveNpc interface + mock stubs + 3 unit tests. (2) 2 opcode handlers + registrations + mockNpc recording + 4 handler tests. (3) `processNpcHunt` + `huntAll` + 4 variant stubs in new `modules/world/npc_hunt.go` + turn() wire + 3 integration tests — closes NAI-7.

**Tech Stack:** Go 1.26+, existing `pkg/objtype.HuntType` (NAI-1), existing tick loop, existing `entity` interface at `modules/world/movement_consts.go:45`.

**Spec:** `docs/superpowers/specs/2026-04-22-nai-7-hunt-core-design.md`

**Roadmap:** `docs/superpowers/specs/2026-04-22-npc-ai-tick-decomposition-design.md`

---

## File Structure

**Created:**
- `modules/world/npc_hunt.go` — `processNpcHunt`, `huntAll`, 4 variant stubs

**Modified:**
- `modules/world/npc.go` — `+4 hunt fields`, NewNpc seeds `huntMode`/`huntRange`, `+SetHuntRange`/`+SetHuntMode` methods, extend `revertType` with hunt resets
- `modules/world/npc_ai.go` — `+s.processNpcHunt(n)` call between isValid gate and `processNpcRegen`
- `pkg/script/active.go` — extend `ActiveNpc` with `SetHuntRange`, `SetHuntMode`
- `pkg/script/handlers_npc.go` — `+handleNpcSetHunt`, `+handleNpcSetHuntMode`
- `pkg/script/handlers.go` — register both opcodes
- `pkg/script/handlers_npc_test.go` — `mockNpc` recording fields + stubs + 4 handler tests
- `pkg/script/handlers_player_test.go` — `mockActiveNpc` no-op stubs
- `modules/world/npc_event_queue_test.go` — 3 unit + 3 integration tests

Three tasks. Task 3 closes NAI-7.

---

## Task 1: Hunt fields + NewNpc seed + revertType extension + Set methods + interface + mock stubs + 3 unit tests

**Files:**
- Modify: `modules/world/npc.go` (fields + NewNpc + methods + revertType)
- Modify: `pkg/script/active.go` (ActiveNpc extension)
- Modify: `pkg/script/handlers_npc_test.go` (mockNpc stubs)
- Modify: `pkg/script/handlers_player_test.go` (mockActiveNpc stubs)
- Modify: `modules/world/npc_event_queue_test.go` (add 3 unit tests)

- [ ] **Step 1: Write the failing tests**

Append to `modules/world/npc_event_queue_test.go`:

```go
func TestNewNpcSeedsHuntFromType(t *testing.T) {
	typ := &objtype.NpcType{
		ConfigType: objtype.ConfigType{ID: 0, DebugName: "test_npc"},
		HuntMode:   3,
		HuntRange:  5,
	}
	n := NewNpc(1, 0, 3094, 3106, 0, typ)

	if n.huntMode != 3 {
		t.Errorf("huntMode: got %d, want 3 (seeded from typ.HuntMode)", n.huntMode)
	}
	if n.huntRange != 5 {
		t.Errorf("huntRange: got %d, want 5 (seeded from typ.HuntRange)", n.huntRange)
	}
}

func TestNpcSetHuntRangeAndMode(t *testing.T) {
	n := newNpcForLifecycleTest(t)

	n.SetHuntRange(7)
	if n.huntRange != 7 {
		t.Errorf("huntRange after SetHuntRange(7): got %d, want 7", n.huntRange)
	}

	n.SetHuntMode(2)
	if n.huntMode != 2 {
		t.Errorf("huntMode after SetHuntMode(2): got %d, want 2", n.huntMode)
	}

	// -1 is a valid clear value (not a no-op like SetTimer).
	n.SetHuntMode(-1)
	if n.huntMode != -1 {
		t.Errorf("huntMode after SetHuntMode(-1): got %d, want -1 (clear)", n.huntMode)
	}
}

func TestNpcRevertTypeResetsHuntFields(t *testing.T) {
	// newNpcForLifecycleTest seeds a typ with HuntMode=0 (default from
	// objtype.NpcType zero value — test expects reset to that).
	// Use a typ with explicit HuntMode=2, HuntRange=4 so we can verify
	// the reset brings fields BACK to those values after scripts mutate them.
	typ := &objtype.NpcType{
		ConfigType: objtype.ConfigType{ID: 0, DebugName: "test_npc"},
		Stats:      []uint16{0, 0, 0, 10, 0, 0},
		Category:   -1,
		HuntMode:   2,
		HuntRange:  4,
	}
	n := NewNpc(1, 0, 3094, 3106, 0, typ)

	// Mutate all 4 hunt fields (simulating live hunt state).
	n.huntRange = 99
	n.huntMode = 0
	n.huntClock = 42
	n.huntTarget = nil // already nil; just documenting the expected reset

	n.revertType()

	if n.huntRange != 4 {
		t.Errorf("huntRange: got %d, want 4 (reset from typ.HuntRange)", n.huntRange)
	}
	if n.huntMode != 2 {
		t.Errorf("huntMode: got %d, want 2 (reset from typ.HuntMode)", n.huntMode)
	}
	if n.huntClock != 0 {
		t.Errorf("huntClock: got %d, want 0 (reset)", n.huntClock)
	}
	if n.huntTarget != nil {
		t.Errorf("huntTarget: got %v, want nil (reset)", n.huntTarget)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run:
```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestNewNpcSeedsHuntFromType|TestNpcSetHuntRangeAndMode|TestNpcRevertTypeResetsHuntFields' -v
```

Expected: compile errors — `n.huntMode undefined`, `n.SetHuntRange undefined`, etc.

- [ ] **Step 3: Add 4 hunt fields to `*Npc`**

In `modules/world/npc.go`, the existing `Npc` struct has an `// === interaction ===` block with `target entity; faceEntity int`. Add a new `// === hunt ===` block immediately after the `// === script state ===` block (which ends with `regenInterval`/`regenClock` fields after NAI-6):

```go
	// === hunt ===
	huntMode   int
	huntRange  int
	huntClock  int
	huntTarget entity
```

(gofmt realigns column widths.)

- [ ] **Step 4: Seed huntMode + huntRange in `NewNpc`**

In `NewNpc` struct literal, add two lines near the other `typ.*`-sourced seeds (grouped with `respawnRate`, `timerInterval`, `regenInterval`):

```go
		respawnRate:     int(typ.RespawnRate),
		timerInterval:   int(typ.Timer),
		regenInterval:   int(typ.RegenRate),
		huntMode:        typ.HuntMode,
		huntRange:       int(typ.HuntRange),
```

(gofmt realigns.)

**Important:** `NewNpc` currently does NOT set `huntMode = -1` explicitly; Go's zero-init makes that 0. But `typ.HuntMode` defaults to -1 (from `NewNpcType`), so when `typ.HuntMode: typ.HuntMode,` seeds, n.huntMode starts at typ's value (-1 for "no hunt" or a real id). Verify the test uses explicit `HuntMode: 3` to get a non-default value.

- [ ] **Step 5: Add `SetHuntRange` and `SetHuntMode` methods**

Append in `modules/world/npc.go`, after existing NAI-2..NAI-6 methods (near `SetTimer`):

```go
// SetHuntRange sets the NPC's hunt search radius. Called by the
// NPC_SETHUNT opcode. Implements script.ActiveNpc.SetHuntRange.
func (n *Npc) SetHuntRange(r int) {
	n.huntRange = r
}

// SetHuntMode sets the NPC's HuntType id. -1 clears the hunt mode
// (unlike SetTimer's -1 no-op — SetHuntMode accepts -1 as a valid
// "clear" command). Callers do no bounds validation; the consumer
// (processNpcHunt) validates when looking up the HuntType. Mirrors
// TS NpcOps.ts:178-185. Implements script.ActiveNpc.SetHuntMode.
func (n *Npc) SetHuntMode(mode int) {
	n.huntMode = mode
}
```

- [ ] **Step 6: Extend `revertType` with hunt-field resets**

In the existing `revertType` method on `*Npc` (added in NAI-5 Task 1), append at the very end (after the existing `n.masks |= rsbuf.NpcMaskChangeType` line):

```go
	// NAI-7: hunt-field resets. Matches TS resetEntity at
	// Engine-TS/.../Npc.ts:309-312.
	n.huntClock = 0
	n.huntTarget = nil
	if n.typ != nil {
		n.huntRange = int(n.typ.HuntRange)
		n.huntMode = n.typ.HuntMode
	}
```

- [ ] **Step 7: Extend `ActiveNpc` interface**

In `pkg/script/active.go`, locate the `ActiveNpc` interface. After existing NAI-2/3/4/6 methods (`StoreActiveScript`, `ClearActiveScript`, `SetDelayed`, `EnqueueScriptForTrigger`, `SetTimer`), append just before the closing `}`:

```go
	// SetHuntRange sets the NPC's hunt search radius. Called by
	// the NPC_SETHUNT opcode. Matches TS NpcOps.ts:174-176 — despite
	// the opcode name, this sets RANGE only; mode uses SetHuntMode.
	SetHuntRange(r int)

	// SetHuntMode sets the NPC's HuntType id. -1 clears. Callers
	// do no bounds validation; the hunt processor validates when
	// looking up the HuntType. Mirrors TS NpcOps.ts:178-185.
	SetHuntMode(mode int)
```

- [ ] **Step 8: Update mocks (compile fixes)**

In `pkg/script/handlers_npc_test.go`, locate the `mockNpc` struct. Add two new fields at the end:

```go
	enqueueCalls                       []mockEnqueueCall
	setTimerCalls                      []int
	setHuntRangeCalls                  []int
	setHuntModeCalls                   []int
```

Locate the existing NAI-4 `SetTimer` method and add two new recording methods immediately after it:

```go
func (m *mockNpc) SetHuntRange(r int) {
	m.setHuntRangeCalls = append(m.setHuntRangeCalls, r)
}

func (m *mockNpc) SetHuntMode(mode int) {
	m.setHuntModeCalls = append(m.setHuntModeCalls, mode)
}
```

In `pkg/script/handlers_player_test.go`, locate `mockActiveNpc`'s existing no-op stubs. Add two no-op stubs for the new interface methods:

```go
func (m *mockActiveNpc) SetHuntRange(_ int) {}
func (m *mockActiveNpc) SetHuntMode(_ int)  {}
```

- [ ] **Step 9: Run tests to verify they pass + full suites**

Run:
```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestNewNpcSeedsHuntFromType|TestNpcSetHuntRangeAndMode|TestNpcRevertTypeResetsHuntFields' -v
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -count=1
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -count=1
```

Expected: clean build; all 3 new tests PASS; all prior tests still PASS.

- [ ] **Step 10: Commit**

```bash
git add modules/world/npc.go pkg/script/active.go pkg/script/handlers_npc_test.go pkg/script/handlers_player_test.go modules/world/npc_event_queue_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world,script): NAI-7 Npc hunt fields + Set methods + ActiveNpc extension

Add huntMode, huntRange, huntClock, huntTarget fields to *Npc in a
new // === hunt === block. Seed huntMode + huntRange from NpcType in
NewNpc. Extend NAI-5's revertType with hunt-field resets (TS
Npc.ts:309-312) — this discharges the NAI-5-deferred hunt-reset work.

Add SetHuntRange(r) and SetHuntMode(mode) methods on *Npc. Unlike
SetTimer's -1-is-noop semantic, SetHuntMode accepts -1 as a valid
clear command (matches TS NpcOps.ts:178-185).

Extend ActiveNpc interface with the two methods. Update mockNpc
(recording) and mockActiveNpc (no-op) to satisfy the extended
interface.

No consumer yet — Task 2 wires the opcode handlers; Task 3 wires
processNpcHunt into the tick loop.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: Opcode handlers + registrations + 4 handler tests

**Files:**
- Modify: `pkg/script/handlers_npc.go` (add `handleNpcSetHunt`, `handleNpcSetHuntMode`)
- Modify: `pkg/script/handlers.go` (register both)
- Modify: `pkg/script/handlers_npc_test.go` (add 4 tests)

- [ ] **Step 1: Write the failing tests**

Append to `pkg/script/handlers_npc_test.go`:

```go
// TestHandleNpcSetHunt — NPC_SETHUNT pops range and calls
// ActiveNpc.SetHuntRange. Mirrors TS NpcOps.ts:174-176.
func TestHandleNpcSetHunt(t *testing.T) {
	npc := &mockNpc{}
	sf := &ScriptFile{
		Name: "test_npc_sethunt",
		Opcodes: []Opcode{
			OpPushConstantInt, // push range (15)
			OpNpcSetHunt,
			OpReturn,
		},
		IntOperands: []int32{15, 0, 0},
	}
	state := Init(sf, nil, false, nil, nil)
	state.ActiveNpc = npc
	state.Pointers |= PtrActiveNpc

	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if len(npc.setHuntRangeCalls) != 1 {
		t.Fatalf("setHuntRangeCalls: got %d, want 1", len(npc.setHuntRangeCalls))
	}
	if npc.setHuntRangeCalls[0] != 15 {
		t.Errorf("setHuntRangeCalls[0]: got %d, want 15", npc.setHuntRangeCalls[0])
	}
}

// TestHandleNpcSetHuntWithoutActiveNpcErrors — defensive nil check.
func TestHandleNpcSetHuntWithoutActiveNpcErrors(t *testing.T) {
	sf := &ScriptFile{
		Name: "npc_sethunt_no_npc",
		Opcodes: []Opcode{
			OpPushConstantInt, OpNpcSetHunt, OpReturn,
		},
		IntOperands: []int32{5, 0, 0},
	}
	state := Init(sf, nil, false, nil, nil)
	// state.ActiveNpc intentionally nil.

	err := Execute(state)
	if err == nil {
		t.Fatalf("Execute: want error, got nil")
	}
	want := "NPC_SETHUNT: no active npc"
	if got := err.Error(); got != want {
		t.Errorf("error: got %q, want %q", got, want)
	}
}

// TestHandleNpcSetHuntMode — NPC_SETHUNTMODE with both positive and
// -1 (clear) values. Mirrors TS NpcOps.ts:178-185.
func TestHandleNpcSetHuntMode(t *testing.T) {
	npc := &mockNpc{}
	sf := &ScriptFile{
		Name: "test_npc_sethuntmode",
		Opcodes: []Opcode{
			OpPushConstantInt, // push mode (3)
			OpNpcSetHuntMode,
			OpPushConstantInt, // push mode (-1, clear)
			OpNpcSetHuntMode,
			OpReturn,
		},
		IntOperands: []int32{3, 0, -1, 0, 0},
	}
	state := Init(sf, nil, false, nil, nil)
	state.ActiveNpc = npc
	state.Pointers |= PtrActiveNpc

	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if len(npc.setHuntModeCalls) != 2 {
		t.Fatalf("setHuntModeCalls: got %d, want 2", len(npc.setHuntModeCalls))
	}
	if npc.setHuntModeCalls[0] != 3 {
		t.Errorf("setHuntModeCalls[0]: got %d, want 3", npc.setHuntModeCalls[0])
	}
	if npc.setHuntModeCalls[1] != -1 {
		t.Errorf("setHuntModeCalls[1]: got %d, want -1 (clear)", npc.setHuntModeCalls[1])
	}
}

// TestHandleNpcSetHuntModeWithoutActiveNpcErrors — defensive nil check.
func TestHandleNpcSetHuntModeWithoutActiveNpcErrors(t *testing.T) {
	sf := &ScriptFile{
		Name: "npc_sethuntmode_no_npc",
		Opcodes: []Opcode{
			OpPushConstantInt, OpNpcSetHuntMode, OpReturn,
		},
		IntOperands: []int32{3, 0, 0},
	}
	state := Init(sf, nil, false, nil, nil)

	err := Execute(state)
	if err == nil {
		t.Fatalf("Execute: want error, got nil")
	}
	want := "NPC_SETHUNTMODE: no active npc"
	if got := err.Error(); got != want {
		t.Errorf("error: got %q, want %q", got, want)
	}
}
```

**Note on Pointers:** per NAI-2's `PtrActiveNpc` precedent, the mockNpc
path sets `state.Pointers |= PtrActiveNpc` — `requireActiveNpc` checks
`state.ActiveNpc != nil`, not the pointer bit, so this is precautionary
consistency with the rest of the test file.

- [ ] **Step 2: Run tests to verify they fail**

Run:
```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run 'TestHandleNpcSetHunt(Mode)?' -v
```

Expected: tests FAIL because `OpNpcSetHunt` / `OpNpcSetHuntMode` have no registered handlers. Error shape: `script "...": no handler for NPC_SETHUNT (opcode 2533) at pc=1` or similar.

- [ ] **Step 3: Add the two handlers**

In `pkg/script/handlers_npc.go`, append (after `handleNpcSetTimer` from NAI-4):

```go
// handleNpcSetHunt (NPC_SETHUNT, opcode 2533) sets the NPC's hunt
// search range. Despite the opcode name, this sets RANGE only —
// hunt mode is set via the separate NPC_SETHUNTMODE opcode.
// Mirrors TS NpcOps.ts:174-176.
func handleNpcSetHunt(s *ScriptState) error {
	if err := requireActiveNpc(s, "NPC_SETHUNT"); err != nil {
		return err
	}
	s.ActiveNpc.SetHuntRange(s.PopInt())
	return nil
}

// handleNpcSetHuntMode (NPC_SETHUNTMODE, opcode 2534) sets the NPC's
// HuntType id. -1 clears the hunt mode (valid input). Mirrors TS
// NpcOps.ts:178-185.
func handleNpcSetHuntMode(s *ScriptState) error {
	if err := requireActiveNpc(s, "NPC_SETHUNTMODE"); err != nil {
		return err
	}
	s.ActiveNpc.SetHuntMode(s.PopInt())
	return nil
}
```

- [ ] **Step 4: Register both opcodes**

In `pkg/script/handlers.go`, locate the NPC-mutating-ops block. Add both registrations alphabetically:

```go
	OpNpcSetHunt:     handleNpcSetHunt,
	OpNpcSetHuntMode: handleNpcSetHuntMode,
	OpNpcSetTimer:    handleNpcSetTimer,
```

(Place between `OpNpcQueue` and `OpNpcSetTimer` if those are adjacent; gofmt realigns.)

- [ ] **Step 5: Run tests to verify they pass + full package**

Run:
```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run 'TestHandleNpcSetHunt(Mode)?' -v
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -count=1
```

Expected: all 4 new tests PASS; no regressions.

- [ ] **Step 6: Commit**

```bash
git add pkg/script/handlers_npc.go pkg/script/handlers.go pkg/script/handlers_npc_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): NAI-7 handleNpcSetHunt + handleNpcSetHuntMode opcodes

Pop-and-delegate handlers for NPC_SETHUNT (2533; sets range only per
TS quirk) and NPC_SETHUNTMODE (2534; -1 clears). Both gated by
requireActiveNpc. Mirrors TS NpcOps.ts:174-185.

Four tests: happy path + defensive-nil for each. SetHuntMode test
exercises both positive (3) and clear (-1) values in sequence.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: processNpcHunt + huntAll + variant stubs + turn() wire + 3 integration tests — closes NAI-7

**Files:**
- Create: `modules/world/npc_hunt.go`
- Modify: `modules/world/npc_ai.go` (wire call in turn())
- Modify: `modules/world/npc_event_queue_test.go` (add 3 integration tests)

- [ ] **Step 1: Write the failing tests**

Append to `modules/world/npc_event_queue_test.go`:

```go
func TestProcessNpcHuntSkipsWhenHuntModeNegative(t *testing.T) {
	s := newServerForScriptTest(t)
	s.currentTick = 100
	n := newNpcForLifecycleTest(t)
	n.server = s
	n.huntMode = -1
	n.huntClock = 0

	s.processNpcHunt(n)

	if n.huntClock != 0 {
		t.Errorf("huntClock: got %d, want 0 (no-op when huntMode=-1)", n.huntClock)
	}
}

func TestProcessNpcHuntIncrementsClockWhenHuntModeValid(t *testing.T) {
	s := newServerForScriptTest(t)
	s.currentTick = 100
	// Seed a HuntTypeConfigs with index 0 being a "always-gate-open"
	// HuntType. Type=Off means huntAll short-circuits at the
	// `HuntModeOff || huntRange < 1` check; NobodyNear=KeepHunting
	// means the observer gate passes. Net effect: gate passes, clock
	// increments, huntAll is a no-op.
	s.huntTypes = &objtype.HuntTypeConfigs{
		Configs: []*objtype.HuntType{
			{
				Type:       objtype.HuntModeOff,
				NobodyNear: objtype.HuntNobodyNearKeepHunting,
				Rate:       1,
			},
		},
	}
	n := newNpcForLifecycleTest(t)
	n.server = s
	n.huntMode = 0
	n.huntClock = 0

	s.processNpcHunt(n)

	if n.huntClock != 1 {
		t.Errorf("huntClock: got %d, want 1 (gate passes, clock increments)", n.huntClock)
	}
}

// TestProcessNpcHuntPauseHuntRunsWithObserverStub — regression guard
// for the observer stub. Currently `observers := 1` means PAUSEHUNT
// gate passes even without real observers. When real observer
// tracking lands, this test's expected value changes to 0 and its
// assertion reverses.
func TestProcessNpcHuntPauseHuntRunsWithObserverStub(t *testing.T) {
	s := newServerForScriptTest(t)
	s.currentTick = 100
	s.huntTypes = &objtype.HuntTypeConfigs{
		Configs: []*objtype.HuntType{
			{
				Type:       objtype.HuntModeNpc,
				NobodyNear: objtype.HuntNobodyNearPauseHunt,
				Rate:       1,
			},
		},
	}
	n := newNpcForLifecycleTest(t)
	n.server = s
	n.huntMode = 0
	n.huntClock = 0

	s.processNpcHunt(n)

	if n.huntClock != 1 {
		t.Errorf("huntClock: got %d, want 1 (observer stub = 1 means PAUSEHUNT gate passes)", n.huntClock)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run:
```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestProcessNpcHunt -v
```

Expected: compile error — `s.processNpcHunt undefined`.

- [ ] **Step 3: Create `modules/world/npc_hunt.go`**

Create the file with full content:

```go
package world

import (
	"math/rand/v2"

	"github.com/zsrv/goscape/pkg/objtype"
)

// processNpcHunt runs the per-tick hunt pass. Matches TS
// Npc.ts:158-171.
//
// Observer gate: TS checks rsbuf.getNpcObservers(this.nid); Go
// has no equivalent observer-count API yet, so we inline
// `observers := 1` (always observed). PAUSEHUNT semantics are
// currently unobservable — tracked as follow-up in nai_followups
// memory. TS NobodyNear values:
//   - HuntNobodyNearKeepHunting: gate always passes
//   - HuntNobodyNearPauseHunt: gate passes iff observers > 0 OR
//     type == HuntModePlayer
//
// Note on mode bounds: SetHuntMode accepts any int (including
// out-of-range) to match TS. processNpcHunt validates bounds
// against s.huntTypes.Configs and silently no-ops on invalid.
func (s *Server) processNpcHunt(n *Npc) {
	if n.huntMode == -1 {
		return
	}
	if s.huntTypes == nil ||
		n.huntMode < 0 ||
		n.huntMode >= len(s.huntTypes.Configs) {
		return
	}
	hunt := s.huntTypes.Configs[n.huntMode]
	if hunt == nil {
		return
	}
	observers := 1 // TODO: rsbuf.GetNpcObservers(n.nid) when available
	if hunt.NobodyNear == objtype.HuntNobodyNearPauseHunt &&
		observers <= 0 &&
		hunt.Type != objtype.HuntModePlayer {
		return
	}
	// Player-type hunts skip the huntAll dispatcher at the turn()
	// level — matches TS Npc.ts:164 "hunt && hunt.type !== HuntModeType.PLAYER".
	// The HuntModePlayer branch in huntAll is reachable only via
	// explicit scripted calls, not the turn() path.
	if hunt.Type != objtype.HuntModePlayer {
		n.huntAll(s, hunt)
	}
	n.huntClock++
}

// huntAll dispatches to a hunted-type variant and sets huntTarget.
// Matches TS Npc.ts:249-277. Variants are stubs at NAI-7; NAI-8
// fills huntPlayers; NAI-9 fills huntNpcs/huntObjs/huntLocs.
func (n *Npc) huntAll(s *Server, hunt *objtype.HuntType) {
	n.huntTarget = nil
	if n.huntClock < hunt.Rate-1 {
		return
	}
	if hunt.Type == objtype.HuntModeOff || n.huntRange < 1 {
		return
	}
	var hunted []entity
	switch hunt.Type {
	case objtype.HuntModePlayer:
		hunted = n.huntPlayers(s, hunt)
	case objtype.HuntModeNpc:
		hunted = n.huntNpcs(s, hunt)
	case objtype.HuntModeObj:
		hunted = n.huntObjs(s, hunt)
	case objtype.HuntModeScenery:
		hunted = n.huntLocs(s, hunt)
	}
	if len(hunted) > 0 {
		n.huntTarget = hunted[rand.IntN(len(hunted))]
	}
}

// huntPlayers is stubbed at NAI-7; NAI-8 fills the body.
func (n *Npc) huntPlayers(s *Server, hunt *objtype.HuntType) []entity { return nil }

// huntNpcs is stubbed at NAI-7; NAI-9 fills the body.
func (n *Npc) huntNpcs(s *Server, hunt *objtype.HuntType) []entity { return nil }

// huntObjs is stubbed at NAI-7; NAI-9 fills the body.
func (n *Npc) huntObjs(s *Server, hunt *objtype.HuntType) []entity { return nil }

// huntLocs is stubbed at NAI-7; NAI-9 fills the body.
func (n *Npc) huntLocs(s *Server, hunt *objtype.HuntType) []entity { return nil }
```

- [ ] **Step 4: Wire the call in `Npc.turn()`**

In `modules/world/npc_ai.go`, locate the existing post-isValid-gate section (after NAI-6):

```go
	// === isValid gate (NAI-5) ===
	if n.dead || n.delayed {
		return
	}

	// === Regen + timer + queue (NAI-6, NAI-4, NAI-3) ===
	s.processNpcRegen(n) // NAI-6 — matches TS Npc.ts:176
	s.processNpcTimer(n)
	s.processNpcQueue(n)
```

Insert `s.processNpcHunt(n)` immediately after the isValid gate, BEFORE `processNpcRegen`:

```go
	// === isValid gate (NAI-5) ===
	if n.dead || n.delayed {
		return
	}

	// === Hunt + regen + timer + queue (NAI-7, NAI-6, NAI-4, NAI-3) ===
	s.processNpcHunt(n)  // NAI-7 — matches TS Npc.ts:158-171
	s.processNpcRegen(n) // NAI-6 — matches TS Npc.ts:176
	s.processNpcTimer(n)
	s.processNpcQueue(n)
```

- [ ] **Step 5: Run tests to verify they pass + full suite + race**

Run:
```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestProcessNpcHunt' -v -count=1
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -count=1
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./...
```

Expected: all 3 new tests PASS; full world suite PASS; race suite clean. Existing `TestKillSets*`, `TestTeleportHome*`, `TestRespawnAfter*`, `TestNpcTurn*`, `TestProcessNpc*` all still green.

- [ ] **Step 6: Grep sanity check**

Run:
```
rg -n '\bhuntMode\b|\bhuntRange\b|\bhuntClock\b|\bhuntTarget\b|\bprocessNpcHunt\b|\bhuntAll\b|\bhuntPlayers\b|\bhuntNpcs\b|\bhuntObjs\b|\bhuntLocs\b|\bSetHuntRange\b|\bSetHuntMode\b' modules/ pkg/
```

Expected: matches appear in:
- `modules/world/npc.go` (fields + seeds + methods + revertType)
- `modules/world/npc_hunt.go` (processNpcHunt + huntAll + 4 stubs)
- `modules/world/npc_ai.go` (call site)
- `modules/world/npc_event_queue_test.go` (6 tests)
- `pkg/script/active.go` (interface methods)
- `pkg/script/handlers_npc.go` (2 handlers)
- `pkg/script/handlers_npc_test.go` (mock + 4 tests)
- `pkg/script/handlers_player_test.go` (mock stubs)

Report match count. No stray references expected.

- [ ] **Step 7: Commit, closing NAI-7**

```bash
git add modules/world/npc_hunt.go modules/world/npc_ai.go modules/world/npc_event_queue_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-7 processNpcHunt + huntAll + variant stubs — closes NAI-7

Add modules/world/npc_hunt.go: processNpcHunt runs per-tick hunt pass
(matches TS Npc.ts:158-171); huntAll dispatcher (TS Npc.ts:249-277)
selects a random target from the winning variant; four variant stubs
(huntPlayers/huntNpcs/huntObjs/huntLocs) return nil — filled by NAI-8
(huntPlayers) and NAI-9 (huntNpcs/Objs/Locs).

Wire processNpcHunt inside the post-isValid section of Npc.turn(),
BEFORE processNpcRegen — matches TS hunt→consumeHuntTarget→regen→
timer→queue order (consumeHuntTarget is NAI-10 scope).

Observer gate stubbed to `observers := 1` with inline TODO for
rsbuf.GetNpcObservers when available. PAUSEHUNT semantics
unobservable until real observer tracking lands; tracked in
nai_followups memory.

Three integration tests cover: huntMode=-1 no-op, valid-mode gate+
clock increment, PAUSEHUNT observer-stub regression guard (test value
changes to 0 when real observer tracking replaces the stub).

Closes NAI-7 — hunt core skeleton ready for NAI-8/9 to fill variants.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Self-review checklist results

**1. Spec coverage:**

| Spec section | Task |
|---|---|
| 4 hunt fields on `*Npc` | Task 1 |
| NewNpc seeds huntMode + huntRange | Task 1 |
| revertType extended with hunt resets | Task 1 |
| SetHuntRange + SetHuntMode methods | Task 1 |
| ActiveNpc interface extension | Task 1 |
| mockNpc recording stubs + mockActiveNpc no-op stubs | Task 1 |
| handleNpcSetHunt + handleNpcSetHuntMode | Task 2 |
| OpNpcSetHunt + OpNpcSetHuntMode registrations | Task 2 |
| processNpcHunt helper | Task 3 |
| huntAll dispatcher | Task 3 |
| 4 variant stubs | Task 3 |
| turn() wire | Task 3 |
| Test 1 `TestNewNpcSeedsHuntFromType` | Task 1 |
| Test 2 `TestNpcSetHuntRangeAndMode` | Task 1 |
| Test 4 `TestNpcRevertTypeResetsHuntFields` | Task 1 |
| Test 5 `TestHandleNpcSetHunt` | Task 2 |
| Test 6 `TestHandleNpcSetHuntWithoutActiveNpcErrors` | Task 2 |
| Test 7 `TestHandleNpcSetHuntMode` | Task 2 |
| Test 8 `TestHandleNpcSetHuntModeWithoutActiveNpcErrors` | Task 2 |
| Test 9 `TestProcessNpcHuntSkipsWhenHuntModeNegative` | Task 3 |
| Test 10 `TestProcessNpcHuntIncrementsClockWhenHuntModeValid` | Task 3 |
| Test 11 `TestProcessNpcHuntPauseHuntRunsWithObserverStub` | Task 3 |

All 10 spec-listed tests have tasks. (Spec originally listed test 3 `TestNpcSetHuntMode` as separate from test 2 `TestNpcSetHuntRange`; plan bundles them into `TestNpcSetHuntRangeAndMode` since they're trivial direct-method asserts — saves one test function, same coverage.)

**2. Placeholder scan:** No TBDs/TODOs in plan text. Inline TODO in `processNpcHunt` code comment references `rsbuf.GetNpcObservers` — that's a legitimate code-level deferred pointer, not a plan failure.

**3. Type consistency:** `huntMode int`, `huntRange int`, `huntClock int`, `huntTarget entity` consistent across Task 1 field declarations, Task 1 NewNpc seed, Task 1 revertType, Task 3 processNpcHunt. `SetHuntRange(r int)` / `SetHuntMode(mode int)` consistent signatures across `*Npc` method, interface declaration, mock stubs, handler call sites. `HuntModeOff`/`HuntModeNpc`/`HuntModeObj`/`HuntModeScenery`/`HuntModePlayer` + `HuntNobodyNearKeepHunting`/`HuntNobodyNearPauseHunt` all from NAI-1's `pkg/objtype` package.

---

## Commit trail (for reference)

Three commits close NAI-7:

1. `feat(world,script): NAI-7 Npc hunt fields + Set methods + ActiveNpc extension`
2. `feat(script): NAI-7 handleNpcSetHunt + handleNpcSetHuntMode opcodes`
3. `feat(world): NAI-7 processNpcHunt + huntAll + variant stubs — closes NAI-7`

Each commit leaves the tree green; the final one closes NAI-7 and unblocks NAI-8/9/10.
