# Sub-spec 5a: Static Loc Loading — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: superpowers:subagent-driven-development (recommended).

**Goal:** Parse `l{X}_{Z}` cache files at world init into `*entity.Loc` with `LifecycleRespawn`, populate zones via `Zone.AddStaticLoc`, retain raw m/l bytes for 5b.

**Architecture:** Four surgical edits across three packages + one new test file. Follows the existing `loadNPCs` accumulate-in-gamemap-then-drain-from-server pattern.

**Tech Stack:** Go 1.26. `pkg/io/packet.GSmart`, `pkg/entity.NewLoc`, existing `pkg/zone.Zone`.

**Build prefix:** `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go ...`.
**Commit flag:** `--no-gpg-sign`.

**Spec reference:** `docs/superpowers/specs/2026-04-20-static-loc-loading-design.md`.

---

## File Structure

**Create:** (none)

**Modify:**
- `pkg/zone/zone.go` — add `AddStaticLoc`
- `pkg/zone/zone_test.go` — 2 new tests
- `pkg/gamemap/gamemap.go` — add mData, lData, staticLocs fields + 3 accessors + init in New
- `pkg/gamemap/load.go` — replace `loadLocs` stub with real parser
- `pkg/gamemap/gamemap_test.go` — 4 new tests
- `modules/world/server.go` — startup pass calling `AddStaticLoc` per static loc
- `modules/world/server_zone_static_test.go` (new) — integration test

**Raw-byte retention touches**: the existing loop inside `gamemap.Init` that reads `m{X}_{Z}` and `l{X}_{Z}` files. Confirm the exact lines during Task 2.

---

## Task 1: `Zone.AddStaticLoc` + tests

Foundation first. Adds the method and verifies its semantics (no events queued, preserved across Reset).

**Files:**
- Modify: `pkg/zone/zone.go`
- Modify: `pkg/zone/zone_test.go`

- [ ] **Step 1.1: Write the failing tests**

Append to `pkg/zone/zone_test.go`:

```go
func TestAddStaticLocAppendsToLocs(t *testing.T) {
	z := New(0, 0, 0, 0)
	loc := entity.NewLoc(0, 3094, 3106, 1, 1, entity.LifecycleRespawn, 100, 5, 2)
	z.AddStaticLoc(loc)
	if len(z.Locs) != 1 || z.Locs[0] != loc {
		t.Errorf("Locs: got %v, want [loc]", z.Locs)
	}
	if len(z.Events()) != 0 {
		t.Errorf("AddStaticLoc should not queue events; got %d", len(z.Events()))
	}
}

func TestAddStaticLocNoEntityEvents(t *testing.T) {
	z := New(0, 0, 0, 0)
	loc := entity.NewLoc(0, 0, 0, 1, 1, entity.LifecycleRespawn, 1, 0, 0)
	z.AddStaticLoc(loc)
	if len(z.entityEvents) != 0 {
		t.Errorf("AddStaticLoc should not register entityEvents; got %d entries", len(z.entityEvents))
	}
}

func TestResetPreservesStaticLocs(t *testing.T) {
	z := New(0, 0, 0, 0)
	loc := entity.NewLoc(0, 0, 0, 1, 1, entity.LifecycleRespawn, 1, 0, 0)
	z.AddStaticLoc(loc)
	// Seed events too, to prove Reset clears those but not Locs.
	z.events = []ZoneEvent{{Type: ZoneEventEnclosed, Bytes: []byte{1}}}
	z.ComputeShared()
	z.Reset()
	if len(z.Locs) != 1 {
		t.Errorf("Locs should survive Reset; got %d", len(z.Locs))
	}
	if len(z.Events()) != 0 || z.Shared() != nil {
		t.Errorf("per-tick state should be cleared; events=%d shared=%v", len(z.Events()), z.Shared())
	}
}
```

