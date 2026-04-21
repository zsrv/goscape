package objtype

import (
	"fmt"
	"path/filepath"

	packet2 "github.com/zsrv/goscape/pkg/io/packet"
)

// LocType is the server-side subset of a cache loc (scenery/door/etc.). The
// full TS LocType decodes many more fields from the client jagfile (models,
// shapes, recolours, resizes, etc.) which this server-only loader skips.
//
// server/loc.dat in the real cache contains only codes 61 (category), 249
// (params), and 250 (debugname); Desc/Width/Length are defined here so the
// LC_* handlers have a place to read from even if the packer never writes
// them to the server blob.
type LocType struct {
	ConfigType
	Category int
	Desc     string
	Width    int
	Length   int
	Params   ParamMap
}

func (lt *LocType) Decode(code uint8, dat *packet2.Packet) error {
	switch code {
	case 3:
		lt.Desc = dat.GJStrLF()
	case 14:
		lt.Width = int(dat.G1())
	case 15:
		lt.Length = int(dat.G1())
	case 61:
		lt.Category = int(dat.G2())
	case 249:
		lt.Params = DecodeParams(dat)
	case 250:
		lt.DebugName = dat.GJStrLF()
	default:
		return fmt.Errorf("unrecognized loc config code %d", code)
	}
	return nil
}

func NewLocType(id int) *LocType {
	return &LocType{
		ConfigType: ConfigType{ID: id},
		Category:   -1,
		Width:      1,
		Length:     1,
		Params:     make(ParamMap),
	}
}

type LocTypeConfigs struct {
	ConfigNames map[string]int
	Configs     []*LocType
}

func LoadLocTypes(dir string) (*LocTypeConfigs, error) {
	server, err := packet2.Load(filepath.Join(dir, "server", "loc.dat"), false)
	if err != nil {
		return nil, err
	}
	return parseLocTypes(server)
}

func parseLocTypes(server *packet2.Packet) (*LocTypeConfigs, error) {
	count := int(server.G2())

	configs := make([]*LocType, count)
	configNames := make(map[string]int, count)

	for id := range count {
		config := NewLocType(id)
		if err := DecodeType(server, config); err != nil {
			return nil, fmt.Errorf("loc id %d: %w", id, err)
		}
		configs[id] = config
		if config.DebugName != "" {
			configNames[config.DebugName] = id
		}
	}

	return &LocTypeConfigs{
		ConfigNames: configNames,
		Configs:     configs,
	}, nil
}
