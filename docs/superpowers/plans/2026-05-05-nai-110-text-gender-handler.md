# NAI-110 — TEXT_GENDER script-opcode handler — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Wire goscape's `OpTextGender` (script opcode 4504) so runescript `text_gender(male, female)` calls dispatch to a handler that mirrors TS `PlayerOps.ts:787-794` instead of erroring at runtime.

**Architecture:** Pure additive port. Add one handler `handleTextGender` in `pkg/script/handlers_player.go` (file home for player-coupled ops that read `s.Self.Gender()`, e.g. `handleSetIdKit`); register one dispatch entry in the `S5a: string ops.` block at `pkg/script/handlers.go:151-162` (TextGender is opcode 4504, sibling to OpAppend/OpLowercase 4500-4503). No interface changes (`ActivePlayer.Gender() int` already declared at `active.go:513` from NAI-47), no mock changes (`mockPlayer.genderValue` + `Gender()` already in test fixture from NAI-47).

**Tech Stack:** Go 1.26+ (per `go_version` memory). Tests via `go test`. Always invoke as `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go ...` per project CLAUDE.md.

**Spec:** `docs/superpowers/specs/2026-05-05-nai-110-text-gender-design.md` (commit `ea14e02`).

**Cadence:** Compressed per `compressed_cadence.md` (~10 prod LOC + ~50 test LOC). 3 tasks (T1 red → T2 green → T3 close) on Sonnet via subagent-driven-development per `execution_mode_default.md`. End-of-bundle review on Sonnet per `superpowers_code_reviewer_model.md`.

**Pre-flight verified at HEAD `ea14e02` (controller_preflight):**
- `OpTextGender = 4504` declared at `pkg/script/opcode.go:416`. ✓
- `OpTextGender` String() case at `pkg/script/opcode.go:1165-1166` (`return "TEXT_GENDER"`). ✓
- NO dispatch entry in `pkg/script/handlers.go` map. ✓
- `ActivePlayer.Gender() int` declared at `pkg/script/active.go:510-513` (NAI-47). ✓
- `(*Player).Gender()` impl at `modules/world/player_script.go:941`. ✓
- `mockPlayer.genderValue` field at `pkg/script/runner_test.go:293`; `Gender()` method at `pkg/script/runner_test.go:586`. ✓
- `requireActivePlayer` helper at `pkg/script/handlers_player.go:33-40`. ✓
- `s.PopString()` at `pkg/script/state.go:307-313`; `s.PushString(v)` at `pkg/script/state.go:297-303`. ✓
- Test fixture idiom (per `scriptstate_test_fixture_idioms` memory): `&ScriptState{Pointers: PtrActivePlayer, Self: mp, IntStack: make([]int, StackCapacity), StringStack: make([]string, StackCapacity)}` — confirmed in `pkg/script/handlers_player_test.go:1826,1847,1865,1883,1901`. ✓
- TS handler at `Engine-TS PlayerOps.ts:787-794`. ✓
- TS pop order at `Engine-TS ScriptState.ts:341-347` (popStrings(2): index 1 popped first, index 0 second). ✓
- S5a: string ops dispatch block at `pkg/script/handlers.go:151-162` hosts the 4500-band string opcodes (Append, AppendNum, Lowercase, Compare, etc.). ✓

---

### Task 1: Red — write 4 failing tests

**Files:**
- Test (modify): `pkg/script/handlers_player_test.go` (append 4 tests at end of file)

**Why this is "red":** The 4 tests dispatch `handleTextGender` (a function that does not yet exist). The Go compiler will fail with `undefined: handleTextGender` during `go test`. That is the expected red state. We do NOT dispatch via the opcode→handler map in these tests; we call `handleTextGender(s)` directly to keep the failure mode at compile-time and the assertions tight on the handler's contract.

- [ ] **Step 1.1: Append 4 tests at end of `pkg/script/handlers_player_test.go`**

