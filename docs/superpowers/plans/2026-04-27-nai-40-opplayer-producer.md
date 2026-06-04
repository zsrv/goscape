# NAI-40 — OPPLAYER trigger producer (player→player op-click) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close `NAI-39-D-ACTIVEPLAYER2-NO-OPPLAYER-PRODUCER` by porting the player→player op-click dispatch path. Client `OPPLAYER1..4` / `OPPLAYERT` / `OPPLAYERU` packets resolve to the target player's `[opplayer<N>,_]` / `[applayer<N>,_]` trigger script, running with `Self` = target and `Self2` = clicker via NAI-39's `case script.ActivePlayer:` arm in `buildPlayerScriptState`.

**Architecture:** Bottom-up by layer — server lookup + sentinels (T1) → handlers `handleOpPlayer1..4` / `handleOpPlayerT` / `handleOpPlayerU` (T2, T3, T4) → trigger maps + fire functions + tryFire dispatch (T5) → E2E smoke + deviation-comment retirement (T6). Mirrors NPC-side `handler_opnpc.go` + `npc_interaction_trigger.go` shape exactly. The Self2 binding flows through NAI-39's existing `runScript`/`buildPlayerScriptState` API (NOT the older manual `script.Init` pattern used by `fireOpTriggerNpc` / `fireOpTriggerLoc`) — this is the ActivePlayer-arm's first production producer.

**Plan correction note (2026-04-27):** Initial plan version (commit `5dcb5b2`) framed T1 as "client-message structs + decoders in `pkg/io/protocol/game/client/op_player.go`." Pre-T1 controller pre-flight surfaced that goscape's actual convention is `(p *Player, payload []byte) error` handlers with **inline** `packet.NewPacket(payload)` parsing in `modules/world/handler_op*.go` files, plus dispatch wiring in `modules/world/handlers_game.go`'s `init()` block — there are no decoder structs and no per-opcode files under `pkg/io/protocol/game/client/`. T1 in the original plan had no actual work; tasks renumbered.

**Tech Stack:** Go 1.26+ (per `go_version.md`; use `use-modern-go` skill). TS source: `Engine-TS` only per `ts_source_canonical_path.md`. HEAD baseline: `5dcb5b2` (NAI-40 plan-correction commit).

---

## Spec reference

Spec at `docs/superpowers/specs/2026-04-27-nai-40-opplayer-producer-design.md`. Test layers map to tasks as:
- **L1 (decoder unit tests)** — *resolved during pre-flight: not needed; payload parsing is inline in handlers, exercised through handler tests*
- **L2 (trigger map)** → T5
- **L3 (handlers)** → T2 (OpPlayer 1..4), T3 (OpPlayerT), T4 (OpPlayerU)
- **L4 (processInteraction Player-arm)** — *resolved during pre-flight: no `processInteraction` body change needed; only `tryFire{Op,Ap}Trigger` type-switch extension* → T5
- **L5 (trigger fire + Self2 binding)** → T5
- **L6 (E2E smoke)** → T6

## Pre-flight notes (controller, performed at spec-write + plan-write)

- **`processInteraction` requires no body change.** Existing `interaction.go:74-122` is target-type-agnostic except for the `*Npc` SetFaceEntity branch (line 96-98). Player target falls through to `p.interacted = true; tryFireOpTrigger(p)` — the type-switch in `tryFireOpTrigger` is the single dispatch point. Confirmed by reading `interaction.go` end-to-end.
- **`effectiveApRange` falls through to `p.apRange` for non-Npc targets** (line 184-191). Player target uses `p.apRange` (default 10) — matches Loc behavior. No change.
- **`SetFaceEntity` is Npc-only by design** at `interaction.go:96-98`. TS `tryInteract` does NOT face-entity-on-player either. **Conditional deviation `NAI-40-D-PLAYER-NO-FACEENTITY-ON-OPCLICK` is NOT needed** — both engines skip face-entity for Player target.
- **OPPLAYER3 follow-op semantics are NOT ported.** TS `Player.ts:1115-1209` has `followOp = targetOp === APPLAYER3 || targetOp === OPPLAYER3` chase logic. Goscape will fire-and-forget. Track as **`NAI-40-D-OPPLAYER3-FOLLOWOP-NOT-PORTED`** at handler exit.
- **Existing fire functions use `script.Init` + manual pointer wiring** at `interaction_trigger.go:79-94`, predating NAI-39. **The new `fireOpTriggerPlayer` / `fireApTriggerPlayer` will use `srv.runScript(sf, target, p, true, nil, nil)`** — exercises NAI-39's `case script.ActivePlayer:` arm at `script.go:54-57`. This is the cleanest closure for the deviation.
- **Script-registry lookup signature:** `srv.scriptProvider.GetByTrigger(trigger, typeId, category)` (3 args, line 72/142 of `interaction_trigger.go`). Players are untyped — pass `-1, -1`. **TBV at T5 impl time**: confirm registry returns the trigger's "no-typeId" script for these sentinels. If registry rejects `-1`, escalate.
- **`s.players` is the slot-indexed array** at `modules/world/server.go:618-686`. Slot 0 reserved (loop starts at `i := 1`). `LookupPlayerBySlot` mirrors `handler_opnpc.go:17-20` lookup pattern.
- **Dispatch table is `gameHandlers [256]func(*Player, []byte) error`** in `modules/world/handlers_game.go:15`, wired in the package `init()` (lines 17-50). OPNPC1..5/T/U + OPLOC1..5/T/U entries serve as the wiring template (lines 29-44). **OPPLAYER1..4/T/U entries are NOT yet wired** — T2/T3/T4 will add them.
- **Handler shape per `handler_opnpc.go`:**
  - `handleOpNpc(p *Player, payload []byte, op int) error` shared body for ops 1..5
  - Thin wrappers: `handleOpNpc1(p, payload) → handleOpNpc(p, payload, 1)`, etc. (lines 71-75)
  - T/U handlers are direct one-shot bodies (no per-op fan-out)
  - Inline parsing: `r := packet.NewPacket(payload); slot := int(r.G2())`
  - `len(payload) < 2 → sendUnsetMapFlag(p); return nil` length check on entry
