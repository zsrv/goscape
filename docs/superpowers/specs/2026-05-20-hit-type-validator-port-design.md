# HitType validator port + NpcStat read-path validator coverage

**Predecessors:** NAI-184 close (HEAD `7eabfb31` after `f9970fd6` test bolt-on); NAI-23 Bundle 4a (left NPC_DAMAGE `dmgType` "raw" pending the validator port — see comment at `pkg/script/handlers_npc.go:341-342`); NAI-120 Bundle 2C (introduced `checkNpcStatID` at `pkg/script/handlers_npc.go:64-72`, applied it at write-path handlers only).

**Status:** drafted 2026-05-20.

## 1. Goal

Close four script-input validation gaps that TS guards with `check(state.popInt(), XxxValid)` but goscape currently leaves raw:

1. `NPC_DAMAGE` (handlers_npc.go:351) — `dmgType` popped without `HitTypeValid` check.
2. `DAMAGE` / P_DAMAGE (handlers_player.go:1518) — `hitType` popped without `HitTypeValid` check.
3. `NPC_STAT` (handlers_npc.go:174) — `stat` popped without `NpcStatValid` check.
4. `NPC_BASESTAT` (handlers_npc.go:184) — `stat` popped without `NpcStatValid` check.

After this slice, every TS `check(_, HitTypeValid)` call site and every TS `check(_, NpcStatValid)` call site has a TS-faithful goscape counterpart, and the stale "stays raw" doc-comment cluster at handlers_npc.go:341-342 is refreshed.

## 2. Why now

NAI-184 closed the last narrow behavioral combat gap. The validator-coverage gap is the next mechanically-bounded TS port in the script-handler surface — small, fully scoped by existing patterns, no design unknowns.

The four sites are also the *only* remaining raw call sites in the "simple enum-range" validator family per the 2026-05-20 audit of `pkg/script/handlers_*.go`: every other validator that TS applies (HuntVis, NpcMode, PlayerStat, MapFindSquareType, LocAngle, LocShape, Duration, NumberNotNull, Queue, ObjStack) is already wrapped or inline-checked at every call site. Closing these four fully retires the family.

## 3. TS reference

### 3.1 Validators

`Engine-TS/src/engine/script/ScriptValidators.ts:117` (HitType):

```ts
HitTypeValid = new ScriptInputRangeValidator(0, 3);
```

`Engine-TS/src/engine/script/ScriptValidators.ts:112` (NpcStat):

```ts
NpcStatValid = new ScriptInputRangeValidator(0, NpcStat._TOTAL);
```

Both use `ScriptInputRangeValidator(lo, hi)` — accept `[lo, hi)`, throw on out-of-range. `NpcStat._TOTAL = 6` in TS (`Engine-TS/src/engine/entity/NpcStat.ts`); `HitType` has three values `[BLOCK=0, DAMAGE=1, POISON=2]` (`HitType.ts:1-5`).

### 3.2 Call sites

- `NpcOps.ts:265` — `const type = check(state.popInt(), HitTypeValid);` inside NPC_DAMAGE handler.
- `PlayerOps.ts:778` — same pattern inside the P_DAMAGE handler. The hitType is popped, validated, then passed to `player.applyDamage(amount, type)`.
- `NpcOps.ts` NPC_STAT / NPC_BASESTAT handlers — `check(state.popInt(), NpcStatValid)` before reading `npc.levels[stat]` / `npc.baseLevels[stat]`.

## 4. Goscape baseline

