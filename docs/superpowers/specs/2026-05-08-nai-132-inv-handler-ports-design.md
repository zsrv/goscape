# NAI-132 — INV_* handler ports (CHANGESLOT / DROPITEM / MOVEITEM_CERT / MOVEITEM_UNCERT) + MOVETOSLOT validator-backfill + Remove minmax

**Predecessor:** NAI-131 close (`1fb03c1`) §3 deferred set. NAI-131 listed 7 handlers as "no goscape implementation yet"; pre-flight at NAI-132 spec-write found `INV_MOVETOSLOT` IS implemented (handlers_inv.go:866) but lacks NAI-131's dual-protect/scope validators. Six TS opcodes remain dispatch-less: `OpBothMoveInv` (4301), `OpInvChangeSlot` (4304), `OpInvDropItemDelayed` (4310), `OpInvDropItem` (4311), `OpInvMoveItemCert` (4319), `OpInvMoveItemUncert` (4320). NAI-132 ports four of them; defers `BOTH_MOVEINV` and `INV_DROPITEM_DELAYED` to later sub-specs that build the prerequisite infrastructure.

**Tech stack:** Go 1.26+. No new dependencies.

## §1 — Scope

NAI-132 ships:

- **T1** — `INV_MOVETOSLOT` (4317) validator-backfill (NAI-131 spec-error fixup; existing handler missing dual gates).
- **T2** — `pkg/inventory/inventory.go` `Remove` minmax modernization (final NAI-126 carryover).
- **T3** — `INV_CHANGESLOT` (4304) full handler port.
- **T4** — `INV_MOVEITEM_UNCERT` (4320) full handler port.
- **T5** — `INV_MOVEITEM_CERT` (4319) full handler port (adds overflow-to-world drop).
- **T6** — `INV_DROPITEM` (4311) full handler port (mirrors INV_DROPSLOT shape).

Estimated ~390 LOC including tests. Closes the "missing INV_* handler" backlog except for two intentionally deferred items below.

## §2 — Out of scope (deferred to later sub-specs)

- **`BOTH_MOVEINV` (4301)** → **NAI-133+ candidate**. TS InvOps.ts:373-495 indexes `ProtectedActivePlayer[secondary?1:0]` / `[secondary?0:1]` for per-pointer-slot protect tracking. Goscape's `s.Protect` is a single bool (state.go:315) shared across Self/Self2 ops. Porting BOTH_MOVEINV requires:
  1. New `Self2Protect bool` field on `ScriptState` (or `PtrProtectedActivePlayer2` Pointer flag).
  2. P_PROTECT routing on `state.intOperand` to set the appropriate slot.
  3. Then BOTH_MOVEINV: `!fromPlayerProtect` / `!toPlayerProtect` per gate.
  Plus DEVIATION-NAI-115-D1 wealth-event-tail skip (TS InvOps.ts:445-494 — addWealthEvent for dueloffer/STAKE and trade/TRADE; goscape skips per established pattern, content emits via OpWealthEvent 2131).
- **`INV_DROPITEM_DELAYED` (4310)** → **NAI-134+ candidate**. TS InvOps.ts:188-209 enqueues an `ObjDelayedRequest` onto `World.objDelayedQueue` (delayed-spawn queue with tick-driven flush). Goscape has no analog; spawning the delayed-obj queue infrastructure is a prerequisite. Audit for adjacent delayed-spawn primitives (npc-respawn, obj-respawn) at sub-spec-write time.

These two are tracked in `nai_followups.md` after NAI-132 close; do not port them in NAI-132.

## §3 — TS source (verbatim cites)

- INV_CHANGESLOT: `Engine-TS/src/engine/script/handlers/InvOps.ts:86-113`
- INV_MOVETOSLOT (validator-backfill target): `InvOps.ts:353-368`
- INV_DROPITEM: `InvOps.ts:163-186`
- INV_MOVEITEM_CERT: `InvOps.ts:535-566`
- INV_MOVEITEM_UNCERT: `InvOps.ts:570-597`

