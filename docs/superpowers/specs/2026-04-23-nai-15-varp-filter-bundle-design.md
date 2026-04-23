# NAI-15 — `checkVars` + `checkNotCombat` + Outer Combat Guard in `huntPlayers`

Close two of the three deferred varp-dependent filters in `huntPlayers`
by porting TS `Npc.ts:942-957`: the outer multi-zone/target-equality
guard, `checkNotCombat`, and `checkVars`. Also ports the supporting
operator evaluator `HuntType.checkHuntCondition` (TS `HuntType.ts:63-75`)
as a method on `*objtype.HuntType`.

Scope is **bounded by the `Player.Varp` infrastructure landed in S5b**:
the third combat filter `checkNotCombatSelf` reads **NPC-side** vars via
TS `Npc.getVar` / `VarNpcType`, which have no Go analogue yet and remain
a tracked follow-up.

**Roadmap:** Third sub-spec in the NAI-8 hunt-filter backfill track
(NAI-8 landed range/level/checkAfk; NAI-12 landed CheckVis LoS/LoW;
NAI-15 closes checkNotCombat + checkVars). Carries forward the NAI-8
deferred-filter comment block in `modules/world/npc_hunt.go:96-103`.
Fidelity risk: **Low** — straight port; `Player.Varp`, `s.currentTick`,
and `s.gamemap.IsMulti` are all established infrastructure.

**Tech Stack:** Go 1.26+. No new packages. Existing `pkg/objtype`
(`HuntType`, `HuntCheckVar`), `pkg/gamemap` (`IsMulti`), `modules/world`
(`Player.Varp`, `Server.currentTick`, `Npc.target`).

## Goal

After NAI-15 ships:

1. **`(*HuntType).CheckHuntCondition`** method added to
   `pkg/objtype/hunttype.go` — mirrors TS `HuntType.checkHuntCondition`
   at `Engine-TS/src/cache/config/HuntType.ts:63-75`. Switch on the
   condition string (`>`, `<`, `=`, `!`); unknown operator → `false`
   (TS default-case behavior — fail-closed for malformed hunt data).
2. **`(*Npc).huntPlayers`** at `modules/world/npc_hunt.go:107-161` gains
   three new filter stages between the existing CheckVis gate (line
   157) and the final `append` (line 158), in TS order:
   - **Outer combat guard** (TS:942): `applyCombatGuard := entity(p) !=
     n.target && (s.gamemap == nil || !s.gamemap.IsMulti(p.x, p.z,
     p.level))`. Gates the two inner combat-var filters. Nil-gamemap
     short-circuits to "treat as not-multi" so the guard applies and
     the combat filter fires — matches the § error-handling table below.
   - **`checkNotCombat`** (TS:943-945): when guard applies and
     `hunt.CheckNotCombat != -1`, skip the player if
     `int(p.Varp(hunt.CheckNotCombat)) + 8 > s.currentTick`.
   - **`checkVars`** (TS:950-957): AND-chain over `hunt.CheckVars`.
     Each entry passes if `cv.VarID == -1` OR
     `hunt.CheckHuntCondition(int(p.Varp(cv.VarID)), cv.Condition, cv.Val)`
     returns true. Any failing entry skips the player.
3. **Deferred-filter comment block** at `modules/world/npc_hunt.go:85-106`
   rewritten to reflect NAI-15's closures and the surviving deferral
   list (checkNotBusy, checkNotTooStrong, checkNotCombatSelf, checkInv).
   `checkNotCombatSelf`'s deferral note cites the missing NPC-vars
   infrastructure as the concrete blocker.
4. **Tests added** — 5 new tests (per-filter T1 shape):
   - `TestHuntTypeCheckHuntCondition` in `pkg/objtype/hunttype_test.go`
     — table-driven, 5 rows (`>`, `<`, `=`, `!`, unknown-op).
   - `TestHuntPlayersCheckVars` in `modules/world/npc_hunt_test.go`.
   - `TestHuntPlayersCheckNotCombat` in `modules/world/npc_hunt_test.go`.
   - `TestHuntPlayersCombatGuard` in `modules/world/npc_hunt_test.go`.
5. **Memory entry** `nai_followups.md` records the NPC-vars-infra
   follow-up (scope: `VarNpcType` registry + `Npc.vars []int32` +
   `Npc.Varp(id) int32` method; consumer: `checkNotCombatSelf` at
   TS:946-948).

## Architecture

### §1. File-by-file delta

| File | Change |
|------|--------|
| `pkg/objtype/hunttype.go` | + `CheckHuntCondition(value int, condition string, checkValue int) bool` method on `*HuntType` |
| `pkg/objtype/hunttype_test.go` | + `TestHuntTypeCheckHuntCondition` (table-driven) |
| `modules/world/npc_hunt.go` | Insert combat guard + `checkNotCombat` + `checkVars` in `huntPlayers`; rewrite deferred-filter comment block |
| `modules/world/npc_hunt_test.go` | + `TestHuntPlayersCheckVars`, `TestHuntPlayersCheckNotCombat`, `TestHuntPlayersCombatGuard` |

