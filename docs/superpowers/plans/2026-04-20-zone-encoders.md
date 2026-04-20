# Sub-spec 4b-2: Zone Packet Encoders — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add 13 zone-related packet encoders + 3 outer opcodes + 10 nested-opcode constants. Pure wire-format layer.

**Architecture:** New file `pkg/rsbuf/zone_encoders.go` + tests, additive edit to `pkg/io/protocol/game/server/prot.go`. Each encoder writes directly to a `*packet.Packet`.

**Tech Stack:** Go 1.26, `pkg/io/packet` (P1/P2/P4/PData), `pkg/coordgrid` (ZoneOrigin).

**Build prefix:** `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go ...`.
**Commit flag:** `--no-gpg-sign`.

**Spec reference:** `docs/superpowers/specs/2026-04-20-zone-encoders-design.md`.

---

## File Structure

**Create:**
- `pkg/rsbuf/zone_encoders.go` — opcode constants + helper + 13 encoders
- `pkg/rsbuf/zone_encoders_test.go` — 13+ byte-level tests

**Modify:**
- `pkg/io/protocol/game/server/prot.go` — add 3 outer `Op{}` entries

---

## Task 1: Outer opcodes in prot.go

**Files:** Modify `pkg/io/protocol/game/server/prot.go`

- [ ] **Step 1.1: Add the 3 outer opcodes**

Inside the existing `var (...)` block (after `OpUpdateInvStopTransmit`), append:

```go
OpUpdateZonePartialFollows  = Op{Opcode: 7,   PayloadSize: -2}
OpUpdateZoneFullFollows     = Op{Opcode: 135, PayloadSize: -2}
OpUpdateZonePartialEnclosed = Op{Opcode: 162, PayloadSize: -2}
```

- [ ] **Step 1.2: Build**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...`
Expected: exit 0.

- [ ] **Step 1.3: Test unchanged**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`
Expected: PASS (nothing behavioural changed yet).

- [ ] **Step 1.4: Commit**

```bash
git add pkg/io/protocol/game/server/prot.go
git commit --no-gpg-sign -m "feat(server-prot): add 3 outer zone-update opcodes

OpUpdateZonePartialFollows  (7,   -2)
OpUpdateZoneFullFollows     (135, -2)
OpUpdateZonePartialEnclosed (162, -2)

Prerequisite for the zone-subsystem sender wrappers landing in sub-spec 4b-4."
```

---

## Task 2: Nested encoders + constants + helper

All 10 nested encoders + `packLocShapeAngle` helper + opcode constants in one file. TDD applied to the test file.

**Files:**
- Create: `pkg/rsbuf/zone_encoders.go`
- Create: `pkg/rsbuf/zone_encoders_test.go`

- [ ] **Step 2.1: Write the failing tests (nested only)**

Create `pkg/rsbuf/zone_encoders_test.go`:

