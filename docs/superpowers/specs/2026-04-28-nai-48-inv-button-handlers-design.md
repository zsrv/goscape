# NAI-48 — INV_BUTTON1–5 + INV_BUTTOND Opcode Handlers

## Motivation

Client opcodes INV_BUTTON1–5 (31, 59, 212, 38, 6) and INV_BUTTOND (159) are
already accepted by the login handshake (`Ops[]` in `prot.go`) and have
`ServerTriggerType` values registered in `trigger.go`, but no `gameHandlers[]`
entry is wired — the server silently discards all six opcodes. Scripts that
listen on `[inv_button1,<com>]` through `[inv_buttond,<com>]` can never fire.

This sub-spec ports the two TS handlers and wires them in.

**TS reference:**
- `Engine-TS/src/network/game/client/handler/InvButtonHandler.ts`
- `Engine-TS/src/network/game/client/handler/InvButtonDHandler.ts`
- `Engine-TS/src/network/game/client/codec/InvButtonDecoder.ts`
- `Engine-TS/src/network/game/client/codec/InvButtonDDecoder.ts`
- `Engine-TS/src/network/game/server/model/UpdateInvPartial.ts`
- `Engine-TS/src/network/game/server/codec/UpdateInvPartialEncoder.ts`

## Tech Stack

**Go 1.26+** (modern syntax). All `go` commands:
`GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go ...`. All commits: `--no-gpg-sign`.

## Deviations

**NAI-48-D1** — Component-registry checks skipped for both handlers.

TS `InvButtonHandler` validates:
- `Component.get(comId)` is defined
- `player.isComponentVisible(com)`
- `com.iop && com.iop[op-1] !== null`
- `root.overlay == false` → protect flag

TS `InvButtonDHandler` validates:
- `Component.get(comId)` is defined and `com.draggable`
- `player.isComponentVisible(com)`
- `root.overlay == false` → protect flag

All of the above are skipped — no component registry exists.
Protect defaults to `true` in all run paths.
Same cluster as: NAI-45-D1, NAI-45-D2, S6m-D2, S6o-D1,
NAI-40-D-COMPONENT-REGISTRY-VALIDATION-SKIPPED.
**Closure:** component-registry sub-spec.

## Scope

**New files:**

- `modules/world/handler_inv_button.go` — Server methods
- `modules/world/handler_inv_button_test.go` — 13 tests

**Modified files:**

- `modules/world/inv_update.go` — add `sendUpdateInvPartial`
- `modules/world/handlers_game.go` — register 6 handlers + 6 adapter funcs

**Out of scope:**

- `INV_BUTTON` mode byte (TS `InvButtonDDecoder` has a `mode` field that is
  decoded but the TS handler comments `// todo: is it necessary to pass
  message.mode to script?` — goscape skips it; a future sub-spec can revisit)
- Component registry validation (NAI-48-D1)
- Debug `messageGame` fallback (TS `Environment.NODE_DEBUG` guard — no debug
  mode in goscape; omitted rather than tracked as a deviation)

---

## Pre-flight (HEAD `ddd18aa`)

| Claim | Result |
|---|---|
| `TriggerInvButton1..D` at `trigger.go:152-157` | ✓ |
| Opcodes 31/59/212/38/6/159 in `prot.go` (all size 6) | ✓ |
| No `gameHandlers[]` entry for any of these opcodes | ✓ absent |
| `invListeners map[int]InventoryListener` on `Player` | ✓ |
| `resolveListenerInv` in `handler_opnpc.go` | ✓ |
| `lastItem`, `lastSlot`, `lastTargetSlot` on `Player` (init to -1) | ✓ |
| `inv.HasAt`, `inv.Get`, `inv.Capacity` on `*Inventory` | ✓ |
| `OpUpdateInvPartial` at `pkg/io/protocol/game/server/prot.go:50` | ✓ |
| `sendUpdateInvFullCom` in `inv_update.go` — model for the new sender | ✓ |
| No `sendUpdateInvPartial` anywhere in goscape | ✓ absent |
| No `handler_inv_button.go` | ✓ absent |

---

## File Map

| Action | Path | What changes |
|---|---|---|
| CREATE | `modules/world/handler_inv_button.go` | `handleInvButton` + `handleInvButtonD` Server methods |
| CREATE | `modules/world/handler_inv_button_test.go` | 13 tests |
| MODIFY | `modules/world/inv_update.go` | Add `sendUpdateInvPartial` after `sendUpdateInvFullCom` |
| MODIFY | `modules/world/handlers_game.go` | 6 init() registrations + 6 adapter functions |

