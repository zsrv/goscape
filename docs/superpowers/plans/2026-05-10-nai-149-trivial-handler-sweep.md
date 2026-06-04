# NAI-149 — Trivial-handler sweep (8 ops) — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Port 8 trivial RuneScript opcode handlers — PLAYERMEMBER, AFK_EVENT, WEIGHT, HEAL_ENERGY, SEQ_LENGTH, INV_STOCKBASE, INV_DEBUG_NAME, SETSKINCOLOUR — closing 3 of 4 user-reported `no handler for X` WARN classes from 2026-05-09/10 smoke logs and the 5 sibling trivial siblings that share the cohort shape. PROJANIM_NPC deferred to NAI-150.

**Architecture:** All handlers live in `pkg/script/handlers_<domain>.go` (player/inv/server) and dispatch via the existing `var handlers = map[Opcode]Handler{}` registry in `pkg/script/handlers.go`. Five new pass-through accessors land on `pkg/script.ActivePlayer` (`Members`, `RunWeight`, `AfkEventReady` + `SetAfkEventReady`, `SetRunEnergy`) with field-read shims in `modules/world/player.go`. One new check helper (`checkSeqType`) joins the existing family in `pkg/script/handlers_player.go`. The `mockPlayer` and `mockConfigs` test fixtures gain matching backing fields.

**Tech Stack:** Go 1.26+ (`go_version.md`). No new dependencies.

**Spec:** `docs/superpowers/specs/2026-05-10-nai-149-trivial-handler-sweep-design.md` (commit `4023236`).

**Cadence:** 100-300 LOC band per `runescript_cadence.md` — separate spec + plan docs, single combined Sonnet reviewer at end-of-impl. Subagent-driven-development per `execution_mode_default.md`. Per-task two-stage review (rubric-light: implementer commit + controller pre-flight, defer Sonnet review to end).

---

## File Structure

| File | Action | Responsibility |
|---|---|---|
| `pkg/script/active.go` | Modify | Append 5 method signatures to `ActivePlayer` interface (Members, RunWeight, AfkEventReady, SetAfkEventReady, SetRunEnergy). |
| `modules/world/player.go` | Modify | Add 5 method bodies (pass-through to existing `members`, `runweight`, `afkEventReady`, `runenergy` fields). |
| `pkg/script/runner_test.go` | Modify | Extend `mockPlayer` with `membersValue`, `runweightValue`, `afkEventReadyValue`, `setAfkEventReadyCalls []bool` + 5 method impls. |
| `pkg/script/handlers_config_test.go` | Modify | Extend `mockConfigs` with `seqs map[int]*objtype.SeqType` + update `SeqType()` method to return from map. |
| `pkg/script/handlers_player.go` | Modify | Add `handlePlayerMember`, `handleAfkEvent`, `handleWeight`, `handleHealEnergy`, `handleSetSkinColour`. |
| `pkg/script/handlers_player_test.go` | Modify | Add tests for the 5 player handlers above (including TS-asymmetry dual-pins). |
| `pkg/script/handlers_inv.go` | Modify | Add `handleInvStockBase`, `handleInvDebugName`. |
| `pkg/script/handlers_inv_test.go` | Modify | Add tests for the 2 inv handlers (3 branches for STOCKBASE). |
| `pkg/script/handlers_server.go` | Modify | Add `handleSeqLength` + new `checkSeqType` helper (or place helper in handlers_player.go beside `checkInvType`/`checkObjType` — see Task 6). |
| `pkg/script/handlers_server_test.go` | Modify (or Create) | Add tests for SEQ_LENGTH (success + invalid-id error). |
| `pkg/script/handlers.go` | Modify | Add 8 entries to `var handlers = map[Opcode]Handler{}` registry. |

Total estimated delta: ~340 LOC production + ~520 LOC test.

---

## Pre-flight verification (controller, not implementer)

Before dispatching T1, the controller (parent) verifies these premises against HEAD `4023236` (per `controller_preflight.md`):

- `pkg/script/active.go:415-420` declares `StaffModLevel() int32` (cohort baseline for new methods).
- `pkg/script/active.go` does NOT declare any of: `Members()`, `RunWeight()`, `AfkEventReady()`, `SetAfkEventReady`, `SetRunEnergy`.
- `modules/world/player.go:88` declares `members bool`, `:233` declares `runweight int`, `:297` declares `afkEventReady bool`, `:232` declares `runenergy int`.
- `pkg/script/handlers_player.go:35` declares `requireActivePlayer`, `:57` declares `requireProtectedActivePlayer`, `:84` declares `checkNotNull`, `:142` declares `checkInvType`. (Spec R3 verified: `checkObjType` exists at `pkg/script/handlers_obj.go:21`.)
- `pkg/objtype/configtype.go:13` declares `ConfigType.ID int` (R4 verified: `ObjType.ID` is via embedding).
- `pkg/objtype/invtype.go:23` declares `StockObj []uint16`, `:24` declares `StockCount []uint16`, and `InvType` has `DebugName` (verified at `:62`).
- `pkg/objtype/seqtype.go:27` declares `SeqType.Duration int`.
- `pkg/script/runner_test.go:99` declares `type mockPlayer struct`; field `runenergyValue int` exists at `:169`, `staffModLevelValue int` at `:264`, `colorParts [5]int` at `:340`. Field `lowMemoryValue bool` exists (LowMemory pattern reference).
- `pkg/script/handlers_config_test.go:11` declares `type mockConfigs struct` with no `seqs` map and `SeqType` returning hardcoded nil at `:34`.
- `pkg/script/opcode.go` declares `OpAfkEvent=2000`, `OpPlayerMember=2090`, `OpInvStockBase=4325`. (Verified at brainstorm.)
- All 8 target opcodes (`OpPlayerMember`, `OpAfkEvent`, `OpWeight`, `OpHealEnergy`, `OpSeqLength`, `OpInvStockBase`, `OpInvDebugName`, `OpSetSkinColour`) appear in `pkg/script/opcode.go` String() table AND in the unhandled list from the audit one-liner.

If any premise has shifted, re-evaluate before dispatch.

---

## Task 1 — Foundation: 5 ActivePlayer methods + mock wiring

**Files:**
- Modify: `pkg/script/active.go`
- Modify: `modules/world/player.go`
- Modify: `pkg/script/runner_test.go`

- [ ] **Step 1.1: Add 5 method signatures to the `ActivePlayer` interface**

In `pkg/script/active.go`, locate the existing `StaffModLevel() int32` declaration (around line 420) and add the new methods adjacent to related neighbors. Use this exact placement: append `Members() bool` next to `Gender() int` (line 597); append `RunWeight() int`, `AfkEventReady() bool`, `SetAfkEventReady(v bool)` adjacent to `RunEnergy()` (line 181); add `SetRunEnergy(v int)` directly below `RunEnergy() int`. Match the existing two-line doc-comment style:

```go
// Members returns whether the player has a members account. Backed by
// the per-player members field set from the login RPC. Mirrors TS
// Player.members consumed by PlayerOps.ts:1212 PLAYERMEMBER.
Members() bool
```