```go
package rsbuf

import (
	"bytes"
	"testing"

	"github.com/zsrv/goscape/pkg/io/packet"
)

func TestEncodeLocAddChange(t *testing.T) {
	buf := packet.NewPacket(nil)
	// coord=0x62, shape=10(0b01010), angle=3 → packed=(10<<2)|3=0x2B
	// locID=5000=0x1388
	EncodeLocAddChange(buf, 0x62, 10, 3, 5000)
	want := []byte{0x62, 0x2B, 0x13, 0x88}
	if !bytes.Equal(buf.Data, want) {
		t.Errorf("got %v, want %v", buf.Data, want)
	}
}

func TestEncodeLocAnim(t *testing.T) {
	buf := packet.NewPacket(nil)
	EncodeLocAnim(buf, 0x00, 0, 0, 1)
	want := []byte{0, 0, 0, 1}
	if !bytes.Equal(buf.Data, want) {
		t.Errorf("got %v, want %v", buf.Data, want)
	}
}

func TestEncodeLocDel(t *testing.T) {
	buf := packet.NewPacket(nil)
	// shape=2, angle=1 → packed=(2<<2)|1=0x09
	EncodeLocDel(buf, 0x77, 2, 1)
	want := []byte{0x77, 0x09}
	if !bytes.Equal(buf.Data, want) {
		t.Errorf("got %v, want %v", buf.Data, want)
	}
}

func TestEncodeLocMerge(t *testing.T) {
	buf := packet.NewPacket(nil)
	// coord=0x42, shape=1, angle=0 → packed=0x04
	// locID=100=0x0064, startCycle=10=0x000A, endCycle=20=0x0014
	// playerSlot=3=0x0003, dxEast=2, dzSouth=2, dxWest=2, dzNorth=2
	EncodeLocMerge(buf, 0x42, 1, 0, 100, 10, 20, 3, 2, 2, 2, 2)
	want := []byte{
		0x42, 0x04,
		0x00, 0x64,
		0x00, 0x0A,
		0x00, 0x14,
		0x00, 0x03,
		2, 2, 2, 2,
	}
	if !bytes.Equal(buf.Data, want) {
		t.Errorf("got %v, want %v", buf.Data, want)
	}
}

func TestEncodeMapAnim(t *testing.T) {
	buf := packet.NewPacket(nil)
	// spotanim=200=0x00C8, delay=50=0x0032
	EncodeMapAnim(buf, 0x11, 200, 5, 50)
	want := []byte{0x11, 0x00, 0xC8, 0x05, 0x00, 0x32}
	if !bytes.Equal(buf.Data, want) {
		t.Errorf("got %v, want %v", buf.Data, want)
	}
}

func TestEncodeMapProjAnim(t *testing.T) {
	buf := packet.NewPacket(nil)
	// coord=0x00, dx=5, dz=10, target=0, spotanim=1,
	// srcHeight=2, dstHeight=3, startDelay=4, endDelay=5,
	// peak=6, arc=7
	EncodeMapProjAnim(buf, 0x00, 5, 10, 0, 1, 2, 3, 4, 5, 6, 7)
	want := []byte{
		0x00,
		5, 10,
		0x00, 0x00, // target
		0x00, 0x01, // spotanim
		2, 3,       // srcHeight, dstHeight
		0x00, 0x04, // startDelay
		0x00, 0x05, // endDelay
		6, 7,       // peak, arc
	}
	if !bytes.Equal(buf.Data, want) {
		t.Errorf("got %v, want %v", buf.Data, want)
	}
}

func TestEncodeMapProjAnimSignedDeltas(t *testing.T) {
	buf := packet.NewPacket(nil)
	// dx=-5 → byte 0xFB; dz=-10 → byte 0xF6
	EncodeMapProjAnim(buf, 0, -5, -10, 0, 0, 0, 0, 0, 0, 0, 0)
	if buf.Data[1] != 0xFB {
		t.Errorf("dx=-5 byte: got %#x, want 0xFB", buf.Data[1])
	}
	if buf.Data[2] != 0xF6 {
		t.Errorf("dz=-10 byte: got %#x, want 0xF6", buf.Data[2])
	}
}

func TestEncodeObjAdd(t *testing.T) {
	buf := packet.NewPacket(nil)
	// obj=4151=0x1037, count=1
	EncodeObjAdd(buf, 0x45, 4151, 1)
	want := []byte{0x45, 0x10, 0x37, 0x00, 0x01}
	if !bytes.Equal(buf.Data, want) {
		t.Errorf("got %v, want %v", buf.Data, want)
	}
}

func TestEncodeObjCount(t *testing.T) {
	buf := packet.NewPacket(nil)
	// Both counts within range.
	EncodeObjCount(buf, 0x10, 995, 100, 200)
	want := []byte{
		0x10,
		0x03, 0xE3, // 995
		0x00, 0x64, // 100
		0x00, 0xC8, // 200
	}
	if !bytes.Equal(buf.Data, want) {
		t.Errorf("got %v, want %v", buf.Data, want)
	}
}

func TestEncodeObjCountZero(t *testing.T) {
	buf := packet.NewPacket(nil)
	EncodeObjCount(buf, 0, 1, 0, 5)
	want := []byte{0, 0, 1, 0, 0, 0, 5}
	if !bytes.Equal(buf.Data, want) {
		t.Errorf("got %v, want %v", buf.Data, want)
	}
}

func TestEncodeObjDel(t *testing.T) {
	buf := packet.NewPacket(nil)
	EncodeObjDel(buf, 0x33, 1)
	want := []byte{0x33, 0x00, 0x01}
	if !bytes.Equal(buf.Data, want) {
		t.Errorf("got %v, want %v", buf.Data, want)
	}
}

func TestEncodeObjReveal(t *testing.T) {
	buf := packet.NewPacket(nil)
	// obj=995, count=100, receiverID=42
	EncodeObjReveal(buf, 0x20, 995, 100, 42)
	want := []byte{
		0x20,
		0x03, 0xE3, // obj=995
		0x00, 0x64, // count=100
		0x00, 0x2A, // receiverID=42
	}
	if !bytes.Equal(buf.Data, want) {
		t.Errorf("got %v, want %v", buf.Data, want)
	}
}

func TestCountClampingAtBoundary(t *testing.T) {
	// 65535 → 0xFF 0xFF unchanged
	buf := packet.NewPacket(nil)
	EncodeObjAdd(buf, 0, 1, 65535)
	if buf.Data[3] != 0xFF || buf.Data[4] != 0xFF {
		t.Errorf("count=65535: got %v, want 0xFF 0xFF tail", buf.Data)
	}

	// 65536 → clamped to 0xFF 0xFF
	buf2 := packet.NewPacket(nil)
	EncodeObjAdd(buf2, 0, 1, 65536)
	if buf2.Data[3] != 0xFF || buf2.Data[4] != 0xFF {
		t.Errorf("count=65536 should clamp; got %v", buf2.Data)
	}

	// ObjCount: both clamped
	buf3 := packet.NewPacket(nil)
	EncodeObjCount(buf3, 0, 1, 70000, 80000)
	want3 := []byte{0, 0, 1, 0xFF, 0xFF, 0xFF, 0xFF}
	if !bytes.Equal(buf3.Data, want3) {
		t.Errorf("oldCount+newCount clamp: got %v, want %v", buf3.Data, want3)
	}

	// ObjReveal: count clamped, receiverID not
	buf4 := packet.NewPacket(nil)
	EncodeObjReveal(buf4, 0, 1, 100000, 42)
	if buf4.Data[3] != 0xFF || buf4.Data[4] != 0xFF {
		t.Errorf("ObjReveal count clamp: got %v", buf4.Data)
	}
}
```

