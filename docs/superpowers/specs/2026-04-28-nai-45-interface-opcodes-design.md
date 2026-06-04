# NAI-45 — IF_BUTTON, CLOSE_MODAL, TUT_CLICKSIDE, IDK_SAVEDESIGN

## Motivation

Four client-game opcodes (all registered as `USER_EVENT` in `prot.go`) have no handler in
`gameHandlers`. This sub-spec wires them and adds the `requestModalClose` deferred-close
mechanism their TS counterparts depend on.

| Opcode | ID | Payload | TS handler |
|--------|----|---------|-----------|
| CLOSE_MODAL | 231 | 0 B | `CloseModalHandler.ts` |
| TUT_CLICKSIDE | 175 | 1 B | `TutClickSideHandler.ts` |
| IF_BUTTON | 155 | 2 B | `IfButtonHandler.ts` |
| IDK_SAVEDESIGN | 52 | 13 B | `IdkSaveDesignHandler.ts` |

TS sources: `Engine-TS` only per `ts_source_canonical_path.md`.

## Tech Stack

**Go 1.26+** (per `go_version.md`; modern Go syntax via the `use-modern-go` skill).

## Deviations

| Tag | Handler | What is skipped | Why |
|-----|---------|----------------|-----|
| **NAI-45-D1** | IF_BUTTON | `buttonType == NO_BUTTON` + `isComponentVisible` checks | No component registry — extends existing S6m-D2/S6o-D1/S6o-D2/NAI-40-D-COMPONENT-REGISTRY-VALIDATION-SKIPPED cluster. Add the new handler to that deviation cluster's doc-comment wherever applicable. |
| **NAI-45-D2** | IF_BUTTON | `protect` always `true` (TS: `root.overlay == false`) | Cannot compute `root.overlay` without component registry. |
| **NAI-45-D3** | IDK_SAVEDESIGN | `IdkType.get(idkit[i])` disable + type checks | No IdkType config registry. Color-range validation retained. |

Pre-existing gaps **not** introduced by NAI-45 (no new deviation tags):

- `CloseModal()` does not clear weak queue or fire `IF_CLOSE` triggers (would require a component registry and weak-queue wiring already missing pre-NAI-45).
- `CloseModal()` does not abort `COUNTDIALOG`/`PAUSEBUTTON` scripts on close (TS Player.ts:757-759).

## Scope

**In scope:**

- `requestModalClose bool` field on `Player` + `processPlayerQueue` pre-close wiring.
- Four `gameHandlers` registrations.
- `handleCloseModal` (player-only, inline in handlers_game.go).
- `(s *Server).handleTutClickSide` + adapter (Server method — needs `scriptProvider`).
- `(s *Server).handleIfButton` + adapter (Server method — needs `scriptProvider`).
- `handleIdkSaveDesign` (player-only, new `handler_interface.go`).
- `designBodyColorCount [5]int` constant (color-range validation for IDK_SAVEDESIGN).
- Tests: `modules/world/handler_interface_test.go`.

**Out of scope:**

- Component registry / `isComponentVisible` (tracked deviations above).
- IdkType config loader (tracked deviation above).
- `IF_CLOSE` triggers on `CloseModal` (pre-existing).
- `OPEN_MODAL` vs overlay `protect` logic (deviation NAI-45-D2).

---

## Pre-flight (verified at HEAD `c22449d`)

