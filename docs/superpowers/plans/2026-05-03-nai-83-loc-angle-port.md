# NAI-83: Port LOC_ANGLE Opcode Handler — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Wire LOC_ANGLE (opcode 3001) end-to-end: extend the `ActiveLoc` interface with `Angle()`, add a `checkLocAngle` range validator, implement `handleLocAngle`, register it in the dispatch map, and update the two test mocks. Cascade-blocker: `[oploc1, newbie_door1]` no-handler error from NAI-82 close smoke.

**Architecture:** Stub-not-completed accessor port. The opcode constant and name table already exist; this plan adds the missing handler + dispatch entry + interface accessor + range validator following the NAI-81 LOC_COORD pattern. `pkg/entity.Loc.Angle()` already exists as the producer-side accessor (`(l.Info >> 19) & 0x3` at `pkg/entity/loc.go:34`); no producer-side change.

**Tech Stack:** Go 1.26+ (per `go_version.md`).

**Spec:** `docs/superpowers/specs/2026-05-03-nai-83-loc-angle-port-design.md` (committed `d8c92fb`).

**Cadence:** spec + plan + single combined review at end (per `compressed_cadence.md` 15–100 LOC band; ~22 production LOC). No per-task two-stage review. One implementer subagent owns T1+T2+T3; reviewer subagent runs once at end.

---

## File Manifest

| File | Action | Responsibility |
|---|---|---|
| `pkg/script/active.go:698-701` | Modify | Add `Angle() int` to `ActiveLoc` interface |
| `pkg/script/handlers_player.go:71` (after `checkNotNull`) | Modify | Add `checkLocAngle(v int) error` validator |
| `pkg/script/handlers_loc.go` (after `handleLocCoord`) | Modify | Add `handleLocAngle` |
| `pkg/script/handlers.go:122-126` | Modify | Add `OpLocAngle: handleLocAngle` dispatch entry |
| `pkg/script/handlers_loc_test.go:11-17` | Modify | Extend `fakeActiveLoc` with `angle` field + `Angle()` method |
| `pkg/script/handlers_player_test.go:15-21` | Modify | Extend `mockActiveLoc` with `angle` field + `Angle()` method |
| `pkg/script/handlers_loc_test.go` (after `TestHandleLocCoordRequiresActiveLoc`) | Modify | Add two test functions for `handleLocAngle` |

---

## Task 1: Red — failing tests + compile-broken interface

**Goal:** Land the interface extension, both mock updates, and both test functions in one commit. Compile fails until the handler exists; tests fail because `handleLocAngle` is undefined.

**Files:**
- Modify: `pkg/script/active.go:698-701`
- Modify: `pkg/script/handlers_loc_test.go:11-17` (mock) + append (tests)
- Modify: `pkg/script/handlers_player_test.go:15-21` (mock)

### Step 1.1: Extend `ActiveLoc` interface

- [ ] Edit `pkg/script/active.go`. Replace the current interface block (lines 695-701):

```go
// ActiveLoc is the surface that LOC_* opcodes use to read the loc
// bound to the script's execution. Set by OPLOC trigger routing
// (S6j fireOpTriggerLoc) and LOC_FIND (future).
type ActiveLoc interface {
	LocType() int              // returns the LocType ID (from packed Loc.Info bitfield)
	Coords() (x, z, level int) // world position; consumed by LOC_COORD
}
```

with:

```go
// ActiveLoc is the surface that LOC_* opcodes use to read the loc
// bound to the script's execution. Set by OPLOC trigger routing
// (S6j fireOpTriggerLoc) and LOC_FIND (future).
type ActiveLoc interface {
	LocType() int              // returns the LocType ID (from packed Loc.Info bitfield)
	Coords() (x, z, level int) // world position; consumed by LOC_COORD
	Angle() int                // rotation (0=west, 1=north, 2=east, 3=south); consumed by LOC_ANGLE
}
```

### Step 1.2: Extend `fakeActiveLoc`

- [ ] Edit `pkg/script/handlers_loc_test.go`. Replace lines 10-17:

```go
// fakeActiveLoc is a minimal ActiveLoc implementation for handler tests.
type fakeActiveLoc struct {
	id          int
	x, z, level int
}

func (f fakeActiveLoc) LocType() int              { return f.id }
func (f fakeActiveLoc) Coords() (x, z, level int) { return f.x, f.z, f.level }
```

with:

```go
// fakeActiveLoc is a minimal ActiveLoc implementation for handler tests.
type fakeActiveLoc struct {
	id          int
	x, z, level int
	angle       int
}

func (f fakeActiveLoc) LocType() int              { return f.id }
func (f fakeActiveLoc) Coords() (x, z, level int) { return f.x, f.z, f.level }
func (f fakeActiveLoc) Angle() int                { return f.angle }
```

