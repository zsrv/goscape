# NAI-34 — `npc_tele` Handler + `Npc.Teleport` Extraction Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fill the NPC_TELE script-VM stub (opcode 2541) and extract a shared `(n *Npc) Teleport(x, z, level int)` method that both the new script handler and the existing AI teleport sites delegate to. Unblocks the visible fishing-spot relocate behavior in `fishing_movement.rs2:10`.

**Architecture:** Five files touched across two packages. `modules/world` gains a new `Npc.Teleport` mirroring the established `Player.Teleport(x, z, level)` shape; `pkg/script` gains an `ActiveNpc.Teleport` interface method, a `handleNpcTele` function, and a dispatch entry. Two existing inline teleport sites in `npc_interaction.go` (wanderMode + patrolMode) are refactored to call the new method. Cadence: middle-tier per `compressed_cadence.md` 15-100 LOC bucket — single combined review at the end, no per-task review pairs.

**Tech Stack:** Go 1.26+. Pre-existing helpers used as-is: `checkCoord` (handlers_npc.go:8 — mirrors TS `CoordValid`), `requireActiveNpc` (handlers_npc.go:75), `unpackCoord` (handlers_player.go:18), `refreshNpcZone` (zone_refresh.go:33 — built-in same-zone short-circuit at :39). Reference shapes: `Player.Teleport` at `modules/world/player_script.go:226`; sibling NPC handler `handleNpcDelay` at `pkg/script/handlers_npc.go:296`. Existing test fixtures: `mockNpc` (handlers_npc_test.go:186); `newTestServer(t)` (server_test.go:311); `s.addNpc(n, -1, true)` wires `n.server = s` on first spawn (npc_registry.go:53).

---

## Pre-flight reminders for the implementer

Before starting any task, read these memory entries (the controller has already verified their applicability):

