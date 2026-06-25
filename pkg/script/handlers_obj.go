// Package script — handlers for the Obj family of script opcodes.
package script

import (
	"fmt"

	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/zsrv/goscape/pkg/coordgrid"
	"github.com/zsrv/goscape/pkg/eventspb"
	"github.com/zsrv/goscape/pkg/inventory"
	"github.com/zsrv/goscape/pkg/telemetry"
	applog "github.com/zsrv/goscape/pkg/util/log"
)

// requireActiveObj returns an error if the operand-resolved active obj slot
// (`.obj`/`.obj2`) is nil, mirroring TS checkedHandler(ActiveObj[intOperand]).
func requireActiveObj(s *ScriptState, op string) error {
	if s.activeObj() == nil {
		return fmt.Errorf("%s: no active obj", op)
	}
	return nil
}

// setActiveObjSlot writes the obj to either ActiveObj (primary) or
// OtherActiveObj (secondary) based on the handler's IntOperand and sets
// the corresponding Pointer flag. Mirrors TS
// state.pointerAdd(ActiveObj[state.intOperand]) at ObjOps.ts:91, 181,
// 199, and the parallel setActiveLocSlot at handlers_loc.go:29-40.
//
// IntOperand==0 → ActiveObj/PtrActiveObj (.obj syntax).
// IntOperand==1 → OtherActiveObj/PtrActiveObj2 (.obj2 syntax).
// Any other value panics (compiler invariant — bytecode only emits 0/1).
func setActiveObjSlot(s *ScriptState, obj ActiveObj) {
	operand := s.Script.IntOperands[s.PC]
	switch operand {
	case 0:
		s.ActiveObj = obj
		s.Pointers |= PtrActiveObj
	case 1:
		s.OtherActiveObj = obj
		s.Pointers |= PtrActiveObj2
	default:
		panic(fmt.Sprintf("setActiveObjSlot: invalid IntOperand %d", operand))
	}
}

// checkObjType validates an ObjType id is registered in s.Configs.
// Mirrors TS check(id, ObjTypeValid) (ScriptValidators.ts).
func checkObjType(s *ScriptState, id int, op string) error {
	if s.Configs == nil || s.Configs.ObjType(id) == nil {
		return fmt.Errorf("%s: no ObjType with value (%d) found", op, id)
	}
	return nil
}

// checkObjStack mirrors TS ObjStackValid (ScriptValidators.ts:121) — a
// ScriptInputRangeValidator over [1, Inventory.STACK_LIMIT=0x7fffffff].
// Rejects 0, negatives, and counts above StackLimit.
func checkObjStack(c int, op string) error {
	if c < 1 || c > inventory.StackLimit {
		return fmt.Errorf("%s: invalid count (%d)", op, c)
	}
	return nil
}

// objAddCommon is the shared body of OBJ_ADD and OBJ_ADDALL. Differs
// only in receiverID: OBJ_ADD passes the active player's UID for a
// caller-only private drop; OBJ_ADDALL passes zone.PublicReceiver (-1)
// for broadcast.
//
// Mirrors TS ObjOps.ts:20-92 (both opcodes share the validation chain
// + stackable branch). Pop order matches popInts(4): top-of-stack is
// duration, then count, then objId, then coord at the bottom.
//
// Mirrors TS ObjOps.ts:20-92 (both opcodes share the validation chain
// + stackable branch + state.activeObj writeback + pointerAdd(ActiveObj)).
// (NAI-115-D3 retired in-bundle stretch: AddObj now returns ActiveObj.)
func objAddCommon(s *ScriptState, op string, receiverID int) error {
	duration := s.PopInt()
	count := s.PopInt()
	objId := s.PopInt()
	coord := s.PopInt()

	if objId == -1 || count == -1 {
		return nil
	}
	if err := checkObjType(s, objId, op); err != nil {
		return err
	}
	if err := checkDuration(duration); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	level, x, z, err := checkCoord(coord, op)
	if err != nil {
		return err
	}
	if err := checkObjStack(count, op); err != nil {
		return err
	}

	objType := s.Configs.ObjType(objId)
	if objType.DummyItem != 0 {
		return fmt.Errorf("%s: attempted to add dummy item: id=%d", op, objId)
	}
	if objType.Members && s.World.MapMembers() == 0 {
		return nil
	}

	// L20: operand-aware activeObj writeback via setActiveObjSlot, mirroring
	// TS `state.activeObj = obj; state.pointerAdd(ActiveObj[state.intOperand])`
	// (ObjOps.ts:50-53/82-85) — IntOperand 1 (.obj2) routes to OtherActiveObj.
	// Previously wrote s.ActiveObj/PtrActiveObj unconditionally, diverging from
	// the operand-aware OBJ_FIND/FINDNEXT path that already uses this helper.
	// For non-stackable count=N this overwrites N times — last wins, matching
	// the TS loop.
	if !objType.Stackable || count == 1 {
		for range count {
			obj := s.World.AddObj(level, x, z, objId, 1, duration, receiverID, 0)
			if obj != nil {
				setActiveObjSlot(s, obj)
			}
		}
	} else {
		obj := s.World.AddObj(level, x, z, objId, count, duration, receiverID, 0)
		if obj != nil {
			setActiveObjSlot(s, obj)
		}
	}
	return nil
}

