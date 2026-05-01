# NAI-61 — ClearPendingAction-vs-members ordering fix in OpObjU/OpLocU/OpPlayerU

> **TS-faithfulness gate.** Pure ordering fix in three handlers to match TS
> `OpObjUHandler.ts:68-74`, `OpLocUHandler.ts:68-74`, `OpPlayerUHandler.ts:66-72`.
> No new deviations. No new APIs. No new behaviour beyond the timing of one
> existing call. Compressed-cadence sub-spec (combined spec+plan); see
> `compressed_cadence.md`.

## §1. Origin

Surfaced as an out-of-scope, untracked TS divergence by NAI-60 T2 + final
cross-task reviewers. Filed under `nai_followups.md` § "NAI-60 — CLOSED
2026-05-01" → "TS-divergence surfaced (pre-existing, untracked at HEAD)".

`handleOpNpcU` (handler_opnpc.go:181) is the canonical reference — it
already calls `p.ClearPendingAction()` BEFORE the members-only check. The
other three U-handlers diverge.

## §2. The divergence (verified at HEAD `ece5767`)

| Handler | TS clear position | goscape current position | Members-check position |
|---|---|---|---|
| OpObjU | `OpObjUHandler.ts:68` (before members) | `handler_opobj.go:269` (after members) | TS: 70-74 / goscape: 258-264 |
| OpLocU | `OpLocUHandler.ts:68` (before members) | `handler_oploc.go:291` (after members) | TS: 70-74 / goscape: 280-286 |
| OpPlayerU | `OpPlayerUHandler.ts:66` (before members) | `handler_op_player.go:212` (after members) | TS: 68-72 / goscape: 201-207 |

**Practical delta:** when the members-only `objTypes`-driven check rejects
the click, TS leaves `pendingAction` cleared (modal closed, target/op
reset); goscape leaves it stale. Minor but real — next-tick interaction
state diverges from TS.

`ClearPendingAction` body (player_script.go:766-771):

```go
func (p *Player) ClearPendingAction() {
    p.interactionKind = InteractionEngine
    p.target = nil
    p.targetOp = -1
    p.CloseModal(true)
}
```

## §3. The fix — three mechanical line-moves

For each handler, **move** the existing `p.ClearPendingAction()` call from
its current position (immediately above `p.opcalled = true`) to immediately
after the inventory `HasAt` check, **before** the `objTypes` members-only
block.

### §3.1 `handler_opobj.go::handleOpObjU`

Current (lines 253-271):

```go
    if !inv.HasAt(useSlot, useObj) {
        sendUnsetMapFlag(p)
        return nil
    }

    if s.objTypes != nil && useObj >= 0 && useObj < len(s.objTypes.Configs) {
        if useObjType := s.objTypes.Configs[useObj]; useObjType != nil && useObjType.Members && !s.cfg.NodeMembers {
            p.MessageGame("To use this item please login to a members' server.")
            sendUnsetMapFlag(p)
            return nil
        }
    }

    p.lastUseItem = useObj
    p.lastUseSlot = useSlot

    p.ClearPendingAction()
    p.opcalled = true
    p.SetInteraction(InteractionEngine, obj, targetOpObjU, -1)
```

Target:

```go
    if !inv.HasAt(useSlot, useObj) {
        sendUnsetMapFlag(p)
        return nil
    }

    p.ClearPendingAction()

    if s.objTypes != nil && useObj >= 0 && useObj < len(s.objTypes.Configs) {
        if useObjType := s.objTypes.Configs[useObj]; useObjType != nil && useObjType.Members && !s.cfg.NodeMembers {
            p.MessageGame("To use this item please login to a members' server.")
            sendUnsetMapFlag(p)
            return nil
        }
    }

    p.lastUseItem = useObj
    p.lastUseSlot = useSlot

    p.opcalled = true
    p.SetInteraction(InteractionEngine, obj, targetOpObjU, -1)
```

### §3.2 `handler_oploc.go::handleOpLocU`

Same shape — current (lines 273-293):

```go
    if !inv.HasAt(useSlot, useObj) {
        sendUnsetMapFlag(p)
        return nil
    }

    // S6m-D4 closed in S6z: reject members-only items on
    // free-to-play worlds. Matches TS OpLocUHandler.ts:70-73.
    if s.objTypes != nil && useObj >= 0 && useObj < len(s.objTypes.Configs) {
        if useObjType := s.objTypes.Configs[useObj]; useObjType != nil && useObjType.Members && !s.cfg.NodeMembers {
            p.MessageGame("To use this item please login to a members' server.")
            sendUnsetMapFlag(p)
            return nil
        }
    }

    p.lastUseItem = useObj
    p.lastUseSlot = useSlot

    p.ClearPendingAction()
    p.opcalled = true
    p.SetInteraction(InteractionEngine, loc, targetOpLocU, -1)
```

