package objtype

import (
	"fmt"
	"path/filepath"

	packet2 "github.com/zsrv/goscape/pkg/io/packet"
)

const (
	VarpScopeTemp = 0
	VarpScopePerm = 1
)

type VarPlayerType struct {
	ConfigType
	Scope      int
	Type       ScriptVarType
	Protect    bool
	ClientCode uint16
	Transmit   bool
}

func (v *VarPlayerType) Decode(code uint8, dat *packet2.Packet) error {
	switch code {
	case 1:
		v.Scope = int(dat.G1())
	case 2:
		v.Type = ScriptVarType(dat.G1())
	case 4:
		v.Protect = false
	case 5:
		v.ClientCode = uint16(dat.G2())
	case 6:
		v.Transmit = true
	case 250:
		v.DebugName = dat.GJStrLF()
	default:
		return fmt.Errorf("unrecognized varp config code %d", code)
	}
	return nil
}

func NewVarPlayerType(id int) *VarPlayerType {
	return &VarPlayerType{
		ConfigType: ConfigType{ID: id},
		Scope:      VarpScopeTemp,
		Type:       ScriptVarTypeInt,
		Protect:    true,
	}
}

type VarpTypeConfigs struct {
	ConfigNames map[string]int
	Configs     []*VarPlayerType
}

func LoadVarpTypes(dir string) (*VarpTypeConfigs, error) {
	server, err := packet2.Load(filepath.Join(dir, "server", "varp.dat"), false)
	if err != nil {
		return nil, err
	}
	return parseVarpTypes(server)
}

func parseVarpTypes(server *packet2.Packet) (*VarpTypeConfigs, error) {
	count := int(server.G2())

	configs := make([]*VarPlayerType, count)
	configNames := make(map[string]int, count)

	for id := range count {
		config := NewVarPlayerType(id)
		if err := DecodeType(server, config); err != nil {
			return nil, err
		}
		configs[id] = config
		if config.DebugName != "" {
			configNames[config.DebugName] = id
		}
	}

	return &VarpTypeConfigs{
		ConfigNames: configNames,
		Configs:     configs,
	}, nil
}
