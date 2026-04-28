# NAI-40 — OPPLAYER trigger producer (player→player op-click) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close `NAI-39-D-ACTIVEPLAYER2-NO-OPPLAYER-PRODUCER` by porting the player→player op-click dispatch path. Client `OPPLAYER1..4` / `OPPLAYERT` / `OPPLAYERU` packets resolve to the target player's `[opplayer<N>,_]` / `[applayer<N>,_]` trigger script, running with `Self` = target and `Self2` = clicker via NAI-39's `case script.ActivePlayer:` arm in `buildPlayerScriptState`.

**Architecture:** Bottom-up by layer — client-message decoders (T1) → server lookup + sentinels (T2) → three handlers (T3, T4, T5) → trigger maps + fire functions + tryFire dispatch (T6) → E2E smoke + deviation-comment retirement (T7). Mirrors NPC-side `npc_interaction_trigger.go` shape for the player-actor side. The Self2 binding flows through NAI-39's existing `runScript`/`buildPlayerScriptState` API (NOT the older manual `script.Init` pattern used by `fireOpTriggerNpc` / `fireOpTriggerLoc`) — this is the ActivePlayer-arm's first production producer.

**Tech Stack:** Go 1.26+ (per `go_version.md`; use `use-modern-go` skill). TS source: `Engine-TS` only per `ts_source_canonical_path.md`. HEAD baseline: `989cde1` (NAI-40 spec commit).

---

## Spec reference

Spec at `docs/superpowers/specs/2026-04-27-nai-40-opplayer-producer-design.md`. Test layers map to tasks as:
- **L1 (decoder)** → T1
- **L2 (trigger map)** → T6
- **L3 (handlers)** → T3 (OpPlayer 1..4), T4 (OpPlayerT), T5 (OpPlayerU)
- **L4 (processInteraction Player-arm)** — *resolved during pre-flight: no `processInteraction` body change needed; only `tryFire{Op,Ap}Trigger` type-switch extension* → T6
- **L5 (trigger fire + Self2 binding)** → T6
- **L6 (E2E smoke)** → T7

## Pre-flight notes (controller, performed at spec-write time)

- **`processInteraction` requires no body change.** Existing `interaction.go:74-122` is target-type-agnostic except for the `*Npc` SetFaceEntity branch (line 96-98). Player target falls through to `p.interacted = true; tryFireOpTrigger(p)` — the type-switch in `tryFireOpTrigger` is the single dispatch point. Confirmed by reading `interaction.go` end-to-end.
- **`effectiveApRange` falls through to `p.apRange` for non-Npc targets** (line 184-191). Player target uses `p.apRange` (default 10) — matches Loc behavior. No change.
- **`SetFaceEntity` is Npc-only by design** at `interaction.go:96-98`. TS `tryInteract` does NOT face-entity-on-player either (re-read `Player.ts:1100-1175`). **Conditional deviation `NAI-40-D-PLAYER-NO-FACEENTITY-ON-OPCLICK` is NOT needed** — both engines skip face-entity for Player target.
- **OPPLAYER3 follow-op semantics are NOT ported.** TS `Player.ts:1115-1209` has `followOp = targetOp === APPLAYER3 || targetOp === OPPLAYER3` chase logic. Goscape will fire-and-forget. Track as **`NAI-40-D-OPPLAYER3-FOLLOWOP-NOT-PORTED`** at handler exit point.
- **Existing fire functions use `script.Init` + manual pointer wiring** at `interaction_trigger.go:79-94`, predating NAI-39. **The new `fireOpTriggerPlayer` / `fireApTriggerPlayer` will use `srv.runScript(sf, target, p, true, nil, nil)`** — exercises NAI-39's `case script.ActivePlayer:` arm at `script.go:54-57`. This is the cleanest closure for the deviation.
- **Script-registry lookup signature:** `srv.scriptProvider.GetByTrigger(trigger, typeId, category)` (3 args). For Player target (untyped), pass `-1, -1`. **TBV at T6**: confirm registry returns the trigger's "no-typeId" script for these sentinels. If registry rejects `-1`, use a dedicated lookup overload — escalate as a T6 blocker.
- **`s.players` is the slot-indexed array** at `modules/world/server.go:618-686`. Slot 0 reserved (loop starts at `i := 1`). `LookupPlayerBySlot` mirrors `handler_opnpc.go:17-20` lookup pattern.
- **`OPPLAYER5` deliberately not in scope.** Real client only sends 1..4; trigger constant `TriggerOpPlayer<5>` does not exist (the trigger.go enum stops at 4 per `trigger.go:97-100`). NPC-AI side has `TriggerAiOpPlayer1..5` for AI mode dispatch — separate concern.
- **`p.ClearPendingAction()` exists** at `modules/world/player_script.go:630`. Already on `script.ActivePlayer` interface. ✓

## File structure

| File | Status | Responsibility |
|------|--------|----------------|
| `pkg/io/protocol/game/client/op_player.go` | new | 3 message structs + 3 decoder funcs (T1) |
| `pkg/io/protocol/game/client/op_player_test.go` | new | decoder unit tests (T1) |
| `modules/world/server.go` | modify | +`LookupPlayerBySlot` (T2) |
| `modules/world/server_test.go` | modify | +`LookupPlayerBySlot` unit tests (T2) |
| `modules/world/interaction.go` | modify | +`targetOpPlayerT/U` sentinel constants (T2) |
| `modules/world/handler_op_player.go` | new | `handleOpPlayer` (T3), `handleOpPlayerT` (T4), `handleOpPlayerU` (T5) |
| `modules/world/handler_op_player_test.go` | new | handler integration tests (T3, T4, T5) |
| `modules/world/player_interaction_trigger.go` | new | `apPlayerTriggerForOp` + `fireOpTriggerPlayer` + `fireApTriggerPlayer` (T6) |
| `modules/world/player_interaction_trigger_test.go` | new | trigger-map unit tests + fire-function tests (T6) |
| `modules/world/interaction_trigger.go` | modify | extend `tryFireOpTrigger` and `tryFireApTrigger` type-switches with `case *Player:` arms (T6) |
| `modules/world/script_test.go` | modify | +E2E smoke test (T7) |
| `pkg/script/state.go` | modify | retire `Self2` deviation comment lines 194-196 (T7) |
| `modules/world/script.go` | modify | retire `buildPlayerScriptState` deviation comment lines 30-33 (T7) |

## Pre-flight checks per task (controller)

Per `controller_preflight.md`: re-grep each premise against HEAD before dispatching each task.

| Task | Pre-dispatch verification |
|------|--------------------------|
| T1 | `rg -n "OPPLAYER" pkg/io/protocol/game/client/prot.go` returns lines 76-81 (opcode table entries 164/53/185/206/177/248). `pkg/io/protocol/game/client/op_npc.go` (or analogous) exists as decoder template. |
| T2 | `rg -n "s\.players\b" modules/world/server.go` returns lines 618-686 (slot array). `rg -n "targetOpLocT\\|targetOpLocU\\|targetOpNpcT\\|targetOpNpcU" modules/world/interaction.go` returns line 30-33. |
| T3 | `rg -n "func handleOp" modules/world/handler_op*.go` shows existing handler shapes. `rg -n "rsbuf\.HasPlayer" pkg/rsbuf/buf*.go` returns the API at HEAD. |
| T4 | T3 committed. `rg -n "Component\.Get\\|Component\\.\\(Get\\|isComponentVisible" modules/world/ pkg/...` shows existing component-validation pattern from OpNpcT handler. |
| T5 | T4 committed. `rg -n "lastUseItem\\|lastUseSlot" modules/world/handler_opnpc*.go` shows OpNpcU snapshot pattern. `rg -n "invListener\\|getInventoryFromListener\\|isComponentUsable" modules/world/ pkg/...` shows existing OpNpcU validation chain. |
| T6 | T5 committed. `rg -n "tryFireOpTrigger\\|tryFireApTrigger" modules/world/interaction_trigger.go` shows current type-switch (line 32-45 / 250-263). `rg -n "scriptProvider\.GetByTrigger" modules/world/` shows registry-lookup call sites. |
| T7 | T6 committed. `rg -n "NAI-39-D-ACTIVEPLAYER2-NO-OPPLAYER-PRODUCER" pkg/ modules/` returns exactly the 2 sites named in the deviation. |

