package world

import "github.com/zsrv/goscape/pkg/script"

// NpcEventType mirrors TS NpcEventType at
// Engine-TS/src/engine/entity/NpcEventRequest.ts.
// NpcEventSpawn is reserved for TS fidelity but has no producer in
// NAI-5 (no script-driven NPC creation yet); NpcEventDespawn is
// queued by the DESPAWN branch of the Npc.turn() Events block.
type NpcEventType int

const (
	NpcEventSpawn   NpcEventType = 0
	NpcEventDespawn NpcEventType = 1
)

// NpcEventRequest is a queued world-level NPC event. The Events
// block in Npc.turn() enqueues one of these when an NPC's
// lifecycleTick hits zero on a DESPAWN-lifecycle NPC; the next
// processNpcEventQueue pass dispatches the script if the NPC is
// not delayed. Matches TS NpcEventRequest.
type NpcEventRequest struct {
	Type   NpcEventType
	Script *script.ScriptFile
	Npc    *Npc
}

// processNpcEventQueue dispatches any queued NPC events whose NPC
// is not currently delayed. Runs BEFORE processNpcs each tick,
// matching TS World.ts:356. Events for delayed NPCs are left in
// the queue and retried next tick — matches TS World.ts:664-673.
//
// Iteration uses the same removal-before-fire + don't-advance-i
// pattern as processNpcQueue (NAI-3) so a fired script that
// appends a new event sees it in the same pass.
func (s *Server) processNpcEventQueue() {
	i := 0
	for i < len(s.npcEventQueue) {
		req := s.npcEventQueue[i]
		if req.Npc.delayed {
			i++
			continue
		}
		s.npcEventQueue = append(s.npcEventQueue[:i], s.npcEventQueue[i+1:]...)
		s.runNpcScript(req.Script, req.Npc, nil, nil)
		// don't advance i — removed current entry
	}
}
