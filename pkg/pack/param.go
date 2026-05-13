package pack

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/zsrv/goscape/pkg/coordgrid"
	"github.com/zsrv/goscape/pkg/objtype"
)

// parseParamConfig is the per-key=value parser for .param config blocks.
//
// Accepted keys:
//   - autodisable  (boolean; yes/no/true/false/1/0)
//   - type         (ScriptVarType name → ScriptVarType code)
//   - default      (raw string; resolution deferred to packParamConfigs
//                   after `type` is known — mirrors TS comment
//                   "defer lookup to pack callback")
//
// Return contract (matches NAI-192 ParseFn):
//   - (value, true, nil)  → accepted
//   - (nil, true, err)    → recognized key with invalid value
//   - (nil, false, nil)   → unrecognized key
//
// TS source: tools/pack/config/ParamConfig.ts parseParamConfig (163-214).
func parseParamConfig(key, value string) (ConfigValue, bool, error) {
	switch key {
	case "autodisable":
		if !IsConfigBoolean(value) {
			return nil, true, fmt.Errorf("invalid boolean: %s", value)
		}
		return GetConfigBoolean(value), true, nil
	case "type":
		t, ok := objtype.ScriptVarTypeFromName(value)
		if !ok {
			return nil, true, fmt.Errorf("unknown script var type: %s", value)
		}
		return t, true, nil
	case "default":
		return value, true, nil
	}
	return nil, false, nil
}

// paramLookups bundles every *PackFile that lookupParamValue may need
// to resolve a typed-id default. Constructed once per PackConfigs call
// via loadParamLookups (only when .param source is present).
//
// NAI-194-D-PACKFILE-SINGLETONS-DEFERRED: TS uses module-level *Pack
// singletons; goscape threads pointers explicitly.
type paramLookups struct {
	enumPF      *PackFile
	objPF       *PackFile
	locPF       *PackFile
	interfacePF *PackFile
	structPF    *PackFile
	categoryPF  *PackFile
	spotanimPF  *PackFile
	npcPF       *PackFile
	invPF       *PackFile
	synthPF     *PackFile
	seqPF       *PackFile
	varpPF      *PackFile
	dbrowPF     *PackFile
}

// paramStats / paramNpcStats are TS-hardcoded ordered lists from
// tools/pack/config/ParamConfig.ts:5-29. The slice index becomes the
// packed DefaultInt. Order is load-bearing and must stay synced.
var paramStats = []string{
	"attack", "defence", "strength", "hitpoints", "ranged", "prayer",
	"magic", "cooking", "woodcutting", "fletching", "fishing", "firemaking",
	"crafting", "smithing", "mining", "herblore", "agility", "thieving",
	"slayer", "farming", "runecraft",
}

var paramNpcStats = []string{
	"hitpoints", "attack", "strength", "defence", "magic", "ranged",
}

