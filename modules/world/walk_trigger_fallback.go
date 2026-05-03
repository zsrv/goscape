package world

// processWalkTriggerFallback is the per-player tick phase that mirrors
// TS World.ts:635-641. Skipped under WalkTriggerSettingPlayerpacket
// (the default — handler-side dispatch already covered the work).
//
// Under non-PLAYERPACKET settings:
//   - re-path from p.userPath each tick (mirrors TS L636
//     `player.pathToMoveClick(player.userPath, !NODE_CLIENT_ROUTEFINDER)`)
//   - PLAYERSETUP additionally fires processWalktrigger when
//     !opcalled && hasWaypoints (TS L638).
//   - PLAYERMOVEMENT re-paths only.
//
// Insertion phase: invoked from tick.go after processInteractions, NOT
// inside processInteraction itself (which is target-gated and would
// skip target-less players — wrong for plain move-click flows).
//
// NAI-77-D-WALKTRIGGER-FALLBACK-PHASE-CHOICE:
// TS World.ts:635-641 runs this fallback inside the per-player loop
// BEFORE processPathing (so re-pathed waypoints are consumed in the
// same tick). Goscape invokes it AFTER processInteractions, which
// itself runs after processPathing — so waypoints queued by the
// fallback are not consumed until the NEXT tick's processPathing.
// Default cfg (PLAYERPACKET) makes this a per-player no-op so there
// is no production-visible effect today; when the project switches
// to PLAYERSETUP / PLAYERMOVEMENT, revisit phase ordering. Tracked
// per spec NAI-77 §7 R3.
func processWalkTriggerFallback(p *Player) {
	if p.client == nil || p.client.server == nil {
		return
	}
	s := p.client.server
	if s.cfg.NodeWalktriggerSetting == WalkTriggerSettingPlayerpacket {
		return
	}

	p.pathToMoveClick(p.userPath, !s.cfg.NodeClientRoutefinder)

	if s.cfg.NodeWalktriggerSetting == WalkTriggerSettingPlayersetup &&
		!p.opcalled && p.hasWaypoints() {
		p.processWalktrigger()
	}
}

// processWalkTriggerFallbacks runs processWalkTriggerFallback once
// per active player per tick. Under default (PLAYERPACKET) cfg this
// is a per-player no-op; the iteration cost is negligible.
func (s *Server) processWalkTriggerFallbacks() {
	s.playersMu.RLock()
	players := make([]*Player, len(s.playerLoop))
	copy(players, s.playerLoop)
	s.playersMu.RUnlock()

	for _, p := range players {
		func(p *Player) {
			defer recoverPlayer(p, "processWalkTriggerFallback", s.log)
			processWalkTriggerFallback(p)
		}(p)
	}
}