## §4 — Per-task design

### Task 1 — INV_MOVETOSLOT validator-backfill (`handlers_inv.go:866`)

Existing handler skips NAI-131's dual-protect/scope gates. TS InvOps.ts:356-364 chain: `InvTypeValid` × 2 → from-protect/scope → to-protect/scope (D1 — both gates evaluate `fromInvType.scope`). Add at the top of the existing function before any `resolveInv`:

```go
func handleInvMoveToSlot(s *ScriptState) error {
    if err := requireActivePlayer(s, "INV_MOVETOSLOT"); err != nil {
        return err
    }
    toSlot := s.PopInt()
    fromSlot := s.PopInt()
    toTypeID := s.PopInt()
    fromTypeID := s.PopInt()

    if err := checkInvType(s, fromTypeID, "INV_MOVETOSLOT"); err != nil {
        return err
    }
    if err := checkInvType(s, toTypeID, "INV_MOVETOSLOT"); err != nil {
        return err
    }

    fromInvType := s.Configs.InvType(fromTypeID)
    toInvType := s.Configs.InvType(toTypeID)

    // TS InvOps.ts:359-361 — from-protect gate uses fromInv.scope.
    if fromInvType.Protect && fromInvType.Scope != objtype.InvTypeScopeShared && !s.Protect {
        return fmt.Errorf("INV_MOVETOSLOT: $inv requires protected access: %s", fromInvType.DebugName)
    }
    // TS InvOps.ts:363-365 — to-protect gate ALSO uses fromInv.scope (DEVIATION-NAI-131-D1).
    if toInvType.Protect && fromInvType.Scope != objtype.InvTypeScopeShared && !s.Protect {
        return fmt.Errorf("INV_MOVETOSLOT: $inv requires protected access: %s", toInvType.DebugName)
    }

    // (existing snapshot/swap body unchanged below)
    ...
}
```

