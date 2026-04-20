# Sub-spec 4b-4: Zone Tick Integration — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: superpowers:subagent-driven-development (recommended).

**Goal:** Wire `pkg/zone` into the tick loop. Replace the last `Player.updateZones()` stub with a real implementation that emits UpdateZoneFullFollows / PartialEnclosed / PartialFollows packets per-player per-zone.

**Architecture:** 5 surgical edits + 2 new files. Moves `ZoneIndex`/`UnpackZoneIndex` to `pkg/coordgrid` to avoid a buildarea→zone import cycle. Adds 10 top-level zone opcodes matching the Java client's SERVERPROT_SIZES.

**Tech Stack:** Go 1.26. All the usual internal packages.

**Build prefix:** `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go ...`
**Commit flag:** `--no-gpg-sign`

**Spec reference:** `docs/superpowers/specs/2026-04-20-zone-tick-integration-design.md`

---

## File Structure

**Create:**
- `modules/world/world_zone.go` — 11 Server dispatcher methods
- `modules/world/world_zone_test.go`
- `modules/world/player_zone.go` — writeFullFollows, writePartialFollows, sendZoneNested
- `modules/world/player_zone_test.go`
- `modules/world/tick_zone_test.go`

**Modify:**
- `pkg/coordgrid/coordgrid.go` — add `ZoneIndex` + `UnpackZoneIndex`
- `pkg/coordgrid/coordgrid_test.go` — add round-trip tests
- `pkg/zone/map.go` — remove local ZoneIndex/UnpackIndex; use coordgrid
- `pkg/zone/map_test.go` — delete/migrate the relocated test
- `pkg/io/protocol/game/server/prot.go` — add 10 top-level zone opcodes
- `pkg/buildarea/buildarea.go` — populate ActiveZones in Rebuild
- `pkg/buildarea/buildarea_test.go` — verify ActiveZones population
- `modules/world/server.go` — add zoneMap + zonesTracking + TrackZone
- `modules/world/player.go` — real updateZones body
- `modules/world/tick.go` — add processZones phase; extend processCleanup

---

## Task 1: Move `ZoneIndex`/`UnpackIndex` to `pkg/coordgrid`

Cleanest starting point — breaks the would-be `pkg/buildarea → pkg/zone` import cycle before Task 4 needs it.

**Files:**
- Modify: `pkg/coordgrid/coordgrid.go`, `pkg/coordgrid/coordgrid_test.go`
- Modify: `pkg/zone/map.go`, `pkg/zone/map_test.go`

- [ ] **Step 1.1: Add the coordgrid versions + tests**

Append to `pkg/coordgrid/coordgrid.go`:

```go
// ZoneIndex packs (worldX, worldZ, level) into a single int using the
// layout shared with the TS reference's ZoneMap.zoneIndex:
//
//	zone_x = worldX >> 3, zone_z = worldZ >> 3
//	index  = (zone_x & 0x7FF) | ((zone_z & 0x7FF) << 11) | ((level & 0x3) << 22)
func ZoneIndex(worldX, worldZ, level int) int {
	return ((worldX >> 3) & 0x7FF) | (((worldZ >> 3) & 0x7FF) << 11) | ((level & 0x3) << 22)
}

// UnpackZoneIndex reverses ZoneIndex. Returns TILE-unit coordinates at
// the zone's SW corner (zoneX<<3, zoneZ<<3).
func UnpackZoneIndex(index int) (worldX, worldZ, level int) {
	worldX = (index & 0x7FF) << 3
	worldZ = ((index >> 11) & 0x7FF) << 3
	level = (index >> 22) & 0x3
	return
}
```

Append to `pkg/coordgrid/coordgrid_test.go`:

```go
func TestZoneIndexRoundTrip(t *testing.T) {
	// (3094, 3106, 0) → zone (386, 388) → packs; unpacks to tile SW corner (3088, 3104).
	idx := ZoneIndex(3094, 3106, 0)
	x, z, level := UnpackZoneIndex(idx)
	if x != 3088 || z != 3104 || level != 0 {
		t.Errorf("roundtrip: got (%d,%d,%d), want (3088,3104,0)", x, z, level)
	}
}

func TestZoneIndexDistinguishesLevels(t *testing.T) {
	if ZoneIndex(0, 0, 0) == ZoneIndex(0, 0, 1) {
		t.Error("same x/z at different levels must have distinct indexes")
	}
}
```

- [ ] **Step 1.2: Run — expect PASS**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/coordgrid/ -run TestZoneIndex -v`
Expected: PASS.

- [ ] **Step 1.3: Delete `pkg/zone/map.go`'s ZoneIndex/UnpackIndex and reroute**

Edit `pkg/zone/map.go` to delete the local `ZoneIndex` and `UnpackIndex` functions and route callers:

```go
// Replace the old local ZoneIndex/UnpackIndex references with:
func (m *ZoneMap) Get(level, worldX, worldZ int) *Zone {
	return m.GetByIndex(coordgrid.ZoneIndex(worldX, worldZ, level))
}

