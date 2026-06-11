package world

import (
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/zsrv/goscape/pkg/coordgrid"
	entitypkg "github.com/zsrv/goscape/pkg/entity"
	"github.com/zsrv/goscape/pkg/inventory"
	"github.com/zsrv/goscape/pkg/io/packet"
	gameserver "github.com/zsrv/goscape/pkg/io/protocol/game/server"
	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/rsbuf"
	"github.com/zsrv/goscape/pkg/script"
)

// playerQueueRequest is one queued fresh-run script request carrying its
// caller-supplied parallel arg slices (IntArgs + StringArgs). Queue
// entries are processed in processPlayerQueue; when Delay reaches zero
// (or below) the target script runs as a brand-new ScriptState. Type
// selects the queue variant (NORMAL/WEAK/LONG/STRONG); STRONG fires
// even when the player is delayed, the others wait for idle.
//
// As of S6h, Script holds the pre-resolved *ScriptFile directly. ID →
// ScriptFile resolution happens at enqueue time via Player.EnqueueScriptArgs;
// engine-dispatch paths (e.g. changeStat) use Player.EnqueueScriptFile.
//
// As of NAI-26 Bundle 1, the single IntArg int field is widened to
// parallel IntArgs []int + StringArgs []string slices to match the TS
// PlayerQueueRequest.args ScriptArgument[] shape (TS
// Engine-TS/src/engine/entity/PlayerQueueRequest.ts:15). The widening
// is required for STRONGQUEUE's variadic popScriptArgs body
// (PlayerOps.ts:98) and LONGQUEUE's 2-element [logoutAction, arg]
// args array (PlayerOps.ts:179), neither of which fit a single-int field.
type playerQueueRequest struct {
	Script     *script.ScriptFile
	Delay      int
	IntArgs    []int
	StringArgs []string
	Type       script.PlayerQueueType
}

// SetDelayed marks the player as suspended for `ticks` ticks starting
// next tick, per the P_DELAY opcode contract: the player resumes at
// currentTick + 1 + ticks.
//
// No-op if the player is not wired to a server (e.g. in fixtures that
// create a player without calling newTestServer + wiring).
func (p *Player) SetDelayed(ticks int) {
	if p.client == nil || p.client.server == nil {
		return
	}
	p.delayed = true
	p.delayedUntil = p.client.server.currentTick + 1 + ticks
}

// EnqueueScriptFile appends a queued fresh-run request for a specific
// ScriptFile. Delay=0 fires on the next processPlayerQueue pass (subject
// to the STRONG/NORMAL gate). Nil sf is a silent no-op — engine
// dispatchers (e.g. changeStat) call GetByTrigger and may legitimately
// pass nil when no cache script is registered for the event.
//
// intArgs and stringArgs are the parallel-slice args the target script
// will read from its IntArgCount / StringArgCount-sized prelude slots
// (matches TS ScriptArgument[] shape per
// Engine-TS/src/engine/entity/PlayerQueueRequest.ts:15). nil/nil
// expresses "no args" — the TS-faithful default for engine-dispatch
// paths (TS Engine-TS/src/engine/entity/Player.ts:821 args=[] default).
func (p *Player) EnqueueScriptFile(sf *script.ScriptFile, delay int, intArgs []int, stringArgs []string, qtype script.PlayerQueueType) {
	if sf == nil {
		return
	}
	req := playerQueueRequest{
		Script:     sf,
		Delay:      delay,
		IntArgs:    intArgs,
		StringArgs: stringArgs,
		Type:       qtype,
	}
	if qtype == script.QueueEngine {
		// NAI-144: TS Player.ts:823-826 — ENGINE entries land in the
		// separate engineQueue; processPlayerEngineQueues drains them
		// between processPlayerTimers and processPathing.
		p.engineQueue = append(p.engineQueue, req)
		return
	}
	p.queue = append(p.queue, req)
}

// clearWeakQueue removes every QueueWeak entry from p.queue, preserving
// relative order of remaining entries. Mirrors TS
// Player.weakQueue.clear() (Player.ts:743). Goscape unifies all queue
// types into p.queue with a Type discriminator, so "clear weak queue"
// becomes a filter on the Type field.
func (p *Player) clearWeakQueue() {
	out := p.queue[:0]
	for _, req := range p.queue {
		if req.Type != script.QueueWeak {
			out = append(out, req)
		}
	}
	p.queue = out
}

// UnlinkQueuedScript removes every p.queue entry whose Script resolves
// to the script at scriptID (default-NORMAL TS arm). Walks the entire
// p.queue regardless of Type discriminator — this matches TS
// unlinkQueuedScript's default branch which walks both `queue` and
// `weakQueue` (Player.ts:843-851). p.engineQueue is intentionally
// untouched: TS gates engineQueue iteration behind type=ENGINE, which
// CLEARQUEUE never passes (the only consumer at this time).
//
// No-op when scriptID does not resolve to a registered script (zero
// possible matches — TS iterates and finds nothing in the same
// scenario). Goscape matches by `req.Script == target` pointer-equality
// after a single provider lookup; pointer stability holds because
// Provider.scripts is append-only (provider.go).
//
// (goscape defensive; TS skips this check) The nil-server guard mirrors
// EnqueueScriptArgs at player_script.go:127 — load-bearing for test
// fixtures that don't wire a Server.
//
// Mirrors TS Player.unlinkQueuedScript(scriptId, type=NORMAL) at
// Engine-TS/src/engine/entity/Player.ts:833-852. NAI-161 T1.
func (p *Player) UnlinkQueuedScript(scriptID int) {
	if p.client == nil || p.client.server == nil || p.client.server.scriptProvider == nil {
		return
	}
	target := p.client.server.scriptProvider.GetByID(uint32(scriptID))
	if target == nil {
		return
	}
	out := p.queue[:0]
	for _, req := range p.queue {
		if req.Script != target {
			out = append(out, req)
		}
	}
	p.queue = out
}

// QueueCount returns the number of p.queue entries whose Script
// resolves to the script at scriptID. Mirrors TS GETQUEUE at
// PlayerOps.ts:915-930 (pin 9aadcec4) which walks BOTH
// `state.activePlayer.queue` AND `state.activePlayer.weakQueue` via
// head()/next() iteration, counting any match in either. Goscape's
// unified p.queue holds Normal/Strong/Long/Weak entries, so a single
// loop over p.queue covers both TS queues. p.engineQueue is a separate
// slice and is intentionally excluded.
//
// (goscape defensive; TS skips this check) See UnlinkQueuedScript.
//
// NAI-161 T2.
func (p *Player) QueueCount(scriptID int) int {
	if p.client == nil || p.client.server == nil || p.client.server.scriptProvider == nil {
		return 0
	}
	target := p.client.server.scriptProvider.GetByID(uint32(scriptID))
	if target == nil {
		return 0
	}
	n := 0
	for _, req := range p.queue {
		if req.Script == target {
			n++
		}
	}
	return n
}

// EnqueueScriptArgs implements script.ActivePlayer.EnqueueScriptArgs by
// resolving scriptID → *ScriptFile via scriptProvider.GetByID and
// delegating to EnqueueScriptFile. Returns a non-nil error when the
// scriptID does not resolve to a registered script — mirrors TS
// PlayerOps.ts:103-105 throw shape ("Unable to find queue script: ${id}").
//
// NAI-26 Bundle 2: this implementation now returns a non-nil error
// when GetByID returns nil — TS-faithful to PlayerOps.ts:103-105
// (and the parallel sites in :127-129, :152-154, :175-177). Bundle 1
// shipped a placeholder body returning nil; the rollout of the error
// activation was deferred to Bundle 2 to keep the mechanical signature
// widening separate from the behavior change for review-surface
// isolation.
//
// Silent no-op on unwired server (p.client / p.client.server /
// p.client.server.scriptProvider nil) is preserved across both bundles
// — that path corresponds to test fixtures that don't wire a Server,
// not to a script-author error worth surfacing.
//
// Resolution shifts from fire-time (pre-S6h) to enqueue-time (S6h).
// Same tick boundary in practice; simpler codepath.
func (p *Player) EnqueueScriptArgs(scriptID uint32, delay int, intArgs []int, stringArgs []string, qtype script.PlayerQueueType) error {
	if p.client == nil || p.client.server == nil || p.client.server.scriptProvider == nil {
		return nil
	}
	sf := p.client.server.scriptProvider.GetByID(scriptID)
	if sf == nil {
		// NAI-26 Bundle 2: surfaces script-author errors that pre-NAI-26
		// silent-no-op masked. Mirrors TS PlayerOps.ts:103-105
		// (STRONGQUEUE), :127-129 (WEAKQUEUE), :152-154 (QUEUE),
		// :175-177 (LONGQUEUE) — all four queue handlers throw
		// `Unable to find queue script: ${scriptId}` when the
		// scriptProvider lookup fails.
		return fmt.Errorf("unable to find queue script: %d", scriptID)
	}
	p.EnqueueScriptFile(sf, delay, intArgs, stringArgs, qtype)
	return nil
}

// StoreActiveScript saves a Suspended ScriptState so the tick loop can
// resume it when the player's delay expires.
func (p *Player) StoreActiveScript(state *script.ScriptState) {
	p.activeScript = state
}

// ClearActiveScript discards any stored ScriptState. Called after
// Finished/Aborted runs and on logout cleanup.
func (p *Player) ClearActiveScript() {
	p.activeScript = nil
}

// OnScriptFinishedOrAborted handles the Finished/Aborted post-Execute
// tail for a player-anchored script. If state is the player's current
// activeScript, nulls it (and the pending resumeButtons); and if no MAIN
// modal is open, fires CloseModal(false) to auto-close any open chat /
// side dialogue.
//
// Mirrors TS Player.executeScript Finished/Aborted tail
// (Player.ts:2224-2231 @2e3bcf43, resumeButtons clear from 2dc4a811):
//
//	} else if (script === this.activeScript) {
//	    this.activeScript = null;
//	    this.resumeButtons = [];
//	    if ((this.modalState & ModalState.MAIN) === ModalState.NONE) {
//	        // close chat dialogues automatically and leave main modals alone
//	        this.closeModal(false);
//	    }
//	}
//
// The match-guard preserves a Suspended / PauseButton / CountDialog
// activeScript when a different fresh script Finishes on the same player
// in the same tick.
//
// NAI-54 T1; A9 resumeButtons.
func (p *Player) OnScriptFinishedOrAborted(state *script.ScriptState) {
	if p.activeScript != state {
		return
	}
	p.activeScript = nil
	p.resumeButtons = nil
	if p.modalState&modalStateMain == modalStateNone {
		p.CloseModal(false)
	}
}

// Playtime implements script.ActivePlayer.Playtime. The playtime field
// is incremented in processIn each tick.
func (p *Player) Playtime() int { return int(p.playtime) }

// LastMovement returns the player's lastMovement field. See the
// pkg/script.ActivePlayer.LastMovement docstring for semantics.
func (p *Player) LastMovement() int { return p.lastMovement }

// S5m: last-input queries. Return the matching Player field.
func (p *Player) LastItem() int       { return p.lastItem }
func (p *Player) LastSlot() int       { return p.lastSlot }
func (p *Player) LastUseItem() int    { return p.lastUseItem }
func (p *Player) LastUseSlot() int    { return p.lastUseSlot }
func (p *Player) LastTargetSlot() int { return p.lastTargetSlot }