```go
// RunWeight returns the player's tracked carry weight in grams (TS
// stores 1/1000 of a kg in `runweight`; production wiring at
// modules/world/player.go:880 mirrors TS update site). Consumed by
// PlayerOps.ts:1181 WEIGHT.
RunWeight() int
```

```go
// AfkEventReady returns the per-player AFK-event ready flag. Set true
// by the random tick gate at modules/world/player.go:1050 (rand <
// 0.0167 every 500 ticks). Consumed by PlayerOps.ts:1058 AFK_EVENT.
AfkEventReady() bool

// SetAfkEventReady writes the AFK-event ready flag. AFK_EVENT clears
// it to false after dispatching (TS PlayerOps.ts:1060).
SetAfkEventReady(v bool)
```

```go
// SetRunEnergy writes the player's current run-energy value (range
// [0, 10000]). Caller is responsible for clamping; HEAL_ENERGY clamps
// in the handler before calling this. Mirrors TS PlayerOps.ts:1054.
SetRunEnergy(v int)
```

- [ ] **Step 1.2: Run the world build to verify the interface compiles even without impls**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./pkg/script/...`
Expected: PASS (interface change is non-breaking until something type-asserts modules/world.Player against ActivePlayer).

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./modules/world/...`
Expected: FAIL with errors like "*Player does not implement script.ActivePlayer (missing Members method)" — this proves the interface is being implemented somewhere and we have a real binding to satisfy.

- [ ] **Step 1.3: Add 5 method bodies to `modules/world/player.go`**

Locate the existing `StaffModLevel()` method on `*Player` (search for `func (p *Player) StaffModLevel()`) and group these new methods adjacent to it (or per the existing local convention — group field-read accessors). Bodies are pure pass-throughs:

```go
// Members reports the per-player members flag (login RPC field).
func (p *Player) Members() bool { return p.members }

// RunWeight returns the cached carry weight in grams.
func (p *Player) RunWeight() int { return p.runweight }

// AfkEventReady reports the AFK-event ready flag.
func (p *Player) AfkEventReady() bool { return p.afkEventReady }

// SetAfkEventReady writes the AFK-event ready flag (cleared by AFK_EVENT
// after dispatch).
func (p *Player) SetAfkEventReady(v bool) { p.afkEventReady = v }

// SetRunEnergy writes the current run-energy value. Caller clamps; this
// method does no validation.
func (p *Player) SetRunEnergy(v int) { p.runenergy = v }
```

- [ ] **Step 1.4: Verify the world build now passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./modules/world/...`
Expected: PASS.

- [ ] **Step 1.5: Extend `mockPlayer` in `pkg/script/runner_test.go`**

Append fields to the `type mockPlayer struct` declaration. Locate `runenergyValue int` (around line 169) and group the new fields nearby for diff-readability:

```go
// NAI-149: trivial-handler-sweep cohort backing fields.
membersValue        bool
runweightValue      int
afkEventReadyValue  bool
setAfkEventReadyCalls []bool // captures every SetAfkEventReady arg in order
```

Then append five method impls. Locate the existing `RunEnergy()` impl (line 512) and place them grouped:

```go
// NAI-149.
func (m *mockPlayer) Members() bool       { return m.membersValue }
func (m *mockPlayer) RunWeight() int      { return m.runweightValue }
func (m *mockPlayer) AfkEventReady() bool { return m.afkEventReadyValue }
func (m *mockPlayer) SetAfkEventReady(v bool) {
    m.setAfkEventReadyCalls = append(m.setAfkEventReadyCalls, v)
    m.afkEventReadyValue = v
}
func (m *mockPlayer) SetRunEnergy(v int) { m.runenergyValue = v }
```

- [ ] **Step 1.6: Run pkg/script tests to ensure mock still compiles and existing tests pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/...`
Expected: PASS (mock changes are additive).

- [ ] **Step 1.7: Commit**

```bash
git add pkg/script/active.go modules/world/player.go pkg/script/runner_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): NAI-149 T1 — 5 ActivePlayer accessors for trivial-handler cohort

Adds Members, RunWeight, AfkEventReady + SetAfkEventReady, SetRunEnergy
to pkg/script.ActivePlayer with pass-through impls in modules/world.Player.
Extends pkg/script test-mock with backing fields. No handler wiring yet
— foundation for T2-T9.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2 — `handlePlayerMember` (OpPlayerMember = 2090)

**Files:**
- Modify: `pkg/script/handlers_player.go`
- Modify: `pkg/script/handlers_player_test.go`
- Modify: `pkg/script/handlers.go`

- [ ] **Step 2.1: Write the failing test**

Append to `pkg/script/handlers_player_test.go`:

```go
// TestHandlePlayerMember_PushesMembersFlag pins TS PlayerOps.ts:1211-1213
// (NAI-149). Both branches: members=true → 1, members=false → 0.
func TestHandlePlayerMember_PushesMembersFlag(t *testing.T) {
    cases := []struct {
        name string
        seed bool
        want int
    }{
        {"members=true pushes 1", true, 1},
        {"members=false pushes 0", false, 0},
    }
    for _, tc := range cases {
        t.Run(tc.name, func(t *testing.T) {
            mp := &mockPlayer{membersValue: tc.seed}
            s := &ScriptState{
                IntStack:    make([]int, StackCapacity),
                StringStack: make([]string, StackCapacity),
                Self:        mp,
                Pointers:    PtrActivePlayer,
            }
            if err := handlePlayerMember(s); err != nil {
                t.Fatalf("handlePlayerMember: %v", err)
            }
            if s.SSP != 1 {
                t.Fatalf("SSP: got %d, want 1", s.SSP)
            }
            if got := s.IntStack[0]; got != tc.want {
                t.Errorf("top of int stack: got %d, want %d", got, tc.want)
            }
        })
    }
}

// TestHandlePlayerMember_RequiresActivePlayer pins the ActivePlayer guard
// (TS checkedHandler(ActivePlayer, ...)).
func TestHandlePlayerMember_RequiresActivePlayer(t *testing.T) {
    s := &ScriptState{
        IntStack:    make([]int, StackCapacity),
        StringStack: make([]string, StackCapacity),
    }
    err := handlePlayerMember(s)
    if err == nil {
        t.Fatalf("handlePlayerMember: expected error, got nil")
    }
    if !strings.Contains(err.Error(), "PLAYERMEMBER") {
        t.Errorf("error: got %q, want to contain \"PLAYERMEMBER\"", err.Error())
    }
}
```

(If `strings` import is missing in the test file, add it.)

- [ ] **Step 2.2: Run the failing test**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestHandlePlayerMember`
Expected: FAIL with "undefined: handlePlayerMember".

- [ ] **Step 2.3: Implement `handlePlayerMember`**

Append to `pkg/script/handlers_player.go`:

```go
// handlePlayerMember (PLAYERMEMBER, opcode 2090) pushes 1 if the active
// player has a members account, else 0. Mirrors TS
// LostCityRS/Engine-TS/.../PlayerOps.ts:1211-1213 — checkedHandler(ActivePlayer).
func handlePlayerMember(s *ScriptState) error {
    if err := requireActivePlayer(s, "PLAYERMEMBER"); err != nil {
        return err
    }
    if s.Self.Members() {
        s.PushInt(1)
    } else {
        s.PushInt(0)
    }
    return nil
}
```

- [ ] **Step 2.4: Register in handlers.go**

In `pkg/script/handlers.go`, locate the `var handlers = map[Opcode]Handler{}` block and add (group under any existing `OpPlayer*` entries to match style):

```go
OpPlayerMember: handlePlayerMember,
```

- [ ] **Step 2.5: Run the test**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestHandlePlayerMember`
Expected: PASS.

- [ ] **Step 2.6: Commit**

```bash
git add pkg/script/handlers_player.go pkg/script/handlers_player_test.go pkg/script/handlers.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): NAI-149 T2 — port PLAYERMEMBER (opcode 2090)

Pushes 1 if Self.Members() else 0 under ActivePlayer guard. Mirrors TS
PlayerOps.ts:1211-1213 checkedHandler(ActivePlayer). Closes one of three
"no handler for X" WARN classes from 2026-05-09 user smoke logs.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3 — `handleAfkEvent` (OpAfkEvent = 2000)

**Files:**
- Modify: `pkg/script/handlers_player.go`
- Modify: `pkg/script/handlers_player_test.go`
- Modify: `pkg/script/handlers.go`

- [ ] **Step 3.1: Write the failing tests (4 cases — TS-asymmetry dual-pin per `ts_asymmetry_dual_pin.md`)**

Append to `pkg/script/handlers_player_test.go`:

```go
// TestHandleAfkEvent_NodeDebugForcesEligible pins TS PlayerOps.ts:1058
// — the `Environment.NODE_DEBUG ||` arm short-circuits the staff-mod
// gate. Even with staffModLevel >= 2, NodeDebug=true + ready=true → push 1.
func TestHandleAfkEvent_NodeDebugForcesEligible(t *testing.T) {
    mp := &mockPlayer{
        staffModLevelValue: 5, // would normally suppress
        afkEventReadyValue: true,
    }
    s := &ScriptState{
        IntStack:    make([]int, StackCapacity),
        StringStack: make([]string, StackCapacity),
        Self:        mp,
        Pointers:    PtrActivePlayer,
        NodeDebug:   true,
    }
    if err := handleAfkEvent(s); err != nil {
        t.Fatalf("handleAfkEvent: %v", err)
    }
    if got := s.IntStack[0]; got != 1 {
        t.Errorf("top: got %d, want 1 (NodeDebug forces eligible)", got)
    }
    // Both branches must clear afkEventReady.
    if mp.afkEventReadyValue != false {
        t.Errorf("afkEventReadyValue: got true, want false (always cleared)")
    }
}

// TestHandleAfkEvent_StaffModSuppressesUnderProduction pins TS PlayerOps.ts:1058
// asymmetry — under NodeDebug=false, staffMod>=2 forces 0 even when ready=true.
func TestHandleAfkEvent_StaffModSuppressesUnderProduction(t *testing.T) {
    mp := &mockPlayer{
        staffModLevelValue: 2, // boundary — TS uses `< 2`
        afkEventReadyValue: true,
    }
    s := &ScriptState{
        IntStack:    make([]int, StackCapacity),
        StringStack: make([]string, StackCapacity),
        Self:        mp,
        Pointers:    PtrActivePlayer,
        NodeDebug:   false,
    }
    if err := handleAfkEvent(s); err != nil {
        t.Fatalf("handleAfkEvent: %v", err)
    }
    if got := s.IntStack[0]; got != 0 {
        t.Errorf("top: got %d, want 0 (staffMod=2 suppresses)", got)
    }
    // Asymmetry-pin: clearing happens REGARDLESS of eligibility.
    if mp.afkEventReadyValue != false {
        t.Errorf("afkEventReadyValue: got true, want false (cleared even when not eligible)")
    }
}

// TestHandleAfkEvent_NotReadyPushesZero pins the `&& afkEventReady`
// short-circuit at TS PlayerOps.ts:1058.
func TestHandleAfkEvent_NotReadyPushesZero(t *testing.T) {
    mp := &mockPlayer{
        staffModLevelValue: 0,     // eligible
        afkEventReadyValue: false, // but not ready
    }
    s := &ScriptState{
        IntStack:    make([]int, StackCapacity),
        StringStack: make([]string, StackCapacity),
        Self:        mp,
        Pointers:    PtrActivePlayer,
        NodeDebug:   true,
    }
    if err := handleAfkEvent(s); err != nil {
        t.Fatalf("handleAfkEvent: %v", err)
    }
    if got := s.IntStack[0]; got != 0 {
        t.Errorf("top: got %d, want 0 (not ready)", got)
    }
}

// TestHandleAfkEvent_RequiresActivePlayer pins the goscape-only
// defensive guard (TS skips this check; see defensive_gate_doc_comment_label).
func TestHandleAfkEvent_RequiresActivePlayer(t *testing.T) {
    s := &ScriptState{
        IntStack:    make([]int, StackCapacity),
        StringStack: make([]string, StackCapacity),
    }
    err := handleAfkEvent(s)
    if err == nil {
        t.Fatalf("handleAfkEvent: expected error, got nil")
    }
    if !strings.Contains(err.Error(), "AFK_EVENT") {
        t.Errorf("error: got %q, want to contain \"AFK_EVENT\"", err.Error())
    }
}
```

- [ ] **Step 3.2: Run the failing tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestHandleAfkEvent`
Expected: FAIL with "undefined: handleAfkEvent".

- [ ] **Step 3.3: Implement `handleAfkEvent`**

Append to `pkg/script/handlers_player.go`:

```go
// handleAfkEvent (AFK_EVENT, opcode 2000) pushes 1 when the player is
// eligible to receive an AFK-event prompt and clears the eligibility
// flag. Mirrors TS LostCityRS/Engine-TS/.../PlayerOps.ts:1057-1062:
//
//   state.pushInt(
//     (Environment.NODE_DEBUG || state.activePlayer.staffModLevel < 2)
//       && state.activePlayer.afkEventReady ? 1 : 0
//   );
//   state.activePlayer.afkEventReady = false;
//
// The active-player guard is goscape defensive (TS skips this check;
// see defensive_gate_doc_comment_label).
func handleAfkEvent(s *ScriptState) error {
    if err := requireActivePlayer(s, "AFK_EVENT"); err != nil {
        return err
    }
    eligible := (s.NodeDebug || s.Self.StaffModLevel() < 2) && s.Self.AfkEventReady()
    if eligible {
        s.PushInt(1)
    } else {
        s.PushInt(0)
    }
    s.Self.SetAfkEventReady(false)
    return nil
}
```

- [ ] **Step 3.4: Register in handlers.go**

```go
OpAfkEvent: handleAfkEvent,
```

- [ ] **Step 3.5: Run the tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestHandleAfkEvent`
Expected: PASS (4 sub-tests).

- [ ] **Step 3.6: Commit**

