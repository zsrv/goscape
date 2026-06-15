package world

import (
	"testing"

	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/script"
)

// TestResumeOrFinishNpcReleasesProtectedActivePlayer verifies the TS
// Npc.executeScript protect-cleanup tail:
//
//	if (script.pointerGet(ScriptPointer.ProtectedActivePlayer) && script._activePlayer) {
//	    script._activePlayer.protect = false;
//	    script.pointerRemove(ScriptPointer.ProtectedActivePlayer);
//	}
//
// When an NPC-anchored script holds protected access on its active player
// (an ai_opplayer / ai_applayer trigger run with protect), the player's
// protect flag must be cleared and the ProtectedActivePlayer pointer removed
// when the script settles — otherwise a later interaction is blocked by a
// stale protect flag (protectedScriptActive stays true). Unlike the player
// dispatcher (which clears the protagonist's own protect, the protagonist
// BEING the active player), an NPC-anchored script's active player is
// SECONDARY, so resumeOrFinishNpc must release it explicitly.
func TestResumeOrFinishNpcReleasesProtectedActivePlayer(t *testing.T) {
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
	p.protect = true

	// Trivial npc-anchored script (just returns) holding protected access on
	// the active player.
	sf := &script.ScriptFile{Name: "npc_noop", Opcodes: []script.Opcode{script.OpReturn}}
	state := s.buildNpcScriptState(sf, npc, p, script.TriggerAiOpPlayer1, nil, nil)
	state.Pointers |= script.PtrProtectedActivePlayer

	s.resumeOrFinishNpc(state, npc)

	if p.protect {
		t.Errorf("player.protect: got true, want false (Npc.executeScript tail must release protected access)")
	}
	if state.Pointers&script.PtrProtectedActivePlayer != 0 {
		t.Errorf("PtrProtectedActivePlayer: still set, want removed (Npc.executeScript tail)")
	}
}
