# NAI-75 — SPLIT_* opcode port to unblock chatnpc dialog rendering

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the constant-returning SPLIT_INIT/GET/PAGECOUNT/LINECOUNT/GETANIM stubs in `pkg/script/handlers_string.go` with real implementations so that runescript content's `[proc,chatnpc]` (and 215 other SPLIT_* call sites) renders chat dialogs to the Java client.

**Architecture:** Light-fidelity port. SPLIT_INIT splits text on `|` (the explicit line-break char used in chatnpc strings) into raw lines, chunks them into pages of `linesPerPage` lines each, parses any leading `<p,name>` mesanim prefix into a tracked but unresolved id, stores results in two new `ScriptState` fields (`SplitPages [][]string`, `SplitMesanim int32`). The other 4 handlers read those fields. No font-aware word-wrap (deferred via `NAI-75-D-FONT-WRAP-NAIVE`); no MesanimType resolution (deferred via `NAI-75-D-MESANIM-NOT-PORTED`).

**Tech Stack:** Go 1.26+. All changes inside `pkg/script/`.

**Spec:** `docs/superpowers/specs/2026-05-03-nai-75-split-ops-chatnpc-render-design.md` (commit `5852a28`).

**TS source-of-truth:** `LostCityRS/Engine-TS/src/engine/script/handlers/StringOps.ts:76-122`.

---

## File map

- **Modify** `pkg/script/state.go` — add `SplitPages [][]string` + `SplitMesanim int32` fields to the `ScriptState` struct (Task 1).
- **Modify** `pkg/script/handlers_string.go` — replace 5 stub bodies (Tasks 2 + 3).
- **Modify** `pkg/script/handlers_string_test.go` — add real-behavior tests for all 5 opcodes; retire the stale `TestSplitStubsReturnZeroes` stub test (Tasks 2 + 3).

No new files. No changes outside `pkg/script/`.

---

## Pre-flight (controller, before Task 1 dispatch)

Per `controller_preflight.md`. Verify these premises against HEAD before dispatching Task 1:

- [ ] **P1.** `pkg/script/state.go:136` declares `type ScriptState struct {` and the field block runs through ~line 240. `Reset` / re-init helper: none — `Init()` in `runner.go:12` is the only constructor; it does NOT explicitly init slice fields beyond `IntStack`/`StringStack`/`IntLocals`/`StringLocals`/`Frames`. New fields will rely on Go zero-value (`nil` for slice, `0` for int32). Verify by `grep -n "func.*Reset\|func Init" pkg/script/state.go pkg/script/runner.go`.
- [ ] **P2.** `pkg/script/handlers_string.go:99-132` contains 5 stub functions: `handleSplitInit`, `handleSplitGet`, `handleSplitGetAnim`, `handleSplitLineCount`, `handleSplitPageCount`. Confirm by `grep -n "func handleSplit" pkg/script/handlers_string.go`.
- [ ] **P3.** `pkg/script/handlers.go:155-159` registers `OpSplitInit/Get/GetAnim/LineCount/PageCount → handleSplit*`. No dispatch table edit needed.
- [ ] **P4.** `pkg/script/handlers_string_test.go` contains `TestSplitStubsReturnZeroes` at the file's tail asserting `SPLIT_LINECOUNT` returns 0 and `SPLIT_PAGECOUNT` returns 0. This test will be retired in Task 3.
- [ ] **P5.** Test helper `runStringOp(t, op, intInputs, stringInputs) (topInt, topStr)` is defined in `handlers_string_test.go`. It uses `Init(sf, nil, false, nil, nil)` and pushes string inputs first, then int inputs (per its existing comment).
- [ ] **P6.** `popInts` semantics in TS `LostCityRS/Engine-TS/src/engine/script/ScriptState.ts`: `popInts(n)` returns an array filled from index `n-1` down to 0 (each call to `popInt()` pops top-of-stack). Therefore `[a, b, c] = popInts(3)` gives `a` = first-pushed (deepest), `c` = last-pushed (top). For the runescript call `split_init(text, maxWidth, linesPerPage, fontId)`, the runtime push order is text→maxWidth→linesPerPage→fontId; so `popString()` returns text, then `popInts(3)` returns `[maxWidth, linesPerPage, fontId]`. Goscape equivalent: pop fontId (top), then linesPerPage, then maxWidth, then string.

If any premise fails, halt and re-spec.

---

## Task 1: Add SplitPages + SplitMesanim fields to ScriptState

**Files:**
- Modify: `pkg/script/state.go` (struct field block)
- Test: `pkg/script/state_test.go` (existing or new — controller checks first; if absent, Task 1 creates it)

- [ ] **Step 1.1: Check whether `pkg/script/state_test.go` exists**

```bash
ls pkg/script/state_test.go 2>/dev/null && echo EXISTS || echo MISSING
```

