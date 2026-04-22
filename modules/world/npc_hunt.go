package world

import (
	"math/rand/v2"

	"github.com/zsrv/goscape/pkg/objtype"
)

// processNpcHunt runs the per-tick hunt pass. Matches TS
// Npc.ts:158-171.
//
// Observer gate: TS checks rsbuf.getNpcObservers(this.nid); Go
// has no equivalent observer-count API yet, so we inline
// `observers := 1` (always observed). PAUSEHUNT semantics are
// currently unobservable — tracked as follow-up in nai_followups
// memory. TS NobodyNear values:
//   - HuntNobodyNearKeepHunting: gate always passes
//   - HuntNobodyNearPauseHunt: gate passes iff observers > 0 OR
//     type == HuntModePlayer
//
// Note on mode bounds: SetHuntMode accepts any int (including
// out-of-range) to match TS. processNpcHunt validates bounds
// against s.huntTypes.Configs and silently no-ops on invalid.
func (s *Server) processNpcHunt(n *Npc) {
	if n.huntMode == -1 {
		return
	}
	if s.huntTypes == nil ||
		n.huntMode < 0 ||
		n.huntMode >= len(s.huntTypes.Configs) {
		return
	}
	hunt := s.huntTypes.Configs[n.huntMode]
	if hunt == nil {
		return
	}
	observers := 1 // TODO: rsbuf.GetNpcObservers(n.nid) when available
	if hunt.NobodyNear == objtype.HuntNobodyNearPauseHunt &&
		observers <= 0 &&
		hunt.Type != objtype.HuntModePlayer {
		return
	}
	// Player-type hunts skip the huntAll dispatcher at the turn()
	// level — matches TS Npc.ts:164 "hunt && hunt.type !== HuntModeType.PLAYER".
	// The HuntModePlayer branch in huntAll is reachable only via
	// explicit scripted calls, not the turn() path.
	if hunt.Type != objtype.HuntModePlayer {
		n.huntAll(s, hunt)
	}
	n.huntClock++
}

// huntAll dispatches to a hunted-type variant and sets huntTarget.
// Matches TS Npc.ts:249-277. Variants are stubs at NAI-7; NAI-8
// fills huntPlayers; NAI-9 fills huntNpcs/huntObjs/huntLocs.
func (n *Npc) huntAll(s *Server, hunt *objtype.HuntType) {
	n.huntTarget = nil
	if n.huntClock < hunt.Rate-1 {
		return
	}
	if hunt.Type == objtype.HuntModeOff || n.huntRange < 1 {
		return
	}
	var hunted []entity
	switch hunt.Type {
	case objtype.HuntModePlayer:
		hunted = n.huntPlayers(s, hunt)
	case objtype.HuntModeNpc:
		hunted = n.huntNpcs(s, hunt)
	case objtype.HuntModeObj:
		hunted = n.huntObjs(s, hunt)
	case objtype.HuntModeScenery:
		hunted = n.huntLocs(s, hunt)
	}
	if len(hunted) > 0 {
		n.huntTarget = hunted[rand.IntN(len(hunted))]
	}
}

// huntPlayers is stubbed at NAI-7; NAI-8 fills the body.
func (n *Npc) huntPlayers(s *Server, hunt *objtype.HuntType) []entity { return nil }

// huntNpcs is stubbed at NAI-7; NAI-9 fills the body.
func (n *Npc) huntNpcs(s *Server, hunt *objtype.HuntType) []entity { return nil }

// huntObjs is stubbed at NAI-7; NAI-9 fills the body.
func (n *Npc) huntObjs(s *Server, hunt *objtype.HuntType) []entity { return nil }

// huntLocs is stubbed at NAI-7; NAI-9 fills the body.
func (n *Npc) huntLocs(s *Server, hunt *objtype.HuntType) []entity { return nil }
