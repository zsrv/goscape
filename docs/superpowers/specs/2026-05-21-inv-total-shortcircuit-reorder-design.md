# INV_TOTAL `obj == -1` short-circuit reorder — design

**Date:** 2026-05-21
**Predecessor:** [[handlers-inv-readside-checkinvtype-wiring-close]] at HEAD `48b05865`
**Size:** XS (~4 lines of motion + 1 test)
**Cadence:** in-session main thread, no subagent dispatch (per [[ai-queue-fencepost-tighten-close]] / [[doc-comment-sweep-close]] XS precedent)

## 1. Motivation

The 12-site `checkInvType` wiring slice closed at HEAD `48b05865` surfaced (via opus whole-slice review) a TS-faithfulness drift in `handleInvTotal`: the `obj == -1` short-circuit runs **before** the registry validator, but the TS source runs the validator **first**. The drift was pre-existing in goscape, not introduced by the wiring slice; the wiring slice preserved the pre-existing ordering rather than fixing it inline (correctly — out of slice scope at the time, and resume-memo for that slice asserted the wrong TS ordering claim across spec/plan/resume-memo chain).

This slice retires the drift with a 4-line motion + 1 test.

## 2. TS truth (source of record)

`/home/owner/Code/github.com/LostCityRS/Engine-TS/src/engine/script/handlers/InvOps.ts:619-631`:

```ts
[ScriptOpcode.INV_TOTAL]: checkedHandler(ActivePlayer, state => {
    const [inv, obj] = state.popInts(2);

    const invType: InvType = check(inv, InvTypeValid);   // :622 — FIRST

    // todo: error instead?
    if (obj === -1) {                                     // :625 — SECOND
        state.pushInt(0);
        return;
    }

    state.pushInt(state.activePlayer.invTotal(invType.id, obj));
}),
```

Order: pop → `check(inv, InvTypeValid)` → `obj === -1` short-circuit → `invTotal` push.

TS comment `// todo: error instead?` at `:624` confirms the TS author noted the `obj === -1 → push 0` short-circuit oddity but ordered registry validation first regardless.

## 3. Current goscape state (drift)

`/home/owner/Code/github.com/zsrv/goscape/pkg/script/handlers_inv.go:26-45` at HEAD `48b05865`:

```go
func handleInvTotal(s *ScriptState) error {
    obj := s.PopInt()
    typeID := s.PopInt()
    // TS INV_TOTAL short-circuits with obj == -1 → push 0.
    if obj == -1 {                                        // currently FIRST — drift
        s.PushInt(0)
        return nil
    }
    if err := checkInvType(s, typeID, "INV_TOTAL"); err != nil {  // currently SECOND — drift
        return err
    }
    inv := resolveInv(s, typeID)
    if inv == nil {
        // Defensive: unreachable post-checkInvType for valid configs;
        // retained for the InvLookup-unset case (s.Inv == nil → resolveInv returns nil).
        return fmt.Errorf("INV_TOTAL: no inv for type %d", typeID)
    }
    s.PushInt(inv.GetItemCount(obj))
    return nil
}
```

Observable behavior of drift: when an unregistered `typeID` arrives together with `obj == -1`, TS throws canonical `"no InvType with value (X) found"`; goscape silently pushes 0 and returns nil. After this slice, both ports return the canonical registry-miss error for that input shape.

## 4. Target state

```go
func handleInvTotal(s *ScriptState) error {
    obj := s.PopInt()
    typeID := s.PopInt()
    if err := checkInvType(s, typeID, "INV_TOTAL"); err != nil {  // FIRST
        return err
    }
    // TS INV_TOTAL short-circuits with obj == -1 → push 0.
    if obj == -1 {                                                // SECOND
        s.PushInt(0)
        return nil
    }
    inv := resolveInv(s, typeID)
    if inv == nil {
        // Defensive: unreachable post-checkInvType for valid configs;
        // retained for the InvLookup-unset case (s.Inv == nil → resolveInv returns nil).
        return fmt.Errorf("INV_TOTAL: no inv for type %d", typeID)
    }
    s.PushInt(inv.GetItemCount(obj))
    return nil
}
```

Exact body order: pop → `checkInvType` → `obj == -1` short-circuit → `resolveInv` + defensive nil → `GetItemCount` push.

The `// TS INV_TOTAL short-circuits...` comment moves with its block. No new comments added.

## 5. Scope guardrails (non-goals)