- **`OPPLAYER5` deliberately not in scope.** Real client only sends 1..4; trigger constant `TriggerOpPlayer<5>` does not exist (the trigger.go enum stops at 4 per `trigger.go:97-100`). NPC-AI side has `TriggerAiOpPlayer1..5` for AI mode dispatch — separate concern.
- **`p.ClearPendingAction()` exists** at `modules/world/player_script.go:630`. ✓
- **`pkg/rsbuf` `HasPlayer(localSlot, otherSlot)` exists** per `pkg/rsbuf/buf_test.go:643+`. ✓
- **Field path**: server's rsbuf is at `srv.rsbuf` (TBV at handler impl — could be `s.rsbufBuilder` per existing handlers; grep before substituting). The `s.players[]`-style direct field access is the existing pattern.

## File structure

| File | Status | Responsibility |
|------|--------|----------------|
| `modules/world/server.go` | modify | +`LookupPlayerBySlot` (T1) |
| `modules/world/server_test.go` | modify | +`LookupPlayerBySlot` unit tests (T1) |
| `modules/world/interaction.go` | modify | +`targetOpPlayerT/U` sentinel constants (T1) |
| `modules/world/handler_op_player.go` | new | `handleOpPlayer` + 4 wrappers (T2), `handleOpPlayerT` (T3), `handleOpPlayerU` (T4) |
| `modules/world/handler_op_player_test.go` | new | handler integration tests (T2, T3, T4) |
| `modules/world/handlers_game.go` | modify | +`gameHandlers[164/53/185/206/177/248]` wiring (T2/T3/T4) |
| `modules/world/player_interaction_trigger.go` | new | `apPlayerTriggerForOp` + `fireOpTriggerPlayer` + `fireApTriggerPlayer` (T5) |
| `modules/world/player_interaction_trigger_test.go` | new | trigger-map unit tests + fire-function tests (T5) |
| `modules/world/interaction_trigger.go` | modify | extend `tryFireOpTrigger` and `tryFireApTrigger` type-switches with `case *Player:` arms (T5) |
| `modules/world/script_test.go` | modify | +E2E smoke test (T6) |
| `pkg/script/state.go` | modify | retire `Self2` deviation comment lines 194-196 (T6) |
| `modules/world/script.go` | modify | retire `buildPlayerScriptState` deviation comment lines 30-33 (T6) |

## Pre-flight checks per task (controller)

Per `controller_preflight.md`: re-grep each premise against HEAD before dispatching each task.

| Task | Pre-dispatch verification |
|------|--------------------------|
| T1 | `rg -n "s\.players\b" modules/world/server.go` returns lines 618-686 (slot array). `rg -n "targetOpLocT\|targetOpLocU\|targetOpNpcT\|targetOpNpcU" modules/world/interaction.go` returns line 30-33. |
| T2 | T1 committed. `rg -n "handleOpNpc[12345T U]" modules/world/handler_opnpc.go` shows handler shape. `rg -n "gameHandlers\[1?\d{1,2}\]" modules/world/handlers_game.go` returns the existing wiring block (line 29-44). `rg -n "rsbuf\\..*HasPlayer\|s\.rsbuf\\b" modules/world/` confirms server's rsbuf field name. |
| T3 | T2 committed. `rg -n "handleOpNpcT" modules/world/handler_opnpc.go` shows OpNpcT handler body for component-validation pattern reference. `rg -n "Component\.Get\|isComponentVisible\|componentLookup" modules/world/` shows existing Component-validation API. |
| T4 | T3 committed. `rg -n "handleOpNpcU" modules/world/handler_opnpc.go` shows OpNpcU handler body for full validation chain. `rg -n "lastUseItem\\|lastUseSlot" modules/world/handler_opnpc*.go` shows snapshot pattern. |
| T5 | T4 committed. `rg -n "tryFireOpTrigger\|tryFireApTrigger" modules/world/interaction_trigger.go` shows current type-switch (lines 32-45 / 250-263). `rg -n "scriptProvider\.GetByTrigger" modules/world/` shows registry-lookup call sites. |
| T6 | T5 committed. `rg -n "NAI-39-D-ACTIVEPLAYER2-NO-OPPLAYER-PRODUCER" pkg/ modules/` returns exactly the 2 sites named in spec (`pkg/script/state.go:194-196`, `modules/world/script.go:30-33`). |

