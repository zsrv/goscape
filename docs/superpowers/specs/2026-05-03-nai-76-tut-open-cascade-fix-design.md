# NAI-76 — Investigation+fix: TUT_OPEN handler port to silence login error and unblock tutorial chatnpc cascade

- **Sub-spec**: NAI-76
- **Date**: 2026-05-03
- **Scope label**: Investigation-and-fix sub-spec — Stage 1 short-circuited at brainstorm time (root cause concrete and grep-confirmed: `pkg/script/opcode.go:222,851` declares `OpTutOpen Opcode = 2122` plus the `case OpTutOpen: return "TUT_OPEN"` name entry, but `pkg/script/handlers.go` has no entry registering a handler for opcode 2122; the runtime emits `WARN script execute error script=[proc,tutorialstep_page] err="no handler for TUT_OPEN (opcode 2122) at pc=112"` on every login). Stage 2 = single port task: handler in `pkg/script`, `OpenTutorial(int)` method on `*world.Player`, `OpTutOpen` opcode in `pkg/io/protocol/game/server/prot.go`, `modalStateTut` bit in `modules/world/player.go`, encodeOut diff-check emit branch, ActivePlayer interface extension. User-mediated Java-client smoke (per `smoke_test_server_handoff.md`) as binding feature-correctness gate. Smoke decision tree (§7) routes residual symptoms (door visual / click-away modal dismiss) into NAI-77+ if not cascade-resolved.
- **Predecessors**: NAI-75 (SPLIT_* opcode port) — last on `main` as `0b2a394`
- **Source root**:
  - `LostCityRS/Engine-TS` (TS canonical for `pkg/script` and `modules/world` per `ts_source_canonical_path.md`)
    - `src/engine/script/handlers/PlayerOps.ts:723-725` — TUT_OPEN handler shape
    - `src/engine/entity/Player.ts:1999-2003` — `openTutorial(com)` method
    - `src/engine/script/ScriptOpcodePointers.ts:171-173` — `require: ['active_player']` gate
    - `src/network/game/server/codec/TutOpenEncoder.ts` — wire encoder (`p2(component)`)
    - `src/network/game/server/ServerGameProt.ts:25` — `TUT_OPEN = (185, 2)` opcode + payload size
  - `LostCityRS/Content/scripts/tutorial/scripts/tutorialstep.rs2:1-33` — `[proc,tutorialstep_page]` consumer (the failing proc; calls `tut_open($interface)` at line 33)
  - `LostCityRS/Content/scripts/tutorial/scripts/tut_doors_and_gates.rs2:39-50` — RS Guide door's `[oploc1,newbie_door1]`, downstream of which `~tutorial_step_moving_around` → `~tutorialstep` → `~tutorialstep_page` → `tut_open` is invoked. Cascade evidence.
  - `LostCityRS/Content/scripts/tutorial/scripts/tut_chatbox_steps.rs2:22-24` — `[proc,tutorial_step_moving_around]`

## Motivation

NAI-75 closed cleanly with the SPLIT_* opcode port (chatnpc dialog rendering unblocked). The user ran a re-smoke against HEAD `0b2a394`. Three symptoms surfaced or remained:

1. **TUT_OPEN error log**: every login emits
   ```
   WARN script execute error script=[proc,tutorialstep_page] err="script \"[proc,tutorialstep_page]\": no handler for TUT_OPEN (opcode 2122) at pc=112"
   ```
   This proc was previously a silent no-op because `[proc,tutorialstep](…)` calls `split_init(…)` then iterates `while ($page < $pagetotal) { ~tutorialstep_page($page); … }` — and pre-NAI-75 `split_pagecount` returned 0, so the loop iterated zero times and `tutorialstep_page` never fired. Post-NAI-75 the loop iterates correctly and immediately hits the missing TUT_OPEN handler.

2. **RS Guide wooden door interaction**: clicking the door, the player walks to it, walks past it (likely `p_teleport(loc_coord)` from `~open_and_close_door`), and then no expected `~tutorial_step_moving_around` chatbox appears. The door also does not appear visually opened. Hypothesis: cascade — `[oploc1,newbie_door1]` (`tut_doors_and_gates.rs2:39-50`) advances `%tutorial`, calls `~tutorial_step_moving_around`, which calls `~tutorialstep`, which calls `tut_open` and aborts. The door visual + walkthrough may complete pre-error; the chatbox never opens.

3. **Click-away modal dismiss**: with a chatnpc dialog open, clicking ground tiles to walk does not dismiss the dialog. Status uncertain re: cascade attribution; may be a chatnpc-flow-aborted artifact rather than a closeModal-on-movement gap.

