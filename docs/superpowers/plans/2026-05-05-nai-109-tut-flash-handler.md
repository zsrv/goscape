# NAI-109 — TUT_FLASH script-opcode handler + wire packet — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Wire goscape's `OpTutFlash` (script opcode 2121) and the matching `ServerGameProt.TUT_FLASH = (126, 1)` server packet so Tutorial Island's `tut_flash(tab)` calls dispatch instead of erroring at runtime.

**Architecture:** Pure additive port. Mirror the existing TUT_OPEN/TUT_CLOSE pattern at `pkg/script/handlers_interface.go:91-111`, except the wire write is direct (one-shot UI hint) instead of deferred (modal-state diff in flushModalState). Three TUT_* handler siblings end up adjacent in the dispatch map. Extend the `ActivePlayer` interface with `FlashTutorial(int)`; implement on both `*Player` (production, calls `writeOut`) and `mockPlayer` (test, records last value + call count).

**Tech Stack:** Go 1.26+ (per `go_version` memory). Tests via `go test`. Always invoke as `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go ...` per project CLAUDE.md.

**Spec:** `docs/superpowers/specs/2026-05-05-nai-109-tut-flash-handler-design.md` (commit `3b069e2`).

**Cadence:** 3 tasks (T1 red → T2 green → T3 close) on Sonnet via subagent-driven-development per `execution_mode_default.md`. End-of-bundle review on Sonnet per `superpowers_code_reviewer_model.md`.

**Pre-flight verified at HEAD `3b069e2` (controller_preflight):**
- `OpTutFlash = 2121` declared at `pkg/script/opcode.go:221`. ✓
- `OpTutFlash` String() case at `pkg/script/opcode.go:849-850`. ✓
- Handler map at `pkg/script/handlers.go:297-298` registers TutOpen + TutClose only. ✓
- ActivePlayer interface decl at `pkg/script/active.go:6`; OpenTutorial at line 181, CloseTutorial at line 188. ✓
- mockPlayer struct at `pkg/script/runner_test.go:99`; `lastOpenTutorial int` at line 164, `lastCloseTutorialCalls int` at line 165. ✓
- mockPlayer methods at `pkg/script/runner_test.go:443-444`. ✓
- handleTutOpen at `pkg/script/handlers_interface.go:91-101`; handleTutClose at lines 105-111. ✓
- (*Player).OpenTutorial at `modules/world/player_script.go:788`; CloseTutorial at line 808. ✓
- TutOpen wire test pattern at `modules/world/player_test.go:766-803` (uses `newTestPlayer`, `isaacPair`, `clientConn`). ✓
- `(*Player).writeOut` at `modules/world/player.go:396-411` (no client-nil guard). ✓
- Direct-writer convention (no client-nil guard): `CamReset` at `player_script.go:189-191`, `HintNpc` at `player_script.go:201-209`, `WriteEnableTracking` at `player.go:416-418`. ✓
- Only two ActivePlayer impls: mockPlayer + *Player (per `enumerate_all_sites` grep). ✓
- TS handler at `Engine-TS PlayerOps.ts:694-696`. ✓
- TS encoder at `Engine-TS TutFlashEncoder.ts:9-11` (single `p1(message.tab)`). ✓
- TS protocol const `ServerGameProt.TUT_FLASH = (126, 1)` at `Engine-TS ServerGameProt.ts:24`. ✓

---

### Task 1: Red — interface, impl, mock, wire constant, 4 tests

**Files:**
- Modify: `pkg/io/protocol/game/server/prot.go` (add OpTutFlash constant)
- Modify: `pkg/script/active.go` (extend ActivePlayer interface)
- Modify: `pkg/script/runner_test.go` (mockPlayer field + method)
- Modify: `modules/world/player_script.go` (add `(*Player).FlashTutorial`)
- Test (add): `pkg/script/handlers_interface_test.go` (3 handler-level tests)
- Test (add): `modules/world/player_test.go` (1 wire-level test)

**Why this is "red":** The 3 handler-level tests dispatch `OpTutFlash`, which is in the opcode enum but not in the handler map at `pkg/script/handlers.go`. The runner returns `"no handler for TUT_FLASH (opcode 2121) at pc=…"`. The wire test calls `(*Player).FlashTutorial` directly and will pass on first run because the production impl is in this task — that's intentional (wire shape is byte-trivial; the wire test exists to pin the byte output, not drive TDD).

- [ ] **Step 1.1: Add OpTutFlash wire constant**

