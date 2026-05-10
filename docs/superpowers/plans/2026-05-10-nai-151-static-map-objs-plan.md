# NAI-151 — static map ground items (loadObjs un-stub) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Un-stub `pkg/gamemap/load.go:243` `loadObjs` so static ground items (bones, knives, pots, etc. encoded in `o{X}_{Z}` cache files) are parsed at gamemap-init, routed into their owning zones at server boot, and replayed to clients via the existing `writeFullFollows` path on zone entry.

**Architecture:** Mirror the existing `npcSpawns` / `populateStaticLocsIntoZones` pattern. Parser layer (`pkg/gamemap`) collects records into `gm.objSpawns` with TS-faithful inline gating on `members && IsFreeToPlay` and `ObjType.Members`. Zone layer adds `Zone.AddStaticObj` paralleling `AddStaticLoc`. World layer reorders `LoadObjTypes` above `gm.Init` to thread configs into the parser, then iterates `ObjSpawns()` at boot and routes each via `zoneMap.Get(...).AddStaticObj`. `Obj` gets a new `IsActive` field paralleling `Loc.IsActive`.

**Tech Stack:** Go 1.26+. Standard goscape packages: `pkg/gamemap`, `pkg/zone`, `modules/world`, `pkg/entity`, `pkg/objtype`. No new dependencies.

---

## File Structure

**Modify:**
- `pkg/entity/obj.go` — add `IsActive bool` field on `Obj` struct (T1).
- `pkg/gamemap/gamemap.go` — add `ObjSpawn` struct, `objSpawns/members/objTypes` fields, `SetMembers/SetObjTypes/ObjSpawns` methods (T2).
- `pkg/gamemap/load.go` — replace stub body of `loadObjs` (T3).
- `pkg/zone/zone.go` — add `AddStaticObj` method (T4).
- `modules/world/server.go` — reorder `LoadParams`+`LoadObjTypes` above `gm.Init`; wire `SetMembers`/`SetObjTypes`; add `populateStaticObjsIntoZones` + call site (T5).

**Create:**
- (Tests are extensions of existing `*_test.go` files in their respective packages — no new test files.)

---

## Task 1: Add `IsActive` to `Obj` struct

**Files:**
- Modify: `pkg/entity/obj.go:6-21` — add `IsActive bool` field
- Test: `pkg/entity/obj_test.go` (new file — confirm absence first)

This task adds the storage backing `Zone.AddStaticObj`'s active-flag write. Mirrors the existing `Loc.IsActive` field at `pkg/entity/loc.go:16`.

- [ ] **Step 1.1: Confirm `pkg/entity/obj_test.go` does not exist**

Run: `ls pkg/entity/obj_test.go 2>&1`
Expected: `ls: cannot access 'pkg/entity/obj_test.go': No such file or directory`

If the file exists, append the new test instead of creating.

- [ ] **Step 1.2: Write the failing test**

Create `pkg/entity/obj_test.go`:

```go
package entity

import "testing"

func TestObjIsActiveDefaultFalse(t *testing.T) {
	o := NewObj(0, 0, 0, LifecycleRespawn, 1, 1)
	if o.IsActive {
		t.Error("fresh obj must have IsActive=false")
	}
}

func TestObjIsActiveSettable(t *testing.T) {
	o := NewObj(0, 0, 0, LifecycleRespawn, 1, 1)
	o.IsActive = true
	if !o.IsActive {
		t.Error("IsActive must be settable to true")
	}
}
```

- [ ] **Step 1.3: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/entity/ -run "TestObjIsActive" -v`
Expected: FAIL — compile error `o.IsActive undefined (type *Obj has no field or method IsActive)`.

- [ ] **Step 1.4: Add the field**

Edit `pkg/entity/obj.go`. After the `LastChange int` line in the `Obj` struct (currently around line 20), add:

```go
	// IsActive is true while the obj is present in its zone's Objs list.
	// Managed by pkg/zone Zone methods (AddStaticObj, AddObj, RemoveObj).
	// Mirrors TS Zone.ts isActive writes (Zone.ts:208,214,295) and
	// pkg/entity/loc.go:16. NAI-151.
	IsActive bool
```

The `Obj` struct after the edit should look like:

```go
type Obj struct {
	NonPathing

	// Construction properties.
	Type  int // ObjType id
	Count int // stack size

	// Runtime state.
	// ReceiverID is UID-space — mirrors TS Engine-TS entity/Obj.ts receiver64.
	// PublicReceiver (-1) for public drops; else the owning player's UID per
	// modules/world.composeUID(username37, slot). Set by worldVarsView.AddObj
	// at modules/world/server_varp.go:169 for private drops.
	ReceiverID int
	Reveal     int // tick countdown until OBJ_REVEAL fires; -1 if already public
	LastChange int // last tick Count was modified; -1 if never

	// IsActive is true while the obj is present in its zone's Objs list.
	// Managed by pkg/zone Zone methods (AddStaticObj, AddObj, RemoveObj).
	// Mirrors TS Zone.ts isActive writes (Zone.ts:208,214,295) and
	// pkg/entity/loc.go:16. NAI-151.
	IsActive bool
}
```

- [ ] **Step 1.5: Run tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/entity/ -run "TestObjIsActive" -v`
Expected: PASS for both tests.

- [ ] **Step 1.6: Run full pkg/entity tests to confirm no regression**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/entity/ -v`
Expected: All tests pass.

- [ ] **Step 1.7: Commit**

```bash
git add pkg/entity/obj.go pkg/entity/obj_test.go
git commit --no-gpg-sign -m "feat(entity): NAI-151 T1 — add Obj.IsActive field

Mirrors entity/loc.go:16 IsActive. Backs the upcoming
Zone.AddStaticObj write per TS Zone.ts:214."
```

---

## Task 2: Add `pkg/gamemap` infrastructure (struct, fields, setters, accessor)

**Files:**
- Modify: `pkg/gamemap/gamemap.go` — add `ObjSpawn`, fields, methods
- Test: `pkg/gamemap/gamemap_test.go` — append setter/accessor tests

Compile-level scaffolding only. No parser body changes yet.

- [ ] **Step 2.1: Write the failing test**

Append to `pkg/gamemap/gamemap_test.go`:

```go
func TestObjSpawnsDefaultEmpty(t *testing.T) {
	gm := New(discardLogger())
	if len(gm.ObjSpawns()) != 0 {
		t.Errorf("ObjSpawns: fresh GameMap should return empty slice; got %d", len(gm.ObjSpawns()))
	}
}

func TestSetMembersAcceptsTrue(t *testing.T) {
	gm := New(discardLogger())
	gm.SetMembers(true)
	if !gm.members {
		t.Error("SetMembers(true) should set gm.members=true")
	}
}

func TestSetMembersDefaultsFalse(t *testing.T) {
	gm := New(discardLogger())
	if gm.members {
		t.Error("fresh GameMap should default members=false")
	}
}

func TestSetObjTypesAcceptsConfigs(t *testing.T) {
	gm := New(discardLogger())
	cfgs := &objtype.ObjTypeConfigs{Configs: []*objtype.ObjType{nil}}
	gm.SetObjTypes(cfgs)
	if gm.objTypes != cfgs {
		t.Error("SetObjTypes should store the supplied pointer")
	}
}
```

The test accesses `gm.members` and `gm.objTypes` (unexported fields) — same-package access works because the test is in `package gamemap`. Confirm via the existing test file's `package` line at gamemap_test.go:1.

- [ ] **Step 2.2: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/gamemap/ -run "TestObjSpawns|TestSetMembers|TestSetObjTypes" -v`
Expected: FAIL — compile errors `undefined: ObjSpawns`, `undefined: members`, etc.

