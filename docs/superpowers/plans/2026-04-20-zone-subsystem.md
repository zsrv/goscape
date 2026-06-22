# Sub-spec 4b-3: Zone Subsystem — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: superpowers:subagent-driven-development (recommended) or superpowers:executing-plans.

**Goal:** New `pkg/zone/` package with `Zone`, `ZoneEvent`, `ZoneMap`, `ZoneGrid` and 11 event-queueing methods. Pure data + composition; no Player/World integration.

**Architecture:** 4 production files + 4 test files. Imports `pkg/entity` (4b-1) and `pkg/rsbuf` (4b-2). Zero changes outside `pkg/zone/`.

**Tech Stack:** Go 1.26. `pkg/io/packet`, `pkg/coordgrid`, `pkg/entity`, `pkg/rsbuf`.

**Build prefix:** `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go ...`
**Commit flag:** `--no-gpg-sign`

**Spec reference:** `docs/superpowers/specs/2026-04-20-zone-subsystem-design.md`

---

## File Structure

**Create:**
- `pkg/zone/event.go` — ZoneEvent + ZoneEventType
- `pkg/zone/map.go` — ZoneMap, ZoneIndex, UnpackIndex
- `pkg/zone/grid.go` — ZoneGrid (vendored from rs-server-225, renamed)
- `pkg/zone/zone.go` — Zone struct + 11 methods
- `pkg/zone/event_test.go`
- `pkg/zone/map_test.go`
- `pkg/zone/grid_test.go`
- `pkg/zone/zone_test.go`

---

## Task 1: Foundation — event + map + grid + their tests

Three small self-contained files that together give the zone infrastructure its bones.

**Files:**
- Create: `pkg/zone/event.go`, `pkg/zone/map.go`, `pkg/zone/grid.go`
- Create: `pkg/zone/event_test.go`, `pkg/zone/map_test.go`, `pkg/zone/grid_test.go`

- [ ] **Step 1.1: Write failing tests**

Create `pkg/zone/event_test.go`:

```go
package zone

import "testing"

func TestZoneEventTypeValues(t *testing.T) {
	if ZoneEventEnclosed != 0 {
		t.Errorf("ZoneEventEnclosed: got %d, want 0", ZoneEventEnclosed)
	}
	if ZoneEventFollows != 1 {
		t.Errorf("ZoneEventFollows: got %d, want 1", ZoneEventFollows)
	}
}

func TestPublicReceiverSentinel(t *testing.T) {
	if PublicReceiver != -1 {
		t.Errorf("PublicReceiver: got %d, want -1", PublicReceiver)
	}
}
```

Create `pkg/zone/map_test.go`:

```go
package zone

import "testing"

func TestZoneIndexRoundTrip(t *testing.T) {
	// Tile coord (3094, 3106, 0) → zone (386, 388, 0) → index.
	// UnpackIndex returns tile-unit coords at the zone's SW corner: (386<<3, 388<<3, 0) = (3088, 3104, 0).
	idx := ZoneIndex(3094, 3106, 0)
	x, z, level := UnpackIndex(idx)
	if x != 3088 || z != 3104 || level != 0 {
		t.Errorf("roundtrip: got (%d,%d,%d), want (3088,3104,0)", x, z, level)
	}
}

func TestZoneIndexLevelMatters(t *testing.T) {
	if ZoneIndex(0, 0, 0) == ZoneIndex(0, 0, 1) {
		t.Error("zones at different levels must have different indexes")
	}
}

func TestZoneMapGetCreatesOnce(t *testing.T) {
	m := NewZoneMap()
	z1 := m.Get(0, 3094, 3106)
	z2 := m.Get(0, 3094, 3106)
	if z1 != z2 {
		t.Error("two Gets at the same coord should return the same Zone pointer")
	}
	if z1 == nil {
		t.Fatal("Get should never return nil")
	}
	if z1.X != 386 || z1.Z != 388 || z1.Level != 0 {
		t.Errorf("zone coords: got (%d,%d,%d), want (386,388,0)", z1.X, z1.Z, z1.Level)
	}
}

func TestZoneMapGetByIndex(t *testing.T) {
	m := NewZoneMap()
	idx := ZoneIndex(3094, 3106, 0)
	z := m.GetByIndex(idx)
	if z.Index != idx {
		t.Errorf("zone.Index: got %d, want %d", z.Index, idx)
	}
}

func TestZoneMapGridPerLevel(t *testing.T) {
	m := NewZoneMap()
	if m.Grid(0) == m.Grid(1) {
		t.Error("Grid(0) and Grid(1) should be distinct instances")
	}
	if m.Grid(0) != m.Grid(0) {
		t.Error("Grid(0) called twice should return the same instance")
	}
}

func TestZoneMapZoneCount(t *testing.T) {
	m := NewZoneMap()
	m.Get(0, 0, 0)
	m.Get(0, 100, 100)
	m.Get(0, 0, 0) // same as first
	if m.ZoneCount() != 2 {
		t.Errorf("ZoneCount: got %d, want 2", m.ZoneCount())
	}
}
```

Create `pkg/zone/grid_test.go`:

```go
package zone

import "testing"

func TestZoneGridFlagUnflag(t *testing.T) {
	g := NewZoneGrid()
	if g.IsFlagged(100, 200, 0) {
		t.Error("brand-new grid should not be flagged anywhere")
	}
	g.Flag(100, 200)
	if !g.IsFlagged(100, 200, 0) {
		t.Error("after Flag(100,200), IsFlagged(100,200,0) should be true")
	}
	g.Unflag(100, 200)
	if g.IsFlagged(100, 200, 0) {
		t.Error("after Unflag(100,200), IsFlagged(100,200,0) should be false")
	}
}

func TestZoneGridRadiusSearch(t *testing.T) {
	g := NewZoneGrid()
	g.Flag(100, 200)
	if !g.IsFlagged(105, 205, 6) {
		t.Error("(105,205) within radius 6 of flagged (100,200) should match")
	}
	if g.IsFlagged(120, 220, 6) {
		t.Error("(120,220) outside radius 6 of flagged (100,200) should not match")
	}
}
```

