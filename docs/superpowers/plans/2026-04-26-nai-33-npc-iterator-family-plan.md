# NAI-33 — NPC iterator family port — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Port the four `NPC_FIND*` iterator-family opcodes (NPC_FINDALL=2514, NPC_FINDALLANY=2515, NPC_FINDALLZONE=2516, NPC_FINDNEXT=2520) currently declared but unhandled in goscape's script VM, closing the runtime WARN `no handler for NPC_FINDALLANY (opcode 2515)` triggered by `[proc,check_fishing_spot_empty]`.

**Architecture:** New `*NpcIterator` struct in `pkg/script` with DISTANCE/ZONE modes and lazy per-zone snapshot via a new `NpcLookup.ZoneNpcs` interface method. New `npcIterator *NpcIterator` field on `ScriptState`. Single-tick lifetime via `Stale(currentTick)` check at FINDNEXT — mirrors TS throw-on-stale through goscape's existing handler-error → `npc_script.go:169` log-warn path. Zero termination-path cleanup invariants added.

**Tech Stack:** Go 1.26+. Existing primitives reused: `pkg/zone.Zone.NpcsSafe(true) iter.Seq[NpcLike]` (zone.go:439), `pkg/coordgrid.DistanceToSW(posX, posZ, otherX, otherZ int) int` (coordgrid.go:131), `pkg/script.WorldVars.CurrentTick() int` (state.go:41), `pkg/zone.ZoneMap.Get(level, worldX, worldZ int) *Zone` (map.go), `pkg/script.ActiveNpc.NpcType()/NpcX()/NpcZ()` (active.go:400-408). Existing helpers: `setActiveNpcSlot` (handlers_npc.go:58), `checkCoord`/`checkNotNull`/`checkHuntVis`/`checkNpcType` (handlers_player.go + handlers_npc.go).

**Predecessors:** NAI-32 closed at `9745a9b`. Spec at `docs/superpowers/specs/2026-04-26-nai-33-npc-iterator-family-design.md` (commit `deb0097`).

**Source roots:**
- `LostCityRS/Engine-TS/src/engine/script/handlers/NpcOps.ts:403-441` (handler bodies)
- `LostCityRS/Engine-TS/src/engine/script/ScriptIterators.ts:297-363` (iterator class)
- `LostCityRS/Engine-TS/src/engine/script/ScriptState.ts:125` (state field)
- `LostCityRS/Engine-TS/src/engine/script/ScriptOpcodePointers.ts:586-600` (pointer contract)

**Cadence:** Full per `runescript_cadence.md` (TDD red→green→commit per task; subagent-driven-development per `execution_mode_default.md`; two-stage review at close per `runescript_cadence.md`).

**Test command prefix** (all `go` commands per CLAUDE.md): `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go ...`

**Commit prefix** (per CLAUDE.md): `git commit --no-gpg-sign -m "..."`

---

## File Structure

| File | Action | Responsibility |
|---|---|---|
| `pkg/script/npc_iterator.go` | CREATE | `NpcIteratorMode` enum, `NpcIterator` struct, constructors, `Next()`, `Stale()`, `passesFilter`, `advanceZone` |
| `pkg/script/npc_iterator_test.go` | CREATE | Layer 1 — iterator mechanics: bounds math, cursor order, distance filter, type filter, ZONE mode, stale check |
| `pkg/script/state.go` | MODIFY | Add `ZoneNpcs(level, zoneX, zoneZ int) []ActiveNpc` method to `NpcLookup` interface (~line 80). Add `npcIterator *NpcIterator` field to `ScriptState` struct (after `OtherActiveNpc` at line 157) |
| `pkg/script/handlers_npc.go` | MODIFY | Append 4 new handler funcs after `handleNpcFindExact` (after current line 479) |
| `pkg/script/handlers.go` | MODIFY | Register 4 new opcode→handler entries in dispatch table (alongside existing `OpNpcFind`/`OpNpcFindCat`/`OpNpcFindExact`) |
| `pkg/script/handlers_npc_test.go` | MODIFY | Append Layer 2 handler tests + Layer 4 integration test |
| `pkg/script/runner_test.go` | MODIFY | Extend `mockNpcLookup` (struct definition at line 522) with `byZone`, `zoneNpcsCalls`, `zoneNpcsCallArgs`, plus `ZoneNpcs(...)` method |
| `modules/world/npc_script_lookup.go` | MODIFY | Add `ZoneNpcs(level, zoneX, zoneZ int) []ActiveNpc` method on `serverNpcLookup` |
| `modules/world/npc_script_lookup_test.go` | MODIFY | Append Layer 3 tests for `ZoneNpcs` |

**No changes to:** `pkg/script/runner.go`, `npc_script.go` termination paths, `pkg/zone/`, `pkg/coordgrid/`, opcode definitions in `pkg/script/opcode.go` (constants + names already declared at lines 251-258 and 901-914).

---

## Task 1: Extend `NpcLookup` interface with `ZoneNpcs` method + extend mockNpcLookup

**Files:**
- Modify: `pkg/script/state.go:67-80` (NpcLookup interface body)
- Modify: `pkg/script/runner_test.go:522-550` (mockNpcLookup struct + methods)

- [ ] **Step 1: Add `ZoneNpcs` to `NpcLookup` interface**