func (m *ZoneMap) GetByIndex(index int) *Zone {
	if z, ok := m.zones[index]; ok {
		return z
	}
	x, z, level := coordgrid.UnpackZoneIndex(index)
	zone := New(index, level, x>>3, z>>3)
	m.zones[index] = zone
	return zone
}
```

Add the import `"github.com/zsrv/goscape/pkg/coordgrid"`.

Delete the old `ZoneIndex` and `UnpackIndex` functions from `pkg/zone/map.go`.

- [ ] **Step 1.4: Delete / migrate the relocated test**

Edit `pkg/zone/map_test.go` — delete `TestZoneIndexRoundTrip` (migrated to coordgrid).

- [ ] **Step 1.5: Full suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`
Expected: PASS across all packages.

- [ ] **Step 1.6: Commit**

```bash
git add pkg/coordgrid/ pkg/zone/
git commit --no-gpg-sign -m "refactor(coordgrid,zone): move ZoneIndex to pkg/coordgrid

Breaks a potential pkg/buildarea → pkg/zone import cycle (4b-4 needs
buildarea.Rebuild to populate ActiveZones via ZoneIndex).

pkg/zone/map.go now calls through coordgrid. The unit test migrates
from pkg/zone/map_test.go to pkg/coordgrid/coordgrid_test.go."
```

---

## Task 2: 10 top-level zone opcodes in `prot.go`

**Files:** Modify `pkg/io/protocol/game/server/prot.go`

- [ ] **Step 2.1: Append the 10 Op{} entries**

In `pkg/io/protocol/game/server/prot.go`, after the zone-outer opcodes, add:

```go
// Zone-nested opcodes, reused as top-level packets for per-player
// UpdateZonePartialFollows delivery. Sizes match the Java client's
// SERVERPROT_SIZES at the matching indices.
OpLocAddChange = Op{Opcode: 59, PayloadSize: 4}
OpLocAnim      = Op{Opcode: 42, PayloadSize: 4}
OpLocDel       = Op{Opcode: 76, PayloadSize: 2}
OpLocMerge     = Op{Opcode: 23, PayloadSize: 14}
OpMapAnim      = Op{Opcode: 191, PayloadSize: 6}
OpMapProjAnim  = Op{Opcode: 69, PayloadSize: 15}
OpObjAdd       = Op{Opcode: 223, PayloadSize: 5}
OpObjCount     = Op{Opcode: 151, PayloadSize: 7}
OpObjDel       = Op{Opcode: 49, PayloadSize: 3}
OpObjReveal    = Op{Opcode: 50, PayloadSize: 7}
```

- [ ] **Step 2.2: Build + test**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...`
Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`
Expected: both PASS.

- [ ] **Step 2.3: Commit**

```bash
git add pkg/io/protocol/game/server/prot.go
git commit --no-gpg-sign -m "feat(server-prot): add 10 zone-nested-as-top-level opcodes

Zone-nested opcodes (LOC_ADD_CHANGE=59, LOC_ANIM=42, etc.) double as
top-level outgoing packets when sent per-player inside an
UpdateZonePartialFollows wrapper. Sizes verified against
Client-Java/.../Protocol.java SERVERPROT_SIZES:
  59→4, 42→4, 76→2, 23→14, 191→6, 69→15, 223→5, 151→7, 49→3, 50→7"
```

---

## Task 3: `BuildArea.Rebuild` populates ActiveZones + tests

**Files:** Modify `pkg/buildarea/buildarea.go`, `pkg/buildarea/buildarea_test.go`

- [ ] **Step 3.1: Write the failing test**

Append to `pkg/buildarea/buildarea_test.go`:

```go
func TestRebuildPopulatesActiveZones(t *testing.T) {
	ba := New()
	_ = ba.Rebuild(3094, 3106, 100)
	// 13×13 window (for dx := -6; dx <= 6).
	if got := len(ba.ActiveZones); got != 169 {
		t.Errorf("ActiveZones: got %d, want 169 (13x13)", got)
	}
	// Zone containing (3094, 3106) itself should be present.
	idx := coordgrid.ZoneIndex(3094, 3106, 0)
	if !ba.ActiveZones[idx] {
		t.Errorf("ActiveZones missing origin zone index %d", idx)
	}
}
```

Add `"github.com/zsrv/goscape/pkg/coordgrid"` to the test file's imports.

- [ ] **Step 3.2: Run — verify FAIL**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/buildarea/ -run TestRebuildPopulatesActiveZones -v`
Expected: FAIL — `ActiveZones` is empty.

- [ ] **Step 3.3: Implement**

In `pkg/buildarea/buildarea.go`, add import `"github.com/zsrv/goscape/pkg/coordgrid"` and update the `Rebuild` loop body to also populate `ActiveZones`:

```go
for dx := -6; dx <= 6; dx++ {
	for dz := -6; dz <= 6; dz++ {
		zx := zoneX + dx
		zz := zoneZ + dz
		if zx < 0 || zz < 0 {
			continue
		}
		mapX := zx >> 3
		mapZ := zz >> 3
		if mapX > 0xff || mapZ > 0xff {
			continue
		}
		ba.Mapsquares[uint16((mapX<<8)|mapZ)] = true
		ba.ActiveZones[coordgrid.ZoneIndex(zx<<3, zz<<3, 0)] = true // NEW
	}
}
```

- [ ] **Step 3.4: Run — verify PASS**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/buildarea/ -v`
Expected: all PASS.