- [ ] **Step 1.2: Run the tests — verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/zone/ -v`
Expected: FAIL — package does not exist.

- [ ] **Step 1.3: Implement `event.go`**

Create `pkg/zone/event.go`:

```go
package zone

// PublicReceiver is the sentinel ReceiverID for zone events visible to
// every observer. Mirrors the TS NO_RECEIVER bigint -1n.
const PublicReceiver = -1

// ZoneEventType distinguishes events that are broadcast to every observer
// of the zone from those that are routed to a specific recipient player.
type ZoneEventType int

const (
	// ZoneEventEnclosed events are shared across every observer; they are
	// concatenated into Zone.shared by ComputeShared and then delivered
	// inside UpdateZonePartialEnclosed packets.
	ZoneEventEnclosed ZoneEventType = iota

	// ZoneEventFollows events are per-receiver; delivery code filters by
	// ReceiverID and writes each inside an UpdateZonePartialFollows wrapper.
	ZoneEventFollows
)

// ZoneEvent carries one already-encoded zone-nested message. Bytes is
// exactly [opcode_byte, ...payload] and is ready to concat into the shared
// buffer (Enclosed) or write per-player (Follows).
//
// A nil Bytes is a tombstone — produced by clearQueuedEvents when an entity
// is removed after queuing events. ComputeShared skips tombstoned entries.
type ZoneEvent struct {
	Type       ZoneEventType
	ReceiverID int // PublicReceiver = -1 for Enclosed events and public Follows
	Bytes      []byte
}
```

- [ ] **Step 1.4: Implement `grid.go` (vendored)**

Create `pkg/zone/grid.go`:

```go
package zone

// Ported from $HOME/Code/github.com/zsrv/rs-server-225/engine/zone/grid.go,
// renamed Grid → ZoneGrid for clarity in the package-qualified zone.ZoneGrid form.

const (
	// ZoneGridSize is the side length of the world in zones (2048 × 8 = 16384 tiles).
	ZoneGridSize = 2048

	zoneGridIntBits     = 5
	zoneGridIntBitsFlag = (1 << zoneGridIntBits) - 1

	// ZoneGridDefaultSize is the int32-slice length needed for a full-world grid.
	ZoneGridDefaultSize = ZoneGridSize * (ZoneGridSize >> zoneGridIntBits)
)

// ZoneGrid is a per-level 2D bitmap of zone-occupancy flags. One bit per
// (zoneX, zoneZ) pair; set when the zone contains at least one player.
type ZoneGrid struct {
	grid []int32
}

// NewZoneGrid returns a zero-initialised ZoneGrid sized for the full world.
func NewZoneGrid() *ZoneGrid {
	return &ZoneGrid{grid: make([]int32, ZoneGridDefaultSize)}
}

func zoneGridIndex(zoneX, zoneZ int) int {
	return (zoneX << zoneGridIntBits) | (zoneZ >> zoneGridIntBits)
}

// Flag marks (zoneX, zoneZ) as occupied.
func (g *ZoneGrid) Flag(zoneX, zoneZ int) {
	g.grid[zoneGridIndex(zoneX, zoneZ)] |= 1 << (zoneZ & zoneGridIntBitsFlag)
}

// Unflag clears the occupied bit at (zoneX, zoneZ).
func (g *ZoneGrid) Unflag(zoneX, zoneZ int) {
	g.grid[zoneGridIndex(zoneX, zoneZ)] &= ^(1 << (zoneZ & zoneGridIntBitsFlag))
}

// IsFlagged reports whether ANY zone within `radius` of (zoneX, zoneZ) is flagged.
func (g *ZoneGrid) IsFlagged(zoneX, zoneZ, radius int) bool {
	minX := max(0, zoneX-radius)
	maxX := min(ZoneGridSize-1, zoneX+radius)
	minY := max(0, zoneZ-radius)
	maxY := min(ZoneGridSize-1, zoneZ+radius)
	bits := zoneGridIntBitsFlag
	startY := minY & ^bits
	endY := maxY >> zoneGridIntBits << zoneGridIntBits

	for x := minX; x <= maxX; x++ {
		for y := startY; y <= endY; y += 32 {
			index := zoneGridIndex(x, y)
			line := g.grid[index]
			trailingTrimmed := line
			if y+bits > maxY {
				trailingTrimmed = line & ((1 << (maxY - y + 1)) - 1)
			}
			leadingTrimmed := trailingTrimmed
			if y < minY {
				leadingTrimmed = trailingTrimmed >> (minY - y)
			}
			if leadingTrimmed != 0 {
				return true
			}
		}
	}
	return false
}
```

- [ ] **Step 1.5: Implement `map.go`**

Create `pkg/zone/map.go`:

```go
package zone

// ZoneIndex packs (worldX, worldZ, level) into a single int using the same
// bit layout as the TS reference:
//
//	zone_x = worldX >> 3, zone_z = worldZ >> 3
//	index  = (zone_x & 0x7FF) | ((zone_z & 0x7FF) << 11) | ((level & 0x3) << 22)
func ZoneIndex(worldX, worldZ, level int) int {
	return ((worldX >> 3) & 0x7FF) | (((worldZ >> 3) & 0x7FF) << 11) | ((level & 0x3) << 22)
}

// UnpackIndex reverses ZoneIndex. Returns TILE-unit coordinates at the
// zone's SW corner (zoneX << 3, zoneZ << 3).
func UnpackIndex(index int) (worldX, worldZ, level int) {
	worldX = (index & 0x7FF) << 3
	worldZ = ((index >> 11) & 0x7FF) << 3
	level = (index >> 22) & 0x3
	return
}

// ZoneMap is the world's collection of Zones, indexed by packed (x,z,level).
// Zones are created on first access; empty zones carry zero state and cost.
type ZoneMap struct {
	zones map[int]*Zone
	grids map[int]*ZoneGrid
}

// NewZoneMap returns an empty map.
func NewZoneMap() *ZoneMap {
	return &ZoneMap{
		zones: make(map[int]*Zone),
		grids: make(map[int]*ZoneGrid),
	}
}

// Get returns the Zone at (level, worldX, worldZ), creating it if absent.
func (m *ZoneMap) Get(level, worldX, worldZ int) *Zone {
	return m.GetByIndex(ZoneIndex(worldX, worldZ, level))
}

