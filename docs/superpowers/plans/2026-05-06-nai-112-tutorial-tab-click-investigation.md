# NAI-112 Tutorial-Tab-Click Chatbox-Advance Investigation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Stage 1 audit-and-bind for the smoke-bound symptom from NAI-110 close (clicking the inventory tab on Tutorial Island advances neither the chatbox nor displays the inventory side panel). Static-first audit; instrument-and-smoke fallback only on inconclusive. Output: a diagnosis report with a single bound hypothesis (H1/H2/H3/H4/H5/H6+), and a follow-up tracker entry that hands off to a Stage-2 fix plan written separately once binding lands.

**Architecture:** Single Sonnet audit subagent reads the TS reference chain (Engine-TS handler + script provider, Server content `tutorial.rs2`, Client-Java rev-225 sidebar dispatcher) end-to-end, then validates the goscape side (pack-server/RuneScript `LookupKey` derivation, `Provider.Load` registration, `[tutorial,_]` opcode coverage) against it. Controller HEAD-verifies every claim in the audit report (per `audit_subagent_fabrication`) before binding. If two-or-more hypotheses remain plausible after static, controller dispatches Bundle 1b: a temporary instrumentation patch + smoke handoff + revert. **No production code changes in NAI-112 Bundle 1.** Stage-2 fix plan is authored after binding lands.

**Tech Stack:** Go 1.26+. TS reference: `LostCityRS/Engine-TS` (per `ts_source_canonical_path`). Java client: `LostCityRS/Client-Java` rev-225. Content: `LostCityRS/Server/content/scripts/tutorial/scripts/tutorial.rs2`.

**Spec:** `docs/superpowers/specs/2026-05-06-nai-112-tutorial-tab-click-investigation-design.md`

---

## File Structure

**Created (Bundle 1):**
- `docs/superpowers/investigations/2026-05-06-nai-112-stage1-audit.md` — audit subagent's report; per-hypothesis verdict + file:line evidence + Stage-2 fix-shape sizing.

**Modified (Bundle 1 close):**
- `/home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md` — append "## NAI-112 — Stage 1 BOUND" section with cascade routing to Stage 2 (in-NAI-112 separate plan).

**Created conditionally (Bundle 1b — instrument-and-smoke fallback only):**
- *None new.* Two surgical edits made temporarily, then reverted in the same bundle:
  - `modules/world/handler_interface.go:138-149` — log `tab` byte + `sf == nil` outcome at handleTutClickSide entry.
  - `pkg/script/provider.go:103-105` — log `byKey` keys filtered by `key < 256` (i.e., global-tier registrations) at `Provider.Load` end.

**Read-only references (audit input):**

Goscape:
- `modules/world/handler_interface.go` (handleTutClickSide; lines 135-149)
- `modules/world/handler_interface_test.go` (Provider.Register fixture-based tests; lines 71-128)
- `pkg/script/provider.go` (`Load` lines 42-106; `GetByTrigger` lines 114-127; `GetByTriggerSpecific` lines 145-153; `Register` lines 182-190)
- `pkg/script/lookup_key.go` (full file; 21 lines)
- `pkg/script/trigger.go` (lines 155-173 for the trigger constant block; line 164 for `TriggerTutorial`)
- `pkg/script/decode.go` (script-blob decoder — for LookupKey field origin)
- `pkg/script/handler_*.go` (opcode-handler registration tables; for H3 cross-check)
- `cmd/goscape/app/` and any pack-server tooling under `cmd/` or `tools/` (locate the RuneScript-head-to-LookupKey path; if absent, the cache is pre-built externally and the audit reads `script.dat` directly)

External:
- `LostCityRS/Engine-TS/src/network/game/client/handler/TutClickSideHandler.ts` (TS wire handler)
- `LostCityRS/Engine-TS/src/lostcity/engine/script/ScriptProvider.ts` (or equivalent — TS lookup-key derivation)
- `LostCityRS/Engine-TS/src/lostcity/engine/script/ServerTriggerType.ts` (TS trigger enum)
- `LostCityRS/Server/content/scripts/tutorial/scripts/tutorial.rs2` (lines 143-176 per spec; expand if `[tutorial,_]` is elsewhere)
- `LostCityRS/Client-Java/src/main/java/...` rev-225 (sidebar-tab click dispatcher; locate by grep for opcode 175 send-site or sidebar-tab handler)

