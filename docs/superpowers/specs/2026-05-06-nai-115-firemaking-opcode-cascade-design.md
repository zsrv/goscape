# NAI-115 — Firemaking opcode-cascade port

**Date:** 2026-05-06
**Status:** Design (brainstorm complete, awaiting plan)
**Predecessor:** NAI-114 CLOSED (`resolveListenerInv` UID-as-slot fix; smoke unblocked OPHELDU dispatch, surfaced first cascade opcode `INV_DROPSLOT` in `[label,tut_light_logs_inv]` PC 71)

## 1. Tech stack

Go 1.26+. Target packages: `pkg/script/` (handlers + dispatch table), `pkg/script/handlers_*_test.go` (unit tests). Foundation primitives in `pkg/zone/`, `pkg/pathfinder/routefinder/`, `pkg/objtype/`, `pkg/entity/` are pre-existing and consumed.

## 2. Bundle 0 finding

The NAI-114 close memo asserted that `MAP_LOCADDUNSAFE (1012)` was still pending. **This is stale**: commit `03bff85` ("feat(script): NAI-114 Stage 2 — port MAP_LOCADDUNSAFE handler") landed the port mid-NAI-114, before Stages 3-5 pivoted to the dispatch-bug fix. `MAP_BLOCKED (1007)` was already done in NAI-36 T4. Remaining cascade is **7 opcodes**, not 8.

Authoritative opcode partition derived from the .rs2 source files (more authoritative than disasm):

- `LostCityRS/Content/scripts/tutorial/scripts/skills/tut_firemaking.rs2` — Tutorial Island flow (`tut_light_logs_inv` + `tut_firemaking_success` proc)
- `LostCityRS/Content/scripts/skill_firemaking/scripts/firemaking.rs2` — non-Tutorial flow (`light_logs_inv` + `firemaking_success` + `push_player` shared proc + `area_allow_loc_add` proc)

**Tutorial-essential (5):** INV_DROPSLOT, OBJ_COORD, OBJ_DEL, OBJ_ADD, LINEOFWALK
**Non-Tutorial-only (2):** OBJ_ADDALL, P_OPOBJ

## 3. Cadence

Per `bundle0_short_circuits_stage1_audit`: **skip the Stage 1 audit subagent** — Bundle 0 produced concrete TS-source mappings (per-opcode line ranges below) and validated foundation availability. An audit subagent would only re-verify.

- **Bundle 1** (subagent-driven TDD): 5 Tutorial-essential opcode ports.
- **Smoke checkpoint** (user-launched): Tutorial Island Survival Expert area; tinderbox-on-logs; expected ✅ animation + inv-drop + fire-loc + XP +400 + ashes drop.
- **Bundle 2** (subagent-driven TDD): 2 non-Tutorial-only opcode ports.
- **Optional second smoke**: outside-Tutorial firemaking (e.g., Lumbridge) for retry-loop + broadcast-ashes pin.
- **Close**: memory updates, deviation tracker, NAI-111 P_TELEJUMP carry-forward.

## 4. Bundle 1 — Tutorial-essential opcodes

| Task | Opcode | TS reference | Est. prod LOC | Est. test LOC |
|------|--------|--------------|---------------|---------------|
| T1 | OBJ_COORD (3502) | `ObjOps.ts:163-166` | 6 | 12 |
| T2 | OBJ_DEL (3504)   | `ObjOps.ts:112-119` | 12 | 15 |
| T3 | OBJ_ADD (3500)   | `ObjOps.ts:20-55`   | 45 | 25 |
| T4 | LINEOFWALK (1006)| `ServerOps.ts:65-82`| 20 | 18 |
| T5 | INV_DROPSLOT (4312) | `InvOps.ts:213-260` | 55 | 30 |

**Bundle 1 totals:** ~140 prod LOC + ~100 test LOC.

### 4.1 Per-task port shapes

**T1 — OBJ_COORD:** read `state.ActiveObj` coord (level/x/z), pack via `gamemap.PackCoord`, push int. Pointer guard: handler runs against ActiveObj pointer (caller-side state-pointer is set by upstream OBJ_FIND/OBJ_ADD/INV_DROPSLOT).

