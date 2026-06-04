---
status: brainstorm-approved
date: 2026-05-11
ts_source:
  - LostCityRS/Engine-TS/src/engine/GameMap.ts:425-427 (isLineOfWalk wrapper)
  - LostCityRS/Engine-TS/src/engine/script/handlers/ServerOps.ts:65-82 (LINEOFWALK handler)
  - LostCityRS/Engine-TS/src/engine/script/handlers/ServerOps.ts:262-368 (MAP_FINDSQUARE LineOfWalk arms)
---

# NAI-165 — `isLineOfWalk` wrapper + `handleLineOfWalk` arg-shape fix

**Cadence:** ~5 LOC prod + ~40 LOC test ≈ ~45 LOC. Symmetric mirror of the NAI-163 B1 T0 / T1 unit. Single bundle, single close commit. Per `runescript_cadence` mid-band cadence is overkill; per `compressed_cadence.md` this is slightly over the ≤15-LOC compressed threshold — using a focused separate spec + plan but no subagent dispatch.

**Tech stack:** Go 1.26+ (`go_version.md`).

---

## §1 Symptom / motivation

NAI-163 B1 T0 widened the `isLineOfSight` wrapper at `pkg/script/handlers_map.go:186-191` from `(1, 0, 0, 0)` to `(1, 1, 1, 0)` to match TS `GameMap.ts:429-431` (canonical `rsmod.hasLineOfSight(level, sX, sZ, dX, dZ, 1, 1, 1, 1, 0)`; goscape collapses TS srcWidth+srcHeight=1 into single `srcSize=1` via RayCast at `pkg/pathfinder/routefinder/linevalidator.go:21`). That fix was explicitly framed as scope-narrowed; the symmetric LineOfWalk counterpart was deferred forward (close commit `57e8828` "Out of scope" list: *"isLineOfWalk wrapper widening (mirrors LOS bug at handlers_map.go:175)"*).

At HEAD `0490d45`, two LOW sites still carry the pre-NAI-163 `(1, 0, 0, 0)` arg shape:

| Site | File:line | Caller surface |
|---|---|---|
| `isLineOfWalk` wrapper | `pkg/script/handlers_map.go:175` | `MAP_FINDSQUARE` LineOfWalk arms (`handlers_map.go:117, 147`) |
| `handleLineOfWalk` direct call | `pkg/script/handlers_map.go:423` | `LINEOFWALK` opcode 1006 |

TS canonical for both is the `isLineOfWalk` wrapper at `GameMap.ts:425-427`:
```ts
export function isLineOfWalk(level, srcX, srcZ, destX, destZ) {
    return rsmod.hasLineOfWalk(level, srcX, srcZ, destX, destZ, 1, 1, 1, 1, 0);
}
```
Both `ServerOps.ts:81` (LINEOFWALK handler) and `ServerOps.ts:293, 348` (MAP_FINDSQUARE LOW arms) call this wrapper.

