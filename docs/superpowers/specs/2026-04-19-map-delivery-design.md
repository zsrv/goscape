# Sub-Spec 3a: Map Delivery Foundation

**Date:** 2026-04-19
**Project:** goscape — Go rewrite of LostCityRS/Engine-TS
**Scope:** First third of the original sub-spec 3. Delivers the cache-config plumbing (`ObjType`, `InvType`), a real `Inventory` type, per-player `BuildArea` tracking, the `RebuildNormal` packet wired into the tick loop, the `UpdateInvFull` packet, and an equipment-driven appearance generator. After this sub-spec a logged-in player sees the map render in the Java client and their own character drawn with the correct body/colors/gender; other players and NPCs remain invisible (sub-specs 3b/3c).

---

## Context

The TS reference delegates bitstream-heavy work to the Rust/WASM `@2004scape/rsbuf` crate. There is no usable Go port in the older `rs-server-225` project (only stubs). Sub-spec 3 was originally defined as a single unit (player/NPC info encoding) but is decomposed here into three independently shippable pieces:

- **3a (this spec):** foundation. Gets the map on screen and the player figure drawn.
- **3b:** PlayerInfo encoder + observer/visibility model (pure-Go rsbuf port).
- **3c:** NPC entity + NpcInfo encoder + wandering AI.

Each sub-spec produces a user-visible improvement.

---

## File Map

| File | Action | Responsibility |
|------|--------|----------------|
| `pkg/objtype/objtype.go` | **New (vendored)** | `ObjType` struct + decode from `obj.dat`, adapted from `rs-server-225/cache/config/objtype.go` |
| `pkg/objtype/invtype.go` | **New (vendored)** | `InvType` struct + decode, convenience `Inv`, `Worn` pre-resolved ids |
| `pkg/objtype/paramtype.go` | **New (vendored)** | `ParamType` and `ParamConfigs` for membership-stripping param lookup |
| `pkg/objtype/configtype.go` | **New (vendored)** | Shared `ConfigType` base + `DecodeType` dispatcher |
| `pkg/objtype/jagfile.go` | **New (vendored)** | `Jagfile` reader (adapted from `rs-server-225/io/jagfile.go` if present, else from `cache/packfile.go`) |
| `pkg/objtype/*_test.go` | New | Load a minimal `obj.dat` fixture, verify field decoding |
| `pkg/inventory/inventory.go` | **New** | `Inventory` struct; `Item`; slot operations; `FromType` factory |
| `pkg/inventory/transaction.go` | **New** | `Transaction{Requested, Completed, Items}` for `Add`/`Remove` results |
| `pkg/inventory/inventory_test.go` | **New** | Unit tests for add/remove/swap/stack-limit |
| `pkg/buildarea/buildarea.go` | **New** | `BuildArea` struct; `ShouldRebuild`; `Rebuild` |
| `pkg/buildarea/buildarea_test.go` | **New** | Edge-of-window tests |
| `pkg/xtea/xtea.go` | **New** | `Keys(mapX, mapZ int) [4]uint32` — returns zeros (stub) |
| `pkg/gamemap/gamemap.go` | Modify | Add `MapsquareCRC(mapX, mapZ) (mCRC, lCRC uint32)` + cache during `Init` |
| `pkg/io/protocol/game/server/prot.go` | Modify | Add `OpRebuildNormal = {237, -2}`, `OpUpdateInvFull = {98, -2}`, `OpUpdateInvPartial = {213, -2}` |
| `modules/world/player.go` | Modify | Add `invs map[int]*inventory.Inventory`, `invListeners []InventoryListener`, `buildArea *buildarea.BuildArea`, and BAS anim fields (`readyanim, turnanim, walkanim, walkanim_b, walkanim_l, walkanim_r, runanim int`) all defaulting to -1 |
| `modules/world/masks.go` | **New** | `MaskAppearance=1`, `MaskAnim=2`, `MaskFaceEntity=4`, … 10 player mask constants (sub-spec 3b consumes them) |
| `modules/world/appearance.go` | **New** | `generateAppearance(p, objTypes, invTypes)` |
| `modules/world/rebuildmap.go` | **New** | `sendRebuildNormal(p, mapsquares)`; fills in `updateMap()` |
| `modules/world/inv_update.go` | **New** | `updateInvs()` real implementation using `UpdateInvFull` |
| `modules/world/server.go` | Modify | Load ObjTypes/InvTypes/Params in `NewServer`; store on `*Server` |
| `modules/world/tick.go` | Modify | `processLogins` initializes `p.buildArea` and `p.invs` from server types |