// CamReset sends an OpCamReset wire packet to the client. Called by
// the CAM_RESET script opcode (e.g. from the LOGIN script).
func (p *Player) CamReset() {
	p.writeOut(gameserver.OpCamReset, nil)
}

// CamShake sends a CAM_SHAKE wire packet (TS ServerGameProt.CAM_SHAKE
// = 13, payload p1×4 = axis, random, amplitude, rate). Direct-write;
// does NOT route through the cameraPackets accumulator (TS PlayerOps.ts:223
// is `state.activePlayer.write(new CamShake(...))`, no accumulator).
// Called by the CAM_SHAKE (opcode 2010) script handler. Mirrors TS
// CamShakeEncoder.ts:9-14.
func (p *Player) CamShake(axis, random, amplitude, rate int) {
	p.writeOut(gameserver.OpCamShake, []byte{
		byte(axis), byte(random), byte(amplitude), byte(rate),
	})
}

// CamMoveTo appends a kind=0 cameraInfo onto p.cameraPackets. The packet
// is drained at the top of updateBuildArea (TS NetworkPlayer.ts:244-253);
// (camX, camZ) is converted to (localX, localZ) at drain-time using
// p.originX/p.originZ. Mirrors TS PlayerOps.ts:213-218.
func (p *Player) CamMoveTo(camX, camZ, height, rate, rate2 int) {
	p.cameraPackets = append(p.cameraPackets, cameraInfo{
		kind: 0, camX: camX, camZ: camZ,
		height: height, rotationSpeed: rate, rotationMultiplier: rate2,
	})
}

// CamLookAt appends a kind=1 cameraInfo. Same drain semantics as CamMoveTo.
// Mirrors TS PlayerOps.ts:206-211.
func (p *Player) CamLookAt(camX, camZ, height, rate, rate2 int) {
	p.cameraPackets = append(p.cameraPackets, cameraInfo{
		kind: 1, camX: camX, camZ: camZ,
		height: height, rotationSpeed: rate, rotationMultiplier: rate2,
	})
}

// HintNpc sends a HINT_ARROW (type=1, NPC variant) wire packet to the
// client. Encodes 6 bytes matching TS HintArrowEncoder type=1 branch:
// p1(type=1), p2(nid), p2(0), p1(0). Called by the HINT_NPC (opcode
// 2028) script handler. Mirrors TS Player.hintNpc at Player.ts:2174-2176.
//
// Sibling encoder branches: (*Player).HintCoord (type=2..6, NAI-39),
// (*Player).HintPlayer (type=10, NAI-39), (*Player).HintStop
// (type=-1, NAI-39). Closes the partial-encoder follow-up from NAI-37.
func (p *Player) HintNpc(nid int) {
	payload := []byte{
		0x01,                      // p1: type = 1 (NPC hint)
		byte(nid >> 8), byte(nid), // p2: nid (big-endian)
		0x00, 0x00, // p2: 0 (unused playerSlot for type=1)
		0x00, // p1: 0 (unused y for type=1)
	}
	p.writeOut(gameserver.OpHintArrow, payload)
}

// HintCoord sends a HINT_ARROW (type=2..6, TILE variant) wire packet to
// the client. Encodes 6 bytes matching TS HintArrowEncoder type=2..6
// branch (HintArrowEncoder.ts:17-27): p1(offset), p2(x), p2(z),
// p1(height). Called by the HINT_COORD (opcode 2027) script handler.
// Mirrors TS Player.hintTile at Player.ts:2178-2180.
//
// Out-of-range offset (not in [2,6]) is TS-faithful: the wire packet
// is emitted with the offset as byte[0]. Script-authors are responsible
// for offset bounds; the entity-method does not validate.
func (p *Player) HintCoord(offset, x, z, height int) {
	payload := []byte{
		byte(offset),          // p1: type = offset (2..6)
		byte(x >> 8), byte(x), // p2: x (big-endian)
		byte(z >> 8), byte(z), // p2: z (big-endian)
		byte(height), // p1: height
	}
	p.writeOut(gameserver.OpHintArrow, payload)
}

// HintPlayer sends a HINT_ARROW (type=10, PL variant) wire packet to
// the client. Encodes 6 bytes matching TS HintArrowEncoder type=10
// branch (HintArrowEncoder.ts:28-32): p1(10), p2(target), p2(0), p1(0).
// Called by the HINT_PL (opcode 2029) script handler with the target
// player's slot (TS PlayerOps HINT_PLAYER passes player.slot). Mirrors
// TS Player.hintPlayer(target) at Player.ts:2215-2217 @2e3bcf43.
func (p *Player) HintPlayer(target int) {
	payload := []byte{
		0x0A,                            // p1: type = 10 (player hint)
		byte(target >> 8), byte(target), // p2: target player slot (big-endian)
		0x00, 0x00, // p2: 0
		0x00, // p1: 0
	}
	p.writeOut(gameserver.OpHintArrow, payload)
}

// HintStop sends a HINT_ARROW (type=-1, STOP variant) wire packet to
// the client, clearing any active hint arrow. Encodes 6 bytes matching
// TS HintArrowEncoder type=-1 branch (HintArrowEncoder.ts:33-38):
// p1(-1), p2(0), p2(0), p1(0). p1(-1) on the wire is 0xFF (low byte of
// two's-complement). Called by the HINT_STOP (opcode 2030) script
// handler. Mirrors TS Player.stopHint at Player.ts:2186-2188.
func (p *Player) HintStop() {
	payload := []byte{
		0xFF,       // p1: type = -1 (stop sentinel; two's-complement low byte)
		0x00, 0x00, // p2: 0
		0x00, 0x00, // p2: 0
		0x00, // p1: 0
	}
	p.writeOut(gameserver.OpHintArrow, payload)
}

// StaffModLevel is provided by player_source.go (returns int32 per
// rsbuf.PlayerSource). Re-used here to satisfy script.ActivePlayer.

// UID implements script.ActivePlayer.UID. Returns the persistent
// account uid captured during login.
func (p *Player) UID() int { return p.uid }

// AccountID implements script.ActivePlayer.AccountID. Returns the
// persistent DB account.id (int64) from the PlayerLogin RPC, used as
// the partition key on every telemetry envelope. Zero for connections
// that bypass the login bridge (standalone world, unit tests).
// NAI-Phase2 backfill of telemetry account_id.
func (p *Player) AccountID() int64 { return p.accountID }

// RecipientSession implements script.ActivePlayer.RecipientSession.
// Returns this player's per-login session UUID ('headless' when none —
// the TS Player.session field default, Player.ts:311 @2e3bcf43). Used
// when this player is the COUNTERPARTY of a wealth event. Mirrors TS
// `recipient_session: toPlayer.session` (InvOps.ts:454/489/707
// @2e3bcf43). rev-254 A3: the 244-era isClientConnected /
// 'disconnected' fork was deleted upstream together with the
// NetworkPlayer addWealthEvent override.
func (p *Player) RecipientSession() string {
	return p.sessionOrHeadless()
}

// CanAccess implements script.ActivePlayer.CanAccess — the P_FINDUID
// protected-binding gate. False when delayed, when a modal main/chat
// is open, or when a protected script is stored on activeScript.
// Mirrors TS Player.canAccess at Engine-TS/src/engine/entity/Player.ts:805-812.
//
// World-shutdown relaxation (TS L806-808): once the world has reached
// its shutdown tick, no protection rules apply — every player becomes
// reachable so the shutdown drain can complete (handler resolutions,
// teleport-to-spawn, etc. must not be blocked by a stuck modal or a
// protected script). Mirrors `if (World.shutdown) return true`. Nil
// guards on p.client / p.client.server tolerate the bare-Player test
// fixtures that don't wire a server.
//
// The third branch reads activeScript.Pointers&PtrProtectedActivePlayer
// to answer TS's `!this.protect` gate. The mapping holds because
// goscape's StoreActiveScript (player_script.go:138-140) preserves
// Pointers across suspensions — so "is a protected script stored on
// the player?" and TS's persisted `this.protect` bool produce
// identical observable behavior across the canAccess + walktrigger
// gates. See DEVIATION-NAI-111-D1 on CloseModal for the full
// narrowed-convergence rationale.
func (p *Player) CanAccess() bool {
	if p.client != nil && p.client.server != nil && p.client.server.shutdown() {
		return true
	}
	if p.delayed {
		return false
	}
	if p.modalState&(modalStateMain|modalStateChat) != 0 {
		return false
	}
	if p.protectedScriptActive() {
		return false
	}
	return true
}

// protectedScriptActive reports whether the player currently has a
// protected script executing or suspended. Thin wrapper over p.protect,
// matching TS Player.protect exactly (Player.ts:359). See the protect-
// field doc-comment in player.go for the full set/clear lifecycle.
//
// Kept as a method (instead of inlining `p.protect` at call sites) so
// the gate has a discoverable name in interaction_debug.go's audit log
// + script.go's runScript entry guard. CanAccess + processWalktrigger
// also read this for symmetry with TS Player.ts:810,1062's `!this.protect`
// gate shape.
//
// NAI-111-D1 closure: pre-refactor this read
// activeScript.Pointers&PtrProtectedActivePlayer, conflating the
// Player gate with the script-state pointerCheck source — which
// blocked TS-faithful CloseModal semantics (clearing the gate would
// also strip the handler pointerCheck and re-trigger NAI-53 T3).
// Split into a dedicated Player.protect field that CloseModal can
// clear independently of state.Pointers.
func (p *Player) protectedScriptActive() bool {
	return p.protect
}

// Varp implements script.ActivePlayer.Varp.
func (p *Player) Varp(id int) int32 {
	if id < 0 || id >= len(p.varps) {
		return 0
	}
	return p.varps[id]
}

// SetVarp implements script.ActivePlayer.SetVarp. Writes the server-
// side value then wire-sends via VARP_SMALL / VARP_LARGE if the varp
// type is transmit=true.
func (p *Player) SetVarp(id int, val int32) {
	if id < 0 || id >= len(p.varps) {
		return
	}
	p.varps[id] = val
	p.writeVarp(id, val)
}

// VarpString implements script.ActivePlayer.VarpString. Returns the
// STRING-typed per-player var at id, or "" on OOB / unsized slice.
func (p *Player) VarpString(id int) string {
	if id < 0 || id >= len(p.varpsString) {
		return ""
	}
	return p.varpsString[id]
}

// SetVarpString implements script.ActivePlayer.SetVarpString. OOB
// silently dropped (slice sized to varpTypes.Configs at login). No
// wire-send: this protocol revision has no varp_string opcode.
func (p *Player) SetVarpString(id int, val string) {
	if id < 0 || id >= len(p.varpsString) {
		return
	}
	p.varpsString[id] = val
}

// varbitMask returns the bit-range mask for a configured varbit, or
// ok=false for unconfigured/garbage ranges.
//
// PORTING-EXCEPTION (varbit-unconfigured-guard): TS getVarBit/setVarBit
// index Packet.bitmask[endbit - startbit + 1] unguarded — a varbit that
// only carries a debugname (code-1 fields left at their -1 initializers)
// would compute bitmask[1] then shift by -1, yielding JS garbage. Go
// panics on negative shifts and OOB indexing, so guard here: GetVarBit
// reads 0 and SetVarBit no-ops for such varbits. Content never ships
// debugname-only varbits; the guard is unreachable on real caches.
func varbitMask(vb *objtype.VarBitType) (mask int32, ok bool) {
	bits := vb.Endbit - vb.Startbit + 1
	if vb.Startbit < 0 || bits < 0 || bits >= len(packet.Bitmask) {
		return 0, false
	}
	return int32(packet.Bitmask[bits]), true
}

