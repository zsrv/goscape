# Sub-spec RuneScript S5g: Dialog Flow Suspension — Design

**Status:** Draft → ready for plan
**Scope:** Close the dialog suspension gap left by S5f. Three new script opcodes (P_PAUSEBUTTON, P_COUNTDIALOG, LAST_COM), one new server wire op (P_COUNTDIALOG outbound), two new client handlers (RESUME_PAUSEBUTTON, RESUME_P_COUNTDIALOG), and `resumeOrFinish` extension to treat `PauseButton` / `CountDialog` execution states as suspensions (store, not discard). One new `ActivePlayer` method: `LastCom() int`. Cleanup: on logout, abort any active script.
**Out of scope:** General IF_BUTTON click routing (not every button click resumes a script — only those whose com matches `resumeButtons`). Multi-line dialog / next-step chat flows (scripts are responsible for orchestrating). NPC_SUSPENDED / WORLD_SUSPENDED states (separate sub-specs). Zone-exit / teleport abort of suspended scripts.

---

## Goal

After S5g:

- `p_pausebutton` suspends a script. When the client clicks a button whose id is in `resumeButtons`, the server resumes the script; the handler that reads `last_com` retrieves the clicked button id.
- `p_countdialog` suspends a script and sends a P_COUNTDIALOG packet to the client. When the client sends `RESUME_P_COUNTDIALOG` with a count, the server resumes the script with that count pushed on the int stack.
- `last_com` pushes `player.lastCom` onto the int stack.
- Chat dialogs from S5f now work end-to-end: open modal, set text, pausebutton, resume on button click, continue with next step.
- Demo: a LOGIN-trigger script that opens a chat dialog with "Press button 1 to continue", calls `p_pausebutton`, then on resume prints a follow-up message via `mes`. Verified by E2E test that simulates the RESUME_PAUSEBUTTON client packet.

## Architecture

```
pkg/io/protocol/game/server/prot.go    + OpPCountDialog (new outbound wire op)
pkg/script/
├── active.go                          + LastCom() int on ActivePlayer
├── handlers_player.go                 + handlePPauseButton, handlePCountDialog, handleLastCom
└── handlers.go                        + 3 map entries

modules/world/
├── player_script.go                   + LastCom() impl
├── script.go                          resumeOrFinish now routes PauseButton + CountDialog to StoreActiveScript
├── resume_dialog.go (new)             Server methods: resumePauseButton, resumeCountDialog
├── handlers_game.go                   + register 2 new client handler fns
├── tick.go                            logout path → ClearActiveScript
└── script_test.go                     + E2E pausebutton + countdialog tests
```

## Components

### 1. New wire op — `pkg/io/protocol/game/server/prot.go`

```go
OpPCountDialog = Op{Opcode: ??, PayloadSize: 0}
```

Implementer verifies exact opcode number from TS `ServerGameProt.ts`. The packet is sent by the server to tell the client "show an 'enter a number' dialog." Client responds later with `RESUME_P_COUNTDIALOG` (already declared inbound at opcode 237).

### 2. `ActivePlayer.LastCom()`

```go
// LastCom returns the component id most recently clicked on the client.
// Used by LAST_COM opcode and pause-button resume gating.
LastCom() int
```

`*Player` impl is one line: `return p.lastCom` (field already exists).

### 3. Script handlers — `pkg/script/handlers_player.go` or new `handlers_dialog.go`

```go
// handlePPauseButton suspends the script until the client sends a
// RESUME_PAUSEBUTTON packet whose button id is in the active player's
// resumeButtons array. The resume path sets Execution = Running and
// re-enters Execute.
func handlePPauseButton(s *ScriptState) error {
    if s.Pointers&PtrActivePlayer == 0 || s.Self == nil {
        return errors.New("P_PAUSEBUTTON: no active player")
    }
    s.Execution = PauseButton
    return nil
}

// handlePCountDialog suspends the script and directs the active player
// to send the P_COUNTDIALOG wire packet. Resume comes via the client's
// RESUME_P_COUNTDIALOG which pushes the count onto the int stack.
func handlePCountDialog(s *ScriptState) error {
    if s.Pointers&PtrActivePlayer == 0 || s.Self == nil {
        return errors.New("P_COUNTDIALOG: no active player")
    }
    s.Self.SendCountDialog()
    s.Execution = CountDialog
    return nil
}

// handleLastCom pushes the active player's lastCom field.
func handleLastCom(s *ScriptState) error {
    if s.Pointers&PtrActivePlayer == 0 || s.Self == nil {
        return errors.New("LAST_COM: no active player")
    }
    s.PushInt(s.Self.LastCom())
    return nil
}
```

