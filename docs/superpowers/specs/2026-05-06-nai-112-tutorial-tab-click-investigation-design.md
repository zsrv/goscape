# NAI-112 — Tutorial-tab-click chatbox-advance investigation

**Status:** spec — investigation sub-spec (Stage 1 audit → Stage 2 fix → smoke; per `investigation_subspec_cadence`).
**Cadence:** Bundle 0 controller pre-flight (no commits) → Bundle 1 Stage 1 risk-weighted-short-circuit audit (single Sonnet subagent + controller HEAD-verification) → Bundle 2+ Stage 2 fix (TDD; shape determined by Stage-1 attribution) → user-launched smoke handoff.
**Tech stack:** Go 1.26+.
**Upstream sources:** `LostCityRS/Engine-TS` (per `ts_source_canonical_path`); `LostCityRS/Client-Java` rev-225 (Java client wire reference); `LostCityRS/Server` (content-side `tutorial.rs2`).

---

## 1. Context & motivation

NAI-110 closed at `e3eecbe` with a user-launched smoke (2026-05-06):

- ✅ TEXT_GENDER warn silenced; `[proc,tutorial_please_wait_woodcutting]` no longer aborts at pc=4.
- 🆕 New blocker on tutorial path: clicking the inventory tab (post `tut_flash(^tab_inventory)`) does **not** advance the chatbox AND does **not** display the inventory side panel. No warn log was reported.

Both symptoms point at the TUT_CLICKSIDE → `[tutorial,_]` script-trigger pipeline failing to fire (or firing without effect). Goscape has the wire handler at `modules/world/handler_interface.go:138-149` and the `TriggerTutorial=159` enum at `pkg/script/trigger.go:164`. Unit tests at `modules/world/handler_interface_test.go:71-128` pass against an in-process `Provider.Register(...)` fixture, so the wiring is correct in isolation but not necessarily against the loaded cache.

**Pre-flight observations (Bundle 0 controller probe at HEAD `e3eecbe`):**

- `modules/world/handler_interface.go:138-149` — `handleTutClickSide` reads tab byte; gates `0 ≤ tab ≤ 13`; calls `s.scriptProvider.GetByTriggerSpecific(script.TriggerTutorial, -1, -1)`; calls `s.runScript(sf, p, nil, true, nil, nil)` — passes `nil, nil` for intArgs/stringArgs.
- `pkg/script/provider.go:145-153` — `GetByTriggerSpecific(t, -1, -1)` returns `byKey[uint32(trigger)]` directly (global lookup; no fallback).
- `pkg/script/lookup_key.go:18-20` — `LookupKeyForGlobal(t)` is `uint32(t)` — no shift, no selector bits.
- `pkg/script/trigger.go:164` — `TriggerTutorial = 159`.
- `pkg/script/provider.go:100-102` — `Provider.Load` writes `byKey[f.LookupKey] = f` for any `LookupKey != 0xFFFFFFFF`. Whether `[tutorial,_]` ends up at `byKey[159]` depends on what `LookupKey` the upstream pack-server / RuneScript compiler emits for the wildcard subject.

---

## 2. Scope

### In scope

- **Stage 1** — derive the TS reference chain end-to-end (handler / lookup / wire / script body), then bind a hypothesis using static-first probes; instrument-and-smoke fallback only if static is inconclusive.
- **Stage 2** — land the fix in NAI-112 regardless of LOC. Shape determined by Stage-1 attribution (one of H1/H2/H3/H4+).
- **Stage 3** (conditional) — re-investigate per `cascade_theory_smoke_binding` if Stage-2 smoke fails.
- TDD per goscape convention for any production change.
- `nai_followups.md` close entry with cascade attribution.

### Out of scope