Append the following block as-is (verify the closing `}` of the file precedes this — these are top-level test functions, not nested):

```go
// TestTextGenderMale: gender=0 → handler pushes the male string (the
// second-popped, i.e. the bottom of the two-string slice on entry).
// Mirrors TS PlayerOps.ts:787-794, gender===0 branch.
func TestTextGenderMale(t *testing.T) {
	mp := &mockPlayer{genderValue: 0}
	s := &ScriptState{
		Pointers:    PtrActivePlayer,
		Self:        mp,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	s.PushString("MALE")   // pushed first → below
	s.PushString("FEMALE") // pushed last → top
	if err := handleTextGender(s); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.SSP != 1 {
		t.Fatalf("SSP: got %d, want 1", s.SSP)
	}
	if got := s.PopString(); got != "MALE" {
		t.Errorf("pushed string: got %q, want %q", got, "MALE")
	}
}

// TestTextGenderFemale: gender=1 → handler pushes the female string
// (the first-popped, i.e. top of stack on entry). Mirrors TS
// PlayerOps.ts:787-794, gender!==0 branch.
func TestTextGenderFemale(t *testing.T) {
	mp := &mockPlayer{genderValue: 1}
	s := &ScriptState{
		Pointers:    PtrActivePlayer,
		Self:        mp,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	s.PushString("MALE")   // pushed first → below
	s.PushString("FEMALE") // pushed last → top
	if err := handleTextGender(s); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.SSP != 1 {
		t.Fatalf("SSP: got %d, want 1", s.SSP)
	}
	if got := s.PopString(); got != "FEMALE" {
		t.Errorf("pushed string: got %q, want %q", got, "FEMALE")
	}
}

// TestTextGenderNoActivePlayer: pointer-gate. Self=nil and/or
// PtrActivePlayer unset → handler returns the standard
// requireActivePlayer error and leaves the string stack untouched.
func TestTextGenderNoActivePlayer(t *testing.T) {
	s := &ScriptState{
		Pointers:    0, // no PtrActivePlayer
		Self:        nil,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	s.PushString("a")
	s.PushString("b")
	err := handleTextGender(s)
	if err == nil {
		t.Fatal("want error for no-active-player, got nil")
	}
	if !strings.Contains(err.Error(), "TEXT_GENDER: no active player") {
		t.Errorf("error: got %q, want substring %q", err.Error(), "TEXT_GENDER: no active player")
	}
	if s.SSP != 2 {
		t.Errorf("SSP: got %d, want 2 (stack must be untouched on guard reject)", s.SSP)
	}
}

// TestTextGenderEmptyStrings: TS does NOT call check(..., StringNotNull)
// on either argument (PlayerOps.ts:787-794 — destructure-and-push, no
// gate). Empty strings are valid input and pass through unchanged.
// Per ts_asymmetry_dual_pin memory: pin the absence of a null gate so
// the test escalates if upstream TS adds one.
func TestTextGenderEmptyStrings(t *testing.T) {
	mp := &mockPlayer{genderValue: 0}
	s := &ScriptState{
		Pointers:    PtrActivePlayer,
		Self:        mp,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	s.PushString("") // male (below)
	s.PushString("") // female (top)
	if err := handleTextGender(s); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.SSP != 1 {
		t.Fatalf("SSP: got %d, want 1", s.SSP)
	}
	if got := s.PopString(); got != "" {
		t.Errorf("pushed string: got %q, want empty string", got)
	}
}
```

- [ ] **Step 1.2: Verify the imports include `"strings"`**

`pkg/script/handlers_player_test.go` already imports `"strings"` (used by other error-substring assertions in the file). If `go test` reports `undefined: strings`, append `"strings"` to the import block. (Expected: no action needed at HEAD `ea14e02`.)

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./pkg/script/ 2>&1 | head -20
```

Expected output includes (compile error):
```
./handlers_player_test.go:NNNN:NN: undefined: handleTextGender
```

- [ ] **Step 1.3: Run the new tests and verify they fail at compile time**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run 'TestTextGender' -v 2>&1 | head -30
```

