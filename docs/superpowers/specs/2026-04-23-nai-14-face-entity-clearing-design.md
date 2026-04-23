# NAI-14 — `faceEntity` Clear-on-Reset Semantics + Player `entitymask` Wiring

Close the last visible-fidelity gaps in the NPC face-entity mask lifecycle by
porting TS `Npc.clearInteraction:407-408`, `Npc.resetDefaults:415`, and
`PathingEntity.resetPathingEntity:611-614`. Additionally wire
`Player.entitymask = rsbuf.MaskFaceEntity` at construction to retire the
latent-no-op parallel of the NAI-13-resolved `Npc.entitymask` assignment.

Scope is **surgical**: preserves NAI-11's deliberate "stripped-down
`resetDefaults`" design decision. Only closes the faceEntity-clearing +
mask-emission half of the parity gap; the `apRange`/`apRangeCalled`/
`targetSubject` non-clearing in `resetDefaults` stays a tracked deviation.

**Roadmap:** Not part of the NPC AI tick decomposition (that closed with
NAI-13). This is the Q3-scope-out follow-up from `nai_followups.md` — two
entries: the NAI-11 "Deferred: Npc.entitymask mask plumbing" entry (never
fully resolved — NAI-13 did the NPC-construction half) and the NAI-13
close-note's mask-plumbing tail referencing PathingEntity sites 534/540/612.
Audit finding (documented below): sites 534 and 540 are in fact already
ported; site 612 is not. Roadmap fidelity risk: **Low** — mechanical
line-for-line port; no algorithmic decisions.

**Tech Stack:** Go 1.26+. No new packages. Existing `pkg/rsbuf` constants
(`NpcMaskFaceEntity`, `MaskFaceEntity`, both `= 0x4`), existing
`modules/world` NPC/Player infrastructure.

## Audit finding — 534/540 are already ported

The three sites tracked as deferred in NAI-13's close note decompose as:

| TS site | Location | Go status |
|---|---|---|
| `PathingEntity.ts:534` | `setInteraction` Player-branch mask emission (gated on `faceEntity !== playerSlot`) | **Already ported** at `npc_interaction.go:604-607`. Conditional shape matches TS. |
| `PathingEntity.ts:540` | `setInteraction` Npc-branch mask emission (gated on `faceEntity !== nid`) | **Already ported** at `npc_interaction.go:608-612`. Conditional shape matches TS. |
| `PathingEntity.ts:611-614` | `resetPathingEntity` trailing clear (`if !target && faceEntity !== -1`) | **Not ported.** No equivalent in Go's `ResetMasks` or elsewhere. |

So NAI-14 ports site 612 only (along with two adjacent same-family
fidelity gaps surfaced by the audit — see Goal §1 and §2).

## Goal

After NAI-14 ships:

1. **`(*Npc).resetDefaults`** at `modules/world/npc_interaction.go:38-42`
   clears `n.faceEntity = -1` after the `targetOp` assignment — mirrors TS
   `Npc.ts:415`. The "INTENTIONALLY does NOT clear ... faceEntity" half of
   the doc comment is rewritten; the apRange/apRangeCalled/targetSubject
   half stays.
2. **`(*Npc).clearInteraction`** at `modules/world/npc_interaction.go:47-53`
   adds `n.faceEntity = -1` and `n.masks |= n.entitymask` at the tail —
   mirrors TS `Npc.ts:407-408`. The "Does NOT touch faceEntity/masks — those
   are cleared by the masks frame-pass" doc comment is rewritten.