- `mock_recorder_field_naming_check.md` — the spec mistakenly says `mockActiveNpc`; the actual struct is `mockNpc`. Use the actual name. Recorder field naming convention: `xxxCalls []struct{...}`.
- `plan_runnable_test_fixtures.md` — every code block here has been written to compile against current HEAD. If you spot an issue, fix it inline and continue (don't escalate).
- `plan_var_name_collision.md` — none anticipated, but mentally compile each function body before pasting.
- `int32_hex_literal_overflow.md` — no `0x...` script IDs in this plan; not applicable.
- `compressed_cadence.md` — middle-tier cadence applies: subagent-driven, single combined review at end, no per-task reviews.

---

## Task 1: Extract `(n *Npc) Teleport(x, z, level int)` (Layer 2 TDD)

**Goal:** Add the world-side `Npc.Teleport` method mirroring `Player.Teleport(x, z, level)` exactly. New code; no consumers yet (Tasks 2 + 3 will wire callers).

**Files:**
- Modify: `modules/world/npc_script.go` (add method after `SetNpcVarN` which ends ~line 84, before `buildNpcScriptState` which starts ~line 88)
- Test: `modules/world/npc_script_test.go` (append new test functions)

- [ ] **Step 1: Write Layer 2 failing tests**

Append these four test functions to `modules/world/npc_script_test.go`:

```go
func TestNpcTeleport_SetsFieldsAndTeleFlag(t *testing.T) {
	s := newTestServer(t)
	n := &Npc{nid: 0, typeId: 0, x: 5000, z: 5000, level: 0, startX: 5000, startZ: 5000, startLevel: 0}
	if err := s.addNpc(n, -1, true); err != nil {
		t.Fatalf("addNpc: %v", err)
	}
	n.tele = false
	n.Teleport(3200, 3200, 1)
	if n.x != 3200 || n.z != 3200 || n.level != 1 {
		t.Errorf("post-Teleport coords: got (%d, %d, %d), want (3200, 3200, 1)", n.x, n.z, n.level)
	}
	if !n.tele {
		t.Error("post-Teleport tele flag: got false, want true")
	}
}

func TestNpcTeleport_CrossZoneRefreshSubscription(t *testing.T) {
	s := newTestServer(t)
	n := &Npc{nid: 0, typeId: 0, x: 3200, z: 3200, level: 0, startX: 3200, startZ: 3200, startLevel: 0}
	if err := s.addNpc(n, -1, true); err != nil {
		t.Fatalf("addNpc: %v", err)
	}
	prevZone := s.zoneMap.Get(0, 3200, 3200)
	n.Teleport(4000, 4000, 0)
	newZone := s.zoneMap.Get(0, 4000, 4000)
	if prevZone.NpcsCount() != 0 {
		t.Errorf("prev zone NpcsCount after Teleport: got %d, want 0", prevZone.NpcsCount())
	}
	if newZone.NpcsCount() != 1 {
		t.Errorf("new zone NpcsCount after Teleport: got %d, want 1", newZone.NpcsCount())
	}
}

func TestNpcTeleport_SameZoneNoRefresh(t *testing.T) {
	s := newTestServer(t)
	n := &Npc{nid: 0, typeId: 0, x: 3200, z: 3200, level: 0, startX: 3200, startZ: 3200, startLevel: 0}
	if err := s.addNpc(n, -1, true); err != nil {
		t.Fatalf("addNpc: %v", err)
	}
	z := s.zoneMap.Get(0, 3200, 3200)
	prevElement := n.zoneListElement
	n.Teleport(3201, 3201, 0) // same 8x8 zone (400, 400)
	if z.NpcsCount() != 1 {
		t.Errorf("same-zone Teleport NpcsCount: got %d, want 1", z.NpcsCount())
	}
	if n.zoneListElement != prevElement {
		t.Error("same-zone Teleport should preserve zoneListElement (no leave/enter)")
	}
}

func TestNpcTeleport_NilServerNoOp(t *testing.T) {
	n := &Npc{nid: 0, typeId: 0, x: 5000, z: 5000, level: 0}
	// n.server intentionally nil — refreshNpcZone has a nil-guard at zone_refresh.go:34.
	n.Teleport(3200, 3200, 0)
	if n.x != 3200 || n.z != 3200 || n.level != 0 {
		t.Errorf("post-Teleport coords (nil server): got (%d, %d, %d), want (3200, 3200, 0)", n.x, n.z, n.level)
	}
	if !n.tele {
		t.Error("post-Teleport tele flag (nil server): got false, want true")
	}
}
```

- [ ] **Step 2: Run tests to verify FAIL**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./modules/world/ -run "TestNpcTeleport_" -v`
Expected: compile error like `n.Teleport undefined (type *Npc has no field or method Teleport)`.

- [ ] **Step 3: Add `(n *Npc) Teleport(x, z, level int)` to `modules/world/npc_script.go`**

Insert this method after `SetNpcVarN` (currently ends ~line 84) and before `buildNpcScriptState` (starts ~line 88). Verify the insertion point with `grep -n "^func\|^// build" modules/world/npc_script.go` — the new method goes between the last `(n *Npc)` method and the first `(s *Server)` method.

```go
// Teleport moves the NPC to (x, z, level), refreshes its zone
// subscription if the zone changed, and flags the client for a tele
// transition (no walk-anim interpolation). Mirrors Player.Teleport at
// player_script.go:226.
//
// Used by NPC_TELE script handler (pkg/script/handlers_npc.go) and by
// AI teleport sites — wanderMode home-tele (npc_interaction.go ~:97)
// and patrolMode waypoint-tele (~:126).
//
// DEVIATION NAI-34-D1..D5 vs TS PathingEntity.teleport (PathingEntity.ts:267):
// no level clamp, no unallocated-zone rejection, no focus(), no
// lastStepX/Z adjust, no previousLevel != level branch. Mirrors the
// established Player.Teleport reduced shape. Closure plan: future
// pathing-entity-teleport-parity sub-spec aligns both Player + Npc.
func (n *Npc) Teleport(x, z, level int) {
	prevX, prevZ, prevLevel := n.x, n.z, n.level
	n.x, n.z, n.level = x, z, level
	refreshNpcZone(n.server, n, prevX, prevZ, prevLevel)
	n.tele = true
}
```

- [ ] **Step 4: Run tests to verify PASS**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./modules/world/ -run "TestNpcTeleport_" -v`
Expected: 4 PASS.

- [ ] **Step 5: Run full module tests for regression baseline**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./modules/world/...`
Expected: PASS (all existing tests still green; the new method adds no consumers yet).

- [ ] **Step 6: Commit**

```bash
git add modules/world/npc_script.go modules/world/npc_script_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-34 Task 1 — Npc.Teleport extraction (mirrors Player.Teleport)

Adds (n *Npc) Teleport(x, z, level int) to modules/world/npc_script.go,
mirroring Player.Teleport(x, z, level) at player_script.go:226 line-for-line.
Refreshes zone subscription via refreshNpcZone (with built-in same-zone
short-circuit) and raises the n.tele mask flag. Behavior-preserving: no
existing call sites yet — Tasks 2 + 3 wire AI sites and the script handler.

5 TS divergences vs PathingEntity.teleport (level clamp, unallocated-zone
rejection, focus, lastStepX/Z adjust, level-change branch) tracked in the
method's doc comment as NAI-34-D1..D5 with closure plan.

Layer 2 tests cover: field assignment + tele flag, cross-zone subscription
refresh, same-zone no-refresh short-circuit, nil-server no-op.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: Refactor wanderMode + patrolMode inline sites; fix `zone_refresh.go` doc comment

**Goal:** Replace the two existing 4-line inline teleport patterns with single-line calls to `n.Teleport(...)`. Behavior-preserving — no semantics change. Existing AI tests act as the regression baseline.

**Files:**
- Modify: `modules/world/npc_interaction.go` (wanderMode block at ~lines 95-98; patrolMode block at ~lines 124-127)
- Modify: `modules/world/zone_refresh.go` (doc comment at lines 28-30)
- Optional: `modules/world/npc_ai_test.go` (add `n.tele` assertion to `TestTeleportHomeAfterStuck` if not already present)

- [ ] **Step 1: Pre-check — establish regression baseline**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./modules/world/ -run "TestTeleportHomeAfterStuck|TestNpcStuckTeleportRefreshSubscription" -v`
Expected: PASS. These are the existing tests that exercise wanderMode home-tele behavior. They MUST stay green after the refactor.

- [ ] **Step 2: Refactor wanderMode home-tele site**

Open `modules/world/npc_interaction.go`. Locate the wanderMode block (search for `n.wanderCounter >= 500`). The existing pattern is:

```go
		if !onSpawn {
			prevX, prevZ, prevLevel := n.x, n.z, n.level
			n.x, n.z, n.level = n.startX, n.startZ, n.startLevel
			refreshNpcZone(s, n, prevX, prevZ, prevLevel)
			n.tele = true
		}
```

Replace with:

```go
		if !onSpawn {
			n.Teleport(n.startX, n.startZ, n.startLevel)
		}
```

- [ ] **Step 3: Refactor patrolMode waypoint-tele site**

In the same file, locate the patrolMode block (search for `s.currentTick >= n.nextPatrolTick` inside `func (n *Npc) patrolMode`). The existing pattern is:

```go
	if (n.x != dest.X || n.z != dest.Z) && n.nextPatrolTick > -1 && s.currentTick >= n.nextPatrolTick {
		prevX, prevZ, prevLevel := n.x, n.z, n.level
		n.x, n.z, n.level = dest.X, dest.Z, 0
		refreshNpcZone(s, n, prevX, prevZ, prevLevel)
		n.tele = true
	}
```

Replace with:

```go
	if (n.x != dest.X || n.z != dest.Z) && n.nextPatrolTick > -1 && s.currentTick >= n.nextPatrolTick {
		n.Teleport(dest.X, dest.Z, 0)
	}
```

**Important:** Preserve the literal `0` for level (do NOT use `dest.Level`). TS Npc.ts:729 uses `dest.level` but goscape's existing inline code uses `0` — this is a pre-existing divergence that NAI-34's refactor surfaces but does NOT introduce. It is logged separately in `nai_followups.md` (Task 5 of this plan). NOT counted as a NAI-34 deviation.

- [ ] **Step 4: Update the stale doc comment in `zone_refresh.go`**

Open `modules/world/zone_refresh.go`. The current doc comment at lines 28-30 reads:

```go
// refreshNpcZone is the NPC-side analogue of refreshPlayerZone. Called from
// (*Npc).stepOnce and the 3 NPC teleport sites in npc_interaction.go +
// npc_ai.go.
```

Replace with:

```go
// refreshNpcZone is the NPC-side analogue of refreshPlayerZone. Called from
// (*Npc).stepOnce, (*Npc).Teleport (used by wanderMode home-tele,
// patrolMode waypoint-tele, and the NPC_TELE script handler), and the
// respawn lifecycle path in (*Npc).turn (npc_ai.go ~:37).
```

- [ ] **Step 5: Run regression baseline**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./modules/world/ -run "TestTeleportHomeAfterStuck|TestNpcStuckTeleportRefreshSubscription" -v`
Expected: PASS — refactor is behavior-preserving.

- [ ] **Step 6: Run full module tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./modules/world/...`
Expected: PASS.

- [ ] **Step 7 (conditional): Add `n.tele` assertion to `TestTeleportHomeAfterStuck` if missing**

Read `modules/world/npc_ai_test.go:31` (the `TestTeleportHomeAfterStuck` test). If it does NOT already assert `n.tele == true` post-teleport, add this assertion after the existing teleport-coord checks:

```go
	if !n.tele {
		t.Error("tele flag should be set after wanderMode home-teleport")
	}
```

If the assertion is already present (a comment search for "tele flag" near line 38-45 will tell you), skip this step.

- [ ] **Step 8: Commit**

```bash
git add modules/world/npc_interaction.go modules/world/zone_refresh.go
# Add modules/world/npc_ai_test.go ONLY if Step 7 modified it.
git status # confirm staged set
git commit --no-gpg-sign -m "$(cat <<'EOF'
refactor(world): NAI-34 Task 2 — wanderMode + patrolMode call Npc.Teleport

Replaces the two inline 4-line teleport patterns in npc_interaction.go
(wanderMode home-tele ~:97, patrolMode waypoint-tele ~:126) with single-line
calls to n.Teleport(...). Behavior-preserving — no semantics change to the
two existing AI sites. Existing tests TestTeleportHomeAfterStuck and
TestNpcStuckTeleportRefreshSubscription pass unchanged as the regression
baseline.

Drive-by: zone_refresh.go:28-30 doc comment was stale ("3 NPC teleport sites"
miscounted both call shape and call-site count). Updated to enumerate the
actual callers of refreshNpcZone (Npc.stepOnce, Npc.Teleport, respawn
lifecycle path in Npc.turn).

Patrol's literal level=0 is preserved as-is — pre-existing divergence vs
TS Npc.ts:729 (which uses dest.level). NOT introduced by NAI-34. Logged
in nai_followups.md as a separate follow-up at NAI-34 close.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: Add `ActiveNpc.Teleport` interface + `mockNpc.Teleport` + `handleNpcTele` + Layer 1 unit tests

**Goal:** Wire the script-side surface — interface method, mock impl, handler function, and direct-call unit tests. Tests pass without dispatch wiring (Task 4) by calling `handleNpcTele(s)` directly.

**Files:**
- Modify: `pkg/script/active.go` (add `Teleport(x, z, level int)` to `ActiveNpc` interface; insert before the closing `}` at line 484)
- Modify: `pkg/script/handlers_npc.go` (add `handleNpcTele` function; can append at end of file)
- Modify: `pkg/script/handlers_npc_test.go` (add `teleportCalls` field + `Teleport` method to `mockNpc` struct; add new test functions)

- [ ] **Step 1: Add `Teleport(x, z, level int)` to `ActiveNpc` interface**

Open `pkg/script/active.go`. Locate the `ActiveNpc` interface — it ends at line 484 (the `}` closing brace, right after `SetHuntMode(mode int)` at line 483). Insert the new method just before the closing `}`:

```go
	// Teleport moves the active NPC to (x, z, level) and flags the client
	// for a tele transition. Mirrors (n *Npc).Teleport on the world side
	// (modules/world/npc_script.go). Called by NPC_TELE handler
	// (handlers_npc.go) after checkCoord validates and unpacks the packed
	// coord.
	//
	// DEVIATION NAI-34-D1..D5 — see Npc.Teleport doc comment for the
	// full divergence list and closure plan.
	Teleport(x, z, level int)
}
```

(The trailing `}` is the existing closing brace of the interface — make sure not to duplicate it.)

- [ ] **Step 2: Add `Teleport` method + recorder field to `mockNpc`**

Open `pkg/script/handlers_npc_test.go`. The `mockNpc` struct is at line 186 (NOT `mockActiveNpc` as it might be referred to elsewhere — the actual type name is `mockNpc`).

Add a new recorder field to the struct. Locate the struct definition (lines 186-202 — fields end with `setHuntModeCalls []int`). Add a new field after `setHuntModeCalls`:

```go
	teleportCalls                      []struct{ x, z, level int }
```

The full updated struct should now look like (showing only the changed tail):

```go
type mockNpc struct {
	typeID, x, z, level, uid, category int
	curHP, baseHP                      int
	varns                              map[int]int32
	sayCalls                           []string
	animCalls                          []struct{ id, delay int }
	faceCoordCalls                     []struct{ x, z int }
	changeTypeCalls                    []struct{ newType, duration int }
	changeTypeKeepAllCalls             []struct{ newType, duration int }
	damageCalls                        []struct{ amount, dmgType int }
	enqueueCalls                       []mockEnqueueCall
	setDelayedCalls                    []int
	setTimerCalls                      []int
	setHuntRangeCalls                  []int
	setHuntModeCalls                   []int
	teleportCalls                      []struct{ x, z, level int }
}
```

Then add the method implementation. The other mutator methods are at lines 242-285 (after `Say` at :238). Add this method after `SetHuntMode` (around line 285):

```go
func (m *mockNpc) Teleport(x, z, level int) {
	m.teleportCalls = append(m.teleportCalls, struct{ x, z, level int }{x, z, level})
}
```

- [ ] **Step 3: Add `handleNpcTele` to `pkg/script/handlers_npc.go`**

Open `pkg/script/handlers_npc.go`. The sibling-shape reference is `handleNpcDelay` at ~line 296. Append the new function at a suitable location (any position after `requireActiveNpc` works; suggested: append after the existing `handleNpcSetTimer` function). Use `grep -n "^func handleNpc" pkg/script/handlers_npc.go` to find the existing positions.

```go
// handleNpcTele (NPC_TELE, opcode 2541) teleports the active NPC to
// the packed coord. Pop order: coord (single int). Mirrors TS
// NpcOps.ts:443 — checkedHandler(ActiveNpc) + CoordValid +
// activeNpc.teleport(x, z, level).
func handleNpcTele(s *ScriptState) error {
	if err := requireActiveNpc(s, "NPC_TELE"); err != nil {
		return err
	}
	coord := s.PopInt()
	level, x, z, err := checkCoord(coord, "NPC_TELE")
	if err != nil {
		return err
	}
	s.ActiveNpc.Teleport(x, z, level)
	return nil
}
```

- [ ] **Step 4: Write Layer 1 direct-call unit tests**

Append these test functions to `pkg/script/handlers_npc_test.go`. They call `handleNpcTele(s)` DIRECTLY without going through the dispatch table — Task 4 will add a separate test that exercises the dispatch path.

```go
func TestNpcTele_PopsCoordValidatesAndDelegates(t *testing.T) {
	// Pack (level=2, x=3200, z=3200) into a single RS2 coord int.
	// Pack format: (level<<28) | (x<<14) | z — see coordgrid.PackCoord.
	packed := (2 << 28) | (3200 << 14) | 3200
	npc := &mockNpc{}
	s := &ScriptState{ActiveNpc: npc}
	s.PushInt(packed)
	if err := handleNpcTele(s); err != nil {
		t.Fatalf("handleNpcTele: unexpected err %v", err)
	}
	if len(npc.teleportCalls) != 1 {
		t.Fatalf("teleportCalls: got %d, want 1", len(npc.teleportCalls))
	}
	got := npc.teleportCalls[0]
	if got.x != 3200 || got.z != 3200 || got.level != 2 {
		t.Errorf("teleportCalls[0]: got (x=%d, z=%d, level=%d), want (3200, 3200, 2)", got.x, got.z, got.level)
	}
}

func TestNpcTele_NoActiveNpcErrors(t *testing.T) {
	s := &ScriptState{ActiveNpc: nil}
	s.PushInt(0)
	err := handleNpcTele(s)
	if err == nil {
		t.Fatal("handleNpcTele: expected error for nil ActiveNpc, got nil")
	}
	if !strings.Contains(err.Error(), "NPC_TELE: no active npc") {
		t.Errorf("err: got %q, want substring %q", err.Error(), "NPC_TELE: no active npc")
	}
}

func TestNpcTele_InvalidCoordErrors(t *testing.T) {
	npc := &mockNpc{}
	s := &ScriptState{ActiveNpc: npc}
	s.PushInt(-1)
	err := handleNpcTele(s)
	if err == nil {
		t.Fatal("handleNpcTele: expected error for coord=-1, got nil")
	}
	if !strings.Contains(err.Error(), "NPC_TELE: coord out of range") {
		t.Errorf("err: got %q, want substring %q", err.Error(), "NPC_TELE: coord out of range")
	}
	if len(npc.teleportCalls) != 0 {
		t.Errorf("teleportCalls on error path: got %d, want 0 (handler must reject before delegating)", len(npc.teleportCalls))
	}
}

func TestNpcTele_PopOrderIsSinglePopInt(t *testing.T) {
	// Push two ints; verify the handler pops exactly 1 (the top one — packed coord).
	npc := &mockNpc{}
	s := &ScriptState{ActiveNpc: npc}
	s.PushInt(0xCAFE)            // bottom — should remain after handler
	s.PushInt((0 << 28) | (3200 << 14) | 3200) // top — packed coord, gets popped
	if err := handleNpcTele(s); err != nil {
		t.Fatalf("handleNpcTele: unexpected err %v", err)
	}
	// Verify remaining stack depth — exactly 1 int left (the 0xCAFE sentinel).
	if got := s.PopInt(); got != 0xCAFE {
		t.Errorf("residual stack top: got %d, want 0xCAFE — handler popped wrong number of ints", got)
	}
}
```

**If `strings` is not already imported in this test file**, add `"strings"` to the import block at the top of the file. Check with `grep -n "\"strings\"" pkg/script/handlers_npc_test.go` — if no match, add the import.

- [ ] **Step 5: Run tests to verify PASS**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./pkg/script/ -run "TestNpcTele_" -v`
Expected: 4 PASS.

- [ ] **Step 6: Run full pkg/script tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./pkg/script/...`
Expected: PASS. The new interface method on `ActiveNpc` may break compilation in OTHER places that implement the interface (e.g., other mock types in other test files). If so, add a no-op `Teleport(x, z, level int) {}` to each affected mock and re-run.

- [ ] **Step 7: Run full repo tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./...`
Expected: PASS. The `*Npc` in `modules/world` already implements `Teleport` from Task 1 — the interface contract is satisfied across packages.

- [ ] **Step 8: Commit**

```bash
git add pkg/script/active.go pkg/script/handlers_npc.go pkg/script/handlers_npc_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): NAI-34 Task 3 — ActiveNpc.Teleport interface + handleNpcTele

