# NAI-163 — Cascade-tail-to-zero (4 ops; 4 bundles) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Port the four remaining unhandled RuneScript opcodes (`OpBusy`, `OpLineOfSight`, `OpNpcHunt`, `OpNpcAdd`) so the tightened missing-handler audit drops from 4 → 0.

**Architecture:** Four cohort bundles, each ending in a close commit; final NAI-163 roll-up close cites the tightened-regex recount. B0 adds two methods to `ActivePlayer` + a 5-line handler. B1 fixes a pre-existing arg-shape bug in the LOS wrapper as a prerequisite, then adds a level/F2P-gated handler. B2 reuses `NewHuntAllNpcIterator` with a local-only iterator (not stashed in `s.npcIterator`) plus `<=` tie-break selection. B3 adds `AddNpcAt` to the `WorldVars` interface + a `modules/world` adapter that constructs a despawn-lifecycle NPC and routes through `(*Server).addNpc`.

**Tech Stack:** Go 1.26+ (per `go_version.md`).

**Source-of-truth pinning at HEAD `0027628`:**
- TS canonical: `LostCityRS/Engine-TS/` (per `ts_source_canonical_path.md`).
- Rust canonical (rsmod): `2004scape/rsmod-pathfinder` (per `rust_source_canonical_path.md`).
- Spec: `docs/superpowers/specs/2026-05-11-nai-163-cascade-tail-zero-design.md` (committed at `bff2902`).

**Test command prefix (per global CLAUDE.md):**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/... ./modules/world/...
```

**Commit prefix (per global CLAUDE.md):** `git commit --no-gpg-sign ...`

---

## File map

### B0 — `OpBusy` (opcode 2005)

- **Modify** `pkg/script/active.go` — extend `ActivePlayer` interface with `Busy() bool` + `LoggingOut() bool`.
- **Modify** `modules/world/player.go` — add `func (p *Player) LoggingOut() bool { return p.loggingOut }` (no other adapter; `Busy()` already exists at `player.go:651`).
- **Modify** `pkg/script/handlers_player.go` — add `handleBusy(s)` next to `handleBusy2` at line 1334+.
- **Modify** `pkg/script/handlers.go` — add `OpBusy: handleBusy,` adjacent to the existing `OpBusy2: handleBusy2,` line at line 425.
- **Test** `pkg/script/handlers_player_test.go` (or `handlers_b0_stubs_test.go` whichever exists; plan-author re-greps for `TestHandleBusy2` to choose the file).

### B1 — `OpLineOfSight` (opcode 1005) + LV wrapper fix

- **Modify** `pkg/script/handlers_map.go` — (T0) patch `isLineOfSight` wrapper line 184 from `(1, 0, 0, 0)` → `(1, 1, 1, 0)`; (T1+) add `handleLineOfSight(s)`.
- **Modify** `pkg/script/handlers.go` — add `OpLineOfSight: handleLineOfSight,` adjacent to `OpLineOfWalk: handleLineOfWalk,` at line 133.
- **Test** `pkg/script/handlers_map_test.go` — add `TestHandleLineOfSight_*` cases + `TestIsLineOfSightWrapper_PassesTSFaithfulArgs` regression test for the wrapper fix.

### B2 — `OpNpcHunt` (opcode 2525)

- **Modify** `pkg/script/handlers_npc.go` — add `handleNpcHunt(s)` next to `handleNpcHuntAll` at line 817+.
- **Modify** `pkg/script/handlers.go` — add `OpNpcHunt: handleNpcHunt,` adjacent to `OpNpcHuntAll: handleNpcHuntAll,` at line 497.
- **Test** `pkg/script/handlers_npc_test.go` — add `TestHandleNpcHunt_*` cases.

### B3 — `OpNpcAdd` (opcode 2500) + `AddNpcAt` adapter

- **Modify** `pkg/script/state.go` — add `AddNpcAt(level, x, z, typeID, duration int) (ActiveNpc, error)` to `WorldVars` interface (next to `RemoveNpc` at line 110).
- **Modify** `modules/world/server_varp.go` — implement `func (w worldVarsView) AddNpcAt(...) (script.ActiveNpc, error)` next to `RemoveNpc` at line 146.
- **Modify** `pkg/script/handlers_npc.go` — add `handleNpcAdd(s)` (placement: file end, or near other NPC create/spawn handlers; plan-author re-greps `OpNpcAnim` placement at line 453 dispatch and mirrors handler placement).
- **Modify** `pkg/script/handlers.go` — add `OpNpcAdd: handleNpcAdd,` adjacent to `OpNpcAnim: handleNpcAnim,` at line 453.
- **Test** `pkg/script/handlers_npc_test.go` — add `TestHandleNpcAdd_*` cases (mock `WorldVars.AddNpcAt`).
- **Test** `modules/world/npc_registry_test.go` (or a new `npc_addat_test.go` if registry-test file has fixture conflicts) — add `TestAddNpcAt_*` cases against real `(*Server).addNpc`.

---

## TS-fidelity references (codify before dispatch)

**`PlayerOps.ts:893-895` (BUSY 2005):**

```ts
[ScriptOpcode.BUSY]: state => {
    state.pushInt(state.activePlayer.busy() || state.activePlayer.loggingOut ? 1 : 0);
},
```

**`ServerOps.ts:144-162` (LINEOFSIGHT 1005):**

```ts
[ScriptOpcode.LINEOFSIGHT]: state => {
    const [c1, c2] = state.popInts(2);
    const from: CoordGrid = check(c1, CoordValid);
    const to: CoordGrid = check(c2, CoordValid);
    if (from.level !== to.level) { state.pushInt(0); return; }
    if (!Environment.NODE_MEMBERS && !World.gameMap.isFreeToPlay(to.x, to.z)) {
        state.pushInt(0); return;
    }
    state.pushInt(isLineOfSight(from.level, from.x, from.z, to.x, to.z) ? 1 : 0);
},
```

**`GameMap.ts:429-431` (TS `isLineOfSight` wrapper — rsmod call shape):**

```ts
export function isLineOfSight(level: number, srcX, srcZ, destX, destZ): boolean {
    return rsmod.hasLineOfSight(level, srcX, srcZ, destX, destZ, 1, 1, 1, 1, 0);
}
```

Verified against rsmod-pathfinder `src/index.ts:355-379` (rsmod sig: `srcWidth, srcHeight, destWidth, destHeight, extraFlag`) and `src/rsmod/LineValidator.ts:15-45`. Goscape's `LineValidator.HasLineOfSight` signature is `(level, srcX, srcZ, destX, destZ, srcSize, destWidth, destLength, extraFlag)` where `srcSize` expands to both `srcWidth` and `srcLength` in `RayCast` (`linevalidator.go:21`). To match TS: pass `(srcSize=1, destWidth=1, destLength=1, extraFlag=0)`.

**`NpcOps.ts:290-321` (NPC_HUNT 2525) — re-read verbatim per spec R4:**

```ts
[ScriptOpcode.NPC_HUNT]: state => {
    const [coord, distance, checkVis] = state.popInts(3);
    const position: CoordGrid = check(coord, CoordValid);
    check(distance, NumberNotNull);
    const huntvis: HuntVis = check(checkVis, HuntVisValid);
    let closestNpc: Npc | null = null;
    let closestDistance = Number.MAX_SAFE_INTEGER;
    const npcs = new NpcHuntAllCommandIterator(World.currentTick, position.level, position.x, position.z, distance, huntvis);
    for (const npc of npcs) {
        if (npc) {
            const npcDistance = CoordGrid.euclideanSquaredDistance(position, npc);
            if (npcDistance <= closestDistance) {
                closestNpc = npc;
                closestDistance = npcDistance;
            }
        }
    }
    if (!closestNpc) { state.pushInt(0); return; }
    state.activeNpc = closestNpc;
    state.pointerAdd(ActiveNpc[state.intOperand]);
    state.pushInt(1);
},
```

Note: TS pops `[coord, distance, checkVis]` — `popInts(3)` destructures top-down so `checkVis` is **top of stack**, `coord` is **bottom**. Goscape pop order (top first): `huntvis`, `distance`, `coord`. The `<=` at line 307 is the tie-break sentinel (later iterator yield wins).

**`NpcOps.ts:42-53` (NPC_ADD 2500):**

```ts
[ScriptOpcode.NPC_ADD]: state => {
    const [coord, id, duration] = state.popInts(3);
    const position: CoordGrid = check(coord, CoordValid);
    const npcType: NpcType = check(id, NpcTypeValid);
    check(duration, DurationValid);
    const npc = new Npc(position.level, position.x, position.z, npcType.size, npcType.size,
        EntityLifeCycle.DESPAWN, World.getNextNid(), npcType.id, npcType.moverestrict, npcType.blockwalk);
    World.addNpc(npc, duration);
    state.activeNpc = npc;
    state.pointerAdd(ActiveNpc[state.intOperand]);
},
```

Note: TS pops `[coord, id, duration]` — goscape pop order (top first): `duration`, `id`, `coord`.

---

## Bundle B0 — `OpBusy` (5 LOC handler + ~15 LOC tests; cascade 4 → 3)

### Task B0.T1 — Extend ActivePlayer interface

**Files:**
- Modify: `pkg/script/active.go` (insert near `HasInteraction`/`HasWaypoints` at lines 398, 403)

- [ ] **Step 1: Add interface methods**

Edit `pkg/script/active.go` — find the `HasInteraction()` / `HasWaypoints()` block. Insert below `HasWaypoints() bool`:

```go
	// Busy reports whether the player cannot accept new interactions —
	// either delayed (suspended by script delay) or has a main/chat modal
	// open. Mirrors TS Player.busy() at Engine-TS/.../Player.ts:801-803.
	// Used by BUSY (PlayerOps.ts:893-895). NAI-163 B0.
	Busy() bool

	// LoggingOut reports whether the player is in the logout-in-progress
	// state (TS Player.loggingOut field). Distinct from delayed/modal/
	// interaction state — set by the logout pipeline before final cleanup.
	// Used by BUSY (PlayerOps.ts:893-895). NAI-163 B0.
	LoggingOut() bool
```

- [ ] **Step 2: Compile check**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...`
Expected: build FAILS — `*Player` does not implement `ActivePlayer` (missing `LoggingOut`).