- [ ] **Step 2.2: Run tests — verify FAIL**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/rsbuf/ -run 'TestEncode(Loc|Obj|Map)|TestCountClamping' -v`
Expected: FAIL — `undefined: EncodeLocAddChange` etc.

- [ ] **Step 2.3: Implement `zone_encoders.go`**

Create `pkg/rsbuf/zone_encoders.go`:

```go
package rsbuf

import "github.com/zsrv/goscape/pkg/io/packet"

// Zone-nested opcode constants. Written by the zone subsystem (sub-spec 4b-3)
// as a single byte before each encoder's payload when composing the shared
// buffer delivered via UpdateZonePartialEnclosed.
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

// packLocShapeAngle returns (shape<<2)|(angle&0x3), the common second byte
// of every LOC_* zone-nested packet.
func packLocShapeAngle(shape, angle int) byte {
	return byte((shape << 2) | (angle & 0x3))
}

// clampU16 clamps a non-negative int count to the [0, 65535] wire range.
func clampU16(n int) uint16 {
	if n < 0 {
		return 0
	}
	if n > 65535 {
		return 65535
	}
	return uint16(n)
}

// --- LOC_* ---

// EncodeLocAddChange writes the 4-byte LOC_ADD_CHANGE payload.
func EncodeLocAddChange(buf *packet.Packet, coord byte, shape, angle, locID int) {
	buf.P1(coord)
	buf.P1(packLocShapeAngle(shape, angle))
	buf.P2(uint16(locID))
}