1. **Do NOT touch `handleInvItemSpace` (INV_ITEMSPACE) or `handleInvItemSpace2` (INV_ITEMSPACE2).** TS `InvOps.ts:289-292` and `:309-312` short-circuit on `count === 0` **before** `check(inv, InvTypeValid)`. Goscape mirrors that ordering and is TS-faithful as-is. Confirmed in [[handlers-inv-readside-checkinvtype-wiring-close]] (predecessor close memo, non-obvious finding: TS itself is internally inconsistent about short-circuit-vs-validator ordering across opcodes in the same file — verify each opcode individually).

2. Do NOT touch other handlers in `handlers_inv.go`.

3. Do NOT touch `resolveInv` / `checkInvType` helpers or their signatures.

4. Do NOT modify existing tests.

## 6. Test plan

### New test

`pkg/script/handlers_inv_test.go` — append (after the existing `TestInvAddThenTotal` cluster, or in any logical position near other INV_TOTAL tests):

```go
func TestInvTotal_UnknownInv_ObjNeg1_RejectsRegistry(t *testing.T) {
    lookup := newTestInvLookup()
    mc := newTestInvConfigs()
    // Unregistered typeID (9999) + obj == -1. TS InvOps.ts:622-625 runs
    // check(inv, InvTypeValid) BEFORE the obj === -1 short-circuit, so an
    // invalid inv id must produce the canonical registry-miss error rather
    // than the silent obj==-1 → push 0 fallthrough.
    runInvOpExpectErrAsPlayer(t, OpInvTotal, []int{9999, -1}, lookup, mc, "no InvType with value (9999) found")
}
```

Helper pattern mirrors `TestInvMoveToSlot_FromInvTypeInvalid` at `pkg/script/handlers_inv_test.go:460-464` (substring assertion against canonical registry-miss wording).

### Existing happy-path coverage

`TestInvAddThenTotal` at `pkg/script/handlers_inv_test.go:244` exercises the registered-InvType + valid-obj path; remains green post-reorder (registry check passes, short-circuit not taken, `GetItemCount` returns 42).

`TestInvDel` at `:256` and other INV_TOTAL-as-readback tests likewise remain green — none use `obj == -1` with an unregistered InvType.

## 7. Gates

- `GOROOT=/home/owner/go/go1.26.3 GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache /home/owner/go/go1.26.3/bin/go test -race ./...` — all packages PASS.
- `GOROOT=/home/owner/go/go1.26.3 GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache /home/owner/go/go1.26.3/bin/go test -run TestPackAll_TwelveStageSmoke ./pkg/packall/...` — PASS.
- `GOROOT=/home/owner/go/go1.26.3 GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache /home/owner/go/go1.26.3/bin/gofmt -l pkg/script/handlers_inv.go pkg/script/handlers_inv_test.go` — empty.
- Audit-grep:
  - `grep -n 'checkInvType(s, ' pkg/script/handlers_inv.go | wc -l` → 12 (unchanged from HEAD; no new wires, just reorder of one existing call).
  - `grep -n 'INV_TOTAL' pkg/script/handlers_inv.go` → matches structurally consistent with target body.

## 8. Cross-session insights applied

1. **"Show me the TS code we're porting" at brainstorm-time** — performed against `InvOps.ts:619-631` at brainstorm start; spec §2 includes the verbatim TS snippet.
2. **TS internally inconsistent about short-circuit-vs-validator ordering across opcodes in the same file** — spec §5.1 explicitly carves out ITEMSPACE/ITEMSPACE2 as TS-faithful with short-circuit-first ordering; do NOT generalize this slice's reordering to them.
3. **Reviewer-prompt "expected N" counts must account for pre-existing instances** — spec §7 audit-grep gates use absolute counts (12 for `checkInvType`) rather than deltas.

## 9. Risks

- **None substantive.** Pure 4-line motion within a single function body; semantics change only for the narrow invalid-typeID + `obj == -1` input shape, which the new test pins. All other input shapes preserve byte-identical behavior (registered InvType + obj == -1 still pushes 0; registered InvType + valid obj still pushes `GetItemCount`).

## 10. Carry-forward post-close

After this slice ships, the next-pivot menu reduces to:
1. NAI-162 analytics RPC.
2. Combat-level read-site verification.
3. Deviation audit refresh.
4. General world/runescript engine work.
5. OC_* Part B + most NC_* bespoke-unknown-id error test coverage gap (low priority).
6. Other type-registry validator-vs-resolver gaps (LocType/NpcType/ObjType read-side opcodes — possible audit slice per [[inv-type-opcode-wiring-audit-phantom-gap]] finding #3).
