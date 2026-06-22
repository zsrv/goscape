# RuneScript S5g: Dialog Flow Suspension Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Register `P_PAUSEBUTTON`, `P_COUNTDIALOG`, `LAST_COM` opcodes. Add 1 new server wire op (`OpPCountDialog`). Wire 2 new client resume handlers (`RESUME_PAUSEBUTTON`, `RESUME_P_COUNTDIALOG`). Extend `resumeOrFinish` to handle PauseButton + CountDialog states. Logout cleanup.

**Architecture:** Handlers set `state.Execution = PauseButton/CountDialog`, the dispatch loop exits, `resumeOrFinish` stores the script via `StoreActiveScript` (same path as `Suspended`). A client resume packet flips `Execution = Running`, pushes the resume value (count) or leaves state alone (button), and re-enters `Execute` via `resumeOrFinish`.

**Spec:** [`docs/superpowers/specs/2026-04-21-runescript-s5g-dialog-suspension-design.md`](../specs/2026-04-21-runescript-s5g-dialog-suspension-design.md)

---

## Task 1: ActivePlayer + mockPlayer + wire op

**Files:**
- Modify: `pkg/io/protocol/game/server/prot.go` (add OpPCountDialog)
- Modify: `pkg/script/active.go` (add LastCom, SendCountDialog)
- Modify: `pkg/script/runner_test.go` (extend mockPlayer)

- [ ] **Step 1: Find OpPCountDialog's TS opcode number.** Read `$HOME/Code/github.com/LostCityRS/Engine-TS/src/network/game/server/ServerGameProt.ts` for `P_COUNTDIALOG` or `PCountDialog`. Find the `static readonly P_COUNTDIALOG = new ServerGameProt(N, M)` line. The payload size is 0 (server just tells client "show count dialog"; no data).

Add to `pkg/io/protocol/game/server/prot.go` near the other IF_/P_ ops:
```go
OpPCountDialog = Op{Opcode: N, PayloadSize: 0}
```

- [ ] **Step 2: Add methods to `ActivePlayer`** in `pkg/script/active.go` inside the existing interface, after the S5f additions:

```go
// S5g: dialog suspension.

// LastCom returns the component id most recently clicked on the client.
// Used by LAST_COM opcode and pause-button resume gating.
LastCom() int

// SendCountDialog writes a P_COUNTDIALOG wire packet to the active
// player's client, prompting an "enter a number" dialog. Called by
// the P_COUNTDIALOG script opcode before suspension.
SendCountDialog()
```

- [ ] **Step 3: Extend mockPlayer** in `pkg/script/runner_test.go`:

```go
lastComValue       int
sendCountDialogCalls int
```

And methods:
```go
func (m *mockPlayer) LastCom() int     { return m.lastComValue }
func (m *mockPlayer) SendCountDialog() { m.sendCountDialogCalls++ }
```

- [ ] **Step 4: Build**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./pkg/script/... ./pkg/io/...
```

Must succeed. Full build will fail at modules/world until Task 3 — fine.

- [ ] **Step 5: Commit**

```bash
git add pkg/io/protocol/game/server/prot.go pkg/script/active.go pkg/script/runner_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script,proto): S5g ActivePlayer.LastCom + SendCountDialog + OpPCountDialog

Adds LastCom (read player.lastCom) and SendCountDialog (send P_COUNTDIALOG
wire packet) to ActivePlayer. OpPCountDialog server wire op added with
the opcode number verified against TS ServerGameProt.ts. mockPlayer
fixture gains capture fields.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: Script handlers (P_PAUSEBUTTON, P_COUNTDIALOG, LAST_COM) + tests

**Files:**
- Create: `pkg/script/handlers_dialog.go` (new file, not appended to handlers_player.go to keep S5g changes isolated)
- Create: `pkg/script/handlers_dialog_test.go`
- Modify: `pkg/script/handlers.go`

- [ ] **Step 1: Verify opcode constants exist** in `pkg/script/opcode.go`:
- OpPPauseButton
- OpPCountDialog
- OpLastCom

Grep to confirm exact names. If missing, **stop and report** — they should be in S1 scaffolding.

