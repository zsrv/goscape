---
status: brainstorm-approved
date: 2026-05-10
closes: NAI-147-D-TRIGGER-NAME-NUMERIC
ts_source:
  - LostCityRS/Engine-TS/src/engine/script/ServerTriggerType.ts:166-170
  - LostCityRS/Engine-TS/src/engine/entity/Player.ts:1093
---

# NAI-148 — `ServerTriggerType.String()` + TS-faithful `defaultOp` debug name

**Cadence:** 15-100 LOC band per `compressed_cadence.md` — separate spec + plan docs, single combined Sonnet reviewer at end-of-impl. Subagent-driven-development per `execution_mode_default.md`.
**Tech stack:** Go 1.26+ (`go_version.md`).
**Closes:** `NAI-147-D-TRIGGER-NAME-NUMERIC` (deferred at NAI-147 T4 close `67e715f`, "50+ entry name table for one debug-only chat is over-investment" — that gate has now been chosen open).

---

## §1 Symptom / motivation

No user-facing symptom — debug-only chat emitted under `cfg.NodeDebug=true` (TS `!NODE_PRODUCTION` analogue).

Today, the `defaultOp` NIH fallback in `modules/world/interaction.go:472` emits e.g. `"No trigger for [8,test_npc]"` for an OpNpc1 click on a rat with no `[opnpc1]` registered. TS at `Player.ts:1093` emits `"No trigger for [opnpc1,test_npc]"` (TS `ServerTriggerType[targetOp+7]` where `targetOp = APNPC1 = 3`, so `+7 → OPNPC1 = 10 → "opnpc1"`).

Two layered divergences:

1. **No `String()` on `ServerTriggerType`.** Goscape's `pkg/script/trigger.go` declares 151 constants (with explicit numeric values matching TS `ServerTriggerType.ts`) but no name table. Format string falls back to `%d`.
2. **`p.targetOp+7` arithmetic ports the TS bytecode unchanged but operates in a different namespace.** TS `targetOp: TargetOp = ServerTriggerType | NpcMode` (PathingEntity.ts:28) — set by `setInteraction(...)` to e.g. `ServerTriggerType.APNPC1+type` (PlayerOps.ts:413). `+7` then maps `APNPC1=3 → OPNPC1=10`, `APLOC1=59 → OPLOC1=66`, `APNPCT=9 → OPNPCT=16`. Goscape's `targetOp` is op-slot index `1..5` plus sentinels `{6..13}` (interaction.go:36-45) — distinct from `ServerTriggerType` integers. So `targetOp+7` produces a number that does not correspond to any TS trigger value, and a hypothetical `ServerTriggerType(8).String()` would return `"apnpcu"` for an OpNpc1 click — *worse* than the existing numeric fallback because it would be misleading.

Fixing only (1) is therefore a regression risk. Both (1) and (2) ship together in this sub-spec.

## §2 Architecture

### §2.1 `ServerTriggerType.String()` table

Add to `pkg/script/trigger.go`:

```go
var serverTriggerNames = map[ServerTriggerType]string{
    TriggerProc:      "proc",
    TriggerLabel:     "label",
    TriggerDebugProc: "debugproc",

    TriggerApNpc1: "apnpc1",
    TriggerApNpc2: "apnpc2",
    // ... all 151 entries, mirroring TS `ServerTriggerType.ts` enum names lowercased.
    // Underscores in the TS source survive (e.g. AI_QUEUE1 -> "ai_queue1",
    // IF_CLOSE -> "if_close", INV_BUTTON1 -> "inv_button1").

    TriggerAiDespawn: "ai_despawn",
}

// String returns the TS-faithful lowered enum name (e.g. TriggerOpNpc1
// returns "opnpc1", TriggerAiQueue4 returns "ai_queue4"). Mirrors TS
// ServerTriggerType.toString at Engine-TS .../ServerTriggerType.ts:166-170:
//
//   ServerTriggerType[trigger].toLowerCase()
//
// Unknown values return "trigger_<N>" rather than panicking. TS would
// throw on `undefined.toLowerCase()`; Go's nil-handling and the
// debug-only call site make a sentinel safer (DEVIATION-NAI-148-D-STRING-FALLBACK).
func (t ServerTriggerType) String() string {
    if name, ok := serverTriggerNames[t]; ok {
        return name
    }
    return fmt.Sprintf("trigger_%d", int(t))
}
```

