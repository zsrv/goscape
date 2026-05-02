# NAI-67 — focus instant-wire port + Player.SetInteraction driver + Npc.unfocus port

**Status:** spec.
**Author:** zsrv (with Claude).
**Date:** 2026-05-02.
**Predecessor:** NAI-66 (closed at `2b903f4`).
**Net deviation tally:** 13 (post-NAI-66) → -1 close, +1 open → **13** at NAI-67 close.
**Tech stack:** Go 1.26+.

---

## 1. Scope

NAI-67 ports the remaining `(*Player).focus` / `(*Npc).focus` `instant=true` wire
semantics, surfaces and closes a previously-untagged divergence in
`(*Player).SetInteraction` (TS PathingEntity.ts:528 driver call missing),
and ports `(*Npc).unfocus` + wires it into `resetEntityForRespawn`.

`(*Player).unfocus` is intentionally deferred — no consumer at HEAD per
goscape's "future combat sub-spec" boundary noted at
`modules/world/player_masks.go:90`.

**Two bundles:**

- **B1 — focus-family wire + driver**: closes
  `NAI-65-D-FOCUS-INSTANT-WIRE`. Surfaces and closes a previously
  untagged divergence (Player.SetInteraction missing focus()) in the
  same bundle, so no new tag is opened.
- **B2 — Npc.unfocus port + respawn wire**: opens
  `NAI-67-D-PLAYER-UNFOCUS-DEFERRED`.

## 2. TS source mapping

| Surface | TS source | Goscape site | Bundle |
|---|---|---|---|
| `focus(fineX, fineZ, client=true)` wire branch | `Engine-TS/src/engine/entity/PathingEntity.ts:321-333` | `modules/world/player_script.go:415-419`; `modules/world/npc_interaction.go:706-710` | B1 |
| `setInteraction` calls `focus(...)` | `Engine-TS/src/engine/entity/PathingEntity.ts:528` | `modules/world/interaction.go:79-100` (currently absent) | B1 |
| `unfocus()` body | `Engine-TS/src/engine/entity/PathingEntity.ts:338-341` | `modules/world/npc_interaction.go` (new) | B2 |
| `Npc.resetEntity(true)` calls `unfocus()` | `Engine-TS/src/engine/entity/Npc.ts:280-296` | `modules/world/npc_registry.go:115` (`resetEntityForRespawn`) | B2 |
| `Player.resetEntity(true)` calls `unfocus()` | `Engine-TS/src/engine/entity/Player.ts:454-457` | none — Player death/respawn flow not ported | B2 (deferred) |

## 3. Bundle 1 — focus-family wire + driver

### 3.1 `(*Player).focus` wire branch

`modules/world/player_script.go:415-419`. Replace `_ = instant` with the
TS:327-331 wire branch:

```go
func (p *Player) focus(fx, fz int, instant bool) {
    p.faceAngleX = fx
    p.faceAngleZ = fz
    if instant {
        p.faceSquareX = fx
        p.faceSquareZ = fz
        p.masks |= rsbuf.MaskFaceCoord
    }
}
```

Drop the `DEVIATION NAI-65-D-FOCUS-INSTANT-WIRE` doc-comment block at
L409-414. Update the helper's doc-comment to cite TS PathingEntity.ts:321-333
and note the input-coord-frame distinction from `(*Player).FaceSquare`
(focus() takes raw fine coords; FaceSquare takes absolute and applies
`*2+1`).

### 3.2 `(*Npc).focus` wire branch

`modules/world/npc_interaction.go:706-710`. Symmetric port using
`rsbuf.NpcMaskFaceCoord`:

```go
func (n *Npc) focus(fx, fz int, instant bool) {
    n.faceAngleX = fx
    n.faceAngleZ = fz
    if instant {
        n.faceSquareX = fx
        n.faceSquareZ = fz
        n.masks |= rsbuf.NpcMaskFaceCoord
    }
}
```

Drop the matching `DEVIATION` block at L701-705.

### 3.3 `(*Player).SetInteraction` TS:528 driver port

`modules/world/interaction.go:61-101`. Insert the missing `p.focus(...)`
call BEFORE the `switch t := target.(type)` entity-dispatch, mirroring
TS PathingEntity.ts:528:

```go
tx, tz, _ := target.Coords()
tw, tl := targetWidthLength(target)
fx := coordgrid.Fine(tx, tw)
fz := coordgrid.Fine(tz, tl)
isNonPathing := false
switch target.(type) {
case *entitypkg.Loc, *entitypkg.Obj:
    isNonPathing = true
}
p.focus(fx, fz, isNonPathing && kind == InteractionEngine)
```

This mirrors the existing pattern at
`modules/world/npc_interaction.go:660-664` (Npc-side SetInteraction).
Adds an `entitypkg "github.com/zsrv/goscape/pkg/entity"` import to
`interaction.go` (the file currently does not import the entity
package — npc_interaction.go does).

Reuse `fx, fz` in the existing default-arm `targetX/Z` write (replaces
the recomputation at L96-99). The default-arm `targetX/Z` cache (NAI-66
closure) stays — `(*Player).reorient` still consumes it.

Update the function's doc-comment to cite TS PathingEntity.ts:528 and
explicitly enumerate the four (kind × target-shape) wire-write cases
exercised by the new tests.

### 3.4 Tests (B1)

Existing `instant=*` tests flip negative-pin → positive-pin:

- `modules/world/player_script_test.go:1140-1180` — `instant=false`
  leaves faceSquare/mask alone; `instant=true` writes
  `faceSquareX==fx && faceSquareZ==fz && masks & rsbuf.MaskFaceCoord != 0`.
- `modules/world/npc_interaction_test.go:900-925` — symmetric.

New `(*Player).SetInteraction` driver tests in
`modules/world/interaction_test.go`:

- `TestSetInteractionNpcTargetWritesFaceAngleNoFaceSquare` — Npc
  target ⇒ faceAngle written, faceSquare unchanged, MaskFaceCoord NOT
  set (instant=false because Npc is pathing).
- `TestSetInteractionPlayerTargetWritesFaceAngleNoFaceSquare` —
  symmetric for Player target.
- `TestSetInteractionLocEngineWritesFaceSquareAndMask` — Loc target,
  `kind=InteractionEngine` ⇒ faceSquare+mask written.
- `TestSetInteractionLocScriptDoesNotWriteFaceSquare` — Loc target,
  `kind=InteractionScript` ⇒ faceSquare unchanged (instant=false).
- `TestSetInteractionObjEngineWritesFaceSquareAndMask` — Obj target,
  `kind=InteractionEngine` ⇒ faceSquare+mask written.

The NAI-66 default-arm `targetX/Z` sizing pins
(`TestSetInteractionLocTargetWritesTargetXZ`,
`TestSetInteractionObjTargetWritesTargetXZ`) stay — verify the new
code preserves them.

Per `ts_asymmetry_dual_pin.md`: every test pins both the presence
(instant=true ⇒ wire writes) AND the conspicuous absence (instant=false
⇒ no wire writes); upstream "fix" of the persistent-faceSquare goscape
divergence (see § 6) would escalate the absence-pin.

### 3.5 LOC budget (B1)

~12 production LOC + ~80 test LOC across 6 files
(`player_script.go`, `npc_interaction.go`, `interaction.go`,
`player_script_test.go`, `npc_interaction_test.go`,
`interaction_test.go`).

## 4. Bundle 2 — Npc.unfocus port + respawn wire

### 4.1 `(*Npc).unfocus`

New method in `modules/world/npc_interaction.go` next to `focus()`:

```go
// unfocus restores the default-south face-angle. Mirrors TS
// PathingEntity.unfocus (Engine-TS/src/engine/entity/PathingEntity.ts:338-341).
// Called from resetEntityForRespawn (TS Npc.resetEntity respawn=true at
// Npc.ts:284). No mask emit — TS unfocus also leaves coordmask alone.
func (n *Npc) unfocus() {
    n.faceAngleX = coordgrid.Fine(n.x, n.size)
    n.faceAngleZ = coordgrid.Fine(n.z-1, n.size)
}
```

### 4.2 Wire into `resetEntityForRespawn`

`modules/world/npc_registry.go:115`. Insert `n.unfocus()` at the top of
the function, before the typeId/uid restore block. `unfocus()` reads
`n.x/n.z/n.size`, none of which are mutated by the function body.

### 4.3 `(*Player).unfocus` deferral