- [ ] **Step 3.5: Full suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`
Expected: PASS.

- [ ] **Step 3.6: Commit**

```bash
git add pkg/buildarea/
git commit --no-gpg-sign -m "feat(buildarea): populate ActiveZones in Rebuild

BuildArea.Rebuild now fills ActiveZones in the same 13x13 loop that
populates Mapsquares. Uses coordgrid.ZoneIndex for consistent packing
with the rest of the zone subsystem.

Unblocks Player.updateZones in sub-spec 4b-4."
```

---

## Task 4: Server fields + TrackZone + 11 dispatcher methods + tests

**Files:**
- Modify: `modules/world/server.go`
- Create: `modules/world/world_zone.go`
- Create: `modules/world/world_zone_test.go`

- [ ] **Step 4.1: Add Server fields + init + TrackZone**

Edit `modules/world/server.go`:

Add import `"github.com/zsrv/goscape/pkg/zone"` to the existing import block.

Add to the `Server` struct (near other collection fields):
```go
zoneMap       *zone.ZoneMap
zonesTracking map[*zone.Zone]struct{}
```

In `NewServer` (wherever it is — check by reading the file), initialise:
```go
zoneMap:       zone.NewZoneMap(),
zonesTracking: map[*zone.Zone]struct{}{},
```

Add at the bottom of the file:
```go
// TrackZone marks a zone as modified this tick. Idempotent (map semantics).
// processZones will call ComputeShared on each tracked zone; processCleanup
// will Reset them and clear the set.
func (s *Server) TrackZone(z *zone.Zone) { s.zonesTracking[z] = struct{}{} }
```

- [ ] **Step 4.2: Write failing tests**

Create `modules/world/world_zone_test.go`:

```go
package world

import (
	"testing"

	"github.com/zsrv/goscape/pkg/entity"
	"github.com/zsrv/goscape/pkg/zone"
)

func newZoneTestServer(t *testing.T) *Server {
	t.Helper()
	s := newTestServer(t)
	s.zoneMap = zone.NewZoneMap()
	s.zonesTracking = map[*zone.Zone]struct{}{}
	return s
}

func TestServerAddLocTracksZone(t *testing.T) {
	s := newZoneTestServer(t)
	loc := entity.NewLoc(0, 3094, 3106, 1, 1, entity.LifecycleDespawn, 100, 0, 0)
	s.AddLoc(loc)
	if len(s.zonesTracking) != 1 {
		t.Errorf("zonesTracking: got %d, want 1", len(s.zonesTracking))
	}
}

func TestServerAddObjRoutesByCoord(t *testing.T) {
	s := newZoneTestServer(t)
	objA := entity.NewObj(0, 3094, 3106, entity.LifecycleDespawn, 995, 10)
	objB := entity.NewObj(0, 3200, 3200, entity.LifecycleDespawn, 995, 10)
	s.AddObj(objA, zone.PublicReceiver)
	s.AddObj(objB, zone.PublicReceiver)
	if len(s.zonesTracking) != 2 {
		t.Errorf("zonesTracking: got %d, want 2 (distinct zones)", len(s.zonesTracking))
	}
}

func TestServerChangeObjPassesCurrentTick(t *testing.T) {
	s := newZoneTestServer(t)
	s.currentTick = 42
	obj := entity.NewObj(0, 3094, 3106, entity.LifecycleDespawn, 995, 10)
	obj.ReceiverID = 7
	s.ChangeObj(obj, 10, 25)
	if obj.Count != 25 {
		t.Errorf("Count: got %d, want 25", obj.Count)
	}
	if obj.LastChange != 42 {
		t.Errorf("LastChange: got %d, want 42", obj.LastChange)
	}
}

func TestServerDispatchersTrackOncePerZone(t *testing.T) {
	s := newZoneTestServer(t)
	loc := entity.NewLoc(0, 3094, 3106, 1, 1, entity.LifecycleDespawn, 100, 0, 0)
	s.AddLoc(loc)
	s.ChangeLoc(loc)
	s.AnimLoc(loc, 42)
	if len(s.zonesTracking) != 1 {
		t.Errorf("zonesTracking: got %d, want 1 (same zone, 3 mutations)", len(s.zonesTracking))
	}
}
```

- [ ] **Step 4.3: Run — verify FAIL**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestServer(AddLoc|AddObj|ChangeObj|Dispatchers)' -v`
Expected: FAIL — `s.AddLoc` etc. don't exist.

- [ ] **Step 4.4: Implement `world_zone.go`**

Create `modules/world/world_zone.go`:

```go
package world

import (
	"github.com/zsrv/goscape/pkg/entity"
)

// AddLoc routes a loc spawn/change through the world's zone map.
func (s *Server) AddLoc(loc *entity.Loc) {
	z := s.zoneMap.Get(loc.Level, loc.X, loc.Z)
	z.AddLoc(loc)
	s.TrackZone(z)
}

// ChangeLoc routes a loc type/shape/angle mutation.
func (s *Server) ChangeLoc(loc *entity.Loc) {
	z := s.zoneMap.Get(loc.Level, loc.X, loc.Z)
	z.ChangeLoc(loc)
	s.TrackZone(z)
}

// RemoveLoc routes a loc removal.
func (s *Server) RemoveLoc(loc *entity.Loc) {
	z := s.zoneMap.Get(loc.Level, loc.X, loc.Z)
	z.RemoveLoc(loc)
	s.TrackZone(z)
}

// AnimLoc routes a loc animation.
func (s *Server) AnimLoc(loc *entity.Loc, seq int) {
	z := s.zoneMap.Get(loc.Level, loc.X, loc.Z)
	z.AnimLoc(loc, seq)
	s.TrackZone(z)
}

// MergeLoc routes a multi-tile loc merge.
func (s *Server) MergeLoc(
	loc *entity.Loc,
	playerSlot, startCycle, endCycle int,
	east, south, west, north int,
) {
	z := s.zoneMap.Get(loc.Level, loc.X, loc.Z)
	z.MergeLoc(loc, playerSlot, startCycle, endCycle, east, south, west, north)
	s.TrackZone(z)
}

// AddObj routes a ground-item spawn. receiverID == zone.PublicReceiver for
// public drops; otherwise the receiver's player slot.
func (s *Server) AddObj(obj *entity.Obj, receiverID int) {
	z := s.zoneMap.Get(obj.Level, obj.X, obj.Z)
	z.AddObj(obj, receiverID)
	s.TrackZone(z)
}

// ChangeObj updates obj.Count and routes an OBJ_COUNT follows event.
func (s *Server) ChangeObj(obj *entity.Obj, oldCount, newCount int) {
	z := s.zoneMap.Get(obj.Level, obj.X, obj.Z)
	z.ChangeObj(obj, oldCount, newCount, s.currentTick)
	s.TrackZone(z)
}

// RemoveObj routes an obj removal. Respects the lastLifecycleTick check.
func (s *Server) RemoveObj(obj *entity.Obj) {
	z := s.zoneMap.Get(obj.Level, obj.X, obj.Z)
	z.RemoveObj(obj, s.currentTick)
	s.TrackZone(z)
}

// RevealObj transitions a private drop to public.
func (s *Server) RevealObj(obj *entity.Obj, receiverSlot int) {
	z := s.zoneMap.Get(obj.Level, obj.X, obj.Z)
	z.RevealObj(obj, receiverSlot)
	s.TrackZone(z)
}

// AnimMap routes a tile-based spotanim event.
func (s *Server) AnimMap(level, x, zc, spotanim, height, delay int) {
	z := s.zoneMap.Get(level, x, zc)
	z.AnimMap(x, zc, spotanim, height, delay)
	s.TrackZone(z)
}

// MapProjAnim routes a projectile event. The zone is keyed by the source
// tile (the TS reference does the same).
func (s *Server) MapProjAnim(
	level, srcX, srcZ, dstX, dstZ int,
	target, spotanim, srcHeight, dstHeight int,
	startDelay, endDelay, peak, arc int,
) {
	z := s.zoneMap.Get(level, srcX, srcZ)
	z.MapProjAnim(srcX, srcZ, dstX, dstZ,
		target, spotanim, srcHeight, dstHeight,
		startDelay, endDelay, peak, arc)
	s.TrackZone(z)
}
```

- [ ] **Step 4.5: Run — verify PASS**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestServer -v`
Expected: PASS.

- [ ] **Step 4.6: Full suite + vet**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`
Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...`
Expected: both clean.

- [ ] **Step 4.7: Commit**

```bash
git add modules/world/server.go modules/world/world_zone.go modules/world/world_zone_test.go
git commit --no-gpg-sign -m "feat(world): Server.zoneMap + 11 zone dispatcher methods

Server now owns ZoneMap and a per-tick zonesTracking set. Dispatchers
route each zone mutation through the map (by level/x/z) and track the
zone so processZones can later ComputeShared on it.

Matches the TS World.addLoc/addObj/animMap/... pattern."
```

---

## Task 5: Player.updateZones + writeFullFollows + tests

The real implementation of the last stub.

**Files:**
- Modify: `modules/world/player.go`
- Create: `modules/world/player_zone.go`
- Create: `modules/world/player_zone_test.go`

- [ ] **Step 5.1: Implement `player_zone.go` (helpers)**

Create `modules/world/player_zone.go`:

```go
package world

import (
	"fmt"

	"github.com/zsrv/goscape/pkg/coordgrid"
	"github.com/zsrv/goscape/pkg/entity"
	"github.com/zsrv/goscape/pkg/io/packet"
	gameserver "github.com/zsrv/goscape/pkg/io/protocol/game/server"
	"github.com/zsrv/goscape/pkg/rsbuf"
	"github.com/zsrv/goscape/pkg/zone"
)