### Task B0.T2 — Add `LoggingOut()` accessor on `*Player`

**Files:**
- Modify: `modules/world/player.go` (insert immediately after `Busy()` at line 651-653)

- [ ] **Step 1: Add accessor**

Edit `modules/world/player.go` — after the existing `Busy()` method ending at line 653, insert:

```go
// LoggingOut returns true while the player is in the logout-in-progress
// state. Field set by the logout pipeline; read by BUSY (PlayerOps.ts:894).
// Mirrors TS Player.loggingOut at Engine-TS/.../Player.ts (boolean field).
// NAI-163 B0.
func (p *Player) LoggingOut() bool {
	return p.loggingOut
}
```

- [ ] **Step 2: Compile check**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...`
Expected: PASS.

### Task B0.T3 — Write failing handler tests

**Files:**
- Test: `pkg/script/handlers_player_test.go` (re-grep `TestHandleBusy2` to confirm; if it lives elsewhere, mirror its location)

- [ ] **Step 1: Locate `TestHandleBusy2`**

Run: `grep -n "TestHandleBusy2\b" pkg/script/*_test.go`

Note the file path — the new tests go in the same file, immediately after `TestHandleBusy2`. If `TestHandleBusy2` does not exist, add the new tests next to other handler tests that use a similar `mockPlayer` / `ScriptState` fixture pattern (search for `s.Self =` to locate adjacent setups).

- [ ] **Step 2: Inspect the existing test mock**

Run: `grep -n "type mockPlayer\b\|busyValue\|hasInteractionValue\|hasWaypointsValue" pkg/script/*_test.go`

Confirm the test mock implements `ActivePlayer`. **If the mock does not have `busyValue` / `loggingOutValue` fields, add them** (the new `Busy()` / `LoggingOut()` interface methods must be implemented on the mock or compilation fails).

Add to the mock (typical shape; adjust field names if the existing mock uses different naming):

```go
// In the mockPlayer / testPlayer struct definition:
busyValue       bool
loggingOutValue bool

// Methods (placed next to other Bool getters):
func (m *mockPlayer) Busy() bool       { return m.busyValue }
func (m *mockPlayer) LoggingOut() bool { return m.loggingOutValue }
```

If multiple test mocks exist (e.g., `mockPlayer` in `handlers_player_test.go` and another in `handlers_inv_test.go`), update each that satisfies `ActivePlayer` — `go build ./pkg/script/...` will fail on any unimplemented mock.

- [ ] **Step 3: Write failing tests**

Add to the test file (adapting fixture style to match neighboring tests):

```go
func TestHandleBusy_NotBusy_NotLoggingOut_PushZero(t *testing.T) {
	self := &mockPlayer{busyValue: false, loggingOutValue: false}
	s := &ScriptState{
		Self:          self,
		StackCapacity: 4,
		Script:        &Script{IntOperands: []int{0}},
	}
	if err := handleBusy(s); err != nil {
		t.Fatalf("handleBusy: %v", err)
	}
	if got := s.PopInt(); got != 0 {
		t.Fatalf("expected push 0, got %d", got)
	}
}

func TestHandleBusy_Busy_PushOne(t *testing.T) {
	self := &mockPlayer{busyValue: true, loggingOutValue: false}
	s := &ScriptState{
		Self:          self,
		StackCapacity: 4,
		Script:        &Script{IntOperands: []int{0}},
	}
	if err := handleBusy(s); err != nil {
		t.Fatalf("handleBusy: %v", err)
	}
	if got := s.PopInt(); got != 1 {
		t.Fatalf("expected push 1, got %d", got)
	}
}

func TestHandleBusy_LoggingOut_PushOne(t *testing.T) {
	// Pins the loggingOut arm — distinguishes BUSY (2005) from BUSY2 (2006).
	self := &mockPlayer{busyValue: false, loggingOutValue: true}
	s := &ScriptState{
		Self:          self,
		StackCapacity: 4,
		Script:        &Script{IntOperands: []int{0}},
	}
	if err := handleBusy(s); err != nil {
		t.Fatalf("handleBusy: %v", err)
	}
	if got := s.PopInt(); got != 1 {
		t.Fatalf("expected push 1, got %d", got)
	}
}
```

If the existing `TestHandleBusy2` test uses a different fixture style (e.g., direct `Self2` setup, alternate mock type, or a `newTestState` helper), mirror that style verbatim. **Do not invent new fixture patterns.**

- [ ] **Step 4: Run tests to verify they FAIL with "handleBusy undefined"**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/... -run TestHandleBusy_ -v`
Expected: COMPILE FAIL — `undefined: handleBusy`.

### Task B0.T4 — Implement `handleBusy`

**Files:**
- Modify: `pkg/script/handlers_player.go` (insert immediately before `handleBusy2` at line 1334)

- [ ] **Step 1: Implement handler**

Insert before `handleBusy2`:

```go
// handleBusy (BUSY, opcode 2005) pushes 1 if the active player is busy
// (delayed/main-or-chat-modal-open) OR is in the logout-in-progress state,
// else 0. Mirrors TS PlayerOps.ts:893-895:
//
//	state.pushInt(state.activePlayer.busy() || state.activePlayer.loggingOut ? 1 : 0);
//
// Gate: ActivePlayer (no Protected requirement). Distinct from BUSY2
// (opcode 2006) which uses HasInteraction()||HasWaypoints(). The
// loggingOut arm is the conspicuous TS-asymmetry vs BUSY2. NAI-163 B0.
func handleBusy(s *ScriptState) error {
	if err := requireActivePlayer(s, "BUSY"); err != nil {
		return err
	}
	if s.Self.Busy() || s.Self.LoggingOut() {
		s.PushInt(1)
	} else {
		s.PushInt(0)
	}
	return nil
}
```

- [ ] **Step 2: Register dispatch**

Edit `pkg/script/handlers.go` line 425 — find:

```go
	OpBusy2:     handleBusy2,
```

Insert immediately above it:

```go
	OpBusy:      handleBusy,
```

(Match the field-name padding / alignment of the surrounding entries.)

- [ ] **Step 3: Run tests to verify they PASS**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/... -run TestHandleBusy_ -v`
Expected: PASS (3 tests).

- [ ] **Step 4: Run the full package test suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/... ./modules/world/...`
Expected: PASS (no regressions).

### Task B0.T5 — Update opcode dispatch comment (cascade-tail recount)

**Files:**
- Modify: `pkg/script/opcode.go` (no changes needed — `OpBusy` is already declared at line 105 and the existing case statement at line 617 already handles it for the disasm path. No changes here.)

- [ ] **Step 1: Skip** — no opcode.go change needed; `OpBusy` was declared but not dispatched.

### Task B0.T6 — Close commit B0

- [ ] **Step 1: Stage + commit**

```bash
git add pkg/script/active.go modules/world/player.go \
        pkg/script/handlers_player.go pkg/script/handlers.go \
        pkg/script/handlers_player_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): NAI-163 B0 — OpBusy handler (opcode 2005)

Ports TS PlayerOps.ts:893-895. Pushes 1 if Self.Busy() ||
Self.LoggingOut(), else 0. The loggingOut arm is the conspicuous
asymmetry vs BUSY2 (2006).

Adds Busy() + LoggingOut() to the ActivePlayer interface and a 1-line
LoggingOut() accessor on *Player. Busy() was already implemented at
modules/world/player.go:651.

Pins the loggingOut arm via TestHandleBusy_LoggingOut_PushOne
(busy()=false, loggingOut=true → push 1) per ts_asymmetry_dual_pin.md.

Cascade-tail (tightened regex Op[A-Za-z][A-Za-z0-9]*): 4 → 3.

Closes memory: ts_asymmetry_dual_pin.md, missing_handler_audit_regex_flaw.md

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

- [ ] **Step 2: Verify commit content matches stated scope**

Run: `git show HEAD --stat`
Expected: only the 5 files staged in Step 1; no stray files.

Run: `git show HEAD -- pkg/script/handlers.go | grep -c 'OpBusy:'`
Expected: 1 (the new dispatch entry).

---

## Bundle B1 — `OpLineOfSight` (~20 LOC handler + ~40 LOC tests; cascade 3 → 2)

### Task B1.T0 — Fix `isLineOfSight` wrapper arg shape (prerequisite)

**Background:** Pre-flight per spec R1 confirms goscape's `isLineOfSight` wrapper at `pkg/script/handlers_map.go:184` calls `s.LineValidator.HasLineOfSight(level, srcX, srcZ, destX, destZ, 1, 0, 0, 0)`. TS calls `rsmod.hasLineOfSight(level, srcX, srcZ, destX, destZ, 1, 1, 1, 1, 0)` (verified at `LostCityRS/Engine-TS/src/engine/GameMap.ts:429-431` and `2004scape/rsmod-pathfinder/src/index.ts:355-379`). Goscape's `srcSize` expands to `srcWidth=srcLength=1` inside `RayCast` (`pkg/pathfinder/routefinder/linevalidator.go:21`), so the divergence is **destWidth/destLength=0 vs TS=1**. This affects the ray-endpoint computation in `lineCoordinate` and is a pre-existing bug that propagates to existing callers (`MapFindSquareLineOfSight` at `handlers_map.go:119-120, 150-151`).

Open deviation tag: `NAI-163-D-LOS-ARG-SHAPE-FIX` — documents the wrapper widening as a TS-fidelity correction; existing callers gain TS-faithful endpoint semantics.

**Files:**
- Modify: `pkg/script/handlers_map.go:184`
- Test: `pkg/script/handlers_map_test.go` — regression pin

- [ ] **Step 1: Write a regression test for the wrapper**

Add to `pkg/script/handlers_map_test.go` (re-grep for `func TestIsLineOfSight\|stubLineValidator` to find the existing stub-validator pattern; if absent, use this minimal recording stub):

```go
// stubLineValidatorArgs records every (Has)LineOfSight call args for
// arg-shape regression tests. NAI-163-D-LOS-ARG-SHAPE-FIX.
type stubLineValidatorArgs struct {
	losCalls []losCall
	losReturn bool
}

type losCall struct {
	level, srcX, srcZ, destX, destZ                int
	srcSize, destWidth, destLength, extraFlag      int
}

func (st *stubLineValidatorArgs) HasLineOfSight(level, srcX, srcZ, destX, destZ, srcSize, destWidth, destLength, extraFlag int) bool {
	st.losCalls = append(st.losCalls, losCall{level, srcX, srcZ, destX, destZ, srcSize, destWidth, destLength, extraFlag})
	return st.losReturn
}

func (st *stubLineValidatorArgs) HasLineOfWalk(level, srcX, srcZ, destX, destZ, srcSize, destWidth, destLength, extraFlag int) bool {
	return true
}

func TestIsLineOfSightWrapper_PassesTSFaithfulArgShape(t *testing.T) {
	// Regression pin: TS GameMap.ts:430 calls
	//   rsmod.hasLineOfSight(level, sX, sZ, dX, dZ, 1, 1, 1, 1, 0)
	// goscape's srcSize expands to srcWidth=srcLength=1 inside RayCast,
	// so the TS-faithful argument tuple at the wrapper level is
	// srcSize=1, destWidth=1, destLength=1, extraFlag=0.
	// Pre-NAI-163-D-LOS-ARG-SHAPE-FIX the wrapper was (1, 0, 0, 0).
	st := &stubLineValidatorArgs{losReturn: true}
	s := &ScriptState{LineValidator: st}
	_ = isLineOfSight(s, 0, 3200, 3300, 3210, 3305)
	if len(st.losCalls) != 1 {
		t.Fatalf("expected 1 LineValidator call, got %d", len(st.losCalls))
	}
	got := st.losCalls[0]
	want := losCall{level: 0, srcX: 3200, srcZ: 3300, destX: 3210, destZ: 3305, srcSize: 1, destWidth: 1, destLength: 1, extraFlag: 0}
	if got != want {
		t.Fatalf("isLineOfSight arg shape mismatch:\n got=%+v\nwant=%+v", got, want)
	}
}
```

- [ ] **Step 2: Run test to verify it FAILS (current wrapper is `(1, 0, 0, 0)`)**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/... -run TestIsLineOfSightWrapper_PassesTSFaithfulArgShape -v`
Expected: FAIL — `destWidth: 0, destLength: 0` (current bug).

- [ ] **Step 3: Patch the wrapper**

Edit `pkg/script/handlers_map.go:178-185`. Replace:

```go
// isLineOfSight delegates to s.LineValidator. See isLineOfWalk for arg-shape
// rationale. NAI-35-T6.
func isLineOfSight(s *ScriptState, level, srcX, srcZ, destX, destZ int) bool {
	if s.LineValidator == nil {
		return true
	}
	return s.LineValidator.HasLineOfSight(level, srcX, srcZ, destX, destZ, 1, 0, 0, 0)
}
```

With:

```go
// isLineOfSight delegates to s.LineValidator. Mirrors TS
// GameMap.ts:429-431: rsmod.hasLineOfSight(level, sX, sZ, dX, dZ, 1, 1, 1, 1, 0).
// goscape's srcSize collapses TS srcWidth+srcHeight (both 1) into a single
// arg via RayCast's `srcSize, srcSize` (linevalidator.go:21); destWidth and
// destLength are passed verbatim. NAI-163-D-LOS-ARG-SHAPE-FIX widens this
// wrapper from the pre-fix (1, 0, 0, 0) shape to TS-faithful (1, 1, 1, 0);
// existing MapFindSquareLineOfSight callers at lines 119-120, 150-151
// inherit the corrected endpoint semantics. NAI-35-T6 (NAI-163 B1 T0).
func isLineOfSight(s *ScriptState, level, srcX, srcZ, destX, destZ int) bool {
	if s.LineValidator == nil {
		return true
	}
	return s.LineValidator.HasLineOfSight(level, srcX, srcZ, destX, destZ, 1, 1, 1, 0)
}
```

(Leave `isLineOfWalk` at lines 166-176 unchanged — out of NAI-163 scope per spec §8; tracked separately if the cascade widens.)

- [ ] **Step 4: Run regression + existing MapFindSquare tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/... -run 'TestIsLineOfSightWrapper|TestMapFindSquare|TestHandleMapFindSquare' -v`
Expected: regression PASSES. If `TestMapFindSquare*` tests **fail** because they encoded the buggy `(1, 0, 0, 0)` shape via collision-flag fixtures, this is the expected propagation of the fix — investigate each failure: a real TS-fidelity regression means the test fixture was tuned to the buggy wrapper. Update the test fixture (don't revert the wrapper). If failures are widespread (more than ~3 tests), STOP and escalate to controller — the propagation surface is larger than expected.

- [ ] **Step 5: Run the full package suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/... ./modules/world/...`
Expected: PASS. Resolve any propagated failures per Step 4 guidance.

### Task B1.T1 — Write failing handler tests

**Files:**
- Test: `pkg/script/handlers_map_test.go`

- [ ] **Step 1: Inspect existing fixture pattern for `handleMapBlocked` / `handleLineOfWalk`**

Run: `grep -n "TestHandleMapBlocked\|TestHandleLineOfWalk\|mockWorldVars\|stubWorldVars\|configsView" pkg/script/handlers_map_test.go`

Mirror that fixture: the test needs a `WorldVars` mock supporting `MapMembers() int` and `IsFreeToPlay(x, z) bool`. If a `mockWorldVars` already exists with the right surface, reuse it; if it lacks a field for these two methods, extend it.

- [ ] **Step 2: Write the six tests**

```go
// mockWorldVarsLOS — minimal MapMembers/IsFreeToPlay surface for LOS tests.
// If the file already has a broader mockWorldVars, extend it instead and drop
// this local mock.
type mockWorldVarsLOS struct {
	mapMembers int
	f2pTiles   map[[2]int]bool
}

func (m *mockWorldVarsLOS) MapMembers() int             { return m.mapMembers }
func (m *mockWorldVarsLOS) IsFreeToPlay(x, z int) bool  { return m.f2pTiles[[2]int{x, z}] }
// ... (implement the remaining WorldVars methods as no-ops / zero returns
// if no shared mock is available — plan-author re-greps file for existing
// helper before duplicating the full surface)

func packCoord(level, x, z int) int { return (level << 28) | (x << 14) | z }

func TestHandleLineOfSight_LevelMismatch_PushZero(t *testing.T) {
	st := &stubLineValidatorArgs{losReturn: true}
	w := &mockWorldVarsLOS{mapMembers: 1}
	s := &ScriptState{
		LineValidator: st,
		World:         w,
		StackCapacity: 4,
	}
	// pops c2 first (top), then c1
	s.PushInt(packCoord(0, 3200, 3300)) // from (c1)
	s.PushInt(packCoord(1, 3200, 3300)) // to   (c2)
	if err := handleLineOfSight(s); err != nil {
		t.Fatalf("handleLineOfSight: %v", err)
	}
	if got := s.PopInt(); got != 0 {
		t.Fatalf("expected push 0 on level mismatch, got %d", got)
	}
	if len(st.losCalls) != 0 {
		t.Fatalf("LineValidator must not be called on level mismatch; got %d calls", len(st.losCalls))
	}
}

func TestHandleLineOfSight_F2PGate_NonMembersWorld_PushZero(t *testing.T) {
	st := &stubLineValidatorArgs{losReturn: true}
	w := &mockWorldVarsLOS{mapMembers: 0, f2pTiles: map[[2]int]bool{}}
	s := &ScriptState{
		LineValidator: st,
		World:         w,
		StackCapacity: 4,
	}
	s.PushInt(packCoord(0, 3200, 3300))
	s.PushInt(packCoord(0, 3210, 3305)) // dest not in f2pTiles → IsFreeToPlay=false
	if err := handleLineOfSight(s); err != nil {
		t.Fatalf("handleLineOfSight: %v", err)
	}
	if got := s.PopInt(); got != 0 {
		t.Fatalf("expected push 0 on F2P gate, got %d", got)
	}
	if len(st.losCalls) != 0 {
		t.Fatalf("LineValidator must not be called when F2P gate trips; got %d calls", len(st.losCalls))
	}
}

func TestHandleLineOfSight_F2PGate_MembersWorld_Bypasses(t *testing.T) {
	st := &stubLineValidatorArgs{losReturn: true}
	w := &mockWorldVarsLOS{mapMembers: 1, f2pTiles: map[[2]int]bool{}}
	s := &ScriptState{
		LineValidator: st,
		World:         w,
		StackCapacity: 4,
	}
	s.PushInt(packCoord(0, 3200, 3300))
	s.PushInt(packCoord(0, 3210, 3305))
	if err := handleLineOfSight(s); err != nil {
		t.Fatalf("handleLineOfSight: %v", err)
	}
	if got := s.PopInt(); got != 1 {
		t.Fatalf("expected push 1 (clear ray, members world bypasses F2P gate), got %d", got)
	}
	if len(st.losCalls) != 1 {
		t.Fatalf("LineValidator must be called when F2P gate bypassed; got %d calls", len(st.losCalls))
	}
}

func TestHandleLineOfSight_RayClear_PushOne(t *testing.T) {
	st := &stubLineValidatorArgs{losReturn: true}
	w := &mockWorldVarsLOS{mapMembers: 1}
	s := &ScriptState{
		LineValidator: st,
		World:         w,
		StackCapacity: 4,
	}
	s.PushInt(packCoord(0, 3200, 3300))
	s.PushInt(packCoord(0, 3210, 3305))
	if err := handleLineOfSight(s); err != nil {
		t.Fatalf("handleLineOfSight: %v", err)
	}
	if got := s.PopInt(); got != 1 {
		t.Fatalf("expected push 1, got %d", got)
	}
}

func TestHandleLineOfSight_RayBlocked_PushZero(t *testing.T) {
	st := &stubLineValidatorArgs{losReturn: false}
	w := &mockWorldVarsLOS{mapMembers: 1}
	s := &ScriptState{
		LineValidator: st,
		World:         w,
		StackCapacity: 4,
	}
	s.PushInt(packCoord(0, 3200, 3300))
	s.PushInt(packCoord(0, 3210, 3305))
	if err := handleLineOfSight(s); err != nil {
		t.Fatalf("handleLineOfSight: %v", err)
	}
	if got := s.PopInt(); got != 0 {
		t.Fatalf("expected push 0 (ray blocked), got %d", got)
	}
}

func TestHandleLineOfSight_ArgShape(t *testing.T) {
	// Pins the TS-faithful wrapper arg shape end-to-end through the handler:
	// srcSize=1, destWidth=1, destLength=1, extraFlag=0. NAI-163 R1 mitigation.
	st := &stubLineValidatorArgs{losReturn: true}
	w := &mockWorldVarsLOS{mapMembers: 1}
	s := &ScriptState{
		LineValidator: st,
		World:         w,
		StackCapacity: 4,
	}
	s.PushInt(packCoord(0, 3200, 3300))
	s.PushInt(packCoord(0, 3210, 3305))
	if err := handleLineOfSight(s); err != nil {
		t.Fatalf("handleLineOfSight: %v", err)
	}
	if len(st.losCalls) != 1 {
		t.Fatalf("expected 1 LineValidator call, got %d", len(st.losCalls))
	}
	got := st.losCalls[0]
	want := losCall{level: 0, srcX: 3200, srcZ: 3300, destX: 3210, destZ: 3305, srcSize: 1, destWidth: 1, destLength: 1, extraFlag: 0}
	if got != want {
		t.Fatalf("LineValidator arg shape mismatch:\n got=%+v\nwant=%+v", got, want)
	}
}
```

**Note on `packCoord`:** verify whether this helper already exists in `pkg/script/handlers_map_test.go` or a shared test helper file. Re-grep `grep -n "func packCoord\|func PackCoord" pkg/script/`. If present, reuse it. If absent, the inline helper above is acceptable for these tests only.

**Note on `WorldVars` mock surface:** if the existing test file uses `s.World = some_existing_mock`, extend that mock instead of adding `mockWorldVarsLOS`. The new tests need only `MapMembers` + `IsFreeToPlay` exposed correctly; other methods can be no-ops/zero-returns.

- [ ] **Step 3: Run tests to verify they FAIL**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/... -run TestHandleLineOfSight_ -v`
Expected: FAIL — `undefined: handleLineOfSight`.

### Task B1.T2 — Implement `handleLineOfSight`

**Files:**
- Modify: `pkg/script/handlers_map.go` — add handler after the `isLineOfSight` helper (i.e., after line 185)
- Modify: `pkg/script/handlers.go` — add dispatch entry

- [ ] **Step 1: Implement handler**

Insert in `pkg/script/handlers_map.go` after the `isLineOfSight` helper:

```go
// handleLineOfSight (LINEOFSIGHT, opcode 1005) pops [from, to] coords and
// pushes 1 iff a line-of-sight ray from `from` to `to` is clear. Mirrors TS
// ServerOps.ts:144-162:
//
//	const [c1, c2] = state.popInts(2);
//	const from: CoordGrid = check(c1, CoordValid);
//	const to:   CoordGrid = check(c2, CoordValid);
//	if (from.level !== to.level) { state.pushInt(0); return; }
//	if (!NODE_MEMBERS && !World.gameMap.isFreeToPlay(to.x, to.z)) {
//	    state.pushInt(0); return;
//	}
//	state.pushInt(isLineOfSight(from.level, from.x, from.z, to.x, to.z) ? 1 : 0);
//
// Pop order (top first): c2 (to), c1 (from). Gate order pinned by tests:
// level-mismatch fires before F2P gate, which fires before LineValidator.
// NAI-163 B1.
func handleLineOfSight(s *ScriptState) error {
	c2 := s.PopInt()
	c1 := s.PopInt()
	fromLevel, fromX, fromZ, err := checkCoord(c1, "LINEOFSIGHT")
	if err != nil {
		return err
	}
	toLevel, toX, toZ, err := checkCoord(c2, "LINEOFSIGHT")
	if err != nil {
		return err
	}
	if fromLevel != toLevel {
		s.PushInt(0)
		return nil
	}
	if s.World.MapMembers() == 0 && !s.World.IsFreeToPlay(toX, toZ) {
		s.PushInt(0)
		return nil
	}
	if isLineOfSight(s, fromLevel, fromX, fromZ, toX, toZ) {
		s.PushInt(1)
	} else {
		s.PushInt(0)
	}
	return nil
}
```

- [ ] **Step 2: Register dispatch**

Edit `pkg/script/handlers.go` line 133 — find:

```go
	OpLineOfWalk:  handleLineOfWalk,
```

Insert immediately above it:

```go
	OpLineOfSight: handleLineOfSight,
```

(Match field-name padding.)

- [ ] **Step 3: Run tests to verify they PASS**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/... -run TestHandleLineOfSight_ -v`
Expected: PASS (6 tests).

- [ ] **Step 4: Run the full package suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/... ./modules/world/...`
Expected: PASS.

### Task B1.T3 — Close commit B1

- [ ] **Step 1: Stage + commit**

```bash
git add pkg/script/handlers_map.go pkg/script/handlers.go pkg/script/handlers_map_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): NAI-163 B1 — OpLineOfSight + LV wrapper arg-shape fix

Ports TS ServerOps.ts:144-162 (opcode 1005). Pop order (top first):
c2 (to), c1 (from). Gate order pinned: level-mismatch → F2P → LV.

Includes NAI-163-D-LOS-ARG-SHAPE-FIX: widens isLineOfSight wrapper from
(1, 0, 0, 0) to (1, 1, 1, 0) to match TS GameMap.ts:429-431's
rsmod.hasLineOfSight(..., 1, 1, 1, 1, 0). Goscape's srcSize collapses
TS srcWidth+srcHeight=1 into srcWidth=srcLength=1 inside RayCast
(linevalidator.go:21); destWidth/destLength=1 (was 0) corrects the
ray-endpoint computation in lineCoordinate. MapFindSquareLineOfSight
callers (handlers_map.go:119-120, 150-151) inherit the corrected
endpoint semantics. isLineOfWalk wrapper (line 175) left unchanged —
out of NAI-163 scope per spec §8.

Cascade-tail (tightened regex Op[A-Za-z][A-Za-z0-9]*): 3 → 2.

Closes memory: tracker_expected_value_premise_pretrace.md,
audit_subagent_fabrication.md

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

- [ ] **Step 2: Verify**

Run: `git show HEAD --stat`
Expected: only the 3 files. `OpLineOfSight: handleLineOfSight,` present in `handlers.go` diff.

---

## Bundle B2 — `OpNpcHunt` (~25 LOC handler + ~50 LOC tests; cascade 2 → 1)

### Task B2.T1 — Write failing tests

**Files:**
- Test: `pkg/script/handlers_npc_test.go`

- [ ] **Step 1: Inspect existing iterator-driven test fixtures**

Run: `grep -n "TestHandleNpcHuntAll\|TestHandleNpcFindNext\|NpcHuntAll\|NewHuntAllNpcIterator" pkg/script/handlers_npc_test.go pkg/script/npc_iterator_test.go`

Read `handleNpcFindNext` tests for the `Stale`/iterator pattern, and the existing `mockNpc` / `mockNpcLookup` definitions in `handlers_npc_test.go`. The new tests must drive these mocks plus a `mockLineValidator` (re-grep `pkg/script/npc_iterator_test.go:304` for `stub`/`stubLineValidator` definition — likely exists as `stubLineValidator` taking a `losReturn`/`lowReturn`).

- [ ] **Step 2: Write the seven tests**

```go
func TestHandleNpcHunt_NilNpcs_PushZero(t *testing.T) {
	s := &ScriptState{
		StackCapacity: 4,
		Script:        &Script{IntOperands: []int{0}},
		World:         &mockWorldVarsTick{tick: 0},
	}
	s.Npcs = nil
	s.PushInt(packCoord(0, 3200, 3300)) // coord
	s.PushInt(5)                        // distance
	s.PushInt(int(objtype.HuntVisOff))  // huntvis (top)
	if err := handleNpcHunt(s); err != nil {
		t.Fatalf("handleNpcHunt: %v", err)
	}
	if got := s.PopInt(); got != 0 {
		t.Fatalf("expected push 0 on nil Npcs, got %d", got)
	}
	if s.ActiveNpc != nil {
		t.Fatalf("ActiveNpc must remain nil")
	}
}

func TestHandleNpcHunt_NoNpcsInRange_PushZero(t *testing.T) {
	s := newNpcHuntFixture(t, 0 /* tick */, nil /* npcs */)
	s.PushInt(packCoord(0, 3200, 3300))
	s.PushInt(5)
	s.PushInt(int(objtype.HuntVisOff))
	if err := handleNpcHunt(s); err != nil {
		t.Fatalf("handleNpcHunt: %v", err)
	}
	if got := s.PopInt(); got != 0 {
		t.Fatalf("expected push 0, got %d", got)
	}
	if s.ActiveNpc != nil {
		t.Fatalf("ActiveNpc must remain nil")
	}
}

