# NAI-9 huntNpcs/huntObjs/huntLocs Variant Fill — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fill the three NAI-7 variant stubs (`huntNpcs`, `huntObjs`, `huntLocs`) with zone/grid iteration + type/category/distance filters, and replace NAI-7's `observers := 1` stub with a real nid-keyed observer counter in `pkg/rsbuf` that mirrors `@2004scape/rsbuf`'s public API.

**Architecture:** Ten tasks in dependency order. Prerequisites first (Obj entity interface, ZoneMap helper). Observer-counter infrastructure in `pkg/rsbuf` next (new file mirroring `@2004scape/rsbuf` shape). Then wire the counter through `EncodeNpc` and `processLogouts`. Then replace the hunt-gate stub and flip the PAUSEHUNT test. Finally the three variant bodies (one task each, structurally similar). Close with memory-only housekeeping.

**Tech Stack:** Go 1.26+, existing `pkg/grid.NearbyNpcs`, existing `pkg/zone.ZoneMap`, existing `pkg/rsbuf.NpcSource` interface (no extension in this plan — observer counter is rsbuf-owned state, not interface-extended).

**Spec:** `docs/superpowers/specs/2026-04-22-nai-9-hunt-variant-fill-design.md`

**Roadmap:** `docs/superpowers/specs/2026-04-22-npc-ai-tick-decomposition-design.md`

**TS reference:** `/home/owner/Code/github.com/LostCityRS/Engine-TS/src/engine/entity/Npc.ts:975-985` (variant bodies), `/home/owner/Code/github.com/LostCityRS/Engine-TS/src/engine/script/ScriptIterators.ts:98-167` (HuntIterator), `/home/owner/Code/github.com/LostCityRS/Engine-TS/node_modules/@2004scape/rsbuf/dist/rsbuf.d.ts:6,13` (rsbuf public API).

---

## File Structure

**New files:**
- `modules/world/npc_hunt_entities.go` — three variant bodies (`huntNpcs`, `huntObjs`, `huntLocs`)
- `modules/world/npc_hunt_entities_test.go` — per-variant unit tests + shared fixtures
- `pkg/rsbuf/npc_observers.go` — observer counter + public API (`GetNpcObservers`, `RemovePlayer`, `SetObserverForTest`)
- `pkg/rsbuf/npc_observers_test.go` — counter unit tests

**Modified:**
- `pkg/entity/obj.go` — add `Slot()` + `Coords()` (entity-interface satisfaction)
- `pkg/entity/obj_test.go` — satisfaction test
- `pkg/zone/map.go` — add `NearbyZones` helper
- `pkg/zone/map_test.go` — helper tests
- `pkg/rsbuf/npcinfo.go` — 3 hook sites (inc on add, dec on two removes)
- `pkg/rsbuf/npcinfo_test.go` — 3 integration tests
- `modules/world/npc.go` — add import of `pkg/rsbuf` if not present (for DropPlayerNpcSubscriptions caller)
- `modules/world/tick.go` — `processLogouts` hook
- `modules/world/npc_hunt.go` — delete 3 stubs, replace `observers := 1`
- `modules/world/npc_event_queue_test.go` — flip PAUSEHUNT test + companion + logout test
- `/home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md` — 3 edits

---

## Task 1: `*entity.Obj` entity-interface satisfaction

**Files:**
- Modify: `pkg/entity/obj.go`
- Modify: `pkg/entity/obj_test.go`

Enables `*Obj` to be assigned to `n.huntTarget` (which is typed as the `entity` interface in `modules/world/movement_consts.go:45`). Direct port of the pattern already established on `*entity.Loc` at `pkg/entity/loc.go:44,49`.

- [ ] **Step 1: Write the failing test**

Append to `pkg/entity/obj_test.go`:

```go
// TestObjSatisfiesEntityInterface locks in the Slot() + Coords()
// methods required for *Obj to be used as a huntTarget in
// modules/world. The interface assertion is compile-time; the test
// itself just confirms values.
func TestObjSatisfiesEntityInterface(t *testing.T) {
	type entityLike interface {
		Slot() int
		Coords() (x, z, level int)
	}
	var _ entityLike = (*Obj)(nil) // compile-time assertion

	o := NewObj(2, 3094, 3106, LifecycleDespawn, 995, 100)
	if o.Slot() != -1 {
		t.Errorf("Slot: got %d, want -1 (objs are not slot-indexed)", o.Slot())
	}
	x, z, level := o.Coords()
	if x != 3094 || z != 3106 || level != 2 {
		t.Errorf("Coords: got (%d,%d,%d), want (3094,3106,2)", x, z, level)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/entity/ -run TestObjSatisfiesEntityInterface -v`

Expected: FAIL with compile error `(*Obj)(nil) does not implement entityLike (missing method Slot)` or similar.

- [ ] **Step 3: Add the methods**

Append to `pkg/entity/obj.go`:

```go
// Slot returns -1 because objs are not slot-indexed (unlike Players
// and Npcs which live in server-wide slot registries). Mirrors
// *entity.Loc.Slot. Required for the world.entity interface so
// objs can be assigned to Npc.huntTarget.
func (o *Obj) Slot() int { return -1 }

// Coords returns the obj's tile position. Reads X/Z/Level from the
// embedded entity.Entity (see entity.go:6-12). Required for the
// world.entity interface.
func (o *Obj) Coords() (x, z, level int) {
	return o.X, o.Z, o.Level
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/entity/ -run TestObjSatisfiesEntityInterface -v`

Expected: PASS

- [ ] **Step 5: Run full pkg/entity test suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/entity/ -v`

Expected: all tests pass (no regression).

- [ ] **Step 6: Commit**

```bash
git add pkg/entity/obj.go pkg/entity/obj_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(entity): NAI-9 Obj satisfies world.entity interface

Prereq for assigning *Obj to Npc.huntTarget in NAI-9's huntObjs
variant. Mirrors the Slot()/Coords() pattern already on *Loc.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: `ZoneMap.NearbyZones` helper

**Files:**
- Modify: `pkg/zone/map.go`
- Modify: `pkg/zone/map_test.go`

Enables `huntObjs` and `huntLocs` to iterate zone buckets in a zoneRadius without inline zone-coord arithmetic at every hunt call site.

- [ ] **Step 1: Write the failing tests**

Append to `pkg/zone/map_test.go`:

```go
func TestNearbyZonesRadius0ReturnsCenter(t *testing.T) {
	m := NewZoneMap()
	center := m.Get(0, 3094, 3106) // materialise center zone
	zones := m.NearbyZones(0, 3094, 3106, 0)
	if len(zones) != 1 || zones[0] != center {
		t.Errorf("NearbyZones radius 0: got %d zones, want [center]", len(zones))
	}
}

func TestNearbyZonesRadius1ReturnsUpTo9(t *testing.T) {
	m := NewZoneMap()
	// Materialise 3x3 around (3094, 3106) level 0.
	for dx := -1; dx <= 1; dx++ {
		for dz := -1; dz <= 1; dz++ {
			m.Get(0, 3094+dx*8, 3106+dz*8)
		}
	}
	zones := m.NearbyZones(0, 3094, 3106, 1)
	if len(zones) != 9 {
		t.Errorf("NearbyZones radius 1: got %d zones, want 9", len(zones))
	}
}

func TestNearbyZonesSkipsUnmaterialisedZones(t *testing.T) {
	m := NewZoneMap()
	center := m.Get(0, 3094, 3106)
	east := m.Get(0, 3094+8, 3106) // one east, zoneRadius 1 neighbour
	// The other 7 neighbours are NOT materialised.
	zones := m.NearbyZones(0, 3094, 3106, 1)
	if len(zones) != 2 {
		t.Errorf("NearbyZones: got %d zones, want 2 (only materialised)", len(zones))
	}
	// Order is deterministic (dx outer, dz inner, both ascending).
	have := map[*Zone]bool{zones[0]: true, zones[1]: true}
	if !have[center] || !have[east] {
		t.Errorf("NearbyZones: missing center or east from result")
	}
}

func TestNearbyZonesLevelFilter(t *testing.T) {
	m := NewZoneMap()
	z0 := m.Get(0, 3094, 3106)
	z1 := m.Get(1, 3094, 3106)
	level0 := m.NearbyZones(0, 3094, 3106, 0)
	level1 := m.NearbyZones(1, 3094, 3106, 0)
	if len(level0) != 1 || level0[0] != z0 {
		t.Errorf("NearbyZones level 0: got %v, want [z0]", level0)
	}
	if len(level1) != 1 || level1[0] != z1 {
		t.Errorf("NearbyZones level 1: got %v, want [z1]", level1)
	}
}

func TestNearbyZonesClampsNegativeCoords(t *testing.T) {
	m := NewZoneMap()
	m.Get(0, 0, 0) // materialise origin
	// Center at (0,0) radius 1 would naively probe (-1, 0..1) etc;
	// helper must skip negatives to avoid malformed zone indexes.
	zones := m.NearbyZones(0, 0, 0, 1)
	// Only materialised neighbour is the origin itself.
	if len(zones) != 1 {
		t.Errorf("NearbyZones near origin: got %d zones, want 1 (just origin)", len(zones))
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/zone/ -run NearbyZones -v`

Expected: FAIL with "m.NearbyZones undefined".

- [ ] **Step 3: Add the helper**

Append to `pkg/zone/map.go` (after `ZoneCount`):

