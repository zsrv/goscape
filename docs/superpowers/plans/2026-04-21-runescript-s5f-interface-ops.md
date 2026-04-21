# RuneScript S5f: Interface Opcodes Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Register 18 IF_* handlers, add 10 new IfSet* wire opcodes, and extend `ActivePlayer` with 18 interface-control methods. Modal open/close piggybacks on existing `modalState`/`refreshModal` infrastructure; `IfSet*` handlers emit wire packets immediately (fire-and-forget, matching TS).

**Architecture:** One handler file (`pkg/script/handlers_interface.go`), 10 proto ops added to `pkg/io/protocol/game/server/prot.go`, wire emitters in a new `modules/world/player_interface.go`, and modal impls inline in `modules/world/player_script.go`. Zero per-component state storage — matches TS.

**Tech Stack:** Go 1.22+, existing Player modal state machine, existing `writeOut` / packet infrastructure.

**Spec:** [`docs/superpowers/specs/2026-04-21-runescript-s5f-interface-ops-design.md`](../specs/2026-04-21-runescript-s5f-interface-ops-design.md)

---

## Task 1: Add 10 IfSet* wire ops to server prot

**Files:**
- Modify: `pkg/io/protocol/game/server/prot.go`

- [ ] **Step 1: Open the spec's wire-format table.** For each IfSet* op, read `/home/owner/Code/github.com/LostCityRS/Engine-TS/src/network/game/server/ServerGameProt.ts` to get the **exact opcode number + payload size**. Do not guess — these are canonical IDs baked into the Java client. The existing 5 OpIfOpen/OpIfClose ops in prot.go use the TS numbers.

- [ ] **Step 2: Add 10 `Op` entries** to `prot.go`:
- `OpIfSetText` — verify size; likely `-2` (variable jstr payload)
- `OpIfSetModel` — fixed; com (2) + modelID (4) = 6
- `OpIfSetNpcHead` — 2+2 = 4
- `OpIfSetPlayerHead` — 2
- `OpIfSetAnim` — 2+2 = 4
- `OpIfSetHide` — 2+1 = 3
- `OpIfSetObject` — 2+2+4 = 8
- `OpIfSetColour` — 2+2 = 4
- `OpIfSetPosition` — 2+2+2 = 6
- `OpIfSetRecol` — 2+2+2 = 6
- `OpIfSetTab` — verify
- `OpIfSetTabActive` — verify

Group with a `// S5f: per-component setters.` comment block next to the existing `OpIfOpenMain` etc.

That's 12 ops total (counting IF_SETTAB and IF_SETTABACTIVE). Count precisely.

- [ ] **Step 3: Build**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...
```

- [ ] **Step 4: Commit**

```bash
git add pkg/io/protocol/game/server/prot.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(proto/server): IfSet* wire opcodes for S5f

Adds OpIfSetText (variable, -2 prefix), OpIfSetModel, OpIfSetNpcHead,
OpIfSetPlayerHead, OpIfSetAnim, OpIfSetHide, OpIfSetObject,
OpIfSetColour, OpIfSetPosition, OpIfSetRecol, OpIfSetTab,
OpIfSetTabActive. Opcode numbers and payload sizes verified against
TS ServerGameProt.ts.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: Extend `ActivePlayer` + `mockPlayer`

**Files:**
- Modify: `pkg/script/active.go`
- Modify: `pkg/script/runner_test.go`

- [ ] **Step 1: Add 18 methods to `ActivePlayer`** (see spec §2 for signatures). Group with a `// S5f: interface / modal control.` comment block after the S5e additions.

- [ ] **Step 2: Extend `mockPlayer` with matching fields + impls.** For modal methods, single captured-value fields (e.g. `lastOpenMain int` + `closeModalCalls int`). For IfSet* methods, a struct per family:

```go
lastIfSetText  struct{ com int; text string }
ifSetTextCalls int

lastIfSetModel  struct{ com, modelID int }
ifSetModelCalls int
// ... one per wire op ...

lastSetResumeButtons [5]int
```

Use your judgment on shape; make it easy for Task 3 handler tests to assert the last call.

- [ ] **Step 3: Build**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./pkg/script/...
```

Expected: pkg/script builds. modules/world will fail until Task 4; that's OK.

- [ ] **Step 4: Commit**

```bash
git add pkg/script/active.go pkg/script/runner_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): S5f ActivePlayer extensions for interface control

