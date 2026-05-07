package objtype

import (
	"fmt"
	"path/filepath"

	packet2 "github.com/zsrv/goscape/pkg/io/packet"
)

type VarNpcType struct {
	ConfigType
	Type ScriptVarType
}

func (v *VarNpcType) Decode(code uint8, dat *packet2.Packet) error {
	switch code {
	case 1:
		v.Type = ScriptVarType(dat.G1())
	case 250:
		v.DebugName = dat.GJStrLF()
	default:
		return fmt.Errorf("unrecognized varn config code %d", code)
	}
	return nil
}

func NewVarNpcType(id int) *VarNpcType {
	return &VarNpcType{
		ConfigType: ConfigType{ID: id},
		Type:       ScriptVarTypeInt,
	}
}

type VarnTypeConfigs struct {
	ConfigNames map[string]int
	Configs     []*VarNpcType
}

func LoadVarnTypes(dir string) (*VarnTypeConfigs, error) {
	server, err := packet2.Load(filepath.Join(dir, "server", "varn.dat"), false)
	if err != nil {
		return nil, err
	}
	return parseVarnTypes(server)
}

func parseVarnTypes(server *packet2.Packet) (*VarnTypeConfigs, error) {
	count := int(server.G2())

	configs := make([]*VarNpcType, count)
	configNames := make(map[string]int, count)

	for id := range count {
		config := NewVarNpcType(id)
		if err := DecodeType(server, config); err != nil {
			return nil, err
		}
		configs[id] = config
		if config.DebugName != "" {
			configNames[config.DebugName] = id
		}
	}

	return &VarnTypeConfigs{
		ConfigNames: configNames,
		Configs:     configs,
	}, nil
}
