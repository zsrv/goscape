# Hunt huntvis filter activation — Distance-mode iterator + FindClosest* prongs

## Status

- **Date:** 2026-05-21
- **Predecessor:** AddXP session-log half port (HEAD `56517545`).
- **Scope:** Activate validated-but-unconsumed huntvis (LoS/LoW) filtering at the two pin sites that hold the `NAI-33-D1 / S7f-D1` deferred posture. Refresh `NAI-35-T3` framing across the codebase.
- **Pins retired:** `NAI-33-D1`, `S7f-D1` (full retire, both prongs closed).
- **Pins refreshed (citation kept, framing scrubbed):** `NAI-35-T3` (no longer "only mode that activates").
- **Out-of-scope pins:** `S7f-D2` (linear-scan vs iterator-DISTANCE architectural divergence — independent of huntvis activation).

## Audit gate satisfied

The deferred posture at `pkg/script/npc_iterator.go:43-46` and `modules/world/npc_script_lookup.go:8-11` is explicitly conditioned on "audit if FINDALL-family / FIND-family consumers gain LoS/LoW gating". A `LostCityRS/Content/scripts` audit identified production consumers:

**NPC_FIND** (routes through `serverNpcLookup.FindClosestNpcByType`):
- `quests/quest_chompybird/scripts/chompy_bird.rs2:24` — `^vis_lineofwalk`
- `quests/quest_chompybird/scripts/rantz.rs2:343` — `^vis_lineofsight`
- `quests/quest_fluffs/scripts/quest_fluffs.rs2:10, 27, 38` — `^vis_lineofsight` (3 instances)

**NPC_FINDALL** (routes through `NpcIterator` DISTANCE mode):
- `general/scripts/flavour_text/ducks.rs2:41` — `^vis_lineofsight`

**Non-consumers (no action needed):**
- `npc_findcat` — 0 non-zero-huntvis consumers found; activation still TS-faithful (TS NPC_FINDCAT also uses `NpcIterator` DISTANCE-mode).
- `npc_findallany` — 0 non-zero consumers; same TS-fidelity argument.
- `npc_findallzone` — no huntvis arg at all (engine.rs2:605 `[command,npc_findallzone](coord $coord)`); TS Zone-mode iterator yields unfiltered (`ScriptIterators.ts:329-335`). Stays unfiltered.

## TS-faithful target behavior

### NpcIterator DISTANCE mode (`ScriptIterators.ts:336-362`)

```
for each NPC in zone-cursor sweep:
  distance check (line 345)
  if vis_lineofsight: isLineOfSight(level, this.x, this.z, npc.x, npc.z) (line 348)
  if vis_lineofwalk:  isLineOfWalk(level, this.x, this.z, npc.x, npc.z)  (line 351)
  if npcType set:     filter by type                                       (line 354)
  yield npc
```

Identical arg-tuple shape to HuntAll mode (line 284, 287). Both call into `isLineOfSight`/`isLineOfWalk` wrappers at `GameMap.ts:429-431` / `425-427` which expand to the `(1, 1, 1, 0)` flag tuple. Already-correct in `npcVisibleViaLineOfSight`/`Walk` per `NAI-166-D-LOW-ARG-SHAPE-SWEEP`.

### NpcIterator ZONE mode (`ScriptIterators.ts:329-335`)

Unfiltered yield — no huntvis check at all. Preserved.

### NPC_FIND / NPC_FINDCAT (`NpcOps.ts:336-400`)

TS uses `NpcIterator(DISTANCE)` then loops for closest-by-euclidean-squared. goscape uses `serverNpcLookup.FindClosestNpcByType/ByCategory` direct scan (S7f-D2 architectural divergence — preserved). Semantic-parity requirement: same filter gates (distance + huntvis + type/category), same closest-by-euclidean-squared selection with later-match-wins tie-break.

## Architecture & component changes

### Prong A — NpcIterator DISTANCE-mode (`pkg/script/npc_iterator.go`)

**1. `passesFilter` restructure (lines 83-129):**

```
if mode == Zone:                       return true        // unchanged
if mode == HuntAll && configs != nil:  op[1] gate         // unchanged (NAI-180)
distance check                                            // unchanged
huntvis switch (NEW: Distance + HuntAll, was HuntAll-only)
typeID filter                                             // unchanged
return true
```

The huntvis switch body is identical to today's HuntAll branch — same `npcVisibleViaLineOfSight`/`npcVisibleViaLineOfWalk` calls. **Move it, don't duplicate.**

**2. `NewDistanceNpcIterator` signature change (line 175):**

