# S7c — BUILDAPPEARANCE + checkInvType Validator Design

> **Sub-spec context:** Twenty-ninth runescript sub-spec; third of the S7 series. Implements `BUILDAPPEARANCE` (opcode 2004), the next missing handler blocking `[proc,update_bas]` after S7b unblocked `[proc,update_all]` past `pc=66`. Smoke confirmed the VM now halts at `[proc,update_bas]` pc=63 with "no handler for BUILDAPPEARANCE".

> **TS-faithfulness gate:** Matches `LostCityRS/Engine-TS/src/engine/script/handlers/PlayerOps.ts:202-204`, `ScriptValidators.ts:122`, and `Player.ts:1836-1839`. One new deviation (S7c-D1: reader-side incompleteness); no other behavioral divergences.

> **Scope:** One opcode (BUILDAPPEARANCE 2004), one state-aware validator helper (`checkInvType`), one setter method (`SetAppearanceInv` — writes TWO fields per TS), one `ActivePlayer` interface method, `mockPlayer` extension with both a captured-value field and a "mask was set" bool, six test cases. ~30 LOC production + ~100 LOC tests. Compressed cadence per `compressed_cadence.md`: combined spec+plan, single review pass.

## 1. Goal

Implement `OpBuildAppearance` (2004) so `[proc,update_bas]` advances past `pc=63`. The opcode:

1. Requires an active player (TS `checkedHandler(ActivePlayer, ...)` — **not** `ProtectedActivePlayer`).
2. Pops one int.
3. Validates the int is a known `InvType` id via `Configs.InvType(id) != nil` (TS `InvTypeValid`).
4. Stores the id on the active player's `appearanceInv` field **AND** sets the `MaskAppearance` bit on `p.masks`. The mask flip triggers `generateAppearance` on the next tick (already wired at `tick.go:325-335`).

## 2. TS reference

- `src/engine/script/handlers/PlayerOps.ts:202-204` —
  ```ts
  [ScriptOpcode.BUILDAPPEARANCE]: checkedHandler(ActivePlayer, state => {
      state.activePlayer.buildAppearance(check(state.popInt(), InvTypeValid).id);
  })
  ```
  Gated on `ActivePlayer` (not Protected). `InvTypeValid` returns the looked-up `InvType`; `.id` unwraps back to the original int.
- `src/engine/script/ScriptValidators.ts:122` —
  ```ts
  export const InvTypeValid: ScriptValidator<number, InvType> =
      new ScriptInputConfigTypeValidator(InvType.get, (input) => input >= 0 && input < InvType.count, 'Inv');
  ```
  Validates `(input >= 0 && input < InvType.count)` AND `InvType.get(input) != null`. Both collapse to `Configs.InvType(id) != nil` in goscape per the `Configs` interface contract at `configs.go:7` ("return nil when the type isn't loaded or the id is out of range").
- `src/engine/entity/Player.ts:1836-1839` —
  ```ts
  buildAppearance(inv: number): void {
      this.appearanceInv = inv;
      this.masks |= PlayerInfoProt.APPEARANCE;
  }
  ```
  Literal two-liner: field assignment + mask bit.
- `src/engine/entity/Player.ts:1318` (the TS reader):
  ```ts
  let worn = this.getInventory(this.appearanceInv);
  ```
  Reads the field to decide which inventory feeds the appearance buffer. **Goscape's reader ignores this field** — see §7 deviation S7c-D1.
