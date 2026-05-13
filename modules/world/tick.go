package world

import (
	"sort"
	"time"

	entitypkg "github.com/zsrv/goscape/pkg/entity"
	"github.com/zsrv/goscape/pkg/inventory"
	gameserver "github.com/zsrv/goscape/pkg/io/protocol/game/server"
	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/rsbuf"
	"github.com/zsrv/goscape/pkg/script"
)

// defaultTickRate is the canonical tick interval. Mirrors TS
// World.TICKRATE (Engine-TS World.ts:120) = 600ms. The ::speed
// dev-block cheat (NAI-188) writes Server.tickRate to a different
// value at runtime.
const defaultTickRate = 600 * time.Millisecond

const (
	timeoutNoResponse   = 100 // ticks = 60s at 600ms
	timeoutNoConnection = 50  // ticks = 30s at 600ms
)

func (s *Server) runTickLoop() {
	s.runTickLoopWithRate(s.tickRate)
}

func (s *Server) runTickLoopWithRate(rate time.Duration) {
	nextTick := time.Now()
	for {
		// NAI-182 — shutdown consumer must run BEFORE any per-tick work
		// so a doomed conn doesn't receive one more tick of activity.
		// Mirrors TS World.cycle (World.ts:419-420 `if (this.shutdown)
		// this.processShutdown();`).
		if s.shutdownTick != -1 && s.currentTick >= s.shutdownTick {
			s.processShutdown()
			if s.shutdownGraceful {
				return // tick loop terminates; Server.Run() returns nil via s.gracefulExit
			}
		}

		start := time.Now()
		drift := start.Sub(nextTick)
		if drift < 0 {
			drift = 0
		}

		s.processClientsIn()
		s.processWorldQueue() // NAI-37: matches TS World.processWorld start-of-cycle ordering
		// NAI-122: processNpcEventQueue moved up to mirror TS World.ts:356
		// (drains BEFORE processPlayers at TS line 376). Closes the
		// V-PARTIAL where AI_SPAWN-populated npc varns
		// (%npc_combat_xp_multiplier and friends) were read as zero by
		// same-tick combat dispatch because the queue drained AFTER
		// processInteractions. DEVIATION-NAI-122-D3 declared in Bundle 0
		// findings: NAI-121 audit's "TS sync-inline" claim was a misread
		// — TS uses a unified queue identical to goscape's, just drained
		// earlier in the tick.
		s.processNpcEventQueue()
		s.processActiveScripts()
		// NAI-134: drain the obj-delayed-spawn queue. Mirrors TS
		// World.cycle ordering at World.ts:563 — runs after script-firing
		// (so same-tick INV_DROPITEM_DELAYED with delay=0 spawns the obj
		// before processNpcs / processInfo reads zone state).
		s.processObjDelayedQueue()
		s.processPlayerTimers()
		// NAI-144: TS World.ts:725 — engineQueue drains between timers and
		// movement. processPlayerEngineQueues mirrors TS
		// Player.processEngineQueue per-player drain semantics.
		s.processPlayerEngineQueues()
		s.processPathing()
		s.processInteractions()
		s.processEnergy() // NAI-135: TS World.ts:731 per-player updateEnergy
		s.processNpcs()
		s.processLogouts()
		s.processLogins()
		s.processInfo()
		s.processZones() // compute ComputeShared before delivery
		s.processClientsOut()
		s.processCleanup()
		s.processSessionLogs() // NAI-74: TS World.cycle session-log block (W.ts:428-442)
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
		func(p *Player) {
			defer recoverPlayer(p, "processIn", s.log)
			p.processIn(s.currentTick)
		}(p)
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

		// sub-spec 3a: initialise worn inventory, and appearance dirty flag.
		// (Scenery-window state is initialised in newPlayer as flat fields
		// since NAI-30 Bundle 4.)
		p.invs = map[int]*inventory.Inventory{}
		s.initPlayerVarps(p)
		if s.invTypes != nil && s.invTypes.Worn >= 0 && s.invTypes.Worn < len(s.invTypes.Configs) {
			wornType := s.invTypes.Configs[s.invTypes.Worn]
			if wornType != nil {
				worn := inventory.FromType(wornType)
				worn.Update = true
				p.invs[s.invTypes.Worn] = worn
			}
		}
		// Default-player skill init — 21 skills at level 1 with 0 XP, then
		// Hitpoints overridden to level 10 with the matching XP. Matches TS
		// PlayerLoading.ts:41-53 (the "no save data" branch). Save-file load
		// + restore is a future sub-spec; this default becomes the no-save
		// fallback when that lands.
		for i := range objtype.PlayerStatCount {
			p.stats[i] = 0
			p.baseLevels[i] = 1
			p.levels[i] = 1
		}
		p.stats[objtype.PlayerStatHitpoints] = int32(objtype.GetExpByLevel(10))
		p.baseLevels[objtype.PlayerStatHitpoints] = 10
		p.levels[objtype.PlayerStatHitpoints] = 10

		p.masks |= MaskAppearance

		// First-tick PlayerInfo must emit a teleport block so the client can
		// set localPlayer to a real scene-local position. Without this the
		// client's RebuildNormal adjustment drops localPlayer to negative
		// absolute X = -(sceneBaseTileX * 128) and crashes in getHeightmapY.
		p.tele = true
		p.jump = true

		// NAI-182 — reconnect branches to onReconnect's TS-faithful
		// resync path; fresh login runs the standard onLogin emit
		// sequence. p.reconnecting is set by the login codec
		// (server.go:650) based on OpReqGameReconnect.
		if p.reconnecting {
			onReconnect(s, p)
			// rebuildNormal will clear p.reconnecting later in processInfo.
		} else {
			// Fresh-login emit sequence per TS Player.onLogin
			// (Player.ts:494-504). DEVIATION-NAI-182-D4 omits IF_CLOSE,
			// DEVIATION-NAI-182-D5 omits ChatFilterSettings /
			// UpdateIgnoreList (deferred social cluster).
			sendUpdatePid(p, p.slot)
			sendResetClientVarCache(p)
			if s.varpTypes != nil {
				for i, vt := range s.varpTypes.Configs {
					if vt != nil && vt.Transmit {
						p.writeVarp(i, p.varps[i])
					}
				}
			}
			sendResetAnims(p)

			// Post-onLogin UPDATE_REBOOT_TIMER emit if shutdown pending.
			// Mirrors TS World.processLogins (World.ts:944-946).
			if s.shutdownTick != -1 {
				sendUpdateRebootTimer(p, s.shutdownTick-s.currentTick)
			}
		}

		// Fire the LOGIN trigger if the cache has one. Sub-spec RuneScript S3.
		if s.scriptProvider != nil {
			sf := s.scriptProvider.GetByTrigger(script.TriggerLogin, -1, -1)
			s.runScript(sf, p, nil, true, nil, nil)
		}

		// TS Player.ts:511-512 — establish the "imaginary previous step
		// from the west" so followX/Z reads a valid coord before the
		// player takes their first step. Mirrors the teleport-time
		// init at player_script.go:565-566. Required for player-follow:
		// a follower's pathToPathingTarget arm at interaction.go:802-809
		// reads the leader's followX/Z (= leader.lastStepX/Z, refreshed
		// by NAI-174 T1's unconditional top writes). Without this init,
		// a stationary post-login leader has followX/Z = (-1, -1) and
		// followers queue queueWaypoint(-1, -1) → SW partial-path stall
		// (NAI-174 Bug 1 — half of NAI-173-FU-FOLLOW-MODE-INVESTIGATION).
		p.lastStepX = p.x - 1
		p.lastStepZ = p.z

		// NAI-73: allocate the InputTracking state machine. Defaults
		// session to "headless" until LOGIN-SERVER-BRIDGE-MOD ships a
		// real UUID assignment.
		p.input = NewInputTracking(p, s.currentTick)
		if p.session == "" {
			p.session = "headless"
		}
	}
}

