# NAI-75 — Investigation+fix: SPLIT_* opcode port to unblock chatnpc dialog rendering

- **Sub-spec**: NAI-75
- **Date**: 2026-05-03
- **Scope label**: Investigation-and-fix sub-spec — Stage 1 short-circuited at brainstorm time (root cause concrete: `pkg/script/handlers_string.go:99-132` ships SPLIT_INIT/GET/PAGECOUNT/LINECOUNT/GETANIM as constant-returning stubs marked "deferred to later sub-spec"; NAI-75 IS that sub-spec). Stage 2 light-fidelity SPLIT_* port: respect `|` as forced line break, chunk into pages of `linesPerPage` lines, real `splitPages [][]string` + `splitMesanim int32` ScriptState fields, real handlers for all 5 opcodes, fixed SPLIT_INIT pop arity (currently pops 2 ints — should pop 3 per `popInts(3)` in `LostCityRS/Engine-TS/src/engine/script/handlers/StringOps.ts:77`). Two new tracked deviations opened (NAI-75-D-FONT-WRAP-NAIVE, NAI-75-D-MESANIM-NOT-PORTED) — both blocked on future FontType / MesanimType cache loaders. User-mediated Java-client smoke (per `smoke_test_server_handoff.md`) as binding feature-correctness gate. Conditional Stage 3 deferred to NAI-76 if smoke surfaces additional content blockers (door interaction, RS Guide re-talk, downstream chatnpc consumers) that are NOT downstream of the same SPLIT_* root cause.
- **Predecessors**: NAI-74 (session-log subsystem foundation) — last on `main` as `9c925dc`
- **Source root**:
  - `LostCityRS/Engine-TS` (TS canonical for `pkg/script` per `ts_source_canonical_path.md`)
  - `LostCityRS/Client-Java` (binding consumer of `OpIfOpenChat` + `OpIfSetText` packets; chatnpc dialog rendering)
  - `2004scape/Server/data/src/scripts/interface_chat/scripts/chat.rs2` (consumer-side proof: `[proc,chatnpc]`, `[proc,chatnpc_page]`, `[proc,p_choice2]`)

## Motivation

NAI-74 closed cleanly with the session-log subsystem foundation. The user ran a manual Java-client smoke against HEAD `9c925dc` to begin walking Tutorial Island content. **The RuneScape Guide chatnpc dialog flow is broken**: a fresh-account opnpc1 click on the Guide jumps directly to the `p_choice2` "Select an Option" popup with choices "Yes please." / "No, thank you." — skipping the preceding `~chatnpc("<p,neutral>Do you want to skip the tutorial?")` preamble. Picking "No, thank you." closes the choice popup and updates the hint arrow from the Guide to the door (correct) but does NOT render the 5 expected `~chatnpc` lines that follow ("Greetings!", "You have already learnt...", "You will find many inhabitants...", "I would also suggest reading...", "To continue the tutorial go through that door..."). Net symptom: 6 sequential `~chatnpc` calls in `runescape_guide_welcome` all silently no-op while the rest of the script (varp set + hint update) executes correctly.

The user separately reports inability to use the door or re-talk to the Guide post-symptom. Working hypothesis: same root cause (door's oploc and Guide's opnpc1 dispatch fire chatnpc subprocs that no-op identically; click is registered but produces no visible chat).

Brainstorm-time grep confirmed the underlying bug deterministically. The TS `[proc,chatnpc]` body in `2004scape/Server/data/src/scripts/interface_chat/scripts/chat.rs2:303-311` reads:

```
[proc,chatnpc](string $string)
split_init($string, 380, 4, q8);
def_int $page = 0;
def_int $pagetotal = split_pagecount;
while ($page < $pagetotal) {
    ~chatnpc_page(npc_name, npc_type, $page);
    facesquare(npc_coord);
    if(npc_getmode ! opplayer2 & npc_getmode ! applayer2) npc_setmode(playerfaceclose);
    p_pausebutton;
    $page = calc($page + 1);
}
```

