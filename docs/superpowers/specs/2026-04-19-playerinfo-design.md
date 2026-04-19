# Sub-Spec 3b: PlayerInfo Encoder

**Date:** 2026-04-19
**Project:** goscape — Go rewrite of LostCityRS/Engine-TS
**Scope:** Second third of the original sub-spec 3. Delivers a pure-Go port of the PlayerInfo bitstream encoder from the `@2004scape/rsbuf` Rust/WASM crate (branch `225`). After this sub-spec, two players can see each other on screen, see each other move, and `::say hello` produces a chat bubble visible to everyone nearby.

---

## Context

The TS reference server delegates all player-info bitstream encoding to the `@2004scape/rsbuf` Rust crate compiled to WASM. goscape has no runtime WASM or cgo, so this work is a pure-Go port targeting the **`225` branch** (matches the game revision we serve).

Sub-spec 3a delivered the map delivery foundation: `ObjType`, `InvType`, `Inventory`, `BuildArea`, `RebuildNormal`, `UpdateInvFull`, and the appearance generator. `Player.masks`, `entitymask`, `appearanceBuf`, and `lastAppearance` fields exist; the mask constants in `modules/world/masks.go` already match the 225 branch (APPEARANCE=1, ANIM=2, FACE_ENTITY=4, SAY=8, DAMAGE=16, FACE_COORD=32, CHAT=64, BIG=128, SPOT_ANIM=256, EXACT_MOVE=512).

Sub-spec 3c (next) covers NPC entity instantiation + NpcInfo encoding.

---

## File Map

| File | Action | Responsibility |
|------|--------|----------------|
| `pkg/io/packet/alt.go` | **New** | Alt byte writers: `P1Alt1`, `P1Alt2`, `P1Alt3`, `P2Alt2`, `P4Alt2`, `IP2`, `PDataAlt1`, `PDataAlt2` |
| `pkg/io/packet/alt_test.go` | **New** | Round-trip each alt writer vs hand-computed bytes |
| `pkg/io/packet/packetbit_audit_test.go` | **New** | Confirm `PacketBit` MSB-first bit ordering matches rsbuf's `pbit` |
| `pkg/grid/grid.go` | **New** | Zone-grid spatial lookup: `Add`, `Remove`, `NearbyPlayers` |
| `pkg/grid/grid_test.go` | **New** | Grid add/remove/lookup tests |
| `pkg/buildarea/buildarea.go` | Modify | Add `Players map[int]struct{}`, `Appearance map[int]uint64`, `HasAppearance`, `RecordAppearance` |
| `pkg/buildarea/buildarea_test.go` | Modify | Tests for the new tracked-players + appearance-hash features |
| `pkg/rsbuf/visibility.go` | **New** | `Visibility` enum (`Default`, `Soft`, `Hard`) |
| `pkg/rsbuf/source.go` | **New** | `PlayerSource` interface — scalar accessor surface for all Player state |
| `pkg/rsbuf/mask_payload.go` | **New** | 9 mask-payload byte encoders + fixed-order writer |
| `pkg/rsbuf/renderer.go` | **New** | `Renderer` with per-slot high-def/low-def byte caches; `ComputePlayers` |
| `pkg/rsbuf/playerinfo.go` | **New** | `Encode(self, all, ba, grid, renderer) []byte` — 4-phase PlayerInfo packet |
| `pkg/rsbuf/*_test.go` | New | Golden-byte tests for mask payloads, encoder, renderer |
| `pkg/io/protocol/game/server/prot.go` | Modify | Add `OpPlayerInfo = Op{184, -2}` |
| `modules/world/player.go` | Modify | Add `visibility`, `active`, and 10 mask-state field groups |
| `modules/world/player_source.go` | **New** | `*Player` method set satisfying `rsbuf.PlayerSource` |
| `modules/world/player_masks.go` | **New** | Setter methods: `Animate`, `Say`, `Chat`, `ShowHit`, `SpotAnim`, `ExactMove`, `FaceCoord`, `FaceEntity`, `ChangeAppearance`, `ResetMasks` |
| `modules/world/player_info.go` | **New** | Fill in `updatePlayers()` — calls `rsbuf.Encode`, writes `OpPlayerInfo` |
| `modules/world/server.go` | Modify | Add `renderer *rsbuf.Renderer`, `grid *grid.Grid`; initialise in `NewServer`; set `p.active=true` in `addPlayer`, false in `removePlayer` |
| `modules/world/tick.go` | Modify | Add `processInfo` (grid updates + `ComputePlayers`) and `processCleanup` (`ResetMasks` + mask clear) phases |
| `modules/world/handlers_game.go` | Modify | Register `gameHandlers[4] = handleClientCheat` — parses `::say <msg>` → `p.Say(msg)` |