`SendCountDialog` is a new `ActivePlayer` method that writes the OpPCountDialog wire packet. One-line impl on `*Player`: `p.writeOut(gameserver.OpPCountDialog, nil)`.

### 4. `resumeOrFinish` extension — `modules/world/script.go`

Currently:
```go
case script.Suspended:
    self.StoreActiveScript(state)
default:
    // CountDialog, PauseButton, NpcSuspended, WorldSuspended are
    // handled by later sub-specs; drop the state for now.
    s.log.Warn("...")
    self.ClearActiveScript()
```

Updated:
```go
case script.Suspended, script.PauseButton, script.CountDialog:
    self.StoreActiveScript(state)
default:
    // NpcSuspended / WorldSuspended — future sub-specs.
    s.log.Warn("script in unsupported execution state",
        "script", state.Script.Name, "execution", state.Execution)
    self.ClearActiveScript()
```

### 5. Client resume handlers — `modules/world/resume_dialog.go` + `handlers_game.go`

Register two new client-packet handlers. Each follows the existing gameHandler shape (reads the packet, validates state, calls into Server methods).

```go
// resumePauseButton is the handler for client opcode 235
// (RESUME_PAUSEBUTTON). Body: u16 component-id.
func (s *Server) handleResumePauseButton(p *Player, buf *packet.Packet) error {
    com := int(buf.G2())

    // Update lastCom so LAST_COM can read it during resume.
    p.lastCom = com

    // Gate: script must be active + in PauseButton state + clicked
    // component must be in resumeButtons.
    if p.activeScript == nil || p.activeScript.Execution != script.PauseButton {
        return nil // ignore stray button click
    }
    matched := false
    for _, b := range p.resumeButtons {
        if b == com {
            matched = true
            break
        }
    }
    if !matched {
        return nil
    }

    // Resume.
    p.activeScript.Execution = script.Running
    s.resumeOrFinish(p.activeScript, p)
    return nil
}

// resumeCountDialog handles client opcode 237 (RESUME_P_COUNTDIALOG).
// Body: i32 count (signed).
func (s *Server) handleResumeCountDialog(p *Player, buf *packet.Packet) error {
    count := int32(buf.G4())

    if p.activeScript == nil || p.activeScript.Execution != script.CountDialog {
        return nil
    }

    // Push the count onto the int stack BEFORE resuming, so the next
    // opcode pops it.
    p.activeScript.PushInt(int(count))
    p.activeScript.Execution = script.Running
    s.resumeOrFinish(p.activeScript, p)
    return nil
}
```

Registered in `handlers_game.go`:
```go
gameclient.OpResumePauseButton: s.handleResumePauseButton,
gameclient.OpResumeCountDialog: s.handleResumeCountDialog,
```

(Verify the exact Go-side const names by grepping `pkg/io/protocol/game/client/prot.go` for opcodes 235 and 237.)

### 6. Logout cleanup — `modules/world/tick.go`

In `processLogouts`, when a player is being closed:

```go
if p.loggingOut {
    p.activeScript = nil
    // ... existing cleanup ...
}
```

Prevents the tick loop from trying to resume a script on a player who's no longer online.

## Testing

**Script unit tests** (`pkg/script/handlers_dialog_test.go` or append to existing handlers_player_test.go):

