# handlers_inv.go read-side `checkInvType` wiring — implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Wire `checkInvType` at 12 sites in `pkg/script/handlers_inv.go` — 9 Shape A read-side handlers (currently `resolveInv`-only) + 3 Shape B inline-registry handlers (currently bespoke `"invalid inv id"` wording) — to match the intra-file sibling precedent at `performInvAdd:344-371` and bring registry-miss errors under canonical `checkInvType` wording.

**Architecture:** Layer `checkInvType` BEFORE existing checks at every Shape A handler, keeping the `resolveInv == nil` defensive fallthrough with the same `"%s: no inv for type %d"` wording (mirroring `performInvAdd:367-372` verbatim). For Shape B, swap inline `s.Configs.InvType(id) == nil` block for a `checkInvType` call, preserving the `invType := s.Configs.InvType(id)` local var for downstream field access (matches the `handleInvDel`-class pattern from [[registry-presence-validators-wiring-close]] Part B).

**Tech Stack:** Go 1.26.x; `pkg/script/handlers_inv.go` (production); `pkg/script/handlers_inv_test.go` (no expected changes); `pkg/script/handlers_player.go:158-170` (existing validator); `pkg/script/handlers_player_test.go:2364` (existing validator-layer test).

**Spec:** `docs/superpowers/specs/2026-05-21-handlers-inv-readside-checkinvtype-wiring-design.md` (HEAD `c37ed1b8`).

---

## Task 1: Wire all 12 sites + verify gates

**Files:**
- Modify: `pkg/script/handlers_inv.go` at 12 distinct call sites (see step-by-step)
- (Expected) no changes: `pkg/script/handlers_inv_test.go`, `pkg/script/handlers_player.go`, `pkg/script/handlers_player_test.go`

### Step 1: Pre-impl audit-grep baseline (record exact HEAD counts)

- [ ] **Step 1.1: Record baseline counts**

Run from repo root:

```bash
grep -c "checkInvType(s, " pkg/script/handlers_inv.go
grep -c "no inv for type" pkg/script/handlers_inv.go
grep -c "invalid inv id" pkg/script/handlers_inv.go
```

Expected at HEAD `c37ed1b8`:
- `checkInvType(s, ` → **23**
- `no inv for type` → **15**
- `invalid inv id` → **3**

If any baseline diverges from these values, STOP and report — HEAD may have drifted. Do NOT proceed.

### Step 2: Wire Shape A (9 read-side handlers) — `checkInvType` BEFORE `resolveInv` + defensive comment

The canonical wiring pattern, applied to each of the 9 Shape A sites below, is:

```go
// Before resolveInv invocation, insert:
if err := checkInvType(s, typeID, "OPCODE_NAME"); err != nil {
    return err
}
// Immediately above the existing `if inv == nil { return ... }` block, insert:
// Defensive: unreachable post-checkInvType for valid configs;
// retained for the InvLookup-unset case (s.Inv == nil → resolveInv returns nil).
```

The defensive `"no inv for type"` error line is PRESERVED VERBATIM. Only the comment above it is new, and a new `checkInvType` call is added higher up.

For sites that pop `typeID` under a different local-var name (e.g., `inv`, `invID`), the variable name passed to `checkInvType` matches the existing local var (no rename).

For `handleInvTotal` ONLY, the `obj == -1` short-circuit MUST stay BEFORE `checkInvType` — when obj is -1 the result is 0 regardless of InvType validity (matches TS short-circuit). All other 8 Shape A handlers have no pre-validator structure; `checkInvType` goes immediately after the `PopInt`s.

- [ ] **Step 2.1: Wire `handleInvTotal` (INV_TOTAL) at `:26-40`**

Existing:

