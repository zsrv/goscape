# S6q — LookupKey Bit-Packing Helpers Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development or superpowers:executing-plans. Steps use checkbox (`- [ ]`) syntax.

**Goal:** Replace 23 inlined `uint32(trigger) | (0x2 << 8) | (uint32(typeID) << 10)` bit-packing sites with 3 named helper functions in `pkg/script/lookup_key.go`.

**Architecture:** Create helpers in a new file, migrate the 4 production sites in `provider.go`, then mechanically migrate the 19 test sites.

**Tech Stack:** Go 1.26, standard `testing` package.

**Spec:** `docs/superpowers/specs/2026-04-21-runescript-s6q-lookupkey-helpers-design.md` (commit `f549cc3`).

---

## File Structure

### Production

- **Create:** `pkg/script/lookup_key.go` — 3 package-level helpers
- **Modify:** `pkg/script/provider.go` — 4 inlined sites (lines 115, 119, 147, 150)

### Tests

- **Create:** `pkg/script/lookup_key_test.go` — 6 tests (formula + boundaries)
- **Modify (Task 2):**
  - `pkg/script/provider_test.go` — 6 sites (91, 92, 182, 194, 232, plus one more)
  - `modules/world/interaction_trigger_test.go` — 5 sites (17, 129, 185, 260, 466)
  - `modules/world/player_script_test.go` — 6 sites (197, 228, 274, 302, 356, 357)
  - `modules/world/script_test.go` — 2 sites (1029, 1084)

---

## Task 1: Helpers + production migration

**Files:**
- Create: `pkg/script/lookup_key.go`
- Create: `pkg/script/lookup_key_test.go`
- Modify: `pkg/script/provider.go:115,119,147,150`

- [ ] **Step 1: Write failing helper tests.**

Create `pkg/script/lookup_key_test.go`:

```go
package script

import "testing"

func TestLookupKeyForTypeExactFormula(t *testing.T) {
	got := LookupKeyForType(TriggerOpNpc1, 42)
	want := uint32(TriggerOpNpc1) | 0x200 | (uint32(42) << 10)
	if got != want {
		t.Errorf("LookupKeyForType: got 0x%x, want 0x%x", got, want)
	}
}

func TestLookupKeyForCategoryExactFormula(t *testing.T) {
	got := LookupKeyForCategory(TriggerOpNpc1, 7)
	want := uint32(TriggerOpNpc1) | 0x100 | (uint32(7) << 10)
	if got != want {
		t.Errorf("LookupKeyForCategory: got 0x%x, want 0x%x", got, want)
	}
}

func TestLookupKeyForGlobalIsJustTrigger(t *testing.T) {
	got := LookupKeyForGlobal(TriggerOpNpc1)
	want := uint32(TriggerOpNpc1)
	if got != want {
		t.Errorf("LookupKeyForGlobal: got 0x%x, want 0x%x", got, want)
	}
}

func TestLookupKeyBoundaryTypeIDZero(t *testing.T) {
	// typeID=0 should leave bits 10+ clear and only set the selector.
	got := LookupKeyForType(TriggerOpNpc1, 0)
	want := uint32(TriggerOpNpc1) | 0x200
	if got != want {
		t.Errorf("typeID=0: got 0x%x, want 0x%x", got, want)
	}
}

func TestLookupKeyBoundaryCategoryIDZero(t *testing.T) {
	got := LookupKeyForCategory(TriggerOpNpc1, 0)
	want := uint32(TriggerOpNpc1) | 0x100
	if got != want {
		t.Errorf("categoryID=0: got 0x%x, want 0x%x", got, want)
	}
}

func TestLookupKeyDistinctnessAcrossSelectors(t *testing.T) {
	// Same (trigger, id) must produce 3 distinct keys across the
	// three selector bit-patterns.
	typeK := LookupKeyForType(TriggerOpNpc1, 7)
	catK := LookupKeyForCategory(TriggerOpNpc1, 7)
	globK := LookupKeyForGlobal(TriggerOpNpc1)
	if typeK == catK || typeK == globK || catK == globK {
		t.Errorf("selectors should produce distinct keys: type=0x%x cat=0x%x glob=0x%x", typeK, catK, globK)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail.**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestLookupKey -v`
Expected: FAIL with "LookupKeyForType undefined" compile error.

- [ ] **Step 3: Create the helpers.**

Create `pkg/script/lookup_key.go`:

```go
package script

// LookupKeyForType returns the specific-script lookup key for a
// (trigger, typeID) pair. Layout: bits 0-7 = trigger, bits 8-9 =
// selector (0b10 = type-specific), bits 10+ = typeID.
func LookupKeyForType(trigger ServerTriggerType, typeID int) uint32 {
	return uint32(trigger) | (0x2 << 8) | (uint32(typeID) << 10)
}

// LookupKeyForCategory returns the category-fallback lookup key.
// Bits 8-9 = 0b01 (category selector).
func LookupKeyForCategory(trigger ServerTriggerType, categoryID int) uint32 {
	return uint32(trigger) | (0x1 << 8) | (uint32(categoryID) << 10)
}

// LookupKeyForGlobal returns the global-fallback lookup key. Bits 8-9
// = 0b00 (no type/category).
func LookupKeyForGlobal(trigger ServerTriggerType) uint32 {
	return uint32(trigger)
}
```

- [ ] **Step 4: Run tests to verify pass.**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestLookupKey -v`
Expected: PASS × 6

- [ ] **Step 5: Migrate `pkg/script/provider.go` production call sites.**

Find and replace the 4 inlined sites. They are at lines 115, 119, 147, 150. Their current form:

```go
// Line 115 — GetByTrigger specific branch
specific := uint32(trigger) | (0x2 << 8) | (uint32(typeID) << 10)