// GetVarBit reads the varbit's bit-range out of its base varp.
// TS Player.ts:1750-1760 @43e02957. Reads through p.Varp so an OOB
// basevar yields 0 — the int32 analog of TS's undefined>>n === 0.
func (p *Player) GetVarBit(id int) int32 {
	vb := p.varbitTypeConfig(id)
	if vb == nil {
		return 0
	}
	mask, ok := varbitMask(vb)
	if !ok {
		return 0
	}
	return p.Varp(vb.Basevar) >> vb.Startbit & mask
}

// SetVarBit writes value into the varbit's bit-range of its base varp,
// preserving the other bits. Out-of-range values write 0 (TS clamps to
// 0, NOT to mask — Player.ts:1771-1773). Routes through SetVarp so the
// VARP_SMALL/LARGE resync fires for transmit varps (TS routes through
// this.setVar at Player.ts:1776). TS Player.ts:1762-1777 @43e02957.
//
// The composed expression mirrors TS
// `mask & value << startbit | this.vars[basevar] & ~mask` with JS
// precedence made explicit: (mask & (value << startbit)) |
// (vars & ^mask). The clamp compares against the unshifted mask as a
// JS number (uint32-wide, so an endbit-startbit+1 == 32 varbit clamps
// exactly like TS), while the bit math runs in int32 like JS's |0
// coercion.
func (p *Player) SetVarBit(id int, value int32) {
	vb := p.varbitTypeConfig(id)
	if vb == nil {
		return
	}
	mask, ok := varbitMask(vb)
	if !ok {
		return
	}
	if int64(value) < 0 || int64(value) > int64(uint32(mask)) {
		value = 0
	}
	mask <<= vb.Startbit
	p.SetVarp(vb.Basevar, mask&(value<<vb.Startbit)|p.Varp(vb.Basevar)&^mask)
}

// SetRun implements script.ActivePlayer.SetRun. Writes the run-mode
// toggle (0=walk, 1=run) to the player's run field. Mirrors TS field
// write at PlayerOps.ts:1205. Backs the P_RUN opcode handler. NAI-117.
func (p *Player) SetRun(v int) {
	p.run = v
}

// Walk implements script.ActivePlayer.Walk. Runs the server pathfinder
// at the player's current level via the s.pathfinder() test seam and
// replaces the waypoint queue with the result. Mirrors TS
// Player.queueWaypoints(findPath(player.level, player.x, player.z,
// destX, destZ)). No-op when no client/server/pathfinder is wired
// (test fixtures without a real or injected pathfinder).
func (p *Player) Walk(destX, destZ int) {
	if p.client == nil || p.client.server == nil {
		return
	}
	pf := p.client.server.pathfinder()
	if pf == nil {
		return
	}
	route := pf.FindPathPlain(p.level, p.x, p.z, destX, destZ)
	p.queueWaypoints(routeToPacked(route))
}

// RunVarpID implements script.ActivePlayer.RunVarpID. Returns the varp
// id discovered at config-load time as the engine run-mode varp (the
// config with ClientCode==7). Mirrors TS VarPlayerType.RUN at
// Engine-TS/src/cache/config/VarPlayerType.ts:50-53. Returns 0 (goscape
// defensive; TS skips this check) if the server has no varpTypes loaded
// (test-fixture / pre-config-load).
func (p *Player) RunVarpID() int {
	if p.client == nil || p.client.server == nil || p.client.server.varpTypes == nil {
		return 0
	}
	return p.client.server.varpTypes.RunID
}

// RunEnergy implements script.ActivePlayer.RunEnergy. Returns the
// player's current run-energy as an int (range [0, 10000]). Backs the
// RUNENERGY opcode handler. NAI-117.
func (p *Player) RunEnergy() int {
	return p.runenergy
}

// RunWeight returns the cached carry weight in grams.
func (p *Player) RunWeight() int { return p.runweight }

// AfkEventReady reports the AFK-event ready flag.
func (p *Player) AfkEventReady() bool { return p.afkEventReady }

// SetAfkEventReady writes the AFK-event ready flag (cleared by AFK_EVENT
// after dispatch).
func (p *Player) SetAfkEventReady(v bool) { p.afkEventReady = v }

// SetRunEnergy writes the current run-energy value. Caller clamps; this
// method does no validation.
func (p *Player) SetRunEnergy(v int) { p.runenergy = v }

// S5c: position / facing / teleport, stats, and animation.

// CoordPacked returns the player's current position as a single RS2 coord
// int: (level<<28) | (x<<14) | z. Used by the COORD opcode.
func (p *Player) CoordPacked() int {
	return (p.level << 28) | (p.x << 14) | p.z
}

// TeleJump instantly teleports the player to (x, z, level) with no
// interpolation, clearing any pending walk. ResetMasks clears the one-
// shot tele/jump flags after emission.
func (p *Player) TeleJump(x, z, level int) {
	prevX, prevZ, prevLevel := p.x, p.z, p.level
	p.x = x
	p.z = z
	p.level = level
	p.tele = true
	p.jump = true
	// Full zone presence (collision-follow + zone swap): TS teleJump
	// routes through teleport → refreshZonePresence (PathingEntity.ts:
	// 283-287, 312).
	refreshPlayerZonePresence(p, prevX, prevZ, prevLevel)
	// rev-254 (f0ccbe8a): refreshPlayerZonePresence now writes lastStepX/Z =
	// prev unconditionally; TS teleJump → teleport overwrites with x-1/z
	// (PathingEntity.ts:313-314) after the refresh, so mirror that tail here
	// (closes the prior TeleJump lastStep skip, which left a stale value).
	p.lastStepX = p.x - 1
	p.lastStepZ = p.z
}

// Teleport moves the player to (x, z, level) and flags the client for a
// smooth teleport transition (tele without jump in the same-level case;
// tele+jump+INSTANT speed when crossing levels). Mirrors TS
// PathingEntity.teleport at PathingEntity.ts:267-298.
//
// NAI-36-T7 closed D1 (level clamp), D2 (unallocated-zone reject), order
// (refresh BEFORE tele=true), and D5 (level-change INSTANT + jump branch)
// for Player. NAI-65 closed D3-Player (focus call) and D4-Player
// (lastStepX = x-1; lastStepZ = z). See DEVIATION block at npc_script.go
// for the full tracker; D4-NPC, D5-NPC, and NAI-41 remain residual.
func (p *Player) Teleport(x, z, level int) {
	// D1: clamp level to [0, 3] per PathingEntity.ts:268-271.
	if level < 0 {
		level = 0
	} else if level > 3 {
		level = 3
	}
	// D2: reject teleports to unallocated zones per PathingEntity.ts:273-278.
	// (TS additionally exempts staffModLevel >= 3; goscape has no staff-mod
	// flag yet, so the gate is unconditional. messageGame on rejection is
	// a future polish item.)
	if p.client != nil && p.client.server != nil &&
		!p.client.server.IsZoneAllocated(level, x, z) {
		return
	}

	prevX, prevZ, prevLevel := p.x, p.z, p.level
	p.x = x
	p.z = z
	p.level = level

	// NAI-65 D3-Player: focus call from TS PathingEntity.ts:286-289.
	// Player width=length=1 (no struct field; PathingEntity-default).
	dir := coordgrid.Face(prevX, prevZ, x, z)
	moveX := coordgrid.MoveX(p.x, dir)
	moveZ := coordgrid.MoveZ(p.z, dir)
	p.focus(coordgrid.Fine(moveX, 1), coordgrid.Fine(moveZ, 1), false)

	// Order: refreshPlayerZonePresence runs BEFORE p.tele = true to match
	// TS PathingEntity.ts:290-293. The two writes are functionally
	// commutative (refresh reads only previous coords + current
	// x/z/level; the tele bit is independent), but TS-faithful order is
	// the project's true-to-TS gate default. Presence = collision-follow
	// + zone swap (TS refreshZonePresence, PathingEntity.ts:163-188).
	refreshPlayerZonePresence(p, prevX, prevZ, prevLevel)

	// NAI-65 D4-Player: lastStep adjust from TS PathingEntity.ts:291-292.
	// Currently dead-write at HEAD (no production reader of
	// p.lastStepX/Z besides the dead-write of p.followX/Z in
	// processInteraction). Tracked.
	p.lastStepX = p.x - 1
	p.lastStepZ = p.z

	p.tele = true

	// D5: level-change → INSTANT + jump per PathingEntity.ts:295-298.
	if prevLevel != level {
		p.moveSpeed = MoveSpeedInstant
		p.jump = true
	}
}

// focus records the fine-grained face-angle coord. Mirrors TS
// PathingEntity.focus (Engine-TS/src/engine/entity/PathingEntity.ts:321-333).
// instant=true ALSO writes faceSquareX/Z to (fx, fz) and ORs
// MaskFaceCoord into masks.
//
// Coord-frame note: focus() takes RAW fine coords (already
// CoordGrid.fine'd). Distinct from (*Player).FaceSquare in
// modules/world/player_masks.go which takes absolute coords and
// applies *2+1.
//
// Drivers per TS: Teleport (PathingEntity.ts:289), takeStep
// (PathingEntity.ts:220), reorient (PathingEntity.ts:353,358),
// setInteraction (PathingEntity.ts:528). The setInteraction site is
// the only one that ever passes instant=true — gated on
// (target instanceof NonPathingEntity && interaction === Interaction.ENGINE).
func (p *Player) focus(fx, fz int, instant bool) {
	p.faceAngleX = fx
	p.faceAngleZ = fz
	if instant {
		p.faceSquareX = fx
		p.faceSquareZ = fz
		p.masks |= rsbuf.MaskFaceCoord
	}
}

// unfocus restores the default-south face-angle. Mirrors TS
// PathingEntity.unfocus (Engine-TS/src/engine/entity/PathingEntity.ts:338-341);
// players are 1x1. Called on login (addPlayer) so a freshly-observed player
// faces south instead of the client's north-east default. Closes the
// NAI-67-D-PLAYER-UNFOCUS-DEFERRED gap for the spawn-orientation case.
func (p *Player) unfocus() {
	p.faceAngleX = coordgrid.Fine(p.x, 1)
	p.faceAngleZ = coordgrid.Fine(p.z-1, 1)
}

// effectiveFaceCoord returns the fine coord the player should be shown facing:
// the active faceSquare when set, else the resting faceAngle (south on login).
// The rsbuf FACE_COORD low-def is always forced, so a player whose faceSquare
// is unset (-1) must fall back to faceAngle or the client orients them to its
// own default (north-east). Twin of (*Npc).effectiveFaceCoord.
func (p *Player) effectiveFaceCoord() (x, z int) {
	if p.faceSquareX == -1 && p.faceSquareZ == -1 {
		return p.faceAngleX, p.faceAngleZ
	}
	return p.faceSquareX, p.faceSquareZ
}

