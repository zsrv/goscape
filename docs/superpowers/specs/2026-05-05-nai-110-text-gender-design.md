# NAI-110 — TEXT_GENDER script-opcode handler

**Date:** 2026-05-05
**Status:** Spec
**Cadence:** Compressed (combined spec+plan, single-bundle subagent-driven-development on Sonnet) per `compressed_cadence.md` (≤15 prod LOC + ≤80 test LOC band) and `execution_mode_default.md`.
**Tech stack:** Go 1.26+ (per `go_version` memory).

---

## 1. Scope

Wire goscape's `OpTextGender` (script opcode 4504) to a handler that mirrors TS `PlayerOps.ts:787-794`. Pure stack op — no wire packet, no side effect.

**In scope:**
- New script handler `handleTextGender` in `pkg/script/handlers_player.go`.
- New dispatch entry `OpTextGender: handleTextGender` in `pkg/script/handlers.go`.
- Handler-level tests (4 cases) in `pkg/script/handlers_player_test.go`.

**Out of scope:**
- `[label,tutorial_complete]` `P_TELEJUMP: script not protected` runtime error — NAI-111 investigation per `nai_followups.md` NAI-109 close.
- Other unhandled opcodes — no current tutorial-path consumer beyond TEXT_GENDER.

---

## 2. Why

NAI-109 close smoke (2026-05-05) surfaced the next chatbox blocker on the tutorial-island smoke path:

```
script execute error  script="[proc,tutorial_please_wait_woodcutting]"
err="no handler for TEXT_GENDER (opcode 4504) at pc=4"
```

`text_gender(male, female)` is called 62 times across `2004scape/Server/data/src/scripts/`, including 4 sites in `tutorial/scripts/tut_chatbox_steps.rs2` and broad content-script use (chatnpc strings in quests, areas, minigames, macro events). Each unhandled call aborts its enclosing proc, propagating up to whichever step invoked it. Wiring this opcode unblocks tutorial-island chatbox progression and 60+ downstream content-script sites.

**Why:** Tutorial Island progression is the load-bearing smoke driver; this is the next opcode-level unblock.
**How to apply:** All infrastructure already exists (`s.Self.Gender()` on the `ActivePlayer` interface from NAI-47, `mockPlayer.genderValue` in tests). This is a pure additive port — no interface changes, no mock changes.

---

## 3. TS reference (canonical port source)

Per `ts_source_canonical_path` memory: `LostCityRS/Engine-TS` only.

**Script handler — `src/engine/script/handlers/PlayerOps.ts:787-794`:**
```typescript
[ScriptOpcode.TEXT_GENDER]: checkedHandler(ActivePlayer, state => {
    const [male, female] = state.popStrings(2);
    if (state.activePlayer.gender === 0) {
        state.pushString(male);
    } else {
        state.pushString(female);
    }
}),
```

**Pop order — `src/engine/script/ScriptState.ts:341-347`:**
```typescript
popStrings(amount: number): string[] {
    const strings = Array<string>(amount);
    for (let i = amount - 1; i >= 0; i--) {
        strings[i] = this.popString();
    }
    return strings;
}
```

So `[male, female] = popStrings(2)` means `strings[1] = female` is popped FIRST off the stack, `strings[0] = male` is popped SECOND. Compiler push order is therefore: `pushString(male)` then `pushString(female)` (top-of-stack = female).

**Pointer registry — `src/engine/script/ScriptOpcodePointers.ts:961-964`:**
```typescript
[ScriptOpcode.TEXT_GENDER]: {
    require: ['active_player'],
    require2: ['active_player2']
},
```

`require` = standard activePlayer pointer (TS `checkedHandler(ActivePlayer, ...)`). `require2` is the dual-pin shape used by string-ops that may run in either active-player slot, but the runtime handler unconditionally reads `state.activePlayer.gender` (singular) — there is no Self2 branch. Goscape ports the `require` arm only (matches TS handler body).

**Pointer gate:** `checkedHandler(ActivePlayer, ...)` → `PtrActivePlayer`; no `ProtectedActivePlayer`.

**Argument validation:** None. TS does not `check(..., StringNotNull)` either string. Empty strings push through unchanged. Goscape mirrors.

---

## 4. Architecture

Two additive changes; no interface or mock surface mutations.

### 4.1 Script handler

`pkg/script/handlers_player.go` — append after the existing handlers (file is the established home for `handleSetIdKit` and other ActivePlayer-only ports that also read `s.Self.Gender()`):

```go
// handleTextGender implements TEXT_GENDER.
// TS PlayerOps.ts:787-794 — pops two strings (popStrings(2) destructures
// [male, female], so female is popped first off the stack, male second),
// then pushes male if gender==0 else female. No null-check on either
// string (TS does not call check(..., StringNotNull)). Pure stack op —
// no wire packet, no side effect.
func handleTextGender(s *ScriptState) error {
    if err := requireActivePlayer(s, "TEXT_GENDER"); err != nil {
        return err
    }
    female := s.PopString()
    male := s.PopString()
    if s.Self.Gender() == 0 {
        s.PushString(male)
    } else {
        s.PushString(female)
    }
    return nil
}
```

### 4.2 Registry

`pkg/script/handlers.go` — add an entry in the dispatch map alongside other 45xx-band string ops (lexical position to match the existing string-ops grouping; final placement decided by plan-author after grepping the immediate neighborhood):