- [ ] **Step 2: Read TS `$HOME/Code/github.com/LostCityRS/Engine-TS/src/engine/script/handlers/PlayerOps.ts`** lines 249-251, 368-371, 424-426 for exact handler behavior:

- `P_PAUSEBUTTON`: sets `state.execution = ScriptState.PAUSEBUTTON`; no stack changes.
- `P_COUNTDIALOG`: writes a PCountDialog packet to the active player's client, then sets `state.execution = ScriptState.COUNTDIALOG`.
- `LAST_COM`: `state.pushInt(state.activePlayer.lastCom)`.

- [ ] **Step 3: Create `pkg/script/handlers_dialog.go`**:

```go
package script

import "errors"

func handlePPauseButton(s *ScriptState) error {
    if s.Pointers&PtrActivePlayer == 0 || s.Self == nil {
        return errors.New("P_PAUSEBUTTON: no active player")
    }
    s.Execution = PauseButton
    return nil
}

func handlePCountDialog(s *ScriptState) error {
    if s.Pointers&PtrActivePlayer == 0 || s.Self == nil {
        return errors.New("P_COUNTDIALOG: no active player")
    }
    s.Self.SendCountDialog()
    s.Execution = CountDialog
    return nil
}

func handleLastCom(s *ScriptState) error {
    if s.Pointers&PtrActivePlayer == 0 || s.Self == nil {
        return errors.New("LAST_COM: no active player")
    }
    s.PushInt(s.Self.LastCom())
    return nil
}
```

- [ ] **Step 4: Register in `pkg/script/handlers.go`** at end of map:

```go
// S5g: dialog suspension.
OpPPauseButton:  handlePPauseButton,
OpPCountDialog:  handlePCountDialog,
OpLastCom:       handleLastCom,
```

- [ ] **Step 5: Create `pkg/script/handlers_dialog_test.go`** via Edit tool (not heredoc — `!=` bug):

```go
package script

import "testing"

func TestPPauseButtonSuspends(t *testing.T) {
    sf := &ScriptFile{
        Name:             "ppb",
        Opcodes:          []Opcode{OpPPauseButton, OpReturn},
        IntOperands:      []int32{0, 0},
        StringOperands:   []string{"", ""},
        InstructionCount: 2,
    }
    mp := &mockPlayer{}
    state := Init(sf, mp, false, nil, nil)
    if err := Execute(state); err != nil {
        t.Fatalf("Execute: %v", err)
    }
    if state.Execution != PauseButton {
        t.Errorf("Execution: got %v, want PauseButton", state.Execution)
    }
}

func TestPCountDialogSuspends(t *testing.T) {
    sf := &ScriptFile{
        Name:             "pcd",
        Opcodes:          []Opcode{OpPCountDialog, OpReturn},
        IntOperands:      []int32{0, 0},
        StringOperands:   []string{"", ""},
        InstructionCount: 2,
    }
    mp := &mockPlayer{}
    state := Init(sf, mp, false, nil, nil)
    if err := Execute(state); err != nil {
        t.Fatalf("Execute: %v", err)
    }
    if state.Execution != CountDialog {
        t.Errorf("Execution: got %v, want CountDialog", state.Execution)
    }
    if mp.sendCountDialogCalls != 1 {
        t.Errorf("sendCountDialogCalls: got %d, want 1", mp.sendCountDialogCalls)
    }
}

func TestLastCom(t *testing.T) {
    sf := &ScriptFile{
        Name:             "lc",
        Opcodes:          []Opcode{OpLastCom, OpReturn},
        IntOperands:      []int32{0, 0},
        StringOperands:   []string{"", ""},
        InstructionCount: 2,
    }
    mp := &mockPlayer{lastComValue: 42}
    state := Init(sf, mp, false, nil, nil)
    if err := Execute(state); err != nil {
        t.Fatalf("Execute: %v", err)
    }
    if got := state.PopInt(); got != 42 {
        t.Errorf("PopInt: got %d, want 42", got)
    }
}

func TestDialogOpsRequireActivePlayer(t *testing.T) {
    for _, op := range []Opcode{OpPPauseButton, OpPCountDialog, OpLastCom} {
        t.Run(op.String(), func(t *testing.T) {
            sf := &ScriptFile{
                Name:             "no_self",
                Opcodes:          []Opcode{op, OpReturn},
                IntOperands:      []int32{0, 0},
                StringOperands:   []string{"", ""},
                InstructionCount: 2,
            }
            state := Init(sf, nil, false, nil, nil)
            if err := Execute(state); err == nil {
                t.Errorf("%v: want error with nil Self", op)
            }
        })
    }
}
```

