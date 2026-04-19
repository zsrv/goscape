# PlayerInfo Encoder Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Pure-Go port of rsbuf's PlayerInfo bitstream encoder (branch `225`), with full mask-state infrastructure, so two players can see each other and `::say` produces a chat bubble visible to nearby players.

**Architecture:** New `pkg/rsbuf/` hosts the encoder + renderer + mask-payload byte writers. `pkg/grid/` indexes players by zone for nearby-player lookup. `pkg/buildarea/` gets extended with a tracked-players set + appearance-hash cache. `Player` gains 10 mask-state field groups and setter methods. A new `processInfo` tick phase runs the renderer compute pass; a new `processCleanup` phase resets masks after output. The CLIENT_CHEAT handler wires `::say` to the end-to-end demo.

**Tech Stack:** Go standard library only — `hash/fnv`, `strings`, `encoding/binary`. Reference: `github.com/2004scape/rsbuf` branch `225`.

> All `go` commands must use the prefix: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache`

---

## File Map

| File | Action |
|------|--------|
| `pkg/io/packet/alt.go` | **New** — alt byte writers (`P1Alt1/2/3`, `P2Alt2`, `P4Alt2`, `IP2`, `PDataAlt1/2`) |
| `pkg/io/packet/alt_test.go` | **New** — round-trip tests for each alt writer |
| `pkg/io/packet/packetbit_test.go` | Modify — add MSB-first audit test against known rsbuf byte sequences |
| `pkg/io/protocol/game/server/prot.go` | Modify — add `OpPlayerInfo = Op{184, -2}` |
| `pkg/grid/grid.go` | **New** — zone-grid add/remove/nearby lookup |
| `pkg/grid/grid_test.go` | **New** |
| `pkg/buildarea/buildarea.go` | Modify — add `Players map[int]struct{}`, `Appearance map[int]uint64`, `HasAppearance`, `RecordAppearance` |
| `pkg/buildarea/buildarea_test.go` | Modify — test the new fields |
| `pkg/rsbuf/visibility.go` | **New** — `Visibility` enum |
| `pkg/rsbuf/source.go` | **New** — `PlayerSource` interface |
| `pkg/rsbuf/mask_payload.go` | **New** — 9 mask-payload encoders + fixed-order writer |
| `pkg/rsbuf/mask_payload_test.go` | **New** |
| `pkg/rsbuf/renderer.go` | **New** — `Renderer` with 3-cache compute pass |
| `pkg/rsbuf/renderer_test.go` | **New** |
| `pkg/rsbuf/playerinfo.go` | **New** — 4-phase `Encode` function |
| `pkg/rsbuf/playerinfo_test.go` | **New** — golden-byte tests |
| `modules/world/player.go` | Modify — add visibility, active, 10 mask-state field groups; update `newPlayer` defaults |
| `modules/world/player_source.go` | **New** — `*Player` accessor methods satisfying `rsbuf.PlayerSource` |
| `modules/world/player_masks.go` | **New** — setter methods + `ResetMasks` |
| `modules/world/player_masks_test.go` | **New** |
| `modules/world/player_info.go` | **New** — `updatePlayers` real impl |
| `modules/world/player_info_test.go` | **New** — integration test |
| `modules/world/server.go` | Modify — add `renderer`, `grid`; initialise in `NewServer`; set `active=true/false` in `addPlayer`/`removePlayer` |
| `modules/world/tick.go` | Modify — add `processInfo` and `processCleanup` phases |
| `modules/world/handlers_game.go` | Modify — register `gameHandlers[4] = handleClientCheat` |

---

## Task 1: Alt byte writers on `pkg/io/packet`

**Files:**
- Create: `pkg/io/packet/alt.go`
- Create: `pkg/io/packet/alt_test.go`

- [ ] **Step 1: Write failing tests**

Create `pkg/io/packet/alt_test.go`:

```go
package packet

import "testing"

func TestP1Alt1(t *testing.T) {
	p := NewPacket(nil)
	p.P1Alt1(5)
	if got := p.Bytes()[0]; got != 133 {
		t.Errorf("P1Alt1(5) = %d, want 133 (5+128)", got)
	}
}

func TestP1Alt2(t *testing.T) {
	p := NewPacket(nil)
	p.P1Alt2(5)
	if got := p.Bytes()[0]; got != 123 {
		t.Errorf("P1Alt2(5) = %d, want 123 (128-5)", got)
	}
}

func TestP1Alt3(t *testing.T) {
	p := NewPacket(nil)
	p.P1Alt3(5)
	if got := p.Bytes()[0]; got != 251 {
		t.Errorf("P1Alt3(5) = %d, want 251 ((-5)&0xff)", got)
	}
}

func TestP2Alt2LittleEndian(t *testing.T) {
	p := NewPacket(nil)
	p.P2Alt2(0x1234)
	got := p.Bytes()
	if got[0] != 0x34 || got[1] != 0x12 {
		t.Errorf("P2Alt2(0x1234) = [%#x %#x], want [0x34 0x12]", got[0], got[1])
	}
}

func TestIP2IsP2Alt2(t *testing.T) {
	p := NewPacket(nil)
	p.IP2(0xABCD)
	got := p.Bytes()
	if got[0] != 0xCD || got[1] != 0xAB {
		t.Errorf("IP2(0xABCD) = [%#x %#x], want [0xCD 0xAB]", got[0], got[1])
	}
}

func TestP4Alt2MiddleEndian(t *testing.T) {
	// rsbuf branch 225 packet.rs: p4_alt2 writes bytes in order [b2, b3, b0, b1]
	// for value 0xAABBCCDD that's [0xBB, 0xAA, 0xDD, 0xCC].
	p := NewPacket(nil)
	p.P4Alt2(0xAABBCCDD)
	got := p.Bytes()
	want := []byte{0xBB, 0xAA, 0xDD, 0xCC}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("P4Alt2(0xAABBCCDD)[%d] = %#x, want %#x", i, got[i], want[i])
		}
	}
}

func TestPDataAlt1(t *testing.T) {
	p := NewPacket(nil)
	p.PDataAlt1([]byte{1, 2, 3})
	got := p.Bytes()
	for i, want := range []byte{129, 130, 131} {
		if got[i] != want {
			t.Errorf("PDataAlt1[%d] = %d, want %d", i, got[i], want)
		}
	}
}

func TestPDataAlt2(t *testing.T) {
	p := NewPacket(nil)
	p.PDataAlt2([]byte{1, 2, 3})
	got := p.Bytes()
	for i, want := range []byte{127, 126, 125} {
		if got[i] != want {
			t.Errorf("PDataAlt2[%d] = %d, want %d", i, got[i], want)
		}
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/io/packet/... -run "TestP1Alt|TestP2Alt|TestIP2|TestP4Alt|TestPData" 2>&1 | head -5
```

Expected: compile errors — methods undefined.

- [ ] **Step 3: Create `pkg/io/packet/alt.go`**

```go
package packet

// Alt byte writers — Jagex's scrambled protocol variants.
// Reference: github.com/2004scape/rsbuf branch 225, src/packet.rs.

func (p *Packet) P1Alt1(v uint8) { p.P1(v + 128) }
func (p *Packet) P1Alt2(v uint8) { p.P1(128 - v) }
func (p *Packet) P1Alt3(v uint8) { p.P1(uint8(-int8(v))) }

// P2Alt2 writes a u16 in little-endian order.
func (p *Packet) P2Alt2(v uint16) {
	p.P1(uint8(v))
	p.P1(uint8(v >> 8))
}

// IP2 is an alias for P2Alt2 (inverse-endian u16).
func (p *Packet) IP2(v uint16) { p.P2Alt2(v) }

// P4Alt2 writes a u32 in middle-endian order: bytes 2, 3, 0, 1.
func (p *Packet) P4Alt2(v uint32) {
	p.P1(uint8(v >> 16))
	p.P1(uint8(v >> 24))
	p.P1(uint8(v))
	p.P1(uint8(v >> 8))
}

// PDataAlt1 writes data with each byte offset by +128.
func (p *Packet) PDataAlt1(b []byte) {
	for _, x := range b {
		p.P1(x + 128)
	}
}

// PDataAlt2 writes data with each byte transformed as (128 - b).
func (p *Packet) PDataAlt2(b []byte) {
	for _, x := range b {
		p.P1(128 - x)
	}
}
```

- [ ] **Step 4: Run tests**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/io/packet/... -v 2>&1 | tail -10
```

Expected: all pass.

- [ ] **Step 5: Commit**

```bash
cd /home/owner/Code/github.com/zsrv/goscape
git add pkg/io/packet/alt.go pkg/io/packet/alt_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(packet): add alt byte writers for scrambled RS2 protocol variants

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: PacketBit MSB-first audit

**Files:**
- Modify: `pkg/io/packet/packetbit_test.go` (add new test)

- [ ] **Step 1: Add the audit test**

Append to `pkg/io/packet/packetbit_test.go` (or create if absent):

```go
func TestPacketBitMSBFirst(t *testing.T) {
	// rsbuf's pbit writes MSB-first. Verify known sequences.
	// Sequence: pbit(3, 5), pbit(11, 1500), pbit(1, 0)
	// Binary: 101 10111011100 0  = 101101110111000 (15 bits)
	// Padded to 16 bits: 1011011101110000
	// Bytes: 0xB7 0x70
	pb := NewPacketBit(NewPacket(nil))
	pb.PBit(3, 5)
	pb.PBit(11, 1500)
	pb.PBit(1, 0)
	// Pad to byte boundary by flushing.
	pb.Finish()
	got := pb.Bytes()
	if len(got) != 2 || got[0] != 0xB7 || got[1] != 0x70 {
		t.Errorf("PBit MSB-first: got %v, want [0xB7 0x70]", got)
	}
}
```

- [ ] **Step 2: Run the audit**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/io/packet/... -run TestPacketBitMSBFirst -v 2>&1 | tail -10
```

Two possible outcomes:

**A) Test passes:** The existing PacketBit is already MSB-first. No changes needed. Proceed to step 4.

**B) Test fails OR PacketBit API differs:** Inspect `pkg/io/packet/packetbit.go` and either:
- Adjust the test to use the actual API (e.g., if the method is `WriteBits` not `PBit`, rename)
- If the bit ordering is actually LSB-first (the byte values are wrong), modify `packetbit.go` to write MSB-first. The fix typically looks like:

```go
func (pb *PacketBit) PBit(numBits int, value int) {
	// Walk bits from high to low.
	for i := numBits - 1; i >= 0; i-- {
		bitValue := (value >> i) & 1
		byteIdx := pb.bitPos >> 3
		bitIdx := 7 - (pb.bitPos & 7)
		if len(pb.buf) <= byteIdx {
			pb.buf = append(pb.buf, 0)
		}
		pb.buf[byteIdx] |= byte(bitValue << bitIdx)
		pb.bitPos++
	}
}
```

- Also confirm `Finish()` (or equivalent) pads to the next byte boundary.

- [ ] **Step 3: Iterate until test passes**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/io/packet/... -v 2>&1 | tail -10
```

Expected: all pass including TestPacketBitMSBFirst. Re-run all existing PacketBit tests to ensure no regressions.

- [ ] **Step 4: Commit**

```bash
cd /home/owner/Code/github.com/zsrv/goscape
git add pkg/io/packet/
git commit --no-gpg-sign -m "$(cat <<'EOF'
test(packet): audit PacketBit MSB-first compatibility with rsbuf pbit

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: Add `OpPlayerInfo` opcode

**Files:**
- Modify: `pkg/io/protocol/game/server/prot.go`
- Modify: `pkg/io/protocol/game/server/prot_test.go`

- [ ] **Step 1: Add the opcode**

In `pkg/io/protocol/game/server/prot.go`, append to the existing `var ( ... )` block:

```go
OpPlayerInfo = Op{Opcode: 184, PayloadSize: -2}
```

- [ ] **Step 2: Add test**

Append to `pkg/io/protocol/game/server/prot_test.go`:

```go
func TestSubSpec3BOpcodes(t *testing.T) {
	if OpPlayerInfo.Opcode != 184 {
		t.Errorf("OpPlayerInfo.Opcode = %d, want 184", OpPlayerInfo.Opcode)
	}
	if OpPlayerInfo.PayloadSize != -2 {
		t.Errorf("OpPlayerInfo.PayloadSize = %d, want -2", OpPlayerInfo.PayloadSize)
	}
}
```

- [ ] **Step 3: Run tests**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/io/protocol/game/server/... -v
```

- [ ] **Step 4: Commit**

```bash
git add pkg/io/protocol/game/server/
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(prot): add OpPlayerInfo server opcode

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: Create `pkg/grid` for zone-indexed player lookup

**Files:**
- Create: `pkg/grid/grid.go`
- Create: `pkg/grid/grid_test.go`

- [ ] **Step 1: Write failing tests**

Create `pkg/grid/grid_test.go`:

```go
package grid

import "testing"

func TestAddAndNearbyReturnsPlayer(t *testing.T) {
	g := New()
	g.Add(5, 3094, 3106, 0)

	near := g.NearbyPlayers(3094, 3106, 0, 1)
	if len(near) != 1 || near[0] != 5 {
		t.Errorf("NearbyPlayers: got %v, want [5]", near)
	}
}

func TestRemoveRemovesPlayer(t *testing.T) {
	g := New()
	g.Add(5, 3094, 3106, 0)
	g.Remove(5, 3094, 3106, 0)

	near := g.NearbyPlayers(3094, 3106, 0, 1)
	if len(near) != 0 {
		t.Errorf("after remove: got %v, want empty", near)
	}
}

func TestLevelFilter(t *testing.T) {
	g := New()
	g.Add(5, 3094, 3106, 0)
	g.Add(6, 3094, 3106, 1)

	level0 := g.NearbyPlayers(3094, 3106, 0, 1)
	if len(level0) != 1 || level0[0] != 5 {
		t.Errorf("level 0: got %v, want [5]", level0)
	}
	level1 := g.NearbyPlayers(3094, 3106, 1, 1)
	if len(level1) != 1 || level1[0] != 6 {
		t.Errorf("level 1: got %v, want [6]", level1)
	}
}

func TestRadiusBoundary(t *testing.T) {
	g := New()
	// Add player exactly 3 zones east.
	g.Add(5, 3094+24, 3106, 0) // 24 tiles = 3 zones

	in := g.NearbyPlayers(3094, 3106, 0, 3)
	if len(in) != 1 {
		t.Errorf("radius 3 should include: got %v", in)
	}
	out := g.NearbyPlayers(3094, 3106, 0, 2)
	if len(out) != 0 {
		t.Errorf("radius 2 should exclude: got %v", out)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/grid/... 2>&1 | head -5
```

- [ ] **Step 3: Create `pkg/grid/grid.go`**

```go
package grid

// Grid indexes players by zone (8x8 tile squares) for nearby-player lookup.
type Grid struct {
	zones map[uint32][]int // packed zone key -> player slots
}

// New returns an empty grid.
func New() *Grid {
	return &Grid{zones: map[uint32][]int{}}
}

// packZone packs (zoneX, zoneZ, level) into a single uint32 key.
// Layout: (level & 0x3) << 22 | (zoneX & 0x7FF) << 11 | (zoneZ & 0x7FF)
func packZone(x, z, level int) uint32 {
	zoneX := (x >> 3) & 0x7FF
	zoneZ := (z >> 3) & 0x7FF
	return (uint32(level)&0x3)<<22 | uint32(zoneX)<<11 | uint32(zoneZ)
}

// Add records a player at the given coordinate.
func (g *Grid) Add(slot, x, z, level int) {
	key := packZone(x, z, level)
	g.zones[key] = append(g.zones[key], slot)
}

// Remove un-records a player from the given coordinate.
func (g *Grid) Remove(slot, x, z, level int) {
	key := packZone(x, z, level)
	slots := g.zones[key]
	for i, s := range slots {
		if s == slot {
			g.zones[key] = append(slots[:i], slots[i+1:]...)
			if len(g.zones[key]) == 0 {
				delete(g.zones, key)
			}
			return
		}
	}
}

// NearbyPlayers returns all player slots within zoneRadius zones (Chebyshev
// distance) of (x, z, level). The level must match exactly.
func (g *Grid) NearbyPlayers(x, z, level, zoneRadius int) []int {
	zoneX := x >> 3
	zoneZ := z >> 3
	out := []int{}
	for dx := -zoneRadius; dx <= zoneRadius; dx++ {
		for dz := -zoneRadius; dz <= zoneRadius; dz++ {
			zx := zoneX + dx
			zz := zoneZ + dz
			if zx < 0 || zz < 0 {
				continue
			}
			key := packZone(zx<<3, zz<<3, level)
			out = append(out, g.zones[key]...)
		}
	}
	return out
}
```

- [ ] **Step 4: Run tests**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/grid/... -v
```

Expected: all 4 tests pass.

- [ ] **Step 5: Commit**

```bash
git add pkg/grid/
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(grid): add zone-indexed player lookup

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 5: Extend `pkg/buildarea` with Players + Appearance cache

**Files:**
- Modify: `pkg/buildarea/buildarea.go`
- Modify: `pkg/buildarea/buildarea_test.go`

- [ ] **Step 1: Add tests**

Append to `pkg/buildarea/buildarea_test.go`:

```go
func TestPlayersSetAddRemove(t *testing.T) {
	ba := New()
	if _, ok := ba.Players[5]; ok {
		t.Error("new BuildArea should have empty Players")
	}
	ba.Players[5] = struct{}{}
	if _, ok := ba.Players[5]; !ok {
		t.Error("add should succeed")
	}
	delete(ba.Players, 5)
	if _, ok := ba.Players[5]; ok {
		t.Error("remove should succeed")
	}
}

func TestAppearanceHasRecord(t *testing.T) {
	ba := New()
	if ba.HasAppearance(5, 0x12345) {
		t.Error("fresh BuildArea should not have appearance cached")
	}
	ba.RecordAppearance(5, 0x12345)
	if !ba.HasAppearance(5, 0x12345) {
		t.Error("RecordAppearance did not stick")
	}
	if ba.HasAppearance(5, 0xdeadbeef) {
		t.Error("different hash should miss")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/buildarea/... -run "TestPlayers|TestAppearance" 2>&1 | head -5
```

- [ ] **Step 3: Extend the struct**

In `pkg/buildarea/buildarea.go`, add fields to `BuildArea`:

```go
Players    map[int]struct{} // player slots currently tracked by this client
Appearance map[int]uint64   // slot -> hash of last APPEARANCE bytes sent
```

Update `New()`:

```go
func New() *BuildArea {
	return &BuildArea{
		OriginX:     -1,
		OriginZ:     -1,
		LoadedZones: map[int]bool{},
		ActiveZones: map[int]bool{},
		Mapsquares:  map[uint16]bool{},
		Players:     map[int]struct{}{},
		Appearance:  map[int]uint64{},
	}
}
```

Add methods:

```go
// HasAppearance reports whether the given appearance hash was already sent
// for the given slot.
func (ba *BuildArea) HasAppearance(slot int, hash uint64) bool {
	stored, ok := ba.Appearance[slot]
	return ok && stored == hash
}

// RecordAppearance remembers the appearance hash that was just sent for the slot.
func (ba *BuildArea) RecordAppearance(slot int, hash uint64) {
	ba.Appearance[slot] = hash
}
```

- [ ] **Step 4: Run tests**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/buildarea/... -v
```

- [ ] **Step 5: Commit**

```bash
git add pkg/buildarea/
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(buildarea): add tracked-players set and appearance-hash cache

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 6: Create `pkg/rsbuf` skeleton — Visibility + PlayerSource

**Files:**
- Create: `pkg/rsbuf/visibility.go`
- Create: `pkg/rsbuf/source.go`

- [ ] **Step 1: Create `pkg/rsbuf/visibility.go`**

```go
// Package rsbuf is a pure-Go port of the @2004scape/rsbuf Rust crate's
// PlayerInfo bitstream encoder. Reference: github.com/2004scape/rsbuf branch 225.
package rsbuf

// Visibility controls who can see a player.
type Visibility int

const (
	VisibilityDefault Visibility = iota // normal: everyone sees
	VisibilitySoft                      // only staff sees (admin invisibility)
	VisibilityHard                      // nobody sees (hidden-online, invis-to-all)
)
```

- [ ] **Step 2: Create `pkg/rsbuf/source.go`**

```go
package rsbuf

// PlayerSource exposes a player's state to the encoder without a dependency on
// the modules/world Player type.
type PlayerSource interface {
	// identity + lifecycle
	Slot() int
	Coords() (x, z, level int)
	Active() bool
	Visibility() Visibility
	StaffModLevel() int32

	// masks
	Masks() int
	EntityMask() int

	// appearance
	AppearanceBytes() []byte
	AppearanceHash() uint64

	// mask payload accessors
	AnimID() int
	AnimDelay() int
	FaceEntity() int
	SayText() []byte
	DamageAmt() int
	DamageType() int
	CurHP() int
	BaseHP() int
	FaceSquareX() int
	FaceSquareZ() int
	ChatColour() int
	ChatEffect() int
	ChatRights() int
	ChatBytes() []byte
	SpotAnimID() int
	SpotAnimHeight() int
	SpotAnimDelay() int
	ExactStartX() int
	ExactStartZ() int
	ExactEndX() int
	ExactEndZ() int
	ExactBegin() int
	ExactFinish() int
	ExactDir() int

	// movement
	WalkDir() int
	RunDir() int
	Tele() bool
	Jump() bool
	LastTickX() int
	LastTickZ() int
	LastLevel() int
	OriginX() int
	OriginZ() int
}

// Mask bit constants — matches rsbuf branch 225 PlayerInfoProt.
const (
	MaskAppearance = 0x1
	MaskAnim       = 0x2
	MaskFaceEntity = 0x4
	MaskSay        = 0x8
	MaskDamage     = 0x10
	MaskFaceCoord  = 0x20
	MaskChat       = 0x40
	MaskBig        = 0x80
	MaskSpotAnim   = 0x100
	MaskExactMove  = 0x200
)
```

- [ ] **Step 3: Build**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./pkg/rsbuf/...
```

Expected: no errors.

- [ ] **Step 4: Commit**

```bash
git add pkg/rsbuf/
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(rsbuf): add Visibility enum and PlayerSource interface

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 7: Add mask-state fields to `Player` + defaults

**Files:**
- Modify: `modules/world/player.go`

- [ ] **Step 1: Add fields**

In `modules/world/player.go`, inside the `Player` struct definition (after the existing "BAS" section from sub-spec 3a), append:

```go
// === visibility + active flag (sub-spec 3b) ===
visibility rsbuf.Visibility
active     bool

// === mask state (sub-spec 3b) ===
animID, animDelay int

sayText []byte

chatColour, chatEffect, chatRights int
chatBytes []byte

damageAmt, damageType int
curHP, baseHP         int

spotanimID, spotanimHeight, spotanimDelay int

exactStartX, exactStartZ, exactEndX, exactEndZ int
exactBegin, exactFinish, exactDir              int

faceEntity               int
faceSquareX, faceSquareZ int
```

Add import:

```go
"github.com/zsrv/goscape/pkg/rsbuf"
```

- [ ] **Step 2: Update `newPlayer` defaults**

Inside the `&Player{ ... }` literal in `newPlayer`, add these initializers alongside the existing ones:

```go
visibility:     rsbuf.VisibilityDefault,
active:         false,
animID:         -1,
animDelay:      -1,
chatColour:     -1,
chatEffect:     -1,
chatRights:     -1,
damageAmt:      -1,
damageType:     -1,
curHP:          -1,
baseHP:         -1,
spotanimID:     -1,
spotanimHeight: -1,
spotanimDelay:  -1,
exactStartX:    -1,
exactStartZ:    -1,
exactEndX:      -1,
exactEndZ:      -1,
exactBegin:     -1,
exactFinish:    -1,
exactDir:       -1,
faceEntity:     -1,
faceSquareX:    -1,
faceSquareZ:    -1,
```

- [ ] **Step 3: Build**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... 2>&1 | tail -5
```

Expected: all existing tests still pass.

- [ ] **Step 4: Commit**

```bash
git add modules/world/player.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): extend Player with mask-state fields and visibility flag

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 8: `Player` setter methods (player_masks.go)

**Files:**
- Create: `modules/world/player_masks.go`
- Create: `modules/world/player_masks_test.go`

- [ ] **Step 1: Write failing tests**

Create `modules/world/player_masks_test.go`:

```go
package world

import (
	"testing"

	"github.com/zsrv/goscape/pkg/rsbuf"
)

func TestAnimateSetsMask(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.Animate(123, 5)
	if p.masks&rsbuf.MaskAnim == 0 {
		t.Error("MaskAnim bit should be set")
	}
	if p.animID != 123 || p.animDelay != 5 {
		t.Errorf("animID/Delay: got (%d,%d), want (123,5)", p.animID, p.animDelay)
	}
}

func TestSaySetsMask(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.Say([]byte("hi"))
	if p.masks&rsbuf.MaskSay == 0 {
		t.Error("MaskSay bit should be set")
	}
	if string(p.sayText) != "hi" {
		t.Errorf("sayText: got %q, want %q", p.sayText, "hi")
	}
}

func TestChatSetsMask(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.Chat(1, 2, 3, []byte("yo"))
	if p.masks&rsbuf.MaskChat == 0 {
		t.Error("MaskChat bit should be set")
	}
	if p.chatColour != 1 || p.chatEffect != 2 || p.chatRights != 3 {
		t.Errorf("chat flags: got (%d,%d,%d), want (1,2,3)", p.chatColour, p.chatEffect, p.chatRights)
	}
}

func TestShowHitSetsMask(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.ShowHit(10, 1, 40, 50)
	if p.masks&rsbuf.MaskDamage == 0 {
		t.Error("MaskDamage bit should be set")
	}
	if p.damageAmt != 10 || p.curHP != 40 || p.baseHP != 50 {
		t.Errorf("damage fields: %+v", p)
	}
}

func TestFaceCoordMultipliesBy2Plus1(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.FaceCoord(100, 200)
	if p.faceSquareX != 201 || p.faceSquareZ != 401 {
		t.Errorf("faceSquare: got (%d,%d), want (201,401) = (100*2+1, 200*2+1)", p.faceSquareX, p.faceSquareZ)
	}
}

func TestFaceEntity(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.FaceEntity(0x8005)
	if p.masks&rsbuf.MaskFaceEntity == 0 {
		t.Error("MaskFaceEntity bit should be set")
	}
	if p.faceEntity != 0x8005 {
		t.Errorf("faceEntity: got %d, want 0x8005", p.faceEntity)
	}
}

func TestResetMasksClearsEphemerals(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.Say([]byte("hi"))
	p.Animate(123, 5)
	p.ShowHit(10, 1, 40, 50)
	p.ResetMasks()
	if p.masks != 0 {
		t.Errorf("masks: got %d, want 0", p.masks)
	}
	if p.sayText != nil {
		t.Error("sayText should be nil after reset")
	}
	if p.damageAmt != -1 {
		t.Errorf("damageAmt: got %d, want -1", p.damageAmt)
	}
	// Persistent (animID, faceEntity) should stay.
	if p.animID != 123 {
		t.Errorf("animID should persist: got %d", p.animID)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run "TestAnimate|TestSay|TestChat|TestShowHit|TestFaceCoord|TestFaceEntity|TestResetMasks" 2>&1 | head -10
```

- [ ] **Step 3: Create `modules/world/player_masks.go`**