**T2 — OBJ_DEL:** Pull `respawnrate` from `ObjType` config of `state.ActiveObj.Type`. Call zone-level `RemoveObj` with the respawn duration. TS branches on `ActivePlayer` pointer presence but both branches invoke the same `World.removeObj(obj, duration)` — port as a single call.

**T3 — OBJ_ADD:** Pop 4 ints `(coord, objId, count, duration)`. Top-of-stack early-return on `objId == -1 || count == -1`. Validators: ObjTypeValid, DurationValid, CoordValid, ObjStackValid. Throw on `dummyitem != 0`. Members gate: skip when `objType.members && !MapMembers()`. Stackable branch: if `!stackable || count == 1`, instantiate `count` separate Obj entities each with stack=1; else instantiate one Obj with stack=count. Each instance: World-level addObj wrapper (or zone-local equivalent — see Risk 6.2) passes the active player's UID (per NAI-113 `composeUID`) as the receiver; sets `state.ActiveObj`; calls `state.PointerAdd(PtrActiveObj)`.

**T4 — LINEOFWALK:** Pop 2 ints (from-coord, to-coord). Both `CoordValid`. Push 0 if `from.level != to.level`. F2P short-circuit: if `!MapMembers() && !IsFreeToPlay(to.x, to.z)` push 0. Else push `HasLineOfWalk(level, srcX, srcZ, destX, destZ, srcSize=1, destWidth=0, destLength=0, extraFlag=0) ? 1 : 0`.

**T5 — INV_DROPSLOT:** Pop 4 ints `(inv, coord, slot, duration)`. Validators: InvTypeValid, DurationValid, CoordValid. **Protect gate** (`scriptstate_test_fixture_idioms`): if not `ProtectedActivePlayer` && `invType.protect` && `invType.scope != SCOPE_SHARED`, return error. Lookup obj at slot via `Player.InvGetSlot(invType.id, slot)`; error if nil/empty. **Wealth-event deviation (D1):** TS inlines `addWealthEvent` for SCOPE_PERM drops; goscape has separate `OpWealthEvent` (2131). Skip inline emission with a tracked deviation comment. Call `Player.InvDel(invType.id, obj.id, obj.count, slot)`; early-return on `completed == 0`. Stackable branch: same shape as T3 OBJ_ADD; passes the active player's UID as the receiver (vs T6's NO_RECEIVER for broadcast).

### 4.2 Bundle 1 smoke binding

User-launched server. Tutorial Island, post-Survival Expert dialog (per NAI-114 dispatch fix unblocking OPHELDU). Path: log on inv-side panel + tinderbox on inv → click logs with tinderbox.

**Pass criteria:**
- Animation `human_createfire` plays.
- Inv loses 1 newbielogs.
- Ground tile gains a fire loc (centrepiece_straight, 150-tick duration).
- Stat panel shows +400 firemaking XP (capped at 1, 2, or 3 base XP per `tutorial_give_xp` proc).
- After ~150 ticks: fire despawns, ashes appear at coord, despawn after `lootdrop_duration / 2` ticks.
- Server log: ZERO `no handler for OPCODE` warnings on `[label,tut_light_logs_inv]`, `[proc,tut_firemaking_success]`, `[proc,push_player]`.

**Stretch (in-scope ≤30 LOC per `smoke_surfaces_adjacent_divergences`):** if smoke surfaces adjacent gap (e.g., a referenced obj/loc config field ungenerated, an ObjType validator differs from TS), pin and fix in-bundle if ≤30 LOC; else route to NAI-116.

## 5. Bundle 2 — non-Tutorial-only opcodes

| Task | Opcode | TS reference | Est. prod LOC | Est. test LOC |
|------|--------|--------------|---------------|---------------|
| T6 | OBJ_ADDALL (3501) | `ObjOps.ts:58-93` | 45 | 25 |
| T7 | P_OPOBJ (2080)    | `PlayerOps.ts:990-1006` | 20 | 22 |