---

## Shared Packages

### `pkg/objtype`

Vendored from `rs-server-225/cache/config/`. Rules follow sub-spec 2's vendoring pattern:

1. Copy `objtype.go`, `invtype.go`, `paramtype.go`, `configtype.go` into `pkg/objtype/`.
2. Copy `jagfile.go` (needed for `Jagfile` reader) into `pkg/objtype/` or into `pkg/cache/` if it's shared. The existing `pkg/cache/` already exists for CRC; extend it rather than duplicate.
3. Rewrite imports: `github.com/zsrv/rs-server-225/cache/config` → `github.com/zsrv/goscape/pkg/objtype`.
4. `rs-server-225`'s `packet.Packet` maps to goscape's `pkg/io/packet.Packet` — adjust imports.
5. Remove references to subsystems we don't have (e.g., `jagex2/wordpack` if referenced).
6. If any loader depends on `VarPlayerType` or similar unvendored types, trim that logic (sub-spec 3a only needs obj field decoding).

**Public surface after vendoring:**

- `ObjType` struct with server-side-relevant fields: `ID int; Name, Desc string; WearPos, WearPos2, WearPos3 int; Stackable, Members, Tradeable bool; Weight, Cost int; Op [5]string; IOp [5]string; CertLink, CertTemplate int; Params map[int]any`
- `InvType` struct: `ID int; Scope int; Size int; StackAll bool; Restock, AllStock bool; StockObj, StockCount []int; Protect bool; DummyInv bool`
- `ObjConfigs`, `InvConfigs`, `ParamConfigs` — each exposes `Configs []*T` indexed by id and `ConfigNames map[string]int` for name lookup.
- `InvConfigs` additionally exposes `Inv, Worn int` — pre-resolved ids for the two most-referenced inventories.
- Loaders: `LoadParams(cacheDir) (*ParamConfigs, error)`, `LoadObjTypes(cacheDir, *ParamConfigs) (*ObjConfigs, error)`, `LoadInvTypes(cacheDir) (*InvConfigs, error)`.

**Cache source:** `cacheDir/server/obj.dat`, `cacheDir/server/inv.dat`, `cacheDir/server/param.dat`, `cacheDir/client/config` (Jagfile for client config). Missing files return a descriptive error — startup fails.

### `pkg/inventory`

Ported to Go from TS `Inventory.ts` (no older Go reference; `rs-server-225/engine/inventory.go` is sparse). Self-contained — no game-engine coupling.

```go
type Item struct { Id, Count int }

type Inventory struct {
    Type      int      // InvType id
    Capacity  int
    StackType int      // StackNormal=0, StackAlways=1, StackNever=2
    Items     []*Item  // length == Capacity, nil = empty slot
    Update    bool     // dirty flag consumed by updateInvs()
}

const (
    StackNormal = 0
    StackAlways = 1
    StackNever  = 2
    StackLimit  = 0x7fffffff
)

type Transaction struct {
    Requested int
    Completed int
    Items     []Item // actual items moved (for transfer)
}

// Factory
func FromType(t *objtype.InvType) *Inventory  // populates StockObj if present

// Queries (pure)
func (inv *Inventory) Get(slot int) *Item
func (inv *Inventory) Contains(id int) bool
func (inv *Inventory) HasAt(slot, id int) bool
func (inv *Inventory) GetItemCount(id int) int
func (inv *Inventory) NextFreeSlot() int
func (inv *Inventory) FreeSlotCount() int
func (inv *Inventory) IsFull() bool
func (inv *Inventory) IsEmpty() bool

// Mutations (set Update=true on change)
func (inv *Inventory) Set(slot int, item *Item)
func (inv *Inventory) Delete(slot int)
func (inv *Inventory) Swap(from, to int)
func (inv *Inventory) Add(id, count int, opts AddOpts) Transaction
func (inv *Inventory) Remove(id, count int, opts RemoveOpts) Transaction
```