```go
package world

import "github.com/zsrv/goscape/pkg/rsbuf"

// Animate sets the player's current animation + delay (triggers MaskAnim).
func (p *Player) Animate(id, delay int) {
	p.animID = id
	p.animDelay = delay
	p.masks |= rsbuf.MaskAnim
}

// Say raises a public speech bubble above the player's head (MaskSay).
func (p *Player) Say(msg []byte) {
	p.sayText = msg
	p.masks |= rsbuf.MaskSay
}

// Chat posts a public-chat message with colour/effect/rights flags (MaskChat).
func (p *Player) Chat(colour, effect, rights int, msg []byte) {
	p.chatColour = colour
	p.chatEffect = effect
	p.chatRights = rights
	p.chatBytes = msg
	p.masks |= rsbuf.MaskChat
}

// ShowHit displays a hitmark (MaskDamage).
func (p *Player) ShowHit(amount, dmgType, cur, base int) {
	p.damageAmt = amount
	p.damageType = dmgType
	p.curHP = cur
	p.baseHP = base
	p.masks |= rsbuf.MaskDamage
}

// SpotAnim attaches a spot animation (MaskSpotAnim).
func (p *Player) SpotAnim(id, height, delay int) {
	p.spotanimID = id
	p.spotanimHeight = height
	p.spotanimDelay = delay
	p.masks |= rsbuf.MaskSpotAnim
}

// ExactMove triggers a precise tile-to-tile move animation (MaskExactMove).
// Coordinates are absolute; encoder converts to local at write time.
func (p *Player) ExactMove(sX, sZ, eX, eZ, begin, finish, dir int) {
	p.exactStartX = sX
	p.exactStartZ = sZ
	p.exactEndX = eX
	p.exactEndZ = eZ
	p.exactBegin = begin
	p.exactFinish = finish
	p.exactDir = dir
	p.masks |= rsbuf.MaskExactMove
}

// FaceCoord makes the player face a tile (MaskFaceCoord).
// Stored as fine-grained coords: tile*2 + 1.
func (p *Player) FaceCoord(x, z int) {
	p.faceSquareX = x*2 + 1
	p.faceSquareZ = z*2 + 1
	p.masks |= rsbuf.MaskFaceCoord
}

// FaceEntity makes the player face another entity (MaskFaceEntity).
// entityIndex: for players, slot + 0x8000; for NPCs, slot.
func (p *Player) FaceEntity(entityIndex int) {
	p.faceEntity = entityIndex
	p.masks |= rsbuf.MaskFaceEntity
}

// ResetMasks clears mask bits and ephemeral state for the next tick.
// Persistent fields (animID, faceEntity, faceSquareX/Z) are retained so newly
// added observers still see them; the mask bit is re-raised only when the
// state changes (caller responsibility).
func (p *Player) ResetMasks() {
	p.masks = 0
	p.sayText = nil
	p.chatBytes = nil
	p.damageAmt = -1
	p.damageType = -1
	p.curHP = -1
	p.baseHP = -1
	p.spotanimID = -1
	p.spotanimHeight = -1
	p.spotanimDelay = -1
	p.exactStartX = -1
	p.exactStartZ = -1
	p.exactEndX = -1
	p.exactEndZ = -1
	p.exactBegin = -1
	p.exactFinish = -1
	p.exactDir = -1
}
```

- [ ] **Step 4: Run tests**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run "TestAnimate|TestSay|TestChat|TestShowHit|TestFaceCoord|TestFaceEntity|TestResetMasks" -v
```

Expected: all 7 tests pass.

- [ ] **Step 5: Commit**

```bash
git add modules/world/player_masks.go modules/world/player_masks_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): add Player mask setter methods and ResetMasks

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 9: `Player` accessors satisfying `rsbuf.PlayerSource`

**Files:**
- Create: `modules/world/player_source.go`

- [ ] **Step 1: Create the file**

```go
package world

import (
	"hash/fnv"

	"github.com/zsrv/goscape/pkg/rsbuf"
)

// Slot returns the player's RS2 slot. (Also used by the entity interface from sub-spec 2.)
// NOTE: if Slot is already defined in player.go from sub-spec 2, DO NOT redefine it here.

// Coords returns absolute (x, z, level). (Already defined in sub-spec 2 too.)

// Active reports whether the player is currently participating in the world
// (true between completion of processLogins and beginning of processLogouts).
func (p *Player) Active() bool { return p.active }

// Visibility returns the player's current visibility mode.
func (p *Player) Visibility() rsbuf.Visibility { return p.visibility }

// StaffModLevel returns the player's staff/moderator level (0 = normal player).
func (p *Player) StaffModLevel() int32 { return p.staffModLevel }

// Masks returns the current mask bitmask.
func (p *Player) Masks() int { return p.masks }

// EntityMask returns the always-on entity bitmask (faceCoord|faceEntity baseline).
func (p *Player) EntityMask() int { return p.entitymask }

// AppearanceBytes returns the raw appearance byte buffer.
func (p *Player) AppearanceBytes() []byte { return p.appearanceBuf }

// AppearanceHash returns a cheap hash identifying the current appearance bytes,
// used to avoid re-sending APPEARANCE to observers who already have it cached.
func (p *Player) AppearanceHash() uint64 {
	h := fnv.New64a()
	h.Write(p.appearanceBuf)
	return h.Sum64()
}

// Mask payload accessors.
func (p *Player) AnimID() int        { return p.animID }
func (p *Player) AnimDelay() int     { return p.animDelay }
func (p *Player) FaceEntity() int    { return p.faceEntity }
func (p *Player) SayText() []byte    { return p.sayText }
func (p *Player) DamageAmt() int     { return p.damageAmt }
func (p *Player) DamageType() int    { return p.damageType }
func (p *Player) CurHP() int         { return p.curHP }
func (p *Player) BaseHP() int        { return p.baseHP }
func (p *Player) FaceSquareX() int   { return p.faceSquareX }
func (p *Player) FaceSquareZ() int   { return p.faceSquareZ }
func (p *Player) ChatColour() int    { return p.chatColour }
func (p *Player) ChatEffect() int    { return p.chatEffect }
func (p *Player) ChatRights() int    { return p.chatRights }
func (p *Player) ChatBytes() []byte  { return p.chatBytes }
func (p *Player) SpotAnimID() int    { return p.spotanimID }
func (p *Player) SpotAnimHeight() int { return p.spotanimHeight }
func (p *Player) SpotAnimDelay() int { return p.spotanimDelay }
func (p *Player) ExactStartX() int   { return p.exactStartX }
func (p *Player) ExactStartZ() int   { return p.exactStartZ }
func (p *Player) ExactEndX() int     { return p.exactEndX }
func (p *Player) ExactEndZ() int     { return p.exactEndZ }
func (p *Player) ExactBegin() int    { return p.exactBegin }
func (p *Player) ExactFinish() int   { return p.exactFinish }
func (p *Player) ExactDir() int      { return p.exactDir }

// Movement accessors.
func (p *Player) WalkDir() int   { return p.walkDir }
func (p *Player) RunDir() int    { return p.runDir }
func (p *Player) Tele() bool     { return p.tele }
func (p *Player) Jump() bool     { return p.jump }
func (p *Player) LastTickX() int { return p.lastTickX }
func (p *Player) LastTickZ() int { return p.lastTickZ }
func (p *Player) LastLevel() int { return p.lastLevel }
func (p *Player) OriginX() int   { return p.originX }
func (p *Player) OriginZ() int   { return p.originZ }
```

> **Note:** if sub-spec 2 already defined `Slot()` and `Coords()` on `*Player`, skip those two methods here. Check `modules/world/player.go` for existing methods and omit duplicates.

- [ ] **Step 2: Compile-time check**

Add a sentinel at the bottom of `player_source.go`:

```go
// Compile-time check that *Player satisfies rsbuf.PlayerSource.
var _ rsbuf.PlayerSource = (*Player)(nil)
```

- [ ] **Step 3: Build**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./modules/world/...
```

If compile fails with "missing method X", add it.

- [ ] **Step 4: Commit**

```bash
git add modules/world/player_source.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): add Player accessors satisfying rsbuf.PlayerSource

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 10: Mask payload encoders

**Files:**
- Create: `pkg/rsbuf/mask_payload.go`
- Create: `pkg/rsbuf/mask_payload_test.go`

- [ ] **Step 1: Write failing tests**

Create `pkg/rsbuf/mask_payload_test.go`:

```go
package rsbuf

import (
	"testing"

	"github.com/zsrv/goscape/pkg/io/packet"
)

// fakeSource is a minimal PlayerSource for byte-layout tests.
type fakeSource struct {
	slot                                     int
	masks, entityMask                        int
	appearance                               []byte
	animID, animDelay                        int
	faceEntity                               int
	sayText                                  []byte
	damageAmt, damageType, curHP, baseHP     int
	faceSquareX, faceSquareZ                 int
	chatColour, chatEffect, chatRights       int
	chatBytes                                []byte
	spotanimID, spotanimHeight, spotanimDelay int
	exactStartX, exactStartZ                 int
	exactEndX, exactEndZ                     int
	exactBegin, exactFinish, exactDir        int
	originX, originZ                         int
	x, z, level                              int
}

func (f *fakeSource) Slot() int              { return f.slot }
func (f *fakeSource) Coords() (int, int, int) { return f.x, f.z, f.level }
func (f *fakeSource) Active() bool           { return true }
func (f *fakeSource) Visibility() Visibility { return VisibilityDefault }
func (f *fakeSource) StaffModLevel() int32   { return 0 }
func (f *fakeSource) Masks() int             { return f.masks }
func (f *fakeSource) EntityMask() int        { return f.entityMask }
func (f *fakeSource) AppearanceBytes() []byte { return f.appearance }
func (f *fakeSource) AppearanceHash() uint64  { return 1 }
func (f *fakeSource) AnimID() int            { return f.animID }
func (f *fakeSource) AnimDelay() int         { return f.animDelay }
func (f *fakeSource) FaceEntity() int        { return f.faceEntity }
func (f *fakeSource) SayText() []byte        { return f.sayText }
func (f *fakeSource) DamageAmt() int         { return f.damageAmt }
func (f *fakeSource) DamageType() int        { return f.damageType }
func (f *fakeSource) CurHP() int             { return f.curHP }
func (f *fakeSource) BaseHP() int            { return f.baseHP }
func (f *fakeSource) FaceSquareX() int       { return f.faceSquareX }
func (f *fakeSource) FaceSquareZ() int       { return f.faceSquareZ }
func (f *fakeSource) ChatColour() int        { return f.chatColour }
func (f *fakeSource) ChatEffect() int        { return f.chatEffect }
func (f *fakeSource) ChatRights() int        { return f.chatRights }
func (f *fakeSource) ChatBytes() []byte      { return f.chatBytes }
func (f *fakeSource) SpotAnimID() int        { return f.spotanimID }
func (f *fakeSource) SpotAnimHeight() int    { return f.spotanimHeight }
func (f *fakeSource) SpotAnimDelay() int     { return f.spotanimDelay }
func (f *fakeSource) ExactStartX() int       { return f.exactStartX }
func (f *fakeSource) ExactStartZ() int       { return f.exactStartZ }
func (f *fakeSource) ExactEndX() int         { return f.exactEndX }
func (f *fakeSource) ExactEndZ() int         { return f.exactEndZ }
func (f *fakeSource) ExactBegin() int        { return f.exactBegin }
func (f *fakeSource) ExactFinish() int       { return f.exactFinish }
func (f *fakeSource) ExactDir() int          { return f.exactDir }
func (f *fakeSource) WalkDir() int           { return -1 }
func (f *fakeSource) RunDir() int            { return -1 }
func (f *fakeSource) Tele() bool             { return false }
func (f *fakeSource) Jump() bool             { return false }
func (f *fakeSource) LastTickX() int         { return -1 }
func (f *fakeSource) LastTickZ() int         { return -1 }
func (f *fakeSource) LastLevel() int         { return -1 }
func (f *fakeSource) OriginX() int           { return f.originX }
func (f *fakeSource) OriginZ() int           { return f.originZ }

func TestAnimPayload(t *testing.T) {
	p := &fakeSource{masks: MaskAnim, animID: 0x1234, animDelay: 5}
	buf := packet.NewPacket(nil)
	writeMaskPayloads(buf, p, MaskAnim, false)
	got := buf.Bytes()
	// ANIM: p2(0x1234) p1_alt3(5) = [0x12, 0x34, (0xff & -5) = 0xfb]
	want := []byte{0x12, 0x34, 0xfb}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("byte[%d]: got %#x, want %#x (full=%v)", i, got[i], want[i], got)
			break
		}
	}
}

func TestFaceCoordPayload(t *testing.T) {
	p := &fakeSource{masks: MaskFaceCoord, faceSquareX: 0x0182, faceSquareZ: 0x0184}
	buf := packet.NewPacket(nil)
	writeMaskPayloads(buf, p, MaskFaceCoord, false)
	got := buf.Bytes()
	// FACE_COORD: p2(x) p2(z) = [0x01, 0x82, 0x01, 0x84]
	want := []byte{0x01, 0x82, 0x01, 0x84}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("byte[%d]: got %#x, want %#x", i, got[i], want[i])
		}
	}
}

func TestAppearancePayload(t *testing.T) {
	p := &fakeSource{masks: MaskAppearance, appearance: []byte{1, 2, 3}}
	buf := packet.NewPacket(nil)
	writeMaskPayloads(buf, p, MaskAppearance, false)
	got := buf.Bytes()
	// APPEARANCE: p1(len=3) pdata_alt1([1,2,3]) = [3, 129, 130, 131]
	want := []byte{3, 129, 130, 131}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("byte[%d]: got %#x, want %#x", i, got[i], want[i])
		}
	}
}

func TestChatPayload(t *testing.T) {
	p := &fakeSource{masks: MaskChat, chatColour: 1, chatEffect: 2, chatRights: 3, chatBytes: []byte("yo")}
	buf := packet.NewPacket(nil)
	writeMaskPayloads(buf, p, MaskChat, false)
	got := buf.Bytes()
	// CHAT: p1(1) p1(2) p1_alt2(3)=125 p1_alt1(len=2)=130 pdata_alt2("yo") = [128-'y'=...]
	// 'y'=0x79=121, 128-121=7; 'o'=0x6f=111, 128-111=17
	want := []byte{1, 2, 125, 130, 7, 17}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("byte[%d]: got %#x, want %#x (full=%v)", i, got[i], want[i], got)
			break
		}
	}
}

func TestDamagePayload(t *testing.T) {
	p := &fakeSource{masks: MaskDamage, damageAmt: 10, damageType: 1, curHP: 40, baseHP: 50}
	buf := packet.NewPacket(nil)
	writeMaskPayloads(buf, p, MaskDamage, false)
	got := buf.Bytes()
	// DAMAGE: p1_alt1(10)=138 p1_alt3(1)=255 p1_alt2(40)=88 p1(50)
	want := []byte{138, 255, 88, 50}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("byte[%d]: got %#x, want %#x", i, got[i], want[i])
		}
	}
}

func TestMaskHeaderSmall(t *testing.T) {
	buf := packet.NewPacket(nil)
	writeMaskHeader(buf, MaskAnim|MaskFaceCoord) // 2|32 = 34
	if buf.Bytes()[0] != 34 {
		t.Errorf("header byte: got %d, want 34", buf.Bytes()[0])
	}
}

func TestMaskHeaderLarge(t *testing.T) {
	buf := packet.NewPacket(nil)
	writeMaskHeader(buf, MaskAnim|MaskSpotAnim) // 2|256 = 258
	// Should write IP2(258|128) = IP2(386) little-endian = [0x82, 0x01]
	got := buf.Bytes()
	if got[0] != 0x82 || got[1] != 0x01 {
		t.Errorf("large header: got %v, want [0x82 0x01]", got)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/rsbuf/... 2>&1 | head -10
```

- [ ] **Step 3: Create `pkg/rsbuf/mask_payload.go`**

```go
package rsbuf

import "github.com/zsrv/goscape/pkg/io/packet"

// writeMaskHeader writes the 1- or 2-byte mask header. If the mask value
// exceeds 0xff, the MaskBig bit is OR'd in and the header is IP2 (little-endian u16).
func writeMaskHeader(buf *packet.Packet, masks int) {
	if masks > 0xff {
		buf.IP2(uint16(masks | MaskBig))
	} else {
		buf.P1(uint8(masks))
	}
}

// writeMaskPayloads writes mask payloads in rsbuf's fixed order:
// ANIM → SAY → EXACT_MOVE → FACE_ENTITY → FACE_COORD → SPOT_ANIM → APPEARANCE → DAMAGE → CHAT
//
// `forceMasks` is the effective mask set to write (may differ from p.Masks() for
// low-def variants). `suppressChat` is true when writing a player's own high-def
// payload (self never sees own chat bubble via PlayerInfo).
func writeMaskPayloads(buf *packet.Packet, p PlayerSource, forceMasks int, suppressChat bool) {
	if forceMasks&MaskAnim != 0 {
		writeAnim(buf, p)
	}
	if forceMasks&MaskSay != 0 {
		writeSay(buf, p)
	}
	if forceMasks&MaskExactMove != 0 {
		writeExactMove(buf, p)
	}
	if forceMasks&MaskFaceEntity != 0 {
		writeFaceEntity(buf, p)
	}
	if forceMasks&MaskFaceCoord != 0 {
		writeFaceCoord(buf, p)
	}
	if forceMasks&MaskSpotAnim != 0 {
		writeSpotAnim(buf, p)
	}
	if forceMasks&MaskAppearance != 0 {
		writeAppearance(buf, p)
	}
	if forceMasks&MaskDamage != 0 {
		writeDamage(buf, p)
	}
	if forceMasks&MaskChat != 0 && !suppressChat {
		writeChat(buf, p)
	}
}

func writeAnim(buf *packet.Packet, p PlayerSource) {
	buf.P2(uint16(p.AnimID()))
	buf.P1Alt3(uint8(p.AnimDelay()))
}

func writeSay(buf *packet.Packet, p PlayerSource) {
	buf.Write(p.SayText())
	buf.P1(10) // line-feed terminator
}

func writeExactMove(buf *packet.Packet, p PlayerSource) {
	localOrigin := ((p.OriginX() >> 3) - 6) << 3
	localZOrigin := ((p.OriginZ() >> 3) - 6) << 3
	buf.P1Alt1(uint8(p.ExactStartX() - localOrigin))
	buf.P1Alt2(uint8(p.ExactStartZ() - localZOrigin))
	buf.P1Alt3(uint8(p.ExactEndX() - localOrigin))
	buf.P1(uint8(p.ExactEndZ() - localZOrigin))
	buf.P2(uint16(p.ExactBegin()))
	buf.P2Alt2(uint16(p.ExactFinish()))
	buf.P1(uint8(p.ExactDir()))
}

func writeFaceEntity(buf *packet.Packet, p PlayerSource) {
	buf.P2Alt2(uint16(p.FaceEntity()))
}

func writeFaceCoord(buf *packet.Packet, p PlayerSource) {
	buf.P2(uint16(p.FaceSquareX()))
	buf.P2(uint16(p.FaceSquareZ()))
}

func writeSpotAnim(buf *packet.Packet, p PlayerSource) {
	buf.P2Alt2(uint16(p.SpotAnimID()))
	buf.P4Alt2(uint32(p.SpotAnimHeight())<<16 | uint32(p.SpotAnimDelay()))
}

func writeAppearance(buf *packet.Packet, p PlayerSource) {
	app := p.AppearanceBytes()
	buf.P1(uint8(len(app)))
	buf.PDataAlt1(app)
}

func writeDamage(buf *packet.Packet, p PlayerSource) {
	buf.P1Alt1(uint8(p.DamageAmt()))
	buf.P1Alt3(uint8(p.DamageType()))
	buf.P1Alt2(uint8(p.CurHP()))
	buf.P1(uint8(p.BaseHP()))
}

func writeChat(buf *packet.Packet, p PlayerSource) {
	buf.P1(uint8(p.ChatColour()))
	buf.P1(uint8(p.ChatEffect()))
	buf.P1Alt2(uint8(p.ChatRights()))
	buf.P1Alt1(uint8(len(p.ChatBytes())))
	buf.PDataAlt2(p.ChatBytes())
}
```

