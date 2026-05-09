# NAI-133 — BOTH_MOVEINV + per-pointer-slot Protect refactor + FINDUID/P_FINDUID slot routing — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Reify `s.Protect bool` as `PtrProtectedActivePlayer{,2}` Pointer-bitmask flags, route FINDUID/P_FINDUID on `intOperand` to bind Self vs Self2 (closing a latent `.p_finduid` clobber bug), and port `BOTH_MOVEINV` (opcode 4301) per TS InvOps.ts:373-495 with NAI-115-D1 wealth-event reuse.

**Architecture:** T1 is a GREEN-only mechanical refactor (no behavior change; existing tests must continue to pass after fixture migration). T2 and T3 are RED→GREEN ports per NAI cadence, mirroring TS source line-by-line. Reuses existing helpers (`requireActivePlayer{,2}`, `checkInvType`, `lookupStackableStockObj`, `runInvOp{,WithWorld}`). Inherits NAI-115-D1 (wealth-event tail skip) and preserves a TS quirk (`BOTH_MOVEINV` to-gate evaluates `fromInvType.Scope`).

**Tech Stack:** Go 1.26+. No new dependencies.

**Spec:** `docs/superpowers/specs/2026-05-08-nai-133-both-moveinv-design.md`

**Cadence:** All `go` invocations prefix with `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache`. All commits use `--no-gpg-sign`. Each task ends with `go test ./pkg/script/... ./modules/world/... -count=1` GREEN before commit.

---

## File Structure

**Modify (no new files):**
- `pkg/script/pointer.go` — add `PtrProtectedActivePlayer{,2}` constants (T1.1)
- `pkg/script/state.go:315` — delete `Protect bool` field (T1.1)
- `pkg/script/runner.go:12-38` — `Init` migrates `protect bool` arg to set the new flag (T1.2)
- `pkg/script/handlers_player.go` — migrate gates (T1.3, T2 rewrite of handleFindUID/handlePFindUID)
- `pkg/script/handlers_inv.go` — migrate ~18 protect-gate sites (T1.4); add `handleBothMoveInv` (T3.1)
- `pkg/script/handlers_vars.go:69` — migrate one site (T1.4)
- `pkg/script/handlers.go:307` neighborhood — register `OpBothMoveInv` dispatch (T3.1)
- `modules/world/player_script.go:277, 300, 303, 716` — migrate three sites + doc comments (T1.5)
- `pkg/script/handlers_player_test.go`, `handlers_inv_test.go`, `runner_test.go`, `handlers_vars_test.go` — migrate ~16 test fixture sites (T1.6); add T2/T3 RED→GREEN test bodies
- Memory files under `/home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/` — update `nai_followups.md` at T4 close

---

## Task 1 — Pointer-flag refactor (`Protect bool` → `PtrProtectedActivePlayer{,2}`)

**Files:**
- Modify: `pkg/script/pointer.go` (add constants)
- Modify: `pkg/script/state.go:315` (delete field)
- Modify: `pkg/script/runner.go:12-38` (migrate `Init`)
- Modify: `pkg/script/handlers_player.go:52-66, 912-933` (gate + handlePFindUID set-site)
- Modify: `pkg/script/handlers_inv.go` (18 gate sites + 1 doc comment)
- Modify: `pkg/script/handlers_vars.go:69`
- Modify: `modules/world/player_script.go:277, 300, 303, 716`
- Modify: `pkg/script/handlers_player_test.go`, `handlers_inv_test.go`, `runner_test.go`, `handlers_vars_test.go` (migrate fixtures)

**Goal:** Replace `s.Protect bool` with a pointer-bitmask flag. T1 ships **no behavior change** — every existing test must pass after the migration. Adds `PtrProtectedActivePlayer2` for slot 1 (consumed by T2/T3 below) and a new `requireProtectedActivePlayer2` helper.

- [ ] **Step 1.1 — Pre-flight: grep all references**

Run these greps; they will be the implementer's enumerate-list per `enumerate_all_sites.md`:

```bash
rg '\bs\.Protect\b' pkg/script/ modules/world/
rg '\bstate\.Protect\b' pkg/script/ modules/world/
rg '\.Protect\s*=' pkg/script/ modules/world/ | grep -v '\.invs\[' | grep -v 'mainInv\|bankInv\|invType\.\|certNote\.\|sword\.\|coins\|arrow\|logs\|ScriptVarTypeInt'
rg 'p\.activeScript\.Protect\b' modules/world/
```

Expected hits (these are the migration sites):

| Path | Line | Kind |
|---|---|---|
| `pkg/script/runner.go` | 27 | `Protect: protect` (Init field literal) |
| `pkg/script/state.go` | 315 | `Protect bool` (field declaration) |
| `pkg/script/handlers_player.go` | 62 | `if !s.Protect` (read in `requireProtectedActivePlayer`) |
| `pkg/script/handlers_player.go` | 915 | `s.Protect && ...` (read in `handlePFindUID` fast-path; T2 will rewrite) |
| `pkg/script/handlers_player.go` | 930 | `s.Protect = true` (write in `handlePFindUID` success; T2 will rewrite) |
| `pkg/script/handlers_inv.go` | 341, 431, 460, 502, 533, 583, 587, 639, 642, 896, 900, 958, 1026, 1030, 1106, 1110, 1180 | `&& !s.Protect` (17 gate reads) |
| `pkg/script/handlers_inv.go` | 760 | doc-comment only (`s.Protect via requireProtectedActivePlayer`) |
| `pkg/script/handlers_vars.go` | 69 | `if protect && !s.Protect` |
| `modules/world/player_script.go` | 277, 300 | doc-comment text (`activeScript.Protect`) |
| `modules/world/player_script.go` | 303 | `p.activeScript != nil && p.activeScript.Protect` (read) |
| `modules/world/player_script.go` | 716 | `p.activeScript.Protect = false` (write) |
| `pkg/script/runner_test.go` | 57 | `if s.Protect != true` (test pin) |
| `pkg/script/handlers_inv_test.go` | 853, 901, 998 | `s.Protect = true` (setters) |
| `pkg/script/handlers_inv_test.go` | 930, 960 | `s.Protect = false` (clearers — drop the lines) |
| `pkg/script/handlers_inv_test.go` | 184, 185, 950 | doc-comment text |
| `pkg/script/handlers_vars_test.go` | 489 | doc-comment text |
| `pkg/script/handlers_player_test.go` | 1333, 1488 | `if state.Protect` (negative pins) |
| `pkg/script/handlers_player_test.go` | 1433, 1462, 1539 | `if !state.Protect` (positive pins) |
| `pkg/script/handlers_player_test.go` | 3650, 3682, 3702 | `s.Protect = true` (setters) |
| `pkg/script/handlers_player_test.go` | 3725, 3737 | `s.Protect = false` (clearer) + doc text |

**IMPORTANT — false-positive filter:** `objtype.InvType.Protect`, `objtype.VarpType.Protect`, `mc.invs[...].Protect`, `mainInv.Protect`, `bankInv.Protect`, `invType.Protect`, `Protect: true` in `objtype.NewInvType` defaults — these are config-type fields, NOT `ScriptState.Protect`. **Do not migrate them.** Distinguish by file context: any `Protect` ref where the LHS/owner is `*ScriptState`, `*objtype.InvType`, `*objtype.VarpType` → only migrate `*ScriptState` references.

- [ ] **Step 1.2 — Add `PtrProtectedActivePlayer{,2}` constants**

Edit `pkg/script/pointer.go` — replace existing `const (...)` block:

```go
const (
	PtrActivePlayer  Pointer = 1 << 0
	PtrActivePlayer2 Pointer = 1 << 1
	PtrActiveNpc     Pointer = 1 << 2
	PtrActiveNpc2    Pointer = 1 << 3
	PtrActiveLoc     Pointer = 1 << 4
	PtrActiveLoc2    Pointer = 1 << 5
	PtrActiveObj     Pointer = 1 << 6
	PtrActiveObj2    Pointer = 1 << 7
	PtrFindDb        Pointer = 1 << 8 // S7g: DB_FIND* / DB_LISTALL* set; DB_FINDNEXT / DB_FIND_REFINE require.

	// PtrProtectedActivePlayer is the slot-0 protect flag — TS
	// ProtectedActivePlayer (ScriptPointer.ts:10). Set by `Init` when
	// `protect=true` and `self != nil`, by P_FINDUID success on
	// intOperand=0, and cleared by Player.CloseModal. NAI-133.
	PtrProtectedActivePlayer Pointer = 1 << 9

	// PtrProtectedActivePlayer2 is the slot-1 protect flag — TS
	// ProtectedActivePlayer2 (ScriptPointer.ts:11). Set ONLY by
	// P_FINDUID success on intOperand=1; TS never sets this from the
	// engine. NAI-133.
	PtrProtectedActivePlayer2 Pointer = 1 << 10
)
```

- [ ] **Step 1.3 — Delete `Protect bool` field from ScriptState**

