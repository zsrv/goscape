# Config-Registry Validator Family Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Port the 7 remaining `ScriptInputConfigTypeValidator` helpers from TS `ScriptValidators.ts` to goscape's `pkg/script/`, unifying error wording across ~20 call sites with zero runtime behavior change.

**Architecture:** Add 7 sibling `checkXType(s *ScriptState, id int, op string) error` helpers (matching the existing `checkInvType` / `checkNpcType` / `checkSeqType` shape). Wire only at sites with current error-return guards (refactor). Define-but-don't-wire at the two labeled goscape-defensive gates (font/mesanim) per existing `defensive_gate_doc_comment_label.md` convention.

**Tech Stack:** Go 1.26.x; existing `pkg/script/` test infrastructure (`mockConfigs`, `newTestConfigs`, `runConfigOpExpectErr`).

**Spec:** `docs/superpowers/specs/2026-05-20-config-registry-validator-family-design.md`

---

## Planning-time deviations from spec

**Spec §4.1 dropped.** The font silent-skip at `handlers_string.go:145` is a labeled goscape-defensive gate (doc-comment at line 127-128 cites `defensive_gate_doc_comment_label.md`). Similarly `handlers_string.go:209` (mesanim) — labeled at line 201-202. Both sites stay silent. `checkFontType` and `checkMesanimType` are defined in T1 but unwired in production this slice.

**Consequence:** Spec §6 conditional pin `NAI-FONT-D-TS-LITERAL-SKIP-REVERT` is moot (no behavior change). T5 collapses (no font-error boundary test needed). Task count drops from 6 → 5.

**Net effect:** Slice is now pure refactor + uniform error wording. Zero behavior change at any production site.

## File map

**Production files modified (5):**
- `pkg/script/handlers_loc.go` — add `checkLocType` (T1); wire 7 LocType sites (T2)
- `pkg/script/handlers_config.go` — add `checkParamType`/`checkEnumType`/`checkStructType` (T1); wire 7 LocType (T2) + 1 ParamType in `paramLookup` (T3) + 2 EnumType + 1 StructType (T4) = 11 sites
- `pkg/script/handlers_player.go` — add `checkIdkType` (T1); wire 1 IdkType site at SETIDKIT (T4)
- `pkg/script/handlers_string.go` — add `checkMesanimType`/`checkFontType` (T1, defined but unwired this slice)
- `pkg/script/handlers_inv.go` — wire 1 ParamType site at INV_TOTALPARAM (T3)

**Test files modified (5):**
- `pkg/script/handlers_loc_test.go` — update LOC_* error wording
- `pkg/script/handlers_config_test.go` — update LC_/ENUM_/STRUCT_PARAM/OC_PARAM error wording; add validator unit tests (per home file convention)
- `pkg/script/handlers_player_test.go` — add `TestCheckIdkType`; update SETIDKIT error wording
- `pkg/script/handlers_string_test.go` — add `TestCheckMesanimType`, `TestCheckFontType`
- `pkg/script/handlers_inv_test.go` — update INV_TOTALPARAM error wording
- `pkg/script/handlers_npc_test.go` — add `TestCheckLocType` (sibling location to `TestCheckNpcType`)

**Sites that stay silent-defensive (NO wiring, intentional):**
- `handlers_loc.go:164` (handleLocOp empty-string fallback — not labeled but defensive shape)
- `handlers_map.go:354` (handleMapLocAddUnsafe batch iter — silent skip in loop)
- `handlers_string.go:145` (handleSplitInit font fallback — labeled defensive)
- `handlers_string.go:209` (handleSplitGetAnim mesanim fallback — labeled defensive)

---

## Task 1: Define all 7 validator helpers + unit tests

**Files:**
- Modify: `pkg/script/handlers_loc.go` (add `checkLocType`)
- Modify: `pkg/script/handlers_config.go` (add `checkParamType`, `checkEnumType`, `checkStructType`)
- Modify: `pkg/script/handlers_player.go` (add `checkIdkType`)
- Modify: `pkg/script/handlers_string.go` (add `checkMesanimType`, `checkFontType`)
- Modify: `pkg/script/handlers_npc_test.go` (add `TestCheckLocType` — sibling location)
- Modify: `pkg/script/handlers_config_test.go` (add `TestCheckParamType`, `TestCheckEnumType`, `TestCheckStructType`)
- Modify: `pkg/script/handlers_player_test.go` (add `TestCheckIdkType`)
- Modify: `pkg/script/handlers_string_test.go` (add `TestCheckMesanimType`, `TestCheckFontType`)

**No call-site wiring in T1.** Production handlers still use inline guards after T1.

- [ ] **Step 1: Add `checkLocType` to `handlers_loc.go`**

Place adjacent to other `check*` helpers near the top of the file. After:

```go
// checkLocType mirrors TS LocTypeValid (ScriptValidators.ts:105) — a
// ScriptInputConfigTypeValidator over LocType. Both the range check
// (0 <= id < LocType.count) and the registry-present check collapse
// into "s.Configs.LocType(id) != nil" per the Configs interface
// contract at configs.go:7. State-aware signature matches sibling
// checkInvType / checkSeqType / checkNpcType.
func checkLocType(s *ScriptState, id int, op string) error {
	if s.Configs == nil || s.Configs.LocType(id) == nil {
		return fmt.Errorf("%s: no LocType with value (%d) found", op, id)
	}
	return nil
}
```