---

## Shared Packages

### `pkg/io/packet/alt.go`

Exact byte transforms used by Jagex's scrambled RS2 protocol. Verify each against `rsbuf/src/packet.rs` 225 during implementation.

```go
func (p *Packet) P1Alt1(v uint8)   { p.P1(v + 128) }
func (p *Packet) P1Alt2(v uint8)   { p.P1(128 - v) }
func (p *Packet) P1Alt3(v int8)    { p.P1(uint8(-v)) }      // two's-complement negation
func (p *Packet) P2Alt2(v uint16)  { p.P1(byte(v)); p.P1(byte(v >> 8)) } // little-endian
func (p *Packet) P4Alt2(v uint32)  { p.P1(byte(v >> 16)); p.P1(byte(v >> 24)); p.P1(byte(v)); p.P1(byte(v >> 8)) } // middle-endian
func (p *Packet) IP2(v uint16)     { p.P2Alt2(v) }
func (p *Packet) PDataAlt1(b []byte) { for _, x := range b { p.P1(x + 128) } }
func (p *Packet) PDataAlt2(b []byte) { for _, x := range b { p.P1(128 - x) } }
```

### `pkg/io/packet/packetbit_audit_test.go`

goscape already has `pkg/io/packet.PacketBit` (sub-spec 1). The audit test verifies its output against rsbuf's `pbit` for a known sequence. If the bit ordering diverges (e.g., LSB-first), the audit fails and the test also covers the fix direction: rewrite `PacketBit` to write MSB-first across byte boundaries.

### `pkg/grid/grid.go`

```go
type Grid struct {
    zones map[uint32][]int  // packed zone coord -> player slots
}

// Packed key: (level & 0x3) << 22 | (zoneX & 0x7FF) << 11 | (zoneZ & 0x7FF)
func packZone(x, z, level int) uint32

func New() *Grid
func (g *Grid) Add(slot, x, z, level int)
func (g *Grid) Remove(slot, x, z, level int)

// NearbyPlayers returns slots whose zone is within zoneRadius zones of the
// given coord (Chebyshev distance in zone units). level must match exactly.
func (g *Grid) NearbyPlayers(x, z, level, zoneRadius int) []int
```

Called from `processInfo` each tick: when a player's `(x>>3, z>>3, level)` differs from their previous-tick zone, `Remove(oldZone)` + `Add(newZone)`. Previous-tick zone derived from `lastTickX/Z/Level` which sub-spec 2's `resolveMovement` maintains.

### `pkg/buildarea` additions

Extends the existing `BuildArea` struct:

```go
type BuildArea struct {
    // existing (sub-spec 3a): OriginX, OriginZ, LastBuild, LoadedZones,
    //                         ActiveZones, Mapsquares

    Players    map[int]struct{}  // player slots currently tracked by this client
    Appearance map[int]uint64    // slot -> hash of last APPEARANCE bytes sent
}

func (ba *BuildArea) HasAppearance(slot int, hash uint64) bool
func (ba *BuildArea) RecordAppearance(slot int, hash uint64)
```

`New()` initialises both maps to empty.

### `pkg/rsbuf/visibility.go`

```go
type Visibility int
const (
    VisibilityDefault Visibility = iota
    VisibilitySoft
    VisibilityHard
)
```

### `pkg/rsbuf/source.go` — `PlayerSource` interface

The encoder reads player state through an interface so `pkg/rsbuf` has no dependency on `modules/world.Player`. Accessor methods return zero values when the corresponding mask bit isn't set.

```go
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

    // mask payload accessors (9 masks)
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

    // movement (for own-player block + other-players delta)
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
```

---

## Player Struct Extensions

Add to `modules/world/player.go`:

```go
// === visibility + active flag (sub-spec 3b) ===
visibility Visibility // placeholder type; see below
active     bool       // false during login/logout/teleport

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

faceEntity int // -1 when unset; player: slot + 0x8000, npc: slot
faceSquareX, faceSquareZ int // fine-grained face coord
```