Edit `pkg/script/state.go` — inside the `NpcLookup` interface body (between line 79's `FindNpcAtExactCoord` and the closing `}` at ~line 80), add:

```go
	// ZoneNpcs returns all NPCs subscribed to the zone at (level, zoneX, zoneZ),
	// filtered by IsValid. Mirrors TS Zone.getAllNpcsSafe(true) consumed by
	// NpcIterator.generator (ScriptIterators.ts:330,341). zoneX/zoneZ are
	// coord-grid coords (not zone indices); the impl converts via
	// ZoneMap.Get which masks internally. Empty/nil slice on miss.
	// No error path. Used by NPC_FINDALL/FINDALLANY/FINDALLZONE iterators.
	ZoneNpcs(level, zoneX, zoneZ int) []ActiveNpc
```

- [ ] **Step 2: Run `go build` to confirm interface compiles + breaks all implementations**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./pkg/script/...`
Expected: FAIL — `*serverNpcLookup` and `*mockNpcLookup` no longer satisfy `NpcLookup` (will be fixed in subsequent steps + Task 2).

- [ ] **Step 3: Extend `mockNpcLookup` with the new field/counters/method**

Edit `pkg/script/runner_test.go:522-550`. Replace the struct definition + add a new method. Updated struct (insertion shown for the FIELDS block — preserve existing fields):

```go
type mockNpcLookup struct {
	byType     ActiveNpc
	byCategory ActiveNpc
	atCoord    ActiveNpc
	// byZone returns the NPC slice keyed by (level, zoneX, zoneZ) tuple
	// packed via mockZoneKey(level, zoneX, zoneZ). nil entry = empty.
	byZone map[uint64][]ActiveNpc

	byTypeCalls     int
	byCategoryCalls int
	atCoordCalls    int
	zoneNpcsCalls   int

	lastArgs []int
	// zoneNpcsCallArgs records each ZoneNpcs call's (level, zoneX, zoneZ)
	// in call order — used by iterator-cursor-order tests to assert
	// the iterator visits zones in TS line 337-340 order.
	zoneNpcsCallArgs [][3]int
}

func mockZoneKey(level, zoneX, zoneZ int) uint64 {
	return uint64(level&0x3)<<28 | uint64(zoneX&0x3FFF)<<14 | uint64(zoneZ&0x3FFF)
}

// ... existing FindClosest* methods unchanged ...

func (m *mockNpcLookup) ZoneNpcs(level, zoneX, zoneZ int) []ActiveNpc {
	m.zoneNpcsCalls++
	m.zoneNpcsCallArgs = append(m.zoneNpcsCallArgs, [3]int{level, zoneX, zoneZ})
	if m.byZone == nil {
		return nil
	}
	return m.byZone[mockZoneKey(level, zoneX, zoneZ)]
}
```

- [ ] **Step 4: Run `go build` to confirm pkg/script test target compiles**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./pkg/script/... && GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./pkg/script/...`
Expected: pkg/script builds clean (modules/world will still fail since serverNpcLookup hasn't been updated — that's Task 2). Run the test compile to confirm mockNpcLookup is OK:
`GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run NoSuchTest ./pkg/script/...`
Expected: tests compile (no actual run results).

- [ ] **Step 5: Commit**

```bash
git add pkg/script/state.go pkg/script/runner_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): NAI-33 Task 1 — extend NpcLookup with ZoneNpcs + mockNpcLookup recorder

Adds ZoneNpcs(level, zoneX, zoneZ int) []ActiveNpc to the NpcLookup interface
for the NPC_FIND iterator family (NPC_FINDALL/FINDALLANY/FINDALLZONE).
Extends test-side mockNpcLookup with byZone return map + zoneNpcsCalls
counter + zoneNpcsCallArgs sequence recorder for iterator-cursor-order tests.

modules/world/npc_script_lookup.go does NOT yet satisfy the extended
interface; Task 2 will land that.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: Implement `serverNpcLookup.ZoneNpcs` in modules/world

**Files:**
- Modify: `modules/world/npc_script_lookup.go` (add method on `serverNpcLookup`)
- Modify: `modules/world/npc_script_lookup_test.go` (Layer 3 tests)

- [ ] **Step 1: Write failing test fixture in npc_script_lookup_test.go**

Append to `modules/world/npc_script_lookup_test.go`:

```go
// --- NAI-33 Task 2: serverNpcLookup.ZoneNpcs tests ----------------------

func TestServerNpcLookup_ZoneNpcs_EmptyZone(t *testing.T) {
	s := newServerForTest(t) // re-use whatever helper this file already uses; if none, see existing test setup pattern at top of this file
	l := serverNpcLookup{s: s}
	got := l.ZoneNpcs(0, 3200, 3300)
	if len(got) != 0 {
		t.Errorf("empty zone: got len=%d, want 0", len(got))
	}
}

func TestServerNpcLookup_ZoneNpcs_SingleNpc(t *testing.T) {
	s := newServerForTest(t)
	n := addNpcToServer(t, s, /*typeID=*/1, /*x=*/3200, /*z=*/3300, /*level=*/0)
	l := serverNpcLookup{s: s}
	got := l.ZoneNpcs(0, 3200, 3300)
	if len(got) != 1 {
		t.Fatalf("got len=%d, want 1", len(got))
	}
	if got[0] != n {
		t.Errorf("got %v, want %v", got[0], n)
	}
}

func TestServerNpcLookup_ZoneNpcs_OnlyRequestedZone(t *testing.T) {
	s := newServerForTest(t)
	nIn := addNpcToServer(t, s, 1, 3200, 3300, 0)
	_ = addNpcToServer(t, s, 1, 3300, 3400, 0) // different zone
	l := serverNpcLookup{s: s}
	got := l.ZoneNpcs(0, 3200, 3300)
	if len(got) != 1 || got[0] != nIn {
		t.Errorf("got %v, want [%v] from requested zone only", got, nIn)
	}
}

func TestServerNpcLookup_ZoneNpcs_OffGridReturnsEmpty(t *testing.T) {
	s := newServerForTest(t)
	l := serverNpcLookup{s: s}
	got := l.ZoneNpcs(0, -1000, -1000) // outside any allocated zone
	if len(got) != 0 {
		t.Errorf("off-grid: got len=%d, want 0", len(got))
	}
}
```

**Plan-author note**: the helpers `newServerForTest` and `addNpcToServer` may have different names in this test file. Pre-flight grep `rg 'newServer|addNpcTo' modules/world/npc_script_lookup_test.go` and substitute the correct helper names. Existing tests in this file already exercise NPC fixtures for `FindClosestNpcByType` etc. — model on those.

- [ ] **Step 2: Run tests to verify they fail with "method ZoneNpcs not implemented"**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestServerNpcLookup_ZoneNpcs -v`
Expected: COMPILE ERROR — `*serverNpcLookup does not implement script.NpcLookup (missing method ZoneNpcs)`.

- [ ] **Step 3: Implement `serverNpcLookup.ZoneNpcs`**

Append to `modules/world/npc_script_lookup.go` after the existing `FindNpcAtExactCoord` method (after current line 106):

```go
// ZoneNpcs returns all valid NPCs in the zone at (level, zoneX, zoneZ).
// Mirrors TS Zone.getAllNpcsSafe(true) consumed by NpcIterator
// (ScriptIterators.ts:330,341). Zone resolution via pkg/zone.ZoneMap.Get
// which masks the world coords to zone bounds internally. nil zone (off-grid)
// returns nil. NpcsSafe filters non-IsValid entries (zone.go:439).
// reverse=true mirrors TS getAllNpcsSafe(true) traversal order.
func (l serverNpcLookup) ZoneNpcs(level, zoneX, zoneZ int) []ActiveNpc {
	if l.s.zoneMap == nil {
		return nil
	}
	z := l.s.zoneMap.Get(level, zoneX, zoneZ)
	if z == nil {
		return nil
	}
	out := make([]ActiveNpc, 0, z.NpcsCount())
	for n := range z.NpcsSafe(true) {
		out = append(out, n.(ActiveNpc)) // *Npc satisfies both NpcLike and ActiveNpc (per modules/world/npc_script.go:N — verify pre-flight item 6)
	}
	return out
}
```

**Plan-author note**: `out` should be `script.ActiveNpc` type at the package boundary. The file already imports `pkg/script` (visible in existing methods returning `script.ActiveNpc`). The file's existing methods use bare `ActiveNpc` because of an alias or because the file is in `world` package importing `script` with named alias. Pre-flight: `head -10 modules/world/npc_script_lookup.go` to confirm. Adjust the return type / loop body imports if needed.

- [ ] **Step 4: Run tests to verify pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestServerNpcLookup_ZoneNpcs -v`
Expected: 4 PASS.

- [ ] **Step 5: Run full modules/world test suite to confirm no regression**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/...`
Expected: PASS (existing tests + 4 new).

- [ ] **Step 6: Commit**

```bash
git add modules/world/npc_script_lookup.go modules/world/npc_script_lookup_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-33 Task 2 — serverNpcLookup.ZoneNpcs

Implements the ZoneNpcs primitive added to NpcLookup in Task 1.
Reads from pkg/zone.ZoneMap.Get + Zone.NpcsSafe(true) — mirrors TS
Zone.getAllNpcsSafe(true) at ScriptIterators.ts:330,341. Returns nil
for nil zoneMap (defense) and off-grid zones (ZoneMap.Get returns nil).

4 new Layer 3 tests pinning empty / single / requested-zone-only /
off-grid cases.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: Add `NpcIterator` skeleton + `Stale` method

**Files:**
- Create: `pkg/script/npc_iterator.go`
- Create: `pkg/script/npc_iterator_test.go`

- [ ] **Step 1: Write failing test for `Stale`**

Create `pkg/script/npc_iterator_test.go`:

```go
package script

import (
	"testing"
)

func TestNpcIterator_StaleCheck(t *testing.T) {
	it := &NpcIterator{creationTick: 100}
	if it.Stale(100) {
		t.Error("Stale(creationTick) should be false")
	}
	if !it.Stale(101) {
		t.Error("Stale(creationTick+1) should be true")
	}
	if !it.Stale(99) {
		t.Error("Stale(creationTick-1) should be true (any !=)")
	}
}
```

- [ ] **Step 2: Run test to verify it fails (no NpcIterator type)**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/... -run TestNpcIterator_StaleCheck -v`
Expected: COMPILE ERROR — `undefined: NpcIterator`.

- [ ] **Step 3: Create `pkg/script/npc_iterator.go` with skeleton**

```go
package script

import (
	"github.com/zsrv/goscape/pkg/coordgrid"
)

// NpcIteratorMode selects between DISTANCE (square radius around center
// coord, walks zones outer-X-desc/inner-Z-desc) and ZONE (single zone
// at center coord). Mirrors TS NpcIteratorType enum.
type NpcIteratorMode int

const (
	NpcIteratorDistance NpcIteratorMode = iota
	NpcIteratorZone
)

// NpcIterator is the script-VM iterator state for the NPC_FIND iterator
// family (NPC_FINDALL / NPC_FINDALLANY / NPC_FINDALLZONE). Mirrors TS
// NpcIterator at ScriptIterators.ts:297-363.
//
// Lifetime: single-tick. Created by FINDALL*; consumed by FINDNEXT.
// Stale() check at FINDNEXT compares creationTick to World.CurrentTick();
// on mismatch, handler returns error → existing npc_script.go:167-172
// log-warn + ClearActiveScript path runs (mirrors TS throw-on-stale at
// ScriptIterators.ts:332,343).
//
// Ownership: held by ScriptState.npcIterator. Nil = no active iterator.
// No termination-path cleanup needed: Aborted/Finished drops state;
// NpcSuspended carries iterator, but Stale() on resume catches stale use.
type NpcIterator struct {
	mode         NpcIteratorMode
	creationTick int
	lookup       NpcLookup

	// Center + filter config
	level    int
	x, z     int
	distance int // DISTANCE mode only; 0 for ZONE
	huntvis  int // validated at handler; not used as filter (NAI-33-D1)
	typeID   int // -1 = no filter (FINDALLANY, FINDALLZONE); else exact match

	// Zone-cursor (DISTANCE mode)
	minZoneX, maxZoneX int
	minZoneZ, maxZoneZ int
	curZoneX, curZoneZ int
	started            bool

	// Intra-zone snapshot (lazy: filled on zone-entry)
	zoneNpcs []ActiveNpc
	zoneIdx  int
}

// Stale reports whether currentTick differs from the iterator's
// creationTick. FINDNEXT handler MUST check this before calling Next.
// Single-tick lifetime: any drift = stale.
func (it *NpcIterator) Stale(currentTick int) bool {
	return currentTick != it.creationTick
}

// passesFilter applies the per-NPC filter chain in TS line 345-356 order.
// huntvis filtering is intentionally omitted (NAI-33-D1 / S7f-D1 carryover —
// see deviation registry). Accessor names match pkg/script/active.go:400-408
// ActiveNpc interface (NpcX/NpcZ/NpcType, NOT X/Z/Type).
func (it *NpcIterator) passesFilter(npc ActiveNpc) bool {
	if it.mode == NpcIteratorZone {
		return true // ZONE mode: no per-NPC filtering per TS line 329-335
	}
	if coordgrid.DistanceToSW(it.x, it.z, npc.NpcX(), npc.NpcZ()) > it.distance {
		return false
	}
	// huntvis filter intentionally omitted — NAI-33-D1 carryover
	if it.typeID >= 0 && npc.NpcType() != it.typeID {
		return false
	}
	return true
}

// _ ensures coordgrid is imported even if passesFilter is the only use.
var _ = coordgrid.DistanceToSW
```

(The trailing `var _` line is defensive — if a build phase strips passesFilter before Next is wired, the import stays satisfied. Remove it in Task 6 once Next exists.)

- [ ] **Step 4: Run test to verify pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/... -run TestNpcIterator_StaleCheck -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/script/npc_iterator.go pkg/script/npc_iterator_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): NAI-33 Task 3 — NpcIterator skeleton + Stale

Adds NpcIterator struct + NpcIteratorMode enum + Stale() single-tick
lifetime check. Mirrors TS NpcIterator (ScriptIterators.ts:297-363) and
ScriptState.npcIterator field semantics (ScriptState.ts:125).

Constructors and Next() come in Tasks 4-7. passesFilter is wired now
since it has no external deps beyond coordgrid + ActiveNpc.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: `NewDistanceNpcIterator` constructor + bounds-math test

**Files:**
- Modify: `pkg/script/npc_iterator.go` (add constructor)
- Modify: `pkg/script/npc_iterator_test.go` (Layer 1 a — bounds math)

- [ ] **Step 1: Write failing test for bounds math (4 boundary cases)**

Append to `pkg/script/npc_iterator_test.go`:

```go
func TestNpcIterator_DistanceMode_BoundsMath(t *testing.T) {
	cases := []struct {
		name                                                       string
		x, z, distance                                             int
		wantMinZX, wantMaxZX, wantMinZZ, wantMaxZZ, wantCurZX, wantCurZZ int
	}{
		// centerX = x>>3, radius = 1 + distance/8, zone-bounds = center ± radius
		// curZone* starts at (max, max) per TS line 337-340
		{"distance=0 → radius 1", 3200, 3300, 0, 399, 401, 411, 413, 401, 413},
		{"distance=8 → radius 2", 3200, 3300, 8, 398, 402, 410, 414, 402, 414},
		{"distance=15 → radius 2 (15/8=1)", 3200, 3300, 15, 398, 402, 410, 414, 402, 414},
		{"distance=16 → radius 3 (16/8=2)", 3200, 3300, 16, 397, 403, 409, 415, 403, 415},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			it := NewDistanceNpcIterator(nil, /*tick=*/0, /*level=*/0, tc.x, tc.z, tc.distance, /*huntvis=*/0, /*typeID=*/-1)
			if it.minZoneX != tc.wantMinZX || it.maxZoneX != tc.wantMaxZX {
				t.Errorf("X bounds: got [%d, %d], want [%d, %d]", it.minZoneX, it.maxZoneX, tc.wantMinZX, tc.wantMaxZX)
			}
			if it.minZoneZ != tc.wantMinZZ || it.maxZoneZ != tc.wantMaxZZ {
				t.Errorf("Z bounds: got [%d, %d], want [%d, %d]", it.minZoneZ, it.maxZoneZ, tc.wantMinZZ, tc.wantMaxZZ)
			}
			if it.curZoneX != tc.wantCurZX || it.curZoneZ != tc.wantCurZZ {
				t.Errorf("cursor: got (%d, %d), want (%d, %d) (start at max,max)", it.curZoneX, it.curZoneZ, tc.wantCurZX, tc.wantCurZZ)
			}
			if it.mode != NpcIteratorDistance {
				t.Errorf("mode: got %v, want NpcIteratorDistance", it.mode)
			}
		})
	}
}
```

**Math check** (mentally pre-execute per `plan_runnable_test_fixtures.md`):
- `x=3200, z=3300`: centerX = 3200>>3 = 400; centerZ = 3300>>3 = 412 (3300/8 = 412.5, integer 412).
- `distance=0`: radius = 1 + 0/8 = 1. Bounds X = [399, 401], Z = [411, 413]. curZone = (401, 413). ✓
- `distance=8`: radius = 1 + 8/8 = 2. Bounds X = [398, 402], Z = [410, 414]. curZone = (402, 414). ✓
- `distance=15`: radius = 1 + 15/8 = 1+1 = 2. Same as distance=8. ✓
- `distance=16`: radius = 1 + 16/8 = 1+2 = 3. Bounds X = [397, 403], Z = [409, 415]. curZone = (403, 415). ✓

- [ ] **Step 2: Run test to verify it fails (no constructor)**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/... -run TestNpcIterator_DistanceMode_BoundsMath -v`
Expected: COMPILE ERROR — `undefined: NewDistanceNpcIterator`.

- [ ] **Step 3: Add `NewDistanceNpcIterator` to `pkg/script/npc_iterator.go`**

Append to `pkg/script/npc_iterator.go` (and remove the `var _ = coordgrid.DistanceToSW` defensive line):

```go
// NewDistanceNpcIterator constructs an iterator that walks NPCs in zones
// within `distance` of (level, x, z), filtered by huntvis (validated only —
// NAI-33-D1) and typeID (-1 = no filter). Mirrors TS NpcIterator
// constructor at ScriptIterators.ts:310-326 with type=DISTANCE.
//
// Bounds math (per TS line 312-321):
//   centerX = x >> 3
//   radius  = 1 + distance/8       // integer division
//   zone bounds = [center - radius, center + radius]
//
// Cursor starts at (maxZoneX, maxZoneZ) per TS line 337-340; advances
// outer X descending, inner Z descending in advanceZone (Task 7).
func NewDistanceNpcIterator(lookup NpcLookup, tick, level, x, z, distance, huntvis, typeID int) *NpcIterator {
	centerX := x >> 3
	centerZ := z >> 3
	radius := 1 + distance/8
	return &NpcIterator{
		mode:         NpcIteratorDistance,
		creationTick: tick,
		lookup:       lookup,
		level:        level,
		x:            x,
		z:            z,
		distance:     distance,
		huntvis:      huntvis,
		typeID:       typeID,
		minZoneX:     centerX - radius,
		maxZoneX:     centerX + radius,
		minZoneZ:     centerZ - radius,
		maxZoneZ:     centerZ + radius,
		curZoneX:     centerX + radius,
		curZoneZ:     centerZ + radius,
	}
}
```

Also delete the `var _ = coordgrid.DistanceToSW` line added in Task 3.

- [ ] **Step 4: Run test to verify pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/... -run TestNpcIterator_DistanceMode_BoundsMath -v`
Expected: PASS (4 sub-tests).

- [ ] **Step 5: Commit**