// EncodeLocAnim writes the 4-byte LOC_ANIM payload.
func EncodeLocAnim(buf *packet.Packet, coord byte, shape, angle, seq int) {
	buf.P1(coord)
	buf.P1(packLocShapeAngle(shape, angle))
	buf.P2(uint16(seq))
}

// EncodeLocDel writes the 2-byte LOC_DEL payload.
func EncodeLocDel(buf *packet.Packet, coord byte, shape, angle int) {
	buf.P1(coord)
	buf.P1(packLocShapeAngle(shape, angle))
}

// EncodeLocMerge writes the 14-byte LOC_MERGE payload for a multi-tile
// NPC standing on a spatially-merged loc. Deltas are relative to srcX/srcZ.
func EncodeLocMerge(
	buf *packet.Packet,
	coord byte,
	shape, angle, locID int,
	startCycle, endCycle, playerSlot int,
	dxEast, dzSouth, dxWest, dzNorth int,
) {
	buf.P1(coord)
	buf.P1(packLocShapeAngle(shape, angle))
	buf.P2(uint16(locID))
	buf.P2(uint16(startCycle))
	buf.P2(uint16(endCycle))
	buf.P2(uint16(playerSlot))
	buf.P1(byte(dxEast))
	buf.P1(byte(dzSouth))
	buf.P1(byte(dxWest))
	buf.P1(byte(dzNorth))
}

// --- MAP_* ---

// EncodeMapAnim writes the 6-byte MAP_ANIM payload.
func EncodeMapAnim(buf *packet.Packet, coord byte, spotanim, height, delay int) {
	buf.P1(coord)
	buf.P2(uint16(spotanim))
	buf.P1(byte(height))
	buf.P2(uint16(delay))
}

// EncodeMapProjAnim writes the 15-byte MAP_PROJANIM payload. dx/dz are the
// signed tile delta (dst - src) and must each fit in a signed i8 (|delta|<=127).
// target: 0=coord; >0=npc+1; <0=-(player slot)-1.
func EncodeMapProjAnim(
	buf *packet.Packet,
	coord byte,
	dx, dz int,
	target, spotanim int,
	srcHeight, dstHeight int,
	startDelay, endDelay int,
	peak, arc int,
) {
	buf.P1(coord)
	buf.P1(byte(dx))
	buf.P1(byte(dz))
	buf.P2(uint16(target))
	buf.P2(uint16(spotanim))
	buf.P1(byte(srcHeight))
	buf.P1(byte(dstHeight))
	buf.P2(uint16(startDelay))
	buf.P2(uint16(endDelay))
	buf.P1(byte(peak))
	buf.P1(byte(arc))
}

// --- OBJ_* ---

// EncodeObjAdd writes the 5-byte OBJ_ADD payload. count is clamped to u16.
func EncodeObjAdd(buf *packet.Packet, coord byte, obj, count int) {
	buf.P1(coord)
	buf.P2(uint16(obj))
	buf.P2(clampU16(count))
}

// EncodeObjCount writes the 7-byte OBJ_COUNT payload. Both counts clamped.
func EncodeObjCount(buf *packet.Packet, coord byte, obj, oldCount, newCount int) {
	buf.P1(coord)
	buf.P2(uint16(obj))
	buf.P2(clampU16(oldCount))
	buf.P2(clampU16(newCount))
}

// EncodeObjDel writes the 3-byte OBJ_DEL payload.
func EncodeObjDel(buf *packet.Packet, coord byte, obj int) {
	buf.P1(coord)
	buf.P2(uint16(obj))
}

