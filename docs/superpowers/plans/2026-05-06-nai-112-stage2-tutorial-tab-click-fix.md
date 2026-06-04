# NAI-112 Stage 2 — Tutorial-tab-click chatbox-advance fix

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development per `execution_mode_default` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Triangulate H6 (sub-hypotheses a/b/c) for tutorial-tab-click silent non-advance via runtime instrumentation, bind one sub-hypothesis on smoke evidence, then ship the bound fix and revert instrumentation.

**Architecture:** Two-phase. Phase 1 (Bundle 1) ships read-only instrumentation at the entry-points that discriminate H6.a/b/c — `[tutorial,_]` body branch trace, `Player.SetVarp`, `Player.OpenTutorial` / encodeOut diff suppression, `Player.IfSetText`, `handleInvAdd`. User runs the smoke; logs settle attribution. Phase 2 (Bundle 2) ships the bound fix per attribution shape (one of three task trees codified below). Bundle 3 reverts instrumentation; Bundle 4 is the user-launched final smoke + close.

**Tech Stack:** Go 1.26+; existing `log/slog` instrumentation pattern from NAI-112 Bundle 1b (`f76c2da`); subagent-driven-development cadence.

**Spec:** `docs/superpowers/specs/2026-05-06-nai-112-tutorial-tab-click-investigation-design.md` (commit `c2b8259`).
**Stage 1 audit:** `docs/superpowers/investigations/2026-05-06-nai-112-stage1-audit.md` (commits `56ab5e2`, `41748a0`).
**Bound parent hypothesis:** H6 — `[tutorial,_]` body runs but downstream effect is silently broken (smoke 2026-05-06 at HEAD `f76c2da`; revert at `41748a0`).
**Branch HEAD at plan-write:** `41748a0`.

---

## Sub-hypotheses (Stage 2.1 will bind one)

| ID | Hypothesis | Discriminating signal | Fix-shape locus | Est LOC |
|---|---|---|---|---|
| **H6.a** | Wrong branch fires — `%tutorial` varp value at click time does not match `^newbie_survival_instructor_open_inventory=20` (varp init/save-load divergence; default-zero ambiguity; or earlier tutorial step left wrong value) | `instr: TUT_CLICKSIDE entry` log line shows `tutorial_varp != 20` AND/OR no `instr: SetVarp tutorial=30` log fires after click | `modules/world/tick.go:104-106` (varps init), or `[label,start_tutorial]` flow seeding (`modules/world/login*` + the label-script invocation), or `Player.SetVarp` propagation | ≤20 |
| **H6.b** | Correct branch fires but a per-opcode handler in the `[tutorial,_]`/`set_tutorial_progress`/`tutorial_step_cut_tree` chain has a TS-divergence that nullifies the visible effect (e.g., `inv_add` silent-no-op; `if_settext` payload encoding bug; `hint_coord` aborting subsequent ops) | Branch trace shows correct path AND `SetVarp tutorial=30` fires, BUT one of `IfSetText`/`InvAdd`/`OpenTutorial` log lines is missing or has wrong arg | The diverging handler in `pkg/script/handlers_*.go` or the `Player.*` shim it calls in `modules/world/player_*.go` | ≤20 (single opcode) to ~80 (multi-opcode); pause for confirmation if >80 per spec §5 |
| **H6.c** | Modal/outbound dispatch divergence — `Player.OpenTutorial` at `modules/world/player_script.go:788-791` defers to `encodeOut` at `modules/world/player.go:387-391` which writes `OpTutOpen` ONLY when `modalTutorial != lastModalTutorial`. TS `Player.openTutorial` at `Engine-TS/src/engine/entity/Player.ts:1999-2003` writes `new TutOpen(com)` UNCONDITIONALLY on every call. When the second `tut_open(tutorial_text)` arrives with the same `com` as the prior call, goscape suppresses the wire packet; TS does not | Branch trace shows correct path AND all `IfSetText` calls fire AND `OpenTutorial(tutorial_text)` is called twice (once for `tutorial_step_view_inventory`, once for `tutorial_step_cut_tree`), BUT only one `instr: TutOpen wire` log fires (suppressed-by-diff log fires for second) | `modules/world/player_script.go:788-791` (move write to call-site) + `modules/world/player.go:387-391` (retire diff or repurpose for close-only) + `lastModalTutorial` field at `modules/world/player.go:157`-area + tests at `modules/world/player_test.go:766+`/`944+` | ≤30 |

