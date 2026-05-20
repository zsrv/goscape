# Compiler deferrals cleanup — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Retire `NAI-208-D-LOGPROCREQ-DEFERRED` by porting `logProcRequirement` (recursive `POINTER_REQUIRED_LOC` HINT chain) from RuneScriptTS into goscape's `PointerChecker`. Refresh the stale `NAI-212-D-INTERFACE-FALLBACK-FROM-COMPONENTINFO` doc comment and remove the phantom `NAI-212-D-CLIENT-PACKERS-DEFERRED` references.

**Architecture:** Item A is a self-contained method on `*PointerChecker` that composes already-existing helpers (`scriptsBySymbol`, `requiresPointerPathScript`, `staticLabelArgsByCall`, `getJumpParamNodes`, `GetPointers`, `requiresPointerAtNodes`) — no new analysis primitives. Called once at the end of `validatePointer` to walk down the call graph from the error node, emitting HINTs at every Gosub/Jump boundary that requires the missing pointer. Item B is doc-only: replace the stale `populateInterfaceOverlay` block in `symbols.go` (+ sibling test comment) with a permanent justification that names the actual standalone-compile caller (`goscape-cli compile` → `LoadCompilerSymbols`).

**Tech Stack:** Go 1.26, `pkg/pack/compiler/cfg` (pointer checker + CFG primitives), `pkg/pack/compiler/diagnostics` (HINT diagnostic templates). Tests run under `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./...` and the `smoke-pack` content gate.

**Predecessor:** Spec at `docs/superpowers/specs/2026-05-20-compiler-deferrals-cleanup-design.md`. HEAD at plan-write: `9955fdc0` (cleanup commit dropping unused log field) on top of `2c5544be` (post runtime-fixups-cluster fixup).

---

## File map

| File | Touched by | Responsibility |
|---|---|---|
| `pkg/pack/compiler/cfg/pointer_checker.go` | T1, T2, T3 | Add `(*PointerChecker).logProcRequirement` method. Wire one call site at end of `validatePointer`. Retire `NAI-208-D-LOGPROCREQ-DEFERRED` from the deferral comment. |
| `pkg/pack/compiler/cfg/pointer_checker_validation_test.go` | T1, T2 | 3 new tests: direct proc HINT, two-hop recursive HINT, static-label-arg fallback HINT. |
| `pkg/pack/compiler/cfg/nai208_deviation_pins_test.go` | T3 | Drop `NAI-208-D-LOGPROCREQ-DEFERRED` from the tags slice at L134. |
| `pkg/pack/compiler/symbols.go` | T4 | Replace stale `populateInterfaceOverlay` doc comment at L594-601 per spec §3.2. |
| `pkg/pack/compiler/symbols_test.go` | T4 | Replace stale `TestPopulateInterfaceOverlay_NilConfig_FallsBack` comment at L451-455 per spec §3.2. |
| `/home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/compiler_deferrals_cleanup_close.md` (new) | T5 | Close memo. |
| `/home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/MEMORY.md` | T5 | New top entry pointer. |

Task ordering: T1 (helper + direct-proc test) → T2 (recursion + static-label-fallback tests) → T3 (wire + retire tag + drop pin) → T4 (NAI-212 doc refresh) → T5 (close). T1-T3 must run sequential (same file). T4 is independent of T1-T3 but depends on T3 being committed first (so the close memo at T5 captures the right HEAD).

---

## Task 1: Add `logProcRequirement` helper + direct-proc HINT test

**Files:**
- Modify: `pkg/pack/compiler/cfg/pointer_checker.go` (add new method)
- Modify: `pkg/pack/compiler/cfg/pointer_checker_validation_test.go` (add new test + helper)

- [ ] **Step 1: Write the failing test for the direct proc HINT chain**

Append to `pkg/pack/compiler/cfg/pointer_checker_validation_test.go` (after the existing `TestPointerChecker_Run_ProtectedPopRequiresP` body):

```go
// TestPointerChecker_Run_LogProcRequirement_DirectProcChain pins that a
// caller→callee Gosub where the callee requires a pointer the caller
// doesn't set produces (1) one ERROR at the caller's gosub site (existing
// behavior) and (2) one HINT at the callee's pointer-requiring instruction
// with MessagePointerRequiredLoc. Mirrors RuneScriptTS PointerChecker.ts
// logProcRequirement leaf case.
func TestPointerChecker_Run_LogProcRequirement_DirectProcChain(t *testing.T) {
	procTr := &trigger.TriggerType{ID: 0, Identifier: "proc"}

	// callee — body: `p_kickout` (requires ACTIVE_PLAYER); trigger does NOT set it.
	calleeSym := &symbol.ServerScriptSymbol{ScriptSymbolFields: symbol.ScriptSymbolFields{Trigger: procTr, Name: "callee"}}
	callee := codegen.NewRuneScript("test.rs2", calleeSym, procTr, "callee", nil)
	cb := codegen.NewBlock(&codegen.Label{Name: "entry"})
	require := makeCommandSymbol("p_kickout")
	cb.Add(codegen.Instruction{Opcode: codegen.Command, Operand: require})
	cb.Add(codegen.Instruction{Opcode: codegen.Return})
	callee.Blocks = []*codegen.Block{cb}

	// caller — body: `~callee` (Gosub callee); trigger does NOT set ACTIVE_PLAYER.
	callerSym := &symbol.ServerScriptSymbol{ScriptSymbolFields: symbol.ScriptSymbolFields{Trigger: procTr, Name: "caller"}}
	caller := codegen.NewRuneScript("test.rs2", callerSym, procTr, "caller", nil)
	ab := codegen.NewBlock(&codegen.Label{Name: "entry"})
	ab.Add(codegen.Instruction{Opcode: codegen.Gosub, Operand: calleeSym})
	ab.Add(codegen.Instruction{Opcode: codegen.Return})
	caller.Blocks = []*codegen.Block{ab}

	cp := map[string]*pointer.PointerHolder{
		"p_kickout": {Required: pointer.NewPointerSet(pointer.ActivePlayer)},
	}
	d := &diagnostics.Diagnostics{}
	pc := NewPointerChecker(d, []*codegen.RuneScript{callee, caller}, cp, semantics.StrictFeatureLevel{})
	pc.Run()

	errs := errorDiagnostics(d)
	if len(errs) != 1 {
		t.Fatalf("got %d error diagnostics, want 1: %v", len(errs), d.List())
	}
	hints := hintDiagnostics(d)
	if len(hints) != 1 {
		t.Fatalf("got %d hint diagnostics, want 1 (HINT at callee's p_kickout site): %v", len(hints), d.List())
	}
	msg := fmt.Sprintf(hints[0].Message, hints[0].MessageArgs...)
	if !strings.Contains(msg, "active_player required here") {
		t.Errorf("hint diagnostic = %q, want substring \"active_player required here\"", msg)
	}
}

// hintDiagnostics filters d to only hint-severity entries.
func hintDiagnostics(d *diagnostics.Diagnostics) []diagnostics.Diagnostic {
	var out []diagnostics.Diagnostic
	for _, e := range d.List() {
		if e.Type == diagnostics.DiagnosticHint {
			out = append(out, e)
		}
	}
	return out
}
```

