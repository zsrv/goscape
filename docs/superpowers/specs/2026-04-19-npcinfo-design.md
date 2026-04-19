# Sub-Spec 3c: NPC Entity + NpcInfo Encoder

**Date:** 2026-04-19
**Project:** goscape — Go rewrite of LostCityRS/Engine-TS
**Scope:** Final third of the original sub-spec 3. Adds NPC entities (`*Npc`), the NpcInfo bitstream encoder (pure-Go port of `@2004scape/rsbuf` branch 225), wandering + patrol AI, lifecycle scaffolding with a test-only `Kill()`, and integration into the existing tick loop. After this sub-spec, players see NPCs rendered around them, with wanderers taking random steps and patrol NPCs walking their fixed routes.

---

## Context

Sub-specs 3a (map delivery) and 3b (PlayerInfo encoder) are complete. The same `pkg/rsbuf/`, `pkg/grid/`, and `pkg/buildarea/` packages that host PlayerInfo machinery now extend to NPCs. The encoder structure is similar to PlayerInfo but simpler — no own-player block, just three phases (delta + new + mask updates). Wire-format differences vs PlayerInfo:

- No own-player block; stream opens with `pbit(8, len(tracked_npcs))`.
- New-npc add header is 35 bits (13-bit nid + 11-bit type + 5+5 dx/dz + 1 ext) vs PlayerInfo's 23 bits.
- Terminator is `pbit(13, 8191)` (vs PlayerInfo's `pbit(11, 2047)`).
- 7 mask payloads with no alt-byte variants (straight P1/P2/P4); mask header is always 1 byte (no BIG flag — NPC mask values fit in 0x80).

NPC AI is two modes for sub-spec 3c: WANDER (12.5% chance per tick to pick a random tile within `WanderRange` from spawn anchor; teleport home if stuck off-spawn for 500 ticks) and PATROL (cycle through `PatrolCoord` array with per-step `PatrolDelay`). Lifecycle is scaffolded with `lifecycle/lifecycleTick/respawnRate/dead/startX/Z/Level` fields plus a test-only `Kill()` helper that marks for respawn.

---

## File Map

| File | Action |
|------|--------|
| `pkg/objtype/npctype.go` | **New (vendored)** — `NpcType`, `NPCTypeConfigs`, `LoadNPCTypes` |
| `pkg/objtype/npctype_test.go` | New — smoke load from `data/pack` |
| `pkg/gamemap/load.go` | Modify — implement `loadNPCs`, store `NpcSpawn` records |
| `pkg/gamemap/gamemap.go` | Modify — add `npcSpawns []NpcSpawn`, `NpcSpawns() []NpcSpawn` |
| `pkg/gamemap/gamemap_test.go` | Modify — add `TestLoadNpcs` with synthetic n-file |
| `pkg/grid/grid.go` | Modify — add `npcZones`, `AddNpc`, `RemoveNpc`, `NearbyNpcs` |
| `pkg/grid/grid_test.go` | Modify — `TestNpcAddRemove`, `TestNearbyNpcsLevelFilter` |
| `pkg/buildarea/buildarea.go` | Modify — add `Npcs map[int]struct{}` |
| `pkg/buildarea/buildarea_test.go` | Modify — `TestNpcsSetAddRemove` |
| `pkg/rsbuf/npc_source.go` | **New** — `NpcSource` interface + 7 NPC mask constants |
| `pkg/rsbuf/npc_mask_payload.go` | **New** — 7 mask payload encoders + header writer |
| `pkg/rsbuf/npc_mask_payload_test.go` | **New** — byte-level tests per payload |
| `pkg/rsbuf/renderer.go` | Modify — add `npcHighDef`/`npcLowDef` arrays + `ComputeNpcs` |
| `pkg/rsbuf/renderer_test.go` | Modify — `TestComputeNpcsHighDef`, `TestComputeNpcsLowDefForcesFaceCoord` |
| `pkg/rsbuf/npcinfo.go` | **New** — `EncodeNpc` 3-phase encoder |
| `pkg/rsbuf/npcinfo_test.go` | **New** — golden-byte tests |
| `pkg/io/protocol/game/server/prot.go` | Modify — add `OpNpcInfo = Op{1, -2}` |
| `modules/world/npc.go` | **New** — `Npc` struct, `NewNpc`, lifecycle constants |
| `modules/world/npc_ai.go` | **New** — `turn`, `wanderMode`, `patrolMode`, `randomWalk`, `Kill` |
| `modules/world/npc_masks.go` | **New** — 7 setter methods + `ResetMasks` |
| `modules/world/npc_source.go` | **New** — `*Npc` accessors satisfying `rsbuf.NpcSource` |
| `modules/world/npc_registry.go` | **New** — `addNpc`, `removeNpc`, `allocNpcSlot`, `npcsSnapshot` on `*Server` |
| `modules/world/npc_test.go` | **New** — defaults, setters, ResetMasks |
| `modules/world/npc_ai_test.go` | **New** — wander/patrol/teleport-home/Kill |
| `modules/world/server.go` | Modify — add `npcTypes`, `npcs [8192]*Npc`, `npcLoop`, `nextNpcSlot`; load NpcTypes + spawn NPCs in `NewServer` |
| `modules/world/tick.go` | Modify — add `processNpcs`; extend `processInfo` (ComputeNpcs); extend `processCleanup` (npc ResetMasks) |
| `modules/world/player.go` | Modify — replace `updateNpcs()` stub |
| `modules/world/player_info.go` | Modify (or new sibling) — implement `updateNpcs()` calling `EncodeNpc` |
| `modules/world/player_npc_test.go` | **New** — integration: player sees NPC, NPC says hello |

---

## Vendoring Notes

`pkg/objtype/npctype.go`:

```bash
cp /home/owner/Code/github.com/zsrv/rs-server-225/cache/config/npctype.go \
   /home/owner/Code/github.com/zsrv/goscape/pkg/objtype/
sed -i '1s/^package config$/package objtype/' npctype.go
sed -i 's|github.com/zsrv/rs-server-225/io/packet|github.com/zsrv/goscape/pkg/io/packet|g' npctype.go
sed -i 's|"github.com/zsrv/rs-server-225/io"|io "github.com/zsrv/goscape/pkg/io/jagfile"|g' npctype.go
```

If the vendored code references `entity.MoveRestrict` / `entity.NPCMode` enum types not present in goscape, replace with plain `int` fields. The minimum public surface required:

```go
type NpcType struct {
    ConfigType
    Name        string
    Desc        string
    Size        int
    CombatLevel int
    VisLevel    int
    MoveRestrict int
    BlockWalk   int
    WanderRange int
    RespawnRate int
    Members     bool

    // Patrol
    PatrolCoord []int  // packed coord per step
    PatrolDelay []int  // ticks between steps

    // Display (TypeID drives client appearance; encoder uses TypeID only)
    ReadyAnim, WalkAnim, WalkAnimB, WalkAnimL, WalkAnimR, RunAnim int
    Models, Heads []int

    Params map[int]any
}

type NPCTypeConfigs struct {
    ConfigNames map[string]int
    Configs     []*NpcType
}

func LoadNPCTypes(cacheDir string) (*NPCTypeConfigs, error)
```

If the vendored loader requires `*ParamTypeConfigs`, accept it the same way `LoadObjTypes` does (existing `s.paramTypes` is loaded in sub-spec 3a startup).

---

## Map-Pack NPC Loading

`pkg/gamemap/load.go` `loadNPCs` was a stub in sub-spec 2. Implement now:

```go
type NpcSpawn struct {
    TypeID int
    X, Z, Level int
}

// In gamemap.go GameMap struct, add:
//   npcSpawns []NpcSpawn

func (gm *GameMap) loadNPCs(data []byte, mapSquareX, mapSquareZ int) {
    p := packet.NewPacket(data)
    for p.Len() >= 3 {
        packed := int(p.G2())
        count := int(p.G1())
        // Layout (from rs-server-225/engine/gamemap.go): top 2 bits level, then 6+6 local x/z.
        level := (packed >> 12) & 0x3
        localX := (packed >> 6) & 0x3F
        localZ := packed & 0x3F
        absX := mapSquareX*64 + localX
        absZ := mapSquareZ*64 + localZ
        for i := 0; i < count && p.Len() >= 2; i++ {
            typeID := int(p.G2())
            gm.npcSpawns = append(gm.npcSpawns, NpcSpawn{
                TypeID: typeID, X: absX, Z: absZ, Level: level,
            })
        }
    }
}

func (gm *GameMap) NpcSpawns() []NpcSpawn { return gm.npcSpawns }
```

Initialise `npcSpawns: nil` in `New()` (zero-value works fine).

---

## Npc Struct + Defaults

```go
type Npc struct {
    nid    int      // 1..8191; assigned by NpcRegistry
    typeId int
    typ    *objtype.NpcType

    // === lifecycle (scaffolded, test-only Kill triggers respawn) ===
    lifecycle     int  // 0=FOREVER, 1=RESPAWN, 2=DESPAWN
    lifecycleTick int
    respawnRate   int
    dead          bool
    startX, startZ, startLevel int

    // === coords ===
    x, z, level                     int
    lastTickX, lastTickZ, lastLevel int
    originX, originZ                int

    // === movement (mirrors Player's PathingEntity surface) ===
    moveSpeed     MoveSpeed
    moveRestrict  MoveRestrict
    moveStrategy  MoveStrategy
    walkDir, runDir int
    waypointIndex int
    waypoints     [25]int
    tele          bool
    stepsTaken    int

    // === AI ===
    targetOp        int  // -1=NONE, 0=WANDER, 1=PATROL
    wanderCounter   int
    nextPatrolTick  int
    nextPatrolPoint int
    delayedPatrol   bool

    // === interaction (sub-spec 3c: target stays nil) ===
    target     entity
    faceEntity int

    // === masks ===
    masks      int
    entitymask int

    // === mask state ===
    animID, animDelay int
    sayText []byte
    damageAmt, damageType int
    curHP, baseHP         int
    spotanimID, spotanimHeight, spotanimDelay int
    faceSquareX, faceSquareZ int
    changeTypeID int
}
```

**Lifecycle constants:**

```go
const (
    NpcLifecycleForever = 0
    NpcLifecycleRespawn = 1
    NpcLifecycleDespawn = 2
)
```

**`NewNpc`** sets sensible defaults: most `int` mask-state fields to `-1`, `lifecycle = NpcLifecycleRespawn` (map-spawned default), `targetOp = npcDefaultMode(typ)`, `moveStrategy = MoveStrategyNaive` (NPCs don't use the pathfinder), `moveRestrict = MoveRestrict(typ.MoveRestrict)`, coords stored in both current and `start*` fields, `respawnRate = typ.RespawnRate`.

**`npcDefaultMode(typ *objtype.NpcType) int`** returns `1` (PATROL) if `len(typ.PatrolCoord) > 0`, else `0` (WANDER) if `typ.WanderRange > 0`, else `-1` (NONE — stationary).

---

## NPC AI

**`(n *Npc) turn(s *Server)`** runs once per tick from `processNpcs`:

```go
func (n *Npc) turn(s *Server) {
    if n.dead {
        n.lifecycleTick--
        if n.lifecycleTick <= 0 && n.lifecycle == NpcLifecycleRespawn {
            n.dead = false
            n.x, n.z, n.level = n.startX, n.startZ, n.startLevel
            n.tele = true
            n.masks |= rsbuf.NpcMaskChangeType
        }
        return
    }
    if n.moveRestrict == MoveRestrictNoMove {
        return
    }

    n.lastTickX, n.lastTickZ, n.lastLevel = n.x, n.z, n.level
    n.tele = false

    if n.waypointIndex >= 0 {
        n.advanceWaypoint(s)
        n.wanderCounter = 0
    } else {
        n.wanderCounter++
        switch n.targetOp {
        case 0: // WANDER
            n.wanderMode(s)
        case 1: // PATROL
            n.patrolMode(s)
        }
        if n.wanderCounter > 500 && (n.x != n.startX || n.z != n.startZ) {
            n.x, n.z, n.level = n.startX, n.startZ, n.startLevel
            n.tele = true
            n.wanderCounter = 0
        }
    }
}
```

**`wanderMode`:**

```go
func (n *Npc) wanderMode(s *Server) {
    if n.typ.WanderRange == 0 { return }
    if rand.IntN(8) != 0 { return } // 12.5%
    rng := n.typ.WanderRange
    dx := rand.IntN(rng*2+1) - rng
    dz := rand.IntN(rng*2+1) - rng
    n.queueWaypoint(n.startX+dx, n.startZ+dz)
}
```

**`patrolMode`:**

```go
func (n *Npc) patrolMode(s *Server) {
    if len(n.typ.PatrolCoord) == 0 { return }
    if s.currentTick < n.nextPatrolTick { return }
    coord := n.typ.PatrolCoord[n.nextPatrolPoint]
    pos := coordgrid.UnpackCoord(coord)
    n.queueWaypoint(pos.X, pos.Z)
    n.nextPatrolTick = s.currentTick + n.typ.PatrolDelay[n.nextPatrolPoint]
    n.nextPatrolPoint = (n.nextPatrolPoint + 1) % len(n.typ.PatrolCoord)
}
```

**`queueWaypoint` / `advanceWaypoint`** — straightforward port of the player versions but simpler: NPCs only walk one tile per tick (no run), no collision checking against other entities (just `gamemap.CanTravel` for terrain). Implementation in `npc_ai.go`.

**`Kill()` — test-only respawn helper:**

```go
func (n *Npc) Kill() {
    n.dead = true
    n.lifecycleTick = n.respawnRate
    if n.lifecycleTick <= 0 {
        n.lifecycleTick = 50 // 30s default
    }
}
```

---

## Mask Setters + Reset

`modules/world/npc_masks.go`:

```go
func (n *Npc) Animate(id, delay int)
func (n *Npc) Say(msg []byte)
func (n *Npc) ShowHit(amount, dmgType, cur, base int)
func (n *Npc) ChangeType(newType int) // sets changeTypeID; future: also update n.typeId
func (n *Npc) SpotAnim(id, height, delay int)
func (n *Npc) FaceCoord(x, z int)     // sets faceSquareX = x*2+1, faceSquareZ = z*2+1
func (n *Npc) SetFaceEntity(idx int)
func (n *Npc) ResetMasks()
```

Each setter raises the matching `rsbuf.NpcMaskXxx` bit in `n.masks`. `ResetMasks` clears `masks`, `sayText`, `damageAmt`/`damageType`/`curHP`/`baseHP` (all to -1 / nil), and `spotanim*` fields. Persistent: `animID`, `faceEntity`, `faceSquareX/Z`, `changeTypeID`.

---

## Encoder

**`pkg/rsbuf/npc_source.go`:**

```go
const (
    NpcMaskAnim       = 0x2
    NpcMaskFaceEntity = 0x4
    NpcMaskSay        = 0x8
    NpcMaskDamage     = 0x10
    NpcMaskChangeType = 0x20
    NpcMaskSpotAnim   = 0x40
    NpcMaskFaceCoord  = 0x80
)

type NpcSource interface {
    Nid() int
    TypeID() int
    Coords() (x, z, level int)
    Active() bool

    Masks() int
    EntityMask() int

    AnimID() int
    AnimDelay() int
    FaceEntity() int
    SayText() []byte
    DamageAmt() int
    DamageType() int
    CurHP() int
    BaseHP() int
    ChangeTypeID() int
    SpotAnimID() int
    SpotAnimHeight() int
    SpotAnimDelay() int
    FaceSquareX() int
    FaceSquareZ() int

    WalkDir() int
    RunDir() int
    Tele() bool
    LastTickX() int
    LastTickZ() int
    LastLevel() int
}
```

**`pkg/rsbuf/npc_mask_payload.go` — fixed write order ANIM → FACE_ENTITY → SAY → DAMAGE → CHANGE_TYPE → SPOT_ANIM → FACE_COORD:**

```go
func writeNpcMaskHeader(buf *packet.Packet, masks int) {
    buf.P1(uint8(masks)) // always 1 byte (no BIG bit since NPC masks fit in 0x80)
}

func writeNpcMaskPayloads(buf *packet.Packet, n NpcSource, forceMasks int) {
    if forceMasks&NpcMaskAnim != 0 {
        buf.P2(uint16(n.AnimID()))
        buf.P1(uint8(n.AnimDelay()))
    }
    if forceMasks&NpcMaskFaceEntity != 0 {
        buf.P2(uint16(n.FaceEntity()))
    }
    if forceMasks&NpcMaskSay != 0 {
        for _, b := range n.SayText() { buf.P1(b) }
        buf.P1(10)
    }
    if forceMasks&NpcMaskDamage != 0 {
        buf.P1(uint8(n.DamageAmt()))
        buf.P1(uint8(n.DamageType()))
        buf.P1(uint8(n.CurHP()))
        buf.P1(uint8(n.BaseHP()))
    }
    if forceMasks&NpcMaskChangeType != 0 {
        buf.P2(uint16(n.ChangeTypeID()))
    }
    if forceMasks&NpcMaskSpotAnim != 0 {
        buf.P2(uint16(n.SpotAnimID()))
        buf.P4(uint32(n.SpotAnimHeight())<<16 | uint32(n.SpotAnimDelay()))
    }
    if forceMasks&NpcMaskFaceCoord != 0 {
        buf.P2(uint16(n.FaceSquareX()))
        buf.P2(uint16(n.FaceSquareZ()))
    }
}
```

**Renderer extension:**

```go
type Renderer struct {
    // existing player caches (highDef, lowDefFull, lowDefNoApp)
    npcHighDef [8192][]byte
    npcLowDef  [8192][]byte
}

func (r *Renderer) ComputeNpcs(npcs []NpcSource)
func (r *Renderer) NpcHighDefOf(nid int) []byte
func (r *Renderer) NpcLowDefOf(nid int) []byte
```

`ComputeNpcs` builds high-def for any non-zero mask, and a low-def variant that always includes `NpcMaskFaceCoord` so newly-tracked observers know where to look.

**`pkg/grid/grid.go` extensions:**

```go
type Grid struct {
    playerZones map[uint32][]int // existing
    npcZones    map[uint32][]int // NEW
}

func (g *Grid) AddNpc(nid, x, z, level int)
func (g *Grid) RemoveNpc(nid, x, z, level int)
func (g *Grid) NearbyNpcs(x, z, level, zoneRadius int) []int
```

**`pkg/buildarea` addition:**

```go
Npcs map[int]struct{} // tracked nids per client
```

Initialise empty in `New()`.

**`pkg/rsbuf/npcinfo.go` — `EncodeNpc`:**

```go
const (
    NpcViewDistanceZones = 15
    PreferredNpcs        = 255
    NpcAddBits           = 35
    NpcTerminator        = 8191
)

func EncodeNpc(self PlayerSource, all []NpcSource, ba *buildarea.BuildArea, g *grid.Grid, r *Renderer) []byte {
    byNid := make(map[int]NpcSource, len(all))
    for _, n := range all { byNid[n.Nid()] = n }

    main := packet.NewPacket(nil)
    updates := packet.NewPacket(nil)

    main.AccessBits()

    // Phase 1: tracked-npcs delta loop.
    main.PBit(8, len(ba.Npcs))
    slots := make([]int, 0, len(ba.Npcs))
    for nid := range ba.Npcs { slots = append(slots, nid) }
    selfX, selfZ, selfLevel := self.Coords()
    for _, nid := range slots {
        n, ok := byNid[nid]
        if !ok || !n.Active() {
            main.PBit(1, 1); main.PBit(2, 3) // remove
            delete(ba.Npcs, nid)
            continue
        }
        nx, nz, nl := n.Coords()
        if nl != selfLevel || zoneDist(selfX, selfZ, nx, nz) > NpcViewDistanceZones {
            main.PBit(1, 1); main.PBit(2, 3)
            delete(ba.Npcs, nid)
            continue
        }
        extend := 0
        payload := r.NpcHighDefOf(nid)
        if len(payload) > 0 && fits(main, updates, len(payload)) { extend = 1 }
        switch {
        case n.RunDir() != -1:
            main.PBit(1, 1); main.PBit(2, 2)
            main.PBit(3, n.WalkDir()); main.PBit(3, n.RunDir())
            main.PBit(1, extend)
        case n.WalkDir() != -1:
            main.PBit(1, 1); main.PBit(2, 1)
            main.PBit(3, n.WalkDir()); main.PBit(1, extend)
        case n.Masks() != 0:
            main.PBit(1, 1); main.PBit(2, 0)
            extend = 1
        default:
            main.PBit(1, 0)
        }
        if extend == 1 && len(payload) > 0 {
            for _, b := range payload { updates.P1(b) }
        }
    }

    // Phase 2: new-npcs loop.
    candidates := g.NearbyNpcs(selfX, selfZ, selfLevel, NpcViewDistanceZones)
    for _, nid := range candidates {
        if _, already := ba.Npcs[nid]; already { continue }
        if len(ba.Npcs) >= PreferredNpcs { break }
        n, ok := byNid[nid]
        if !ok || !n.Active() { continue }
        payload := r.NpcLowDefOf(nid)
        if !fits(main, updates, len(payload)+5) { // 5 bytes ≈ 35-bit add header
            main.PBit(13, NpcTerminator)
            break
        }
        nx, nz, _ := n.Coords()
        dx := clamp(nx-selfX, -15, 15)
        dz := clamp(nz-selfZ, -15, 15)

        main.PBit(13, nid)
        main.PBit(11, n.TypeID())
        main.PBit(5, dx&0x1f)
        main.PBit(5, dz&0x1f)
        main.PBit(1, boolToInt(len(payload) > 0))

        ba.Npcs[nid] = struct{}{}
        if len(payload) > 0 {
            for _, b := range payload { updates.P1(b) }
        }
    }

    main.AccessBytes()
    for _, b := range updates.Data { main.P1(b) }
    return main.Data
}
```

Reuses `fits`, `clamp`, `zoneDist`, `boolToInt` from `playerinfo.go`.

**`OpNpcInfo`** in `pkg/io/protocol/game/server/prot.go`:

```go
OpNpcInfo = Op{Opcode: 1, PayloadSize: -2}
```

---

## Server + Tick Integration

**`Server` additions:**

```go
npcTypes    *objtype.NPCTypeConfigs
npcs        [8192]*Npc
npcLoop     []*Npc
nextNpcSlot int
```

**`NewServer` after `s.invTypes = invTypes`:**

```go
npcTypes, err := objtype.LoadNPCTypes(cfg.CachePath)
if err != nil {
    return nil, fmt.Errorf("load npc types: %w", err)
}
s.npcTypes = npcTypes

for _, spawn := range s.gamemap.NpcSpawns() {
    if spawn.TypeID < 0 || spawn.TypeID >= len(npcTypes.Configs) {
        continue
    }
    typ := npcTypes.Configs[spawn.TypeID]
    if typ == nil { continue }
    nid := s.allocNpcSlot()
    if nid < 0 {
        s.log.Warn("npc registry full; dropping remaining spawns")
        break
    }
    n := NewNpc(nid, spawn.TypeID, spawn.X, spawn.Z, spawn.Level, typ)
    s.npcs[nid] = n
    s.npcLoop = append(s.npcLoop, n)
    s.grid.AddNpc(nid, n.x, n.z, n.level)
}
```

**`allocNpcSlot()` / `addNpc` / `removeNpc` / `npcsSnapshot`** in `modules/world/npc_registry.go`. Slot allocation: scan `s.npcs[1..8191]` from `s.nextNpcSlot` for the first nil; bump `s.nextNpcSlot` for next call.

**Tick ordering — extend `runTickLoopWithRate`:**

```
processClientsIn
processPathing       (players)
processNpcs          ← NEW
processLogouts
processLogins
processInfo          ← extends: ComputeNpcs alongside ComputePlayers
processClientsOut    (calls updatePlayers + updateNpcs per player)
processCleanup       ← extends: reset npc masks
```

**`processNpcs`:**

```go
func (s *Server) processNpcs() {
    for _, n := range s.npcLoop {
        prevX, prevZ, prevLevel := n.x, n.z, n.level
        n.turn(s)
        if n.x != prevX || n.z != prevZ || n.level != prevLevel {
            s.grid.RemoveNpc(n.nid, prevX, prevZ, prevLevel)
            s.grid.AddNpc(n.nid, n.x, n.z, n.level)
        }
    }
}
```

**`processInfo` extension:** after building the player `sources` and calling `ComputePlayers`, also build `npcSources []rsbuf.NpcSource` (cast each `*Npc` from the `npcLoop` snapshot) and call `s.renderer.ComputeNpcs(npcSources)`.

**`processCleanup` extension:** after `p.ResetMasks()` for each player, iterate `s.npcLoop` and call `n.ResetMasks()`.

**`Player.updateNpcs()` — replaces sub-spec 1 stub:**

```go
func (p *Player) updateNpcs() {
    s := p.client.server
    if s == nil || p.buildArea == nil || s.renderer == nil || s.grid == nil {
        return
    }
    s.playersMu.RLock()
    snapshot := make([]*Npc, len(s.npcLoop))
    copy(snapshot, s.npcLoop)
    s.playersMu.RUnlock()

    sources := make([]rsbuf.NpcSource, len(snapshot))
    for i, n := range snapshot {
        sources[i] = n
    }
    payload := rsbuf.EncodeNpc(p, sources, p.buildArea, s.grid, s.renderer)
    p.writeOut(gameserver.OpNpcInfo, payload)
}
```

Implementation lives in `modules/world/player_info.go` (sibling to `updatePlayers`) or a new `modules/world/player_npc_info.go`.

---

## Testing Strategy

| Package | Tests |
|---------|-------|
| `pkg/objtype/npctype_test.go` | Smoke load of `data/pack` (skip if cache missing); assert non-empty Configs |
| `pkg/gamemap/gamemap_test.go` | `TestLoadNpcs` — synthetic n-file with 2 spawns; assert `NpcSpawns()` returns both with correct coords/typeIDs |
| `pkg/grid/grid_test.go` | `TestNpcAddRemove`, `TestNearbyNpcsLevelFilter`, `TestPlayerAndNpcSeparateIndexes` |
| `pkg/buildarea/buildarea_test.go` | `TestNpcsSetAddRemove` |
| `pkg/rsbuf/npc_mask_payload_test.go` | One per mask payload (e.g., `TestNpcAnim`: id=100, delay=5 → `[0x00, 0x64, 0x05]`). Plus `TestNpcMaskHeader1Byte` (always P1 since values fit 0x80) |
| `pkg/rsbuf/renderer_test.go` | `TestComputeNpcsHighDefSkipsZero`, `TestComputeNpcsLowDefForcesFaceCoord` |
| `pkg/rsbuf/npcinfo_test.go` | `TestEncodeNpcEmpty` (no tracked, no nearby → first byte's top 8 bits = `pbit(8, 0)`); `TestEncodeNpcAddsNew` (one nearby npc, ba.Npcs gets populated); `TestEncodeNpcRemovesAfterMove` (npc moves >15 zones, removed from tracked set) |
| `modules/world/npc_test.go` | `NewNpc` defaults; each setter raises correct mask bit; `ResetMasks` clears ephemeral state but retains persistent (animID, faceEntity) |
| `modules/world/npc_ai_test.go` | Seeded RNG: `wanderMode` queues a waypoint exactly 1/8 ticks; destination is within `WanderRange` of spawn; `wanderCounter` resets after a step; teleport-home triggers after 500 stuck ticks; `patrolMode` cycles through coords with correct delays; `Kill()` sets dead+lifecycleTick; tick advancement respawns at start coords |
| `modules/world/player_npc_test.go` | Integration: setup player + 1 NPC at adjacent tile with `WanderRange=0`. After `processNpcs`+`processInfo`+`updateNpcs`, player's `buildArea.Npcs` contains the NPC's nid. Then `n.Say([]byte("hello"))`, run `processInfo` again, assert renderer's `NpcHighDefOf` contains the SAY payload bytes (`hello\n`) |

---

## Scope Boundary — Sub-Spec 3c Does NOT Include

- Combat (no auto-attack, no damage formulas, no targeted modes)
- Hunt mode / hunt clock / hunt target
- Stat regen, prayer drain, poison
- Script-spawned NPCs (`npc_add` runtime command)
- NPC-on-NPC interaction
- `levels[6]` / `baseLevels[6]` stat arrays beyond field declarations
- `vars` / `varsString` script vars
- NPC dialog / interface triggers
- Multi-tile NPCs (size > 1) — stored on the NpcType but encoder treats all as size-1
- Real "Kill from combat" — `Kill()` is a test-only helper
- `delayedPatrol` re-trigger logic beyond cycling through coords
- Pathfinder routing for NPCs (NaivePath only — straight-line walk + collision check)

**Observable outcome:** Players log in and see NPCs around them. NPCs whose `WanderRange > 0` periodically take random steps within wander range, returning home if stuck off-spawn for 500 ticks. NPCs with `PatrolCoord` walk fixed routes. Test-only `n.Kill()` removes the NPC; after `RespawnRate` ticks it reappears at its spawn coord with a CHANGE_TYPE mask flagged. Two players see the same NPCs (each has their own `BuildArea.Npcs` tracked set, so observation is per-client). NPC say/animate/spotanim payloads encode correctly when triggered by the setter methods.

This completes the original sub-spec 3 (PlayerInfo + NpcInfo + appearance + map delivery). The next milestone (sub-spec 4 in the original decomposition) covers the remaining outbound update functions: `updateZones`, full `updateInvs` + listeners, `updateStats`, `updateAfkZones`.
