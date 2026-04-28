# NAI-41 — `Player.SetInteraction` face-entity TS-fidelity

**Status:** spec
**Date:** 2026-04-27
**HEAD at spec-write:** `39e5d71` (post-NAI-40)
**Tech Stack:** Go 1.26+

## 1. Purpose

Port the face-entity dispatch of TS `PathingEntity.setInteraction` (`PathingEntity.ts:530-541`) into `Player.SetInteraction`. Closes two divergences in one sub-spec:

- `NAI-40-D-PLAYER-NO-FACEENTITY-ON-OPCLICK` — `Player.faceEntity` never updated when target is `*Player`. Tagged at `modules/world/interaction.go:101-112` by NAI-40 polish commit `39e5d71`.
- Pre-existing *Player→*Npc timing divergence — `Player.faceEntity` updated at *contact distance* (`processInteraction`, `interaction.go:99`) instead of at *anchor time* (`SetInteraction`). Pre-NAI-40 convention; never numbered before this sub-spec.

The closure is a structural copy of the same dispatch already present in `Npc.SetInteraction` (`modules/world/npc_interaction.go:651-666`) — the in-codebase TS-faithful template.

Behavioral effect after closure: when a player op-clicks any other player or NPC, the clicker's `player_facingmask` updates on the click tick (matching TS), not on the tick they reach contact range (or never, for *Player targets).

## 2. TS Reference

`/home/owner/Code/github.com/LostCityRS/Engine-TS/src/engine/entity/PathingEntity.ts:510-548`

```ts
setInteraction(interaction: Interaction, target: Entity, op: TargetOp, com?: number): boolean {
    if (!target.isValid(this instanceof Player ? this.hash64 : undefined)) {
        return false;                                // L511 — IsValid pre-check (DEFERRED, see §6)
    }
    this.target = target;
    this.targetOp = op;
    this.apRange = 10;
    this.apRangeCalled = false;
    this.targetSubject.com = com ? com : -1;
    if (target instanceof Npc || target instanceof Loc || target instanceof Obj) {
        this.targetSubject.type = target.type;
    } else {
        this.targetSubject.type = -1;
    }

    this.focus(...);                                  // L528 — focus() (DEFERRED, see §6)

    if (target instanceof Player) {                   // L530-535 — PORTED
        const playerSlot: number = target.slot + 32768;
        if (this.faceEntity !== playerSlot) {
            this.faceEntity = playerSlot;
            this.masks |= this.entitymask;
        }
    } else if (target instanceof Npc) {               // L536-541 — PORTED
        const nid: number = target.nid;
        if (this.faceEntity !== nid) {
            this.faceEntity = nid;
            this.masks |= this.entitymask;
        }
    } else {                                          // L542-545 — DEFERRED, see §6
        this.targetX = CoordGrid.fine(target.x, target.width);
        this.targetZ = CoordGrid.fine(target.z, target.length);
    }
    return true;                                      // L547 — return-bool (DEFERRED, see §6)
}
```

In-codebase TS-faithful template (already shipped, used as the structural reference): `Npc.SetInteraction` at `modules/world/npc_interaction.go:607-669`.

## 3. Goscape HEAD State

`Player.SetInteraction` at `modules/world/interaction.go:43-57`:

```go
func (p *Player) SetInteraction(kind InteractionKind, target entity, op, com int) {
    p.target = target
    p.targetOp = op
    p.targetSubject.com = com
    p.interactionKind = kind
    p.apRange = 10
    p.apRangeCalled = false
    p.interacted = false
    p.repathed = false
    p.interactionFired = false
}
```

`processInteraction` contact branch at `modules/world/interaction.go:95-117`:

```go
if inOperableDistance(p.x, p.z, tx, tz) {
    // Contact range — fire OP. Matches TS Player.ts:1123-1135 ...
    if npc, ok := p.target.(*Npc); ok {
        p.SetFaceEntity(npc.nid)                      // <-- contact-time write (DELETED post-port)
    }
    // DEVIATION NAI-40-D-PLAYER-NO-FACEENTITY-ON-OPCLICK: ... <-- comment block (DELETED post-port)
    p.interacted = true
    if !p.interactionFired {
        tryFireOpTrigger(p)
    }
    return
}
```