No new files. No new packages. No changes to `pkg/gamemap`, `Player`,
`Npc`, or `Server`.

### §2. `CheckHuntCondition` shape

```go
// CheckHuntCondition evaluates condition against (value, checkValue) using the
// hunt-config operator string. Mirrors TS HuntType.checkHuntCondition at
// Engine-TS/src/cache/config/HuntType.ts:63-75. Unknown operators return
// false (TS default-case behavior — fail-closed for malformed hunt data).
func (t *HuntType) CheckHuntCondition(value int, condition string, checkValue int) bool {
    switch condition {
    case ">":
        return value > checkValue
    case "<":
        return value < checkValue
    case "=":
        return value == checkValue
    case "!":
        return value != checkValue
    }
    return false
}
```

**Signature choice**: `int`, not `int32`. Rationale: `s.currentTick` is
`int`; widening at the call site (`int(p.Varp(id))`) keeps call-site
code clean and avoids scattering conversions across filter branches.
No overflow risk — hunt values and tick counters both fit comfortably
in `int32`'s range, let alone `int`.

**Placement rationale**: method on `*HuntType` (not a free function)
because TS puts it on the `HuntType` class, and because the dispatcher
call site reads naturally: `hunt.CheckHuntCondition(...)`. This is the
first behavioral method on `HuntType` beyond decode — establishes the
pattern for future filters (notably `checkInv`, which will use the same
helper).

### §3. Filter-chain insertion in `huntPlayers`

Pseudocode (exact code comes with the plan):

```go
// ... existing CheckAfk + CheckVis filters up to modules/world/npc_hunt.go:157 ...

// Outer combat guard — TS:942. Only when the candidate is not the NPC's
// current target AND not in a multi-combat zone.
// FIDELITY: when s.gamemap is nil, IsMulti can't be called. Treat as
// not-multi (safe default consistent with CheckVis's nil-handling in
// the same file), so the guard APPLIES and the combat filter fires.
// Note the polarity: the predicate here is "not multi" (guard wants
// that to be true), the opposite of CheckVis's "not obstructed".
applyCombatGuard := entity(p) != n.target &&
    (s.gamemap == nil || !s.gamemap.IsMulti(p.x, p.z, p.level))
if applyCombatGuard {
    // checkNotCombat (TS:943-945): skip players whose last-combat varp
    // was written within the past 8 ticks.
    if hunt.CheckNotCombat != -1 &&
        int(p.Varp(hunt.CheckNotCombat))+8 > s.currentTick {
        continue
    }
    // checkNotCombatSelf (TS:946-948) — DEFERRED: requires NPC-vars
    // infra (VarNpcType, Npc.vars, Npc.Varp). See nai_followups.md.
}

// checkVars (TS:950-957): AND-chain of varp/operator/value predicates.
// Nil/empty CheckVars → no-op (ranging nil slice yields zero iterations,
// matching TS empty-`every` → true semantics).
passCheckVars := true
for _, cv := range hunt.CheckVars {
    if cv.VarID == -1 {
        continue
    }
    if !hunt.CheckHuntCondition(int(p.Varp(cv.VarID)), cv.Condition, cv.Val) {
        passCheckVars = false
        break
    }
}
if !passCheckVars {
    continue
}

hunted = append(hunted, p)
```

### §4. Updated deferred-filter comment block

The existing comment at `modules/world/npc_hunt.go:85-106` is rewritten
to reflect NAI-15's closures and the remaining deferrals:

```go
// huntPlayers iterates the player grid in huntRange and returns
// players passing the filter chain. Matches TS Npc.huntPlayers at
// Engine-TS/.../Npc.ts:921-973.
//
// Filter coverage:
//   - Range + level match:     always
//   - checkAfk                 (NAI-8,  TS:935-937)
//   - CheckVis LoS/LoW         (NAI-12, TS per ScriptIterators.ts:88-94)
//   - Outer combat guard       (NAI-15, TS:942)
//   - checkNotCombat           (NAI-15, TS:943-945)
//   - checkVars                (NAI-15, TS:950-957)
//
// CheckVis (NAI-12) preserves the TS player-as-source / NPC-as-dest
// argument swap quirk — see FIDELITY note at the gate below.
//
// Filters DEFERRED (infra missing; each TS line cited):
//   - checkNotBusy             (TS:931-933)       — no Player.Busy()
//   - checkNotTooStrong        (TS:939-941)       — wilderness + combat-level
//   - checkNotCombatSelf       (TS:946-948)       — needs NPC-vars infra
//                                                   (VarNpcType, Npc.vars, Npc.Varp)
//   - checkInv                 (TS:959-969)       — inventory queries
//
// NAI-8 dispatches NO scripts. TS huntPlayers is a config-driven
// filter pipeline, not a script runner.
```

## Test strategy

### Unit test: `TestHuntTypeCheckHuntCondition`

Location: `pkg/objtype/hunttype_test.go`. Table-driven with 5 rows:

| Row | `value` | `condition` | `checkValue` | Expected |
|---|---|---|---|---|
| `>`-true | `5` | `">"` | `3` | `true` |
| `<`-false | `5` | `"<"` | `3` | `false` |
| `=`-true | `7` | `"="` | `7` | `true` |
| `!`-true | `7` | `"!"` | `8` | `true` |
| unknown | `5` | `"??"` | `5` | `false` |