If MISSING, create it in Step 1.2 with full file scaffold; if EXISTS, append the test function.

- [ ] **Step 1.2: Write the failing test**

If `state_test.go` exists, append this test:

```go
func TestScriptStateSplitFieldsZeroValue(t *testing.T) {
	s := &ScriptState{}
	if s.SplitPages != nil {
		t.Errorf("fresh ScriptState.SplitPages: got %v, want nil", s.SplitPages)
	}
	if s.SplitMesanim != 0 {
		t.Errorf("fresh ScriptState.SplitMesanim: got %d, want 0", s.SplitMesanim)
	}
}
```

If `state_test.go` is missing, create it with:

```go
package script

import "testing"

func TestScriptStateSplitFieldsZeroValue(t *testing.T) {
	s := &ScriptState{}
	if s.SplitPages != nil {
		t.Errorf("fresh ScriptState.SplitPages: got %v, want nil", s.SplitPages)
	}
	if s.SplitMesanim != 0 {
		t.Errorf("fresh ScriptState.SplitMesanim: got %d, want 0", s.SplitMesanim)
	}
}
```

- [ ] **Step 1.3: Run test to verify it fails**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestScriptStateSplitFieldsZeroValue -count=1
```
Expected: FAIL with `s.SplitPages undefined` (build error).

- [ ] **Step 1.4: Add the two fields to ScriptState**

In `pkg/script/state.go`, locate the `ScriptState` struct field block (starts at `type ScriptState struct {` ~line 136). Append immediately AFTER the existing `npcIterator` field block (or at the end of the struct, just before the closing brace — controller picks the location that keeps related fields grouped; the test is location-agnostic). Add:

```go
	// SplitPages holds the per-page, per-line wrapped chat-dialog text
	// produced by SPLIT_INIT and consumed by SPLIT_GET / SPLIT_PAGECOUNT
	// / SPLIT_LINECOUNT. Nil before any SPLIT_INIT call. Each call to
	// SPLIT_INIT replaces (not appends) the slice. Mirrors TS
	// ScriptState.splitPages (StringOps.ts:91). NAI-75.
	SplitPages [][]string

	// SplitMesanim is the MesanimType id parsed from a leading <p,name>
	// prefix on SPLIT_INIT's text input, or -1 when no prefix is present.
	// Currently set by SPLIT_INIT but consumed by SPLIT_GETANIM as -1
	// unconditionally per NAI-75-D-MESANIM-NOT-PORTED (no MesanimType
	// cache loader yet). Mirrors TS ScriptState.splitMesanim
	// (StringOps.ts:85). NAI-75.
	SplitMesanim int32
```

- [ ] **Step 1.5: Run test to verify it passes**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestScriptStateSplitFieldsZeroValue -count=1
```
Expected: PASS.

- [ ] **Step 1.6: Run full pkg/script tests to confirm no regressions**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -count=1
```
Expected: all tests pass (the existing `TestSplitStubsReturnZeroes` still passes — stubs unchanged).

- [ ] **Step 1.7: Commit**

```bash
git add pkg/script/state.go pkg/script/state_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): NAI-75 T1 — ScriptState.SplitPages + SplitMesanim fields

Adds the two ScriptState fields that SPLIT_INIT will populate and that
SPLIT_GET/PAGECOUNT/LINECOUNT/GETANIM will read. Mirrors TS
ScriptState.splitPages + splitMesanim (StringOps.ts:85, 91).
Zero-value safe: nil/0 before any SPLIT_INIT call.

NAI-75 T1.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: Real SPLIT_INIT implementation

**Files:**
- Modify: `pkg/script/handlers_string.go:99-106` (replace `handleSplitInit` body)
- Test: `pkg/script/handlers_string_test.go` (append SPLIT_INIT tests)

**Pop arity bug fix:** the current stub pops 2 ints + 1 string. TS pops 3 ints (`maxWidth, linesPerPage, fontId`) + 1 string. The order is: pop `fontId` (top of stack) → pop `linesPerPage` → pop `maxWidth` → pop `text`.

**Mesanim prefix parsing:** per TS `StringOps.ts:82-89`. If text starts with `<p,` AND contains `>`, the substring between `<p,` and the first `>` is the mesanim name; light-fidelity skips the `MesanimType.getId(...)` lookup and sets `SplitMesanim = -1`. Strip the prefix from text either way.

**Light-fidelity wrap:** split the (prefix-stripped) text on `|` (forced line-break char per `runescape_guide.rs2` convention). Chunk the resulting lines into pages of `linesPerPage` lines each. `maxWidth` and `fontId` are popped but unused (NAI-75-D-FONT-WRAP-NAIVE).

- [ ] **Step 2.1: Write the failing tests**