```go
func NewDistanceNpcIterator(lookup NpcLookup, lv LineValidator, tick, level, x, z, distance, huntvis, typeID int) *NpcIterator
```

Sets `lineValidator: lv` in the struct literal. Position chosen to mirror `NewHuntAllNpcIterator` arg order (`lookup, lv, configs, tick, ...`) — `configs` not added because Distance mode has no op[1] gate.

**3. Doc-comment refreshes (5 sites in this file):**

- `huntvis` field comment (lines 41-46) — drop "Distance and Zone modes validate but do not filter, preserving NAI-33-D1's deferred-not-consumed posture for the FINDALL/FINDALLANY/FINDALLZONE family. Audit if those families gain LoS/LoW content-script consumers." Replace with: "Consumed by passesFilter in Distance + HuntAll modes (TS ScriptIterators.ts:348-352 / 284-287). Zone mode unfiltered (TS line 329-335 — no huntvis arg at the npc_findallzone command site either)."
- `lineValidator` field comment (lines 48-52) — widen scope from "HuntAll-mode passesFilter" to "Distance + HuntAll modes".
- `passesFilter` doc (lines 83-91) — rewrite to describe the unified-Distance+HuntAll shape; drop "Distance mode keeps the pre-NAI-35 deferred behavior (huntvis validated but not consumed; tracked as NAI-33-D1 / S7f-D1)".
- `NewDistanceNpcIterator` doc (lines 156-174) — drop the entire paragraph beginning "huntvis is stored at construction (validated upstream by handlers via checkHuntVis) but NOT consumed by passesFilter — preserves the NAI-33-D1 deferred posture for FINDALL family (no LoS/LoW content-script consumers identified). HuntAll mode (NAI-35-T3) is the only iterator-mode that activates the huntvis filter." Replace with brief "huntvis is consumed by passesFilter per TS ScriptIterators.ts:348-352; `lv` may be nil (pessimistic-allow)."
- `NewHuntAllNpcIterator` doc (lines 215-219) — drop "partially closes NAI-33-D1 for HuntAll mode; Distance mode + FindClosest* still residual" — HuntAll is no longer special.

### Prong A wiring — `pkg/script/handlers_npc.go`

**4. `handleNpcFindAllAny` (line 835):** pass `s.LineValidator` as 2nd arg to `NewDistanceNpcIterator`.

**5. `handleNpcFindAll` (line 873):** same.

**6. Doc-comment refreshes (2 sites):**

- Lines 805-813 (`handleNpcFindAllAny`) — drop "NAI-33-D1: huntvis validated but not consumed by passesFilter (Distance mode preserves the deferred-not-consumed posture; HuntAll mode at NAI-35-T3 is the only mode that activates LoS/LoW filtering)."
- Lines 842-849 (`handleNpcFindAll`) — same scrub.

### Prong B — `modules/world/npc_script_lookup.go`

**7. `FindClosestNpcByType` (line 24):** rename `_ int` → `huntvis int`. Insert huntvis filter after the `dx/dz > dist` square-bounds check (after line 41) and before the distance-compute (line 42):

```go
if !l.huntvisGate(level, x, z, n.x, n.z, huntvis) {
    continue
}
```

**8. Private helper:**

```go
// huntvisGate applies the HuntVisOff/LineOfSight/LineOfWalk filter using
// the server's scriptLineValidator. Nil-validator → pessimistic-allow,
// matching the pkg/script iterator convention at
// npc_iterator.go:138-141 (npcVisibleViaLineOfSight).
//
// Arg tuple (1, 1, 1, 0) and source-as-iterator-coord ordering mirror
// TS NpcIterator DISTANCE-mode at ScriptIterators.ts:348/351 — NOT the
// player-iterator-reversed shape at line 216 (see PlayerIterator
// passesFilter for that variant).
func (l serverNpcLookup) huntvisGate(level, srcX, srcZ, dstX, dstZ, huntvis int) bool {
    switch huntvis {
    case objtype.HuntVisOff:
        return true
    case objtype.HuntVisLineOfSight:
        lv := l.s.scriptLineValidator()
        if lv == nil {
            return true
        }
        return lv.HasLineOfSight(level, srcX, srcZ, dstX, dstZ, 1, 1, 1, 0)
    case objtype.HuntVisLineOfWalk:
        lv := l.s.scriptLineValidator()
        if lv == nil {
            return true
        }
        return lv.HasLineOfWalk(level, srcX, srcZ, dstX, dstZ, 1, 1, 1, 0)
    }
    return true
}
```