- [ ] **Step 6: Run**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/... -run 'TestPPauseButton|TestPCountDialog|TestLastCom|TestDialogOps'
```

- [ ] **Step 7: Commit**

```bash
git add pkg/script/handlers_dialog.go pkg/script/handlers_dialog_test.go pkg/script/handlers.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): S5g dialog suspension opcodes (P_PAUSEBUTTON, P_COUNTDIALOG, LAST_COM)

P_PAUSEBUTTON sets Execution=PauseButton (dispatch loop exits).
P_COUNTDIALOG calls Self.SendCountDialog() to write the wire packet,
then sets Execution=CountDialog.
LAST_COM pushes Self.LastCom().

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: Player impls + resumeOrFinish update + logout cleanup

**Files:**
- Modify: `modules/world/player_script.go` (+LastCom, SendCountDialog impls)
- Modify: `modules/world/script.go` (resumeOrFinish routing)
- Modify: `modules/world/tick.go` (logout cleanup)

- [ ] **Step 1: Add impls to `player_script.go`** after S5f block:

```go
// S5g: dialog suspension.

func (p *Player) LastCom() int { return p.lastCom }

func (p *Player) SendCountDialog() {
    p.writeOut(gameserver.OpPCountDialog, nil)
}
```

Import `gameserver` if not already in this file.

- [ ] **Step 2: Update `resumeOrFinish` in `modules/world/script.go`**:

```go
switch state.Execution {
case script.Finished, script.Aborted:
    self.ClearActiveScript()
case script.Suspended, script.PauseButton, script.CountDialog:
    self.StoreActiveScript(state)
default:
    // NpcSuspended / WorldSuspended — future sub-specs.
    s.log.Warn("script in unsupported execution state",
        "script", state.Script.Name, "execution", state.Execution)
    self.ClearActiveScript()
}
```

- [ ] **Step 3: Logout cleanup in `tick.go`**:

Find `processLogouts`. Add to the branch that executes the logout (where `p.loggingOut` is final):

```go
p.activeScript = nil
```

This prevents a stale resume attempt from a late client packet.

- [ ] **Step 4: Full build + test**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
```

All pass.

- [ ] **Step 5: Commit**

```bash
git add modules/world/player_script.go modules/world/script.go modules/world/tick.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): Player LastCom/SendCountDialog + resumeOrFinish handles
pause/count states + logout clears activeScript

LastCom returns p.lastCom; SendCountDialog writes OpPCountDialog.
resumeOrFinish now routes PauseButton and CountDialog to
StoreActiveScript alongside Suspended; the dispatch loop exits cleanly
and the tick / client handlers can resume. processLogouts explicitly
clears activeScript so late resume packets don't reference a stale
player state.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: Client resume handlers + E2E test

**Files:**
- Create: `modules/world/resume_dialog.go`
- Modify: `modules/world/handlers_game.go`
- Modify: `modules/world/script_test.go`

- [ ] **Step 1: Find the Go-side client opcode constants** for RESUME_PAUSEBUTTON (235) and RESUME_P_COUNTDIALOG (237). Grep `pkg/io/protocol/game/client/prot.go`. Likely names: `OpResumePauseButton`, `OpResumePCountDialog`. Report actual names in the commit if different.

- [ ] **Step 2: Create `modules/world/resume_dialog.go`**:

```go
package world

import (
    "github.com/zsrv/goscape/pkg/io/packet"
    "github.com/zsrv/goscape/pkg/script"
)

// handleResumePauseButton handles client opcode 235 (RESUME_PAUSEBUTTON).
// Body: u16 component-id of the clicked button.
func (s *Server) handleResumePauseButton(p *Player, buf *packet.Packet) error {
    com := int(buf.G2())
    p.lastCom = com

    if p.activeScript == nil || p.activeScript.Execution != script.PauseButton {
        return nil
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

    p.activeScript.Execution = script.Running
    s.resumeOrFinish(p.activeScript, p)
    return nil
}

// handleResumeCountDialog handles client opcode 237 (RESUME_P_COUNTDIALOG).
// Body: i32 count.
func (s *Server) handleResumeCountDialog(p *Player, buf *packet.Packet) error {
    count := int32(buf.G4())

    if p.activeScript == nil || p.activeScript.Execution != script.CountDialog {
        return nil
    }

    p.activeScript.PushInt(int(count))
    p.activeScript.Execution = script.Running
    s.resumeOrFinish(p.activeScript, p)
    return nil
}
```

- [ ] **Step 3: Register in `modules/world/handlers_game.go`**. Find the existing handler map and add:

```go
gameclient.OpResumePauseButton:   s.handleResumePauseButton,
gameclient.OpResumeCountDialog:   s.handleResumeCountDialog,
```

(Use the actual Go-side const names from Step 1.)

- [ ] **Step 4: Add E2E test in `modules/world/script_test.go`** (Edit tool, not heredoc):

```go
func TestPauseButtonResumesAfterClick(t *testing.T) {
    s := newTestServer(t)
    s.scriptProvider = script.NewProvider()
    s.configsView = serverConfigsView{s: s}
    s.invLookup = invLookupView{s: s}
    p, cc := newTestPlayer(t)
    p.client.server = s
    p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
    p.resumeButtons = [5]int{7, 0, 0, 0, 0}

    // Script: push "before", mes, p_pausebutton, push "after", mes, return
    sf := &script.ScriptFile{
        Name: "[pausebutton,test]",
        Opcodes: []script.Opcode{
            script.OpPushConstantString,
            script.OpMes,
            script.OpPPauseButton,
            script.OpPushConstantString,
            script.OpMes,
            script.OpReturn,
        },
        IntOperands:      []int32{0, 0, 0, 0, 0, 0},
        StringOperands:   []string{"before", "", "", "after", "", ""},
        InstructionCount: 6,
    }

    received := drainConn(t, cc)
    s.runScript(sf, p, true, nil, nil)
    p.client.flushWrite()
    first := <-received

    if p.activeScript == nil {
        t.Fatal("expected activeScript to be set after p_pausebutton")
    }
    if p.activeScript.Execution != script.PauseButton {
        t.Errorf("Execution: got %v, want PauseButton", p.activeScript.Execution)
    }

    // Simulate RESUME_PAUSEBUTTON with com=7.
    buf := packet.NewPacket([]byte{0, 7})
    if err := s.handleResumePauseButton(p, buf); err != nil {
        t.Fatalf("resume: %v", err)
    }
    p.client.flushWrite()

    received2 := drainConn(t, cc)
    second := <-received2

    // first should contain "before\n"; second should contain "after\n"
    if string(first[2:8]) != "before" {
        t.Errorf("first payload: got %q", first[2:])
    }
    if string(second[2:7]) != "after" {
        t.Errorf("second payload: got %q", second[2:])
    }
    if p.activeScript != nil {
        t.Errorf("activeScript after resume-and-finish: got %v, want nil", p.activeScript)
    }
}
```

- [ ] **Step 5: Run tests + race + vet**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestPauseButton -v
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...
```

Handler count: `grep -cE "^\s+Op[A-Z].*handle" pkg/script/handlers.go` → **173** (170 + 3).

- [ ] **Step 6: Commit**

```bash
git add modules/world/resume_dialog.go modules/world/handlers_game.go modules/world/script_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): client handlers for RESUME_PAUSEBUTTON + RESUME_P_COUNTDIALOG

handleResumePauseButton validates activeScript is in PauseButton state
and clicked com is in resumeButtons, then flips Execution=Running and
re-enters resumeOrFinish. handleResumeCountDialog pushes the int count
onto the activeScript's stack before resuming.

E2E TestPauseButtonResumesAfterClick: script emits "before", pauses,
resumes on com=7 click, emits "after", completes.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Self-Review

- [ ] `go build ./...` clean
- [ ] `go test ./...` passes
- [ ] `go test -race ./...` clean
- [ ] `go vet ./...` clean
- [ ] Handler count = 173
