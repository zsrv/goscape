---
status: brainstorm-approved
date: 2026-05-11
ts_source:
  - LostCityRS/Engine-TS/src/engine/GameMap.ts:425-427 (isLineOfWalk wrapper)
  - LostCityRS/Engine-TS/src/engine/GameMap.ts:429-431 (isLineOfSight wrapper)
  - LostCityRS/Engine-TS/src/engine/script/ScriptIterators.ts:88, 92, 113, 116, 137, 140, 160, 163, 216, 220, 284, 287, 348, 351 (iterator wrapper calls)
  - LostCityRS/Engine-TS/src/engine/entity/Npc.ts (huntAll / huntEntities — wrapper calls)
  - LostCityRS/Engine-TS/src/engine/script/handlers/ServerOps.ts:65-82 (LINEOFWALK handler — wrapper-routed)
---

# NAI-166 — Iterator/hunt-site LOW+LOS arg-shape sweep + `handleLineOfWalk` wrapper-routing

**Cadence:** ~12 production lines + ~4 new tests + new modules/world stub + ~5 LOC handler refactor + 1 test inversion + doc-comment retirement. Mid-band per `runescript_cadence.md` — three-task subagent dispatch with two-stage review per task.

**Tech stack:** Go 1.26+ (`go_version.md`).

---

## §1 Symptom / motivation

NAI-163 B1 T0 and NAI-165 closed the LOS- and LOW-wrapper arg-shape gaps at `pkg/script/handlers_map.go` (the `isLineOfSight`/`isLineOfWalk` wrappers + the `handleLineOfSight`/`handleLineOfWalk` direct calls). Both fixes were explicitly scope-narrowed to the wrapper file. The iterator and hunt-site call paths still carry the pre-fix `(1, 0, 0, 0)` arg shape, and the `handleLineOfWalk` handler still has a pessimistic-deny nil-guard that asymmetrically diverges from `handleLineOfSight`'s wrapper-routed pessimistic-allow.

This sub-spec closes the **two queued tails** (Candidate A + Candidate B from the NAI-165 close), per the brainstorm decision to bundle them under a single NAI-166.

### §1.1 Part A — call-site arg-shape sweep

Twelve production lines across four files still pass `(1, 0, 0, 0)` to `s.LineValidator.HasLineOfWalk` / `HasLineOfSight`:

| File | LOS line(s) | LOW line(s) |
|---|---|---|
| `pkg/script/player_iterator.go` | 71 | 77 |
| `pkg/script/npc_iterator.go` | 127 | 139 |
| `modules/world/npc_hunt.go` | 163 | 168 |
| `modules/world/npc_hunt_entities.go` | 68, 137, 214 | 73, 142, 219 |

TS canonical for every site is the wrapper at `GameMap.ts:425-427` (LOW) and `:429-431` (LOS), invoked verbatim from `ScriptIterators.ts:88, 92, 113, 116, 137, 140, 160, 163, 216, 220, 284, 287, 348, 351` and from `Npc.ts` huntAll / huntEntities. goscape's `srcSize` collapses TS srcWidth+srcHeight (both 1) into a single arg via RayCast at `pkg/pathfinder/routefinder/linevalidator.go:21`; destWidth/destLength pass through verbatim. TS-faithful tuple at these sites is therefore `(srcSize=1, destWidth=1, destLength=1, extraFlag=0)`.

**Stale doc-comments** at `pkg/script/npc_iterator.go:121-127` and `:133-139` already claim the helpers "mirror TS isLineOfSight/isLineOfWalk wrapper" but use the pre-fix `(1, 0, 0, 0)` shape — a doc-vs-code mismatch per `doc_comment_vs_code_mismatch.md`. Those comments need joint correction with the arg-shape fix.

### §1.2 Part B — `handleLineOfWalk` wrapper-routing + nil-LV semantics

`handleLineOfWalk` (`pkg/script/handlers_map.go` ~lines 412-446) carries an explicit `if s.LineValidator == nil { s.PushInt(0); return nil }` guard followed by a direct call to `s.LineValidator.HasLineOfWalk(...)`. The doc-comment at `:402-409` already labels this branch "(goscape defensive; TS routes through isLineOfWalk wrapper which is pessimistic-ALLOW on nil — pre-existing asymmetry vs handleLineOfSight ... tracked separately as a NAI-166 candidate)".

`handleLineOfSight` at the same file (`:236`) already routes through `isLineOfSight(s, ...)`; the wrapper at `:191-198` has its own `if s.LineValidator == nil { return true }` guard (pessimistic-allow). TS routes both opcodes through their respective wrappers.