`AddOpts{BeginSlot, AssureFullInsertion, ForceNoStack, DryRun bool}` and similar for `RemoveOpts`. Transaction semantics mirror TS: partial inserts report `Completed < Requested`; `DryRun` computes without mutating.

### `pkg/buildarea`

```go
type BuildArea struct {
    OriginX, OriginZ int
    LastBuild        int
    LoadedZones      map[int]bool  // keyed by ZonePack(x, z, level)
    ActiveZones      map[int]bool
    Mapsquares       map[uint16]bool // packed (mapX<<8)|mapZ
}

func New() *BuildArea {
    return &BuildArea{
        OriginX: -1, OriginZ: -1,
        LoadedZones: map[int]bool{},
        ActiveZones: map[int]bool{},
        Mapsquares:  map[uint16]bool{},
    }
}

// ShouldRebuild implements the TS BuildArea.rebuildNormal trigger.
// Returns true when player.x/z has crossed the 13x13 zone window centered on origin,
// or when reconnect is true (force full rebuild).
func (ba *BuildArea) ShouldRebuild(playerX, playerZ int, reconnect bool) bool

// Rebuild commits the new origin and returns the 169 mapsquares
// (13x13 zones / 8 = ~1.6 mapsquares per side, rounded up) that must be loaded.
// The list is packed: (mapX<<8)|mapZ per uint16.
func (ba *BuildArea) Rebuild(playerX, playerZ, currentTick int) []uint16
```

**`ShouldRebuild` math (TS-faithful):**

```
originZoneX := ba.OriginX >> 3
originZoneZ := ba.OriginZ >> 3
if ba.OriginX == -1 { return true }   // first build
reloadLeftX   := (originZoneX - 4) << 3
reloadRightX  := (originZoneX + 5) << 3
reloadTopZ    := (originZoneZ + 5) << 3
reloadBottomZ := (originZoneZ - 4) << 3
if reconnect ||
   playerX < reloadLeftX ||
   playerZ < reloadBottomZ ||
   playerX > reloadRightX - 1 ||
   playerZ > reloadTopZ - 1 {
    return true
}
return false
```

**`Rebuild` logic:** reset `LoadedZones`, `ActiveZones`, `Mapsquares`; compute zone window `[zoneX-6, zoneX+6] × [zoneZ-6, zoneZ+6]`; for each zone, compute its mapsquare `(zoneX>>3, zoneZ>>3)` and add to `Mapsquares`; set `OriginX = playerX; OriginZ = playerZ; LastBuild = currentTick`; return sorted mapsquare list as `[]uint16`.

### `pkg/xtea`

```go
package xtea

// Keys returns the 4-word XTEA key for a mapsquare.
// Sub-spec 3a: always returns zeros. The map pack files in data/pack/client/maps/
// are already decrypted, so the client's zero-key decrypt returns the same bytes.
// TODO: load real keys from maps/xteas.json when encrypted distribution is needed.
func Keys(mapX, mapZ int) [4]uint32 {
    return [4]uint32{0, 0, 0, 0}
}
```

### `pkg/gamemap` addition

Extend the existing gamemap package with CRC32 caching during `Init`:

```go
type GameMap struct {
    // ... existing fields ...
    mapCRC map[uint16]uint32  // packed (mapX<<8)|mapZ -> CRC32 of m{x}_{z} file
    locCRC map[uint16]uint32  // ditto for l{x}_{z}
}

// During Init, after reading each mData / lData: compute CRC and store.
// Use hash/crc32 with the IEEE polynomial (matches TS Packet.getcrc).

func (gm *GameMap) MapsquareCRC(mapX, mapZ int) (mCRC, lCRC uint32)
```

If a file was missing during `Init`, its CRC is 0 — the client treats 0 as "skip loading this piece".

---

## Player Struct Extensions

Add fields to `Player` (in `modules/world/player.go`):

```go
// === inventory / appearance ===
invs          map[int]*inventory.Inventory  // keyed by InvType.ID
invListeners  []InventoryListener           // client-sync listeners

// === build area ===
buildArea     *buildarea.BuildArea
```

`InventoryListener` struct lives in `modules/world/player.go`:

```go
type InventoryListener struct {
    Type   int // InvType id
    Com    int // UI component id that's showing this inv
    Source int // player slot of the inv's owner
}
```

Sub-spec 3a does not wire listener updates yet — the field exists for sub-spec 3b+ to hook into.

---

## Algorithms

### `modules/world/appearance.go` — `generateAppearance`

```go
func (p *Player) generateAppearance(
    objTypes *objtype.ObjConfigs,
    invTypes *objtype.InvConfigs,
    currentTick int,
) {
    buf := packet.NewPacket(nil)

    // Resolve worn inventory (may be nil if not yet populated)
    worn := p.invs[invTypes.Worn]

    // Build skipped-slots set from wearPos2/wearPos3 on each worn item
    skipped := map[int]bool{}
    if worn != nil {
        for _, item := range worn.Items {
            if item == nil { continue }
            ot := objTypes.Configs[item.Id]
            if ot == nil { continue }
            if ot.WearPos2 != -1 { skipped[ot.WearPos2] = true }
            if ot.WearPos3 != -1 { skipped[ot.WearPos3] = true }
        }
    }

    buf.P1(uint8(p.gender))
    buf.P1(uint8(p.headicons))

    // 12-slot loop
    for slot := 0; slot < 12; slot++ {
        if skipped[slot] {
            buf.P1(0)
            continue
        }
        var equipped *Item
        if worn != nil && slot < len(worn.Items) {
            equipped = worn.Items[slot]
        }
        if equipped != nil {
            buf.P2(uint16(0x200 | equipped.Id))
            continue
        }
        // Body-part fallback
        bodyIdx, ok := slotToBodyIndex(slot)
        if !ok || p.body[bodyIdx] == -1 {
            buf.P1(0)
            continue
        }
        buf.P2(uint16(0x100 | p.body[bodyIdx]))
    }

    // Colors
    for i := 0; i < 5; i++ {
        buf.P1(uint8(p.colors[i]))
    }

    // BAS anims (will be -1 in sub-spec 3a; client interprets as default)
    buf.P2(uint16(p.readyanim))
    buf.P2(uint16(p.turnanim))
    buf.P2(uint16(p.walkanim))
    buf.P2(uint16(p.walkanim_b))
    buf.P2(uint16(p.walkanim_l))
    buf.P2(uint16(p.walkanim_r))
    buf.P2(uint16(p.runanim))

    buf.P8(p.username37)
    buf.P1(uint8(p.combatLevel))

    p.appearanceBuf = buf.Bytes()
    p.lastAppearance = currentTick
}

var slotToBodyTable = map[int]int{
    8: 0, 11: 1, 4: 2, 6: 3, 9: 4, 7: 5, 10: 6,
}

func slotToBodyIndex(slot int) (int, bool) {
    v, ok := slotToBodyTable[slot]
    return v, ok
}
```

