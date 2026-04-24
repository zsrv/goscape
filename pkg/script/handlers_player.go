package script

import (
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
)

// NumStats is the authentic skill count in rev 225. Stat ops validate
// that the requested id is within [0, NumStats) and return an error
// otherwise; they do not silently clamp.
const NumStats = 21

// unpackCoord returns (level, x, z) from the packed coord int used by
// TS CoordGrid.packCoord: `(level << 28) | (x << 14) | z` with a 2-bit
// level mask (rev 225 only has 4 levels) and 14-bit x/z masks.
func unpackCoord(c int) (level, x, z int) {
	level = (c >> 28) & 0x3
	x = (c >> 14) & 0x3fff
	z = c & 0x3fff
	return
}

// checkStatID validates id is a valid skill slot. Used by every stat op.
func checkStatID(id int, op string) error {
	if id < 0 || id >= NumStats {
		return errors.New(op + ": stat id out of range")
	}
	return nil
}

// requireActivePlayer is a one-line guard to keep handler bodies tidy.
// Every handler that dereferences s.Self calls this first.
func requireActivePlayer(s *ScriptState, op string) error {
	if s.Pointers&PtrActivePlayer == 0 || s.Self == nil {
		return errors.New(op + ": no active player")
	}
	return nil
}

// requireProtectedActivePlayer is requireActivePlayer plus a check that
// the script was started with protect=true. Used by opcodes that TS
// wraps in checkedHandler(ProtectedActivePlayer, ...) — currently
// P_OPLOC and P_OPNPC (S6w closure of S6v-D1). Chains through to
// requireActivePlayer first so the "no active player" error message
// matches the unprotected variant.
func requireProtectedActivePlayer(s *ScriptState, op string) error {
	if err := requireActivePlayer(s, op); err != nil {
		return err
	}
	if !s.Protect {
		return errors.New(op + ": script not protected")
	}
	return nil
}

// -- Stat read ops -------------------------------------------------------

// handleStat pushes the active player's current (boosted/drained) level
// for the popped stat id.
func handleStat(s *ScriptState) error {
	if err := requireActivePlayer(s, "STAT"); err != nil {
		return err
	}
	id := s.PopInt()
	if err := checkStatID(id, "STAT"); err != nil {
		return err
	}
	s.PushInt(s.Self.Stat(id))
	return nil
}

// handleStatBase pushes the player's base level for the popped stat id.
func handleStatBase(s *ScriptState) error {
	if err := requireActivePlayer(s, "STAT_BASE"); err != nil {
		return err
	}
	id := s.PopInt()
	if err := checkStatID(id, "STAT_BASE"); err != nil {
		return err
	}
	s.PushInt(s.Self.StatBase(id))
	return nil
}

// handleStatTotal pushes the sum of all base levels. Per the spec we
// iterate via StatBase to avoid adding another interface method.
func handleStatTotal(s *ScriptState) error {
	if err := requireActivePlayer(s, "STAT_TOTAL"); err != nil {
		return err
	}
	total := 0
	for i := 0; i < NumStats; i++ {
		total += s.Self.StatBase(i)
	}
	s.PushInt(total)
	return nil
}

// -- Stat mutation ops ---------------------------------------------------
//
// TS `popInts(3)` fills [stat, constant, percent] top-down: the stack
// top is `percent`, middle is `constant`, bottom is `stat`. We mirror
// that pop order exactly.

// handleStatAdd implements STAT_ADD.
// TS formula (PlayerOps.ts:501-519):
//
//	added = current + ((constant + (base*percent)/100) | 0)
//	levels[stat] = min(added, 255)
func handleStatAdd(s *ScriptState) error {
	if err := requireActivePlayer(s, "STAT_ADD"); err != nil {
		return err
	}
	percent := s.PopInt()
	constant := s.PopInt()
	id := s.PopInt()
	if err := checkStatID(id, "STAT_ADD"); err != nil {
		return err
	}
	base := s.Self.StatBase(id)
	cur := s.Self.Stat(id)
	added := cur + (constant + (base*percent)/100)
	if added > 255 {
		added = 255
	}
	s.Self.SetCurLevel(id, added)
	return nil
}

