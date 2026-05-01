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

// requireActivePlayer2 is the dual-pin validator for the secondary
// active-player slot (Self2). Every handler that dereferences s.Self2
// calls this first. NAI-39.
func requireActivePlayer2(s *ScriptState, op string) error {
	if s.Pointers&PtrActivePlayer2 == 0 || s.Self2 == nil {
		return errors.New(op + ": no active player2")
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

// checkNotNull mirrors TS NumberNotNull (ScriptValidators.ts:36-41) — rejects
// the script "null number" sentinel -1, accepts every other int. Used by
// handlers wrapping a popInt result with TS check(..., NumberNotNull).
func checkNotNull(v int, op string) error {
	if v == -1 {
		return fmt.Errorf("%s: input number was null(-1)", op)
	}
	return nil
}

// checkStringNotNull mirrors TS StringNotNull
// (ScriptInputStringNotNullValidator at ScriptValidators.ts:50-55) —
// rejects empty strings, accepts any non-empty string. Used by handlers
// wrapping a popString result with TS check(..., StringNotNull). TS
// error literal: "An input string was null(-1)." — goscape drops the
// "(-1)" suffix since strings have no -1 sentinel (the sentinel is "").
func checkStringNotNull(v, op string) error {
	if v == "" {
		return fmt.Errorf("%s: input string was null", op)
	}
	return nil
}

// checkInvType mirrors TS InvTypeValid (ScriptValidators.ts:122) — a
// ScriptInputConfigTypeValidator over InvType. Both the range check
// (0 <= id < InvType.count) and the registry-present check collapse
// into "s.Configs.InvType(id) != nil" per the Configs interface contract
// at configs.go:7 ("return nil when the type isn't loaded or the id is
// out of range"). State-aware signature diverges from sibling check
// helpers because the bound is runtime-loaded.
func checkInvType(s *ScriptState, id int, op string) error {
	if s.Configs == nil || s.Configs.InvType(id) == nil {
		return fmt.Errorf("%s: no InvType with value (%d) found", op, id)
	}
	return nil
}

// handlePAnimProtect (P_ANIMPROTECT, opcode 2066) sets the active player's
// animProtect flag. While nonzero, in-engine animation requests should be
// suppressed (TS Player.ts:1842 — reader path not yet ported in goscape;
// tracked as S7b-D1). Mirrors TS PlayerOps.ts:1171-1172.
func handlePAnimProtect(s *ScriptState) error {
	if err := requireProtectedActivePlayer(s, "P_ANIMPROTECT"); err != nil {
		return err
	}
	v := s.PopInt()
	if err := checkNotNull(v, "P_ANIMPROTECT"); err != nil {
		return err
	}
	s.Self.SetAnimProtect(v)
	return nil
}

// handleAllowDesign (ALLOWDESIGN, opcode 2001) sets the active player's
// allowDesign flag. Pops one int, rejects -1 via NumberNotNull, and stores
// (v == 1) as a bool. Gate is ActivePlayer (not Protected). Mirrors TS
// PlayerOps.ts:1022-1024. The gate permits `IdkSaveDesign` inbound packets
// (character-design recustomise) — reader path unported, see S7e-D1.
func handleAllowDesign(s *ScriptState) error {
	if err := requireActivePlayer(s, "ALLOWDESIGN"); err != nil {
		return err
	}
	v := s.PopInt()
	if err := checkNotNull(v, "ALLOWDESIGN"); err != nil {
		return err
	}
	s.Self.SetAllowDesign(v == 1)
	return nil
}

// handleBuildAppearance (BUILDAPPEARANCE, opcode 2004) validates the popped
// InvType id and stages an appearance refresh on the active player. Mirrors
// TS PlayerOps.ts:202-204. Gate is ActivePlayer (not Protected). Validator
// mirrors TS InvTypeValid. The setter writes both Player.appearanceInv and
// flags MaskAppearance — MaskAppearance is consumed by tick.go:325-335 which
// regenerates the appearance buffer. NAI-21 Bundle 1 closed S7c-D1:
// generateAppearance now honors p.appearanceInv.
func handleBuildAppearance(s *ScriptState) error {
	if err := requireActivePlayer(s, "BUILDAPPEARANCE"); err != nil {
		return err
	}
	id := s.PopInt()
	if err := checkInvType(s, id, "BUILDAPPEARANCE"); err != nil {
		return err
	}
	s.Self.SetAppearanceInv(id)
	return nil
}

// handleSetIdKit (SETIDKIT, opcode 2100) sets one body-part slot on the
// active player's appearance. Pops (idkit int, color int) from the stack.
// Validates idkit via Configs.IdkType; writes body[slot] and
// colors[colorSlot] (slot adjusted for gender). Script must call
// BUILDAPPEARANCE separately to trigger the appearance rebuild.
// Mirrors TS PlayerOps.ts:1066-1106.
func handleSetIdKit(s *ScriptState) error {
	if err := requireActivePlayer(s, "SETIDKIT"); err != nil {
		return err
	}
	color := s.PopInt()
	idkit := s.PopInt()
	if s.Configs == nil {
		return fmt.Errorf("SETIDKIT: invalid idkit %d", idkit)
	}
	idk := s.Configs.IdkType(idkit)
	if idk == nil {
		return fmt.Errorf("SETIDKIT: invalid idkit %d", idkit)
	}
	gender := s.Self.Gender()
	slot := idk.Type
	if gender == 1 {
		slot -= 7
	}
	s.Self.SetBodyPart(slot, idkit)
	if cs := idkColorSlot(slot); cs >= 0 {
		s.Self.SetColorPart(cs, color)
	}
	return nil
}

// idkColorSlot maps the gender-adjusted body-part type (0-6) to the
// colors array index. Returns -1 when no color write is needed (type=4,
// hands/skin — set via SETSKINCOLOUR instead).
// Mirrors TS PlayerOps.ts:1082-1103 color-slot mapping.
func idkColorSlot(t int) int {
	switch t {
	case 0, 1:
		return 0 // hair / jaw
	case 2, 3:
		return 1 // torso / arms
	case 5:
		return 2 // legs
	case 6:
		return 3 // feet
	default:
		return -1 // type=4 (hands/skin): no color write
	}
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
	if err := checkNotNull(percent, "STAT_ADD"); err != nil {
		return err
	}
	constant := s.PopInt()
	if err := checkNotNull(constant, "STAT_ADD"); err != nil {
		return err
	}
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
	if err := checkNotNull(percent, "STAT_SUB"); err != nil {
		return err
	}
	constant := s.PopInt()
	if err := checkNotNull(constant, "STAT_SUB"); err != nil {
		return err
	}
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
	if err := checkNotNull(percent, "STAT_BOOST"); err != nil {
		return err
	}
	constant := s.PopInt()
	if err := checkNotNull(constant, "STAT_BOOST"); err != nil {
		return err
	}
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
	if err := checkNotNull(percent, "STAT_DRAIN"); err != nil {
		return err
	}
	constant := s.PopInt()
	if err := checkNotNull(constant, "STAT_DRAIN"); err != nil {
		return err
	}
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
	if err := checkNotNull(percent, "STAT_HEAL"); err != nil {
		return err
	}
	constant := s.PopInt()
	if err := checkNotNull(constant, "STAT_HEAL"); err != nil {
		return err
	}
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
//
// TS asymmetry: STAT_ADVANCE wraps stat with NumberNotNull (not
// PlayerStatValid like sibling stat ops); both stat and xp get NumberNotNull
// (PlayerOps.ts:762-763). goscape mirrors that — the NumberNotNull wraps
// fire before checkStatID, which then enforces the [0, NumStats) bound.
func handleStatAdvance(s *ScriptState) error {
	if err := requireActivePlayer(s, "STAT_ADVANCE"); err != nil {
		return err
	}
	xp := s.PopInt()
	if err := checkNotNull(xp, "STAT_ADVANCE"); err != nil {
		return err
	}
	id := s.PopInt()
	if err := checkNotNull(id, "STAT_ADVANCE"); err != nil {
		return err
	}
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
// top is delay, then height, then spotanim id at bottom. TS wraps only
// delay with NumberNotNull (PlayerOps.ts:589); height and spotanim are
// raw popInts.
func handleSpotAnimPl(s *ScriptState) error {
	if err := requireActivePlayer(s, "SPOTANIM_PL"); err != nil {
		return err
	}
	delay := s.PopInt()
	if err := checkNotNull(delay, "SPOTANIM_PL"); err != nil {
		return err
	}
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

// handleWalkTrigger (P_WALKTRIGGER, opcode 2128) sets the active player's
// queued walktrigger script id. Pops one int. Mirrors TS PlayerOps.ts:1035-1037.
// Consumed by (*Player).processWalktrigger on the next interaction tick.
func handleWalkTrigger(s *ScriptState) error {
	if err := requireActivePlayer(s, "WALKTRIGGER"); err != nil {
		return err
	}
	s.Self.SetWalkTrigger(s.PopInt())
	return nil
}

// handleGetWalkTrigger (GETWALKTRIGGER, opcode 2023) pushes the active
// player's current walktrigger script id. Returns -1 when unset.
// Mirrors TS PlayerOps.ts:1039-1042.
func handleGetWalkTrigger(s *ScriptState) error {
	if err := requireActivePlayer(s, "GETWALKTRIGGER"); err != nil {
		return err
	}
	s.PushInt(s.Self.WalkTrigger())
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
// re-fires. Matches TS PlayerOps.ts:352-355.
//
// TS wraps with NumberNotNull (PlayerOps.ts:353); -1 is rejected.
// Other negatives functionally disable the trigger (inApproachDistance
// returns false for apRange<=0) but TS accepts them — scripts passing
// negative are misconfigured, not a security concern.
func handlePApRange(s *ScriptState) error {
	if err := requireProtectedActivePlayer(s, "P_APRANGE"); err != nil {
		return err
	}
	n := s.PopInt()
	if err := checkNotNull(n, "P_APRANGE"); err != nil {
		return err
	}
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
	if err := checkNotNull(op, "P_OPLOC"); err != nil {
		return err
	}
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
	if err := checkNotNull(op, "P_OPNPC"); err != nil {
		return err
	}
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

// handleMidiSong (MIDI_SONG, opcode 2064) plays a MIDI song by name to
// the active player. Silent no-op if the player has lowMemory set.
// Mirrors TS PlayerOps.ts:796-804.
//
// Pointer gate: require active_player (TS ScriptOpcodePointers.ts:272
// require: ['active_player']).
func handleMidiSong(s *ScriptState) error {
	name := s.PopString()
	if err := checkStringNotNull(name, "MIDI_SONG"); err != nil {
		return err
	}
	if err := requireActivePlayer(s, "MIDI_SONG"); err != nil {
		return err
	}
	if s.Self.LowMemory() {
		return nil
	}
	s.Self.PlaySong(name)
	return nil
}

// handleMidiJingle (MIDI_JINGLE, opcode 2063) plays a short MIDI jingle
// by name and delay to the active player. Silent no-op if the player
// has lowMemory set. Mirrors TS PlayerOps.ts:806-816.
//
// Pointer gate: require active_player (TS ScriptOpcodePointers.ts:269
// require: ['active_player']).
//
// Pop order (top-of-stack first): delay (NumberNotNull), then name
// (StringNotNull). Matches TS `check(state.popInt(), NumberNotNull)` /
// `check(state.popString(), StringNotNull)` evaluation order.
func handleMidiJingle(s *ScriptState) error {
	delay := s.PopInt()
	if err := checkNotNull(delay, "MIDI_JINGLE"); err != nil {
		return err
	}
	name := s.PopString()
	if err := checkStringNotNull(name, "MIDI_JINGLE"); err != nil {
		return err
	}
	if err := requireActivePlayer(s, "MIDI_JINGLE"); err != nil {
		return err
	}
	if s.Self.LowMemory() {
		return nil
	}
	s.Self.PlayJingle(delay, name)
	return nil
}

// handleHuntAll (HUNTALL, opcode 2031) pops [coord, distance, huntvis]
// and stores a HuntAll-mode PlayerIterator in s.playerIterator
// (consumed by HUNTNEXT 2032 in T5). Mirrors TS PlayerOps.ts:1215-1223.
//
// Pop order (top-of-stack first): huntvis, distance, coord.
// Validation: checkCoord, checkNotNull(distance), checkHuntVis.
// Nil-PlayerLookup degrades silently (matches NPC_HUNTALL convention).
func handleHuntAll(s *ScriptState) error {
	checkVis := s.PopInt()
	distance := s.PopInt()
	coord := s.PopInt()

	level, x, z, err := checkCoord(coord, "HUNTALL")
	if err != nil {
		return err
	}
	if err := checkNotNull(distance, "HUNTALL"); err != nil {
		return err
	}
	if err := checkHuntVis(checkVis, "HUNTALL"); err != nil {
		return err
	}

	if s.PlayerLookup == nil {
		return nil
	}
	s.playerIterator = NewHuntAllPlayerIterator(
		s.PlayerLookup, s.LineValidator, s.World.CurrentTick(),
		level, x, z, distance, checkVis,
	)
	return nil
}

// handleHuntNext (HUNTNEXT, opcode 2032) advances the active
// PlayerIterator and either sets active_player + pushes 1 on hit, or
// pushes 0 on miss / nil-iterator. Mirrors TS PlayerOps.ts:1226-1233
// and the analogous NPC handler at handlers_npc.go:641 (handleNpcFindNext).
//
// Active-player slot pattern (s.Self + Pointers |= PtrActivePlayer)
// mirrors FINDUID at handlers_player.go:683-684. Stale check uses
// strict-greater-than per iterator_state_pattern.md element 3.
//
// Exhaustion does NOT clear s.playerIterator (matches NPC_FINDNEXT
// behavior; iterator_state_pattern.md element 7). NAI-35-T5.
func handleHuntNext(s *ScriptState) error {
	it := s.playerIterator
	if it == nil {
		s.PushInt(0)
		return nil
	}
	if it.Stale(s.World.CurrentTick()) {
		return fmt.Errorf("HUNTNEXT: tried to use an old iterator. Create a new iterator instead.")
	}
	p, ok := it.Next()
	if !ok {
		s.PushInt(0)
		return nil
	}
	s.Self = p
	s.Pointers |= PtrActivePlayer
	s.PushInt(1)
	return nil
}

// handleHintNpc (HINT_NPC, opcode 2028) sends a HintArrow type=1 wire
// packet to the active player, pointing at the active NPC. Mirrors TS
// PlayerOps.ts:972-974:
//
//	state.activePlayer.hintNpc(state.activeNpc.nid)
//
// Full HintArrowEncoder coverage: HINT_NPC (type=1, NAI-37 T6),
// HINT_COORD (type=2..6, NAI-39), HINT_PL (type=10, NAI-39),
// HINT_STOP (type=-1, NAI-39).
func handleHintNpc(s *ScriptState) error {
	if err := requireActivePlayer(s, "HINT_NPC"); err != nil {
		return err
	}
	if err := requireActiveNpc(s, "HINT_NPC"); err != nil {
		return err
	}
	s.Self.HintNpc(s.ActiveNpc.Nid())
	return nil
}

// handleHintCoord (HINT_COORD, opcode 2027) sends a HintArrow type=2..6
// (TILE) wire packet to the active player at the unpacked coord. Pop
// order: [offset, coord, height] (per TS popInts(3) destructuring at
// PlayerOps.ts:867); goscape's PopInt order is height, coord, offset.
// Mirrors TS PlayerOps.ts:866-871. NAI-39.
func handleHintCoord(s *ScriptState) error {
	if err := requireActivePlayer(s, "HINT_COORD"); err != nil {
		return err
	}
	height := s.PopInt()
	coord := s.PopInt()
	offset := s.PopInt()
	_, x, z, err := checkCoord(coord, "HINT_COORD")
	if err != nil {
		return err
	}
	s.Self.HintCoord(offset, x, z, height)
	return nil
}

// handleHintPl (HINT_PL, opcode 2029) sends a HintArrow type=10 (PL)
// wire packet to the active player, pointing at the secondary
// active_player2 by slot. Mirrors TS PlayerOps.ts:976-978:
//
//	state.activePlayer.hintPlayer(state.activePlayer2.slot)
//
// Requires both active_player and active_player2 to be bound. NAI-39.
func handleHintPl(s *ScriptState) error {
	if err := requireActivePlayer(s, "HINT_PL"); err != nil {
		return err
	}
	if err := requireActivePlayer2(s, "HINT_PL"); err != nil {
		return err
	}
	s.Self.HintPlayer(s.Self2.Slot())
	return nil
}

// handleHintStop (HINT_STOP, opcode 2030) sends a HintArrow type=-1
// (STOP) wire packet to the active player, clearing any active hint.
// Mirrors TS PlayerOps.ts:873-875. NAI-39.
func handleHintStop(s *ScriptState) error {
	if err := requireActivePlayer(s, "HINT_STOP"); err != nil {
		return err
	}
	s.Self.HintStop()
	return nil
}