Target: insert `p.ClearPendingAction()` immediately after the `HasAt` block,
delete the existing call between `lastUseSlot` and `opcalled`. Preserve the
existing `S6m-D4 closed in S6z` comment block above the members check.

### §3.3 `handler_op_player.go::handleOpPlayerU`

Same shape — current (lines 185-214):

```go
    if !inv.HasAt(useSlot, useObj) {
        sendUnsetMapFlag(p)
        return nil
    }

    other := s.LookupPlayerBySlot(slot)
    if other == nil {
        sendUnsetMapFlag(p)
        return nil
    }

    if !s.rsbuf.HasPlayer(int32(p.slot), int32(other.slot)) {
        sendUnsetMapFlag(p)
        return nil
    }

    if s.objTypes != nil && useObj >= 0 && useObj < len(s.objTypes.Configs) {
        if useObjType := s.objTypes.Configs[useObj]; useObjType != nil && useObjType.Members && !s.cfg.NodeMembers {
            p.MessageGame("To use this item please login to a members' server.")
            sendUnsetMapFlag(p)
            return nil
        }
    }

    p.lastUseItem = useObj
    p.lastUseSlot = useSlot

    p.ClearPendingAction()
    p.opcalled = true
    p.SetInteraction(InteractionEngine, other, targetOpPlayerU, -1)
```

**Important sequence:** TS clears AFTER `rsbuf.hasPlayer` and BEFORE the
members check (TS lines 60-72). So the goscape clear must go between the
`rsbuf.HasPlayer` block and the `objTypes` block — not right after `HasAt`.
Sequence in TS:

1. `useInv.hasAt(useSlot, useObj)` reject
2. `World.getPlayer(playerSlot)` reject (`other`)
3. `rsbuf.hasPlayer(...)` reject
4. **clearPendingAction**
5. members reject
6. lastUseItem/Slot snapshot + setInteraction

Target:

```go
    if !inv.HasAt(useSlot, useObj) {
        sendUnsetMapFlag(p)
        return nil
    }

    other := s.LookupPlayerBySlot(slot)
    if other == nil {
        sendUnsetMapFlag(p)
        return nil
    }

    if !s.rsbuf.HasPlayer(int32(p.slot), int32(other.slot)) {
        sendUnsetMapFlag(p)
        return nil
    }

    p.ClearPendingAction()

    if s.objTypes != nil && useObj >= 0 && useObj < len(s.objTypes.Configs) {
        if useObjType := s.objTypes.Configs[useObj]; useObjType != nil && useObjType.Members && !s.cfg.NodeMembers {
            p.MessageGame("To use this item please login to a members' server.")
            sendUnsetMapFlag(p)
            return nil
        }
    }

    p.lastUseItem = useObj
    p.lastUseSlot = useSlot

    p.opcalled = true
    p.SetInteraction(InteractionEngine, other, targetOpPlayerU, -1)
```

**Note for OpObjU and OpLocU:** TS sequence is `hasAt → clearPendingAction →
members`, so the clear goes immediately after `HasAt`. For OpPlayerU, TS
inserts `getPlayer` and `rsbuf.hasPlayer` checks between `hasAt` and
`clearPendingAction` — the clear goes after `rsbuf.HasPlayer`, not after
`HasAt`. Implementer must respect the per-handler difference.

### §3.4 Doc-comment trailers

Update each handler's doc-comment "On success" trailer (or equivalent) to
reflect the new sequence. Concretely:

- `handler_opobj.go:170-184` (handleOpObjU): the existing "Gates per TS" list
  enumerates the members check at gate 10. Add an explicit one-line trailer
  above the function body, mirroring the form already used by `handleOpLocU`
  (handler_oploc.go:200-204): `"On success: ClearPendingAction (after HasAt
  reject, before members check) → set lastUseItem/Slot → SetInteraction(...)
  → targetSubject snapshot."`
- `handler_oploc.go:201-204`: rewrite "On success: set p.lastUseItem = useObj,
  p.lastUseSlot = useSlot → ClearPendingAction → SetInteraction(...)" to
  reflect the new ordering: `"On success: ClearPendingAction (after HasAt
  reject, before members check) → set p.lastUseItem = useObj, p.lastUseSlot
  = useSlot → SetInteraction(...)"`.