- `src/engine/entity/Player.ts:392` — field declaration `appearanceInv: number = -1;` (default -1, matches goscape's `player.go:314`).

## 3. Architecture

### 3.1 Validator helper (`pkg/script/handlers_player.go`)

State-aware helper — the only such helper in the file. Signature differs from siblings `checkStatID(id, op)` / `checkNotNull(v, op)` because range + existence are a single lookup against runtime-loaded config (unlike `NumStats = 21`, a compile-time const).

Insert after `checkNotNull` at line 66:

```go
// checkInvType mirrors TS InvTypeValid (ScriptValidators.ts:122) — a
// ScriptInputConfigTypeValidator over InvType. Both the range check
// (0 <= id < InvType.count) and the registry-present check collapse
// into "s.Configs.InvType(id) != nil" per the Configs interface contract
// at configs.go:7 ("return nil when the type isn't loaded or the id is
// out of range"). State-aware signature diverges from sibling check
// helpers because the bound is runtime-loaded.
func checkInvType(s *ScriptState, id int, op string) error {
    if s.Configs == nil || s.Configs.InvType(id) == nil {
        return fmt.Errorf("%s: no InvType with value (%d) found", op, id)
    }
    return nil
}
```

### 3.2 Handler (`pkg/script/handlers_player.go`)

Insert after `handlePAnimProtect` at line 82. Uses the existing `requireActivePlayer` gate (line 35), **not** the protected variant.

```go
// handleBuildAppearance (BUILDAPPEARANCE, opcode 2004) validates the popped
// InvType id and stages an appearance refresh on the active player. Mirrors
// TS PlayerOps.ts:202-204. Gate is ActivePlayer (not Protected). Validator
// mirrors TS InvTypeValid. The setter writes both Player.appearanceInv and
// flags MaskAppearance — MaskAppearance is consumed by tick.go:325-335 which
// regenerates the appearance buffer. Note: goscape's generateAppearance
// currently reads from invs.Worn rather than p.appearanceInv — tracked as
// S7c-D1.
func handleBuildAppearance(s *ScriptState) error {
    if err := requireActivePlayer(s, "BUILDAPPEARANCE"); err != nil {
        return err
    }
    id := s.PopInt()
    if err := checkInvType(s, id, "BUILDAPPEARANCE"); err != nil {
        return err
    }
    s.Self.SetAppearanceInv(id)
    return nil
}
```

### 3.3 Dispatch registration (`pkg/script/handlers.go`)

Insert at line 337 area, as its own S7c block after the S7b block:

```go
// S7c: BUILDAPPEARANCE dispatch.
OpBuildAppearance: handleBuildAppearance,
```

### 3.4 ActivePlayer interface (`pkg/script/active.go`)

Append after the existing S7b `SetAnimProtect` entry at line 321:

```go
// S7c: appearance refresh.

// SetAppearanceInv updates the active player's appearanceInv field AND
// flags MaskAppearance so the next tick regenerates the appearance buffer
// (tick.go:325-335). Mirrors TS Player.buildAppearance at
// Engine-TS/src/engine/entity/Player.ts:1836-1839 — both side-effects are
// required; tests assert both. Note: goscape's generateAppearance reads
// from invs.Worn rather than this field — deviation S7c-D1. Callers
// pre-validate id via checkInvType.
SetAppearanceInv(id int)
```

### 3.5 Concrete setter (`modules/world/player_script.go`)

Insert near the existing mask-flipping setters (around line 340 — between `PlayAnim` at :341 and `PlaySpotAnim` at :349 is a natural home; adjacent to other `p.masks |= rsbuf.MaskX` patterns):

```go
// SetAppearanceInv stores id on Player.appearanceInv and flags
// MaskAppearance. Mirrors TS Player.buildAppearance (the literal
// two-liner at Engine-TS/src/engine/entity/Player.ts:1836-1839). The
// mask triggers generateAppearance regeneration on the next tick in
// tick.go:325-335.
func (p *Player) SetAppearanceInv(id int) {
    p.appearanceInv = id
    p.masks |= rsbuf.MaskAppearance
}
```

No new imports needed — `rsbuf` is already imported at player_script.go top (used by the existing `MaskFaceCoord`, `MaskAnim`, `MaskSpotAnim` sites).

### 3.6 Player struct field (`modules/world/player.go`)

**No change required.** Field `appearanceInv int` already exists at line 123, initialized to `-1` at line 314.

## 4. File map

| File | Action | LOC |
|---|---|---|
| `pkg/script/handlers_player.go` | +`checkInvType` helper (§3.1), +`handleBuildAppearance` handler (§3.2) | +20 |
| `pkg/script/handlers.go` | Register `OpBuildAppearance: handleBuildAppearance` in a new S7c block | +2 |
| `pkg/script/active.go` | Add `SetAppearanceInv(id int)` to `ActivePlayer` interface | +5 |
| `modules/world/player_script.go` | `SetAppearanceInv` method writing both fields | +4 |
| `pkg/script/runner_test.go` | Extend `mockPlayer` with capture fields + setter method | +6 |
| `pkg/script/handlers_player_test.go` | +`TestCheckInvType` table test, +5 `handleBuildAppearance` tests | +110 |

**Production total:** ~31 LOC. **Test total:** ~110 LOC. Inside compressed-cadence upper band (15-100 LOC → combined spec+plan, single review).

## 5. Test plan

### 5.1 Validator table test (`handlers_player_test.go`)

Appended after S7b's `TestCheckNotNull`:

```go
func TestCheckInvType(t *testing.T) {
    tests := []struct {
        name    string
        id      int
        setup   func() *mockConfigs
        wantErr bool
    }{
        {
            name:    "valid id",
            id:      5,
            setup:   func() *mockConfigs { return &mockConfigs{invs: map[int]*objtype.InvType{5: {}}} },
            wantErr: false,
        },
        {
            name:    "unknown id",
            id:      100,
            setup:   func() *mockConfigs { return &mockConfigs{invs: map[int]*objtype.InvType{}} },
            wantErr: true,
        },
        {
            name:    "negative id",
            id:      -1,
            setup:   func() *mockConfigs { return &mockConfigs{invs: map[int]*objtype.InvType{}} },
            wantErr: true,
        },
        {
            name:    "nil Configs",
            id:      0,
            setup:   func() *mockConfigs { return nil },
            wantErr: true,
        },
    }
    for _, tc := range tests {
        t.Run(tc.name, func(t *testing.T) {
            s := &ScriptState{}
            if cfg := tc.setup(); cfg != nil {
                s.Configs = cfg
            }
            err := checkInvType(s, tc.id, "OP")
            // assert err presence + message contains "OP: no InvType with value"
            // when expected
        })
    }
}
```

Existing `mockConfigs` at `handlers_config_test.go:10-27` already has an `invs map[int]*objtype.InvType` field and an `InvType(id)` method that returns the map entry — directly reusable.

### 5.2 mockPlayer extension (`runner_test.go`)

Append to the `mockPlayer` struct near existing S7b `animProtectValue` capture (line ~200):

```go
// S7c: BUILDAPPEARANCE captures. appearanceInv is the last id passed to
// SetAppearanceInv; appearanceInvCalls counts invocations (0 verifies the
// setter was NOT reached for error paths); appearanceMaskSet tracks whether
// the setter flipped the mask side-effect (mockPlayer has no real masks
// field, so we capture intent as a bool).
lastAppearanceInv  int
appearanceInvCalls int
appearanceMaskSet  bool
```

Plus method:

```go
func (m *mockPlayer) SetAppearanceInv(id int) {
    m.lastAppearanceInv = id
    m.appearanceInvCalls++
    m.appearanceMaskSet = true
}
```

### 5.3 Handler tests (`handlers_player_test.go`)

Five cases after the S7b handlePAnimProtect tests (use `newSingleOp("buildappearance_*", OpBuildAppearance)` or equivalent — check the existing test harness; S7a/S7b used `newSingleOp(..., Op...)` with `handler = handle...`). Each asserts the expected `mockPlayer.appearanceInvCalls` count AND `mockPlayer.appearanceMaskSet` bool — this is the §3 two-side-effects guarantee.

1. **TestBuildAppearanceHappyPath** — `Self != nil`, `Configs.invs` has id=5, push 5 → no error, `lastAppearanceInv == 5`, `appearanceInvCalls == 1`, `appearanceMaskSet == true`.
2. **TestBuildAppearanceInvalidInvRejected** — `Self != nil`, `Configs.invs` empty, push 999 → error message contains `"BUILDAPPEARANCE: no InvType with value (999) found"`; `appearanceInvCalls == 0`, `appearanceMaskSet == false`.
3. **TestBuildAppearanceNegativeIdRejected** — `Self != nil`, `Configs.invs` empty, push -1 → error; setter not called. (Covers TS `input >= 0` branch via nil lookup since goscape collapses both checks.)
4. **TestBuildAppearanceNoActivePlayer** — `Self = nil` → error from `requireActivePlayer` chain (message contains `"BUILDAPPEARANCE"`); `appearanceInvCalls == 0`. Also verify the int stack was **not** popped — the gate runs before `PopInt` so the value should remain on the stack.
5. **TestBuildAppearanceNotProtectedOK** — `Protect = false`, `Self != nil`, `Configs.invs` has id=3, push 3 → **no error** (BUILDAPPEARANCE uses `ActivePlayer`, not `ProtectedActivePlayer`); `lastAppearanceInv == 3`, `appearanceMaskSet == true`. **Gate-regression guard**: if a future edit copy-pastes S7b's `requireProtectedActivePlayer` here, this test catches it.

Test #5 is the explicit codification of the first wrinkle the user flagged (`checkedHandler(ActivePlayer, ...)` ≠ `ProtectedActivePlayer`).

### 5.4 Script-VM integration (no new test required)

`[proc,update_bas]` running through to completion is the implicit integration test — covered by post-merge smoke retest, not a unit test in this sub-spec.

## 6. Task split

**Single task.** ~31 LOC production + ~110 LOC tests across 6 files. No external deps, no migrations, no fixture churn.

Commit messages (close-commit format per `close_commit_memory_trailer.md`):

1. `feat(script): handleBuildAppearance + checkInvType validator (S7c)` — production code + new tests, all in one commit since the diff is small and the pieces are mutually dependent (handler ↔ validator ↔ setter ↔ interface ↔ mock).
2. `chore(script): S7c closed — BUILDAPPEARANCE + checkInvType` — close-commit. Trailers:
   - `Closes memory: nai_followups.md (BUILDAPPEARANCE unblock)` — if `nai_followups.md` holds an entry for this opcode (implementer must grep at start of task).
   - `Smoke: update_bas advances past pc=63`.

Two commits total. Acceptable to bundle into one if the implementer prefers, since ~140 total LOC is small.

## 7. Deviations

| ID | Status | Description |
|---|---|---|
| **S7a-D1** | Pre-existing | Collapsed pointer model. Carried. |
| **S7a-D2** | Pre-existing | `Player.uid` source-of-truth unwired. Carried. |
| **S7b-D1** | Pre-existing | `Player.animProtect` set but unread. Carried. |
| **S7c-D1** | **NEW** | `Player.appearanceInv` field is **set** by `BUILDAPPEARANCE` but **ignored** by the reader (`modules/world/appearance.go:27` reads `p.invs[invs.Worn]` — the global `Worn` InvType id — rather than TS `this.getInventory(this.appearanceInv)` at `Player.ts:1318`). Observable effect: scripts that call `buildAppearance(<non-worn-id>)` will not see their custom inv's items on the wire; the appearance buffer always regenerates from the worn inv. Acceptable for `update_bas` / `update_all` which pass the player's current worn-inv id (observationally indistinguishable from `invs.Worn`). Will surface when custom-outfit or disguise scripts land. **Follow-up tracked in `nai_followups.md`** at close time. |

**Closures:** None carried by this sub-spec beyond optional `nai_followups.md` pruning if a BUILDAPPEARANCE-gap entry exists there.

## 8. Follow-ups

- **Port TS appearance.go:27 reader** — change `worn = p.invs[invs.Worn]` to `worn = p.invs[p.appearanceInv]` (with a sensible default when `appearanceInv == -1`, matching TS `Player.getInventory` sentinel handling). Closes S7c-D1. Likely a standalone sub-spec because it needs smoke-test coverage (confirm `update_bas` still looks right when the field is passed through; validate with a non-worn-inv caller once such a script appears).
- **Audit other `ScriptInputConfigTypeValidator` sites in TS** — `InvTypeValid` is one of a family (`CategoryTypeValid`, `IDKTypeValid`, `SeqTypeValid`, `VarPlayerValid`, etc. — see `ScriptValidators.ts:121-128`). Opportunistic: if other handlers port and need the same `check<XType>` shape, extract a generic `checkConfigType[T any](s, id, op, lookup)` — but not ahead of the second consumer (YAGNI, `dead_api_polish.md`).
- **`require2` / secondary-active-player gating** — `ScriptOpcodePointers.ts` may declare BUILDAPPEARANCE also takes a `require2` form (S7a-D1 territory). Not blocking; no known caller in `update_all` / `update_bas`.

## 9. Self-review notes

- **Placeholders:** None. All identifier names and line numbers concrete.
- **Internal consistency:** §3.2 handler ↔ §5.3 tests 1:1 (happy/invalid/negative/no-active/unprotected). §3.1 validator ↔ §5.1 tests 1:1 (valid/unknown/negative/nil). §3.5 setter ↔ §5.2 mockPlayer captures both side-effects. §4 file map sums match §3 + §5. §7 S7c-D1 cross-references §3.5's two-field write + the `generateAppearance` reader site.
- **Scope:** Single task, single sub-spec. Compressed cadence — no separate plan doc.
- **Ambiguity:** (1) validator signature — resolved: state-aware because the bound is runtime-loaded. (2) mockPlayer mask tracking — resolved: bool flag captures the TS two-side-effects guarantee without porting real mask semantics. (3) is this write-only-mask deviation? — resolved: **no** (MaskAppearance fully consumed at tick.go:325-335 + mask_payload.go:40 + renderer.go:46,52), but field-reader IS incomplete → S7c-D1.
- **Test-coverage crosscheck** (per `plan_test_coverage_crosscheck.md` memory): every §3 branch has a §5 test. Handler: happy, bad-input, negative-input, no-active-player, unprotected-ok. Validator: valid, unknown, negative, nil-configs. Setter: both side-effects asserted in happy path + negative assertion in error paths.
- **Plan-helper coverage** (per `plan_helper_coverage.md` memory): new `checkInvType` helper has exactly one consumer (`handleBuildAppearance`). Every branch of that consumer is tested. No hidden helper flag-sets.
- **Enumerate-all-sites** (per `enumerate_all_sites.md` memory): grep confirmed `SetAppearanceInv` is new (no existing callers to update); `appearanceInv` field exists only at player.go:123 + init at :314 (neither consumers of a "set via opcode" path); no existing test fixtures use `appearanceInv = -1` beyond the zero-value default, which remains compatible because our setter never special-cases -1.
- **true-to-TS gate** (per `true_to_ts_gate.md` memory): every behavioral divergence from TS is tracked as a deviation with rationale + follow-up. S7c-D1 is the sole new deviation.
- **Close-commit trailer** (per `close_commit_memory_trailer.md` memory): §6 specifies `Closes memory:` and `Smoke:` trailers for the close commit.