Adds Teleport(x, z, level int) to the ActiveNpc interface
(pkg/script/active.go) and the matching handleNpcTele function
(pkg/script/handlers_npc.go) that pops a packed coord, validates via
checkCoord (mirrors TS CoordValid), and delegates to s.ActiveNpc.Teleport.

Wires mockNpc test fixture (handlers_npc_test.go) with a teleportCalls
recorder + Teleport method to satisfy the updated interface.

Layer 1 unit tests cover: happy-path pop+validate+delegate, no-active-NPC
error gate, invalid-coord rejection (handler must NOT delegate on error
path), single-popInt pop order. Tests call handleNpcTele directly — Task 4
adds the dispatch entry that makes the handler reachable via the script VM.

Mirrors TS NpcOps.ts:443 checkedHandler(ActiveNpc) + CoordValid +
activeNpc.teleport(x, z, level).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: Register `OpNpcTele` in dispatch table + dispatch integration test

**Goal:** Add the one-line dispatch entry so the script VM can route opcode 2541 to `handleNpcTele`. Add one integration test that exercises the full dispatch path via `runNpcOp`.

**Files:**
- Modify: `pkg/script/handlers.go` (add `OpNpcTele: handleNpcTele,` after `OpNpcSetTimer:` at line 347)
- Modify: `pkg/script/handlers_npc_test.go` (append one dispatch-routed integration test)