---

## Conventions for this plan

- **No production code changes in Bundle 1.** Stage 2 production work is authored as a separate plan after Bundle 1 binding lands.
- **`go` invocation prefix** (per global CLAUDE.md): every `go test`/`go build` runs as `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go ...`.
- **Subagent fabrication guard** (`audit_subagent_fabrication`, `verify_implementer_claims`): controller verifies every claimed file:line citation, `LookupKey` arithmetic, and TS-handler line range with `Read` / `rg` / `git show HEAD -- <file>` before binding. Highest fabrication risk: TS `LookupKey` derivation (cross-repo, unfamiliar territory).
- **Commit policy:** Bundle 1 has at most two commits — (a) the audit report doc; (b) the close commit with `nai_followups.md` update. Bundle 1b adds two extra commits (instrument + revert) iff triggered. No `--no-verify`. All commits use `git commit --no-gpg-sign` per global CLAUDE.md.
- **Closes memory trailer** on the close commit per `close_commit_memory_trailer`.
- **Short-circuit policy:** if the audit subagent reports a single-hypothesis bind with HEAD-verified evidence, skip Bundle 1b. If two-or-more remain plausible, dispatch Bundle 1b.

---

## Task 1: Bundle 0 — controller pre-flight at HEAD (no commits)

**Purpose:** Re-verify spec §9 premises against current HEAD before dispatching audit work. The spec was committed at `a4e21ce`; pre-flight observations were captured at `7797185`. HEAD has moved one commit. Stale citations cause wasted audit cycles (`controller_preflight`, `spec_followup_tracker_freshness`).

**Files:** read-only.

- [ ] **Step 1.1: Verify HEAD shape**

Run: `git log --oneline -3`

Expected: `a4e21ce docs(spec): NAI-112 …` at HEAD; `7797185 chore(close): NAI-110 …` at HEAD~1; `81f9c53 feat(script): NAI-110 T2 …` at HEAD~2. If unexpected commits appear, halt and reconcile.

- [ ] **Step 1.2: Verify `handleTutClickSide` shape**

Run: `sed -n '135,150p' modules/world/handler_interface.go`

Expected: function reads `tab := int(payload[0])`, gates `tab < 0 || tab > 13`, calls `s.scriptProvider.GetByTriggerSpecific(script.TriggerTutorial, -1, -1)`, calls `s.runScript(sf, p, nil, true, nil, nil)`. If the call has been changed (e.g., to pass intArgs), halt and update the spec.

- [ ] **Step 1.3: Verify `GetByTriggerSpecific` global-lookup branch**

Run: `sed -n '145,153p' pkg/script/provider.go`

Expected: when both `typeID == -1` and `categoryID == -1`, returns `p.byKey[uint32(trigger)]` directly. No selector bits, no shift. If the global branch has changed, halt and update the spec.

- [ ] **Step 1.4: Verify `LookupKeyForGlobal` is `uint32(trigger)`**

Run: `sed -n '16,21p' pkg/script/lookup_key.go`

Expected: `return uint32(trigger)` — no shift, no selector. If shifts/selectors have been added, halt and update the spec.

- [ ] **Step 1.5: Verify `TriggerTutorial = 159`**

Run: `rg -n 'TriggerTutorial' pkg/script/trigger.go`

Expected: single hit at line 164: `TriggerTutorial    ServerTriggerType = 159`. If the value has changed, halt and update spec + audit prompt.

- [ ] **Step 1.6: Verify `Provider.Load` `byKey` registration gate**

Run: `sed -n '95,106p' pkg/script/provider.go`

Expected: `if f.LookupKey != 0xFFFFFFFF { p.byKey[f.LookupKey] = f }`. If the gate has been removed or changed, halt and update R5 in spec §8.

- [ ] **Step 1.7: Verify `handler_interface_test.go` unit tests are at the cited lines**

Run: `sed -n '71,128p' modules/world/handler_interface_test.go`

