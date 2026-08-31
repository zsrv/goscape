package objtype

import (
	"fmt"
	"path/filepath"

	packet2 "github.com/zsrv/goscape/pkg/io/packet"
)

type ParamMap map[uint32]any

func DecodeParams(dat *packet2.Packet) ParamMap {
	count := dat.G1()
	params := make(ParamMap, count)
	for range count {
		key := dat.G3()
		isString := dat.GBool()
		if isString {
			params[key] = dat.GJStrLF()
		} else {
			params[key] = dat.G4()
		}
	}
	return params
}

type ParamTypeConfigs struct {
	ConfigNames map[string]int
	Configs     []*ParamType
}

// LoadParams loads ParamType configs from the given cache dir. Alias retained
// for the historical LoadParamTypes name.
func LoadParams(dir string) (*ParamTypeConfigs, error) {
	return LoadParamTypes(dir)
}

func LoadParamTypes(dir string) (*ParamTypeConfigs, error) {
	dat, err := packet2.Load(filepath.Join(dir, "server", "param.dat"), false)
	if err != nil {
		return nil, err
	}

	ptc, err := parseParamTypes(dat)
	if err != nil {
		return nil, err
	}

	return ptc, nil
}

func parseParamTypes(dat *packet2.Packet) (*ParamTypeConfigs, error) {
	count := int(dat.G2())

	configs := make([]*ParamType, count)
	configNames := make(map[string]int, count)

	for id := range count {
		config := NewParamType(id)
		if err := DecodeType(dat, config); err != nil {
			return nil, err
		}
		configs[id] = config

		if config.DebugName != "" {
			configNames[config.DebugName] = id
		}
	}

	ptc := &ParamTypeConfigs{
		ConfigNames: configNames,
		Configs:     configs,
	}

	return ptc, nil
}

type ParamType struct {
	ConfigType
	Type          ScriptVarType
	DefaultInt    int32
	AutoDisable   bool
	DefaultString string
}

func (pt *ParamType) Decode(code uint8, dat *packet2.Packet) error {
	switch code {
	case 1:
		pt.Type = ScriptVarType(dat.G1())
	case 2:
		pt.DefaultInt = int32(dat.G4())
	case 4:
		pt.AutoDisable = false
	case 5:
		pt.DefaultString = dat.GJStrLF()
	case 250:
		pt.DebugName = dat.GJStrLF()
	default:
		return fmt.Errorf("unrecognized param config code %d", code)
	}

	return nil
}

func (pt *ParamType) GetType() string {
	switch pt.Type {
	case ScriptVarTypeInt:
		return "int"
	case ScriptVarTypeString:
		return "string"
	case ScriptVarTypeEnum:
		return "enum"
	case ScriptVarTypeObj:
		return "obj"
	case ScriptVarTypeLoc:
		return "loc"
	case ScriptVarTypeComponent:
		return "component"
	case ScriptVarTypeNamedObj:
		return "namedobj"
	case ScriptVarTypeStruct:
		return "struct"
	case ScriptVarTypeBoolean:
		return "boolean"
	case ScriptVarTypeCoord:
		return "coord"
	case ScriptVarTypeCategory:
		return "category"
	case ScriptVarTypeSpotanim:
		return "spotanim"
	case ScriptVarTypeNPC:
		return "npc"
	case ScriptVarTypeInv:
		return "inv"
	case ScriptVarTypeSynth:
		return "synth"
	case ScriptVarTypeSeq:
		return "seq"
	case ScriptVarTypeStat:
		return "stat"
	case ScriptVarTypeInterface:
		return "interface"
	case ScriptVarTypeMidi:
		// TS ParamType.getType case ScriptVarType.MIDI → 'midi'
		// (ParamType.ts:120-121 @2e3bcf43, added at the 254 pin).
		return "midi"
	default:
		return "unknown"
	}
}

// NewParamType allocates a ParamType slot. AutoDisable defaults to
// true per TS src/cache/config/ParamType.ts:64 (NAI-194 fix — goscape
// previously omitted the field and inherited Go zero false, silently
// diverging from TS for params that emit no opcode-4).
func NewParamType(id int) *ParamType {
	return &ParamType{
		ID:          id,
		AutoDisable: true,
		// M21: TS ParamType.defaultInt defaults to -1 (ParamType.ts:62), returned
		// by the `default` getter when no opcode-1 sets it. goscape inherited Go's
		// zero (0), diverging for params that emit no opcode-1.
		DefaultInt: -1,
	}
}
