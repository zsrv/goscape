# S7e — ALLOWDESIGN Design

> **Sub-spec context:** Thirty-first runescript sub-spec; fifth of S7. Implements `ALLOWDESIGN` (opcode 2001), the next missing handler blocking `[label,start_tutorial]` after S7d landed. The last smoke run stalled at `script=[label,start_tutorial] err="no handler for ALLOWDESIGN (opcode 2001) at pc=50"` — this is the tutorial-entry path, fired on every fresh/tutorial character before any combat script runs, so it blocks verification of S7d's `combat_get_damagetype` → `DB_GETFIELD` path until ALLOWDESIGN ships.

> **TS-faithfulness gate:** Matches `LostCityRS/Engine-TS/src/engine/script/handlers/PlayerOps.ts:1022-1024`, `ScriptOpcodePointers.ts:17-19`, and `Player.ts:323`. One new deviation (`S7e-D1`): the `allowDesign` flag's reader path (`IdkSaveDesignHandler`) is not ported — goscape writes the flag but no consumer gates on it yet.

> **Scope:** One opcode (ALLOWDESIGN 2001), one `ActivePlayer` interface method, one `Player` field + setter, one handler. ~15 LOC production + ~70 LOC tests. Compressed cadence per `compressed_cadence.md` memory: combined spec+plan, formal two-stage review skipped. **Bundled polish** from the S7d code review is folded in: (a) `handlers.go:115` comment correction, (b) `handleDbFindByIndex` pop/check reorder to match TS.

## 1. Goal

Implement `OpAllowDesign` (2001) so `[label,start_tutorial]` advances past `pc=50`. The opcode:

1. Requires an active player (TS `require: ['active_player']` — **not** Protected).
2. Pops one int.
3. Rejects `-1` (TS `NumberNotNull` validator).
4. Coerces the value to `bool` via `v == 1` and stores it on the active player's `allowDesign` field.

The field is a boolean gate: when `true`, the client's `IdkSaveDesign` inbound packet (character-design recustomise) is permitted. The reader path lives in `IdkSaveDesignHandler.ts:10` (`if (!player.allowDesign) return;`) and is **not** ported in goscape — the inbound packet handler itself is unported. The setter still has to land for `start_tutorial` to advance; the reader follows when the IdkSaveDesign inbound packet is ported (tracked as **S7e-D1**).

Bundled with the opcode: two polish items from the S7d review that naturally land in the same file(s):

- `pkg/script/handlers.go:115` — `// DB ops (7501-7510).` is inaccurate; the actual registered set is 7501-7506 plus 7510, with 7500/7507-7509 deferred. Corrected in this sub-spec since we're editing the registration block anyway to add ALLOWDESIGN.
- `pkg/script/handlers_db.go:177-181` — `handleDbFindByIndex` currently pops `index` before the `DbTable==nil` check; TS `DbOps.ts:152-164` does the check first. Pure reorder; no behavioral delta (`s.PushInt(-1)` on range miss only fires when a table is selected anyway, and the existing `TestHandleDbFindByIndex_NoTableSelected` asserts only error-presence, not stack state). Closes a `true_to_ts_gate` fidelity nit flagged in S7d review.

## 2. TS reference

- `src/engine/script/handlers/PlayerOps.ts:1022-1024` — `[ScriptOpcode.ALLOWDESIGN]: state => { state.activePlayer.allowDesign = check(state.popInt(), NumberNotNull) === 1; },`. One line of behavior: pops one int, rejects -1, coerces `=== 1 → boolean`, assigns.
- `src/engine/script/ScriptOpcodePointers.ts:17-19` — `[ScriptOpcode.ALLOWDESIGN]: { require: ['active_player'] }`. Unlike S7b's `P_ANIMPROTECT`, this is the **unprotected** form — any script holding `active_player` may call it. No `require2`.
- `src/engine/entity/Player.ts:323` — `allowDesign: boolean = false;` — field declaration, default `false`.
- `src/network/game/client/handler/IdkSaveDesignHandler.ts:10` — reader: `if (!player.allowDesign) { return; }`. **Not ported in goscape** (the `IdkSaveDesign` inbound packet handler is not registered). See deviation S7e-D1.
- `src/engine/script/ScriptValidators.ts:36-41` — `NumberNotNull` is already ported in goscape as `checkNotNull` at `pkg/script/handlers_player.go:61` (landed in S7b). Reused as-is.