Fields `readyanim, turnanim, walkanim, walkanim_b, walkanim_l, walkanim_r, runanim int` are added to the Player struct as part of this sub-spec (sub-spec 2's appearance section does not include them). All default to `-1` in `newPlayer`.

### `modules/world/rebuildmap.go` — `updateMap` + `sendRebuildNormal`

```go
func (p *Player) updateMap() {
    if p.buildArea == nil {
        return // not yet initialised — processLogins hasn't run
    }
    if !p.buildArea.ShouldRebuild(p.x, p.z, p.reconnecting) {
        return
    }
    ms := p.buildArea.Rebuild(p.x, p.z, p.client.server.currentTick)
    p.reconnecting = false
    sendRebuildNormal(p, ms)
}

func sendRebuildNormal(p *Player, mapsquares []uint16) {
    gm := p.client.server.gamemap

    // Payload: p2(zoneX) p2(zoneZ) + for each mapsquare: p1(mx) p1(mz) p4(mCRC) p4(lCRC)
    buf := packet.NewPacket(nil)
    buf.P2(uint16(p.x >> 3))
    buf.P2(uint16(p.z >> 3))
    for _, msq := range mapsquares {
        mx := int(msq >> 8)
        mz := int(msq & 0xff)
        mCRC, lCRC := gm.MapsquareCRC(mx, mz)
        buf.P1(uint8(mx))
        buf.P1(uint8(mz))
        buf.P4(mCRC)
        buf.P4(lCRC)
    }

    p.writeOut(gameserver.OpRebuildNormal, buf.Bytes())
}
```

### `modules/world/inv_update.go` — `updateInvs`

```go
func (p *Player) updateInvs() {
    for invId, inv := range p.invs {
        if !inv.Update {
            continue
        }
        // Sub-spec 3a: always send full. Partial updates land later.
        sendUpdateInvFull(p, invId, inv)
        inv.Update = false
    }
}

func sendUpdateInvFull(p *Player, invId int, inv *inventory.Inventory) {
    invTypes := p.client.server.invTypes
    // Component id for the "worn" inv is typically 149 in RS2; other invs
    // map via listener registration (not in scope for 3a).
    // Sub-spec 3a: use invId as the component placeholder — sub-spec 3b will
    // consult p.invListeners to find the right component per subscribing player.
    com := invId

    buf := packet.NewPacket(nil)
    buf.P2(uint16(com))
    size := inv.Capacity
    if size > 0xff {
        size = 0xff // p1 fits
    }
    buf.P1(uint8(size))
    for slot := 0; slot < size; slot++ {
        item := inv.Get(slot)
        if item == nil {
            buf.P2(0)
            buf.P1(0)
            continue
        }
        buf.P2(uint16(item.Id + 1))
        if item.Count >= 255 {
            buf.P1(255)
            buf.P4(uint32(item.Count))
        } else {
            buf.P1(uint8(item.Count))
        }
    }
    _ = invTypes // unused in 3a; referenced here so import stays live when listener code lands
    p.writeOut(gameserver.OpUpdateInvFull, buf.Bytes())
}
```

---

## Startup + Tick Integration

### Server startup (`NewServer`)

After the existing `gamemap.Init` call:

```go
paramTypes, err := objtype.LoadParams(cfg.CachePath)
if err != nil {
    return nil, fmt.Errorf("load params: %w", err)
}
objTypes, err := objtype.LoadObjTypes(cfg.CachePath, paramTypes)
if err != nil {
    return nil, fmt.Errorf("load obj types: %w", err)
}
invTypes, err := objtype.LoadInvTypes(cfg.CachePath)
if err != nil {
    return nil, fmt.Errorf("load inv types: %w", err)
}
s.paramTypes = paramTypes
s.objTypes = objTypes
s.invTypes = invTypes
```

Server struct gains:

```go
paramTypes *objtype.ParamConfigs
objTypes   *objtype.ObjConfigs
invTypes   *objtype.InvConfigs
```

### `processLogins`

After `addPlayer(p)` (existing from sub-spec 2), before the existing `p.lastConnected = s.currentTick` line:

```go
p.buildArea = buildarea.New()
p.invs = map[int]*inventory.Inventory{}
// Initialise the worn inventory (empty; stock items defined on InvType).
wornType := s.invTypes.Configs[s.invTypes.Worn]
if wornType != nil {
    worn := inventory.FromType(wornType)
    worn.Update = true  // trigger an UpdateInvFull on first tick
    p.invs[s.invTypes.Worn] = worn
}
// Mark appearance as dirty so it gets regenerated before sub-spec 3b's encoder uses it.
p.masks |= MaskAppearance
```

`MaskAppearance` and the other nine player mask constants live in the new `modules/world/masks.go` file (see File Map). They are used by `processLogins` here and by sub-spec 3b's PlayerInfo encoder.

### `processOut` — no change to the ordered list

`processOut` (from sub-spec 1) already calls `updateMap()` and `updateInvs()` as stubs. Sub-spec 3a fills in their bodies.

---

## Testing Strategy

| Package | Tests |
|---------|-------|
| `pkg/objtype` | Vendored tests from rs-server-225 where they exist; smoke test: load `obj.dat` and assert `ObjTypes.Configs[netsteel_sword_id].WearPos == 3` (pick any known item) |
| `pkg/inventory` | Unit tests for `Add`/`Remove` partial completion, `Swap`, `Stack`/`NoStack` behaviour, `Capacity` overflow, `FromType` stock population |
| `pkg/buildarea` | `ShouldRebuild` at each corner of the 13×13 window; `ShouldRebuild` first-time (OriginX=-1); `Rebuild` emits correct mapsquare set |
| `pkg/xtea` | Trivial (returns zeros) |
| `pkg/gamemap` | Extend existing test: `MapsquareCRC` returns non-zero CRC for a fixture mapsquare; returns 0 when no file |
| `modules/world/appearance_test.go` | Naked player: all bodies from `p.body[]`; platebody in wearPos=4 hides arms (wearPos2=6)/torso; full helm hides hair/beard; `appearanceBuf` non-empty after call |
| `modules/world/rebuildmap_test.go` | `sendRebuildNormal` wire format: opcode 237, length prefix, zoneX/Z, per-mapsquare entries with CRCs |
| `modules/world/inv_update_test.go` | `updateInvs` sends UpdateInvFull only when `inv.Update` is true; clears flag after send |
| Integration | `TestLoginSendsRebuildNormal`: log a player in via `sendLoginOK`, tick once, assert a RebuildNormal packet was written to the client-side connection |

---

## What Sub-Spec 3a Does NOT Include

- PlayerInfo encoder (sub-spec 3b)
- NpcInfo encoder + NPC entity type + spawning (sub-spec 3c)
- `UpdateInvPartial` — only full updates
- Real XTEA keys — zeros only
- Bank, shop, trade inventories — only `worn`
- Full inventory listener lifecycle (`invListenOnCom`/`invStopListenOnCom`) — the `invListeners` field exists but no operations on it
- Appearance resync trigger on inventory change — sub-spec 3b wires this via masks
- `preloadedCRC` from the TS is computed on first map load in `pkg/gamemap.Init` and cached; real TS `PreloadedPacks` pack loading is out of scope
- Visibility / staff invisibility (sub-spec 3b)
- `lastMapZone`/`lastZone` zone-change hooks (sub-spec 3c needs them for npc vis)
- Equipment stat totals (combat stats from bonus — deferred)

**Observable outcome:** Player logs in, Java client renders the 13×13 mapsquare window, player figure draws with the gender/colors/body parts from the hard-coded defaults. Player can walk around; as they cross zone boundaries, new mapsquares stream in via subsequent RebuildNormal packets. Worn inventory is empty but the slot is allocated and ready for equipment operations. Other players and NPCs still invisible until sub-specs 3b and 3c.