```go
// NearbyZones returns all materialised zones whose (zoneX, zoneZ) is
// within zoneRadius Chebyshev distance of the zone containing
// (x, z) at the given level. Unmaterialised zones are skipped —
// callers don't need to nil-check entries.
//
// Iteration order is dx ascending (outer) then dz ascending (inner),
// matching the grid.NearbyPlayers/NearbyNpcs convention (not the TS
// HuntIterator's descending order — order is distribution-neutral
// for the random picker in huntAll; logged as deviation D1 in
// 2026-04-22-nai-9-hunt-variant-fill-design.md).
//
// Used by modules/world/npc_hunt_entities.go for huntObjs/huntLocs
// zone-iteration.
func (m *ZoneMap) NearbyZones(level, x, z, zoneRadius int) []*Zone {
	zoneX := x >> 3
	zoneZ := z >> 3
	out := make([]*Zone, 0, (2*zoneRadius+1)*(2*zoneRadius+1))
	for dx := -zoneRadius; dx <= zoneRadius; dx++ {
		for dz := -zoneRadius; dz <= zoneRadius; dz++ {
			zx := zoneX + dx
			zz := zoneZ + dz
			if zx < 0 || zz < 0 {
				continue
			}
			idx := coordgrid.ZoneIndex(zx<<3, zz<<3, level)
			if z, ok := m.zones[idx]; ok {
				out = append(out, z)
			}
		}
	}
	return out
}
```

