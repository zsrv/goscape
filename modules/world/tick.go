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
	// DEVIATION SEC1-D2: save everyone, then let the panic continue.
	defer func() {
		if r := recover(); r != nil {
			s.crashSaveAll(r)
			panic(r)
		}
	}()
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

		// arch-28.4a: drain guaranteed disconnect removals before the
		// lossy relay queue, so a disconnect enqueued last tick is
		// processed before any relay traffic this tick.
		s.drainRemovals()

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

		s.tickBodyFn()

		// ── CYCLE (TS World.ts:487) + snapshot (W.ts:489-500) ────────────
		// Measured "before telemetry" — identical placement to TS.
		// snapshotCycleStats then copies cycleStats → lastCycleStats so
		// the MAP_LAST* debug script ops always see a consistent prior-tick
		// snapshot rather than a partially-updated current tick.
		s.cycleStats[statCycle] = uint16(time.Since(start).Milliseconds()) // TS W.ts:487
		s.snapshotCycleStats()                                             // TS W.ts:489-500

		s.currentTick++
		s.stampTick() // arch-29.6: mirror tick state into health-snapshot atomics

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

// tickOnce runs the per-tick game-processing body: client-in, world, npc,
// player, logout, login, zone, client-out, and cleanup passes, plus the
// per-tick session-log flush. Extracted from runTickLoopWithRate (SEC1
// M-1b) as s.tickBodyFn's default so tests can substitute a stub body
// (e.g. one that panics) without perturbing the surrounding sleep/drift/
// quit bookkeeping, which stays in the loop.
func (s *Server) tickOnce() {
	// Cycle-stat instrumentation: zero the ten timing entries at tick
	// start. BANDWIDTH_IN is reset at its own TS-cited point
	// (World.ts:629). Mirrors the implicit zero that TS achieves by
	// assigning cycleStats[X] = Date.now() - start once per section.
	s.resetCycleTimes()

	// PORTING-EXCEPTION (rev244-b4-bwout-reset, BANDWIDTH_OUT reset
	// moved to tick start): TS resets at World.ts:1111 — the head of
	// processClientsOut, which is TS's ONLY write pass, so the stat
	// covers every byte written that cycle. goscape emits packets
	// incrementally THROUGHOUT the tick (login resync, script-driven
	// sends, relay-action friends/PM packets), so resetting at the TS
	// line would silently drop everything written before client-out.
	// Resetting here preserves the stat's TS-INTENT ("bytes out this
	// cycle") at the cost of the literal reset-line position. See
	// docs/PORTING.md §B4.
	s.cycleStats[statBandwidthOut] = 0

	// ── CLIENT_IN (TS World.ts:626-691) ──────────────────────────────
	// BANDWIDTH_IN reset matches TS World.ts:629 (before the player
	// loop that accumulates bytes-read into cycleStats[BANDWIDTH_IN]).
	t0 := time.Now()
	s.cycleStats[statBandwidthIn] = 0 // TS World.ts:629
	s.processClientsIn()
	s.addCycleTime(statClientIn, t0)

	// ── WORLD (TS World.processWorld, W.ts:558-620) ──────────────────
	// TS processWorld covers: world-script queue, obj-delayed queue,
	// and npc-hunt-players. goscape deviates by splitting processWorld
	// into multiple passes (NAI-37/NAI-122/NAI-134/NAI-217); the WORLD
	// accumulator is updated after each member pass so the total matches
	// what TS measures in a single function. processNpcEventQueue and
	// processActiveScripts have no TS stat bucket of their own and are
	// folded here as "world-level infrastructure" passes.
	t0 = time.Now()
	s.processWorldQueue() // NAI-37: matches TS World.processWorld start-of-cycle ordering
	s.addCycleTime(statWorld, t0)

	// NAI-122: processNpcEventQueue moved up to mirror TS World.ts:356
	// (drains BEFORE processPlayers at TS line 376). Closes the
	// V-PARTIAL where AI_SPAWN-populated npc varns
	// (%npc_combat_xp_multiplier and friends) were read as zero by
	// same-tick combat dispatch because the queue drained AFTER
	// processInteractions. DEVIATION-NAI-122-D3 declared in Bundle 0
	// findings: NAI-121 audit's "TS sync-inline" claim was a misread
	// — TS uses a unified queue identical to goscape's, just drained
	// earlier in the tick.
	t0 = time.Now()
	s.processNpcEventQueue()
	s.addCycleTime(statWorld, t0)

	// ── NPC (TS World.ts:711-722) ─────────────────────────────────────
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
	t0 = time.Now()
	s.processNpcHuntPlayers()
	s.addCycleTime(statWorld, t0)

	t0 = time.Now()
	s.processNpcs()
	s.addCycleTime(statNpc, t0)

	// processActiveScripts has no dedicated TS stat bucket — it fires
	// world-suspended scripts, which TS drains inside processWorld.
	// Folded into WORLD accumulator.
	t0 = time.Now()
	s.processActiveScripts()
	s.addCycleTime(statWorld, t0)

	// NAI-134: drain the obj-delayed-spawn queue here, after script-firing,
	// so a same-tick INV_DROPITEM_DELAYED with delay=0 spawns the obj before
	// processInfo reads zone state. DEVIATION from TS (L1): TS drains the
	// objDelayedQueue inside processWorld at cycle START (World.ts:562-574),
	// BEFORE the same cycle's processNpcs (L365), so a delayed obj is visible
	// to NPC hunt the SAME tick. goscape drains it AFTER processNpcs, so a
	// delayed-spawned obj is visible to NPC obj-hunt one tick later. Accepted
	// LOW: 1-tick latency on the rare HuntModeObj path; the placement is what
	// gives delay=0 player drops same-tick visibility before processInfo.
	t0 = time.Now()
	s.processObjDelayedQueue()
	s.addCycleTime(statWorld, t0)

	// ── PLAYER (TS World.processPlayers, W.ts:724-776) ───────────────
	// goscape splits processPlayers into discrete passes; each is timed
	// and accumulated into statPlayer. The split is the documented
	// Arc-29/NAI-217 pre-step/post-step deviation; call ORDER is
	// preserved exactly.
	t0 = time.Now()
	s.processPlayerTimers()
	s.addCycleTime(statPlayer, t0)

	// NAI-144: TS World.ts:725 — engineQueue drains between timers and
	// movement. processPlayerEngineQueues mirrors TS
	// Player.processEngineQueue per-player drain semantics.
	t0 = time.Now()
	s.processPlayerEngineQueues()
	s.addCycleTime(statPlayer, t0)

	// "// Update target facing" — TS World.ts:707-708 @2e3bcf43
	// (ee28c1aa): player.setFaceEntity() runs per player after
	// processEngineQueue and before processInteraction.
	t0 = time.Now()
	s.processPlayerFacing()
	s.addCycleTime(statPlayer, t0)

	// TS Player.processInteraction interleaves updateMovement between its
	// pre-step and post-step interact arms (Player.ts:1241). goscape splits
	// that around the movement pass: the pre-step interact (+ path recompute)
	// runs at the player's PRE-movement position here, then processPathing
	// moves, then processInteractions runs the post-step arm + tail. This is
	// what lets a player who clicks an in-range NPC attack from where they
	// stand instead of stepping to contact first.
	t0 = time.Now()
	s.processInteractionsPreMove()
	s.addCycleTime(statPlayer, t0)

	t0 = time.Now()
	s.processPathing()
	s.addCycleTime(statPlayer, t0)

	t0 = time.Now()
	s.processInteractions()
	s.addCycleTime(statPlayer, t0)

	// TS World.ts @4c95f87e (e31a8719): player.reorient() runs after
	// movement — face a loc/obj target if we walked over and held
	// still. MUST run after processPathing/processInteractions so
	// stepsTaken reflects this tick.
	t0 = time.Now()
	s.processPlayerReorient()
	s.addCycleTime(statPlayer, t0)

	t0 = time.Now()
	s.processEnergy() // NAI-135: TS World.ts:731 per-player updateEnergy
	s.addCycleTime(statPlayer, t0)

	// M3: TS World.ts:733-735 — jump-snap any player who moved >2 tiles
	// this tick (gated by EXACT_MOVE). Runs after movement+energy, before
	// processInfo serializes the jump bit.
	t0 = time.Now()
	s.processValidateDistanceWalked()
	s.addCycleTime(statPlayer, t0)

	// ── LOGOUT (TS World.ts:778-846) ─────────────────────────────────
	t0 = time.Now()
	s.processLogouts()
	s.addCycleTime(statLogout, t0)

	// ── LOGIN (TS World.ts:848-976) ──────────────────────────────────
	t0 = time.Now()
	s.processLogins()
	s.addCycleTime(statLogin, t0)

	// ── ZONE (TS World.ts:978-1005) ──────────────────────────────────
	// L2 DEVIATION (accepted, documented NAI-93): TS runs processZones
	// (W.ts:388) BEFORE processInfo (W.ts:395); goscape runs processInfo
	// first so rebuildNormal (TS BuildArea slot, W.ts:996) settles before
	// zone compute. Cost is a 1-tick facing artifact for a just-revealed
	// zone — see the NAI-93 notes in player.go / processInfo below.
	// Stat attribution: TS leaves processInfo UNMEASURED (its body,
	// W.ts:1012-1108, writes no cycleStats entry); goscape attributes
	// it to CLIENT_OUT — the adjacent phase that consumes its rsbuf
	// computes — rather than leaving the time invisible. Deviation:
	// goscape's CLIENT_OUT therefore reads slightly higher than TS's
	// (which times only processClientsOut, W.ts:1109-1145).
	t0 = time.Now()
	s.processInfo()
	s.addCycleTime(statClientOut, t0)

	t0 = time.Now()
	s.processZones() // compute ComputeShared before delivery
	s.addCycleTime(statZone, t0)

	// ── CLIENT_OUT (TS World.ts:1108-1145) ───────────────────────────
	// BANDWIDTH_OUT reset: moved to tick start — see the
	// PORTING-EXCEPTION (rev244-b4-bwout-reset) note there (TS resets
	// at World.ts:1111, but goscape writes throughout the tick).
	t0 = time.Now()
	s.processClientsOut()
	s.addCycleTime(statClientOut, t0)

	// ── CLEANUP (TS World.ts:1147-1219) ──────────────────────────────
	t0 = time.Now()
	s.processCleanup()
	s.addCycleTime(statCleanup, t0)

	// processSessionLogs counts only toward CYCLE (like TS's
	// session-log block at W.ts:462-485, which runs after CLEANUP and
	// before the CYCLE measurement).
	s.processSessionLogs() // NAI-74: TS World.cycle session-log block (W.ts:428-442)
}

