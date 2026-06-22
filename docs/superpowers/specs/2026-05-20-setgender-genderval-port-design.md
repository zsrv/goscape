# SETGENDER body port + GenderValid validator

Author: zsrv + Claude (Opus 4.7)
Date: 2026-05-20
Status: Approved
Predecessor: [Queue + SkinColour validator port](2026-05-20-queue-skincolour-validator-design.md) — §9 explicitly named `GenderValid` as out-of-scope "blocked by handleSetGender stub NAI-162-D-STUB-SETGENDER even though TS PlayerOps.ts:1104-1118 has real body".

---

## 1. Goal

Replace the `handleSetGender` TS-unimplemented stub with a faithful port of TS `PlayerOps.ts:1104-1118`, and add the sibling `checkGender(v int, op string) error` validator (TS `GenderValid`, inclusive `[0, 1]`).

The port rewrites the active player's 7-slot `body[]` idkit array via two class-level lookup maps (`MALE_FEMALE_MAP`, `FEMALE_MALE_MAP`), applies a special-case slot-1 hardcode on female→male direction, and writes the gender field. It does NOT flip `MaskAppearance` — TS-faithful deferred-rebuild pattern: real content (`makeover_mage.rs2:58-64`) explicitly follows `setgender` + `setskincolour` with `buildappearance(worn)`.

This retires `NAI-162-D-STUB-SETGENDER` and opens `NAI-SETGENDER-D-TS-UNMAPPED-IDKIT-INHERITED` (TS-literal `Map.get() ?? -1` writes -1 garbage idkit on unmapped keys; pinned for future TS sync).

## 2. Why now

Natural successor to the just-closed Queue + SkinColour validator slice — adds the third sibling of the `ScriptInputRangeValidator` family (`QueueValid`/`SkinColourValid`/`GenderValid`), all touching adjacent opcodes in TS `PlayerOps.ts` (SETSKINCOLOUR at 1121-1124, SETGENDER at 1104-1118, SETIDKIT around 1073). The predecessor's §9 explicitly named this as the natural follow-up.

Closing the SETGENDER stub also retires one of the long-standing `NAI-162-D-STUB-*` pins (one of six stubs declared at `handlers_b0_stubs.go`), incrementally cleaning the deviation board.

Slice is bounded: one validator + two const maps + one setter + one handler unstub + retire one pin + open one pin + ~13 tests.

## 3. TS reference

### 3.1 Validator

```ts
// Engine-TS/src/engine/script/ScriptValidators.ts:136
export const GenderValid: ScriptValidator<number, number>
    = new ScriptInputRangeValidator(0, 1, 'Gender');
```

`ScriptInputRangeValidator(min, max, name)` is INCLUSIVE on both bounds (validator body at `ScriptValidators.ts:75-90` is `input >= min && input <= max`). Per `[[hit-type-validator-slice-close]]` non-obvious finding #1, this is the inclusive max NOT exclusive count.

### 3.2 Handler body

```ts
// Engine-TS/src/engine/script/handlers/PlayerOps.ts:1104-1119
[ScriptOpcode.SETGENDER]: state => {
    const gender = check(state.popInt(), GenderValid);
    // convert idkit, have to use a mapping cause order + there's not always an equivalence
    for (let i = 0; i < 7; i++) {
        if (gender === 1) {
            state.activePlayer.body[i] = Player.MALE_FEMALE_MAP.get(state.activePlayer.body[i]) ?? -1;
        } else {
            if (i === 1) {
                state.activePlayer.body[i] = 14;
                continue;
            }
            state.activePlayer.body[i] = Player.FEMALE_MALE_MAP.get(state.activePlayer.body[i]) ?? -1;
        }
    }
    state.activePlayer.gender = gender;
},
```

### 3.3 Lookup maps

