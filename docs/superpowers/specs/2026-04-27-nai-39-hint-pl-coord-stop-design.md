# NAI-39 — HINT_PL + HINT_COORD + HINT_STOP (with activePlayer2 substrate)

## Motivation

Three script-VM opcodes are declared in `pkg/script/opcode.go` but have no
handler registered in `pkg/script/handlers.go` at HEAD `e3ed6e1` (NAI-38
close):

- **HINT_COORD** (opcode 2027, declared at `pkg/script/opcode.go:127`) —
  pops `[offset, coord, height]`, validates the coord, calls
  `activePlayer.hintTile(offset, x, z, height)`. TS
  `PlayerOps.ts:866-871`.
- **HINT_PL** (opcode 2029, declared at `pkg/script/opcode.go:129`) — zero
  pops; reads `activePlayer2.slot`, calls
  `activePlayer.hintPlayer(slot)`. TS `PlayerOps.ts:976-978`.
- **HINT_STOP** (opcode 2030, declared at `pkg/script/opcode.go:130`) —
  zero pops; calls `activePlayer.stopHint()`. TS `PlayerOps.ts:873-875`.

The three opcodes are sibling stubs of HINT_NPC (opcode 2028), which
shipped at NAI-37 T6 (commit `8daaf98`). NAI-37 explicitly tagged the
remaining encoder branches as a deferred follow-up under tracker entry
`NAI-37-D-HINTARROW-PARTIAL-ENCODER` (declared at NAI-37 spec:184; tagged
at three code-tree sites — `pkg/script/handlers_player.go:842`,
`modules/world/player_script.go:154`, `pkg/io/protocol/game/server/prot.go:44`
— plus four NAI-37 plan/spec doc-tree sites). NAI-39 closes that deviation by porting the three remaining
handlers and the four remaining `HintArrowEncoder.ts` branches (type=2..6
TILE, type=10 PL, type=-1 STOP).

**Substrate work**: HINT_PL reads `state.activePlayer2.slot` in TS, but
goscape has no production-wired secondary-player slot at HEAD `e3ed6e1`.
`PtrActivePlayer2` is declared at `pkg/script/pointer.go:9` with **zero
consumers** — no `Self2`/`ActivePlayer2` field on `ScriptState`, no
`requireActivePlayer2` validator, no producer wiring. TS sets
`_activePlayer2` at three sites (`ScriptRunner.ts:86`, `ScriptState.ts:239`
push-frame, `PlayerOps.ts:1150` P_FINDUID), none of which are ported.
NAI-39 ports the exec-trigger seed (the first of those three sites) by
mirroring the NAI-11 `buildNpcScriptState` shape at
`modules/world/npc_script.go:225-261`. The other two TS producer sites
(push-frame save/restore, P_FINDUID) remain unported under existing /
new tracker entries.

Pre-NAI-39 behavior: 3 opcodes abort with `Aborted` execution state; logs
spam `runner.go:71`. The HintArrow encoder ships only the type=1 (NPC)
branch.

Post-NAI-39 behavior: all 3 opcodes execute TS-faithfully; every TS
`HintArrowEncoder.ts` branch has a goscape implementation; goscape gains
an activePlayer2 slot at the script-state layer plus a target-dispatch
refactor of `runScript` mirroring the NAI-11 NPC-side shape. A new
deviation `NAI-39-D-ACTIVEPLAYER2-NO-OPPLAYER-PRODUCER` documents that
the new rails have no production producer until OPPLAYER triggers are
ported in a future sub-spec.

## Tech stack

- **Go 1.26+** (per `go_version.md`; use modern Go syntax via the
  `use-modern-go` skill).