---

## Task 1 — `sendUpdateInvPartial` in `inv_update.go`

**File:** `modules/world/inv_update.go`

**TS reference:** `UpdateInvPartialEncoder.ts:9-32`.

Wire format: `p2(com)` then per-slot: `p1(slot) p2(id+1) p1(count)` or
`p1(255) p4(count)` for count≥255, or `p2(0) p1(0)` for empty slots.

Add after `sendUpdateInvFullCom` (line 41):

```go
// sendUpdateInvPartial writes an UpdateInvPartial packet for the listed slots.
// Used by handleInvButtonD to revert the client drag visual when the player
// is delayed. Mirrors TS UpdateInvPartialEncoder.ts:9-32.
func sendUpdateInvPartial(p *Player, com int, inv *inventory.Inventory, slots ...int) {
	buf := packet.NewPacket(nil)
	buf.P2(uint16(com))
	for _, slot := range slots {
		item := inv.Get(slot)
		buf.P1(uint8(slot))
		if item != nil {
			buf.P2(uint16(item.Id + 1))
			if item.Count >= 255 {
				buf.P1(255)
				buf.P4(uint32(item.Count))
			} else {
				buf.P1(uint8(item.Count))
			}
		} else {
			buf.P2(0)
			buf.P1(0)
		}
	}
	p.writeOut(gameserver.OpUpdateInvPartial, buf.Bytes())
}
```

### 1a. Compile check

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./modules/world/... 2>&1
```

Expected: no output.

### 1b. Commit

```bash
git add modules/world/inv_update.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-48 T1 — sendUpdateInvPartial (UpdateInvPartial revert)

Adds sendUpdateInvPartial(p, com, inv, slots...) to inv_update.go.
Encodes per-slot p1(slot)+p2(id+1)+p1(count) or large-count variant,
empty slots as p2(0)+p1(0). Mirrors TS UpdateInvPartialEncoder.ts:9-32.
Used by handleInvButtonD to revert client drag visual when player delayed.

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 2 — Handler implementations in `handler_inv_button.go`

**File:** `modules/world/handler_inv_button.go` (new)

**TS references:** `InvButtonHandler.ts`, `InvButtonDHandler.ts`.

Create the file with the following content:

```go
package world

import (
	"github.com/zsrv/goscape/pkg/io/packet"
	"github.com/zsrv/goscape/pkg/script"
)

// handleInvButton is the shared implementation for INV_BUTTON1..INV_BUTTON5.
// op is 1..5. Wire format: obj:G2 | slot:G2 | com:G2 (6 bytes).
//
// Validation gates (mirrors TS InvButtonHandler.ts):
//  1. delayed player → drop
//  2. payload < 6 bytes → drop
//  3. comId not in invListeners → drop
//  4. listener's inventory unresolved → drop
//  5. inv.HasAt(slot, obj) false → drop (covers TS validSlot + hasAt)
//
// On pass: set p.lastItem=obj, p.lastSlot=slot, look up
// [inv_button<op>,<comId>] via GetByTrigger and run with protect=true.
//
// DEVIATION NAI-48-D1: component lookup, com.iop[op-1] null-check,
// isComponentVisible, and root.overlay protect computation skipped —
// no component registry. protect=true always. Same cluster as NAI-45-D1/D2.
// Closure: component-registry sub-spec.
func (s *Server) handleInvButton(p *Player, payload []byte, op int) error {
	if p.delayed && s.currentTick < p.delayedUntil {
		return nil
	}
	if len(payload) < 6 {
		return nil
	}
	r := packet.NewPacket(payload)
	obj := int(r.G2())
	slot := int(r.G2())
	comId := int(r.G2())

	listener, ok := p.invListeners[comId]
	if !ok {
		return nil
	}
	inv := resolveListenerInv(s, listener)
	if inv == nil {
		return nil
	}
	if !inv.HasAt(slot, obj) {
		return nil
	}

	p.lastItem = obj
	p.lastSlot = slot

	trigger := script.TriggerInvButton1 + script.ServerTriggerType(op-1)
	sf := s.scriptProvider.GetByTrigger(trigger, comId, -1)
	s.runScript(sf, p, nil, true, nil, nil)
	return nil
}

// handleInvButtonD is the handler for INV_BUTTOND (opcode 159, 6-byte payload).
// Inventory drag-and-drop: player drags an item from slot to targetSlot within
// the same UI component. Wire format: com:G2 | slot:G2 | targetSlot:G2.
//
// Validation gates (mirrors TS InvButtonDHandler.ts — NOTE: delayed check
// is intentionally AFTER slot/item validation so the client visual can be
// reverted):
//  1. comId not in invListeners → drop
//  2. listener's inventory unresolved → drop
//  3. slot or targetSlot out of inv.Capacity bounds → drop
//  4. source slot empty (inv.Get(slot)==nil) → drop
//  5. player delayed → sendUpdateInvPartial to revert drag visual, then drop
//
// On pass: set p.lastSlot=slot, p.lastTargetSlot=targetSlot, look up
// [inv_buttond,<comId>] via GetByTrigger and run with protect=true.
//
// DEVIATION NAI-48-D1: component lookup, com.draggable, and
// isComponentVisible skipped — no component registry. protect=true always.
// Closure: component-registry sub-spec.
func (s *Server) handleInvButtonD(p *Player, payload []byte) error {
	if len(payload) < 6 {
		return nil
	}
	r := packet.NewPacket(payload)
	comId := int(r.G2())
	slot := int(r.G2())
	targetSlot := int(r.G2())

	listener, ok := p.invListeners[comId]
	if !ok {
		return nil
	}
	inv := resolveListenerInv(s, listener)
	if inv == nil {
		return nil
	}
	if slot < 0 || slot >= inv.Capacity || targetSlot < 0 || targetSlot >= inv.Capacity {
		return nil
	}
	if inv.Get(slot) == nil {
		return nil
	}

	if p.delayed && s.currentTick < p.delayedUntil {
		sendUpdateInvPartial(p, comId, inv, slot, targetSlot)
		return nil
	}

	p.lastSlot = slot
	p.lastTargetSlot = targetSlot

	sf := s.scriptProvider.GetByTrigger(script.TriggerInvButtonD, comId, -1)
	s.runScript(sf, p, nil, true, nil, nil)
	return nil
}
```