Expected: 3 unit tests covering handleTutClickSide (one for in-range tab firing the script, one for out-of-range tab not firing, one for `Provider.Register(...)` fixture). If layout differs, update spec §9.

**Halt criterion:** if any pre-flight step diverges, controller updates the spec inline (no commit) before dispatching Task 2 — or aborts and re-brainstorms.

---

## Task 2: Bundle 1 — dispatch Stage 1 audit subagent

**Purpose:** Have a single Sonnet audit subagent derive the TS reference chain end-to-end and emit a hypothesis-binding recommendation with file:line evidence.

**Files:**
- Create: `docs/superpowers/investigations/2026-05-06-nai-112-stage1-audit.md` (subagent writes; controller does not pre-create).

- [ ] **Step 2.1: Confirm investigations directory exists**

Run: `ls -d docs/superpowers/investigations/ 2>/dev/null && echo OK || mkdir -p docs/superpowers/investigations && echo CREATED`

Expected: `OK` (directory already exists from NAI-99 / NAI-97). If `CREATED`, fine — the subagent will write into it.

- [ ] **Step 2.2: Dispatch the audit subagent**

Use the `Agent` tool with `subagent_type=Explore` (read-only across multiple repos), `model=sonnet` (per `superpowers_code_reviewer_model`-equivalent cost discipline; audit work is read-heavy), and the prompt below. The subagent has Read access to `/home/owner/Code/github.com/zsrv/goscape`, `/home/owner/Code/github.com/LostCityRS/Engine-TS`, `/home/owner/Code/github.com/LostCityRS/Client-Java`, and `/home/owner/Code/github.com/LostCityRS/Server`.