// handleStatSub implements STAT_SUB.
// TS formula (PlayerOps.ts:521-536):
//
//	subbed = current - ((constant + (base*percent)/100) | 0)
//	levels[stat] = max(subbed, 0)
func handleStatSub(s *ScriptState) error {
	if err := requireActivePlayer(s, "STAT_SUB"); err != nil {
		return err
	}
	percent := s.PopInt()
	constant := s.PopInt()
	id := s.PopInt()
	if err := checkStatID(id, "STAT_SUB"); err != nil {
		return err
	}
	base := s.Self.StatBase(id)
	cur := s.Self.Stat(id)
	subbed := cur - (constant + (base*percent)/100)
	if subbed < 0 {
		subbed = 0
	}
	s.Self.SetCurLevel(id, subbed)
	return nil
}

// handleStatBoost implements STAT_BOOST.
// TS formula (PlayerOps.ts:538-558):
//
//	boost = (constant + (base*percent)/100) | 0
//	boosted = max(min(current + boost, base + boost), current)
//	levels[stat] = min(boosted, 255)
//
// The max(..., current) clamp means a boost never lowers the stat —
// useful when the stat is already boosted above base + boost.
func handleStatBoost(s *ScriptState) error {
	if err := requireActivePlayer(s, "STAT_BOOST"); err != nil {
		return err
	}
	percent := s.PopInt()
	constant := s.PopInt()
	id := s.PopInt()
	if err := checkStatID(id, "STAT_BOOST"); err != nil {
		return err
	}
	base := s.Self.StatBase(id)
	cur := s.Self.Stat(id)
	boost := constant + (base*percent)/100
	boosted := cur + boost
	if ceiling := base + boost; boosted > ceiling {
		boosted = ceiling
	}
	if boosted < cur {
		boosted = cur
	}
	if boosted > 255 {
		boosted = 255
	}
	s.Self.SetCurLevel(id, boosted)
	return nil
}

// handleStatDrain implements STAT_DRAIN. Unlike STAT_SUB, the percent
// is applied to the CURRENT level (not base).
//
// TS formula (PlayerOps.ts:560-575):
//
//	subbed = current - ((constant + (current*percent)/100) | 0)
//	levels[stat] = max(subbed, 0)
func handleStatDrain(s *ScriptState) error {
	if err := requireActivePlayer(s, "STAT_DRAIN"); err != nil {
		return err
	}
	percent := s.PopInt()
	constant := s.PopInt()
	id := s.PopInt()
	if err := checkStatID(id, "STAT_DRAIN"); err != nil {
		return err
	}
	cur := s.Self.Stat(id)
	subbed := cur - (constant + (cur*percent)/100)
	if subbed < 0 {
		subbed = 0
	}
	s.Self.SetCurLevel(id, subbed)
	return nil
}

// handleStatHeal implements STAT_HEAL.
// TS formula (PlayerOps.ts:596-616):
//
//	healed = current + ((constant + (base*percent)/100) | 0)
//	levels[stat] = max(min(healed, base), current)
//
// The max(..., current) clamp means healing never drops the stat below
// its current (boosted) value; min(..., base) caps at base.
func handleStatHeal(s *ScriptState) error {
	if err := requireActivePlayer(s, "STAT_HEAL"); err != nil {
		return err
	}
	percent := s.PopInt()
	constant := s.PopInt()
	id := s.PopInt()
	if err := checkStatID(id, "STAT_HEAL"); err != nil {
		return err
	}
	base := s.Self.StatBase(id)
	cur := s.Self.Stat(id)
	healed := cur + (constant + (base*percent)/100)
	if healed > base {
		healed = base
	}
	if healed < cur {
		healed = cur
	}
	s.Self.SetCurLevel(id, healed)
	return nil
}