---

## Task 1: client-message decoders for OpPlayer / OpPlayerT / OpPlayerU

**Goal:** Add wire-decoder for the three player→player op-click packets.

**Files:**
- New: `pkg/io/protocol/game/client/op_player.go`
- New: `pkg/io/protocol/game/client/op_player_test.go`

**TS reference:** `src/network/game/client/model/OpPlayer.ts`, `OpPlayerT.ts`, `OpPlayerU.ts`.

**Pre-flight:** read `pkg/io/protocol/game/client/op_npc.go` (or whichever existing per-opcode decoder file is the closest template) to mirror struct + decoder shape exactly.

- [ ] **Step 1.1: Read the analogous OpNpc decoder file**

Run: `find pkg/io/protocol/game/client -name "op_npc*" -type f` and read whichever decoder file follows the project's per-opcode convention. Mirror its struct + decoder + registration shape in the new `op_player.go`.

- [ ] **Step 1.2: Write 4 failing decoder tests**

Create `pkg/io/protocol/game/client/op_player_test.go`:

```go
package client

import (
	"bytes"
	"testing"

	"github.com/zsrv/goscape/pkg/io/packet"
)

// TestOpPlayer_DecodeOp1 pins the wire shape: u2 PlayerSlot for opcode 164.
// The Op field is set to 1 by the dispatcher (or by the decoder if it
// receives the opcode). For the OpPlayer{} struct, what matters is
// PlayerSlot decode + Op assignment.
func TestOpPlayer_DecodeOp1(t *testing.T) {
	var buf bytes.Buffer
	pk := packet.New(&buf)
	pk.P2(99) // PlayerSlot

	msg, err := DecodeOpPlayer(buf.Bytes(), 1 /* op */)
	if err != nil {
		t.Fatalf("DecodeOpPlayer: %v", err)
	}
	if msg.PlayerSlot != 99 {
		t.Errorf("PlayerSlot: got %d, want 99", msg.PlayerSlot)
	}
	if msg.Op != 1 {
		t.Errorf("Op: got %d, want 1", msg.Op)
	}
}

func TestOpPlayer_DecodeOp4(t *testing.T) {
	var buf bytes.Buffer
	pk := packet.New(&buf)
	pk.P2(2047)

	msg, err := DecodeOpPlayer(buf.Bytes(), 4)
	if err != nil {
		t.Fatalf("DecodeOpPlayer: %v", err)
	}
	if msg.PlayerSlot != 2047 {
		t.Errorf("PlayerSlot: got %d, want 2047", msg.PlayerSlot)
	}
	if msg.Op != 4 {
		t.Errorf("Op: got %d, want 4", msg.Op)
	}
}

// TestOpPlayerT_Decode pins (PlayerSlot, SpellCom) byte order.
// Wire body = 4 bytes total: u2 PlayerSlot, u2 SpellCom.
func TestOpPlayerT_Decode(t *testing.T) {
	var buf bytes.Buffer
	pk := packet.New(&buf)
	pk.P2(123) // PlayerSlot
	pk.P2(456) // SpellCom

	msg, err := DecodeOpPlayerT(buf.Bytes())
	if err != nil {
		t.Fatalf("DecodeOpPlayerT: %v", err)
	}
	if msg.PlayerSlot != 123 {
		t.Errorf("PlayerSlot: got %d, want 123", msg.PlayerSlot)
	}
	if msg.SpellCom != 456 {
		t.Errorf("SpellCom: got %d, want 456", msg.SpellCom)
	}
}

// TestOpPlayerU_Decode pins (PlayerSlot, UseObj, UseSlot, UseCom)
// byte order. Wire body = 8 bytes total.
func TestOpPlayerU_Decode(t *testing.T) {
	var buf bytes.Buffer
	pk := packet.New(&buf)
	pk.P2(50)   // PlayerSlot
	pk.P2(1511) // UseObj
	pk.P2(3)    // UseSlot
	pk.P2(789)  // UseCom

	msg, err := DecodeOpPlayerU(buf.Bytes())
	if err != nil {
		t.Fatalf("DecodeOpPlayerU: %v", err)
	}
	if msg.PlayerSlot != 50 {
		t.Errorf("PlayerSlot: got %d, want 50", msg.PlayerSlot)
	}
	if msg.UseObj != 1511 {
		t.Errorf("UseObj: got %d, want 1511", msg.UseObj)
	}
	if msg.UseSlot != 3 {
		t.Errorf("UseSlot: got %d, want 3", msg.UseSlot)
	}
	if msg.UseCom != 789 {
		t.Errorf("UseCom: got %d, want 789", msg.UseCom)
	}
}
```

**Note for implementer:** the actual `packet.P2` / `packet.New` API may differ — check `pkg/io/packet/`'s public API and use the canonical write methods. The test should write 2 bytes per field (big-endian u16) matching what `prot.go` declares as the body sizes (2/4/8).

- [ ] **Step 1.3: Run tests, confirm failures (RED)**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/io/protocol/game/client/ -run "TestOpPlayer"
```

Expected: 4 compilation errors (undefined `DecodeOpPlayer`, `DecodeOpPlayerT`, `DecodeOpPlayerU`, types `OpPlayer`, `OpPlayerT`, `OpPlayerU`).

- [ ] **Step 1.4: Implement decoders + structs**

Create `pkg/io/protocol/game/client/op_player.go`:

```go
package client

import (
	"github.com/zsrv/goscape/pkg/io/packet"
)

// OpPlayer covers the four player→player op-click variants 1..4.
// Wire body = u2 PlayerSlot. Op is set by the dispatcher from the
// opcode (164→1, 53→2, 185→3, 206→4) so the handler can do
// TriggerApPlayer1 + (Op-1) trigger arithmetic.
type OpPlayer struct {
	PlayerSlot int
	Op         int // 1..4
}

// OpPlayerT — wire body = u2 PlayerSlot, u2 SpellCom (opcode 177, size 4).
type OpPlayerT struct {
	PlayerSlot int
	SpellCom   int
}

// OpPlayerU — wire body = u2 PlayerSlot, u2 UseObj, u2 UseSlot, u2 UseCom
// (opcode 248, size 8).
type OpPlayerU struct {
	PlayerSlot int
	UseObj     int
	UseSlot    int
	UseCom     int
}

// DecodeOpPlayer parses an OPPLAYER1..4 packet body. The op argument
// (1..4) comes from the dispatcher — the wire body alone does not
// carry it.
func DecodeOpPlayer(body []byte, op int) (OpPlayer, error) {
	pk := packet.NewFromBytes(body)
	slot := pk.G2()
	return OpPlayer{PlayerSlot: slot, Op: op}, nil
}

func DecodeOpPlayerT(body []byte) (OpPlayerT, error) {
	pk := packet.NewFromBytes(body)
	slot := pk.G2()
	spell := pk.G2()
	return OpPlayerT{PlayerSlot: slot, SpellCom: spell}, nil
}