Existing GREEN test (handlers_inv_test.go) must continue to pass — fixture must satisfy the new gates (e.g., set both inv types' `Scope = InvTypeScopeShared` or set `s.Protect = true`).

### Task 2 — `inventory.Remove` minmax modernization (`pkg/inventory/inventory.go:291-321`)

Replace C-style guards with Go 1.21+ builtins:

```go
// Before (lines 297-300):
removed := 0
begin := opts.BeginSlot
if begin < 0 {
    begin = 0
}
// After:
removed := 0
begin := max(opts.BeginSlot, 0)

// Before (lines 306-309):
take := count - removed
if take > it.Count {
    take = it.Count
}
// After:
take := min(count-removed, it.Count)
```

~6 LOC delta. Mechanical refactor; no new tests (existing INV_DEL test paths cover Remove). Run `go test ./pkg/inventory/... ./pkg/script/... -count=1` after to confirm no regression.

### Task 3 — `INV_CHANGESLOT` (4304)

TS InvOps.ts:86-113. Pops `[inv, find, replace, replaceCount]` (popInts(4) order — replaceCount on top, inv at bottom). TS validator chain: `InvTypeValid` → protect/scope → `ObjTypeValid(find)` → `ObjTypeValid(replace)` → inv-resolve. **TS does not validate `replaceCount` via ObjStackValid** — pop-without-validate is intentional. Loop `inv.Capacity` for first slot whose item.Id == findObj.Id; on hit, write replace; no-match is a silent no-op.

```go
func handleInvChangeSlot(s *ScriptState) error {
    if err := requireActivePlayer(s, "INV_CHANGESLOT"); err != nil {
        return err
    }
    replaceCount := s.PopInt()
    replace := s.PopInt()
    find := s.PopInt()
    typeID := s.PopInt()

    if err := checkInvType(s, typeID, "INV_CHANGESLOT"); err != nil {
        return err
    }

    invType := s.Configs.InvType(typeID)
    if invType.Protect && invType.Scope != objtype.InvTypeScopeShared && !s.Protect {
        return fmt.Errorf("INV_CHANGESLOT: $inv requires protected access: %s", invType.DebugName)
    }

    if err := checkObjType(s, find, "INV_CHANGESLOT"); err != nil {
        return err
    }
    if err := checkObjType(s, replace, "INV_CHANGESLOT"); err != nil {
        return err
    }

    inv := resolveInv(s, typeID)
    if inv == nil {
        return fmt.Errorf("INV_CHANGESLOT: no inv for type %d", typeID)
    }

    findObj := s.Configs.ObjType(find)
    replaceObj := s.Configs.ObjType(replace)
    for slot := 0; slot < inv.Capacity; slot++ {
        it := inv.Get(slot)
        if it == nil {
            continue
        }
        if it.Id == findObj.Id {
            inv.Set(slot, &inventory.Item{Id: replaceObj.Id, Count: replaceCount})
            return nil
        }
    }
    return nil
}
```

Wire in `handlers.go`: `OpInvChangeSlot: handleInvChangeSlot`.

### Task 4 — `INV_MOVEITEM_UNCERT` (4320)

TS InvOps.ts:570-597. Pops `[fromInv, toInv, obj, count]` (popInts(4) — count on top). Validators: from/to InvTypeValid → ObjTypeValid → ObjStackValid → from-protect/scope → to-protect/scope (D1 dual-from-scope). Logic: `tx := fromInv.Remove(obj, count, RemoveOpts{BeginSlot:-1})`; if `tx.Completed == 0` return; if `objType.CertTemplate >= 0 && objType.CertLink >= 0` add `objType.CertLink` else add `objType.Id`. Use `lookupStackableStockObj` for both add paths. **No overflow drop** — TS just calls invAdd without the false-arg-overflow-handling.

```go
func handleInvMoveItemUncert(s *ScriptState) error {
    if err := requireActivePlayer(s, "INV_MOVEITEM_UNCERT"); err != nil {
        return err
    }
    count := s.PopInt()
    obj := s.PopInt()
    toTypeID := s.PopInt()
    fromTypeID := s.PopInt()

    if err := checkInvType(s, fromTypeID, "INV_MOVEITEM_UNCERT"); err != nil { return err }
    if err := checkInvType(s, toTypeID, "INV_MOVEITEM_UNCERT"); err != nil { return err }
    if err := checkObjType(s, obj, "INV_MOVEITEM_UNCERT"); err != nil { return err }
    if err := checkObjStack(count, "INV_MOVEITEM_UNCERT"); err != nil { return err }

    fromInvType := s.Configs.InvType(fromTypeID)
    toInvType := s.Configs.InvType(toTypeID)

    if fromInvType.Protect && fromInvType.Scope != objtype.InvTypeScopeShared && !s.Protect {
        return fmt.Errorf("INV_MOVEITEM_UNCERT: $inv requires protected access: %s", fromInvType.DebugName)
    }
    // DEVIATION-NAI-131-D1.
    if toInvType.Protect && fromInvType.Scope != objtype.InvTypeScopeShared && !s.Protect {
        return fmt.Errorf("INV_MOVEITEM_UNCERT: $inv requires protected access: %s", toInvType.DebugName)
    }

    fromInv := resolveInv(s, fromTypeID)
    if fromInv == nil { return fmt.Errorf("INV_MOVEITEM_UNCERT: no inv for from-type %d", fromTypeID) }
    toInv := resolveInv(s, toTypeID)
    if toInv == nil { return fmt.Errorf("INV_MOVEITEM_UNCERT: no inv for to-type %d", toTypeID) }

    tx := fromInv.Remove(obj, count, inventory.RemoveOpts{BeginSlot: -1})
    if tx.Completed == 0 {
        return nil
    }

    objType := s.Configs.ObjType(obj)
    finalObj := obj
    if objType.CertTemplate >= 0 && objType.CertLink >= 0 {
        finalObj = objType.CertLink
    }
    stackable, stockObj := lookupStackableStockObj(s, toInv.Type, finalObj)
    toInv.Add(finalObj, tx.Completed, inventory.AddOpts{
        BeginSlot: -1,
        Stackable: stackable,
        StockObj:  stockObj,
    })
    return nil
}
```

Wire in `handlers.go`: `OpInvMoveItemUncert: handleInvMoveItemUncert`.

### Task 5 — `INV_MOVEITEM_CERT` (4319)

TS InvOps.ts:535-566. Same pop+validator chain as T4. Logic: invDel → cert-resolve (`if CertTemplate==-1 && CertLink>=0 → finalObj=CertLink` — note **inverted** sense vs UNCERT) → invAdd → `overflow = count - tx.Completed`; if overflow > 0, single `World.AddObj(level, x, z, finalObj, overflow, 200, receiverID)` call (TS comment "should be a stackable cert already" — no per-item branch). Skip-guard `if s.World != nil` is goscape-defensive (NAI-130-D2 / `defensive_gate_doc_comment_label`).

```go
// (validator + invDel chain identical to T4 with op="INV_MOVEITEM_CERT")
// Cert-resolve:
finalObj := obj
if objType.CertTemplate == -1 && objType.CertLink >= 0 {
    finalObj = objType.CertLink
}
stackable, stockObj := lookupStackableStockObj(s, toInv.Type, finalObj)
tx2 := toInv.Add(finalObj, tx.Completed, inventory.AddOpts{
    BeginSlot: -1,
    Stackable: stackable,
    StockObj:  stockObj,
})

overflow := count - tx2.Completed
if overflow > 0 && s.World != nil {
    level := (s.Self.CoordPacked() >> 28) & 0x3
    receiverID := s.Self.UID()
    s.World.AddObj(level, s.Self.X(), s.Self.Z(), finalObj, overflow, 200, receiverID)
}
return nil
```

Wire in `handlers.go`: `OpInvMoveItemCert: handleInvMoveItemCert`.

### Task 6 — `INV_DROPITEM` (4311)

TS InvOps.ts:163-186. Pops `[inv, coord, obj, count, duration]` (popInts(5) — duration on top, inv at bottom). Validators: InvTypeValid → CoordValid → ObjTypeValid → ObjStackValid → DurationValid → protect/scope. Logic: mirror INV_DROPSLOT (handlers_inv.go:771) but with `inv.Remove(obj, count, ...)` instead of slot-scoped delete. Stackable per-item-or-stacked branching identical to NAI-115 patterns. Set `s.ActiveObj` + `s.Pointers |= PtrActiveObj` after each spawn (last-wins semantics from TS InvOps.ts:184-185).

```go
func handleInvDropItem(s *ScriptState) error {
    if err := requireActivePlayer(s, "INV_DROPITEM"); err != nil {
        return err
    }
    duration := s.PopInt()
    count := s.PopInt()
    obj := s.PopInt()
    coord := s.PopInt()
    invID := s.PopInt()

    if err := checkInvType(s, invID, "INV_DROPITEM"); err != nil { return err }
    level, x, z, err := checkCoord(coord, "INV_DROPITEM")
    if err != nil { return err }
    if err := checkObjType(s, obj, "INV_DROPITEM"); err != nil { return err }
    if err := checkObjStack(count, "INV_DROPITEM"); err != nil { return err }
    if err := checkDuration(duration); err != nil {
        return fmt.Errorf("INV_DROPITEM: %w", err)
    }

    invType := s.Configs.InvType(invID)
    if invType.Protect && invType.Scope != objtype.InvTypeScopeShared && !s.Protect {
        return fmt.Errorf("INV_DROPITEM: $inv requires protected access: %s", invType.DebugName)
    }

    inv := resolveInv(s, invID)
    if inv == nil { return fmt.Errorf("INV_DROPITEM: inv unresolved (id=%d)", invID) }
    tx := inv.Remove(obj, count, inventory.RemoveOpts{BeginSlot: -1})
    completed := tx.Completed
    if completed == 0 {
        return nil
    }
    if s.World == nil {
        return fmt.Errorf("INV_DROPITEM: no world surface")
    }
    objType := s.Configs.ObjType(obj)
    receiverID := s.Self.UID()
    if !objType.Stackable || completed == 1 {
        for range completed {
            o := s.World.AddObj(level, x, z, obj, 1, duration, receiverID)
            if o != nil {
                s.ActiveObj = o
                s.Pointers |= PtrActiveObj
            }
        }
    } else {
        o := s.World.AddObj(level, x, z, obj, completed, duration, receiverID)
        if o != nil {
            s.ActiveObj = o
            s.Pointers |= PtrActiveObj
        }
    }
    return nil
}
```

Wire in `handlers.go`: `OpInvDropItem: handleInvDropItem`.

## §5 — Test strategy

Per-task RED-then-GREEN per project cadence (`runescript_cadence`). Reuse `runInvOpExpectErrAsPlayer` from NAI-131 T0 where possible. Each gate test:

1. Constructs a `ScriptState` initialized to violate exactly one gate (or to satisfy all as a regression-pin GREEN).
2. Runs `Execute` on a single-opcode `ScriptFile`.
3. Asserts the error literal contains the gate-specific TS-shape substring.

### Per-gate assertion patterns

- **No active player** → `"<OP>: no active player"` (T1, T3-T6).
- **InvTypeValid** → `"no InvType with value (N) found"` (all tasks).
- **ObjTypeValid** → `"no ObjType with value (N) found"` (T3 ×2, T4-T6 ×1).
- **ObjStackValid** → `"invalid count (N)"` (T4-T6; not T3 — TS skips this gate per §4 T3).
- **CoordValid (T6)** → `"INV_DROPITEM: coord out of range (N)"` (literal from `checkCoord` at handlers_npc.go:13-19; takes `op` arg → already prefixed).
- **DurationValid (T6)** → `"INV_DROPITEM: duration out of range [1, 2147483647]: N"` (literal from `checkDuration` at handlers_loc.go:278-283; does NOT take `op` — handler wraps via `fmt.Errorf("INV_DROPITEM: %w", err)` per existing INV_DROPSLOT pattern at handlers_inv.go:789).
- **Protect/scope** → `"<OP>: $inv requires protected access: <debugname>"`.
- **D1 absence-pin (T1, T4, T5):** toInvType.Scope=Shared but fromInvType.Scope=Perm → still rejects (`fromInvType.scope` is the live gate); presence-and-absence per `ts_asymmetry_dual_pin`.

### Per-handler GREEN regression pins

- **T1 INV_MOVETOSLOT** — existing swap test must continue passing after validator addition. Patch fixture: set both inv types to `Scope = InvTypeScopeShared` OR set `s.Protect = true`.
- **T3 INV_CHANGESLOT** — pin (a) hit-on-first-match returns early (slots after match unchanged), (b) no-match leaves inv unchanged (silent no-op), (c) replaceCount=0 still writes (TS skips ObjStackValid — absence-pin via test).
- **T4 INV_MOVEITEM_UNCERT** — pin: (a) non-cert obj → invAdd uses obj.Id (CertTemplate=-1, CertLink=-1 path), (b) cert obj (CertTemplate>=0, CertLink>=0) → invAdd uses CertLink, (c) UNCERT-stackable cert overflow does NOT drop to world (no overflow-drop branch).
- **T5 INV_MOVEITEM_CERT** — pin: (a) non-cert obj (CertTemplate=-1, CertLink=-1) → invAdd uses obj.Id (no transform); (b) certable obj (CertTemplate=-1, CertLink>=0) → finalObj=CertLink; (c) overflow > 0 → single World.AddObj call with overflow count (NOT per-item). Inverted-cert-sense vs T4 must be tested with explicit fixtures.
- **T6 INV_DROPITEM** — pin: (a) Stackable=true && completed>1 → single World.AddObj(count=completed); (b) Stackable=false → completed-many AddObj(count=1); (c) Stackable=true && completed=1 → single AddObj(count=1) via the `completed == 1` branch (fall-through to per-item); (d) `s.ActiveObj` and `PtrActiveObj` pointer set after spawn (last-wins for the per-item branch); (e) `tx.Completed == 0` → no World.AddObj.

### Test fixture seeds (T0-style preflight)

- ObjType fixtures need `CertTemplate=-1`, `CertLink=N` pairs for T4/T5 GREEN tests. Confirmed `pkg/objtype/objtype.go:124-125` field shape; default seed produces CertLink=-1, CertTemplate=-1 (`pkg/objtype/objtype.go:300`).
- InvType fixtures with both `Scope=Perm`+`Protect=true` AND `Scope=Shared`+`Protect=true` for D1 dual-pin tests.

Estimated test count: **~30-40 tests** across T1-T6 (T1 ~6, T3 ~7, T4 ~8, T5 ~8, T6 ~10).

## §6 — Anticipated DEVIATIONs

- **DEVIATION-NAI-131-D1 (inherited; applies to T1, T4, T5):** TS asymmetry — both protect/scope gates check `fromInvType.scope`, never `toInvType.scope`. Pin via dual-presence + absence-pin pattern per `ts_asymmetry_dual_pin`. Re-tagged for NAI-132 in doc comments.

- **DEVIATION-NAI-130-D2 (inherited; applies to T5, T6):** defensive `s.World != nil` skip-guard around overflow drops / world spawns (goscape defensive; TS uses static World import which is never null). Doc-comment labeled per `defensive_gate_doc_comment_label`.

- **DEVIATION-NAI-130-D3 (inherited; applies to T4, T5):** `lookupStackableStockObj` defensive nil-Configs fallback (unreachable post-checkObjType/checkInvType but retained for sibling callers).

No new D-tags anticipated for NAI-132 itself. If new divergences surface during implementation, route via `true_to_ts_gate` (track every divergence with rationale + follow-up).

## §7 — Risk register (for plan-author / controller pre-flight)

| Risk | Mitigation |
|---|---|
| INV_CHANGESLOT TS lacks `check(replaceCount, ObjStackValid)` — gate gap relative to siblings | Verified at spec-write (TS InvOps.ts:86-113 — only InvTypeValid + protect/scope + ObjTypeValid×2). Document as deliberate "TS-faithful pop-without-validate"; add absence-pin GREEN test for `replaceCount=0`. |
| `inv.Set(slot, &Item{Id, Count})` for INV_CHANGESLOT — does goscape's `inv.Set` mirror TS `invSet(typeId, objId, count, slot)` at the type-level? | Goscape's `inv.Set(slot, *Item)` is the single-slot setter (inventory.go:144). TS `invSet` is at the Player level and forwards to inv. Direct-Set is the goscape equivalent. Verified at spec-write. |
| ObjType default seed has `CertLink=-1, CertTemplate=-1` (pkg/objtype/objtype.go:300) — would T4/T5 cert-branch tests trigger? | Test fixtures must explicitly populate CertTemplate/CertLink. Plan-author preflight: grep `Configs.ObjType` test seeds for CertTemplate field. |
| `inv.Type` field exists for `lookupStackableStockObj` arg — verify | Confirmed: `inventory.Inventory.Type int` exists (pkg/inventory/inventory.go); used in handleInvAdd at handlers_inv.go:380 (`lookupStackableStockObj(s, inv.Type, obj)`). |
| `s.Self.CoordPacked()>>28&0x3` for level extraction (T5 overflow, T6 drop) | Established pattern (handlers_inv.go:367-381). No risk. |
| `checkCoord` / `checkDuration` helper signatures and error literals | `checkCoord(coord int, op string) (level, x, z int, err error)` at handlers_obj.go (used in handleInvDropSlot:793). `checkDuration(d int) error` at handlers_obj.go (used in handleInvDropSlot:789 wrapped via `%w`). Verify literals at plan-write. |
| Fixture churn: T1 validator-backfill breaks existing INV_MOVETOSLOT GREEN test | T1 implementer patches the existing test fixture as part of T1 (single-task scope); full `go test ./pkg/script/... -count=1` after each task. |
| `inventory.Remove` modernization changes behavior with negative `BeginSlot` | `max(opts.BeginSlot, 0)` is bit-identical to the existing C-style guard. Mechanical-equivalent refactor; existing INV_DEL tests are sufficient regression coverage. |

## §8 — Plan-author notes (future writing-plans pass)

- Plan tasks ordered T1, T2, T3, T4, T5, T6 — each task is self-contained with no inter-task dependencies (T2 minmax has no consumer-side change).
- Each port task must be RED-then-GREEN: write failing tests for each gate first, then add the gate. Pattern from NAI-131.
- Pre-flight grep before each task dispatch (per `controller_preflight`):
  - **T1**: read `handlers_inv_test.go` for existing INV_MOVETOSLOT test fixture; identify the GREEN regression pin and the patch needed.
  - **T2**: confirm `pkg/inventory/inventory.go:291-321` Remove signature unchanged; no other callers of `RemoveOpts.BeginSlot`.
  - **T3**: confirm `inv.Set(slot, *Item)` and `inv.Get(slot) *Item` semantics; no surprise side-effects in `inv.Set` beyond slot-write.
  - **T4 / T5**: read `pkg/objtype/objtype.go:124-125, 300` for CertTemplate/CertLink defaults; verify `lookupStackableStockObj(s, inv.Type, finalObj)` arg shape.
  - **T6**: read `handleInvDropSlot` (handlers_inv.go:771) for the canonical World.AddObj per-item-vs-stacked branch shape; verify `checkCoord` / `checkDuration` literals.
- Verify field names against HEAD (per `mock_recorder_field_naming_check` and `plan_type_name_grep`):
  - `objtype.InvType.Protect`, `Scope`, `DummyInv` — confirmed at NAI-131 spec-write (pkg/objtype/invtype.go:13, 18, 26, 28).
  - `objtype.ObjType.DebugName`, `Stackable`, `CertLink`, `CertTemplate`, `DummyItem` — confirmed at NAI-132 spec-write.
  - `inventory.AddOpts{BeginSlot, Stackable, StockObj}` and `RemoveOpts{BeginSlot}` — confirmed.
- Per `plan_doc_replaceall_timeline`: NAI-132 plan doc may have multiple "INV_X" identifier shapes per task — use per-instance Edits with task-section context, not `replace_all`.

## §9 — Closing criteria

1. T1-T6 each RED→GREEN per cadence.
2. `go test ./pkg/script/... ./pkg/inventory/... ./modules/world/... -count=1` passes after T6.
3. No new D-tags beyond the inherited D1/D2/D3 set (anticipated). Any new divergence tracked via `true_to_ts_gate`.
4. Close commit follows `close_commit_memory_trailer` (NAI-15+) format with `Closes memory:` trailer for any new memory entries.
5. Update `nai_followups.md` to reflect:
   - NAI-132 close (this sub-spec).
   - **NAI-133+ candidate**: BOTH_MOVEINV (Self2-protect infra prerequisite documented in §2 above).
   - **NAI-134+ candidate**: INV_DROPITEM_DELAYED (delayed-obj queue infra prerequisite documented in §2 above).
   - Retire the `inventory.Remove` minmax carryover (closed by T2).
6. No smoke required — TS-fidelity polish + new-handler ports. Scripts that previously dispatched the missing opcodes would have errored at the dispatch table miss; any content that genuinely depended on the missing-handler error path would surface in a future smoke as a behavior change.
