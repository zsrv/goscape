# NAI-36 — runscript stub handlers + PathingEntity.teleport partial parity Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Wire 5 smoke-driven `protocol_stub_not_completed` script-VM handlers (NPC_WALK, NPC_GETMODE, NPC_SETMODE, SPOTANIM_MAP, MAP_BLOCKED), fix PatrolMode's level-discard bug, and partially close the NAI-34 PathingEntity.teleport divergence thread (D1+D2 for both entities, D5 for Player).

**Architecture:** Cross-package work spanning `pkg/script` (handler dispatch + interfaces) and `modules/world` (entity-method bodies + adapters). Two-theme structure: Theme A (T1-T6 = stub-not-completed ports) shares one file with Theme B (T7 = parity sweep + PatrolMode fix) at `modules/world/npc_interaction.go:121`. Sequencing: T1 lands all interface seams + queueWaypoint→QueueWaypoint rename, T2-T6 add dispatch on top of T1, T7 modifies Teleport bodies + the line-121 PatrolMode call site, T8 closes.

**Tech Stack:**
- Go 1.26+ (per `go_version.md` memory; `use-modern-go` skill).
- TS source: `Engine-TS` only (per `ts_source_canonical_path.md`).
- Spec: `docs/superpowers/specs/2026-04-27-nai-36-runscript-stub-handlers-and-teleport-parity-design.md` (commit `3844587`).

---

## Pre-flight (controller pre-flight per `controller_preflight.md`)

Before dispatching any task, the controller (whoever orchestrates this plan, whether subagent-driven or inline) verifies these premises against HEAD:

- [ ] **PF1.** `HEAD` is `61af038` or a strictly-ahead descendant. Run: `git log --oneline -1`. Expected: `61af038` or a commit on top of NAI-35 close.
- [ ] **PF2.** `pkg/script/opcode.go:81,94,259,272,281` declares `OpMapBlocked=1007`, `OpSpotAnimMap=1020`, `OpNpcGetMode=2522`, `OpNpcSetMode=2535`, `OpNpcWalk=2544`. Run: `grep -n 'OpMapBlocked\|OpSpotAnimMap\|OpNpcGetMode\|OpNpcSetMode\|OpNpcWalk' pkg/script/opcode.go | head`.
- [ ] **PF3.** `pkg/script/handlers.go` has NO entries for those 5 opcodes. Run: `grep -n 'OpMapBlocked\|OpSpotAnimMap\|OpNpcGetMode\|OpNpcSetMode\|OpNpcWalk' pkg/script/handlers.go`. Expected: only the `case` arms in `opcode.go`'s String/lookup, no map entries.
- [ ] **PF4.** `(n *Npc) queueWaypoint` exists at `modules/world/npc_ai.go:84` (lowercase, unexported). Run: `grep -n 'queueWaypoint' modules/world/npc_ai.go`.
- [ ] **PF5.** `(n *Npc) Teleport` body at `modules/world/npc_script.go:109-114` matches the spec's HEAD baseline (refresh-then-flag order; no clamp; no zone-reject). Run: `sed -n '109,114p' modules/world/npc_script.go`.
- [ ] **PF6.** `(p *Player) Teleport` body at `modules/world/player_script.go:226-233` matches spec baseline (flag-then-refresh order; no clamp; no zone-reject; no level-change branch). Run: `sed -n '226,233p' modules/world/player_script.go`.
- [ ] **PF7.** `nextPatrolTick` field exists on Npc at `modules/world/npc.go:84`. Run: `grep -n 'nextPatrolTick' modules/world/npc.go`.
- [ ] **PF8.** All tests green pre-work: Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`. Expected: all 23 packages PASS.

If ANY pre-flight check fails, STOP and re-verify the spec against HEAD before proceeding.

---

## Task 1: Foundation seams (queueWaypoint export + interface methods + adapter wiring)

**Files:**
- Modify: `modules/world/npc_ai.go:84` (rename `queueWaypoint` → `QueueWaypoint`)
- Modify: `modules/world/npc_interaction.go:87,118,135,351` (rename callers)
- Modify: `modules/world/npc_player_modes.go:167,174,176` (rename callers)
- Modify: `modules/world/npc_player_modes_test.go:284` (rename test comment)
- Modify: `pkg/script/active.go` (add `QueueWaypoint`, `TargetOp` to ActiveNpc interface; add `AnimMap` to State.World interface — actually World/WorldVars lives in `state.go`)
- Modify: `pkg/script/state.go:50-76` (add `AnimMap` to `WorldVars` interface)
- Modify: `modules/world/npc_script.go` (add `(n *Npc) QueueWaypoint` adapter and `(n *Npc) TargetOp` adapter)
- Modify: `pkg/script/handlers_vars_test.go:9-38` (add `AnimMap` no-op to mockWorld)
- Modify: `pkg/script/handlers_npc_test.go:186-202` (add `queueWaypointCalls` recorder + `targetOp int` field + matching methods on mockNpc)

**Test file:** `modules/world/npc_ai_test.go` (new) for adapter sanity.

### Step 1.1: Pre-flight grep — enumerate every `queueWaypoint` site

- [ ] **Step 1.1: Run enumeration grep**

Run: `grep -rn '\.queueWaypoint\(' modules/world/`

Expected output (8 lines including the definition):
```
modules/world/npc_ai.go:84:func (n *Npc) queueWaypoint(x, z int) {
modules/world/npc_interaction.go:87:			n.queueWaypoint(n.startX+dx, n.startZ+dz)
modules/world/npc_interaction.go:118:		n.queueWaypoint(dest.X, dest.Z)
modules/world/npc_interaction.go:135:	n.queueWaypoint(dest.X, dest.Z)
modules/world/npc_interaction.go:351:	n.queueWaypoint(tx, tz)
modules/world/npc_player_modes.go:167:		n.queueWaypoint(mx, mz)
modules/world/npc_player_modes.go:174:		n.queueWaypoint(n.x, mz)
modules/world/npc_player_modes.go:176:		n.queueWaypoint(mx, n.z)
```

If there are MORE than 8 lines (additional callers added since spec-write), expand the rename list before proceeding. If FEWER, abort and reconcile with the spec.

### Step 1.2: Rename definition

- [ ] **Step 1.2: Edit `modules/world/npc_ai.go:84`**

Change:
```go
// queueWaypoint clears any existing path and sets a single destination.
func (n *Npc) queueWaypoint(x, z int) {
	n.waypoints[0] = coordgrid.PackCoord(n.level, x, z)
	n.waypointIndex = 0
}
```

To:
```go
// QueueWaypoint clears any existing path and sets a single destination.
// Exported for use by pkg/script's ActiveNpc adapter (NAI-36).
func (n *Npc) QueueWaypoint(x, z int) {
	n.waypoints[0] = coordgrid.PackCoord(n.level, x, z)
	n.waypointIndex = 0
}
```

### Step 1.3: Rename all 7 in-package callers

- [ ] **Step 1.3a: Edit `modules/world/npc_interaction.go:87`**

Change `n.queueWaypoint(n.startX+dx, n.startZ+dz)` → `n.QueueWaypoint(n.startX+dx, n.startZ+dz)`.

- [ ] **Step 1.3b: Edit `modules/world/npc_interaction.go:118`**

Change `n.queueWaypoint(dest.X, dest.Z)` → `n.QueueWaypoint(dest.X, dest.Z)`.

- [ ] **Step 1.3c: Edit `modules/world/npc_interaction.go:135`**

Change `n.queueWaypoint(dest.X, dest.Z)` → `n.QueueWaypoint(dest.X, dest.Z)`.

- [ ] **Step 1.3d: Edit `modules/world/npc_interaction.go:351`**

Change `n.queueWaypoint(tx, tz)` → `n.QueueWaypoint(tx, tz)`.

- [ ] **Step 1.3e: Edit `modules/world/npc_player_modes.go:167`**

Change `n.queueWaypoint(mx, mz)` → `n.QueueWaypoint(mx, mz)`.

- [ ] **Step 1.3f: Edit `modules/world/npc_player_modes.go:174`**

Change `n.queueWaypoint(n.x, mz)` → `n.QueueWaypoint(n.x, mz)`.

- [ ] **Step 1.3g: Edit `modules/world/npc_player_modes.go:176`**

Change `n.queueWaypoint(mx, n.z)` → `n.QueueWaypoint(mx, n.z)`.

### Step 1.4: Rename test comment

- [ ] **Step 1.4: Edit `modules/world/npc_player_modes_test.go:284`**

Change comment text `// queueWaypoint writes waypoints[0] = ...` → `// QueueWaypoint writes waypoints[0] = ...`.

### Step 1.5: Verify rename completeness

- [ ] **Step 1.5: Confirm zero hits to lowercase form**

Run: `grep -n 'queueWaypoint' modules/world/`

Expected: ZERO hits. If any hits remain, fix them and re-run.

### Step 1.6: Compile check after rename

- [ ] **Step 1.6: Compile modules/world**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./modules/world/...`
Expected: clean build, no errors.

- [ ] **Step 1.7: Test modules/world after rename**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/...`
Expected: all tests PASS (rename is mechanically equivalent).

### Step 1.8: Add `QueueWaypoint` and `TargetOp` to `ActiveNpc` interface

- [ ] **Step 1.8: Edit `pkg/script/active.go`**

Locate the `ActiveNpc` interface (around lines 410-504). After the existing `Teleport(x, z, level int)` method, add:

```go
	// QueueWaypoint clears any existing path and sets a single destination
	// (level-implicit by current NPC level). Mirrors TS Npc.queueWaypoint
	// at Engine-TS/.../Npc.ts. Used by NPC_WALK (opcode 2544).
	QueueWaypoint(x, z int)

	// TargetOp returns the NPC's current targetOp/mode value (the field set
	// by NPC_SETMODE / interaction binding). Used by NPC_GETMODE (opcode 2522).
	TargetOp() int
```

### Step 1.9: Add `AnimMap` to `WorldVars` interface

- [ ] **Step 1.9: Edit `pkg/script/state.go`**

Locate the `WorldVars` interface (around lines 50-76). After the existing `IsFreeToPlay(x, z int) bool` method, add:

```go
	// AnimMap broadcasts a tile-anchored spotanim event to every player in
	// the affected zone. Mirrors TS World.animMap at Engine-TS/.../World.ts.
	// Used by SPOTANIM_MAP (opcode 1020).
	AnimMap(level, x, z, spotanim, height, delay int)
```

### Step 1.10: Wire `QueueWaypoint` and `TargetOp` adapters on Npc

- [ ] **Step 1.10: Edit `modules/world/npc_script.go`**

After the existing `(n *Npc) Teleport(...)` method (around line 114), add:

```go
// TargetOp returns n.targetOp. ActiveNpc interface adapter for NPC_GETMODE
// (NAI-36).
func (n *Npc) TargetOp() int {
	return n.targetOp
}
```

`QueueWaypoint` is already the renamed method from Step 1.2 — it satisfies the interface as-is (signature `(n *Npc) QueueWaypoint(x, z int)` matches the interface declaration `QueueWaypoint(x, z int)`).

### Step 1.11: Add `AnimMap` no-op to mockWorld

- [ ] **Step 1.11: Edit `pkg/script/handlers_vars_test.go`**

After the existing `IsFreeToPlay(x, z int) bool` method on `mockWorld` (around line 38), add:

```go
// NAI-36: default no-op stub for SPOTANIM_MAP test fixture. Real recording
// is layered on by handler-specific test types.
func (m *mockWorld) AnimMap(level, x, z, spotanim, height, delay int) {}
```

### Step 1.12: Add `queueWaypointCalls` recorder + `targetOp` field on mockNpc

- [ ] **Step 1.12: Edit `pkg/script/handlers_npc_test.go`**

In the `mockNpc` struct (around lines 186-202), after `teleportCalls []struct{ x, z, level int }`, add:

```go
	queueWaypointCalls []struct{ x, z int }
	targetOpField      int
```

Then add the methods (after the existing `Teleport` method on mockNpc):

```go
func (m *mockNpc) QueueWaypoint(x, z int) {
	m.queueWaypointCalls = append(m.queueWaypointCalls, struct{ x, z int }{x, z})
}

func (m *mockNpc) TargetOp() int { return m.targetOpField }
```

(The field is named `targetOpField` to avoid collision with the existing `targetOp` field on the production `*Npc` if any test code references both via embedding. If `mockNpc` has no such collision, plain `targetOp` is fine — verify at edit time.)

### Step 1.13: Run all tests

- [ ] **Step 1.13: Verify Task 1 leaves the build green**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`
Expected: all tests PASS, no compile errors anywhere. (No new behavior yet — just plumbing.)

### Step 1.14: Commit

- [ ] **Step 1.14: Commit Task 1**

```bash
git add modules/world/npc_ai.go modules/world/npc_interaction.go modules/world/npc_player_modes.go modules/world/npc_player_modes_test.go modules/world/npc_script.go pkg/script/active.go pkg/script/state.go pkg/script/handlers_vars_test.go pkg/script/handlers_npc_test.go
git commit --no-gpg-sign -m "refactor(world,script): NAI-36 T1 — foundation seams (QueueWaypoint export + ActiveNpc/WorldVars interface methods)

Rename (n *Npc) queueWaypoint → QueueWaypoint across 8 sites (definition +
7 in-package callers + 1 test comment) per enumerate_all_sites.md.

Add ActiveNpc.QueueWaypoint and ActiveNpc.TargetOp methods + matching Npc
adapters; add WorldVars.AnimMap interface method + mockWorld no-op for
T2-T6 dispatch wiring. No new behavior — pure plumbing for the 5 stub
ports to plug into."
```

---

## Task 2: NPC_WALK (opcode 2544) handler

**Files:**
- Modify: `pkg/script/handlers_npc.go` (add `handleNpcWalk` after `handleNpcTele`)
- Modify: `pkg/script/handlers.go` (register `OpNpcWalk: handleNpcWalk`)
- Test: `pkg/script/handlers_npc_test.go` (add 3 tests)

### Step 2.1: Write failing test — pop+validate+delegate

- [ ] **Step 2.1: Edit `pkg/script/handlers_npc_test.go`**

After `TestNpcTele_NoActiveNpcErrors` (around line 2070), add:

```go
// --- NAI-36 Task 2: NPC_WALK Layer 1 unit tests --------------------------

func TestNpcWalk_PopsCoordValidatesAndDelegates(t *testing.T) {
	npc := &mockNpc{}
	mc := &mockConfigs{}

	// coord pack(level=2, x=3200, z=3300)
	const level, x, z = 2, 3200, 3300
	coord := (level << 28) | (x << 14) | z

	state := runNpcOp(t, npc, mc, OpNpcWalk, []int{coord})
	_ = state

	if len(npc.queueWaypointCalls) != 1 {
		t.Fatalf("queueWaypointCalls: got %d, want 1", len(npc.queueWaypointCalls))
	}
	got := npc.queueWaypointCalls[0]
	if got.x != x || got.z != z {
		t.Errorf("queueWaypointCalls[0]: got (x=%d, z=%d), want (x=%d, z=%d)",
			got.x, got.z, x, z)
	}
}

// TS-asymmetry pin per ts_asymmetry_dual_pin.md — pin presence (QueueWaypoint
// called with x/z) AND conspicuous absence (level discarded TS-faithfully —
// no Teleport call, no level path). Escalates if upstream TS adds a level
// argument to NPC_WALK in a future fix.
func TestNpcWalk_DiscardsLevelTSFaithfully(t *testing.T) {
	npc := &mockNpc{}
	mc := &mockConfigs{}

	// coord pack(level=3, x=3200, z=3300) — non-zero level
	const x, z = 3200, 3300
	coord := (3 << 28) | (x << 14) | z

	_ = runNpcOp(t, npc, mc, OpNpcWalk, []int{coord})

	// Presence: QueueWaypoint called.
	if len(npc.queueWaypointCalls) != 1 {
		t.Fatalf("queueWaypointCalls: got %d, want 1", len(npc.queueWaypointCalls))
	}
	// Conspicuous absence: Teleport NOT called (no 3-arg level path).
	if len(npc.teleportCalls) != 0 {
		t.Errorf("teleportCalls: got %d, want 0 (NPC_WALK must not Teleport — level is dropped TS-faithfully)",
			len(npc.teleportCalls))
	}
}

func TestNpcWalk_NoActiveNpcErrors(t *testing.T) {
	state := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	state.PushInt((1 << 28) | (3200 << 14) | 3300)

	err := handleNpcWalk(state)
	if err == nil || !strings.Contains(err.Error(), "no active npc") {
		t.Errorf("handleNpcWalk with no active npc: got %v, want error containing 'no active npc'", err)
	}
}
```

### Step 2.2: Run failing test

- [ ] **Step 2.2: Confirm test fails (handler undefined)**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run 'TestNpcWalk' ./pkg/script/...`
Expected: FAIL with `undefined: handleNpcWalk` or compile error.

### Step 2.3: Implement `handleNpcWalk`

- [ ] **Step 2.3: Edit `pkg/script/handlers_npc.go`**

After the existing `handleNpcTele` (around line 364), add:

```go
// handleNpcWalk (NPC_WALK, opcode 2544) queues a single waypoint for the
// active NPC at the unpacked coord. Pop order: coord (single int). Mirrors
// TS NpcOps.ts:451-455 — checkedHandler(ActiveNpc) + CoordValid +
// activeNpc.queueWaypoint(x, z). NOTE: level is dropped TS-faithfully; the
// waypoint uses the NPC's current level by convention.
func handleNpcWalk(s *ScriptState) error {
	if err := requireActiveNpc(s, "NPC_WALK"); err != nil {
		return err
	}
	coord := s.PopInt()
	_, x, z, err := checkCoord(coord, "NPC_WALK")
	if err != nil {
		return err
	}
	s.ActiveNpc.QueueWaypoint(x, z)
	return nil
}
```

### Step 2.4: Register in handlers.go

- [ ] **Step 2.4: Edit `pkg/script/handlers.go`**

Locate the existing `OpNpcTele: handleNpcTele,` registration. Add a sibling line (alphabetical or numerical placement — match existing convention):

```go
	OpNpcWalk:              handleNpcWalk,
```

### Step 2.5: Run tests — should pass

- [ ] **Step 2.5: Confirm test passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run 'TestNpcWalk' ./pkg/script/...`
Expected: PASS for all 3 sub-tests.

### Step 2.6: Run full pkg/script test suite

- [ ] **Step 2.6: Regression check**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/...`
Expected: all PASS.

### Step 2.7: Commit

- [ ] **Step 2.7: Commit Task 2**

```bash
git add pkg/script/handlers_npc.go pkg/script/handlers.go pkg/script/handlers_npc_test.go
git commit --no-gpg-sign -m "feat(script): NAI-36 T2 — NPC_WALK handler (opcode 2544)

Mirrors TS NpcOps.ts:451-455 — pop coord, validate, delegate to
ActiveNpc.QueueWaypoint(x, z) with level dropped TS-faithfully.
TS-asymmetry dual-pin per ts_asymmetry_dual_pin.md: presence test +
conspicuous-absence test (no Teleport call)."
```

---

## Task 3: NPC_GETMODE (opcode 2522) handler

**Files:**
- Modify: `pkg/script/handlers_npc.go` (add `handleNpcGetMode`)
- Modify: `pkg/script/handlers.go` (register `OpNpcGetMode`)
- Test: `pkg/script/handlers_npc_test.go` (add 2 tests)

### Step 3.1: Write failing test

- [ ] **Step 3.1: Edit `pkg/script/handlers_npc_test.go`**

Append:

```go
// --- NAI-36 Task 3: NPC_GETMODE Layer 1 unit tests -----------------------

func TestNpcGetMode_PushesTargetOp(t *testing.T) {
	npc := &mockNpc{targetOpField: 5} // NPCModePlayerFace per pkg/objtype constants
	mc := &mockConfigs{}

	state := runNpcOp(t, npc, mc, OpNpcGetMode, nil)

	if state.ISP != 1 {
		t.Fatalf("ISP after NPC_GETMODE: got %d, want 1 (one push)", state.ISP)
	}
	got := state.IntStack[0]
	if got != 5 {
		t.Errorf("pushed value: got %d, want 5", got)
	}
}

func TestNpcGetMode_NoActiveNpcErrors(t *testing.T) {
	state := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	err := handleNpcGetMode(state)
	if err == nil || !strings.Contains(err.Error(), "no active npc") {
		t.Errorf("handleNpcGetMode with no active npc: got %v, want error containing 'no active npc'", err)
	}
}
```

### Step 3.2: Run failing test

- [ ] **Step 3.2: Confirm fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run 'TestNpcGetMode' ./pkg/script/...`
Expected: FAIL with `undefined: handleNpcGetMode`.

### Step 3.3: Implement `handleNpcGetMode`

- [ ] **Step 3.3: Edit `pkg/script/handlers_npc.go`**

After `handleNpcWalk`, add:

```go
// handleNpcGetMode (NPC_GETMODE, opcode 2522) pushes the active NPC's
// targetOp value (the mode set by NPC_SETMODE / interaction binding).
// Mirrors TS NpcOps.ts:473-475 — checkedHandler(ActiveNpc) + pushInt.
func handleNpcGetMode(s *ScriptState) error {
	if err := requireActiveNpc(s, "NPC_GETMODE"); err != nil {
		return err
	}
	s.PushInt(s.ActiveNpc.TargetOp())
	return nil
}
```

### Step 3.4: Register

- [ ] **Step 3.4: Edit `pkg/script/handlers.go`**

Add (sibling of `OpNpcTele` / `OpNpcWalk`):

```go
	OpNpcGetMode:           handleNpcGetMode,
```

### Step 3.5: Run tests — should pass

- [ ] **Step 3.5: Confirm passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run 'TestNpcGetMode' ./pkg/script/...`
Expected: PASS.

### Step 3.6: Commit

- [ ] **Step 3.6: Commit Task 3**

```bash
git add pkg/script/handlers_npc.go pkg/script/handlers.go pkg/script/handlers_npc_test.go
git commit --no-gpg-sign -m "feat(script): NAI-36 T3 — NPC_GETMODE handler (opcode 2522)

Mirrors TS NpcOps.ts:473-475 — push state.activeNpc.targetOp."
```

---

## Task 4: MAP_BLOCKED (opcode 1007) handler

**Files:**
- Modify: `pkg/script/handlers_map.go` (add `handleMapBlocked`)
- Modify: `pkg/script/handlers.go` (register `OpMapBlocked`)
- Test: `pkg/script/handlers_map_test.go` (add 4 tests)

**Note:** The handler home is `handlers_map.go` (it already exists per spec; it hosts `handleMapFindSquare` from NAI-35). If for some reason it's `handlers_server.go` instead at HEAD, place there and update commit message accordingly.

### Step 4.1: Pre-flight verify handler home

- [ ] **Step 4.1: Confirm `handlers_map.go` exists**

Run: `ls pkg/script/handlers_map.go pkg/script/handlers_server.go 2>&1`

Expected: at least one of these exists. Place new handler in whichever the existing `handleMapPlayerCount` / `handleMapFindSquare` already live in. If neither file exists, fall back to extending `handlers_server.go` (create it if needed).

### Step 4.2: Write failing tests

- [ ] **Step 4.2: Edit `pkg/script/handlers_map_test.go`**

Append:

```go
// --- NAI-36 Task 4: MAP_BLOCKED Layer 1 unit tests -----------------------

// mapBlockedWorld extends mockWorld with controllable IsMapBlocked +
// IsFreeToPlay return values for the 4-branch coverage.
type mapBlockedWorld struct {
	mockWorld
	mapBlocked bool
	freeToPlay bool
}

func (w *mapBlockedWorld) IsMapBlocked(level, x, z int) bool { return w.mapBlocked }
func (w *mapBlockedWorld) IsFreeToPlay(x, z int) bool        { return w.freeToPlay }

func TestMapBlocked_MembersWorldClearTilePushes0(t *testing.T) {
	w := &mapBlockedWorld{mockWorld: mockWorld{mapMembers: 1}, mapBlocked: false}
	state := runMapOp(t, w, OpMapBlocked, []int{(0 << 28) | (3200 << 14) | 3300})

	if state.ISP != 1 || state.IntStack[0] != 0 {
		t.Errorf("members-world clear tile: got top=%d ISP=%d, want top=0 ISP=1",
			state.IntStack[0], state.ISP)
	}
}

func TestMapBlocked_MembersWorldBlockedTilePushes1(t *testing.T) {
	w := &mapBlockedWorld{mockWorld: mockWorld{mapMembers: 1}, mapBlocked: true}
	state := runMapOp(t, w, OpMapBlocked, []int{(0 << 28) | (3200 << 14) | 3300})

	if state.ISP != 1 || state.IntStack[0] != 1 {
		t.Errorf("members-world blocked tile: got top=%d ISP=%d, want top=1 ISP=1",
			state.IntStack[0], state.ISP)
	}
}

// F2P-world non-F2P tile: short-circuits to push 1 BEFORE the IsMapBlocked
// check. Tests the early-return per TS ServerOps.ts:132-135.
func TestMapBlocked_F2PWorldNonF2PTilePushes1(t *testing.T) {
	w := &mapBlockedWorld{
		mockWorld:  mockWorld{mapMembers: 0}, // F2P world
		mapBlocked: false,                    // would push 0 if reached
		freeToPlay: false,                    // tile is NOT F2P
	}
	state := runMapOp(t, w, OpMapBlocked, []int{(0 << 28) | (3200 << 14) | 3300})

	if state.ISP != 1 || state.IntStack[0] != 1 {
		t.Errorf("F2P-world non-F2P tile: got top=%d ISP=%d, want top=1 ISP=1 (short-circuit)",
			state.IntStack[0], state.ISP)
	}
}

// F2P-world F2P tile: passes the gate; falls through to IsMapBlocked.
func TestMapBlocked_F2PWorldF2PTilePushesIsBlocked(t *testing.T) {
	w := &mapBlockedWorld{
		mockWorld:  mockWorld{mapMembers: 0}, // F2P world
		mapBlocked: true,
		freeToPlay: true, // tile IS F2P
	}
	state := runMapOp(t, w, OpMapBlocked, []int{(0 << 28) | (3200 << 14) | 3300})

	if state.ISP != 1 || state.IntStack[0] != 1 {
		t.Errorf("F2P-world F2P-blocked tile: got top=%d ISP=%d, want top=1 ISP=1",
			state.IntStack[0], state.ISP)
	}
}
```

**Note:** `runMapOp` may already exist (NAI-35 added it for MAP_FINDSQUARE) — if so, reuse. If not, add a parallel helper modeled on `runNpcOp`:

```go
// runMapOp executes a single map opcode against the given world fixture
// and returns the post-execution state.
func runMapOp(t *testing.T, w WorldVars, op Opcode, intInputs []int) *ScriptState {
	t.Helper()
	sf := &ScriptFile{
		Name:             "test_" + op.String(),
		Opcodes:          []Opcode{op, OpReturn},
		IntOperands:      []int32{0, 0},
		StringOperands:   []string{"", ""},
		InstructionCount: 2,
	}
	state := &ScriptState{
		Script:      sf,
		World:       w,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	for _, v := range intInputs {
		state.PushInt(v)
	}
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	return state
}
```

### Step 4.3: Run failing test

- [ ] **Step 4.3: Confirm fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run 'TestMapBlocked' ./pkg/script/...`
Expected: FAIL with `undefined: handleMapBlocked` (or compile error if `runMapOp` was new).

### Step 4.4: Implement `handleMapBlocked`

- [ ] **Step 4.4: Edit `pkg/script/handlers_map.go`** (or `handlers_server.go` per Step 4.1)

Add:

```go
// handleMapBlocked (MAP_BLOCKED, opcode 1007) reports whether the tile at
// the unpacked coord blocks walking. F2P-world short-circuit: any tile
// that's not F2P-zoned pushes 1 (effectively "blocked" for non-members
// content). Mirrors TS ServerOps.ts:129-138.
func handleMapBlocked(s *ScriptState) error {
	coord := s.PopInt()
	level, x, z, err := checkCoord(coord, "MAP_BLOCKED")
	if err != nil {
		return err
	}
	// F2P-world gate: !NODE_MEMBERS && !isFreeToPlay → push 1
	if s.World.MapMembers() == 0 && !s.World.IsFreeToPlay(x, z) {
		s.PushInt(1)
		return nil
	}
	if s.World.IsMapBlocked(level, x, z) {
		s.PushInt(1)
	} else {
		s.PushInt(0)
	}
	return nil
}
```

### Step 4.5: Register

- [ ] **Step 4.5: Edit `pkg/script/handlers.go`**

Add:

```go
	OpMapBlocked:           handleMapBlocked,
```

### Step 4.6: Run tests — should pass

- [ ] **Step 4.6: Confirm passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run 'TestMapBlocked' ./pkg/script/...`
Expected: PASS for all 4 sub-tests.

### Step 4.7: Commit

- [ ] **Step 4.7: Commit Task 4**

```bash
git add pkg/script/handlers_map.go pkg/script/handlers.go pkg/script/handlers_map_test.go
git commit --no-gpg-sign -m "feat(script): NAI-36 T4 — MAP_BLOCKED handler (opcode 1007)

Mirrors TS ServerOps.ts:129-138 — F2P-world short-circuit (push 1 if
tile not F2P) + IsMapBlocked push. Reuses NAI-35-T6 World interface
methods (IsMapBlocked, IsFreeToPlay, MapMembers); no new infrastructure."
```

---

## Task 5: SPOTANIM_MAP (opcode 1020) handler

**Files:**
- Modify: `pkg/script/handlers_map.go` or `handlers_server.go` (add `checkSpotAnimType` + `handleSpotAnimMap`)
- Modify: `pkg/script/handlers.go` (register `OpSpotAnimMap`)
- Modify: `modules/world/world_zone.go` — verify Server.AnimMap signature already matches the interface added in T1
- Modify: `modules/world/server.go` (or wherever `s.worldVars` is declared) — verify the worldVars struct used to satisfy WorldVars; add an `AnimMap` shim if `worldVars` is a separate struct from `Server`
- Test: `pkg/script/handlers_map_test.go` (add 4 tests)

### Step 5.1: Pre-flight verify worldVars structure

- [ ] **Step 5.1: Identify the production WorldVars implementation**

Run: `grep -rn 'WorldVars\|worldVars' modules/world/ | head -20`

The script-side `state.World` is set somewhere via `state.World = s.worldVars` (or similar). Identify the concrete type and confirm its method-set. Two cases:
- **Case A:** `worldVars` is `*Server` directly. T1 already added `AnimMap` interface; Server.AnimMap already exists at `world_zone.go:76` — interface is satisfied automatically.
- **Case B:** `worldVars` is a distinct struct that delegates. Add a delegating method:
  ```go
  func (w *worldVars) AnimMap(level, x, z, spotanim, height, delay int) {
      w.server.AnimMap(level, x, z, spotanim, height, delay)
  }
  ```

Confirm which case applies. Adjust the rest of T5 accordingly.

### Step 5.2: Write failing tests

- [ ] **Step 5.2: Edit `pkg/script/handlers_map_test.go`**

Append:

```go
// --- NAI-36 Task 5: SPOTANIM_MAP Layer 1 unit tests ----------------------

type spotAnimMapWorld struct {
	mockWorld
	animMapCalls []struct {
		level, x, z, spotanim, height, delay int
	}
}

func (w *spotAnimMapWorld) AnimMap(level, x, z, spotanim, height, delay int) {
	w.animMapCalls = append(w.animMapCalls, struct {
		level, x, z, spotanim, height, delay int
	}{level, x, z, spotanim, height, delay})
}

func TestSpotAnimMap_PopsValidatesAndDelegates(t *testing.T) {
	w := &spotAnimMapWorld{}

	const spotanim, height, delay = 200, 50, 5
	const level, x, z = 0, 3200, 3300
	coord := (level << 28) | (x << 14) | z

	// Pop order (TS): popInts(4) → spotanim, coord, height, delay (top-down).
	// Push reversed for stack order: delay first, height, coord, spotanim last.
	state := runMapOp(t, w, OpSpotAnimMap, []int{spotanim, coord, height, delay})
	_ = state

	if len(w.animMapCalls) != 1 {
		t.Fatalf("animMapCalls: got %d, want 1", len(w.animMapCalls))
	}
	got := w.animMapCalls[0]
	want := struct {
		level, x, z, spotanim, height, delay int
	}{level, x, z, spotanim, height, delay}
	if got != want {
		t.Errorf("animMapCalls[0]: got %+v, want %+v", got, want)
	}
}

func TestSpotAnimMap_InvalidCoordErrors(t *testing.T) {
	w := &spotAnimMapWorld{}
	state := &ScriptState{
		World:       w,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	// Push 4 ints with an out-of-range coord (-1).
	state.PushInt(200)
	state.PushInt(-1) // invalid coord
	state.PushInt(50)
	state.PushInt(5)

	err := handleSpotAnimMap(state)
	if err == nil || !strings.Contains(err.Error(), "SPOTANIM_MAP") {
		t.Errorf("invalid coord: got %v, want SPOTANIM_MAP error", err)
	}
	if len(w.animMapCalls) != 0 {
		t.Errorf("animMapCalls on error path: got %d, want 0", len(w.animMapCalls))
	}
}

// NAI-36-D2: SpotAnimType config-port absent at HEAD 61af038. Falling back
// to range-validation (id < 0 reject). Pin the divergence with a test that
// expects -1 to error and a high id to PASS (no upper bound at goscape).
func TestSpotAnimMap_NegativeSpotanimIDErrors(t *testing.T) {
	w := &spotAnimMapWorld{}
	state := &ScriptState{
		World:       w,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	state.PushInt(-1) // invalid spotanim id
	state.PushInt((0 << 28) | (3200 << 14) | 3300)
	state.PushInt(50)
	state.PushInt(5)

	err := handleSpotAnimMap(state)
	if err == nil || !strings.Contains(err.Error(), "SPOTANIM_MAP") {
		t.Errorf("negative spotanim id: got %v, want SPOTANIM_MAP error", err)
	}
}

func TestSpotAnimMap_ZeroDelayPassesThrough(t *testing.T) {
	w := &spotAnimMapWorld{}

	const spotanim, height, delay = 200, 0, 0
	coord := (0 << 28) | (3200 << 14) | 3300

	_ = runMapOp(t, w, OpSpotAnimMap, []int{spotanim, coord, height, delay})

	if len(w.animMapCalls) != 1 {
		t.Fatalf("animMapCalls: got %d, want 1", len(w.animMapCalls))
	}
	got := w.animMapCalls[0]
	if got.height != 0 || got.delay != 0 {
		t.Errorf("zero height/delay: got height=%d delay=%d, want 0/0",
			got.height, got.delay)
	}
}
```

### Step 5.3: Run failing test

- [ ] **Step 5.3: Confirm fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run 'TestSpotAnimMap' ./pkg/script/...`
Expected: FAIL with `undefined: handleSpotAnimMap`.

### Step 5.4: Implement `checkSpotAnimType` + `handleSpotAnimMap`

- [ ] **Step 5.4: Edit `pkg/script/handlers_map.go`** (or wherever `handleMapBlocked` was placed)

Add:

```go
// checkSpotAnimType validates a spotanim type id. Per NAI-36-D2: full
// SpotAnimType config-port is absent at HEAD 61af038; fall back to range
// validation (id < 0 rejected). When a SpotAnimType config accessor lands
// on the Configs interface, this helper should be tightened to mirror TS
// SpotAnimTypeValid (presence check against config table).
func checkSpotAnimType(id int, op string) error {
	if id < 0 {
		return fmt.Errorf("%s: invalid spotanim id (%d)", op, id)
	}
	return nil
}

// handleSpotAnimMap (SPOTANIM_MAP, opcode 1020) broadcasts a tile-anchored
// spotanim event at the unpacked coord. Pop order: 4 ints (spotanim, coord,
// height, delay) — TS uses popInts(4) which destructures top-down.
// Mirrors TS ServerOps.ts:84-90.
func handleSpotAnimMap(s *ScriptState) error {
	delay := s.PopInt()
	height := s.PopInt()
	coord := s.PopInt()
	spotanim := s.PopInt()

	level, x, z, err := checkCoord(coord, "SPOTANIM_MAP")
	if err != nil {
		return err
	}
	if err := checkSpotAnimType(spotanim, "SPOTANIM_MAP"); err != nil {
		return err
	}
	s.World.AnimMap(level, x, z, spotanim, height, delay)
	return nil
}
```

**Pop-order verification:** TS uses `const [spotanim, coord, height, delay] = state.popInts(4);` — `popInts(N)` destructures from a sliding window of N items off the top of stack, in declaration order of the destructure (NOT reverse). Goscape's `s.PopInt()` returns the top item; calling N times pops in TOP-DOWN order. So calling `delay := s.PopInt()` first, then `height`, then `coord`, then `spotanim` matches TS popInts(4) destructure as `[spotanim=stack[-4], coord=stack[-3], height=stack[-2], delay=stack[-1]]`. Confirm against an existing 4-int handler before committing.

### Step 5.5: Register

- [ ] **Step 5.5: Edit `pkg/script/handlers.go`**

Add:

```go
	OpSpotAnimMap:          handleSpotAnimMap,
```

### Step 5.6: Run tests — should pass

- [ ] **Step 5.6: Confirm passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run 'TestSpotAnimMap' ./pkg/script/...`
Expected: PASS for all 4 sub-tests.

### Step 5.7: Commit

- [ ] **Step 5.7: Commit Task 5**

```bash
git add pkg/script/handlers_map.go pkg/script/handlers.go pkg/script/handlers_map_test.go
git commit --no-gpg-sign -m "feat(script): NAI-36 T5 — SPOTANIM_MAP handler (opcode 1020)

Mirrors TS ServerOps.ts:84-90 — popInts(4) [spotanim, coord, height,
delay], validate, delegate to World.AnimMap. Reuses NAI-36-T1 AnimMap
seam on World interface. checkSpotAnimType range-only per NAI-36-D2
(SpotAnimType config-port absent at HEAD)."
```

---

## Stage 1 review checkpoint (T1-T5)

After T5 commits, dispatch combined-review subagent:

> **Stage 1 review prompt:** Review commits since `3844587` (NAI-36 spec commit) on the current branch. Scope: NAI-36 Tasks 1-5 — foundation seams + 4 simpler stub-not-completed handler ports (NPC_WALK, NPC_GETMODE, MAP_BLOCKED, SPOTANIM_MAP). Verify against the spec at `docs/superpowers/specs/2026-04-27-nai-36-runscript-stub-handlers-and-teleport-parity-design.md`. Critical checks: (a) `queueWaypoint→QueueWaypoint` rename complete (zero hits to lowercase); (b) ActiveNpc.QueueWaypoint, ActiveNpc.TargetOp, WorldVars.AnimMap interface methods correctly added + adapter-side wiring intact; (c) all 4 handlers mirror their TS shapes accurately; (d) NPC_WALK TS-asymmetry dual-pin tests are present (presence + conspicuous absence); (e) MAP_BLOCKED 4-branch coverage exhaustive; (f) SPOTANIM_MAP pop-order matches TS popInts(4) destructure; (g) NAI-36-D2 (SpotAnimType range-only fallback) tracked in code comments. Report: critical (blocks T6 dispatch), high-priority, low-priority, or NOOP.

If review surfaces Critical issues, fix before proceeding to T6.

---

## Task 6: NPC_SETMODE (opcode 2535) handler

**Files:**
- Modify: `pkg/script/checks.go` (or inline at `handlers_npc.go`) — add `checkNpcMode`
- Modify: `pkg/script/active.go` — add `SetInteractionScript` to ActiveNpc interface
- Modify: `pkg/script/handlers_npc.go` — add `handleNpcSetMode`
- Modify: `pkg/script/handlers.go` — register `OpNpcSetMode`
- Modify: `modules/world/npc_interaction.go` — add `(n *Npc) clearPatrol` method
- Modify: `modules/world/npc_script.go` — add `(n *Npc) SetInteractionScript` adapter (with type-switch on script.Active* values)
- Modify: `pkg/script/handlers_npc_test.go` — add 7+ branch-coverage tests
- Modify: `pkg/script/handlers_npc_test.go` — extend mockNpc with `setInteractionScriptCalls` recorder, `clearInteractionCalls`, `resetDefaultsCalls`, `clearPatrolCalls` recorders

### Step 6.1: Pre-flight — verify NPCMode constants and PtrActiveNpc2 wiring

- [ ] **Step 6.1: Verify NPCMode constant inventory**

Run: `grep -n 'NPCMode' pkg/objtype/npctype.go | head -40`

Expected: full enum (Null=-1, None=0, Wander=1, Patrol=2, PlayerEscape=3, PlayerFollow=4, PlayerFace=5, PlayerFaceClose=6, OpPlayer1..5=7..11, ApPlayer1..5=12..16, OpLoc1..5=17..21, ApLoc1..5=22..26, OpObj1..5=27..31, ApObj1..5=32..36, OpNpc1..5=37..41, ApNpc1..5=42..46). If any constant is missing, abort and add it before T6.

### Step 6.2: Add `clearInteraction` / `resetDefaults` / `clearPatrol` recorders to mockNpc

- [ ] **Step 6.2: Edit `pkg/script/handlers_npc_test.go`**

In the `mockNpc` struct, append:

```go
	clearInteractionCalls            int
	resetDefaultsCalls               int
	clearPatrolCalls                 int
	setInteractionScriptCalls        []struct {
		target any
		mode   int
	}
```

Then add the matching methods (after the existing methods on mockNpc):

```go
func (m *mockNpc) ClearInteraction() { m.clearInteractionCalls++ }
func (m *mockNpc) ResetDefaults()    { m.resetDefaultsCalls++ }
func (m *mockNpc) ClearPatrol()      { m.clearPatrolCalls++ }
func (m *mockNpc) SetInteractionScript(target any, mode int) {
	m.setInteractionScriptCalls = append(m.setInteractionScriptCalls, struct {
		target any
		mode   int
	}{target, mode})
}
```

**Note on interface signature:** the spec says ActiveNpc gains `SetInteractionScript(target any, mode int)`, plus exposes `ClearInteraction()`, `ResetDefaults()`, `ClearPatrol()` for the dispatch branches (these are already methods on the production `*Npc` per `modules/world/npc_interaction.go`; the adapter just exposes them via the interface).

### Step 6.3: Add interface methods to ActiveNpc

- [ ] **Step 6.3: Edit `pkg/script/active.go`**

In the `ActiveNpc` interface, after `TargetOp() int` (added in T1), append:

```go
	// ClearInteraction clears the NPC's current interaction binding.
	// Mirrors TS PathingEntity.clearInteraction. Used by NPC_SETMODE
	// clear-target branch (NAI-36).
	ClearInteraction()

	// ResetDefaults reverts the NPC to defaultMode + clears interaction
	// + emits faceEntity reset mask. Mirrors TS Npc.resetDefaults. Used
	// by NPC_SETMODE NULL-mode + no-target-fallthrough branches (NAI-36).
	ResetDefaults()

	// ClearPatrol resets nextPatrolTick to -1. Mirrors TS Npc.clearPatrol
	// at Engine-TS/.../Npc.ts:377-379. Used by NPC_SETMODE PATROL branch
	// (NAI-36).
	ClearPatrol()

	// SetTargetOp sets n.targetOp directly (no interaction binding). Used
	// by NPC_SETMODE clear-target and target-binding branches that assign
	// targetOp before the interaction call. Mirrors TS direct property
	// write `state.activeNpc.targetOp = mode` at NpcOps.ts:196,205.
	SetTargetOp(mode int)

	// SetInteractionScript binds the NPC's interaction to target with mode
	// as the targetOp, using Interaction.SCRIPT. Mirrors TS
	// Npc.setInteraction(Interaction.SCRIPT, target, mode) at NpcOps.ts:225-228.
	// target is one of: ActivePlayer, ActiveNpc, ActiveLoc, ActiveObj
	// (script-side interfaces). Adapter type-switches on the underlying
	// concrete world-side entity. Pass nil to no-op (caller handles
	// null-target as resetDefaults).
	SetInteractionScript(target any, mode int)
```

### Step 6.4: Add the matching mock methods + setTargetOp recorder

- [ ] **Step 6.4: Edit `pkg/script/handlers_npc_test.go` mockNpc**

Add field `setTargetOpCalls []int` to mockNpc, plus method:

```go
func (m *mockNpc) SetTargetOp(mode int) {
	m.targetOpField = mode
	m.setTargetOpCalls = append(m.setTargetOpCalls, mode)
}
```

### Step 6.5: Wire production adapters on `*Npc`

- [ ] **Step 6.5a: Edit `modules/world/npc_interaction.go` — add `clearPatrol` method**

After the `(n *Npc) defaultMode()` method (around line 696), add:

```go
// clearPatrol resets the patrol-tick countdown so the NPC immediately
// resumes patrol-pathing on the next tick. Mirrors TS Npc.clearPatrol at
// Engine-TS/.../Npc.ts:377-379.
//
// Called by NPC_SETMODE handler when the new mode is PATROL (NAI-36).
func (n *Npc) clearPatrol() {
	n.nextPatrolTick = -1
}
```

**Capitalization note:** lowercase `clearPatrol` matches the world-package internal convention (other methods like `resetDefaults`, `clearInteraction` are already lowercase). The exported wrapper for the script-side ActiveNpc interface goes through a separate `ClearPatrol` method (Step 6.5b).

- [ ] **Step 6.5b: Edit `modules/world/npc_script.go` — add ActiveNpc adapter methods**

After the existing `(n *Npc) TargetOp()` method (added in T1 Step 1.10), add:

```go
// ClearInteraction is the ActiveNpc-interface adapter for n.clearInteraction
// (NAI-36). Production caller is the NPC_SETMODE script handler.
func (n *Npc) ClearInteraction() {
	n.clearInteraction()
}

// ResetDefaults is the ActiveNpc-interface adapter for n.resetDefaults
// (NAI-36). Production caller is the NPC_SETMODE script handler.
func (n *Npc) ResetDefaults() {
	n.resetDefaults()
}

// ClearPatrol is the ActiveNpc-interface adapter for n.clearPatrol
// (NAI-36). Production caller is the NPC_SETMODE script handler when
// the new mode is PATROL.
func (n *Npc) ClearPatrol() {
	n.clearPatrol()
}

// SetTargetOp sets n.targetOp directly. ActiveNpc-interface adapter.
// Used by NPC_SETMODE handler for both clear-target and target-binding
// branches (NAI-36).
func (n *Npc) SetTargetOp(mode int) {
	n.targetOp = mode
}

// SetInteractionScript binds the NPC's interaction to target via
// InteractionScript with mode as the targetOp/op argument. Type-switches
// the script-side script.Active* interface value to the underlying
// world-side concrete entity, then delegates to n.SetInteraction. Mirrors
// TS Npc.setInteraction(Interaction.SCRIPT, target, mode) at NpcOps.ts:225-228.
//
// Passing nil is valid: the caller (NPC_SETMODE handler) calls resetDefaults
// instead of this method when the resolved target is nil.
func (n *Npc) SetInteractionScript(target any, mode int) {
	var ent entity
	switch t := target.(type) {
	case *Player:
		ent = t
	case *Npc:
		ent = t
	case *entitypkg.Loc:
		ent = t
	case *entitypkg.Obj:
		ent = t
	default:
		// Should not happen — script-side resolution narrows to one of
		// the four concrete world-side types via interface dispatch.
		// If the type-switch misses, log + no-op rather than panic so
		// production stays alive.
		return
	}
	// SetInteraction(kind, target, op, com): TS calls setInteraction(SCRIPT,
	// target, mode) → goscape's 4-arg signature with op=mode, com=-1
	// (TS sentinel for "no com").
	n.SetInteraction(InteractionScript, ent, mode, -1)
}
```

**Note:** the import for `entitypkg` may need to be added/verified — check the file's existing imports against the production `*Npc.SetInteraction` body (which already imports `entitypkg`).

### Step 6.6: Add `checkNpcMode` helper

- [ ] **Step 6.6: Edit `pkg/script/checks.go` (or inline at `handlers_npc.go`)**

Locate where `checkNotNull`, `checkCoord` live (per recon: `handlers_player.go:61` for checkNotNull, `handlers_npc.go:11` for checkCoord — there's no central `checks.go`). Place `checkNpcMode` near `checkCoord` in `handlers_npc.go`.

Add (after `checkCoord`):

```go
// checkNpcMode validates an NPC mode value against the full NPCMode* enum
// at pkg/objtype/npctype.go. Accepts every declared value (Null=-1 through
// ApNpc5=46). Mirrors TS NpcModeValid (ScriptValidators.ts) — same range,
// no enum-table dispatch.
func checkNpcMode(mode int, op string) error {
	if mode < objtype.NPCModeNull || mode > objtype.NPCModeApNpc5 {
		return fmt.Errorf("%s: invalid npc mode (%d)", op, mode)
	}
	return nil
}
```

If the file's import block doesn't already include `pkg/objtype`, add it.

### Step 6.7: Write failing tests for all NPC_SETMODE branches

- [ ] **Step 6.7: Edit `pkg/script/handlers_npc_test.go`**

Append:

```go
// --- NAI-36 Task 6: NPC_SETMODE Layer 1 unit tests -----------------------

func TestNpcSetMode_ModeNoneClearsInteractionAndSetsOp(t *testing.T) {
	npc := &mockNpc{}
	mc := &mockConfigs{}

	state := runNpcOp(t, npc, mc, OpNpcSetMode, []int{int(objtype.NPCModeNone)})
	_ = state

	if npc.clearInteractionCalls != 1 {
		t.Errorf("clearInteractionCalls: got %d, want 1", npc.clearInteractionCalls)
	}
	if len(npc.setTargetOpCalls) != 1 || npc.setTargetOpCalls[0] != int(objtype.NPCModeNone) {
		t.Errorf("setTargetOpCalls: got %v, want [NPCModeNone]", npc.setTargetOpCalls)
	}
	if npc.clearPatrolCalls != 0 {
		t.Errorf("clearPatrolCalls: got %d, want 0 (only PATROL mode triggers clearPatrol)", npc.clearPatrolCalls)
	}
	if len(npc.setInteractionScriptCalls) != 0 {
		t.Errorf("setInteractionScriptCalls: got %d, want 0 (clear-target branch must not bind)", len(npc.setInteractionScriptCalls))
	}
}

func TestNpcSetMode_ModeWanderClearsInteractionAndSetsOp(t *testing.T) {
	npc := &mockNpc{}
	mc := &mockConfigs{}

	_ = runNpcOp(t, npc, mc, OpNpcSetMode, []int{int(objtype.NPCModeWander)})

	if npc.clearInteractionCalls != 1 {
		t.Errorf("clearInteractionCalls: got %d, want 1", npc.clearInteractionCalls)
	}
	if len(npc.setTargetOpCalls) != 1 || npc.setTargetOpCalls[0] != int(objtype.NPCModeWander) {
		t.Errorf("setTargetOpCalls: got %v, want [NPCModeWander]", npc.setTargetOpCalls)
	}
	if npc.clearPatrolCalls != 0 {
		t.Errorf("clearPatrolCalls: got %d, want 0", npc.clearPatrolCalls)
	}
}

func TestNpcSetMode_ModePatrolAlsoClearsPatrol(t *testing.T) {
	npc := &mockNpc{}
	mc := &mockConfigs{}

	_ = runNpcOp(t, npc, mc, OpNpcSetMode, []int{int(objtype.NPCModePatrol)})

	if npc.clearInteractionCalls != 1 {
		t.Errorf("clearInteractionCalls: got %d, want 1", npc.clearInteractionCalls)
	}
	if len(npc.setTargetOpCalls) != 1 || npc.setTargetOpCalls[0] != int(objtype.NPCModePatrol) {
		t.Errorf("setTargetOpCalls: got %v, want [NPCModePatrol]", npc.setTargetOpCalls)
	}
	if npc.clearPatrolCalls != 1 {
		t.Errorf("clearPatrolCalls: got %d, want 1 (PATROL must reset patrol-tick)", npc.clearPatrolCalls)
	}
}

func TestNpcSetMode_ModeNullCallsResetDefaults(t *testing.T) {
	npc := &mockNpc{}
	mc := &mockConfigs{}

	_ = runNpcOp(t, npc, mc, OpNpcSetMode, []int{int(objtype.NPCModeNull)})

	if npc.resetDefaultsCalls != 1 {
		t.Errorf("resetDefaultsCalls: got %d, want 1", npc.resetDefaultsCalls)
	}
	if npc.clearInteractionCalls != 0 {
		t.Errorf("clearInteractionCalls: got %d, want 0 (NULL goes through resetDefaults, not direct clear)", npc.clearInteractionCalls)
	}
	if len(npc.setInteractionScriptCalls) != 0 {
		t.Errorf("setInteractionScriptCalls: got %d, want 0", len(npc.setInteractionScriptCalls))
	}
}

func TestNpcSetMode_OpPlayerWithSelfTargetBindsToActivePlayer(t *testing.T) {
	npc := &mockNpc{}
	player := &mockPlayer{}
	mc := &mockConfigs{}

	state := &ScriptState{
		Script: &ScriptFile{
			Name:             "test_npc_setmode",
			Opcodes:          []Opcode{OpNpcSetMode, OpReturn},
			IntOperands:      []int32{0, 0},
			StringOperands:   []string{"", ""},
			InstructionCount: 2,
		},
		ActiveNpc:   npc,
		Self:        player,
		Configs:     mc,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	state.PushInt(int(objtype.NPCModeOpPlayer1))

	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if len(npc.setInteractionScriptCalls) != 1 {
		t.Fatalf("setInteractionScriptCalls: got %d, want 1", len(npc.setInteractionScriptCalls))
	}
	got := npc.setInteractionScriptCalls[0]
	if got.mode != int(objtype.NPCModeOpPlayer1) {
		t.Errorf("mode: got %d, want NPCModeOpPlayer1", got.mode)
	}
	if got.target != ActivePlayer(player) {
		t.Errorf("target: got %v, want player (%v)", got.target, player)
	}
}

func TestNpcSetMode_OpNpcWithIntOperandZeroBindsToOtherActiveNpc(t *testing.T) {
	npc := &mockNpc{}
	otherNpc := &mockNpc{}
	mc := &mockConfigs{}

	// IntOperands[0] = 0 → use OtherActiveNpc per TS NpcOps.ts:212-216.
	state := &ScriptState{
		Script: &ScriptFile{
			Name:             "test_npc_setmode",
			Opcodes:          []Opcode{OpNpcSetMode, OpReturn},
			IntOperands:      []int32{0, 0},
			StringOperands:   []string{"", ""},
			InstructionCount: 2,
		},
		ActiveNpc:      npc,
		OtherActiveNpc: otherNpc,
		Configs:        mc,
		IntStack:       make([]int, StackCapacity),
		StringStack:    make([]string, StackCapacity),
	}
	state.PushInt(int(objtype.NPCModeOpNpc1))

	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if len(npc.setInteractionScriptCalls) != 1 {
		t.Fatalf("setInteractionScriptCalls: got %d, want 1", len(npc.setInteractionScriptCalls))
	}
	if npc.setInteractionScriptCalls[0].target != ActiveNpc(otherNpc) {
		t.Errorf("target: got %v, want otherNpc (%v) — operand=0 selects OtherActiveNpc",
			npc.setInteractionScriptCalls[0].target, otherNpc)
	}
}

func TestNpcSetMode_OpNpcWithIntOperandNonZeroBindsToActiveNpc(t *testing.T) {
	npc := &mockNpc{}
	otherNpc := &mockNpc{}
	mc := &mockConfigs{}

	// IntOperands[0] = 1 → use ActiveNpc per TS NpcOps.ts:214.
	state := &ScriptState{
		Script: &ScriptFile{
			Name:             "test_npc_setmode",
			Opcodes:          []Opcode{OpNpcSetMode, OpReturn},
			IntOperands:      []int32{1, 0},
			StringOperands:   []string{"", ""},
			InstructionCount: 2,
		},
		ActiveNpc:      npc,
		OtherActiveNpc: otherNpc,
		Configs:        mc,
		IntStack:       make([]int, StackCapacity),
		StringStack:    make([]string, StackCapacity),
	}
	state.PushInt(int(objtype.NPCModeOpNpc1))

	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if len(npc.setInteractionScriptCalls) != 1 {
		t.Fatalf("setInteractionScriptCalls: got %d, want 1", len(npc.setInteractionScriptCalls))
	}
	if npc.setInteractionScriptCalls[0].target != ActiveNpc(npc) {
		t.Errorf("target: got %v, want npc (self) — operand!=0 selects ActiveNpc",
			npc.setInteractionScriptCalls[0].target)
	}
}

func TestNpcSetMode_OpObjBindsToActiveObj(t *testing.T) {
	npc := &mockNpc{}
	obj := &mockActiveObj{}
	mc := &mockConfigs{}

	state := &ScriptState{
		Script: &ScriptFile{
			Name: "test", Opcodes: []Opcode{OpNpcSetMode, OpReturn},
			IntOperands: []int32{0, 0}, StringOperands: []string{"", ""},
			InstructionCount: 2,
		},
		ActiveNpc:   npc,
		ActiveObj:   obj,
		Configs:     mc,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	state.PushInt(int(objtype.NPCModeOpObj1))

	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if len(npc.setInteractionScriptCalls) != 1 {
		t.Fatalf("setInteractionScriptCalls: got %d, want 1", len(npc.setInteractionScriptCalls))
	}
	if npc.setInteractionScriptCalls[0].target != ActiveObj(obj) {
		t.Errorf("target: got %v, want obj (%v)", npc.setInteractionScriptCalls[0].target, obj)
	}
}

func TestNpcSetMode_OpLocBindsToActiveLoc(t *testing.T) {
	npc := &mockNpc{}
	loc := &mockActiveLoc{}
	mc := &mockConfigs{}

	state := &ScriptState{
		Script: &ScriptFile{
			Name: "test", Opcodes: []Opcode{OpNpcSetMode, OpReturn},
			IntOperands: []int32{0, 0}, StringOperands: []string{"", ""},
			InstructionCount: 2,
		},
		ActiveNpc:   npc,
		ActiveLoc:   loc,
		Configs:     mc,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	state.PushInt(int(objtype.NPCModeOpLoc1))

	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if len(npc.setInteractionScriptCalls) != 1 {
		t.Fatalf("setInteractionScriptCalls: got %d, want 1", len(npc.setInteractionScriptCalls))
	}
	if npc.setInteractionScriptCalls[0].target != ActiveLoc(loc) {
		t.Errorf("target: got %v, want loc (%v)", npc.setInteractionScriptCalls[0].target, loc)
	}
}

func TestNpcSetMode_OpPlayerWithNoSelfFallsThroughToResetDefaults(t *testing.T) {
	npc := &mockNpc{}
	mc := &mockConfigs{}

	// Self == nil → no-target fallthrough → resetDefaults
	state := &ScriptState{
		Script: &ScriptFile{
			Name: "test", Opcodes: []Opcode{OpNpcSetMode, OpReturn},
			IntOperands: []int32{0, 0}, StringOperands: []string{"", ""},
			InstructionCount: 2,
		},
		ActiveNpc:   npc,
		Self:        nil,
		Configs:     mc,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	state.PushInt(int(objtype.NPCModeOpPlayer1))

	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if npc.resetDefaultsCalls != 1 {
		t.Errorf("resetDefaultsCalls: got %d, want 1 (no-target fallthrough)", npc.resetDefaultsCalls)
	}
	if len(npc.setInteractionScriptCalls) != 0 {
		t.Errorf("setInteractionScriptCalls: got %d, want 0 (no target → no bind)", len(npc.setInteractionScriptCalls))
	}
}

func TestNpcSetMode_NoActiveNpcErrors(t *testing.T) {
	state := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	state.PushInt(int(objtype.NPCModeNone))

	err := handleNpcSetMode(state)
	if err == nil || !strings.Contains(err.Error(), "no active npc") {
		t.Errorf("handleNpcSetMode with no active npc: got %v, want error", err)
	}
}

// --- mockActiveLoc / mockActiveObj fixtures ------------------------------

type mockActiveLoc struct {
	locType int
}

func (m *mockActiveLoc) LocType() int { return m.locType }

type mockActiveObj struct {
	objType, x, z, level int
}

func (m *mockActiveObj) ObjType() int                     { return m.objType }
func (m *mockActiveObj) Coords() (x, z, level int)        { return m.x, m.z, m.level }
```

(If `mockActiveLoc` / `mockActiveObj` already exist in test fixtures from earlier NAI work, reuse them — grep `pkg/script/` for `type mockActive` before adding.)

### Step 6.8: Run failing tests

- [ ] **Step 6.8: Confirm tests fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run 'TestNpcSetMode' ./pkg/script/...`
Expected: FAIL with `undefined: handleNpcSetMode` (or compile errors related to missing interface methods).

### Step 6.9: Implement `handleNpcSetMode`

- [ ] **Step 6.9: Edit `pkg/script/handlers_npc.go`**

After `handleNpcGetMode` (Task 3), add:

```go
// handleNpcSetMode (NPC_SETMODE, opcode 2535) sets the active NPC's mode
// (targetOp). 3-branch dispatch: clear-target modes (NONE/WANDER/PATROL),
// NULL → resetDefaults, target-binding modes (OPNPC*/OPOBJ*/OPLOC*/OPPLAYER*).
// Mirrors TS NpcOps.ts:188-249.
func handleNpcSetMode(s *ScriptState) error {
	if err := requireActiveNpc(s, "NPC_SETMODE"); err != nil {
		return err
	}
	mode := s.PopInt()
	if err := checkNpcMode(mode, "NPC_SETMODE"); err != nil {
		return err
	}

	// Branch 1: clear-target modes.
	if mode == objtype.NPCModeNone || mode == objtype.NPCModeWander || mode == objtype.NPCModePatrol {
		s.ActiveNpc.ClearInteraction()
		s.ActiveNpc.SetTargetOp(mode)
		if mode == objtype.NPCModePatrol {
			s.ActiveNpc.ClearPatrol()
		}
		return nil
	}

	// Branch 2: NULL → resetDefaults.
	if mode == objtype.NPCModeNull {
		s.ActiveNpc.ResetDefaults()
		return nil
	}

	// Branch 3: target-binding modes.
	s.ActiveNpc.SetTargetOp(mode)

	var target any
	switch {
	case mode >= objtype.NPCModeOpNpc1: // OpNpc/ApNpc range
		operand := s.Script.IntOperands[s.PC]
		if operand == 0 {
			target = s.OtherActiveNpc
		} else {
			target = s.ActiveNpc
		}
	case mode >= objtype.NPCModeOpObj1: // OpObj/ApObj range
		target = s.ActiveObj
	case mode >= objtype.NPCModeOpLoc1: // OpLoc/ApLoc range
		target = s.ActiveLoc
	default: // OpPlayer/ApPlayer + PlayerEscape/Follow/Face/FaceClose
		target = s.Self
	}

	// nil-target check: TS uses `if (target)` which truthy-checks for both
	// nil and undefined. Goscape's interface-value comparison: target == nil
	// catches an explicitly-nil interface; for typed-nil (e.g. (*mockPlayer)(nil)
	// stored in an interface), reflect on the value. Since production callers
	// either set the pointer or leave the field zero (untyped nil), interface
	// nil check is sufficient for TS-faithful behavior.
	if target == nil {
		s.ActiveNpc.ResetDefaults()
		return nil
	}
	s.ActiveNpc.SetInteractionScript(target, mode)
	return nil
}
```

**Type-switch ordering note:** the case ordering above matches TS's branch order at NpcOps.ts:212-220 — OpNpc range tested FIRST (highest mode values), then OpObj, OpLoc, then default-player. Reversing the order would mis-route OpNpc modes to the OpObj branch. Verify against TS before commit.

**Field-name verification per `mock_recorder_field_naming_check.md`:** the spec uses `s.OtherActiveNpc` and `s.ActiveObj` and `s.ActiveLoc` and `s.Self` — these are the exact ScriptState field names per recon. Do not type `s.OtherActiveNpc2` or `s.ActiveObj1` (common typos).

### Step 6.10: Register

- [ ] **Step 6.10: Edit `pkg/script/handlers.go`**

Add:

```go
	OpNpcSetMode:           handleNpcSetMode,
```

### Step 6.11: Run tests — should pass

- [ ] **Step 6.11: Confirm tests pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run 'TestNpcSetMode' ./pkg/script/...`
Expected: PASS for all 11 sub-tests (10 branches + no-active-npc).

### Step 6.12: Run full pkg/script + modules/world test suite

- [ ] **Step 6.12: Regression check**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/... ./modules/world/...`
Expected: all PASS.

### Step 6.13: Commit

- [ ] **Step 6.13: Commit Task 6**

```bash
git add pkg/script/handlers_npc.go pkg/script/handlers.go pkg/script/handlers_npc_test.go pkg/script/active.go modules/world/npc_interaction.go modules/world/npc_script.go
git commit --no-gpg-sign -m "feat(script,world): NAI-36 T6 — NPC_SETMODE handler (opcode 2535)

3-branch dispatch (clear-target / NULL / target-binding) mirroring TS
NpcOps.ts:188-249. Adds clearPatrol method to Npc, plus 5 new ActiveNpc
interface methods (ClearInteraction, ResetDefaults, ClearPatrol, SetTargetOp,
SetInteractionScript) with adapters that delegate to existing world-side
methods. Type-switch on script.Active* values in SetInteractionScript
recovers concrete world-side entity for n.SetInteraction(SCRIPT, target,
mode, -1) call (per NAI-36-D3 — adapter-shape divergence resolved at T6
entry, no deviation tracker entry needed).

Test coverage: 10 branches + no-active-npc (per spec test strategy section
NPC_SETMODE branch coverage gate)."
```

---

## Task 7: PathingEntity.teleport partial parity sweep + PatrolMode level fix

**Files:**
- Modify: `pkg/script/state.go` — add `IsZoneAllocated(level, x, z int) bool` to WorldVars interface
- Modify: `pkg/script/handlers_vars_test.go` — add `IsZoneAllocated` no-op stub on mockWorld (returns true so existing tests pass)
- Modify: `modules/world/server.go` (or wherever the worldVars adapter is) — wire IsZoneAllocated through to FlagMap.IsZoneAllocated
- Modify: `modules/world/player_script.go:226-233` — close Player.Teleport divergences (D1+D2+order+D5)
- Modify: `modules/world/npc_script.go:109-114` — close Npc.Teleport divergences (D1+D2)
- Modify: `modules/world/npc_interaction.go:121` — fix PatrolMode level discard
- Modify: `modules/world/npc_script.go:95-97` — update DEVIATION comment for partial closure
- Modify: `pkg/script/active.go:501` — update DEVIATION comment for partial closure
- Test: `modules/world/player_script_test.go` (new or extend) — Player.Teleport tests
- Test: `modules/world/npc_script_test.go` (new or extend) — Npc.Teleport tests
- Test: `modules/world/npc_player_modes_test.go` — PatrolMode level fix test

### Step 7.1: Pre-flight grep — Teleport callers + DEVIATION comments

- [ ] **Step 7.1a: Enumerate Teleport callers**

Run: `grep -rn '\.Teleport(' modules/world/ pkg/`
Expected (8 lines per recon):
- 2 in modules/world/npc_interaction.go (lines 95, 121) — Npc-side
- 1 in pkg/script/handlers_npc.go (NPC_TELE handler) — interface call
- 1 in pkg/script/handlers_player.go (PLAYER_TELE) — interface call
- 1 each TeleJump-related in player_script.go

If any caller passes a level > 3 or < 0 currently, document for clamp regression risk.

- [ ] **Step 7.1b: Enumerate DEVIATION comments**

Run: `grep -rn 'NAI-34-D' pkg/ modules/ cmd/`
Expected sites:
- `modules/world/npc_script.go:95` (block comment block referencing D1..D5)
- `pkg/script/active.go:501` (cross-reference)

Both will be updated in Step 7.10.

### Step 7.2: Add `IsZoneAllocated` to WorldVars interface

- [ ] **Step 7.2: Edit `pkg/script/state.go`**

In the `WorldVars` interface, after `AnimMap(...)` (added in T1 Step 1.9), add:

```go
	// IsZoneAllocated reports whether the zone containing (x, z) at level
	// has been allocated (initialized for live play). Used by Teleport
	// safety check per TS PathingEntity.ts:271 — teleports to unallocated
	// zones are silently ignored.
	IsZoneAllocated(level, x, z int) bool
```

### Step 7.3: Add stub to mockWorld

- [ ] **Step 7.3: Edit `pkg/script/handlers_vars_test.go`**

Add (next to other no-op stubs):

```go
// NAI-36-T7: default returns true (assume zone allocated) so existing
// fixtures that don't care about zone-rejection don't see false-rejects.
func (m *mockWorld) IsZoneAllocated(level, x, z int) bool { return true }
```

If any T7 test fixture needs to FORCE false (zone-reject path), use a derived struct with method override (pattern from `mapBlockedWorld`).

### Step 7.4: Wire `IsZoneAllocated` on the production WorldVars adapter

- [ ] **Step 7.4: Edit `modules/world/server.go`** (or wherever `*Server.IsMapBlocked`/`IsFreeToPlay` live — discover via `grep -rn 'IsMapBlocked' modules/world/`)

Add (next to the existing IsMapBlocked / IsFreeToPlay methods):

```go
// IsZoneAllocated reports whether the (level, x, z) zone is allocated.
// Delegates to the FlagMap collision layer at
// pkg/pathfinder/collision/flagmap.go. NAI-36-T7.
func (s *Server) IsZoneAllocated(level, x, z int) bool {
	return s.flagMap.IsZoneAllocated(x, z, level)
}
```

(Adjust `s.flagMap` to the actual field name in production `*Server` — verify at edit time. The exact accessor likely already exists for some other handler.)

### Step 7.5: Write failing tests for Player.Teleport

- [ ] **Step 7.5: Edit `modules/world/player_script_test.go`** (create if missing)

Append:

```go
// --- NAI-36 Task 7: Player.Teleport partial parity ----------------------

// TestPlayerTeleport_LevelClampNegative verifies D1 closure: level=-1
// clamps to 0 per TS PathingEntity.ts:269.
func TestPlayerTeleport_LevelClampNegative(t *testing.T) {
	srv := newTestServer(t) // helper from existing tests
	p := newTestPlayer(t, srv, 3200, 3300, 0)

	p.Teleport(3210, 3310, -1)

	if p.level != 0 {
		t.Errorf("level after Teleport(level=-1): got %d, want 0 (clamp)", p.level)
	}
	if p.x != 3210 || p.z != 3310 {
		t.Errorf("x/z after Teleport: got (%d, %d), want (3210, 3310)", p.x, p.z)
	}
}

func TestPlayerTeleport_LevelClampHigh(t *testing.T) {
	srv := newTestServer(t)
	p := newTestPlayer(t, srv, 3200, 3300, 0)

	p.Teleport(3210, 3310, 4)

	if p.level != 3 {
		t.Errorf("level after Teleport(level=4): got %d, want 3 (clamp)", p.level)
	}
}

// TestPlayerTeleport_UnallocatedZoneRejects verifies D2 closure: teleport
// to a zone where IsZoneAllocated returns false is silently ignored.
func TestPlayerTeleport_UnallocatedZoneRejects(t *testing.T) {
	srv := newTestServer(t)
	srv.flagMap.UnallocateAllForTest() // helper or zero-init pattern
	p := newTestPlayer(t, srv, 3200, 3300, 0)
	prevX, prevZ, prevLevel := p.x, p.z, p.level

	p.Teleport(3210, 3310, 0)

	if p.x != prevX || p.z != prevZ || p.level != prevLevel {
		t.Errorf("Teleport to unallocated zone: state changed (%d,%d,%d) → (%d,%d,%d), want unchanged",
			prevX, prevZ, prevLevel, p.x, p.z, p.level)
	}
	if p.tele {
		t.Errorf("tele flag: got true, want false (rejected teleport must not set flag)")
	}
}

// TestPlayerTeleport_OrderRefreshThenFlag verifies the body-order
// alignment: refreshPlayerZone is called BEFORE p.tele = true.
// Pinned via a recording fixture or order-spy.
func TestPlayerTeleport_OrderRefreshThenFlag(t *testing.T) {
	// Use a recording fixture that timestamps zone-refresh and tele-write.
	srv := newTestServerWithOrderRecording(t)
	p := newTestPlayer(t, srv, 3200, 3300, 0)

	p.Teleport(3210, 3310, 0)

	// zoneRefreshTime should be < teleFlagTime per TS PathingEntity.ts:290-293.
	if srv.zoneRefreshTime >= srv.teleFlagTime {
		t.Errorf("body order: refreshPlayerZone (t=%d) ran AFTER p.tele=true (t=%d), want refresh-then-flag",
			srv.zoneRefreshTime, srv.teleFlagTime)
	}
}

// TestPlayerTeleport_SameLevelNoMoveSpeedChange verifies D5 closure
// negative case: same-level teleport does NOT touch moveSpeed/jump.
func TestPlayerTeleport_SameLevelNoMoveSpeedChange(t *testing.T) {
	srv := newTestServer(t)
	p := newTestPlayer(t, srv, 3200, 3300, 0)
	p.moveSpeed = MoveSpeedWalk
	p.jump = false

	p.Teleport(3210, 3310, 0) // same level

	if p.moveSpeed != MoveSpeedWalk {
		t.Errorf("same-level moveSpeed: got %v, want MoveSpeedWalk (unchanged)", p.moveSpeed)
	}
	if p.jump {
		t.Errorf("same-level jump: got true, want false (unchanged)")
	}
}

// TestPlayerTeleport_LevelChangeSetsInstantAndJump verifies D5 closure
// positive case: level-change sets moveSpeed=Instant + jump=true.
func TestPlayerTeleport_LevelChangeSetsInstantAndJump(t *testing.T) {
	srv := newTestServer(t)
	p := newTestPlayer(t, srv, 3200, 3300, 0)
	p.moveSpeed = MoveSpeedWalk
	p.jump = false

	p.Teleport(3210, 3310, 1) // level changed 0 → 1

	if p.moveSpeed != MoveSpeedInstant {
		t.Errorf("level-change moveSpeed: got %v, want MoveSpeedInstant", p.moveSpeed)
	}
	if !p.jump {
		t.Errorf("level-change jump: got false, want true")
	}
}
```

**Note on test helpers:** `newTestServer`, `newTestPlayer`, `UnallocateAllForTest`, `newTestServerWithOrderRecording` may need to be created. Verify against existing player-side test fixtures (`modules/world/movement_test.go` and `modules/world/afkzone_test.go` are good references). If absent, create minimal helpers — keep them in the same test file.

### Step 7.6: Run failing tests

- [ ] **Step 7.6: Confirm fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run 'TestPlayerTeleport_(LevelClamp|UnallocatedZone|Order|SameLevelNo|LevelChangeSets)' ./modules/world/...`
Expected: FAIL — current Player.Teleport has neither clamp nor reject nor proper order nor level-change branch.

### Step 7.7: Implement Player.Teleport parity

- [ ] **Step 7.7: Edit `modules/world/player_script.go:224-233`**

Replace the existing `Teleport` body with:

```go
// Teleport moves the player to (x, z, level) and flags the client for a
// smooth teleport transition (tele without jump in same-level case).
// Mirrors TS PathingEntity.teleport at PathingEntity.ts:267-298 (NAI-36-T7
// closes D1+D2+order+D5; D3 (focus) and D4 (lastStepX/Z reset) remain
// residual — see DEVIATION block at npc_script.go for tracker).
func (p *Player) Teleport(x, z, level int) {
	// D1: clamp level to [0, 3] per PathingEntity.ts:269.
	if level < 0 {
		level = 0
	} else if level > 3 {
		level = 3
	}
	// D2: reject teleports to unallocated zones per PathingEntity.ts:271.
	if !p.server.IsZoneAllocated(level, x, z) {
		return
	}

	prevX, prevZ, prevLevel := p.x, p.z, p.level
	p.x = x
	p.z = z
	p.level = level

	// Order: refresh BEFORE tele=true per PathingEntity.ts:290-293.
	refreshPlayerZone(p, prevX, prevZ, prevLevel)
	p.tele = true

	// D5: level-change → INSTANT + jump per PathingEntity.ts:294-297.
	if prevLevel != level {
		p.moveSpeed = MoveSpeedInstant
		p.jump = true
	}
}
```

**Note:** the call `p.server.IsZoneAllocated(level, x, z)` assumes Player has access to the Server (or a FlagMap reference). If `p.server` field doesn't exist, use the actual accessor pattern at HEAD — verify via `grep 'p\.server\.' modules/world/player_script.go` and `grep 'flagMap' modules/world/player.go`.

### Step 7.8: Run Player.Teleport tests — should pass

- [ ] **Step 7.8: Confirm passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run 'TestPlayerTeleport_' ./modules/world/...`
Expected: all 6 sub-tests PASS.

### Step 7.9: Write failing tests + implement Npc.Teleport parity

- [ ] **Step 7.9a: Edit `modules/world/npc_script_test.go`** (create if missing)

Append:

```go
// --- NAI-36 Task 7: Npc.Teleport partial parity (D1+D2 only) ------------

func TestNpcTeleport_LevelClampNegative(t *testing.T) {
	srv := newTestServer(t)
	n := newTestNpc(t, srv, 3200, 3300, 0)

	n.Teleport(3210, 3310, -1)

	if n.level != 0 {
		t.Errorf("level after Teleport(level=-1): got %d, want 0 (clamp)", n.level)
	}
}

func TestNpcTeleport_LevelClampHigh(t *testing.T) {
	srv := newTestServer(t)
	n := newTestNpc(t, srv, 3200, 3300, 0)

	n.Teleport(3210, 3310, 4)

	if n.level != 3 {
		t.Errorf("level after Teleport(level=4): got %d, want 3 (clamp)", n.level)
	}
}

func TestNpcTeleport_UnallocatedZoneRejects(t *testing.T) {
	srv := newTestServer(t)
	srv.flagMap.UnallocateAllForTest()
	n := newTestNpc(t, srv, 3200, 3300, 0)
	prevX, prevZ, prevLevel := n.x, n.z, n.level

	n.Teleport(3210, 3310, 0)

	if n.x != prevX || n.z != prevZ || n.level != prevLevel {
		t.Errorf("Teleport to unallocated zone: state changed, want unchanged")
	}
	if n.tele {
		t.Errorf("tele flag: got true, want false")
	}
}
```

- [ ] **Step 7.9b: Edit `modules/world/npc_script.go:109-114`**

Replace the existing Npc.Teleport body with:

```go
// Teleport moves the NPC to (x, z, level) and flags the client. Mirrors
// TS PathingEntity.teleport at PathingEntity.ts:267-298 (NAI-36-T7 closes
// D1+D2; D3 (focus), D4 (lastStepX/Z reset), D5 (NPC jump field absent)
// remain residual — see DEVIATION block below for tracker.
func (n *Npc) Teleport(x, z, level int) {
	// D1: clamp level to [0, 3] per PathingEntity.ts:269.
	if level < 0 {
		level = 0
	} else if level > 3 {
		level = 3
	}
	// D2: reject teleports to unallocated zones per PathingEntity.ts:271.
	if !n.server.IsZoneAllocated(level, x, z) {
		return
	}

	prevX, prevZ, prevLevel := n.x, n.z, n.level
	n.x, n.z, n.level = x, z, level

	refreshNpcZone(n.server, n, prevX, prevZ, prevLevel)
	n.tele = true
}
```

### Step 7.10: Run Npc.Teleport tests — should pass

- [ ] **Step 7.10: Confirm passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run 'TestNpcTeleport_' ./modules/world/...`
Expected: PASS.

### Step 7.11: Update DEVIATION comments per `retire_deviation_grep_all_comments.md`

- [ ] **Step 7.11a: Edit `modules/world/npc_script.go:95-97`**

Replace the existing block comment:

```go
// DEVIATION NAI-34-D1..D5 vs TS PathingEntity.teleport (PathingEntity.ts:267):
// no level clamp, no unallocated-zone rejection, no focus(), no
// lastStepX/Z adjust, no previousLevel != level branch. Mirrors the
// [...]
```

With:

```go
// DEVIATION NAI-34-D3, D4, D5-NPC vs TS PathingEntity.teleport
// (PathingEntity.ts:267) — partial closure as of NAI-36-T7:
//
// CLOSED in NAI-36-T7:
//   - D1 (level clamp to [0, 3]) — closed for both Npc.Teleport and
//     Player.Teleport.
//   - D2 (unallocated-zone reject via IsZoneAllocated) — closed for both
//     entities.
//
// RESIDUAL (active deviations):
//   - D3: no focus() call (PathingEntity.ts:286). Closure requires
//     designing fine-coord conversion + instant-flag semantics for the
//     NPC side. Tracked for future "pathing-entity-focus-and-step-tracking"
//     sub-spec.
//   - D4: no lastStepX/Z adjust (PathingEntity.ts:289-290). Npc has no
//     lastStepX/Z fields; adding without a consumer is dead-API per
//     dead_api_polish.md. Tracked for the same future sub-spec.
//   - D5-NPC: no `previousLevel != level → moveSpeed=INSTANT + jump=true`
//     branch. Npc has no jump field; same dead-API concern. (D5 is closed
//     for Player in NAI-36-T7 since Player has both lastStepX/Z and jump.)
```

- [ ] **Step 7.11b: Edit `pkg/script/active.go:501`**

Replace the existing cross-reference comment with similar partial-closure framing pointing to the updated block in npc_script.go.

### Step 7.12: Fix PatrolMode level discard

- [ ] **Step 7.12a: Write failing test**

Edit `modules/world/npc_player_modes_test.go` or `modules/world/npc_interaction_test.go` (whichever hosts patrolMode tests):

```go
// TestPatrolMode_PreservesDestLevel verifies NAI-36-T7: PatrolMode at
// npc_interaction.go:121 passes dest.Level (not 0) per TS Npc.ts:729.
// Pre-NAI-36-T7 bug: NPC silently teleports to level 0 ignoring dest.Level.
func TestPatrolMode_PreservesDestLevel(t *testing.T) {
	srv := newTestServer(t)
	n := newTestNpcWithPatrol(t, srv, /* patrolCoords with dest.Level=1 */)
	n.nextPatrolTick = 0 // force the level-1 teleport branch
	srv.currentTick = 1

	n.patrolMode(srv)

	if n.level != 1 {
		t.Errorf("PatrolMode level after patrol-tele: got %d, want 1 (dest.Level)", n.level)
	}
}
```

- [ ] **Step 7.12b: Edit `modules/world/npc_interaction.go:121`**

Change:

```go
		n.Teleport(dest.X, dest.Z, 0)
```

To:

```go
		n.Teleport(dest.X, dest.Z, dest.Level)
```

- [ ] **Step 7.12c: Run PatrolMode test**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run 'TestPatrolMode_PreservesDestLevel' ./modules/world/...`
Expected: PASS.

### Step 7.13: Full regression + commit

- [ ] **Step 7.13: Run all tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`
Expected: all packages PASS.

- [ ] **Step 7.14: Commit Task 7**

```bash
git add pkg/script/state.go pkg/script/handlers_vars_test.go pkg/script/active.go modules/world/server.go modules/world/player_script.go modules/world/player_script_test.go modules/world/npc_script.go modules/world/npc_script_test.go modules/world/npc_interaction.go modules/world/npc_player_modes_test.go
git commit --no-gpg-sign -m "fix(world,script): NAI-36 T7 — PathingEntity.teleport partial parity (D1+D2 both entities, D5 Player; PatrolMode level fix)

Closes NAI-34-D1 (level clamp [0,3]) and NAI-34-D2 (unallocated-zone
reject) for both Npc.Teleport and Player.Teleport. Aligns Player.Teleport
body order to refresh-then-flag matching TS PathingEntity.ts:290-293.
Adds Player.Teleport level-change branch (moveSpeed=Instant + jump=true)
per PathingEntity.ts:294-297. Fixes PatrolMode level-discard bug at
npc_interaction.go:121 (n.Teleport(dest.X, dest.Z, 0) → dest.Level)
per TS Npc.ts:729.

Adds WorldVars.IsZoneAllocated interface seam + production *Server
adapter delegating to FlagMap.IsZoneAllocated. mockWorld stubs return
true so existing fixtures don't see false-rejects.

RESIDUAL: NAI-34-D3 (focus orientation), NAI-34-D4 (lastStepX/Z adjust),
NAI-34-D5-NPC (jump field absent on NPC) remain active deviations —
NPC-side infrastructure has no current consumers per dead_api_polish.md.
Tracked for future pathing-entity-focus-and-step-tracking sub-spec.

DEVIATION comments at npc_script.go:95+ and active.go:501 updated to
partial-closure framing per retire_deviation_grep_all_comments.md."
```

---

## Stage 2 review checkpoint (T6-T7)

Dispatch combined-review subagent:

> **Stage 2 review prompt:** Review commits since the Stage 1 review on the current branch. Scope: NAI-36 Tasks 6-7 — NPC_SETMODE 3-branch dispatch + PathingEntity.teleport partial parity sweep + PatrolMode level fix. Verify against the spec at `docs/superpowers/specs/2026-04-27-nai-36-runscript-stub-handlers-and-teleport-parity-design.md`. Critical checks: (a) NPC_SETMODE 10-branch coverage gate (clear-target × 3, NULL, OPNPC × intOperand=0, OPNPC × intOperand!=0, OPOBJ, OPLOC, OPPLAYER+self, no-target-fallthrough); (b) SetInteractionScript adapter type-switches correctly across all 4 concrete world-side entity types; (c) Player.Teleport closure of D1 (clamp), D2 (zone-reject), order alignment, D5 (level-change INSTANT/jump); (d) Npc.Teleport closure of D1+D2 only; (e) DEVIATION comments updated to partial-closure framing AT BOTH SITES (npc_script.go:95+ AND active.go:501) per retire_deviation_grep_all_comments.md; (f) PatrolMode level fix at npc_interaction.go:121 verified via dest.Level test; (g) WorldVars.IsZoneAllocated correctly added to interface, mocked, AND wired to production. Report: critical (blocks T8 close), high-priority, low-priority, or NOOP.

If review surfaces Critical issues, fix before T8.

---

## Task 8: Close polish — memory + tracker + smoke handoff

**Files:**
- Modify: `/home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md`
- Modify: `/home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/MEMORY.md` (only if new memory entries materialize)
- No code changes.

### Step 8.1: Final DEVIATION grep sweep

- [ ] **Step 8.1: Verify no stale references to D1/D2 closure**

Run: `grep -rn 'NAI-34-D1\|NAI-34-D2' pkg/ modules/ cmd/ docs/`
Expected: every occurrence describes the closure-framing (no naked "open deviation" framing for D1 or D2). If any stale comment surfaces, update it.

### Step 8.2: Update `nai_followups.md` with NAI-36 close

- [ ] **Step 8.2: Append NAI-36 close section**

Edit `/home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md`:

Add a new section under "## From NAI-35 (2026-04-27)" matching the existing pattern:

```markdown
## From NAI-36 (2026-04-27)

### NAI-36 close — runscript stub handlers (NPC_WALK/GETMODE/SETMODE, SPOTANIM_MAP, MAP_BLOCKED) + PathingEntity.teleport partial parity

**Closes:**
- NAI-34 follow-up #2 — PatrolMode level discard fix at npc_interaction.go:121.
- NAI-34 follow-up #3 — NPC_WALK opcode 2544 handler.
- NAI-34-D1 (level clamp) — both Npc.Teleport and Player.Teleport.
- NAI-34-D2 (unallocated-zone reject) — both entities.
- Player.Teleport order divergence (refresh-then-flag) and Player-side D5
  (level-change INSTANT/jump branch) — pre-existing pre-NAI-34 divergences,
  not numbered, retired in T7.

**NAI-36 deviations (introduced + tracked):**

- **NAI-36-D1 (dissolved at plan-write recon):** intOperand access pattern.
  Spec predicted a divergence; recon found `setActiveNpcSlot` at
  `pkg/script/handlers_npc.go:59` already uses direct
  `s.Script.IntOperands[s.PC]`. T6 follows that pattern; no deviation. ID
  reserved (not reused) to keep on-disk grep history stable per
  `retire_deviation_grep_all_comments.md`.
- **NAI-36-D2 (active):** SpotAnimType config-port absent at HEAD; T5
  uses range-validation (id < 0 reject) only. Closure when
  `Configs.SpotAnimType(id)` accessor is added. Tracking commit: T5
  (`6f97412`). Production tag: `pkg/script/handlers_map.go:207`. Test
  pin: `pkg/script/handlers_map_test.go:471`.
- **NAI-36-D3 (resolved at T6 entry):** SetInteractionScript adapter
  shape resolved via type-switch on script.Active* values; no deviation
  needed.

**Carryover residual (still active post-NAI-36):**

- **NAI-34-D3** — Npc.Teleport doesn't call focus(). Closure requires
  designing NPC-side fine-coord conversion. Future
  pathing-entity-focus-and-step-tracking sub-spec.
- **NAI-34-D4** — Npc.Teleport doesn't reset lastStepX/Z. Npc has no
  such fields; dead-API foot-gun.
- **NAI-34-D5-NPC** — Npc.Teleport doesn't set jump on level change.
  Npc has no jump field; dead-API foot-gun.

**Net deviation tally:** 16 (post-NAI-35) → 14 (post-NAI-36)
(- NAI-34-D1, - NAI-34-D2, + NAI-36-D2).

**Items deferred to future sub-specs (still open NAI-36 follow-ups):**

1. **`pathing-entity-focus-and-step-tracking` sub-spec** — close
   NAI-34-D3, D4, D5-NPC. Conditional on a consumer materializing for
   NPC-side stride/path-tracking / focus / jump.
2. **NPC_WALKTRIGGER (opcode 2545)** — separate stub, sibling of
   NPC_WALK. Tiny sub-spec (~15 LOC); compressed cadence eligible.
3. **NAI-35-T3-D1 audit** — op[1] operability gate, deferred from
   NAI-35; revisit when HUNTALL smoke surfaces a real-content miss.

**Memory entries touched:** none new at close-time; `mock_recorder_field_naming_check.md` reinforced via T6 mockNpc field expansion (`setInteractionScriptCalls`, `setTargetOpCalls`, etc.); `enumerate_all_sites.md` reinforced via T1 queueWaypoint rename.
```

### Step 8.3: Update MEMORY.md if new memory items material

- [ ] **Step 8.3: Decide on new memory items**

Review the work. Did anything surface that's worth a NEW memory entry per the saving-when-applicable rules? Candidates:
- The `script.Active*` interface type-switch pattern in T6's SetInteractionScript adapter is a reusable cross-package-boundary technique. Consider a memory entry like `script_active_to_world_entity_typeswitch.md` if other future ports will need it.
- The "partial closure forks tracker entry" pattern (D1+D2 close, D3+D4+D5-NPC stay) is novel for this project; consider if it warrants explicit memory.

If yes for either, write the memory file + add a one-liner to MEMORY.md per the auto-memory protocol.

### Step 8.4: Smoke handoff prompt

- [ ] **Step 8.4: Compose user-facing smoke handoff**

Output the following list to the user verbatim:

> **NAI-36 smoke handoff (per `smoke_test_server_handoff.md`):**
>
> Server-launched smokes — please run with the goscape server up against the Java client:
>
> 1. NPC_WALK script — pick any content script using `npc_walk` (audit `LostCityRS/Content/scripts/`); verify NPC walks to destination.
> 2. NPC_GETMODE / NPC_SETMODE — patrol-clear-then-set path; verify mode switch + faceEntity mask.
> 3. MAP_BLOCKED — F2P-world non-F2P tile (push 1) + members-world tile (collision-driven push).
> 4. SPOTANIM_MAP — visual graphic at a coord (Falador square / Wizards' Tower).
> 5. PatrolMode multi-level — patrol NPC with `dest.Level != 0` (multi-level dungeon patrol or Lumbridge cellar); verify NPC arrives at correct level.
> 6. Player.Teleport level-change — `::tele` cheat across level boundaries; verify smooth transition with INSTANT + jump.
>
> Plus carryover from NAI-35 smoke gate (still pending):
>
> 7. Lumbridge NPC_PARAM (NAI-35-T1).
> 8. Al-Kharid HUNTALL (NAI-35-T4).
> 9. Barbarian Village NPC_HUNTALL (NAI-35-T3).
> 10. Wizards' Tower MAP_FINDSQUARE (NAI-35-T6).

### Step 8.5: Final close commit

- [ ] **Step 8.5: Commit Task 8 with `Closes memory:` trailer**

```bash
git add docs/superpowers/specs/2026-04-27-nai-36-runscript-stub-handlers-and-teleport-parity-design.md docs/superpowers/plans/2026-04-27-nai-36-runscript-stub-handlers-and-teleport-parity-plan.md
# (Plus any new memory files added in Step 8.3.)
git commit --no-gpg-sign -m "chore(script,world): NAI-36 closed — 5 stub handlers ported + teleport partial parity

Closes the post-NAI-35 smoke-driven dispatch gaps (NPC_WALK, NPC_GETMODE,
NPC_SETMODE, SPOTANIM_MAP, MAP_BLOCKED) and partially closes the NAI-34
PathingEntity.teleport divergence thread (D1+D2 for both entities, D5 for
Player). PatrolMode level-discard bug fixed.

Test coverage: ~34 new tests across pkg/script/handlers_npc_test.go,
handlers_map_test.go, modules/world/player_script_test.go,
npc_script_test.go, npc_player_modes_test.go.

Net deviation tally: 16 → 14 (-NAI-34-D1, -NAI-34-D2, +NAI-36-D2).

Closes memory: NAI-34 follow-up #2 (PatrolMode level discard) + NAI-34
follow-up #3 (NPC_WALK port) + NAI-34-D1 (level clamp) + NAI-34-D2
(zone-reject) + Player.Teleport order divergence + Player.Teleport D5
(level-change INSTANT/jump). Three carryovers remain: NAI-34-D3,
NAI-34-D4, NAI-34-D5-NPC — tracked for future
pathing-entity-focus-and-step-tracking sub-spec."
```

---

## Self-review (post-plan write)

(per the writing-plans skill self-review checklist)

**1. Spec coverage check** — every spec section/requirement maps to a task:

| Spec section | Implementing task |
|---|---|
| In scope item 1 (Foundation seams) | T1 (Steps 1.1-1.14) |
| In scope item 2 (NPC_WALK) | T2 (Steps 2.1-2.7) |
| In scope item 3 (NPC_GETMODE) | T3 (Steps 3.1-3.6) |
| In scope item 4 (MAP_BLOCKED) | T4 (Steps 4.1-4.7) |
| In scope item 5 (SPOTANIM_MAP) | T5 (Steps 5.1-5.7) |
| In scope item 6 (NPC_SETMODE) | T6 (Steps 6.1-6.13) |
| In scope item 7 (Parity sweep + PatrolMode) | T7 (Steps 7.1-7.14) |
| In scope item 8 (Close polish) | T8 (Steps 8.1-8.5) |
| Out-of-scope items (D3/D4/D5-NPC, NPC_WALKTRIGGER, PLAYER_FINDALL family, full SpotAnimType config-port, NAI-35-T3-D1 audit) | Documented in T8 follow-ups |
| Test strategy (Layer 1 + Layer 2 + branch coverage gate + TS-asymmetry pin) | Each T2-T7 has matching test code blocks |
| Expected deviations (NAI-36-D1/D2/D3) | D1 dissolved at recon (intOperand pattern existed); T5 tracks D2 (SpotAnimType range-only); T6 resolves D3 (SetInteractionScript adapter shape, no entry needed) |
| Cadence (full, two-stage review) | Stage 1 between T5 and T6, Stage 2 between T7 and T8 |
| Smoke handoff (T8) | Step 8.4 |

**2. Placeholder scan** — searched for "TBD/TODO/FIXME/implement later":

- All occurrences are accompanied by concrete fallbacks (e.g. T6 type-switch has explicit type-list; T7 test helper "newTestServer" mentions creating if absent with reference patterns). No bare placeholders.
- Step 5.1 has a Case A vs Case B branch — both branches are fully specified with code; the plan-author picks based on the actual structure. This is acceptable since the recon couldn't determine which without reading more files.

**3. Type consistency check:**

- `QueueWaypoint(x, z int)` consistent across active.go interface declaration (Step 1.8), Npc adapter (Step 1.10), mockNpc method (Step 1.12), handler delegation (Step 2.3), and rename callers.
- `TargetOp() int` consistent across active.go (1.8), Npc adapter (1.10), mockNpc (1.12), handler (3.3).
- `AnimMap(level, x, z, spotanim, height, delay int)` consistent across state.go interface (1.9), mockWorld stub (1.11), spotAnimMapWorld test fixture (5.2), Server adapter (already exists at world_zone.go:76), handler delegation (5.4).
- `IsZoneAllocated(level, x, z int) bool` consistent across state.go (7.2), mockWorld (7.3), Server adapter (7.4), Player.Teleport call (7.7), Npc.Teleport call (7.9b).
- `SetInteractionScript(target any, mode int)` consistent across active.go (6.3), mockNpc method (6.2), Npc adapter (6.5b), handler call (6.9).
- `clearPatrol` (lowercase) at npc_interaction.go:696 vs `ClearPatrol` (capital) on ActiveNpc interface — adapter wrapper bridges; consistent.

**No type/method/field name divergences across tasks.**

The plan is complete and self-consistent. Plan-author / implementer dispatches can begin from the Pre-flight section.
