# Sub-spec 4b-2: Zone Packet Encoders — Design

**Status:** Draft → ready for plan
**Scope:** 13 zone-related packet encoders in `pkg/rsbuf/`, plus the 3 outer opcodes on `prot.go` and the 10 nested-opcode constants. Pure wire-format layer.
**Out of scope:** Zone bookkeeping (`Zone`/`ZoneEvent`) → 4b-3. Integration (`Player.updateZones`, senders) → 4b-4.

---

## Goal

Add every zone-related byte-level encoder the server needs, with exhaustive byte-level tests. After this sub-spec, 4b-3 can compose shared zone buffers by calling these encoders, and 4b-4 can wrap the three outer packets with `p.writeOut(gameserver.OpUpdateZone*, ...)`.

## Architecture

One new file `pkg/rsbuf/zone_encoders.go` holding:
- 10 nested-opcode constants (`ZoneOpLocMerge = 23` etc.)
- 1 private helper (`packLocShapeAngle`)
- 10 nested encoder functions (write payload only, no opcode byte)
- 3 outer encoder functions (write 2-byte zone-relative header; `Enclosed` also appends data bytes)

One edit to `pkg/io/protocol/game/server/prot.go` adding the 3 outer `Op` entries.

All encoder functions write directly to a caller-supplied `*packet.Packet`, matching the style of `EncodePlayer`/`EncodeNpc` already in `pkg/rsbuf/`.

## Components

### 1. Outer opcodes — `pkg/io/protocol/game/server/prot.go`

Add to the existing `var (...)` block:
```go
OpUpdateZonePartialFollows  = Op{Opcode: 7,   PayloadSize: 2}
OpUpdateZoneFullFollows     = Op{Opcode: 135, PayloadSize: 2}
OpUpdateZonePartialEnclosed = Op{Opcode: 162, PayloadSize: -2}
```

Sizes verified against the Java client's `SERVERPROT_SIZES` table (`Client-Java/src/main/java/jagex2/io/Protocol.java`): opcodes 7 and 135 are **fixed 2-byte** (header-only — no length prefix on the wire); opcode 162 is `-2` because it carries variable-length data.

### 2. Nested opcode constants — `pkg/rsbuf/zone_encoders.go`

```go
const (
	ZoneOpLocMerge     = 23
	ZoneOpLocAnim      = 42
	ZoneOpObjDel       = 49
	ZoneOpObjReveal    = 50
	ZoneOpLocAddChange = 59
	ZoneOpMapProjAnim  = 69
	ZoneOpLocDel       = 76
	ZoneOpObjCount     = 151
	ZoneOpMapAnim      = 191
	ZoneOpObjAdd       = 223
)
```

These will be prepended by the zone subsystem (4b-3) when composing the shared buffer, so the encoders themselves don't write them.

### 3. Private helper

```go
// packLocShapeAngle returns the byte (shape<<2)|(angle&0x3), the common
// second byte of every LOC_* zone-nested packet.
func packLocShapeAngle(shape, angle int) byte {
	return byte((shape << 2) | (angle & 0x3))
}
```

### 4. Nested encoders — payload wire formats

All payloads start with `p1(coord)` where `coord byte` was produced by `coordgrid.PackZoneCoord(x, z)`. Wire formats from the TS reference (`src/network/game/server/codec/*Encoder.ts`):

