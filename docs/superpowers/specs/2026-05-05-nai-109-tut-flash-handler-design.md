# NAI-109 — TUT_FLASH script-opcode handler + wire packet

**Date:** 2026-05-05
**Status:** Spec
**Cadence:** Single-bundle subagent-driven-development on Sonnet (per `execution_mode_default.md`); end-of-bundle review on Sonnet (per `superpowers_code_reviewer_model.md`).
**Tech stack:** Go 1.26+ (per `go_version` memory).

---

## 1. Scope

Wire goscape's `OpTutFlash` (script opcode 2121) and the matching server→client wire packet (`ServerGameProt.TUT_FLASH = 126`). Pure additive port — no behavior change to any existing code path.

**In scope:**
- New script handler `handleTutFlash` registered at `pkg/script/handlers.go`.
- New `ActivePlayer.FlashTutorial(tab int)` interface method.
- New `(*Player).FlashTutorial` impl with direct (non-deferred) wire write.
- New `OpTutFlash = Op{Opcode: 126, PayloadSize: 1}` server packet.
- Tests at handler level (3 cases) and wire level (1 case).

**Out of scope:**
- The `[label,tutorial_complete]` `P_TELEJUMP: script not protected` runtime error. Routed to NAI-110 (investigation sub-spec) per brainstorm split. Root cause not yet bound; suspected to be CloseModal ordering on dialog-choice resume vs. a TS-divergent protection requirement on `p_telejump`.
- Other unhandled opcodes in the 90-declared-but-unhandled set (the Explore sweep confirmed no other tutorial-referenced opcodes are in that set today).

---

## 2. Why

Tutorial Island runtime emits:

```
script execute error  script=[proc,tutorial_step_view_inventory]
err="script \"[proc,tutorial_step_view_inventory]\": no handler for TUT_FLASH (opcode 2121) at pc=11"
```

`tut_flash(tab)` is called 12+ times across `2004scape/Server/data/src/scripts/tutorial/scripts/tut_chatbox_steps.rs2` (game-options tab, inventory tab, skills tab, music tab, prayer tab, magic tab, friends/ignore tabs, worn-items tab, combat-options tab, quest-journal tab, player-controls tab). Each unhandled call aborts its enclosing proc, which in turn aborts whichever tutorial step invoked it. Wiring this opcode unblocks the chatbox-driven tutorial-step UX.

**Why:** Tutorial Island progression is currently the user's load-bearing smoke driver; this is the lowest-friction unblock available.
**How to apply:** This is the only currently-called-but-unhandled opcode on tutorial paths; other 90 unhandled opcodes have no current consumer.

---

## 3. TS reference (canonical port source)

Per `ts_source_canonical_path` memory: `LostCityRS/Engine-TS` only.

**Script handler — `src/engine/script/handlers/PlayerOps.ts:694-696`:**
```typescript
[ScriptOpcode.TUT_FLASH]: checkedHandler(ActivePlayer, state => {
    state.activePlayer.write(new TutFlash(check(state.popInt(), NumberNotNull)));
}),
```

**Wire encoder — `src/network/game/server/codec/TutFlashEncoder.ts`:**
```typescript
prot = ServerGameProt.TUT_FLASH;  // (126, 1) per ServerGameProt.ts:24
encode(buf: Packet, message: TutFlash): void {
    buf.p1(message.tab);
}
```

**Model — `src/network/game/server/model/TutFlash.ts`:** `class TutorialFlashSide { readonly tab: number }`.

**Pointer gate:** `checkedHandler(ActivePlayer, ...)` → requires `PtrActivePlayer`; does NOT require `ProtectedActivePlayer`. No protect-flag interaction.

**Argument validation:** `check(tab, NumberNotNull)` rejects only the `-1` sentinel; no upper bound. Goscape's `^tab_*` constants are non-negative single-byte tab indices, so `p1` (1-byte unsigned) always fits.

---

## 4. Architecture

Four additive changes; mirror the existing `TUT_OPEN` / `TUT_CLOSE` pattern (NAI-76 / NAI-102 era) except wire write is direct, not deferred.

### 4.1 Wire opcode declaration

`pkg/io/protocol/game/server/prot.go` — alongside the existing `OpTutOpen = Op{Opcode: 185, PayloadSize: 2}` block:

```go
OpTutFlash = Op{Opcode: 126, PayloadSize: 1}
```

### 4.2 ActivePlayer interface method

`pkg/script/active.go` — append after the `CloseTutorial()` declaration (after line 188):