Append to `pkg/script/handlers_string_test.go` (replace the existing `TestSplitStubsReturnZeroes` with the new real-behavior tests in Task 3 step 3.1; for now, just add the SPLIT_INIT tests):

```go
// runSplitInit pushes (text, maxWidth, linesPerPage, fontId) and runs
// SPLIT_INIT against a fresh state, then returns the state for assertion.
func runSplitInit(t *testing.T, text string, maxWidth, linesPerPage, fontId int) *ScriptState {
	t.Helper()
	sf := &ScriptFile{
		Name:             "test_split_init",
		Opcodes:          []Opcode{OpSplitInit, OpReturn},
		IntOperands:      []int32{0, 0},
		StringOperands:   []string{"", ""},
		InstructionCount: 2,
	}
	state := Init(sf, nil, false, nil, nil)
	state.PushString(text)
	state.PushInt(maxWidth)
	state.PushInt(linesPerPage)
	state.PushInt(fontId)
	if err := Execute(state); err != nil {
		t.Fatalf("SPLIT_INIT: unexpected error: %v", err)
	}
	return state
}

func TestSplitInitNoMesanimPrefix(t *testing.T) {
	s := runSplitInit(t, "first line|second line", 380, 4, 8)
	if s.SplitMesanim != -1 {
		t.Errorf("SplitMesanim: got %d, want -1 (no prefix)", s.SplitMesanim)
	}
	if len(s.SplitPages) != 1 {
		t.Fatalf("len(SplitPages): got %d, want 1", len(s.SplitPages))
	}
	if got, want := s.SplitPages[0], []string{"first line", "second line"}; !equalStrings(got, want) {
		t.Errorf("SplitPages[0]: got %v, want %v", got, want)
	}
}

func TestSplitInitMesanimPrefixStripped(t *testing.T) {
	s := runSplitInit(t, "<p,neutral>Greetings|stranger", 380, 4, 8)
	// NAI-75-D-MESANIM-NOT-PORTED: prefix parsed but id lookup deferred,
	// so SplitMesanim stays -1 even when a prefix is present.
	if s.SplitMesanim != -1 {
		t.Errorf("SplitMesanim: got %d, want -1 (NAI-75-D-MESANIM-NOT-PORTED pin)", s.SplitMesanim)
	}
	if len(s.SplitPages) != 1 {
		t.Fatalf("len(SplitPages): got %d, want 1", len(s.SplitPages))
	}
	// Prefix stripped: text is "Greetings|stranger" → 2 lines.
	if got, want := s.SplitPages[0], []string{"Greetings", "stranger"}; !equalStrings(got, want) {
		t.Errorf("SplitPages[0]: got %v, want %v (prefix should be stripped)", got, want)
	}
}

func TestSplitInitMultiPageChunking(t *testing.T) {
	// 5 lines, linesPerPage=4 → 2 pages: page 0 = 4 lines, page 1 = 1 line.
	s := runSplitInit(t, "a|b|c|d|e", 380, 4, 8)
	if len(s.SplitPages) != 2 {
		t.Fatalf("len(SplitPages): got %d, want 2", len(s.SplitPages))
	}
	if got, want := s.SplitPages[0], []string{"a", "b", "c", "d"}; !equalStrings(got, want) {
		t.Errorf("SplitPages[0]: got %v, want %v", got, want)
	}
	if got, want := s.SplitPages[1], []string{"e"}; !equalStrings(got, want) {
		t.Errorf("SplitPages[1]: got %v, want %v", got, want)
	}
}

func TestSplitInitReplacesNotAppends(t *testing.T) {
	// Multi-call SAME ScriptState: second SPLIT_INIT must replace SplitPages,
	// not append. Mirrors chatnpc's repeated calls within Welcome flow.
	sf := &ScriptFile{
		Name:             "test_split_init_replace",
		Opcodes:          []Opcode{OpSplitInit, OpSplitInit, OpReturn},
		IntOperands:      []int32{0, 0, 0},
		StringOperands:   []string{"", "", ""},
		InstructionCount: 3,
	}
	state := Init(sf, nil, false, nil, nil)
	// Push order matters: stack is LIFO, both opcodes execute in instruction
	// order, so the FIRST opcode pops what was pushed LAST. Push the
	// SECOND call's args FIRST (deepest), then the FIRST call's args
	// (top of stack — popped by the first SPLIT_INIT instruction).
	//
	// Second SPLIT_INIT: 1 line.
	state.PushString("only")
	state.PushInt(380)
	state.PushInt(4)
	state.PushInt(8)
	// First SPLIT_INIT: 3 lines.
	state.PushString("x|y|z")
	state.PushInt(380)
	state.PushInt(4)
	state.PushInt(8)
	if err := Execute(state); err != nil {
		t.Fatalf("SPLIT_INIT chain: %v", err)
	}
	// After two SPLIT_INIT calls, SplitPages reflects the SECOND call's
	// result ("only"), proving replacement (not append) semantics.
	if len(state.SplitPages) != 1 {
		t.Fatalf("len(SplitPages) after second SPLIT_INIT: got %d, want 1", len(state.SplitPages))
	}
	if got, want := state.SplitPages[0], []string{"only"}; !equalStrings(got, want) {
		t.Errorf("SplitPages[0]: got %v, want %v (second call must replace, not append)", got, want)
	}
}

func TestSplitInitEmptyText(t *testing.T) {
	s := runSplitInit(t, "", 380, 4, 8)
	// Empty text → strings.Split("", "|") returns [""] → 1 page, 1 line.
	// This matches TS font.split("") which returns [""].
	if len(s.SplitPages) != 1 {
		t.Fatalf("len(SplitPages) for empty text: got %d, want 1", len(s.SplitPages))
	}
	if got, want := s.SplitPages[0], []string{""}; !equalStrings(got, want) {
		t.Errorf("SplitPages[0]: got %v, want %v", got, want)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
```