```
Audit task — NAI-112 Stage 1 (read-only).

You are the Stage 1 audit subagent for goscape investigation sub-spec NAI-112.

CONTEXT:
- Goscape is a Go port of LostCityRS/Engine-TS, communicating with LostCityRS/Client-Java rev-225.
- After the NAI-110 close (commit 7797185), user-launched smoke (2026-05-06) reported: clicking the inventory tab on Tutorial Island (post `tut_flash(^tab_inventory)`) advances neither the chatbox nor displays the inventory side panel. No warn log was reported.
- Goscape's inbound packet handler `handleTutClickSide` (modules/world/handler_interface.go:138-149) reads opcode 175, gates 0≤tab≤13, calls `s.scriptProvider.GetByTriggerSpecific(script.TriggerTutorial=159, -1, -1)` (returns `byKey[uint32(159)]` directly — no fallback), and calls `s.runScript(sf, p, nil, true, nil, nil)` — no script args.
- Unit tests at modules/world/handler_interface_test.go:71-128 pass via `Provider.Register(...)` fixture, which bypasses `Provider.Load` cache-load path.
- Spec at: docs/superpowers/specs/2026-05-06-nai-112-tutorial-tab-click-investigation-design.md (read this first for full context).

YOUR TASK — derive the TS reference chain end-to-end, then bind a hypothesis. Read in this order:

1. /home/owner/Code/github.com/LostCityRS/Engine-TS/src/network/game/client/handler/TutClickSideHandler.ts — full body. Note:
   a. The lookup call shape: does TS use `getByTriggerSpecific` (single-tier global), `getByTrigger` (3-tier with fallback), or some other dispatch?
   b. The `runScript` arg shape: does TS construct ScriptState with the tab byte as an argument? If yes, what arg index?
   c. Any pre-dispatch gates beyond the 0-13 range check.

2. /home/owner/Code/github.com/LostCityRS/Engine-TS — locate ScriptProvider (likely at src/lostcity/engine/script/ScriptProvider.ts or similar). Find the `LookupKey` derivation for a script header. Determine: how does the TS RuneScript compiler encode `[tutorial,_]` (wildcard subject)? What `LookupKey` does it produce?

3. /home/owner/Code/github.com/LostCityRS/Server/content/scripts/tutorial/scripts/tutorial.rs2 — locate `[tutorial,_]` (spec cites lines 143-176; verify and expand if necessary). Enumerate every opcode the body invokes. Note any `getarg` reads (would bind H4).

4. /home/owner/Code/github.com/LostCityRS/Client-Java rev-225 — locate the sidebar-tab click dispatcher. Confirm or refute opcode 175 (TUT_CLICKSIDE) transmission on Tutorial-Island inventory-tab click. Note any `overrideChat` / mode gates per `java_client_coord_chat_suppression.md` discipline.

5. GOSCAPE SIDE — cross-check:
   a. Locate the pack-server / RuneScript-compile path that produces `LookupKey` for a script header. If goscape has its own compiler at `cmd/` or `tools/`, walk a `[tutorial,_]` head through it. If goscape consumes pre-built script.dat (built externally by the LostCityRS RuneScript compiler), document that — and note that H1 may require reading the compiler's encoding directly.
   b. For every opcode in the rs2 body (3), grep `pkg/script/handler_*.go` registration tables to confirm a handler is wired. Flag any unhandled opcode.
   c. Re-read pkg/script/provider.go:42-106 (Load), :114-127 (GetByTrigger), :145-153 (GetByTriggerSpecific), :180-207 (Register/RegisterAt). Verify the spec's claims about behavior.

OUTPUT: write to docs/superpowers/investigations/2026-05-06-nai-112-stage1-audit.md, structured as:

  # NAI-112 Stage 1 — Tutorial-tab-click chatbox-advance audit

  ## TS reference chain summary
  - TutClickSideHandler.ts shape: <one paragraph + file:line citations>
  - LookupKey derivation for `[tutorial,_]`: <one paragraph + file:line>
  - `[tutorial,_]` body opcode list: <bulleted list with rs2:line for each>
  - Client-Java sidebar dispatcher: <one paragraph + file:line; opcode 175 sent yes/no>
  - Goscape pack-server / compile path: <one paragraph + file:line, OR "consumes pre-built script.dat externally — see X">

  ## Per-hypothesis verdict

  ### H1 — `[tutorial,_]` not under byKey[159]
  Verdict: confirmed | refuted | inconclusive
  Evidence: <file:line + arithmetic>
  Fix-shape size (if confirmed): <LOC estimate + which goscape files>

  ### H2 — Java client doesn't send opcode 175
  (same shape)

  ### H3 — downstream opcode aborts in [tutorial,_] body
  (same shape; if confirmed, list which opcode + handler-registration miss)

  ### H4 — runScript args mismatch
  (same shape; if confirmed, cite getarg reads in rs2 body)

  ### H5 — GetByTriggerSpecific too narrow vs TS dispatch
  (same shape)

  ### H6+ (if surfaced)
  <name + verdict + evidence + fix-shape>

  ## Recommended binding
  <single hypothesis or "two+ plausible — Bundle 1b instrumentation needed">

  ## "Verified at HEAD" claims for controller spot-check
  - <file:line citation 1>
  - <file:line citation 2>
  - <LookupKey arithmetic claim>
  - ...

CONSTRAINTS:
- Read-only. Do NOT modify production code or tests.
- Cite every TS / Client-Java / rs2 line range with file:line. Quote relevant lines verbatim where helpful.
- For LookupKey arithmetic, show the bit math explicitly (e.g., "trigger=159, selector=0b00, typeID/category absent → key = 159").
- Per `audit_subagent_fabrication.md`: do not invent file paths or line numbers; if you cannot find a TS file, say so explicitly. Do not synthesize arithmetic.
- Use Read for files; do NOT modify anything.
- Length cap: aim ≤ 4000 words. Cite, don't quote large blocks.

When done, summarize the recommended binding in <200 words for the controller's transcript.
```

- [ ] **Step 2.3: Receive subagent report**

Read the audit subagent's summary. Cross-check that `docs/superpowers/investigations/2026-05-06-nai-112-stage1-audit.md` was created. If not, halt — re-dispatch with stricter output instructions.

**Do NOT commit yet.** Controller verification (Task 3) precedes any commit.

---

## Task 3: Bundle 1 — controller HEAD-verification of audit claims

**Purpose:** Verify every "Verified at HEAD" claim from the audit report against current HEAD (`a4e21ce` or later). Per `audit_subagent_fabrication`, any unverifiable claim disqualifies that hypothesis from binding.