// handleStatAdvance implements STAT_ADVANCE. TS pops (stat, xp); stack
// top is xp. The handler just forwards to Self.AddXP — scaling is the
// Player implementation's responsibility.
func handleStatAdvance(s *ScriptState) error {
	if err := requireActivePlayer(s, "STAT_ADVANCE"); err != nil {
		return err
	}
	xp := s.PopInt()
	id := s.PopInt()
	if err := checkStatID(id, "STAT_ADVANCE"); err != nil {
		return err
	}
	s.Self.AddXP(id, xp)
	return nil
}

// handleStatRandom implements STAT_RANDOM. TS uses JavaRandom's next
// double * 256; we use math/rand/v2.IntN(256) — close enough for
// smoke-testing script flow but *not* bit-identical.
//
// TS formula (PlayerOps.ts:578-586):
//
//	value = floor(low*(99-level)/98) + floor(high*(level-1)/98) + 1
//	chance = floor(random * 256)          // [0, 255]
//	pushInt(value > chance ? 1 : 0)
//
// probability tuning TBD — revisit once we have authentic RNG seeds.
func handleStatRandom(s *ScriptState) error {
	if err := requireActivePlayer(s, "STAT_RANDOM"); err != nil {
		return err
	}
	high := s.PopInt()
	low := s.PopInt()
	id := s.PopInt()
	if err := checkStatID(id, "STAT_RANDOM"); err != nil {
		return err
	}
	level := s.Self.Stat(id)
	value := (low*(99-level))/98 + (high*(level-1))/98 + 1
	chance := rand.IntN(256)
	if value > chance {
		s.PushInt(1)
	} else {
		s.PushInt(0)
	}
	return nil
}

// -- Coord / facing / teleport ops --------------------------------------

// handleCoord pushes the player's packed coord.
func handleCoord(s *ScriptState) error {
	if err := requireActivePlayer(s, "COORD"); err != nil {
		return err
	}
	s.PushInt(s.Self.CoordPacked())
	return nil
}

// handleFaceSquare pops a packed coord and calls Self.FaceSquare(x, z).
// The level component of the packed coord is ignored — facing is always
// on the player's current level.
func handleFaceSquare(s *ScriptState) error {
	if err := requireActivePlayer(s, "FACESQUARE"); err != nil {
		return err
	}
	_, x, z := unpackCoord(s.PopInt())
	s.Self.FaceSquare(x, z)
	return nil
}

// handlePTeleport pops a packed coord and calls Self.Teleport(x, z, level).
func handlePTeleport(s *ScriptState) error {
	if err := requireProtectedActivePlayer(s, "P_TELEPORT"); err != nil {
		return err
	}
	level, x, z := unpackCoord(s.PopInt())
	s.Self.Teleport(x, z, level)
	return nil
}

// handlePTeleJump pops a packed coord and calls Self.TeleJump(x, z, level).
func handlePTeleJump(s *ScriptState) error {
	if err := requireProtectedActivePlayer(s, "P_TELEJUMP"); err != nil {
		return err
	}
	level, x, z := unpackCoord(s.PopInt())
	s.Self.TeleJump(x, z, level)
	return nil
}

// handlePWalk is a stub. Real implementation requires pathfinder +
// waypoint queue integration; pops the coord, logs, and returns nil.
func handlePWalk(s *ScriptState) error {
	_ = s.PopInt()
	slog.Debug("P_WALK stub invoked; pathfinder integration pending",
		"script", s.Script.Name, "pc", s.PC)
	return nil
}

// -- Animation ops -------------------------------------------------------

// handleAnim implements ANIM. TS pops (seq, delay); stack top is delay.
func handleAnim(s *ScriptState) error {
	if err := requireActivePlayer(s, "ANIM"); err != nil {
		return err
	}
	delay := s.PopInt()
	seq := s.PopInt()
	s.Self.PlayAnim(seq, delay)
	return nil
}