> **Note:** `buf.Write([]byte)` is used for SAY — verify `*packet.Packet` has a `Write([]byte)` method. If not, substitute `for _, b := range p.SayText() { buf.P1(b) }`.

- [ ] **Step 4: Run tests**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/rsbuf/... -v 2>&1 | tail -20
```

Expected: all mask-payload tests pass.

- [ ] **Step 5: Commit**

```bash
git add pkg/rsbuf/mask_payload.go pkg/rsbuf/mask_payload_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(rsbuf): implement 9 mask-payload byte encoders in fixed rsbuf order

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 11: Renderer with 3-cache compute pass

**Files:**
- Create: `pkg/rsbuf/renderer.go`
- Create: `pkg/rsbuf/renderer_test.go`

- [ ] **Step 1: Write failing tests**

Create `pkg/rsbuf/renderer_test.go`:

```go
package rsbuf

import "testing"

func TestComputePlayersSkipsZeroMask(t *testing.T) {
	r := NewRenderer()
	p := &fakeSource{slot: 5, masks: 0, entityMask: 0}
	r.ComputePlayers([]PlayerSource{p})
	if r.HighDefOf(5) != nil {
		t.Error("HighDefOf(zero-mask) should be nil")
	}
	if r.LowDefFullOf(5) != nil {
		t.Error("LowDefFullOf(zero-mask) should be nil")
	}
}

func TestComputePlayersHighDef(t *testing.T) {
	r := NewRenderer()
	p := &fakeSource{slot: 5, masks: MaskAnim, animID: 100, animDelay: 2}
	r.ComputePlayers([]PlayerSource{p})
	got := r.HighDefOf(5)
	// header=MaskAnim=2 (1 byte), then p2(100) p1_alt3(2) = [0x00, 0x64, (0xff & -2)=0xfe]
	// = [2, 0, 100, 254]
	want := []byte{2, 0, 100, 254}
	if len(got) != len(want) {
		t.Fatalf("length: got %d, want %d (bytes=%v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("byte[%d]: got %#x, want %#x", i, got[i], want[i])
		}
	}
}

func TestComputePlayersLowDefForcesAppearance(t *testing.T) {
	r := NewRenderer()
	p := &fakeSource{
		slot: 5, masks: 0, entityMask: MaskFaceCoord,
		appearance:  []byte{1, 2, 3},
		faceSquareX: 100, faceSquareZ: 200,
	}
	r.ComputePlayers([]PlayerSource{p})
	lowFull := r.LowDefFullOf(5)
	if len(lowFull) == 0 {
		t.Fatal("LowDefFullOf should include APPEARANCE + FACE_COORD")
	}
	// First byte should be the mask header with APPEARANCE|FACE_COORD = 1|32 = 33.
	if lowFull[0] != 33 {
		t.Errorf("header byte: got %d, want 33 (APPEARANCE|FACE_COORD)", lowFull[0])
	}
}

func TestComputePlayersLowDefNoApp(t *testing.T) {
	r := NewRenderer()
	p := &fakeSource{slot: 5, masks: 0, entityMask: 0, faceSquareX: 100, faceSquareZ: 200}
	r.ComputePlayers([]PlayerSource{p})
	lowNo := r.LowDefNoAppOf(5)
	if len(lowNo) == 0 {
		t.Fatal("LowDefNoAppOf should include FACE_COORD at minimum")
	}
	// Should NOT include APPEARANCE bit (1).
	if lowNo[0]&MaskAppearance != 0 {
		t.Errorf("header byte: APPEARANCE bit should be unset in lowDefNoApp; got %d", lowNo[0])
	}
	// Should include FACE_COORD bit (32).
	if lowNo[0]&MaskFaceCoord == 0 {
		t.Errorf("header byte: FACE_COORD bit should be set; got %d", lowNo[0])
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/rsbuf/... -run TestComputePlayers 2>&1 | head -5
```

- [ ] **Step 3: Create `pkg/rsbuf/renderer.go`**

```go
package rsbuf

import "github.com/zsrv/goscape/pkg/io/packet"

// Renderer caches per-slot mask-payload byte slices for the current tick.
// ComputePlayers must run once per tick before any encoder calls into it.
type Renderer struct {
	highDef     [2048][]byte
	lowDefFull  [2048][]byte // includes forced APPEARANCE + FACE_COORD
	lowDefNoApp [2048][]byte // forces FACE_COORD but NOT APPEARANCE
}

// NewRenderer returns an empty renderer.
func NewRenderer() *Renderer { return &Renderer{} }

// ComputePlayers builds the three per-slot caches for the current tick.
func (r *Renderer) ComputePlayers(players []PlayerSource) {
	for _, p := range players {
		slot := p.Slot()
		if slot < 1 || slot >= len(r.highDef) {
			continue
		}
		masks := p.Masks()
		if masks == 0 && p.EntityMask() == 0 {
			r.highDef[slot] = nil
			r.lowDefFull[slot] = nil
			r.lowDefNoApp[slot] = nil
			continue
		}

		r.highDef[slot] = buildPayload(p, masks, true /*suppressChat*/)

		fullMasks := masks | MaskAppearance | MaskFaceCoord
		r.lowDefFull[slot] = buildPayload(p, fullMasks, true)

		noAppMasks := (masks | MaskFaceCoord) &^ MaskAppearance
		r.lowDefNoApp[slot] = buildPayload(p, noAppMasks, true)
	}
}

// HighDefOf returns the high-def mask payload bytes for the given slot (nil if no masks).
func (r *Renderer) HighDefOf(slot int) []byte {
	if slot < 1 || slot >= len(r.highDef) {
		return nil
	}
	return r.highDef[slot]
}

// LowDefFullOf returns the low-def payload bytes including APPEARANCE.
func (r *Renderer) LowDefFullOf(slot int) []byte {
	if slot < 1 || slot >= len(r.lowDefFull) {
		return nil
	}
	return r.lowDefFull[slot]
}

// LowDefNoAppOf returns the low-def payload bytes WITHOUT APPEARANCE.
func (r *Renderer) LowDefNoAppOf(slot int) []byte {
	if slot < 1 || slot >= len(r.lowDefNoApp) {
		return nil
	}
	return r.lowDefNoApp[slot]
}

func buildPayload(p PlayerSource, masks int, suppressChat bool) []byte {
	buf := packet.NewPacket(nil)
	writeMaskHeader(buf, masks)
	writeMaskPayloads(buf, p, masks, suppressChat)
	return append([]byte(nil), buf.Bytes()...)
}
```

- [ ] **Step 4: Run tests**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/rsbuf/... -v 2>&1 | tail -20
```

- [ ] **Step 5: Commit**

```bash
git add pkg/rsbuf/renderer.go pkg/rsbuf/renderer_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(rsbuf): add Renderer with 3-cache compute-info pass

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 12: 4-phase Encode function

**Files:**
- Create: `pkg/rsbuf/playerinfo.go`
- Create: `pkg/rsbuf/playerinfo_test.go`

- [ ] **Step 1: Write a basic failing test**

Create `pkg/rsbuf/playerinfo_test.go`:

```go
package rsbuf

import (
	"testing"

	"github.com/zsrv/goscape/pkg/buildarea"
	"github.com/zsrv/goscape/pkg/grid"
)

func TestEncodeIdlePlayer(t *testing.T) {
	self := &fakeSource{
		slot:       1,
		x:          3094, z: 3106, level: 0,
		originX:    3094, originZ: 3106,
		masks:      0,
		entityMask: 0,
	}
	all := []PlayerSource{self}
	ba := buildarea.New()
	g := grid.New()
	r := NewRenderer()
	r.ComputePlayers(all)

	payload := Encode(self, all, ba, g, r)
	if len(payload) == 0 {
		t.Fatal("Encode should produce non-empty payload")
	}
	// Idle player, no other players, no masks: own block = pbit(1,0) then
	// pbit(8, 0) for zero tracked players, then new-players loop (empty).
	// The terminator pbit(11, 2047) fires only if there's mask data — there isn't.
	// Minimum expected: 1 byte containing the initial 0 bit, the 8 bits for
	// tracked count (=0), and padding. Let's just assert it doesn't panic and
	// the first bit is 0.
	if payload[0]&0x80 != 0 {
		t.Errorf("first bit (idle flag): got 1, want 0; payload=%v", payload)
	}
}

func TestEncodeTwoPlayersEachSeesOther(t *testing.T) {
	a := &fakeSource{
		slot:       1,
		x:          3094, z: 3106, level: 0,
		originX:    3094, originZ: 3106,
		masks:      0,
		appearance: []byte{1, 2, 3},
	}
	b := &fakeSource{
		slot:       2,
		x:          3095, z: 3106, level: 0,
		originX:    3095, originZ: 3106,
		masks:      0,
		appearance: []byte{4, 5, 6},
	}
	all := []PlayerSource{a, b}

	baA := buildarea.New()
	g := grid.New()
	g.Add(a.Slot(), a.x, a.z, a.level)
	g.Add(b.Slot(), b.x, b.z, b.level)

	r := NewRenderer()
	r.ComputePlayers(all)

	payload := Encode(a, all, baA, g, r)
	if len(payload) == 0 {
		t.Fatal("Encode should produce non-empty payload")
	}
	// After encoding, a's BuildArea.Players should contain b's slot.
	if _, ok := baA.Players[2]; !ok {
		t.Errorf("after encode, a's tracked players should include slot 2; got %v", baA.Players)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/rsbuf/... -run TestEncode 2>&1 | head -5
```