Note: if `coordgrid` is not already imported in `map.go`, add it. Check the existing import block and add `"github.com/zsrv/goscape/pkg/coordgrid"` if missing (it's already used elsewhere in `pkg/zone`).

- [ ] **Step 4: Run tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/zone/ -run NearbyZones -v`

Expected: all 5 tests PASS.

- [ ] **Step 5: Run full pkg/zone suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/zone/ -v`

Expected: all tests pass.

- [ ] **Step 6: Commit**

```bash
git add pkg/zone/map.go pkg/zone/map_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(zone): NAI-9 ZoneMap.NearbyZones helper

Enables zone-bucket iteration across a zoneRadius Chebyshev
neighbourhood. Skips unmaterialised zones. Used by NAI-9's huntObjs
and huntLocs variants.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: `pkg/rsbuf/npc_observers.go` — counter core

**Files:**
- Create: `pkg/rsbuf/npc_observers.go`
- Create: `pkg/rsbuf/npc_observers_test.go`

Observer-count storage + the 4 public API functions: `GetNpcObservers(nid)`, `RemovePlayer(pid, subscribedNpcs)`, `SetObserverForTest(nid, count)`, `cleanupForTest()`. Mirrors `@2004scape/rsbuf`'s public API at `rsbuf.d.ts:6,13`.

- [ ] **Step 1: Write the failing tests**

Create `pkg/rsbuf/npc_observers_test.go`:

```go
package rsbuf

import "testing"

// resetObserversForTest clears observer state between tests. All
// tests in this file must call this at the start.
func resetObserversForTest() {
	clear(npcObservers)
}

func TestGetNpcObserversDefaultZero(t *testing.T) {
	resetObserversForTest()
	if got := GetNpcObservers(42); got != 0 {
		t.Errorf("GetNpcObservers(42) fresh: got %d, want 0", got)
	}
}

func TestIncNpcObserverIncrements(t *testing.T) {
	resetObserversForTest()
	incNpcObserver(7)
	incNpcObserver(7)
	if got := GetNpcObservers(7); got != 2 {
		t.Errorf("GetNpcObservers(7) after 2 inc: got %d, want 2", got)
	}
}

func TestDecNpcObserverFloorsAtZero(t *testing.T) {
	resetObserversForTest()
	decNpcObserver(9) // dec with no prior inc
	if got := GetNpcObservers(9); got != 0 {
		t.Errorf("GetNpcObservers(9) after dec-from-zero: got %d, want 0", got)
	}
}

func TestDecNpcObserverDecrementsPositive(t *testing.T) {
	resetObserversForTest()
	incNpcObserver(3)
	incNpcObserver(3)
	decNpcObserver(3)
	if got := GetNpcObservers(3); got != 1 {
		t.Errorf("GetNpcObservers(3) after inc+inc+dec: got %d, want 1", got)
	}
}

func TestRemovePlayerDecrementsEachSubscribedNid(t *testing.T) {
	resetObserversForTest()
	incNpcObserver(10)
	incNpcObserver(10)
	incNpcObserver(20)
	subs := map[int]struct{}{10: {}, 20: {}}
	RemovePlayer(1, subs)
	if got := GetNpcObservers(10); got != 1 {
		t.Errorf("GetNpcObservers(10) after RemovePlayer: got %d, want 1", got)
	}
	if got := GetNpcObservers(20); got != 0 {
		t.Errorf("GetNpcObservers(20) after RemovePlayer: got %d, want 0", got)
	}
}

func TestRemovePlayerEmptySetIsNoOp(t *testing.T) {
	resetObserversForTest()
	incNpcObserver(5)
	RemovePlayer(1, map[int]struct{}{})
	if got := GetNpcObservers(5); got != 1 {
		t.Errorf("GetNpcObservers(5) after empty RemovePlayer: got %d, want 1", got)
	}
}

func TestRemovePlayerNilSetIsNoOp(t *testing.T) {
	resetObserversForTest()
	incNpcObserver(6)
	RemovePlayer(1, nil) // must not panic
	if got := GetNpcObservers(6); got != 1 {
		t.Errorf("GetNpcObservers(6) after nil RemovePlayer: got %d, want 1", got)
	}
}

func TestSetObserverForTestOverridesCount(t *testing.T) {
	resetObserversForTest()
	SetObserverForTest(42, 5)
	if got := GetNpcObservers(42); got != 5 {
		t.Errorf("GetNpcObservers(42) after SetObserverForTest(5): got %d, want 5", got)
	}
	SetObserverForTest(42, 0)
	if got := GetNpcObservers(42); got != 0 {
		t.Errorf("GetNpcObservers(42) after SetObserverForTest(0): got %d, want 0", got)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/rsbuf/ -run Observer -v`

Expected: FAIL with "undefined: npcObservers" / "undefined: GetNpcObservers" / etc.

- [ ] **Step 3: Create the observer counter source file**

Create `pkg/rsbuf/npc_observers.go`:

```go
package rsbuf

// npcObservers counts, per-NPC, the number of players currently
// subscribed to that NPC via NpcInfo. Mirrors the @2004scape/rsbuf
// WASM's internal state backing the getNpcObservers(nid) public API
// (see Engine-TS/node_modules/@2004scape/rsbuf/dist/rsbuf.d.ts:13).
//
// Maintained at three sites in npcinfo.go's EncodeNpc:
//   - incNpcObserver on subscription-add (line ~108)
//   - decNpcObserver on subscription-remove (inactive-path, ~line 39)
//   - decNpcObserver on subscription-remove (out-of-range, ~line 46)
// And in bulk via RemovePlayer on player logout.
//
// Read by consumers via GetNpcObservers — currently called from
// modules/world/npc_hunt.go's PAUSEHUNT gate.
var npcObservers = map[int]int{}

// GetNpcObservers returns the number of players currently subscribed
// to this NPC via NpcInfo. Returns 0 for any nid never observed or
// whose count floored at zero. Public API; mirrors
// @2004scape/rsbuf's getNpcObservers(nid) at rsbuf.d.ts:13.
func GetNpcObservers(nid int) int { return npcObservers[nid] }

// RemovePlayer performs bulk-decrement of observer counts for every
// NPC in the player's subscription set. Mirrors @2004scape/rsbuf's
// removePlayer(pid) at rsbuf.d.ts:6, whose WASM internals iterate
// the player's build.npcs set and decrement each NPC's observer
// count.
//
// Caller supplies the subscription set because goscape's pkg/rsbuf
// doesn't own per-player BuildArea state (see deviation D5 in
// the NAI-9 design spec). pid is unused in the goscape
// implementation but kept for API-shape parity with TS.
//
// Safe to call with a nil or empty set.
func RemovePlayer(pid int, subscribedNpcs map[int]struct{}) {
	for nid := range subscribedNpcs {
		decNpcObserver(nid)
	}
}

// SetObserverForTest is a test-only helper that directly writes an
// observer count. Used by tests in modules/world that need to seed
// a specific count (e.g., PAUSEHUNT gate tests) without going
// through the full EncodeNpc pipeline.
//
// NOT for production use. Present on the public API surface only
// because cross-package tests in modules/world need to reach it.
func SetObserverForTest(nid, count int) {
	if count <= 0 {
		delete(npcObservers, nid)
		return
	}
	npcObservers[nid] = count
}

// incNpcObserver increments nid's observer count. Unexported;
// called only from EncodeNpc.
func incNpcObserver(nid int) { npcObservers[nid]++ }

// decNpcObserver decrements nid's observer count, flooring at 0.
// Matches @2004scape/rsbuf semantics (Math.max(x - 1, 0) observable
// from the TS shim). Unexported; called from EncodeNpc remove sites
// and RemovePlayer.
func decNpcObserver(nid int) {
	if v := npcObservers[nid]; v > 0 {
		npcObservers[nid] = v - 1
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/rsbuf/ -run Observer -v`

Expected: all 8 tests PASS.

- [ ] **Step 5: Run full pkg/rsbuf suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/rsbuf/ -v`

Expected: all tests pass (no regression in existing rsbuf tests).

- [ ] **Step 6: Commit**

```bash
git add pkg/rsbuf/npc_observers.go pkg/rsbuf/npc_observers_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(rsbuf): NAI-9 NPC observer counter core

Mirrors @2004scape/rsbuf's GetNpcObservers + RemovePlayer public API
shape. Package-level nid-keyed map with inc/dec/floor-at-zero
semantics. SetObserverForTest helper for cross-package test seeding.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: Wire observer counter into `EncodeNpc` hook sites

**Files:**
- Modify: `pkg/rsbuf/npcinfo.go`
- Modify: `pkg/rsbuf/npcinfo_test.go`

Three one-line additions to `EncodeNpc`: inc on subscription-add, dec on two subscription-remove paths. Plus three integration tests that assert the counter moves correctly through the real encode path.

- [ ] **Step 1: Write the failing tests**

Append to `pkg/rsbuf/npcinfo_test.go`:

```go
func TestEncodeNpcAddIncrementsObservers(t *testing.T) {
	resetObserversForTest()
	// Build a minimal scene: one player, one nearby active NPC,
	// empty subscription. EncodeNpc should emit an add and the
	// observer counter for that nid should tick from 0 to 1.
	self := makeTestPlayerSource(1, 3094, 3106, 0)
	npc := makeTestNpcSource(100, 1, 3094+2, 3106+2, 0, true)
	g := grid.New()
	g.AddNpc(npc.Nid(), npc.X(), npc.Z(), npc.Level())
	ba := buildarea.New()
	r := NewRenderer()

	EncodeNpc(self, []NpcSource{npc}, ba, g, r)

	if got := GetNpcObservers(npc.Nid()); got != 1 {
		t.Errorf("GetNpcObservers(%d) after add: got %d, want 1", npc.Nid(), got)
	}
}

func TestEncodeNpcRemoveOnInactiveDecrementsObservers(t *testing.T) {
	resetObserversForTest()
	// Pre-seed: NPC already subscribed + observer count = 1.
	// EncodeNpc sees Active() == false and removes it — counter
	// must decrement to 0.
	self := makeTestPlayerSource(1, 3094, 3106, 0)
	npc := makeTestNpcSource(200, 1, 3094+2, 3106+2, 0, false /* inactive */)
	g := grid.New()
	g.AddNpc(npc.Nid(), npc.X(), npc.Z(), npc.Level())
	ba := buildarea.New()
	ba.Npcs[npc.Nid()] = struct{}{}
	SetObserverForTest(npc.Nid(), 1)
	r := NewRenderer()

	EncodeNpc(self, []NpcSource{npc}, ba, g, r)

	if got := GetNpcObservers(npc.Nid()); got != 0 {
		t.Errorf("GetNpcObservers(%d) after inactive-remove: got %d, want 0", npc.Nid(), got)
	}
}

func TestEncodeNpcRemoveOnOutOfRangeDecrementsObservers(t *testing.T) {
	resetObserversForTest()
	// Pre-seed: NPC subscribed + count = 1. This tick, move the
	// NPC far enough that zoneDist > NpcViewDistanceZones. EncodeNpc
	// should remove + decrement.
	self := makeTestPlayerSource(1, 3094, 3106, 0)
	// NpcViewDistanceZones = 15; put NPC at (3094 + 16*8, ...) so
	// zone-distance is 16 > 15.
	npc := makeTestNpcSource(300, 1, 3094+16*8, 3106, 0, true)
	g := grid.New()
	g.AddNpc(npc.Nid(), npc.X(), npc.Z(), npc.Level())
	ba := buildarea.New()
	ba.Npcs[npc.Nid()] = struct{}{}
	SetObserverForTest(npc.Nid(), 1)
	r := NewRenderer()

	EncodeNpc(self, []NpcSource{npc}, ba, g, r)

	if got := GetNpcObservers(npc.Nid()); got != 0 {
		t.Errorf("GetNpcObservers(%d) after out-of-range-remove: got %d, want 0", npc.Nid(), got)
	}
}
```

**Fixture note:** `makeTestPlayerSource` and `makeTestNpcSource` already exist in `pkg/rsbuf/npcinfo_test.go` (used by existing `TestEncodeNpcAddsNew` etc.). Inspect the file before writing the tests to confirm the exact helper signatures and adjust. If the helpers don't produce what the tests need (e.g., need control over `Active()` return), extend them minimally.

- [ ] **Step 2: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/rsbuf/ -run "TestEncodeNpc(Add|Remove).*Observers" -v`

Expected: FAIL with "got 0, want 1" or similar — EncodeNpc doesn't touch the counter yet.

- [ ] **Step 3: Wire the three hook sites in EncodeNpc**

Open `pkg/rsbuf/npcinfo.go`. Locate three sites:

**Site A — line ~39, inside the `!ok || !n.Active()` branch.** Before `delete(ba.Npcs, nid)`:

Current:
```go
		if !ok || !n.Active() {
			main.PBit(1, 1)
			main.PBit(2, 3) // remove
			delete(ba.Npcs, nid)
			continue
		}
```

After:
```go
		if !ok || !n.Active() {
			main.PBit(1, 1)
			main.PBit(2, 3) // remove
			decNpcObserver(nid)
			delete(ba.Npcs, nid)
			continue
		}
```

**Site B — line ~46, inside the level-mismatch / zone-distance-too-far branch.** Before `delete(ba.Npcs, nid)`:

Current:
```go
		if nl != selfLevel || zoneDist(selfX, selfZ, nx, nz) > NpcViewDistanceZones {
			main.PBit(1, 1)
			main.PBit(2, 3)
			delete(ba.Npcs, nid)
			continue
		}
```

After:
```go
		if nl != selfLevel || zoneDist(selfX, selfZ, nx, nz) > NpcViewDistanceZones {
			main.PBit(1, 1)
			main.PBit(2, 3)
			decNpcObserver(nid)
			delete(ba.Npcs, nid)
			continue
		}
```

**Site C — line ~108, inside the new-NPC phase-2 loop.** After `ba.Npcs[nid] = struct{}{}`:

Current:
```go
		ba.Npcs[nid] = struct{}{}
		if len(payload) > 0 {
			for _, b := range payload {
				updates.P1(b)
			}
		}
```

After:
```go
		ba.Npcs[nid] = struct{}{}
		incNpcObserver(nid)
		if len(payload) > 0 {
			for _, b := range payload {
				updates.P1(b)
			}
		}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/rsbuf/ -run "TestEncodeNpc(Add|Remove).*Observers" -v`

Expected: all 3 tests PASS.

- [ ] **Step 5: Run full pkg/rsbuf suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/rsbuf/ -v`

Expected: all tests pass. If an existing `EncodeNpc` test fails because it doesn't reset observer state, add `resetObserversForTest()` at the top. The NpcInfo existing tests may need this; inspect failures and fix in-place.

- [ ] **Step 6: Commit**

```bash
git add pkg/rsbuf/npcinfo.go pkg/rsbuf/npcinfo_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(rsbuf): NAI-9 wire observer counter into EncodeNpc

3 hook sites: inc on subscription-add (phase-2), dec on two
subscription-remove branches (phase-1 inactive, phase-1 out-of-range).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 5: Wire `rsbuf.RemovePlayer` into `processLogouts`

**Files:**
- Modify: `modules/world/tick.go`
- Modify: `modules/world/npc_event_queue_test.go`

Engine-side call of `rsbuf.RemovePlayer(p.slot, p.buildArea.Npcs)` before the player is torn down. Mirrors TS's single `rsbuf.removePlayer(pid)` call shape; the extra argument is a goscape divergence (D5).

- [ ] **Step 1: Write the failing test**

Append to `modules/world/npc_event_queue_test.go`:

```go
func TestProcessLogoutsDecrementsSubscribedNpcObservers(t *testing.T) {
	rsbuf.SetObserverForTest(101, 0) // cleanup — ensure clean state
	rsbuf.SetObserverForTest(102, 0)

	s := newServerForScriptTest(t)
	s.currentTick = 1

	// Seed a player with a buildArea subscribing to two NPCs;
	// seed their observer counts to 1.
	p := addPlayerToServer(t, s, 1, 3094, 3106, 0)
	p.buildArea = buildarea.New()
	p.buildArea.Npcs[101] = struct{}{}
	p.buildArea.Npcs[102] = struct{}{}
	rsbuf.SetObserverForTest(101, 1)
	rsbuf.SetObserverForTest(102, 1)

	// Mark the player for logout (processLogouts reads the logout queue).
	s.logoutQueue = append(s.logoutQueue, p)

	s.processLogouts()

	if got := rsbuf.GetNpcObservers(101); got != 0 {
		t.Errorf("GetNpcObservers(101) after logout: got %d, want 0", got)
	}
	if got := rsbuf.GetNpcObservers(102); got != 0 {
		t.Errorf("GetNpcObservers(102) after logout: got %d, want 0", got)
	}
}
```

**Fixture verification:** confirm `s.logoutQueue` is the correct field name + queueing convention by reading `processLogouts` before writing the test. If the mechanism is different (e.g., `p.pendingLogout = true`), adjust the test setup accordingly. The test body's assertions don't change.

- [ ] **Step 2: Run test to verify it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestProcessLogoutsDecrementsSubscribedNpcObservers -v`

Expected: FAIL with "got 1, want 0" — `processLogouts` doesn't touch observer counts yet.

- [ ] **Step 3: Add the hook in processLogouts**

In `modules/world/tick.go`, inside `processLogouts`, find the point where each exiting player is being torn down (before `p.buildArea` is cleared / the player is removed from `s.players`). Add:

```go
// NAI-9: bulk-decrement observer counts for every NPC this player
// was subscribed to. Mirrors @2004scape/rsbuf's removePlayer(pid)
// contract. Must run BEFORE buildArea is cleared.
if p.buildArea != nil {
    rsbuf.RemovePlayer(p.slot, p.buildArea.Npcs)
}
```

Ensure `pkg/rsbuf` is imported in `tick.go` (it may already be; confirm).

- [ ] **Step 4: Run test to verify it passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestProcessLogoutsDecrementsSubscribedNpcObservers -v`

Expected: PASS.

- [ ] **Step 5: Run full modules/world suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -v`

Expected: all tests pass. Watch for regressions in other `processLogouts` tests — if any fail due to observer-counter state pollution between tests, add `rsbuf.SetObserverForTest(nid, 0)` setup/teardown as needed.

- [ ] **Step 6: Commit**

```bash
git add modules/world/tick.go modules/world/npc_event_queue_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-9 processLogouts bulk-decrement observers

processLogouts calls rsbuf.RemovePlayer(slot, buildArea.Npcs)
before tearing down the player. Implements TS's
rsbuf.removePlayer(pid) contract.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 6: Replace `observers := 1` stub + flip PAUSEHUNT test + companion

**Files:**
- Modify: `modules/world/npc_hunt.go`
- Modify: `modules/world/npc_event_queue_test.go`

- [ ] **Step 1: Update the PAUSEHUNT test (flip + add companion)**

Locate `TestProcessNpcHuntPauseHuntRunsWithObserverStub` in `modules/world/npc_event_queue_test.go`. Rename + invert assertion:

```go
func TestProcessNpcHuntPauseHuntBailsWithNoObservers(t *testing.T) {
	rsbuf.SetObserverForTest(0, 0) // n.nid = 0 for this test
	s := newServerForScriptTest(t)
	n := newNpcForLifecycleTest(t) // nid = 1 per NewNpc's arg
	rsbuf.SetObserverForTest(n.nid, 0)
	n.server = s
	n.huntMode = 0 // index into huntTypes
	n.huntRange = 10
	n.huntClock = 0
	s.huntTypes = &objtype.HuntTypeConfigs{
		Configs: []*objtype.HuntType{
			{
				Type:       objtype.HuntModeNpc,
				NobodyNear: objtype.HuntNobodyNearPauseHunt,
				Rate:       1,
			},
		},
	}

	s.processNpcHunt(n)

	if n.huntClock != 0 {
		t.Errorf("huntClock: got %d, want 0 (PAUSEHUNT gate short-circuited)", n.huntClock)
	}
}

func TestProcessNpcHuntPauseHuntRunsWithObservers(t *testing.T) {
	s := newServerForScriptTest(t)
	n := newNpcForLifecycleTest(t)
	rsbuf.SetObserverForTest(n.nid, 1) // seed one observer
	defer rsbuf.SetObserverForTest(n.nid, 0) // cleanup
	n.server = s
	n.huntMode = 0
	n.huntRange = 10
	n.huntClock = 0
	s.huntTypes = &objtype.HuntTypeConfigs{
		Configs: []*objtype.HuntType{
			{
				Type:       objtype.HuntModeNpc,
				NobodyNear: objtype.HuntNobodyNearPauseHunt,
				Rate:       1,
			},
		},
	}

	s.processNpcHunt(n)

	if n.huntClock != 1 {
		t.Errorf("huntClock: got %d, want 1 (gate passed, huntClock advanced)", n.huntClock)
	}
}
```

The new `TestProcessNpcHuntPauseHuntBailsWithNoObservers` expects `huntClock == 0` because with 0 observers the PAUSEHUNT gate short-circuits at the observer-count branch. The companion exercises the opposite path.

- [ ] **Step 2: Run tests to verify both fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run "TestProcessNpcHuntPauseHunt" -v`

Expected: both FAIL. `BailsWithNoObservers` fails because the current stub (`observers := 1`) makes the gate always pass; `RunsWithObservers` fails because the old test name doesn't exist.

Actually, `RunsWithObservers` will pass under the stub (`observers := 1` matches 1 seeded observer coincidentally). It's `BailsWithNoObservers` that drives the code change.

- [ ] **Step 3: Replace the stub in npc_hunt.go**

In `modules/world/npc_hunt.go`, find:

```go
	observers := 1 // TODO: rsbuf.GetNpcObservers(n.nid) when available
```

Replace with:

```go
	observers := rsbuf.GetNpcObservers(n.nid)
```

Update the comment block above `processNpcHunt` to drop the stub-followup language. Current:

```go
// Observer gate: TS checks rsbuf.getNpcObservers(this.nid); Go
// has no equivalent observer-count API yet, so we inline
// `observers := 1` (always observed). PAUSEHUNT semantics are
// currently unobservable — tracked as follow-up in nai_followups
// memory. TS NobodyNear values:
```

Replace the first two sentences:

```go
// Observer gate: calls rsbuf.GetNpcObservers(n.nid) — the counter
// maintained by pkg/rsbuf's NpcInfo encoder (subscription add/remove)
// and by processLogouts's bulk-decrement. Mirrors TS rsbuf.getNpcObservers
// public API. TS NobodyNear values:
```

Import `pkg/rsbuf` at the top of `npc_hunt.go` if not already present.

- [ ] **Step 4: Delete the three variant stubs**

In `modules/world/npc_hunt.go`, find and delete:

```go
// huntNpcs is stubbed at NAI-7; NAI-9 fills the body.
func (n *Npc) huntNpcs(s *Server, hunt *objtype.HuntType) []entity { return nil }

// huntObjs is stubbed at NAI-7; NAI-9 fills the body.
func (n *Npc) huntObjs(s *Server, hunt *objtype.HuntType) []entity { return nil }

// huntLocs is stubbed at NAI-7; NAI-9 fills the body.
func (n *Npc) huntLocs(s *Server, hunt *objtype.HuntType) []entity { return nil }
```

These will be re-added in Tasks 7, 8, 9 in a dedicated file.

- [ ] **Step 5: Run tests to verify**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -v`

Expected: **compilation error** — the `npc_hunt.go:65-73` switch statement references `huntNpcs`/`huntObjs`/`huntLocs` which no longer exist.

That's OK — we leave it broken here. The next tasks fill the variants.

Actually: this breaks the build, so we can't commit here. We either need to (a) combine Task 6 with Tasks 7-9 into a single commit, or (b) leave the stubs in Task 6 and delete them when each variant lands.

**Revised Step 4:** DO NOT delete the stubs in Task 6. Leave them. Tasks 7, 8, 9 each REPLACE one stub individually (moving it from `npc_hunt.go` to `npc_hunt_entities.go` as the body fills in). Update Step 5's expected output accordingly.

- [ ] **Step 5 (revised): Run tests to verify**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run "TestProcessNpcHuntPauseHunt" -v`

Expected: both tests PASS.

- [ ] **Step 6: Run full modules/world suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -v`

Expected: all tests pass. Variant stubs still return nil — any other tests that exercise the hunt path still behave the same as under NAI-7.

- [ ] **Step 7: Commit**

```bash
git add modules/world/npc_hunt.go modules/world/npc_event_queue_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-9 PAUSEHUNT reads real observer counter

Replace observers := 1 stub with rsbuf.GetNpcObservers(n.nid). Flip
TestProcessNpcHuntPauseHuntRunsWithObserverStub →
TestProcessNpcHuntPauseHuntBailsWithNoObservers (expects
huntClock == 0). Add companion test asserting gate passes when
observer count > 0.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 7: `huntNpcs` variant body

**Files:**
- Create: `modules/world/npc_hunt_entities.go`
- Create: `modules/world/npc_hunt_entities_test.go`
- Modify: `modules/world/npc_hunt.go` (delete the `huntNpcs` stub)

Three test-setup helpers get created in the new test file (reused by Tasks 8 and 9): `addNpcToServerAt`, `seedNpcType`, `addLocToZone`, `addObjToZone`, `seedLocType`, `seedObjType`. Task 7 builds the NPC-side helpers; Tasks 8 and 9 build Obj/Loc helpers.

- [ ] **Step 1: Write the failing tests**

Create `modules/world/npc_hunt_entities_test.go`:

```go
package world

import (
	"testing"

	"github.com/zsrv/goscape/pkg/objtype"
)

// addNpcToServerAt seeds s.npcs[nid], registers the NPC's type in
// s.npcTypes.Configs, and indexes into s.grid. Returns the *Npc
// so tests can further mutate fields. Slot 0 is reserved; use 1+.
func addNpcToServerAt(t *testing.T, s *Server, nid, typeId, category, x, z, level int) *Npc {
	t.Helper()
	if s.grid == nil {
		s.grid = grid.New()
	}
	if s.npcTypes == nil {
		s.npcTypes = &objtype.NPCTypeConfigs{Configs: make([]*objtype.NpcType, 100)}
	}
	if typeId < len(s.npcTypes.Configs) && s.npcTypes.Configs[typeId] == nil {
		s.npcTypes.Configs[typeId] = &objtype.NpcType{
			ConfigType: objtype.ConfigType{ID: typeId},
			Category:   category,
		}
	}
	n := NewNpc(nid, typeId, x, z, level, s.npcTypes.Configs[typeId])
	if nid >= len(s.npcs) {
		grown := make([]*Npc, nid+1)
		copy(grown, s.npcs)
		s.npcs = grown
	}
	s.npcs[nid] = n
	s.grid.AddNpc(nid, x, z, level)
	return n
}

func TestHuntNpcsInRangeSameLevel(t *testing.T) {
	s := newServerForScriptTest(t)
	n := newNpcForLifecycleTest(t)
	n.server = s
	n.x, n.z, n.level = 3094, 3106, 0
	n.huntRange = 10

	tIn := addNpcToServerAt(t, s, 10, 1, -1, n.x+3, n.z+3, n.level)
	_ = addNpcToServerAt(t, s, 11, 1, -1, n.x+20, n.z+20, n.level) // out of range

	hunt := &objtype.HuntType{CheckNpc: -1, CheckCategory: -1}
	hunted := n.huntNpcs(s, hunt)

	if len(hunted) != 1 {
		t.Fatalf("hunted: got %d, want 1 (in-range only)", len(hunted))
	}
	if hunted[0].Slot() != tIn.nid {
		t.Errorf("hunted[0]: got nid %d, want %d", hunted[0].Slot(), tIn.nid)
	}
}

func TestHuntNpcsDifferentLevelExcluded(t *testing.T) {
	s := newServerForScriptTest(t)
	n := newNpcForLifecycleTest(t)
	n.server = s
	n.x, n.z, n.level = 3094, 3106, 0
	n.huntRange = 10

	_ = addNpcToServerAt(t, s, 20, 1, -1, n.x, n.z, n.level+1) // different level

	hunted := n.huntNpcs(s, &objtype.HuntType{CheckNpc: -1, CheckCategory: -1})
	if len(hunted) != 0 {
		t.Errorf("hunted: got %d, want 0 (level mismatch)", len(hunted))
	}
}

func TestHuntNpcsCheckNpcFilter(t *testing.T) {
	s := newServerForScriptTest(t)
	n := newNpcForLifecycleTest(t)
	n.server = s
	n.x, n.z, n.level = 3094, 3106, 0
	n.huntRange = 10

	_ = addNpcToServerAt(t, s, 30, 5, -1, n.x+2, n.z+2, n.level)  // typeId 5
	match := addNpcToServerAt(t, s, 31, 7, -1, n.x+3, n.z+3, n.level) // typeId 7 (target)

	hunt := &objtype.HuntType{CheckNpc: 7, CheckCategory: -1}
	hunted := n.huntNpcs(s, hunt)

	if len(hunted) != 1 {
		t.Fatalf("hunted: got %d, want 1 (CheckNpc=7 only)", len(hunted))
	}
	if hunted[0].Slot() != match.nid {
		t.Errorf("hunted[0]: got nid %d, want %d", hunted[0].Slot(), match.nid)
	}
}

func TestHuntNpcsCheckNpcNegativeOneAllowsAll(t *testing.T) {
	s := newServerForScriptTest(t)
	n := newNpcForLifecycleTest(t)
	n.server = s
	n.x, n.z, n.level = 3094, 3106, 0
	n.huntRange = 10

	_ = addNpcToServerAt(t, s, 40, 5, -1, n.x+2, n.z+2, n.level)
	_ = addNpcToServerAt(t, s, 41, 7, -1, n.x+3, n.z+3, n.level)

	hunted := n.huntNpcs(s, &objtype.HuntType{CheckNpc: -1, CheckCategory: -1})
	if len(hunted) != 2 {
		t.Errorf("hunted: got %d, want 2 (CheckNpc=-1 allows all)", len(hunted))
	}
}

func TestHuntNpcsCheckCategoryFilter(t *testing.T) {
	s := newServerForScriptTest(t)
	n := newNpcForLifecycleTest(t)
	n.server = s
	n.x, n.z, n.level = 3094, 3106, 0
	n.huntRange = 10

	_ = addNpcToServerAt(t, s, 50, 1, 42, n.x+2, n.z+2, n.level)  // cat 42
	match := addNpcToServerAt(t, s, 51, 2, 99, n.x+3, n.z+3, n.level) // cat 99 (target)

	hunt := &objtype.HuntType{CheckNpc: -1, CheckCategory: 99}
	hunted := n.huntNpcs(s, hunt)

	if len(hunted) != 1 {
		t.Fatalf("hunted: got %d, want 1", len(hunted))
	}
	if hunted[0].Slot() != match.nid {
		t.Errorf("hunted[0]: got nid %d, want %d", hunted[0].Slot(), match.nid)
	}
}

func TestHuntNpcsChebyshevDistanceBoundary(t *testing.T) {
	s := newServerForScriptTest(t)
	n := newNpcForLifecycleTest(t)
	n.server = s
	n.x, n.z, n.level = 3094, 3106, 0
	n.huntRange = 10

	// At dx = dz = 10: included (|dx|, |dz| both <= 10).
	in1 := addNpcToServerAt(t, s, 60, 1, -1, n.x+10, n.z+10, n.level)
	// At dx = 11, dz = 0: excluded.
	_ = addNpcToServerAt(t, s, 61, 1, -1, n.x+11, n.z, n.level)

	hunted := n.huntNpcs(s, &objtype.HuntType{CheckNpc: -1, CheckCategory: -1})
	if len(hunted) != 1 || hunted[0].Slot() != in1.nid {
		t.Errorf("boundary: got %v, want [nid=%d]", hunted, in1.nid)
	}
}

func TestHuntNpcsMissingTypeConfigSkipsOnCategoryFilter(t *testing.T) {
	// When an NPC has a typeId that's out of bounds of npcTypes.Configs,
	// and CheckCategory is active, the entry should be silently skipped
	// rather than crashing.
	s := newServerForScriptTest(t)
	n := newNpcForLifecycleTest(t)
	n.server = s
	n.x, n.z, n.level = 3094, 3106, 0
	n.huntRange = 10

	// Create an NPC whose typeId exceeds the Configs length.
	s.npcTypes = &objtype.NPCTypeConfigs{Configs: make([]*objtype.NpcType, 3)}
	s.npcTypes.Configs[1] = &objtype.NpcType{
		ConfigType: objtype.ConfigType{ID: 1},
		Category:   -1,
	}
	// typeId = 99 > len(Configs) = 3.
	other := NewNpc(70, 99, n.x+3, n.z+3, n.level, nil)
	if 70 >= len(s.npcs) {
		grown := make([]*Npc, 71)
		copy(grown, s.npcs)
		s.npcs = grown
	}
	s.npcs[70] = other
	s.grid = grid.New()
	s.grid.AddNpc(70, other.x, other.z, other.level)

	hunted := n.huntNpcs(s, &objtype.HuntType{CheckNpc: -1, CheckCategory: 42})
	if len(hunted) != 0 {
		t.Errorf("hunted: got %d, want 0 (oob typeId + category filter → skip)", len(hunted))
	}
}

func TestHuntNpcsNilRegistriesReturnsNil(t *testing.T) {
	s := newServerForScriptTest(t)
	n := newNpcForLifecycleTest(t)
	n.server = s
	n.huntRange = 10

	// s.grid is nil per newServerForScriptTest.
	hunted := n.huntNpcs(s, &objtype.HuntType{})
	if hunted != nil {
		t.Errorf("hunted: got %v, want nil (grid nil)", hunted)
	}

	s.grid = grid.New()
	// s.npcTypes is nil.
	hunted = n.huntNpcs(s, &objtype.HuntType{})
	if hunted != nil {
		t.Errorf("hunted: got %v, want nil (npcTypes nil)", hunted)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestHuntNpcs -v`

Expected: the `huntNpcs` stub returns nil for all cases, so assertions like `want 1` and `want 2` will FAIL; nil-registry tests will PASS already (stub returns nil).

- [ ] **Step 3: Create the variant body file**

Create `modules/world/npc_hunt_entities.go`:

```go
package world

import (
	"github.com/zsrv/goscape/pkg/objtype"
)

// huntNpcs iterates NPCs in the grid within huntRange and returns
// those passing type + category + distance filters. Matches TS
// Npc.huntNpcs at Engine-TS/.../Npc.ts:975-977, which delegates to
// HuntIterator's NPC branch at ScriptIterators.ts:98-120.
//
// Filter coverage (NAI-9):
//   - Range: Chebyshev distance <= n.huntRange
//   - Level match: NearbyNpcs is level-filtered internally
//   - CheckNpc: type-ID filter (-1 == allow all)
//   - CheckCategory: NpcType.Category filter (-1 == allow all)
//
// DEFERRED to audit pass:
//   - CheckVis (TS ScriptIterators.ts:113-118) — LoS/LoW gate.
//
// Does NOT exclude self (TS doesn't either — NPC can appear in its
// own zone's NPC list and pass all filters). Preserves TS quirk.
func (n *Npc) huntNpcs(s *Server, hunt *objtype.HuntType) []entity {
	if s.grid == nil || s.npcTypes == nil {
		return nil
	}
	zoneRadius := 1 + n.huntRange/8
	nids := s.grid.NearbyNpcs(n.x, n.z, n.level, zoneRadius)
	var hunted []entity
	for _, nid := range nids {
		if nid < 0 || nid >= len(s.npcs) {
			continue
		}
		other := s.npcs[nid]
		if other == nil {
			continue
		}
		if hunt.CheckNpc != -1 && other.typeId != hunt.CheckNpc {
			continue
		}
		if hunt.CheckCategory != -1 {
			if other.typeId < 0 || other.typeId >= len(s.npcTypes.Configs) {
				continue
			}
			ot := s.npcTypes.Configs[other.typeId]
			if ot == nil || ot.Category != hunt.CheckCategory {
				continue
			}
		}
		dx := other.x - n.x
		if dx < 0 {
			dx = -dx
		}
		dz := other.z - n.z
		if dz < 0 {
			dz = -dz
		}
		if dx > n.huntRange || dz > n.huntRange {
			continue
		}
		// TODO: CheckVis gate — TS ScriptIterators.ts:113-118.
		// Deferred; see nai_followups.md.
		hunted = append(hunted, other)
	}
	return hunted
}
```

- [ ] **Step 4: Delete the stub from `npc_hunt.go`**

In `modules/world/npc_hunt.go`, delete:

```go
// huntNpcs is stubbed at NAI-7; NAI-9 fills the body.
func (n *Npc) huntNpcs(s *Server, hunt *objtype.HuntType) []entity { return nil }
```

- [ ] **Step 5: Run tests to verify**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestHuntNpcs -v`

Expected: all 8 tests PASS.

- [ ] **Step 6: Run full modules/world suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -v`

Expected: all tests pass.

- [ ] **Step 7: Commit**

```bash
git add modules/world/npc_hunt_entities.go modules/world/npc_hunt_entities_test.go modules/world/npc_hunt.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-9 huntNpcs variant fill

Grid-iterated NPC hunt with CheckNpc/CheckCategory/Chebyshev-
distance filters. CheckVis deferred per spec D2.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 8: `huntObjs` variant body

**Files:**
- Modify: `modules/world/npc_hunt_entities.go` (append body)
- Modify: `modules/world/npc_hunt_entities_test.go` (append tests + Obj fixture)
- Modify: `modules/world/npc_hunt.go` (delete the `huntObjs` stub)

- [ ] **Step 1: Write the failing tests**

Append to `modules/world/npc_hunt_entities_test.go`:

```go
// addObjToZone seeds an *entity.Obj at (level, x, z) in the
// containing zone, registers its type in s.objTypes.Configs, and
// appends to zone.Objs. Returns the Obj for test assertions.
func addObjToZone(t *testing.T, s *Server, level, x, z, typeId, category int) *entitypkg.Obj {
	t.Helper()
	if s.zoneMap == nil {
		s.zoneMap = zone.NewZoneMap()
	}
	if s.objTypes == nil {
		s.objTypes = &objtype.ObjTypeConfigs{Configs: make([]*objtype.ObjType, 100)}
	}
	if typeId < len(s.objTypes.Configs) && s.objTypes.Configs[typeId] == nil {
		s.objTypes.Configs[typeId] = &objtype.ObjType{
			ConfigType: objtype.ConfigType{ID: typeId},
			Category:   category,
		}
	}
	o := entitypkg.NewObj(level, x, z, entitypkg.LifecycleDespawn, typeId, 1)
	zn := s.zoneMap.Get(level, x, z)
	zn.Objs = append(zn.Objs, o)
	return o
}

func TestHuntObjsInRangeSameLevel(t *testing.T) {
	s := newServerForScriptTest(t)
	n := newNpcForLifecycleTest(t)
	n.server = s
	n.x, n.z, n.level = 3094, 3106, 0
	n.huntRange = 10

	oIn := addObjToZone(t, s, n.level, n.x+3, n.z+3, 1, -1)
	_ = addObjToZone(t, s, n.level, n.x+20, n.z+20, 1, -1) // out of range

	hunt := &objtype.HuntType{CheckObj: -1, CheckCategory: -1}
	hunted := n.huntObjs(s, hunt)

	if len(hunted) != 1 {
		t.Fatalf("hunted: got %d, want 1", len(hunted))
	}
	if hunted[0] != oIn {
		t.Errorf("hunted[0]: got %v, want oIn", hunted[0])
	}
}

func TestHuntObjsDifferentLevelExcluded(t *testing.T) {
	s := newServerForScriptTest(t)
	n := newNpcForLifecycleTest(t)
	n.server = s
	n.x, n.z, n.level = 3094, 3106, 0
	n.huntRange = 10

	_ = addObjToZone(t, s, n.level+1, n.x, n.z, 1, -1) // different level

	hunted := n.huntObjs(s, &objtype.HuntType{CheckObj: -1, CheckCategory: -1})
	if len(hunted) != 0 {
		t.Errorf("hunted: got %d, want 0 (level mismatch)", len(hunted))
	}
}

func TestHuntObjsCheckObjFilter(t *testing.T) {
	s := newServerForScriptTest(t)
	n := newNpcForLifecycleTest(t)
	n.server = s
	n.x, n.z, n.level = 3094, 3106, 0
	n.huntRange = 10

	_ = addObjToZone(t, s, n.level, n.x+2, n.z+2, 5, -1)
	match := addObjToZone(t, s, n.level, n.x+3, n.z+3, 7, -1)

	hunt := &objtype.HuntType{CheckObj: 7, CheckCategory: -1}
	hunted := n.huntObjs(s, hunt)

	if len(hunted) != 1 || hunted[0] != match {
		t.Errorf("hunted: got %v, want [match]", hunted)
	}
}

func TestHuntObjsCheckObjNegativeOneAllowsAll(t *testing.T) {
	s := newServerForScriptTest(t)
	n := newNpcForLifecycleTest(t)
	n.server = s
	n.x, n.z, n.level = 3094, 3106, 0
	n.huntRange = 10

	_ = addObjToZone(t, s, n.level, n.x+2, n.z+2, 5, -1)
	_ = addObjToZone(t, s, n.level, n.x+3, n.z+3, 7, -1)

	hunted := n.huntObjs(s, &objtype.HuntType{CheckObj: -1, CheckCategory: -1})
	if len(hunted) != 2 {
		t.Errorf("hunted: got %d, want 2 (CheckObj=-1 allows all)", len(hunted))
	}
}

func TestHuntObjsCheckCategoryFilter(t *testing.T) {
	s := newServerForScriptTest(t)
	n := newNpcForLifecycleTest(t)
	n.server = s
	n.x, n.z, n.level = 3094, 3106, 0
	n.huntRange = 10

	_ = addObjToZone(t, s, n.level, n.x+2, n.z+2, 1, 42)
	match := addObjToZone(t, s, n.level, n.x+3, n.z+3, 2, 99)

	hunt := &objtype.HuntType{CheckObj: -1, CheckCategory: 99}
	hunted := n.huntObjs(s, hunt)

	if len(hunted) != 1 || hunted[0] != match {
		t.Errorf("hunted: got %v, want [match]", hunted)
	}
}

func TestHuntObjsChebyshevDistanceBoundary(t *testing.T) {
	s := newServerForScriptTest(t)
	n := newNpcForLifecycleTest(t)
	n.server = s
	n.x, n.z, n.level = 3094, 3106, 0
	n.huntRange = 10

	in1 := addObjToZone(t, s, n.level, n.x+10, n.z+10, 1, -1)
	_ = addObjToZone(t, s, n.level, n.x+11, n.z, 1, -1)

	hunted := n.huntObjs(s, &objtype.HuntType{CheckObj: -1, CheckCategory: -1})
	if len(hunted) != 1 || hunted[0] != in1 {
		t.Errorf("boundary: got %v, want [in1]", hunted)
	}
}

func TestHuntObjsMissingTypeConfigSkipsOnCategoryFilter(t *testing.T) {
	s := newServerForScriptTest(t)
	n := newNpcForLifecycleTest(t)
	n.server = s
	n.x, n.z, n.level = 3094, 3106, 0
	n.huntRange = 10

	s.zoneMap = zone.NewZoneMap()
	s.objTypes = &objtype.ObjTypeConfigs{Configs: make([]*objtype.ObjType, 3)}
	// Obj with typeId 99 (exceeds Configs length).
	o := entitypkg.NewObj(n.level, n.x+3, n.z+3, entitypkg.LifecycleDespawn, 99, 1)
	zn := s.zoneMap.Get(n.level, o.X, o.Z)
	zn.Objs = append(zn.Objs, o)

	hunted := n.huntObjs(s, &objtype.HuntType{CheckObj: -1, CheckCategory: 42})
	if len(hunted) != 0 {
		t.Errorf("hunted: got %d, want 0 (oob typeId + category filter → skip)", len(hunted))
	}
}

func TestHuntObjsNilRegistriesReturnsNil(t *testing.T) {
	s := newServerForScriptTest(t)
	n := newNpcForLifecycleTest(t)
	n.server = s
	n.huntRange = 10

	hunted := n.huntObjs(s, &objtype.HuntType{})
	if hunted != nil {
		t.Errorf("hunted: got %v, want nil (zoneMap nil)", hunted)
	}

	s.zoneMap = zone.NewZoneMap()
	hunted = n.huntObjs(s, &objtype.HuntType{})
	if hunted != nil {
		t.Errorf("hunted: got %v, want nil (objTypes nil)", hunted)
	}
}
```

Imports to add at the top of `npc_hunt_entities_test.go`: `entitypkg "github.com/zsrv/goscape/pkg/entity"` and `"github.com/zsrv/goscape/pkg/zone"` (if not already present from Task 7 — Task 7's tests only needed `objtype` and `grid`).

- [ ] **Step 2: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestHuntObjs -v`

Expected: tests that assert non-zero hunted FAIL (stub returns nil); nil-registry tests PASS.

- [ ] **Step 3: Append huntObjs body**

Append to `modules/world/npc_hunt_entities.go`:

```go
// huntObjs iterates dynamic objs in zone-radius and returns those
// passing the filter chain. Matches TS Npc.huntObjs at
// Engine-TS/.../Npc.ts:979-981 (HuntIterator OBJ branch at
// ScriptIterators.ts:121-144).
//
// goscape Zone.Objs contains only LifecycleDespawn (dynamic) objs
// by construction (pkg/zone/zone.go:221). Matches TS comment
// "scripting only cares about dynamic objs??" at
// ScriptIterators.ts:122.
//
// Filter coverage (NAI-9):
//   - Range: Chebyshev distance <= n.huntRange
//   - CheckObj: obj.Type filter (-1 == allow all)
//   - CheckCategory: ObjType.Category filter (-1 == allow all)
//
// DEFERRED to audit pass:
//   - CheckVis (TS ScriptIterators.ts:137-142) — LoS/LoW gate.
func (n *Npc) huntObjs(s *Server, hunt *objtype.HuntType) []entity {
	if s.zoneMap == nil || s.objTypes == nil {
		return nil
	}
	zoneRadius := 1 + n.huntRange/8
	var hunted []entity
	for _, zn := range s.zoneMap.NearbyZones(n.level, n.x, n.z, zoneRadius) {
		for _, o := range zn.Objs {
			if o == nil {
				continue
			}
			if hunt.CheckObj != -1 && o.Type != hunt.CheckObj {
				continue
			}
			if hunt.CheckCategory != -1 {
				if o.Type < 0 || o.Type >= len(s.objTypes.Configs) {
					continue
				}
				ot := s.objTypes.Configs[o.Type]
				if ot == nil || ot.Category != hunt.CheckCategory {
					continue
				}
			}
			dx := o.X - n.x
			if dx < 0 {
				dx = -dx
			}
			dz := o.Z - n.z
			if dz < 0 {
				dz = -dz
			}
			if dx > n.huntRange || dz > n.huntRange {
				continue
			}
			// TODO: CheckVis gate — TS ScriptIterators.ts:137-142.
			hunted = append(hunted, o)
		}
	}
	return hunted
}
```

- [ ] **Step 4: Delete the stub from `npc_hunt.go`**

In `modules/world/npc_hunt.go`, delete:

```go
// huntObjs is stubbed at NAI-7; NAI-9 fills the body.
func (n *Npc) huntObjs(s *Server, hunt *objtype.HuntType) []entity { return nil }
```

- [ ] **Step 5: Run tests to verify**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestHuntObjs -v`

Expected: all 8 tests PASS.

- [ ] **Step 6: Run full modules/world suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -v`

Expected: all tests pass.

- [ ] **Step 7: Commit**

```bash
git add modules/world/npc_hunt_entities.go modules/world/npc_hunt_entities_test.go modules/world/npc_hunt.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-9 huntObjs variant fill

Zone-iterated Obj hunt with CheckObj/CheckCategory/Chebyshev-
distance filters. Dynamic-only (Zone.Objs only contains
LifecycleDespawn objs by construction). CheckVis deferred per
spec D2.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 9: `huntLocs` variant body

**Files:**
- Modify: `modules/world/npc_hunt_entities.go` (append body)
- Modify: `modules/world/npc_hunt_entities_test.go` (append tests + Loc fixture)
- Modify: `modules/world/npc_hunt.go` (delete the `huntLocs` stub)

Structurally identical to Task 8 with the Obj→Loc substitution. `Zone.Locs` contains both static (Respawn) and dynamic (Despawn) locs — matches TS `getAllLocsSafe(true)`.

- [ ] **Step 1: Write the failing tests**

Append to `modules/world/npc_hunt_entities_test.go`:

```go
// addLocToZone seeds an *entity.Loc at (level, x, z) in the
// containing zone, registers its type in s.locTypes.Configs, and
// appends to zone.Locs. Returns the Loc for test assertions.
func addLocToZone(t *testing.T, s *Server, level, x, z, typeId, category int) *entitypkg.Loc {
	t.Helper()
	if s.zoneMap == nil {
		s.zoneMap = zone.NewZoneMap()
	}
	if s.locTypes == nil {
		s.locTypes = &objtype.LocTypeConfigs{Configs: make([]*objtype.LocType, 5200)}
	}
	if typeId < len(s.locTypes.Configs) && s.locTypes.Configs[typeId] == nil {
		s.locTypes.Configs[typeId] = &objtype.LocType{
			ConfigType: objtype.ConfigType{ID: typeId},
			Category:   category,
		}
	}
	l := entitypkg.NewLoc(level, x, z, 1, 1, entitypkg.LifecycleForever, typeId, 10, 0)
	zn := s.zoneMap.Get(level, x, z)
	zn.AddStaticLoc(l)
	return l
}

func TestHuntLocsInRangeSameLevel(t *testing.T) {
	s := newServerForScriptTest(t)
	n := newNpcForLifecycleTest(t)
	n.server = s
	n.x, n.z, n.level = 3094, 3106, 0
	n.huntRange = 10

	lIn := addLocToZone(t, s, n.level, n.x+3, n.z+3, 1000, -1)
	_ = addLocToZone(t, s, n.level, n.x+20, n.z+20, 1000, -1)

	hunted := n.huntLocs(s, &objtype.HuntType{CheckLoc: -1, CheckCategory: -1})
	if len(hunted) != 1 {
		t.Fatalf("hunted: got %d, want 1", len(hunted))
	}
	if hunted[0] != lIn {
		t.Errorf("hunted[0]: got %v, want lIn", hunted[0])
	}
}

func TestHuntLocsDifferentLevelExcluded(t *testing.T) {
	s := newServerForScriptTest(t)
	n := newNpcForLifecycleTest(t)
	n.server = s
	n.x, n.z, n.level = 3094, 3106, 0
	n.huntRange = 10

	_ = addLocToZone(t, s, n.level+1, n.x, n.z, 1000, -1)

	hunted := n.huntLocs(s, &objtype.HuntType{CheckLoc: -1, CheckCategory: -1})
	if len(hunted) != 0 {
		t.Errorf("hunted: got %d, want 0", len(hunted))
	}
}

func TestHuntLocsCheckLocFilter(t *testing.T) {
	s := newServerForScriptTest(t)
	n := newNpcForLifecycleTest(t)
	n.server = s
	n.x, n.z, n.level = 3094, 3106, 0
	n.huntRange = 10

	_ = addLocToZone(t, s, n.level, n.x+2, n.z+2, 1000, -1)
	match := addLocToZone(t, s, n.level, n.x+3, n.z+3, 2000, -1)

	hunted := n.huntLocs(s, &objtype.HuntType{CheckLoc: 2000, CheckCategory: -1})
	if len(hunted) != 1 || hunted[0] != match {
		t.Errorf("hunted: got %v, want [match]", hunted)
	}
}

func TestHuntLocsCheckLocNegativeOneAllowsAll(t *testing.T) {
	s := newServerForScriptTest(t)
	n := newNpcForLifecycleTest(t)
	n.server = s
	n.x, n.z, n.level = 3094, 3106, 0
	n.huntRange = 10

	_ = addLocToZone(t, s, n.level, n.x+2, n.z+2, 1000, -1)
	_ = addLocToZone(t, s, n.level, n.x+3, n.z+3, 2000, -1)

	hunted := n.huntLocs(s, &objtype.HuntType{CheckLoc: -1, CheckCategory: -1})
	if len(hunted) != 2 {
		t.Errorf("hunted: got %d, want 2", len(hunted))
	}
}

func TestHuntLocsCheckCategoryFilter(t *testing.T) {
	s := newServerForScriptTest(t)
	n := newNpcForLifecycleTest(t)
	n.server = s
	n.x, n.z, n.level = 3094, 3106, 0
	n.huntRange = 10

	_ = addLocToZone(t, s, n.level, n.x+2, n.z+2, 1000, 42)
	match := addLocToZone(t, s, n.level, n.x+3, n.z+3, 2000, 99)

	hunted := n.huntLocs(s, &objtype.HuntType{CheckLoc: -1, CheckCategory: 99})
	if len(hunted) != 1 || hunted[0] != match {
		t.Errorf("hunted: got %v, want [match]", hunted)
	}
}

func TestHuntLocsChebyshevDistanceBoundary(t *testing.T) {
	s := newServerForScriptTest(t)
	n := newNpcForLifecycleTest(t)
	n.server = s
	n.x, n.z, n.level = 3094, 3106, 0
	n.huntRange = 10

	in1 := addLocToZone(t, s, n.level, n.x+10, n.z+10, 1000, -1)
	_ = addLocToZone(t, s, n.level, n.x+11, n.z, 1000, -1)

	hunted := n.huntLocs(s, &objtype.HuntType{CheckLoc: -1, CheckCategory: -1})
	if len(hunted) != 1 || hunted[0] != in1 {
		t.Errorf("boundary: got %v, want [in1]", hunted)
	}
}

func TestHuntLocsMissingTypeConfigSkipsOnCategoryFilter(t *testing.T) {
	s := newServerForScriptTest(t)
	n := newNpcForLifecycleTest(t)
	n.server = s
	n.x, n.z, n.level = 3094, 3106, 0
	n.huntRange = 10

	s.zoneMap = zone.NewZoneMap()
	s.locTypes = &objtype.LocTypeConfigs{Configs: make([]*objtype.LocType, 3)}
	l := entitypkg.NewLoc(n.level, n.x+3, n.z+3, 1, 1, entitypkg.LifecycleForever, 99, 10, 0)
	s.zoneMap.Get(n.level, l.X, l.Z).AddStaticLoc(l)

	hunted := n.huntLocs(s, &objtype.HuntType{CheckLoc: -1, CheckCategory: 42})
	if len(hunted) != 0 {
		t.Errorf("hunted: got %d, want 0", len(hunted))
	}
}

func TestHuntLocsNilRegistriesReturnsNil(t *testing.T) {
	s := newServerForScriptTest(t)
	n := newNpcForLifecycleTest(t)
	n.server = s
	n.huntRange = 10

	hunted := n.huntLocs(s, &objtype.HuntType{})
	if hunted != nil {
		t.Errorf("hunted: got %v, want nil (zoneMap nil)", hunted)
	}

	s.zoneMap = zone.NewZoneMap()
	hunted = n.huntLocs(s, &objtype.HuntType{})
	if hunted != nil {
		t.Errorf("hunted: got %v, want nil (locTypes nil)", hunted)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestHuntLocs -v`

Expected: similar to Task 8 — positive assertions FAIL, nil-registry ones PASS.

- [ ] **Step 3: Append huntLocs body**

Append to `modules/world/npc_hunt_entities.go`:

```go
// huntLocs iterates locs in zone-radius and returns those passing
// the filter chain. Matches TS Npc.huntLocs at
// Engine-TS/.../Npc.ts:983-985 (HuntIterator SCENERY branch at
// ScriptIterators.ts:145-167).
//
// Zone.Locs contains both static (Respawn) and dynamic (Despawn)
// locs — matches TS getAllLocsSafe(true).
//
// Multi-tile locs use SW corner for distance (l.X/l.Z ARE the SW
// corner by goscape entity.Entity convention, matching TS which
// passes {x: loc.x, z: loc.z} to distanceToSW).
//
// Filter coverage (NAI-9):
//   - Range: Chebyshev distance <= n.huntRange
//   - CheckLoc: loc.Type() filter (-1 == allow all)
//   - CheckCategory: LocType.Category filter (-1 == allow all)
//
// DEFERRED to audit pass:
//   - CheckVis (TS ScriptIterators.ts:160-165) — LoS/LoW gate.
func (n *Npc) huntLocs(s *Server, hunt *objtype.HuntType) []entity {
	if s.zoneMap == nil || s.locTypes == nil {
		return nil
	}
	zoneRadius := 1 + n.huntRange/8
	var hunted []entity
	for _, zn := range s.zoneMap.NearbyZones(n.level, n.x, n.z, zoneRadius) {
		for _, l := range zn.Locs {
			if l == nil {
				continue
			}
			if hunt.CheckLoc != -1 && l.Type() != hunt.CheckLoc {
				continue
			}
			if hunt.CheckCategory != -1 {
				if l.Type() < 0 || l.Type() >= len(s.locTypes.Configs) {
					continue
				}
				lt := s.locTypes.Configs[l.Type()]
				if lt == nil || lt.Category != hunt.CheckCategory {
					continue
				}
			}
			dx := l.X - n.x
			if dx < 0 {
				dx = -dx
			}
			dz := l.Z - n.z
			if dz < 0 {
				dz = -dz
			}
			if dx > n.huntRange || dz > n.huntRange {
				continue
			}
			// TODO: CheckVis gate — TS ScriptIterators.ts:160-165.
			hunted = append(hunted, l)
		}
	}
	return hunted
}
```

- [ ] **Step 4: Delete the stub from `npc_hunt.go`**

In `modules/world/npc_hunt.go`, delete:

```go
// huntLocs is stubbed at NAI-7; NAI-9 fills the body.
func (n *Npc) huntLocs(s *Server, hunt *objtype.HuntType) []entity { return nil }
```

- [ ] **Step 5: Run tests to verify**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestHuntLocs -v`

Expected: all 8 tests PASS.

- [ ] **Step 6: Run full module test suite + race detector**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`

Expected: all tests pass across all packages.

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/world/ ./pkg/rsbuf/`

Expected: no race conditions detected.

- [ ] **Step 7: Commit**

```bash
git add modules/world/npc_hunt_entities.go modules/world/npc_hunt_entities_test.go modules/world/npc_hunt.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-9 huntLocs variant fill

Zone-iterated Loc hunt with CheckLoc/CheckCategory/Chebyshev-
distance filters (SW-corner for multi-tile). Zone.Locs contains
both static and dynamic locs (matches TS getAllLocsSafe(true)).
CheckVis deferred per spec D2.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 10: Memory housekeeping — update `nai_followups.md`

**Files:**
- Modify: `/home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md`

Three edits to the memory file: (a) add CheckVis gap entry under "From NAI-8", (b) amend checkNotCombat/Self entries with the outer-guard note, (c) mark the NAI-7 observer-counting blocker resolved.

- [ ] **Step 1: Edit 1 — add CheckVis entry under "From NAI-8"**

Open `/home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md`. Locate the "From NAI-8 (2026-04-22)" section header. Before the existing `### Deferred filters in huntPlayers (future audit)` entry, insert a new entry:

```markdown
### Deferred: CheckVis (LoS/LoW) gate on all four hunt variants

TS `HuntIterator` applies `HuntVis.LINEOFSIGHT` / `HuntVis.LINEOFWALK`
gates in each per-mode branch:

- PLAYER: `ScriptIterators.ts:88-94`
- NPC: `ScriptIterators.ts:113-118`
- OBJ: `ScriptIterators.ts:137-142`
- SCENERY: `ScriptIterators.ts:160-165`

NAI-8 shipped huntPlayers without this gate (silently — no TODO
breadcrumb was left). NAI-9 surfaced the gap during fidelity review
and deferred CheckVis symmetrically across all four variants with
matching TS-line TODO breadcrumbs.

Remediation when the audit pass runs: the `pkg/pathfinder/routefinder`
package has `LineOfSight` / `LineOfWalk` routines available. Each
variant applies its gate AFTER distance check, BEFORE the `hunted`
append. The gate is a no-op when `hunt.CheckVis == HuntVisOff`.
```

- [ ] **Step 2: Edit 2 — amend checkNotCombat / checkNotCombatSelf entries**

In the same "From NAI-8" section's `### Deferred filters in huntPlayers (future audit)` entry, find the `checkNotCombat` and `checkNotCombatSelf` bullets. Amend them by appending the outer-guard note:

Replace:
```markdown
3. **checkNotCombat (TS:943-945)** — needs a Varp read with an
   8-tick combat-window comparison against `World.currentTick`.
   Varp read infrastructure exists (S5b); the combat-window gate
   needs tracking when the active player last entered combat.

4. **checkNotCombatSelf (TS:946-948)** — same as checkNotCombat but
   on the NPC side (`this.getVar(hunt.checkNotCombatSelf)`). Needs
   per-NPC varn read (varns infrastructure exists from S6-era).
```

With:
```markdown
3. **checkNotCombat (TS:943-945)** — needs a Varp read with an
   8-tick combat-window comparison against `World.currentTick`.
   Varp read infrastructure exists (S5b); the combat-window gate
   needs tracking when the active player last entered combat.
   **Outer guard:** Both this filter AND checkNotCombatSelf are
   conditionally applied inside an outer `this.target !== player
   && !World.gameMap.isMulti(CoordGrid.packCoord(player.level,
   player.x, player.z))` check at TS `Npc.ts:942`. The audit-pass
   implementer MUST port this guard alongside both filters —
   otherwise both apply unconditionally and diverge from TS.

4. **checkNotCombatSelf (TS:946-948)** — same as checkNotCombat but
   on the NPC side (`this.getVar(hunt.checkNotCombatSelf)`). Needs
   per-NPC varn read (varns infrastructure exists from S6-era).
   See checkNotCombat entry above for the shared outer guard.
```

- [ ] **Step 3: Edit 3 — mark NAI-7 observer-counting blocker resolved**

In the "From NAI-7 (2026-04-22)" section, find the `### NAI-9 blocker: real observer counting for PAUSEHUNT gate` entry. Replace its entire body with a resolution note:

Replace:
```markdown
### NAI-9 blocker: real observer counting for PAUSEHUNT gate

NAI-7's `processNpcHunt` at `modules/world/npc_hunt.go:37` inlines
`observers := 1` with a TODO comment pointing at `rsbuf.GetNpcObservers`.
This was intentional — `rsbuf` has no observer-count API yet, and
NAI-7 scope didn't include adding one.

[... rest of original body ...]
```

With:
```markdown
### NAI-9 blocker: real observer counting for PAUSEHUNT gate

**Resolved 2026-04-22 (NAI-9).** `pkg/rsbuf/npc_observers.go`
ships the observer counter matching `@2004scape/rsbuf`'s public
API (`GetNpcObservers(nid int) int`, `RemovePlayer(pid int,
subscribedNpcs map[int]struct{})`). Counter is maintained by
`EncodeNpc` at three hook sites (subscription add + two remove
branches), bulk-decremented on player logout via
`processLogouts`. `processNpcHunt` now reads the real count.

The original follow-up proposed two implementation options (rsbuf
API or grid-walk approximation). Neither was precisely right —
NAI-9 brainstorm traced the `@2004scape/rsbuf` shim at
`Engine-TS/node_modules/@2004scape/rsbuf/dist/rsbuf.d.ts:13` and
mirrored that API shape exactly. See
`docs/superpowers/specs/2026-04-22-nai-9-hunt-variant-fill-design.md`.

Tests flipped: `TestProcessNpcHuntPauseHuntRunsWithObserverStub` →
`TestProcessNpcHuntPauseHuntBailsWithNoObservers` (now expects
`huntClock == 0` when no observers). Companion test
`TestProcessNpcHuntPauseHuntRunsWithObservers` asserts the inverse.
```

- [ ] **Step 4: Commit the memory edits**

```bash
cd /home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory
# Memory dir is not a git repo — no commit. Just verify the file
# is saved and syntactically OK.
head -60 nai_followups.md
cd /home/owner/Code/github.com/zsrv/goscape
```

Memory files are not tracked in git. The edit is complete once the file is saved.

- [ ] **Step 5: Final verification — full suite + NAI-9 scope closure**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./... && GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...`

Expected: everything passes.

Manually re-grep `npc_hunt.go` to confirm no stubs remain:

```bash
grep -n "stubbed at NAI-7" modules/world/npc_hunt.go
grep -n "observers := 1" modules/world/npc_hunt.go
```

Expected: no matches.

- [ ] **Step 6: Closing commit**

```bash
git commit --no-gpg-sign --allow-empty -m "$(cat <<'EOF'
feat(world): NAI-9 closes huntNpcs/huntObjs/huntLocs + PAUSEHUNT observer counter — closes NAI-9

Three variant bodies (huntNpcs grid-iter, huntObjs/huntLocs zone-iter)
with type/category/distance filters. Observer counter in pkg/rsbuf
mirrors @2004scape/rsbuf public API. CheckVis deferred symmetrically
with matching TODO breadcrumbs.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

This closing commit is `--allow-empty` because all code landed in Tasks 1-9; this commit exists purely to mark NAI-9 as closed in git history following the series' "closes NAI-N" convention (see cb35fe0, 055077f, etc.).

---

## Self-review notes (executed during plan-write)

**Spec coverage check:** every Scope-IN item has a task:
- Three variant bodies → Tasks 7, 8, 9 ✓
- ZoneMap.NearbyZones helper → Task 2 ✓
- rsbuf-owned observer counter → Task 3 ✓
- rsbuf-owned logout cleanup → Task 5 (+ hook in Task 5) ✓
- Obj entity-interface satisfaction → Task 1 ✓
- Test assertion flip → Task 6 ✓
- nai_followups.md updates → Task 10 ✓

**Placeholder scan:** none found — all code blocks are complete.

**Type consistency:**
- `rsbuf.GetNpcObservers(nid int) int` used consistently across Tasks 3/4/5/6.
- `rsbuf.RemovePlayer(pid int, subscribedNpcs map[int]struct{})` used consistently in Tasks 3/5.
- `rsbuf.SetObserverForTest(nid, count int)` used consistently in Tasks 3/5/6.
- `*objtype.NPCTypeConfigs` (capital-NPC) used in Task 7 tests — matches server.go:82. Verified.
- `*objtype.ObjTypeConfigs` / `*objtype.LocTypeConfigs` used in Tasks 8/9 — matches server.go:67/73. Verified.
- `entitypkg.NewObj(level, x, z, Lifecycle, typeId, count)` — matches `pkg/entity/obj.go:24`. Verified.
- `entitypkg.NewLoc(level, x, z, width, length, Lifecycle, typ, shape, angle)` — matches `pkg/entity/loc.go:14`. Verified.
- `*Zone.AddStaticLoc(loc)` — matches `pkg/zone/zone.go:120`. Verified.

No inconsistencies.