```bash
git add pkg/script/npc_iterator.go pkg/script/npc_iterator_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): NAI-33 Task 4 — NewDistanceNpcIterator constructor + bounds math

Implements bounds-math constructor mirroring TS ScriptIterators.ts:310-326.
4 boundary cases pinned: distance=0/8/15/16 cover the integer-division
radius formula (1 + distance/8) and the cursor start at (maxX, maxZ).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 5: `NewZoneNpcIterator` constructor

**Files:**
- Modify: `pkg/script/npc_iterator.go`
- Modify: `pkg/script/npc_iterator_test.go`

- [ ] **Step 1: Write failing test**

Append to `pkg/script/npc_iterator_test.go`:

```go
func TestNpcIterator_ZoneMode_Construction(t *testing.T) {
	it := NewZoneNpcIterator(nil, /*tick=*/42, /*level=*/3, /*x=*/3200, /*z=*/3300)
	if it.mode != NpcIteratorZone {
		t.Errorf("mode: got %v, want NpcIteratorZone", it.mode)
	}
	if it.creationTick != 42 {
		t.Errorf("creationTick: got %d, want 42", it.creationTick)
	}
	if it.level != 3 || it.x != 3200 || it.z != 3300 {
		t.Errorf("center: got (level=%d, x=%d, z=%d), want (3, 3200, 3300)", it.level, it.x, it.z)
	}
	if it.typeID != -1 {
		t.Errorf("typeID: got %d, want -1 (no filter in ZONE mode)", it.typeID)
	}
	if it.started {
		t.Error("started: should be false before first Next call")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/... -run TestNpcIterator_ZoneMode_Construction -v`
Expected: COMPILE ERROR — `undefined: NewZoneNpcIterator`.

- [ ] **Step 3: Add `NewZoneNpcIterator`**

Append to `pkg/script/npc_iterator.go`:

```go
// NewZoneNpcIterator constructs an iterator that yields all NPCs in the
// single zone containing (level, x, z) — no distance/type filtering.
// Mirrors TS NpcIterator constructor at ScriptIterators.ts:310-326 with
// type=ZONE (no npcType arg). Cursor (curZoneX/Z) is set on first Next
// call by advanceZone (Task 6).
func NewZoneNpcIterator(lookup NpcLookup, tick, level, x, z int) *NpcIterator {
	return &NpcIterator{
		mode:         NpcIteratorZone,
		creationTick: tick,
		lookup:       lookup,
		level:        level,
		x:            x,
		z:            z,
		typeID:       -1, // not used in ZONE mode
	}
}
```

- [ ] **Step 4: Run test to verify pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/... -run TestNpcIterator_ZoneMode_Construction -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/script/npc_iterator.go pkg/script/npc_iterator_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): NAI-33 Task 5 — NewZoneNpcIterator constructor

Single-zone iterator constructor. typeID forced to -1 (no per-NPC type
filter in ZONE mode per TS ScriptIterators.ts:329-335). Cursor remains
unset until advanceZone runs in Task 6.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 6: `Next()` for ZONE mode + `advanceZone`

**Files:**
- Modify: `pkg/script/npc_iterator.go`
- Modify: `pkg/script/npc_iterator_test.go`

- [ ] **Step 1: Write failing tests**

Append to `pkg/script/npc_iterator_test.go`:

```go
func TestNpcIterator_ZoneMode_SingleZone(t *testing.T) {
	npc1 := &mockNpc{typeID: 1, x: 3200, z: 3300, level: 0}
	npc2 := &mockNpc{typeID: 2, x: 3201, z: 3301, level: 0}
	lookup := &mockNpcLookup{
		byZone: map[uint64][]ActiveNpc{
			mockZoneKey(0, 3200&^7, 3300&^7): {npc1, npc2},
		},
	}
	it := NewZoneNpcIterator(lookup, 0, 0, 3200, 3300)

	got1, ok1 := it.Next()
	if !ok1 || got1 != npc1 {
		t.Errorf("first: got (%v, %v), want (npc1, true)", got1, ok1)
	}
	got2, ok2 := it.Next()
	if !ok2 || got2 != npc2 {
		t.Errorf("second: got (%v, %v), want (npc2, true)", got2, ok2)
	}
	got3, ok3 := it.Next()
	if ok3 || got3 != nil {
		t.Errorf("third (exhausted): got (%v, %v), want (nil, false)", got3, ok3)
	}

	// Pin: ZoneNpcs called exactly once with zone-aligned coords
	if lookup.zoneNpcsCalls != 1 {
		t.Errorf("zoneNpcsCalls: got %d, want 1 (lazy single fetch)", lookup.zoneNpcsCalls)
	}
	wantArgs := [3]int{0, (3200 >> 3) * 8, (3300 >> 3) * 8} // = (0, 3200, 3296)
	if lookup.zoneNpcsCallArgs[0] != wantArgs {
		t.Errorf("zoneNpcsCallArgs[0]: got %v, want %v", lookup.zoneNpcsCallArgs[0], wantArgs)
	}
}

func TestNpcIterator_ZoneMode_TerminatesAfterOneZone(t *testing.T) {
	// Empty zone, second Next is also (nil, false) — and ZoneNpcs called once
	lookup := &mockNpcLookup{} // byZone nil → returns nil
	it := NewZoneNpcIterator(lookup, 0, 0, 3200, 3300)
	if got, ok := it.Next(); ok || got != nil {
		t.Errorf("first on empty: got (%v, %v), want (nil, false)", got, ok)
	}
	if got, ok := it.Next(); ok || got != nil {
		t.Errorf("second on empty: got (%v, %v), want (nil, false)", got, ok)
	}
	if lookup.zoneNpcsCalls != 1 {
		t.Errorf("zoneNpcsCalls: got %d, want 1 (no re-fetch after exhaustion)", lookup.zoneNpcsCalls)
	}
}
```

**Note**: 3300>>3 = 412; 412*8 = 3296. So zone-aligned z is 3296, not 3300. Verify by hand: 3300 / 8 = 412.5 → int 412 → 412 * 8 = 3296. ✓

- [ ] **Step 2: Run tests to verify fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/... -run TestNpcIterator_ZoneMode -v`
Expected: COMPILE ERROR — `undefined: (*NpcIterator).Next`.

- [ ] **Step 3: Implement `Next()` and `advanceZone()`**

Append to `pkg/script/npc_iterator.go`:

```go
// Next advances the iterator and returns the next matching NPC. Returns
// (nil, false) on exhaustion. Caller must check Stale(currentTick) before
// invoking Next when the single-tick lifetime invariant matters; FINDNEXT
// handler does this. Mirrors TS NpcIterator.generator at
// ScriptIterators.ts:328-362 (the for-of consumption shape).
func (it *NpcIterator) Next() (ActiveNpc, bool) {
	if it.lookup == nil {
		return nil, false
	}
	for {
		// Drain current intra-zone snapshot
		for it.zoneIdx < len(it.zoneNpcs) {
			npc := it.zoneNpcs[it.zoneIdx]
			it.zoneIdx++
			if it.passesFilter(npc) {
				return npc, true
			}
		}
		// Snapshot exhausted; advance zone cursor (or terminate)
		if !it.advanceZone() {
			return nil, false
		}
		it.zoneNpcs = it.lookup.ZoneNpcs(it.level, it.curZoneX*8, it.curZoneZ*8)
		it.zoneIdx = 0
	}
}

// advanceZone moves the (curZoneX, curZoneZ) cursor and returns true if
// a new zone is now selected, false if iteration has exhausted the
// bounding region. Walks outer-X-desc / inner-Z-desc per TS line 337-340.
//
// ZONE mode: returns true exactly once (the single-zone visit), false
// thereafter. ZONE-mode cursor is initialized HERE on first call (the
// constructor leaves curZoneX/Z at zero so the lazy initialization
// sits with the rest of the cursor logic).
func (it *NpcIterator) advanceZone() bool {
	if it.mode == NpcIteratorZone {
		if it.started {
			return false
		}
		it.started = true
		// Initialize cursor for the single zone at (level, x, z)
		it.curZoneX = it.x >> 3
		it.curZoneZ = it.z >> 3
		return true
	}
	// DISTANCE mode
	if !it.started {
		it.started = true
		// Cursor already at (maxX, maxZ) from constructor
		return true
	}
	// Inner Z descends; on underflow, reset to maxZ and outer X descends;
	// on outer-X underflow, exhausted.
	it.curZoneZ--
	if it.curZoneZ < it.minZoneZ {
		it.curZoneZ = it.maxZoneZ
		it.curZoneX--
		if it.curZoneX < it.minZoneX {
			return false
		}
	}
	return true
}
```

**Plan-author note** (per `plan_var_name_collision.md`): mentally compile — no `:=` in either method body collides with field names or parameters. `it.curZoneX*8` is multiplication, not pointer deref. Good.

- [ ] **Step 4: Run tests to verify pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/... -run TestNpcIterator -v`
Expected: PASS for all 5 NpcIterator tests so far (StaleCheck, BoundsMath × 4 sub, ZoneConstruction, ZoneSingleZone, ZoneTerminates).

- [ ] **Step 5: Commit**

```bash
git add pkg/script/npc_iterator.go pkg/script/npc_iterator_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): NAI-33 Task 6 — NpcIterator.Next + advanceZone (ZONE mode)

Implements Next() with lazy per-zone snapshot and advanceZone() with
ZONE-mode single-fetch + DISTANCE-mode outer-X-desc/inner-Z-desc walk
(TS ScriptIterators.ts:328-362). DISTANCE-mode cursor walk is verified
by Task 7's CursorOrder test; ZONE mode pinned now.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 7: DISTANCE-mode `Next()` walk + filter tests

**Files:**
- Modify: `pkg/script/npc_iterator_test.go`

- [ ] **Step 1: Write failing tests for cursor order, distance filter, type filter**

Append to `pkg/script/npc_iterator_test.go`:

```go
func TestNpcIterator_DistanceMode_CursorOrder(t *testing.T) {
	// distance=0 → radius 1 → 9 zones (3x3) walked outer-X-desc, inner-Z-desc.
	// centerX=400, centerZ=412, bounds X=[399,401], Z=[411,413].
	// Expected zone visit order (in zone-aligned coord-grid coords, *8):
	// (401,413), (401,412), (401,411),  ← x=401 inner z desc
	// (400,413), (400,412), (400,411),  ← x=400
	// (399,413), (399,412), (399,411).  ← x=399
	// Per TS line 337-340: outer X descending, inner Z descending.
	lookup := &mockNpcLookup{} // byZone nil → returns nil per zone (empty)
	it := NewDistanceNpcIterator(lookup, 0, 0, 3200, 3300, 0, 0, -1)

	// Drain — Next loops until exhaustion. Empty zones produce no yields,
	// so we just drive Next() until it returns false.
	for {
		if _, ok := it.Next(); !ok {
			break
		}
	}

	want := [][3]int{
		{0, 401 * 8, 413 * 8},
		{0, 401 * 8, 412 * 8},
		{0, 401 * 8, 411 * 8},
		{0, 400 * 8, 413 * 8},
		{0, 400 * 8, 412 * 8},
		{0, 400 * 8, 411 * 8},
		{0, 399 * 8, 413 * 8},
		{0, 399 * 8, 412 * 8},
		{0, 399 * 8, 411 * 8},
	}
	if len(lookup.zoneNpcsCallArgs) != len(want) {
		t.Fatalf("zone visits: got %d, want %d. Sequence: %v", len(lookup.zoneNpcsCallArgs), len(want), lookup.zoneNpcsCallArgs)
	}
	for i := range want {
		if lookup.zoneNpcsCallArgs[i] != want[i] {
			t.Errorf("visit[%d]: got %v, want %v", i, lookup.zoneNpcsCallArgs[i], want[i])
		}
	}
}

func TestNpcIterator_DistanceMode_DistanceFilter(t *testing.T) {
	// Center (3200, 3300, lvl=0); distance=5. Zone-aligned coords for
	// the center zone are (3200, 3296) (3300>>3*8 = 3296). Place 3 NPCs
	// at: dist 0 (in), dist 5 (in, equal), dist 6 (out).
	// All three live in the SAME zone since they're within ~7 tiles.
	npcIn0  := &mockNpc{typeID: 1, x: 3200, z: 3300, level: 0} // dist 0
	npcIn5  := &mockNpc{typeID: 1, x: 3205, z: 3300, level: 0} // dist 5
	npcOut6 := &mockNpc{typeID: 1, x: 3206, z: 3300, level: 0} // dist 6 → filter out
	zoneKey := mockZoneKey(0, 3200, 3296)
	lookup := &mockNpcLookup{byZone: map[uint64][]ActiveNpc{zoneKey: {npcIn0, npcIn5, npcOut6}}}
	it := NewDistanceNpcIterator(lookup, 0, 0, 3200, 3300, /*distance=*/5, 0, -1)

	yielded := []ActiveNpc{}
	for {
		n, ok := it.Next()
		if !ok {
			break
		}
		yielded = append(yielded, n)
	}
	if len(yielded) != 2 || yielded[0] != npcIn0 || yielded[1] != npcIn5 {
		t.Errorf("yielded: got %v, want [npcIn0, npcIn5]", yielded)
	}
}

func TestNpcIterator_DistanceMode_TypeFilter(t *testing.T) {
	// 2 NPCs in same zone, different types. Filter on typeID=42 yields only the matching one.
	npcMatch := &mockNpc{typeID: 42, x: 3200, z: 3300, level: 0}
	npcMiss  := &mockNpc{typeID: 99, x: 3201, z: 3300, level: 0}
	zoneKey := mockZoneKey(0, 3200, 3296)
	lookup := &mockNpcLookup{byZone: map[uint64][]ActiveNpc{zoneKey: {npcMiss, npcMatch}}}
	it := NewDistanceNpcIterator(lookup, 0, 0, 3200, 3300, 5, 0, /*typeID=*/42)

	yielded := []ActiveNpc{}
	for {
		n, ok := it.Next()
		if !ok {
			break
		}
		yielded = append(yielded, n)
	}
	if len(yielded) != 1 || yielded[0] != npcMatch {
		t.Errorf("typeID=42: got %v, want [npcMatch only]", yielded)
	}

	// Negative-branch: typeID=-1 yields BOTH. Per test_passes_for_wrong_reason.md.
	lookup2 := &mockNpcLookup{byZone: map[uint64][]ActiveNpc{zoneKey: {npcMiss, npcMatch}}}
	it2 := NewDistanceNpcIterator(lookup2, 0, 0, 3200, 3300, 5, 0, /*typeID=*/-1)
	yielded2 := []ActiveNpc{}
	for {
		n, ok := it2.Next()
		if !ok {
			break
		}
		yielded2 = append(yielded2, n)
	}
	if len(yielded2) != 2 {
		t.Errorf("typeID=-1: got len=%d, want 2 (no filter)", len(yielded2))
	}
}
```

**Math check** (per `plan_runnable_test_fixtures.md`): `coordgrid.DistanceToSW` is the existing helper at `pkg/coordgrid/coordgrid.go:131`. Verify it computes `max(abs(dx), abs(dz))` semantically (Chebyshev) — pre-flight reads the body if needed. For (3200, 3300) → (3206, 3300): dx=6, dz=0, distance=6 > 5 → filtered. ✓

- [ ] **Step 2: Run tests to verify fail (will already pass since Task 6 implemented Next — these are confirmation tests)**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/... -run TestNpcIterator_DistanceMode -v`
Expected: All NEW tests should PASS (Next was fully implemented in Task 6, including the DISTANCE-mode walk and filter chain). If they fail, debug Task 6 implementation.

If they all pass, this is a **green-on-arrival** task — the implementation was already complete; these tests pin behavior that was implemented in Task 6. Per TDD discipline, the failing-first step is unmet for these confirmation tests but valuable as quality gates.

- [ ] **Step 3: Run full pkg/script test suite to confirm no regression**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/...`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add pkg/script/npc_iterator_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
test(script): NAI-33 Task 7 — DISTANCE-mode cursor order + filter tests

Pins three quirks: (a) zone visit order outer-X-desc / inner-Z-desc per TS
ScriptIterators.ts:337-340 (dual-pin per ts_asymmetry_dual_pin.md);
(b) Chebyshev distance filter via coordgrid.DistanceToSW with both
in-bounds and out-of-bounds NPCs; (c) typeID filter with both
positive (typeID=42 yields one) and negative (typeID=-1 yields both)
branches per test_passes_for_wrong_reason.md.

All tests green-on-arrival — Next/advanceZone were complete in Task 6;
these tests pin the behavior.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 8: Add `npcIterator` field to `ScriptState`

**Files:**
- Modify: `pkg/script/state.go` (insert after `OtherActiveNpc` field at line 157)

- [ ] **Step 1: Insert field**

Edit `pkg/script/state.go`. Find the `OtherActiveNpc ActiveNpc` line at 157 (within `ScriptState` struct). Insert AFTER it (and before the next field block):

```go

	// npcIterator holds the active NPC_FIND iterator state. Set by
	// FINDALL/FINDALLANY/FINDALLZONE; consumed by FINDNEXT. Lifetime is
	// single-tick — Stale() check enforced at FINDNEXT against
	// s.World.CurrentTick(). Nil = no active iterator. Mirrors TS
	// ScriptState.npcIterator (ScriptState.ts:125). Lowercase = package-
	// private; handlers in pkg/script access directly. NAI-33.
	npcIterator *NpcIterator
```

- [ ] **Step 2: Verify compiles**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./pkg/script/... && GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/...`
Expected: build PASS; tests PASS (no test references the new field yet).

- [ ] **Step 3: Commit**

```bash
git add pkg/script/state.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): NAI-33 Task 8 — npcIterator field on ScriptState

