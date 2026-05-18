package objtype

import (
	"fmt"
	"path/filepath"

	packet2 "github.com/zsrv/goscape/pkg/io/packet"
)

// FloType is a minimal binary view of a floor-type entry. The full
// TS FloType has many more fields; goscape's worldmap packer only
// needs the debugname → id mapping and the total count, so we keep
// this lean.
type FloType struct {
	Id        int
	DebugName string
}

type FloTypeConfigs struct {
	Configs     []*FloType
	ConfigNames map[string]int
}

// GetId returns the numeric id for debugname, or -1 if unknown.
// Mirrors TS FloType.getId.
func (f *FloTypeConfigs) GetId(debugName string) int {
	if id, ok := f.ConfigNames[debugName]; ok {
		return id
	}
	return -1
}

// LoadFloTypes reads dir/server/flo.dat and returns the minimal
// view used by the worldmap packer.
func LoadFloTypes(dir string) (*FloTypeConfigs, error) {
	dat, err := packet2.Load(filepath.Join(dir, "server", "flo.dat"), false)
	if err != nil {
		return nil, fmt.Errorf("server/flo.dat: %w", err)
	}
	return parseFloTypes(dat)
}

func parseFloTypes(dat *packet2.Packet) (*FloTypeConfigs, error) {
	count := int(dat.G2())
	configs := make([]*FloType, count)
	names := make(map[string]int, count)
	for id := range count {
		ft := &FloType{Id: id}
		for {
			code := dat.G1()
			if code == 0 {
				break
			}
			switch code {
			case 1:
				_ = dat.G3()
			case 2:
				ft.DebugName = dat.GJStrLF()
			case 3:
				_ = dat.GJStrLF()
			case 5:
				_ = dat.G1()
			case 6:
				_ = dat.G2()
			case 7:
				_ = dat.G3()
			case 8:
				_ = dat.G3()
			default:
				return nil, fmt.Errorf("flo id %d: unknown opcode %d", id, code)
			}
		}
		configs[id] = ft
		if ft.DebugName != "" {
			names[ft.DebugName] = id
		}
	}
	return &FloTypeConfigs{Configs: configs, ConfigNames: names}, nil
}