### 2a. Write failing tests first

Create `modules/world/handler_inv_button_test.go`:

```go
package world

import (
	"testing"

	io2 "github.com/zsrv/goscape/pkg/io/isaac"
	"github.com/zsrv/goscape/pkg/inventory"
	"github.com/zsrv/goscape/pkg/script"
)

// invButtonPayload encodes a 6-byte INV_BUTTON1-5 payload (obj:G2 slot:G2 com:G2).
func invButtonPayload(obj, slot, com int) []byte {
	return []byte{
		byte(obj >> 8), byte(obj),
		byte(slot >> 8), byte(slot),
		byte(com >> 8), byte(com),
	}
}

// invButtonDPayload encodes a 6-byte INV_BUTTOND payload (com:G2 slot:G2 targetSlot:G2).
func invButtonDPayload(com, slot, targetSlot int) []byte {
	return []byte{
		byte(com >> 8), byte(com),
		byte(slot >> 8), byte(slot),
		byte(targetSlot >> 8), byte(targetSlot),
	}
}

// setupInvButtonServer returns a Server + Player pre-wired with a world inv at
// invType=93, com=149, source=-1. Item id=555, count=1 lives at slot=3.
func setupInvButtonServer(t *testing.T) (*Server, *Player) {
	t.Helper()
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	s.configsView = serverConfigsView{s: s}
	s.invLookup = invLookupView{s: s}
	s.npcLookup = serverNpcLookup{s: s}
	s.invs = make(map[int]*inventory.Inventory)
	inv := inventory.New(93, 28, inventory.StackNormal)
	inv.Items[3] = &inventory.Item{Id: 555, Count: 1}
	s.invs[93] = inv
	p, _ := newTestPlayer(t)
	p.client.server = s
	p.invListenOnCom(93, 149, -1)
	return s, p
}

// --- INV_BUTTON1-5 tests ---

// TestHandleInvButtonDelayed pins that a delayed player causes an early drop
// with no state mutation (mirrors TS InvButtonHandler.ts:14-17).
func TestHandleInvButtonDelayed(t *testing.T) {
	s, p := setupInvButtonServer(t)
	s.currentTick = 5
	p.delayed = true
	p.delayedUntil = 10

	_ = s.handleInvButton(p, invButtonPayload(555, 3, 149), 1)

	if p.lastItem != -1 {
		t.Errorf("lastItem: got %d, want -1 (not set when delayed)", p.lastItem)
	}
	if p.lastSlot != -1 {
		t.Errorf("lastSlot: got %d, want -1 (not set when delayed)", p.lastSlot)
	}
}

// TestHandleInvButtonShortPayload pins that payloads under 6 bytes are dropped.
func TestHandleInvButtonShortPayload(t *testing.T) {
	s, p := setupInvButtonServer(t)

	_ = s.handleInvButton(p, []byte{0, 0, 0, 0}, 1)

	if p.lastItem != -1 || p.lastSlot != -1 {
		t.Error("state mutated on short payload")
	}
}

// TestHandleInvButtonNoListener pins that a comId absent from invListeners
// causes a drop (mirrors TS InvButtonHandler.ts:30-36).
func TestHandleInvButtonNoListener(t *testing.T) {
	s, p := setupInvButtonServer(t)

	// com=999 not registered
	_ = s.handleInvButton(p, invButtonPayload(555, 3, 999), 1)

	if p.lastItem != -1 {
		t.Error("lastItem mutated despite no listener")
	}
}

// TestHandleInvButtonNilInv pins that a listener whose inv cannot be resolved
// causes a drop (mirrors TS InvButtonHandler.ts:37-41).
func TestHandleInvButtonNilInv(t *testing.T) {
	s, p := setupInvButtonServer(t)
	delete(s.invs, 93) // break the world-inv so resolveListenerInv returns nil

	_ = s.handleInvButton(p, invButtonPayload(555, 3, 149), 1)

	if p.lastItem != -1 {
		t.Error("lastItem mutated despite nil inventory")
	}
}

// TestHandleInvButtonItemMismatch pins that HasAt(slot, obj) false causes a
// drop (mirrors TS InvButtonHandler.ts:43-47: validSlot + hasAt checks).
func TestHandleInvButtonItemMismatch(t *testing.T) {
	s, p := setupInvButtonServer(t)

	// obj=9999 is not at slot 3 (inv has id=555)
	_ = s.handleInvButton(p, invButtonPayload(9999, 3, 149), 1)

	if p.lastItem != -1 {
		t.Error("lastItem mutated despite item mismatch")
	}
}

// TestHandleInvButtonSetsStateAndRunsScript pins the happy path: valid payload
// sets lastItem/lastSlot and fires the matching [inv_button1,<com>] script
// (mirrors TS InvButtonHandler.ts:49-58).
func TestHandleInvButtonSetsStateAndRunsScript(t *testing.T) {
	s, p := setupInvButtonServer(t)
	sf := &script.ScriptFile{
		Name:      "[inv_button1,149]",
		LookupKey: script.LookupKeyForType(script.TriggerInvButton1, 149),
		Opcodes:   []script.Opcode{script.OpReturn},
		IntOperands: []int32{0}, StringOperands: []string{""}, InstructionCount: 1,
	}
	s.scriptProvider.Register(sf)

	_ = s.handleInvButton(p, invButtonPayload(555, 3, 149), 1)

	if p.lastItem != 555 {
		t.Errorf("lastItem: got %d, want 555", p.lastItem)
	}
	if p.lastSlot != 3 {
		t.Errorf("lastSlot: got %d, want 3", p.lastSlot)
	}
	// Script returns immediately — no suspension.
	if p.activeScript != nil {
		t.Error("activeScript: want nil after RETURN, got non-nil")
	}
}

// TestHandleInvButtonOpVariant pins that op=2 looks up TriggerInvButton2
// (not TriggerInvButton1). Registers a Button2-specific script and
// confirms it fires for op=2.
func TestHandleInvButtonOpVariant(t *testing.T) {
	s, p := setupInvButtonServer(t)
	sf := &script.ScriptFile{
		Name:      "[inv_button2,149]",
		LookupKey: script.LookupKeyForType(script.TriggerInvButton2, 149),
		Opcodes:   []script.Opcode{script.OpReturn},
		IntOperands: []int32{0}, StringOperands: []string{""}, InstructionCount: 1,
	}
	s.scriptProvider.Register(sf)

	_ = s.handleInvButton(p, invButtonPayload(555, 3, 149), 2)

	// Script for Button2 fired and returned — no suspension.
	if p.activeScript != nil {
		t.Error("activeScript: want nil after RETURN (Button2 script should have fired)")
	}
	// Button1 was NOT registered; the lack of Button1 script with op=2 passing
	// confirms the trigger offset computation is correct.
}

// --- INV_BUTTOND tests ---

// TestHandleInvButtonDNoListener pins that a comId absent from invListeners
// causes a drop (mirrors TS InvButtonDHandler.ts:18-22).
func TestHandleInvButtonDNoListener(t *testing.T) {
	s, p := setupInvButtonServer(t)

	_ = s.handleInvButtonD(p, invButtonDPayload(999, 3, 5))

	if p.lastSlot != -1 {
		t.Error("lastSlot mutated despite no listener")
	}
}

// TestHandleInvButtonDNilInv pins that an unresolvable inv causes a drop.
func TestHandleInvButtonDNilInv(t *testing.T) {
	s, p := setupInvButtonServer(t)
	delete(s.invs, 93)

	_ = s.handleInvButtonD(p, invButtonDPayload(149, 3, 5))

	if p.lastSlot != -1 {
		t.Error("lastSlot mutated despite nil inventory")
	}
}

// TestHandleInvButtonDSlotOOB pins that a slot or targetSlot outside
// inv.Capacity causes a drop (mirrors TS InvButtonDHandler.ts:31-35:
// validSlot(slot) || validSlot(targetSlot) false).
func TestHandleInvButtonDSlotOOB(t *testing.T) {
	s, p := setupInvButtonServer(t)

	// inv capacity=28; slot=28 is OOB
	_ = s.handleInvButtonD(p, invButtonDPayload(149, 28, 5))

	if p.lastSlot != -1 {
		t.Error("lastSlot mutated despite OOB slot")
	}
}

// TestHandleInvButtonDSourceEmpty pins that an empty source slot causes a drop
// (mirrors TS InvButtonDHandler.ts:36-39: inv.get(slot) falsy).
func TestHandleInvButtonDSourceEmpty(t *testing.T) {
	s, p := setupInvButtonServer(t)

	// slot=10 has no item (only slot 3 is populated)
	_ = s.handleInvButtonD(p, invButtonDPayload(149, 10, 5))

	if p.lastSlot != -1 {
		t.Error("lastSlot mutated despite empty source slot")
	}
}

// TestHandleInvButtonDDelayedRevert pins that a delayed player triggers
// an UpdateInvPartial revert packet and does NOT set lastSlot/lastTargetSlot
// (mirrors TS InvButtonDHandler.ts:41-44: UpdateInvPartial + return false).
func TestHandleInvButtonDDelayedRevert(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	s.invs = make(map[int]*inventory.Inventory)
	s.currentTick = 5
	inv := inventory.New(93, 28, inventory.StackNormal)
	inv.Items[3] = &inventory.Item{Id: 555, Count: 1}
	s.invs[93] = inv

	p, cc := newTestPlayer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	p.invListenOnCom(93, 149, -1)
	p.delayed = true
	p.delayedUntil = 10

	received := drainConn(t, cc)
	_ = s.handleInvButtonD(p, invButtonDPayload(149, 3, 5))
	p.client.flushWrite()
	got := <-received

	if len(got) == 0 {
		t.Error("delayed INV_BUTTOND: want UpdateInvPartial revert packet, got none")
	}
	if p.lastSlot != -1 {
		t.Errorf("lastSlot: got %d, want -1 (not set when delayed)", p.lastSlot)
	}
	if p.lastTargetSlot != -1 {
		t.Errorf("lastTargetSlot: got %d, want -1 (not set when delayed)", p.lastTargetSlot)
	}
}

// TestHandleInvButtonDSetsStateAndRunsScript pins the happy path: valid payload
// sets lastSlot/lastTargetSlot and fires [inv_buttond,<com>]
// (mirrors TS InvButtonDHandler.ts:46-55).
func TestHandleInvButtonDSetsStateAndRunsScript(t *testing.T) {
	s, p := setupInvButtonServer(t)
	sf := &script.ScriptFile{
		Name:      "[inv_buttond,149]",
		LookupKey: script.LookupKeyForType(script.TriggerInvButtonD, 149),
		Opcodes:   []script.Opcode{script.OpReturn},
		IntOperands: []int32{0}, StringOperands: []string{""}, InstructionCount: 1,
	}
	s.scriptProvider.Register(sf)

	_ = s.handleInvButtonD(p, invButtonDPayload(149, 3, 5))

	if p.lastSlot != 3 {
		t.Errorf("lastSlot: got %d, want 3", p.lastSlot)
	}
	if p.lastTargetSlot != 5 {
		t.Errorf("lastTargetSlot: got %d, want 5", p.lastTargetSlot)
	}
	if p.activeScript != nil {
		t.Error("activeScript: want nil after RETURN, got non-nil")
	}
}
```