**9. `FindClosestNpcByCategory` (line 62):** rename `_ int` → `huntvis int`. Insert identical `huntvisGate` call at the same point in the loop (after line 88, before line 90).

**10. Doc-comment refreshes (3 sites):**

- Package comment (lines 8-11) — drop "and the residual deviation NAI-33-D1 / S7f-D1 (huntvis validated-only on FindClosest* — partially closed by NAI-35 for HuntAll-mode iterators)". Keep S7f-D2 reference (linear iteration).
- `FindClosestNpcByType` doc (lines 21-23) — drop "huntvis is validated upstream but NOT filtered on here — preserves the NAI-33-D1 / S7f-D1 deferred posture (audit if NPC_FIND consumers gain LoS/LoW gating). HuntAll-mode iterators (NewHuntAllNpcIterator) DO filter; this method does not." Replace with: "huntvis applied via huntvisGate (nil-validator pessimistically allows); semantic-equivalent to TS NPC_FIND's NpcIterator(DISTANCE)-then-closest at NpcOps.ts:347-365."
- `FindClosestNpcByCategory` doc (lines 59-61) — same scrub, cite `NpcOps.ts:380-396`.

### Prong B test infrastructure decision

`Server.scriptLineValidator()` reads `s.gamemap.Pathfinder.LineValidator`; returns `nil` when `s.gamemap == nil`. New FindClosest tests need a stub LineValidator.

**Approach:** Use the existing pattern — search for stub setup in `modules/world/*_test.go` that already wires a Pathfinder LineValidator for hunt tests; reuse. If no such pattern exists, the cheapest path is to introduce a private test-only field `s.lineValidatorTest` on `Server` and have `scriptLineValidator()` prefer it when non-nil. Decision deferred to T3 implementer; document choice in T3 commit message.

### Sweep — additional NAI-33-D1 / NAI-35-T3 framing

**11. `pkg/script/state.go` (lines 195-200, NpcLookup interface comment):** drop "FindClosestNpcByType / FindClosestNpcByCategory currently validate huntvis but do not filter on it (NAI-33-D1 / S7f-D1 residual after NAI-35 — HuntAll-mode iterators NewHuntAllNpcIterator / NewHuntAllPlayerIterator DO filter). Callers must still validate via checkHuntVis." Replace with: "Implementations apply LoS/LoW filtering per TS Distance-mode semantics (ScriptIterators.ts:348-352). Callers validate via checkHuntVis upstream."

**12. `pkg/script/handlers_npc.go` (lines 905-908, `handleNpcHuntAll` doc):** drop "Distance mode + FindClosestNpc* still residual" — the residual is gone.

### No changes

- `pkg/script/player_iterator.go` — single-mode (NAI-35-D2), no deferred-not-consumed surface.
- `modules/world/npc_hunt*.go` — separate hunt-engine code path, already filters.
- `NewZoneNpcIterator` — Zone mode stays unfiltered.
- `serverNpcLookup{s: s}` test fixtures (~30 sites) — struct shape unchanged.
- `FindNpcAtExactCoord` — no huntvis arg, TS unfiltered.
- `FindNpcByUID` — slot/type lookup, no spatial component.

## Testing strategy

### New unit tests — Prong A (`pkg/script/npc_iterator_test.go`)

Reuse existing `lineValidatorStub` pattern in this file.

1. `TestNpcIteratorDistance_HuntVisOff_NoFilter` — baseline: matching NPC inside distance, huntvis=`HuntVisOff` → emitted regardless of validator returns.
2. `TestNpcIteratorDistance_HuntVisLineOfSight` — table-driven 2×2: stub returns true → emit; stub returns false → skip.
3. `TestNpcIteratorDistance_HuntVisLineOfWalk` — same shape with `HasLineOfWalk`.
4. `TestNpcIteratorDistance_NilLineValidator_PessimisticAllow` — `lv=nil` with `vis_lineofsight` set → NPC still emitted (matches existing HuntAll convention at `npc_iterator.go:138-141`).
5. `TestNpcIteratorDistance_LineOfSightArgShape` — assert validator received exact tuple `(level, it.x, it.z, npc.NpcX(), npc.NpcZ(), 1, 1, 1, 0)`. Guards `NAI-166-D-LOW-ARG-SHAPE-SWEEP` precedent.
6. `TestNpcIteratorZone_HuntVisStillUnfiltered` — explicit pin: Zone mode with huntvis=`vis_lineofsight` and always-false validator → NPC still emitted (TS `ScriptIterators.ts:329-335`).