// handleObjAdd (OBJ_ADD, opcode 3500) drops a private (caller-only) obj
// at the unpacked coord. Mirrors TS ObjOps.ts:20-55.
func handleObjAdd(s *ScriptState) error {
	if s.Self == nil {
		return fmt.Errorf("OBJ_ADD: no active player")
	}
	if s.World == nil {
		return fmt.Errorf("OBJ_ADD: no world surface")
	}
	return objAddCommon(s, "OBJ_ADD", s.activePlayer().UID())
}

// handleObjDel (OBJ_DEL, opcode 3504) removes the active obj. Mirrors
// TS ObjOps.ts:112-119.
//
// TS branches on `pointerGet(ActivePlayer)` but both arms call identical
// World.removeObj(activeObj, duration) — collapsed here to a single
// unconditional call (TS-side oddity).
//
// duration is ObjType.RespawnRate; Server.RemoveObj gates on
// lifecycle+duration to decide between respawn-scheduling and untrack.
func handleObjDel(s *ScriptState) error {
	if err := requireActiveObj(s, "OBJ_DEL"); err != nil {
		return err
	}
	if s.World == nil {
		return fmt.Errorf("OBJ_DEL: no world surface")
	}
	duration := 0
	if s.Configs != nil {
		if objCfg := s.Configs.ObjType(s.activeObj().ObjType()); objCfg != nil {
			duration = objCfg.RespawnRate
		}
	}
	s.World.RemoveObj(s.activeObj(), duration)
	return nil
}

// objAddAllReceiverID is the receiverID sentinel passed to
// WorldVars.AddObj for broadcast (visible-to-all) drops. The world
// adapter resolves this to zone.PublicReceiver. Kept package-local so
// pkg/script does not depend on pkg/zone directly.
const objAddAllReceiverID = -1

// handleObjAddAll (OBJ_ADDALL, opcode 3501) drops a broadcast
// (visible-to-all) obj at the unpacked coord. Twin of handleObjAdd —
// identical validation chain via objAddCommon; only the receiverID
// differs. Mirrors TS ObjOps.ts:58-93.
func handleObjAddAll(s *ScriptState) error {
	// L22: nil-World guard matching the twin handleObjAdd (objAddCommon
	// dereferences s.World for MapMembers + AddObj). OBJ_ADDALL needs no
	// Self guard — it broadcasts via objAddAllReceiverID, not activePlayer.
	if s.World == nil {
		return fmt.Errorf("OBJ_ADDALL: no world surface")
	}
	return objAddCommon(s, "OBJ_ADDALL", objAddAllReceiverID)
}

// handleObjCoord (OBJ_COORD, opcode 3502) packs the active obj's tile
// position into a single RS2 coord int and pushes it. Mirrors TS
// ObjOps.ts:163-166.
func handleObjCoord(s *ScriptState) error {
	if err := requireActiveObj(s, "OBJ_COORD"); err != nil {
		return err
	}
	x, z, level := s.activeObj().Coords()
	s.PushInt(coordgrid.PackCoord(level, x, z))
	return nil
}