// FaceSquare rotates the player to face the square at absolute (x, z)
// on the current level. Wire coords are doubled+1 (face-center).
//
// Mirrors TS Player.faceSquare (Player.ts:1898-1900) → focus(fineX,fineZ,
// client=true) at PathingEntity.ts:321-333: the client=true branch writes
// the same fine coord to BOTH faceAngleX/Z and faceSquareX/Z and ORs the
// coordmask. The faceAngleX/Z writes are load-bearing because faceAngle
// is the resting orientation that survives the per-tick faceSquare reset
// (see effectiveFaceCoord above); without it, a P_FACESQUARE issued
// without a follow-up walk step leaves faceAngle stuck at its prior value
// (often south from unfocus()), so the next tick's forced FACE_COORD
// emit orients the entity back to that stale direction.
func (p *Player) FaceSquare(x, z int) {
	fineX := x*2 + 1
	fineZ := z*2 + 1
	p.faceAngleX = fineX
	p.faceAngleZ = fineZ
	p.faceSquareX = fineX
	p.faceSquareZ = fineZ
	p.masks |= rsbuf.MaskFaceCoord
}

// statBounds bounds-checks a skill id against the 21-skill array range.
func statBounds(id int) bool { return id >= 0 && id < 21 }

// Stat returns the player's current (possibly boosted/drained) level for
// skill id. Returns 0 on OOB.
func (p *Player) Stat(id int) int {
	if !statBounds(id) {
		return 0
	}
	return int(p.levels[id])
}

// StatBase returns the player's base (unboosted) level for skill id.
// Returns 0 on OOB.
func (p *Player) StatBase(id int) int {
	if !statBounds(id) {
		return 0
	}
	return int(p.baseLevels[id])
}

// StatXP returns the player's accumulated XP for skill id as a scaled
// integer (authentic: XP * 10). Returns 0 on OOB.
func (p *Player) StatXP(id int) int {
	if !statBounds(id) {
		return 0
	}
	return int(p.stats[id])
}

// SetCurLevel overrides the player's current level for skill id, clamped
// to [0, 255]. OOB ids are dropped silently. The existing updateStats()
// diff against lastLevels picks up the change and emits UpdateStat.
func (p *Player) SetCurLevel(id int, level int) {
	if !statBounds(id) {
		return
	}
	if level < 0 {
		level = 0
	} else if level > 255 {
		level = 255
	}
	p.levels[id] = uint8(level)
}

// SetStat clamps level to [1, 99] and writes baseLevels, levels, and
// stats (XP) for the given stat slot. Mirrors TS Player.setLevel
// (Player.ts:1823-1834). Used by ::setstat and ::minme cheats (NAI-184).
//
// On any change, calls recomputeCombatLevel(true) — TS guards the
// rebuild on (combatLevel != getCombatLevel()) so non-combat-stat
// changes and no-op cases don't flip MaskAppearance.
func (p *Player) SetStat(stat, level int) {
	if !statBounds(stat) {
		return
	}
	if level < 1 {
		level = 1
	}
	if level > 99 {
		level = 99
	}
	p.baseLevels[stat] = uint8(level)
	p.levels[stat] = uint8(level)
	p.stats[stat] = int32(objtype.GetExpByLevel(level))
	p.recomputeCombatLevel(true)
}

// calcCombatLevel ports TS Player.getCombatLevel (Player.ts:1302-1308).
// Pure formula, no side effects. Uses baseLevels[] (NOT levels[]) so
// buffs/drains don't move combat level. Result is bounded: at fresh
// stats (all=1, hp=10) CL=3; at all-99 maxed stats CL=126.
//
// Integer division (Go) on non-negative operands floors exactly like
// TS Math.floor — prayer/2, rng/2, mag/2 don't need an explicit floor.
// math.Floor on the final float64 mirrors the outer TS Math.floor.
//
// NAI-184.
func (p *Player) calcCombatLevel() int {
	def := int(p.baseLevels[objtype.PlayerStatDefence])
	hp := int(p.baseLevels[objtype.PlayerStatHitpoints])
	prayer := int(p.baseLevels[objtype.PlayerStatPrayer])
	att := int(p.baseLevels[objtype.PlayerStatAttack])
	str := int(p.baseLevels[objtype.PlayerStatStrength])
	rng := int(p.baseLevels[objtype.PlayerStatRanged])
	mag := int(p.baseLevels[objtype.PlayerStatMagic])

	base := 0.25 * float64(def+hp+prayer/2)
	melee := 0.325 * float64(att+str)
	rangd := 0.325 * float64(rng/2+rng)
	magic := 0.325 * float64(mag/2+mag)

	return int(math.Floor(base + math.Max(melee, math.Max(rangd, magic))))
}

// recomputeCombatLevel updates p.combatLevel if calcCombatLevel now
// yields a different value. When triggerRebuild is true, also flips
// MaskAppearance (via SetAppearanceInv) so the next encodeOut emits a
// fresh appearance — required after stat-changing operations that
// happen post-login. When triggerRebuild is false, only updates the
// field — used at LoadSave time, before the client has any appearance.
//
// SetStat and AddXP pass true; LoadSave passes false. Mirrors the
// guarded-rebuild blocks at TS Player.ts:1821-1824 (giveExp/advanceStat
// path) and TS Player.ts:1841-1843 (setLevel/SetStat path).
//
// 244 delta (TS Player.ts:1823 + 1843): both rebuild sites call
// buildAppearance(InvType.WORN) rather than buildAppearance(this.appearanceInv).
// Use p.client.server.invTypes.Worn when available; fall back to
// p.appearanceInv on test paths that call this without a fully-wired
// server (production always has client+server+invTypes bound).
// handleIfPlayerDesign uses the analogous invTypes nil-guard, though it
// needs no client/server guards (it is a *Server method).
//
// NAI-184.
func (p *Player) recomputeCombatLevel(triggerRebuild bool) {
	newCL := p.calcCombatLevel()
	if newCL == p.combatLevel {
		return
	}
	p.combatLevel = newCL
	if triggerRebuild {
		// 244: buildAppearance(InvType.WORN), not buildAppearance(appearanceInv).
		// Fall back to p.appearanceInv when invTypes is unavailable (no-server test paths).
		wornId := p.appearanceInv
		if p.client != nil && p.client.server != nil && p.client.server.invTypes != nil {
			wornId = p.client.server.invTypes.Worn
		}
		p.SetAppearanceInv(wornId)
	}
}

// changeStat fires the [changestat,<skill>] trigger for the given stat
// slot when a cache script is registered for that exact stat (or its
// category, or globally). Mirrors TS Player.changeStat (Player.ts:1816-1821).
//
// Enqueued as QueueEngine — TS PlayerQueueType.ENGINE: distinct from
// the primary queue, drains in processPlayerEngineQueues between
// processPlayerTimers and processPathing (NAI-144). Closes the S6h
// QueueNormal-as-ENGINE deviation.
//
// Silent no-op if no script is registered (GetByTrigger returns nil →
// EnqueueScriptFile's nil-check short-circuits). Called from AddXP's
// level-up branch.
func (p *Player) changeStat(stat int) {
	if p.client == nil || p.client.server == nil || p.client.server.scriptProvider == nil {
		return
	}
	sf := p.client.server.scriptProvider.GetByTrigger(script.TriggerChangeStat, stat, -1)
	p.EnqueueScriptFile(sf, 0, nil, nil, script.QueueEngine)
}

// ChangeStat implements script.ActivePlayer. Exported wrapper around the
// internal changeStat so script handlers (STAT_ADD / STAT_SUB / STAT_BOOST
// / STAT_DRAIN / STAT_HEAL) can fire the [changestat,<skill>] trigger via
// the ActivePlayer interface after SetCurLevel. Mirrors TS
// PlayerOps.ts:516-518, :534-536, :555-557, :572-574, :613-615.
func (p *Player) ChangeStat(stat int) {
	p.changeStat(stat)
}

// advanceStat fires the [advancestat,<skill>] trigger for the given stat
// slot when a cache script is registered for that exact stat. Unlike
// changeStat (which uses the 3-level fallback via GetByTrigger), this
// uses GetByTriggerSpecific — type-specific only, no category or global
// fallback. A global [advancestat,_] script would be wrong here: cache
// scripts that say "Congratulations, you just advanced an Attack level!"
// must be skill-keyed.
//
// Enqueued as QueueEngine — TS PlayerQueueType.ENGINE (NAI-144).
// Matches TS Player.ts:1804-1807 exactly.
//
// Silent no-op if no specific script is registered (GetByTriggerSpecific
// returns nil → EnqueueScriptFile's nil-check short-circuits). Called
// from AddXP's level-up branch after changeStat.
func (p *Player) advanceStat(stat int) {
	if p.client == nil || p.client.server == nil || p.client.server.scriptProvider == nil {
		return
	}
	sf := p.client.server.scriptProvider.GetByTriggerSpecific(script.TriggerAdvanceStat, stat, -1)
	p.EnqueueScriptFile(sf, 0, nil, nil, script.QueueEngine)
}

// AddXP adds xp (scaled ×10) to the player's stored XP for skill id and
// recomputes baseLevels from the XP curve. Matches TS Player.advanceStat
// (Player.ts:1752-1772) in three branches:
//
//   - Un-buffed (levels[id] == baseLevels[id]): advance BOTH levels and
//     baseLevels together. This is the common case — every fresh-player
//     training session. TS line 1760-1763.
//   - Buffed (levels[id] > baseLevels[id]): update baseLevels only;
//     preserve the buff on levels. Level-ups don't strip active potions.
//   - Drained (levels[id] < baseLevels[id]): update baseLevels; on
//     level-up replenish levels by the level delta. TS line 1767-1770.
//
// XP is clamped at objtype.MaxXP (200m real, stored as 2B ×10). Negative
// xp is clamped to keep stats[id] >= 0 defensively — deviation from TS
// where a bug could reduce stored XP. Matches the convention from
// Player.Damage / *Npc.Damage negative-amount clamps.
//
// On level-up (baseLevels increases), fires the [changestat,<skill>] trigger
// via changeStat (TS Player.ts:1772) then the [advancestat,<skill>] trigger
// via advanceStat (TS Player.ts:1804-1807), then calls recomputeCombatLevel
// to mirror TS Player.ts:1810-1813. On level-up also emits ADVENTURE
// session-log entries (Levelled up + milestone-250 + p2p-1881 + f2p-1485)
// per TS Player.ts:1773-1803.
// xpRate returns the configured XP multiplier (TS Environment.NODE_XPRATE),
// defaulting to 1 when the server/config is unreachable (bare test fixtures) or
// the value is non-positive, so XP is never zeroed or reversed by a bad config.
func (p *Player) xpRate() int {
	if p.client == nil || p.client.server == nil {
		return 1
	}
	if r := p.client.server.cfg.NodeXPRate; r > 0 {
		return r
	}
	return 1
}