// handleSpotAnimPl implements SPOTANIM_PL. TS pops three ints; stack
// top is delay, then height, then spotanim id at bottom.
func handleSpotAnimPl(s *ScriptState) error {
	if err := requireActivePlayer(s, "SPOTANIM_PL"); err != nil {
		return err
	}
	delay := s.PopInt()
	height := s.PopInt()
	spotanim := s.PopInt()
	s.Self.PlaySpotAnim(spotanim, height, delay)
	return nil
}

// handleReadyAnim implements READYANIM — pops a seq id, stores as the
// player's idle/stand animation.
func handleReadyAnim(s *ScriptState) error {
	if err := requireActivePlayer(s, "READYANIM"); err != nil {
		return err
	}
	s.Self.SetReadyAnim(s.PopInt())
	return nil
}

// handleTurnAnim implements TURNANIM.
func handleTurnAnim(s *ScriptState) error {
	if err := requireActivePlayer(s, "TURNANIM"); err != nil {
		return err
	}
	s.Self.SetTurnAnim(s.PopInt())
	return nil
}

// handleWalkAnim implements WALKANIM.
func handleWalkAnim(s *ScriptState) error {
	if err := requireActivePlayer(s, "WALKANIM"); err != nil {
		return err
	}
	s.Self.SetWalkAnim(s.PopInt())
	return nil
}

// handleWalkAnimB implements WALKANIM_B (backward).
func handleWalkAnimB(s *ScriptState) error {
	if err := requireActivePlayer(s, "WALKANIM_B"); err != nil {
		return err
	}
	s.Self.SetWalkAnimB(s.PopInt())
	return nil
}

// handleWalkAnimL implements WALKANIM_L (strafe left).
func handleWalkAnimL(s *ScriptState) error {
	if err := requireActivePlayer(s, "WALKANIM_L"); err != nil {
		return err
	}
	s.Self.SetWalkAnimL(s.PopInt())
	return nil
}

// handleWalkAnimR implements WALKANIM_R (strafe right).
func handleWalkAnimR(s *ScriptState) error {
	if err := requireActivePlayer(s, "WALKANIM_R"); err != nil {
		return err
	}
	s.Self.SetWalkAnimR(s.PopInt())
	return nil
}

// handleRunAnim implements RUNANIM.
func handleRunAnim(s *ScriptState) error {
	if err := requireActivePlayer(s, "RUNANIM"); err != nil {
		return err
	}
	s.Self.SetRunAnim(s.PopInt())
	return nil
}

// S5h: action-clear.

func handlePStopAction(s *ScriptState) error {
	if err := requireProtectedActivePlayer(s, "P_STOPACTION"); err != nil {
		return err
	}
	s.Self.StopAction()
	return nil
}

func handlePClearPendingAction(s *ScriptState) error {
	if err := requireProtectedActivePlayer(s, "P_CLEARPENDINGACTION"); err != nil {
		return err
	}
	s.Self.ClearPendingAction()
	return nil
}

// handlePApRange pops the approach range (in tiles) and sets it on
// the active player along with apRangeCalled=true. Called from APLOC
// trigger scripts to extend the approach-distance at which the trigger
// re-fires. Matches TS PlayerOps.ts:P_APRANGE.
//
// No clamping or bounds check: TS is permissive (any int accepted).
// Negative values functionally disable the trigger
// (inApproachDistance returns false for apRange<=0) — scripts passing
// negative are misconfigured, not a security concern.
func handlePApRange(s *ScriptState) error {
	if err := requireProtectedActivePlayer(s, "P_APRANGE"); err != nil {
		return err
	}
	n := s.PopInt()
	s.Self.SetApRange(n)
	return nil
}

// -- p_op* script-queued interaction anchoring (S6v) --------------------