- [ ] **Step 2.3: Add the `ObjSpawn` struct + fields + methods**

Edit `pkg/gamemap/gamemap.go`. After the existing `NpcSpawn` struct definition (around line 17-20), add:

```go
// ObjSpawn records a static ground-obj spawn position from a mapsquare's
// o-file. Mirrors NpcSpawn (above). NAI-151.
type ObjSpawn struct {
	TypeID, Count int
	X, Z, Level   int
}
```

Then in the `GameMap` struct (around line 23-36), after the existing `npcSpawns []NpcSpawn` field, add:

```go
	objSpawns []ObjSpawn
	members   bool                    // NodeMembers flag — set via SetMembers before Init. Mirrors TS GameMap.members. NAI-151.
	objTypes  *objtype.ObjTypeConfigs // optional; consumed by loadObjs gate. nil-OK preserves t.TempDir() test fixtures with empty caches. NAI-151.
```

After the existing `SetLocTypes` method (around line 50-58), add:

```go
// SetMembers registers the world's NodeMembers flag for use by loadObjs.
// Must be called BEFORE Init; calling later has no effect on already-
// loaded static objs. Default false. Mirrors TS GameMap.members.
// NAI-151.
func (gm *GameMap) SetMembers(m bool) {
	gm.members = m
}

// SetObjTypes registers the ObjType configs used by loadObjs to gate
// per-obj members visibility. Must be called BEFORE Init. nil-OK:
// when unset, loadObjs records no objs (preserves test fixtures with
// empty caches). Mirrors TS ObjType.get() inside GameMap.loadObjs.
// NAI-151.
func (gm *GameMap) SetObjTypes(cfgs *objtype.ObjTypeConfigs) {
	gm.objTypes = cfgs
}
```

Then near the existing `NpcSpawns()` accessor (around line 176-177), add:

```go
// ObjSpawns returns the list of static-obj spawn records collected
// during Init. Returned slice is internal — do not mutate. NAI-151.
func (gm *GameMap) ObjSpawns() []ObjSpawn { return gm.objSpawns }
```

- [ ] **Step 2.4: Run tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/gamemap/ -run "TestObjSpawns|TestSetMembers|TestSetObjTypes" -v`
Expected: PASS for all four tests.

- [ ] **Step 2.5: Run full pkg/gamemap tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/gamemap/ -v`
Expected: all existing tests still pass; the four new tests pass.

- [ ] **Step 2.6: Commit**

```bash
git add pkg/gamemap/gamemap.go pkg/gamemap/gamemap_test.go
git commit --no-gpg-sign -m "feat(gamemap): NAI-151 T2 — ObjSpawn struct + SetMembers/SetObjTypes setters

Scaffolding only. Parser body still stubs. Mirrors NpcSpawn /
SetLocTypes patterns at gamemap.go:17-20 / :50-58."
```

---

## Task 3: Implement `loadObjs` parser body

**Files:**
- Modify: `pkg/gamemap/load.go:243-249` — replace stub body
- Test: `pkg/gamemap/load_test.go` — append parser tests

Implements the TS-faithful parser per `Engine-TS/src/engine/GameMap.ts:139-159`. Tests cover: empty input, single/multi tile, level encoding, truncation, both gates (tile-level F2P + ObjType.Members), and edge cases (typeID OOB, nil entry, nil objTypes).

- [ ] **Step 3.1: Write the failing tests (parser fixtures)**

Append to `pkg/gamemap/load_test.go`:

```go
// --- NAI-151: static-obj parser tests ---

// objFileHeader builds the 3-byte record header for a single tile:
// G2(packed coord) + G1(tileCount).
// packed = (level<<12) | (localX<<6) | localZ.
func objFileHeader(level, localX, localZ, tileCount int) []byte {
	packed := (level << 12) | (localX << 6) | localZ
	return []byte{byte(packed >> 8), byte(packed & 0xFF), byte(tileCount)}
}

// objFileEntry builds a 3-byte obj entry: G2(typeID) + G1(count).
func objFileEntry(typeID, count int) []byte {
	return []byte{byte(typeID >> 8), byte(typeID & 0xFF), byte(count)}
}

// freeMapForCoord returns a freemap with a single F2P entry covering (x, z).
// IsFreeToPlay packs by zone; the simplest helper builds via the public
// SetMulti idiom or direct freemap insertion. Use the same approach as
// existing TestSetMulti / freemap-test idioms in this package.
func setFreemapAt(gm *GameMap, x, z int) {
	// gm.freemap is keyed by packed zone coord. Mirror the loader idiom:
	// (x>>3) << 11 | (z>>3) for an 8×8 zone grid.
	zoneX := x >> 3
	zoneZ := z >> 3
	packed := (zoneX << 11) | zoneZ
	gm.freemap[packed] = true
}

// objTypeConfigs builds an ObjTypeConfigs slice with len entries; each
// non-nil index has Members=members[i].
func objTypeConfigs(members map[int]bool) *objtype.ObjTypeConfigs {
	maxID := 0
	for id := range members {
		if id > maxID {
			maxID = id
		}
	}
	cfgs := make([]*objtype.ObjType, maxID+1)
	for id, m := range members {
		cfgs[id] = &objtype.ObjType{Members: m}
	}
	return &objtype.ObjTypeConfigs{Configs: cfgs}
}

func TestLoadObjs_Empty(t *testing.T) {
	gm := newTestGameMap()
	gm.SetObjTypes(objTypeConfigs(map[int]bool{1: false}))
	gm.loadObjs([]byte{}, 0, 0)
	if len(gm.ObjSpawns()) != 0 {
		t.Errorf("empty input: got %d spawns, want 0", len(gm.ObjSpawns()))
	}
}

func TestLoadObjs_SingleTileSingleObj(t *testing.T) {
	const mapX, mapZ = 50, 50
	const localX, localZ = 10, 20
	const level = 0
	const typeID = 100
	const count = 5
	absX, absZ := mapX*64+localX, mapZ*64+localZ

	gm := newTestGameMap()
	gm.SetMembers(false)
	gm.SetObjTypes(objTypeConfigs(map[int]bool{typeID: false}))
	setFreemapAt(gm, absX, absZ)

	data := append(objFileHeader(level, localX, localZ, 1), objFileEntry(typeID, count)...)
	gm.loadObjs(data, mapX, mapZ)

	spawns := gm.ObjSpawns()
	if len(spawns) != 1 {
		t.Fatalf("got %d spawns, want 1", len(spawns))
	}
	got := spawns[0]
	want := ObjSpawn{TypeID: typeID, Count: count, X: absX, Z: absZ, Level: level}
	if got != want {
		t.Errorf("spawn: got %+v, want %+v", got, want)
	}
}

func TestLoadObjs_SingleTileMultiObj(t *testing.T) {
	const mapX, mapZ = 50, 50
	const localX, localZ = 10, 20
	absX, absZ := mapX*64+localX, mapZ*64+localZ

	gm := newTestGameMap()
	gm.SetMembers(false)
	gm.SetObjTypes(objTypeConfigs(map[int]bool{1: false, 2: false, 3: false}))
	setFreemapAt(gm, absX, absZ)

	data := objFileHeader(0, localX, localZ, 3)
	data = append(data, objFileEntry(1, 10)...)
	data = append(data, objFileEntry(2, 20)...)
	data = append(data, objFileEntry(3, 30)...)
	gm.loadObjs(data, mapX, mapZ)

	spawns := gm.ObjSpawns()
	if len(spawns) != 3 {
		t.Fatalf("got %d spawns, want 3", len(spawns))
	}
	for i, want := range []struct{ id, c int }{{1, 10}, {2, 20}, {3, 30}} {
		if spawns[i].TypeID != want.id || spawns[i].Count != want.c {
			t.Errorf("spawn[%d]: got TypeID=%d Count=%d, want TypeID=%d Count=%d",
				i, spawns[i].TypeID, spawns[i].Count, want.id, want.c)
		}
	}
}

func TestLoadObjs_MultiTile(t *testing.T) {
	const mapX, mapZ = 50, 50
	gm := newTestGameMap()
	gm.SetMembers(false)
	gm.SetObjTypes(objTypeConfigs(map[int]bool{1: false, 2: false}))
	setFreemapAt(gm, mapX*64+5, mapZ*64+5)
	setFreemapAt(gm, mapX*64+10, mapZ*64+10)

	data := append(objFileHeader(0, 5, 5, 1), objFileEntry(1, 7)...)
	data = append(data, objFileHeader(0, 10, 10, 1)...)
	data = append(data, objFileEntry(2, 8)...)
	gm.loadObjs(data, mapX, mapZ)

	spawns := gm.ObjSpawns()
	if len(spawns) != 2 {
		t.Fatalf("got %d spawns, want 2", len(spawns))
	}
	if spawns[0].TypeID != 1 || spawns[0].X != mapX*64+5 || spawns[0].Z != mapZ*64+5 {
		t.Errorf("spawn[0]: got %+v", spawns[0])
	}
	if spawns[1].TypeID != 2 || spawns[1].X != mapX*64+10 || spawns[1].Z != mapZ*64+10 {
		t.Errorf("spawn[1]: got %+v", spawns[1])
	}
}

func TestLoadObjs_LevelEncoding(t *testing.T) {
	const mapX, mapZ = 50, 50
	const localX, localZ = 0, 0
	for level := 0; level < 4; level++ {
		t.Run(fmt.Sprintf("level=%d", level), func(t *testing.T) {
			gm := newTestGameMap()
			gm.SetMembers(false)
			gm.SetObjTypes(objTypeConfigs(map[int]bool{1: false}))
			setFreemapAt(gm, mapX*64+localX, mapZ*64+localZ)

			data := append(objFileHeader(level, localX, localZ, 1), objFileEntry(1, 1)...)
			gm.loadObjs(data, mapX, mapZ)
			spawns := gm.ObjSpawns()
			if len(spawns) != 1 {
				t.Fatalf("got %d spawns, want 1", len(spawns))
			}
			if spawns[0].Level != level {
				t.Errorf("Level: got %d, want %d", spawns[0].Level, level)
			}
		})
	}
}

func TestLoadObjs_TruncatedTrailing(t *testing.T) {
	const mapX, mapZ = 50, 50
	gm := newTestGameMap()
	gm.SetMembers(false)
	gm.SetObjTypes(objTypeConfigs(map[int]bool{1: false}))
	setFreemapAt(gm, mapX*64+5, mapZ*64+5)

	// Header claims 2 entries but only 1 entry's bytes follow.
	data := append(objFileHeader(0, 5, 5, 2), objFileEntry(1, 7)...)
	gm.loadObjs(data, mapX, mapZ)

	spawns := gm.ObjSpawns()
	if len(spawns) != 1 {
		t.Fatalf("truncated: got %d spawns, want 1 (loop should stop at p.Len()<3)", len(spawns))
	}
}

func TestLoadObjs_TileGate_F2POnlyServer_NonF2PTile_Drops(t *testing.T) {
	const mapX, mapZ = 50, 50
	const localX, localZ = 5, 5
	gm := newTestGameMap()
	gm.SetMembers(false) // F2P-only server
	gm.SetObjTypes(objTypeConfigs(map[int]bool{1: false}))
	// NOTE: do NOT call setFreemapAt — tile is non-F2P.

	data := append(objFileHeader(0, localX, localZ, 1), objFileEntry(1, 7)...)
	gm.loadObjs(data, mapX, mapZ)
	if len(gm.ObjSpawns()) != 0 {
		t.Errorf("F2P-only server, members tile: got %d spawns, want 0", len(gm.ObjSpawns()))
	}
}

func TestLoadObjs_TileGate_MembersWorld_NonF2PTile_Includes(t *testing.T) {
	const mapX, mapZ = 50, 50
	const localX, localZ = 5, 5
	gm := newTestGameMap()
	gm.SetMembers(true) // members world
	gm.SetObjTypes(objTypeConfigs(map[int]bool{1: false}))
	// NOTE: tile is non-F2P, but members=true bypasses tile gate.

	data := append(objFileHeader(0, localX, localZ, 1), objFileEntry(1, 7)...)
	gm.loadObjs(data, mapX, mapZ)
	if len(gm.ObjSpawns()) != 1 {
		t.Errorf("members world, non-F2P tile: got %d spawns, want 1", len(gm.ObjSpawns()))
	}
}

func TestLoadObjs_TileGate_F2POnlyServer_F2PTile_Includes(t *testing.T) {
	const mapX, mapZ = 50, 50
	const localX, localZ = 5, 5
	absX, absZ := mapX*64+localX, mapZ*64+localZ
	gm := newTestGameMap()
	gm.SetMembers(false)
	gm.SetObjTypes(objTypeConfigs(map[int]bool{1: false}))
	setFreemapAt(gm, absX, absZ)

	data := append(objFileHeader(0, localX, localZ, 1), objFileEntry(1, 7)...)
	gm.loadObjs(data, mapX, mapZ)
	if len(gm.ObjSpawns()) != 1 {
		t.Errorf("F2P server + F2P tile: got %d spawns, want 1", len(gm.ObjSpawns()))
	}
}

func TestLoadObjs_ObjTypeGate_F2POnlyServer_MembersObj_Drops(t *testing.T) {
	const mapX, mapZ = 50, 50
	const localX, localZ = 5, 5
	absX, absZ := mapX*64+localX, mapZ*64+localZ
	gm := newTestGameMap()
	gm.SetMembers(false)
	gm.SetObjTypes(objTypeConfigs(map[int]bool{1: true})) // members-only obj
	setFreemapAt(gm, absX, absZ)                          // F2P tile passes tile gate

	data := append(objFileHeader(0, localX, localZ, 1), objFileEntry(1, 7)...)
	gm.loadObjs(data, mapX, mapZ)
	if len(gm.ObjSpawns()) != 0 {
		t.Errorf("F2P server + members-only obj: got %d spawns, want 0", len(gm.ObjSpawns()))
	}
}

func TestLoadObjs_ObjTypeGate_MembersWorld_MembersObj_Includes(t *testing.T) {
	const mapX, mapZ = 50, 50
	const localX, localZ = 5, 5
	absX, absZ := mapX*64+localX, mapZ*64+localZ
	gm := newTestGameMap()
	gm.SetMembers(true)
	gm.SetObjTypes(objTypeConfigs(map[int]bool{1: true}))
	setFreemapAt(gm, absX, absZ)

	data := append(objFileHeader(0, localX, localZ, 1), objFileEntry(1, 7)...)
	gm.loadObjs(data, mapX, mapZ)
	if len(gm.ObjSpawns()) != 1 {
		t.Errorf("members world + members obj: got %d spawns, want 1", len(gm.ObjSpawns()))
	}
}

func TestLoadObjs_ObjTypeGate_F2POnlyServer_NonMembersObj_Includes(t *testing.T) {
	const mapX, mapZ = 50, 50
	const localX, localZ = 5, 5
	absX, absZ := mapX*64+localX, mapZ*64+localZ
	gm := newTestGameMap()
	gm.SetMembers(false)
	gm.SetObjTypes(objTypeConfigs(map[int]bool{1: false}))
	setFreemapAt(gm, absX, absZ)

	data := append(objFileHeader(0, localX, localZ, 1), objFileEntry(1, 7)...)
	gm.loadObjs(data, mapX, mapZ)
	if len(gm.ObjSpawns()) != 1 {
		t.Errorf("F2P server + non-members obj: got %d spawns, want 1", len(gm.ObjSpawns()))
	}
}

func TestLoadObjs_ObjTypesNil_DropsAll(t *testing.T) {
	const mapX, mapZ = 50, 50
	const localX, localZ = 5, 5
	absX, absZ := mapX*64+localX, mapZ*64+localZ
	gm := newTestGameMap()
	gm.SetMembers(true)
	// NOTE: SetObjTypes NOT called — objTypes==nil
	setFreemapAt(gm, absX, absZ)

	data := append(objFileHeader(0, localX, localZ, 1), objFileEntry(1, 7)...)
	gm.loadObjs(data, mapX, mapZ)
	if len(gm.ObjSpawns()) != 0 {
		t.Errorf("nil objTypes: got %d spawns, want 0 (nil-guard)", len(gm.ObjSpawns()))
	}
}

func TestLoadObjs_TypeIDOutOfRange_Drops(t *testing.T) {
	const mapX, mapZ = 50, 50
	const localX, localZ = 5, 5
	absX, absZ := mapX*64+localX, mapZ*64+localZ
	gm := newTestGameMap()
	gm.SetMembers(true)
	gm.SetObjTypes(objTypeConfigs(map[int]bool{0: false})) // len(Configs)=1
	setFreemapAt(gm, absX, absZ)

	data := append(objFileHeader(0, localX, localZ, 1), objFileEntry(99, 7)...) // typeID=99 > len-1
	gm.loadObjs(data, mapX, mapZ)
	if len(gm.ObjSpawns()) != 0 {
		t.Errorf("typeID OOB: got %d spawns, want 0", len(gm.ObjSpawns()))
	}
}

func TestLoadObjs_TypeIDNilEntry_Drops(t *testing.T) {
	const mapX, mapZ = 50, 50
	const localX, localZ = 5, 5
	absX, absZ := mapX*64+localX, mapZ*64+localZ
	gm := newTestGameMap()
	gm.SetMembers(true)
	// Configs[1] = nil
	cfgs := &objtype.ObjTypeConfigs{Configs: []*objtype.ObjType{nil, nil}}
	gm.SetObjTypes(cfgs)
	setFreemapAt(gm, absX, absZ)

	data := append(objFileHeader(0, localX, localZ, 1), objFileEntry(1, 7)...)
	gm.loadObjs(data, mapX, mapZ)
	if len(gm.ObjSpawns()) != 0 {
		t.Errorf("nil Configs entry: got %d spawns, want 0", len(gm.ObjSpawns()))
	}
}
```