**Files:** read-only. Audit report is `docs/superpowers/investigations/2026-05-06-nai-112-stage1-audit.md`.

- [ ] **Step 3.1: Re-grep every goscape file:line citation in the audit report**

For each goscape `<path>:<line>` claim in the audit's "Verified at HEAD" list:

```bash
sed -n '<line-2>,<line+2>p' <path>
```

Verify the cited line matches the audit's quoted content. If a citation mismatches HEAD, mark that claim ❌ and re-evaluate the hypothesis it supports.

- [ ] **Step 3.2: Re-read every TS / Client-Java / rs2 line range cited**

For each external repo citation:

```bash
sed -n '<start>,<end>p' /home/owner/Code/github.com/LostCityRS/<repo>/<path>
```

Verify the cited content matches the audit's claim. If a citation cannot be located (file missing, wrong path), mark ❌.

- [ ] **Step 3.3: Re-derive any `LookupKey` arithmetic**

For each `LookupKey` claim, manually compute: `key = uint32(trigger) | (selector << 8) | (typeID << 10)` per `pkg/script/lookup_key.go`. Confirm against the audit's stated key.

- [ ] **Step 3.4: Spot-check the rs2 opcode list against goscape's handler tables**

For each opcode the audit claims is unhandled (H3-relevant):

```bash
rg -n '<opcode-name>|<opcode-numeric-code>' pkg/script/handler_*.go pkg/script/opcode.go
```

Confirm absence (H3 binding) or presence (H3 refuted). Note: a constant declared in `pkg/script/opcode.go` without a registered handler is the NAI-110 / NAI-109 shape — surface this explicitly.

- [ ] **Step 3.5: Decide binding**

Based on verified evidence:
- **Single hypothesis bound (all key claims ✅):** record the binding decision; proceed to Task 4.
- **Two-or-more plausible:** dispatch Bundle 1b (Task 5).
- **Audit surfaced an H6+:** treat as a new hypothesis; bind if evidence is HEAD-verified, else dispatch Bundle 1b instrumentation focused on the new hypothesis.

If ANY citation failed verification, controller appends a "Controller-revised verdict" section to the audit report inline (no commit yet) noting which claims were rejected and adjusting the binding.

---

## Task 4: Bundle 1 close — write follow-up tracker entry + commit

**Purpose:** Persist the audit report + binding decision to git; update `nai_followups.md` so the Stage-2 plan can be authored against a stable reference.

**Files:**
- Modify: `/home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md`
- Stage: `docs/superpowers/investigations/2026-05-06-nai-112-stage1-audit.md` (created in Task 2; possibly amended with controller-revised verdict in Task 3.5)

- [ ] **Step 4.1: Append NAI-112 Stage 1 section to `nai_followups.md`**

Append (do not overwrite) at the bottom of the file:

```markdown
---

## NAI-112 — Stage 1 BOUND <YYYY-MM-DD>

**Bound hypothesis:** H<N> — <one-line description>
**Evidence:** <file:line + arithmetic; ≤3 lines>
**Sized fix shape:** <LOC estimate + goscape files to touch>
**Stage 2 routing:** in-NAI-112 separate plan (per spec §5; LOC ≤ 80 guardrail).
**Audit report:** `docs/superpowers/investigations/2026-05-06-nai-112-stage1-audit.md`
**Controller-verified at HEAD:** <commit-sha>

**Stage 2 carry-forward:** controller writes `docs/superpowers/plans/<YYYY-MM-DD>-nai-112-stage2-<binding-tag>.md` next session (or this session if no /clear).
```

Replace `<N>`, `<YYYY-MM-DD>`, `<sha>`, `<binding-tag>`, etc. with concrete values from Task 3.

- [ ] **Step 4.2: Stage and commit**

```bash
git add docs/superpowers/investigations/2026-05-06-nai-112-stage1-audit.md
git status
```

Expected: only the audit report is staged. The `nai_followups.md` file is in `~/.claude/projects/...`, outside the repo — it's not staged, just persisted on disk.

