# Sub-spec RuneScript S5f: Interface Opcodes — Design

**Status:** Draft → ready for plan
**Scope:** 18 `IF_*` handlers covering modal management (5), visual property setters (5), layout (5), and misc (3). 10 new server wire ops for the `IfSet*` family. ~18 new methods on `ActivePlayer`. Modal-open/close handlers piggyback on the existing `modalState`/`refreshModal` infrastructure; `IfSet*` writes emit wire packets immediately (fire-and-forget, matching TS).
**Out of scope:** `P_COUNTDIALOG` and `P_PAUSEBUTTON` suspension — these are separate opcode families that pause the script until user input. Without them the dialog scripts run to completion immediately (visible on client as flashed dialogs). Future sub-spec. Per-component state persistence on the server — TS doesn't do this either; state is ephemeral on the wire. `IF_MULTIZONE`, `INVOTHER`-style cross-player variants. `IF_SETRESUMEBUTTONS` stores the buttons but the pause-button handler that consumes them is deferred.

---

## Goal

After S5f:

- Cache scripts can open and close full-screen / chat / side / main+side modals (`IF_OPENMAIN`, `IF_OPENCHAT`, `IF_OPENSIDE`, `IF_OPENMAIN_SIDE`, `IF_CLOSE`). Existing `encodeOut()` already emits the wire packets from `modalMain`/`modalChat`/`modalSide` state — handlers just set those fields.
- Cache scripts can set per-component visual properties: text (`IF_SETTEXT`), 3D model (`IF_SETMODEL`), NPC head (`IF_SETNPCHEAD`), player head (`IF_SETPLAYERHEAD`), animation (`IF_SETANIM`), hide flag (`IF_SETHIDE`), tab binding (`IF_SETTAB`, `IF_SETTABACTIVE`), obj+scale (`IF_SETOBJECT`), text colour (`IF_SETCOLOUR`), x/y position (`IF_SETPOSITION`), model recolour (`IF_SETRECOL`). Each emits a new wire op (`OpIfSetText` etc.) directly on mutation.
- `IF_SETRESUMEBUTTONS` stores a 5-button array on `Player.resumeButtons` for later `P_PAUSEBUTTON` consumption.
- Demo: a LOGIN trigger that calls `if_openchat(dialog_com); if_setnpcheadmodel(...); if_settext("Welcome, adventurer!")` opens a chat dialog on the Java client with the welcome text visible. (Without pausebutton suspension the dialog may auto-advance, but the visuals render.)

## Architecture

```
pkg/io/protocol/game/server/prot.go    + 10 new IfSet* wire ops
pkg/script/
├── handlers_interface.go              (new) 18 handlers
├── handlers_interface_test.go         (new)
├── active.go                          + ~18 interface-control methods on ActivePlayer
└── handlers.go                        + 18 map entries

modules/world/
├── player.go                          + resumeButtons [5]int, if not already present
├── player_script.go                   + 18 Player impls (mostly wire emitters)
├── player_interface.go                (new) wire helpers (writeIfSetText, etc.)
└── script_test.go                     + E2E IF_OPENMAIN → modal + IF_SETTEXT wire test
```

## Components

### 1. New wire ops — `pkg/io/protocol/game/server/prot.go`

Implementer verifies exact opcode numbers + payload sizes by reading `$HOME/Code/github.com/LostCityRS/Engine-TS/src/network/game/server/ServerGameProt.ts`. Expected shapes:

| Op | Wire format |
|---|---|
| `OpIfSetText` | u16 com + jstr text (variable, -2 prefix) |
| `OpIfSetModel` | u16 com + u32 modelID (6) |
| `OpIfSetNpcHead` | u16 com + u16 npcID (4) |
| `OpIfSetPlayerHead` | u16 com (2) |
| `OpIfSetAnim` | u16 com + u16 seqID (4) |
| `OpIfSetHide` | u16 com + u8 bool (3) |
| `OpIfSetObject` | u16 com + u16 objID + u32 scale (8) |
| `OpIfSetColour` | u16 com + u16 colour (4) |
| `OpIfSetPosition` | u16 com + u16 x + u16 y (6) |
| `OpIfSetRecol` | u16 com + u16 src + u16 dst (6) |
| `OpIfSetTab` | u16 com + u8 tab (3) — verify |
| `OpIfSetTabActive` | u8 tab (1) — verify |

