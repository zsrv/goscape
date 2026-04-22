# S6q — `script.LookupKeyFor{Type,Category,Global}` Bit-Packing Helpers Design

> **Sub-spec context:** Seventeenth runescript sub-spec. Pure infrastructure DRY — extracts the inlined `uint32(trigger) | (0x2 << 8) | (uint32(typeID) << 10)` bit-packing pattern into three named helpers, then migrates all 23 call sites.

> **TS-faithfulness gate:** Not applicable — TS doesn't have these helpers either. The key format is a protocol convention encoded in both languages; goscape adds named Go-level helpers for readability. **Zero new deviations.**

> **Scope:** Approach 1 (most faithful). 2 tasks.

## 1. Goal

Replace the 23 inlined sites of the `trigger | (0x2 << 8) | (typeID << 10)` bit-packing pattern with three named helper functions. Behavior-preserving DRY refactor.

Observable gain: new trigger-wiring sub-specs no longer re-derive the key formula from protocol docs or copy it from neighboring tests. Mistyping the shift-by-10 or the 0x2-vs-0x1 selector becomes impossible at call sites.

## 2. Architecture

One new file, three trivial functions, mass-migration of call sites.

**New file `pkg/script/lookup_key.go`:**

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

## 3. File Map

| File | Action | Sites | Task |
|---|---|---|---|
| `pkg/script/lookup_key.go` | Create | 3 helper functions | 1 |
| `pkg/script/lookup_key_test.go` | Create | 3 formula tests + 3 boundary tests | 1 |
| `pkg/script/provider.go` | Modify | 4 inlined sites → helper calls (`GetByTrigger` + handleGosub-adjacent) | 1 |
| `pkg/script/provider_test.go` | Modify | 6 inlined sites | 2 |
| `modules/world/interaction_trigger_test.go` | Modify | 5 inlined sites | 2 |
| `modules/world/player_script_test.go` | Modify | 6 inlined sites | 2 |
| `modules/world/script_test.go` | Modify | 2 inlined sites | 2 |

## 4. Component Details

### 4.1 Helpers (§2 above)

Design choices:
- **Package-level functions**, not methods on Provider — no state needed.
- **`int` typeID/categoryID** — matches the inlined sites which already pass `uint32(...)` casts of int variables. Keeps call sites clean.
- **Return `uint32`** matching `ScriptFile.LookupKey` and `Provider.byKey` key types.
- **No `LookupKeyForGlobal` usage yet** in existing call sites (they either use GetByTrigger's fallback or pass `uint32(trigger)` raw). Still ship the helper for API symmetry and future global-trigger wiring.

### 4.2 Test coverage

`lookup_key_test.go` asserts:
- `LookupKeyForType(TriggerOpNpc1, 42) == uint32(TriggerOpNpc1) | 0x200 | (42 << 10)` — exact formula
- `LookupKeyForCategory(TriggerOpNpc1, 7) == uint32(TriggerOpNpc1) | 0x100 | (7 << 10)` — exact formula
- `LookupKeyForGlobal(TriggerOpNpc1) == uint32(TriggerOpNpc1)` — no bits set beyond trigger
- Boundary: typeID=0 → 0b00 in bits 10+
- Boundary: categoryID=0 → same
- Distinctness: for the same (trigger, id), Type key ≠ Category key ≠ Global key

### 4.3 Migration constraints

- **Net zero behavior**: every migrated site's pre-migration and post-migration computed value must be bit-for-bit equal. Any test that compared lookup keys against raw u32 literals continues working.
- **No reordering** of statements.
- **No variable renaming** at migrated sites (keep existing local names `key`, `specificKey`, `categoryKey`, `typeKey`, `catKey`, etc.).

## 5. Task Split

### Task 1 — Helpers + production migration

- Create `pkg/script/lookup_key.go` with 3 functions
- Create `pkg/script/lookup_key_test.go` with 6 tests
- Migrate 4 inlined sites in `pkg/script/provider.go`
- Build green; full repo tests green (helpers are a strict superset of inlined math)
- Commit: `feat(script): add LookupKeyFor{Type,Category,Global} helpers + migrate provider (S6q-1)`

### Task 2 — Test-site migration

- Migrate 19 test sites across 4 test files
- Full repo tests green
- Commit: `refactor(script,world): migrate inlined lookup-key packing to helpers (S6q-2)`

## 6. Deviations

**Zero new deviations.** Zero deviation closures (this is a refactor, not a behavior change).

## 7. Scope

- Impl: ~35 LOC (3 functions + 3-line migrations × 23)
- Tests: ~50 LOC new
- 2 commits