- `handler_op_player.go:141-142`: same — rewrite the "On success" trailer
  to put ClearPendingAction first and call out the position (after
  rsbuf.HasPlayer reject, before members check).

## §4. Tests — new dedicated ordering pin per handler

The existing members-on-free-world tests (`TestHandleOpLocUMembersOnFreeWorldRejected`
at handler_oploc_test.go:744, `TestHandleOpPlayerU_MembersOnNonMembersServer`
at handler_op_player_test.go:530) assert `p.target` remains `nil` after
rejection. They do NOT pre-seed a stale pending action, so they pass with
both pre- and post-fix code (target was never set in the first place). They
do not pin the new ordering.

Add a new dedicated test per handler that **pre-seeds a stale pending
action**, invokes the handler with members-only payload, and asserts
post-call that the stale state is cleared. This is the only assertion that
distinguishes pre- and post-fix code.

OpObjU has NO existing members-on-free-world test fixture at all (see
`grep "Members" handler_opobj_test.go` → 0 hits). Implementer must build the
fixture using the existing OpObjU test helpers (`makeOpObjFixture`,
`opObjUPayload`, `seedComponentTypes`, etc. — pattern is parallel to
`TestHandleOpObjUItemMismatchRejected` at handler_opobj_test.go:479).

### §4.1 New tests

Three new tests, one per handler. Pattern (illustrative — implementer must
adapt to per-handler fixture helpers; see §4.2 for grep hints):

```go
func TestHandleOpObjUMembersOnFreeWorldClearsPendingAction(t *testing.T) {
    // setup: standard OpObjU fixture with valid component, listener,
    // inventory item; seed members-only ObjType for useObj; NodeMembers=false.
    // ... (use seedComponentTypes, listener wiring, inv seeding, members-ObjType
    // seeding patterns from TestHandleOpLocUMembersOnFreeWorldRejected as
    // reference; payload via opObjUPayload)

    // pre-seed stale pending action — proves the members reject clears it
    p.targetOp = 99
    p.target = someStaleEntity // any non-nil entity (e.g., the obj itself, or
                               // a fresh *Loc/*Npc — anything matching the
                               // entity interface)

    received := drainConn(t, cc)
    _ = handleOpObjU(p, opObjUPayload(...))
    p.client.flushWrite()
    got := <-received

    // existing assertions: packet was emitted (members-reject path)
    if len(got) == 0 {
        t.Fatal("expected MessageGame + UnsetMapFlag for members-on-free, got nothing")
    }

    // NEW: ordering pin — ClearPendingAction must have run before members reject
    if p.targetOp != -1 {
        t.Errorf("targetOp: got %d, want -1 (cleared by ClearPendingAction before members reject)", p.targetOp)
    }
    if p.target != nil {
        t.Errorf("target: got %v, want nil (cleared by ClearPendingAction before members reject)", p.target)
    }
}
```

For OpLocU (`TestHandleOpLocUMembersOnFreeWorldClearsPendingAction`) and
OpPlayerU (`TestHandleOpPlayerU_MembersOnNonMembersServerClearsPendingAction`),
mirror the shape — copy the existing members-on-free-world test setup, add
the pre-seed + assertions.

### §4.2 Implementer grep hints

- `makeOpObjFixture` / `makeOpLocFixture` / `makeOpPlayerFixture` — fixture
  constructors; signature differs per family (some return `(s, p, _, cc)`,
  others `(s, clicker, other, cc)`).
- `opObjUPayload` / `p2x6Payload` / `opPlayerUPayload` — payload builders.
- `seedComponentTypes` — component-registry seeder.
- `seedOpPlayerUInv` — OpPlayerU-specific listener+inv seeding helper
  (handler_op_player_test.go uses it; OpLocU writes its inv seed inline at
  oploc_test.go:760-766; OpObjU has no equivalent — replicate the inline
  pattern).
- For the stale-target value, use any concrete entity. The simplest is the
  same `obj`/`loc`/`other` the test would set on success; or seed a separate
  `&Loc{...}`. Implementer chooses.

### §4.3 TDD discipline

Write the new test FIRST (RED — should fail against unpatched code because
`p.targetOp` stays at 99), then move the `ClearPendingAction()` call (GREEN),
then verify all existing tests still pass (REGRESSION). One handler at a
time. No mass-edit.

## §5. Bundle / dispatch shape