Refactor: delete the explicit nil-guard from `handleLineOfWalk`, replace the direct LV call with `isLineOfWalk(s, fromLevel, fromX, fromZ, toX, toZ)`. This flips nil-LV semantics from pessimistic-deny (push 0) to pessimistic-allow (push 1). The existing `TestHandleLineOfWalk_ArgShape` (`handlers_map_test.go:1160`) keeps passing because the wrapper passes the same `(1, 1, 1, 0)` tuple. The existing `TestHandleLineOfWalkNilValidator` (`handlers_map_test.go:945`) flips from expect-0 to expect-1.

---

## §2 Architecture

### §2.1 Two-part decomposition (intertwined themes, separate execution)

Part A (call-site sweep) and Part B (handler refactor) share thematic root — TS-fidelity for LOW/LOS dispatch through the canonical wrapper — but no code overlap. They proceed as independent tasks under one sub-spec.

### §2.2 Test infrastructure reuse + new stub

`pkg/script/npc_iterator_test.go` already declares both `stubLineValidator` (`:11-30`, fixed-response) and `recordingLineValidator` (`:30-40`, captures full arg tuple). Same-package reuse from `player_iterator_test.go` is straightforward.

`modules/world/` has NO equivalent recording stub. Hunt-site tests at `modules/world/npc_hunt_test.go` and `modules/world/npc_hunt_entities_test.go` currently exercise the real `s.gamemap.Pathfinder.LineValidator` (a `*routefinder.LineValidator` constructed by `gamemap.New`). NAI-166 introduces a fresh `recordingLineValidator` test-double in `modules/world/npc_hunt_test.go` (same-package visible from `npc_hunt_entities_test.go`, per `test_export_underscore_test_visibility.md`). Injection works by direct field assignment on `s.gamemap.Pathfinder.LineValidator` post-`gamemap.New`; plan-author confirms the field is settable at plan-write time.

### §2.3 `handleLineOfWalk` refactor shape

After Part B:

```go
func handleLineOfWalk(s *ScriptState) error {
    c2 := s.PopInt()
    c1 := s.PopInt()

    fromLevel, fromX, fromZ, err := checkCoord(c1, "LINEOFWALK")
    if err != nil {
        return err
    }
    toLevel, toX, toZ, err := checkCoord(c2, "LINEOFWALK")
    if err != nil {
        return err
    }
    if fromLevel != toLevel {
        s.PushInt(0)
        return nil
    }
    if s.World.MapMembers() == 0 && !s.World.IsFreeToPlay(toX, toZ) {
        s.PushInt(0)
        return nil
    }
    if isLineOfWalk(s, fromLevel, fromX, fromZ, toX, toZ) {
        s.PushInt(1)
    } else {
        s.PushInt(0)
    }
    return nil
}
```

Same shape as `handleLineOfSight` (`:225-249`) modulo opcode label and wrapper name. Nil-LV handling delegated to the wrapper.

---

## §3 Changes

### §3.1 Production — Part A (12 lines)

All twelve sites flip `(1, 0, 0, 0)` → `(1, 1, 1, 0)`:

```go
// pkg/script/player_iterator.go:71  (LOS branch of player iterator)
return it.lineValidator.HasLineOfSight(it.level, p.X(), p.Z(), it.x, it.z, 1, 1, 1, 0)

// pkg/script/player_iterator.go:77  (LOW branch of player iterator)
return it.lineValidator.HasLineOfWalk(it.level, p.X(), p.Z(), it.x, it.z, 1, 1, 1, 0)

// pkg/script/npc_iterator.go:127  (LOS helper)
return it.lineValidator.HasLineOfSight(it.level, it.x, it.z, npc.NpcX(), npc.NpcZ(), 1, 1, 1, 0)

// pkg/script/npc_iterator.go:139  (LOW helper)
return it.lineValidator.HasLineOfWalk(it.level, it.x, it.z, npc.NpcX(), npc.NpcZ(), 1, 1, 1, 0)

// modules/world/npc_hunt.go:163 (LOS), :168 (LOW)
// modules/world/npc_hunt_entities.go:68/137/214 (LOS), :73/142/219 (LOW)
//   each call site: ...HasLineOfSight(..., 1, 1, 1, 0)
//                   ...HasLineOfWalk(..., 1, 1, 1, 0)
```

### §3.2 Production — Part B (handler refactor)

```go
// pkg/script/handlers_map.go — inside handleLineOfWalk

// DELETE these four lines (currently ~433-436):
//   if s.LineValidator == nil {
//       s.PushInt(0)
//       return nil
//   }

// REPLACE the direct LV call (currently ~line 436) with a wrapper call:
if isLineOfWalk(s, fromLevel, fromX, fromZ, toX, toZ) {
    s.PushInt(1)
} else {
    s.PushInt(0)
}
```

### §3.3 Doc-comment retirement

