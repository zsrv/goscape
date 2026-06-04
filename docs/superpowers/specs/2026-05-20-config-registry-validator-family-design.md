# Config-registry validator family port — design spec

**Date:** 2026-05-20
**Predecessor close memory:** `[[setgender-genderval-port-close]]`
**Branch base:** `main` at `6f078e25`

## 1. Goal

Port the 7 remaining `ScriptInputConfigTypeValidator` helpers from TS `ScriptValidators.ts` to goscape's `pkg/script/`, completing the "registry-presence" validator family started in earlier slices.

## 2. Scope

### In scope

- New validators: `checkLocType`, `checkParamType`, `checkEnumType`, `checkIdkType`, `checkStructType`, `checkMesanimType`, `checkFontType`
- Wire every existing call site (`s.Configs.XType(id)` → `checkXType(s, id, "OP_NAME")` + safe deref)
- Adopt canonical error wording `"%s: no XType with value (%d) found"` (replaces ~20 bespoke `"unknown X id %d"` strings)
- 21 new validator unit tests + ~20 handler test assertion updates + 1 new font-error boundary test

### Out of scope (deferred, per existing pins or blocking conditions)

- `HuntTypeValid` — no `Configs.HuntType` getter exists yet. Informal English deferral; leave as-is.
- `CategoryTypeValid` full count-bound — pinned `S7f-D3` (no CategoryType loader). `checkCategoryType` already has the null-sentinel half.
- `VarPlayerValid` / `VarNpcValid` / `VarSharedValid` — pinned `DEVIATION-NAI-121-D3` (degraded-mode fallback). TS-faithful port would un-defer the deviation.

## 3. Validator definitions

All 7 helpers follow the established sibling pattern (`checkInvType`, `checkSeqType`, `checkNpcType`):

```go
func checkX(s *ScriptState, id int, op string) error {
    if s.Configs == nil || s.Configs.XType(id) == nil {
        return fmt.Errorf("%s: no XType with value (%d) found", op, id)
    }
    return nil
}
```

| Validator | TS mirror | Home file |
|---|---|---|
| `checkLocType` | `LocTypeValid` (ScriptValidators.ts:105) | `handlers_loc.go` |
| `checkParamType` | `ParamTypeValid` (ScriptValidators.ts:110) | `handlers_config.go` |
| `checkEnumType` | `EnumTypeValid` (ScriptValidators.ts:119) | `handlers_config.go` |
| `checkStructType` | `StructTypeValid` (ScriptValidators.ts:133) | `handlers_config.go` |
| `checkIdkType` | `IDKTypeValid` (ScriptValidators.ts:124) | `handlers_player.go` |
| `checkMesanimType` | `MesanimValid` (ScriptValidators.ts:132) | `handlers_string.go` |
| `checkFontType` | `FontTypeValid` (ScriptValidators.ts:131) | `handlers_string.go` |

**Naming note:** TS uses `IDKTypeValid` (all-caps); goscape uses `IdkType` everywhere (per `objtype/idktype.go`). Validator is `checkIdkType` to match existing-codebase casing — sibling `checkSeqType` not `checkSEQType` precedent.

**Configs nil-guard:** Mirrors `checkInvType` exactly — checks `s.Configs == nil` before deref. Handlers also call `requireConfigs(s, op)` for their own nil-Configs guard; the validator's check is a defensive second layer that's harmless when the handler already early-returned.

## 4. Call-site wiring

### Pattern at every site

**Before:**
```go
lt := s.Configs.LocType(id)
if lt == nil {
    return fmt.Errorf("LC_NAME: unknown loc id %d", id)
}
// use lt
```

**After:**
```go
if err := checkLocType(s, id, "LC_NAME"); err != nil {
    return err
}
lt := s.Configs.LocType(id)  // guaranteed non-nil
// use lt
```

No behavior change at refactor sites — same error semantics, new error wording.

### Call sites enumerated (from initial grep)

| Validator | Sites | Files |
|---|---|---|
| `checkLocType` | ~12 | `handlers_config.go` (7 LC_* sites: lines 155, 176, 189, 204, 222, 240, 254), `handlers_loc.go` (multiple sites at 126, 228, 250, 273, 335, 356, 420 — verify each is `Configs.LocType` not `ActiveLoc.LocType()`), `handlers_map.go` (line 354) |
| `checkParamType` | 2 | `handlers_config.go` (`paramLookup` helper, line 21), `handlers_inv.go` (line 234) |
| `checkEnumType` | 3 | `handlers_config.go` (lines 79, 121) — plus any others discovered during impl |
| `checkStructType` | 1 | `handlers_config.go` (line 139) |
| `checkIdkType` | 1 | `handlers_player.go` (line 246, `handleSetIdKit`) |
| `checkMesanimType` | 1 | `handlers_string.go` (line 209) |
| `checkFontType` | 1 | `handlers_string.go` (line 145) — **behavior change site**, see §4.1 |