If `diagnostics.Diagnostic.Type` is not the exact field name, inspect `pkg/pack/compiler/diagnostics/diagnostic.go` for the severity-tag field. The existing `errorDiagnostics` helper at L145 uses `e.IsError()` — if a `IsHint()` method exists, use that instead:

```go
func hintDiagnostics(d *diagnostics.Diagnostics) []diagnostics.Diagnostic {
	var out []diagnostics.Diagnostic
	for _, e := range d.List() {
		if e.IsHint() {
			out = append(out, e)
		}
	}
	return out
}
```

- [ ] **Step 2: Run the test to verify it fails**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run TestPointerChecker_Run_LogProcRequirement_DirectProcChain ./pkg/pack/compiler/cfg/ -v
```

Expected: FAIL — got 0 hint diagnostics, want 1. (The `validatePointer` deferral block doesn't emit any HINT for the proc path.)

- [ ] **Step 3: Add the `logProcRequirement` method to `pointer_checker.go`**

Insert the new method immediately AFTER the `cloneNodeSet` function (current L196) in `pkg/pack/compiler/cfg/pointer_checker.go`. The method composes existing helpers and recurses via `scriptsBySymbol`:

```go
// logProcRequirement walks the call graph downward from node, emitting
// MessagePointerRequiredLoc HINT diagnostics at every Gosub/Jump boundary
// where pt is required. When the called script directly requires pt
// (requiresPointerPathScript returns a path), emits a HINT at the path's
// first node and recurses into it. When the called script does NOT
// directly require pt, falls back to inspecting the call's
// staticLabelArgsByCall entry and emits a HINT at each label-typed
// parameter whose label requires pt and whose jump-param node confirms
// the requirement. Mirrors TS PointerChecker.logProcRequirement
// (RuneScriptTS src/compiler/codegen/script/config/PointerChecker.ts:243-301).
//
// TS throws on script-lookup miss / nil instruction source; goscape
// silently returns at those points — defensive fallthrough matching the
// no-panic posture documented at NAI-209-D-PUSHLONG-PANIC etc. The
// earlier error diagnostic already surfaces user-visible failure.
//
// Retires NAI-208-D-LOGPROCREQ-DEFERRED.
func (p *PointerChecker) logProcRequirement(
	node *InstructionNode,
	pt *pointer.PointerType,
	analysis *scriptPointerAnalysis,
) {
	inst := node.Instruction
	if inst == nil {
		return
	}
	if inst.Opcode != codegen.Gosub && inst.Opcode != codegen.Jump {
		return
	}

	sym, ok := inst.Operand.(symbol.Symbol)
	if !ok {
		return
	}
	calledScript, ok := p.scriptsBySymbol[sym]
	if !ok {
		return
	}

	scriptPath := p.requiresPointerPathScript(calledScript, pt)
	if scriptPath == nil {
		staticArgs, present := analysis.staticLabelArgsByCall[inst]
		if !present {
			return
		}
		jumpParamNodes := p.getJumpParamNodes(calledScript)
		// Sort param indexes for deterministic HINT emission order
		// (Go map iteration is unordered; mirrors NAI-210-D-LOADER-SORTED-ITERATION posture).
		indexes := make([]int, 0, len(staticArgs))
		for i := range staticArgs {
			indexes = append(indexes, i)
		}
		sort.Ints(indexes)
		for _, paramIndex := range indexes {
			labelSym := staticArgs[paramIndex]
			if !p.GetPointers(labelSym).Required.Has(pt) {
				continue
			}
			nodes := jumpParamNodes[paramIndex]
			if len(nodes) == 0 {
				continue
			}
			if !p.requiresPointerAtNodes(calledScript, pt, nodes) {
				continue
			}
			required := nodes[0]
			if required.Instruction == nil {
				continue
			}
			p.diagnostics.Report(diagnostics.NewDiagnostic(
				required.Instruction.Source,
				diagnostics.DiagnosticHint,
				diagnostics.MessagePointerRequiredLoc,
				pt.Representation,
			))
		}
		return
	}

	required := scriptPath[0]
	if required.Instruction == nil {
		return
	}
	p.diagnostics.Report(diagnostics.NewDiagnostic(
		required.Instruction.Source,
		diagnostics.DiagnosticHint,
		diagnostics.MessagePointerRequiredLoc,
		pt.Representation,
	))
	p.logProcRequirement(required, pt, analysis)
}
```

Add the `"sort"` import to the import block at the top of `pointer_checker.go` if not already present. Verify with `grep -n '"sort"' pkg/pack/compiler/cfg/pointer_checker.go`.

Then replace the deferral comment at L180-185 (`// NAI-208-D-LOGPROCREQ-DEFERRED: ...`) with a single call site:

```go
	p.logProcRequirement(errorNode, pt, analysis)
```