Adds *NpcIterator field for the NPC_FIND iterator family handlers.
Lowercase (package-private). No new termination-path cleanup needed:
GC handles Aborted/Finished; Stale() check at FINDNEXT handles the
NpcSuspended carry-over case.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 9: `handleNpcFindAllAny` (opcode 2515) + tests

**Files:**
- Modify: `pkg/script/handlers_npc.go` (append after `handleNpcFindExact` at line 479)
- Modify: `pkg/script/handlers_npc_test.go` (append after existing FIND tests around line 1530)

- [ ] **Step 1: Write failing tests**

Append to `pkg/script/handlers_npc_test.go`:

```go
// --- NAI-33 Task 9: NPC_FINDALLANY handler tests -----------------------

// newNpcFindAllAnyState pushes (coord, distance, checkVis) — TS popInts(3) order.
func newNpcFindAllAnyState(t *testing.T, coord, distance, huntvis int, lookup *mockNpcLookup) *ScriptState {
	t.Helper()
	s := &ScriptState{
		Script:      &ScriptFile{IntOperands: []int32{0}},
		PC:          0,
		Configs:     newTestConfigsWithNpcTypes(map[int]bool{}),
		Npcs:        lookup,
		World:       &mockWorldVars{tick: 100}, // verify mockWorldVars name pre-flight
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	s.PushInt(coord)
	s.PushInt(distance)
	s.PushInt(huntvis)
	return s
}

func TestNpcFindAllAny_SetsIterator(t *testing.T) {
	lookup := &mockNpcLookup{}
	coord := (2 << 28) | (3200 << 14) | 3300
	s := newNpcFindAllAnyState(t, coord, /*distance=*/10, /*huntvis=*/0, lookup)

	if err := handleNpcFindAllAny(s); err != nil {
		t.Fatalf("handleNpcFindAllAny: %v", err)
	}
	if s.npcIterator == nil {
		t.Fatal("npcIterator should be non-nil after FINDALLANY")
	}
	if s.npcIterator.mode != NpcIteratorDistance {
		t.Errorf("mode: got %v, want NpcIteratorDistance", s.npcIterator.mode)
	}
	if s.npcIterator.typeID != -1 {
		t.Errorf("typeID: got %d, want -1 (FINDALLANY = no type filter)", s.npcIterator.typeID)
	}
	if s.npcIterator.creationTick != 100 {
		t.Errorf("creationTick: got %d, want 100", s.npcIterator.creationTick)
	}
	if s.npcIterator.level != 2 || s.npcIterator.x != 3200 || s.npcIterator.z != 3300 {
		t.Errorf("center: got (level=%d, x=%d, z=%d)", s.npcIterator.level, s.npcIterator.x, s.npcIterator.z)
	}
	if s.npcIterator.distance != 10 {
		t.Errorf("distance: got %d, want 10", s.npcIterator.distance)
	}
	if s.ISP != 0 {
		t.Errorf("FINDALLANY should not push; ISP=%d", s.ISP)
	}
}

func TestNpcFindAllAny_PopOrder(t *testing.T) {
	// Distinguishable values — if pop order is wrong, the validators (or
	// the iterator's stored fields) catch it. Use coord=valid, distance=99,
	// huntvis=0 (valid). Then assert iterator.distance == 99 (not 0 or coord).
	lookup := &mockNpcLookup{}
	coord := (0 << 28) | (3200 << 14) | 3300
	s := newNpcFindAllAnyState(t, coord, /*distance=*/99, /*huntvis=*/0, lookup)

	if err := handleNpcFindAllAny(s); err != nil {
		t.Fatalf("handleNpcFindAllAny: %v", err)
	}
	if s.npcIterator.distance != 99 {
		t.Errorf("distance pop order wrong: got %d, want 99", s.npcIterator.distance)
	}
	if s.npcIterator.huntvis != 0 {
		t.Errorf("huntvis pop order wrong: got %d, want 0", s.npcIterator.huntvis)
	}
}

func TestNpcFindAllAny_InvalidCoord(t *testing.T) {
	s := newNpcFindAllAnyState(t, /*coord=*/-1, 10, 0, &mockNpcLookup{})
	if err := handleNpcFindAllAny(s); err == nil {
		t.Fatal("expected validator error for coord=-1")
	} else if !strings.Contains(err.Error(), "NPC_FINDALLANY: coord out of range") {
		t.Errorf("wrong error: %v", err)
	}
}

func TestNpcFindAllAny_NullDistance(t *testing.T) {
	coord := (0 << 28) | (3200 << 14) | 3300
	s := newNpcFindAllAnyState(t, coord, /*distance=*/-1, 0, &mockNpcLookup{})
	if err := handleNpcFindAllAny(s); err == nil {
		t.Fatal("expected validator error for null distance")
	} else if !strings.Contains(err.Error(), "NPC_FINDALLANY") {
		t.Errorf("wrong error: %v", err)
	}
}

func TestNpcFindAllAny_InvalidHuntVis(t *testing.T) {
	coord := (0 << 28) | (3200 << 14) | 3300
	s := newNpcFindAllAnyState(t, coord, 10, /*huntvis=*/99, &mockNpcLookup{})
	if err := handleNpcFindAllAny(s); err == nil {
		t.Fatal("expected validator error for invalid huntvis")
	}
}

func TestNpcFindAllAny_NilNpcLookup(t *testing.T) {
	coord := (0 << 28) | (3200 << 14) | 3300
	s := newNpcFindAllAnyState(t, coord, 10, 0, nil)
	s.Npcs = nil // explicit
	if err := handleNpcFindAllAny(s); err != nil {
		t.Fatalf("handleNpcFindAllAny with nil Npcs: %v", err)
	}
	if s.npcIterator != nil {
		t.Error("npcIterator should remain nil when Npcs is nil (degrades to FINDNEXT push-0)")
	}
}
```

**Plan-author note**: `mockWorldVars` may have a different name. Pre-flight: `rg 'WorldVars' pkg/script/*_test.go | head` to find the existing test fixture for WorldVars, and substitute. If none exists, create a minimal one in this same task:

```go
type mockWorldVars struct {
	tick int
}
func (m *mockWorldVars) CurrentTick() int               { return m.tick }
func (m *mockWorldVars) PlayerCount() int               { return 0 }
func (m *mockWorldVars) VarsInt(id int) int32           { return 0 }
func (m *mockWorldVars) SetVarsInt(id int, v int32)     {}
func (m *mockWorldVars) VarsString(id int) string       { return "" }
func (m *mockWorldVars) SetVarsString(id int, v string) {}
// ... add stubs for any other WorldVars methods at HEAD per state.go:30-46
```

Verify the full WorldVars surface with `grep -A 30 'type WorldVars interface' pkg/script/state.go`.

- [ ] **Step 2: Run tests to verify fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/... -run TestNpcFindAllAny -v`
Expected: COMPILE ERROR — `undefined: handleNpcFindAllAny`.

- [ ] **Step 3: Implement `handleNpcFindAllAny`**

Append to `pkg/script/handlers_npc.go` after `handleNpcFindExact` (after line 479):

```go
// handleNpcFindAllAny (NPC_FINDALLANY, opcode 2515) pops (coord, distance,
// huntvis), validates, and stores a DISTANCE-mode NpcIterator on
// state.npcIterator with no type filter. Mirrors TS NpcOps.ts:403-411.
// Pointer-set is `set ['find_npc']` (ScriptOpcodePointers.ts:586-588);
// goscape encodes the find_npc pointer as state.npcIterator != nil.
// No push (TS doesn't push either). Carries NAI-33-D1: huntvis validated
// but not used as filter (S7f-D1 carryover).
func handleNpcFindAllAny(s *ScriptState) error {
	checkVis := s.PopInt()
	distance := s.PopInt()
	coord := s.PopInt()

	level, x, z, err := checkCoord(coord, "NPC_FINDALLANY")
	if err != nil {
		return err
	}
	if err := checkNotNull(distance, "NPC_FINDALLANY"); err != nil {
		return err
	}
	if err := checkHuntVis(checkVis, "NPC_FINDALLANY"); err != nil {
		return err
	}

	// Mirror existing FIND handler nil-Npcs degradation pattern: skip
	// iterator creation; FINDNEXT's nil-iterator branch pushes 0.
	if s.Npcs == nil {
		return nil
	}
	s.npcIterator = NewDistanceNpcIterator(
		s.Npcs, s.World.CurrentTick(),
		level, x, z, distance, checkVis, /*typeID=*/ -1,
	)
	return nil
}
```

- [ ] **Step 4: Run tests to verify pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/... -run TestNpcFindAllAny -v`
Expected: 6 PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/script/handlers_npc.go pkg/script/handlers_npc_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): NAI-33 Task 9 — handleNpcFindAllAny (opcode 2515)

Implements the proximate-fix opcode that closes the runtime WARN
triggered by [proc,check_fishing_spot_empty]. Pop order matches TS
popInts(3): checkVis (top), distance, coord (bottom). Sets a
DISTANCE-mode NpcIterator with typeID=-1 (no type filter — FINDALLANY).
Nil-Npcs degradation: skip iterator creation; FINDNEXT pushes 0.

6 tests pin: side-effect, pop order, validators (coord/distance/huntvis),
nil-Npcs degradation. Dispatch table not yet wired (Task 13).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 10: `handleNpcFindAll` (opcode 2514) + tests