The full key set is the 151 `Trigger*` constants in `trigger.go:8-172`. Numeric gaps in the TS enum (values `{22, 23, 29, 30, 50, 51, 57, 58, 78, 79, 85, 86, 106, 107, 113, 114, 115}`) are intentionally absent — TS reverse-mapping returns `undefined` for those, which maps cleanly onto the Go fallback path.

### §2.2 `tsTriggerForOpFire` mapper

Add to `modules/world/interaction.go` (private helper, package-scoped):

```go
// tsTriggerForOpFire returns the TS-faithful OP* ServerTriggerType for the
// given target/targetOp pair, used only by defaultOp's debug chat.
//
// TS Player.ts:1093 emits ServerTriggerType[targetOp+7] where targetOp is
// the AP* trigger set by setInteraction; +7 maps AP* -> OP*. Goscape stores
// targetOp as an op-slot int (1..5) or one of the {targetOpLocT, targetOpLocU,
// targetOpNpcT, targetOpNpcU, targetOpPlayerT, targetOpPlayerU,
// targetOpObjT, targetOpObjU} sentinels. This helper bridges both
// namespaces.
//
// Sentinel matches dispatch by targetOp alone (TS L1086 — APNPCT/APPLAYERT/
// APLOCT/APOBJT all evaluate independent of target type). Numeric op-slots
// disambiguate via target type. Returns ServerTriggerType(-1) when target
// is nil or an unrecognised entity (defensive — tryInteract gates target!=nil
// upstream, and DEVIATION-NAI-148-D-OPFIRE-FALLBACK tracks the deviation from TS, which
// has no analogous fallback).
func tsTriggerForOpFire(target entity, targetOp int) script.ServerTriggerType {
    switch targetOp {
    case targetOpLocT:
        return script.TriggerOpLocT
    case targetOpLocU:
        return script.TriggerOpLocU
    case targetOpNpcT:
        return script.TriggerOpNpcT
    case targetOpNpcU:
        return script.TriggerOpNpcU
    case targetOpPlayerT:
        return script.TriggerOpPlayerT
    case targetOpPlayerU:
        return script.TriggerOpPlayerU
    case targetOpObjT:
        return script.TriggerOpObjT
    case targetOpObjU:
        return script.TriggerOpObjU
    }
    if targetOp < 1 || targetOp > 5 {
        return script.ServerTriggerType(-1)
    }
    offset := script.ServerTriggerType(targetOp - 1)
    switch target.(type) {
    case *Npc:
        return script.TriggerOpNpc1 + offset
    case *entitypkg.Loc:
        return script.TriggerOpLoc1 + offset
    case *entitypkg.Obj:
        return script.TriggerOpObj1 + offset
    case *Player:
        return script.TriggerOpPlayer1 + offset
    }
    return script.ServerTriggerType(-1)
}
```

### §2.3 `defaultOp` wire format

Replace `interaction.go:472`:

```go
// before (NAI-147 T4)
p.MessageGame(fmt.Sprintf("No trigger for [%d,%s]", p.targetOp+7, debugname))

// after (NAI-148)
trigger := tsTriggerForOpFire(p.target, p.targetOp)
p.MessageGame(fmt.Sprintf("No trigger for [%s,%s]", trigger, debugname))
```

Update the doc-comment block above `defaultOp` (interaction.go:456-466) to retire the `NAI-147-D-TRIGGER-NAME-NUMERIC` deviation note; preserve the `NAI-78-D-DEBUG-MSG-DEFERRED` closure crumb (still relevant context).

## §3 Test surface

### §3.1 `pkg/script/trigger_test.go` (new file)

Single test `TestServerTriggerType_String` with table-driven cases covering:

- **Constant-prefix free-of-underscore:** `TriggerProc → "proc"`, `TriggerLabel → "label"`, `TriggerDebugProc → "debugproc"`.
- **AP/OP families (1-5 + U + T):** `TriggerApNpc1 → "apnpc1"`, `TriggerApNpcU → "apnpcu"`, `TriggerApNpcT → "apnpct"`, `TriggerOpNpc1 → "opnpc1"`, `TriggerOpNpcT → "opnpct"`, `TriggerOpLoc1 → "oploc1"`, `TriggerOpObj1 → "opobj1"`, `TriggerOpPlayer1 → "opplayer1"`, `TriggerOpHeld1 → "opheld1"`, `TriggerOpHeldT → "opheldt"`.
- **AI prefix retains underscore:** `TriggerAiApNpc1 → "ai_apnpc1"`, `TriggerAiOpNpc5 → "ai_opnpc5"`, `TriggerAiQueue4 → "ai_queue4"`, `TriggerAiTimer → "ai_timer"`, `TriggerAiSpawn → "ai_spawn"`, `TriggerAiDespawn → "ai_despawn"`, `TriggerAiWalkTrigger → "ai_walktrigger"`.
- **Compound underscore retention:** `TriggerIfButton → "if_button"`, `TriggerIfClose → "if_close"`, `TriggerInvButton1 → "inv_button1"`, `TriggerInvButtonD → "inv_buttond"`.
- **Single-token specials:** `TriggerSoftTimer → "softtimer"`, `TriggerTimer → "timer"`, `TriggerWalkTrigger → "walktrigger"`, `TriggerLogin → "login"`, `TriggerLogout → "logout"`, `TriggerTutorial → "tutorial"`, `TriggerAdvanceStat → "advancestat"`, `TriggerMapZone → "mapzone"`, `TriggerMapZoneExit → "mapzoneexit"`, `TriggerZone → "zone"`, `TriggerZoneExit → "zoneexit"`, `TriggerChangeStat → "changestat"`.
- **Unknown-value fallback:** `ServerTriggerType(9999).String() → "trigger_9999"`, `ServerTriggerType(22).String() → "trigger_22"` (gap value), `ServerTriggerType(-1).String() → "trigger_-1"`.

Failures fail the table row and continue (per goscape convention `t.Errorf` not `t.Fatalf` for table tests).

### §3.2 `modules/world/interaction_test.go` — `tsTriggerForOpFire` matrix

Single test `TestTsTriggerForOpFire` with table-driven cases (12 rows + 2 fallback rows):

| target ctor                         | targetOp           | want                           |
|-------------------------------------|--------------------|--------------------------------|
| `&Npc{...}`                         | `1`                | `script.TriggerOpNpc1`         |
| `&Npc{...}`                         | `5`                | `script.TriggerOpNpc5`         |
| `&Npc{...}`                         | `targetOpNpcU`     | `script.TriggerOpNpcU`         |
| `&Npc{...}`                         | `targetOpNpcT`     | `script.TriggerOpNpcT`         |
| `entitypkg.NewLoc(...)`             | `2`                | `script.TriggerOpLoc2`         |
| `entitypkg.NewLoc(...)`             | `targetOpLocT`     | `script.TriggerOpLocT`         |
| `entitypkg.NewObj(...)`             | `3`                | `script.TriggerOpObj3`         |
| `entitypkg.NewObj(...)`             | `targetOpObjU`     | `script.TriggerOpObjU`         |
| `&Player{...}`                      | `4`                | `script.TriggerOpPlayer4`      |
| `&Player{...}`                      | `targetOpPlayerT`  | `script.TriggerOpPlayerT`      |
| `&Player{...}` (T-sentinel mismatch) | `targetOpNpcT`    | `script.TriggerOpNpcT`         |
| `&Npc{...}` (sentinel mismatch)     | `targetOpLocT`     | `script.TriggerOpLocT`         |
| `nil`                               | `1`                | `script.ServerTriggerType(-1)` |
| `&Npc{...}`                         | `99` (bad slot)    | `script.ServerTriggerType(-1)` |

The two "sentinel mismatch" rows pin §2.2's TS-faithful semantics: sentinels override target type because TS `targetOp+7` does not consult target type either.

### §3.3 Update existing `TestDefaultOp_NoTriggerEmitsDebug_NodeDebugTrue`

`modules/world/interaction_default_op_debug_test.go:32-56` currently asserts `[8,test_npc]` and carries a `_ = script.TriggerOpNpc1 // deviation doc anchor` line. After NAI-148:

- Replace `wantDebug := []byte("No trigger for [" + strconv.Itoa(1+7) + ",test_npc]")` with `wantDebug := []byte("No trigger for [opnpc1,test_npc]")`.
- Drop the `_ = script.TriggerOpNpc1` deviation-anchor and accompanying comment lines (NAI-147-D-TRIGGER-NAME-NUMERIC retired); the import remains used by other tests in the file (`strconv` may also become unused if only this test referenced it — verify and drop if so).
- Update the file-level `// NAI-147 T4` header comment to add a `// NAI-148: TS-faithful trigger names via ServerTriggerType.String()` line.

The 14 other `TestDefaultOp_*` tests in the same file all assert `,debugname]` substrings; none read the trigger-name half. Verified by reading lines 78-294 — none break.