func TestHandleNpcHunt_SingleNpc_PicksIt(t *testing.T) {
	target := &mockNpc{nid: 1, typeID: 42, x: 3201, z: 3300, level: 0, uid: (42 << 16) | 1}
	s := newNpcHuntFixture(t, 0, []*mockNpc{target})
	s.PushInt(packCoord(0, 3200, 3300))
	s.PushInt(5)
	s.PushInt(int(objtype.HuntVisOff))
	if err := handleNpcHunt(s); err != nil {
		t.Fatalf("handleNpcHunt: %v", err)
	}
	if got := s.PopInt(); got != 1 {
		t.Fatalf("expected push 1, got %d", got)
	}
	if s.ActiveNpc != target {
		t.Fatalf("ActiveNpc = %v, want %v", s.ActiveNpc, target)
	}
	if s.Pointers&PtrActiveNpc == 0 {
		t.Fatalf("PtrActiveNpc must be set")
	}
}

func TestHandleNpcHunt_PicksClosest_ByEuclideanSquared(t *testing.T) {
	// origin (3200,3300); npcs at distances²: a=4, b=1, c=9 — b should win.
	a := &mockNpc{nid: 1, typeID: 42, x: 3202, z: 3300, level: 0, uid: (42 << 16) | 1}
	b := &mockNpc{nid: 2, typeID: 42, x: 3200, z: 3301, level: 0, uid: (42 << 16) | 2}
	c := &mockNpc{nid: 3, typeID: 42, x: 3203, z: 3300, level: 0, uid: (42 << 16) | 3}
	s := newNpcHuntFixture(t, 0, []*mockNpc{a, b, c})
	s.PushInt(packCoord(0, 3200, 3300))
	s.PushInt(5)
	s.PushInt(int(objtype.HuntVisOff))
	if err := handleNpcHunt(s); err != nil {
		t.Fatalf("handleNpcHunt: %v", err)
	}
	if got := s.PopInt(); got != 1 {
		t.Fatalf("expected push 1, got %d", got)
	}
	if s.ActiveNpc != ActiveNpc(b) {
		t.Fatalf("expected b (closest), got %v", s.ActiveNpc)
	}
}

