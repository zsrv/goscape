# NPC_NAME / NPC_CATEGORY Read-Side Validator Wiring — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Wire the existing `checkNpcType` validator at `handleNpcName` and `handleNpcCategory` in `pkg/script/handlers_npc.go` to match TS `NpcOps.ts:270-272` and `:68-70`, converting silent sentinel fallbacks (`"null"` / `-1` on registry miss) to script-level errors per TS `check(activeNpc.type, NpcTypeValid)`.

**Architecture:** Refactor-shaped XS slice. Mirrors the canonical four-guard pattern at `NPC_TYPE` / `NPC_CHANGETYPE` / `NPC_CHANGETYPE_KEEPALL` (`handlers_npc.go:186`, `:373`, `:393`): `requireActiveNpc` → `requireConfigs` → `checkNpcType` → field access. NPC_NAME preserves TS-faithful `Name → DebugName → "null"` field cascade (matches TS `?? 'null'`). NPC_CATEGORY direct field access, no fallback. Two test flips, one new error-asserting test helper, single atomic impl commit.

**Tech Stack:** Go 1.26 (per `GOROOT=/home/owner/go/go1.26.3`), `pkg/script/` script-engine package, `testing` standard library.

**Spec:** `docs/superpowers/specs/2026-05-21-npc-name-category-readside-validator-wiring-design.md` (HEAD `977d6a48`).

**Go command prefix (per CLAUDE.md global):**

```
GOROOT=/home/owner/go/go1.26.3 GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache /home/owner/go/go1.26.3/bin/go ...
GOROOT=/home/owner/go/go1.26.3 GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache /home/owner/go/go1.26.3/bin/gofmt -l ...
```

---

## File Structure

| File | Action | Responsibility |
|---|---|---|
| `pkg/script/handlers_npc.go` | Modify (lines 239-263, 303-321) | Wire `requireConfigs` + `checkNpcType` into `handleNpcName` and `handleNpcCategory`; remove silent fallback paths; preserve `DebugName` field cascade at NPC_NAME. |
| `pkg/script/handlers_npc_test.go` | Modify (line 462 area; lines 606-613 and 640-647) | Add `runNpcOpExpectErr` helper next to `runNpcOp` (line 462). Flip `TestNpcCategoryUnknownTypeReturnsMinusOne` and `TestNpcNameUnknownTypeReturnsNull` to error-asserting equivalents. |

No new files. No package-level structural changes.

---

## Task 1: Add `runNpcOpExpectErr` test helper

**Files:**
- Modify: `pkg/script/handlers_npc_test.go` (insert after existing `runNpcOp` at line 483)

**Why:** Existing `runNpcOp` calls `t.Fatalf` on any handler error. The flipped tests need to ASSERT an error rather than fail on one. Mirrors `runConfigOpExpectErr` at `handlers_config_test.go:264` — the canonical error-path test helper in this package.

- [ ] **Step 1: Verify `strings` import is already present in the test file**

Run: `GOROOT=/home/owner/go/go1.26.3 GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache /home/owner/go/go1.26.3/bin/go run -trimpath -tags ignore /dev/stdin <<<'package main' 2>/dev/null; grep -n '^import\|"strings"' pkg/script/handlers_npc_test.go | head -5`

Simpler: `grep -n '"strings"' pkg/script/handlers_npc_test.go | head -3`

