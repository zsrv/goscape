# NAI-24 — follow-up bundle (PlayerOps NumberNotNull audit + INV_TRANSMIT source remediation)

- **Sub-spec**: NAI-24
- **Date**: 2026-04-25
- **Scope label**: B (logical-grouping follow-up bundle — `pkg/script` only; ~30-50 LOC production + ~30-50 LOC tests across 2 bundles; advances the `From NAI-23` NumberNotNull-sweep tracker on its highest-leverage remaining file; resolves the NAI-23 INV_TRANSMIT source-uid divergence as a silent porting-bug fix; introduces 0 new deviations; net deviation count 14 → 14)
- **Predecessors**: NAI-23 (follow-up bundle) — last on `main` as `a1b2b0f`
- **TS source root**: `LostCityRS/Engine-TS`

## Motivation

Two actionable items land in NAI-24:

1. **`handlers_player.go` NumberNotNull audit (Bundle 1).** The `From NAI-23` tracker entry at `nai_followups.md:1394-1397` enumerates `handlers_player.go` as the highest-leverage remaining audit-pass file: TS counterpart `PlayerOps.ts` has 35 unwrapped NumberNotNull sites (56 total minus 21 absorbed by NAI-23 Bundle 4c's IF_* audit). Pre-flight against HEAD `a1b2b0f` confirms density: `pkg/script/handlers_player.go` is 715 LOC with 49 `s.PopInt()` call sites and 3 existing `checkNotNull` wraps (lines 104/121/700 — `handleAnimProtect`, `handleAllowDesign`, `handleMidiJingle`) — comparable to NAI-23 Bundle 4b's `handlers_inv.go` (50 pops, 0 wraps going in) and Bundle 4c's `handlers_interface.go` (35 pops, 0 wraps going in). Bundle 1 applies the NAI-23 Bundle 4 audit cadence verbatim on this single file: per-pop-site WRAP / SKIP / ESCALATE rubric anchored to PlayerOps.ts; one `checkNotNull` wrap per WRAP; one negative-pin test per WRAP; single feat commit with the audit table embedded in the message.

2. **INV_TRANSMIT source-uid remediation (Bundle 2).** The `From NAI-23` tracker entry at `nai_followups.md:1356-1386` documents a divergence surfaced by NAI-23 Bundle 4b's code-quality review: `handleInvTransmit` at `pkg/script/handlers_inv.go:429` calls `s.Self.InvListenOnCom(invType, com, -1)` (source=-1, world-shared inventory dispatch); TS `InvOps.ts:650` calls `state.activePlayer.invListenOnCom(invType.id, com, state.activePlayer.uid)` (source = active player's own uid). Origin commit `5b67653` (S6u). Pre-flight equivalence determination against `(*Player).invListenOnCom` docstring at `modules/world/player.go:632-633` and the `updateInvs` dispatch at `:471-479`: **not equivalent**. `-1` reads from `Server.invs[Type]`; `p.uid` reads from `Server.players[uid].invs[Type]`. For a typical backpack listen, those resolve to different inventory objects — INV_TRANSMIT in goscape is reading from the wrong store. Bundle 2 applies a 1-line silent porting-bug fix (no deviation tag), updates the doc-comment narration to TS-faithful, flips one existing test assertion from `Source: -1` to `Source: <self.uid>`, and marks the NAI-23 tracker entry Resolved.

The two items cluster naturally by package boundary (both in `pkg/script`) and by theme (audit-pass fidelity hygiene). Bundle 1 is the primary scope item and follows established Bundle 4 audit cadence; Bundle 2 is a bounded cleanup that absorbs a NAI-23 spillover before it goes stale. Bundles touch disjoint files (`handlers_player.go` vs. `handlers_inv.go`) — no inter-bundle dependencies.

## Tech stack

- Go 1.26+
- Existing packages touched:
  - `pkg/script/handlers_player.go` (Bundle 1: per-handler `checkNotNull` wraps — count determined by per-pop-site audit)
  - `pkg/script/handlers_inv.go` (Bundle 2: 1-line production fix at line 429 + doc-comment narration update at lines 412-419)
- Test files touched:
  - `pkg/script/handlers_player_test.go` (Bundle 1: per-handler null-pin tests; one test per newly added WRAP)
  - `pkg/script/handlers_inv_test.go` (Bundle 2: flip `TestInvTransmitRegistersListener` assertion at lines 386-412 from `Source: -1` to `Source: <self.uid>`)
- Memory file:
  - `~/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md` (NAI-24 close: Bundle 2 marks tracker entry at line 1356-1386 Resolved; new "From NAI-24" section adds the now-dead `-1` API-surface deferral)