| File:line range | Action |
|---|---|
| `pkg/script/handlers_map.go:172-176` (`isLineOfWalk` wrapper preamble) | Drop the "iterator/hunt-site sweep ... tracked separately as a NAI-166 candidate" sentence; add a reference to NAI-166-D-LOW-ARG-SHAPE-SWEEP retiring the sweep. |
| `pkg/script/handlers_map.go:188-196` (`isLineOfSight` wrapper preamble) | Audit and update if it carries any symmetric LOS-sweep foreshadow (likely none, but verify at task start). |
| `pkg/script/handlers_map.go:402-409` (`handleLineOfWalk` preamble) | Drop the "goscape defensive ... NAI-166 candidate" paragraph; replace with: *"Nil-LineValidator: routes through `isLineOfWalk` wrapper (pessimistic-allow), matching `handleLineOfSight`. NAI-166-D-LOW-WRAPPER-ROUTING."* |
| `pkg/script/npc_iterator.go:121-127` | Rewrite stale "mirrors TS isLineOfSight wrapper" comment to match the corrected `(1, 1, 1, 0)` shape; cite NAI-166-D-LOW-ARG-SHAPE-SWEEP. |
| `pkg/script/npc_iterator.go:133-139` | Same for the LOW helper. |
| `pkg/script/player_iterator.go:63-77` | Scan for any stale `(1, 0, 0, 0)` references; fix in place. |

### §3.4 Tests — Part A

Four new arg-recording tests, one per affected file:

| New test | Package | Stub used |
|---|---|---|
| `TestPlayerIterator_LineValidatorArgShape` | `pkg/script/player_iterator_test.go` | Existing `recordingLineValidator` (declared in `npc_iterator_test.go`, same package) |
| `TestNpcIterator_LineValidatorArgShape` | `pkg/script/npc_iterator_test.go` | Existing `recordingLineValidator` |
| `TestHuntPlayers_LineValidatorArgShape` | `modules/world/npc_hunt_test.go` | New `recordingLineValidator` declared in this file |
| `TestHuntEntities_LineValidatorArgShape` | `modules/world/npc_hunt_entities_test.go` | Reuses the modules/world `recordingLineValidator` from `npc_hunt_test.go` |

Each test:
- Constructs the minimum iterator/hunt state to drive one LOS and one LOW call.
- Substitutes a `recordingLineValidator` for the production `LineValidator`.
- Invokes the LOS branch, asserts the recorded tuple == `(level, srcX, srcZ, destX, destZ, 1, 1, 1, 0)`.
- Invokes the LOW branch, asserts the recorded tuple == same shape.

For `TestHuntEntities_LineValidatorArgShape`, the test covers all three hunt-mode branches at `npc_hunt_entities.go:67-73`, `:136-142`, `:213-219` (these correspond to the three hunt-mode arms in `Npc.ts` huntEntities — plan-author enumerates the exact HuntType configuration that selects each branch).

### §3.5 Tests — Part B (test inversion)

`pkg/script/handlers_map_test.go:945` (`TestHandleLineOfWalkNilValidator`):

| Before | After |
|---|---|
| Expects `s.PopInt() == 0` | Expects `s.PopInt() == 1` |
| Comment: "nil validator → fail closed" | Comment: "nil validator → pessimistic-allow via `isLineOfWalk` wrapper; matches `handleLineOfSight`" |
| Test name: `TestHandleLineOfWalkNilValidator` | Rename to `TestHandleLineOfWalkNilValidatorPessimisticAllow` |

The existing `TestHandleLineOfWalk_ArgShape` (`handlers_map_test.go:1160`) continues to pass unchanged — the wrapper passes through the same `(1, 1, 1, 0)` tuple.

### §3.6 Tracked deviations

| Tag | Open at | Close at | Scope |
|---|---|---|---|
| `NAI-166-D-LOW-ARG-SHAPE-SWEEP` | T1 start | T3 end | Documents the iterator/hunt-site sweep; narrative-only retirement of the gap left forward by NAI-165. |
| `NAI-166-D-LOW-WRAPPER-ROUTING` | T3 start | T3 end | Documents `handleLineOfWalk`'s migration to wrapper-routed nil-LV handling; closes the goscape-vs-TS asymmetry NAI-165 left in place. |

Both tags grep-discoverable from the doc-comment retirements in §3.3. Both closed in the same sub-spec; retirement listed in the close commit's `Closes memory:` trailer per `close_commit_memory_trailer.md`.

---

## §4 TS-fidelity gates

- **Arg-shape parity across all wrapper-routed LOW/LOS dispatch sites.** Every iterator/hunt-site call now passes `(1, 1, 1, 0)`. Pinned per-site by the four new `*_LineValidatorArgShape` tests.
- **Wrapper-routing symmetry between `handleLineOfWalk` and `handleLineOfSight`.** Both opcodes route through their respective wrappers. Pinned by the inverted nil-LV test + the preserved `TestHandleLineOfWalk_ArgShape`.
- **MAP_FINDSQUARE LOW/LOS arm parity.** Unchanged by NAI-166 (NAI-165 / NAI-163 already covered MAP_FINDSQUARE; wrapper now correct).
- **Behavioral effect of `destWidth=destLength=1`.** Same as NAI-163 / NAI-165 — the rsmod-ported ray endpoint computation now treats dest as the canonical 1×1 rectangle rather than the degenerate 0×0; mechanical symmetry with the prior fixes.

