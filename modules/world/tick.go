package world

import (
	"sort"
	"time"

	"github.com/zsrv/goscape/pkg/buildarea"
	"github.com/zsrv/goscape/pkg/inventory"
	gameserver "github.com/zsrv/goscape/pkg/io/protocol/game/server"
	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/rsbuf"
	"github.com/zsrv/goscape/pkg/script"
)

const tickRate = 600 * time.Millisecond

const (
	timeoutNoResponse   = 100 // ticks = 60s at 600ms
	timeoutNoConnection = 50  // ticks = 30s at 600ms
)

func (s *Server) runTickLoop() {
	s.runTickLoopWithRate(tickRate)
}

func (s *Server) runTickLoopWithRate(rate time.Duration) {
	nextTick := time.Now()
	for {
		start := time.Now()
		drift := start.Sub(nextTick)
		if drift < 0 {
			drift = 0
		}

		s.processClientsIn()
		s.processActiveScripts()
		s.processPlayerTimers()
		s.processPathing()
		s.processInteractions()
		s.processNpcs()
		s.processLogouts()
		s.processLogins()
		s.processInfo()
		s.processZones() // compute ComputeShared before delivery
		s.processClientsOut()
		s.processCleanup()
		s.currentTick++

		nextTick = nextTick.Add(rate)
		delay := rate - time.Since(start) - drift
		if delay < 0 {
			delay = 0
		}

		select {
		case <-s.quit:
			return
		case <-time.After(delay):
		}
	}
}

func (s *Server) processClientsIn() {
	s.playersMu.RLock()
	players := make([]*Player, len(s.playerLoop))
	copy(players, s.playerLoop)
	s.playersMu.RUnlock()

	for _, p := range players {
		p.processIn(s.currentTick)
	}
}

func (s *Server) processLogins() {
	s.playersMu.Lock()
	batch := s.newPlayers
	s.newPlayers = nil
	s.playersMu.Unlock()

	for _, p := range batch {
		if err := s.addPlayer(p); err != nil {
			// world full — reject cleanly
			p.writeOut(gameserver.OpLogout, nil)
			_ = p.client.flushWrite()
			_ = p.client.conn.Close()
			continue
		}
		p.lastConnected = s.currentTick
		p.lastResponse = s.currentTick
		p.originX = p.x
		p.originZ = p.z

		// sub-spec 3a: initialise buildarea, worn inventory, and appearance dirty flag
		p.buildArea = buildarea.New()
		p.invs = map[int]*inventory.Inventory{}
		if s.varpTypes != nil {
			p.varps = make([]int32, len(s.varpTypes.Configs))
		}
		if s.invTypes != nil && s.invTypes.Worn >= 0 && s.invTypes.Worn < len(s.invTypes.Configs) {
			wornType := s.invTypes.Configs[s.invTypes.Worn]
			if wornType != nil {
				worn := inventory.FromType(wornType)
				worn.Update = true
				p.invs[s.invTypes.Worn] = worn
			}
		}
		// Seed Hitpoints to 10 (RS2 default starting HP) before any code
		// reads p.levels[PlayerStatHitpoints]. Matches TS PlayerLoading.ts:49-51.
		// Full skill initialization (all 21 skills with persisted XP) is a
		// future sub-spec; S6e covers Hitpoints only because the persistent-HP
		// design requires it.
		p.baseLevels[objtype.PlayerStatHitpoints] = 10
		p.levels[objtype.PlayerStatHitpoints] = 10

		p.masks |= MaskAppearance

		// First-tick PlayerInfo must emit a teleport block so the client can
		// set localPlayer to a real scene-local position. Without this the
		// client's RebuildNormal adjustment drops localPlayer to negative
		// absolute X = -(sceneBaseTileX * 128) and crashes in getHeightmapY.
		p.tele = true
		p.jump = true

		// Fire the LOGIN trigger if the cache has one. Sub-spec RuneScript S3.
		if s.scriptProvider != nil {
			sf := s.scriptProvider.GetByTrigger(script.TriggerLogin, -1, -1)
			s.runScript(sf, p, true, nil, nil)
		}
	}
}