- No new files in production packages.

## Scope

### Bundle 1 — `handlers_player.go` NumberNotNull audit

**Goal**: Audit every `s.PopInt()` call in `pkg/script/handlers_player.go` and add `checkNotNull` wraps where the TS counterpart in `PlayerOps.ts` applies `NumberNotNull`. Adds a per-handler null-pin test for each newly wrapped handler. Closes the `From NAI-23` tracker's `handlers_player.go` audit-pass ask.

**Source**: NAI-23 close — tracker entry `Future NumberNotNull sweep targets (out-of-scope file enumeration)` at `nai_followups.md:1388-1418`.

#### TS source canonical path

`/home/owner/Code/github.com/LostCityRS/Engine-TS/src/engine/script/handlers/PlayerOps.ts` (per `ts_source_canonical_path` memory). Note: TS has no separate file boundary between IF_* and player-ops — both live in PlayerOps.ts. NAI-23 Bundle 4c absorbed the IF_* sites; NAI-24 Bundle 1 covers the remaining player-ops surface that maps to goscape's `pkg/script/handlers_player.go`.

#### Per-pop-site decision rubric

For each `s.PopInt()` site in `handlers_player.go`, the implementer subagent applies this rubric anchored to the matching TS `popInt` in `PlayerOps.ts`:

1. **TS wraps with `check(state.popInt(), NumberNotNull)`** → **WRAP**. Add `if err := checkNotNull(v, "OP_NAME"); err != nil { return err }` at the same logical position (immediately after the `s.PopInt()` that produces the value). The op-name string follows the existing convention (lowercase opcode mnemonic, e.g., `"p_aprange"`, `"p_telejump"`). The implementer subagent reads the 5 pre-existing wraps in `handlers_player.go` as templates and reuses the casing convention.

2. **TS wraps with a typed validator** (`StatValid`, `SeqTypeValid`, `VarPlayerValid`, `CategoryTypeValid`, `EnumTypeValid`, `LocAngleValid`, `SpotanimTypeValid`, etc.) → **SKIP**. The typed validator is a separate fidelity gate that lives outside the NumberNotNull sweep charter. Audit table records `<ValidatorName>` in the rationale.

3. **TS does not wrap the popped value at all** → **SKIP**. Preserves TS tolerance. Audit table records `TS does not wrap; preserve tolerance`.

4. **The popped value is semantically signed** (coord delta, search-relative offset, arithmetic operand, queue-arg slot count, varbit-cleared sentinel) → **SKIP** regardless of TS. `-1` is a legitimate value here. Audit table records `signed value; -1 sentinel does not apply`.