Adds 18 methods: CloseModal + Open{Main,Chat,Side,MainSide} for modal
state, 12 IfSet* fire-and-forget wire emitters, and SetResumeButtons.
mockPlayer fixture gains matching capture fields for handler tests.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: Handlers + tests + map registration

**Files:**
- Create: `pkg/script/handlers_interface.go`
- Create: `pkg/script/handlers_interface_test.go`
- Modify: `pkg/script/handlers.go`

- [ ] **Step 1: Verify opcode constants exist** in `pkg/script/opcode.go`. The survey found 18 OpIf* declared:

OpIfClose, OpIfOpenChat, OpIfOpenMainSide, OpIfOpenMain, OpIfOpenSide, OpIfSetAnim, OpIfSetColour, OpIfSetHide, OpIfSetModel, OpIfSetNpcHead, OpIfSetObject, OpIfSetPlayerHead, OpIfSetPosition, OpIfSetRecol, OpIfSetResumeButtons, OpIfSetTab, OpIfSetTabActive, OpIfSetText.

- [ ] **Step 2: Read TS `/home/owner/Code/github.com/LostCityRS/Engine-TS/src/engine/script/handlers/PlayerOps.ts`** lines 245, 641-757 for exact pop order per handler. Pop orders to verify:

- IF_OPENMAIN: pops `com`
- IF_OPENCHAT: pops `com`
- IF_OPENSIDE: pops `com`
- IF_OPENMAIN_SIDE: pops `(main, side)` — side on top
- IF_CLOSE: no pops
- IF_SETTEXT: pops `(com, text)` — the survey said `(text, com)` but verify; TS `popInts` vs `popString` ordering matters
- IF_SETMODEL: `(com, model)`
- IF_SETNPCHEAD: `(com, npc)`
- IF_SETPLAYERHEAD: `(com)`
- IF_SETANIM: `(com, seq)`
- IF_SETHIDE: `(com, hide)` — hide is a 0/1 boolean
- IF_SETTAB: `(com, tab)`
- IF_SETOBJECT: `(com, obj, scale)`
- IF_SETCOLOUR: `(com, colour)`
- IF_SETPOSITION: `(com, x, y)`
- IF_SETRECOL: `(com, src, dst)`
- IF_SETTABACTIVE: `(tab)`
- IF_SETRESUMEBUTTONS: `(b1, b2, b3, b4, b5)` — five ints

- [ ] **Step 3: Write `pkg/script/handlers_interface.go`** with all 18 handlers. Each:
- Validates `s.Pointers&PtrActivePlayer != 0 && s.Self != nil`, returns `fmt.Errorf` on fail.
- Pops args in correct order.
- Calls the corresponding `Self.XXX(...)` method.
- Returns nil.

Group with `// -- Modal management --`, `// -- Per-component setters --`, `// -- Misc --` sub-comments.

- [ ] **Step 4: Register 18 handlers in `pkg/script/handlers.go`** at the end of the map with an `// S5f: interface / modal.` comment block.

- [ ] **Step 5: Write `pkg/script/handlers_interface_test.go`** (use Edit/Write tool, NOT bash heredoc):

- One test per handler using the mockPlayer capture fields.
- At minimum 18 happy-path tests + a few "requires active player" negatives.
- Example:

```go
func TestIfOpenMain(t *testing.T) {
    sf := &ScriptFile{
        Name:             "if_openmain",
        Opcodes:          []Opcode{OpPushConstantInt, OpIfOpenMain, OpReturn},
        IntOperands:      []int32{1234, 0, 0},
        StringOperands:   []string{"", "", ""},
        InstructionCount: 3,
    }
    mp := &mockPlayer{}
    state := Init(sf, mp, false, nil, nil)
    if err := Execute(state); err != nil {
        t.Fatalf("Execute: %v", err)
    }
    if mp.lastOpenMain != 1234 {
        t.Errorf("OpenMain: got %d, want 1234", mp.lastOpenMain)
    }
}
```

- [ ] **Step 6: Run tests**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/... -run 'TestIf'
```

Must pass.

- [ ] **Step 7: Commit**

```bash
git add pkg/script/handlers_interface.go pkg/script/handlers_interface_test.go pkg/script/handlers.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): S5f interface opcodes (18 handlers)

