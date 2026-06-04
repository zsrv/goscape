# NAI-140 — Port `ColorConversion` and apply `rgb24to15` to `IF_SETCOLOUR` writer

**Date:** 2026-05-09
**Status:** Brainstorm-approved spec; combined spec+plan doc per compressed cadence.
**Cadence:** Compressed (`compressed_cadence.md`) — single end-of-impl reviewer on Sonnet; no two-stage review.
**Tech stack:** Go 1.26+ (`go_version.md`).
**Origin:** NAI-139 SECONDARY S1 (`docs/superpowers/handoffs/2026-05-09-nai-139-stage-1-smoke.md` §2 S1) — Quest Journal tab colors diverge from canonical 225 client expectation: not-started quests render **black** (expected red); in-progress quests render **orange** (expected yellow). Tab is populated and clickable; NAI-139 §1 criterion 5 still PASS.
**TS source:** `LostCityRS/Engine-TS/src/util/ColorConversion.ts` (full module). Consumer: `LostCityRS/Engine-TS/src/engine/script/handlers/PlayerOps.ts:632-639` (`IF_SETCOLOUR` handler).
**Content reference:** `LostCityRS/Content/scripts/general/scripts/quests.rs2:5-13` (`~send_quest_progress_colour` — calls `if_setcolour($component, ^red_rgb|^yellow_rgb|^green_rgb)`); `_constants/general/configs/colour.constant:1-10` defines `^red_rgb=0xFF0000`, `^green_rgb=0xFF00`, `^yellow_rgb=0xFFFF00` (24-bit RGB888).

---

## §1 PRIMARY criteria

User-driven re-run of NAI-139 fresh-tutorial smoke at HEAD post-T2 must observe:

1. **Quest Journal tab — not-started quests render red** (currently black). Spec-bound goldens: ANY quest with progress varp=0 (e.g. `runemysteries`, `doric`, `cook` on a fresh-tutorial account) renders in red on first open of Quest Journal tab at Lumbridge spawn.
2. **Quest Journal tab — in-progress quests render yellow** (currently orange). Spec-bound goldens: any quest where 0 < progress < complete-threshold renders in yellow. Fresh-tutorial accounts have none in this state by default; criterion (2) is verified by stat/cheat-driven mid-progress varp set OR deferred to first organic encounter.
3. **No regression in NAI-139 §1 criteria 0-6.** Tutorial-completion cascade still PASS; tabs still clickable + populated.

**Non-criterion (informational only, per `cascade_theory_smoke_binding.md`):**
- Completed-quest **green** branch is not smoke-bound on fresh-tutorial path (no completed quests reachable). Unit-test coverage at `pkg/colorconv/colorconv_test.go` `TestRgb24to15_Goldens` pins the green case (`0x00FF00 → 0x03E0`); production correctness for green follows from theory-of-fix (single writer-level conversion).

## §2 Architecture

### §2.1 Root cause

- TS `PlayerOps.ts:638` calls `state.activePlayer.write(new IfSetColour(com, ColorConversion.rgb24to15(colour)))`.
- Goscape `pkg/script/handlers_interface.go:302-315` (`handleIfSetColour`) defers conversion to `Player.IfSetColour`. Doc-comment at `:299-300` makes this explicit:
  > The TS handler converts rgb24→rgb15 before writing the wire op; that conversion is the Player impl's responsibility in this codebase.
- Goscape `modules/world/player_interface.go:92-98` (`Player.IfSetColour`) writes raw 24-bit RGB888 directly via `buf.P2(uint16(colour))` — **no conversion applied**, contradicting its own contract.

### §2.2 Symptom math

| Constant | TS `rgb24to15` | Goscape `uint16(colour)` low-16 | Client renders |
|---|---|---|---|
| `^red_rgb=0xFF0000` | `0x7C00` (red 5-5-5) | `0x0000` | **black** ✓ |
| `^yellow_rgb=0xFFFF00` | `0x7FE0` (yellow 5-5-5) | `0xFF00` | **orange** ✓ |
| `^green_rgb=0x00FF00` | `0x03E0` (green 5-5-5) | `0xFF00` | unobserved (no completed quests in fresh-tutorial smoke); same garbage as yellow's low-16 — this confirms the bug is uniform across all colours and not branch-specific. |

