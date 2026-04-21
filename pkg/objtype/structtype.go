package objtype

import (
	"fmt"
	"path/filepath"

	packet2 "github.com/zsrv/goscape/pkg/io/packet"
)

// StructType is the server-side cache config for a struct (a bag of params).
// Only Params and DebugName are decoded; there are no other fields in the
// wire format (see StructType.ts).
type StructType struct {
	ConfigType
	Params ParamMap
}

func (st *StructType) Decode(code uint8, dat *packet2.Packet) error {
	switch code {
	case 249:
		st.Params = DecodeParams(dat)
	case 250:
		st.DebugName = dat.GJStrLF()
	default:
		return fmt.Errorf("unrecognized struct config code %d", code)
	}
	return nil
}

func NewStructType(id int) *StructType {
	return &StructType{
		ConfigType: ConfigType{ID: id},
		Params:     make(ParamMap),
	}
}

type StructTypeConfigs struct {
	ConfigNames map[string]int
	Configs     []*StructType
}

func LoadStructTypes(dir string) (*StructTypeConfigs, error) {
	server, err := packet2.Load(filepath.Join(dir, "server", "struct.dat"), false)
	if err != nil {
		return nil, err
	}
	return parseStructTypes(server)
}

func parseStructTypes(server *packet2.Packet) (*StructTypeConfigs, error) {
	count := int(server.G2())

	configs := make([]*StructType, count)
	configNames := make(map[string]int, count)

	for id := range count {
		config := NewStructType(id)
		if err := DecodeType(server, config); err != nil {
			return nil, err
		}
		configs[id] = config
		if config.DebugName != "" {
			configNames[config.DebugName] = id
		}
	}

	return &StructTypeConfigs{
		ConfigNames: configNames,
		Configs:     configs,
	}, nil
}