- [ ] **Step 1.2: Run tests — verify FAIL**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/zone/ -run 'TestAddStaticLoc|TestResetPreservesStaticLocs' -v`
Expected: FAIL — `z.AddStaticLoc undefined`.

- [ ] **Step 1.3: Implement `AddStaticLoc`**

Append to `pkg/zone/zone.go` (near `AddLoc`, probably after it for readability):

```go
// AddStaticLoc appends a static (LifecycleRespawn) loc to z.Locs WITHOUT
// queuing a zone event. Statics are delivered to clients via the mapsquare
// download (sub-spec 5b), not via zone events. Called once per loc during
// world init.
func (z *Zone) AddStaticLoc(loc *entity.Loc) {
	z.Locs = append(z.Locs, loc)
}
```

- [ ] **Step 1.4: Run tests — verify PASS**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/zone/ -v`
Expected: all PASS (3 new + existing 29).

- [ ] **Step 1.5: Commit**

```bash
git add pkg/zone/zone.go pkg/zone/zone_test.go
git commit --no-gpg-sign -m "feat(zone): add AddStaticLoc for LifecycleRespawn locs

Statics append to z.Locs without queuing zone events — they're delivered
to clients via the mapsquare download (sub-spec 5b), not through the
Enclosed/Follows event pipeline. Distinct from AddLoc which targets
dynamic (Despawn-lifecycle) locs.

Tests verify no events or entityEvents entries are produced, and that
Reset preserves statics while clearing per-tick state."
```

---

## Task 2: `GameMap` byte retention + accessors

Wire the fields and accessors. Loc parser filled in Task 3.

**Files:**
- Modify: `pkg/gamemap/gamemap.go`
- Modify: `pkg/gamemap/gamemap_test.go`

- [ ] **Step 2.1: Read `gamemap.go` first**

Before editing, open the file and locate:
- The `GameMap` struct definition.
- The `New` function.
- Inside `Init`, the loop where `os.ReadFile(mPath)` / `os.ReadFile(lPath)` happens and their results are passed to `loadGround` / `loadLocs`.

- [ ] **Step 2.2: Write failing test for byte retention**

Append to `pkg/gamemap/gamemap_test.go`:

```go
func TestGameMapRetainsRawBytes(t *testing.T) {
	dir := t.TempDir()
	mapsDir := filepath.Join(dir, "client", "maps")
	if err := os.MkdirAll(mapsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mapsDir, "m50_51"), []byte{0xDE, 0xAD}, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mapsDir, "l50_51"), []byte{0xBE, 0xEF}, 0644); err != nil {
		t.Fatal(err)
	}

	gm := New(discardLogger())
	if err := gm.Init(dir); err != nil {
		t.Fatal(err)
	}
	if got := gm.LandBytes(50, 51); !bytes.Equal(got, []byte{0xDE, 0xAD}) {
		t.Errorf("LandBytes: got %v, want [0xDE, 0xAD]", got)
	}
	if got := gm.LocBytes(50, 51); !bytes.Equal(got, []byte{0xBE, 0xEF}) {
		t.Errorf("LocBytes: got %v, want [0xBE, 0xEF]", got)
	}
	if gm.LandBytes(0, 0) != nil {
		t.Errorf("LandBytes(0,0) unloaded should return nil; got %v", gm.LandBytes(0, 0))
	}
}
```

Add imports as needed: `"bytes"`, `"os"`, `"path/filepath"`.

