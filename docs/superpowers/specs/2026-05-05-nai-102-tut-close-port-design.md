## NAI-102: TUT_CLOSE handler + Player.closeTutorial() port

**Date**: 2026-05-05
**Cadence**: combined spec + plan, single end-of-impl review (per
`compressed_cadence.md`, ≤15 production-LOC threshold; ~10 production
LOC).
**Predecessor**: NAI-101 (HEAD `9b92ee8` — queueWaypoints TS-faithful
reverse-copy; Lumbridge fountain path-around chain
NAI-96→98→100→101 closed).
**Trigger**: NAI-76 spec §6 R4 deferral + `stub_deferred_comment_marker.md`
grep at `pkg/script/handlers.go:297` (`// OpTutClose: deferred to later
sub-spec — TUT_CLOSE handler port + Player.closeTutorial() method.
See NAI-76 spec §5 R4.`).
**Tech stack**: Go 1.26+ (per `go_version.md`).
**Successor**: TBD; no residuals expected (no-content-smoke port; full
TS fidelity).

### 1. Problem

`TUT_CLOSE` (opcode 2120) is declared at `pkg/script/opcode.go:220,847`
with no handler registration; the dispatch table at
`pkg/script/handlers.go:297` carries the deferred-stub annotation. The
matching `Player.closeTutorial()` method (TS Player.ts:716-726) is
absent from `modules/world/player_script.go`. The wire side was already
plumbed in NAI-76: setting `p.modalTutorial = -1` triggers the diff at
`modules/world/player.go:388-391` which emits `OpTutOpen` with payload
`[0xFF, 0xFF]` (signed -1 → uint16 0xFFFF), pinned by
`TestEncodeOut_TutorialResetEmitsMinusOne` (`modules/world/player_test.go:836-879`).

The single in-engine surface this opcode hits is
`tutorial.rs2:297` (per NAI-76 §6 R4); current smoke matrix doesn't
exercise tutorial completion, so this is pure tech-debt closure of an
NAI-76-era deferral.

### 2. TS source (canonical, single read)

**`Engine-TS/src/engine/entity/Player.ts:716-726`** — `closeTutorial`:

```typescript
closeTutorial() {
    if (this.modalTutorial !== -1) {
        const closeTrigger = ScriptProvider.getByTrigger(ServerTriggerType.IF_CLOSE, this.modalTutorial);
        if (closeTrigger) {
            this.executeScript(ScriptRunner.init(closeTrigger, this), false);
        }

        this.modalTutorial = -1;
        this.write(new TutOpen(-1));
    }
}
```

**`Engine-TS/src/engine/script/handlers/PlayerOps.ts:877-879`** — handler:

```typescript
[ScriptOpcode.TUT_CLOSE]: state => {
    state.activePlayer.closeTutorial();
},
```

**Observations driving the port:**
- TS `closeTutorial` does **not** call `clearComListeners(this.modalTutorial)`
  (contrast with TS `closeModal` which does call `clearComListeners` on
  each main/chat/side slot before resetting). Goscape mirrors:
  `CloseTutorial` does **not** call `clearComListeners`.
- TS `closeTutorial` does **not** touch a "modal-state bitmap" — TS has
  no such field. Goscape has a goscape-internal `modalState` bitmap
  (`modules/world/player.go:36-40`) where `OpenTutorial` does
  `p.modalState |= modalStateTut` (`modules/world/player_script.go:790`).
  To keep the bitmap symmetric with the field, `CloseTutorial` clears
  `modalStateTut`. (Goscape-internal abstraction; labelled as such in
  the doc-comment per `defensive_gate_doc_comment_label.md`.)
- The `this.write(new TutOpen(-1))` line in TS is implicit in goscape:
  the diff-check at `modules/world/player.go:388-391` already emits
  `OpTutOpen` with `[0xFF, 0xFF]` whenever `modalTutorial` transitions
  to -1. No explicit `writeOut` call needed in `CloseTutorial`.

### 3. Solution

#### 3.1 Production changes

**(P1)** `pkg/script/active.go` — add `CloseTutorial()` method to the
`Self` interface, sited adjacent to `OpenTutorial(com int)` at line 180:

```go
// CloseTutorial closes any currently-open tutorial overlay. Per TS,
// this is a no-op when no tutorial is open; otherwise it dispatches
// the matching IF_CLOSE trigger script (if registered) and resets
// the tutorial slot. Mirrors LostCityRS/Engine-TS Player.closeTutorial
// (Player.ts:716-726).
CloseTutorial()
```

**(P2)** `pkg/script/handlers_interface.go` — add `handleTutClose`
mirroring `handleIfClose` shape:

```go
// handleTutClose implements TUT_CLOSE.
// TS PlayerOps.ts:877-879 — no pops; just delegates to closeTutorial().
func handleTutClose(s *ScriptState) error {
    if s.Pointers&PtrActivePlayer == 0 || s.Self == nil {
        return errors.New("TUT_CLOSE: no active player")
    }
    s.Self.CloseTutorial()
    return nil
}
```

**(P3)** `pkg/script/handlers.go:297` — replace the deferred-stub
comment with the dispatch table entry:

```go
OpTutOpen:        handleTutOpen,
OpTutClose:       handleTutClose,
```

(Drop the entire `// OpTutClose: deferred ...` and `// + Player.closeTutorial()
method. See NAI-76 spec §5 R4.` two-line comment block.)

**(P4)** `modules/world/player_script.go` — add `(*Player).CloseTutorial`
method, sited adjacent to `OpenTutorial` at line 788:

```go
// CloseTutorial closes the player's tutorial overlay. Per TS:
// no-op if no tutorial open; otherwise dispatches the matching
// IF_CLOSE trigger (if registered) for the current modalTutorial
// component, then resets modalTutorial to -1. The wire OpTutOpen(-1)
// emission is implicit via encodeOut's diff-check at
// player.go:388-391 (NAI-76 pin).
//
// TS does NOT call clearComListeners(modalTutorial) here (contrast
// with closeModal); we mirror that absence.
//
// Clears modalStateTut on the goscape-internal modalState bitmap
// (goscape-internal; TS has no equivalent field). Labelled per
// defensive_gate_doc_comment_label.md.
//
// Mirrors LostCityRS/Engine-TS Player.closeTutorial (Player.ts:716-726).
func (p *Player) CloseTutorial() {
    if p.modalTutorial == -1 {
        return
    }
    if p.client != nil && p.client.server != nil {
        p.runIfCloseTrigger(p.client.server, p.modalTutorial)
    }
    p.modalTutorial = -1
    p.modalState &^= modalStateTut
}
```

**(P5)** `pkg/script/runner_test.go` — extend `mockPlayer`:
- Add field `lastCloseTutorialCalls int` adjacent to `lastOpenTutorial`
  at line 163.
- Implement `func (m *mockPlayer) CloseTutorial() { m.lastCloseTutorialCalls++ }`
  adjacent to `OpenTutorial` at line 440.

#### 3.2 Test changes

**(T1)** `pkg/script/handlers_interface_test.go` — three new tests
mirroring `TestTutOpen` / `TestTutOpenNoActivePlayer` shapes
(handlers_interface_test.go:1129-1191):

- `TestTutClose` — script `[OpTutClose, OpReturn]`, run with
  `Pointers: PtrActivePlayer` and `Self: mp`; assert
  `mp.lastCloseTutorialCalls == 1`.
- `TestTutCloseNoActivePlayer` — same script, but with
  `Pointers: 0` (or `Self: nil`); assert error message contains
  `"TUT_CLOSE: no active player"` and `mp.lastCloseTutorialCalls == 0`.

**(T2)** `modules/world/modal_close_test.go` — three new unit tests
adjacent to the existing `TestCloseModalIfCloseDispatch*` family at
line 299. (Sited here, not in `player_script_test.go` near the
`TestOpenTutorial_*` tests, because the close-family lives in this
file.) Mirror the existing `TestCloseModalIfCloseDispatchMain` shape
(modal_close_test.go:299-329) for IF_CLOSE script registration:

- `TestCloseTutorial_EarlyReturnsWhenNoTutorialOpen` — fresh player
  (modalTutorial = -1 from `newPlayer` default at player.go:460);
  set `s.scriptProvider = script.NewProvider()` (no scripts);
  call `p.CloseTutorial()`; assert `p.modalTutorial == -1` and
  `p.modalState == modalStateNone` (TS-faithful no-op).