**Files:**
- Modify: `pkg/script/handlers_npc.go`
- Modify: `pkg/script/handlers_npc_test.go`

- [ ] **Step 1: Write failing tests**

Append to `pkg/script/handlers_npc_test.go`:

```go
// --- NAI-33 Task 10: NPC_FINDALL handler tests -------------------------

// newNpcFindAllState pushes (coord, npcType, distance, huntvis) — popInts(4) order.
func newNpcFindAllState(t *testing.T, coord, npcTypeID, distance, huntvis int, loaded map[int]bool, lookup *mockNpcLookup) *ScriptState {
	t.Helper()
	s := &ScriptState{
		Script:      &ScriptFile{IntOperands: []int32{0}},
		PC:          0,
		Configs:     newTestConfigsWithNpcTypes(loaded),
		Npcs:        lookup,
		World:       &mockWorldVars{tick: 100},
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	s.PushInt(coord)
	s.PushInt(npcTypeID)
	s.PushInt(distance)
	s.PushInt(huntvis)
	return s
}

func TestNpcFindAll_SetsIteratorWithTypeFilter(t *testing.T) {
	coord := (2 << 28) | (3200 << 14) | 3300
	s := newNpcFindAllState(t, coord, /*npcTypeID=*/7, /*distance=*/10, 0, map[int]bool{7: true}, &mockNpcLookup{})
	if err := handleNpcFindAll(s); err != nil {
		t.Fatalf("handleNpcFindAll: %v", err)
	}
	if s.npcIterator == nil {
		t.Fatal("npcIterator should be non-nil")
	}
	if s.npcIterator.typeID != 7 {
		t.Errorf("typeID: got %d, want 7 (FINDALL = type filter set)", s.npcIterator.typeID)
	}
	if s.npcIterator.mode != NpcIteratorDistance {
		t.Errorf("mode: got %v, want NpcIteratorDistance", s.npcIterator.mode)
	}
}

func TestNpcFindAll_InvalidNpcType(t *testing.T) {
	coord := (0 << 28) | (3200 << 14) | 3300
	s := newNpcFindAllState(t, coord, /*npcTypeID=*/99, 10, 0, map[int]bool{7: true}, &mockNpcLookup{})
	if err := handleNpcFindAll(s); err == nil {
		t.Fatal("expected NpcType validator error for unloaded npcTypeID")
	} else if !strings.Contains(err.Error(), "NPC_FINDALL") {
		t.Errorf("wrong error: %v", err)
	}
}

func TestNpcFindAll_NilNpcLookup(t *testing.T) {
	coord := (0 << 28) | (3200 << 14) | 3300
	s := newNpcFindAllState(t, coord, 7, 10, 0, map[int]bool{7: true}, nil)
	s.Npcs = nil
	if err := handleNpcFindAll(s); err != nil {
		t.Fatalf("handleNpcFindAll: %v", err)
	}
	if s.npcIterator != nil {
		t.Error("nil Npcs → no iterator")
	}
}
```

- [ ] **Step 2: Run tests to verify fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/... -run TestNpcFindAll -v`
Expected: COMPILE ERROR — `undefined: handleNpcFindAll`.

- [ ] **Step 3: Implement `handleNpcFindAll`**

Append to `pkg/script/handlers_npc.go`:

```go
// handleNpcFindAll (NPC_FINDALL, opcode 2514) pops (coord, npc, distance,
// huntvis), validates, and stores a DISTANCE-mode NpcIterator with
// typeID set to filter by NPC type. Mirrors TS NpcOps.ts:413-422.
// Pop order matches TS popInts(4): top → bottom = checkVis, distance,
// npcTypeID, coord. NAI-33-D1: huntvis validated but not used as filter.
func handleNpcFindAll(s *ScriptState) error {
	checkVis := s.PopInt()
	distance := s.PopInt()
	npcTypeID := s.PopInt()
	coord := s.PopInt()

	level, x, z, err := checkCoord(coord, "NPC_FINDALL")
	if err != nil {
		return err
	}
	if err := checkNotNull(distance, "NPC_FINDALL"); err != nil {
		return err
	}
	if err := checkNpcType(s, npcTypeID, "NPC_FINDALL"); err != nil {
		return err
	}
	if err := checkHuntVis(checkVis, "NPC_FINDALL"); err != nil {
		return err
	}

	if s.Npcs == nil {
		return nil
	}
	s.npcIterator = NewDistanceNpcIterator(
		s.Npcs, s.World.CurrentTick(),
		level, x, z, distance, checkVis, npcTypeID,
	)
	return nil
}
```

- [ ] **Step 4: Run tests to verify pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/... -run TestNpcFindAll -v`
Expected: 3 PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/script/handlers_npc.go pkg/script/handlers_npc_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): NAI-33 Task 10 — handleNpcFindAll (opcode 2514)

DISTANCE-mode iterator with typeID filter. Pop order popInts(4):
top → bottom = checkVis, distance, npcTypeID, coord. Mirrors TS
NpcOps.ts:413-422.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 11: `handleNpcFindAllZone` (opcode 2516) + tests

**Files:**
- Modify: `pkg/script/handlers_npc.go`
- Modify: `pkg/script/handlers_npc_test.go`

- [ ] **Step 1: Write failing tests**

Append to `pkg/script/handlers_npc_test.go`:

```go
// --- NAI-33 Task 11: NPC_FINDALLZONE handler tests ---------------------

func newNpcFindAllZoneState(t *testing.T, coord int, lookup *mockNpcLookup) *ScriptState {
	t.Helper()
	s := &ScriptState{
		Script:      &ScriptFile{IntOperands: []int32{0}},
		PC:          0,
		Configs:     newTestConfigsWithNpcTypes(map[int]bool{}),
		Npcs:        lookup,
		World:       &mockWorldVars{tick: 100},
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	s.PushInt(coord)
	return s
}

func TestNpcFindAllZone_SetsZoneIterator(t *testing.T) {
	coord := (2 << 28) | (3200 << 14) | 3300
	s := newNpcFindAllZoneState(t, coord, &mockNpcLookup{})
	if err := handleNpcFindAllZone(s); err != nil {
		t.Fatalf("handleNpcFindAllZone: %v", err)
	}
	if s.npcIterator == nil {
		t.Fatal("npcIterator should be non-nil")
	}
	if s.npcIterator.mode != NpcIteratorZone {
		t.Errorf("mode: got %v, want NpcIteratorZone", s.npcIterator.mode)
	}
	if s.npcIterator.level != 2 || s.npcIterator.x != 3200 || s.npcIterator.z != 3300 {
		t.Errorf("center: got (level=%d, x=%d, z=%d)", s.npcIterator.level, s.npcIterator.x, s.npcIterator.z)
	}
	if s.npcIterator.typeID != -1 {
		t.Errorf("typeID: got %d, want -1 (no filter in ZONE mode)", s.npcIterator.typeID)
	}
}

func TestNpcFindAllZone_InvalidCoord(t *testing.T) {
	s := newNpcFindAllZoneState(t, /*coord=*/-1, &mockNpcLookup{})
	if err := handleNpcFindAllZone(s); err == nil {
		t.Fatal("expected coord validator error")
	} else if !strings.Contains(err.Error(), "NPC_FINDALLZONE: coord out of range") {
		t.Errorf("wrong error: %v", err)
	}
}

func TestNpcFindAllZone_NilNpcLookup(t *testing.T) {
	coord := (0 << 28) | (3200 << 14) | 3300
	s := newNpcFindAllZoneState(t, coord, nil)
	s.Npcs = nil
	if err := handleNpcFindAllZone(s); err != nil {
		t.Fatalf("handleNpcFindAllZone: %v", err)
	}
	if s.npcIterator != nil {
		t.Error("nil Npcs → no iterator")
	}
}
```

- [ ] **Step 2: Run tests to verify fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/... -run TestNpcFindAllZone -v`
Expected: COMPILE ERROR — `undefined: handleNpcFindAllZone`.

- [ ] **Step 3: Implement `handleNpcFindAllZone`**

Append to `pkg/script/handlers_npc.go`:

```go
// handleNpcFindAllZone (NPC_FINDALLZONE, opcode 2516) pops a coord,
// validates, and stores a ZONE-mode NpcIterator targeting the single
// zone containing that coord. Mirrors TS NpcOps.ts:424-428. No
// distance/huntvis/type validation (TS doesn't do them either).
func handleNpcFindAllZone(s *ScriptState) error {
	coord := s.PopInt()
	level, x, z, err := checkCoord(coord, "NPC_FINDALLZONE")
	if err != nil {
		return err
	}
	if s.Npcs == nil {
		return nil
	}
	s.npcIterator = NewZoneNpcIterator(s.Npcs, s.World.CurrentTick(), level, x, z)
	return nil
}
```

- [ ] **Step 4: Run tests to verify pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/... -run TestNpcFindAllZone -v`
Expected: 3 PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/script/handlers_npc.go pkg/script/handlers_npc_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): NAI-33 Task 11 — handleNpcFindAllZone (opcode 2516)

ZONE-mode iterator, single-zone scope, coord-only validation.
Mirrors TS NpcOps.ts:424-428.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 12: `handleNpcFindNext` (opcode 2520) + 4-branch tests

**Files:**
- Modify: `pkg/script/handlers_npc.go`
- Modify: `pkg/script/handlers_npc_test.go`

- [ ] **Step 1: Write failing tests pinning all 4 termination branches**

Append to `pkg/script/handlers_npc_test.go`:

```go
// --- NAI-33 Task 12: NPC_FINDNEXT handler tests ------------------------

func newNpcFindNextState(t *testing.T, tick int, iter *NpcIterator) *ScriptState {
	t.Helper()
	s := &ScriptState{
		Script:      &ScriptFile{IntOperands: []int32{0}},
		PC:          0,
		Configs:     newTestConfigsWithNpcTypes(map[int]bool{}),
		World:       &mockWorldVars{tick: tick},
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	s.npcIterator = iter
	return s
}

func TestNpcFindNext_NilIterator(t *testing.T) {
	s := newNpcFindNextState(t, 100, nil)
	if err := handleNpcFindNext(s); err != nil {
		t.Fatalf("handleNpcFindNext: %v", err)
	}
	if got := s.PopInt(); got != 0 {
		t.Errorf("nil iterator: got push %d, want 0", got)
	}
	if s.ActiveNpc != nil {
		t.Error("ActiveNpc should not be set on nil iterator")
	}
}

func TestNpcFindNext_StaleIterator(t *testing.T) {
	npc := &mockNpc{typeID: 1, x: 3200, z: 3300, level: 0}
	zoneKey := mockZoneKey(0, 3200, 3296)
	lookup := &mockNpcLookup{byZone: map[uint64][]ActiveNpc{zoneKey: {npc}}}
	iter := NewZoneNpcIterator(lookup, /*creationTick=*/99, 0, 3200, 3300)
	s := newNpcFindNextState(t, /*currentTick=*/100, iter) // tick advanced

	err := handleNpcFindNext(s)
	if err == nil {
		t.Fatal("stale iterator should return error")
	}
	if !strings.Contains(err.Error(), "tried to use an old iterator") {
		t.Errorf("wrong error message: %v", err)
	}
}

func TestNpcFindNext_HitSetsActiveNpcAndPushes1(t *testing.T) {
	npc := &mockNpc{typeID: 1, x: 3200, z: 3300, level: 0}
	zoneKey := mockZoneKey(0, 3200, 3296)
	lookup := &mockNpcLookup{byZone: map[uint64][]ActiveNpc{zoneKey: {npc}}}
	iter := NewZoneNpcIterator(lookup, 100, 0, 3200, 3300)
	s := newNpcFindNextState(t, 100, iter)

	if err := handleNpcFindNext(s); err != nil {
		t.Fatalf("handleNpcFindNext: %v", err)
	}
	if got := s.PopInt(); got != 1 {
		t.Errorf("hit: got push %d, want 1", got)
	}
	if s.ActiveNpc != npc {
		t.Errorf("ActiveNpc: got %v, want %v", s.ActiveNpc, npc)
	}
	if s.Pointers&PtrActiveNpc == 0 {
		t.Error("PtrActiveNpc should be set on hit")
	}
}

func TestNpcFindNext_ExhaustionPushes0AndDoesNotClearIterator(t *testing.T) {
	// Empty zone — first Next exhausts immediately.
	lookup := &mockNpcLookup{}
	iter := NewZoneNpcIterator(lookup, 100, 0, 3200, 3300)
	s := newNpcFindNextState(t, 100, iter)

	if err := handleNpcFindNext(s); err != nil {
		t.Fatalf("handleNpcFindNext: %v", err)
	}
	if got := s.PopInt(); got != 0 {
		t.Errorf("exhaustion: got push %d, want 0", got)
	}
	// Critical TS-fidelity quirk: iterator NOT cleared on exhaustion
	// (matches TS state.npcIterator?.next() returning {done:true} without nulling).
	if s.npcIterator == nil {
		t.Error("npcIterator should NOT be cleared on exhaustion (TS parity)")
	}
}

func TestNpcFindNext_ExhaustionThenSecondCallStillPushes0(t *testing.T) {
	// Subsequent FINDNEXT calls on exhausted iterator continue to push 0.
	lookup := &mockNpcLookup{}
	iter := NewZoneNpcIterator(lookup, 100, 0, 3200, 3300)
	s := newNpcFindNextState(t, 100, iter)

	_ = handleNpcFindNext(s)
	_ = s.PopInt() // discard first

	if err := handleNpcFindNext(s); err != nil {
		t.Fatalf("second handleNpcFindNext: %v", err)
	}
	if got := s.PopInt(); got != 0 {
		t.Errorf("second exhaustion: got push %d, want 0", got)
	}
}
```

- [ ] **Step 2: Run tests to verify fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/... -run TestNpcFindNext -v`
Expected: COMPILE ERROR — `undefined: handleNpcFindNext`.

- [ ] **Step 3: Implement `handleNpcFindNext`**

Append to `pkg/script/handlers_npc.go`:

```go
// handleNpcFindNext (NPC_FINDNEXT, opcode 2520) advances the active
// NpcIterator and either sets active_npc + pushes 1 on hit, or pushes 0
// on miss / nil-iterator. Mirrors TS NpcOps.ts:430-441. Pointer-set is
// `require ['find_npc']`, `set ['active_npc']`, conditional
// (ScriptOpcodePointers.ts:595-600). Goscape encodes the require as a
// nil-check on s.npcIterator.
//
// Stale-iterator semantics: TS throws on stale (ScriptIterators.ts:332,343);
// goscape returns error → existing npc_script.go:169 log-warn +
// ClearActiveScript path runs. Single-tick lifetime preserved.
//
// Exhaustion does NOT clear s.npcIterator (matches TS
// state.npcIterator?.next() returning {done:true} without nulling).
// Subsequent FINDNEXT calls continue to return push-0.
func handleNpcFindNext(s *ScriptState) error {
	it := s.npcIterator
	if it == nil {
		s.PushInt(0)
		return nil
	}
	if it.Stale(s.World.CurrentTick()) {
		return fmt.Errorf("NPC_FINDNEXT: tried to use an old iterator. Create a new iterator instead.")
	}
	npc, ok := it.Next()
	if !ok {
		s.PushInt(0)
		return nil
	}
	setActiveNpcSlot(s, npc)
	s.PushInt(1)
	return nil
}
```

- [ ] **Step 4: Run tests to verify pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/... -run TestNpcFindNext -v`
Expected: 5 PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/script/handlers_npc.go pkg/script/handlers_npc_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): NAI-33 Task 12 — handleNpcFindNext (opcode 2520)

Consumer of *NpcIterator. 4 termination branches: nil iterator → push 0;
stale iterator → error (mirrors TS throw at ScriptIterators.ts:332,343);
hit → setActiveNpcSlot + push 1; exhaustion → push 0 + iterator NOT
cleared (TS-parity quirk). 5 tests pin each branch.

Mirrors TS NpcOps.ts:430-441. Pointer-set semantics encoded as
nil-check on s.npcIterator (the practical equivalent of
ScriptOpcodePointers.ts:595-600 require ['find_npc']).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 13: Register all 4 handlers in dispatch table

**Files:**
- Modify: `pkg/script/handlers.go` (extend FIND-cluster section)

- [ ] **Step 1: Locate existing FIND-cluster registration**

Pre-flight: confirmed at brainstorm — `pkg/script/handlers.go` contains:
```
	// NPC find (S7f) — closest-single cluster.
	OpNpcFind:      handleNpcFind,
	OpNpcFindCat:   handleNpcFindCat,
	OpNpcFindExact: handleNpcFindExact,
```

- [ ] **Step 2: Add 4 new entries**

Edit `pkg/script/handlers.go`. Locate the comment `// NPC find (S7f) — closest-single cluster.` and the 3 registration lines below it. Replace the comment + 3 lines with:

```go
	// NPC find (S7f) — closest-single cluster.
	OpNpcFind:      handleNpcFind,
	OpNpcFindCat:   handleNpcFindCat,
	OpNpcFindExact: handleNpcFindExact,

	// NPC find (NAI-33) — iterator family (DISTANCE + ZONE).
	OpNpcFindAll:     handleNpcFindAll,
	OpNpcFindAllAny:  handleNpcFindAllAny,
	OpNpcFindAllZone: handleNpcFindAllZone,
	OpNpcFindNext:    handleNpcFindNext,
```

- [ ] **Step 3: Verify build + run pkg/script test suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/...`
Expected: PASS — all existing + 17+ new tests across Tasks 3-12.

- [ ] **Step 4: Commit**

```bash
git add pkg/script/handlers.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): NAI-33 Task 13 — register iterator-family handlers in dispatch

Registers OpNpcFindAll/FindAllAny/FindAllZone/FindNext → their handlers
in the central dispatch table. Closes the runtime "no handler for
NPC_FINDALLANY (opcode 2515)" WARN for [proc,check_fishing_spot_empty]
once a server restart picks up the new handler table.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 14: Layer 4 — integration test (FINDALLANY → FINDNEXT loop end-to-end)

**Files:**
- Modify: `pkg/script/handlers_npc_test.go`

- [ ] **Step 1: Write integration test**

Append to `pkg/script/handlers_npc_test.go`:

```go
// --- NAI-33 Task 14: integration test (Layer 4) ------------------------

// TestIteratorFamily_Integration_FindAllAnyThenLoopFindNext exercises the
// end-to-end binding: FINDALLANY sets s.npcIterator; subsequent FINDNEXT
// calls visit-and-yield matching NPCs from the same iterator state.
// Mirrors the [proc,check_fishing_spot_empty] use pattern.
func TestIteratorFamily_Integration_FindAllAnyThenLoopFindNext(t *testing.T) {
	npc1 := &mockNpc{typeID: 1, x: 3200, z: 3300, level: 0}
	npc2 := &mockNpc{typeID: 2, x: 3201, z: 3300, level: 0}
	zoneKey := mockZoneKey(0, 3200, 3296)
	lookup := &mockNpcLookup{byZone: map[uint64][]ActiveNpc{zoneKey: {npc1, npc2}}}

	s := &ScriptState{
		Script:      &ScriptFile{IntOperands: []int32{0}},
		PC:          0,
		Configs:     newTestConfigsWithNpcTypes(map[int]bool{}),
		Npcs:        lookup,
		World:       &mockWorldVars{tick: 100},
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}

	// Stage 1: FINDALLANY (3 args: coord, distance, huntvis)
	coord := (0 << 28) | (3200 << 14) | 3300
	s.PushInt(coord)
	s.PushInt(5)
	s.PushInt(0)
	if err := handleNpcFindAllAny(s); err != nil {
		t.Fatalf("FINDALLANY: %v", err)
	}
	if s.npcIterator == nil {
		t.Fatal("FINDALLANY did not set iterator")
	}

	// Stage 2: FINDNEXT loop
	yielded := []ActiveNpc{}
	for i := 0; i < 5; i++ { // bounded loop
		if err := handleNpcFindNext(s); err != nil {
			t.Fatalf("FINDNEXT iter %d: %v", i, err)
		}
		got := s.PopInt()
		if got == 0 {
			break
		}
		yielded = append(yielded, s.ActiveNpc)
	}

	if len(yielded) != 2 {
		t.Errorf("yielded count: got %d, want 2 (npc1, npc2)", len(yielded))
	}
	// Iterator persists across FINDNEXT calls (TS-parity).
	if s.npcIterator == nil {
		t.Error("iterator should persist after exhaustion")
	}
}
```

- [ ] **Step 2: Run test**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/... -run TestIteratorFamily_Integration -v`
Expected: PASS.

- [ ] **Step 3: Run full pkg/script suite + race detector**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./pkg/script/...`
Expected: PASS.