func DecodeOpPlayerU(body []byte) (OpPlayerU, error) {
	pk := packet.NewFromBytes(body)
	slot := pk.G2()
	useObj := pk.G2()
	useSlot := pk.G2()
	useCom := pk.G2()
	return OpPlayerU{
		PlayerSlot: slot,
		UseObj:     useObj,
		UseSlot:    useSlot,
		UseCom:     useCom,
	}, nil
}
```

**Note for implementer:** `packet.NewFromBytes` and `packet.G2` may have different actual names — adapt to the existing API in `pkg/io/packet/`. The shape of *how* an existing per-opcode decoder reads its body is the canonical reference.

- [ ] **Step 1.5: Run tests, confirm GREEN**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/io/protocol/game/client/ -run "TestOpPlayer"
```

- [ ] **Step 1.6: Run full suite + vet**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...
```

- [ ] **Step 1.7: Commit**

```
feat(io): NAI-40 T1 — OpPlayer{,T,U} client-message structs + decoders
```

---

## Task 2: `Server.LookupPlayerBySlot` + `targetOpPlayerT/U` sentinels

**Goal:** Add server-side slot lookup helper + the two new T/U sentinel constants.

**Files:**
- Modify: `modules/world/server.go` (insert `LookupPlayerBySlot` near line 706 after `LookupPlayerByUID`)
- Modify: `modules/world/server_test.go` (append unit tests)
- Modify: `modules/world/interaction.go` (extend the sentinel const block at line 29-34)

- [ ] **Step 2.1: Write 3 failing tests**

Append to `modules/world/server_test.go`:

```go
// TestLookupPlayerBySlot_Found returns the player at the slot.
func TestLookupPlayerBySlot_Found(t *testing.T) {
	s := newTestServer(t)
	p := newTestPlayer(t, s)
	slot := 5
	s.players[slot] = p
	t.Cleanup(func() { s.players[slot] = nil })

	got := s.LookupPlayerBySlot(slot)
	if got != p {
		t.Errorf("LookupPlayerBySlot(%d): got %v, want %p", slot, got, p)
	}
}

// TestLookupPlayerBySlot_OutOfRange returns nil for indices outside
// [0, len(s.players)).
func TestLookupPlayerBySlot_OutOfRange(t *testing.T) {
	s := newTestServer(t)
	if got := s.LookupPlayerBySlot(-1); got != nil {
		t.Errorf("LookupPlayerBySlot(-1): got %v, want nil", got)
	}
	if got := s.LookupPlayerBySlot(len(s.players)); got != nil {
		t.Errorf("LookupPlayerBySlot(len): got %v, want nil", got)
	}
	if got := s.LookupPlayerBySlot(len(s.players) + 100); got != nil {
		t.Errorf("LookupPlayerBySlot(len+100): got %v, want nil", got)
	}
}

// TestLookupPlayerBySlot_EmptySlotReturnsNil — slot is in range but no
// player logged in there.
func TestLookupPlayerBySlot_EmptySlotReturnsNil(t *testing.T) {
	s := newTestServer(t)
	if got := s.LookupPlayerBySlot(7); got != nil {
		t.Errorf("LookupPlayerBySlot(empty): got %v, want nil", got)
	}
}
```

**Note for implementer:** `newTestServer` / `newTestPlayer` helper-naming may differ; use whichever convention already exists in `server_test.go`.

- [ ] **Step 2.2: Run tests, confirm RED**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run "TestLookupPlayerBySlot"
```

- [ ] **Step 2.3: Implement `LookupPlayerBySlot`**

Insert into `modules/world/server.go` after `LookupPlayerByUID` (line 715-end):

```go
// LookupPlayerBySlot returns the logged-in player at the given slot
// index, or nil if slot is out of range or unoccupied. Mirrors TS
// World.getPlayer(slot). Used by OpPlayer handlers to resolve a
// message's PlayerSlot to a target Player.
func (s *Server) LookupPlayerBySlot(slot int) *Player {
	if slot < 0 || slot >= len(s.players) {
		return nil
	}
	return s.players[slot]
}
```

- [ ] **Step 2.4: Add the two sentinels**

In `modules/world/interaction.go`, extend the const block at lines 29-34:

```go
const (
	targetOpLocT    = 6 // APLOCT / OPLOCT dispatch marker
	targetOpLocU    = 7 // APLOCU / OPLOCU dispatch marker
	targetOpNpcT    = 8 // APNPCT / OPNPCT dispatch marker (S6o)
	targetOpNpcU    = 9 // APNPCU / OPNPCU dispatch marker (S6o)
	targetOpPlayerT = 10 // APPLAYERT / OPPLAYERT dispatch marker (NAI-40)
	targetOpPlayerU = 11 // APPLAYERU / OPPLAYERU dispatch marker (NAI-40)
)
```

- [ ] **Step 2.5: Run tests, confirm GREEN + vet**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run "TestLookupPlayerBySlot"
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...
```

- [ ] **Step 2.6: Commit**

```
feat(world): NAI-40 T2 — LookupPlayerBySlot + targetOpPlayer{T,U} sentinels
```

---

## Task 3: `handleOpPlayer` (ops 1..4)

**Goal:** Implement the parametric op-1..4 handler. Mirrors TS `OpPlayerHandler.ts` (45 lines).

**Files:**
- New: `modules/world/handler_op_player.go`
- New: `modules/world/handler_op_player_test.go`

**TS reference:** `src/network/game/client/handler/OpPlayerHandler.ts`.

**Pre-flight:** read `modules/world/handler_opnpc.go` to mirror handler shape (gate sequence, error returns, UnsetMapFlag emission). Read `pkg/rsbuf/buf*.go` to confirm `HasPlayer(localSlot, otherSlot)` signature.

- [ ] **Step 3.1: Write 5 failing handler tests**

Create `modules/world/handler_op_player_test.go`:

```go
package world

import (
	"testing"

	clientmsg "github.com/zsrv/goscape/pkg/io/protocol/game/client"
)

// TestHandleOpPlayer_HappyPath_AllOps — for each of op 1..4, the handler
// sets target = other, targetOp = op, targetSubject.com = -1, kind =
// InteractionEngine.
func TestHandleOpPlayer_HappyPath_AllOps(t *testing.T) {
	for op := 1; op <= 4; op++ {
		t.Run(fmt.Sprintf("op=%d", op), func(t *testing.T) {
			s := newTestServer(t)
			p, other := twoTestPlayers(t, s) // helper to allocate two players in s.players

			// Make `other` visible to `p` via rsbuf so HasPlayer returns true.
			s.rsbuf.PlayerInsert(p.slot, other.slot)

			err := handleOpPlayer(p, clientmsg.OpPlayer{PlayerSlot: other.slot, Op: op})
			if err != nil {
				t.Fatalf("handleOpPlayer: %v", err)
			}

			if p.target != other {
				t.Errorf("target: got %v, want other (%p)", p.target, other)
			}
			if p.targetOp != op {
				t.Errorf("targetOp: got %d, want %d", p.targetOp, op)
			}
			if p.targetSubject.com != -1 {
				t.Errorf("targetSubject.com: got %d, want -1", p.targetSubject.com)
			}
			if p.interactionKind != InteractionEngine {
				t.Errorf("interactionKind: got %v, want InteractionEngine", p.interactionKind)
			}
		})
	}
}

// TestHandleOpPlayer_DelayedSendsUnsetMapFlag — when the player is
// delayed, handler skips interaction setup and writes UnsetMapFlag.
func TestHandleOpPlayer_DelayedSendsUnsetMapFlag(t *testing.T) {
	s := newTestServer(t)
	p, other := twoTestPlayers(t, s)
	s.rsbuf.PlayerInsert(p.slot, other.slot)
	p.delayed = true
	p.delayedUntil = s.currentTick + 5

	prevTarget := p.target
	err := handleOpPlayer(p, clientmsg.OpPlayer{PlayerSlot: other.slot, Op: 1})
	if err != nil {
		t.Fatalf("handleOpPlayer: %v", err)
	}
	if p.target != prevTarget {
		t.Error("target should not change while delayed")
	}
	assertUnsetMapFlagWritten(t, p)
}