**Bundle 2 totals:** ~65 prod LOC + ~50 test LOC.

### 5.1 Per-task port shapes

**T6 — OBJ_ADDALL:** Twin of T3 OBJ_ADD. Identical pop, validation, members gate, stackable loop. Difference: receiver is `Obj.NO_RECEIVER` (broadcast) vs T3's per-caller UID. Refactor: extract a private helper from T3 that takes a `receiverID` argument; T6 invokes the helper with the broadcast sentinel. Per `plan_grep_helper_patterns` — search `pkg/script/handlers_obj.go` (created at T3) for a colocatable helper before reinlining.

**T7 — P_OPOBJ:** Pop 1 int `op`. NumberNotNull validator. Range gate: `op-1 ∈ [0, 5)` else error. Lookup `objType.Op[op-1]` (5-element op-name slice on ObjType); early-return if nil/empty. ProtectedActivePlayer pointer gate (handler-level; checkedHandler in TS). Effects: `state.ActivePlayer.StopAction()`, `state.ActivePlayer.QueueWaypoint(activeObj.X, activeObj.Z)`, `state.ActivePlayer.SetInteraction(InteractionScript, activeObj, ServerTriggerAPObj1 + (op-1))`. Verify trigger-type enum supports indexed addition; fall back to switch on `op-1` if not.

### 5.2 Bundle 2 smoke (optional)

Non-Tutorial firemaking on Lumbridge tile (or any post-tutorial coord with `area_allow_loc_add` evaluating true and not in bank/duel zone). Tinderbox-on-logs from inv. Expected: retry-loop completes after `stat_random` roll succeeds; ashes drop visible to all clients (broadcast).

## 6. Risk register

### 6.1 INV_DROPSLOT wealth-event divergence (D1)

TS `InvOps.ts:230-237` inlines `addWealthEvent` for `scope == SCOPE_PERM`. Goscape has separate `OpWealthEvent (2131)` already declared. **Decision: skip inline emission with tracked deviation comment.** Rationale: wealth-event subsystem hookup is out of scope; existing `OpWealthEvent` opcode lets content emit explicitly when needed. Memory crosscheck: `defensive_gate_doc_comment_label` — label inline as "(goscape defensive; TS calls addWealthEvent here)".

### 6.2 World-level addObj/removeObj wrappers

Zone-level `(*Zone).AddObj(obj, receiverID)` and `(*Zone).RemoveObj(obj, currentTick)` exist (`pkg/zone/zone.go:253,291`). Plan-author MUST grep for an existing world-level wrapper that derives `(level, x, z) → zone` and invokes the zone method; if absent, add minimal wrapper inline at first consumer (T3) and reuse from T6. Per `plan_sibling_site_guard_audit`, grep all sibling AddObj call sites first.

### 6.3 ServerTriggerType indexed addition (P_OPOBJ)

TS uses `ServerTriggerType.APOBJ1 + op`. Plan-author MUST verify goscape's trigger-type enum supports `+` (i.e., trigger types are `iota`-sequential APOBJ1..APOBJ5). Fall back: switch-table mapping `op-1 → ServerTriggerAPObj1..5`. Cite `pkg/script/<trigger>.go` line ranges in plan.

### 6.4 ObjType field availability

Required fields per port: `respawnrate` (T2), `dummyitem` (T3, T6), `members` (T3, T6), `stackable` (T3, T5, T6), `op[5]` (T7). Plan-author MUST grep `pkg/objtype/objtype.go` and citem definition; flag any missing field as a foundation port pre-req.

### 6.5 Player.InvGetSlot / InvDel signatures

INV_DROPSLOT depends on these. Plan-author confirms signatures match TS shape (`invGetSlot(invId, slot) → *Obj`, `invDel(invId, objId, count, slot) → completedCount`). Fixture-side: `scriptstate_test_fixture_idioms` for Pointers + StackCapacity setup.

### 6.6 Adjacent-divergence smoke surfacing