Some of these are already declared in Go? Grep `prot.go` for `OpIf` to confirm which are new. At minimum the 5 Open/Close ops exist (OpIfClose, OpIfOpenMain, OpIfOpenChat, OpIfOpenSide, OpIfOpenMainSide). The rest are new.

### 2. `ActivePlayer` interface extensions

Group by operation kind:

```go
// S5f: interface / modal control.

// Modal management. These mutate Player.modalState and are flushed by
// encodeOut() on the next tick.
CloseModal()
OpenMain(com int)
OpenChat(com int)
OpenSide(com int)
OpenMainSide(mainCom, sideCom int)

// Immediate wire emitters (no server-side persistence — TS behaviour).
IfSetText(com int, text string)
IfSetModel(com, modelID int)
IfSetNpcHead(com, npcID int)
IfSetPlayerHead(com int)
IfSetAnim(com, seqID int)
IfSetHide(com int, hide bool)
IfSetTab(com, tab int)
IfSetObject(com, objID, scale int)
IfSetColour(com, colour int)
IfSetPosition(com, x, y int)
IfSetRecol(com, srcColour, dstColour int)
IfSetTabActive(tab int)

// Resume-button storage for future P_PAUSEBUTTON.
SetResumeButtons(b1, b2, b3, b4, b5 int)
```

That's 18 methods. `Player.resumeButtons [5]int` field added if it doesn't exist.

### 3. Handler shapes — `pkg/script/handlers_interface.go`

Modal-open handlers are one-liners:

```go
func handleIfOpenMain(s *ScriptState) error {
    if s.Pointers&PtrActivePlayer == 0 || s.Self == nil {
        return errors.New("IF_OPENMAIN: no active player")
    }
    com := s.PopInt()
    s.Self.OpenMain(com)
    return nil
}
```

`IfSet*` handlers pop the right number of args and call the matching `Self.IfSet*` method. Exact pop order verified from TS `PlayerOps.ts` (the survey gives approximate shapes; implementer confirms).

### 4. Player impls — `modules/world/player_script.go` + `player_interface.go`

Modal handlers mutate existing fields:

```go
func (p *Player) OpenMain(com int) {
    p.modalMain = com
    p.modalState |= modalStateMain
    p.refreshModal = true
}

func (p *Player) CloseModal() {
    p.modalMain = -1
    p.modalChat = -1
    p.modalSide = -1
    p.modalState = 0
    p.refreshModalClose = true
}
```

Wire emitters live in `player_interface.go` as thin helpers that build a packet and call `writeOut`:

```go
func (p *Player) IfSetText(com int, text string) {
    buf := packet.NewPacket(nil)
    buf.P2(uint16(com))
    buf.PJStrLF(text)
    p.writeOut(gameserver.OpIfSetText, buf.Bytes())
}

func (p *Player) IfSetModel(com, modelID int) {
    buf := packet.NewPacket(nil)
    buf.P2(uint16(com))
    buf.P4(uint32(modelID))
    p.writeOut(gameserver.OpIfSetModel, buf.Bytes())
}
// ... one per IfSet* ...
```

`SetResumeButtons` writes the 5 ids into `p.resumeButtons` — no wire.

### 5. Existing infrastructure to respect

- **modalState bitmask**: `modalStateMain = 0x1`, `modalStateChat = 0x2`, `modalStateSide = 0x4` (from existing `encodeOut()` flow). `OpenMain` sets only the main bit and clears chat/side? Or does it leave them? Per TS: opening a main closes chat and side; opening chat keeps main; opening side keeps main. Implementer verifies TS rules in `Player.ts:1928-2022` and replicates.
- **`refreshModal` / `refreshModalClose` flags** drive `encodeOut()` to actually write the packet. Modal-open handlers must set `refreshModal = true`. `CloseModal` sets `refreshModalClose = true`.
- **Fire-and-forget wire writes** for `IfSet*` — no persistence. Tests assert that calling the handler immediately produces wire bytes.