- [ ] **Step 2.3: Run — verify FAIL**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/gamemap/ -run TestGameMapRetainsRawBytes -v`
Expected: FAIL — `gm.LandBytes undefined`.

- [ ] **Step 2.4: Implement fields, accessors, and init population**

In `pkg/gamemap/gamemap.go`, add fields to the `GameMap` struct:

```go
mData      map[uint16][]byte
lData      map[uint16][]byte
staticLocs []*entity.Loc
```

Add import `"github.com/zsrv/goscape/pkg/entity"`.

In `New`, initialise the maps:

```go
gm := &GameMap{
	// ... existing fields ...
	mData: make(map[uint16][]byte),
	lData: make(map[uint16][]byte),
}
return gm
```

(Exact pattern follows whatever `New` currently does — merge carefully.)

In `Init`, at the spot where `mBytes` is read and passed to `loadGround`:

```go
mBytes, err := os.ReadFile(mPath)
// (keep existing error handling)
gm.mData[uint16((mapX<<8)|mapZ)] = mBytes
gm.loadGround(mBytes, mapX, mapZ)
```

And symmetrically for lBytes:

```go
lBytes, err := os.ReadFile(lPath)
// (keep existing error handling)
gm.lData[uint16((mapX<<8)|mapZ)] = lBytes
gm.loadLocs(lBytes, mapX, mapZ)
```

Add the three accessors (at the bottom of `gamemap.go`):

```go
// StaticLocs returns the parsed static locs accumulated during Init.
// Pointers are stable for the lifetime of GameMap.
func (gm *GameMap) StaticLocs() []*entity.Loc { return gm.staticLocs }

// LandBytes returns the raw (on-disk) bytes of m{X}_{Z}, or nil if unloaded.
func (gm *GameMap) LandBytes(mapX, mapZ int) []byte {
	return gm.mData[uint16((mapX<<8)|mapZ)]
}

// LocBytes returns the raw (on-disk) bytes of l{X}_{Z}, or nil if unloaded.
func (gm *GameMap) LocBytes(mapX, mapZ int) []byte {
	return gm.lData[uint16((mapX<<8)|mapZ)]
}
```

- [ ] **Step 2.5: Run — verify PASS**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/gamemap/ -v`
Expected: PASS (new test + all existing).

- [ ] **Step 2.6: Commit**

```bash
git add pkg/gamemap/gamemap.go pkg/gamemap/gamemap_test.go
git commit --no-gpg-sign -m "feat(gamemap): retain raw m/l bytes + staticLocs accessors

New fields mData / lData / staticLocs on GameMap. LandBytes and LocBytes
accessors return raw on-disk bytes for a mapsquare (nil if unloaded).
StaticLocs returns parsed static locs — empty until the loadLocs stub is
replaced in the next commit.

Prerequisite for sub-spec 5a's loc parser and 5b's opcode 150 response
serving."
```

---

## Task 3: Replace `loadLocs` stub with real parser

**Files:**
- Modify: `pkg/gamemap/load.go`
- Modify: `pkg/gamemap/gamemap_test.go`

- [ ] **Step 3.1: Write the failing parser tests**

Append to `pkg/gamemap/gamemap_test.go`:

```go
func TestLoadLocsParsesKnownFixture(t *testing.T) {
	// Encode: locID delta 101 (1 byte 0x65), coord delta 200 (2 bytes 0x80C8),
	// info = (shape=5 << 2) | (angle=2) = 0x16, inner terminator 0x00,
	// outer terminator 0x00.
	// coord packed = 199 = (0<<12) | (3<<6) | 7 → level=0 localX=3 localZ=7.
	fixture := []byte{
		0x65,
		0x80, 0xC8,
		0x16,
		0x00,
		0x00,
	}

	gm := New(discardLogger())
	gm.loadLocs(fixture, 50, 51)

	statics := gm.StaticLocs()
	if len(statics) != 1 {
		t.Fatalf("StaticLocs: got %d, want 1; fixture=%v", len(statics), fixture)
	}
	loc := statics[0]
	if loc.Level != 0 {
		t.Errorf("Level: got %d, want 0", loc.Level)
	}
	if loc.X != 50*64+3 {
		t.Errorf("X: got %d, want %d", loc.X, 50*64+3)
	}
	if loc.Z != 51*64+7 {
		t.Errorf("Z: got %d, want %d", loc.Z, 51*64+7)
	}
	if loc.Type() != 100 {
		t.Errorf("Type: got %d, want 100", loc.Type())
	}
	if loc.Shape() != 5 {
		t.Errorf("Shape: got %d, want 5", loc.Shape())
	}
	if loc.Angle() != 2 {
		t.Errorf("Angle: got %d, want 2", loc.Angle())
	}
	if loc.Lifecycle != entity.LifecycleRespawn {
		t.Errorf("Lifecycle: got %v, want Respawn", loc.Lifecycle)
	}
}

func TestLoadLocsMultipleLocIDs(t *testing.T) {
	// Two locs: ID 10 at coord 0, ID 21 at coord 0.
	fixture := []byte{
		0x0B, // locID delta 11 → locID = 10
		0x01, // coord delta 1 → coord = 0
		0x00, // info (shape 0, angle 0)
		0x00, // inner end
		0x0B, // locID delta 11 → locID = 21
		0x01, // coord delta 1 → coord = 0
		0x00, // info
		0x00, // inner end
		0x00, // outer end
	}
	gm := New(discardLogger())
	gm.loadLocs(fixture, 0, 0)

	if got := len(gm.StaticLocs()); got != 2 {
		t.Errorf("StaticLocs count: got %d, want 2", got)
	}
	if gm.StaticLocs()[0].Type() != 10 {
		t.Errorf("first loc type: got %d, want 10", gm.StaticLocs()[0].Type())
	}
	if gm.StaticLocs()[1].Type() != 21 {
		t.Errorf("second loc type: got %d, want 21", gm.StaticLocs()[1].Type())
	}
}

func TestLoadLocsEmptyFile(t *testing.T) {
	gm := New(discardLogger())
	// Passing an empty slice should not panic and should produce no locs.
	// Protective: the parser reads GSmart() immediately, which on an empty
	// buffer must gracefully detect end-of-stream.
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("loadLocs panicked on empty input: %v", r)
		}
	}()
	gm.loadLocs([]byte{}, 0, 0)
	if got := len(gm.StaticLocs()); got != 0 {
		t.Errorf("empty input should produce 0 locs; got %d", got)
	}
}
```

- [ ] **Step 3.2: Run — verify FAIL**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/gamemap/ -run TestLoadLocs -v`
Expected: FAIL — `TestLoadLocsParsesKnownFixture` gets 0 locs (stub) instead of 1.
(Or `TestLoadLocsEmptyFile` may panic; see note below.)

> **Note on the empty-file test**: `Packet.GSmart` at `pkg/io/packet/packet.go:267` dereferences `p.Data[p.Pos]` without length-checking — it will panic on an empty buffer. The parser must guard against this. Add a `p.Len() == 0` check at the top of each loop iteration.

- [ ] **Step 3.3: Replace `loadLocs`**

Edit `pkg/gamemap/load.go`. Add import `"github.com/zsrv/goscape/pkg/entity"` if not present. Replace the stub:

```go
// loadLocs parses a mapsquare's l{X}_{Z} file. The loc stream encodes:
//
//   locID = -1
//   loop:
//     delta = gsmart(); if delta == 0: end.
//     locID += delta
//     coord = 0
//     loop:
//       coordDelta = gsmart(); if coordDelta == 0: next locID.
//       coord += coordDelta - 1
//       level  = (coord >> 12) & 0x3
//       localX = (coord >> 6)  & 0x3F
//       localZ =  coord         & 0x3F
//       info   = g1()
//       shape  = info >> 2
//       angle  = info & 0x3
//       instantiate LifecycleRespawn loc at absolute (mapX*64+localX, mapZ*64+localZ)
//
// Footprint is hardcoded to 1×1 until LocType config loading lands.
// TODO(loctype): use LocType.Width/Length for multi-tile locs.
// TODO(bridged-levels): honour LINK_BELOW for bridge tiles.
func (gm *GameMap) loadLocs(data []byte, mapsquareX, mapsquareZ int) {
	p := packet.NewPacket(data)
	locID := -1
	for {
		if p.Len() == 0 {
			return
		}
		delta := int(p.GSmart())
		if delta == 0 {
			return
		}
		locID += delta
		coord := 0
		for {
			if p.Len() == 0 {
				return
			}
			coordDelta := int(p.GSmart())
			if coordDelta == 0 {
				break
			}
			coord += coordDelta - 1
			localZ := coord & 0x3F
			localX := (coord >> 6) & 0x3F
			level := (coord >> 12) & 0x3

			if p.Len() == 0 {
				return
			}
			info := p.G1()
			shape := int(info >> 2)
			angle := int(info & 0x3)

			absX := mapsquareX*mapSquareSize + localX
			absZ := mapsquareZ*mapSquareSize + localZ

			loc := entity.NewLoc(level, absX, absZ, 1, 1,
				entity.LifecycleRespawn,
				locID, shape, angle)
			gm.staticLocs = append(gm.staticLocs, loc)
		}
	}
}
```

- [ ] **Step 3.4: Run — verify PASS**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/gamemap/ -v`
Expected: all PASS.