Expected: build failure on `undefined: handleTextGender`. This is the red state.

- [ ] **Step 1.4: Commit T1**

```bash
git add pkg/script/handlers_player_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
test(script): NAI-110 T1 — TEXT_GENDER red — 4 failing tests

Pin gender=0/male, gender=1/female, no-active-player guard, and
empty-string passthrough (TS divergence-from-norm: no
check(..., StringNotNull) on either string arg).

Compile-time red: handleTextGender undefined.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 2: Green — handler + dispatch entry

**Files:**
- Modify: `pkg/script/handlers_player.go` (add `handleTextGender` at end of file)
- Modify: `pkg/script/handlers.go` (add dispatch entry in `S5a: string ops.` block)

- [ ] **Step 2.1: Add handler at end of `pkg/script/handlers_player.go`**

Append the following function at the end of the file (after the last existing handler — verify by reading the final 30 lines first; the file should currently end with a closing `}` of the last handler function):

```go
// handleTextGender implements TEXT_GENDER (opcode 4504). Mirrors TS
// PlayerOps.ts:787-794 — pops two strings (popStrings(2) destructures
// [male, female]; per ScriptState.ts:341-347 index 1 is popped first,
// so female is popped first off the stack, male second), then pushes
// male if gender==0 else female. No null-check on either string (TS
// does not call check(..., StringNotNull)). Pure stack op — no wire
// packet, no side effect.
func handleTextGender(s *ScriptState) error {
	if err := requireActivePlayer(s, "TEXT_GENDER"); err != nil {
		return err
	}
	female := s.PopString()
	male := s.PopString()
	if s.Self.Gender() == 0 {
		s.PushString(male)
	} else {
		s.PushString(female)
	}
	return nil
}
```

- [ ] **Step 2.2: Register dispatch entry in `pkg/script/handlers.go`**

Modify `pkg/script/handlers.go` — in the `// S5a: string ops.` block at lines 151-162. Insert immediately after `OpLowercase: handleLowercase,` (currently line 156) so that opcodes 4503 (Lowercase) and 4504 (TextGender) sit adjacent and the rest of the block remains undisturbed:

Before (block excerpt):
```go
	// S5a: string ops.
	OpAppend:              handleAppend,
	OpAppendNum:           handleAppendNum,
	OpAppendChar:          handleAppendChar,
	OpAppendSignNum:       handleAppendSignNum,
	OpLowercase:           handleLowercase,
	OpCompare:             handleCompare,
```

After:
```go
	// S5a: string ops.
	OpAppend:              handleAppend,
	OpAppendNum:           handleAppendNum,
	OpAppendChar:          handleAppendChar,
	OpAppendSignNum:       handleAppendSignNum,
	OpLowercase:           handleLowercase,
	OpTextGender:          handleTextGender,
	OpCompare:             handleCompare,
```

The map keys' visual alignment may auto-rebalance via `gofmt`; do not hand-tune column widths — `gofmt` owns alignment. After saving, run `gofmt -w pkg/script/handlers.go`.

- [ ] **Step 2.3: Run the 4 new tests and verify they pass**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run 'TestTextGender' -v 2>&1 | head -30
```

Expected:
```
=== RUN   TestTextGenderMale
--- PASS: TestTextGenderMale (0.00s)
=== RUN   TestTextGenderFemale
--- PASS: TestTextGenderFemale (0.00s)
=== RUN   TestTextGenderNoActivePlayer
--- PASS: TestTextGenderNoActivePlayer (0.00s)
=== RUN   TestTextGenderEmptyStrings
--- PASS: TestTextGenderEmptyStrings (0.00s)
PASS
ok  	github.com/zsrv/goscape/pkg/script	...
```

- [ ] **Step 2.4: Run full pkg/script test suite — no regressions**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ 2>&1 | tail -5
```

