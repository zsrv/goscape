# NAI-153 — pickup-chain cascade-tail (OBJ_COUNT + OBJ_TAKEITEM)

## 1. Scope

Successor to NAI-152 B2 (`06f4571`). B2 PRIMARY closed (handler + reach
landed); the resulting Java-client mindrune-pickup smoke surfaced two
downstream blockers on the same pickup chain — both `[label,pickup_obj_floor]`
and `[label,pickup_obj_table]` now crash on `OBJ_COUNT (opcode 3503) at pc=1`.
Per the NAI-152 close commit and `nai_followups.md`
NAI-152-B2-FOLLOWUP-NAI-153 entry, the cascade-tail is two opcodes that
ride a shared `ActiveObj` surface extension:

1. **`OpObjCount` (3503)** — declared at `pkg/script/opcode.go:311`,
   no handler. TS `ObjOps.ts:121-130`: pushes `obj.count` if
   `obj.isValid(state.activePlayer.hash64)`, else `0`.
2. **`OpObjTakeItem` (3510)** — declared at `pkg/script/opcode.go:318`,
   no handler. TS `ObjOps.ts:137-161`: pops `invType`, `obj.isValid`
   guard, `activePlayer.invAdd(invType.id, obj.type, obj.count)`,
   `addWealthEvent(...)`, lifecycle-branched `World.removeObj(...)`.

Both share the same `ActiveObj`-surface extension (`ObjCount`, `IsValidFor`)
plus a `performInvAdd` helper extraction so OBJ_TAKEITEM can call into
the existing `handleInvAdd` validation chain without going through
PopInt-driven dispatch. Per the seed entry, splitting into separate
sub-specs wastes review cycles. Single bundle, four T-tasks. Smoke-gated
close on the deferred B2 §10 acceptance (mindrune-in-inventory + ground
tile clears).

## 2. Tech stack

Go 1.26+. No new deps. Touches `pkg/script` (T1, T2 interface, T3, T4),
`pkg/entity` (T2 methods on `*Obj`), and the existing `mockActiveObj`
test fixtures in `pkg/script/handlers_obj_test.go` and
`pkg/script/handlers_vars_test.go`. Pure refactor (T1) plus three new
handler wirings; no new module surface, no protocol-layer changes.

## 3. TS source

- **T1 reuse target — `Player.invAdd`:** TS `Engine-TS/src/engine/entity/Player.ts`
  defines `invAdd(invID, objID, count)` as the shared inventory-mutation
  impl that both INV_ADD and OBJ_TAKEITEM call. goscape's equivalent is
  `handleInvAdd` (`pkg/script/handlers_inv.go:318+`); T1 extracts the
  body into `performInvAdd` so OBJ_TAKEITEM can call it directly.

- **T3 OBJ_COUNT handler:** `Engine-TS/src/engine/script/handlers/ObjOps.ts:121-130`
  ```ts
  [ScriptOpcode.OBJ_COUNT]: state => {
      const obj: Obj = state.activeObj;
      if (obj.isValid(state.activePlayer.hash64)) {
          state.pushInt(state.activeObj.count);
          return;
      }
      state.pushInt(0);
  },
  ```

- **T4 OBJ_TAKEITEM handler:** `Engine-TS/src/engine/script/handlers/ObjOps.ts:137-161`
  ```ts
  [ScriptOpcode.OBJ_TAKEITEM]: state => {
      const invType: InvType = check(state.popInt(), InvTypeValid);
      const obj: Obj = state.activeObj;
      const objType = ObjType.get(obj.type);

      if (!obj.isValid(state.activePlayer.hash64)) {
          return false;
      }

      state.activePlayer.invAdd(invType.id, obj.type, obj.count);

      const value = obj.count * objType.cost;
      state.activePlayer.addWealthEvent({
          event_type: WealthEventType.PICKUP,
          account_items: [{ id: objType.id, name: objType.debugname, count: obj.count }],
          account_value: value
      });

      if (obj.lifecycle === EntityLifeCycle.RESPAWN) {
          World.removeObj(obj, objType.respawnrate);
      } else if (obj.lifecycle === EntityLifeCycle.DESPAWN) {
          World.removeObj(obj, 0);
      }
  },
  ```