---

## Task 1: `Server.LookupPlayerBySlot` + `targetOpPlayerT/U` sentinels

**Goal:** Add server-side slot lookup helper + the two new T/U sentinel constants. Foundation work for the handlers.

**Files:**
- Modify: `modules/world/server.go` (insert `LookupPlayerBySlot` after `LookupPlayerByUID` near line 715)
- Modify: `modules/world/server_test.go` (append unit tests)
- Modify: `modules/world/interaction.go` (extend the sentinel const block at line 29-34)

- [ ] **Step 1.1: Write 3 failing tests**

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

- [ ] **Step 1.2: Run tests, confirm RED**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run "TestLookupPlayerBySlot"
```

- [ ] **Step 1.3: Implement `LookupPlayerBySlot`**

Insert into `modules/world/server.go` after `LookupPlayerByUID` (after line 715-end of that function):

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

- [ ] **Step 1.4: Add the two sentinels**

In `modules/world/interaction.go`, extend the const block at lines 29-34:

```go
const (
	targetOpLocT    = 6  // APLOCT / OPLOCT dispatch marker
	targetOpLocU    = 7  // APLOCU / OPLOCU dispatch marker
	targetOpNpcT    = 8  // APNPCT / OPNPCT dispatch marker (S6o)
	targetOpNpcU    = 9  // APNPCU / OPNPCU dispatch marker (S6o)
	targetOpPlayerT = 10 // APPLAYERT / OPPLAYERT dispatch marker (NAI-40)
	targetOpPlayerU = 11 // APPLAYERU / OPPLAYERU dispatch marker (NAI-40)
)
```

- [ ] **Step 1.5: Run tests, confirm GREEN + vet + full suite**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run "TestLookupPlayerBySlot"
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...
```

- [ ] **Step 1.6: Commit**

```
feat(world): NAI-40 T1 — LookupPlayerBySlot + targetOpPlayer{T,U} sentinels
```

---

## Task 2: `handleOpPlayer1..4` (parametric op-click handler)

**Goal:** Implement the parametric op-1..4 handler + 4 thin wrappers + dispatch wiring. Mirrors TS `OpPlayerHandler.ts` and goscape's `handleOpNpc` shape exactly.

**Files:**
- New: `modules/world/handler_op_player.go`
- New: `modules/world/handler_op_player_test.go`
- Modify: `modules/world/handlers_game.go` (wire 4 dispatch entries in `init()` block)

**TS reference:** `src/network/game/client/handler/OpPlayerHandler.ts` (45 lines).

**Pre-flight:** read `modules/world/handler_opnpc.go:27-75` to mirror handler shape exactly (length check, gate sequence, error returns, sendUnsetMapFlag emission, payload-parsing pattern, 5-wrapper fan-out). Confirm `srv.rsbuf` field path via `rg -n "s\.rsbuf\\.\\|server\\.rsbuf\\.\\|rsbuf [A-Z]" modules/world/`.

- [ ] **Step 2.1: Write 5 failing handler tests**

Create `modules/world/handler_op_player_test.go`:

```go
package world

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/zsrv/goscape/pkg/io/packet"
)

// buildOpPlayerPayload encodes a 2-byte u2 PlayerSlot. The Op slot is
// supplied to the handler via a separate argument (per-op fan-out
// pattern, like handleOpNpc).
func buildOpPlayerPayload(t *testing.T, playerSlot int) []byte {
	t.Helper()
	var buf bytes.Buffer
	pk := packet.NewPacket(nil)
	pk.P2(uint16(playerSlot))
	buf.Write(pk.Bytes()) // exact API may differ; use whatever existing tests use
	return buf.Bytes()
}

// TestHandleOpPlayer_HappyPath_AllOps — for each of op 1..4, the handler
// sets target = other, targetOp = op, targetSubject.com = -1, kind =
// InteractionEngine.
func TestHandleOpPlayer_HappyPath_AllOps(t *testing.T) {
	for op := 1; op <= 4; op++ {
		t.Run(fmt.Sprintf("op=%d", op), func(t *testing.T) {
			s := newTestServer(t)
			p, other := twoTestPlayers(t, s)
			s.rsbuf.PlayerInsert(p.slot, other.slot)

			payload := buildOpPlayerPayload(t, other.slot)
			err := handleOpPlayer(p, payload, op)
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
	payload := buildOpPlayerPayload(t, other.slot)
	if err := handleOpPlayer(p, payload, 1); err != nil {
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

	payload := buildOpPlayerPayload(t, missingSlot)
	if err := handleOpPlayer(p, payload, 1); err != nil {
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

	payload := buildOpPlayerPayload(t, other.slot)
	if err := handleOpPlayer(p, payload, 1); err != nil {
		t.Fatalf("handleOpPlayer: %v", err)
	}
	if p.target != nil {
		t.Errorf("target should remain nil; got %v", p.target)
	}
	assertUnsetMapFlagWritten(t, p)
}

// TestHandleOpPlayer_TruncatedPayload — payload < 2 bytes → UnsetMapFlag.
func TestHandleOpPlayer_TruncatedPayload(t *testing.T) {
	s := newTestServer(t)
	p := newTestPlayer(t, s)

	if err := handleOpPlayer(p, []byte{0x01}, 1); err != nil {
		t.Fatalf("handleOpPlayer: %v", err)
	}
	if p.target != nil {
		t.Errorf("target: got %v, want nil", p.target)
	}
	assertUnsetMapFlagWritten(t, p)
}
```

**Note for implementer:**
- `twoTestPlayers`, `assertUnsetMapFlagWritten`, `s.rsbuf.PlayerInsert`, `buildOpPlayerPayload`, packet API names — all may differ from the placeholders shown. Use existing test helpers from `handler_opnpc_test.go` and `rsbuf_lifecycle_test.go` as canonical references.
- The `buildOpPlayerPayload` helper is a 2-byte big-endian u16 write — use whichever `pkg/io/packet` API the existing tests use (e.g., reading `interaction_test.go` or `handler_opnpc_test.go` test fixtures will show the canonical pattern).

- [ ] **Step 2.2: Run tests, confirm RED**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run "TestHandleOpPlayer"
```

Expected: 5 compilation errors (undefined `handleOpPlayer`).

- [ ] **Step 2.3: Implement `handleOpPlayer` + wrappers**

Create `modules/world/handler_op_player.go`:

```go
package world

import (
	"github.com/zsrv/goscape/pkg/io/packet"
)

// handleOpPlayer is the shared implementation for OPPLAYER1..OPPLAYER4
// (real client only sends ops 1..4 — no OPPLAYER5 wire packet).
//
// Op is 1..4. Payload = u2 PlayerSlot.
//
// Mirrors TS OpPlayerHandler.ts (45 lines): validate not-delayed,
// look up target by slot, validate visibility via rsbuf.HasPlayer,
// then anchor the engine interaction with op = msg.Op (1..4) and
// com = -1.
//
// The trigger arithmetic (TriggerApPlayer<N>, +7 → TriggerOpPlayer<N>)
// happens later in the trigger-fire path (player_interaction_trigger.go).
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
func handleOpPlayer(p *Player, payload []byte, op int) error {
	if p.client == nil || p.client.server == nil {
		return nil
	}
	s := p.client.server

	if p.delayed && s.currentTick < p.delayedUntil {
		sendUnsetMapFlag(p)
		return nil
	}

	if len(payload) < 2 {
		sendUnsetMapFlag(p)
		return nil
	}

	r := packet.NewPacket(payload)
	slot := int(r.G2())

	other := s.LookupPlayerBySlot(slot)
	if other == nil {
		sendUnsetMapFlag(p)
		return nil
	}

	if !s.rsbuf.HasPlayer(p.slot, other.slot) {
		sendUnsetMapFlag(p)
		return nil
	}

	p.ClearPendingAction()
	p.SetInteraction(InteractionEngine, other, op, -1)
	return nil
}

func handleOpPlayer1(p *Player, payload []byte) error { return handleOpPlayer(p, payload, 1) }
func handleOpPlayer2(p *Player, payload []byte) error { return handleOpPlayer(p, payload, 2) }
func handleOpPlayer3(p *Player, payload []byte) error { return handleOpPlayer(p, payload, 3) }
func handleOpPlayer4(p *Player, payload []byte) error { return handleOpPlayer(p, payload, 4) }
```

**Implementer note:** confirm `s.rsbuf.HasPlayer` exact field+method via grep (could be `s.rsbufBuilder.HasPlayer` or similar).

- [ ] **Step 2.4: Wire dispatch table**

In `modules/world/handlers_game.go`, append to the `init()` block (after the OPLOC block, around line 41):

```go
	gameHandlers[164] = handleOpPlayer1 // OPPLAYER1
	gameHandlers[53] = handleOpPlayer2  // OPPLAYER2
	gameHandlers[185] = handleOpPlayer3 // OPPLAYER3
	gameHandlers[206] = handleOpPlayer4 // OPPLAYER4