Note: the call site replaces the comment ONLY. Do NOT delete the existing `if isCorrupted && corruptedNode.Instruction != nil { ... }` HINT block above it — that's the corrupted-pair HINT which stays.

After the edit, the tail of `validatePointer` reads:

```go
	if isCorrupted && corruptedNode.Instruction != nil {
		p.diagnostics.Report(diagnostics.NewDiagnostic(
			corruptedNode.Instruction.Source,
			diagnostics.DiagnosticHint,
			diagnostics.MessagePointerCorruptedLoc,
			pt.Representation,
		))
	}
	p.logProcRequirement(errorNode, pt, analysis)
}
```

- [ ] **Step 4: Run the test to verify it passes**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run TestPointerChecker_Run_LogProcRequirement_DirectProcChain ./pkg/pack/compiler/cfg/ -v
```

Expected: PASS.

- [ ] **Step 5: Run the full cfg-package test suite to verify no regressions**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./pkg/pack/compiler/cfg/
```

Expected: PASS. The four pre-existing `TestPointerChecker_Run_*` tests should all stay green — they exercise single-script scripts with no Gosub, so `logProcRequirement` returns immediately on its `opcode != Gosub && opcode != Jump` guard.

- [ ] **Step 6: Commit**

```bash
cd /home/owner/Code/github.com/zsrv/goscape
git status
git add pkg/pack/compiler/cfg/pointer_checker.go pkg/pack/compiler/cfg/pointer_checker_validation_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(cfg): port logProcRequirement HINT chain (direct proc case)

Adds (*PointerChecker).logProcRequirement, wired at end of validatePointer.
Mirrors TS PointerChecker.ts:243-301 (RuneScriptTS). Walks the call graph
downward from the error node, emitting MessagePointerRequiredLoc HINT
diagnostics at every Gosub/Jump boundary where the missing pointer is
required. This commit covers the scriptPath-found branch (direct proc
requirement). The static-label-arg fallback branch lands in the next commit.

TS throws on script-lookup miss / nil source; goscape silently returns at
those points (no-panic posture per NAI-209-D-PUSHLONG-PANIC etc.).

Test: TestPointerChecker_Run_LogProcRequirement_DirectProcChain pins
one ERROR + one HINT for a caller→callee gosub where the callee requires
ACTIVE_PLAYER and the caller's trigger does not set it.
EOF
)"
git show --stat HEAD
```

Expected: commit succeeds with 2 files changed (pointer_checker.go + pointer_checker_validation_test.go), `git show --stat HEAD` confirms the new method + test were captured. If `git status` shows any other modified files, do NOT add them.

---

## Task 2: Two-hop recursion + static-label-arg fallback tests

**Files:**
- Modify: `pkg/pack/compiler/cfg/pointer_checker_validation_test.go` (add 2 new tests)

- [ ] **Step 1: Write the failing test for two-hop recursion**

Append to `pkg/pack/compiler/cfg/pointer_checker_validation_test.go`:

```go
// TestPointerChecker_Run_LogProcRequirement_RecursesAcrossTwoHops pins
// that the helper recurses across script boundaries: caller→mid→leaf,
// where only `leaf` requires ACTIVE_PLAYER, produces 1 ERROR (at caller's
// gosub-to-mid) + 2 HINTs (at mid's gosub-to-leaf, and at leaf's
// pointer-requiring instruction).
func TestPointerChecker_Run_LogProcRequirement_RecursesAcrossTwoHops(t *testing.T) {
	procTr := &trigger.TriggerType{ID: 0, Identifier: "proc"}

	// leaf — body: `p_kickout` (requires ACTIVE_PLAYER); trigger does NOT set it.
	leafSym := &symbol.ServerScriptSymbol{ScriptSymbolFields: symbol.ScriptSymbolFields{Trigger: procTr, Name: "leaf"}}
	leaf := codegen.NewRuneScript("test.rs2", leafSym, procTr, "leaf", nil)
	lb := codegen.NewBlock(&codegen.Label{Name: "entry"})
	require := makeCommandSymbol("p_kickout")
	lb.Add(codegen.Instruction{Opcode: codegen.Command, Operand: require})
	lb.Add(codegen.Instruction{Opcode: codegen.Return})
	leaf.Blocks = []*codegen.Block{lb}

	// mid — body: `~leaf`; trigger does NOT set ACTIVE_PLAYER.
	midSym := &symbol.ServerScriptSymbol{ScriptSymbolFields: symbol.ScriptSymbolFields{Trigger: procTr, Name: "mid"}}
	mid := codegen.NewRuneScript("test.rs2", midSym, procTr, "mid", nil)
	mb := codegen.NewBlock(&codegen.Label{Name: "entry"})
	mb.Add(codegen.Instruction{Opcode: codegen.Gosub, Operand: leafSym})
	mb.Add(codegen.Instruction{Opcode: codegen.Return})
	mid.Blocks = []*codegen.Block{mb}

	// caller — body: `~mid`; trigger does NOT set ACTIVE_PLAYER.
	callerSym := &symbol.ServerScriptSymbol{ScriptSymbolFields: symbol.ScriptSymbolFields{Trigger: procTr, Name: "caller"}}
	caller := codegen.NewRuneScript("test.rs2", callerSym, procTr, "caller", nil)
	ab := codegen.NewBlock(&codegen.Label{Name: "entry"})
	ab.Add(codegen.Instruction{Opcode: codegen.Gosub, Operand: midSym})
	ab.Add(codegen.Instruction{Opcode: codegen.Return})
	caller.Blocks = []*codegen.Block{ab}

	cp := map[string]*pointer.PointerHolder{
		"p_kickout": {Required: pointer.NewPointerSet(pointer.ActivePlayer)},
	}
	d := &diagnostics.Diagnostics{}
	pc := NewPointerChecker(d, []*codegen.RuneScript{leaf, mid, caller}, cp, semantics.StrictFeatureLevel{})
	pc.Run()

	// Note: depending on graph propagation, both `caller` and `mid` may
	// themselves produce a separate error+hint pair (they both "require"
	// ACTIVE_PLAYER via callee propagation). The deterministic invariant
	// is: `caller` produces at least one HINT chain of length 2 (mid's
	// gosub-to-leaf + leaf's p_kickout). Count total HINTs across all
	// scripts; demand at least 2.
	hints := hintDiagnostics(d)
	if len(hints) < 2 {
		t.Fatalf("got %d hint diagnostics, want at least 2 (recursion two hops): %v", len(hints), d.List())
	}
	// Every HINT must use MessagePointerRequiredLoc with "active_player".
	for i, h := range hints {
		msg := fmt.Sprintf(h.Message, h.MessageArgs...)
		if !strings.Contains(msg, "active_player required here") {
			t.Errorf("hint[%d] = %q, want substring \"active_player required here\"", i, msg)
		}
	}
}
```