### Existing fixture updates — Prong A

`NewDistanceNpcIterator` signature gains `lv LineValidator`. Five call sites in `npc_iterator_test.go` (lines 73, 166, 207, 228, 244) need `nil` inserted as 2nd positional arg. Two production sites in `handlers_npc.go` (lines 835, 873) get `s.LineValidator`.

### Handler-level smoke tests — Prong A (`pkg/script/handlers_npc_test.go`)

Plumbing-only — iterator tests cover behavior.

7. `TestHandleNpcFindAll_PlumbsLineValidatorToIterator` — wire `s.LineValidator` to a stub returning false; run FINDALL → FINDNEXT loop with `vis_lineofsight` → zero hits (proves `s.LineValidator` reaches iterator).
8. `TestHandleNpcFindAllAny_PlumbsLineValidatorToIterator` — same shape.

Existing FINDALL/FINDALLANY tests using huntvis=`vis_off` or `s.LineValidator=nil` stay green (pessimistic-allow).

### New unit tests — Prong B (`modules/world/npc_script_lookup_test.go`)

9. `TestFindClosestNpcByType_HuntVisOff_Baseline` — existing behavior preserved (regression guard).
10. `TestFindClosestNpcByType_HuntVisLineOfSight_FiltersBlocked` — 2 candidates same dist; only LoS-passer emitted.
11. `TestFindClosestNpcByType_HuntVisLineOfWalk_FiltersBlocked` — same with LoW.
12. `TestFindClosestNpcByType_NilLineValidator_PessimisticAllow` — `s.gamemap=nil` ⇒ `scriptLineValidator()=nil` ⇒ filter pessimistically allows; closest match returned regardless of huntvis value.
13. `TestFindClosestNpcByType_HuntVisAfterDistance_ClosestStillWins` — 2 LoS-passing NPCs at different distances; closer one wins (validates filter doesn't disturb closest-by-euclidean-squared selection or `<=` later-match-wins tie-break).
14. `TestFindClosestNpcByType_LineOfSightArgShape` — assert arg tuple `(level, lookupX, lookupZ, npcX, npcZ, 1, 1, 1, 0)`.
15. `TestFindClosestNpcByCategory_HuntVisOff_Baseline` — regression guard.
16. `TestFindClosestNpcByCategory_HuntVisLineOfSight_FiltersBlocked`.
17. `TestFindClosestNpcByCategory_HuntVisLineOfWalk_FiltersBlocked`.
18. `TestFindClosestNpcByCategory_NilLineValidator_PessimisticAllow`.

(Skip a per-category arg-shape test — same helper `huntvisGate` covers both type and category variants; one arg-shape test in #14 suffices.)

### Audit-grep gates (zero hits in production `.go` post-slice)

- `NAI-33-D1` — fully retired.
- `S7f-D1` — alias, same retire.
- `deferred-not-consumed posture` — phrase scrub.
- `HuntAll is the only mode` / `HuntAll-mode is the only mode that activates` — phrase scrub.
- `Distance mode + FindClosest* still residual` — phrase scrub.
- `validated upstream but NOT filtered on here` / `validated upstream by handlers via checkHuntVis) but NOT consumed` — phrase scrub.

### Allowed citations preserved

- `NAI-35-T3` — task-label for HuntAll-mode filtering provenance; keep.
- `NAI-180` — closes `NAI-35-T3-D1` op[1] gate; unrelated, keep.
- `NAI-166-D-LOW-ARG-SHAPE-SWEEP` — arg-shape provenance, keep.
- `S7f-D2` — linear-scan vs iterator-DISTANCE architectural divergence, keep (separate concern).
- TS line citations (`ScriptIterators.ts:348-352`, `:329-335`, `:284-287`, `NpcOps.ts:336-400`) — keep, expand.

### Smoke + race gates

- `-race ./...` 57 pkgs / 0 FAIL.
- `TestPackAll_TwelveStageSmoke` PASS.

## Pin retirement table

| Pin | Pre-slice | Post-slice | Notes |
|---|---|---|---|
| `NAI-33-D1` | Live (~141 board) | **Retired** | Audit gate satisfied: chompy_bird, rantz, quest_fluffs ×3 (NPC_FIND), ducks (NPC_FINDALL). |
| `S7f-D1` | Live alias | **Retired** | Same retire — alias of NAI-33-D1. |
| `NAI-35-T3` | Live citation | Live citation (refreshed) | Task-label preserved; "only mode that activates" framing scrubbed. |
| `NAI-180` | Live | Live (unchanged) | Op[1] gate, unrelated. |
| `S7f-D2` | Live | Live (unchanged) | Architectural divergence in FindClosest*; semantic parity sufficient for huntvis activation. |

Net board: ~141 live → **~139 live** (-2).

## Non-obvious risks

1. **Arg-shape regression** — `NAI-166-D-LOW-ARG-SHAPE-SWEEP` previously fixed `(1,0,0,0)` → `(1,1,1,0)` and the player-iterator-reversed shape at `ScriptIterators.ts:216`. NPC iterator + FindClosest both use iterator-as-src `(level, it.x, it.z, npc.x, npc.z, 1, 1, 1, 0)` per TS `:348` (NPC variant). Test #5 (iterator) + #14 (FindClosest) guard.

2. **Pessimistic-allow convention consistency** — Both prongs follow the same rule: nil LineValidator → return-true. Iterator at `npc_iterator.go:138-141`, new `huntvisGate` helper mirrors. Documented inline at helper definition.

3. **FindClosest tie-break preservation** — Existing semantic: later-iterated matches win ties (`<= bestDist`). Inserting huntvis filter narrows candidates without disturbing iteration order. Test #13 guards.

4. **Production test path regression risk** — `s.LineValidator` is wired via `scriptLineValidator()` from `s.gamemap.Pathfinder.LineValidator`. If `s.gamemap == nil` (test fixture), result nil → pessimistic-allow → existing tests preserved without modification. Tests that DON'T set up gamemap continue to pass huntvis=anything and see all candidates.

5. **`paramLookup`-style cascade not needed here** — Unlike the config-registry validator family (`config_registry_validator_family_close.md` non-obvious finding #1), `huntvisGate` is a per-method-private helper, not a shared cascade. No cross-handler signature change required.

6. **NAI-35-T3 retention rationale** — It's a task label, not a deviation pin. References like "NAI-35-T3 HuntAll filtering" remain accurate as provenance; only the subordinate "only mode that activates" framing gets scrubbed. Don't mass-delete the marker.

## Sequencing (T1 → T5 outline for plan stage)

1. **T1 — Iterator core** (`pkg/script/npc_iterator.go` + tests). `passesFilter` restructure + `NewDistanceNpcIterator` sig change + new unit tests 1-6 + 5 existing fixture updates. Single-file scope (plus test file).
2. **T2 — Iterator production wiring** (`pkg/script/handlers_npc.go` + tests). 2 call sites + 2 doc-comment refreshes + handler smoke tests 7-8. Depends on T1.
3. **T3 — FindClosest core** (`modules/world/npc_script_lookup.go` + tests). `huntvisGate` helper + 2 method body updates + doc refreshes + new unit tests 9-18. **Independent of T1/T2** (different file, different package boundary). Decide stub-LineValidator infrastructure approach in commit msg.
4. **T4 — Doc-comment sweep** (`pkg/script/state.go:195-200` + `pkg/script/handlers_npc.go:905-908` + audit-grep verification). Last; sweeps any framing left after T1-T3.
5. **T5 — Close** — race gate + smoke gate + memory entry + pin retire. No production code.

Dependencies: T1→T2 sequential. T3 independent. T4 after T1+T2+T3. T5 finalization.

Suggested execution: subagent-driven-development with sonnet implementers (T1-T4) + two-stage review per task (sonnet spec-reviewer + sonnet code-reviewer) + opus whole-slice reviewer after T4.

## Out of scope (explicit do-not-do)

- Refactoring NPC_FIND/FINDCAT to use `NpcIterator` instead of `FindClosestNpc*` (S7f-D2; separate decision; semantic parity sufficient here).
- ZONE-mode huntvis activation (TS `ScriptIterators.ts:329-335` unfiltered).
- `FindNpcAtExactCoord` huntvis (TS unfiltered).
- PlayerIterator audit (single-mode by NAI-35-D2).
- Hunt-engine LineValidator changes (`modules/world/npc_hunt*.go` already filters in its own code path).
- Compiler-side ai_queueN rework (phantom carry-forward).

## Acceptance criteria

- Both prongs filter huntvis at the production call sites that previously discarded it.
- Audit-grep gates listed under Testing § return zero production hits.
- All new tests (1-18) pass; all existing tests pass without behavioral assertion changes (only signature/fixture updates).
- `-race ./...` 0 FAIL across 57+ pkgs.
- `TestPackAll_TwelveStageSmoke` PASS.
- NAI-33-D1 + S7f-D1 retired; NAI-35-T3 refreshed; net pin board -2.