Single bundle, single implementer (subagent-driven-development per
`execution_mode_default`). Compressed cadence — no formal review pass per
`compressed_cadence.md` (3 line-moves + 3 doc-comments + 3 new tests).

### §5.1 Pre-flight (controller, before dispatch)

Per `controller_preflight.md`:

1. `git status` clean (no work-in-progress).
2. Re-Read each handler at HEAD to confirm the line numbers in §3 match.
3. `grep -n "ClearPendingAction" handler_op{obj,loc,_player}.go` to confirm
   exactly one call per handler (no double-clear bugs hiding).
4. `grep -n "Members" handler_op{obj,loc,_player}_test.go` to confirm the
   existing fixtures match §4 references.

### §5.2 Worktree

Per `using-git-worktrees`: fresh worktree off `main`. Branch name
`nai-61-clearpending-ordering`.

### §5.3 Verification (implementer, before commit)

Per `verification-before-completion.md`:

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./modules/world/...
```

Plus one cross-package smoke:

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...
```

### §5.4 Commit shape

Single commit, both impl + close, since this is a one-bundle compressed
sub-spec:

```
feat(world): NAI-61 — clear-pending-action-vs-members ordering in OpObjU/OpLocU/OpPlayerU

Move p.ClearPendingAction() to fire BEFORE the members-only objTypes
check in handleOpObjU, handleOpLocU, handleOpPlayerU, matching TS
OpObjUHandler.ts:68, OpLocUHandler.ts:68, OpPlayerUHandler.ts:66.
handleOpNpcU was already correct (canonical reference).

Pre-existing untracked TS divergence surfaced by NAI-60 T2 + final
cross-task reviewers. Members-reject now leaves pendingAction cleared
(modal closed, target/op reset) instead of stale.

Tests: new TestHandleOp{Obj,Loc,Player}U…ClearsPendingAction per
handler — pre-seed stale targetOp+target, invoke members-reject path,
assert post-call cleared.

Closes memory: nai_followups.md → "NAI-60 — CLOSED 2026-05-01" →
"TS-divergence surfaced (pre-existing, untracked at HEAD)" →
"ClearPendingAction-vs-members ordering in OpObjU/OpLocU/OpPlayerU".

Refs: TS OpObjUHandler.ts:68-74, OpLocUHandler.ts:68-74,
OpPlayerUHandler.ts:66-72; goscape handleOpNpcU at handler_opnpc.go:250.
```

## §6. Tracker delta

- **Retire:** none (no existing tracker entry).
- **Add (separate followup, not in this sub-spec):** new NAI-62 candidate
  for the broader **`targetSubject.com → typeId` dispatch override** plus
  **OpPlayerU `useObj`-as-4th-arg producer** divergence. Surfaced during
  NAI-61 brainstorm option-C exploration. Affects 5 trigger-lookup sites:
  `interaction_trigger.go:76, 146, 317, 377, 478, 535` and
  `player_interaction_trigger.go:55, 86`. TS `Player.getOpTrigger` /
  `getApTrigger` (Player.ts:993-997, 1027-1031) overrides `typeId` with
  `targetSubject.com` when `.com != -1`; goscape consumers ignore it. This
  changes WHICH scripts get selected for T-/U-handler dispatch — a real
  behavioural divergence that warrants its own brainstorm + spec + plan +
  test review (changes script selection). Append to `nai_followups.md` as
  part of NAI-61's close commit.

## §7. Out of scope

Explicitly NOT in this sub-spec:

1. The dispatch typeId-override divergence (see §6 — separate NAI-62).
2. OpPlayerU `setInteraction(useObj)` producer-side fix (subset of §6).
3. Any other doc-comment cleanups in the same files.
4. Refactoring the members-only check helper (see existing pattern at
   handler_opnpc.go:252-258 — same inline `if s.objTypes != nil && ...`
   block; consistent across all 4 handlers post-fix; no reason to extract).

## §8. Definition of done

- [ ] Three handlers patched per §3.1, §3.2, §3.3.
- [ ] Three doc-comment trailers updated per §3.4.
- [ ] Three new tests added per §4.1; each fails RED before fix, passes
      GREEN after.
- [ ] All existing `modules/world/...` tests pass post-fix.
- [ ] `go vet ./modules/world/...` clean.
- [ ] `go build ./...` succeeds.
- [ ] Single commit per §5.4 shape.
- [ ] `nai_followups.md` appended with `## NAI-61 — CLOSED YYYY-MM-DD`
      block + NAI-62-candidate carve-out per §6.