Confirm the `fmt` import is added if not already imported by `load_test.go`. Run `grep -n '"fmt"' pkg/gamemap/load_test.go` — if absent, add it to the import block. Likewise confirm `objtype` import — add `"github.com/zsrv/goscape/pkg/objtype"` if not already in scope.

- [ ] **Step 3.2: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/gamemap/ -run "TestLoadObjs_" -v`
Expected: FAIL — all 13 tests fail because the stub body discards data.

If any test passes before the parser is implemented, that's a test bug (likely `TestLoadObjs_Empty` will pass because empty input + stub both produce 0 spawns — that's expected; the other 12 should fail).

- [ ] **Step 3.3: Replace the `loadObjs` stub body**

Replace the entire `loadObjs` function in `pkg/gamemap/load.go:243-249` with:

```go
// loadObjs records ground-object positions from the o{X}_{Z} file.
// Mirrors LostCityRS/Engine-TS/src/engine/GameMap.ts:139-159.
//
// Wire layout per record: position(G2, packed level<<12|localX<<6|localZ)
// + tile-count(G1) + tile-count × (typeID(G2) + count(G1)).
//
// Two gates mirror TS:
//   - tile gate: skip when !members && !isFreeToPlay(absX, absZ)
//   - objtype gate: include when (objType.Members && members) || !objType.Members
//
// nil objTypes (test fixtures without registered configs) → skip all
// records silently. NAI-151.
func (gm *GameMap) loadObjs(data []byte, mapSquareX, mapSquareZ int) {
	p := packet.NewPacket(data)
	for p.Len() >= 3 {
		packed := int(p.G2())
		tileCount := int(p.G1())
		level := (packed >> 12) & 0x3
		localX := (packed >> 6) & 0x3F
		localZ := packed & 0x3F
		absX := mapSquareX*mapSquareSize + localX
		absZ := mapSquareZ*mapSquareSize + localZ
		for i := 0; i < tileCount && p.Len() >= 3; i++ {
			typeID := int(p.G2())
			count := int(p.G1())
			// Tile gate: skip members-only tile in F2P-only server.
			if !gm.members && !gm.IsFreeToPlay(absX, absZ) {
				continue
			}
			// nil-objTypes guard preserves test fixtures with empty caches.
			if gm.objTypes == nil {
				continue
			}
			if typeID < 0 || typeID >= len(gm.objTypes.Configs) {
				continue
			}
			ot := gm.objTypes.Configs[typeID]
			if ot == nil {
				continue
			}
			// ObjType gate: TS expression `(Members && members) || !Members`.
			if !((ot.Members && gm.members) || !ot.Members) {
				continue
			}
			gm.objSpawns = append(gm.objSpawns, ObjSpawn{
				TypeID: typeID, Count: count, X: absX, Z: absZ, Level: level,
			})
		}
	}
}
```

- [ ] **Step 3.4: Run tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/gamemap/ -run "TestLoadObjs_" -v`
Expected: PASS for all 13 tests.

- [ ] **Step 3.5: Run full pkg/gamemap suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/gamemap/ -v`
Expected: all tests pass; no regressions in `loadNPCs` / `loadGround` etc.

- [ ] **Step 3.6: Commit**

```bash
git add pkg/gamemap/load.go pkg/gamemap/load_test.go
git commit --no-gpg-sign -m "feat(gamemap): NAI-151 T3 — port loadObjs parser

Mirrors Engine-TS GameMap.ts:139-159. Inline gates on members && IsFreeToPlay
and ObjType.Members. nil-objTypes guard preserves t.TempDir() fixtures."
```

---

## Task 4: Add `Zone.AddStaticObj`

**Files:**
- Modify: `pkg/zone/zone.go` — add method adjacent to `AddStaticLoc`
- Test: `pkg/zone/zone_test.go` — append three tests mirroring `TestAddStaticLoc*`

- [ ] **Step 4.1: Write the failing tests**

Append to `pkg/zone/zone_test.go`:

```go
func TestAddStaticObjAppendsToObjs(t *testing.T) {
	z := New(0, 0, 0, 0)
	obj := entity.NewObj(0, 3094, 3106, entity.LifecycleRespawn, 1234, 5)
	z.AddStaticObj(obj)
	if len(z.Objs) != 1 || z.Objs[0] != obj {
		t.Errorf("Objs: got %v, want [obj]", z.Objs)
	}
	if len(z.Events()) != 0 {
		t.Errorf("AddStaticObj should not queue events; got %d", len(z.Events()))
	}
}