func TestHandleNpcHunt_TieBreak_PrefersLaterYield(t *testing.T) {
	// TS NpcOps.ts:307 uses `<=`. Equidistant later yield overwrites earlier.
	// origin (3200,3300); a and b both at distance²=1.
	a := &mockNpc{nid: 1, typeID: 42, x: 3201, z: 3300, level: 0, uid: (42 << 16) | 1}
	b := &mockNpc{nid: 2, typeID: 42, x: 3199, z: 3300, level: 0, uid: (42 << 16) | 2}
	s := newNpcHuntFixture(t, 0, []*mockNpc{a, b})
	s.PushInt(packCoord(0, 3200, 3300))
	s.PushInt(5)
	s.PushInt(int(objtype.HuntVisOff))
	if err := handleNpcHunt(s); err != nil {
		t.Fatalf("handleNpcHunt: %v", err)
	}
	if got := s.PopInt(); got != 1 {
		t.Fatalf("expected push 1, got %d", got)
	}
	// Iterator yields a then b; <= means b wins the tie.
	if s.ActiveNpc != ActiveNpc(b) {
		t.Fatalf("tie-break: expected b (later yield wins under `<=`), got %v", s.ActiveNpc)
	}
}

func TestHandleNpcHunt_HuntVisLineOfSight_FiltersBlocked(t *testing.T) {
	// stubLineValidator returns false for one NPC → it must be filtered.
	// Block destination at b's tile so HuntVisLineOfSight rejects it.
	a := &mockNpc{nid: 1, typeID: 42, x: 3202, z: 3300, level: 0, uid: (42 << 16) | 1}
	b := &mockNpc{nid: 2, typeID: 42, x: 3200, z: 3301, level: 0, uid: (42 << 16) | 2}
	s := newNpcHuntFixture(t, 0, []*mockNpc{a, b})
	s.LineValidator = &filteringLineValidator{blockedTiles: map[[2]int]bool{
		{3200, 3301}: true, // b blocked
	}}
	s.PushInt(packCoord(0, 3200, 3300))
	s.PushInt(5)
	s.PushInt(int(objtype.HuntVisLineOfSight))
	if err := handleNpcHunt(s); err != nil {
		t.Fatalf("handleNpcHunt: %v", err)
	}
	if got := s.PopInt(); got != 1 {
		t.Fatalf("expected push 1, got %d", got)
	}
	if s.ActiveNpc != ActiveNpc(a) {
		t.Fatalf("expected a (b filtered by LOS), got %v", s.ActiveNpc)
	}
}

