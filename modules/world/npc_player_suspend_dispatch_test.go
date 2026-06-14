package world

import (
	"testing"

	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/script"
)

// TestResumeOrFinishNpcStoresPlayerSuspendOnActivePlayer verifies the NPC
// dispatcher's player-suspend arm (Suspended / PauseButton / CountDialog).
//
// An NPC-anchored script can carry an active player: buildNpcScriptState
// binds an ActivePlayer target → state.Self (e.g. an ai_opplayer /
// ai_applayer trigger whose target is the player). If such a script suspends
// ON THAT PLAYER (p_delay here), TS Npc.executeScript stores the continuation
// on the active player:
//
//	} else { script.activePlayer.activeScript = script; }
//
// so the player's tick loop resumes it. goscape's resumeOrFinishNpc default
// arm previously DROPPED it (npc.ClearActiveScript + warn) — the same
// dropped-continuation shape as the Strange Plant opnpc bug, on the NPC side.
//
// This path is unreachable in the pinned content (the player-suspend opcodes
// require ProtectedActivePlayer, which NPC-anchored scripts don't grant; the
// test grants it explicitly), but is mirrored for TS fidelity so a future
// content path can't silently lose the continuation.
func TestResumeOrFinishNpcStoresPlayerSuspendOnActivePlayer(t *testing.T) {
	s := newTestServer(t)
	s.worldVars = worldVarsView{s: s}
	s.currentTick = 100

	typ := &objtype.NpcType{ConfigType: objtype.ConfigType{ID: 0, DebugName: "test_npc"}, Size: 1}
	npc := NewNpc(1, 0, 3200, 3200, 0, typ)
	npc.nid = 1
	npc.server = s
	s.npcs[1] = npc

	p, _ := newTestPlayer(t)
	p.client.server = s

	// npc-anchored script that delays the active player (p_delay) → Suspended,
	// with a continuation (the trailing return) the player must resume.
	sf := &script.ScriptFile{
		Name:        "npc_pdelay_player",
		Opcodes:     []script.Opcode{script.OpPushConstantInt, script.OpPDelay, script.OpReturn},
		IntOperands: []int32{5},
	}

	// ai_opplayer shape: target is the player, so state.Self = player +
	// PtrActivePlayer. Grant protected access so p_delay is permitted.
	state := s.buildNpcScriptState(sf, npc, p, script.TriggerAiOpPlayer1, nil, nil)
	state.Pointers |= script.PtrProtectedActivePlayer

	s.resumeOrFinishNpc(state, npc)

	if state.Execution != script.Suspended {
		t.Fatalf("Execution: got %v, want Suspended (p_delay)", state.Execution)
	}
	if p.activeScript != state {
		t.Fatalf("player.activeScript: got %v, want the suspended continuation "+
			"(resumeOrFinishNpc dropped the player-suspend continuation)", p.activeScript)
	}
	if npc.activeScript != nil {
		t.Errorf("npc.activeScript: got %v, want nil (continuation belongs to the player)", npc.activeScript)
	}
}
