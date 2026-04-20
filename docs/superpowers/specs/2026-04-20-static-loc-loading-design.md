# Sub-spec 5a: Static Loc Loading — Design

**Status:** Draft → ready for plan
**Scope:** Parse static loc files at world init and populate `Zone.Locs` with `LifecycleRespawn` entries. Retain raw mapsquare bytes so sub-spec 5b can serve them to the client via opcode 150 responses.
**Out of scope:** Opcode 150 handler + server response encoders (5b); `writeFullFollows` Respawn-branch replay (5c, deferred until scripts exist); `LocType` config loading; collision integration; bridged-level handling.

---

## Goal

Close the `loadLocs` stub at `pkg/gamemap/load.go:70`. Parse each `l{X}_{Z}` file into `*entity.Loc` instances with `LifecycleRespawn`, accumulate them in `GameMap`, and have the Server push them into their zones during startup.

After this sub-spec the server holds correct static-loc state for every mapsquare on disk. No user-visible change until 5b ships the wire responses.

## Architecture

Follows the established `loadNPCs` pattern: `GameMap` parses + accumulates in its own fields; `Server` reads the accessor post-Init and wires entities into the world. No new package.

```
pkg/gamemap/gamemap.go     + mData, lData retention maps, + StaticLocs() / LandBytes() / LocBytes() accessors
pkg/gamemap/load.go        replace loadLocs stub with real parser; stash raw bytes in mData/lData
pkg/zone/zone.go           + AddStaticLoc method (no event queue)
modules/world/server.go    + startup pass that iterates StaticLocs and populates zoneMap
```

## Components

### 1. `Zone.AddStaticLoc` — `pkg/zone/zone.go`

```go
// AddStaticLoc appends a static (LifecycleRespawn) loc to z.Locs WITHOUT
// queuing a zone event. Statics are delivered to clients via the mapsquare
// download (sub-spec 5b), not via zone events. Called once per loc during
// world init.
func (z *Zone) AddStaticLoc(loc *entity.Loc) {
	z.Locs = append(z.Locs, loc)
}
```

~3 LOC. Distinct from `AddLoc` (which queues a `LocAddChange` enclosed event); statics bypass the event pipeline entirely.

### 2. `GameMap` additions — `pkg/gamemap/gamemap.go`

Add three fields to the `GameMap` struct:

```go
mData      map[uint16][]byte // raw m{X}_{Z} bytes, keyed by (mapX<<8)|mapZ
lData      map[uint16][]byte // raw l{X}_{Z} bytes
staticLocs []*entity.Loc      // parsed static locs with absolute world coords
```