- [ ] **Step 1: Write the failing dispatch integration test**

Append this test function to `pkg/script/handlers_npc_test.go`:

```go
func TestNpcTele_DispatchRoutes(t *testing.T) {
	// Integration test — exercises the dispatch table to confirm
	// OpNpcTele is routed to handleNpcTele. If dispatch is unset,
	// runNpcOp's internal t.Fatalf fires on the unknown-opcode error
	// returned by Execute (per runner_test.go:8 convention) before
	// reaching the assertions below.
	npc := &mockNpc{}
	packed := (0 << 28) | (3200 << 14) | 3200
	state := runNpcOp(t, npc, nil, OpNpcTele, []int{packed})
	if len(npc.teleportCalls) != 1 {
		t.Fatalf("teleportCalls after dispatch: got %d, want 1 (handler ran but didn't delegate to mock)", len(npc.teleportCalls))
	}
	got := npc.teleportCalls[0]
	if got.x != 3200 || got.z != 3200 || got.level != 0 {
		t.Errorf("teleportCalls[0]: got (x=%d, z=%d, level=%d), want (3200, 3200, 0)", got.x, got.z, got.level)
	}
	// Confirm the script reached normal completion (Finished), not Aborted.
	if state.Execution != Finished {
		t.Errorf("state.Execution after NPC_TELE: got %v, want Finished", state.Execution)
	}
}
```

