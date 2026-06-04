# NAI-124 Stage 2 — SCALE opcode operand swap

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix the SCALE runescript opcode so it computes the TS-faithful `(a*c)/b` instead of goscape's incorrect `(a*b)/c`. This binds the bronze-dagger-vs-rat "always 3 damage" smoke residual surfaced at NAI-123 close (`b7c16b0`).

**Architecture:** Single-file production fix at `pkg/script/handlers_number.go:128-136`. Test updates at `pkg/script/handlers_number_test.go`: rewrite the one bug-pinning case to the smoke-trace inputs, add a TS-semantic positive-direction case, and add a `TestScaleDivideByZeroAborts` mirroring the existing `TestDivideByZeroAborts` pattern. Single bundle, single Sonnet implementer + Sonnet reviewer per `superpowers_code_reviewer_model`.

**Tech Stack:** Go 1.26+. No new dependencies.

---

## Background

NAI-124 Stage 1 (controller-direct, see `docs/superpowers/findings/2026-05-08-nai-124-stage1-findings.md` at `01178eb`) audited 6 risk surfaces and refuted the spec's lead hypothesis (S1 sign-extension). The smoke-binding root cause is **S5 — SCALE operand swap** at `pkg/script/handlers_number.go:135`.

**TS reference** at `LostCityRS/Engine-TS/src/engine/script/handlers/NumberOps.ts:124-127`:

```ts
[ScriptOpcode.SCALE]: state => {
    const [a, b, c] = state.popInts(3);
    state.pushInt((a * c) / b);
},
```

**Goscape current** at `pkg/script/handlers_number.go:128-136`:

```go
func handleScale(s *ScriptState) error {
    c := s.PopInt()
    b := s.PopInt()
    a := s.PopInt()
    if c == 0 {
        return errors.New("SCALE: division by zero")
    }
    s.PushInt(floorDiv(a*b, c))
    return nil
}
```

For runescript `scale(value, max, newMax)` (left-to-right push → bottom-to-top pop): `a=value`, `b=max`, `c=newMax`. TS computes `value*newMax/max` (standard runescript scale); goscape computes `value*max/newMax` (operands swapped). The divisor is `b` (= `max`) in TS, not `c`.

**Smoke trace.** `Content/scripts/skill_combat/scripts/combat.rs2:5` calls `scale(max(100, $prayerbonus), 100, $stat_level)`. For a level-1 fresh char with no prayer: `scale(100, 100, 1)`.
- TS: `(100 * 1) / 100 = 1` → `$effective_strength = 9` → `%com_maxhit ≈ 1`.
- Goscape (current): `(100 * 100) / 1 = 10000` → `%com_maxhit ≈ 1063` → `randominc(min(1063, 1000))` produces nearly all hits ≥ 3, clamped to rat HP=3. Hence "always 3".

**S1 + S2 (paramtype default-branch sign-extension at `handlers_config.go:51`, `handlers_inv.go:256`, `npc_hunt.go:297`; plus `paramtype.go:111` `uint32` storage)** are real divergences but NOT contributors to this smoke (every relevant ParamType has a non-negative configured default). They route to NAI-125 follow-up per `cascade_theory_smoke_binding`. **Do not bundle them into this Stage 2.**

## File Structure

- Modify: `pkg/script/handlers_number.go` — `handleScale` operand swap and divide-by-zero predicate.
- Modify: `pkg/script/handlers_number_test.go` — update bug-pinning case at line 46; add positive TS-semantic case; add `TestScaleDivideByZeroAborts`.

## Pre-flight (controller did this; implementer reads only)

Re-grepped `pkg/script/handlers_number.go:128-136` and `pkg/script/handlers_number_test.go:46` against HEAD `01178eb`. Cited shapes match exactly. No drift since Stage 1.

---

## Task 1: TDD — pin the smoke trace and TS-semantic positive direction

**Files:**
- Modify: `pkg/script/handlers_number_test.go:46`

- [ ] **Step 1: Replace the existing bug-pinning case with two TS-semantic cases**

The existing case at line 46 is:

```go
{"scale 3/4 of 200", OpScale, []int{200, 3, 4}, 150},
```

This pins the bug: the test author intended `(value, num, denom)` → `200*3/4 = 150`. With TS semantics `(value, max, newMax)` → `value*newMax/max`, the same inputs `{200, 3, 4}` produce `200*4/3 = 266`. We replace this single line with two cases — one pinning the smoke trace exactly, and one with a clean integer answer in the TS-semantic shape.

In `pkg/script/handlers_number_test.go`, change line 46:

```go
		{"scale 3/4 of 200", OpScale, []int{200, 3, 4}, 150},
```

to:

```go
		{"scale (100,100,1) smoke trace", OpScale, []int{100, 100, 1}, 1},
		{"scale value*newMax/max", OpScale, []int{200, 4, 3}, 150},
```