func (p *Player) AddXP(id int, xp int, allowMulti bool) {
	if !statBounds(id) {
		return
	}
	// player-core-3: TS Player.addXp (Player.ts:1742-1744) begins with
	// `if (xp < 0) throw new Error(...)`. The throw aborts the script
	// before any stat mutation; goscape previously fell through to the
	// min/clamp math, which silently REDUCED stored XP on negative input.
	// Mirror TS's "no mutation on negative" intent by early-returning
	// here. The full TS-faithful "abort the script" surface is at the
	// STAT_ADVANCE handler (handleStatAdvance in pkg/script) — that
	// script-error path is a deferred deviation; this guard plugs the
	// load-bearing entity-layer silent-reduction bug.
	if xp < 0 {
		return
	}
	// TS Player.addXp (Player.ts:1751): `const multi = allowMulti ?
	// Environment.NODE_XPRATE : 1; this.stats[stat] += xp * multi`. M7 — the
	// node_xp_rate config (cfg.NodeXPRate, default 1) was never applied, so the
	// multiplier was dead. allowMulti=false (e.g. the ::setlevel cheat) bypasses
	// it so an exact-level grant isn't scaled.
	if allowMulti {
		xp *= p.xpRate()
	}
	next := min(int64(p.stats[id])+int64(xp), int64(objtype.MaxXP))
	if next < 0 {
		next = 0
	}
	beforeBase := int(p.baseLevels[id])
	p.stats[id] = int32(next)
	newBase := objtype.GetLevelByExp(int(p.stats[id]))

	// Un-buffed branch: advance levels in lockstep with baseLevels so a
	// fresh-player level-up is visible on the stat display. TS Player.ts:1760-1763.
	if int(p.levels[id]) == beforeBase {
		p.levels[id] = uint8(newBase)
	}
	p.baseLevels[id] = uint8(newBase)
	afterBase := newBase

	if afterBase > beforeBase && int(p.levels[id]) < beforeBase {
		// Drained + level-up: replenish levels by the level delta.
		// Matches TS Player.ts:1767-1770.
		p.levels[id] = uint8(min(int(p.levels[id])+(afterBase-beforeBase), 255))
	}
	if afterBase > beforeBase {
		// Level-up: fire [changestat,<skill>] then [advancestat,<skill>]
		// triggers if registered. Matches TS Player.ts:1772, 1804-1807.
		p.changeStat(id)

		// TS Player.ts:1773-1803 — ADVENTURE session-log entries on level-up.
		p.AddSessionLog(LoggerEventTypeAdventure,
			"Levelled up "+objtype.PlayerStatNames[id]+
				" from "+strconv.Itoa(beforeBase)+
				" to "+strconv.Itoa(afterBase))

		total, freeTotal := 0, 0
		for i := range objtype.PlayerStatCount {
			if !objtype.PlayerStatEnabled[i] {
				continue
			}
			total += int(p.baseLevels[i])
			if objtype.PlayerStatFree[i] {
				freeTotal += int(p.baseLevels[i])
			}
		}
		const milestone = 250
		prevMilestone := (total - (afterBase - beforeBase)) / milestone
		currMilestone := total / milestone
		if currMilestone > prevMilestone {
			p.AddSessionLog(LoggerEventTypeAdventure,
				"Reached total level "+strconv.Itoa(currMilestone*milestone))
		}
		if total == 1881 {
			p.AddSessionLog(LoggerEventTypeAdventure,
				"Reached total level 1881 - you beat p2p!")
		}
		if freeTotal == 1485 {
			p.AddSessionLog(LoggerEventTypeAdventure,
				"Reached total level 1485 - you beat f2p!")
		}

		p.advanceStat(id)
		p.recomputeCombatLevel(true) // TS Player.ts:1810-1813
	}
}

// PlayAnim schedules sequence seqID with the given client-side delay on
// the player's primary animation slot. seqID=-1 clears. Mirrors TS
// Player.playAnimation (Player.ts:1852-1862) at Engine-TS pin 9aadcec4:
// bounds-reject on seqID >= SeqType.count, animProtect early-return, and
// priority >= overwrite gate. The seqID==-1 / animID==-1 short-circuits
// guard the slice dereferences. 244 changed strict `>` to `>=`, meaning
// equal nonzero priority now overwrites. Closes deviation NAI-56-D1.
func (p *Player) PlayAnim(seqID, delay int) {
	if seqID >= p.seqTypes.Count() || p.animProtect != 0 {
		return // TS Player.ts:1853
	}
	if seqID == -1 || p.animID == -1 ||
		p.seqTypes.Configs[seqID].Priority >= p.seqTypes.Configs[p.animID].Priority {
		p.animID = seqID
		p.animDelay = delay
		p.masks |= rsbuf.MaskAnim
	}
}

// PlaySpotAnim schedules a graphic (spotanim) on the player at the given
// height with the given client-side delay. id=-1 clears.
func (p *Player) PlaySpotAnim(id, height, delay int) {
	p.spotanimID = id
	p.spotanimHeight = height
	p.spotanimDelay = delay
	p.masks |= rsbuf.MaskSpotAnim
}

// SetAppearanceInv stores id on Player.appearanceInv and flags
// MaskAppearance. Mirrors TS Player.buildAppearance (the literal
// two-liner at Engine-TS/src/engine/entity/Player.ts:1836-1839). The
// mask triggers generateAppearance regeneration on the next tick in
// tick.go:325-335.
func (p *Player) SetAppearanceInv(id int) {
	p.appearanceInv = id
	p.masks |= rsbuf.MaskAppearance
}

// SetReadyAnim sets the player's idle/stand animation. BAS anims are
// persistent and flow through the appearance buffer, which regenerates
// on MaskAppearance — no per-call mask flip needed.
func (p *Player) SetReadyAnim(seqID int) { p.readyanim = seqID }

// SetTurnAnim sets the player's turn-in-place animation.
func (p *Player) SetTurnAnim(seqID int) { p.turnanim = seqID }

// SetWalkAnim sets the player's forward-walk animation.
func (p *Player) SetWalkAnim(seqID int) { p.walkanim = seqID }

// SetWalkAnimB sets the player's backward-walk animation.
func (p *Player) SetWalkAnimB(seqID int) { p.walkanim_b = seqID }

// SetWalkAnimL sets the player's strafe-left walk animation.
func (p *Player) SetWalkAnimL(seqID int) { p.walkanim_l = seqID }

// SetWalkAnimR sets the player's strafe-right walk animation.
func (p *Player) SetWalkAnimR(seqID int) { p.walkanim_r = seqID }

// SetRunAnim sets the player's run animation.
func (p *Player) SetRunAnim(seqID int) { p.runanim = seqID }

// WalkTrigger implements script.ActivePlayer.WalkTrigger. Returns the
// queued walktrigger script id, or -1 if none. NAI-51.
func (p *Player) WalkTrigger() int { return p.walktrigger }

// SetWalkTrigger implements script.ActivePlayer.SetWalkTrigger. Stores
// scriptID in p.walktrigger. -1 clears. NAI-51.
func (p *Player) SetWalkTrigger(scriptID int) { p.walktrigger = scriptID }

// S5f: interface / modal control.
//
// Modal mutex rules mirror LostCityRS/Engine-TS Player.ts:1928-2022:
//   - OpenMain          closes CHAT + SIDE.
//   - OpenChat          closes MAIN + SIDE.
//   - OpenSide          closes MAIN + CHAT.
//   - OpenMainModalSide closes CHAT (keeps MAIN + SIDE by definition).
//
// All methods set refreshModal so the next encodeOut() emits the matching
// IF_OPEN* (and any IF_CLOSE) packets.

// CloseModal clears every modal slot and flags the client to emit
// IF_CLOSE on the next encodeOut pass. When clearWeakQueue is true
// (TS default), drops every QueueWeak entry from p.queue before
// processing. Early-returns if no modal is currently open. Otherwise:
// nulls activeScript on COUNTDIALOG/PAUSEBUTTON suspends (closes
// NAI-52-F1) and dispatches a per-slot IF_CLOSE trigger script
// (Main → Chat → Side, TS order).
//
// Mirrors TS Player.closeModal (Player.ts:741-794). Body fully
// landed across NAI-53 T1-T5; per-slot clearComListeners wired in
// NAI-64 (TS Player.ts:728-739, 767, 778, 789).
//
// DEVIATION-NAI-111-D1: NAI-52 convergence narrowed. CloseModal does
// NOT touch p.activeScript.Pointers — TS Player.closeModal clears
// this.protect (Player-level bool) but contains zero pointer
// operations on script.pointers&PAP. Goscape maps PAP only to the
// script-state bitmask; the external "is a protected script
// suspended?" question is answered by protectedScriptActive, which
// reads the preserved pointer on the stored *ScriptState across
// suspensions (StoreActiveScript at player_script.go:138-140
// preserves Pointers). NAI-53 T3's earlier convergence claim that
// CloseModal must clear PAP was incorrect — it stripped the gate
// from in-flight resumed scripts (e.g. tut_close inside
// [label,tutorial_complete] caused P_TELEJUMP to abort).
func (p *Player) CloseModal(clearWeakQueue bool) {
	if clearWeakQueue {
		p.clearWeakQueue()
	}

	if p.modalState == modalStateNone {
		return
	}

	p.modalState = modalStateNone

	// TS Player.ts:746 — this.protect = false. Unconditionally clear the
	// Player-level protect gate even when activeScript is mid-flight with
	// PAP still set. The script-state PAP pointer is NOT touched
	// (preserved on activeScript.Pointers for downstream handler
	// pointerCheck — NAI-53 T3 regression-guard: clearing PAP-on-state
	// broke in-flight resumed scripts like tut_close inside
	// [label,tutorial_complete] aborting P_TELEJUMP). The TS-faithful
	// split — Player.protect Player-level gate vs script-state PAP
	// pointer — sidesteps that regression by only clearing the gate.
	// NAI-111-D1.
	p.protect = false

	// Close any input-dialogue suspended scripts. NAI-52-F1; A9 adds the
	// resumeButtons clear — TS Player.ts:775-779 @2e3bcf43 (2dc4a811):
	//
	//	// close any input dialogue suspended scripts.
	//	if (this.activeScript?.execution === ScriptState.COUNTDIALOG ||
	//	    this.activeScript?.execution === ScriptState.PAUSEBUTTON) {
	//	    this.activeScript = null;
	//	    this.resumeButtons = [];
	//	}
	if p.activeScript != nil &&
		(p.activeScript.Execution == script.CountDialog ||
			p.activeScript.Execution == script.PauseButton) {
		p.activeScript = nil
		p.resumeButtons = nil
	}

	// Per-slot IF_CLOSE dispatch (Main → Chat → Side, TS order).
	if p.client != nil && p.client.server != nil {
		s := p.client.server
		if p.modalMain != -1 {
			p.runIfCloseTrigger(s, p.modalMain)
			p.clearComListeners(p.modalMain)
			p.modalMain = -1
		}
		if p.modalChat != -1 {
			p.runIfCloseTrigger(s, p.modalChat)
			p.clearComListeners(p.modalChat)
			p.modalChat = -1
		}
		if p.modalSide != -1 {
			p.runIfCloseTrigger(s, p.modalSide)
			p.clearComListeners(p.modalSide)
			p.modalSide = -1
		}
	} else {
		// No server (test path with no Server bound) — still reset slots.
		p.modalMain = -1
		p.modalChat = -1
		p.modalSide = -1
	}

	p.refreshModalClose = true
}

// runIfCloseTrigger looks up TriggerIfClose for slotCom and runs it
// if found. Mirrors TS Player.closeModal per-slot
// `executeScript(ScriptRunner.init(closeTrigger, this), false)`
// (Player.ts:761-769, 772-780, 783-791).
//
// Nil-safe on s.scriptProvider; runScript is itself nil-safe on the
// returned ScriptFile.
func (p *Player) runIfCloseTrigger(s *Server, slotCom int) {
	if s.scriptProvider == nil {
		return
	}
	sf := s.scriptProvider.GetByTriggerSpecific(script.TriggerIfClose, slotCom, -1)
	s.runScript(sf, p, nil, script.TriggerIfClose, false, nil, nil)
}