- [ ] **Step 2: Run the new test to verify it passes (regression pin for the recursive impl from T1)**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run TestPointerChecker_Run_LogProcRequirement_RecursesAcrossTwoHops ./pkg/pack/compiler/cfg/ -v
```

Expected: PASS. The recursion was built into the helper in T1; this test pins that behavior.

If FAIL with fewer-than-2 HINTs: the recursion isn't firing. Double-check the `p.logProcRequirement(required, pt, analysis)` line in `pointer_checker.go` is present at the end of the `scriptPath != nil` branch (NOT only inside the `scriptPath == nil` branch).

- [ ] **Step 3: Write the failing test for the static-label-arg fallback**

Append to `pkg/pack/compiler/cfg/pointer_checker_validation_test.go`:

```go
// TestPointerChecker_Run_LogProcRequirement_StaticLabelArgFallback pins
// the helper's scriptPath==nil branch: when the called script does NOT
// directly require the pointer but is passed a label-typed static arg
// whose label DOES require it, a HINT is emitted at the jump-param node
// inside the called script. Fixture mirrors
// TestPointerChecker_LabelJump_RequirementPropagates from
// pointer_checker_labels_test.go.
func TestPointerChecker_Run_LogProcRequirement_StaticLabelArgFallback(t *testing.T) {
	procTr := &trigger.TriggerType{ID: 0, Identifier: "proc"}
	labelTr := &trigger.TriggerType{ID: 1, Identifier: "label", Pointers: pointer.NewPointerSet(pointer.ActivePlayer)}

	// label script — requires ACTIVE_PLAYER.
	labelSym := &symbol.ServerScriptSymbol{ScriptSymbolFields: symbol.ScriptSymbolFields{Trigger: labelTr, Name: "mylabel"}}
	labelScript := codegen.NewRuneScript("test.rs2", labelSym, labelTr, "mylabel", nil)
	lb := codegen.NewBlock(&codegen.Label{Name: "entry"})
	require := makeCommandSymbol("p_kickout")
	lb.Add(codegen.Instruction{Opcode: codegen.Command, Operand: require})
	lb.Add(codegen.Instruction{Opcode: codegen.Return})
	labelScript.Blocks = []*codegen.Block{lb}

	// consumer — accepts a label parameter, jumps to it.
	labelMetaType := typ.NewMetaScript("label", typ.PrimitiveInt, typ.PrimitiveInt)
	consumerSym := &symbol.ServerScriptSymbol{
		ScriptSymbolFields: symbol.ScriptSymbolFields{
			Trigger:    procTr,
			Name:       "consumer",
			Parameters: labelMetaType,
		},
	}
	consumer := codegen.NewRuneScript("test.rs2", consumerSym, procTr, "consumer", nil)
	cb := codegen.NewBlock(&codegen.Label{Name: "entry"})
	labelParam := &symbol.LocalVariableSymbol{Name: "lbl", Type: labelMetaType}
	consumer.Locals = &codegen.LocalTable{
		Parameters: []*symbol.LocalVariableSymbol{labelParam},
		All:        []*symbol.LocalVariableSymbol{labelParam},
	}
	jumpCmd := makeCommandSymbol("jump")
	cb.Add(codegen.Instruction{Opcode: codegen.PushLocalVar, Operand: labelParam})
	cb.Add(codegen.Instruction{Opcode: codegen.Command, Operand: jumpCmd})
	cb.Add(codegen.Instruction{Opcode: codegen.Return})
	consumer.Blocks = []*codegen.Block{cb}

	// caller — gosubs consumer with .mylabel as the static arg.
	callerSym := &symbol.ServerScriptSymbol{ScriptSymbolFields: symbol.ScriptSymbolFields{Trigger: procTr, Name: "caller"}}
	caller := codegen.NewRuneScript("test.rs2", callerSym, procTr, "caller", nil)
	calb := codegen.NewBlock(&codegen.Label{Name: "entry"})
	calb.Add(codegen.Instruction{Opcode: codegen.PushConstantSymbol, Operand: labelSym})
	calb.Add(codegen.Instruction{Opcode: codegen.Gosub, Operand: consumerSym})
	calb.Add(codegen.Instruction{Opcode: codegen.Return})
	caller.Blocks = []*codegen.Block{calb}

	cp := map[string]*pointer.PointerHolder{
		"p_kickout": {Required: pointer.NewPointerSet(pointer.ActivePlayer)},
	}
	d := &diagnostics.Diagnostics{}
	pc := NewPointerChecker(d, []*codegen.RuneScript{labelScript, consumer, caller}, cp, semantics.StrictFeatureLevel{})
	pc.Run()

	// Expect at least one ERROR (caller's gosub site, via label-propagation)
	// and at least one HINT (jump-param node inside consumer with
	// MessagePointerRequiredLoc).
	if len(errorDiagnostics(d)) == 0 {
		t.Fatalf("expected at least one error diagnostic; got %v", d.List())
	}
	hints := hintDiagnostics(d)
	if len(hints) == 0 {
		t.Fatalf("got 0 hint diagnostics, want at least 1 (static-label-arg fallback HINT inside consumer): %v", d.List())
	}
	foundRequiredLoc := false
	for _, h := range hints {
		msg := fmt.Sprintf(h.Message, h.MessageArgs...)
		if strings.Contains(msg, "active_player required here") {
			foundRequiredLoc = true
			break
		}
	}
	if !foundRequiredLoc {
		t.Errorf("no hint with \"active_player required here\" message; hints=%v", hints)
	}
}
```

- [ ] **Step 4: Run the new test — should pass (impl from T1 already covers the fallback branch)**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run TestPointerChecker_Run_LogProcRequirement_StaticLabelArgFallback ./pkg/pack/compiler/cfg/ -v
```

