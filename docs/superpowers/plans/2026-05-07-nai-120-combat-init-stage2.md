# NAI-120 Stage 2 — Combat-init missing-handler ports Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Port the 11 (D) opcode handlers enumerated by NAI-120 Bundle 1 audit so the combat-init chain dispatched from `[opnpc2,_]` / `[apnpc2,_]` runs end-to-end against Tutorial Island giant rats without "no handler" errors.

**Architecture:** Five sequential bundles (2A → 2E). Each bundle ports a small group of handlers that share new interface dependencies and lands in one commit. Per-bundle TDD shape: pre-flight grep → add interface methods + concrete impls + mock stubs → write failing tests → run RED → implement handler + dispatch → run GREEN → cross-package `go test ./...` → commit → Sonnet code-reviewer subagent. Bundles 2A–2D are required for smoke; Bundle 2E (`inv_dropitem_delayed`) is a stretch goal whose dependency `ObjDelayedQueue` is the largest new infrastructure item — defer to NAI-121 if smoke binds without it.

**Tech Stack:** Go 1.26+. Tests in `pkg/script/handlers_*_test.go`. Production in `pkg/script/handlers_*.go`. Concrete entity impls in `modules/world/`. `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache` prefix on every `go` invocation per global CLAUDE.md.

**Inputs:**
- Spec: `docs/superpowers/specs/2026-05-07-nai-120-combat-init-path-investigation-design.md` (commit `b020daa`).
- Bundle 0 findings: `docs/superpowers/investigations/2026-05-07-nai-120-bundle0-findings.md` (commit `47fa923`).
- Bundle 1 audit: `docs/superpowers/investigations/2026-05-07-nai-120-bundle1-audit.md` (commit `4e74be9`).
- TS source: `LostCityRS/Engine-TS/src/engine/script/handlers/{ServerOps,PlayerOps,NpcOps,InvOps}.ts`.

---

## File Structure

Files this plan creates or modifies:

| File | Action | Responsibility |
|---|---|---|
| `pkg/script/active.go` | Modify | Add 6 ActivePlayer methods (HasInteraction, HasWaypoints, SetInteractionScriptNpcT, SetInteractionScriptPlayer) + 3 ActiveNpc methods (SetNpcStat, PlaySpotAnim, AddHeroPoints) |
| `pkg/script/state.go` | Modify | Add IsMulti to WorldVars; add FindNpcByUID to NpcLookup; add AddObjDelayed (Bundle 2E only) |
| `pkg/script/handlers_player.go` | Modify | Add handleBusy2, handlePOpNpcT, handlePOpPlayer |
| `pkg/script/handlers_npc.go` | Modify | Add handleNpcFindUID, handleNpcRange, handleNpcStatAdd, handleNpcStatSub, handleSpotAnimNpc, handleNpcHeroPoints, checkNpcStatID helper |
| `pkg/script/handlers_map.go` | Modify | Add handleMapMultiway |
| `pkg/script/handlers_inv.go` | Modify (Bundle 2E only) | Add handleInvDropItemDelayed |
| `pkg/script/handlers.go` | Modify | Add 11 dispatch entries |
| `pkg/script/handlers_player_test.go` | Modify | Add tests for busy2, p_opnpct, p_opplayer |
| `pkg/script/handlers_npc_test.go` | Modify | Add tests for npc_finduid, npc_range, npc_statadd, npc_statsub, spotanim_npc, npc_heropoints; extend mockNpc with setNpcStatCalls + playSpotAnimCalls + addHeroPointsCalls fields |
| `pkg/script/handlers_map_test.go` | Modify | Add tests for map_multiway |
| `pkg/script/handlers_inv_test.go` | Modify (Bundle 2E only) | Add tests for inv_dropitem_delayed |
| `pkg/script/handlers_vars_test.go` | Modify | Add IsMulti stub to mockWorld; add AddObjDelayed (Bundle 2E only) |
| `pkg/script/runner_test.go` | Modify | Extend mockNpcLookup with FindNpcByUID + byUID field; extend mockPlayer with hasInteractionValue / hasWaypointsValue / lastSetInteractionScriptNpcT / lastSetInteractionScriptPlayer fields |
| `modules/world/player_script.go` | Modify | Add HasInteraction, HasWaypoints, SetInteractionScriptNpcT, SetInteractionScriptPlayer methods on `*Player` |
| `modules/world/npc_script.go` | Modify | Add SetNpcStat, PlaySpotAnim, AddHeroPoints methods on `*Npc` |
| `modules/world/npc.go` | Modify | Add `heroPoints HeroPoints` field on `Npc` struct |
| `modules/world/heropoints.go` | Create (Bundle 2D) | Minimal HeroPoints structure (capped at 16 entries — TS new HeroPoints(16)) |
| `modules/world/npc_script_lookup.go` | Modify | Add FindNpcByUID method on `serverNpcLookup` |
| `modules/world/world_zone.go` (or new file) | Modify (Bundle 2E only) | Add AddObjDelayed + ObjDelayedQueue infrastructure |

---

## Pre-flight reminders for the executing engineer

**Before EVERY handler dispatch:** Re-grep at HEAD per `controller_preflight`. Do not trust this plan's line numbers blindly — they are correct at HEAD `4e74be9` but may shift if intervening commits land. If a sibling pattern moved, follow the actual code at HEAD, not the plan's quoted line.

**On test fixtures:** Every `&ScriptState{}` in this plan that performs PushInt/PopInt MUST initialize stacks via `IntStack: make([]int, StackCapacity)` and `StringStack: make([]string, StackCapacity)`. The codebase uses `StackCapacity` const (1024). Per `scriptstate_test_fixture_idioms`.

**On Bundle pre-flight grep:** Each bundle starts with a "Pre-flight" task that re-greps the audit's claimed-absent symbols at HEAD. If any are present (added since `4e74be9`), merge with existing rather than duplicate.

**On commits:** Use `git commit --no-gpg-sign` per global CLAUDE.md.

---

## Bundle 2A — Pure-read server/NPC ops (REQUIRED)

**Handlers ported:** `map_multiway` (1014), `npc_finduid` (2521), `npc_range` (2531).

**New interface surface:**
- `WorldVars.IsMulti(level, x, z int) bool`
- `NpcLookup.FindNpcByUID(uid int) ActiveNpc`

**Goal:** Bundle 2A unblocks the proc body of `[proc,player_in_combat_check]` (pc=1 MAP_MULTIWAY surfaced as the original NAI-119 smoke residual) and unblocks `npc_range`/`npc_finduid` calls in `player_combat.rs2`.

### Task 2A.0: Pre-flight verification

**Files:** None — read-only grep.

- [ ] **Step 1: Verify the 3 (D) opcodes are still undispatched at HEAD**

Run:
```bash
rg -n "OpMapMultiway|OpNpcFindUID|OpNpcRange" pkg/script/handlers.go
```

Expected: No matches in dispatch tables. If matches found, the opcode has been wired in an intervening commit — merge with existing rather than duplicate.

- [ ] **Step 2: Verify sibling helpers exist**

Run:
```bash
rg -n "func handleMapBlocked|func checkCoord|func unpackCoord|func setActiveNpcSlot|func requireActiveNpc" pkg/script/handlers_map.go pkg/script/handlers_npc.go pkg/script/handlers_player.go
```

Expected: All five helpers present (handleMapBlocked at handlers_map.go:190, checkCoord at handlers_npc.go:13, unpackCoord at handlers_player.go:18, setActiveNpcSlot at handlers_npc.go:71, requireActiveNpc at handlers_npc.go:88).

- [ ] **Step 3: Verify `gamemap.GameMap.IsMulti` signature**

Run:
```bash
rg -n "func .*GameMap.*IsMulti" pkg/gamemap/multimap.go
```

Expected: `func (gm *GameMap) IsMulti(x, z, level int) bool` at `pkg/gamemap/multimap.go:17`. Note arg order: `(x, z, level)` — the WorldVars wrapper canonicalises to `(level, x, z)` matching `IsMapBlocked`.

### Task 2A.1: Add `IsMulti` to WorldVars

**Files:**
- Modify: `pkg/script/state.go` (WorldVars interface)
- Modify: `pkg/script/handlers_vars_test.go` (mockWorld stub)

- [ ] **Step 1: Add the method to the WorldVars interface**

Insert after the existing `IsFreeToPlay` declaration in `pkg/script/state.go` (currently around line 75):

```go
	// IsMulti reports whether the tile at (level, x, z) is in a multi-combat
	// zone. Mirrors TS World.gameMap.isMulti at Engine-TS/.../GameMap.ts.
	// Used by MAP_MULTIWAY (opcode 1014).
	IsMulti(level, x, z int) bool
```

- [ ] **Step 2: Add the mockWorld stub**

Insert after `IsFreeToPlay` stub at `pkg/script/handlers_vars_test.go:38`:

```go
// NAI-120 Bundle 2A: default no-op stub. Tests exercising MAP_MULTIWAY override
// via a recorder type that wraps mockWorld.
func (m *mockWorld) IsMulti(level, x, z int) bool { return false }
```

- [ ] **Step 3: Verify the package still compiles (interface assertions tighten)**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./pkg/script/...`

Expected: Clean build. If the production `*Server` impl in `modules/world/` doesn't satisfy the interface yet, the build will break — that's Task 2A.2.

### Task 2A.2: Add `IsMulti` impl on `*Server`

**Files:**
- Modify: `modules/world/server.go` or a sibling `_world.go` file (whichever currently hosts `IsMapBlocked`)

- [ ] **Step 1: Locate the existing IsMapBlocked impl on *Server**

Run:
```bash
rg -n "func \(s \*Server\) IsMapBlocked" modules/world/
```

Expected: One match. Add `IsMulti` immediately after.

- [ ] **Step 2: Add the impl**

Insert after `IsMapBlocked`:

```go
// IsMulti delegates to the world's GameMap.IsMulti, swapping arg order to
// match the WorldVars convention (level, x, z) — gamemap uses (x, z, level).
// Mirrors TS World.gameMap.isMulti(coord). NAI-120 Bundle 2A.
func (s *Server) IsMulti(level, x, z int) bool {
	if s.gameMap == nil {
		return false
	}
	return s.gameMap.IsMulti(x, z, level)
}
```

If the field name is not `s.gameMap`, locate the actual field via:
```bash
rg -n "GameMap\s*$|gameMap\s+\*" modules/world/server.go
```

- [ ] **Step 3: Verify cross-package build**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...`

Expected: Clean build.

### Task 2A.3: Write failing test for `map_multiway`

**Files:**
- Modify: `pkg/script/handlers_map_test.go` (add MAP_MULTIWAY test + a recorder mock)

- [ ] **Step 1: Add the test cases**

Append to `pkg/script/handlers_map_test.go` (or create the file if absent — sibling pattern at `handlers_npc_test.go`):

```go
// multiWorld extends mockWorld with a coord→bool map for IsMulti so MAP_MULTIWAY
// tests can pin per-tile multi-zone results. NAI-120 Bundle 2A.
type multiWorld struct {
	*mockWorld
	multiTiles map[[3]int]bool // key: [level, x, z]
}

func (m *multiWorld) IsMulti(level, x, z int) bool {
	return m.multiTiles[[3]int{level, x, z}]
}

func TestMapMultiway_MultiTile(t *testing.T) {
	w := &multiWorld{mockWorld: newMockWorld(), multiTiles: map[[3]int]bool{
		{0, 3222, 3218}: true,
	}}
	s := &ScriptState{
		World:       w,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	// Push packed coord (level<<28) | (x<<14) | z = 3222 << 14 | 3218.
	s.PushInt((0 << 28) | (3222 << 14) | 3218)
	if err := handleMapMultiway(s); err != nil {
		t.Fatalf("MAP_MULTIWAY multi tile: unexpected error %v", err)
	}
	if got := s.PopInt(); got != 1 {
		t.Errorf("MAP_MULTIWAY multi tile: got %d, want 1", got)
	}
}

func TestMapMultiway_NonMultiTile(t *testing.T) {
	w := &multiWorld{mockWorld: newMockWorld(), multiTiles: nil}
	s := &ScriptState{
		World:       w,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	s.PushInt((0 << 28) | (3000 << 14) | 3000)
	if err := handleMapMultiway(s); err != nil {
		t.Fatalf("MAP_MULTIWAY non-multi: unexpected error %v", err)
	}
	if got := s.PopInt(); got != 0 {
		t.Errorf("MAP_MULTIWAY non-multi: got %d, want 0", got)
	}
}

func TestMapMultiway_NoWorld(t *testing.T) {
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	s.PushInt(0)
	if err := handleMapMultiway(s); err == nil {
		t.Error("MAP_MULTIWAY with nil World: want error")
	}
}
```