## §4 Tracked deviations

| ID                    | Site                                            | Description                                                                                                                                                                                                | Rationale                                                                                                                                                                                                                                                                                                                                                                              |
|-----------------------|-------------------------------------------------|------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `NAI-148-D-STRING-FALLBACK` | `pkg/script/trigger.go` `String()`        | Unknown `ServerTriggerType` returns `"trigger_<N>"` instead of throwing. TS `ServerTriggerType[N].toLowerCase()` would `TypeError` on an undefined reverse-lookup at runtime.                              | JS implicit error not portable; Go has no equivalent of `undefined.method` semantics. The only consumer is debug-only chat (`defaultOp` under `cfg.NodeDebug`) — a sentinel preserves debuggability. Documented inline; no follow-up.                                                                                                                                                  |
| `NAI-148-D-OPFIRE-FALLBACK` | `interaction.go` `tsTriggerForOpFire`     | Returns `script.ServerTriggerType(-1)` (which `String()`s to `"trigger_-1"`) when `target == nil`, target is an unrecognised entity, or `targetOp` is out-of-range (not 1..5 and not a known sentinel).    | Defensive — `tryInteract` gates `target != nil` upstream (interaction.go:374-378 post-NAI-147 T5) and `targetOp` is set by `SetInteraction` from controlled call sites. TS source has no analogue (it would throw via the same `undefined.toLowerCase()` mechanism). Per `defensive_gate_doc_comment_label.md`, label inline with `(goscape defensive; TS would throw)`. No follow-up. |

## §5 Tasks

Three TDD tasks, one combined end-of-impl reviewer (Sonnet) per `superpowers_code_reviewer_model.md`. Linear dependency: T1 → T2 → T3 (T2 imports T1's `String()`; T3 wires T2's helper).

### §5.1 T1 — `ServerTriggerType.String()` table + tests

- **Add:** `pkg/script/trigger.go` — `serverTriggerNames` map (151 entries) + `String() string` method + `fmt` import.
- **Add:** `pkg/script/trigger_test.go` — table-driven `TestServerTriggerType_String` covering §3.1 spread (≥30 rows + 3 fallback rows).
- **Verify:** `go test ./pkg/script/...` passes; no new lints (`go vet ./...`).

### §5.2 T2 — `tsTriggerForOpFire` helper + tests

- **Add:** `modules/world/interaction.go` — `tsTriggerForOpFire(target entity, targetOp int) script.ServerTriggerType` placed adjacent to existing `defaultOpDebugname` helper.
- **Add:** `modules/world/interaction_default_op_debug_test.go` (or sibling new file) — table-driven `TestTsTriggerForOpFire` covering §3.2 14-row matrix.
- **Verify:** `go test ./modules/world/...` passes; no new lints.

### §5.3 T3 — wire `defaultOp`; update existing T4 fixture

- **Edit:** `modules/world/interaction.go:472` — replace `%d` → `%s`, `p.targetOp+7` → `tsTriggerForOpFire(p.target, p.targetOp)`. Update the doc-comment block (interaction.go:456-466) per §2.3.
- **Edit:** `modules/world/interaction_default_op_debug_test.go:32-56` — `wantDebug` change + drop `_ = script.TriggerOpNpc1` anchor + header comment update per §3.3.
- **Verify:** full repo `go test -race ./...` passes; `go vet ./...` clean.

## §6 Memory entries to apply at close

- `compressed_cadence` — 15-100 LOC band, separate spec/plan, single combined review.
- `superpowers_code_reviewer_model` — Sonnet reviewer (never Opus).
- `defensive_gate_doc_comment_label` — `NAI-148-D-OPFIRE-FALLBACK` labelled inline in `tsTriggerForOpFire`.
- `true_to_ts_gate` — both deviations tracked with rationale + no follow-up.
- `close_commit_memory_trailer` — `Closes memory: nai_followups.md NAI-147-D-TRIGGER-NAME-NUMERIC` on close commit.
- `verify_implementer_claims` — controller verifies fresh build/test green at each task HEAD.
- `feedback_subagent_wt_path` — `git status` after each commit confirms clean main working tree.

## §7 Smoke / PRIMARY pin

**No PRIMARY smoke required.** The change affects only the `cfg.NodeDebug=true` debug chat — invisible in production builds. SECONDARY pins (test-only) cover:

- §3.1 unit tests (`String()` table — broad spread + fallback).
- §3.2 unit tests (`tsTriggerForOpFire` matrix — 4 entity types × {1..5, U, T} + sentinel mismatches + fallback).
- §3.3 integration (existing `TestDefaultOp_NoTriggerEmitsDebug_NodeDebugTrue` updated to assert TS-faithful `[opnpc1,test_npc]`).
- Adjacency fence: 14 unchanged `TestDefaultOp_*` tests in `interaction_default_op_debug_test.go` continue to pass (verified at §3.3).

If a future user-driven smoke runs with `NodeDebug=true` and observes unexpected trigger-name strings, route as NAI-148 carry-forward (no anticipated case at spec-write).

## §8 Out of scope

- **Other `defaultOp` deviations.** All other NAI-78 deviations (NULL-TYPE-GUARD-OMITTED, HASINTERACTION-GUARD, DEBUG-MSG-DEFERRED) closed at NAI-147.
- **`Cfg.NodeDebug` other consumers.** Only `defaultOp` consults it for trigger-name purposes; no audit-sweep here.
- **`NpcMode` String() method.** TS `TargetOp = ServerTriggerType | NpcMode`; goscape's NPC-side `targetOp` uses `objtype.NPCMode*` constants, but no equivalent missing-trigger debug chat fires from NPC code paths today. If one is added later, route as a separate sub-spec.

## §9 Risk register

| ID | Risk                                                                                                | Likelihood | Impact | Mitigation                                                                                                                                                |
|----|-----------------------------------------------------------------------------------------------------|-----------:|-------:|-----------------------------------------------------------------------------------------------------------------------------------------------------------|
| R1 | Off-by-one in `serverTriggerNames` table — wrong name keyed to a constant.                          |  LOW       | LOW    | T1 tests pin the spread by name + fallback. Reviewer Sonnet verifies via grep against `ServerTriggerType.ts:1-162`.                                       |
| R2 | `tsTriggerForOpFire` returns the wrong OP\* when sentinel + target type mismatch (e.g. NpcT + Player target — used by `TestDefaultOp_DebugnameTBranch_NpcTGuarded`). | LOW | LOW | §3.2's two "sentinel mismatch" rows pin TS-faithful semantics: sentinel wins over target type (matches TS `targetOp+7`-only logic). |
| R3 | `entity` interface in `interaction.go` does not actually accept the four `*Npc/*Player/*Loc/*Obj` types via type-switch. | LOW | LOW | Existing `defaultOpDebugname` already type-switches on the same concrete types (interaction.go:482-503). Pattern already proven. |
| R4 | `String()` method on `ServerTriggerType` shadows existing pretty-printer somewhere. | LOW | LOW | grep confirms no existing `String()` on `ServerTriggerType` at HEAD `b02bd72`. Adding one only changes `%v`/`%s` rendering — no production code relies on the bare numeric in user output. |

## §10 References

- TS source:
  - `Engine-TS/src/engine/script/ServerTriggerType.ts:1-172` — full enum + toString.
  - `Engine-TS/src/engine/entity/Player.ts:1072-1097` — `defaultOp` body.
  - `Engine-TS/src/engine/entity/PathingEntity.ts:28,66,510-516` — `TargetOp` type, `targetOp` field, `setInteraction`.
  - `Engine-TS/src/engine/script/handlers/PlayerOps.ts:399,413,420,1005,1019,1134` — `setInteraction(..., ServerTriggerType.AP*+type, ...)` call sites confirming AP\*-namespace storage.
- Goscape anchors at HEAD `b02bd72`:
  - `pkg/script/trigger.go:1-173` — `ServerTriggerType` constants.
  - `modules/world/interaction.go:36-45` — `targetOp{Loc,Npc,Player,Obj}{T,U}` sentinels.
  - `modules/world/interaction.go:90-149` — `(*Player).SetInteraction` (sets `p.targetOp = op`).
  - `modules/world/interaction.go:467-477` — `defaultOp` (NAI-147 T4).
  - `modules/world/interaction.go:482-540` — `defaultOpDebugname` (TS L1077-1090 fan-out, type-switch precedent).
  - `modules/world/interaction_default_op_debug_test.go:1-294` — full NAI-147 T4 test suite.
- Tracker: `nai_followups.md` — `NAI-147-D-TRIGGER-NAME-NUMERIC` (currently in NAI-147 close block at git `b02bd72`; `nai_followups.md` text append still pending — captured in close commit body).
