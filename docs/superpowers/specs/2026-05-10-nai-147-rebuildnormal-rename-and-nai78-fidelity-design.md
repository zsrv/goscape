# NAI-147 — `rebuildNormal` rename + NAI-78 fidelity batch

**Date:** 2026-05-10
**Cascade context:** picks up open queue from NAI-146 close. Closes
4 tracker entries: `NAI-142-D-R-D4`, `NAI-78-D-NULL-TYPE-GUARD-OMITTED`,
`NAI-78-D-DEBUG-MSG-DEFERRED`, `NAI-78-D-HASINTERACTION-GUARD`. Adds
defensive-only doc-comment labels on 8 fire-helper no-script branches per
NAI-78 close note (cosmetic scaffolding).

## 1. Symptom / motivation

No user-facing symptom. This is a TS-fidelity polish bundle that
absorbs the open NAI-78 deviation tags + the deferred R-D4 cosmetic
rename (now triggered, since R-D1/R-D2/R-D3 closed via NAI-143/NAI-145).

- **R-D4** (`updateMap` → `rebuildNormal`): `(*Player).updateMap`
  (`modules/world/player.go:771`) ports TS `BuildArea.rebuildNormal`,
  not TS `NetworkPlayer.updateMap`. The real `NetworkPlayer.updateMap`
  slot is `(*Player).updateBuildArea` (NAI-142). Mis-naming hurts
  grep-discoverability and reserves the `updateMap` name for future
  consolidation work. Per `nai_followups.md` line 6632-6634: "Defer
  until R-D1/R-D2/R-D3 cumulatively justify it" — that condition now
  holds.
- **NAI-78-D-NULL-TYPE-GUARD-OMITTED**: TS `Player.ts:986-988` /
  `:1020-1022` — `if (!type) return null` returns null from
  `getOpTrigger`/`getApTrigger` when type lookup fails. Goscape's
  `triggerTypeAndCategory` (`interaction_trigger.go:551-585`) instead
  falls through with `categoryId=0`, leaving the 3-tier
  `GetByTrigger` fallback to filter. Behaviorally equivalent in
  practice (production cache always registers types for spawned
  entities) but TS-divergent in the surface contract.
- **NAI-78-D-DEBUG-MSG-DEFERRED**: TS `Player.ts:1076-1093` emits
  `[debug] No trigger for [<trigger-name>,<debugname>]` chat under
  `!NODE_PRODUCTION && !opTrigger && !apTrigger`. Goscape's
  `defaultOp` (`interaction.go:463-466`) skips the debug emit. The
  `Cfg.NodeDebug` analogue (`config.go:76`) is in place; this sub-spec
  ports the chat under that gate.
- **NAI-78-D-HASINTERACTION-GUARD**: TS `Player.ts:1114` —
  `if (!this.target || !this.hasInteraction() || !this.canAccess())`.
  Goscape's `tryInteract` (`interaction.go:374-378`) only checks
  `p.target == nil`. Follow-op targets (APPLAYER3/OPPLAYER3) currently
  reach branches 1-4; post-fix they short-circuit at top.
- **Defensive-only labels** (NAI-78 close minor): 8 `if sf == nil`
  branches in fire helpers became defensive-only after NAI-78's
  pre-gate on resolved-trigger-non-nil. Adds doc-comment labels per
  `defensive_gate_doc_comment_label.md`.

**Tracker entries closed by this:** R-D4 + 3× NAI-78-D items.
**Tracker entries opened by this:** none anticipated; possibly one
sub-deviation if T4-a numeric-fallback for the trigger-name lookup
lands as `NAI-147-D-TRIGGER-NAME-NUMERIC` (debug-build chat only;
non-functional).

## 2. Scope

5 tasks; 4 tracker entries closed; ~60-90 LOC production + ~80 LOC
test. All changes within `modules/world/`. No new packages, no new
exported APIs.

### 2.1 Task list (bottom-up by risk)