`Visibility` in `modules/world` is a type alias over `rsbuf.Visibility` so the Player struct can reference it without importing rsbuf recursively. `modules/world/masks.go` gets:

```go
type Visibility = rsbuf.Visibility
```

**Defaults set in `newPlayer`:**

```go
animID:         -1,
animDelay:      -1,
faceEntity:     -1,
faceSquareX:    -1,
faceSquareZ:    -1,
visibility:     VisibilityDefault, // = rsbuf.VisibilityDefault
active:         false,
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
```

### `modules/world/player_source.go` — satisfy `rsbuf.PlayerSource`

Accessor methods for every `PlayerSource` method, each returning the corresponding field. Example:

```go
func (p *Player) Masks() int                  { return p.masks }
func (p *Player) EntityMask() int             { return p.entitymask }
func (p *Player) AppearanceBytes() []byte     { return p.appearanceBuf }
func (p *Player) AppearanceHash() uint64      { return appearanceHash(p.appearanceBuf) }
// ... etc
```

`appearanceHash(buf []byte) uint64` uses `hash/fnv` — a cheap, stable, non-cryptographic hash suitable for comparing whether a player's appearance has changed since the last send.

### `modules/world/player_masks.go` — setter methods

Each setter stores the payload state and raises the corresponding mask bit.

```go
func (p *Player) Animate(id, delay int) {
    p.animID = id
    p.animDelay = delay
    p.masks |= MaskAnim
}

func (p *Player) Say(msg []byte) {
    p.sayText = msg
    p.masks |= MaskSay
}

func (p *Player) Chat(colour, effect, rights int, msg []byte) {
    p.chatColour = colour
    p.chatEffect = effect
    p.chatRights = rights
    p.chatBytes = msg
    p.masks |= MaskChat
}

func (p *Player) ShowHit(amount, dmgType, cur, base int) {
    p.damageAmt = amount
    p.damageType = dmgType
    p.curHP = cur
    p.baseHP = base
    p.masks |= MaskDamage
}

func (p *Player) SpotAnim(id, height, delay int) {
    p.spotanimID = id
    p.spotanimHeight = height
    p.spotanimDelay = delay
    p.masks |= MaskSpotAnim
}

func (p *Player) ExactMove(sX, sZ, eX, eZ, begin, finish, dir int) {
    p.exactStartX = sX
    p.exactStartZ = sZ
    p.exactEndX = eX
    p.exactEndZ = eZ
    p.exactBegin = begin
    p.exactFinish = finish
    p.exactDir = dir
    p.masks |= MaskExactMove
}

func (p *Player) FaceCoord(x, z int) {
    p.faceSquareX = x*2 + 1
    p.faceSquareZ = z*2 + 1
    p.masks |= MaskFaceCoord
}

func (p *Player) FaceEntity(entityIndex int) {
    p.faceEntity = entityIndex
    p.masks |= MaskFaceEntity
}

func (p *Player) ChangeAppearance() {
    p.generateAppearance(p.client.server.objTypes, p.client.server.invTypes, p.client.server.currentTick)
    p.masks |= MaskAppearance
}

// ResetMasks clears all mask bits and ephemeral mask state for the next tick.
// Persistent fields (animID for looping anims, faceEntity target) are retained.
func (p *Player) ResetMasks() {
    p.masks = 0
    p.sayText = nil
    p.chatBytes = nil
    p.damageAmt = -1
    p.damageType = -1
    p.curHP = -1
    p.baseHP = -1
    p.spotanimID = -1
    // Persistent: animID, faceEntity, faceSquareX/Z — kept so newly-added
    // observers still see them.
}
```

---

## Encoder Flow

### `pkg/rsbuf.Encode(self, all, ba, grid, renderer) []byte`

Produces the full PlayerInfo payload (no opcode / length prefix).

**Four phases, via `PacketBit`:**

#### Phase 1 — own-player block

```
if self.Tele():
    pbit(1,1); pbit(2,3); pbit(1,jump); pbit(2,level);
    pbit(7,localZ); pbit(7,localX); pbit(1,extend)
elif self.RunDir() != -1:
    pbit(1,1); pbit(2,2); pbit(3,walkDir); pbit(3,runDir); pbit(1,extend)
elif self.WalkDir() != -1:
    pbit(1,1); pbit(2,1); pbit(3,walkDir); pbit(1,extend)
elif self.Masks() != 0:
    pbit(1,1); pbit(2,0); extend=1
else:
    pbit(1,0)
```

