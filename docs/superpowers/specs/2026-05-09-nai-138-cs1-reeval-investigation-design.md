# NAI-138 — Run-toggle cs1 re-eval timing on bare varp echo (investigation + fix)

**Status:** spec
**Date:** 2026-05-09
**Predecessors:** NAI-137 (run varp clientcode-7 dynamic discovery) closed at `1bc1800`. PRIMARY closed TS-faithful; visual symptom (run-toggle button does not visually de-toggle when run-energy depletes to 0, despite server-correct `OpVarpSmall(173, 0)` on the wire) reframed as this sub-spec per `dispatch_correct_reach_blocked`. NAI-137 carryover queue (`nai_followups.md` line 6432) named this candidate verbatim.
**Tech stack:** Go 1.26+
**Cadence:** Investigation + fix (`investigation_subspec_cadence`). Bundle 0 controller pre-flight + Bundle 1 parallel Stage 1 audit (three Explore subagents) + Bundle 2 fix at indicated layer + smoke + Bundle 3 conditional templates. Single sub-spec must ship a fix at SOME layer (no "no-fix close" branch).

## 1. Goal

Identify the layer (Engine-TS port gap, Client-Java #225 cs1 re-eval semantics, Content script idiom, or goscape encoder defect) responsible for the run-toggle button not visually de-toggling at runenergy=0 despite a server-correct `OpVarpSmall(173, 0)` packet, then ship the corresponding fix.

The investigation must produce a binding verdict on:
- whether goscape's NAI-137 cutover missed a TS-side refresh emitter (engine port gap),
- whether OSRS client #225 re-evaluates `Component.script1` on bare varp packets for `buttontype=select` components (client semantics),
- and what the LostCity content `%v = %v` self-write idiom emits on the wire vs. compiles to (content semantics).

The fix layer is a function of the synthesis matrix (§5).

## 2. Background — anchored

### 2.1 NAI-137 close state (HEAD `1bc1800`)

- **Engine cutover:** `(*Player).updateEnergy` and `handlePRun` write `OpVarpSmall` to the cache-resolved run varp id (typically 173 = `option_run`, per `Content/scripts/interface_controls/configs/player_controls.varp:5-8` `clientcode=7`).
- **TS counterpart:** `Engine-TS/src/engine/entity/Player.ts:697-699` (energy=0 reset) and `Engine-TS/src/engine/script/handlers/PlayerOps.ts:1208` (P_RUN handler) — both bare `setVar(VarPlayerType.RUN, ...)`. NAI-137 close asserted these are the ONLY engine-side `VarPlayerType.RUN` consumers; this sub-spec's Bundle 0 re-grep verifies that assertion comprehensively.
- **Smoke result (2026-05-09):** SECONDARY met (clicking the toggle while running keeps state consistent across server-driven redraws). PRIMARY UNRESOLVED (button stays stuck-on at energy=0).

### 2.2 Click-path content scripts (already read)

`Content/scripts/interface_controls/scripts/player_controls.rs2:25-50`:

```
[if_button,controls:com_4]
if_close;
...
    p_run(^player_run_off);
...
%option_run = %option_run; // resync varp

[if_button,controls:com_5]
if_close;
...
        p_run(^player_run_off);
...
    p_run(^player_run_on);
...
%option_run = %option_run; // resync varp
```

Both click-driven scripts emit the run varp **TWICE**: once via `p_run` (which calls into the engine `P_RUN` handler), once via the explicit `%option_run = %option_run;` self-write (commented "resync varp"). The engine-side energy=0 path emits ONCE.

This is the load-bearing observation: **why does the click path explicitly re-emit, and why doesn't the engine-side energy=0 path?**

### 2.3 NAI-137 close-commit hypothesis (now under audit)

Per `1bc1800` commit body: "OSRS client #225 does not re-evaluate `buttontype=select` cs1 `script1op1=pushvar,option_run` binding scripts on bare varp echoes, requiring an accompanying interface-state event (the click path's `[if_button,controls:com_5]` runs `if_close` BEFORE `p_run`, providing the refresh signal)."

The hypothesis attributes refresh to `if_close`. The §2.2 finding doesn't outright refute that — `if_close` is still emitted on the click path — but it adds a second plausible refresh trigger (`%v = %v` self-write) that NAI-137's close-time investigation overlooked. NAI-138 resolves which (or which combination) actually drives client cs1 re-eval.

## 3. Bundle 0 — Controller pre-flight

No commits, no subagent. Two read+grep passes; verdict appended to this spec doc as §6 before Bundle 1 dispatch.

### 3.1 Engine-TS comprehensive re-grep

Goal: refute or refine NAI-137's assertion that TS engine is bare `setVar` everywhere. Search scope: `/home/owner/Code/github.com/LostCityRS/Engine-TS/src/`.

Grep targets:
- `VarPlayerType.RUN` — all consumers
- `setVar(.*RUN` — all writes
- `option_run` — string references
- `processEnergy`, `updateEnergy`, `drainRunEnergy` — energy-transition pathways
- `OpVarpSmall`, `OpVarpLarge`, `IF_RESYNC`, `IF_OPENMAIN`, `IF_OPENMODAL`, `IF_OPENBOTTOM`, `IF_RUNSCRIPT`, `IF_SETSCRIPT`, `IF_RESETANIMS` — refresh-signal emitters
- All script-handlers that mutate run state (search `state.activePlayer.run = `, `this.run = `)
- All call sites of any helper that emits a varp (e.g., `pushVarp`, `writeVarp`, `flushVarp`)

For each emit site, trace the call-chain's emitted packet **sequence** during energy=0 and during click-toggle. Compare sequences.

### 3.2 Click-path semantic re-read

Goal: characterize whether `%option_run = %option_run;` emits an `OpVarpSmall` on the wire or is compiled away.

Sources to consult:
- `LostCityRS/RuneScriptKt` (compiler) — search for varp self-write optimization
- `LostCityRS/Engine-TS` runtime — search for the `PUSH_VARP`/`POP_VARP` opcode handlers (or equivalent) that the `%v = %v` line compiles to; check whether they unconditionally call `setVar` or short-circuit on equal-value
- `LostCityRS/Content/scripts/` — grep for any other `%v = %v` self-write usage to confirm the idiom is established (not unique to player_controls.rs2)

### 3.3 Bundle 0 short-circuit conditions

If §3.1 surfaces a missed TS engine emit at the energy=0 transition (e.g., a parent-interface re-send, a sibling varp, a paired packet), append the verdict and **skip Bundle 1**. Go directly to Bundle 2 with fix-layer = "Engine port (missed TS emit)."

If §3.1 confirms TS is bare `setVar` AND §3.2 conclusively binds `%v = %v` semantics (emits or no-op), Bundle 1 dispatches with sharper substage prompts but still runs all three substages — 1.B is not fully covered by Bundle 0.

## 4. Bundle 1 — Stage 1 parallel audit

Three Explore subagents dispatched in a single turn (`dispatching-parallel-agents`). Each gets a self-contained prompt; substages do not share state.

### 4.1 Substage 1.A — Engine-TS deep audit

**Goal:** binding verdict on whether goscape's `(*Player).updateEnergy` energy=0 emit path is missing any TS-side emitter that the click path implicitly receives.

**Inputs:** `/home/owner/Code/github.com/LostCityRS/Engine-TS/src/`.

**Method:** Bundle 0 §3.1 pre-narrows the search; subagent fans out from there. Trace each call-chain's emitted packet sequence for:
- `[if_button,controls:com_4]` → `if_close` → `p_run` → `%option_run = %option_run`
- `(*Player).updateEnergy` energy=0 transition → `setVar(RUN, 0)`

**Output:** enumerated TS emit sites (file:line), per-pathway packet sequence, and verdict:
- `MISSING_TS_EMIT(site, signal)` — TS emits packet X that goscape doesn't; ports it
- `TS_BARE_SETVAR_CONFIRMED` — TS engine truly emits only the bare `OpVarpSmall(173, 0)` at energy=0; no extra signal
- `INCONCLUSIVE` — controller escalates to Bundle 3 Template α (TS empirical smoke)

### 4.2 Substage 1.B — Client-Java #225 cs1 re-eval audit

**Goal:** binding verdict on what triggers `Component.script1` re-evaluation in OSRS client #225, specifically for `buttontype=select` components.

**Inputs:** `/home/owner/Code/github.com/LostCityRS/Client-Java/src/`.

**Method:**
- Locate the deobfuscated `Component` class (or its `ref/` equivalent). Find `script1` / `scriptComparator1` / `scriptOperand1` field reads.
- Identify all call sites that re-evaluate cs1 bindings (varp-receive packet handler, interface-state events, frame redraw, tab-switch, modal close).
- Determine whether `OpVarpSmall` packet handler invalidates-and-redraws cs1-bound components, or only updates the underlying varp value.
- Determine whether `buttontype=select` routes through a different draw path or scope than default `buttontype` (the OSRS UI grammar treats `select` as a stateful toggle; it may have different invalidation semantics).

**Output:** enumerated re-eval triggers + answer to "does bare varp echo re-eval cs1 for `buttontype=select`?"
- `BARE_VARP_RE_EVALS` — client SHOULD re-eval on `OpVarpSmall(173, 0)`; if smoke says it doesn't, this points at goscape encoder defect
- `BARE_VARP_NO_RE_EVAL(needs_event=X)` — client only re-evals on event X; fix point is "emit X alongside the varp"
- `INCONCLUSIVE` — controller escalates to Bundle 3 Template β (probe instrumentation)

### 4.3 Substage 1.C — Content script audit

**Goal:** characterize the LostCity content language's `[varp,*]` trigger pattern and `%v = %v` self-write semantics.

**Inputs:** `/home/owner/Code/github.com/LostCityRS/Content/scripts/`, with secondary sources in `LostCityRS/RuneScriptKt` (compiler) and `LostCityRS/Engine-TS` (runtime opcode handlers).

**Method:**
- Grep `[varp,` triggers across all Content scripts. Enumerate all uses of the trigger pattern.
- Grep `%X = %X;` self-write patterns. Confirm or refute that `%option_run = %option_run` is an established idiom.
- Cross-reference compiler / runtime to determine whether the self-write emits `OpVarpSmall` to the client or is compiled away.
- If `[varp,*]` triggers exist as an established pattern, document a representative example for use in the Content fix layer.

**Output:** trigger-pattern catalog + self-write semantics verdict.
- `SELF_WRITE_EMITS_OP_VARP` — `%v = %v` does emit on the wire; supports content-fix or engine-double-emit fix layers
- `SELF_WRITE_NOOP` — compiled away; the `// resync varp` comment is misleading and the click-path refresh comes from elsewhere (probably `if_close`)
- `INCONCLUSIVE` — controller defaults to engine-double-emit fix (lower content-PR blast radius)

## 5. Synthesis matrix

Controller combines the three substage verdicts into a fix-layer decision:

| 1.A | 1.B | 1.C | → Fix layer |
|---|---|---|---|
| `MISSING_TS_EMIT` | * | * | **Engine port (TS-faithful)** — highest fidelity, lowest blast radius |
| `TS_BARE` | `BARE_VARP_NO_RE_EVAL` | `SELF_WRITE_EMITS` | **Content `[varp,option_run]` trigger** OR engine double-emit (layer choice deferred to R4 — prefer content if TS-smoke available to verify non-breakage; prefer engine double-emit otherwise) |
| `TS_BARE` | `BARE_VARP_NO_RE_EVAL` | `SELF_WRITE_NOOP` | **Engine ad-hoc refresh** (drift; deviation tag required) |
| `TS_BARE` | `BARE_VARP_RE_EVALS` | * | **Goscape encoder defect** — `OpVarpSmall(173, 0)` wire bytes diverge from TS |
| any `INCONCLUSIVE` | | | Bundle 3 Template α (TS empirical smoke) or Template β (probe) |

**Pre-Bundle-2 verification step (per `audit_subagent_fabrication`):** Controller verifies each substage's load-bearing claims by independent grep+Read before dispatching Bundle 2. Specifically: any cited TS file:line in 1.A, any cited `Component.script1` trigger site in 1.B, any cited Content trigger in 1.C — re-grep at synthesis time. Any unverifiable claim → escalate substage to `INCONCLUSIVE`.

## 6. Bundle 0 verdict

### 6.1 Engine-TS re-grep verdict

**TS emit sites — Pathway A (energy=0 transition):**
- `Engine-TS/src/engine/entity/Player.ts:682-704` — `updateEnergy()`. At L696-700:
  ```ts
  if (this.runenergy === 0) {
      this.run = 0;
      // todo: better way to sync engine varp
      this.setVar(VarPlayerType.RUN, this.run);
  }
  ```
  - L698 comment is load-bearing: TS author flagged the same concern this sub-spec investigates ("better way to sync engine varp" — they knew the bare setVar wasn't quite right but didn't fix it).
- `setVar` (Player.ts:1715-1730) → `writeVarp` (Player.ts:1732-1738) → emits **one** `VarpSmall(173, 0)` packet (since 0 ∈ [-128, 127]). No deduplication; no extra refresh signal.

**TS emit sites — Pathway B (click-toggle):**
- `Content/scripts/interface_controls/scripts/player_controls.rs2:25-50` runs three operations:
  1. `if_close;` — opcode handler at `PlayerOps.ts:245`; emits `IfClose()` packet (ServerGameProt.IF_CLOSE = 129, 0-byte payload per `IfCloseEncoder.ts:9` `encode(_, __): void {}`).
  2. `p_run(...)` — handler at `PlayerOps.ts:1208`; calls `setVar(VarPlayerType.RUN, ...)` → `writeVarp` → emits `VarpSmall(173, 0/1)`.
  3. `%option_run = %option_run;` — lowers to `PUSH_VARP option_run` + `POP_VARP option_run` (per RuneScriptKt `BinaryScriptWriter.kt:131,146`); POP_VARP handler at `CoreOps.ts:41-58` calls `setVar` unconditionally → emits **another** `VarpSmall(173, 0/1)`.

**Per-pathway packet sequence comparison:**

| Step | Pathway A (energy=0) | Pathway B (click-toggle) |
|---|---|---|
| 1 | `VarpSmall(173, 0)` | `IfClose()` |
| 2 | (none) | `VarpSmall(173, 0/1)` (from p_run) |
| 3 | (none) | `VarpSmall(173, 0/1)` (from %option_run = %option_run) |

Pathway B emits **3 packets**; Pathway A emits **1**. The asymmetry is in CONTENT (which provides the `if_close` and self-write packets for free), not in TS engine behavior.

**Verdict:** **`TS_BARE_SETVAR_CONFIRMED`**. The TS engine truly emits exactly one bare `VarpSmall(173, 0)` at energy=0; no missed emit. NAI-137 close commit's claim was correct on this point. **§7.1 (Engine port — missed TS emit) is INVALID as a fix layer.**

### 6.2 `%v = %v` semantics verdict

**Compiler evidence:** `RuneScriptKt/clientscript-compiler/src/main/kotlin/me/filby/neptune/clientscript/compiler/writer/BinaryScriptWriter.kt:131,146` lowers `VarPlayerType` reads to `PUSH_VARP` and writes to `POP_VARP`. So `%option_run = %option_run;` lowers to:
```
PUSH_VARP option_run    (reads vars[173], pushes to stack)
POP_VARP option_run     (pops, calls setVar(173, value))
```

**Runtime handler:** `Engine-TS/src/engine/script/handlers/CoreOps.ts:41-58` — `POP_VARP` handler calls `player.setVar(varpType.id, state.popInt())` unconditionally. No equal-value short-circuit. `setVar` (`Player.ts:1715-1730`) writes `vars[id] = value` AND calls `writeVarp` if `varp.transmit` is set. `writeVarp` (`Player.ts:1732-1738`) unconditionally emits `VarpSmall` or `VarpLarge` packet.

**Conclusion:** `%X = %X` **DOES** emit `OpVarpSmall` (or Large) on the wire, even when the value is unchanged.

**Other Content uses of the idiom:**
- Total hits: 50+ (not exhaustively counted; idiom is well-established).
- Sample sites:
  - `Content/scripts/interface_controls/scripts/player_controls.rs2:12,21,32,49` — `option_nodef`, `option_run` resync writes
  - `Content/scripts/tutorial/scripts/tut_chatbox_steps.rs2:118` — `option_run` resync
  - `Content/scripts/skill_prayer/scripts/prayers/*.rs2` — `prayer0..prayer10` resync (one per prayer script)
  - `Content/scripts/minigames/game_trawler/scripts/trawler_flood.rs2:8-10,30-32` — npc-side vars
  - `Content/scripts/minigames/game_duelarena/scripts/duel_arena_settings.rs2:87` — `dueloptions` resync
- The `// resync varp` comment is consistent across player_controls usages, indicating the idiom is INTENTIONAL and treated as a refresh trigger by content authors.

**Verdict:** **`SELF_WRITE_EMITS_OP_VARP`**.

**Additional finding — `[varp,X]` trigger non-existence (refutes spec §7.2):**

`grep -E "^\[varp," /home/owner/Code/github.com/LostCityRS/Content/scripts/` returned **zero hits**. `Engine-TS/src/engine/script/ServerTriggerType.ts` has no `VARP` or `VAR_` enum entries (only PROC, LABEL, DEBUGPROC, AP*/OP* for npc/obj/loc, and trigger types for explicit interface events — no varp-change triggers). **The LostCity engine does not dispatch script triggers on varp changes.** §7.2 (Content `[varp,option_run]` trigger) is **INVALID** as a fix layer.

This narrows the fix-layer matrix significantly:
- §7.1 invalid (no missed TS emit)
- §7.2 invalid (no `[varp,X]` trigger pattern in engine)
- §7.3 (engine ad-hoc refresh — which is what TS L698's "todo" comment hints at) — primary candidate
- §7.4 (encoder defect) — secondary candidate, contingent on 1.B

**Bundle 1 dispatch decision:** Bundle 0 has resolved 1.A and 1.C definitively. Only 1.B (Java client cs1 re-eval audit) remains unbound. Bundle 1 dispatches **only the 1.B substage** (single Explore subagent), not the planned three. 1.B determines whether the fix is §7.3 (client needs additional event → engine ad-hoc refresh) or §7.4 (client should re-eval but doesn't due to goscape encoder defect).

### 6.3 Stage 1 synthesis verdict

**Substage verdicts (verified at T5 controller pre-flight):**

- **1.A: `TS_BARE_SETVAR_CONFIRMED`** — citations `Engine-TS Player.ts:682-704`, `Player.ts:1715-1738`. Independently verified.
- **1.B: `BARE_VARP_RE_EVALS`** — citations `Client-Java deob/client.java:9363-9377` (VarpSmall handler sets `redrawSidebar=true` when value changes), `:4696-4698` (redrawSidebar drives `drawSidebar()`), `:10387-10395` (`drawSidebar()` calls `drawInterface()`), `:7962-7986` (`executeInterfaceScript` evaluates cs1 comparators), `:8678-8680` (opcode 5 reads `varps[id]`). Independently verified — minor line discrepancy in subagent report (cited 8685 instead of 8678 for opcode 5; substance correct).
- **1.C: `SELF_WRITE_EMITS_OP_VARP`** — citations `RuneScriptKt BinaryScriptWriter.kt:131,146`, `Engine-TS CoreOps.ts:41-58`, `Player.ts:1715-1738`. Independently verified. Bonus finding: no `[varp,X]` triggers in Content; `ServerTriggerType.ts` has no VARP entries.

**Pre-Bundle-2 verification (per `audit_subagent_fabrication`):** all 1.B citations independently verified at T5; 1.A and 1.C citations established directly by controller in Bundle 0.

**Synthesis matrix row matched:** Row 4 — `TS_BARE | BARE_VARP_RE_EVALS | * → §7.4 Goscape encoder defect`.

**Goscape-side pre-flight (controller, 5-min sanity check):**
- `pkg/io/protocol/game/server/prot.go:66` — `OpVarpSmall = Op{Opcode: 150, PayloadSize: 3}` matches TS `ServerGameProt.VARP_SMALL = (150, 3)` exactly.
- `modules/world/player_varp.go:12-26` — `writeVarp` structurally mirrors TS Player.ts:1732-1738: P2(id) + P1(int8 value) for small range, P4(value) for large range.
- Both opcodes and packet shapes look structurally correct. **An encoder-byte defect is not the most likely Stage 2 finding.**

**Refined Stage 2 framing:** matrix row 4 nominally routes to §7.4 (encoder defect), but the goscape-side pre-flight suggests the encoder bytes are correct. The actual goscape-side defect is more likely:
- **Hypothesis A:** Sequencing — `varps[173]` client-side never transitions to 1 in the smoke scenario, so `OpVarpSmall(173, 0)` at energy=0 hits the `if (varps[var26] != var52)` short-circuit at `client.java:9367` and produces no redraw.
- **Hypothesis B:** Tick timing — packet emitted but flushed too late or to wrong client connection.
- **Hypothesis C:** Encoder-byte divergence — actual on-wire bytes diverge from TS despite same opcode / payload-size constants (e.g., `P1(uint8(int8(value)))` cast at `player_varp.go:20` may produce wrong byte for negative values, though run varp values are 0/1 only).

**Chosen fix layer:** **§7.4 Goscape encoder defect** (matrix-driven), with Stage 2 plan author instructed to start with Bundle 3 Template β (probe instrumentation) — capture goscape's actual on-wire bytes at energy=0 vs click-toggle paths, plus client-side `varps[173]` state at the moment of energy=0 emit. The probe binds whether Hypothesis A, B, or C is the actual defect; the fix (which may NOT be encoder-byte work) is then authored against the bound hypothesis.

**Stage 2 plan handoff:** `docs/superpowers/handoffs/2026-05-09-nai-138-stage-1-binding.md`

## 7. Bundle 2 — Fix at indicated layer

Single goscape commit (or Content PR for the content-fix layer). TDD red→green→commit per `test-driven-development`.

### 7.1 Layer: Engine port (missed TS emit)

- TDD red: extend `TestUpdateEnergy_EnergyZeroResetsRunAndVarp` (or appropriate handler test) to assert the additional emit on the wire stream.
- TDD green: port the missed TS emitter to `(*Player).updateEnergy` (or sibling site).
- Commit body cites TS source line range. **No deviation tag** — this is a fidelity catch-up.

### 7.2 Layer: Content `[varp,option_run]` trigger

- Out-of-tree relative to goscape: PR against `LostCityRS/Content`, modifying `Content/scripts/interface_controls/scripts/player_controls.rs2` (or sibling) to add a `[varp,option_run]` trigger that emits the appropriate refresh signal. Body shape determined by 1.C trigger-pattern catalog.
- Goscape side: zero code changes. Tracking-only commit (or none) with the upstream Content PR link in this spec doc.
- Smoke binds correctness on goscape AND on TS (Content is consumed by both).

### 7.3 Layer: Engine double-emit / ad-hoc refresh (drift)

- Explicit deviation tag: `NAI-138-DEV-RUN-VARP-<SIGNAL>` (final tag suffix names the actual signal emitted — e.g. `DOUBLE-EMIT`, `IF_RESYNC`, `IF_RESETANIMS` — chosen per Stage 1 verdict). Per `defensive_gate_doc_comment_label` style: `// (goscape deviation; TS does not emit this — see NAI-138 spec §7.3)`.
- TDD red: assert the second emit on the wire stream during energy=0 transition.
- TDD green: emit the additional packet from `(*Player).updateEnergy`.
- Memory entry under `nai_followups.md` documents the drift; entry includes retire-on-OSRS-divergence-fix condition.

### 7.4 Layer: Goscape encoder defect

- Investigate `pkg/io/protocol/server/OpVarpSmall` encoder for bit-order / field-order / value-range divergence vs TS / vs Java client expectation.
- Per `rsbuf_roundtrip_tests` and `dispatch_order_audit_blind_spot`: add a roundtrip test against the reference encoder. Decode in Java client reader order.
- Fix shape: encoder change + roundtrip pin.

## 8. Smoke + close decision tree

Goscape smoke (user-run per `smoke_test_server_handoff`): drain energy to 0 (e.g., long-distance walk on full energy until depletion), observe button.

| Outcome | Close action |
|---|---|
| Button visually de-toggles at energy=0 | PRIMARY met — close NAI-138 |
| Button stays stuck-on; click toggles still work | Bundle 3 Template α (empirical TS smoke) or Template β (probe instrumentation), don't close |
| New regression (click toggles broken, NPC info echo broken, etc.) | Revert fix commit; open NAI-138 stretch with regression-pin test |

Per `cascade_theory_smoke_binding`: smoke is binding for cascade attribution. Don't close on argument alone.

## 9. Bundle 3 — Conditional templates (pre-baked)

### 9.1 Template α — TS empirical smoke escalation

Triggered when any Stage 1 substage returns `INCONCLUSIVE` AND the substage's verdict is load-bearing for fix-layer choice.

- Out-of-band: user runs Engine-TS server with same Java client #225, drains energy, reports button behavior.
- TS-mirror behavior (button also stays stuck-on) → cascade attribution to client/content layer; re-route to 1.B/1.C re-audit with sharper prompts
- TS-divergent behavior (TS button DOES de-toggle) → goscape engine port has hidden defect 1.A missed; re-dispatch 1.A with 1.A's own gaps as input

### 9.2 Template β — Probe instrumentation

Triggered when goscape smoke fails after Bundle 2 fix.

- Add `s.log.Info` gateways in `(*Player).updateEnergy` and in `handlePRun` per `nodedebug_gateway_probe_pattern` — log the exact packet stream emitted during energy=0 and during click-toggle transitions.
- User runs paired smokes (energy=0 path + click-toggle path), captures logs.
- Byte-level diff binds the actual refresh trigger.
- Bundle 3.β fix per the diff. Revert the failed Bundle 2 fix in the same commit.

## 10. Risks

- **R1 Audit-subagent fabrication** (`audit_subagent_fabrication`). Per §5 pre-Bundle-2 verification step, controller verifies each substage's load-bearing claims by independent grep+Read before dispatching Bundle 2.
- **R2 Cascade theory wrong** (`cascade_theory_smoke_binding`). The diagnosis "OSRS client #225 doesn't re-eval cs1 on bare varp echoes" is a hypothesis. Goscape smoke is binding. If 1.B says "should re-eval" but smoke says "didn't," that's a goscape defect 1.B missed — escalate to encoder-defect fix layer.
- **R3 Fidelity drift** (`true_to_ts_gate`). Layer §7.3 (engine ad-hoc refresh) is the only fix that actively diverges from TS. Default away from it; require explicit deviation tag + memory entry if chosen.
- **R4 Content-PR blast radius.** Content is a sibling repo; consumed by goscape AND TS engine. Content fix must be verified non-breaking against TS. If TS server isn't easily smoked, prefer engine fix (§7.1, §7.3, §7.4) over content fix (§7.2).
- **R5 LOC overrun.** Investigation sub-specs are unbounded by nature. Per `compressed_cadence`: if Bundle 0 binds + fix is ≤~15 LOC + only one layer touched, collapse to single spec+plan doc (this one) and skip formal review.
- **R6 Plan-author audit of sibling-site guards** (`plan_sibling_site_guard_audit`). If §7.1 ports a TS emit, plan author greps ALL sibling emit sites in goscape and reproduces guard patterns (e.g., `s.scriptProvider != nil`, varp-id zero check).

## 11. Memory routing at close

Regardless of fix layer, memory entries to write at close:

- **Verdict-binding memory** capturing what 1.B established about OSRS client #225's `Component.script1` re-eval cadence — load-bearing for future varp-binding sub-specs (cs1 graphic-binding, cs2 click-binding, etc.). Suggested filename: `cs1_re_eval_triggers.md`.
- **`%v = %v` self-write semantics** if 1.C established it — pairs with `chatnpc_pipe_line_break.md` as a content-language idiom entry. Suggested filename: `runescript_self_write_semantics.md`.
- **NAI-137 carryover update** in `nai_followups.md` line 6432: replace the open NAI-138+ candidate with NAI-138 outcome (close, deferred, etc.), include final fix-layer + post-fix smoke evidence.
- **If §7.3 chosen (engine drift):** add deviation entry to deviation tracker AND memory entry under `nai_followups.md` with retire-on-OSRS-divergence-fix condition.

## 12. Non-goals

- **No fix to other clientcode-N varps.** NAI-137 covered clientcode-7. Other clientcode values (1, 2, 3, 4, 5, 6, 8) have separate engine semantics; out of scope.
- **No `Component.script1` re-eval audit beyond what 1.B requires for the run-toggle case.** If 1.B surfaces broader cs1 bugs, route to NAI-N+1.
- **No general OSRS-vs-LostCity content-divergence catalog.** Even if §7.2 lands a content fix, the broader question of "which OSRS client behaviors are absent in #225 build" is out of scope.
- **No retirement of any existing deviation tags** (NAI-115-D1/D2, NAI-108-D, etc.).
- **No login-time push of the run varp.** NAI-137 §4 already declared this out of scope; NAI-138 inherits.
- **No goscape-side smoke-harness work.** Per `smoke_test_server_handoff`, smoke is user-driven.
- **No "no-fix close" branch.** User constraint: must ship a fix at SOME layer. If audit verdict is "OSRS client #225 has a known bug and there's no clean fix point," the fix layer defaults to §7.3 (engine ad-hoc refresh with deviation tag).
