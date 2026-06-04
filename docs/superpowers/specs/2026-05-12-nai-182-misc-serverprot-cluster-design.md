# NAI-182 — Misc ServerProt cluster (`UPDATE_PID` / `RESET_ANIMS` / `RESET_CLIENT_VARCACHE` / `UPDATE_REBOOT_TIMER`) + onReconnect lifecycle + shutdown consumer + reboot cheats

**Date:** 2026-05-12
**Status:** Design (standard cadence — brainstorm → spec → plan → subagent-driven TDD)
**Tracker:** Ports 4 of 8 missing TS `ServerGameProt.ts` opcodes. Sibling cluster of 4 social opcodes (UPDATE_FRIENDLIST / UPDATE_IGNORELIST / MESSAGE_PRIVATE server-side / CHAT_FILTER_SETTINGS) deferred to a future sub-spec.
**Predecessor:** NAI-181 (LAST_LOGIN_INFO) closed at `2c00fae`.
**HEAD at design:** `main` (post-NAI-181 close).

## 1. Problem

`pkg/io/protocol/game/server/prot.go` is missing 4 server opcode declarations and their wire encoders. Each opcode has documented TS call sites that establish login telemetry, animation reset on login/reconnect, client-side varp cache invalidation, and the reboot-timer countdown. Beyond the encoders, the surrounding TS subsystems include `Player.onReconnect` (a separate resync lifecycle that does not currently exist in goscape) and the world-side shutdown consumer (`World.processShutdown` + `::reboot` / `::slowreboot` staff-cheats).

This sub-spec lands the full TS-parity cluster:

- 4 encoders (`UPDATE_PID`, `RESET_ANIMS`, `RESET_CLIENT_VARCACHE`, `UPDATE_REBOOT_TIMER`).
- Fresh-login wiring at `processLogins` matching TS `Player.onLogin` (Player.ts:494-504).
- `onReconnect(p)` lifecycle branching on `p.reconnecting==true` at `processLogins`.
- `Server.shutdownTick` + `Server.rebootTimer(duration)` broadcaster.
- `Server.processShutdown()` consumer at the top of `s.tick()` — kicks players, force-removes after 1024 ticks, requests graceful module stop on zero online.
- 3 ServerProt-coupled staff-cheats: `::reboot`, `::slowreboot <seconds>`, `::serverdrop`.

The 4 social opcodes (UPDATE_FRIENDLIST/IGNORELIST/MESSAGE_PRIVATE server-bound/CHAT_FILTER_SETTINGS) and the other 25 TS ClientCheatHandler cheats are explicitly out of scope (see §6 deviations).

## 2. Affected sites (pre-flight verified at HEAD `2c00fae`)

Verified at HEAD via grep + Read:

| Path | State at HEAD |
| --- | --- |
| `pkg/io/protocol/game/server/prot.go` | No `OpUpdatePid` / `OpResetAnims` / `OpResetClientVarCache` / `OpUpdateRebootTimer` entries. Opcodes 139/136/193/43 unused in this file. |
| `modules/world/tick.go:99-181` (`processLogins`) | No `UPDATE_PID` / `RESET_*` / `UPDATE_REBOOT_TIMER` emits. No `p.reconnecting` branch. |
| `modules/world/tick.go:22-83` (`runTickLoopWithRate`) | Tick body: `processClientsIn` → `processWorldQueue` → `processNpcEventQueue` → `processActiveScripts` → `processObjDelayedQueue` → `processPlayerTimers` → `processPlayerEngineQueues` → `processPathing` → `processInteractions` → `processEnergy` → `processNpcs` → **`processLogouts`** → **`processLogins`** → `processInfo` → `processZones` → `processClientsOut` → `processCleanup` → `processSessionLogs` → `s.currentTick++`. No shutdown consumer. |
| `modules/world/server.go:60` | `currentTick int` exists. |
| `modules/world/server.go:425-446` | `Server.Stop()` calls `s.handler.Stop()` (input-handler stop). `Server.Shutdown()` closes `s.quit`, closes TCP listener, waits on `s.tcpWg`. Self-initiated graceful exit currently has no path — see Risk §5-3. |
| `modules/world/world.go:93-108` (`runFn`) | Waits on `ctx.Done()` (dskit Manager) OR `serverDone <- serv.Run()`. A nil-error from `Run()` is treated as "stopped unexpectedly". |
| `modules/world/player.go:79,87,237` | `slot int`, `uid int`, `runenergy int` all present (verified). |
| `modules/world/player.go:301` (`reconnecting`) | `p.reconnecting bool` exists; set by login codec (`server.go:650`), consumed only by `shouldRebuild` (player.go:694) and cleared by `rebuildNormal` (player.go:809). |
| `modules/world/player_script.go:857` | `(*Player).CloseModal(clearWeakQueue bool)` exists. |
| `modules/world/player_interface.go:73` | `(*Player).IfSetTab(com, tab int)` exists. |
| `modules/world/stat_update.go:10-24` | `sendUpdateStat(p, stat, exp, level)` and `sendUpdateRunEnergy(p, energy)` exist. |
| `modules/world/player_varp.go:14` | `(*Player).writeVarp(id int, value int32)` exists, gated on `VarPlayerType.Transmit`. |
| `pkg/objtype/varptype.go:22,36` | `VarPlayerType.Transmit bool` populated by cache loader. |
| `modules/world/player.go:843-895` | `(*Player).updateInvs()` reads `l.FirstSeen` from each `p.invListeners[com]`; flips to `false` after emit via read-modify-write (map-value addressability — see Risk §5-8). |
| `modules/world/handler_opheld.go:95,196,362` | `p.masks |= p.entitymask` idiom established (3 sites). |
| `modules/world/handlers_game.go:335-411` | `handleClientCheat` with `staffModLevel >= 2` gate; 3 cases wired: `say`, `getcoord`, `tele`. No `reboot` / `slowreboot` / `serverdrop`. |
| `modules/world/handlers_game_test.go:354-470` | `dispatchTeleCheat` helper at line 384-394 is the cheat-dispatch test template. |
| `modules/world/stat_update.go` | Template for new top-level send-functions: `func sendXxx(p *Player, ...) { buf := packet.NewPacket(nil); ...; p.writeOut(gameserver.OpXxx, buf.Bytes()) }`. |