## 3. Architecture

### 3.1 `ActivePlayer` interface method (`pkg/script/active.go`)

Inserted after the S7c `SetAppearanceInv` block (end of file, before the `ActiveNpc` interface). Type is `bool`, not `int`, because TS stores a boolean and the coercion is lossy — there's no reason to propagate the pre-coercion int to the Player side.

```go
// S7e: character-design save gate.

// SetAllowDesign updates the active player's allowDesign flag. When true,
// the client's IdkSaveDesign inbound packet (character-design recustomise)
// is permitted to apply. Mirrors TS Player.allowDesign
// (Engine-TS/src/engine/entity/Player.ts:323). The handler coerces the
// popped int via v==1 before calling. Reader path (IdkSaveDesignHandler)
// unported — deviation S7e-D1.
SetAllowDesign(v bool)
```

### 3.2 Handler (`pkg/script/handlers_player.go`)

Inserted next to `handleBuildAppearance` (same file, same shape: ActivePlayer gate, popInt, checkNotNull, setter call). Not co-located with `handlePAnimProtect` because that one is Protected-gated and ALLOWDESIGN is not — keeping the gate families grouped aids readability.

```go
// handleAllowDesign (ALLOWDESIGN, opcode 2001) sets the active player's
// allowDesign flag. Pops one int, rejects -1 via NumberNotNull, and stores
// (v == 1) as a bool. Gate is ActivePlayer (not Protected). Mirrors TS
// PlayerOps.ts:1022-1024. The gate permits `IdkSaveDesign` inbound packets
// (character-design recustomise) — reader path unported, see S7e-D1.
func handleAllowDesign(s *ScriptState) error {
    if err := requireActivePlayer(s, "ALLOWDESIGN"); err != nil {
        return err
    }
    v := s.PopInt()
    if err := checkNotNull(v, "ALLOWDESIGN"); err != nil {
        return err
    }
    s.Self.SetAllowDesign(v == 1)
    return nil
}
```

### 3.3 `Player` field + setter (`modules/world/`)

Field declaration goes in the `player.go` struct near the `// === anim-protect (S7b) ===` block (both are write-only script-set gates with unported readers — symmetric structure aids discoverability). Setter lives in `player_script.go` next to `SetAnimProtect`.

`player.go`:
```go
// === character-design gate (S7e) ===
// allowDesign permits IdkSaveDesign inbound packets (character-design
// recustomise) when true. Set by the ALLOWDESIGN script opcode. Reader
// path unported (S7e-D1).
allowDesign bool
```

`player_script.go`:
```go
// SetAllowDesign implements script.ActivePlayer.SetAllowDesign. Stores the
// flag; ALLOWDESIGN (opcode 2001) is the sole writer. Reader path unported
// per S7e-D1.
func (p *Player) SetAllowDesign(v bool) { p.allowDesign = v }
```

### 3.4 Handler registry (`pkg/script/handlers.go`)

Register `OpAllowDesign` near the other player flag-setters. The natural insertion point is adjacent to where BUILDAPPEARANCE and P_ANIMPROTECT are registered (grep `OpBuildAppearance\|OpPAnimProtect` to find the exact block at implementation time — confirmed in repo but exact line will shift with polish edits).

```go
// Player flag setters.
OpAllowDesign: handleAllowDesign,
```

### 3.5 Bundled polish — `handlers.go:115` comment

Before:
```go
// DB ops (7501-7510).
```

After:
```go
// DB ops (7501-7506, 7510; 7500/7507-7509 deferred).
```