- **T2 `Obj.isValid`:** `Engine-TS/src/engine/entity/Obj.ts:52-62`
  ```ts
  isValid(hash64?: bigint): boolean {
      if (this.reveal > -1 && hash64 && hash64 !== this.receiver64) {
          return false;
      }
      if (this.count < 1) {
          return false;
      }
      return super.isValid();
  }
  ```

## 4. Existing surface

### 4.1. T1 (`performInvAdd` extraction)

- `pkg/script/handlers_inv.go:318+` — `handleInvAdd` body. Validation
  chain (`InvTypeValid → ObjTypeValid → ObjStackValid → protect/scope →
  dummyitem`), `resolveInv`, `lookupStackableStockObj`, `Inventory.Add`,
  overflow drop via `s.World.AddObj`. The body is self-contained — only
  the leading three `PopInt` calls differ from what OBJ_TAKEITEM needs.
- Existing `handleInvAdd` tests in `pkg/script/handlers_inv_test.go`
  cover the full chain via PopInt-driven dispatch; they continue to pass
  unchanged after T1 since the wrapper preserves exact behavior.

### 4.2. T2 (ActiveObj surface + `*entity.Obj` methods)

- `pkg/script/active.go:910-913` — current `ActiveObj` interface (only
  `ObjType()` and `Coords()`).
- `pkg/entity/obj.go:8-29` — `Obj` struct with `Count int`, `Reveal int`,
  `ReceiverID int`, `Lifecycle Lifecycle` fields.
- `pkg/entity/obj.go:59-61` — existing no-arg `IsValid() bool` returning
  `true` (intrinsic-validity base; zone-membership lives in world module).
- `pkg/entity/obj.go:66` — existing `ObjType() int` method (precedent
  for the field-method-collision rename pattern; see §6.3).
- `pkg/script/handlers_obj_test.go` and `pkg/script/handlers_vars_test.go`
  define `mockActiveObj` test fixture; both need extending.

### 4.3. T3 (`handleObjCount`)

- `pkg/script/opcode.go:311` — `OpObjCount Opcode = 3503` declared.
- `pkg/script/handlers.go:120-127` — OBJ family map block; missing
  `OpObjCount: handleObjCount` entry.
- `pkg/script/handlers_obj.go:11-17` — `requireActiveObj(s, op)` helper.
- `pkg/script/handlers_inv.go:319` — `requireActivePlayer(s, op)` helper.
- `pkg/script/active.go:444` — `Self.UID() int` already on the surface.

### 4.4. T4 (`handleObjTakeItem`)

- `pkg/script/opcode.go:318` — `OpObjTakeItem Opcode = 3510` declared.
- Same OBJ family map block as T3.
- `pkg/script/handlers_obj.go:11-17, 19-26` — `requireActiveObj`,
  `checkObjType`, plus pattern from `handleObjAdd` for `requireActivePlayer`
  + `s.World == nil` guards.
- `pkg/script/handlers_inv.go` — `checkInvType(s, id, op)` helper for
  the popped invType validation.
- `pkg/script/handlers_obj.go:117-121` — existing NAI-115-D2 deviation
  doc-comment on `handleObjDel`; T4 extends to mention TAKEITEM.
- `pkg/script/handlers_obj_test.go:31-40` — existing
  `fakeWorldRemoveObj` recorder reusable by T4 tests.

## 5. Bundle design

Single bundle, four T-tasks. T1 and T2 are independent (controller may
dispatch in parallel per `dispatching-parallel-agents`). T3 depends on
T2; T4 depends on T1, T2, T3.

### 5.1. T1 — Extract `performInvAdd` (`pkg/script/handlers_inv.go`)

**Refactor (zero behavior change):**