func TestAddStaticObjNoEntityEvents(t *testing.T) {
	z := New(0, 0, 0, 0)
	obj := entity.NewObj(0, 0, 0, entity.LifecycleRespawn, 1234, 1)
	z.AddStaticObj(obj)
	if len(z.entityEvents) != 0 {
		t.Errorf("AddStaticObj should not register entityEvents; got %d entries", len(z.entityEvents))
	}
}

func TestAddStaticObjSetsIsActive(t *testing.T) {
	z := New(0, 0, 0, 0)
	obj := entity.NewObj(0, 3094, 3106, entity.LifecycleRespawn, 1234, 1)
	if obj.IsActive {
		t.Fatal("setup: fresh obj must default IsActive=false")
	}
	z.AddStaticObj(obj)
	if !obj.IsActive {
		t.Error("AddStaticObj must set obj.IsActive=true (mirrors TS Zone.addStaticObj Zone.ts:214)")
	}
}
```

- [ ] **Step 4.2: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/zone/ -run "TestAddStaticObj" -v`
Expected: FAIL — compile error `z.AddStaticObj undefined`.

- [ ] **Step 4.3: Add the `AddStaticObj` method**

In `pkg/zone/zone.go`, add the following method directly after `AddStaticLoc` (currently at line 146-153, before `// ---- loc mutations ----` comment continues with `AddLoc`). Place this method either next to `AddStaticLoc` or in the `// ---- obj mutations ----` section before `AddObj` (around line 247-252) — pick whichever placement matches the existing file structure (loc-mutations and obj-mutations are separated by an `// ---- obj mutations ----` divider; place `AddStaticObj` at the top of the obj-mutations block immediately before `AddObj`):

```go
// AddStaticObj appends a static (LifecycleRespawn) obj to z.Objs WITHOUT
// queuing a zone event. Statics are delivered to clients via the
// FullFollows replay on zone entry (modules/world/player_zone.go:42-58),
// not via Enclosed/Follows events. Called once per obj during world init.
// Mirrors LostCityRS/Engine-TS/src/engine/zone/Zone.ts:211-215. NAI-151.
func (z *Zone) AddStaticObj(obj *entity.Obj) {
	z.Objs = append(z.Objs, obj)
	obj.IsActive = true
}
```

- [ ] **Step 4.4: Run tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/zone/ -run "TestAddStaticObj" -v`
Expected: PASS for all three tests.

- [ ] **Step 4.5: Run full pkg/zone suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/zone/ -v`
Expected: all tests pass.

- [ ] **Step 4.6: Commit**

```bash
git add pkg/zone/zone.go pkg/zone/zone_test.go
git commit --no-gpg-sign -m "feat(zone): NAI-151 T4 — Zone.AddStaticObj

Mirrors AddStaticLoc / TS Zone.ts:211-215. Appends to z.Objs, sets
obj.IsActive=true, queues no event. Static objs reach clients via
FullFollows replay on zone entry."
```

---

## Task 5: Server-boot wiring — reorder + `populateStaticObjsIntoZones`

**Files:**
- Modify: `modules/world/server.go:180-220` — move `LoadParams`+`LoadObjTypes` above `gm.Init`; add `SetMembers`/`SetObjTypes` calls
- Modify: `modules/world/server.go:319` (call site) + new helper `populateStaticObjsIntoZones`
- Test: `modules/world/world_zone_test.go` — append three wiring tests

This task depends on both T2 (gamemap setters) and T4 (Zone.AddStaticObj). Run T1-T4 commit hashes via `git log --oneline -5` to confirm they're in place before starting T5.

- [ ] **Step 5.1: Write the failing tests**

Append to `modules/world/world_zone_test.go`:

```go
// --- NAI-151: populateStaticObjsIntoZones wiring ---

func TestPopulateStaticObjsIntoZones_RoutesByCoord(t *testing.T) {
	s := newZoneTestServer(t)
	// Hand-build a GameMap with two ObjSpawns at distinct zones.
	// ZoneMap is keyed by (level, x>>3, z>>3); two spawns far enough
	// apart guarantee distinct zones.
	gm := gamemap.New(discardLogger())
	gm.SetMembers(false)
	gm.SetObjTypes(&objtype.ObjTypeConfigs{Configs: []*objtype.ObjType{
		nil,
		{Members: false},
	}})
	// Use the package-internal ObjSpawn append. Tests in modules/world
	// can't access pkg/gamemap unexported fields; instead drive via the
	// loadObjs path, which we know now works (T3).
	const mapX, mapZ = 50, 50
	// Build an o-file with two records at different tiles.
	header1 := []byte{byte((0<<12 | 5<<6 | 5) >> 8), byte((5 << 6) | 5), 0x01}
	entry1 := []byte{0x00, 0x01, 0x07} // typeID=1, count=7
	header2 := []byte{byte((0<<12 | 50<<6 | 50) >> 8), byte((50 << 6) | 50), 0x01}
	entry2 := []byte{0x00, 0x01, 0x08} // typeID=1, count=8
	data := append(header1, entry1...)
	data = append(data, header2...)
	data = append(data, entry2...)
	// freemap covers both tiles (gm.SetMembers(false) requires F2P tile).
	for _, tile := range [][2]int{{mapX*64 + 5, mapZ*64 + 5}, {mapX*64 + 50, mapZ*64 + 50}} {
		zoneX, zoneZ := tile[0]>>3, tile[1]>>3
		gm.SetFreeMapForTest((zoneX<<11)|zoneZ, true)
	}
	gm.LoadObjsForTest(data, mapX, mapZ) // exposed in T5.2 below

	s.gamemap = gm
	s.populateStaticObjsIntoZones()

	// Both zones now have one static obj each.
	zA := s.zoneMap.Get(0, mapX*64+5, mapZ*64+5)
	zB := s.zoneMap.Get(0, mapX*64+50, mapZ*64+50)
	if len(zA.Objs) != 1 || zA.Objs[0].Count != 7 {
		t.Errorf("zone A: got Objs=%v, want one obj count=7", zA.Objs)
	}
	if len(zB.Objs) != 1 || zB.Objs[0].Count != 8 {
		t.Errorf("zone B: got Objs=%v, want one obj count=8", zB.Objs)
	}
	if zA == zB {
		t.Error("zone A and B should be distinct (tiles 8 zones apart)")
	}
}

func TestPopulateStaticObjsIntoZones_LifecycleRespawnAndActive(t *testing.T) {
	s := newZoneTestServer(t)
	gm := gamemap.New(discardLogger())
	gm.SetMembers(false)
	gm.SetObjTypes(&objtype.ObjTypeConfigs{Configs: []*objtype.ObjType{
		nil,
		{Members: false},
	}})
	const mapX, mapZ = 50, 50
	header := []byte{byte((0<<12 | 5<<6 | 5) >> 8), byte((5 << 6) | 5), 0x01}
	entry := []byte{0x00, 0x01, 0x07}
	gm.SetFreeMapForTest(((mapX*64+5)>>3)<<11|((mapZ*64+5)>>3), true)
	gm.LoadObjsForTest(append(header, entry...), mapX, mapZ)
	s.gamemap = gm

	s.populateStaticObjsIntoZones()

	z := s.zoneMap.Get(0, mapX*64+5, mapZ*64+5)
	if len(z.Objs) != 1 {
		t.Fatalf("Objs: got %d, want 1", len(z.Objs))
	}
	o := z.Objs[0]
	if o.Lifecycle != entitypkg.LifecycleRespawn {
		t.Errorf("Lifecycle: got %v, want LifecycleRespawn", o.Lifecycle)
	}
	if !o.IsActive {
		t.Error("IsActive: got false, want true")
	}
	if o.X != mapX*64+5 || o.Z != mapZ*64+5 {
		t.Errorf("Coords: got (%d,%d), want (%d,%d)", o.X, o.Z, mapX*64+5, mapZ*64+5)
	}
}

func TestPopulateStaticObjsIntoZones_EmptySpawnsNoOp(t *testing.T) {
	s := newZoneTestServer(t)
	gm := gamemap.New(discardLogger())
	s.gamemap = gm
	// gm has no spawns. populateStaticObjsIntoZones should run without panic.
	s.populateStaticObjsIntoZones()
	// No assertion needed beyond "did not panic".
}
```