// TestHandleOpPlayer_TargetNotLoggedIn — LookupPlayerBySlot returns nil
// (slot empty) → UnsetMapFlag, no interaction set.
func TestHandleOpPlayer_TargetNotLoggedIn(t *testing.T) {
	s := newTestServer(t)
	p := newTestPlayer(t, s)
	missingSlot := 99
	s.players[missingSlot] = nil

	err := handleOpPlayer(p, clientmsg.OpPlayer{PlayerSlot: missingSlot, Op: 1})
	if err != nil {
		t.Fatalf("handleOpPlayer: %v", err)
	}
	if p.target != nil {
		t.Errorf("target should remain nil; got %v", p.target)
	}
	assertUnsetMapFlagWritten(t, p)
}

// TestHandleOpPlayer_NotVisibleViaRsbuf — target exists but not visible
// to local player per rsbuf.HasPlayer → UnsetMapFlag, no interaction set.
func TestHandleOpPlayer_NotVisibleViaRsbuf(t *testing.T) {
	s := newTestServer(t)
	p, other := twoTestPlayers(t, s)
	// Deliberately do NOT call s.rsbuf.PlayerInsert.

	err := handleOpPlayer(p, clientmsg.OpPlayer{PlayerSlot: other.slot, Op: 1})
	if err != nil {
		t.Fatalf("handleOpPlayer: %v", err)
	}
	if p.target != nil {
		t.Errorf("target should remain nil; got %v", p.target)
	}
	assertUnsetMapFlagWritten(t, p)
}

// TestHandleOpPlayer_ClearsPendingActionAndPriorInteraction — sanity:
// handler calls ClearPendingAction; prior loc/npc target is replaced.
func TestHandleOpPlayer_ClearsPendingActionAndPriorInteraction(t *testing.T) {
	s := newTestServer(t)
	p, other := twoTestPlayers(t, s)
	s.rsbuf.PlayerInsert(p.slot, other.slot)

	// Seed a stale prior interaction.
	staleNpc := newTestNpc(t, s)
	p.SetInteraction(InteractionEngine, staleNpc, 1, -1)

	err := handleOpPlayer(p, clientmsg.OpPlayer{PlayerSlot: other.slot, Op: 2})
	if err != nil {
		t.Fatalf("handleOpPlayer: %v", err)
	}
	if p.target != other {
		t.Errorf("target: got %v, want other (op-click should overwrite stale npc target)", p.target)
	}
	if p.targetOp != 2 {
		t.Errorf("targetOp: got %d, want 2", p.targetOp)
	}
}
```

**Note for implementer:** `twoTestPlayers`, `assertUnsetMapFlagWritten`, `s.rsbuf.PlayerInsert` may not exist with those exact names. Use the existing test helpers from `handler_opnpc_test.go` and `rsbuf_lifecycle_test.go` as templates. Add new helpers if needed (commit them in a separate test-helper commit if they're shared, or inline them in this test file otherwise).

- [ ] **Step 3.2: Run tests, confirm RED**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run "TestHandleOpPlayer"
```

- [ ] **Step 3.3: Implement `handleOpPlayer`**

Create `modules/world/handler_op_player.go`:

```go
package world

import (
	clientmsg "github.com/zsrv/goscape/pkg/io/protocol/game/client"
)

// handleOpPlayer dispatches an OPPLAYER1..4 client packet. Mirrors TS
// OpPlayerHandler.ts: validate not-delayed, look up target by slot,
// validate visibility via rsbuf.HasPlayer, then anchor the engine
// interaction with op = msg.Op (1..4) and com = -1.
//
// The trigger arithmetic (TriggerApPlayer<N>, +7 → TriggerOpPlayer<N>)
// happens later in the trigger-fire path (see player_interaction_trigger.go).
//
// DEVIATION NAI-40-D-OPCALLED-MISSING: TS sets player.opcalled = true
// at handler exit; goscape uses interactionFired (set by trigger fire)
// instead. Pre-existing S6a-era convention. Closure: NAI-40-SB1
// (cross-cutting opcalled-flag convergence).
//
// DEVIATION NAI-40-D-OPPLAYER3-FOLLOWOP-NOT-PORTED: TS Player.ts:1115
// special-cases targetOp == APPLAYER3 || OPPLAYER3 to keep the
// interaction anchored while chasing the target. Goscape fires-and-
// forgets. Tag-only; closure when player-script-lifecycle alignment
// sub-spec ports follow-op semantics.
func handleOpPlayer(p *Player, msg clientmsg.OpPlayer) error {
	srv := p.client.server

	if p.delayed && srv.currentTick < p.delayedUntil {
		sendUnsetMapFlag(p)
		return nil
	}

	other := srv.LookupPlayerBySlot(msg.PlayerSlot)
	if other == nil {
		sendUnsetMapFlag(p)
		return nil
	}

	if !srv.rsbuf.HasPlayer(p.slot, other.slot) {
		sendUnsetMapFlag(p)
		return nil
	}

	p.ClearPendingAction()
	p.SetInteraction(InteractionEngine, other, msg.Op, -1)
	return nil
}
```

**Implementer notes:**
- Confirm exact `srv.rsbuf` field path; may be `srv.rsbufBuilder` / `srv.rsBuf`. Grep at HEAD before substituting.
- `sendUnsetMapFlag` is defined at `modules/world/interaction.go:37`; reuse.
- The handler returns `error` for parity with other handlers if the project convention is `error`-returning; if existing handlers return bool or void, match that.

- [ ] **Step 3.4: Wire dispatch (if needed)**

Check whether goscape has a central client-packet dispatch table that needs the OpPlayer handlers registered (e.g., `dispatch.go` mapping opcode → handler). If so, register OPPLAYER1..4 (164/53/185/206) → `handleOpPlayer` (with the dispatcher passing the op slot 1..4 derived from the opcode).

If the project convention is per-opcode dispatch wiring, this is part of T3. If it's deferred to a single dispatch-table commit, leave a clear `// TODO(NAI-40 dispatch)` and resolve in T7.

- [ ] **Step 3.5: Run tests, confirm GREEN + vet + full suite**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run "TestHandleOpPlayer"
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...
```

- [ ] **Step 3.6: Commit**

```
feat(world): NAI-40 T3 — handleOpPlayer (ops 1..4)
```

---

## Task 4: `handleOpPlayerT` (use spell on player)

**Goal:** Add the T variant: spell-component validation → SetInteraction with com = spellCom and op = targetOpPlayerT.

**Files:** modify `modules/world/handler_op_player.go` and `modules/world/handler_op_player_test.go`.

**TS reference:** `src/network/game/client/handler/OpPlayerTHandler.ts` (49 lines).

**Pre-flight:** read `modules/world/handler_opnpc.go` for the existing OpNpcT handler implementation (the goscape pattern for component-validation + spellCom snapshot). The OpNpcT handler tests at `handler_opnpc_test.go:229+` are the template for OpPlayerT tests.

- [ ] **Step 4.1: Write 4 failing tests**

Append to `modules/world/handler_op_player_test.go`:

```go
// TestHandleOpPlayerT_HappyPath — spellCom valid + visible + actionTarget
// has PLAYER → handler sets target = other, targetOp = targetOpPlayerT,
// targetSubject.com = spellCom, kind = InteractionEngine.
func TestHandleOpPlayerT_HappyPath(t *testing.T) {
	s := newTestServer(t)
	p, other := twoTestPlayers(t, s)
	s.rsbuf.PlayerInsert(p.slot, other.slot)

	const spellComID = 7777
	registerTestComponent(t, s, spellComID, ComActionTargetPlayer, true /* visible */, false /* usable irrelevant */)

	err := handleOpPlayerT(p, clientmsg.OpPlayerT{PlayerSlot: other.slot, SpellCom: spellComID})
	if err != nil {
		t.Fatalf("handleOpPlayerT: %v", err)
	}
	if p.target != other {
		t.Errorf("target: got %v, want other", p.target)
	}
	if p.targetOp != targetOpPlayerT {
		t.Errorf("targetOp: got %d, want targetOpPlayerT (%d)", p.targetOp, targetOpPlayerT)
	}
	if p.targetSubject.com != spellComID {
		t.Errorf("targetSubject.com: got %d, want %d (spellCom)", p.targetSubject.com, spellComID)
	}
}