```go
// LOC_ADD_CHANGE (59, 4 bytes payload)
//   p1 coord, p1 packed(shape,angle), p2 locId
func EncodeLocAddChange(buf *packet.Packet, coord byte, shape, angle, locID int)

// LOC_ANIM (42, 4 bytes payload) — same layout, locID replaced by seq
func EncodeLocAnim(buf *packet.Packet, coord byte, shape, angle, seq int)

// LOC_DEL (76, 2 bytes payload)
//   p1 coord, p1 packed(shape,angle)
func EncodeLocDel(buf *packet.Packet, coord byte, shape, angle int)

// LOC_MERGE (23, 14 bytes payload)
//   p1 coord
//   p1 packed(shape,angle)
//   p2 locId
//   p2 startCycle
//   p2 endCycle
//   p2 playerSlot
//   p1 dxEast    (east  - srcX)
//   p1 dzSouth   (south - srcZ)
//   p1 dxWest    (west  - srcX)
//   p1 dzNorth   (north - srcZ)
func EncodeLocMerge(
	buf *packet.Packet,
	coord byte,
	shape, angle, locID int,
	startCycle, endCycle, playerSlot int,
	dxEast, dzSouth, dxWest, dzNorth int,
)

// MAP_ANIM (191, 6 bytes payload)
//   p1 coord, p2 spotanim, p1 height, p2 delay
func EncodeMapAnim(buf *packet.Packet, coord byte, spotanim, height, delay int)

// MAP_PROJANIM (69, 15 bytes payload)
//   p1 coord
//   p1 dx, p1 dz                // dst - src, signed i8
//   p2 target                   // 0=coord, >0=npc+1, <0=-(player slot)-1
//   p2 spotanim
//   p1 srcHeight, p1 dstHeight
//   p2 startDelay, p2 endDelay
//   p1 peak, p1 arc
func EncodeMapProjAnim(
	buf *packet.Packet,
	coord byte,
	dx, dz int,
	target, spotanim int,
	srcHeight, dstHeight int,
	startDelay, endDelay int,
	peak, arc int,
)

// OBJ_ADD (223, 5 bytes payload)
//   p1 coord, p2 obj, p2 min(count, 65535)
func EncodeObjAdd(buf *packet.Packet, coord byte, obj, count int)

// OBJ_COUNT (151, 7 bytes payload)
//   p1 coord, p2 obj, p2 min(oldCount,65535), p2 min(newCount,65535)
func EncodeObjCount(buf *packet.Packet, coord byte, obj, oldCount, newCount int)

// OBJ_DEL (49, 3 bytes payload)
//   p1 coord, p2 obj
func EncodeObjDel(buf *packet.Packet, coord byte, obj int)

// OBJ_REVEAL (50, 7 bytes payload)
//   p1 coord, p2 obj, p2 min(count,65535), p2 receiverID
func EncodeObjReveal(buf *packet.Packet, coord byte, obj, count, receiverID int)
```

Count clamping (`min(count, 65535)`): implemented as `if count > 65535 { count = 65535 }` inline in each of the three encoders that need it.

### 5. Outer encoders

```go
// EncodeZoneFullFollows writes the 2-byte zone-relative header for
// OpUpdateZoneFullFollows (135). Callers wrap this in p.writeOut.
func EncodeZoneFullFollows(buf *packet.Packet, zoneX, zoneZ, originX, originZ int)

// EncodeZonePartialFollows writes the 2-byte zone-relative header for
// OpUpdateZonePartialFollows (7). Callers wrap this in p.writeOut.
func EncodeZonePartialFollows(buf *packet.Packet, zoneX, zoneZ, originX, originZ int)

// EncodeZonePartialEnclosed writes the 2-byte zone-relative header followed
// by the precomputed shared data bytes for OpUpdateZonePartialEnclosed (162).
func EncodeZonePartialEnclosed(buf *packet.Packet, zoneX, zoneZ, originX, originZ int, data []byte)
```

All three compute the zone-relative offset via:
```go
dx := byte((zoneX << 3) - coordgrid.ZoneOrigin(originX))
dz := byte((zoneZ << 3) - coordgrid.ZoneOrigin(originZ))
buf.P1(dx)
buf.P1(dz)
// Enclosed only:
buf.PData(data)
```

## Data Flow

None within this sub-spec. 4b-3 will call nested encoders while composing `Zone.shared`; 4b-4 will call outer encoders while building outbound packets per-player-per-zone.

## Error Handling

None — inputs are raw ints with documented overflow behaviour (16-bit clamping on counts). No I/O.

## Testing

One byte-level test per encoder (13 total). Each test constructs a Packet, calls the encoder, and asserts exact `buf.Data` bytes. Plus two edge-case tests.

### `pkg/rsbuf/zone_encoders_test.go`

**Nested encoders (10 tests):**