The tests reference helpers `gm.LoadObjsForTest(...)` and `gm.SetFreeMapForTest(...)` that don't yet exist — these are test-only exports of the unexported `loadObjs` and `freemap[k]=v` operations, needed because the test file is in `package world` and cannot access `pkg/gamemap` unexported state directly. Add them in Step 5.2.

Confirm the imports needed in `world_zone_test.go`. Run `grep -n "^import\\|gamemap\\|objtype\\|entitypkg" modules/world/world_zone_test.go | head -10`. The file currently imports `entitypkg "github.com/zsrv/goscape/pkg/entity"` and `"github.com/zsrv/goscape/pkg/zone"` per existing usage. Add:
- `"github.com/zsrv/goscape/pkg/gamemap"`
- `"github.com/zsrv/goscape/pkg/objtype"`

Also confirm `discardLogger` is accessible (defined in `modules/world/server_test.go:21`).

- [ ] **Step 5.2: Add test-only export helpers in `pkg/gamemap`**

Append to `pkg/gamemap/gamemap.go` (use a `_test_export.go` filename if you want them quarantined, but adding to gamemap.go with explicit "test-only" doc-comments is acceptable and matches the existing pattern in this repo per the existing `TestAddStaticLocPublicAPI` check at gamemap_test.go:176):

Actually, prefer a separate test-export file. Create `pkg/gamemap/export_test.go`:

```go
package gamemap

// LoadObjsForTest exposes the unexported loadObjs parser for tests in
// downstream packages (modules/world). NAI-151.
func (gm *GameMap) LoadObjsForTest(data []byte, mapSquareX, mapSquareZ int) {
	gm.loadObjs(data, mapSquareX, mapSquareZ)
}

// SetFreeMapForTest exposes the unexported freemap for tests. Key is the
// packed zone coord (zoneX<<11 | zoneZ). NAI-151.
func (gm *GameMap) SetFreeMapForTest(packedZoneCoord int, free bool) {
	gm.freemap[packedZoneCoord] = free
}
```

Note: `_test.go` suffix means this file is compiled only for tests (in any package importing `pkg/gamemap` for testing), so the exported symbols are not part of the production API surface. This is the standard Go idiom for cross-package test exports.

- [ ] **Step 5.3: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run "TestPopulateStaticObjsIntoZones" -v`
Expected: FAIL — compile error `s.populateStaticObjsIntoZones undefined`.

- [ ] **Step 5.4: Reorder `LoadParams`+`LoadObjTypes` above `gm.Init` and wire setters**

Current `modules/world/server.go` lines 180-216 read:

```go
locTypes, err := objtype.LoadLocTypes(cfg.CachePath)
if err != nil {
    return nil, fmt.Errorf("load loc types: %w", err)
}
s.locTypes = locTypes

gm := gamemap.New(logger)
gm.SetLocTypes(locTypes)
if err := gm.Init(cfg.CachePath); err != nil {
    return nil, fmt.Errorf("failed to load game map: %w", err)
}
s.gamemap = gm

params, err := objtype.LoadParams(cfg.CachePath)
if err != nil {
    return nil, fmt.Errorf("load params: %w", err)
}
objTypes, err := objtype.LoadObjTypes(cfg.CachePath, params)
if err != nil {
    return nil, fmt.Errorf("load obj types: %w", err)
}
```

Replace with (move `LoadParams`+`LoadObjTypes` above `gm.New`; add `SetMembers`+`SetObjTypes` calls):

```go
locTypes, err := objtype.LoadLocTypes(cfg.CachePath)
if err != nil {
    return nil, fmt.Errorf("load loc types: %w", err)
}
s.locTypes = locTypes

params, err := objtype.LoadParams(cfg.CachePath)
if err != nil {
    return nil, fmt.Errorf("load params: %w", err)
}
objTypes, err := objtype.LoadObjTypes(cfg.CachePath, params)
if err != nil {
    return nil, fmt.Errorf("load obj types: %w", err)
}

gm := gamemap.New(logger)
gm.SetLocTypes(locTypes)
gm.SetMembers(cfg.NodeMembers)
gm.SetObjTypes(objTypes)
if err := gm.Init(cfg.CachePath); err != nil {
    return nil, fmt.Errorf("failed to load game map: %w", err)
}
s.gamemap = gm
```

Then below the gamemap block, **delete** the now-duplicate `params` and `objTypes` declarations (the original lines 193-200), since they're already declared above. The `s.paramTypes = params` and `s.objTypes = objTypes` assignments (currently at lines ~215-216) still need to run — leave those alone.

After your edit, the section should flow as:
1. LoadLocTypes → s.locTypes
2. LoadParams → params (local var)
3. LoadObjTypes → objTypes (local var)
4. gm.New / SetLocTypes / SetMembers / SetObjTypes / Init / s.gamemap = gm
5. (existing) LoadInvTypes, LoadDbTableTypes, LoadDbRowTypes
6. (existing) s.paramTypes = params; s.objTypes = objTypes; ...

Verify with: `grep -n "params, err\\|objTypes, err\\|s\\.paramTypes\\|s\\.objTypes\\b" modules/world/server.go | head` — there should be exactly ONE `params, err :=` and ONE `objTypes, err :=` after the edit.

- [ ] **Step 5.5: Add `populateStaticObjsIntoZones` helper and call site**

In `modules/world/server.go`, add the new helper directly after the existing `populateStaticLocsIntoZones` function (currently ends around line 345):

```go
// populateStaticObjsIntoZones constructs an *entity.Obj per parsed
// ObjSpawn and routes it to its owning Zone via Zone.AddStaticObj.
// Called once at server startup, adjacent to populateStaticLocsIntoZones.
// Mirrors TS GameMap.loadObjs's inline getZone().addStaticObj() call;
// goscape splits the parse (gamemap.loadObjs) from the zone-routing
// (here) because the zone registry lives on Server, not GameMap.
// NAI-151.
func (s *Server) populateStaticObjsIntoZones() {
	for _, spawn := range s.gamemap.ObjSpawns() {
		obj := entitypkg.NewObj(spawn.Level, spawn.X, spawn.Z,
			entitypkg.LifecycleRespawn, spawn.TypeID, spawn.Count)
		obj.Count = spawn.Count
		z := s.zoneMap.Get(spawn.Level, spawn.X, spawn.Z)
		z.AddStaticObj(obj)
	}
	s.log.Info("static objs loaded", "count", len(s.gamemap.ObjSpawns()))
}
```

Note: `obj.Count = spawn.Count` is redundant because `NewObj` already sets it via the constructor's `count` parameter; remove the redundant line.

Final form:

```go
func (s *Server) populateStaticObjsIntoZones() {
	for _, spawn := range s.gamemap.ObjSpawns() {
		obj := entitypkg.NewObj(spawn.Level, spawn.X, spawn.Z,
			entitypkg.LifecycleRespawn, spawn.TypeID, spawn.Count)
		z := s.zoneMap.Get(spawn.Level, spawn.X, spawn.Z)
		z.AddStaticObj(obj)
	}
	s.log.Info("static objs loaded", "count", len(s.gamemap.ObjSpawns()))
}
```

Then add the call site in `Server.New`. Currently `s.populateStaticLocsIntoZones()` is called at line 319; immediately after that line, add:

```go
s.populateStaticObjsIntoZones()
```

So the boot tail reads:

```go
    s.populateStaticLocsIntoZones()
    s.populateStaticObjsIntoZones()

    return s, nil
}
```

- [ ] **Step 5.6: Run tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run "TestPopulateStaticObjsIntoZones" -v`
Expected: PASS for all three tests.