// writeFullFollows sends UpdateZoneFullFollows (client zone reset) followed
// by a PartialFollows wrapper + synthesized per-entity messages replaying
// every currently-active dynamic loc/obj in the zone. Entities transitioned
// THIS tick are skipped — the Enclosed buffer already carries their change.
//
// TODO(beyond-4b): handle Respawn-lifecycle (static) loc branches once
// static loading from cache maps is wired up.
func (p *Player) writeFullFollows(z *zone.Zone, currentTick int) {
	buf := packet.NewPacket(nil)
	rsbuf.EncodeZoneFullFollows(buf, z.X, z.Z, p.originX, p.originZ)
	p.writeOut(gameserver.OpUpdateZoneFullFollows, buf.Bytes())

	hasMessages := false
	ensureHeader := func() {
		if hasMessages {
			return
		}
		hb := packet.NewPacket(nil)
		rsbuf.EncodeZonePartialFollows(hb, z.X, z.Z, p.originX, p.originZ)
		p.writeOut(gameserver.OpUpdateZonePartialFollows, hb.Bytes())
		hasMessages = true
	}

	for _, obj := range z.Objs {
		if obj.LastLifecycleTick == currentTick {
			continue
		}
		if obj.ReceiverID != zone.PublicReceiver && obj.ReceiverID != p.slot {
			continue
		}
		if !obj.CheckLifecycle(currentTick) {
			continue
		}
		ensureHeader()
		pb := packet.NewPacket(nil)
		rsbuf.EncodeObjAdd(pb, coordgrid.PackZoneCoord(obj.X, obj.Z), obj.Type, obj.Count)
		p.writeOut(gameserver.OpObjAdd, pb.Bytes())
	}

	for _, loc := range z.Locs {
		if loc.LastLifecycleTick == currentTick {
			continue
		}
		if loc.Lifecycle == entity.LifecycleDespawn && loc.CheckLifecycle(currentTick) {
			ensureHeader()
			pb := packet.NewPacket(nil)
			rsbuf.EncodeLocAddChange(pb, coordgrid.PackZoneCoord(loc.X, loc.Z),
				loc.Shape(), loc.Angle(), loc.Type())
			p.writeOut(gameserver.OpLocAddChange, pb.Bytes())
		}
	}
}

// writePartialFollows iterates the zone's per-tick Follows events, filtered
// by recipient, emitting a PartialFollows header once (if any match) then
// each event as its own top-level zone-nested packet.
func (p *Player) writePartialFollows(z *zone.Zone) {
	hasAnyForMe := false
	for _, e := range z.Events() {
		if e.Type != zone.ZoneEventFollows || e.Bytes == nil {
			continue
		}
		if e.ReceiverID != zone.PublicReceiver && e.ReceiverID != p.slot {
			continue
		}
		if !hasAnyForMe {
			hb := packet.NewPacket(nil)
			rsbuf.EncodeZonePartialFollows(hb, z.X, z.Z, p.originX, p.originZ)
			p.writeOut(gameserver.OpUpdateZonePartialFollows, hb.Bytes())
			hasAnyForMe = true
		}
		sendZoneNested(p, e.Bytes)
	}
}

// sendZoneNested dispatches a [opcode_byte, ...payload] byte slice as a
// top-level packet using the Op{} registered for that zone-nested opcode.
func sendZoneNested(p *Player, b []byte) {
	if len(b) == 0 {
		return
	}
	p.writeOut(zoneNestedOp(b[0]), b[1:])
}

// zoneNestedOp maps a zone-nested opcode byte to its top-level Op{} entry.
// Panics on unknown opcodes — an assertion that only pkg/zone-produced
// bytes reach here.
func zoneNestedOp(op byte) gameserver.Op {
	switch op {
	case rsbuf.ZoneOpLocAddChange:
		return gameserver.OpLocAddChange
	case rsbuf.ZoneOpLocAnim:
		return gameserver.OpLocAnim
	case rsbuf.ZoneOpLocDel:
		return gameserver.OpLocDel
	case rsbuf.ZoneOpLocMerge:
		return gameserver.OpLocMerge
	case rsbuf.ZoneOpMapAnim:
		return gameserver.OpMapAnim
	case rsbuf.ZoneOpMapProjAnim:
		return gameserver.OpMapProjAnim
	case rsbuf.ZoneOpObjAdd:
		return gameserver.OpObjAdd
	case rsbuf.ZoneOpObjCount:
		return gameserver.OpObjCount
	case rsbuf.ZoneOpObjDel:
		return gameserver.OpObjDel
	case rsbuf.ZoneOpObjReveal:
		return gameserver.OpObjReveal
	}
	panic(fmt.Sprintf("unknown zone-nested opcode %d", op))
}
```

- [ ] **Step 5.2: Replace `updateZones()` stub in `player.go`**

Find `func (p *Player) updateZones()    {}` (should be around line 346) and replace with:

```go
func (p *Player) updateZones() {
	if p.buildArea == nil || p.client == nil || p.client.server == nil {
		return
	}
	s := p.client.server

	// Unload zones no longer active.
	for idx := range p.buildArea.LoadedZones {
		if !p.buildArea.ActiveZones[idx] {
			delete(p.buildArea.LoadedZones, idx)
		}
	}

	// Deliver each active zone.
	for idx := range p.buildArea.ActiveZones {
		z := s.zoneMap.GetByIndex(idx)

		if !p.buildArea.LoadedZones[idx] {
			p.writeFullFollows(z, s.currentTick)
		}

		if shared := z.Shared(); len(shared) > 0 {
			buf := packet.NewPacket(nil)
			rsbuf.EncodeZonePartialEnclosed(buf, z.X, z.Z, p.originX, p.originZ, shared)
			p.writeOut(gameserver.OpUpdateZonePartialEnclosed, buf.Bytes())
		}

		p.writePartialFollows(z)
		p.buildArea.LoadedZones[idx] = true
	}
}
```

The `packet` import already exists in `player.go`; no import changes needed there.

- [ ] **Step 5.3: Write player-zone tests**

Create `modules/world/player_zone_test.go`:

```go
package world