**Behavioral effect of the broken arg shape:** `destWidth=0, destLength=0` collapses the destination rectangle to a degenerate 0×0, which changes how the rsmod-ported ray endpoint computation treats the dest tile. The visible production effect is subtle ray-edge divergence for LOW checks at certain boundary geometries. No user-reported smoke regression cites this directly, but the fix is mechanically symmetric to NAI-163-D-LOS-ARG-SHAPE-FIX and required for cross-call-chain TS fidelity (the `MAP_FINDSQUARE` family's LOW and LOS arms now use different arg shapes, which is its own latent divergence).

---

## §2 Architecture

### §2.1 Two production sites

The fix lands at two call sites that both pass `(1, 0, 0, 0)` to `s.LineValidator.HasLineOfWalk`:

1. **`isLineOfWalk` wrapper** (`pkg/script/handlers_map.go:171-176`) — `MAP_FINDSQUARE` LOW arms inherit the fix transparently (callers at `handlers_map.go:117, 147`).
2. **`handleLineOfWalk` direct call** (`pkg/script/handlers_map.go:423`) — the LINEOFWALK opcode 1006 handler does NOT route through the wrapper (asymmetric vs `handleLineOfSight` at `handlers_map.go:230`, which DOES route through `isLineOfSight`). The direct call is fixed in place; **the asymmetry itself (wrapper-vs-direct + pessimistic-deny-on-nil-vs-pessimistic-allow) is NOT touched by NAI-165** — it's pre-existing, tracked separately (see §6 out-of-scope).

### §2.2 Test infrastructure

The existing `stubLineValidatorArgs` fixture at `handlers_map_test.go:971-988` was built for NAI-163 B1 T0/T1. It records `HasLineOfSight` calls into a `losCalls` slice but `HasLineOfWalk` is a no-recording stub (returns `true`). NAI-165 extends this fixture to also record LineOfWalk calls into a parallel `lowCalls` slice with the same `losCall`-shaped struct (renamed `lineCall` if appropriate, or kept as a fresh `lowCall` type — plan-author resolves at fixture-edit time).

---

## §3 Changes

### §3.1 Production

```go
// pkg/script/handlers_map.go:175 — wrapper
return s.LineValidator.HasLineOfWalk(level, srcX, srcZ, destX, destZ, 1, 1, 1, 0)

// pkg/script/handlers_map.go:423 — handleLineOfWalk direct call
if s.LineValidator.HasLineOfWalk(fromLevel, fromX, fromZ, toX, toZ, 1, 1, 1, 0) {
```

Doc-comment updates at:
- `pkg/script/handlers_map.go:166-170` (the `isLineOfWalk` wrapper preamble) — mirror NAI-163-D-LOS-ARG-SHAPE-FIX framing at lines 178-185; cite `NAI-163-T6 (NAI-165)` provenance.
- `pkg/script/handlers_map.go:389-398` (the `handleLineOfWalk` preamble) — add the same NAI-165-D-LOW-ARG-SHAPE-FIX reference.

### §3.2 Tests

Extend `stubLineValidatorArgs` (`handlers_map_test.go:971-988`) to record `HasLineOfWalk` calls into a `lowCalls []lineCall` slice paralleling `losCalls`. The existing `losCall` struct shape is reused (both opcodes pass the same 9-int tuple).

Add two pins (each mirrors a NAI-163 B1 T0/T1 sibling):

| New test | Mirrors | Pins |
|---|---|---|
| `TestIsLineOfWalkWrapper_PassesTSFaithfulArgShape` | `TestIsLineOfSightWrapper_PassesTSFaithfulArgShape` (line 990) | Wrapper call records `srcSize=1, destWidth=1, destLength=1, extraFlag=0` |
| `TestHandleLineOfWalk_ArgShape` | `TestHandleLineOfSight_ArgShape` (line 1106) | End-to-end opcode dispatch records the same TS-faithful tuple |

The existing NAI-163-B1-T1 LOW handler tests (`TestHandleLineOfWalk_*` family for level-mismatch / F2P / ray-clear / ray-blocked) are unaffected — they use a coarse `stubLineValidator` that doesn't record arg-shapes.

### §3.3 Tracked deviation

**NAI-165-D-LOW-ARG-SHAPE-FIX** — narrative-only documentation deviation; closed in the same sub-spec. Mirrors NAI-163-D-LOS-ARG-SHAPE-FIX. Tag grep-visible at both citation sites: `handlers_map.go:166-170` and `handlers_map.go:389-398`.

---

## §4 TS-fidelity gates

- **Arg shape parity:** both sites call `HasLineOfWalk(..., 1, 1, 1, 0)`. Pinned by both new tests.
- **Pop order, gate order, level-mismatch / F2P short-circuit, nil-LineValidator handling:** unchanged by NAI-165; already pinned by NAI-163-B1-T1 LOW test family.
- **MAP_FINDSQUARE LOW arm parity:** transparently inherits the wrapper fix; no separate pin needed (existing MAP_FINDSQUARE LOW tests already exercise the wrapper through the handler).

---

## §5 Out of scope (forward-routed)

The following are explicitly NOT touched by NAI-165. They are tracked as NAI-166 candidates in `nai_followups.md`:

1. **Iterator/hunt-site LOW+LOS arg-shape sweep.** The same `(1, 0, 0, 0)` divergence exists at:
   - `pkg/script/player_iterator.go:71, 77`
   - `pkg/script/npc_iterator.go:127, 139`
   - `modules/world/npc_hunt_entities.go:68/73/137/142/214/219`
   - `modules/world/npc_hunt.go:163`

   TS source (`ScriptIterators.ts:88, 92, 113, 116, 137, 140, 160, 163, 216, 220, 284, 287, 348, 351`) confirms all these sites flow through the canonical `isLineOfSight`/`isLineOfWalk` wrappers. The sites have stale doc-comments at `npc_iterator.go:133-139` claiming `(1, 0, 0, 0)` "mirrors TS isLineOfWalk wrapper" — those comments need joint correction with the arg-shape fix. Single sub-spec sweep, ~10 sites + comment edits, queued as **NAI-166**.

2. **`handleLineOfWalk` wrapper routing + nil-LineValidator semantics asymmetry.** `handleLineOfSight` (line 230) routes through the `isLineOfSight` wrapper → pessimistic-ALLOW on nil validator (push 1). `handleLineOfWalk` (line 419-421) has an explicit nil-guard → pessimistic-DENY (push 0). Both opcodes should share the wrapper-routing pattern for TS fidelity (both TS handlers go through their respective wrappers). Pre-existing divergence; not introduced by NAI-165. Routes to NAI-166 or NAI-167.

---

## §6 Cadence & verification

- **Spec & plan:** this doc + a focused plan doc generated by `writing-plans` skill.
- **Implementation:** single combined commit (no subagent dispatch needed; ≤45 LOC). TDD: red test first (extend fixture + add two pins; expect them to fail with current `(1, 0, 0, 0)` shape), then flip the two production sites to `(1, 1, 1, 0)`, then green.
- **Verification:** `go test ./pkg/script/... -run 'TestIsLineOfWalkWrapper|TestHandleLineOfWalk_ArgShape'` for the new pins; full `go test ./...` for non-regression. Verify the existing NAI-163-B1-T1 LOW handler family still passes.
- **Close commit:** standard `chore(close): NAI-165 — isLineOfWalk arg-shape fix` with `Closes memory:` trailer if any new memory entry is needed (likely none — the pattern is already covered by `audit_full_method_against_ts.md` and `dispatch_order_audit_blind_spot.md`).

---

## §7 Risks

- **Latent test breakage at iterator/hunt sites.** When NAI-166 lands the iterator/hunt sweep, tests that currently pass `(1, 0, 0, 0)` to mock LineValidators (e.g. `linevalidator_test.go:167-236`) need joint review. NAI-165 itself only touches `handlers_map.go` + `handlers_map_test.go` — no cross-package test impact expected. **Mitigation:** controller pre-flight grep + Read at plan-author time.
- **Pop-order / argument-name confusion in handleLineOfWalk fixup.** `handleLineOfWalk`'s direct call site is one logically-equivalent line change; mentally compile to confirm the parameter names match (`fromLevel, fromX, fromZ, toX, toZ`). **Mitigation:** the new `TestHandleLineOfWalk_ArgShape` pin catches mis-application end-to-end.

---

## §8 No deviations from TS

After the fix lands, both LOW sites use the TS-canonical wrapper arg shape. No deviations introduced. The single tracked deviation (`NAI-165-D-LOW-ARG-SHAPE-FIX`) is narrative-only — it documents that the goscape-side TS-fidelity gap was closed in NAI-165.
