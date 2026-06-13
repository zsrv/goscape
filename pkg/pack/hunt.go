package pack

import (
	"fmt"
	"strconv"

	"github.com/zsrv/goscape/pkg/objtype"
)

// huntCheckInv is the parser-produced value for check_inv=inv,obj,condition+val.
type huntCheckInv struct {
	inv       int
	obj       int
	condition string
	val       int
}

// huntCheckInvParam is the parser-produced value for check_invparam=inv,param,condition+val.
type huntCheckInvParam struct {
	inv       int
	param     int
	condition string
	val       int
}

// huntCheckVarParsed is the parser-produced value for extracheck_var=%varp,condition+val.
type huntCheckVarParsed struct {
	varp      int
	condition string
	val       int
}

// parseHuntConfigFor returns the per-key=value parser for .hunt config
// blocks. Routes 17 keys to enum/registry/struct values per TS
// HuntConfig.ts:9-381.
//
// NAI-195-D-DEADBRANCH-OMITTED: TS stringKeys: [] and booleanKeys: []
// arrays are empty — branches omitted.
//
// NAI-198-D-HUNT-OPOBJ2-TS-BUG: the find_newmode arm faithfully ports
// the TS bug at HuntConfig.ts:201-202 (string 'opobj2' maps to
// NPCModeOpObj1 = 27, not NPCModeOpObj2 = 28). See deviation-tag pin
// in nai198_deviation_pins_test.go.
//
// TS source: tools/pack/config/HuntConfig.ts:9-381.
func parseHuntConfigFor(
	categoryPack, invPack, locPack, npcPack, objPack, paramPack, varnPack, varpPack *PackFile,
) ParseFn {
	return func(key, value string) (ConfigValue, bool, error) {
		switch key {
		case "rate":
			var number int
			if len(value) > 2 && value[:2] == "0x" {
				n, e := strconv.ParseInt(value[2:], 16, 64)
				if e != nil {
					return nil, true, fmt.Errorf("invalid rate hex: %s", value)
				}
				number = int(n)
			} else {
				n, e := strconv.ParseInt(value, 10, 64)
				if e != nil {
					return nil, true, fmt.Errorf("invalid rate: %s", value)
				}
				number = int(n)
			}
			if number < 1 || number > 255 {
				return nil, true, fmt.Errorf("rate out of range [1,255]: %d", number)
			}
			return number, true, nil

		case "type":
			switch value {
			case "off":
				return objtype.HuntModeOff, true, nil
			case "player":
				return objtype.HuntModePlayer, true, nil
			case "npc":
				return objtype.HuntModeNpc, true, nil
			case "obj":
				return objtype.HuntModeObj, true, nil
			case "scenery":
				return objtype.HuntModeScenery, true, nil
			default:
				return nil, true, fmt.Errorf("unknown hunt type: %s", value)
			}

		case "check_vis":
			switch value {
			case "off":
				return objtype.HuntVisOff, true, nil
			case "lineofsight":
				return objtype.HuntVisLineOfSight, true, nil
			case "lineofwalk":
				return objtype.HuntVisLineOfWalk, true, nil
			default:
				return nil, true, fmt.Errorf("unknown check_vis: %s", value)
			}

		case "check_nottoostrong":
			switch value {
			case "off":
				return objtype.HuntCheckNotTooStrongOff, true, nil
			case "outside_wilderness":
				return objtype.HuntCheckNotTooStrongOutsideWilderness, true, nil
			default:
				return nil, true, fmt.Errorf("unknown check_nottoostrong: %s", value)
			}

		case "check_notcombat":
			if len(value) == 0 || value[0] != '%' {
				return nil, true, fmt.Errorf("check_notcombat value must start with %%: %s", value)
			}
			name := value[1:]
			idx := varpPack.GetByName(name)
			if idx == -1 {
				return nil, true, fmt.Errorf("unknown varp %q for check_notcombat", name)
			}
			return idx, true, nil

		case "check_notcombat_self":
			if len(value) == 0 || value[0] != '%' {
				return nil, true, fmt.Errorf("check_notcombat_self value must start with %%: %s", value)
			}
			name := value[1:]
			idx := varnPack.GetByName(name)
			if idx == -1 {
				return nil, true, fmt.Errorf("unknown varn %q for check_notcombat_self", name)
			}
			return idx, true, nil

		case "check_notbusy":
			switch value {
			case "off":
				return false, true, nil
			case "on":
				return true, true, nil
			default:
				return nil, true, fmt.Errorf("unknown check_notbusy: %s", value)
			}

		case "find_keephunting":
			switch value {
			case "off":
				return false, true, nil
			case "on":
				return true, true, nil
			default:
				return nil, true, fmt.Errorf("unknown find_keephunting: %s", value)
			}

		case "find_newmode":
			switch value {
			case "opplayer1":
				return objtype.NPCModeOpPlayer1, true, nil
			case "opplayer2":
				return objtype.NPCModeOpPlayer2, true, nil
			case "opplayer3":
				return objtype.NPCModeOpPlayer3, true, nil
			case "opplayer4":
				return objtype.NPCModeOpPlayer4, true, nil
			case "opplayer5":
				return objtype.NPCModeOpPlayer5, true, nil
			case "applayer1":
				return objtype.NPCModeApPlayer1, true, nil
			case "applayer2":
				return objtype.NPCModeApPlayer2, true, nil
			case "applayer3":
				return objtype.NPCModeApPlayer3, true, nil
			case "applayer4":
				return objtype.NPCModeApPlayer4, true, nil
			case "applayer5":
				return objtype.NPCModeApPlayer5, true, nil
			case "queue1":
				return objtype.NPCModeQueue1, true, nil
			case "queue2":
				return objtype.NPCModeQueue2, true, nil
			case "queue3":
				return objtype.NPCModeQueue3, true, nil
			case "queue4":
				return objtype.NPCModeQueue4, true, nil
			case "queue5":
				return objtype.NPCModeQueue5, true, nil
			case "queue6":
				return objtype.NPCModeQueue6, true, nil
			case "queue7":
				return objtype.NPCModeQueue7, true, nil
			case "queue8":
				return objtype.NPCModeQueue8, true, nil
			case "queue9":
				return objtype.NPCModeQueue9, true, nil
			case "queue10":
				return objtype.NPCModeQueue10, true, nil
			case "queue11":
				return objtype.NPCModeQueue11, true, nil
			case "queue12":
				return objtype.NPCModeQueue12, true, nil
			case "queue13":
				return objtype.NPCModeQueue13, true, nil
			case "queue14":
				return objtype.NPCModeQueue14, true, nil
			case "queue15":
				return objtype.NPCModeQueue15, true, nil
			case "queue16":
				return objtype.NPCModeQueue16, true, nil
			case "queue17":
				return objtype.NPCModeQueue17, true, nil
			case "queue18":
				return objtype.NPCModeQueue18, true, nil
			case "queue19":
				return objtype.NPCModeQueue19, true, nil
			case "queue20":
				return objtype.NPCModeQueue20, true, nil
			case "opobj1":
				return objtype.NPCModeOpObj1, true, nil
			// NAI-198-D-HUNT-OPOBJ2-TS-BUG: TS HuntConfig.ts:201-202 maps the
			// 'opobj2' string to NpcMode.OPOBJ1 (typo in upstream — should be
			// OPOBJ2). Ported literally per [[true_to_ts_gate]]. Goscape constant
			// NPCModeOpObj1 = 27. Tracked for upstream reconciliation in
			// [[nai_followups]].
			case "opobj2":
				return objtype.NPCModeOpObj1, true, nil
			case "opobj3":
				return objtype.NPCModeOpObj3, true, nil
			case "opobj4":
				return objtype.NPCModeOpObj4, true, nil
			case "opobj5":
				return objtype.NPCModeOpObj5, true, nil
			case "apobj1":
				return objtype.NPCModeApObj1, true, nil
			case "apobj2":
				return objtype.NPCModeApObj2, true, nil
			case "apobj3":
				return objtype.NPCModeApObj3, true, nil
			case "apobj4":
				return objtype.NPCModeApObj4, true, nil
			case "apobj5":
				return objtype.NPCModeApObj5, true, nil
			case "opnpc1":
				return objtype.NPCModeOpNpc1, true, nil
			case "opnpc2":
				return objtype.NPCModeOpNpc2, true, nil
			case "opnpc3":
				return objtype.NPCModeOpNpc3, true, nil
			case "opnpc4":
				return objtype.NPCModeOpNpc4, true, nil
			case "opnpc5":
				return objtype.NPCModeOpNpc5, true, nil
			case "apnpc1":
				return objtype.NPCModeApNpc1, true, nil
			case "apnpc2":
				return objtype.NPCModeApNpc2, true, nil
			case "apnpc3":
				return objtype.NPCModeApNpc3, true, nil
			case "apnpc4":
				return objtype.NPCModeApNpc4, true, nil
			case "apnpc5":
				return objtype.NPCModeApNpc5, true, nil
			case "oploc1":
				return objtype.NPCModeOpLoc1, true, nil
			case "oploc2":
				return objtype.NPCModeOpLoc2, true, nil
			case "oploc3":
				return objtype.NPCModeOpLoc3, true, nil
			case "oploc4":
				return objtype.NPCModeOpLoc4, true, nil
			case "oploc5":
				return objtype.NPCModeOpLoc5, true, nil
			case "aploc1":
				return objtype.NPCModeApLoc1, true, nil
			case "aploc2":
				return objtype.NPCModeApLoc2, true, nil
			case "aploc3":
				return objtype.NPCModeApLoc3, true, nil
			case "aploc4":
				return objtype.NPCModeApLoc4, true, nil
			case "aploc5":
				return objtype.NPCModeApLoc5, true, nil
			default:
				return nil, true, fmt.Errorf("unknown find_newmode: %s", value)
			}

		case "nobodynear":
			switch value {
			case "keephunting":
				return objtype.HuntNobodyNearKeepHunting, true, nil
			case "pausehunt":
				return objtype.HuntNobodyNearPauseHunt, true, nil
			default:
				return nil, true, fmt.Errorf("unknown nobodynear: %s", value)
			}

		case "check_afk":
			switch value {
			case "off":
				return false, true, nil
			case "on":
				return true, true, nil
			default:
				return nil, true, fmt.Errorf("unknown check_afk: %s", value)
			}

		case "check_category":
			idx := categoryPack.GetByName(value)
			if idx == -1 {
				return nil, true, fmt.Errorf("unknown category %q", value)
			}
			return idx, true, nil

		case "check_npc":
			idx := npcPack.GetByName(value)
			if idx == -1 {
				return nil, true, fmt.Errorf("unknown npc %q", value)
			}
			return idx, true, nil

		case "check_obj":
			idx := objPack.GetByName(value)
			if idx == -1 {
				return nil, true, fmt.Errorf("unknown obj %q", value)
			}
			return idx, true, nil

		case "check_loc":
			idx := locPack.GetByName(value)
			if idx == -1 {
				return nil, true, fmt.Errorf("unknown loc %q", value)
			}
			return idx, true, nil

		case "check_inv":
			// check_inv=inv,obj,condition+val
			parts := parseCsv(value)
			if len(parts) != 3 {
				return nil, true, fmt.Errorf("check_inv requires 3 parts: %s", value)
			}
			inv := invPack.GetByName(parts[0])
			if inv == -1 {
				return nil, true, fmt.Errorf("unknown inv %q for check_inv", parts[0])
			}
			obj := objPack.GetByName(parts[1])
			if obj == -1 {
				return nil, true, fmt.Errorf("unknown obj %q for check_inv", parts[1])
			}
			conditionWithVal := parts[2]
			if len(conditionWithVal) == 0 {
				return nil, true, fmt.Errorf("empty condition in check_inv: %s", value)
			}
			condition := string(conditionWithVal[0])
			if condition != "=" && condition != ">" && condition != "<" && condition != "!" {
				return nil, true, fmt.Errorf("invalid condition %q in check_inv: %s", condition, value)
			}
			val, err := strconv.Atoi(conditionWithVal[1:])
			if err != nil {
				return nil, true, fmt.Errorf("invalid val in check_inv: %s", value)
			}
			return huntCheckInv{inv: inv, obj: obj, condition: condition, val: val}, true, nil

		case "check_invparam":
			// check_invparam=inv,param,condition+val
			parts := parseCsv(value)
			if len(parts) != 3 {
				return nil, true, fmt.Errorf("check_invparam requires 3 parts: %s", value)
			}
			inv := invPack.GetByName(parts[0])
			if inv == -1 {
				return nil, true, fmt.Errorf("unknown inv %q for check_invparam", parts[0])
			}
			param := paramPack.GetByName(parts[1])
			if param == -1 {
				return nil, true, fmt.Errorf("unknown param %q for check_invparam", parts[1])
			}
			conditionWithVal := parts[2]
			if len(conditionWithVal) == 0 {
				return nil, true, fmt.Errorf("empty condition in check_invparam: %s", value)
			}
			condition := string(conditionWithVal[0])
			if condition != "=" && condition != ">" && condition != "<" && condition != "!" {
				return nil, true, fmt.Errorf("invalid condition %q in check_invparam: %s", condition, value)
			}
			val, err := strconv.Atoi(conditionWithVal[1:])
			if err != nil {
				return nil, true, fmt.Errorf("invalid val in check_invparam: %s", value)
			}
			return huntCheckInvParam{inv: inv, param: param, condition: condition, val: val}, true, nil

		case "extracheck_var":
			// extracheck_var=%varp,condition+val
			parts := parseCsv(value)
			if len(parts) != 2 {
				return nil, true, fmt.Errorf("extracheck_var requires 2 parts: %s", value)
			}
			if len(parts[0]) == 0 || parts[0][0] != '%' {
				return nil, true, fmt.Errorf("extracheck_var first part must start with %%: %s", value)
			}
			varpName := parts[0][1:]
			varp := varpPack.GetByName(varpName)
			if varp == -1 {
				return nil, true, fmt.Errorf("unknown varp %q for extracheck_var", varpName)
			}
			conditionWithVal := parts[1]
			if len(conditionWithVal) == 0 {
				return nil, true, fmt.Errorf("empty condition in extracheck_var: %s", value)
			}
			condition := string(conditionWithVal[0])
			// '&' (no-common-bits) is whitelisted only for extracheck_var per TS
			// HuntConfig.ts:366 @dee467c8 (['=', '>', '<', '!', '&']). The
			// check_inv / check_invparam whitelists above intentionally omit it,
			// matching TS HuntConfig.ts:320 / :346 (still ['=', '>', '<', '!']).
			if condition != "=" && condition != ">" && condition != "<" && condition != "!" && condition != "&" {
				return nil, true, fmt.Errorf("invalid condition %q in extracheck_var: %s", condition, value)
			}
			val, err := strconv.Atoi(conditionWithVal[1:])
			if err != nil {
				return nil, true, fmt.Errorf("invalid val in extracheck_var: %s", value)
			}
			return huntCheckVarParsed{varp: varp, condition: condition, val: val}, true, nil
		}

		return nil, false, nil
	}
}