- [ ] **Step 2: Add `checkParamType`, `checkEnumType`, `checkStructType` to `handlers_config.go`**

Place after `requireConfigs` (line 58-63), before the `paramLookup` and other handlers:

```go
// checkParamType mirrors TS ParamTypeValid (ScriptValidators.ts:110).
func checkParamType(s *ScriptState, id int, op string) error {
	if s.Configs == nil || s.Configs.ParamType(id) == nil {
		return fmt.Errorf("%s: no ParamType with value (%d) found", op, id)
	}
	return nil
}

// checkEnumType mirrors TS EnumTypeValid (ScriptValidators.ts:119).
func checkEnumType(s *ScriptState, id int, op string) error {
	if s.Configs == nil || s.Configs.EnumType(id) == nil {
		return fmt.Errorf("%s: no EnumType with value (%d) found", op, id)
	}
	return nil
}

// checkStructType mirrors TS StructTypeValid (ScriptValidators.ts:133).
func checkStructType(s *ScriptState, id int, op string) error {
	if s.Configs == nil || s.Configs.StructType(id) == nil {
		return fmt.Errorf("%s: no StructType with value (%d) found", op, id)
	}
	return nil
}
```

- [ ] **Step 3: Add `checkIdkType` to `handlers_player.go`**

Place adjacent to `checkInvType` and `checkSeqType` (around lines 156-177):

```go
// checkIdkType mirrors TS IDKTypeValid (ScriptValidators.ts:124).
// Goscape lowercase "Idk" follows the objtype.IdkType naming convention
// and the sibling checkSeqType (not "checkSEQType") precedent.
func checkIdkType(s *ScriptState, id int, op string) error {
	if s.Configs == nil || s.Configs.IdkType(id) == nil {
		return fmt.Errorf("%s: no IdkType with value (%d) found", op, id)
	}
	return nil
}
```

- [ ] **Step 4: Add `checkMesanimType` and `checkFontType` to `handlers_string.go`**

Place near the top of the file (after imports / before first handler):

```go
// checkMesanimType mirrors TS MesanimValid (ScriptValidators.ts:132).
// Defined for completeness — current call site (handleSplitGetAnim,
// handlers_string.go:209) is a labeled goscape-defensive gate per
// defensive_gate_doc_comment_label.md and does not wire this validator.
func checkMesanimType(s *ScriptState, id int, op string) error {
	if s.Configs == nil || s.Configs.MesanimType(id) == nil {
		return fmt.Errorf("%s: no MesanimType with value (%d) found", op, id)
	}
	return nil
}

// checkFontType mirrors TS FontTypeValid (ScriptValidators.ts:131).
// Defined for completeness — current call site (handleSplitInit,
// handlers_string.go:145) is a labeled goscape-defensive gate per
// defensive_gate_doc_comment_label.md and does not wire this validator.
func checkFontType(s *ScriptState, id int, op string) error {
	if s.Configs == nil || s.Configs.FontType(id) == nil {
		return fmt.Errorf("%s: no FontType with value (%d) found", op, id)
	}
	return nil
}
```

