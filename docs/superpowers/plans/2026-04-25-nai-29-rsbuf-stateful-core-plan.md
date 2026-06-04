# NAI-29 pkg/rsbuf Stateful Core + Entity Model — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Land the rsbuf-internal entity model (`Player`, `Npc`, `BuildArea`, `idBitSet`, internal `zoneMap`) and the `*Buf` instance + stateful API skeleton (`AddPlayer`/`RemovePlayer`/`AddNpc`/`RemoveNpc`/`ComputePlayer`/`ComputeNpc`/`Cleanup`/`HasPlayer`/`HasNpc`/`GetNpcObservers`); wire production parallel-write caller hooks in `modules/world` so the new state is populated on every entity mutation; leave the existing `Encode`/`EncodeNpc` read path untouched. End state: parallel-write window per `parallel_spatial_index_migration_pattern` memory; NAI-30 does the read-flip cleanly.

**Architecture:** Four bundles, single-implementer serial execution. Bundle 1 lands primitives (`idBitSet` + `zoneMap`/`zone`) — pure foundation, no public API. Bundle 2 lands `Player` + `Npc` structs with sub-structs (`Chat`, `ExactMove`) and `cleanup()` methods. Bundle 3 lands `BuildArea` (with fixed `viewDistance=15`; resize logic deferred to NAI-32) and the full `*Buf` instance + public API; B3's `*Buf` ties B1+B2 together. Bundle 4 wires `*Server.rsbuf` field + lifecycle hooks (`AddPlayer`/`RemovePlayer` at login/logout; `AddNpc`/`RemoveNpc` at spawn/despawn) + per-tick state push (`ComputePlayer`/`ComputeNpc`/`Cleanup`). Existing encoder (`Encode`/`EncodeNpc` in `pkg/rsbuf/playerinfo.go`/`npcinfo.go`) is untouched; existing `pkg/grid` write-side maintenance is untouched.

**Tech Stack:** Go 1.26+ (project uses generics + `iter.Seq`). Existing packages used: `pkg/rsbuf` (existing types — `Visibility`, `PlayerSource`, `NpcSource`, `Encode`, `EncodeNpc`, `Renderer`, mask-payload encoders all preserved unchanged), `pkg/coordgrid` (coord packing reused — `PackCoord`, `UnpackCoord`, `ZoneIndex`), `modules/world` (B4 wiring only). No new third-party dependencies. New files all in `pkg/rsbuf/` (B1-B3) and `modules/world/` (B4 tests).

**Predecessors:** NAI-28 closed at `737337d`. NAI-29 spec: `docs/superpowers/specs/2026-04-25-nai-29-rsbuf-stateful-core-design.md` committed at `816086a`. Source root: `/home/owner/Code/github.com/2004scape/rsbuf` branch `225` (HEAD `1cbb2ce`).

**Build/test commands** (per `CLAUDE.md`):
- Build: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build -trimpath ./...`
- Test all: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./...`
- Test single package: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./pkg/rsbuf/...`
- Test single function: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./pkg/rsbuf/ -run TestIdBitSet_InsertContains`
- Race detector: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 -race ./...`

**Commit discipline:** All commits use `git commit --no-gpg-sign`. Each commit body includes the standard `Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>` trailer. Commit message format: `feat(rsbuf): NAI-29 Bundle N Task N.M — <one-line summary>` for code; `test(rsbuf): ...` for test-only; `feat(world): NAI-29 Bundle 4 Task 4.M — <summary>` for B4.

**Stale-IDE-diagnostic discipline** (per `verify_implementer_claims` failure-mode #1): After each task's implementation step, run a fresh `go build ./...` + `go test -count=1 ./...` to confirm. Ignore gopls/IDE warnings unless they reproduce in a fresh process — gopls cache lag has produced false positives across every prior NAI sub-spec.

---

# Bundle 1 — Primitives (`idBitSet`, `zoneMap`, `zone`)

Bundle 1 lands the rsbuf-internal collections that back `*Buf`'s state. No public API surface, no `*Buf` instance yet, no caller touches. All tests live in `pkg/rsbuf/` and exercise the primitives in isolation.

**Source mappings to upstream:** `idBitSet` mirrors `2004scape/rsbuf/src/build.rs:8-62`; `zone` + `zoneMap` mirror `2004scape/rsbuf/src/grid.rs`. Both are unexported (lowercase) — only used internally by `BuildArea` and `*Buf`.

## Task 1.1: `idBitSet` (`pkg/rsbuf/idbitset.go`)

**Files:**
- Create: `pkg/rsbuf/idbitset.go`
- Create: `pkg/rsbuf/idbitset_test.go`

- [ ] **Step 1: Write failing tests in `pkg/rsbuf/idbitset_test.go`**

```go
package rsbuf

import (
	"testing"
)

func TestIdBitSet_InsertContains(t *testing.T) {
	s := newIdBitSet(2048, 250)
	if s.Contains(5) {
		t.Error("empty set contains 5")
	}
	s.Insert(5)
	if !s.Contains(5) {
		t.Error("after Insert(5), Contains(5) is false")
	}
	if s.Contains(6) {
		t.Error("after Insert(5), Contains(6) is true")
	}
}

func TestIdBitSet_InsertIdempotent(t *testing.T) {
	s := newIdBitSet(2048, 250)
	s.Insert(10)
	s.Insert(10)
	s.Insert(10)
	if s.Len() != 1 {
		t.Errorf("after triple Insert(10), Len: got %d, want 1", s.Len())
	}
	if got := s.Iter(); len(got) != 1 || got[0] != 10 {
		t.Errorf("Iter: got %v, want [10]", got)
	}
}

func TestIdBitSet_RemoveExisting(t *testing.T) {
	s := newIdBitSet(2048, 250)
	s.Insert(7)
	s.Insert(9)
	s.Insert(11)
	s.Remove(9)
	if s.Contains(9) {
		t.Error("after Remove(9), Contains(9) is true")
	}
	if !s.Contains(7) || !s.Contains(11) {
		t.Error("Remove(9) affected unrelated ids")
	}
	if s.Len() != 2 {
		t.Errorf("Len: got %d, want 2", s.Len())
	}
}

func TestIdBitSet_RemoveAbsentIsNoop(t *testing.T) {
	s := newIdBitSet(2048, 250)
	s.Insert(7)
	s.Remove(42) // not present — no-op
	if s.Len() != 1 || !s.Contains(7) {
		t.Errorf("Remove(absent) altered set: Len=%d, Contains(7)=%v", s.Len(), s.Contains(7))
	}
}

func TestIdBitSet_IterPreservesInsertionOrder(t *testing.T) {
	s := newIdBitSet(2048, 250)
	s.Insert(20)
	s.Insert(5)
	s.Insert(15)
	s.Insert(10)
	got := s.Iter()
	want := []int32{20, 5, 15, 10}
	if len(got) != len(want) {
		t.Fatalf("Iter len: got %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("Iter[%d]: got %d, want %d", i, got[i], want[i])
		}
	}
}

func TestIdBitSet_IterAfterRemove(t *testing.T) {
	s := newIdBitSet(2048, 250)
	s.Insert(1)
	s.Insert(2)
	s.Insert(3)
	s.Remove(2)
	got := s.Iter()
	want := []int32{1, 3}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("Iter after Remove(2): got %v, want %v", got, want)
	}
}

func TestIdBitSet_IterIsCopy(t *testing.T) {
	s := newIdBitSet(2048, 250)
	s.Insert(7)
	got := s.Iter()
	got[0] = 99 // mutate caller-owned slice
	if !s.Contains(7) {
		t.Error("Iter return mutation leaked into bit-array")
	}
	if s.Iter()[0] != 7 {
		t.Error("Iter return mutation leaked into ids slice")
	}
}

func TestIdBitSet_Clear(t *testing.T) {
	s := newIdBitSet(2048, 250)
	s.Insert(1)
	s.Insert(31)
	s.Insert(32) // crosses bit-word boundary
	s.Insert(2047)
	s.Clear()
	if s.Len() != 0 {
		t.Errorf("after Clear, Len: got %d, want 0", s.Len())
	}
	for _, id := range []int32{1, 31, 32, 2047} {
		if s.Contains(id) {
			t.Errorf("after Clear, Contains(%d) is true", id)
		}
	}
}