// GetByIndex returns the Zone with the given packed index, creating it if absent.
func (m *ZoneMap) GetByIndex(index int) *Zone {
	if z, ok := m.zones[index]; ok {
		return z
	}
	x, z, level := UnpackIndex(index)
	zone := New(index, level, x>>3, z>>3)
	m.zones[index] = zone
	return zone
}

// Grid returns the per-level ZoneGrid, creating it if absent.
func (m *ZoneMap) Grid(level int) *ZoneGrid {
	if g, ok := m.grids[level]; ok {
		return g
	}
	g := NewZoneGrid()
	m.grids[level] = g
	return g
}

// ZoneCount returns the number of materialised zones.
func (m *ZoneMap) ZoneCount() int { return len(m.zones) }

// LocCount sums len(Locs) across all materialised zones.
func (m *ZoneMap) LocCount() int {
	total := 0
	for _, z := range m.zones {
		total += len(z.Locs)
	}
	return total
}

// ObjCount sums len(Objs) across all materialised zones.
func (m *ZoneMap) ObjCount() int {
	total := 0
	for _, z := range m.zones {
		total += len(z.Objs)
	}
	return total
}
```

- [ ] **Step 1.6: Stub `zone.go` (enough to satisfy map.go refs)**

Create `pkg/zone/zone.go` with just the type and constructor — Task 2 will fill it in:

```go
package zone

import "github.com/zsrv/goscape/pkg/entity"

// Zone is an 8×8 tile region of the world. See the file-level doc in
// future tasks for full semantics.
type Zone struct {
	Index       int
	X, Z, Level int

	Locs []*entity.Loc
	Objs []*entity.Obj

	events       []ZoneEvent
	entityEvents map[*entity.NonPathing][]int

	shared []byte
}

// New constructs a zone for the given packed index and (level, zoneX, zoneZ).
func New(index, level, x, z int) *Zone {
	return &Zone{
		Index:        index,
		X:            x,
		Z:            z,
		Level:        level,
		entityEvents: make(map[*entity.NonPathing][]int),
	}
}
```

- [ ] **Step 1.7: Run tests — verify PASS**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/zone/ -v`
Expected: all PASS (9 tests across 3 files).

- [ ] **Step 1.8: `go vet` + full suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...`
Expected: clean.

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`
Expected: PASS across all packages.

- [ ] **Step 1.9: Commit**

```bash
git add pkg/zone/
git commit --no-gpg-sign -m "feat(zone): add ZoneEvent, ZoneMap, ZoneGrid scaffolding

Three foundation files plus a stub Zone struct. ZoneIndex/UnpackIndex
match the TS 11-bit-x, 11-bit-z, 2-bit-level packing. ZoneGrid vendored
from rs-server-225 with rename Grid → ZoneGrid. ZoneMap lazily creates
Zones on first access.

Zone methods (event queuing, ComputeShared, Reset) land in sub-spec 4b-3
Tasks 2 and 3."
```

---

## Task 2: Zone struct + Reset + ComputeShared + helpers + their tests

Fill out `zone.go` with the shared-buffer composition machinery. No mutation methods yet — those come in Task 3.

**Files:**
- Modify: `pkg/zone/zone.go`
- Create: `pkg/zone/zone_test.go`

- [ ] **Step 2.1: Write failing tests for the base methods**

Create `pkg/zone/zone_test.go` with only the base-method tests:

```go
package zone

import (
	"bytes"
	"testing"
)

func TestNewZoneFields(t *testing.T) {
	z := New(42, 1, 100, 200)
	if z.Index != 42 {
		t.Errorf("Index: got %d, want 42", z.Index)
	}
	if z.Level != 1 || z.X != 100 || z.Z != 200 {
		t.Errorf("coords: got (L=%d, X=%d, Z=%d), want (1,100,200)", z.Level, z.X, z.Z)
	}
	if z.entityEvents == nil {
		t.Error("entityEvents map should be initialised")
	}
	if z.Shared() != nil {
		t.Error("fresh zone should have nil Shared()")
	}
	if len(z.Events()) != 0 {
		t.Error("fresh zone should have no events")
	}
}

func TestComputeSharedEmptyIsNil(t *testing.T) {
	z := New(0, 0, 0, 0)
	z.ComputeShared()
	if z.Shared() != nil {
		t.Errorf("Shared after empty ComputeShared: got %v, want nil", z.Shared())
	}
}

func TestComputeSharedConcatsEnclosed(t *testing.T) {
	z := New(0, 0, 0, 0)
	z.events = []ZoneEvent{
		{Type: ZoneEventEnclosed, ReceiverID: PublicReceiver, Bytes: []byte{0x01, 0x02}},
		{Type: ZoneEventEnclosed, ReceiverID: PublicReceiver, Bytes: []byte{0x03, 0x04, 0x05}},
	}
	z.ComputeShared()
	want := []byte{0x01, 0x02, 0x03, 0x04, 0x05}
	if !bytes.Equal(z.Shared(), want) {
		t.Errorf("Shared: got %v, want %v", z.Shared(), want)
	}
}

func TestComputeSharedSkipsFollows(t *testing.T) {
	z := New(0, 0, 0, 0)
	z.events = []ZoneEvent{
		{Type: ZoneEventEnclosed, Bytes: []byte{0xEE}},
		{Type: ZoneEventFollows, ReceiverID: 5, Bytes: []byte{0xFF}},
	}
	z.ComputeShared()
	if !bytes.Equal(z.Shared(), []byte{0xEE}) {
		t.Errorf("Shared: got %v, want [0xEE]", z.Shared())
	}
}

func TestComputeSharedSkipsTombstones(t *testing.T) {
	z := New(0, 0, 0, 0)
	z.events = []ZoneEvent{
		{Type: ZoneEventEnclosed, Bytes: []byte{0x11}},
		{Type: ZoneEventEnclosed, Bytes: nil}, // tombstone
		{Type: ZoneEventEnclosed, Bytes: []byte{0x22}},
	}
	z.ComputeShared()
	if !bytes.Equal(z.Shared(), []byte{0x11, 0x22}) {
		t.Errorf("Shared: got %v, want [0x11 0x22]", z.Shared())
	}
}

func TestResetClearsEverything(t *testing.T) {
	z := New(0, 0, 0, 0)
	z.events = []ZoneEvent{{Type: ZoneEventEnclosed, Bytes: []byte{1}}}
	z.ComputeShared()
	z.Reset()
	if z.Shared() != nil {
		t.Error("Shared should be nil after Reset")
	}
	if len(z.Events()) != 0 {
		t.Error("events should be empty after Reset")
	}
	if len(z.entityEvents) != 0 {
		t.Error("entityEvents should be empty after Reset")
	}
}
```