3. **`(*Npc).ResetMasks`** at `modules/world/npc_masks.go:70-78` adds a
   four-line conditional trailing clear after the existing ephemeral
   resets — mirrors TS `PathingEntity.ts:611-614` adapted for Go's
   tick-end-cleanup timing (see Error handling § "One-tick lag on site-612
   emission"):
   ```go
   if n.target == nil && n.faceEntity != -1 {
       n.masks |= n.entitymask
       n.faceEntity = -1
   }
   ```
4. **`NewPlayer`** at `modules/world/player.go:345-355` Player struct
   literal gains `entitymask: rsbuf.MaskFaceEntity,` — mirrors TS
   `PathingEntity.ts:107`. Field already declared at `player.go:115`;
   this assigns it. Closes the Player-side latent-no-op parallel of
   NAI-13's NPC-side fix.
5. **Tests flipped:** `TestNpcResetDefaultsClearsTargetKeepsOtherState`
   (`npc_interaction_test.go:738-769`) — `n.faceEntity` assertion flips
   from `== 99` to `== -1`; inline comment updated.
   `TestNpcClearInteractionResetsState` (`npc_interaction_test.go:771+`)
   — add assertions that `n.faceEntity == -1` and
   `n.masks & rsbuf.NpcMaskFaceEntity != 0` post-call.
6. **New tests added:** 6 total — one each fresh-assert for the faceEntity
   clears in resetDefaults and clearInteraction, three
   `TestResetMasksTrailingClear*` guards (positive fire, target-present
   no-fire, already-minus-one no-fire), and one `TestNewPlayerSetsEntityMask`
   mirroring NAI-13's NPC-side test.
7. **Memory entry** `nai_followups.md` § "From NAI-11 → Deferred:
   Npc.entitymask mask plumbing" is marked **Resolved 2026-04-23 (NAI-14)**
   with the resolution pointer. The NAI-13 close note's mask-plumbing
   tail gets a cross-reference.

## Scope — what's IN

### NPC-side code (3 sites)

1. **`resetDefaults` faceEntity clear** — `modules/world/npc_interaction.go`
   (within the existing 4-line `resetDefaults` method). Single line insert:
   `n.faceEntity = -1` before the existing `n.masks |= n.entitymask`.
   Mirrors TS `Npc.ts:415`. Doc comment at lines 32-37 rewritten: remove
   "faceEntity" from the "INTENTIONALLY does NOT clear" list; keep
   apRange/apRangeCalled/targetSubject.
2. **`clearInteraction` faceEntity clear + mask emit** —
   `modules/world/npc_interaction.go` (within the existing 5-line
   `clearInteraction` method). Two-line insert at the tail:
   `n.faceEntity = -1; n.masks |= n.entitymask`. Mirrors TS Npc.ts:407-408
   (which inlines what TS `Npc.clearInteraction` overrides on top of
   `PathingEntity.clearInteraction`). Doc comment at lines 44-46
   rewritten: replace "Does NOT touch faceEntity/masks" with a
   description of the new TS-matching behavior.
3. **`ResetMasks` trailing defensive clear** — `modules/world/npc_masks.go`
   (within the existing `ResetMasks` method). Four-line conditional
   added after the existing ephemeral resets:
   ```go
   if n.target == nil && n.faceEntity != -1 {
       n.masks |= n.entitymask
       n.faceEntity = -1
   }
   ```
   Mirrors TS `PathingEntity.ts:611-614`. Comment above the conditional
   points at the TS line and notes the one-tick-lag deviation (see Error
   handling).

### Player-side code (1 site)

4. **`NewPlayer` entitymask assignment** — `modules/world/player.go:352`
   region. Insert `entitymask: rsbuf.MaskFaceEntity,` in the Player
   struct literal (between `faceEntity: -1,` and the sentinel-init
   comment, in the `// === masks ===` block). Single line. Mirrors TS
   `PathingEntity.ts:107`.

### Doc comment updates (2)

- `resetDefaults` at `npc_interaction.go:32-37` — the current comment
  reads "INTENTIONALLY does NOT clear apRange, apRangeCalled, faceEntity,
  or the rest of masks — those are overwritten only by the next
  SetInteraction call." Rewrite: keep the apRange/apRangeCalled clause;
  drop the "faceEntity" and "rest of masks" claim (now mirrors
  TS `Npc.ts:415` by clearing faceEntity; the mask OR persists across
  ticks until `ResetMasks` fires, which is the existing behavior).
- `clearInteraction` at `npc_interaction.go:44-46` — the current comment
  reads "Does NOT touch faceEntity/masks — those are cleared by the
  masks frame-pass, not here." Rewrite: now clears faceEntity and
  emits the entitymask bit per TS `Npc.ts:407-408`.

### Test modifications (2 flipped) and additions (6 new)

**Flipped:**

1. **`TestNpcResetDefaultsClearsTargetKeepsOtherState`** at
   `npc_interaction_test.go:738-769`. Change `n.faceEntity` assertion
   from "equals 99" to "equals -1". Update the inline comment
   "These stay untouched — next SetInteraction call will overwrite"
   to reflect that faceEntity is now cleared (apRange/apRangeCalled
   stay). The existing `n.masks != 0xff` assertion stays valid because
   `NpcMaskFaceEntity = 0x4` ORs into 0xff without change.
2. **`TestNpcClearInteractionResetsState`** at
   `npc_interaction_test.go:771+`. Add two assertions post-call:
   `n.faceEntity == -1` and `n.masks & rsbuf.NpcMaskFaceEntity != 0`.

**New (6):**

1. `TestNpcResetDefaultsClearsFaceEntity` — fresh assertion specifically
   for the TS :415 clear, so the line-to-TS mapping is named-test
   explicit (separated from the "keeps-other-state" regression guard).
2. `TestNpcClearInteractionEmitsEntityMaskAndClearsFaceEntity` —
   fresh companion for clearInteraction covering TS :407-408 as a
   single named unit.
3. `TestResetMasksTrailingClearFires` — `n.target = nil`, `n.faceEntity = 42`,
   `n.masks = 0`, call `ResetMasks`; assert `n.faceEntity == -1` AND
   `n.masks & rsbuf.NpcMaskFaceEntity != 0`.
4. `TestResetMasksTrailingClearSkippedWhenTargetPresent` —
   `n.target = someNpc`, `n.faceEntity = 42`; call `ResetMasks`;
   assert `n.faceEntity == 42` (trailing clear did NOT fire). Proves
   the target-nil guard.
5. `TestResetMasksTrailingClearSkippedWhenFaceEntityAlreadyMinusOne` —
   `n.target = nil`, `n.faceEntity = -1`; call `ResetMasks`;
   assert `n.masks == 0` (no mask bit emitted — no-op path). Proves
   the faceEntity-nonminus-one guard.
6. `TestNewPlayerSetsEntityMaskToMaskFaceEntity` —
   `NewPlayer(...).entitymask == rsbuf.MaskFaceEntity`. Mirrors
   NAI-13's `TestNewNpcSetsEntityMaskToFaceEntity`.

### Memory updates (2)

- `~/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md`
  § "From NAI-11 (2026-04-22) → Deferred: Npc.entitymask mask plumbing"
  (lines 389-396) — add **Resolved 2026-04-23 (NAI-14)** header block
  above the existing body, pointing at this spec and the commit(s).
  Preserve historical body for context.
- Same file, NAI-13 close note (lines 271-303) — add a one-line
  cross-reference that the "mask-plumbing tail" referenced at NAI-13
  close was closed by NAI-14, plus the audit finding that sites 534/540
  were already ported.

## Scope — what's OUT (tracked as continuing deviations)

1. **`resetDefaults` apRange/apRangeCalled/targetSubject clearing** — TS's
   `resetDefaults` calls `clearInteraction` (which resets these); Go's
   flat `resetDefaults` does not. Stays as NAI-11 deliberate deviation.
   The half of the doc comment describing this stays.
2. **`resetDefaults` → `clearInteraction` structural delegation** — TS
   `Npc.ts:412` has `this.clearInteraction()` as the first statement of
   `resetDefaults`. Go's flat (non-delegating) structure is kept per
   the Approach-A surgical scope decision. Future sub-spec may restore
   full structural parity; not this one.
3. **Player-side analog of sites 534/540/612** — Go's Player has no
   `p.masks |= p.entitymask` emission sites today (verified by grep),
   and no `Player.SetInteraction` equivalent. Assigning `p.entitymask`
   in `NewPlayer` is structural future-proofing; it becomes functional
   only when/if a future Player-interaction port sub-spec adds the
   `|=` lines. That future sub-spec should use `p.entitymask` at its
   mask-emission sites rather than hardcoding `rsbuf.MaskFaceEntity`.
4. **TS `PathingEntity.resetPathingEntity` full port** — NAI-14 ports
   only site 612 (the trailing target-clear). The other ~20 lines of
   TS `resetPathingEntity` (`moveSpeed`, `walkDir`, `runDir`, `jump`,
   `tele`, `lastTickX/Z/level`, `stepsTaken`, `interacted`,
   `exactStart*`, `exactEnd*`, `exactMove*`, `animId`, `animDelay`,
   `sayMessage`, `hitmark*`, `spotanim*`, `faceSquareX/Z`) are partly
   resident in Go's `ResetMasks` already, and partly not applicable
   (e.g., `interacted` is a known NAI-11 deferral). A future sub-spec
   may consolidate these into a TS-named `resetPathingEntity()` method
   port; not a goal here.
5. **One-tick lag on site-612 emission** — Go's `ResetMasks` runs at
   tick end; TS's `resetPathingEntity` runs at tick start. Mask bit
   emitted by site 612 survives into the next tick's info-pass and
   reaches the client one tick later than TS. Only observable on the
   "stray `n.target = nil` without mask emission" edge case, which
   NAI-14's §1 and §2 changes mostly close. Tracked as an accepted
   deviation; comment at the conditional documents the intent.

## Architecture

### File modifications (no new files)

| File | Change |
|---|---|
| `modules/world/npc_interaction.go` | `resetDefaults`: +1 line (`n.faceEntity = -1`); `clearInteraction`: +2 lines (`n.faceEntity = -1; n.masks \|= n.entitymask`); both doc comments revised. |
| `modules/world/npc_masks.go` | `ResetMasks`: +4-line conditional trailing clear, +2 comment lines pointing at TS `PathingEntity.ts:611-614` and noting one-tick-lag deviation. |
| `modules/world/player.go` | `NewPlayer` struct literal: +1 line `entitymask: rsbuf.MaskFaceEntity,`. |
| `modules/world/npc_interaction_test.go` | 2 tests flipped (faceEntity clear assertions, mask-emit assertion). |
| `modules/world/npc_masks_test.go` | 5 new tests (1 resetDefaults fresh-assert, 1 clearInteraction fresh-assert, 3 ResetMasks trailing-clear variants). |
| `modules/world/player_test.go` OR `modules/world/player_masks_test.go` | 1 new test (Player entitymask assignment). Placement matches NAI-13's `TestNewNpcSetsEntityMaskToFaceEntity` convention (NPC-side used `npc_masks_test.go`). Match it on Player side: `player_masks_test.go`. |

**Net production LOC:** ~10 lines (3 in `npc_interaction.go` + 6 in `npc_masks.go` + 1 in `player.go`). **Net test LOC:** ~60-80 lines.

### Type additions — none

No struct field additions, no new types, no new interface methods.
`Player.entitymask` already exists at `player.go:115` (unassigned).
`Npc.entitymask` already exists and is already assigned by NAI-13.

### Helpers consumed (all exist)

- `rsbuf.NpcMaskFaceEntity = 0x4` at `pkg/rsbuf/npc_source.go:6`.
- `rsbuf.MaskFaceEntity = 0x4` at `pkg/rsbuf/visibility.go:18`.

## Data flow

### Before NAI-14 — faceEntity mask-emission coverage

```
Target-clear code path                          Emits faceEntity mask?
  ├─ SetInteraction() [new Player/Npc target]   YES (NAI-13)
  ├─ resetDefaults()                            YES (NAI-13) — but faceEntity NOT cleared
  ├─ clearInteraction()                         NO
  └─ stray n.target = nil                       NO
```

Gap 1: `clearInteraction` sets `n.target = nil` but never emits the mask
or clears `faceEntity`. Any of the 24 call sites in
`npc_interaction_trigger.go` that call `clearInteraction` leave the
client thinking the NPC is still facing its old target.

Gap 2: `resetDefaults` emits the mask but does NOT clear
`faceEntity` — so even though the mask bit fires, the encoded
`faceEntity` value on the wire is still the old slot/nid, not -1.

### After NAI-14 — faceEntity mask-emission coverage

```
Target-clear code path                          Emits mask?  Clears faceEntity?
  ├─ SetInteraction() [new Player/Npc target]   YES          N/A (sets to new)
  ├─ resetDefaults()                            YES          YES (NEW — TS :415)
  ├─ clearInteraction()                         YES (NEW)    YES (NEW — TS :407-408)
  └─ stray n.target = nil                       YES*         YES*  (*via ResetMasks trailing clear)
```

Result: **every target-clear code path ensures the client receives a
faceEntity=-1 update** — either same-tick (official paths) or next-tick
(stray paths, via `ResetMasks` catch-net).

### ResetMasks tick-timing (Go)

`ResetMasks` is called in `processCleanup` at `modules/world/tick.go:383,386`
— the **end** of each tick, after the info-encoding pass. Sequence:

```
Tick N:
  1. processInteraction / ai / movement / etc.   [reads + writes state, may call resetDefaults/clearInteraction]
  2. info-encoding pass                          [reads n.masks, n.faceEntity — wire output]
  3. processCleanup → ResetMasks                 [clears masks, ephemeral state]
     └─ [NEW] if target==nil && faceEntity!=-1:
          masks |= entitymask
          faceEntity = -1
       ↑ The mask bit set here survives into Tick N+1's info-pass.

Tick N+1:
  1. processInteraction etc.
  2. info-encoding pass                          [reads the entitymask bit set at end of Tick N → client]
  3. processCleanup → ResetMasks                 [clears again]
```

TS fires site 612 at tick start (before info-pass of that same tick).
Go fires it at tick end, so the wire update reaches the client one
tick later. Acceptable deviation (see Error handling).

## Error handling

### No new error paths

Each code change is a pure state mutation — no new failure modes, no
new log emission, no new returns. The trailing clear in `ResetMasks` is
conditional on a two-predicate guard (`n.target == nil && n.faceEntity != -1`);
any other combination is a no-op (matches TS).

### One-tick lag on site-612 emission (accepted deviation)

TS PathingEntity.ts:611-614 fires at tick start; Go's `ResetMasks`
fires at tick end. The mask bit set by Go's trailing clear is read by
the **next** tick's info-pass, not the current one.

Consequence: on the rare edge case where `n.target` is assigned `nil`
outside of `resetDefaults`/`clearInteraction`/`ResetMasks` (e.g., some
future direct assignment elsewhere), the "NPC stopped facing X"
update reaches the client one tick later than TS. During that
one-tick window the client still shows the NPC facing the old
entity.

Why acceptable:
1. The common paths (`resetDefaults`, `clearInteraction`) emit the
   mask same-tick post-NAI-14. Only stray `n.target = nil`
   assignments rely on the trailing-clear net, and none exist in
   the codebase today (grep confirms).
2. A one-tick visual lag on an NPC's face direction is below
   observable threshold for players (client tick is 600ms; the NPC
   briefly keeps facing its old target then snaps away).
3. Eliminating the lag would require moving `ResetMasks` (or a new
   `resetPathingEntity`) to tick start — a tick-loop refactor out
   of scope for NAI-14. Approach 3 in the brainstorm covers that
   path for a future sub-spec.

Tracked: inline comment at the new conditional in `ResetMasks` cites
TS line + notes the timing deviation. No new `nai_followups.md`
entry (captured in this spec's "Scope — what's OUT" §5).

### Trailing clear preserves `ResetMasks` function (no regression risk)

The existing `ResetMasks` ephemeral-reset behavior is unchanged —
the new conditional runs **after** the existing resets, and only
mutates `n.masks` / `n.faceEntity` when both predicates hold. All
existing `TestNpc*ResetMasks*` tests pass unchanged.

## Testing strategy

### Test file layout

- **`modules/world/npc_interaction_test.go`** — 2 existing tests flipped
  (resetDefaults, clearInteraction).
- **`modules/world/npc_masks_test.go`** — 5 new tests: 2 named
  fresh-assert companions for resetDefaults / clearInteraction faceEntity
  clears; 3 ResetMasks-trailing-clear variants (fire, target-present
  skip, already-minus-one skip). File already hosts the analogous
  NAI-13 `TestResetDefaultsEmitsEntityMask` and
  `TestSetInteractionEmitsEntityMask` — matching pattern and fixtures.
- **`modules/world/player_masks_test.go`** — 1 new test for Player
  entitymask assignment. File hosts the analogous Player
  mask-plumbing tests; matching placement.

### Fixtures

All reuse existing patterns. No new mocks:

- `newTestNpc(t, ...)` — NPC construction.
- `NewPlayer(...)` — exists; tested by the new test directly (no
  fixture needed since the test is construction-only).
- No gamemap dependency in any of these tests (trailing clear is
  pure state mutation).

### Test inventory (8 tests: 6 new + 2 flipped)

**Flipped (2):**

- `TestNpcResetDefaultsClearsTargetKeepsOtherState`
  (`npc_interaction_test.go:738-769`) — faceEntity assertion flips
  from `== 99` to `== -1`. Inline comment edited. `n.masks != 0xff`
  assertion stays (NpcMaskFaceEntity = 0x4; `0xff | 0x4 == 0xff`).
- `TestNpcClearInteractionResetsState`
  (`npc_interaction_test.go:771+`) — add
  `if n.faceEntity != -1` and
  `if n.masks & rsbuf.NpcMaskFaceEntity == 0` assertion blocks.

**New — fresh assertions for the flipped behavior (2):**

- `TestNpcResetDefaultsClearsFaceEntity`
  (`npc_masks_test.go`) — NPC with `n.faceEntity = 42`, call
  `resetDefaults`, assert `n.faceEntity == -1`. Name makes the TS
  `Npc.ts:415` mapping explicit.
- `TestNpcClearInteractionEmitsEntityMaskAndClearsFaceEntity`
  (`npc_masks_test.go`) — NPC with `n.faceEntity = 42, n.masks = 0`,
  call `clearInteraction`, assert `n.faceEntity == -1` AND
  `n.masks & rsbuf.NpcMaskFaceEntity != 0`. Name documents TS
  `Npc.ts:407-408` port in one place.

**New — ResetMasks trailing-clear guards (3):**

- `TestResetMasksTrailingClearFires` — `n.target = nil`,
  `n.faceEntity = 42`, `n.masks = 0` (clear any construction-time
  bits), call `ResetMasks`, assert `n.faceEntity == -1` AND
  `n.masks & rsbuf.NpcMaskFaceEntity != 0`.
- `TestResetMasksTrailingClearSkippedWhenTargetPresent` —
  `n.target = &Npc{nid: 7}` (or some non-nil), `n.faceEntity = 42`,
  call `ResetMasks`, assert `n.faceEntity == 42` (unchanged). Proves
  the `target == nil` guard.
- `TestResetMasksTrailingClearSkippedWhenFaceEntityAlreadyMinusOne` —
  `n.target = nil`, `n.faceEntity = -1`, `n.masks = 0`, call
  `ResetMasks`, assert `n.masks == 0` (no mask bit emitted — no-op
  path). Proves the `faceEntity != -1` guard.

**New — Player entitymask wiring (1):**

- `TestNewPlayerSetsEntityMaskToMaskFaceEntity`
  (`player_masks_test.go`) — `NewPlayer(...)`, assert
  `p.entitymask == rsbuf.MaskFaceEntity`. Mirrors NAI-13's
  `TestNewNpcSetsEntityMaskToFaceEntity`.

### Verification sweep (at close commit)

- `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/...`
  — NPC + Player test pass.
- `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...` — no
  regressions anywhere.
- `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./...` —
  confirm no new data-race shape (unlikely; changes are all
  single-goroutine ephemeral state).
- Grep for stale doc-comment strings:
  - `"INTENTIONALLY does NOT clear.*faceEntity"` — confirm absent.
  - `"Does NOT touch faceEntity/masks"` — confirm absent.
- Grep `nai_followups.md` for "Deferred: Npc.entitymask" — confirm
  **Resolved 2026-04-23 (NAI-14)** marker landed.

### Plan ordering

The plan should sequence tasks as follows. Each task lands with its
TDD-executable test set and is independently committable.

1. **Task 1 — `clearInteraction` + `resetDefaults` faceEntity clearing.**
   Both TS-line-adjacent changes in `npc_interaction.go`. Update both
   doc comments. Flip the 2 existing tests; add the 2 fresh-assert
   tests. ~3 prod LOC + 4 test edits/adds. TS-line-density highest
   of the three tasks, smallest blast radius.
2. **Task 2 — `ResetMasks` trailing clear + quirk guards.**
   Code change in `npc_masks.go`; add the 3 new tests. ~4 prod LOC
   + ~30 test LOC. Exercises the tick-end-timing deviation.
3. **Task 3 — Player `entitymask` wiring + close commit.**
   `player.go` one-liner + 1 new test. Memory updates to
   `nai_followups.md` (NAI-11 entry Resolved marker + NAI-13 cross-ref).
   Final `go test ./...` sweep. ~1 prod LOC + ~15 test LOC.

## Files changed

**New:** none.

**Modified — production (3):**
- `modules/world/npc_interaction.go` (+3 LOC in method bodies; 2 doc
  comments revised)
- `modules/world/npc_masks.go` (+6 LOC: 4 conditional + 2 comment)
- `modules/world/player.go` (+1 LOC in struct literal)

**Modified — tests (3):**
- `modules/world/npc_interaction_test.go` (2 existing tests adjusted)
- `modules/world/npc_masks_test.go` (5 new tests added)
- `modules/world/player_masks_test.go` (1 new test added)

**No changes:**
- `pkg/rsbuf` — mask constants already exist and already consumed.
- `pkg/pathfinder` — unrelated.
- `pkg/objtype` — unrelated.
- `pkg/entity` — unrelated.

## References

### TS source
- `Engine-TS/src/engine/entity/Npc.ts:402-409` — `clearInteraction`
  (Task 1 target).
- `Engine-TS/src/engine/entity/Npc.ts:411-425` — `resetDefaults`
  (Task 1 target).
- `Engine-TS/src/engine/entity/PathingEntity.ts:107` — base-class
  `entitymask` assignment (Task 3 target).
- `Engine-TS/src/engine/entity/PathingEntity.ts:577-615` — full
  `resetPathingEntity` body; site 612 is the line-611-614 trailing
  clear (Task 2 target).

### Go code
- `modules/world/npc_interaction.go:32-42` — `resetDefaults` target.
- `modules/world/npc_interaction.go:44-53` — `clearInteraction` target.
- `modules/world/npc_masks.go:66-78` — `ResetMasks` target.
- `modules/world/player.go:115` — `Player.entitymask` field
  declaration.
- `modules/world/player.go:345-355` — `NewPlayer` struct literal
  (insertion point for `entitymask: rsbuf.MaskFaceEntity,`).
- `modules/world/npc_interaction_test.go:738-810` — tests to flip.
- `modules/world/tick.go:383,386` — `ResetMasks` call sites
  confirming tick-end timing.
- `pkg/rsbuf/npc_source.go:6` — `NpcMaskFaceEntity = 0x4`.
- `pkg/rsbuf/visibility.go:18` — `MaskFaceEntity = 0x4`.

### Memory / prior specs
- `~/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md`
  § "From NAI-11 → Deferred: Npc.entitymask mask plumbing" (to be
  marked Resolved by this sub-spec).
- `docs/superpowers/specs/2026-04-23-nai-13-player-modes-design.md`
  — NAI-13 close note (source of the "mask-plumbing tail" scope-out
  that NAI-14 addresses).
- `docs/superpowers/specs/2026-04-22-nai-11-npc-movement-interaction-design.md`
  — original NAI-11 spec where `resetDefaults`/`clearInteraction`
  were deliberately stripped.