The first case is the bronze-dagger-vs-rat smoke trace from `combat.rs2:5` (`scale(max(100, prayerbonus), 100, stat_level)` for a level-1 fresh char with no prayer): TS computes `(100*1)/100 = 1`, goscape currently computes `(100*100)/1 = 10000`. The second case uses TS-faithful runescript semantics `(value, max, newMax) → value*newMax/max`: `200*3/4 = 150` (the original author's "3/4 of 200" intent, expressed in correct semantics).

- [ ] **Step 2: Run the tests to verify they FAIL against current code**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestNumberHandlers -v`

Expected: FAIL on both new cases.

- `scale (100,100,1) smoke trace`: got `10000`, want `1` (current `(100*100)/1`).
- `scale value*newMax/max`: got `266` (current `(200*4)/3`, floored), want `150`.

This proves the test cases pin the bug before we fix it.

- [ ] **Step 3: Commit the failing tests**

```bash
git add pkg/script/handlers_number_test.go
git commit --no-gpg-sign -m "test(nai-124): Stage 2 — pin SCALE smoke trace and TS-semantic case (RED)"
```

---

## Task 2: Add SCALE divide-by-zero abort test

**Files:**
- Modify: `pkg/script/handlers_number_test.go`

- [ ] **Step 1: Add `TestScaleDivideByZeroAborts` mirroring the existing `TestDivideByZeroAborts`**

The existing `TestDivideByZeroAborts` at `pkg/script/handlers_number_test.go:81-98` is the template. After the fix, SCALE's divisor is `b` (the second-popped operand, which is the runescript `max` argument), not `c`. We add a new test in the same shape.

Insert this function immediately after `TestDivideByZeroAborts` (after the closing `}` at line 98):

```go
func TestScaleDivideByZeroAborts(t *testing.T) {
	sf := &ScriptFile{
		Name:             "scalezero",
		Opcodes:          []Opcode{OpScale, OpReturn},
		IntOperands:      []int32{0, 0},
		StringOperands:   []string{"", ""},
		InstructionCount: 2,
	}
	state := Init(sf, nil, false, nil, nil)
	// scale(value=100, max=0, newMax=5) — divisor is max (the second-popped operand).
	state.PushInt(100)
	state.PushInt(0)
	state.PushInt(5)
	if err := Execute(state); err == nil {
		t.Fatal("Execute: want error on SCALE divide by zero")
	}
	if state.Execution != Aborted {
		t.Errorf("Execution: got %v, want Aborted", state.Execution)
	}
}
```

- [ ] **Step 2: Run the new test to verify it FAILS against current code**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestScaleDivideByZeroAborts -v`

Expected: FAIL. Current code's `if c == 0` does NOT trigger when `b == 0` (current `c` is the top-popped operand, which is `5` here, non-zero), so the test sees `Execute` return nil (no error) when it expects an error. With inputs `{100, 0, 5}`, current code computes `floorDiv(100*0, 5) = 0` and pushes `0` — no abort.

This proves the test pins the divide-by-zero predicate's wrongness too.

- [ ] **Step 3: Commit the failing test**

```bash
git add pkg/script/handlers_number_test.go
git commit --no-gpg-sign -m "test(nai-124): Stage 2 — pin SCALE divide-by-zero on b (RED)"
```

---

## Task 3: GREEN — fix `handleScale` operands and divide-by-zero predicate

**Files:**
- Modify: `pkg/script/handlers_number.go:128-136`

- [ ] **Step 1: Apply the operand-swap fix**

Edit `pkg/script/handlers_number.go`. Replace:

```go
func handleScale(s *ScriptState) error {
	c := s.PopInt()
	b := s.PopInt()
	a := s.PopInt()
	if c == 0 {
		return errors.New("SCALE: division by zero")
	}
	s.PushInt(floorDiv(a*b, c))
	return nil
}
```

with:

```go
func handleScale(s *ScriptState) error {
	// SCALE: TS NumberOps.ts:124-127 — pushInt((a*c)/b).
	// Runescript scale(value, max, newMax) → value*newMax/max.
	c := s.PopInt()
	b := s.PopInt()
	a := s.PopInt()
	if b == 0 {
		return errors.New("SCALE: division by zero")
	}
	s.PushInt(floorDiv(a*c, b))
	return nil
}
```

Three changes inside the function body: divisor predicate `c == 0` → `b == 0`; arithmetic `a*b, c` → `a*c, b`; new doc-comment cross-referencing the TS source.

- [ ] **Step 2: Run the SCALE-related tests to verify they PASS**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run "TestNumberHandlers|TestScaleDivideByZeroAborts" -v`

Expected: PASS for `scale (100,100,1) smoke trace` (now `(100*1)/100 = 1`), `scale value*newMax/max` (now `(200*3)/4 = 150`), and `TestScaleDivideByZeroAborts` (now aborts on `b == 0`). All other `TestNumberHandlers` subtests still pass (unchanged).

- [ ] **Step 3: Run the full pkg/script suite to catch regressions**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -v`

Expected: ALL tests PASS. If any `pkg/script/` test that uses SCALE indirectly was relying on the old swapped semantics, surface and report it — do NOT silently update the expectation. Per `latent_bug_at_migration_boundary`, latent bugs may surface at the cutover; if so, document and route per `cascade_theory_smoke_binding`.