Expected: `Encode` undefined.

- [ ] **Step 3: Create `pkg/rsbuf/playerinfo.go`**

```go
package rsbuf

import (
	"github.com/zsrv/goscape/pkg/buildarea"
	"github.com/zsrv/goscape/pkg/grid"
	"github.com/zsrv/goscape/pkg/io/packet"
)

const (
	// PreferredPlayers caps how many players are added to a client's tracked set.
	PreferredPlayers = 255
	// MaxPacketBytes is the 4997-byte budget from rsbuf for a single PlayerInfo packet.
	MaxPacketBytes = 4997
	// ViewDistanceZones is the zone-radius used in Phase 3 new-player search.
	ViewDistanceZones = 15
)

// Encode produces the full PlayerInfo payload for `self` (no opcode/length prefix).
// The caller wraps it with OpPlayerInfo via writeOut.
func Encode(self PlayerSource, all []PlayerSource, ba *buildarea.BuildArea, g *grid.Grid, r *Renderer) []byte {
	// all[] is keyed by slot; build a slot -> PlayerSource lookup.
	bySlot := make(map[int]PlayerSource, len(all))
	for _, p := range all {
		bySlot[p.Slot()] = p
	}

	main := packet.NewPacket(nil)
	pb := packet.NewPacketBit(main)
	updates := packet.NewPacket(nil)

	// Phase 1: own-player block.
	writeLocalPlayer(pb, updates, self, r)

	// Phase 2: other-players delta.
	writeOtherPlayers(pb, updates, self, bySlot, ba, r)

	// Phase 3: new-players loop.
	writeNewPlayers(pb, updates, self, bySlot, ba, g, r)

	// Phase 4: terminator + updates.
	if updates.Len() > 0 {
		pb.PBit(11, 2047)
	}
	pb.Finish()

	// Append the mask-updates buffer to the bit-packed section.
	main.Write(updates.Bytes())
	return main.Bytes()
}

func writeLocalPlayer(pb *packet.PacketBit, updates *packet.Packet, self PlayerSource, r *Renderer) {
	x, z, level := self.Coords()
	masks := self.Masks()
	extend := 0
	payload := r.HighDefOf(self.Slot())
	if len(payload) > 0 && fits(pb, updates, len(payload)) {
		extend = 1
	}

	switch {
	case self.Tele():
		originX := self.OriginX()
		originZ := self.OriginZ()
		localX := x - (((originX >> 3) - 6) << 3)
		localZ := z - (((originZ >> 3) - 6) << 3)
		pb.PBit(1, 1)
		pb.PBit(2, 3)
		pb.PBit(1, boolToInt(self.Jump()))
		pb.PBit(2, level)
		pb.PBit(7, localZ)
		pb.PBit(7, localX)
		pb.PBit(1, extend)
	case self.RunDir() != -1:
		pb.PBit(1, 1)
		pb.PBit(2, 2)
		pb.PBit(3, self.WalkDir())
		pb.PBit(3, self.RunDir())
		pb.PBit(1, extend)
	case self.WalkDir() != -1:
		pb.PBit(1, 1)
		pb.PBit(2, 1)
		pb.PBit(3, self.WalkDir())
		pb.PBit(1, extend)
	case masks != 0:
		pb.PBit(1, 1)
		pb.PBit(2, 0)
		extend = 1
	default:
		pb.PBit(1, 0)
	}

	if extend == 1 && len(payload) > 0 {
		updates.Write(payload)
	}
}

func writeOtherPlayers(pb *packet.PacketBit, updates *packet.Packet, self PlayerSource, bySlot map[int]PlayerSource, ba *buildarea.BuildArea, r *Renderer) {
	pb.PBit(8, len(ba.Players))

	for slot := range ba.Players {
		other, ok := bySlot[slot]
		selfX, selfZ, selfLevel := self.Coords()

		if !ok || !other.Active() || other.Tele() {
			pb.PBit(1, 1)
			pb.PBit(2, 3) // remove
			delete(ba.Players, slot)
			continue
		}
		ox, oz, ol := other.Coords()
		if ol != selfLevel || zoneDist(selfX, selfZ, ox, oz) > ViewDistanceZones {
			pb.PBit(1, 1)
			pb.PBit(2, 3)
			delete(ba.Players, slot)
			continue
		}
		if other.Visibility() == VisibilityHard {
			pb.PBit(1, 1)
			pb.PBit(2, 3)
			delete(ba.Players, slot)
			continue
		}
		if other.Visibility() == VisibilitySoft && self.StaffModLevel() < 1 {
			pb.PBit(1, 1)
			pb.PBit(2, 3)
			delete(ba.Players, slot)
			continue
		}

		// Movement.
		extend := 0
		payload := r.HighDefOf(slot)
		if len(payload) > 0 && fits(pb, updates, len(payload)) {
			extend = 1
		}
		switch {
		case other.RunDir() != -1:
			pb.PBit(1, 1)
			pb.PBit(2, 2)
			pb.PBit(3, other.WalkDir())
			pb.PBit(3, other.RunDir())
			pb.PBit(1, extend)
		case other.WalkDir() != -1:
			pb.PBit(1, 1)
			pb.PBit(2, 1)
			pb.PBit(3, other.WalkDir())
			pb.PBit(1, extend)
		case other.Masks() != 0:
			pb.PBit(1, 1)
			pb.PBit(2, 0)
			extend = 1
		default:
			pb.PBit(1, 0)
		}

		if extend == 1 && len(payload) > 0 {
			updates.Write(payload)
		}
	}
}

func writeNewPlayers(pb *packet.PacketBit, updates *packet.Packet, self PlayerSource, bySlot map[int]PlayerSource, ba *buildarea.BuildArea, g *grid.Grid, r *Renderer) {
	selfX, selfZ, selfLevel := self.Coords()
	candidates := g.NearbyPlayers(selfX, selfZ, selfLevel, ViewDistanceZones)

	for _, slot := range candidates {
		if slot == self.Slot() {
			continue
		}
		if _, already := ba.Players[slot]; already {
			continue
		}
		if len(ba.Players) >= PreferredPlayers {
			break
		}
		other, ok := bySlot[slot]
		if !ok || !other.Active() || other.Visibility() == VisibilityHard {
			continue
		}
		if other.Visibility() == VisibilitySoft && self.StaffModLevel() < 1 {
			continue
		}

		ox, oz, _ := other.Coords()
		dx := clamp(ox-selfX, -15, 15)
		dz := clamp(oz-selfZ, -15, 15)

		pb.PBit(11, slot)
		pb.PBit(5, dz&0x1f)
		pb.PBit(1, 1)
		pb.PBit(1, boolToInt(other.Jump()))
		pb.PBit(5, dx&0x1f)

		ba.Players[slot] = struct{}{}

		hash := other.AppearanceHash()
		if ba.HasAppearance(slot, hash) {
			if payload := r.LowDefNoAppOf(slot); len(payload) > 0 {
				updates.Write(payload)
			}
		} else {
			if payload := r.LowDefFullOf(slot); len(payload) > 0 {
				updates.Write(payload)
			}
			ba.RecordAppearance(slot, hash)
		}
	}
}

// fits reports whether adding another `nBytes` of mask updates keeps the
// packet within the MaxPacketBytes budget.
func fits(pb *packet.PacketBit, updates *packet.Packet, nBytes int) bool {
	total := pb.BytesSoFar() + updates.Len() + nBytes
	return total <= MaxPacketBytes
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func zoneDist(x1, z1, x2, z2 int) int {
	dx := abs((x1 >> 3) - (x2 >> 3))
	dz := abs((z1 >> 3) - (z2 >> 3))
	if dx > dz {
		return dx
	}
	return dz
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
```

> **PacketBit helpers needed:** this code assumes `*packet.PacketBit` exposes `PBit(numBits, value int)`, `Finish()` (pads to byte boundary), and `BytesSoFar() int` (returns the byte count written so far, rounded up from bitPos). Verify these exist; if not, either add them to `packetbit.go` or compute `BytesSoFar` inline. `packet.NewPacketBit(p *Packet) *PacketBit` is the constructor.

- [ ] **Step 4: Run tests**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/rsbuf/... -v 2>&1 | tail -20
```

Expected: tests pass. If `PacketBit` API mismatches occur, adjust the encoder (or add missing methods to packetbit.go). If `packet.Packet` lacks `Write([]byte)`, add one or substitute a loop.

- [ ] **Step 5: Commit**

```bash
git add pkg/rsbuf/playerinfo.go pkg/rsbuf/playerinfo_test.go
# Also commit any packetbit.go adjustments required
git add pkg/io/packet/
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(rsbuf): implement 4-phase PlayerInfo Encode function

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 13: Wire renderer + grid into Server; processInfo tick phase

**Files:**
- Modify: `modules/world/server.go`
- Modify: `modules/world/tick.go`

- [ ] **Step 1: Add fields to `Server`**

In `modules/world/server.go`, add to the `Server` struct:

```go
renderer *rsbuf.Renderer
grid     *grid.Grid
```

Add imports:

```go
"github.com/zsrv/goscape/pkg/grid"
"github.com/zsrv/goscape/pkg/rsbuf"
```

- [ ] **Step 2: Initialise in `NewServer`**

After `s.invTypes = invTypes` (from sub-spec 3a), add:

```go
s.renderer = rsbuf.NewRenderer()
s.grid = grid.New()
```

- [ ] **Step 3: Update `addPlayer` / `removePlayer`**

Find `addPlayer` — after the slot assignment succeeds, set `p.active = true`. Find `removePlayer` — set `p.active = false` before removal.

- [ ] **Step 4: Add `processInfo` in `tick.go`**

```go
func (s *Server) processInfo() {
	s.playersMu.RLock()
	players := make([]*Player, len(s.playerLoop))
	copy(players, s.playerLoop)
	s.playersMu.RUnlock()

	// Update grid positions for players that crossed a zone boundary.
	for _, p := range players {
		curZX, curZZ := p.x>>3, p.z>>3
		prevZX, prevZZ := p.lastTickX>>3, p.lastTickZ>>3
		if prevZX != curZX || prevZZ != curZZ || p.lastLevel != p.level {
			if p.lastTickX >= 0 {
				s.grid.Remove(p.slot, p.lastTickX, p.lastTickZ, p.lastLevel)
			}
			s.grid.Add(p.slot, p.x, p.z, p.level)
		}
	}

	// Build PlayerSource list and run renderer compute pass.
	sources := make([]rsbuf.PlayerSource, len(players))
	for i, p := range players {
		sources[i] = p
	}
	s.renderer.ComputePlayers(sources)
}

func (s *Server) processCleanup() {
	s.playersMu.RLock()
	players := make([]*Player, len(s.playerLoop))
	copy(players, s.playerLoop)
	s.playersMu.RUnlock()
	for _, p := range players {
		p.ResetMasks()
	}
}
```

Add imports:

```go
"github.com/zsrv/goscape/pkg/rsbuf"
```

- [ ] **Step 5: Insert phases in `runTickLoopWithRate`**

Find the existing tick body:

```go
s.processClientsIn()
s.processPathing()
s.processLogouts()
s.processLogins()
s.processClientsOut()
```

Rewrite to:

```go
s.processClientsIn()
s.processPathing()
s.processLogouts()
s.processLogins()
s.processInfo()       // NEW
s.processClientsOut()
s.processCleanup()    // NEW
```

- [ ] **Step 6: Run existing tests**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... 2>&1 | tail -5
```

Expected: all pass.

- [ ] **Step 7: Commit**

```bash
git add modules/world/server.go modules/world/tick.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): wire renderer + grid into Server; add processInfo/processCleanup phases

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 14: Replace `updatePlayers` stub

**Files:**
- Create: `modules/world/player_info.go`
- Modify: `modules/world/player.go` (remove the stub)

- [ ] **Step 1: Create `modules/world/player_info.go`**

```go
package world

import (
	gameserver "github.com/zsrv/goscape/pkg/io/protocol/game/server"
	"github.com/zsrv/goscape/pkg/rsbuf"
)

// updatePlayers fills in the sub-spec 1 stub. Runs during processClientsOut.
// Snapshots the player loop, feeds it to rsbuf.Encode, and writes the resulting
// bytes as an OpPlayerInfo packet.
func (p *Player) updatePlayers() {
	s := p.client.server
	if s == nil || p.buildArea == nil || s.renderer == nil || s.grid == nil {
		return
	}

	s.playersMu.RLock()
	snapshot := make([]*Player, len(s.playerLoop))
	copy(snapshot, s.playerLoop)
	s.playersMu.RUnlock()

	sources := make([]rsbuf.PlayerSource, len(snapshot))
	for i, op := range snapshot {
		sources[i] = op
	}

	payload := rsbuf.Encode(p, sources, p.buildArea, s.grid, s.renderer)
	p.writeOut(gameserver.OpPlayerInfo, payload)
}
```

- [ ] **Step 2: Remove the existing stub**

In `modules/world/player.go`, find `func (p *Player) updatePlayers() {}` and delete it. The real implementation now lives in `player_info.go`.

- [ ] **Step 3: Build**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./modules/world/...
```

Expected: no errors.

- [ ] **Step 4: Run existing tests**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... 2>&1 | tail -5
```

- [ ] **Step 5: Commit**

```bash
git add modules/world/player_info.go modules/world/player.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): implement updatePlayers using rsbuf.Encode

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 15: CLIENT_CHEAT handler — `::say` command

**Files:**
- Modify: `modules/world/handlers_game.go`

- [ ] **Step 1: Add the handler**

Append to `modules/world/handlers_game.go`:

```go
import "strings"

func handleClientCheat(p *Player, payload []byte) error {
	r := packet.NewPacket(payload)
	_ = r.G1() // unused byte per TS handler
	raw := r.GJStrLF()
	if !strings.HasPrefix(string(raw), "::") {
		return nil
	}
	cmd := strings.TrimPrefix(string(raw), "::")
	parts := strings.SplitN(cmd, " ", 2)
	switch parts[0] {
	case "say":
		if len(parts) == 2 {
			p.Say([]byte(parts[1]))
		}
	}
	return nil
}
```

In the `init()` block of `handlers_game.go`, register it:

```go
gameHandlers[4] = handleClientCheat // CLIENT_CHEAT
```

> **Verify:** if `packet.NewPacket(payload)` in the existing handlers uses a different accessor than `GJStrLF` for variable-length strings, match that convention. The TS `ClientCheat` codec reads `pjstr` (NUL-terminated); goscape's equivalent is `GJStrLF` (if present) or equivalent.

- [ ] **Step 2: Build**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./modules/world/...
```

- [ ] **Step 3: Commit**

```bash
git add modules/world/handlers_game.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): CLIENT_CHEAT handler parses ::say <msg> and calls p.Say

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 16: Integration test — two players see each other + chat

**Files:**
- Create: `modules/world/player_info_test.go`

- [ ] **Step 1: Write the integration test**

```go
package world

import (
	"bytes"
	"testing"

	"github.com/zsrv/goscape/pkg/buildarea"
	"github.com/zsrv/goscape/pkg/gamemap"
	"github.com/zsrv/goscape/pkg/grid"
	"github.com/zsrv/goscape/pkg/rsbuf"
)

// setupInfoPlayer constructs a Player with the full 3a/3b scaffolding.
func setupInfoPlayer(t *testing.T, s *Server, slot, x, z, level int) *Player {
	t.Helper()
	p, _ := newTestPlayer(t)
	p.client.server = s
	enc, dec := isaacPair([4]uint32{uint32(slot), 2, 3, 4})
	p.client.encryptor = enc
	p.client.decryptor = dec
	p.x, p.z, p.level = x, z, level
	p.originX, p.originZ = x, z
	p.lastTickX, p.lastTickZ, p.lastLevel = x, z, level
	p.buildArea = buildarea.New()
	p.slot = slot
	s.players[slot] = p
	s.playerLoop = append(s.playerLoop, p)
	p.active = true
	s.grid.Add(slot, x, z, level)
	return p
}

func TestTwoPlayersSeeEachOther(t *testing.T) {
	s := newTestServer(t)
	s.gamemap = gamemap.New(discardLogger())
	s.gamemap.Init(t.TempDir())
	s.renderer = rsbuf.NewRenderer()
	s.grid = grid.New()

	a := setupInfoPlayer(t, s, 1, 3094, 3106, 0)
	b := setupInfoPlayer(t, s, 2, 3095, 3106, 0)

	// Run one processInfo + updatePlayers cycle for a.
	s.processInfo()
	a.updatePlayers()

	// After a.updatePlayers, a.buildArea.Players should contain b.
	if _, ok := a.buildArea.Players[2]; !ok {
		t.Errorf("a should track b after updatePlayers; got %v", a.buildArea.Players)
	}
}

func TestSayProducesChatMaskInNextTick(t *testing.T) {
	s := newTestServer(t)
	s.gamemap = gamemap.New(discardLogger())
	s.gamemap.Init(t.TempDir())
	s.renderer = rsbuf.NewRenderer()
	s.grid = grid.New()

	a := setupInfoPlayer(t, s, 1, 3094, 3106, 0)

	a.Say([]byte("hello"))
	s.processInfo()

	highDef := s.renderer.HighDefOf(1)
	if len(highDef) == 0 {
		t.Fatal("high-def should be non-empty after Say()")
	}
	// Header byte should include MaskSay (0x8).
	if highDef[0]&rsbuf.MaskSay == 0 {
		t.Errorf("high-def header should have MaskSay: got %d", highDef[0])
	}
	// Payload should contain the "hello" bytes followed by terminator.
	if !bytes.Contains(highDef, []byte("hello\n")) {
		// SAY terminator is byte 10 (line-feed).
		hasHello := bytes.Contains(highDef, []byte("hello")) && bytes.Contains(highDef, []byte{10})
		if !hasHello {
			t.Errorf("high-def should contain 'hello' + LF; got %v", highDef)
		}
	}
}
```

- [ ] **Step 2: Run tests**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run "TestTwoPlayersSeeEachOther|TestSayProducesChatMask" -v 2>&1 | tail -20
```

Expected: both pass.

- [ ] **Step 3: Commit**

```bash
git add modules/world/player_info_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
test(world): integration test — two players see each other, Say raises chat mask

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 17: Final verification

- [ ] **Step 1: Full test suite**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
```

Expected: all `ok`.

- [ ] **Step 2: Race detector**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./... 2>&1 | grep -E "FAIL|DATA RACE|^ok" | tail -20
```

Expected: all pass, zero races.

- [ ] **Step 3: Binary builds**

```bash
CGO_ENABLED=0 GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build -trimpath ./cmd/goscape && rm -f goscape && echo "build OK"
```

Expected: `build OK`.
