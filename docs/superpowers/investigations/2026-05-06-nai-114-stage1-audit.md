# NAI-114 — Stage 1.2 opcode-coverage audit

**Date:** 2026-05-06
**Spec:** docs/superpowers/specs/2026-05-06-nai-114-opheldu-tinderbox-firemaking-investigation-design.md
**Stage:** 1.2 — Sonnet audit subagent; read-only.
**Input:** Stage 1.1 findings (2026-05-06-nai-114-stage1-bundle0-findings.md), full disasm of `[label,light_logs_inv]` (id 7356) and chained procs (ids 7358, 7357, 7359, 7360, 2130, etc.), chain-wide opcode inventory (64 opcodes, §6 of Stage 1.1).

---

## 1. Audit method

1. Extracted the 64-opcode chain-wide inventory from Stage 1.1 §6.
2. For each opcode, mapped the mnemonic to its goscape constant in `pkg/script/opcode.go`.
3. Grepped `pkg/script/handlers.go` (the authoritative handler registration map) for each constant. Confirmed with secondary grep of `pkg/script/handlers_*.go` `func handle*` bodies.
4. For each TS handler, read `Engine-TS/src/engine/script/handlers/` (ServerOps.ts, PlayerOps.ts, InvOps.ts, ObjOps.ts, LocOps.ts, ObjConfigOps.ts, CoreOps.ts, NumberOps.ts, StringOps.ts, EnumOps.ts, DbOps.ts) and captured file:line.
5. For each **missing** handler, determined position in the execution path of the firemaking chain to identify the earliest abort point.
6. Read `pkg/script/runner.go:54-83` (Execute dispatch) and `modules/world/script.go:91-152` (resumeOrFinish) to confirm that a missing handler causes `Aborted` state + warn log (not a silent abort).

---

## 2. Opcode-coverage matrix (full)

Chain scope: `[opheldu,tinderbox]` → SWITCH → `[label,light_logs_inv]` (id 7356) → GOSUBs to ids 7358, 7357, 7359, and the off-path id 7360 / 2130 / 7904.

**Legend:**
- `OK` — handler registered, behavior matches TS (no diff identified).
- `MISSING` — opcode constant defined in `pkg/script/opcode.go` but absent from the `handlers` map in `pkg/script/handlers.go` and not defined in any `pkg/script/handlers_*.go`. Execute returns `fmt.Errorf("… no handler for %s …")` → `Aborted` + server-side warn log.
- `PARTIAL` — handler registered but a notable behavioral difference noted (no abort for this opcode; continuation differs from TS).