```ts
// Engine-TS/src/engine/entity/Player.ts:110-148
static readonly MALE_FEMALE_MAP = new Map<number, number>([
    [0, 45], [1, 47], [2, 48], [3, 49], [4, 50], [5, 51], [6, 52], [7, 53], [8, 54], [9, 55],
    [18, 56], [19, 56], [20, 56], [21, 56], [22, 56], [23, 56], [24, 56], [25, 56],
    [26, 61], [27, 63], [28, 62], [29, 65], [30, 64], [31, 63], [32, 66], [33, 67],
    [34, 68], [35, 69], [36, 70], [37, 71], [38, 72], [39, 76], [40, 75], [41, 78],
    [42, 79], [43, 80], [44, 81]
]);

// Engine-TS/src/engine/entity/Player.ts:150-188
static readonly FEMALE_MALE_MAP = new Map<number, number>([
    [45, 0], [46, 0], [47, 1], [48, 2], [49, 3], [50, 4], [51, 5], [52, 6], [53, 7], [54, 8], [55, 9],
    [56, 18], [57, 18], [58, 18], [59, 18], [60, 18],
    [61, 26], [62, 27], [63, 28], [64, 29], [65, 29], [66, 32], [67, 33], [68, 34], [69, 35],
    [70, 36], [71, 37], [72, 38], [73, 36], [74, 36], [75, 40], [76, 39], [77, 36],
    [78, 41], [79, 42], [80, 43], [81, 44]
]);
```

## 4. Audit findings

### 4.1 Content bytecode audit

Greppable `setgender` invocations in `$HOME/Code/github.com/LostCityRS/Content/scripts/`:

- **Exactly one production callsite**: `areas/area_falador/scripts/makeover_mage.rs2:58` — `setgender(calc(%if1 - 1));`
- Surrounding logic (`if_button` cases at lines 34-41) sets `%if1 ∈ {1, 2}`, so the value passed to `setgender` is always `{0, 1}` (no garbage inputs reach this opcode in production content).
- The same script demonstrates the canonical TS-pattern explicitly:
  ```
  setgender(calc(%if1 - 1));       // line 58
  setskincolour(calc(%if2 - 1));   // line 61
  buildappearance(worn);            // line 64
  ```
  This is the **explicit confirmation** that SETGENDER + SETSKINCOLOUR are *appearance-mutation* opcodes that must NOT flip `MaskAppearance` — the BUILDAPPEARANCE call at line 64 is the explicit rebuild trigger. Mirrors the established `SetBodyPart` precedent at `pkg/script/active.go:733-736`.

### 4.2 Map round-trip audit