- [ ] **Step 5.7: Run full modules/world suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -v 2>&1 | tail -40`
Expected: all tests pass. If there's a regression in `TestServer*` tests due to the LoadParams/LoadObjTypes reorder, investigate immediately — the reorder must not change runtime semantics.

- [ ] **Step 5.8: Commit**

```bash
git add modules/world/server.go modules/world/world_zone_test.go pkg/gamemap/export_test.go
git commit --no-gpg-sign -m "feat(world): NAI-151 T5 — populateStaticObjsIntoZones + boot wiring

Reorders LoadParams/LoadObjTypes above gm.Init so SetObjTypes can
thread configs into the parser. Adds populateStaticObjsIntoZones
adjacent to populateStaticLocsIntoZones, called from Server.New."
```

---

## Task 6: End-to-end zone-entry replay test

**Files:**
- Test: `modules/world/player_zone_test.go` — append one e2e test

This test asserts the full chain: server-boot static-obj wiring → writeFullFollows replay → client-bound packet stream contains OBJ_ADD for the static obj. Mirrors the existing `TestWriteFullFollows_ObjAdd_RespawnEmits` shape at player_zone_test.go:419.

- [ ] **Step 6.1: Read the existing template test**

Run: `grep -n "TestWriteFullFollows_ObjAdd_RespawnEmits\\|TestStaticObjReplays" modules/world/player_zone_test.go`
Then `sed -n '415,460p' modules/world/player_zone_test.go` to read the template body.

If `TestStaticObjReplays...` already exists, skip this task entirely (would mean a prior commit already shipped this) — coordinate with controller.

- [ ] **Step 6.2: Write the failing test**

Mirrors `TestWriteFullFollows_ObjAdd_RespawnEmits` at player_zone_test.go:419 — same idiom: `newZoneTestServer` → `newZoneTestPlayer` → `drainConn` → `writeFullFollows` → `flushWrite` → byte-length assertion. Difference: the obj reaches `z.Objs` via `populateStaticObjsIntoZones`, not direct `z.Objs = append(...)`.

Append to `modules/world/player_zone_test.go`:

```go
// TestStaticObjReplaysOnZoneEntry pins the full chain: o-file parse →
// populateStaticObjsIntoZones → writeFullFollows → client receives
// OBJ_ADD on zone entry. Mirrors TestWriteFullFollows_ObjAdd_RespawnEmits
// (player_zone_test.go:419) but routes the obj through the boot helper
// instead of direct z.Objs append. NAI-151.
//
// Wire shape: FullFollows header (3) + PartialFollows wrapper (3) +
// ObjAdd nested (6) = 12 bytes.
func TestStaticObjReplaysOnZoneEntry(t *testing.T) {
	s := newZoneTestServer(t)
	gm := gamemap.New(discardLogger())
	gm.SetMembers(false)
	gm.SetObjTypes(&objtype.ObjTypeConfigs{Configs: []*objtype.ObjType{
		nil,
		{Members: false},
	}})
	// Place the static obj at the same coords newZoneTestPlayer uses
	// (3094, 3106, level=0) so the player's origin observes it.
	const absX, absZ = 3094, 3106
	const mapX, mapZ = absX / 64, absZ / 64
	const localX, localZ = absX % 64, absZ % 64
	gm.SetFreeMapForTest((absX>>3)<<11|(absZ>>3), true)
	header := []byte{
		byte((0<<12 | localX<<6 | localZ) >> 8),
		byte((0<<12 | localX<<6 | localZ) & 0xFF),
		0x01,
	}
	entry := []byte{0x00, 0x01, 0x07} // typeID=1, count=7
	gm.LoadObjsForTest(append(header, entry...), mapX, mapZ)
	s.gamemap = gm
	s.populateStaticObjsIntoZones()

	z := s.zoneMap.Get(0, absX, absZ)
	if len(z.Objs) != 1 {
		t.Fatalf("setup: zone Objs len got %d, want 1 (populateStaticObjsIntoZones did not route)", len(z.Objs))
	}
	// LifecycleRespawn + LifecycleTick=0 + currentTick=1 → CheckLifecycle: 0<1 → alive.
	if !z.Objs[0].IsActive {
		t.Fatal("setup: static obj IsActive should be true after AddStaticObj")
	}

	p, cc := newZoneTestPlayer(t, s, 1, absX, absZ, 0)
	received := drainConn(t, cc)
	p.writeFullFollows(z, 1) // LastLifecycleTick=0 ≠ 1 → not skipped.
	p.client.flushWrite()
	got := <-received
	// FullFollows header (3) + PartialFollows wrapper (3) + ObjAdd (6) = 12.
	if len(got) != 12 {
		t.Errorf("want 12 bytes (FullFollows+PartialFollows+ObjAdd for static obj); got %d (% x)", len(got), got)
	}
}
```

Confirm imports: `gamemap`, `objtype` (the existing file already imports `entitypkg`, `zone`, `gameserver`). Run `grep -n "^import\\|^\t\"" modules/world/player_zone_test.go | head -20` and add the missing import lines.

- [ ] **Step 6.3: Run test to verify it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run "TestStaticObjReplaysOnZoneEntry" -v`
Expected: PASS (because all the wiring from T5 is already complete) — OR FAIL if there's a bug in the wiring that this e2e test surfaces. If it passes immediately, that's a "test passes for wrong reason" risk per `test_passes_for_wrong_reason.md` — verify by temporarily setting the static obj's `LifecycleTick` so `CheckLifecycle` fails, re-run, confirm test fails, then revert.

- [ ] **Step 6.4: If failing, debug and re-run until passing**

Most likely failure modes:
- Packet-byte assertion off-by-N: re-read template test for exact byte count.
- Player pose vs zone-origin mismatch: ensure `p.originX/Z` matches the zone the obj is in.
- Helper-function name mismatch: confirm `newTestPlayer` exists; if not, check whether `newTestPlayerForZone` is the right one.

- [ ] **Step 6.5: Commit**

