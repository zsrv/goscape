package script

import (
	"fmt"

	"github.com/zsrv/goscape/pkg/coordgrid"
	"github.com/zsrv/goscape/pkg/pathfinder/loc"
)

// requireActiveLoc returns an error tagged with the opcode name if the
// script has no ActiveLoc bound. All LOC_* read handlers start with
// this check to mirror TS `checkedHandler(ActiveLoc, ...)`.
func requireActiveLoc(s *ScriptState, op string) error {
	if s.ActiveLoc == nil {
		return fmt.Errorf("%s: no active loc", op)
	}
	return nil
}

// setActiveLocSlot writes the loc to either ActiveLoc (primary) or
// OtherActiveLoc (secondary) based on the handler's IntOperand and sets
// the corresponding Pointer flag. Mirrors TS
// state.pointerAdd(ActiveLoc[state.intOperand]) at LocOps.ts:110, and
// the parallel setActiveNpcSlot at handlers_npc.go:64-83.
//
// IntOperand==0 → ActiveLoc/PtrActiveLoc (.loc syntax).
// IntOperand==1 → OtherActiveLoc/PtrActiveLoc2 (.loc2 syntax).
// Any other value panics (compiler invariant — bytecode only emits 0/1).
func setActiveLocSlot(s *ScriptState, loc ActiveLoc) {
	operand := s.Script.IntOperands[s.PC]
	switch operand {
	case 0:
		s.ActiveLoc = loc
		s.Pointers |= PtrActiveLoc
	case 1:
		s.OtherActiveLoc = loc
		s.Pointers |= PtrActiveLoc2
	default:
		panic(fmt.Sprintf("setActiveLocSlot: invalid IntOperand %d", operand))
	}
}

// handleLocFindAllZone (LOC_FINDALLZONE, opcode 3008) pops a coord,
// validates, and stores a single-zone LocIterator targeting the zone
// containing that coord. Mirrors TS LocOps.ts:96-100. No
// distance/category/type filtering (TS LocIterator is single-mode).
//
// Nil-LocOps degrades silently (matches NPC_FINDALLZONE convention at
// handlers_npc.go:714-716).
func handleLocFindAllZone(s *ScriptState) error {
	coord := s.PopInt()
	level, x, z, err := checkCoord(coord, "LOC_FINDALLZONE")
	if err != nil {
		return err
	}
	if s.LocOps == nil {
		return nil
	}
	s.locIterator = NewZoneLocIterator(s.LocOps, s.World.CurrentTick(), level, x, z)
	return nil
}

// handleLocFindNext (LOC_FINDNEXT, opcode 3009) advances the active
// LocIterator and either sets active_loc + pushes 1 on hit, or pushes 0
// on miss / nil-iterator. Mirrors TS LocOps.ts:102-112.
//
// Stale-iterator semantics: mirror NPC_FINDNEXT (handlers_npc.go:778-795)
// — return error on stale; existing runtime path catches and clears the
// active script (parallel to npc_script.go:167-172).
//
// Pointer-set: setActiveLocSlot threads IntOperand 0/1 to choose
// primary/secondary slot per TS state.pointerAdd(ActiveLoc[intOperand]).
//
// Exhaustion does NOT clear s.locIterator (matches NPC family —
// handlers_npc.go:769-771). Subsequent FINDNEXT calls continue to
// return push-0.
func handleLocFindNext(s *ScriptState) error {
	it := s.locIterator
	if it == nil {
		s.PushInt(0)
		return nil
	}
	if it.Stale(s.World.CurrentTick()) {
		return fmt.Errorf("LOC_FINDNEXT: tried to use an old iterator. Create a new iterator instead.")
	}
	loc, ok := it.Next()
	if !ok {
		s.PushInt(0)
		return nil
	}
	setActiveLocSlot(s, loc)
	s.PushInt(1)
	return nil
}