```go
func handleInvTotal(s *ScriptState) error {
	obj := s.PopInt()
	typeID := s.PopInt()
	// TS INV_TOTAL short-circuits with obj == -1 → push 0.
	if obj == -1 {
		s.PushInt(0)
		return nil
	}
	inv := resolveInv(s, typeID)
	if inv == nil {
		return fmt.Errorf("INV_TOTAL: no inv for type %d", typeID)
	}
	s.PushInt(inv.GetItemCount(obj))
	return nil
}
```

Replace with:

```go
func handleInvTotal(s *ScriptState) error {
	obj := s.PopInt()
	typeID := s.PopInt()
	// TS INV_TOTAL short-circuits with obj == -1 → push 0.
	if obj == -1 {
		s.PushInt(0)
		return nil
	}
	if err := checkInvType(s, typeID, "INV_TOTAL"); err != nil {
		return err
	}
	inv := resolveInv(s, typeID)
	if inv == nil {
		// Defensive: unreachable post-checkInvType for valid configs;
		// retained for the InvLookup-unset case (s.Inv == nil → resolveInv returns nil).
		return fmt.Errorf("INV_TOTAL: no inv for type %d", typeID)
	}
	s.PushInt(inv.GetItemCount(obj))
	return nil
}
```

- [ ] **Step 2.2: Wire `handleInvGetObj` (INV_GETOBJ) at `:44-58`**

Existing:

```go
func handleInvGetObj(s *ScriptState) error {
	slot := s.PopInt()
	typeID := s.PopInt()
	inv := resolveInv(s, typeID)
	if inv == nil {
		return fmt.Errorf("INV_GETOBJ: no inv for type %d", typeID)
	}
	it := inv.Get(slot)
	if it == nil {
		s.PushInt(-1)
		return nil
	}
	s.PushInt(it.Id)
	return nil
}
```

Replace with:

```go
func handleInvGetObj(s *ScriptState) error {
	slot := s.PopInt()
	typeID := s.PopInt()
	if err := checkInvType(s, typeID, "INV_GETOBJ"); err != nil {
		return err
	}
	inv := resolveInv(s, typeID)
	if inv == nil {
		// Defensive: unreachable post-checkInvType for valid configs;
		// retained for the InvLookup-unset case (s.Inv == nil → resolveInv returns nil).
		return fmt.Errorf("INV_GETOBJ: no inv for type %d", typeID)
	}
	it := inv.Get(slot)
	if it == nil {
		s.PushInt(-1)
		return nil
	}
	s.PushInt(it.Id)
	return nil
}
```

- [ ] **Step 2.3: Wire `handleInvGetNum` (INV_GETNUM) at `:62-76`**

Existing:

```go
func handleInvGetNum(s *ScriptState) error {
	slot := s.PopInt()
	typeID := s.PopInt()
	inv := resolveInv(s, typeID)
	if inv == nil {
		return fmt.Errorf("INV_GETNUM: no inv for type %d", typeID)
	}
	it := inv.Get(slot)
	if it == nil {
		s.PushInt(0)
		return nil
	}
	s.PushInt(it.Count)
	return nil
}
```

Replace with:

```go
func handleInvGetNum(s *ScriptState) error {
	slot := s.PopInt()
	typeID := s.PopInt()
	if err := checkInvType(s, typeID, "INV_GETNUM"); err != nil {
		return err
	}
	inv := resolveInv(s, typeID)
	if inv == nil {
		// Defensive: unreachable post-checkInvType for valid configs;
		// retained for the InvLookup-unset case (s.Inv == nil → resolveInv returns nil).
		return fmt.Errorf("INV_GETNUM: no inv for type %d", typeID)
	}
	it := inv.Get(slot)
	if it == nil {
		s.PushInt(0)
		return nil
	}
	s.PushInt(it.Count)
	return nil
}
```

- [ ] **Step 2.4: Wire `handleInvSize` (INV_SIZE) at `:79-87`**

Existing:

```go
func handleInvSize(s *ScriptState) error {
	typeID := s.PopInt()
	inv := resolveInv(s, typeID)
	if inv == nil {
		return fmt.Errorf("INV_SIZE: no inv for type %d", typeID)
	}
	s.PushInt(inv.Capacity)
	return nil
}
```

Replace with:

```go
func handleInvSize(s *ScriptState) error {
	typeID := s.PopInt()
	if err := checkInvType(s, typeID, "INV_SIZE"); err != nil {
		return err
	}
	inv := resolveInv(s, typeID)
	if inv == nil {
		// Defensive: unreachable post-checkInvType for valid configs;
		// retained for the InvLookup-unset case (s.Inv == nil → resolveInv returns nil).
		return fmt.Errorf("INV_SIZE: no inv for type %d", typeID)
	}
	s.PushInt(inv.Capacity)
	return nil
}
```

- [ ] **Step 2.5: Wire `handleInvFreeSpace` (INV_FREESPACE) at `:91-99`**

Existing:

```go
func handleInvFreeSpace(s *ScriptState) error {
	typeID := s.PopInt()
	inv := resolveInv(s, typeID)
	if inv == nil {
		return fmt.Errorf("INV_FREESPACE: no inv for type %d", typeID)
	}
	s.PushInt(inv.FreeSlotCount())
	return nil
}
```

Replace with:

```go
func handleInvFreeSpace(s *ScriptState) error {
	typeID := s.PopInt()
	if err := checkInvType(s, typeID, "INV_FREESPACE"); err != nil {
		return err
	}
	inv := resolveInv(s, typeID)
	if inv == nil {
		// Defensive: unreachable post-checkInvType for valid configs;
		// retained for the InvLookup-unset case (s.Inv == nil → resolveInv returns nil).
		return fmt.Errorf("INV_FREESPACE: no inv for type %d", typeID)
	}
	s.PushInt(inv.FreeSlotCount())
	return nil
}
```

- [ ] **Step 2.6: Wire `handleInvItemSpace` (INV_ITEMSPACE) at `:175-197`**

Existing:

```go
func handleInvItemSpace(s *ScriptState) error {
	size := s.PopInt()
	count := s.PopInt()
	obj := s.PopInt()
	typeID := s.PopInt()
	if count == 0 {
		s.PushInt(0)
		return nil
	}
	inv := resolveInv(s, typeID)
	if inv == nil {
		return fmt.Errorf("INV_ITEMSPACE: no inv for type %d", typeID)
	}
	if size < 0 || size > inv.Capacity {
		return fmt.Errorf("INV_ITEMSPACE: size %d out of range for inv %d", size, typeID)
	}
	if invItemSpaceRemaining(s, inv, obj, count, size) == 0 {
		s.PushInt(1)
	} else {
		s.PushInt(0)
	}
	return nil
}
```

Replace with:

```go
func handleInvItemSpace(s *ScriptState) error {
	size := s.PopInt()
	count := s.PopInt()
	obj := s.PopInt()
	typeID := s.PopInt()
	if count == 0 {
		s.PushInt(0)
		return nil
	}
	if err := checkInvType(s, typeID, "INV_ITEMSPACE"); err != nil {
		return err
	}
	inv := resolveInv(s, typeID)
	if inv == nil {
		// Defensive: unreachable post-checkInvType for valid configs;
		// retained for the InvLookup-unset case (s.Inv == nil → resolveInv returns nil).
		return fmt.Errorf("INV_ITEMSPACE: no inv for type %d", typeID)
	}
	if size < 0 || size > inv.Capacity {
		return fmt.Errorf("INV_ITEMSPACE: size %d out of range for inv %d", size, typeID)
	}
	if invItemSpaceRemaining(s, inv, obj, count, size) == 0 {
		s.PushInt(1)
	} else {
		s.PushInt(0)
	}
	return nil
}
```

Note: `count == 0` short-circuit stays BEFORE `checkInvType` — matches TS `InvOps.ts:289-292` short-circuit shape exactly (TS returns 0 before invoking check).