```bash
git add pkg/script/handlers_player.go pkg/script/handlers_player_test.go pkg/script/handlers.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): NAI-149 T3 — port AFK_EVENT (opcode 2000)

(NodeDebug || staffMod<2) && afkEventReady → push; clear ready in both
branches. Dual-pin tests cover NodeDebug-overrides-staffMod asymmetry
and the always-clear write. Defensive ActivePlayer guard labeled per
defensive_gate_doc_comment_label. Closes WARN class from
[label,attempt_pick_pocket] in 2026-05-09 user logs.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 4 — `handleWeight` (OpWeight)

**Files:**
- Modify: `pkg/script/handlers_player.go`
- Modify: `pkg/script/handlers_player_test.go`
- Modify: `pkg/script/handlers.go`

- [ ] **Step 4.1: Write the failing tests (positive + Protected guard pin)**

Append to `pkg/script/handlers_player_test.go`:

```go
// TestHandleWeight_PushesRunWeight pins TS PlayerOps.ts:1180-1182
// (NAI-149). Tests both zero and non-zero weights.
func TestHandleWeight_PushesRunWeight(t *testing.T) {
    cases := []struct {
        name string
        seed int
    }{
        {"zero weight", 0},
        {"non-zero weight (4500g)", 4500},
        {"negative weight (light items)", -1500}, // TS allows negative
    }
    for _, tc := range cases {
        t.Run(tc.name, func(t *testing.T) {
            mp := &mockPlayer{runweightValue: tc.seed}
            s := &ScriptState{
                IntStack:    make([]int, StackCapacity),
                StringStack: make([]string, StackCapacity),
                Self:        mp,
                Pointers:    PtrActivePlayer | PtrProtectedActivePlayer,
            }
            if err := handleWeight(s); err != nil {
                t.Fatalf("handleWeight: %v", err)
            }
            if got := s.IntStack[0]; got != tc.seed {
                t.Errorf("top: got %d, want %d", got, tc.seed)
            }
        })
    }
}

// TestHandleWeight_RequiresProtected pins TS checkedHandler(ProtectedActivePlayer).
func TestHandleWeight_RequiresProtected(t *testing.T) {
    mp := &mockPlayer{runweightValue: 100}
    s := &ScriptState{
        IntStack:    make([]int, StackCapacity),
        StringStack: make([]string, StackCapacity),
        Self:        mp,
        Pointers:    PtrActivePlayer, // active, but NOT protected
    }
    err := handleWeight(s)
    if err == nil {
        t.Fatalf("handleWeight: expected error, got nil")
    }
    if !strings.Contains(err.Error(), "WEIGHT") {
        t.Errorf("error: got %q, want to contain \"WEIGHT\"", err.Error())
    }
    if !strings.Contains(err.Error(), "not protected") {
        t.Errorf("error: got %q, want to contain \"not protected\"", err.Error())
    }
}
```

- [ ] **Step 4.2: Run the failing tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestHandleWeight`
Expected: FAIL with "undefined: handleWeight".

- [ ] **Step 4.3: Implement `handleWeight`**

Append to `pkg/script/handlers_player.go`:

```go
// handleWeight (WEIGHT) pushes the player's tracked carry weight.
// Mirrors TS LostCityRS/Engine-TS/.../PlayerOps.ts:1180-1182 —
// checkedHandler(ProtectedActivePlayer).
func handleWeight(s *ScriptState) error {
    if err := requireProtectedActivePlayer(s, "WEIGHT"); err != nil {
        return err
    }
    s.PushInt(s.Self.RunWeight())
    return nil
}
```

- [ ] **Step 4.4: Register in handlers.go**

```go
OpWeight: handleWeight,
```

- [ ] **Step 4.5: Run the tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestHandleWeight`
Expected: PASS.

- [ ] **Step 4.6: Commit**

```bash
git add pkg/script/handlers_player.go pkg/script/handlers_player_test.go pkg/script/handlers.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): NAI-149 T4 — port WEIGHT under ProtectedActivePlayer

Mirrors TS PlayerOps.ts:1180-1182. Uses requireProtectedActivePlayer
helper; tests pin both happy path (3 weight values incl. negative)
and the "not protected" guard message.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 5 — `handleHealEnergy` (OpHealEnergy)

**Files:**
- Modify: `pkg/script/handlers_player.go`
- Modify: `pkg/script/handlers_player_test.go`
- Modify: `pkg/script/handlers.go`

- [ ] **Step 5.1: Write the failing tests (clamp + NumberNotNull + defensive guard)**

Append to `pkg/script/handlers_player_test.go`:

```go
// TestHandleHealEnergy_AddsAndClamps pins TS PlayerOps.ts:1050-1054 —
// runenergy clamped to [0, 10000] after add. Mirrors TS Math.min/max.
func TestHandleHealEnergy_AddsAndClamps(t *testing.T) {
    cases := []struct {
        name        string
        startEnergy int
        amount      int
        want        int
    }{
        {"normal add", 5000, 1000, 6000},
        {"clamps to 10000 ceiling", 9500, 1000, 10000},
        {"clamps to 0 floor on negative", 200, -500, 0},
        {"max+max stays at 10000", 10000, 10000, 10000},
        {"exact 10000 from below", 9000, 1000, 10000},
    }
    for _, tc := range cases {
        t.Run(tc.name, func(t *testing.T) {
            mp := &mockPlayer{runenergyValue: tc.startEnergy}
            s := &ScriptState{
                IntStack:    make([]int, StackCapacity),
                StringStack: make([]string, StackCapacity),
                Self:        mp,
                Pointers:    PtrActivePlayer,
            }
            s.PushInt(tc.amount)
            if err := handleHealEnergy(s); err != nil {
                t.Fatalf("handleHealEnergy: %v", err)
            }
            if mp.runenergyValue != tc.want {
                t.Errorf("runenergy: got %d, want %d", mp.runenergyValue, tc.want)
            }
        })
    }
}

// TestHandleHealEnergy_RejectsNullAmount pins TS check(amount, NumberNotNull).
// amount=-1 is the script null sentinel.
func TestHandleHealEnergy_RejectsNullAmount(t *testing.T) {
    mp := &mockPlayer{runenergyValue: 5000}
    s := &ScriptState{
        IntStack:    make([]int, StackCapacity),
        StringStack: make([]string, StackCapacity),
        Self:        mp,
        Pointers:    PtrActivePlayer,
    }
    s.PushInt(-1)
    err := handleHealEnergy(s)
    if err == nil {
        t.Fatalf("handleHealEnergy: expected error for amount=-1")
    }
    if !strings.Contains(err.Error(), "HEAL_ENERGY") {
        t.Errorf("error: got %q, want to contain \"HEAL_ENERGY\"", err.Error())
    }
    // No write on error path.
    if mp.runenergyValue != 5000 {
        t.Errorf("runenergy: got %d, want 5000 (unchanged on error)", mp.runenergyValue)
    }
}

// TestHandleHealEnergy_RequiresActivePlayer pins the goscape-only
// defensive guard (TS skips this check).
func TestHandleHealEnergy_RequiresActivePlayer(t *testing.T) {
    s := &ScriptState{
        IntStack:    make([]int, StackCapacity),
        StringStack: make([]string, StackCapacity),
    }
    s.PushInt(100) // amount, will be dead pop on error path
    err := handleHealEnergy(s)
    if err == nil {
        t.Fatalf("handleHealEnergy: expected error, got nil")
    }
    if !strings.Contains(err.Error(), "HEAL_ENERGY") {
        t.Errorf("error: got %q, want to contain \"HEAL_ENERGY\"", err.Error())
    }
}
```