// EncodeObjReveal writes the 7-byte OBJ_REVEAL payload. count is clamped;
// receiverID is the original dropper's player slot (NOT clamped — it's a u16).
func EncodeObjReveal(buf *packet.Packet, coord byte, obj, count, receiverID int) {
	buf.P1(coord)
	buf.P2(uint16(obj))
	buf.P2(clampU16(count))
	buf.P2(uint16(receiverID))
}
```

- [ ] **Step 2.4: Run tests — verify PASS**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/rsbuf/ -run 'TestEncode(Loc|Obj|Map)|TestCountClamping' -v`
Expected: 13/13 PASS.

- [ ] **Step 2.5: Commit**

```bash
git add pkg/rsbuf/zone_encoders.go pkg/rsbuf/zone_encoders_test.go
git commit --no-gpg-sign -m "feat(rsbuf): add 10 zone-nested packet encoders

EncodeLocAddChange/Anim/Del/Merge, EncodeMapAnim/ProjAnim, EncodeObjAdd/Count/
Del/Reveal. Each writes payload-only bytes (no opcode prefix); the zone
subsystem will prepend the opcode byte when composing the shared buffer.

u16 count fields are clamped to 65535 in ObjAdd/Count/Reveal."
```

---

## Task 3: Outer encoders + tests

**Files:**
- Append: `pkg/rsbuf/zone_encoders.go`
- Append: `pkg/rsbuf/zone_encoders_test.go`

- [ ] **Step 3.1: Write the failing tests**

Append to `pkg/rsbuf/zone_encoders_test.go`:

```go
// Outer-encoder header math: dx = (zoneX<<3) - ZoneOrigin(originX).
// For originX=3094: Zone(3094)=386; ZoneCenter(386)=380; ZoneOrigin=380<<3=3040.
// For zoneX=386: dx = (386<<3) - 3040 = 3088 - 3040 = 48.
// (Previously the spec guessed 16 — recompute from the actual ZoneOrigin formula.)

func TestEncodeZoneFullFollowsHeader(t *testing.T) {
	buf := packet.NewPacket(nil)
	EncodeZoneFullFollows(buf, 386, 388, 3094, 3106)
	// originX=3094 → ZoneOrigin = (((3094>>3) - 6) << 3) = ((386-6)<<3) = 380<<3 = 3040
	// dx = (386<<3) - 3040 = 3088 - 3040 = 48
	// originZ=3106 → ZoneOrigin = (((3106>>3) - 6) << 3) = ((388-6)<<3) = 382<<3 = 3056
	// dz = (388<<3) - 3056 = 3104 - 3056 = 48
	want := []byte{48, 48}
	if !bytes.Equal(buf.Data, want) {
		t.Errorf("got %v, want %v", buf.Data, want)
	}
}

func TestEncodeZonePartialFollowsHeader(t *testing.T) {
	buf := packet.NewPacket(nil)
	EncodeZonePartialFollows(buf, 386, 388, 3094, 3106)
	if len(buf.Data) != 2 {
		t.Errorf("len: got %d, want 2", len(buf.Data))
	}
	if buf.Data[0] != 48 || buf.Data[1] != 48 {
		t.Errorf("bytes: got %v, want [48 48]", buf.Data)
	}
}

func TestEncodeZonePartialEnclosedAppendsData(t *testing.T) {
	buf := packet.NewPacket(nil)
	data := []byte{0xAA, 0xBB, 0xCC}
	EncodeZonePartialEnclosed(buf, 386, 388, 3094, 3106, data)
	want := []byte{48, 48, 0xAA, 0xBB, 0xCC}
	if !bytes.Equal(buf.Data, want) {
		t.Errorf("got %v, want %v", buf.Data, want)
	}
}
```