### 2b. Run failing tests

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... \
  -run 'TestHandleInvButton' -v 2>&1 | head -20
```

Expected: compile failure (`handleInvButton` / `handleInvButtonD` undefined).

### 2c. Create `handler_inv_button.go`

Write the file as shown in the Task 2 description above.

### 2d. Run tests (should pass)

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... \
  -run 'TestHandleInvButton' -v 2>&1 | tail -25
```

Expected: all 13 tests PASS.

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... 2>&1 | tail -5
```

Expected: PASS.

### 2e. Commit

```bash
git add modules/world/handler_inv_button.go modules/world/handler_inv_button_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-48 T2 — handleInvButton + handleInvButtonD Server methods

Ports TS InvButtonHandler.ts and InvButtonDHandler.ts. INV_BUTTON1-5:
delayed-first gate, invListeners lookup, HasAt validation, set
lastItem/lastSlot, GetByTrigger(TriggerInvButton1+(op-1), comId, -1).
INV_BUTTOND: validation-first (listener, inv, bounds, source non-empty),
then delayed check reverts drag visual via sendUpdateInvPartial, then
sets lastSlot/lastTargetSlot and runs [inv_buttond,<com>] script.
protect=true always (NAI-48-D1).
13 tests cover all validation gates + success paths for both handlers.