Modify `pkg/io/protocol/game/server/prot.go` — add a single-line declaration immediately after the existing `OpTutOpen` line at line 17:

```go
OpTutOpen        = Op{Opcode: 185, PayloadSize: 2}
OpTutFlash       = Op{Opcode: 126, PayloadSize: 1}
OpLogout         = Op{Opcode: 142, PayloadSize: 0}
```

Confirms TS `ServerGameProt.TUT_FLASH = (126, 1)` at `Engine-TS ServerGameProt.ts:24`. Single-byte payload (`p1`).

- [ ] **Step 1.2: Extend ActivePlayer interface**

Modify `pkg/script/active.go` — append after the `CloseTutorial()` declaration (currently at line 188):

```go
	// CloseTutorial closes any currently-open tutorial overlay. Per TS,
	// this is a no-op when no tutorial is open; otherwise it dispatches
	// the matching IF_CLOSE trigger script (if registered) and resets
	// the tutorial slot. Mirrors LostCityRS/Engine-TS Player.closeTutorial
	// (Player.ts:716-726).
	CloseTutorial()

	// FlashTutorial directs the client to flash the named tab to draw
	// the player's attention to it. Fire-and-forget: writes a single
	// TUT_FLASH server packet (opcode 126, 1-byte tab payload) and
	// returns. Mirrors LostCityRS/Engine-TS PlayerOps.ts:694-696 +
	// TutFlashEncoder.ts.
	FlashTutorial(tab int)
```

- [ ] **Step 1.3: Extend mockPlayer (struct + method)**

Modify `pkg/script/runner_test.go`:

(a) Add `lastFlashTutorial` and `lastFlashTutorialCalls` fields adjacent to `lastOpenTutorial` / `lastCloseTutorialCalls` (currently at lines 164-165). Result:

```go
	lastOpenTutorial    int
	lastCloseTutorialCalls int
	lastFlashTutorial      int
	lastFlashTutorialCalls int
```

(b) Add the `FlashTutorial` method immediately after the existing `OpenTutorial` / `CloseTutorial` methods (currently at lines 443-444):

```go
func (m *mockPlayer) OpenTutorial(com int) { m.lastOpenTutorial = com }
func (m *mockPlayer) CloseTutorial()       { m.lastCloseTutorialCalls++ }
func (m *mockPlayer) FlashTutorial(tab int) {
	m.lastFlashTutorial = tab
	m.lastFlashTutorialCalls++
}
```

- [ ] **Step 1.4: Add (*Player).FlashTutorial implementation**

Modify `modules/world/player_script.go` — append immediately after `(*Player).CloseTutorial` (currently ends around line 820, locate the closing `}` of `func (p *Player) CloseTutorial() { ... }`):

```go
// FlashTutorial implements script.ActivePlayer.FlashTutorial. Writes
// a TUT_FLASH server packet (opcode 126, 1-byte tab payload). Direct
// write — TUT_FLASH is fire-and-forget UI hint, not a modal-state
// transition like TUT_OPEN, so no deferred-flush pathway. Mirrors
// LostCityRS/Engine-TS Player.write(new TutFlash(tab)) call from
// PlayerOps.ts:694-696 + TutFlashEncoder.ts:9-11.
//
// No client-nil guard — matches goscape's direct-writer convention
// (CamReset at line 189-191, HintNpc at line 201-209, WriteEnableTracking
// at player.go:416-418); writeOut itself does not nil-guard either.
func (p *Player) FlashTutorial(tab int) {
	p.writeOut(gameserver.OpTutFlash, []byte{byte(tab)})
}
```

Verify the existing import alias matches: `gameserver "github.com/zsrv/goscape/pkg/io/protocol/game/server"` — the file already uses this import for `OpCamReset`, `OpHintArrow`, etc. No new import needed.

- [ ] **Step 1.5: Add 3 handler-level tests**

Modify `pkg/script/handlers_interface_test.go` — append immediately after the existing `TestTutCloseNoActivePlayer` block (locate the closing `}` of that test). Add a section header comment and three tests mirroring the TutOpen pattern at lines 1129-1194:

```go
// -- NAI-109: TUT_FLASH tests ----------------------------------------------

// TestTutFlash pins TUT_FLASH script-opcode dispatch:
// state.popInt() → ActivePlayer.FlashTutorial(tab).
// Mirrors TS PlayerOps.ts:694-696.
func TestTutFlash(t *testing.T) {
	sf := &ScriptFile{
		Name:             "tut_flash",
		Opcodes:          []Opcode{OpPushConstantInt, OpTutFlash, OpReturn},
		IntOperands:      []int32{42, 0, 0},
		StringOperands:   []string{"", "", ""},
		InstructionCount: 3,
	}
	mp := &mockPlayer{}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if mp.lastFlashTutorial != 42 {
		t.Errorf("FlashTutorial: got %d, want 42", mp.lastFlashTutorial)
	}
	if mp.lastFlashTutorialCalls != 1 {
		t.Errorf("FlashTutorial calls: got %d, want 1", mp.lastFlashTutorialCalls)
	}
}

// TestHandleTutFlashNullRejected pins TUT_FLASH: TS wraps tab with
// NumberNotNull (PlayerOps.ts:694-695). A tab value of -1 must be
// rejected before any side-effect occurs.
func TestHandleTutFlashNullRejected(t *testing.T) {
	mp := &mockPlayer{}
	sf := &ScriptFile{
		Name: "tut_flash_null_tab",
		Opcodes: []Opcode{
			OpPushConstantInt, // tab = -1
			OpTutFlash,
			OpReturn,
		},
		IntOperands: []int32{-1, 0, 0},
	}
	state := Init(sf, mp, false, nil, nil)

	err := Execute(state)
	if err == nil {
		t.Fatalf("Execute: want error for tab=-1, got nil")
	}
	want := "TUT_FLASH: input number was null(-1)"
	if !strings.Contains(err.Error(), want) {
		t.Errorf("error: got %q, want substring %q", err.Error(), want)
	}
	if mp.lastFlashTutorialCalls != 0 {
		t.Errorf("FlashTutorial: should not have been called, got %d calls", mp.lastFlashTutorialCalls)
	}
}

// TestTutFlashNoActivePlayer pins the no-active-player guard on TUT_FLASH.
func TestTutFlashNoActivePlayer(t *testing.T) {
	sf := &ScriptFile{
		Name:             "tut_flash_nap",
		Opcodes:          []Opcode{OpPushConstantInt, OpTutFlash, OpReturn},
		IntOperands:      []int32{1, 0, 0},
		StringOperands:   []string{"", "", ""},
		InstructionCount: 3,
	}
	state := Init(sf, nil, false, nil, nil)
	if err := Execute(state); err == nil {
		t.Fatal("expected error from TUT_FLASH with no active player, got nil")
	}
	if state.Execution != Aborted {
		t.Errorf("Execution: got %v, want Aborted", state.Execution)
	}
}
```

The `strings` package is already imported in `handlers_interface_test.go` (used by `TestHandleTutOpenNullRejected` at line 1171). No new import needed.

- [ ] **Step 1.6: Add wire-level test**

Modify `modules/world/player_test.go` — append after the existing `TestEncodeOutSendsTutOpen` block (locate the closing `}` after line 803). Mirror the TutOpen wire test at lines 766-803:

```go
// TestPlayerFlashTutorialWireBytes pins the TUT_FLASH wire shape:
// (*Player).FlashTutorial(tab) → OpTutFlash (126, 1) with 1-byte
// payload = byte(tab). Mirrors TS TutFlashEncoder.ts:9-11
// (buf.p1(message.tab)).
func TestPlayerFlashTutorialWireBytes(t *testing.T) {
	enc, _ := isaacPair([4]uint32{17, 18, 19, 20})
	wantEnc, _ := isaacPair([4]uint32{17, 18, 19, 20})

	p, clientConn := newTestPlayer(t)
	p.client.encryptor = enc

	received := make(chan []byte, 1)
	go func() {
		buf := make([]byte, 2) // 1 encrypted opcode + 1 payload byte
		clientConn.SetReadDeadline(time.Now().Add(time.Second))
		if _, err := io.ReadFull(clientConn, buf); err == nil {
			received <- buf
		}
	}()

	p.FlashTutorial(7)
	p.client.flushWrite()

	expectedByte := byte((int(gameserver.OpTutFlash.Opcode) + int(wantEnc.GetNext())) & 0xff)

	select {
	case got := <-received:
		if got[0] != expectedByte {
			t.Errorf("TUT_FLASH encrypted opcode: got %d, want %d", got[0], expectedByte)
		}
		if got[1] != 7 {
			t.Errorf("TUT_FLASH tab payload: got %d, want 7", got[1])
		}
	case <-time.After(time.Second):
		t.Error("timed out waiting for TUT_FLASH")
	}
}
```