// Line 119 — GetByTrigger category branch
category := uint32(trigger) | (0x1 << 8) | (uint32(categoryID) << 10)

// Line 147 — handleGosub-adjacent specific
return p.byKey[uint32(trigger)|(0x2<<8)|(uint32(typeID)<<10)]

// Line 150 — handleGosub-adjacent category
return p.byKey[uint32(trigger)|(0x1<<8)|(uint32(categoryID)<<10)]
```

Replace with:

```go
// Line 115
specific := LookupKeyForType(trigger, typeID)

// Line 119
category := LookupKeyForCategory(trigger, categoryID)

// Line 147
return p.byKey[LookupKeyForType(trigger, typeID)]

// Line 150
return p.byKey[LookupKeyForCategory(trigger, categoryID)]
```

- [ ] **Step 6: Run full repo tests.**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`
Expected: PASS (behavior unchanged; helpers produce bit-for-bit identical keys).

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...`
Expected: no diagnostics.

- [ ] **Step 7: Commit.**

```bash
git add pkg/script/lookup_key.go pkg/script/lookup_key_test.go pkg/script/provider.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): add LookupKeyFor{Type,Category,Global} helpers + migrate provider (S6q-1)

Extract the inlined bit-packing pattern
  uint32(trigger) | (0x2 << 8) | (uint32(typeID) << 10)
into three package-level helpers in pkg/script/lookup_key.go:
  LookupKeyForType(trigger, typeID) uint32
  LookupKeyForCategory(trigger, categoryID) uint32
  LookupKeyForGlobal(trigger) uint32

Migrate the 4 production call sites in provider.go (GetByTrigger
specific/category branches and the handleGosub-adjacent branches).
Behavior-preserving: every migrated site produces the bit-for-bit
identical u32 key.

6 new tests (formula × 3 + typeID=0 / categoryID=0 boundaries +
selector distinctness).

Task 2 migrates the 19 test-site inlined patterns.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: Test-site migration

**Files (all modifications, mechanical):**
- `pkg/script/provider_test.go` — 6 sites
- `modules/world/interaction_trigger_test.go` — 5 sites
- `modules/world/player_script_test.go` — 6 sites
- `modules/world/script_test.go` — 2 sites

### TDD context

This task is mechanical: find-and-replace with build verification. Tests never change their assertions or semantics.

- [ ] **Step 1: Migrate `pkg/script/provider_test.go`.**

Use Grep to find sites: search for `<< 10` in that file. Replace each inlined form with the appropriate helper. Examples:

Before:
```go
specificKey := uint32(trigger) | (0x2 << 8) | (uint32(typeID) << 10)
categoryKey := uint32(trigger) | (0x1 << 8) | (uint32(catID) << 10)
```

After:
```go
specificKey := LookupKeyForType(trigger, typeID)
categoryKey := LookupKeyForCategory(trigger, catID)
```

The file imports are already correct (same-package `script`). No import changes needed.

- [ ] **Step 2: Migrate `modules/world/interaction_trigger_test.go`.**

This file imports `script` as `"github.com/zsrv/goscape/pkg/script"`. Call sites will look like:

Before:
```go
key := uint32(trigger) | (0x2 << 8) | (uint32(typeID) << 10)
```

After:
```go
key := script.LookupKeyForType(trigger, typeID)
```

Note: if `trigger` is already typed as `script.ServerTriggerType` the call compiles. If it's a literal `uint32(...)` cast, trace back to verify the underlying variable type.

For sites like:
```go
LookupKey: uint32(script.TriggerOpNpc1) | (0x2 << 8) | (uint32(7) << 10),
```

Replace with:
```go
LookupKey: script.LookupKeyForType(script.TriggerOpNpc1, 7),
```

- [ ] **Step 3: Migrate `modules/world/player_script_test.go`.**

Same pattern — 6 sites. The variables are usually `key`, `changeKey`, `advKey`. Preserve the variable names.

- [ ] **Step 4: Migrate `modules/world/script_test.go`.**

2 sites, same pattern.

- [ ] **Step 5: Verify no inlined patterns remain in the migrated files.**

Run (from the repo root): grep for `<< 10` in the 4 touched test files. Expected: 0 matches in test code. Use the Grep tool with pattern `<< 10` and glob `**/{provider_test,interaction_trigger_test,player_script_test,script_test}.go`.

If any remain, they're sites that were missed — migrate them.

- [ ] **Step 6: Run full repo tests.**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`
Expected: PASS (same test count as after Task 1).

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...`
Expected: clean.

- [ ] **Step 7: Commit.**

```bash
git add pkg/script/provider_test.go \
        modules/world/interaction_trigger_test.go \
        modules/world/player_script_test.go \
        modules/world/script_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
refactor(script,world): migrate inlined lookup-key packing to helpers (S6q-2)

Replace 19 inlined uint32(trigger) | (0x2 << 8) | (uint32(typeID) << 10)
sites across 4 test files with calls to the S6q-1 helpers:
  pkg/script/provider_test.go — 6 sites
  modules/world/interaction_trigger_test.go — 5 sites
  modules/world/player_script_test.go — 6 sites
  modules/world/script_test.go — 2 sites

Behavior-preserving: every migrated site computes the bit-for-bit
identical key. Test counts and assertions unchanged.

Closes S6q. No production changes in this commit — the helpers landed
in S6q-1.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Deviations

**Zero** new deviations. Zero closures. This is a pure DRY refactor.
