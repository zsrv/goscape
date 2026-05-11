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
// the script holds the slot-0 protect flag (PtrProtectedActivePlayer).
// Used by opcodes that TS wraps in checkedHandler(ProtectedActivePlayer, ...)
// at intOperand=0. Chains through requireActivePlayer first so the
// "no active player" error message matches the unprotected variant.
func requireProtectedActivePlayer(s *ScriptState, op string) error {
	if err := requireActivePlayer(s, op); err != nil {
		return err
	}
	if s.Pointers&PtrProtectedActivePlayer == 0 {
		return errors.New(op + ": script not protected")
	}
	return nil
}

// requireProtectedActivePlayer2 is the slot-1 analogue of
// requireProtectedActivePlayer. Chains through requireActivePlayer2 first
// so error messages match the unprotected variant. Currently consumed
// only by BOTH_MOVEINV's secondary branch. NAI-133.
func requireProtectedActivePlayer2(s *ScriptState, op string) error {
	if err := requireActivePlayer2(s, op); err != nil {
		return err
	}
	if s.Pointers&PtrProtectedActivePlayer2 == 0 {
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

// checkLocAngle mirrors TS LocAngleValid (ScriptValidators.ts:106) — a
// ScriptInputRangeValidator over [LocAngle.WEST=0, LocAngle.SOUTH=3].
// Rejects values outside that range.
//
// Note: pkg/entity.Loc.Angle() is mask-bounded to [0,3] by construction
// ((l.CurrentInfo >> 19) & 0x3 at loc.go), so this validator is unreachable
// when fed from the entity layer. Retained for TS-fidelity parity per
// true_to_ts_gate.md — future ActiveLoc producers (e.g. LOC_FIND results
// from external sources) may bypass the bit mask.
func checkLocAngle(v int) error {
	if v < 0 || v > 3 {
		return fmt.Errorf("LocAngle out of range: %d", v)
	}
	return nil
}

// checkLocShape mirrors TS LocShapeValid (ScriptValidators.ts) — a
// ScriptInputRangeValidator over [LocShape.WALL_STRAIGHT=0,
// LocShape.GROUND_DECOR=22]. Rejects values outside that range.
//
// Note: pkg/entity.Loc.Shape() returns (l.CurrentInfo >> 14) & 0x1F which
// covers [0,31] — wider than the LocShape valid range. Caller wraps
// the error with "LOC_SHAPE: %w" so the script abort message names
// the opcode.
func checkLocShape(v int) error {
	if v < 0 || v > 22 {
		return fmt.Errorf("LocShape out of range: %d", v)
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

// checkSeqType validates a SeqType id is registered in s.Configs.
// Mirrors TS check(id, SeqTypeValid) (ScriptValidators.ts).
func checkSeqType(s *ScriptState, id int, op string) error {
	if s.Configs == nil || s.Configs.SeqType(id) == nil {
		return fmt.Errorf("%s: no SeqType with value (%d) found", op, id)
	}
	return nil
}

// handlePAnimProtect (P_ANIMPROTECT, opcode 2066) sets the active player's
// animProtect flag. While nonzero, the (*Player).PlayAnim reader gate at
// TS Player.ts:1842 suppresses in-engine animation requests (NAI-56).
// Mirrors TS PlayerOps.ts:1171-1172.
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

// handleDisplayName (DISPLAYNAME, opcode 2016) pushes the active
// player's display name. Mirrors TS PlayerOps.ts:235-237.
//
// Pointer gate: require active_player (TS ScriptOpcodePointers.ts
// :95-98 require: ['active_player']).
func handleDisplayName(s *ScriptState) error {
	if err := requireActivePlayer(s, "DISPLAYNAME"); err != nil {
		return err
	}
	s.PushString(s.Self.DisplayName())
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
	argCoord := s.PopInt()
	level, x, z := unpackCoord(argCoord)

	if s.NodeDebug {
		var (
			scriptName string
			selfCoord  int
			selfName   string
		)
		if s.Script != nil {
			scriptName = s.Script.Name
		}
		if s.Self != nil {
			selfCoord = s.Self.CoordPacked()
			selfName = s.Self.Username()
		}
		slog.Info("p_teleport",
			"script_name", scriptName,
			"script_pc", s.PC,
			"self_username", selfName,
			"self_coord_pre", selfCoord,
			"arg_coord", argCoord,
			"arg_x", x,
			"arg_z", z,
			"arg_level", level,
		)
	}

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

// handlePExactMove implements OpPExactMove (TS P_EXACTMOVE at
// PlayerOps.ts:881-890). Pops 5 ints, validates two packed coords,
// clears the map-flag, then calls ExactMove with horizontal coords only
// (TS-faithful: the unpacked `level` component is discarded —
// NAI-160-D-EXACTMOVE-COORDLEVEL-IGNORE per spec §3).
//
// Pop order: TS `state.popInts(5)` destructures
// [start, end, startCycle, endCycle, direction] from push order;
// direction is top-of-stack. Goscape pops top-first: dir → endCycle →
// startCycle → endPacked → startPacked. Critical per
// handler_pop_order_test_masking.md. NAI-160 T4.
func handlePExactMove(s *ScriptState) error {
	if err := requireProtectedActivePlayer(s, "P_EXACTMOVE"); err != nil {
		return err
	}
	direction := s.PopInt()
	endCycle := s.PopInt()
	startCycle := s.PopInt()
	endPacked := s.PopInt()
	startPacked := s.PopInt()
	_, sX, sZ, err := checkCoord(startPacked, "P_EXACTMOVE")
	if err != nil {
		return err
	}
	_, eX, eZ, err := checkCoord(endPacked, "P_EXACTMOVE")
	if err != nil {
		return err
	}
	s.Self.UnsetMapFlag()
	s.Self.ExactMove(sX, sZ, eX, eZ, startCycle, endCycle, direction)
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

// handlePRun implements P_RUN (opcode 2085). Pops the run-mode int and
// writes it to the player's run field, then mirrors it to the
// cache-resolved run-mode varp id (`RunVarpID()`, the config with
// `ClientCode==7`). Mirrors TS PlayerOps.ts:1204-1209 line-for-line.
//
// Two-step (field write + varp mirror) is intentional per
// ts_helper_method_bundles memory; TS itself flags the duplication
// with `// todo: better way to sync engine varp` (PlayerOps.ts:1207).
// Gate: ProtectedActivePlayer (TS checkedHandler).
//
// NAI-117 T1.
func handlePRun(s *ScriptState) error {
	if err := requireProtectedActivePlayer(s, "P_RUN"); err != nil {
		return err
	}
	v := s.PopInt()
	s.Self.SetRun(v)
	varpID := s.Self.RunVarpID()
	if s.NodeDebug && s.Log != nil {
		var (
			scriptName string
			tick       int
			varpPre    int32
		)
		if s.Script != nil {
			scriptName = s.Script.Name
		}
		if s.World != nil {
			tick = s.World.CurrentTick()
		}
		varpPre = s.Self.Varp(varpID)
		s.Log.Info("nai138.p_run",
			"script_name", scriptName,
			"script_pc", s.PC,
			"tick", tick,
			"value", v,
			"varp_id", varpID,
			"varp_pre", varpPre,
		)
	}
	// todo: better way to sync engine varp (mirrored from TS PlayerOps.ts:1207)
	s.Self.SetVarp(varpID, int32(v))
	return nil
}

// handleRunEnergy implements RUNENERGY (opcode 2096). Pushes the active
// player's current run-energy as an int (range [0, 10000]). Mirrors TS
// PlayerOps.ts:1175-1178. Gate: ActivePlayer (no Protected requirement).
//
// NAI-117 T2.
func handleRunEnergy(s *ScriptState) error {
	if err := requireActivePlayer(s, "RUNENERGY"); err != nil {
		return err
	}
	s.PushInt(s.Self.RunEnergy())
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

// handleSessionLog ports TS PlayerOps.ts:1184-1189 (SESSION_LOG opcode).
// Pops eventType (with TS +2 offset — script-content domain collapses
// the engine-only ENGINE/WEALTH values out, leaving 0=MODERATOR and
// 1=ADVENTURE for content authors) and event string, then dispatches
// to ActivePlayer.AddSessionLog. NAI-74.
func handleSessionLog(s *ScriptState) error {
	if err := requireActivePlayer(s, "SESSION_LOG"); err != nil {
		return err
	}
	eventType := s.PopInt() + 2
	event := s.PopString()
	s.Self.AddSessionLog(eventType, event)
	return nil
}

// handlePLogout (P_LOGOUT, opcode 2075) flags the active player for
// logout processing. The tick loop's processLogouts pass tears the
// session down at the next boundary. Mirrors TS PlayerOps.ts:622-624.
func handlePLogout(s *ScriptState) error {
	if err := requireProtectedActivePlayer(s, "P_LOGOUT"); err != nil {
		return err
	}
	s.Self.RequestLogout()
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
// S6v-D1 closed in S6w: gates on PtrProtectedActivePlayer via
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
// S6v-D1 closed in S6w: gates on PtrProtectedActivePlayer via
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

// handleFindUID resolves the popped uid via PlayerLookup and binds it
// to the slot selected by intOperand: 0 → Self + PtrActivePlayer,
// 1 → Self2 + PtrActivePlayer2. Pushes 1 on success, 0 on miss /
// nil-PlayerLookup. Errors on invalid intOperand. Does NOT check
// CanAccess; does NOT set PtrProtectedActivePlayer{,2} — that's
// P_FINDUID's job. Mirrors TS PlayerOps.ts:60-72 with goscape's
// collapsed pointer model. NAI-133.
func handleFindUID(s *ScriptState) error {
	operand := s.Script.IntOperands[s.PC]
	if operand != 0 && operand != 1 {
		return fmt.Errorf("FINDUID: invalid intOperand %d", operand)
	}
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
	if operand == 0 {
		s.Self = target
		s.Pointers |= PtrActivePlayer
	} else {
		s.Self2 = target
		s.Pointers |= PtrActivePlayer2
	}
	s.PushInt(1)
	return nil
}

// handlePFindUID is P_FINDUID — the protected variant of FINDUID. Pops
// a uid, tries to rebind the slot selected by intOperand with protected
// access. Three outcomes per slot:
//   - Self-reacquire fast-path: script already runs protected on a
//     player whose UID matches → push 1, no state change, no lookup.
//   - Lookup miss OR target.CanAccess()==false → push 0.
//   - Success → slot rebinds, both PtrActivePlayer{,2} and
//     PtrProtectedActivePlayer{,2} flags set, push 1.
//
// Mirrors TS PlayerOps.ts:75-94. NAI-133 added intOperand-based slot
// routing (closes latent `.p_finduid` clobber bug).
func handlePFindUID(s *ScriptState) error {
	operand := s.Script.IntOperands[s.PC]
	if operand != 0 && operand != 1 {
		return fmt.Errorf("P_FINDUID: invalid intOperand %d", operand)
	}
	uid := s.PopInt()

	// Self-reacquire fast-path: already protected on this slot's player.
	if operand == 0 {
		if s.Pointers&PtrProtectedActivePlayer != 0 && s.Self != nil && s.Self.UID() == uid {
			s.PushInt(1)
			return nil
		}
	} else {
		if s.Pointers&PtrProtectedActivePlayer2 != 0 && s.Self2 != nil && s.Self2.UID() == uid {
			s.PushInt(1)
			return nil
		}
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

	if operand == 0 {
		s.Self = target
		s.Pointers |= PtrActivePlayer | PtrProtectedActivePlayer
	} else {
		s.Self2 = target
		s.Pointers |= PtrActivePlayer2 | PtrProtectedActivePlayer2
	}
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

// handleSoundSynth (SOUND_SYNTH, opcode 2104) plays a synthesized
// sound effect to the active player. Silent no-op if the player has
// lowMemory set. Mirrors TS PlayerOps.ts:466-474.
//
// Pointer gate: require active_player (TS ScriptOpcodePointers.ts:434
// require: ['active_player']).
//
// Pop order (top-of-stack first per ScriptState.ts:325-331):
// delay, loops, synth. TS uses popInts(3) which fills the result
// slice from index amount-1 down to 0, so the destructured
// `[synth, loops, delay]` gets `synth = bottom-most pop`,
// `delay = first pop`. No check() validation — TS has none.
func handleSoundSynth(s *ScriptState) error {
	delay := s.PopInt()
	loops := s.PopInt()
	synth := s.PopInt()
	if err := requireActivePlayer(s, "SOUND_SYNTH"); err != nil {
		return err
	}
	if s.Self.LowMemory() {
		return nil
	}
	s.Self.PlaySynth(synth, loops, delay)
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

// handleTextGender implements TEXT_GENDER (opcode 4504). Mirrors TS
// PlayerOps.ts:787-794 — pops two strings (popStrings(2) destructures
// [male, female]; per ScriptState.ts:341-347 index 1 is popped first,
// so female is popped first off the stack, male second), then pushes
// male if gender==0 else female. No null-check on either string (TS
// does not call check(..., StringNotNull)). Pure stack op — no wire
// packet, no side effect.
func handleTextGender(s *ScriptState) error {
	if err := requireActivePlayer(s, "TEXT_GENDER"); err != nil {
		return err
	}
	female := s.PopString()
	male := s.PopString()
	if s.Self.Gender() == 0 {
		s.PushString(male)
	} else {
		s.PushString(female)
	}
	return nil
}

// handleP_OpObj (P_OPOBJ, opcode 2080) re-anchors the active player on
// the active obj with AP trigger APOBJ<op>. Pops 1-based op, validates
// [1,5], looks up ObjType.Op[op-1] and silently returns if empty.
// Else: StopAction → QueueWaypoint to obj tile → SetInteractionScriptObj.
//
// Mirrors TS PlayerOps.ts:990-1006 (op subtracted to 0-based in TS;
// goscape keeps 1-based throughout for consistency with sibling
// SetInteractionScript* signatures).
func handleP_OpObj(s *ScriptState) error {
	if err := requireProtectedActivePlayer(s, "P_OPOBJ"); err != nil {
		return err
	}
	if err := requireActiveObj(s, "P_OPOBJ"); err != nil {
		return err
	}
	op := s.PopInt()
	if err := checkNotNull(op, "P_OPOBJ"); err != nil {
		return err
	}
	if op < 1 || op > 5 {
		return fmt.Errorf("P_OPOBJ: invalid op %d (must be 1..5)", op)
	}
	if s.Configs == nil {
		return errors.New("P_OPOBJ: no configs")
	}
	objType := s.Configs.ObjType(s.ActiveObj.ObjType())
	if objType == nil {
		return fmt.Errorf("P_OPOBJ: invalid active obj type (%d)", s.ActiveObj.ObjType())
	}
	if op-1 >= len(objType.Op) || objType.Op[op-1] == "" {
		return nil // TS: type.op[op-1] === null → silent skip
	}
	x, z, _ := s.ActiveObj.Coords()
	s.Self.StopAction()
	s.Self.QueueWaypoint(x, z)
	s.Self.SetInteractionScriptObj(s.ActiveObj, op)
	return nil
}

// handleLowMem (LOWMEM, opcode 2061) pushes 1 if the active player's
// client requested low-memory mode at login, else 0. Mirrors TS
// PlayerOps.ts:1062-1064: pushes state.activePlayer.lowMemory ? 1 : 0.
func handleLowMem(s *ScriptState) error {
	if err := requireActivePlayer(s, "LOWMEM"); err != nil {
		return err
	}
	if s.Self.LowMemory() {
		s.PushInt(1)
	} else {
		s.PushInt(0)
	}
	return nil
}

// handleBusy2 (BUSY2, opcode 2006) pushes 1 if the active player has either
// an interaction target OR queued waypoints, else 0. Mirrors TS
// PlayerOps.ts:898-900 (https://x.com/JagexAsh/status/1791053667228856563):
//
//	state.pushInt(state.activePlayer.hasInteraction() ||
//	              state.activePlayer.hasWaypoints() ? 1 : 0);
//
// Gate: ActivePlayer (no Protected requirement). NAI-120 Bundle 2B.
func handleBusy2(s *ScriptState) error {
	if err := requireActivePlayer(s, "BUSY2"); err != nil {
		return err
	}
	if s.Self.HasInteraction() || s.Self.HasWaypoints() {
		s.PushInt(1)
	} else {
		s.PushInt(0)
	}
	return nil
}

// handlePOpNpcT (P_OPNPCT, opcode 2079) anchors the active player on the
// active NPC with the APNPCT/OPNPCT trigger family and stores spellCom as
// the targetSubject.com. Mirrors TS PlayerOps.ts:417-421
// (https://x.com/JagexAsh/status/1791472651623370843):
//
//	const spellId: number = check(state.popInt(), NumberNotNull);
//	state.activePlayer.stopAction();
//	state.activePlayer.setInteraction(Interaction.SCRIPT, state.activeNpc,
//	    ServerTriggerType.APNPCT, spellId);
//
// Gate: ProtectedActivePlayer + ActiveNpc (goscape defensive; TS skips this
// check — TS checkedHandler(ProtectedActivePlayer) does not gate ActiveNpc).
// NAI-120 Bundle 2B.
func handlePOpNpcT(s *ScriptState) error {
	if err := requireProtectedActivePlayer(s, "P_OPNPCT"); err != nil {
		return err
	}
	if s.ActiveNpc == nil {
		return errors.New("P_OPNPCT: no active npc")
	}
	spellCom := s.PopInt()
	if err := checkNotNull(spellCom, "P_OPNPCT"); err != nil {
		return err
	}
	s.Self.StopAction()
	s.Self.SetInteractionScriptNpcT(s.ActiveNpc, spellCom)
	return nil
}

// handlePOpPlayer (P_OPPLAYER, opcode 2081) anchors the active player on the
// secondary active player (Self2) with the APPLAYER<op>/OPPLAYER<op> trigger
// family. Mirrors TS PlayerOps.ts:1009-1020
// (https://x.com/JagexAsh/status/1791472651623370843):
//
//	const type = check(state.popInt(), NumberNotNull) - 1;
//	if (type < 0 || type >= 5) {
//	    throw new Error(`Invalid opplayer: ${type + 1}`);
//	}
//	const target = state._activePlayer2;
//	if (!target) { return; }
//	state.activePlayer.stopAction();
//	state.activePlayer.setInteraction(Interaction.SCRIPT, target,
//	    ServerTriggerType.APPLAYER1 + type);
//
// Gate: ProtectedActivePlayer. The popped op is 1-indexed (1..5); after
// subtracting 1 it must be in [0,4]. Self2-nil is a silent return (TS-faithful).
// NAI-120 Bundle 2B.
func handlePOpPlayer(s *ScriptState) error {
	if err := requireProtectedActivePlayer(s, "P_OPPLAYER"); err != nil {
		return err
	}
	op := s.PopInt()
	if err := checkNotNull(op, "P_OPPLAYER"); err != nil {
		return err
	}
	idx := op - 1
	if idx < 0 || idx >= 5 {
		return fmt.Errorf("P_OPPLAYER: invalid op %d", op)
	}
	if s.Self2 == nil {
		return nil // TS-faithful silent return
	}
	s.Self.StopAction()
	s.Self.SetInteractionScriptPlayer(s.Self2, op)
	return nil
}

// handleFindHero (FINDHERO, opcode 2018) returns the player with the
// largest HeroPoints credit on the active player's ledger, binding
// them to the SECONDARY active-player slot regardless of IntOperand.
// Pushes 1 on success, 0 if the ledger is empty, the resolved player
// has logged out, or s.World is nil. Mirrors TS PlayerOps.ts:1138-1154.
//
// DEVIATION-NAI-127-D1: defensive nil-s.World guard (goscape defensive;
// TS skips this check). Retire per the same condition as NPC_FINDHERO.
func handleFindHero(s *ScriptState) error {
	if err := requireActivePlayer(s, "FINDHERO"); err != nil {
		return err
	}
	if s.World == nil {
		s.PushInt(0)
		return nil
	}
	uid := s.Self.TopContributor()
	if uid == 0 {
		s.PushInt(0)
		return nil
	}
	player := s.World.LookupPlayerByUID(uid)
	if player == nil {
		s.PushInt(0)
		return nil
	}
	s.Self2 = player
	s.Pointers |= PtrActivePlayer2
	s.PushInt(1)
	return nil
}

// handleBothHeroPoints (BOTH_HEROPOINTS, opcode 2003) credits `damage`
// to the receiving player's HeroPoints ledger, attributed to the
// sending player's UID. IntOperand selects the swap direction:
//
//	IntOperand=0 → from=Self (primary),    to=Self2 (secondary)
//	IntOperand=1 → from=Self2 (secondary), to=Self (primary)
//
// Mirrors TS PlayerOps.ts:1156-1167. Returns an error if either slot
// is nil (TS throws).
func handleBothHeroPoints(s *ScriptState) error {
	if err := requireActivePlayer(s, "BOTH_HEROPOINTS"); err != nil {
		return err
	}
	damage := s.PopInt()
	secondary := s.Script.IntOperands[s.PC] == 1
	var from, to ActivePlayer
	if secondary {
		from, to = s.Self2, s.Self
	} else {
		from, to = s.Self, s.Self2
	}
	if from == nil || to == nil {
		return fmt.Errorf("BOTH_HEROPOINTS: player is null")
	}
	to.AddHeroPoints(from.UID(), damage)
	return nil
}


// handleDamage (DAMAGE, opcode 2015) applies damage to the player
// resolved from a UID popped from the stack. Pop order (TS): amount,
// hitType, uid (LIFO via popInt). Silent no-op if the UID does not
// resolve to a logged-in player. Mirrors TS PlayerOps.ts:768-779.
//
// DEVIATION-NAI-127-D1: defensive nil-s.World guard. Without s.World
// there is no way to resolve the UID.
//
// Note: no PtrActivePlayer gate — TS uses raw `state =>`, not
// checkedHandler. This is intentional and not a deviation; the
// handler is UID-driven and never reads s.Self. Pinned by
// TestDamage_NoPointerGate.
func handleDamage(s *ScriptState) error {
	amount := s.PopInt()
	hitType := s.PopInt()
	uid := s.PopInt()
	if s.World == nil {
		return nil
	}
	player := s.World.LookupPlayerByUID(uid)
	if player == nil {
		return nil
	}
	player.ApplyDamage(amount, hitType)
	return nil
}

// handleGender (GENDER, opcode 2020) pushes the active player's
// gender (0=male, 1=female). Mirrors TS PlayerOps.ts:968-970.
//
// DEVIATION-NAI-127-D2: TS uses raw `state =>` — there is no pointer
// gate (no requireActivePlayer). state.activePlayer access is
// nil-unsafe. Goscape preserves this quirk per ts_asymmetry_dual_pin.
// Pinned by TestGender_Male/Female (no PtrActivePlayer in fixture).
// Retire only if upstream TS adds a checkedHandler wrapping.
func handleGender(s *ScriptState) error {
	s.PushInt(s.Self.Gender())
	return nil
}

// handlePlayerMember (PLAYERMEMBER, opcode 2090) pushes 1 if the active
// player has a members account, else 0. Mirrors TS
// LostCityRS/Engine-TS/.../PlayerOps.ts:1211-1213 — checkedHandler(ActivePlayer).
func handlePlayerMember(s *ScriptState) error {
	if err := requireActivePlayer(s, "PLAYERMEMBER"); err != nil {
		return err
	}
	if s.Self.Members() {
		s.PushInt(1)
	} else {
		s.PushInt(0)
	}
	return nil
}

// handlePPreventLogout (P_PREVENTLOGOUT, opcode 2084) sets the
// player's anti-log message and absolute tick deadline. Pop order
// (TS): popString first (message), then popInt (additional ticks
// from current tick). Goscape's int and string stacks are
// independent so the in-handler order between PopInt and PopString
// does not affect runtime. Mirrors TS PlayerOps.ts:626-630.
//
// DEVIATION-NAI-127-D1: defensive nil-s.World guard (currentTick read
// requires World).
func handlePPreventLogout(s *ScriptState) error {
	if err := requireProtectedActivePlayer(s, "P_PREVENTLOGOUT"); err != nil {
		return err
	}
	if s.World == nil {
		return nil
	}
	ticks := s.PopInt()
	msg := s.PopString()
	s.Self.SetPreventLogout(msg, s.World.CurrentTick()+ticks)
	return nil
}

// handleAfkEvent (AFK_EVENT, opcode 2000) pushes 1 when the player is
// eligible to receive an AFK-event prompt and clears the eligibility
// flag. Mirrors TS LostCityRS/Engine-TS/.../PlayerOps.ts:1057-1062:
//
//	state.pushInt(
//	  (Environment.NODE_DEBUG || state.activePlayer.staffModLevel < 2)
//	    && state.activePlayer.afkEventReady ? 1 : 0
//	);
//	state.activePlayer.afkEventReady = false;
//
// The active-player guard is goscape defensive (TS skips this check;
// see defensive_gate_doc_comment_label).
func handleAfkEvent(s *ScriptState) error {
	if err := requireActivePlayer(s, "AFK_EVENT"); err != nil {
		return err
	}
	eligible := (s.NodeDebug || s.Self.StaffModLevel() < 2) && s.Self.AfkEventReady()
	if eligible {
		s.PushInt(1)
	} else {
		s.PushInt(0)
	}
	s.Self.SetAfkEventReady(false)
	return nil
}

// handleWeight (WEIGHT) pushes the player's tracked carry weight.
// Mirrors TS LostCityRS/Engine-TS/.../PlayerOps.ts:1180-1182 —
// checkedHandler(ProtectedActivePlayer).
func handleWeight(s *ScriptState) error {
	if err := requireProtectedActivePlayer(s, "WEIGHT"); err != nil {
		return err
	}
	s.PushInt(s.Self.RunWeight())
	return nil
}

// handleHealEnergy (HEAL_ENERGY) adds the popped amount to the player's
// run-energy and clamps the result to [0, 10000]. Mirrors TS
// LostCityRS/Engine-TS/.../PlayerOps.ts:1050-1054:
//
//	const amount = check(state.popInt(), NumberNotNull) // 100=1%, 10000=100%
//	player.runenergy = Math.min(Math.max(player.runenergy + amount, 0), 10000)
//
// The active-player guard is goscape defensive (TS skips this check;
// see defensive_gate_doc_comment_label).
func handleHealEnergy(s *ScriptState) error {
	if err := requireActivePlayer(s, "HEAL_ENERGY"); err != nil {
		return err
	}
	amount := s.PopInt()
	if err := checkNotNull(amount, "HEAL_ENERGY"); err != nil {
		return err
	}
	next := s.Self.RunEnergy() + amount
	if next < 0 {
		next = 0
	} else if next > 10000 {
		next = 10000
	}
	s.Self.SetRunEnergy(next)
	return nil
}

// handleSetSkinColour (SETSKINCOLOUR) writes the player's skin-colour
// slot (colors[4]) after a [0,7] range check. Mirrors TS
// LostCityRS/Engine-TS/.../PlayerOps.ts:1121-1124:
//
//	const skin = check(state.popInt(), SkinColourValid)
//	state.activePlayer.colors[4] = skin
//
// The active-player guard is goscape defensive (TS skips this check;
// see defensive_gate_doc_comment_label).
func handleSetSkinColour(s *ScriptState) error {
	if err := requireActivePlayer(s, "SETSKINCOLOUR"); err != nil {
		return err
	}
	skin := s.PopInt()
	if skin < 0 || skin > 7 {
		return fmt.Errorf("SETSKINCOLOUR: invalid skin colour %d (range 0..7)", skin)
	}
	s.Self.SetColorPart(4, skin)
	return nil
}

// handleSay implements OpSay (TS SAY at PlayerOps.ts:462-464).
// Mirrors `state.activePlayer.say(state.popString())` —
// checkedHandler(ActivePlayer, ...). NAI-160 T1.
func handleSay(s *ScriptState) error {
	if err := requireActivePlayer(s, "SAY"); err != nil {
		return err
	}
	text := s.PopString()
	s.Self.Say([]byte(text))
	return nil
}

// handleHeadIconsGet implements OpHeadIconsGet (TS HEADICONS_GET at
// PlayerOps.ts:980-982). Pushes the player's headicons bitmask.
//
// goscape defensive: requireActivePlayer guard fronts the dereference;
// TS deref-panics on a nil activePlayer (no checkedHandler wrap).
// NAI-160 T2.
func handleHeadIconsGet(s *ScriptState) error {
	if err := requireActivePlayer(s, "HEADICONS_GET"); err != nil {
		return err
	}
	s.PushInt(s.Self.HeadIcons())
	return nil
}

// handleHeadIconsSet implements OpHeadIconsSet (TS HEADICONS_SET at
// PlayerOps.ts:984-986). Pops an int, checks NumberNotNull, writes into
// the player's headicons bitmask.
//
// goscape defensive: requireActivePlayer guard (TS deref-panics).
// Order is pop → check → set so a failed gate leaves headicons untouched
// (covered by TestHeadIconsSetRejectsNull). NAI-160 T3.
func handleHeadIconsSet(s *ScriptState) error {
	if err := requireActivePlayer(s, "HEADICONS_SET"); err != nil {
		return err
	}
	v := s.PopInt()
	if err := checkNotNull(v, "HEADICONS_SET"); err != nil {
		return err
	}
	s.Self.SetHeadIcons(v)
	return nil
}

// handleClearQueue implements OpClearQueue (TS CLEARQUEUE at
// PlayerOps.ts:1045-1048). Pops a scriptID, delegates to
// ActivePlayer.UnlinkQueuedScript — which (per NAI-161 T1) walks the
// player's p.queue and drops every entry whose Script resolves to that
// scriptID. NAI-161 T4.
func handleClearQueue(s *ScriptState) error {
	if err := requireActivePlayer(s, "CLEARQUEUE"); err != nil {
		return err
	}
	s.Self.UnlinkQueuedScript(s.PopInt())
	return nil
}

// handleGetQueue implements OpGetQueue (TS GETQUEUE at
// PlayerOps.ts:903-912). Pops a scriptID, pushes
// ActivePlayer.QueueCount(scriptID) — the count of non-Weak queue
// entries whose Script matches. The for-loop in the TS body lives
// inside QueueCount per NAI-161 T2. NAI-161 T5.
func handleGetQueue(s *ScriptState) error {
	if err := requireActivePlayer(s, "GETQUEUE"); err != nil {
		return err
	}
	s.PushInt(s.Self.QueueCount(s.PopInt()))
	return nil
}