Zero-risk one-line edit. Accurately describes the registered set (`OpDbFindNext` 7501, `OpDbGetField` 7502, `OpDbGetFieldCount` 7503, `OpDbListAllWithCount` 7504, `OpDbGetRowTable` 7505, `OpDbFindByIndex` 7506, `OpDbListAll` 7510 — verified at `handlers.go:116-122`) and names what's deferred (`OpDbFind` 7500, `OpDbFindRefine` 7507, `OpDbFindRefineWithCount` 7508, `OpDbFindWithCount` 7509).

### 3.6 Bundled polish — `handleDbFindByIndex` reorder

Before (`handlers_db.go:177-181`):
```go
func handleDbFindByIndex(s *ScriptState) error {
    index := s.PopInt()
    if s.DbTable == nil {
        return fmt.Errorf("DB_FINDBYINDEX: no table selected")
    }
    ...
}
```

After:
```go
func handleDbFindByIndex(s *ScriptState) error {
    if s.DbTable == nil {
        return fmt.Errorf("DB_FINDBYINDEX: no table selected")
    }
    index := s.PopInt()
    ...
}
```

Matches TS `DbOps.ts:152-164`:
```ts
if (!state.dbTable) { throw new Error('No table selected'); }
const index = state.popInt();
```

Behavioral delta: in the "no table selected" path, the int stays on the Go stack instead of being popped. Script abort follows in both cases, so no observable script-level difference. Existing test `TestHandleDbFindByIndex_NoTableSelected` at `handlers_db_test.go:565-572` asserts error-presence only (not stack state), so it passes unchanged.

## 4. File map

| File | Change | Est. LOC |
|---|---|---|
| `pkg/script/active.go` | + `SetAllowDesign(v bool)` interface method with doc | +8 |
| `pkg/script/handlers_player.go` | + `handleAllowDesign` function | +16 |
| `pkg/script/handlers.go` | + `OpAllowDesign` registry entry; edit `// DB ops...` comment | +2, ±1 |
| `pkg/script/handlers_db.go` | reorder `handleDbFindByIndex` pop/check (polish) | ±4 |
| `modules/world/player.go` | + `allowDesign bool` field with comment | +5 |
| `modules/world/player_script.go` | + `SetAllowDesign` method with doc | +4 |
| `pkg/script/runner_test.go` | + `mockPlayer.SetAllowDesign` stub + counters | +10 |
| `pkg/script/handlers_player_test.go` | + 5 test cases for ALLOWDESIGN | +80 |

**Production total:** ~35 LOC touched (of which ~15 is net-new behavior; rest is bundled polish + plumbing).
**Test total:** ~90 LOC.

## 5. Test plan

All new tests live in `pkg/script/handlers_player_test.go`. Shape mirrors existing `TestBuildAppearance*` / `TestPAnimProtect*` coverage (same gate/validator/setter pattern).

### 5.1 TestAllowDesignTrue

Push `1`, dispatch `handleAllowDesign`, assert:
- returns nil
- `mockPlayer.allowDesignValue == true`
- `mockPlayer.allowDesignCalls == 1`

### 5.2 TestAllowDesignFalse

Push `0`, dispatch, assert:
- returns nil
- `mockPlayer.allowDesignValue == false`
- `mockPlayer.allowDesignCalls == 1`

### 5.3 TestAllowDesignNonOneCoercesToFalse

Push `2` (valid int, not 1, not null-sentinel), dispatch, assert:
- returns nil
- `mockPlayer.allowDesignValue == false`
- `mockPlayer.allowDesignCalls == 1`

Pins the exact `v == 1` coercion shape — a truthy-style `v != 0` mistake would fail this test. Also: push `-2` (negative, not null sentinel) as a table-driven sub-case to confirm only `1` produces `true`.

Prefer a single table-driven test over three separate top-level tests if the mocks compose cleanly (see 5.6 for the mock structure). Implementation detail — either shape is acceptable.

### 5.4 TestAllowDesignNullInput

Push `-1`, dispatch, assert:
- returns non-nil error matching `input number was null(-1)`
- `mockPlayer.allowDesignCalls == 0` (setter must NOT be called on validator failure)

Pins the `checkNotNull` precedence vs. the setter call.

### 5.5 TestAllowDesignRequiresActivePlayer