- TS source: `Engine-TS` only per `ts_source_canonical_path.md`.
  - `src/network/game/server/codec/HintArrowEncoder.ts:9-39` (encoder
    shape: 4 remaining branches type=2..6 / 10 / -1, all 6 bytes
    `p1+p2+p2+p1`)
  - `src/engine/script/handlers/PlayerOps.ts:866-871` (HINT_COORD)
  - `src/engine/script/handlers/PlayerOps.ts:873-875` (HINT_STOP)
  - `src/engine/script/handlers/PlayerOps.ts:976-978` (HINT_PL)
  - `src/engine/script/ScriptOpcodePointers.ts:128-139` (pointer
    requirements: HINT_COORD/HINT_STOP need active_player; HINT_PL
    additionally needs active_player2)
  - `src/engine/entity/Player.ts:2178-2180` (`hintTile(offset, x, z,
    height)` method; writes `HintArrow(offset, 0, 0, x, z, height)`)
  - `src/engine/entity/Player.ts:2182-2184` (`hintPlayer(playerSlot)`
    method; writes `HintArrow(10, 0, playerSlot, 0, 0, 0)`)
  - `src/engine/entity/Player.ts:2186-2188` (`stopHint()` method; writes
    `HintArrow(-1, 0, 0, 0, 0, 0)`)
  - `src/engine/script/ScriptRunner.ts:84-87` (target-dispatch seed for
    `_activePlayer2` when self=Player AND target=Player)
- Existing infrastructure (verified at HEAD `e3ed6e1`):
  - `OpHintArrow = Op{Opcode: 25, PayloadSize: 6}` at
    `pkg/io/protocol/game/server/prot.go:46`. PayloadSize already covers
    all 4 remaining branches; no protocol-layer change.
  - Script opcodes `OpHintCoord/OpHintPl/OpHintStop` already declared at
    `pkg/script/opcode.go:127-130` and named in the dispatcher (`opcode.go:661-668`).
    No opcode-table churn.
  - `(*Player).HintNpc` at `modules/world/player_script.go:158-166` —
    sibling-shape template for the 3 new methods.
  - `(*Player).Slot()` at `modules/world/player.go:434` — already exists;
    only the `ActivePlayer` interface needs the method declared.
  - `requireActivePlayer` at `pkg/script/handlers_player.go:35-40` —
    template for `requireActivePlayer2`.
  - `checkCoord` at `pkg/script/handlers_npc.go:13-19` — coord validator
    used by HINT_COORD.
  - `(s *Server).runScript` at `modules/world/script.go:27-40` —
    extraction target for `buildPlayerScriptState`.
  - `(s *Server).buildNpcScriptState` at
    `modules/world/npc_script.go:225-261` — direct shape parity for the
    new `buildPlayerScriptState`.
  - `mockPlayer` at `pkg/script/runner_test.go:95` — extend per
    `mock_recorder_field_naming_check.md`.
  - `Frame` at `pkg/script/state.go:128-133` — does NOT save active-entity
    state; this is an existing NAI-11-era divergence inherited unchanged
    by NAI-39 (push-frame save/restore is out of scope).
- Validators are inline range checks (NOT exported `XxxValid` predicates
  per goscape convention).
- TS source canonical path: per `ts_source_canonical_path.md`.

## Scope

**In scope:**