| Claim | Result |
|---|---|
| `requestModalClose` does NOT exist on `*Player` | ✓ absent; `refreshModalClose` exists at player.go:194 |
| `p.refreshModalClose bool` at player.go:194 | ✓ |
| `p.CloseModal()` at player_script.go:555 | ✓ |
| `script.QueueStrong` at tick.go:243 | ✓ |
| `func (s *Server) processPlayerQueue(p *Player)` at tick.go:233 | ✓ |
| `p.lastCom int` at player.go:204 | ✓ |
| `p.resumeButtons [5]int` at player.go:198 | ✓ |
| `p.activeScript *script.ScriptState` at player.go:120 | ✓ |
| `script.PauseButton` execution constant | ✓ used at resume_dialog.go:18 |
| `script.TriggerIfButton = 147` at trigger.go:150 | ✓ |
| `script.TriggerTutorial = 159` at trigger.go:164 | ✓ |
| `s.scriptProvider.GetByTriggerSpecific(trigger, typeID, catID)` at provider.go:145 | ✓ |
| `s.runScript(sf, p, nil, protect, nil, nil)` at script.go:86 | ✓ |
| `p.allowDesign bool` at player.go:175 | ✓ |
| `p.body [7]int` at player.go:136; `p.colors [5]int` at player.go:137; `p.gender int` at player.go:138 | ✓ |
| `p.SetAppearanceInv(id)` at player_script.go:514 (sets `masks |= rsbuf.MaskAppearance`) | ✓ |
| `gameHandlers[155/231/175/52]` — all nil (no existing registration) | ✓ |
| Adapter pattern: `p.client.server.handleXxx(p, packet.NewPacket(payload))` — see handlers_game.go:62-74 | ✓ |
| `newTestServer(t)` + `newTestPlayer(t)` + `defaultTestProvider()` available in test files | ✓ server_test.go:311,284; player_test.go:14 |
| `script.LookupKeyForType`, `script.LookupKeyForGlobal` for building test ScriptFiles | ✓ lookup_key.go:6,18 |

---

## File structure

**Modified:**

- `modules/world/player.go` — T1.1: add `requestModalClose bool` field.
- `modules/world/tick.go` — T1.2: prepend STRONG-scan + consume to `processPlayerQueue`.
- `modules/world/handlers_game.go` — T1.3/T2.1: `handleCloseModal` + `handleTutClickSide` + `handleIfButton` + `handleIdkSaveDesign` adapter shims + four `gameHandlers` registrations.

**New:**

- `modules/world/handler_interface.go` — `(s *Server).handleTutClickSide`, `(s *Server).handleIfButton`, `handleIdkSaveDesign`, `designBodyColorCount`.
- `modules/world/handler_interface_test.go` — all interface opcode tests.

---

## Bundle 1: requestModalClose + CLOSE_MODAL + TUT_CLICKSIDE

> For agentic workers: use `superpowers:test-driven-development`. Commit when both tasks are green.

### Task 1.1 — `requestModalClose` field + processPlayerQueue wiring

**Files:** `modules/world/player.go`, `modules/world/tick.go`

Mirrors TS `Player.processQueues()` (Player.ts:854-865).

**Step 1: Write the failing tests**

Add to `modules/world/handler_interface_test.go` (create the file):

```go
package world

import (
    "testing"

    "github.com/zsrv/goscape/pkg/rsbuf"
    "github.com/zsrv/goscape/pkg/script"
)

// TestCloseModalHandlerSetsRequestModalClose pins that handleCloseModal sets
// requestModalClose and does NOT immediately call CloseModal (TS semantics:
// modal is deferred until processPlayerQueue).
func TestCloseModalHandlerSetsRequestModalClose(t *testing.T) {
    p, _ := newTestPlayer(t)
    p.modalMain = 42

    _ = handleCloseModal(p, nil)

    if !p.requestModalClose {
        t.Error("requestModalClose: want true, got false")
    }
    if p.modalMain != 42 {
        t.Errorf("modalMain changed prematurely: got %d, want 42", p.modalMain)
    }
}

// TestProcessPlayerQueueConsumesRequestModalClose pins that processPlayerQueue
// calls CloseModal before running queued scripts when requestModalClose is set.
func TestProcessPlayerQueueConsumesRequestModalClose(t *testing.T) {
    s := newTestServer(t)
    p, _ := newTestPlayer(t)
    p.client.server = s
    p.modalMain = 10
    p.requestModalClose = true

    s.processPlayerQueue(p)

    if p.requestModalClose {
        t.Error("requestModalClose: want false after processPlayerQueue")
    }
    if p.modalMain != -1 {
        t.Errorf("modalMain: got %d, want -1 (CloseModal should have fired)", p.modalMain)
    }
}

// TestProcessPlayerQueueStrongQueueClosesModal pins that a STRONG-typed queue
// entry causes modal close even when requestModalClose was false before the
// tick (TS processQueues lines 854-860).
func TestProcessPlayerQueueStrongQueueClosesModal(t *testing.T) {
    s := newTestServer(t)
    s.scriptProvider = script.NewProvider()
    p, _ := newTestPlayer(t)
    p.client.server = s
    p.modalMain = 99

    p.queue = append(p.queue, PlayerQueueRequest{
        Type:  script.QueueStrong,
        Delay: 0,
    })

    s.processPlayerQueue(p)

    if p.modalMain != -1 {
        t.Errorf("modalMain: got %d, want -1 (STRONG queue should trigger CloseModal)", p.modalMain)
    }
}
```