**Pre-step verification:** confirm `runNpcOp` signature, `Finished` / `Aborted` constants, and `state.Execution` field name with `grep -n "func runNpcOp\|Execution\b\|^	Finished\|^	Aborted" pkg/script/handlers_npc_test.go pkg/script/state.go pkg/script/execution.go | head -10`. There is NO `state.Error` field — `runNpcOp`'s internal `t.Fatalf` surfaces dispatch errors directly via the error returned from `Execute`.

- [ ] **Step 2: Run test to verify FAIL**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./pkg/script/ -run "TestNpcTele_DispatchRoutes" -v`
Expected: FAIL. The error message should mention "missing dispatch entry for OpNpcTele=2541" (from the test's own diagnostic) or "unknown opcode" (from the dispatch path).

- [ ] **Step 3: Add dispatch entry**

Open `pkg/script/handlers.go`. Locate the NPC mutator block — `OpNpcSetTimer: handleNpcSetTimer,` is at line 347. Insert the new entry on the next line (line 348), keeping it grouped with the sibling mutators:

```go
	OpNpcSetTimer:          handleNpcSetTimer,
	OpNpcTele:              handleNpcTele,
```

Maintain the existing tab/space alignment. If your editor auto-reformats and the existing entries align differently from this snippet, run `gofmt -w pkg/script/handlers.go` after the edit.

- [ ] **Step 4: Run test to verify PASS**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./pkg/script/ -run "TestNpcTele_DispatchRoutes" -v`
Expected: PASS.