- [ ] **Step 4: Run the full repository test suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`

Expected: ALL tests PASS. Per `verify_implementer_claims`, do not accept "pre-existing failure" attributions without verification — if anything fails, report the test name and last-known-passing state at HEAD (`01178eb`) before this plan started.

- [ ] **Step 5: Commit the fix**

```bash
git add pkg/script/handlers_number.go
git commit --no-gpg-sign -m "fix(nai-124): Stage 2 — swap SCALE opcode operands per TS NumberOps.ts:124-127

handleScale was computing (a*b)/c where TS computes (a*c)/b. For
runescript scale(value, max, newMax) (left-to-right push → bottom-up
pop), goscape evaluated value*max/newMax instead of value*newMax/max.

Smoking-gun: combat.rs2:5 calls scale(100, 100, \$stat_level) inside
combat_effective_stat. For a level-1 fresh char (\$stat_level=1):
  TS:      (100*1)/100   = 1     → \$effective_strength = 9
  Goscape: (100*100)/1   = 10000 → \$effective_strength = 10008

The cascade pushed %com_maxhit to ~1063 (clamped to npc_param(max_dealt)=1000),
producing randominc(1000) hits clamped to rat HP=3 — the bronze-dagger-vs-rat
\"always 3 damage\" smoke residual at NAI-123 close (b7c16b0).

Also corrected the divide-by-zero predicate from c==0 to b==0
(b is the divisor under TS semantics).

Refs Stage 1 findings: docs/superpowers/findings/2026-05-08-nai-124-stage1-findings.md."
```

---

## Task 4: Smoke handoff to user

**Files:** none modified.

- [ ] **Step 1: Verify final state**

Run from repo root:

```bash
git status
git log --oneline -5
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
```

Expected: clean working tree (no untracked or modified files outside `.claude/` and `test_typed_nil.go`); 3 new commits ahead of `01178eb`; build succeeds; all tests pass.

- [ ] **Step 2: Report a paste-ready smoke prompt for the user**

Per `smoke_test_server_handoff`, the user must launch the server. Output this verbatim to the controller for hand-off:

```
NAI-124 Stage 2 implementation complete. Smoke handoff:

1. Start the server: CGO_ENABLED=0 go run -trimpath ./cmd/goscape --config.file config.yaml
2. Connect Java client (LostCityRS/Client-Java #225). Log in as fresh tutorial char.
3. Walk through tutorial onto Tutorial Island combat instructor area.
4. Equip bronze dagger; engage giant rat; record damage on each successful hit
   for ≥15 attacks.

Smoke binds (NAI-124 PRIMARY closes) when:
  - Damage distribution shifts from "3 every hit" to a mix of 0/1/(rare 2-3).
  - Specifically: ≥5/15 zero hits, AND non-zero hits cap at 0 or 1 (per spec §6).

If smoke binds: NAI-124 closes. Per cascade_theory_smoke_binding, close PRIMARY
even if adjacent residuals appear. Route any new residual (e.g. NPC death-anim,
loot drop, etc.) to NAI-125+ as a fresh sub-spec.

If smoke does NOT bind: re-open the brainstorm — Stage 1's S5 verdict was
wrong, and a different surface is binding. Spec hypothesised S1 (sign-ext);
S1 + S2 still on the table for NAI-125 even if smoke binds Stage 2 cleanly.
```

- [ ] **Step 3: Verify post-merge git status on main**

Per `feedback_subagent_wt_path`, run from main working tree (not from any worktree):

```bash
git status
```

Expected: only `.claude/` and `test_typed_nil.go` untracked (pre-existing, per session-start status). No subagent-written stray files in the main tree. If anything else appears, surface it before declaring the task done.

---

## Self-Review (controller, post-write)

**Spec coverage:** §scope of `2026-05-08-nai-124-stage1-findings.md` calls for two changes — `handlers_number.go` operand swap + predicate flip, and `handlers_number_test.go` test refresh including a divide-by-zero pinning case. Both are tasked (Tasks 3 and 1+2 respectively). The "optional doc-comment cross-reference to TS NumberOps.ts:124-127" is included in Task 3.

**Placeholder scan:** No TBD/TODO/"add appropriate"/etc. Every code block is the literal final form.

**Type consistency:** `handleScale` keeps its `(s *ScriptState) error` signature. `runSingleOp` helper is reused unchanged. Test struct shape matches existing `TestNumberHandlers` table. `TestScaleDivideByZeroAborts` mirrors `TestDivideByZeroAborts` field-by-field.

**Test independence:** Both new TestNumberHandlers cases use distinct names; the divide-by-zero abort test runs in its own top-level function, not coupled to any subtest state.

**Out-of-scope guardrails:** S1/S2 paramtype-default-branch sign-ext is explicitly excluded per §scope and `cascade_theory_smoke_binding`. Implementer must NOT touch `handlers_config.go:51`, `handlers_inv.go:256`, `npc_hunt.go:297`, or `paramtype.go:111` in this bundle.