// initPlayerVarps allocates p.varps + p.varpsString to len(s.varpTypes.Configs)
// and per-type-seeds each slot per TS Player.ts:418-432:
//
//	STRING → varpsString[i] = "" (Go zero-value)
//	INT    → varps[i] = 0        (Go zero-value)
//	else   → varps[i] = -1
//
// Defensive (DEVIATION-NAI-121-D3): nil s.varpTypes → no-op (test paths).
// (goscape defensive; TS skips this check.)
func (s *Server) initPlayerVarps(p *Player) {
	if s.varpTypes == nil {
		return
	}
	p.varps = make([]int32, len(s.varpTypes.Configs))
	p.varpsString = make([]string, len(s.varpTypes.Configs))
	for i, vt := range s.varpTypes.Configs {
		switch vt.Type {
		case objtype.ScriptVarTypeString:
			// varpsString[i] = "" already (Go zero-value)
		case objtype.ScriptVarTypeInt:
			// varps[i] = 0 already (Go zero-value)
		default:
			p.varps[i] = -1
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
		if p.forceRemove {
			force = true
		}
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

			// NAI-30 Bundle 4: per-Buf observer decrement for every NPC this
			// player tracked is performed inside s.removePlayer →
			// s.rsbuf.RemovePlayer (server.go), mirroring upstream rsbuf
			// remove_player(pid) at lib.rs:186-203. The legacy package-level
			// rsbuf.RemovePlayer + p.buildArea.Npcs producer was retired here.
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

// processEnergy drives one tick of per-player run-energy
// drain/recovery + run-mode auto-disable. Mirrors TS World.ts:731
// (player.updateEnergy() per-player iteration). NAI-135.
func (s *Server) processEnergy() {
	s.playersMu.RLock()
	players := make([]*Player, len(s.playerLoop))
	copy(players, s.playerLoop)
	s.playersMu.RUnlock()

	for _, p := range players {
		p.updateEnergy()
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
		func(p *Player) {
			defer recoverPlayer(p, "processActiveScripts", s.log)
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
		}(p)
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
	// TS Player.processQueues (Player.ts:854-865): any STRONG-queue item
	// closes the modal before queues run; then consume the deferred flag
	// (also set by handleCloseModal for the CLOSE_MODAL client packet).
	for _, req := range p.queue {
		if req.Type == script.QueueStrong {
			p.requestModalClose = true
			break
		}
	}
	if p.requestModalClose {
		p.requestModalClose = false
		p.CloseModal(true)
	}
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
		sf := req.Script
		intArgs := req.IntArgs
		stringArgs := req.StringArgs
		p.queue = append(p.queue[:i], p.queue[i+1:]...)
		if sf != nil {
			// TS Player.ts:891,903 — processQueues + processWeakQueue
			// both fire scripts as protected (executeScript(script, true)).
			s.runScript(sf, p, nil, true, intArgs, stringArgs)
		}
		// Don't advance i: we just removed the current element, so i
		// now points to what was the next element (or past end).
	}
}

// processPlayerEngineQueues drains each player's engineQueue (NAI-144).
// Mirrors TS Player.processEngineQueue (Engine-TS/.../Player.ts:641-651):
// per entry, decrement Delay; if CanAccess() && Delay <= 0, fire (as a
// protected script) and remove. Iteration is index-based and re-evaluates
// len(p.engineQueue) each pass so a script that re-enqueues during fire
// (TS LinkList chain semantics) is visible same-tick (T6 pin).
//
// Distinct from processPlayerQueue: no QueueStrong modal-close pre-pass;
// gated by CanAccess() not by req.Type==QueueStrong; no STRONG-style
// preemption.
//
// DEVIATION-NAI-144-D4: TS canAccess() (Player.ts:805-812) returns true
// unconditionally when World.shutdown=true; goscape has no equivalent
// shutdown flag and omits this branch. All other gates (delayed, modal,
// protect-equivalent) are functionally identical between TS and goscape's
// CanAccess() — see player_script.go:308-322 for the full mapping.
//
// Tick-loop slot: between processPlayerTimers and processPathing,
// matching TS World.ts:725.
func (s *Server) processPlayerEngineQueues() {
	s.playersMu.RLock()
	players := make([]*Player, len(s.playerLoop))
	copy(players, s.playerLoop)
	s.playersMu.RUnlock()

	for _, p := range players {
		func(p *Player) {
			defer recoverPlayer(p, "processPlayerEngineQueues", s.log)
			i := 0
			for i < len(p.engineQueue) {
				req := &p.engineQueue[i]
				req.Delay--
				if req.Delay > 0 || !p.CanAccess() {
					i++
					continue
				}
				sf := req.Script
				intArgs := req.IntArgs
				stringArgs := req.StringArgs
				p.engineQueue = append(p.engineQueue[:i], p.engineQueue[i+1:]...)
				if sf != nil {
					// TS Player.ts:646 — executeScript(script, true): protected.
					s.runScript(sf, p, nil, true, intArgs, stringArgs)
				}
				// Don't advance i — the slice shrunk by one; index now points
				// at what was the next entry (or past end).
			}
		}(p)
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
		func(p *Player) {
			defer recoverPlayer(p, "processPlayerTimers", s.log)
			if len(p.timers) == 0 {
				return
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
				// TS Player.ts:938: NORMAL timers run protected, SOFT
				// timers do not.
				s.runScript(sf, p, nil, t.Type == script.TimerNormal, t.IntArgs, t.StringArgs)
			}
		}(p)
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

	// NAI-66: TS World.ts:995 — per-tick refocus before rsbuf compute.
	// Refocuses on a moved PathingEntity target or clears the cached
	// Loc/Obj targetX/Z when the player took zero steps this tick.
	//
	// NAI-93: TS World.ts:996 — buildArea.rebuildNormal() runs in this
	// loop, BEFORE the ComputePlayers/ComputePlayer calls below, so the
	// rsbuf-cached Origin matches the just-emitted RebuildNormal packet's
	// zoneX/zoneZ. Inverting this order produces stale-origin tele leaves
	// on cross-window teles → Java client AIOOBE in getHeightmapY/getTopLevel.
	// TS comment at World.ts:996 verbatim: "set origin before compute
	// player is why this is above."
	for _, p := range players {
		p.reorient()
		p.rebuildNormal()
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

	// NAI-66: TS World.ts:1046 — npc-side per-tick refocus.
	for _, n := range s.npcLoop {
		n.reorient()
	}

	npcSources := make([]rsbuf.NpcSource, len(s.npcLoop))
	for i, n := range s.npcLoop {
		npcSources[i] = n
	}
	s.renderer.ComputeNpcs(npcSources)

	// NAI-29 Bundle 4 Task 4.4 — parallel-write to rsbuf state. Existing
	// Encode/EncodeNpc path doesn't yet read this; NAI-30 does the
	// read-flip. Pushed AFTER all per-player movement + appearance regen
	// is finalized so coord/originX/originZ/appearanceBuf are stable.
	if s.rsbuf != nil {
		for _, p := range players {
			if p == nil {
				continue
			}
			var sayPtr *string
			if len(p.sayText) > 0 {
				ss := string(p.sayText)
				sayPtr = &ss
			}
			s.rsbuf.ComputePlayer(int32(p.slot),
				p.x, p.level, p.z,
				p.originX, p.originZ,
				p.tele, p.jump,
				int8(p.runDir), int8(p.walkDir),
				p.visibility,
				p.staffModLevel,
				p.active,
				uint32(p.masks),
				p.appearanceBuf,
				int32(p.lastAppearance),
				int32(p.faceEntity),
				int32(p.faceSquareX), int32(p.faceSquareZ),
				int32(p.OrientationX), int32(p.OrientationZ),
				int32(p.damageAmt), int32(p.damageType),
				int32(p.CurHP()), int32(p.BaseHP()),
				int32(p.animID), int32(p.animDelay),
				sayPtr,
				p.chatBytes, uint8(p.chatColour), uint8(p.chatEffect), uint8(p.chatRights),
				int32(p.spotanimID), int32(p.spotanimHeight), int32(p.spotanimDelay),
				int32(p.exactStartX), int32(p.exactStartZ),
				int32(p.exactEndX), int32(p.exactEndZ),
				int32(p.exactBegin), int32(p.exactFinish), int32(p.exactDir),
			)
		}

		// NAI-29 Bundle 4 Task 4.5 — parallel-write npc state push.
		// Iterates s.npcLoop (active list, parallel to T4.4's per-player
		// iteration over playerLoop); skips slots where n is nil or
		// n.dead is true (goscape "dead-bool divergence" — dead npcs
		// remain in npcLoop until the existing dead-cleanup pass prunes
		// them; we don't push their state to rsbuf).
		for _, n := range s.npcLoop {
			if n == nil || n.dead {
				continue
			}
			var sayPtr *string
			if len(n.SayText()) > 0 {
				ss := string(n.SayText())
				sayPtr = &ss
			}
			s.rsbuf.ComputeNpc(int32(n.nid), int32(n.typeId),
				n.x, n.level, n.z,
				n.tele,
				int8(n.runDir), int8(n.walkDir),
				!n.dead, // active = !dead
				uint32(n.Masks()),
				int32(n.FaceEntity()),
				int32(n.FaceSquareX()), int32(n.FaceSquareZ()),
				int32(n.OrientationX), int32(n.OrientationZ),
				int32(n.DamageAmt()), int32(n.DamageType()),
				int32(n.CurHP()), int32(n.BaseHP()),
				int32(n.AnimID()), int32(n.AnimDelay()),
				sayPtr,
				int32(n.SpotAnimID()), int32(n.SpotAnimHeight()), int32(n.SpotAnimDelay()),
			)
		}
	}
}

func (s *Server) processNpcs() {
	for _, n := range s.npcLoop {
		n.turn(s)
	}
}

// processZones drives per-tick lifecycle transitions for tracked
// NonPathing entities (Loc / future Obj) and computes the shared
// Enclosed-event buffer for every tracked zone. Mirrors TS
// World.processZones (Engine-TS/.../World.ts:961-986).
//
// Snapshots the tracker before iterating because each turnLoc may
// mutate the tracker (RemoveLoc / RevertLoc both call SetLifeCycle(-1)
// → Unregister) and we cannot iterate a list that's being unlinked.
func (s *Server) processZones() {
	if s.locObjTracker != nil {
		// Snapshot to a slice — the tracker uses a linked list whose
		// iteration is invalidated by mid-iteration Unlink. The bare
		// type-assert (no comma-ok) panics if the field ever holds
		// something other than *locObjTracker, surfacing the bug
		// loudly rather than silently dropping all per-tick processing.
		t := s.locObjTracker.(*locObjTracker)
		snap := make([]*entitypkg.NonPathing, 0, t.list.Size())
		for np := range t.All() {
			snap = append(snap, np)
		}
		for _, np := range snap {
			switch p := np.Parent().(type) {
			case *entitypkg.Loc:
				s.turnLoc(p, s.currentTick)
			case *entitypkg.Obj:
				s.turnObj(p, s.currentTick)
			}
		}
	}
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
		func(p *Player) {
			defer recoverPlayer(p, "processInteraction", s.log)
			p.processInteraction()
		}(p)
	}
}

func (s *Server) processCleanup() {
	s.playersMu.RLock()
	players := make([]*Player, len(s.playerLoop))
	copy(players, s.playerLoop)
	s.playersMu.RUnlock()
	for _, p := range players {
		p.ResetMasks()
		// NAI-72/108 — TS Player.resetEntity(false) at Player.ts:454-467.
		// Reset social/report spam-protect flags so the next tick admits
		// at most one social/report packet per type per player.
		//
		// Deferred resetEntity fields now resolved by NAI-108:
		//   - protect → activeScript.Pointers&PtrProtectedActivePlayer
		//     (already-converged divergence; see interaction.go:308,
		//     player_script.go:276,297-300).
		//   - chatColour/Effect/Rights → moved to ResetMasks per TS fidelity.
		//   - chat msg field → dead field deleted (player.go:196 retired).
		//   - logMessage → TS-only, no goscape consumer (YAGNI).
		// unfocus() remains deferred per NAI-67-D-PLAYER-UNFOCUS-DEFERRED
		// (Player respawn/death sub-spec).
		p.socialProtect = false
		p.reportAbuseProtect = false
	}
	for _, n := range s.npcLoop {
		n.resetPathingEntity()
		n.ResetMasks()
	}
	for z := range s.zonesTracking {
		z.Reset()
	}
	clear(s.zonesTracking)
	// NAI-19: prune DESPAWN-lifecycle dead NPCs from s.npcLoop at
	// end-of-tick. Runs AFTER processInfo's NpcInfo writes (which
	// fire upstream in processTick) so the just-despawned NPC's
	// removal mask is already in the client stream via rsbuf.RemoveNpc.
	s.compactNpcLoop()

	// NAI-29 Bundle 4 Task 4.6 — clear transient rsbuf state at end of
	// tick. Mirrors upstream cleanup at lib.rs:348-363: clears playerGrid
	// (rebuilt fresh each tick from ComputePlayer pushes) and calls
	// Player.cleanup() / Npc.cleanup() on every populated slot to zero
	// transient per-tick fields while preserving persistent ones
	// (Appearance, FaceEntity, OrientationX/Z, Observers).
	if s.rsbuf != nil {
		s.rsbuf.Cleanup()
	}
}

// compactNpcLoop prunes DESPAWN-lifecycle dead NPCs from s.npcLoop.
// Called once per tick from processCleanup AFTER NpcInfo writes have
// completed — the just-despawned NPC's removal mask is already in the
// client write stream via rsbuf.RemoveNpc (called from removeNpc).
// RESPAWN-lifecycle dead NPCs are preserved; their dead=true flips on
// the next lifecycleTick==0 in npc_ai.go's processNpcLifecycle.
//
// Mirrors TS's per-zone linked-list splice in World.removeNpc, which
// goscape can't do safely mid-iteration (s.npcLoop is an append-only
// slice). End-of-tick mark/compact is observably identical at tick
// boundaries.
// Tracked deviation: NAI-19-D-DEFERRED-COMPACT-VS-IMMEDIATE-SPLICE.
func (s *Server) compactNpcLoop() {
	write := 0
	for _, n := range s.npcLoop {
		if n.dead && n.lifecycle == NpcLifecycleDespawn {
			continue
		}
		s.npcLoop[write] = n
		write++
	}
	for i := write; i < len(s.npcLoop); i++ {
		s.npcLoop[i] = nil // GC hint: drop pointer retention
	}
	s.npcLoop = s.npcLoop[:write]
}
