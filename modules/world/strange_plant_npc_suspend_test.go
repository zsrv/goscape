package world

import (
	"testing"

	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/script"
)

// TestOpNpcScriptNpcDelayStoresContinuationOnNpc reproduces the Strange
// Plant (triffid) random-event bug: after the player picks the fruit, the
// plant still attacks.
//
// The content script macro_event_triffid.rs2 ends its [opnpc1] pick handler
// with `npc_delay(22); npc_del;`. opnpc is a PLAYER-anchored script that
// also carries an ActiveNpc (the plant), so npc_delay transitions the script
// to NpcSuspended and delays the NPC.
//
// TS Player.executeScript (Engine-TS Player.ts:2220-2221) stores the
// NpcSuspended continuation ON THE ACTIVE NPC:
//
//	} else if (state === ScriptState.NPC_SUSPENDED) {
//	    script.activeNpc.activeScript = script;
//	}
//
// so Npc.turn() (Npc.ts:116-118) resumes it when the NPC's delay expires —
// running npc_del and removing the plant before its hostile ai_timer can
// fire again.
//
// goscape's player-side resumeOrFinish had no NpcSuspended arm: it fell to
// the default case, logged "unsupported execution state", and dropped the
// continuation. The plant was delayed 22 ticks but never deleted, so its
// ai_timer resumed and turned it hostile — the user picked the fruit yet was
// still attacked.
func TestOpNpcScriptNpcDelayStoresContinuationOnNpc(t *testing.T) {
	s := newTestServer(t)
	s.worldVars = worldVarsView{s: s} // production wiring (server.go:533)
	s.currentTick = 100

	typ := &objtype.NpcType{
		ID: 0, DebugName: "macro_triffidseed",
		Op:   []string{"Pick"},
		Size: 1,
	}
	npc := NewNpc(1, 0, 3200, 3200, 0, typ)
	npc.nid = 1
	npc.lifecycle = NpcLifecycleDespawn // macro-event NPCs are dynamically spawned
	npc.server = s
	s.npcs[1] = npc
	s.npcLoop = append(s.npcLoop, npc)

	p, _ := newTestPlayer(t)
	p.client.server = s

	// Mirror the opnpc1 tail: npc_delay(22); npc_del;
	sf := &script.ScriptFile{
		Name:        "triffid_pick_tail",
		Opcodes:     []script.Opcode{script.OpPushConstantInt, script.OpNpcDelay, script.OpNpcDel, script.OpReturn},
		IntOperands: []int32{22},
	}

	// Player picks the fruit (opnpc1 fires the player-anchored script).
	s.runScript(sf, p, npc, script.TriggerOpNpc1, false, nil, nil)

	// The NPC must be delayed AND hold the suspended continuation so that
	// npc_del runs when the delay expires.
	if !npc.delayed {
		t.Fatalf("npc.delayed: got false, want true after npc_delay(22)")
	}
	if npc.activeScript == nil {
		t.Fatalf("npc.activeScript: got nil, want the suspended npc_del continuation " +
			"(player resumeOrFinish dropped the NpcSuspended continuation)")
	}
	if npc.activeScript.Execution != script.NpcSuspended {
		t.Fatalf("npc.activeScript.Execution: got %v, want NpcSuspended", npc.activeScript.Execution)
	}

	// Resume after the delay expires: npc_del must run and remove the plant.
	nid := npc.nid
	s.currentTick = npc.delayedUntil
	npc.turn(s)

	if !npc.dead {
		t.Errorf("npc.dead: got false, want true (npc_del should have removed the plant)")
	}
	if s.npcs[nid] != nil {
		t.Errorf("s.npcs[%d]: got %v, want nil (despawn should release the slot)", nid, s.npcs[nid])
	}
}