func TestHandleOpPlayerT_ComponentNotForPlayer(t *testing.T) {
	s := newTestServer(t)
	p, other := twoTestPlayers(t, s)
	s.rsbuf.PlayerInsert(p.slot, other.slot)
	const spellComID = 7777
	registerTestComponent(t, s, spellComID, 0 /* actionTarget bits without PLAYER */, true, false)

	err := handleOpPlayerT(p, clientmsg.OpPlayerT{PlayerSlot: other.slot, SpellCom: spellComID})
	if err != nil {
		t.Fatalf("handleOpPlayerT: %v", err)
	}
	if p.target != nil {
		t.Errorf("target should remain nil; got %v", p.target)
	}
	assertUnsetMapFlagWritten(t, p)
}

func TestHandleOpPlayerT_ComponentNotVisible(t *testing.T) {
	s := newTestServer(t)
	p, other := twoTestPlayers(t, s)
	s.rsbuf.PlayerInsert(p.slot, other.slot)
	const spellComID = 7777
	registerTestComponent(t, s, spellComID, ComActionTargetPlayer, false /* not visible */, false)

	err := handleOpPlayerT(p, clientmsg.OpPlayerT{PlayerSlot: other.slot, SpellCom: spellComID})
	if err != nil {
		t.Fatalf("handleOpPlayerT: %v", err)
	}
	if p.target != nil {
		t.Errorf("target should remain nil; got %v", p.target)
	}
	assertUnsetMapFlagWritten(t, p)
}

func TestHandleOpPlayerT_TargetNotVisible(t *testing.T) {
	s := newTestServer(t)
	p, other := twoTestPlayers(t, s)
	// rsbuf.PlayerInsert NOT called → HasPlayer returns false
	const spellComID = 7777
	registerTestComponent(t, s, spellComID, ComActionTargetPlayer, true, false)

	err := handleOpPlayerT(p, clientmsg.OpPlayerT{PlayerSlot: other.slot, SpellCom: spellComID})
	if err != nil {
		t.Fatalf("handleOpPlayerT: %v", err)
	}
	if p.target != nil {
		t.Errorf("target should remain nil; got %v", p.target)
	}
	assertUnsetMapFlagWritten(t, p)
}
```

**Note for implementer:** `registerTestComponent` and `ComActionTargetPlayer` may need to be located or invented based on what `handler_opnpc_test.go` already uses. If the project has `pkg/objtype/component.go` or similar with the actionTarget bits enum, use it directly.

- [ ] **Step 4.2: Run tests, confirm RED**

- [ ] **Step 4.3: Implement `handleOpPlayerT`**

Append to `modules/world/handler_op_player.go`:

```go
// handleOpPlayerT dispatches an OPPLAYERT (use-spell-on-player) packet.
// Mirrors TS OpPlayerTHandler.ts: validate not-delayed, validate the
// spell component (must exist, actionTarget include PLAYER, be visible),
// look up target by slot, validate visibility, anchor the engine
// interaction with op = targetOpPlayerT and com = spellCom.
//
// DEVIATION NAI-40-D-OPCALLED-MISSING: see handleOpPlayer.
func handleOpPlayerT(p *Player, msg clientmsg.OpPlayerT) error {
	srv := p.client.server

	if p.delayed && srv.currentTick < p.delayedUntil {
		sendUnsetMapFlag(p)
		return nil
	}

	spellCom := srv.componentLookup(msg.SpellCom) // exact API TBV at impl time
	if spellCom == nil || (spellCom.ActionTarget & ComActionTargetPlayer) == 0 {
		sendUnsetMapFlag(p)
		return nil
	}
	if !p.IsComponentVisible(spellCom) {
		sendUnsetMapFlag(p)
		return nil
	}

	other := srv.LookupPlayerBySlot(msg.PlayerSlot)
	if other == nil {
		sendUnsetMapFlag(p)
		return nil
	}
	if !srv.rsbuf.HasPlayer(p.slot, other.slot) {
		sendUnsetMapFlag(p)
		return nil
	}

	p.ClearPendingAction()
	p.SetInteraction(InteractionEngine, other, targetOpPlayerT, msg.SpellCom)
	return nil
}
```

**Implementer note:** `srv.componentLookup(id)`, `spellCom.ActionTarget`, `ComActionTargetPlayer`, `p.IsComponentVisible(...)` are placeholder names — match whatever the existing OpNpcT handler uses. If `handler_opnpc.go` has a similar component-validation helper extracted, reuse it (DRY) rather than duplicating the validation chain.

- [ ] **Step 4.4: Run tests + vet + full suite**

- [ ] **Step 4.5: Commit**

```
feat(world): NAI-40 T4 — handleOpPlayerT (use spell on player)
```

---

## Task 5: `handleOpPlayerU` (use item on player)

**Goal:** Add the U variant: full validation chain (component usable + visible, invListener exists, slot valid, item match, members check) → snapshot lastUseItem/lastUseSlot → SetInteraction with com = -1 and op = targetOpPlayerU.

**Files:** modify `modules/world/handler_op_player.go` and `modules/world/handler_op_player_test.go`.

**TS reference:** `src/network/game/client/handler/OpPlayerUHandler.ts` (75 lines).

**Pre-flight:** read `modules/world/handler_opnpc.go`'s OpNpcU handler. Specifically read `handler_opnpc_test.go:327-360` for the lastUseItem/lastUseSlot snapshot pattern and com=-1 quirk.

- [ ] **Step 5.1: Write 6 failing tests**

Append to `modules/world/handler_op_player_test.go`:

```go
// TestHandleOpPlayerU_HappyPath — full validation chain passes; handler
// snapshots lastUseItem/lastUseSlot and anchors interaction with
// op = targetOpPlayerU and targetSubject.com = -1 (TS quirk: useCom is
// NOT snapshotted).
func TestHandleOpPlayerU_HappyPath(t *testing.T) {
	s := newTestServer(t)
	p, other := twoTestPlayers(t, s)
	s.rsbuf.PlayerInsert(p.slot, other.slot)
	const useComID = 9999
	const useObj = 1511
	const useSlot = 3
	registerTestComponent(t, s, useComID, 0, true /* visible */, true /* usable */)
	addInvListener(t, p, useComID, useObj, useSlot)

	err := handleOpPlayerU(p, clientmsg.OpPlayerU{
		PlayerSlot: other.slot, UseObj: useObj, UseSlot: useSlot, UseCom: useComID,
	})
	if err != nil {
		t.Fatalf("handleOpPlayerU: %v", err)
	}
	if p.target != other {
		t.Errorf("target: got %v, want other", p.target)
	}
	if p.targetOp != targetOpPlayerU {
		t.Errorf("targetOp: got %d, want targetOpPlayerU (%d)", p.targetOp, targetOpPlayerU)
	}
	if p.targetSubject.com != -1 {
		t.Errorf("targetSubject.com: got %d, want -1 (TS quirk)", p.targetSubject.com)
	}
	if p.lastUseItem != useObj {
		t.Errorf("lastUseItem: got %d, want %d", p.lastUseItem, useObj)
	}
	if p.lastUseSlot != useSlot {
		t.Errorf("lastUseSlot: got %d, want %d", p.lastUseSlot, useSlot)
	}
}