TS source verified at `/home/owner/Code/github.com/LostCityRS/Engine-TS`:

- `src/network/game/server/ServerGameProt.ts` — opcodes 139 (UPDATE_PID, 2), 136 (RESET_ANIMS, 0), 193 (RESET_CLIENT_VARCACHE, 0), 43 (UPDATE_REBOOT_TIMER, 2).
- `src/network/game/server/codec/UpdatePidEncoder.ts:9` — `buf.p2(message.uid)` (TS-side field is the literal `slot`, passed at `Player.ts:495` via `new UpdatePid(this.slot)`).
- `src/network/game/server/codec/{ResetAnims,ResetClientVarCache}Encoder.ts` — empty `encode()` body (0-byte).
- `src/network/game/server/codec/UpdateRebootTimerEncoder.ts:9` — `buf.p2(message.ticks)`.
- `src/engine/entity/Player.ts:494-504` (`onLogin`) — emit order: `rebuildNormal` → `ChatFilterSettings` → (if no friend-server) `UpdateIgnoreList` → `IfClose` → **`UpdatePid(slot)`** → **`ResetClientVarCache`** → **varp-transmit-loop** → **`ResetAnims`** → fire LOGIN trigger.
- `src/engine/entity/Player.ts:516-574` (`onReconnect`) — emit order: **`ResetClientVarCache`** → **varp-transmit-loop** → `buildArea.clear(true)` → `buildArea.rebuildNormal(true)` → (if `World.isPendingShutdown`) **`UpdateRebootTimer(shutdownTicksRemaining)`** → `closeModal()` → per-tab `IfSetTab` → `refreshInvs()` → per-stat `UpdateStat` → `UpdateRunEnergy` → **`ResetAnims`** → `masks |= entitymask`.
- `src/engine/World.ts:166,944-946,1787-1801,1198-1226` — `shutdownTick: number = -1`; `processLogins` emit if `shutdownTick != -1`; `rebootTimer(duration)` broadcaster; `processShutdown` body (per-player logout, force-remove after 1024-tick duration, `process.exit(0)` on zero online + zero logout requests, `tickRate=0` after 2-tick duration).
- `src/engine/World.ts:419` — `if (this.shutdown) this.processShutdown();` at top of tick body.
- `src/network/game/client/handler/ClientCheatHandler.ts:360-376` — `::reboot` → `World.rebootTimer(0)`; `::slowreboot <s>` → `World.rebootTimer(Math.ceil(tryParseInt(args[0], 30) * 1000 / 600))`; `::serverdrop` → `player.terminate()`.
- `tryParseInt(args[0], 30)` — TS helper. Default 30 seconds when `args[0]` missing or unparseable.

## 3. Architecture

### 3.1 Opcode declarations

Append to `pkg/io/protocol/game/server/prot.go` in the "misc player-state" group (adjacent to `OpUpdatePid` group placement; existing `OpUpdateStat` / `OpUpdateRunEnergy` precedent):

```go
// OpUpdatePid carries the player's server-side slot to the client so
// the client's localPlayer reference is bound to the correct PlayerInfo
// slot. Emitted once at onLogin (and onReconnect — though TS onReconnect
// omits it; UpdatePid is login-only). Fixed 2-byte payload: p2(slot).
// Mirrors TS ServerGameProt.UPDATE_PID (139, 2) and UpdatePidEncoder.ts.
OpUpdatePid = Op{Opcode: 139, PayloadSize: 2}

// OpResetAnims tells the client to clear all animation layers on the
// local player. Zero-byte payload. Emitted at onLogin (after varp
// resync) and onReconnect (after per-stat UpdateStat/UpdateRunEnergy).
// Mirrors TS ServerGameProt.RESET_ANIMS (136, 0) and ResetAnimsEncoder.ts.
OpResetAnims = Op{Opcode: 136, PayloadSize: 0}

// OpResetClientVarCache tells the client to drop its cached varp values
// so the next varp packets become authoritative. Emitted at onLogin and
// onReconnect immediately before the varp-transmit-loop. Zero-byte
// payload. Mirrors TS ServerGameProt.RESET_CLIENT_VARCACHE (193, 0) and
// ResetClientVarCacheEncoder.ts.
OpResetClientVarCache = Op{Opcode: 193, PayloadSize: 0}

// OpUpdateRebootTimer carries the number of game ticks (600ms each)
// remaining until the world reboots. Sent broadcast by Server.rebootTimer
// and to each connecting player at processLogins if a shutdown is
// pending. Fixed 2-byte payload: p2(ticks). Mirrors TS
// ServerGameProt.UPDATE_REBOOT_TIMER (43, 2) and UpdateRebootTimerEncoder.ts.
OpUpdateRebootTimer = Op{Opcode: 43, PayloadSize: 2}
```