Expected: PASS. The static-label-arg fallback branch was built into the T1 method body (the `if scriptPath == nil { ... }` block).

If FAIL with 0 hints: the fallback branch returned early. Verify that:
- `analysis.staticLabelArgsByCall[inst]` is populated for the caller's Gosub instruction (the framework wires this in `getAnalysis` via `buildStaticLabelArgsByCall`).
- `p.GetPointers(labelSym).Required.Has(pt)` returns true for `pt = ActivePlayer`.

- [ ] **Step 5: Run the full cfg-package test suite**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./pkg/pack/compiler/cfg/
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git status
git add pkg/pack/compiler/cfg/pointer_checker_validation_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
test(cfg): pin recursive + static-label HINT chain (logProcRequirement)

Adds two regression tests for (*PointerChecker).logProcRequirement
landed in the previous commit:

- TestPointerChecker_Run_LogProcRequirement_RecursesAcrossTwoHops:
  pins the recursive descent — caller→mid→leaf produces at least 2
  HINTs (mid's gosub-to-leaf + leaf's p_kickout site).

- TestPointerChecker_Run_LogProcRequirement_StaticLabelArgFallback:
  pins the scriptPath==nil branch — gosub passes a label-typed static
  arg whose label requires ACTIVE_PLAYER; HINT fires at the jump-param
  node inside the called script.

No production code change.
EOF
)"
git show --stat HEAD
```

Expected: 1 file changed.

---

## Task 3: Retire `NAI-208-D-LOGPROCREQ-DEFERRED` from pin walker

**Files:**
- Modify: `pkg/pack/compiler/cfg/nai208_deviation_pins_test.go:124-136`

- [ ] **Step 1: Run the grep-walker pin test to confirm current state**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run TestPin_NAI208_GrepWalker ./pkg/pack/compiler/cfg/ -v
```

Expected: PASS at HEAD (T2). The tag `NAI-208-D-LOGPROCREQ-DEFERRED` still appears in `pointer_checker.go`? Let me verify with grep first:

```bash
grep -rn "NAI-208-D-LOGPROCREQ-DEFERRED" --include="*.go" .
```

Expected at HEAD T2: zero matches in `pointer_checker.go` (deleted in T1 Step 3) but ONE match in `nai208_deviation_pins_test.go:134` (the slice entry itself). The grep-walker would now fail because it's looking for a tag that only appears in its own slice — and `filepath.Walk` includes the test file in its walk, so the walker's own self-reference satisfies the search. The walker should PASS at HEAD T2.

If the grep-walker FAILS at HEAD T2 (i.e., the tag isn't found in any .go file because the walker's `filepath.SkipDir` skips its own file): check the walker logic. Otherwise proceed.

- [ ] **Step 2: Remove the tag entry from the slice**

Edit `pkg/pack/compiler/cfg/nai208_deviation_pins_test.go` at L134. Delete the line:

```go
		"NAI-208-D-LOGPROCREQ-DEFERRED",
```

The slice should now have 10 entries instead of 11. Confirm with `grep -c '"NAI-208-D-' pkg/pack/compiler/cfg/nai208_deviation_pins_test.go` — expect output `10` (plus whatever non-slice references exist; should be the test function name only).

- [ ] **Step 3: Run the grep-walker test again to verify it stays green**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run TestPin_NAI208_GrepWalker ./pkg/pack/compiler/cfg/ -v
```

Expected: PASS.

- [ ] **Step 4: Verify zero tag references repo-wide**

```bash
grep -rn "NAI-208-D-LOGPROCREQ-DEFERRED" --include="*.go" .
```

Expected: zero matches.

- [ ] **Step 5: Run the full cfg-package + parent test suite**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./pkg/pack/compiler/cfg/ ./pkg/pack/compiler/...
```

Expected: PASS across all packages under `pkg/pack/compiler/`.

- [ ] **Step 6: Commit**

```bash
git status
git add pkg/pack/compiler/cfg/nai208_deviation_pins_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
chore(cfg): retire NAI-208-D-LOGPROCREQ-DEFERRED from pin walker

The deviation was retired by the logProcRequirement port in commits
<T1-sha> and <T2-sha> (recursive HINT chain + static-label-arg fallback,
mirroring RuneScriptTS PointerChecker.ts:243-301). Drops the tag from
the grep-walker slice in nai208_deviation_pins_test.go.

After this commit: zero NAI-208-D-LOGPROCREQ-DEFERRED references repo-wide.
EOF
)"
git show --stat HEAD
```

Before committing, replace `<T1-sha>` and `<T2-sha>` with the actual commit SHAs from `git log --oneline -3`.

---

## Task 4: NAI-212 doc-comment refresh

**Files:**
- Modify: `pkg/pack/compiler/symbols.go:594-601`
- Modify: `pkg/pack/compiler/symbols_test.go:451-455`

- [ ] **Step 1: Replace the stale `populateInterfaceOverlay` doc comment**

In `pkg/pack/compiler/symbols.go`, locate the comment block at L594-601 that currently reads:

```go
// NAI-212-D-INTERFACE-FALLBACK-FROM-COMPONENTINFO: with packClientInterface
// deferred (see pack_all.go NAI-212-D-CLIENT-PACKERS-DEFERRED), the cache
// is empty when compile runs and Component.get-equivalent has no data.
// TS would also skip in that state, but TS never reaches it because its
// packAll runs packClientInterface first. Goscape falls back to populating
// interfaceInfo from componentInfo.Map alone (base names, no ComName
// override, no overlay flag) so `interface`-typed identifier lookups
// resolve to the right BasicSymbol. Retires when packClientInterface lands.
```

