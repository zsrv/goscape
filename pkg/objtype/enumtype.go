package objtype

import (
	"fmt"
	"path/filepath"

	packet2 "github.com/zsrv/goscape/pkg/io/packet"
)

// EnumType is the server-side representation of a cache enum. Its Values map
// is keyed by the input (int32) and the stored value is either int32 (when
// OutputType is int-ish) or string (when OutputType is ScriptVarTypeString).
type EnumType struct {
	ConfigType
	InputType     ScriptVarType
	OutputType    ScriptVarType
	DefaultInt    int32
	DefaultString string
	Values        map[int32]any
}

func (e *EnumType) Decode(code uint8, dat *packet2.Packet) error {
	switch code {
	case 1:
		e.InputType = ScriptVarType(dat.G1())
	case 2:
		e.OutputType = ScriptVarType(dat.G1())
	case 3:
		e.DefaultString = dat.GJStrLF()
	case 4:
		e.DefaultInt = int32(dat.G4())
	case 5:
		count := int(dat.G2())
		for range count {
			key := int32(dat.G4())
			e.Values[key] = dat.GJStrLF()
		}
	case 6:
		count := int(dat.G2())
		for range count {
			key := int32(dat.G4())
			e.Values[key] = int32(dat.G4())
		}
	case 250:
		e.DebugName = dat.GJStrLF()
	default:
		return fmt.Errorf("unrecognized enum config code %d", code)
	}
	return nil
}

func NewEnumType(id int) *EnumType {
	return &EnumType{
		ConfigType:    ConfigType{ID: id},
		InputType:     ScriptVarTypeInt,
		OutputType:    ScriptVarTypeInt,
		DefaultInt:    0,
		DefaultString: "null",
		Values:        make(map[int32]any),
	}
}

type EnumTypeConfigs struct {
	ConfigNames map[string]int
	Configs     []*EnumType
}

func LoadEnumTypes(dir string) (*EnumTypeConfigs, error) {
	server, err := packet2.Load(filepath.Join(dir, "server", "enum.dat"), false)
	if err != nil {
		return nil, err
	}
	return parseEnumTypes(server)
}

func parseEnumTypes(server *packet2.Packet) (*EnumTypeConfigs, error) {
	count := int(server.G2())

	configs := make([]*EnumType, count)
	configNames := make(map[string]int, count)

	for id := range count {
		config := NewEnumType(id)
		if err := DecodeType(server, config); err != nil {
			return nil, err
		}
		configs[id] = config
		if config.DebugName != "" {
			configNames[config.DebugName] = id
		}
	}

	return &EnumTypeConfigs{
		ConfigNames: configNames,
		Configs:     configs,
	}, nil
}