Initialise both maps in `New` (or `Init` if that's where other maps are initialised). Three accessors:

```go
// StaticLocs returns the list of parsed static locs. Caller owns iteration
// order; pointers are stable for the lifetime of GameMap.
func (gm *GameMap) StaticLocs() []*entity.Loc { return gm.staticLocs }

// LandBytes returns the raw (bzip2-compressed) bytes of the m{X}_{Z} file
// for the given mapsquare, or nil if unloaded.
func (gm *GameMap) LandBytes(mapX, mapZ int) []byte {
	return gm.mData[uint16((mapX<<8)|mapZ)]
}

// LocBytes returns the raw (bzip2-compressed) bytes of the l{X}_{Z} file
// for the given mapsquare, or nil if unloaded.
func (gm *GameMap) LocBytes(mapX, mapZ int) []byte {
	return gm.lData[uint16((mapX<<8)|mapZ)]
}
```

### 3. Byte retention in Init — `pkg/gamemap/gamemap.go`

During the existing Init loop (where `m{X}_{Z}` and `l{X}_{Z}` files are read), stash the raw bytes:

```go
mBytes, _ := os.ReadFile(mPath)
gm.mData[uint16((mapX<<8)|mapZ)] = mBytes
// ... existing loadGround(mBytes, mapX, mapZ) ...

lBytes, _ := os.ReadFile(lPath)
gm.lData[uint16((mapX<<8)|mapZ)] = lBytes
// ... gm.loadLocs(lBytes, mapX, mapZ) ...
```

Existing error handling unchanged. If a file is missing, the entry stays absent from the map.

### 4. Loc parser — `pkg/gamemap/load.go`

Replace the current stub:

```go
// loadLocs parses a mapsquare's l{X}_{Z} file. The format:
//
//   locID = -1
//   loop:
//     delta = gsmart(); if delta == 0: end of file.
//     locID += delta
//     coord = 0
//     loop:
//       coordDelta = gsmart(); if coordDelta == 0: next locID.
//       coord += coordDelta - 1
//       level  = (coord >> 12) & 0x3
//       localX = (coord >> 6) & 0x3F
//       localZ =  coord        & 0x3F
//       info   = g1()
//       shape  = info >> 2
//       angle  = info & 0x3
//       // instantiate a LifecycleRespawn loc at absolute coords
//
// Footprint is hardcoded to 1×1 until LocType config loading lands.
// Multi-tile locs (trees, large buildings) will render correctly client-side
// because the client has its own LocType cache; server-side positional
// queries (pathing, aggro) will be wrong for those locs until LocType
// arrives. TODO(loctype): use LocType.Width/Length.
// TODO(bridged-levels): honour LINK_BELOW for bridge tiles (see TS reference).
func (gm *GameMap) loadLocs(data []byte, mapsquareX, mapsquareZ int) {
	p := packet.NewPacket(data)
	locID := -1
	for {
		delta := int(p.GSmart())
		if delta == 0 {
			return
		}
		locID += delta
		coord := 0
		for {
			coordDelta := int(p.GSmart())
			if coordDelta == 0 {
				break
			}
			coord += coordDelta - 1
			localZ := coord & 0x3F
			localX := (coord >> 6) & 0x3F
			level := (coord >> 12) & 0x3

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

Requires import of `github.com/zsrv/goscape/pkg/entity` at the top of `load.go`.

Note: `Packet.GSmart` already exists in `pkg/io/packet/packet.go:266` and returns `uint16`. The parser uses `int(p.GSmart())` to widen for arithmetic.

### 5. Server startup wiring — `modules/world/server.go`

After the existing `gm.Init(cacheDir)` call (around line 114-120 depending on current state), and alongside the existing NPC-spawn pass (line 145), add:

```go
for _, loc := range s.gamemap.StaticLocs() {
	z := s.zoneMap.Get(loc.Level, loc.X, loc.Z)
	z.AddStaticLoc(loc)
}
```

Placement: after Init succeeds but before the tick loop starts. Adjacent to the existing NPC-spawn pass is the natural location.

## Data Flow

```
server startup
    │
    │ gamemap.Init(cacheDir)
    │   │
    │   ├─ glob m*_*, l*_*, n*_*, o*_*
    │   ├─ for each mapsquare:
    │   │    mBytes = os.ReadFile(m{X}_{Z})
    │   │    gm.mData[(X<<8)|Z] = mBytes        ← NEW
    │   │    loadGround(mBytes, X, Z)
    │   │
    │   │    lBytes = os.ReadFile(l{X}_{Z})
    │   │    gm.lData[(X<<8)|Z] = lBytes        ← NEW
    │   │    loadLocs(lBytes, X, Z)              ← REPLACES STUB
    │   │      └─ appends N entity.Locs with LifecycleRespawn to gm.staticLocs
    │   │
    │   └─ loadNPCs / loadObjs (existing)
    │
    │ (Server post-Init)
    │   ├─ for loc in gm.StaticLocs():             ← NEW
    │   │    zoneMap.Get(loc.Level, loc.X, loc.Z).AddStaticLoc(loc)
    │   │
    │   └─ for spawn in gm.NpcSpawns(): (existing)
    │
    └─ tick loop starts
       zone.Locs now contains both static and dynamic locs.
       Zone.AddLoc/RemoveLoc/AnimLoc/ChangeLoc work transparently.
       writeFullFollows currently emits nothing for Respawn-lifecycle entries
       (correct behaviour — clients build statics from the mapsquare download
       which 5b will deliver).
```

## Error Handling

- `loadLocs` terminates cleanly on any `GSmart() == 0` (outer or inner loop). A truncated file produces fewer locs than the file theoretically contains; no panic.
- `os.ReadFile` errors stay where they already live — unchanged.
- `Zone.AddStaticLoc` is a pointer append, no failure modes.
- `entity.NewLoc` never returns an error.
- Missing l-file for an existing m-file: lData[key] stays absent, staticLocs receives no contribution, zone stays empty of that mapsquare's locs. Acceptable.

## Testing

### `pkg/zone/zone_test.go` (extend)

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

func TestResetPreservesStaticLocs(t *testing.T) {
	z := New(0, 0, 0, 0)
	loc := entity.NewLoc(0, 0, 0, 1, 1, entity.LifecycleRespawn, 1, 0, 0)
	z.AddStaticLoc(loc)
	z.events = []ZoneEvent{{Type: ZoneEventEnclosed, Bytes: []byte{1}}}
	z.ComputeShared()
	z.Reset()
	if len(z.Locs) != 1 {
		t.Errorf("Locs should survive Reset; got %d", len(z.Locs))
	}
}
```

### `pkg/gamemap/gamemap_test.go` (extend)

Build a minimal `l{X}_{Z}` fixture by hand-encoding the gsmart stream:

```go
func TestLoadLocsParsesKnownFixture(t *testing.T) {
	// One locID=100 at level=0, localX=3, localZ=7, shape=5, angle=2.
	// Coord packed = (0<<12) | (3<<6) | 7 = 199 → coordDelta = 200 (coord+1).
	// gsmart(101) for locID delta: 101 < 128 → 1-byte 0x65.
	// gsmart(200) for coord delta: 200 >= 128 → 2-byte 0x80C8.
	// info = (5<<2)|2 = 0x16.
	// Terminator: gsmart(0) = 0x00 for inner loop, gsmart(0) = 0x00 for outer.
	fixture := []byte{
		0x65,             // locID delta = 101 → locID = 100
		0x80, 0xC8,       // coord delta = 200 → coord = 199
		0x16,             // info = shape 5, angle 2
		0x00,             // inner-loop terminator
		0x00,             // outer-loop terminator
	}

	gm := New(discardLogger())
	gm.loadLocs(fixture, 50, 51)

	statics := gm.StaticLocs()
	if len(statics) != 1 {
		t.Fatalf("StaticLocs: got %d, want 1", len(statics))
	}
	loc := statics[0]
	if loc.Level != 0 || loc.X != 50*64+3 || loc.Z != 51*64+7 {
		t.Errorf("position: got L%d (%d,%d), want 0 (3203, 3271)", loc.Level, loc.X, loc.Z)
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

func TestLoadLocsMultipleLocsPerTile(t *testing.T) {
	// Two locs at the same tile: locID 10 then locID 20, both at coord 0.
	// gsmart(11) = 0x0B  (locID delta)
	// gsmart(1)  = 0x01  (coord delta so coord += 0)
	// info       = 0x00  (shape 0, angle 0)
	// gsmart(0)  = 0x00  (next coord → end inner)
	// gsmart(11) = 0x0B  (locID delta → 21)
	// Actually locs 10 and 21, close enough. Let's just use delta=10 then delta=10.
	fixture := []byte{
		0x0B,       // locID delta 11 → locID 10
		0x01,       // coord delta 1 → coord 0
		0x00,       // info
		0x00,       // end inner
		0x0B,       // locID delta 11 → locID 21
		0x01,       // coord delta 1 → coord 0
		0x00,       // info
		0x00,       // end inner
		0x00,       // end outer
	}
	gm := New(discardLogger())
	gm.loadLocs(fixture, 0, 0)
	if len(gm.StaticLocs()) != 2 {
		t.Errorf("StaticLocs: got %d, want 2", len(gm.StaticLocs()))
	}
}

func TestLoadLocsEmptyFile(t *testing.T) {
	gm := New(discardLogger())
	gm.loadLocs([]byte{}, 0, 0)
	if len(gm.StaticLocs()) != 0 {
		t.Errorf("empty file should produce 0 locs; got %d", len(gm.StaticLocs()))
	}
}

func TestGameMapRetainsRawBytes(t *testing.T) {
	// Set up cacheDir with one m file and one l file.
	dir := t.TempDir()
	mapsDir := filepath.Join(dir, "client", "maps")
	os.MkdirAll(mapsDir, 0755)
	os.WriteFile(filepath.Join(mapsDir, "m50_51"), []byte{0xDE, 0xAD}, 0644)
	os.WriteFile(filepath.Join(mapsDir, "l50_51"), []byte{0xBE, 0xEF}, 0644)

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
		t.Error("LandBytes(0,0) unloaded should return nil")
	}
}
```

### `modules/world/` integration test

```go
// In a new file or appended to server_test.go:
func TestServerStaticLocsPopulateZones(t *testing.T) {
	s := newTestServer(t)
	s.zoneMap = zone.NewZoneMap()
	s.zonesTracking = map[*zone.Zone]struct{}{}

	// Hand-seed the gamemap with a static loc (bypassing file I/O).
	s.gamemap = gamemap.New(discardLogger())
	loc := entity.NewLoc(0, 3094, 3106, 1, 1, entity.LifecycleRespawn, 100, 0, 0)
	s.gamemap.InjectStaticLocForTest(loc) // new test-only method or direct-field access

	// The startup pass. In production this runs in Server init; here we
	// exercise the code path directly.
	for _, l := range s.gamemap.StaticLocs() {
		z := s.zoneMap.Get(l.Level, l.X, l.Z)
		z.AddStaticLoc(l)
	}

	z := s.zoneMap.Get(0, 3094, 3106)
	if len(z.Locs) != 1 || z.Locs[0] != loc {
		t.Errorf("zone should contain the static loc; got %v", z.Locs)
	}
}
```

(`InjectStaticLocForTest` can be a tiny test-only accessor, or the test can write directly to `gm.staticLocs` through an exported-for-test field — either works; the plan will pick one.)

## Acceptance Criteria

1. `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...` passes.
2. `go vet ./...` clean.
3. `Zone.AddStaticLoc` exists and is distinct from `Zone.AddLoc`.
4. `GameMap.StaticLocs()`, `LandBytes(x,z)`, `LocBytes(x,z)` all return non-nil sensibly.
5. No regressions in existing gamemap/zone tests.

## LOC Estimate

| File | LOC |
|---|---|
| `pkg/zone/zone.go` | +10 |
| `pkg/zone/zone_test.go` | +45 |
| `pkg/gamemap/gamemap.go` | +25 |
| `pkg/gamemap/load.go` | +45 |
| `pkg/gamemap/gamemap_test.go` | +130 |
| `modules/world/server.go` | +5 |
| `modules/world/<integration>_test.go` | +60 |
| **Total** | **~320** |

## Dependencies & Risks

- **`pkg/entity` (4b-1)** — `NewLoc`, `LifecycleRespawn`.
- **`pkg/zone` (4b-3)** — Zone struct; extend with `AddStaticLoc`.
- **`pkg/io/packet`** — `GSmart` already exists.
- **No risk to existing wire format** — pure server-side state.
- **Risk: 1×1 footprint assumption** — documented; corrected when LocType lands.
- **Risk: bridged-level handling missing** — bridges are rare and primarily affect level classification; documented as TODO.
- **Risk: empty file tolerance** — parser handles it but the underlying `Packet.GSmart` on a zero-byte buffer may read past the slice. Test covers this case explicitly.

## Deferred to Later Sub-specs

- **5b**: Opcode 150 handler + `DATA_LAND`/`DATA_LOC`/`_DONE` serving.
- **5c (or later)**: `writeFullFollows` Respawn-branch replay — not needed until scripts can remove/change statics.
- **Beyond 5**: `LocType` config load, loc collision wiring, bridged levels, static obj loading, `StaticObjs()` mirror.
