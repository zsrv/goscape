# NAI-138 Stage 1 → Stage 2 handoff

**Stage 1 close commit:** `13806b8`
**Spec doc:** `docs/superpowers/specs/2026-05-09-nai-138-cs1-reeval-investigation-design.md`
**Stage 1 plan:** `docs/superpowers/plans/2026-05-09-nai-138-stage-1-cs1-reeval-investigation.md`
**Date:** 2026-05-09
**Bundle 0 commit:** `ca8d937`
**Stage 1 synthesis commit:** `13806b8`

## Synthesis verdict

- **1.A: `TS_BARE_SETVAR_CONFIRMED`** — TS engine emits exactly one bare `VarpSmall(173, 0)` at energy=0 reset (`Engine-TS Player.ts:682-704`); the L698 `// todo: better way to sync engine varp` comment confirms TS author flagged the same concern this sub-spec addresses. No missed engine emit. **§7.1 invalid.**
- **1.B: `BARE_VARP_RE_EVALS`** — OSRS Client-Java #225 `redrawSidebar=true` flag is set in the `VarpSmall` packet handler when the underlying varp transitions to a new value (`deob/client.java:9367-9374`); next frame triggers `drawSidebar()` → `drawInterface()` → `executeInterfaceScript()` → cs1 binding re-eval via `executeClientscript1` opcode 5 (`varps[id]` read). Bare varp echo IS sufficient to update `buttontype=select` components.
- **1.C: `SELF_WRITE_EMITS_OP_VARP`** — `%X = %X` lowers to `PUSH_VARP + POP_VARP`; POP_VARP handler unconditionally calls `setVar` → `writeVarp` → emits `OpVarpSmall`. Plus: **`[varp,X]` trigger pattern does NOT exist** in the engine (no entry in `Engine-TS ServerTriggerType.ts`; zero matches in `Content/scripts/`). **§7.2 invalid.**

**Matched matrix row:** Row 4 — `TS_BARE | BARE_VARP_RE_EVALS | * → §7.4 Goscape encoder defect`.

**Chosen fix layer:** **§7.4** (Goscape encoder defect, matrix-driven) — but with the strong caveat (per spec §6.3) that a goscape-side pre-flight showed `OpVarpSmall` opcode/payload constants and the `writeVarp` structure both match TS exactly. An encoder-byte defect is **NOT** the most likely Stage 2 finding.

## Stage 2 entry point — probe-first cadence

**Stage 2 should NOT author an encoder fix immediately.** The matrix-driven §7.4 framing assumes encoder bytes diverge; the goscape pre-flight refutes that. Stage 2 starts with **Bundle 3 Template β (probe instrumentation)** to bind which of three hypotheses is the actual defect:

- **Hypothesis A — Sequencing:** Client-side `varps[173]` never transitions to 1 in the failing smoke scenario, so `OpVarpSmall(173, 0)` at energy=0 hits the `client.java:9367` `if (varps[var26] != var52)` short-circuit and produces no redraw. Test: log goscape's varp[173] state immediately before the energy=0 emit.
- **Hypothesis B — Tick timing:** Packet emitted but flushed too late, after a tick boundary, or written to wrong client connection. Test: log emit timestamp + connection id at the energy=0 emit site.
- **Hypothesis C — Encoder-byte divergence:** Actual on-wire bytes diverge from TS despite matching opcode/payload constants. Test: capture raw bytes at goscape's connection write boundary, compare against TS bytes for the same packet.

The probe instrumentation should hook `(*Player).updateEnergy` and `handlePRun` per `nodedebug_gateway_probe_pattern`. Capture both pathways' wire-byte stream + pre-emit varp state + connection id. Paired smoke runs (energy=0 path + click-toggle path) then byte-level diff binds the actual defect.

## Citations needed for Stage 2 plan