### 4.1 `checkFontType` behavior change

`handlers_string.go:145` currently uses a silent-skip pattern:

```go
if font := s.Configs.FontType(fontId); font != nil {
    // ...
}
```

Missing font ID is silently absorbed (no error, no push). TS-literal port via `check(fontId, FontTypeValid)` throws on missing.

**Port flips this site from silent-skip to error-return.** This is the slice's sole behavior-change site. Implementer task (T5) must:

1. Identify the host opcode (likely `FONT_SETSIZE` or related split-pages handler)
2. Add `TestHandleX_RejectsUnknownFontID` test pinning the new error-return path
3. Audit `LostCityRS/Content/scripts/` for any script that intentionally passes invalid font IDs — see §6 for the conditional new-pin protocol

### 4.2 Sites that look like getter calls but aren't

- `s.ActiveLoc.LocType()` reads — loc-instance type accessor, not Configs lookup. Leave unchanged.
- Iterations inside LOC_FIND batch loops where a missing type is expected — confirm against TS site-by-site during impl.

### 4.3 Audit-grep keywords (zero-tolerance post-slice)

The following must return 0 hits across `pkg/script/` (excluding archaeology comments in test docs):

- `unknown loc id`
- `unknown param id`
- `unknown enum id`
- `unknown struct id`
- `unknown idk id`
- `unknown mesanim id`
- `unknown font id`
- `param lookup: unknown`

## 5. Tests

### 5.1 Per-validator unit tests

3 cases per new helper × 7 validators = 21 new tests:

```go
func TestCheckLocType_NilConfigs(t *testing.T) { ... }
func TestCheckLocType_UnregisteredID_Errors(t *testing.T) { ... }
func TestCheckLocType_RegisteredID_OK(t *testing.T) { ... }
```

Sibling pattern to existing `TestCheckInvType_*` cases. Reuse `fakeConfigs` test double from existing handler tests.

### 5.2 Handler test error-wording updates

~15-20 existing handler tests pattern-match on `"unknown <X> id"` strings. Each flips to assert on `"no XType with value"`. Touched files:

- `handlers_config_test.go` — LC_*, EnumType, StructType handlers
- `handlers_loc_test.go` — LocType inline-guard handlers
- `handlers_inv_test.go` — ParamType wiring site
- `handlers_player_test.go` — SETIDKIT
- `handlers_string_test.go` — Mesanim, Font

### 5.3 New behavior test (font flip)

`TestHandleX_RejectsUnknownFontID` — pins the new font-error path introduced by §4.1. One test, identifies host opcode during T5 impl.

### 5.4 Standard gates

- `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./...` clean across all packages
- `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run TestPackAll_TwelveStageSmoke ./...` — 12 OK / 0 ERR / 0 SKIP

## 6. Deviation board

**Retired pins:** None directly. Slice is a refactor with one TS-faithfulness correction (§4.1) that doesn't have a pre-existing tag.

**Opened pins:** None expected. Out-of-scope items already have pins.

**Conditional new pin (T5):** If `LostCityRS/Content/scripts/` audit shows any script depends on the silent-skip behavior at `handlers_string.go:145`, open `NAI-FONT-D-TS-LITERAL-SKIP-REVERT` and revert that one site to silent-skip. Implementer must run the audit before finalizing T5. Expected outcome: no risky uses, no new pin opens.

**Net deviation board:** ~142 → ~142 live `NAI-XXX-D-*` pins (no change).

## 7. Build sequence (task breakdown)

Subagent-driven-development pattern, 6 tasks, two-stage review per task (sonnet spec-reviewer + sonnet feature-dev:code-reviewer).

### T1 — Define all 7 validator functions + unit tests

- Add the 7 `checkX` helpers across `handlers_loc.go`, `handlers_config.go`, `handlers_player.go`, `handlers_string.go` (locations per §3)
- Write 21 unit tests using existing `fakeConfigs` pattern
- **No call-site wiring** — production handlers still use inline guards
- Acceptance: helpers compile, 21 unit tests green, no other production code touched

### T2 — Wire `checkLocType` call sites

