# handlers_inv.go read-side `checkInvType` wiring — design

**Date:** 2026-05-21
**Status:** Draft
**Builds on:** [[registry-presence-validators-wiring-close]] (slice 4, validator-wiring pattern); intra-file sibling precedent at `handleInvAdd:344-371`
**Surfaced by:** [[inv-type-opcode-wiring-audit-phantom-gap]] (the InvType "audit other files" carry-forward menu item was empty; this gap was uncovered by the audit motion itself)

## 1. Goal

Wire `checkInvType` at every read-side and inline-registry handler in `pkg/script/handlers_inv.go` that currently checks InvType identity via either:

- **Shape A:** `resolveInv(s, typeID) == nil` only (no registry check, just player-container check) — 9 sites.
- **Shape B:** inline `s.Configs.InvType(id) == nil` with bespoke error wording — 3 sites.

After this slice, every script-input InvType id in `handlers_inv.go` flows through the canonical `checkInvType` validator first (mirroring TS `check(inv, InvTypeValid)` from `InvOps.ts`), with consistent `"%s: no InvType with value (%d) found"` error wording on registry miss.

## 2. TS upstream

TS `InvOps.ts` uses the pattern:

```ts
const invType: InvType = check(state.popInt(), InvTypeValid);
// then access state.activePlayer.invXxx(invType.id, ...)
```

at every read-side opcode (INV_TOTAL `:619`, INV_SIZE `:27`, INV_GETOBJ `:278`, INV_GETNUM `:270`, INV_FREESPACE `:263`, INV_ITEMSPACE `:286`, INV_ITEMSPACE2 `:306`, INV_TOTALPARAM `:786`, INV_TOTALCAT `:634`) and at every write-side opcode (including the 2 Shape B handlers in scope). `InvTypeValid` is defined at `ScriptValidators.ts:122` as a `ScriptInputConfigTypeValidator` over `InvType` — identical shape to NpcTypeValid/ObjTypeValid wired by [[registry-presence-validators-wiring-close]].

The registry check (`check(inv, InvTypeValid)`) and the player-container access (`state.activePlayer.invXxx(...)`) are two distinct stages in TS. The TS player-container access doesn't have an explicit nil-guard because TS lazy-allocates containers via `getInventory`.

## 3. Goscape current state

### 3.1 Sibling precedent — `handleInvAdd:344-371`

```go
// handlers_inv.go:344-371
if err := checkInvType(s, typeID, op); err != nil {
    return err
}
// ... other validators (ObjTypeValid, ObjStackValid, etc.) ...
invType := s.Configs.InvType(typeID)  // local var for downstream field access
// ... protect/scope gate ...
inv := resolveInv(s, typeID)
if inv == nil {
    // Defensive: unreachable post-checkInvType for valid configs;
    // retained for the InvLookup-unset case (s.Inv == nil → resolveInv returns nil).
    return fmt.Errorf("%s: no inv for type %d", op, typeID)
}
```

This is the EXACT pattern this slice extends to the 9 read-side handlers + 2 inline-registry handlers. Same canonical wording, same defensive nil-fallthrough comment.

### 3.2 Validator — `checkInvType`

Defined at `pkg/script/handlers_player.go:158-170`. State-aware signature `checkInvType(s *ScriptState, id int, op string) error`. Canonical wording `"%s: no InvType with value (%d) found"`. Pre-existing test at `handlers_player_test.go:2364 TestCheckInvType` covers registry-miss rejection.

### 3.3 Configs interface contract

`pkg/script/configs.go:7`: `Configs.InvType(id)` returns nil when the type isn't loaded or the id is out of range. The `< InvType.count` bound collapses with registry-presence per [[registry-presence-validators-wiring-close]] §1.2.

## 4. In-scope sites

### 4.1 Shape A — read-side `resolveInv`-only (9 sites)

All in `pkg/script/handlers_inv.go`:

| # | Handler                  | Line  | Opcode               | TS reference              |
|---|--------------------------|-------|----------------------|---------------------------|
| 1 | `handleInvTotal`         | `:26` | INV_TOTAL            | `InvOps.ts:619`           |
| 2 | `handleInvGetObj`        | `:44` | INV_GETOBJ           | `InvOps.ts:278`           |
| 3 | `handleInvGetNum`        | `:62` | INV_GETNUM           | `InvOps.ts:270`           |
| 4 | `handleInvSize`          | `:79` | INV_SIZE             | `InvOps.ts:27`            |
| 5 | `handleInvFreeSpace`     | `:91` | INV_FREESPACE        | `InvOps.ts:263`           |
| 6 | `handleInvItemSpace`     | `:175`| INV_ITEMSPACE        | `InvOps.ts:286`           |
| 7 | `handleInvItemSpace2`    | `:202`| INV_ITEMSPACE2       | `InvOps.ts:306`           |
| 8 | `handleInvTotalParam`    | `:224`| INV_TOTALPARAM       | `InvOps.ts:786`           |
| 9 | `handleInvTotalCat`      | `:261`| INV_TOTALCAT         | `InvOps.ts:634`           |

**Wiring pattern (per site):**

```go
// BEFORE:
func handleInvTotal(s *ScriptState) error {
    obj := s.PopInt()
    typeID := s.PopInt()
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

// AFTER:
func handleInvTotal(s *ScriptState) error {
    obj := s.PopInt()
    typeID := s.PopInt()
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

The pop ordering for INV_TOTAL has a TS-faithful `obj == -1` short-circuit BEFORE the InvType check; this is preserved exactly. (TS pops `[inv, obj]` and the short-circuit is on obj; the inv is unused when obj == -1, so it's TS-faithful not to invoke InvType validation in that path.) Verify against TS at impl time.

Note: only ONE handler (`handleInvTotal`) has a pre-validator short-circuit. The other 8 have no such structure; `checkInvType` goes immediately after `PopInt`s.

**Defensive doc-comment retention:** the `// Defensive: unreachable post-checkInvType ...` comment is copied verbatim per site, matching the `handleInvAdd:369-370` precedent exactly. This is the only comment addition per site.

### 4.2 Shape B — inline registry-check canonicalization (3 sites)

| #  | Handler              | Line    | Current bespoke wording                      |
|----|----------------------|---------|-----------------------------------------------|
| 10 | `handleInvDropSlot`  | `:795`  | `"INV_DROPSLOT: invalid inv id (%d)"`        |
| 11 | `handleBothDropSlot` | `:1658` | `"BOTH_DROPSLOT: invalid inv id (%d)"`       |
| 12 | `handleInvDropAll`   | `:1801` | `"INV_DROPALL: invalid inv id (%d)"`         |

**Wiring pattern (per site):**

```go
// BEFORE (handleInvDropSlot:795-798):
invType := s.Configs.InvType(invID)
if invType == nil {
    return fmt.Errorf("INV_DROPSLOT: invalid inv id (%d)", invID)
}

// AFTER:
if err := checkInvType(s, invID, "INV_DROPSLOT"); err != nil {
    return err
}
invType := s.Configs.InvType(invID)  // preserved for downstream field access
```

All three handlers access `invType.Protect` / `invType.Scope` downstream (DROPSLOT at `:812-820`, BOTH_DROPSLOT at the protect/scope gate further down the function, DROPALL at `:1818-1828`), so the local var must be preserved per the "preserve local var" rule from [[registry-presence-validators-wiring-close]] §5.4. None qualifies for OBJ_FIND's "no local var" exception (which applied only because OBJ_FIND used the id solely for downstream lookup).

## 5. Out of scope

### 5.1 `handleInvTotalParamStack` (`:1922`)

Delegates entirely to `s.Self.InvTotalParamStack(inv, param)`. The handler has no inline InvType access; any validation belongs in the player impl. Out of scope for this slice; consider as a separate audit if the player impl also lacks the registry check.

### 5.2 INV_SIZE push-semantics divergence