// handleObjCount (OBJ_COUNT, opcode 3503) pushes the active obj's
// count if it's valid for the active player; else pushes 0. Mirrors
// TS ObjOps.ts:121-130:
//
//	const obj: Obj = state.activeObj;
//	if (obj.isValid(state.activePlayer.hash64)) {
//	    state.pushInt(obj.count);
//	    return;
//	}
//	state.pushInt(0);
//
// goscape uses Self.UID() (composeUID-shaped int) instead of TS bigint
// hash64. See NAI-153-D2 in the spec.
func handleObjCount(s *ScriptState) error {
	if err := requireActiveObj(s, "OBJ_COUNT"); err != nil {
		return err
	}
	if err := requireActivePlayer(s, "OBJ_COUNT"); err != nil {
		return err
	}
	if s.activeObj().IsValidFor(s.activePlayer().UID()) {
		s.PushInt(s.activeObj().ObjCount())
		return nil
	}
	s.PushInt(0)
	return nil
}

// handleObjTakeItem (OBJ_TAKEITEM, opcode 3510) pops invType, validates,
// guards on isValid, adds the obj to the player's inv via performInvAdd,
// emits a PICKUP wealth event, and removes the obj from the world.
// Mirrors TS ObjOps.ts:137-161.
// NAI-115-D1 retired at NAI-162 B2; wealth event now emitted inline
// between invAdd and removeObj per TS ObjOps.ts:149-154.
//
// NAI-153-D3: TS OBJ_TAKEITEM (ObjOps.ts:147) calls Player.invAdd
// directly — the bare entity method (Player.ts:1496-1504), bypassing
// the InvOps INV_ADD opcode gates (InvTypeValid + ObjTypeValid +
// ObjStackValid + protect/scope + dummyitem). goscape routes through
// performInvAdd, which DOES apply the gates; for realistic call
// shapes (mindrune-style: non-protected inv 93, non-dummyitem obj)
// the gates are no-ops.
//
// h-obj-2: TS Player.invAdd's `assureFullInsertion` arg defaults to
// `true` (Player.ts:1496), so OBJ_TAKEITEM's bare call inherits an
// all-or-nothing semantic — Inventory.add either fully inserts or
// rolls back. INV_ADD (InvOps.ts:73) passes `false` explicitly.
// goscape now threads the bit through performInvAdd: OBJ_TAKEITEM
// passes `true` here, INV_ADD passes `false`. Prior to this fix
// the helper hard-coded `false` for both call sites, producing a
// partial-fill on tight destinations where TS would have rolled back.
func handleObjTakeItem(s *ScriptState) error {
	if err := requireActiveObj(s, "OBJ_TAKEITEM"); err != nil {
		return err
	}
	if err := requireActivePlayer(s, "OBJ_TAKEITEM"); err != nil {
		return err
	}
	if s.World == nil {
		return fmt.Errorf("OBJ_TAKEITEM: no world surface")
	}

	invID := s.PopInt()
	// TS validates invType first (ObjOps.ts:138, hard-error) THEN checks
	// obj.isValid (ObjOps.ts:143, soft no-op). Pre-check here preserves
	// that order so a bad invType paired with an invalid obj hard-errors
	// like TS, instead of silently no-op'ing through performInvAdd's
	// own checkInvType (which fires after IsValidFor's early return).
	if err := checkInvType(s, invID, "OBJ_TAKEITEM"); err != nil {
		return err
	}

	if !s.activeObj().IsValidFor(s.activePlayer().UID()) {
		return nil // TS returns false; goscape no-op (matches OBJ_DEL idiom)
	}

	// TS Player.ts:1496 — assureFullInsertion defaults to true; OBJ_TAKEITEM's
	// bare invAdd call (ObjOps.ts:147) inherits it.
	if err := performInvAdd(s, invID, s.activeObj().ObjType(), s.activeObj().ObjCount(), true, "OBJ_TAKEITEM"); err != nil {
		return err
	}

	// TS-faithful per ObjOps.ts:149-154: emit PICKUP wealth event between
	// invAdd and removeObj. NAI-115-D1 retired at NAI-162 B2.
	objTypeID := s.activeObj().ObjType()
	objCount := s.activeObj().ObjCount()
	if objCfg := s.Configs.ObjType(objTypeID); objCfg != nil {
		s.activePlayer().AddWealthEvent(WealthEvent{
			EventType:    WealthEventTypePickup,
			AccountItems: []WealthItem{{ID: objTypeID, Name: objCfg.DebugName, Count: objCount}},
			AccountValue: objCount * objCfg.Cost,
		})
		if s.NodeDebug && s.Log != nil {
			applog.Trace(s.Log, "nai162.wealth.objtake",
				"event_type", WealthEventTypePickup,
				"value", objCount*objCfg.Cost,
				"obj", objTypeID,
			)
		}
	}

	// Emit a wealth event for optional downstream consumers. Sibling to the
	// in-process AddWealthEvent above (NAI-162's per-player wealth tracker);
	// the two emit independently because they serve different consumers. DroppedByAccountId from the new Obj.DropperAccountID
	// field is the persistent account_id of the human dropper, or 0
	// for NPC/world-spawned items.
	//
	// s.World != nil is guaranteed by the early return at the top of
	// this handler; no extra guard needed here.
	worldID := int32(s.World.NodeID())
	x, z, level := s.activeObj().Coords()
	telemetry.Get().EmitWealth(&eventspb.WealthEnvelope{
		SchemaVersion: 1,
		EventId:       uuid.NewString(),
		Ts:            timestamppb.Now(),
		AccountId:     s.activePlayer().AccountID(),
		WorldId:       worldID,
		Payload: &eventspb.WealthEnvelope_ItemPickedUp{
			ItemPickedUp: &eventspb.ItemPickedUpEvent{
				ItemId:             int32(objTypeID),
				Qty:                int32(objCount),
				X:                  int32(x),
				Y:                  int32(z),
				Plane:              int32(level),
				DroppedByAccountId: s.activeObj().DropperAccountID(),
			},
		},
	})

	duration := 0
	if s.activeObj().IsRespawnLifecycle() {
		if objCfg := s.Configs.ObjType(s.activeObj().ObjType()); objCfg != nil {
			duration = objCfg.RespawnRate
		}
	}
	s.World.RemoveObj(s.activeObj(), duration)
	return nil
}

