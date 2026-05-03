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