- [ ] **Step 5: Run full repo tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./... && GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...`
Expected: PASS for both `go test` and `go vet`.

- [ ] **Step 6: Commit**

```bash
git add pkg/script/handlers.go pkg/script/handlers_npc_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): NAI-34 Task 4 — register handleNpcTele in dispatch

Adds OpNpcTele -> handleNpcTele entry to pkg/script/handlers.go dispatch
table (between OpNpcSetTimer and the FIND family). Closes the stub-not-
completed shape: opcode 2541 was declared at opcode.go:278 with a name-
stringifier at :955 but never wired into dispatch — a textbook instance
of the pattern recorded in protocol_stub_not_completed.md.

Adds TestNpcTele_DispatchRoutes integration test that exercises the full
dispatch path via runNpcOp and verifies handleNpcTele receives the popped
coord and delegates to s.ActiveNpc.Teleport.

After this commit, the visible chain is end-to-end functional:
  fishing_movement.rs2:10  npc_tele($rand_coord)
    -> bytecode OP 2541
    -> dispatch[OpNpcTele] = handleNpcTele
    -> requireActiveNpc + checkCoord + s.ActiveNpc.Teleport(x, z, level)
    -> (*Npc).Teleport refreshes zone subscription + sets n.tele flag
    -> NPC info encode emits NpcMaskTele next tick