func TestHandleOpPlayerU_ComponentNotUsable(t *testing.T) {
	// useCom registered with usable=false → UnsetMapFlag, no interaction
	// (full body modeled on OpNpcU equivalent test)
}

func TestHandleOpPlayerU_InvListenerMissing(t *testing.T) { /* ... */ }
func TestHandleOpPlayerU_InvalidSlot(t *testing.T)        { /* ... */ }
func TestHandleOpPlayerU_ItemMismatch(t *testing.T)       { /* ... */ }
func TestHandleOpPlayerU_MembersOnNonMembersServer(t *testing.T) { /* ... */ }
```

**Implementer note:** flesh out the 5 elided test bodies by mirroring the equivalent OpNpcU tests in `handler_opnpc_test.go`. Reuse helpers (`addInvListener`, `registerTestComponent`, `setMembersServer(t, s, false)`).

- [ ] **Step 5.2: Run tests, confirm RED**

- [ ] **Step 5.3: Implement `handleOpPlayerU`**

Append to `modules/world/handler_op_player.go`:

```go
// handleOpPlayerU dispatches an OPPLAYERU (use-item-on-player) packet.
// Mirrors TS OpPlayerUHandler.ts: validate not-delayed, validate the
// use component (usable + visible), look up the inventory listener and
// validate slot + item match, members-server check, then snapshot
// lastUseItem/lastUseSlot and anchor the engine interaction with
// op = targetOpPlayerU and com = -1 (TS quirk: useCom not snapshotted).
//
// DEVIATION NAI-40-D-OPCALLED-MISSING: see handleOpPlayer.
func handleOpPlayerU(p *Player, msg clientmsg.OpPlayerU) error {
	srv := p.client.server

	if p.delayed && srv.currentTick < p.delayedUntil {
		sendUnsetMapFlag(p)
		return nil
	}

	useCom := srv.componentLookup(msg.UseCom)
	if useCom == nil || !useCom.Usable {
		sendUnsetMapFlag(p)
		return nil
	}
	if !p.IsComponentVisible(useCom) {
		sendUnsetMapFlag(p)
		return nil
	}

	listener := p.findInvListener(msg.UseCom)
	if listener == nil {
		sendUnsetMapFlag(p)
		return nil
	}
	inv := p.getInventoryFromListener(listener)
	if inv == nil {
		sendUnsetMapFlag(p)
		return nil
	}
	if !inv.ValidSlot(msg.UseSlot) {
		sendUnsetMapFlag(p)
		return nil
	}
	if !inv.HasAt(msg.UseSlot, msg.UseObj) {
		sendUnsetMapFlag(p)
		return nil
	}

	other := srv.LookupPlayerBySlot(msg.PlayerSlot)
	if other == nil {
		sendUnsetMapFlag(p)
		return nil
	}
	if !srv.rsbuf.HasPlayer(p.slot, other.slot) {
		sendUnsetMapFlag(p)
		return nil
	}

	p.ClearPendingAction()

	if objType := srv.objTypeLookup(msg.UseObj); objType != nil && objType.Members && !srv.cfg.NodeMembers {
		p.MessageGame("To use this item please login to a members' server.")
		sendUnsetMapFlag(p)
		return nil
	}

	p.lastUseItem = msg.UseObj
	p.lastUseSlot = msg.UseSlot
	p.SetInteraction(InteractionEngine, other, targetOpPlayerU, -1)
	return nil
}
```

**Implementer notes:**
- All `srv.componentLookup`, `p.findInvListener`, `p.getInventoryFromListener`, `inv.ValidSlot`, `inv.HasAt`, `srv.objTypeLookup`, `srv.cfg.NodeMembers` are placeholder names — match whatever the existing OpNpcU handler uses.
- The members-check ordering in TS is **after** target+rsbuf validation. Mirror that ordering exactly.
- The snapshot of lastUseItem/lastUseSlot happens **after** ClearPendingAction and **before** SetInteraction.

- [ ] **Step 5.4: Run tests + vet + full suite**

- [ ] **Step 5.5: Commit**

```
feat(world): NAI-40 T5 — handleOpPlayerU (use item on player)
```

---

## Task 6: `player_interaction_trigger.go` — trigger maps + fire functions + tryFire dispatch

**Goal:** Land the trigger plumbing that closes the loop. Add the player-actor trigger map (`apPlayerTriggerForOp`), the AP/OP fire functions (`fireOpTriggerPlayer`, `fireApTriggerPlayer`), and extend the existing `tryFireOpTrigger` / `tryFireApTrigger` type-switches with a `case *Player:` arm. Self2 is bound by routing the fire functions through `srv.runScript(sf, target, p, ...)` so NAI-39's `case script.ActivePlayer:` arm in `buildPlayerScriptState` (script.go:54-57) fires.

**Files:**
- New: `modules/world/player_interaction_trigger.go`
- New: `modules/world/player_interaction_trigger_test.go`
- Modify: `modules/world/interaction_trigger.go` (extend `tryFireOpTrigger` / `tryFireApTrigger` switches)

**TS reference:** `src/engine/entity/Player.ts` `tryInteract()` (lines 1115-1199), `src/engine/script/ScriptRunner.ts:78-92`.

**Pre-flight:**
- `rg -n "tryFireOpTrigger\|tryFireApTrigger" modules/world/` and enumerate every call site that observes `p.target.(*Player)` flow — confirm we cover both AP and OP entry points.
- `rg -n "scriptProvider\.GetByTrigger" modules/world/` to verify the registry call sites' arg shapes.
- `rg -n "TriggerApPlayer\|TriggerOpPlayer" pkg/script/trigger.go` to confirm constants exist for ops 1..4 + T + U (they do per spec pre-flight, but re-verify).
- TBV at impl time: registry behavior for `GetByTrigger(trigger, -1, -1)` when target has no type. If registry rejects -1, this becomes a blocker — escalate.

- [ ] **Step 6.1: Write 6 failing tests**

Create `modules/world/player_interaction_trigger_test.go`:

```go
package world

import (
	"testing"

	"github.com/zsrv/goscape/pkg/script"
)

// TestApPlayerTriggerForOp pins the op→trigger map.
func TestApPlayerTriggerForOp(t *testing.T) {
	cases := []struct {
		op      int
		want    script.ServerTriggerType
		wantOk  bool
	}{
		{1, script.TriggerApPlayer1, true},
		{2, script.TriggerApPlayer2, true},
		{3, script.TriggerApPlayer3, true},
		{4, script.TriggerApPlayer4, true},
		{targetOpPlayerT, script.TriggerApPlayerT, true},
		{targetOpPlayerU, script.TriggerApPlayerU, true},
		{0, 0, false},
		{5, 0, false}, // no OPPLAYER5 wire packet
		{-1, 0, false},
	}
	for _, c := range cases {
		got, ok := apPlayerTriggerForOp(c.op)
		if ok != c.wantOk {
			t.Errorf("apPlayerTriggerForOp(%d): ok=%v, want %v", c.op, ok, c.wantOk)
		}
		if got != c.want {
			t.Errorf("apPlayerTriggerForOp(%d): trigger=%d, want %d", c.op, got, c.want)
		}
	}
}