- `TestPPauseButtonSuspends`: script `[p_pausebutton, return]`, run, assert `state.Execution == PauseButton`.
- `TestPCountDialogSuspends`: script `[p_countdialog, return]` with a mock player that captures `SendCountDialog` calls, run, assert `Execution == CountDialog` AND `mp.sendCountDialogCalls == 1`.
- `TestLastCom`: seed `mp.lastCom = 42`, script `[last_com, return]`, assert `state.PopInt() == 42`.

**E2E tests** (`modules/world/script_test.go`):

- `TestPauseButtonResumes`: run a script `[push "a", mes, push 7, if_setresumebuttons (but only one), p_pausebutton, push "b", mes, return]`. After runScript, assert `p.activeScript != nil` and `p.activeScript.Execution == script.PauseButton`. Simulate an IfButton click by setting `p.lastCom = 7`, call `s.handleResumePauseButton(p, packetWith(7))`. Drain conn; expect the "b" MessageGame packet.
- `TestCountDialogResumes`: script `[p_countdialog, push "total=", name_append_num, mes, return]` (or simpler: `[p_countdialog, stat_advance 0, return]` just to push the count somewhere). After runScript, script is suspended. Simulate `s.handleResumeCountDialog(p, packetWith(42))`. Assert the count was consumed.

## LOC estimate

| File | LOC |
|---|---|
| `pkg/io/protocol/game/server/prot.go` (diff) | +2 |
| `pkg/script/active.go` (diff) | +4 (2 methods: LastCom, SendCountDialog) |
| `pkg/script/handlers_player.go` (diff) | +40 (3 handlers) |
| `pkg/script/handlers.go` (diff) | +4 |
| `pkg/script/runner_test.go` (diff) | +20 (mockPlayer) |
| `pkg/script/handlers_player_test.go` (diff) | +80 |
| `modules/world/player_script.go` (diff) | +10 (LastCom, SendCountDialog) |
| `modules/world/script.go` (diff) | +3 |
| `modules/world/resume_dialog.go` | 75 |
| `modules/world/handlers_game.go` (diff) | +6 |
| `modules/world/tick.go` (diff) | +3 |
| `modules/world/script_test.go` (diff) | +120 |
| **Total** | **~370** |

## Key design calls

- **`PauseButton` / `CountDialog` use the same activeScript slot as `Suspended`.** Only one script can be suspended at a time per player — matches TS.
- **LastCom gate on pausebutton**: `resumeButtons` array (from IF_SETRESUMEBUTTONS, S5f) limits which buttons can trigger a resume. Clicking an unregistered button is ignored.
- **CountDialog push-before-resume**: the count is pushed onto `state.IntStack` directly, then `Execution = Running` is set, then `resumeOrFinish` re-enters Execute. The script's next opcode sees the count on top of the stack.
- **New `SendCountDialog` method on ActivePlayer** (not a wire helper on Player only) — allows the handler to be pure pkg/script.
- **Logout cleanup clears activeScript**. Previously the GC reclaimed it with the Player, but now that the resume path is client-driven, a disconnected player's state could still be referenced via a stale pointer if the client packet arrives after disconnect. Explicitly nil it.
- **No IF_BUTTON general handler.** This sub-spec only handles `RESUME_PAUSEBUTTON`. General button clicks (bank deposit, shop buy) come with a later sub-spec when we wire IF_BUTTON → script triggers.

## Gotchas

- **Opcode constants**: verify `OpPPauseButton`, `OpPCountDialog`, `OpLastCom` exist in `pkg/script/opcode.go`. All three should be there from S1 scaffolding.
- **Client opcode names in Go**: grep `pkg/io/protocol/game/client/prot.go` for the exact names of opcodes 235 and 237 — might be `OpResumePauseButton` / `OpResumePCountDialog` or similar.
- **Packet.G4 returns uint32**; `int32(buf.G4())` for signed.
- **Execution state comparison**: `PauseButton` and `CountDialog` are `Execution` enum values defined in `pkg/script/execution.go` from S1.
- **Tests for client handlers**: the existing gameHandler-dispatch test pattern probably uses a synthetic `*packet.Packet` with pre-seeded bytes. Check `modules/world/handlers_game_test.go` or adjacent tests for the pattern.
- **Heredoc `!=` bug**: use Edit/Write for tests.