- [ ] **Step 2.2: Run tests to verify they fail**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestSplitInit -count=1 -v
```
Expected: all 5 SPLIT_INIT tests FAIL — current stub pops only 2 ints, leaves stack mis-aligned, and `SplitPages` stays nil.

- [ ] **Step 2.3: Replace handleSplitInit body**

In `pkg/script/handlers_string.go`, replace the entire `handleSplitInit` function (currently lines 99-106) with:

```go
// handleSplitInit ports TS SPLIT_INIT (StringOps.ts:76-96). Pops
// (text, maxWidth, linesPerPage, fontId), parses any leading <p,name>
// mesanim prefix, splits the prefix-stripped text on '|' (the explicit
// line-break char used in chatnpc strings), and chunks the lines into
// pages of linesPerPage lines each. Stores results in s.SplitPages +
// s.SplitMesanim.
//
// NAI-75-D-FONT-WRAP-NAIVE: maxWidth + fontId are popped but unused
// (no font-aware word-wrap; relies on '|' breaks). Closure: future
// FontType cache loader sub-spec calls font.split(text, maxWidth) here.
//
// NAI-75-D-MESANIM-NOT-PORTED: <p,name> prefix is parsed and stripped
// but SplitMesanim is left at -1 (no MesanimType.getId lookup yet).
// Closure: future MesanimType cache loader sub-spec resolves the id.
func handleSplitInit(s *ScriptState) error {
	// Pop order matches TS popInts(3) semantics: top of stack is fontId.
	_ = s.PopInt() // fontId — unused per NAI-75-D-FONT-WRAP-NAIVE
	linesPerPage := s.PopInt()
	_ = s.PopInt() // maxWidth — unused per NAI-75-D-FONT-WRAP-NAIVE
	text := s.PopString()

	s.SplitMesanim = -1
	if strings.HasPrefix(text, "<p,") {
		if end := strings.IndexByte(text, '>'); end != -1 {
			// Prefix recognised; light-fidelity skips MesanimType lookup.
			// SplitMesanim stays -1 per NAI-75-D-MESANIM-NOT-PORTED.
			text = text[end+1:]
		}
	}

	if linesPerPage < 1 {
		// Defensive: TS would divide-by-zero on splice(0, 0); we no-op
		// to avoid an infinite chunking loop. Goscape defensive (TS
		// throws); labelled per defensive_gate_doc_comment_label.md.
		s.SplitPages = [][]string{{text}}
		return nil
	}

	lines := strings.Split(text, "|")
	pages := make([][]string, 0, (len(lines)+linesPerPage-1)/linesPerPage)
	for i := 0; i < len(lines); i += linesPerPage {
		end := i + linesPerPage
		if end > len(lines) {
			end = len(lines)
		}
		pages = append(pages, lines[i:end])
	}
	s.SplitPages = pages
	slog.Debug("SPLIT_INIT processed",
		"script", s.Script.Name, "pages", len(pages), "mesanim", s.SplitMesanim)
	return nil
}
```

- [ ] **Step 2.4: Run tests to verify they pass**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestSplitInit -count=1 -v
```
Expected: all 5 SPLIT_INIT tests PASS.

- [ ] **Step 2.5: Run full pkg/script tests**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -count=1
```
Expected: PASS. The existing `TestSplitStubsReturnZeroes` still passes (it tests SPLIT_LINECOUNT + SPLIT_PAGECOUNT, both still stubs at this point).

- [ ] **Step 2.6: Run race detector**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -race -count=1
```
Expected: PASS, no data races.

- [ ] **Step 2.7: Commit**