Existing call sites (`handlers_loc_test.go:46`, `:148`, `:165`) construct `fakeActiveLoc{id: ...}` or `fakeActiveLoc{id: 42, x: 3200, z: 3200, level: 0}`; Go zero-values for the new `angle` field preserve current behaviour. **Do not modify those call sites.**

### Step 1.3: Extend `mockActiveLoc`

- [ ] Edit `pkg/script/handlers_player_test.go`. Replace lines 15-21:

```go
type mockActiveLoc struct {
	locType     int
	x, z, level int
}

func (m *mockActiveLoc) LocType() int              { return m.locType }
func (m *mockActiveLoc) Coords() (x, z, level int) { return m.x, m.z, m.level }
```

with:

```go
type mockActiveLoc struct {
	locType     int
	x, z, level int
	angle       int
}

func (m *mockActiveLoc) LocType() int              { return m.locType }
func (m *mockActiveLoc) Coords() (x, z, level int) { return m.x, m.z, m.level }
func (m *mockActiveLoc) Angle() int                { return m.angle }
```

Existing call sites at `:950`, `:1008`, `:1089`, `:2367` construct `&mockActiveLoc{locType: 42}`; zero-values for the new field preserve behaviour. **Do not modify those call sites.**

### Step 1.4: Add the two test functions

- [ ] Append to `pkg/script/handlers_loc_test.go` (after `TestHandleLocCoordRequiresActiveLoc` at line 194):

```go
func TestHandleLocAngleHappyPath(t *testing.T) {
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		ActiveLoc:   fakeActiveLoc{id: 42, angle: 2},
	}

	if err := handleLocAngle(s); err != nil {
		t.Fatalf("handleLocAngle: %v", err)
	}

	if s.ISP != 1 {
		t.Fatalf("ISP: got %d, want 1", s.ISP)
	}
	if got := s.IntStack[0]; got != 2 {
		t.Errorf("top of int stack: got %d, want 2", got)
	}
}

func TestHandleLocAngleRequiresActiveLoc(t *testing.T) {
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}

	err := handleLocAngle(s)
	if err == nil {
		t.Fatal("handleLocAngle: expected error, got nil")
	}
	if got := err.Error(); got != "LOC_ANGLE: no active loc" {
		t.Errorf("error: got %q, want \"LOC_ANGLE: no active loc\"", got)
	}
}
```

### Step 1.5: Verify red — compile fails

- [ ] Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...`

Expected: build fails in `pkg/script/handlers_loc_test.go` with an "undefined: handleLocAngle" error. Compilation OK in non-test packages (the interface extension compiles cleanly because `pkg/entity.Loc.Angle()` already exists and zero other concrete `ActiveLoc` implementers exist outside test files).

If the build passes (i.e., something *did* satisfy `handleLocAngle`), STOP — investigate before proceeding.

### Step 1.6: Commit T1

- [ ] Run:

```bash
git add pkg/script/active.go pkg/script/handlers_loc_test.go pkg/script/handlers_player_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
test(script): NAI-83 T1 — LOC_ANGLE failing tests + Angle() on ActiveLoc

Adds Angle() to the ActiveLoc interface, extends fakeActiveLoc and
mockActiveLoc to satisfy it, and lands TestHandleLocAngleHappyPath +
TestHandleLocAngleRequiresActiveLoc. Compile fails on undefined
handleLocAngle until T2.
EOF
)"
```

---

## Task 2: Green — validator + handler + dispatch

**Goal:** Land `checkLocAngle`, `handleLocAngle`, and the dispatch wiring in one commit. Tests pass after this.

**Files:**
- Modify: `pkg/script/handlers_player.go` (after `checkNotNull` at line 76)
- Modify: `pkg/script/handlers_loc.go` (append after `handleLocCoord`)
- Modify: `pkg/script/handlers.go:122-126`

### Step 2.1: Add `checkLocAngle`

- [ ] Edit `pkg/script/handlers_player.go`. Insert immediately after the closing `}` of `checkNotNull` (current line 76, before the `checkStringNotNull` block at line 78):

```go