// hasKey reports whether any line in cfg has the given key.
func hasKey(cfg []ConfigLine, key string) bool {
	for _, line := range cfg {
		if line.Key == key {
			return true
		}
	}
	return false
}

// findTypeValue returns the value of the first 'type' line in cfg, or -1
// if absent. (Hunt opcodes 12-17 require a matching type for context.)
func findTypeValue(cfg []ConfigLine) int {
	for _, line := range cfg {
		if line.Key == "type" {
			if v, ok := line.Value.(int); ok {
				return v
			}
		}
	}
	return -1
}

// packHuntConfigs walks every id and emits opcodes 1-17 plus
// 18+extracheckVarsCount (max 3 → opcodes 18, 19, 20), gated by
// per-arm default-skip and mutex-predicate logic.
//
// Server-only — TS allocates a client PackedData but never writes to
// it. Goscape omits the client buffer entirely.
//
// modelFlags is accepted for TS ConfigPackCallback parity
// (PackShared.ts:137-141); hunt does not write any model flags.
//
// TS source: tools/pack/config/HuntConfig.ts:383-545.
func packHuntConfigs(configs map[string][]ConfigLine, pf *PackFile, modelFlags []int) (*PackedData, error) {
	pd := NewPackedData(pf.Max)
	for id := range pf.Max {
		name := pf.GetByID(id)
		if cfg, ok := configs[name]; ok {
			extracheckVarsCount := 0

			for _, line := range cfg {
				switch line.Key {
				case "type":
					// Opcode 1: only when value != HuntModeOff.
					if line.Value.(int) != objtype.HuntModeOff {
						pd.P1(1)
						pd.P1(uint8(line.Value.(int)))
					}

				case "check_vis":
					// Opcode 2: only when value != HuntVisOff.
					if line.Value.(int) != objtype.HuntVisOff {
						pd.P1(2)
						pd.P1(uint8(line.Value.(int)))
					}

				case "check_nottoostrong":
					// Opcode 3: only when value != HuntCheckNotTooStrongOff.
					if line.Value.(int) != objtype.HuntCheckNotTooStrongOff {
						pd.P1(3)
						pd.P1(uint8(line.Value.(int)))
					}

				case "check_notbusy":
					// Opcode 4: only when value != false.
					if line.Value.(bool) {
						pd.P1(4)
					}

				case "find_keephunting":
					// Opcode 5: only when value != false.
					if line.Value.(bool) {
						pd.P1(5)
					}

				case "find_newmode":
					// Opcode 6: only when value != NPCModeNone.
					if line.Value.(int) != objtype.NPCModeNone {
						pd.P1(6)
						pd.P1(uint8(line.Value.(int)))
					}

				case "nobodynear":
					// Opcode 7: only when value != HuntNobodyNearPauseHunt.
					if line.Value.(int) != objtype.HuntNobodyNearPauseHunt {
						pd.P1(7)
						pd.P1(uint8(line.Value.(int)))
					}

				case "check_notcombat":
					// Opcode 8: always when value != null (non-nil in Go — always true
					// since parser returns a valid int index).
					pd.P1(8)
					pd.P2(uint16(line.Value.(int)))

				case "check_notcombat_self":
					// Opcode 9: always when value != null (same as above).
					pd.P1(9)
					pd.P2(uint16(line.Value.(int)))

				case "check_afk":
					// Opcode 10: only when value != true (TS: if value !== true → emit).
					if !line.Value.(bool) {
						pd.P1(10)
					}

				case "rate":
					// Opcode 11: only when value != 1.
					if line.Value.(int) != 1 {
						pd.P1(11)
						pd.P2(uint16(line.Value.(int)))
					}

				case "check_category":
					// Opcode 12: mutex — must NOT have check_npc, check_obj, check_loc,
					// check_inv, check_invparam; must have type ∈ {NPC, OBJ, SCENERY}.
					if !hasKey(cfg, "check_npc") && !hasKey(cfg, "check_obj") &&
						!hasKey(cfg, "check_loc") && !hasKey(cfg, "check_inv") &&
						!hasKey(cfg, "check_invparam") {
						tv := findTypeValue(cfg)
						if tv == objtype.HuntModeNpc || tv == objtype.HuntModeObj || tv == objtype.HuntModeScenery {
							pd.P1(12)
							pd.P2(uint16(line.Value.(int)))
						} else {
							return nil, packStepError(name, "unable to pack line!!!\nInvalid property value: check_category=%v", line.Value)
						}
					} else {
						return nil, packStepError(name, "unable to pack line!!!\nInvalid property value: check_category=%v", line.Value)
					}

				case "check_npc":
					// Opcode 13: mutex — must NOT have check_category, check_obj, check_loc,
					// check_inv, check_invparam; must have type=NPC.
					if !hasKey(cfg, "check_category") && !hasKey(cfg, "check_obj") &&
						!hasKey(cfg, "check_loc") && !hasKey(cfg, "check_inv") &&
						!hasKey(cfg, "check_invparam") {
						if findTypeValue(cfg) == objtype.HuntModeNpc {
							pd.P1(13)
							pd.P2(uint16(line.Value.(int)))
						} else {
							return nil, packStepError(name, "unable to pack line!!!\nInvalid property value: check_npc=%v", line.Value)
						}
					} else {
						return nil, packStepError(name, "unable to pack line!!!\nInvalid property value: check_npc=%v", line.Value)
					}

				case "check_obj":
					// Opcode 14: mutex — must NOT have check_category, check_npc, check_loc,
					// check_inv, check_invparam; must have type=OBJ.
					if !hasKey(cfg, "check_category") && !hasKey(cfg, "check_npc") &&
						!hasKey(cfg, "check_loc") && !hasKey(cfg, "check_inv") &&
						!hasKey(cfg, "check_invparam") {
						if findTypeValue(cfg) == objtype.HuntModeObj {
							pd.P1(14)
							pd.P2(uint16(line.Value.(int)))
						} else {
							return nil, packStepError(name, "unable to pack line!!!\nInvalid property value: check_obj=%v", line.Value)
						}
					} else {
						return nil, packStepError(name, "unable to pack line!!!\nInvalid property value: check_obj=%v", line.Value)
					}

				case "check_loc":
					// Opcode 15: mutex — must NOT have check_category, check_npc, check_obj,
					// check_inv, check_invparam; must have type=SCENERY.
					if !hasKey(cfg, "check_category") && !hasKey(cfg, "check_npc") &&
						!hasKey(cfg, "check_obj") && !hasKey(cfg, "check_inv") &&
						!hasKey(cfg, "check_invparam") {
						if findTypeValue(cfg) == objtype.HuntModeScenery {
							pd.P1(15)
							pd.P2(uint16(line.Value.(int)))
						} else {
							return nil, packStepError(name, "unable to pack line!!!\nInvalid property value: check_loc=%v", line.Value)
						}
					} else {
						return nil, packStepError(name, "unable to pack line!!!\nInvalid property value: check_loc=%v", line.Value)
					}

				case "check_inv":
					// Opcode 16: mutex — must NOT have check_category, check_npc, check_obj,
					// check_loc, check_invparam; must have type=PLAYER.
					if !hasKey(cfg, "check_category") && !hasKey(cfg, "check_npc") &&
						!hasKey(cfg, "check_obj") && !hasKey(cfg, "check_loc") &&
						!hasKey(cfg, "check_invparam") {
						if findTypeValue(cfg) == objtype.HuntModePlayer {
							checkInv := line.Value.(huntCheckInv)
							pd.P1(16)
							pd.P2(uint16(checkInv.inv))
							pd.P2(uint16(checkInv.obj))
							pd.PJStr(checkInv.condition)
							pd.P4(uint32(int32(checkInv.val)))
						} else {
							return nil, packStepError(name, "unable to pack line!!!\nInvalid property value: check_inv=%v", line.Value)
						}
					} else {
						return nil, packStepError(name, "unable to pack line!!!\nInvalid property value: check_inv=%v", line.Value)
					}

				case "check_invparam":
					// Opcode 17: mutex — must NOT have check_category, check_npc, check_obj,
					// check_loc, check_inv; must have type=PLAYER.
					if !hasKey(cfg, "check_category") && !hasKey(cfg, "check_npc") &&
						!hasKey(cfg, "check_obj") && !hasKey(cfg, "check_loc") &&
						!hasKey(cfg, "check_inv") {
						if findTypeValue(cfg) == objtype.HuntModePlayer {
							checkInv := line.Value.(huntCheckInvParam)
							pd.P1(17)
							pd.P2(uint16(checkInv.inv))
							pd.P2(uint16(checkInv.param))
							pd.PJStr(checkInv.condition)
							pd.P4(uint32(int32(checkInv.val)))
						} else {
							return nil, packStepError(name, "unable to pack line!!!\nInvalid property value: check_invparam=%v", line.Value)
						}
					} else {
						return nil, packStepError(name, "unable to pack line!!!\nInvalid property value: check_invparam=%v", line.Value)
					}

				case "extracheck_var":
					// Opcodes 18-20: max 3 entries.
					if extracheckVarsCount > 2 {
						return nil, packStepError(name, "unable to pack line!!!\nLimit of 3 extracheck_var properties exceeded.")
					}
					// TS gates: value != null AND type=PLAYER.
					if findTypeValue(cfg) == objtype.HuntModePlayer {
						checkVar := line.Value.(huntCheckVarParsed)
						pd.P1(uint8(18 + extracheckVarsCount))
						pd.P2(uint16(checkVar.varp))
						pd.PJStr(checkVar.condition)
						pd.P4(uint32(int32(checkVar.val)))
						extracheckVarsCount++
					} else {
						return nil, packStepError(name, "unable to pack line!!!\nInvalid property value: extracheck_var=%v", line.Value)
					}
				}
			}
		}

		if len(name) > 0 {
			pd.P1(250)
			pd.PJStr(name)
		}
		pd.Next()
	}
	return pd, nil
}
