package script

import (
	"fmt"

	"github.com/zsrv/goscape/pkg/objtype"
)

// paramLookup is the shared path for OC_PARAM / NC_PARAM / LC_PARAM /
// STRUCT_PARAM. It reads params[paramID] from the config's ParamMap,
// falling back to the ParamType defaults when the key isn't present.
// The push target (int vs string) is dispatched by ParamType.Type.
//
// ParamMap values are populated by DecodeParams in pkg/objtype/paramtype.go:
// int entries are stored as uint32, string entries as string. We cast
// accordingly.
func paramLookup(s *ScriptState, params objtype.ParamMap, paramID int, op string) error {
	if err := checkParamType(s, paramID, op); err != nil {
		return err
	}
	pt := s.Configs.ParamType(paramID)
	isString := pt.Type == objtype.ScriptVarTypeString
	if v, ok := params[uint32(paramID)]; ok {
		if isString {
			// h-config-3: TS ParamHelper.getStringParam (ParamHelper.ts:10-16)
			// gates on `typeof value !== 'string'` — a stored non-string
			// (i.e. a number under a string-typed param) falls through to
			// the default rather than throwing. goscape pre-fix returned
			// a hard error here and aborted the script; the TS-faithful
			// behaviour is to fall through to the ParamType.DefaultString
			// branch below.
			if sv, isStr := v.(string); isStr {
				s.PushString(sv)
				return nil
			}
		} else {
			// h-config-3: TS ParamHelper.getIntParam (ParamHelper.ts:18-24)
			// gates on `typeof value !== 'number'` — falls through to the
			// default on type mismatch rather than throwing.
			if iv, isInt := v.(uint32); isInt {
				// NAI-122 in-scope-stretch: param ints are stored as the
				// raw uint32 wire bytes (DecodeParams reads via Packet.G4).
				// Cast through int32 to sign-extend negative values
				// (RuneScape weapon configs encode bonuses like -4 stab
				// as 0xFFFFFFFC). Direct uint32→int loses the sign.
				s.PushInt(int(int32(iv)))
				return nil
			}
		}
	}
	// Fall through to ParamType defaults — either the param key is
	// missing OR the stored value is the wrong runtime type (h-config-3).
	// TS ParamHelper.getStringParam
	// (ParamHelper.ts:10-16) returns `defaultValue ?? 'null'` — when the
	// ParamType's defaultString is unset (TS field default `null`), TS
	// pushes the literal string "null". goscape stores DefaultString as a
	// Go `string` (zero-value ""), so an unset defaultString surfaces as
	// "" — map that case to "null" to match TS. (The int side has no
	// equivalent; TS ParamType.defaultInt defaults to -1, which goscape
	// already preserves as the zero-value of int32 sign-extended.)
	if isString {
		if pt.DefaultString == "" {
			s.PushString("null")
		} else {
			s.PushString(pt.DefaultString)
		}
	} else {
		s.PushInt(int(pt.DefaultInt))
	}
	return nil
}

// requireConfigs returns a non-nil error when ScriptState.Configs is unset.
// All config-read handlers call this first.
func requireConfigs(s *ScriptState, op string) error {
	if s.Configs == nil {
		return fmt.Errorf("%s: Configs not set on ScriptState", op)
	}
	return nil
}

// checkParamType mirrors TS ParamTypeValid (ScriptValidators.ts:110).
func checkParamType(s *ScriptState, id int, op string) error {
	if s.Configs == nil || s.Configs.ParamType(id) == nil {
		return fmt.Errorf("%s: no ParamType with value (%d) found", op, id)
	}
	return nil
}

// checkEnumType mirrors TS EnumTypeValid (ScriptValidators.ts:119).
func checkEnumType(s *ScriptState, id int, op string) error {
	if s.Configs == nil || s.Configs.EnumType(id) == nil {
		return fmt.Errorf("%s: no EnumType with value (%d) found", op, id)
	}
	return nil
}

// checkStructType mirrors TS StructTypeValid (ScriptValidators.ts:133).
func checkStructType(s *ScriptState, id int, op string) error {
	if s.Configs == nil || s.Configs.StructType(id) == nil {
		return fmt.Errorf("%s: no StructType with value (%d) found", op, id)
	}
	return nil
}