Smoke gate (closes NAI-33 spec item 4): user runs server, walks to a
fishing-spot zone, observes fishing NPCs visibly relocating when their
ai_timer fires.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 5: Combined review + smoke gate + close commit

**Goal:** Final review pass, smoke acceptance, memory updates, close commit. No new code.

**Files:**
- Modify: `/home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md` (add "From NAI-34" section; remove `NPC_TELE 2541` from "From NAI-33")

- [ ] **Step 1: Combined code review (subagent dispatch)**

Per `compressed_cadence.md` middle-tier cadence, dispatch a single combined-review subagent covering all 4 implementation commits. Use the `superpowers:requesting-code-review` skill or dispatch a `feature-dev:code-reviewer` subagent with this brief:

> Review commits since `e1201e1` (NAI-33 close) on the current branch. Scope: NAI-34 — `npc_tele` opcode 2541 handler + `Npc.Teleport` extraction. Verify against the spec at `docs/superpowers/specs/2026-04-26-nai-34-npc-tele-handler-design.md`. Critical checks: (a) `Npc.Teleport` mirrors `Player.Teleport(x, z, level)` shape exactly; (b) the 2 refactored AI sites in `npc_interaction.go` are behavior-preserving; (c) the 5 tracked deviations NAI-34-D1..D5 are documented in code comments; (d) `handleNpcTele` mirrors TS `NpcOps.ts:443` shape; (e) tests cover Layer 1 unit + Layer 1 dispatch + Layer 2 world + regression of existing AI tests; (f) no undocumented divergences from TS introduced beyond the 5 tracked. Report: critical (blocks close), high-priority, low-priority, or NOOP.

If the reviewer flags critical issues, fix and re-review. If only minor issues, apply as a `polish(script,world): NAI-34 final-review polish` commit.

- [ ] **Step 2: Smoke acceptance (user-launched)**