```bash
git commit --no-gpg-sign -m "$(cat <<'EOF'
docs(investigation): NAI-112 Stage 1 — <binding-tag> bound

<one-paragraph summary of the binding: which hypothesis, evidence,
sized fix shape, controller-verified at HEAD>.

Closes memory: investigation_subspec_cadence (Stage 1 close instance) ·
audit_subagent_fabrication (controller HEAD-verification gate held;
<N> claims spot-checked, <M> rejected) · controller_preflight
(Bundle 0 verified <K> spec citations at HEAD).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

- [ ] **Step 4.3: Verify commit**

Run: `git log --oneline -2`

Expected: `<new-sha> docs(investigation): NAI-112 Stage 1 …` at HEAD; `a4e21ce docs(spec): NAI-112 …` at HEAD~1.

- [ ] **Step 4.4: Emit Stage-2 resume prompt for user**

If Bundle 1b was NOT triggered (single bind from static), emit a paste-ready resume prompt for the next session:

```
Author Stage-2 fix plan for NAI-112 — <binding-tag>.

Spec: docs/superpowers/specs/2026-05-06-nai-112-tutorial-tab-click-investigation-design.md
Stage 1 audit: docs/superpowers/investigations/2026-05-06-nai-112-stage1-audit.md (commit <sha>)
Bound hypothesis: H<N> — <description>
Sized fix shape: <LOC estimate + files>

Stage 2 cadence: subagent-driven-development per execution_mode_default;
TDD red→green; LOC guardrail at ~80 (spec §5); close commit with
Closes memory: trailer.

Skip brainstorm; go straight to writing-plans against the bound
hypothesis. The bound hypothesis is the spec's §3 H<N>; copy the
fix-shape template from there.
```

If Bundle 1b WAS triggered (Task 5 below ran), the resume prompt comes after Bundle 1b close, not here.

---

## Task 5: Bundle 1b — instrument-and-smoke fallback (CONDITIONAL)

**Trigger:** Task 3.5 binding decision was "two-or-more plausible" or "H6+ needs runtime evidence". Skip this task entirely if Task 3.5 produced a single bind.

**Purpose:** Add temporary log lines that discriminate the remaining hypotheses on a single user-driven smoke run, then revert.

**Files:**
- Modify (temporarily): `modules/world/handler_interface.go:138-149`
- Modify (temporarily): `pkg/script/provider.go:103-105`

- [ ] **Step 5.1: Add instrumentation at `handleTutClickSide`**

Edit `modules/world/handler_interface.go:138-149` to log entry payload + lookup result:

```go
// handleTutClickSide handles client opcode 175 (TUT_CLICKSIDE).
// Body: u8 sidebar tab index. Fires [tutorial] if tab is in [0,13].
// Mirrors TS TutClickSideHandler.ts.
func (s *Server) handleTutClickSide(p *Player, payload []byte) error {
	if len(payload) < 1 {
		return nil
	}
	tab := int(payload[0])
	slog.Info("NAI-112 instr: TUT_CLICKSIDE entry", "tab", tab, "payloadLen", len(payload))
	if tab < 0 || tab > 13 {
		slog.Info("NAI-112 instr: TUT_CLICKSIDE out-of-range; not firing", "tab", tab)
		return nil
	}
	sf := s.scriptProvider.GetByTriggerSpecific(script.TriggerTutorial, -1, -1)
	slog.Info("NAI-112 instr: TUT_CLICKSIDE lookup", "tab", tab, "scriptFound", sf != nil)
	s.runScript(sf, p, nil, true, nil, nil)
	return nil
}
```

Ensure `log/slog` is imported (it usually is at the top of the file; if not, add it).

- [ ] **Step 5.2: Add instrumentation at `Provider.Load` end**

Edit `pkg/script/provider.go:103-105` to enumerate global-tier registrations after the load loop. Insert before the `return nil`:

```go
	// NAI-112 instr: enumerate global-tier registrations (key < 256).
	for k, f := range p.byKey {
		if k < 256 {
			slog.Info("NAI-112 instr: byKey global-tier registration", "key", k, "scriptName", f.Name)
		}
	}

	return nil
```

- [ ] **Step 5.3: Build to confirm clean compile**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...`

Expected: clean exit. If any error, fix the import / typo before proceeding.