```

- [ ] **Step 2.5: Run tests, confirm GREEN + vet + full suite**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run "TestHandleOpPlayer"
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...
```

- [ ] **Step 2.6: Commit**

```
feat(world): NAI-40 T2 — handleOpPlayer (ops 1..4) + dispatch wiring
```

---

## Task 3: `handleOpPlayerT` (use spell on player)

**Goal:** Add the T variant: spell-component validation → SetInteraction with com = spellCom and op = targetOpPlayerT. Wire `gameHandlers[177]`.

**Files:** modify `modules/world/handler_op_player.go`, `modules/world/handler_op_player_test.go`, `modules/world/handlers_game.go`.

**TS reference:** `src/network/game/client/handler/OpPlayerTHandler.ts` (49 lines).

**Pre-flight:** read `modules/world/handler_opnpc.go` `handleOpNpcT` body for the existing component-validation pattern. Specifically note the exact API used for `Component.get(id)`, `actionTarget` flag check, and `isComponentVisible(com)`.

- [ ] **Step 3.1: Write 4 failing tests**

Append to `modules/world/handler_op_player_test.go`:

```go
// buildOpPlayerTPayload encodes (u2 PlayerSlot, u2 SpellCom).
func buildOpPlayerTPayload(t *testing.T, playerSlot, spellCom int) []byte {
	t.Helper()
	pk := packet.NewPacket(nil)
	pk.P2(uint16(playerSlot))
	pk.P2(uint16(spellCom))
	return pk.Bytes()
}

// TestHandleOpPlayerT_HappyPath — spellCom valid + visible + actionTarget
// has PLAYER → handler sets target = other, targetOp = targetOpPlayerT,
// targetSubject.com = spellCom, kind = InteractionEngine.
func TestHandleOpPlayerT_HappyPath(t *testing.T) {
	s := newTestServer(t)
	p, other := twoTestPlayers(t, s)
	s.rsbuf.PlayerInsert(p.slot, other.slot)

	const spellComID = 7777
	registerTestComponent(t, s, spellComID, ComActionTargetPlayer, true /* visible */, false)

	payload := buildOpPlayerTPayload(t, other.slot, spellComID)
	if err := handleOpPlayerT(p, payload); err != nil {
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

	payload := buildOpPlayerTPayload(t, other.slot, spellComID)
	if err := handleOpPlayerT(p, payload); err != nil {
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

	payload := buildOpPlayerTPayload(t, other.slot, spellComID)
	if err := handleOpPlayerT(p, payload); err != nil {
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

	payload := buildOpPlayerTPayload(t, other.slot, spellComID)
	if err := handleOpPlayerT(p, payload); err != nil {
		t.Fatalf("handleOpPlayerT: %v", err)
	}
	if p.target != nil {
		t.Errorf("target should remain nil; got %v", p.target)
	}
	assertUnsetMapFlagWritten(t, p)
}
```

**Implementer note:** `registerTestComponent` and `ComActionTargetPlayer` may need to be located in existing test helpers used by `handler_opnpc_test.go` (specifically the OpNpcT tests). Reuse them; add new helpers only if no equivalent exists.

- [ ] **Step 3.2: Run tests, confirm RED**

- [ ] **Step 3.3: Implement `handleOpPlayerT`**

Append to `modules/world/handler_op_player.go`:

```go
// handleOpPlayerT dispatches an OPPLAYERT (use-spell-on-player) packet.
// Mirrors TS OpPlayerTHandler.ts: validate not-delayed, validate the
// spell component (must exist, actionTarget include PLAYER, be visible),
// look up target by slot, validate visibility, anchor the engine
// interaction with op = targetOpPlayerT and com = spellCom.
//
// Payload = u2 PlayerSlot, u2 SpellCom (4 bytes total).
//
// DEVIATION NAI-40-D-OPCALLED-MISSING: see handleOpPlayer.
func handleOpPlayerT(p *Player, payload []byte) error {
	if p.client == nil || p.client.server == nil {
		return nil
	}
	s := p.client.server

	if p.delayed && s.currentTick < p.delayedUntil {
		sendUnsetMapFlag(p)
		return nil
	}

	if len(payload) < 4 {
		sendUnsetMapFlag(p)
		return nil
	}

	r := packet.NewPacket(payload)
	slot := int(r.G2())
	spellCom := int(r.G2())

	// Spell component must exist, target Players, and be visible.
	// Match the existing handleOpNpcT validation API exactly.
	com := s.componentLookup(spellCom)
	if com == nil || (com.ActionTarget & ComActionTargetPlayer) == 0 {
		sendUnsetMapFlag(p)
		return nil
	}
	if !p.IsComponentVisible(com) {
		sendUnsetMapFlag(p)
		return nil
	}

	other := s.LookupPlayerBySlot(slot)
	if other == nil {
		sendUnsetMapFlag(p)
		return nil
	}
	if !s.rsbuf.HasPlayer(p.slot, other.slot) {
		sendUnsetMapFlag(p)
		return nil
	}

	p.ClearPendingAction()
	p.SetInteraction(InteractionEngine, other, targetOpPlayerT, spellCom)
	return nil
}
```