Expected: at least one match showing `"strings"` is already imported in the test file (it's used by many existing error-path tests). If NO match appears, add `"strings"` to the import block in a separate edit before continuing.

- [ ] **Step 2: Insert the helper immediately after `runNpcOp`**

Locate the closing brace of `runNpcOp` (line 483 area in current file). Insert the following function on the line immediately after (preserve a single blank line between the two helpers):

```go
// runNpcOpExpectErr executes a single-opcode script against npc + optional
// mc, with pre-pushed int inputs, and asserts the resulting error contains
// substr. Mirrors runConfigOpExpectErr at handlers_config_test.go:264.
func runNpcOpExpectErr(t *testing.T, npc ActiveNpc, mc *mockConfigs, op Opcode, intInputs []int, substr string) {
	t.Helper()
	sf := &ScriptFile{
		Name:             "test_" + op.String(),
		Opcodes:          []Opcode{op, OpReturn},
		IntOperands:      []int32{0, 0},
		StringOperands:   []string{"", ""},
		InstructionCount: 2,
	}
	state := Init(sf, nil, false, nil, nil)
	state.ActiveNpc = npc
	if mc != nil {
		state.Configs = mc
	}
	for _, v := range intInputs {
		state.PushInt(v)
	}
	err := Execute(state)
	if err == nil {
		t.Fatalf("%s: expected error containing %q, got nil", op.String(), substr)
	}
	if !strings.Contains(err.Error(), substr) {
		t.Fatalf("%s: expected error containing %q, got %q", op.String(), substr, err.Error())
	}
}
```

- [ ] **Step 3: Verify file compiles**

Run: `GOROOT=/home/owner/go/go1.26.3 GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache /home/owner/go/go1.26.3/bin/go vet ./pkg/script/`

Expected: empty output (no errors). The new helper is currently unused — `go vet` will not flag that for a `_test.go` file at package level since other tests will pick it up shortly.

- [ ] **Step 4: Verify existing tests still pass**

Run: `GOROOT=/home/owner/go/go1.26.3 GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache /home/owner/go/go1.26.3/bin/go test ./pkg/script/ -run 'TestNpcName$|TestNpcNameFallsBackToDebugName|TestNpcNameUnknownTypeReturnsNull|TestNpcCategory$|TestNpcCategoryUnknownTypeReturnsMinusOne' -count=1`

Expected: PASS. All five existing tests should still pass at this point — the helper is added but no other test calls it yet, and the handlers are unchanged.

---

## Task 2: Flip the two `Unknown*` tests (test-first)

**Files:**
- Modify: `pkg/script/handlers_npc_test.go:606-613` (`TestNpcCategoryUnknownTypeReturnsMinusOne`)
- Modify: `pkg/script/handlers_npc_test.go:640-647` (`TestNpcNameUnknownTypeReturnsNull`)

**Why:** TDD-shape — flip the tests BEFORE wiring the handlers so they fail first (proving the test exercises the new behavior), then pass once the handler is wired (proving the wire is correct).

- [ ] **Step 1: Replace `TestNpcCategoryUnknownTypeReturnsMinusOne` with the error-asserting equivalent**

Find the current block at `pkg/script/handlers_npc_test.go:606-613`:

```go
func TestNpcCategoryUnknownTypeReturnsMinusOne(t *testing.T) {
	mc := newTestConfigs()
	npc := &mockNpc{typeID: 9999}
	state := runNpcOp(t, npc, mc, OpNpcCategory, nil)
	if got := state.PopInt(); got != -1 {
		t.Errorf("NPC_CATEGORY(unknown): got %d, want -1", got)
	}
}
```

Replace with:

```go
func TestNpcCategory_UnknownType_ReturnsError(t *testing.T) {
	mc := newTestConfigs()
	npc := &mockNpc{typeID: 9999}
	runNpcOpExpectErr(t, npc, mc, OpNpcCategory, nil,
		"NPC_CATEGORY: no NpcType with value (9999) found")
}
```

- [ ] **Step 2: Replace `TestNpcNameUnknownTypeReturnsNull` with the error-asserting equivalent**

Find the current block at `pkg/script/handlers_npc_test.go:640-647`:

```go
func TestNpcNameUnknownTypeReturnsNull(t *testing.T) {
	mc := newTestConfigs()
	npc := &mockNpc{typeID: 9999}
	state := runNpcOp(t, npc, mc, OpNpcName, nil)
	if got := state.PopString(); got != "null" {
		t.Errorf("NPC_NAME(unknown): got %q, want %q", got, "null")
	}
}
```

Replace with:

```go
func TestNpcName_UnknownType_ReturnsError(t *testing.T) {
	mc := newTestConfigs()
	npc := &mockNpc{typeID: 9999}
	runNpcOpExpectErr(t, npc, mc, OpNpcName, nil,
		"NPC_NAME: no NpcType with value (9999) found")
}
```

- [ ] **Step 3: Run the two flipped tests and verify they FAIL with the expected diagnostic**

Run: `GOROOT=/home/owner/go/go1.26.3 GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache /home/owner/go/go1.26.3/bin/go test ./pkg/script/ -run 'TestNpcCategory_UnknownType_ReturnsError|TestNpcName_UnknownType_ReturnsError' -count=1 -v`

Expected: BOTH tests FAIL with messages like `NPC_CATEGORY: expected error containing "NPC_CATEGORY: no NpcType with value (9999) found", got nil` and `NPC_NAME: expected error containing "NPC_NAME: no NpcType with value (9999) found", got nil`. This is the TDD red phase — the tests correctly exercise behavior that the current handlers don't have.

- [ ] **Step 4: Verify other NPC_NAME / NPC_CATEGORY tests still pass**

Run: `GOROOT=/home/owner/go/go1.26.3 GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache /home/owner/go/go1.26.3/bin/go test ./pkg/script/ -run 'TestNpcName$|TestNpcNameFallsBackToDebugName|TestNpcCategory$' -count=1`

Expected: PASS. The happy-path and DebugName-fallback tests are unaffected by the flips.

---

## Task 3: Wire `checkNpcType` in `handleNpcCategory`

**Files:**
- Modify: `pkg/script/handlers_npc.go:303-321`

**Why:** Replace the silent-`-1` fallback with the canonical four-guard pattern that throws on registry miss, matching TS `NpcOps.ts:68-70` `check(activeNpc.type, NpcTypeValid).category`.

- [ ] **Step 1: Replace the `handleNpcCategory` body**

Find the current block at `pkg/script/handlers_npc.go:303-321`:

```go
// handleNpcCategory looks up the ActiveNpc's NpcType via Configs and
// pushes its Category, or -1 if the type can't be resolved.
func handleNpcCategory(s *ScriptState) error {
	if err := requireActiveNpc(s, "NPC_CATEGORY"); err != nil {
		return err
	}
	if s.Configs == nil {
		s.PushInt(-1)
		return nil
	}
	cfg := s.Configs.NpcType(s.ActiveNpc.NpcType())
	if cfg == nil {
		s.PushInt(-1)
		return nil
	}
	s.PushInt(cfg.Category)
	return nil
}
```

Replace with:

```go
// handleNpcCategory looks up the ActiveNpc's NpcType via Configs and
// pushes its Category. Mirrors TS NpcOps.ts:68-70 —
// check(activeNpc.type, NpcTypeValid).category (no fallback).
func handleNpcCategory(s *ScriptState) error {
	if err := requireActiveNpc(s, "NPC_CATEGORY"); err != nil {
		return err
	}
	if err := requireConfigs(s, "NPC_CATEGORY"); err != nil {
		return err
	}
	typeID := s.ActiveNpc.NpcType()
	if err := checkNpcType(s, typeID, "NPC_CATEGORY"); err != nil {
		return err
	}
	s.PushInt(s.Configs.NpcType(typeID).Category)
	return nil
}
```

- [ ] **Step 2: Run the flipped NPC_CATEGORY test and verify it PASSES**

Run: `GOROOT=/home/owner/go/go1.26.3 GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache /home/owner/go/go1.26.3/bin/go test ./pkg/script/ -run 'TestNpcCategory_UnknownType_ReturnsError' -count=1 -v`

Expected: PASS. The handler now errors with `"NPC_CATEGORY: no NpcType with value (9999) found"`.

- [ ] **Step 3: Run the happy-path NPC_CATEGORY test and verify still PASSES**

Run: `GOROOT=/home/owner/go/go1.26.3 GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache /home/owner/go/go1.26.3/bin/go test ./pkg/script/ -run 'TestNpcCategory$' -count=1`

Expected: PASS. Registered type still pushes `Category=99` correctly.

---

## Task 4: Wire `checkNpcType` in `handleNpcName`

**Files:**
- Modify: `pkg/script/handlers_npc.go:239-263`

**Why:** Same TS-faithfulness wire as Task 3, but at NPC_NAME. Preserve the goscape `DebugName` field cascade (still matches TS `?? 'null'` observable behavior — the cascade only fires when the registered type's `Name` field is empty, equivalent to TS `null`).

- [ ] **Step 1: Replace the `handleNpcName` body**

Find the current block at `pkg/script/handlers_npc.go:239-263`:

```go
// handleNpcName looks up the ActiveNpc's NpcType via Configs and pushes
// its Name, falling back to DebugName, then "null" (matching TS
// nullish-coalesce on NpcType.name).
func handleNpcName(s *ScriptState) error {
	if err := requireActiveNpc(s, "NPC_NAME"); err != nil {
		return err
	}
	if s.Configs == nil {
		return errors.New("NPC_NAME: no configs")
	}
	cfg := s.Configs.NpcType(s.ActiveNpc.NpcType())
	if cfg == nil {
		s.PushString("null")
		return nil
	}
	name := cfg.Name
	if name == "" {
		name = cfg.DebugName
	}
	if name == "" {
		name = "null"
	}
	s.PushString(name)
	return nil
}
```

Replace with:

```go
// handleNpcName looks up the ActiveNpc's NpcType via Configs and pushes
// its Name, falling back to DebugName then "null" (matching TS
// nullish-coalesce on NpcType.name).
// Mirrors TS NpcOps.ts:270-272 — check(activeNpc.type, NpcTypeValid).
func handleNpcName(s *ScriptState) error {
	if err := requireActiveNpc(s, "NPC_NAME"); err != nil {
		return err
	}
	if err := requireConfigs(s, "NPC_NAME"); err != nil {
		return err
	}
	typeID := s.ActiveNpc.NpcType()
	if err := checkNpcType(s, typeID, "NPC_NAME"); err != nil {
		return err
	}
	cfg := s.Configs.NpcType(typeID)
	name := cfg.Name
	if name == "" {
		name = cfg.DebugName
	}
	if name == "" {
		name = "null"
	}
	s.PushString(name)
	return nil
}
```

- [ ] **Step 2: Check whether `errors` import is still used elsewhere in `handlers_npc.go`**

The original `handleNpcName` was the only call site of `errors.New` removed by this slice — but other handlers in the same file may still use `errors`. Verify before deciding to remove the import:

Run: `grep -n '\berrors\.' pkg/script/handlers_npc.go`

Expected: list of remaining `errors.` references. If the only remaining reference was the one we just removed (i.e. no other matches), proceed to Step 3 to remove the import. If other matches exist, skip Step 3.

- [ ] **Step 3 (conditional): Remove the `errors` import if unused**

Only execute this step if Step 2 returned no remaining `errors.` references. Open the import block at the top of `pkg/script/handlers_npc.go` and remove the line containing `"errors"`. If a blank line is left, collapse it.

- [ ] **Step 4: Run the flipped NPC_NAME test and verify it PASSES**

Run: `GOROOT=/home/owner/go/go1.26.3 GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache /home/owner/go/go1.26.3/bin/go test ./pkg/script/ -run 'TestNpcName_UnknownType_ReturnsError' -count=1 -v`

Expected: PASS. The handler now errors with `"NPC_NAME: no NpcType with value (9999) found"`.

- [ ] **Step 5: Run the happy-path + DebugName-fallback NPC_NAME tests and verify still PASS**

Run: `GOROOT=/home/owner/go/go1.26.3 GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache /home/owner/go/go1.26.3/bin/go test ./pkg/script/ -run 'TestNpcName$|TestNpcNameFallsBackToDebugName' -count=1`

Expected: PASS. Registered type with `Name="Hans"` still returns `"Hans"`. Registered type with empty `Name` and `DebugName="unnamed_npc"` still returns `"unnamed_npc"`.

---

## Task 5: Run full validation gates and commit

**Files:** None modified. Validation + atomic commit.

- [ ] **Step 1: Run the full package test with race detector**

Run: `GOROOT=/home/owner/go/go1.26.3 GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache /home/owner/go/go1.26.3/bin/go test -race ./... -count=1`

Expected: all packages PASS, no race warnings. Should complete in ~150 seconds based on predecessor-slice telemetry.

- [ ] **Step 2: Run the cache pipeline smoke test**

Run: `GOROOT=/home/owner/go/go1.26.3 GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache /home/owner/go/go1.26.3/bin/go test ./pkg/packall/ -run TestPackAll_TwelveStageSmoke -count=1`

Expected: PASS.

- [ ] **Step 3: Run `gofmt -l` on the two edited files**

Run: `GOROOT=/home/owner/go/go1.26.3 GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache /home/owner/go/go1.26.3/bin/gofmt -l pkg/script/handlers_npc.go pkg/script/handlers_npc_test.go`

Expected: empty output (no files need reformatting).

- [ ] **Step 4: Run audit-greps and verify deltas**

Run each and verify the count matches the expected value:

```bash
grep -c 'checkNpcType(s, ' pkg/script/handlers_npc.go
# Expected: 10 (was 8, +2 for NPC_NAME and NPC_CATEGORY)

grep -cE 'requireConfigs\(s, "(NPC_NAME|NPC_CATEGORY)"' pkg/script/handlers_npc.go
# Expected: 2 (was 0)

grep -n 'NPC_NAME: no configs' pkg/script/handlers_npc.go
# Expected: 0 matches (bespoke wording removed; canonical via requireConfigs)
```

If any count diverges from the expected value, STOP and re-check the previous tasks before committing.

- [ ] **Step 5: Verify git status before commit**

Run: `git status --short`

Expected output (in some order):

```
 M config.yaml
 M pkg/script/handlers_npc.go
 M pkg/script/handlers_npc_test.go
?? .bash_profile
?? .bashrc
?? .claude/
... (standing untracked noise)
```

CRITICAL: do NOT stage `config.yaml` or any of the standing untracked noise (`.claude/`, `.bashrc`/`.zshrc`/etc., `.vscode`, `.mcp.json`, `RUNESCRIPT.md`, `.bash_profile`, `.gitconfig`, `.gitmodules`, `.profile`, `.ripgreprc`, `.zprofile`). The only staged paths must be `pkg/script/handlers_npc.go` and `pkg/script/handlers_npc_test.go`.

- [ ] **Step 6: Stage and commit**

Run:

```bash
git add pkg/script/handlers_npc.go pkg/script/handlers_npc_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
refactor(script): wire checkNpcType at NPC_NAME/NPC_CATEGORY read sites

Closes Shape B subset (NPC_NAME / NPC_CATEGORY) of resume-memo item #1
deferred by predecessor 712a407a. Wires checkNpcType + requireConfigs at
handleNpcName (handlers_npc.go:239) and handleNpcCategory (:303) to
match TS NpcOps.ts:270-272 / :68-70 (check(activeNpc.type, NpcTypeValid)).
Converts silent sentinel fallbacks ("null" / -1 on registry miss) to
script-level errors. Preserves TS-faithful Name -> DebugName -> "null"
field cascade at NPC_NAME (TS ?? 'null' is field-null fallback).

Adds runNpcOpExpectErr helper mirroring runConfigOpExpectErr at
handlers_config_test.go:264. Flips TestNpcCategoryUnknownTypeReturns-
MinusOne and TestNpcNameUnknownTypeReturnsNull to error-asserting
equivalents (renamed to *_UnknownType_ReturnsError per Go test
convention). Happy-path TestNpcName / TestNpcCategory / TestNpcName-
FallsBackToDebugName unchanged.

Spec: docs/superpowers/specs/2026-05-21-npc-name-category-readside-
validator-wiring-design.md (977d6a48).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

- [ ] **Step 7: Verify commit landed**

Run: `git log --oneline -1`

Expected: shows the new commit at HEAD with subject `refactor(script): wire checkNpcType at NPC_NAME/NPC_CATEGORY read sites`.

Run: `git status --short`

Expected: only the standing untracked noise + `config.yaml` drift; no `pkg/script/handlers_npc*` entries.

---

## Self-Review Summary

**Spec coverage:**
- Spec §3.1 in-scope items (handleNpcName, handleNpcCategory edits + test flips) → Tasks 3, 4, 2.
- Spec §4 handler shape and field-cascade preservation → Tasks 3, 4 (with exact code blocks).
- Spec §5.1 unchanged tests → Verified in Task 3 Step 3, Task 4 Step 5, Task 5 Step 1.
- Spec §5.2 flipped tests → Task 2 with full code.
- Spec §5.3 optional nil-Configs additions → deferred to a future coverage-gap slice (matches spec language "If sibling coverage is implicit / absent across the whole family, defer"; existing sibling NPC_TYPE / NPC_CHANGETYPE tests don't have nil-Configs error-path coverage either, so deferring is consistent).
- Spec §6 validation gates → Task 5 Steps 1-4 with exact commands and expected counts.
- Spec §7 TS-faithfulness checklist → All items satisfied by Task 3 / Task 4 handler shapes.

**Placeholder scan:** No TBD, TODO, vague directives, or unspecified code blocks. The one conditional step (Task 4 Step 3 — remove `errors` import if unused) has an explicit precondition gate via Step 2 grep.

**Type / name consistency:**
- Helper name `runNpcOpExpectErr` consistent across Task 1 definition and Task 2 call sites.
- Error wording `"NPC_NAME: no NpcType with value (9999) found"` and `"NPC_CATEGORY: no NpcType with value (9999) found"` consistent with `checkNpcType`'s `fmt.Errorf("%s: no NpcType with value (%d) found", op, id)` at `handlers_npc.go:90`.
- Test renames `_UnknownType_ReturnsError` consistent across both flipped tests.

**Scope check:** Single XS slice, two-handler refactor + one helper + two test flips, one atomic impl commit. Appropriately focused.

---

## Carry-forward menu (post-slice)

Per spec §10:

1. NPC_DEL cached Respawnrate vs registry divergence (XS audit, low priority).
2. NAI-162 analytics RPC.
3. Combat-level read-site verification.
4. Deviation audit refresh.
5. General world/runescript engine work.
6. OC_* Part B + most NC_* bespoke-unknown-id error test coverage gap (low priority).