// handleObjFind (OBJ_FIND, opcode 3505) pops [coord, objId], resolves
// the obj via WorldVars.GetObj, and either slot-routes it via
// setActiveObjSlot + pushes 1 on hit, or pushes 0 on miss. Mirrors TS
// ObjOps.ts:168-183.
//
// Pop order: objId is at the top of the stack (last pushed); coord
// below it. Matches TS `[coord, objId] = state.popInts(2)`.
//
// Receiver UID is s.activePlayer().UID() per NAI-153-D2 (goscape UID vs TS hash64).
func handleObjFind(s *ScriptState) error {
	if err := requireActivePlayer(s, "OBJ_FIND"); err != nil {
		return err
	}
	if err := requireConfigs(s, "OBJ_FIND"); err != nil {
		return err
	}
	objId := s.PopInt()
	coord := s.PopInt()
	// L23: validate objType before coord, matching TS order
	// (ObjOps.ts:172-173: ObjTypeValid then CoordValid). When both are
	// invalid TS surfaces the ObjType error first.
	if err := checkObjType(s, objId, "OBJ_FIND"); err != nil {
		return err
	}
	level, x, z, err := checkCoord(coord, "OBJ_FIND")
	if err != nil {
		return err
	}
	if s.World == nil {
		s.PushInt(0)
		return nil
	}
	obj := s.World.GetObj(level, x, z, objId, s.activePlayer().UID())
	if obj == nil {
		s.PushInt(0)
		return nil
	}
	setActiveObjSlot(s, obj)
	s.PushInt(1)
	return nil
}