`(*Player).unfocus` is NOT ported. Per `dead_api_polish.md`: don't ship
helpers with zero consumers. The TS caller (`Player.resetEntity(true)`
at Player.ts:454-457) requires Player death/respawn flow, which goscape
does not have at HEAD (`modules/world/player_masks.go:90` defers them
to a future combat sub-spec).

This deferral opens `NAI-67-D-PLAYER-UNFOCUS-DEFERRED` (tracked in
`nai_followups.md`). Closure: future Player respawn / death sub-spec
ports `(*Player).unfocus` and wires it into the to-be-written respawn
path.

### 4.4 Tests (B2)

In `modules/world/npc_interaction_test.go`:

- `TestNpcUnfocusWritesDefaultSouthFaceAngle`:
  - Position Npc at known absolute (x, z) with size=1; assert
    `faceAngleX == coordgrid.Fine(x, 1)` and
    `faceAngleZ == coordgrid.Fine(z-1, 1)`.
  - Sub-pin: same assertions at size=2.
  - Pre-state: set `faceAngleX/Z` to nonzero distinguishable values;
    assert they're overwritten.
  - **Conspicuous-absence pin** (per
    `ts_asymmetry_dual_pin.md`): assert
    `n.masks & rsbuf.NpcMaskFaceCoord == 0` — TS unfocus does not OR
    the mask; escalates if upstream changes that.

In `modules/world/npc_registry_test.go`:

- `TestResetEntityForRespawnInvokesUnfocus`:
  - Set `n.faceAngleX/Z` to a non-default sentinel before respawn.
  - Call `resetEntityForRespawn`.
  - Assert post-state matches `coordgrid.Fine(n.x, n.size)` /
    `coordgrid.Fine(n.z-1, n.size)`.

### 4.5 LOC budget (B2)

~5 production LOC + ~40 test LOC across 4 files (`npc_interaction.go`,
`npc_registry.go`, `npc_interaction_test.go`, `npc_registry_test.go`).

## 5. Test-coverage crosscheck

Per `plan_test_coverage_crosscheck.md`:

| Branch | Bundle | Test |
|---|---|---|
| `(*Player).focus(_, _, false)` leaves faceSquare/mask alone | B1 | flipped existing |
| `(*Player).focus(_, _, true)` writes faceSquare + mask | B1 | flipped existing |
| `(*Npc).focus(_, _, false)` leaves faceSquare/mask alone | B1 | flipped existing |
| `(*Npc).focus(_, _, true)` writes faceSquare + mask | B1 | flipped existing |
| `Player.SetInteraction` Npc target ⇒ instant=false | B1 | new |
| `Player.SetInteraction` Player target ⇒ instant=false | B1 | new |
| `Player.SetInteraction` Loc + Engine ⇒ instant=true | B1 | new |
| `Player.SetInteraction` Loc + Script ⇒ instant=false | B1 | new |
| `Player.SetInteraction` Obj + Engine ⇒ instant=true | B1 | new |
| Default-arm `targetX/Z` sizing preserved (NAI-66) | B1 | retained |
| `(*Npc).unfocus` writes default-south at size=1 | B2 | new |
| `(*Npc).unfocus` writes default-south at size=2 | B2 | sub-pin |
| `(*Npc).unfocus` does NOT OR mask | B2 | absence-pin |
| `resetEntityForRespawn` invokes `unfocus` | B2 | new |

## 6. TS-fidelity gates and known orthogonal divergences

Per `true_to_ts_gate.md`: every behavioural change cites TS source
line numbers (PathingEntity.ts:321-333, :338-341, :528; Npc.ts:280-296;
Player.ts:454-457).

**Coord-frame note.** Bundle 1 wire writes use raw fine coords (no
`*2+1`) per TS:329-330 — distinct from `(*Player).FaceSquare`
(`modules/world/player_masks.go:38-43`) which doubles+1 because its
input is absolute. Doc-comment on `focus()` notes this explicitly so
future readers don't mis-port a future call site that hands focus()
absolute coords.

**Known orthogonal divergence preserved.** Goscape promoted
`faceSquareX/Z` to **persistent** across `ResetMasks`
(`modules/world/player_masks.go:51` + `modules/world/npc_masks.go:180-208`).
TS resets to -1 at end of tick (PathingEntity.ts:608-609). NAI-67
leaves that divergence in place — it predates this sub-spec and is
intentional goscape design. The conspicuous-absence pin in B1's flipped
tests (instant=false ⇒ no wire writes) escalates if anyone tries to
"fix" the persistent-faceSquare without rewriting the absence
expectations.