Run with `Self == nil` and `Pointers & PtrActivePlayer == 0`, dispatch, assert:
- returns non-nil error matching `no active player`
- `mockPlayer.allowDesignCalls == 0`

Pins the `requireActivePlayer` gate. Mirrors `TestBuildAppearanceRequiresActivePlayer` structure.

### 5.6 Mock extension (`runner_test.go`)

Add to `mockPlayer`:

```go
// S7e: SetAllowDesign stores the coerced-bool flag for ALLOWDESIGN tests.
// allowDesignCalls counts invocations so error-path tests can assert the
// setter was NOT called.
allowDesignValue bool
allowDesignCalls int
```

And the method:

```go
func (m *mockPlayer) SetAllowDesign(v bool) {
    m.allowDesignValue = v
    m.allowDesignCalls++
}
```

Two fields (value + call count) mirror the `appearanceInvCalls` pattern at `runner_test.go:418` — error-path tests assert the count stays at 0, happy-path tests assert both value and count.

### 5.7 No integration test

`[label,start_tutorial]` advancing past `pc=50` is the implicit integration test — covered by the post-merge smoke retest (the user's follow-up smoke run), not a unit test in this sub-spec. Same pattern as S7b §5.4.

## 6. Task split

**Single commit, single task.** Estimated total diff: ~125 LOC across 8 files. No external deps, no migrations, no fixture churn. The polish items ride in the same commit since they touch the same two files we're already editing and carry zero risk.

Close commit message (format per `close_commit_memory_trailer.md`, though S-series hasn't historically used the trailer — following it here anyway since it's a polish-bundle close):

```
feat(script): S7e closed — ALLOWDESIGN (2001) + S7d review polish

- handleAllowDesign: popInt + checkNotNull + SetAllowDesign(v == 1)
- Player.allowDesign field (write-only; reader path S7e-D1)
- polish: handlers.go:115 DB-ops comment accuracy
- polish: handleDbFindByIndex pops index after DbTable check (TS order)

Smoke: unblocks [label,start_tutorial] past pc=50; S7d combat_get_damagetype
path re-verified implicitly by the next tutorial-entry run.
```

(Acceptable to split into two commits — one for ALLOWDESIGN proper, one for the two polish fixes — if the implementer prefers reviewable-in-isolation granularity. Not required; diff is small.)

## 7. Deviations

| ID | Status |
|---|---|
| **S7a-D1** | Pre-existing — collapsed pointer model (no `_activePlayer2`). Carried; unrelated to S7e's single-active-player op. |
| **S7a-D2** | Pre-existing — `Player.uid` source-of-truth unwired. Carried; unrelated. |
| **S7b-D1** | Pre-existing — `Player.animProtect` reader path unported. Carried; unrelated field. |
| **S7c-D1** | Pre-existing — `generateAppearance` reads `invs.Worn` instead of `p.appearanceInv`. Carried; unrelated. |
| **S7d-D1** | Pre-existing — carried. |
| **S7d-D2** | Pre-existing — carried. |
| **S7d-D3** | Pre-existing — carried. |
| **S7d-D4** | Pre-existing — carried. |
| **S7e-D1** | **NEW** — `Player.allowDesign` field is **set** by `ALLOWDESIGN` (2001) but never **read** by goscape: the `IdkSaveDesign` inbound packet handler (TS `IdkSaveDesignHandler.ts`) is not ported. Effect: scripts can flag players as design-save-permitted, but no inbound packet route honours (or requires) the flag yet. Acceptable for `start_tutorial` correctness (which only writes the flag); paid down when the `IdkSaveDesign` inbound is ported, likely bundled with wider character-customise plumbing. Follow-up added below. |

**Closures:** None. S7d's two code-review nits (handlers.go:115 comment and handleDbFindByIndex order) are not tracked as deviations — they're pre-merge polish, closed by bundling into this commit per the plan §3.5–§3.6.

## 8. Follow-ups

- **`IdkSaveDesign` inbound packet port.** When the character-design recustomise inbound packet is ported, its handler must gate on `p.allowDesign` (mirroring TS `IdkSaveDesignHandler.ts:10`). Reference this design doc and deviation S7e-D1 in that future sub-spec. Likely a network-layer sub-spec rather than another RuneScript one.
- **S7d smoke re-verification.** After S7e merges and the next `start_tutorial` smoke clears `pc=50`, confirm the log shows `combat_get_damagetype` reaching `DB_GETFIELD` cleanly (promoting S7d from inferential to smoke-confirmed). If a new stall surfaces there, file as S7f; if clean, note it in the S7e close report.
- **Polish sweep candidates remaining.** After S7e's bundle, no known outstanding polish items in `pkg/script/` from the S7a-S7d review cycle. Opportunistic future sub-specs should re-grep `true_to_ts_gate`-flagged fidelity nits (pop-order mismatches, check-order mismatches) when touching the relevant files.

## 9. Self-review notes

- **Placeholders:** None. Every identifier cited (`handleBuildAppearance`, `handlePAnimProtect`, `requireActivePlayer`, `checkNotNull`, `SetAnimProtect`, `mockPlayer`, `OpAllowDesign`) is verified present in the repo by grep during brainstorm.
- **Internal consistency:** §3.2 handler matches §5.1-§5.5 test expectations 1:1 (gate → pop → null-check → setter coerce). §3.3 Player field + setter matches §5.6 mock extension signature (`bool`, one arg). §4 file map LOC sums align with the production+test budget named in §3/§5. §7 deviation S7e-D1 cross-references §3.2 handler comment and §1 goal paragraph.
- **Scope:** Single task, single sub-spec, compressed cadence. The two polish items are in scope because they're colocated with the ALLOWDESIGN edits (same two files) and each is under 5 LOC — separating them into a standalone polish sub-spec would create a second sub-spec touching only files we're already editing, which is the exact premature-factoring shape `compressed_cadence` warns against.
- **Ambiguity:** Three checked. (1) "bool vs int for the field type?" — resolved to `bool` in §3.1/§3.3; TS uses `boolean`, and the lossy `=== 1` coercion means preserving int on the Go side would only invent extra state. (2) "Gate is ActivePlayer, not Protected?" — resolved in §1 and §2 with `ScriptOpcodePointers.ts:17-19` citation. (3) "Does the DbFindByIndex reorder break existing tests?" — resolved in §3.6 by reading `TestHandleDbFindByIndex_NoTableSelected` at `handlers_db_test.go:565-572`: asserts error-presence only, no stack-state assertion, so passes unchanged.
- **Test-coverage crosscheck (per `plan_test_coverage_crosscheck.md` memory):** every code path in §3.2 has a corresponding test in §5: `requireActivePlayer` gate (5.5), `PopInt` + `checkNotNull` success (5.1–5.3), `checkNotNull` failure (5.4), `v == 1` coercion in both branches (5.1 + 5.2 + 5.3). The bundled polish in §3.6 is covered by the pre-existing `TestHandleDbFindByIndex_NoTableSelected` — no new test needed (§3.6 polish is pure ordering).
- **Helper coverage (per `plan_helper_coverage.md` memory):** no new shared helper introduced. `checkNotNull` is reused from S7b with one additional consumer (ALLOWDESIGN) covered by test 5.4.
- **TS source read (per `spec_ts_source_read.md` memory):** all §3 code blocks are drafted against directly-read TS source at `PlayerOps.ts:1022-1024`, `ScriptOpcodePointers.ts:17-19`, `Player.ts:323`, `IdkSaveDesignHandler.ts:10`, and `DbOps.ts:152-164` — not inferred by analogy from S7b/S7c.
- **Enumerate-all-sites (per `enumerate_all_sites.md` memory):** no propagation through a shared file required — ALLOWDESIGN is isolated. The polish in §3.6 opens `handlers_db.go`; pre-implementation grep of `handleDbFindByIndex\|DB_FINDBYINDEX` (performed during brainstorm) surfaced the four existing test cases in `handlers_db_test.go:520-585` — all confirmed compatible with the reorder.