func (s *Server) processLogouts() {
	s.playersMu.RLock()
	players := make([]*Player, len(s.playerLoop))
	copy(players, s.playerLoop)
	s.playersMu.RUnlock()

	for _, p := range players {
		force := false
		if s.currentTick-p.lastResponse >= timeoutNoResponse {
			p.loggingOut = true
			force = true
		} else if s.currentTick-p.lastConnected >= timeoutNoConnection {
			p.requestIdleLogout = true
		}

		if p.requestLogout || p.requestIdleLogout {
			if s.currentTick >= p.preventLogoutUntil {
				p.loggingOut = true
			}
			p.requestLogout = false
			p.requestIdleLogout = false
		}

		if p.loggingOut && (force || s.currentTick >= p.preventLogoutUntil) {
			// Clear any suspended script so a late RESUME_* packet doesn't
			// reference a player that's logged out.
			p.activeScript = nil
			p.writeOut(gameserver.OpLogout, nil)
			_ = p.client.flushWrite()
			_ = p.client.conn.Close()
			s.removePlayer(p)
		}
	}
}

func (s *Server) processPathing() {
	s.playersMu.RLock()
	players := make([]*Player, len(s.playerLoop))
	copy(players, s.playerLoop)
	s.playersMu.RUnlock()

	for _, p := range players {
		p.resolveMovement()
	}
}

// processActiveScripts expires any elapsed delay, resumes suspended
// scripts, and fires ready queue entries. Runs between processClientsIn
// and processPathing so that a resumed or queued script that sets up
// movement has its movement applied this tick.
func (s *Server) processActiveScripts() {
	s.playersMu.RLock()
	players := make([]*Player, len(s.playerLoop))
	copy(players, s.playerLoop)
	s.playersMu.RUnlock()

	for _, p := range players {
		// (1) Expire delay.
		if p.delayed && s.currentTick >= p.delayedUntil {
			p.delayed = false
		}
		// (2) Resume suspended activeScript if delay has expired.
		if !p.delayed && p.activeScript != nil &&
			p.activeScript.Execution == script.Suspended {
			state := p.activeScript
			state.Execution = script.Running
			s.resumeOrFinish(state, p)
		}
		// (3) Process queue (fresh runs).
		s.processPlayerQueue(p)
	}
}

// processPlayerQueue walks the player's queue, decrementing delays and
// firing ready entries as fresh script runs. Iterates by index so an
// entry appended mid-pass (via a fired script calling EnqueueScript
// again) is visible in the same iteration — this preserves TS's
// authentic "speedup quirk" where queue-chain reactions cascade.
//
// Removal happens BEFORE firing so a re-entrant EnqueueScript doesn't
// collide with the index pointer.
func (s *Server) processPlayerQueue(p *Player) {
	i := 0
	for i < len(p.queue) {
		req := &p.queue[i]
		req.Delay--
		if req.Delay > 0 {
			i++
			continue
		}
		// STRONG queue fires even when delayed; others wait for idle.
		if p.delayed && req.Type != script.QueueStrong {
			i++
			continue
		}
		scriptID := req.ScriptID
		intArg := req.IntArg
		p.queue = append(p.queue[:i], p.queue[i+1:]...)

		if s.scriptProvider != nil {
			if sf := s.scriptProvider.GetByID(scriptID); sf != nil {
				s.runScript(sf, p, false, []int{intArg}, nil)
			}
		}
		// Don't advance i: we just removed the current element, so i
		// now points to what was the next element (or past end).
	}
}