Relevant invariants (all confirmed at HEAD):

- `Player.entitymask` is set to `rsbuf.MaskFaceEntity` in `NewPlayer` (`player.go:411`), matching `Npc.entitymask` shape — so the `p.masks |= p.entitymask` idiom carries the correct bit.
- `Player.slot` is `int` (`player.go:63`); `Npc.nid` is `int`; both fit the 2-byte unsigned wire encoding `P2(uint16(p.FaceEntity()))` at `pkg/rsbuf/mask_payload.go:79`. The `slot + 32768` magic encodes "this is a player" on the client, no encoder change needed.
- Player has **no** `targetX`/`targetZ` fields, **no** `faceAngleX/Z`, **no** `focus()` method. These belong to the deferred fine-coord / NAI-34-D3 sub-spec.
- 9 production call sites for `Player.SetInteraction` (handlers `oploc`/`opnpc`/`op_player`); all use the void return.

## 4. Approach (Auto-mode default: true-to-TS minimal — Option 1)

### 4.1 In scope

1. Extend `Player.SetInteraction` with a type-switch on `target.(type)` after the existing field assignments:
   - `*Player` → `slot := t.slot + 32768; if p.faceEntity != slot { p.faceEntity = slot; p.masks |= p.entitymask }`
   - `*Npc` → `if p.faceEntity != t.nid { p.faceEntity = t.nid; p.masks |= p.entitymask }`
   - `default` → no-op + `// DEVIATION NAI-41-D-PLAYER-NO-LOCOBJ-TARGETXZ: ...` comment block (see §6).
2. Delete the contact-time `SetFaceEntity(npc.nid)` block in `processInteraction` (`interaction.go:96-100`) — anchor-time write makes it redundant.
3. Delete the `NAI-40-D-PLAYER-NO-FACEENTITY-ON-OPCLICK` deviation-comment block (`interaction.go:101-112`); the deviation closes.
4. Inline the TS idempotency check (`if faceEntity !== X`) — matches the `Npc.SetInteraction` template at `npc_interaction.go:651-666` and avoids spurious `MaskFaceEntity` re-emission on re-clicks of the same target.

### 4.2 Out of scope (carry-forward residuals tracked in §6)