- [ ] **Step 2.7: Wire `handleInvItemSpace2` (INV_ITEMSPACE2) at `:202-216`**

Existing:

```go
func handleInvItemSpace2(s *ScriptState) error {
	size := s.PopInt()
	count := s.PopInt()
	obj := s.PopInt()
	typeID := s.PopInt()
	if count == 0 {
		s.PushInt(0)
		return nil
	}
	inv := resolveInv(s, typeID)
	if inv == nil {
		return fmt.Errorf("INV_ITEMSPACE2: no inv for type %d", typeID)
	}
	s.PushInt(invItemSpaceRemaining(s, inv, obj, count, size))
	return nil
}
```

Replace with:

```go
func handleInvItemSpace2(s *ScriptState) error {
	size := s.PopInt()
	count := s.PopInt()
	obj := s.PopInt()
	typeID := s.PopInt()
	if count == 0 {
		s.PushInt(0)
		return nil
	}
	if err := checkInvType(s, typeID, "INV_ITEMSPACE2"); err != nil {
		return err
	}
	inv := resolveInv(s, typeID)
	if inv == nil {
		// Defensive: unreachable post-checkInvType for valid configs;
		// retained for the InvLookup-unset case (s.Inv == nil → resolveInv returns nil).
		return fmt.Errorf("INV_ITEMSPACE2: no inv for type %d", typeID)
	}
	s.PushInt(invItemSpaceRemaining(s, inv, obj, count, size))
	return nil
}
```

- [ ] **Step 2.8: Wire `handleInvTotalParam` (INV_TOTALPARAM) at `:224-257`**

Existing:

```go
func handleInvTotalParam(s *ScriptState) error {
	param := s.PopInt()
	typeID := s.PopInt()
	inv := resolveInv(s, typeID)
	if inv == nil {
		return fmt.Errorf("INV_TOTALPARAM: no inv for type %d", typeID)
	}
	if err := checkParamType(s, param, "INV_TOTALPARAM"); err != nil {
		return err
	}
	// ... rest of function unchanged ...
```

Replace ONLY the lines from `inv := resolveInv...` through the `if inv == nil { ... }` block with:

```go
	if err := checkInvType(s, typeID, "INV_TOTALPARAM"); err != nil {
		return err
	}
	inv := resolveInv(s, typeID)
	if inv == nil {
		// Defensive: unreachable post-checkInvType for valid configs;
		// retained for the InvLookup-unset case (s.Inv == nil → resolveInv returns nil).
		return fmt.Errorf("INV_TOTALPARAM: no inv for type %d", typeID)
	}
```

The existing `checkParamType` call and the rest of the function stay unchanged. Resulting validator order: InvType → resolveInv → ParamType. This matches TS `InvOps.ts:786-…` shape where `check(inv, InvTypeValid)` runs first.

- [ ] **Step 2.9: Wire `handleInvTotalCat` (INV_TOTALCAT) at `:261-286`**

Existing:

```go
func handleInvTotalCat(s *ScriptState) error {
	category := s.PopInt()
	typeID := s.PopInt()
	inv := resolveInv(s, typeID)
	if inv == nil {
		return fmt.Errorf("INV_TOTALCAT: no inv for type %d", typeID)
	}
	if s.Configs == nil {
		return fmt.Errorf("INV_TOTALCAT: Configs not set on ScriptState")
	}
	// ... rest unchanged ...
```

Replace ONLY the lines from `inv := resolveInv...` through the `if inv == nil { ... }` block with:

```go
	if err := checkInvType(s, typeID, "INV_TOTALCAT"); err != nil {
		return err
	}
	inv := resolveInv(s, typeID)
	if inv == nil {
		// Defensive: unreachable post-checkInvType for valid configs;
		// retained for the InvLookup-unset case (s.Inv == nil → resolveInv returns nil).
		return fmt.Errorf("INV_TOTALCAT: no inv for type %d", typeID)
	}
```