- [ ] **Step 5: Verify compile**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...`
Expected: clean exit, no compile errors.

- [ ] **Step 6: Add `TestCheckLocType` to `handlers_npc_test.go`**

Place sibling to `TestCheckNpcType` (around line 55). Sibling test-table form:

```go
func TestCheckLocType(t *testing.T) {
	tests := []struct {
		name      string
		id        int
		setup     func() *mockConfigs
		wantErr   bool
		wantSubst string
	}{
		{
			name:    "valid id",
			id:      0,
			setup:   func() *mockConfigs { return &mockConfigs{locs: map[int]*objtype.LocType{0: {}}} },
			wantErr: false,
		},
		{
			name:      "unknown id",
			id:        100,
			setup:     func() *mockConfigs { return &mockConfigs{locs: map[int]*objtype.LocType{}} },
			wantErr:   true,
			wantSubst: "OP: no LocType with value (100) found",
		},
		{
			name:      "negative id",
			id:        -1,
			setup:     func() *mockConfigs { return &mockConfigs{locs: map[int]*objtype.LocType{}} },
			wantErr:   true,
			wantSubst: "OP: no LocType with value (-1) found",
		},
		{
			name:      "nil Configs",
			id:        0,
			setup:     func() *mockConfigs { return nil },
			wantErr:   true,
			wantSubst: "OP: no LocType with value (0) found",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := &ScriptState{}
			if cfg := tc.setup(); cfg != nil {
				s.Configs = cfg
			}
			err := checkLocType(s, tc.id, "OP")
			if tc.wantErr {
				if err == nil {
					t.Fatalf("checkLocType(%d): want error, got nil", tc.id)
				}
				if !strings.Contains(err.Error(), tc.wantSubst) {
					t.Errorf("error message: got %q, want contains %q", err.Error(), tc.wantSubst)
				}
			} else {
				if err != nil {
					t.Fatalf("checkLocType(%d): want nil, got %v", tc.id, err)
				}
			}
		})
	}
}
```

- [ ] **Step 7: Add `TestCheckParamType`, `TestCheckEnumType`, `TestCheckStructType` to `handlers_config_test.go`**

Place near the top of the file (after the `mockConfigs` and `newTestConfigs` helpers, before the handler tests start around line 290). Use the same table-driven form as Step 6, replacing:
- `locs` map field → `params`/`enums`/`structs` respectively
- `objtype.LocType` → `objtype.ParamType` / `objtype.EnumType` / `objtype.StructType`
- Substring `"no LocType"` → `"no ParamType"` / `"no EnumType"` / `"no StructType"`
- Function name `checkLocType` → `checkParamType` / `checkEnumType` / `checkStructType`

For each test:

```go
func TestCheckParamType(t *testing.T) {
	tests := []struct {
		name      string
		id        int
		setup     func() *mockConfigs
		wantErr   bool
		wantSubst string
	}{
		{
			name:    "valid id",
			id:      1,
			setup:   func() *mockConfigs { return &mockConfigs{params: map[int]*objtype.ParamType{1: {}}} },
			wantErr: false,
		},
		{
			name:      "unknown id",
			id:        100,
			setup:     func() *mockConfigs { return &mockConfigs{params: map[int]*objtype.ParamType{}} },
			wantErr:   true,
			wantSubst: "OP: no ParamType with value (100) found",
		},
		{
			name:      "negative id",
			id:        -1,
			setup:     func() *mockConfigs { return &mockConfigs{params: map[int]*objtype.ParamType{}} },
			wantErr:   true,
			wantSubst: "OP: no ParamType with value (-1) found",
		},
		{
			name:      "nil Configs",
			id:        0,
			setup:     func() *mockConfigs { return nil },
			wantErr:   true,
			wantSubst: "OP: no ParamType with value (0) found",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := &ScriptState{}
			if cfg := tc.setup(); cfg != nil {
				s.Configs = cfg
			}
			err := checkParamType(s, tc.id, "OP")
			if tc.wantErr {
				if err == nil {
					t.Fatalf("checkParamType(%d): want error, got nil", tc.id)
				}
				if !strings.Contains(err.Error(), tc.wantSubst) {
					t.Errorf("error message: got %q, want contains %q", err.Error(), tc.wantSubst)
				}
			} else {
				if err != nil {
					t.Fatalf("checkParamType(%d): want nil, got %v", tc.id, err)
				}
			}
		})
	}
}
```

Repeat for `TestCheckEnumType` (id=0 valid case, `enums` map, `objtype.EnumType`) and `TestCheckStructType` (id=0 valid case, `structs` map, `objtype.StructType`).

- [ ] **Step 8: Add `TestCheckIdkType` to `handlers_player_test.go`**

Place sibling to `TestCheckInvType` (around line 2098). Same table-driven form as Step 6, using `idks` field, `objtype.IdkType`, `"no IdkType"` substring, `checkIdkType` function.

- [ ] **Step 9: Add `TestCheckMesanimType` and `TestCheckFontType` to `handlers_string_test.go`**

If `handlers_string_test.go` doesn't exist yet, create it with:

```go
package script

import (
	"strings"
	"testing"

	"github.com/zsrv/goscape/pkg/fonttype"
	"github.com/zsrv/goscape/pkg/objtype"
)
```

Then add `TestCheckMesanimType` using `mesanims` map, `objtype.MesanimType`, `"no MesanimType"`, `checkMesanimType`.

For `TestCheckFontType`, the registry uses the `fonttype` package not `objtype`:

```go
func TestCheckFontType(t *testing.T) {
	tests := []struct {
		name      string
		id        int
		setup     func() *mockConfigs
		wantErr   bool
		wantSubst string
	}{
		{
			name:    "valid id",
			id:      0,
			setup:   func() *mockConfigs { return &mockConfigs{fonts: map[int]*fonttype.FontType{0: {}}} },
			wantErr: false,
		},
		{
			name:      "unknown id",
			id:        100,
			setup:     func() *mockConfigs { return &mockConfigs{fonts: map[int]*fonttype.FontType{}} },
			wantErr:   true,
			wantSubst: "OP: no FontType with value (100) found",
		},
		{
			name:      "negative id",
			id:        -1,
			setup:     func() *mockConfigs { return &mockConfigs{fonts: map[int]*fonttype.FontType{}} },
			wantErr:   true,
			wantSubst: "OP: no FontType with value (-1) found",
		},
		{
			name:      "nil Configs",
			id:        0,
			setup:     func() *mockConfigs { return nil },
			wantErr:   true,
			wantSubst: "OP: no FontType with value (0) found",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := &ScriptState{}
			if cfg := tc.setup(); cfg != nil {
				s.Configs = cfg
			}
			err := checkFontType(s, tc.id, "OP")
			if tc.wantErr {
				if err == nil {
					t.Fatalf("checkFontType(%d): want error, got nil", tc.id)
				}
				if !strings.Contains(err.Error(), tc.wantSubst) {
					t.Errorf("error message: got %q, want contains %q", err.Error(), tc.wantSubst)
				}
			} else {
				if err != nil {
					t.Fatalf("checkFontType(%d): want nil, got %v", tc.id, err)
				}
			}
		})
	}
}
```

- [ ] **Step 10: Run all 7 new validator unit tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run 'TestCheckLocType|TestCheckParamType|TestCheckEnumType|TestCheckStructType|TestCheckIdkType|TestCheckMesanimType|TestCheckFontType' ./pkg/script/ -v`
Expected: all 7 tests PASS (each with 4 sub-cases = 28 sub-test runs).