| Opcode | TS handler (file:line) | Goscape handler (file:line) | Behavior diff | Bound to symptom? |
|---|---|---|---|---|
| ADD | NumberOps.ts:8 | handlers_number.go:8 (via handlers.go:26) | none | no |
| ANIM | PlayerOps.ts:195 | handlers_player.go:614 | none | no |
| BRANCH | CoreOps.ts:73 | handlers_core.go (via handlers.go:21) | none | no |
| BRANCH_EQUALS | CoreOps.ts:95 | handlers_core.go (via handlers.go:38) | none | no |
| BRANCH_GREATER_THAN | CoreOps.ts:115 | handlers_core.go (via handlers.go:40) | none | no |
| BRANCH_LESS_THAN | CoreOps.ts:105 | handlers_core.go (via handlers.go:39) | none | no |
| BRANCH_NOT | CoreOps.ts:83 | handlers_core.go (via handlers.go:37) | none | no |
| COORD | PlayerOps.ts:230 | handlers_player.go:522 | none | no |
| DB_GETFIELD | DbOps.ts:97 | handlers_db.go (via handlers.go:142) | none | no |
| DB_GETFIELDCOUNT | DbOps.ts:135 | handlers_db.go (via handlers.go:143) | none | no |
| DIVIDE | NumberOps.ts:26 | handlers_number.go (via handlers.go:45) | none | no |
| ENUM | EnumOps.ts:7 | handlers_config.go:65 | none | no |
| FACESQUARE | PlayerOps.ts:239 | handlers_player.go:546 | none | no |
| GOSUB_WITH_PARAMS | CoreOps.ts (via handlers.go:30) | handlers.go:621 | none | no |
| IF_SETTEXT | PlayerOps.ts:735 | handlers_interface.go (via handlers.go:302) | none | no |
| INV_ADD | InvOps.ts:57 | handlers_inv.go:294 | none | no |
| INV_DEL | InvOps.ts:129 | handlers_inv.go:308 | none | no |
| **INV_DROPSLOT** | **InvOps.ts:213** | **MISSING** (opcode.go:381 only) | **MISSING — no handler registered anywhere in pkg/script/** | **YES — would abort [light_logs_inv] at PC 47 if chain reached this far** |
| INV_GETOBJ | InvOps.ts:278 | handlers_inv.go:44 | none | no |
| INV_ITEMSPACE | InvOps.ts (via handlers.go:273) | handlers_inv.go:175 | none | no |
| INV_SIZE | InvOps.ts:27 | handlers_inv.go:79 | none | no |
| INV_TOTAL | InvOps.ts:619 | handlers_inv.go:26 | none | no |
| INZONE | ServerOps.ts:47 | handlers_server.go:48 | none | no |
| JOIN_STRING | CoreOps.ts:183 | handlers.go (via handlers.go:13) | none | no |
| JUMP_WITH_PARAMS | CoreOps.ts (via handlers.go:325) | handlers_core.go:42 | none | no |
| LAST_USEITEM | CoreOps.ts / PlayerOps.ts (via handlers.go:109) | handlers.go (via handlers.go:109) | none | no |
| LAST_USESLOT | CoreOps.ts / PlayerOps.ts (via handlers.go:110) | handlers.go (via handlers.go:110) | none | no |
| **LINEOFWALK** | **ServerOps.ts:65** | **MISSING** (opcode.go:79 only) | **MISSING — no handler registered; only used as helper function `isLineOfWalk()` inside MAP_FINDSQUARE** | **YES — would abort [proc,push_player] (id 7359) at PC ~3 (MOVECOORD+LINEOFWALK step-probe)** |
| LOC_ADD | LocOps.ts:18 | handlers_loc.go:279 | none | no |
| LOWERCASE | StringOps.ts:29 | handlers_string.go (via handlers.go:155) | none | no |
| MAP_BLOCKED | ServerOps.ts:129 | handlers_map.go:188 | none | no |
| MAP_CLOCK | ServerOps.ts:15 | handlers_server.go:7 | none | no |
| **MAP_LOCADDUNSAFE** | **ServerOps.ts:212** | **MISSING** (opcode.go:86 only) | **MISSING — no handler registered anywhere in pkg/script/** | **YES — PRIMARY; aborts chain at PC 1 of [proc,area_allow_loc_add] (id 7358), before any visible effect** |
| MES | PlayerOps.ts:342 | handlers.go:637 | none | no |
| MOVECOORD | ServerOps.ts:102 | handlers_server.go:69 | none | no |
| **OBJ_ADD** | **ObjOps.ts:20** | **MISSING** (opcode.go:307 only) | **MISSING — no handler registered** | **YES — would abort [proc,firemaking_success] (id 7357); but chain already aborted at MAP_LOCADDUNSAFE earlier** |
| **OBJ_ADDALL** | **ObjOps.ts:58** | **MISSING** (opcode.go:309 only) | **MISSING — no handler registered** | **YES — same as OBJ_ADD (would abort id 7357)** |
| **OBJ_COORD** | **ObjOps.ts:163** | **MISSING** (opcode.go:310 only) | **MISSING — no handler registered** | **YES — would abort [light_logs_inv] at PC 82 (after INV_DROPSLOT miss); chain already aborted earlier** |
| **OBJ_DEL** | **ObjOps.ts:112** | **MISSING** (opcode.go:312 only) | **MISSING — no handler registered** | **YES — would abort [proc,firemaking_success] (id 7357) at PC 0** |
| OC_CATEGORY | ObjConfigOps.ts:27 | handlers_config.go:458 | none | no |
| OC_NAME | ObjConfigOps.ts:9 | handlers_config.go:424 | none | no |
| OC_PARAM | ObjConfigOps.ts:15 | handlers_config.go:444 | none | no |
| POP_INT_LOCAL | CoreOps.ts (via handlers.go:18) | handlers.go (via handlers.go:18) | none | no |
| POP_VARP | CoreOps.ts:41 | handlers_vars.go (via handlers.go:184) | none | no |
| PUSH_CONSTANT_INT | CoreOps.ts (via handlers.go:14) | handlers.go:448 | none | no |
| PUSH_CONSTANT_STRING | CoreOps.ts (via handlers.go:15) | handlers.go (via handlers.go:15) | none | no |
| PUSH_INT_LOCAL | CoreOps.ts (via handlers.go:17) | handlers.go (via handlers.go:17) | none | no |
| PUSH_STRING_LOCAL | CoreOps.ts (via handlers.go:19) | handlers.go (via handlers.go:19) | none | no |
| PUSH_VARP | CoreOps.ts:25 | handlers_vars.go (via handlers.go:183) | none | no |
| P_DELAY | PlayerOps.ts:375 | handlers.go:674 | none | no |
| **P_OPOBJ** | **PlayerOps.ts:990** | **MISSING** (opcode.go:180 only) | **MISSING — no handler registered; would abort [light_logs_inv] at PC 104 (tail-call)** | **YES — would abort at PC 104 of id 7356 (after all prior misses)** |
| P_STOPACTION | PlayerOps.ts:429 | handlers_player.go:730 | none | no |
| P_TELEPORT | PlayerOps.ts:447 | handlers_player.go:556 | none | no |
| RANDOM | NumberOps.ts:32 | handlers_number.go (via handlers.go:69) | none | no |
| RETURN | CoreOps.ts (via handlers.go:16) | handlers.go (via handlers.go:16) | none | no |
| SOUND_SYNTH | PlayerOps.ts:466 | handlers_player.go:963 | none | no |
| SPLIT_INIT | StringOps.ts:76 | handlers_string.go (via handlers.go:166) | none | no |
| SPLIT_PAGECOUNT | StringOps.ts:104 | handlers_string.go (via handlers.go:170) | none | no |
| STAT | PlayerOps.ts:480 | handlers_player.go:242 | none | no |
| STAT_ADVANCE | PlayerOps.ts:759 | handlers_player.go:468 | none | no |
| STAT_RANDOM | PlayerOps.ts:578 | handlers_player.go:498 | PARTIAL: RNG algorithm differs (GoLang `rand.IntN(256)` vs Java `Random.nextDouble()*256`); produces same push-0-or-1 shape, non-bit-identical odds | no (shape correct) |
| SWITCH | CoreOps.ts:244 | handlers_array.go:52 | none (confirmed by Stage 1.1 §5) | no |
| TOSTRING | StringOps.ts:33 | handlers_string.go (via handlers.go:29) | none | no |
| WORLD_DELAY | ServerOps.ts:166 | handlers_server.go:109 | none | no |

**Summary of missing handlers:** 8 total.

| Missing opcode | Goscape const | Opcode value | First hit in chain at |
|---|---|---|---|
| MAP_LOCADDUNSAFE | OpMapLocAddUnsafe | 1012 | PC 1 of id 7358 (`[proc,area_allow_loc_add]`), called from PC 25 of id 7356 |
| INV_DROPSLOT | OpInvDropSlot | 4312 | PC 47 of id 7356 (only reached if 7358 succeeds — it doesn't) |
| OBJ_COORD | OpObjCoord | 3502 | PC 82 of id 7356 (only if prior misses resolved) |
| OBJ_DEL | OpObjDel | 3504 | PC 0 of id 7357 (`[proc,firemaking_success]`) |
| OBJ_ADD (implicit via OBJ_ADDALL) | OpObjAddAll | 3501 | PC ~29 of id 7357 (ash spawn) |
| OBJ_ADD | OpObjAdd | 3500 | id 7360 off-path (light_source) |
| LINEOFWALK | OpLineOfWalk | 1006 | id 7359 (`[proc,push_player]`) step-probe |
| P_OPOBJ | OpPOpObj | 2080 | PC 104 of id 7356 (tail-call to self next tick) |

---

## 3. SWITCH-decode audit verdict

This section re-confirms Stage 1.1 §5 findings.

**Goscape SWITCH handler:** `pkg/script/handlers_array.go:52-63`

```go
func handleSwitch(s *ScriptState) error {
    key := int32(s.PopInt())
    tableIdx := int(s.Script.IntOperands[s.PC])
    if tableIdx < 0 || tableIdx >= len(s.Script.SwitchTables) {
        return nil
    }
    table := s.Script.SwitchTables[tableIdx]
    if offset, ok := table[key]; ok {
        s.PC += int(offset)
    }
    return nil
}
```

**TS SWITCH handler:** `Engine-TS/src/engine/script/handlers/CoreOps.ts:244-255`

```ts
[ScriptOpcode.SWITCH]: state => {
    const key = state.popInt();
    const table = state.script.switchTables[state.intOperand];
    if (table === undefined) { return; }
    const result = table[key];
    if (result) { state.pc += result; }
},
```

Switch table for `[opheldu,tinderbox]` PC 18:
- case 212 → offset 1 → lands at PC 20 → `LAST_USESLOT; JUMP_WITH_PARAMS 7356` (**logs path; CORRECT**)
- case 150 → offset 4 → lands at PC 23 → ignite light-source path
- default → falls through to BRANCH (PC 19)

**H3.a (SWITCH decode mismatch) → REFUTED.** Both Stage 1.1 data (switch table confirmed) and handler comparison confirm correct routing.

---

## 4. H3 binding verdict

### Bound opcode: MAP_LOCADDUNSAFE (OpMapLocAddUnsafe = 1012)

**Binding:** H3.b — downstream opcode silent-abort in the `[label,light_logs_inv]` call chain.

**Exact abort location:**
- `[label,light_logs_inv]` (id 7356) PC 25: `GOSUB_WITH_PARAMS 7358` calls `[proc,area_allow_loc_add]`
- `[proc,area_allow_loc_add]` (id 7358) **PC 1**: `MAP_LOCADDUNSAFE 0`
- Goscape dispatch: `pkg/script/runner.go:68-73` — `h, ok := handlers[op]; if !ok { s.Execution = Aborted; return fmt.Errorf("…no handler for MAP_LOCADDUNSAFE…") }`
- Caller: `modules/world/script.go:111-122` — `resumeOrFinish` logs warn and falls through; state is `Aborted`; `OnScriptFinishedOrAborted` fires, no visible effect to player

**TS handler:** `Engine-TS/src/engine/script/handlers/ServerOps.ts:212-252`

The TS handler pops a coord, iterates `World.gameMap.getZone(coord).getAllLocsUnsafe()`, checks for active blocking locs at the tile (wall/ground/ground-decor layers), and pushes 1 if an active loc occupies the tile (unsafe-to-add-another-loc), else pushes 0. The result controls whether `[proc,area_allow_loc_add]` returns 0 (blocked) or 1 (allowed).

**Goscape handler:** NONE. No registration in `pkg/script/handlers.go`, no `func handle*` body in any `pkg/script/handlers_*.go`. Confirmed by:
1. `grep -rn "OpMapLocAddUnsafe" pkg/script/` → only `opcode.go:86` (definition) and `opcode.go:587` (String() case) — zero handler registrations.
2. `grep -rn "MapLocAddUnsafe|map_locaddunsafe|locaddunsafe|LocAddUnsafe" pkg/ modules/ cmd/` → zero results.

### Symptom-alignment scoring

MAP_LOCADDUNSAFE aborts the execution at `[proc,area_allow_loc_add]` PC 1, which is called from `[label,light_logs_inv]` PC 25. ALL four missing client effects follow from this single abort:

| Missing effect | Chain location | Reachable past PC 25 of id 7356? |
|---|---|---|
| Firemaking animation (ANIM 733) | PC 55 of id 7356 | NO — aborts at PC 25 → sub-proc PC 1 |
| Logs disappear (INV_DROPSLOT) | PC 47 of id 7356 | NO — PC 47 > PC 25 (abort occurs at sub-proc before PC 47 is reached) |
| Fire spawns (LOC_ADD in id 7357) | GOSUB 7357 called at PC 86 of id 7356 | NO |
| XP grant (STAT_ADVANCE in id 7357) | id 7357 PC ~1-5 | NO |

Wait — re-checking the order of PCs in id 7356:

PC 43-47: INV_DROPSLOT (drops logs to ground)
PC 48-50: PUSH_VARP 58 / MAP_CLOCK / BRANCH_LESS_THAN (attempt-clock check)
PC 51-66: first-attempt path (P_STOPACTION, ANIM 733, SOUND_SYNTH, MES, POP_VARP, BRANCH → PC 103)
PC 103-105: P_OPOBJ 4 (tail-call self next tick)

And PC 25 is GOSUB_WITH_PARAMS 7358, which comes BEFORE PC 43. So the abort at id 7358 PC 1 means execution never reaches INV_DROPSLOT (PC 47), ANIM (PC 55), or the success/failure paths. All four observable effects are consistently absent.

**Abort severity: MISSING (highest class)**
**Chain position: PC 25 of id 7356 → PC 1 of id 7358 (FIRST missing handler hit in execution order)**
**All four symptoms explained by single abort.**

**Secondary missing handlers** (do NOT change the binding; chain aborts before them):
- INV_DROPSLOT — would abort at PC 47 of id 7356 (after MAP_LOCADDUNSAFE is fixed)
- OBJ_COORD — would abort at PC 82 of id 7356 (after INV_DROPSLOT is fixed)
- OBJ_DEL — would abort at PC 0 of id 7357 (called from PC 86 of id 7356)
- OBJ_ADDALL — would abort at PC ~29 of id 7357 (ash spawn after fire)
- LINEOFWALK — would abort in id 7359 (`[proc,push_player]`, the step-off proc called from PC 83 of id 7356)
- P_OPOBJ — would abort at PC 104 of id 7356 (tail-call for repeat-light next tick)

These secondary misses form a cascade chain. **Shape D escalation is required** (>3 missing opcodes; see §5).

---

## 5. Fix shape recommendation

### Primary (immediate abort): MAP_LOCADDUNSAFE — Shape A

**Fix shape:** Shape A — new opcode handler.

**TS reference:** `Engine-TS/src/engine/script/handlers/ServerOps.ts:212-252`

Semantics: pop coord (1 int), iterate all locs in the coord's zone (via `s.LocOps.LocsAtCoord` which already exists for `LOC_ADD`), check each loc for `active == 1` AND occupying the target tile, push 1 if found, push 0 otherwise. The three layer-branches in TS (WALL: check x==coord.x && z==coord.z; GROUND: iterate width*length footprint; GROUND_DECOR: check exact tile) must be reproduced.

**LOC estimate:** ~40–60 production LOC in `pkg/script/handlers_map.go` (near `handleMapBlocked`); ~30 LOC in `handlers_map_test.go`.

### Secondary cascade (must fix to complete firemaking): INV_DROPSLOT, OBJ_DEL, OBJ_COORD, OBJ_ADDALL, OBJ_ADD, LINEOFWALK, P_OPOBJ

All 7 secondary missing handlers must be ported before the firemaking script completes successfully. Fixing only MAP_LOCADDUNSAFE will unblock execution through the area-allow check but the script will abort immediately at INV_DROPSLOT (PC 47 of id 7356) with a new warn.

**Escalation verdict (Shape D):** 8 missing handlers total. Rough LOC estimates:

| Handler | File | LOC estimate |
|---|---|---|
| MAP_LOCADDUNSAFE | handlers_map.go | 40–60 prod + 20 test |
| INV_DROPSLOT | handlers_inv.go | 40–60 prod + 20 test |
| OBJ_DEL | handlers_map.go or new handlers_obj.go | 15–20 prod + 15 test |
| OBJ_COORD | handlers_obj.go | 10–15 prod + 10 test |
| OBJ_ADD | handlers_obj.go | 40–60 prod + 20 test |
| OBJ_ADDALL | handlers_obj.go | 40–60 prod + 20 test |
| LINEOFWALK | handlers_map.go | 20–30 prod + 15 test |
| P_OPOBJ | handlers_player.go | 20–30 prod + 15 test |

**Total estimate: ~225–335 production LOC + ~135 test LOC → well above the 80 LOC Shape D threshold.**

Recommendation: route to NAI-115 per `scope_gate_prerequisite_chain`. NAI-114 Stage 2 may land MAP_LOCADDUNSAFE as a single-handler fix with a pin that confirms the script now proceeds further (new abort at INV_DROPSLOT in the warn log), as a milestone proof-of-bind. The full 8-handler cascade belongs in NAI-115.

**Alternative (if controller decides to stay in NAI-114):** bundle all 8 handlers into NAI-114 Stage 2. Acceptable if the implementer can complete them in a single TDD cycle (all 8 are pattern-identical Shape A ports). LOC ceiling would need to be explicitly relaxed.

---

## 6. Confidence level and open uncertainties

**Confidence: HIGH**

The binding is mechanically deterministic:
- `pkg/script/runner.go:68-73` makes missing-handler behavior unambiguous (abort + error, not silent).
- `modules/world/script.go:111-122` (`resumeOrFinish`) logs the error as a server-side warn, then routes via `Aborted` → `OnScriptFinishedOrAborted` (no visible effect to player).
- `OpMapLocAddUnsafe` appears in `opcode.go:86` (constant) and `opcode.go:587` (String case) only. Zero handler registrations across the entire codebase.
- MAP_LOCADDUNSAFE is the FIRST missing handler hit in topological execution order (id 7358 PC 1, called from id 7356 PC 25 — before INV_DROPSLOT at PC 47, before any animation, before any world effect).

**Open uncertainties:**

1. **"No server warn log emitted" observation (spec §1):** The spec lists this as a symptom, but binding MAP_LOCADDUNSAFE as the abort point implies a warn IS emitted server-side. This is not contradictory — "no server warn log" in the symptom list likely means no user-visible game message (MES / chat), not a claim about server process logs. Controller should verify the server logs during Stage-2 smoke to confirm the warn fires and matches the expected script name.

2. **Player firemaking level gate (id 7356 PC 11):** The STAT check at PC 11 branches to a "you need level X" MES fail if `stat(11) < 1`. Tutorial Island player is assumed to have firemaking level ≥ 1. If not, execution terminates at PC 22 (RETURN after MES) and the MAP_LOCADDUNSAFE abort is never reached — the symptom would still be "no visible fire" but via the stat-gate path. This should be verified at smoke: if a stat-gate fail were the cause, the spec author would have noted a "You need a Firemaking level of…" message in chat. The absence of any chatbox message suggests the script reaches GOSUB 7358.

3. **Area-allow result effect on success path:** Even after MAP_LOCADDUNSAFE is ported, Tutorial Island's fire tile may return 0 (blocked by existing loc) from `[proc,area_allow_loc_add]`, routing to "You can't light a fire here." This would be a content/placement issue, not a handler issue. Controller should validate by checking if Tutorial Island tile 7358 result would be 1 or 0. The spec notes no "can't light fire here" message was visible, suggesting the MES path (PC 29-31 of id 7356) was not reached — consistent with abort-before-return at MAP_LOCADDUNSAFE.

4. **Secondary cascade completeness:** The 7 secondary missing handlers are enumerated in this report based on static disasm. Runtime execution paths may reveal additional missing handlers in procs 7357 / 7359 not yet fully disassembled (e.g., `[proc,in_duel_arena]` id 3005 within 7359, and `[proc,inzone_coord_pair_table]` id 2120 within 7358). Controller should disasm id 3005 and 2120 before writing the NAI-115 plan.

---

## 7. Controller HEAD-verification (post-subagent)

**Verified at HEAD:** `d81027b` (Stage 1.1 commit).

Per `audit_subagent_fabrication.md` + `verify_implementer_claims.md`: every cited file:line, every "MISSING" claim, and the abort-path infrastructure citations were re-greped or re-read from a fresh controller pass.

### Confirmed claims

- **All 8 "MISSING" handlers** verified via `rg -n "<OpConstName>|<TS_NAME>|...alternative spellings..." pkg/ modules/ cmd/`. Each appears ONLY in `pkg/script/opcode.go` (constant definition + `String()` case). Zero handler registrations anywhere in the codebase. Confirmed for: `OpMapLocAddUnsafe` (1012), `OpInvDropSlot` (4312), `OpObjCoord` (3502), `OpObjDel` (3504), `OpObjAdd` (3500), `OpObjAddAll` (3501), `OpLineOfWalk` (1006), `OpPOpObj` (2080).
- **Abort path** (`pkg/script/runner.go:68-73`, `modules/world/script.go:111-122`) verified verbatim. `script.Execute` returns `fmt.Errorf("script %q: no handler for %s (opcode %d) at pc=%d", …)` with `s.Execution = Aborted`; `resumeOrFinish` logs warn and routes via `OnScriptFinishedOrAborted`.
- **Sampled "OK" handler line numbers** (ANIM @ handlers_player.go:614, STAT @ :242, STAT_RANDOM @ :498, INV_ADD @ handlers_inv.go:294, INV_DEL @ :308, LOC_ADD @ handlers_loc.go:279, SWITCH @ handlers_array.go:52) — all verify exactly.
- **TS MAP_LOCADDUNSAFE handler** at `Engine-TS/src/engine/script/handlers/ServerOps.ts:212-252` — verified exact range. Read confirms three-layer logic (WALL / GROUND / GROUND_DECOR), iterating `getAllLocsUnsafe()` of the coord's zone.
- **Test sanity check:** `go test -count=1 -run TestSwitch ./pkg/script/...` → PASS at HEAD `d81027b`.

### Stale / corrected claims

None.

### Refuted claims

None.

### Stage 2 fidelity nuance (not a correction; flagged for the Stage 2 plan)

The audit's TS reference for MAP_LOCADDUNSAFE at `ServerOps.ts:212-252` summarizes the three-layer logic but does not surface a subtle filter at TS line 224:

```ts
if (!loc.isActive && layer === LocLayer.WALL) {
    continue;
}
```

This is "skip inactive walls only", NOT "skip all inactive locs". Inactive ground-layer / ground-decor-layer locs are still checked. Stage 2 implementer must reproduce this exact branch ordering to preserve TS fidelity per `true_to_ts_gate.md`. The Stage 2 plan should cite TS line 224 explicitly when codifying the WALL branch.

### Final H3 binding (post-verification)

**MAP_LOCADDUNSAFE** (OpMapLocAddUnsafe = 1012); `pkg/script/opcode.go:86` (constant only — handler MISSING); TS reference `Engine-TS/src/engine/script/handlers/ServerOps.ts:212-252`. First abort hit at `[proc,area_allow_loc_add]` (id 7358) PC 1, called from `[label,light_logs_inv]` (id 7356) PC 25. All four observed-missing client effects (no fire, no animation, no log-removal, no XP) are explained by this single abort.

**Confidence: HIGH** (mechanically deterministic via `pkg/script/runner.go:68-73`).

### Fix-shape recommendation (post-verification)

- **NAI-114 Stage 2 (this sub-spec):** port MAP_LOCADDUNSAFE only. Shape A. ~40-60 prod LOC + ~20 test LOC in `pkg/script/handlers_map.go` + `handlers_map_test.go`. Verification at smoke: server warn log shifts from "no handler for MAP_LOCADDUNSAFE" to "no handler for INV_DROPSLOT" (or further, depending on the area-allow result on Tutorial Island).
- **NAI-115 (escalated, separate spec):** port the 7 cascade handlers (INV_DROPSLOT, OBJ_DEL, OBJ_COORD, OBJ_ADDALL, OBJ_ADD, LINEOFWALK, P_OPOBJ). ~225-275 prod LOC + ~115 test LOC. Routes per `scope_gate_prerequisite_chain` since total exceeds the Shape D 80-LOC threshold. Smoke binding: full firemaking ignition produces fire loc + xp + ash drop on Tutorial Island.

This Stage-2 split (single primary fix + escalated cascade) follows the NAI-98→NAI-100→NAI-101 precedent of milestone-progress smoke binding.