Edit `pkg/script/state.go:315` — remove the line `Protect bool` (the empty line above can stay).

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...
```

Expected: many compile errors at the migration sites listed in Step 1.1. This is the workload boundary for Steps 1.4–1.7.

- [ ] **Step 1.4 — Migrate `runner.Init`**

Edit `pkg/script/runner.go:12-38`. Replace the function body:

```go
func Init(script *ScriptFile, self ActivePlayer, protect bool, intArgs []int, stringArgs []string) *ScriptState {
	s := &ScriptState{
		Script:    script,
		PC:        0,
		Execution: Running,

		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),

		IntLocals:    make([]int, max(int(script.IntLocalCount), len(intArgs))),
		StringLocals: make([]string, max(int(script.StringLocalCount), len(stringArgs))),

		Frames: make([]Frame, FrameCapacity),

		Self: self,
	}

	copy(s.IntLocals, intArgs)
	copy(s.StringLocals, stringArgs)

	if self != nil {
		s.Pointers |= PtrActivePlayer
		if protect {
			s.Pointers |= PtrProtectedActivePlayer
		}
	}

	return s
}
```

Note: `protect=true` with `self=nil` now silently drops the flag (was: set unconditionally). Audit for callers passing `nil` self with `protect=true`:

```bash
rg 'script\.Init\([^,]*,\s*nil\s*,\s*true' --type go
```

Expected: zero hits. (Verified at spec-write — `npc_script.go:318` uses `nil, false`.)

- [ ] **Step 1.5 — Migrate `requireProtectedActivePlayer` + add `requireProtectedActivePlayer2`**

Edit `pkg/script/handlers_player.go:52-66`. Replace the existing `requireProtectedActivePlayer` and append `requireProtectedActivePlayer2`:

```go
// requireProtectedActivePlayer is requireActivePlayer plus a check that
// the script holds the slot-0 protect flag (PtrProtectedActivePlayer).
// Used by opcodes that TS wraps in checkedHandler(ProtectedActivePlayer, ...)
// at intOperand=0. Chains through requireActivePlayer first so the
// "no active player" error message matches the unprotected variant.
func requireProtectedActivePlayer(s *ScriptState, op string) error {
	if err := requireActivePlayer(s, op); err != nil {
		return err
	}
	if s.Pointers&PtrProtectedActivePlayer == 0 {
		return errors.New(op + ": script not protected")
	}
	return nil
}