- [ ] **Step 5.2: Run the failing tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestHandleHealEnergy`
Expected: FAIL with "undefined: handleHealEnergy".

- [ ] **Step 5.3: Implement `handleHealEnergy`**

Append to `pkg/script/handlers_player.go`:

```go
// handleHealEnergy (HEAL_ENERGY) adds the popped amount to the player's
// run-energy and clamps the result to [0, 10000]. Mirrors TS
// LostCityRS/Engine-TS/.../PlayerOps.ts:1050-1054:
//
//   const amount = check(state.popInt(), NumberNotNull) // 100=1%, 10000=100%
//   player.runenergy = Math.min(Math.max(player.runenergy + amount, 0), 10000)
//
// The active-player guard is goscape defensive (TS skips this check;
// see defensive_gate_doc_comment_label).
func handleHealEnergy(s *ScriptState) error {
    if err := requireActivePlayer(s, "HEAL_ENERGY"); err != nil {
        return err
    }
    amount := s.PopInt()
    if err := checkNotNull(amount, "HEAL_ENERGY"); err != nil {
        return err
    }
    next := s.Self.RunEnergy() + amount
    if next < 0 {
        next = 0
    } else if next > 10000 {
        next = 10000
    }
    s.Self.SetRunEnergy(next)
    return nil
}
```

- [ ] **Step 5.4: Register in handlers.go**

```go
OpHealEnergy: handleHealEnergy,
```

- [ ] **Step 5.5: Run the tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestHandleHealEnergy`
Expected: PASS.

- [ ] **Step 5.6: Commit**

```bash
git add pkg/script/handlers_player.go pkg/script/handlers_player_test.go pkg/script/handlers.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): NAI-149 T5 — port HEAL_ENERGY with [0,10000] clamp

Pops amount, validates NumberNotNull, adds to runenergy, clamps
both directions. Tests pin 5 clamp scenarios + null-sentinel rejection
+ defensive active-player guard.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 6 — `handleSeqLength` + `checkSeqType` helper (OpSeqLength)

**Files:**
- Modify: `pkg/script/handlers_player.go` (place `checkSeqType` adjacent to `checkInvType` for DRY/discoverability — see plan_grep_helper_patterns.md)
- Modify: `pkg/script/handlers_server.go`
- Modify: `pkg/script/handlers_config_test.go` (extend `mockConfigs` with `seqs` map)
- Create or Modify: `pkg/script/handlers_server_test.go`
- Modify: `pkg/script/handlers.go`

- [ ] **Step 6.1: Extend `mockConfigs` with seqs support**

In `pkg/script/handlers_config_test.go`, add a `seqs` field to the struct and update the `SeqType` method:

```go
type mockConfigs struct {
    // ... existing fields ...
    seqs          map[int]*objtype.SeqType
}
```

Update line 34 from:
```go
func (m *mockConfigs) SeqType(id int) *objtype.SeqType              { return nil }
```
to:
```go
func (m *mockConfigs) SeqType(id int) *objtype.SeqType              { return m.seqs[id] }
```

- [ ] **Step 6.2: Verify the package still builds**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestHandle -count=1` (or any pkg/script test that uses mockConfigs)
Expected: PASS — additive field is non-breaking.

- [ ] **Step 6.3: Write the failing tests**

Check whether `pkg/script/handlers_server_test.go` exists. If yes, append. If not, create it with `package script` and `import` boilerplate matching `handlers_inv_test.go`.

```go
// TestHandleSeqLength_PushesDuration pins TS ServerOps.ts:109-111
// (NAI-149). state.pushInt(check(popInt(), SeqTypeValid).duration).
func TestHandleSeqLength_PushesDuration(t *testing.T) {
    seq := &objtype.SeqType{
        ConfigType: objtype.ConfigType{ID: 42},
        Duration:   180, // ticks
    }
    mc := &mockConfigs{
        seqs: map[int]*objtype.SeqType{42: seq},
    }
    s := &ScriptState{
        IntStack:    make([]int, StackCapacity),
        StringStack: make([]string, StackCapacity),
        Configs:     mc,
    }
    s.PushInt(42)
    if err := handleSeqLength(s); err != nil {
        t.Fatalf("handleSeqLength: %v", err)
    }
    if got := s.IntStack[0]; got != 180 {
        t.Errorf("top: got %d, want 180", got)
    }
}

// TestHandleSeqLength_RejectsUnknownID pins TS check(id, SeqTypeValid)
// — unknown id throws.
func TestHandleSeqLength_RejectsUnknownID(t *testing.T) {
    mc := &mockConfigs{
        seqs: map[int]*objtype.SeqType{}, // empty
    }
    s := &ScriptState{
        IntStack:    make([]int, StackCapacity),
        StringStack: make([]string, StackCapacity),
        Configs:     mc,
    }
    s.PushInt(99)
    err := handleSeqLength(s)
    if err == nil {
        t.Fatalf("handleSeqLength: expected error for unknown id")
    }
    if !strings.Contains(err.Error(), "SEQ_LENGTH") {
        t.Errorf("error: got %q, want to contain \"SEQ_LENGTH\"", err.Error())
    }
}
```

(If `strings` import or `objtype` import is missing, add.)

- [ ] **Step 6.4: Run the failing tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestHandleSeqLength`
Expected: FAIL with "undefined: handleSeqLength".

- [ ] **Step 6.5: Add `checkSeqType` helper**

Append to `pkg/script/handlers_player.go` adjacent to `checkInvType` (around line 142+) — keep the family of `check*Type` helpers grouped:

```go
// checkSeqType validates a SeqType id is registered in s.Configs.
// Mirrors TS check(id, SeqTypeValid) (ScriptValidators.ts).
func checkSeqType(s *ScriptState, id int, op string) error {
    if s.Configs == nil || s.Configs.SeqType(id) == nil {
        return fmt.Errorf("%s: no SeqType with value (%d) found", op, id)
    }
    return nil
}
```

(Helper file already imports `fmt`; verify with a quick grep before saving.)

- [ ] **Step 6.6: Implement `handleSeqLength`**

Append to `pkg/script/handlers_server.go`:

```go
// handleSeqLength (SEQ_LENGTH) pushes the configured duration of a
// SeqType. Mirrors TS LostCityRS/Engine-TS/.../ServerOps.ts:109-111:
//
//   state.pushInt(check(state.popInt(), SeqTypeValid).duration);
func handleSeqLength(s *ScriptState) error {
    id := s.PopInt()
    if err := checkSeqType(s, id, "SEQ_LENGTH"); err != nil {
        return err
    }
    s.PushInt(s.Configs.SeqType(id).Duration)
    return nil
}
```

- [ ] **Step 6.7: Register in handlers.go**

```go
OpSeqLength: handleSeqLength,
```

- [ ] **Step 6.8: Run the tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestHandleSeqLength`
Expected: PASS.