// handleLocFind (LOC_FIND, opcode 3007) pops [coord, locId], looks up
// the matching loc at that tile, and either binds it to the ActiveLoc
// slot selected by IntOperand (0 → primary, 1 → secondary) and pushes
// 1, or pushes 0 on miss. Mirrors TS LocOps.ts:79-94:
//
//	const [coord, locId] = state.popInts(2);
//	const locType = check(locId, LocTypeValid);
//	const position = check(coord, CoordValid);
//	const loc = World.getLoc(position.x, position.z, position.level, locType.id);
//	if (!loc) { state.pushInt(0); return; }
//	state.activeLoc = loc;
//	state.pointerAdd(ActiveLoc[state.intOperand]);
//	state.pushInt(1);
//
// Pointer-set threads IntOperand 0/1 through setActiveLocSlot
// (handlers_loc.go:29) — same pattern as LOC_FINDNEXT.
//
// Miss-semantics: ActiveLoc / OtherActiveLoc are NOT mutated on miss
// (TS only writes activeLoc inside the hit arm). Pinned by test.
//
// Configs/LocOps nil are surfaced as errors (LOC_ADD / LOC_CHANGE
// precedent; goscape defensive — TS reaches via static accessor) rather
// than silent push-0, because the TS `check` operator throws on
// LocTypeValid lookup failure.
func handleLocFind(s *ScriptState) error {
	if err := requireConfigs(s, "LOC_FIND"); err != nil {
		return err
	}
	locId := s.PopInt()
	coord := s.PopInt()
	if s.Configs.LocType(locId) == nil {
		return fmt.Errorf("LOC_FIND: unknown loc id %d", locId)
	}
	level, x, z, err := checkCoord(coord, "LOC_FIND")
	if err != nil {
		return err
	}
	if s.LocOps == nil {
		return fmt.Errorf("LOC_FIND: LocOps unavailable")
	}
	loc := s.LocOps.GetLoc(level, x, z, locId)
	if loc == nil {
		s.PushInt(0)
		return nil
	}
	setActiveLocSlot(s, loc)
	s.PushInt(1)
	return nil
}

// handleLocOp pops a 1-indexed op slot and pushes the ActiveLoc's
// LocType.Op[op-1] string. Pushes "" if:
//   - Configs is nil (test-only defensive guard)
//   - LocType is not loaded for the ActiveLoc's type ID
//   - op is out of [1, len(Op)] range
//
// Mirrors handleNpcHasOp (handlers_npc.go:87) in structure — same read
// path through Configs → LocType.Op — but returns the string rather
// than a bool (NPC_HASOP's boolean form).
func handleLocOp(s *ScriptState) error {
	if err := requireActiveLoc(s, "LOC_OP"); err != nil {
		return err
	}
	op := s.PopInt()
	if s.Configs == nil {
		s.PushString("")
		return nil
	}
	cfg := s.Configs.LocType(s.ActiveLoc.LocType())
	if cfg == nil {
		s.PushString("")
		return nil
	}
	idx := op - 1
	if idx < 0 || idx >= len(cfg.Op) {
		s.PushString("")
		return nil
	}
	s.PushString(cfg.Op[idx])
	return nil
}

// handleLocCoord pushes the ActiveLoc's packed (level, x, z) coord onto
// the int stack. TS:
//
//	pushInt(CoordGrid.packCoord(activeLoc.level, activeLoc.x, activeLoc.z));
//
// Requires an ActiveLoc; returns "LOC_COORD: no active loc" otherwise.
func handleLocCoord(s *ScriptState) error {
	if err := requireActiveLoc(s, "LOC_COORD"); err != nil {
		return err
	}
	x, z, level := s.ActiveLoc.Coords()
	s.PushInt(coordgrid.PackCoord(level, x, z))
	return nil
}

// handleLocAngle pushes the ActiveLoc's rotation onto the int stack,
// validated through the [0,3] LocAngle range. TS:
//
//	pushInt(check(activeLoc.angle, LocAngleValid));
//
// Requires an ActiveLoc; returns "LOC_ANGLE: no active loc" otherwise.
func handleLocAngle(s *ScriptState) error {
	if err := requireActiveLoc(s, "LOC_ANGLE"); err != nil {
		return err
	}
	angle := s.ActiveLoc.Angle()
	if err := checkLocAngle(angle); err != nil {
		return fmt.Errorf("LOC_ANGLE: %w", err)
	}
	s.PushInt(angle)
	return nil
}

// handleLocCategory pushes the ActiveLoc's resolved LocType.Category.
// Mirrors TS LOC_CATEGORY (LocOps.ts:56-58):
//
//	pushInt(check(activeLoc.type, LocTypeValid).category);
//
// Goscape default LocType.Category is -1 (loctype.go:194) — TS-faithful
// for unset categories. Surfaced by NAI-119 smoke (tut_open_mining_gate
// branches on `loc_category = tut_mining_exit`); spec §1's claim that
// downstream LOC_* handlers "all already exist" missed this one.
func handleLocCategory(s *ScriptState) error {
	if err := requireConfigs(s, "LOC_CATEGORY"); err != nil {
		return err
	}
	if err := requireActiveLoc(s, "LOC_CATEGORY"); err != nil {
		return err
	}
	id := s.ActiveLoc.LocType()
	lt := s.Configs.LocType(id)
	if lt == nil {
		return fmt.Errorf("LOC_CATEGORY: unknown loc id %d", id)
	}
	s.PushInt(lt.Category)
	return nil
}