The TUT_OPEN error is loud, concrete, and grep-confirmed. Per `investigation_subspec_cadence.md` the Stage-1 audit was already complete at brainstorm. NAI-76 ships the port as a single Stage 2 task and uses smoke to characterize whether symptoms 2 and 3 cascade-resolve.

## Stage 1 short-circuit evidence

Re-grep at brainstorm time against HEAD `0b2a394`:

- `pkg/script/opcode.go:222`: `OpTutOpen             Opcode = 2122`
- `pkg/script/opcode.go:851`: `case OpTutOpen:` → `return "TUT_OPEN"`
- `pkg/script/handlers.go`: no entry for `OpTutOpen` in dispatch map (controller pre-flight verified per `controller_preflight.md`).
- `modules/world/player.go:246`: `modalTutorial int` field present.
- `modules/world/player.go:431`: `modalTutorial: -1` init present in `newPlayer`.
- `modules/world/player_interface.go:157`: `IsComponentVisible` already uses `modalTutorial` (without bitmask gate; per existing TS-asymmetry decision — NOT touched in NAI-76).
- `modules/world/player_script.go`: `OpenMain/OpenChat/OpenSide/OpenMainSide` exist; **no** `OpenTutorial` method.
- `pkg/io/protocol/game/server/prot.go`: `OpIfOpenMain (168, 2)`, `OpIfOpenChat (14, 2)`, `OpIfOpenSide (195, 2)`, `OpIfOpenMainSide (28, 4)`, `OpIfClose (129, 0)` exist; **no** `OpTutOpen`.

Conclusion: Stage 1 short-circuit conclusive. No subagent audit dispatch needed.

## Architecture

Seven touch points across three packages, mirroring the TS reference one-for-one.

### A — Wire protocol (`pkg/io/protocol/game/server/prot.go`)

Add:

```go
OpTutOpen        = Op{Opcode: 185, PayloadSize: 2}
```

Source: TS `ServerGameProt.TUT_OPEN = new ServerGameProt(185, 2)` at `ServerGameProt.ts:25`.

Insertion order: alphabetically-adjacent to existing `OpIfOpenMain` etc., or grouped at end of file — match the existing convention at HEAD.

### B — modalState bit (`modules/world/player.go:35-38`)

Add:

```go
modalStateTut  = 0x8
```

Independent bit (not mutex with main/chat/side, per TS `Player.ts:2001` `this.modalState |= ModalState.TUT`).

### C — `lastModalTutorial` field + init (`modules/world/player.go`)

Add `lastModalTutorial int` adjacent to existing `lastModalMain, lastModalChat, lastModalSide` declarations (~line 244).

Init `-1` in `newPlayer` adjacent to existing `modalTutorial: -1,` (~line 431).

### D — `encodeOut` tutorial-emit branch (`modules/world/player.go::encodeOut`, ~line 327)

After the existing main/chat/side switch (post-line 363), add:

```go
if p.modalTutorial != p.lastModalTutorial {
    payload := []byte{byte(p.modalTutorial >> 8), byte(p.modalTutorial)}
    p.writeOut(gameserver.OpTutOpen, payload)
    p.lastModalTutorial = p.modalTutorial
}
```

Diff-check pattern (mirrors `lastModalMain/Chat/Side` change-detection at lines 328-345). No new refresh flag needed. Handles open (com=N) and close (com=-1) symmetrically; the close path is inert in NAI-76 because no public API mutates `modalTutorial` back to -1, but the wire shape is correct for future TUT_CLOSE (deferred — see §6).

### E — `Player.OpenTutorial` method (`modules/world/player_script.go`, after `OpenMainSide`)

```go
// OpenTutorial sets the player's tutorial-overlay component. Per TS,
// opening the tutorial does NOT close any other modal — the TUT bit is
// OR'd into modalState. Mirrors LostCityRS/Engine-TS Player.ts:1999-2003
// (openTutorial). The packet write is deferred to the next encodeOut
// pass, which detects the modalTutorial != lastModalTutorial diff.
func (p *Player) OpenTutorial(com int) {
    p.modalTutorial = com
    p.modalState |= modalStateTut
}
```

Doc comment lists three TS-reference anchors per existing project convention.

### F — ActivePlayer interface extension (`pkg/script/active_player.go` or matching file)

Add `OpenTutorial(com int)` method to the `ActivePlayer` interface. Implementer: `*world.Player` (via §E). Plan-author MUST grep the actual interface filename + member naming convention before codifying — per `plan_grep_helper_patterns.md`.

### G — Opcode handler + dispatch registration (`pkg/script/handlers_player.go` or matching shard)