func TestHandleNpcHunt_StaleIterator_ReturnsError(t *testing.T) {
	// Construct iterator at tick=0; advance world to tick=2 before handler
	// finishes — the iterator's Stale check (matches handleNpcFindNext)
	// must trip. Mock world's tick is read at iterator construction AND
	// during the Stale check; handler runs both with the same s.World, so
	// inject a world that bumps tick between calls.
	npc := &mockNpc{nid: 1, typeID: 42, x: 3201, z: 3300, level: 0, uid: (42 << 16) | 1}
	w := &tickAdvancingWorld{initial: 0, advanceTo: 100} // see helper below
	s := newNpcHuntFixtureWithWorld(t, w, []*mockNpc{npc})
	s.PushInt(packCoord(0, 3200, 3300))
	s.PushInt(5)
	s.PushInt(int(objtype.HuntVisOff))
	err := handleNpcHunt(s)
	if err == nil {
		t.Fatalf("expected stale-iterator error, got nil")
	}
	if !strings.Contains(err.Error(), "old iterator") {
		t.Fatalf("error msg should mention stale-iterator; got: %v", err)
	}
}
```

**Plan-author re-greps for `newNpcHuntFixture`** — this helper does not yet exist; you must add it adjacent to the new tests. Suggested shape:

```go
// newNpcHuntFixture constructs a ScriptState with a mock NpcLookup that
// exposes `npcs` via ZoneNpcs (matching the surface NpcIterator queries).
// Plan-author re-greps NpcIterator's lookup-surface usage in
// npc_iterator.go to confirm which methods to mock.
func newNpcHuntFixture(t *testing.T, tick int, npcs []*mockNpc) *ScriptState {
	t.Helper()
	lookup := &mockNpcLookup{ /* seed npcs into the zones the iterator walks */ }
	return &ScriptState{
		Npcs:          lookup,
		World:         &mockWorldVarsTick{tick: tick},
		StackCapacity: 4,
		Script:        &Script{IntOperands: []int{0}},
		LineValidator: &allowAllLineValidator{},
	}
}
```

**Plan-author CRITICAL pre-flight before writing this helper:** re-grep `npc_iterator.go` to see which `NpcLookup` methods `NewHuntAllNpcIterator` uses (`ZoneNpcs`, etc.), then make sure `mockNpcLookup` (defined in `handlers_npc_test.go:185+`) returns the correct seeded NPCs from those methods. If the existing mock does not support zone-keyed lookup, extend it.

**`filteringLineValidator` and `tickAdvancingWorld`:** these are new test helpers — define them adjacent to the tests. `tickAdvancingWorld.CurrentTick()` returns `initial` on first call and `advanceTo` thereafter (drives the Stale check). `filteringLineValidator.HasLineOfSight` returns false for any (destX, destZ) in `blockedTiles`.

- [ ] **Step 3: Run tests to verify FAIL**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/... -run TestHandleNpcHunt_ -v`
Expected: COMPILE FAIL — `undefined: handleNpcHunt`.

### Task B2.T2 — Implement `handleNpcHunt`

**Files:**
- Modify: `pkg/script/handlers_npc.go` (insert after `handleNpcHuntAll` at line 841)
- Modify: `pkg/script/handlers.go` (insert next to `OpNpcHuntAll` at line 497)

- [ ] **Step 1: Implement handler**

Insert in `handlers_npc.go` after `handleNpcHuntAll`:

```go
// handleNpcHunt (NPC_HUNT, opcode 2525) pops [coord, distance, huntvis] and
// selects the closest NPC by euclidean² distance from a HuntAll-mode
// iterator over zone-sweep candidates, then sets ActiveNpc + pushes 1. On
// empty iterator (no candidates), nil-Npcs, or no in-range NPCs, pushes 0.
// Mirrors TS NpcOps.ts:290-321.
//
// Pop order (top first): huntvis, distance, coord.
// Validation: checkCoord, checkNotNull(distance), checkHuntVis.
// Tie-break: TS uses `<=` (NpcOps.ts:307), so later iterator yields win
// equidistant comparisons; pinned by TestHandleNpcHunt_TieBreak_*.
//
// Iterator lifetime: LOCAL to this handler — not stored in s.npcIterator
// (unlike NPC_HUNTALL which exposes its iterator to a subsequent
// NPC_FINDNEXT). Stale-check matches handleNpcFindNext convention.
//
// NAI-163 B2.
func handleNpcHunt(s *ScriptState) error {
	huntvis := s.PopInt()
	distance := s.PopInt()
	coord := s.PopInt()

	level, x, z, err := checkCoord(coord, "NPC_HUNT")
	if err != nil {
		return err
	}
	if err := checkNotNull(distance, "NPC_HUNT"); err != nil {
		return err
	}
	if err := checkHuntVis(huntvis, "NPC_HUNT"); err != nil {
		return err
	}

	if s.Npcs == nil {
		s.PushInt(0)
		return nil
	}

	tick := s.World.CurrentTick()
	it := NewHuntAllNpcIterator(s.Npcs, s.LineValidator, tick, level, x, z, distance, huntvis)

	var closest ActiveNpc
	closestDist := math.MaxInt
	for {
		if it.Stale(s.World.CurrentTick()) {
			return fmt.Errorf("NPC_HUNT: tried to use an old iterator. Create a new iterator instead.")
		}
		npc, ok := it.Next()
		if !ok {
			break
		}
		dx := npc.NpcX() - x
		dz := npc.NpcZ() - z
		d := dx*dx + dz*dz
		if d <= closestDist {
			closest = npc
			closestDist = d
		}
	}

	if closest == nil {
		s.PushInt(0)
		return nil
	}
	setActiveNpcSlot(s, closest)
	s.PushInt(1)
	return nil
}
```

**Note on imports:** ensure `handlers_npc.go` imports `"math"` (re-grep existing imports — currently imports `errors`, `fmt`, `objtype`). Add `"math"` to the import block.

- [ ] **Step 2: Register dispatch**

Edit `pkg/script/handlers.go` line 497 — find:

```go
	OpNpcHuntAll: handleNpcHuntAll,
```

Insert immediately above it:

```go
	OpNpcHunt:    handleNpcHunt,
```

- [ ] **Step 3: Run tests to verify PASS**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/... -run TestHandleNpcHunt_ -v`
Expected: PASS (7 tests).

- [ ] **Step 4: Run full package suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/... ./modules/world/...`
Expected: PASS.

### Task B2.T3 — Close commit B2

- [ ] **Step 1: Stage + commit**