### §2.3 Fix

**New package:** `pkg/colorconv/` containing the full TS `ColorConversion.ts` port:

| Go symbol | TS counterpart | Notes |
|---|---|---|
| `Hsl24to16(hue, saturation, lightness int) int` | `hsl24to16` | 4-branch saturation-shift table on lightness. |
| `Rgb15to24(rgb int) int` | `rgb15to24` | Pure bit-twiddle. |
| `Rgb15toHsl16(rgb int) int` | `rgb15toHsl16` | Calls `RgbToHsl`. |
| `Rgb24to15(rgb int) int` | `rgb24to15` | **Sole production consumer today.** |
| `Rgb24toHsl16(rgb int) int` | `rgb24toHsl16` | Calls `RgbToHsl`. |
| `RgbToHsl(red, green, blue float64) int` | `rgbToHsl` | Float arithmetic; mirrors `(x*256.0)\|0` truncation (see §2.4 R2). |
| `RGB15_HSL16 [32768]int32` | `static readonly RGB15_HSL16` | Populated by package `init()`. |
| `ReverseHsl(hsl int) []int` | `reverseHsl` | Ships unused (see deviation below). |

**Wiring change:** `modules/world/player_interface.go:96`
```go
// Before
buf.P2(uint16(colour))
// After
buf.P2(uint16(colorconv.Rgb24to15(colour)))
```
Add import: `"github.com/zsrv/goscape/pkg/colorconv"`.

The pre-existing handler doc-comment at `pkg/script/handlers_interface.go:299-300` becomes accurate post-fix; no change needed.

### §2.4 Risks

- **R1 — Package-init cost.** `RGB15_HSL16` does 32K `Rgb15toHsl16` calls at world-server start. Estimated <50ms on commodity hardware; matches TS static-initializer cost. Acceptable; no mitigation needed.
- **R2 — TS `(x*256.0)|0` vs Go `int(x)` truncation.** TS `|0` does 32-bit signed wrap; Go `int(float64)` truncates toward zero. `RgbToHsl` can produce negative `hNorm` when `red==max && green<blue`, then computes `(hNorm/6)*256|0`. Mitigation:
  - Port using `int(x)` for TS-equivalent truncation when the value is in safe int32 range (the realistic case here — `hNorm` lies in roughly `[-1, 1]` after normalization, so `(hNorm/6)*256` lies in `[-43, 43]`).
  - Add a golden-value test (`TestRgbToHsl_Goldens` case `(red=1.0, green=0.0, blue=0.5)`) that exercises the negative-hue branch with a hand-computed expected value matching TS exactly. If divergence surfaces in CI, escalate to `int32(int64(x))` cast.
- **R3 — `ReverseHsl` is dead code on day 1.** Zero callers in goscape. Tracked as `NAI-140-D-REVERSEHSL-DEAD-API` per `dead_api_polish.md`; retire if no consumer materializes within ~3 sub-specs that touch `pkg/colorconv`.

### §2.5 Deviations introduced

- **`NAI-140-D-REVERSEHSL-DEAD-API`** (NEW CODE, fidelity-port): `colorconv.ReverseHsl` ships with no production consumer. **Why:** user chose full-module port at brainstorm time for fidelity. **How to apply:** at next sub-spec close that touches `pkg/colorconv`, audit consumer count; retire if still zero. Track in `nai_followups.md`.

No other deviations. The `(x*256.0)|0` semantics are preserved by hand-computed goldens (R2); the package-init pattern is idiomatic Go and matches TS lifetime.

## §3 Test strategy

### §3.1 Unit tests — `pkg/colorconv/colorconv_test.go` (new file)