Modal management (5): IF_CLOSE, IF_OPENMAIN, IF_OPENCHAT, IF_OPENSIDE,
IF_OPENMAIN_SIDE — delegate to ActivePlayer.OpenX/CloseModal.
Visual setters (5): IF_SETTEXT, IF_SETMODEL, IF_SETNPCHEAD,
IF_SETPLAYERHEAD, IF_SETANIM.
Layout (5): IF_SETHIDE, IF_SETTAB, IF_SETOBJECT, IF_SETCOLOUR,
IF_SETPOSITION.
Misc (3): IF_SETRECOL, IF_SETRESUMEBUTTONS, IF_SETTABACTIVE.

Pop orders verified against TS PlayerOps.ts.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: Player impls + wire emitters

**Files:**
- Modify: `modules/world/player.go` (+resumeButtons field if missing)
- Modify: `modules/world/player_script.go` (+modal impls)
- Create: `modules/world/player_interface.go` (wire emitters)

- [ ] **Step 1: Investigate existing modal state**

Read `modules/world/player.go` to find:
- `modalMain, modalChat, modalSide int` fields (exist per survey)
- `modalState int` bitmask + `modalStateMain`/`modalStateChat`/`modalStateSide` constants (exist per survey)
- `refreshModal bool`, `refreshModalClose bool` (exist per survey)
- Existing `encodeOut()` flow for emitting the modal packets

Check for `resumeButtons` field; add `resumeButtons [5]int` if missing.

- [ ] **Step 2: Write modal impls in `player_script.go`** (append after S5e methods):

```go
// S5f: interface / modal control.

func (p *Player) CloseModal() {
    p.modalMain = -1
    p.modalChat = -1
    p.modalSide = -1
    p.modalState = 0
    p.refreshModalClose = true
}

func (p *Player) OpenMain(com int) {
    // Opening main closes chat + side (per TS Player.ts:1928-2022).
    p.modalMain = com
    p.modalChat = -1
    p.modalSide = -1
    p.modalState = modalStateMain
    p.refreshModal = true
}

func (p *Player) OpenChat(com int) {
    // Opening chat keeps main, closes side.
    p.modalChat = com
    p.modalSide = -1
    p.modalState |= modalStateChat
    p.modalState &^= modalStateSide
    p.refreshModal = true
}

func (p *Player) OpenSide(com int) {
    // Opening side keeps main, closes chat.
    p.modalSide = com
    p.modalChat = -1
    p.modalState |= modalStateSide
    p.modalState &^= modalStateChat
    p.refreshModal = true
}

func (p *Player) OpenMainSide(mainCom, sideCom int) {
    p.modalMain = mainCom
    p.modalSide = sideCom
    p.modalChat = -1
    p.modalState = modalStateMain | modalStateSide
    p.refreshModal = true
}

func (p *Player) SetResumeButtons(b1, b2, b3, b4, b5 int) {
    p.resumeButtons = [5]int{b1, b2, b3, b4, b5}
}
```

**VERIFY against TS `Player.ts:1928-2022`** — the exact "opening X closes/keeps Y" rules may differ from the spec's guess. Adapt.

- [ ] **Step 3: Create `modules/world/player_interface.go`** with 12 wire emitters. All follow the same shape: build a packet, call `writeOut`. Example:

```go
package world

import (
    "github.com/zsrv/goscape/pkg/io/packet"
    gameserver "github.com/zsrv/goscape/pkg/io/protocol/game/server"
)

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

func (p *Player) IfSetNpcHead(com, npcID int) {
    buf := packet.NewPacket(nil)
    buf.P2(uint16(com))
    buf.P2(uint16(npcID))
    p.writeOut(gameserver.OpIfSetNpcHead, buf.Bytes())
}

// ... repeat for IfSetPlayerHead (com only), IfSetAnim, IfSetHide
// (with u8 bool), IfSetTab, IfSetObject (com + obj + u32 scale),
// IfSetColour, IfSetPosition, IfSetRecol, IfSetTabActive.
```

Verify each wire format against TS `/home/owner/Code/github.com/LostCityRS/Engine-TS/src/network/game/server/codec/*Encoder.ts` files.

- [ ] **Step 4: Full build**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...
```

Clean. The `var _ script.ActivePlayer = (*Player)(nil)` assertion catches any missing impl.

- [ ] **Step 5: Run full test suite** for regression check:

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
```