**Implementer note:** `s.componentLookup`, `com.ActionTarget`, `ComActionTargetPlayer`, `p.IsComponentVisible(com)` are placeholder names — match whatever `handleOpNpcT` uses verbatim. If the existing handler uses a different gate-check helper, reuse it (DRY).

- [ ] **Step 3.4: Wire dispatch entry**

Append to `modules/world/handlers_game.go` `init()` block (immediately after the 4 OPPLAYER<N> entries from T2):

```go
	gameHandlers[177] = handleOpPlayerT // OPPLAYERT
```

- [ ] **Step 3.5: Run tests + vet + full suite**

- [ ] **Step 3.6: Commit**

```
feat(world): NAI-40 T3 — handleOpPlayerT (use spell on player) + dispatch wiring
```

---

## Task 4: `handleOpPlayerU` (use item on player)

**Goal:** Add the U variant: full validation chain (component usable + visible, invListener exists, slot valid, item match, members check) → snapshot lastUseItem/lastUseSlot → SetInteraction with com = -1 and op = targetOpPlayerU. Wire `gameHandlers[248]`.

**Files:** modify `modules/world/handler_op_player.go`, `modules/world/handler_op_player_test.go`, `modules/world/handlers_game.go`.

**TS reference:** `src/network/game/client/handler/OpPlayerUHandler.ts` (75 lines).

**Pre-flight:** read `modules/world/handler_opnpc.go` `handleOpNpcU` body for the existing full validation chain — invListener / inv slot / hasAt / members / lastUseItem snapshot. Confirm helper names: `findInvListener` / `getInventoryFromListener` / `Inv.ValidSlot` / `Inv.HasAt` / `objType.Members` / `s.cfg.NodeMembers`.

- [ ] **Step 4.1: Write 6 failing tests**

Append to `modules/world/handler_op_player_test.go`:

```go
func buildOpPlayerUPayload(t *testing.T, playerSlot, useObj, useSlot, useCom int) []byte {
	t.Helper()
	pk := packet.NewPacket(nil)
	pk.P2(uint16(playerSlot))
	pk.P2(uint16(useObj))
	pk.P2(uint16(useSlot))
	pk.P2(uint16(useCom))
	return pk.Bytes()
}

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

	payload := buildOpPlayerUPayload(t, other.slot, useObj, useSlot, useComID)
	if err := handleOpPlayerU(p, payload); err != nil {
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

// 5 more tests (bodies modeled on the equivalent OpNpcU tests in
// handler_opnpc_test.go); structure shown, body elided:
func TestHandleOpPlayerU_ComponentNotUsable(t *testing.T)        { /* ... */ }
func TestHandleOpPlayerU_InvListenerMissing(t *testing.T)        { /* ... */ }
func TestHandleOpPlayerU_InvalidSlot(t *testing.T)               { /* ... */ }
func TestHandleOpPlayerU_ItemMismatch(t *testing.T)              { /* ... */ }
func TestHandleOpPlayerU_MembersOnNonMembersServer(t *testing.T) { /* ... */ }
```

**Implementer note:** flesh out the 5 elided test bodies by mirroring the equivalent OpNpcU tests in `handler_opnpc_test.go`. Reuse helpers (`addInvListener`, `registerTestComponent`, etc.).

- [ ] **Step 4.2: Run tests, confirm RED**

- [ ] **Step 4.3: Implement `handleOpPlayerU`**

Append to `modules/world/handler_op_player.go`:

```go
// handleOpPlayerU dispatches an OPPLAYERU (use-item-on-player) packet.
// Mirrors TS OpPlayerUHandler.ts: validate not-delayed, validate the
// use component (usable + visible), look up the inventory listener and
// validate slot + item match, members-server check, then snapshot
// lastUseItem/lastUseSlot and anchor the engine interaction with
// op = targetOpPlayerU and com = -1 (TS quirk: useCom not snapshotted).
//
// Payload = u2 PlayerSlot, u2 UseObj, u2 UseSlot, u2 UseCom (8 bytes total).
//
// DEVIATION NAI-40-D-OPCALLED-MISSING: see handleOpPlayer.
func handleOpPlayerU(p *Player, payload []byte) error {
	if p.client == nil || p.client.server == nil {
		return nil
	}
	s := p.client.server

	if p.delayed && s.currentTick < p.delayedUntil {
		sendUnsetMapFlag(p)
		return nil
	}

	if len(payload) < 8 {
		sendUnsetMapFlag(p)
		return nil
	}

	r := packet.NewPacket(payload)
	slot := int(r.G2())
	useObj := int(r.G2())
	useSlot := int(r.G2())
	useCom := int(r.G2())

	// Use component must exist, be usable, and be visible.
	com := s.componentLookup(useCom)
	if com == nil || !com.Usable {
		sendUnsetMapFlag(p)
		return nil
	}
	if !p.IsComponentVisible(com) {
		sendUnsetMapFlag(p)
		return nil
	}

	listener := p.findInvListener(useCom)
	if listener == nil {
		sendUnsetMapFlag(p)
		return nil
	}
	inv := resolveListenerInv(s, *listener)
	if inv == nil {
		sendUnsetMapFlag(p)
		return nil
	}
	if !inv.ValidSlot(useSlot) {
		sendUnsetMapFlag(p)
		return nil
	}
	if !inv.HasAt(useSlot, useObj) {
		sendUnsetMapFlag(p)
		return nil
	}

	other := s.LookupPlayerBySlot(slot)
	if other == nil {
		sendUnsetMapFlag(p)
		return nil
	}
	if !s.rsbuf.HasPlayer(p.slot, other.slot) {
		sendUnsetMapFlag(p)
		return nil
	}

	p.ClearPendingAction()

	if objType := s.objTypeLookup(useObj); objType != nil && objType.Members && !s.cfg.NodeMembers {
		p.MessageGame("To use this item please login to a members' server.")
		sendUnsetMapFlag(p)
		return nil
	}

	p.lastUseItem = useObj
	p.lastUseSlot = useSlot
	p.SetInteraction(InteractionEngine, other, targetOpPlayerU, -1)
	return nil
}
```

**Implementer notes:**
- Helper names (`s.componentLookup`, `p.findInvListener`, `resolveListenerInv`, `inv.ValidSlot`, `inv.HasAt`, `s.objTypeLookup`, `s.cfg.NodeMembers`, `objType.Members`) are placeholders — match whatever the existing OpNpcU handler uses verbatim. `resolveListenerInv` actually exists at `handler_opnpc.go:13` and may be the canonical helper.
- The members-check ordering in TS is **after** target+rsbuf validation and **before** the lastUseItem snapshot. Mirror exactly.

- [ ] **Step 4.4: Wire dispatch entry**

Append to `modules/world/handlers_game.go` `init()` block:

```go
	gameHandlers[248] = handleOpPlayerU // OPPLAYERU
```

- [ ] **Step 4.5: Run tests + vet + full suite**

- [ ] **Step 4.6: Commit**

```
feat(world): NAI-40 T4 — handleOpPlayerU (use item on player) + dispatch wiring
```

---

## Task 5: `player_interaction_trigger.go` — trigger maps + fire functions + tryFire dispatch

**Goal:** Land the trigger plumbing that closes the loop. Add the player-actor trigger map (`apPlayerTriggerForOp`), the AP/OP fire functions (`fireOpTriggerPlayer`, `fireApTriggerPlayer`), and extend the existing `tryFireOpTrigger` / `tryFireApTrigger` type-switches with a `case *Player:` arm. Self2 is bound by routing the fire functions through `srv.runScript(sf, target, p, ...)` so NAI-39's `case script.ActivePlayer:` arm in `buildPlayerScriptState` (script.go:54-57) fires.

**Files:**
- New: `modules/world/player_interaction_trigger.go`
- New: `modules/world/player_interaction_trigger_test.go`
- Modify: `modules/world/interaction_trigger.go` (extend `tryFireOpTrigger` / `tryFireApTrigger` switches)

**TS reference:** `src/engine/entity/Player.ts` `tryInteract()` (lines 1115-1199), `src/engine/script/ScriptRunner.ts:78-92`.

**Pre-flight:**
- `rg -n "tryFireOpTrigger\|tryFireApTrigger" modules/world/` and enumerate every call site (per `enumerate_all_sites.md`).
- `rg -n "scriptProvider\.GetByTrigger" modules/world/` to verify the registry call sites' arg shapes.
- `rg -n "TriggerApPlayer\|TriggerOpPlayer" pkg/script/trigger.go` to confirm constants for ops 1..4 + T + U.
- TBV at impl time: registry behavior for `GetByTrigger(trigger, -1, -1)` when target has no type. If registry rejects -1, this becomes a blocker — escalate.

- [ ] **Step 5.1: Write 5 failing tests**

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
		op     int
		want   script.ServerTriggerType
		wantOk bool
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
	clicker.MoveTo(target.x+1, target.z, target.level)
	clicker.SetInteraction(InteractionEngine, target, 1, -1)
	clicker.processInteraction()

	if clicker.target != nil {
		t.Errorf("target should be cleared; got %v", clicker.target)
	}
}