// TestFireOpTriggerPlayer_BindsSelf2ToClicker — register an
// [opplayer1,_] script that does `~hint_pl(active_player2)` and assert
// that after fire, target's outbound mask carries a hint-arrow pointing
// at the clicker (Self2 = clicker via NAI-39 substrate).
func TestFireOpTriggerPlayer_BindsSelf2ToClicker(t *testing.T) {
	s := newTestServer(t)
	clicker, target := twoTestPlayers(t, s)
	s.rsbuf.PlayerInsert(clicker.slot, target.slot)

	// Register a [opplayer1,_] script that calls hint_pl(active_player2).
	registerTriggerScript(t, s, script.TriggerOpPlayer1, scriptHintPlActivePlayer2(t))

	// Place clicker adjacent to target.
	clicker.MoveTo(target.x+1, target.z, target.level)

	// Anchor interaction at op 1 (engine kind, contact distance).
	clicker.SetInteraction(InteractionEngine, target, 1, -1)
	clicker.processInteraction()

	// Assert hint-arrow mask on target points at clicker.
	assertHintPlayerOnMask(t, target, clicker.slot)
}

func TestFireOpTriggerPlayer_NoScriptRegistered(t *testing.T) {
	s := newTestServer(t)
	clicker, target := twoTestPlayers(t, s)
	s.rsbuf.PlayerInsert(clicker.slot, target.slot)
	// Don't register any [opplayer1,_].
	clicker.MoveTo(target.x+1, target.z, target.level)
	clicker.SetInteraction(InteractionEngine, target, 1, -1)
	clicker.processInteraction()

	// No script → silent clear; no panic, target's mask unchanged.
	if clicker.target != nil {
		t.Errorf("target should be cleared; got %v", clicker.target)
	}
}

func TestFireApTriggerPlayer_NoScriptSetsApRangeMinusOne(t *testing.T) {
	s := newTestServer(t)
	clicker, target := twoTestPlayers(t, s)
	s.rsbuf.PlayerInsert(clicker.slot, target.slot)
	// Don't register any [applayer1,_].
	clicker.MoveTo(target.x+5, target.z, target.level) // AP range, not contact
	clicker.apRange = 10
	clicker.SetInteraction(InteractionEngine, target, 1, -1)
	clicker.processInteraction()

	if clicker.apRange != -1 {
		t.Errorf("apRange: got %d, want -1 (no-script-found marker)", clicker.apRange)
	}
}

func TestTryFireOpTrigger_PlayerArm(t *testing.T) {
	// Pin the type-switch dispatch: when p.target is *Player,
	// tryFireOpTrigger calls fireOpTriggerPlayer (not the default skip).
	// Indirect: register [opplayer1,_] that mutates a side-effect, run
	// tryFireOpTrigger directly, assert side-effect.
	// (full body — model on TestTryFireOpTrigger_NpcArm if it exists)
}

func TestTryFireApTrigger_PlayerArm(t *testing.T) {
	// Symmetric to OP arm.
}
```

**Implementer note:** `scriptHintPlActivePlayer2` is a fixture helper that compiles a tiny script with bytecode `OpHintPl` after pushing `active_player2`'s slot. If NAI-39's tests have a similar fixture, reuse it; otherwise build per `scriptstate_test_fixture_idioms.md` (correct push order + Pointers flag).

- [ ] **Step 6.2: Run tests, confirm RED**

- [ ] **Step 6.3: Implement `apPlayerTriggerForOp` + fire functions**

Create `modules/world/player_interaction_trigger.go`:

```go
package world

import "github.com/zsrv/goscape/pkg/script"

// apPlayerTriggerForOp returns the APPLAYER trigger for the player's
// targetOp. Returns ok=false if op is neither 1..4 nor a T/U sentinel.
// fireOpTriggerPlayer derives the OPPLAYER trigger by adding 7 to the
// returned APPLAYER (TS Player.ts:~997 offset convention):
//
//	APPLAYER1..4 (87..90) + 7 → OPPLAYER1..4 (94..97)
//	APPLAYERT    (93)     + 7 → OPPLAYERT    (100)
//	APPLAYERU    (92)     + 7 → OPPLAYERU    (99)
//
// Note: ops are 1..4 here, NOT 1..5 — real client only sends OPPLAYER1..4.
// The 5-slot TriggerOpPlayer<5> referenced in spec materials is an
// NPC-AI side concept (TriggerAiOpPlayer1..5).
func apPlayerTriggerForOp(op int) (script.ServerTriggerType, bool) {
	switch {
	case op >= 1 && op <= 4:
		return script.TriggerApPlayer1 + script.ServerTriggerType(op-1), true
	case op == targetOpPlayerT:
		return script.TriggerApPlayerT, true
	case op == targetOpPlayerU:
		return script.TriggerApPlayerU, true
	default:
		return 0, false
	}
}

// fireOpTriggerPlayer fires the [opplayer<op>,_] trigger for a Player
// target. Self = target, Self2 = clicker (the receiver `p`). Self2
// binding flows through srv.runScript → buildPlayerScriptState's
// `case script.ActivePlayer:` arm (script.go:54-57).
//
// Closes NAI-39-D-ACTIVEPLAYER2-NO-OPPLAYER-PRODUCER: this is the
// production producer for state.Self2.
func fireOpTriggerPlayer(p *Player, srv *Server, target *Player) {
	if p.delayed && srv.currentTick < p.delayedUntil {
		return
	}

	apTrigger, ok := apPlayerTriggerForOp(p.targetOp)
	if !ok {
		p.ClearInteraction()
		p.interactionFired = true
		return
	}
	trigger := apTrigger + 7 // APPLAYER → OPPLAYER offset

	// Players have no type; pass -1 sentinels for typeId + category.
	// TBV: registry behaviour for -1 sentinels — confirmed at T6 impl
	// time against scriptProvider.GetByTrigger semantics.
	sf := srv.scriptProvider.GetByTrigger(trigger, -1, -1)
	if sf == nil {
		p.ClearInteraction()
		p.interactionFired = true
		return
	}

	// Run with target as Self and `p` (clicker) threaded as the
	// ActivePlayer-typed second arg → buildPlayerScriptState's
	// case-ActivePlayer arm sets state.Self2 = p.
	srv.runScript(sf, target, p, true, nil, nil)
	p.interactionFired = true
}