```go
// handleTutOpen mirrors LostCityRS/Engine-TS PlayerOps.ts:723-725
// (TUT_OPEN). Pops the component id, rejects -1 via
// check(_, NumberNotNull), and calls ActivePlayer.OpenTutorial.
// TS reserves com=-1 for the closeTutorial path (Player.ts:716-726
// writes TutOpen(-1) directly via Player.write, not through this
// opcode) — deferred per stub_deferred_comment_marker.md.
func handleTutOpen(s *ScriptState) error {
    if s.Pointers&PtrActivePlayer == 0 || s.Self == nil {
        return errors.New("TUT_OPEN: no active player")
    }
    com := s.PopInt()
    if err := checkNotNull(com, "TUT_OPEN"); err != nil {
        return err
    }
    s.Self.OpenTutorial(com)
    return nil
}
```

Register `OpTutOpen → handleTutOpen` in the dispatch map at `pkg/script/handlers.go:285`-area (alongside the existing IF_OPEN_* entries). Active-player gate matches the local file convention at `handlers_interface.go` (inline `s.Pointers&PtrActivePlayer == 0 || s.Self == nil`, per `plan_grep_helper_patterns.md` — that file does NOT use the `requireActivePlayer` helper from `handlers_player.go:35`; mirror the local convention to avoid mixed style).

## Test strategy

### `pkg/script` handler tests (new, in matching `handlers_player_test.go`)

- `TestTutOpen` — push `com=42` via `OpPushConstantInt`/`OpTutOpen`/`OpReturn`; assert mock `OpenTutorial` recorder captured 42 (matching pattern of `TestIfOpenMain` at `handlers_interface_test.go:34-50`).
- `TestHandleTutOpenNullRejected` — push `com=-1`; assert `Execute` returns error containing `"TUT_OPEN: input number was null(-1)"`; assert mock `lastOpenTutorial == 0` (never called) (matching pattern of `TestHandleIfOpenMainNullRejected` at `handlers_interface_test.go:546-570`).
- `TestTutOpenNoActivePlayer` — set `state.Pointers = 0` before Execute; assert error contains `"TUT_OPEN: no active player"` (matching pattern of `TestIfOpenMainNoActivePlayer` at `handlers_interface_test.go:500`).

Mock recorder: add `openTutorialCalls []int` (or sibling field matching existing convention) to the mockActivePlayer struct. Plan-author MUST grep the existing mock struct field names per `mock_recorder_field_naming_check.md` before codifying.

### `modules/world` Player.OpenTutorial tests (new, in `player_script_test.go`)

- `TestOpenTutorial_SetsFieldsWithoutClosingOthers` — pre-state: `modalMain=5, modalChat=7, modalSide=9, modalState=Main|Chat|Side`. Call `OpenTutorial(42)`. Assert: `modalTutorial==42`, `modalState == Main|Chat|Side|Tut` (TS-fidelity: tutorial does NOT mutex with other modals).
- `TestOpenTutorial_RefreshFlagsUntouched` — assert `refreshModal` and `refreshModalClose` unchanged (tutorial uses lastModalTutorial diff-check, not the existing refresh flag).

### `modules/world` encodeOut tests (new branch in existing encodeOut test file)

- `TestEncodeOut_TutorialOpenEmitsOpTutOpen` — `OpenTutorial(42)`, run `encodeOut`, capture writeOut calls; assert `gameserver.OpTutOpen` emitted with payload `[0x00, 0x2A]`. After call: `lastModalTutorial==42`.
- `TestEncodeOut_TutorialNoChangeNoEmit` — second `encodeOut` with no field change → no second `OpTutOpen` emit.
- `TestEncodeOut_TutorialResetEmitsMinusOne` — set `p.modalTutorial = -1` directly (no public API yet), re-run `encodeOut`; assert `OpTutOpen` emitted with payload `[0xFF, 0xFF]`. Pins the wire shape for future TUT_CLOSE.
- `TestEncodeOut_TutorialIndependentOfMainChatSide` — open main + tutorial in same tick (`OpenMain(5)` then `OpenTutorial(42)`); run `encodeOut`; assert BOTH `OpIfOpenMain` AND `OpTutOpen` emitted in the same flush.

### Existing `IsComponentVisible_MatchesModalTutorial` (`player_interface_test.go:143-158`)

Pre-existing test; modalTutorial-by-value semantic without bit gate, mirrors a HEAD pin. **Do not touch** in NAI-76.

### Cross-package regression

`go test ./... -count=1` plus `-race` per `verify_implementer_claims.md`. Stale IDE diagnostics ignored per failure-mode-1.

## Smoke matrix + decision tree

Server launch (per `smoke_test_server_handoff.md`, user-driven):

```
CGO_ENABLED=0 go run -trimpath ./cmd/goscape --config.file config.yaml
```

**Items:**

