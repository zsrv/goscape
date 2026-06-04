# NPC_NAME / NPC_CATEGORY read-side validator wiring

**Status:** Design
**Date:** 2026-05-21
**Predecessor:** `docs/superpowers/specs/2026-05-21-loc-npc-obj-readside-validator-wiring-design.md` (memory: `loc_npc_obj_readside_validator_wiring_shape_a_close.md`)
**HEAD at design:** `712a407a`

## 1. Summary

Wire the existing `checkNpcType` validator at the two read-side handlers `handleNpcName` (NPC_NAME) and `handleNpcCategory` (NPC_CATEGORY) in `pkg/script/handlers_npc.go`. Closes the Shape B subset of resume-memo item #1 ("NPC_NAME / NPC_CATEGORY silent-fallback semantic divergence") that the predecessor slice deferred at its §5.1.

Both handlers currently silently push a sentinel value (`"null"` / `-1`) when the `ActiveNpc.NpcType()` isn't registered in `Configs`. TS throws via `check(activeNpc.type, NpcTypeValid)` at the same sites. This slice converts the silent fallback to a script-level error to match TS, while preserving NPC_NAME's TS-faithful field-level `Name → DebugName → "null"` fallback cascade.

## 2. Motivation

### 2.1 TS source

`Engine-TS/src/engine/script/handlers/NpcOps.ts`:

```ts
// :68-70
[ScriptOpcode.NPC_CATEGORY]: checkedHandler(ActiveNpc, state => {
    state.pushInt(check(state.activeNpc.type, NpcTypeValid).category);
}),

// :270-272
[ScriptOpcode.NPC_NAME]: checkedHandler(ActiveNpc, state => {
    state.pushString(check(state.activeNpc.type, NpcTypeValid).name ?? 'null');
}),
```

`check(activeNpc.type, NpcTypeValid)` throws when the type id isn't registered. The `?? 'null'` at NPC_NAME is a **field-null** fallback — TS only returns `'null'` when the registered NpcType's `.name` field is null/undefined, not when the type itself is missing.

### 2.2 Current goscape behavior

`pkg/script/handlers_npc.go:242-263` (NPC_NAME) and `:305-321` (NPC_CATEGORY):

- Both check `requireActiveNpc` ✓
- NPC_NAME errors on `Configs == nil` with bespoke wording `"NPC_NAME: no configs"`
- NPC_CATEGORY silently pushes `-1` on `Configs == nil`
- Both silently push sentinel (`"null"` / `-1`) on `Configs.NpcType(...) == nil` (registry miss)

### 2.3 Divergence reachability

After the predecessor Shape A slice (`712a407a`), all script-side entry points that set `ActiveNpc.NpcType()` validate the type id:

- NPC_ADD (`handlers_npc.go:38`) ✓
- NPC_TYPE setter (`:190`) ✓
- NPC_CHANGETYPE (`:376`) ✓
- NPC_CHANGETYPE_KEEPALL (`:396`) ✓

Engine-side entry paths (mapdata spawns, pre-engine state) are not script-validated. Wiring NPC_NAME / NPC_CATEGORY surfaces engine-side broken types as script errors rather than silent sentinels — the correct behavior per TS.

### 2.4 Content dependency audit

Grep across `Content/scripts/**/*.rs2` for `npc_name` / `npc_category` returned 40+ callers, all bare reads of ActiveNpc (no unregistered-id arguments). Content cannot exercise the divergence. The silent-fallback path is reachable only via engine-spawn paths with broken type ids, which represent upstream bugs that should surface as script errors.

## 3. Scope

### 3.1 In scope

- `pkg/script/handlers_npc.go`:
  - `handleNpcName` (lines 239-263): rewrite guard block to canonical `requireActiveNpc` → `requireConfigs` → `checkNpcType` order; preserve `Name → DebugName → "null"` field cascade after registry validation.
  - `handleNpcCategory` (lines 303-321): rewrite guard block to canonical order; direct field access, no fallback.
  - Update both handler doc-comments to drop "falls back to" / "or -1" language and cite TS line numbers.

- `pkg/script/handlers_npc_test.go`:
  - Flip `TestNpcNameUnknownTypeReturnsNull` (line 640) to assert error.
  - Flip `TestNpcCategoryUnknownTypeReturnsMinusOne` (line 606) to assert error.
  - Optionally add nil-Configs error tests if sibling-test infra doesn't implicitly cover.

### 3.2 Out of scope