- [ ] **Step 3.5: Race + vet**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./pkg/gamemap/`
Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./pkg/gamemap/`
Expected: clean.

- [ ] **Step 3.6: Commit**

```bash
git add pkg/gamemap/load.go pkg/gamemap/gamemap_test.go
git commit --no-gpg-sign -m "feat(gamemap): parse static locs from l{X}_{Z} files

Replace the loadLocs stub with a real parser. Each entry produces a
LifecycleRespawn loc with 1x1 footprint at absolute world coords,
accumulated in gm.staticLocs. Empty-file input is tolerated (guard
against GSmart panic on zero-length buffer).

Multi-tile footprints and bridged-level handling are TODOs pending
LocType config loading."
```

---

## Task 4: Server startup wiring + integration test

**Files:**
- Modify: `modules/world/server.go`
- Create: `modules/world/server_zone_static_test.go`

- [ ] **Step 4.1: Read `server.go` first**

Locate:
- The Server initialisation path where `gm.Init(...)` is called (around line 114-120 in the current code).
- The existing NPC-spawn pass (around line 145) that iterates `s.gamemap.NpcSpawns()`.

The static-loc population pass goes adjacent to the NPC pass.

- [ ] **Step 4.2: Write failing test**

Create `modules/world/server_zone_static_test.go`:

```go
package world

import (
	"testing"

	"github.com/zsrv/goscape/pkg/entity"
	"github.com/zsrv/goscape/pkg/gamemap"
	"github.com/zsrv/goscape/pkg/zone"
)

func TestServerStaticLocsPopulateZones(t *testing.T) {
	s := newTestServer(t)
	s.zoneMap = zone.NewZoneMap()
	s.zonesTracking = map[*zone.Zone]struct{}{}

	// Build a gamemap with one pre-seeded static loc, bypassing file I/O.
	s.gamemap = gamemap.New(discardLogger())
	loc := entity.NewLoc(0, 3094, 3106, 1, 1, entity.LifecycleRespawn, 100, 0, 0)
	// Seed via test hook — see note below on the helper.
	gamemap.InjectStaticLocForTest(s.gamemap, loc)

	s.populateStaticLocsIntoZones() // the method added in Task 4.3

	z := s.zoneMap.Get(0, 3094, 3106)
	if len(z.Locs) != 1 || z.Locs[0] != loc {
		t.Errorf("zone should contain the seeded static loc; Locs=%v", z.Locs)
	}
}
```

For the `InjectStaticLocForTest` helper: add a tiny test-only export in `pkg/gamemap/gamemap.go` — either:
- A new `_test.go` file in `pkg/gamemap` that exposes `InjectStaticLocForTest(gm *GameMap, loc *entity.Loc) { gm.staticLocs = append(gm.staticLocs, loc) }` — but this is in `pkg/gamemap`'s test, not reachable from `modules/world`.
- OR a separate, production-file helper `InjectStaticLoc` exported for testing (mildly ugly but keeps the test simple).