Per `smoke_surfaces_adjacent_divergences`: if Bundle 1 smoke reveals e.g., obj-spawn flicker, missing config field, or content-script load error, route ≤30 LOC fixes in-scope; else NAI-116.

## 7. Test strategy

**Per-opcode unit test** in colocated `pkg/script/handlers_*_test.go`:

- Pin pop-order via stack arrangement (LIFO; topmost pop is rightmost arg in `popInts(n)`).
- Pin validations: each validator failure path returns expected `*ScriptValidationError` shape.
- Pin push value (where applicable).
- Pin pointer-add semantics for handlers that set ActiveObj (T3, T5).
- Pin protect-gate & wealth-event-deviation paths (T5 D1).
- For T4 LINEOFWALK: stub `World` with HasLineOfWalk fake returning configured booleans; test 4-corner directional matrix.
- For T6 OBJ_ADDALL: assert helper-shared with T3 by exercising the helper directly with both receiver values; one wrapper test per opcode confirms wiring.
- For T7 P_OPOBJ: assert StopAction + QueueWaypoint + SetInteraction calls in order via mockPlayer recorder. Verify per `mock_recorder_field_naming_check` — read actual mockPlayer struct field names before referencing.

**No world-integration test** for opcode ports; smoke handoff covers content-layer correctness (cf. `cascade_theory_smoke_binding`).

**ScriptState fixture idioms** (`scriptstate_test_fixture_idioms`):
- StackCapacity init with at least 4 (max popInts).
- Pointers flag bits set per task: T3/T5 require `PtrActivePlayer`; T5 also `PtrProtectedActivePlayer` for the happy path.
- Push-order: bottom-up matches TS popInts destructuring.

## 8. Deviations tracker (initial)

**D1 — INV_DROPSLOT wealth-event skip** (T5). TS `InvOps.ts:230-237` inlines `addWealthEvent`; goscape skips inline, defers to `OpWealthEvent (2131)`. Doc-comment per `defensive_gate_doc_comment_label`. Follow-up: if wealth-event subsystem is later wired into Player, port the inline emission and retire D1.

## 9. Out-of-scope

- Wealth-event subsystem hookup (would resolve D1 without divergence).
- Content-script verification beyond opcode-level wiring (smoke covers content correctness).
- `[label,light_logs_ground]` / `[proc,firemaking_success]` integration tests beyond Bundle 2 smoke.
- NAI-111 P_TELEJUMP investigation (queued post-NAI-115).

## 10. Close-out (post-Bundle 2)

- Memory updates: NAI-115 close commit with `Closes memory:` trailer per `close_commit_memory_trailer`.
- Update `nai_followups.md` NAI-115 CLOSED section with smoke results, commits per task, deviation D1 status, NAI-111 carry-forward.
- Retire NAI-115 from `MEMORY.md` index if no follow-ups; otherwise leave a one-line cross-ref to the carry-forward.

## 11. References

**Source files (canonical):**
- TS: `LostCityRS/Engine-TS/src/engine/script/handlers/{ObjOps.ts, ServerOps.ts, InvOps.ts, PlayerOps.ts}`
- Content: `LostCityRS/Content/scripts/tutorial/scripts/skills/tut_firemaking.rs2`, `LostCityRS/Content/scripts/skill_firemaking/scripts/firemaking.rs2`

**Predecessor:**
- NAI-114 close memo (memory/nai_followups.md ~L5748)
- Stage 1.2 audit: `docs/superpowers/investigations/2026-05-06-nai-114-stage1-audit.md` (commit `4a0ad1e`)

**Memory crosschecks applied:** `bundle0_short_circuits_stage1_audit`, `smoke_surfaces_adjacent_divergences`, `cascade_theory_smoke_binding`, `scriptstate_test_fixture_idioms`, `defensive_gate_doc_comment_label`, `plan_sibling_site_guard_audit`, `plan_grep_helper_patterns`, `mock_recorder_field_naming_check`, `close_commit_memory_trailer`, `ts_source_canonical_path`.
