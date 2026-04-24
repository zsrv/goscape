# S7b — P_ANIMPROTECT + NumberNotNull Helper Design

> **Sub-spec context:** Twenty-eighth runescript sub-spec; second of S7. Implements `P_ANIMPROTECT` (opcode 2066), the next missing handler blocking `[proc,update_all]` after S7a (now stalls at `pc=66`). Bundled with the introduction of a shared `NumberNotNull` validator that closes the back-fill deferral at `pkg/script/handlers_npc.go:251` (recorded in `nai_followups` memory).

> **TS-faithfulness gate:** Matches `LostCityRS/Engine-TS/src/engine/script/handlers/PlayerOps.ts:1171-1172` and `ScriptValidators.ts:36-41`. One pre-existing deviation surface (the field's reader path is unported); no new behavioral divergences.

> **Scope:** One opcode (P_ANIMPROTECT 2066), one validator helper with two consumers (`handlePAnimProtect` + back-fill into `handleNpcSetTimer`), one `Player` field, one `ActivePlayer` interface method. ~25 LOC production + ~75 LOC tests. Compressed cadence per `compressed_cadence.md` memory: combined spec+plan, formal review skipped.

## 1. Goal

Implement `OpPAnimProtect` (2066) so `[proc,update_all]` advances past `pc=66`. The opcode:

1. Requires the script to hold protected access on its active player (TS `checkedHandler(ProtectedActivePlayer, ...)`).
2. Pops one int.
3. Rejects `-1` (TS `NumberNotNull` validator).
4. Stores the value on the active player's `animProtect` field.

The field is a truthy gate: when nonzero, in-engine animation requests should be suppressed. The reader path lives in `Player.ts:1842` (`if (anim >= SeqType.count || this.animProtect)`) and is **not** ported in goscape — anim playback as a whole has not been ported yet. The setter still has to land for `update_all` to make forward progress; the reader follows when anim playback is ported (tracked as **S7b-D1**).

Bundled with the validator: introduce `checkNotNull(v int, op string) error` next to the existing `checkStatID` precedent, and back-fill it into `handleNpcSetTimer` at `pkg/script/handlers_npc.go:251` (which carries an explicit `"No NumberNotNull check — tracked as future fidelity-audit item"` deferral). Also remove the corresponding entry from `nai_followups` memory.

## 2. TS reference

- `src/engine/script/handlers/PlayerOps.ts:1171-1172` — `[ScriptOpcode.P_ANIMPROTECT]: checkedHandler(ProtectedActivePlayer, state => { state.activePlayer.animProtect = check(state.popInt(), NumberNotNull); })`. One line of behavior: gated on protected active player, pops one int, rejects -1, assigns.
- `src/engine/script/ScriptOpcodePointers.ts:505-508` — declares `[ScriptOpcode.P_ANIMPROTECT]: { require: ['p_active_player'], require2: ['p_active_player2'] }`. The `require2` (secondary active player) form is unused by `update_all` and not in scope here; goscape's collapsed pointer model has no `_activePlayer2` to gate against (S7a-D1 territory).
- `src/engine/script/ScriptValidators.ts:36-41` — `ScriptInputNumberNotNullValidator.validate`: `if (input !== -1) return input; throw Error('An input number was null(-1).');`. Exact rule: rejects -1, accepts everything else (including `0`, `MIN_INT`, `MAX_INT`).
- `src/engine/entity/Player.ts:321` — `animProtect: number = 0;` — field declaration, default 0.
- `src/engine/entity/Player.ts:1842` — reader: `if (anim >= SeqType.count || this.animProtect) { return; }`. **Not ported in goscape** (anim playback path absent). See deviation S7b-D1.

## 3. Architecture

### 3.1 Validator helper (`pkg/script/handlers_player.go`)

Colocated with the existing `checkStatID` (handlers_player.go:67 area). Same call-site convention: handler does `if err := checkX(...); err != nil { return err }`.

```go
// checkNotNull mirrors TS NumberNotNull (ScriptValidators.ts:36-41) — rejects
// the script "null number" sentinel -1, accepts every other int. Used by
// handlers wrapping a popInt result with TS check(..., NumberNotNull).
func checkNotNull(v int, op string) error {
    if v == -1 {
        return fmt.Errorf("%s: input number was null(-1)", op)
    }
    return nil
}
```

### 3.2 Handler (`pkg/script/handlers_player.go`)

Inserted alphabetically near `handlePFindUID` (S7a precedent — same file). Uses the existing `requireProtectedActivePlayer` gate (handlers_player.go:48) — no new gate machinery.

```go
// handlePAnimProtect (P_ANIMPROTECT, opcode 2066) sets the active player's
// animProtect flag. While nonzero, in-engine animation requests should be
// suppressed (TS Player.ts:1842 — reader path not yet ported in goscape;
// tracked as S7b-D1). Mirrors TS PlayerOps.ts:1171-1172.
func handlePAnimProtect(s *ScriptState) error {
    if err := requireProtectedActivePlayer(s, "P_ANIMPROTECT"); err != nil {
        return err
    }
    v := s.PopInt()
    if err := checkNotNull(v, "P_ANIMPROTECT"); err != nil {
        return err
    }
    s.Self.SetAnimProtect(v)
    return nil
}
```

### 3.3 Dispatch registration (`pkg/script/handlers.go`)

One line added to the `handlers` map, near the existing `OpPFindUID: handlePFindUID` (line 334):

```go
OpPAnimProtect: handlePAnimProtect,
```

### 3.4 ActivePlayer interface (`pkg/script/active.go`)

One new method on the `ActivePlayer` interface (active.go:6):

```go
// SetAnimProtect updates the player's anim-protect flag. While nonzero, the
// engine should suppress in-engine animation requests (gated reader unported,
// see PAnimProtect handler comment).
SetAnimProtect(v int)
```

### 3.5 Player field + impl

Field added to `Player` struct in `modules/world/player.go`. Placement: next to existing per-player flags (e.g., near the identity / mode block after `staffModLevel`), in a new mini-section comment if no obvious adjacent group exists. Defaults to 0 (Go zero value, matches TS).

```go
animProtect int
```

Setter in `modules/world/player_script.go` (the script-interface impl file established by S7a's commit `fdeccf5`):

```go
func (p *Player) SetAnimProtect(v int) { p.animProtect = v }
```

### 3.6 NPC back-fill (`pkg/script/handlers_npc.go`)

Replace the deferral comment + insert the check. The current handler (lines 247-258):

```go
// handleNpcSetTimer (NPC_SETTIMER, opcode 2536) sets the active
// NPC's ai_timer tick interval. Pop order: interval. Mirrors TS
// NpcOps.ts:278-280. No NumberNotNull check — tracked as future
// fidelity-audit item in nai_followups memory.
func handleNpcSetTimer(s *ScriptState) error {
    if err := requireActiveNpc(s, "NPC_SETTIMER"); err != nil {
        return err
    }
    interval := s.PopInt()
    s.ActiveNpc.SetTimer(interval)
    return nil
}
```

Becomes:

```go
// handleNpcSetTimer (NPC_SETTIMER, opcode 2536) sets the active
// NPC's ai_timer tick interval. Pop order: interval. Mirrors TS
// NpcOps.ts:278-280, including the NumberNotNull check (closed in S7b).
func handleNpcSetTimer(s *ScriptState) error {
    if err := requireActiveNpc(s, "NPC_SETTIMER"); err != nil {
        return err
    }
    interval := s.PopInt()
    if err := checkNotNull(interval, "NPC_SETTIMER"); err != nil {
        return err
    }
    s.ActiveNpc.SetTimer(interval)
    return nil
}
```

### 3.7 Memory hygiene

Remove the `NPC_SETTIMER` deferral entry from `nai_followups.md` after the back-fill lands. (Per `close_commit_memory_trailer.md`, the close commit also gets `Closes memory: nai_followups.md (NPC_SETTIMER NumberNotNull)` trailer.)

## 4. File map

| File | Action | LOC |
|---|---|---|
| `pkg/script/handlers_player.go` | +`checkNotNull` helper, +`handlePAnimProtect` | +18 |
| `pkg/script/handlers.go` | Register `OpPAnimProtect: handlePAnimProtect` | +1 |
| `pkg/script/active.go` | Add `SetAnimProtect(v int)` to `ActivePlayer` interface | +2 |
| `pkg/script/handlers_npc.go` | Insert `checkNotNull` call in `handleNpcSetTimer`; rewrite comment | +3 / -2 |
| `pkg/script/runner_test.go` | Stub `SetAnimProtect(int)` on `mockPlayer` (mirrors S7a's `CanAccess` stub addition) | +3 |
| `pkg/script/handlers_player_test.go` | +`checkNotNull` table test (5 cases), +5 `handlePAnimProtect` tests | +85 |
| `pkg/script/handlers_npc_test.go` | +1 regression test: `interval=-1` errors | +15 |
| `modules/world/player.go` | `animProtect int` field on `Player` struct | +1 |
| `modules/world/player_script.go` | `SetAnimProtect` method | +3 |

**Production total:** ~28 LOC. **Test total:** ~100 LOC. Inside compressed-cadence territory; one task.

## 5. Test plan

### 5.1 Validator tests (`handlers_player_test.go`)

```go
func TestCheckNotNull(t *testing.T) {
    tests := []struct {
        name    string
        v       int
        wantErr bool
    }{
        {"null sentinel", -1, true},
        {"zero", 0, false},
        {"positive", 1, false},
        {"min int32", math.MinInt32, false},
        {"max int32", math.MaxInt32, false},
    }
    // table loop, assert err presence and message contains "OP: input number was null(-1)" on -1
}
```

### 5.2 Handler tests (`handlers_player_test.go`)

Five cases for `handlePAnimProtect`. Pattern follows S7a's `pfinduid_*` tests (use `newSingleOp("panimprotect_*", OpPAnimProtect)` fixture).

1. **TestPAnimProtectHappyPathZero** — `Protect=true`, `Self != nil`, push 0 → no error, `mockPlayer.animProtectValue == 0`, `Self` unchanged.
2. **TestPAnimProtectHappyPathNonzero** — `Protect=true`, push 1 → no error, `mockPlayer.animProtectValue == 1`.
3. **TestPAnimProtectNullRejected** — `Protect=true`, push -1 → error message contains `"P_ANIMPROTECT: input number was null(-1)"`. `mockPlayer.animProtectValue` unchanged from initial sentinel.
4. **TestPAnimProtectNotProtected** — `Protect=false`, `Self != nil`, push 0 → error message contains `"P_ANIMPROTECT: script not protected"`. `animProtectValue` unchanged. (Verifies `requireProtectedActivePlayer` precedes `popInt` — value should NOT have been popped or applied.)
5. **TestPAnimProtectNoActivePlayer** — `Self=nil` → error from `requireActivePlayer` chain (message contains `"P_ANIMPROTECT"`). `animProtectValue` unchanged.

`mockPlayer` (in `runner_test.go`) gains:
- `animProtectValue int` field
- `SetAnimProtect(v int) { m.animProtectValue = v }` method

### 5.3 NPC regression test (`handlers_npc_test.go`)

6. **TestNpcSetTimerNullRejected** — Pre-S7b: passing `interval=-1` silently called `SetTimer(-1)`. Post-S7b: returns error matching `"NPC_SETTIMER: input number was null(-1)"`; `mockNpc.SetTimer` was NOT called.

If existing `TestNpcSetTimer*` tests pass `interval=-1` anywhere, they need the value updated to 0 or 1. Pre-implementation grep: `grep -n "SetTimer\|NPC_SETTIMER" pkg/script/handlers_npc_test.go` to enumerate. (Per `enumerate_all_sites.md` memory: re-grep at HEAD~1 too — confirm no test fixture currently relies on -1 being silently accepted. If any do, update them in the same commit and call them out in the close report.)

### 5.4 Script-VM integration (no new test required)

`update_all` running through to completion is the implicit integration test — covered by the post-merge smoke retest, not a unit test in this sub-spec.

## 6. Task split

**Single task.** ~28 LOC production + ~100 LOC tests across 9 files. No external deps, no migrations, no fixture churn beyond the deliberate NPC test back-fill (and any -1 fixtures the §5.3 grep surfaces).

Commit messages (close-commit format per `close_commit_memory_trailer.md`):

1. `feat(script): handlePAnimProtect + checkNotNull validator (S7b)` — production code + new tests.
2. `fix(script): NPC_SETTIMER null-input check (S7b back-fill)` — back-fill + regression test, separate commit so the back-fill is reviewable in isolation. Trailer: `Closes memory: nai_followups.md (NPC_SETTIMER NumberNotNull deferral)`.
3. `chore(script): S7b closed — P_ANIMPROTECT + NumberNotNull` — close-commit. Trailer same as above plus `Smoke: update_all advances past pc=66`.

(Three small commits beat one bundled — keeps the back-fill auditable. Acceptable to bundle 1+2 if the implementer prefers, since the full diff is small.)

## 7. Deviations

| ID | Status |
|---|---|
| **S7a-D1** | Pre-existing — collapsed pointer model. Carried, not changed. Same observable behavior. |
| **S7a-D2** | Pre-existing — `Player.uid` source-of-truth unwired. Carried; unrelated to S7b's path. |
| **S7b-D1** | **NEW** — `Player.animProtect` field is **set** by `P_ANIMPROTECT` but never **read** by goscape's anim path (TS `Player.ts:1842`'s playback gate is unported because anim playback as a whole isn't yet ported). Effect: scripts can mark a player as anim-protected, but engine-side anim requests that *should* be suppressed will still play if/when they're added. Acceptable for `update_all` correctness (which only writes the flag); will be paid down when anim playback is ported (likely a separate sub-spec). Follow-up tracked in `nai_followups`. |

**Closures:** None for S7a. **One closure for the wider deviation log:** the `NPC_SETTIMER NumberNotNull` deferral at `handlers_npc.go:249` is closed by §3.6 — remove its `nai_followups` entry as part of the close commit.

## 8. Follow-ups

- **Anim playback porting** — when an in-engine anim playback path lands, the consumer of `Player.animProtect` must gate `playAnim` (or whichever entry point) on `animProtect == 0`, mirroring `Player.ts:1842`. Reference this design doc in that future sub-spec.
- **Other `check(state.popInt(), NumberNotNull)` sites in TS** — opportunistic audit candidate. `grep -rn "NumberNotNull" /home/owner/Code/github.com/LostCityRS/Engine-TS/src/engine/script/handlers/` enumerates ~12 TS sites; goscape may still have unported equivalents (or ported-without-check sites beyond the NPC one). Worth a one-shot deferral-sweep sub-spec à la NAI-16 once the count grows.
- **`require2` / secondary-active-player gating** — `ScriptOpcodePointers.ts:507` declares P_ANIMPROTECT also takes `require2: ['p_active_player2']`. Goscape's collapsed pointer model has no `_activePlayer2`. If/when a script is shown to invoke this opcode in the secondary form, the gating model needs revisiting (S7a-D1 territory). Not blocking; no known caller in `update_all`.

## 9. Self-review notes

- **Placeholders:** None. `math` import for the table test is implicit; helper signature is concrete.
- **Internal consistency:** §3.2 handler matches §5.2 test expectations 1:1; §3.6 NPC back-fill matches §5.3 regression test; §4 file map sums match the production+test LOC budget in §3 and §5; §7 deviations cross-reference §3.5 (write-only field) and the §3.6 closure.
- **Scope:** Single task, single sub-spec. No decomposition needed. The bundled NPC back-fill is in scope because it shares the helper introduced here — separating it would create a second sub-spec that reuses code with no other purpose, which is premature factoring.
- **Ambiguity:** Two checked. (1) "Where does the helper live?" explicitly resolved to `handlers_player.go` per Q2 in conversation; will move to `validators.go` in a future sub-spec only if a second TS validator gets ported. (2) "Does the deviation table close the existing NPC deferral?" explicitly resolved in §7's closure paragraph + §3.7 memory hygiene step.
- **Test-coverage crosscheck (per `plan_test_coverage_crosscheck.md` memory):** every code path in §3 has a corresponding test in §5: helper happy + sad path (5.1), handler protected + null + unprotected + no-active-player (5.2), back-fill regression (5.3). The `Self.SetAnimProtect` setter is exercised through cases 1 & 2; the `requireProtectedActivePlayer` chain is exercised through cases 4 & 5.
- **Plan-helper coverage (per `plan_helper_coverage.md` memory):** the new `checkNotNull` helper has exactly two consumers — `handlePAnimProtect` (case 3 above) and `handleNpcSetTimer` (case 6). Both call sites are covered with a -1 input test.
- **Enumerate-all-sites (per `enumerate_all_sites.md` memory):** §5.3 calls out the pre-implementation grep for `interval=-1` fixtures in `handlers_npc_test.go`. Implementer must execute it; close commit must include the result ("0 sites updated" or "N sites updated, listed below").