Replace it with:

```go
// NAI-212-D-INTERFACE-FALLBACK-FROM-COMPONENTINFO: defensive fallback
// when loaders.comp is empty or missing entries. When compile runs
// inside packall.PackAll, packall calls clientinterface.Pack first
// (packall/packall.go:51) so loaders.comp is populated and the
// fallback is dormant. Standalone callers — primarily
// `goscape-cli compile` via compiler.LoadCompilerSymbols
// (cmd_compile.go:91) — do NOT run clientinterface.Pack first, and
// LoadComponentTypes returns empty configs when the client/interface
// jagfile is missing (componenttype.go:133-134). In that state the
// fallback populates interfaceInfo from componentInfo.Map alone (base
// names, no ComName override, no overlay flag) so `interface`-typed
// identifier lookups still resolve to a BasicSymbol. Permanent —
// removing the fallback would break `goscape-cli compile` on a fresh
// dataPackDir.
```

- [ ] **Step 2: Replace the stale test-file doc comment**

In `pkg/pack/compiler/symbols_test.go`, locate the comment block at L451-455 (above `TestPopulateInterfaceOverlay_NilConfig_FallsBack`) that currently reads:

```go
// TestPopulateInterfaceOverlay_NilConfig_FallsBack pins that when the
// cache is empty (packClientInterface deferred), populateInterfaceOverlay
// still populates interfaceInfo from componentInfo.Map alone using the
// base name. See NAI-212-D-INTERFACE-FALLBACK-FROM-COMPONENTINFO in
// symbols.go. Retires alongside the deferral once packAll runs
// packClientInterface first.
```

Replace it with:

```go
// TestPopulateInterfaceOverlay_NilConfig_FallsBack pins the defensive
// fallback path: when loaders.comp is empty (standalone `goscape-cli
// compile` against a fresh dataPackDir), populateInterfaceOverlay
// populates interfaceInfo from componentInfo.Map alone using the base
// name. Permanent pin for NAI-212-D-INTERFACE-FALLBACK-FROM-COMPONENTINFO.
```

- [ ] **Step 3: Verify phantom references are gone**

```bash
grep -rn "NAI-212-D-CLIENT-PACKERS-DEFERRED" --include="*.go" .
grep -rn "pack_all.go" --include="*.go" .
```

Expected:
- First grep: zero matches.
- Second grep: zero matches (or only legitimate non-comment uses; expected to be zero).

- [ ] **Step 4: Verify NAI-212 pin tests still pass (the comment refresh kept the canonical tag string)**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run "TestPopulateInterfaceOverlay|TestNAI212" ./pkg/pack/compiler/... ./pkg/pack/...
```

Expected: PASS. Specifically:
- `TestPopulateInterfaceOverlay_NilConfig_FallsBack` still references `NAI-212-D-INTERFACE-FALLBACK-FROM-COMPONENTINFO` (the canonical tag stayed in the new comment text).
- `pkg/pack/nai212_deviation_pins_test.go` tests are unaffected (they pin different tags in different files).

- [ ] **Step 5: Run the full pack-package test suite**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./pkg/pack/...
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git status
git add pkg/pack/compiler/symbols.go pkg/pack/compiler/symbols_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
docs(compiler): refresh NAI-212 populateInterfaceOverlay doc comment

The doc comment at symbols.go:594-601 contained two stale references:
- `pack_all.go` (file doesn't exist; actual file is packall/packall.go)
- `NAI-212-D-CLIENT-PACKERS-DEFERRED` (phantom tag, never used canonically)

It also claimed "Retires when packClientInterface lands" — but
clientinterface.Pack DID land (pkg/pack/clientinterface/pack.go) and is
wired into packall.PackAll before compiler.RunServerCompiler.

Recharacterizes the fallback as PERMANENT. The fallback is still
load-bearing for standalone `goscape-cli compile` (cmd_compile.go:91
calls LoadCompilerSymbols without running clientinterface.Pack first;
LoadComponentTypes returns empty configs on missing client/interface
jagfile). Naming the actual standalone caller in the comment makes the
justification concrete.

Refreshes the sibling comment block above
TestPopulateInterfaceOverlay_NilConfig_FallsBack to match.

The canonical tag NAI-212-D-INTERFACE-FALLBACK-FROM-COMPONENTINFO stays
in place (pinned by the test); only the surrounding prose changes.
EOF
)"
git show --stat HEAD
```

---

## Task 5: Close — gates + memory + close commit

**Files:**
- Create: `/home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/compiler_deferrals_cleanup_close.md`
- Modify: `/home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/MEMORY.md` (add top entry)

- [ ] **Step 1: Run the full `-race` gate**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./...
```

Expected: 56+ packages OK / 0 FAIL. Note the elapsed time and key package times (modules/world, pkg/pack/compiler/cfg) for the close memo.

- [ ] **Step 2: Run the smoke-pack gate**

```bash
unset GOROOT; export PATH="/home/owner/go/current/bin:$PATH"
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go run -trimpath ./cmd/goscape-cli smoke-pack --content-dir /home/owner/Code/github.com/LostCityRS/content
```

Expected: `Result: 12 OK, 0 ERR, 0 SKIP` line. Note the total elapsed time.

- [ ] **Step 3: Verify final tag-cleanup gates**

```bash
echo "=== logproc deferral ===" && grep -rn "NAI-208-D-LOGPROCREQ-DEFERRED" --include="*.go" .
echo "=== client-packers phantom ===" && grep -rn "NAI-212-D-CLIENT-PACKERS-DEFERRED" --include="*.go" .
echo "=== pack_all.go phantom ===" && grep -rn "pack_all.go" --include="*.go" .
echo "=== canonical NAI-212 fallback tag (should still appear) ===" && grep -rn "NAI-212-D-INTERFACE-FALLBACK-FROM-COMPONENTINFO" --include="*.go" .
```