// requireProtectedActivePlayer2 is the slot-1 analogue of
// requireProtectedActivePlayer. Chains through requireActivePlayer2 first
// so error messages match the unprotected variant. Currently consumed
// only by BOTH_MOVEINV's secondary branch. NAI-133.
func requireProtectedActivePlayer2(s *ScriptState, op string) error {
	if err := requireActivePlayer2(s, op); err != nil {
		return err
	}
	if s.Pointers&PtrProtectedActivePlayer2 == 0 {
		return errors.New(op + ": script not protected")
	}
	return nil
}
```

Defer `handlePFindUID` rewrite to T2 — for now, fix the compile error by replacing `s.Protect` reads/writes mechanically (T1 keeps slot-0 semantics):

`handlers_player.go:915`: change

```go
if s.Protect && s.Self != nil && s.Self.UID() == uid {
```

to

```go
if s.Pointers&PtrProtectedActivePlayer != 0 && s.Self != nil && s.Self.UID() == uid {
```

`handlers_player.go:930`: change

```go
s.Protect = true
```

to

```go
s.Pointers |= PtrProtectedActivePlayer
```

- [ ] **Step 1.6 — Migrate `handlers_inv.go` gate sites (17 reads + 1 doc-comment)**

For each line in `pkg/script/handlers_inv.go` at the listed line numbers (341, 431, 460, 502, 533, 583, 587, 639, 642, 896, 900, 958, 1026, 1030, 1106, 1110, 1180), apply the same mechanical replacement:

Before:
```go
if invType.Protect && invType.Scope != objtype.InvTypeScopeShared && !s.Protect {
```

After:
```go
if invType.Protect && invType.Scope != objtype.InvTypeScopeShared && s.Pointers&PtrProtectedActivePlayer == 0 {
```

(Some sites use `fromInvType.Protect` / `toInvType.Protect` — preserve the LHS variable; only migrate the trailing `!s.Protect`.)

Then update doc-comment at `handlers_inv.go:760`:

Before:
```go
// invType.Scope is not InvTypeScopeShared, require s.Protect via
// requireProtectedActivePlayer. Otherwise require only ActivePlayer.
```

After:
```go
// invType.Scope is not InvTypeScopeShared, require PtrProtectedActivePlayer
// via requireProtectedActivePlayer. Otherwise require only ActivePlayer.
```

- [ ] **Step 1.7 — Migrate `handlers_vars.go:69`**

Edit `pkg/script/handlers_vars.go:69`:

Before:
```go
if protect && !s.Protect {
```

After:
```go
if protect && s.Pointers&PtrProtectedActivePlayer == 0 {
```

- [ ] **Step 1.8 — Migrate `modules/world/player_script.go`**

Edit `modules/world/player_script.go`:

Line 277 doc-comment (`activeScript.Protect`) → `activeScript.Pointers&PtrProtectedActivePlayer`.

Line 300 doc-comment (`activeScript.Protect ↔ TS Player.protect equivalence`) → `activeScript.Pointers&PtrProtectedActivePlayer ↔ TS Player.protect equivalence`.

Line 303 (read):

Before:
```go
return p.activeScript != nil && p.activeScript.Protect
```

After:
```go
return p.activeScript != nil && p.activeScript.Pointers&script.PtrProtectedActivePlayer != 0
```

Line 716 (write):

Before:
```go
p.activeScript.Protect = false
```

After:
```go
p.activeScript.Pointers &^= script.PtrProtectedActivePlayer
```

Verify the `script` import is already present (it is — `protectedScriptActive` and `Init` calls earlier in the same file already use it).

- [ ] **Step 1.9 — Migrate test fixtures**

Pattern: every `state.Protect = true` or `s.Protect = true` becomes `state.Pointers |= PtrProtectedActivePlayer` / `s.Pointers |= PtrProtectedActivePlayer`. Every `state.Protect = false` or `s.Protect = false` is dropped (the zero-value `Pointers` already lacks the flag, so an explicit clear is redundant unless the test pre-set it). Every `if state.Protect` / `if !state.Protect` test assertion becomes `if state.Pointers&PtrProtectedActivePlayer != 0` / `if state.Pointers&PtrProtectedActivePlayer == 0`.

**`pkg/script/runner_test.go:57`** — the `Init` test:

Before:
```go
if s.Protect != true {
	t.Errorf("Protect: got %v, want true", s.Protect)
}
```

After:
```go
if s.Pointers&PtrProtectedActivePlayer == 0 {
	t.Errorf("Protect: PtrProtectedActivePlayer should be set, pointers=%b", s.Pointers)
}
```

**`pkg/script/handlers_inv_test.go`:**

Line 853, 901, 998 — all `s.Protect = true` → `s.Pointers |= PtrProtectedActivePlayer`.

Line 930 — drop the line `s.Protect = false // not protected — protect gate must fire`. Replace with comment:
```go
// not protected — protect gate must fire (Pointers zero-value lacks PtrProtectedActivePlayer)
```

Line 960 — drop the line `s.Protect = false // not protected`. Replace with comment as above.

Line 184-185 doc-comment update:
Before:
```go
// dummyitem) use this helper. s.Protect remains false (Init's third
// arg) — tests that need a protected script set state.Protect = true
```
After:
```go
// dummyitem) use this helper. PtrProtectedActivePlayer remains unset
// (Init's third arg) — tests that need a protected script set
// state.Pointers |= PtrProtectedActivePlayer
```

Line 950 doc-comment update:
Before:
```go
t.Errorf("INV_DROPSLOT protect-required without s.Protect: expected error, got nil")
```
After:
```go
t.Errorf("INV_DROPSLOT protect-required without PtrProtectedActivePlayer: expected error, got nil")
```

**`pkg/script/handlers_vars_test.go:489`** — doc-comment update:

Before:
```go
// Confirm Protect=false varps don't gate even when state.Protect=false.
```
After:
```go
// Confirm Protect=false varps don't gate even when PtrProtectedActivePlayer is unset.
```

**`pkg/script/handlers_player_test.go:1333` (TestFindUIDFound):**

Before:
```go
if state.Protect {
	t.Errorf("Protect should remain false for FINDUID")
}
```
After:
```go
if state.Pointers&PtrProtectedActivePlayer != 0 {
	t.Errorf("PtrProtectedActivePlayer should remain unset for FINDUID, pointers=%b", state.Pointers)
}
```

**`pkg/script/handlers_player_test.go:1433` (TestPFindUIDSelfReacquire):**

Before:
```go
if !state.Protect {
	t.Errorf("Protect should remain true")
}
```
After:
```go
if state.Pointers&PtrProtectedActivePlayer == 0 {
	t.Errorf("PtrProtectedActivePlayer should remain set, pointers=%b", state.Pointers)
}
```

**`pkg/script/handlers_player_test.go:1462` (TestPFindUIDFoundCanAccess):** same `!state.Protect` migration as line 1433.

**`pkg/script/handlers_player_test.go:1488` (TestPFindUIDFoundCannotAccess):** same `if state.Protect` migration as line 1333.

**`pkg/script/handlers_player_test.go:1539`:** same `!state.Protect` migration.

**`pkg/script/handlers_player_test.go:3650, 3682, 3702`:** all `s.Protect = true` → `s.Pointers |= PtrProtectedActivePlayer`.

**`pkg/script/handlers_player_test.go:3725`:** drop the line `s.Protect = false // not protected — gate must fire`. Replace with a one-line comment.

**`pkg/script/handlers_player_test.go:3737`:** doc-comment text `without s.Protect` → `without PtrProtectedActivePlayer`.

- [ ] **Step 1.10 — Verify build + full test green**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/... ./modules/world/... -count=1
```

Expected: build succeeds, all existing tests pass. No new tests in T1 — this is a behavior-preserving refactor.

- [ ] **Step 1.11 — Re-grep to confirm no `s.Protect` remains**

```bash
rg '\bProtect\b' pkg/script/runner.go pkg/script/state.go pkg/script/handlers_player.go pkg/script/handlers_inv.go pkg/script/handlers_vars.go modules/world/player_script.go
```

Expected: only references should be in doc-comments that mention "protect" or `objtype.InvType.Protect`-shaped config-field references. No `*ScriptState.Protect` field reads/writes remain.

```bash
rg '\bs\.Protect\b|\bstate\.Protect\b|p\.activeScript\.Protect\b' pkg/script/ modules/world/
```

Expected: zero hits.

- [ ] **Step 1.12 — Commit**

```bash
git add pkg/script/pointer.go pkg/script/state.go pkg/script/runner.go pkg/script/handlers_player.go pkg/script/handlers_inv.go pkg/script/handlers_vars.go modules/world/player_script.go pkg/script/runner_test.go pkg/script/handlers_inv_test.go pkg/script/handlers_vars_test.go pkg/script/handlers_player_test.go
git commit --no-gpg-sign -m "refactor(nai-133): T1 — Protect bool → PtrProtectedActivePlayer{,2} pointer flags

Reify s.Protect as Pointer-bitmask flags (PtrProtectedActivePlayer = 1<<9, PtrProtectedActivePlayer2 = 1<<10). Behavior-preserving — slot-0 routing unchanged. New requireProtectedActivePlayer2 helper for T2/T3 consumption. ~30 production sites + ~16 test fixtures migrated."
```

---

## Task 2 — FINDUID + P_FINDUID slot routing on `intOperand`

**Files:**
- Modify: `pkg/script/handlers_player.go:885-933` (rewrite both handlers)
- Test: `pkg/script/handlers_player_test.go` (append T2 test bodies)

**Goal:** Both opcodes read `s.Script.IntOperands[s.PC]`. `intOperand=0` binds Self (current behavior); `intOperand=1` binds Self2. P_FINDUID also sets the slot's protect flag. Closes a latent `.p_finduid` / `.finduid` clobber bug — current code always writes Self regardless of intOperand.

- [ ] **Step 2.1 — Write failing RED tests**

Append to `pkg/script/handlers_player_test.go` after the existing P_FINDUID tests (after `TestPFindUIDNotFound`, around line 1500+):

```go
// -- NAI-133 T2: FINDUID/P_FINDUID slot-1 routing --

// finduidSlotOp builds a one-instruction ScriptFile with the requested
// intOperand value (0 or 1). Sister to newSingleOp which always uses 0.
func finduidSlotOp(name string, op Opcode, operand int32) *ScriptFile {
	return &ScriptFile{
		Name:             name,
		Opcodes:          []Opcode{op, OpReturn},
		IntOperands:      []int32{operand, 0},
		StringOperands:   []string{"", ""},
		InstructionCount: 2,
	}
}

// TestFindUID_Slot1_BindsSelf2 — operand=1, lookup hits → Self2 set,
// PtrActivePlayer2 set, Self UNTOUCHED. NAI-133 T2 closes the latent
// `.finduid` clobber bug.
func TestFindUID_Slot1_BindsSelf2(t *testing.T) {
	target := &mockPlayer{username: "Target", uidValue: 99}
	origSelf := &mockPlayer{username: "Orig", uidValue: 1}
	lookup := &mockPlayerLookup{byUID: map[int]ActivePlayer{99: target}}

	sf := finduidSlotOp("finduid_slot1", OpFindUID, 1)
	state := Init(sf, origSelf, false, nil, nil)
	state.PlayerLookup = lookup
	state.PushInt(99)

	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if state.ISP != 1 || state.IntStack[0] != 1 {
		t.Errorf("stack: got [%v], want [1]", state.IntStack[:state.ISP])
	}
	if state.Self != origSelf {
		t.Errorf("Self should be UNCHANGED on slot-1 routing, got %v", state.Self)
	}
	if state.Self2 != target {
		t.Errorf("Self2: got %v, want target", state.Self2)
	}
	if state.Pointers&PtrActivePlayer2 == 0 {
		t.Errorf("PtrActivePlayer2 should be set, pointers=%b", state.Pointers)
	}
}

// TestFindUID_Slot1_LookupMiss — operand=1, lookup miss → push 0,
// no state change.
func TestFindUID_Slot1_LookupMiss(t *testing.T) {
	origSelf := &mockPlayer{username: "Orig", uidValue: 1}
	lookup := &mockPlayerLookup{byUID: map[int]ActivePlayer{}}

	sf := finduidSlotOp("finduid_slot1_miss", OpFindUID, 1)
	state := Init(sf, origSelf, false, nil, nil)
	state.PlayerLookup = lookup
	state.PushInt(999)

	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if state.ISP != 1 || state.IntStack[0] != 0 {
		t.Errorf("stack: got [%v], want [0]", state.IntStack[:state.ISP])
	}
	if state.Self2 != nil {
		t.Errorf("Self2 should remain nil on miss, got %v", state.Self2)
	}
	if state.Pointers&PtrActivePlayer2 != 0 {
		t.Errorf("PtrActivePlayer2 should remain unset on miss, pointers=%b", state.Pointers)
	}
}

// TestFindUID_InvalidOperand_Errors — operand=2 → error.
func TestFindUID_InvalidOperand_Errors(t *testing.T) {
	origSelf := &mockPlayer{username: "Orig", uidValue: 1}
	lookup := &mockPlayerLookup{byUID: map[int]ActivePlayer{}}

	sf := finduidSlotOp("finduid_bad", OpFindUID, 2)
	state := Init(sf, origSelf, false, nil, nil)
	state.PlayerLookup = lookup
	state.PushInt(1)

	err := Execute(state)
	if err == nil {
		t.Fatalf("expected error on intOperand=2, got nil")
	}
	if !strings.Contains(err.Error(), "FINDUID: invalid intOperand 2") {
		t.Errorf("err message: got %q, want containing %q", err.Error(), "FINDUID: invalid intOperand 2")
	}
}

// TestPFindUID_Slot1_Success — operand=1, lookup hits + CanAccess=true →
// Self2 set, PtrActivePlayer2 + PtrProtectedActivePlayer2 set, push 1.
func TestPFindUID_Slot1_Success(t *testing.T) {
	target := &mockPlayer{username: "Target", uidValue: 99, canAccess: true}
	origSelf := &mockPlayer{username: "Orig", uidValue: 1}
	lookup := &mockPlayerLookup{byUID: map[int]ActivePlayer{99: target}}

	sf := finduidSlotOp("pfinduid_slot1", OpPFindUID, 1)
	state := Init(sf, origSelf, false, nil, nil)
	state.PlayerLookup = lookup
	state.PushInt(99)

	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if state.ISP != 1 || state.IntStack[0] != 1 {
		t.Errorf("stack: got [%v], want [1]", state.IntStack[:state.ISP])
	}
	if state.Self != origSelf {
		t.Errorf("Self UNCHANGED: got %v, want %v", state.Self, origSelf)
	}
	if state.Self2 != target {
		t.Errorf("Self2: got %v, want target", state.Self2)
	}
	if state.Pointers&PtrActivePlayer2 == 0 {
		t.Errorf("PtrActivePlayer2 should be set, pointers=%b", state.Pointers)
	}
	if state.Pointers&PtrProtectedActivePlayer2 == 0 {
		t.Errorf("PtrProtectedActivePlayer2 should be set, pointers=%b", state.Pointers)
	}
}

// TestPFindUID_Slot1_SelfReacquire — slot-1 fast-path: Self2 already
// bound + PtrProtectedActivePlayer2 set + popped uid == Self2.UID() →
// push 1, no state mutation, no lookup call.
func TestPFindUID_Slot1_SelfReacquire(t *testing.T) {
	self2 := &mockPlayer{username: "Self2", uidValue: 42}
	origSelf := &mockPlayer{username: "Orig", uidValue: 1}
	lookup := &mockPlayerLookup{byUID: map[int]ActivePlayer{}}

	sf := finduidSlotOp("pfinduid_slot1_reacquire", OpPFindUID, 1)
	state := Init(sf, origSelf, false, nil, nil)
	state.Self2 = self2
	state.Pointers |= PtrActivePlayer2 | PtrProtectedActivePlayer2
	state.PlayerLookup = lookup
	state.PushInt(42)

	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if state.ISP != 1 || state.IntStack[0] != 1 {
		t.Errorf("stack: got [%v], want [1]", state.IntStack[:state.ISP])
	}
	if state.Self2 != self2 {
		t.Errorf("Self2 should remain unchanged on fast-path")
	}
	if lookup.calls != 0 {
		t.Errorf("fast-path should skip lookup, calls=%d", lookup.calls)
	}
}

// TestPFindUID_Slot0_NoFastPathWhenSlot1Protected — only the matching
// slot's protect flag triggers the fast-path. Slot-0 P_FINDUID with
// PtrProtectedActivePlayer2 set (but PtrProtectedActivePlayer UNSET)
// must NOT fast-path; it must perform a real lookup.
func TestPFindUID_Slot0_NoFastPathWhenSlot1Protected(t *testing.T) {
	self := &mockPlayer{username: "Self", uidValue: 42, canAccess: true}
	target := &mockPlayer{username: "Target", uidValue: 42, canAccess: true}
	lookup := &mockPlayerLookup{byUID: map[int]ActivePlayer{42: target}}

	sf := finduidSlotOp("pfinduid_slot0_no_cross", OpPFindUID, 0)
	state := Init(sf, self, false, nil, nil) // protect=false: slot-0 flag UNSET
	state.Pointers |= PtrProtectedActivePlayer2 // slot-1 protected (irrelevant)
	state.PlayerLookup = lookup
	state.PushInt(42)

	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if lookup.calls != 1 {
		t.Errorf("expected real lookup, calls=%d (fast-path leaked from slot-1)", lookup.calls)
	}
	// Slot-0 protect flag set after success.
	if state.Pointers&PtrProtectedActivePlayer == 0 {
		t.Errorf("PtrProtectedActivePlayer should be set after success, pointers=%b", state.Pointers)
	}
}

// TestPFindUID_Slot1_LookupMiss — operand=1, lookup miss → push 0.
func TestPFindUID_Slot1_LookupMiss(t *testing.T) {
	origSelf := &mockPlayer{username: "Orig", uidValue: 1}
	lookup := &mockPlayerLookup{byUID: map[int]ActivePlayer{}}

	sf := finduidSlotOp("pfinduid_slot1_miss", OpPFindUID, 1)
	state := Init(sf, origSelf, false, nil, nil)
	state.PlayerLookup = lookup
	state.PushInt(999)

	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if state.ISP != 1 || state.IntStack[0] != 0 {
		t.Errorf("stack: got [%v], want [0]", state.IntStack[:state.ISP])
	}
	if state.Self2 != nil {
		t.Errorf("Self2 should remain nil on miss")
	}
	if state.Pointers&PtrProtectedActivePlayer2 != 0 {
		t.Errorf("PtrProtectedActivePlayer2 should remain unset on miss")
	}
}

// TestPFindUID_Slot1_CanAccessFalse — operand=1, lookup hits but
// CanAccess=false → push 0, no state change.
func TestPFindUID_Slot1_CanAccessFalse(t *testing.T) {
	target := &mockPlayer{username: "Target", uidValue: 99, canAccess: false}
	origSelf := &mockPlayer{username: "Orig", uidValue: 1}
	lookup := &mockPlayerLookup{byUID: map[int]ActivePlayer{99: target}}

	sf := finduidSlotOp("pfinduid_slot1_no_access", OpPFindUID, 1)
	state := Init(sf, origSelf, false, nil, nil)
	state.PlayerLookup = lookup
	state.PushInt(99)

	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if state.ISP != 1 || state.IntStack[0] != 0 {
		t.Errorf("stack: got [%v], want [0]", state.IntStack[:state.ISP])
	}
	if state.Self2 != nil {
		t.Errorf("Self2 should remain nil when CanAccess=false")
	}
}

// TestPFindUID_InvalidOperand_Errors — operand=-1 → error.
func TestPFindUID_InvalidOperand_Errors(t *testing.T) {
	origSelf := &mockPlayer{username: "Orig", uidValue: 1}
	lookup := &mockPlayerLookup{byUID: map[int]ActivePlayer{}}

	sf := finduidSlotOp("pfinduid_bad", OpPFindUID, -1)
	state := Init(sf, origSelf, false, nil, nil)
	state.PlayerLookup = lookup
	state.PushInt(1)

	err := Execute(state)
	if err == nil {
		t.Fatalf("expected error on intOperand=-1, got nil")
	}
	if !strings.Contains(err.Error(), "P_FINDUID: invalid intOperand -1") {
		t.Errorf("err message: got %q", err.Error())
	}
}
```

**Pre-flight check** — `mockPlayer` struct must already have `canAccess bool` and `uidValue int` fields plus `CanAccess()` / `UID()` methods. Verify:

```bash
rg 'canAccess|uidValue' pkg/script/runner_test.go pkg/script/handlers_player_test.go | head -10
```

Expected: at least one definition + multiple consumers. (NAI-30+ tests already use these.)

- [ ] **Step 2.2 — Run tests, verify RED**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run 'TestFindUID_Slot1|TestPFindUID_Slot1|TestPFindUID_Slot0_NoFastPath|TestFindUID_InvalidOperand|TestPFindUID_InvalidOperand' -count=1 -v
```

Expected: all 8 new tests FAIL — current `handleFindUID` ignores intOperand and always writes Self; `handlePFindUID` likewise.

- [ ] **Step 2.3 — Implement `handleFindUID` slot routing**

Edit `pkg/script/handlers_player.go:885-900`. Replace the function body:

```go
// handleFindUID resolves the popped uid via PlayerLookup and binds it
// to the slot selected by intOperand: 0 → Self + PtrActivePlayer,
// 1 → Self2 + PtrActivePlayer2. Pushes 1 on success, 0 on miss /
// nil-PlayerLookup. Errors on invalid intOperand. Mirrors TS
// PlayerOps.ts:60-72 with goscape's collapsed pointer model. NAI-133.
func handleFindUID(s *ScriptState) error {
	operand := s.Script.IntOperands[s.PC]
	if operand != 0 && operand != 1 {
		return fmt.Errorf("FINDUID: invalid intOperand %d", operand)
	}
	uid := s.PopInt()
	if s.PlayerLookup == nil {
		s.PushInt(0)
		return nil
	}
	target := s.PlayerLookup.LookupPlayerByUID(uid)
	if target == nil {
		s.PushInt(0)
		return nil
	}
	if operand == 0 {
		s.Self = target
		s.Pointers |= PtrActivePlayer
	} else {
		s.Self2 = target
		s.Pointers |= PtrActivePlayer2
	}
	s.PushInt(1)
	return nil
}
```

- [ ] **Step 2.4 — Implement `handlePFindUID` slot routing**

Edit `pkg/script/handlers_player.go:912-933`. Replace the function body:

```go
// handlePFindUID is P_FINDUID — the protected variant of FINDUID. Pops
// a uid, tries to rebind the slot selected by intOperand with protected
// access. Three outcomes per slot:
//   - Self-reacquire fast-path: script already runs protected on a
//     player whose UID matches → push 1, no state change, no lookup.
//   - Lookup miss OR target.CanAccess()==false → push 0.
//   - Success → slot rebinds, both PtrActivePlayer{,2} and
//     PtrProtectedActivePlayer{,2} flags set, push 1.
//
// Mirrors TS PlayerOps.ts:75-94. NAI-133 added intOperand-based slot
// routing (closes latent `.p_finduid` clobber bug).
func handlePFindUID(s *ScriptState) error {
	operand := s.Script.IntOperands[s.PC]
	if operand != 0 && operand != 1 {
		return fmt.Errorf("P_FINDUID: invalid intOperand %d", operand)
	}
	uid := s.PopInt()

	// Self-reacquire fast-path: already protected on this slot's player.
	if operand == 0 {
		if s.Pointers&PtrProtectedActivePlayer != 0 && s.Self != nil && s.Self.UID() == uid {
			s.PushInt(1)
			return nil
		}
	} else {
		if s.Pointers&PtrProtectedActivePlayer2 != 0 && s.Self2 != nil && s.Self2.UID() == uid {
			s.PushInt(1)
			return nil
		}
	}

	if s.PlayerLookup == nil {
		s.PushInt(0)
		return nil
	}
	target := s.PlayerLookup.LookupPlayerByUID(uid)
	if target == nil || !target.CanAccess() {
		s.PushInt(0)
		return nil
	}

	if operand == 0 {
		s.Self = target
		s.Pointers |= PtrActivePlayer | PtrProtectedActivePlayer
	} else {
		s.Self2 = target
		s.Pointers |= PtrActivePlayer2 | PtrProtectedActivePlayer2
	}
	s.PushInt(1)
	return nil
}
```

- [ ] **Step 2.5 — Run new tests, verify GREEN**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run 'TestFindUID_Slot1|TestPFindUID_Slot1|TestPFindUID_Slot0_NoFastPath|TestFindUID_InvalidOperand|TestPFindUID_InvalidOperand' -count=1 -v
```

Expected: all 8 new tests PASS.

- [ ] **Step 2.6 — Run full pkg/script test suite, verify no regressions**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/... ./modules/world/... -count=1
```

Expected: GREEN (existing FINDUID/P_FINDUID tests still pass — they use intOperand=0 by default via `newSingleOp`).

- [ ] **Step 2.7 — Commit**

```bash
git add pkg/script/handlers_player.go pkg/script/handlers_player_test.go
git commit --no-gpg-sign -m "feat(nai-133): T2 — FINDUID/P_FINDUID intOperand slot routing (GREEN)

intOperand=0 binds Self + PtrActivePlayer (and PtrProtectedActivePlayer for P_FINDUID success). intOperand=1 binds Self2 + PtrActivePlayer2 (and PtrProtectedActivePlayer2 for P_FINDUID success). Closes a latent .p_finduid / .finduid clobber bug — pre-NAI-133 these always wrote Self regardless of intOperand. Mirrors TS PlayerOps.ts:60-72 + 75-94."
```

---

## Task 3 — `handleBothMoveInv` (opcode 4301)

**Files:**
- Modify: `pkg/script/handlers_inv.go` (append `handleBothMoveInv`)
- Modify: `pkg/script/handlers.go:307` neighborhood (register dispatch)
- Test: `pkg/script/handlers_inv_test.go` (append T3 tests)

**Goal:** Port TS `InvOps.ts:373-495`. Drains `fromInv` of `fromPlayer` into `toInv` of `toPlayer`, with overflow drops to `toPlayer`'s tile. `intOperand=1` swaps Self/Self2 roles. Skip wealth-event tail per NAI-115-D1 reuse. Preserve TS quirk: to-gate evaluates `fromInvType.Scope`.

- [ ] **Step 3.1 — Pre-flight: verify dependent APIs unchanged**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go doc -all ./pkg/inventory/ Inventory | head -40
```

Confirm:
- `Inventory.Capacity` is a **field** (no parens needed)
- `(*Inventory).Get(slot int) *Item`
- `(*Inventory).Delete(slot int)` exists
- `(*Inventory).Add(id, count int, opts AddOpts) inventory.AddTx` (or whatever signature — implementer reads handlers_inv.go:357 for the working invocation pattern)

```bash
rg 'WorldVars\b|AddObj\(' pkg/script/state.go | head -5
```

Confirm `AddObj(level, x, z, typeID, count, duration, receiverID int) ActiveObj` signature.

- [ ] **Step 3.2 — Write failing RED tests for the BOTH_MOVEINV handler**

Append to `pkg/script/handlers_inv_test.go`. The new tests need a sister `runInvOpWithOperandAsBothPlayers` helper that constructs a state with both `Self` and `Self2` bound. Append the helper first, then the tests:

```go
// -- NAI-133 T3: BOTH_MOVEINV tests --

// runBothMoveInv executes OpBothMoveInv with the given intOperand against
// a state pre-bound with Self + Self2. intInputs are pushed in order
// (matching the TS popInts(2) order: from on bottom, to on top).
// Returns the post-execution state.
func runBothMoveInv(t *testing.T, operand int32, intInputs []int, lookup InvLookup, configs Configs, world WorldVars, self, self2 *mockPlayer, slot1Protected bool) *ScriptState {
	t.Helper()
	sf := &ScriptFile{
		Name:             "test_BOTH_MOVEINV",
		Opcodes:          []Opcode{OpBothMoveInv, OpReturn},
		IntOperands:      []int32{operand, 0},
		StringOperands:   []string{"", ""},
		InstructionCount: 2,
	}
	state := Init(sf, self, true, nil, nil) // slot-0 protected
	state.Self2 = self2
	state.Pointers |= PtrActivePlayer2
	if slot1Protected {
		state.Pointers |= PtrProtectedActivePlayer2
	}
	state.Inv = lookup
	state.Configs = configs
	state.World = world
	for _, v := range intInputs {
		state.PushInt(v)
	}
	if err := Execute(state); err != nil {
		t.Fatalf("BOTH_MOVEINV: unexpected error: %v", err)
	}
	return state
}

// runBothMoveInvExpectErr is the error variant.
func runBothMoveInvExpectErr(t *testing.T, operand int32, intInputs []int, lookup InvLookup, configs Configs, world WorldVars, self, self2 ActivePlayer, slot0Protected, slot1Protected bool, substr string) {
	t.Helper()
	sf := &ScriptFile{
		Name:             "test_BOTH_MOVEINV",
		Opcodes:          []Opcode{OpBothMoveInv, OpReturn},
		IntOperands:      []int32{operand, 0},
		StringOperands:   []string{"", ""},
		InstructionCount: 2,
	}
	state := Init(sf, self, slot0Protected, nil, nil)
	if self2 != nil {
		state.Self2 = self2
		state.Pointers |= PtrActivePlayer2
	}
	if slot1Protected {
		state.Pointers |= PtrProtectedActivePlayer2
	}
	state.Inv = lookup
	state.Configs = configs
	state.World = world
	for _, v := range intInputs {
		state.PushInt(v)
	}
	err := Execute(state)
	if err == nil {
		t.Fatalf("BOTH_MOVEINV: expected error containing %q, got nil", substr)
	}
	if !strings.Contains(err.Error(), substr) {
		t.Fatalf("BOTH_MOVEINV: expected error containing %q, got %q", substr, err.Error())
	}
}

// twoPlayerInvLookup routes Get(player, typeID) to one of two per-player
// inventory maps based on the receiver pointer. Tests use this to give
// Self and Self2 distinct main/bank inventories. Other player addresses
// return nil.
type twoPlayerInvLookup struct {
	self     *mockPlayer
	self2    *mockPlayer
	selfInvs  map[int]*inventory.Inventory
	self2Invs map[int]*inventory.Inventory
}

func (m *twoPlayerInvLookup) Get(p ActivePlayer, typeID int) *inventory.Inventory {
	mp, ok := p.(*mockPlayer)
	if !ok {
		return nil
	}
	switch mp {
	case m.self:
		return m.selfInvs[typeID]
	case m.self2:
		return m.self2Invs[typeID]
	}
	return nil
}

// newTwoPlayerInvFixture builds a fixture where Self and Self2 each have
// their own main + bank inventories. Inventories are seeded as fresh
// (capacity 28 main, 100 bank, both StackNormal/StackAlways per testInvMain/Bank).
func newTwoPlayerInvFixture() (*twoPlayerInvLookup, *mockPlayer, *mockPlayer) {
	self := &mockPlayer{username: "Self", uidValue: 1, x: 100, z: 100}
	self2 := &mockPlayer{username: "Self2", uidValue: 2, x: 200, z: 200}
	selfMain := inventory.New(testInvMain, 28, inventory.StackNormal)
	selfBank := inventory.New(testInvBank, 100, inventory.StackAlways)
	self2Main := inventory.New(testInvMain, 28, inventory.StackNormal)
	self2Bank := inventory.New(testInvBank, 100, inventory.StackAlways)
	return &twoPlayerInvLookup{
		self:      self,
		self2:     self2,
		selfInvs:  map[int]*inventory.Inventory{testInvMain: selfMain, testInvBank: selfBank},
		self2Invs: map[int]*inventory.Inventory{testInvMain: self2Main, testInvBank: self2Bank},
	}, self, self2
}

// TestBothMoveInv_Primary_DrainsFromSelfToSelf2 — operand=0; populate
// Self's main with {coins x 5, sword x 1}; expect Self2's main to hold
// the items post; Self's main empty.
func TestBothMoveInv_Primary_DrainsFromSelfToSelf2(t *testing.T) {
	mc := newTestInvConfigs()
	lookup, self, self2 := newTwoPlayerInvFixture()
	world := &mockWorldVars{}

	// Seed Self's main: slot 0 = coins x 5, slot 1 = sword x 1.
	lookup.selfInvs[testInvMain].Items[0] = &inventory.Item{Id: testObjCoin, Count: 5}
	lookup.selfInvs[testInvMain].Items[1] = &inventory.Item{Id: testObjSword, Count: 1}

	// from = main, to = main (both players' main inv).
	st := runBothMoveInv(t, 0, []int{testInvMain, testInvMain}, lookup, mc, world, self, self2, false)
	_ = st

	// Self's main should be empty.
	for slot, it := range lookup.selfInvs[testInvMain].Items {
		if it != nil {
			t.Errorf("Self.main slot %d should be nil, got %+v", slot, it)
		}
	}
	// Self2's main should hold coins (5) + sword (1).
	if it := lookup.self2Invs[testInvMain].Get(0); it == nil || it.Id != testObjCoin || it.Count != 5 {
		t.Errorf("Self2.main slot 0: got %+v, want {coins, 5}", it)
	}
	if it := lookup.self2Invs[testInvMain].Get(1); it == nil || it.Id != testObjSword || it.Count != 1 {
		t.Errorf("Self2.main slot 1: got %+v, want {sword, 1}", it)
	}
	// No overflow → no World.AddObj calls.
	if len(world.addObjCalls) != 0 {
		t.Errorf("expected zero AddObj calls, got %d: %+v", len(world.addObjCalls), world.addObjCalls)
	}
}

// TestBothMoveInv_Secondary_DrainsFromSelf2ToSelf — operand=1; pointers
// flip; Self2's bank → Self's bank.
func TestBothMoveInv_Secondary_DrainsFromSelf2ToSelf(t *testing.T) {
	mc := newTestInvConfigs()
	lookup, self, self2 := newTwoPlayerInvFixture()
	world := &mockWorldVars{}

	// Seed Self2's bank: slot 0 = arrow x 100.
	lookup.self2Invs[testInvBank].Items[0] = &inventory.Item{Id: testObjArr, Count: 100}

	// from = bank, to = bank. Operand=1 means: fromPlayer = Self2, toPlayer = Self.
	// Both invs are Scope=Shared so no protect gate fires. Slot-1 protect not required.
	st := runBothMoveInv(t, 1, []int{testInvBank, testInvBank}, lookup, mc, world, self, self2, false)
	_ = st

	// Self2's bank should be empty.
	if it := lookup.self2Invs[testInvBank].Get(0); it != nil {
		t.Errorf("Self2.bank slot 0: got %+v, want nil", it)
	}
	// Self's bank should hold arrows.
	if it := lookup.selfInvs[testInvBank].Get(0); it == nil || it.Id != testObjArr || it.Count != 100 {
		t.Errorf("Self.bank slot 0: got %+v, want {arrow, 100}", it)
	}
}

// TestBothMoveInv_Overflow_StackableDropsSingleStack — toInv full of
// stackable item leaving zero free slots; fromInv has stackable count=N
// → toInv absorbs 0 (or partial), World.AddObj called once with the
// overflow count at toPlayer's tile.
func TestBothMoveInv_Overflow_StackableDropsSingleStack(t *testing.T) {
	mc := newTestInvConfigs()
	lookup, self, self2 := newTwoPlayerInvFixture()
	world := &mockWorldVars{}

	// Self's main slot 0 = coins x 7 (stackable).
	lookup.selfInvs[testInvMain].Items[0] = &inventory.Item{Id: testObjCoin, Count: 7}
	// Fill Self2's main with non-coin/non-stockable items to leave NO free slot
	// AND no existing coin-stack to merge into (forces full overflow).
	for i := range lookup.self2Invs[testInvMain].Items {
		lookup.self2Invs[testInvMain].Items[i] = &inventory.Item{Id: testObjSword, Count: 1}
	}

	st := runBothMoveInv(t, 0, []int{testInvMain, testInvMain}, lookup, mc, world, self, self2, false)
	_ = st

	// Self.main slot 0 should be cleared (deleted before Add).
	if it := lookup.selfInvs[testInvMain].Get(0); it != nil {
		t.Errorf("Self.main slot 0 should be nil after delete, got %+v", it)
	}
	// One AddObj call with count=7 at Self2's tile.
	if len(world.addObjCalls) != 1 {
		t.Fatalf("expected 1 AddObj call (stackable overflow), got %d: %+v", len(world.addObjCalls), world.addObjCalls)
	}
	call := world.addObjCalls[0]
	if call.typeID != testObjCoin || call.count != 7 {
		t.Errorf("AddObj: got typeID=%d count=%d, want %d / 7", call.typeID, call.count, testObjCoin)
	}
	if call.x != self2.x || call.z != self2.z {
		t.Errorf("AddObj coords: got (%d, %d), want (%d, %d) (toPlayer=Self2)", call.x, call.z, self2.x, self2.z)
	}
}

// TestBothMoveInv_Overflow_NonStackableDropsPerUnit — non-stackable
// fromInv has obj count=K (TS allows it via the slot.count read), toInv
// is full → World.AddObj called K times, count=1 each.
func TestBothMoveInv_Overflow_NonStackableDropsPerUnit(t *testing.T) {
	mc := newTestInvConfigs()
	lookup, self, self2 := newTwoPlayerInvFixture()
	world := &mockWorldVars{}

	// Self's main slot 0 = sword x 3 (non-stackable; TS allows the count).
	lookup.selfInvs[testInvMain].Items[0] = &inventory.Item{Id: testObjSword, Count: 3}
	// Fill Self2's main → no free slot → full overflow.
	for i := range lookup.self2Invs[testInvMain].Items {
		lookup.self2Invs[testInvMain].Items[i] = &inventory.Item{Id: testObjArr, Count: 1}
	}

	st := runBothMoveInv(t, 0, []int{testInvMain, testInvMain}, lookup, mc, world, self, self2, false)
	_ = st

	// 3 per-unit AddObj calls.
	if len(world.addObjCalls) != 3 {
		t.Fatalf("expected 3 AddObj calls (non-stackable per-unit), got %d: %+v", len(world.addObjCalls), world.addObjCalls)
	}
	for i, call := range world.addObjCalls {
		if call.typeID != testObjSword || call.count != 1 {
			t.Errorf("call %d: got typeID=%d count=%d, want %d / 1", i, call.typeID, call.count, testObjSword)
		}
	}
}

// TestBothMoveInv_FromProtectGate_FiresWhenSlotUnprotected — primary,
// fromInv.Protect=true + Scope=TEMP, slot-0 unprotected (Init protect=false)
// → from-gate fires.
func TestBothMoveInv_FromProtectGate_FiresWhenSlotUnprotected(t *testing.T) {
	mc := newTestInvConfigs()
	mc.invs[testInvMain].Protect = true
	mc.invs[testInvMain].Scope = objtype.InvTypeScopeTemp
	lookup, self, self2 := newTwoPlayerInvFixture()
	world := &mockWorldVars{}

	// Override the helper's slot-0 protect: rebuild state manually.
	sf := &ScriptFile{
		Name:             "from_gate",
		Opcodes:          []Opcode{OpBothMoveInv, OpReturn},
		IntOperands:      []int32{0, 0},
		StringOperands:   []string{"", ""},
		InstructionCount: 2,
	}
	state := Init(sf, self, false, nil, nil) // slot-0 NOT protected
	state.Self2 = self2
	state.Pointers |= PtrActivePlayer2
	state.Inv = lookup
	state.Configs = mc
	state.World = world
	state.PushInt(testInvMain)
	state.PushInt(testInvMain)

	err := Execute(state)
	if err == nil || !strings.Contains(err.Error(), "BOTH_MOVEINV: $from_inv requires protected access: main") {
		t.Errorf("expected from-gate error, got %v", err)
	}
}

// TestBothMoveInv_ToProtectGate_UsesFromInvScope — TS quirk pin
// (InvOps.ts:397). toInv.Protect=true, fromInv.Scope=TEMP (NOT shared)
// → gate FIRES because it reads fromInv.Scope. Slot-0 protected (passes
// from-gate); slot-1 unprotected (fails to-gate).
func TestBothMoveInv_ToProtectGate_UsesFromInvScope_Fires(t *testing.T) {
	mc := newTestInvConfigs()
	mc.invs[testInvMain].Scope = objtype.InvTypeScopeTemp // from-scope NOT shared
	mc.invs[testInvBank].Protect = true                   // toInv.Protect=true
	mc.invs[testInvBank].Scope = objtype.InvTypeScopeShared // toInv.Scope IS shared (TS quirk: gate ignores this)
	lookup, self, self2 := newTwoPlayerInvFixture()
	world := &mockWorldVars{}

	runBothMoveInvExpectErr(t, 0, []int{testInvMain, testInvBank}, lookup, mc, world,
		self, self2,
		true,  // slot-0 protected
		false, // slot-1 NOT protected
		"BOTH_MOVEINV: $to_inv requires protected access: bank",
	)
}

// TestBothMoveInv_ToProtectGate_UsesFromInvScope_DoesNotFire — inverse
// pin: same toInv but fromInv.Scope=Shared → gate DOES NOT fire.
func TestBothMoveInv_ToProtectGate_UsesFromInvScope_DoesNotFire(t *testing.T) {
	mc := newTestInvConfigs()
	mc.invs[testInvMain].Scope = objtype.InvTypeScopeShared // from-scope shared → gate skipped
	mc.invs[testInvBank].Protect = true
	mc.invs[testInvBank].Scope = objtype.InvTypeScopeTemp // toInv.Scope NOT shared (TS quirk: ignored)
	lookup, self, self2 := newTwoPlayerInvFixture()
	world := &mockWorldVars{}

	st := runBothMoveInv(t, 0, []int{testInvMain, testInvBank}, lookup, mc, world, self, self2, false)
	if st == nil {
		t.Fatal("expected handler to complete without error")
	}
}

// TestBothMoveInv_NoSelf2_Primary_Errors — operand=0 with PtrActivePlayer2
// unset → error "no active player2".
func TestBothMoveInv_NoSelf2_Primary_Errors(t *testing.T) {
	mc := newTestInvConfigs()
	lookup, self, _ := newTwoPlayerInvFixture()
	world := &mockWorldVars{}

	sf := &ScriptFile{
		Name:             "no_self2",
		Opcodes:          []Opcode{OpBothMoveInv, OpReturn},
		IntOperands:      []int32{0, 0},
		StringOperands:   []string{"", ""},
		InstructionCount: 2,
	}
	state := Init(sf, self, true, nil, nil)
	// Self2 deliberately NOT set; PtrActivePlayer2 NOT set.
	state.Inv = lookup
	state.Configs = mc
	state.World = world
	state.PushInt(testInvMain)
	state.PushInt(testInvMain)

	err := Execute(state)
	if err == nil || !strings.Contains(err.Error(), "no active player2") {
		t.Errorf("expected 'no active player2' error, got %v", err)
	}
}

// TestBothMoveInv_NoSelf_Secondary_Errors — operand=1; requireActivePlayer2
// passes (Self2 set), but the secondary path also requires Self bound
// for toPlayer. Drop Self.
func TestBothMoveInv_NoSelf_Secondary_Errors(t *testing.T) {
	mc := newTestInvConfigs()
	lookup, _, self2 := newTwoPlayerInvFixture()
	world := &mockWorldVars{}

	sf := &ScriptFile{
		Name:             "no_self_secondary",
		Opcodes:          []Opcode{OpBothMoveInv, OpReturn},
		IntOperands:      []int32{1, 0},
		StringOperands:   []string{"", ""},
		InstructionCount: 2,
	}
	state := Init(sf, nil, false, nil, nil) // Self nil
	state.Self2 = self2
	state.Pointers |= PtrActivePlayer2
	state.Inv = lookup
	state.Configs = mc
	state.World = world
	state.PushInt(testInvMain)
	state.PushInt(testInvMain)

	err := Execute(state)
	// requireActivePlayer2 passes; the in-handler nil-check on toPlayer fails.
	if err == nil || !strings.Contains(err.Error(), "no active player") {
		t.Errorf("expected 'no active player' error, got %v", err)
	}
}

// TestBothMoveInv_FromInvNil_Errors — InvLookup returns nil for from →
// "inv is null".
func TestBothMoveInv_FromInvNil_Errors(t *testing.T) {
	mc := newTestInvConfigs()
	lookup, self, self2 := newTwoPlayerInvFixture()
	delete(lookup.selfInvs, testInvMain)
	world := &mockWorldVars{}

	runBothMoveInvExpectErr(t, 0, []int{testInvMain, testInvBank}, lookup, mc, world,
		self, self2, true, false,
		"BOTH_MOVEINV: inv is null",
	)
}

// TestBothMoveInv_ToInvNil_Errors — InvLookup returns nil for to.
func TestBothMoveInv_ToInvNil_Errors(t *testing.T) {
	mc := newTestInvConfigs()
	lookup, self, self2 := newTwoPlayerInvFixture()
	delete(lookup.self2Invs, testInvBank)
	world := &mockWorldVars{}

	runBothMoveInvExpectErr(t, 0, []int{testInvMain, testInvBank}, lookup, mc, world,
		self, self2, true, false,
		"BOTH_MOVEINV: inv is null",
	)
}

// TestBothMoveInv_InvalidOperand_Errors — operand=2 → error.
func TestBothMoveInv_InvalidOperand_Errors(t *testing.T) {
	mc := newTestInvConfigs()
	lookup, self, self2 := newTwoPlayerInvFixture()
	world := &mockWorldVars{}

	runBothMoveInvExpectErr(t, 2, []int{testInvMain, testInvMain}, lookup, mc, world,
		self, self2, true, false,
		"BOTH_MOVEINV: invalid intOperand 2",
	)
}

// TestBothMoveInv_WealthEventSkip_NoEmission — D1 absence pin: even with
// fromInvType.DebugName="dueloffer" and a non-empty drain, no
// OpWealthEvent recorder fires. Per ts_asymmetry_dual_pin.md: pin the
// absence so a future WealthEvent wiring escalates this test.
func TestBothMoveInv_WealthEventSkip_NoEmission(t *testing.T) {
	mc := newTestInvConfigs()
	mc.invs[testInvMain].DebugName = "dueloffer" // STAKE branch trigger in TS
	mc.invs[testInvMain].Protect = false         // skip protect gate
	lookup, self, self2 := newTwoPlayerInvFixture()
	lookup.selfInvs[testInvMain].Items[0] = &inventory.Item{Id: testObjCoin, Count: 1000}
	world := &mockWorldVars{}

	st := runBothMoveInv(t, 0, []int{testInvMain, testInvMain}, lookup, mc, world, self, self2, false)
	_ = st

	// Production sanity: items moved.
	if it := lookup.self2Invs[testInvMain].Get(0); it == nil || it.Count != 1000 {
		t.Fatalf("transfer must succeed before D1 absence-pin is meaningful: got %+v", it)
	}
	// D1 absence: mockPlayer has no addWealthEvent recorder; nothing to assert directly.
	// When the WealthEvent subsystem lands, this test should be extended to assert
	// a recorder field on mockPlayer remained empty. Until then, the comment is
	// the contract.
}
```

**Pre-flight check** — `mockWorldVars` must already have an `addObjCalls` recorder. Verify:

```bash
rg 'mockWorldVars\b|addObjCalls\b' pkg/script/handlers_inv_test.go pkg/script/handlers_test.go pkg/script/runner_test.go | head -10
```

Expected: `mockWorldVars` exists with `addObjCalls []addObjCall` slice. (NAI-115/NAI-130 introduced this.) If the field name differs, adjust the test code to the actual field name. If the recorder is missing, abort and message the controller.

- [ ] **Step 3.3 — Run tests, verify RED**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run 'TestBothMoveInv' -count=1 -v
```

Expected: All ~12 new tests FAIL — `OpBothMoveInv` has no handler registered yet, so `Execute` returns "no handler for BOTH_MOVEINV (opcode 4301)".

- [ ] **Step 3.4 — Implement `handleBothMoveInv`**

Append to `pkg/script/handlers_inv.go`:

```go
// handleBothMoveInv ports TS InvOps.ts:373-495 (BOTH_MOVEINV, opcode 4301).
//
// Dispatch shape: state.intOperand selects primary (0) vs secondary (1).
// Primary:    from = active_player (Self), to = .active_player (Self2).
// Secondary:  pointers swap — from = Self2, to = Self.
//
// Pop order (TS popInts(2)): from on bottom, to on top → PopInt() returns
// to first.
//
// Protect gates per TS (slot-flipped on secondary):
//   - fromPlayer's slot must be Protected if fromInv.Protect && fromInv.Scope != Shared
//   - toPlayer's slot must be Protected if toInv.Protect && fromInv.Scope != Shared
//     (TS quirk preserved: to-gate gates on FROM scope, InvOps.ts:397)
//
// Drain loop: for each non-empty slot in fromInv, delete the slot, attempt
// to add the count to toInv at toPlayer; spill any overflow to toPlayer's
// tile via World.AddObj using TS InvOps.ts:423-432 stackable branching
// (per-unit loop for non-stackable / overflow==1, single stack for the
// stackable many-overflow case).
//
// DEVIATION-NAI-115-D1 (reuse): TS InvOps.ts:445-494 emits addWealthEvent
// for dueloffer/STAKE and trade/TRADE. Goscape skips inline emission;
// content can emit via OpWealthEvent (2131). Single-point retire when
// WealthEvent subsystem lands. NAI-115-D1.
func handleBothMoveInv(s *ScriptState) error {
	operand := s.Script.IntOperands[s.PC]
	if operand != 0 && operand != 1 {
		return fmt.Errorf("BOTH_MOVEINV: invalid intOperand %d", operand)
	}
	secondary := operand == 1

	// checkedHandler(ActivePlayer[intOperand]) gate: when secondary, the
	// intOperand selects ActivePlayer2.
	if secondary {
		if err := requireActivePlayer2(s, "BOTH_MOVEINV"); err != nil {
			return err
		}
	} else {
		if err := requireActivePlayer(s, "BOTH_MOVEINV"); err != nil {
			return err
		}
	}

	// Pop [from, to] (TS popInts(2) order).
	to := s.PopInt()
	from := s.PopInt()

	// InvTypeValid × 2.
	if err := checkInvType(s, from, "BOTH_MOVEINV"); err != nil {
		return err
	}
	if err := checkInvType(s, to, "BOTH_MOVEINV"); err != nil {
		return err
	}

	fromInvType := s.Configs.InvType(from)
	toInvType := s.Configs.InvType(to)

	// Resolve fromPlayer / toPlayer per `secondary`. Both must be bound;
	// when secondary, the toPlayer (Self) must also be bound — else error
	// matching TS `if (!fromPlayer || !toPlayer)`.
	var fromPlayer, toPlayer ActivePlayer
	var fromProtectedFlag, toProtectedFlag Pointer
	if secondary {
		fromPlayer = s.Self2
		toPlayer = s.Self
		fromProtectedFlag = PtrProtectedActivePlayer2
		toProtectedFlag = PtrProtectedActivePlayer
		if toPlayer == nil || s.Pointers&PtrActivePlayer == 0 {
			return fmt.Errorf("BOTH_MOVEINV: no active player")
		}
	} else {
		fromPlayer = s.Self
		toPlayer = s.Self2
		fromProtectedFlag = PtrProtectedActivePlayer
		toProtectedFlag = PtrProtectedActivePlayer2
		if toPlayer == nil || s.Pointers&PtrActivePlayer2 == 0 {
			return fmt.Errorf("BOTH_MOVEINV: no active player2")
		}
	}

	// From-protect gate.
	if fromInvType.Protect && fromInvType.Scope != objtype.InvTypeScopeShared &&
		s.Pointers&fromProtectedFlag == 0 {
		return fmt.Errorf("BOTH_MOVEINV: $from_inv requires protected access: %s", fromInvType.DebugName)
	}
	// TS quirk preserved (InvOps.ts:397): to-gate gates on FROM scope.
	if toInvType.Protect && fromInvType.Scope != objtype.InvTypeScopeShared &&
		s.Pointers&toProtectedFlag == 0 {
		return fmt.Errorf("BOTH_MOVEINV: $to_inv requires protected access: %s", toInvType.DebugName)
	}

	if s.Inv == nil {
		return fmt.Errorf("BOTH_MOVEINV: no inv lookup")
	}
	fromInv := s.Inv.Get(fromPlayer, from)
	toInv := s.Inv.Get(toPlayer, to)
	if fromInv == nil || toInv == nil {
		return fmt.Errorf("BOTH_MOVEINV: inv is null")
	}

	// Drain loop. TS InvOps.ts:413-443.
	for slot := 0; slot < fromInv.Capacity; slot++ {
		it := fromInv.Get(slot)
		if it == nil {
			continue
		}
		objID := it.Id
		count := it.Count

		objType := s.Configs.ObjType(objID)
		if objType == nil {
			return fmt.Errorf("BOTH_MOVEINV: invalid obj id at slot (id=%d)", objID)
		}

		fromInv.Delete(slot)

		stackable, stockObj := lookupStackableStockObj(s, toInv.Type, objID)
		tx := toInv.Add(objID, count, inventory.AddOpts{
			BeginSlot:           -1,
			AssureFullInsertion: false,
			Stackable:           stackable,
			StockObj:            stockObj,
		})
		overflow := count - tx.Completed
		if overflow > 0 && s.World != nil {
			level := (toPlayer.CoordPacked() >> 28) & 0x3
			x := toPlayer.X()
			z := toPlayer.Z()
			receiverID := toPlayer.UID()
			if !objType.Stackable || overflow == 1 {
				for range overflow {
					s.World.AddObj(level, x, z, objID, 1, 200, receiverID)
				}
			} else {
				s.World.AddObj(level, x, z, objID, overflow, 200, receiverID)
			}
		}
	}

	// NAI-115-D1 (reuse): TS InvOps.ts:445-494 emits addWealthEvent for
	// dueloffer/STAKE and trade/TRADE. Skipped — content emits via
	// OpWealthEvent (2131).

	return nil
}
```

**Pre-flight check** — confirm `inventory.AddOpts` and the return shape (`tx.Completed`) match. The handler at `handlers_inv.go:357-364` is the canonical reference (`Inventory.Add(id, count, opts) Transaction`). If the return shape has changed, mirror that handler exactly.

Imports: `handlers_inv.go` already has `fmt`, `inventory`, `objtype` — `handleBothMoveInv` uses only those (errors emitted via `fmt.Errorf`). No import changes required.

- [ ] **Step 3.5 — Register opcode dispatch**

Edit `pkg/script/handlers.go` near line 307 (where `OpInvAdd: handleInvAdd,` lives). Add a new entry — alphabetical order keeps it near the top of the inventory block:

```go
OpBothMoveInv:    handleBothMoveInv,
```

(Place between `OpBothDropSlot` if it exists, or before `OpInvAdd`.)

```bash
grep -n "OpBothDropSlot\|OpInvAdd" pkg/script/handlers.go | head -5
```

- [ ] **Step 3.6 — Run new tests, verify GREEN**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run 'TestBothMoveInv' -count=1 -v
```

Expected: all ~12 BOTH_MOVEINV tests PASS.

- [ ] **Step 3.7 — Run full test suite, verify no regressions**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/... ./modules/world/... -count=1
```

Expected: GREEN.

- [ ] **Step 3.8 — Commit**

```bash
git add pkg/script/handlers_inv.go pkg/script/handlers.go pkg/script/handlers_inv_test.go
git commit --no-gpg-sign -m "feat(nai-133): T3 — BOTH_MOVEINV (4301) handler (GREEN)

Ports TS InvOps.ts:373-495. intOperand=1 swaps Self/Self2 roles. Per-pointer-slot Protect gates via PtrProtectedActivePlayer{,2}. TS quirk preserved (InvOps.ts:397): to-gate evaluates fromInv.Scope. NAI-115-D1 reuse for wealth-event tail skip (dueloffer/STAKE, trade/TRADE)."
```

---

## Task 4 — Close

**Files:**
- Modify: `/home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md` (mark BOTH_MOVEINV closed)
- Modify: spec doc cross-reference (no actual edit needed; close commit body provides the linkage)

**Goal:** Memory updates + close commit per project convention.

- [ ] **Step 4.1 — Update `nai_followups.md`**

Read the file:

```bash
cat /home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md
```

Append (or update existing line) under a "Closed" section or similar:

```markdown
- BOTH_MOVEINV (opcode 4301) — closed by NAI-133 (commit <SHA from T3>). Ports TS InvOps.ts:373-495. Reuses NAI-115-D1 for wealth-event tail skip.
- Per-pointer-slot Protect tracking (NAI-132 §2 prerequisite) — closed by NAI-133 T1. Pointer-flag refactor: PtrProtectedActivePlayer{,2}, retired `s.Protect bool`.
- FINDUID/P_FINDUID intOperand slot routing — closed by NAI-133 T2. Latent `.p_finduid` / `.finduid` clobber bug fixed.
```

- [ ] **Step 4.2 — Final repo-wide test verification**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./... -count=1
```

Expected: GREEN. (Use `-race` per project convention for the close-commit gate.)

- [ ] **Step 4.3 — Close commit with `Closes memory:` trailer**

```bash
git add /home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md
git commit --no-gpg-sign -m "$(cat <<'EOF'
chore(nai-133): close — BOTH_MOVEINV (4301) + per-pointer-slot Protect (Pointer flag refactor) + FINDUID/P_FINDUID slot routing

T1 reified s.Protect as PtrProtectedActivePlayer{,2} flags (1<<9, 1<<10). T2 routed FINDUID/P_FINDUID on intOperand to bind Self vs Self2; closed latent .p_finduid clobber bug. T3 ported BOTH_MOVEINV per TS InvOps.ts:373-495 with NAI-115-D1 reuse and TS-quirk to-gate-uses-from-scope dual-pin. Spec: docs/superpowers/specs/2026-05-08-nai-133-both-moveinv-design.md.

Closes memory: nai_followups.md (BOTH_MOVEINV closed; per-pointer-slot Protect closed; FINDUID/P_FINDUID slot routing closed).
EOF
)"
```

- [ ] **Step 4.4 — Verify clean tree**

```bash
git status
```

Expected: clean working tree.

```bash
git log --oneline -5
```

Expected: T1 refactor → T2 GREEN → T3 GREEN → T4 close, in order.

---

## Self-Review Checklist (controller pre-flight)

- [ ] **Spec coverage:** Every section of the spec maps to a task.
  - §1 Scope T1-T4 → tasks T1-T4 ✓
  - §3 TS sources → cited inline in T2 (FINDUID/P_FINDUID), T3 (BOTH_MOVEINV) handler doc-comments ✓
  - §4.T1 Pointer-flag refactor → Task 1 ✓
  - §4.T2 FINDUID/P_FINDUID slot routing → Task 2 ✓
  - §4.T3 handleBothMoveInv → Task 3 ✓
  - §4.T4 Close → Task 4 ✓
  - §5 Deviations: D1-reuse (T3 doc-comment), TS-quirk preservation (T3 inline + dual-pin tests), Engine-side ProtectedActivePlayer2 unreachable (T1 pointer.go doc-comment), Init(protect=true, self=nil) silently drops (T1 Step 1.4 audit grep) ✓
- [ ] **Placeholder scan:** No "TBD" / "TODO" / "implement later" / "appropriate error handling" / "similar to ..." appears in the plan.
- [ ] **Type consistency:**
  - `PtrProtectedActivePlayer` / `PtrProtectedActivePlayer2` consistent across all tasks.
  - `requireProtectedActivePlayer2` defined in T1.5; consumed by T3 (and future sub-specs).
  - `handleBothMoveInv` signature consistent.
  - `inventory.AddOpts{BeginSlot, AssureFullInsertion, Stackable, StockObj}` and return `tx.Completed` mirror the pattern at `handlers_inv.go:357-364`.
  - `s.World.AddObj(level, x, z, typeID, count, duration, receiverID int) ActiveObj` matches `state.go:109`.
  - `mockPlayer.canAccess`, `mockPlayer.uidValue`, `mockPlayer.x/z` exist (NAI-30+ established).
- [ ] **Plan-codified test fixtures runnable:** Every new test ScriptFile has `IntOperands` of the right length (≥ Opcodes length); tests that need `s.World` / `s.Configs` / `s.Inv` / `s.PlayerLookup` set them; per `scriptstate_test_fixture_idioms.md`.

---

## Execution Notes

- T1 ships **GREEN-only** — no new tests; existing tests must continue to pass after fixture migration. If any test fails post-T1, the migration was incomplete; do NOT alter test logic to compensate.
- T2 and T3 follow standard NAI RED→GREEN cadence.
- All `go` commands prefix with `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache` (CLAUDE.md convention).
- All commits use `--no-gpg-sign` (CLAUDE.md convention).
- Per `controller_preflight.md`: at each task dispatch, controller re-greps the listed migration sites against HEAD before sending the prompt — NAI-132's plan timeline shifted line numbers; same risk here.
- Per `verify_implementer_claims.md`: after each implementer commit, run `git show <SHA> --stat` and `git status` to confirm the diff matches the stated scope.
- Per `plan_doc_replaceall_timeline.md`: when applying edits to this plan doc post-dispatch, never use `replace_all` — use per-instance edits with task-section context.