Neither direction is a full bijection — this is canonical OSRS (the makeover-mage isn't fully reversible).

**M→F→M lossy cases (selection):**
- Males `{18..25}` all → female `56`, then back to male `18` (canonical, others lost)
- Males `{27, 31}` both → female `63`, then back to male `28`

**F→M→F lossy cases (selection):**
- Females `{45, 46}` both → male `0`, then → female `45`
- Females `{73, 74, 77}` → male `36`, then → female `70`

**Slot-1 hardcode (PlayerOps.ts:1111-1113):**
On female→male direction, `body[1]` is forced to `14`, overriding any `femaleMaleMap` lookup. This is deliberate TS canon for the canonical male hair model — not a bug.

**Implication:** mirror behavior verbatim. Do not introduce identity preservation or no-change guards.

### 4.3 -1 garbage behavior (deviation pin material)

TS `Map.get(k) ?? -1` writes `-1` to `body[i]` when the current value is not present in the relevant lookup map. Real content cannot reach this case because:
- Players' body idkit values originate from character-design flows that constrain to mapped values, OR
- Players' body values came from a prior same-direction `setgender` call which itself wrote mapped values.

However, this is a latent TS issue that should be pinned for future TS sync — open `NAI-SETGENDER-D-TS-UNMAPPED-IDKIT-INHERITED` on `(*Player).SetGender` doc comment, pinned by a test `TestPlayerSetGender_UnmappedKeysWriteMinusOne`.

## 5. Design

### 5.1 Slice scope summary

| Item | Location | Action |
|---|---|---|
| `checkGender(v int, op string) error` | `pkg/script/handlers_player.go` (after `checkSkinColour` ~L99) | Add new |
| `maleFemaleMap` (var) | `modules/world/player_gender.go` (new file) | Add new |
| `femaleMaleMap` (var) | `modules/world/player_gender.go` | Add new |
| `lookupGenderIdkit(m, k) int` helper | `modules/world/player_gender.go` | Add new |
| `(*Player).SetGender(gender int)` | `modules/world/player_gender.go` | Add new |
| `ActivePlayer.SetGender(int)` method | `pkg/script/active.go` (near `Gender() int` ~L704) | Add new |
| `mockPlayer.SetGender` + capture | `pkg/script/runner_test.go` (near `SetBodyPart` ~L772) | Add new |
| `handleSetGender` (real impl) | `pkg/script/handlers_player.go` (after `handleSetSkinColour` ~L1678) | Add real |
| `handleSetGender` (stub) | `pkg/script/handlers_b0_stubs.go` L21-25 | Delete |
| `SET_GENDER` stub-table row | `pkg/script/handlers_b0_stubs_test.go` L20 | Delete |
| `NAI-162-D-STUB-SETGENDER` pin | doc comment at stub | Retired (deleted with stub) |
| `NAI-SETGENDER-D-TS-UNMAPPED-IDKIT-INHERITED` pin | `SetGender` doc comment | Opened |

### 5.2 Out of scope

- `GenderValid` → other ScriptValidators (the registry-cluster family `InvType`/`NpcType`/etc. — next-up family for a separate slice per predecessor §9)
- Behavioral fix for `MaskAppearance` (would be TS-deviating; pinned by `TestPlayerSetGender_DoesNotFlipMaskAppearance`)
- Compiler-side work (rs2-compiler already emits opcode 2099 unchanged)
- E2E/integration test of SETGENDER + BUILDAPPEARANCE wire sequence (out of scope; unit coverage is sufficient given single content callsite)

### 5.3 Faithful-port assertions

1. **No `MaskAppearance` flip** — pinned by `TestPlayerSetGender_DoesNotFlipMaskAppearance`. Justified by TS source + `makeover_mage.rs2:58-64` content evidence.
2. **Slot-1 hardcode to 14 on female→male** — pinned by `TestPlayerSetGender_FemaleToMale_Slot1HardcodedTo14`. Cross-ref `PlayerOps.ts:1111-1113`.
3. **Unmapped key → -1** — pinned by `TestPlayerSetGender_UnmappedKeysWriteMinusOne` (the new deviation pin).
4. **Identity loop even when gender unchanged** — TS-faithful (TS doesn't guard on `gender === state.activePlayer.gender`). No optimization. Documented inline.
5. **Direction-aware lossiness preserved** — `TestPlayerSetGender_LossyCollapse` documents 19→56→18 collapse explicitly.

### 5.4 Defensive gate posture

The handler retains the goscape-defensive `requireActivePlayer(s, "SETGENDER")` guard following sibling `handleSetSkinColour` shape. TS skips this check (it relies on caller-provided `state.activePlayer` non-null guarantee); goscape adds the guard via the `defensive_gate_doc_comment_label` convention. Doc-comment cross-refs this convention.

### 5.5 Validator placement

`checkGender` lives in `pkg/script/handlers_player.go` alongside `checkSkinColour` and `checkNotNull`. It is a free function (not a method on `ScriptState`), matching the sibling `checkSkinColour` / `checkQueue` / `checkHitType` shape.

### 5.6 Lookup map data structure

`map[int]int` package-level vars, direct mirror of TS `Map<number, number>`. Sparse representation preserves the visual signal of which keys are mapped vs unmapped. Helper `lookupGenderIdkit(m, k) int` returns `-1` for missing keys, mirroring TS `Map.get(k) ?? -1`.

### 5.7 Seam shape

Thin handler, fat setter:
- Handler does popInt + checkGender + dispatch
- `(*Player).SetGender` does the loop + lookups + slot-1 hardcode + final field write
- Mirrors `handleSetSkinColour` → `(*Player).SetColorPart(4, skin)` shape

This places the TS class-level constants (`Player.MALE_FEMALE_MAP`, `Player.FEMALE_MALE_MAP`) alongside the `Player` struct in `modules/world/`, matching their TS conceptual home.

## 6. File layout

**New files (1):**
- `modules/world/player_gender.go` — maps + helper + `(*Player).SetGender`. Carries `NAI-SETGENDER-D-TS-UNMAPPED-IDKIT-INHERITED` pin in doc comment.

**Modified files (production, 4):**
- `pkg/script/handlers_player.go` — add `checkGender`, add real `handleSetGender`
- `pkg/script/handlers_b0_stubs.go` — delete stub `handleSetGender`
- `pkg/script/active.go` — add `SetGender(int)` to `ActivePlayer` interface
- `pkg/script/runner_test.go` — add `mockPlayer.SetGender` + `setGenderCalls` capture (test-support file, but consumed by all `pkg/script` tests; treated as production seam by convention)

**New test file (1):**
- `modules/world/player_gender_test.go` — 6 `TestPlayerSetGender_*` tests

**Modified test files (2):**
- `pkg/script/handlers_player_test.go` — add 2 `TestCheckGender_*` + 4 `TestHandleSetGender_*` tests
- `pkg/script/handlers_b0_stubs_test.go` — delete `SET_GENDER` table row

## 7. Test plan

### 7.1 Validator tests (pkg/script)

1. `TestCheckGender_AcceptsValid` — `{0, 1}` both return nil
2. `TestCheckGender_RejectsOutOfRange` — table-driven: `-1, 2, math.MinInt, math.MaxInt` — each returns non-nil error containing `"out of range"` and `"SETGENDER"`

### 7.2 Handler tests (pkg/script)

3. `TestHandleSetGender_RequiresActivePlayer` — no active player → returns active-player-guard error; `setGenderCalls` empty
4. `TestHandleSetGender_RejectsOutOfRange` — push `2`, run handler, assert error contains `"SETGENDER"` and `"out of range"`; `setGenderCalls` empty
5. `TestHandleSetGender_DispatchesToSetter` — push `1`, run handler, assert `setGenderCalls == [1]` + stack empty
6. `TestHandleSetGender_AcceptsZeroEdge` — push `0`, run handler, assert `setGenderCalls == [0]` (explicit boundary pin, mirrors predecessor pattern)

### 7.3 Setter tests (modules/world)

7. `TestPlayerSetGender_MaleToFemale_RewritesAllSlotsViaMap` — body `[0..6]` + `SetGender(1)` → body `[45, 47, 48, 49, 50, 51, 52]` + `gender == 1`
8. `TestPlayerSetGender_FemaleToMale_Slot1HardcodedTo14` — body `[45, 47, 48, 49, 50, 51, 52]` + `SetGender(0)` → `body[1] == 14`; other slots = `femaleMaleMap` lookup (`{0, 2, 3, 4, 5, 6}`); `gender == 0`
9. `TestPlayerSetGender_FemaleToMale_NonSlot1UsesMap` — focused sub-case verifying slots `{0, 2, 3, 4, 5, 6}` go via `femaleMaleMap` (e.g., body[0]=45 → body[0]=0)
10. `TestPlayerSetGender_UnmappedKeysWriteMinusOne` — body `[999, 999, 999, 999, 999, 999, 999]` + `SetGender(1)` → body `[-1, -1, -1, -1, -1, -1, -1]`. Direction-0 variant: body `[999, 999, ...]` + `SetGender(0)` → `body[1] == 14`, others == `-1`. **Pins `NAI-SETGENDER-D-TS-UNMAPPED-IDKIT-INHERITED`.**
11. `TestPlayerSetGender_DoesNotFlipMaskAppearance` — capture `p.masks` before/after `SetGender(1)` and `SetGender(0)`; assert `p.masks` unchanged in both directions. **Pins the TS-faithful deferred-rebuild assertion.**
12. `TestPlayerSetGender_LossyCollapse` — body[0]=19 + `SetGender(1)` (→ 56) + `SetGender(0)` (→ 18, NOT 19). Documents canonical OSRS lossiness inline.

### 7.4 Modified test (pkg/script)

13. `handlers_b0_stubs_test.go` — drop `SET_GENDER` row. Remaining stubs verified: `PUSH_VARBIT`, `POP_VARBIT`, `LC_OP`, `OC_IOP`, `OC_OP` (5 rows, not 6).

## 8. Audit-grep keywords (carry-forward)

Add to the carry-forward keyword set per `[[queue-skincolour-validator-slice-close]]` audit-grep pattern:

- `"SET_GENDER: unimplemented"` (production: expect 0 hits post-slice)
- `"NAI-162-D-STUB-SETGENDER"` (expect 0 hits post-slice — pin fully retired)
- `"TS-unimplemented stub.*SET_GENDER"` (expect 0 hits)
- `"setgender("` in goscape codebase (only test-side hits acceptable; production has zero callers because rs2-compiler emits opcode 2099 directly)

Pre-commit audit greps (each must show 0 or expected-only):
```bash
grep -rn "SET_GENDER: unimplemented" pkg/ modules/    # expect 0
grep -rn "NAI-162-D-STUB-SETGENDER"  pkg/ modules/    # expect 0 (full retirement)
grep -c  "TS-unimplemented" pkg/script/handlers_b0_stubs.go  # expect 5 rows, not 6
```

## 9. Pre-acknowledged watchpoints

1. **`mockPlayer` is in `pkg/script/runner_test.go` only** — confirmed via predecessor slice's one-site fake-sweep audit. Adding `SetGender` to the `ActivePlayer` interface will break compile in `runner_test.go` only; no other `ActivePlayer` impls exist outside `modules/world/player.go`.
2. **`handlers_b0_stubs_test.go` is table-driven** — dropping the `SET_GENDER` row may or may not require renaming the test function depending on its current generic name; if the function is named generically (`TestHandlers_B0Stubs_*`), no rename needed. Implementer reads the file before editing.
3. **Spurious gopls "No active builds contain ..." warnings** — pre-existing carry-forward for `pkg/script/` + `modules/world/` Read/Edit/Write; all false-positives; ignore.
4. **`modules/world/player.go` struct has fields `body [7]int`, `gender int`, `masks int` already** — no struct changes needed. Confirmed at `player.go:198, 202, 204`.

## 10. Out-of-scope follow-ups (named, not pursued)

- **`GenderValid`-as-named-export factoring**: future slice could collect `checkQueue` / `checkSkinColour` / `checkGender` into a small validator package or registry; not pursued.
- **Config-registry validator family** (`InvTypeValid`, `NpcTypeValid`, etc. — `ScriptInputConfigTypeValidator` cohort): out-of-scope from this slice, predecessor §9 named as a separate next-up port; needs a fresh brainstorming cycle.
- **Behavioral `MaskAppearance` flip**: would be TS-deviating; not pursued. Pinned via `TestPlayerSetGender_DoesNotFlipMaskAppearance`.
- **Round-trip identity correction in the maps**: not pursued; lossiness is canonical OSRS.
- **Compiler-side `setgender` validation**: rs2-compiler emits opcode 2099 unchanged; no compiler work.

## 11. Slice ordering (provisional task breakdown — refines in plan)

- **T1**: `checkGender` validator (`pkg/script/handlers_player.go`) + 2 validator unit tests
- **T2**: `maleFemaleMap` + `femaleMaleMap` + `lookupGenderIdkit` + `(*Player).SetGender` (`modules/world/player_gender.go`) + 6 setter unit tests
- **T3**: `ActivePlayer.SetGender` interface addition + `mockPlayer.SetGender` capture
- **T4**: `handleSetGender` real impl in `handlers_player.go` + deletion from `handlers_b0_stubs.go` + 4 handler unit tests + `handlers_b0_stubs_test.go` row deletion + retire `NAI-162-D-STUB-SETGENDER` pin
- **T5**: Audit-grep carry-forward sweep + `chore(close)` commit + memory write

## 12. Close mechanics

- Final commit: `chore(close): SETGENDER body port + GenderValid` with `--allow-empty`
- Memory: `[[setgender-genderval-port-close]]` capturing slice digest, new pin, audit findings (Content callsite, round-trip lossiness, slot-1 hardcode)
- MEMORY.md index prepend above `[[queue-skincolour-validator-slice-close]]`