// handleLocType pushes the ActiveLoc's resolved LocType ID. Mirrors TS
// LOC_TYPE: pushInt(check(activeLoc.type, LocTypeValid).id).
//
// Goscape ActiveLoc.LocType() returns the int ID directly; the TS
// LocTypeValid non-null check translates to a Configs lookup nil-check
// (matches handleNpcParam pattern at handlers_config.go:307-308).
func handleLocType(s *ScriptState) error {
	if err := requireConfigs(s, "LOC_TYPE"); err != nil {
		return err
	}
	if err := requireActiveLoc(s, "LOC_TYPE"); err != nil {
		return err
	}
	id := s.ActiveLoc.LocType()
	lt := s.Configs.LocType(id)
	if lt == nil {
		return fmt.Errorf("LOC_TYPE: unknown loc id %d", id)
	}
	s.PushInt(id)
	return nil
}

// handleLocName pushes the ActiveLoc's LocType name, with "null" fallback
// when the name is empty. Mirrors TS LOC_NAME: pushString(check(activeLoc.type,
// LocTypeValid).name ?? 'null').
//
// Note: TS active-loc LOC_NAME does NOT fall back to debugname; only the
// locID-arg LC_NAME (handlers_config.go:159-165) does. LC_NAME is already
// TS-correct (Name → DebugName → "null" per TS LocConfigOps.ts:12).
func handleLocName(s *ScriptState) error {
	if err := requireConfigs(s, "LOC_NAME"); err != nil {
		return err
	}
	if err := requireActiveLoc(s, "LOC_NAME"); err != nil {
		return err
	}
	id := s.ActiveLoc.LocType()
	lt := s.Configs.LocType(id)
	if lt == nil {
		return fmt.Errorf("LOC_NAME: unknown loc id %d", id)
	}
	if lt.Name != "" {
		s.PushString(lt.Name)
	} else {
		s.PushString("null")
	}
	return nil
}

// handleLocShape pushes the ActiveLoc's shape, validated through the
// [0,22] LocShape range. TS:
//
//	pushInt(check(activeLoc.shape, LocShapeValid));
//
// Requires an ActiveLoc; returns "LOC_SHAPE: no active loc" otherwise.
// Range-validates because Loc.Shape()'s mask is [0,31] — wider than
// the LocShape valid range.
func handleLocShape(s *ScriptState) error {
	if err := requireActiveLoc(s, "LOC_SHAPE"); err != nil {
		return err
	}
	shape := s.ActiveLoc.Shape()
	if err := checkLocShape(shape); err != nil {
		return fmt.Errorf("LOC_SHAPE: %w", err)
	}
	s.PushInt(shape)
	return nil
}

// checkLocType mirrors TS LocTypeValid (ScriptValidators.ts:105) — a
// ScriptInputConfigTypeValidator over LocType. Both the range check
// (0 <= id < LocType.count) and the registry-present check collapse
// into "s.Configs.LocType(id) != nil" per the Configs interface
// contract at configs.go:7. State-aware signature matches sibling
// checkInvType / checkSeqType / checkNpcType.
func checkLocType(s *ScriptState, id int, op string) error {
	if s.Configs == nil || s.Configs.LocType(id) == nil {
		return fmt.Errorf("%s: no LocType with value (%d) found", op, id)
	}
	return nil
}

// checkDuration mirrors TS DurationValid (ScriptValidators.ts:108) — a
// range validator rejecting [<1, >2147483647]. Reused by LOC_CHANGE,
// LOC_ADD, LOC_DEL.
func checkDuration(v int) error {
	if v < 1 || v > 2147483647 {
		return fmt.Errorf("duration out of range [1, 2147483647]: %d", v)
	}
	return nil
}

// handleLocChange pops [id, duration] from the int stack and asks
// LocOps to mutate the ActiveLoc's type to id, preserving shape/angle.
// Mirrors TS LOC_CHANGE (LocOps.ts:60-67):
//
//	const [id, duration] = state.popInts(2);
//	check(duration, DurationValid);
//	check(id, LocTypeValid);
//	World.changeLoc(state.activeLoc, id, state.activeLoc.shape, state.activeLoc.angle, duration);
func handleLocChange(s *ScriptState) error {
	if err := requireActiveLoc(s, "LOC_CHANGE"); err != nil {
		return err
	}
	if err := requireConfigs(s, "LOC_CHANGE"); err != nil {
		return err
	}
	duration := s.PopInt()
	id := s.PopInt()
	if err := checkDuration(duration); err != nil {
		return fmt.Errorf("LOC_CHANGE: %w", err)
	}
	if s.Configs.LocType(id) == nil {
		return fmt.Errorf("LOC_CHANGE: unknown loc id %d", id)
	}
	if s.LocOps == nil {
		return fmt.Errorf("LOC_CHANGE: LocOps unavailable")
	}
	return s.LocOps.ChangeLoc(s.ActiveLoc, id, s.ActiveLoc.Shape(), s.ActiveLoc.Angle(), duration)
}

