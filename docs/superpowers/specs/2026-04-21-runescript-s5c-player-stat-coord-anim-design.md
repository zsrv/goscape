# Sub-spec RuneScript S5c: Player Stat/Coord/Facing/Anim Opcodes — Design

**Status:** Draft → ready for plan
**Scope:** 23 PlayerOps handlers covering stat read/write, position/coord query, facing, teleport, and the ANIM/SPOTANIM/BAS animation setter family. Extends `ActivePlayer` with coord/facing/teleport/anim methods; stat mutation piggybacks on the existing `lastLevels` dirty-compare that already drives `OpUpdateStat` each tick.
**Out of scope:** `P_WALK` (needs pathfinder + interaction-system waypoint integration — own sub-spec), protect-gated ops (OPHELD/OPLOC/OPNPC family — S6 territory), XP scaling curves (TS uses `getLevelByExp` tables — we'll stub the base-level calc and refine later), `STAT_RANDOM` probability tables (the handler is trivial but we need confirmation of the "level boosts success chance" formula).

---

## Goal

After S5c:

- Cache scripts that read or mutate player stats (`STAT`, `STAT_BASE`, `STAT_ADD`, `STAT_HEAL`, `STAT_ADVANCE` etc.) run without `unknown opcode` errors. Mutations surface to the Java client via the existing stat dirty-compare that emits `OpUpdateStat` each tick.
- Cache scripts that query position (`COORD`) get the player's packed `(level, x, z)` coord.
- Cache scripts that call `P_TELEPORT` / `P_TELEJUMP` warp the player to a new tile; the client sees the teleport block in `OpPlayerInfo` next tick.
- Cache scripts that call `FACESQUARE` set the player's facing target; the client sees the mask next tick.
- Cache scripts that call `ANIM` / `SPOTANIM_PL` / the BAS setters (`READYANIM`, `TURNANIM`, `WALKANIM`/`_B`/`_L`/`_R`, `RUNANIM`) set per-player animation state.
- Demo: a LOGIN trigger that checks `stat_base(0)` (attack) < 5 and teleports the player to Lumbridge runs end-to-end, visible on the Java client.

## Architecture

One new handler file, one new test file, and per-method additions on the ActivePlayer interface + Player adapter:

```
pkg/script/
├── handlers_player.go        (new) 23 handlers
├── handlers_player_test.go   (new)
└── active.go                 + ~15 new methods on ActivePlayer

modules/world/
├── player_script.go          + ActivePlayer method impls that mutate Player fields / call existing helpers
└── script_test.go            + end-to-end TeleJump + StatAdvance wire test
```

Single handlers file (not split further): 23 handlers × ~8 LOC each ≈ 180 LOC, still comfortable in one file.

`handlers.go` grows by 23 map entries in one block labeled "S5c".

## Components

### 1. Opcode inventory (all constants already in `pkg/script/opcode.go`)

**Stat (10):**
- `OpStat` (2116), `OpStatBase` (2109), `OpStatTotal` (2115)
- `OpStatAdd` (2107), `OpStatSub` (2114), `OpStatBoost` (2110)
- `OpStatDrain` (2111), `OpStatHeal` (2112), `OpStatAdvance` (2108), `OpStatRandom` (2113)

**Coord / Facing / Teleport (5):**
- `OpCoord` (2014), `OpFaceSquare` (2017)
- `OpPTeleport` (2088), `OpPTeleJump` (2087)
- **`OpPWalk` (2089) — registered as a `slog.Debug` stub; real impl in a later sub-spec.**

**Animation (9):**
- `OpAnim` (2002), `OpSpotAnimPl` (2105)
- `OpReadyAnim` (2094), `OpTurnAnim` (2119), `OpRunAnim` (2095)
- `OpWalkAnim` (2127), `OpWalkAnimB` (2124), `OpWalkAnimL` (2125), `OpWalkAnimR` (2126)

**Total: 24 handlers (23 active + 1 stub).**

### 2. `ActivePlayer` interface extensions

Group by operation kind:

```go
// === S5c: position ===

// CoordPacked returns the player's packed coord: (level << 28) | (x << 14) | z.
// 14-bit x and z, 4-bit level. Matches TS CoordGrid.packCoord.
CoordPacked() int

// TeleJump warps to (x, z, level) with a jump animation block.
// Coordinates are absolute tile coords.
TeleJump(x, z, level int)

// Teleport warps to (x, z, level) without a jump animation.
Teleport(x, z, level int)

// FaceSquare sets the facing target to tile (x, z) at the player's level.
// Stored as square center (x*2+1, z*2+1) per RS convention.
FaceSquare(x, z int)

// === S5c: stats ===

// Stat returns the current (possibly boosted/drained) level for skill id.
Stat(id int) int

// StatBase returns the base level for skill id.
StatBase(id int) int

// StatXP returns the accumulated XP for skill id (raw int32 form; wire divides by 10).
StatXP(id int) int

// SetCurLevel writes the current level directly, clamped [0, 255].
// Used by STAT_ADD/SUB/BOOST/DRAIN/HEAL — the dirty-compare in
// updateStats() picks up the change and emits OpUpdateStat.
SetCurLevel(id int, level int)

// AddXP credits the given XP delta to skill id and updates base level
// if the XP crosses a level threshold. Implementation is free to leave
// base-level re-derivation as a TODO (simple monotonic add is fine for S5c).
AddXP(id int, xp int)

// === S5c: animation ===

// PlayAnim queues a one-shot animation (seq id + delay ticks).
// -1 disables / clears.
PlayAnim(seqID, delay int)

// PlaySpotAnim queues a one-shot spotanim (id, height pixels, delay ticks).
PlaySpotAnim(id, height, delay int)

// SetReadyAnim / SetTurnAnim / SetWalkAnim / SetWalkAnimB/L/R / SetRunAnim
// override the corresponding BAS field. -1 clears.
SetReadyAnim(seqID int)
SetTurnAnim(seqID int)
SetWalkAnim(seqID int)
SetWalkAnimB(seqID int)
SetWalkAnimL(seqID int)
SetWalkAnimR(seqID int)
SetRunAnim(seqID int)
```

All bodies delegate to existing `Player` fields. The Player implementations are thin — most are one-line field assignments plus a mask set.

### 3. Handler layout — `pkg/script/handlers_player.go`

Shape of a representative handler:

```go
func handleStat(s *ScriptState) error {
    if s.Pointers&PtrActivePlayer == 0 || s.Self == nil {
        return errors.New("STAT: no active player")
    }
    id := s.PopInt()
    if id < 0 || id >= 21 {
        return errors.New("STAT: stat id out of range")
    }
    s.PushInt(s.Self.Stat(id))
    return nil
}

func handleStatAdvance(s *ScriptState) error {
    // TS pops stat, xp (delta in wire units, i.e. XP/10 scale). We keep
    // the same convention — Player.AddXP is responsible for scaling.
    if s.Pointers&PtrActivePlayer == 0 || s.Self == nil {
        return errors.New("STAT_ADVANCE: no active player")
    }
    xp := s.PopInt()
    id := s.PopInt()
    if id < 0 || id >= 21 {
        return errors.New("STAT_ADVANCE: stat id out of range")
    }
    s.Self.AddXP(id, xp)
    return nil
}

func handleStatAdd(s *ScriptState) error {
    // TS popInts(3) = [stat, constant, percent]. Stack top is percent.
    // Formula: boosted = min(level + constant + (base * percent / 100), 255).
    // Verify exact formula against TS PlayerOps.ts line 501-519 during impl.
    if s.Pointers&PtrActivePlayer == 0 || s.Self == nil {
        return errors.New("STAT_ADD: no active player")
    }
    percent := s.PopInt()
    constant := s.PopInt()
    id := s.PopInt()
    if id < 0 || id >= 21 {
        return errors.New("STAT_ADD: stat id out of range")
    }
    cur := s.Self.Stat(id)
    base := s.Self.StatBase(id)
    delta := constant + (base*percent)/100
    newLevel := cur + delta
    if newLevel > 255 {
        newLevel = 255
    }
    s.Self.SetCurLevel(id, newLevel)
    return nil
}
```

Implementer **must** cross-verify the exact formula + pop order for each of STAT_ADD / SUB / BOOST / DRAIN / HEAL against TS `PlayerOps.ts`. The spec lists the shape; the formulas need final confirmation from source.

`STAT_RANDOM` implementation: TS uses `Math.random() * 256` vs a level-based threshold. Default implementation uses `math/rand/v2.IntN(256)`; a dedicated note flags "probability tuning TBD" for later refinement.

`COORD` packs as shown:

```go
func handleCoord(s *ScriptState) error {
    if s.Pointers&PtrActivePlayer == 0 || s.Self == nil {
        return errors.New("COORD: no active player")
    }
    s.PushInt(s.Self.CoordPacked())
    return nil
}
```

`P_TELEPORT` / `P_TELEJUMP` / `FACESQUARE` unpack the popped coord:

```go
// unpackCoord returns (level, x, z) from the packed coord format.
func unpackCoord(c int) (level, x, z int) {
    level = (c >> 28) & 0x3
    x = (c >> 14) & 0x3fff
    z = c & 0x3fff
    return
}

func handlePTeleJump(s *ScriptState) error {
    if s.Pointers&PtrActivePlayer == 0 || s.Self == nil {
        return errors.New("P_TELEJUMP: no active player")
    }
    level, x, z := unpackCoord(s.PopInt())
    s.Self.TeleJump(x, z, level)
    return nil
}
```

`OpPWalk` stub:

```go
func handlePWalk(s *ScriptState) error {
    _ = s.PopInt() // drop the coord
    slog.Debug("P_WALK stub invoked; pathfinder integration pending",
        "script", s.Script.Name, "pc", s.PC)
    return nil
}
```

### 4. `Player` method impls — `modules/world/player_script.go`

All thin. Examples:

```go
func (p *Player) CoordPacked() int {
    return (p.level << 28) | (p.x << 14) | p.z
}

func (p *Player) TeleJump(x, z, level int) {
    p.x = x
    p.z = z
    p.level = level
    p.tele = true
    p.jump = true
}

func (p *Player) Teleport(x, z, level int) {
    p.x = x
    p.z = z
    p.level = level
    p.tele = true
    // jump stays false — no animation block
}

func (p *Player) FaceSquare(x, z int) {
    p.faceSquareX = x*2 + 1
    p.faceSquareZ = z*2 + 1
    p.masks |= MaskFaceCoord // implementer verifies the exact mask name
}

func (p *Player) Stat(id int) int        { return int(p.levels[id]) }
func (p *Player) StatBase(id int) int    { return int(p.baseLevels[id]) }
func (p *Player) StatXP(id int) int      { return int(p.stats[id]) }

func (p *Player) SetCurLevel(id int, level int) {
    if level < 0 {
        level = 0
    } else if level > 255 {
        level = 255
    }
    p.levels[id] = uint8(level)
    // No explicit dirty flag needed: updateStats() compares p.levels[i]
    // against p.lastLevels[i] every tick and emits OpUpdateStat on diff.
}

func (p *Player) AddXP(id int, xp int) {
    p.stats[id] += int32(xp)
    // Base-level re-derivation from XP curve is a TODO — for S5c we
    // leave baseLevels untouched. A later sub-spec wires the proper
    // getLevelByExp table.
}

func (p *Player) PlayAnim(seqID, delay int) {
    p.animID = seqID
    p.animDelay = delay
    p.masks |= MaskAnim // implementer verifies mask name
}

func (p *Player) PlaySpotAnim(id, height, delay int) {
    // implementer: verify existing spotanim fields on Player or add them.
    // Typical shape: p.spotanim = id, p.spotanimHeight = height,
    // p.spotanimDelay = delay, p.masks |= MaskSpotAnim
}

func (p *Player) SetReadyAnim(seqID int) { p.readyanim = seqID }
func (p *Player) SetTurnAnim(seqID int)  { p.turnanim = seqID }
func (p *Player) SetWalkAnim(seqID int)  { p.walkanim = seqID }
func (p *Player) SetWalkAnimB(seqID int) { p.walkanim_b = seqID }
func (p *Player) SetWalkAnimL(seqID int) { p.walkanim_l = seqID }
func (p *Player) SetWalkAnimR(seqID int) { p.walkanim_r = seqID }
func (p *Player) SetRunAnim(seqID int)   { p.runanim = seqID }
```

**Implementer's job**: confirm the exact mask constants (`MaskFaceCoord` / `MaskAnim` / `MaskSpotAnim` or their current names) and spotanim field names from `modules/world/player.go`. If any are missing, add them following the existing mask pattern.

### 5. Testing

**`pkg/script/handlers_player_test.go`** — mockPlayer-driven unit tests:

- `TestStat` — mp seeds Stat=50; handler pushes 50.
- `TestStatBase` — same for base.
- `TestStatTotal` — mp returns sum of baseLevels; handler pushes it. (Add `StatTotal()` to mockPlayer or compute via 21 `StatBase` calls in the handler — design call: handler iterates via `StatBase` to avoid another interface method.)
- `TestStatAdvance` — captures AddXP(id, xp) on mp; verifies correct id/xp values.
- `TestStatAddFormula` — seeds stat+base, runs handler with constant+percent, asserts SetCurLevel called with expected value.
- `TestCoord` — mp returns CoordPacked; handler pushes.
- `TestPTeleJump` — captures TeleJump(x,z,level); verifies unpack is correct for `packed = (level<<28)|(x<<14)|z`.
- `TestFaceSquare` — captures FaceSquare(x,z).
- `TestAnim` — captures PlayAnim(seq, delay).
- `TestPlayerHandlersRequireActivePlayer` — all 23 handlers return error when `Self == nil` (parametrised).

**`modules/world/script_test.go`** — one end-to-end test:

- `TestTelejumpViaScript`: LOGIN-style script that calls `push_constant_int <coord>, p_telejump`. Verifies `p.x`, `p.z`, `p.level` match the popped coord and that `p.tele && p.jump` are set.
- Optional extension: `TestStatAdvanceEmitsUpdateStat`: add XP via script, tick once, verify `OpUpdateStat` bytes on the wire.

### 6. LOC estimate

| File | LOC |
|---|---|
| `pkg/script/handlers_player.go` | ~220 |
| `pkg/script/handlers_player_test.go` | ~220 |
| `pkg/script/active.go` (diff) | +45 |
| `pkg/script/handlers.go` (diff) | +25 (register 24) |
| `pkg/script/runner_test.go` (diff) | +60 (extend mockPlayer) |
| `modules/world/player_script.go` (diff) | +120 |
| `modules/world/script_test.go` (diff) | +50 |
| **Total** | **~740** |

## Key design calls

- **`P_WALK` stubbed**, not implemented. Real walk needs pathfinder + the existing waypoint queue integrated with `processInteractions`. That's a self-contained sub-spec later.
- **Stat write surface is `SetCurLevel(id, level)`** — the absolute-set shape. Handlers compute deltas; Player.SetCurLevel just stores. The existing dirty-compare in `updateStats()` emits `OpUpdateStat` on mismatch. Zero new wire code.
- **`AddXP` is a server-side-only write for S5c.** `baseLevels` update from XP curve deferred — add after we confirm the exact `getLevelByExp` table.
- **`STAT_RANDOM` uses plain `rand.IntN`** with a "probability tuning TBD" comment. Adequate for smoke-testing script flow; refine when real success probabilities matter.
- **Interface extension is 15 methods.** Large but most are single-line field reads/writes. The compile-time assertion in `message_game.go` catches drift.
- **No new Player fields needed** (spotanim excepted — implementer verifies). Everything slots into existing `stats/levels/baseLevels/x/z/level/tele/jump/faceSquareX/faceSquareZ/animID/animDelay/readyanim/turnanim/walkanim*/runanim`.

## Gotchas

- **Coord mask**: TS `packCoord` uses 14-bit x/z, 4-bit level. `level` mask is `0x3` not `0xf` (only 4 levels in RS2 rev 225). Verify against TS during impl.
- **Pop order for STAT_ADD / SUB / BOOST / DRAIN / HEAL**: TS `popInts(3)` fills top-down → stack top is `percent`, middle `constant`, bottom `stat`. Easy to get backwards; worth an explicit comment in each handler.
- **Stat index range**: always `[0, 21)` — 21 skills in rev 225. Handlers return error on OOB; tests should cover boundary cases (0, 20, 21, -1).
- **XP scaling convention**: TS stores XP as `xp * 10` internally (so `stat_advance 50` adds 5 XP). We store raw in `stats[i]`. Implementer verifies and either matches TS scaling (multiply in AddXP) or documents the divergence.
- **Mask constants**: the spec guesses `MaskFaceCoord` / `MaskAnim` / `MaskSpotAnim`. Implementer checks `modules/world/player_masks.go` and uses the actual names.
- **Spotanim fields on Player**: survey didn't confirm they exist. Implementer adds if missing.
- **TeleJump clears `jump` flag**: survey noted "`tele` and `jump` cleared in ResetMasks after emission" — good, the one-shot semantic is already in place.
