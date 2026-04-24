# NAI-17 — NPC Stats Array + `NPC_CHANGETYPE_KEEPALL`

Port TS's full 6-stat `levels` / `baseLevels` arrays onto `*Npc`, migrate the
S6d-era `curHP` / `baseHP` scalars into those arrays, and wire the previously
deferred `NPC_CHANGETYPE_KEEPALL` opcode (2506) through a new
`ChangeTypeKeepAll` interface method. Closes the single "From NAI-16"
deferral in `nai_followups.md` ("Deferred: NPC stats-array + KEEPALL
variant").

**Scope items:**

1. **Item A — `levels` / `baseLevels` arrays** on `*Npc`. Seeded from
   `typ.Stats` at `NewNpc` time (mirrors TS `Npc.ts:90-94` constructor
   loop). New `objtype.NpcStatCount = 6` named constant replaces bare
   `[6]int` array sizes at call sites for self-documentation.

2. **Item B — Full HP-slot migration.** The `curHP` / `baseHP` struct
   fields are deleted. All 7 Go sites that read/write them migrate to
   `levels[objtype.NpcStatHitpoints]` / `baseLevels[...]`. Public
   `CurHP()` / `BaseHP()` accessors survive as one-liners through the
   array (external rsbuf encode paths unaffected). `initialHP` helper
   is deleted — after migration there is no single-slot seeder.

3. **Item C — `ChangeType` refactor + `ChangeTypeKeepAll`.** Private
   `(*Npc).changeTypeImpl(newType, duration int, reset bool)` holds
   the full TS `Npc.ts:427-449` body including the stats-reset branch
   at TS:436-443 (boost/drain-preserving formula). Two public entry
   points on the `ActiveNpc` interface: existing `ChangeType` (reset=
   true) and new `ChangeTypeKeepAll` (reset=false). New `resetOnRevert
   bool` field on `*Npc` captures the `reset` flag for the revert-time
   consumer (Item D).

4. **Item D — `revertType` honors `resetOnRevert`.** When
   `!resetOnRevert`, the revert takes the light path (typeId + uid +
   CHANGE_TYPE mask only, no stats/queue/waypoints/hunt reset),
   mirroring TS `Npc.ts:1086-1090`. When `resetOnRevert`, current
   heavy-path body runs (tracked as NAI-17-D1: Go inline reset vs
   TS despawn+respawn structural divergence, pre-existing since S6d
   but now explicitly named). The heavy-path reseed expands from
   single-HP-slot to the full 6-slot loop matching TS `resetEntity:287-290`.

5. **Item E — Regen loop 6-stat expansion.** `npc_script.go:237-240`
   regen body changes from a two-case HP-only comparison to a
   6-iteration loop matching TS `Npc.ts:515-523`. Behaviorally a no-op
   at HEAD (no producer writes non-HP `levels[]`), but forecloses the
   silent divergence the moment a stat-boost/drain opcode lands.

6. **Item F — Opcode 2506 dispatch.** New `handleNpcChangeTypeKeepAll`
   registered in `handlers.go` against existing `OpNpcChangeTypeKeepAll`
   constant at `pkg/script/opcode.go:243`. Removes the two existing
   `DEFERRED: NPC_CHANGETYPE_KEEPALL` breadcrumbs at
   `handlers_npc.go:176-178` and `active.go:340-343`.

**Roadmap:** NAI-17 is a medium sub-spec bundling one data-model
migration, one TS-body port, and two small mechanical ripples (regen
loop, revert branching). Fidelity risk: **Medium-low**. No new
subsystems, no new packages. One new interface method
(`ChangeTypeKeepAll`) with fully enumerated call sites; one struct-field
rename with 7 enumerated migration sites.

**Tech Stack:** Go 1.26+. No new packages. Existing `pkg/script`
(`ActiveNpc` interface, `handlers_npc.go`, `handlers.go`, `opcode.go`),
`pkg/objtype` (`npctype.go` for `NpcStatCount` constant),
`modules/world` (`Npc`, `npc.go`, `npc_masks.go`, `npc_script.go`,
`npc_source.go`).

## Goal

After NAI-17 ships:

1. **`*Npc`** carries `levels [objtype.NpcStatCount]int`,
   `baseLevels [objtype.NpcStatCount]int`, and `resetOnRevert bool`
   fields. The `curHP` and `baseHP` fields are deleted.
2. **`NewNpc`** seeds both arrays from `typ.Stats` via an in-constructor
   loop (TS `Npc.ts:90-94`). `resetOnRevert` defaults to `true`.
3. **`*Npc.NpcStat(stat)`** and **`*Npc.NpcBaseStat(stat)`** return the
   correct value for all 6 stat ids (0..5), with defensive bounds
   checks outside that range. The S6a-era "TODO: full NPC stats" comment
   is removed.
4. **`*Npc.CurHP()`** and **`*Npc.BaseHP()`** public accessors survive
   the migration; bodies become one-liners reading `levels` /
   `baseLevels` at the `NpcStatHitpoints` slot.
5. **`ActiveNpc` interface** gains `ChangeTypeKeepAll(newType, duration
   int)`. Existing `ChangeType(newType, duration int)` signature
   unchanged; behavior unchanged externally (still reset=true semantics)
   but internally delegates to a private `changeTypeImpl`.
6. **`*Npc.changeTypeImpl`** ports the full TS `Npc.ts:427-449` body:
   `duration < 1 || n.dead` guard, typeId/uid/mask writes,
   `resetOnRevert = reset` write, conditional stats-reset loop (uses
   `max(newBase - (baseLevels[i] - levels[i]), 0)` boost-preservation
   formula), and the TS:444-445 fast-path (`newType == baseType &&
   lifecycle == RESPAWN → lifecycleTick = -1`).
7. **`*Npc.revertType`** branches on `resetOnRevert`: light path
   matches TS:1086-1090 exactly (typeId + uid + CHANGE_TYPE mask only);
   heavy path preserves current NAI-16 body but reseeds all 6 stats
   via a loop (not just HP). Tail re-arms `resetOnRevert = true` so a
   subsequent CHANGETYPE after a KEEPALL→revert cycle behaves as
   default.
8. **`processNpcRegen`** (npc_script.go) iterates all 6 stats,
   converging `levels[i]` toward `baseLevels[i]` per TS:515-523.
9. **`handleNpcChangeTypeKeepAll`** ships in `handlers_npc.go` and
   registers in the dispatch table at `handlers.go`. Mirrors TS
   `NpcOps.ts:465-471`.
10. **`nai_followups.md`** "From NAI-16: Deferred: NPC stats-array +
    KEEPALL variant" entry marked Resolved with close-commit ref.

## Architecture

### §1. File-by-file delta

| File | Change | Item |
|------|--------|------|
| `pkg/objtype/npctype.go` | + `NpcStatCount = 6` named constant next to `NpcStatAttack..NpcStatMagic` | A |
| `modules/world/npc.go` | Replace `curHP, baseHP int` struct fields (line 101) with `levels, baseLevels [objtype.NpcStatCount]int`; add `resetOnRevert bool`. `NewNpc` (line 108): remove `curHP:initialHP(typ), baseHP:initialHP(typ)` literal entries; add post-literal stats-seeding loop and `n.resetOnRevert = true`. Delete `initialHP` helper (lines 166-177) | A, B |
| `modules/world/npc.go` | `revertType` (lines 261-285): full rewrite with `!resetOnRevert` light-path early-return and heavy-path 6-stat reseed loop. Update doc comment. Add `n.resetOnRevert = true` re-arm tail | D |
| `modules/world/npc_masks.go` | Replace `*Npc.ChangeType` body (lines 45-58) with one-line delegate to `changeTypeImpl(newType, duration, true)`. Add `*Npc.ChangeTypeKeepAll` delegating `changeTypeImpl(newType, duration, false)`. Add private `changeTypeImpl`, `lookupType`, `resetStatsForType` helpers. Rewrite `Damage` (lines 92-103) to hit `n.levels[NpcStatHitpoints]`. Rewrite `ResetHP` (lines 140-141) to assign array slot. Update `ResetMasks` doc comment (line 106: replace `curHP, baseHP` list entries with array-slot language) | C |
| `modules/world/npc_source.go` | `CurHP()` body (line 20): `return n.levels[objtype.NpcStatHitpoints]`. `BaseHP()` body (line 21): `return n.baseLevels[objtype.NpcStatHitpoints]` | B |
| `modules/world/npc_script.go` | `NpcStat` / `NpcBaseStat` (lines 35-51): bounds-checked array reads for all 6 stats; remove "S6a: only HP (id 0) is real" comments. `processNpcRegen` HP cases (lines 237-240): replace with `for i := 0; i < objtype.NpcStatCount; i++` convergence loop | A, E |
| `pkg/script/active.go` | + `ChangeTypeKeepAll(newType, duration int)` method on `ActiveNpc` interface (below existing `ChangeType`). Remove `DEFERRED: NPC_CHANGETYPE_KEEPALL` block at lines 340-343. Update `ChangeType` doc comment to drop "DEFERRED stats-reset" language (stats-reset now in scope) | C, F |
| `pkg/script/handlers_npc.go` | Remove `DEFERRED: NPC_CHANGETYPE_KEEPALL` block at lines 176-178. + `handleNpcChangeTypeKeepAll` function mirroring `handleNpcChangeType`'s shape | F |
| `pkg/script/handlers.go` | + `OpNpcChangeTypeKeepAll: handleNpcChangeTypeKeepAll` entry in the dispatch table (alongside `OpNpcChangeType`) | F |
| `pkg/script/handlers_npc_test.go` | `mockNpc.ChangeType` signature unchanged; add `ChangeTypeKeepAll(newType, duration int)` stub with `changeTypeKeepAllCalls` recorder slice. + `TestHandleNpcChangeTypeKeepAllDispatch` and `TestHandleNpcChangeTypeKeepAllNoActiveNpc`. Extend "no active NPC" test table at lines 352-353 to include KEEPALL | F |
| `pkg/script/handlers_player_test.go` | `mockActiveNpc.ChangeTypeKeepAll(newType, duration int)` stub | F |
| `modules/world/npc_test.go` | + `TestNewNpcSeedsStatsFromType`, `TestNpcStatAllSlots`, `TestNpcBaseStatAllSlots`, `TestChangeTypeResetsStatsWithBoostPreservation`, `TestChangeTypeResetsStatsClampedAtZero`, `TestChangeTypeKeepAllPreservesStats`, `TestRevertTypeHonorsResetOnRevertFalse`, `TestRevertTypeReArmsResetOnRevert`. Existing changetype tests (lines 43-136) retain signatures; only touch-point is asserting through `CurHP()` where applicable | A, C, D |
| `modules/world/npc_event_queue_test.go` | Rewrite 4 `n.curHP, n.baseHP = X, Y` assignments (lines 174, 193, 232, 250) as `n.levels[objtype.NpcStatHitpoints], n.baseLevels[objtype.NpcStatHitpoints] = X, Y`. All reads (`n.curHP`) rewrite as `n.CurHP()` or direct array access | B |
| `modules/world/npc_script_test.go` | + `TestNpcRegenIteratesAllSixStats` (pair of cases: drain-converges-up and boost-converges-down on a non-HP slot) | E |
| `~/.claude/projects/.../memory/nai_followups.md` | "From NAI-16: Deferred: NPC stats-array + KEEPALL variant" entry gains a Resolved preamble with close-commit ref | All |

No new files. No new packages. One new constant (`NpcStatCount`), one
new interface method (`ChangeTypeKeepAll`), one new struct field
(`resetOnRevert`), three new `*Npc` methods (`ChangeTypeKeepAll` public,
`changeTypeImpl` + `lookupType` + `resetStatsForType` private), two
deleted fields (`curHP`, `baseHP`), one deleted helper (`initialHP`).

### §2. Data model (Item A + B)

**`NpcStatCount` constant** at `pkg/objtype/npctype.go` (insert after
line 21):

```go
const (
    NpcStatAttack    = 0
    NpcStatDefence   = 1
    NpcStatStrength  = 2
    NpcStatHitpoints = 3
    NpcStatRanged    = 4
    NpcStatMagic     = 5
    NpcStatCount     = 6 // Total number of stat slots; matches TS NpcStat enum.
)
```

**Struct fields** at `modules/world/npc.go:101-105`:

```go
// Replace:
//   curHP, baseHP                             int
// With:
levels        [objtype.NpcStatCount]int // current (boosted) stat values
baseLevels    [objtype.NpcStatCount]int // base values (regen convergence target)
resetOnRevert bool                       // TS Npc.ts:72; CHANGETYPE→true, KEEPALL→false
```

**`NewNpc` seeding** at `modules/world/npc.go:108-163`:

```go
// In the struct literal, REMOVE:
//   curHP:           initialHP(typ),
//   baseHP:          initialHP(typ),

// AFTER the struct literal (and after n.targetOp assignment, line 161),
// ADD (mirrors TS Npc.ts:90-94):
for i := 0; i < objtype.NpcStatCount && i < len(typ.Stats); i++ {
    v := int(typ.Stats[i])
    n.levels[i] = v
    n.baseLevels[i] = v
}
n.resetOnRevert = true
```

The `i < len(typ.Stats)` guard is Go-side paranoia against a malformed
.dat; TS trusts the .dat to have populated all 6 slots. If `typ` is
nil, the loop runs zero iterations and both arrays stay zero-valued —
same semantics as the old `initialHP(nil) == 0`.

**`initialHP` helper** at `modules/world/npc.go:165-177`: **deleted**.
Post-migration there is no single-slot seeder — both `NewNpc` (above)
and `revertType` (§4) use the full 6-slot loop.

**Getters** at `modules/world/npc_script.go:37-51`:

```go
// NpcStat returns the current (boosted) stat level for the given stat id.
// Reads n.levels[stat] — seeded from typ.Stats at NewNpc time and maintained
// by ChangeType / Damage / processNpcRegen.
func (n *Npc) NpcStat(stat int) int {
    if stat < 0 || stat >= objtype.NpcStatCount {
        return 0
    }
    return n.levels[stat]
}

// NpcBaseStat returns the base stat level for the given stat id.
func (n *Npc) NpcBaseStat(stat int) int {
    if stat < 0 || stat >= objtype.NpcStatCount {
        return 0
    }
    return n.baseLevels[stat]
}
```

**Public HP accessors** at `modules/world/npc_source.go:20-21`:

```go
func (n *Npc) CurHP() int  { return n.levels[objtype.NpcStatHitpoints] }
func (n *Npc) BaseHP() int { return n.baseLevels[objtype.NpcStatHitpoints] }
```

**`Damage`** at `modules/world/npc_masks.go:92-103`:

```go
func (n *Npc) Damage(amount, dmgType int) {
    if amount < 0 {
        amount = 0
    }
    cur := n.levels[objtype.NpcStatHitpoints]
    n.damageAmt = min(amount, cur)
    n.damageType = dmgType
    cur -= amount
    if cur < 0 {
        cur = 0
    }
    n.levels[objtype.NpcStatHitpoints] = cur
    n.masks |= rsbuf.NpcMaskDamage
}
```

Pull `cur` into a local so the two-sided clamp reads linearly; behavior
identical to current impl.

**`ResetHP`** at `modules/world/npc_masks.go:140-141`:

```go
func (n *Npc) ResetHP() {
    hp := int(n.typ.Stats[objtype.NpcStatHitpoints])
    n.levels[objtype.NpcStatHitpoints] = hp
    n.baseLevels[objtype.NpcStatHitpoints] = hp
}
```

**Call-site enumeration (per `enumerate_all_sites.md`)** — every
`n.curHP` / `n.baseHP` site that needs migration:

| # | Site | Kind | New form |
|---|------|------|----------|
| 1 | `modules/world/npc.go:101` | struct field decl | Delete; replaced by arrays |
| 2 | `modules/world/npc.go:151-152` | NewNpc literal init | Delete; replaced by post-literal loop |
| 3 | `modules/world/npc.go:271-272` | revertType reseed | Full 6-slot loop (§4) |
| 4 | `modules/world/npc_source.go:20-21` | public getters | `n.levels[NpcStatHitpoints]` / `n.baseLevels[...]` |
| 5 | `modules/world/npc_script.go:38-48` | NpcStat/NpcBaseStat bodies | Array-slot reads for all 6 |
| 6 | `modules/world/npc_script.go:237-240` | regen cases | 6-iteration loop (§5) |
| 7 | `modules/world/npc_masks.go:96-100` | Damage arithmetic | Array-slot read/write |
| 8 | `modules/world/npc_masks.go:140-141` | ResetHP | Array-slot writes |
| 9 | `modules/world/npc_event_queue_test.go:174, 193, 232, 250` | test assignments | Array-slot assignments |
| 10 | `modules/world/npc_event_queue_test.go:181-257` (scattered) | test reads | `n.CurHP()` or array read |

### §3. `ChangeType` refactor + `ChangeTypeKeepAll` (Item C)

**TS source** (verified, `Engine-TS/.../Npc.ts:427-449`):

```ts
changeType(type: number, duration: number, reset: boolean = true) {
    if (!this.isActive || duration < 1) {
        return;
    }
    this.type = type;
    this.masks |= NpcInfoProt.CHANGE_TYPE;
    this.uid = (type << 16) | this.nid;
    this.resetOnRevert = reset;

    if (reset) {
        const npcType = NpcType.get(type);
        for (let index = 0; index < npcType.stats.length; index++) {
            const level = npcType.stats[index];
            this.levels[index] = Math.max(level - (this.baseLevels[index] - this.levels[index]), 0);
            this.baseLevels[index] = level;
        }
    }
    if (type === this.baseType && this.lifecycle === EntityLifeCycle.RESPAWN) {
        this.setLifeCycle(-1);
    } else {
        this.setLifeCycle(duration);
    }
}
```

**TS handler dispatch** (verified, `Engine-TS/.../NpcOps.ts:457-471`):

```ts
[ScriptOpcode.NPC_CHANGETYPE]: checkedHandler(ActiveNpc, state => {
    const [id, duration] = state.popInts(2);
    const npcType: number = check(id, NpcTypeValid).id;
    check(duration, DurationValid);
    state.activeNpc.changeType(npcType, duration);
}),

[ScriptOpcode.NPC_CHANGETYPE_KEEPALL]: checkedHandler(ActiveNpc, state => {
    const [id, duration] = state.popInts(2);
    const npcType: number = check(id, NpcTypeValid).id;
    check(duration, DurationValid);
    state.activeNpc.changeType(npcType, duration, false);
}),
```

**`ActiveNpc` interface** at `pkg/script/active.go` (replace the
existing `ChangeType` comment block at lines 335-344):

```go
// ChangeType morphs the NPC to newType and schedules a revert to
// baseType after `duration` ticks. Resets all 6 stats onto the new
// type's base values using a boost/drain-preserving formula. Mirrors
// TS Npc.changeType at Engine-TS/.../Npc.ts:427-449 with reset=true.
// No-op when duration < 1 OR when the NPC is dead.
ChangeType(newType, duration int)

// ChangeTypeKeepAll morphs the NPC to newType and schedules a revert
// after `duration` ticks, preserving all current stat values (no
// reset). The revert, when it fires, takes the light path
// (resetOnRevert=false → typeId + uid + CHANGE_TYPE mask only).
// Mirrors TS Npc.changeType at Engine-TS/.../Npc.ts:427-449 with
// reset=false, dispatched from NPC_CHANGETYPE_KEEPALL (opcode 2506,
// TS NpcOps.ts:465-471). No-op when duration < 1 OR when the NPC is dead.
ChangeTypeKeepAll(newType, duration int)
```

**`*Npc` public methods** at `modules/world/npc_masks.go` (replace
the existing `ChangeType` body + DEFERRED comment block at lines 16-58):

```go
// ChangeType morphs the NPC to newType and schedules a revert to
// baseType after `duration` ticks. Resets stats onto the new type's
// base values. Mirrors TS Npc.changeType at Engine-TS/.../Npc.ts:427-449
// with reset=true. See changeTypeImpl for the full body.
func (n *Npc) ChangeType(newType, duration int) {
    n.changeTypeImpl(newType, duration, true)
}

// ChangeTypeKeepAll morphs the NPC to newType and schedules a revert
// after `duration` ticks without resetting stats. Dispatched from
// NPC_CHANGETYPE_KEEPALL (opcode 2506). Mirrors TS Npc.changeType at
// Engine-TS/.../Npc.ts:427-449 with reset=false.
func (n *Npc) ChangeTypeKeepAll(newType, duration int) {
    n.changeTypeImpl(newType, duration, false)
}
```

**Private core** (new function in `npc_masks.go`):

```go
// changeTypeImpl is the shared body behind ChangeType and ChangeTypeKeepAll.
// Mirrors TS Npc.changeType at Engine-TS/.../Npc.ts:427-449.
//
// Semantics:
//   - No-op when duration < 1 (TS guard; rejects 0 and negatives) OR
//     when the NPC is dead (TS `!this.isActive`).
//   - On success: writes typeId, changeTypeID, CHANGE_TYPE mask,
//     recomputes uid, writes resetOnRevert=reset.
//   - If reset: looks up the NEW type's NpcType config, then runs the
//     TS:436-443 stats-reset loop: levels[i] = max(newBase -
//     (baseLevels[i] - levels[i]), 0); baseLevels[i] = newBase.
//   - Fast-path TS:444-445: if newType==baseType && lifecycle==RESPAWN,
//     lifecycleTick=-1 (suppresses Events-block revert). Otherwise
//     lifecycleTick=duration.
func (n *Npc) changeTypeImpl(newType, duration int, reset bool) {
    if duration < 1 || n.dead {
        return
    }
    n.typeId = newType
    n.changeTypeID = newType
    n.masks |= rsbuf.NpcMaskChangeType
    n.uid = (newType << 16) | n.nid
    n.resetOnRevert = reset

    if reset {
        if newTyp := n.lookupType(newType); newTyp != nil {
            n.resetStatsForType(newTyp)
        }
    }

    if newType == n.baseType && n.lifecycle == NpcLifecycleRespawn {
        n.lifecycleTick = -1
    } else {
        n.lifecycleTick = duration
    }
}

// lookupType returns the NpcType config for typeId, or nil if server
// or registry is unavailable or typeId is out of bounds. Mirrors the
// guard shape revertType already uses (npc.go:265-268 pre-NAI-17).
func (n *Npc) lookupType(typeId int) *objtype.NpcType {
    if n.server == nil || n.server.npcTypes == nil {
        return nil
    }
    if typeId < 0 || typeId >= len(n.server.npcTypes.Configs) {
        return nil
    }
    return n.server.npcTypes.Configs[typeId]
}

// resetStatsForType applies the TS Npc.ts:436-443 boost/drain-preserving
// stats reset against newTyp's Stats. For each slot i:
//   drain := baseLevels[i] - levels[i]     // positive: drained; negative: boosted
//   levels[i] = max(newBase - drain, 0)
//   baseLevels[i] = newBase
// Iterates over min(NpcStatCount, len(newTyp.Stats)) slots.
func (n *Npc) resetStatsForType(newTyp *objtype.NpcType) {
    for i := 0; i < objtype.NpcStatCount && i < len(newTyp.Stats); i++ {
        newBase := int(newTyp.Stats[i])
        drain := n.baseLevels[i] - n.levels[i]
        v := newBase - drain
        if v < 0 {
            v = 0
        }
        n.levels[i] = v
        n.baseLevels[i] = newBase
    }
}
```

**Handler** at `pkg/script/handlers_npc.go` (insert after existing
`handleNpcChangeType`):

```go
// handleNpcChangeTypeKeepAll pops (newType, duration) in TS order
// (duration on top) and morphs the NPC preserving all current stats.
// Matches TS NpcOps.ts:465-471.
func handleNpcChangeTypeKeepAll(s *ScriptState) error {
    if err := requireActiveNpc(s, "NPC_CHANGETYPE_KEEPALL"); err != nil {
        return err
    }
    duration := s.PopInt()
    newType := s.PopInt()
    s.ActiveNpc.ChangeTypeKeepAll(newType, duration)
    return nil
}
```

**Dispatch table** at `pkg/script/handlers.go` (add alongside
`OpNpcChangeType: handleNpcChangeType`):

```go
OpNpcChangeTypeKeepAll: handleNpcChangeTypeKeepAll,
```

**Existing `DEFERRED` breadcrumbs** get struck:
- `handlers_npc.go:176-178` comment block — delete.
- `active.go:340-343` comment block — delete. Keep the `ChangeType`
  doc comment but drop the "DEFERRED stats-reset" language (in scope
  now).

### §4. `revertType` `resetOnRevert` branching (Item D)

**TS source** (verified, `Engine-TS/.../Npc.ts:1082-1091`):

```ts
private revertType(): void {
    if (this.resetOnRevert) {
        World.removeNpc(this, -1);
        World.addNpc(this, -1, false);
    } else {
        this.type = this.baseType;
        this.masks |= NpcInfoProt.CHANGE_TYPE;
        this.uid = (this.type << 16) | this.nid;
    }
}
```

**Go port** (replace entire `revertType` body at `modules/world/npc.go:261-285`):

```go
// revertType restores the NPC to its baseline type. Called from the
// Events block (npc_ai.go:37-40) when lifecycleTick hits 0 on
// RESPAWN+alive, and from the respawn path on revival.
//
// Branches on resetOnRevert (written by changeTypeImpl):
//   - resetOnRevert=false (KEEPALL path): TS Npc.ts:1086-1090 light path.
//     Restore typeId/uid + raise CHANGE_TYPE mask. No stats reset, no
//     queue clear, no waypoint clear, no hunt-field reset. Intended
//     for short-lived morphs that must preserve combat state.
//   - resetOnRevert=true (default, CHANGETYPE path): current heavy-path
//     behavior — inline reset of type/uid/typ, full 6-slot stats reseed,
//     queue/waypoint clear, tele flag, hunt-field reset.
//
// NAI-17-D1 (tracked deviation): TS's heavy path is World.removeNpc +
// World.addNpc — a despawn+respawn that re-runs the constructor. Go
// does an INLINE reset instead, pre-existing since S6d. See §8.
//
// Tail re-arm: sets resetOnRevert = true so a subsequent CHANGETYPE
// on the same NPC starts from the default. TS gets this for free via
// the ctor rerun; Go must re-arm explicitly.
func (n *Npc) revertType() {
    if !n.resetOnRevert {
        // Light path — TS:1086-1090.
        if n.typeId != n.baseType {
            n.typeId = n.baseType
            n.uid = (n.typeId << 16) | n.nid
        }
        n.masks |= rsbuf.NpcMaskChangeType
        n.resetOnRevert = true
        return
    }

    // Heavy path — inline reset matching TS resetEntity:280-317 semantics
    // (minus the World.removeNpc/addNpc structural call; see NAI-17-D1).
    if n.typeId != n.baseType {
        n.typeId = n.baseType
        n.uid = (n.typeId << 16) | n.nid
        if newTyp := n.lookupType(n.baseType); newTyp != nil {
            n.typ = newTyp
        }
    }
    // Full 6-slot stats reseed (TS resetEntity:287-290).
    if n.typ != nil {
        for i := 0; i < objtype.NpcStatCount && i < len(n.typ.Stats); i++ {
            v := int(n.typ.Stats[i])
            n.levels[i] = v
            n.baseLevels[i] = v
        }
    }
    n.queue = nil
    n.waypointIndex = -1
    n.tele = true
    n.masks |= rsbuf.NpcMaskChangeType
    n.huntClock = 0
    n.huntTarget = nil
    if n.typ != nil {
        n.huntRange = int(n.typ.HuntRange)
        n.huntMode = n.typ.HuntMode
    }
    n.resetOnRevert = true // re-arm default for next morph cycle
}
```

### §5. Regen loop 6-stat expansion (Item E)

**TS source** (verified, `Engine-TS/.../Npc.ts:515-523`):

```ts
for (let index = 0; index < this.baseLevels.length; index++) {
    const stat = this.levels[index];
    const baseStat = this.baseLevels[index];
    if (stat < baseStat) {
        this.levels[index]++;
    } else if (stat > baseStat) {
        this.levels[index]--;
    }
}
```

**Go port** at `modules/world/npc_script.go:237-240` (replace the
existing two-case HP block):

```go
// Replace:
//   case n.curHP < n.baseHP:
//       n.curHP++
//   case n.curHP > n.baseHP:
//       n.curHP--
// With (mirrors TS Npc.ts:515-523):
for i := 0; i < objtype.NpcStatCount; i++ {
    switch {
    case n.levels[i] < n.baseLevels[i]:
        n.levels[i]++
    case n.levels[i] > n.baseLevels[i]:
        n.levels[i]--
    }
}
```

Surrounding regen-gate logic (interval check, clock reset) preserved
unchanged. At HEAD this is behaviorally a no-op for non-HP slots (no
producer writes `levels[0..2,4,5]`), but forecloses the silent
divergence the moment a stat-boost or stat-drain opcode lands.

## Test strategy

All tests use the existing test-server and NPC-fixture scaffolding in
`modules/world/*_test.go` + `pkg/script/handlers_*_test.go`. No new test
fixture infrastructure needed.

| Test | File | Asserts |
|------|------|---------|
| `TestNewNpcSeedsStatsFromType` | `npc_test.go` | Seed a `typ.Stats` with 6 distinct values ({1,2,3,4,5,6}); construct NPC; assert `n.NpcStat(i) == want[i]` AND `n.NpcBaseStat(i) == want[i]` for all i in 0..5. Catches index mix-ups + confirms both arrays parallel-seed. |
| `TestNewNpcWithNilStatsStaysZero` | `npc_test.go` | Seed `typ.Stats = nil`; construct NPC; assert all 6 stats return 0 from both getters. Matches old `initialHP(nil) == 0` behavior. |
| `TestNpcStatOutOfRange` | `npc_script_test.go` | `NpcStat(-1)` and `NpcStat(6)` return 0. (Defensive bounds check; no TS parallel — NAI-17-D2.) |
| `TestNpcBaseStatOutOfRange` | `npc_script_test.go` | Same for `NpcBaseStat`. |
| `TestChangeTypeResetsStatsWithBoostPreservation` | `npc_test.go` | Seed NPC with baseLevels={10,10,10,10,10,10}, levels={8,12,10,5,10,10} (drained ATK=8/base=10, boosted DEF=12/base=10, drained HP=5/base=10). ChangeType to a type with Stats={20,15,12,20,12,12}. Assert: ATK levels=18 (20−2 drain), DEF levels=17 (15+2 boost), HP levels=15 (20−5 drain), baseLevels now match new type's values. Exercises the max-clamp via one slot where drain exceeds new base (seed STR levels=5/base=20, morph to STR base=10 → expect levels=0 from clamp). |
| `TestChangeTypeResetsStatsClampedAtZero` | `npc_test.go` | Dedicated test of the `max(..., 0)` clamp: seed a drain of 100 against a new base of 10; assert `levels[i] == 0` after ChangeType. (Could be folded into the above if preferred; separate for clarity of intent.) |
| `TestChangeTypeKeepAllPreservesStats` | `npc_test.go` | Seed baseLevels={10,10,10,10,10,10}, levels={5,5,5,5,5,5}. Call `ChangeTypeKeepAll` to a type with Stats={99,99,99,99,99,99}. Assert all `levels[i] == 5` AND `baseLevels[i] == 10` unchanged. Assert `n.resetOnRevert == false`. Asserts typeId updated, uid recomputed, mask raised. |
| `TestChangeTypeKeepAllDurationZeroNoOp` | `npc_test.go` | `ChangeTypeKeepAll(newType, 0)` total no-op. Parallels existing `TestNpcChangeTypeDurationZeroNoOp`. |
| `TestChangeTypeKeepAllDeadNoOp` | `npc_test.go` | `ChangeTypeKeepAll` on `n.dead=true` total no-op. Parallels existing dead-npc test. |
| `TestRevertTypeHonorsResetOnRevertFalse` | `npc_test.go` | Manual state setup: set `n.typeId=99`, `n.baseType=7`, `n.resetOnRevert=false`, populate `n.levels` / `n.baseLevels` / `n.queue` / `n.waypointIndex` / `n.huntClock` with non-zero markers. Call `revertType()`. Assert `n.typeId==7`, `n.uid==(7<<16)\|n.nid`, mask raised, BUT all stats/queue/waypoints/hunt fields unchanged from pre-revert markers. Also assert `n.resetOnRevert == true` (re-armed). |
| `TestRevertTypeHonorsResetOnRevertTrue` | `npc_test.go` | Manual setup: `n.typeId=99`, `n.baseType=7`, `n.resetOnRevert=true`, populate `n.levels` with non-typ values. Call `revertType()`. Assert heavy path ran: typeId back to 7, `n.levels[i]` all reseeded from `n.typ.Stats`, queue cleared, etc. Replaces / complements the existing NAI-16 morph-revert tests. |
| `TestRevertTypeReArmsResetOnRevert` | `npc_test.go` | After `n.resetOnRevert=false` + `revertType()`, assert `n.resetOnRevert == true`. Asserts the re-arm tail. |
| `TestNpcRegenIteratesAllSixStats` | `npc_script_test.go` or `npc_event_queue_test.go` | Seed `n.levels[STRENGTH]=10, n.baseLevels[STRENGTH]=11` (drain). Tick regen. Assert `n.levels[STRENGTH]==11`. Second case: seed `n.levels[MAGIC]=12, n.baseLevels[MAGIC]=11` (boost). Tick regen. Assert `n.levels[MAGIC]==11`. Confirms the loop iterates beyond HP. |
| `TestHandleNpcChangeTypeKeepAllDispatch` | `handlers_npc_test.go` | Run opcode `OpNpcChangeTypeKeepAll` with popped args `[npcType=42, duration=100]`. Assert `mockNpc.changeTypeKeepAllCalls` contains `{newType:42, duration:100}`. Confirms pop order matches TS (duration on top). |
| `TestHandleNpcChangeTypeKeepAllNoActiveNpc` | `handlers_npc_test.go` | Extend the existing no-active-npc table at lines 352-353 to include KEEPALL. Expects error `"NPC_CHANGETYPE_KEEPALL: no active npc"`. |

Existing NAI-16 tests at `npc_test.go:43-136` (CHANGETYPE happy path,
duration<1 no-op, dead no-op, morph-to-baseType fast-path) stay
unchanged — their assertions read `n.typeId`, `n.lifecycleTick`,
`n.masks`, `n.changeTypeID`; none touches stats directly. If any
asserts `n.curHP` / `n.baseHP` they migrate to `n.CurHP()` /
`n.BaseHP()` per Item B.

## Edge cases

| Scenario | Expected |
|----------|----------|
| `typ.Stats` shorter than 6 slots | Loop iterates only over populated slots; untouched slots stay at zero-value. Matches old `initialHP` behavior on malformed typ |
| `typ == nil` at NewNpc | Stats-seeding loop runs zero iterations (TS would throw; Go tolerates). `CurHP()` / `BaseHP()` return 0. |
| `ChangeType(newType, 0)` | Total no-op per `duration < 1` guard. No state writes. |
| `ChangeType(newType, -1)` | Total no-op (same guard). |
| `ChangeType` when `n.dead` | Total no-op per TS `!isActive` guard. |
| `ChangeType(baseType, duration)` on RESPAWN NPC | Fast-path fires: `lifecycleTick = -1`, stats-reset still runs (TS behavior). Event-block revert suppressed. |
| `ChangeType(baseType, duration)` on non-RESPAWN NPC | No fast-path: `lifecycleTick = duration`. |
| `ChangeTypeKeepAll(newType, duration)` then morph-back-to-base via `revertType` | Light path runs (no stats reset). Stats preserved across revert. `resetOnRevert` re-arms to true. |
| Drain larger than new base in stats-reset | `levels[i] = 0` via `max(..., 0)` clamp. TS Math.max behavior preserved. |
| Boost larger than new base in stats-reset | `levels[i] = newBase + boost`. No upper clamp in TS; no upper clamp in Go. |
| Regen on a slot where `levels[i] == baseLevels[i]` | No change (both conditions false). |
| `ChangeTypeKeepAll` called twice before revert | Second call wins: `resetOnRevert` stays `false` (second KEEPALL writes false again), new typeId/uid/mask applied, stats still preserved. |
| `ChangeType` (reset=true) after a prior `ChangeTypeKeepAll` (resetOnRevert=false, no revert yet) | Second `ChangeType` overwrites: runs stats-reset against its own new type, writes `resetOnRevert=true`. Revert will take heavy path. |
| Unknown typeId passed to `ChangeType` (lookupType returns nil) | typeId/uid/mask still update; stats-reset skipped silently. Same tolerance revertType already shows. Out-of-scope `NpcTypeValid` check would harden this (fidelity-audit deferral). |

## Gotchas / tracked deviations

### Deviation ledger (§8)

| ID | Description | Rationale | Closure |
|----|-------------|-----------|---------|
| NAI-17-D1 | `revertType` heavy path uses inline reset instead of TS's `World.removeNpc` + `World.addNpc` despawn+respawn (Npc.ts:1083-1085) | Pre-existing structural divergence since S6d — never explicitly tracked until NAI-17 makes it visible by wiring the light path. Reworking the heavy path to align with TS touches the NPC registry, observer counts, and entity-list invariants — a separate sub-spec. The KEEPALL light path matches TS exactly, which is the NAI-17 fidelity target | Future "revertType respawn alignment" sub-spec |
| NAI-17-D2 | `NpcStat` / `NpcBaseStat` have defensive `< 0 \|\| >= NpcStatCount` bounds checks that TS lacks | Go's script dispatch cannot panic a fixture-driven tick run; a bogus opcode arg would otherwise array-out-of-bounds. TS implicitly returns `undefined` which coerces; behavior for in-range ids matches TS exactly | Unlikely — Go-idiom concession |

### Gotchas

1. **`initialHP` deletion has zero consumers after migration.** Per
   `dead_api_polish.md`, helpers shipped with zero consumers get
   cleaned up at sub-spec close. `initialHP` is called only from
   `NewNpc` and `revertType`; both switch to inline loops. The
   implementer MUST delete the function and its doc comment, not
   leave it orphaned.

2. **`n.typ != nil` guard in `revertType` heavy path.** Current code
   (pre-NAI-17) dereferences `n.typ` after the `if n.server != nil`
   guard — the typ pointer is only rebuilt from `n.server.npcTypes`
   when `n.typeId != n.baseType`. If the NPC was constructed with
   `typ == nil` (test fixture) and `typeId == baseType`, the stats
   reseed must skip. Port faithfully.

3. **`resetOnRevert` re-arm in BOTH `revertType` branches.** The
   light-path early-return must also re-arm — otherwise a KEEPALL
   followed by a revert leaves `resetOnRevert=false`, which would
   corrupt a subsequent CHANGETYPE on the same NPC. The re-arm goes
   at the tail of each branch.

4. **Regen loop's `switch` on `<` / `>`.** A single `switch` with
   three cases (`<`, `>`, implicit equal) mirrors TS's `if/else if`
   with no else arm. Do not introduce a third explicit case — equal
   is a no-op.

5. **`resetStatsForType` only called when `reset=true`.** The KEEPALL
   path does NOT invoke it. If a future refactor inlines the call
   regardless of `reset`, KEEPALL would silently break. Keep the
   `if reset` gate in `changeTypeImpl` obvious.

6. **Existing NAI-16 `TestNpcChangeTypeSetsMask` and siblings.** These
   assertions read `n.typeId`, `n.lifecycleTick`, `n.masks`,
   `n.changeTypeID` — none touches stats. Signatures unchanged. Any
   HP-related assertion in `npc_test.go` migrates to `n.CurHP()` /
   `n.BaseHP()`.

7. **`changeTypeCalls` recorder pattern in tests.** The existing
   `mockNpc.changeTypeCalls []struct{ newType, duration int }` stays
   unchanged (documents only `ChangeType` calls). A new
   `changeTypeKeepAllCalls []struct{ newType, duration int }` slice
   records KEEPALL calls separately. Keeping them separate avoids
   flag-vs-call ambiguity in assertions.

## Out of scope (tracked)

1. **`NpcTypeValid` / `DurationValid` opcode-side gates.** Both
   CHANGETYPE and CHANGETYPE_KEEPALL handlers skip them; joins the
   existing NumberNotNull fidelity-audit list in `nai_followups.md`
   "From NAI-2."
2. **`revertType` heavy-path respawn alignment** — NAI-17-D1 closure.
   Distinct sub-spec reworks the inline reset into a proper despawn
   + re-add through the NPC registry.
3. **Boost/drain opcodes (`NPC_STATBOOST`, `NPC_STATHEAL`, etc.).**
   No current opcode writes to non-HP `levels[]` slots, making the
   regen loop a behavioral no-op for those slots at HEAD. A future
   combat/stats sub-spec ships the producer side.
4. **`NpcStat` / `NpcBaseStat` script-side divergence where TS would
   throw on an out-of-range id.** NAI-17-D2 retains the Go-side
   defensive bounds check.
5. **`resetEntity` structural port.** TS has a `resetEntity(respawn:
   boolean)` method distinct from `revertType`; Go collapses the two
   into `revertType`'s heavy path. Unchanged by NAI-17; unlikely to
   change.

## References

- TS source:
  - `LostCityRS/Engine-TS/src/engine/entity/Npc.ts:50-51` (levels / baseLevels Uint16Array decls)
  - `LostCityRS/Engine-TS/src/engine/entity/Npc.ts:72` (resetOnRevert default)
  - `LostCityRS/Engine-TS/src/engine/entity/Npc.ts:90-94` (ctor stats seeding loop)
  - `LostCityRS/Engine-TS/src/engine/entity/Npc.ts:287-290` (resetEntity stats reseed)
  - `LostCityRS/Engine-TS/src/engine/entity/Npc.ts:427-449` (changeType full body)
  - `LostCityRS/Engine-TS/src/engine/entity/Npc.ts:472-485` (applyDamage HP site)
  - `LostCityRS/Engine-TS/src/engine/entity/Npc.ts:515-523` (regen loop)
  - `LostCityRS/Engine-TS/src/engine/entity/Npc.ts:1082-1091` (revertType branching)
  - `LostCityRS/Engine-TS/src/engine/entity/NpcStat.ts:1-17` (NpcStat enum)
  - `LostCityRS/Engine-TS/src/engine/script/handlers/NpcOps.ts:457-471` (CHANGETYPE + CHANGETYPE_KEEPALL handlers)
- Go source at HEAD:
  - `pkg/objtype/npctype.go:11-21` (NpcStat* constants + Stats slice decl)
  - `pkg/objtype/npctype.go:165, 234-244` (Stats []uint16 type + .dat loader)
  - `pkg/script/active.go:315-344` (ActiveNpc interface — NpcStat/NpcBaseStat + ChangeType + DEFERRED block)
  - `pkg/script/handlers_npc.go:38-55` (handleNpcStat / handleNpcBaseStat)
  - `pkg/script/handlers_npc.go:173-187` (handleNpcChangeType + DEFERRED comment)
  - `pkg/script/opcode.go:243, 274, 886` (OpNpcChangeTypeKeepAll constant + opcode-name switch)
  - `modules/world/npc.go:101-105` (struct fields curHP/baseHP)
  - `modules/world/npc.go:108-163` (NewNpc)
  - `modules/world/npc.go:165-177` (initialHP helper — to delete)
  - `modules/world/npc.go:261-285` (revertType)
  - `modules/world/npc_masks.go:16-58` (ChangeType + DEFERRED comment)
  - `modules/world/npc_masks.go:92-103` (Damage)
  - `modules/world/npc_masks.go:140-141` (ResetHP)
  - `modules/world/npc_script.go:35-51` (NpcStat / NpcBaseStat)
  - `modules/world/npc_script.go:237-240` (regen HP cases)
  - `modules/world/npc_source.go:20-21` (CurHP / BaseHP accessors)
- Prior specs:
  - NAI-16 (`docs/superpowers/specs/2026-04-23-nai-16-deferral-sweep-design.md` — ChangeType duration + stats-array deferrals)
  - NAI-5 (`docs/superpowers/specs/2026-04-22-nai-5-npc-events-block-design.md` — revertType + Events block origin)
  - S6a (`docs/superpowers/specs/2026-04-21-runescript-s6a-active-npc-reads-design.md` — NpcStat/NpcBaseStat interface decl)
  - S6d (`docs/superpowers/specs/2026-04-21-runescript-s6d-persistent-npc-hp-design.md` — curHP/baseHP promotion to persistent fields)