Expected: `ok  	github.com/zsrv/goscape/pkg/script	<time>` — no FAIL lines anywhere in output.

- [ ] **Step 2.5: Run full repo tests + vet (per `verify_implementer_claims` memory — package-scoped green can mask cross-package breakage)**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./... 2>&1 | tail -20
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./... 2>&1
```

Expected: every package reports `ok`; no `FAIL` lines; `go vet` produces no output.

- [ ] **Step 2.6: Commit T2**

```bash
git add pkg/script/handlers_player.go pkg/script/handlers.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): NAI-110 T2 — TEXT_GENDER green — handleTextGender + register

Pop [female, male] (popStrings(2) order per TS ScriptState.ts:341-347),
push male if Self.Gender()==0 else female. Dispatch entry in
S5a: string ops block adjacent to OpLowercase (sibling 4503).

All 4 NAI-110 tests pass; full repo go test + go vet green.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 3: Close commit

**Files:**
- (no code changes — this task ships only the close commit + memory updates)

This task is the explicit cadence-close per `close_commit_memory_trailer` memory. It also updates `nai_followups.md` to reflect that NAI-110 is closed and queue NAI-111 (P_TELEJUMP) per `nai_followups.md` NAI-109 close section.

- [ ] **Step 3.1: Final verification (re-run before close)**