- [ ] **Step 6: Commit**

```bash
git add modules/world/player.go modules/world/player_script.go modules/world/player_interface.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): Player impls for S5f interface methods

Modal impls (OpenMain/Chat/Side/MainSide, CloseModal, SetResumeButtons)
mutate modalState/refreshModal — existing encodeOut flushes the wire
packet on the next tick. IfSet* fire-and-forget emitters build the
packet and call writeOut immediately per TS behaviour.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 5: End-to-end tests

**Files:**
- Modify: `modules/world/script_test.go`

- [ ] **Step 1: Add E2E tests** using Edit tool:

**`TestIfOpenMainSetsModalState`:**
```go
func TestIfOpenMainSetsModalState(t *testing.T) {
    s := newTestServer(t)
    s.scriptProvider = script.NewProvider()
    s.configsView = serverConfigsView{s: s}
    s.invLookup = invLookupView{s: s}
    p, _ := newTestPlayer(t)
    p.client.server = s
    p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})

    sf := &script.ScriptFile{
        Name: "[ifopen,test]",
        Opcodes: []script.Opcode{
            script.OpPushConstantInt,
            script.OpIfOpenMain,
            script.OpReturn,
        },
        IntOperands:      []int32{1234, 0, 0},
        StringOperands:   []string{"", "", ""},
        InstructionCount: 3,
    }
    s.runScript(sf, p, true, nil, nil)

    if p.modalMain != 1234 {
        t.Errorf("modalMain: got %d, want 1234", p.modalMain)
    }
    if p.modalState & modalStateMain == 0 {
        t.Error("modalState: main bit not set")
    }
    if !p.refreshModal {
        t.Error("refreshModal: want true")
    }
}
```

**`TestIfSetTextEmitsWire`:**
```go
func TestIfSetTextEmitsWire(t *testing.T) {
    s := newTestServer(t)
    s.scriptProvider = script.NewProvider()
    s.configsView = serverConfigsView{s: s}
    s.invLookup = invLookupView{s: s}
    p, cc := newTestPlayer(t)
    p.client.server = s
    p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})

    // Script: push "hi", push 7, if_settext, return
    sf := &script.ScriptFile{
        Name: "[ifsettext,test]",
        Opcodes: []script.Opcode{
            script.OpPushConstantString,
            script.OpPushConstantInt,
            script.OpIfSetText,
            script.OpReturn,
        },
        IntOperands:      []int32{0, 7, 0, 0},
        StringOperands:   []string{"hi", "", "", ""},
        InstructionCount: 4,
    }

    received := drainConn(t, cc)
    s.runScript(sf, p, true, nil, nil)
    p.client.flushWrite()
    got := <-received

    // Wire = opcode(1) + len(1) + P2(com=7)(2) + PJStrLF("hi")(3) = 7 bytes
    if len(got) != 7 {
        t.Fatalf("wire: got %d bytes, want 7", len(got))
    }
    // got[1] = payload length byte
    // got[2..3] = P2(7)
    // got[4..5] = "hi"
    // got[6] = 0x0a
    if got[2] != 0 || got[3] != 7 {
        t.Errorf("com: got %d, want 7", (int(got[2])<<8)|int(got[3]))
    }
    if string(got[4:6]) != "hi" || got[6] != 0x0a {
        t.Errorf("text: got %q", got[4:])
    }
}
```

**VERIFY pop order for IF_SETTEXT against TS** before writing the test — the spec said `(com, text)` with text on top, but the handler might be the reverse. Adapt the script + assertion accordingly.

- [ ] **Step 2: Run tests**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestIf' -v
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...
```

All green.

- [ ] **Step 3: Handler count check**

```bash
grep -cE "^\s+Op[A-Z].*handle" pkg/script/handlers.go
```

Should be **170** (152 + 18).

- [ ] **Step 4: Commit**

```bash
git add modules/world/script_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
test(world): end-to-end S5f interface-opcode tests

TestIfOpenMainSetsModalState: if_openmain(1234) sets modalMain,
modalStateMain bit, and refreshModal — the existing encodeOut flushes
OpIfOpenMain next tick.
TestIfSetTextEmitsWire: if_settext(com=7, text="hi") emits a 7-byte
OpIfSetText packet with the expected com + PJStrLF payload.

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
- [ ] Handler count = 170