```go
// FlashTutorial directs the client to flash the named tab to draw the
// player's attention to it. Fire-and-forget: writes a single
// TUT_FLASH server packet with the tab byte and returns. Mirrors
// LostCityRS/Engine-TS PlayerOps.ts:694-696 + TutFlashEncoder.ts.
FlashTutorial(tab int)
```

### 4.3 (*Player) implementation

`modules/world/player_script.go` — alongside the existing `OpenTutorial` / `CloseTutorial` methods (around line 782):

```go
// FlashTutorial implements script.ActivePlayer.FlashTutorial. Writes
// a TUT_FLASH server packet (opcode 126, 1-byte tab payload). Direct
// write — TUT_FLASH is fire-and-forget UI hint, not a modal-state
// transition like TUT_OPEN, so no deferred-flush pathway. Mirrors
// LostCityRS/Engine-TS Player.write(new TutFlash(tab)) call from
// PlayerOps.ts:694-696.
func (p *Player) FlashTutorial(tab int) {
    p.writeOut(gameserver.OpTutFlash, []byte{byte(tab)})
}
```

No `client == nil` guard — matches goscape's existing direct-writer convention (`(*Player).CamReset` at `player_script.go:189-191`, `HintNpc` at `player_script.go:201-209`, `WriteEnableTracking` at `player.go:416-418`). Callers ensure `p.client` is non-nil; `writeOut` (`player.go:396`) does not nil-guard either.

### 4.4 Script handler + registry

**Handler** — `pkg/script/handlers_interface.go`, after `handleTutClose` (after line 111):

```go
// handleTutFlash implements TUT_FLASH.
// TS PlayerOps.ts:694-696 — pops a single int (tab); check(tab,
// NumberNotNull). No protect gate (TS uses checkedHandler(ActivePlayer,
// ...), not ProtectedActivePlayer). Fire-and-forget — writes a
// TUT_FLASH server packet to draw the player's attention to the
// named tab.
func handleTutFlash(s *ScriptState) error {
    if s.Pointers&PtrActivePlayer == 0 || s.Self == nil {
        return errors.New("TUT_FLASH: no active player")
    }
    tab := s.PopInt()
    if err := checkNotNull(tab, "TUT_FLASH"); err != nil {
        return err
    }
    s.Self.FlashTutorial(tab)
    return nil
}
```

**Registry** — `pkg/script/handlers.go`, in the dispatch map between `OpTutOpen` (line 297) and `OpTutClose` (line 298):

```go
OpTutOpen:        handleTutOpen,
OpTutClose:       handleTutClose,
OpTutFlash:       handleTutFlash,
```

Order chosen to keep the three TUT_* siblings adjacent.

---

## 5. Test strategy

### 5.1 mockPlayer extension (`pkg/script/runner_test.go`)

Add to `mockPlayer` struct (paralleling `lastOpenTutorial int` + `lastCloseTutorialCalls int`):

```go
lastFlashTutorial      int  // last tab argument
lastFlashTutorialCalls int  // total invocation count
```

Method:

```go
func (m *mockPlayer) FlashTutorial(tab int) {
    m.lastFlashTutorial = tab
    m.lastFlashTutorialCalls++
}
```

Both fields recorded — `lastFlashTutorial` lets us assert on the most recent value (single-call tests); `lastFlashTutorialCalls` lets us assert "not called" on null-rejected / no-active-player paths without false-pass when tab=0 happens to coincide with the zero-value.

### 5.2 Handler-level tests (`pkg/script/handlers_interface_test.go`)

Add three tests adjacent to the existing `TestTutOpen*` block (after line 1194):

**(a) `TestTutFlash`** — happy path. Push int `42`, dispatch `OpTutFlash`, assert `mp.lastFlashTutorial == 42` and `mp.lastFlashTutorialCalls == 1`. (Mirrors `TestTutOpen` at lines 1132-1148.)

**(b) `TestHandleTutFlashNullRejected`** — `check(tab, NumberNotNull)` gate. Push int `-1`, dispatch, assert error contains substring `"TUT_FLASH: input number was null(-1)"` and `mp.lastFlashTutorialCalls == 0`. (Mirrors `TestHandleTutOpenNullRejected` at lines 1153-1177.)

**(c) `TestTutFlashNoActivePlayer`** — pointer-gate test. Init with `self=nil`, push int `1`, dispatch, assert error and `state.Execution == Aborted`. (Mirrors `TestTutOpenNoActivePlayer` at lines 1179-1194.)