// processPlayerTimers fires any ready timers. Soft timers fire even
// while p.delayed; normal timers wait for idle.
func (s *Server) processPlayerTimers() {
	s.playersMu.RLock()
	players := make([]*Player, len(s.playerLoop))
	copy(players, s.playerLoop)
	s.playersMu.RUnlock()

	for _, p := range players {
		if len(p.timers) == 0 {
			continue
		}
		// Deterministic fire order (maps are unordered).
		ids := make([]uint32, 0, len(p.timers))
		for id := range p.timers {
			ids = append(ids, id)
		}
		sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })

		for _, id := range ids {
			t, ok := p.timers[id]
			if !ok {
				continue
			}
			if s.currentTick < t.Clock+t.Interval {
				continue
			}
			if t.Type == script.TimerNormal && p.delayed {
				continue
			}
			t.Clock = s.currentTick
			if s.scriptProvider == nil {
				continue
			}
			sf := s.scriptProvider.GetByID(id)
			if sf == nil {
				continue
			}
			s.runScript(sf, p, false, []int{t.IntArg}, nil)
		}
	}
}

func (s *Server) processClientsOut() {
	s.playersMu.RLock()
	players := make([]*Player, len(s.playerLoop))
	copy(players, s.playerLoop)
	s.playersMu.RUnlock()

	for _, p := range players {
		p.processOut()
	}
}

func (s *Server) processInfo() {
	s.playersMu.RLock()
	players := make([]*Player, len(s.playerLoop))
	copy(players, s.playerLoop)
	s.playersMu.RUnlock()

	// Update grid when players cross zone boundaries.
	for _, p := range players {
		curZX, curZZ := p.x>>3, p.z>>3
		prevZX, prevZZ := p.lastTickX>>3, p.lastTickZ>>3
		if prevZX != curZX || prevZZ != curZZ || p.lastLevel != p.level {
			if p.lastTickX >= 0 {
				s.grid.Remove(p.slot, p.lastTickX, p.lastTickZ, p.lastLevel)
			}
			s.grid.Add(p.slot, p.x, p.z, p.level)
		}
	}

	// Regenerate appearance buffer for any player whose MaskAppearance is set
	// (set on login, and when equipment changes). Without this pass, the client
	// allocates a zero-length appearance buffer and throws
	// ArrayIndexOutOfBoundsException when parsing gender/headicons/body slots.
	if s.objTypes != nil && s.invTypes != nil {
		for _, p := range players {
			if p.masks&MaskAppearance != 0 {
				p.generateAppearance(s.objTypes, s.invTypes, s.currentTick)
			}
		}
	}

	sources := make([]rsbuf.PlayerSource, len(players))
	for i, p := range players {
		sources[i] = p
	}
	s.renderer.ComputePlayers(sources)

	npcSources := make([]rsbuf.NpcSource, len(s.npcLoop))
	for i, n := range s.npcLoop {
		npcSources[i] = n
	}
	s.renderer.ComputeNpcs(npcSources)
}

func (s *Server) processNpcs() {
	for _, n := range s.npcLoop {
		prevX, prevZ, prevLevel := n.x, n.z, n.level
		n.turn(s)
		if n.x != prevX || n.z != prevZ || n.level != prevLevel {
			s.grid.RemoveNpc(n.nid, prevX, prevZ, prevLevel)
			s.grid.AddNpc(n.nid, n.x, n.z, n.level)
		}
	}
}

func (s *Server) processZones() {
	for z := range s.zonesTracking {
		z.ComputeShared()
	}
}

func (s *Server) processInteractions() {
	s.playersMu.RLock()
	players := make([]*Player, len(s.playerLoop))
	copy(players, s.playerLoop)
	s.playersMu.RUnlock()
	for _, p := range players {
		p.processInteraction()
	}
}

func (s *Server) processCleanup() {
	s.playersMu.RLock()
	players := make([]*Player, len(s.playerLoop))
	copy(players, s.playerLoop)
	s.playersMu.RUnlock()
	for _, p := range players {
		p.ResetMasks()
	}
	for _, n := range s.npcLoop {
		n.ResetMasks()
	}
	for z := range s.zonesTracking {
		z.Reset()
	}
	clear(s.zonesTracking)
}