// lookupParamValue resolves a raw `default=` value against a
// ScriptVarType. Returns the resolved scalar (int for indexed/primitive
// types, string for STRING) or an error. The "null" string is a
// sentinel: returns -1 for non-STRING types and "" for STRING.
//
// TS source: tools/pack/config/ParamConfig.ts lookupParamValue
// (31-161). 20 arms over ScriptVarType + 1 null-sentinel early-return.
// NAMEDOBJ and OBJ share an arm. COMPONENT routes through interfacePF.
// INTERFACE rejects values containing ':' before the pack lookup.
func lookupParamValue(typ objtype.ScriptVarType, value string, lk *paramLookups) (any, error) {
	if value == "null" {
		if typ == objtype.ScriptVarTypeString {
			return "", nil
		}
		return int(-1), nil
	}

	switch typ {
	case objtype.ScriptVarTypeInt:
		n, err := strconv.ParseInt(value, 0, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid int default %q", value)
		}
		return int(n), nil

	case objtype.ScriptVarTypeString:
		if len(value) > 1000 {
			return nil, fmt.Errorf("string default exceeds 1000 chars")
		}
		return value, nil

	case objtype.ScriptVarTypeBoolean:
		if !IsConfigBoolean(value) {
			return nil, fmt.Errorf("invalid boolean default %q", value)
		}
		if GetConfigBoolean(value) {
			return int(1), nil
		}
		return int(0), nil

	case objtype.ScriptVarTypeCoord:
		return parseParamCoord(value)

	case objtype.ScriptVarTypeEnum:
		return paramIndexOrErr(lk.enumPF, value, "enum")
	case objtype.ScriptVarTypeNamedObj, objtype.ScriptVarTypeObj:
		return paramIndexOrErr(lk.objPF, value, "obj")
	case objtype.ScriptVarTypeLoc:
		return paramIndexOrErr(lk.locPF, value, "loc")
	case objtype.ScriptVarTypeComponent:
		// COMPONENT IDs index into InterfacePack (no dedicated component
		// pack); TS routes COMPONENT through InterfacePack.getByName.
		return paramIndexOrErr(lk.interfacePF, value, "component")
	case objtype.ScriptVarTypeStruct:
		return paramIndexOrErr(lk.structPF, value, "struct")
	case objtype.ScriptVarTypeCategory:
		return paramIndexOrErr(lk.categoryPF, value, "category")
	case objtype.ScriptVarTypeSpotanim:
		return paramIndexOrErr(lk.spotanimPF, value, "spotanim")
	case objtype.ScriptVarTypeNPC:
		return paramIndexOrErr(lk.npcPF, value, "npc")
	case objtype.ScriptVarTypeInv:
		return paramIndexOrErr(lk.invPF, value, "inv")
	case objtype.ScriptVarTypeSynth:
		return paramIndexOrErr(lk.synthPF, value, "synth")
	case objtype.ScriptVarTypeSeq:
		return paramIndexOrErr(lk.seqPF, value, "seq")
	case objtype.ScriptVarTypeVarp:
		return paramIndexOrErr(lk.varpPF, value, "varp")
	case objtype.ScriptVarTypeDbrow:
		return paramIndexOrErr(lk.dbrowPF, value, "dbrow")

	case objtype.ScriptVarTypeStat:
		i := slices.Index(paramStats, value)
		if i < 0 {
			return nil, fmt.Errorf("unknown stat %q", value)
		}
		return i, nil

	case objtype.ScriptVarTypeNpcStat:
		i := slices.Index(paramNpcStats, value)
		if i < 0 {
			return nil, fmt.Errorf("unknown npc_stat %q", value)
		}
		return i, nil

	case objtype.ScriptVarTypeInterface:
		// The colon notation is parent:component path syntax that cannot
		// resolve to a single interface ID, hence rejected.
		if strings.Contains(value, ":") {
			return nil, fmt.Errorf("interface default may not contain ':': %q", value)
		}
		return paramIndexOrErr(lk.interfacePF, value, "interface")
	}

	return nil, fmt.Errorf("unsupported default ScriptVarType %d", typ)
}

// paramIndexOrErr resolves `value` against pf. Returns the id, or an
// error if pf is nil (not loaded) or name is unknown.
//
// goscape stricter than TS: TS crashes on undefined.getByName when the
// *Pack singleton hasn't been initialized; goscape returns a typed
// error so the failure mode is named.
func paramIndexOrErr(pf *PackFile, value, kind string) (int, error) {
	if pf == nil {
		return 0, fmt.Errorf("%s pack not loaded", kind)
	}
	i := pf.GetByName(value)
	if i < 0 {
		return 0, fmt.Errorf("unknown %s %q", kind, value)
	}
	return i, nil
}

