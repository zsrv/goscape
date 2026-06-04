# NAI-120 — Bundle 1 TS-Source Signature Audit

**Date:** 2026-05-07
**Input:** Bundle 0 findings — `docs/superpowers/investigations/2026-05-07-nai-120-bundle0-findings.md` (commit `47fa923` per spec §4.5 / 50e3a7d HEAD-at-pre-flight)
**Scope:** Per-entry TS-source signature audit for all 12 entries in the NAI-120 missing-handler list (11 × (D) opcodes + 1 × V-PARTIAL varn binding).

This document is the complete Stage 2 reference. A plan-author can read only this file plus the rs2 source to write per-handler tests and ports without consulting Engine-TS.

---

## Table of Contents

1. [busy2 (OpBusy2) — opcode 2006](#1-busy2-opbusy2--opcode-2006)
2. [inv_dropitem_delayed (OpInvDropItemDelayed) — opcode 4310](#2-inv_dropitem_delayed-opinvdropitemdelayed--opcode-4310)
3. [map_multiway (OpMapMultiway) — opcode 1014](#3-map_multiway-opmapmultiway--opcode-1014)
4. [npc_finduid (OpNpcFindUID) — opcode 2521](#4-npc_finduid-opnpcfinduid--opcode-2521)
5. [npc_heropoints (OpNpcHeroPoints) — opcode 2524](#5-npc_heropoints-opnpcheropoints--opcode-2524)
6. [npc_range (OpNpcRange) — opcode 2531](#6-npc_range-opnpcrange--opcode-2531)
7. [npc_statadd (OpNpcStatAdd) — opcode 2538](#7-npc_statadd-opnpcstatadd--opcode-2538)
8. [npc_statsub (OpNpcStatSub) — opcode 2540](#8-npc_statsub-opnpcstatsub--opcode-2540)
9. [p_opnpct (OpPOpNpcT) — opcode 2079](#9-p_opnpct-oppopnpct--opcode-2079)
10. [p_opplayer (OpPOpPlayer) — opcode 2081](#10-p_opplayer-oppopplayer--opcode-2081)
11. [spotanim_npc (OpSpotAnimNpc) — opcode 2547](#11-spotanim_npc-opspotanimnpc--opcode-2547)
12. [%npc_combat_xp_multiplier — V-PARTIAL varn binding](#12-npc_combat_xp_multiplier--v-partial-varn-binding)

---

## 1. `busy2` (`OpBusy2`) — opcode 2006

**TS impl location:** `Engine-TS/src/engine/script/handlers/PlayerOps.ts:898-900`

**TS impl verbatim:**
```typescript
    // https://x.com/JagexAsh/status/1791053667228856563
    [ScriptOpcode.BUSY2]: state => {
        state.pushInt(state.activePlayer.hasInteraction() || state.activePlayer.hasWaypoints() ? 1 : 0);
    },
```

**Pop/push signature:** No pops. Pushes one `int` (1 if player has an interaction target OR has waypoints queued, else 0).

**Side effects:** None — pure read.

**Goscape sibling pattern:** Closest sibling is the `OpBusy` pattern (not yet dispatched in goscape, but its TS form at `PlayerOps.ts:893-895` is `state.pushInt(state.activePlayer.busy() || state.activePlayer.loggingOut ? 1 : 0)`). The nearest dispatched sibling in goscape is `handleRunEnergy` at `pkg/script/handlers_player.go:637-643` — pure read, requires `ActivePlayer`, pushes one int. Use `requireActivePlayer` guard.

**Edge cases:**
- No active player: return error via `requireActivePlayer`. TS `busy2` is registered as a plain handler (not `checkedHandler`) — however the TS engine always runs player-context opcodes with `activePlayer` set. In goscape, `requireActivePlayer` is the correct defensive guard (matching how all other player reads are implemented).
- `hasInteraction()` in TS = player has a non-nil `target` field (interaction is set). Mirrors `p.target \!= nil` in goscape (`modules/world/interaction.go` — the field is private `target entity`).
- `hasWaypoints()` in TS = player has waypoints queued. In goscape, `p.hasWaypoints()` is defined at `modules/world/interaction.go:297` and returns `p.waypointIndex >= 0`.
- Neither `HasInteraction()` nor `HasWaypoints()` currently exists on the `ActivePlayer` interface in `pkg/script/active.go`. Both must be added.

**Dependencies:**
- Needs `HasInteraction() bool` added to `ActivePlayer` interface — add to `pkg/script/active.go:ActivePlayer` interface and implement on `*Player` in `modules/world/player_script.go` (delegates to `p.target \!= nil`).
- Needs `HasWaypoints() bool` added to `ActivePlayer` interface — add to `pkg/script/active.go:ActivePlayer` interface and implement on `*Player` in `modules/world/player_script.go` (delegates to `p.hasWaypoints()`).

**Test-case skeletons:**
- Happy-path interaction: `&ScriptState{StackCapacity: 32, Pointers: PtrActivePlayer, Self: &mockPlayer{hasInteraction: true, hasWaypoints: false}}` → after handler, `PopInt()` == 1.
- Happy-path waypoints: `&ScriptState{StackCapacity: 32, Pointers: PtrActivePlayer, Self: &mockPlayer{hasInteraction: false, hasWaypoints: true}}` → `PopInt()` == 1.
- Neither set: `Self: &mockPlayer{hasInteraction: false, hasWaypoints: false}` → `PopInt()` == 0.
- Nil Self: `&ScriptState{StackCapacity: 32}` → handler returns non-nil error (requireActivePlayer guard).
- Used in `auto_retaliate.rs2:3,27` — the rs2 test for `playerhit_n_retaliate` and `pvp_retaliate` both check `busy2 = false` before queuing an op.

---

## 2. `inv_dropitem_delayed` (`OpInvDropItemDelayed`) — opcode 4310

**TS impl location:** `Engine-TS/src/engine/script/handlers/InvOps.ts:188-209`

**TS impl verbatim:**
```typescript
    [ScriptOpcode.INV_DROPITEM_DELAYED]: checkedHandler(ActivePlayer, state => {
        const [inv, coord, obj, count, duration, delay] = state.popInts(6);

        const invType: InvType = check(inv, InvTypeValid);
        const position: CoordGrid = check(coord, CoordValid);
        const objType: ObjType = check(obj, ObjTypeValid);
        check(count, ObjStackValid);
        check(duration, DurationValid);

        if (\!state.pointerGet(ProtectedActivePlayer[state.intOperand]) && invType.protect && invType.scope \!== InvType.SCOPE_SHARED) {
            throw new Error(`$inv requires protected access: ${invType.debugname}`);
        }

        const player = state.activePlayer;
        const completed = player.invDel(invType.id, objType.id, count);
        if (completed == 0) {
            return;
        }

        const floorObj: Obj = new Obj(position.level, position.x, position.z, EntityLifeCycle.DESPAWN, objType.id, completed);
        World.objDelayedQueue.addTail(new ObjDelayedRequest(floorObj, duration, delay, player.hash64));
    }),
```

**Pop/push signature:** Pops 6 ints, top-to-bottom: `delay`, `duration`, `count`, `obj` (ObjType id), `coord` (packed coord int), `inv` (InvType id). No push.

**Side effects:**
1. Removes `count` units of `obj` from the player's inv (via `invDel`). If `invDel` returns 0 (nothing removed), returns early — no floor drop.
2. Constructs a floor Obj at `position` (DESPAWN lifecycle) and enqueues it on `World.objDelayedQueue` for spawning after `delay` ticks, despawning after `duration` ticks, with private visibility for the owner player (`player.hash64`).

**Goscape sibling pattern:** Closest is `handleInvDropSlot` at `pkg/script/handlers_inv.go:512` — same inv-protect gate, same `checkDuration`, same `AddObj` routing. The key difference is the `delay` parameter and a new world-side `AddObjDelayed` method. `handleInvDel` at `pkg/script/handlers_inv.go:308` provides the `invDel` pattern.

**Edge cases:**
- No active player: `requireActivePlayer` gate (checkedHandler(ActivePlayer) in TS). No Protected requirement by default — but the inv-protect gate reads `ProtectedActivePlayer[state.intOperand]` pointer flag and throws when invType.protect && scope \!= SHARED and the pointer is absent.
- `invDel` returns 0: early-return, no floor obj queued (matches TS `if (completed == 0) return`).
- ObjTypeValid / InvTypeValid / DurationValid / ObjStackValid: apply in pop order. Goscape uses `checkInvType`, `checkCoord`, and needs `checkObjType`, `checkObjStack`, `checkDuration` (see `handleInvDropSlot` for the duration check at `handlers_inv.go`).
- `count` must pass `ObjStackValid` (not `NumberNotNull`) — in TS that is `ObjStackValid = new ScriptInputRangeValidator(1, 2147483647, 'ObjStack')` (rejects 0 and negatives). Goscape's `handleInvDropSlot` does not use ObjStackValid — verify this is needed by cross-referencing ScriptValidators.ts.
- The `delay` parameter has no TS validator call shown (only the 5 checks for inv, coord, obj, count, duration appear; delay is a raw popInt used only in the `ObjDelayedRequest` constructor).

**Dependencies:**
- Needs `AddObjDelayed(level, x, z, typeID, count, duration, delay, receiverID int) ActiveObj` added to the `WorldLike` interface in `pkg/script/state.go` (or a variant that doesn't return ActiveObj, since TS doesn't set activeObj here). Add implementation to `modules/world/server.go` or equivalent.
- The `ObjDelayedRequest` world-side mechanism (`World.objDelayedQueue`) must be implemented in `modules/world/` (enqueus a deferred floor spawn). No goscape counterpart exists yet.

**Test-case skeletons:**
- Happy-path: player owns 5 arrows in worn inv, pop `[worn, coord, arrow_id, 1, 200, 3]` → `invDel` removes 1, world receives `AddObjDelayed` call with count=1, delay=3, duration=200.
- `invDel` returns 0 (item not in inv): world's `AddObjDelayed` must NOT be called.
- Nil Self: `&ScriptState{StackCapacity: 32}` → error from requireActivePlayer.
- Protect gate: invType with protect=true, scope=per-player, and `Pointers` lacks `PtrProtectedActivePlayer` bit → error `"$inv requires protected access: <name>"`.
- Invalid inv id: error from InvTypeValid check.

---

## 3. `map_multiway` (`OpMapMultiway`) — opcode 1014

**TS impl location:** `Engine-TS/src/engine/script/handlers/ServerOps.ts:376-380`

**TS impl verbatim:**
```typescript
    [ScriptOpcode.MAP_MULTIWAY]: state => {
        const coord = state.popInt();

        state.pushInt(World.gameMap.isMulti(coord) ? 1 : 0);
    }
```

**Pop/push signature:** Pops 1 `coord` (packed int). Pushes 1 `int` (1 if the tile at that coord is a multi-combat zone, else 0).

**Side effects:** None — pure read.

**Goscape sibling pattern:** Closest is `handleMapBlocked` at `pkg/script/handlers_map.go:190-207`. Same structure: pop packed coord → unpack → query WorldLike → push boolean int. The world-side query (`IsMapBlocked`) is on the `WorldLike` interface at `pkg/script/state.go:70`. `IsMulti` must be added analogously.

**Edge cases:**
- TS does NOT call `CoordValid` on the popped coord (unlike `handleMapBlocked` which does call `checkCoord`). The TS impl passes the raw packed int directly to `isMulti(coord)`. TS-faithful goscape port should NOT call `checkCoord` — pass the raw packed coord to `WorldLike.IsMulti`.
- No active player/npc required (server-context op).
- `World.gameMap.isMulti(coord)` in TS takes a packed coord and unpacks internally: `pkg/gamemap/multimap.go:17` has `func (gm *GameMap) IsMulti(x, z, level int) bool` which takes unpacked args. The goscape `WorldLike.IsMulti` method signature must match — either add a packed-coord variant or unpack before calling.

**Dependencies:**
- Needs `IsMulti(level, x, z int) bool` added to the `WorldLike` interface in `pkg/script/state.go` and implemented on the world side (delegates to `pkg/gamemap/multimap.go:17` `GameMap.IsMulti`). Check how goscape's `modules/world/server.go` exposes `IsMapBlocked` for the pattern.

**Test-case skeletons:**
- Multi tile: pop packed coord for a known multi zone → push 1.
- Non-multi tile: pop packed coord for a non-multi zone → push 0.
- Coord unpacking: validate that level/x/z bits are decoded correctly (same as `unpackCoord` at `handlers_player.go:18`).
- No WorldLike: if `s.World == nil` → handler should return an error (follow `handleMapClock` pattern at `handlers_server.go:7` which gates on `s.World`).

---

## 4. `npc_finduid` (`OpNpcFindUID`) — opcode 2521

**TS impl location:** `Engine-TS/src/engine/script/handlers/NpcOps.ts:26-40`

**TS impl verbatim:**
```typescript
    [ScriptOpcode.NPC_FINDUID]: state => {
        const npcUid = state.popInt();
        const slot = npcUid & 0xffff;
        const expectedType = (npcUid >> 16) & 0xffff;
        const npc = World.getNpc(slot);

        if (\!npc || npc.type \!== expectedType) {
            state.pushInt(0);
            return;
        }

        state.activeNpc = npc;
        state.pointerAdd(ActiveNpc[state.intOperand]);
        state.pushInt(1);
    },
```

**Pop/push signature:** Pops 1 `int` (packed NPC UID: `(typeId << 16) | slot`). Pushes 1 `int` (1 on hit, 0 on miss).

**Side effects:** On hit: sets `state.activeNpc` (primary or secondary slot per `state.intOperand`) and adds the corresponding ActiveNpc pointer.

**Goscape sibling pattern:** Direct parallel to `handleFindUID` (player) at `pkg/script/handlers_player.go:882-899`. Both pop a UID, look up entity, set active slot + pointer, push 1/0. For NPC, use `setActiveNpcSlot` at `pkg/script/handlers_npc.go:71-83` (which handles primary/secondary slot based on `s.Script.IntOperands[s.PC]`). The lookup uses `s.Npcs.FindNpcByUID(uid int) ActiveNpc` — check `NpcLookup` interface in `pkg/script/state.go`.

**Edge cases:**
- TS extracts `slot = uid & 0xffff` and `expectedType = (uid >> 16) & 0xffff`, then does `World.getNpc(slot)` (slot-indexed array lookup). If slot OOB or NPC's `type` doesn't match `expectedType`, push 0.
- Goscape: `s.Npcs` may be nil → push 0 (match the nil-Npcs degradation pattern from `handleNpcFind`).
- NpcUID packing matches `handleNpcUID` at `handlers_npc.go:192`: `(typeId << 16) | nid`. The lookup must verify type-id matches.
- The TS pattern does NOT call `checkedHandler` (no ActiveNpc pre-required) — this is a find-and-set opcode.
- `intOperand` (0 = primary slot, 1 = secondary) is read via `s.Script.IntOperands[s.PC]` by `setActiveNpcSlot`.

**Dependencies:**
- Needs `FindNpcByUID(uid int) ActiveNpc` on the `NpcLookup` interface in `pkg/script/state.go` (or equivalent slot+type decomposition in the handler). Check if `NpcLookup` already has this method; if not, add it and implement on world side.

**Test-case skeletons:**
- Happy-path primary slot: push uid `(typeId<<16)|slot`, mock Npcs returns matching NPC → `s.ActiveNpc` set, `s.Pointers & PtrActiveNpc \!= 0`, PopInt() == 1.
- Happy-path secondary slot: `Script.IntOperands[PC] = 1` → `s.OtherActiveNpc` set, `PtrActiveNpc2` set.
- Type mismatch: uid with wrong `expectedType`, NPC exists at slot but `npc.type \!= expectedType` → push 0, no pointer set.
- Nil Npcs: `s.Npcs == nil` → push 0.
- Slot OOB: Npcs returns nil for slot → push 0.

---

## 5. `npc_heropoints` (`OpNpcHeroPoints`) — opcode 2524

**TS impl location:** `Engine-TS/src/engine/script/handlers/NpcOps.ts:478-480`

**TS impl verbatim:**
```typescript
    // https://x.com/JagexAsh/status/1704492467226091853
    [ScriptOpcode.NPC_HEROPOINTS]: checkedHandler([ScriptPointer.ActivePlayer, ...ActiveNpc], state => {
        state.activeNpc.heroPoints.addHero(state.activePlayer.hash64, check(state.popInt(), NumberNotNull));
    }),
```

**Pop/push signature:** Pops 1 `int` (damage/contribution amount, must not be -1). No push.

**Side effects:** Calls `state.activeNpc.heroPoints.addHero(playerHash64, amount)` — writes the player's contribution to the NPC's hero-point ledger. This determines which player gets the drop when the NPC dies.

**Goscape sibling pattern:** `handleNpcDamage` at `pkg/script/handlers_npc.go:291-300` is the closest: requires ActiveNpc + NumberNotNull check on popped int, writes to NPC state. The guard is `checkedHandler([ActivePlayer, ...ActiveNpc])` — requires **both** `ActivePlayer` AND `ActiveNpc`. In goscape terms: check `PtrActivePlayer \!= 0 && s.Self \!= nil` AND `s.ActiveNpc \!= nil`.

**Edge cases:**
- Requires both Active Player AND ActiveNpc — unique dual guard. Use `requireActivePlayer` + `requireActiveNpc` in sequence.
- Amount must pass `NumberNotNull` (reject -1).
- `state.activePlayer.hash64` in TS is the player's 64-bit account hash — in goscape, `s.Self.UID()` serves as the player identifier (or a 64-bit equivalent if one is available on `ActivePlayer`).
- `heroPoints.addHero` is a side-effect on the NPC's hero-point tracker. No equivalent exists on the `ActiveNpc` interface — must be added.

**Dependencies:**
- Needs `AddHeroPoints(playerUID int, amount int)` added to the `ActiveNpc` interface in `pkg/script/active.go` and implemented on `*Npc` in `modules/world/npc_script.go`. The underlying `Npc.heroPoints` field exists (visible in Engine-TS Npc.ts:76), so the goscape world-side `*Npc` needs an analogous `HeroPoints` tracker.
- The `HeroPoints` structure (a per-player contribution ledger, capped at 16 entries per TS `HeroPoints(16)`) must be implemented or stubbed in `modules/world/`.
- `s.Self.UID()` is already on `ActivePlayer` interface at `pkg/script/active.go:388`. Use this as `playerUID`.

**Test-case skeletons:**
- Happy-path: ActivePlayer + ActiveNpc both set, pop 42 → `ActiveNpc.AddHeroPoints(playerUID, 42)` called.
- Nil ActiveNpc: error from requireActiveNpc.
- Nil ActivePlayer: error from requireActivePlayer.
- Amount = -1: error from `checkNotNull(-1, "NPC_HEROPOINTS")`.
- Amount = 0: passes (0 is a valid contribution — NumberNotNull only rejects -1).

---

## 6. `npc_range` (`OpNpcRange`) — opcode 2531

**TS impl location:** `Engine-TS/src/engine/script/handlers/NpcOps.ts:152-168`

**TS impl verbatim:**
```typescript
    [ScriptOpcode.NPC_RANGE]: checkedHandler(ActiveNpc, state => {
        const coord: CoordGrid = check(state.popInt(), CoordValid);

        const npc = state.activeNpc;
        if (coord.level \!== npc.level) {
            state.pushInt(-1);
        } else {
            state.pushInt(
                CoordGrid.distanceTo(npc, {
                    x: coord.x,
                    z: coord.z,
                    width: 1,
                    length: 1
                })
            );
        }
    }),
```

**Pop/push signature:** Pops 1 `coord` (packed int, CoordValid checked). Pushes 1 `int` (Chebyshev distance from NPC to the coord tile; or -1 if different level).

**Side effects:** None — pure read.

**Goscape sibling pattern:** `handleNpcCoord` at `pkg/script/handlers_npc.go:106-113` is the closest: requires ActiveNpc, reads NPC position fields. For the distance computation, the goscape equivalent of `CoordGrid.distanceTo` with `width=1, length=1` is Chebyshev distance: `max(abs(npc.x - coord.x), abs(npc.z - coord.z))`.

**Edge cases:**
- Calls `CoordValid` (i.e. `checkCoord`) on the popped coord — unlike `MAP_MULTIWAY`, this DOES validate.
- If `coord.level \!= npc.level`: push -1 (sentinel for "different level, no range").
- `CoordGrid.distanceTo` with a 1×1 target is Chebyshev (max of absolute differences). For an NPC with size > 1, TS uses the NPC's own `x/z/width/length` but compares against the coord with width=1/length=1. The goscape equivalent: `max(abs(npcX - coordX), abs(npcZ - coordZ))` but accounting for NPC size (Chebyshev from the NPC entity's SW corner + size to the target 1×1 tile). Check `CoordGrid.distanceTo` in Engine-TS for exact formula — it is `Math.max(0, Math.abs(a.x - b.x) - (b.width - 1), Math.abs(a.z - b.z) - (b.length - 1))`.
- Nil ActiveNpc: error from `requireActiveNpc`.

**Dependencies:**
- Needs a Chebyshev-distance helper or inline computation using `NpcX()`, `NpcZ()` from the `ActiveNpc` interface (already present at `pkg/script/active.go:574-575`).
- If NPC size > 1 matters: needs `NpcSize() int` on `ActiveNpc` (or width/length accessors). Check if NPC type size is needed for the distance calc. In the inner-ring call-sites (`player_combat.rs2:6,27`) the coord popped is `coord` (player's current coord) and the npc is the active combat target — npc size matters for correct range calculation.

**Test-case skeletons:**
- Same level, adjacent tile: NPC at (3222, 3218, 0), pop coord (3223, 3218, 0) → push 1.
- Same level, diagonal: NPC at (3222, 3218, 0), coord (3223, 3219, 0) → push 1 (Chebyshev).
- Different level: NPC at level 0, coord at level 1 → push -1.
- Nil ActiveNpc: `&ScriptState{StackCapacity: 32}` → error.
- Invalid coord: pop -1 (negative packed coord fails CoordValid) → error.

---

## 7. `npc_statadd` (`OpNpcStatAdd`) — opcode 2538

**TS impl location:** `Engine-TS/src/engine/script/handlers/NpcOps.ts:492-504`

**TS impl verbatim:**
```typescript
    [ScriptOpcode.NPC_STATADD]: checkedHandler(ActiveNpc, state => {
        const [stat, constant, percent] = state.popInts(3);

        check(stat, NpcStatValid);
        check(constant, NumberNotNull);
        check(percent, NumberNotNull);

        const npc = state.activeNpc;
        const base = npc.baseLevels[stat];
        const current = npc.levels[stat];
        const added = current + ((constant + (base * percent) / 100) | 0);
        npc.levels[stat] = Math.min(added, 255);
    }),
```

**Pop/push signature:** Pops 3 ints, `popInts(3)` destructures top-to-bottom as `[stat, constant, percent]` — meaning `percent` is popped first (top of stack), then `constant`, then `stat` (bottom). No push.

**Side effects:** Writes `npc.levels[stat]` = `min(current + trunc(constant + (base * percent) / 100), 255)`. Mutates the NPC's current (boosted) stat level.

**Goscape sibling pattern:** `handleStatAdd` at `pkg/script/handlers_player.go:292-316` is the exact structural twin for players. Same formula. For NPCs: requires ActiveNpc, pops in same order (percent top, constant middle, stat bottom), applies `min(..., 255)`. The NPC version validates with `NpcStatValid` (range [0, 5] — `NpcStatCount = 6` at `pkg/objtype/npctype.go:22`) rather than `PlayerStatValid`.

**Edge cases:**
- `NpcStatValid` in TS: `ScriptInputRangeValidator(NpcStat.ATTACK=0, NpcStat.MAGIC=5, 'NpcStat')` — valid range is [0, 5]. Goscape does not yet have `checkNpcStatID`; add it: `if stat < 0 || stat >= objtype.NpcStatCount { return error }`.
- `constant` and `percent` both require `checkNotNull` (-1 rejected).
- Integer truncation: `(constant + (base * percent) / 100) | 0` is JS bitwise-OR truncation. Go integer division truncates toward zero by default — equivalent for non-negative inputs. Goscape formula: `current + (constant + (base*percent)/100)`.
- Cap at 255: `min(added, 255)`.
- No stat write method on `ActiveNpc` interface. Must be added.

**Dependencies:**
- Needs `SetNpcStat(stat int, level int)` added to `ActiveNpc` interface in `pkg/script/active.go` and implemented on `*Npc` in `modules/world/npc_script.go` (writes `n.levels[stat]` with bounds check: `if stat < 0 || stat >= NpcStatCount { return }`).
- Needs `checkNpcStatID` helper in `pkg/script/handlers_npc.go` (or inline): `if id < 0 || id >= objtype.NpcStatCount { return fmt.Errorf("%s: npc stat id out of range", op) }`.

**Test-case skeletons:**
- Happy-path: ActiveNpc with baseLevels[0]=70, levels[0]=50, pop `[0, 5, 10]` → levels[0] = 50 + (5 + 70*10/100) = 50 + 12 = 62.
- Cap at 255: levels[0]=250, base=100, pop `[0, 10, 100]` → 250 + 110 = 360, clamped to 255.
- Nil ActiveNpc: error.
- stat=-1: NpcStatValid error.
- stat=6 (OOB): NpcStatValid error.
- constant=-1: NumberNotNull error.
- percent=-1: NumberNotNull error.

---

## 8. `npc_statsub` (`OpNpcStatSub`) — opcode 2540

**TS impl location:** `Engine-TS/src/engine/script/handlers/NpcOps.ts:506-518`

**TS impl verbatim:**
```typescript
    [ScriptOpcode.NPC_STATSUB]: checkedHandler(ActiveNpc, state => {
        const [stat, constant, percent] = state.popInts(3);

        check(stat, NpcStatValid);
        check(constant, NumberNotNull);
        check(percent, NumberNotNull);

        const npc = state.activeNpc;
        const base = npc.baseLevels[stat];
        const current = npc.levels[stat];
        const subbed = current - ((constant + (base * percent) / 100) | 0);
        npc.levels[stat] = Math.max(subbed, 0);
    }),
```

**Pop/push signature:** Pops 3 ints, same `popInts(3)` order as `NPC_STATADD` — `percent` top, `constant` middle, `stat` bottom. No push.

**Side effects:** Writes `npc.levels[stat]` = `max(current - trunc(constant + (base * percent) / 100), 0)`. Clamps at 0 (stat cannot go negative).

**Goscape sibling pattern:** `handleStatSub` at `pkg/script/handlers_player.go:319-347` is the exact structural twin for players. Same formula, clamped at 0 instead of min-255. For NPC: same guard, same pop order, same validation (`checkNpcStatID` + `checkNotNull` × 2), uses `NpcBaseStat(stat)` and `NpcStat(stat)` reads, then `SetNpcStat(stat, result)` write.

**Edge cases:** Identical to `npc_statadd` except the floor is 0 (not 255). If the result would be negative, write 0.

**Dependencies:** Same as `npc_statadd` — needs `SetNpcStat` on `ActiveNpc` interface and `checkNpcStatID` helper.

**Test-case skeletons:**
- Happy-path: base=70, current=50, pop `[0, 5, 10]` → 50 - (5 + 7) = 38.
- Floor at 0: current=5, pop `[0, 100, 100]` → 5 - (100 + 70) < 0, clamped to 0.
- Nil ActiveNpc: error.
- stat=6: NpcStatValid error.
- constant=-1: NumberNotNull error.

---

## 9. `p_opnpct` (`OpPOpNpcT`) — opcode 2079

**TS impl location:** `Engine-TS/src/engine/script/handlers/PlayerOps.ts:417-421`

**TS impl verbatim:**
```typescript
    // https://x.com/JagexAsh/status/1791472651623370843
    [ScriptOpcode.P_OPNPCT]: checkedHandler(ProtectedActivePlayer, state => {
        const spellId: number = check(state.popInt(), NumberNotNull);
        state.activePlayer.stopAction();
        state.activePlayer.setInteraction(Interaction.SCRIPT, state.activeNpc, ServerTriggerType.APNPCT, spellId);
    }),
```

**Pop/push signature:** Pops 1 `int` (spellId / spellCom — the UI component id of the spell; must not be -1). No push.

**Side effects:**
1. `activePlayer.stopAction()` — clears interaction and pending action.
2. `activePlayer.setInteraction(Interaction.SCRIPT, state.activeNpc, ServerTriggerType.APNPCT, spellId)` — anchors the player on the active NPC with APNPCT trigger and stores `spellId` as the target-subject com.

**Goscape sibling pattern:** `handleP_OpNpc` at `pkg/script/handlers_player.go:859-876` is the direct sibling. `P_OPNPC` calls `s.Self.SetInteractionScriptNpc(s.ActiveNpc, op)` with op in [1,5]. `P_OPNPCT` differs: no op-range check (spellId is arbitrary), passes spellId as the com argument (stored in `p.targetSubject.com`). In goscape, `(*Player).SetInteraction` at `modules/world/interaction.go:72` takes `(kind, target, op, com)` — the com field is where spellId goes.

**Edge cases:**
- Guard: `ProtectedActivePlayer` — must use `requireProtectedActivePlayer`.
- ActiveNpc must be non-nil (TS accesses `state.activeNpc` unconditionally) — add `requireActiveNpc` before `setInteraction`.
- spellId must pass `NumberNotNull` (-1 rejected). Any other int (including 0) is valid.
- `targetOpNpcT = 8` in goscape's interaction layer (at `modules/world/interaction.go:35`). The TS `ServerTriggerType.APNPCT` = 9 in goscape at `pkg/script/trigger.go:18`. The `op` stored by `SetInteraction` is the targetOp; for NpcT interactions goscape uses sentinel `targetOpNpcT = 8` at the interaction layer. Confirm how `SetInteractionScriptNpc` vs a new `SetInteractionScriptNpcT` method should carry the spellId com.
- No `SetInteractionScriptNpcT` method on `ActivePlayer` interface — must be added or the existing `SetInteraction` plumbing extended.

**Dependencies:**
- Needs `SetInteractionScriptNpcT(npc ActiveNpc, spellCom int)` added to `ActivePlayer` interface in `pkg/script/active.go` and implemented on `*Player` in `modules/world/player_script.go` (delegates to `p.SetInteraction(InteractionScript, realNpc, targetOpNpcT, spellCom)`).

**Test-case skeletons:**
- Happy-path: ProtectedActivePlayer + ActiveNpc both set, pop spellCom=1234 → `StopAction()` called, `SetInteractionScriptNpcT(activeNpc, 1234)` called.
- Not protected: Pointers has PtrActivePlayer but Protect=false → error from requireProtectedActivePlayer.
- Nil ActiveNpc: error from requireActiveNpc.
- spellId = -1: NumberNotNull error.
- Used at `player_magic.rs2:17` — `p_opnpct(db_getfield($spell_data, magic_spell_table:spellcom, 0))` queues the autocast spell interaction.

---

## 10. `p_opplayer` (`OpPOpPlayer`) — opcode 2081

**TS impl location:** `Engine-TS/src/engine/script/handlers/PlayerOps.ts:1009-1020`

**TS impl verbatim:**
```typescript
    // https://x.com/JagexAsh/status/1791472651623370843
    [ScriptOpcode.P_OPPLAYER]: checkedHandler(ProtectedActivePlayer, state => {
        const type = check(state.popInt(), NumberNotNull) - 1;
        if (type < 0 || type >= 5) {
            throw new Error(`Invalid opplayer: ${type + 1}`);
        }
        const target = state._activePlayer2;
        if (\!target) {
            return;
        }
        state.activePlayer.stopAction();
        state.activePlayer.setInteraction(Interaction.SCRIPT, target, ServerTriggerType.APPLAYER1 + type);
    }),
```

**Pop/push signature:** Pops 1 `int` (1-indexed op, 1..5; must not be -1). No push.

**Side effects:**
1. Validates popped value is in [1,5] (after subtracting 1, resulting `type` must be [0,4]).
2. If `state._activePlayer2` is nil: silent return (no error, no interaction set).
3. `activePlayer.stopAction()`.
4. `activePlayer.setInteraction(Interaction.SCRIPT, target, ServerTriggerType.APPLAYER1 + type)` — anchors the player on the secondary active player with trigger APPLAYER1+type.

**Goscape sibling pattern:** `handleP_OpNpc` at `pkg/script/handlers_player.go:859-876` is the structural sibling: ProtectedActivePlayer guard, pop op (1-indexed), validate [1,5], StopAction, SetInteraction. The difference is the target is `s.Self2` (secondary active player) instead of `s.ActiveNpc`, and the trigger is `TriggerApPlayer1 + type` (at `pkg/script/trigger.go:90`).

**Edge cases:**
- Guard: ProtectedActivePlayer — use `requireProtectedActivePlayer`.
- After subtracting 1: if result < 0 or >= 5, throw error. In goscape: `if type < 0 || type >= 5 { return fmt.Errorf(...) }`.
- If `s.Self2` is nil: **silent return** (no error) — matches TS `if (\!target) { return; }`. This is the only handler with this pattern.
- Uses `Self2` (secondary active player at `pkg/script/state.go:222`) — requires both `s.Self2 \!= nil` AND PtrActivePlayer2 to be set. But per TS, when `target == null` it just silently returns. No error is thrown.
- Trigger computation: `TriggerApPlayer1 + type` where type is 0-based (0..4), giving `TriggerApPlayer1`..`TriggerApPlayer5` at `pkg/script/trigger.go:90-94`.

**Dependencies:**
- Needs `SetInteractionScriptPlayer(player2 ActivePlayer, op int)` added to `ActivePlayer` interface in `pkg/script/active.go` and implemented on `*Player` in `modules/world/player_script.go` (delegates to `p.SetInteraction(InteractionScript, realPlayer2, op, -1)` where op is the 1-based op slot — or alternatively `TriggerApPlayer1 + (op - 1)` computed at the interaction level). Check how `player_interaction_trigger.go:5-11` routes APPLAYER ops.

**Test-case skeletons:**
- Happy-path: ProtectedActivePlayer + Self2 set, pop 2 → StopAction called, `SetInteractionScriptPlayer(Self2, 2)` called (APPLAYER2 trigger).
- Nil Self2: pop 1, Self2 nil → silent return, no error, no interaction.
- Not protected: error from requireProtectedActivePlayer.
- op=0: check(0, NumberNotNull) passes (0 is not -1), then 0-1=-1 < 0 → error "Invalid opplayer: 0".
- op=6: type=5 >= 5 → error "Invalid opplayer: 6".
- op=-1: NumberNotNull error.
- Used at `auto_retaliate.rs2:45` — `p_opplayer(2)` from `pvp_retaliate` queue, which also checks `busy2 = false`.

---

## 11. `spotanim_npc` (`OpSpotAnimNpc`) — opcode 2547

**TS impl location:** `Engine-TS/src/engine/script/handlers/NpcOps.ts:282-288`

**TS impl verbatim:**
```typescript
    [ScriptOpcode.SPOTANIM_NPC]: checkedHandler(ActiveNpc, state => {
        const delay = check(state.popInt(), NumberNotNull);
        const height = check(state.popInt(), NumberNotNull);
        const spotanimType: SpotanimType = check(state.popInt(), SpotAnimTypeValid);

        state.activeNpc.spotanim(spotanimType.id, height, delay);
    }),
```

**Pop/push signature:** Pops 3 ints, top-to-bottom: `delay` (NumberNotNull), `height` (NumberNotNull), `spotanim` id (SpotAnimTypeValid). No push.

**Side effects:** Calls `state.activeNpc.spotanim(spotanimType.id, height, delay)` — queues a spotanim graphic on the NPC this tick.

**Goscape sibling pattern:** `handleSpotAnimPl` at `pkg/script/handlers_player.go:658-674` is the direct structural twin for players: same pop order (delay top, height middle, spotanim bottom), same NumberNotNull on delay+height, same SpotAnimTypeValid on spotanim id. For NPC: use `requireActiveNpc`, same pop order, call `s.ActiveNpc.PlaySpotAnim(spotanim, height, delay)` (or equivalent method to add).

**Edge cases:**
- `delay` and `height` both require `NumberNotNull` (reject -1). TS-faithful.
- `spotanim` requires `SpotAnimTypeValid` — the `checkSpotAnimType` helper at `pkg/script/handlers_map.go:209-219` already exists and can be reused.
- No spotanim method on `ActiveNpc` interface yet — must be added.
- The player variant uses `s.Self.PlaySpotAnim(spotanim, height, delay)`. The NPC variant must use an equivalent.
- Note: in `player_magic.rs2:228`, `spotanim_npc` is called with `(db_getfield(...), $duration)` — so duration/delay is calculated from the spell cast, not a constant. In `player_magic.rs2:242`: `spotanim_npc(failedspell_impact, 92, $duration)` — 3 args exactly matching the pop order.

**Dependencies:**
- Needs `PlaySpotAnim(id, height, delay int)` added to `ActiveNpc` interface in `pkg/script/active.go` and implemented on `*Npc` in `modules/world/npc_script.go` (flags NpcMaskSpotAnim, stores spotanim/height/delay for the NPC-info encoder).

**Test-case skeletons:**
- Happy-path: ActiveNpc set, pop `[spotanim=id, height=92, delay=30]` → `s.ActiveNpc.PlaySpotAnim(id, 92, 30)` called.
- Nil ActiveNpc: error.
- delay=-1: NumberNotNull error.
- height=-1: NumberNotNull error.
- Invalid spotanim id (not in SpotanimType config): SpotAnimTypeValid error.
- Valid spotanim id=0: must NOT trigger error (0 is a valid spotanim, SpotAnimTypeValid only rejects IDs not in config registry).

---

## 12. `%npc_combat_xp_multiplier` — V-PARTIAL varn binding

**TS source:** `LostCityRS/Content/scripts/npc/configs/ai_spawn.varn:1`

```
[npc_combat_xp_multiplier]
```

The varn declaration has no explicit default value or type annotation in the file — it declares only the name.

**How the value is established at runtime:**

The `ai_spawn` trigger script at `LostCityRS/Content/scripts/npc/scripts/ai_spawn.rs2:1-3`:
```
[ai_spawn,_]
%npc_combat_xp_multiplier = npc_param(combat_xp_multiplier);
%npc_start_coord = npc_coord;
```

On NPC spawn, this script is executed and writes `npc_param(combat_xp_multiplier)` into `%npc_combat_xp_multiplier`. The `combat_xp_multiplier` param is defined at `LostCityRS/Content/scripts/skill_combat/configs/combat.param:157-159`:

```
// 1000 = 1x
[combat_xp_multiplier]
type=int
default=1000
```

This means: for any NPC that does NOT explicitly set `param=combat_xp_multiplier,<value>` in its `.npc` config, `npc_param(combat_xp_multiplier)` returns the default value **1000**. For anti-macro NPCs (e.g. `antimacro.npc`), explicit `param=combat_xp_multiplier,25` overrides the default to 25.

The value `1000` represents `1.0x` multiplier (the `give_combat_experience` proc scales XP as `scale($multiplier, 1000, ...)` per `skill_combat/scripts/combat.rs2:86-87`: `// default multiplier is 1000`).

**TS initialization:**

TS initializes all integer VarNpc to `0` on NPC `resetEntity()` at `Engine-TS/src/engine/entity/Npc.ts:302`: `this.vars[i] = varn.type === ScriptVarType.INT ? 0 : -1`. However, the `ai_spawn` trigger immediately overwrites the varn with `npc_param(combat_xp_multiplier)` (= 1000 for most NPCs) before combat scripts ever read it. The varn is therefore **1000** for all standard combat NPCs when the combat path executes — never 0 in practice, because `ai_spawn` fires before any player can attack.

**Goscape current behavior:**

Goscape's `varn.dat` loader does not exist; `PushVarn` at `pkg/script/handlers_vars.go:52-58` reads `s.ActiveNpc.NpcVarN(id)` which returns `n.varns[id]` (or `0` for never-written ids). If the `ai_spawn` trigger is NOT yet ported to goscape, `%npc_combat_xp_multiplier` will read as `0` for all NPCs.

**Decision:**

Option (a) — port a varn defaults loader — is not required, because the varn has no `.varn`-side default value (the file contains only the name). The effective runtime value of `1000` comes from `npc_param(combat_xp_multiplier)` baked by the `ai_spawn` trigger.

Option (b) — handle the default in the runtime read path — is incorrect because the fix belongs at the trigger layer, not the VarN reader.

**Stage 2 recommendation: (c) — BUT with a tracking item.** If the `ai_spawn` trigger script is NOT ported in Stage 2 (it is a frontier proc per §2.5 routing — body is at `npc/scripts/ai_spawn.rs2` outside the inner ring), then:
- `%npc_combat_xp_multiplier` will read as `0` for all NPCs.
- `give_combat_experience` receives `$multiplier = 0` and `scale(0, 1000, base_xp) = 0` — all combat XP will be **zero** for unported NPCs.
- This is a **known V-PARTIAL divergence** that makes all combat XP zero until `ai_spawn` is ported.

**Stage 2 action:** Mark as V-PARTIAL divergence in the Stage 2 plan. To unblock the combat path without a full varn loader, Stage 2 can either:
1. Port the `ai_spawn` script body directly in Stage 2 (it is 2 lines: `%npc_combat_xp_multiplier = npc_param(combat_xp_multiplier)` and `%npc_start_coord = npc_coord`) — this requires `OpNpcParam` (already wired at `handlers.go:265`) and the varn write (already wired at `handlers_vars.go:60-69`).
2. Accept zero XP for all NPCs during Stage 2 testing (if Stage 2 only tests hit-roll and animation, not XP accumulation).

**Pre-flight check for Stage 2:** Verify `ai_spawn` is listed as a trigger in goscape's script provider. If it is not registered, porting the trigger body has no effect (the script will never run). This is a separate dependency from the handler ports in entries 1-11.

---

## Stage 2 bundle ordering

### Inter-handler dependencies

- `npc_statadd` and `npc_statsub` share a dependency: both need `SetNpcStat(stat, level int)` on `ActiveNpc` and `checkNpcStatID`. These two handlers must be ported together in the same bundle.
- `p_opnpct` depends on a new `SetInteractionScriptNpcT` method on `ActivePlayer`. `p_opplayer` depends on a new `SetInteractionScriptPlayer` method on `ActivePlayer`. Neither depends on the other, but both extend the `ActivePlayer` interface — they can be bundled together.
- `npc_heropoints` depends on a new `AddHeroPoints` method on `ActiveNpc` plus a `HeroPoints` implementation in `modules/world/`. No other handler shares this dependency.
- `busy2` depends on `HasInteraction()` and `HasWaypoints()` on `ActivePlayer`. No other handler in this set shares these dependencies.
- `inv_dropitem_delayed` depends on `AddObjDelayed` on `WorldLike` — the largest new infrastructure item. No other handler in this set shares this.
- `map_multiway` depends on `IsMulti` on `WorldLike` — small addition.
- `npc_finduid` depends on `FindNpcByUID` on `NpcLookup` — small addition.
- `npc_range` depends on Chebyshev distance calculation using existing `NpcX()`, `NpcZ()` — no new interface methods needed (may need `NpcSize()` if multi-tile NPC range is in scope).
- `spotanim_npc` depends on `PlaySpotAnim` on `ActiveNpc`.

**No circular dependencies between handlers.** All 11 are independently portable at the logic level, but several share new interface-method dependencies.

### Recommended Stage 2 bundle merge/split

The 5-bundle hypothesis from Bundle 0 §7 (2A–2E) is structurally sound but can be optimized:

| Bundle | Handlers | Rationale |
|---|---|---|
| 2A | `map_multiway`, `npc_finduid`, `npc_range` | Server/NPC pure-read ops; minimal new interface surface (`IsMulti`, `FindNpcByUID`); supports `player_combat.rs2` |
| 2B | `busy2`, `p_opnpct`, `p_opplayer` | Player-interaction ops; two new `ActivePlayer` interface methods each; supports `auto_retaliate.rs2` + `player_magic.rs2` |
| 2C | `npc_statadd`, `npc_statsub`, `spotanim_npc` | NPC-write ops; share `SetNpcStat` dependency; `spotanim_npc` adds `PlaySpotAnim`; supports `player_magic.rs2` + `player_ranged.rs2` |
| 2D | `npc_heropoints` | Isolated: needs full `HeroPoints` structure; can land after 2C |
| 2E | `inv_dropitem_delayed` | Largest infrastructure: needs `ObjDelayedQueue` world machinery; can be ported last or deferred to NAI-121 if `ai_spawn` trigger is also needed |

This differs from the original 2A-2E split in that `player_melee.rs2` (Bundle 2B original) has no (D) opcodes of its own — it calls `npc_heropoints` and `npc_statadd`/`npc_statsub` which land in 2C-2D above. Bundle 2E (`inv_dropitem_delayed`) should be treated as a stretch goal for Stage 2 vs. a hard dependency.

### Pre-flight items for Stage 2 plan-author

1. **Before `npc_finduid`:** Grep `pkg/script/state.go` for the `NpcLookup` interface and verify whether `FindNpcByUID(uid int) ActiveNpc` already exists. If it exists (perhaps added after Bundle 0 pre-flight), skip the dependency. If not, add it before writing the handler.

2. **Before `busy2`:** Grep `pkg/script/active.go` for `HasInteraction` and `HasWaypoints`. Both must be absent (confirmed at Bundle 1 audit time). Also verify `(*Player).hasWaypoints()` (lowercase, package-private) exists at `modules/world/interaction.go:297` — the exported wrapper goes on `*Player` in `player_script.go`.

3. **Before `npc_statadd`/`npc_statsub`:** Grep `pkg/script/handlers_npc.go` for any partial `handleNpcStatAdd` or `handleNpcStatSub` implementation that may have been added since `50e3a7d`. Merge rather than duplicate if found.

4. **Before `spotanim_npc`:** Grep `pkg/script/active.go` `ActiveNpc` interface for `PlaySpotAnim` — confirmed absent at Bundle 1 audit time. Reuse `checkSpotAnimType` already at `pkg/script/handlers_map.go:212`.

5. **Before `npc_heropoints`:** Confirm whether a `HeroPoints` struct exists anywhere in `modules/world/`. Search for `heroPoints` or `HeroPoints`. If absent, a minimal stub (add-hero + get-top-contributor for loot routing) must be designed before writing the handler.

6. **Before `p_opnpct`:** Confirm `targetOpNpcT = 8` at `modules/world/interaction.go:35` is the correct sentinel for NpcT interactions, and verify that `(*Player).SetInteraction` with this sentinel + com=spellId routes correctly through `interaction_trigger.go:222-232`.

7. **Before `inv_dropitem_delayed`:** Confirm whether the `ObjDelayedQueue` machinery in `modules/world/` exists (it did not exist at Bundle 1 audit time). If `WorldLike.AddObj` at `pkg/script/state.go:95` can accept a `delay` parameter, consider extending its signature rather than adding a separate `AddObjDelayed`. Compare with `handleInvDropSlot` for the non-delayed precedent.

8. **For `%npc_combat_xp_multiplier`:** Confirm whether the `ai_spawn` trigger (`[ai_spawn,_]`) is registered in goscape's script provider. If it is registered but has no handler body (because `ai_spawn.rs2` hasn't been ported), the varn will remain 0. The 2-line `ai_spawn` body is trivially portable (uses only `npc_param` + varn write, both wired) and should be included in Stage 2 if the XP path is being tested.


---

## Controller spot-check addendum (Task 8)

Per `audit_subagent_fabrication`, the controller independently re-verified a sample of audit verdicts against TS source at HEAD before trusting the audit for Stage 2 dispatch.

**Entries spot-checked (4):**

1. **Entry 3 — `map_multiway` (simplest D)**
   - TS impl location `ServerOps.ts:376-380` — verified at HEAD via `rg -n 'MAP_MULTIWAY' Engine-TS/.../ServerOps.ts`. Audit's verbatim TS body matches actual file content byte-for-byte.
   - `pkg/gamemap/multimap.go:17` `IsMulti(x, z, level int) bool` exists at HEAD (verified). Note: arg order is `(x, z, level)` but audit's proposed `WorldLike.IsMulti(level, x, z int) bool` swaps to `(level, x, z)` — Stage 2 plan-author should pick one and document the bridge.

2. **Entry 5 — `npc_heropoints` (non-trivial side effects + dual guard)**
   - TS impl location `NpcOps.ts:478-480` — verified verbatim. Dual guard `[ScriptPointer.ActivePlayer, ...ActiveNpc]` matches actual TS source.

3. **Entry 2 — `inv_dropitem_delayed` (largest dependency footprint)**
   - TS impl location `InvOps.ts:188-209` — verified verbatim against actual file content.
   - `objDelayedQueue` / `ObjDelayedRequest` infrastructure absence in goscape — verified via `rg -n 'objDelayedQueue|ObjDelayed' pkg/ modules/`; only an unrelated test file matches. Audit's "no goscape counterpart exists yet" claim is correct.

4. **Frontier sanity (1) — `~chronozon_spell` and `~combat_maxhit`**
   - Bodies live at `quests/quest_crest/scripts/crest_chronozon.rs2` and `skill_combat/scripts/combat.rs2` respectively — confirmed via direct grep. Bundle 0's frontier classification stands.

5. **Entry 12 — `%npc_combat_xp_multiplier` V-PARTIAL binding**
   - `ai_spawn.rs2` body content (2 lines) — verified verbatim.
   - `ai_spawn.varn` declaration (no default) — verified verbatim.
   - `combat.param` `combat_xp_multiplier` declaration at line 157 — verified verbatim.
   - The decision "(c) — port `ai_spawn` body in Stage 2 stretch goal" is well-grounded.

**Verdict:** Clean. No fabricated citations, no fabricated dependency claims, no Bundle 0 (D) misclassifications surfaced. Audit binds for Stage 2 dispatch.

**One minor cleanup applied at controller-spot-check time:** TOC anchor for §3 had a typo (`opmapultiway` → `opmapmultiway`), fixed in this same commit.