- [ ] **Step 2.2: Run tests — verify FAIL**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/zone/ -run TestComputeShared -v`
Expected: FAIL — ComputeShared/Reset/Events/Shared not yet implemented.

- [ ] **Step 2.3: Fill in `zone.go` with base methods**

Replace the contents of `pkg/zone/zone.go` (produced in Task 1 as a stub) with the full version:

```go
package zone

import "github.com/zsrv/goscape/pkg/entity"

// Zone is an 8×8 tile region of the world. It owns the active dynamic
// entities inside the region, a per-tick event queue, and a composed
// shared buffer of every Enclosed event (built by ComputeShared).
//
// All state except Locs and Objs is per-tick; Reset clears it between ticks.
// Locs and Objs persist until the entities are explicitly removed.
type Zone struct {
	Index       int
	X, Z, Level int // X and Z in zone units (tile >> 3)

	Locs []*entity.Loc
	Objs []*entity.Obj

	events       []ZoneEvent
	entityEvents map[*entity.NonPathing][]int // entity pointer → indexes into events

	shared []byte
}

// New constructs a zone for the given packed index and (level, zoneX, zoneZ).
// Callers typically reach Zones through ZoneMap.Get rather than calling this.
func New(index, level, x, z int) *Zone {
	return &Zone{
		Index:        index,
		X:            x,
		Z:            z,
		Level:        level,
		entityEvents: make(map[*entity.NonPathing][]int),
	}
}

// Reset clears per-tick state: events, entityEvents, shared. Called from
// processCleanup at the end of each tick. Locs and Objs persist.
func (z *Zone) Reset() {
	z.events = z.events[:0]
	clear(z.entityEvents)
	z.shared = nil
}

// Shared returns the composed Enclosed-event buffer for the current tick,
// or nil if ComputeShared hasn't been called or no Enclosed events exist.
func (z *Zone) Shared() []byte { return z.shared }

// Events returns the per-tick event queue (read-only view; callers that
// need per-player Follows iteration read from this slice directly).
func (z *Zone) Events() []ZoneEvent { return z.events }

// ComputeShared concatenates the Bytes of every non-tombstoned Enclosed
// event into z.shared. Must be called once per tick before any per-player
// delivery pass. Safe on a zone with zero Enclosed events — sets shared
// to nil.
func (z *Zone) ComputeShared() {
	// First pass: compute total size to size the buffer exactly once.
	total := 0
	for i := range z.events {
		e := &z.events[i]
		if e.Type != ZoneEventEnclosed || e.Bytes == nil {
			continue
		}
		total += len(e.Bytes)
	}
	if total == 0 {
		z.shared = nil
		return
	}
	buf := make([]byte, 0, total)
	for i := range z.events {
		e := &z.events[i]
		if e.Type != ZoneEventEnclosed || e.Bytes == nil {
			continue
		}
		buf = append(buf, e.Bytes...)
	}
	z.shared = buf
}

// queueEvent appends an event and records its index under np so
// clearQueuedEvents can purge it if the entity is removed this same tick.
func (z *Zone) queueEvent(np *entity.NonPathing, e ZoneEvent) {
	idx := len(z.events)
	z.events = append(z.events, e)
	if np != nil {
		z.entityEvents[np] = append(z.entityEvents[np], idx)
	}
}

// clearQueuedEvents tombstones every event indexed by np. ComputeShared
// skips tombstones. Per-player Follows iteration must also skip nil-Bytes.
func (z *Zone) clearQueuedEvents(np *entity.NonPathing) {
	for _, idx := range z.entityEvents[np] {
		z.events[idx].Bytes = nil
	}
	delete(z.entityEvents, np)
}
```

- [ ] **Step 2.4: Run tests — verify PASS**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/zone/ -v`
Expected: all zone-base tests PASS; Task 1's 9 tests still PASS.

- [ ] **Step 2.5: `go vet` + full suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...` — PASS.
Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...` — clean.

- [ ] **Step 2.6: Commit**

```bash
git add pkg/zone/zone.go pkg/zone/zone_test.go
git commit --no-gpg-sign -m "feat(zone): implement Zone core — Reset, ComputeShared, event queue

Per-tick event queue backed by a slice. entityEvents tracks per-entity
event indexes so clearQueuedEvents can tombstone pending events when an
entity is removed mid-tick. ComputeShared concatenates non-tombstoned
Enclosed event Bytes into Zone.shared; Follows events stay in the queue
for per-player filtering in sub-spec 4b-4."
```

---

## Task 3: All 11 mutation methods + tests

The actual event-queueing + entity-list maintenance work.

**Files:**
- Modify: `pkg/zone/zone.go` (append mutation methods)
- Modify: `pkg/zone/zone_test.go` (append mutation tests)

- [ ] **Step 3.1: Write failing tests**

Append to `pkg/zone/zone_test.go`:

```go
import (
	// already imported: "bytes", "testing"
	// add:
	"github.com/zsrv/goscape/pkg/entity"
	"github.com/zsrv/goscape/pkg/rsbuf"
)

// --- Loc mutations ---

func TestAddLocQueuesEnclosedLocAddChange(t *testing.T) {
	z := New(0, 0, 0, 0)
	loc := entity.NewLoc(0, 0, 0, 1, 1, entity.LifecycleDespawn, 100, 5, 2)
	z.AddLoc(loc)

	if len(z.Events()) != 1 {
		t.Fatalf("events len: got %d, want 1", len(z.Events()))
	}
	e := z.Events()[0]
	if e.Type != ZoneEventEnclosed {
		t.Errorf("Type: got %v, want Enclosed", e.Type)
	}
	if e.ReceiverID != PublicReceiver {
		t.Errorf("ReceiverID: got %d, want -1", e.ReceiverID)
	}
	if len(e.Bytes) == 0 || e.Bytes[0] != rsbuf.ZoneOpLocAddChange {
		t.Errorf("Bytes[0]: got %v, want ZoneOpLocAddChange=%d", e.Bytes, rsbuf.ZoneOpLocAddChange)
	}
}

func TestAddLocDespawnAppendsToLocs(t *testing.T) {
	z := New(0, 0, 0, 0)
	loc := entity.NewLoc(0, 0, 0, 1, 1, entity.LifecycleDespawn, 100, 0, 0)
	z.AddLoc(loc)
	if len(z.Locs) != 1 || z.Locs[0] != loc {
		t.Errorf("Locs: got %v, want [loc]", z.Locs)
	}
}

func TestAddLocRespawnDoesNotAppendToLocs(t *testing.T) {
	z := New(0, 0, 0, 0)
	loc := entity.NewLoc(0, 0, 0, 1, 1, entity.LifecycleRespawn, 100, 0, 0)
	z.AddLoc(loc)
	if len(z.Locs) != 0 {
		t.Errorf("Locs: got %d entries, want 0 (Respawn lifecycle)", len(z.Locs))
	}
	// But event still queued.
	if len(z.Events()) != 1 {
		t.Errorf("events: got %d, want 1", len(z.Events()))
	}
}

func TestChangeLocEmitsLocAddChange(t *testing.T) {
	z := New(0, 0, 0, 0)
	loc := entity.NewLoc(0, 0, 0, 1, 1, entity.LifecycleDespawn, 100, 0, 0)
	z.ChangeLoc(loc)
	if z.Events()[0].Bytes[0] != rsbuf.ZoneOpLocAddChange {
		t.Errorf("opcode: got %d, want %d", z.Events()[0].Bytes[0], rsbuf.ZoneOpLocAddChange)
	}
}

func TestRemoveLocEmitsLocDelAndPurges(t *testing.T) {
	z := New(0, 0, 0, 0)
	loc := entity.NewLoc(0, 0, 0, 1, 1, entity.LifecycleDespawn, 100, 0, 0)
	z.AddLoc(loc)    // queues LocAddChange
	z.RemoveLoc(loc) // tombstones LocAddChange + queues LocDel

	if len(z.Locs) != 0 {
		t.Errorf("Locs after remove: got %d, want 0", len(z.Locs))
	}
	z.ComputeShared()
	// After tombstoning the add, only the LocDel bytes should be in shared.
	if len(z.Shared()) == 0 {
		t.Fatal("Shared should include LocDel bytes")
	}
	if z.Shared()[0] != rsbuf.ZoneOpLocDel {
		t.Errorf("first shared opcode: got %d, want LocDel=%d", z.Shared()[0], rsbuf.ZoneOpLocDel)
	}
	// The original AddChange opcode should NOT appear in shared (tombstoned).
	// (Can't rely on byte equality of 59 in payload; check length).
	// LocDel payload is 2 bytes (coord + packed) + 1 opcode = 3 bytes.
	if len(z.Shared()) != 3 {
		t.Errorf("Shared len: got %d, want 3 (just the LocDel)", len(z.Shared()))
	}
}

func TestAnimLocDoesNotTouchLocs(t *testing.T) {
	z := New(0, 0, 0, 0)
	loc := entity.NewLoc(0, 0, 0, 1, 1, entity.LifecycleDespawn, 100, 0, 0)
	z.AnimLoc(loc, 42)
	if len(z.Locs) != 0 {
		t.Errorf("AnimLoc should not append to Locs; got %d", len(z.Locs))
	}
	if z.Events()[0].Bytes[0] != rsbuf.ZoneOpLocAnim {
		t.Errorf("opcode: want LocAnim=%d", rsbuf.ZoneOpLocAnim)
	}
}

func TestMergeLocEmitsLocMerge(t *testing.T) {
	z := New(0, 0, 0, 0)
	loc := entity.NewLoc(0, 5, 5, 2, 2, entity.LifecycleDespawn, 100, 0, 0)
	z.MergeLoc(loc, 3, 10, 20, 6, 4, 4, 6)
	if z.Events()[0].Bytes[0] != rsbuf.ZoneOpLocMerge {
		t.Errorf("opcode: want LocMerge=%d", rsbuf.ZoneOpLocMerge)
	}
}

// --- Obj mutations ---

func TestAddObjPublicIsEnclosed(t *testing.T) {
	z := New(0, 0, 0, 0)
	obj := entity.NewObj(0, 0, 0, entity.LifecycleDespawn, 995, 10)
	z.AddObj(obj, PublicReceiver)
	e := z.Events()[0]
	if e.Type != ZoneEventEnclosed {
		t.Errorf("public drop should be Enclosed; got %v", e.Type)
	}
	if e.Bytes[0] != rsbuf.ZoneOpObjAdd {
		t.Errorf("opcode: want ObjAdd=%d", rsbuf.ZoneOpObjAdd)
	}
	if len(z.Objs) != 1 {
		t.Errorf("Objs: got %d, want 1", len(z.Objs))
	}
}

func TestAddObjPrivateIsFollows(t *testing.T) {
	z := New(0, 0, 0, 0)
	obj := entity.NewObj(0, 0, 0, entity.LifecycleDespawn, 995, 10)
	z.AddObj(obj, 5)
	e := z.Events()[0]
	if e.Type != ZoneEventFollows {
		t.Errorf("private drop should be Follows; got %v", e.Type)
	}
	if e.ReceiverID != 5 {
		t.Errorf("ReceiverID: got %d, want 5", e.ReceiverID)
	}
}

func TestChangeObjEmitsFollowsObjCount(t *testing.T) {
	z := New(0, 0, 0, 0)
	obj := entity.NewObj(0, 0, 0, entity.LifecycleDespawn, 995, 10)
	obj.ReceiverID = 7
	z.ChangeObj(obj, 10, 25, 100)
	e := z.Events()[0]
	if e.Type != ZoneEventFollows {
		t.Errorf("ChangeObj should be Follows; got %v", e.Type)
	}
	if e.Bytes[0] != rsbuf.ZoneOpObjCount {
		t.Errorf("opcode: want ObjCount=%d", rsbuf.ZoneOpObjCount)
	}
	if obj.Count != 25 {
		t.Errorf("Count after ChangeObj: got %d, want 25", obj.Count)
	}
	if obj.LastChange != 100 {
		t.Errorf("LastChange: got %d, want 100", obj.LastChange)
	}
}

func TestRemoveObjPurgesPendingAdd(t *testing.T) {
	z := New(0, 0, 0, 0)
	obj := entity.NewObj(0, 0, 0, entity.LifecycleDespawn, 995, 10)
	z.AddObj(obj, PublicReceiver)
	z.RemoveObj(obj, 100)

	if len(z.Objs) != 0 {
		t.Errorf("Objs after remove: got %d, want 0", len(z.Objs))
	}
	z.ComputeShared()
	// The add was tombstoned; only the del remains.
	if len(z.Shared()) == 0 {
		t.Fatal("Shared should include ObjDel bytes")
	}
	if z.Shared()[0] != rsbuf.ZoneOpObjDel {
		t.Errorf("first shared opcode: got %d, want ObjDel=%d", z.Shared()[0], rsbuf.ZoneOpObjDel)
	}
}

func TestRemoveObjSkipsEventIfLifecycleTransitionedThisTick(t *testing.T) {
	z := New(0, 0, 0, 0)
	obj := entity.NewObj(0, 0, 0, entity.LifecycleDespawn, 995, 10)
	obj.LastLifecycleTick = 100
	z.RemoveObj(obj, 100) // lastLifecycleTick == currentTick → skip queuing
	if len(z.Events()) != 0 {
		t.Errorf("events: got %d, want 0 (skip because lifecycle transition this tick)", len(z.Events()))
	}
}

func TestRevealObjEmitsEnclosedObjReveal(t *testing.T) {
	z := New(0, 0, 0, 0)
	obj := entity.NewObj(0, 0, 0, entity.LifecycleDespawn, 995, 10)
	obj.ReceiverID = 5
	obj.Reveal = 50
	z.RevealObj(obj, 5)

	e := z.Events()[0]
	if e.Type != ZoneEventEnclosed {
		t.Errorf("RevealObj should be Enclosed; got %v", e.Type)
	}
	if e.Bytes[0] != rsbuf.ZoneOpObjReveal {
		t.Errorf("opcode: want ObjReveal=%d", rsbuf.ZoneOpObjReveal)
	}
	if obj.ReceiverID != PublicReceiver {
		t.Errorf("ReceiverID after reveal: got %d, want -1", obj.ReceiverID)
	}
	if obj.Reveal != -1 {
		t.Errorf("Reveal after reveal: got %d, want -1", obj.Reveal)
	}
}

// --- Non-entity events ---

func TestAnimMapEnclosed(t *testing.T) {
	z := New(0, 0, 0, 0)
	z.AnimMap(3, 4, 200, 5, 50)
	e := z.Events()[0]
	if e.Type != ZoneEventEnclosed {
		t.Errorf("AnimMap should be Enclosed")
	}
	if e.Bytes[0] != rsbuf.ZoneOpMapAnim {
		t.Errorf("opcode: want MapAnim=%d", rsbuf.ZoneOpMapAnim)
	}
}

func TestMapProjAnimEnclosed(t *testing.T) {
	z := New(0, 0, 0, 0)
	z.MapProjAnim(3, 4, 5, 7, 0, 100, 10, 0, 0, 50, 40, 30)
	e := z.Events()[0]
	if e.Type != ZoneEventEnclosed {
		t.Errorf("MapProjAnim should be Enclosed")
	}
	if e.Bytes[0] != rsbuf.ZoneOpMapProjAnim {
		t.Errorf("opcode: want MapProjAnim=%d", rsbuf.ZoneOpMapProjAnim)
	}
}

func TestEventOrderPreserved(t *testing.T) {
	z := New(0, 0, 0, 0)
	loc := entity.NewLoc(0, 0, 0, 1, 1, entity.LifecycleDespawn, 100, 0, 0)
	z.AnimLoc(loc, 1) // event 0: LocAnim
	z.AddLoc(loc)     // event 1: LocAddChange
	z.ComputeShared()
	shared := z.Shared()
	if len(shared) == 0 || shared[0] != rsbuf.ZoneOpLocAnim {
		t.Errorf("first shared opcode: got %d, want LocAnim=%d", shared[0], rsbuf.ZoneOpLocAnim)
	}
}
```