// -- EnumOps --

// handleEnum (ENUM, opcode 4400) pops [inputType, outputType, enumID, key]
// (key is stack top, matching TS popInts(4)), validates enum input/output
// types, looks up the value, and pushes it via the output-type dispatch.
func handleEnum(s *ScriptState) error {
	if err := requireConfigs(s, "ENUM"); err != nil {
		return err
	}
	key := s.PopInt()
	enumID := s.PopInt()
	outputType := s.PopInt()
	inputType := s.PopInt()

	if err := checkEnumType(s, enumID, "ENUM"); err != nil {
		return err
	}
	et := s.Configs.EnumType(enumID)
	if int(et.InputType) != inputType || int(et.OutputType) != outputType {
		return fmt.Errorf("ENUM: type validation error for %q key %d: expected input %d got %d, expected output %d got %d",
			et.DebugName, key, inputType, int(et.InputType), outputType, int(et.OutputType))
	}

	// TS EnumOps.ts:17-22 dispatches by the VALUE's runtime type:
	//
	//     const value = enumType.values.get(key);
	//     if (typeof value === 'string') {
	//         state.pushString(value ?? enumType.defaultString);
	//     } else {
	//         state.pushInt(value ?? enumType.defaultInt);
	//     }
	//
	// Two behaviours fall out of this that goscape's pre-fix OutputType-based
	// dispatch did not match (h-config-1 / h-core-1):
	//
	//  1. When the key resolves to a value, the dispatch keys off the value's
	//     own type — match TS by doing the same type-switch on the Go any.
	//  2. When the key is MISSING (Values.get returns undefined),
	//     `typeof undefined !== 'string'` falls into the else branch, so TS
	//     ALWAYS pushes defaultInt to the INT stack — even when the enum's
	//     declared OutputType is string. goscape previously routed missing
	//     keys on string-output enums to PushString(DefaultString), which
	//     diverges from TS and could leave the int stack underpopulated
	//     relative to TS-faithful callers.
	if v, ok := et.Values[int32(key)]; ok {
		switch vt := v.(type) {
		case string:
			s.PushString(vt)
		case int32:
			s.PushInt(int(vt))
		default:
			return fmt.Errorf("ENUM: enum %d value at key %d: unexpected runtime type %T", enumID, key, v)
		}
		return nil
	}
	s.PushInt(int(et.DefaultInt))
	return nil
}

// handleEnumGetOutputCount (ENUM_GETOUTPUTCOUNT, opcode 4401) pops an enum
// id and pushes the number of entries in its Values map.
func handleEnumGetOutputCount(s *ScriptState) error {
	if err := requireConfigs(s, "ENUM_GETOUTPUTCOUNT"); err != nil {
		return err
	}
	enumID := s.PopInt()
	if err := checkEnumType(s, enumID, "ENUM_GETOUTPUTCOUNT"); err != nil {
		return err
	}
	et := s.Configs.EnumType(enumID)
	s.PushInt(len(et.Values))
	return nil
}

// -- STRUCT_PARAM (244: ServerOps; StructOps.ts deleted upstream) --

// handleStructParam (STRUCT_PARAM, opcode 1028) pops [structID, paramID]
// (paramID is stack top) and delegates to paramLookup. 244 deleted
// StructOps.ts and moved STRUCT_PARAM into the server block
// (ServerOps.ts:254-264 at pin 9aadcec4).
func handleStructParam(s *ScriptState) error {
	if err := requireConfigs(s, "STRUCT_PARAM"); err != nil {
		return err
	}
	paramID := s.PopInt()
	structID := s.PopInt()
	if err := checkStructType(s, structID, "STRUCT_PARAM"); err != nil {
		return err
	}
	st := s.Configs.StructType(structID)
	return paramLookup(s, st.Params, paramID, "STRUCT_PARAM")
}

// -- LocConfigOps --

// handleLcName (LC_NAME) pops a loc id and pushes its name, falling
// back to debugname, then "null".
func handleLcName(s *ScriptState) error {
	if err := requireConfigs(s, "LC_NAME"); err != nil {
		return err
	}
	id := s.PopInt()
	if err := checkLocType(s, id, "LC_NAME"); err != nil {
		return err
	}
	lt := s.Configs.LocType(id)
	if lt.Name != "" {
		s.PushString(lt.Name)
	} else if lt.DebugName != "" {
		s.PushString(lt.DebugName)
	} else {
		s.PushString("null")
	}
	return nil
}