5. **Ambiguous** (TS reads opaque, multiple TS sites with diverging treatment, or the goscape pop order doesn't match TS) → **ESCALATE** to the controller before deciding. Per NAI-23 precedent: implementer subagent reports the escalation in its summary and the controller resolves before the audit table is finalized.

This rubric mirrors NAI-23 Bundle 4 exactly — no methodological divergence.

#### Pre-existing wraps are pre-WRAPs

`handlers_player.go` enters NAI-24 with **3** existing `checkNotNull` wraps at lines 104 (`handleAnimProtect` / `"P_ANIMPROTECT"`), 121 (`handleAllowDesign` / `"ALLOWDESIGN"`), 700 (`handleMidiJingle` / `"MIDI_JINGLE"`). The audit covers **all 49** `s.PopInt()` sites; the 3 pre-existing wraps appear in the audit table as `WRAP (pre-existing)` rows confirming they're TS-faithful. Net new wraps land among the remaining 46 popInt sites between the SKIP/WRAP split.

#### Wrap shape

```go
v := s.PopInt()
if err := checkNotNull(v, "OP_NAME"); err != nil {
    return err
}
```

`OP_NAME` matches the existing `checkNotNull` consumer pattern. The implementer subagent reads existing wrapped handlers as templates and reuses the casing convention.

#### Audit table format (canonical artifact)

Embedded in the Bundle 1 feat commit message, mirroring NAI-23 Bundle 4c's shape (commit `ab9c681`):

| Handler | popInt context | TS wraps? | Decision | Rationale (TS file:line) |
|---------|---------------|-----------|----------|-------------------------|
| handleSomeOp | com | NumberNotNull | WRAP | PlayerOps.ts:NNN |
| handleOther | x | not wrapped | SKIP | TS does not wrap x (PlayerOps.ts:NNN-NNN) |
| ... | ... | ... | ... | ... |

The table is the canonical archaeology — preserves per-pop-site reasoning for the next sweep and answers "why didn't NAI-24 wrap X" decisively. Skip-reason breakdown summarized at the end (typed-validator skips / signed-sentinel skips / TS-not-wrapped skips counts).

#### Touch points

1. `pkg/script/handlers_player.go`:
   - Add `checkNotNull` wraps per audit-table WRAP rows.
   - No other behavior changes; doc-comments updated only where necessary to record SKIP rationale on signed-value sites (sparing — signed-value SKIPs that are non-obvious can warrant a 1-line `// signed: <reason>` comment; obvious cases get no comment).

2. `pkg/script/handlers_player_test.go`:
   - For each WRAP row, add a negative-pin test:
     - **Naming**: `TestHandle<OpName>RejectsNullSentinel` (single-int handlers) or `TestHandle<OpName>RejectsNullSentinel/<field>` (table-driven sub-cases when a handler pops multiple wrapped ints).
     - **Fixture**: re-use the file's existing test scaffolding. The plan doc identifies the existing builder by name and confirms its signature at plan-write time (per `controller_preflight` memory).
     - **Assertion**: handler returns a non-nil error AND the error string contains the op-name token (e.g., `if err == nil || !strings.Contains(err.Error(), "OP_NAME")`).
   - Multi-wrap handlers: each newly wrapped int gets its own sub-case. Pin only one int's null at a time; the other wrapped ints stay valid so the rejection is attributable to the specific wrap.

#### Tests

- Per-handler null-pin tests: 1 per newly added WRAP (table-driven where ≥3 in same handler). Test count is bounded by the audit table's WRAP row count.

Per `plan_test_coverage_crosscheck` memory: the plan doc lists the per-handler expected-test-count so the reviewer can verify implementers didn't drop tests silently. Per-bundle cross-check is on the controller.

Per `plan_runnable_test_fixtures` memory: every plan-codified test fixture is mentally executed at spec-write time (or a `go test -run <test-name>` dry-run is performed) to catch coord/sentinel/lifecycle errors before dispatch.

Per `plan_grep_helper_patterns` memory: before prescribing inline boilerplate in plan doc, grep `handlers_player.go` for existing wrap-helper patterns (`requireActivePlayer`, `checkNotNull`) and reuse them.

#### Deviation impact

0 — audit confirms TS-faithful gates; SKIP decisions get archaeology comments only where non-obvious. No deviation tags retired or introduced.

#### Commit shape

Single feat commit: `feat(script): NAI-24 Bundle 1 — handlers_player.go NumberNotNull audit`. Body contains the audit table inline (per NAI-23 Bundle 4c precedent at commit `ab9c681`). Skip-reason breakdown summarized at the end. Standard `Co-Authored-By: Claude Opus 4.7 (1M context)` trailer.

### Bundle 2 — INV_TRANSMIT source-uid remediation

**Goal**: Fix `handleInvTransmit` at `pkg/script/handlers_inv.go:429` to pass `s.Self.UID()` instead of `-1`, matching TS `InvOps.ts:650`. Update doc-comment narration at lines 412-419 to TS-faithful. Flip the existing `TestInvTransmitRegistersListener` assertion. Mark the NAI-23 tracker entry Resolved.

**Source**: NAI-23 close — tracker entry `INV_TRANSMIT source-uid divergence (surfaced by Bundle 4b audit)` at `nai_followups.md:1356-1386`. Origin commit `5b67653` (S6u).

#### Equivalence determination (pre-flight finding)

`(*Player).invListenOnCom` docstring at `modules/world/player.go:632-633` documents:
- `Source = -1` → world-shared inventory (`Server.invs[Type]`).
- `Source >= 0` → another player's slot (`Server.players[Source].invs[Type]`).

`updateInvs()` at `modules/world/player.go:471-479` confirms the dispatch:
```go
if l.Source == -1 {
    inv = p.client.server.invs[l.Type]
} else {
    other := p.client.server.players[l.Source]
    if other == nil {
        continue
    }
    inv = other.invs[l.Type]
}
```

**Conclusion**: `-1` is **not** semantically equivalent to `self.uid`. They route through different inventory stores. The S6u port hard-coded `-1` in `handleInvTransmit`, which means INV_TRANSMIT in goscape attaches a UI component to the world-shared `Server.invs[Type]` slot rather than the player's own backpack at `Server.players[p.uid].invs[Type]`. This is a porting bug.

#### Remediation strategy: silent fix (no deviation tag)

Per the brainstorming Approach 1 decision: treat as a S6u porting bug and remediate in the same commit that surfaces it. No NAI-24-D1 tag opened (no post-fix divergence to track). The `true_to_ts_gate` memory's "every behavioral divergence needs a tracked deviation" gate applies to *active* divergences; a 1-line porting bug being fixed in the same bundle does not need a transient tag for a state that lasts one commit. The tracker entry resolution + commit hash already serve as the archaeological record.

#### Touch points

1. `pkg/script/handlers_inv.go` line 429 (production fix):
```go
// Before
s.Self.InvListenOnCom(invType, com, -1)
// After
s.Self.InvListenOnCom(invType, com, s.Self.UID())
```

2. `pkg/script/handlers_inv.go` lines 412-419 (doc-comment narration update):
```go
// handleInvTransmit implements INV_TRANSMIT. Registers a listener on
// the active player for UI component `com` tracking the active
// player's own inventory of type `invType` (source = activePlayer.UID()).
//
// TS: InvOps.ts INV_TRANSMIT — popInt(inv), popInt(com),
// activePlayer.invListenOnCom(inv, com, activePlayer.uid). com is
// wrapped with check(com, NumberNotNull) in TS; invType uses
// InvTypeValid (not NumberNotNull) — stays raw (NAI-23 Bundle 4b).
// Source porting fix landed in NAI-24 Bundle 2 — origin commit
// 5b67653 (S6u) erroneously hard-coded -1.
```

3. `pkg/script/handlers_inv_test.go` lines 386-412 (`TestInvTransmitRegistersListener` assertion flip):
   - Construct `mp := &mockPlayer{uidValue: 42}` (or any deterministic non-zero test uid; reuse an existing uidValue convention if other tests in the file already set it).
   - Assertion at lines 409-411 changes from `got.InvType != 93 || got.Com != 149 || got.Source != -1` to `got.InvType != 93 || got.Com != 149 || got.Source != 42`.
   - Update the test's doc-comment line 385 from "InvListenOnCom(invType, com, -1)" to "InvListenOnCom(invType, com, activePlayer.uid)".
   - The error-format string at line 410 changes from `"want {InvType:93, Com:149, Source:-1}"` to `"want {InvType:93, Com:149, Source:42}"`.

4. `nai_followups.md:1356-1386` (NAI-24 close, not Bundle 2 commit):
   - Append a `**Resolved 2026-04-25 (NAI-24 Bundle 2, commit `<hash>`)**` block recording: equivalence determination outcome, remediation choice (silent fix per Approach 1), cited commit hash. Preserve the original tracker body under the existing separator.

#### Tests

No new tests. The existing `TestInvTransmitRegistersListener` is flipped to pin post-fix behavior. The existing `TestInvTransmitNoActivePlayerErrors` (line 416) is unchanged — `requireActivePlayer` fires before `s.Self.UID()` is called, so the early-return path is unaffected.

The internal-API tests at `modules/world/player_inv_test.go` exercise `(*Player).invListenOnCom` directly with literal `-1` arguments to test the lazy-init / replace / nil-map paths; they're testing the listener-API contract, not the INV_TRANSMIT opcode, so they remain unchanged.

#### Cross-package pin search (controller pre-flight)

Per `enumerate_all_sites` memory: at controller dispatch time for Bundle 2, re-grep the entire repo for `lastInvListenOnCom` and `Source.*-1` to verify no test in any other package pins INV_TRANSMIT's `Source: -1`. Pre-flight verification at spec-write time confirms only `pkg/script/handlers_inv_test.go` pins INV_TRANSMIT-specific behavior at lines 405-411; INVOTHER_TRANSMIT pins at lines 487-490 are unaffected (that handler already passes the popped uid, not -1); rejection-no-call assertions at lines 541-542 and 629-630 don't pin a Source value.

#### Deviation impact

0 — silent porting-bug fix, no deviation tag opened or closed. The NAI-23 tracker entry's resolution is the archaeological record.

#### Commit shape

Single feat commit: `feat(script): NAI-24 Bundle 2 — INV_TRANSMIT source uid remediation`. Body explains the divergence (S6u origin, behavioral mismatch, equivalence-determination evidence from `(*Player).invListenOnCom` docstring + `updateInvs` dispatch), the 1-line fix, the test assertion flip, and the tracker resolution. Standard `Co-Authored-By` trailer.

## Out-of-scope (explicitly deferred)

1. **Cleanup of the `-1` API surface in `(*Player).invListenOnCom` and `updateInvs`** (`modules/world/player.go:471-479`, `:636-646`). Post-Bundle 2 there are zero production callers passing `-1` to `InvListenOnCom`, but the `script.ActivePlayer.InvListenOnCom` interface contract still documents `source=-1` as world-shared. Deciding whether to retract that API surface is a separate scope decision (interface change vs. bug fix). NAI-24 close adds a new `## From NAI-24` tracker entry documenting this deferral with the rationale (per `ts_asymmetry_dual_pin` memory: pin the active fidelity AND the conspicuous absence).

2. **Other `handlers_*.go` NumberNotNull sweep targets** (handlers_loc.go, handlers_obj.go, handlers_db.go, handlers_string.go, handlers_dialog.go, handlers_timer.go, handlers_vars.go, handlers_array.go, handlers_lastinput.go, handlers_debug.go, handlers_server.go, handlers_core.go). Each its own future audit-pass sub-spec. The tracker entry at `nai_followups.md:1388-1418` already enumerates priority order. NAI-24 close updates the entry to mark `handlers_player.go` Resolved and re-orders the remaining priorities if needed.

3. **`handlers_config.go` and `handlers_number.go` audits** — explicitly out of scope per NAI-23 Bundle 4 precedent (config-ID reads have weaker fidelity asymmetry; arithmetic operators don't use the `-1` sentinel). Inherits NAI-23's deferral.

## Risks & mitigations

- **Bundle 1 scope blow-up.** Risk: per-pop-site audit reveals more candidates than the ~30-50 LOC estimate, or requires deeper TS reading per handler than the rubric absorbs. Mitigation: rubric explicitly allows the implementer subagent to escalate "unclear" cases instead of guessing. If the audit balloons past the NAI-23 Bundle 4b density (50 pops, 4 wraps) and lands closer to Bundle 4c density (35 pops, 21 wraps), the bundle is still single-commit-sized; the controller absorbs the variance via the audit table.

- **Bundle 1 test-fixture mismatch.** Risk: the existing `handlers_player_test.go` builder doesn't match what the plan codifies, causing implementer self-catches per `plan_runnable_test_fixtures` memory. Mitigation: plan-author identifies the existing builder by name and confirms its signature at plan-write time via grep+Read. Per-test fixture is mentally executed before dispatch.

- **Bundle 2 cross-package test pin missed.** Risk: a non-script-package test pins INV_TRANSMIT's `Source: -1` and breaks silently after the flip. Mitigation: controller pre-flight grep for `lastInvListenOnCom` and `Source.*-1` across the whole repo before dispatch. Spec-write-time grep confirms no such pins outside `pkg/script/handlers_inv_test.go`.

- **Bundle 2 hidden production caller.** Risk: a non-script production code path (e.g., engine init wiring) calls `InvListenOnCom` with `-1` that the pre-flight missed. Mitigation: pre-flight already enumerated every `InvListenOnCom`/`invListenOnCom` reference in `pkg/` and `modules/`; only `handlers_inv.go:429` and `handlers_inv.go:475` are non-test production callers (line 475 already passes a popped uid, not -1). No hidden caller exists.

- **`controller_preflight` discipline at task dispatch.** Per memory: 30-second grep+Read pass against HEAD before each implementer dispatch to verify file paths, line numbers, signatures, helper init state. Applied per-bundle.

## Review structure

Per `runescript_cadence` memory: two-stage review per bundle (spec compliance → code quality, both via opus). Final whole-impl review after all bundles.

- **Bundle 1**: Stage 1 audit-table review (per-pop-site decisions cross-checked against TS) + Stage 2 code-quality review (test naming, error-message conventions, doc-comment adjustments).
- **Bundle 2**: Stage 1 spec compliance (1-line fix matches spec; doc-comment matches; assertion flip preserves test intent) + Stage 2 code-quality (cross-package pin check, narrative consistency).
- **Whole-impl review**: validates that NAI-24 closes the NAI-23 tracker entries cited (Bundle 1 entry at `nai_followups.md:1394-1397`, Bundle 2 entry at `:1356-1386`) and adds the new "From NAI-24" tracker entry for the `-1` API-surface deferral.

Polish commits land if final whole-impl review surfaces remediable findings, per NAI-23 precedent.

## NAI-24 close

The close commit:
- Updates `nai_followups.md`: marks the two NAI-23 tracker entries Resolved (Bundle 1 entry: `handlers_player.go` row in the priority list at `:1394-1397`; Bundle 2 entry: full entry at `:1356-1386`); adds the new `## From NAI-24 (2026-04-25)` section with the `-1` API-surface deferral.
- Per `close_commit_memory_trailer` memory: includes the standard `Co-Authored-By` trailer; carries `Closes memory: nai_followups.md` if memory edits are part of the close commit.
- Per `post_task_handoff` memory: at NAI-24 close, save non-derivable info to memory AND give the user a paste-ready resume prompt for NAI-25 (with HEAD hash, deviation count, and the most actionable next-NAI candidates).
