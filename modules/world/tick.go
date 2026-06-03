package world

import (
	"context"
	"log/slog"
	"sort"
	"time"

	entitypkg "github.com/zsrv/goscape/pkg/entity"
	"github.com/zsrv/goscape/pkg/friendspb"
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

// invStockRate is the decay interval for unlisted shop stock (general
// stores). Mirrors TS World.INV_STOCKRATE (World.ts:122) = 100 ticks (~1m).
const invStockRate = 100

func (s *Server) runTickLoop() {
	s.runTickLoopWithRate(s.tickRate)
}

func (s *Server) runTickLoopWithRate(rate time.Duration) {
	// Seed s.tickRate from the parameter so test-injected rates are
	// observable via the field. The loop body below re-reads s.tickRate
	// each iteration so ::speed mutations (NAI-188) take effect on the
	// next sleep computation. Per spec §6: writer (cheat dispatch) and
	// reader (this loop) both run on the tick goroutine, so no lock.
	s.tickRate = rate
	nextTick := time.Now()
	for {
		// NAI-REBUILD-ASYNC — drain at top-of-body so Reload runs before
		// any per-tick work observes mid-swap state. Mirrors
		// processShutdown's top-of-body placement.
		select {
		case r := <-s.rebuildResult:
			s.handleRebuildResult(r)
		default:
		}

		// Slice 5b: drain inbound RELAY_* actions enqueued by the world
		// events dispatcher BEFORE processShutdown so a same-tick
		// RELAY_SHUTDOWN observes its own shutdownTick assignment.
		s.drainRelayActions()

		// NAI-182 — shutdown consumer runs at top-of-body so a doomed conn
		// doesn't receive one more tick of activity. DEVIATION from TS (L3):
		// TS runs processShutdown at cycle END (World.ts:419-420, after
		// processCleanup), goscape hoists it to the top. Accepted LOW: the
		// only observable effect is a shutting-down world cuts activity one
		// tick earlier than TS. The TS citation is the call shape
		// (`if (this.shutdown) this.processShutdown();`), not its position.
		if s.shutdownTick != -1 && s.currentTick >= s.shutdownTick {
			s.processShutdown()
			if s.shutdownGraceful {
				return // tick loop terminates; Server.Run() returns nil via s.gracefulExit
			}
		}

		// NAI-PLAYERLOADING — autosave every PlayerSaveRate (1500) ticks.
		// Gate at top-of-body so currentTick has been incremented by the
		// previous iteration's tail. currentTick==0 on the very first
		// iteration is excluded by the >0 guard. DEVIATION from TS (L4): TS
		// runs savePlayers at cycle END (World.ts:424); goscape at top. Benign
		// — identical 1500-tick cadence, and a tick-boundary save sees a
		// consistent snapshot whether taken at the start or end of the tick.
		if s.currentTick%PlayerSaveRate == 0 && s.currentTick > 0 {
			s.autosavePlayers()
			// PlayerAutosave RPC + cadence gate pinned by TestAutosavePlayers_*
			// (server_autosave_test.go).
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
		// NAI-217: processNpcs moved up to mirror TS World.cycle order
		// (Engine-TS/src/engine/World.ts:365 processNpcs → :376
		// processPlayers). Player-side processInteraction at the post-
		// pathing call below must see this-tick NPC positions (after
		// the NPC moved THIS cycle), not the stale end-of-previous-tick
		// positions that resulted when processNpcs ran later. Pre-NAI-217
		// symptom: when the player chases a wandering NPC,
		// inOperableDistance measures against the NPC's last-tick-end
		// position, so branch-1 OP fire skips even though the NPC will
		// end this tick visually adjacent. processNpcs internally drives
		// NPC ai_spawn resume, stat regen, timer, queue, movement, and
		// modes — all of which must settle before the per-player block
		// reads NPC state.
		//
		// World-level player-hunt pass (TS World.processWorld at
		// World.ts:577-589) runs immediately before processNpcs so the
		// huntTarget it sets on aggressive (HuntModePlayer) NPCs is consumed
		// into an interaction by the same tick's consumeHuntTarget inside
		// turn() — matching TS processWorld → processNpcs ordering. This is
		// what makes aggressive NPCs initiate combat instead of only reacting
		// when attacked.
		s.processNpcHuntPlayers()
		s.processNpcs()
		s.processActiveScripts()
		// NAI-134: drain the obj-delayed-spawn queue here, after script-firing,
		// so a same-tick INV_DROPITEM_DELAYED with delay=0 spawns the obj before
		// processInfo reads zone state. DEVIATION from TS (L1): TS drains the
		// objDelayedQueue inside processWorld at cycle START (World.ts:562-574),
		// BEFORE the same cycle's processNpcs (L365), so a delayed obj is visible
		// to NPC hunt the SAME tick. goscape drains it AFTER processNpcs, so a
		// delayed-spawned obj is visible to NPC obj-hunt one tick later. Accepted
		// LOW: 1-tick latency on the rare HuntModeObj path; the placement is what
		// gives delay=0 player drops same-tick visibility before processInfo.
		s.processObjDelayedQueue()
		s.processPlayerTimers()
		// NAI-144: TS World.ts:725 — engineQueue drains between timers and
		// movement. processPlayerEngineQueues mirrors TS
		// Player.processEngineQueue per-player drain semantics.
		s.processPlayerEngineQueues()
		// TS Player.processInteraction interleaves updateMovement between its
		// pre-step and post-step interact arms (Player.ts:1241). goscape splits
		// that around the movement pass: the pre-step interact (+ path recompute)
		// runs at the player's PRE-movement position here, then processPathing
		// moves, then processInteractions runs the post-step arm + tail. This is
		// what lets a player who clicks an in-range NPC attack from where they
		// stand instead of stepping to contact first.
		s.processInteractionsPreMove()
		s.processPathing()
		s.processInteractions()
		s.processEnergy() // NAI-135: TS World.ts:731 per-player updateEnergy
		// M3: TS World.ts:733-735 — jump-snap any player who moved >2 tiles
		// this tick (gated by EXACT_MOVE). Runs after movement+energy, before
		// processInfo serializes the jump bit.
		s.processValidateDistanceWalked()
		s.processLogouts()
		s.processLogins()
		// L2 DEVIATION (accepted, documented NAI-93): TS runs processZones
		// (W.ts:388) BEFORE processInfo (W.ts:395); goscape runs processInfo
		// first so rebuildNormal (TS BuildArea slot, W.ts:996) settles before
		// zone compute. Cost is a 1-tick facing artifact for a just-revealed
		// zone — see the NAI-93 notes in player.go / processInfo below.
		s.processInfo()
		s.processZones() // compute ComputeShared before delivery
		s.processClientsOut()
		s.processCleanup()
		s.processSessionLogs() // NAI-74: TS World.cycle session-log block (W.ts:428-442)
		s.currentTick++

		// NAI-188: re-read s.tickRate every iteration so ::speed
		// mutations take effect on the next sleep. Named currentRate
		// (not rate) to avoid shadowing the parameter; per memory
		// plan_var_name_collision.
		currentRate := s.tickRate
		nextTick = nextTick.Add(currentRate)
		delay := currentRate - time.Since(start) - drift
		if delay < 0 {
			delay = 0
		}

		select {
		case <-s.quit:
			// Shutdown is tearing down: save+remove every still-online player
			// on-tick (race-free) before the loop exits. Shutdown waits on
			// saveWg afterwards so these saves flush. Mirrors TS, which logs
			// every player out (with save) on shutdown.
			s.saveAllOnShutdown()
			return
		case <-time.After(delay):
		}
	}
}

// snapshotPlayers returns a stable copy of s.playerLoop for one tick pass
// to iterate: passes like processLogouts splice players out of the live
// slice mid-iteration, so ranging playerLoop directly would skip entries.
//
// The copy lands in s.playerScratch, reused across passes — pre-PERF-1
// each of the 13 passes allocated a fresh slice, ~13 allocs/tick scaling
// with player count. Tick-goroutine-only. The returned slice is valid
// until the next snapshotPlayers call; callers must not retain it across
// passes.
func (s *Server) snapshotPlayers() []*Player {
	s.playersMu.RLock()
	prev := len(s.playerScratch)
	s.playerScratch = append(s.playerScratch[:0], s.playerLoop...)
	s.playersMu.RUnlock()
	// Nil any tail left over from a previous, larger snapshot so departed
	// players aren't pinned by the scratch's spare capacity. Invariant:
	// playerScratch[len:cap] is all-nil between calls.
	if n := len(s.playerScratch); prev > n {
		clear(s.playerScratch[n:prev])
	}
	return s.playerScratch
}

func (s *Server) processClientsIn() {
	players := s.snapshotPlayers()

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

		// NAI-S2-D-PLAYERLOGIN-AT-PROCESSLOGINS: register the player on
		// the friends server when they enter the world. Mirrors TS's
		// PLAYER_LOGIN-on-world-entry semantics. On cap-rejection the
		// world logs warn and continues — TS-faithful: the friends-
		// server is silent toward the world on rejection (TS
		// FriendServer.ts:128-132 early-returns without notifying).
		if s.friendsClient != nil && p.username != "" {
			username37 := p.username37
			worldID := int32(s.cfg.NodeID)
			privateChat := int32(p.privateChat)
			staffLvl := p.staffModLevel
			// Arc 18 R3 — per-call timeout + shutdown-derived parent so a
			// hung friends-server cannot pile up goroutines.
			go func() {
				ctx, cancel := context.WithTimeout(s.bridgesCtx, bridgeCallTimeout)
				defer cancel()
				s.friendsClient.PlayerLogin(ctx, &friendspb.PlayerLoginRequest{
					WorldId:     worldID,
					Username37:  username37,
					PrivateChat: privateChat,
					StaffLvl:    staffLvl,
				}, func(accepted bool) {
					if !accepted {
						s.log.Warn("friends-server rejected player login (cap reached or RPC error)",
							slog.Int("world_id", int(worldID)),
							slog.Uint64("username37", username37),
						)
					}
				})
			}()
		}

		// NAI-S4A: start the SubscribeUpdates stream subscriber.
		// Lives until logout/disconnect cancels p.friendsSubCancel.
		if s.friendsClient != nil && p.username != "" {
			subCtx, subCancel := context.WithCancel(context.Background())
			p.friendsSubCancel = subCancel
			p.friendsSub = newFriendsSubscriber(s.friendsClient, int32(s.cfg.NodeID), p.username37, s.friendsDispatcher, s.log)
			go p.friendsSub.run(subCtx)
		}

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
		// Player state load. Delegates to LoadSave which handles both
		// populated-save (full decode) and empty-save bootstrap paths.
		// On decode error, log + fall back to empty bootstrap so a
		// corrupt SAV doesn't deny login. Deviation
		// NAI-PLAYERLOADING-D-DECODE-ERR-FALLS-BACK-TO-BOOTSTRAP.
		if err := LoadSave(p, p.client.savePayload, s.invTypes, s.varpTypes); err != nil {
			s.log.Warn("LoadSave failed; falling back to empty bootstrap",
				slog.String("username", p.username), slog.Any("err", err))
			_ = LoadSave(p, nil, s.invTypes, s.varpTypes)
		}
		// LoadSave branches covered end-to-end by TestProcessLogins_*
		// (player_load_integration_test.go) and the RPC site by
		// TestCallPlayerLoginRPC_* (login_client_test.go).

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
			// (Player.ts:486-504). DEVIATION-NAI-182-D4 omits IF_CLOSE.
			// UpdateIgnoreList([]) defensive emit is permanently skipped
			// (DEVIATION-NAI-182-D5-NO-DEFENSIVE-IGNORELIST-LOGIN-EMIT —
			// goscape always runs with a friends server).
			sendChatFilterSettings(p, p.publicChat, p.privateChat, p.tradeDuel)
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
			s.runScriptFn(sf, p, nil, script.TriggerLogin, true, nil, nil)
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

		// NAI-73: allocate the InputTracking state machine.
		// session is normally assigned in newPlayer() from the
		// PlayerLoginResponse.session_uuid; the "headless" fallback
		// below covers standalone-world and unit-test paths that bypass
		// the login bridge.
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
	players := s.snapshotPlayers()

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
			} else if p.requestLogout && p.preventLogoutMessage != "" {
				// L7: within the prevent-logout window, surface the
				// P_PREVENTLOGOUT message to the player and consume it.
				// Mirrors TS World.processLogouts (World.ts:765-767). Idle
				// logout requests do not trigger the message (TS gates on
				// requestLogout only).
				p.MessageGame(p.preventLogoutMessage)
				p.preventLogoutMessage = ""
			}
			p.requestLogout = false
			p.requestIdleLogout = false
		}

		if p.loggingOut && (force || s.currentTick >= p.preventLogoutUntil) {
			// Mirrors TS World.processLogouts (World.ts:773-800).
			// Close any open modal first (also clears the weak queue).
			p.CloseModal(true)

			// The primary queue must be fully discardable before logout: every
			// entry a LONG marked logoutAction==1 (discard). Any other entry
			// blocks logout this tick. TS World.ts:776-787.
			queueDiscardable := true
			for i := range p.queue {
				req := &p.queue[i]
				if req.Type == script.QueueLong && len(req.IntArgs) > 0 && req.IntArgs[0] == 1 {
					continue
				}
				queueDiscardable = false
				break
			}

			// Only log out when the player can run a script, has no pending
			// engine-queue work, and the queue is discardable. TS World.ts:788.
			if !p.CanAccess() || len(p.engineQueue) > 0 || !queueDiscardable {
				continue
			}

			// Fire the LOGOUT trigger as a protected script before teardown
			// (TS World.ts:789-797). DEVIATION: TS skips removal entirely when
			// no [logout] script is registered ('LOGOUT TRIGGER IS BROKEN!');
			// goscape instead logs and proceeds with removal so a
			// misconfigured/script-less world (and most test fixtures) can't
			// leak players that never log out. Production ships logout.rs2.
			var logoutScript *script.ScriptFile
			if s.scriptProvider != nil {
				logoutScript = s.scriptProvider.GetByTriggerSpecific(script.TriggerLogout, -1, -1)
			}
			if logoutScript != nil {
				s.runScript(logoutScript, p, nil, script.TriggerLogout, true, nil, nil)
			} else {
				s.log.Warn("no [logout] trigger registered; removing player without it",
					"player", p.username)
			}

			// Clear any suspended script so a late RESUME_* packet doesn't
			// reference a player that's logged out.
			p.activeScript = nil
			p.writeOut(gameserver.OpLogout, nil)
			_ = p.client.flushWrite()
			_ = p.client.conn.Close()

			// NAI-30 Bundle 4: per-Buf observer decrement for every NPC this
			// player tracked is performed inside removePlayerInternal →
			// s.rsbuf.RemovePlayer (server.go), mirroring upstream rsbuf
			// remove_player(pid) at lib.rs:186-203. The legacy package-level
			// rsbuf.RemovePlayer + p.buildArea.Npcs producer was retired here.
			s.removePlayerOnTick(p)
		}
	}
}