---

## §5 Risks

- **Modules/world stub plumbing — injection-point fragility.** `s.gamemap.Pathfinder.LineValidator` injection requires that the field is settable post-`gamemap.New`. If the field is unexported or initialized through a constructor that locks it down, the plan-author must adapt (constructor extension, or test-only setter mirroring the `*ForTest` pattern in `test_export_underscore_test_visibility.md`). **Mitigation:** controller pre-flight Reads `pkg/pathfinder/Pathfinder` struct definition before T2 dispatch.
- **Hunt-mode branch enumeration in `TestHuntEntities_LineValidatorArgShape`.** Three branches at `npc_hunt_entities.go:67/136/213` correspond to specific `HuntType` configurations; mis-identifying the selector pushes the test through the wrong branch. **Mitigation:** plan-author Reads each of the three branches in full + reads the corresponding `Npc.ts` huntEntities sections to pin the selector for each branch.
- **`recordingLineValidator` name collision.** A new struct in `modules/world` with the same name as the `pkg/script` test struct is fine (different packages), but a future refactor that merges them risks confusion. **Mitigation:** doc-comment the modules/world copy as a deliberate package-local mirror; cross-reference the pkg/script counterpart.
- **Test passes for wrong reason at hunt sites.** If the plan codifies hunt-test fixtures that route through an earlier filter (range/CheckVars/CheckLoc) before reaching the LV call, the recorded tuple is empty and the test silently passes by no-op. Per `test_passes_for_wrong_reason.md` and `helper_as_oracle_test_anti_pattern.md`. **Mitigation:** each new test asserts `len(rec.calls) == 1` (or the expected positive count) BEFORE inspecting the tuple; plan codifies the assertion.
- **Nil-test inversion misses other nil-LV consumers.** Part B only touches `handleLineOfWalk`. If any other handler relies on the old pessimistic-deny semantics for nil LV at this opcode, a downstream test could flip red. **Mitigation:** controller pre-flight greps `s.LineValidator == nil` across `pkg/script/` and `modules/world/` before T3 dispatch; expects zero matches outside the deleted block.

---

## §6 Cadence & verification

**Three-task subagent-driven TDD** per `runescript_cadence.md` and `execution_mode_default.md`, two-stage review per task, reviewer model = Sonnet per `superpowers_code_reviewer_model.md`:

| Task | Scope | Estimated diff |
|---|---|---|
| T1 | `pkg/script` sweep (player_iterator + npc_iterator) — 4 production lines, 2 new tests, doc-comment fixes | ~50 LOC |
| T2 | `modules/world` sweep (npc_hunt + npc_hunt_entities) + new `recordingLineValidator` stub — 8 production lines, 2 new tests | ~120 LOC |
| T3 | `handleLineOfWalk` refactor + nil-LV test inversion + doc-comment retirement at `handlers_map.go` | ~25 LOC |

Per-task controller pre-flight per `controller_preflight.md`: ~30s grep+Read pass before each implementer dispatch to verify line refs still match HEAD (HEAD will shift between T1→T2→T3 commits).

**Verification commands:**
- After T1: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/... -run 'PlayerIterator|NpcIterator'`
- After T2: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run 'HuntPlayers|HuntEntities'`
- After T3: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/... -run 'HandleLineOfWalk'`
- Full non-regression after T3: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`

**Close-out grep (post-T3):** `rg "NAI-166" pkg/ modules/ cmd/` enumerates remaining references; every match should be a deliberate deviation-tag header introduced by T1–T3, with zero residual "candidate" / "tracked separately" foreshadow text.

**Close commit:** `chore(close): NAI-166 — iterator/hunt LOW+LOS sweep + handleLineOfWalk wrapper-routing` with `Closes memory:` trailer retiring both deviation tags.

---

## §7 No deviations from TS

After NAI-166 lands:
- Every wrapper-routed LOW/LOS dispatch site in the codebase passes the TS-canonical `(srcSize=1, destWidth=1, destLength=1, extraFlag=0)` tuple.
- `handleLineOfWalk` and `handleLineOfSight` both route through their respective wrappers; nil-LV handling is symmetric (pessimistic-allow at both opcodes).

No new deviations from TS introduced. The two tracked tags (`NAI-166-D-LOW-ARG-SHAPE-SWEEP`, `NAI-166-D-LOW-WRAPPER-ROUTING`) are narrative-only and close in the same sub-spec.