// handleLcParam (LC_PARAM) pops [locID, paramID] and delegates to paramLookup.
func handleLcParam(s *ScriptState) error {
	if err := requireConfigs(s, "LC_PARAM"); err != nil {
		return err
	}
	paramID := s.PopInt()
	locID := s.PopInt()
	if err := checkLocType(s, locID, "LC_PARAM"); err != nil {
		return err
	}
	lt := s.Configs.LocType(locID)
	return paramLookup(s, lt.Params, paramID, "LC_PARAM")
}

// handleLcCategory (LC_CATEGORY) pops a loc id and pushes its category.
func handleLcCategory(s *ScriptState) error {
	if err := requireConfigs(s, "LC_CATEGORY"); err != nil {
		return err
	}
	id := s.PopInt()
	if err := checkLocType(s, id, "LC_CATEGORY"); err != nil {
		return err
	}
	lt := s.Configs.LocType(id)
	s.PushInt(lt.Category)
	return nil
}

// handleLcDesc (LC_DESC) pops a loc id and pushes its description.
// Empty Desc pushes "null" matching TS fallback.
func handleLcDesc(s *ScriptState) error {
	if err := requireConfigs(s, "LC_DESC"); err != nil {
		return err
	}
	id := s.PopInt()
	if err := checkLocType(s, id, "LC_DESC"); err != nil {
		return err
	}
	lt := s.Configs.LocType(id)
	if lt.Desc == "" {
		s.PushString("null")
	} else {
		s.PushString(lt.Desc)
	}
	return nil
}

// handleLcDebugName (LC_DEBUGNAME) pops a loc id and pushes its debugname.
func handleLcDebugName(s *ScriptState) error {
	if err := requireConfigs(s, "LC_DEBUGNAME"); err != nil {
		return err
	}
	id := s.PopInt()
	if err := checkLocType(s, id, "LC_DEBUGNAME"); err != nil {
		return err
	}
	lt := s.Configs.LocType(id)
	if lt.DebugName == "" {
		s.PushString("null")
	} else {
		s.PushString(lt.DebugName)
	}
	return nil
}

// handleLcWidth (LC_WIDTH) pops a loc id and pushes its tile width.
func handleLcWidth(s *ScriptState) error {
	if err := requireConfigs(s, "LC_WIDTH"); err != nil {
		return err
	}
	id := s.PopInt()
	if err := checkLocType(s, id, "LC_WIDTH"); err != nil {
		return err
	}
	lt := s.Configs.LocType(id)
	s.PushInt(lt.Width)
	return nil
}

// handleLcLength (LC_LENGTH) pops a loc id and pushes its tile length.
func handleLcLength(s *ScriptState) error {
	if err := requireConfigs(s, "LC_LENGTH"); err != nil {
		return err
	}
	id := s.PopInt()
	if err := checkLocType(s, id, "LC_LENGTH"); err != nil {
		return err
	}
	lt := s.Configs.LocType(id)
	s.PushInt(lt.Length)
	return nil
}

// -- NpcConfigOps --

// handleNcName (NC_NAME) pops a npc id and pushes its name (or debugname
// fallback; "null" when both are empty).
func handleNcName(s *ScriptState) error {
	if err := requireConfigs(s, "NC_NAME"); err != nil {
		return err
	}
	id := s.PopInt()
	if err := checkNpcType(s, id, "NC_NAME"); err != nil {
		return err
	}
	nt := s.Configs.NpcType(id)
	if nt.Name != "" {
		s.PushString(nt.Name)
	} else if nt.DebugName != "" {
		s.PushString(nt.DebugName)
	} else {
		s.PushString("null")
	}
	return nil
}