- [ ] **Step 6.9: Commit**

```bash
git add pkg/script/handlers_player.go pkg/script/handlers_server.go pkg/script/handlers_server_test.go pkg/script/handlers_config_test.go pkg/script/handlers.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): NAI-149 T6 — port SEQ_LENGTH + add checkSeqType helper

handleSeqLength pushes SeqType.Duration via new checkSeqType helper
(grouped with checkInvType/checkObjType family per
plan_grep_helper_patterns). Extends mockConfigs.seqs map for tests.
Mirrors TS ServerOps.ts:109-111.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 7 — `handleInvStockBase` (OpInvStockBase = 4325)

**Files:**
- Modify: `pkg/script/handlers_inv.go`
- Modify: `pkg/script/handlers_inv_test.go`
- Modify: `pkg/script/handlers.go`

- [ ] **Step 7.1: Write the failing tests (3-branch dual-pin per `vararg_opcode_shapes_dont_share_with_fixed_arg_siblings.md` analogue — separate fixtures per branch)**

Append to `pkg/script/handlers_inv_test.go`:

```go
// TestHandleInvStockBase_NilStockObjReturnsMinusOne pins TS
// InvOps.ts:46-49 — `if (!invType.stockobj || !invType.stockcount) return -1`.
func TestHandleInvStockBase_NilStockObjReturnsMinusOne(t *testing.T) {
    invType := objtype.NewInvType(testInvMain)
    invType.DebugName = "main"
    // StockObj/StockCount default-nil per NewInvType.
    obj := objtype.NewObjType(testObjCoin)
    mc := &mockConfigs{
        invs: map[int]*objtype.InvType{testInvMain: invType},
        objs: map[int]*objtype.ObjType{testObjCoin: obj},
    }
    s := &ScriptState{
        IntStack:    make([]int, StackCapacity),
        StringStack: make([]string, StackCapacity),
        Configs:     mc,
    }
    s.PushInt(testObjCoin) // obj (popped second)
    s.PushInt(testInvMain) // inv (popped first per spec — verify pop order matches TS)

    // NOTE: TS popInts(2) returns [inv, obj] from a stack pushed [inv, obj].
    // Verify pop order in the implementation matches; if it differs, swap PushInts above.

    if err := handleInvStockBase(s); err != nil {
        t.Fatalf("handleInvStockBase: %v", err)
    }
    if got := s.IntStack[0]; got != -1 {
        t.Errorf("top: got %d, want -1 (nil StockObj)", got)
    }
}

// TestHandleInvStockBase_ObjNotInListReturnsMinusOne pins TS InvOps.ts:51-52
// — index < 0 → push -1.
func TestHandleInvStockBase_ObjNotInListReturnsMinusOne(t *testing.T) {
    invType := objtype.NewInvType(testInvMain)
    invType.DebugName = "main"
    invType.StockObj = []uint16{1, 2, 3}
    invType.StockCount = []uint16{10, 20, 30}
    obj := objtype.NewObjType(99) // not in stock
    mc := &mockConfigs{
        invs: map[int]*objtype.InvType{testInvMain: invType},
        objs: map[int]*objtype.ObjType{99: obj},
    }
    s := &ScriptState{
        IntStack:    make([]int, StackCapacity),
        StringStack: make([]string, StackCapacity),
        Configs:     mc,
    }
    s.PushInt(99)
    s.PushInt(testInvMain)

    if err := handleInvStockBase(s); err != nil {
        t.Fatalf("handleInvStockBase: %v", err)
    }
    if got := s.IntStack[0]; got != -1 {
        t.Errorf("top: got %d, want -1 (obj not in list)", got)
    }
}

// TestHandleInvStockBase_ObjInListReturnsCount pins TS InvOps.ts:52
// — push stockcount[index].
func TestHandleInvStockBase_ObjInListReturnsCount(t *testing.T) {
    invType := objtype.NewInvType(testInvMain)
    invType.DebugName = "main"
    invType.StockObj = []uint16{10, 20, 30}
    invType.StockCount = []uint16{100, 200, 300}
    obj := objtype.NewObjType(20) // index=1
    mc := &mockConfigs{
        invs: map[int]*objtype.InvType{testInvMain: invType},
        objs: map[int]*objtype.ObjType{20: obj},
    }
    s := &ScriptState{
        IntStack:    make([]int, StackCapacity),
        StringStack: make([]string, StackCapacity),
        Configs:     mc,
    }
    s.PushInt(20)
    s.PushInt(testInvMain)

    if err := handleInvStockBase(s); err != nil {
        t.Fatalf("handleInvStockBase: %v", err)
    }
    if got := s.IntStack[0]; got != 200 {
        t.Errorf("top: got %d, want 200 (stockcount[1])", got)
    }
}
```

> **Implementer attention — `handler_pop_order_test_masking.md`:** TS `state.popInts(2)` returns `[inv, obj]` from pushes `[inv, obj]` (LIFO destructure). The implementation MUST pop in the order matching the test fixture's PUSH order. The tests above push `obj` first, then `inv`, expecting `inv = PopInt()` THEN `obj = PopInt()`. If the implementation reverses, ALL THREE TESTS WILL PASS WITH SWAPPED FIXTURES — re-read `handler_pop_order_test_masking.md` before committing. Verify against an existing INV_* handler with `popInts(2)` (e.g., grep `handlers_inv.go` for a 2-arg pop).

- [ ] **Step 7.2: Run the failing tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestHandleInvStockBase`
Expected: FAIL with "undefined: handleInvStockBase".

- [ ] **Step 7.3: Implement `handleInvStockBase`**

Before writing, run: `grep -n "popInts\|PopInt" pkg/script/handlers_inv.go | head -10` and pick a 2-arg sibling (e.g., the INV_GETOBJ/INV_GETNUM family) to confirm the canonical pop order.

Append to `pkg/script/handlers_inv.go`:

```go
// handleInvStockBase (INV_STOCKBASE, opcode 4325) returns the configured
// stock count for an object in an inventory's stock list, or -1 if the
// inventory has no stock or the object is not in the stock list.
// Mirrors TS LostCityRS/Engine-TS/.../InvOps.ts:41-54.
func handleInvStockBase(s *ScriptState) error {
    inv := s.PopInt()
    obj := s.PopInt()
    if err := checkInvType(s, inv, "INV_STOCKBASE"); err != nil {
        return err
    }
    if err := checkObjType(s, obj, "INV_STOCKBASE"); err != nil {
        return err
    }
    invType := s.Configs.InvType(inv)
    objType := s.Configs.ObjType(obj)
    if len(invType.StockObj) == 0 || len(invType.StockCount) == 0 {
        s.PushInt(-1)
        return nil
    }
    idx := -1
    for i, id := range invType.StockObj {
        if int(id) == objType.ID {
            idx = i
            break
        }
    }
    if idx < 0 {
        s.PushInt(-1)
        return nil
    }
    s.PushInt(int(invType.StockCount[idx]))
    return nil
}
```