- [ ] **Step 2: Run the test to verify RED**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestMapMultiway -v
```

Expected: FAIL — `undefined: handleMapMultiway`.

### Task 2A.4: Implement `handleMapMultiway` + dispatch

**Files:**
- Modify: `pkg/script/handlers_map.go` (handler body)
- Modify: `pkg/script/handlers.go` (dispatch entry)

- [ ] **Step 1: Add the handler**

Insert at the end of `pkg/script/handlers_map.go`:

```go
// handleMapMultiway (MAP_MULTIWAY, opcode 1014) reports whether the tile at
// the popped coord is in a multi-combat zone. Mirrors TS ServerOps.ts:376-380:
//
//	state.pushInt(World.gameMap.isMulti(coord) ? 1 : 0);
//
// TS does NOT call CoordValid on the coord (unlike MAP_BLOCKED). Goscape
// matches: pass the unpacked coord directly to WorldVars.IsMulti. NAI-120
// Bundle 2A.
func handleMapMultiway(s *ScriptState) error {
	coord := s.PopInt()
	if s.World == nil {
		return errors.New("MAP_MULTIWAY: no world surface")
	}
	level, x, z := unpackCoord(coord)
	if s.World.IsMulti(level, x, z) {
		s.PushInt(1)
	} else {
		s.PushInt(0)
	}
	return nil
}
```

If `errors` is not already imported in `handlers_map.go`, add it.

- [ ] **Step 2: Add the dispatch entry**

Locate the existing `OpMapBlocked: handleMapBlocked,` line in `pkg/script/handlers.go` (around line 100). Add immediately after:

```go
	OpMapMultiway: handleMapMultiway,
```

- [ ] **Step 3: Run the test to verify GREEN**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestMapMultiway -v
```

Expected: PASS for all three subtests.

### Task 2A.5: Add `FindNpcByUID` to `NpcLookup`

**Files:**
- Modify: `pkg/script/state.go` (NpcLookup interface)
- Modify: `pkg/script/runner_test.go` (extend mockNpcLookup)
- Modify: `modules/world/npc_script_lookup.go` (serverNpcLookup impl)

- [ ] **Step 1: Add the method to the NpcLookup interface**

Insert before the closing brace of the `NpcLookup` interface in `pkg/script/state.go` (after `ZoneNpcs`):

```go
	// FindNpcByUID resolves a packed NPC UID `(typeId<<16)|nid` to the
	// matching ActiveNpc. The lookup verifies BOTH the slot has a live
	// NPC AND the NPC's typeId equals the high-16-bit type. Returns nil
	// on miss. Mirrors TS NpcOps.ts:26-40 (NPC_FINDUID). NAI-120 Bundle 2A.
	FindNpcByUID(uid int) ActiveNpc
```

- [ ] **Step 2: Extend mockNpcLookup**

Locate the `mockNpcLookup` struct in `pkg/script/runner_test.go:689` and add a `byUID` field + counter. After the `byZone` map field:

```go
	// byUID returns the NPC keyed by uid. nil entry = miss. NAI-120 Bundle 2A.
	byUID map[int]ActiveNpc
```

After the `zoneNpcsCalls` counter:

```go
	byUIDCalls int
```

After the `ZoneNpcs` method, add:

```go
func (m *mockNpcLookup) FindNpcByUID(uid int) ActiveNpc {
	m.byUIDCalls++
	m.lastArgs = []int{uid}
	if m.byUID == nil {
		return nil
	}
	return m.byUID[uid]
}
```

- [ ] **Step 3: Add the production impl on serverNpcLookup**

Append to `modules/world/npc_script_lookup.go` (after `ZoneNpcs`):

```go
// FindNpcByUID resolves a packed NPC UID to the live NPC at that slot
// only when the NPC's typeId matches the high-16-bit `expectedType`
// embedded in the UID. Mirrors TS NpcOps.ts:26-40:
//
//	const slot = npcUid & 0xffff;
//	const expectedType = (npcUid >> 16) & 0xffff;
//	const npc = World.getNpc(slot);
//	if (!npc || npc.type !== expectedType) { ... return null }
//
// NAI-120 Bundle 2A.
func (l serverNpcLookup) FindNpcByUID(uid int) script.ActiveNpc {
	slot := uid & 0xffff
	expectedType := (uid >> 16) & 0xffff
	if slot < 0 || slot >= len(l.s.npcs) {
		return nil
	}
	n := l.s.npcs[slot]
	if n == nil || n.typeId != expectedType {
		return nil
	}
	return n
}
```