// handleLocParam pops paramID, resolves the ActiveLoc's LocType, and
// delegates to paramLookup. Mirrors TS LOC_PARAM (LocOps.ts:114-123) —
// the active-loc-bound counterpart of LC_PARAM (handlers_config.go:163).
func handleLocParam(s *ScriptState) error {
	if err := requireConfigs(s, "LOC_PARAM"); err != nil {
		return err
	}
	if err := requireActiveLoc(s, "LOC_PARAM"); err != nil {
		return err
	}
	paramID := s.PopInt()
	id := s.ActiveLoc.LocType()
	lt := s.Configs.LocType(id)
	if lt == nil {
		return fmt.Errorf("LOC_PARAM: unknown loc id %d", id)
	}
	return paramLookup(s, lt.Params, paramID)
}

// handleLocDel pops [duration] and removes the ActiveLoc. Mirrors TS
// LOC_DEL (LocOps.ts:74-77).
func handleLocDel(s *ScriptState) error {
	if err := requireActiveLoc(s, "LOC_DEL"); err != nil {
		return err
	}
	duration := s.PopInt()
	if err := checkDuration(duration); err != nil {
		return fmt.Errorf("LOC_DEL: %w", err)
	}
	if s.LocOps == nil {
		return fmt.Errorf("LOC_DEL: LocOps unavailable")
	}
	return s.LocOps.RemoveLoc(s.ActiveLoc, duration)
}

// handleLocAnim pops [seq], validates against Configs.SeqType, and
// dispatches an animation event for the ActiveLoc. Mirrors TS LOC_ANIM
// (LocOps.ts:50-54).
func handleLocAnim(s *ScriptState) error {
	if err := requireActiveLoc(s, "LOC_ANIM"); err != nil {
		return err
	}
	if err := requireConfigs(s, "LOC_ANIM"); err != nil {
		return err
	}
	seq := s.PopInt()
	if s.Configs.SeqType(seq) == nil {
		return fmt.Errorf("LOC_ANIM: unknown seq id %d", seq)
	}
	if s.LocOps == nil {
		return fmt.Errorf("LOC_ANIM: LocOps unavailable")
	}
	return s.LocOps.AnimLoc(s.ActiveLoc, seq)
}

// handleLocAdd pops [coord, type, angle, shape, duration] and either
// (a) finds a same-layer loc at coord and changes it, or (b) creates a
// new DESPAWN-lifecycle loc. Mirrors TS LOC_ADD (LocOps.ts:18-43):
//
//	const [coord, type, angle, shape, duration] = state.popInts(5);
//	[validators]
//	for loc at zone-coord:
//	    if loc.layer === locShapeLayer(shape):
//	        World.changeLoc(loc, type, shape, angle, duration); return
//	const created = new Loc(level, x, z, locType.width, locType.length, DESPAWN, type, shape, angle);
//	World.addLoc(created, duration);
func handleLocAdd(s *ScriptState) error {
	if err := requireConfigs(s, "LOC_ADD"); err != nil {
		return err
	}
	duration := s.PopInt()
	shape := s.PopInt()
	angle := s.PopInt()
	typ := s.PopInt()
	coord := s.PopInt()

	if s.Configs.LocType(typ) == nil {
		return fmt.Errorf("LOC_ADD: unknown loc id %d", typ)
	}
	if err := checkLocAngle(angle); err != nil {
		return fmt.Errorf("LOC_ADD: %w", err)
	}
	if err := checkLocShape(shape); err != nil {
		return fmt.Errorf("LOC_ADD: %w", err)
	}
	if err := checkDuration(duration); err != nil {
		return fmt.Errorf("LOC_ADD: %w", err)
	}
	if s.LocOps == nil {
		return fmt.Errorf("LOC_ADD: LocOps unavailable")
	}

	pos := coordgrid.UnpackCoord(coord)
	wantLayer := int(loc.LayerOf(loc.Shape(shape)))
	for _, existing := range s.LocOps.LocsAtCoord(pos.Level, pos.X, pos.Z) {
		if existing.Layer() == wantLayer {
			if err := s.LocOps.ChangeLoc(existing, typ, shape, angle, duration); err != nil {
				return err
			}
			s.ActiveLoc = existing
			return nil
		}
	}
	created, err := s.LocOps.AddLoc(pos.Level, pos.X, pos.Z, typ, shape, angle, duration)
	if err != nil {
		return err
	}
	s.ActiveLoc = created
	return nil
}