```bash
git add pkg/script/handlers_npc.go pkg/script/handlers.go pkg/script/handlers_npc_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): NAI-163 B2 — OpNpcHunt handler (opcode 2525)

Ports TS NpcOps.ts:290-321. Pop order (top first): huntvis, distance,
coord. Reuses NewHuntAllNpcIterator (NAI-35-T3); iterator is LOCAL to
the handler — not stored in s.npcIterator (distinct from NPC_HUNTALL).

Tie-break per TS NpcOps.ts:307 `<=`: later iterator yield wins on
equidistant comparisons. Pinned by TestHandleNpcHunt_TieBreak_*.

Stale-iterator handling matches handleNpcFindNext.

Cascade-tail (tightened regex Op[A-Za-z][A-Za-z0-9]*): 2 → 1.

Closes memory: ts_asymmetry_dual_pin.md, iterator_state_pattern.md

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

- [ ] **Step 2: Verify**

Run: `git show HEAD --stat`
Expected: 3 files; `OpNpcHunt: handleNpcHunt,` present in `handlers.go` diff.

---

## Bundle B3 — `OpNpcAdd` + `AddNpcAt` adapter (~20 + ~30 + ~50 LOC tests; cascade 1 → 0)

### Task B3.T1 — Add `AddNpcAt` to `WorldVars` interface

**Files:**
- Modify: `pkg/script/state.go` (insert in `WorldVars` interface near `RemoveNpc` at line 110)

- [ ] **Step 1: Insert interface method**

Edit `pkg/script/state.go`. Find the `RemoveNpc` block in the `WorldVars` interface (line 104-110). Insert immediately after `RemoveNpc(npc ActiveNpc, duration int)`:

```go
	// AddNpcAt spawns a new despawn-lifecycle NPC of `typeID` at (level, x, z)
	// with the given despawn `duration` in ticks. duration=-1 means no
	// scheduled despawn (TS DurationValid permits this — caller is
	// responsible for explicit removeNpc). Returns the spawned ActiveNpc on
	// success or an error if the NPC registry is full or typeID is unknown.
	// Mirrors TS NpcOps.ts:42-53 (NPC_ADD) + World.addNpc at
	// World.ts:1258-1294. Routes through (*Server).addNpc with
	// firstSpawn=true, hard-setting EntityLifeCycle.DESPAWN. NAI-163 B3.
	AddNpcAt(level, x, z, typeID, duration int) (ActiveNpc, error)
```

- [ ] **Step 2: Compile check (will FAIL until adapter lands)**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...`
Expected: build FAILS — `worldVarsView` does not implement `WorldVars` (missing `AddNpcAt`).

### Task B3.T2 — Implement adapter in `modules/world/server_varp.go`

**Files:**
- Modify: `modules/world/server_varp.go` (insert after `RemoveNpc` at line 152)

- [ ] **Step 1: Implement adapter**

Insert in `server_varp.go` after `RemoveNpc`:

```go
// AddNpcAt implements script.WorldVars.AddNpcAt. Looks up the NpcType,
// constructs a despawn-lifecycle Npc via NewNpc (overriding the default
// RESPAWN lifecycle to DESPAWN), and routes through (*Server).addNpc
// with firstSpawn=true. Bubbles errNpcsFull on registry-full; returns a
// defensive error on unknown typeID (TS-side checkNpcType already
// rejects this case at the handler, so this branch is goscape-defensive;
// TS skips this check). Mirrors TS World.addNpc consumer pattern at
// NpcOps.ts:42-53. NAI-163 B3.
func (w worldVarsView) AddNpcAt(level, x, z, typeID, duration int) (script.ActiveNpc, error) {
	if typeID < 0 || typeID >= len(w.s.npcTypes.Configs) {
		return nil, fmt.Errorf("AddNpcAt: typeID %d out of range", typeID)
	}
	typ := w.s.npcTypes.Configs[typeID]
	if typ == nil {
		return nil, fmt.Errorf("AddNpcAt: no NpcType for id %d", typeID)
	}
	n := NewNpc(0 /* nid; allocated inside addNpc */, typeID, x, z, level, typ)
	n.lifecycle = NpcLifecycleDespawn
	if err := w.s.addNpc(n, duration, true); err != nil {
		return nil, err
	}
	return n, nil
}
```

**Plan-author re-grep:** confirm `server_varp.go` already imports `"fmt"` and `"github.com/zsrv/goscape/pkg/script"`. If `"fmt"` is missing, add it. The file should already import `script` (used by the `RemoveNpc` signature above).