- [ ] **Step 5.4: Run unit tests to confirm no regression**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... ./pkg/script/...`

Expected: PASS. If existing tests fail because they assert log absence, re-evaluate (likely the log line interferes with a test fixture; downgrade to `slog.Debug` or guard with a build tag).

- [ ] **Step 5.5: Commit instrumentation**

```bash
git add modules/world/handler_interface.go pkg/script/provider.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
chore(instr): NAI-112 Bundle 1b — TUT_CLICKSIDE + Provider.Load logs

Temporary instrumentation for Stage 1 hypothesis discrimination.
Logs handleTutClickSide entry payload + lookup result, and
Provider.Load global-tier byKey registrations on load.

Reverted in next commit after user-launched smoke produces logs.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

- [ ] **Step 5.6: Smoke handoff to user**

Output to user, paste-ready:

```
NAI-112 Bundle 1b — instrumentation in place at <commit-sha>.

Please run goscape against the Java client rev-225:

  CGO_ENABLED=0 GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go run -trimpath ./cmd/goscape --config.file config.yaml

Server-launch logs will include lines like:
  level=INFO msg="NAI-112 instr: byKey global-tier registration" key=159 scriptName=...

When you click the inventory tab on Tutorial Island (post tut_flash),
two more lines fire:
  level=INFO msg="NAI-112 instr: TUT_CLICKSIDE entry" tab=...
  level=INFO msg="NAI-112 instr: TUT_CLICKSIDE lookup" tab=... scriptFound=...

Paste:
  (a) all "NAI-112 instr" lines from server boot through one inventory-
      tab click during the chatbox sequence;
  (b) confirm whether the chatbox advanced or remained stuck.

I'll attribute the binding from the logs and revert the instrumentation
in the next commit.
```

Wait for user-pasted logs.

- [ ] **Step 5.7: Bind hypothesis from logs**