- [ ] **Step 3.2: Run tests — verify FAIL**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/zone/ -run 'TestAddLoc|TestChangeLoc|TestRemoveLoc|TestAnimLoc|TestMergeLoc|TestAddObj|TestChangeObj|TestRemoveObj|TestRevealObj|TestAnimMap|TestMapProjAnim|TestEventOrder' -v`
Expected: FAIL — undefined AddLoc / AddObj / etc.

- [ ] **Step 3.3: Implement all 11 mutation methods**

Append to `pkg/zone/zone.go`:

```go
import (
	// existing: "github.com/zsrv/goscape/pkg/entity"
	// add:
	"github.com/zsrv/goscape/pkg/coordgrid"
	"github.com/zsrv/goscape/pkg/io/packet"
	"github.com/zsrv/goscape/pkg/rsbuf"
)

// encodeNested builds [opcode, ...payload] by calling the rsbuf encoder
// into a fresh Packet. Returns a newly-owned byte slice.
func encodeNested(opcode byte, fn func(*packet.Packet)) []byte {
	buf := packet.NewPacket(nil)
	buf.P1(opcode)
	fn(buf)
	return append([]byte(nil), buf.Data...)
}

// ---- loc mutations ----

// AddLoc activates a loc and queues a LOC_ADD_CHANGE Enclosed event. For
// dynamic (Despawn-lifecycle) locs the pointer is appended to z.Locs so
// the full-follows replay in 4b-4 can iterate active dynamics.
func (z *Zone) AddLoc(loc *entity.Loc) {
	coord := coordgrid.PackZoneCoord(loc.X, loc.Z)
	bytes := encodeNested(rsbuf.ZoneOpLocAddChange, func(buf *packet.Packet) {
		rsbuf.EncodeLocAddChange(buf, coord, loc.Shape(), loc.Angle(), loc.Type())
	})
	if loc.Lifecycle == entity.LifecycleDespawn {
		z.Locs = append(z.Locs, loc)
	}
	z.queueEvent(&loc.NonPathing, ZoneEvent{
		Type:       ZoneEventEnclosed,
		ReceiverID: PublicReceiver,
		Bytes:      bytes,
	})
}