// fireApTriggerPlayer fires the [applayer<op>,_] trigger.
// On no-script-found: sets p.apRange = -1 to skip re-lookup next tick
// (matches existing Loc behavior).
func fireApTriggerPlayer(p *Player, srv *Server, target *Player) {
	if p.delayed && srv.currentTick < p.delayedUntil {
		return
	}

	trigger, ok := apPlayerTriggerForOp(p.targetOp)
	if !ok {
		p.ClearInteraction()
		p.interactionFired = true
		return
	}

	sf := srv.scriptProvider.GetByTrigger(trigger, -1, -1)
	if sf == nil {
		p.apRange = -1
		return
	}

	srv.runScript(sf, target, p, true, nil, nil)
	p.interactionFired = true
}
```

- [ ] **Step 6.4: Extend `tryFireOpTrigger` / `tryFireApTrigger`**

Modify `modules/world/interaction_trigger.go`. At the top type-switch in `tryFireOpTrigger` (around line 35-44):

```go
func tryFireOpTrigger(p *Player) {
	srv := p.client.server

	switch tgt := p.target.(type) {
	case *Npc:
		fireOpTriggerNpc(p, srv, tgt)
	case *entitypkg.Loc:
		fireOpTriggerLoc(p, srv, tgt)
	case *Player:
		fireOpTriggerPlayer(p, srv, tgt)
	default:
		p.interactionFired = true
	}
}
```

Same extension for `tryFireApTrigger` (around line 250-263):

```go
func tryFireApTrigger(p *Player) {
	srv := p.client.server

	switch tgt := p.target.(type) {
	case *entitypkg.Loc:
		fireApTriggerLoc(p, srv, tgt)
	case *Npc:
		fireApTriggerNpc(p, srv, tgt)
	case *Player:
		fireApTriggerPlayer(p, srv, tgt)
	default:
		p.interactionFired = true
	}
}
```

- [ ] **Step 6.5: Run tests + vet + full suite**

- [ ] **Step 6.6: Commit**

```
feat(world): NAI-40 T6 — player_interaction_trigger.go + tryFire dispatch arms
```

---

## Task 7: E2E smoke + close commit (deviation comment retirement)

**Goal:** Land the end-to-end deviation-closure pin and remove the now-stale deviation comments at `pkg/script/state.go:194-196` and `modules/world/script.go:30-33`.

**Files:**
- Modify: `modules/world/script_test.go` (append E2E smoke)
- Modify: `pkg/script/state.go` (retire deviation comment lines 194-196)
- Modify: `modules/world/script.go` (retire deviation comment lines 30-33)

- [ ] **Step 7.1: Write E2E smoke**

Append to `modules/world/script_test.go`:

```go
// TestOpPlayer1_E2E_HintPlOnTarget — full path: simulate an
// OPPLAYER1 client packet → handler → tick processInteraction →
// trigger fires → target's [opplayer1,_] script runs
// `~hint_pl(active_player2)` → assert hint-arrow mask on target
// points at clicker. Closes NAI-39-D-ACTIVEPLAYER2-NO-OPPLAYER-PRODUCER.
func TestOpPlayer1_E2E_HintPlOnTarget(t *testing.T) {
	s := newTestServer(t)
	clicker, target := twoTestPlayers(t, s)
	s.rsbuf.PlayerInsert(clicker.slot, target.slot)

	// Register [opplayer1,_] script that calls hint_pl(active_player2).
	registerTriggerScript(t, s, script.TriggerOpPlayer1,
		buildScript(t, []script.Opcode{
			script.OpPushIntLocal, // index of active_player2.slot? OR direct: PtrActivePlayer2 + Slot()
			// Implementer: build the bytecode that pushes active_player2 onto the int stack
			// then invokes OpHintPl. Use the canonical hint-arrow opcode constants.
		}))

	// Place clicker adjacent to target so the OP branch fires.
	clicker.MoveTo(target.x+1, target.z, target.level)

	// Simulate the wire packet via the handler entry point.
	if err := handleOpPlayer(clicker, clientmsg.OpPlayer{
		PlayerSlot: target.slot, Op: 1,
	}); err != nil {
		t.Fatalf("handleOpPlayer: %v", err)
	}

	// Drive one tick.
	clicker.processInteraction()

	// Assert: target's outbound mask has a hint-arrow pointing at clicker.
	assertHintPlayerOnMask(t, target, clicker.slot)
}
```

**Implementer note:** the bytecode-build helper details depend on existing test fixtures (`runner_test.go` / `script_test.go`'s NAI-39 hint tests are the closest template). If a `[opplayer1,_]` script registration helper doesn't exist, add one.

- [ ] **Step 7.2: Run E2E test, confirm GREEN**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run "TestOpPlayer1_E2E_HintPlOnTarget"
```

- [ ] **Step 7.3: Retire deviation comments**

In `pkg/script/state.go` lines 188-200, change the comment block from:

```go
	// Self2 is the secondary active-player slot consumed by HINT_PL and
	// (future) BOTH_HEROPOINTS / OPPLAYER triggers. Mirrors TS
	// ScriptState._activePlayer2 (ScriptState.ts:80).
	//
	// DEVIATION NAI-39-D-ACTIVEPLAYER2-NO-OPPLAYER-PRODUCER: no production
	// trigger seeds Self2 yet; the rails are exercised only by tests.
	// Closure: a future sub-spec wires production OPPLAYER triggers.
	Self2 ActivePlayer
```

to:

```go
	// Self2 is the secondary active-player slot consumed by HINT_PL and
	// the player→player OPPLAYER<N>/APPLAYER<N> trigger family.
	// Mirrors TS ScriptState._activePlayer2 (ScriptState.ts:80).
	// Production producer: fireOpTriggerPlayer / fireApTriggerPlayer
	// (modules/world/player_interaction_trigger.go) wired by NAI-40.
	Self2 ActivePlayer
```

In `modules/world/script.go` lines 28-35, change:

```go
// DEVIATION NAI-39-D-ACTIVEPLAYER2-NO-OPPLAYER-PRODUCER: the
// case script.ActivePlayer branch lays the rails for OPPLAYER triggers
// (Player→Player invocations). No production trigger seeds Self2 yet;
// the rails are exercised only by tests.
```

to:

```go
// case script.ActivePlayer is the secondary-binding arm consumed by
// the OPPLAYER<N>/APPLAYER<N> player→player trigger family
// (player_interaction_trigger.go). Sets state.Self2 + PtrActivePlayer2
// when target is a *Player (NAI-40 closure of the activePlayer2
// substrate that NAI-39 introduced).
```

- [ ] **Step 7.4: Verify deviation-tag retirement**

```
rg -n "NAI-39-D-ACTIVEPLAYER2-NO-OPPLAYER-PRODUCER" pkg/ modules/
```

Expected: 0 hits in production code. (Test-file references that pin the *behavior* the deviation framed are fine to keep — they document the ActivePlayer2 contract.)

- [ ] **Step 7.5: Run full suite + vet**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...
```

- [ ] **Step 7.6: Final whole-impl review**

Dispatch a code-reviewer agent (subagent_type: `superpowers:code-reviewer`) over commits T1..T7 with the spec at `docs/superpowers/specs/2026-04-27-nai-40-opplayer-producer-design.md` for spec-compliance + code-quality cross-check. Apply review fixes as `polish(...)` commits per `runescript_cadence.md`.

- [ ] **Step 7.7: Close commit**

```
chore(script,world,io): NAI-40 closed — OPPLAYER triggers (player→player op-click)

Closes NAI-39-D-ACTIVEPLAYER2-NO-OPPLAYER-PRODUCER. Self2 substrate
(NAI-39 T1+T3+T4) finally has its production producer:
fireOpTriggerPlayer + fireApTriggerPlayer thread the clicker through
runScript → buildPlayerScriptState's case-ActivePlayer arm.

Closes memory: <list of memory entries this sub-spec produced>

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
```

---

## Cadence reminders (controller)

- **Pre-flight before each task dispatch** per `controller_preflight.md`: 30-second grep+Read pass against HEAD before each implementer dispatch. The pre-flight table at the top of this plan is the starting point — re-verify each premise immediately before dispatch.
- **Two-stage review per task** per `runescript_cadence.md`: spec-compliance opus → code-quality opus, both via `superpowers:code-reviewer`. Apply minor fixes as `polish(...)` commits separate from the task commit.
- **Verify implementer claims** per `verify_implementer_claims.md`: after each task commit, run `git show <SHA> --stat` + `go test ./...` from the controller's clean working tree. Don't trust the implementer's "tests pass" without independent verification.
- **Closes-memory trailer** per `close_commit_memory_trailer.md`: T7 close commit lists every memory entry produced during NAI-40.
- **Memory updates at close** per `post_task_handoff.md`:
  - Add NAI-40 entry to `nai_followups.md` with the deviation ledger above
  - Refresh the "OPPLAYER triggers" item in the followups list (it's now CLOSED, no longer a candidate)
  - Save any non-derivable lessons learned

## TS source citations (for plan-author audit and implementer reference)

- `src/network/game/client/handler/OpPlayerHandler.ts` (45 lines)
- `src/network/game/client/handler/OpPlayerTHandler.ts` (49 lines)
- `src/network/game/client/handler/OpPlayerUHandler.ts` (75 lines)
- `src/engine/script/ScriptRunner.ts:78-92` (target-binding switch)
- `src/engine/entity/Player.ts:1115-1199` (`tryInteract` body — interaction tick)
- `src/engine/script/ScriptState.ts:80,215-243` (activePlayer/activePlayer2 getters/setters)