- TS L511 `IsValid()` pre-check (return `false` early when target is not valid). 9 call-site updates required for the bool return; separate sub-spec.
- TS L528 `focus()` call (fine-coord face-angle). Already deferred under NAI-34-D3 (`pathing-entity-focus-and-step-tracking`).
- TS L542-545 `targetX/Z` fine-coord cache for *Loc/*Obj targets. No Player consumer at HEAD; dead-API per `dead_api_polish.md`. Track as `NAI-41-D-PLAYER-NO-LOCOBJ-TARGETXZ`; bundle closure with the focus/step-tracking sub-spec.
- TS L547 return-bool. Coupled to L511 (the IsValid early-return is the only false-return path). Same separate sub-spec.
- TS L520-526 `targetSubject.type` snapshot. Player's `targetSubject` shape (`{typ, x, z, level, com int}`) and current write convention (handler-side post-`SetInteraction`) is a pre-existing pattern unrelated to face-entity; not in this scope.

## 5. Components & Data Flow

### 5.1 New `Player.SetInteraction` body (post-port)

```go
func (p *Player) SetInteraction(kind InteractionKind, target entity, op, com int) {
    p.target = target
    p.targetOp = op
    p.targetSubject.com = com
    p.interactionKind = kind
    p.apRange = 10
    p.apRangeCalled = false
    p.interacted = false
    p.repathed = false
    p.interactionFired = false

    // Mirrors TS PathingEntity.setInteraction (PathingEntity.ts:530-545)
    // and Npc.SetInteraction (npc_interaction.go:651-666). The Loc/Obj
    // branch is intentionally not ported (see DEVIATION below).
    switch t := target.(type) {
    case *Player:
        slot := t.slot + 32768
        if p.faceEntity != slot {
            p.faceEntity = slot
            p.masks |= p.entitymask
        }
    case *Npc:
        if p.faceEntity != t.nid {
            p.faceEntity = t.nid
            p.masks |= p.entitymask
        }
    default:
        // DEVIATION NAI-41-D-PLAYER-NO-LOCOBJ-TARGETXZ: TS L542-545 sets
        // targetX = CoordGrid.fine(target.x, target.width) and targetZ
        // analogously for *Loc/*Obj targets. Player has no targetX/Z
        // fields and no consumer reads them; deferred to the focus/
        // step-tracking sub-spec that closes NAI-34-D3 (which already
        // touches Player fine-coord infra).
    }
}
```

### 5.2 New `processInteraction` contact branch (post-port)

```go
if inOperableDistance(p.x, p.z, tx, tz) {
    // Contact range — fire OP. Matches TS Player.ts:1123-1135 (OP
    // checked before AP at contact). faceEntity was set at
    // SetInteraction-time per NAI-41; no contact-time write needed.
    p.interacted = true
    if !p.interactionFired {
        tryFireOpTrigger(p)
    }
    return
}
```

### 5.3 Wire-format invariant

The `MaskFaceEntity` bit + 2-byte `slot+32768` / `nid` payload reach the client identically; only the timing changes (one to many ticks earlier).

The TS idempotency check (`if faceEntity !== X`) means re-clicking the same target on consecutive ticks no longer re-emits `MaskFaceEntity`. Pre-port behavior (per-contact write via `SetFaceEntity`, which always re-emits) was strictly more verbose than TS; the new shape is exactly TS-faithful.

## 6. Tracked Deviations

### 6.1 Closes (this sub-spec)

- **`NAI-40-D-PLAYER-NO-FACEENTITY-ON-OPCLICK`** — closed: Player target now writes `faceEntity = slot + 32768` at SetInteraction time.
- **Pre-existing *Player→*Npc timing** — closed: Npc target now writes `faceEntity = nid` at SetInteraction time, not at contact. Never numbered before; closure noted in NAI-41 close commit.

### 6.2 New (this sub-spec introduces)

- **`NAI-41-D-PLAYER-NO-LOCOBJ-TARGETXZ`** — *Loc/*Obj target branch (TS L542-545) not ported. Player has no `targetX/Z` fields and no AP/OP consumer reads them. Tag site: `interaction.go` `default` arm of the type-switch in `SetInteraction`. Closure: bundle with the focus/step-tracking sub-spec that closes NAI-34-D3 (the natural home — both deviations need fine-coord infra).

### 6.3 Carryovers (NOT closed by this sub-spec)

- `NAI-34-D3` (both entities, `focus()`) — still deferred.
- `NAI-34-D4` (lastStepX/Z), `NAI-34-D5-NPC` — still deferred.
- TS L511 `IsValid()` pre-check + L547 return-bool for `Player.SetInteraction` — separate sub-spec; needs 9 call-site bool-return migrations.

## 7. Testing

New tests in `modules/world/interaction_test.go` (placement: alongside existing `SetInteraction` / `processInteraction` tests):

1. **`TestSetInteractionPlayerTargetSetsFaceEntity`** — create two test players; `p.SetInteraction(InteractionEngine, other, 1, -1)`; assert `p.faceEntity == other.slot + 32768` and `p.masks & MaskFaceEntity != 0`. Pins the *Player branch.
2. **`TestSetInteractionNpcTargetSetsFaceEntity`** — `p.SetInteraction(InteractionEngine, npc, 1, -1)`; assert `p.faceEntity == npc.nid` and `p.masks & MaskFaceEntity != 0`. Pins the *Npc branch.
3. **`TestSetInteractionLocTargetDoesNotSetFaceEntity`** — `p.SetInteraction(InteractionEngine, loc, 1, -1)`; assert `p.faceEntity == -1` (default from `NewPlayer`) and `p.masks & MaskFaceEntity == 0`. Pins the deferred-default branch (proves the deviation is intentional, not a partial port).
4. **`TestSetInteractionFaceEntityIdempotent`** — `p.SetInteraction(...)` twice with same target back-to-back, with `p.masks = 0` reset between calls; assert second call leaves `p.masks & MaskFaceEntity == 0`. Pins the TS `if (faceEntity !== X)` check.

Existing tests (relocate one assertion):

- **`TestProcessInteractionInRangeFacesTarget`** (`interaction_test.go:124-149`) — currently asserts `p.faceEntity == npc.nid` and `p.masks & MaskFaceEntity != 0` after `processInteraction`. Both assertions are now covered by new test #2 (`TestSetInteractionNpcTargetSetsFaceEntity`). **Required change:** delete the two `faceEntity`/`MaskFaceEntity` assertions (lines 143-148) from `TestProcessInteractionInRangeFacesTarget`. The test's remaining role is narrower: pin that contact-distance triggers `p.interacted == true` (and that the OP trigger fires — already implicit via `received`/`flushWrite`). Rename to `TestProcessInteractionInRangeMarksInteracted` if the implementer judges the rename clarifies intent; either name is acceptable, the assertion delete is required.

Test fixtures: `newTestPlayer(t)` returns a `*Player` with `faceEntity == -1` and `entitymask == rsbuf.MaskFaceEntity` (from `NewPlayer`). For the *Player target test, instantiate a second `*Player` via `newTestPlayer(t)` and ignore its `cc` connection. For the *Npc target test, `makeInteractionNpc(t, s, ...)` provides the canonical fixture (already used at `interaction_test.go:126`).

## 8. Sizing / Cadence

- ~15 net production LOC (extension of `SetInteraction` + 1 deletion in `processInteraction` + 1 deviation-comment delete).
- ~50 net test LOC (4 new tests + 1 relocated assertion).
- Single TDD task likely (one feat commit + optional polish).
- Compressed-cadence eligible by LOC (`compressed_cadence.md`: ≤ ~15 LOC) — but with 4 new tests + a meaningful behavioral move + a new tracked deviation, full cadence (spec → plan → subagent-driven → two-stage review) is the right call. The marginal cost of a plan doc is small relative to the discovery value of dispatching this with full review.

## 9. Risk / Verification

- **Risk: existing tests pinning the contact-time write.** Grepped at spec-write: only `TestProcessInteractionInRangeFacesTarget` asserts on `p.faceEntity` post-`processInteraction`; relocation handles it. No other test depends on the old timing.
- **Risk: `processInteraction` callers expecting the write at contact tick.** No production reader of `p.faceEntity` runs between `SetInteraction` and `processInteraction` within the same tick — the player_info encoder runs at the *end* of the tick after `processInteraction`, so the wire output is unchanged for the contact-range case. For the (newly-correct) approach-range and out-of-range cases, the wire now includes `MaskFaceEntity` on the click tick; this is the TS-faithful behavior.
- **Risk: `Player.entitymask` is zero by accident in some path.** Confirmed at HEAD: `entitymask: rsbuf.MaskFaceEntity` in `NewPlayer` (`player.go:411`), the only `*Player` constructor in `modules/world/`. No code mutates it. Same shape as `Npc.entitymask`.
- **Verification protocol** (per `verify_implementer_claims.md`): run `go test ./modules/world/... ./pkg/rsbuf/...` before claiming green; cross-check `git show <SHA> --stat` matches stated scope (per `implementer_commit_content_verify.md`).

## 10. References

- `runescript_cadence.md` — sub-spec workflow.
- `dead_api_polish.md` — drives the *Loc/*Obj targetX/Z deferral.
- `plan_grep_helper_patterns.md` — applied to the inline-vs-helper choice (kept inline for TS-fidelity + Npc.SetInteraction symmetry).
- `verify_implementer_claims.md`, `implementer_commit_content_verify.md` — verification protocol for the implementer.
- TS source: `LostCityRS/Engine-TS/src/engine/entity/PathingEntity.ts:510-548`.
- In-codebase template: `Npc.SetInteraction` at `modules/world/npc_interaction.go:607-669`.