- **T1** — Defensive-gate doc-comment labels (zero-behavior).
- **T2** — R-D4 rename (`updateMap` → `rebuildNormal`, zero-behavior).
- **T3** — NULL-TYPE-GUARD-OMITTED port (TS-fidelity).
- **T4** — DEBUG-MSG-DEFERRED port (TS-fidelity, NodeDebug-gated).
- **T5** — HASINTERACTION-GUARD port (TS-fidelity, follow-op
  branch-routing change).

### 2.2 Non-goals

- No new follow-op semantics beyond the L1114 guard. The HASINTERACTION
  short-circuit is the SINGLE delta from this sub-spec; broader
  follow-op modeling stays deferred.
- No `ServerTriggerType.String()` name table (see §3.2 T4-a sub-decision).
- No port of the TS `nextTarget` / `apRangeCalled` carry-forward
  beyond what NAI-78/NAI-69 already delivered.
- No retirement of the 8 fire-helper `if sf == nil` defensive branches
  themselves — they remain as goscape-defensive belt-and-braces.

## 3. Files touched

| File | Tasks | Nature |
|------|-------|--------|
| `modules/world/interaction_trigger.go` | T1, T3 | 6 doc-comment labels (T1); `triggerTypeAndCategory` signature + `getOpTrigger`/`getApTrigger` early-return (T3) |
| `modules/world/player_interaction_trigger.go` | T1 | 2 doc-comment labels |
| `modules/world/player.go` | T2 | rename `(*Player).updateMap` → `rebuildNormal`, retarget self-doc-comment refs |
| `modules/world/tick.go` | T2 | retarget 1 caller (`tick.go:471`) + adjacent doc-comment ref |
| `modules/world/login_map_test.go` | T2 | 3 call-site retargets |
| `modules/world/tick_order_test.go` | T2 | doc-comment ref retargets |
| `modules/world/player_zone_test.go` | T2 | doc-comment ref retargets |
| `modules/world/interaction.go` | T4, T5 | `defaultOp` signature + NodeDebug-gated chat (T4); top-of-`tryInteract` follow-op short-circuit (T5) |
| `modules/world/interaction_trigger_null_guard_test.go` | T3 | new — NULL-guard + dual-pin tests |
| `modules/world/interaction_default_op_debug_test.go` | T4 | new — NodeDebug-gated chat coverage |
| `modules/world/interaction_tryinteract_guard_test.go` | T5 | new — HASINTERACTION-GUARD branch-routing pins |

### 3.1 Per-item TS source map

**T1 — Defensive-gate doc-comment labels** *(zero-behavior)*

8 sites (verified at HEAD `58fe41f`):
- `interaction_trigger.go:79` (`fireOpTriggerNpc`)
- `interaction_trigger.go:158` (`fireOpTriggerLoc`)
- `interaction_trigger.go:345` (`fireApTriggerNpc`)
- `interaction_trigger.go:432` (`fireApTriggerLoc`)
- `interaction_trigger.go:674` (`fireOpTriggerObj`)
- `interaction_trigger.go:740` (`fireApTriggerObj`)
- `player_interaction_trigger.go:64` (`fireOpTriggerPlayer`)
- `player_interaction_trigger.go:115` (`fireApTriggerPlayer`)

Standard label per `defensive_gate_doc_comment_label.md`:

```go
// Defensive-only post-NAI-78 (goscape defensive; TS skips this
// re-check). tryInteract pre-gates on resolved-trigger-non-nil so
// this branch is unreachable from the hot path. Preserved for
// non-tryInteract callers and as a goscape belt-and-braces.
```

Plan-author re-greps every `if sf == nil` site at HEAD before the T1
commit to confirm exact line numbers (per `enumerate_all_sites.md` +
`controller_preflight.md`).

**T2 — R-D4 rename** *(zero-behavior)*