// ChangeLoc emits a LOC_ADD_CHANGE for an existing active loc whose
// type/shape/angle changed. Does not modify Locs.
func (z *Zone) ChangeLoc(loc *entity.Loc) {
	coord := coordgrid.PackZoneCoord(loc.X, loc.Z)
	bytes := encodeNested(rsbuf.ZoneOpLocAddChange, func(buf *packet.Packet) {
		rsbuf.EncodeLocAddChange(buf, coord, loc.Shape(), loc.Angle(), loc.Type())
	})
	z.queueEvent(&loc.NonPathing, ZoneEvent{
		Type:       ZoneEventEnclosed,
		ReceiverID: PublicReceiver,
		Bytes:      bytes,
	})
}

// RemoveLoc removes a dynamic loc from z.Locs (if Despawn lifecycle),
// tombstones its pending events, and queues a LOC_DEL Enclosed event.
func (z *Zone) RemoveLoc(loc *entity.Loc) {
	coord := coordgrid.PackZoneCoord(loc.X, loc.Z)
	if loc.Lifecycle == entity.LifecycleDespawn {
		for i, l := range z.Locs {
			if l == loc {
				z.Locs = append(z.Locs[:i], z.Locs[i+1:]...)
				break
			}
		}
	}
	z.clearQueuedEvents(&loc.NonPathing)
	bytes := encodeNested(rsbuf.ZoneOpLocDel, func(buf *packet.Packet) {
		rsbuf.EncodeLocDel(buf, coord, loc.Shape(), loc.Angle())
	})
	z.queueEvent(&loc.NonPathing, ZoneEvent{
		Type:       ZoneEventEnclosed,
		ReceiverID: PublicReceiver,
		Bytes:      bytes,
	})
}

// AnimLoc queues a LOC_ANIM event. Does not modify Locs.
func (z *Zone) AnimLoc(loc *entity.Loc, seq int) {
	coord := coordgrid.PackZoneCoord(loc.X, loc.Z)
	bytes := encodeNested(rsbuf.ZoneOpLocAnim, func(buf *packet.Packet) {
		rsbuf.EncodeLocAnim(buf, coord, loc.Shape(), loc.Angle(), seq)
	})
	z.queueEvent(&loc.NonPathing, ZoneEvent{
		Type:       ZoneEventEnclosed,
		ReceiverID: PublicReceiver,
		Bytes:      bytes,
	})
}

// MergeLoc queues a LOC_MERGE event. east/south/west/north are absolute
// tile coordinates; the encoder stores deltas relative to srcX/srcZ.
func (z *Zone) MergeLoc(
	loc *entity.Loc,
	playerSlot, startCycle, endCycle int,
	east, south, west, north int,
) {
	coord := coordgrid.PackZoneCoord(loc.X, loc.Z)
	bytes := encodeNested(rsbuf.ZoneOpLocMerge, func(buf *packet.Packet) {
		rsbuf.EncodeLocMerge(buf, coord,
			loc.Shape(), loc.Angle(), loc.Type(),
			startCycle, endCycle, playerSlot,
			east-loc.X, south-loc.Z, west-loc.X, north-loc.Z)
	})
	z.queueEvent(&loc.NonPathing, ZoneEvent{
		Type:       ZoneEventEnclosed,
		ReceiverID: PublicReceiver,
		Bytes:      bytes,
	})
}

// ---- obj mutations ----

// AddObj appends a dynamic obj to z.Objs and queues an OBJ_ADD event.
// Enclosed if receiverID == PublicReceiver; Follows otherwise.
// TODO(beyond-4b): enforce per-zone obj cap (TS: OBJS = 129) with
// oldest-obj eviction.
func (z *Zone) AddObj(obj *entity.Obj, receiverID int) {
	coord := coordgrid.PackZoneCoord(obj.X, obj.Z)
	if obj.Lifecycle == entity.LifecycleDespawn {
		z.Objs = append(z.Objs, obj)
	}
	bytes := encodeNested(rsbuf.ZoneOpObjAdd, func(buf *packet.Packet) {
		rsbuf.EncodeObjAdd(buf, coord, obj.Type, obj.Count)
	})
	evType := ZoneEventEnclosed
	if receiverID != PublicReceiver {
		evType = ZoneEventFollows
	}
	z.queueEvent(&obj.NonPathing, ZoneEvent{
		Type:       evType,
		ReceiverID: receiverID,
		Bytes:      bytes,
	})
}

// ChangeObj updates obj.Count + LastChange and queues a FOLLOWS OBJ_COUNT
// event routed to the obj's current receiver.
func (z *Zone) ChangeObj(obj *entity.Obj, oldCount, newCount, currentTick int) {
	obj.Count = newCount
	obj.LastChange = currentTick
	coord := coordgrid.PackZoneCoord(obj.X, obj.Z)
	bytes := encodeNested(rsbuf.ZoneOpObjCount, func(buf *packet.Packet) {
		rsbuf.EncodeObjCount(buf, coord, obj.Type, oldCount, newCount)
	})
	z.queueEvent(&obj.NonPathing, ZoneEvent{
		Type:       ZoneEventFollows,
		ReceiverID: obj.ReceiverID,
		Bytes:      bytes,
	})
}