func (s *Server) processPathing() {
	players := s.snapshotPlayers()

	for _, p := range players {
		p.resolveMovement()
	}
}

// processEnergy drives one tick of per-player run-energy
// drain/recovery + run-mode auto-disable. Mirrors TS World.ts:731
// (player.updateEnergy() per-player iteration). NAI-135.
func (s *Server) processEnergy() {
	players := s.snapshotPlayers()

	for _, p := range players {
		p.updateEnergy()
	}
}

// processValidateDistanceWalked forces a jump on any player whose net movement
// this tick exceeded 2 tiles, unless an EXACT_MOVE mask is already driving the
// displacement. Mirrors TS World.ts:733-735 (`if ((player.masks & EXACT_MOVE)
// == 0) player.validateDistanceWalked()`), which runs immediately after
// updateEnergy in the same player loop. Placed right after processEnergy and
// before processInfo so the jump bit is set before the renderer reads it and
// reset in processCleanup. M3.
func (s *Server) processValidateDistanceWalked() {
	players := s.snapshotPlayers()

	for _, p := range players {
		if p.masks&MaskExactMove == 0 {
			p.validateDistanceWalked()
		}
	}
}

// processActiveScripts expires any elapsed delay, resumes suspended
// scripts, and fires ready queue entries. Runs between processClientsIn
// and processPathing so that a resumed or queued script that sets up
// movement has its movement applied this tick.
func (s *Server) processActiveScripts() {
	players := s.snapshotPlayers()

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
// firing ready entries as fresh script runs.
//
// PLAYER-SCRIPT-2 closure: TS Player.processQueues (Player.ts:854-869)
// delegates to processQueue() (walks `this.queue`, the non-weak LinkList)
// and then processWeakQueue() (walks the separate `this.weakQueue`),
// so all non-weak entries (NORMAL/STRONG/LONG) fire BEFORE any weak
// entries on the same tick, regardless of insertion order. goscape
// stores both kinds in a single p.queue slice (discriminated by
// req.Type==QueueWeak); reproducing the TS ordering requires two
// filtered passes — non-weak first, then weak. Each pass decrements
// every matching entry's Delay exactly once per tick (matching the
// original single-pass per-entry semantics).
func (s *Server) processPlayerQueue(p *Player) {
	// TS Player.processQueues (Player.ts:854-865): any STRONG-queue item
	// closes the modal before queues run; then consume the deferred flag
	// (also set by handleCloseModal for the CLOSE_MODAL client packet).
	// TS scans `this.queue` (non-weak LinkList) only — weak entries can't
	// be STRONG (distinct enum values), so scanning all of p.queue is
	// behaviour-equivalent.
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
	// Pass 1: non-weak (NORMAL/STRONG/LONG). Pass 2: weak.
	// Mirrors TS Player.ts:867-868 (processQueue → processWeakQueue).
	s.processPlayerQueueForType(p, false)
	s.processPlayerQueueForType(p, true)
}

// processPlayerQueueForType is the per-kind helper. Walks p.queue
// firing only entries whose weak/non-weak kind matches the filter,
// in insertion order; entries of the other kind are skipped without
// being consumed (the other pass picks them up).
//
// Iterates by index so an entry of the matching kind appended mid-pass
// (via a fired script calling EnqueueScript again of the same kind) is
// visible in the same pass — this preserves TS's authentic "speedup
// quirk" within a single LinkList. A WEAK entry appended during the
// non-weak pass is skipped by this pass's filter but picked up by the
// subsequent weak pass on the same tick, matching TS processWeakQueue
// running AFTER processQueue. A NON-weak entry appended during the
// weak pass fires NEXT tick (the non-weak pass has already returned) —
// also TS-faithful (processQueue does not re-run after processWeakQueue).
//
// Removal happens BEFORE firing so a re-entrant EnqueueScript doesn't
// collide with the index pointer.
func (s *Server) processPlayerQueueForType(p *Player, weak bool) {
	i := 0
	for i < len(p.queue) {
		req := &p.queue[i]
		if (req.Type == script.QueueWeak) != weak {
			i++
			continue
		}
		// TS Player.ts:877-881 — logout-acceleration for LONG entries
		// marked ACCELERATE (args[0] == 0). Force-fires this tick by
		// zeroing the remaining delay before the post-decrement runs.
		// Weak entries are not LONG (distinct enum values), so this
		// guard is a no-op in the weak pass.
		if p.loggingOut && req.Type == script.QueueLong &&
			len(req.IntArgs) > 0 && req.IntArgs[0] == 0 {
			req.Delay = 0
		}
		// TS Player.ts:883 / Player.ts:898 — `const delay = request.delay--;`
		// reads the PRE-decrement value, then decrements; the gate is
		// `delay <= 0`. So a queue entry enqueued with delay=N fires after
		// N ticks, not N-1. Decrementing first and gating on the new value
		// fired one tick early.
		delay := req.Delay
		req.Delay--
		if delay > 0 {
			i++
			continue
		}
		// TS Player.processQueue (Player.ts:883-884) and processWeakQueue
		// (Player.ts:898-899) both gate EVERY entry on
		// `canAccess() && delay <= 0` — there is no per-type exception.
		// STRONG's only special behavior is the modal-close pre-pass in
		// processPlayerQueue above (TS processQueues L854-865); the entry
		// itself still waits for canAccess. M4 widens the gate from
		// delayed-only to full CanAccess (delayed || modal || protected);
		// M5 drops the bogus STRONG-fires-while-delayed exception (TS has
		// none — a STRONG queue closes the modal but still waits for the
		// busy/delay/protect state to clear before firing).
		if !p.CanAccess() {
			i++
			continue
		}
		sf := req.Script
		intArgs := req.IntArgs
		stringArgs := req.StringArgs
		queueType := req.Type
		p.queue = append(p.queue[:i], p.queue[i+1:]...)
		if sf != nil {
			if queueType == script.QueueLong && len(intArgs) > 0 {
				// TS Player.ts:887-889 — LONG's first int arg is the
				// logoutAction indicator (0 = ACCELERATE, others reserved).
				// Strip before the script sees it. The prepend happens at
				// LONGQUEUE enqueue (handlers.go:988) and LONGQUEUEVARARG
				// enqueue (handlers_player_vararg.go:80-81); both fixed-arg
				// and vararg LONG handlers prepend, so the strip is
				// symmetric.
				intArgs = intArgs[1:]
			}
			// TS Player.ts:891,903 — processQueues + processWeakQueue
			// both fire scripts as protected (executeScript(script, true)).
			// Player queue family fires as TriggerQueue (TS Player.processQueues
			// → ScriptRunner.init uses ServerTriggerType.QUEUE).
			s.runScriptFn(sf, p, nil, script.TriggerQueue, true, intArgs, stringArgs)
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
	players := s.snapshotPlayers()

	for _, p := range players {
		func(p *Player) {
			defer recoverPlayer(p, "processPlayerEngineQueues", s.log)
			i := 0
			for i < len(p.engineQueue) {
				req := &p.engineQueue[i]
				// TS Player.ts:643 — post-decrement: read old delay, decrement,
				// gate on `canAccess() && delay <= 0`. Gating on the new value
				// fired one tick early.
				delay := req.Delay
				req.Delay--
				if delay > 0 || !p.CanAccess() {
					i++
					continue
				}
				sf := req.Script
				intArgs := req.IntArgs
				stringArgs := req.StringArgs
				p.engineQueue = append(p.engineQueue[:i], p.engineQueue[i+1:]...)
				if sf != nil {
					// TS Player.ts:646 — executeScript(script, true): protected.
					// engineQueue fires as TriggerQueue (same family as
					// PlayerQueueRequest — TS Player.processEngineQueue uses
					// ScriptRunner.init's default ServerTriggerType.QUEUE).
					s.runScriptFn(sf, p, nil, script.TriggerQueue, true, intArgs, stringArgs)
				}
				// Don't advance i — the slice shrunk by one; index now points
				// at what was the next entry (or past end).
			}
		}(p)
	}
}

// processPlayerTimers fires any ready timers across all players.
//
// PLAYER-SCRIPT-3 closure: drives two passes over the player loop —
// NORMAL timers first (gated by CanAccess), then SOFT timers (no
// CanAccess gate). Mirrors TS World.processPlayers (World.ts:718-723)
// which calls processNormalTimers then processSoftTimers in sequence
// across the whole playerLoop, not interleaved per-player.
//
// The pre-closure implementation iterated each player's timers in
// id-sorted order with NORMAL and SOFT mixed by id; an
// id=5 SOFT timer would fire before an id=10 NORMAL timer on the
// same player, even though TS fires NORMAL first regardless of id.
// Splitting into two distinct passes restores the TS-faithful
// ordering and preserves the across-player NORMAL-before-SOFT
// invariant when scripts on adjacent players observe each other's
// timer-emitted state changes.
//
// Soft timers fire even while p.delayed; normal timers wait for idle.
func (s *Server) processPlayerTimers() {
	s.processPlayerTimersForType(script.TimerNormal)
	s.processPlayerTimersForType(script.TimerSoft)
}

// processPlayerTimersForType is the per-pass helper. Iterates a fresh
// playerLoop snapshot and fires only timers whose Type matches
// filterType, in id-sorted order. Independent snapshots per pass match
// the conventional pattern (cf. processPlayerEngineQueues,
// processClientsOut); within a single tick the playerLoop is only
// mutated on the tick goroutine itself, so the two snapshots are
// identical in practice.
func (s *Server) processPlayerTimersForType(filterType script.PlayerTimerType) {
	players := s.snapshotPlayers()

	for _, p := range players {
		func(p *Player) {
			defer recoverPlayer(p, "processPlayerTimers", s.log)
			// L6: a logging-out player fires no timers (normal OR soft). TS
			// World.processPlayers wraps both processTimers calls in
			// `if (!player.loggingOut)` (World.ts:717-722).
			if p.loggingOut {
				return
			}
			if len(p.timers) == 0 {
				return
			}
			// Deterministic fire order within a pass (maps are unordered).
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
				if t.Type != filterType {
					continue
				}
				if s.currentTick < t.Clock+t.Interval {
					continue
				}
				// TS Player.processTimers (Player.ts:933): a SOFT timer fires
				// whenever it is due; a NORMAL timer additionally requires
				// canAccess(). M6 widens the NORMAL gate from delayed-only to
				// full CanAccess (delayed || modal || protected). TS's
				// World.shutdown→true canAccess branch (the "shutdown force-fire")
				// is the same documented omission as the engine queue
				// (DEVIATION-NAI-144-D4: goscape has no world-shutdown flag).
				if t.Type == script.TimerNormal && !p.CanAccess() {
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
				timerTrigger := script.TriggerTimer
				if t.Type == script.TimerSoft {
					timerTrigger = script.TriggerSoftTimer
				}
				s.runScriptFn(sf, p, nil, timerTrigger, t.Type == script.TimerNormal, t.IntArgs, t.StringArgs)
			}
		}(p)
	}
}

func (s *Server) processClientsOut() {
	players := s.snapshotPlayers()

	for _, p := range players {
		p.processOut()
	}
}

func (s *Server) processInfo() {
	players := s.snapshotPlayers()

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
		p.buildArea.rebuildNormal()
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
			// Send the active faceSquare when set, else the resting orientation
			// (faceAngle, south on login) so the always-forced FACE_COORD
			// low-def orients a fresh player south, not north-east.
			pFaceX, pFaceZ := p.effectiveFaceCoord()
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
				int32(pFaceX), int32(pFaceZ),
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
			// Send the active faceSquare when set, else the resting orientation
			// (faceAngle) so the always-forced FACE_COORD low-def orients a
			// fresh NPC south instead of the client's north-east default.
			faceX, faceZ := n.effectiveFaceCoord()
			s.rsbuf.ComputeNpc(int32(n.nid), int32(n.typeId),
				n.x, n.level, n.z,
				n.tele,
				int8(n.runDir), int8(n.walkDir),
				!n.dead, // active = !dead
				uint32(n.Masks()),
				int32(n.FaceEntity()),
				int32(faceX), int32(faceZ),
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
	// Per-NPC recover mirrors TS World.processNpcs (World.ts:681-690) try/catch
	// around `npc.turn()` → `removeNpc(npc,-1)`. The recovery surface is the
	// counterpart to the despawn-comment claim at npc_ai.go:46-48.
	for _, n := range s.npcLoop {
		func(n *Npc) {
			defer recoverNpc(n, s, "processNpcTurn", s.log)
			n.turn(s)
		}(n)
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

// processInteractionsPreMove runs the pre-movement half of each player's
// interaction cycle (pre-step interact + path recompute), at the player's
// position before this tick's processPathing. See processInteractionPreMove.
func (s *Server) processInteractionsPreMove() {
	players := s.snapshotPlayers()
	for _, p := range players {
		func(p *Player) {
			defer recoverPlayer(p, "processInteractionPreMove", s.log)
			p.processInteractionPreMove()
		}(p)
	}
}

// processInteractions runs the post-movement half of each player's
// interaction cycle (post-step interact + tail), after processPathing.
func (s *Server) processInteractions() {
	players := s.snapshotPlayers()
	for _, p := range players {
		func(p *Player) {
			defer recoverPlayer(p, "processInteractionPostMove", s.log)
			p.processInteractionPostMove()
		}(p)
	}
}

// processCleanup mirrors TS World.processCleanup (reset zones/players/npcs/invs).
// L5 DEVIATION (accepted, benign): the within-cleanup reset ORDER differs from
// TS (goscape resets player masks + flags + invs here, then world invs; TS
// orders zones→players→npcs→invs). No cross-step data dependency makes the
// order observable — each reset clears independent end-of-tick state.
func (s *Server) processCleanup() {
	players := s.snapshotPlayers()
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
		// Note: unfocus() is NOT called per-tick (TS resetEntity(false) doesn't
		// either). The default-south orientation is seeded once at login
		// (addPlayer → p.unfocus()); respawn-after-death re-seeding remains
		// part of the Player death sub-spec.
		p.socialProtect = false
		p.reportAbuseProtect = false
		// Clear per-player inventory update flags at end-of-tick — AFTER every
		// player's updateInvs (in processInfo) has read them. Clearing inside
		// updateInvs would let the first-processed player consume the flag and
		// starve a cross-player listener (a trade offer shown to the partner
		// via invother_transmit). Mirrors TS World.ts:1140-1147.
		for _, inv := range p.invs {
			if inv != nil {
				inv.Update = false
			}
		}
	}
	// World-shared inventories: clear the update flag, then restock/decay shop
	// stock. Mirrors TS World.ts:1155-1190.
	for _, inv := range s.invs {
		if inv == nil {
			continue
		}
		inv.Update = false
		if s.invTypes == nil || inv.Type < 0 || inv.Type >= len(s.invTypes.Configs) {
			continue
		}
		invType := s.invTypes.Configs[inv.Type]
		if invType == nil || !invType.Restock || len(invType.StockCount) == 0 || len(invType.StockRate) == 0 {
			continue
		}
		for index := range inv.Items {
			item := inv.Items[index]
			if item == nil {
				continue
			}
			hasStockCount := index < len(invType.StockCount)
			// rate 0 would be a modulo-by-zero panic in Go (TS yields NaN and
			// skips); guard it like TS's per-element falsy check.
			hasStockRate := index < len(invType.StockRate) && invType.StockRate[index] != 0
			rateHit := hasStockRate && s.currentTick%int(invType.StockRate[index]) == 0
			// Stock-obj retention is derived inside Add/Remove from the
			// inventory's own InvType (matching TS), so callers no longer
			// pass it.
			switch {
			case hasStockCount && rateHit && item.Count < int(invType.StockCount[index]):
				// Below min → restock one at this slot. Stackable mirrors TS,
				// which reads ObjType.stackable inside add() (World.ts:1173 →
				// Inventory.ts:159). Inert for stackall shops (every shipped
				// restock shop is stackall), but correct for a non-stackall
				// restock shop stocking a stackable obj.
				stackable := false
				if ot := s.objTypeFor(item.Id); ot != nil {
					stackable = ot.Stackable
				}
				inv.Add(item.Id, 1, inventory.AddOpts{BeginSlot: index, AssureFullInsertion: true, Stackable: stackable})
				inv.Update = true
			case hasStockCount && rateHit && item.Count > int(invType.StockCount[index]):
				// Above min → decay one.
				inv.Remove(item.Id, 1, inventory.RemoveOpts{BeginSlot: index, AssureFullRemoval: true})
				inv.Update = true
			case invType.AllStock && (!hasStockCount || invType.StockCount[index] == 0) && s.currentTick%invStockRate == 0:
				// Unlisted stock (e.g. general stores) decays one per minute.
				inv.Remove(item.Id, 1, inventory.RemoveOpts{BeginSlot: index, AssureFullRemoval: true})
				inv.Update = true
			}
		}
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