- `TestEncodeLocAddChange` — coord=0x62, shape=10, angle=3, locID=5000 → `[0x62, 0x2B, 0x13, 0x88]`
- `TestEncodeLocAnim` — coord=0x00, shape=0, angle=0, seq=1 → `[0, 0, 0, 1]`
- `TestEncodeLocDel` — coord=0x77, shape=2, angle=1 → `[0x77, 0x09]`
- `TestEncodeLocMerge` — full example with shape=1, angle=0, locID=100, start=10, end=20, slot=3, dx/dz all 2 → 14 bytes
- `TestEncodeMapAnim` — coord=0x11, spotanim=200, height=5, delay=50 → `[0x11, 0x00, 0xC8, 0x05, 0x00, 0x32]`
- `TestEncodeMapProjAnim` — happy path, verify all 15 bytes in order
- `TestEncodeMapProjAnimSignedDeltas` — dx=-5, dz=-10 should serialize as `0xFB, 0xF6`
- `TestEncodeObjAdd` — coord=0x45, obj=4151, count=1 → `[0x45, 0x10, 0x37, 0x00, 0x01]`
- `TestEncodeObjCount` — verify oldCount AND newCount clamped at 65535
- `TestEncodeObjDel` — coord=0x33, obj=1 → `[0x33, 0x00, 0x01]`
- `TestEncodeObjReveal` — 7 bytes including receiverID

**Outer encoders (3 tests):**

- `TestEncodeZoneFullFollows` — zone(386, 388) viewed from origin(3094, 3106): originX=3094 → ZoneOrigin=3072; (386<<3)-3072 = 3088-3072 = 16; verify `[16, dz]` where dz computed the same way.
- `TestEncodeZonePartialFollows` — identical header math to FullFollows; assert payload == 2 bytes.
- `TestEncodeZonePartialEnclosedAppendsData` — header 2 bytes + `data = []byte{0xAA, 0xBB, 0xCC}` → total 5 bytes with data appended verbatim.

### Count-clamping edge cases

Add one test (`TestCountClampingAtBoundary`) that calls:
- `EncodeObjAdd(buf, 0, 1, 65535)` — exactly at boundary, should write `0xFF 0xFF`
- `EncodeObjAdd(buf2, 0, 1, 65536)` — one over, should also write `0xFF 0xFF`
- `EncodeObjCount(buf3, 0, 1, 70000, 80000)` — both clamped
- `EncodeObjReveal(buf4, 0, 1, 100000, 42)` — clamped

### Zero-count edge

- `TestEncodeObjCountZero` — oldCount=0, newCount=5 should serialize both cleanly (`0x00 0x00 0x00 0x05`).

## Acceptance Criteria

1. `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...` passes.
2. `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...` clean.
3. `pkg/rsbuf/zone_encoders.go` exports 13 encoders + 10 opcode constants.
4. `pkg/io/protocol/game/server/prot.go` has exactly 3 new `OpUpdateZone*` entries with PayloadSize -2.
5. Zero changes outside `pkg/rsbuf/` and `pkg/io/protocol/game/server/prot.go`.

## LOC Estimate

| File | LOC |
|---|---|
| `pkg/rsbuf/zone_encoders.go` | ~180 |
| `pkg/rsbuf/zone_encoders_test.go` | ~200 |
| `pkg/io/protocol/game/server/prot.go` | +5 |
| **Total** | **~385** |

## Dependencies & Risks

- **`coordgrid.PackZoneCoord`** — added in 4b-1; used by tests to construct example `coord` bytes.
- **`coordgrid.ZoneOrigin`** — pre-existing (`pkg/coordgrid/coordgrid.go:29`); used by outer encoders.
- **`packet.PData`** — pre-existing (`pkg/io/packet/packet.go:407`); used by `EncodeZonePartialEnclosed`.
- **No risk of breaking existing code** — everything is additive; 4b-2 has no integration points.

## Deferred to Later Sub-specs

- **4b-3:** `Zone` struct, `ZoneEvent`, `ZoneMap`, `ZoneGrid`, World-level `addLoc/addObj/delLoc/delObj`, `Zone.computeShared` that prepends opcode bytes + calls these encoders in a loop.
- **4b-4:** `Player.updateZones`, `BuildArea.RebuildZones`, `processZones` tick phase, sender functions (`sendUpdateZoneFullFollows(p, zoneX, zoneZ)`).