### 5.3 Wire-level test (`modules/world/player_test.go`)

Add one test paralleling the existing TutOpen wire tests (lines 786, 879, 941). Method receives a fresh `*Player` with mock client, calls `(*Player).FlashTutorial(7)`, flushes, then decodes:

- First byte: ISAAC-encrypted `(126 + isaac.GetNext()) & 0xff`
- Second byte (payload): `0x07`

Single test, single tab value; the wire shape is byte-trivial. No `client == nil` test — direct-writer convention has no such guard (see §4.3).

### 5.4 Coverage cross-check

Per `plan_test_coverage_crosscheck` memory: every test case in §5 must appear in the plan's task code blocks and run as part of the bundle.

---

## 6. Risks & validation

| Risk | Mitigation |
|------|------------|
| Wire byte shape wrong | TS encoder is single `p1(tab)`; decoded byte must equal exact tab value (no width reinterpretation). Wire-level test pins this. |
| TS `NumberNotNull` semantics differ from goscape `checkNotNull` | TUT_OPEN already uses identical pattern at `handlers_interface.go:96` with confirmed error string `"TUT_OPEN: input number was null(-1)"`; identical wrapper for TUT_FLASH. |
| `checkedHandler(ActivePlayer, ...)` vs `ProtectedActivePlayer` confusion | TS uses `ActivePlayer` (no protect gate). Spec calls this out at §3 and §4.4 to prevent accidental over-gating. |
| Tab argument out-of-range (>255) | Single-byte `p1` would silently truncate; TS does the same. Match TS — no extra range check. Document in handler comment. |
| `(*Player).FlashTutorial` `client==nil` guard divergence | Match the no-guard convention of existing direct-writers (`CamReset`, `HintNpc`, `WriteEnableTracking`). `writeOut` itself does not nil-guard. |

**Verified premises (controller pre-flight `controller_preflight`):**
- `OpTutFlash = 2121` declared at `pkg/script/opcode.go:221`. ✓
- Handler map at `pkg/script/handlers.go:297-298` registers TutOpen + TutClose only; no TutFlash entry. ✓
- `ActivePlayer` interface at `pkg/script/active.go:6`; `OpenTutorial` at line 181, `CloseTutorial` at line 188. ✓
- `mockPlayer` impl at `pkg/script/runner_test.go:443-444`. ✓
- `(*Player).OpenTutorial` at `modules/world/player_script.go:788`; `CloseTutorial` at line 808. ✓
- `OpTutOpen = {185, 2}` at `pkg/io/protocol/game/server/prot.go:17`. ✓
- TS `ServerGameProt.TUT_FLASH = (126, 1)` at `Engine-TS/src/network/game/server/ServerGameProt.ts:24`. ✓
- TS handler at `PlayerOps.ts:694-696`. ✓
- TS encoder at `TutFlashEncoder.ts:9-11` (single `p1(message.tab)`). ✓

All 9 premises verified at HEAD `bff2a12` before plan-author dispatch.

---

## 7. Smoke

User-launched server + Java client, post-bundle. Walk the player through any tutorial chatbox step that triggers a `tut_flash` (e.g., open inventory tab on Survival Expert's prompt). Pre-fix: WARN log `"no handler for TUT_FLASH (opcode 2121)"`. Post-fix: tab flashes (visual confirm) and no warn log.

Per `smoke_test_server_handoff` memory: server is user-launched; sandbox cannot reach Java client.

Per `cascade_theory_smoke_binding` memory: smoke binds the wire-up. If the post-fix walkthrough surfaces a different tutorial blocker (e.g., the P_TELEJUMP issue from runescape_guide → tutorial_complete path), route to NAI-110.

---

## 8. Closes

- Tutorial-step `tut_flash` runtime errors (12+ call sites in `tut_chatbox_steps.rs2`).

**Does NOT close:**
- `[label,tutorial_complete]` `P_TELEJUMP: script not protected` — NAI-110 investigation.
- The remaining ~89 declared-but-unhandled opcodes — no current tutorial consumer.

---

## 9. Out-of-scope follow-ups

- **NAI-110:** P_TELEJUMP protect-context investigation. Stage 1 instrumentation around `Player.CloseModal` ordering on dialog-choice resume; static check whether TS `Engine-TS/PlayerOps.ts` puts P_TELEJUMP under `ProtectedActivePlayer` (currently unconfirmed).
- Future "missing handler bring-up" sub-specs as new content surfaces consume opcodes from the 90-strong unhandled set.
