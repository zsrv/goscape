package world

import (
	"testing"

	"github.com/zsrv/goscape/pkg/objtype"
)

// TS MoveClickHandler.ts:26-32 @4c95f87e (upstream 3da10133):
//
//	// Clear previous interaction — but not for op-click moves.
//	// A MOVE_OPCLICK is always paired with a following op packet that clears+sets
//	// the interaction itself. Clearing here would drop the target in the gap when
//	// the per-tick user packet limit splits the pair across ticks.
//	if (!message.opClick) {
//	    player.clearPendingAction();
//	}
//
// This reverts the f0ccbe8a "unconditional clear" posture goscape carried at
// the dee467c8 pin: MOVE_OPCLICK must no longer clear the pending
// interaction, while MOVE_GAMECLICK (and MOVE_MINIMAPCLICK, which also
// decodes opClick=false) still do.

// TestMoveOpClickPreservesPendingAction pins that MOVE_OPCLICK (opcode 167)
// leaves a previously-set interaction target intact. The client always
// follows an op-click move with a separate op packet that clears+sets the
// interaction itself, so clearing it here would drop the target in the gap
// if the per-tick user packet limit splits the pair across ticks.
func TestMoveOpClickPreservesPendingAction(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.client.server = newTestServer(t)

	npc := NewNpc(1, 0, 100, 100, 0, &objtype.NpcType{})
	p.SetInteraction(InteractionEngine, npc, 1, -1)
	if p.target != npc {
		t.Fatalf("setup: p.target got %v, want npc", p.target)
	}

	// Valid single-waypoint move click at the player's own tile (within the
	// 104-tile distanceToSW bound).
	payload := buildMovePayload(0, p.x, p.z)
	if err := handleMoveOpClick(p, payload); err != nil {
		t.Fatalf("handleMoveOpClick: %v", err)
	}

	if p.target != npc {
		t.Errorf("target: got %v, want npc (MOVE_OPCLICK must NOT clear the pending interaction, TS MoveClickHandler.ts:26-32 @4c95f87e)", p.target)
	}
}

// TestMoveGameClickClearsPendingAction pins that MOVE_GAMECLICK (opcode 63)
// continues to clear a previously-set interaction target — the !opClick
// branch of TS MoveClickHandler.ts:30-32 @4c95f87e.
func TestMoveGameClickClearsPendingAction(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.client.server = newTestServer(t)

	npc := NewNpc(1, 0, 100, 100, 0, &objtype.NpcType{})
	p.SetInteraction(InteractionEngine, npc, 1, -1)
	if p.target != npc {
		t.Fatalf("setup: p.target got %v, want npc", p.target)
	}

	payload := buildMovePayload(0, p.x, p.z)
	if err := handleMoveGameClick(p, payload); err != nil {
		t.Fatalf("handleMoveGameClick: %v", err)
	}

	if p.target != nil {
		t.Errorf("target: got %v, want nil (MOVE_GAMECLICK must still clear the pending interaction)", p.target)
	}
}