**Step 2: Add `requestModalClose bool` to player.go**

In `modules/world/player.go`, add adjacent to `refreshModalClose` (line 194):

```go
    refreshModal, refreshModalClose, requestModalClose bool
```

**Step 3: Prepend to processPlayerQueue in tick.go**

At the top of `func (s *Server) processPlayerQueue(p *Player)`, before `i := 0`:

```go
    // TS Player.processQueues (Player.ts:854-865): any STRONG-queue item
    // closes the modal before queues run; then consume the deferred flag
    // (also set by handleCloseModal for the CLOSE_MODAL client packet).
    for _, req := range p.queue {
        if req.Type == script.QueueStrong {
            p.requestModalClose = true
            break
        }
    }
    if p.requestModalClose {
        p.requestModalClose = false
        p.CloseModal()
    }
```

---

### Task 1.2 — CLOSE_MODAL + TUT_CLICKSIDE handlers + tests

**Files:** `modules/world/handlers_game.go`, `modules/world/handler_interface.go`, `modules/world/handler_interface_test.go`

**Step 1: Write failing tests**

Append to `modules/world/handler_interface_test.go`:

```go
// TestHandleTutClickSideOutOfRange pins that tab values outside [0,13]
// are silently dropped (TS TutClickSideHandler.ts:13-15).
func TestHandleTutClickSideOutOfRange(t *testing.T) {
    s := newTestServer(t)
    s.scriptProvider = script.NewProvider()
    p, _ := newTestPlayer(t)
    p.client.server = s

    for _, tab := range []int{-1, 14, 255} {
        if err := s.handleTutClickSide(p, []byte{byte(tab)}); err != nil {
            t.Errorf("tab %d: unexpected error: %v", tab, err)
        }
        if p.activeScript != nil {
            t.Errorf("tab %d: activeScript set unexpectedly", tab)
        }
    }
}

// TestHandleTutClickSideFiresTutorialScript pins that a valid tab fires
// the global [tutorial] script (TS TutClickSideHandler.ts:17-20).
func TestHandleTutClickSideFiresTutorialScript(t *testing.T) {
    s := newTestServer(t)
    s.scriptProvider = script.NewProvider()
    tutScript := &script.ScriptFile{
        Name:      "[tutorial]",
        LookupKey: script.LookupKeyForGlobal(script.TriggerTutorial),
        Opcodes:   []script.Opcode{script.OpReturn},
        IntOperands: []int32{0}, StringOperands: []string{""}, InstructionCount: 1,
    }
    s.scriptProvider.Register(tutScript)
    s.configsView = serverConfigsView{s: s}
    s.invLookup = invLookupView{s: s}
    s.npcLookup = serverNpcLookup{s: s}
    p, _ := newTestPlayer(t)
    p.client.server = s

    if err := s.handleTutClickSide(p, []byte{7}); err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    // Script returns immediately; activeScript is nil after finish.
    if p.activeScript != nil {
        t.Errorf("activeScript: want nil after RETURN, got %v", p.activeScript)
    }
}

// TestHandleTutClickSideNoScriptNoOp pins that missing [tutorial] script
// is a silent no-op (no panic, no error).
func TestHandleTutClickSideNoScriptNoOp(t *testing.T) {
    s := newTestServer(t)
    s.scriptProvider = script.NewProvider()
    p, _ := newTestPlayer(t)
    p.client.server = s

    if err := s.handleTutClickSide(p, []byte{0}); err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
}
```

**Step 2: Implement in handler_interface.go**

Create `modules/world/handler_interface.go`:

```go
package world

import "github.com/zsrv/goscape/pkg/script"

// designBodyColorCount holds the number of valid color values per body-part
// slot. Mirrors the lengths of TS Player.DESIGN_BODY_COLORS
// (Engine-TS/src/engine/entity/Player.ts:102-108).
var designBodyColorCount = [5]int{12, 16, 16, 6, 8}

// handleTutClickSide handles client opcode 175 (TUT_CLICKSIDE).
// Body: u8 sidebar tab index. Fires [tutorial] if tab is in [0,13].
// Mirrors TS TutClickSideHandler.ts.
func (s *Server) handleTutClickSide(p *Player, payload []byte) error {
    if len(payload) < 1 {
        return nil
    }
    tab := int(payload[0])
    if tab < 0 || tab > 13 {
        return nil
    }
    sf := s.scriptProvider.GetByTriggerSpecific(script.TriggerTutorial, -1, -1)
    s.runScript(sf, p, nil, true, nil, nil)
    return nil
}
```

**Step 3: Add adapters and registrations to handlers_game.go**

In `init()`, append:

```go
    gameHandlers[231] = handleCloseModal   // CLOSE_MODAL
    gameHandlers[175] = handleTutClickSide // TUT_CLICKSIDE
```

After the existing adapter functions, add:

```go
// handleCloseModal handles client opcode 231 (CLOSE_MODAL). Zero-byte
// payload. Sets requestModalClose so processPlayerQueue closes the modal
// before queue scripts run this tick.
// Mirrors TS CloseModalHandler.ts — the modal is NOT closed directly here.
func handleCloseModal(p *Player, _ []byte) error {
    p.requestModalClose = true
    return nil
}

func handleTutClickSide(p *Player, payload []byte) error {
    if p.client == nil || p.client.server == nil {
        return nil
    }
    return p.client.server.handleTutClickSide(p, payload)
}
```

---

## Bundle 2: IF_BUTTON + IDK_SAVEDESIGN

> For agentic workers: use `superpowers:test-driven-development`. Commit when both tasks are green.

### Task 2.1 — IF_BUTTON handler + tests

**Files:** `modules/world/handler_interface.go`, `modules/world/handlers_game.go`, `modules/world/handler_interface_test.go`

Mirrors TS `IfButtonHandler.ts`. Two branches:
1. **Resume path**: comId is in `resumeButtons` AND `activeScript.Execution == PauseButton` → set Running, resumeOrFinish.
2. **Trigger path**: otherwise → `GetByTriggerSpecific(TriggerIfButton, comId, -1)`, `runScript(sf, p, nil, true, nil, nil)`.

`lastCom` is always set before branching (TS line before the `if`).

DEVIATION NAI-45-D1: skip `buttonType == NO_BUTTON` and `isComponentVisible` checks (no component registry).  
DEVIATION NAI-45-D2: `protect = true` always (TS: `root.overlay == false`).

**Step 1: Write failing tests**

Append to `modules/world/handler_interface_test.go`:

```go
// TestHandleIfButtonSetsLastCom pins that lastCom is always updated
// regardless of branch taken (TS IfButtonHandler.ts:18).
func TestHandleIfButtonSetsLastCom(t *testing.T) {
    s := newTestServer(t)
    s.scriptProvider = script.NewProvider()
    p, _ := newTestPlayer(t)
    p.client.server = s

    _ = s.handleIfButton(p, []byte{0, 42}) // com = 42

    if p.lastCom != 42 {
        t.Errorf("lastCom: got %d, want 42", p.lastCom)
    }
}

// TestHandleIfButtonResumesPauseButton pins the resume path: comId in
// resumeButtons + activeScript in PauseButton → resumes execution.
func TestHandleIfButtonResumesPauseButton(t *testing.T) {
    s := newTestServer(t)
    s.scriptProvider = script.NewProvider()
    // Minimal script: RETURN.
    retScript := &script.ScriptFile{
        Name: "[test_resume]",
        Opcodes: []script.Opcode{script.OpReturn},
        IntOperands: []int32{0}, StringOperands: []string{""}, InstructionCount: 1,
    }
    s.configsView = serverConfigsView{s: s}
    s.invLookup = invLookupView{s: s}
    s.npcLookup = serverNpcLookup{s: s}
    p, _ := newTestPlayer(t)
    p.client.server = s
    p.resumeButtons = [5]int{7, 0, 0, 0, 0}

    // Build an already-suspended script state.
    st := script.Init(retScript, p, false, nil, nil)
    st.Provider = s.scriptProvider
    st.Configs = s.configsView
    st.Inv = s.invLookup
    st.Npcs = s.npcLookup
    st.PlayerLookup = s
    st.Execution = script.PauseButton
    p.StoreActiveScript(st)

    _ = s.handleIfButton(p, []byte{0, 7}) // com = 7

    // Script finishes (RETURN) → activeScript cleared.
    if p.activeScript != nil {
        t.Errorf("activeScript: want nil after resume+finish, got non-nil")
    }
}

// TestHandleIfButtonPauseButtonNotInResumeButtons pins that a PauseButton
// script is NOT resumed when comId is absent from resumeButtons
// (falls through to trigger lookup).
func TestHandleIfButtonPauseButtonNotInResumeButtons(t *testing.T) {
    s := newTestServer(t)
    s.scriptProvider = script.NewProvider()
    p, _ := newTestPlayer(t)
    p.client.server = s
    p.resumeButtons = [5]int{99, 0, 0, 0, 0} // 7 is not in the list

    st := &script.ScriptState{Execution: script.PauseButton}
    p.StoreActiveScript(st)

    _ = s.handleIfButton(p, []byte{0, 7}) // com = 7

    // activeScript unchanged (not resumed)
    if p.activeScript == nil || p.activeScript.Execution != script.PauseButton {
        t.Errorf("activeScript: want PauseButton state unchanged, got %v", p.activeScript)
    }
}

// TestHandleIfButtonFiresIfButtonScript pins the trigger-lookup path:
// comId not in resumeButtons → [if_button,<com>] script fires.
func TestHandleIfButtonFiresIfButtonScript(t *testing.T) {
    s := newTestServer(t)
    s.scriptProvider = script.NewProvider()
    ifBtnScript := &script.ScriptFile{
        Name:      "[if_button,42]",
        LookupKey: script.LookupKeyForType(script.TriggerIfButton, 42),
        Opcodes:   []script.Opcode{script.OpReturn},
        IntOperands: []int32{0}, StringOperands: []string{""}, InstructionCount: 1,
    }
    s.scriptProvider.Register(ifBtnScript)
    s.configsView = serverConfigsView{s: s}
    s.invLookup = invLookupView{s: s}
    s.npcLookup = serverNpcLookup{s: s}
    p, _ := newTestPlayer(t)
    p.client.server = s

    _ = s.handleIfButton(p, []byte{0, 42})

    // Script returns immediately; no suspension.
    if p.activeScript != nil {
        t.Errorf("activeScript: want nil after RETURN, got non-nil")
    }
}

// TestHandleIfButtonNoScriptNoOp pins that missing [if_button,<com>]
// is a silent no-op when comId is not in resumeButtons.
func TestHandleIfButtonNoScriptNoOp(t *testing.T) {
    s := newTestServer(t)
    s.scriptProvider = script.NewProvider()
    p, _ := newTestPlayer(t)
    p.client.server = s

    if err := s.handleIfButton(p, []byte{0, 7}); err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
}
```

**Step 2: Implement (s *Server).handleIfButton in handler_interface.go**

Append to `modules/world/handler_interface.go`:

```go
// handleIfButton handles client opcode 155 (IF_BUTTON).
// Body: u16 component-id.
//
// Sets lastCom, then:
//   - If comId is in resumeButtons and activeScript is in PauseButton state →
//     resumes the suspended script (mirrors TS IfButtonHandler.ts:20-23).
//   - Otherwise → looks up [if_button,<comId>] and runs it with protect=true.
//
// DEVIATION NAI-45-D1: buttonType and isComponentVisible checks skipped —
// no component registry (same cluster as S6m-D2, S6o-D1, NAI-40-D-COMPONENT-
// REGISTRY-VALIDATION-SKIPPED). Closure: component-registry sub-spec.
//
// DEVIATION NAI-45-D2: protect=true always; TS uses root.overlay==false
// which requires the component registry. Closure: component-registry sub-spec.
func (s *Server) handleIfButton(p *Player, payload []byte) error {
    if len(payload) < 2 {
        return nil
    }
    comId := int(uint16(payload[0])<<8 | uint16(payload[1]))
    p.lastCom = comId

    for _, b := range p.resumeButtons {
        if b == comId {
            if p.activeScript != nil && p.activeScript.Execution == script.PauseButton {
                p.activeScript.Execution = script.Running
                s.resumeOrFinish(p.activeScript, p)
            }
            return nil
        }
    }

    sf := s.scriptProvider.GetByTriggerSpecific(script.TriggerIfButton, comId, -1)
    s.runScript(sf, p, nil, true, nil, nil) // protect=true per NAI-45-D2
    return nil
}
```