// checkLocAngle mirrors TS LocAngleValid (ScriptValidators.ts:106) — a
// ScriptInputRangeValidator over [LocAngle.WEST=0, LocAngle.SOUTH=3].
// Rejects values outside that range.
//
// Note: pkg/entity.Loc.Angle() is mask-bounded to [0,3] by construction
// ((l.Info >> 19) & 0x3 at loc.go:34), so this validator is unreachable
// when fed from the entity layer. Retained for TS-fidelity parity per
// true_to_ts_gate.md — future ActiveLoc producers (e.g. LOC_FIND results
// from external sources) may bypass the bit mask.
func checkLocAngle(v int) error {
	if v < 0 || v > 3 {
		return fmt.Errorf("LocAngle out of range: %d", v)
	}
	return nil
}
```

`fmt` is already imported in `handlers_player.go`. No import changes.

### Step 2.2: Add `handleLocAngle`

- [ ] Edit `pkg/script/handlers_loc.go`. Append after the closing `}` of `handleLocCoord` (current line 82):

```go

// handleLocAngle pushes the ActiveLoc's rotation onto the int stack,
// validated through the [0,3] LocAngle range. TS:
//
//	pushInt(check(activeLoc.angle, LocAngleValid));
//
// Requires an ActiveLoc; returns "LOC_ANGLE: no active loc" otherwise.
func handleLocAngle(s *ScriptState) error {
	if err := requireActiveLoc(s, "LOC_ANGLE"); err != nil {
		return err
	}
	angle := s.ActiveLoc.Angle()
	if err := checkLocAngle(angle); err != nil {
		return fmt.Errorf("LOC_ANGLE: %w", err)
	}
	s.PushInt(angle)
	return nil
}
```

`fmt` is already imported in `handlers_loc.go` (line 4). No import changes.

### Step 2.3: Wire into dispatch

- [ ] Edit `pkg/script/handlers.go`. Replace lines 122-126:

```go
	// LOC lookup — stub (always "not found"). Real impl ships with S6.
	OpLocCoord: handleLocCoord,
	OpLocFind:  handleLocFind,
	// LOC active-loc reads.
	OpLocOp: handleLocOp,
```

with:

```go
	// LOC lookup — stub (always "not found"). Real impl ships with S6.
	OpLocCoord: handleLocCoord,
	OpLocFind:  handleLocFind,
	// LOC active-loc reads.
	OpLocAngle: handleLocAngle,
	OpLocOp:    handleLocOp,
```

(`OpLocAngle` slots in lexically before `OpLocOp` under the existing "LOC active-loc reads" sub-comment. The pre-existing labeling for `OpLocCoord` / `OpLocFind` stays untouched per spec §4.4 — out of scope.)

### Step 2.4: Verify green — pkg/script tests pass

- [ ] Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/... -run TestHandleLocAngle -v`

Expected:
```
=== RUN   TestHandleLocAngleHappyPath
--- PASS: TestHandleLocAngleHappyPath (0.00s)
=== RUN   TestHandleLocAngleRequiresActiveLoc
--- PASS: TestHandleLocAngleRequiresActiveLoc (0.00s)
PASS
```

If either fails, STOP and diagnose against the spec §4 / §5 code blocks.

### Step 2.5: Commit T2

- [ ] Run:

```bash
git add pkg/script/handlers_player.go pkg/script/handlers_loc.go pkg/script/handlers.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): NAI-83 T2 — handleLocAngle + checkLocAngle validator + dispatch

Greens T1's failing tests. checkLocAngle mirrors TS LocAngleValid
(range [0,3]); handleLocAngle wraps requireActiveLoc + checkLocAngle +
PushInt. Dispatch entry slots OpLocAngle under the existing
"LOC active-loc reads" sub-comment in lexical order.
EOF
)"
```

---

## Task 3: Verify — full-repo regression scan

**Goal:** Confirm the additive interface change broke nothing elsewhere and the new code passes `go vet`.

**Files:** none modified.

### Step 3.1: Full test suite

- [ ] Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`

Expected: all packages green. The `ActiveLoc` interface extension is purely additive — `pkg/entity.Loc` already implements `Angle()` (loc.go:34), and the only other concrete implementers are `fakeActiveLoc` / `mockActiveLoc` (already extended in T1).

If any package fails, the most likely cause is an additional `ActiveLoc` implementer outside `pkg/script` test files. Grep first:

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./... 2>&1 | grep -i "does not implement ActiveLoc\|missing Angle"
```

Add `Angle() int` to any concrete type the grep surfaces (returning the underlying angle field — for an `*entity.Loc` wrapper, delegate to `Loc.Angle()`).

### Step 3.2: Vet clean