// handleP_OpLoc (P_OPLOC, opcode 2077) re-anchors the active player on
// the active loc with AP trigger APLOC<op>. Matches TS
// PlayerOps.ts:386-402.
//
// S6v-D1 closed in S6w: gates on ScriptState.Protect via
// requireProtectedActivePlayer, matching TS checkedHandler(ProtectedActivePlayer).
func handleP_OpLoc(s *ScriptState) error {
	if err := requireProtectedActivePlayer(s, "P_OPLOC"); err != nil {
		return err
	}
	if s.ActiveLoc == nil {
		return errors.New("P_OPLOC: no active loc")
	}
	op := s.PopInt()
	if op < 1 || op > 5 {
		return fmt.Errorf("P_OPLOC: invalid op %d (must be 1..5)", op)
	}
	s.Self.StopAction()
	s.Self.SetInteractionScriptLoc(s.ActiveLoc, op)
	return nil
}

// handleP_OpNpc (P_OPNPC, opcode 2078) re-anchors on the active npc.
// Matches TS PlayerOps.ts:404-415.
//
// S6v-D1 closed in S6w: gates on ScriptState.Protect via
// requireProtectedActivePlayer, matching TS checkedHandler(ProtectedActivePlayer).
func handleP_OpNpc(s *ScriptState) error {
	if err := requireProtectedActivePlayer(s, "P_OPNPC"); err != nil {
		return err
	}
	if s.ActiveNpc == nil {
		return errors.New("P_OPNPC: no active npc")
	}
	op := s.PopInt()
	if op < 1 || op > 5 {
		return fmt.Errorf("P_OPNPC: invalid op %d (must be 1..5)", op)
	}
	s.Self.StopAction()
	s.Self.SetInteractionScriptNpc(s.ActiveNpc, op)
	return nil
}

// handleFindUID (opcode 2019) pops a uid, looks up the logged-in player
// with that uid, and rebinds Self on success. Pushes 1 if found, 0 if
// the lookup returned nil or no PlayerLookup is configured.
//
// Does NOT check CanAccess — that's P_FINDUID's job. Does NOT set
// Protect. Mirrors TS PlayerOps.ts:60-72 with goscape's collapsed
// pointer model (single PtrActivePlayer).
func handleFindUID(s *ScriptState) error {
	uid := s.PopInt()
	if s.PlayerLookup == nil {
		s.PushInt(0)
		return nil
	}
	target := s.PlayerLookup.LookupPlayerByUID(uid)
	if target == nil {
		s.PushInt(0)
		return nil
	}
	s.Self = target
	s.Pointers |= PtrActivePlayer
	s.PushInt(1)
	return nil
}

// handlePFindUID (opcode 2073) is P_FINDUID — the protected variant of
// FINDUID. Pops a uid, tries to rebind Self with protected access.
// Three outcomes:
//   - Self-reacquire fast-path: script already runs protected on a
//     player whose UID matches → push 1, no state change, no lookup.
//   - Lookup miss OR target.CanAccess()==false → push 0.
//   - Success → Self rebinds, PtrActivePlayer set, Protect=true, push 1.
//
// Mirrors TS PlayerOps.ts:75-94 with goscape's collapsed pointer model
// (single PtrActivePlayer + ScriptState.Protect bool).
func handlePFindUID(s *ScriptState) error {
	uid := s.PopInt()
	// Self-reacquire fast-path: already protected on this player.
	if s.Protect && s.Self != nil && s.Self.UID() == uid {
		s.PushInt(1)
		return nil
	}
	if s.PlayerLookup == nil {
		s.PushInt(0)
		return nil
	}
	target := s.PlayerLookup.LookupPlayerByUID(uid)
	if target == nil || !target.CanAccess() {
		s.PushInt(0)
		return nil
	}
	s.Self = target
	s.Pointers |= PtrActivePlayer
	s.Protect = true
	s.PushInt(1)
	return nil
}