**Step 3: Add adapter and registration to handlers_game.go**

In `init()`, append:

```go
    gameHandlers[155] = handleIfButton // IF_BUTTON
```

After the existing adapter functions, add:

```go
func handleIfButton(p *Player, payload []byte) error {
    if p.client == nil || p.client.server == nil {
        return nil
    }
    return p.client.server.handleIfButton(p, payload)
}
```

---

### Task 2.2 — IDK_SAVEDESIGN handler + tests

**Files:** `modules/world/handler_interface.go`, `modules/world/handlers_game.go`, `modules/world/handler_interface_test.go`

Mirrors TS `IdkSaveDesignHandler.ts`. Validates `allowDesign`, `gender <= 1`, color ranges
against `designBodyColorCount`. Skips IdkType validation (NAI-45-D3). On pass: sets
`p.gender`, `p.body`, `p.colors`, calls `p.SetAppearanceInv(p.appearanceInv)` to flag
`MaskAppearance` (equivalent to TS `player.buildAppearance(player.appearanceInv)`).

Wire format: `u8 gender | u8[7] idkit (255→-1) | u8[5] color`.

**Step 1: Write failing tests**

Append to `modules/world/handler_interface_test.go`:

```go
// idkPayload builds a 13-byte IDK_SAVEDESIGN payload.
func idkPayload(gender byte, idkit [7]byte, color [5]byte) []byte {
    p := make([]byte, 13)
    p[0] = gender
    for i, v := range idkit {
        p[1+i] = v
    }
    for i, v := range color {
        p[8+i] = v
    }
    return p
}

// TestHandleIdkSaveDesignAllowDesignFalse pins that the packet is dropped
// when allowDesign is false.
func TestHandleIdkSaveDesignAllowDesignFalse(t *testing.T) {
    p, _ := newTestPlayer(t)
    p.allowDesign = false

    _ = handleIdkSaveDesign(p, idkPayload(0, [7]byte{0,1,2,3,4,5,6}, [5]byte{0,0,0,0,0}))

    if p.gender != 0 || p.body != [7]int{0, 10, 18, 26, 33, 36, 42} {
        t.Error("player state changed despite allowDesign=false")
    }
}

// TestHandleIdkSaveDesignInvalidGender pins that gender > 1 is rejected.
func TestHandleIdkSaveDesignInvalidGender(t *testing.T) {
    p, _ := newTestPlayer(t)
    p.allowDesign = true

    _ = handleIdkSaveDesign(p, idkPayload(2, [7]byte{}, [5]byte{}))

    if p.gender != 0 {
        t.Errorf("gender changed: got %d, want 0", p.gender)
    }
}

// TestHandleIdkSaveDesignColorOutOfBounds pins that a color value >=
// designBodyColorCount[i] is rejected.
func TestHandleIdkSaveDesignColorOutOfBounds(t *testing.T) {
    p, _ := newTestPlayer(t)
    p.allowDesign = true

    // color[0] max is 11 (count=12); send 12 → out of bounds.
    _ = handleIdkSaveDesign(p, idkPayload(0, [7]byte{}, [5]byte{12, 0, 0, 0, 0}))

    if p.gender != 0 {
        t.Errorf("state changed despite invalid color: gender=%d", p.gender)
    }
}

// TestHandleIdkSaveDesignSuccess pins the happy path: valid inputs update
// gender/body/colors and flag MaskAppearance.
func TestHandleIdkSaveDesignSuccess(t *testing.T) {
    p, _ := newTestPlayer(t)
    p.allowDesign = true
    p.appearanceInv = 0 // prod default via SetAppearanceInv

    body := [7]byte{3, 4, 5, 6, 7, 8, 9}
    colors := [5]byte{0, 1, 2, 0, 0}
    _ = handleIdkSaveDesign(p, idkPayload(1, body, colors))

    if p.gender != 1 {
        t.Errorf("gender: got %d, want 1", p.gender)
    }
    for i, v := range body {
        if p.body[i] != int(v) {
            t.Errorf("body[%d]: got %d, want %d", i, p.body[i], v)
        }
    }
    for i, v := range colors {
        if p.colors[i] != int(v) {
            t.Errorf("colors[%d]: got %d, want %d", i, p.colors[i], v)
        }
    }
    // MaskAppearance set via SetAppearanceInv.
    if p.masks&rsbuf.MaskAppearance == 0 {
        t.Error("MaskAppearance: want set, got unset")
    }
}

// TestHandleIdkSaveDesignIdkit255ConvertedToMinus1 pins that wire value 255
// is stored as -1 (TS IdkSaveDesignDecoder.ts:14-16). The IDK_SAVEDESIGN
// handler receives the already-decoded value.
func TestHandleIdkSaveDesignIdkit255ConvertedToMinus1(t *testing.T) {
    p, _ := newTestPlayer(t)
    p.allowDesign = true

    // idkit[0] = 255 → decoded to -1; color all zero.
    _ = handleIdkSaveDesign(p, idkPayload(0, [7]byte{255, 1, 2, 3, 4, 5, 6}, [5]byte{}))

    if p.body[0] != -1 {
        t.Errorf("body[0]: got %d, want -1 (decoded from wire 255)", p.body[0])
    }
}
```