// RemoveObj removes a dynamic obj from z.Objs, tombstones pending events,
// and queues an OBJ_DEL event — unless this same tick already transitioned
// the obj's lifecycle (matches TS's lastLifecycleTick === currentTick check).
func (z *Zone) RemoveObj(obj *entity.Obj, currentTick int) {
	coord := coordgrid.PackZoneCoord(obj.X, obj.Z)
	if obj.Lifecycle == entity.LifecycleDespawn {
		for i, o := range z.Objs {
			if o == obj {
				z.Objs = append(z.Objs[:i], z.Objs[i+1:]...)
				break
			}
		}
	}
	z.clearQueuedEvents(&obj.NonPathing)
	if obj.LastLifecycleTick == currentTick {
		return
	}
	bytes := encodeNested(rsbuf.ZoneOpObjDel, func(buf *packet.Packet) {
		rsbuf.EncodeObjDel(buf, coord, obj.Type)
	})
	evType := ZoneEventEnclosed
	receiver := PublicReceiver
	if obj.Lifecycle == entity.LifecycleDespawn && obj.ReceiverID != PublicReceiver {
		evType = ZoneEventFollows
		receiver = obj.ReceiverID
	}
	z.queueEvent(&obj.NonPathing, ZoneEvent{
		Type:       evType,
		ReceiverID: receiver,
		Bytes:      bytes,
	})
}

// RevealObj transitions a private drop to public and queues an ENCLOSED
// OBJ_REVEAL event. Simplified vs TS — does not consult ObjType
// tradeable/members flags.
// TODO(beyond-4b): wire tradeability gating.
func (z *Zone) RevealObj(obj *entity.Obj, receiverSlot int) {
	initialReceiver := obj.ReceiverID
	_ = initialReceiver // retained for future filtering logic
	obj.ReceiverID = PublicReceiver
	obj.Reveal = -1
	obj.LastChange = -1
	coord := coordgrid.PackZoneCoord(obj.X, obj.Z)
	bytes := encodeNested(rsbuf.ZoneOpObjReveal, func(buf *packet.Packet) {
		rsbuf.EncodeObjReveal(buf, coord, obj.Type, obj.Count, receiverSlot)
	})
	z.queueEvent(&obj.NonPathing, ZoneEvent{
		Type:       ZoneEventEnclosed,
		ReceiverID: PublicReceiver,
		Bytes:      bytes,
	})
}

// ---- non-entity events ----

// AnimMap queues a MAP_ANIM event at (x, zCoord).
func (z *Zone) AnimMap(x, zCoord, spotanim, height, delay int) {
	coord := coordgrid.PackZoneCoord(x, zCoord)
	bytes := encodeNested(rsbuf.ZoneOpMapAnim, func(buf *packet.Packet) {
		rsbuf.EncodeMapAnim(buf, coord, spotanim, height, delay)
	})
	z.queueEvent(nil, ZoneEvent{
		Type:       ZoneEventEnclosed,
		ReceiverID: PublicReceiver,
		Bytes:      bytes,
	})
}

// MapProjAnim queues a MAP_PROJANIM event for a projectile traveling
// (srcX, srcZ) → (dstX, dstZ).
func (z *Zone) MapProjAnim(
	srcX, srcZ, dstX, dstZ int,
	target, spotanim, srcHeight, dstHeight int,
	startDelay, endDelay, peak, arc int,
) {
	coord := coordgrid.PackZoneCoord(srcX, srcZ)
	bytes := encodeNested(rsbuf.ZoneOpMapProjAnim, func(buf *packet.Packet) {
		rsbuf.EncodeMapProjAnim(buf, coord,
			dstX-srcX, dstZ-srcZ,
			target, spotanim,
			srcHeight, dstHeight,
			startDelay, endDelay,
			peak, arc)
	})
	z.queueEvent(nil, ZoneEvent{
		Type:       ZoneEventEnclosed,
		ReceiverID: PublicReceiver,
		Bytes:      bytes,
	})
}
```

- [ ] **Step 3.4: Run tests — verify PASS**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/zone/ -v`
Expected: all PASS (Task 1's 9 + Task 2's 6 + Task 3's ~17 = ~32 tests).

- [ ] **Step 3.5: Full suite + vet + race**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`
Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...`
Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./...`
Expected: all clean.

- [ ] **Step 3.6: Commit**

```bash
git add pkg/zone/zone.go pkg/zone/zone_test.go
git commit --no-gpg-sign -m "feat(zone): 11 event-queueing mutation methods

AddLoc/ChangeLoc/RemoveLoc/AnimLoc/MergeLoc for dynamic locs; AddObj/
ChangeObj/RemoveObj/RevealObj for ground items; AnimMap/MapProjAnim for
non-entity events. Each encodes the zone-nested packet via pkg/rsbuf,
prepends the opcode byte, and stores the combined bytes in a ZoneEvent
with the correct Type (Enclosed vs Follows) and ReceiverID.

RemoveLoc/RemoveObj purge pending events via clearQueuedEvents (tombstone
pattern); ComputeShared skips tombstoned entries. RemoveObj honours the
lastLifecycleTick check from the TS reference.

Simplifications vs TS (documented with TODOs): no OBJS=129 eviction cap;
RevealObj does not consult ObjType tradeable/members."
```

---

## Final Verification

- [ ] **Step F.1: Race detector**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./...`
Expected: PASS.

- [ ] **Step F.2: go vet**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...`
Expected: no output.

- [ ] **Step F.3: Package file count**

Run: `ls pkg/zone/ | wc -l`
Expected: `8` (4 production + 4 test).

---

## Spec coverage map

| Spec requirement | Task |
|---|---|
| `ZoneEvent` + `ZoneEventType` | Task 1 |
| `PublicReceiver` constant | Task 1 |
| `ZoneIndex` / `UnpackIndex` | Task 1 |
| `ZoneMap` with Get/GetByIndex/Grid + counts | Task 1 |
| `ZoneGrid` vendored with radius-search | Task 1 |
| `Zone` struct + `New` | Task 1 (stub) / Task 2 (full) |
| `Reset`, `Shared`, `Events`, `ComputeShared` | Task 2 |
| `queueEvent`, `clearQueuedEvents` tombstone pattern | Task 2 |
| 5 loc mutations (Add/Change/Remove/Anim/Merge) | Task 3 |
| 4 obj mutations (Add/Change/Remove/Reveal) | Task 3 |
| 2 map events (AnimMap, MapProjAnim) | Task 3 |
| All acceptance criteria | Task F |

No gaps.