> **NOTE:** If grep in this step shows that `handlers_inv.go` siblings consistently `popInts(2)` and destructure `[a, b]` from a stack PUSHED `[a, b]`, the order above is right. If the convention differs (some packages reverse), align to the local convention and update test PUSH order to match.

- [ ] **Step 7.4: Register in handlers.go**

```go
OpInvStockBase: handleInvStockBase,
```

- [ ] **Step 7.5: Run the tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestHandleInvStockBase`
Expected: PASS (3 sub-tests).

- [ ] **Step 7.6: Commit**

```bash
git add pkg/script/handlers_inv.go pkg/script/handlers_inv_test.go pkg/script/handlers.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): NAI-149 T7 — port INV_STOCKBASE (opcode 4325)

3-branch implementation: nil StockObj/StockCount → -1, obj-not-in-list
→ -1, obj-in-list → StockCount[idx]. Distinct fixtures per branch
(handler_pop_order_test_masking guard). Closes WARN class from
[proc,price_mod] in 2026-05-09 user logs.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 8 — `handleInvDebugName` (OpInvDebugName)

**Files:**
- Modify: `pkg/script/handlers_inv.go`
- Modify: `pkg/script/handlers_inv_test.go`
- Modify: `pkg/script/handlers.go`

- [ ] **Step 8.1: Write the failing tests (debugname present + null-fallback)**

Append to `pkg/script/handlers_inv_test.go`:

```go
// TestHandleInvDebugName_PushesName pins TS InvOps.ts:34-38 happy path.
func TestHandleInvDebugName_PushesName(t *testing.T) {
    invType := objtype.NewInvType(testInvMain)
    invType.DebugName = "main"
    mc := &mockConfigs{
        invs: map[int]*objtype.InvType{testInvMain: invType},
    }
    s := &ScriptState{
        IntStack:    make([]int, StackCapacity),
        StringStack: make([]string, StackCapacity),
        Configs:     mc,
    }
    s.PushInt(testInvMain)
    if err := handleInvDebugName(s); err != nil {
        t.Fatalf("handleInvDebugName: %v", err)
    }
    if got := s.StringStack[0]; got != "main" {
        t.Errorf("top of string stack: got %q, want %q", got, "main")
    }
}

// TestHandleInvDebugName_EmptyFallsBackToNullLiteral pins TS InvOps.ts:37
// — `invType.debugname ?? 'null'`.
func TestHandleInvDebugName_EmptyFallsBackToNullLiteral(t *testing.T) {
    invType := objtype.NewInvType(testInvMain)
    invType.DebugName = "" // simulate the TS undefined → ?? 'null' arm
    mc := &mockConfigs{
        invs: map[int]*objtype.InvType{testInvMain: invType},
    }
    s := &ScriptState{
        IntStack:    make([]int, StackCapacity),
        StringStack: make([]string, StackCapacity),
        Configs:     mc,
    }
    s.PushInt(testInvMain)
    if err := handleInvDebugName(s); err != nil {
        t.Fatalf("handleInvDebugName: %v", err)
    }
    if got := s.StringStack[0]; got != "null" {
        t.Errorf("top of string stack: got %q, want %q", got, "null")
    }
}
```

- [ ] **Step 8.2: Run the failing tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestHandleInvDebugName`
Expected: FAIL with "undefined: handleInvDebugName".

- [ ] **Step 8.3: Implement `handleInvDebugName`**

Append to `pkg/script/handlers_inv.go`:

```go
// handleInvDebugName (INV_DEBUG_NAME) pushes the debug name of an
// InvType, or "null" if the field is empty. Mirrors TS
// LostCityRS/Engine-TS/.../InvOps.ts:34-38:
//
//   const invType = check(state.popInt(), InvTypeValid)
//   state.pushString(invType.debugname ?? 'null')
func handleInvDebugName(s *ScriptState) error {
    inv := s.PopInt()
    if err := checkInvType(s, inv, "INV_DEBUG_NAME"); err != nil {
        return err
    }
    invType := s.Configs.InvType(inv)
    if invType.DebugName == "" {
        s.PushString("null")
    } else {
        s.PushString(invType.DebugName)
    }
    return nil
}
```

- [ ] **Step 8.4: Register in handlers.go**

```go
OpInvDebugName: handleInvDebugName,
```

- [ ] **Step 8.5: Run the tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestHandleInvDebugName`
Expected: PASS.

- [ ] **Step 8.6: Commit**

```bash
git add pkg/script/handlers_inv.go pkg/script/handlers_inv_test.go pkg/script/handlers.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): NAI-149 T8 — port INV_DEBUG_NAME

Pushes InvType.DebugName, or literal "null" string if empty
(TS `invType.debugname ?? 'null'`). Mirrors InvOps.ts:34-38.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 9 — `handleSetSkinColour` (OpSetSkinColour)

**Files:**
- Modify: `pkg/script/handlers_player.go`
- Modify: `pkg/script/handlers_player_test.go`
- Modify: `pkg/script/handlers.go`

- [ ] **Step 9.1: Write the failing tests (happy + off-by-one boundary + defensive guard)**

Append to `pkg/script/handlers_player_test.go`:

```go
// TestHandleSetSkinColour_WritesColors4 pins TS PlayerOps.ts:1121-1124
// — colors[4] = skin (range 0..7).
func TestHandleSetSkinColour_WritesColors4(t *testing.T) {
    cases := []struct {
        name string
        skin int
    }{
        {"min boundary 0", 0},
        {"mid 3", 3},
        {"max boundary 7", 7},
    }
    for _, tc := range cases {
        t.Run(tc.name, func(t *testing.T) {
            mp := &mockPlayer{}
            s := &ScriptState{
                IntStack:    make([]int, StackCapacity),
                StringStack: make([]string, StackCapacity),
                Self:        mp,
                Pointers:    PtrActivePlayer,
            }
            s.PushInt(tc.skin)
            if err := handleSetSkinColour(s); err != nil {
                t.Fatalf("handleSetSkinColour: %v", err)
            }
            if got := mp.colorParts[4]; got != tc.skin {
                t.Errorf("colorParts[4]: got %d, want %d", got, tc.skin)
            }
            // Other slots must NOT be touched.
            for i, c := range mp.colorParts {
                if i == 4 {
                    continue
                }
                if c != 0 {
                    t.Errorf("colorParts[%d]: got %d, want 0 (only slot 4 written)", i, c)
                }
            }
        })
    }
}