// snapshotPlayers returns a stable copy of s.players for one tick pass
// to iterate: passes like processLogouts remove players from the live
// registry mid-iteration, so ranging players.all() directly would
// misbehave. (playerList.all() ranges the playerLoop bucket slices —
// rev-254 processing order, bucket then login order — and a concurrent
// loopUnlink shifts bucket entries under the range. The TS HashTable
// iterator instead pre-reads node.next before yielding; snapshotting is
// simpler and preserves the existing tick-goroutine ownership
// invariant.)
//
// The copy lands in s.playerScratch, reused across passes — pre-PERF-1
// each of the 13 passes allocated a fresh slice, ~13 allocs/tick scaling
// with player count. Tick-goroutine-only. The returned slice is valid
// until the next snapshotPlayers call; callers must not retain it across
// passes.
func (s *Server) snapshotPlayers() []*Player {
	s.playersMu.RLock()
	prev := len(s.playerScratch)
	s.playerScratch = s.playerScratch[:0]
	for p := range s.players.all() {
		s.playerScratch = append(s.playerScratch, p)
	}
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
			p.client.closeConn()
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
			profile := s.cfg.NodeProfile
			privateChat := int32(p.privateChat)
			staffLvl := p.staffModLevel
			// arch-29.13: enqueue on the single global friends dispatcher
			// instead of firing an ad hoc goroutine, so this PlayerLogin
			// executes strictly before any later friends mutation for
			// this player (e.g. a same-tick or later-tick PlayerLogout)
			// — restoring the TS friendThread.postMessage FIFO
			// guarantee. The onResponse callback runs synchronously on
			// the dispatcher's worker goroutine, outside the per-call
			// ctx's cancellation reach, so it MUST stay fast and
			// non-blocking — a blocking callback stalls every queued
			// friends mutation (see the FriendsClient.PlayerLogin
			// contract). This Warn-only body satisfies that, and it
			// touches only s.logTick, never tick-owned player state. The
			// per-call timeout bounds the gRPC call and now lives on the
			// dispatcher's worker (context.WithTimeout(bridgesCtx,
			// callTimeout) per dequeue).
			s.friendsMutationDispatcher.enqueue(func(ctx context.Context) {
				s.friendsClient.PlayerLogin(ctx, &friendspb.PlayerLoginRequest{
					WorldId:     worldID,
					Profile:     profile,
					Username37:  username37,
					PrivateChat: privateChat,
					StaffLvl:    staffLvl,
				}, func(accepted bool) {
					if !accepted {
						s.logTick.Warn("friends-server rejected player login (cap reached or RPC error)",
							slog.Int("world_id", int(worldID)),
							slog.Uint64("username37", username37),
						)
					}
				})
			})
		}

		// NAI-S4A: start the SubscribeUpdates stream subscriber.
		// Lives until logout/disconnect cancels p.friendsSubCancel.
		if s.friendsClient != nil && p.username != "" {
			subCtx, subCancel := context.WithCancel(context.Background())
			p.friendsSubCancel = subCancel
			p.friendsSub = newFriendsSubscriber(s.friendsClient, int32(s.cfg.NodeID), s.cfg.NodeProfile, p.username37, s.friendsDispatcher, s.logFriends)
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
			s.logTick.Warn("LoadSave failed; falling back to empty bootstrap",
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
			// (Player.ts:486-504 @43e02957). DEVIATION-NAI-182-D4 omits
			// IF_CLOSE.
			sendChatFilterSettings(p, p.publicChat, p.privateChat, p.tradeDuel)
			// Friends-list bootstrap status right after CHAT_FILTER_SETTINGS
			// (TS Player.ts:496-501 @43e02957). With a friends server,
			// status 1 ("connecting"); the relayed list later completes with
			// a status-2 emit (bridges.go OnFriendlistUpdate, TS
			// World.ts:2008). Without one, status 2 immediately plus the
			// empty UPDATE_IGNORELIST bootstrap. 254 INVERTS the 245.2
			// conditional (was `if (!FRIEND_SERVER) UpdateIgnoreList([])`);
			// retires DEVIATION-NAI-182-D5-NO-DEFENSIVE-IGNORELIST-LOGIN-EMIT.
			// Gate mirrors TS Environment.FRIEND_SERVER: s.friendsClient is
			// non-nil iff FriendsServerEnabled and the client dialed
			// (world.go:66-75) — same gate as the PlayerLogin RPC below.
			if s.friendsClient != nil {
				sendFriendlistLoaded(p, 1)
			} else {
				sendFriendlistLoaded(p, 2)
				sendUpdateIgnoreList(p, nil)
			}
			// TS UpdatePidEncoder (unchanged @2e3bcf43): p2 + pbool.
			// TS Player.ts:500 `new UpdatePid(this.slot, this.members)` —
			// the wire value is the player's slot; members is the
			// player's own membership flag.
			sendUpdatePid(p, p.slot, p.members)
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
		// DEVIATION SEC1-D1: TS has no per-player catch here (a throw
		// reaches cycle()'s catch and the process exits). goscape contains
		// the panic to this player: recoverPlayer logs, flags requestLogout
		// and closes the socket, so one corrupt save/script cannot take the
		// world down. Closure so the deferred recover runs per player.
		if s.scriptProvider != nil {
			func() {
				defer recoverPlayer(p, "loginTrigger", s.logTick)
				sf := s.scriptProvider.GetByTrigger(script.TriggerLogin, -1, -1)
				s.runScriptFn(sf, p, nil, script.TriggerLogin, true, nil, nil)
			}()
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

		// p.input is allocated in newPlayer (TS Player.ts:428 ctor
		// parity; the 254 InputTracking has no tick-scheduled state).
		// session is normally assigned in newPlayer() from the
		// PlayerLoginResponse.session_uuid; the "headless" fallback
		// below covers standalone-world and unit-test paths that bypass
		// the login bridge.
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
				// DEVIATION SEC1-D1 (see login trigger above): contain a
				// [logout] panic to this player. Removal continues below
				// regardless — recoverPlayer's requestLogout/close are
				// no-ops for a player already being torn down.
				func() {
					defer recoverPlayer(p, "logoutTrigger", s.logTick)
					s.runScriptFn(logoutScript, p, nil, script.TriggerLogout, true, nil, nil)
				}()
			} else {
				s.logTick.Warn("no [logout] trigger registered; removing player without it",
					"player", p.username)
			}

			// Clear any suspended script so a late RESUME_* packet doesn't
			// reference a player that's logged out.
			p.activeScript = nil
			p.writeOut(gameserver.OpLogout, nil)
			_ = p.client.flushWrite()
			p.client.closeConn()

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
// across the whole players list, not interleaved per-player.
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
// players-list snapshot and fires only timers whose Type matches
// filterType, in id-sorted order. Independent snapshots per pass match
// the conventional pattern (cf. processPlayerEngineQueues,
// processClientsOut); within a single tick the players list is only
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
	// rev-274 perf gate (TS World.ts:979-981 @dee467c8): an empty world has
	// no observers, so the info-processing pass (player reorient/rebuild +
	// rsbuf player/npc compute) can produce no visible work. Short-circuit
	// before doing any of it. TS: `if (this.getTotalPlayers() === 0) return;`.
	if s.getTotalPlayers() == 0 {
		return
	}

	players := s.snapshotPlayers()

	// facing (reorientEntity/reorient) runs in the player's turn
	// (processPlayers), not here. TS World.ts:992 @4c95f87e (e31a8719).
	//
	// NAI-93: TS World.ts:996 — buildArea.rebuildNormal() runs in this
	// loop, BEFORE the ComputePlayers/ComputePlayer calls below, so the
	// rsbuf-cached Origin matches the just-emitted RebuildNormal packet's
	// zoneX/zoneZ. Inverting this order produces stale-origin tele leaves
	// on cross-window teles → Java client AIOOBE in getHeightmapY/getTopLevel.
	// TS comment at World.ts:996 verbatim: "set origin before compute
	// player is why this is above."
	for _, p := range players {
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

	// facing (reorientEntity/reorient) runs in Npc.turn(), not here. TS
	// World.ts:1045 @4c95f87e (e31a8719).
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
				int32(p.damage2Amt), int32(p.damage2Type),
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
		// Iterates s.npcLoop (all NPCs, alive and dead) mirroring TS
		// World.ts:1066-1096 which iterates this.npcs unconditionally and
		// passes npc.isActive (= !dead) to rsbuf.computeNpc. RESPAWN-lifecycle
		// dead NPCs must receive ComputeNpc(active=false) each tick so the rsbuf
		// flips Active=false and writeNpcs removes the corpse from client
		// tracking. Skipping dead NPCs (pre-fix "dead-bool divergence") left
		// Active=true in the rsbuf, pinning the corpse indefinitely on clients
		// (B6 live-smoke: "corpse remains on ground after death" — rev-244 B6).
		// DESPAWN-lifecycle dead NPCs have nid=-1 (set by Cleanup in removeNpc)
		// or a valid nid with b.npcs[nid]==nil (set by rsbuf.RemoveNpc) — both
		// cause ComputeNpc to no-op safely (nid<0 guard / nil-slot guard).
		for _, n := range s.npcLoop {
			if n == nil {
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
				n.jump, // rev-274 add-leaf jump bit (World.ts:1048 @dee467c8)
				int8(n.runDir), int8(n.walkDir),
				!n.dead, // active = !dead
				uint32(n.Masks()),
				int32(n.FaceEntity()),
				int32(faceX), int32(faceZ),
				int32(n.OrientationX), int32(n.OrientationZ),
				int32(n.DamageAmt()), int32(n.DamageType()),
				int32(n.Damage2Amt()), int32(n.Damage2Type()),
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

// processPlayerFacing runs the per-player "Update target facing" pass —
// TS World.ts @4c95f87e: both halves of "face the interaction target" run
// here, the same tick the op set the target and before processInteraction
// can clear it: the FACE_ENTITY mask (setFaceEntity, ee28c1aa @2e3bcf43),
// plus the serverside faceAngle toward a pathing target for new observers
// (reorientEntity, e31a8719).
func (s *Server) processPlayerFacing() {
	players := s.snapshotPlayers()
	for _, p := range players {
		p.setFaceEntity()
		p.reorientEntity()
	}
}

// processPlayerReorient runs after movement: face a loc/obj target if we
// walked over and held still (needs this tick's stepsTaken). TS World.ts
// @4c95f87e — player.reorient() between processInteraction and
// updateEnergy (e31a8719).
func (s *Server) processPlayerReorient() {
	players := s.snapshotPlayers()
	for _, p := range players {
		p.reorient()
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
		// Clear per-player inventory update tracking at end-of-tick — AFTER
		// every player's updateInvs (in processInfo) has read both the update
		// flag and the dirty-slot set. Clearing inside updateInvs would let the
		// first-processed player consume them and starve a cross-player listener
		// (a trade offer shown to the partner via invother_transmit). 274:
		// ResetTracking clears both inv.update and inv.dirtySlots. Mirrors TS
		// World.ts:1140 (inv.resetTracking()).
		for _, inv := range p.invs {
			if inv != nil {
				inv.ResetTracking()
			}
		}
	}
	// World-shared inventories: reset tracking, then restock/decay shop stock.
	// 274: ResetTracking clears both inv.update and inv.dirtySlots; the restock
	// add/remove below re-dirty exactly the slots they touch, so the next
	// tick's partial update carries only the restocked slots. Mirrors TS
	// World.ts:1151-1186.
	for _, inv := range s.invs {
		if inv == nil {
			continue
		}
		inv.ResetTracking()
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
				// Below min → restock one at this slot. 274: bare 3-param add
				// (beginSlot=index) — TS World.ts:1170 `inv.add(item.id, 1,
				// index)`. Stackable mirrors TS, which reads ObjType.stackable
				// inside add() (Inventory.ts:108-109); inert for stackall shops
				// (every shipped restock shop is stackall), but correct for a
				// non-stackall restock shop stocking a stackable obj.
				stackable := false
				if ot := s.objTypeFor(item.Id); ot != nil {
					stackable = ot.Stackable
				}
				inv.Add(item.Id, 1, index, stackable)
				inv.Update = true
			case hasStockCount && rateHit && item.Count > int(invType.StockCount[index]):
				// Above min → decay one. 274: bare 3-param remove (TS
				// World.ts:1176 `inv.remove(item.id, 1, index)`).
				inv.Remove(item.Id, 1, index)
				inv.Update = true
			case invType.AllStock && (!hasStockCount || invType.StockCount[index] == 0) && s.currentTick%invStockRate == 0:
				// Unlisted stock (e.g. general stores) decays one per minute.
				inv.Remove(item.Id, 1, index)
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