```go
// performInvAdd is the shared invAdd impl. Mirrors TS Player.invAdd —
// the method both INV_ADD opcode and OBJ_TAKEITEM call. Validates
// invType + objType + count, enforces protect/scope + dummyitem gates,
// resolves the inv, and routes via Inventory.Add (with overflow drop).
//
// Pre-conditions: requireActivePlayer was called by the opcode handler.
// Inputs are raw script ints; performInvAdd does its own check chain so
// each call site stays minimal.
func performInvAdd(s *ScriptState, invID, objID, count int, op string) error {
    // verbatim copy of the existing handleInvAdd body from the first
    // checkInvType call through the overflow-drop block.
}

func handleInvAdd(s *ScriptState) error {
    if err := requireActivePlayer(s, "INV_ADD"); err != nil {
        return err
    }
    count := s.PopInt()
    obj := s.PopInt()
    typeID := s.PopInt()
    return performInvAdd(s, typeID, obj, count, "INV_ADD")
}
```

Plan-author Reads `handlers_inv.go:318-end` line-by-line and copies the
body verbatim into `performInvAdd`; only the three `PopInt` calls and
the `requireActivePlayer` guard stay in the wrapper. Per
`audit_full_method_when_restructuring.md`.

**LOC:** ~10 net delta + ~25 new test.

### 5.2. T2 — ActiveObj surface + `*entity.Obj` methods

**Production change (`pkg/script/active.go:910-913`):**

```go
// ActiveObj is the surface that OBJ_* and AI_APOBJ/AI_OPOBJ handlers
// use to read obj state. Narrow by design — extend as future sub-specs
// wire more obj script opcodes.
type ActiveObj interface {
    ObjType() int                  // underlying ObjType id
    Coords() (x, z, level int)     // world position
    ObjCount() int                 // current stack size; consumed by OBJ_COUNT, OBJ_TAKEITEM
    IsValidFor(playerUID int) bool // private-receiver + count>0; consumed by OBJ_COUNT, OBJ_TAKEITEM
}
```

**Production change (`pkg/entity/obj.go`):**

The existing no-arg `IsValid() bool` (intrinsic base, always true) is
required by the polymorphic `entity` interface at
`modules/world/movement_consts.go:45-49` and consumed at 6+ sites
(`npc_interaction.go:577,804`, `npc_interaction_trigger.go:216,236`,
`pkg/zone/zone.go:441,456`). It MUST be left untouched. The new
player-aware variant gets a distinct name:

```go
// ObjCount returns the obj's current stack size. Method wrapper around
// the public Count field so *Obj satisfies script.ActiveObj. (Go
// disallows same-name field + method; same convention as ObjType().)
func (o *Obj) ObjCount() int { return o.Count }

// IsValidFor reports whether the obj is consumable by the given player
// UID. Mirrors TS Obj.ts:52-62 with goscape's UID-int receiver instead
// of TS bigint hash64. Reveal>-1 means private; non-receiver players
// see invalid. Count<1 means depleted.
//
// NAI-153-D2: TS uses hash64 (bigint username hash); goscape uses
// ReceiverID = composeUID(username37, slot) per
// modules/world/server_varp.go:169.
//
// Distinct from the no-arg IsValid() (intrinsic base, always true)
// which satisfies the polymorphic entity interface — Go disallows
// method overloading, so the player-aware variant gets its own name.
func (o *Obj) IsValidFor(playerUID int) bool {
    if o.Reveal > -1 && playerUID != o.ReceiverID {
        return false
    }
    if o.Count < 1 {
        return false
    }
    return true
}
```

**Test fixture extensions (`pkg/script/handlers_obj_test.go`,
`pkg/script/handlers_vars_test.go`):**

`mockActiveObj` gains `count int`, `receiverID int`, `reveal int`
fields plus `ObjCount() int` and `IsValidFor(playerUID int) bool`
methods. Plan-author greps every `mockActiveObj{` literal across
`pkg/script/*_test.go` and updates per
`plan_enumerate_struct_literals.md`.

**LOC:** ~25 production + ~50 test (incl. the `pkg/entity/obj_test.go`
unit tests for `IsValidFor` itself).

### 5.3. T3 — `handleObjCount` (`pkg/script/handlers_obj.go`)

**Production:**