// handleNcParam (NC_PARAM) pops [npcID, paramID] and delegates to paramLookup.
func handleNcParam(s *ScriptState) error {
	if err := requireConfigs(s, "NC_PARAM"); err != nil {
		return err
	}
	paramID := s.PopInt()
	npcID := s.PopInt()
	if err := checkNpcType(s, npcID, "NC_PARAM"); err != nil {
		return err
	}
	nt := s.Configs.NpcType(npcID)
	return paramLookup(s, nt.Params, paramID, "NC_PARAM")
}

// handleNpcParam (NPC_PARAM, opcode 2523) reads a param from the
// ACTIVE npc's NpcType (vs. NC_PARAM which pops an explicit npcID).
// Pop order: paramID. Mirrors TS NpcOps.ts:132-141 — checkedHandler
// (ActiveNpc) + ParamHelper.getIntParam / getStringParam.
func handleNpcParam(s *ScriptState) error {
	if err := requireConfigs(s, "NPC_PARAM"); err != nil {
		return err
	}
	if err := requireActiveNpc(s, "NPC_PARAM"); err != nil {
		return err
	}
	paramID := s.PopInt()
	npcID := s.activeNpc().NpcType()
	if err := checkNpcType(s, npcID, "NPC_PARAM"); err != nil {
		return err
	}
	nt := s.Configs.NpcType(npcID)
	return paramLookup(s, nt.Params, paramID, "NPC_PARAM")
}

// handleNcCategory (NC_CATEGORY) pops a npc id and pushes its category.
func handleNcCategory(s *ScriptState) error {
	if err := requireConfigs(s, "NC_CATEGORY"); err != nil {
		return err
	}
	id := s.PopInt()
	if err := checkNpcType(s, id, "NC_CATEGORY"); err != nil {
		return err
	}
	nt := s.Configs.NpcType(id)
	s.PushInt(nt.Category)
	return nil
}

// handleNcDesc (NC_DESC) pops a npc id and pushes its description.
func handleNcDesc(s *ScriptState) error {
	if err := requireConfigs(s, "NC_DESC"); err != nil {
		return err
	}
	id := s.PopInt()
	if err := checkNpcType(s, id, "NC_DESC"); err != nil {
		return err
	}
	nt := s.Configs.NpcType(id)
	if nt.Desc == "" {
		s.PushString("null")
	} else {
		s.PushString(nt.Desc)
	}
	return nil
}

// handleNcDebugName (NC_DEBUGNAME) pops a npc id and pushes its debugname.
func handleNcDebugName(s *ScriptState) error {
	if err := requireConfigs(s, "NC_DEBUGNAME"); err != nil {
		return err
	}
	id := s.PopInt()
	if err := checkNpcType(s, id, "NC_DEBUGNAME"); err != nil {
		return err
	}
	nt := s.Configs.NpcType(id)
	if nt.DebugName == "" {
		s.PushString("null")
	} else {
		s.PushString(nt.DebugName)
	}
	return nil
}

// handleNcOp (NC_OP) pops [npcID, op] where op is 1-based, and pushes
// npc.Op[op-1] if in range, otherwise "".
//
// op==-1 (null sentinel) returns an error per TS NumberNotNull
// (NpcConfigOps.ts:43).
func handleNcOp(s *ScriptState) error {
	if err := requireConfigs(s, "NC_OP"); err != nil {
		return err
	}
	op := s.PopInt()
	if err := checkNotNull(op, "NC_OP"); err != nil {
		return err
	}
	npcID := s.PopInt()
	if err := checkNpcType(s, npcID, "NC_OP"); err != nil {
		return err
	}
	nt := s.Configs.NpcType(npcID)
	idx := op - 1
	if nt.Op == nil || idx < 0 || idx >= len(nt.Op) {
		s.PushString("")
		return nil
	}
	s.PushString(nt.Op[idx])
	return nil
}

// handleNcSize (NC_SIZE) pops a npc id and pushes its tile size.
func handleNcSize(s *ScriptState) error {
	if err := requireConfigs(s, "NC_SIZE"); err != nil {
		return err
	}
	id := s.PopInt()
	if err := checkNpcType(s, id, "NC_SIZE"); err != nil {
		return err
	}
	nt := s.Configs.NpcType(id)
	s.PushInt(int(nt.Size))
	return nil
}