Mechanical replace:
- declaration: `func (p *Player) updateMap()` → `func (p *Player) rebuildNormal()`
- call sites: `tick.go:471` (`p.updateMap()` → `p.rebuildNormal()`); 3
  test sites in `login_map_test.go` (lines 36, 74, 88).
- doc-comment refs: every `updateMap` reference within
  `modules/world/` that points at THIS function (NOT references to
  TS `NetworkPlayer.updateMap` which is a separate symbol).
  Plan-author enumerates references via
  `rg -n "\bupdateMap\b" modules/world/` and edits per-instance per
  `plan_doc_replaceall_timeline.md` (no global `replace_all`).

TS source: `BuildArea.ts` `rebuildNormal` method.

**T3 — NULL-TYPE-GUARD-OMITTED** *(TS-fidelity)*

TS source: `Player.ts:986-988` (`getOpTrigger`) and `:1020-1022`
(`getApTrigger`):

```typescript
if (!type) {
    return null;
}
```

Port shape: change `triggerTypeAndCategory` signature from

```go
func triggerTypeAndCategory(p *Player, srv *Server) (typeId, categoryId int)
```

to

```go
func triggerTypeAndCategory(p *Player, srv *Server) (typeId, categoryId int, ok bool)
```

Behavior:
- `*Npc` target: `ok = (tgt.typ != nil)`. Drop the `else { categoryId = 0 }` branch.
- `*entitypkg.Loc` target: `ok = (srv.locTypes != nil && locId >= 0 && locId < len(srv.locTypes.Configs) && srv.locTypes.Configs[locId] != nil)`.
- `*entitypkg.Obj` target: `ok = (srv.objTypes != nil && tgt.Type >= 0 && tgt.Type < len(srv.objTypes.Configs) && srv.objTypes.Configs[tgt.Type] != nil)`.
- `*Player` target: `ok = true` (Player branch has no type lookup
  in TS — typeId/categoryId stay -1).

`getOpTrigger` and `getApTrigger` both add:

```go
typeId, categoryId, ok := triggerTypeAndCategory(p, srv)
if !ok {
    return nil
}
```

between the existing `triggerTypeAndCategory` call and the
`GetByTrigger` invocation.

**Caller cascade** (per R1): `triggerTypeAndCategory` is also referenced
by tests directly. Fire helpers do NOT call it (they use
`resolveTriggerTypeId` + per-helper category logic). Plan-author
enumerates every call site with `rg -n "triggerTypeAndCategory\(" modules/`
and confirms the change is bounded.

**T4 — DEBUG-MSG-DEFERRED** *(TS-fidelity, NodeDebug-gated)*

TS source: `Player.ts:1076-1097`. Goscape port shape:

```go
func defaultOp(p *Player, opTrigger, apTrigger *script.ScriptFile) {
    s := p.client.server
    if s != nil && s.cfg.NodeDebug && opTrigger == nil && apTrigger == nil {
        debugname := defaultOpDebugname(p, s)
        // T4-a sub-decision: numeric trigger-name fallback.
        // ServerTriggerType has no String() method; use numeric
        // form per NAI-147-D-TRIGGER-NAME-NUMERIC.
        p.MessageGame(fmt.Sprintf("No trigger for [%d,%s]", p.targetOp+7, debugname))
    }
    p.MessageGame("Nothing interesting happens.")
    p.waypointIndex = -1 // TS Player.ts:1096 — clearWaypoints()
}

// defaultOpDebugname mirrors TS Player.ts:1077-1090 fan-out.
func defaultOpDebugname(p *Player, s *Server) string {
    switch tgt := p.target.(type) {
    case *Npc:
        if tgt.typ != nil && tgt.typ.DebugName != "" {
            return tgt.typ.DebugName
        }
        return strconv.Itoa(tgt.typeId)
    case *entitypkg.Loc:
        if s.locTypes != nil && tgt.Type() >= 0 && tgt.Type() < len(s.locTypes.Configs) {
            if lt := s.locTypes.Configs[tgt.Type()]; lt != nil && lt.DebugName != "" {
                return lt.DebugName
            }
        }
        return strconv.Itoa(tgt.Type())
    case *entitypkg.Obj:
        if s.objTypes != nil && tgt.Type >= 0 && tgt.Type < len(s.objTypes.Configs) {
            if ot := s.objTypes.Configs[tgt.Type]; ot != nil && ot.DebugName != "" {
                return ot.DebugName
            }
        }
        return strconv.Itoa(tgt.Type)
    }

    // T-trigger com-branch (TS L1086).
    if p.targetSubject.com != -1 && isApTTrigger(p.targetOp) {
        if s.componentTypes != nil && p.targetSubject.com >= 0 && p.targetSubject.com < len(s.componentTypes.Configs) {
            if ct := s.componentTypes.Configs[p.targetSubject.com]; ct != nil && ct.ComName != "" {
                return ct.ComName
            }
        }
        return strconv.Itoa(p.targetSubject.com)
    }

    // targetSubject.typ override branch (TS L1088 — TS field name `type`,
    // goscape field name `typ` per `player.go:143` `targetSubject struct{
    // typ, x, z, level, com int }`).
    if p.targetSubject.typ != -1 {
        if s.objTypes != nil && p.targetSubject.typ >= 0 && p.targetSubject.typ < len(s.objTypes.Configs) {
            if ot := s.objTypes.Configs[p.targetSubject.typ]; ot != nil && ot.DebugName != "" {
                return ot.DebugName
            }
        }
        return strconv.Itoa(p.targetSubject.typ)
    }

    return "_"
}
```

Caller update at `interaction.go:443`: `defaultOp(p)` →
`defaultOp(p, opTrigger, apTrigger)`. Both triggers are already in
scope at that point (resolved at `interaction.go:389-390`).

`isApTTrigger` is a small helper testing
`targetOp ∈ {targetOpNpcT, targetOpPlayerT, targetOpLocT, targetOpObjT}`.
Plan-author confirms all 4 sentinels exist in `modules/world/interaction.go`
or the equivalent file (R6 mitigation).

**T5 — HASINTERACTION-GUARD** *(TS-fidelity, branch-routing change)*

TS source: `Player.ts:1114`:

```typescript
if (!this.target || !this.hasInteraction() || !this.canAccess()) {
    return false;
}
```

Goscape port at `interaction.go:374-378`:

```go
func (p *Player) tryInteract(allowOpScenery bool) bool {
    if p.target == nil || !p.HasInteraction() || !p.CanAccess() {
        recordTryInteractBranch(p, 0) // NAI-79 Stage 1 (no-target / follow-op / canAccess early-return)
        return false
    }
    // (existing body unchanged)
```

**`(*Player).CanAccess()` semantics** (R4): verified at HEAD
`58fe41f` — `player_script.go:324-335` returns false on
`p.delayed || (modalState & main|chat != 0) || protectedScriptActive()`.
This matches TS `canAccess()` at `Player.ts:805-812`
(`!this.protect && !this.busy()`, where `busy() = delayed ||
containsModalInterface()`) via the NAI-111 narrowed convergence
(`protectedScriptActive` ↔ TS `this.protect`). World-shutdown
short-circuit (TS L806-808) is a non-issue (no `World.shutdown`
analogue on this code path). T5 uses `(*Player).CanAccess()`
directly.

**Existing `processInteraction:196` `delayed && currentTick <
delayedUntil` check**: this is `tryInteract`'s caller, so the new
`tryInteract` guard's delayed-check is redundant there but TS-faithful
(TS calls `canAccess()` inline in tryInteract). Defense in depth —
keep both. Note that `CanAccess()` is STRICTER than the existing
`processInteraction:196` check (it also gates on modalState +
protectedScriptActive), so adding it inside tryInteract may surface
short-circuits the inline check would not have triggered. Plan-author
audits whether any of those new short-circuit conditions affect
existing tryInteract test coverage; if so, mark with
`NAI-147-D-CANACCESS-MODAL-GATE` and follow the dual-pin pattern.

The retired `if p.target == nil` at line 375 is folded into the new
3-part guard. The branch-0 `recordTryInteractBranch` call captures
all three early-return paths.

## 4. Test coverage plan

### 4.1 T1 — Defensive labels

No new tests. Verification = `go vet ./...` + `go build ./...` green.

### 4.2 T2 — R-D4 rename

No new tests. Existing `TestLoginSendsRebuildNormal` (already named
correctly!), `TestUpdateMapAnchorsOriginToPlayer`, and zone-test
references retarget the new name. Verification =
`go test ./modules/world/... ./...` green.

### 4.3 T3 — `interaction_trigger_null_guard_test.go` (new)

| Test | Setup | Expect |
|------|-------|--------|
| `TestTriggerTypeAndCategory_NpcWithNilType_OkFalse` | `Npc{typeId: 999, typ: nil}` | `(_, _, ok) → ok=false` |
| `TestTriggerTypeAndCategory_LocOutOfRange_OkFalse` | `Loc{Type: 999999}`, locTypes len=10 | `ok=false` |
| `TestTriggerTypeAndCategory_LocNilConfig_OkFalse` | `Loc{Type: 5}`, `srv.locTypes.Configs[5]==nil` | `ok=false` |
| `TestTriggerTypeAndCategory_ObjOutOfRange_OkFalse` | `Obj{Type: 999999}` | `ok=false` |
| `TestTriggerTypeAndCategory_ObjNilConfig_OkFalse` | `Obj{Type: 5}`, nil config | `ok=false` |
| `TestTriggerTypeAndCategory_NpcOk` | `Npc{typ: &NpcType{Category: 7}}` | `ok=true, typeId=npc.typeId, categoryId=7` |
| `TestTriggerTypeAndCategory_PlayerOk` | `target: *Player` | `ok=true, typeId=-1, categoryId=-1` |
| `TestGetOpTrigger_NilTypeReturnsNil` | Npc target, type unloaded, script registered at category=0 | `getOpTrigger → nil` (proves short-circuit fires) |
| `TestGetApTrigger_NilTypeReturnsNil` | parallel for AP | `getApTrigger → nil` |
| `TestGetOpTrigger_TypeKnownResolvesAtCategoryFallback` | typed entity with category=0 fallback script registered | `getOpTrigger → script` (dual-pin per `ts_asymmetry_dual_pin`) |

### 4.4 T4 — `interaction_default_op_debug_test.go` (new)

| Test | Setup | Expect |
|------|-------|--------|
| `TestDefaultOp_NoTriggerEmitsDebug_NodeDebugTrue` | `cfg.NodeDebug=true`, opTrigger=nil, apTrigger=nil, target Npc with `DebugName="test_npc"`, `targetOp=ApNpc1` | MessageGame queue contains `[debug] No trigger for [<targetOp+7>,test_npc]` AND `Nothing interesting happens.` |
| `TestDefaultOp_NoTriggerSuppressed_NodeDebugFalse` | `cfg.NodeDebug=false`, both nil | only `Nothing interesting happens.` |
| `TestDefaultOp_DebugSuppressed_OpTriggerPresent` | `cfg.NodeDebug=true`, opTrigger non-nil | only `Nothing interesting happens.` (debug gated on both nil) |
| `TestDefaultOp_DebugSuppressed_ApTriggerPresent` | `cfg.NodeDebug=true`, apTrigger non-nil | only `Nothing interesting happens.` |
| `TestDefaultOp_DebugnameNpc_FallbackToTypeId` | NpcType with `DebugName=""` | debug message ends in `,42]` (numeric typeId) |
| `TestDefaultOp_DebugnameLoc` | LocType with `DebugName="newbie_door1"` | debug message ends in `,newbie_door1]` |
| `TestDefaultOp_DebugnameObj` | ObjType with `DebugName="bones"` | `,bones]` |
| `TestDefaultOp_DebugnameComOverride_TBranch` | `targetSubject.com=200`, `targetOp=targetOpNpcT`, ComponentType `ComName="spell_blast"` | `,spell_blast]` |
| `TestDefaultOp_DebugnameSubjectTypeOverride` | target=`*Player`, `targetSubject.typ=42`, ObjType[42] with `DebugName="bones"` | `,bones]` |
| `TestDefaultOp_DebugnameDefault_Underscore` | target=`*Player`, no com, `targetSubject.typ=-1` | `,_]` |
| `TestDefaultOp_ClearWaypointsAlwaysFires` | any setup | `waypointIndex == -1` post-call (regression fence on existing behavior) |
| `TestDefaultOp_NothingInteresting_AlwaysFires` | NodeDebug both true and false | `Nothing interesting happens.` always present (regression fence) |

### 4.5 T5 — `interaction_tryinteract_guard_test.go` (new)

| Test | Setup | Expect |
|------|-------|--------|
| `TestTryInteract_FollowOp_ShortCircuits` | target=`*Npc`, `targetOp=` follow-op sentinel (APPLAYER3/OPPLAYER3 analogue) | `tryInteract(false)→false`; `recordTryInteractBranch(p, 0)`; `interactionFired==false`; `apRange` unchanged |
| `TestTryInteract_NotFollowOp_NotShortCircuited` | target valid, non-follow-op, non-delayed | proceeds into branches 1-4 (existing behavior) |
| `TestTryInteract_Delayed_ShortCircuits` | `delayed=true`, `delayedUntil>currentTick` | `tryInteract → false` via `!CanAccess()` gate |
| `TestTryInteract_NoTarget_ShortCircuits` | `target=nil` | `tryInteract → false` (existing branch unchanged) |
| `TestTryInteract_FollowOpDelayed_BothGatesGuard` | follow-op + delayed | short-circuit (regression fence — no panic, returns false) |
| `TestTryInteract_HasInteractionTrue_ProceedsToBranch1` | non-follow-op OP target with op-trigger registered, operable | `tryInteract → true`, branch 1 fires (regression fence) |

### 4.6 Cross-cutting

`go test -race ./...` green; `go vet ./...` green; `go build ./...` green
at each task HEAD.

## 5. Plan-author pre-flight verification

Per `controller_preflight.md` and `risk_register_premise_grep.md`, the
plan-author re-verifies at HEAD before task dispatch:

- **R1**: re-enumerate `triggerTypeAndCategory(` call sites via
  `rg -n "triggerTypeAndCategory\(" modules/`. Verified at spec-write
  HEAD `58fe41f`: 2 production callers — `getOpTrigger`
  (`interaction_trigger.go:603`) and `getApTrigger` (`:618`). Fire
  helpers do NOT call this function. Codify the exhaustive list in T3
  task block; re-grep at T3 close.
- **R2**: confirm `defaultOp(` callers via
  `rg -n "defaultOp\(" modules/`. Verified at HEAD `58fe41f`: 1
  production caller (`interaction.go:443`) + 1 test caller
  (`interaction_test.go:1545`). T4 updates both.
- **R3**: read TS `Player.ts:1113-1184` end-to-end, then trace each
  goscape branch in `tryInteract`. If any branch mutates state that
  would now be skipped (e.g., branch 3's `apRange = -1` for
  follow-ops), document the delta in T5 task block and add a
  companion mutation OR open `NAI-147-D-FOLLOWOP-APRANGE`.
- **R4**: read `(*Player).CanAccess()` at HEAD; confirm semantics or
  document delta inline in T5.
- **R5**: confirm `s.componentTypes.Configs[com].ComName` access
  pattern at `handler_opheld.go:206` is current at HEAD.
- **R6**: enumerate T-trigger sentinels via
  `rg -n "targetOp\w*T\b\s*=" modules/world/interaction.go` to
  populate `isApTTrigger`'s set. Cross-check against TS L1086 list
  (APNPCT, APPLAYERT, APLOCT, APOBJT).
- **R7**: confirm `newTestServer` already seeds locTypes/objTypes/
  npcTypes/componentTypes; if any missing, T3 task includes the
  fixture extension per `test_fixture_view_parity.md`.

## 6. Tracked deviations

- **`NAI-147-D-TRIGGER-NAME-NUMERIC`** (T4-a): debug-build chat emits
  numeric trigger value `[%d,...]` instead of TS lowered name
  `[<trigger-name>,...]`. *Why:* `pkg/script.ServerTriggerType` lacks
  a `String()` method; adding a 50+ entry name table for a
  debug-only chat is over-investment. *Goscape defensive label:* not
  needed (numeric form is unambiguous; ServerTriggerType.ts is the
  reverse-lookup table). *Carry-forward:* land with future
  `ServerTriggerType.String()` work if any non-debug consumer needs
  it; no current trigger.

(none anticipated for T1/T2/T3/T5; plan-author may add during
audit.)

## 7. Smoke

**Deferred** per `cascade_theory_smoke_binding.md` and the answered
brainstorm question. PRIMARY pin = test-only; this extends the
deferred-smoke batch alongside NAI-143 / NAI-144 / NAI-145 / NAI-146.

**Carry-forward**: bind a smoke at the next user-facing tick-touching
sub-spec close. NAI-147's HASINTERACTION-GUARD is technically
branch-routing-touching, but the practical delta for follow-ops is
minimal (existing branch 3 already routed to a no-op apRange=-1
return). The bundle ships on test-only evidence per the chosen
strategy.

## 8. Cadence

5-task TDD bundle per `runescript_cadence.md`. Per
`superpowers_clear_between_spec_and_impl.md`: emit resume prompt and
stop after plan-write; user `/clear` before implementing.

Per `execution_mode_default.md`: dispatch via subagent-driven-development.

T1 + T2 (zero-behavior) ship without TDD red/green pairs (no
behavior to test). T3 + T4 + T5 follow standard TDD: red commit
(failing test) → green commit (impl + adjustments) → review.

Reviewer: single Sonnet at end of bundle per
`superpowers_code_reviewer_model.md`. No mid-bundle review unless a
task surfaces a deviation that wasn't anticipated at spec-write.

## 9. Tech stack

- Go 1.26+ per `go_version.md`.
- TS source canonical path: `Engine-TS` only per
  `ts_source_canonical_path.md` — `$HOME/Code/github.com/LostCityRS/Engine-TS`.

## 10. Pattern memories applicable

- `defensive_gate_doc_comment_label.md` — T1.
- `enumerate_all_sites.md` — T1 (8 fire-helper sites), T3 (callers
  of `triggerTypeAndCategory`), T4 (callers of `defaultOp`).
- `plan_doc_replaceall_timeline.md` — T2 rename: per-instance Edits,
  no global `replace_all`.
- `controller_preflight.md` — §5 pre-flight verification before each
  task dispatch.
- `ts_asymmetry_dual_pin.md` — T3 dual-pin: nil-type returns nil AND
  known-type resolves at category fallback.
- `risk_register_premise_grep.md` — §5 R1-R7 verified at HEAD.
- `test_fixture_view_parity.md` — T3/T4/T5 fixture seeding.
- `superpowers_code_reviewer_model.md` — Sonnet for reviewer + impl.
- `superpowers_clear_between_spec_and_impl.md` — `/clear` between
  plan-author and impl.
- `cascade_theory_smoke_binding.md` — smoke-deferral honest-noting.
- `close_commit_memory_trailer.md` — `Closes memory:` trailer in close
  commit.
- `compressed_cadence.md` — does NOT apply (multi-task plan, ~140-170
  LOC total, exceeds the upper band; standard TDD cadence applies).
- `mock_recorder_field_naming_check.md` — T3/T4/T5 fixture field
  names verified before writing tests.