- [ ] **Step 11: Run full test suite to verify no regressions**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/`
Expected: PASS (no production code changed yet — only new defs and tests).

- [ ] **Step 12: Commit**

```bash
git status
git add pkg/script/handlers_loc.go pkg/script/handlers_config.go pkg/script/handlers_player.go pkg/script/handlers_string.go pkg/script/handlers_npc_test.go pkg/script/handlers_config_test.go pkg/script/handlers_player_test.go pkg/script/handlers_string_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): add 7 config-registry validator helpers (T1)

Adds checkLocType, checkParamType, checkEnumType, checkStructType,
checkIdkType, checkMesanimType, checkFontType — mirroring TS
ScriptValidators.ts entries. Same sibling shape as existing
checkInvType/checkSeqType/checkNpcType. 7 new validator unit tests
(28 sub-cases) using existing mockConfigs test double.

No production call-site wiring in T1 — handlers still use inline
guards. Wiring lands in T2-T4. checkMesanimType and checkFontType
defined but intentionally unwired this slice: their would-be call
sites are labeled goscape-defensive gates per
defensive_gate_doc_comment_label.md.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
git show --stat HEAD
```

---

## Task 2: Wire `checkLocType` at 14 error-return call sites

**Files:**
- Modify: `pkg/script/handlers_config.go` (7 sites)
- Modify: `pkg/script/handlers_loc.go` (7 sites)
- Modify: `pkg/script/handlers_config_test.go` (1 assertion)
- Modify: `pkg/script/handlers_loc_test.go` (3+ exact-equality assertions)

**Sites to wire (all currently have `if x == nil { return fmt.Errorf("OP: unknown loc id %d", id) }` form):**

| # | File | Line | Op label |
|---|---|---|---|
| 1 | `handlers_config.go` | 155-158 | `LC_NAME` |
| 2 | `handlers_config.go` | 176-179 | `LC_PARAM` |
| 3 | `handlers_config.go` | 189-192 | `LC_CATEGORY` |
| 4 | `handlers_config.go` | 204-207 | `LC_DESC` |
| 5 | `handlers_config.go` | 222-225 | `LC_DEBUGNAME` |
| 6 | `handlers_config.go` | 240-243 | `LC_WIDTH` |
| 7 | `handlers_config.go` | 254-257 | `LC_LENGTH` |
| 8 | `handlers_loc.go` | 126-128 | `LOC_FIND` |
| 9 | `handlers_loc.go` | 228-231 | `LOC_CATEGORY` |
| 10 | `handlers_loc.go` | 250-253 | `LOC_TYPE` |
| 11 | `handlers_loc.go` | 273-276 | `LOC_NAME` |
| 12 | `handlers_loc.go` | 335-337 | `LOC_CHANGE` |
| 13 | `handlers_loc.go` | 356-359 | `LOC_PARAM` |
| 14 | `handlers_loc.go` | 420-422 | `LOC_ADD` |

**Sites NOT to wire (silent-defensive — leave inline guard alone):**
- `handlers_loc.go:164` (handleLocOp empty-string fallback)
- `handlers_map.go:354` (handleMapLocAddUnsafe batch iter)

- [ ] **Step 1: Refactor site 1 (LC_NAME, handlers_config.go:150-167) — representative pattern**

Before (lines 150-158):
```go
func handleLcName(s *ScriptState) error {
	if err := requireConfigs(s, "LC_NAME"); err != nil {
		return err
	}
	id := s.PopInt()
	lt := s.Configs.LocType(id)
	if lt == nil {
		return fmt.Errorf("LC_NAME: unknown loc id %d", id)
	}
```

After:
```go
func handleLcName(s *ScriptState) error {
	if err := requireConfigs(s, "LC_NAME"); err != nil {
		return err
	}
	id := s.PopInt()
	if err := checkLocType(s, id, "LC_NAME"); err != nil {
		return err
	}
	lt := s.Configs.LocType(id)
```

The subsequent `if lt.Name != ""` etc. block is unchanged (lt is now guaranteed non-nil).

- [ ] **Step 2: Refactor all remaining 6 LC_* sites in `handlers_config.go` (sites 2-7) using the same pattern**

For each of LC_PARAM, LC_CATEGORY, LC_DESC, LC_DEBUGNAME, LC_WIDTH, LC_LENGTH:
- Replace `lt := s.Configs.LocType(id); if lt == nil { return fmt.Errorf("OP: unknown loc id %d", id) }`
- With `if err := checkLocType(s, id, "OP"); err != nil { return err }; lt := s.Configs.LocType(id)`

Note: LC_PARAM uses `locID` not `id` as the variable name (line 174); preserve the local variable name.

- [ ] **Step 3: Refactor 7 sites in `handlers_loc.go`**

For each of the sites at lines 126-128, 228-231, 250-253, 273-276, 335-337, 356-359, 420-422:
- Two forms exist:
  - **Two-variable form** (e.g., line 228): `lt := s.Configs.LocType(id); if lt == nil { return ... }` — replace with `checkLocType` + fetch (same as Step 1)
  - **Single-test form** (e.g., line 126): `if s.Configs.LocType(locId) == nil { return ... }` — replace with `if err := checkLocType(s, locId, "OP"); err != nil { return err }` (no subsequent fetch needed if handler doesn't use the type)

For LOC_ADD (site 14, line 420-422) — single-test form, no subsequent fetch. Just:

Before:
```go
if s.Configs.LocType(typ) == nil {
    return fmt.Errorf("LOC_ADD: unknown loc id %d", typ)
}
```

After:
```go
if err := checkLocType(s, typ, "LOC_ADD"); err != nil {
    return err
}
```

- [ ] **Step 4: Verify compile and zero stale wording**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...`
Expected: clean.

Run: `grep -n "unknown loc id" pkg/script/`
Expected: only `handlers_loc_test.go` hits remaining (~3-5 — test assertions to update next step).

- [ ] **Step 5: Update test assertions**

Find every test assertion that pattern-matches on `"unknown loc id"`:

```bash
grep -n "unknown loc id" pkg/script/handlers_config_test.go pkg/script/handlers_loc_test.go
```

Expected hits (verified):
- `handlers_config_test.go:485` — `runConfigOpExpectErr(t, mc, OpLcName, []int{999}, "unknown loc id")` → flip substring to `"no LocType with value (999) found"`
- `handlers_loc_test.go:326-327` — exact-equality `"LOC_CATEGORY: unknown loc id 999"` → flip to `"LOC_CATEGORY: no LocType with value (999) found"`
- `handlers_loc_test.go:384-385` — exact-equality `"LOC_TYPE: unknown loc id 999"` → flip to `"LOC_TYPE: no LocType with value (999) found"`
- `handlers_loc_test.go:1238` — `got, want := err.Error(), "LOC_FIND: unknown loc id 999"` → flip want string to `"LOC_FIND: no LocType with value (999) found"`

Also grep for `handlers_config_test.go` other LC_* assertions that may exist beyond line 485 — there may be more not in the initial grep due to varied substring forms.

```bash
grep -nE 'unknown loc id|LC_[A-Z]+: unknown' pkg/script/handlers_config_test.go pkg/script/handlers_loc_test.go
```

Update each hit to the canonical wording.

- [ ] **Step 6: Run targeted tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run 'TestLc|TestLoc|TestHandleLoc|TestHandleLc' ./pkg/script/ -v`
Expected: PASS.

- [ ] **Step 7: Run full test suite for regressions**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/`
Expected: PASS.

- [ ] **Step 8: Audit-grep clean**

Run: `grep -rn "unknown loc id" pkg/script/`
Expected: 0 hits.

- [ ] **Step 9: Commit**

```bash
git status
git add pkg/script/handlers_config.go pkg/script/handlers_loc.go pkg/script/handlers_config_test.go pkg/script/handlers_loc_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
refactor(script): wire checkLocType at 14 error-return sites (T2)

Replaces inline "if s.Configs.LocType(id) == nil" guards with
checkLocType(s, id, "OP") helper at 7 sites in handlers_config.go
(LC_NAME, LC_PARAM, LC_CATEGORY, LC_DESC, LC_DEBUGNAME, LC_WIDTH,
LC_LENGTH) + 7 sites in handlers_loc.go (LOC_FIND, LOC_CATEGORY,
LOC_TYPE, LOC_NAME, LOC_CHANGE, LOC_PARAM, LOC_ADD). Unifies error
wording to canonical "%s: no LocType with value (%d) found" format.

Updates corresponding test assertions in handlers_config_test.go
and handlers_loc_test.go.

Sites NOT wired (intentional silent-defensive): handlers_loc.go:164
(handleLocOp empty-string fallback), handlers_map.go:354
(handleMapLocAddUnsafe batch iter).

Zero behavior change. grep "unknown loc id" pkg/script/ → 0 hits.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
git show --stat HEAD
```

---

## Task 3: Wire `checkParamType` at 2 call sites

**Files:**
- Modify: `pkg/script/handlers_config.go` (`paramLookup` helper, line 17-54)
- Modify: `pkg/script/handlers_inv.go` (`handleInvTotalParam`, line ~234)
- Modify: `pkg/script/handlers_config_test.go` (1 assertion at line 744)
- Modify: `pkg/script/handlers_inv_test.go` (assertions matching `"INV_TOTALPARAM: unknown param id"`)

**Wording cascade note:** `paramLookup` is called by multiple opcodes (OC_PARAM, NC_PARAM, LC_PARAM, NPC_PARAM, LOC_PARAM, STRUCT_PARAM). The error wording change cascades to all of them — all become `"<OP>: no ParamType with value (X) found"` instead of `"param lookup: unknown param id %d"`. The current message even drops the originating opcode (just says "param lookup:"); the new message uses the op string passed by the calling handler.

- [ ] **Step 1: Refactor `paramLookup` in `handlers_config.go`**

Before (lines 17-24):
```go
func paramLookup(s *ScriptState, params objtype.ParamMap, paramID int) error {
	if s.Configs == nil {
		return fmt.Errorf("param lookup: Configs not set on ScriptState")
	}
	pt := s.Configs.ParamType(paramID)
	if pt == nil {
		return fmt.Errorf("param lookup: unknown param id %d", paramID)
	}
```

The signature needs an op string for the validator error. Update the signature and all callers:

After:
```go
func paramLookup(s *ScriptState, params objtype.ParamMap, paramID int, op string) error {
	if err := checkParamType(s, paramID, op); err != nil {
		return err
	}
	pt := s.Configs.ParamType(paramID)
```

(Drops the explicit Configs nil-check since `checkParamType` handles it. Drops the `"param lookup:"` prefix in favor of the calling op's name.)

- [ ] **Step 2: Update every `paramLookup` caller to pass `op` string**

Run: `grep -n "paramLookup(" pkg/script/`
Expected hits (each line is a caller — pass the calling handler's op-name string):

- `pkg/script/handlers_config.go:180` (LC_PARAM) → `paramLookup(s, lt.Params, paramID, "LC_PARAM")`
- Search for all other callers — likely OC_PARAM, NC_PARAM, NPC_PARAM, LOC_PARAM, STRUCT_PARAM each call paramLookup once. Use the corresponding op label.

For each caller, locate the existing `paramLookup(s, X.Params, paramID)` call site and add the op-name string as the 4th argument. Use the same op-label the handler already passes to its own `requireConfigs` / `checkX` calls.

- [ ] **Step 3: Refactor `handleInvTotalParam` in `handlers_inv.go`**

Read the current handler around line 234:
```bash
sed -n '225,245p' pkg/script/handlers_inv.go
```

Expected pattern:
```go
param := s.PopInt()
pt := s.Configs.ParamType(param)
if pt == nil {
    return fmt.Errorf("INV_TOTALPARAM: unknown param id %d", param)
}
```

Replace with:
```go
param := s.PopInt()
if err := checkParamType(s, param, "INV_TOTALPARAM"); err != nil {
    return err
}
pt := s.Configs.ParamType(param)
```

- [ ] **Step 4: Verify compile**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...`
Expected: clean (every paramLookup caller now passes 4 args).

- [ ] **Step 5: Update test assertions**

Find every assertion pattern-matching `"unknown param id"` or `"param lookup: unknown"`:

```bash
grep -rn "unknown param id\|param lookup: unknown" pkg/script/
```

Expected (verified):
- `handlers_config_test.go:744` — `runConfigOpExpectErr(t, mc, OpOcParam, []int{995, 999}, "unknown param id")` → flip substring to `"no ParamType with value (999) found"` (or `"OC_PARAM: no ParamType"` for stronger match).
- `handlers_inv_test.go` — if it has assertions on INV_TOTALPARAM error string, update similarly.

There may be MORE assertions in `handlers_config_test.go` for NC_PARAM, NPC_PARAM, LOC_PARAM, STRUCT_PARAM (each opcode probably has its own missing-param test). Grep:

```bash
grep -nE 'unknown param|param lookup' pkg/script/*_test.go
```

Update every hit to the canonical wording. The new error string per opcode looks like `"OC_PARAM: no ParamType with value (999) found"`.

- [ ] **Step 6: Run targeted tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run 'Param' ./pkg/script/ -v`
Expected: PASS.

- [ ] **Step 7: Run full test suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/`
Expected: PASS.

- [ ] **Step 8: Audit-grep clean**

Run: `grep -rn "unknown param id\|param lookup: unknown" pkg/script/`
Expected: 0 hits.

- [ ] **Step 9: Commit**

```bash
git status
git add pkg/script/handlers_config.go pkg/script/handlers_inv.go pkg/script/handlers_config_test.go pkg/script/handlers_inv_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
refactor(script): wire checkParamType at 2 call sites (T3)

Replaces paramLookup helper's inline Configs/ParamType nil-checks
with checkParamType validator. Adds op-name parameter to
paramLookup signature so the validator error carries the calling
opcode label (was "param lookup: unknown param id" — now
"<OP>: no ParamType with value (X) found"). Cascades wording
change across all paramLookup callers (OC_PARAM, NC_PARAM,
LC_PARAM, NPC_PARAM, LOC_PARAM, STRUCT_PARAM).

Also wires checkParamType at handlers_inv.go INV_TOTALPARAM
direct ParamType lookup.

Updates corresponding test assertions. Zero behavior change.
grep "unknown param id|param lookup: unknown" pkg/script/ → 0 hits.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
git show --stat HEAD
```

---

## Task 4: Wire `checkEnumType`, `checkStructType`, `checkIdkType`

**Files:**
- Modify: `pkg/script/handlers_config.go` (3 sites: lines 79, 121, 139)
- Modify: `pkg/script/handlers_player.go` (1 site: line 246, SETIDKIT)
- Modify: `pkg/script/handlers_config_test.go` (2 assertions at lines 360, 373, 398)
- Modify: `pkg/script/handlers_player_test.go` (SETIDKIT error wording assertion if present)

| # | File | Line | Op label | Validator |
|---|---|---|---|---|
| 1 | `handlers_config.go` | 79-81 | `ENUM` | `checkEnumType` |
| 2 | `handlers_config.go` | 121-123 | `ENUM_GETOUTPUTCOUNT` | `checkEnumType` |
| 3 | `handlers_config.go` | 139-141 | `STRUCT_PARAM` | `checkStructType` |
| 4 | `handlers_player.go` | 246-248 | `SETIDKIT` | `checkIdkType` |

- [ ] **Step 1: Refactor ENUM (handlers_config.go:79-81)**

Read context first to confirm op label:
```bash
sed -n '70,90p' pkg/script/handlers_config.go
```

Expected pattern:
```go
et := s.Configs.EnumType(enumID)
if et == nil {
    return fmt.Errorf("ENUM: unknown enum id %d", enumID)
}
```

Replace with:
```go
if err := checkEnumType(s, enumID, "ENUM"); err != nil {
    return err
}
et := s.Configs.EnumType(enumID)
```

- [ ] **Step 2: Refactor ENUM_GETOUTPUTCOUNT (handlers_config.go:121-123)**

Same shape as Step 1 with op label `"ENUM_GETOUTPUTCOUNT"`.

- [ ] **Step 3: Refactor STRUCT_PARAM (handlers_config.go:139-141)**

Same shape with op label `"STRUCT_PARAM"` and validator `checkStructType`:

Before:
```go
st := s.Configs.StructType(structID)
if st == nil {
    return fmt.Errorf("STRUCT_PARAM: unknown struct id %d", structID)
}
```

After:
```go
if err := checkStructType(s, structID, "STRUCT_PARAM"); err != nil {
    return err
}
st := s.Configs.StructType(structID)
```

- [ ] **Step 4: Refactor SETIDKIT (handlers_player.go:243-249)**

Before:
```go
if s.Configs == nil {
    return fmt.Errorf("SETIDKIT: invalid idkit %d", idkit)
}
idk := s.Configs.IdkType(idkit)
if idk == nil {
    return fmt.Errorf("SETIDKIT: invalid idkit %d", idkit)
}
```

After:
```go
if err := checkIdkType(s, idkit, "SETIDKIT"); err != nil {
    return err
}
idk := s.Configs.IdkType(idkit)
```

The explicit Configs nil-check + dual error sites collapse into one validator call (validator handles nil Configs).

- [ ] **Step 5: Verify compile**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...`
Expected: clean.

- [ ] **Step 6: Update test assertions**

Run: `grep -rn "unknown enum id\|unknown struct id\|invalid idkit\|unknown idk id" pkg/script/`

Expected hits to flip:
- `handlers_config_test.go:360` — `"unknown enum id"` → `"no EnumType with value"`
- `handlers_config_test.go:373` — `"unknown enum id"` → `"no EnumType with value"`
- `handlers_config_test.go:398` — `"unknown struct id"` → `"no StructType with value"`
- Any SETIDKIT test that asserts on `"invalid idkit"` — flip to `"no IdkType with value"`

Grep for SETIDKIT tests:
```bash
grep -n "invalid idkit\|SETIDKIT" pkg/script/handlers_player_test.go
```

Update each hit.

- [ ] **Step 7: Run targeted tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run 'Enum|Struct|SetIdKit|Idkit' ./pkg/script/ -v`
Expected: PASS.

- [ ] **Step 8: Run full test suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/`
Expected: PASS.

- [ ] **Step 9: Audit-grep clean**

Run: `grep -rn "unknown enum id\|unknown struct id\|invalid idkit\|unknown idk id" pkg/script/`
Expected: 0 hits.

- [ ] **Step 10: Commit**

```bash
git status
git add pkg/script/handlers_config.go pkg/script/handlers_player.go pkg/script/handlers_config_test.go pkg/script/handlers_player_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
refactor(script): wire checkEnumType/checkStructType/checkIdkType (T4)

Replaces inline nil-checks with validator calls at 4 sites:
ENUM + ENUM_GETOUTPUTCOUNT (checkEnumType, handlers_config.go),
STRUCT_PARAM (checkStructType, handlers_config.go),
SETIDKIT (checkIdkType, handlers_player.go). SETIDKIT also drops
its bespoke Configs-nil-check sentinel in favor of the validator's
collapsed check.

Unifies error wording: was "unknown enum id"/"unknown struct id"/
"invalid idkit" — now "%s: no <X>Type with value (%d) found".

Updates corresponding test assertions. Zero behavior change.
grep audit clean.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
git show --stat HEAD
```

---

## Task 5: Carry-forward audit + doc-comment refresh + close gates

**Files:**
- Audit only — no production code changes unless audit-grep surfaces stale references
- Possibly: doc-comment refresh in production OR test files

**Goal:** Catch every stale phrasing reference left over from the wording change. Predecessor pattern (HitType slice T6) caught test-doc references to retired terminology.

- [ ] **Step 1: Audit-grep for retired error wording across pkg/script/**

```bash
grep -rn "unknown loc id\|unknown param id\|unknown enum id\|unknown struct id\|unknown idk id\|invalid idkit\|param lookup: unknown" pkg/script/
```

Expected: 0 hits (any hit is a leftover from T2-T4).

- [ ] **Step 2: Audit-grep test doc-comments for phrasing references**

Per HitType slice T6 finding: pre-existing test doc-comments may reference the old wording in archaeological notes. Search broader:

```bash
grep -rn "stays raw\|wrapped via" pkg/script/ 2>/dev/null
```

Expected: review each hit. Most should be unrelated (HitType slice scope). If any reference the validator family ports (LocType/ParamType/etc.) — update the doc text to reflect the new wiring.

- [ ] **Step 3: Doc-comment refresh on production handlers**

For each handler touched in T2-T4 (paramLookup, ENUM, STRUCT_PARAM, LC_*, LOC_*, SETIDKIT, INV_TOTALPARAM), check whether its doc comment still references the old inline-guard pattern in a way that's now misleading.

Common stale patterns to look for:
- `// On unknown loc id, returns "OP: unknown loc id" error.`
- `// inline nil-check rejects unloaded types`
- `// custom error wording per opcode`

Update each to reflect the new validator wiring:
- `// Validates loc id via checkLocType (mirrors TS LocTypeValid).`

If the existing doc comment doesn't reference the validation form in detail, leave it.

- [ ] **Step 4: Doc-comment refresh on test docs**

Per predecessor pattern, test files often have archaeological doc comments. Spot-check:
- `handlers_loc_test.go` test docs referencing `"LOC_X: unknown loc id"`
- `handlers_config_test.go` test docs referencing param/enum/struct errors

Update phrasing as needed to reflect the new canonical wording.

- [ ] **Step 5: Final audit-grep — comprehensive**

```bash
grep -rn "unknown loc id\|unknown param id\|unknown enum id\|unknown struct id\|unknown idk id\|invalid idkit\|param lookup: unknown\|param lookup:" pkg/script/
```

Expected: 0 hits.

- [ ] **Step 6: Run `-race` gate**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./...`
Expected: PASS across all packages. Note: pre-existing `clientinterface: CRC mismatch (NAI-213-D-BUILDVERIFY-INTERFACE-MAY-DIVERGE)` noise is expected.

- [ ] **Step 7: Run smoke-pack gate**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run TestPackAll_TwelveStageSmoke ./...`
Expected: 12 OK / 0 ERR / 0 SKIP.

- [ ] **Step 8: Commit any T5 changes**

If Steps 1-5 surfaced stale references that needed updating:

```bash
git status
git add <files-changed>
git commit --no-gpg-sign -m "$(cat <<'EOF'
chore(script): retire stale validator-error wording references (T5)

Sweeps test doc-comments and handler doc-comments for stale
references to the pre-validator inline error wording ("unknown X
id", "invalid idkit", "param lookup: unknown"). Pure phrasing fix
— no logic change. grep audit clean across pkg/script/.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
git show --stat HEAD
```

If Steps 1-5 surfaced no changes needed: skip the commit and proceed to close commit (handled by close skill / final close commit).

- [ ] **Step 9: Final close commit**

```bash
git status
# Verify working tree is clean apart from standing untracked noise (config.yaml drift, .claude/, etc.)
git log --oneline a496ad48..HEAD
# Expected: 4-5 commits (T1, T2, T3, T4, optionally T5)

git commit --no-gpg-sign --allow-empty -m "$(cat <<'EOF'
chore(close): config-registry validator family port

Ports the 7 remaining ScriptInputConfigTypeValidator helpers
(LocType, ParamType, EnumType, IdkType, StructType, MesanimType,
FontType) from TS ScriptValidators.ts. Wires 20 call sites at
error-return form to the new checkXType helpers. Unifies error
wording to canonical "%s: no XType with value (%d) found" format.

MesanimType + FontType validators defined but intentionally unwired
this slice: their would-be call sites (handlers_string.go:209
handleSplitGetAnim, handlers_string.go:145 handleSplitInit) are
labeled goscape-defensive gates per defensive_gate_doc_comment_label.md.

7 new validator unit tests (28 sub-cases) + ~6 handler test
assertion updates. Zero behavior change anywhere.

-race ./... clean. TestPackAll_TwelveStageSmoke PASS 12/0/0.
EOF
)"
```

(`--allow-empty` is a safety net — if all T5 phrasing fixes happened in earlier commits, this final commit is a logical marker.)

- [ ] **Step 10: Verify board state**

```bash
git log --oneline a496ad48..HEAD
git show --stat HEAD
```

Confirm slice commit range, file count, and clean state for memory entry write at close.

---

## Spec coverage check

- §1 Goal — covered by slice as a whole
- §2 Scope (in/out) — T1 covers all 7 validator defs; T2-T4 cover all in-scope wiring; out-of-scope items left as-is
- §3 Validator definitions — T1 Steps 1-4
- §4 Call-site wiring — T2 (LocType), T3 (ParamType), T4 (Enum/Struct/Idk)
- §4.1 Font behavior change — **dropped per planning-time discovery (defensive convention)**
- §4.2 Sites that aren't candidates — explicitly enumerated in plan's "Sites that stay silent-defensive" section
- §4.3 Audit-grep keywords — T5 Steps 1-5
- §5 Tests — T1 Steps 6-9 (unit tests); T2-T4 test assertion updates
- §5.3 New behavior test — **dropped per planning-time discovery**
- §5.4 Standard gates — T5 Steps 6-7
- §6 Deviation board — no change (conditional pin dropped)
- §7 Build sequence — T1-T5 in this plan
- §8 Close criteria — T5 Step 9
- §9 Carry-forward — unchanged

## Self-review notes

- Type/function names consistent across tasks: `checkLocType`/`checkParamType`/`checkEnumType`/`checkStructType`/`checkIdkType`/`checkMesanimType`/`checkFontType` used identically in defs (T1) and call sites (T2-T4)
- All file paths absolute via `pkg/script/` prefix
- Test fixture references confirmed against `handlers_config_test.go:11-29` mockConfigs struct
- Error message format `"%s: no XType with value (%d) found"` consistent across all 7 validators
- One ambiguity flagged for implementer: T3 Step 2 says "Search for all other callers" of paramLookup — this is intentional since the grep is short (paramLookup is a single helper) and implementer can enumerate exactly. Acceptable plan-time deferral.