```bash
git add pkg/script/handlers_string.go pkg/script/handlers_string_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): NAI-75 T2 — real SPLIT_INIT implementation

Replaces the constant-returning stub in handlers_string.go:99-106
with a real implementation per TS StringOps.ts:76-96. Fixes pop arity
(now pops 3 ints + 1 string per popInts(3)), parses <p,name> mesanim
prefix, splits text on '|' (chatnpc forced line-break char), chunks
into pages of linesPerPage lines.

Light-fidelity port: maxWidth + fontId popped but unused
(NAI-75-D-FONT-WRAP-NAIVE); MesanimType lookup deferred
(NAI-75-D-MESANIM-NOT-PORTED) — prefix parsed and stripped but
SplitMesanim stays -1.

NAI-75 T2.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: Real SPLIT_GET / PAGECOUNT / LINECOUNT / GETANIM + retire stale stub test

**Files:**
- Modify: `pkg/script/handlers_string.go:108-132` (replace 4 stub bodies)
- Modify: `pkg/script/handlers_string_test.go` (replace `TestSplitStubsReturnZeroes`; add real tests for each handler)

- [ ] **Step 3.1: Replace `TestSplitStubsReturnZeroes` with real-behavior tests**

In `pkg/script/handlers_string_test.go`, find `TestSplitStubsReturnZeroes` (the test at the file's tail asserting the stubs return 0) and DELETE it entirely. Replace with these new tests:

```go
// runSplitInitThen runs SPLIT_INIT then a single follow-up opcode in the
// same script, returning the state. Used by SPLIT_GET/PAGECOUNT/etc tests.
func runSplitInitThen(t *testing.T, initText string, linesPerPage int, follow Opcode, followInts []int) *ScriptState {
	t.Helper()
	ops := []Opcode{OpSplitInit, follow, OpReturn}
	sf := &ScriptFile{
		Name:             "test_split_init_then_" + follow.String(),
		Opcodes:          ops,
		IntOperands:      make([]int32, len(ops)),
		StringOperands:   make([]string, len(ops)),
		InstructionCount: int32(len(ops)),
	}
	state := Init(sf, nil, false, nil, nil)
	// SPLIT_INIT pushes: text, maxWidth, linesPerPage, fontId.
	state.PushString(initText)
	state.PushInt(380)
	state.PushInt(linesPerPage)
	state.PushInt(8)
	// Follow-up opcode args (e.g. page index for SPLIT_GET).
	for _, v := range followInts {
		state.PushInt(v)
	}
	if err := Execute(state); err != nil {
		t.Fatalf("SPLIT_INIT+%s: unexpected error: %v", follow.String(), err)
	}
	return state
}

func TestSplitPageCountAfterInit(t *testing.T) {
	// 5 lines, linesPerPage=4 → 2 pages.
	s := runSplitInitThen(t, "a|b|c|d|e", 4, OpSplitPageCount, nil)
	got := s.PopInt()
	if got != 2 {
		t.Errorf("SPLIT_PAGECOUNT: got %d, want 2", got)
	}
}

func TestSplitPageCountBeforeInit(t *testing.T) {
	// No SPLIT_INIT call — SplitPages is nil; len(nil) = 0.
	sf := &ScriptFile{
		Name:             "test_split_pagecount_uninit",
		Opcodes:          []Opcode{OpSplitPageCount, OpReturn},
		IntOperands:      []int32{0, 0},
		StringOperands:   []string{"", ""},
		InstructionCount: 2,
	}
	state := Init(sf, nil, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("SPLIT_PAGECOUNT uninit: %v", err)
	}
	if got := state.PopInt(); got != 0 {
		t.Errorf("SPLIT_PAGECOUNT (no prior SPLIT_INIT): got %d, want 0", got)
	}
}

func TestSplitLineCountAfterInit(t *testing.T) {
	// 5 lines, linesPerPage=4 → page 0 has 4 lines, page 1 has 1.
	s := runSplitInitThen(t, "a|b|c|d|e", 4, OpSplitLineCount, []int{0})
	if got := s.PopInt(); got != 4 {
		t.Errorf("SPLIT_LINECOUNT(0): got %d, want 4", got)
	}
	s = runSplitInitThen(t, "a|b|c|d|e", 4, OpSplitLineCount, []int{1})
	if got := s.PopInt(); got != 1 {
		t.Errorf("SPLIT_LINECOUNT(1): got %d, want 1", got)
	}
}

func TestSplitLineCountOutOfBounds(t *testing.T) {
	// Defensive: TS would throw; goscape pushes 0 and logs debug.
	s := runSplitInitThen(t, "a|b", 4, OpSplitLineCount, []int{99})
	if got := s.PopInt(); got != 0 {
		t.Errorf("SPLIT_LINECOUNT(99) on 1-page state: got %d, want 0 (defensive)", got)
	}
}