| # | Name | Coverage |
|---|------|----------|
| T1.1 | `TestRgb24to15_Goldens` | `0xFF0000→0x7C00`, `0x00FF00→0x03E0`, `0xFFFF00→0x7FE0`, `0x000000→0x0000`, `0xFFFFFF→0x7FFF`. **The three quest goldens (red/green/yellow) directly validate the §1 PRIMARY fix.** |
| T1.2 | `TestRgb15to24_Goldens` | `0x7C00→0xF80000`, `0x03E0→0x00F800`, `0x7FFF→0xF8F8F8`. Round-trip-with-precision-loss. |
| T1.3 | `TestHsl24to16_BranchTable` | One case per branch: `lightness=244` (`>>4`), `lightness=218` (`>>3`), `lightness=193` (`>>2`), `lightness=180` (`>>1`), `lightness=100` (default). |
| T1.4 | `TestRgbToHsl_Goldens` | `(1.0,0.0,0.0)`, `(0.0,1.0,0.0)`, `(0.0,0.0,1.0)`, `(0.5,0.5,0.5)` (achromatic), `(1.0,0.0,0.5)` (negative-hue branch — R2 mitigation). Hand-computed oracle. |
| T1.5 | `TestRgb24toHsl16_DelegatesToRgbToHsl` | Single sentinel value matches independent computation. |
| T1.6 | `TestRgb15toHsl16_DelegatesToRgbToHsl` | Single sentinel value. |
| T1.7 | `TestRGB15HSL16_PopulatedByInit` | `len==32768`; `[0]==RgbToHsl(0,0,0)`; `[32767]==RgbToHsl(31/31,31/31,31/31)`. |
| T1.8 | `TestReverseHsl_RoundTripFromTable` | Pick two known HSL values; assert `ReverseHsl(h)` returns the rgb15 indices that map to `h` in `RGB15_HSL16`. |

### §3.2 Integration test — `modules/world/player_interface_test.go` (extend)

| # | Name | Coverage |
|---|------|----------|
| T2.1 | `TestIfSetColour_AppliesRgb24to15_OnWire` | Call `p.IfSetColour(com=12, colour=0xFF0000)`, drain bytes, assert payload bytes `[com_hi=0x00, com_lo=0x0C, colour_hi=0x7C, colour_lo=0x00]`. Pins the writer-level fix. Mirror `TestIfSetTextEmitsWire` pattern (`script_test.go:710`). |

### §3.3 Existing tests preserved

- `pkg/script/handlers_interface_test.go:361-381` (`TestIfSetColour`) **stays unchanged**. The mock recorder pins `{12, 0xff0000}` because the script handler still passes raw colour into `s.Self.IfSetColour` — the conversion happens in the writer, not the handler. This is per the existing architectural split (handler doc-comment `:299-300`); no test churn.

### §3.4 TDD sequencing (compressed cadence)

- **T1 (red)**: add `pkg/colorconv/colorconv_test.go` (8 tests) and `TestIfSetColour_AppliesRgb24to15_OnWire`. T1.1–T1.8 fail because `pkg/colorconv` doesn't exist; T2.1 fails on byte mismatch.
- **T2 (green)**: port `pkg/colorconv/colorconv.go` (full TS module + `init()` table); modify `modules/world/player_interface.go:96` + add import. All 9 tests pass; full repo `go test ./...` and `go vet ./...` green.
- **Reviewer (Sonnet)**: end-of-impl `superpowers:code-reviewer` agent on Sonnet (per `superpowers_code_reviewer_model.md`). Pass requires no Critical/Important issues.
- **T3 (close)**: smoke handoff per §4.

### §3.5 Pre-flight verification (controller pre-dispatch, per `controller_preflight.md`)

Before plan-author dispatch, controller verifies at HEAD `dd83c4d` (NAI-139 close):
- `rg "func handleIfSetColour" pkg/script/handlers_interface.go` → confirm signature + line `:302`.
- `rg "func \(p \*Player\) IfSetColour" modules/world/player_interface.go` → confirm signature + line `:93`; confirm `buf.P2(uint16(colour))` at `:96` is verbatim.
- `ls pkg/colorconv 2>/dev/null` → confirm package does not pre-exist.
- Re-Read `pkg/script/handlers_interface_test.go:361-381` → confirm mock-pin `{12, 0xff0000}` verbatim.
- Re-Read `LostCityRS/Engine-TS/src/util/ColorConversion.ts:1-133` → confirm TS source unchanged.

## §4 Smoke handoff