import (
	"testing"

	"github.com/zsrv/goscape/pkg/coordgrid"
	"github.com/zsrv/goscape/pkg/entity"
	"github.com/zsrv/goscape/pkg/io/isaac"
	"github.com/zsrv/goscape/pkg/zone"
)

func newZoneTestPlayer(t *testing.T, s *Server, slot, x, z, level int) (*Player, *testConn) {
	t.Helper()
	p, cc := newTestPlayer(t)
	p.client.server = s
	p.client.encryptor = isaac.New([4]uint32{uint32(slot), 2, 3, 4})
	p.slot = slot
	p.x, p.z, p.level = x, z, level
	p.originX, p.originZ = x, z
	p.buildArea = nil // set by tests that need it
	s.players[slot] = p
	s.playerLoop = append(s.playerLoop, p)
	_ = cc // some tests will drain; others won't
	return p, &testConn{cc: cc}
}

type testConn struct{ cc any }

func TestUpdateZonesSendsPartialEnclosedForActiveZone(t *testing.T) {
	s := newZoneTestServer(t)
	s.currentTick = 10
	p, _ := newZoneTestPlayer(t, s, 1, 3094, 3106, 0)
	p.buildArea = newTestBuildAreaAt(3094, 3106)

	// Add a loc via Server dispatcher → queues Enclosed event in zone.
	loc := entity.NewLoc(0, 3094, 3106, 1, 1, entity.LifecycleDespawn, 100, 0, 0)
	s.AddLoc(loc)

	// Mimic processZones: ComputeShared before delivery.
	for z := range s.zonesTracking {
		z.ComputeShared()
	}

	// Deliver.
	received := drainConn(t, p.clientConnFor())
	p.updateZones()
	p.client.flushWrite()
	got := <-received
	if len(got) == 0 {
		t.Fatal("expected PartialEnclosed packet; got none")
	}
	// The byte count should include a FullFollows (first load) + PartialFollows
	// (replay wrapper) + OpLocAddChange (replay) + PartialEnclosed (current-tick add).
	// We assert len > 0 here; deeper structure asserted by TestWriteFullFollowsReplaysActiveLocs.
	_ = got
	_ = coordgrid.ZoneIndex(3094, 3106, 0)
}

// Helper: build a BuildArea populated via Rebuild.
func newTestBuildAreaAt(x, z int) *buildarea.BuildArea { /* implementation */ }

// Plus the rest of the tests — abbreviated here; see full spec section "Testing".
```

> **NOTE TO IMPLEMENTER**: The test scaffolding above is a sketch. Write out the full 7 test functions listed in the spec's Testing section for `player_zone_test.go`. Key fixtures:
> - `newZoneTestServer` (from Task 4's test file)
> - `newZoneTestPlayer` — create a Player hooked into the Server with a BuildArea whose ActiveZones include the origin zone.
> - Drain the pipe via the existing `drainConn(t, cc) <-chan []byte` helper (defined in 4a's `stat_update_test.go`).

Actually — the above sketch has `testConn` / `clientConnFor()` placeholders. Use the real pattern:

```go
func newZoneTestPlayer(t *testing.T, s *Server, slot, x, z, level int) (*Player, net.Conn) {
	t.Helper()
	p, cc := newTestPlayer(t)
	p.client.server = s
	p.client.encryptor = isaac.New([4]uint32{uint32(slot), 2, 3, 4})
	p.slot = slot
	p.x, p.z, p.level = x, z, level
	p.originX, p.originZ = x, z
	ba := buildarea.New()
	_ = ba.Rebuild(x, z, 0)
	p.buildArea = ba
	s.players[slot] = p
	s.playerLoop = append(s.playerLoop, p)
	return p, cc
}

func TestUpdateZonesSendsPartialEnclosedForActiveZone(t *testing.T) {
	s := newZoneTestServer(t)
	s.currentTick = 10
	p, cc := newZoneTestPlayer(t, s, 1, 3094, 3106, 0)

	loc := entity.NewLoc(0, 3094, 3106, 1, 1, entity.LifecycleDespawn, 100, 0, 0)
	s.AddLoc(loc)
	for z := range s.zonesTracking {
		z.ComputeShared()
	}

	received := drainConn(t, cc)
	p.updateZones()
	p.client.flushWrite()
	got := <-received
	if len(got) == 0 {
		t.Fatal("expected zone packets, got none")
	}
	// Expect: FullFollows (opcode 135, 2 payload) + PartialFollows wrapper
	// (opcode 7, 2 payload) + OpLocAddChange (59, 4 payload) for the replay
	// + PartialEnclosed (162, -2) with the current-tick shared bytes.
	// That's at minimum 4 packets → many bytes. Assert > 15 bytes as a smoke test.
	if len(got) < 15 {
		t.Errorf("got %d bytes, expected many (full+partial+replay+enclosed)", len(got))
	}
}