// TestHandleSetSkinColour_RejectsOutOfRange pins TS check(skin, SkinColourValid)
// — range 0..7 inclusive. Tests both off-by-one boundaries.
func TestHandleSetSkinColour_RejectsOutOfRange(t *testing.T) {
    cases := []struct {
        name string
        skin int
    }{
        {"-1 below min", -1},
        {"8 above max", 8},
        {"large negative", -100},
        {"large positive", 100},
    }
    for _, tc := range cases {
        t.Run(tc.name, func(t *testing.T) {
            mp := &mockPlayer{}
            s := &ScriptState{
                IntStack:    make([]int, StackCapacity),
                StringStack: make([]string, StackCapacity),
                Self:        mp,
                Pointers:    PtrActivePlayer,
            }
            s.PushInt(tc.skin)
            err := handleSetSkinColour(s)
            if err == nil {
                t.Fatalf("handleSetSkinColour(%d): expected error, got nil", tc.skin)
            }
            if !strings.Contains(err.Error(), "SETSKINCOLOUR") {
                t.Errorf("error: got %q, want to contain \"SETSKINCOLOUR\"", err.Error())
            }
            // No write on error.
            if mp.colorParts[4] != 0 {
                t.Errorf("colorParts[4]: got %d, want 0 (no write on error)", mp.colorParts[4])
            }
        })
    }
}

// TestHandleSetSkinColour_RequiresActivePlayer pins the goscape-only
// defensive guard.
func TestHandleSetSkinColour_RequiresActivePlayer(t *testing.T) {
    s := &ScriptState{
        IntStack:    make([]int, StackCapacity),
        StringStack: make([]string, StackCapacity),
    }
    s.PushInt(3)
    err := handleSetSkinColour(s)
    if err == nil {
        t.Fatalf("handleSetSkinColour: expected error, got nil")
    }
    if !strings.Contains(err.Error(), "SETSKINCOLOUR") {
        t.Errorf("error: got %q, want to contain \"SETSKINCOLOUR\"", err.Error())
    }
}
```

- [ ] **Step 9.2: Run the failing tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestHandleSetSkinColour`
Expected: FAIL with "undefined: handleSetSkinColour".

- [ ] **Step 9.3: Implement `handleSetSkinColour`**

Append to `pkg/script/handlers_player.go`:

```go
// handleSetSkinColour (SETSKINCOLOUR) writes the player's skin-colour
// slot (colors[4]) after a [0,7] range check. Mirrors TS
// LostCityRS/Engine-TS/.../PlayerOps.ts:1121-1124:
//
//   const skin = check(state.popInt(), SkinColourValid)
//   state.activePlayer.colors[4] = skin
//
// The active-player guard is goscape defensive (TS skips this check;
// see defensive_gate_doc_comment_label).
func handleSetSkinColour(s *ScriptState) error {
    if err := requireActivePlayer(s, "SETSKINCOLOUR"); err != nil {
        return err
    }
    skin := s.PopInt()
    if skin < 0 || skin > 7 {
        return fmt.Errorf("SETSKINCOLOUR: invalid skin colour %d (range 0..7)", skin)
    }
    s.Self.SetColorPart(4, skin)
    return nil
}
```

- [ ] **Step 9.4: Register in handlers.go**

```go
OpSetSkinColour: handleSetSkinColour,
```

- [ ] **Step 9.5: Run the tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestHandleSetSkinColour`
Expected: PASS.

- [ ] **Step 9.6: Final whole-package run**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/... ./modules/world/... -count=1`
Expected: PASS — verifies no regression in unrelated handlers and that the modules/world Player still implements ActivePlayer correctly.

- [ ] **Step 9.7: Commit**

```bash
git add pkg/script/handlers_player.go pkg/script/handlers_player_test.go pkg/script/handlers.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): NAI-149 T9 — port SETSKINCOLOUR with [0,7] range guard

Pops skin, range-checks 0..7 inclusive, writes colors[4] via existing
SetColorPart helper. Off-by-one boundary tests at -1 and 8.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Closing — review + memory

After T9 commits cleanly:

- [ ] **Single-shot Sonnet `superpowers:code-reviewer` pass** over all 9 commits (per `superpowers_code_reviewer_model.md` — Sonnet, never Opus). Address any review findings as fixup commits BEFORE the close commit.

- [ ] **Re-run missing-handler audit one-liner** to confirm cohort is fully wired:

```bash
mkdir -p /tmp/claude
awk '/^var handlers = map\[Opcode\]/,/^}/' pkg/script/handlers.go | grep -oE 'Op[A-Za-z_]+' | sort -u > /tmp/claude/handled.txt
awk '/^const \(/,/^\)/' pkg/script/opcode.go | grep -oE 'Op[A-Za-z_]+\b' | sort -u > /tmp/claude/declared.txt
comm -23 /tmp/claude/declared.txt /tmp/claude/handled.txt | wc -l
```
Expected: 39 (down from 47 at HEAD `0a5085c`).

- [ ] **Smoke-handoff prep:** update the user with the 4 originally-flagged WARN classes' status: PLAYERMEMBER ✓, AFK_EVENT ✓, INV_STOCKBASE ✓, PROJANIM_NPC deferred to NAI-150.

- [ ] **Close commit** (use `Closes memory:` trailer per `close_commit_memory_trailer.md` if any memory entry is produced — otherwise standard chore close):

```bash
git commit --no-gpg-sign --allow-empty -m "$(cat <<'EOF'
chore(close): NAI-149 — trivial-handler sweep (8 ops)

Ports PLAYERMEMBER, AFK_EVENT, WEIGHT, HEAL_ENERGY, SEQ_LENGTH,
INV_STOCKBASE, INV_DEBUG_NAME, SETSKINCOLOUR. Closes 3 of 4 user-reported
"no handler for X" WARN classes from 2026-05-09/10 smoke logs (PROJANIM_NPC
deferred to NAI-150 — needs Zone.MapProjAnim wiring + NPC slot lookup).

Cascade-tail: 47 → 39 unhandled opcodes (audit one-liner verified post-T9).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

- [ ] **NAI-150 follow-up bookkeeping** — append to `MEMORY/nai_followups.md` if not already present:
  - "NAI-150 candidate: PROJANIM_NPC + PROJANIM_MAP + PROJANIM_PL cluster (3 ops sharing Zone.MapProjAnim infra)"

---

## Self-Review Checklist (controller, post-write)

1. **Spec coverage:** All 8 handlers in spec §2.3 mapped to T2-T9 ✓. Foundation (§2.1) → T1 ✓. checkSeqType helper (§2.2) → T6 ✓. Risk register R3 (checkObjType) verified at pre-flight ✓. R4 (ObjType.ID) verified at pre-flight ✓.
2. **Placeholder scan:** No TBD/TODO. The "verify pop order" note in T7.1 is genuine plan-author flag (per `handler_pop_order_test_masking.md`), not a placeholder.
3. **Type consistency:** `mockPlayer.runweightValue int`, `mockPlayer.membersValue bool`, `mockPlayer.afkEventReadyValue bool` consistent across T1, T2, T3, T4. `s.Configs.SeqType(id).Duration` int return consistent with `objtype.SeqType.Duration int` (verified pre-flight).
4. **Test fixture runnability** (per `plan_runnable_test_fixtures.md`): all `&ScriptState{}` literals init `IntStack`/`StringStack` with `make([]int/string, StackCapacity)`. All Configs-needing tests provide `Configs:`. All Self-needing tests provide `Self:` AND `Pointers:`.