- `handlers_config.go` (7 LC_* sites) + `handlers_loc.go` (verify each site is `Configs.LocType` not `ActiveLoc.LocType()`) + `handlers_map.go` (1 site)
- Update corresponding handler tests' error-wording assertions
- Acceptance: `grep -n "unknown loc id" pkg/script/` returns 0 hits

### T3 — Wire `checkParamType` call sites

- `handlers_config.go` (`paramLookup` helper) + `handlers_inv.go` (line 234)
- Update tests; `paramLookup`'s wording change cascades to every opcode that calls it
- Acceptance: `grep -n "unknown param id\|param lookup: unknown" pkg/script/` returns 0 hits

### T4 — Wire `checkEnumType` + `checkStructType` call sites

- `handlers_config.go` (3 enum + 1 struct sites)
- Update tests
- Acceptance: `grep -n "unknown enum id\|unknown struct id" pkg/script/` returns 0 hits

### T5 — Wire `checkIdkType` + `checkMesanimType` + `checkFontType` + font behavior pin

- `handlers_player.go` SETIDKIT + `handlers_string.go` Mesanim + Font sites
- **Font site is the slice's sole behavior-change site** (§4.1) — flip silent-skip to error-return
- Audit `LostCityRS/Content/scripts/` per §6; open `NAI-FONT-D-TS-LITERAL-SKIP-REVERT` if needed
- Add `TestHandleX_RejectsUnknownFontID` boundary test
- Update other tests
- Acceptance: `grep -n "unknown idk id\|unknown mesanim id\|unknown font id" pkg/script/` returns 0 hits; new font-error test green

### T6 — Carry-forward audit + doc-comment refresh

- Audit-grep predecessor-style: `"stays raw"`, `"wrapped via"`, `"unknown <X> id"` phrasing in production AND test doc comments
- Doc-comment refresh: any handler's doc comment that mentions the old inline-guard pattern updates to reference the validator helper
- Final gates per §5.4
- Acceptance: zero stale phrasing hits; both gates green

### Task ordering rationale

T1 lands all helpers first so T2-T5 are independently runnable. Run sequentially for review-checkpoint clarity rather than parallelizing. T6 closes with audit + gates.

### Pre-acknowledged spec-vs-plan deviations to expect during planning

- T2 site count may shift if some `handlers_loc.go` lines turn out to be `ActiveLoc.LocType()` reads — implementer corrects inline with comment
- T5's font behavior-change host opcode needs identification during impl
- `paramLookup` test wording cascade in T3 may touch more tests than the initial estimate

## 8. Close criteria

1. `-race ./...` clean across all 57+ packages
2. `TestPackAll_TwelveStageSmoke` — 12 OK / 0 ERR / 0 SKIP
3. Audit-grep clean per §4.3
4. Doc-comment audit clean
5. Whole-slice opus reviewer: zero Critical/Important findings

### Close commit message format

```
chore(close): config-registry validator family port

Ports the 7 remaining ScriptInputConfigTypeValidator helpers
(LocType, ParamType, EnumType, IdkType, StructType, MesanimType,
FontType) from TS ScriptValidators.ts. Wires every call site
to the new checkXType helpers. Unifies error wording to canonical
"%s: no XType with value (%d) found" format. One TS-faithfulness
behavior correction: font silent-skip → error-return
(handlers_string.go:145 pre-port). 21 new validator unit tests
+ ~20 handler test assertion updates + 1 new font-error boundary
test.

-race ./... clean. TestPackAll_TwelveStageSmoke PASS.
```

### Memory entry to write at close

`[[config-registry-validator-family-close]]` — slice digest, audit findings, font-site decision outcome (revert or stay), commit range.

## 9. Carry-forward (not addressed by this slice)

- `HuntTypeValid` — requires `Configs.HuntType` getter; full HuntType registry port is its own slice
- `CategoryTypeValid` full count-bound — requires CategoryType loader (`S7f-D3` still live)
- `VarPlayerValid` / `VarNpcValid` / `VarSharedValid` — un-deferring `DEVIATION-NAI-121-D3` requires removing the degraded-mode fallback (separate decision)
- Compiler-side 0-based `ai_queueN` emission rework (`[[queue-skincolour-validator-slice-close]]` §9 carry-forward)
- Behavioral HitType handling (HP-on-BLOCK gate etc., `[[hit-type-validator-slice-close]]` §9 carry-forward)
- NAI-162 analytics RPC (multi-predecessor carry-forward)
- Combat sub-spec follow-ups (`[[nai184-combat-level-recompute-close]]`)