- [ ] **Step 3.2: Run tests — verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/rsbuf/ -run TestEncodeZone -v`
Expected: FAIL — undefined `EncodeZone*`.

- [ ] **Step 3.3: Implement the outer encoders**

Append to `pkg/rsbuf/zone_encoders.go`:

```go
// --- Outer zone packets ---

// zoneRelHeader writes the 2-byte zone-relative header: the first byte is
// (zoneX<<3) - ZoneOrigin(originX) and the second is the same for z.
// ZoneOrigin produces the build-area origin's mapsquare base, so the result
// is a small signed offset that fits in one byte.
func zoneRelHeader(buf *packet.Packet, zoneX, zoneZ, originX, originZ int) {
	buf.P1(byte((zoneX << 3) - coordgrid.ZoneOrigin(originX)))
	buf.P1(byte((zoneZ << 3) - coordgrid.ZoneOrigin(originZ)))
}

// EncodeZoneFullFollows writes the 2-byte header for the outer UpdateZoneFullFollows
// packet (opcode 135, -2). The opcode and length prefix are emitted by writeOut.
func EncodeZoneFullFollows(buf *packet.Packet, zoneX, zoneZ, originX, originZ int) {
	zoneRelHeader(buf, zoneX, zoneZ, originX, originZ)
}

// EncodeZonePartialFollows writes the 2-byte header for the outer
// UpdateZonePartialFollows packet (opcode 7, -2).
func EncodeZonePartialFollows(buf *packet.Packet, zoneX, zoneZ, originX, originZ int) {
	zoneRelHeader(buf, zoneX, zoneZ, originX, originZ)
}

// EncodeZonePartialEnclosed writes the 2-byte header followed by the
// precomputed shared-data bytes for UpdateZonePartialEnclosed (opcode 162, -2).
func EncodeZonePartialEnclosed(buf *packet.Packet, zoneX, zoneZ, originX, originZ int, data []byte) {
	zoneRelHeader(buf, zoneX, zoneZ, originX, originZ)
	buf.PData(data)
}
```

Also update the imports at the top of `pkg/rsbuf/zone_encoders.go` to include `"github.com/zsrv/goscape/pkg/coordgrid"`:

```go
import (
	"github.com/zsrv/goscape/pkg/coordgrid"
	"github.com/zsrv/goscape/pkg/io/packet"
)
```

- [ ] **Step 3.4: Run tests — verify PASS**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/rsbuf/ -run TestEncodeZone -v`
Expected: 3/3 PASS.

- [ ] **Step 3.5: Full test + vet**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`
Expected: PASS across all packages.

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...`
Expected: no output.

- [ ] **Step 3.6: Commit**

```bash
git add pkg/rsbuf/zone_encoders.go pkg/rsbuf/zone_encoders_test.go
git commit --no-gpg-sign -m "feat(rsbuf): add 3 outer zone-update encoders

EncodeZoneFullFollows / PartialFollows / PartialEnclosed each write the
2-byte zone-relative header. Enclosed additionally appends the precomputed
shared data bytes."
```

---

## Final Verification

- [ ] **Step F.1: Full test suite with race detector**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./...`
Expected: PASS.

- [ ] **Step F.2: `go vet`**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...`
Expected: clean.

---

## Spec Coverage Map

| Spec requirement | Task |
|---|---|
| 3 outer opcodes (`OpUpdateZonePartialFollows/FullFollows/PartialEnclosed`) | Task 1 |
| 10 zone-nested opcode constants | Task 2 |
| `packLocShapeAngle` helper | Task 2 |
| 10 nested encoders (LOC/MAP/OBJ) | Task 2 |
| 3 outer encoders (FullFollows, PartialFollows, PartialEnclosed) | Task 3 |
| u16 count clamping in ObjAdd/Count/Reveal | Task 2 |
| Byte-level tests per encoder | Tasks 2 & 3 |
| All acceptance criteria (build, vet, test, race) | Task F |

No gaps. Every spec bullet maps to a task.