| Piece | Where | State |
| --- | --- | --- |
| `HitType` constants | (nowhere) | Unported. Wire values used as bare literal ints. |
| `checkHitType` | (nowhere) | Unported. |
| `checkNpcStatID` | handlers_npc.go:64-72 | Ported (NAI-120 Bundle 2C). Range `[0, NpcStatCount)`. Already wraps NPC_STATADD / NPC_STATSUB / NPC_STATHEAL (write path). **Not** called at NPC_STAT / NPC_BASESTAT (read path) — the two remaining raw sites. |
| `NpcStatCount` | pkg/objtype/npcstat.go | Exported. Value: 6. Matches TS `NpcStat._TOTAL`. |
| `handleNpcDamage` | handlers_npc.go:343-354 | `PopInt` for amount (with `checkNotNull`); `PopInt` for dmgType (raw, no check). Doc comment L341-342 says "wrapped with HitTypeValid (not NumberNotNull) and stays raw (NAI-23 Bundle 4a)" — the wrapping was deliberately deferred. |
| `handleDamage` (P_DAMAGE) | handlers_player.go:1516-1529 | `PopInt` for amount; `PopInt` for hitType (raw); `PopInt` for uid; UID lookup → `player.ApplyDamage(amount, hitType)`. No validators. |
| `handleNpcStat` | handlers_npc.go:170-177 | `PopInt` for stat (raw); pushes `s.ActiveNpc.NpcStat(stat)`. |
| `handleNpcBaseStat` | handlers_npc.go:180-187 | `PopInt` for stat (raw); pushes `s.ActiveNpc.NpcBaseStat(stat)`. |

Net gap: one new const group, one new validator, four call-site wraps, one doc-comment refresh.

## 5. Design

### 5.1 New file `pkg/objtype/hittype.go`

```go
package objtype

// HitType wire values used by the client hitmark encoding and by
// RuneScript callers of NPC_DAMAGE / P_DAMAGE. Mirrors TS
// Engine-TS/src/engine/entity/HitType.ts:1-5.
const (
    HitTypeBlock  = 0
    HitTypeDamage = 1
    HitTypePoison = 2

    HitTypeCount = 3 // exclusive upper bound for HitTypeValid
)
```

Mirrors the layout of `pkg/objtype/hunttype.go` (analogous enum-of-wire-values + count sentinel).

### 5.2 New validator in `pkg/script/handlers_npc.go`

Inserted alongside `checkNpcStatID` / `checkNpcMode` / `checkNpcType`:

```go
// checkHitType validates a hit-type wire value against
// objtype.HitTypeCount. Mirrors TS HitTypeValid (ScriptValidators.ts:117)
// — ScriptInputRangeValidator(0, 3). Accepts BLOCK / DAMAGE / POISON.
func checkHitType(v int, op string) error {
    if v < 0 || v >= objtype.HitTypeCount {
        return fmt.Errorf("%s: hit type out of range (%d)", op, v)
    }
    return nil
}
```

### 5.3 Apply at four call sites

| Site | Change |
| --- | --- |
| `handlers_npc.go:343-354` (handleNpcDamage) | After `dmgType := s.PopInt()` insert `if err := checkHitType(dmgType, "NPC_DAMAGE"); err != nil { return err }`. |
| `handlers_npc.go:170-177` (handleNpcStat) | After `stat := s.PopInt()` insert `if err := checkNpcStatID(stat, "NPC_STAT"); err != nil { return err }`. |
| `handlers_npc.go:180-187` (handleNpcBaseStat) | After `stat := s.PopInt()` insert `if err := checkNpcStatID(stat, "NPC_BASESTAT"); err != nil { return err }`. |
| `handlers_player.go:1516-1529` (handleDamage) | After `hitType := s.PopInt()` insert `if err := checkHitType(hitType, "DAMAGE"); err != nil { return err }`. The existing `s.World == nil` and `player == nil` silent-no-op gates remain unchanged downstream. |

### 5.4 Doc-comment refresh

At handlers_npc.go:341-342, replace the stale wording:

> ```
> Mirrors TS NpcOps.ts NPC_DAMAGE: check(amount, NumberNotNull); dmgType is
> wrapped with HitTypeValid (not NumberNotNull) and stays raw (NAI-23 Bundle 4a).
> ```

with:

> ```
> Mirrors TS NpcOps.ts NPC_DAMAGE: check(amount, NumberNotNull) +
> check(dmgType, HitTypeValid). Goscape mirrors via checkNotNull +
> checkHitType.
> ```

No other doc-comments require updates — the NPC_STAT / NPC_BASESTAT / DAMAGE handlers don't currently carry "stays raw" wording that becomes stale.

## 6. Data flow

```
script bytecode
  → PopInt (HitType or NpcStat wire value)
    → checkHitType / checkNpcStatID
      ├── ok    → existing handler body proceeds unchanged
      └── error → return up to ScriptState.Execute loop → script halts with
                  opcode-tagged error (existing error pathway, no change)
```