`localX` = `self.x - (((self.originX >> 3) - 6) << 3)` (0..103). `localZ` similar. `extend=1` when `Masks() != 0` and there's room in the 4997-byte budget.

If `extend=1`: append `renderer.HighDefOf(self.Slot())` to the mask-updates buffer.

#### Phase 2 — other-players delta

```
pbit(8, len(ba.Players))
for slot in ba.Players:
    other := all[slot]
    if slot == -1 || other.Tele() || other.Level() != self.Level() ||
       !other.Active() || other.Visibility() == VisibilityHard ||
       zoneDistance(other, self) > 15:
       // (+ SOFT visibility check: observer.StaffModLevel() < 1)
        pbit(1,1); pbit(2,3)  // remove
        delete(ba.Players, slot)
        continue

    if other.RunDir() != -1:
        pbit(1,1); pbit(2,2); pbit(3,walkDir); pbit(3,runDir); pbit(1,extend)
    elif other.WalkDir() != -1:
        pbit(1,1); pbit(2,1); pbit(3,walkDir); pbit(1,extend)
    elif other.Masks() != 0:
        pbit(1,1); pbit(2,0); extend=1
    else:
        pbit(1,0)

    if extend: append renderer.HighDefOf(slot) to updates
```

Suppress CHAT in the high-def payload when `slot == self.Slot()` (player doesn't see their own bubble via this path — but phase 1 already handled self; still belt-and-suspenders).

#### Phase 3 — new-players loop

```
candidates := grid.NearbyPlayers(self.x, self.z, self.level, 15)
for slot in candidates:
    if slot == self.Slot() || slot in ba.Players: continue
    if len(ba.Players) >= PREFERREDPLAYERS (255): break

    other := all[slot]
    if !other.Active() || other.Visibility() == VisibilityHard: continue

    dx := clamp(other.x - self.x, -15, 15)   // biased to 5 bits
    dz := clamp(other.z - self.z, -15, 15)
    pbit(11, slot); pbit(5, dz); pbit(1, 1); pbit(1, other.Jump()); pbit(5, dx)

    ba.Players.add(slot)

    if ba.HasAppearance(slot, other.AppearanceHash()):
        append renderer.LowDefNoAppOf(slot) to updates    // no APPEARANCE payload
    else:
        append renderer.LowDefFullOf(slot) to updates     // includes APPEARANCE
        ba.RecordAppearance(slot, other.AppearanceHash())
```

#### Phase 4 — terminator + updates

```
if len(updates) > 0:
    pbit(11, 2047)  // sentinel
switch PacketBit to byte mode
append updates bytes
```

### Mask-header byte

Written at the start of each player's payload block (by the renderer when building high-def / low-def byte slices):

```go
if masks > 0xff {
    updates.IP2(uint16(masks | MaskBig))  // little-endian
} else {
    updates.P1(uint8(masks))
}
```

### Mask-payload byte layouts (225 branch)

Written in fixed order: **ANIM → SAY → EXACT_MOVE → FACE_ENTITY → FACE_COORD → SPOT_ANIM → APPEARANCE → DAMAGE → CHAT**.

| Mask | Value | Bytes | Layout |
|------|-------|-------|--------|
| ANIM | 2 | 3 | `p2(animID) p1_alt3(animDelay)` |
| SAY | 8 | 1+N | `pjstr(sayText, terminator=10)` |
| EXACT_MOVE | 512 | 9 | `p1_alt1(localStartX) p1_alt2(localStartZ) p1_alt3(localEndX) p1(localEndZ) p2(begin) p2_alt2(finish) p1(dir)` — local = minus `((originX>>3)-6)<<3` |
| FACE_ENTITY | 4 | 2 | `p2_alt2(faceEntity)` |
| FACE_COORD | 32 | 4 | `p2(faceSquareX) p2(faceSquareZ)` |
| SPOT_ANIM | 256 | 6 | `p2_alt2(spotAnimID) p4_alt2((spotHeight<<16) \| spotDelay)` |
| APPEARANCE | 1 | 1+N | `p1(len(appearanceBuf)) pdata_alt1(appearanceBuf)` |
| DAMAGE | 16 | 4 | `p1_alt1(damageAmt) p1_alt3(damageType) p1_alt2(curHP) p1(baseHP)` |
| CHAT | 64 | 4+N | `p1(chatColour) p1(chatEffect) p1_alt2(chatRights) p1_alt1(len(chatBytes)) pdata_alt2(chatBytes)` |

**`pjstr(data, terminator)`**: writes the bytes then the terminator byte (10 = line-feed in this codec).

---

## Rendering Pipeline

### `Renderer.ComputePlayers(all []PlayerSource)`

Called once per tick in `processInfo`, before any `updatePlayers()` runs.

```
for p in all:
    if p.Masks() == 0 && p.EntityMask() == 0:
        highDef[p.Slot()] = nil
        lowDefFull[p.Slot()] = nil
        lowDefNoApp[p.Slot()] = nil
        continue

    // High-def: mask payloads for p's current mask bits (what existing observers see).
    highDef[p.Slot()] = buildMaskPayloadBytes(p, p.Masks(), suppressChat=true)

    // Low-def variants for new observers.
    fullMasks := p.Masks() | MaskAppearance | MaskFaceCoord
    noAppMasks := (p.Masks() | MaskFaceCoord) & ^MaskAppearance
    lowDefFull[p.Slot()]  = buildMaskPayloadBytes(p, fullMasks, suppressChat=true)
    lowDefNoApp[p.Slot()] = buildMaskPayloadBytes(p, noAppMasks, suppressChat=true)
```

`buildMaskPayloadBytes` writes the mask header (P1 or IP2 with MaskBig) + all payloads in fixed order.

The encoder reads `renderer.HighDefOf(slot)` / `LowDefOf(slot)` with O(1) lookup.

**Low-def appearance caching.** The renderer caches **two** low-def variants per slot:
- `lowDefFull` — masks include forced APPEARANCE + FACE_COORD (sent when observer has not seen this appearance before)
- `lowDefNoApp` — masks include forced FACE_COORD but NOT APPEARANCE (sent when observer's `ba.HasAppearance(slot, hash)` returns true)

The encoder picks based on the HasAppearance check. This avoids per-observer byte-splicing at encode time; memory cost is 3 byte slices per player per tick (high + 2 low variants).

**CHAT suppression**: when building the own-player's high-def payload, the encoder strips the CHAT mask (player doesn't see their own chat bubble). Handled by checking `slot == self.Slot()` at encode time — the cache stores the full payload; the encoder skips the CHAT portion.

### Fit budget

The 4997-byte rsbuf limit applies to the entire PlayerInfo packet. Before appending a mask payload, the encoder calls `fits(bitPos, nextPayloadBytes)`:

```go
func fits(bitsWrittenSoFar, updatesBytesSoFar, nextPayloadBytes int) bool {
    totalBytes := ((bitsWrittenSoFar + 7) >> 3) + updatesBytesSoFar + nextPayloadBytes
    return totalBytes <= 4997
}
```

If a mask payload would push past the limit, `extend=0` for that player on this tick (they'll get their mask payload next tick).

---

## Tick Integration

### New `processInfo` phase

Insert into `runTickLoopWithRate` between `processLogins` and `processClientsOut`:

```go
s.processInfo()
```

Implementation:

```go
func (s *Server) processInfo() {
    s.playersMu.RLock()
    players := make([]*Player, len(s.playerLoop))
    copy(players, s.playerLoop)
    s.playersMu.RUnlock()

    // 1. Update grid for any player that moved zones this tick.
    for _, p := range players {
        prevZX, prevZZ, prevLevel := p.lastTickX>>3, p.lastTickZ>>3, p.lastLevel
        curZX, curZZ, curLevel := p.x>>3, p.z>>3, p.level
        if prevZX != curZX || prevZZ != curZZ || prevLevel != curLevel {
            if p.lastTickX >= 0 {
                s.grid.Remove(p.slot, p.lastTickX, p.lastTickZ, p.lastLevel)
            }
            s.grid.Add(p.slot, p.x, p.z, p.level)
        }
    }

    // 2. Build PlayerSource list.
    sources := make([]rsbuf.PlayerSource, len(players))
    for i, p := range players {
        sources[i] = p
    }

    // 3. Renderer pre-pass.
    s.renderer.ComputePlayers(sources)
}
```

### New `processCleanup` phase

Insert after `processClientsOut` (so masks are reset for next tick):

```go
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

### `Player.updatePlayers()` — replaces sub-spec 1 stub

```go
func (p *Player) updatePlayers() {
    s := p.client.server
    if s == nil || p.buildArea == nil {
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

### `Server` additions

```go
renderer *rsbuf.Renderer
grid     *grid.Grid
```

Initialised in `NewServer` after `s.invTypes = invTypes`:

```go
s.renderer = rsbuf.NewRenderer()
s.grid = grid.New()
```

### `active` flag lifecycle

- `addPlayer(p)`: the sub-spec-2 flow stays the same, but set `p.active = true` before returning.
- `removePlayer(p)`: set `p.active = false` before removing from `playerLoop`.
- Teleport (out of scope for 3b): the teleport branch in Phase 1 of the encoder handles the common case where a player teleports and observers auto-remove via the `self.Tele()` check. Transient `active=false` cycles for cross-zone teleports can land in sub-spec 3c or later.

### CLIENT_CHEAT handler (the end-to-end demo)

`handleClientCheat` in `modules/world/handlers_game.go`:

```go
import "strings"

func handleClientCheat(p *Player, payload []byte) error {
    r := packet.NewPacket(payload)
    _ = r.G1() // unused byte per the TS handler
    raw := string(r.GJStrLF())
    if !strings.HasPrefix(raw, "::") {
        return nil
    }
    cmd := strings.TrimPrefix(raw, "::")
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

Registered via `gameHandlers[4] = handleClientCheat` in the `init()` block of `handlers_game.go`.

---

## Testing Strategy

| Package | Tests |
|---------|-------|
| `pkg/io/packet/alt_test.go` | Each alt writer produces the expected byte for a known input (hand-computed). E.g., `P1Alt1(5)` writes 133; `P2Alt2(0x1234)` writes `0x34, 0x12` |
| `pkg/io/packet/packetbit_audit_test.go` | `pbit(3, 5); pbit(11, 1500)` produces the exact byte sequence rsbuf produces. If it doesn't, fix `PacketBit` to be MSB-first across byte boundaries |
| `pkg/grid/grid_test.go` | `Add` + `NearbyPlayers` returns the player; after `Remove`, returns empty; crossing zone boundaries works; level filtering works |
| `pkg/buildarea/buildarea_test.go` | `Players` set is populated after `HasAppearance` + `RecordAppearance` round-trip with a hash |
| `pkg/rsbuf/mask_payload_test.go` | Each of 9 mask payloads: set the mask bit + state, build payload bytes, compare to hand-computed expected |
| `pkg/rsbuf/renderer_test.go` | `ComputePlayers` skips zero-mask players; high-def and low-def populated for non-zero; low-def forces APPEARANCE + FACE_COORD |
| `pkg/rsbuf/playerinfo_test.go` | Golden-byte tests: idle alone, walk alone, two players visible, teleport case |
| `modules/world/player_masks_test.go` | Each setter raises the right mask bit + sets the right field |
| `modules/world/player_info_test.go` | Integration: two players login, tick 3x, `a.Say("hi")` → `b`'s PlayerInfo packet contains CHAT payload for `a` |

---

## Scope Boundary — Sub-Spec 3b Does NOT Include

- NPC entity type + NpcInfo encoder + NPC spawning (sub-spec 3c)
- Admin-invisibility triggers (`VisibilitySoft` is checked but no command sets it)
- Friend-list online/offline/hidden state (deferred — friend chat is its own feature)
- Combat, poison, prayer drain, skill XP — no gameplay triggers that would set DAMAGE
- Gameplay triggers for SpotAnim / ExactMove (setters exist, nothing calls them)
- Chat filtering, word filter, mute enforcement — raw bytes only
- Public vs private chat routing — everyone sees `p.Say()`
- `PREFERREDPLAYERS=255` priority heuristics — 3b uses closest-first truncation
- Real hit-animation queue (the `ShowHit` setter just raises the mask; gameplay-side hit queuing is later)

**Observable outcome:** Two Java clients connect, log in, see each other's character figure rendered with correct equipment-driven appearance (3a), and see each other walk/run/teleport. `::say hello` typed by one player produces a chat bubble above that player's head visible to everyone nearby. Login/logout correctly adds/removes the figure on other clients' screens. Other players further than 15 zones (~120 tiles) are invisible.

Sub-spec 3c (next) makes NPCs visible using the same patterns.