The downstream `if s.Configs == nil` defensive check is now unreachable (checkInvType already dereferenced `s.Configs`), but DO NOT remove it — `checkInvType` only proves `s.Configs != nil` when `checkInvType` returned nil; defensive symmetry across handlers is preserved. (Per [[registry-presence-validators-wiring-close]] §5.4 "preserve local var" rule generalized: defensive checks adjacent to wired sites stay even when post-validator they're unreachable, matching `performInvAdd:367-372` precedent which also retains the post-validator defensive.)

### Step 3: Wire Shape B (3 inline-registry handlers) — swap inline for `checkInvType`, preserve local var

The canonical Shape B wiring pattern: replace the inline block

```go
invType := s.Configs.InvType(invID)
if invType == nil {
    return fmt.Errorf("OPCODE: invalid inv id (%d)", invID)
}
```

with

```go
if err := checkInvType(s, invID, "OPCODE"); err != nil {
    return err
}
invType := s.Configs.InvType(invID)
```

The local var `invType` is preserved verbatim because each handler accesses `invType.Protect` and `invType.Scope` downstream. The replacement is a 4-line-block → 4-line-block swap (1 inline line drops, 1 line added on top, var assignment kept).

- [ ] **Step 3.1: Wire `handleInvDropSlot` (INV_DROPSLOT) at `:794-798`**

Existing (lines 794-798):

```go
	// InvTypeValid: resolve InvType config.
	invType := s.Configs.InvType(invID)
	if invType == nil {
		return fmt.Errorf("INV_DROPSLOT: invalid inv id (%d)", invID)
	}
```

Replace with:

```go
	// InvTypeValid: registry-presence check via canonical validator.
	if err := checkInvType(s, invID, "INV_DROPSLOT"); err != nil {
		return err
	}
	invType := s.Configs.InvType(invID)
```

The downstream `if s.Configs == nil` guard at `:790-792` (`"INV_DROPSLOT: no configs"`) stays unchanged — runs BEFORE `checkInvType` so `s.Configs` may be nil at `:790`. Defensive symmetry preserved.

- [ ] **Step 3.2: Wire `handleBothDropSlot` (BOTH_DROPSLOT) at `:1657-1661`**

Existing (lines 1657-1661):

```go
	// InvTypeValid.
	invType := s.Configs.InvType(invID)
	if invType == nil {
		return fmt.Errorf("BOTH_DROPSLOT: invalid inv id (%d)", invID)
	}
```

Replace with:

```go
	// InvTypeValid: registry-presence check via canonical validator.
	if err := checkInvType(s, invID, "BOTH_DROPSLOT"); err != nil {
		return err
	}
	invType := s.Configs.InvType(invID)
```

The upstream `if s.Configs == nil` guard at `:1645-1647` stays — same defensive symmetry.

- [ ] **Step 3.3: Wire `handleInvDropAll` (INV_DROPALL) at `:1800-1804`**

Existing (lines 1800-1804):

```go
	// InvTypeValid.
	invType := s.Configs.InvType(invID)
	if invType == nil {
		return fmt.Errorf("INV_DROPALL: invalid inv id (%d)", invID)
	}
```

Replace with:

```go
	// InvTypeValid: registry-presence check via canonical validator.
	if err := checkInvType(s, invID, "INV_DROPALL"); err != nil {
		return err
	}
	invType := s.Configs.InvType(invID)
```

The upstream `if s.Configs == nil` guard at `:1789-1791` stays — same defensive symmetry.

### Step 4: Test review — audit for bespoke-wording assertions that may need flipping

- [ ] **Step 4.1: Audit-grep test file for bespoke wording**

Run:

```bash
grep -n "invalid inv id" pkg/script/handlers_inv_test.go
```

Expected: **0 hits.** If hits exist, each is a test asserting one of the now-canonicalized Shape B error strings. For each such hit:
1. Read the test to determine whether it passes an unregistered InvType id (→ canonical wording fires) or a registered InvType id with some other invalidity path (→ different wording fires).
2. If canonical: flip the asserted substring from `"invalid inv id"` to `"no InvType with value"`.
3. Cite the line and the decision in the commit message audit table.

- [ ] **Step 4.2: Verify existing `TestInvLookupNilReturnsError` semantics**

Open `pkg/script/handlers_inv_test.go` at line `535-541`:

```go
func TestInvLookupNilReturnsError(t *testing.T) {
	mc := newTestInvConfigs()
	// No lookup: every INV_* mutation / read that needs inv must error.
	runInvOpExpectErr(t, OpInvTotal, []int{testInvMain, testObjCoin}, nil, mc, "no inv for type")
	runInvOpExpectErr(t, OpInvAdd, []int{testInvMain, testObjCoin, 1}, nil, mc, "no active player")
	runInvOpExpectErr(t, OpInvClear, []int{testInvMain}, nil, mc, "no active player")
}
```

This test passes `testInvMain` (REGISTERED in `newTestInvConfigs()`) with `lookup=nil`. Under the wired code, `checkInvType` passes (InvType registered), `resolveInv` returns nil (`s.Inv == nil` from nil lookup), defensive fallthrough fires `"INV_TOTAL: no inv for type %d"`. **Assertion stays unchanged.** No edit needed.

Confirm: read the file, verify `newTestInvConfigs()` registers `testInvMain` in the `InvType` map. If yes, no edit. If no (drift), STOP and report.

```bash
grep -n "newTestInvConfigs\b" pkg/script/handlers_inv_test.go | head -5
```

Then read the function body to confirm `testInvMain` is registered.

### Step 5: Gates

- [ ] **Step 5.1: `gofmt -l` clean**

```bash
GOROOT=/home/owner/go/go1.26.3 GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache /home/owner/go/go1.26.3/bin/gofmt -l pkg/script/handlers_inv.go
```

Expected: **no output** (file already gofmt-clean).

- [ ] **Step 5.2: `go test -race ./...` 0 FAIL**

```bash
GOROOT=/home/owner/go/go1.26.3 GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache /home/owner/go/go1.26.3/bin/go test -race ./...
```

Expected: all packages PASS. Wall clock ~150-160s (modules/world is the long pole). If any FAIL, halt and diagnose.

- [ ] **Step 5.3: `TestPackAll_TwelveStageSmoke` PASS**

```bash
GOROOT=/home/owner/go/go1.26.3 GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache /home/owner/go/go1.26.3/bin/go test ./pkg/cache/pack -run TestPackAll_TwelveStageSmoke -count=1
```

Expected: PASS.

- [ ] **Step 5.4: Audit-grep post-impl counts**

```bash
grep -c "checkInvType(s, " pkg/script/handlers_inv.go
grep -c "no inv for type" pkg/script/handlers_inv.go
grep -c "invalid inv id" pkg/script/handlers_inv.go
```

Expected post-impl:
- `checkInvType(s, ` → **35** (+12 vs HEAD baseline 23)
- `no inv for type` → **15** (unchanged; defensive lines preserved verbatim, no new lines added)
- `invalid inv id` → **0** (−3 vs HEAD baseline 3; all Shape B canonicalized)

If any post-impl count differs from expected, STOP — wiring incomplete or unintended changes occurred. Report each divergence.

- [ ] **Step 5.5: Targeted handlers_inv_test PASS**

```bash
GOROOT=/home/owner/go/go1.26.3 GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache /home/owner/go/go1.26.3/bin/go test ./pkg/script/ -run "TestInv|TestBoth" -count=1 -v 2>&1 | tail -40
```

Expected: all `TestInv*` and `TestBoth*` tests PASS, including `TestInvLookupNilReturnsError`.

### Step 6: Commit

- [ ] **Step 6.1: Stage and commit**

```bash
git add pkg/script/handlers_inv.go
```

If Step 4.1 surfaced any test edits, also:

```bash
git add pkg/script/handlers_inv_test.go
```

Then commit:

```bash
git commit --no-gpg-sign -m "$(cat <<'EOF'
refactor(script): wire checkInvType at 12 read-side / inline-registry sites

Layer checkInvType before existing checks at 9 Shape A read-side
handlers in pkg/script/handlers_inv.go (INV_TOTAL/GETOBJ/GETNUM/
SIZE/FREESPACE/ITEMSPACE/ITEMSPACE2/TOTALPARAM/TOTALCAT), mirroring
the intra-file sibling precedent at performInvAdd:344-371. The
existing resolveInv-nil defensive fallthrough is retained verbatim
with a TS-precedent comment ("unreachable post-checkInvType for
valid configs; retained for the InvLookup-unset case"). Two-stage
error semantics preserved: registry-miss now fires canonical
"%s: no InvType with value (%d) found"; container-miss continues
to fire "%s: no inv for type %d".

Canonicalize 3 Shape B inline-registry handlers (INV_DROPSLOT,
BOTH_DROPSLOT, INV_DROPALL) — replace bespoke
"OPCODE: invalid inv id (%d)" inline checks with checkInvType
calls, preserving the invType local var for downstream Protect/
Scope field access. Per the registry-presence-validators-wiring
close precedent.

TS-faithful per InvOps.ts check(inv, InvTypeValid) at every cited
site (InvOps.ts:27/263/270/278/286/306/619/634/786 for Shape A;
write-side handlers DROPSLOT/BOTH_DROPSLOT/DROPALL for Shape B).

Zero new tests — validator-layer TestCheckInvType
(handlers_player_test.go:2364) already covers registry-miss
rejection. TestInvLookupNilReturnsError stays unchanged: it passes
a registered InvType id with nil lookup, exercising the defensive
container-miss path which is preserved.

Audit-grep delta vs HEAD c37ed1b8:
- checkInvType(s,  → 23 → 35  (+12)
- no inv for type → 15 → 15   (unchanged; defensive lines preserved)
- invalid inv id  → 3  → 0    (−3; all Shape B canonicalized)

Spec: docs/superpowers/specs/2026-05-21-handlers-inv-readside-checkinvtype-wiring-design.md

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

- [ ] **Step 6.2: Verify commit**

```bash
git log -1 --stat
git status
```

Expected:
- One new commit at HEAD touching `pkg/script/handlers_inv.go` (and optionally `handlers_inv_test.go` if Step 4.1 found assertions to flip).
- `git status` shows only standing untracked noise + `config.yaml` drift — no other modified files.

---

## Self-Review Notes

- **Spec coverage:** every section of the spec maps to steps above. §4.1 → Steps 2.1-2.9. §4.2 → Steps 3.1-3.3. §5 (out of scope) → no steps (intentional). §6.1 (existing assertion) → Step 4.2. §6.3 (impl-time audit-grep) → Steps 4.1 + 5.4. §8 (gates) → Steps 5.1-5.5. §9 (cadence) → single task per spec direction.
- **Placeholder scan:** no TBD/TODO; every step has concrete code OR concrete command + expected output.
- **Type consistency:** `checkInvType(s *ScriptState, id int, op string) error` signature used uniformly. `invType := s.Configs.InvType(...)` local var preserved at all Shape B sites.
- **Naming consistency:** opcode literals (`"INV_TOTAL"`, etc.) match the existing handler doc-comment opcode labels and the test op names (`OpInvTotal`, etc.).
- **Defensive comment text:** identical 2-line comment used at every Shape A site, copied verbatim from `performInvAdd:369-370`.