// handleObjFindAllZone (OBJ_FINDALLZONE, opcode 3506) pops a coord and
// stores a single-zone ObjIterator targeting the zone containing that
// coord. Mirrors TS ObjOps.ts:185-189.
//
// Nil-World degrades silently (matches LOC_FINDALLZONE convention at
// handlers_loc.go).
func handleObjFindAllZone(s *ScriptState) error {
	coord := s.PopInt()
	level, x, z, err := checkCoord(coord, "OBJ_FINDALLZONE")
	if err != nil {
		return err
	}
	if s.World == nil {
		return nil
	}
	s.objIterator = NewZoneObjIterator(s.World, s.World.CurrentTick(), level, x, z)
	return nil
}

// handleObjFindNext (OBJ_FINDNEXT, opcode 3507) advances the active
// ObjIterator and either sets the active obj slot + pushes 1 on hit, or
// pushes 0 on miss / nil-iterator. Mirrors TS ObjOps.ts:191-201.
//
// Stale-iterator semantics mirror LOC_FINDNEXT — return error on stale.
// Pointer-set: setActiveObjSlot threads IntOperand 0/1 per TS
// state.pointerAdd(ActiveObj[intOperand]).
func handleObjFindNext(s *ScriptState) error {
	it := s.objIterator
	if it == nil {
		s.PushInt(0)
		return nil
	}
	if it.Stale(s.World.CurrentTick()) {
		return fmt.Errorf("OBJ_FINDNEXT: tried to use an old iterator. Create a new iterator instead.")
	}
	obj, ok := it.Next()
	if !ok {
		s.PushInt(0)
		return nil
	}
	setActiveObjSlot(s, obj)
	s.PushInt(1)
	return nil
}

// handleObjName (OBJ_NAME, opcode 3508) pushes the active obj's name
// (or debugname fallback; "null" when both are empty). Mirrors TS
// ObjOps.ts:106-110 and the existing handleOcName at
// handlers_config.go:429.
func handleObjName(s *ScriptState) error {
	if err := requireActiveObj(s, "OBJ_NAME"); err != nil {
		return err
	}
	if err := requireConfigs(s, "OBJ_NAME"); err != nil {
		return err
	}
	id := s.activeObj().ObjType()
	if err := checkObjType(s, id, "OBJ_NAME"); err != nil {
		return err
	}
	ot := s.Configs.ObjType(id)
	if ot.Name != "" {
		s.PushString(ot.Name)
	} else if ot.DebugName != "" {
		s.PushString(ot.DebugName)
	} else {
		s.PushString("null")
	}
	return nil
}

// handleObjParam (OBJ_PARAM, opcode 3509) pops a paramID and delegates
// to paramLookup using the active obj's type Params. Mirrors TS
// ObjOps.ts:95-104 and the existing handleOcParam at
// handlers_config.go:449.
func handleObjParam(s *ScriptState) error {
	if err := requireActiveObj(s, "OBJ_PARAM"); err != nil {
		return err
	}
	if err := requireConfigs(s, "OBJ_PARAM"); err != nil {
		return err
	}
	paramID := s.PopInt()
	id := s.activeObj().ObjType()
	if err := checkObjType(s, id, "OBJ_PARAM"); err != nil {
		return err
	}
	ot := s.Configs.ObjType(id)
	return paramLookup(s, ot.Params, paramID, "OBJ_PARAM")
}

// handleObjType (OBJ_TYPE, opcode 3511) pushes the active obj's type id.
// Mirrors TS ObjOps.ts:132-134:
//
//	[ScriptOpcode.OBJ_TYPE]: state => {
//	    state.pushInt(check(state.activeObj.type, ObjTypeValid).id);
//	},
//
// TS validates the type id via ObjTypeValid. In goscape the active obj is
// pre-validated at the wire handler (handler_opobj.go:62-70 looks up
// ObjType.Configs[objId] before constructing the obj), so the id is
// round-trip-clean. (goscape defensive guard upstream; TS re-validates here.)
func handleObjType(s *ScriptState) error {
	if err := requireActiveObj(s, "OBJ_TYPE"); err != nil {
		return err
	}
	s.PushInt(s.activeObj().ObjType())
	return nil
}