- [ ] **Step 2: Compile check**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...`
Expected: PASS.

### Task B3.T3 — Write failing adapter tests

**Files:**
- Test: `modules/world/npc_registry_test.go` (or new `modules/world/npc_addat_test.go` — plan-author re-greps for `TestAddNpc\|TestServerAddNpc\|npc_registry_test` setup boilerplate; if `npc_registry_test.go` already has a fixture builder reusable for these tests, extend it; otherwise create `npc_addat_test.go`)

- [ ] **Step 1: Inspect existing test scaffolding**

Run: `grep -n "^func Test\|newTestServer\|func .s .Server. addNpc\|errNpcsFull" modules/world/npc_registry_test.go 2>/dev/null | head -30`

Identify how existing tests construct a `*Server` and seed `npcTypes`. Mirror that pattern.

- [ ] **Step 2: Write the five adapter tests**

```go
func TestAddNpcAt_AllocsNidAndRegisters(t *testing.T) {
	s := newServerForNpcAddAt(t) // builds *Server with seeded npcTypes, gamemap, zoneMap
	w := worldVarsView{s: s}
	npc, err := w.AddNpcAt(0, 3200, 3300, 1 /* typeID seeded in newServerForNpcAddAt */, -1)
	if err != nil {
		t.Fatalf("AddNpcAt: %v", err)
	}
	real, ok := npc.(*Npc)
	if !ok {
		t.Fatalf("AddNpcAt returned %T, want *Npc", npc)
	}
	if real.nid < 1 || real.nid >= len(s.npcs) {
		t.Fatalf("nid out of range: %d", real.nid)
	}
	if s.npcs[real.nid] != real {
		t.Fatalf("Npc not registered at s.npcs[%d]", real.nid)
	}
	found := false
	for _, n := range s.npcLoop {
		if n == real {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("Npc not appended to s.npcLoop")
	}
}

func TestAddNpcAt_SetsDespawnLifecycle(t *testing.T) {
	s := newServerForNpcAddAt(t)
	w := worldVarsView{s: s}
	npc, err := w.AddNpcAt(0, 3200, 3300, 1, -1)
	if err != nil {
		t.Fatalf("AddNpcAt: %v", err)
	}
	real := npc.(*Npc)
	if real.lifecycle != NpcLifecycleDespawn {
		t.Fatalf("lifecycle = %d, want NpcLifecycleDespawn (%d)", real.lifecycle, NpcLifecycleDespawn)
	}
}

func TestAddNpcAt_WritesLifecycleTick(t *testing.T) {
	s := newServerForNpcAddAt(t)
	w := worldVarsView{s: s}
	npc, err := w.AddNpcAt(0, 3200, 3300, 1, 50)
	if err != nil {
		t.Fatalf("AddNpcAt: %v", err)
	}
	real := npc.(*Npc)
	if real.lifecycleTick != 50 {
		t.Fatalf("lifecycleTick = %d, want 50", real.lifecycleTick)
	}
}

func TestAddNpcAt_RegistryFull_ReturnsErrNpcsFull(t *testing.T) {
	s := newServerForNpcAddAt(t)
	// Fill all slots — fixture must seed s.npcs as fully-occupied except
	// slot 0 (unused). Implementer: re-grep newServerForNpcAddAt and
	// expose a "fill" hook, or construct a minimal *Server inline with
	// s.npcs initialised to a 2-element array with slot 1 pre-filled.
	for i := 1; i < len(s.npcs); i++ {
		if s.npcs[i] == nil {
			s.npcs[i] = &Npc{nid: i, typeId: 1}
		}
	}
	w := worldVarsView{s: s}
	_, err := w.AddNpcAt(0, 3200, 3300, 1, -1)
	if !errors.Is(err, errNpcsFull) {
		t.Fatalf("err = %v, want errNpcsFull", err)
	}
}

func TestAddNpcAt_PopulatesSizeBlockWalkMoveRestrict(t *testing.T) {
	s := newServerForNpcAddAt(t) // typeID=1 seeded with Size=2, BlockWalk=BlockWalkNPC, MoveRestrict=indoors
	w := worldVarsView{s: s}
	npc, err := w.AddNpcAt(0, 3200, 3300, 1, -1)
	if err != nil {
		t.Fatalf("AddNpcAt: %v", err)
	}
	real := npc.(*Npc)
	typ := s.npcTypes.Configs[1]
	if real.size != int(typ.Size) {
		t.Fatalf("size = %d, want %d", real.size, typ.Size)
	}
	if real.blockWalk != typ.BlockWalk {
		t.Fatalf("blockWalk = %v, want %v", real.blockWalk, typ.BlockWalk)
	}
	if real.moveRestrict != MoveRestrict(typ.MoveRestrict) {
		t.Fatalf("moveRestrict = %v, want %v", real.moveRestrict, typ.MoveRestrict)
	}
}
```

**Plan-author CRITICAL pre-flight before writing `newServerForNpcAddAt`:** re-grep `modules/world/server.go` for `*Server` construction patterns used in existing tests (`newTestServer`, etc.). The fixture must:
1. Allocate `s.npcs` (typically `s.npcs = make([]*Npc, N)` for some N — check production sizing).
2. Initialise `s.nextNpcSlot = 1`.
3. Seed `s.npcTypes = &objtype.NPCTypeConfigs{Configs: []*objtype.NpcType{nil, {Id: 1, Size: 2, BlockWalk: objtype.BlockWalkNPC, ...}}}`.
4. Initialise `s.zoneMap` (or accept that addNpc's `if s.zoneMap != nil` guard skips zone-enter; the test still validates registry + lifecycle).
5. Leave `s.gamemap = nil` so the collision-flag toggle is skipped (or wire a minimal `gamemap` fixture if existing tests do so).
6. Leave `s.scriptProvider = nil` so the AI_SPAWN producer is skipped.

Document the helper inline in the test file. **Do not invent fields not present on `*Server`** — re-grep `type Server struct` at `server.go:60+` and use the exact field names.

- [ ] **Step 3: Run tests to verify FAIL**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestAddNpcAt_ -v`
Expected: compile if helpers exist; runtime FAIL on Step 2 tests (or compile-fail if `newServerForNpcAddAt` doesn't exist yet — define it).

### Task B3.T4 — Implement adapter (already done in B3.T2 — adapter tests now pass)

- [ ] **Step 1: Run adapter tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestAddNpcAt_ -v`
Expected: PASS (5 tests).

- [ ] **Step 2: Verify full `modules/world` suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/...`
Expected: PASS.

### Task B3.T5 — Write failing handler tests

**Files:**
- Test: `pkg/script/handlers_npc_test.go`

- [ ] **Step 1: Extend mock WorldVars**

The handler tests need a `WorldVars` mock with `AddNpcAt` exposed. Re-grep `pkg/script/handlers_npc_test.go` for the existing world mock used by `TestHandleNpcHuntAll` and similar tests. Extend that mock with:

```go
// On the existing mock world struct (rename addNpcAtFn etc. to match the
// file's mock-naming convention):
addNpcAtFn func(level, x, z, typeID, duration int) (ActiveNpc, error)

func (m *mockWorldVars) AddNpcAt(level, x, z, typeID, duration int) (ActiveNpc, error) {
	if m.addNpcAtFn == nil {
		return nil, errors.New("addNpcAt not stubbed")
	}
	return m.addNpcAtFn(level, x, z, typeID, duration)
}
```

If the existing world mock is shared across `pkg/script/...` test files, ALL files that build it as a literal need the new field; using a function pointer avoids that (zero value is safely-nil and the method returns a default error). If multiple distinct world mocks exist, extend each that may be used in `handlers_npc_test.go`.

- [ ] **Step 2: Write the five handler tests**

```go
func TestHandleNpcAdd_Success_SetsActiveNpc(t *testing.T) {
	spawned := &mockNpc{nid: 7, typeID: 42, x: 3200, z: 3300, level: 0, uid: (42 << 16) | 7}
	called := 0
	world := &mockWorldVars{
		addNpcAtFn: func(level, x, z, typeID, duration int) (ActiveNpc, error) {
			called++
			if level != 0 || x != 3200 || z != 3300 || typeID != 42 || duration != 100 {
				t.Fatalf("AddNpcAt args: level=%d x=%d z=%d typeID=%d duration=%d", level, x, z, typeID, duration)
			}
			return spawned, nil
		},
	}
	s := &ScriptState{
		StackCapacity: 4,
		Script:        &Script{IntOperands: []int{0}},
		World:         world,
		Configs:       newTestConfigsWithNpcTypes(map[int]bool{42: true}),
	}
	// Pop order (top first): duration, id, coord — push bottom first.
	s.PushInt(packCoord(0, 3200, 3300))
	s.PushInt(42)
	s.PushInt(100)
	if err := handleNpcAdd(s); err != nil {
		t.Fatalf("handleNpcAdd: %v", err)
	}
	if called != 1 {
		t.Fatalf("AddNpcAt called %d times, want 1", called)
	}
	if s.ActiveNpc != ActiveNpc(spawned) {
		t.Fatalf("ActiveNpc = %v, want %v", s.ActiveNpc, spawned)
	}
	if s.Pointers&PtrActiveNpc == 0 {
		t.Fatalf("PtrActiveNpc must be set")
	}
	if s.SP != 0 {
		// TS handler does not push — stack must be empty after pops + no push.
		t.Fatalf("expected stack empty (no push); SP=%d", s.SP)
	}
}

func TestHandleNpcAdd_RegistryFull_ReturnsError(t *testing.T) {
	world := &mockWorldVars{
		addNpcAtFn: func(level, x, z, typeID, duration int) (ActiveNpc, error) {
			return nil, errors.New("npc registry full")
		},
	}
	s := &ScriptState{
		StackCapacity: 4,
		Script:        &Script{IntOperands: []int{0}},
		World:         world,
		Configs:       newTestConfigsWithNpcTypes(map[int]bool{42: true}),
	}
	s.PushInt(packCoord(0, 3200, 3300))
	s.PushInt(42)
	s.PushInt(100)
	err := handleNpcAdd(s)
	if err == nil {
		t.Fatalf("expected error on registry full, got nil")
	}
}

func TestHandleNpcAdd_InvalidCoord_ReturnsError(t *testing.T) {
	world := &mockWorldVars{}
	s := &ScriptState{
		StackCapacity: 4,
		Script:        &Script{IntOperands: []int{0}},
		World:         world,
		Configs:       newTestConfigsWithNpcTypes(map[int]bool{42: true}),
	}
	s.PushInt(-1) // invalid coord
	s.PushInt(42)
	s.PushInt(100)
	err := handleNpcAdd(s)
	if err == nil {
		t.Fatalf("expected checkCoord error on negative coord")
	}
}

func TestHandleNpcAdd_InvalidNpcType_ReturnsError(t *testing.T) {
	world := &mockWorldVars{}
	s := &ScriptState{
		StackCapacity: 4,
		Script:        &Script{IntOperands: []int{0}},
		World:         world,
		Configs:       newTestConfigsWithNpcTypes(map[int]bool{42: true}),
	}
	s.PushInt(packCoord(0, 3200, 3300))
	s.PushInt(999) // unknown type
	s.PushInt(100)
	err := handleNpcAdd(s)
	if err == nil {
		t.Fatalf("expected checkNpcType error on unknown type")
	}
}

func TestHandleNpcAdd_InvalidDuration_ReturnsError(t *testing.T) {
	world := &mockWorldVars{}
	s := &ScriptState{
		StackCapacity: 4,
		Script:        &Script{IntOperands: []int{0}},
		World:         world,
		Configs:       newTestConfigsWithNpcTypes(map[int]bool{42: true}),
	}
	s.PushInt(packCoord(0, 3200, 3300))
	s.PushInt(42)
	s.PushInt(-2) // invalid duration: re-read checkDuration at handlers_loc.go:307-320 for the exact accepted range; if -1 is accepted (TS DurationValid permits indefinite), use a value clearly outside [accepted_min, accepted_max].
	err := handleNpcAdd(s)
	if err == nil {
		t.Fatalf("expected checkDuration error on invalid duration")
	}
}
```

**Plan-author re-grep:** confirm `newTestConfigsWithNpcTypes` exists (`handlers_npc_test.go:55` — already used by existing tests).

**Plan-author re-grep for `checkDuration` accepted range:** read `handlers_loc.go:307-320` to know what value to pass for the invalid-duration test. The TS-faithful `DurationValid` is at `LostCityRS/Engine-TS/.../ScriptValidators.ts:108` — re-read to confirm semantics. If `-1` is accepted (likely, for indefinite-lifetime spawns), use `-2` or `math.MinInt32` for the invalid case.

- [ ] **Step 3: Run tests to verify FAIL**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/... -run TestHandleNpcAdd_ -v`
Expected: COMPILE FAIL — `undefined: handleNpcAdd`.

### Task B3.T6 — Implement `handleNpcAdd`

**Files:**
- Modify: `pkg/script/handlers_npc.go` (insert near top of NPC handlers — after the `setActiveNpcSlot` helper, before any existing NPC opcode handler that operates on a pre-existing ActiveNpc)
- Modify: `pkg/script/handlers.go` (insert near `OpNpcAnim: handleNpcAnim,` at line 453)

- [ ] **Step 1: Implement handler**

Insert in `handlers_npc.go` (placement: near other NPC create/spawn handlers; the file's existing ordering by opcode value suggests adjacent to `handleNpcAnim`/early in the file — plan-author chooses placement that minimises diff churn):

```go
// handleNpcAdd (NPC_ADD, opcode 2500) pops [coord, id, duration] and
// spawns a despawn-lifecycle NPC of typeID `id` at the unpacked coord.
// Mirrors TS NpcOps.ts:42-53:
//
//	const [coord, id, duration] = state.popInts(3);
//	const position = check(coord, CoordValid);
//	const npcType  = check(id,    NpcTypeValid);
//	check(duration, DurationValid);
//	const npc = new Npc(level, x, z, size, size, DESPAWN, getNextNid(),
//	    id, moverestrict, blockwalk);
//	World.addNpc(npc, duration);
//	state.activeNpc = npc;
//	state.pointerAdd(ActiveNpc[state.intOperand]);
//
// Pop order (top first): duration, id, coord. NO push on success
// (TS handler does not push). Sets ActiveNpc + PtrActiveNpc via
// setActiveNpcSlot. NAI-163 B3.
func handleNpcAdd(s *ScriptState) error {
	duration := s.PopInt()
	id := s.PopInt()
	coord := s.PopInt()

	level, x, z, err := checkCoord(coord, "NPC_ADD")
	if err != nil {
		return err
	}
	if err := checkNpcType(s, id, "NPC_ADD"); err != nil {
		return err
	}
	if err := checkDuration(duration); err != nil {
		return err
	}

	npc, err := s.World.AddNpcAt(level, x, z, id, duration)
	if err != nil {
		return err
	}
	setActiveNpcSlot(s, npc)
	return nil
}
```

- [ ] **Step 2: Register dispatch**

Edit `pkg/script/handlers.go` line 453 — find:

```go
	OpNpcAnim:              handleNpcAnim,
```

Insert immediately above it:

```go
	OpNpcAdd:               handleNpcAdd,
```

- [ ] **Step 3: Run tests to verify PASS**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/... -run TestHandleNpcAdd_ -v`
Expected: PASS (5 tests).

- [ ] **Step 4: Run full package suites**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/... ./modules/world/...`
Expected: PASS.

### Task B3.T7 — Close commit B3

- [ ] **Step 1: Stage + commit**

```bash
git add pkg/script/state.go modules/world/server_varp.go \
        pkg/script/handlers_npc.go pkg/script/handlers.go \
        pkg/script/handlers_npc_test.go modules/world/npc_registry_test.go
# (if you created modules/world/npc_addat_test.go instead, swap that in)
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): NAI-163 B3 — OpNpcAdd + AddNpcAt adapter (opcode 2500)

Ports TS NpcOps.ts:42-53. Pop order (top first): duration, id, coord.
NO push on success (TS handler does not push); sets ActiveNpc +
PtrActiveNpc.

Adds AddNpcAt(level, x, z, typeID, duration) (ActiveNpc, error) to the
WorldVars interface. modules/world adapter constructs a
despawn-lifecycle Npc via NewNpc + n.lifecycle=NpcLifecycleDespawn
(NewNpc default is NpcLifecycleRespawn; resetEntityForRespawn does not
touch lifecycle), then routes through (*Server).addNpc with
firstSpawn=true. errNpcsFull bubbles handler-side; unknown typeID
returns a goscape-defensive error (TS-side checkNpcType already
rejects at the handler).

R3 pin: TestAddNpcAt_SetsDespawnLifecycle locks NpcLifecycleDespawn
(not Respawn) — guards script-spawned NPCs against surviving across
server restart.

Cascade-tail (tightened regex Op[A-Za-z][A-Za-z0-9]*): 1 → 0.

Closes memory: defensive_gate_doc_comment_label.md, plan_type_name_grep.md

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

- [ ] **Step 2: Verify**

Run: `git show HEAD --stat`
Expected: 5-6 files (depending on test-file placement choice).

---

## Final close — NAI-163 roll-up

### Task NAI-163.CLOSE — Tightened-regex recount + close commit

**Files:**
- No code changes — recount + close commit only.

- [ ] **Step 1: Run the tightened-regex audit at HEAD**

```bash
declared=$(grep -E "^\s*Op[A-Za-z][A-Za-z0-9]*\s+Opcode\s+=" pkg/script/opcode.go | awk '{print $1}' | sort -u)
dispatched=$(grep -E "^\s*Op[A-Za-z][A-Za-z0-9]*\s*:" pkg/script/handlers.go | awk -F: '{print $1}' | tr -d '[:space:]' | sort -u)
comm -23 <(echo "$declared") <(echo "$dispatched")
```

Expected output: **empty** (cascade-tail = 0). If any opcodes remain, STOP — the close-commit's "4 → 0" claim is wrong; investigate.

- [ ] **Step 2: Verify the four bundle commits exist**

```bash
git log --oneline -10
```

Expected: HEAD-3..HEAD contains commits with subjects:
- `feat(script): NAI-163 B0 — OpBusy handler (opcode 2005)`
- `feat(script): NAI-163 B1 — OpLineOfSight + LV wrapper arg-shape fix`
- `feat(script): NAI-163 B2 — OpNpcHunt handler (opcode 2525)`
- `feat(script): NAI-163 B3 — OpNpcAdd + AddNpcAt adapter (opcode 2500)`

- [ ] **Step 3: Run the full test suite + race detector smoke**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./pkg/script/... ./modules/world/...
```

Expected: PASS.

- [ ] **Step 4: Create empty roll-up close commit**

```bash
git commit --allow-empty --no-gpg-sign -m "$(cat <<'EOF'
chore(close): NAI-163 — cascade-tail to zero (4 ops, 4 bundles)

Closes the NAI-163 cohort: 4 unhandled opcodes drained to 0 across
four bundles. Each bundle has its own close commit (B0 → B1 → B2 → B3);
this is the roll-up.

Tightened missing-handler audit (regex Op[A-Za-z][A-Za-z0-9]*) per
missing_handler_audit_regex_flaw.md:

  Pre-NAI-163 (at 0027628): 4 opcodes (OpBusy, OpLineOfSight,
                            OpNpcAdd, OpNpcHunt)
  Post-NAI-163:              0

The original audit regex Op[A-Za-z]+ collapses OpBusy into the
dispatched OpBusy2; future cohorts must use the tightened
Op[A-Za-z][A-Za-z0-9]* pattern. missing_handler_audit.md doc update
declined in scoping vote (per spec §8); the tightened regex lives in
this commit body and in the missing_handler_audit_regex_flaw memory.

Deviations opened:
- NAI-163-D-LOS-ARG-SHAPE-FIX (B1 T0) — widens isLineOfSight wrapper
  from (1, 0, 0, 0) to (1, 1, 1, 0) to match TS rsmod call shape;
  fixes pre-existing endpoint-computation divergence inherited by
  MapFindSquareLineOfSight callers. isLineOfWalk wrapper (line 175)
  left unchanged — out of scope per spec §8.

Pinned TS-asymmetries:
- B0: loggingOut arm (TestHandleBusy_LoggingOut_PushOne)
- B2: <= tie-break (TestHandleNpcHunt_TieBreak_PrefersLaterYield)
- B3: DESPAWN lifecycle (TestAddNpcAt_SetsDespawnLifecycle)

Out of scope, deferred to future NAI-N:
- isLineOfWalk wrapper widening (mirrors LOS bug)
- OPHELD / OcOp / LcOp / OcIop content-trigger cohort
- B0-stub re-ports (PUSH_VARBIT 25, POP_VARBIT 27, SET_GENDER 2099,
  LC_OP 4105, OC_IOP 4205, OC_OP 4208)
- WealthEvent RecipientItems/RecipientValue carry-forwards
- ActivePlayer.Session() exposure (deferred per InvOps.ts:1639,1731
  notes)
- missing_handler_audit.md doc update (declined)

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

- [ ] **Step 5: Smoke handoff (user-launched per smoke_test_server_handoff.md)**

Per `smoke_test_server_handoff.md`, the Java-client-driven smoke needs the user to launch the server (sandbox cannot reach the host). Emit this handoff message to the user:

> NAI-163 ready for smoke. To bind cascade-tail attribution per `cascade_theory_smoke_binding.md`:
>
> 1. Launch the server: `CGO_ENABLED=0 go run -trimpath ./cmd/goscape --config.file config.yaml`
> 2. Connect with the Java client.
> 3. Trigger a content script that exercises each opcode (plan-author re-greps `LostCityRS/Content/scripts/` for callers of `busy` / `lineofsight` / `npc_hunt` / `npc_add` and lists the trigger NPCs/objects).
> 4. Confirm: BUSY arm flips while logging out; LOS true/false matches OSRS; NPC_HUNT picks closest; NPC_ADD spawns then despawns at duration.
>
> Report observed behavior; if any smoke fails, open a follow-up sub-spec.

---

## Self-review checklist (run after writing this plan)

### Spec coverage

- §1 motivation — covered in plan header.
- §2.1 bundle table — bundles B0/B1/B2/B3 with matching opcodes/sources.
- §2.2 TS-fidelity gates — pinned per bundle (loggingOut, gate-order, `<=`, DESPAWN).
- §3 components — all four handler implementations + AddNpcAt adapter.
- §4 data flow — encoded in handler implementations.
- §5 error handling — encoded in tests (nil-Npcs, nil-LV inherited pessimistic-allow, registry full, stale iterator, validators).
- §6 tests — all named tests reflected in plan.
- §6.5 smoke handoffs — emitted in NAI-163.CLOSE Step 5.
- §7 risk register — R1 (B1 T0 wrapper fix), R2 (B0 T1+T2 accessor add), R3 (B3 T7 close commit cites pin), R4 (B2 T1 + B2 T2 codifies `<=`), R5 (B3 T2 adapter routes through addNpc → resetEntityForRespawn which reseeds stats), R6 (NAI-163.CLOSE commit body documents tightened regex).
- §8 out of scope — listed in NAI-163.CLOSE commit body.

### Placeholder scan

No "TODO", "implement later", "fill in details" — all task code blocks contain compilable content.

### Type consistency

- `Busy() bool` / `LoggingOut() bool` — added to interface (B0.T1) + impl (B0.T2 only adds LoggingOut; Busy exists) + mock (B0.T3 Step 2) — consistent.
- `AddNpcAt(level, x, z, typeID, duration int) (ActiveNpc, error)` — interface (B3.T1), adapter (B3.T2), mock (B3.T5 Step 1) — consistent.
- `NpcLifecycleDespawn` — used in adapter (B3.T2) and test (B3.T3) — matches `modules/world/npc.go:15`.
- `worldVarsView` — adapter receiver matches existing methods in `modules/world/server_varp.go`.
- `setActiveNpcSlot(s, npc)` — called in B2 and B3 handlers; signature matches `handlers_npc.go:81`.

### Plan-author runtime pre-flight reminders (re-grep before dispatch)

Each task block flags re-grep premises explicitly. Most-load-bearing:
- B0.T3 Step 2: name of existing player mock + which test file holds `TestHandleBusy2`.
- B1.T1: existing `stubLineValidator` / `mockWorldVars` shape.
- B2.T1: existing `mockNpcLookup` / `NewHuntAllNpcIterator` lookup surface.
- B3.T2: `server_varp.go` import list (`"fmt"`, `script` package).
- B3.T3: `newTestServer` / `*Server` literal fields.
- B3.T5: `mockWorldVars` field-naming convention; `checkDuration` accepted range.

---

**Plan complete.** Pre-flight findings from before this plan was authored:

- (a) **B0:** `(*Player).Busy()` already implemented at `modules/world/player.go:651`; `loggingOut` is a private field at line 293. No public `LoggingOut()` accessor exists today — added in B0.T2. The `ActivePlayer` interface (active.go:41-761) does not currently expose either `Busy()` or `LoggingOut()` (only `HasInteraction`/`HasWaypoints` at lines 398, 403) — both added in B0.T1.
- (b) **B1 — R1 trips:** TS calls `rsmod.hasLineOfSight(level, sX, sZ, dX, dZ, 1, 1, 1, 1, 0)` (verified at `LostCityRS/Engine-TS/src/engine/GameMap.ts:429-431` and `2004scape/rsmod-pathfinder/src/index.ts:355-379`). Goscape's `isLineOfSight` wrapper passes `(srcSize=1, destWidth=0, destLength=0, extraFlag=0)`, which expands to TS-equivalent `(srcWidth=1, srcLength=1, destWidth=0, destHeight=0, extraFlag=0)` inside `RayCast` (`linevalidator.go:21`). Divergence at `destWidth=0 vs 1` and `destLength=0 vs 1`. Pre-existing bug; B1 T0 widens the wrapper as prerequisite + opens `NAI-163-D-LOS-ARG-SHAPE-FIX`.
- (c) **B2 — R4:** `NpcOps.ts:307` confirmed as `<=` verbatim (later iterator yield wins on tie).
- (d) **B3:** Npc struct field names confirmed (`level, startX, startZ, baseType, typeId, size, blockWalk, moveRestrict, typ, lifecycle, lifecycleTick`). `NpcLifecycleDespawn = 2` at `npc.go:15`. Default constructor `NewNpc` at `npc.go:159-228` sets `lifecycle: NpcLifecycleRespawn` (line 166) — adapter must override. `resetEntityForRespawn` (npc_registry.go:121) does NOT touch `lifecycle`. World interface is named `WorldVars`, not `World` (spec mislabeled). Return type is `script.ActiveNpc` (no `script.Npc` exists).