// OpenMain opens com as the main modal. Per TS, opening main closes any
// currently-open chat and side modals. M8: the modal bits are cleared/OR'd
// individually (not assigned) so unrelated bits — notably TUT (0x8) — survive,
// matching TS Player.openMainModal (Player.ts:1928-1953 at 244 pin 9aadcec4).
//
// rev-254 delta (A9): the "clear old suspended scripts" block is BACK —
// TS 93ef2d7f re-added it (the 244 pin had deleted it) and 2dc4a811
// extended it to clear resumeButtons. TS openMainModal Player.ts:2012-2016
// @2e3bcf43:
//
//	// clear old suspended scripts
//	if (this.activeScript?.execution === ScriptState.COUNTDIALOG ||
//	    this.activeScript?.execution === ScriptState.PAUSEBUTTON) {
//	    this.activeScript = null;
//	    this.resumeButtons = [];
//	}
//
// h-interface-1: TS writes `new IfClose()` per displaced slot
// (Player.ts:1994-2006 for CHAT/SIDE) BEFORE the
// refreshModal-driven IF_OPEN. encodeOut's close-before-open order
// (player.go:466-477 → IF_CLOSE then IF_OPEN) makes setting
// refreshModalClose here equivalent to TS's inline writes. TS's two
// IfClose writes (one per displaced slot) coalesce into goscape's
// single IF_CLOSE — the wire packet is a close-all-modals signal on
// the client, so 1 and 2 are functionally identical. Self-displacement
// of main is NOT a close (TS uses |= MAIN with no prior bit check).
func (p *Player) OpenMain(com int) {
	if p.modalState&(modalStateChat|modalStateSide) != modalStateNone {
		p.refreshModalClose = true
	}
	p.modalState &^= modalStateChat
	p.modalChat = -1
	p.modalState &^= modalStateSide
	p.modalSide = -1
	p.modalState |= modalStateMain
	p.modalMain = com
	p.refreshModal = true
	p.clearSuspendedDialogScript()
}

// OpenChat opens com as the chat modal. Per TS, opening chat closes any
// currently-open main and side modals. M8: bit-wise clear/OR preserves TUT.
// TS name at the 254 pin: openChatModal (93ef2d7f renamed openChat back to
// the 225-style name; Go keeps OpenChat — Go-idiomatic, mapping only).
// TS Player.openChatModal @2e3bcf43, Player.ts:2031-2053.
//
// rev-254 delta (A9): the "clear old suspended scripts" block is BACK
// (TS Player.ts:2048-2052 — see OpenMain doc for the quote/history).
//
// h-interface-1: TS writes `new IfClose()` per displaced slot
// (Player.ts:2032-2042 for MAIN/SIDE). See OpenMain
// for the close-coalescing rationale.
func (p *Player) OpenChat(com int) {
	if p.modalState&(modalStateMain|modalStateSide) != modalStateNone {
		p.refreshModalClose = true
	}
	p.modalState &^= modalStateMain
	p.modalMain = -1
	p.modalState &^= modalStateSide
	p.modalSide = -1
	p.modalState |= modalStateChat
	p.modalChat = com
	p.refreshModal = true
	p.clearSuspendedDialogScript()
}

// OpenSide opens com as the side modal. Per TS, opening side closes any
// currently-open main and chat modals. M8: bit-wise clear/OR preserves TUT
// (TS Player.openSideModal @2e3bcf43, Player.ts:2055-2077).
//
// rev-254 delta (A9): the "clear old suspended scripts" block is BACK
// (TS Player.ts:2072-2076 — see OpenMain doc for the quote/history).
//
// h-interface-1: TS writes `new IfClose()` per displaced slot
// (Player.ts:2056-2066 for MAIN/CHAT). See OpenMain
// for the close-coalescing rationale.
func (p *Player) OpenSide(com int) {
	if p.modalState&(modalStateMain|modalStateChat) != modalStateNone {
		p.refreshModalClose = true
	}
	p.modalState &^= modalStateMain
	p.modalMain = -1
	p.modalState &^= modalStateChat
	p.modalChat = -1
	p.modalState |= modalStateSide
	p.modalSide = com
	p.refreshModal = true
	p.clearSuspendedDialogScript()
}

// OpenMainModalSide opens mainCom as the main modal and sideCom as the side
// modal simultaneously. Per TS, this closes any currently-open chat modal.
// M8: bit-wise clear/OR preserves TUT and existing side state. TS name at
// the 254 pin: openMainSideModal (93ef2d7f renamed openMainModalSide back
// to the 225-style name; Go keeps OpenMainModalSide — mapping only).
// TS Player.openMainSideModal @2e3bcf43, Player.ts:2085-2103.
//
// rev-254 delta (A9): the "clear old suspended scripts" block is BACK
// (TS Player.ts:2098-2102 — see OpenMain doc for the quote/history).
//
// h-interface-1: TS writes `new IfClose()` only when CHAT was open
// (Player.ts:2086-2090) — MAIN and SIDE are about to be set to new
// coms, so a per-slot close for them would be meaningless (TS uses
// |= to OR them on without an IfClose). See OpenMain for the close-
// coalescing rationale.
func (p *Player) OpenMainModalSide(mainCom, sideCom int) {
	if p.modalState&modalStateChat != modalStateNone {
		p.refreshModalClose = true
	}
	p.modalState &^= modalStateChat
	p.modalChat = -1
	p.modalState |= modalStateMain
	p.modalMain = mainCom
	p.modalState |= modalStateSide
	p.modalSide = sideCom
	p.refreshModal = true
	p.clearSuspendedDialogScript()
}

// clearSuspendedDialogScript is the shared "clear old suspended scripts"
// tail of the four modal-open methods — TS @2e3bcf43 (93ef2d7f re-add +
// 2dc4a811 resumeButtons):
//
//	// clear old suspended scripts
//	if (this.activeScript?.execution === ScriptState.COUNTDIALOG ||
//	    this.activeScript?.execution === ScriptState.PAUSEBUTTON) {
//	    this.activeScript = null;
//	    this.resumeButtons = [];
//	}
//
// TS inlines the block in each method (openMainModal :2012-2016,
// openChatModal :2048-2052, openSideModal :2072-2076, openMainSideModal
// :2098-2102); goscape factors it to keep the four copies byte-identical.
// openMainOverlay and openTutorial do NOT have the block.
func (p *Player) clearSuspendedDialogScript() {
	if p.activeScript != nil &&
		(p.activeScript.Execution == script.CountDialog ||
			p.activeScript.Execution == script.PauseButton) {
		p.activeScript = nil
		p.resumeButtons = nil
	}
}

// OpenTutorial sets the player's tutorial-overlay component and writes
// the matching TUT_OPEN wire packet UNCONDITIONALLY. Mirrors TS
// Player.openTutorial at Engine-TS/src/engine/entity/Player.ts:1999-2003,
// which writes `new TutOpen(com)` on every call regardless of prior
// state. NAI-112 Stage 2.2 retired the goscape diff at
// modules/world/player.go (pre-NAI-112: emit-only-on-change introduced
// by NAI-76 T2) — that suppressed the re-open the client needs to flush
// IF_SETTEXT updates when the same tutorial component is re-opened.
// Surfaced at NAI-110 close smoke; bound NAI-112 Stage 2.1 instrumentation
// 2026-05-06.
//
// Opening the tutorial does NOT close any other modal — the TUT bit is
// OR'd into modalState and the tutorial id is stored alongside the
// wire emit.
func (p *Player) OpenTutorial(com int) {
	payload := []byte{byte(com >> 8), byte(com)}
	p.writeOut(gameserver.OpTutOpen, payload)
	p.modalState |= modalStateTut
	p.modalTutorial = com
}

// CloseTutorial closes the player's tutorial overlay. Per TS:
// no-op if no tutorial open; otherwise dispatches the matching IF_CLOSE
// trigger (if registered) for the current modalTutorial component,
// resets modalTutorial to -1, and writes TUT_OPEN(-1) directly.
//
// TS Player.closeTutorial at Engine-TS/src/engine/entity/Player.ts:716-726
// writes `new TutOpen(-1)` directly. NAI-76 routed this through
// encodeOut's diff-check (NAI-76 pin); NAI-112 Stage 2.2 inlines the
// write to match TS unconditional-emit semantics and to symmetrize with
// the OpenTutorial fix.
//
// TS does NOT call clearComListeners(modalTutorial) here (contrast with
// closeModal); we mirror that absence.
//
// Clears modalStateTut on the goscape-internal modalState bitmap
// (goscape defensive; TS skips this check). Labelled per
// defensive_gate_doc_comment_label.md.
func (p *Player) CloseTutorial() {
	if p.modalTutorial == -1 {
		return
	}
	if p.client != nil && p.client.server != nil {
		p.runIfCloseTrigger(p.client.server, p.modalTutorial)
	}
	p.modalTutorial = -1
	p.modalState &^= modalStateTut
	payload := []byte{0xff, 0xff} // -1 as int16 BE
	p.writeOut(gameserver.OpTutOpen, payload)
}

// OpenOverlay sets the player's full-screen overlay interface. TS name at
// the 254 pin: openMainOverlay (93ef2d7f renamed openOverlay; Go keeps
// OpenOverlay — mapping only). Mirrors TS Player.openMainOverlay at
// Engine-TS/src/engine/entity/Player.ts:2019-2029 @2e3bcf43 (body
// unchanged from 244; NO "clear old suspended scripts" block here):
//
//	openMainOverlay(com: number) {
//	    if (this.overlay === com) { return; }
//	    if (com === -1) { this.clearComListeners(this.overlay); }
//	    this.overlay = com;
//	}
//
// The wire flush (IF_OPENOVERLAY, opcode 158) is deferred to encodeOut
// (player.go) which emits on overlay != lastOverlay each tick, matching
// TS NetworkPlayer.ts:192-195.
//
// NOTE: com == -1 clears listeners of the OLD overlay (the current one
// before this call) — matches TS literally. The call site lands with
// B4's IF_OPENOVERLAY script op; B2 shipped the wire row (0ef495fb).
func (p *Player) OpenOverlay(com int) {
	if p.overlay == com {
		return
	}
	if com == -1 {
		p.clearComListeners(p.overlay)
	}
	p.overlay = com
}

// FlashTutorial implements script.ActivePlayer.FlashTutorial. Writes
// a TUT_FLASH server packet (opcode 126, 1-byte tab payload). Direct
// write — TUT_FLASH is fire-and-forget UI hint, not a modal-state
// transition like TUT_OPEN, so no deferred-flush pathway. Mirrors
// LostCityRS/Engine-TS Player.write(new TutFlash(tab)) call from
// PlayerOps.ts:694-696 + TutFlashEncoder.ts:9-11.
//
// No client-nil guard — matches goscape's direct-writer convention
// (CamReset at line 189-191, HintNpc at line 201-209); writeOut itself
// does not nil-guard either.
func (p *Player) FlashTutorial(tab int) {
	p.writeOut(gameserver.OpTutFlash, []byte{byte(tab)})
}