TS `INV_SIZE` pushes `invType.size` (the registry-configured size). Go pushes `inv.Capacity` (the player container's capacity). These are likely byte-identical at runtime (Inventory allocator copies `InvType.Size` into `Inventory.Capacity`), but this slice does NOT alter the push semantics. If a future slice wants to switch Go to push `invType.Size` for TS-strict fidelity, it can — out of scope here.

### 5.3 `invItemSpaceRemaining` helper (`:112-167`)

This is a private helper invoked by INV_ITEMSPACE / INV_ITEMSPACE2 / INV_ADD overflow logic. Its `s.Configs.InvType(inv.Type)` access at `:134` is on the *already-resolved* `inv.Type` field of an `*inventory.Inventory` pointer — not a script-input. Not in scope.

### 5.4 Other type-registry read-side gaps

Pattern recurrence risk noted in [[inv-type-opcode-wiring-audit-phantom-gap]] non-obvious finding #3 ("validator-vs-resolver boundary"): LocType/NpcType/ObjType read-side opcodes may have analogous gaps. Out of scope here; future audit slice candidate.

## 6. Tests

### 6.1 Existing assertions

`pkg/script/handlers_inv_test.go:538` — `TestInvLookupNilReturnsError`:

```go
runInvOpExpectErr(t, OpInvTotal, []int{testInvMain, testObjCoin}, nil, mc, "no inv for type")
```

This test passes `testInvMain` (REGISTERED in the mock at `newTestInvConfigs()`) with `lookup=nil`. Under Approach B:
- `checkInvType` succeeds (registered InvType).
- `resolveInv` returns nil (`s.Inv == nil` because lookup is nil).
- Defensive fallthrough fires: `"INV_TOTAL: no inv for type %d"`.

**This assertion stays unchanged.** ✓

### 6.2 New tests

**None required.** Per [[registry-presence-validators-wiring-close]] non-obvious finding #3, validator-layer coverage at `TestCheckInvType` (`handlers_player_test.go:2364`) is sufficient — every wired site flows through the same validator, so handler-layer "unknown id" tests would be redundant.

### 6.3 Impl-time audit-grep

Before commit, grep `pkg/script/handlers_inv_test.go` for:
- `"invalid inv id"` — bespoke DROPSLOT/DROPALL wording. If any test asserts this, flip to `"no InvType with value"`. Expected hits: 0 or low single digits.
- `"no inv for type"` outside the `TestInvLookupNilReturnsError` defensive-path test. If any test asserts this for a handler in Shape A scope WITH an unregistered InvType id, flip to `"no InvType with value"`.

Cite findings in the implementation plan's audit table.

## 7. Risk / rollback

- **Behavior change:** error wording changes for registry-miss paths (`"invalid inv id"` → `"no InvType with value"`). Container-miss wording (`"no inv for type"`) is preserved on the defensive path. No runtime semantic change beyond error-string content.
- **Test impact:** known to be minimal (1 existing test stays, 0 new tests expected, possible 1-2 bespoke-wording flips for DROPSLOT/DROPALL).
- **Rollback:** trivial — `git revert` of the impl commit.

## 8. Gates

- `gofmt -l` clean on every edited file.
- `go test -race ./...` 0 FAIL.
- `go test -run TestPackAll_TwelveStageSmoke` PASS.
- Audit-greps (deltas vs HEAD, not absolute counts — many pre-existing wired sites already have the defensive `"no inv for type"` fallthrough):
- Audit-grep baseline counts at HEAD `ce918de2`:
  - `grep -c "checkInvType(s, " pkg/script/handlers_inv.go` → **23**
  - `grep -c "no inv for type" pkg/script/handlers_inv.go` → **15**
  - `grep -c "invalid inv id" pkg/script/handlers_inv.go` → **3** (the 3 Shape B sites)
- Audit-grep expected post-slice (deltas vs baseline):
  - `grep -c "checkInvType(s, " pkg/script/handlers_inv.go` → expect **35** (+12; 9 new Shape A calls + 3 new Shape B calls replacing inline checks).
  - `grep -c "no inv for type" pkg/script/handlers_inv.go` → expect **15** (no change; Shape A wires preserve the existing `"no inv for type"` line as the defensive fallthrough, they don't add a new one; Shape B wires don't touch "no inv for type" lines).
  - `grep -c "invalid inv id" pkg/script/handlers_inv.go` → expect **0** (−3; all Shape B bespoke wording canonicalized via `checkInvType`).

## 9. Cadence

Single sonnet implementer + two-stage review (sonnet spec-conformance + sonnet code-quality). Single task, single commit. Pre-existing intra-file sibling precedent (`handleInvAdd`) + pre-existing validator (`checkInvType`) + pre-existing validator-layer test (`TestCheckInvType`) + minimal test churn justifies lighter cadence than [[registry-presence-validators-wiring-close]]'s 4-task split.

Implementation plan written separately via `writing-plans` skill.

## 10. Carry-forward delta

Net carry-forward menu after this slice:
- −1 item retired (handlers_inv.go read-side `checkInvType` wiring).
- 0 new items (no surfacing expected — sibling-precedent close).

Remaining menu (post-slice): NAI-162 analytics RPC, combat-level read-site verification, deviation audit refresh, general world/runescript, OC_* Part B + most NC_* bespoke-unknown-id test coverage gap, **other type-registry validator-vs-resolver gaps** (potential future audit if §5.4 pattern recurs at LocType/NpcType/ObjType read-side).