Note: this test calls `p.FlashTutorial(7)` directly (not `p.encodeOut()` like the TutOpen test) because TUT_FLASH is fire-and-forget — `writeOut` runs synchronously inside `FlashTutorial` and `flushWrite` is called explicitly to push bytes to `clientConn`. No `encodeOut` pass needed because there's no modal-state diff for FlashTutorial.

The existing imports in `player_test.go` already include `io`, `time`, and `gameserver` (used by the TutOpen tests). Verify no new imports are required.

- [ ] **Step 1.7: Run tests — confirm 3 handler tests fail with "no handler"**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run 'TestTutFlash|TestHandleTutFlash' -v
```

Expected:
- `TestTutFlash` → FAIL with `Execute: script "tut_flash": no handler for TUT_FLASH (opcode 2121) at pc=1` (or similar, depending on how the runner formats; the substring "no handler for TUT_FLASH" must appear).
- `TestHandleTutFlashNullRejected` → FAIL with the same "no handler" error (because the handler isn't registered, the null check never runs).
- `TestTutFlashNoActivePlayer` → may PASS (the runner aborts at handler-lookup before checking active player) OR FAIL with "no handler" — either is acceptable; this test will go reliably green in T2.

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestPlayerFlashTutorialWireBytes -v
```

Expected: PASS — the wire shape is exercised entirely by `(*Player).FlashTutorial` which is implemented in this task.

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...
```

Expected: builds clean. Catches any missed `FlashTutorial` impl on a third ActivePlayer impl that the pre-flight grep may have missed.

- [ ] **Step 1.8: Commit**

```bash
git add pkg/io/protocol/game/server/prot.go \
        pkg/script/active.go \
        pkg/script/runner_test.go \
        modules/world/player_script.go \
        pkg/script/handlers_interface_test.go \
        modules/world/player_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
test(world): NAI-109 T1 — TUT_FLASH red — interface + impl + 4 tests

Stage the additive surface for TUT_FLASH: OpTutFlash wire constant
(126, 1) in protocol prot.go, ActivePlayer.FlashTutorial(int)
interface decl, mockPlayer field+method, (*Player).FlashTutorial
direct-writer impl, 3 handler-level tests, 1 wire-level test.

Handler tests fail at HEAD because OpTutFlash is in the opcode enum
but not in the dispatch map at pkg/script/handlers.go. T2 wires the
handler and registers it.

Wire test passes at HEAD because the production impl is in this
task (wire shape is byte-trivial; the test exists to pin the byte
output, not drive TDD on production code).

Mirrors LostCityRS/Engine-TS PlayerOps.ts:694-696 + TutFlashEncoder.ts.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 2: Green — handleTutFlash + register

**Files:**
- Modify: `pkg/script/handlers_interface.go` (add handleTutFlash)
- Modify: `pkg/script/handlers.go` (register OpTutFlash → handleTutFlash)

- [ ] **Step 2.1: Add handleTutFlash**

Modify `pkg/script/handlers_interface.go` — append immediately after `handleTutClose` (closing `}` currently at line 111):

```go
// handleTutFlash implements TUT_FLASH.
// TS PlayerOps.ts:694-696 — pops a single int (tab); check(tab,
// NumberNotNull). No protect gate (TS uses checkedHandler(ActivePlayer,
// ...), not ProtectedActivePlayer). Fire-and-forget — writes a
// TUT_FLASH server packet to draw the player's attention to the
// named tab.
//
// Tab argument is not range-checked: TS encoder uses p1() which
// silently truncates >255 to a single byte. Goscape's ^tab_* runescript
// constants are non-negative single-byte tab indices, so this is
// behaviorally equivalent to TS for in-range inputs.
func handleTutFlash(s *ScriptState) error {
	if s.Pointers&PtrActivePlayer == 0 || s.Self == nil {
		return errors.New("TUT_FLASH: no active player")
	}
	tab := s.PopInt()
	if err := checkNotNull(tab, "TUT_FLASH"); err != nil {
		return err
	}
	s.Self.FlashTutorial(tab)
	return nil
}
```

Verify `errors` is already imported (the existing `handleTutOpen` uses `errors.New` at line 93). No new import needed.

- [ ] **Step 2.2: Register OpTutFlash → handleTutFlash**

Modify `pkg/script/handlers.go` — locate the existing TutOpen / TutClose entries (currently at lines 297-298) and add OpTutFlash immediately after to keep the three TUT_* siblings adjacent:

```go
	OpTutOpen:        handleTutOpen,
	OpTutClose:       handleTutClose,
	OpTutFlash:       handleTutFlash,
```

- [ ] **Step 2.3: Run tests — confirm green**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run 'TestTutFlash|TestHandleTutFlash' -v
```

Expected: 3 PASS (`TestTutFlash`, `TestHandleTutFlashNullRejected`, `TestTutFlashNoActivePlayer`).

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestPlayerFlashTutorialWireBytes -v
```

Expected: PASS.

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
```

Expected: full project test suite passes — no regressions. Per `verify_implementer_claims` memory: do NOT report "tests pass" off a package-scoped run alone; the cross-package run is required.

- [ ] **Step 2.4: Commit**

```bash
git add pkg/script/handlers_interface.go pkg/script/handlers.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): NAI-109 T2 — TUT_FLASH green — handleTutFlash + register

Add handleTutFlash dispatch: pointer-gate ActivePlayer, popInt tab,
checkNotNull(tab, "TUT_FLASH") (TS NumberNotNull at PlayerOps.ts:695),
delegate to s.Self.FlashTutorial(tab). Register in handler map at
pkg/script/handlers.go between OpTutClose and the next opcode to
keep TUT_OPEN / TUT_CLOSE / TUT_FLASH siblings adjacent.

Closes T1 reds: 3 handler-level tests now pass; wire test still
passes. Full project test suite green (verify_implementer_claims).

Mirrors LostCityRS/Engine-TS PlayerOps.ts:694-696.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 3: End-of-bundle review + close

**Files:** none modified directly; this task may produce post-review fix-up commits if the reviewer surfaces issues.

- [ ] **Step 3.1: Dispatch end-of-bundle review subagent (Sonnet)**

Per `superpowers_code_reviewer_model` memory: review must run on Sonnet, never Opus. Per `superpowers:requesting-code-review` skill: dispatch a `feature-dev:code-reviewer` subagent in foreground with the spec + plan + commit range as context.

Reviewer prompt should include:
- Spec path: `docs/superpowers/specs/2026-05-05-nai-109-tut-flash-handler-design.md`
- Plan path: `docs/superpowers/plans/2026-05-05-nai-109-tut-flash-handler.md`
- Commit range: T1 SHA..T2 SHA (`git log --oneline 3b069e2..HEAD`)
- Specific verification asks:
  - Does T2's `handleTutFlash` exactly mirror TS PlayerOps.ts:694-696 (pop int → check NumberNotNull → call FlashTutorial)?
  - Is the wire byte shape (opcode 126, single-byte tab payload) correct against TS TutFlashEncoder.ts?
  - Are the 3 handler tests + 1 wire test sufficient to pin behavior, or are there obvious gaps (e.g. edge tab values, multi-flash sequence)?
  - Is the registry order (TutOpen → TutClose → TutFlash) consistent with the file's existing convention?
  - Per `defensive_gate_doc_comment_label` memory: any goscape-only defensive gates not labeled?
  - Per `dead_api_polish` memory: any same-bundle YAGNI surface (helpers shipped with zero consumers)?

- [ ] **Step 3.2: Apply reviewer-flagged fix-ups (if any)**

If the reviewer surfaces issues, apply fixes inline as a separate commit (NEVER amend prior commits per CLAUDE.md). Use commit prefix `fix(...)` or `docs(...)` matching the change kind.

If no issues surfaced, proceed to Step 3.3 directly.

- [ ] **Step 3.3: Verify final state**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...
git log --oneline 3b069e2..HEAD
git status
```

Expected:
- `go test ./...` → all packages pass.
- `go vet ./...` → clean.
- `git log` → 2-4 commits (T1 + T2 + optional fix-ups).
- `git status` → clean working tree on tracked files (only pre-existing untracked dotfiles per `feedback_subagent_wt_path`).

- [ ] **Step 3.4: Close commit with memory trailer**

Per `close_commit_memory_trailer` memory: close commits carry a `Closes memory:` trailer for grep-discoverable provenance.

```bash
git commit --no-gpg-sign --allow-empty -m "$(cat <<'EOF'
chore(close): NAI-109 — TUT_FLASH script-opcode handler + wire packet

Wire goscape's OpTutFlash (script opcode 2121) and ServerGameProt.
TUT_FLASH (126, 1) wire packet. Pure additive port; no behavior
change to existing paths.

Resolves the runtime "no handler for TUT_FLASH (opcode 2121)" error
hit by [proc,tutorial_step_view_inventory] and 11 sibling chatbox-
step procs across tut_chatbox_steps.rs2.