For Bundle 3 Template β probe (always — Stage 2's first step):
- Probe attach point: `modules/world/player_energy.go` or wherever `(*Player).updateEnergy` lives (verify at Stage 2 plan-write).
- Probe gate pattern reference: existing `s.log.Info` gateways elsewhere in goscape; per `nodedebug_gateway_probe_pattern` memory, gate behind a NodeDebug or similar runtime flag.
- Reference for capturing on-wire bytes: `modules/world/player_varp.go:21` `p.writeOut(gameserver.OpVarpSmall, buf.Bytes())` — log `buf.Bytes()` AND `p.varps[173]` (or its goscape equivalent) at this site.
- Reference for click-toggle baseline: P_RUN handler in `pkg/script/handlers_player.go` (or wherever `handlePRun` lives — verify path at Stage 2 plan-write).

Per-hypothesis fix citations (filled by Stage 2 author after probe binds the defect):

For Hypothesis A (Sequencing — varp never transitioned to 1):
- Audit goscape's run-state lifecycle: who writes varp[173]=1 on click, when, and to which player struct.
- Likely root cause: `(*Player).RunVarpID()` shim returns 0 instead of resolved id at toggle-time, or P_RUN handler emits but server-side varp dict isn't updated.
- Fix shape: ensure varp[173]=1 actually lands client-side AND server-side after click.

For Hypothesis B (Tick timing):
- Audit `(*Player).updateEnergy` call site in tick loop: is it called inside the tick-end flush, or before? Compare against the click-handler emit timing.
- Likely fix shape: move emit to tick-end flush, or add explicit flush call.

For Hypothesis C (Encoder-byte divergence):
- Encoder under suspicion: `pkg/io/packet.Packet.P1`/`P2` implementations vs TS `Packet.p1`/`p2`.
- Likely candidates: `P1(uint8(int8(value)))` cast at `player_varp.go:20` for negative values (though run varp is 0/1 only, so this shouldn't matter for the smoke case).
- Fix shape: encoder change + roundtrip pin per `rsbuf_roundtrip_tests`.

## Stage 2 plan author resume prompt

```
/clear

Author the NAI-138 Stage 2 implementation plan at
docs/superpowers/plans/2026-05-09-nai-138-stage-2-encoder-defect.md.

Predecessor: NAI-138 Stage 1 closed at 13806b8 (Stage 1 synthesis
commit). Verdict in
docs/superpowers/specs/2026-05-09-nai-138-cs1-reeval-investigation-design.md
§6.3. Handoff doc:
docs/superpowers/handoffs/2026-05-09-nai-138-stage-1-binding.md.

Fix layer: §7.4 (matrix-driven) with probe-first cadence per the
handoff. Stage 2 starts with Bundle 3 Template β probe instrumentation
to bind which of three hypotheses (A sequencing / B tick timing / C
encoder-byte) is the actual defect; THEN authors the fix against the
bound hypothesis.

Stage 2 plan structure:
- Bundle β.1: probe instrumentation (NodeDebug-gated s.log.Info
  gateways at (*Player).updateEnergy emit + handlePRun emit). User
  smoke handoff. Paired runs (energy=0 + click-toggle).
- Bundle β.2: synthesize probe output → bind hypothesis A/B/C.
- Bundle β.3: fix at bound hypothesis. TDD red→green→commit.
- Smoke handoff #2: verify visual button de-toggle at energy=0.

Cadence: subagent-driven-development per execution_mode_default.

Use the citations in the handoff doc for probe attach points; verify
each cite against HEAD at plan-write per
spec_test_runtime_behavior_verify (paths may have shifted).
```

## Smoke handoff at Stage 2 close

Per spec §8, goscape smoke binds Stage 2 close. Decision tree:
- Button visually de-toggles at energy=0 → close NAI-138 PRIMARY met
- Button stays stuck-on, click toggles still work → re-open Bundle 3 (probe again with sharper scope)
- New regression → revert + open NAI-138 stretch

## Compressed-cadence eligibility

Per spec §10 R5 + `compressed_cadence`: §7.4 fix shape is variable.
- Hypothesis A fix (sequencing): typically ≤15 LOC, single touch point. Compressed-eligible.
- Hypothesis B fix (tick timing): typically ≤15 LOC. Compressed-eligible.
- Hypothesis C fix (encoder-byte): NOT compressed-eligible — encoder changes carry roundtrip-test obligation.

Stage 2 plan author estimates compressed-eligibility AFTER probe binds the hypothesis.

## Memory routing at Stage 2 close

Regardless of fix shape, memory entries to write:
- `cs1_re_eval_triggers.md` — captures 1.B's findings about OSRS client #225 `Component.script1` re-eval cadence (varp packet sets `redrawSidebar=true` on value change → next-frame `drawSidebar()` → `executeInterfaceScript()` → opcode 5 reads `varps[id]`). Load-bearing for future varp-binding sub-specs.
- `runescript_self_write_semantics.md` — `%X = %X` is a real wire emit, not a no-op. Pairs with `chatnpc_pipe_line_break.md`.
- `lostcity_no_varp_trigger.md` — content language has no `[varp,X]` trigger pattern; engine doesn't dispatch on varp changes. Important for future content-side fix proposals.
- NAI-137 carryover update in `nai_followups.md` line 6432: replace open NAI-138+ candidate with NAI-138 closed entry, include Stage 2 fix layer + post-fix smoke evidence.