func TestSplitGetAfterInit(t *testing.T) {
	s := runSplitInitThen(t, "first|second|third", 4, OpSplitGet, []int{0, 1})
	if got := s.PopString(); got != "second" {
		t.Errorf("SPLIT_GET(0,1): got %q, want %q", got, "second")
	}
	s = runSplitInitThen(t, "first|second|third", 4, OpSplitGet, []int{0, 0})
	if got := s.PopString(); got != "first" {
		t.Errorf("SPLIT_GET(0,0): got %q, want %q", got, "first")
	}
}

func TestSplitGetOutOfBounds(t *testing.T) {
	// Defensive: TS would throw on undefined access; goscape pushes "".
	s := runSplitInitThen(t, "a", 4, OpSplitGet, []int{99, 99})
	if got := s.PopString(); got != "" {
		t.Errorf("SPLIT_GET(99,99): got %q, want \"\" (defensive)", got)
	}
}

func TestSplitGetAnimReturnsMinusOne(t *testing.T) {
	// NAI-75-D-MESANIM-NOT-PORTED: SPLIT_GETANIM unconditionally returns -1
	// regardless of prefix, until MesanimType cache loader ports.
	s := runSplitInitThen(t, "<p,neutral>Greetings", 4, OpSplitGetAnim, []int{0})
	if got := s.PopInt(); got != -1 {
		t.Errorf("SPLIT_GETANIM(0) with prefix: got %d, want -1 (NAI-75-D-MESANIM-NOT-PORTED pin)", got)
	}
	s = runSplitInitThen(t, "no prefix", 4, OpSplitGetAnim, []int{0})
	if got := s.PopInt(); got != -1 {
		t.Errorf("SPLIT_GETANIM(0) no prefix: got %d, want -1", got)
	}
}
```

- [ ] **Step 3.2: Run tests to verify they fail**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run "TestSplitPageCount|TestSplitLineCount|TestSplitGet" -count=1 -v
```
Expected: most tests FAIL (handlers still stubs):
- `TestSplitPageCountAfterInit` fails (stub returns 0, want 2)
- `TestSplitPageCountBeforeInit` may pass coincidentally (stub returns 0)
- `TestSplitLineCountAfterInit` fails (stub returns 0, want 4)
- `TestSplitLineCountOutOfBounds` may pass coincidentally
- `TestSplitGetAfterInit` fails (stub pushes "", want "second")
- `TestSplitGetOutOfBounds` may pass coincidentally (stub pushes "")
- `TestSplitGetAnimReturnsMinusOne` fails (stub pushes -1 actually — wait, check current stub)

Verify the failure set; the controller may need to amend specific assertions if any fail differently than expected.

- [ ] **Step 3.3: Replace 4 stub handlers**

In `pkg/script/handlers_string.go`, replace `handleSplitGet`, `handleSplitGetAnim`, `handleSplitLineCount`, `handleSplitPageCount` (currently lines ~108-132) with:

```go
// handleSplitGet ports TS SPLIT_GET (StringOps.ts:98-102). Pops
// (page, line); pushes s.SplitPages[page][line]. Out-of-bounds pushes
// empty string (goscape defensive; TS throws — labelled per
// defensive_gate_doc_comment_label.md).
func handleSplitGet(s *ScriptState) error {
	line := s.PopInt()
	page := s.PopInt()
	if page < 0 || page >= len(s.SplitPages) {
		s.PushString("")
		slog.Debug("SPLIT_GET out of page range",
			"script", s.Script.Name, "page", page, "pages", len(s.SplitPages))
		return nil
	}
	pg := s.SplitPages[page]
	if line < 0 || line >= len(pg) {
		s.PushString("")
		slog.Debug("SPLIT_GET out of line range",
			"script", s.Script.Name, "page", page, "line", line, "lines", len(pg))
		return nil
	}
	s.PushString(pg[line])
	return nil
}

// handleSplitGetAnim ports TS SPLIT_GETANIM (StringOps.ts:114-122).
// Pops page; pushes -1 unconditionally per NAI-75-D-MESANIM-NOT-PORTED
// (no MesanimType cache loader; the TS path requires
// MesanimValid.len[lineCount-1] which depends on it). Closure: future
// MesanimType cache loader sub-spec wires the lookup here.
func handleSplitGetAnim(s *ScriptState) error {
	_ = s.PopInt() // page — unused per NAI-75-D-MESANIM-NOT-PORTED
	s.PushInt(-1)
	return nil
}

// handleSplitLineCount ports TS SPLIT_LINECOUNT (StringOps.ts:108-112).
// Pops page; pushes len(s.SplitPages[page]). Out-of-bounds pushes 0
// (goscape defensive; TS throws — labelled per
// defensive_gate_doc_comment_label.md).
func handleSplitLineCount(s *ScriptState) error {
	page := s.PopInt()
	if page < 0 || page >= len(s.SplitPages) {
		s.PushInt(0)
		slog.Debug("SPLIT_LINECOUNT out of page range",
			"script", s.Script.Name, "page", page, "pages", len(s.SplitPages))
		return nil
	}
	s.PushInt(len(s.SplitPages[page]))
	return nil
}

// handleSplitPageCount ports TS SPLIT_PAGECOUNT (StringOps.ts:104-106).
// Pushes len(s.SplitPages). Returns 0 before any SPLIT_INIT call
// (Go zero-value: SplitPages is nil, len(nil) == 0).
func handleSplitPageCount(s *ScriptState) error {
	s.PushInt(len(s.SplitPages))
	return nil
}
```

