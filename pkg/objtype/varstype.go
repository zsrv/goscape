package objtype

import (
	"fmt"
	"path/filepath"

	packet2 "github.com/zsrv/goscape/pkg/io/packet"
)

type VarSharedType struct {
	ConfigType
	Type ScriptVarType
}

func (v *VarSharedType) Decode(code uint8, dat *packet2.Packet) error {
	switch code {
	case 1:
		v.Type = ScriptVarType(dat.G1())
	case 250:
		v.DebugName = dat.GJStrLF()
	default:
		return fmt.Errorf("unrecognized vars config code %d", code)
	}
	return nil
}

func NewVarSharedType(id int) *VarSharedType {
	return &VarSharedType{
		ConfigType: ConfigType{ID: id},
		Type:       ScriptVarTypeInt,
	}
}

type VarsTypeConfigs struct {
	ConfigNames map[string]int
	Configs     []*VarSharedType
}

func LoadVarsTypes(dir string) (*VarsTypeConfigs, error) {
	server, err := packet2.Load(filepath.Join(dir, "server", "vars.dat"), false)
	if err != nil {
		return nil, err
	}
	return parseVarsTypes(server)
}

func parseVarsTypes(server *packet2.Packet) (*VarsTypeConfigs, error) {
	count := int(server.G2())

	configs := make([]*VarSharedType, count)
	configNames := make(map[string]int, count)

	for id := range count {
		config := NewVarSharedType(id)
		if err := DecodeType(server, config); err != nil {
			return nil, err
		}
		configs[id] = config
		if config.DebugName != "" {
			configNames[config.DebugName] = id
		}
	}

	return &VarsTypeConfigs{
		ConfigNames: configNames,
		Configs:     configs,
	}, nil
}