## 7. Implementation-side risks (memory-derived)

Apply at plan-write time and pre-flight before each implementer dispatch:

- `controller_preflight.md` — pre-flight grep at HEAD against doc-comment
  line numbers cited in this spec (`player_script.go:409-414`,
  `npc_interaction.go:701-705`, `player_script_test.go:1140-1180`,
  `npc_interaction_test.go:900-925`). Re-confirm before plan dispatch.
- `enumerate_all_sites.md` — pre-flight: enumerate all `focus(` call
  sites in `modules/world/`. Current count: 11 production callers
  across `npc_interaction.go` (6), `movement.go` (3),
  `player_script.go` (3), `npc_script.go` (1). All currently pass
  `instant=false` except `npc_interaction.go:665`. Bundle 1 changes the
  helper body — every existing caller's faceSquare-untouched assertion
  (where it exists) must continue to hold for `instant=false` paths.
- `plan_grep_helper_patterns.md` — `targetWidthLength` already in
  `npc_interaction.go:690-695`; reuse, don't inline.
- `plan_var_name_collision.md` — Bundle 1's new `(*Player).SetInteraction`
  block introduces `tx, tz, fx, fz`. Confirm at plan-write that none
  collide with the existing scope.
- `dead_api_polish.md` — Bundle 2 ships `(*Npc).unfocus` only. Do NOT
  add `(*Player).unfocus` "for symmetry."
- `audit_full_method_against_ts.md` — when porting Player.SetInteraction's
  TS:528 line, audit the entire TS `setInteraction` body line-by-line; the
  only added obligation is the focus() call (the rest — apRange reset,
  faceEntity, targetX/Z — was ported earlier).
- `ts_base_class_read_for_inherited_behavior.md` — TS NPC respawn
  calls `unfocus()` from `Npc.ts:284` (leaf override), not the base;
  goscape's matching site is `resetEntityForRespawn`, the goscape-shape
  equivalent of TS `Npc.resetEntity(true)`'s respawn arm.
- `plan_test_coverage_crosscheck.md` — see § 5.
- `ts_asymmetry_dual_pin.md` — applied in § 3.4 and § 4.4.

## 8. Cadence

Per `runescript_cadence.md` + `execution_mode_default.md`: full
cadence, subagent-driven TDD, two-stage review per bundle. Spec → plan
(two bundles) → /clear gate → subagent-driven implementation → close
commit (with `Closes memory:` trailer per
`close_commit_memory_trailer.md`).

Per `superpowers_clear_between_spec_and_impl.md`: spec → plan → /clear
before implementation.

Per `superpowers_code_reviewer_model.md`: review-stage subagents run
on Sonnet (or smaller).

## 9. Smoke posture

No end-to-end smoke required. The wire-protocol writes are exercised
via Player/Npc info encoder unit tests; `tick.go:397` already reads
`faceSquareX/Z`; the renderer `MaskFaceCoord` branch is unchanged. If
a smoke would surface here, it would be "Loc-clicked engine
interaction faces the loc immediately" — observable but not a
regression risk relative to HEAD because the faceSquare path is
already wired through the FaceSquare opcode.

## 10. Carry-forwards after NAI-67 close

- `NAI-67-D-PLAYER-UNFOCUS-DEFERRED` (new) — Player.unfocus port,
  blocked on Player respawn / death sub-spec.
- `NAI-34-D4-NPC` + `NAI-34-D5-NPC` — permanent dead-API skip
  (unchanged).
- `NAI-35-T3-D1` op[1] operability gate audit (unchanged).
- All NAI-65 / NAI-66 carry-forwards minus `NAI-65-D-FOCUS-INSTANT-WIRE`
  (closed by B1).
- All other deferred carry-forwards (NAI-37 / NAI-44 walktrigger,
  NAI-40-SB1/SB2/SB4, NAI-44-D-IMMEDIATE-POP-VS-NEXTTARGET,
  NAI-44-D-CANACCESS-NO-STUN-CHECK, NAI-59-D-MODALTUTORIAL-NO-PRODUCER)
  unchanged.