Expected:
- Sections 1, 2, 3: zero matches.
- Section 4: at least 2 matches (in `symbols.go` and `symbols_test.go`).

- [ ] **Step 4: Capture the commit range for the close memo**

```bash
git log --oneline 9955fdc0..HEAD
```

Expected: 4 commits (T1, T2, T3, T4) plus this close commit will land at Step 6. Note the SHAs.

- [ ] **Step 5: Write the close memo**

Create `/home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/compiler_deferrals_cleanup_close.md`:

```markdown
---
name: compiler-deferrals-cleanup-close
description: Post-runtime-fixups compiler-deferrals-cleanup slice — NAI-208 logProcRequirement port + NAI-212 fallback recharacterization, 4 impl + 1 close commit on top of [[drop-unused-log-field-cleanup]]
metadata:
  type: project
---

Compiler deferrals cleanup shipped 2026-05-20 across 4 impl commits + 1 close on top of `9955fdc0` (drop unused log field cleanup).

**Item A — NAI-208 logProcRequirement port:**
- New `(*PointerChecker).logProcRequirement(node, pt, analysis)` method in `pkg/pack/compiler/cfg/pointer_checker.go` (~55 LOC). Mirrors TS `RuneScriptTS/src/compiler/codegen/script/config/PointerChecker.ts:243-301`.
- Recursive descent across script boundaries via `scriptsBySymbol` lookup. Both branches: (a) `scriptPath != nil` emits HINT at `scriptPath[0]` and recurses; (b) `scriptPath == nil` falls back to `staticLabelArgsByCall` and per-label-param HINT at jump-param node.
- Wired at end of `validatePointer` (replaces 5-line deferral comment block).
- Sorted iteration of `staticArgs` map for deterministic HINT order (mirrors `NAI-210-D-LOADER-SORTED-ITERATION` posture).
- TS panics on script-lookup miss / nil instruction source; goscape silently returns (no-panic posture per `NAI-209-D-PUSHLONG-PANIC` etc.).
- 3 new tests in `pointer_checker_validation_test.go`: DirectProcChain (1 HINT), RecursesAcrossTwoHops (≥2 HINTs), StaticLabelArgFallback (≥1 HINT at jump-param node).
- New test helper `hintDiagnostics(d)` extracts HINT-severity entries (sibling to existing `errorDiagnostics`).
- Tag `NAI-208-D-LOGPROCREQ-DEFERRED` dropped from `nai208_deviation_pins_test.go` slice (11→10 entries).
- **Zero NAI-208-D-LOGPROCREQ-DEFERRED references repo-wide after T3.**

**Item B — NAI-212 fallback recharacterization (doc-only):**
- `populateInterfaceOverlay` doc comment in `pkg/pack/compiler/symbols.go:594-601` refreshed: removed broken `pack_all.go` reference, removed phantom `NAI-212-D-CLIENT-PACKERS-DEFERRED` reference, recharacterized as permanent (defensive fallback for standalone `goscape-cli compile` via `cmd_compile.go:91 → LoadCompilerSymbols`).
- Sibling comment block in `symbols_test.go:451-455` above `TestPopulateInterfaceOverlay_NilConfig_FallsBack` refreshed with matching prose.
- Canonical tag `NAI-212-D-INTERFACE-FALLBACK-FROM-COMPONENTINFO` preserved (still pinned by the test).
- **Zero `NAI-212-D-CLIENT-PACKERS-DEFERRED` and `pack_all.go` references repo-wide after T4.**

**Gates at HEAD:** `-race ./...` 56+ pkgs OK / 0 FAIL; smoke-pack 12 OK / 0 ERR / 0 SKIP.

**Retires:** `NAI-208-D-LOGPROCREQ-DEFERRED` (sole real retirement). Removes 2 phantom string references (`NAI-212-D-CLIENT-PACKERS-DEFERRED`, `pack_all.go`).

**Opens:** none.

**Next pivot candidates:**
- `NAI-211-D-MACRO-LOOKUP-DEFERRED` — explicitly deferred from this slice; needs macros to be ported first.
- See [[post-runtime-fixups-cluster-close]] for the broader pivot menu.
```

Replace `[[drop-unused-log-field-cleanup]]` with a placeholder if no memory file exists for that yet — the cleanup was a one-off commit `9955fdc0` and may not have a dedicated memo. If so, simply reference the commit SHA: "on top of `9955fdc0` (drop-unused-log-field cleanup commit)".

- [ ] **Step 6: Add the MEMORY.md top-entry pointer**

Edit `/home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/MEMORY.md`. Insert a new entry at the top of the entries list (immediately after any existing top-of-file content). The new line:

```markdown
- [Compiler deferrals cleanup close](compiler_deferrals_cleanup_close.md) — NAI-208 logProcRequirement port (~55 LOC + 3 tests) + NAI-212 fallback recharacterization (doc-only); 4 impl + 1 close commit on top of 9955fdc0; -race clean 56+ pkgs; smoke-pack 12 OK / 0 ERR / 0 SKIP; retires NAI-208-D-LOGPROCREQ-DEFERRED + removes 2 phantom string references; NAI-211-D-MACRO-LOOKUP-DEFERRED deferred to macro-port slice
```

Verify line is ≤200 chars per MEMORY.md conventions (count it; trim if needed).

- [ ] **Step 7: Make the close commit (empty summary commit per project convention)**

The spec and plan docs are committed in their own commits BEFORE the slice begins (see the runtime-fixups slice predecessors `0329b352 docs: spec` and `0ca0ee4c docs: plan`). The close commit itself is an empty commit summarizing what shipped — the close memo lives outside the git tree at `~/.claude/projects/.../memory/`.