Pragmatic choice: add an exported `AddStaticLoc(loc *entity.Loc)` method on `GameMap` that accepts any pre-built loc. It's analogous to `NpcSpawns()` being read-only — but symmetric with actually seeding for testing. Public API surface grows by one method, but the method is genuinely useful and documented.

```go
// AddStaticLoc accepts a pre-built loc and appends it to the static-loc
// list. Intended for tests and future runtime-loc-spawn code.
func (gm *GameMap) AddStaticLoc(loc *entity.Loc) {
	gm.staticLocs = append(gm.staticLocs, loc)
}
```

Update the test to call `s.gamemap.AddStaticLoc(loc)` instead of `InjectStaticLocForTest`.

Also the `populateStaticLocsIntoZones` method is a private server method exported for the test's benefit. Actually, call it directly since it's within the `world` package:

```go
// Test uses private method:
s.populateStaticLocsIntoZones()
```

- [ ] **Step 4.3: Implement the startup pass**

In `modules/world/server.go`, add a private method:

```go
// populateStaticLocsIntoZones pushes each parsed static loc from the
// gamemap into its owning Zone. Called once at server startup, adjacent
// to the NPC-spawn pass.
func (s *Server) populateStaticLocsIntoZones() {
	for _, loc := range s.gamemap.StaticLocs() {
		z := s.zoneMap.Get(loc.Level, loc.X, loc.Z)
		z.AddStaticLoc(loc)
	}
}
```

Call it from the existing Server startup sequence, after `gm.Init(...)` succeeds and near the NPC-spawn pass. The exact placement depends on current code:

```go
// Existing:
// for _, spawn := range s.gamemap.NpcSpawns() { ... }

// New, either before or after the NPC pass:
s.populateStaticLocsIntoZones()
```

Also add `AddStaticLoc` on `GameMap` as described above for test seeding.

- [ ] **Step 4.4: Run the integration test**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestServerStaticLocsPopulateZones -v`
Expected: PASS.

- [ ] **Step 4.5: Full suite + vet + race**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`
Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...`
Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./...`
Expected: all clean.

- [ ] **Step 4.6: Commit**

```bash
git add modules/world/server.go modules/world/server_zone_static_test.go pkg/gamemap/gamemap.go
git commit --no-gpg-sign -m "feat(world): populate zones with static locs at startup

After gamemap.Init, iterate StaticLocs() and push each into its owning
zone via Zone.AddStaticLoc. Adjacent to the existing NPC-spawn pass.

Adds GameMap.AddStaticLoc as a public seeding method (used by the
integration test and available for future runtime-loc-spawn code)."
```

---

## Final Verification

- [ ] **Step F.1: Race detector**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./...`
Expected: PASS.

- [ ] **Step F.2: go vet**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...`
Expected: no output.

- [ ] **Step F.3: Verify loadLocs stub is gone**

Run: `grep -n "sub-spec 2 doesn't have LocType" pkg/gamemap/load.go`
Expected: no match (the stub comment is gone).

---

## Spec Coverage Map

| Spec requirement | Task |
|---|---|
| `Zone.AddStaticLoc` (no event queue) | Task 1 |
| `GameMap.mData/lData` retention | Task 2 |
| `GameMap.StaticLocs/LandBytes/LocBytes` accessors | Task 2 |
| `loadLocs` parser replacing stub | Task 3 |
| 1×1 footprint + `LifecycleRespawn` + absolute coords | Task 3 |
| Server startup population pass | Task 4 |
| `GameMap.AddStaticLoc` test-seeding helper | Task 4 |
| Integration test | Task 4 |
| All acceptance criteria | Task F |

No gaps.