```bash
git add modules/world/player_zone_test.go
git commit --no-gpg-sign -m "test(world): NAI-151 T6 — e2e static-obj zone-entry replay

Pins the full chain: o-file parse → populateStaticObjsIntoZones →
writeFullFollows → client packet stream contains OBJ_ADD."
```

---

## Task 7: Repo-wide validation

**Files:** none

- [ ] **Step 7.1: Run the full repo test suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`
Expected: all packages pass. If any pre-existing failure surfaces, verify it predates T1 by checking out HEAD~5 (or the controller's pre-NAI-151 baseline) and re-running — per `verify_implementer_claims.md`.

- [ ] **Step 7.2: Run go vet**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...`
Expected: no warnings.

- [ ] **Step 7.3: Run with race detector for the touched packages**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./pkg/gamemap/ ./pkg/zone/ ./pkg/entity/ ./modules/world/`
Expected: pass; no race warnings.

- [ ] **Step 7.4: Confirm no commit-content/diff drift**

Run: `git log --oneline -8`
Expected: 6 commits in order T1 → T6, all bearing NAI-151 trailer in the subject.

Run: `git diff main~6..HEAD --stat`
Expected: changes confined to `pkg/entity/`, `pkg/gamemap/`, `pkg/zone/`, `modules/world/server.go`, `modules/world/world_zone_test.go`, `modules/world/player_zone_test.go`. No drive-by edits in unrelated packages.

If any unexpected file appears in the stat, investigate immediately — per `implementer_commit_content_verify.md`.

---

## Task 8: Reviewer pass

**Files:** none (review-only)

- [ ] **Step 8.1: Dispatch reviewer subagent on Sonnet**

Per `superpowers_code_reviewer_model.md`, the `superpowers:code-reviewer` agent must run on Sonnet (or smaller), never Opus.

Dispatch via Agent tool:
- subagent_type: `feature-dev:code-reviewer` (or `superpowers:code-reviewer` if available)
- model: `sonnet`
- prompt: focus on commits T1-T6 (`git log --oneline -7`) for NAI-151. Verify TS-faithful gates against `Engine-TS/src/engine/GameMap.ts:139-159`. Verify deviation §8 of `docs/superpowers/specs/2026-05-10-nai-151-static-map-objs-design.md` matches what was shipped. Output: confidence-filtered findings only; under 300 words.

- [ ] **Step 8.2: Address findings**

If reviewer surfaces issues:
- True bugs / TS-fidelity divergences → fix in a new T8.X commit before close.
- Style / nit feedback → address inline only if it improves clarity; otherwise note in close commit.
- Confidence-low findings → defer; document in NAI-N+1 if significant.

---

## Task 9: Close commit

**Files:** none (chore commit only)

- [ ] **Step 9.1: Write close commit**

Per `close_commit_memory_trailer.md`, the close commit must include a `Closes memory:` trailer if any new memory entries were written during this sub-spec.

Run:

```bash
git commit --allow-empty --no-gpg-sign -m "$(cat <<'EOF'
chore(close): NAI-151 — static map ground items (loadObjs un-stub)

Closes user-reported "ground items do not spawn anywhere" symptom.

T1: Obj.IsActive field (mirrors Loc.IsActive)
T2: pkg/gamemap ObjSpawn struct + SetMembers/SetObjTypes setters
T3: loadObjs parser body (Engine-TS GameMap.ts:139-159)
T4: Zone.AddStaticObj
T5: populateStaticObjsIntoZones + boot reorder
T6: e2e static-obj zone-entry replay test

Cascade-tail: pickup-respawn cycling deferred to NAI-152 if smoke
shows broken behaviour; verified per §6.5 user-driven smoke.
EOF
)"
```

If new memory entries are warranted (e.g., test-helper `LoadObjsForTest` / `SetFreeMapForTest` pattern is reusable), append `Closes memory: <entry>.md` lines to the trailer per `close_commit_memory_trailer.md`.

---

## Task 10: User-driven smoke

**Files:** none (handoff)

Per `smoke_test_server_handoff.md`, the Java client smoke must run on the user's host, not in this sandbox.

- [ ] **Step 10.1: Hand off to user**

Output the resume prompt:

> NAI-151 implementation complete (commits at HEAD~9..HEAD). Please:
> 1. Build and launch goscape against the production cache: `make build-image && ./run-server.sh` (or your usual command).
> 2. Confirm boot log shows `static objs loaded count=N` with N > 0.
> 3. Connect with the Java client; walk to a known static-obj area (e.g., bones at Lumbridge ~`(3221, 3219)` or knife at Cooking Guild).
> 4. Confirm static ground items appear visible.
> 5. Pickup test: pick up a static obj. If it disappears and never respawns within the configured `ObjType.respawnrate` ticks, that's pickup-respawn breakage → I'll open NAI-152.
>
> Report back what you see.

- [ ] **Step 10.2: Conditional NAI-152**

If smoke shows static objs visible AND pickup-respawn works → close NAI-151 cleanly.
If smoke shows static objs visible but pickup-respawn broken → open NAI-152 per spec §10 / R3.
If smoke shows static objs still missing → revert close and investigate per `cache_staleness_masquerades_as_encoder_bug.md` (cache vs server-state ordering).

---

## Self-Review

Spec coverage check:
- §3 (TS source) → T3 implements parser body matching wire layout.
- §5.1 (gamemap extensions) → T2 covers struct + setters; T3 covers parser.
- §5.2 (Zone.AddStaticObj) → T4.
- §5.3 (server-boot reorder + populateStaticObjsIntoZones) → T5.
- §6.1 (parser tests, 15 cases listed in spec) → T3 ships 13 cases; cases #14 (typeID OOB) and #15 (nil entry) included as `TestLoadObjs_TypeIDOutOfRange_Drops` and `TestLoadObjs_TypeIDNilEntry_Drops`. Cases #5 (LevelEncoding) covered as a sub-test loop. Two original spec cases (`SingleTileMultiObj`, `MultiTile`) ship distinctly. Coverage is the same 13 distinct behaviours; spec's "15" double-counted.
- §6.2 (Zone tests) → T4 ships 3 tests matching the spec.
- §6.3 (server-boot wiring tests) → T5 ships 3 tests.
- §6.4 (e2e replay) → T6.
- §6.5 (user smoke) → T10.
- §7 (cadence) → T7-T9.
- §8 (deviations) → T1 introduces `Obj.IsActive` field — this is the one item NOT explicitly called out in spec §8, because the spec assumed the field existed (preflight surfaced its absence). Document in T9 close commit body OR open as `NAI-151-D3` deviation.

Placeholder scan: every code step shows complete code; no "implement appropriately" or "similar to above" placeholders. Test bodies are concrete and runnable.

Type consistency: `gm.members`, `gm.objTypes`, `gm.objSpawns` shapes match between T2 (declaration) and T3 (consumer) and T5 (`s.gamemap.ObjSpawns()`). `Obj.IsActive` declared in T1 used by T4. `populateStaticObjsIntoZones` signature `func (s *Server) ()` consistent across T5 declaration and T5/T6 callers. `entitypkg.NewObj` signature matches existing usage at world_zone_test.go:32 (verified preflight).

One gap: T6's test uses helpers `outBytesForTest` / `containsOpcodeForTest` that may not exist. Step 6.2 explicitly directs the implementer to read the existing template (`TestWriteFullFollows_ObjAdd_RespawnEmits`) and copy its idiom. This is an acceptable instruction because the existing test idiom IS load-bearing; codifying it here without verification would risk plan-author drift per `plan_runnable_test_fixtures.md`.