```bash
git status
git commit --no-gpg-sign --allow-empty -m "$(cat <<'EOF'
chore(close): compiler-deferrals-cleanup — NAI-208 logProcRequirement + NAI-212 doc refresh

Bundles two compiler-deferral cleanups that surfaced from the post-
runtime-fixups deviation audit. Neither individually justified a slice.

  A. NAI-208 logProcRequirement port — recursive POINTER_REQUIRED_LOC
     HINT chain mirrors RuneScriptTS src/compiler/codegen/script/
     config/PointerChecker.ts:243-301. New (*PointerChecker).
     logProcRequirement (~55 LOC) composes scriptsBySymbol +
     requiresPointerPathScript + staticLabelArgsByCall +
     getJumpParamNodes + GetPointers + requiresPointerAtNodes.
     Single call site at end of validatePointer. 3 new tests in
     pointer_checker_validation_test.go.
  B. NAI-212 doc refresh — populateInterfaceOverlay comment in
     symbols.go:594-601 (+ sibling test comment) replaced. Removed
     broken pack_all.go reference, removed phantom
     NAI-212-D-CLIENT-PACKERS-DEFERRED reference. Recharacterized
     NAI-212-D-INTERFACE-FALLBACK-FROM-COMPONENTINFO as permanent
     (defensive fallback for goscape-cli compile → LoadCompilerSymbols
     standalone path that doesn't run clientinterface.Pack first).

Shipped across:
  feat(cfg): port logProcRequirement HINT chain (direct proc case)  [<T1-sha>]
  test(cfg): pin recursive + static-label HINT chain                [<T2-sha>]
  chore(cfg): retire NAI-208-D-LOGPROCREQ-DEFERRED from pin walker  [<T3-sha>]
  docs(compiler): refresh NAI-212 populateInterfaceOverlay comment  [<T4-sha>]

Retires NAI-208-D-LOGPROCREQ-DEFERRED. Two phantom string references
removed (NAI-212-D-CLIENT-PACKERS-DEFERRED, pack_all.go).

Gates: -race ./... clean across 56+ pkgs; smoke-pack 12 OK / 0 ERR /
0 SKIP.

Grep gates clean:
  - NAI-208-D-LOGPROCREQ-DEFERRED        --include="*.go" → 0
  - NAI-212-D-CLIENT-PACKERS-DEFERRED    --include="*.go" → 0
  - pack_all.go                          --include="*.go" → 0

NAI-211-D-MACRO-LOOKUP-DEFERRED explicitly deferred to a future
macro-port slice (blocked on macros being ported first).

Closes memory:
- compiler-deferrals-cleanup-close
EOF
)"
git show --stat HEAD
```

Before committing, replace `<T1-sha>` … `<T4-sha>` with the actual SHAs from `git log --oneline -5`. Replace the captured gate numbers (wall times, exact pkg counts) with the real values from T5 Steps 1+2.

- [ ] **Step 8: Final repo-wide health check**

```bash
git status
git log --oneline 9955fdc0..HEAD
```

Expected:
- `git status` shows nothing staged or modified (apart from the standing untracked noise per `[[post-runtime-fixups-cluster-close]]` resume doc).
- `git log` shows 5 commits since predecessor `9955fdc0` (T1, T2, T3, T4, T5-close).

---

## Self-review notes for the executor

- **Sequential tasks** (T1→T2→T3): all touch `pkg/pack/compiler/cfg/pointer_checker*.go`. Do NOT parallelize.
- **T4 is independent** of T1-T3 but the close memo in T5 captures the full commit range — run T4 between T3 and T5.
- **`hintDiagnostics` helper:** if the existing `diagnostics.Diagnostic` API has an `IsHint()` method, prefer it over field access (see existing `errorDiagnostics` helper at `pointer_checker_validation_test.go:145` which uses `e.IsError()` rather than direct field access). Check `pkg/pack/compiler/diagnostics/diagnostic.go` once at T1 Step 1 and use the right form throughout.
- **`sort.Ints` import:** the `pointer_checker.go` file may not yet import `"sort"`. Check existing imports before adding. If `sort` is already imported elsewhere in the file (for any reason), no edit needed beyond verification.
- **`pointer.PointerSet.Has` vs direct map access:** confirm the field name with `grep -n "Has\|Contains" pkg/pack/compiler/pointer/*.go`. The spec uses `Required.Has(pt)`; adjust if the actual API differs (e.g., `Required.Contains(pt)` or `Required[pt]`).
- **Test fixture details:** `pointer_checker_labels_test.go:19-75` (`TestPointerChecker_LabelJump_RequirementPropagates`) is the closest analogue for the T2 Step 3 static-label-arg fallback test — refer to it if `labelMetaType` / `LocalTable` construction needs detail.
- **Pre-commit `git status` paranoia:** per `[[git-pre-commit-status-check]]` feedback, concurrent shell sessions can stage things between the snapshot you read and the `git commit` call. Always re-run `git status` immediately before each commit (the Step blocks include this) and follow each commit with `git show --stat HEAD` to confirm what landed.
- **`--no-gpg-sign`** is mandatory per the project CLAUDE.md (global) — never omit it.
- **Per `[[git-pre-commit-status-check]]`:** every per-task code-quality review historically found at least one Important-level issue in recent slices. Budget for ~1 fixup commit per task if reviews are run. Don't `--amend` — repo convention forbids it.
- **Test for `TestPointerChecker_LabelJump_DisableStaticLabelArgPropagation`:** this existing test (at `pointer_checker_labels_test.go:82-134`) sets `feats.DisableStaticLabelArgPropagation: true`. Confirm at T2 Step 5 (full suite run) that this test still passes — `logProcRequirement` should respect the same feature gate by virtue of `analysis.staticLabelArgsByCall` being empty when the flag is set (verify via `pointer_checker.go:425-427` where the map is conditionally populated). If the test starts failing, the new helper is over-emitting and needs a feature-flag guard.
- **Cyclic call graph risk:** the spec §7 flags that recursive `logProcRequirement` could stack-overflow on a cyclic call graph (A gosubs B, B gosubs A). The current implementation does NOT have a visited-set guard. TS doesn't have one either. If the smoke-pack at T5 Step 2 OOMs or stack-overflows, add a `visited map[symbol.Symbol]struct{}` parameter to `logProcRequirement` and short-circuit on re-entry. The spec deems this defensive (no real-world script does this), so the impl ships without the guard; add only if production content surfaces it.