1. **B1 — Encoder closure (4 of 4 remaining HintArrowEncoder.ts
   branches).**
   - `(*Player).HintCoord(offset, x, z, height int)` —
     `modules/world/player_script.go`. Bytes: `p1(offset), p2(x), p2(z),
     p1(height)`. Mirrors `HintArrowEncoder.ts:17-27` (type=2..6 TILE).
   - `(*Player).HintPlayer(slot int)` — bytes: `p1(0x0A), p2(slot), p2(0),
     p1(0)`. Mirrors `HintArrowEncoder.ts:28-32` (type=10 PL).
   - `(*Player).HintStop()` — bytes: `p1(0xFF), p2(0), p2(0), p1(0)`.
     `0xFF` is `p1(-1)` low-byte (two's-complement). Mirrors
     `HintArrowEncoder.ts:33-38` (type=-1 STOP).
   - All three follow `(*Player).HintNpc` shape (writeOut OpHintArrow with
     a 6-byte payload).
   - **Out-of-range offset handling**: TS-faithful. The `(*Player).HintCoord`
     method does NOT validate offset ∈ [2,6]; out-of-range scripts
     produce a 6-byte packet whose first byte is the bad offset. Matches
     TS encoder which writes nothing for unrecognized type; goscape
     emits a packet because the entity-method is type-uncoded. Script-
     authors are responsible for offset bounds.

2. **B2 — `ActivePlayer` interface extension.**
   - Add `Slot() int` (consumed by HINT_PL handler).
   - Add `HintPlayer(slot int)`, `HintCoord(offset, x, z, height int)`,
     `HintStop()` (consumed by the 3 new handlers).
   - Doc-comments cite the TS source line numbers.

3. **B3 — `ScriptState` substrate: `Self2` field.**
   - Add `Self2 ActivePlayer` to `pkg/script/state.go` directly below
     `Self` and `Target`.
   - Doc-comment references `PtrActivePlayer2`, the producer site
     (`buildPlayerScriptState`), and the deviation
     `NAI-39-D-ACTIVEPLAYER2-NO-OPPLAYER-PRODUCER`.

4. **B4 — `requireActivePlayer2` validator.**
   - One-liner mirroring `requireActivePlayer` at
     `pkg/script/handlers_player.go:35-40`.
   - Asserts both `s.Pointers&PtrActivePlayer2 != 0` AND `s.Self2 != nil`.
   - Direct unit tests pin both conditions per `test_passes_for_wrong_reason.md`.

5. **B5 — `buildPlayerScriptState` extraction + target dispatch.**
   - Extract the inline state-init body of `runScript` (script.go:31-38)
     into a parallel helper. Add `target any` parameter.
   - Type-switch shape mirrors `buildNpcScriptState` at
     `modules/world/npc_script.go:242-258`:
     - `nil` → no secondary pointer
     - `script.ActivePlayer` → `state.Self2 = t; Pointers |= PtrActivePlayer2`
     - `script.ActiveNpc` → `state.ActiveNpc = t; Pointers |= PtrActiveNpc`
     - `script.ActiveLoc` → `state.ActiveLoc = t; Pointers |= PtrActiveLoc`
     - `script.ActiveObj` → `state.ActiveObj = t; Pointers |= PtrActiveObj`
   - Note Player→Player produces `Self2 + PtrActivePlayer2` (TS
     `ScriptRunner.ts:84-87`), NOT `Self + PtrActivePlayer` — because
     when `self instanceof Player`, the target Player is the secondary
     not the primary.
   - `runScript` becomes a thin wrapper calling `buildPlayerScriptState`
     then `resumeOrFinish`.

6. **B6 — `runScript` signature change: thread `target` parameter.**
   - New signature: `runScript(sf, self, target, protect, intArgs, stringArgs)`.
   - Production sites (3): `modules/world/tick.go:135, 246, 291`. All
     pass `nil` for target (no producer ports activePlayer2 yet).
   - Test sites (~13 in `modules/world/script_test.go`): all pass `nil`.
   - Per `plan_doc_replaceall_timeline.md`: per-instance Edits with
     surrounding context, NOT replace_all.

7. **B7 — Three handler functions.**
   - `handleHintCoord`: `requireActivePlayer` → `popInt(height)` →
     `popInt(coord)` → `popInt(offset)` → `checkCoord(coord)` → drop
     `level` → `s.Self.HintCoord(offset, x, z, height)`. Mirrors
     `PlayerOps.ts:866-871`.
   - `handleHintPl`: `requireActivePlayer` → `requireActivePlayer2` →
     `s.Self.HintPlayer(s.Self2.Slot())`. Mirrors `PlayerOps.ts:976-978`.
   - `handleHintStop`: `requireActivePlayer` → `s.Self.HintStop()`.
     Mirrors `PlayerOps.ts:873-875`.
   - Register all three in `pkg/script/handlers.go` dispatch table.

8. **B8 — `mockPlayer` test-double extensions.**
   - Extend `pkg/script/runner_test.go:95-` with capture fields
     `hintPlayerCalls []int`, `hintCoordCalls []hintCoordCall`,
     `hintStopCalls int`, plus a `slot int` field for `Slot()`.
   - Per `mock_recorder_field_naming_check.md`: implementer reads the
     existing struct shape before naming.

9. **B9 — Close `NAI-37-D-HINTARROW-PARTIAL-ENCODER` deviation.**
   - Per `retire_deviation_grep_all_comments.md`: enumerate sites at
     spec-write time (re-grepped post-brainstorm). Three code-tree
     sites to delete:
     1. `pkg/script/handlers_player.go:842-844` — `handleHintNpc`
        function-level deviation block.
     2. `modules/world/player_script.go:154-157` — `(*Player).HintNpc`
        method-level deviation block.
     3. `pkg/io/protocol/game/server/prot.go:43-44` — `OpHintArrow`
        proto-declaration narrative ("goscape ships only the type=1 NPC
        variant at NAI-37 — tracked deviation …").
   - All three need their narrative refreshed to past-tense / present-
     state (e.g., `OpHintArrow` comment becomes "covers all 4
     HintArrowEncoder branches: NPC type=1 (NAI-37), TILE type=2..6 +
     PL type=10 + STOP type=-1 (NAI-39)"). Implementer rewrites in
     place; not a pure deletion.
   - **Doc-tree matches stay** (historical record): NAI-37 plan-doc
     lines 405/568/686/1856, NAI-37 spec:105/184. Only `pkg/` and
     `modules/` doc-comments are touched.
   - Post-commit verification: `rg
     "NAI-37-D-HINTARROW-PARTIAL-ENCODER" pkg/ modules/` returns zero
     hits.

10. **B10 — Open `NAI-39-D-ACTIVEPLAYER2-NO-OPPLAYER-PRODUCER` deviation.**
    - Tagged at `modules/world/script.go` (the `case script.ActivePlayer:`
      branch in `buildPlayerScriptState`).
    - Closure condition: lands when goscape ports the first OPPLAYER
      trigger handler. At that point: (a) verify the OPPLAYER trigger
      calls `runScript(..., target: targetPlayer, ...)`, (b) add an
      integration test exercising Player→Player → HINT_PL pipeline,
      (c) remove this deviation entry.

**Out of scope:**

- Push-frame save/restore for `Self2` (and `OtherActiveNpc`, `ActiveLoc`,
  etc). Goscape's `Frame` struct (`pkg/script/state.go:128-133`) does NOT
  save active-entity state; this is an existing NAI-11-era divergence
  inherited unchanged. A future sub-spec that broadly aligns
  pushFrame/popFrame with TS would address all *_2 slots together; doing
  it just for activePlayer2 here would be inconsistent.
- P_FINDUID handler port (TS `PlayerOps.ts:1140-1153`). The handler
  declares `_activePlayer2 = player` on success; goscape doesn't have a
  P_FINDUID handler at HEAD. Future P_FINDUID sub-spec will consume the
  rails NAI-39 lays.
- BOTH_HEROPOINTS handler (TS `PlayerOps.ts:1156-1180`). Reads `_activePlayer2`
  via the operand-toggle accessor; out of scope for HINT_PL closure.
- OPPLAYER trigger routing (the production producer for activePlayer2).
  Tracked at `NAI-39-D-ACTIVEPLAYER2-NO-OPPLAYER-PRODUCER`.
- Operand-toggle accessor pattern (TS `ScriptState.ts:223-241` —
  `activePlayer` getter swaps primary/secondary based on `intOperand`).
  Goscape handlers read `s.Self` and `s.Self2` directly; the operand
  toggle is not needed for any NAI-39 handler.
- Combat-arrow / minimap-marker variants of HintArrow not declared in TS
  at rev 225.

## Test strategy

Following `rsbuf_roundtrip_tests.md` (byte-pin every encoder branch in
client-decoder reader order), `ts_asymmetry_dual_pin.md` (pin both
presence and absence), and `test_passes_for_wrong_reason.md` (direct
helper tests, not just handler-level integration). Five test buckets:

**A. Handler unit tests (`pkg/script/handlers_player_test.go`)** — extend
the NAI-37 T6 HINT_NPC test pattern at line 2633.

| Test | Asserts |
|------|---------|
| `TestHintCoord_NoActivePlayer_Errors` | requireActivePlayer guard fires; mockPlayer.hintCoordCalls len 0 |
| `TestHintCoord_InvalidCoord_Errors` | checkCoord guard fires (negative coord) |
| `TestHintCoord_Success_RecordsArgs` | Pop order [offset=3, coord=packed(0,100,200), height=42] dispatches `(3,100,200,42)` |
| `TestHintCoord_PopOrderDistinctValues` | offset=2, coord=packed(0,1,2), height=99 → dispatch (2,1,2,99) — pins which value lands in which arg |
| `TestHintPl_NoActivePlayer_Errors` | first guard fires |
| `TestHintPl_NoActivePlayer2_Errors` | Self set + PtrActivePlayer; Self2 nil OR PtrActivePlayer2 unset → second guard fires |
| `TestHintPl_Success_RecordsSlot` | Self2.Slot() value (e.g. 7) appears in mockPlayer.hintPlayerCalls |
| `TestHintStop_NoActivePlayer_Errors` | requireActivePlayer guard fires |
| `TestHintStop_Success_IncrementsCounter` | mockPlayer.hintStopCalls == 1 |

ScriptState test fixtures need `Pointers: PtrActivePlayer | PtrActivePlayer2`,
`Self: pl`, `Self2: pl2`, plus explicit `IntStack/StringStack` init per
`scriptstate_test_fixture_idioms.md`.

**B. requireActivePlayer2 helper unit tests
(`pkg/script/handlers_player_test.go`)** — direct-helper tests so failures
aren't masked by the handler-level pop/dispatch path.

| Test | Asserts |
|------|---------|
| `TestRequireActivePlayer2_NoBit_Errors` | Self2 set, PtrActivePlayer2 unset → error |
| `TestRequireActivePlayer2_NilSelf2_Errors` | bit set, Self2 == nil → error |
| `TestRequireActivePlayer2_Both_OK` | both present → returns nil |

**C. Wire-format byte-pin tests
(`modules/world/player_script_test.go`)** — sibling of the NAI-37 T5
HintNpc byte-pin at 875e4ea. One per (*Player) method, each pinning all
6 payload bytes against the captured wire output.

| Test | Asserts |
|------|---------|
| `TestHintCoord_PinsBytes` | offset=3, x=0x1234, z=0x5678, height=0x42 → bytes `03 12 34 56 78 42` |
| `TestHintCoord_OffsetBoundaries` | offset=2 and offset=6 both produce well-formed 6-byte packets with the offset in byte[0] |
| `TestHintPlayer_PinsBytes` | slot=0xABCD → bytes `0A AB CD 00 00 00` |
| `TestHintStop_PinsBytes` | bytes `FF 00 00 00 00 00` (the type=-1 → 0xFF asymmetry pin) |

**D. buildPlayerScriptState target-dispatch tests
(`modules/world/script_test.go`)** — direct mirror of buildNpcScriptState
target-dispatch coverage. Verifies the rails work even though no
production producer fires through them yet — closes the loop on
dual-pin (presence-of-rails).

| Test | Asserts |
|------|---------|
| `TestBuildPlayerScriptState_NilTarget` | no secondary pointer set; Self wired |
| `TestBuildPlayerScriptState_PlayerTarget` | Self2 wired + PtrActivePlayer2 set; Self unchanged (NOT overwritten) |
| `TestBuildPlayerScriptState_NpcTarget` | ActiveNpc wired + PtrActiveNpc set |
| `TestBuildPlayerScriptState_LocTarget` | ActiveLoc wired + PtrActiveLoc set |
| `TestBuildPlayerScriptState_ObjTarget` | ActiveObj wired + PtrActiveObj set |

**E. mockPlayer extensions (`pkg/script/runner_test.go`)** — extend the
existing mockPlayer at line 95 with capture fields and method impls. Per
`mock_recorder_field_naming_check.md`, implementer reads the existing
struct shape before naming new fields. Reference shape:

```go
type hintCoordCall struct{ offset, x, z, height int }

// inside mockPlayer:
hintPlayerCalls []int
hintCoordCalls  []hintCoordCall
hintStopCalls   int
slot            int

func (m *mockPlayer) Slot() int                 { return m.slot }
func (m *mockPlayer) HintPlayer(s int)          { m.hintPlayerCalls = append(m.hintPlayerCalls, s) }
func (m *mockPlayer) HintCoord(o, x, z, h int)  { m.hintCoordCalls = append(m.hintCoordCalls, hintCoordCall{o, x, z, h}) }
func (m *mockPlayer) HintStop()                 { m.hintStopCalls++ }
```

## Deviations & follow-ups

**Closing**: `NAI-37-D-HINTARROW-PARTIAL-ENCODER`. Plan stage B9 retires
the tag at three code-tree sites (`pkg/script/handlers_player.go:842`,
`modules/world/player_script.go:154`,
`pkg/io/protocol/game/server/prot.go:44`) — each refreshed to a present-
state narrative. Per `retire_deviation_grep_all_comments.md`, post-commit
verification: `rg "NAI-37-D-HINTARROW-PARTIAL-ENCODER" pkg/ modules/`
returns zero hits.

**Opening**: `NAI-39-D-ACTIVEPLAYER2-NO-OPPLAYER-PRODUCER`. Tagged in
`modules/world/script.go` at the `case script.ActivePlayer:` branch of
`buildPlayerScriptState`. Closure when OPPLAYER triggers are ported.

**Inherited (unchanged)**: NAI-11-era divergence on `Frame` struct
omitting active-entity state — applies to `Self2` as it does to
`OtherActiveNpc`, `ActiveLoc`, `ActiveObj`. Not a NAI-39 deviation;
flagged here for crosscheck.

## Cadence

Full cadence per `runescript_cadence.md`: brainstorm → spec (this doc) →
plan (writing-plans skill) → subagent-driven TDD with two-stage review.

LOC budget (per Section 6 of brainstorm): ~210 LOC production, ~305 LOC
test, ~515 LOC gross. Medium sub-spec — past the user-quoted 80-120 LOC
estimate; the activePlayer2 substrate accounts for ~130 of the 210
production lines.

## Outcomes

After NAI-39 closes:

- Every TS `HintArrowEncoder.ts` branch (type=1, 2..6, 10, -1) has a
  goscape implementation.
- HINT_NPC, HINT_COORD, HINT_PL, HINT_STOP all execute TS-faithfully
  from PlayerOps cache scripts.
- Goscape gains an activePlayer2 slot at the script-state layer with the
  exec-trigger seed (target-dispatch in `buildPlayerScriptState`) wired,
  ready for OPPLAYER trigger ports / P_FINDUID port / BOTH_HEROPOINTS port.
- One deviation closes (`NAI-37-D-HINTARROW-PARTIAL-ENCODER`); one opens
  (`NAI-39-D-ACTIVEPLAYER2-NO-OPPLAYER-PRODUCER`).

## HEAD baseline

`e3ed6e1` (NAI-38 close).