// AddResumeButton appends one resume-button interface id for later
// consumption by the if_button resume gate. No wire op is emitted.
// Mirrors TS PlayerOps.ts IF_ADDRESUMEBUTTON @2e3bcf43
// (`state.activePlayer.resumeButtons.push(comId)`).
func (p *Player) AddResumeButton(comId int) {
	p.resumeButtons = append(p.resumeButtons, comId)
}

// S5g: dialog suspension.

func (p *Player) LastCom() int { return p.lastCom }

func (p *Player) SendCountDialog() {
	p.writeOut(gameserver.OpPCountDialog, nil)
}

// S5h: action-clear.

// StopAction implements script.ActivePlayer.StopAction. Mirrors TS
// Player.stopAction (Player.ts:944-947) = clearPendingAction() +
// unsetMapFlag() — clears any anchored interaction target plus any
// pending-action state (modals, interaction kind) AND clears the walk
// queue + emits OpUnsetMapFlag so the client drops its map-click
// indicator. unsetMapFlag is the lowercase TS-bundled helper
// (interaction.go:61), NOT the wire-only sendUnsetMapFlag.
//
// player-script-1 (player_script.go LANDED 2026-05-30) added the leading
// ClearInteraction call as a defensive cover for player-script-7's
// partial ClearPendingAction — that gap is now closed (post-fix
// ClearPendingAction delegates to ClearInteraction internally) so the
// explicit leading call has been dropped to match TS shape exactly.
func (p *Player) StopAction() {
	p.ClearPendingAction()
	p.unsetMapFlag()
}

// RequestLogout implements script.ActivePlayer.RequestLogout. Flags the
// player for tick-loop logout processing; processLogouts (tick.go) tears
// the session down at the next boundary. Mirrors TS PlayerOps.ts:622-624
// (P_LOGOUT) — `state.activePlayer.requestLogout = true`.
func (p *Player) RequestLogout() {
	p.requestLogout = true
}

// ClearPendingAction implements script.ActivePlayer.ClearPendingAction.
// Mirrors TS Player.clearPendingAction (Player.ts:950-953) =
// clearInteraction() + closeModal(). Walk queue is preserved.
//
// Pre-fix this method did a partial inline reset (target + targetOp only),
// leaving apRange / apRangeCalled / targetSubject stuck at their last
// interaction's values. A content-script p_clearpendingaction (or any
// callsite that funnels through here — opheld, op_player, minimap walk
// modal-close) then left half the interaction state alive: the next
// SetInteraction inherited a stale apRange / apRangeCalled / targetSubject
// where TS gives it a clean slate (TS PathingEntity.clearInteraction
// PathingEntity.ts:550-555).
//
// Post-interaction-6 (memorialized at interaction.go:226-249)
// (*Player).ClearInteraction is itself TS-faithful (target/targetOp/
// targetSubject + apRange=10/apRangeCalled=false), so delegating here
// gives the full TS-shape with no duplicated knowledge. interactionKind
// is goscape-specific and outside TS's clearInteraction — preserved as
// a defensive reset to InteractionEngine (the zero-value default).
func (p *Player) ClearPendingAction() {
	p.interactionKind = InteractionEngine
	p.ClearInteraction()
	p.CloseModal(true)
}

// SetApRange implements script.ActivePlayer.SetApRange. Sets apRange
// and marks apRangeCalled=true in a single call (tick-serialized by
// the engine; no lock needed) to persist the interaction past the
// current tick. Matches TS PlayerOps.ts:P_APRANGE.
func (p *Player) SetApRange(n int) {
	p.apRange = n
	p.apRangeCalled = true
}

// TargetSubjectCom implements script.ActivePlayer.TargetSubjectCom.
// Returns p.targetSubject.com which was set by OpLocT's SetInteraction
// call (spellCom) or -1 for non-com callers.
func (p *Player) TargetSubjectCom() int { return p.targetSubject.com }

// InvListenOnCom implements script.ActivePlayer. Thin wrapper
// delegating to the internal unexported method landed in S6p-2.
func (p *Player) InvListenOnCom(invType, com, source int) {
	p.invListenOnCom(invType, com, source)
}

// InvStopListenOnCom implements script.ActivePlayer. Thin wrapper
// delegating to the internal unexported method landed in S6p-2.
func (p *Player) InvStopListenOnCom(com int) {
	p.invStopListenOnCom(com)
}

// SetInteractionScriptLoc implements script.ActivePlayer. Type-asserts
// the narrow script.ActiveLoc back to *entity.Loc and anchors the
// player with trigger ApLoc<op> + InteractionScript. Matches TS
// PlayerOps.ts P_OPLOC terminal setInteraction. Silently no-ops if the
// loc isn't a real *entity.Loc (defensive — only goscape's OPLOC
// routing sets ScriptState.ActiveLoc with this concrete type).
func (p *Player) SetInteractionScriptLoc(loc script.ActiveLoc, op int) {
	realLoc, ok := loc.(*entitypkg.Loc)
	if !ok {
		return
	}
	p.SetInteraction(InteractionScript, realLoc, op, -1)
}

// SetAnimProtect implements script.ActivePlayer.SetAnimProtect. Stores the
// anim-protect flag; when nonzero, PlayAnim suppresses in-engine animation
// requests (NAI-56 wired the reader at PlayAnim's L1842 gate). Matches TS
// Player.ts:321 (field) + PlayerOps.ts:1171-1172.
func (p *Player) SetAnimProtect(v int) { p.animProtect = v }

// SetAllowDesign implements script.ActivePlayer.SetAllowDesign. Stores the
// flag; ALLOWDESIGN (opcode 2001) is the sole writer. Reader path unported
// per S7e-D1.
func (p *Player) SetAllowDesign(v bool) { p.allowDesign = v }

// SetInteractionScriptNpc implements script.ActivePlayer.
func (p *Player) SetInteractionScriptNpc(npc script.ActiveNpc, op int) {
	realNpc, ok := npc.(*Npc)
	if !ok {
		return
	}
	p.SetInteraction(InteractionScript, realNpc, op, -1)
}

// QueueWaypoint implements script.ActivePlayer.QueueWaypoint by
// delegating to the package-private queueWaypoint at movement.go.
// NAI-115 T7.
func (p *Player) QueueWaypoint(x, z int) { p.queueWaypoint(x, z) }

// InOperableDistance implements script.ActivePlayer.InOperableDistance
// by type-asserting the narrow script.ActiveLoc back to *entity.Loc and
// delegating to the package-private inOperableDistance shape-aware
// reach probe at modules/world/interaction.go:634. Mirrors TS
// Player.inOperableDistance consumed by P_OPLOC
// (PlayerOps.ts:396-398) to gate queueWaypoint dispatch when the
// player has not yet reached the active loc.
//
// Returns true on type-assert failure so the script-side gate
// suppresses queueWaypoint for non-production active-loc inputs (test
// doubles). Production OPLOC routing always installs a concrete
// *entity.Loc.
func (p *Player) InOperableDistance(loc script.ActiveLoc) bool {
	realLoc, ok := loc.(*entitypkg.Loc)
	if !ok {
		return true
	}
	return inOperableDistance(p, realLoc)
}

// SetInteractionScriptObj implements script.ActivePlayer. Type-asserts
// the script-side ActiveObj to *entity.Obj and anchors the player
// with InteractionScript + APOBJ<op>. Silently no-ops if the obj
// isn't a real *entity.Obj. NAI-115 T7.
func (p *Player) SetInteractionScriptObj(obj script.ActiveObj, op int) {
	realObj, ok := obj.(*entitypkg.Obj)
	if !ok {
		return
	}
	p.SetInteraction(InteractionScript, realObj, op, -1)
}

// HasInteraction reports whether the player has an interaction target,
// excluding the follow interaction (APPLAYER3 / OPPLAYER3). Mirrors TS
// Player.hasInteraction at Engine-TS Player.ts:955-964 — "the follow
// interaction doesn't do anything", so it is reported as not-busy.
// NAI-120 Bundle 2B. Implements script.ActivePlayer.
func (p *Player) HasInteraction() bool {
	if p.target == nil {
		return false
	}
	if isFollowOp(p) {
		return false
	}
	return true
}

// HasWaypoints reports whether the player has a waypoint queue active.
// Wraps the package-private hasWaypoints helper at interaction.go:297.
// NAI-120 Bundle 2B. Implements script.ActivePlayer.
func (p *Player) HasWaypoints() bool { return p.hasWaypoints() }

// SetInteractionScriptNpcT implements script.ActivePlayer.
// Routes via SetInteraction(InteractionScript, npc, targetOpNpcT, spellCom)
// — the targetOpNpcT sentinel (=8 at modules/world/interaction.go:35) selects
// the APNPCT/OPNPCT trigger family in resolveTriggerTypeId. NAI-120 Bundle 2B.
func (p *Player) SetInteractionScriptNpcT(npc script.ActiveNpc, spellCom int) {
	realNpc, ok := npc.(*Npc)
	if !ok {
		return
	}
	p.SetInteraction(InteractionScript, realNpc, targetOpNpcT, spellCom)
}

// SetInteractionScriptPlayer implements script.ActivePlayer. Routes via
// SetInteraction(InteractionScript, realPlayer2, op, -1). The com=-1 means
// no spellCom association — APPLAYER<N> reads no targetSubject.com. NAI-120
// Bundle 2B.
func (p *Player) SetInteractionScriptPlayer(player2 script.ActivePlayer, op int) {
	realPlayer2, ok := player2.(*Player)
	if !ok {
		return
	}
	p.SetInteraction(InteractionScript, realPlayer2, op, -1)
}

// LowMemory returns the player's low-memory flag as plumbed from the
// RS2 login request (req.LowMemory) through client.lowMemory and
// copied onto the Player at newPlayer().
func (p *Player) LowMemory() bool { return p.lowMemory }

// NAI-47: SETIDKIT appearance mutation.
func (p *Player) Gender() int { return p.gender }

// Members reports the per-player members flag (login RPC field).
func (p *Player) Members() bool { return p.members }

// AddHeroPoints implements script.ActivePlayer. Credits amount to
// playerUID on the player's hero-point ledger. Used by BOTH_HEROPOINTS.
// Mirrors TS Player.heroPoints.addHero at PlayerOps.ts:1167.
func (p *Player) AddHeroPoints(playerUID, amount int) {
	p.heroPoints.AddHero(playerUID, amount)
}

// TopContributor implements script.ActivePlayer. Returns the playerUID
// with the largest HeroPoints credit, or 0 if the ledger is empty.
// Used by FINDHERO. Mirrors TS state.activePlayer.heroPoints.findHero()
// at PlayerOps.ts:1139.
func (p *Player) TopContributor() int {
	return p.heroPoints.TopContributor()
}

// HeroPointsClear implements script.ActivePlayer. Resets the player's
// hero-point contributor ledger. Mirrors TS Player.heroPoints.clear() at
// PlayerOps.ts:513-515, :552-554, :609-611 (HP-full branches of STAT_ADD
// / STAT_BOOST / STAT_HEAL). NAI-120 Bundle 2D follow-up.
func (p *Player) HeroPointsClear() {
	p.heroPoints.Clear()
}
func (p *Player) SetBodyPart(slot, idkit int)  { p.body[slot] = idkit }
func (p *Player) SetColorPart(slot, color int) { p.colors[slot] = color }