Run (per `verify_implementer_claims` memory — fresh independent verification, not relying on prior task's output):

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./... 2>&1 | tail -5
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./... 2>&1
git status --short
git log --oneline -5
```

Expected:
- `go test ./...` ends with `ok` for every package; no `FAIL`.
- `go vet ./...` produces no output.
- `git status --short` shows no uncommitted changes (NAI-110 spec/plan/T1/T2 all committed; tracker updates land in this task).
- `git log --oneline -5` shows the spec → T1 → T2 sequence at HEAD.

- [ ] **Step 3.2: Update auto-memory `nai_followups.md` — append NAI-110 close section**

Append a new section to `/home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md` immediately AFTER the existing `## NAI-109 — CLOSED 2026-05-05` section (and before any older section). Content (replace `<T1-SHA>`, `<T2-SHA>` with the actual short-SHAs from `git log --oneline -3`):

```markdown
---

## NAI-110 — CLOSED 2026-05-05

**Scope:** Wire goscape's `OpTextGender` (script opcode 4504) to a
handler mirroring TS `PlayerOps.ts:787-794`. Pure stack op — pops
[female, male] (popStrings(2) destructure order per TS
ScriptState.ts:341-347), pushes male if `Self.Gender()==0` else
female. No interface changes (Gender() already on ActivePlayer
from NAI-47), no mock changes, no wire packet.

**Cadence:** Compressed (combined spec+plan, single bundle on
Sonnet via subagent-driven-development) per `compressed_cadence.md`.
Pre-flight verification of all 12 plan premises held at HEAD
`ea14e02`. ~10 production LOC + ~50 test LOC actual.

**Spec:** `docs/superpowers/specs/2026-05-05-nai-110-text-gender-design.md` (commit `ea14e02`).
**Plan:** `docs/superpowers/plans/2026-05-05-nai-110-text-gender-handler.md` (this commit's parent).

**Commits (chronological):**
T1 red: `<T1-SHA>`. T2 green: `<T2-SHA>`. Close: this commit.

**Cascade:** Resolves the `[proc,tutorial_please_wait_woodcutting]`
no-handler-for-TEXT_GENDER abort at pc=4 surfaced by NAI-109 close
smoke. Cascade attribution closes at the next user-driven Tutorial
Island chatbox-step smoke covering a `text_gender(...)` call site
(4 sites in `tut_chatbox_steps.rs2`; 60+ content-script sites
unblocked downstream).

**Deviations opened:** None. TS-fidel one-to-one port.

**Smoke handoff:** User runs server + Java client; walks tutorial
chatbox to a `text_gender` site (NAI-109 trace pinned the
`tutorial_please_wait_woodcutting` proc at the woodcutting prompt).
Pre-fix: WARN log `"no handler for TEXT_GENDER (opcode 4504)"`.
Post-fix: chatbox renders gender-substituted text; no warn log.
If smoke surfaces a different blocker, route per `cascade_theory_smoke_binding`.

---
```

- [ ] **Step 3.3: Confirm NAI-111 P_TELEJUMP item is already queued in `nai_followups.md`**

Grep the file:
```bash
rg -n "NAI-111|P_TELEJUMP" /home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md | head -5
```

Expected: NAI-109 close section (added in NAI-109 close commit `d3269ae`) names NAI-111 as the P_TELEJUMP investigation. If grep returns 0 matches, append a `## Queued` section noting the routing per the user's brainstorm message; otherwise no action.

- [ ] **Step 3.4: Commit close**

Per `close_commit_memory_trailer` memory: NAI-N close commits MUST include `Closes memory:` trailer naming any memory entries written/updated this sub-spec. NAI-110 only updates `nai_followups.md` (the close section) — no new top-level memory entries.

```bash
git add /home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md
git commit --no-gpg-sign -m "$(cat <<'EOF'
chore(close): NAI-110 — TEXT_GENDER (opcode 4504) script-opcode handler

Wires text_gender(male, female) runescript calls to a goscape handler
mirroring TS PlayerOps.ts:787-794. Closes the
tutorial_please_wait_woodcutting proc abort surfaced by NAI-109 close
smoke; unblocks 4 tutorial chatbox sites + 60+ content-script sites.

Compressed cadence (~10 prod LOC + ~50 test LOC). Pre-flight 12
premises green at HEAD ea14e02. Smoke handoff to user.

Closes memory: nai_followups.md (NAI-110 close section)

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

- [ ] **Step 3.5: Final post-close verification (per `verify_implementer_claims` memory)**

Run:
```bash
git log --oneline -6
git status --short
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./... 2>&1 | tail -3
```

Expected:
- `git log --oneline -6` shows: close → T2 → T1 → spec → (NAI-109 close) → (NAI-109 T2). Five new commits since NAI-109 close.
- `git status --short` shows no unstaged/uncommitted changes (the auto-memory file lives outside the repo, but per project convention it's hand-edited and not tracked here — confirmed by NAI-109's identical close pattern).
- `go test ./...` ends green.

---

## Self-review (post-write checklist per writing-plans skill)

**Spec coverage:**
- §1 scope (handler + dispatch) → T2.1, T2.2. ✓
- §3 TS reference (pop order, no protect gate, no null check) → T1 tests pin all three (Male/Female pop order, no-active-player not protect, EmptyStrings absence-pin). ✓
- §4.1 handler body → T2.1 verbatim. ✓
- §4.2 dispatch entry → T2.2 with explicit before/after. ✓
- §5 four tests → T1.1 ships all four with literal sentinels. ✓
- §6 risks (pop order reversed, error string drift, Gender semantics, byte width N/A) → T1 tests pin first 3; §6 flagged byte-width N/A. ✓
- §7 smoke → T3.2 close section captures handoff and cascade routing. ✓
- §8/§9 closes/follow-ups → T3.2 close section + T3.3 NAI-111 confirmation. ✓

**Placeholder scan:** No "TBD" / "TODO" / "fill in" / "similar to Task N". `<T1-SHA>` and `<T2-SHA>` in T3.2 are explicit substitution markers (replaced from `git log` output at execution time), not placeholders.

**Type consistency:** `handleTextGender(s *ScriptState) error` signature identical between T1 (test caller) and T2.1 (impl). `requireActivePlayer(s, "TEXT_GENDER")` op-name string identical between T1 (assertion substring `"TEXT_GENDER: no active player"`) and T2.1 (call). `s.Self.Gender()` matches existing `ActivePlayer.Gender() int` declaration.

No issues found.