No change to wire format. No change to applied damage magnitude. No change to NPC stat values. Pure script-input safety: bytecode that pushes an out-of-range value now halts the script with a clear error instead of silently triggering downstream UB (array out-of-bounds panic at `npc.levels[stat]` for example, or a downstream consumer interpreting an invalid hitType as DAMAGE).

## 7. Error semantics

- **Existing `checkNpcStatID`** already returns `fmt.Errorf("%s: npc stat id out of range (%d)", op, id)`. Re-used verbatim.
- **New `checkHitType`** returns `fmt.Errorf("%s: hit type out of range (%d)", op, v)`. Mirrors the wording of the existing range validators.
- **Propagation** uses Go's `return err` from the handler function — `ScriptState.Execute` (existing) consumes the error and halts the script. Same pathway as every other validator-emitting handler today; nothing new at the runtime layer.

## 8. Testing

Mirrors existing validator-test patterns in `pkg/script/handlers_npc_test.go` and `pkg/script/handlers_player_test.go`.

### 8.1 Unit tests for `checkHitType`

Table-driven test `TestCheckHitType`. Cases:

| Input | Expected |
| --- | --- |
| -1 | error (below range) |
| 0 (BLOCK) | ok |
| 1 (DAMAGE) | ok |
| 2 (POISON) | ok |
| 3 | error (at exclusive upper bound) |
| 100 | error (well above) |

Mirrors the boundary-set used by `TestCheckHuntVis` (handlers_npc_test.go:77) and `TestCheckCategoryType` (handlers_npc_test.go:90) for the existing range validators.

### 8.2 Handler invalid-input tests

Four new tests:

| Test | Handler | Pushed input | Expected |
| --- | --- | --- | --- |
| `TestHandleNpcDamage_InvalidHitType` | handleNpcDamage | amount=5, dmgType=3 | error; NPC HP unchanged |
| `TestHandleDamage_InvalidHitType` | handleDamage | amount=5, hitType=3, uid=any | error; uid never popped (validator short-circuits); no `ApplyDamage` call observed |
| `TestHandleNpcStat_InvalidStat` | handleNpcStat | stat=NpcStatCount | error; no value pushed |
| `TestHandleNpcBaseStat_InvalidStat` | handleNpcBaseStat | stat=NpcStatCount | error; no value pushed |

Each test sets up a minimal `ScriptState` via existing test scaffolding, pushes inputs in TS-faithful LIFO order, calls the handler, asserts on the returned error string + entity state.

### 8.3 Regression coverage

Existing valid-input tests in `handlers_npc_test.go` (NPC_DAMAGE / NPC_STAT / NPC_BASESTAT) and `handlers_player_test.go` (DAMAGE / TestDamage_NoPointerGate) MUST still pass without modification. Validators reject only previously-undefined input; in-range bytecode is unaffected.

## 9. Out of scope

- Other validator-family ports (`InvTypeValid`, `NpcTypeValid`, `ObjTypeValid`, etc.) — these are config-registry validators that require live `Configs.XxxType(id)` lookups, not simple range checks; they're a different porting story. Most are already partially-ported via inline `Configs.XxxType(id) != nil` patterns.
- Extracting the inline-validated `Queue` check (handlers_npc.go:467-468) and `SkinColour` check (handlers_player.go:1659-1660) into standalone validators. Both work correctly inline; extraction is cosmetic.
- Any behavioral change to `Player.Damage` / `Npc.Damage` (HP decrement on BLOCK, hitmark suppression on POISON, etc.) — TS does none of this at the engine layer; the wire `type` is purely a packet annotation.

## 10. Acceptance criteria

1. New file `pkg/objtype/hittype.go` with three exported constants + `HitTypeCount`.
2. New function `checkHitType` in `pkg/script/handlers_npc.go`.
3. Four handler call sites apply their respective validator (`checkHitType` × 2, `checkNpcStatID` × 2) immediately after the relevant `PopInt`.
4. Doc-comment at handlers_npc.go:341-342 refreshed.
5. New tests: 1 validator unit test + 4 handler invalid-input tests, all passing.
6. Existing tests in `pkg/script/` and `modules/world/` all pass (`go test -race ./...` clean).
7. Smoke-pack still passes (12 OK / 0 ERR / 0 SKIP).