Cadence: 2-task TDD bundle (T1 red → T2 green) on Sonnet via
subagent-driven-development per execution_mode_default.md, with
end-of-bundle review on Sonnet per superpowers_code_reviewer_model.

Mirrors LostCityRS/Engine-TS PlayerOps.ts:694-696 (handler) +
TutFlashEncoder.ts:9-11 (wire) + ServerGameProt.ts:24 (proto).

Out of scope: [label,tutorial_complete] P_TELEJUMP "script not
protected" runtime error — root cause not yet bound. Routed to
NAI-110 investigation per spec §9.

Closes memory: nai_followups.md NAI-91 + NAI-92 tutorial-progression
references — partial unblock; full tutorial-island progression
remains gated on NAI-110 protect-context investigation.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

- [ ] **Step 3.5: Update memory**

Per `post_task_handoff` memory: at every task close, save non-derivable info to memory. For NAI-109 specifically, this likely surfaces no new memory entries (compressed-cadence-equivalent additive port; pattern-continuation of NAI-76/NAI-102 TutOpen/TutClose). If a non-obvious lesson surfaces during T1 or T2 (e.g. unexpected ActivePlayer impl, wire-test divergence, registry-ordering surprise), write a topic file under `/home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/` and add a one-line index entry to `MEMORY.md`.

Update `nai_followups.md`:
- Add `## NAI-109 — CLOSED <date>` section under the existing pattern (mirror NAI-108 close at line 5487+).
- Note: NAI-110 routing for P_TELEJUMP investigation.

---

## Self-Review

**Spec coverage check:**

| Spec section | Plan coverage |
|---|---|
| §1 Scope (in-scope items) | T1.1-1.4 (interface + impl + wire constant); T1.5-1.6 (tests); T2.1-2.2 (handler + register) |
| §1 Out of scope (NAI-110 P_TELEJUMP) | T3.4 close commit narrative; not a coding task |
| §3 TS reference | T1.4 docstring + T2.1 docstring cite PlayerOps.ts:694-696, TutFlashEncoder.ts:9-11 |
| §4.1 Wire opcode | T1.1 |
| §4.2 ActivePlayer interface | T1.2 |
| §4.3 (*Player) impl | T1.4 |
| §4.4 Handler + registry | T2.1 + T2.2 |
| §5.1 mockPlayer extension | T1.3 |
| §5.2 Three handler tests | T1.5 (TestTutFlash, TestHandleTutFlashNullRejected, TestTutFlashNoActivePlayer) |
| §5.3 Wire test | T1.6 (TestPlayerFlashTutorialWireBytes) |
| §6 Risks | T2.1 docstring covers tab-range / no-protect-gate; T1.4 docstring covers no-client-nil-guard convention |
| §7 Smoke | Out-of-bundle (user-launched per smoke_test_server_handoff); referenced in T3 close commit |
| §8 Closes | T3.4 close commit narrative |
| §9 Out-of-scope follow-ups | T3.4 close commit narrative |

**Placeholder scan:** No "TBD" / "TODO" / "implement later" tokens. Every test body and every production code block is complete and pasteable. ✓

**Type consistency check:**
- `FlashTutorial(tab int)` signature consistent across §1.2 (interface), §1.3 (mockPlayer method), §1.4 ((*Player) method), §1.5 (test asserts), §2.1 (handler delegate). ✓
- `lastFlashTutorial int` + `lastFlashTutorialCalls int` field names consistent across §1.3 (decl) and §1.5 (test reads). ✓
- `OpTutFlash` constant referenced consistently as `OpTutFlash` across handler map (§2.2) and wire constant (§1.1) — note: `pkg/script.OpTutFlash` is the script opcode (2121), `gameserver.OpTutFlash` is the wire op `{126, 1}`; same name, different types, different packages — matches the existing TutOpen pattern (`pkg/script.OpTutOpen` = 2122 vs `gameserver.OpTutOpen` = `{185, 2}`). No ambiguity at use sites because each is referenced under its package qualifier. ✓
- Error string `"TUT_FLASH: input number was null(-1)"` consistent with `checkNotNull` output and test assertion. Note the format follows `checkNotNull`'s `"%s: input number was null(-1)"` template (verified at TUT_OPEN test pattern, `handlers_interface_test.go:1170`). ✓
- `Aborted` test assertion at §1.5 matches `state.Execution` enum value used by `TestTutOpenNoActivePlayer` precedent. ✓
