# NAI-49: OPNPC Fidelity — npc.delayed + rsbuf.HasNpc + opcalled closure

**Date:** 2026-04-28  
**Tech stack:** Go 1.26+  
**Cadence:** Compressed (≤50 LOC production changes — spec and plan combined)

---

## Background

`handleOpNpc` / `handleOpNpcT` / `handleOpNpcU` are wired and tested but diverge from their TS counterparts in three ways:

| Gap | TS source | Status |
|-----|-----------|--------|
| `npc.delayed` gate | `OpNpcHandler.ts`, `OpNpcTHandler.ts`, `OpNpcUHandler.ts` | Missing |
| `rsbuf.hasNpc` visibility gate | same files | `HasNpc` exists in `pkg/rsbuf/buf.go:392`; not called |
| `player.opcalled = true` on success | all op handlers | `NAI-40-D-OPCALLED-MISSING`; field not yet declared |

`handleOpPlayer` already demonstrates both patterns: `s.rsbuf.HasPlayer` at handler_op_player.go:53, and the op-player deviation comment for opcalled awaiting closure.

---

## Spec

### Gate ordering (mirrors TS)

All three handlers adopt this ordering:

```
1. player.delayed             (existing)
2. payload length             (existing)
3. slot OOB                   (existing)
4. npc == nil || npc.dead     (existing)
5. npc.delayed                (NEW — T1)
6. !rsbuf.HasNpc(p, npc)      (NEW — T2)
7. op/typ validation          (existing; all three — Op[] check in handleOpNpc, typ==nil check in T/U)
8. success → opcalled = true  (NEW — T3)
```

### T1 — npc.delayed gate

Add after the `npc.dead` block in each of the three handlers (handler_opnpc.go):

```go
if npc.delayed && s.currentTick < npc.delayedUntil {
    sendUnsetMapFlag(p)
    return nil
}
```

### T2 — rsbuf.HasNpc visibility gate

Add after the npc.delayed block in each handler:

```go
if !s.rsbuf.HasNpc(int32(p.slot), int32(npc.nid)) {
    sendUnsetMapFlag(p)
    return nil
}
```

No deviation tag needed — this closes the gap (HasNpc exists in pkg/rsbuf/buf.go:392).

### T3 — opcalled field + cross-handler closure

**player.go:**
- Add `opcalled bool` to the session-flags group near `moveClickRequest` (player.go:184).
- Reset at top of `processIn` alongside the rate-limit resets (player.go:656): `p.opcalled = false`.

**On success path, add `p.opcalled = true` immediately after `p.ClearPendingAction()`** in:
- `handleOpNpc` (handler_opnpc.go:66) — covers OPNPC1–5
- `handleOpNpcT` (handler_opnpc.go:139) — covers OPNPCT
- `handleOpNpcU` (handler_opnpc.go:238) — covers OPNPCU
- `handleOpLoc` (handler_oploc.go:87) — covers OPLOC1–5
- `handleOpLocT` (handler_oploc.go:169) — covers OPLOCT
- `handleOpLocU` (handler_oploc.go:286) — covers OPLOCU
- `handleOpPlayer` (handler_op_player.go:58) — covers OPPLAYER1–4
- `handleOpPlayerT` (handler_op_player.go:121) — covers OPPLAYERT
- `handleOpPlayerU` (handler_op_player.go:207) — covers OPPLAYERU

**Remove deviation comments** `NAI-40-D-OPCALLED-MISSING` from handler_op_player.go (lines 21–24, 86, 144).

**No change** to doc-comments in handler_opnpc.go or handler_oploc.go — they had no opcalled deviation comment.

### Deviation tracking

No new deviations created. T1 and T2 close existing gaps. T3 closes `NAI-40-D-OPCALLED-MISSING`.

---

## Test strategy

### Fixture update (`makeOpNpcFixture` in handler_opnpc_test.go)

The existing fixture creates `p` but doesn't register it in rsbuf. Adding the HasNpc gate means all tests using `p` directly now require rsbuf wiring. Update `makeOpNpcFixture`:

```go
p.slot = 1
s.players[1] = p
s.rsbuf.AddPlayer(1)
s.rsbuf.SubscribeNpcForTest(1, int32(npc.nid)) // nid=1
```

