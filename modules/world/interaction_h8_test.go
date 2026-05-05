package world

import (
	"testing"

	io2 "github.com/zsrv/goscape/pkg/io/isaac"
)

// TestProcessInteractionRepathsAfterPathExhaustion — NAI-98 sub-H8 regression.
// Pre-NAI-98: interaction.go:236-239 gates pathToTarget() on `!p.repathed`,
// a once-per-interaction-lifecycle boolean. Post-fix: pathToPathingTarget
// gates SMART repath on isLastOrNoWaypoint (TS Player.ts:1034-1055,
// PathingEntity.ts:374-376).
//
// Repro shape: player anchors target NPC at cheb=15 (out of apRange=10);
// first processInteraction queues path; we manually exhaust the path
// (waypointIndex=-1 with target still anchored); second processInteraction
// must re-queue path. Pre-fix: !p.repathed gate is false on tick 2 →
// pathToTarget skipped → !hasWaypoints && stepsTaken==0 → "I can't reach
// that" + ClearInteraction (target=nil).
func TestProcessInteractionRepathsAfterPathExhaustion(t *testing.T) {
	s := newTestServer(t)
	s.cfg.NodeClientRoutefinder = true
	npc := makeInteractionNpc(t, s, 1, 115, 100, 0) // cheb=15

	p, cc := newTestPlayer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	p.x, p.z, p.level = 100, 100, 0

	p.SetInteraction(InteractionEngine, npc, 1, -1)

	// Tick 1: initial pathToPathingTarget queues path.
	received := drainConn(t, cc)
	p.processInteraction()
	p.client.flushWrite()
	<-received

	if p.waypointIndex < 0 {
		t.Fatalf("tick 1: waypointIndex=%d, want >= 0 (initial repath)", p.waypointIndex)
	}
	if p.target == nil {
		t.Fatal("tick 1: target cleared unexpectedly")
	}

	// Simulate path exhaustion mid-interaction.
	p.waypointIndex = -1

	// Tick 2: pathToPathingTarget MUST re-queue path. Pre-fix would skip
	// because !p.repathed=false, then "I can't reach that" + Clear fires.
	received = drainConn(t, cc)
	p.processInteraction()
	p.client.flushWrite()
	<-received

	if p.target == nil {
		t.Errorf("tick 2: target cleared (interaction abandoned). Pre-fix !p.repathed gate prevented repath; expected pathToPathingTarget to re-queue path on isLastOrNoWaypoint.")
	}
	if p.waypointIndex < 0 {
		t.Errorf("tick 2: waypointIndex=%d after path exhaustion. pathToPathingTarget did not re-queue waypoints. This is the H8 bug: TS Player.ts:1052-1054 gates on isLastOrNoWaypoint; goscape gated on !p.repathed once-per-interaction.", p.waypointIndex)
	}
}