// PlaySong sends a MIDI song by TRACK ID to the client. A10 @2e3bcf43:
// the 244-era name-based signature (normalize + midiPack registry
// lookup) is retired — names resolve to ids at script compile time
// (ScriptVarType MIDI; tools/pack/Compiler.ts:199). Mirrors TS
// Player.playSong (Player.ts:1985-1987 @2e3bcf43):
//
//	playSong(id: number) {
//	    this.write(new MidiSong(id));
//	}
//
// No id guard — TS writes unconditionally (the encoder p2 truncates).
// Nil client guard is goscape-defensive for bare test players.
func (p *Player) PlaySong(id int) {
	if p.client == nil {
		return
	}
	buf := packet.NewPacket(make([]byte, 0, 4))
	encodeMidiSong(buf, id)
	p.writeOut(gameserver.OpMidiSong, buf.Bytes())
}

// PlayJingle sends a short MIDI jingle by TRACK ID; the wire delay field
// is the track's parsed length in milliseconds from the boot-time Midi
// cache. Mirrors TS Player.playJingle (Player.ts:1989-1991 @2e3bcf43):
//
//	playJingle(id: number): void {
//	    this.write(new MidiJingle(id, Midi.getLength(id)));
//	}
//
// A10: the 244-era (delay, name) signature is retired — the script no
// longer supplies the delay; the server derives it. Nil server (bare
// test player) degrades the length to 0, matching the TS unknown-id
// `lengths[id] ?? 0` posture.
func (p *Player) PlayJingle(id int) {
	if p.client == nil {
		return
	}
	length := 0
	if p.client.server != nil {
		length = p.client.server.midi.GetLength(id)
	}
	buf := packet.NewPacket(make([]byte, 0, 4))
	encodeMidiJingle(buf, id, length)
	p.writeOut(gameserver.OpMidiJingle, buf.Bytes())
}

// PlaySynth sends a synthesized sound effect to the client. Called by
// the SOUND_SYNTH script opcode (PlayerOps.ts:466-474). Encodes
// synth/loops/delay via encodeSynthSound and writes OpSynthSound.
//
// No name normalization, no PRELOADED lookup, no validation — TS
// handler has none. Out-of-range int values truncate at the
// uint16/uint8/uint16 cast boundary (matches TS p1/p2 narrowing).
func (p *Player) PlaySynth(synth, loops, delay int) {
	buf := packet.NewPacket(make([]byte, 0, 5))
	encodeSynthSound(buf, uint16(synth), uint8(loops), uint16(delay))
	p.writeOut(gameserver.OpSynthSound, buf.Bytes())
}

// SetPreventLogout implements script.ActivePlayer. Mirrors TS
// PlayerOps.ts:628-629 (state.activePlayer.preventLogoutMessage =
// msg; state.activePlayer.preventLogoutUntil = currentTick + ticks).
// NAI-127 Bundle 2.
func (p *Player) SetPreventLogout(message string, untilTick int) {
	p.preventLogoutMessage = message
	p.preventLogoutUntil = untilTick
}

// ApplyDamage implements script.ActivePlayer. Delegates to
// Player.Damage (player_masks.go:126), which is the existing
// damage-mask producer. Mirrors TS player.applyDamage(amount, type)
// at PlayerOps.ts:778. NAI-127 Bundle 2.
func (p *Player) ApplyDamage(amount, dmgType int) {
	p.Damage(amount, dmgType)
}

// AddWealthEvent appends evt to this player's in-memory wealth log.
// Mirrors TS Player.addWealthEvent. Per NAI-162-D-WEALTHEVENT-IN-MEMORY-ONLY,
// goscape does not emit an analytics RPC; the log is a queryable
// in-memory record only.
//
// NAI-162-D-WEALTHEVENT-IN-MEMORY-ONLY — CONFIRMED EXCEPTION (closes
// logger-transport-5 + world-tick-8 from the 2026-05-28 fresh audit).
// TS World.addWealthEvent (World.ts:2233-2263) does two things this Go
// path intentionally omits:
//
//  1. Filtering: TS drops wealth events whose type is in
//     filteredEventTypes (DROP/PICKUP) when value <
//     Environment.NODE_MINIMUM_WEALTH_VALUE_EVENT, suppressing low-value
//     noise from the downstream analytics sink.
//  2. Per-tick grouping: TS coalesces successive events whose type is in
//     groupedEventTypes (DEATH / PVP / PARTY_ROOM) into one analytics
//     record per (player, tick), summing values and concatenating
//     extra-item lists.
//
// Both behaviours exist exclusively to shape the downstream Logger
// analytics dispatch — TS's per-tick flushWealth pushes the
// post-filter, post-group buffer into the moderator-facing
// wealth-event pipeline. goscape opted out of that dispatch (NAI-162
// B2): the wealthLog is a queryable in-memory record only — no
// network sink, no analytics RPC, no third-party consumer that
// depends on the filtered + grouped shape. Filtering would silently
// drop entries that scripts and tests can legitimately observe via
// the in-memory accessor; grouping would change the slice cardinality
// that callers iterate over. Neither porting move makes sense without
// a consuming analytics pipeline, which is itself out of scope for
// the engine port (see the broader NAI-72/73/74/Phase2 logger-bridge
// deferrals in docs/superpowers/audits/2026-05-28-ts-parity-audit-fresh
// -coverage.md).
//
// If a future revision wires a real wealth-event analytics consumer,
// the filtering/grouping belongs at the dispatch boundary (a Logger-
// equivalent module's submit method, applied per outbound batch), not
// here on the per-event append path that scripts observe. Keep this
// helper one-line and side-effect-free until then.
//
// rev-254 A3 — session_uuid re-key (TS 43e02957..2e3bcf43):
// Player.addWealthEvent stamps `session_uuid: this.session`
// (Player.ts:653-658 @2e3bcf43); the 244-era account_id /
// account_session pair is gone (WealthEvent.ts:18-21) and the
// NetworkPlayer addWealthEvent override (with its isClientConnected /
// 'disconnected' fork) was deleted — the parent impl handles all
// players. goscape mirrors the TS field-default semantics: p.session
// is the per-login UUID when assigned, 'headless' otherwise (the TS
// Player.ts:311 ctor default).
//
// Coord is NOT embedded in the in-memory WealthEvent (goscape's internal
// shape); it is computed at the analytics-dispatch boundary if/when a
// real wealth-event consumer is wired (NAI-162-D-WEALTHEVENT-IN-MEMORY-ONLY).
func (p *Player) AddWealthEvent(evt script.WealthEvent) {
	evt.SessionUUID = p.sessionOrHeadless()
	p.wealthLog = append(p.wealthLog, evt)
}

// LastLoginInfo emits a LAST_LOGIN_INFO server packet with the
// previous-login timestamp and IP. Mirrors TS Player.lastLoginInfo
// (Player.ts:2190-2200).
//
// First call (lastLoginTime==0): daysSinceLogin computes to 0 because
// lastDate falls back to now. Subsequent calls compute integer days
// between previous lastLoginTime and now. After writing, lastLoginTime
// advances to now.
//
// lastIp is hardcoded to 127.0.0.1 (2130706433) and
// daysSinceRecoveriesChanged to 201 ("hide :)") per TS Player.ts:2194,2196.
//
// warnMembersInNonMembers: TS Player.ts:2197
// `!Environment.NODE_MEMBERS && this.members` — true when the world is
// a non-members world and the player has a members subscription (warns them
// members content is unavailable). Requires server config access; if
// p.client or p.client.server is nil (test context), resolves to false.
// rev-244-b2: placeholder — the B3 Player.ts:2198 producer surface is
// not yet ported; warnMembersInNonMembers is derived from available
// state and is correct for the non-B3 surface.
func (p *Player) LastLoginInfo() {
	now := time.Now().UnixMilli()
	lastDate := p.lastLoginTime
	if lastDate == 0 {
		lastDate = now
	}
	lastIp := int32(2130706433) // 127.0.0.1
	const dayMillis = int64(1000 * 60 * 60 * 24)
	daysSinceLogin := int((now - lastDate) / dayMillis)
	daysSinceRecoveriesChanged := 201

	// warnMembersInNonMembers: !NODE_MEMBERS && player.members.
	// TS Player.ts:2197 `!Environment.NODE_MEMBERS && this.members`.
	var warnMembersInNonMembers bool
	if p.client != nil && p.client.server != nil {
		warnMembersInNonMembers = !p.client.server.cfg.NodeMembers && p.members
	}

	var warnByte byte
	if warnMembersInNonMembers {
		warnByte = 1
	}
	payload := []byte{
		byte(lastIp >> 24), byte(lastIp >> 16), byte(lastIp >> 8), byte(lastIp), // p4: lastIp
		byte(daysSinceLogin >> 8), byte(daysSinceLogin), // p2: daysSinceLogin
		byte(daysSinceRecoveriesChanged),                // p1
		byte(p.messageCount >> 8), byte(p.messageCount), // p2: messageCount
		warnByte, // pbool: warnMembersInNonMembers (TS Player.ts:2197)
	}
	p.writeOut(gameserver.OpLastLoginInfo, payload)
	p.lastLoginTime = now
}

// InvTotalParamStack sums slot.count × objType.Params[paramID] (falling
// back to paramType.DefaultInt when the key is absent) across every
// non-empty slot of the inventory at invID. Returns zero on nil client,
// missing inventory, or out-of-range paramID.
//
// Mirrors TS Player._invTotalParam(inv, param, stack=true)
// at Player.ts:1668-1694.
//
// (goscape defensive; TS skips this check) Nil-client guard mirrors
// other player methods that operate via server resolution.
func (p *Player) InvTotalParamStack(invID, paramID int) int {
	if p.client == nil || p.client.server == nil {
		return 0
	}
	s := p.client.server
	if s.paramTypes == nil || paramID < 0 || paramID >= len(s.paramTypes.Configs) {
		return 0
	}
	pt := s.paramTypes.Configs[paramID]
	if pt == nil {
		return 0
	}
	// Resolve inventory directly (avoids depending on invLookup.s being set).
	// Per-player inv: p.invs[invID]. World-shared inv: s.invs[invID].
	// Mirrors invLookupView.Get scope logic at server_invs.go.
	var inv *inventory.Inventory
	if s.invTypes != nil && invID >= 0 && invID < len(s.invTypes.Configs) {
		cfg := s.invTypes.Configs[invID]
		if cfg != nil && cfg.Scope == objtype.InvTypeScopeShared {
			if s.invs != nil {
				inv = s.invs[invID]
			}
		} else {
			if p.invs != nil {
				inv = p.invs[invID]
			}
		}
	} else if p.invs != nil {
		inv = p.invs[invID]
	}
	if inv == nil {
		return 0
	}
	total := 0
	for _, item := range inv.Items {
		if item == nil || item.Id < 0 {
			continue
		}
		if s.objTypes == nil || item.Id >= len(s.objTypes.Configs) {
			continue
		}
		ot := s.objTypes.Configs[item.Id]
		if ot == nil {
			continue
		}
		var paramVal int
		if v, ok := ot.Params[uint32(paramID)]; ok {
			if iv, ok2 := v.(uint32); ok2 {
				// Params are stored as raw uint32 wire bytes; cast through
				// int32 to sign-extend negative values (matches paramLookup
				// in pkg/script/handlers_config.go).
				paramVal = int(int32(iv))
			}
		} else {
			paramVal = int(pt.DefaultInt)
		}
		total += item.Count * paramVal
	}
	return total
}