1. **TUT_OPEN log silence** — log in fresh; grep server stderr for `TUT_OPEN`. Pass = no error line.
2. **Door interaction full path** — walk to RS Guide; advance `%tutorial` past `^newbie_basics_instructor_interact_with_scenery`; click wooden door. Pass = door visually opens, player walks through, "Moving around" tutorial chatbox appears with split content rendered.
3. **Click-away modal dismiss** — with a chatnpc dialog open, click ground tiles to walk. Pass = dialog closes on movement.

**Decision tree at smoke close:**

| Outcome | Route |
|---|---|
| 1+2+3 all pass | NAI-76 closes single-fix. Mirror NAI-75 close-commit shape. |
| 1+2 pass, 3 fail | In-scope-stretch click-away if fix ≤30 LOC (per `smoke_surfaces_adjacent_divergences.md`); else route to NAI-77. |
| 1 passes, 2 partial (walk-through but no modal) | Investigate post-tut_open wiring (likely encodeOut diff-check / OpenTutorial wiring); fix in-scope. |
| 1 passes, 2 fail (still walks past door, no chat) | Door has independent root (loc_change wire, oploc dispatch, tutorial state advance). Route to NAI-77 with the new evidence (TUT_OPEN no longer a noise source). |
| 1 fails | Port has a defect (handler not registered, popInt arity, encodeOut not emitting). Re-investigate before close. |

## Out of scope (deferred)

- **TUT_CLOSE (opcode 2120)** — declared at `pkg/script/opcode.go:220,847`, no handler. Single content site (`tutorial.rs2:297`), not loud at current smoke matrix. Defer to NAI-N+1 unless a smoke surfaces it.
- **`Player.closeTutorial()`** (TS Player.ts:716-726) — paired with TUT_CLOSE. Same deferral.
- **Tutorial-modal IF_CLOSE trigger dispatch** — pulled in only when `closeTutorial()` is ported.
- **`IsComponentVisible` bit-gate extension** — current modalTutorial-by-value semantic at `player_interface.go:157` is a HEAD pin; not touched.
- **Door visual `loc_change`** wire-out path — independent of tut_open. Routed by smoke decision-tree row 4 if it remains.

Annotation: at the dispatch table site for TUT_CLOSE registration (the missing entry adjacent to TUT_OPEN registration), add a `// deferred to later sub-spec — TUT_CLOSE port` comment per `stub_deferred_comment_marker.md` so future grep enumerates this surface.

## Risk register

- **R1 — `IsComponentVisible` semantics drift.** Existing test at `player_interface_test.go:143-158` pins modalTutorial-by-value without bit-gate. **Mitigation:** §3 explicit non-touch decision; this sub-spec does not extend `IsComponentVisible`.
- **R2 — `lastModalTutorial` init.** Default `0` would cause spurious `OpTutOpen(0)` emit on first encodeOut. **Mitigation:** init `-1` in `newPlayer` (§C), mirroring `modalTutorial`'s init at `player.go:431`.
- **R3 — `NumberNotNull` parity.** TS wraps the popInt in `check(…, NumberNotNull)` which throws on `com == -1`. Goscape mirrors with `checkNotNull(com, "TUT_OPEN")` (helper at `pkg/script/handlers_player.go:71-76`). **Test pin:** `TestHandleTutOpenNullRejected` matching the pattern of `TestHandleIfOpenMainNullRejected` at `handlers_interface_test.go:546-570`. No divergence opened.
- **R4 — TUT_CLOSE absence.** Smoke matrix doesn't exercise tutorial completion. Risk = future smoke surfaces it. **Mitigation:** stub_deferred annotation at the dispatch site.
- **R5 — Door visual loc_change unrelated.** Door not opening visually may be a separate `loc_change` wire-out gap, not downstream of tut_open. **Mitigation:** smoke decision-tree row 4 catches this and routes to NAI-77.
- **R6 — Cascade theory wrong on click-away.** Click-away modal dismiss may not cascade-resolve from tut_open port — could be a closeModal-on-movement integration gap independent of tutorial. **Mitigation:** smoke decision-tree row 2 routes to in-scope stretch (≤30 LOC) or NAI-77, per `smoke_surfaces_adjacent_divergences.md`.

## Deviations opened by this sub-spec

None expected. The port is straight-line TS-fidelity. R3's defensive label is documentation, not a tracked deviation.

## Net deviation tally projection

14 → 14 (no opens, no closes). Tutorial-side cleanup deferred to NAI-77+ if smoke routes work there.

## Cadence

Investigation-and-fix variant per `runescript_cadence.md`. Single Stage-2 task (the port). Smoke handoff per `smoke_test_server_handoff.md`. Close commit memory trailer per `close_commit_memory_trailer.md`. Subagent-driven-development per `execution_mode_default.md`. Plan author follows up via `superpowers:writing-plans` immediately after this spec is approved.