// packParamConfigs walks every id ∈ [0, pf.Max), pre-scans for the
// `type` key (needed before `default` can resolve via lookupParamValue),
// then emits per-config opcodes on the server buffer:
//
//	type        → P1(1) P1(typechar)
//	default     → P1(2) P4(int)        for non-STRING
//	               P1(5) PJStr(value)   for STRING
//	autodisable → P1(4)                 only when value is false
//	debugname   → P1(250) PJStr(name)   when slot has a name
//
// The client buffer is initialized but never written between Next()
// calls — TS-faithful per NAI-194-D-PARAM-EMPTY-CLIENT-FAITHFUL.
//
// Returns (server, client, err). err propagates from missing-type
// assertion or from lookupParamValue's default-value resolution.
//
// TS source: tools/pack/config/ParamConfig.ts:216-265. TS uses `!`
// non-null assertion on the type-find; goscape returns an explicit
// error to name the failure mode.
func packParamConfigs(configs map[string][]ConfigLine, pf *PackFile, lk *paramLookups) (server, client *PackedData, err error) {
	server = NewPackedData(pf.Max)
	client = NewPackedData(pf.Max)

	for id := range pf.Max {
		name := pf.GetByID(id)
		if cfg, ok := configs[name]; ok {
			var typ objtype.ScriptVarType
			typFound := false
			for _, line := range cfg {
				if line.Key == "type" {
					typ = line.Value.(objtype.ScriptVarType)
					typFound = true
					break
				}
			}
			if !typFound {
				return nil, nil, fmt.Errorf("param %q missing type", name)
			}

			for _, line := range cfg {
				switch line.Key {
				case "type":
					server.P1(1)
					server.P1(uint8(typ))
				case "default":
					raw := line.Value.(string)
					resolved, lookupErr := lookupParamValue(typ, raw, lk)
					if lookupErr != nil {
						return nil, nil, fmt.Errorf("param %q default: %w", name, lookupErr)
					}
					if typ == objtype.ScriptVarTypeString {
						server.P1(5)
						server.PJStr(resolved.(string))
					} else {
						server.P1(2)
						server.P4(uint32(resolved.(int)))
					}
				case "autodisable":
					if !line.Value.(bool) {
						server.P1(4)
					}
				}
			}
		}
		if len(name) > 0 {
			server.P1(250)
			server.PJStr(name)
		}
		server.Next()
		client.Next()
	}
	return server, client, nil
}

// parseParamCoord splits `level_mX_mZ_lX_lZ` and packs via
// coordgrid.PackCoord. Bounds: level ∈ [0,3], mX/mZ ∈ [0,255],
// lX/lZ ∈ [0,63]. All parts must be non-negative integers.
//
// TS source: ParamConfig.ts:77-104.
func parseParamCoord(value string) (int, error) {
	parts := strings.Split(value, "_")
	if len(parts) != 5 {
		return 0, fmt.Errorf("coord must be 5 parts (level_mX_mZ_lX_lZ): %q", value)
	}
	level, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, fmt.Errorf("coord level: %w", err)
	}
	mX, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, fmt.Errorf("coord mX: %w", err)
	}
	mZ, err := strconv.Atoi(parts[2])
	if err != nil {
		return 0, fmt.Errorf("coord mZ: %w", err)
	}
	lX, err := strconv.Atoi(parts[3])
	if err != nil {
		return 0, fmt.Errorf("coord lX: %w", err)
	}
	lZ, err := strconv.Atoi(parts[4])
	if err != nil {
		return 0, fmt.Errorf("coord lZ: %w", err)
	}
	if level < 0 || mX < 0 || mZ < 0 || lX < 0 || lZ < 0 {
		return 0, fmt.Errorf("coord parts must be non-negative: %q", value)
	}
	if level > 3 || mX > 255 || mZ > 255 || lX > 63 || lZ > 63 {
		return 0, fmt.Errorf("coord part out of range (level≤3, m*≤255, l*≤63): %q", value)
	}
	x := mX*64 + lX
	z := mZ*64 + lZ
	return coordgrid.PackCoord(level, x, z), nil
}