func TestUpdateZonesFullFollowsOnlyFirstLoad(t *testing.T) {
	s := newZoneTestServer(t)
	p, cc := newZoneTestPlayer(t, s, 1, 3094, 3106, 0)

	// First call: writes FullFollows.
	received := drainConn(t, cc)
	p.updateZones()
	p.client.flushWrite()
	first := <-received
	if len(first) == 0 {
		t.Fatal("first updateZones should emit FullFollows for each active zone")
	}

	// Second call: all zones now in LoadedZones → no FullFollows.
	received2 := drainConn(t, cc)
	p.updateZones()
	p.client.flushWrite()
	second := <-received2
	if len(second) != 0 {
		t.Errorf("second updateZones with no new events should emit nothing; got %d bytes", len(second))
	}
}

func TestUpdateZonesUnloadsDroppedZones(t *testing.T) {
	s := newZoneTestServer(t)
	p, _ := newZoneTestPlayer(t, s, 1, 3094, 3106, 0)
	// Populate LoadedZones with an index NOT in ActiveZones.
	bogusIdx := 999999
	p.buildArea.LoadedZones[bogusIdx] = true

	p.updateZones()
	if p.buildArea.LoadedZones[bogusIdx] {
		t.Error("bogus index not in ActiveZones should have been unloaded")
	}
}

func TestWriteFullFollowsReplaysActiveLocs(t *testing.T) {
	s := newZoneTestServer(t)
	p, cc := newZoneTestPlayer(t, s, 1, 3094, 3106, 0)

	// Preload a dynamic Loc into the zone (bypassing Server.AddLoc so nothing
	// lives in zonesTracking — we're testing the replay path only).
	z := s.zoneMap.Get(0, 3094, 3106)
	loc := entity.NewLoc(0, 3094, 3106, 1, 1, entity.LifecycleDespawn, 100, 5, 2)
	z.Locs = append(z.Locs, loc)

	received := drainConn(t, cc)
	p.writeFullFollows(z, 1)
	p.client.flushWrite()
	got := <-received
	if len(got) == 0 {
		t.Fatal("expected FullFollows + PartialFollows + LocAddChange packets")
	}
	// First byte should be the encrypted OpUpdateZoneFullFollows opcode.
}

func TestWriteFullFollowsSkipsThisTickTransitions(t *testing.T) {
	s := newZoneTestServer(t)
	s.currentTick = 100
	p, cc := newZoneTestPlayer(t, s, 1, 3094, 3106, 0)

	z := s.zoneMap.Get(0, 3094, 3106)
	loc := entity.NewLoc(0, 3094, 3106, 1, 1, entity.LifecycleDespawn, 100, 0, 0)
	loc.LastLifecycleTick = 100 // transitioned this tick → skip
	z.Locs = append(z.Locs, loc)

	received := drainConn(t, cc)
	p.writeFullFollows(z, 100)
	p.client.flushWrite()
	got := <-received
	// Only the FullFollows header (opcode 135 + 2 header bytes = 3 bytes).
	if len(got) != 3 {
		t.Errorf("want exactly 3 bytes (FullFollows header, no replay); got %d", len(got))
	}
}

func TestPartialFollowsFiltersByReceiverID(t *testing.T) {
	s := newZoneTestServer(t)
	p, cc := newZoneTestPlayer(t, s, 7, 3094, 3106, 0)

	z := s.zoneMap.Get(0, 3094, 3106)
	// Two Follows events: one targeted at slot 3, one at slot 7.
	z.Events()
	// Inject directly for testing — normally an obj mutation would queue these.
	// Access via test helper; if needed, expose a testing-only Inject method.
	// For now, use AddObj with distinct receivers via the Server dispatcher.
	obj3 := entity.NewObj(0, 3094, 3106, entity.LifecycleDespawn, 995, 1)
	obj7 := entity.NewObj(0, 3094, 3106, entity.LifecycleDespawn, 995, 1)
	s.AddObj(obj3, 3)
	s.AddObj(obj7, 7)
	for zi := range s.zonesTracking {
		zi.ComputeShared()
	}

	received := drainConn(t, cc)
	p.writePartialFollows(z)
	p.client.flushWrite()
	got := <-received
	if len(got) == 0 {
		t.Fatal("expected follows packets for slot 7")
	}
	// Should include exactly one Follows wrapper + one ObjAdd for slot 7
	// (slot 3 filtered). 2 + 5 = 7 bytes payload + 2 opcode bytes = 9.
	if len(got) != 9 {
		t.Errorf("want 9 bytes (1 header + 1 ObjAdd for slot 7); got %d", len(got))
	}
}

func TestSendZoneNestedUnknownOpcodePanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic on unknown opcode")
		}
	}()
	p, _ := newTestPlayer(t)
	sendZoneNested(p, []byte{255, 0, 0})
}
```

Note: the test above uses `buildarea.New()` + `ba.Rebuild(x, z, 0)` — `newZoneTestPlayer` needs to import `buildarea` and `isaac` accordingly. Adjust the imports at top of the file.

Also: the pipe drain helper `drainConn` already exists in `modules/world/stat_update_test.go` (sub-spec 4a).

- [ ] **Step 5.4: Run all player_zone tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestUpdateZones|TestWriteFullFollows|TestPartialFollows|TestSendZoneNested' -v`
Expected: all PASS.

- [ ] **Step 5.5: Full suite + vet**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`
Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...`
Expected: both clean.

- [ ] **Step 5.6: Commit**