Per `smoke_test_server_handoff.md`, the smoke server must be user-launched (Claude's sandboxed server is unreachable from the host Java client). Provide the user with these instructions and wait for the result:

```
Smoke gate for NAI-34 (closes NAI-33 spec item 4):

1. Start server:
     CGO_ENABLED=0 GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache \
       go run -trimpath ./cmd/goscape --config.file config.yaml

2. Launch Java client; log in.

3. Walk to a fishing-spot zone (e.g., shrimp/anchovies at Lumbridge Swamp).

4. Wait for the move_fishing_spot ai_timer to fire (~280-530 tick period).

PASS criterion (binary):
  - Fishing NPC visibly relocates within fishing_movement_enum coord set.
  - Server log shows NO error containing "NPC_TELE" or "opcode 2541".

FAIL escalation (if any):
  - NPC stays put OR log shows OpNpcTele error.
  - Triage: (a) script calling NPC_TELE through indirect dispatch path
    the refactor missed; (b) n.tele flag not picked up by NPC info
    encode (separate stub); (c) n.server nil at script-handler bridge.
```

- [ ] **Step 3: Update `nai_followups.md`**

Use the Write or Edit tool (NOT bash printf/touch — per `memory_write_sandbox_quirk.md`, only Write/Edit can write to the memory directory).

Open `/home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md`. Two changes:

(a) **Remove** the `NPC_TELE 2541` entry from the "From NAI-33" section (it's been closed by NAI-34).

(b) **Add a new "From NAI-34" section** (or append to it if it already exists) with these three follow-up items:

```markdown
### From NAI-34

1. **`pathing-entity-teleport-parity` sub-spec** — closes NAI-34-D1..D5
   (Npc.Teleport divergences) AND the analogous 5 Player.Teleport
   divergences vs TS PathingEntity.teleport (PathingEntity.ts:267) in one
   sitting: level clamp to [0,3], unallocated-zone rejection,
   focus()/orient toward teleport vector, lastStepX/Z adjust for renderer
   step inference, previousLevel != level off-screen branch. Estimated
   ~80 LOC + tests; medium sub-spec.

2. **PatrolMode level discard** — `modules/world/npc_interaction.go:126`
   (post-refactor: the call to `n.Teleport(dest.X, dest.Z, 0)`) hardcodes
   `level = 0`; TS Npc.ts:729 uses `dest.level`. Pre-existing divergence
   surfaced (NOT introduced) by NAI-34's refactor read-through. Estimated
   1-line fix + 1 test. Could co-ship with the parity sub-spec above.

3. **NPC_WALK opcode 2542** — sibling of NPC_TELE in TS NpcOps.ts:451-455
   (`checkedHandler(ActiveNpc) + CoordValid + queueWaypoint(x, z)`). Same
   shape as NPC_TELE; calls `n.queueWaypoint(x, z)` instead of
   `n.Teleport`. Tiny sub-spec (~20 LOC).
```

- [ ] **Step 4: Close commit**

```bash
git add -A # stages only the polish commit if any; nai_followups.md is outside the repo
git status # confirm only repo-internal changes
git commit --no-gpg-sign --allow-empty -m "$(cat <<'EOF'
chore(script,world): NAI-34 closed — npc_tele handler + Npc.Teleport extraction

Closes NPC_TELE (opcode 2541) stub-not-completed and unblocks the visible
fishing-spot relocate behavior in fishing_movement.rs2:10. Also closes
NAI-33 spec item 4 (the smoke gate that NAI-33 left blocked one opcode
downstream in the same script).

Sub-spec scope (per docs/superpowers/specs/2026-04-26-nai-34-npc-tele-handler-design.md):

- Task 1: Extracted (n *Npc) Teleport(x, z, level int) mirroring
  Player.Teleport at player_script.go:226. Layer 2 tests cover field
  assignment, cross-zone refresh, same-zone short-circuit, nil-server
  no-op.
- Task 2: Refactored 2 inline NPC-teleport sites in npc_interaction.go
  (wanderMode home-tele + patrolMode waypoint-tele) to call n.Teleport.
  Behavior-preserving — existing tests TestTeleportHomeAfterStuck and
  TestNpcStuckTeleportRefreshSubscription pass unchanged.
- Task 3: Added ActiveNpc.Teleport interface method, mockNpc recorder,
  handleNpcTele function. Layer 1 direct-call unit tests.
- Task 4: Registered OpNpcTele -> handleNpcTele in dispatch table.
  Layer 1 dispatch integration test via runNpcOp.

5 documented deviations from TS PathingEntity.teleport tracked as
NAI-34-D1..D5 with closure plan = future pathing-entity-teleport-parity
sub-spec. 1 surfaced pre-existing patrolMode level=0 divergence vs
TS Npc.ts:729 logged in nai_followups.md (NOT counted as a NAI-34
deviation since not introduced by this sub-spec).

Smoke acceptance: fishing NPCs visibly relocate when ai_timer fires;
server log silent on OpNpcTele errors.

Closes memory: nai_followups.md "From NAI-33" entry NPC_TELE 2541.
Adds memory: nai_followups.md "From NAI-34" entries (3 follow-ups —
parity sub-spec, patrolMode level fix, NPC_WALK 2542).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

(The `--allow-empty` flag handles the case where Step 1 polish commit landed but Steps 2-3 didn't add anything else to the repo. The close commit is a chore-marker — its body documents the close, even if the diff is empty. If a polish commit was applied in Step 1, it lands separately before this close commit.)

---

## Close criteria

NAI-34 closes when ALL of these hold:

1. ✅ All Layer 1 + Layer 2 + Layer 3 tests pass via `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./...`.
2. ✅ `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...` clean.
3. ✅ Combined code-review pass identifies no critical issues (or any criticals are fixed).
4. ✅ Smoke gate observed: fishing NPC visibly relocates after `ai_timer` fires; server log silent on `OpNpcTele`.
5. ✅ `nai_followups.md` updated: 3 follow-ups added under "From NAI-34"; `NPC_TELE 2541` removed from "From NAI-33".
6. ✅ Close commit `chore(script,world): NAI-34 closed — ...` lands with `Closes memory:` trailer.

After NAI-34 closes, the visible chain `move_fishing_spot → check_fishing_spot_empty (NAI-33) → npc_tele (NAI-34) → npc_settimer` is end-to-end functional for fishing-spot relocation.