- [ ] **Step 4: Verify cross-package build**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...`

Expected: Clean build.

### Task 2A.6: Write failing test for `npc_finduid`

**Files:**
- Modify: `pkg/script/handlers_npc_test.go` (append tests)

- [ ] **Step 1: Add the tests**

Append to `pkg/script/handlers_npc_test.go`:

```go
func TestNpcFindUID_Hit_PrimarySlot(t *testing.T) {
	npc := &mockNpc{typeID: 42, nid: 7, uid: (42 << 16) | 7}
	lookup := &mockNpcLookup{byUID: map[int]ActiveNpc{npc.uid: npc}}
	s := &ScriptState{
		Npcs:        lookup,
		Script:      &ScriptFile{IntOperands: []int32{0}},
		PC:          0,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	s.PushInt(npc.uid)
	if err := handleNpcFindUID(s); err != nil {
		t.Fatalf("NPC_FINDUID hit: unexpected error %v", err)
	}
	if got := s.PopInt(); got != 1 {
		t.Errorf("NPC_FINDUID hit: pushed %d, want 1", got)
	}
	if s.ActiveNpc != npc {
		t.Errorf("NPC_FINDUID hit: ActiveNpc not bound (got %v, want %v)", s.ActiveNpc, npc)
	}
	if s.Pointers&PtrActiveNpc == 0 {
		t.Error("NPC_FINDUID hit: PtrActiveNpc not set")
	}
}

func TestNpcFindUID_Hit_SecondarySlot(t *testing.T) {
	npc := &mockNpc{typeID: 42, nid: 7, uid: (42 << 16) | 7}
	lookup := &mockNpcLookup{byUID: map[int]ActiveNpc{npc.uid: npc}}
	s := &ScriptState{
		Npcs:        lookup,
		Script:      &ScriptFile{IntOperands: []int32{1}},
		PC:          0,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	s.PushInt(npc.uid)
	if err := handleNpcFindUID(s); err != nil {
		t.Fatalf("NPC_FINDUID secondary hit: unexpected error %v", err)
	}
	if got := s.PopInt(); got != 1 {
		t.Errorf("NPC_FINDUID secondary hit: pushed %d, want 1", got)
	}
	if s.OtherActiveNpc != npc {
		t.Errorf("NPC_FINDUID secondary hit: OtherActiveNpc not bound (got %v, want %v)", s.OtherActiveNpc, npc)
	}
	if s.Pointers&PtrActiveNpc2 == 0 {
		t.Error("NPC_FINDUID secondary hit: PtrActiveNpc2 not set")
	}
}

func TestNpcFindUID_Miss(t *testing.T) {
	lookup := &mockNpcLookup{byUID: nil}
	s := &ScriptState{
		Npcs:        lookup,
		Script:      &ScriptFile{IntOperands: []int32{0}},
		PC:          0,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	s.PushInt((42 << 16) | 999)
	if err := handleNpcFindUID(s); err != nil {
		t.Fatalf("NPC_FINDUID miss: unexpected error %v", err)
	}
	if got := s.PopInt(); got != 0 {
		t.Errorf("NPC_FINDUID miss: pushed %d, want 0", got)
	}
	if s.ActiveNpc != nil {
		t.Errorf("NPC_FINDUID miss: ActiveNpc should remain nil (got %v)", s.ActiveNpc)
	}
	if s.Pointers&PtrActiveNpc != 0 {
		t.Error("NPC_FINDUID miss: PtrActiveNpc should NOT be set")
	}
}

func TestNpcFindUID_NilNpcs(t *testing.T) {
	s := &ScriptState{
		Npcs:        nil,
		Script:      &ScriptFile{IntOperands: []int32{0}},
		PC:          0,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	s.PushInt((42 << 16) | 7)
	if err := handleNpcFindUID(s); err != nil {
		t.Fatalf("NPC_FINDUID nil Npcs: unexpected error %v", err)
	}
	if got := s.PopInt(); got != 0 {
		t.Errorf("NPC_FINDUID nil Npcs: pushed %d, want 0", got)
	}
}
```

- [ ] **Step 2: Run the test to verify RED**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestNpcFindUID -v
```

Expected: FAIL — `undefined: handleNpcFindUID`.

### Task 2A.7: Implement `handleNpcFindUID` + dispatch

**Files:**
- Modify: `pkg/script/handlers_npc.go`
- Modify: `pkg/script/handlers.go`

- [ ] **Step 1: Add the handler**

Append to `pkg/script/handlers_npc.go` (e.g., after the existing find-family handlers; if unsure, place after `handleNpcFind` or before `handleNpcStat`):

```go
// handleNpcFindUID (NPC_FINDUID, opcode 2521) pops a packed NPC UID and
// binds the matching live NPC to the active slot dictated by the bytecode
// IntOperand (.npc → primary, .npc2 → secondary). Pushes 1 on hit, 0 on
// miss. Does NOT set the Protect bit. Mirrors TS NpcOps.ts:26-40:
//
//	const slot = npcUid & 0xffff;
//	const expectedType = (npcUid >> 16) & 0xffff;
//	const npc = World.getNpc(slot);
//	if (!npc || npc.type !== expectedType) {
//	    state.pushInt(0);
//	    return;
//	}
//	state.activeNpc = npc;
//	state.pointerAdd(ActiveNpc[state.intOperand]);
//	state.pushInt(1);
//
// goscape's NpcLookup.FindNpcByUID encapsulates the slot-lookup +
// type-match check, returning nil on miss. NAI-120 Bundle 2A.
func handleNpcFindUID(s *ScriptState) error {
	uid := s.PopInt()
	if s.Npcs == nil {
		s.PushInt(0)
		return nil
	}
	npc := s.Npcs.FindNpcByUID(uid)
	if npc == nil {
		s.PushInt(0)
		return nil
	}
	setActiveNpcSlot(s, npc)
	s.PushInt(1)
	return nil
}
```

- [ ] **Step 2: Add the dispatch entry**

In `pkg/script/handlers.go`, locate the NPC dispatch block (around line 385–435 — `OpNpcCoord` etc.). Add:

```go
	OpNpcFindUID: handleNpcFindUID,
```

- [ ] **Step 3: Run the test to verify GREEN**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestNpcFindUID -v
```

Expected: PASS for all four subtests.

### Task 2A.8: Write failing test for `npc_range`

**Files:**
- Modify: `pkg/script/handlers_npc_test.go`

- [ ] **Step 1: Add the tests**

Append to `pkg/script/handlers_npc_test.go`:

```go
func TestNpcRange_SameLevel_Adjacent(t *testing.T) {
	npc := &mockNpc{x: 3222, z: 3218, level: 0}
	s := &ScriptState{
		ActiveNpc:   npc,
		Pointers:    PtrActiveNpc,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	// Pop a packed coord at (3223, 3218, level 0): chebyshev distance 1.
	s.PushInt((0 << 28) | (3223 << 14) | 3218)
	if err := handleNpcRange(s); err != nil {
		t.Fatalf("NPC_RANGE same-level adjacent: unexpected error %v", err)
	}
	if got := s.PopInt(); got != 1 {
		t.Errorf("NPC_RANGE same-level adjacent: got %d, want 1", got)
	}
}

func TestNpcRange_SameLevel_Diagonal(t *testing.T) {
	npc := &mockNpc{x: 3222, z: 3218, level: 0}
	s := &ScriptState{
		ActiveNpc:   npc,
		Pointers:    PtrActiveNpc,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	// Pop a packed coord at (3223, 3219, 0): chebyshev distance 1.
	s.PushInt((0 << 28) | (3223 << 14) | 3219)
	if err := handleNpcRange(s); err != nil {
		t.Fatalf("NPC_RANGE same-level diagonal: unexpected error %v", err)
	}
	if got := s.PopInt(); got != 1 {
		t.Errorf("NPC_RANGE same-level diagonal: got %d, want 1", got)
	}
}

func TestNpcRange_DifferentLevel_Sentinel(t *testing.T) {
	npc := &mockNpc{x: 3222, z: 3218, level: 0}
	s := &ScriptState{
		ActiveNpc:   npc,
		Pointers:    PtrActiveNpc,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	// Pop a packed coord at (3222, 3218, level 1): different level → -1.
	s.PushInt((1 << 28) | (3222 << 14) | 3218)
	if err := handleNpcRange(s); err != nil {
		t.Fatalf("NPC_RANGE diff-level: unexpected error %v", err)
	}
	if got := s.PopInt(); got != -1 {
		t.Errorf("NPC_RANGE diff-level: got %d, want -1", got)
	}
}

func TestNpcRange_NoActiveNpc(t *testing.T) {
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	s.PushInt(0)
	if err := handleNpcRange(s); err == nil {
		t.Error("NPC_RANGE with no ActiveNpc: want error")
	}
}

func TestNpcRange_InvalidCoord(t *testing.T) {
	npc := &mockNpc{x: 3222, z: 3218, level: 0}
	s := &ScriptState{
		ActiveNpc:   npc,
		Pointers:    PtrActiveNpc,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	// Negative coord fails CoordValid.
	s.PushInt(-1)
	if err := handleNpcRange(s); err == nil {
		t.Error("NPC_RANGE invalid coord: want error")
	}
}
```

- [ ] **Step 2: Run to verify RED**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestNpcRange -v`

Expected: FAIL — `undefined: handleNpcRange`.

### Task 2A.9: Implement `handleNpcRange` + dispatch

**Files:**
- Modify: `pkg/script/handlers_npc.go`
- Modify: `pkg/script/handlers.go`

- [ ] **Step 1: Add the handler**

Append to `pkg/script/handlers_npc.go`:

```go
// handleNpcRange (NPC_RANGE, opcode 2531) pops a packed coord and pushes the
// Chebyshev distance from the active NPC to that 1×1 tile. Returns -1 when
// the coord's level differs from the NPC's level (TS sentinel). Mirrors TS
// NpcOps.ts:152-168:
//
//	const coord: CoordGrid = check(state.popInt(), CoordValid);
//	const npc = state.activeNpc;
//	if (coord.level !== npc.level) {
//	    state.pushInt(-1);
//	} else {
//	    state.pushInt(CoordGrid.distanceTo(npc, {x, z, width:1, length:1}));
//	}
//
// `CoordGrid.distanceTo` for a 1×1 target reduces to Chebyshev:
// max(|npcX - x|, |npcZ - z|) — width=1/length=1 contributes 0 to the
// per-axis subtractions in the TS formula. NAI-120 Bundle 2A.
//
// Multi-tile NPCs (size > 1): the inner-ring call sites in
// player_combat.rs2 do not require size-aware distance — sites pass
// `coord` (the player's own coord) and the active NPC is the combat
// target. This handler treats the NPC as a 1×1 source (matches TS
// behaviour for size=1 NPCs; size>1 audit deferred to a future sub-spec
// per NAI-120 Bundle 1 audit §6 dependency note).
func handleNpcRange(s *ScriptState) error {
	if err := requireActiveNpc(s, "NPC_RANGE"); err != nil {
		return err
	}
	coord := s.PopInt()
	level, x, z, err := checkCoord(coord, "NPC_RANGE")
	if err != nil {
		return err
	}
	n := s.ActiveNpc
	if level != n.NpcLevel() {
		s.PushInt(-1)
		return nil
	}
	dx := n.NpcX() - x
	if dx < 0 {
		dx = -dx
	}
	dz := n.NpcZ() - z
	if dz < 0 {
		dz = -dz
	}
	if dx > dz {
		s.PushInt(dx)
	} else {
		s.PushInt(dz)
	}
	return nil
}
```

- [ ] **Step 2: Add dispatch**

In `pkg/script/handlers.go`, NPC dispatch block:

```go
	OpNpcRange: handleNpcRange,
```

- [ ] **Step 3: Run to verify GREEN**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestNpcRange -v`

Expected: PASS for all five subtests.

### Task 2A.10: Cross-package green + commit

**Files:** None (verification + commit).

- [ ] **Step 1: Run the full test suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`

Expected: PASS across all packages. If a package fails, do not "fix forward" by editing tests — diagnose. Most likely cause is a missing interface stub (e.g., a sibling mock in a different test file lacking IsMulti). Add the stub.

- [ ] **Step 2: Stage and commit**

Run:
```bash
git add pkg/script/state.go pkg/script/active.go pkg/script/handlers.go pkg/script/handlers_map.go pkg/script/handlers_npc.go pkg/script/handlers_map_test.go pkg/script/handlers_npc_test.go pkg/script/handlers_vars_test.go pkg/script/runner_test.go modules/world/server.go modules/world/npc_script_lookup.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): NAI-120 Bundle 2A — port map_multiway, npc_finduid, npc_range

Adds handler + dispatch for the three pure-read server/NPC ops surfaced by
NAI-120 Bundle 1 audit. New interface surface: WorldVars.IsMulti and
NpcLookup.FindNpcByUID. Tests pin push/pop signature, level sentinel
behaviour for NPC_RANGE, primary+secondary slot binding for NPC_FINDUID,
and nil-degradation paths.

EOF
)"
```

Adjust the `git add` list to actual file paths edited.

- [ ] **Step 3: Verify the commit**

Run: `git show HEAD --stat`

Expected: Commit appears with the file list above. No surprises.

### Task 2A.11: Code-reviewer dispatch (Sonnet)

**Files:** None (subagent dispatch).

- [ ] **Step 1: Dispatch the reviewer**

Use the `feature-dev:code-reviewer` agent (or `superpowers:code-reviewer` if available — model: Sonnet, NOT Opus, per `superpowers_code_reviewer_model`). Prompt template:

> Review commit `<SHA>` for NAI-120 Bundle 2A. Three handler ports:
> - `MAP_MULTIWAY` (handlers_map.go, audit §3, TS ServerOps.ts:376-380)
> - `NPC_FINDUID` (handlers_npc.go, audit §4, TS NpcOps.ts:26-40)
> - `NPC_RANGE` (handlers_npc.go, audit §6, TS NpcOps.ts:152-168)
>
> Verify each handler's pop/push signature, validation order, and edge cases match the TS reference. Cite TS file:line for each verdict. Cross-check that new interface methods (`WorldVars.IsMulti`, `NpcLookup.FindNpcByUID`) carry doc comments matching the wider codebase convention. Flag any test that asserts a wrong-reason pass (per `test_passes_for_wrong_reason`).
>
> Report under 400 words; surface only HIGH-CONFIDENCE issues.

- [ ] **Step 2: Address feedback if any**

If the reviewer surfaces a real issue (not stylistic preference), make the fix in a follow-up commit on the same branch. If the issue is stylistic / preference-only, note it and move on.

---

## Bundle 2B — Player-interaction ops (REQUIRED)

**Handlers ported:** `busy2` (2006), `p_opnpct` (2079), `p_opplayer` (2081).

**New interface surface:**
- `ActivePlayer.HasInteraction() bool`
- `ActivePlayer.HasWaypoints() bool`
- `ActivePlayer.SetInteractionScriptNpcT(npc ActiveNpc, spellCom int)`
- `ActivePlayer.SetInteractionScriptPlayer(player2 ActivePlayer, op int)` (op is 1-based, 1..5)

**Goal:** Bundle 2B unblocks `auto_retaliate.rs2` (`busy2` and `p_opplayer`) and `player_magic.rs2` (`p_opnpct`).

### Task 2B.0: Pre-flight

- [ ] **Step 1: Verify the 3 (D) opcodes still undispatched**

Run: `rg -n "OpBusy2|OpPOpNpcT|OpPOpPlayer" pkg/script/handlers.go`

Expected: No matches.

- [ ] **Step 2: Verify the 4 new ActivePlayer methods are still absent**

Run:
```bash
rg -n "HasInteraction|HasWaypoints|SetInteractionScriptNpcT|SetInteractionScriptPlayer" pkg/script/active.go modules/world/player_script.go
```

Expected: No matches in either file.

- [ ] **Step 3: Verify `(*Player).hasWaypoints` (lowercase) exists at `modules/world/interaction.go:297`**

Run: `rg -n "func \(p \*Player\) hasWaypoints" modules/world/interaction.go`

Expected: One match at line 297 returning `p.waypointIndex >= 0`.

- [ ] **Step 4: Verify `targetOpNpcT = 8` sentinel**

Run: `rg -n "targetOpNpcT\s*=\s*8" modules/world/interaction.go`

Expected: One match at `interaction.go:35`.

### Task 2B.1: Add `HasInteraction` + `HasWaypoints` to ActivePlayer

**Files:**
- Modify: `pkg/script/active.go`
- Modify: `modules/world/player_script.go`
- Modify: `pkg/script/runner_test.go` (extend mockPlayer)

- [ ] **Step 1: Add the interface methods**

Insert into the `ActivePlayer` interface in `pkg/script/active.go`, near the action-clear group (after the existing `StopAction`):

```go
	// HasInteraction reports whether the player has a current interaction
	// target (i.e., `target != nil`). Used by BUSY2 (opcode 2006). Mirrors
	// TS Player.hasInteraction at Engine-TS/.../PathingEntity.ts. NAI-120
	// Bundle 2B.
	HasInteraction() bool

	// HasWaypoints reports whether the player has waypoints queued
	// (waypointIndex >= 0). Used by BUSY2 (opcode 2006). Mirrors TS
	// Player.hasWaypoints. NAI-120 Bundle 2B.
	HasWaypoints() bool
```

- [ ] **Step 2: Add concrete impls on `*Player`**

In `modules/world/player_script.go`, append (or place near other simple getters like `RunEnergy`):

```go
// HasInteraction reports whether the player has an interaction target.
// NAI-120 Bundle 2B. Implements script.ActivePlayer.
func (p *Player) HasInteraction() bool { return p.target != nil }

// HasWaypoints reports whether the player has a waypoint queue active.
// Wraps the package-private hasWaypoints helper at interaction.go:297.
// NAI-120 Bundle 2B. Implements script.ActivePlayer.
func (p *Player) HasWaypoints() bool { return p.hasWaypoints() }
```

- [ ] **Step 3: Add mockPlayer fields + methods**

In `pkg/script/runner_test.go`, locate the `mockPlayer` struct (around line 99). After the `slot` field (around line 243), add:

```go
	// NAI-120 Bundle 2B: BUSY2 read-side seeds.
	hasInteractionValue bool
	hasWaypointsValue   bool
```

After the existing `StopAction` mock-method (or near other getters), add:

```go
func (m *mockPlayer) HasInteraction() bool { return m.hasInteractionValue }
func (m *mockPlayer) HasWaypoints() bool   { return m.hasWaypointsValue }
```

- [ ] **Step 4: Cross-package build**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...`

Expected: Clean build.

### Task 2B.2: Write failing test for `busy2`

**Files:**
- Modify: `pkg/script/handlers_player_test.go`

- [ ] **Step 1: Add tests**

Append to `pkg/script/handlers_player_test.go`:

```go
func TestBusy2_HasInteraction(t *testing.T) {
	mp := &mockPlayer{hasInteractionValue: true}
	s := &ScriptState{
		Self:        mp,
		Pointers:    PtrActivePlayer,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	if err := handleBusy2(s); err != nil {
		t.Fatalf("BUSY2 hasInteraction: unexpected error %v", err)
	}
	if got := s.PopInt(); got != 1 {
		t.Errorf("BUSY2 hasInteraction: got %d, want 1", got)
	}
}

func TestBusy2_HasWaypoints(t *testing.T) {
	mp := &mockPlayer{hasWaypointsValue: true}
	s := &ScriptState{
		Self:        mp,
		Pointers:    PtrActivePlayer,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	if err := handleBusy2(s); err != nil {
		t.Fatalf("BUSY2 hasWaypoints: unexpected error %v", err)
	}
	if got := s.PopInt(); got != 1 {
		t.Errorf("BUSY2 hasWaypoints: got %d, want 1", got)
	}
}

func TestBusy2_NeitherSet(t *testing.T) {
	mp := &mockPlayer{}
	s := &ScriptState{
		Self:        mp,
		Pointers:    PtrActivePlayer,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	if err := handleBusy2(s); err != nil {
		t.Fatalf("BUSY2 neither: unexpected error %v", err)
	}
	if got := s.PopInt(); got != 0 {
		t.Errorf("BUSY2 neither: got %d, want 0", got)
	}
}

func TestBusy2_NoActivePlayer(t *testing.T) {
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	if err := handleBusy2(s); err == nil {
		t.Error("BUSY2 with no active player: want error")
	}
}
```

- [ ] **Step 2: Run to verify RED**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestBusy2 -v`

Expected: FAIL — `undefined: handleBusy2`.

### Task 2B.3: Implement `handleBusy2` + dispatch

**Files:**
- Modify: `pkg/script/handlers_player.go`
- Modify: `pkg/script/handlers.go`

- [ ] **Step 1: Add handler**

Append to `pkg/script/handlers_player.go`:

```go
// handleBusy2 (BUSY2, opcode 2006) pushes 1 if the active player has either
// an interaction target OR queued waypoints, else 0. Mirrors TS
// PlayerOps.ts:898-900 (https://x.com/JagexAsh/status/1791053667228856563):
//
//	state.pushInt(state.activePlayer.hasInteraction() ||
//	              state.activePlayer.hasWaypoints() ? 1 : 0);
//
// Gate: ActivePlayer (no Protected requirement). NAI-120 Bundle 2B.
func handleBusy2(s *ScriptState) error {
	if err := requireActivePlayer(s, "BUSY2"); err != nil {
		return err
	}
	if s.Self.HasInteraction() || s.Self.HasWaypoints() {
		s.PushInt(1)
	} else {
		s.PushInt(0)
	}
	return nil
}
```

- [ ] **Step 2: Add dispatch entry**

In `pkg/script/handlers.go` (near other player-op dispatches):

```go
	OpBusy2: handleBusy2,
```

- [ ] **Step 3: Run to verify GREEN**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestBusy2 -v`

Expected: PASS for all four subtests.

### Task 2B.4: Add `SetInteractionScriptNpcT` to ActivePlayer

**Files:**
- Modify: `pkg/script/active.go`
- Modify: `modules/world/player_script.go`
- Modify: `pkg/script/runner_test.go` (extend mockPlayer)

- [ ] **Step 1: Add interface method**

Insert into the `ActivePlayer` interface, near the existing `SetInteractionScriptNpc` (around line 451 of `pkg/script/active.go`):

```go
	// SetInteractionScriptNpcT anchors the player on `npc` with trigger
	// ApNpcT as a script-queued interaction (TS Interaction.SCRIPT) and
	// stores `spellCom` as the targetSubject.com (the UI component id of
	// the spell being cast). Matches TS PlayerOps.ts:417-421 (P_OPNPCT)
	// terminal setInteraction call. NAI-120 Bundle 2B.
	SetInteractionScriptNpcT(npc ActiveNpc, spellCom int)
```

- [ ] **Step 2: Add concrete impl on `*Player`**

In `modules/world/player_script.go`, near `SetInteractionScriptNpc` (around line 955):

```go
// SetInteractionScriptNpcT implements script.ActivePlayer.
// Routes via SetInteraction(InteractionScript, npc, targetOpNpcT, spellCom)
// — the targetOpNpcT sentinel (=8 at modules/world/interaction.go:35) selects
// the APNPCT/OPNPCT trigger family in resolveTriggerTypeId. NAI-120 Bundle 2B.
func (p *Player) SetInteractionScriptNpcT(npc script.ActiveNpc, spellCom int) {
	realNpc, ok := npc.(*Npc)
	if !ok {
		return
	}
	p.SetInteraction(InteractionScript, realNpc, targetOpNpcT, spellCom)
}
```

- [ ] **Step 3: Add mockPlayer field + method**

In `pkg/script/runner_test.go`, locate the existing `lastSetInteractionScriptNpc` field (search for it). Add nearby:

```go
	// NAI-120 Bundle 2B: P_OPNPCT capture.
	lastSetInteractionScriptNpcT []struct {
		npc      ActiveNpc
		spellCom int
	}
```

Add the method (near other interaction-script methods on mockPlayer):

```go
func (m *mockPlayer) SetInteractionScriptNpcT(npc ActiveNpc, spellCom int) {
	m.lastSetInteractionScriptNpcT = append(m.lastSetInteractionScriptNpcT, struct {
		npc      ActiveNpc
		spellCom int
	}{npc, spellCom})
}
```

- [ ] **Step 4: Cross-package build**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...`

Expected: Clean.

### Task 2B.5: Write failing test for `p_opnpct`

**Files:**
- Modify: `pkg/script/handlers_player_test.go`

- [ ] **Step 1: Add tests**

```go
func TestPOpNpcT_HappyPath(t *testing.T) {
	mp := &mockPlayer{}
	npc := &mockNpc{typeID: 42, nid: 7}
	s := &ScriptState{
		Self:        mp,
		ActiveNpc:   npc,
		Pointers:    PtrActivePlayer | PtrActiveNpc,
		Protect:     true,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	s.PushInt(1234) // spellCom
	if err := handlePOpNpcT(s); err != nil {
		t.Fatalf("P_OPNPCT happy: unexpected error %v", err)
	}
	if mp.stopActionCalls != 1 {
		t.Errorf("P_OPNPCT happy: stopActionCalls = %d, want 1", mp.stopActionCalls)
	}
	if got := len(mp.lastSetInteractionScriptNpcT); got != 1 {
		t.Fatalf("P_OPNPCT happy: SetInteractionScriptNpcT calls = %d, want 1", got)
	}
	call := mp.lastSetInteractionScriptNpcT[0]
	if call.npc != npc {
		t.Errorf("P_OPNPCT happy: npc = %v, want %v", call.npc, npc)
	}
	if call.spellCom != 1234 {
		t.Errorf("P_OPNPCT happy: spellCom = %d, want 1234", call.spellCom)
	}
}

func TestPOpNpcT_NotProtected(t *testing.T) {
	mp := &mockPlayer{}
	npc := &mockNpc{typeID: 42, nid: 7}
	s := &ScriptState{
		Self:        mp,
		ActiveNpc:   npc,
		Pointers:    PtrActivePlayer | PtrActiveNpc,
		Protect:     false, // not protected
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	s.PushInt(1234)
	if err := handlePOpNpcT(s); err == nil {
		t.Error("P_OPNPCT not-protected: want error")
	}
}

func TestPOpNpcT_NoActiveNpc(t *testing.T) {
	mp := &mockPlayer{}
	s := &ScriptState{
		Self:        mp,
		Pointers:    PtrActivePlayer,
		Protect:     true,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	s.PushInt(1234)
	if err := handlePOpNpcT(s); err == nil {
		t.Error("P_OPNPCT no active npc: want error")
	}
}

func TestPOpNpcT_NullSpellCom(t *testing.T) {
	mp := &mockPlayer{}
	npc := &mockNpc{typeID: 42, nid: 7}
	s := &ScriptState{
		Self:        mp,
		ActiveNpc:   npc,
		Pointers:    PtrActivePlayer | PtrActiveNpc,
		Protect:     true,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	s.PushInt(-1) // null sentinel
	if err := handlePOpNpcT(s); err == nil {
		t.Error("P_OPNPCT spellCom=-1: want NumberNotNull error")
	}
}
```

- [ ] **Step 2: Run to verify RED**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestPOpNpcT -v`

Expected: FAIL — `undefined: handlePOpNpcT`.

### Task 2B.6: Implement `handlePOpNpcT` + dispatch

**Files:**
- Modify: `pkg/script/handlers_player.go`
- Modify: `pkg/script/handlers.go`

- [ ] **Step 1: Add handler**

Append to `pkg/script/handlers_player.go`:

```go
// handlePOpNpcT (P_OPNPCT, opcode 2079) anchors the active player on the
// active NPC with the APNPCT/OPNPCT trigger family and stores spellCom as
// the targetSubject.com. Mirrors TS PlayerOps.ts:417-421
// (https://x.com/JagexAsh/status/1791472651623370843):
//
//	const spellId: number = check(state.popInt(), NumberNotNull);
//	state.activePlayer.stopAction();
//	state.activePlayer.setInteraction(Interaction.SCRIPT, state.activeNpc,
//	    ServerTriggerType.APNPCT, spellId);
//
// Gate: ProtectedActivePlayer + ActiveNpc. NAI-120 Bundle 2B.
func handlePOpNpcT(s *ScriptState) error {
	if err := requireProtectedActivePlayer(s, "P_OPNPCT"); err != nil {
		return err
	}
	if s.ActiveNpc == nil {
		return errors.New("P_OPNPCT: no active npc")
	}
	spellCom := s.PopInt()
	if err := checkNotNull(spellCom, "P_OPNPCT"); err != nil {
		return err
	}
	s.Self.StopAction()
	s.Self.SetInteractionScriptNpcT(s.ActiveNpc, spellCom)
	return nil
}
```

- [ ] **Step 2: Add dispatch**

```go
	OpPOpNpcT: handlePOpNpcT,
```

- [ ] **Step 3: Run to verify GREEN**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestPOpNpcT -v`

Expected: PASS for all four subtests.

### Task 2B.7: Add `SetInteractionScriptPlayer` to ActivePlayer

**Files:**
- Modify: `pkg/script/active.go`
- Modify: `modules/world/player_script.go`
- Modify: `pkg/script/runner_test.go`

- [ ] **Step 1: Add interface method**

In `pkg/script/active.go`, near the other `SetInteractionScript*` methods:

```go
	// SetInteractionScriptPlayer anchors the player on `player2` (a secondary
	// active player) with trigger ApPlayer<op> as a script-queued interaction
	// (TS Interaction.SCRIPT). op is 1-indexed (1..5; engine fire-path
	// supports 1..4 — see modules/world/player_interaction_trigger.go's
	// apPlayerTriggerForOp). Matches TS PlayerOps.ts:1009-1020 (P_OPPLAYER)
	// terminal setInteraction. NAI-120 Bundle 2B.
	SetInteractionScriptPlayer(player2 ActivePlayer, op int)
```

- [ ] **Step 2: Add concrete impl**

In `modules/world/player_script.go`:

```go
// SetInteractionScriptPlayer implements script.ActivePlayer. Routes via
// SetInteraction(InteractionScript, realPlayer2, op, -1). The com=-1 means
// no spellCom association — APPLAYER<N> reads no targetSubject.com. NAI-120
// Bundle 2B.
func (p *Player) SetInteractionScriptPlayer(player2 script.ActivePlayer, op int) {
	realPlayer2, ok := player2.(*Player)
	if !ok {
		return
	}
	p.SetInteraction(InteractionScript, realPlayer2, op, -1)
}
```

- [ ] **Step 3: Add mockPlayer field + method**

In `pkg/script/runner_test.go`:

```go
	// NAI-120 Bundle 2B: P_OPPLAYER capture.
	lastSetInteractionScriptPlayer []struct {
		player2 ActivePlayer
		op      int
	}
```

```go
func (m *mockPlayer) SetInteractionScriptPlayer(player2 ActivePlayer, op int) {
	m.lastSetInteractionScriptPlayer = append(m.lastSetInteractionScriptPlayer, struct {
		player2 ActivePlayer
		op      int
	}{player2, op})
}
```

- [ ] **Step 4: Cross-package build**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...`

Expected: Clean.

### Task 2B.8: Write failing test for `p_opplayer`

**Files:**
- Modify: `pkg/script/handlers_player_test.go`

- [ ] **Step 1: Add tests**

```go
func TestPOpPlayer_HappyPath(t *testing.T) {
	mp := &mockPlayer{}
	mp2 := &mockPlayer{}
	s := &ScriptState{
		Self:        mp,
		Self2:       mp2,
		Pointers:    PtrActivePlayer | PtrActivePlayer2,
		Protect:     true,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	s.PushInt(2) // op
	if err := handlePOpPlayer(s); err != nil {
		t.Fatalf("P_OPPLAYER happy: unexpected error %v", err)
	}
	if mp.stopActionCalls != 1 {
		t.Errorf("P_OPPLAYER happy: stopActionCalls = %d, want 1", mp.stopActionCalls)
	}
	if got := len(mp.lastSetInteractionScriptPlayer); got != 1 {
		t.Fatalf("P_OPPLAYER happy: SetInteractionScriptPlayer calls = %d, want 1", got)
	}
	call := mp.lastSetInteractionScriptPlayer[0]
	if call.player2 != mp2 {
		t.Errorf("P_OPPLAYER happy: player2 = %v, want %v", call.player2, mp2)
	}
	if call.op != 2 {
		t.Errorf("P_OPPLAYER happy: op = %d, want 2", call.op)
	}
}

func TestPOpPlayer_NoSelf2_SilentReturn(t *testing.T) {
	mp := &mockPlayer{}
	s := &ScriptState{
		Self:        mp,
		Self2:       nil,
		Pointers:    PtrActivePlayer,
		Protect:     true,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	s.PushInt(1)
	if err := handlePOpPlayer(s); err != nil {
		t.Fatalf("P_OPPLAYER no Self2: want silent return, got error %v", err)
	}
	if mp.stopActionCalls != 0 {
		t.Errorf("P_OPPLAYER no Self2: stopActionCalls = %d, want 0 (no-op)", mp.stopActionCalls)
	}
	if got := len(mp.lastSetInteractionScriptPlayer); got != 0 {
		t.Errorf("P_OPPLAYER no Self2: should not call SetInteractionScriptPlayer, got %d calls", got)
	}
}

func TestPOpPlayer_NotProtected(t *testing.T) {
	mp := &mockPlayer{}
	mp2 := &mockPlayer{}
	s := &ScriptState{
		Self:        mp,
		Self2:       mp2,
		Pointers:    PtrActivePlayer | PtrActivePlayer2,
		Protect:     false,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	s.PushInt(1)
	if err := handlePOpPlayer(s); err == nil {
		t.Error("P_OPPLAYER not-protected: want error")
	}
}

func TestPOpPlayer_OpZero(t *testing.T) {
	mp := &mockPlayer{}
	mp2 := &mockPlayer{}
	s := &ScriptState{
		Self:        mp,
		Self2:       mp2,
		Pointers:    PtrActivePlayer | PtrActivePlayer2,
		Protect:     true,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	s.PushInt(0) // 0 is NOT NumberNotNull-rejected; but op 0 → type=-1 → out of [0,5)
	if err := handlePOpPlayer(s); err == nil {
		t.Error("P_OPPLAYER op=0: want range error")
	}
}

func TestPOpPlayer_OpSix(t *testing.T) {
	mp := &mockPlayer{}
	mp2 := &mockPlayer{}
	s := &ScriptState{
		Self:        mp,
		Self2:       mp2,
		Pointers:    PtrActivePlayer | PtrActivePlayer2,
		Protect:     true,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	s.PushInt(6) // type = 5; type >= 5 fails
	if err := handlePOpPlayer(s); err == nil {
		t.Error("P_OPPLAYER op=6: want range error")
	}
}

func TestPOpPlayer_OpNullSentinel(t *testing.T) {
	mp := &mockPlayer{}
	mp2 := &mockPlayer{}
	s := &ScriptState{
		Self:        mp,
		Self2:       mp2,
		Pointers:    PtrActivePlayer | PtrActivePlayer2,
		Protect:     true,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	s.PushInt(-1)
	if err := handlePOpPlayer(s); err == nil {
		t.Error("P_OPPLAYER op=-1: want NumberNotNull error")
	}
}
```

- [ ] **Step 2: Run to verify RED**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestPOpPlayer -v`

Expected: FAIL — `undefined: handlePOpPlayer`.

### Task 2B.9: Implement `handlePOpPlayer` + dispatch

**Files:**
- Modify: `pkg/script/handlers_player.go`
- Modify: `pkg/script/handlers.go`

- [ ] **Step 1: Add handler**

```go
// handlePOpPlayer (P_OPPLAYER, opcode 2081) anchors the active player on the
// secondary active player (Self2) with the APPLAYER<op>/OPPLAYER<op> trigger
// family. Mirrors TS PlayerOps.ts:1009-1020
// (https://x.com/JagexAsh/status/1791472651623370843):
//
//	const type = check(state.popInt(), NumberNotNull) - 1;
//	if (type < 0 || type >= 5) {
//	    throw new Error(`Invalid opplayer: ${type + 1}`);
//	}
//	const target = state._activePlayer2;
//	if (!target) { return; }
//	state.activePlayer.stopAction();
//	state.activePlayer.setInteraction(Interaction.SCRIPT, target,
//	    ServerTriggerType.APPLAYER1 + type);
//
// Gate: ProtectedActivePlayer. The popped op is 1-indexed (1..5); after
// subtracting 1 it must be in [0,4]. Self2-nil is a silent return (TS-faithful).
// NAI-120 Bundle 2B.
func handlePOpPlayer(s *ScriptState) error {
	if err := requireProtectedActivePlayer(s, "P_OPPLAYER"); err != nil {
		return err
	}
	op := s.PopInt()
	if err := checkNotNull(op, "P_OPPLAYER"); err != nil {
		return err
	}
	idx := op - 1
	if idx < 0 || idx >= 5 {
		return fmt.Errorf("P_OPPLAYER: invalid op %d", op)
	}
	if s.Self2 == nil {
		return nil // TS-faithful silent return
	}
	s.Self.StopAction()
	s.Self.SetInteractionScriptPlayer(s.Self2, op)
	return nil
}
```

- [ ] **Step 2: Add dispatch**

```go
	OpPOpPlayer: handlePOpPlayer,
```

- [ ] **Step 3: Run to verify GREEN**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestPOpPlayer -v`

Expected: PASS for all six subtests.

### Task 2B.10: Cross-package green + commit

- [ ] **Step 1: Run full test suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`

Expected: PASS.

- [ ] **Step 2: Commit**

```bash
git add pkg/script/active.go pkg/script/handlers.go pkg/script/handlers_player.go pkg/script/handlers_player_test.go pkg/script/runner_test.go modules/world/player_script.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): NAI-120 Bundle 2B — port busy2, p_opnpct, p_opplayer

Adds the three player-interaction ops surfaced by NAI-120 Bundle 1 audit.
New ActivePlayer surface: HasInteraction, HasWaypoints,
SetInteractionScriptNpcT (spellCom-bearing), SetInteractionScriptPlayer
(1-indexed op, routed through APPLAYER<op>). Tests pin the dual-OR gate in
BUSY2, the protect+activeNpc gate in P_OPNPCT, and the silent-return on
Self2=nil in P_OPPLAYER (TS-faithful per PlayerOps.ts:1015).

EOF
)"
```

### Task 2B.11: Code-reviewer dispatch

- [ ] **Step 1: Dispatch Sonnet reviewer**

Same template as 2A.11, citing audit §1, §9, §10 and TS PlayerOps.ts:898-900, 417-421, 1009-1020.

---

## Bundle 2C — NPC-write ops (REQUIRED)

**Handlers ported:** `npc_statadd` (2538), `npc_statsub` (2540), `spotanim_npc` (2547).

**New interface surface:**
- `ActiveNpc.SetNpcStat(stat, level int)`
- `ActiveNpc.PlaySpotAnim(id, height, delay int)`
- `checkNpcStatID(id int, op string) error` helper in `handlers_npc.go`

**Goal:** Bundle 2C unblocks NPC stat boost/drain procs called from the combat dispatch chain plus NPC-side spotanim graphics in `player_magic.rs2`.

### Task 2C.0: Pre-flight

- [ ] **Step 1: Verify the 3 (D) opcodes still undispatched**

Run: `rg -n "OpNpcStatAdd|OpNpcStatSub|OpSpotAnimNpc" pkg/script/handlers.go`

Expected: No matches.

- [ ] **Step 2: Verify ActiveNpc interface lacks SetNpcStat / PlaySpotAnim**

Run: `rg -n "SetNpcStat|PlaySpotAnim" pkg/script/active.go`

Expected: No matches.

- [ ] **Step 3: Verify checkSpotAnimType is reusable + NpcStatCount = 6**

Run:
```bash
rg -n "func checkSpotAnimType|NpcStatCount\s*=\s*6" pkg/script/handlers_map.go pkg/objtype/npctype.go
```

Expected: `checkSpotAnimType` at `handlers_map.go:212`, `NpcStatCount = 6` at `objtype/npctype.go:22`.

### Task 2C.1: Add `SetNpcStat` to ActiveNpc + helper

**Files:**
- Modify: `pkg/script/active.go`
- Modify: `pkg/script/handlers_npc.go` (add checkNpcStatID helper)
- Modify: `modules/world/npc_script.go`
- Modify: `pkg/script/handlers_npc_test.go` (extend mockNpc)

- [ ] **Step 1: Add interface method**

In `pkg/script/active.go`, in the `ActiveNpc` interface, near the existing `NpcStat` / `NpcBaseStat` reads:

```go
	// SetNpcStat writes `level` into the NPC's current (boosted) stat slot
	// `stat`. OOB stats are dropped silently (impl bounds-checks against
	// objtype.NpcStatCount=6). Used by NPC_STATADD / NPC_STATSUB. Mirrors TS
	// `npc.levels[stat] = ...` in NpcOps.ts:492-518. NAI-120 Bundle 2C.
	SetNpcStat(stat, level int)
```

- [ ] **Step 2: Add the helper**

Append to `pkg/script/handlers_npc.go` (place near `checkNpcType`, before the stat-mutation handlers):

```go
// checkNpcStatID validates a stat id against objtype.NpcStatCount. Mirrors
// TS NpcStatValid (ScriptValidators.ts) — range [0, NpcStatCount). NAI-120
// Bundle 2C.
func checkNpcStatID(id int, op string) error {
	if id < 0 || id >= objtype.NpcStatCount {
		return fmt.Errorf("%s: npc stat id out of range (%d)", op, id)
	}
	return nil
}
```

- [ ] **Step 3: Add concrete impl on `*Npc`**

In `modules/world/npc_script.go`, near the `NpcStat` / `NpcBaseStat` getters:

```go
// SetNpcStat writes the NPC's current (boosted) stat. OOB drops silently.
// NAI-120 Bundle 2C.
func (n *Npc) SetNpcStat(stat, level int) {
	if stat < 0 || stat >= objtype.NpcStatCount {
		return
	}
	n.levels[stat] = level
}
```

- [ ] **Step 4: Extend mockNpc**

In `pkg/script/handlers_npc_test.go`, add fields to the `mockNpc` struct:

```go
	// NAI-120 Bundle 2C: NPC_STATADD/SUB capture. The mock stores levels in
	// a map (sparse) so tests pin specific stat slots without seeding all 6.
	levels        map[int]int
	baseLevels    map[int]int
	setNpcStatCalls []struct{ stat, level int }
```

Wait — the existing mockNpc already has `curHP, baseHP int` for stat=0 (HP) only. We need a richer model. Replace the `NpcStat` and `NpcBaseStat` mock methods:

```go
func (m *mockNpc) NpcStat(stat int) int {
	if m.levels != nil {
		if v, ok := m.levels[stat]; ok {
			return v
		}
	}
	if stat == 0 {
		return m.curHP
	}
	return 0
}

func (m *mockNpc) NpcBaseStat(stat int) int {
	if m.baseLevels != nil {
		if v, ok := m.baseLevels[stat]; ok {
			return v
		}
	}
	if stat == 0 {
		return m.baseHP
	}
	return 0
}
```

(This preserves the existing curHP/baseHP behaviour as a fallback while letting Bundle 2C tests seed arbitrary stat slots via the maps.)

Add the recorder method:

```go
func (m *mockNpc) SetNpcStat(stat, level int) {
	m.setNpcStatCalls = append(m.setNpcStatCalls, struct{ stat, level int }{stat, level})
	if m.levels == nil {
		m.levels = make(map[int]int)
	}
	m.levels[stat] = level
}
```

- [ ] **Step 5: Cross-package build**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...`

Expected: Clean. (If any sibling test file has its own ActiveNpc fixture that doesn't impl SetNpcStat, it will fail to compile — add the stub there.)

### Task 2C.2: Write failing test for `npc_statadd`

**Files:**
- Modify: `pkg/script/handlers_npc_test.go`

- [ ] **Step 1: Add tests**

```go
func TestNpcStatAdd_HappyPath(t *testing.T) {
	npc := &mockNpc{
		baseLevels: map[int]int{0: 70},
		levels:     map[int]int{0: 50},
	}
	s := &ScriptState{
		ActiveNpc:   npc,
		Pointers:    PtrActiveNpc,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	// Pop order: percent (top), constant, stat (bottom).
	s.PushInt(0)  // stat
	s.PushInt(5)  // constant
	s.PushInt(10) // percent
	if err := handleNpcStatAdd(s); err != nil {
		t.Fatalf("NPC_STATADD happy: unexpected error %v", err)
	}
	// 50 + (5 + 70*10/100) = 50 + (5 + 7) = 62
	if got := len(npc.setNpcStatCalls); got != 1 {
		t.Fatalf("NPC_STATADD happy: SetNpcStat calls = %d, want 1", got)
	}
	call := npc.setNpcStatCalls[0]
	if call.stat != 0 || call.level != 62 {
		t.Errorf("NPC_STATADD happy: SetNpcStat(%d,%d), want (0,62)", call.stat, call.level)
	}
}

func TestNpcStatAdd_CapAt255(t *testing.T) {
	npc := &mockNpc{
		baseLevels: map[int]int{0: 100},
		levels:     map[int]int{0: 250},
	}
	s := &ScriptState{
		ActiveNpc:   npc,
		Pointers:    PtrActiveNpc,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	s.PushInt(0)
	s.PushInt(10)
	s.PushInt(100)
	if err := handleNpcStatAdd(s); err != nil {
		t.Fatalf("NPC_STATADD cap: unexpected error %v", err)
	}
	// 250 + (10 + 100*100/100) = 250 + 110 = 360 → clamp 255.
	if got := npc.setNpcStatCalls[0].level; got != 255 {
		t.Errorf("NPC_STATADD cap: level = %d, want 255", got)
	}
}

func TestNpcStatAdd_NoActiveNpc(t *testing.T) {
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	s.PushInt(0)
	s.PushInt(5)
	s.PushInt(10)
	if err := handleNpcStatAdd(s); err == nil {
		t.Error("NPC_STATADD no active npc: want error")
	}
}

func TestNpcStatAdd_StatOOB(t *testing.T) {
	npc := &mockNpc{}
	s := &ScriptState{
		ActiveNpc:   npc,
		Pointers:    PtrActiveNpc,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	s.PushInt(6) // OOB
	s.PushInt(5)
	s.PushInt(10)
	if err := handleNpcStatAdd(s); err == nil {
		t.Error("NPC_STATADD stat=6: want range error")
	}
}

func TestNpcStatAdd_ConstantNull(t *testing.T) {
	npc := &mockNpc{baseLevels: map[int]int{0: 10}, levels: map[int]int{0: 5}}
	s := &ScriptState{
		ActiveNpc:   npc,
		Pointers:    PtrActiveNpc,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	s.PushInt(0)
	s.PushInt(-1) // null
	s.PushInt(10)
	if err := handleNpcStatAdd(s); err == nil {
		t.Error("NPC_STATADD constant=-1: want NumberNotNull error")
	}
}

func TestNpcStatAdd_PercentNull(t *testing.T) {
	npc := &mockNpc{baseLevels: map[int]int{0: 10}, levels: map[int]int{0: 5}}
	s := &ScriptState{
		ActiveNpc:   npc,
		Pointers:    PtrActiveNpc,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	s.PushInt(0)
	s.PushInt(5)
	s.PushInt(-1) // null
	if err := handleNpcStatAdd(s); err == nil {
		t.Error("NPC_STATADD percent=-1: want NumberNotNull error")
	}
}
```

- [ ] **Step 2: Run to verify RED**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestNpcStatAdd -v`

Expected: FAIL — `undefined: handleNpcStatAdd`.

### Task 2C.3: Implement `handleNpcStatAdd` + dispatch

**Files:**
- Modify: `pkg/script/handlers_npc.go`
- Modify: `pkg/script/handlers.go`

- [ ] **Step 1: Add handler**

Append to `pkg/script/handlers_npc.go`:

```go
// handleNpcStatAdd (NPC_STATADD, opcode 2538) boosts the active NPC's stat.
// Pop order: percent (top), constant, stat (bottom). Formula clamped at 255:
//
//	added = current + trunc(constant + (base*percent)/100)
//	npc.levels[stat] = min(added, 255)
//
// Mirrors TS NpcOps.ts:492-504. NAI-120 Bundle 2C.
func handleNpcStatAdd(s *ScriptState) error {
	if err := requireActiveNpc(s, "NPC_STATADD"); err != nil {
		return err
	}
	percent := s.PopInt()
	if err := checkNotNull(percent, "NPC_STATADD"); err != nil {
		return err
	}
	constant := s.PopInt()
	if err := checkNotNull(constant, "NPC_STATADD"); err != nil {
		return err
	}
	stat := s.PopInt()
	if err := checkNpcStatID(stat, "NPC_STATADD"); err != nil {
		return err
	}
	base := s.ActiveNpc.NpcBaseStat(stat)
	cur := s.ActiveNpc.NpcStat(stat)
	added := cur + (constant + (base*percent)/100)
	if added > 255 {
		added = 255
	}
	s.ActiveNpc.SetNpcStat(stat, added)
	return nil
}
```

- [ ] **Step 2: Add dispatch**

```go
	OpNpcStatAdd: handleNpcStatAdd,
```

- [ ] **Step 3: Run to verify GREEN**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestNpcStatAdd -v`

Expected: PASS.

### Task 2C.4: Write failing test for `npc_statsub`

**Files:**
- Modify: `pkg/script/handlers_npc_test.go`

- [ ] **Step 1: Add tests**

```go
func TestNpcStatSub_HappyPath(t *testing.T) {
	npc := &mockNpc{
		baseLevels: map[int]int{0: 70},
		levels:     map[int]int{0: 50},
	}
	s := &ScriptState{
		ActiveNpc:   npc,
		Pointers:    PtrActiveNpc,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	s.PushInt(0)
	s.PushInt(5)
	s.PushInt(10)
	if err := handleNpcStatSub(s); err != nil {
		t.Fatalf("NPC_STATSUB happy: unexpected error %v", err)
	}
	// 50 - (5 + 70*10/100) = 50 - 12 = 38
	if got := npc.setNpcStatCalls[0].level; got != 38 {
		t.Errorf("NPC_STATSUB happy: level = %d, want 38", got)
	}
}

func TestNpcStatSub_FloorAtZero(t *testing.T) {
	npc := &mockNpc{
		baseLevels: map[int]int{0: 70},
		levels:     map[int]int{0: 5},
	}
	s := &ScriptState{
		ActiveNpc:   npc,
		Pointers:    PtrActiveNpc,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	s.PushInt(0)
	s.PushInt(100)
	s.PushInt(100)
	if err := handleNpcStatSub(s); err != nil {
		t.Fatalf("NPC_STATSUB floor: unexpected error %v", err)
	}
	// 5 - (100 + 70) = -165 → clamp 0.
	if got := npc.setNpcStatCalls[0].level; got != 0 {
		t.Errorf("NPC_STATSUB floor: level = %d, want 0", got)
	}
}

func TestNpcStatSub_NoActiveNpc(t *testing.T) {
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	s.PushInt(0)
	s.PushInt(5)
	s.PushInt(10)
	if err := handleNpcStatSub(s); err == nil {
		t.Error("NPC_STATSUB no active npc: want error")
	}
}

func TestNpcStatSub_StatOOB(t *testing.T) {
	npc := &mockNpc{}
	s := &ScriptState{
		ActiveNpc:   npc,
		Pointers:    PtrActiveNpc,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	s.PushInt(-1)
	s.PushInt(5)
	s.PushInt(10)
	if err := handleNpcStatSub(s); err == nil {
		t.Error("NPC_STATSUB stat=-1: want range error")
	}
}

func TestNpcStatSub_ConstantNull(t *testing.T) {
	npc := &mockNpc{baseLevels: map[int]int{0: 10}, levels: map[int]int{0: 5}}
	s := &ScriptState{
		ActiveNpc:   npc,
		Pointers:    PtrActiveNpc,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	s.PushInt(0)
	s.PushInt(-1)
	s.PushInt(10)
	if err := handleNpcStatSub(s); err == nil {
		t.Error("NPC_STATSUB constant=-1: want NumberNotNull error")
	}
}
```

- [ ] **Step 2: Run to verify RED**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestNpcStatSub -v`

Expected: FAIL.

### Task 2C.5: Implement `handleNpcStatSub` + dispatch

**Files:**
- Modify: `pkg/script/handlers_npc.go`
- Modify: `pkg/script/handlers.go`

- [ ] **Step 1: Add handler**

```go
// handleNpcStatSub (NPC_STATSUB, opcode 2540) drains the active NPC's stat.
// Pop order matches NPC_STATADD. Formula clamped at 0:
//
//	subbed = current - trunc(constant + (base*percent)/100)
//	npc.levels[stat] = max(subbed, 0)
//
// Mirrors TS NpcOps.ts:506-518. NAI-120 Bundle 2C.
func handleNpcStatSub(s *ScriptState) error {
	if err := requireActiveNpc(s, "NPC_STATSUB"); err != nil {
		return err
	}
	percent := s.PopInt()
	if err := checkNotNull(percent, "NPC_STATSUB"); err != nil {
		return err
	}
	constant := s.PopInt()
	if err := checkNotNull(constant, "NPC_STATSUB"); err != nil {
		return err
	}
	stat := s.PopInt()
	if err := checkNpcStatID(stat, "NPC_STATSUB"); err != nil {
		return err
	}
	base := s.ActiveNpc.NpcBaseStat(stat)
	cur := s.ActiveNpc.NpcStat(stat)
	subbed := cur - (constant + (base*percent)/100)
	if subbed < 0 {
		subbed = 0
	}
	s.ActiveNpc.SetNpcStat(stat, subbed)
	return nil
}
```

- [ ] **Step 2: Add dispatch**

```go
	OpNpcStatSub: handleNpcStatSub,
```

- [ ] **Step 3: Run to verify GREEN**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestNpcStatSub -v`

Expected: PASS.

### Task 2C.6: Add `PlaySpotAnim` to ActiveNpc

**Files:**
- Modify: `pkg/script/active.go`
- Modify: `modules/world/npc_script.go`
- Modify: `pkg/script/handlers_npc_test.go` (extend mockNpc)

- [ ] **Step 1: Add interface method**

In `pkg/script/active.go`, ActiveNpc interface, near `Animate`:

```go
	// PlaySpotAnim schedules a spotanim graphic on the NPC for this tick
	// at the given height with the given client-side delay. Used by
	// SPOTANIM_NPC (opcode 2547). Mirrors TS Npc.spotanim. NAI-120 Bundle 2C.
	PlaySpotAnim(id, height, delay int)
```

- [ ] **Step 2: Add concrete impl on `*Npc`**

In `modules/world/npc_script.go`, near `Animate`:

```go
// PlaySpotAnim schedules a spotanim on the NPC. Sets MaskSpotAnim and
// stashes the (id, height, delay) tuple for the NPC-info encoder. NAI-120
// Bundle 2C.
//
// If the goscape NPC encoder does not yet read a spotanim mask field on
// *Npc, the encoder-side wiring is a deferred follow-up; this method
// flags the mask AND writes the spotanim slot so the next encoder pass
// reads coherent state. Confirm the mask constant (NpcMaskSpotAnim, or
// equivalent) before writing.
func (n *Npc) PlaySpotAnim(id, height, delay int) {
	n.spotanimID = id
	n.spotanimHeight = height
	n.spotanimDelay = delay
	n.masks |= NpcMaskSpotAnim
}
```

**NOTE for the executing engineer:** Before writing this body, grep `modules/world/npc.go` for the actual fields on `*Npc` that the NPC-info encoder reads:

```bash
rg -n "spotanim|SpotAnim|masks|NpcMask" modules/world/npc.go modules/world/npc_info.go
```

If no spotanim-related fields exist on `*Npc` yet, add them (3 fields: `spotanimID, spotanimHeight, spotanimDelay int`) and the corresponding mask constant. The encoder-side write (NpcMaskSpotAnim → wire bytes) is OUT OF SCOPE for NAI-120; mark with a deviation comment if the encoder doesn't read this field yet:

```go
// NAI-120 Bundle 2C deviation: spotanim mask write surface added; encoder
// consumption pending NAI-121+ (NPC update mask audit). Tutorial-island
// combat smoke does not read NPC spotanims off the wire — this state is
// purely producer-side scaffolding for future encoder wiring.
```

If the fields and mask constant DON'T exist and adding them is non-trivial, a reduced scope is acceptable: skeleton the method as a no-op with the deviation comment, then file a frontier follow-up. Tutorial-island melee combat does not actually emit NPC spotanims.

- [ ] **Step 3: Add mockNpc field + method**

In `pkg/script/handlers_npc_test.go`:

```go
	// NAI-120 Bundle 2C: SPOTANIM_NPC capture.
	playSpotAnimCalls []struct{ id, height, delay int }
```

```go
func (m *mockNpc) PlaySpotAnim(id, height, delay int) {
	m.playSpotAnimCalls = append(m.playSpotAnimCalls, struct{ id, height, delay int }{id, height, delay})
}
```

- [ ] **Step 4: Cross-package build**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...`

Expected: Clean.

### Task 2C.7: Write failing test for `spotanim_npc`

**Files:**
- Modify: `pkg/script/handlers_npc_test.go`

- [ ] **Step 1: Locate or create a configs fixture that registers spotanim id 5**

Look at how `checkSpotAnimType` is exercised in the existing test for SPOTANIM_PL. Search for `mockConfigs` and `SpotAnimType`:

```bash
rg -n "SpotAnimType\(|mockConfigs.*spotanim" pkg/script/
```

Reuse the existing fixture pattern. If there's a `newTestConfigsWithSpotAnims(map[int]bool)` helper, use it. If not, follow the `newTestConfigsWithNpcTypes` shape and add an analogous helper.

- [ ] **Step 2: Add tests**

```go
func TestSpotAnimNpc_HappyPath(t *testing.T) {
	npc := &mockNpc{}
	cfg := newTestConfigsWithSpotAnims(map[int]bool{5: true})
	s := &ScriptState{
		ActiveNpc:   npc,
		Configs:     cfg,
		Pointers:    PtrActiveNpc,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	// Pop order: delay (top), height, spotanim id (bottom).
	s.PushInt(5)  // spotanim id
	s.PushInt(92) // height
	s.PushInt(30) // delay
	if err := handleSpotAnimNpc(s); err != nil {
		t.Fatalf("SPOTANIM_NPC happy: unexpected error %v", err)
	}
	if got := len(npc.playSpotAnimCalls); got != 1 {
		t.Fatalf("SPOTANIM_NPC happy: PlaySpotAnim calls = %d, want 1", got)
	}
	call := npc.playSpotAnimCalls[0]
	if call.id != 5 || call.height != 92 || call.delay != 30 {
		t.Errorf("SPOTANIM_NPC happy: PlaySpotAnim(%d,%d,%d), want (5,92,30)", call.id, call.height, call.delay)
	}
}

func TestSpotAnimNpc_NoActiveNpc(t *testing.T) {
	cfg := newTestConfigsWithSpotAnims(map[int]bool{5: true})
	s := &ScriptState{
		Configs:     cfg,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	s.PushInt(5)
	s.PushInt(92)
	s.PushInt(30)
	if err := handleSpotAnimNpc(s); err == nil {
		t.Error("SPOTANIM_NPC no active npc: want error")
	}
}

func TestSpotAnimNpc_DelayNull(t *testing.T) {
	npc := &mockNpc{}
	cfg := newTestConfigsWithSpotAnims(map[int]bool{5: true})
	s := &ScriptState{
		ActiveNpc:   npc,
		Configs:     cfg,
		Pointers:    PtrActiveNpc,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	s.PushInt(5)
	s.PushInt(92)
	s.PushInt(-1)
	if err := handleSpotAnimNpc(s); err == nil {
		t.Error("SPOTANIM_NPC delay=-1: want NumberNotNull error")
	}
}

func TestSpotAnimNpc_HeightNull(t *testing.T) {
	npc := &mockNpc{}
	cfg := newTestConfigsWithSpotAnims(map[int]bool{5: true})
	s := &ScriptState{
		ActiveNpc:   npc,
		Configs:     cfg,
		Pointers:    PtrActiveNpc,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	s.PushInt(5)
	s.PushInt(-1)
	s.PushInt(30)
	if err := handleSpotAnimNpc(s); err == nil {
		t.Error("SPOTANIM_NPC height=-1: want NumberNotNull error")
	}
}

func TestSpotAnimNpc_InvalidSpotAnim(t *testing.T) {
	npc := &mockNpc{}
	cfg := newTestConfigsWithSpotAnims(map[int]bool{5: true})
	s := &ScriptState{
		ActiveNpc:   npc,
		Configs:     cfg,
		Pointers:    PtrActiveNpc,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	s.PushInt(999) // not in registry
	s.PushInt(92)
	s.PushInt(30)
	if err := handleSpotAnimNpc(s); err == nil {
		t.Error("SPOTANIM_NPC invalid id: want SpotAnimTypeValid error")
	}
}
```

- [ ] **Step 3: Run to verify RED**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestSpotAnimNpc -v`

Expected: FAIL — `undefined: handleSpotAnimNpc` (and possibly `newTestConfigsWithSpotAnims` if not yet added; add it via the same shape as `newTestConfigsWithNpcTypes`).

### Task 2C.8: Implement `handleSpotAnimNpc` + dispatch

**Files:**
- Modify: `pkg/script/handlers_npc.go`
- Modify: `pkg/script/handlers.go`

- [ ] **Step 1: Add handler**

```go
// handleSpotAnimNpc (SPOTANIM_NPC, opcode 2547) queues a spotanim on the
// active NPC. Pop order: delay (top), height, spotanim id (bottom). Mirrors
// TS NpcOps.ts:282-288:
//
//	const delay = check(state.popInt(), NumberNotNull);
//	const height = check(state.popInt(), NumberNotNull);
//	const spotanimType = check(state.popInt(), SpotAnimTypeValid);
//	state.activeNpc.spotanim(spotanimType.id, height, delay);
//
// NAI-120 Bundle 2C.
func handleSpotAnimNpc(s *ScriptState) error {
	if err := requireActiveNpc(s, "SPOTANIM_NPC"); err != nil {
		return err
	}
	delay := s.PopInt()
	if err := checkNotNull(delay, "SPOTANIM_NPC"); err != nil {
		return err
	}
	height := s.PopInt()
	if err := checkNotNull(height, "SPOTANIM_NPC"); err != nil {
		return err
	}
	id := s.PopInt()
	if err := checkSpotAnimType(s, id, "SPOTANIM_NPC"); err != nil {
		return err
	}
	s.ActiveNpc.PlaySpotAnim(id, height, delay)
	return nil
}
```

- [ ] **Step 2: Add dispatch**

```go
	OpSpotAnimNpc: handleSpotAnimNpc,
```

- [ ] **Step 3: Run to verify GREEN**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestSpotAnimNpc -v`

Expected: PASS.

### Task 2C.9: Cross-package green + commit

- [ ] **Step 1: Run full test suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`

Expected: PASS.

- [ ] **Step 2: Commit**

```bash
git add pkg/script/active.go pkg/script/handlers.go pkg/script/handlers_npc.go pkg/script/handlers_npc_test.go modules/world/npc_script.go modules/world/npc.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): NAI-120 Bundle 2C — port npc_statadd, npc_statsub, spotanim_npc

Ports the three NPC-write ops from NAI-120 Bundle 1 audit. New ActiveNpc
surface: SetNpcStat (with NpcStatCount bounds-check via checkNpcStatID
helper) and PlaySpotAnim. Stat-mutation tests pin the formula's truncation
behaviour and the 255-cap / 0-floor clamps. SPOTANIM_NPC reuses the
existing checkSpotAnimType validator from handlers_map.go.

EOF
)"
```

### Task 2C.10: Code-reviewer dispatch

Same template as 2A.11 / 2B.11, citing audit §7, §8, §11 and TS NpcOps.ts:492-518, 282-288.

---

## Bundle 2D — `npc_heropoints` (REQUIRED)

**Handler ported:** `npc_heropoints` (2524).

**New interface surface:** `ActiveNpc.AddHeroPoints(playerUID, amount int)`

**New infrastructure:** `HeroPoints` struct in `modules/world/heropoints.go` (capped 16-entry per-player contribution ledger).

**Goal:** Bundle 2D unblocks the hero-point tracker write that runs after damage in `~combat_maxhit` (called from `[label,player_melee_attack]`).

### Task 2D.0: Pre-flight

- [ ] **Step 1: Verify the (D) opcode still undispatched**

Run: `rg -n "OpNpcHeroPoints" pkg/script/handlers.go`

Expected: No matches.

- [ ] **Step 2: Verify HeroPoints is absent from goscape**

Run: `rg -n "heroPoints|HeroPoints" modules/world/ pkg/script/`

Expected: No matches (or only the planning doc).

- [ ] **Step 3: Verify ActivePlayer.UID() exists**

Run: `rg -n "UID\(\)\s+int" pkg/script/active.go`

Expected: One match in the ActivePlayer interface.

### Task 2D.1: Add the `HeroPoints` structure

**Files:**
- Create: `modules/world/heropoints.go`

- [ ] **Step 1: Create the file**

```go
package world

// HeroPoints is the per-NPC ledger that tracks each player's damage
// contribution (or other hero-point credit) toward the NPC. The largest
// contributor at death gets the loot. Capped at 16 entries — TS uses
// `new HeroPoints(16)` for combat NPCs (Engine-TS/.../HeroPoints.ts).
//
// Lookups and writes are O(N) over the slice — N <= 16. Insertions evict
// the lowest-contribution entry when full and the incoming amount exceeds
// it; otherwise the incoming write is dropped (TS-faithful).
//
// Read by World on NPC death to choose the loot recipient. The death-side
// reader is OUT OF SCOPE for NAI-120 — only the writer (NPC_HEROPOINTS
// opcode) lands here. NAI-120 Bundle 2D.
type HeroPoints struct {
	cap     int
	entries []heroEntry
}

type heroEntry struct {
	playerUID int
	amount    int
}

// NewHeroPoints constructs an empty HeroPoints with the given cap.
func NewHeroPoints(cap int) HeroPoints {
	return HeroPoints{cap: cap}
}

// AddHero credits `amount` to `playerUID`. If the player already has an
// entry, increments it. Otherwise inserts a new entry; if the ledger is
// full, evicts the lowest-contribution entry only when `amount` strictly
// exceeds it. Mirrors TS HeroPoints.addHero. amount=0 is a valid
// contribution (TS NumberNotNull only rejects -1) — recorded as a
// 0-amount entry when there's room, dropped when full (cannot beat any
// existing entry's amount strictly). NAI-120 Bundle 2D.
func (h *HeroPoints) AddHero(playerUID, amount int) {
	for i := range h.entries {
		if h.entries[i].playerUID == playerUID {
			h.entries[i].amount += amount
			return
		}
	}
	if len(h.entries) < h.cap {
		h.entries = append(h.entries, heroEntry{playerUID, amount})
		return
	}
	// Full: evict lowest if amount exceeds it.
	lowestIdx := 0
	for i := 1; i < len(h.entries); i++ {
		if h.entries[i].amount < h.entries[lowestIdx].amount {
			lowestIdx = i
		}
	}
	if amount > h.entries[lowestIdx].amount {
		h.entries[lowestIdx] = heroEntry{playerUID, amount}
	}
}

// TopContributor returns the playerUID with the highest contribution, or 0
// if the ledger is empty. (Stub for future loot-routing consumer.)
func (h *HeroPoints) TopContributor() int {
	if len(h.entries) == 0 {
		return 0
	}
	bestIdx := 0
	for i := 1; i < len(h.entries); i++ {
		if h.entries[i].amount > h.entries[bestIdx].amount {
			bestIdx = i
		}
	}
	return h.entries[bestIdx].playerUID
}
```

- [ ] **Step 2: Add a HeroPoints field on `*Npc`**

In `modules/world/npc.go`, locate the `type Npc struct {` block. After the `levels` / `baseLevels` fields:

```go
	heroPoints HeroPoints // NAI-120 Bundle 2D — per-NPC contribution ledger
```

Locate the constructor (`NewNpc` or equivalent) and initialise:

```go
	heroPoints: NewHeroPoints(16),
```

- [ ] **Step 3: Cross-package build**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...`

Expected: Clean.

### Task 2D.2: Add `AddHeroPoints` to ActiveNpc

**Files:**
- Modify: `pkg/script/active.go`
- Modify: `modules/world/npc_script.go`
- Modify: `pkg/script/handlers_npc_test.go` (mockNpc)

- [ ] **Step 1: Add interface method**

In `pkg/script/active.go`:

```go
	// AddHeroPoints credits `amount` to `playerUID` on the NPC's hero-point
	// ledger. Used by NPC_HEROPOINTS (opcode 2524) to track damage
	// contributions for loot routing. Mirrors TS Npc.heroPoints.addHero(...).
	// NAI-120 Bundle 2D.
	AddHeroPoints(playerUID, amount int)
```

- [ ] **Step 2: Add concrete impl on `*Npc`**

In `modules/world/npc_script.go`:

```go
// AddHeroPoints implements script.ActiveNpc. NAI-120 Bundle 2D.
func (n *Npc) AddHeroPoints(playerUID, amount int) {
	n.heroPoints.AddHero(playerUID, amount)
}
```

- [ ] **Step 3: Add mockNpc field + method**

In `pkg/script/handlers_npc_test.go`:

```go
	// NAI-120 Bundle 2D: NPC_HEROPOINTS capture.
	addHeroPointsCalls []struct{ playerUID, amount int }
```

```go
func (m *mockNpc) AddHeroPoints(playerUID, amount int) {
	m.addHeroPointsCalls = append(m.addHeroPointsCalls, struct{ playerUID, amount int }{playerUID, amount})
}
```

- [ ] **Step 4: Cross-package build**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...`

Expected: Clean.

### Task 2D.3: Write failing test for `npc_heropoints`

**Files:**
- Modify: `pkg/script/handlers_npc_test.go`

- [ ] **Step 1: Add tests**

```go
func TestNpcHeroPoints_HappyPath(t *testing.T) {
	mp := &mockPlayer{uidValue: 12345}
	npc := &mockNpc{}
	s := &ScriptState{
		Self:        mp,
		ActiveNpc:   npc,
		Pointers:    PtrActivePlayer | PtrActiveNpc,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	s.PushInt(42)
	if err := handleNpcHeroPoints(s); err != nil {
		t.Fatalf("NPC_HEROPOINTS happy: unexpected error %v", err)
	}
	if got := len(npc.addHeroPointsCalls); got != 1 {
		t.Fatalf("NPC_HEROPOINTS happy: AddHeroPoints calls = %d, want 1", got)
	}
	call := npc.addHeroPointsCalls[0]
	if call.playerUID != 12345 || call.amount != 42 {
		t.Errorf("NPC_HEROPOINTS happy: AddHeroPoints(%d,%d), want (12345,42)", call.playerUID, call.amount)
	}
}

func TestNpcHeroPoints_NoActivePlayer(t *testing.T) {
	npc := &mockNpc{}
	s := &ScriptState{
		ActiveNpc:   npc,
		Pointers:    PtrActiveNpc,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	s.PushInt(42)
	if err := handleNpcHeroPoints(s); err == nil {
		t.Error("NPC_HEROPOINTS no active player: want error")
	}
}

func TestNpcHeroPoints_NoActiveNpc(t *testing.T) {
	mp := &mockPlayer{uidValue: 1}
	s := &ScriptState{
		Self:        mp,
		Pointers:    PtrActivePlayer,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	s.PushInt(42)
	if err := handleNpcHeroPoints(s); err == nil {
		t.Error("NPC_HEROPOINTS no active npc: want error")
	}
}

func TestNpcHeroPoints_AmountNull(t *testing.T) {
	mp := &mockPlayer{uidValue: 1}
	npc := &mockNpc{}
	s := &ScriptState{
		Self:        mp,
		ActiveNpc:   npc,
		Pointers:    PtrActivePlayer | PtrActiveNpc,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	s.PushInt(-1)
	if err := handleNpcHeroPoints(s); err == nil {
		t.Error("NPC_HEROPOINTS amount=-1: want NumberNotNull error")
	}
}

func TestNpcHeroPoints_AmountZero(t *testing.T) {
	mp := &mockPlayer{uidValue: 1}
	npc := &mockNpc{}
	s := &ScriptState{
		Self:        mp,
		ActiveNpc:   npc,
		Pointers:    PtrActivePlayer | PtrActiveNpc,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	s.PushInt(0)
	if err := handleNpcHeroPoints(s); err != nil {
		t.Fatalf("NPC_HEROPOINTS amount=0: unexpected error %v (NumberNotNull only rejects -1)", err)
	}
	// Handler still calls AddHeroPoints(uid, 0); the ledger no-ops on amount=0.
	if got := len(npc.addHeroPointsCalls); got != 1 {
		t.Errorf("NPC_HEROPOINTS amount=0: AddHeroPoints calls = %d, want 1", got)
	}
}
```

- [ ] **Step 2: Run to verify RED**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestNpcHeroPoints -v`

Expected: FAIL — `undefined: handleNpcHeroPoints`.

### Task 2D.4: Implement `handleNpcHeroPoints` + dispatch

**Files:**
- Modify: `pkg/script/handlers_npc.go`
- Modify: `pkg/script/handlers.go`

- [ ] **Step 1: Add handler**

```go
// handleNpcHeroPoints (NPC_HEROPOINTS, opcode 2524) credits the active
// player's UID with `amount` hero points on the active NPC's ledger. Used
// for damage-contribution loot routing on NPC death. Mirrors TS
// NpcOps.ts:478-480 (https://x.com/JagexAsh/status/1704492467226091853):
//
//	state.activeNpc.heroPoints.addHero(state.activePlayer.hash64,
//	    check(state.popInt(), NumberNotNull));
//
// Gate: ProtectedActivePlayer NOT required — TS uses plain ActivePlayer +
// ActiveNpc. Goscape uses player UID instead of TS hash64. NAI-120 Bundle 2D.
func handleNpcHeroPoints(s *ScriptState) error {
	if err := requireActivePlayer(s, "NPC_HEROPOINTS"); err != nil {
		return err
	}
	if err := requireActiveNpc(s, "NPC_HEROPOINTS"); err != nil {
		return err
	}
	amount := s.PopInt()
	if err := checkNotNull(amount, "NPC_HEROPOINTS"); err != nil {
		return err
	}
	s.ActiveNpc.AddHeroPoints(s.Self.UID(), amount)
	return nil
}
```

- [ ] **Step 2: Add dispatch**

```go
	OpNpcHeroPoints: handleNpcHeroPoints,
```

- [ ] **Step 3: Run to verify GREEN**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestNpcHeroPoints -v`

Expected: PASS.

### Task 2D.5: Cross-package green + commit

- [ ] **Step 1: Run full test suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`

Expected: PASS.

- [ ] **Step 2: Commit**

```bash
git add pkg/script/active.go pkg/script/handlers.go pkg/script/handlers_npc.go pkg/script/handlers_npc_test.go modules/world/heropoints.go modules/world/npc.go modules/world/npc_script.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script,world): NAI-120 Bundle 2D — port npc_heropoints + HeroPoints ledger

Adds NPC_HEROPOINTS (opcode 2524) handler with the dual ActivePlayer +
ActiveNpc gate. New infrastructure: HeroPoints struct in modules/world/
(capped 16-entry per-player contribution ledger; AddHero with eviction-on-
amount-exceeds-lowest semantics matching TS HeroPoints). The ledger reader
(loot-routing on NPC death) is OUT OF SCOPE for NAI-120 and tracked as a
frontier item.

EOF
)"
```

### Task 2D.6: Code-reviewer dispatch

Same template, citing audit §5 and TS NpcOps.ts:478-480.

---

## Bundle 2E — `inv_dropitem_delayed` (STRETCH)

**Handler ported:** `inv_dropitem_delayed` (4310).

**New infrastructure:** `WorldVars.AddObjDelayed(...) ActiveObj` + `objDelayedQueue` machinery in `modules/world/`.

**Status:** Bundle 2E is a STRETCH goal. The handler is only used in `player_ranged.rs2` for ammo-drop-on-hit. Tutorial Island combat is melee-only, so the smoke binds without it. Defer Bundle 2E to NAI-121 unless Smoke Handoff (§Smoke) surfaces a `no handler for INV_DROPITEM_DELAYED` warning.

If pursued in NAI-120:

### Task 2E.0: Pre-flight

- [ ] **Step 1: Verify still undispatched**

Run: `rg -n "OpInvDropItemDelayed" pkg/script/handlers.go`

Expected: No matches.

- [ ] **Step 2: Verify ObjDelayedQueue absent**

Run: `rg -n "ObjDelayedQueue|objDelayedQueue|ObjDelayedRequest" modules/world/`

Expected: No matches.

### Task 2E.1: Implement ObjDelayedQueue infrastructure

**Files:**
- Create: `modules/world/obj_delayed_queue.go`
- Modify: `modules/world/server.go` (add field + tick processing)

- [ ] **Step 1: Create the queue file**

The detailed design here exceeds plan scope — the executing engineer should consult `Engine-TS/.../World.ts` for `objDelayedQueue` shape. Minimum viable implementation:

```go
package world

import (
	entitypkg "github.com/zsrv/goscape/pkg/entity"
)

// objDelayedRequest is one queued floor-spawn pending its delay countdown.
// Mirrors TS ObjDelayedRequest. NAI-120 Bundle 2E.
type objDelayedRequest struct {
	obj         *entitypkg.Obj
	duration    int
	delay       int
	receiverUID int
}

// objDelayedQueue is a FIFO queue of pending floor spawns, processed each
// tick. When a request's delay reaches 0, it's spawned via the world's
// addObj path and removed. NAI-120 Bundle 2E.
type objDelayedQueue struct {
	entries []objDelayedRequest
}

func (q *objDelayedQueue) addTail(r objDelayedRequest) {
	q.entries = append(q.entries, r)
}

// processTick decrements all delays and spawns any that have reached 0.
// Called from the world tick loop.
func (q *objDelayedQueue) processTick(s *Server) {
	remaining := q.entries[:0]
	for _, r := range q.entries {
		r.delay--
		if r.delay <= 0 {
			s.AddObj(r.obj, r.receiverUID)
		} else {
			remaining = append(remaining, r)
		}
	}
	q.entries = remaining
}
```

- [ ] **Step 2: Wire into Server**

Add field to `Server` struct:

```go
	objDelayedQueue objDelayedQueue
```

Add tick-loop call in the tick processor (search `s.processX(` patterns to find the right phase):

```go
	s.objDelayedQueue.processTick(s)
```

- [ ] **Step 3: Add `AddObjDelayed` method on Server**

```go
// AddObjDelayed enqueues a floor-spawn for `level, x, z, typeID, count`
// owned by `receiverID`, to be spawned after `delay` ticks and despawned
// after `duration` more ticks. Mirrors TS World.objDelayedQueue.addTail
// + new ObjDelayedRequest(...). NAI-120 Bundle 2E.
func (s *Server) AddObjDelayed(level, x, z, typeID, count, duration, delay, receiverID int) ActiveObj {
	obj := entitypkg.NewObj(level, x, z, typeID, count, duration)
	s.objDelayedQueue.addTail(objDelayedRequest{
		obj:         obj,
		duration:    duration,
		delay:       delay,
		receiverUID: receiverID,
	})
	return obj
}
```

(Adjust `entitypkg.NewObj` signature to match the actual constructor; if there's no DESPAWN-lifecycle constructor variant, refactor accordingly.)

### Task 2E.2: Add `AddObjDelayed` to WorldVars

**Files:**
- Modify: `pkg/script/state.go`
- Modify: `pkg/script/handlers_vars_test.go` (mockWorld stub)

```go
	// AddObjDelayed enqueues a floor-spawn delayed by `delay` ticks.
	// Mirrors TS World.objDelayedQueue.addTail. NAI-120 Bundle 2E.
	AddObjDelayed(level, x, z, typeID, count, duration, delay, receiverID int) ActiveObj
```

```go
func (m *mockWorld) AddObjDelayed(level, x, z, typeID, count, duration, delay, receiverID int) ActiveObj {
	return nil
}
```

### Task 2E.3: Implement `handleInvDropItemDelayed` + tests + dispatch

This handler closely parallels `handleInvDropSlot` (handlers_inv.go:512). Re-read that handler at HEAD before writing — copy-edit the validators and inv-protect gate, then add the `delay` parameter and route via `AddObjDelayed`.

**Files:**
- Modify: `pkg/script/handlers_inv.go`
- Modify: `pkg/script/handlers_inv_test.go`
- Modify: `pkg/script/handlers.go`

Test fixtures need a `fakeWorldAddObjDelayed` recorder paralleling `fakeWorldAddObj` (search for the latter in `handlers_inv_test.go`).

The detailed test bodies and impl are deferred to the executing engineer if Bundle 2E is pursued. Reference: audit §2 for TS pop order `[delay, duration, count, obj, coord, inv]` (delay top), validator chain (InvTypeValid, CoordValid, ObjTypeValid, ObjStackValid, DurationValid), inv-protect gate condition, early-return on invDel=0.

### Task 2E.4: Cross-package green + commit + reviewer

If Bundle 2E lands, follow the same green/commit/reviewer pattern as 2A–2D.

---

## V-PARTIAL: `%npc_combat_xp_multiplier` deferral

Per Bundle 1 audit §12, the `%npc_combat_xp_multiplier` varn reads as 0 for all NPCs until the `[ai_spawn,_]` trigger script is ported. The script body is 2 lines (writes `npc_param(combat_xp_multiplier)` and `npc_coord` into varns). Both opcodes are wired.

**NAI-120 routing decision:** Defer. The varn affects combat-XP scaling, which is not part of the NAI-120 close criterion (smoke binds on "dispatcher reaches `@player_melee_attack` without missing-handler errors", not on XP accuracy). If smoke surfaces XP=0 issues on Tutorial Island progression gates, route the `ai_spawn` script-side port to NAI-121.

**At NAI-120 final close, record in `nai_followups.md`:**
- `NAI-120-V-PARTIAL: %npc_combat_xp_multiplier reads 0 because ai_spawn trigger isn't firing on NPC spawn (or isn't in the loaded script.dat). Effect: all combat XP scales to 0. Routing: NAI-121.`

---

## Bundle 0 follow-ups (record at NAI-120 final close)

Per Bundle 0 findings §8:

- `pkg/script/handlers.go:207-208` carries a stale `stub until S6` doc-comment on PUSH_VARN/POP_VARN; the real handler is wired at `handlers_vars.go:52-69`. Single-line comment cleanup. Track in `nai_followups.md` for NAI-120 close commit.
- `OpPushVarbit` (`opcode.go:52`) and `OpPopVarbit` (`opcode.go:53`) are declared but not dispatched. No inner-ring blocker — defer to first downstream sub-spec that touches a varbit-typed var.

---

## Smoke Handoff

Per spec §7 and `smoke_test_server_handoff`:

- [ ] **Step 1: Build the binary**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache CGO_ENABLED=0 go build -trimpath -o /tmp/goscape ./cmd/goscape`

Expected: Clean build, binary at `/tmp/goscape`.

- [ ] **Step 2: Hand off to user**

Print this message to the user:

> Stage 2 ports (Bundles 2A–2D) are committed. Build is clean.
>
> **Smoke target:** Tutorial Island, attack a giant rat. Expected: combat dispatcher reaches `@player_melee_attack` without `no handler for ...` warnings.
>
> **To run the server:**
> ```bash
> CGO_ENABLED=0 go run -trimpath ./cmd/goscape --config.file config.yaml
> ```
> Then connect via the Java client and progress to the Tutorial Island combat step (attack the giant rat in the dungeon).
>
> Capture the server stderr for any `no handler for ...` lines and report back. If smoke is silent (no missing-handler warnings) and the rat takes damage / first hit lands, NAI-120 closes.

- [ ] **Step 3: Wait for user smoke result**

Do NOT run the server in the agent's sandbox — per `smoke_test_server_handoff`, the Java client cannot reach a sandboxed server process.

---

## Conditional Stage 3 (only if smoke surfaces an inner-ring residual >30 LOC)

Per spec §8 and `cascade_theory_smoke_binding`:

- **Outcome A — clean smoke (no missing-handler warnings, first hit lands):** Close NAI-120. Run final commit, write `Closes memory:` trailer per `close_commit_memory_trailer`, drop frontier list into `nai_followups.md`.
- **Outcome B — outer-ring residual:** New `no handler for ...` warning naming an opcode whose body lives in the frontier list (Bundle 0 §6). Route to NAI-121 (combat-sibling files) or NAI-122 (NPC-side combat) per the table in Bundle 0 §6. Close NAI-120 against scope.
- **Outcome C — inner-ring residual ≤30 LOC:** Apply in-scope-stretch fix per `smoke_surfaces_adjacent_divergences`. Re-smoke.
- **Outcome D — inner-ring residual >30 LOC:** Stage 3 sub-spec inside NAI-120. Investigate root cause (audit subagent if shape is wire-traceable; gated runtime instrumentation if shape is silent per `nai_114_stage3_instrumentation_probe`). Re-dispatch a Stage 2-style bundle.
- **Outcome E — silent breakage (rat takes no damage but no warnings logged):** Stage 3 = gated runtime instrumentation. Cascade-theory binding from `cascade_theory_smoke_binding`: the missing piece may be downstream of dispatch (engine-side reach, hit-roll math, damage application). Per `dispatch_correct_reach_blocked`, NAI-120 closes the PRIMARY (TS-faithful inner-ring port) and routes the SECONDARY (content outcome) to a successor sub-spec.

---

## NAI-120 Final Close

After smoke binds:

- [ ] **Step 1: Record frontier items in `nai_followups.md`**

Append entries for:
- The 28 outer-ring frontier procs/labels from Bundle 0 §6.
- The V-PARTIAL `%npc_combat_xp_multiplier` deferral.
- `handlers.go:207-208` stale doc-comment cleanup.
- `OpPushVarbit` / `OpPopVarbit` dispatch deferral.
- Bundle 2E (`inv_dropitem_delayed`) deferral, if not landed.
- Any deviation tags written during Stage 2 (e.g. `NAI-120 Bundle 2C deviation: spotanim mask write surface added; encoder consumption pending`).

- [ ] **Step 2: Final close commit**

```bash
git add docs/superpowers/closes/2026-05-XX-nai-120-close.md  # if writing a close note
git commit --no-gpg-sign -m "$(cat <<'EOF'
chore(close): NAI-120 — final close after smoke binding

All 11 (D) opcode handlers from Bundle 1 audit ported across Bundles 2A–2D
(2E deferred to NAI-121). Smoke 2026-05-XX confirms combat dispatcher
reaches @player_melee_attack without missing-handler errors.

Closes memory: nai_followups.md (28 frontier items + V-PARTIAL
ai_spawn + handlers.go stub-comment cleanup + OpPushVarbit/OpPopVarbit
dispatch deferral)

EOF
)"
```

---

## Self-review checklist

Before handing off this plan for execution:

- [ ] **Spec coverage:** Every (D) entry from Bundle 1 audit (§1–§11) maps to a Stage 2 task. V-PARTIAL §12 mapped to deferral note. ✓
- [ ] **Placeholder scan:** Greppable for "TBD", "TODO", "implement later" → only references to "future sub-spec" (intentional deferrals). ✓
- [ ] **Type consistency:** Method names match between interface declaration and concrete impl tasks: `HasInteraction`, `HasWaypoints`, `SetInteractionScriptNpcT`, `SetInteractionScriptPlayer`, `SetNpcStat`, `PlaySpotAnim`, `AddHeroPoints`, `IsMulti`, `FindNpcByUID`. ✓
- [ ] **Bundle ordering:** No inter-bundle handler logic deps; interface deps respected (Bundle 2C's mockNpc extension carries through to 2D). ✓
- [ ] **Pre-flight tasks:** Each bundle starts with grep-verified premises per `controller_preflight`. ✓
- [ ] **Test fixtures runnable as-is:** Each `&ScriptState{...}` fixture has IntStack + StringStack init + correct push order + Pointers flag where needed per `scriptstate_test_fixture_idioms`. ✓ (Verify mentally before execution; if any fixture push-order mismatches the handler's pop order, fix at execution time.)
- [ ] **Cross-package green at every commit:** Each bundle ends with `go test ./...` per `verify_implementer_claims`. ✓
- [ ] **Reviewer dispatch:** Sonnet model cap per `superpowers_code_reviewer_model`. ✓
- [ ] **Commit messages:** No-gpg-sign per global CLAUDE.md. ✓
