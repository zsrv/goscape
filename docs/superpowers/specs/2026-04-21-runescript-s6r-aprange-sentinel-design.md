# S6r — `apRange == -1` Sentinel (closes S6l-D1) Design

> **Sub-spec context:** Eighteenth runescript sub-spec. Closes S6l-D1. Tiny behavior-preserving optimization in `fireApTriggerLoc`: when the AP-script lookup returns nil, cache that fact via `p.apRange = -1` so `inApproachDistance` short-circuits false on subsequent ticks, avoiding redundant provider lookups.

> **TS-faithfulness gate:** This matches TS's apRange=-1 sentinel semantics. **Zero new deviations, closes S6l-D1.**

> **Scope:** Single-task sub-spec. ~15 LOC impl + 3 tests.

## 1. Goal

When `fireApTriggerLoc` finds no registered AP script for the current (trigger, loc.Type, category) triple, cache the "no-AP" result on the player via `p.apRange = -1` so that:
- `inApproachDistance(..., apRange=-1)` short-circuits `false` on subsequent ticks (already handled by `apRange <= 0` guard at `interaction.go:145`)
- `processInteraction` falls through the AP branch straight to path-to-target or contact (`inOperableDistance`)
- Provider lookup happens at most once per Loc interaction instead of every tick

Observable gain: negligible at small player counts; matches TS behavior faithfully.

## 2. Architecture

One-line insert + one-line deviation comment update. NPC path untouched (it already `ClearInteraction`s on no-AP-script, so the sentinel has no analog there).

### 2.1 Why Loc but not NPC

| Branch | `sf == nil` behavior | Sentinel needed? |
|---|---|---|
| `fireApTriggerLoc` | Anchor stays; next tick re-enters AP path | **Yes** — sentinel short-circuits re-lookup |
| `fireApTriggerNpc` | `ClearInteraction()` — no next tick AP re-entry | **No** — interaction ends |

### 2.2 Fields already correct — no changes needed

- `inApproachDistance(apRange)` already guards `if apRange <= 0 { return false }` at `interaction.go:145`. `-1` passes this gate.
- `effectiveApRange(p)` returns `p.apRange` for non-Npc targets (`interaction.go:186`). For Loc, the `-1` flows through.
- `SetInteraction` resets `p.apRange = 10` (`interaction.go:47`). Fresh interaction → fresh range.
- `ClearInteraction` resets `p.apRange = 10` (`interaction.go:58`). Ensures no stale sentinel after abort.

### 2.3 Why this doesn't break other call sites

The only other consumer of `p.apRange` is `effectiveApRange(p)` → `inApproachDistance(...)`. Both treat `<= 0` as "never in approach distance." The sentinel is semantically equivalent to "approach range has no effect" — exactly what we want.

Scripts reading `p_aprange` via opcode (S6l) see the raw field. A script reading during the sentinel window would see `-1`, which might confuse downstream script logic. However:
- The sentinel is ONLY set in the "no-AP-script-exists" branch, so no AP script is reading it.
- If a non-AP script reads it (unusual), the "-1" is semantically correct ("this player is not pursuing AP gating right now").

## 3. File Map

| File | Action | Task |
|---|---|---|
| `modules/world/interaction_trigger.go:355-362` | Modify — add `p.apRange = -1` + update S6l-D1 comment | 1 |
| `modules/world/interaction.go:103-112` | Modify — update the S6l-D1 comment in `processInteraction` | 1 |
| `modules/world/interaction_trigger_test.go` | Modify — existing S6l-D1 test notes + update; add 3 new sentinel tests | 1 |

## 4. Component Details

### 4.1 `fireApTriggerLoc` change

Current (`interaction_trigger.go:355-362`):

```go
sf := srv.scriptProvider.GetByTrigger(trigger, loc.Type(), category)
if sf == nil {
    // No AP script registered. DEVIATION S6l-D1: skip TS apRange=-1
    // sentinel. Interaction stays anchored; next tick re-evaluates.
    // If player has reached contact, OP/defaultOp takes over.
    p.interactionFired = true
    return
}
```

New:

```go
sf := srv.scriptProvider.GetByTrigger(trigger, loc.Type(), category)
if sf == nil {
    // S6l-D1 closed in S6r: cache "no AP script for this (trigger,
    // locType, category) triple" via the apRange=-1 sentinel so
    // inApproachDistance short-circuits on subsequent ticks.
    // Matches TS Player.ts:~1139-1170 behavior: apRange=-1 means
    // "AP path permanently disabled for this interaction;
    // anchor stays — contact (OP) takes over on a later tick."
    p.apRange = -1
    p.interactionFired = true
    return
}
```

### 4.2 `processInteraction` comment update

Current (`interaction.go:103-112`):

```go
if inApproachDistance(p.x, p.z, tx, tz, effectiveApRange(p)) {
    // Approach range — fire AP. Matches TS Player.ts:1139-1170.
    // DEVIATION S6l-D1: goscape skips TS's apRange=-1 sentinel
    // optimization; each tick does a fresh provider lookup.
    p.interacted = true
    ...
}
```

New:

```go
if inApproachDistance(p.x, p.z, tx, tz, effectiveApRange(p)) {
    // Approach range — fire AP. Matches TS Player.ts:1139-1170.
    // S6l-D1 closed in S6r: when fireApTriggerLoc finds no script,
    // it sets p.apRange = -1. Next tick's inApproachDistance sees
    // apRange <= 0 and returns false, skipping re-lookup.
    p.interacted = true
    ...
}
```

## 5. Test Plan

New tests in `modules/world/interaction_trigger_test.go`:

1. **TestFireApTriggerLocNoScriptSetsApRangeSentinel** — anchor an interaction on a Loc with NO AP script registered for (trigger, locType, category); call `fireApTriggerLoc`; assert `p.apRange == -1` post-call.

2. **TestApRangeSentinelShortCircuitsNextTick** — after the sentinel is set, invoke `processInteraction` with player within the old 10-tile range of the target; verify the AP branch is NOT entered (e.g., by asserting `p.interactionFired` stays false the second tick, or by checking no AP-script-registered-but-empty side effects). Cleanest probe: assert `inApproachDistance(..., effectiveApRange(p))` returns false post-sentinel.

3. **TestSetInteractionResetsApRangeSentinel** — set `p.apRange = -1` manually; call `p.SetInteraction(...)` with a new target; assert `p.apRange == 10` post-call (already works from S6l; test codifies the contract).

### Existing test that may need a touch

Grep `interaction_trigger_test.go` for `S6l-D1` mentions — update any "DEVIATION note" comments to "S6p closed" / "S6r closed" language.

## 6. Task Split

**Single task.** ~15 LOC impl + ~60 LOC tests + 2 comment updates. Not worth splitting.

- Commit: `feat(world): apRange=-1 sentinel for fireApTriggerLoc no-AP-script path — closes S6l-D1 (S6r)`

## 7. Deviations

| ID | Status |
|---|---|
| **S6l-D1** | **✅ CLOSED in S6r** |

No new deviations.

## 8. Scope

- Impl: ~4 LOC (one assignment + comment changes)
- Tests: ~60 LOC (3 new tests)
- 1 commit
- Total: ~75 LOC