- NPC_DEL cached `Respawnrate` divergence (resume-memo item #2, separate XS audit / slice).
- NAI-162 analytics RPC (resume-memo item #3).
- Combat-level read-site verification (item #4).
- Other type-registry read-side wiring (LocType, ObjType — already verified clean in predecessor slice).

## 4. Design

### 4.1 Handler shape

Both handlers adopt the canonical four-guard pattern used by NPC_TYPE / NPC_CHANGETYPE / NPC_CHANGETYPE_KEEPALL:

1. `requireActiveNpc(s, "<OP>")` — existing first guard
2. `requireConfigs(s, "<OP>")` — replaces bespoke inline check
3. Extract `typeID := s.ActiveNpc.NpcType()` to a local
4. `checkNpcType(s, typeID, "<OP>")` — registry-presence validator
5. `cfg := s.Configs.NpcType(typeID)` — safe to dereference (`checkNpcType` ensures non-nil)
6. Field access and push

### 4.2 handleNpcName

Target body (in `pkg/script/handlers_npc.go`):

```go
// handleNpcName looks up the ActiveNpc's NpcType via Configs and pushes
// its Name, falling back to DebugName then "null" (matching TS
// nullish-coalesce on NpcType.name).
// Mirrors TS NpcOps.ts:270-272 — check(activeNpc.type, NpcTypeValid).
func handleNpcName(s *ScriptState) error {
    if err := requireActiveNpc(s, "NPC_NAME"); err != nil {
        return err
    }
    if err := requireConfigs(s, "NPC_NAME"); err != nil {
        return err
    }
    typeID := s.ActiveNpc.NpcType()
    if err := checkNpcType(s, typeID, "NPC_NAME"); err != nil {
        return err
    }
    cfg := s.Configs.NpcType(typeID)
    name := cfg.Name
    if name == "" {
        name = cfg.DebugName
    }
    if name == "" {
        name = "null"
    }
    s.PushString(name)
    return nil
}
```

### 4.3 handleNpcCategory

Target body:

```go
// handleNpcCategory looks up the ActiveNpc's NpcType via Configs and
// pushes its Category. Mirrors TS NpcOps.ts:68-70 —
// check(activeNpc.type, NpcTypeValid).category (no fallback).
func handleNpcCategory(s *ScriptState) error {
    if err := requireActiveNpc(s, "NPC_CATEGORY"); err != nil {
        return err
    }
    if err := requireConfigs(s, "NPC_CATEGORY"); err != nil {
        return err
    }
    typeID := s.ActiveNpc.NpcType()
    if err := checkNpcType(s, typeID, "NPC_CATEGORY"); err != nil {
        return err
    }
    s.PushInt(s.Configs.NpcType(typeID).Category)
    return nil
}
```

### 4.4 Guard placement rationale

Per predecessor close-memo insight #1: when wiring a new validator at a previously-validator-free handler, grep the corresponding `Test*` functions and verify guard placement doesn't preempt earlier-priority guard wording assertions.

Both flipped tests (`TestNpc*Unknown*`) assert NEW wording (the canonical `"OP: no NpcType with value (N) found"`), not pre-existing wording. No other tests assert error-priority at these handlers. Placement is unambiguous: `requireActiveNpc` first (TS `checkedHandler(ActiveNpc, ...)`), then `requireConfigs`, then `checkNpcType` — matches NPC_TYPE / NPC_CHANGETYPE precedent verbatim.

### 4.5 Local-var extraction

Current code calls `s.ActiveNpc.NpcType()` twice (NPC_NAME) and once (NPC_CATEGORY). The new shape extracts to `typeID` once and reuses for both `checkNpcType` and `Configs.NpcType(typeID)`. Quiet quality improvement mirroring predecessor close-memo finding #7 (OBJ_NAME / OBJ_PARAM).

## 5. Test plan

### 5.1 Unchanged tests

- `TestNpcName` (handlers_npc_test.go:615): registered `typeID=7`, `Name="Hans"` → `"Hans"`.
- `TestNpcNameFallsBackToDebugName` (`:630`): registered `typeID=1`, empty `Name`, `DebugName="unnamed_npc"` → `"unnamed_npc"`. **TS-faithful** — exercises the `?? 'null'` field-fallback after a successful registry check.
- `TestNpcCategory` (`:591`): registered `typeID=7`, `Category=99` → `99`.

### 5.2 Flipped tests

`TestNpcNameUnknownTypeReturnsNull` (`:640`) → `TestNpcName_UnknownType_ReturnsError`:

```go
func TestNpcName_UnknownType_ReturnsError(t *testing.T) {
    mc := newTestConfigs()
    npc := &mockNpc{typeID: 9999}
    _, err := runNpcOpErr(t, npc, mc, OpNpcName, nil)
    if err == nil {
        t.Fatal("NPC_NAME(unknown): expected error, got nil")
    }
    if !strings.Contains(err.Error(), "NPC_NAME: no NpcType with value (9999) found") {
        t.Errorf("NPC_NAME(unknown): err = %q, want substring %q",
            err.Error(), "NPC_NAME: no NpcType with value (9999) found")
    }
}
```

`TestNpcCategoryUnknownTypeReturnsMinusOne` (`:606`) → `TestNpcCategory_UnknownType_ReturnsError`:

```go
func TestNpcCategory_UnknownType_ReturnsError(t *testing.T) {
    mc := newTestConfigs()
    npc := &mockNpc{typeID: 9999}
    _, err := runNpcOpErr(t, npc, mc, OpNpcCategory, nil)
    if err == nil {
        t.Fatal("NPC_CATEGORY(unknown): expected error, got nil")
    }
    if !strings.Contains(err.Error(), "NPC_CATEGORY: no NpcType with value (9999) found") {
        t.Errorf("NPC_CATEGORY(unknown): err = %q, want substring %q",
            err.Error(), "NPC_CATEGORY: no NpcType with value (9999) found")
    }
}
```

Implementation note: if existing infra exposes only `runNpcOp` (non-error variant), the impl plan should introduce or use the error-returning equivalent (mirroring sibling NPC_TYPE error-path tests). To be confirmed at impl-plan time.

### 5.3 Optional additions (impl-time decision)

If `TestNpcType`-family or `TestNpcChangeType`-family tests don't already exercise the nil-Configs path for these op names, add:

- `TestNpcName_NilConfigs_ReturnsError` — assert `"NPC_NAME: no configs"`.
- `TestNpcCategory_NilConfigs_ReturnsError` — assert `"NPC_CATEGORY: no configs"`.

Both small, mirror NPC_TYPE / NPC_CHANGETYPE precedent if it exists. If sibling coverage is implicit / absent across the whole family, defer to a future coverage-gap slice.

## 6. Validation gates

- `GOROOT=/home/owner/go/go1.26.3 GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache /home/owner/go/go1.26.3/bin/go test -race ./...` — 0 FAIL across all packages.
- `TestPackAll_TwelveStageSmoke` PASS (cache pipeline smoke).
- `GOROOT=/home/owner/go/go1.26.3 GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache /home/owner/go/go1.26.3/bin/gofmt -l pkg/script/handlers_npc.go pkg/script/handlers_npc_test.go` — empty.
- Audit-grep deltas:
  - `grep -c 'checkNpcType(s, ' pkg/script/handlers_npc.go` → 8 → 10 (+2).
  - `grep -cE 'requireConfigs\(s, "(NPC_NAME|NPC_CATEGORY)"' pkg/script/handlers_npc.go` → 0 → 2 (+2).
  - `grep -n 'NPC_NAME: no configs' pkg/script/handlers_npc.go` → 1 → 0 (bespoke wording removed; canonical equivalent via `requireConfigs` substitutes).
  - In the `handleNpcCategory` body, the `s.PushInt(-1)` early-return silent-fallback path is removed (0 occurrences post-slice). In `handleNpcName`, the final `name = "null"` field-null fallback is preserved per TS `?? 'null'` — that is the ONLY remaining `"null"` literal in the handler.

## 7. TS-faithfulness checklist

- [x] NPC_NAME registry check via `checkNpcType` mirrors TS `check(activeNpc.type, NpcTypeValid)`.
- [x] NPC_NAME `?? 'null'` field-null fallback preserved (registered type with empty `Name`).
- [x] NPC_NAME goscape-only `DebugName` cascade preserved (layered on top of TS-faithful behavior; matches TS observable behavior when `Name` field is null/empty).
- [x] NPC_CATEGORY registry check via `checkNpcType` mirrors TS `check(activeNpc.type, NpcTypeValid)`.
- [x] NPC_CATEGORY no fallback (TS pushes `.category` directly).
- [x] Error wording matches sibling `checkNpcType` canonical: `"OP: no NpcType with value (N) found"`.

## 8. Risks

1. **Engine-side spawn paths producing unregistered ActiveNpc.NpcType()** — surfacing as error rather than silent sentinel is the desired behavior, but could expose pre-existing engine bugs. Mitigation: if discovered at runtime via Content scripts, file as separate engine-side bug, not a regression of this slice.

2. **`runNpcOp` test helper may swallow handler errors** — needs verification at impl-plan time. If so, introduce error-returning variant rather than amending the existing helper (avoid blast radius to other tests).

3. **NPC_DEL Respawnrate divergence still open** — explicitly out of scope. Carry-forward for separate XS audit/slice.

## 9. Cadence

- **Shape:** XS refactor-shaped slice (pre-existing validator + intra-file sibling precedent).
- **Dispatch:** sonnet-subagent implementer + sonnet 2-stage review (spec-conformance + code-quality) + opus whole-slice review.
- **Pre-conditions:** branch clean at `712a407a`; no pending working-tree changes apart from standing untracked noise.
- **Commits:** spec (this doc) → plan → impl.

## 10. Carry-forward menu (post-slice)

1. NPC_DEL cached Respawnrate vs registry divergence (XS audit, low priority — item #2 from predecessor).
2. NAI-162 analytics RPC.
3. Combat-level read-site verification.
4. Deviation audit refresh.
5. General world/runescript engine work.
6. OC_* Part B + most NC_* bespoke-unknown-id error test coverage gap (low priority).