**H6.c is the strongest static suspicion** (TS/goscape divergence already visible in code-read at plan-write — see §"Verified premises" below), but the plan does NOT pre-bind: smoke evidence from Bundle 1 instrumentation is the binding source. If smoke contradicts H6.c (e.g., the second `OpenTutorial` call doesn't fire at all because the script aborts mid-way), Bundle 2 routes to H6.a or H6.b accordingly.

---

## Verified premises (controller pre-flight at HEAD `41748a0`)

- ✅ `modules/world/handler_interface.go:138-149` — `handleTutClickSide` shape post-revert (instrumentation removed by `41748a0`).
- ✅ `modules/world/player_script.go:307-312` — `Player.Varp(id)` returns `0` for out-of-range ids; reads `p.varps[id]` otherwise.
- ✅ `modules/world/player_script.go:317-323` — `Player.SetVarp(id, val)` writes `p.varps[id] = val` then calls `p.writeVarp(id, val)`.
- ✅ `modules/world/player_varp.go:12-26` — `writeVarp` is gated by `cfg.Transmit`; non-transmit varps stay server-only (relevant for client-side visibility of `%tutorial` writes if any client logic reads them).
- ✅ `modules/world/player_script.go:788-791` — `OpenTutorial(com)` sets `modalTutorial = com` and ORs `modalStateTut`; **does NOT call `writeOut(OpTutOpen, ...)` directly**.
- ✅ `modules/world/player.go:387-391` — encodeOut diff: `if p.modalTutorial != p.lastModalTutorial { writeOut(OpTutOpen, payload); lastModalTutorial = modalTutorial }`.
- ✅ `Engine-TS/src/engine/entity/Player.ts:1999-2003` — TS `openTutorial(com) { this.write(new TutOpen(com)); modalState |= TUT; modalTutorial = com; }` — **unconditional write**.
- ✅ `modules/world/player_interface.go:15-22` — `IfSetText(com, text)` writes `OpIfSetText` immediately on every call (no diff).
- ✅ `pkg/script/handlers_inv.go:294` — `handleInvAdd` exists; will need to verify side-effects fire on instrumented run.
- ✅ `Server/content/scripts/tutorial/configs/tutorial.constant:6-7` — `^newbie_survival_instructor_open_inventory=20`; `^newbie_survival_instructor_cut_tree=30`.
- ✅ `Server/content/scripts/tutorial/scripts/tutorial.rs2:147-150` — branch shape: `else if (%tutorial = ^newbie_survival_instructor_open_inventory) { inv_add(inv, bronze_axe, 1); inv_add(inv, tinderbox, 1); %tutorial = ^newbie_survival_instructor_cut_tree; }`.
- ✅ `Server/content/scripts/tutorial/scripts/tut_chatbox_steps.rs2:33-35` — `tutorial_step_cut_tree` calls `hint_coord` then `~tutorialstep("Cut down a tree", "...")`.
- ✅ `Server/content/scripts/tutorial/scripts/tutorialstep.rs2:37-46` — `~tutorialstep` issues `if_settext(title)` then loops `~tutorialstep_page` which issues `if_settext(line1..N)` + `tut_open(tutorial_text)`.

**Controller pre-flight gate:** before each implementer dispatch, controller re-greps + Reads each cited file:line per `controller_preflight`. Any rot → fix the plan inline before dispatch.

---

## Task 1 — Bundle 1: Instrument the [tutorial,_] dispatch chain

**Files:**
- Modify: `modules/world/handler_interface.go:138-149`
- Modify: `modules/world/player_script.go:307-323` (Varp + SetVarp)
- Modify: `modules/world/player_script.go:782-791` (OpenTutorial)
- Modify: `modules/world/player.go:387-391` (encodeOut diff)
- Modify: `modules/world/player_interface.go:15-22` (IfSetText)
- Modify: `pkg/script/handlers_inv.go:290-...` (handleInvAdd)

**Goal:** add `slog.Info("NAI-112 instr: ...")` log lines at each discriminator point so a single user-launched smoke produces a complete event trace covering all three sub-hypotheses. Mirror the style of commit `f76c2da`.

- [ ] **Step 1.1: Add TUT_CLICKSIDE entry log line that captures `%tutorial` value**

The Bundle 1b instrumentation only logged `tab` and `scriptFound`. For Stage 2.1 we need the varp value at click time to discriminate H6.a. Edit `modules/world/handler_interface.go:138-149`:

```go
import (
	"log/slog"

	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/script"
)
```

(re-add `log/slog` import — removed by `41748a0` revert.)

Replace the body of `handleTutClickSide`:

```go
func (s *Server) handleTutClickSide(p *Player, payload []byte) error {
	if len(payload) < 1 {
		return nil
	}
	tab := int(payload[0])
	// NAI-112 Stage 2.1 instr: capture %tutorial varp at click time. The
	// varp id for "tutorial" is resolved by name via the loaded varp config
	// table; if absent, log -1 to surface a config-load divergence.
	tutorialVarpID := -1
	if s.varpTypes != nil {
		for i, cfg := range s.varpTypes.Configs {
			if cfg != nil && cfg.Debugname == "tutorial" {
				tutorialVarpID = i
				break
			}
		}
	}
	tutorialVal := int32(-999)
	if tutorialVarpID >= 0 {
		tutorialVal = p.Varp(tutorialVarpID)
	}
	slog.Info("NAI-112 Stage2.1 instr: TUT_CLICKSIDE entry",
		"tab", tab,
		"tutorialVarpID", tutorialVarpID,
		"tutorialVal", tutorialVal)
	if tab < 0 || tab > 13 {
		return nil
	}
	sf := s.scriptProvider.GetByTriggerSpecific(script.TriggerTutorial, -1, -1)
	slog.Info("NAI-112 Stage2.1 instr: TUT_CLICKSIDE lookup", "tab", tab, "scriptFound", sf != nil)
	s.runScript(sf, p, nil, true, nil, nil)
	slog.Info("NAI-112 Stage2.1 instr: TUT_CLICKSIDE postScript",
		"tab", tab,
		"tutorialValAfter", func() int32 {
			if tutorialVarpID < 0 {
				return -999
			}
			return p.Varp(tutorialVarpID)
		}())
	return nil
}
```

> **Note on config field name:** verify `objtype.VarPlayerType.Debugname` exists at HEAD before dispatch. If the field is `Name` or `DebugName` instead, adapt. Run `rg "type VarPlayerType" pkg/objtype/` to confirm.

- [ ] **Step 1.2: Run handler test to verify still compiles + tests pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestHandleTutClickSide -v -count=1`

Expected: PASS (existing tests at `modules/world/handler_interface_test.go:71-128` continue to pass; instrumentation is additive). If the varp-config lookup field name is wrong, this surfaces as a compile error.

- [ ] **Step 1.3: Instrument `Player.SetVarp` to log every varp write**

Edit `modules/world/player_script.go:317-323`:

```go
func (p *Player) SetVarp(id int, val int32) {
	if id < 0 || id >= len(p.varps) {
		return
	}
	prev := p.varps[id]
	p.varps[id] = val
	// NAI-112 Stage 2.1 instr: log every varp write. High-volume but
	// scoped to a single user-launched smoke run; reverted in Bundle 3.
	slog.Info("NAI-112 Stage2.1 instr: SetVarp", "id", id, "prev", prev, "val", val)
	p.writeVarp(id, val)
}
```

Add the `log/slog` import to `modules/world/player_script.go` if missing.

- [ ] **Step 1.4: Instrument `Player.OpenTutorial` and `Player.CloseTutorial`**

Edit `modules/world/player_script.go:788-791`:

```go
func (p *Player) OpenTutorial(com int) {
	prev := p.modalTutorial
	p.modalTutorial = com
	p.modalState |= modalStateTut
	slog.Info("NAI-112 Stage2.1 instr: OpenTutorial",
		"com", com,
		"prevModalTutorial", prev,
		"lastModalTutorial", p.lastModalTutorial,
		"willEmitOnEncodeOut", com != p.lastModalTutorial)
}
```

Edit `CloseTutorial` (player_script.go:808-817) to log entry/exit:

```go
func (p *Player) CloseTutorial() {
	if p.modalTutorial == -1 {
		slog.Info("NAI-112 Stage2.1 instr: CloseTutorial noop (already closed)")
		return
	}
	slog.Info("NAI-112 Stage2.1 instr: CloseTutorial enter", "modalTutorial", p.modalTutorial)
	if p.client != nil && p.client.server != nil {
		p.runIfCloseTrigger(p.client.server, p.modalTutorial)
	}
	p.modalTutorial = -1
	p.modalState &^= modalStateTut
}
```

- [ ] **Step 1.5: Instrument the encodeOut TutOpen diff at `modules/world/player.go:387-391`**

Replace:

```go
if p.modalTutorial != p.lastModalTutorial {
	payload := []byte{byte(p.modalTutorial >> 8), byte(p.modalTutorial)}
	p.writeOut(gameserver.OpTutOpen, payload)
	p.lastModalTutorial = p.modalTutorial
}
```

With:

```go
if p.modalTutorial != p.lastModalTutorial {
	payload := []byte{byte(p.modalTutorial >> 8), byte(p.modalTutorial)}
	p.writeOut(gameserver.OpTutOpen, payload)
	slog.Info("NAI-112 Stage2.1 instr: encodeOut TutOpen wire emit",
		"modalTutorial", p.modalTutorial,
		"prevLast", p.lastModalTutorial)
	p.lastModalTutorial = p.modalTutorial
} else if p.modalTutorial != -1 {
	// Diff suppress: TS Player.openTutorial writes unconditionally
	// (Engine-TS Player.ts:1999-2003); goscape's diff may be the H6.c
	// divergence. Log every suppress event so smoke can count them.
	slog.Info("NAI-112 Stage2.1 instr: encodeOut TutOpen diff-suppressed",
		"modalTutorial", p.modalTutorial,
		"lastModalTutorial", p.lastModalTutorial)
}
```

Add `log/slog` import to `modules/world/player.go` if missing.

- [ ] **Step 1.6: Instrument `Player.IfSetText`**

Edit `modules/world/player_interface.go:15-22`. Truncate text to 80 chars in the log to keep volume manageable:

```go
func (p *Player) IfSetText(com int, text string) {
	logText := text
	if len(logText) > 80 {
		logText = logText[:80] + "…"
	}
	slog.Info("NAI-112 Stage2.1 instr: IfSetText", "com", com, "text", logText)
	buf := packet.NewPacket(nil)
	buf.PSmart(uint16(com))
	buf.PJStrLF(text)
	p.writeOut(gameserver.OpIfSetText, buf.Bytes())
}
```

> Verify the existing IfSetText body (encoder calls) at HEAD before edit — the snippet above mirrors the doc comment but the actual encoder calls may differ. Use the Read tool first; preserve the existing encoder logic verbatim and only add the slog call before the writeOut.

- [ ] **Step 1.7: Instrument `handleInvAdd`**

Edit `pkg/script/handlers_inv.go:294-…`. Add a log line that captures inv id, obj, count, and the active player's ref so smoke can confirm the bronze_axe + tinderbox writes fire:

```go
func handleInvAdd(s *ScriptState) error {
	// ... preserve all existing pop/check logic verbatim ...
	// After all pops + before/around the actual inv mutation, add:
	slog.Info("NAI-112 Stage2.1 instr: INV_ADD",
		"inv", invID,
		"obj", objID,
		"count", count,
		"hasActivePlayer", s.Self != nil)
	// ... then preserve the existing mutation + return ...
}
```

> Read `pkg/script/handlers_inv.go:290-410` first; the variable names (`invID`, `objID`, `count`) above are placeholders — match what the existing handler uses. Do NOT alter the pop order or any check logic.

Add `log/slog` import to `pkg/script/handlers_inv.go` if missing.

- [ ] **Step 1.8: Run all goscape tests to confirm instrumentation is additive**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`

Expected: ALL PASS. Instrumentation is read-only (slog calls); no behavior changes.

If any test fails: instrumentation accidentally altered code paths. Revert and re-add minimally.

- [ ] **Step 1.9: Commit Bundle 1 instrumentation**

```bash
git add modules/world/handler_interface.go modules/world/player_script.go modules/world/player.go modules/world/player_interface.go pkg/script/handlers_inv.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
chore(instr): NAI-112 Stage 2.1 — [tutorial,_] dispatch chain logs

Temporary instrumentation for H6 sub-hypothesis discrimination.
Logs %tutorial varp at TUT_CLICKSIDE entry + post-script; every
SetVarp write; OpenTutorial calls + emit/suppress decisions in
encodeOut; IfSetText calls (text truncated to 80c); handleInvAdd
calls. Reverted in Bundle 3 after user-launched smoke produces logs.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2 — User-launched smoke handoff (no code changes)

**Files:** none.

- [ ] **Step 2.1: Emit smoke handoff prompt to user**

Per `smoke_test_server_handoff`, the server must be user-launched. Emit this prompt (paste-ready):

> NAI-112 Stage 2.1 instrumentation is committed at HEAD. Please:
>
> 1. Build and run goscape: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache CGO_ENABLED=0 go run -trimpath ./cmd/goscape --config.file config.yaml`
> 2. Connect with Java client rev-225; log in as fresh account (LOGIN_RESULT_NEW_PLAYER).
> 3. Walk through Survival Expert dialog (RESUME_PAUSEBUTTON repeated) until the chatbox shows "Click on the flashing backpack icon to the …".
> 4. Click the inventory tab.
> 5. Observe whether chatbox advances to "Cut down a tree" AND whether inventory side panel displays.
> 6. Save server stdout/stderr from the moment of the click (about 5s window) and paste into the conversation. Include any `NAI-112 Stage2.1 instr` lines.

- [ ] **Step 2.2: Wait for user to paste the smoke logs back**

User pastes logs. Controller (next task) does NOT proceed without log evidence.

- [ ] **Step 2.3: Append the smoke trace to the investigation doc**

Append a new "Stage 2.1 — runtime evidence (smoke YYYY-MM-DD)" section to `docs/superpowers/investigations/2026-05-06-nai-112-stage1-audit.md` with the verbatim log excerpt and a one-paragraph reading.

```bash
git add docs/superpowers/investigations/2026-05-06-nai-112-stage1-audit.md
git commit --no-gpg-sign -m "$(cat <<'EOF'
docs(investigation): NAI-112 Stage 2.1 — runtime evidence appended

Smoke YYYY-MM-DD against instrumented HEAD. Captured: TUT_CLICKSIDE
entry %tutorial value; every SetVarp/IfSetText/OpenTutorial/InvAdd
call during the click event; encodeOut TutOpen emit/suppress
counters. Bundle 2 binding decision in next commit.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3 — Triangulation: bind H6.a, H6.b, or H6.c

**Files:**
- Modify: `docs/superpowers/investigations/2026-05-06-nai-112-stage1-audit.md` (append "Stage 2.1 binding" section)

**Goal:** read the smoke logs against the discriminating-signal table in the plan header and bind exactly one sub-hypothesis. Document the binding rationale.

- [ ] **Step 3.1: Apply the discrimination matrix**

Walk the smoke log against the table at the top of this plan:

- **If `tutorialVal != 20` at TUT_CLICKSIDE entry** → bind **H6.a**. Most-likely root cause: `%tutorial` was never seeded by `[label,start_tutorial]` (the label-script invocation may not fire on fresh-account login), or the survival-guide dialog didn't actually run `%tutorial = ^newbie_survival_instructor_open_inventory`.
- **If `tutorialVal == 20` AND no `SetVarp tutorial=30` log fires post-script-entry** → bind **H6.a** (sub-shape: POP_VARP failed silently). Surface: `handlePopVarp` at `pkg/script/handlers_vars.go:21-28`.
- **If `SetVarp tutorial=30` fires AND `INV_ADD` logs for bronze_axe (id depends on obj table) + tinderbox AND `IfSetText` logs for "Cut down a tree" title + body lines AND `OpenTutorial` is called twice (once for view_inventory, once for cut_tree) BUT the second call logs `willEmitOnEncodeOut=false` AND `encodeOut TutOpen diff-suppressed` fires** → bind **H6.c**.
- **If `SetVarp tutorial=30` fires AND `IfSetText` logs for "Cut down a tree" fire AND `OpenTutorial(tutorial_text)` second call logs `willEmitOnEncodeOut=true` AND `encodeOut TutOpen wire emit` fires** → smoke contradicts both H6.b and H6.c on the chatbox-side; chatbox-advance regression escalates to a NEW H7 (client-side rendering bug, possibly client_chat_suppression-shape; needs Java-client-side investigation, out of NAI-112 scope; close NAI-112 as investigation-only).
- **If any of `INV_ADD bronze_axe`, `INV_ADD tinderbox`, or any expected `IfSetText` line is missing from the trace** → bind **H6.b** to the missing-effect opcode.

**Note on `inventory side panel did NOT display` symptom:** the `if_settab(inventory, ^tab_inventory)` call from `tutorial_step_view_inventory` (tut_chatbox_steps.rs2:30) ran during the survival_guide step. After click, `tutorial_step_cut_tree` does NOT re-call `if_settab`. So the panel-not-displaying is either the SAME divergence as the chatbox issue (e.g., tab+overlay state both depend on TUT_OPEN re-emission) OR a separate divergence in `if_settab`/`Player.IfSetTab`. Stage 2.1 instrumentation does NOT cover `Player.IfSetTab`; if smoke logs show all expected events fire and the panel still doesn't render, ADD a Bundle 1.5 round of instrumentation on `IfSetTab` — do NOT bind blind.

- [ ] **Step 3.2: Append binding section to investigation doc**

Append to `docs/superpowers/investigations/2026-05-06-nai-112-stage1-audit.md`:

```markdown
## Stage 2.1 binding (YYYY-MM-DD)

**Bound: H6.X** — <one-sentence rationale referencing log lines>.

Discriminating signals from smoke trace:
- TUT_CLICKSIDE entry: tutorialVal=<value>
- Branch: <fired branch identified by SetVarp tutorial=<new value> log>
- Effects observed: <list of IfSetText/InvAdd/OpenTutorial events>
- TutOpen wire emits: <count>; diff-suppresses: <count>

Stage 2.2 fix routes to: <Task 4a / 4b / 4c per binding>.
```

- [ ] **Step 3.3: Commit triangulation**

```bash
git add docs/superpowers/investigations/2026-05-06-nai-112-stage1-audit.md
git commit --no-gpg-sign -m "$(cat <<'EOF'
docs(investigation): NAI-112 Stage 2.1 binding — H6.X

Smoke trace settled the H6 sub-hypothesis to H6.X.
Bundle 2 fix shape per Task 4X of plan.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 4 — Stage 2.2 fix per binding (one of 4a / 4b / 4c)

**Pre-flight gate:** controller re-greps and re-Reads every cited file:line in the bound sub-task before dispatching the implementer subagent (per `controller_preflight`).

> **Implementer dispatch rule:** ONE of Task 4a, 4b, 4c executes; the other two are skipped. Controller picks per Task 3 binding.

### Task 4a — Fix H6.a (varp init/seeding divergence)

**Files (likely; controller refines after binding):**
- Modify: `modules/world/tick.go:104-106` (varps init) — only if init shape diverges
- Modify: the `[label,start_tutorial]` invocation path — typically a missing dispatch in the login flow that should fire when `%tutorial < ^tutorial_complete && in_tutorial_island(coord)`. Trace from `modules/world/tick.go:140-142` (login script invocation) — `[login,_]` content-side calls `@start_tutorial`; if `[label,…]` dispatch is broken in goscape, ports start there.
- Modify: `pkg/script/handlers_vars.go:21-28` (`handlePopVarp`) — only if smoke shows POP_VARP silently fails

**Fix template (varp seeding shape — most likely):**

- [ ] **Step 4a.1: Read `[label,start_tutorial]` dispatch path in goscape**

The TS engine resolves `@start_tutorial` to a `[label,…]` script via the `JUMP_WITH_PARAMS` opcode. Verify goscape's `handleJumpWithParams` at `pkg/script/handlers.go:324`-area correctly resolves a `[label,…]` script when invoked from within `[login,_]`. Run:

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestJumpWithParams -v
```

If TestJumpWithParams doesn't exist or doesn't cover the label-resolution case, this is the binding's surface.

- [ ] **Step 4a.2: Write the failing test**

Test surface depends on the smoke binding's specific signal. If `tutorialVal=0` at click time despite a fresh-login smoke walking through Survival Expert, the test asserts that after `[login,_]` execution against a fresh-coord-on-Tutorial-Island player, `%tutorial` is non-zero (or that `[label,start_tutorial]` was reached).

```go
// pkg/script/handlers_test.go OR modules/world/script_test.go (location TBD by binding)
func TestNAI_112_StartTutorialLabelDispatchedFromLogin(t *testing.T) {
	// setup: provider with [login,_] (calls @start_tutorial) and
	// [label,start_tutorial] (sets %tutorial = X for assertion).
	// Player at Tutorial-Island coord, fresh state (varps zeroed).
	// run [login,_].
	// assert: p.Varp(tutorialVarpID) == X.
}
```

- [ ] **Step 4a.3: Run to verify test fails**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./<package> -run TestNAI_112_StartTutorialLabelDispatchedFromLogin -v -count=1
```

Expected: FAIL.

- [ ] **Step 4a.4: Write the minimal fix**

Per binding rationale. ≤20 LOC. Track any TS-divergence (e.g., goscape-only defensive check) per `defensive_gate_doc_comment_label` and `true_to_ts_gate`.

- [ ] **Step 4a.5: Run to verify test passes**

Same command as 4a.3. Expected: PASS.

- [ ] **Step 4a.6: Run full test suite**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
```

Expected: PASS.

- [ ] **Step 4a.7: Commit**

```bash
git add <files>
git commit --no-gpg-sign -m "$(cat <<'EOF'
fix(script): NAI-112 Stage 2.2 — H6.a <one-line summary>

<2-3 sentence why-not-what rationale>. Mirrors TS <ref>.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

### Task 4b — Fix H6.b (per-opcode handler divergence)

**Files (controller refines after binding):**
- Modify: the diverging handler in `pkg/script/handlers_*.go` (likely `handlers_inv.go`, `handlers_interface.go`, or `handlers_dialog.go`)
- Modify: the corresponding `Player.*` shim in `modules/world/player_*.go`
- Test: `pkg/script/handlers_*_test.go` (red test → green fix)

**Fix template (single-opcode shape):**

- [ ] **Step 4b.1: Identify the diverging opcode from Task 3 binding**

The smoke trace identified one of: `IfSetText` (text not propagating), `InvAdd` (silent no-op), `Hint*`, `IfSetTab`, etc. Read the corresponding TS handler in `Engine-TS/src/lostcity/engine/script/handlers/PlayerOps.ts` or `InvOps.ts` etc. Note pop-order, check-list, and downstream effect.

- [ ] **Step 4b.2: Write the failing test**

```go
// pkg/script/handlers_<area>_test.go
func TestNAI_112_<OpcodeName>_<DivergingBehavior>(t *testing.T) {
	// setup ScriptState with required Pointers + Self mock + push args.
	// invoke handler.
	// assert: <bound divergence pinned by literal expected output>.
}
```

> Per `scriptstate_test_fixture_idioms`: `&ScriptState{}` needs `StackCapacity` init + correct push-order + `Pointers` flag.

- [ ] **Step 4b.3: Run to verify test fails**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestNAI_112_<OpcodeName>_<DivergingBehavior> -v -count=1
```

Expected: FAIL with a specific divergence (mock state mutation count wrong, or wire payload byte-mismatch).

- [ ] **Step 4b.4: Write the minimal fix**

Per binding rationale. Pop-order changes, missing `check…`, missing pointer-bit gate, etc. Track any TS divergence per `true_to_ts_gate`.

- [ ] **Step 4b.5: Run to verify test passes**

Same command as 4b.3. Expected: PASS.

- [ ] **Step 4b.6: Run full test suite**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
```

Expected: PASS. If a downstream test in `modules/world/` fails because it asserted the buggy behavior, evaluate per `latent_bug_at_migration_boundary`.

- [ ] **Step 4b.7: Commit**

```bash
git add <files>
git commit --no-gpg-sign -m "$(cat <<'EOF'
fix(script): NAI-112 Stage 2.2 — H6.b <opcode> <divergence summary>

<rationale>. Mirrors TS <ref>.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

### Task 4c — Fix H6.c (TutOpen wire-emit divergence)

**Files:**
- Modify: `modules/world/player_script.go:782-791` (OpenTutorial — write wire packet on call, not via deferred diff)
- Modify: `modules/world/player.go:387-391` (encodeOut TutOpen diff — retire OR repurpose for close-only)
- Modify: `modules/world/player.go:157`-area (`lastModalTutorial` field — retain for close-detection only OR retire entirely)
- Test: `modules/world/player_test.go:766-…` (`TestEncodeOutSendsTutOpen`) and `:944-…` (TUT_CLOSE-shape tests) — adapt to new behavior
- Test: `modules/world/player_test.go` — add new test for unconditional re-emission on duplicate `OpenTutorial(com)`

**Fix shape (TS-faithful unconditional write):**

Per Engine-TS Player.ts:1999-2003, `openTutorial(com)` writes `new TutOpen(com)` UNCONDITIONALLY. Goscape's diff at `player.go:387-391` is the H6.c divergence. Fix shape:

1. Move the wire emit out of `encodeOut` into `OpenTutorial(com)` itself.
2. Retain `CloseTutorial`'s wire emit pathway via the SAME unconditional write (TutOpen(-1)).
3. Either retire `lastModalTutorial` entirely (no consumer post-fix) OR retain only as a `previousModalTutorial` field for the IF_CLOSE trigger lookup in `CloseTutorial`.

- [ ] **Step 4c.1: Read existing tests at HEAD**

Run: `Read modules/world/player_test.go:760-995` (covers `TestEncodeOutSendsTutOpen` + duplicate-suppression tests). Note which tests assert the diff behavior (these will need to flip to assert unconditional emission).

- [ ] **Step 4c.2: Write the failing test**

```go
// modules/world/player_test.go
func TestNAI_112_OpenTutorialUnconditionalReEmit(t *testing.T) {
	// Per Engine-TS Player.ts:1999-2003, openTutorial(com) writes
	// TutOpen(com) UNCONDITIONALLY — even when com == previous com.
	// goscape pre-NAI-112 diffed against lastModalTutorial and
	// silently suppressed the second emit; this regressed
	// `tut_open(tutorial_text)` after `if_settext` updates because
	// the client requires a re-open to flush the overlay redraw.
	srv, p, _ := newTestPlayer(t)
	_ = srv

	// First open: emits.
	p.OpenTutorial(42)
	out1, err := drainEncodeOut(p)
	if err != nil { t.Fatal(err) }
	if !containsTutOpen(out1, 42) {
		t.Fatalf("first OpenTutorial(42) did not emit TutOpen wire packet")
	}

	// Second open with SAME com: must STILL emit (not diff-suppress).
	p.OpenTutorial(42)
	out2, err := drainEncodeOut(p)
	if err != nil { t.Fatal(err) }
	if !containsTutOpen(out2, 42) {
		t.Fatalf("duplicate OpenTutorial(42) was diff-suppressed (H6.c divergence) — expected unconditional re-emit per TS Player.ts:1999-2003")
	}
}
```

> `drainEncodeOut` and `containsTutOpen` helpers: if not present at HEAD, define minimally in this test file (`drainEncodeOut` runs encodeOut and returns the bufw byte slice; `containsTutOpen` scans for ISAAC-encrypted opcode = `(OpTutOpen.Opcode + nextEncryptor) & 0xff` followed by 2-byte payload `[com>>8, com&0xff]`). Pattern is established by the existing `TestEncodeOutSendsTutOpen` at line 766 — mirror it.

- [ ] **Step 4c.3: Run to verify test fails**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestNAI_112_OpenTutorialUnconditionalReEmit -v -count=1
```

Expected: FAIL on the second-emit assertion (the diff suppresses).

- [ ] **Step 4c.4: Write the fix — move wire emit into OpenTutorial**

Edit `modules/world/player_script.go:788-791` (post-instrumentation: this is what the Bundle 1 instrumentation modified):

```go
// OpenTutorial sets the player's tutorial-overlay component and writes
// the matching TUT_OPEN wire packet UNCONDITIONALLY. Mirrors TS
// Player.openTutorial at Engine-TS/src/engine/entity/Player.ts:1999-2003,
// which writes `new TutOpen(com)` on every call regardless of prior
// state. NAI-112 Stage 2.2 retired the goscape diff at
// modules/world/player.go (pre-NAI-112: emit-only-on-change) — that
// suppressed the re-open the client needs to flush IF_SETTEXT updates
// when the same tutorial component is re-opened (H6.c divergence
// surfaced at NAI-110 close smoke; bound NAI-112 Stage 2.1).
func (p *Player) OpenTutorial(com int) {
	p.modalTutorial = com
	p.modalState |= modalStateTut
	payload := []byte{byte(com >> 8), byte(com)}
	p.writeOut(gameserver.OpTutOpen, payload)
}
```

Edit `modules/world/player.go:387-391` to retire the diff. Determine the new behavior for `CloseTutorial`: `CloseTutorial` writes `TutOpen(-1)`. The cleanest port mirrors TS — make `CloseTutorial` also write the wire packet directly:

Edit `CloseTutorial` at `modules/world/player_script.go:808-817`:

```go
func (p *Player) CloseTutorial() {
	if p.modalTutorial == -1 {
		return
	}
	if p.client != nil && p.client.server != nil {
		p.runIfCloseTrigger(p.client.server, p.modalTutorial)
	}
	p.modalTutorial = -1
	p.modalState &^= modalStateTut
	// TS Player.closeTutorial at Engine-TS/src/engine/entity/Player.ts:716-726
	// writes `new TutOpen(-1)` directly. NAI-76 pin previously routed this
	// through encodeOut diff; NAI-112 Stage 2.2 inlines the write to
	// match TS unconditional-emit semantics.
	payload := []byte{0xff, 0xff} // -1 as i16 BE
	p.writeOut(gameserver.OpTutOpen, payload)
}
```

Then retire the diff at `modules/world/player.go:387-391`:

```go
// (the entire `if p.modalTutorial != p.lastModalTutorial { … }` block
// is deleted; encodeOut no longer manages TUT_OPEN — OpenTutorial /
// CloseTutorial write directly per TS Player.ts:1999-2003 + 716-726.
// NAI-112 Stage 2.2 / H6.c.)
```

If `lastModalTutorial` has no other consumer, retire the field at `modules/world/player.go:157`-area as well. Run `rg "lastModalTutorial" modules/world/` to confirm consumer set; retire only if no production reader remains.

- [ ] **Step 4c.5: Update existing tests that asserted diff-suppression**

`modules/world/player_test.go:766+` (`TestEncodeOutSendsTutOpen`) and `:944+` (TUT_CLOSE-shape tests) likely assert the diff behavior. Read those tests; flip the expectations to assert unconditional emission. If a test was specifically pinning the diff-suppression as intentional, retire it (track removal in commit body per `true_to_ts_gate`).

- [ ] **Step 4c.6: Run the new test**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestNAI_112_OpenTutorialUnconditionalReEmit -v -count=1
```

Expected: PASS.

- [ ] **Step 4c.7: Run full test suite**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
```

Expected: ALL PASS. Pay attention to any test in `modules/world/` that previously asserted the diff (Step 4c.5 covers the most-likely surface).

- [ ] **Step 4c.8: Commit**

```bash
git add modules/world/player_script.go modules/world/player.go modules/world/player_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
fix(world): NAI-112 Stage 2.2 — H6.c TUT_OPEN unconditional re-emit

TS Player.openTutorial / closeTutorial (Engine-TS Player.ts:1999-2003,
716-726) write TUT_OPEN UNCONDITIONALLY on every call. Goscape pre-fix
diffed modalTutorial != lastModalTutorial in encodeOut and silently
suppressed the second OpenTutorial(com) call when com matched the
prior state. The Java client requires a re-open to flush the overlay
redraw after IF_SETTEXT updates, so the diff regressed
~tutorialstep's second tut_open(tutorial_text) call (e.g.,
tutorial_step_view_inventory → tutorial_step_cut_tree on
inventory-tab-click). Surfaced at NAI-110 close smoke; bound NAI-112
Stage 2.1 instrumentation 2026-05-06.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 5 — Bundle 3: Revert instrumentation

**Files (mirror Task 1 surface):**
- Modify: `modules/world/handler_interface.go` (revert Step 1.1)
- Modify: `modules/world/player_script.go` (revert Steps 1.3, 1.4)
- Modify: `modules/world/player.go` (revert Step 1.5)
- Modify: `modules/world/player_interface.go` (revert Step 1.6)
- Modify: `pkg/script/handlers_inv.go` (revert Step 1.7)

> If Task 4 was 4c, the OpenTutorial / encodeOut edits are intertwined with the Bundle 1 instrumentation. Revert ONLY the `slog.Info("NAI-112 Stage2.1 instr: ...")` calls; preserve the Stage 2.2 fix.

- [ ] **Step 5.1: Remove instrumentation from each file**

For each file, remove every `slog.Info("NAI-112 Stage2.1 instr: ...")` line. Remove the `log/slog` import if no other consumer. Keep all behavior changes from Task 4 intact.

- [ ] **Step 5.2: Run full test suite**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
```

Expected: ALL PASS. The Stage 2.2 fix from Task 4 remains green; instrumentation removal is a no-op on observable behavior.

- [ ] **Step 5.3: Verify revert is clean**

```bash
git diff <Task1.commit-sha>..HEAD -- modules/world/handler_interface.go modules/world/player_interface.go pkg/script/handlers_inv.go
```

Expected: empty diff for files that were ONLY instrumented (not edited by Task 4). For files edited by Task 4 (potentially `player_script.go` and `player.go`), expect ONLY the Task-4 fix remains.

- [ ] **Step 5.4: Commit revert**

```bash
git add modules/world/handler_interface.go modules/world/player_script.go modules/world/player.go modules/world/player_interface.go pkg/script/handlers_inv.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
chore(instr): NAI-112 Stage 2 — revert Stage 2.1 instrumentation

Stage 2.1 instrumentation served its triangulation purpose at
<smoke date>; H6.X bound and fixed at <Task 4 commit>. Removing
slog.Info lines and unused log/slog imports.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 6 — User-launched final smoke (Stage 3)

**Files:** none.

- [ ] **Step 6.1: Emit final smoke handoff prompt**

Per `smoke_test_server_handoff` and spec §6:

> NAI-112 Stage 2.2 fix is committed. Please:
>
> 1. Build and run goscape.
> 2. Connect with Java client rev-225; log in as fresh account.
> 3. Walk through Survival Expert dialog to "Click on the flashing backpack icon to the …".
> 4. Click the inventory tab.
> 5. Confirm: chatbox advances to "Cut down a tree" AND inventory side panel displays the bronze axe + tinderbox.
> 6. Report pass/fail + any new symptoms.

- [ ] **Step 6.2: Wait for user smoke result**

User reports.

**Pass routing:** proceed to Task 7 (close commit).

**Fail routing:**
- Symptom unchanged → per `smoke_unchanged_means_multiple_blockers`, brainstorm under-diagnosed; re-open Stage 2.1 with smoke-bound evidence.
- New symptom (different blocker) → per `cascade_theory_smoke_binding`, route per spec §6 — adjacent ≤30 LOC stretch in-scope; else NAI-113.

---

## Task 7 — Close commit + memory hygiene

**Files:**
- Modify: `/home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md` (add NAI-112 close entry with cascade attribution)
- Modify: `/home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/MEMORY.md` if a new memory is added (≥1 line, <200 chars)

**Goal:** finalize NAI-112 with a `Closes memory:` trailer per `close_commit_memory_trailer` (apply NAI-15 onward).

- [ ] **Step 7.1: Append nai_followups.md close entry**

Append to `nai_followups.md`:

```markdown
- **NAI-112** (closed YYYY-MM-DD by NAI-112 Stage 2.2): tutorial-tab-click chatbox-advance silent non-advance.
  Stage 1 audit + Bundle 1b runtime smoke refuted H1-H5; Stage 2.1 instrumentation bound H6.X.
  Fix: <one-line summary>. Smoke YYYY-MM-DD confirms pass.
  Residuals: <list any out-of-scope adjacents that route to NAI-113 / future, OR "none">.
```

- [ ] **Step 7.2: (Optional) Add a non-derivable memory entry**

Per `post_task_handoff`: if Stage 2.1 surfaced a non-derivable lesson (e.g., "TS Player.openTutorial writes unconditionally; never diff modal-state wire emits in goscape" if H6.c was bound), save it as a `feedback` memory file with index entry under 200 chars. Skip if the close is purely a varp-init or per-opcode bug fix that future-NAI grep would find anyway.

- [ ] **Step 7.3: Compose final close commit**

```bash
git add <memory files only — code changes are already committed in Tasks 4 + 5>
git commit --no-gpg-sign -m "$(cat <<'EOF'
chore(close): NAI-112 — H6.X tutorial-tab-click <one-line>

<3-4 sentence why-not-what summary covering: Stage 1 H1-H5 refutation,
Stage 2.1 H6 sub-binding, Stage 2.2 fix shape, smoke confirmation.>

Closes NAI-112.

Closes memory: <relative-path-of-new-memory-file-from-MEMORY.md, OR "none">

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Risk register

- **R1 — Smoke contradicts all three sub-hypotheses.** Trace shows correct branch fires + correct wire emits + chatbox still doesn't advance. Mitigation: bind to H7 (client-side rendering bug); close NAI-112 as investigation-only; route Java-client investigation to NAI-113.
- **R2 — Bundle 1 instrumentation alters behavior.** `slog.Info` should be side-effect-free, but a careless capture of mutated state in a closure (Step 1.1 `tutorialValAfter`) could skew. Mitigation: Step 1.8 full test suite + post-build sanity smoke before user-launched smoke.
- **R3 — Task 4c retires `lastModalTutorial` but leaves a consumer.** Mitigation: `rg "lastModalTutorial" modules/world/` before retiring the field; if any reader remains, scope down to "stop writing in encodeOut" without retiring the field.
- **R4 — Task 4c breaks an unrelated test that pinned the diff-suppression as desired (e.g., NAI-76 pin).** Mitigation: read NAI-76 history (`git log --grep "NAI-76"`) before flipping the test expectation; if the pin was specifically about close-emission, preserve close-emission semantics.
- **R5 — `inv_add` count not surfacing in Step 1.7 instrumentation because `handleInvAdd`'s var names diverge from the placeholder.** Mitigation: Step 1.7 explicitly prescribes Read-first; do not paste literally.
- **R6 — Inventory-side-panel symptom is a separate divergence from chatbox-advance.** Mitigation: §"Note on `inventory side panel did NOT display`" in Task 3 explicitly handles via Bundle 1.5 if needed.
- **R7 — Stage 2.2 LOC creeps past 80 due to multi-opcode H6.b.** Mitigation: spec §5 guardrail; pause and confirm with user before auto-expanding.

---

## Cadence summary

- **Bundle 1** (Task 1) — instrument
- **Smoke** (Task 2) — user-launched, observation-only
- **Bind** (Task 3) — controller picks H6.a/b/c
- **Bundle 2** (Task 4) — fix per binding (one of 4a/4b/4c)
- **Bundle 3** (Task 5) — revert instrumentation
- **Smoke** (Task 6) — user-launched, final
- **Close** (Task 7) — `Closes memory:` trailer

Per `superpowers_clear_between_spec_and_impl`: emit a paste-ready resume prompt and stop after this plan is saved; user `/clear`s before implementer dispatch.