- [ ] **Step 4: Run full project test suite + race detector**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/script/handlers_npc_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
test(script): NAI-33 Task 14 — Layer 4 integration test

Exercises the full FINDALLANY → FINDNEXT loop binding through the
state.npcIterator field — the cross-handler-dispatch path that single
unit tests can't catch. Mirrors the [proc,check_fishing_spot_empty]
proc's use pattern. Verifies iterator persistence across FINDNEXT
calls (TS-parity quirk).

All 14 tasks complete. Iterator family fully wired; smoke gate next.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 15: Smoke handoff (binding feature-correctness gate)

**Files:** none (user-driven)

- [ ] **Step 1: Hand the smoke procedure to the user per `smoke_test_server_handoff.md`**

Output to user:

```
NAI-33 implementation complete (Tasks 1-14, 14 commits on main).
Smoke gate per smoke_test_server_handoff.md:

1. Restart the dev server:
   CGO_ENABLED=0 GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go run -trimpath ./cmd/goscape --config.file config.yaml

2. Trigger a fishing-NPC AI timer:
   - Default spawn (3094, 3106) is on Tutorial Island; fishing NPCs aren't there.
   - Either: log in, walk to a fishing-eligible location (e.g. Lumbridge swamp
     fishing spots near 3242, 3151), and idle near a fishing NPC.
   - Or: use a debug ::move command if available.
   - Wait at least one full npc_settimer interval (~3-9 minutes per
     calc(280 + random(250))).

3. Confirm in server log:
   ✓ No occurrence of "no handler for NPC_FINDALLANY (opcode 2515)"
   ✓ No occurrence of "no handler for NPC_FINDNEXT (opcode 2520)"
   ✓ No occurrence of "tried to use an old iterator"

4. Confirm visually:
   ✓ Fishing NPCs (any of category freshfish/saltfish/rarefish/memberfish, or
     0_45_152_lavafish) visibly relocate between fishing spots over time.

If smoke fails: open Bundle 2 (conditional investigation per
investigation_subspec_cadence.md). Common layered-bug suspects:
- Dispatch-table miss (re-grep `OpNpcFindAllAny:` in pkg/script/handlers.go)
- Type-assertion panic in serverNpcLookup.ZoneNpcs (n.(ActiveNpc))
- Missing pre-flight item — re-check the 11 items in spec § Plan-author pre-flight checklist
```

- [ ] **Step 2: Wait for user smoke verdict**

If GO → proceed to Task 16 (close commit).
If NO-GO → open Bundle 2 conditional investigation per `investigation_subspec_cadence.md`. Stage 1 audit first; do not jump to Stage 2 fix without an independent root-cause verification per `audit_subagent_fabrication.md`.

---

## Task 16: Close commit + memory entry (post-smoke success)

**Files:** memory entry only (no code)

- [ ] **Step 1: Add memory entry on the iterator-state pattern**

Per `close_commit_memory_trailer.md`. The non-derivable insight from this sub-spec is the **iterator-state-as-package-private-field pattern**: how to plumb a stateful, single-tick iterator across the script-VM ↔ world boundary without entangling termination paths. This is the template for the LOC iterator family (`OpLocFindAllZone=3008` and siblings) and any future *Iterator opcodes (Obj, Player, etc.).

Use the `Write` tool (not Bash printf — per `memory_write_sandbox_quirk.md`) to create:

`/home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/iterator_state_pattern.md`:

```markdown
---
name: Stateful single-tick iterator template (NPC_FIND family)
description: Pattern for porting iterator-family script opcodes (FINDALL+FINDNEXT shape) — package-private *T field on ScriptState, lazy per-zone snapshot, Stale(currentTick) check at NEXT, no termination-path cleanup needed
type: feedback
---

When porting a script-opcode family of the shape "SETUP-iterator-then-loop-NEXT" (NPC_FINDALL/FINDNEXT, future LOC_FINDALL/FINDNEXT, etc.), use this template:

1. **State field**: package-private `*FooIterator` on `ScriptState`. Lowercase = same-package handler access without exporting.
2. **Constructor + lazy fetch**: setup handler builds iterator with center coord + filters; iterator holds back-pointer to a Lookup interface (e.g., NpcLookup). Per-zone snapshot is fetched on cursor advance, not up-front.
3. **Single-tick lifetime**: iterator holds `creationTick`; NEXT handler checks `Stale(s.World.CurrentTick())` BEFORE calling Next. Mirrors TS throw-on-stale via goscape's existing handler-error → log-warn path.
4. **No new termination cleanup**: nil-or-non-nil semantics, GC handles Aborted/Finished, NpcSuspended-resume catches stale via Stale(). Avoid the iter.Pull approach — its goroutine lifecycle imposes a stop() invariant on every termination path.
5. **Nil-Lookup degradation**: setup handler skips iterator creation when `s.Npcs == nil` (mirrors existing FIND handler convention); NEXT handler's nil-iterator branch then pushes 0 naturally.
6. **Pop order = TS popInts(N) reverse**: top of stack first. Pin via tests with distinguishable values.
7. **Exhaustion does NOT clear iterator**: matches TS `state.iterator?.next()` returning `{done:true}` without nulling. Subsequent NEXT calls continue to push 0.

**Why:** NAI-33 ported the NPC iterator family using this template. Approach C (custom struct) chosen over A (eager snapshot) and B (iter.Pull) precisely because it generalizes cleanly without imposing new VM-level invariants. Counter-evidence considered: B (iter.Pull) is more idiomatic Go but introduces a hidden goroutine that must be stopped on every script abort/clear/finish path — a high-coupling invariant the script VM's multiple termination paths would silently violate.

**How to apply:**
- LOC iterator family (`OpLocFindAllZone=3008` and siblings, currently stubbed in `pkg/script/opcode.go`): copy this template. New `LocIterator` struct in `pkg/script/loc_iterator.go`; new `LocLookup.ZoneLocs` method; new `state.locIterator *LocIterator` field; mirror handler-pop-order + stale-check pattern.
- Any future *Iterator port: same template. The NAI-33 NpcIterator is the reference impl.
- Tests: 4-layer suite (iterator mechanics, handlers, world-side primitive, integration). Layer 4 catches "handler runs but state field isn't visible across opcode dispatch" failures that single-handler unit tests miss.
```

Then add a line to `MEMORY.md`:

```
- [Iterator-state pattern](iterator_state_pattern.md) — single-tick iterator template (NpcIterator + state field + Lookup.ZoneFoo); reusable for LOC iterator family
```

- [ ] **Step 2: Close commit**

```bash
git commit --no-gpg-sign --allow-empty -m "$(cat <<'EOF'
chore(script,world): NAI-33 closed — NPC iterator family ported (FINDALL/FINDALLANY/FINDALLZONE/FINDNEXT)

Closes runtime WARN `no handler for NPC_FINDALLANY (opcode 2515)` triggered
by [proc,check_fishing_spot_empty] in skill_fishing/scripts/fishing_movement.rs2.
All 4 iterator-family opcodes (2514, 2515, 2516, 2520) now wired:

- pkg/script/npc_iterator.go (NEW): NpcIterator + NpcIteratorMode + constructors
  + Stale + Next + advanceZone + passesFilter
- pkg/script/state.go: ZoneNpcs added to NpcLookup; npcIterator field on ScriptState
- pkg/script/handlers_npc.go: handleNpcFindAll/FindAllAny/FindAllZone/FindNext
- pkg/script/handlers.go: 4 dispatch entries
- modules/world/npc_script_lookup.go: serverNpcLookup.ZoneNpcs
- 4-layer test suite: iterator mechanics (7 tests) + handlers (17 tests) + world (4 tests) + integration (1 test)

Smoke verified: fishing NPCs visibly relocate between spots; no opcode-not-found WARNs.

Net deviation count: 14 → 14. NAI-33-D1 carries existing S7f-D1 huntvis-not-filtered
posture across the iterator family; retires when LoS/LoW filtering is wired across
the entire FIND* family in one sweep.

Establishes the iterator-state template (memory: iterator_state_pattern.md) for the
parallel-shaped LOC iterator family port.

Closes memory: iterator_state_pattern.md

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

- [ ] **Step 3: Hand back to user**

Output: NAI-33 closed at `<close-commit-SHA>`. Iterator family ported, smoke green, deviation count unchanged at 14, new memory entry `iterator_state_pattern.md` available for the LOC iterator family port (likely NAI-34 or later candidate).

---

## Self-Review Notes (post-write)

**Spec coverage check:** every spec surface mapped to a task:
- Spec § Architecture three-layer addition → Tasks 1, 2, 3-7, 8, 9-12, 13.
- Spec § NpcIterator data model → Tasks 3-7.
- Spec § Handler dispatch (4 handlers) → Tasks 9, 10, 11, 12.
- Spec § ZoneNpcs world-side impl → Tasks 1, 2.
- Spec § ScriptState integration → Task 8.
- Spec § Testing 4 layers → Layer 1 (Tasks 4-7), Layer 2 (Tasks 9-12), Layer 3 (Task 2), Layer 4 (Task 14).
- Spec § Smoke gate → Task 15.
- Spec § Plan-author pre-flight checklist 11 items → all run pre-plan-write; results inlined in Tasks 1-13.

**Type-consistency check:**
- `NewDistanceNpcIterator(lookup, tick, level, x, z, distance, huntvis, typeID int)` signature: 8 args. Used in Task 4 test (8 args), Task 9 handler (8 args), Task 10 handler (8 args), Task 7 tests (8 args). Consistent.
- `NewZoneNpcIterator(lookup, tick, level, x, z int)` signature: 5 args. Used in Task 5 test (5 args), Task 11 handler (5 args), Task 6/12 tests (5 args). Consistent.
- `NpcLookup.ZoneNpcs(level, zoneX, zoneZ int) []ActiveNpc`: declared Task 1, mock impl Task 1, prod impl Task 2, consumed in iterator (Task 6) + integration test (Task 14). Consistent.
- `mockZoneKey(level, zoneX, zoneZ int) uint64`: defined Task 1, used in Tasks 6, 7, 12, 14. Consistent.

**Placeholder scan:** zero "TBD"/"TODO"/"implement later" in plan. Two `Plan-author note` annotations contain pre-flight verification reminders, NOT placeholders — each names the exact grep target the implementer should run.

**Step granularity:** every step is 2-5 min. Tests show concrete code; commands show exact runtime; commits show exact message HEREDOCs.