- **NAI-111** — P_TELEJUMP `[label,tutorial_complete]` "script not protected"; remains queued; pre-NAI-110 baseline.
- **Adjacent divergences** surfaced by Stage-2 smoke route per `smoke_surfaces_adjacent_divergences` (≤30 LOC stretch in-scope; else NAI-113).
- **Client-Java patch.** If H2 binds (Java client doesn't send opcode 175), NAI-112 closes as investigation-only with deviation note; Client-Java fix routes elsewhere.

---

## 3. Hypothesis register (risk-weighted-short-circuit ordered)

| # | Hypothesis | Probe cost | Probe shape | If confirmed (Stage 2 fix shape) |
|---|---|---|---|---|
| **H1** | `[tutorial,_]` not registered under `byKey[159]` at script-load — pack-server / RuneScript compiler maps the `_` wildcard subject to a non-global `LookupKey`, OR `Provider.Load` skips/mis-derives the entry for that head shape | Low — read RuneScript head-parse path + decode a real `script.dat` entry | **Stage 1.1:** locate goscape's pack-server / RuneScript compile path that produces `LookupKey` for a script header; trace `[tutorial,_]` through it. Cross-check by enumerating `byKey` post-`Provider.Load()` (instrument fallback if needed) | Fix in goscape pack-server / `pkg/script/decode.go` `LookupKey` derivation OR `Provider.Load` to register the wildcard subject at `byKey[159]`. Unit test: load a fixture `script.dat` containing `[tutorial,_]`; assert `byKey[uint32(TriggerTutorial)]` is non-nil |
| **H2** | Java client at rev-225 does not send opcode 175 on Tutorial-Island inventory-tab click — gated by tutorial mode, sidebar dispatcher mismatch, or the click is consumed by an earlier handler | Low — read Client-Java sidebar-click handler + tutorial-mode gates | **Stage 1.2:** locate the click-side dispatcher in `Client-Java/src/main/java` rev-225; confirm or refute that opcode 175 is sent; note any `overrideChat` / mode gates per `java_client_coord_chat_suppression` | NAI-112 closes as investigation-only with deviation note; Client-Java patch routes to a separate sub-spec (out-of-scope here) |
| **H3** | Script runs but a downstream opcode aborts silently — `[tutorial,_]` body uses an opcode goscape lacks a handler for, or one that returns nil-error without effect | Med — read `tutorial.rs2:143-176` and walk every opcode against goscape's handler coverage | **Stage 1.3:** enumerate opcodes used in `[tutorial,_]`; cross-check `pkg/script/handler_*.go` registration. User reported no warn log — but per `audit_full_method_against_ts`, do not over-weight that absence (logs may have scrolled). Static walk is authoritative | Port the missing handler(s) à la NAI-110 / NAI-109. TDD: red test invoking the opcode with mock state → green handler. Could be ≥1 LOC (single varp write) to ~80 LOC (multi-opcode chain) |
| **H4** | `runScript` is invoked but with the wrong arg shape — TS `TutClickSideHandler.ts` passes the tab index as a script argument; goscape passes `nil, nil`. If `[tutorial,_]` reads `$arg0` via `getarg`, it sees zero on every click | Low — read TS handler line + `[tutorial,_]` body for `getarg` use | **Stage 1.4 (folded into 1.1's TS-handler read):** check whether TS `TutClickSideHandler` constructs `ScriptState` with the tab as an argument. If yes AND `[tutorial,_]` reads it, H4 binds | Fix at `modules/world/handler_interface.go:147` — thread tab byte through as `intArgs=[]int{tab}` (or whatever shape TS uses). ≤10 LOC. Extend `handler_interface_test.go:71-128` to assert intArgs propagation |
| **H5** | `GetByTriggerSpecific(..., -1, -1)` is too narrow — TS uses the 3-tier `getByTrigger` fallback (or fires both specific `[tutorial,<tab_name>]` AND global `[tutorial,_]`), and goscape's global-only call misses a content-required specific script | Low — read TS handler dispatch | **Stage 1.5 (folded into 1.1):** check TS handler's lookup call shape. If TS uses `getByTrigger` (3-tier) or a different selector, H5 binds | Switch to `GetByTrigger` (3-tier fallback) at the call site, OR replicate TS's tiered dispatch (specific-first, fallback-to-global). Unit test asserting both specific and global tiers fire correctly |

**Order rationale:**

- H1 first: matches the user's prior ranking; cheapest static probe (Provider.Load + LookupKey arithmetic) and highest-risk failure mode (cache-load registration is hard to unit-test and the existing tests use `Provider.Register(...)` which bypasses Load entirely).
- H4 / H5 promoted from "not catalogued" to medium-priority because the audit is full-method (Q3=B). Each is a single TS-line read; near-zero probe cost.
- H2 third: probe is cheap but binding shifts blame outside goscape.
- H3 last: highest probe cost (whole-body opcode walk) and lowest prior probability (no warn log reported), but cannot be ruled out statically without reading the body.

---

## 4. Stage 1 — audit dispatch

**Subagent:** one Sonnet audit subagent dispatched after Bundle 0 pre-flight, read-only. Has Read access to all three external repos (`/home/owner/Code/github.com/LostCityRS/{Engine-TS,Client-Java,Server}`) plus the goscape repo.

**Task:** derive the TS reference chain in this order:

1. `Engine-TS/src/network/game/client/handler/TutClickSideHandler.ts` — full body. Note: lookup call (specific vs tiered), `runScript` arg shape (does it pass tab?), any pre-dispatch gates.
2. `Engine-TS/src/lostcity/engine/script/ScriptProvider.ts` (or equivalent) — how `LookupKey` is derived for a `[tutorial,_]` head; how the wildcard subject `_` encodes.
3. `LostCityRS/Server/content/scripts/tutorial/scripts/tutorial.rs2:143-176` — `[tutorial,_]` body. Enumerate every opcode. Note any `getarg` reads.
4. `LostCityRS/Client-Java` rev-225 — locate the sidebar-tab click dispatcher. Confirm or refute opcode 175 transmission on Tutorial-Island context.
5. **Goscape side:** locate the pack-server / RuneScript compile path that produces `LookupKey` for a script header. Trace `[tutorial,_]` through it. Walk every opcode in (3) against `pkg/script/handler_*.go` registration tables.

**Deliverables (subagent writes audit report to `docs/superpowers/investigations/2026-05-06-nai-112-stage1-audit.md`):**

- TS reference summary with file:line for every derivation point.
- Per-hypothesis verdict: confirmed / refuted / inconclusive, with evidence.
- If confirmed: sized fix shape (estimated LOC + which goscape files).
- "Verified at HEAD" claims explicitly listed for controller spot-check.

**Controller HEAD-verification (post-subagent, mandatory):**

- Re-grep every goscape symbol the audit cites.
- Re-read every cited TS / Client-Java / rs2 line range.
- Re-derive any `LookupKey` arithmetic claimed.
- Per `audit_subagent_fabrication`: any unverifiable claim → fall back to instrument-and-smoke probe; do not bind a hypothesis on fabricated evidence.

**Stage 1 binding rule:**

- **Single hypothesis bound** by static evidence (audit + controller verification) → proceed to Stage 2 with the fix shape from §3.
- **Two+ plausible after static** → add Bundle 1b: instrument-and-smoke probe (log lines at `handleTutClickSide` entry capturing payload + lookup result, plus `Provider.Load` post-load `byKey` enumeration filtered to TriggerTutorial keys); user re-runs server + Java client; logs settle binding.
- **All five negative + audit surfaces an H6+** → bind the new hypothesis; size and proceed to Stage 2.
- **Stale tracker / pre-NAI-110 binding suspected** → re-grep `nai_followups.md` and HEAD per `spec_followup_tracker_freshness`.

---

## 5. Stage 2 — fix dispatch

Per Q2=A, the fix lands in NAI-112 regardless of LOC. Cadence:

- **subagent-driven-development** per `execution_mode_default`.
- **Plan dispatch** with controller pre-flight per `controller_preflight` (30s grep+Read pass against HEAD before each implementer dispatch).
- **TDD:** red test → green fix → close.
- **Goscape-only defensive checks** labeled per `defensive_gate_doc_comment_label`.
- **TS divergences** (e.g., args-shape changes) tracked per `true_to_ts_gate` with deviation note in close commit.
- **Test-helper coverage** cross-checked against all consumers per `plan_helper_coverage` if a shared helper is introduced.

**LOC scope guardrail:** if Stage-1 attribution sizes Stage 2 at >~80 LOC across 3+ opcodes (multi-opcode H3 chain), pause and confirm with user before auto-expanding NAI-112; otherwise proceed.

---

## 6. Stage 3 — smoke handoff

Per `smoke_test_server_handoff`, Stage-2 smoke is user-launched.

**Smoke path:**
1. User runs goscape server + Java client rev-225.
2. Log in; walk Tutorial Island chatbox steps to `tut_flash(^tab_inventory)`.
3. Click the inventory tab.

**Pass criteria:** chatbox advances AND inventory side panel displays.

**Fail criteria:** symptom unchanged (per `smoke_unchanged_means_multiple_blockers`: brainstorm under-diagnosed; re-open Stage 1 with smoke-bound evidence) OR new blocker (route per `cascade_theory_smoke_binding`).

**Cascade attribution close:** smoke binds NAI-112 if pass; new follow-up sub-spec opens if fail.

**Smoke close commit:** `Closes memory:` trailer per `close_commit_memory_trailer`.

---

## 7. Test discipline

- **Stage 1:** read-only; no test changes. Audit subagent does not write code.
- **Stage 2:** TDD per goscape convention. Unit tests live alongside the fix.
  - H1 fix → `pkg/script/provider_test.go` or pack-server-equivalent: load a fixture `script.dat` containing `[tutorial,_]`; assert `byKey[uint32(TriggerTutorial)]` resolves.
  - H2 fix → out of scope (Client-Java).
  - H3 fix → `pkg/script/handler_*_test.go` for the new handler(s); red-then-green per NAI-110 template.
  - H4 fix → extend `modules/world/handler_interface_test.go:71-128` to assert intArgs propagation.
  - H5 fix → unit test asserting both tiered-fallback paths fire correctly.
- **Smoke:** Stage 3 user-launched.

---

## 8. Risk register

- **R1 — Audit subagent fabricates `LookupKey` arithmetic or TS-handler claims.** Mitigation: controller HEAD-verification pass (mandatory; §4) per `audit_subagent_fabrication`.
- **R2 — Static probe inconclusive on H1** because pack-server is in a TS-only repo and goscape's RuneScript compile path differs. Mitigation: instrument-and-smoke fallback (Q1=C escape hatch); `Provider.Load` post-load `byKey` enumeration log line.
- **R3 — H3 binds with a multi-opcode chain.** Mitigation: §5 LOC guardrail at ~80 LOC; pause for confirmation rather than auto-expand.
- **R4 — Stage-2 fix surfaces adjacent divergence at smoke.** Routing per `smoke_surfaces_adjacent_divergences` (≤30 LOC stretch in-scope; else NAI-113).
- **R5 — `[tutorial,_]` registered as `byKey[159]` but `Provider.Load` skips it because `LookupKey == 0xFFFFFFFF`** (provider.go:100 gate). Mitigation: Stage 1.1 explicitly checks for the 0xFFFFFFFF sentinel in the `[tutorial,_]` decode path.
- **R6 — Tracker assertions about `nai_followups.md` rot during NAI-112.** Mitigation: per `spec_followup_tracker_freshness`, re-grep+Read tracker assertions against HEAD at plan-write and at each implementer dispatch.

---

## 9. Verified premises (controller pre-flight at HEAD `e3eecbe`)

- ✅ `modules/world/handler_interface.go:138-149` — `handleTutClickSide` shape as quoted in §1.
- ✅ `pkg/script/provider.go:145-153` — `GetByTriggerSpecific` returns `byKey[uint32(trigger)]` for `(-1, -1)`.
- ✅ `pkg/script/lookup_key.go:18-20` — `LookupKeyForGlobal(t)` = `uint32(t)`.
- ✅ `pkg/script/trigger.go:164` — `TriggerTutorial = 159`.
- ✅ `pkg/script/provider.go:100-102` — `Provider.Load` registers `byKey[f.LookupKey]` if `f.LookupKey != 0xFFFFFFFF`.
- ✅ `modules/world/handler_interface_test.go:71-128` — 3 unit tests pass (`Provider.Register(...)` based; bypasses cache load).

---

## 10. Cadence summary

- **Bundle 0** — controller pre-flight ✅ (this spec § 9; no commits).
- **Bundle 1** — Stage 1 audit subagent + controller verification + binding decision.
- **Bundle 1b** (conditional) — instrument-and-smoke fallback if §4 binding rule yields "two+ plausible".
- **Bundle 2+** — Stage 2 fix (TDD; shape per §3 + §5).
- **Smoke handoff** — user-launched (§6).
- **Close commit** — cascade attribution + `Closes memory:` trailer.

---

## 11. Decisions captured

- **Q1 = C** — Stage 1 static-first; instrument-and-smoke only if inconclusive.
- **Q2 = A** — Stage 2 fix lands in NAI-112 regardless of LOC (subject to §5 guardrail).
- **Q3 = B** — Full-method TS audit before hypothesis-binding (surfaces H4/H5 alongside catalogued H1/H2/H3).
- **Q4 = B** — Single Sonnet audit subagent + controller HEAD-verification.