- `TestCloseTutorial_DispatchesIfCloseTriggerAndResets` — register
  an `[if_close,42]` `ScriptFile` (LookupKey =
  `script.LookupKeyForType(script.TriggerIfClose, 42)`,
  Opcodes = `[OpReturn]`) on the test server's scriptProvider per
  the modal_close_test.go:303-310 idiom; set `p.modalTutorial = 42`
  and `p.modalState |= modalStateTut`; call `p.CloseTutorial()`;
  assert `p.modalTutorial == -1`, `p.modalState&modalStateTut == 0`,
  and `p.activeScript == nil` (OpReturn-only IF_CLOSE script finishes
  immediately, mirroring modal_close_test.go:326-328).
- `TestCloseTutorial_NoIfCloseTriggerStillResets` — same setup but
  with `script.NewProvider()` carrying no registered script; assert
  `p.modalTutorial == -1` and `p.modalState&modalStateTut == 0`
  (silent no-op via `runScript`'s nil-safe ScriptFile handling).

**(T3)** `modules/world/player_test.go` — add one new test
adjacent to `TestEncodeOut_TutorialResetEmitsMinusOne` at line 836:

- `TestEncodeOut_CloseTutorialEmitsMinusOne` — opens tutorial via
  `p.OpenTutorial(42)`, drains the open-emission encodeOut pass,
  then calls `p.CloseTutorial()` and re-runs encodeOut; assert
  `OpTutOpen` packet with payload `[0xFF, 0xFF]` is written. Drives
  the wire emission through the new `CloseTutorial` API instead of
  the direct `p.modalTutorial = -1` field write that
  `TestEncodeOut_TutorialResetEmitsMinusOne` uses (the older test
  is preserved unchanged — it pins the diff-check itself, not the
  API path).

### 4. Out of scope

- Other deferred stubs at `pkg/script/handlers_string.go:97`
  (SPLIT_* font-aware wrap + mesanim lookup) — depend on FontType /
  MesanimType cache config loaders; separate sub-specs.
- Tutorial-modal IF_CLOSE trigger dispatch coverage at content level
  (NAI-76 §6 R4 noted "not loud at current smoke matrix"); no smoke
  added.

### 5. Deviations introduced

**None.** Full TS-faithful port. Annotations:

- The `modalState &^= modalStateTut` is goscape-internal (no TS
  counterpart); labelled in the `CloseTutorial` doc-comment per
  `defensive_gate_doc_comment_label.md`. This is a goscape-symmetry
  cleanup, not a divergence from TS behavior (TS has no such field).

### 6. Deviations retired

- **NAI-76-R4** (TUT_CLOSE absence) — retired by P3 (handler wired) +
  P4 (closeTutorial method ported). Grep `rg "deferred to later sub-spec
  — TUT_CLOSE" pkg/ modules/` post-impl should yield zero matches; if
  a sibling reference exists, retire the comment in the same task.

### 7. Implementation plan (subagent-driven, single bundle)

Single subagent dispatch covers all changes; compressed cadence skips
formal review.

**Bundle 1: TUT_CLOSE port (single dispatch)**

Tasks for the implementer (TDD per `superpowers:test-driven-development`):

1. **T1 (TDD)**: Write `TestTutClose` + `TestTutCloseNoActivePlayer` in
   `pkg/script/handlers_interface_test.go`. Both should fail-compile
   (mockPlayer.CloseTutorial undefined). Add `lastCloseTutorialCalls
   int` + `CloseTutorial()` to mockPlayer in `pkg/script/runner_test.go`
   to make the tests compile-but-fail (handler not registered → opcode
   resolves to "unhandled" path).

2. **T2 (RED→GREEN)**: Add `handleTutClose` in
   `pkg/script/handlers_interface.go` and register it at
   `pkg/script/handlers.go:297` (replacing the deferred-stub comment).
   Add `CloseTutorial()` to the `Self` interface in
   `pkg/script/active.go` adjacent to `OpenTutorial`. Tests in T1 turn
   green.

3. **T3 (TDD)**: Write three new tests in
   `modules/world/modal_close_test.go` (adjacent to the
   `TestCloseModalIfCloseDispatch*` family at line 299):
   `TestCloseTutorial_EarlyReturnsWhenNoTutorialOpen`,
   `TestCloseTutorial_DispatchesIfCloseTriggerAndResets`,
   `TestCloseTutorial_NoIfCloseTriggerStillResets`. Tests must
   fail-compile (Player.CloseTutorial undefined).

4. **T4 (RED→GREEN)**: Add `(*Player).CloseTutorial` in
   `modules/world/player_script.go` adjacent to `OpenTutorial`. T3
   tests turn green.

5. **T5 (TDD+impl, paired)**: Add
   `TestEncodeOut_CloseTutorialEmitsMinusOne` in
   `modules/world/player_test.go`. Should pass first try (T4 already
   reset modalTutorial; encodeOut diff at player.go:388-391 emits
   the wire packet). If it doesn't, the issue is in the test fixture,
   not the production code — debug accordingly.

6. **T6 (verification)**: Run `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache
   go test ./pkg/script/... ./modules/world/...`. Run
   `rg "deferred to later sub-spec — TUT_CLOSE" pkg/ modules/` and
   confirm zero matches. Run
   `rg "// OpTutClose:" pkg/script/handlers.go` and confirm the
   comment block is fully removed (no residual stale annotation).
   Verify NAI-76's `TestEncodeOut_TutorialResetEmitsMinusOne` still
   passes unchanged.

7. **T7 (close commit)**: Single chore(close) commit with body listing
   retired deviation (NAI-76-R4) and `Closes memory:` trailer for any
   memory entries this sub-spec retires. Per close-commit
   memory-trailer convention: nothing to close (no NAI-102-specific
   memory entry yet); the trailer should reference no memory keys
   unless the close surfaces residuals.

### 8. Risk register

- **R1 — `runIfCloseTrigger` nil-safety** [GREEN]. Pre-flight verified
  at `modules/world/player_script.go:734-740`: `runIfCloseTrigger`
  early-returns if `s.scriptProvider == nil`, and `s.runScript`
  is itself nil-safe on the returned ScriptFile (per NAI-51 T2.1
  conclusion + `plan_sibling_site_guard_audit.md`). The
  `p.client != nil && p.client.server != nil` guard in
  `(*Player).CloseTutorial` mirrors the same pattern at
  `(*Player).CloseModal` (player_script.go:700).

- **R2 — encodeOut diff fires only on transition** [GREEN]. Pre-flight
  verified at `modules/world/player.go:388-391`. Guard is
  `p.modalTutorial != p.lastModalTutorial`; sequence is "open(42) →
  encodeOut drains → close → encodeOut emits -1". T5 fixture must
  drain the open-pass before calling close (mirror
  `TestEncodeOut_TutorialResetEmitsMinusOne` fixture shape).

- **R3 — modalState&modalStateTut clearing has no read site**
  [INFO/AUDITED]. `rg "modalStateTut"` returns only the constant decl
  + `OpenTutorial`'s `|=` write. Clearing on close is symmetric
  hygiene, not a behavioral fix. Future readers (e.g., a future
  bitmap-driven refresh-flag site) will see the bit transition
  correctly.

### 9. Verification before close

- `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...` clean.
- `rg "deferred to later sub-spec — TUT_CLOSE" pkg/ modules/` → 0 matches.
- `rg "OpTutClose" pkg/ modules/` → registers in
  `pkg/script/handlers.go` (dispatch table), `pkg/script/opcode.go`
  (decl), and any bytecode test fixtures using it. No stale
  deferred-stub comments.
- `git show HEAD --stat` matches stated bundle scope; no stray
  worktree writes (per `feedback_subagent_wt_path.md`).

### 10. Notes

This is a textbook compressed-cadence sub-spec: ~10 production LOC,
no novel infrastructure, no new content surfaces, no smoke. The TS
source is 11 lines and the goscape port is 11 lines. Fidelity is
verified by the `Self.OpenTutorial` / `Self.CloseTutorial` interface
parity (each modal-family method has open + close; tutorial finally
joins).