```go
// handleObjCount (OBJ_COUNT, opcode 3503) pushes the active obj's
// count if it's valid for the active player; else pushes 0. Mirrors
// TS ObjOps.ts:121-130:
//
//   const obj: Obj = state.activeObj;
//   if (obj.isValid(state.activePlayer.hash64)) {
//       state.pushInt(state.activeObj.count);
//       return;
//   }
//   state.pushInt(0);
func handleObjCount(s *ScriptState) error {
    if err := requireActiveObj(s, "OBJ_COUNT"); err != nil {
        return err
    }
    if err := requireActivePlayer(s, "OBJ_COUNT"); err != nil {
        return err
    }
    if s.ActiveObj.IsValidFor(s.Self.UID()) {
        s.PushInt(s.ActiveObj.ObjCount())
        return nil
    }
    s.PushInt(0)
    return nil
}
```

**Registration (`pkg/script/handlers.go:120-127`):** add
`OpObjCount: handleObjCount,` to the OBJ family map.

**LOC:** ~15 production + ~70 test (5 cases — see §6).

### 5.4. T4 — `handleObjTakeItem` (`pkg/script/handlers_obj.go`)

**Production:**

```go
// handleObjTakeItem (OBJ_TAKEITEM, opcode 3510) pops invType, validates,
// guards on isValid, adds the obj to the player's inv, and removes the
// obj from the world. Mirrors TS ObjOps.ts:137-161.
//
// NAI-153-D1: TS calls activePlayer.addWealthEvent(...) between invAdd
// and removeObj. Skipped per NAI-115-D1 precedent — content can emit
// via OpWealthEvent (2131). (goscape defensive skip; TS inlines.)
//
// NAI-115-D2 (extended to TAKEITEM): TS calls World.removeObj(obj,
// respawnrate) for RESPAWN-lifecycle and World.removeObj(obj, 0) for
// DESPAWN. goscape's WorldVars.RemoveObj has no duration arg — both
// branches collapse to a single zero-arg RemoveObj call.
// RESPAWN-lifecycle respawn-after-delay remains a foundation gap
// (shared with OBJ_DEL).
func handleObjTakeItem(s *ScriptState) error {
    if err := requireActiveObj(s, "OBJ_TAKEITEM"); err != nil {
        return err
    }
    if err := requireActivePlayer(s, "OBJ_TAKEITEM"); err != nil {
        return err
    }
    if s.World == nil {
        return fmt.Errorf("OBJ_TAKEITEM: no world surface")
    }

    invID := s.PopInt()
    if err := checkInvType(s, invID, "OBJ_TAKEITEM"); err != nil {
        return err
    }

    if !s.ActiveObj.IsValidFor(s.Self.UID()) {
        return nil // TS returns false; goscape no-op (matches OBJ_DEL idiom)
    }

    if err := performInvAdd(s, invID, s.ActiveObj.ObjType(), s.ActiveObj.ObjCount(), "OBJ_TAKEITEM"); err != nil {
        return err
    }
    s.World.RemoveObj(s.ActiveObj)
    return nil
}
```

**Registration (`pkg/script/handlers.go:120-127`):** add
`OpObjTakeItem: handleObjTakeItem,` to the OBJ family map.

**Doc-comment retrofit (`pkg/script/handlers_obj.go:117-121`):** extend
the existing NAI-115-D2 deviation comment on `handleObjDel` to mention
TAKEITEM as a sibling consumer of the same gap.

**LOC:** ~50 production + ~90 test.

## 6. Test strategy

| Layer | New tests | LOC est. |
|---|---|---|
| T1 (`pkg/script/handlers_inv_test.go`) | `TestPerformInvAdd_DirectCall` | ~25 |
| T2 (`pkg/entity/obj_test.go`) | 5 cases (Public, PrivateSelf, PrivateOther, DepletedCount, ObjCount) | ~50 |
| T3 (`pkg/script/handlers_obj_test.go`) | 5 cases (PushesCount/Valid, PushesZero/Invalid, PushesZero/Depleted, NoActiveObj, NoActivePlayer) | ~70 |
| T4 (`pkg/script/handlers_obj_test.go`) | 7 cases (HappyPath, InvalidObj_Noop, BadInvType, NoActiveObj, NoActivePlayer, NoWorld, DepletedObj_Noop) | ~90 |

Per `scriptstate_test_fixture_idioms.md` — every fixture sets
`StackCapacity` + correct `Pointers` flags (`PtrActiveObj`,
`PtrActivePlayer`). Per `mock_recorder_field_naming_check.md` —
plan-author greps `mockActiveObj{`, `fakeWorldRemoveObj{`, and existing
inv-recorder helpers for actual field names before referencing.