Decision matrix:
- **No "TUT_CLICKSIDE entry" line on tab click** → H2 bound (Java client didn't send opcode 175). Cross-check Client-Java audit.
- **"TUT_CLICKSIDE entry" but `scriptFound=false`** → H1 bound (`[tutorial,_]` not under byKey[159]).
- **"TUT_CLICKSIDE entry" + `scriptFound=true` + chatbox unchanged** → H3 (downstream opcode abort) or H4 (args mismatch); cross-check via the `byKey` boot-log: if `key=159` registration is present, falls to H3 or H4.
- **`byKey global-tier registration` log shows key=159 with scriptName=`[tutorial,_]`** → H1 refuted; H3/H4/H5 remain.

Append the binding decision + log excerpts to `docs/superpowers/investigations/2026-05-06-nai-112-stage1-audit.md` under a new section `## Bundle 1b — runtime evidence`.

- [ ] **Step 5.8: Revert instrumentation**

Revert the two production-file edits. Hand-revert (do not `git revert` the previous commit; the revert needs to leave the audit-report changes intact):

Edit `modules/world/handler_interface.go:138-149` back to its pre-Bundle-1b shape (3 `slog.Info` lines removed; `tab<0 || tab>13` early return restored to its single-line shape per Task 1 step 1.2).

Edit `pkg/script/provider.go:103-105` back to its pre-Bundle-1b shape (4-line instrumentation block removed; trailing `return nil` only).

- [ ] **Step 5.9: Verify revert is byte-identical to pre-Bundle-1b shape**

```bash
git diff a4e21ce -- modules/world/handler_interface.go pkg/script/provider.go
```

Expected: empty output. Iff non-empty, fix the revert until it matches.

- [ ] **Step 5.10: Re-run unit tests after revert**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... ./pkg/script/...`

Expected: PASS.

- [ ] **Step 5.11: Commit revert + audit-report addendum**

```bash
git add modules/world/handler_interface.go pkg/script/provider.go docs/superpowers/investigations/2026-05-06-nai-112-stage1-audit.md
```

If Task 4 already committed the audit report, the report file may have new edits (the §Bundle 1b section). Otherwise it's still uncommitted from Task 2.

```bash
git commit --no-gpg-sign -m "$(cat <<'EOF'
chore(close): NAI-112 Bundle 1b — runtime-bound H<N>; revert instrumentation

User-launched smoke produced logs that bind H<N> (<one-line description>).
Audit report appended with §Bundle 1b runtime evidence section.
Instrumentation reverted to pre-Bundle-1b state (byte-diff vs a4e21ce = empty).

Closes memory: investigation_subspec_cadence (Bundle 1b instrument-and-smoke
fallback instance) · cascade_theory_smoke_binding (runtime evidence binds
attribution).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

- [ ] **Step 5.12: Update `nai_followups.md` with the runtime binding**

If Task 4 step 4.1 already wrote the entry, edit it to reflect the runtime binding (replace placeholder `<N>` with the runtime-bound hypothesis).

- [ ] **Step 5.13: Emit Stage-2 resume prompt for user**

Same shape as Task 4 step 4.4, with `<binding-tag>` reflecting the runtime-bound hypothesis.

---

## Task 6: Bundle 1 / 1b complete — Stage-2 plan handoff

**Purpose:** Mark NAI-112 Stage 1 closed; queue Stage 2 fix plan authoring.

- [ ] **Step 6.1: Confirm artifacts in place**

Run: `git log --oneline -3 && ls docs/superpowers/investigations/2026-05-06-nai-112-stage1-audit.md`

Expected: investigation file exists; commits show NAI-112 Stage 1 closed (1 commit if Task 5 skipped, 3 commits if Task 5 ran).

- [ ] **Step 6.2: Confirm Stage-2 plan is NOT in this plan's scope**

Per the spec §10 cadence summary, Stage 2 fix plan is authored as a separate `docs/superpowers/plans/<YYYY-MM-DD>-nai-112-stage2-<binding-tag>.md` doc, written after binding lands. The Stage-2 plan template lives in spec §3 (per-hypothesis fix shape) and §5 (cadence + LOC guardrail).

- [ ] **Step 6.3: End of plan**

NAI-112 Bundle 1 (and conditionally 1b) is complete. The user receives the Task 4.4 (or Task 5.13) resume prompt and may /clear → fresh session for Stage 2 plan authoring + implementation.

---

## Self-review (post-write)

Verified before declaring this plan complete:

1. **Spec coverage:**
   - Spec §1 context → Plan File Structure read-only refs + Task 1 pre-flight ✅
   - Spec §2 in-scope (Stage 1) → Task 2 audit + Task 3 verification + Task 4 close ✅
   - Spec §2 in-scope (Stage 2) → explicitly out-of-this-plan; Task 6 step 6.2 routes ✅
   - Spec §3 hypothesis register → audit-subagent prompt covers all 5 hypotheses ✅
   - Spec §4 Stage 1 audit dispatch → Task 2 ✅
   - Spec §5 Stage 2 fix dispatch → out-of-this-plan; Task 6 routes ✅
   - Spec §6 Stage 3 smoke → out-of-this-plan; Task 5.6 covers Bundle 1b smoke if triggered ✅
   - Spec §7 test discipline (Bundle 1 read-only) → "No production code changes in Bundle 1" convention + Task 5.4/5.10 unit-test gates ✅
   - Spec §8 risks R1-R6 → R1 (Task 3 verification), R2 (Bundle 1b), R3 (Task 4.4 prompt), R4 (Task 6 routing), R5 (Task 1 step 1.6), R6 (Task 1 pre-flight) ✅
   - Spec §9 verified premises → Task 1 (re-verification at current HEAD) ✅
   - Spec §10 cadence summary → Tasks 1-6 mapped ✅
   - Spec §11 Q-decisions → embedded in plan structure ✅

2. **Placeholders:** None. Concrete file:line refs, exact shell commands, full audit-subagent prompt, full revert criteria, exact commit message templates with HEREDOC.

3. **Type/signature consistency:** `handleTutClickSide` signature matches spec §1 + Task 1.2 + Task 5.1 (parameters: `p *Player, payload []byte; return error`). `GetByTriggerSpecific` signature matches spec §1 + Task 1.3 + audit-prompt step 5c. `Provider.Load` line range 42-106 consistent across all citations.

4. **No invented symbols:** all referenced functions / files exist at HEAD `a4e21ce` per Bundle 0 pre-flight.