- [ ] **Step 3.4: Run all SPLIT_* tests**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run "TestSplit" -count=1 -v
```
Expected: ALL SPLIT_* tests PASS (Task 2's SPLIT_INIT tests + this task's 7 new tests).

- [ ] **Step 3.5: Run full pkg/script tests**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -count=1
```
Expected: PASS. The deleted `TestSplitStubsReturnZeroes` no longer runs; its replacements cover the same ground at higher fidelity.

- [ ] **Step 3.6: Run cross-package tests to confirm no regressions**

Per `verify_implementer_claims.md` package-scoped-green-mask check:

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./... -count=1
```
Expected: ALL tests pass across the entire module.

- [ ] **Step 3.7: Run race detector on full module**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./... -race -count=1
```
Expected: PASS, no data races.

- [ ] **Step 3.8: Commit**

```bash
git add pkg/script/handlers_string.go pkg/script/handlers_string_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): NAI-75 T3 — real SPLIT_GET/PAGECOUNT/LINECOUNT/GETANIM

Replaces the 4 constant-returning stubs in handlers_string.go:108-132
with real implementations per TS StringOps.ts:98-122. SPLIT_GET reads
s.SplitPages[page][line]; SPLIT_PAGECOUNT/LINECOUNT return the slice
lengths; SPLIT_GETANIM returns -1 unconditionally per
NAI-75-D-MESANIM-NOT-PORTED (no MesanimType lookup yet).

Defensive bounds-checks on GET/LINECOUNT push empty/0 + log debug
when TS would throw (labelled per defensive_gate_doc_comment_label.md).

Retires stale TestSplitStubsReturnZeroes; replaced with 7 real-behavior
tests covering both populated and uninitialized states + out-of-bounds.

NAI-75 T3.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Smoke handoff (between Task 3 and close commit)

**Controller-driven, NOT a subagent task.** Per `smoke_test_server_handoff.md`: the goscape server must be launched by the user (sandboxed processes are unreachable from the host Java client).

Controller emits to user:

> "Task 3 complete; SPLIT_* opcodes now ported. Please rebuild and run goscape locally, connect with the Java client (LostCityRS/Client-Java), create a brand-new account, and walk through the Tutorial Island RuneScape Guide flow. Report:
>
> 1. Does the chatnpc preamble `'Do you want to skip the tutorial?'` render as a chat dialog before the choice2 popup?
> 2. After picking 'No, thank you.', do 5 sequential chat dialogs render with click-through (Greetings → Talking to others → Click on inhabitants → Read website → Go through that door)?
> 3. Does the npc-name + chathead area appear (chathead anim may be static — that's NAI-75-D-MESANIM-NOT-PORTED, expected)?
> 4. Can you click and use the door to advance? (downstream confirmation)
> 5. Can you re-talk to the RS Guide? (downstream confirmation — fires `@runescape_guide_return`'s 3 chatnpcs)
>
> Reply with pass/fail per item."

**Smoke verdict routing:**
- **All 5 pass** → proceed to close commit.
- **Items 1+2 pass, items 3-5 fail** → close NAI-75; downstream issues are independent; open NAI-76 for whichever symptom remains.
- **Items 1 OR 2 fail** → enter Stage 3 (runtime instrumentation; spec §"Stage 3 — Runtime instrumentation (conditional)").

---

## Close commit (controller, after smoke pass)

**Files:**
- Modify: `~/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md` — append NAI-75 close entry; append NAI-75-D-FONT-WRAP-NAIVE + NAI-75-D-MESANIM-NOT-PORTED to "Active deviations".
- Modify: `~/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/MEMORY.md` — only if the close-time review (per spec §"Memory entries to potentially add at NAI-75 close") promotes any of the candidate lessons. Default: no MEMORY.md edits.

- [ ] **Step C.1: Verify all commits land cleanly on main**

```bash
git log --oneline -5
git status
```
Expected: T1, T2, T3 commits visible; working tree clean.

- [ ] **Step C.2: Append NAI-75 close section to nai_followups.md**

Following the established NAI-N-CLOSED template (see NAI-74 entry at lines 3872-3997 of nai_followups.md for the canonical shape). Section should cover: Scope, Cadence, Spec/Plan paths, Commits chronological, Follow-ups closed (none), Deviations opened (the 2 NAI-75-D entries with closure paths), Net deviation tally (12 → 14), Wire-behaviour delta at HEAD, Lessons confirmed, Lessons surfaced, Carry-forwards still open, Smoke result.

- [ ] **Step C.3: Append the 2 new active deviations to nai_followups.md**

Under "Active deviations" section:

```markdown
- **NAI-75-D-FONT-WRAP-NAIVE** — `pkg/script/handlers_string.go::handleSplitInit` skips TS `font.split(text, maxWidth)` (no FontType cache loader). Light-fidelity wrap respects only `|` as forced line break. Consequence: long lines without `|` overflow the dialog component. Closure path: future FontType cache loader sub-spec ports per-character pixel widths + word-wrap algorithm; SPLIT_INIT calls `font.Split(text, maxWidth)` here.
- **NAI-75-D-MESANIM-NOT-PORTED** — `pkg/script/handlers_string.go::handleSplitInit` parses and strips the `<p,name>` mesanim prefix but does NOT resolve the name to a MesanimType id; `SplitMesanim` stays -1. `handleSplitGetAnim` returns -1 unconditionally. Consequence: chathead animations on chat dialogs are absent (static head, no talk-anim). Closure path: future MesanimType cache loader sub-spec adds `MesanimType` to `Configs` interface; SPLIT_INIT writes a real id and SPLIT_GETANIM reads `MesanimType.len[lineCount-1]`.
```

- [ ] **Step C.4: Create the close commit**

Per `close_commit_memory_trailer.md`, include `Closes memory:` trailer for the followups-md updates and `Opens:` trailer for the 2 new deviations:

```bash
git commit --allow-empty --no-gpg-sign -m "$(cat <<'EOF'
chore(close): NAI-75 — SPLIT_* port unblocks chatnpc dialog rendering             (opens 2: FONT-WRAP-NAIVE + MESANIM-NOT-PORTED; tally 12 → 14)