**Cross-bundle regression:** full
`GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./...`
after each task lands. Plan-author preflight per
`enumerate_all_sites.md` greps:

- `mockActiveObj{` literals across `pkg/script/*_test.go` (T2 fixture-extension impact)
- `\.Count\b` field reads on `*entity.Obj` across `pkg/zone`, `modules/world` (T2 method-collision blast-radius — should be zero since the new method is named `ObjCount`, distinct from the existing `Count` field)
- `handleInvAdd` test fixtures (T1 refactor regression check)

**Smoke gate (Java-client mindrune-pickup repro,
user-launched server per `smoke_test_server_handoff.md`):**

| Smoke step | Pass condition |
|---|---|
| Drop mindrune (id=558) on player tile | item appears at player coord |
| Right-click → Take | no `"no handler for OBJ_COUNT"` log; no `"no handler for OBJ_TAKEITEM"` log |
| Inventory check | mindrune in inventory at expected slot |
| Ground state | item disappears from ground tile |
| Off-tile pickup (one tile away) | identical pass shape |

**Deferred (non-blocking):** RESPAWN-lifecycle respawn-after-delay (mindrune
is RESPAWN-lifecycle; ground tile stays empty until server restart).
NAI-115-D2 foundation gap, shared with OBJ_DEL. Documented in close
commit; smoke gate doesn't pin.

Adjacent surprises route per `smoke_surfaces_adjacent_divergences.md` —
≤30 LOC stretch-in, larger to NAI-154.

## 7. Cadence

Per `runescript_cadence.md` mid-band, single bundle, four T-tasks.
~95 production + ~235 test = ~330 LOC delta — within the ~300-400 LOC
band where `runescript_cadence` predicts ~1-1.5h subagent work.

| Task | Deps | Carrier |
|---|---|---|
| T1 (performInvAdd extract) | none | refactor; enables T4 |
| T2 (ActiveObj surface + Obj methods) | none | foundation; enables T3+T4 |
| T3 (handleObjCount) | T2 | small handler |
| T4 (handleObjTakeItem + smoke handoff) | T1, T2, T3 | smoke-gate carrier |

T1 and T2 are independent; controller may dispatch in parallel per
`dispatching-parallel-agents`. T3 and T4 are sequential dependents on T2.

**Workflow:** spec write → user spec review → plan (writing-plans skill)
→ `/clear` between plan and impl per `superpowers_clear_between_spec_and_impl.md`
→ subagent-driven impl per `execution_mode_default.md` → reviewer subagent
on Sonnet per `superpowers_code_reviewer_model.md` → smoke handoff →
close commit with `Closes memory:` trailer per `close_commit_memory_trailer.md`.

## 8. TS-fidelity deviations

- **NAI-153-D1 (T4) [NEW]:** OBJ_TAKEITEM skips TS's inline
  `addWealthEvent` call. Mirrors NAI-115-D1 precedent for INV_DEL/SCOPE_PERM.
  Content can emit via `OpWealthEvent` (2131). Doc-comment label per
  `defensive_gate_doc_comment_label.md`. Risk: none — observability
  ledger only, not gameplay-binding.
- **NAI-153-D2 (T2) [NEW]:** `Obj.IsValidFor(playerUID int)` uses goscape's
  UID-int receiver instead of TS bigint hash64. ReceiverID =
  `composeUID(username37, slot)` per `server_varp.go:169`. UID space is
  the established receiver type throughout obj routing. Risk: none —
  semantically equivalent identity check.
- **NAI-115-D2 (T4) [EXTENDED]:** OBJ_TAKEITEM inherits the same
  RemoveObj-no-duration foundation gap as OBJ_DEL. RESPAWN-lifecycle
  respawn-after-delay does not fire; DESPAWN-lifecycle works correctly
  (terminal removal). Doc-comment update at `handlers_obj.go:117-121`
  lists TAKEITEM as a sibling consumer.
- **NAI-153-D-X:** open new entry at impl time per `true_to_ts_gate.md`
  for any surfaced divergence.