**Binding per `cascade_theory_smoke_binding.md`:** user re-runs the NAI-139 fresh-tutorial flow:
1. Fresh tutorial-stage account → complete Tutorial Island → Magic Instructor "Yes, go to mainland" → arrive Lumbridge spawn (3222, 3222, level 0).
2. Open Quest Journal tab.
3. **PRIMARY pin (criterion 1)**: not-started quest names render **red** (was: black).
4. **PRIMARY pin (criterion 2)**: any in-progress quest renders **yellow** (was: orange). If fresh-tutorial has none in this state, criterion 2 is informally verified by inspection of any pre-existing in-progress account on next handoff or via stat/cheat — not blocking close if all unit goldens (T1.1) pass.
5. **Regression check**: NAI-139 §1 criteria 0-6 all still PASS.
6. No `script not protected`, `proc not found`, opcode-dispatch warnings, panics, or stack traces during the cascade.

**Adjacent consumers not in smoke path** (scrolls via `general/scripts/scroll.rs2`, quest-complete UI via `send_quest_complete`): not blocked-on at close. Theory of fix (single writer-level conversion) covers them. Future smoke surfacing a divergence routes as separate sub-spec.

## §5 Implementation plan (compressed; combined into spec)

### §5.1 Files to create
- `pkg/colorconv/colorconv.go` — full TS port; package `colorconv`; `init()` populates `RGB15_HSL16`.
- `pkg/colorconv/colorconv_test.go` — tests T1.1–T1.8.

### §5.2 Files to modify
- `modules/world/player_interface.go` — import `pkg/colorconv`; change line `:96` from `buf.P2(uint16(colour))` to `buf.P2(uint16(colorconv.Rgb24to15(colour)))`.
- `modules/world/player_interface_test.go` — append `TestIfSetColour_AppliesRgb24to15_OnWire` (T2.1).

### §5.3 Files NOT to modify
- `pkg/script/handlers_interface.go` — handler unchanged; doc-comment `:299-300` already describes correct post-fix architecture.
- `pkg/script/handlers_interface_test.go` — mock-pin `{12, 0xff0000}` stays (handler doesn't convert).

### §5.4 Build sequence
1. **T1 red** — add tests T1.1–T1.8 + T2.1. Verify red. Commit: `test(colorconv): NAI-140 T1 red — rgb24to15 + writer goldens fail`.
2. **T2 green** — port `pkg/colorconv/colorconv.go`; wire `IfSetColour`. Verify all green + `go vet ./...` clean. Commit: `feat(colorconv): NAI-140 T2 green — port ColorConversion + apply rgb24to15 in IfSetColour`.
3. **Reviewer (Sonnet)** — dispatch `superpowers:code-reviewer` agent (model: sonnet). Address Critical/Important only.
4. **Close** — handoff doc + commit `chore(close): NAI-140 — quest-color cascade closed; rgb24to15 in writer`. Update `nai_followups.md`. Save memory entry.

### §5.5 Implementer notes
- TS `(x*256.0)|0` truncation: implement as `int(x)` in Go for in-range floats (`hNorm` in `[-1,1]` after `/6` and `*256` lands well within int range). If reviewer flags fidelity concern for hypothetical out-of-range inputs, escalate to `int32(int64(x))` and update test goldens accordingly.
- Mirror TS structure file-for-file: function order, local-variable names, branch order. Per `flat_arg_signature_for_cross_lang_parity.md` for cross-language review readability.
- `RGB15_HSL16` declared as package-level `var RGB15_HSL16 [32768]int32`; populate in `init()`. Do NOT use `sync.Once` — TS does eager init at class load, mirror that.
- Function exports: all 6 functions + `RGB15_HSL16` + `ReverseHsl` exported (`PascalCase`). Even `ReverseHsl` despite zero consumers (per user decision; tracked as `NAI-140-D-REVERSEHSL-DEAD-API`).

## §6 Closure criteria

- All 9 tests green; `go test ./... && go vet ./...` clean.
- Reviewer (Sonnet) verdict: no Critical/Important issues.
- Smoke §4 PRIMARY criteria 1 and 3 met (criterion 2 best-effort).
- Memory entries written: `colorconv_rgb24to15_in_writer.md` (project memory) pinning the architectural split; `nai_followups.md` entries for NAI-140 close + `NAI-140-D-REVERSEHSL-DEAD-API`.
- Close commit body includes `Closes memory:` trailer per `close_commit_memory_trailer.md`.
- Cascade attribution: NAI-140 closes the NAI-139 SECONDARY S1 thread; no new cascade-blockers expected (writer-level fix is exhaustive across all `if_setcolour` consumers).