// handleNcVisLevel (NC_VISLEVEL) pops a npc id and pushes its visible
// combat level (vislevel).
func handleNcVisLevel(s *ScriptState) error {
	if err := requireConfigs(s, "NC_VISLEVEL"); err != nil {
		return err
	}
	id := s.PopInt()
	if err := checkNpcType(s, id, "NC_VISLEVEL"); err != nil {
		return err
	}
	nt := s.Configs.NpcType(id)
	s.PushInt(nt.VisLevel)
	return nil
}

// -- ObjConfigOps --

// handleOcName (OC_NAME) pops an obj id and pushes its name (or debugname
// fallback; "null" when both are empty).
func handleOcName(s *ScriptState) error {
	if err := requireConfigs(s, "OC_NAME"); err != nil {
		return err
	}
	id := s.PopInt()
	if err := checkObjType(s, id, "OC_NAME"); err != nil {
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

// handleOcParam (OC_PARAM) pops [objID, paramID] and delegates to paramLookup.
func handleOcParam(s *ScriptState) error {
	if err := requireConfigs(s, "OC_PARAM"); err != nil {
		return err
	}
	paramID := s.PopInt()
	objID := s.PopInt()
	if err := checkObjType(s, objID, "OC_PARAM"); err != nil {
		return err
	}
	ot := s.Configs.ObjType(objID)
	return paramLookup(s, ot.Params, paramID, "OC_PARAM")
}

// handleOcCategory (OC_CATEGORY) pops an obj id and pushes its category.
func handleOcCategory(s *ScriptState) error {
	if err := requireConfigs(s, "OC_CATEGORY"); err != nil {
		return err
	}
	id := s.PopInt()
	if err := checkObjType(s, id, "OC_CATEGORY"); err != nil {
		return err
	}
	ot := s.Configs.ObjType(id)
	s.PushInt(ot.Category)
	return nil
}

// handleOcDesc (OC_DESC) pops an obj id and pushes its description.
func handleOcDesc(s *ScriptState) error {
	if err := requireConfigs(s, "OC_DESC"); err != nil {
		return err
	}
	id := s.PopInt()
	if err := checkObjType(s, id, "OC_DESC"); err != nil {
		return err
	}
	ot := s.Configs.ObjType(id)
	if ot.Desc == "" {
		s.PushString("null")
	} else {
		s.PushString(ot.Desc)
	}
	return nil
}

// handleOcMembers (OC_MEMBERS) pops an obj id and pushes 1/0 for members.
func handleOcMembers(s *ScriptState) error {
	if err := requireConfigs(s, "OC_MEMBERS"); err != nil {
		return err
	}
	id := s.PopInt()
	if err := checkObjType(s, id, "OC_MEMBERS"); err != nil {
		return err
	}
	ot := s.Configs.ObjType(id)
	if ot.Members {
		s.PushInt(1)
	} else {
		s.PushInt(0)
	}
	return nil
}

// handleOcWeight (OC_WEIGHT) pops an obj id and pushes its weight (grams).
func handleOcWeight(s *ScriptState) error {
	if err := requireConfigs(s, "OC_WEIGHT"); err != nil {
		return err
	}
	id := s.PopInt()
	if err := checkObjType(s, id, "OC_WEIGHT"); err != nil {
		return err
	}
	ot := s.Configs.ObjType(id)
	s.PushInt(ot.Weight)
	return nil
}

// handleOcWearPos (OC_WEARPOS) pops an obj id and pushes its primary wearpos.
func handleOcWearPos(s *ScriptState) error {
	if err := requireConfigs(s, "OC_WEARPOS"); err != nil {
		return err
	}
	id := s.PopInt()
	if err := checkObjType(s, id, "OC_WEARPOS"); err != nil {
		return err
	}
	ot := s.Configs.ObjType(id)
	s.PushInt(ot.WearPos)
	return nil
}

// handleOcWearPos2 (OC_WEARPOS2) pops an obj id and pushes its 2nd wearpos.
func handleOcWearPos2(s *ScriptState) error {
	if err := requireConfigs(s, "OC_WEARPOS2"); err != nil {
		return err
	}
	id := s.PopInt()
	if err := checkObjType(s, id, "OC_WEARPOS2"); err != nil {
		return err
	}
	ot := s.Configs.ObjType(id)
	s.PushInt(ot.WearPos2)
	return nil
}

// handleOcWearPos3 (OC_WEARPOS3) pops an obj id and pushes its 3rd wearpos.
func handleOcWearPos3(s *ScriptState) error {
	if err := requireConfigs(s, "OC_WEARPOS3"); err != nil {
		return err
	}
	id := s.PopInt()
	if err := checkObjType(s, id, "OC_WEARPOS3"); err != nil {
		return err
	}
	ot := s.Configs.ObjType(id)
	s.PushInt(ot.WearPos3)
	return nil
}

// handleOcCost (OC_COST) pops an obj id and pushes its shop cost.
func handleOcCost(s *ScriptState) error {
	if err := requireConfigs(s, "OC_COST"); err != nil {
		return err
	}
	id := s.PopInt()
	if err := checkObjType(s, id, "OC_COST"); err != nil {
		return err
	}
	ot := s.Configs.ObjType(id)
	s.PushInt(ot.Cost)
	return nil
}

// handleOcTradeable (OC_TRADEABLE) pops an obj id and pushes 1/0 tradeable.
func handleOcTradeable(s *ScriptState) error {
	if err := requireConfigs(s, "OC_TRADEABLE"); err != nil {
		return err
	}
	id := s.PopInt()
	if err := checkObjType(s, id, "OC_TRADEABLE"); err != nil {
		return err
	}
	ot := s.Configs.ObjType(id)
	if ot.Tradeable {
		s.PushInt(1)
	} else {
		s.PushInt(0)
	}
	return nil
}

// handleOcDebugName (OC_DEBUGNAME) pops an obj id and pushes its debugname.
func handleOcDebugName(s *ScriptState) error {
	if err := requireConfigs(s, "OC_DEBUGNAME"); err != nil {
		return err
	}
	id := s.PopInt()
	if err := checkObjType(s, id, "OC_DEBUGNAME"); err != nil {
		return err
	}
	ot := s.Configs.ObjType(id)
	if ot.DebugName == "" {
		s.PushString("null")
	} else {
		s.PushString(ot.DebugName)
	}
	return nil
}

// handleOcCert (OC_CERT) swaps an uncerted item id for its certed (noted)
// counterpart via ObjType.CertLink / CertTemplate. Follows TS rule:
//
//	certtemplate == -1 && certlink >= 0  →  push certlink (swap to cert)
//	otherwise                            →  push input id unchanged
func handleOcCert(s *ScriptState) error {
	if err := requireConfigs(s, "OC_CERT"); err != nil {
		return err
	}
	id := s.PopInt()
	if err := checkObjType(s, id, "OC_CERT"); err != nil {
		return err
	}
	ot := s.Configs.ObjType(id)
	if ot.CertTemplate == -1 && ot.CertLink >= 0 {
		s.PushInt(ot.CertLink)
	} else {
		s.PushInt(ot.ID)
	}
	return nil
}

// handleOcUncert (OC_UNCERT) swaps a certed (noted) item id for its base
// item via ObjType.CertLink / CertTemplate. Follows TS rule:
//
//	certtemplate >= 0 && certlink >= 0  →  push certlink (swap to base)
//	otherwise                           →  push input id unchanged
func handleOcUncert(s *ScriptState) error {
	if err := requireConfigs(s, "OC_UNCERT"); err != nil {
		return err
	}
	id := s.PopInt()
	if err := checkObjType(s, id, "OC_UNCERT"); err != nil {
		return err
	}
	ot := s.Configs.ObjType(id)
	if ot.CertTemplate >= 0 && ot.CertLink >= 0 {
		s.PushInt(ot.CertLink)
	} else {
		s.PushInt(ot.ID)
	}
	return nil
}

// handleOcStackable (OC_STACKABLE) pops an obj id and pushes 1/0 stackable.
func handleOcStackable(s *ScriptState) error {
	if err := requireConfigs(s, "OC_STACKABLE"); err != nil {
		return err
	}
	id := s.PopInt()
	if err := checkObjType(s, id, "OC_STACKABLE"); err != nil {
		return err
	}
	ot := s.Configs.ObjType(id)
	if ot.Stackable {
		s.PushInt(1)
	} else {
		s.PushInt(0)
	}
	return nil
}