## 9. Risk register

- **R1 (med):** Field-method collision on `*entity.Obj.Count` (existing
  public field) blocks the planned `Count() int` method. **Mitigation:**
  spec prescribes the method name `ObjCount()` (mirrors existing
  `ObjType field → ObjType() method` precedent on the same struct).
  Plan-author greps `pkg/zone`, `modules/world` for `\.Count\b` field
  reads to confirm zero rename pressure (the new method name avoids
  collision entirely).
- **R2 (resolved at spec-write):** `*entity.Obj.IsValid()` (no-arg,
  always true) is required by the polymorphic `entity` interface at
  `modules/world/movement_consts.go:45-49` and consumed at 6+ sites
  (`npc_interaction.go:577,804`, `npc_interaction_trigger.go:216,236`,
  `pkg/zone/zone.go:441,456`). The new player-aware variant therefore
  uses a distinct name `IsValidFor(playerUID int) bool` (Go disallows
  method overloading). The existing no-arg method is left untouched.
  No rename, no compile-graph cascade.
- **R3 (low-med):** `mockActiveObj` test fixtures across
  `pkg/script/handlers_obj_test.go`, `handlers_vars_test.go` need
  extending to satisfy the new interface (`ObjCount`, `IsValidFor`). Any
  existing test that constructs an `ActiveObj`-typed mock without the
  new methods fails to compile after T2. **Mitigation:** plan-author
  preflight greps `mockActiveObj{` and `ActiveObj =` across `_test.go`
  per `enumerate_all_sites.md`; T2 ships fixture extensions in the same
  commit per `latent_bug_at_migration_boundary.md`.
- **R4 (low):** `performInvAdd` extraction (T1) — if the existing
  `handleInvAdd` body has stack-side effects beyond the three pops
  (e.g. consumes pointer flags), the refactor leaks those into the new
  helper. **Mitigation:** plan-author Reads `handlers_inv.go:318-end`
  line-by-line and copies the body verbatim; only the three `PopInt`
  calls and the `requireActivePlayer` guard stay in the wrapper. Per
  `audit_full_method_when_restructuring.md`.
- **R5 (low):** Mindrune respawn-after-pickup (RESPAWN lifecycle) won't
  fire — pickup works, ground stays empty. If user expects respawn in
  the smoke, NAI-115-D2 needs to surface. **Mitigation:** smoke gate
  explicitly excludes respawn from acceptance; close commit documents
  the carry-forward; route to NAI-154 if user requests respawn.
- **R6 (low):** B2 smoke (06f4571) confirmed
  `[label,pickup_obj_floor]` AND `[label,pickup_obj_table]` both crash
  on OBJ_COUNT — content's pickup chain is wired and dispatching. If a
  content patch lands between B2 close and NAI-153 close that changes
  the script header (e.g. `pickup_obj_floor` rename), the smoke might
  re-pin a different blocker. **Mitigation:** controller pre-flight at
  smoke time re-greps `LostCityRS/Content/scripts/**/*.rs2` for
  `pickup_obj_floor` / `pickup_obj_table` and confirms registration matches.
- **R7 (low):** `Self.UID()` is consumed by `IsValidFor(playerUID)` calls
  in T3+T4. If `Self.UID()` returns a non-UID-space value (e.g. raw slot
  or hash37), the receiver match silently fails for private drops.
  **Mitigation:** plan-author greps `Self.UID()` consumers
  (`handlers_obj.go:104`, `handler_opobj.go`, `worldVarsView.AddObj` at
  `server_varp.go:169-170`) — all use composeUID; new sites inherit the
  same UID space. Pin test in T3 exercises the round-trip via a private
  obj with `ReceiverID == s.Self.UID()`.

## 10. Acceptance gate

NAI-153 closes only after the Java-client smoke pass shape in §6:
mindrune in inventory, ground tile clears, no `"no handler for OBJ_COUNT"`
log, no `"no handler for OBJ_TAKEITEM"` log. Anything beyond is routed
to NAI-153 stretch (≤30 LOC) or NAI-154 (larger).

Closes deviation: NAI-152-B2-FOLLOWUP-NAI-153 (retire from
`nai_followups.md`).