`split_pagecount` is declared `(int)`. In goscape's `pkg/script/handlers_string.go:129-132` it is implemented as:

```go
func handleSplitPageCount(s *ScriptState) error {
    s.PushInt(0)
    return nil
}
```

`split_pagecount` returns 0 unconditionally. The chatnpc proc's `while ($page < 0)` loop iterates zero times. `p_pausebutton` is never invoked. The proc completes immediately with no visible side effect. All 6 chatnpc calls in the Welcome flow execute the same way; the script falls through to `tutorial_progress = 4` + `~set_tutorial_progress`, which DO have visible side effects (varp set + hint arrow). User observes the hint arrow advance but no chat — exactly the reported symptom.

`p_choice2` (the choice popup that DID render correctly) does not depend on SPLIT_*. Its body uses `if_settext` + `if_openchat(multi2)` + `if_setresumebuttons` + `p_pausebutton` — all of which are wired in goscape today (`handlers_interface.go`, `handlers_dialog.go`, `resume_dialog.go`). That confirms `p_pausebutton` itself is functional and the bug is isolated to the SPLIT_* stub set.

`grep -c split_init|split_pagecount|split_get|split_linecount|split_getanim` across `2004scape/Server/data/src/scripts/` returns 216 matches. SPLIT_* underpins ALL chat dialog procs (`chatnpc`, `chatnpcrange`, `chatnpcnoturn`, `chatplayer`, `mesbox`, plus their `.npc_name`-prefixed variants). Until NAI-75 ships, no chat dialog renders anywhere in the runescript content layer. This is the binding blocker for Tutorial Island playability and for every NPC-dialog content path in the world.

## Tech stack

- Go 1.26+
- Existing packages **read** from at brainstorm time:
  - `pkg/script/handlers_string.go:99-132` (current SPLIT_* stubs — full retire target).
  - `pkg/script/state.go:135-...` (`type ScriptState struct` — needs new `splitPages [][]string` + `splitMesanim int32` fields).
  - `pkg/script/opcode.go:425-429` (SPLIT_* opcode constants, all already present at correct numeric ids).
  - `pkg/script/handlers.go:155-159` (dispatch table bindings, already wired to the stubs).
  - `pkg/script/handlers_dialog.go:1-30` (`P_PAUSEBUTTON` handler — confirms `Execution = PauseButton` works; no change needed).
  - `pkg/script/handlers_interface.go:23-180` (`IF_OPENCHAT`, `IF_SETTEXT`, `IF_SETNPCHEAD`, `IF_SETANIM` — all wired; chatnpc_page proc dependencies already present).
  - `modules/world/resume_dialog.go:8-30` (RESUME_PAUSEBUTTON resumes the suspended script — confirms the resume path works).
  - `modules/world/player.go:355-360` (`writeOut(gameserver.OpIfOpenChat, payload)` — wire-out is wired).
  - `LostCityRS/Engine-TS/src/engine/script/handlers/StringOps.ts:76-122` (canonical SPLIT_* implementations — port reference).
  - `LostCityRS/Engine-TS/src/engine/script/ScriptState.ts` (canonical `splitPages` + `splitMesanim` field declarations — port reference for ScriptState shape).
  - `LostCityRS/Engine-TS/src/cache/config/FontType.ts` (canonical FontType — `font.split(text, maxWidth)` is the word-wrap entry; out-of-scope for NAI-75 light-fidelity port; deferred via NAI-75-D-FONT-WRAP-NAIVE).
  - `LostCityRS/Engine-TS/src/cache/config/MesanimType.ts` (canonical MesanimType — `<p,name>` chathead-anim lookup; out-of-scope for NAI-75; deferred via NAI-75-D-MESANIM-NOT-PORTED).
  - `2004scape/Server/data/src/scripts/interface_chat/scripts/chat.rs2:303-360` (consumer-side: `[proc,chatnpc]`, `[proc,chatnpc_page]`, sibling chat procs — proves the bytecode call shape).