Add helper (mirrors handler_op_player_test.go's `rsbufSeesNpc`):

```go
// rsbufSeesNpc makes s.rsbuf.HasNpc(playerSlot, nid) return true.
func rsbufSeesNpc(t *testing.T, s *Server, playerSlot, nid int) {
    t.Helper()
    s.rsbuf.AddPlayer(int32(playerSlot))
    s.rsbuf.SubscribeNpcForTest(int32(playerSlot), int32(nid))
}
```

### Tests needing rsbuf setup for gates AFTER HasNpc

These tests create a separate `p2` (or fresh `p`) that isn't in rsbuf, but test a gate that comes AFTER HasNpc. They would reject for the wrong reason without the fix:

- `TestHandleOpNpc1HiddenOpSendsUnsetMapFlag` — uses `p2` with slot unset. Fix: `p2.slot = 2; rsbufSeesNpc(t, s, 2, 1)`.
- `TestHandleOpNpcOpIndexOutOfRange` — creates its own `p` with slot 0. Fix: `p.slot = 1; rsbufSeesNpc(t, s, 1, 1)`.

### New tests

**T1 tests (npc.delayed):**
```
TestHandleOpNpcDelayedNpcRejected      — npc.delayed=true, s.currentTick < npc.delayedUntil → UnsetMapFlag, target=nil
TestHandleOpNpcTDelayedNpcRejected     — same for handleOpNpcT
TestHandleOpNpcUDelayedNpcRejected     — same for handleOpNpcU
```

**T2 tests (rsbuf.HasNpc):**
```
TestHandleOpNpcNpcNotVisibleRejected   — npc exists, not dead, not delayed, but NOT in rsbuf → UnsetMapFlag, target=nil
TestHandleOpNpcTNpcNotVisibleRejected  — same for handleOpNpcT
TestHandleOpNpcUNpcNotVisibleRejected  — same for handleOpNpcU
```

**T3 tests (opcalled):**
```
TestHandleOpNpc1SetsOpcalled           — success path sets p.opcalled=true
TestHandleOpNpcRejectedDoesNotSetOpcalled — rejection path leaves p.opcalled=false
TestProcessInResetsOpcalled            — after processIn, p.opcalled reverts to false (needs: set opcalled=true before call, verify false after)
```

opcalled tests for Loc and Player handlers: one success-path pin per family is sufficient (OpLoc1, OpPlayer1) to confirm the field is set; exhaustive per-variant tests are unnecessary given it's a single assignment on the shared success path.

---

## Implementation plan

### Task 1 — npc.delayed + rsbuf.HasNpc in handler_opnpc.go (TDD)

**Red:** Write the 6 new tests listed above (3×delayed, 3×not-visible). All fail.

**Green:** In `handleOpNpc`, after the `npc.dead` block (line 53–56), insert:

```go
if npc.delayed && s.currentTick < npc.delayedUntil {
    sendUnsetMapFlag(p)
    return nil
}
if !s.rsbuf.HasNpc(int32(p.slot), int32(npc.nid)) {
    sendUnsetMapFlag(p)
    return nil
}
```

Identical blocks after each handler's `npc.dead` block:
- `handleOpNpcT`: after line 130–132
- `handleOpNpcU`: after line 198–200

**Fixture + regression fixes:**
- Update `makeOpNpcFixture` as described above (add `rsbufSeesNpc` helper + fixture wiring).
- Update `TestHandleOpNpc1HiddenOpSendsUnsetMapFlag` and `TestHandleOpNpcOpIndexOutOfRange` to add rsbuf setup.

All tests must pass after this task.

### Task 2 — opcalled field + cross-handler set (TDD)

**Red:** Write the 3 opcalled tests (opnpc1 sets, rejection doesn't, processIn resets). Write one pin each for OpLoc1 and OpPlayer1. All fail (field doesn't exist).

**Green (player.go):**
- Add `opcalled bool` to the `afkEventReady, moveClickRequest bool` line or immediately adjacent.
- In `processIn`, add `p.opcalled = false` alongside the rate-limit resets at line 656.

**Green (handler files) — set `p.opcalled = true` after `p.ClearPendingAction()` in each success path:**

handler_opnpc.go (3 sites):
```go
p.ClearPendingAction()
p.opcalled = true   // ← add
p.SetInteraction(...)
```

handler_oploc.go (3 sites): same pattern at lines 87, 169, 286.

handler_op_player.go (3 sites): same pattern at lines 58, 121, 207.
Then remove the `NAI-40-D-OPCALLED-MISSING` deviation comments (lines 21–24, 86, 144).

All tests must pass after this task.

### Task 3 — Close commit

```
chore(close): NAI-49 — npc.delayed + rsbuf.HasNpc + opcalled closure

Closes memory: nai_followups.md (NAI-40-D-OPCALLED-MISSING)
```

---

## Notes

- `processWalktrigger()` is a stub and `moveClickRequest` is unread in goscape, so `opcalled` has no live behavioral effect yet. The field establishes the data model for when those consumers land.
- `handleOpLocT/U` and `handleOpPlayerT/U` have existing component-registry deviations (S6m-D1/D2, NAI-40-D-COMPONENT-REGISTRY); those are unchanged by this spec.
- The `rsbuf.hasNpc` gate was previously listed as unimplementable in the brainstorm sketch — corrected here once `pkg/rsbuf/buf.go:HasNpc` was confirmed.