```go
OpTextGender: handleTextGender,
```

---

## 5. Test strategy

All tests in `pkg/script/handlers_player_test.go`. No new test fixtures needed — `mockPlayer.genderValue` and `Gender() int` already exist (NAI-47).

Per `plan_test_coverage_crosscheck` memory: every case below MUST appear in the plan's task code block and run as part of the bundle.

**(a) `TestTextGenderMale`** — happy path, gender=0.
- `mp := &mockPlayer{genderValue: 0}`, init state with `Self: mp` and `Pointers: PtrActivePlayer`.
- Push `"MALE"` then `"FEMALE"` (stack: [MALE, FEMALE], top = FEMALE).
- Dispatch `OpTextGender`.
- Assert: no error; SSP=1; `state.PopString() == "MALE"`.

**(b) `TestTextGenderFemale`** — happy path, gender=1.
- Same as (a) but `genderValue: 1`.
- Assert: no error; SSP=1; `state.PopString() == "FEMALE"`.

**(c) `TestTextGenderNoActivePlayer`** — pointer-gate.
- Init state with `Self: nil` (or `Pointers: 0`).
- Push two arbitrary strings.
- Dispatch.
- Assert: error with substring `"TEXT_GENDER: no active player"`; string stack unchanged at SSP=2.

**(d) `TestTextGenderEmptyStrings`** — null-string passthrough pin.
- `genderValue: 0`. Push `""` then `""`. Dispatch. Assert: no error; SSP=1; `PopString() == ""`. Pins TS divergence-from-norm: TS does NOT call `check(..., StringNotNull)`, so empty strings are valid input. Per `ts_asymmetry_dual_pin` memory: this absence-pin escalates if upstream TS adds a check.

---

## 6. Risks & validation

| Risk | Mitigation |
|------|------------|
| Pop order reversed (gender=0 returns female) | TS `popStrings(2)` order traced explicitly in §3; tests (a)+(b) pin both branches with literal string sentinels (`"MALE"`/`"FEMALE"`) so a swap shows as a direct string mismatch, not a stack-shape bug. |
| `requireActivePlayer` returns wrong error string | Established convention; TUT_FLASH (NAI-109), GETTIMER, every queue handler use the same `op + ": no active player"` shape. Test (c) pins. |
| `Gender()` semantics drift (0/1 mapping) | `Player.gender` field at `modules/world/player.go:166` is the int written into the appearance buffer at `modules/world/appearance.go:64` (`buf.P1(uint8(p.gender))`). TS uses `=== 0` for male; goscape mirrors. NAI-47 SETIDKIT already depends on this mapping (`handlers_player.go:207-209`). |
| Tab/byte width concerns | N/A — strings, not numbers. |

**Verified premises (controller pre-flight `controller_preflight`) at HEAD `278663f`:**
- `OpTextGender = 4504` declared at `pkg/script/opcode.go:416`. ✓
- Name-table entry at `pkg/script/opcode.go:1165-1166` (`return "TEXT_GENDER"`). ✓
- NO dispatch entry in `pkg/script/handlers.go` map. ✓
- `ActivePlayer.Gender() int` declared at `pkg/script/active.go:513` (NAI-47). ✓
- `(*Player).Gender()` impl at `modules/world/player_script.go:941`. ✓
- `mockPlayer.genderValue` field + `Gender()` method at `pkg/script/runner_test.go:293, 586`. ✓
- `requireActivePlayer` helper at `pkg/script/handlers_player.go:33-40`. ✓
- `s.PopString()` / `s.PushString(v)` at `pkg/script/state.go:295-313`. ✓
- TS handler at `PlayerOps.ts:787-794`. ✓
- TS pop order at `ScriptState.ts:341-347`. ✓

All 10 premises verified at HEAD `278663f` before plan-author dispatch.

---

## 7. Smoke

User-launched server + Java client, post-bundle. Walk to any tutorial chatbox step that triggers `text_gender` — `tut_chatbox_steps.rs2:36` (`tutorialstep("Please wait...", "...whilst <text_gender("he", "she")> does all the hard work.")`) is the binding cite from NAI-109's smoke trace.

- Pre-fix: WARN log `"no handler for TEXT_GENDER (opcode 4504) at pc=4"`.
- Post-fix: chatbox renders the gender-substituted text; no warn log.

Per `smoke_test_server_handoff` memory: server is user-launched; sandbox cannot reach Java client.

Per `cascade_theory_smoke_binding` memory: smoke binds the wire-up. If the post-fix walkthrough surfaces a different blocker (P_TELEJUMP, missing handler from the 90-strong unhandled set), route to NAI-111 or a follow-up.

---

## 8. Closes

- Tutorial-step `text_gender` runtime errors (4 sites in `tut_chatbox_steps.rs2`; 60+ content-script sites unblocked downstream).

**Does NOT close:**
- `[label,tutorial_complete]` `P_TELEJUMP: script not protected` — NAI-111 investigation per `nai_followups.md` NAI-109 close section.
- Remaining unhandled opcodes — no current tutorial consumer.

---

## 9. Out-of-scope follow-ups

- **NAI-111:** P_TELEJUMP protect-context investigation, queued on `nai_followups.md` from NAI-109 close.
- Future "missing handler bring-up" sub-specs as content surfaces consume opcodes from the unhandled set.