- Modified files in `pkg/script/`:
  - `state.go` — add two ScriptState fields: `SplitPages [][]string` (per-page, per-line wrapped text) and `SplitMesanim int32` (mesanim type id from `<p,name>` prefix; -1 if absent). Init both in any `Reset`/init helper that touches existing slice fields.
  - `handlers_string.go` — replace the 5 stub bodies with real implementations:
    - `handleSplitInit`: pop `(text, maxWidth, linesPerPage, fontId)` per `popInts(3)` + `popString()` (current stub pops only 2 ints — bug fix). Parse `<p,name>` mesanim prefix into `s.SplitMesanim` (set to -1 if absent; light-fidelity skips MesanimType lookup, defers to NAI-75-D-MESANIM-NOT-PORTED with a logged debug message). Strip the prefix from text. Light-fidelity wrap: split on `|` (manual line-break char used in scripts) into raw lines; chunk into pages of `linesPerPage` lines each. Store in `s.SplitPages`. Ignore `maxWidth` and `fontId` (deferred per NAI-75-D-FONT-WRAP-NAIVE; documented inline at the helper). Per `defensive_gate_doc_comment_label.md`, both deferrals labelled "(NAI-75-D-… — deferred to FontType/MesanimType port)".
    - `handleSplitGet`: pop `(page, line)`. Push `s.SplitPages[page][line]`. Bounds-check both indices; on out-of-bounds, push empty string + log debug (matches TS behavior of throwing on undefined access — goscape defensive divergence per `defensive_gate_doc_comment_label.md`).
    - `handleSplitPageCount`: push `len(s.SplitPages)`.
    - `handleSplitLineCount`: pop page; push `len(s.SplitPages[page])`. Bounds-check; out-of-bounds pushes 0 + log debug.
    - `handleSplitGetAnim`: pop page. Per NAI-75-D-MESANIM-NOT-PORTED, push -1 unconditionally (TS path requires `MesanimValid.len[lineCount-1]` lookup which depends on MesanimType cache that goscape doesn't have). Documented inline.
- Modified files in `pkg/script/` (tests):
  - `handlers_string_test.go` — extend (or create if absent — Stage 2 grep+Read at plan-author time confirms) with the following assertions:
    - `SPLIT_INIT` of `"<p,neutral>Greetings...|first line|second line"` with `(maxWidth=380, linesPerPage=4, fontId=q8)` populates `SplitPages = [[Greetings..., first line, second line]]`, `SplitMesanim = -1` (defensive: not -1 only after MesanimType ports), pages=1.
    - Multi-page chunking: input with 5 `|`-separated lines and `linesPerPage=4` → 2 pages, page 0 has 4 lines, page 1 has 1 line.
    - `SPLIT_PAGECOUNT` returns `len(SplitPages)`.
    - `SPLIT_LINECOUNT(0)` returns 4 in the multi-page case.
    - `SPLIT_GET(0, 1)` returns the second line of page 0.
    - `SPLIT_GETANIM(0)` returns -1 (NAI-75-D-MESANIM-NOT-PORTED pin).
    - Out-of-bounds `SPLIT_GET(99, 99)` pushes "" + does not panic (defensive).
    - End-to-end runescript-fixture test (per `plan_runnable_test_fixtures.md`): a synthetic `[proc,chatnpc_test]` body that does `split_init` + `while page < pagecount` + `p_pausebutton`-equivalent (substituted with a script-end check) verifies the loop iterates the expected number of times. Mentally executed at plan-author time.
- Modified files in `modules/world/`:
  - None expected. The wire-out path (`OpIfOpenChat`, `OpIfSetText`) is already wired. If smoke surfaces a wire-framing gap, scope expands at smoke-failure time per `investigation_subspec_cadence.md` Stage 3 trigger.
- New files: none anticipated.
- Memory files:
  - `~/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md` — at NAI-75 close: append "## NAI-75 — CLOSED <date>" close entry following the established NAI-N section template; append two new active deviations (NAI-75-D-FONT-WRAP-NAIVE, NAI-75-D-MESANIM-NOT-PORTED) with closure paths; net deviation count 12 → 14 (-0 +2).
  - Close commit body carries `Closes:` for any retired tracker rows (none expected — NAI-74's 16 deferred `addSessionLog` tracker rows are independent of this surface) and `Opens:` for the two new deviations per `close_commit_memory_trailer.md`.
  - Potentially-new memory entries (added at close if surfaced): see "Memory entries to potentially add at NAI-75 close" section below.

## Scope

### Stage 1 — Static audit (already short-circuited at brainstorm)

Stage 1 verdict: **CONCLUSIVE_BUG_FOUND** at the script-VM layer. Substages 1.2-1.4 (wire framing, production wiring, encoder edges) skipped per the established `investigation_subspec_cadence.md` short-circuit rule.

The audit's smoking-gun citation is in Motivation above:
- Symptom: 6 chatnpc calls render zero visible dialogs but the post-loop varp+hint side-effects fire.
- Cause: `handleSplitPageCount` returns 0 → chatnpc proc's `while ($page < $pagetotal)` loop never iterates → `p_pausebutton` never fires → no `if_openchat` packet ever leaves the server.
- Fix layer: replace 5 stub bodies in `pkg/script/handlers_string.go` with real implementations + extend `ScriptState` with `SplitPages` + `SplitMesanim` fields.

No subagent dispatch needed for Stage 1. The plan can go straight to Stage 2.

### Stage 2 — SPLIT_* light-fidelity port (TDD)

Stage 2 follows TDD discipline per `superpowers:test-driven-development`: red → green → refactor. Bundle structure is plan-author's call but the working hypothesis below assumes a single bundle since all surfaces are colocated in `pkg/script/`.

**Task 2.1 — ScriptState fields.** Add `SplitPages [][]string` + `SplitMesanim int32` to `ScriptState` (per `pkg/script/state.go:135-…`). Initialise both to nil/-1 in any existing `Reset`/init helper that touches sibling slice fields (plan-author greps for the pattern; if no central reset exists, document why per-handler init is OK). Single failing test asserts a fresh `ScriptState{}` has zero-valued fields.

**Task 2.2 — SPLIT_INIT real implementation.** Pop arity fix (2 ints → 3 ints) + `<p,name>` mesanim prefix parse + `|`-split + page chunking. Light-fidelity skips font-aware wrap and MesanimType lookup. Each deferral labelled inline per `defensive_gate_doc_comment_label.md`. TDD: failing test for pop-arity, prefix-parse, prefix-stripped, `|`-split, multi-page chunking.

**Task 2.3 — SPLIT_GET / SPLIT_PAGECOUNT / SPLIT_LINECOUNT / SPLIT_GETANIM real implementations.** Per the per-handler spec in Tech stack. Each handler gets a TDD-red-then-green test pair. SPLIT_GETANIM pinned at -1 with explicit comment citing NAI-75-D-MESANIM-NOT-PORTED.

**Task 2.4 — Tracker entries opened.** Append `NAI-75-D-FONT-WRAP-NAIVE` + `NAI-75-D-MESANIM-NOT-PORTED` to `nai_followups.md` "Active deviations" with closure paths (FontType cache loader sub-spec; MesanimType cache loader + chathead-anim wire sub-spec). Per `retire_deviation_grep_all_comments.md` precedent at write-time: every `pkg/script/handlers_string.go` doc-comment that references the deferral cites the deviation tag.

**Task 2.5 — Verify implementer claims.** Per `verify_implementer_claims.md`: `git show <SHA> --stat` after each task commit; fresh `go test ./... -count=1` and `-race` from a clean checkout; verify against HEAD not against IDE.

### Smoke handoff (between Stage 2 and close commit)

Per `smoke_test_server_handoff.md`: ask the user to launch the server with the latest binary and connect with the Java client. User reports each:

- **Chatnpc preamble "Do you want to skip the tutorial?" renders before the choice2 popup** → SPLIT_INIT path good.
- **Picking "No, thank you." renders 5 sequential chat dialogs (Greetings → Talking → Inhabitants → Website → Door) with click-through** → multi-call chatnpc + pausebutton good.
- **The npc-name + chathead area renders** (even if chathead anim is static — NAI-75-D-MESANIM-NOT-PORTED ack'd) → if_setnpchead + chatnpc_page wiring good.
- **Door interaction works** (clicking the door fires the door's oploc which presumably uses chatnpc; door state advances) → downstream confirmation that the same fix unblocks oploc-driven chat.
- **RS Guide re-talk works** (after dialog completion, clicking Guide again fires `@runescape_guide_return`'s 3 chatnpcs) → repeated-dispatch confirmation.

If all 5 pass → close commit with `Opens:` trailer for the 2 new deviations.

If chatnpc renders BUT door/re-talk still broken → those are downstream bugs of a different root cause; NAI-76 opens to investigate. NAI-75 still closes (its scoped fix shipped).

If chatnpc still doesn't render → enter Stage 3.

### Stage 3 — Runtime instrumentation (conditional)

Only created if Stage 2's smoke test fails on chatnpc rendering specifically. Plan adds a Bundle 3 at that point.

**Task 3.1.** Add gated `slog.Info` logs (env-var-controlled) at:
- `pkg/script/handlers_string.go::handleSplitInit`: log final `SplitPages` length + first-line preview.
- `pkg/script/handlers_string.go::handleSplitPageCount`: log returned count.
- `pkg/script/handlers_dialog.go::handlePPauseButton`: log "suspending now" with Self id.
- `modules/world/player.go` near the OpIfOpenChat write: log payload bytes len.

**Task 3.2.** User runs server with the env var set, captures log, sends back.

**Task 3.3.** Analyze logs, identify root cause, iterate Stage 2 with new findings. Re-run smoke handoff.

**Task 3.4.** Once smoke passes, Stage 3 instrumentation removed before close commit.

### Bundle structure (working hypothesis)

Per `compressed_cadence.md`: total Stage 2 LOC estimate is ~80 (2 ScriptState fields + 5 handler bodies + ~25 LOC of tests). NOT compressed-cadence territory (>15 LOC). Standard cadence applies.

- **Bundle 1 — Stage 2 SPLIT_* port (5 tasks).** TDD per task; one commit per task; controller-side merge to main per `superpowers:subagent-driven-development`. Per `superpowers_code_reviewer_model.md` reviewer dispatches use Sonnet (never Opus).
- **Smoke handoff.** Out-of-band; no commit.
- **Bundle 2 (conditional).** Stage 3 instrumentation if smoke fails on chatnpc render.
- **Close commit.** chore-tagged, `Opens:` trailer for the 2 new deviations. Updates `nai_followups.md`. Net deviation count 12 → 14.

## True-to-TS gate

Per `true_to_ts_gate.md`: every behavioral divergence needs a tracked deviation with rationale + follow-up. NAI-75's gate behavior:

**Source-of-truth precedence:**
1. `LostCityRS/Engine-TS/src/engine/script/handlers/StringOps.ts:76-122` for SPLIT_* opcode shape (pop arity, push value, mesanim semantics).
2. `LostCityRS/Engine-TS/src/engine/script/ScriptState.ts` for `splitPages` / `splitMesanim` field shape.
3. `2004scape/Server/data/src/scripts/interface_chat/scripts/chat.rs2:303-…` for caller shape (the runescript proc that drives SPLIT_*).

**Tracked divergences (new deviations opened):**

- **NAI-75-D-FONT-WRAP-NAIVE.** Light-fidelity wrap respects `|` as forced line break but does not run TS's `font.split(text, maxWidth)` algorithm. Consequence: long lines that exceed `maxWidth` pixels but contain no `|` are NOT wrapped; they overflow the dialog component. The runescript content layer historically uses explicit `|` for chatnpc lines (verified by inspection of `runescape_guide.rs2`, `tutorial.rs2`), so this works for tutorial smoke. Closure path: future FontType cache loader sub-spec ports the per-character pixel-width font + the `Font.split(text, maxWidth)` algorithm; NAI-75-D-FONT-WRAP-NAIVE retires when SPLIT_INIT calls the new loader.

- **NAI-75-D-MESANIM-NOT-PORTED.** SPLIT_INIT parses the `<p,name>` mesanim prefix string but does NOT resolve it to a MesanimType id (sets `SplitMesanim = -1` regardless). SPLIT_GETANIM unconditionally returns -1. Consequence: chathead animations on chat dialogs are absent (static head, no talk-anim). Closure path: future MesanimType cache loader sub-spec adds `MesanimType` to the `Configs` interface; NAI-75-D-MESANIM-NOT-PORTED retires when SPLIT_INIT writes a real id and SPLIT_GETANIM reads `MesanimType.len[lineCount-1]`.

**Defensive divergences (labelled per `defensive_gate_doc_comment_label.md`, NOT tracked deviations):**

- SPLIT_GET / SPLIT_LINECOUNT bounds-check + push empty/0 on out-of-bounds. TS throws on undefined access. Goscape-defensive comment cites `defensive_gate_doc_comment_label.md`.

## Risks & mitigations

- **R1 — Real chatnpc text contains content that needs wrap NOT manual `|` breaks.** If any tutorial chatnpc string is wider than 380px without `|` breaks, light-fidelity wrap overflows the dialog. **Mitigation:** Pre-Stage-2 grep `2004scape/Server/data/src/scripts/tutorial/` for chatnpc strings, eyeball the longest ones; if any look wider than the npcchat dialog component, escalate scope to include a minimal char-count wrap (split on whitespace at ~60-char boundaries). Likely-not-needed: tutorial scripts use `|` consistently.

- **R2 — `<p,name>` prefix parsing edge cases.** TS parses by `text.startsWith('<p,') && text.indexOf('>') !== -1`. Edge cases: nested `<p,`, missing `>`, empty name. **Mitigation:** Test fixture covers each. Match TS substring slicing exactly.

- **R3 — `popInts(3)` order semantics.** TS `popInts(amount)` fills array from index `amount-1` down to 0, so `[a, b, c] = popInts(3)` gives `a` = first-pushed (deepest), `c` = last-pushed (top of stack). Goscape's stub pops only 2 ints in the wrong order. Stage 2 must reproduce TS order exactly. **Mitigation:** test fixture pushes 3 distinct ints in known order, asserts handler reads them as `(maxWidth=push1, linesPerPage=push2, fontId=push3)`.

- **R4 — Multi-call chatnpc + reused ScriptState.** The chatnpc proc is called 6 times in the Welcome flow on the SAME ScriptState. Each `split_init` call must replace `SplitPages` (not append). **Mitigation:** SPLIT_INIT explicitly assigns (not appends) — `s.SplitPages = pages`. Test fixture chains two split_init calls with different inputs and asserts the second replaces, not extends.

- **R5 — Pausebutton / resume cycles vs ScriptState reuse.** `p_pausebutton` suspends the script. Resume re-enters with the SAME ScriptState. The next iteration of the chatnpc proc's `while` loop calls `~chatnpc_page` again. Implementation must NOT reset `SplitPages` on resume (which would break the loop). **Mitigation:** SPLIT_INIT only assigns; PAUSEBUTTON / resume don't touch SplitPages; verified by reading `handlers_dialog.go`.

- **R6 — Smoke uncovers door/re-talk not downstream.** Per the conditional-Stage-3 plan above, those are NAI-76 if NAI-75's chat fix doesn't unblock them. Spec stays scoped.

- **R7 — `controller_preflight.md` premise rot.** If audit takes long enough that file paths drift, Stage 2 dispatch must re-grep at HEAD. Standard discipline.

- **R8 — `verify_implementer_claims.md` failure modes.** Stage 2 implementer reports test-green from package scope; controller verifies with `go test ./...` from a fresh checkout before close commit. Three failure modes the controller watches for: stale IDE diagnostics, package-scoped green masking cross-package breakage, false "pre-existing failures" attributions. Each task's commit verified independently.

- **R9 — `risk_register_premise_grep.md` recurrence.** R3-R5 above are explicit cross-call-chain assertions about ScriptState lifecycle — each one needs grep+Read evidence, not analogy reasoning. Plan-author validates each at premise-pre-flight time.

- **R10 — Test-passes-for-wrong-reason (per `test_passes_for_wrong_reason.md`).** A SPLIT_GET test fixture that pre-populates `SplitPages` directly (instead of going through SPLIT_INIT) would mask SPLIT_INIT bugs. **Mitigation:** end-to-end test fixture uses real SPLIT_INIT to populate; bounds-check tests use direct field-write but explicitly mark as "infra test, not handler test."

## Sequencing

Stage 2 task ordering: 2.1 (ScriptState fields) → 2.2 (SPLIT_INIT) → 2.3 (the other 4 handlers) → 2.4 (deviation entries) → 2.5 (verify). 2.1 blocks 2.2 (handler implementation needs the fields); 2.2 blocks 2.3 (SPLIT_INIT populates the fields the others read). 2.3's 4 handlers can ship in a single commit (each is ≤10 LOC).

Smoke handoff blocks close commit. Conditional Stage 3 (if needed) blocks close commit re-entry.

## Open questions for plan-author

- **Q1.** Should Task 2.3 ship one commit (all 4 simple handlers) or four (one per handler)? **Recommendation:** one commit — each handler is trivial and they share a logical unit; per `compressed_cadence.md`, simple handler quartet is OK as one TDD red-then-green cycle with 4 sub-asserts.
- **Q2.** Should `SplitPages [][]string` be a slice-of-slices or a flat slice with separator markers? **Recommendation:** slice-of-slices — matches TS shape exactly, indexing is O(1), bounds-checks are natural, and it's what handlers_string.go's stub-replacement signature already implies.
- **Q3.** Test framework — should the end-to-end fixture use the existing test-helper that builds `&ScriptState{}` + sets `Pointers`, or a minimal new helper? **Recommendation:** existing helper per `scriptstate_test_fixture_idioms.md` — `&ScriptState{}` + `StackCapacity` init + correct push order + `Pointers` flag. Plan-author re-reads the memory entry before writing the test code-block.
- **Q4.** Does NAI-75 retire any of the 16 deferred TS `addSessionLog` tracker rows from NAI-74? **Recommendation:** no — they are independent of this surface (login flow, advanceStat, TCP teardown, ClientCheat). Net deviation count 12 → 14 (-0 +2 new).

## Memory entries to potentially add at NAI-75 close

The following candidates may warrant memory entries at close (decision deferred to close-time review):

- **stub-deferred-comment-as-canonical-marker** — pattern: a stubbed handler body with comment "deferred to later sub-spec" is itself a canonical marker that the deferred sub-spec exists but is unscheduled. Brainstorm-time grep across `pkg/` for "deferred to later sub-spec" surfaces a backlog of these candidate sub-specs in concrete code form. NAI-75 is the first instance of one of these stubs being explicitly retired.
- **content-driven-investigation-cadence** — NAI-75 is the first investigation sub-spec triggered by content-side smoke (Java client manual test) rather than by a NAI-N-D tracker entry from a prior port. The Stage 1 short-circuit was conclusive enough at brainstorm to skip subagent dispatch entirely. If this pattern recurs, capture as: "content-driven investigations often have shorter Stage 1 because the bug surface is constrained by what the content actually exercises."
- **chatnpc-text-uses-pipe-as-line-break** — content-side convention: `|` is the explicit line-break char in chatnpc strings. Confirmed by inspection of `runescape_guide.rs2`, `tutorial.rs2`. Worth memorializing if a future content-port sub-spec needs to know.

These are candidates only; close-time review decides which actually become memory entries based on whether NAI-75's experience confirmed the pattern.