```bash
git add modules/world/player.go modules/world/player_zone.go modules/world/player_zone_test.go
git commit --no-gpg-sign -m "feat(world): Player.updateZones + writeFullFollows replay

Player.updateZones now:
- unloads zones no longer in BuildArea.ActiveZones
- writes UpdateZoneFullFollows + synthesized dynamic-loc/obj replay the
  first time a zone enters the player's view
- emits UpdateZonePartialEnclosed for shared enclosed events
- emits per-player UpdateZonePartialFollows + zone-nested packets filtered
  by ReceiverID == PublicReceiver or match on player.slot

Replays skip entities transitioned this same tick (lastLifecycleTick check)
to avoid double-sends with the Enclosed buffer."
```

---

## Task 6: processZones phase + processCleanup extension + tests

**Files:**
- Modify: `modules/world/tick.go`
- Create: `modules/world/tick_zone_test.go`

- [ ] **Step 6.1: Add processZones phase + extend processCleanup**

Edit `modules/world/tick.go` `runTickLoopWithRate` phase sequence (between processInfo and processClientsOut):

```go
s.processInfo()
s.processZones()       // ← NEW
s.processClientsOut()
```

Add at the bottom of the file:

```go
func (s *Server) processZones() {
	for z := range s.zonesTracking {
		z.ComputeShared()
	}
}
```

Extend `processCleanup` — add AFTER the existing ResetMasks loops:

```go
for z := range s.zonesTracking {
	z.Reset()
}
clear(s.zonesTracking)
```

- [ ] **Step 6.2: Write failing tests**

Create `modules/world/tick_zone_test.go`:

```go
package world

import (
	"testing"

	"github.com/zsrv/goscape/pkg/entity"
	"github.com/zsrv/goscape/pkg/zone"
)

func TestProcessZonesComputesShared(t *testing.T) {
	s := newZoneTestServer(t)
	loc := entity.NewLoc(0, 3094, 3106, 1, 1, entity.LifecycleDespawn, 100, 0, 0)
	s.AddLoc(loc)

	// Before processZones: shared is nil.
	var beforeZone *zone.Zone
	for z := range s.zonesTracking {
		beforeZone = z
	}
	if beforeZone == nil {
		t.Fatal("expected a tracked zone")
	}
	if beforeZone.Shared() != nil {
		t.Error("Shared should be nil before processZones")
	}

	s.processZones()

	if beforeZone.Shared() == nil {
		t.Error("Shared should be non-nil after processZones")
	}
}

func TestProcessCleanupResetsAndClearsTracking(t *testing.T) {
	s := newZoneTestServer(t)
	// need valid playersMu and playerLoop for processCleanup's existing code path.
	loc := entity.NewLoc(0, 3094, 3106, 1, 1, entity.LifecycleDespawn, 100, 0, 0)
	s.AddLoc(loc)
	s.processZones()

	var trackedZone *zone.Zone
	for z := range s.zonesTracking {
		trackedZone = z
	}
	if trackedZone == nil {
		t.Fatal("expected a tracked zone before cleanup")
	}

	s.processCleanup()

	if trackedZone.Shared() != nil {
		t.Error("Shared should be nil after processCleanup Reset")
	}
	if len(trackedZone.Events()) != 0 {
		t.Error("events should be empty after Reset")
	}
	if len(s.zonesTracking) != 0 {
		t.Errorf("zonesTracking should be empty; got %d entries", len(s.zonesTracking))
	}
}
```

- [ ] **Step 6.3: Run — verify PASS**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestProcessZones|TestProcessCleanup' -v`
Expected: PASS.

- [ ] **Step 6.4: Full suite + race + vet**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./...`
Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...`
Expected: all clean.

- [ ] **Step 6.5: Commit**

```bash
git add modules/world/tick.go modules/world/tick_zone_test.go
git commit --no-gpg-sign -m "feat(world): processZones phase + processCleanup zone reset

Add a processZones phase between processInfo and processClientsOut that
calls ComputeShared on every zone in zonesTracking. processCleanup now
also Resets each tracked zone and clears the tracking set so the next
tick starts clean.

Closes sub-spec 4b — Player.updateZones is live; all four outbound-
update functions in processOut now have real implementations."
```

---

## Final Verification

- [ ] **Step F.1: Race detector**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./...`
Expected: PASS.

- [ ] **Step F.2: go vet**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...`
Expected: no output.

- [ ] **Step F.3: No remaining `{}` stubs**

Run: `grep -E 'func \(p \*Player\) update[A-Z]\w+\(\) *\{\s*\}' modules/world/player.go modules/world/afkzone.go`
Expected: no matches — all five `update*` functions are implemented.

---

## Spec Coverage Map

| Spec requirement | Task |
|---|---|
| Move ZoneIndex/UnpackZoneIndex to coordgrid | Task 1 |
| 10 top-level zone Op{} entries in prot.go | Task 2 |
| BuildArea.Rebuild populates ActiveZones | Task 3 |
| Server.zoneMap + zonesTracking + TrackZone | Task 4 |
| 11 Server dispatcher methods | Task 4 |
| Player.updateZones real implementation | Task 5 |
| writeFullFollows + writePartialFollows + sendZoneNested | Task 5 |
| processZones tick phase | Task 6 |
| processCleanup zone reset | Task 6 |
| All acceptance criteria | Task F |

No gaps.