**Step 2: Implement handleIdkSaveDesign in handler_interface.go**

Append to `modules/world/handler_interface.go`:

```go
// handleIdkSaveDesign handles client opcode 52 (IDK_SAVEDESIGN).
// Body: u8 gender | u8[7] idkit (255 → -1) | u8[5] color.
//
// Validates allowDesign, gender ≤ 1, and color ranges. On pass: updates
// p.gender/body/colors and calls SetAppearanceInv to flag MaskAppearance
// (mirrors TS buildAppearance(player.appearanceInv) at IdkSaveDesignHandler.ts:37).
//
// DEVIATION NAI-45-D3: IdkType.get(idkit[i]) disable+type checks skipped —
// no IdkType config registry. Closure: IdkType-config sub-spec.
func handleIdkSaveDesign(p *Player, payload []byte) error {
    if len(payload) < 13 {
        return nil
    }
    if !p.allowDesign {
        return nil
    }

    gender := int(payload[0])
    if gender > 1 {
        return nil
    }

    var idkit [7]int
    for i := range 7 {
        v := int(payload[1+i])
        if v == 255 {
            v = -1
        }
        idkit[i] = v
    }

    var color [5]int
    for i := range 5 {
        color[i] = int(payload[8+i])
    }

    for i, c := range color {
        if c >= designBodyColorCount[i] {
            return nil
        }
    }

    p.gender = gender
    p.body = idkit
    p.colors = color
    p.SetAppearanceInv(p.appearanceInv)
    return nil
}
```

**Step 3: Add registration to handlers_game.go**

`handleIdkSaveDesign` is a plain `func(*Player, []byte) error` (not a Server method), so no
adapter is needed — register it directly in `init()`:

```go
    gameHandlers[52] = handleIdkSaveDesign // IDK_SAVEDESIGN
```

---

## Test strategy

All tests in `modules/world/handler_interface_test.go`. Run with:

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestHandleCloseModal\|TestProcessPlayerQueue\|TestHandleIfButton\|TestHandleTut\|TestHandleIdk
```

Full suite:

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/...
```

## Close commit trailer

The close commit body should include:

```
Closes memory: nai_followups.md — no new open items from NAI-45; extends
existing NAI-40-D-COMPONENT-REGISTRY-VALIDATION-SKIPPED cluster with
NAI-45-D1/D2 (IF_BUTTON). NAI-45-D3 (IDK_SAVEDESIGN IdkType) is new.
```