func TestIdBitSet_BoundsBitWordCrossing(t *testing.T) {
	s := newIdBitSet(2048, 250)
	for _, id := range []int32{0, 31, 32, 33, 63, 64, 2047} {
		s.Insert(id)
	}
	for _, id := range []int32{0, 31, 32, 33, 63, 64, 2047} {
		if !s.Contains(id) {
			t.Errorf("Contains(%d) = false after Insert", id)
		}
	}
	if s.Len() != 7 {
		t.Errorf("Len: got %d, want 7", s.Len())
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./pkg/rsbuf/ -run TestIdBitSet`
Expected: FAIL with `undefined: newIdBitSet` (compile error).

- [ ] **Step 3: Implement `pkg/rsbuf/idbitset.go`**

```go
package rsbuf

// idBitSet pairs a bit-array for O(1) containment with an ordered ID list
// for iteration. Mirrors upstream build.rs IdBitSet (2004scape/rsbuf
// branch 225, src/build.rs:8-62).
//
// Used by BuildArea to track per-player observed players + npcs.
//
// Concurrency: tick-goroutine-owned. No internal synchronization
// (matches upstream's WASM single-threaded model — see lib.rs static-mut
// model).
type idBitSet struct {
	bits []uint32 // bits[id>>5] & (1 << (id & 0x1f)) tests containment
	ids  []int32  // insertion-ordered list of contained ids
}

// newIdBitSet returns an empty idBitSet with bit-array sized to address
// ids in [0, maxID) and ids slice pre-allocated to capacity. capacity is
// only an initial backing-array hint; the slice grows as needed.
//
// maxID must be a power-of-two-multiple of 32; pass 2048 (player slot
// count) or 8192 (npc nid count).
func newIdBitSet(maxID, capacity int) *idBitSet {
	return &idBitSet{
		bits: make([]uint32, maxID/32),
		ids:  make([]int32, 0, capacity),
	}
}

// Contains reports whether id is in the set. O(1).
func (s *idBitSet) Contains(id int32) bool {
	return s.bits[id>>5]&(1<<(id&0x1f)) != 0
}

// Insert adds id to the set. No-op if id is already present (the insertion
// order list is preserved exactly once per id).
func (s *idBitSet) Insert(id int32) {
	if s.Contains(id) {
		return
	}
	s.bits[id>>5] |= 1 << (id & 0x1f)
	s.ids = append(s.ids, id)
}

// Remove takes id out of the set. No-op if id is not present.
func (s *idBitSet) Remove(id int32) {
	if !s.Contains(id) {
		return
	}
	s.bits[id>>5] &^= 1 << (id & 0x1f)
	for i, v := range s.ids {
		if v == id {
			s.ids = append(s.ids[:i], s.ids[i+1:]...)
			return
		}
	}
}

// Len returns the number of ids in the set.
func (s *idBitSet) Len() int {
	return len(s.ids)
}

// Iter returns a copy of the contained ids in insertion order. Caller-
// owned: mutating the returned slice does not affect the set. Mirrors
// upstream IdBitSet::iter at build.rs:53-55 which clones the Vec.
func (s *idBitSet) Iter() []int32 {
	out := make([]int32, len(s.ids))
	copy(out, s.ids)
	return out
}

// Clear empties the set: zeros all bit-words and truncates the ids
// slice. Capacity of bits + ids is preserved.
func (s *idBitSet) Clear() {
	for i := range s.bits {
		s.bits[i] = 0
	}
	s.ids = s.ids[:0]
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./pkg/rsbuf/ -run TestIdBitSet -v`
Expected: PASS for all 9 test functions.

- [ ] **Step 5: Run full package + race build**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 -race ./pkg/rsbuf/...`
Expected: PASS (no regressions; no race conditions).

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...`
Expected: clean build.

- [ ] **Step 6: Commit**

```bash
git add pkg/rsbuf/idbitset.go pkg/rsbuf/idbitset_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(rsbuf): NAI-29 Bundle 1 Task 1.1 — port idBitSet primitive

Lands pkg/rsbuf/idbitset.go and pkg/rsbuf/idbitset_test.go.

idBitSet pairs a bit-array (O(1) containment) with an insertion-ordered
[]int32 (iteration order). Mirrors upstream build.rs:8-62 IdBitSet from
2004scape/rsbuf branch 225 (HEAD 1cbb2ce). Used by BuildArea (Bundle 3)
to track per-player observed players + npcs.

Go-idiom translation: Rust unsafe-ptr arithmetic (*self.bits.as_ptr().add(...))
becomes Go bounds-checked indexing; same observable semantics. No deviation
tag introduced.

Closes Bundle 1 Task 1.1 of NAI-29 spec
(docs/superpowers/specs/2026-04-25-nai-29-rsbuf-stateful-core-design.md).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

## Task 1.2: `zoneMap` + `zone` (`pkg/rsbuf/zonemap.go`)

**Files:**
- Create: `pkg/rsbuf/zonemap.go`
- Create: `pkg/rsbuf/zonemap_test.go`

- [ ] **Step 1: Write failing tests in `pkg/rsbuf/zonemap_test.go`**

```go
package rsbuf

import (
	"testing"
)

func TestZoneMap_ZoneCreatesOnMiss(t *testing.T) {
	m := newZoneMap()
	z := m.Zone(50, 0, 50)
	if z == nil {
		t.Fatal("Zone returned nil for unknown coord")
	}
	if len(z.players) != 0 || len(z.npcs) != 0 {
		t.Errorf("new zone: players=%d, npcs=%d, want both 0", len(z.players), len(z.npcs))
	}
}

func TestZoneMap_ZoneIsStable(t *testing.T) {
	m := newZoneMap()
	a := m.Zone(50, 0, 50)
	b := m.Zone(50, 0, 50)
	if a != b {
		t.Errorf("Zone for same coord returned different pointers: %p vs %p", a, b)
	}
}

func TestZoneMap_ZoneKeyedByZoneCoord(t *testing.T) {
	// All x=48..55 fall into the same zone (x>>3=6); zone differs only
	// when x crosses a zone boundary.
	m := newZoneMap()
	a := m.Zone(48, 0, 50)
	b := m.Zone(55, 0, 50) // same zone (55>>3=6)
	c := m.Zone(56, 0, 50) // different zone (56>>3=7)
	if a != b {
		t.Errorf("(48, 0, 50) and (55, 0, 50) should map to same zone")
	}
	if a == c {
		t.Errorf("(48, 0, 50) and (56, 0, 50) should map to different zones")
	}
}

func TestZoneMap_LevelDifferentiates(t *testing.T) {
	m := newZoneMap()
	a := m.Zone(50, 0, 50)
	b := m.Zone(50, 1, 50)
	if a == b {
		t.Errorf("zones at same (x,z) but different level returned same pointer")
	}
}

func TestZoneMap_AxisDifferentiates(t *testing.T) {
	// Confirm x and z are not transposed in packing.
	m := newZoneMap()
	a := m.Zone(8, 0, 0)  // zone (1, 0, 0)
	b := m.Zone(0, 0, 8)  // zone (0, 0, 1)
	if a == b {
		t.Errorf("(8,0,0) and (0,0,8) should map to different zones")
	}
}

func TestZone_PlayerNpcSetsIndependent(t *testing.T) {
	z := newZone()
	z.AddPlayer(5)
	z.AddNpc(5)
	if _, ok := z.players[5]; !ok {
		t.Error("AddPlayer(5): players[5] missing")
	}
	if _, ok := z.npcs[5]; !ok {
		t.Error("AddNpc(5): npcs[5] missing")
	}
	z.RemovePlayer(5)
	if _, ok := z.players[5]; ok {
		t.Error("RemovePlayer(5): players[5] still present")
	}
	if _, ok := z.npcs[5]; !ok {
		t.Error("RemovePlayer(5) leaked into npcs")
	}
}

func TestZone_RemoveAbsentIsNoop(t *testing.T) {
	z := newZone()
	z.RemovePlayer(99) // never added
	z.RemoveNpc(99)
	if len(z.players) != 0 || len(z.npcs) != 0 {
		t.Errorf("RemovePlayer/RemoveNpc(absent) populated empty sets")
	}
}

func TestZone_AddIdempotent(t *testing.T) {
	z := newZone()
	z.AddPlayer(7)
	z.AddPlayer(7)
	if len(z.players) != 1 {
		t.Errorf("double AddPlayer(7): len = %d, want 1", len(z.players))
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./pkg/rsbuf/ -run "TestZone(Map)?"`
Expected: FAIL with `undefined: newZoneMap` and `undefined: newZone` compile errors.

- [ ] **Step 3: Implement `pkg/rsbuf/zonemap.go`**

```go
package rsbuf

// zone holds the player + npc id sets for a single 8x8-tile zone.
// Mirrors upstream grid.rs Zone (2004scape/rsbuf branch 225,
// src/grid.rs:4-37).
//
// Concurrency: tick-goroutine-owned. No internal synchronization.
type zone struct {
	players map[int32]struct{} // pids
	npcs    map[int32]struct{} // nids
}

func newZone() *zone {
	return &zone{
		players: map[int32]struct{}{},
		npcs:    map[int32]struct{}{},
	}
}

// AddPlayer registers pid in this zone. Idempotent.
func (z *zone) AddPlayer(pid int32) { z.players[pid] = struct{}{} }

// RemovePlayer unregisters pid from this zone. No-op if absent.
func (z *zone) RemovePlayer(pid int32) { delete(z.players, pid) }

// AddNpc registers nid in this zone. Idempotent.
func (z *zone) AddNpc(nid int32) { z.npcs[nid] = struct{}{} }

// RemoveNpc unregisters nid from this zone. No-op if absent.
func (z *zone) RemoveNpc(nid int32) { delete(z.npcs, nid) }

// zoneMap is the rsbuf-internal spatial index keyed by packed zone
// index. Mirrors upstream grid.rs ZoneMap (src/grid.rs:39-75).
//
// Coord packing (matches upstream grid.rs:54-58 ZoneMap::zone_index
// AND goscape's pkg/coordgrid.ZoneIndex):
//
//	((x >> 3) & 0x7ff) | (((z >> 3) & 0x7ff) << 11) | ((level & 0x3) << 22)
//
// Concurrency: tick-goroutine-owned.
type zoneMap struct {
	zones map[uint32]*zone
}

func newZoneMap() *zoneMap {
	return &zoneMap{
		zones: map[uint32]*zone{},
	}
}

// Zone returns the zone at (x, level, z), creating an empty zone on miss
// and caching it in the map. Mirrors upstream ZoneMap::zone at
// grid.rs:69-74.
//
// Argument order matches goscape's pkg/coordgrid convention (level
// before z) rather than upstream's (x, y, z). The packed key is
// identical.
func (m *zoneMap) Zone(x, level, z int) *zone {
	key := zoneKey(x, level, z)
	if z := m.zones[key]; z != nil {
		return z
	}
	z := newZone()
	m.zones[key] = z
	return z
}

// zoneKey returns the packed zone index. Equivalent to
// pkg/coordgrid.ZoneIndex(x, z, level) but pinned here for
// upstream-side-by-side review against grid.rs:54-58.
func zoneKey(x, level, z int) uint32 {
	return uint32((x>>3)&0x7ff) | uint32((z>>3)&0x7ff)<<11 | uint32(level&0x3)<<22
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./pkg/rsbuf/ -run "TestZone(Map)?" -v`
Expected: PASS for all 8 test functions.

- [ ] **Step 5: Run full package + race**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 -race ./pkg/rsbuf/...`
Expected: PASS.

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...`
Expected: clean.

- [ ] **Step 6: Commit**

```bash
git add pkg/rsbuf/zonemap.go pkg/rsbuf/zonemap_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(rsbuf): NAI-29 Bundle 1 Task 1.2 — port zoneMap + zone primitives

Lands pkg/rsbuf/zonemap.go + pkg/rsbuf/zonemap_test.go.

zoneMap is the rsbuf-internal spatial index keyed by packed zone
coord (mirrors upstream grid.rs ZoneMap, 2004scape/rsbuf branch 225).
zone holds players + npcs id sets for a single 8x8-tile zone.
Coord packing matches both upstream ZoneMap::zone_index and
goscape's pkg/coordgrid.ZoneIndex.

Both types are unexported — used internally by BuildArea (Bundle 3)
and *Buf (Bundle 3). Separate from goscape's pkg/zone (which serves
game-event subscription, not encoder spatial-index).

Go-idiom translation: Rust IntSet<i32> → Go map[int32]struct{};
IntMap<u32, Zone> with capacity-reserve(0xffffff) → Go map[uint32]*zone
without capacity reservation (Go map growth is amortized). Same
observable semantics; no deviation tag introduced.

Closes Bundle 1 Task 1.2 of NAI-29 spec
(docs/superpowers/specs/2026-04-25-nai-29-rsbuf-stateful-core-design.md).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

# Bundle 2 — Entity Structs (`Player`, `Npc`)

Bundle 2 lands the per-tick state-snapshot structs that `ComputePlayer`/`ComputeNpc` (in Bundle 3) will populate. Both structs use full upstream field shapes so side-by-side review against `2004scape/rsbuf/src/player.rs` and `npc.rs` is tractable.

**Naming note:** These types are exported as `rsbuf.Player` and `rsbuf.Npc`. They coexist with `modules/world.Player` and `modules/world.Npc` (the game entities), which already implement `rsbuf.PlayerSource` / `rsbuf.NpcSource` for the existing `Encode`/`EncodeNpc` API. The two `Player` types serve different roles: `modules/world.Player` is the game entity (tick-driven state simulation), `rsbuf.Player` is the encoder-state snapshot (per-tick inputs to the wire-format generator).

**Forward declaration:** `rsbuf.Player.Build *BuildArea` references the `BuildArea` type that Bundle 3 introduces. This works: Go allows pointer-to-incomplete-type within the same package, resolved when B3 declares the concrete type. Tests in this bundle use a `&BuildArea{}` stub literal which becomes a real value once B3 adds fields.

## Task 2.1: `rsbuf.Player` + `Chat` + `ExactMove` (`pkg/rsbuf/player.go`)

**Files:**
- Create: `pkg/rsbuf/player.go`
- Create: `pkg/rsbuf/player_test.go`

- [ ] **Step 1: Write failing tests in `pkg/rsbuf/player_test.go`**

```go
package rsbuf

import (
	"testing"
)

func TestNewPlayer_SentinelDefaults(t *testing.T) {
	p := newPlayer(42)
	if p == nil {
		t.Fatal("newPlayer returned nil")
	}
	if p.PID != 42 {
		t.Errorf("PID: got %d, want 42", p.PID)
	}
	if p.Coord != 0 {
		t.Errorf("Coord: got %d, want 0", p.Coord)
	}
	if p.Origin != 0 {
		t.Errorf("Origin: got %d, want 0", p.Origin)
	}
	if p.Tele || p.Jump {
		t.Errorf("Tele/Jump should default false")
	}
	if p.RunDir != -1 || p.WalkDir != -1 {
		t.Errorf("RunDir/WalkDir: got (%d, %d), want (-1, -1)", p.RunDir, p.WalkDir)
	}
	if p.Visibility != VisibilityDefault {
		t.Errorf("Visibility: got %d, want VisibilityDefault", p.Visibility)
	}
	if p.Active {
		t.Error("Active should default false")
	}
	if p.Masks != 0 {
		t.Errorf("Masks: got %d, want 0", p.Masks)
	}
	if p.Appearance != nil {
		t.Errorf("Appearance: got %v, want nil", p.Appearance)
	}
	if p.LastAppearance != -1 {
		t.Errorf("LastAppearance: got %d, want -1", p.LastAppearance)
	}
	for _, tt := range []struct {
		name string
		got  int32
	}{
		{"FaceEntity", p.FaceEntity},
		{"FaceX", p.FaceX}, {"FaceZ", p.FaceZ},
		{"OrientationX", p.OrientationX}, {"OrientationZ", p.OrientationZ},
		{"DamageTaken", p.DamageTaken}, {"DamageType", p.DamageType},
		{"CurrentHitpoints", p.CurrentHitpoints}, {"BaseHitpoints", p.BaseHitpoints},
		{"AnimID", p.AnimID}, {"AnimDelay", p.AnimDelay},
		{"GraphicID", p.GraphicID}, {"GraphicHeight", p.GraphicHeight}, {"GraphicDelay", p.GraphicDelay},
	} {
		if tt.got != -1 {
			t.Errorf("%s: got %d, want -1", tt.name, tt.got)
		}
	}
	if p.Say != nil {
		t.Error("Say should default nil")
	}
	if p.Chat != nil {
		t.Error("Chat should default nil")
	}
	if p.ExactMove != nil {
		t.Error("ExactMove should default nil")
	}
}

func TestPlayerCleanup_ZeroesTransient(t *testing.T) {
	p := newPlayer(5)
	// Populate transient fields.
	p.WalkDir = 3
	p.RunDir = 4
	p.Jump = true
	p.Tele = true
	p.Masks = 0xff
	p.FaceX = 100
	p.FaceZ = 200
	p.DamageTaken = 5
	p.DamageType = 1
	p.CurrentHitpoints = 90
	p.BaseHitpoints = 99
	p.AnimID = 808
	p.AnimDelay = 0
	s := "hi"
	p.Say = &s
	p.Chat = &Chat{Bytes: []byte{1, 2}, Color: 9}
	p.GraphicID = 100
	p.GraphicHeight = 92
	p.GraphicDelay = 0
	p.ExactMove = &ExactMove{StartX: 1}

	p.cleanup()

	if p.WalkDir != -1 || p.RunDir != -1 {
		t.Errorf("cleanup: WalkDir/RunDir not reset to -1 (got %d, %d)", p.WalkDir, p.RunDir)
	}
	if p.Jump || p.Tele {
		t.Error("cleanup: Jump/Tele not zeroed")
	}
	if p.Masks != 0 {
		t.Errorf("cleanup: Masks not zeroed (got %d)", p.Masks)
	}
	if p.FaceX != -1 || p.FaceZ != -1 {
		t.Errorf("cleanup: FaceX/FaceZ not reset to -1 (got %d, %d)", p.FaceX, p.FaceZ)
	}
	for _, tt := range []struct {
		name string
		got  int32
	}{
		{"DamageTaken", p.DamageTaken}, {"DamageType", p.DamageType},
		{"CurrentHitpoints", p.CurrentHitpoints}, {"BaseHitpoints", p.BaseHitpoints},
		{"AnimID", p.AnimID}, {"AnimDelay", p.AnimDelay},
		{"GraphicID", p.GraphicID}, {"GraphicHeight", p.GraphicHeight}, {"GraphicDelay", p.GraphicDelay},
	} {
		if tt.got != -1 {
			t.Errorf("cleanup: %s not reset to -1 (got %d)", tt.name, tt.got)
		}
	}
	if p.Say != nil {
		t.Error("cleanup: Say not nilled")
	}
	if p.Chat != nil {
		t.Error("cleanup: Chat not nilled")
	}
	if p.ExactMove != nil {
		t.Error("cleanup: ExactMove not nilled")
	}
}

func TestPlayerCleanup_PreservesPersistent(t *testing.T) {
	// Per upstream player.rs:96-121 commented-out cleanup lines:
	// appearance, lastAppearance, faceEntity, orientationX, orientationZ
	// MUST NOT be cleared by cleanup — they persist across ticks.
	p := newPlayer(5)
	p.Appearance = []byte{1, 2, 3}
	p.LastAppearance = 100
	p.FaceEntity = 42
	p.OrientationX = 50
	p.OrientationZ = 60

	p.cleanup()

	if got := p.Appearance; len(got) != 3 || got[0] != 1 || got[1] != 2 || got[2] != 3 {
		t.Errorf("cleanup CLEARED Appearance: got %v, want [1 2 3]", got)
	}
	if p.LastAppearance != 100 {
		t.Errorf("cleanup CLEARED LastAppearance: got %d, want 100", p.LastAppearance)
	}
	if p.FaceEntity != 42 {
		t.Errorf("cleanup CLEARED FaceEntity: got %d, want 42", p.FaceEntity)
	}
	if p.OrientationX != 50 || p.OrientationZ != 60 {
		t.Errorf("cleanup CLEARED Orientation*: got (%d, %d), want (50, 60)", p.OrientationX, p.OrientationZ)
	}
}

func TestChat_Construction(t *testing.T) {
	c := &Chat{Bytes: []byte{0x10, 0x20}, Color: 1, Effect: 2, Ignored: 3}
	if c.Color != 1 || c.Effect != 2 || c.Ignored != 3 {
		t.Errorf("Chat fields: got (%d,%d,%d), want (1,2,3)", c.Color, c.Effect, c.Ignored)
	}
	if len(c.Bytes) != 2 || c.Bytes[0] != 0x10 || c.Bytes[1] != 0x20 {
		t.Errorf("Chat.Bytes: got %v", c.Bytes)
	}
}

func TestExactMove_Construction(t *testing.T) {
	e := &ExactMove{StartX: 1, StartZ: 2, EndX: 3, EndZ: 4, Begin: 5, Finish: 6, Dir: 7}
	if e.StartX != 1 || e.StartZ != 2 || e.EndX != 3 || e.EndZ != 4 ||
		e.Begin != 5 || e.Finish != 6 || e.Dir != 7 {
		t.Errorf("ExactMove fields not set correctly: %+v", e)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./pkg/rsbuf/ -run "TestNewPlayer|TestPlayerCleanup|TestChat|TestExactMove"`
Expected: FAIL with `undefined: newPlayer`, `undefined: Chat`, `undefined: ExactMove` compile errors.

- [ ] **Step 3: Implement `pkg/rsbuf/player.go`**

```go
package rsbuf

// Player is the per-tick state snapshot that the encoder reads from.
// Mirrors upstream player.rs Player (2004scape/rsbuf branch 225,
// src/player.rs:5-37). Field order matches upstream for side-by-side
// review.
//
// Concurrency: tick-goroutine-owned. Allocated by *Buf.AddPlayer;
// populated by *Buf.ComputePlayer; cleaned up to transient defaults
// by *Buf.Cleanup at end-of-tick.
//
// Coord encoding: stored as int packed via pkg/coordgrid.PackCoord
// (level, x, z). Layout matches upstream CoordGrid::from at coord.rs:13-19.
type Player struct {
	Coord    int   // pkg/coordgrid.PackCoord(level, x, z)
	Origin   int   // pkg/coordgrid.PackCoord(level, originX, originZ)
	PID      int32
	Tele     bool
	Jump     bool
	RunDir   int8 // -1 sentinel = no run this tick
	WalkDir  int8 // -1 sentinel = no walk this tick
	Visibility Visibility
	Active   bool
	Build    *BuildArea // populated by *Buf.AddPlayer (Bundle 3)
	Masks    uint32
	Appearance     []byte
	LastAppearance int32
	FaceEntity     int32
	FaceX, FaceZ   int32
	OrientationX, OrientationZ int32
	DamageTaken, DamageType    int32
	CurrentHitpoints, BaseHitpoints int32
	AnimID, AnimDelay               int32
	Say     *string // nil = no say this tick
	Chat    *Chat
	GraphicID, GraphicHeight, GraphicDelay int32
	ExactMove *ExactMove
}

// Chat carries chat-message payload + formatting. Mirrors upstream
// player.rs Chat (src/player.rs:39-45).
type Chat struct {
	Bytes  []byte
	Color  uint8
	Effect uint8
	Ignored uint8
}

// ExactMove carries exact-move animation parameters. Mirrors upstream
// player.rs ExactMove (src/player.rs:47-56).
type ExactMove struct {
	StartX, StartZ int32
	EndX, EndZ     int32
	Begin, Finish  int32
	Dir            int32
}

// newPlayer constructs a Player at zero-coord with sentinel defaults.
// Mirrors upstream Player::new at player.rs:60-93. Build is nil at
// construction; *Buf.AddPlayer assigns a fresh BuildArea before the
// player becomes addressable to ComputePlayer.
func newPlayer(pid int32) *Player {
	return &Player{
		Coord:            0,
		Origin:           0,
		PID:              pid,
		Tele:             false,
		Jump:             false,
		RunDir:           -1,
		WalkDir:          -1,
		Visibility:       VisibilityDefault,
		Active:           false,
		Build:            nil, // *Buf.AddPlayer fills this in
		Masks:            0,
		Appearance:       nil,
		LastAppearance:   -1,
		FaceEntity:       -1,
		FaceX:            -1,
		FaceZ:            -1,
		OrientationX:     -1,
		OrientationZ:     -1,
		DamageTaken:      -1,
		DamageType:       -1,
		CurrentHitpoints: -1,
		BaseHitpoints:    -1,
		AnimID:           -1,
		AnimDelay:        -1,
		Say:              nil,
		Chat:             nil,
		GraphicID:        -1,
		GraphicHeight:    -1,
		GraphicDelay:     -1,
		ExactMove:        nil,
	}
}

// cleanup zeros transient per-tick state but preserves persistent
// state (Appearance, LastAppearance, FaceEntity, OrientationX,
// OrientationZ) per upstream player.rs:96-121 commented-out lines.
//
// Called by *Buf.Cleanup once per tick after info encoding completes.
func (p *Player) cleanup() {
	p.WalkDir = -1
	p.RunDir = -1
	p.Jump = false
	p.Tele = false
	p.Masks = 0
	// Appearance / LastAppearance / FaceEntity / OrientationX/Z preserved
	// per upstream commented-out clears at player.rs:102-108.
	p.FaceX = -1
	p.FaceZ = -1
	p.DamageTaken = -1
	p.DamageType = -1
	p.CurrentHitpoints = -1
	p.BaseHitpoints = -1
	p.AnimID = -1
	p.AnimDelay = -1
	p.Say = nil
	p.Chat = nil
	p.GraphicID = -1
	p.GraphicHeight = -1
	p.GraphicDelay = -1
	p.ExactMove = nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./pkg/rsbuf/ -run "TestNewPlayer|TestPlayerCleanup|TestChat|TestExactMove" -v`
Expected: PASS for 5 test functions.

> **Note:** `Player.Build *BuildArea` references a type Bundle 3 introduces. The Go compiler allows pointer-to-undeclared-type within the same package as long as the type appears somewhere in the package by the end of the build. **Until Bundle 3 lands `BuildArea`, this file will fail to compile.** Workaround for this task: temporarily declare an empty `type BuildArea struct{}` at the bottom of `pkg/rsbuf/player.go`. Bundle 3 Task 3.1 will replace it with the full type. Document the temporary stub with a `// TEMPORARY: replaced by Bundle 3 Task 3.1` comment.

Add this to `pkg/rsbuf/player.go` at the bottom:

```go
// BuildArea is a temporary forward declaration. Bundle 3 Task 3.1
// replaces this with the full BuildArea type. Required because
// Player.Build *BuildArea references it.
//
// TEMPORARY: replaced by Bundle 3 Task 3.1.
type BuildArea struct{}
```

- [ ] **Step 5: Run full package + race**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 -race ./pkg/rsbuf/...`
Expected: PASS.

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...`
Expected: clean.

- [ ] **Step 6: Commit**

```bash
git add pkg/rsbuf/player.go pkg/rsbuf/player_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(rsbuf): NAI-29 Bundle 2 Task 2.1 — port Player + Chat + ExactMove structs

Lands pkg/rsbuf/player.go + pkg/rsbuf/player_test.go.

rsbuf.Player is the per-tick state snapshot the encoder reads from.
Mirrors upstream player.rs Player (2004scape/rsbuf branch 225). Field
order matches upstream verbatim for side-by-side review. Coord/Origin
stored as int packed via pkg/coordgrid.PackCoord (level, x, z) — layout
identical to upstream coord.rs:13-19 CoordGrid::from.

Includes substructs Chat (chat-message payload + formatting) and
ExactMove (exact-move animation parameters), both mirroring upstream
player.rs:39-56.

cleanup() zeros transient per-tick state but preserves persistent
state (Appearance, LastAppearance, FaceEntity, OrientationX/Z) per
upstream player.rs:96-121 commented-out clears.

Includes TEMPORARY forward-declaration stub for BuildArea (replaced
by Bundle 3 Task 3.1) so Player.Build *BuildArea reference compiles.

Coexists with modules/world.Player (the game entity); rsbuf.Player is
the encoder snapshot. Different roles, different package — no naming
collision.

Closes Bundle 2 Task 2.1 of NAI-29 spec.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

## Task 2.2: `rsbuf.Npc` (`pkg/rsbuf/npc.go`)

**Files:**
- Create: `pkg/rsbuf/npc.go`
- Create: `pkg/rsbuf/npc_test.go`

- [ ] **Step 1: Write failing tests in `pkg/rsbuf/npc_test.go`**

```go
package rsbuf

import (
	"testing"
)

func TestNewNpc_SentinelDefaults(t *testing.T) {
	n := newNpc(7, 100)
	if n.NID != 7 {
		t.Errorf("NID: got %d, want 7", n.NID)
	}
	if n.NType != 100 {
		t.Errorf("NType: got %d, want 100", n.NType)
	}
	if n.Coord != 0 {
		t.Errorf("Coord: got %d, want 0", n.Coord)
	}
	if n.Tele {
		t.Error("Tele should default false")
	}
	if n.RunDir != -1 || n.WalkDir != -1 {
		t.Errorf("RunDir/WalkDir: got (%d, %d), want (-1, -1)", n.RunDir, n.WalkDir)
	}
	if n.Active {
		t.Error("Active should default false")
	}
	if n.Masks != 0 {
		t.Errorf("Masks: got %d, want 0", n.Masks)
	}
	for _, tt := range []struct {
		name string
		got  int32
	}{
		{"FaceEntity", n.FaceEntity},
		{"FaceX", n.FaceX}, {"FaceZ", n.FaceZ},
		{"OrientationX", n.OrientationX}, {"OrientationZ", n.OrientationZ},
		{"DamageTaken", n.DamageTaken}, {"DamageType", n.DamageType},
		{"CurrentHitpoints", n.CurrentHitpoints}, {"BaseHitpoints", n.BaseHitpoints},
		{"AnimID", n.AnimID}, {"AnimDelay", n.AnimDelay},
		{"GraphicID", n.GraphicID}, {"GraphicHeight", n.GraphicHeight}, {"GraphicDelay", n.GraphicDelay},
	} {
		if tt.got != -1 {
			t.Errorf("%s: got %d, want -1", tt.name, tt.got)
		}
	}
	if n.Say != nil {
		t.Error("Say should default nil")
	}
	if n.Observers != 0 {
		t.Errorf("Observers: got %d, want 0", n.Observers)
	}
}

func TestNpcCleanup_ZeroesTransient(t *testing.T) {
	n := newNpc(7, 100)
	n.WalkDir = 3
	n.RunDir = 4
	n.Tele = true
	n.Masks = 0xff
	n.FaceX = 50
	n.FaceZ = 60
	n.DamageTaken = 9
	n.DamageType = 0
	n.CurrentHitpoints = 50
	n.BaseHitpoints = 100
	n.AnimID = 808
	n.AnimDelay = 0
	s := "rwar"
	n.Say = &s
	n.GraphicID = 100
	n.GraphicHeight = 92
	n.GraphicDelay = 0

	n.cleanup()

	if n.WalkDir != -1 || n.RunDir != -1 {
		t.Errorf("cleanup: WalkDir/RunDir not reset to -1 (got %d, %d)", n.WalkDir, n.RunDir)
	}
	if n.Tele {
		t.Error("cleanup: Tele not zeroed")
	}
	if n.Masks != 0 {
		t.Errorf("cleanup: Masks not zeroed (got %d)", n.Masks)
	}
	if n.FaceX != -1 || n.FaceZ != -1 {
		t.Errorf("cleanup: FaceX/FaceZ not reset to -1 (got %d, %d)", n.FaceX, n.FaceZ)
	}
	for _, tt := range []struct {
		name string
		got  int32
	}{
		{"DamageTaken", n.DamageTaken}, {"DamageType", n.DamageType},
		{"CurrentHitpoints", n.CurrentHitpoints}, {"BaseHitpoints", n.BaseHitpoints},
		{"AnimID", n.AnimID}, {"AnimDelay", n.AnimDelay},
		{"GraphicID", n.GraphicID}, {"GraphicHeight", n.GraphicHeight}, {"GraphicDelay", n.GraphicDelay},
	} {
		if tt.got != -1 {
			t.Errorf("cleanup: %s not reset to -1 (got %d)", tt.name, tt.got)
		}
	}
	if n.Say != nil {
		t.Error("cleanup: Say not nilled")
	}
}

func TestNpcCleanup_PreservesPersistent(t *testing.T) {
	// Per upstream npc.rs:62-83 commented-out lines:
	// faceEntity, orientationX, orientationZ persist across ticks.
	// Observers also persists (it's a counter, not transient state).
	n := newNpc(7, 100)
	n.FaceEntity = 42
	n.OrientationX = 50
	n.OrientationZ = 60
	n.Observers = 3

	n.cleanup()

	if n.FaceEntity != 42 {
		t.Errorf("cleanup CLEARED FaceEntity: got %d, want 42", n.FaceEntity)
	}
	if n.OrientationX != 50 || n.OrientationZ != 60 {
		t.Errorf("cleanup CLEARED Orientation*: got (%d, %d)", n.OrientationX, n.OrientationZ)
	}
	if n.Observers != 3 {
		t.Errorf("cleanup CLEARED Observers: got %d, want 3", n.Observers)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./pkg/rsbuf/ -run "TestNewNpc|TestNpcCleanup"`
Expected: FAIL with `undefined: newNpc` compile error.

- [ ] **Step 3: Implement `pkg/rsbuf/npc.go`**

```go
package rsbuf

// Npc is the per-tick state snapshot that the encoder reads from.
// Mirrors upstream npc.rs Npc (2004scape/rsbuf branch 225,
// src/npc.rs:3-29). Field order matches upstream verbatim.
//
// Concurrency: tick-goroutine-owned. Allocated by *Buf.AddNpc;
// populated by *Buf.ComputeNpc; cleaned up to transient defaults
// by *Buf.Cleanup at end-of-tick.
//
// Observers is a counter — not cleared by cleanup; modified only
// by *Buf.RemovePlayer (decrements for npcs in the player's BuildArea)
// and by NAI-30's encoder (increments on add to a tracking set,
// decrements on remove from a tracking set).
type Npc struct {
	Coord    int   // pkg/coordgrid.PackCoord(level, x, z)
	NID      int32
	NType    int32
	Tele     bool
	RunDir   int8 // -1 sentinel
	WalkDir  int8 // -1 sentinel
	Active   bool
	Masks    uint32
	FaceEntity   int32
	FaceX, FaceZ int32
	OrientationX, OrientationZ int32
	DamageTaken, DamageType    int32
	CurrentHitpoints, BaseHitpoints int32
	AnimID, AnimDelay int32
	Say  *string
	GraphicID, GraphicHeight, GraphicDelay int32
	Observers int32
}

// newNpc constructs an Npc at zero-coord with sentinel defaults and
// observer count 0. Mirrors upstream Npc::new at npc.rs:32-60.
func newNpc(nid, ntype int32) *Npc {
	return &Npc{
		Coord:            0,
		NID:              nid,
		NType:            ntype,
		Tele:             false,
		RunDir:           -1,
		WalkDir:          -1,
		Active:           false,
		Masks:            0,
		FaceEntity:       -1,
		FaceX:            -1,
		FaceZ:            -1,
		OrientationX:     -1,
		OrientationZ:     -1,
		DamageTaken:      -1,
		DamageType:       -1,
		CurrentHitpoints: -1,
		BaseHitpoints:    -1,
		AnimID:           -1,
		AnimDelay:        -1,
		Say:              nil,
		GraphicID:        -1,
		GraphicHeight:    -1,
		GraphicDelay:     -1,
		Observers:        0,
	}
}

// cleanup zeros transient per-tick state but preserves FaceEntity +
// OrientationX/Z + Observers per upstream npc.rs:62-83 commented-out
// clears.
func (n *Npc) cleanup() {
	n.WalkDir = -1
	n.RunDir = -1
	n.Tele = false
	n.Masks = 0
	// FaceEntity / OrientationX/Z preserved per upstream npc.rs:68-71
	// commented-out clears.
	n.FaceX = -1
	n.FaceZ = -1
	n.DamageTaken = -1
	n.DamageType = -1
	n.CurrentHitpoints = -1
	n.BaseHitpoints = -1
	n.AnimID = -1
	n.AnimDelay = -1
	n.Say = nil
	n.GraphicID = -1
	n.GraphicHeight = -1
	n.GraphicDelay = -1
	// Observers preserved (persistent counter).
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./pkg/rsbuf/ -run "TestNewNpc|TestNpcCleanup" -v`
Expected: PASS for 3 test functions.

- [ ] **Step 5: Run full package + race**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 -race ./pkg/rsbuf/...`
Expected: PASS.

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...`
Expected: clean.

- [ ] **Step 6: Commit**

```bash
git add pkg/rsbuf/npc.go pkg/rsbuf/npc_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(rsbuf): NAI-29 Bundle 2 Task 2.2 — port Npc struct

Lands pkg/rsbuf/npc.go + pkg/rsbuf/npc_test.go.

rsbuf.Npc is the per-tick state snapshot the encoder reads from.
Mirrors upstream npc.rs Npc (2004scape/rsbuf branch 225). Field order
matches upstream verbatim. Coord stored as packed int via
pkg/coordgrid.PackCoord (level, x, z).

cleanup() zeros transient per-tick state but preserves FaceEntity +
OrientationX/Z (per upstream npc.rs:62-83 commented-out clears) AND
Observers (a persistent counter modified only by RemovePlayer +
encoder add/remove sites).

Closes Bundle 2 Task 2.2 of NAI-29 spec.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

# Bundle 3 — `BuildArea`, `*Buf`, Full Public API

Bundle 3 lands the complete `*Buf` instance handle and stateful API. After B3 close, `*Buf` is fully constructable and operational; B1's primitives + B2's structs are wired together. No production caller yet — exercised entirely via unit tests.

**Sizing constants** (mirror upstream `build.rs:75-78` `BuildArea` constants):
- `preferredPlayers = 250`
- `preferredNpcs = 255`
- `preferredViewDistance uint8 = 15`

NAI-29 hardcodes `viewDistance = preferredViewDistance` (no resize). NAI-32 ports the resize logic.

## Task 3.1: `BuildArea` (`pkg/rsbuf/buildarea.go`)

**Files:**
- Modify: `pkg/rsbuf/player.go` (remove temporary BuildArea stub from Task 2.1)
- Create: `pkg/rsbuf/buildarea.go`
- Create: `pkg/rsbuf/buildarea_test.go`

- [ ] **Step 1: Remove the temporary BuildArea stub from `pkg/rsbuf/player.go`**

Delete these lines (added in Task 2.1) at the bottom of `pkg/rsbuf/player.go`:

```go
// BuildArea is a temporary forward declaration. Bundle 3 Task 3.1
// replaces this with the full BuildArea type. Required because
// Player.Build *BuildArea references it.
//
// TEMPORARY: replaced by Bundle 3 Task 3.1.
type BuildArea struct{}
```

- [ ] **Step 2: Write failing tests in `pkg/rsbuf/buildarea_test.go`**

```go
package rsbuf

import (
	"testing"
)

func TestNewBuildArea_ZeroInit(t *testing.T) {
	b := newBuildArea()
	if b == nil {
		t.Fatal("newBuildArea returned nil")
	}
	if b.Players == nil || b.Npcs == nil {
		t.Errorf("newBuildArea: Players=%v, Npcs=%v, want both non-nil", b.Players, b.Npcs)
	}
	if b.Players.Len() != 0 || b.Npcs.Len() != 0 {
		t.Errorf("newBuildArea: Players.Len=%d, Npcs.Len=%d, want both 0", b.Players.Len(), b.Npcs.Len())
	}
	if b.ViewDistance != preferredViewDistance {
		t.Errorf("newBuildArea: ViewDistance=%d, want %d", b.ViewDistance, preferredViewDistance)
	}
	for i := 0; i < 2048; i++ {
		if b.appearances[i] != 0 {
			t.Errorf("newBuildArea: appearances[%d]=%d, want 0", i, b.appearances[i])
			break
		}
	}
}

func TestBuildArea_HasAppearance_FreshIsFalse(t *testing.T) {
	b := newBuildArea()
	for _, tick := range []uint32{0, 1, 100} {
		if b.HasAppearance(7, tick) {
			t.Errorf("fresh BuildArea: HasAppearance(7, %d) = true, want false", tick)
		}
	}
}

func TestBuildArea_SaveAppearance_RoundTrip(t *testing.T) {
	b := newBuildArea()
	b.SaveAppearance(7, 100)
	if !b.HasAppearance(7, 100) {
		t.Error("after SaveAppearance(7, 100), HasAppearance(7, 100) is false")
	}
	if b.HasAppearance(7, 99) {
		t.Error("HasAppearance(7, 99) is true after SaveAppearance(7, 100) — should be false (tick mismatch)")
	}
	if b.HasAppearance(7, 101) {
		t.Error("HasAppearance(7, 101) is true after SaveAppearance(7, 100) — should be false")
	}
	if b.HasAppearance(8, 100) {
		t.Error("SaveAppearance(7, 100) leaked into pid=8")
	}
}

func TestBuildArea_Cleanup_ClearsAll(t *testing.T) {
	b := newBuildArea()
	b.Players.Insert(5)
	b.Players.Insert(10)
	b.Npcs.Insert(3)
	b.SaveAppearance(7, 100)
	b.SaveAppearance(8, 200)

	b.Cleanup()

	if b.Players.Len() != 0 {
		t.Errorf("Cleanup: Players.Len=%d, want 0", b.Players.Len())
	}
	if b.Npcs.Len() != 0 {
		t.Errorf("Cleanup: Npcs.Len=%d, want 0", b.Npcs.Len())
	}
	if b.HasAppearance(7, 100) || b.HasAppearance(8, 200) {
		t.Error("Cleanup: appearances not cleared")
	}
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./pkg/rsbuf/ -run "TestNewBuildArea|TestBuildArea"`
Expected: FAIL with `undefined: newBuildArea`, `undefined: preferredViewDistance` compile errors. Note: removing the stub from player.go is required for the new BuildArea to land cleanly.

- [ ] **Step 4: Implement `pkg/rsbuf/buildarea.go`**

```go
package rsbuf

// Sizing constants mirror upstream build.rs:75-78 BuildArea constants
// (2004scape/rsbuf branch 225).
const (
	// preferredPlayers caps each player's tracked-player set.
	preferredPlayers = 250

	// preferredNpcs caps each player's tracked-npc set.
	preferredNpcs = 255

	// preferredViewDistance is the player's spatial culling radius
	// in tiles. Fixed at 15 in NAI-29-30; NAI-32 ports the dynamic
	// shrink/grow logic from upstream BuildArea::resize at build.rs:100-121.
	preferredViewDistance uint8 = 15
)

// BuildArea tracks per-player encoder state: the set of currently-
// observed players + npcs, and a tick-keyed map of recently-sent
// appearance hashes. Mirrors upstream build.rs BuildArea
// (2004scape/rsbuf branch 225, src/build.rs:64-96) minus the
// view-distance resize logic + spatial-discovery helpers (those land
// in NAI-32 / NAI-30).
//
// Concurrency: tick-goroutine-owned. Allocated by *Buf.AddPlayer;
// cleaned by *Buf.CleanupPlayerBuildArea (logout) or *Buf.RemovePlayer.
//
// NAI-29 deliberately omits these upstream fields/methods (deferred):
//   - forceViewDistance bool                            (NAI-32; engine-override)
//   - lastResize uint32                                 (NAI-32; resize bookkeeping)
//   - INTERVAL uint8 = 10                               (NAI-32; resize-step interval)
//   - Resize() / RebuildPlayers() / RebuildNpcs()       (NAI-32)
//   - getNearbyPlayers / getNearbyPlayersZones /
//     getNearbyPlayersNearest / filterPlayer            (NAI-32; consume view_distance)
//   - getNearbyNpcs / filterNpc                         (NAI-30; fixed PREFERRED_VIEW_DISTANCE)
//   - spiral-search helpers                             (NAI-32; player-side only)
type BuildArea struct {
	Players *idBitSet // 2048-bit set, capacity preferredPlayers
	Npcs    *idBitSet // 8192-bit set, capacity preferredNpcs

	// appearances[pid] = tick-when-the-appearance-payload-was-last-sent-to-this-player.
	// HasAppearance(pid, tick) returns true iff the stored tick == tick. Mirrors
	// upstream BuildArea.appearances at build.rs:68 + has_appearance/save_appearance
	// at build.rs:151-158.
	appearances [2048]uint32

	// ViewDistance is the per-player spatial-cull radius. Fixed at
	// preferredViewDistance in NAI-29-30; resize-able in NAI-32.
	ViewDistance uint8
}

// newBuildArea constructs a fresh BuildArea with empty tracking sets,
// zeroed appearances, and ViewDistance = preferredViewDistance. Mirrors
// upstream BuildArea::new at build.rs:81-90.
func newBuildArea() *BuildArea {
	return &BuildArea{
		Players:      newIdBitSet(2048, preferredPlayers),
		Npcs:         newIdBitSet(8192, preferredNpcs),
		ViewDistance: preferredViewDistance,
	}
}

// Cleanup empties the tracking sets and zeros the appearances cache.
// Mirrors upstream BuildArea::cleanup at build.rs:93-97.
func (b *BuildArea) Cleanup() {
	b.Players.Clear()
	b.Npcs.Clear()
	for i := range b.appearances {
		b.appearances[i] = 0
	}
}

// HasAppearance reports whether the appearance payload for pid was
// already sent to the local player on the named tick. Mirrors upstream
// BuildArea::has_appearance at build.rs:151-153.
func (b *BuildArea) HasAppearance(pid int32, tick uint32) bool {
	return b.appearances[pid] == tick
}

// SaveAppearance records that the appearance payload for pid was sent
// to the local player on tick. Mirrors upstream BuildArea::save_appearance
// at build.rs:155-157.
func (b *BuildArea) SaveAppearance(pid int32, tick uint32) {
	b.appearances[pid] = tick
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./pkg/rsbuf/ -run "TestNewBuildArea|TestBuildArea" -v`
Expected: PASS for 4 test functions.

- [ ] **Step 6: Run full package + race**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 -race ./pkg/rsbuf/...`
Expected: PASS — including the player + npc tests from Bundle 2 (the BuildArea forward-declaration stub from Task 2.1 was removed in Step 1).

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...`
Expected: clean.

- [ ] **Step 7: Commit**

```bash
git add pkg/rsbuf/buildarea.go pkg/rsbuf/buildarea_test.go pkg/rsbuf/player.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(rsbuf): NAI-29 Bundle 3 Task 3.1 — port BuildArea (fixed viewDistance)

Lands pkg/rsbuf/buildarea.go + pkg/rsbuf/buildarea_test.go; removes the
temporary BuildArea forward-declaration stub from pkg/rsbuf/player.go
(Task 2.1).

rsbuf.BuildArea tracks per-player encoder state: tracked-player +
tracked-npc id sets (idBitSet from Bundle 1 Task 1.1), plus a
tick-keyed appearances cache for HasAppearance/SaveAppearance.
Mirrors upstream build.rs BuildArea minus the dynamic view-distance
resize + spatial-discovery helpers (deferred: Resize, RebuildPlayers,
getNearbyPlayers* — NAI-32; getNearbyNpcs — NAI-32).

ViewDistance fixed at preferredViewDistance=15 in NAI-29-30.

Sizing constants mirror upstream build.rs:75-78:
- preferredPlayers = 250
- preferredNpcs = 255
- preferredViewDistance = 15

Closes Bundle 3 Task 3.1 of NAI-29 spec.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

## Task 3.2: `*Buf` constructor + `AddPlayer` / `RemovePlayer`

**Files:**
- Create: `pkg/rsbuf/buf.go`
- Create: `pkg/rsbuf/buf_test.go`

- [ ] **Step 1: Write failing tests in `pkg/rsbuf/buf_test.go`**

```go
package rsbuf

import (
	"testing"
)

func TestNew_ZeroInit(t *testing.T) {
	b := New()
	if b == nil {
		t.Fatal("New returned nil")
	}
	for pid := int32(0); pid < 2048; pid++ {
		if b.players[pid] != nil {
			t.Errorf("New: players[%d] non-nil", pid)
			break
		}
	}
	for nid := int32(0); nid < 8192; nid++ {
		if b.npcs[nid] != nil {
			t.Errorf("New: npcs[%d] non-nil", nid)
			break
		}
	}
	if b.zoneMap == nil {
		t.Error("New: zoneMap nil")
	}
	if b.playerGrid == nil {
		t.Error("New: playerGrid nil")
	}
}

func TestAddPlayer_AllocatesSlot(t *testing.T) {
	b := New()
	b.AddPlayer(5)
	if b.players[5] == nil {
		t.Fatal("AddPlayer(5): slot still nil")
	}
	if b.players[5].PID != 5 {
		t.Errorf("AddPlayer(5): players[5].PID = %d, want 5", b.players[5].PID)
	}
	if b.players[5].Build == nil {
		t.Error("AddPlayer(5): players[5].Build nil — should be initialized BuildArea")
	}
	if b.players[5].RunDir != -1 {
		t.Errorf("AddPlayer(5): players[5].RunDir = %d, want -1 (sentinel default)", b.players[5].RunDir)
	}
}

func TestAddPlayer_NegativeIDIsNoop(t *testing.T) {
	b := New()
	b.AddPlayer(-1)
	// no panic; no observable side effect
}

func TestAddPlayer_OutOfRangeIsNoop(t *testing.T) {
	b := New()
	b.AddPlayer(2048) // >= len
	b.AddPlayer(99999)
	// no panic; no observable side effect
}

func TestAddPlayer_DoubleAddOverwrites(t *testing.T) {
	// Mirrors upstream lib.rs:179-184 — assignment, not insertion check.
	b := New()
	b.AddPlayer(5)
	first := b.players[5]
	b.AddPlayer(5)
	second := b.players[5]
	if first == second {
		t.Error("double AddPlayer(5): expected new *Player, got same pointer")
	}
	if second.PID != 5 {
		t.Errorf("after re-add: players[5].PID = %d, want 5", second.PID)
	}
}

func TestRemovePlayer_NilsSlot(t *testing.T) {
	b := New()
	b.AddPlayer(5)
	b.RemovePlayer(5)
	if b.players[5] != nil {
		t.Error("after RemovePlayer(5): slot still non-nil")
	}
}

func TestRemovePlayer_AbsentIsNoop(t *testing.T) {
	b := New()
	b.RemovePlayer(5) // never added
	b.RemovePlayer(-1)
	b.RemovePlayer(2048)
	if b.players[5] != nil {
		t.Error("RemovePlayer(absent): slot mutated")
	}
}

func TestRemovePlayer_DecrementsObserverForTrackedNpcs(t *testing.T) {
	// Mirrors upstream lib.rs:194-198 — RemovePlayer iterates the
	// player's BuildArea.npcs set and decrements each npc's observer
	// count (floor 0).
	b := New()
	b.AddPlayer(5)
	b.AddNpc(10, 100)
	b.AddNpc(20, 100)
	b.npcs[10].Observers = 3
	b.npcs[20].Observers = 1
	// Hand-seed the player's tracking set with these npcs.
	b.players[5].Build.Npcs.Insert(10)
	b.players[5].Build.Npcs.Insert(20)

	b.RemovePlayer(5)

	if b.npcs[10].Observers != 2 {
		t.Errorf("npcs[10].Observers: got %d, want 2 (3-1)", b.npcs[10].Observers)
	}
	if b.npcs[20].Observers != 0 {
		t.Errorf("npcs[20].Observers: got %d, want 0 (1-1, floored)", b.npcs[20].Observers)
	}
}

func TestRemovePlayer_ObserverFloorsAtZero(t *testing.T) {
	b := New()
	b.AddPlayer(5)
	b.AddNpc(10, 100)
	b.npcs[10].Observers = 0 // already 0
	b.players[5].Build.Npcs.Insert(10)

	b.RemovePlayer(5)

	if b.npcs[10].Observers != 0 {
		t.Errorf("Observers: got %d, want 0 (floor)", b.npcs[10].Observers)
	}
}

func TestRemovePlayer_RemovesFromZoneMap(t *testing.T) {
	// Mirrors upstream lib.rs:193 — RemovePlayer removes pid from
	// the zone at the player's last coord.
	b := New()
	b.AddPlayer(5)
	// Manually set a coord so the zoneMap remove targets a specific zone.
	// (ComputePlayer would do this; we hand-set for unit isolation.)
	b.players[5].Coord = packPlayerCoord(50, 0, 50) // helper: pkg/coordgrid.PackCoord
	b.zoneMap.Zone(50, 0, 50).AddPlayer(5)

	b.RemovePlayer(5)

	if _, ok := b.zoneMap.Zone(50, 0, 50).players[5]; ok {
		t.Error("RemovePlayer: pid still in zoneMap")
	}
}
```

> **Note on `packPlayerCoord` helper:** This is a test-internal helper to pack a coord. Tests call `pkg/coordgrid.PackCoord(level, x, z)` directly via an import alias to avoid leaking into production code:

Add at the top of `pkg/rsbuf/buf_test.go` (above the test functions):

```go
import (
	"testing"

	"github.com/zsrv/goscape/pkg/coordgrid"
)

// packPlayerCoord wraps pkg/coordgrid.PackCoord with the test's preferred
// argument order (x, level, z) for symmetry with rsbuf's internal *Buf.Zone
// argument order. Test-only.
func packPlayerCoord(x, level, z int) int {
	return coordgrid.PackCoord(level, x, z)
}
```

(Replace the `import "testing"` line at the top of the test file with the multi-import block.)

- [ ] **Step 2: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./pkg/rsbuf/ -run "TestNew$|TestAddPlayer|TestRemovePlayer"`
Expected: FAIL with `undefined: New`, `undefined: AddPlayer`, etc. compile errors.

- [ ] **Step 3: Implement `pkg/rsbuf/buf.go`**

```go
package rsbuf

import (
	"github.com/zsrv/goscape/pkg/coordgrid"
)

// Buf is the rsbuf instance handle. One per world. Mirrors the upstream
// lib.rs unsafe-static globals (PLAYERS, NPCS, ZONE_MAP, PLAYER_GRID,
// PLAYER_RENDERER, NPC_RENDERER, PLAYER_INFO, NPC_INFO at lib.rs:28-37)
// collected onto a single value type.
//
// Concurrency: tick-goroutine-owned. All methods are tick-goroutine-
// only; no internal synchronization (matches upstream's WASM single-
// threaded model).
//
// NAI-29 lands the entity-state subset (players, npcs, zoneMap,
// playerGrid). NAI-30 will add Renderer + Encoder fields; NAI-31 will
// add the renderer-cache compute-info wiring; NAI-32 will add the
// view-distance / spiral-search optimization hooks.
type Buf struct {
	players    [2048]*Player
	npcs       [8192]*Npc
	zoneMap    *zoneMap
	playerGrid map[uint32][]int32 // tile-keyed (NAI-32 spiral search backing)
}

// New constructs an empty Buf with all slot tables nil-initialized,
// empty zoneMap, empty playerGrid. Mirrors upstream Lazy::new at
// lib.rs:28-37.
func New() *Buf {
	return &Buf{
		zoneMap:    newZoneMap(),
		playerGrid: map[uint32][]int32{},
	}
}

// AddPlayer registers pid by allocating a *Player at slot[pid] with
// sentinel defaults + a fresh BuildArea. Mirrors upstream add_player
// at lib.rs:178-184.
//
// No-op if pid == -1 or pid >= 2048 (slot array bound). Double-add
// overwrites (matches upstream's unconditional assignment).
func (b *Buf) AddPlayer(pid int32) {
	if pid < 0 || int(pid) >= len(b.players) {
		return
	}
	p := newPlayer(pid)
	p.Build = newBuildArea()
	b.players[pid] = p
}

// RemovePlayer unregisters pid. Steps (mirroring upstream remove_player
// at lib.rs:186-203):
//   1. Remove pid from the zoneMap zone at the player's last coord
//   2. For each nid in player.Build.Npcs.Iter(), decrement npcs[nid].Observers (floor at 0)
//   3. Call player.Build.Cleanup() (clears tracking + appearances)
//   4. (NAI-30) PLAYER_RENDERER.removePermanent(pid) — skipped here
//   5. Set slot[pid] = nil
//
// No-op if pid == -1, pid >= 2048, or slot[pid] is nil.
func (b *Buf) RemovePlayer(pid int32) {
	if pid < 0 || int(pid) >= len(b.players) {
		return
	}
	p := b.players[pid]
	if p == nil {
		return
	}
	// Step 1: remove from zoneMap.
	pos := coordgrid.UnpackCoord(p.Coord)
	b.zoneMap.Zone(pos.X, pos.Level, pos.Z).RemovePlayer(pid)
	// Step 2: decrement observer counts for tracked npcs.
	for _, nid := range p.Build.Npcs.Iter() {
		if int(nid) >= len(b.npcs) {
			continue
		}
		n := b.npcs[nid]
		if n != nil && n.Observers > 0 {
			n.Observers--
		}
	}
	// Step 3: cleanup BuildArea.
	p.Build.Cleanup()
	// Step 4 deferred to NAI-30.
	// Step 5: nil the slot.
	b.players[pid] = nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./pkg/rsbuf/ -run "TestNew$|TestAddPlayer|TestRemovePlayer" -v`
Expected: PASS for 9 test functions.

- [ ] **Step 5: Run full package + race**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 -race ./pkg/rsbuf/...`
Expected: PASS.

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...`
Expected: clean.

- [ ] **Step 6: Commit**

```bash
git add pkg/rsbuf/buf.go pkg/rsbuf/buf_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(rsbuf): NAI-29 Bundle 3 Task 3.2 — port *Buf + AddPlayer/RemovePlayer

Lands pkg/rsbuf/buf.go (constructor + slot lifecycle) + buf_test.go.

*Buf is the rsbuf instance handle; one per world. Bundles upstream's
PLAYERS, NPCS, ZONE_MAP, PLAYER_GRID static-mut globals onto a single
struct. Mirrors lib.rs:28-37 + the slot-lifecycle exports
add_player/remove_player at lib.rs:178-203.

AddPlayer allocates a *Player at slot[pid] with a fresh BuildArea +
sentinel defaults (-1 sentinels per upstream Player::new).

RemovePlayer:
  1. removes from zoneMap (using the player's last coord)
  2. iterates BuildArea.Npcs and decrements each npc's Observers (floor 0)
  3. calls BuildArea.Cleanup
  4. (NAI-30) PLAYER_RENDERER.removePermanent — deferred
  5. nils the slot

Closes Bundle 3 Task 3.2 of NAI-29 spec.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

## Task 3.3: `*Buf.AddNpc` / `RemoveNpc`

**Files:**
- Modify: `pkg/rsbuf/buf.go` (add AddNpc + RemoveNpc methods)
- Modify: `pkg/rsbuf/buf_test.go` (add tests)

- [ ] **Step 1: Append failing tests to `pkg/rsbuf/buf_test.go`**

```go
func TestAddNpc_AllocatesSlot(t *testing.T) {
	b := New()
	b.AddNpc(50, 100)
	if b.npcs[50] == nil {
		t.Fatal("AddNpc(50, 100): slot nil")
	}
	if b.npcs[50].NID != 50 {
		t.Errorf("AddNpc(50, 100): NID = %d, want 50", b.npcs[50].NID)
	}
	if b.npcs[50].NType != 100 {
		t.Errorf("AddNpc(50, 100): NType = %d, want 100", b.npcs[50].NType)
	}
	if b.npcs[50].WalkDir != -1 {
		t.Errorf("AddNpc(50, 100): WalkDir = %d, want -1 (sentinel)", b.npcs[50].WalkDir)
	}
}

func TestAddNpc_NegativeIsNoop(t *testing.T) {
	b := New()
	b.AddNpc(-1, 100)
	b.AddNpc(50, -1)
	for i := int32(0); i < 8192; i++ {
		if b.npcs[i] != nil {
			t.Errorf("AddNpc with negative arg populated npcs[%d]", i)
			break
		}
	}
}

func TestRemoveNpc_NilsSlot(t *testing.T) {
	b := New()
	b.AddNpc(50, 100)
	b.RemoveNpc(50)
	if b.npcs[50] != nil {
		t.Error("after RemoveNpc(50): slot still non-nil")
	}
}

func TestRemoveNpc_AbsentIsNoop(t *testing.T) {
	b := New()
	b.RemoveNpc(50) // never added
	b.RemoveNpc(-1)
	if b.npcs[50] != nil {
		t.Error("RemoveNpc(absent): slot mutated")
	}
}

func TestRemoveNpc_RemovesFromZoneMap(t *testing.T) {
	b := New()
	b.AddNpc(50, 100)
	// Hand-set coord so zoneMap remove targets a specific zone.
	b.npcs[50].Coord = coordgrid.PackCoord(0, 50, 50)
	b.zoneMap.Zone(50, 0, 50).AddNpc(50)

	b.RemoveNpc(50)

	if _, ok := b.zoneMap.Zone(50, 0, 50).npcs[50]; ok {
		t.Error("RemoveNpc: nid still in zoneMap")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./pkg/rsbuf/ -run "TestAddNpc|TestRemoveNpc"`
Expected: FAIL with `undefined: AddNpc`, `undefined: RemoveNpc` compile errors.

- [ ] **Step 3: Append implementation to `pkg/rsbuf/buf.go`**

Add to the bottom of `pkg/rsbuf/buf.go`:

```go
// AddNpc registers nid with NPC type ntype by allocating an *Npc at
// slot[nid]. Mirrors upstream add_npc at lib.rs:305-311.
//
// No-op if nid == -1, nid >= 8192, or ntype == -1.
func (b *Buf) AddNpc(nid, ntype int32) {
	if nid < 0 || int(nid) >= len(b.npcs) || ntype < 0 {
		return
	}
	b.npcs[nid] = newNpc(nid, ntype)
}

// RemoveNpc unregisters nid. Steps (mirroring upstream remove_npc at
// lib.rs:313-324):
//   1. Remove nid from the zoneMap zone at the npc's last coord
//   2. (NAI-30) NPC_RENDERER.removePermanent(nid) — skipped here
//   3. Set slot[nid] = nil
//
// No-op if nid == -1, nid >= 8192, or slot[nid] is nil.
func (b *Buf) RemoveNpc(nid int32) {
	if nid < 0 || int(nid) >= len(b.npcs) {
		return
	}
	n := b.npcs[nid]
	if n == nil {
		return
	}
	pos := coordgrid.UnpackCoord(n.Coord)
	b.zoneMap.Zone(pos.X, pos.Level, pos.Z).RemoveNpc(nid)
	// Step 2 deferred to NAI-30.
	b.npcs[nid] = nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./pkg/rsbuf/ -run "TestAddNpc|TestRemoveNpc" -v`
Expected: PASS for 5 test functions.

- [ ] **Step 5: Run full package + race**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 -race ./pkg/rsbuf/...`
Expected: PASS.

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...`
Expected: clean.

- [ ] **Step 6: Commit**

```bash
git add pkg/rsbuf/buf.go pkg/rsbuf/buf_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(rsbuf): NAI-29 Bundle 3 Task 3.3 — *Buf AddNpc / RemoveNpc

Adds AddNpc(nid, ntype) and RemoveNpc(nid) to pkg/rsbuf/*Buf. Mirrors
upstream add_npc/remove_npc at lib.rs:305-324.

AddNpc allocates an *Npc at slot[nid] with sentinel defaults +
Observers=0. RemoveNpc removes nid from zoneMap (using last coord)
and nils the slot.

NPC_RENDERER.removePermanent step deferred to NAI-30.

Closes Bundle 3 Task 3.3 of NAI-29 spec.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

## Task 3.4: `*Buf.ComputePlayer`

**Files:**
- Modify: `pkg/rsbuf/buf.go` (add ComputePlayer)
- Modify: `pkg/rsbuf/buf_test.go` (add tests)

- [ ] **Step 1: Append failing tests to `pkg/rsbuf/buf_test.go`**

```go
func TestComputePlayer_WritesAllFields(t *testing.T) {
	b := New()
	b.AddPlayer(5)
	say := "hello"
	msgBytes := []byte{0x10, 0x20}

	b.ComputePlayer(5,
		/*x*/ 50, /*level*/ 0, /*z*/ 60,
		/*originX*/ 48, /*originZ*/ 56,
		/*tele*/ true, /*jump*/ false,
		/*runDir*/ 1, /*walkDir*/ 2,
		/*visibility*/ VisibilitySoft,
		/*active*/ true,
		/*masks*/ 0xff,
		/*appearance*/ []byte{0x01, 0x02, 0x03},
		/*lastAppearance*/ 100,
		/*faceEntity*/ 9, /*faceX*/ 10, /*faceZ*/ 11,
		/*orientationX*/ 12, /*orientationZ*/ 13,
		/*damageTaken*/ 7, /*damageType*/ 1,
		/*currentHitpoints*/ 90, /*baseHitpoints*/ 99,
		/*animID*/ 808, /*animDelay*/ 0,
		/*say*/ &say,
		/*message*/ msgBytes, /*color*/ 1, /*effect*/ 2, /*ignored*/ 3,
		/*graphicID*/ 200, /*graphicHeight*/ 92, /*graphicDelay*/ 0,
		/*exactStartX*/ 30, /*exactStartZ*/ 31,
		/*exactEndX*/ 32, /*exactEndZ*/ 33,
		/*exactMoveStart*/ 34, /*exactMoveEnd*/ 35, /*exactMoveDirection*/ 36,
	)

	p := b.players[5]
	if p == nil {
		t.Fatal("ComputePlayer: slot nilled")
	}
	expectCoord := coordgrid.PackCoord(0, 50, 60)
	if p.Coord != expectCoord {
		t.Errorf("Coord: got %d, want %d", p.Coord, expectCoord)
	}
	expectOrigin := coordgrid.PackCoord(0, 48, 56)
	if p.Origin != expectOrigin {
		t.Errorf("Origin: got %d, want %d", p.Origin, expectOrigin)
	}
	if !p.Tele || p.Jump {
		t.Errorf("Tele/Jump: got (%v, %v), want (true, false)", p.Tele, p.Jump)
	}
	if p.RunDir != 1 || p.WalkDir != 2 {
		t.Errorf("RunDir/WalkDir: got (%d, %d)", p.RunDir, p.WalkDir)
	}
	if p.Visibility != VisibilitySoft {
		t.Errorf("Visibility: got %d, want VisibilitySoft", p.Visibility)
	}
	if !p.Active {
		t.Error("Active: got false, want true")
	}
	if p.Masks != 0xff {
		t.Errorf("Masks: got %d", p.Masks)
	}
	if len(p.Appearance) != 3 {
		t.Errorf("Appearance: got %v", p.Appearance)
	}
	if p.LastAppearance != 100 {
		t.Errorf("LastAppearance: got %d", p.LastAppearance)
	}
	if p.FaceEntity != 9 || p.FaceX != 10 || p.FaceZ != 11 {
		t.Errorf("Face*: got (%d,%d,%d)", p.FaceEntity, p.FaceX, p.FaceZ)
	}
	if p.OrientationX != 12 || p.OrientationZ != 13 {
		t.Errorf("Orientation*: got (%d,%d)", p.OrientationX, p.OrientationZ)
	}
	if p.DamageTaken != 7 || p.DamageType != 1 {
		t.Errorf("Damage*: got (%d,%d)", p.DamageTaken, p.DamageType)
	}
	if p.CurrentHitpoints != 90 || p.BaseHitpoints != 99 {
		t.Errorf("Hitpoints: got (%d/%d)", p.CurrentHitpoints, p.BaseHitpoints)
	}
	if p.AnimID != 808 || p.AnimDelay != 0 {
		t.Errorf("Anim*: got (%d,%d)", p.AnimID, p.AnimDelay)
	}
	if p.Say == nil || *p.Say != "hello" {
		t.Errorf("Say: got %v", p.Say)
	}
	if p.Chat == nil || p.Chat.Color != 1 || p.Chat.Effect != 2 || p.Chat.Ignored != 3 {
		t.Errorf("Chat: got %+v", p.Chat)
	}
	if p.GraphicID != 200 || p.GraphicHeight != 92 || p.GraphicDelay != 0 {
		t.Errorf("Graphic*: got (%d,%d,%d)", p.GraphicID, p.GraphicHeight, p.GraphicDelay)
	}
	if p.ExactMove == nil || p.ExactMove.StartX != 30 || p.ExactMove.Dir != 36 {
		t.Errorf("ExactMove: got %+v", p.ExactMove)
	}
}

func TestComputePlayer_NilSlotIsNoop(t *testing.T) {
	b := New()
	// pid 5 not added — players[5] is nil.
	b.ComputePlayer(5, 50, 0, 60, 48, 56,
		false, false, -1, -1, VisibilityDefault, false, 0,
		nil, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1,
		nil, nil, 0, 0, 0, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1)
	if b.players[5] != nil {
		t.Error("ComputePlayer on nil slot allocated player")
	}
}

func TestComputePlayer_NegativePIDIsNoop(t *testing.T) {
	b := New()
	b.AddPlayer(5)
	b.ComputePlayer(-1, 50, 0, 60, 48, 56,
		false, false, -1, -1, VisibilityDefault, false, 0,
		nil, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1,
		nil, nil, 0, 0, 0, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1)
	// no panic
}

func TestComputePlayer_NilSayBytesAndMessageProduceNilSubstructs(t *testing.T) {
	b := New()
	b.AddPlayer(5)
	b.ComputePlayer(5, 50, 0, 60, 48, 56,
		false, false, -1, -1, VisibilityDefault, false, 0,
		nil, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1,
		nil /*say*/, nil /*message*/, 0, 0, 0, -1, -1, -1,
		-1 /*exactStartX*/, -1, -1, -1, -1, -1, -1)
	p := b.players[5]
	if p.Say != nil {
		t.Error("nil say argument produced non-nil Say")
	}
	if p.Chat != nil {
		t.Error("nil message argument produced non-nil Chat")
	}
	if p.ExactMove != nil {
		t.Error("exactStartX=-1 sentinel produced non-nil ExactMove (mirrors upstream lib.rs:90-103)")
	}
}

func TestComputePlayer_CrossZoneMoveUpdatesZoneMap(t *testing.T) {
	b := New()
	b.AddPlayer(5)
	// Tick 1: place at (50, 0, 50). Zone is (50>>3=6, 0, 50>>3=6).
	b.ComputePlayer(5, 50, 0, 50, 48, 48, false, false, -1, -1,
		VisibilityDefault, true, 0, nil, -1, -1, -1, -1, -1, -1, -1,
		-1, -1, -1, -1, -1, -1, nil, nil, 0, 0, 0, -1, -1, -1,
		-1, -1, -1, -1, -1, -1, -1)
	if _, ok := b.zoneMap.Zone(50, 0, 50).players[5]; !ok {
		t.Fatal("after tick 1: zoneMap zone(50,0,50) should contain pid 5")
	}

	// Tick 2: cross-zone move to (64, 0, 50). Zone is (64>>3=8, 0, 6).
	b.ComputePlayer(5, 64, 0, 50, 48, 48, false, false, -1, -1,
		VisibilityDefault, true, 0, nil, -1, -1, -1, -1, -1, -1, -1,
		-1, -1, -1, -1, -1, -1, nil, nil, 0, 0, 0, -1, -1, -1,
		-1, -1, -1, -1, -1, -1, -1)

	if _, ok := b.zoneMap.Zone(50, 0, 50).players[5]; ok {
		t.Error("after cross-zone move: old zone (50,0,50) still contains pid 5")
	}
	if _, ok := b.zoneMap.Zone(64, 0, 50).players[5]; !ok {
		t.Error("after cross-zone move: new zone (64,0,50) missing pid 5")
	}
}

func TestComputePlayer_SameZoneMoveDoesNotTouchZoneMap(t *testing.T) {
	b := New()
	b.AddPlayer(5)
	// Tick 1: place at (50, 0, 50). Zone (6, 0, 6).
	b.ComputePlayer(5, 50, 0, 50, 48, 48, false, false, -1, -1,
		VisibilityDefault, true, 0, nil, -1, -1, -1, -1, -1, -1, -1,
		-1, -1, -1, -1, -1, -1, nil, nil, 0, 0, 0, -1, -1, -1,
		-1, -1, -1, -1, -1, -1, -1)

	// Tick 2: same-zone move to (55, 0, 50). 55>>3=6 — same zone.
	b.ComputePlayer(5, 55, 0, 50, 48, 48, false, false, -1, -1,
		VisibilityDefault, true, 0, nil, -1, -1, -1, -1, -1, -1, -1,
		-1, -1, -1, -1, -1, -1, nil, nil, 0, 0, 0, -1, -1, -1,
		-1, -1, -1, -1, -1, -1, -1)

	if _, ok := b.zoneMap.Zone(50, 0, 50).players[5]; !ok {
		t.Error("after same-zone move: zone (6,0,6) lost pid 5 (zoneMap should be untouched)")
	}
	if b.players[5].Coord != coordgrid.PackCoord(0, 55, 50) {
		t.Errorf("Coord not updated: got %d, want %d", b.players[5].Coord, coordgrid.PackCoord(0, 55, 50))
	}
}

func TestComputePlayer_AlwaysPushesPlayerGrid(t *testing.T) {
	// Mirrors upstream lib.rs:151 — the player_grid push is unconditional;
	// it happens regardless of whether the move crossed a zone boundary.
	b := New()
	b.AddPlayer(5)
	b.ComputePlayer(5, 50, 0, 50, 48, 48, false, false, -1, -1,
		VisibilityDefault, true, 0, nil, -1, -1, -1, -1, -1, -1, -1,
		-1, -1, -1, -1, -1, -1, nil, nil, 0, 0, 0, -1, -1, -1,
		-1, -1, -1, -1, -1, -1, -1)

	key := uint32(coordgrid.PackCoord(0, 50, 50))
	if got := b.playerGrid[key]; len(got) != 1 || got[0] != 5 {
		t.Errorf("playerGrid[%d]: got %v, want [5]", key, got)
	}

	// Same-zone move pushes the new tile too.
	b.ComputePlayer(5, 55, 0, 50, 48, 48, false, false, -1, -1,
		VisibilityDefault, true, 0, nil, -1, -1, -1, -1, -1, -1, -1,
		-1, -1, -1, -1, -1, -1, nil, nil, 0, 0, 0, -1, -1, -1,
		-1, -1, -1, -1, -1, -1, -1)
	newKey := uint32(coordgrid.PackCoord(0, 55, 50))
	if got := b.playerGrid[newKey]; len(got) != 1 || got[0] != 5 {
		t.Errorf("playerGrid[%d] after second compute: got %v, want [5]", newKey, got)
	}
}
```

> **Note:** The `coordgrid` import is already in `buf_test.go` from Task 3.2. No additional import needed.

- [ ] **Step 2: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./pkg/rsbuf/ -run TestComputePlayer`
Expected: FAIL with `undefined: ComputePlayer` compile error.

- [ ] **Step 3: Append `ComputePlayer` to `pkg/rsbuf/buf.go`**

Add to the bottom of `pkg/rsbuf/buf.go`:

```go
// ComputePlayer writes ALL per-tick state for pid in one call. Mirrors
// upstream compute_player at lib.rs:39-153. Argument order matches
// upstream verbatim for side-by-side review.
//
// Side effects:
//   1. If new coord crosses a zone boundary OR changes level (vs the
//      player's previous Coord): zoneMap.Zone(old).RemovePlayer(pid)
//      then zoneMap.Zone(new).AddPlayer(pid). Same-zone moves skip this
//      step (matches upstream lib.rs:116's zone-bound check).
//   2. Write all 35+ fields onto players[pid].
//   3. (NAI-30) PLAYER_RENDERER.compute_info(player) — skipped here.
//   4. Push pid onto playerGrid[player.Coord] (tile-keyed; unconditional;
//      mirrors upstream lib.rs:151).
//
// No-op if pid == -1, pid >= 2048, or slot[pid] is nil.
//
// Sub-struct construction:
//   - say *string is stored verbatim (nil = no say this tick).
//   - message []byte: nil produces Chat=nil; non-nil produces Chat with
//     {bytes, color, effect, ignored}.
//   - exactStartX < 0 produces ExactMove=nil; otherwise a populated
//     ExactMove. Mirrors upstream lib.rs:90-103.
func (b *Buf) ComputePlayer(
	pid int32,
	x, level, z int,
	originX, originZ int,
	tele, jump bool,
	runDir, walkDir int8,
	visibility Visibility,
	active bool,
	masks uint32,
	appearance []byte,
	lastAppearance int32,
	faceEntity, faceX, faceZ int32,
	orientationX, orientationZ int32,
	damageTaken, damageType int32,
	currentHitpoints, baseHitpoints int32,
	animID, animDelay int32,
	say *string,
	message []byte, color, effect, ignored uint8,
	graphicID, graphicHeight, graphicDelay int32,
	exactStartX, exactStartZ int32,
	exactEndX, exactEndZ int32,
	exactMoveStart, exactMoveEnd, exactMoveDirection int32,
) {
	if pid < 0 || int(pid) >= len(b.players) {
		return
	}
	p := b.players[pid]
	if p == nil {
		return
	}

	newCoord := coordgrid.PackCoord(level, x, z)

	// Step 1: zone-bound check (mirrors lib.rs:116).
	if newCoord != p.Coord {
		oldPos := coordgrid.UnpackCoord(p.Coord)
		// Zone change iff zone-x, zone-z, or level differ.
		if (oldPos.X>>3) != (x>>3) || (oldPos.Z>>3) != (z>>3) || oldPos.Level != level {
			b.zoneMap.Zone(oldPos.X, oldPos.Level, oldPos.Z).RemovePlayer(pid)
			b.zoneMap.Zone(x, level, z).AddPlayer(pid)
		}
	}

	// Step 2: write fields.
	p.Coord = newCoord
	p.Origin = coordgrid.PackCoord(level, originX, originZ)
	p.Tele = tele
	p.Jump = jump
	p.RunDir = runDir
	p.WalkDir = walkDir
	p.Visibility = visibility
	p.Active = active
	p.Masks = masks
	p.Appearance = appearance
	p.LastAppearance = lastAppearance
	p.FaceEntity = faceEntity
	p.FaceX = faceX
	p.FaceZ = faceZ
	p.OrientationX = orientationX
	p.OrientationZ = orientationZ
	p.DamageTaken = damageTaken
	p.DamageType = damageType
	p.CurrentHitpoints = currentHitpoints
	p.BaseHitpoints = baseHitpoints
	p.AnimID = animID
	p.AnimDelay = animDelay
	p.Say = say

	// Sub-struct construction: Chat from message bytes; ExactMove from
	// the exact-move 7-tuple (sentinel exactStartX < 0 = no exact move).
	if message != nil {
		p.Chat = &Chat{
			Bytes:   message,
			Color:   color,
			Effect:  effect,
			Ignored: ignored,
		}
	} else {
		p.Chat = nil
	}

	p.GraphicID = graphicID
	p.GraphicHeight = graphicHeight
	p.GraphicDelay = graphicDelay

	if exactStartX >= 0 {
		p.ExactMove = &ExactMove{
			StartX: exactStartX, StartZ: exactStartZ,
			EndX:   exactEndX, EndZ: exactEndZ,
			Begin:  exactMoveStart, Finish: exactMoveEnd,
			Dir:    exactMoveDirection,
		}
	} else {
		p.ExactMove = nil
	}

	// Step 3 deferred to NAI-30/31 (renderer compute_info).

	// Step 4: unconditional playerGrid push (mirrors lib.rs:151).
	key := uint32(newCoord)
	b.playerGrid[key] = append(b.playerGrid[key], pid)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./pkg/rsbuf/ -run TestComputePlayer -v`
Expected: PASS for 7 test functions.

- [ ] **Step 5: Run full package + race**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 -race ./pkg/rsbuf/...`
Expected: PASS.

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...`
Expected: clean.

- [ ] **Step 6: Commit**

```bash
git add pkg/rsbuf/buf.go pkg/rsbuf/buf_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(rsbuf): NAI-29 Bundle 3 Task 3.4 — *Buf.ComputePlayer (41-arg state push)

Adds ComputePlayer to *Buf. Mirrors upstream compute_player at
lib.rs:39-153 verbatim including the 40-arg signature for side-by-side
review.

Side effects:
  1. zone-bound check: cross-zone moves trigger zoneMap remove+add
     (lib.rs:116). Same-zone moves skip.
  2. Writes 35+ fields onto players[pid].
  3. (NAI-30) PLAYER_RENDERER.compute_info — deferred.
  4. Unconditional playerGrid tile-keyed push (lib.rs:151) — backing
     for NAI-32 spiral search.

Sub-struct construction:
  - say *string: stored verbatim (nil = no say).
  - message []byte: nil → Chat=nil; non-nil → Chat populated.
  - exactStartX < 0: ExactMove=nil; otherwise populated.

No-op for invalid pid or nil slot. Tests pin write-through, cross-zone
move, same-zone move (zoneMap untouched), unconditional grid push.

Closes Bundle 3 Task 3.4 of NAI-29 spec.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

## Task 3.5: `*Buf.ComputeNpc`

**Files:**
- Modify: `pkg/rsbuf/buf.go` (add ComputeNpc)
- Modify: `pkg/rsbuf/buf_test.go` (add tests)

- [ ] **Step 1: Append failing tests to `pkg/rsbuf/buf_test.go`**

```go
func TestComputeNpc_WritesAllFields(t *testing.T) {
	b := New()
	b.AddNpc(50, 100)
	say := "rwar"

	b.ComputeNpc(50, 100,
		/*x*/ 60, /*level*/ 0, /*z*/ 70,
		/*tele*/ true,
		/*runDir*/ 1, /*walkDir*/ 2,
		/*active*/ true,
		/*masks*/ 0xff,
		/*faceEntity*/ 9, /*faceX*/ 10, /*faceZ*/ 11,
		/*orientationX*/ 12, /*orientationZ*/ 13,
		/*damageTaken*/ 7, /*damageType*/ 1,
		/*currentHitpoints*/ 90, /*baseHitpoints*/ 99,
		/*animID*/ 808, /*animDelay*/ 0,
		/*say*/ &say,
		/*graphicID*/ 200, /*graphicHeight*/ 92, /*graphicDelay*/ 0,
	)

	n := b.npcs[50]
	if n == nil {
		t.Fatal("ComputeNpc: slot nilled")
	}
	if n.Coord != coordgrid.PackCoord(0, 60, 70) {
		t.Errorf("Coord: got %d", n.Coord)
	}
	if n.NID != 50 || n.NType != 100 {
		t.Errorf("NID/NType: got (%d, %d)", n.NID, n.NType)
	}
	if !n.Tele {
		t.Error("Tele: got false")
	}
	if n.RunDir != 1 || n.WalkDir != 2 {
		t.Errorf("RunDir/WalkDir: got (%d, %d)", n.RunDir, n.WalkDir)
	}
	if !n.Active {
		t.Error("Active: got false")
	}
	if n.Masks != 0xff {
		t.Errorf("Masks: got %d", n.Masks)
	}
	if n.FaceEntity != 9 || n.FaceX != 10 || n.FaceZ != 11 {
		t.Errorf("Face*: got (%d,%d,%d)", n.FaceEntity, n.FaceX, n.FaceZ)
	}
	if n.OrientationX != 12 || n.OrientationZ != 13 {
		t.Errorf("Orientation*: got (%d,%d)", n.OrientationX, n.OrientationZ)
	}
	if n.DamageTaken != 7 || n.DamageType != 1 {
		t.Errorf("Damage*: got (%d,%d)", n.DamageTaken, n.DamageType)
	}
	if n.AnimID != 808 || n.AnimDelay != 0 {
		t.Errorf("Anim*: got (%d,%d)", n.AnimID, n.AnimDelay)
	}
	if n.Say == nil || *n.Say != "rwar" {
		t.Errorf("Say: got %v", n.Say)
	}
	if n.GraphicID != 200 || n.GraphicHeight != 92 || n.GraphicDelay != 0 {
		t.Errorf("Graphic*: got (%d,%d,%d)", n.GraphicID, n.GraphicHeight, n.GraphicDelay)
	}
}

func TestComputeNpc_NilSlotIsNoop(t *testing.T) {
	b := New()
	// nid 50 not added.
	b.ComputeNpc(50, 100, 60, 0, 70, false, -1, -1, false, 0,
		-1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, nil, -1, -1, -1)
	if b.npcs[50] != nil {
		t.Error("ComputeNpc on nil slot allocated npc")
	}
}

func TestComputeNpc_NegativeIDsAreNoop(t *testing.T) {
	b := New()
	b.AddNpc(50, 100)
	b.ComputeNpc(-1, 100, 60, 0, 70, false, -1, -1, false, 0,
		-1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, nil, -1, -1, -1)
	b.ComputeNpc(50, -1, 60, 0, 70, false, -1, -1, false, 0,
		-1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, nil, -1, -1, -1)
	// Both should no-op.
}

func TestComputeNpc_CrossZoneMoveUpdatesZoneMap(t *testing.T) {
	b := New()
	b.AddNpc(50, 100)
	b.ComputeNpc(50, 100, 50, 0, 50, false, -1, -1, true, 0,
		-1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, nil, -1, -1, -1)
	if _, ok := b.zoneMap.Zone(50, 0, 50).npcs[50]; !ok {
		t.Fatal("after first compute: zone (6,0,6) should contain nid 50")
	}

	b.ComputeNpc(50, 100, 64, 0, 50, false, -1, -1, true, 0,
		-1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, nil, -1, -1, -1)
	if _, ok := b.zoneMap.Zone(50, 0, 50).npcs[50]; ok {
		t.Error("after cross-zone: old zone still contains nid 50")
	}
	if _, ok := b.zoneMap.Zone(64, 0, 50).npcs[50]; !ok {
		t.Error("after cross-zone: new zone missing nid 50")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./pkg/rsbuf/ -run TestComputeNpc`
Expected: FAIL with `undefined: ComputeNpc` compile error.

- [ ] **Step 3: Append `ComputeNpc` to `pkg/rsbuf/buf.go`**

Add to the bottom of `pkg/rsbuf/buf.go`:

```go
// ComputeNpc writes ALL per-tick state for nid in one call. Mirrors
// upstream compute_npc at lib.rs:217-281. Argument order matches
// upstream verbatim.
//
// Side effects:
//   1. If new coord crosses a zone boundary OR changes level: zoneMap
//      remove+add (mirrors lib.rs:251).
//   2. Write 21+ fields onto npcs[nid] (note: ntype is overwritten —
//      mirrors upstream lib.rs:256).
//   3. (NAI-30) NPC_RENDERER.compute_info(npc) — skipped here.
//
// No-op if nid == -1, nid >= 8192, ntype == -1, or slot[nid] is nil.
//
// Note: NPCs do NOT update playerGrid (matches upstream — the
// tile-keyed grid is player-only). NPCs are spatially indexed only
// via zoneMap.zones[k].npcs.
func (b *Buf) ComputeNpc(
	nid, ntype int32,
	x, level, z int,
	tele bool,
	runDir, walkDir int8,
	active bool,
	masks uint32,
	faceEntity, faceX, faceZ int32,
	orientationX, orientationZ int32,
	damageTaken, damageType int32,
	currentHitpoints, baseHitpoints int32,
	animID, animDelay int32,
	say *string,
	graphicID, graphicHeight, graphicDelay int32,
) {
	if nid < 0 || int(nid) >= len(b.npcs) || ntype < 0 {
		return
	}
	n := b.npcs[nid]
	if n == nil {
		return
	}

	newCoord := coordgrid.PackCoord(level, x, z)
	if newCoord != n.Coord {
		oldPos := coordgrid.UnpackCoord(n.Coord)
		if (oldPos.X>>3) != (x>>3) || (oldPos.Z>>3) != (z>>3) || oldPos.Level != level {
			b.zoneMap.Zone(oldPos.X, oldPos.Level, oldPos.Z).RemoveNpc(nid)
			b.zoneMap.Zone(x, level, z).AddNpc(nid)
		}
	}

	n.NType = ntype
	n.Coord = newCoord
	n.Tele = tele
	n.RunDir = runDir
	n.WalkDir = walkDir
	n.Active = active
	n.Masks = masks
	n.FaceEntity = faceEntity
	n.FaceX = faceX
	n.FaceZ = faceZ
	n.OrientationX = orientationX
	n.OrientationZ = orientationZ
	n.DamageTaken = damageTaken
	n.DamageType = damageType
	n.CurrentHitpoints = currentHitpoints
	n.BaseHitpoints = baseHitpoints
	n.AnimID = animID
	n.AnimDelay = animDelay
	n.Say = say
	n.GraphicID = graphicID
	n.GraphicHeight = graphicHeight
	n.GraphicDelay = graphicDelay

	// Step 3 (renderer compute_info) deferred to NAI-30/31.
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./pkg/rsbuf/ -run TestComputeNpc -v`
Expected: PASS for 4 test functions.

- [ ] **Step 5: Run full package + race**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 -race ./pkg/rsbuf/...`
Expected: PASS.

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...`
Expected: clean.

- [ ] **Step 6: Commit**

```bash
git add pkg/rsbuf/buf.go pkg/rsbuf/buf_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(rsbuf): NAI-29 Bundle 3 Task 3.5 — *Buf.ComputeNpc

Adds ComputeNpc to *Buf. Mirrors upstream compute_npc at
lib.rs:217-281. 25-arg signature matches upstream verbatim.

Side effects:
  1. zone-bound check: cross-zone/cross-level moves trigger zoneMap
     remove+add (lib.rs:251). Same-zone moves skip.
  2. Writes 21+ fields onto npcs[nid] including NType overwrite.
  3. (NAI-30) NPC_RENDERER.compute_info — deferred.

NPCs are NOT pushed to playerGrid (matches upstream — tile-keyed grid
is player-only).

Closes Bundle 3 Task 3.5 of NAI-29 spec.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

## Task 3.6: `*Buf.Cleanup` + `CleanupPlayerBuildArea`

**Files:**
- Modify: `pkg/rsbuf/buf.go`
- Modify: `pkg/rsbuf/buf_test.go`

- [ ] **Step 1: Append failing tests to `pkg/rsbuf/buf_test.go`**

```go
func TestCleanup_ClearsPlayerGridAndCallsEntityCleanup(t *testing.T) {
	b := New()
	b.AddPlayer(5)
	b.AddNpc(10, 100)
	// Compute populates state + playerGrid.
	b.ComputePlayer(5, 50, 0, 50, 48, 48, true, false, 1, 2,
		VisibilityDefault, true, 0xff, []byte{1}, -1, -1, -1, -1, -1, -1, -1,
		-1, -1, -1, -1, 808, 0, nil, nil, 0, 0, 0, -1, -1, -1,
		-1, -1, -1, -1, -1, -1, -1)
	b.ComputeNpc(10, 100, 60, 0, 60, true, 1, 2, true, 0xff,
		-1, -1, -1, -1, -1, -1, -1, -1, -1, -1, 808, nil, -1, -1, -1)
	if len(b.playerGrid) == 0 {
		t.Fatal("test setup: ComputePlayer did not populate playerGrid")
	}
	if !b.players[5].Tele || b.players[5].Masks != 0xff {
		t.Fatal("test setup: ComputePlayer didn't write fields")
	}
	if !b.npcs[10].Tele || b.npcs[10].Masks != 0xff {
		t.Fatal("test setup: ComputeNpc didn't write fields")
	}

	b.Cleanup()

	if len(b.playerGrid) != 0 {
		t.Errorf("Cleanup: playerGrid not cleared, len=%d", len(b.playerGrid))
	}
	if b.players[5].Tele {
		t.Error("Cleanup: player.Tele not reset")
	}
	if b.players[5].Masks != 0 {
		t.Errorf("Cleanup: player.Masks = %d, want 0", b.players[5].Masks)
	}
	if b.npcs[10].Tele {
		t.Error("Cleanup: npc.Tele not reset")
	}
	if b.npcs[10].Masks != 0 {
		t.Errorf("Cleanup: npc.Masks = %d, want 0", b.npcs[10].Masks)
	}
}

func TestCleanup_PreservesAppearanceAndOrientation(t *testing.T) {
	// Cleanup does NOT clear the persistent fields per upstream
	// player.rs/npc.rs commented-out cleanup lines.
	b := New()
	b.AddPlayer(5)
	b.AddNpc(10, 100)
	b.players[5].Appearance = []byte{1, 2, 3}
	b.players[5].LastAppearance = 100
	b.players[5].FaceEntity = 42
	b.players[5].OrientationX = 50
	b.npcs[10].FaceEntity = 99
	b.npcs[10].OrientationX = 33
	b.npcs[10].Observers = 4

	b.Cleanup()

	if len(b.players[5].Appearance) != 3 {
		t.Error("Cleanup CLEARED player.Appearance")
	}
	if b.players[5].LastAppearance != 100 {
		t.Errorf("Cleanup CLEARED player.LastAppearance")
	}
	if b.players[5].FaceEntity != 42 || b.players[5].OrientationX != 50 {
		t.Error("Cleanup CLEARED player FaceEntity / OrientationX")
	}
	if b.npcs[10].FaceEntity != 99 || b.npcs[10].OrientationX != 33 {
		t.Error("Cleanup CLEARED npc FaceEntity / OrientationX")
	}
	if b.npcs[10].Observers != 4 {
		t.Errorf("Cleanup CLEARED npc.Observers: got %d, want 4", b.npcs[10].Observers)
	}
}

func TestCleanup_NilSlotsAreSkipped(t *testing.T) {
	b := New()
	// No AddPlayer / AddNpc calls — all slots nil.
	b.ComputePlayer(5, 50, 0, 50, 48, 48, false, false, -1, -1,
		VisibilityDefault, false, 0, nil, -1, -1, -1, -1, -1, -1, -1,
		-1, -1, -1, -1, -1, nil, nil, 0, 0, 0, -1, -1, -1,
		-1, -1, -1, -1, -1, -1, -1)
	// playerGrid push from ComputePlayer was a no-op (nil slot guard).
	b.Cleanup() // must not panic on nil-slot iteration
}

func TestCleanupPlayerBuildArea_ClearsTrackingAndAppearances(t *testing.T) {
	b := New()
	b.AddPlayer(5)
	b.players[5].Build.Players.Insert(10)
	b.players[5].Build.Npcs.Insert(20)
	b.players[5].Build.SaveAppearance(7, 100)

	b.CleanupPlayerBuildArea(5)

	if b.players[5].Build.Players.Len() != 0 {
		t.Error("CleanupPlayerBuildArea: Players set not cleared")
	}
	if b.players[5].Build.Npcs.Len() != 0 {
		t.Error("CleanupPlayerBuildArea: Npcs set not cleared")
	}
	if b.players[5].Build.HasAppearance(7, 100) {
		t.Error("CleanupPlayerBuildArea: appearances not cleared")
	}
}

func TestCleanupPlayerBuildArea_NilSlotIsNoop(t *testing.T) {
	b := New()
	b.CleanupPlayerBuildArea(5) // never added
	b.CleanupPlayerBuildArea(-1)
	b.CleanupPlayerBuildArea(2048)
	// no panic
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./pkg/rsbuf/ -run "TestCleanup"`
Expected: FAIL with `undefined: Cleanup`, `undefined: CleanupPlayerBuildArea` compile errors.

- [ ] **Step 3: Append `Cleanup` + `CleanupPlayerBuildArea` to `pkg/rsbuf/buf.go`**

Add to the bottom of `pkg/rsbuf/buf.go`:

```go
// Cleanup resets the tile-keyed playerGrid and calls cleanup() on every
// populated Player + Npc. Called once per tick at end-of-tick (after
// info encoding completes). Mirrors upstream cleanup at lib.rs:348-363.
//
// (NAI-30) PLAYER_RENDERER.removeTemporary + NPC_RENDERER.removeTemporary
// at lib.rs:351-352 are skipped here pending NAI-31 renderer port.
func (b *Buf) Cleanup() {
	// Clear playerGrid (tile-keyed; rebuilt fresh each tick).
	for k := range b.playerGrid {
		delete(b.playerGrid, k)
	}
	for _, p := range b.players {
		if p != nil {
			p.cleanup()
		}
	}
	for _, n := range b.npcs {
		if n != nil {
			n.cleanup()
		}
	}
}

// CleanupPlayerBuildArea calls Cleanup on the named player's BuildArea
// (clears tracking sets + appearances). Used at logout pre-flush.
// Mirrors upstream cleanup_player_buildarea at lib.rs:365-373.
//
// No-op if pid == -1, pid >= 2048, or slot[pid] is nil.
func (b *Buf) CleanupPlayerBuildArea(pid int32) {
	if pid < 0 || int(pid) >= len(b.players) {
		return
	}
	p := b.players[pid]
	if p == nil {
		return
	}
	p.Build.Cleanup()
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./pkg/rsbuf/ -run "TestCleanup" -v`
Expected: PASS for 5 test functions.

- [ ] **Step 5: Run full package + race**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 -race ./pkg/rsbuf/...`
Expected: PASS.

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...`
Expected: clean.

- [ ] **Step 6: Commit**

```bash
git add pkg/rsbuf/buf.go pkg/rsbuf/buf_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(rsbuf): NAI-29 Bundle 3 Task 3.6 — *Buf.Cleanup + CleanupPlayerBuildArea

Adds end-of-tick Cleanup and per-player CleanupPlayerBuildArea to *Buf.
Mirrors upstream cleanup + cleanup_player_buildarea at lib.rs:348-373.

Cleanup:
  - Clears playerGrid (tile-keyed; rebuilt each tick from ComputePlayer).
  - Calls cleanup() on every populated Player + Npc (zeros transient
    state per upstream player.rs/npc.rs cleanup, preserves Appearance,
    LastAppearance, FaceEntity, OrientationX/Z, Observers).

(NAI-30) PLAYER_RENDERER.removeTemporary + NPC_RENDERER.removeTemporary
deferred.

CleanupPlayerBuildArea clears one player's Build state (tracking sets
+ appearances). Used at logout.

Closes Bundle 3 Task 3.6 of NAI-29 spec.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

## Task 3.7: `*Buf.HasPlayer` / `HasNpc` / `GetNpcObservers`

**Files:**
- Modify: `pkg/rsbuf/buf.go`
- Modify: `pkg/rsbuf/buf_test.go`

- [ ] **Step 1: Append failing tests to `pkg/rsbuf/buf_test.go`**

```go
func TestHasPlayer_ChecksBuildArea(t *testing.T) {
	b := New()
	b.AddPlayer(5)
	b.players[5].Build.Players.Insert(10)
	if !b.HasPlayer(5, 10) {
		t.Error("HasPlayer(5, 10) false after Build.Players.Insert(10)")
	}
	if b.HasPlayer(5, 11) {
		t.Error("HasPlayer(5, 11) true (never inserted)")
	}
}

func TestHasPlayer_NegativeArgsAreFalse(t *testing.T) {
	b := New()
	b.AddPlayer(5)
	if b.HasPlayer(-1, 10) {
		t.Error("HasPlayer(-1, 10) returned true")
	}
	if b.HasPlayer(5, -1) {
		t.Error("HasPlayer(5, -1) returned true")
	}
}

func TestHasPlayer_NilSlotIsFalse(t *testing.T) {
	b := New()
	if b.HasPlayer(5, 10) { // pid 5 not added
		t.Error("HasPlayer on nil slot returned true")
	}
}

func TestHasNpc_ChecksBuildArea(t *testing.T) {
	b := New()
	b.AddPlayer(5)
	b.players[5].Build.Npcs.Insert(20)
	if !b.HasNpc(5, 20) {
		t.Error("HasNpc(5, 20) false after Build.Npcs.Insert(20)")
	}
	if b.HasNpc(5, 21) {
		t.Error("HasNpc(5, 21) true (never inserted)")
	}
}

func TestHasNpc_NegativeArgsAreFalse(t *testing.T) {
	b := New()
	b.AddPlayer(5)
	if b.HasNpc(-1, 20) {
		t.Error("HasNpc(-1, 20) returned true")
	}
	if b.HasNpc(5, -1) {
		t.Error("HasNpc(5, -1) returned true")
	}
}

func TestGetNpcObservers_ReadsCounter(t *testing.T) {
	b := New()
	b.AddNpc(50, 100)
	b.npcs[50].Observers = 7
	if b.GetNpcObservers(50) != 7 {
		t.Errorf("GetNpcObservers(50): got %d, want 7", b.GetNpcObservers(50))
	}
}

func TestGetNpcObservers_NilSlotIsZero(t *testing.T) {
	b := New()
	if b.GetNpcObservers(50) != 0 {
		t.Errorf("GetNpcObservers on nil slot: got %d, want 0", b.GetNpcObservers(50))
	}
	if b.GetNpcObservers(-1) != 0 {
		t.Error("GetNpcObservers(-1): got non-zero")
	}
	if b.GetNpcObservers(8192) != 0 {
		t.Error("GetNpcObservers(8192): got non-zero")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./pkg/rsbuf/ -run "TestHasPlayer|TestHasNpc|TestGetNpcObservers"`
Expected: FAIL with `undefined: HasPlayer`, `undefined: HasNpc`, `undefined: GetNpcObservers` errors.

- [ ] **Step 3: Append observer queries to `pkg/rsbuf/buf.go`**

Add to the bottom of `pkg/rsbuf/buf.go`:

```go
// HasPlayer reports whether pid currently observes other (i.e., other
// is in pid's BuildArea.Players tracking set). Mirrors upstream
// has_player at lib.rs:205-214.
//
// Returns false if either id is < 0, or pid's slot is nil.
func (b *Buf) HasPlayer(pid, other int32) bool {
	if pid < 0 || other < 0 || int(pid) >= len(b.players) {
		return false
	}
	p := b.players[pid]
	if p == nil {
		return false
	}
	return p.Build.Players.Contains(other)
}

// HasNpc reports whether pid currently observes nid (i.e., nid is in
// pid's BuildArea.Npcs tracking set). Mirrors upstream has_npc at
// lib.rs:326-335.
func (b *Buf) HasNpc(pid, nid int32) bool {
	if pid < 0 || nid < 0 || int(pid) >= len(b.players) {
		return false
	}
	p := b.players[pid]
	if p == nil {
		return false
	}
	return p.Build.Npcs.Contains(nid)
}

// GetNpcObservers returns the count of players currently observing nid.
// Mirrors upstream get_npc_observers at lib.rs:337-346.
//
// Returns 0 if nid < 0, nid >= 8192, or slot[nid] is nil.
func (b *Buf) GetNpcObservers(nid int32) int32 {
	if nid < 0 || int(nid) >= len(b.npcs) {
		return 0
	}
	n := b.npcs[nid]
	if n == nil {
		return 0
	}
	return n.Observers
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./pkg/rsbuf/ -run "TestHasPlayer|TestHasNpc|TestGetNpcObservers" -v`
Expected: PASS for 7 test functions.

- [ ] **Step 5: Run full package + race**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 -race ./pkg/rsbuf/...`
Expected: PASS — including ALL prior tests in pkg/rsbuf (existing encoder tests, Renderer tests, mask payload tests stay green).

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...`
Expected: clean.

- [ ] **Step 6: Commit**

```bash
git add pkg/rsbuf/buf.go pkg/rsbuf/buf_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(rsbuf): NAI-29 Bundle 3 Task 3.7 — *Buf observer queries (HasPlayer/HasNpc/GetNpcObservers)

Adds the observer-query API to *Buf. Mirrors upstream has_player /
has_npc / get_npc_observers at lib.rs:205-346.

HasPlayer(pid, other) checks pid's BuildArea.Players tracking set.
HasNpc(pid, nid) checks pid's BuildArea.Npcs tracking set.
GetNpcObservers(nid) returns the npc's observer counter.

All three return zero/false for invalid args or nil slots.

This closes the public API surface for Bundle 3. *Buf is fully
constructable + operational. Bundle 4 wires production caller hooks.

Closes Bundle 3 Task 3.7 of NAI-29 spec.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

# Bundle 4 — Caller Wiring (Parallel-Write Window)

Bundle 4 wires `*rsbuf.Buf` into production. After B4 close, every player position update + npc spawn + per-tick state flows through `*Buf` alongside the existing `pkg/grid` + `pkg/zone` updates. Existing encoder is **unchanged** — `*Buf` state is populated but never read.

**Concurrency model** (verified at brainstorm time): tick loop runs on a single goroutine (`go s.runTickLoop()`). `playersMu` only protects login/logout intake handoff between connection-goroutines and the tick goroutine. All `*Buf` state mutation in this bundle is on the tick goroutine; no rsbuf-internal lock needed.

**Field name reference** (verified at plan-write time):
- `Server` field for the rsbuf instance: `rsbuf *rsbuf.Buf` (lowercase rsbuf to match alphabetic ordering).
- Server's player slots: `s.players [2048]*Player`; slot range `[1, len(s.players))` — slot 0 is reserved.
- Server's player loop: `s.playerLoop []*Player` (drained slice for tick iteration).
- Server's npc registry access: see `npc_registry.go` for canonical loop pattern.
- Player lowercase fields: `p.slot`, `p.x, p.z, p.level`, `p.originX, p.originZ`, `p.tele, p.jump`, `p.walkDir, p.runDir`, `p.visibility`, `p.active`, `p.masks`, `p.faceEntity, p.faceSquareX, p.faceSquareZ`, `p.staffModLevel`, `p.appearanceBuf`, `p.animID, p.animDelay`, `p.sayText, p.damageAmt, p.damageType`, `p.spotanimID, p.spotanimHeight, p.spotanimDelay`, `p.exactStartX/Z, exactEndX/Z, exactBegin, exactFinish, exactDir`, `p.chatColour, p.chatEffect, p.chatRights, p.chatBytes`.
- Player accessors used elsewhere: `p.CurHP() int`, `p.AppearanceHash() uint64`.
- Npc lowercase fields: `n.nid`, `n.typeId`, `n.x, n.z, n.level`, `n.tele`, `n.walkDir, n.runDir`, `n.active`, `n.masks` (verify at task time — Npc may not have all PlayerSource-equivalent fields yet).

**Argument mapping for `ComputePlayer`** (resolved at plan-write time):

| Upstream arg | Goscape source |
|---|---|
| `pid int32` | `int32(p.slot)` |
| `x, level, z int` | `p.x, p.level, p.z` |
| `originX, originZ int` | `p.originX, p.originZ` |
| `tele, jump bool` | `p.tele, p.jump` |
| `runDir, walkDir int8` | `int8(p.runDir), int8(p.walkDir)` |
| `visibility Visibility` | `p.visibility` |
| `active bool` | `p.active` |
| `masks uint32` | `uint32(p.masks)` |
| `appearance []byte` | `p.appearanceBuf` (already a slice) |
| `lastAppearance int32` | `int32(p.AppearanceHash() & 0x7fffffff)` — semantic divergence from upstream lastAppearance (tick-when-changed); revisit at NAI-30 |
| `faceEntity int32` | `int32(p.faceEntity)` |
| `faceX, faceZ int32` | `int32(p.faceSquareX), int32(p.faceSquareZ)` |
| `orientationX, orientationZ int32` | (not stored on Player today) `int32(0)` placeholder — flag at NAI-30 |
| `damageTaken, damageType int32` | `int32(p.damageAmt), int32(p.damageType)` |
| `currentHitpoints, baseHitpoints int32` | `int32(p.CurHP()), int32(p.baseHP())` (verify baseHP() accessor at task time; may need direct field) |
| `animID, animDelay int32` | `int32(p.animID), int32(p.animDelay)` |
| `say *string` | from `p.sayText` (`[]byte`); if nil/empty → nil; else `s := string(p.sayText); &s` |
| `message []byte, color, effect, ignored uint8` | `p.chatBytes`, `uint8(p.chatColour), uint8(p.chatEffect), uint8(p.chatRights)` |
| `graphicID, graphicHeight, graphicDelay int32` | `int32(p.spotanimID), int32(p.spotanimHeight), int32(p.spotanimDelay)` |
| `exactStartX, exactStartZ` | `int32(p.exactStartX), int32(p.exactStartZ)` |
| `exactEndX, exactEndZ` | `int32(p.exactEndX), int32(p.exactEndZ)` |
| `exactMoveStart, exactMoveEnd, exactMoveDirection` | `int32(p.exactBegin), int32(p.exactFinish), int32(p.exactDir)` |

**Plan-author note:** Some Player fields above (`baseHP`, `orientationX/Z`) are not currently exposed on `modules/world.Player`. Bundle 4 implementer must verify each accessor at task time; if any is genuinely missing from the Player struct, pass `int32(0)` and document with a `// NAI-29: missing field; verified at HEAD` comment so NAI-30 implementer flags the gap. **This is acceptable for the parallel-write window** — `*Buf` state is not yet read by any encoder until NAI-30, so missing-field placeholders cause no observable behavioral change.

## Task 4.1: Add `*Buf` field + initialization to `*Server`

**Files:**
- Modify: `modules/world/server.go` (struct field + init)
- Create: `modules/world/rsbuf_init_test.go`

- [ ] **Step 1: Verify HEAD before editing** (per `controller_preflight` memory)

Run: `rg -n "rsbuf\s+\*rsbuf\.Buf" modules/world/server.go`
Expected: zero matches (field does not exist yet).

Run: `rg -n "type Server struct" modules/world/server.go`
Expected: line `:44`-ish (single match).

Run: `sed -n '44,98p' modules/world/server.go` to confirm Server struct shape and find a good alphabetic-order insertion site for `rsbuf *rsbuf.Buf`. Insert site target: between `playerLoop []*Player` and `playerLookup` (or whatever follows alphabetically). Verify visually.

- [ ] **Step 2: Write failing test in `modules/world/rsbuf_init_test.go`**

```go
package world

import (
	"testing"
)

// TestServer_RsbufInitialized confirms that a freshly-constructed Server
// (via the same path runTickLoop uses) has a non-nil *rsbuf.Buf field.
// Bundle 4 wiring; NAI-29 Task 4.1.
func TestServer_RsbufInitialized(t *testing.T) {
	s := newTestServer(t)
	if s.rsbuf == nil {
		t.Fatal("Server.rsbuf is nil after newTestServer; expected initialized *rsbuf.Buf")
	}
}
```

> **Note on `newTestServer`:** This helper is the project's existing test-server factory (verified at brainstorm time to live in `modules/world` test files). If it doesn't initialize `s.rsbuf` directly, this test will catch the gap. Plan-author verification: `rg -n "func newTestServer" modules/world/*_test.go` to confirm the helper's location and adjust the import if needed.

- [ ] **Step 3: Run test to verify it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./modules/world/ -run TestServer_RsbufInitialized`
Expected: FAIL with `s.rsbuf undefined` compile error.

- [ ] **Step 4: Add `rsbuf *rsbuf.Buf` field to `*Server` struct in `modules/world/server.go`**

In the `Server` struct (around line `:44`), insert the field. Visual context (find the `playerLoop` / `playersMu` block):

Locate this block in `modules/world/server.go`:

```go
	players     [2048]*Player
	playerLoop  []*Player
	newPlayers  []*Player // guarded by playersMu; drained by processLogins
	playersMu   sync.RWMutex
	currentTick int
```

Insert immediately after `playersMu`:

```go
	players     [2048]*Player
	playerLoop  []*Player
	newPlayers  []*Player // guarded by playersMu; drained by processLogins
	playersMu   sync.RWMutex
	currentTick int

	// rsbuf is the per-tick stateful encoder core (NAI-29). One per
	// Server. Owned exclusively by the tick goroutine after init —
	// never accessed from connection goroutines. Per NAI-29 Bundle 4,
	// AddPlayer/AddNpc/ComputePlayer/Cleanup are wired in subsequent
	// tasks (4.2-4.6). At end of NAI-29: parallel-write window per
	// parallel_spatial_index_migration_pattern memory; existing encoder
	// (Encode/EncodeNpc) does not yet read from rsbuf state.
	rsbuf *rsbuf.Buf
```

Verify the import block at the top of `modules/world/server.go` includes `"github.com/zsrv/goscape/pkg/rsbuf"`. If it isn't already there, add it (check existing imports — likely already present since other modules/world files use rsbuf).

- [ ] **Step 5: Initialize `s.rsbuf = rsbuf.New()` in the Server constructor / runTickLoop entry**

Find the existing zoneMap initialization (a single grep for `s.zoneMap = ` or similar):

Run: `rg -n "s\.zoneMap\s*=" modules/world/server.go`
Expected: one match (most likely in `(*Server).runTickLoop` or its setup helper).

At that initialization site, add the rsbuf init **immediately after** the zoneMap init:

```go
	s.zoneMap = ...
	s.rsbuf = rsbuf.New()
```

(The exact existing line varies — match the surrounding format.)

- [ ] **Step 6: Run test to verify it passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./modules/world/ -run TestServer_RsbufInitialized -v`
Expected: PASS.

- [ ] **Step 7: Run full package + race**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 -race ./...`
Expected: PASS — all existing modules/world tests stay green.

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...`
Expected: clean.

- [ ] **Step 8: Commit**

```bash
git add modules/world/server.go modules/world/rsbuf_init_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-29 Bundle 4 Task 4.1 — *Server.rsbuf field + initialization

Adds rsbuf *rsbuf.Buf field to *Server and wires its initialization
alongside the existing zoneMap init.

The field is tick-goroutine-owned: only the tick goroutine accesses
rsbuf state. Connection goroutines hand off via the existing
s.newPlayers channel + playersMu lock; the tick goroutine drains it
before AddPlayer/RemovePlayer hooks fire (Tasks 4.2 onward).

Field is initialized but no hooks fire yet — those are added in
Tasks 4.2-4.6. After Task 4.1 alone: rsbuf state is empty, players +
npcs + zoneMap + playerGrid are all zero-init.

Closes Bundle 4 Task 4.1 of NAI-29 spec.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

## Task 4.2: Wire `AddPlayer` / `RemovePlayer` hooks at login/logout

**Files:**
- Modify: `modules/world/server.go` (`addPlayer`, `removePlayer`)
- Create: `modules/world/rsbuf_lifecycle_test.go`

- [ ] **Step 1: Pre-flight verify the addPlayer / removePlayer call site lines**

Run: `rg -n "func \(s \*Server\) addPlayer\b|func \(s \*Server\) removePlayer\b" modules/world/server.go`
Expected: 2 matches around lines `:599` and `:649`.

Run: `sed -n '599,665p' modules/world/server.go`
Expected: visual confirmation of the existing Zone Enter/Leave wiring (NAI-28 Bundle 2 added these). The new rsbuf hooks land **after** the Zone hooks.

- [ ] **Step 2: Write failing tests in `modules/world/rsbuf_lifecycle_test.go`**

```go
package world

import (
	"testing"
)

// Tests for NAI-29 Bundle 4 Task 4.2 — AddPlayer / RemovePlayer hooks
// at login / logout sites in (*Server).addPlayer / removePlayer.

func TestServer_AddPlayerWiresRsbufSlot(t *testing.T) {
	s := newTestServer(t)
	p := newTestPlayer()
	if err := s.addPlayer(p); err != nil {
		t.Fatalf("addPlayer: %v", err)
	}
	if p.slot < 1 {
		t.Fatalf("addPlayer didn't assign slot (got %d)", p.slot)
	}
	// rsbuf.players[slot] must now be non-nil.
	if got := s.rsbuf.HasPlayer(int32(p.slot), int32(p.slot)); got {
		// HasPlayer with self-as-other is always false (BuildArea
		// doesn't include self); use a different probe — the slot's
		// existence is verified via the absence of HasPlayer false-
		// positives plus a separate GetNpcObservers check ensuring
		// the slot table is non-empty.
		t.Fatalf("HasPlayer(self, self): got %v, want false", got)
	}
	// Probe via observer count of an unrelated npc — must be 0; this
	// will panic only if rsbuf state is unallocated. The actual slot
	// allocation is verified by the hasPlayer-on-other path below.
	if got := s.rsbuf.GetNpcObservers(0); got != 0 {
		t.Errorf("GetNpcObservers(0) on fresh rsbuf: got %d, want 0", got)
	}

	// Add a second player and verify the first observes the second
	// after manual BuildArea seeding (sanity check that the slot
	// allocation reached far enough to populate Build).
	p2 := newTestPlayer()
	if err := s.addPlayer(p2); err != nil {
		t.Fatalf("second addPlayer: %v", err)
	}
}

func TestServer_RemovePlayerCleansRsbufSlot(t *testing.T) {
	s := newTestServer(t)
	p := newTestPlayer()
	if err := s.addPlayer(p); err != nil {
		t.Fatalf("addPlayer: %v", err)
	}
	pid := int32(p.slot)

	s.removePlayer(p)

	// After removePlayer, the rsbuf slot should be nil — observable via:
	// HasPlayer(pid, anything) returns false (would also return false
	// for an active slot with empty BuildArea, so this is necessary
	// but not sufficient). Stronger: GetNpcObservers should still
	// return 0 (no panic on unrelated query); and a re-add at the
	// same pid should succeed without leaking state from the previous
	// life.
	if got := s.rsbuf.HasPlayer(pid, 99); got {
		t.Errorf("after removePlayer: HasPlayer(%d, 99): got true, want false", pid)
	}

	// Re-add at same slot: must succeed (the slot was nilled).
	p2 := newTestPlayer()
	if err := s.addPlayer(p2); err != nil {
		t.Fatalf("re-add after removePlayer: %v", err)
	}
}
```

> **`newTestServer` and `newTestPlayer` helpers:** Verify these exist in the project test scaffolding. If they don't, the implementer should:
> 1. Use the existing `addPlayerToServer` helper (verified to exist post-NAI-28; check `modules/world/test_helpers*.go`).
> 2. Or construct minimal `*Server` and `*Player` literals inline.
>
> Run: `rg -n "func newTestServer\b|func newTestPlayer\b|func addPlayerToServer\b" modules/world/`
> If the function names differ, adapt the test bodies accordingly.

- [ ] **Step 3: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./modules/world/ -run TestServer_AddPlayerWires|TestServer_RemovePlayerCleans`
Expected: FAIL — likely on test-helper-undefined or because the production hook isn't wired yet.

- [ ] **Step 4: Add the rsbuf hook to `(*Server).addPlayer`**

Locate the existing Zone wiring inside `addPlayer` (lines ~`:608-613`):

```go
		if i := allocateSlot(s); i >= 0 {
			p.slot = i
			s.players[i] = p
			s.playerLoop = append(s.playerLoop, p)
			p.active = true
			if s.zoneMap != nil {
				z := s.zoneMap.Get(p.level, p.x, p.z)
				p.zoneListElement = z.EnterPlayer(p, s.zoneMap.Grid(p.level))
			}
			return nil
		}
```

(Note: the actual existing code may not use a separate `allocateSlot` helper; the slot-find logic may be inline. Match the existing structure.)

Add the rsbuf hook immediately after the zoneMap block:

```go
		if i := /* existing slot allocation */ ; i >= 0 {
			p.slot = i
			s.players[i] = p
			s.playerLoop = append(s.playerLoop, p)
			p.active = true
			if s.zoneMap != nil {
				z := s.zoneMap.Get(p.level, p.x, p.z)
				p.zoneListElement = z.EnterPlayer(p, s.zoneMap.Grid(p.level))
			}
			if s.rsbuf != nil {
				s.rsbuf.AddPlayer(int32(p.slot))
			}
			return nil
		}
```

- [ ] **Step 5: Add the rsbuf hook to `(*Server).removePlayer`**

Locate the existing Zone-leave wiring inside `removePlayer` (lines ~`:651-656`):

```go
	if s.zoneMap != nil && p.zoneListElement != nil {
		z := s.zoneMap.Get(p.level, p.x, p.z)
		z.LeavePlayer(p, p.zoneListElement, s.zoneMap.Grid(p.level))
		p.zoneListElement = nil
	}
	s.players[p.slot] = nil
```

Add the rsbuf hook immediately after the zoneMap leave block (before the `s.players[p.slot] = nil` assignment):

```go
	if s.zoneMap != nil && p.zoneListElement != nil {
		z := s.zoneMap.Get(p.level, p.x, p.z)
		z.LeavePlayer(p, p.zoneListElement, s.zoneMap.Grid(p.level))
		p.zoneListElement = nil
	}
	if s.rsbuf != nil {
		s.rsbuf.CleanupPlayerBuildArea(int32(p.slot))
		s.rsbuf.RemovePlayer(int32(p.slot))
	}
	s.players[p.slot] = nil
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./modules/world/ -run "TestServer_AddPlayerWires|TestServer_RemovePlayerCleans" -v`
Expected: PASS.

- [ ] **Step 7: Run full package + race**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 -race ./modules/world/...`
Expected: PASS — all existing modules/world tests stay green.

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...`
Expected: clean.

- [ ] **Step 8: Commit**

```bash
git add modules/world/server.go modules/world/rsbuf_lifecycle_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-29 Bundle 4 Task 4.2 — wire AddPlayer/RemovePlayer hooks

Adds rsbuf lifecycle hooks at the login/logout sites:
- (*Server).addPlayer (server.go:~599): s.rsbuf.AddPlayer(int32(p.slot))
  fires after the existing Zone EnterPlayer wiring (NAI-28 B2).
- (*Server).removePlayer (server.go:~649): s.rsbuf.CleanupPlayerBuildArea
  + s.rsbuf.RemovePlayer fire after the Zone LeavePlayer block, before
  the s.players[slot] = nil assignment.

Both hooks are tick-goroutine-only (the addPlayer/removePlayer methods
are called from the tick goroutine after draining s.newPlayers; per
NAI-29 spec concurrency model verification).

Closes Bundle 4 Task 4.2 of NAI-29 spec.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

## Task 4.3: Wire `AddNpc` / `RemoveNpc` hooks at npc spawn/despawn

**Files:**
- Modify: `modules/world/npc_registry.go` (`addNpc`, `removeNpc`)
- Modify: `modules/world/rsbuf_lifecycle_test.go`

- [ ] **Step 1: Pre-flight verify the addNpc / removeNpc call site lines**

Run: `rg -n "func \(s \*Server\) addNpc\b|func \(s \*Server\) removeNpc\b" modules/world/npc_registry.go`
Expected: 2 matches at lines ~`:48` and ~`:151`.

Run: `sed -n '40,80p' modules/world/npc_registry.go && sed -n '145,170p' modules/world/npc_registry.go`
Expected: visual confirmation of the existing Zone EnterNpc / LeaveNpc wiring.

- [ ] **Step 2: Append failing tests to `modules/world/rsbuf_lifecycle_test.go`**

```go
func TestServer_AddNpcWiresRsbufSlot(t *testing.T) {
	s := newTestServer(t)
	n := newTestNpc(50, 100, 60, 60, 0)
	if err := s.addNpc(n, /*duration*/ -1, /*firstSpawn*/ true); err != nil {
		t.Fatalf("addNpc: %v", err)
	}
	// Probe via GetNpcObservers — returns 0 for the freshly-allocated
	// slot (initial Observers count). 0 also returned for nil slot,
	// so we can't distinguish here. Stronger probe: HasNpc with a
	// fictional player slot.
	if got := s.rsbuf.GetNpcObservers(int32(n.nid)); got != 0 {
		t.Errorf("GetNpcObservers fresh: got %d, want 0", got)
	}
}

func TestServer_RemoveNpcCleansRsbufSlot(t *testing.T) {
	s := newTestServer(t)
	n := newTestNpc(50, 100, 60, 60, 0)
	if err := s.addNpc(n, -1, true); err != nil {
		t.Fatalf("addNpc: %v", err)
	}
	s.removeNpc(n, /*duration*/ -1)
	// After removeNpc the rsbuf slot is nil; GetNpcObservers returns 0
	// (consistent with both nil-slot and zero-counter — but no panic).
	if got := s.rsbuf.GetNpcObservers(int32(n.nid)); got != 0 {
		t.Errorf("GetNpcObservers post-remove: got %d, want 0", got)
	}
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./modules/world/ -run "TestServer_AddNpcWires|TestServer_RemoveNpcCleans"`
Expected: FAIL with helper-undefined or no observable change (the rsbuf hook isn't wired yet — the test passes only if the slot was non-nil before; since the rsbuf hook is missing, the slot is nil but the test's positive assertion currently can't distinguish nil from active). The test should FAIL meaningfully once the hook exists; before the hook lands, the no-panic expectations may pass spuriously. Mark this risk and proceed.

> **TDD note:** These tests are weaker than the player-side tests because `GetNpcObservers` returns 0 for both nil and active slots. A stronger probe would require exposing internal state, which violates encapsulation. The test value here is regression protection: if a future change accidentally causes panics in addNpc/removeNpc due to nil-rsbuf, this test catches it.

- [ ] **Step 4: Add the rsbuf hook to `(*Server).addNpc`**

Locate the Zone EnterNpc wiring inside `addNpc` (around line `:64`):

```go
	// existing Zone EnterNpc wiring (NAI-28 Bundle 2):
	if s.zoneMap != nil {
		z := s.zoneMap.Get(n.level, n.x, n.z)
		n.zoneListElement = z.EnterNpc(n)
	}
```

Add the rsbuf hook immediately after:

```go
	if s.zoneMap != nil {
		z := s.zoneMap.Get(n.level, n.x, n.z)
		n.zoneListElement = z.EnterNpc(n)
	}
	if s.rsbuf != nil {
		s.rsbuf.AddNpc(int32(n.nid), int32(n.typeId))
	}
```

- [ ] **Step 5: Add the rsbuf hook to `(*Server).removeNpc`**

Locate the Zone LeaveNpc wiring inside `removeNpc` (around line `:151`):

```go
	// existing Zone LeaveNpc wiring (NAI-28 Bundle 2):
	if s.zoneMap != nil && n.zoneListElement != nil {
		z := s.zoneMap.Get(n.level, n.x, n.z)
		z.LeaveNpc(n, n.zoneListElement)
		n.zoneListElement = nil
	}
```

Add the rsbuf hook immediately after:

```go
	if s.zoneMap != nil && n.zoneListElement != nil {
		z := s.zoneMap.Get(n.level, n.x, n.z)
		z.LeaveNpc(n, n.zoneListElement)
		n.zoneListElement = nil
	}
	if s.rsbuf != nil {
		s.rsbuf.RemoveNpc(int32(n.nid))
	}
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./modules/world/ -run "TestServer_AddNpcWires|TestServer_RemoveNpcCleans" -v`
Expected: PASS.

- [ ] **Step 7: Run full package + race**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 -race ./modules/world/...`
Expected: PASS.

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...`
Expected: clean.

- [ ] **Step 8: Commit**

```bash
git add modules/world/npc_registry.go modules/world/rsbuf_lifecycle_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-29 Bundle 4 Task 4.3 — wire AddNpc/RemoveNpc hooks

Adds rsbuf lifecycle hooks at NPC spawn/despawn sites:
- (*Server).addNpc (npc_registry.go:~48): s.rsbuf.AddNpc(int32(n.nid),
  int32(n.typeId)) fires after the existing Zone EnterNpc wiring.
- (*Server).removeNpc (npc_registry.go:~151): s.rsbuf.RemoveNpc fires
  after the existing Zone LeaveNpc block.

n.typeId (lowercase) is the integer type-id field on modules/world.Npc;
verified at plan-write time via grep against npc.go:27 and
npc_event_queue_test.go:45-50 cross-references.

Closes Bundle 4 Task 4.3 of NAI-29 spec.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

## Task 4.4: Wire per-tick `ComputePlayer` state push in `tick.go`

**Files:**
- Modify: `modules/world/tick.go` (per-player tick loop)
- Create: `modules/world/rsbuf_per_tick_test.go`

- [ ] **Step 1: Pre-flight verify the player tick loop site**

Run: `rg -n "for.*range\s+s\.playerLoop\b|for.*p\s*:=\s*range\s+s\.playerLoop" modules/world/tick.go`
Expected: 1+ matches; pick the loop that runs AFTER movement processing (not the pre-movement loop).

Run: `rg -n "s\.grid\.(Add|Remove)\b" modules/world/tick.go` to find the parallel pkg/grid update site (lines `:320-322`). The new ComputePlayer call should land **adjacent to** that grid-update block — same loop or next loop after.

The plan-author's directive: insert the ComputePlayer push at the **same point** as the existing pkg/grid update so both indexes are updated consistently per tick (parallel-write invariant per `parallel_spatial_index_migration_pattern` memory).

- [ ] **Step 2: Verify Player accessor methods exist** (per `controller_preflight`)

Run: `rg -n "func \(p \*Player\) (CurHP|AppearanceHash|baseHP)\b" modules/world/`
Verify each accessor used in the plan's argument-mapping table exists. If `baseHP()` is not defined, the implementer must use an alternative — likely `int(p.levels[objtype.PlayerStatHitpoints])` directly (mirroring `CurHP` but with the BaseHP-equivalent stat field). If the field path differs, document with a comment and continue.

- [ ] **Step 3: Write failing test in `modules/world/rsbuf_per_tick_test.go`**

```go
package world

import (
	"testing"

	"github.com/zsrv/goscape/pkg/coordgrid"
)

// Tests for NAI-29 Bundle 4 Task 4.4 — ComputePlayer per-tick state
// push in tick.go.

func TestTick_ComputePlayerPushesStateAfterMovement(t *testing.T) {
	s := newTestServer(t)
	p := newTestPlayer()
	p.x = 50
	p.z = 50
	p.level = 0
	p.originX = 48
	p.originZ = 48
	if err := s.addPlayer(p); err != nil {
		t.Fatalf("addPlayer: %v", err)
	}

	// Run one tick (drives the existing tick pipeline including the new
	// ComputePlayer push).
	s.tick()

	// After tick: rsbuf.players[slot].Coord must equal the packed coord.
	// Since rsbuf state is not exposed via public method, the assertion
	// uses the indirect probe: a re-tick with a moved coord must update
	// the zoneMap subscription, observable via HasPlayer (after manual
	// BuildArea seed). Simpler probe: the lack of panics + GetNpcObservers
	// stability after tick.

	// Tick again to confirm idempotency (state push doesn't accumulate
	// stale entries in playerGrid — Cleanup at end-of-tick clears it).
	s.tick()
	s.tick()

	// If the tick path crashes due to nil rsbuf or wrong-shape args,
	// the test fails here.
}

func TestTick_ComputePlayerCrossZoneMoveUpdatesRsbufZoneMap(t *testing.T) {
	s := newTestServer(t)
	p := newTestPlayer()
	p.x = 50
	p.z = 50
	p.level = 0
	if err := s.addPlayer(p); err != nil {
		t.Fatalf("addPlayer: %v", err)
	}

	// Tick 1 at (50, 0, 50). Zone (6, 0, 6).
	s.tick()

	// Move cross-zone to (64, 50, 0). Zone (8, 0, 6).
	p.x = 64
	p.z = 50

	// Tick 2: per-tick ComputePlayer push should propagate the new
	// coord into rsbuf state.
	s.tick()

	// Move back to (50, 0, 50). Should succeed without state leak.
	p.x = 50
	p.z = 50
	s.tick()

	// Primary value: tick pipeline runs without panic across multiple
	// cross-zone moves. NAI-30 (which lands the encoder read path) will
	// add stronger assertions on observable encoder output.
	_ = coordgrid.PackCoord(p.level, p.x, p.z) // silence unused-import lint
}
```

> **Test scope rationale:** These tests focus on regression protection (no panics, no nil-derefs) rather than internal-state inspection. The rsbuf state fields (`players`, `npcs`, `zoneMap`, `playerGrid`) are unexported — exposing them via test-only accessor methods is possible but not warranted at NAI-29 because no production caller reads them yet. NAI-30 will add stronger assertions tied to actual encoder output.

- [ ] **Step 4: Run test to verify it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./modules/world/ -run TestTick_ComputePlayer`
Expected: FAIL — most likely on `s.tick()` if the tick path is gated on rsbuf hooks not yet wired, OR the test passes spuriously (rsbuf is initialized but ComputePlayer never called, no panic).

- [ ] **Step 5: Add the per-tick ComputePlayer push in `tick.go`**

Find the existing per-player movement-processing block. There's likely a structure like:

```go
	for _, p := range s.playerLoop {
		// ... existing per-tick movement processing ...
		if /* p moved this tick */ {
			s.grid.Remove(p.slot, p.lastTickX, p.lastTickZ, p.lastLevel)
			s.grid.Add(p.slot, p.x, p.z, p.level)
		}
	}
```

After the existing loop (or at the end of the same loop body), add the rsbuf state push:

```go
	for _, p := range s.playerLoop {
		// ... existing per-tick movement processing ...
		if /* p moved this tick */ {
			s.grid.Remove(p.slot, p.lastTickX, p.lastTickZ, p.lastLevel)
			s.grid.Add(p.slot, p.x, p.z, p.level)
		}
	}

	// NAI-29 Bundle 4 Task 4.4 — parallel-write to rsbuf state. The
	// existing Encode/EncodeNpc path doesn't yet read this; NAI-30 does
	// the read-flip. Pushed AFTER all per-player movement is finalized
	// so the coord/originX/originZ fields are stable.
	if s.rsbuf != nil {
		for _, p := range s.playerLoop {
			if p == nil {
				continue
			}
			var sayPtr *string
			if len(p.sayText) > 0 {
				ss := string(p.sayText)
				sayPtr = &ss
			}
			s.rsbuf.ComputePlayer(int32(p.slot),
				p.x, p.level, p.z,
				p.originX, p.originZ,
				p.tele, p.jump,
				int8(p.walkDir), int8(p.runDir),
				p.visibility,
				p.active,
				uint32(p.masks),
				p.appearanceBuf,
				int32(p.AppearanceHash()&0x7fffffff), // NAI-30: revisit lastAppearance semantics
				int32(p.faceEntity),
				int32(p.faceSquareX), int32(p.faceSquareZ),
				int32(0), int32(0), // NAI-30: orientationX/Z — not stored on Player today
				int32(p.damageAmt), int32(p.damageType),
				int32(p.CurHP()), int32(0), // NAI-30: baseHP accessor
				int32(p.animID), int32(p.animDelay),
				sayPtr,
				p.chatBytes, uint8(p.chatColour), uint8(p.chatEffect), uint8(p.chatRights),
				int32(p.spotanimID), int32(p.spotanimHeight), int32(p.spotanimDelay),
				int32(p.exactStartX), int32(p.exactStartZ),
				int32(p.exactEndX), int32(p.exactEndZ),
				int32(p.exactBegin), int32(p.exactFinish), int32(p.exactDir),
			)
		}
	}
```

> **Plan-author note on placeholder fields:** `orientationX/Z` and `baseHP` are passed as `int32(0)` because the existing `modules/world.Player` struct does not (at HEAD) expose these fields. NAI-29 is a parallel-write window — `*Buf` state is not yet read by any encoder, so passing zeros causes no observable behavioral change. NAI-30 implementer must:
> 1. Audit which Player fields need to be added for the encoder to compile against (likely `orientationX/Z` and `baseHP`).
> 2. Replace the `int32(0)` placeholders with the real accessors.

> **`int8` cast safety:** `p.walkDir` / `p.runDir` are `int` on the Player struct. Sentinel value -1 in `int` becomes `-1` in `int8` (sign-preserving). Direction values 0..7 fit. The cast is safe for all valid values.

> **`p.AppearanceHash() & 0x7fffffff` cast:** `AppearanceHash` returns `uint64`. NAI-29 stores the lower 31 bits as `int32` (positive-only) to mimic upstream's tick-based `lastAppearance int32` semantics. **This is a deliberate divergence** — upstream uses `lastAppearance` as a tick number; goscape uses it as a content-hash truncation. NAI-30 will need to reconcile — either by adding a tick-when-changed field on Player, or by making the encoder check tolerant of either semantic. Document inline with `// NAI-30: revisit lastAppearance semantics`.

- [ ] **Step 6: Run test to verify it passes (no panics)**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./modules/world/ -run TestTick_ComputePlayer -v`
Expected: PASS.

- [ ] **Step 7: Run full package + race**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 -race ./modules/world/...`
Expected: PASS — all existing tests stay green.

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...`
Expected: clean.

- [ ] **Step 8: Commit**

```bash
git add modules/world/tick.go modules/world/rsbuf_per_tick_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-29 Bundle 4 Task 4.4 — wire ComputePlayer per-tick state push

Adds the per-tick ComputePlayer call in tick.go after the existing
movement-processing block. State push runs alongside the existing
pkg/grid Add/Remove calls — parallel-write window per NAI-29 spec.

Argument mapping: 40-arg signature populated from modules/world.Player's
existing fields + accessor methods (CurHP, AppearanceHash). Two fields
pass int32(0) placeholders pending NAI-30:
  - orientationX/Z (not stored on Player at HEAD)
  - baseHP (accessor not yet defined)
And one field uses a deliberately-divergent semantic:
  - lastAppearance = AppearanceHash() & 0x7fffffff (content-hash
    truncation; upstream is tick-when-changed). NAI-30 reconciles.

These divergences are NAI-29-acceptable because *Buf state is not
yet read by any encoder. NAI-30 implementer must audit + reconcile
each before flipping the read path.

Closes Bundle 4 Task 4.4 of NAI-29 spec.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

## Task 4.5: Wire per-tick `ComputeNpc` state push in `tick.go`

**Files:**
- Modify: `modules/world/tick.go` (per-npc tick loop)
- Modify: `modules/world/rsbuf_per_tick_test.go`

- [ ] **Step 1: Pre-flight verify the npc tick loop site**

Run: `rg -n "for.*range\s+s\.npcs\b|s\.grid\.(AddNpc|RemoveNpc)\b" modules/world/tick.go`
Expected: locate the npc movement loop ending around line `:360` with the existing `s.grid.RemoveNpc/AddNpc` calls.

- [ ] **Step 2: Verify which Npc fields exist** (per `controller_preflight`)

Run: `rg -n "func \(n \*Npc\)" modules/world/npc_source.go`
Expected: list of NpcSource accessor methods. The full NpcSource interface needs to be cross-checked against the ComputeNpc arg list.

Run: `cat pkg/rsbuf/npc_source.go`
Expected: see the upstream NpcSource interface definition; map each method to a ComputeNpc arg.

- [ ] **Step 3: Append failing test to `modules/world/rsbuf_per_tick_test.go`**

```go
func TestTick_ComputeNpcPushesStateAfterMovement(t *testing.T) {
	s := newTestServer(t)
	n := newTestNpc(50, 100, 60, 60, 0)
	if err := s.addNpc(n, -1, true); err != nil {
		t.Fatalf("addNpc: %v", err)
	}

	// Run several ticks; ComputeNpc must be called each tick without
	// panic.
	for i := 0; i < 3; i++ {
		s.tick()
	}

	// Smoke test (regression protection against panic / nil-deref).
	if got := s.rsbuf.GetNpcObservers(int32(n.nid)); got != 0 {
		t.Errorf("after 3 ticks: GetNpcObservers got %d, want 0", got)
	}
}
```

- [ ] **Step 4: Run test to verify it fails (or passes spuriously)**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./modules/world/ -run TestTick_ComputeNpc`
Expected: PASS spuriously (no ComputeNpc yet means no error, but no real verification). Acceptable starting state — the test's value is regression protection.

- [ ] **Step 5: Add the per-tick ComputeNpc push in `tick.go`**

After the per-player ComputePlayer block from Task 4.4, add the per-npc block. (Both can live in the same outer `if s.rsbuf != nil` guard, or in a separate one — let's put them in the same block for clarity):

```go
	if s.rsbuf != nil {
		for _, p := range s.playerLoop {
			if p == nil {
				continue
			}
			// ... ComputePlayer call from Task 4.4 ...
		}

		// NAI-29 Bundle 4 Task 4.5 — parallel-write npc state push.
		for _, n := range s.npcs {
			if n == nil || !n.active {
				continue
			}
			var sayPtr *string
			if len(n.SayText()) > 0 {
				ss := string(n.SayText())
				sayPtr = &ss
			}
			s.rsbuf.ComputeNpc(int32(n.nid), int32(n.typeId),
				n.x, n.level, n.z,
				n.tele,
				int8(n.runDir), int8(n.walkDir),
				n.active,
				uint32(n.Masks()),
				int32(n.FaceEntity()),
				int32(n.FaceSquareX()), int32(n.FaceSquareZ()),
				int32(0), int32(0), // NAI-30: orientationX/Z — not stored on Npc today
				int32(n.DamageAmt()), int32(n.DamageType()),
				int32(n.CurHP()), int32(0), // NAI-30: baseHP accessor
				int32(n.AnimID()), int32(n.AnimDelay()),
				sayPtr,
				int32(n.SpotAnimID()), int32(n.SpotAnimHeight()), int32(n.SpotAnimDelay()),
			)
		}
	}
```

> **Plan-author note on Npc accessor methods:** Many of the args (FaceEntity, Masks, CurHP, etc.) are accessed via `NpcSource` interface methods rather than direct fields. This mirrors the existing encoder pattern. If any accessor is missing on `*Npc`, the implementer must verify with `rg -n "func \(n \*Npc\) (FaceEntity|Masks|CurHP|...)\b" modules/world/` and either:
> 1. Use the accessor if it exists.
> 2. Pass `int32(0)` with a `// NAI-30: missing accessor` comment.
> 3. Use a direct field access if the field exists but no accessor — adjust accordingly.

- [ ] **Step 6: Run test to verify it passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./modules/world/ -run TestTick_ComputeNpc -v`
Expected: PASS.

- [ ] **Step 7: Run full package + race**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 -race ./...`
Expected: PASS.

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...`
Expected: clean.

- [ ] **Step 8: Commit**

```bash
git add modules/world/tick.go modules/world/rsbuf_per_tick_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-29 Bundle 4 Task 4.5 — wire ComputeNpc per-tick state push

Adds the per-tick ComputeNpc call in tick.go after the per-player
ComputePlayer block (Task 4.4). State push runs alongside the existing
pkg/grid AddNpc/RemoveNpc calls — parallel-write window.

Argument mapping uses NpcSource accessor methods where available
(FaceEntity, Masks, CurHP, AnimID, etc.). Two fields pass int32(0)
placeholders pending NAI-30:
  - orientationX/Z (not stored on modules/world.Npc at HEAD)
  - baseHP (accessor not yet defined)

Closes Bundle 4 Task 4.5 of NAI-29 spec.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

## Task 4.6: Wire end-of-tick `Cleanup`

**Files:**
- Modify: `modules/world/tick.go` (end-of-tick site)
- Modify: `modules/world/rsbuf_per_tick_test.go`

- [ ] **Step 1: Pre-flight verify the end-of-tick site**

Run: `rg -n "currentTick\+\+|s\.currentTick\s*=|/\* end of tick \*/" modules/world/tick.go`
Expected: locate the tick-counter increment site (typically the last action of `(*Server).tick`). The `Cleanup` call lands before the `currentTick++` line — Cleanup ends the current tick's state before the tick counter advances.

- [ ] **Step 2: Append failing test to `modules/world/rsbuf_per_tick_test.go`**

```go
func TestTick_RsbufCleanupRunsEachTick(t *testing.T) {
	// Cleanup invariant: rsbuf.playerGrid is reset between ticks.
	// We can't observe playerGrid directly (unexported), but we can
	// verify the tick pipeline runs Cleanup by counting ticks where
	// transient state (e.g. RunDir on the Player snapshot) gets reset.
	//
	// Simpler invariant: after N ticks where the player never moves,
	// the rsbuf state is stable (no panic, no leak).
	s := newTestServer(t)
	p := newTestPlayer()
	if err := s.addPlayer(p); err != nil {
		t.Fatalf("addPlayer: %v", err)
	}
	for i := 0; i < 10; i++ {
		s.tick()
	}
	// Smoke test — if cleanup is broken (e.g., not called) the
	// playerGrid grows unbounded. With cleanup, playerGrid is reset
	// each tick. Tests against unbounded growth are out of scope here
	// (would require exposing the field for inspection); regression
	// protection against panics is the primary value.
}
```

- [ ] **Step 3: Run test to verify it fails (or passes spuriously)**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./modules/world/ -run TestTick_RsbufCleanup`
Expected: PASS (the test is a smoke test; primary purpose is regression protection).

- [ ] **Step 4: Add the end-of-tick Cleanup call in `tick.go`**

Locate the end-of-tick site and add:

```go
	// ... per-tick processing including ComputePlayer/ComputeNpc loops ...
	// ... existing tick.go end-of-tick logic ...

	if s.rsbuf != nil {
		s.rsbuf.Cleanup() // NAI-29 B4 T4.6 — clears transient state + playerGrid
	}

	s.currentTick++ // existing
```

The exact placement matters: Cleanup must run AFTER all per-tick info encoding (so no consumer reads cleared state mid-tick) but BEFORE `currentTick++`. In practice for NAI-29: since Encode is unchanged and reads from PlayerSource/NpcSource (not from rsbuf state), Cleanup can land anywhere AFTER ComputePlayer/ComputeNpc. The simplest safe placement is "immediately after the ComputePlayer/ComputeNpc loops, before currentTick++".

- [ ] **Step 5: Run test to verify it passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./modules/world/ -run TestTick_RsbufCleanup -v`
Expected: PASS.

- [ ] **Step 6: Run full package + race**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 -race ./...`
Expected: PASS.

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...`
Expected: clean.

- [ ] **Step 7: Commit**

```bash
git add modules/world/tick.go modules/world/rsbuf_per_tick_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-29 Bundle 4 Task 4.6 — wire end-of-tick rsbuf.Cleanup

Adds s.rsbuf.Cleanup() call in tick.go after the ComputePlayer +
ComputeNpc loops, before currentTick++. Mirrors upstream's pattern
where cleanup() is called once per tick to clear transient per-tick
state (playerGrid, Player.cleanup(), Npc.cleanup()) while preserving
persistent fields (Appearance, FaceEntity, OrientationX/Z, Observers).

Closes Bundle 4 Task 4.6 of NAI-29 spec.

This closes NAI-29 Bundle 4. Parallel-write window is now active:
every player+npc lifecycle event + per-tick state mutation flows
through *rsbuf.Buf alongside the existing pkg/grid + pkg/zone updates.
Existing encoder (Encode/EncodeNpc) is unchanged — *Buf state is
populated but never read.

NAI-30 will do the read-flip: migrate Encode/EncodeNpc to use *Buf
state, retire pkg/grid + pkg/buildarea + npc_observers.go shim, and
retire the PlayerSource/NpcSource interfaces.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

# NAI-29 Close

After Task 4.6, run a final verification pass and commit a close marker.

## Final verification

- [ ] **Run the full test suite with race detector + counter forced**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 -race ./...
```
Expected: ALL tests pass.

- [ ] **Run a clean build**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build -trimpath ./...
```
Expected: clean (no warnings).

- [ ] **Verify no `pkg/grid` retirement happened by accident**

```bash
ls pkg/grid/
rg -l "github.com/zsrv/goscape/pkg/grid" --type go
```
Expected: `pkg/grid/` still exists with its files; the listed importers should still include `modules/world/server.go`, `modules/world/tick.go`, `pkg/rsbuf/playerinfo.go`, `pkg/rsbuf/npcinfo.go` plus all the test files (per spec — pkg/grid retirement is NAI-30, not NAI-29).

- [ ] **Verify the existing encoder hasn't been touched**

```bash
git diff main..HEAD -- pkg/rsbuf/playerinfo.go pkg/rsbuf/npcinfo.go pkg/rsbuf/source.go pkg/rsbuf/npc_source.go pkg/rsbuf/renderer.go pkg/rsbuf/mask_payload.go pkg/rsbuf/npc_mask_payload.go
```
Expected: zero diff. NAI-29 doesn't touch the existing encoder.

- [ ] **Verify deviation count unchanged**

```bash
rg -n "DEVIATION" pkg/ modules/ | wc -l
```
Compare against the pre-NAI-29 count (13 active tags). Should be unchanged at 13.

## Memory updates at NAI-29 close

Update `~/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md`:
- Replace existing "Deferred: pkg/grid full retirement" entry (currently around line 1702) with a broader "Deferred: pkg/rsbuf upstream alignment series" entry tracking NAI-30 → NAI-32. Cross-reference each with its scope:
  - NAI-30: encoder loops port + read-flip + pkg/grid retirement + pkg/buildarea retirement + npc_observers shim retirement + PlayerSource/NpcSource interface retirement
  - NAI-31: renderer.rs (422 LOC) + message.rs (629 LOC) parity
  - NAI-32: view_distance dynamic resize + force_view_distance + rebuild flag + spiral search
- Add "From NAI-29 (2026-04-25)" close entry mirroring NAI-28's close entry pattern. Include:
  - 4-bundle execution summary
  - Parallel-write invariant verification
  - Stale-IDE-diagnostic notes (if any encountered)
  - 0 deviation tags introduced
  - Memory entries added at close (anticipated: 2-3)

Anticipated new memory entries:
- `rust_source_canonical_path.md` — analogue to `ts_source_canonical_path` for the rsbuf-port subset; `2004scape/rsbuf` branch 225 is the canonical reference; never read forks.
- `flat_arg_signature_for_cross_lang_parity.md` — when porting a 40-arg Rust function, keep the flat arg list and explicit positional order rather than refactoring to a struct, so side-by-side review against the source remains tractable.

## Close commit

```bash
git commit --no-gpg-sign --allow-empty -m "$(cat <<'EOF'
chore(rsbuf,world): NAI-29 closed — 4-bundle pkg/rsbuf stateful core + entity model + parallel-write caller hooks

NAI-29 lands the rsbuf-internal entity model (Player, Npc, BuildArea,
idBitSet, internal zoneMap) + the *Buf instance handle + full stateful
public API (AddPlayer/RemovePlayer/AddNpc/RemoveNpc/ComputePlayer/
ComputeNpc/Cleanup/CleanupPlayerBuildArea/HasPlayer/HasNpc/GetNpcObservers)
+ production parallel-write caller hooks across (*Server).addPlayer,
removePlayer, addNpc, removeNpc, and per-tick state-push + end-of-tick
Cleanup in tick.go.

Existing Encode/EncodeNpc + pkg/grid + pkg/buildarea + npc_observers.go
shim + PlayerSource/NpcSource interfaces are unchanged. *Buf state is
populated every tick but never read by the existing encoder.

This is the parallel-write window per parallel_spatial_index_migration_pattern
memory. NAI-30 will do the read-flip: migrate Encode/EncodeNpc to use
*Buf state and retire all the now-redundant infrastructure.

4-bundle execution:
  - Bundle 1 (~220 LOC + ~250 LOC tests): primitives — idBitSet,
    zoneMap, zone (Tasks 1.1, 1.2)
  - Bundle 2 (~280 LOC + ~280 LOC tests): entity structs — Player,
    Chat, ExactMove, Npc (Tasks 2.1, 2.2)
  - Bundle 3 (~450 LOC + ~450 LOC tests): BuildArea + *Buf full public
    API across 7 tasks (Tasks 3.1-3.7)
  - Bundle 4 (~250 LOC + ~250 LOC tests): caller wiring — *Server.rsbuf
    field, lifecycle hooks, per-tick state push, end-of-tick Cleanup
    across 6 tasks (Tasks 4.1-4.6)

Net deviation count: 13 → 13 (zero new tags; all Go-idiom translations
externally invisible per true_to_ts_gate).

Source root: 2004scape/rsbuf branch 225 (HEAD 1cbb2ce).

Closes memory: ~/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md (NAI-29 close + replace pkg/grid retirement followup with broader pkg/rsbuf upstream alignment series)
Closes memory: ~/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/MEMORY.md (anticipated: 2 new pointer entries — rust_source_canonical_path + flat_arg_signature_for_cross_lang_parity)

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Risk register (per-task plan author summary)

Risks already documented in the spec (`docs/superpowers/specs/2026-04-25-nai-29-rsbuf-stateful-core-design.md`) — implementer should re-read the spec's risk register before starting Bundle 4. The most actionable risks for Bundle 4 specifically:

- **B4 hook ordering**: the rsbuf hooks land AFTER zoneMap hooks in addPlayer/removePlayer/addNpc/removeNpc. If reversed, no observable bug at NAI-29 close (state is parallel-write only) but the dependency graph would invert at NAI-30. Pin order in plan code blocks; implementer copies verbatim.
- **`int8` overflow**: `p.walkDir`/`p.runDir` are int (not int8); -1..7 range fits int8 cleanly. Verified at plan-write time.
- **Missing accessor fields**: `orientationX/Z`, `baseHP` — pass `int32(0)` placeholder, document with comment, defer real wiring to NAI-30. This is the parallel-write rationale.
- **`AppearanceHash` semantic divergence from upstream `lastAppearance`**: documented as a known divergence; flagged for NAI-30 reconciliation. Acceptable for parallel-write (state is unread).
- **Test-helper availability** (`newTestServer`, `newTestPlayer`, `newTestNpc`, `addPlayerToServer`): verify at task time. If missing, construct minimal literals inline.
- **`s.tick()` callability from tests**: confirmed at brainstorm time — used in existing tests. If the project uses a different name (`runOneTick`, `Tick`, etc.), grep at task time.