(The operator half of each sign is exercised once in the positive
sense; the `unknown` row covers the fail-closed default.)

### Integration tests (huntPlayers-level)

All three live in `modules/world/npc_hunt_test.go` alongside the
existing `TestHuntPlayersCheckAfk` (NAI-8) and `TestHuntPlayersCheckVis*`
(NAI-12) tests. Fixtures reuse the established harness — `s.grid`
populated with one player at the NPC's range; `p.varps = make([]int32, N)`
for varp-dependent cases; NAI-12's gamemap/multimap fixture extended
with multi-zone coord entries where needed.

**`TestHuntPlayersCheckVars`** — 6 sub-cases:
- (a) `hunt.CheckVars == nil` → player included (no-filter default).
- (b) Single entry passing (`VarID`=0, `">"`, `0`; `p.varps[0]=5`) →
  included.
- (c) Single entry failing (`VarID`=0, `">"`, `10`; `p.varps[0]=5`) →
  excluded.
- (d) Two entries, both pass → included.
- (e) Two entries, second fails → excluded (AND semantics: first-pass
  doesn't override later failure).
- (f) Entry with `VarID == -1` → that entry skipped; remaining entries
  evaluated normally.

**`TestHuntPlayersCheckNotCombat`** — 5 sub-cases (window semantics are
the key fidelity point):
- (a) `hunt.CheckNotCombat == -1` (default) → player included regardless
  of varp value.
- (b) `p.varps[id] == s.currentTick` → excluded (`currentTick + 8 >
  currentTick` holds).
- (c) `p.varps[id] == s.currentTick - 7` → excluded
  (`currentTick-7+8 = currentTick+1 > currentTick` holds).
- (d) `p.varps[id] == s.currentTick - 8` → included
  (`currentTick-8+8 = currentTick > currentTick` is false — boundary
  is exclusive on the old-end).
- (e) `p.varps[id] == 0` (fresh player, no combat) with `s.currentTick
  > 8` → included.

**`TestHuntPlayersCombatGuard`** — 5 sub-cases. All use
`hunt.CheckNotCombat` set to a value that would otherwise filter the
player (i.e., `p.varps[id] == s.currentTick`) and assert whether the
guard disables the filter:
- (a) `n.target == p` → guard skipped → player included (filter
  disabled).
- (b) `s.gamemap.IsMulti(p.x, p.z, p.level) == true` → guard skipped →
  included.
- (c) `n.target == nil` → guard applies → excluded (filter fires).
- (d) `n.target` set to a different player → guard applies → excluded.
- (e) `s.gamemap == nil` → guard applies → excluded (matches fidelity
  note — nil gamemap treats as not-multi).

## Error handling & edge cases

| Case | Behavior | TS reference |
|------|----------|--------------|
| Empty/nil `CheckVars` | Filter passes (zero iterations) | TS `[].every(f) === true` |
| `CheckVars` entry with `VarID == -1` | Entry skipped, chain continues | TS:953 `checkVar.varId === -1 \|\|` short-circuit |
| Out-of-range `VarID` in `CheckVars` entry | `Player.Varp` returns 0; `CheckHuntCondition(0, ...)` runs | TS `VarPlayerType.get` returns undefined → `getVar` returns 0 (TS:1708-1710) |
| Unknown `Condition` string | `CheckHuntCondition` returns `false` → entry fails → filter excludes | TS default-case fall-through (HuntType.ts:74) |
| `hunt.CheckNotCombat == -1` | Combat filter disabled (gate short-circuit) | TS:943 `!== -1` gate |
| `s.gamemap == nil` | Combat guard still applies (treats as not-multi) | Safe default — matches CheckVis nil-short-circuit in same file |
| `n.target == nil` | Guard applies (nil != p) | TS:942 `this.target !== player` — null path |

## Fidelity deviations

None. Straight port.

- `CheckHuntCondition` placement on `*HuntType` matches TS class
  structure.
- Filter order (outer guard → checkNotCombat → checkVars) matches TS
  `Npc.ts:942-957`.
- Edge-case semantics (nil/empty CheckVars, VarID=-1, unknown operator,
  out-of-range VarID) all match TS behavior verbatim.
- The `s.gamemap == nil` fidelity note preserves the existing CheckVis
  convention in the same file rather than inventing new behavior.

## Follow-ups recorded at close

**`nai_followups.md` entry** (new or updated in the existing NAI series
section):

> **NPC-vars infrastructure** — Required to close `checkNotCombatSelf`
> in `huntPlayers`. Scope: `VarNpcType` config registry (parallels
> `VarPlayerType`), `Npc.vars []int32` field + allocation site,
> `Npc.Varp(id int) int32` method (matching `Player.Varp` signature).
> TS ref: `Npc.ts:195-198` (NPC `getVar`), `Npc.ts:946-948` (consumer).
> Deferred from NAI-15. Suggested sub-spec: NAI-NN "NPC vars +
> checkNotCombatSelf".