- [ ] Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...`

Expected: no output (clean).

### Step 3.3: No commit needed for T3

T3 is verification-only. No file changes. Proceed to combined review.

---

## Combined Review (single, end-of-impl)

**Per `compressed_cadence.md` 15–100 LOC band:** dispatch ONE reviewer subagent against the cumulative diff `d8c92fb..HEAD` (spec→impl range), not per-task reviewers.

**Reviewer prompt template (subagent):**

> Review the implementation of NAI-83 (LOC_ANGLE opcode port). Spec at `docs/superpowers/specs/2026-05-03-nai-83-loc-angle-port-design.md` (commit `d8c92fb`). Plan at `docs/superpowers/plans/2026-05-03-nai-83-loc-angle-port.md`. Cumulative diff: `git diff d8c92fb..HEAD`.
>
> TS reference: `LostCityRS/Engine-TS/src/engine/script/handlers/LocOps.ts:45-47` and `ScriptValidators.ts:106` (`LocAngleValid`).
>
> Verify:
> 1. **TS-fidelity**: `handleLocAngle` matches `pushInt(check(activeLoc.angle, LocAngleValid))` semantics — push happens iff validator passes; error path returns instead of pushing.
> 2. **Validator shape**: `checkLocAngle` rejects v<0 or v>3, accepts 0/1/2/3.
> 3. **Interface additive**: `ActiveLoc.Angle()` is the only signature change; existing implementers (`*entity.Loc`, `fakeActiveLoc`, `mockActiveLoc`) all implement it; no other production types broken.
> 4. **Dispatch wiring**: `OpLocAngle: handleLocAngle` registered exactly once under "LOC active-loc reads" sub-comment.
> 5. **Test coverage**: happy-path pushes correct value through stack; missing-active-loc returns the spec'd error literal `"LOC_ANGLE: no active loc"`. No synthetic out-of-range test (correct per spec §5 — that branch is unreachable from `entity.Loc.Angle()` mask).
> 6. **No scope creep**: spec §4.4 minimal-touch dispatch wiring honored — no regrouping of `OpLocCoord` / `OpLocFind` labeling.
> 7. **`go test ./...`** and **`go vet ./...`** both clean against working tree.
> 8. **`git show <commit-SHA> --stat`** for T1 and T2 — verify implementer commits' file lists match the spec/plan file manifest (per `implementer_commit_content_verify.md`).
>
> Report any deviations or missed items. Sonnet model only (per `superpowers_code_reviewer_model.md`).

---

## Close Commit

After reviewer subagent reports clean (or after addressing any review feedback), run a close commit:

- [ ] Run:

```bash
git commit --no-gpg-sign --allow-empty -m "$(cat <<'EOF'
close: NAI-83 — LOC_ANGLE ported

Cascade: door-click smoke at NAI-82 close surfaced
[oploc1, newbie_door1] no-handler-for-LOC_ANGLE error. Now wired:
ActiveLoc.Angle() accessor, checkLocAngle range validator, dispatch
entry. Cascade attribution closes at the next user-driven Tutorial
Island door-click smoke.

Closes memory: nai83_seed_loc_angle.md
EOF
)"
```

(Empty close commit per `close_commit_memory_trailer.md` — gives the memory ledger a discoverable provenance entry from `git log --grep`.)

---

## Self-Review

**Spec coverage:**
- §4.1 interface extension → T1 Step 1.1 ✓
- §4.2 validator → T2 Step 2.1 ✓
- §4.3 handler → T2 Step 2.2 ✓
- §4.4 dispatch wiring → T2 Step 2.3 ✓
- §4.5 mock updates → T1 Steps 1.2, 1.3 ✓
- §5.1 happy-path test → T1 Step 1.4 ✓
- §5.2 no-active-loc test → T1 Step 1.4 ✓
- §6 TS-fidelity ledger → reviewer prompt items 1–5 ✓
- §8 smoke / cascade routing → close commit body ✓
- §10 plan handoff (T1 red / T2 green / T3 verify / single review / close commit) → all sections ✓

**Placeholder scan:** no TBDs, no "appropriate error handling", no "TODO". All code blocks complete.

**Type consistency:** `Angle() int` on `ActiveLoc` interface (Step 1.1) matches `(f fakeActiveLoc) Angle() int` (Step 1.2), `(m *mockActiveLoc) Angle() int` (Step 1.3, pointer receiver), and `(l *Loc) Angle() int` (pre-existing at `pkg/entity/loc.go:34`). `checkLocAngle(v int) error` signature (Step 2.1) matches the call site in `handleLocAngle` (Step 2.2). `handleLocAngle(s *ScriptState) error` signature (Step 2.2) matches the test call sites (Step 1.4). Dispatch entry `OpLocAngle: handleLocAngle` (Step 2.3) matches the function signature.

**Spec-to-plan exit clean.**