func TestFireApTriggerPlayer_NoScriptSetsApRangeMinusOne(t *testing.T) {
	s := newTestServer(t)
	clicker, target := twoTestPlayers(t, s)
	s.rsbuf.PlayerInsert(clicker.slot, target.slot)
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
	// (full body — model on existing TestTryFireOpTrigger_NpcArm if it
	// exists, otherwise model on interaction_trigger_test.go fixtures)
}
```

**Implementer note:** `scriptHintPlActivePlayer2` is a fixture helper that compiles a tiny script with bytecode that pushes the active_player2 reference and emits the HINT_PL opcode. Reuse the closest NAI-39 fixture from `pkg/script/handlers_player_test.go` (the HINT_PL tests). If no analogous helper exists in `modules/world/`, build per `scriptstate_test_fixture_idioms.md` (correct push order + Pointers flag for multi-guard handlers).

- [ ] **Step 5.2: Run tests, confirm RED**

- [ ] **Step 5.3: Implement `apPlayerTriggerForOp` + fire functions**

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
// Note: ops are 1..4, NOT 1..5 — real client only sends OPPLAYER1..4.
// TriggerOpPlayer<5> referenced in NPC-AI material is an
// AI-side concept (TriggerAiOpPlayer1..5 in trigger.go).
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
	// TBV: registry behaviour for -1 sentinels — confirmed at impl
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

- [ ] **Step 5.4: Extend `tryFireOpTrigger` / `tryFireApTrigger`**

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

- [ ] **Step 5.5: Run tests + vet + full suite**

- [ ] **Step 5.6: Commit**

```
feat(world): NAI-40 T5 — player_interaction_trigger.go + tryFire dispatch arms
```

---

## Task 6: E2E smoke + close commit (deviation comment retirement)

**Goal:** Land the end-to-end deviation-closure pin and remove the now-stale deviation comments at `pkg/script/state.go:194-196` and `modules/world/script.go:30-33`.

**Files:**
- Modify: `modules/world/script_test.go` (append E2E smoke)
- Modify: `pkg/script/state.go` (retire deviation comment lines 194-196)
- Modify: `modules/world/script.go` (retire deviation comment lines 30-33)

- [ ] **Step 6.1: Write E2E smoke**

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
	registerTriggerScript(t, s, script.TriggerOpPlayer1, scriptHintPlActivePlayer2(t))

	// Place clicker adjacent to target so the OP branch fires.
	clicker.MoveTo(target.x+1, target.z, target.level)

	// Simulate the wire packet via the handler entry point.
	payload := buildOpPlayerPayload(t, target.slot)
	if err := handleOpPlayer(clicker, payload, 1); err != nil {
		t.Fatalf("handleOpPlayer: %v", err)
	}

	// Drive one tick.
	clicker.processInteraction()

	// Assert: target's outbound mask has a hint-arrow pointing at clicker.
	assertHintPlayerOnMask(t, target, clicker.slot)
}
```

- [ ] **Step 6.2: Run E2E test, confirm GREEN**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run "TestOpPlayer1_E2E_HintPlOnTarget"
```

- [ ] **Step 6.3: Retire deviation comments**

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

- [ ] **Step 6.4: Verify deviation-tag retirement**

```
rg -n "NAI-39-D-ACTIVEPLAYER2-NO-OPPLAYER-PRODUCER" pkg/ modules/
```

Expected: 0 hits in production code.

- [ ] **Step 6.5: Run full suite + vet**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...
```

- [ ] **Step 6.6: Final whole-impl review**

Dispatch a code-reviewer agent (subagent_type: `superpowers:code-reviewer`) over commits T1..T5 (and the T6 retirement commit) with the spec at `docs/superpowers/specs/2026-04-27-nai-40-opplayer-producer-design.md` for spec-compliance + code-quality cross-check. Apply review fixes as `polish(...)` commits per `runescript_cadence.md`.

- [ ] **Step 6.7: Close commit**

```
chore(script,world): NAI-40 closed — OPPLAYER triggers (player→player op-click)

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
- **Verify implementer claims** per `verify_implementer_claims.md`: after each task commit, run `git show <SHA> --stat` + `go test ./...` from the controller's clean working tree.
- **Closes-memory trailer** per `close_commit_memory_trailer.md`: T6 close commit lists every memory entry produced during NAI-40.
- **Memory updates at close** per `post_task_handoff.md`: add NAI-40 entry to `nai_followups.md` with the deviation ledger, refresh the "OPPLAYER triggers" item in the followups list (now CLOSED), save lessons learned.

## TS source citations (for plan-author audit and implementer reference)

- `src/network/game/client/handler/OpPlayerHandler.ts` (45 lines)
- `src/network/game/client/handler/OpPlayerTHandler.ts` (49 lines)
- `src/network/game/client/handler/OpPlayerUHandler.ts` (75 lines)
- `src/engine/script/ScriptRunner.ts:78-92` (target-binding switch)
- `src/engine/entity/Player.ts:1115-1199` (`tryInteract` body — interaction tick)
- `src/engine/script/ScriptState.ts:80,215-243` (activePlayer/activePlayer2 getters/setters)