Stage 1 (brainstorm-time): root cause confirmed — handlers_string.go:99-132
ships SPLIT_INIT/GET/PAGECOUNT/LINECOUNT/GETANIM as constant-returning
stubs marked "deferred to later sub-spec." chatnpc proc's
`while ($page < split_pagecount=0)` loop iterates zero times → p_pausebutton
never fires → no chat dialog reaches client.

Stage 2 (T1-T3): light-fidelity SPLIT_* port. T1: ScriptState.SplitPages
+ SplitMesanim fields. T2: SPLIT_INIT real impl (pop arity 2→3, <p,name>
prefix parse+strip, '|' line-split, page chunking). T3: 4 simple handlers
+ retired stale stub test.

Smoke (user-mediated): chatnpc dialogs render in Tutorial Island
RuneScape Guide Welcome flow.

Opens: NAI-75-D-FONT-WRAP-NAIVE (font cache port closure),
       NAI-75-D-MESANIM-NOT-PORTED (mesanim cache port closure).
Closes memory: nai_followups.md NAI-75 entry.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

(Body details on smoke outcome + commit SHAs filled in by controller at close time.)

---

## Self-review checklist (controller, before dispatching Task 1)

- [x] **Spec coverage:** all spec Stage 2 tasks (2.1 ScriptState fields, 2.2 SPLIT_INIT, 2.3 four handlers, 2.4 deviations, 2.5 verify) mapped to plan tasks (T1 → 2.1; T2 → 2.2; T3 → 2.3; close commit → 2.4; in-task `go test ./...` runs → 2.5).
- [x] **Placeholder scan:** every step has full code; no TBDs; commit messages are complete; failure-expectation lines specify mode.
- [x] **Type consistency:** field names `SplitPages` / `SplitMesanim` match across struct decl (Task 1), SPLIT_INIT writes (Task 2), and the 4 reader handlers (Task 3). Pop arity on SPLIT_INIT is `(int, int, int, string)` consistently across the test fixture and the handler body.
- [x] **TDD discipline:** every task has Red (failing test) → Green (impl) → commit, with explicit run commands and expected outcomes per step.
- [x] **Per `superpowers_code_reviewer_model.md`**: any reviewer dispatch the controller chooses must be Sonnet (never Opus).
- [x] **Per `enumerate_all_sites.md`:** the 5 SPLIT_* opcode handler bodies are the complete set; no additional dispatch table edits needed (handlers.go already routes them).

---

## Memory entries to potentially add at NAI-75 close

(See spec §"Memory entries to potentially add at NAI-75 close". Three candidates: stub-deferred-comment-as-canonical-marker, content-driven-investigation-cadence, chatnpc-text-uses-pipe-as-line-break. Decision deferred to close-time review.)