### 3.2 Encoder send-functions

New file `modules/world/login_resync.go`:

```go
package world

import (
	"github.com/zsrv/goscape/pkg/io/packet"
	gameserver "github.com/zsrv/goscape/pkg/io/protocol/game/server"
)

// sendUpdatePid writes one UPDATE_PID packet. Mirrors TS
// UpdatePidEncoder (`buf.p2(message.uid)`); TS passes p.slot at
// Player.ts:495 — slot is the int field, not the composed uid.
func sendUpdatePid(p *Player, slot int) {
	buf := packet.NewPacket(nil)
	buf.P2(uint16(slot))
	p.writeOut(gameserver.OpUpdatePid, buf.Bytes())
}

// sendResetClientVarCache writes one RESET_CLIENT_VARCACHE packet
// (0-byte payload).
func sendResetClientVarCache(p *Player) {
	p.writeOut(gameserver.OpResetClientVarCache, nil)
}

// sendResetAnims writes one RESET_ANIMS packet (0-byte payload).
func sendResetAnims(p *Player) {
	p.writeOut(gameserver.OpResetAnims, nil)
}
```

New file `modules/world/reboot.go`:

```go
package world

import (
	"math"

	"github.com/zsrv/goscape/pkg/io/packet"
	gameserver "github.com/zsrv/goscape/pkg/io/protocol/game/server"
)

// sendUpdateRebootTimer writes one UPDATE_REBOOT_TIMER packet carrying
// the remaining tick count (NOT seconds). Mirrors TS
// UpdateRebootTimerEncoder (`buf.p2(message.ticks)`).
func sendUpdateRebootTimer(p *Player, ticks int) {
	buf := packet.NewPacket(nil)
	buf.P2(uint16(int16(ticks)))
	p.writeOut(gameserver.OpUpdateRebootTimer, buf.Bytes())
}
```

`ticks` is stored as `int` in goscape (matching TS `number`); the wire is `p2` which TS treats as signed 16-bit (`buf.p2` accepts the JS number directly). Negative values are possible mid-tick when `currentTick > shutdownTick`; the `int16` cast preserves the bit pattern under the conversion to `uint16`. In practice the producer only emits non-negative ticks because `processShutdown` runs first and drops the world before subsequent `rebootTimer` broadcasts.

### 3.3 `Server.shutdownTick` field + reboot infra

Add to `modules/world/server.go` near `currentTick int` at line 60:

```go
// shutdownTick is the tick on which the world will halt. -1 means no
// shutdown scheduled. Set by Server.rebootTimer; consumed by
// Server.processShutdown (called at top of tick body when
// s.currentTick >= s.shutdownTick).
// Mirrors TS World.shutdownTick (World.ts:166).
shutdownTick int
```

Initialise to `-1` in `newServer` (the existing constructor). Add to `reboot.go`:

```go
// rebootTimer schedules a world reboot in `duration` ticks and
// broadcasts the new countdown to every connected player. Mirrors TS
// World.rebootTimer (World.ts:1787-1793).
func (s *Server) rebootTimer(duration int) {
	s.shutdownTick = s.currentTick + duration
	for _, p := range s.playerLoop {
		sendUpdateRebootTimer(p, s.shutdownTick-s.currentTick)
	}
}

// isPendingShutdown reports whether a shutdown is currently scheduled.
// Mirrors TS World.isPendingShutdown (World.ts:1795-1797). Equivalent
// to s.shutdownTicksRemaining() > -1.
func (s *Server) isPendingShutdown() bool {
	return s.shutdownTicksRemaining() > -1
}

// shutdownTicksRemaining returns shutdownTick - currentTick. Returns a
// negative number when no shutdown is scheduled (shutdownTick == -1
// and currentTick >= 0). Mirrors TS World.shutdownTicksRemaining
// (World.ts:1799-1801).
func (s *Server) shutdownTicksRemaining() int {
	return s.shutdownTick - s.currentTick
}
```

### 3.4 Shutdown consumer (D2 — `processShutdown`)

Add to `reboot.go`:

```go
// shutdownGraceful is closed by processShutdown when graceful exit
// conditions are met. The world.go runFn consumes this to distinguish
// a graceful self-initiated stop from an "unexpected" Run() return.
// Wire-up of the runFn consumer side is in §3.4.1 below.
//
// Replaces the bare `serverDone <- serv.Run() ; return fmt.Errorf("server
// stopped unexpectedly")` path at world.go:106 for the reboot-initiated
// case.

// processShutdown runs at the top of s.tick() when s.shutdownTick != -1
// && s.currentTick >= s.shutdownTick. Mirrors TS World.processShutdown
// (World.ts:1198-1226).
func (s *Server) processShutdown() {
	// (a) For every connected player, request logout. goscape's
	// processLogouts handles the per-player drain; we set
	// p.loggingOut=true here so the next processLogouts pass removes
	// them. TS calls player.logout() + player.client.close() inline;
	// goscape reuses the existing logout machinery to keep one drain path.
	for _, p := range s.playerLoop {
		if p != nil && p.client != nil {
			p.loggingOut = true
		}
	}

	duration := s.currentTick - s.shutdownTick

	// (b) After 1024 ticks (~10 minutes at 600ms/tick), force-remove any
	// player that hasn't completed logout. TS uses
	// `this.removePlayer(player)` inline; goscape's removePlayer
	// equivalent is the processLogouts force-removal branch (see
	// tick.go:218-233 timeoutNoResponse/Connection). Mirror TS by
	// flagging each remaining player with a force-removal sentinel.
	if duration >= 1024 {
		for _, p := range s.playerLoop {
			if p != nil {
				p.forceRemove = true // new field, drives processLogouts force-branch
			}
		}
	}

	// (c) Graceful exit when zero players remain. TS calls
	// process.exit(0); goscape signals via shutdownGraceful so the
	// caller (the tick loop in runTickLoopWithRate) returns from Run().
	// We deliberately do NOT close(s.quit) here — the dskit stoppingFn
	// later calls Server.Shutdown() which closes s.quit, and double-close
	// would panic. The early return from the tick loop (§3.4 patch below)
	// is sufficient to terminate Run() via the existing send-on-defer
	// path in world.go runFn.
	if s.getTotalPlayers() == 0 {
		s.shutdownGraceful = true
		return
	}
}
```

Add `forceRemove bool` to `Player` (new field, defaults false).

Add `shutdownGraceful bool` to `Server`.

Modify `tick.go:22-83` `runTickLoopWithRate`: insert at the **top** of the for-loop body, BEFORE `s.processClientsIn()`:

```go
if s.shutdownTick != -1 && s.currentTick >= s.shutdownTick {
	s.processShutdown()
	if s.shutdownGraceful {
		return // graceful exit: tick loop terminates
	}
}
```

Modify `processLogouts` (tick.go:210-) to check `p.forceRemove` and unconditionally remove the player when set.

#### 3.4.1 World runFn graceful-exit handshake

`modules/world/world.go:99-107` currently treats every `serverDone <- err` as an error. The runFn pre-flight (Risk §5-3) reads the current select block. Patch:

```go
case err := <-serverDone:
	if err != nil {
		return err
	}
	if serv.shutdownGraceful {
		return nil // ::reboot / ::slowreboot graceful exit
	}
	return fmt.Errorf("server stopped unexpectedly")
```

Modify `Server.Run()` to return `nil` when the tick loop exits via the `shutdownGraceful` path. Currently `Run()` exits when `s.quit` is closed inside `Shutdown()`; processShutdown closes `s.quit` directly, so `Run()` returns nil.

### 3.5 `processLogins` fresh-login wiring (Bundle 3)

Modify `modules/world/tick.go:99-181` `processLogins`. Inside the per-player loop, at the existing block immediately AFTER `p.input = NewInputTracking(p, s.currentTick); ... p.session = "headless"` and AFTER the LOGIN trigger fire (line 158) — actually, **before** the LOGIN trigger fire, mirroring TS order at Player.ts:494-509. Inserts:

```go
if p.reconnecting {
	onReconnect(s, p)
	continue // skip fresh-login init below
}

// Fresh-login emit order matches TS Player.onLogin (Player.ts:494-504).
// (a) UPDATE_PID — bind localPlayer to slot
sendUpdatePid(p, p.slot)
// (b) RESET_CLIENT_VARCACHE — drop client-side varp cache
sendResetClientVarCache(p)
// (c) varp transmit-loop — every transmit-true varp's server default
if s.varpTypes != nil {
	for i, vt := range s.varpTypes.Configs {
		if vt != nil && vt.Transmit {
			p.writeVarp(i, p.varps[i])
		}
	}
}
// (d) RESET_ANIMS — clear animation layers
sendResetAnims(p)

// Post-onLogin reboot-timer emit (TS World.ts:944-946).
if s.shutdownTick != -1 {
	sendUpdateRebootTimer(p, s.shutdownTick-s.currentTick)
}
```

**Ordering note:** TS fires `IF_CLOSE` (Player.ts:494) before `UPDATE_PID`. goscape's `processLogins` does not emit `IF_CLOSE` currently. Adding it here drifts beyond scope (modal-state subsystem). Defer with **DEVIATION-NAI-182-D4** — see §6.

**Ordering note 2:** TS fires `ChatFilterSettings` and `UpdateIgnoreList` BEFORE `UPDATE_PID` (Player.ts:487-491). Both opcodes are in the deferred social cluster and not yet ported. The fresh-login wiring here starts at the `UPDATE_PID` line of TS Player.onLogin and runs forward; the prior 3 lines are out of scope.

### 3.6 `onReconnect` lifecycle (Bundle 4)

New function in `modules/world/login_resync.go`:

```go
// onReconnect runs the resync sequence for a reconnecting player.
// Called from processLogins when p.reconnecting == true. Mirrors TS
// Player.onReconnect (Player.ts:516-574).
//
// DEVIATION-NAI-182-D1-RECONNECT-NO-RESTORE — goscape currently runs
// the fresh-login init in processLogins BEFORE this function (no
// save/restore subsystem yet), so the resync packets carry the
// post-fresh-init defaults rather than the player's pre-disconnect
// state. The wire ordering is TS-faithful; the data is default-valued.
// Clears when save/restore lands.
func onReconnect(s *Server, p *Player) {
	// (a) RESET_CLIENT_VARCACHE
	sendResetClientVarCache(p)

	// (b) varp transmit-loop
	if s.varpTypes != nil {
		for i, vt := range s.varpTypes.Configs {
			if vt != nil && vt.Transmit {
				p.writeVarp(i, p.varps[i])
			}
		}
	}

	// (c) buildArea clear + rebuild — already handled in goscape by the
	// p.reconnecting==true → shouldRebuild path at player.go:694. No new
	// code; rebuildNormal fires in processInfo this tick.

	// (d) reboot-timer if pending
	if s.shutdownTick != -1 {
		sendUpdateRebootTimer(p, s.shutdownTick-s.currentTick)
	}

	// (e) closeModal(false) — preserves main modal, drops chat/side
	p.CloseModal(false)

	// (f) per-tab IfSetTab resync — emit for every non-default tab slot
	for tab, com := range p.tabs {
		if com != 0 {
			p.IfSetTab(com, tab)
		}
	}

	// (g) refreshInvs — flip every invListener's FirstSeen back to true
	// so the NEXT updateInvs (later this tick or next) re-emits each as
	// UpdateInvFull. Map-value addressability dance per player.go:884-888.
	for com, l := range p.invListeners {
		l.FirstSeen = true
		p.invListeners[com] = l
	}

	// (h) per-stat UpdateStat for all 21 skills
	for i := 0; i < objtype.PlayerStatCount; i++ {
		sendUpdateStat(p, i, int(p.stats[i]), int(p.levels[i]))
	}

	// (i) UpdateRunEnergy
	sendUpdateRunEnergy(p, p.runenergy)

	// (j) RESET_ANIMS
	sendResetAnims(p)

	// (k) masks |= entitymask — resync face_entity on the next mask block
	p.masks |= p.entitymask
}
```

### 3.7 Staff-cheats (Bundle 6 — D3-narrowed)

Add to the `switch` block in `modules/world/handlers_game.go:handleClientCheat` (existing `staffModLevel >= 2` gate at the top covers all three; TS gates on `NODE_PRODUCTION` instead — see DEVIATION-NAI-182-D2 in §6):

```go
case "reboot":
	// Mirrors TS ClientCheatHandler.ts:360-364. duration=0 means
	// immediate shutdown (shutdownTick = currentTick).
	s.rebootTimer(0)

case "slowreboot":
	// Mirrors TS ClientCheatHandler.ts:365-373. Default 30 seconds when
	// args[0] is missing or unparseable (tryParseInt semantics).
	// Formula: ticks = ceil(seconds * 1000 / 600).
	seconds := 30
	if len(parts) >= 2 {
		if v, err := strconv.Atoi(strings.TrimSpace(parts[1])); err == nil {
			seconds = v
		}
	}
	ticks := int(math.Ceil(float64(seconds) * 1000.0 / 600.0))
	s.rebootTimer(ticks)

case "serverdrop":
	// Mirrors TS ClientCheatHandler.ts:374-376 player.terminate().
	// Closes the TCP conn without removing the player from s.players;
	// the next reconnect (OpReqGameReconnect) hits this player's slot
	// and runs the onReconnect path.
	if p.client != nil && p.client.conn != nil {
		_ = p.client.conn.Close()
	}
```

**Import audit at HEAD `2c00fae`:** `strconv` and `strings` already imported (lines 5-6). `math` is NOT imported — add it.

**`TestProcessShutdown_ZeroPlayersTriggersGracefulExit` clarification:** since processShutdown no longer closes `s.quit`, the test should assert (a) `s.shutdownGraceful == true` and (b) `s.quit` is still **open** (not closed). The close happens later via dskit `stoppingFn → Server.Shutdown()`.

## 4. Test plan

### 4.1 Encoder byte-pins (Bundle 1)

`modules/world/login_resync_test.go` and `modules/world/reboot_test.go`. Pattern follows `TestHintNpcPayloadBytes` at `player_script_test.go:1131` — sibling ISAAC stream + `drainConn` channel.

- `TestSendUpdatePid_EmitsExactByteSequence` — set `p.slot = 0x1234`, expect opcode-byte (encrypted) followed by `0x12, 0x34`.
- `TestSendResetClientVarCache_EmitsOpcodeOnly` — expect single encrypted opcode byte, no payload.
- `TestSendResetAnims_EmitsOpcodeOnly` — expect single encrypted opcode byte, no payload.
- `TestSendUpdateRebootTimer_EmitsExactByteSequence` — `sendUpdateRebootTimer(p, 50)` expects opcode byte + `0x00, 0x32`.
- `TestSendUpdateRebootTimer_ZeroTicks` — `sendUpdateRebootTimer(p, 0)` expects opcode byte + `0x00, 0x00`.

### 4.2 `processLogins` fresh-login wiring (Bundle 3)

`modules/world/login_resync_test.go`:

- `TestProcessLogins_FreshLogin_EmitsOpcodeOrder` — seed a fresh player with `p.slot=7`, two transmit-true varps (default values `-1` and `0`) plus one transmit-false varp; run `s.processLogins()`; assert emitted byte sequence opens with `UPDATE_PID(7)` → `RESET_CLIENT_VARCACHE` → `VARP_*(varp0,-1)` → `VARP_*(varp1,0)` → `RESET_ANIMS`. Assert the transmit-false varp is NOT emitted.
- `TestProcessLogins_FreshLogin_WithShutdownPending_EmitsRebootTimer` — set `s.shutdownTick = s.currentTick + 25`; run `s.processLogins()`; assert `UPDATE_REBOOT_TIMER(25)` lands AFTER `RESET_ANIMS`.
- `TestProcessLogins_FreshLogin_NoShutdown_NoRebootTimer` — `s.shutdownTick == -1`; assert no UPDATE_REBOOT_TIMER opcode in emitted bytes.

### 4.3 `onReconnect` lifecycle (Bundle 4)

`modules/world/reconnect_test.go`. **All state must be assigned via direct field write AFTER fresh-init has run** (per memory `test_helper_bypass_masks_production_path.md`), since DEVIATION-NAI-182-D1 means fresh-init clobbers seeded state.

- `TestOnReconnect_EmitsResyncSequence` — set `p.reconnecting=true`; after `s.processLogins()` has populated defaults, the function under test will have already run; instead drive `onReconnect(s, p)` directly OR have the test set `p.reconnecting=true` and seed defaults to match assertions. Assert opcode sequence: `RESET_CLIENT_VARCACHE` → transmit-true varps → (optional UPDATE_REBOOT_TIMER if shutdown pending) → (no opcode for closeModal — modal subsystem internal) → per non-zero tab `IF_SETTAB` → per-stat `UPDATE_STAT` × 21 → `UPDATE_RUN_ENERGY` → `RESET_ANIMS`.
- `TestOnReconnect_FlipsAllInvListenerFirstSeenToTrue` — register 3 invListeners with `FirstSeen=false`; call `onReconnect(s, p)`; assert all 3 listeners have `FirstSeen=true` after the call.
- `TestOnReconnect_OrsEntityMaskIntoMasks` — set `p.entitymask = 0x80`, `p.masks = 0x01`; call `onReconnect(s, p)`; assert `p.masks & 0x80 != 0`.
- `TestOnReconnect_WithShutdownPending_EmitsRebootTimer` — set `s.shutdownTick = s.currentTick + 100`; assert `UPDATE_REBOOT_TIMER(100)` appears BEFORE `IF_SETTAB` (TS order at Player.ts:541-547).
- `TestOnReconnect_TransmitFalseVarpsNotEmitted` — seed varps with mixed Transmit; assert opcode count for VARP_SMALL/LARGE matches count of `Transmit==true` configs only.

### 4.4 Reboot infra (Bundle 2)

`modules/world/reboot_test.go`:

- `TestNewServer_ShutdownTickDefaultsToMinusOne` — fresh `Server` from `newServer`; assert `s.shutdownTick == -1`.
- `TestRebootTimer_SetsShutdownTickAndBroadcasts` — seed 3 players in `s.playerLoop`; `s.rebootTimer(50)`; assert `s.shutdownTick == s.currentTick + 50`; assert each of the 3 player conns received `UPDATE_REBOOT_TIMER(50)`.
- `TestRebootTimer_DurationZero` — `s.rebootTimer(0)`; assert `s.shutdownTick == s.currentTick`; assert each player got `UPDATE_REBOOT_TIMER(0)`.
- `TestIsPendingShutdown_AndTicksRemaining` — pre-`rebootTimer`: `isPendingShutdown()==false`; post-`rebootTimer(50)`: `isPendingShutdown()==true`, `shutdownTicksRemaining()==50`. Tick the server 10 times; assert `shutdownTicksRemaining()==40`.

### 4.5 Shutdown consumer (Bundle 5 — D2)

`modules/world/reboot_test.go`:

- `TestProcessShutdown_TriggeredAtTopOfTick` — seed `s.shutdownTick = s.currentTick`; seed one player; run one tick iteration (extract the tick body into a test-callable helper if needed); assert `processShutdown` was entered (test sentinel — e.g., observe `p.loggingOut == true`).
- `TestProcessShutdown_MarksAllConnectedPlayersForLogout` — seed 3 players; call `s.processShutdown()` directly with `s.shutdownTick == s.currentTick`; assert all 3 have `p.loggingOut == true`.
- `TestProcessShutdown_ForceRemoveAfter1024Ticks` — seed 1 player with `loggingOut=true` but stuck; set `s.shutdownTick = s.currentTick - 1024`; call `s.processShutdown()`; assert `p.forceRemove == true`.
- `TestProcessShutdown_ZeroPlayersTriggersGracefulExit` — empty `s.playerLoop`; set `s.shutdownTick = s.currentTick`; call `s.processShutdown()`; assert `s.shutdownGraceful == true`. Do NOT assert `s.quit` is closed — closure happens later via dskit `stoppingFn → Server.Shutdown()`. Asserting closed-state here would mask the double-close-panic bug if a future refactor reintroduces `close(s.quit)` inside `processShutdown`.
- `TestProcessShutdownGate_ScheduledNotReached_NoConsume` — set `s.shutdownTick = s.currentTick + 5`; run one tick iteration; assert `processShutdown` NOT entered (`p.loggingOut` unchanged).
- `TestProcessShutdown_RunsBeforeProcessLogins` — preload `s.newPlayers` with a fresh login candidate; set `s.shutdownTick = s.currentTick`; run one tick iteration; assert the candidate did NOT graduate to `s.players` (processShutdown's exit prevented processLogins from running).

### 4.6 Staff-cheats (Bundle 6 — D3-narrowed)

`modules/world/handlers_game_test.go` — using `dispatchTeleCheat` template at line 384:

- `TestHandleClientCheat_Reboot_TriggersImmediateBroadcast` — `dispatchCheat(p, "reboot")`; assert `s.shutdownTick == s.currentTick`; assert `UPDATE_REBOOT_TIMER(0)` broadcast to all players in `s.playerLoop`.
- `TestHandleClientCheat_SlowReboot_NoArgsDefaultsTo30Seconds` — `dispatchCheat(p, "slowreboot")`; expected ticks = `ceil(30000/600) = 50`; assert `s.shutdownTick == s.currentTick + 50`; assert `UPDATE_REBOOT_TIMER(50)` broadcast.
- `TestHandleClientCheat_SlowReboot_WithSecondsArg` — `dispatchCheat(p, "slowreboot 60")`; expected ticks = `ceil(60000/600) = 100`; assert broadcast `UPDATE_REBOOT_TIMER(100)`.
- `TestHandleClientCheat_SlowReboot_NonIntegerArgFallsBackToDefault` — `dispatchCheat(p, "slowreboot abc")`; assert ticks = 50 (default-on-parse-error per TS `tryParseInt`).
- `TestHandleClientCheat_ServerDrop_ClosesConn` — record `p.client.conn.Close()` call (mock conn or sentinel); `dispatchCheat(p, "serverdrop")`; assert `Close()` was called; assert `p` is still in `s.players` (NOT removed — only the TCP conn dropped).
- `TestHandleClientCheat_RebootCheats_StaffGate` — `p.staffModLevel = 1` (below the gate); `dispatchCheat(p, "reboot")`; assert `s.shutdownTick` unchanged (gate blocked).

## 5. Risks & open premises (controller pre-flight)

Per memory `controller_preflight.md`, the controller does a 30-second grep+Read pass against HEAD before dispatching each bundle. The premises below are flagged for verification:

1. **`s.currentTick` field access from new files.** Verified — `currentTick int` at server.go:60. The new files (`login_resync.go`, `reboot.go`) live in `package world`, same as `server.go`, so direct access is legal. ✅ Verified.

2. **`processShutdown` and `processLogins` ordering inside the tick.** Spec places `processShutdown` at the **top** of the tick body, before `processClientsIn`. TS World.ts:419 also places `this.shutdown` check at the top. This pre-empts `processLogins` so new player joins don't graduate during a collapsing world. Test `TestProcessShutdown_RunsBeforeProcessLogins` pins this. **Implementer warning:** placing `processShutdown` between `processClientsIn` and `processLogins` would let the in-flight login go through one tick before being marked for logout, leaking a wire write to a doomed conn. Pin in plan.

3. **dskit Manager graceful-exit handshake.** Verified at HEAD `2c00fae`: `world.go:99-107` runFn returns `fmt.Errorf("server stopped unexpectedly")` on nil-error from `Run()`. Spec adds `s.shutdownGraceful bool` and modifies runFn to return nil when that flag is set. **Implementer task:** also ensure `Server.Run()` returns nil (not `fmt.Errorf(...)`) when the tick loop exits via the `shutdownGraceful` path. Re-grep `Run() error` body before changing.

4. **`p.reconnecting==true` semantics — fresh-init clobbers state.** goscape currently re-runs the fresh-init block (invs/skills/varps reset to defaults, LOGIN trigger fired) on every `processLogins`, regardless of `p.reconnecting`. The spec's `if p.reconnecting { onReconnect(s, p); continue }` BRANCHES BEFORE fresh-init, so reconnect callers carry over whatever per-Server state existed (slot, uid, level, x/z from `p`). **But `processLogins` calls `s.initPlayerVarps(p)` and zeroes stats/invs** unconditionally TODAY. The `continue` BEFORE those statements is the correct placement — verify in the plan by reading the full `processLogins` body. Test `TestOnReconnect_*` must skip the fresh-init path: either preload state with `p.reconnecting=true` and trust the `continue` to skip clobbering, OR call `onReconnect(s, p)` directly (preferred for unit tests).

5. **`::slowreboot` argument parsing — `tryParseInt(args[0], 30)` semantics.** TS: parses `args[0]` as integer, falls back to 30 on missing/unparseable. goscape spec uses `strconv.Atoi(strings.TrimSpace(parts[1]))` with fallback. TS `tryParseInt` lives at `src/util/TryParse.ts` (likely) — pre-flight reads the body to confirm "missing arg → 30" AND "negative number → 30" semantics, OR documents the exact divergence. The default-on-error path is the more conservative read.

6. **`Player.terminate()` TS body for `::serverdrop`.** Pre-flight: read `src/engine/entity/Player.ts` for `terminate()`. Spec claim: "close TCP conn, leave p.reconnecting=true for next login". If TS also calls `addSessionLog` or marks the player for save, the spec needs adjustment. Read before B6 dispatch.

7. **Varp resync on fresh login — broad fixture impact.** The new transmit-loop in `processLogins` adds N varp packets to every fresh-login byte stream (N = count of `Transmit==true` configs in the test fixture's varp table). Existing login-byte-pin tests will need their expected output extended OR the new code path needs a defensive `if len(s.varpTypes.Configs) == 0 { skip }` to keep test fixtures stable. **Plan-author must enumerate** all login-byte-pin sites at HEAD before dispatch — grep `processLogins\(\)` callers in tests:

   ```
   modules/world/server_test.go:457,492
   modules/world/tick_test.go:19
   modules/world/tick_logins_test.go:34,49
   ```

   Each site may need fixture-side adjustment if it asserts byte-equality of post-login emission. **Per memory `enumerate_all_sites.md`**, the plan must list every callsite explicitly.

8. **`p.invListeners[com].FirstSeen` map-value addressability.** Read-modify-write idiom per player.go:884-888. The reconnect path's loop is shown explicitly in §3.6(g). Verify in plan code-block before dispatch.

9. **TS `Player.onLogin` line-486 `IF_CLOSE` emit not yet wired.** DEVIATION-NAI-182-D4 declared in §6. The TS line is `this.write(new IfClose());` immediately before `UpdatePid`. Modal-state subsystem in goscape exists (`p.CloseModal`) but does not emit `IF_CLOSE` to wire on its own (it sets internal modal slots; wire emit happens via the existing chat-modal close path). Leave OUT of the fresh-login emit sequence. Deviation tag tracks for future port.

10. **`p.tabs` array iteration — zero vs non-zero check.** Spec assumes `p.tabs[tab] == 0` means "no tab assigned" (skip emit). Verify by reading `modules/world/player.go` for the `tabs` field declaration to confirm initial value 0 and the convention. If goscape uses `-1` or another sentinel, adjust the `if com != 0` guard.

11. **`processLogouts` force-removal branch wiring.** Bundle 5 adds `p.forceRemove bool` and modifies `processLogouts` to act on it. Pre-flight reads `tick.go:210-260` (`processLogouts` body) to find the right injection point. The existing `force := false` at line 217 is reused; new branch: `if p.forceRemove { force = true }` near the top of the per-player loop.

## 6. Deviations

- **DEVIATION-NAI-182-D1-RECONNECT-NO-RESTORE** — `onReconnect` emits resync packets containing post-fresh-init defaults rather than restored save state. Cause: no save/restore subsystem yet. Wire ordering is TS-faithful; data is default-valued. Retires when PlayerLoading lands.
- **DEVIATION-NAI-182-D2-CHEAT-NODE-PRODUCTION-GATE** — TS gates `::reboot` / `::slowreboot` on `Environment.NODE_PRODUCTION`. goscape uses the existing `staffModLevel >= 2` gate (stricter — no test-env bypass). Practical impact: production-gated TS cheats run for any staff-level-2 user; goscape's gate is by user role, not deployment env. Retires when goscape gets a deployment-env concept.
- **DEVIATION-NAI-182-D3-OTHER-CHEATS** — 25 TS `ClientCheatHandler` cheats remain unported: `reload`, `rebuild`, `speed`, `fly`, `naive`, `random`, `setvarother`, `getvar`, `getvarother`, `give`, `giveother`, `givecrap`, `givemany`, `broadcast`, `teleother`, `setstat`, `advancestat`, `minme`, `locadd`, `npcadd`, `openmain`, `snapshot`, `teleto`, `setvis`, `ban`, `mute`, `kick`. Each touches an unrelated subsystem. Listed by name as candidate-marker per memory `stub_deferred_comment_marker.md`.
- **DEVIATION-NAI-182-D4-IFCLOSE-LOGIN-NOT-EMITTED** — TS `Player.onLogin` (Player.ts:494) emits `new IfClose()` immediately before `UpdatePid`. goscape's fresh-login sequence omits this opcode. Modal subsystem boundary; tracker for IF_CLOSE login-emit port.
- **DEVIATION-NAI-182-D5-SOCIAL-CLUSTER-PRE-PID-NOT-EMITTED** — TS `Player.onLogin` emits `ChatFilterSettings` and (no-friend-server branch) `UpdateIgnoreList` BEFORE `UpdatePid`. Both opcodes are in the deferred social cluster (separate sub-spec). The goscape login sequence starts at the `UpdatePid` line of TS Player.onLogin.

## 7. Bundle ordering (for plan-author)

Plan-author drafts a per-bundle plan doc following the memory pattern at `plan_runnable_test_fixtures.md` (every code-block in the plan must be runnable as-is, including imports). Order:

- **B0 — Opcode declarations** (§3.1). 4 `Op` entries in `pkg/io/protocol/game/server/prot.go`. No callers; compiles clean. Pure data; no test needed (existing pattern at NAI-181).
- **B1 — Encoders** (§3.2). 4 send-functions + 5 byte-pin tests (§4.1). No callers yet.
- **B2 — Reboot infra** (§3.3). `s.shutdownTick` field + init, `rebootTimer` broadcaster, `isPendingShutdown` / `shutdownTicksRemaining` getters. Tests §4.4.
- **B3 — `processLogins` fresh-login wiring** (§3.5). UPDATE_PID + RESET_CLIENT_VARCACHE + varp transmit-loop + RESET_ANIMS + reboot-timer emit. **Risk §5-7 fixture audit MUST run BEFORE dispatch.** Tests §4.2.
- **B4 — `onReconnect` lifecycle** (§3.6). Branch in processLogins; new `onReconnect(s, p)` function. Tests §4.3.
- **B5 — Shutdown consumer (D2)** (§3.4). `processShutdown` + tick-body wiring + `p.forceRemove` field + `s.shutdownGraceful` field + world.go runFn handshake + processLogouts force-branch. Tests §4.5.
- **B6 — Staff-cheats (D3-narrowed)** (§3.7). 3 new switch arms in `handleClientCheat`. Tests §4.6.

Each bundle should commit independently with `feat(world): NAI-182 B<N> — <title>` per repo convention (recent NAI commits at `git log --oneline -20`).

## 8. Close-time memory entries

To save at NAI-182 close (per memory `nai_followups.md` and `post_task_handoff.md`):

- **Fresh-login varp-resync fixture impact** — once B3 lands, every test that drives `processLogins()` and asserts byte equality must extend its expected sequence with the transmit-true varps from the fixture's varp table. Worth a dedicated memory entry: "login-byte-pin tests must enumerate varp emits".
- **`processShutdown` ordering invariant** — `processShutdown` MUST run before `processLogins`; reversing the order leaks one tick of activity to a doomed conn. Plan-time grep target: `tick.go` for-loop body.
- **`Server.shutdownGraceful` and the world.go runFn handshake** — non-obvious because the existing runFn treats nil-error from `Run()` as failure. Future readers reading the patched `case err := <-serverDone` branch may revert without this context.

If smoke or implementer steps surface NAI-182-adjacent TS-fidelity divergences, route in-scope-stretch (≤30 LOC) per memory `smoke_surfaces_adjacent_divergences.md`, else NAI-183.