### 6. Testing

**Handler unit tests** (`pkg/script/handlers_interface_test.go`):

- For each handler, run a 1-instruction script via mockPlayer that records the call. Assert the recorded args match the input. Aim for 20+ sub-tests.
- One "requires active player" negative per handler family.

**Wire tests** (`modules/world/player_interface_test.go` or added to `player_test.go`):

- `TestIfSetTextWireFormat`: call `p.IfSetText(123, "hello")`, drain conn, assert wire bytes = `[encrypted_opcode, len, P2(123), PJStrLF("hello")]`.
- Similar single-case tests for IfSetModel / IfSetAnim / IfSetColour — one per wire shape category.

**E2E** (`modules/world/script_test.go`):

- `TestIfOpenMainSetsModalState`: run `push_constant_int 1234, if_openmain, return`, assert `p.modalMain == 1234`, `p.modalState & modalStateMain != 0`, `p.refreshModal == true`.
- `TestIfSetTextViaScriptEmitsWire`: run a script that pushes a string + com and calls `if_settext`; drain conn; assert OpIfSetText bytes on wire.

## LOC estimate

| File | LOC |
|---|---|
| `pkg/io/protocol/game/server/prot.go` (diff) | +14 (10 ops) |
| `pkg/script/active.go` (diff) | +45 (18 methods) |
| `pkg/script/handlers_interface.go` | ~260 |
| `pkg/script/handlers_interface_test.go` | ~280 |
| `pkg/script/handlers.go` (diff) | +22 |
| `pkg/script/runner_test.go` (diff) | +80 (extend mockPlayer) |
| `modules/world/player.go` (diff) | +3 (resumeButtons field) |
| `modules/world/player_script.go` (diff) | +30 (modal methods) |
| `modules/world/player_interface.go` | ~130 (wire emitters) |
| `modules/world/script_test.go` (diff) | +80 |
| **Total** | **~940** |

## Key design calls

- **18 interface methods on `ActivePlayer`.** Big growth but each is 1-5 LOC — same pattern as S5c (19 methods). The compile-time `var _ script.ActivePlayer = (*Player)(nil)` catches drift.
- **No per-component state storage.** TS doesn't persist IF_SET* text/model/colour server-side — they're fire-and-forget packets. Scripts must regenerate state on re-login or new-viewer joins. We mirror exactly.
- **`SetResumeButtons` stores but never fires.** Until `P_PAUSEBUTTON` suspension lands, the buttons are written to `resumeButtons[5]` and nothing consumes them. Document as partial-impl.
- **Modal mutual exclusion** per TS rules: `OpenMain` closes chat+side; `OpenChat` keeps main but closes side; `OpenSide` keeps main but closes chat. `OpenMainSide` opens both main and side. Implementer verifies against `Player.ts:1928-2022`.
- **Heredoc `!=` bug**: use Edit/Write tool for test files.

## Gotchas

- **`OpIfSetText` is variable-length**: `PayloadSize: -2` (2-byte length prefix). All other `IfSet*` ops are fixed-size.
- **Modal close on main/side/chat individually** — TS has separate paths for "close this modal" vs "close all modals". `IF_CLOSE` in cache typically closes all; verify.
- **tab argument for `IF_SETTAB` / `IF_SETTABACTIVE`**: TS packs the tab index as u8 (0-15 range). Clamp if needed.
- **`IF_SETOBJECT` scale is u32 in wire**: real scale values are small ints; still encoded as u32.
- **`P_COUNTDIALOG` / `P_PAUSEBUTTON` suspension deferred**: real chat dialogs in cache scripts do `chatnpc(...)` then `~pausebutton`. With S5f but no pausebutton, the dialog opens and the script continues past, so the close-on-button-click never cleans up. Document as "known glitch until pause-button sub-spec."
- **ServerGameProt opcode numbers**: implementer MUST read TS `ServerGameProt.ts` (or the prot registry) to get exact opcode numbers for all 10 new wire ops. Do NOT guess.