DEVIATION NAI-48-D1: component registry checks skipped (iop, draggable,
isComponentVisible, root.overlay). Closure: component-registry sub-spec.

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 3 — Wire handlers in `handlers_game.go`

**File:** `modules/world/handlers_game.go`

### 3a. Add init() registrations

In the `init()` function (after `gameHandlers[155] = handleIfButton` on line 61),
add:

```go
	gameHandlers[31] = handleInvButton1  // INV_BUTTON1
	gameHandlers[59] = handleInvButton2  // INV_BUTTON2
	gameHandlers[212] = handleInvButton3 // INV_BUTTON3
	gameHandlers[38] = handleInvButton4  // INV_BUTTON4
	gameHandlers[6] = handleInvButton5   // INV_BUTTON5
	gameHandlers[159] = handleInvButtonD // INV_BUTTOND
```

### 3b. Add adapter functions

After `handleIfButton` (line 107), add:

```go
func handleInvButton1(p *Player, payload []byte) error {
	if p.client == nil || p.client.server == nil {
		return nil
	}
	return p.client.server.handleInvButton(p, payload, 1)
}

func handleInvButton2(p *Player, payload []byte) error {
	if p.client == nil || p.client.server == nil {
		return nil
	}
	return p.client.server.handleInvButton(p, payload, 2)
}

func handleInvButton3(p *Player, payload []byte) error {
	if p.client == nil || p.client.server == nil {
		return nil
	}
	return p.client.server.handleInvButton(p, payload, 3)
}

func handleInvButton4(p *Player, payload []byte) error {
	if p.client == nil || p.client.server == nil {
		return nil
	}
	return p.client.server.handleInvButton(p, payload, 4)
}

func handleInvButton5(p *Player, payload []byte) error {
	if p.client == nil || p.client.server == nil {
		return nil
	}
	return p.client.server.handleInvButton(p, payload, 5)
}

func handleInvButtonD(p *Player, payload []byte) error {
	if p.client == nil || p.client.server == nil {
		return nil
	}
	return p.client.server.handleInvButtonD(p, payload)
}

### 3c. Compile + full test run

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./... 2>&1
```

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./... 2>&1 | tail -10
```

Expected: clean build, all packages PASS.

### 3d. Commit

```bash
git add modules/world/handlers_game.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-48 T3 — wire INV_BUTTON1-5 + INV_BUTTOND in handlers_game.go

Registers 6 new gameHandlers entries (opcodes 31/59/212/38/6/159) and
adds 6 package-level adapter funcs that forward into the Server methods
added in T2. Completes the dispatch chain for all INV_BUTTON opcodes.

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Close commit

After all tasks pass:

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./... 2>&1 | grep -E 'ok|FAIL'
```

Expected: all packages `ok`, no `FAIL`.

```bash
git commit --no-gpg-sign --allow-empty -m "$(cat <<'EOF'
chore(close): NAI-48 — INV_BUTTON1-5 + INV_BUTTOND handlers

Closes memory: NAI-48-D1
EOF
)"
```

---

## Deviation tally

- Retired: 0
- Opened: 1 (NAI-48-D1 — component-registry checks)
- Net: +1
