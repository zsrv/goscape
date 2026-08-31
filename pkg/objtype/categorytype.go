package objtype

import (
	"errors"
	"log/slog"
	"os"
	"path/filepath"

	packet2 "github.com/zsrv/goscape/pkg/io/packet"
)

// CategoryType mirrors TS Engine-TS/src/cache/config/CategoryType.ts —
// a "virtual" config type carrying only a debugname. Used by
// NPC_FINDCAT (NpcOps.ts:373) and INV_TOTALCAT (InvOps.ts:638) for
// CategoryTypeValid (ScriptValidators.ts:123) bound validation.
type CategoryType struct {
	ConfigType
}

// Decode handles the single TS field at CategoryType.ts:62-66
// (code=1 → debugname). Any other code is warn-logged and ignored
// (mirrors sibling varntype.go).
func (c *CategoryType) Decode(code uint8, dat *packet2.Packet) error {
	switch code {
	case 1:
		c.DebugName = dat.GJStrLF()
	default:
		slog.Warn("objtype: unrecognized category config code", "code", code)
	}
	return nil
}

func NewCategoryType(id int) *CategoryType {
	return &CategoryType{ID: id}
}

type CategoryTypeConfigs struct {
	ConfigNames map[string]int
	Configs     []*CategoryType
}

// LoadCategoryTypes mirrors TS CategoryType.load (CategoryType.ts:12-19).
// Missing data/pack/server/category.dat returns an empty registry
// (TS guards with fs.existsSync; goscape sibling precedent: fonttype.Load
// at modules/world/server.go:537-545). Other I/O / decode errors propagate.
func LoadCategoryTypes(dir string) (*CategoryTypeConfigs, error) {
	server, err := packet2.Load(filepath.Join(dir, "server", "category.dat"), false)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			slog.Warn("objtype: category.dat missing; CategoryType registry empty",
				"dir", dir)
			return &CategoryTypeConfigs{
				ConfigNames: map[string]int{},
				Configs:     nil,
			}, nil
		}
		return nil, err
	}
	return parseCategoryTypes(server)
}

func parseCategoryTypes(server *packet2.Packet) (*CategoryTypeConfigs, error) {
	count := int(server.G2())
	configs := make([]*CategoryType, count)
	configNames := make(map[string]int, count)
	for id := range count {
		config := NewCategoryType(id)
		if err := DecodeType(server, config); err != nil {
			return nil, err
		}
		configs[id] = config
		if config.DebugName != "" {
			configNames[config.DebugName] = id
		}
	}
	return &CategoryTypeConfigs{
		ConfigNames: configNames,
		Configs:     configs,
	}, nil
}
