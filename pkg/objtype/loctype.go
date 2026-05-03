package objtype

import (
	"fmt"
	"path/filepath"

	jagfile "github.com/zsrv/goscape/pkg/io/jagfile"
	packet2 "github.com/zsrv/goscape/pkg/io/packet"
)

// LocType mirrors Engine-TS/src/cache/config/LocType.ts. Loaded via a
// dual-pass decode: server/loc.dat contributes codes 61/249/250 (category,
// params, debugname), and the client jagfile entry loc.dat contributes
// the render+gameplay fields (codes 1-73). PostDecode infers Active from
// Shapes/Op when the cache leaves it unset.
//
// "hidden" → "" coercion in code 30-34 (NAI-80-D1) is preserved from S6k
// for handler-gate simplicity; see follow-up note in spec §6.
type LocType struct {
	ConfigType

	// Client-side render + gameplay fields (codes 1-73)
	Models        []uint16 // code 1, paired with Shapes
	Shapes        []uint8  // code 1, paired with Models
	Name          string   // code 2
	Desc          string   // code 3
	Width         int      // code 14, default 1
	Length        int      // code 15, default 1
	BlockWalk     bool     // code 17 sets false; default true
	BlockRange    bool     // code 18 sets false; default true
	Active        int      // code 19; default -1, PostDecode coerces to 0/1
	HillSkew      bool     // code 21
	ShareLight    bool     // code 22
	Occlude       bool     // code 23
	Anim          int      // code 24, 65535 → -1; default -1
	HasAlpha      bool     // code 25
	WallWidth     int      // code 28; default 16
	Ambient       int8     // code 29 (G1B)
	Contrast      int8     // code 39 (G1B)
	Op            []string // codes 30-34, lazy 5-slot init; "hidden"→"" (D1)
	RecolS        []uint16 // code 40, paired with RecolD
	RecolD        []uint16 // code 40
	MapFunction   int      // code 60; default -1
	Mirror        bool     // code 62
	Shadow        bool     // code 64 sets false; default true
	ResizeX       int      // code 65; default 128
	ResizeY       int      // code 66; default 128
	ResizeZ       int      // code 67; default 128
	MapScene      int      // code 68; default -1
	ForceApproach int      // code 69
	OffsetX       int16    // code 70 (G2S)
	OffsetY       int16    // code 71 (G2S)
	OffsetZ       int16    // code 72 (G2S)
	ForceDecor    bool     // code 73

	// Server-side fields
	Category int      // code 61
	Params   ParamMap // code 249
}

func (lt *LocType) Decode(code uint8, dat *packet2.Packet) error {
	switch code {
	case 1:
		count := int(dat.G1())
		lt.Models = make([]uint16, count)
		lt.Shapes = make([]uint8, count)
		for i := range count {
			lt.Models[i] = dat.G2()
			lt.Shapes[i] = dat.G1()
		}
	case 2:
		lt.Name = dat.GJStrLF()
	case 3:
		lt.Desc = dat.GJStrLF()
	case 14:
		lt.Width = int(dat.G1())
	case 15:
		lt.Length = int(dat.G1())
	case 17:
		lt.BlockWalk = false
	case 18:
		lt.BlockRange = false
	case 19:
		lt.Active = int(dat.G1())
	case 21:
		lt.HillSkew = true
	case 22:
		lt.ShareLight = true
	case 23:
		lt.Occlude = true
	case 24:
		lt.Anim = int(dat.G2())
		if lt.Anim == 65535 {
			lt.Anim = -1
		}
	case 25:
		lt.HasAlpha = true
	case 28:
		lt.WallWidth = int(dat.G1())
	case 29:
		lt.Ambient = dat.G1B()
	case 30, 31, 32, 33, 34:
		// Op-name slots. Lazy 5-slot init mirrors NpcType.Op
		// (npctype.go:124-132). TS LocType.ts:152-157 uses
		// `code >= 30 && < 35`. The "hidden" keyword in the cache
		// marks a disabled op slot; we coerce to "" here so the
		// handler gate in modules/world/handler_oploc.go can do a
		// single empty-string check at runtime (NAI-80-D1).
		if lt.Op == nil {
			lt.Op = make([]string, 5)
		}
		lt.Op[code-30] = dat.GJStrLF()
		if lt.Op[code-30] == "hidden" {
			lt.Op[code-30] = ""
		}
	case 39:
		lt.Contrast = dat.G1B()
	case 40:
		count := int(dat.G1())
		lt.RecolS = make([]uint16, count)
		lt.RecolD = make([]uint16, count)
		for i := range count {
			lt.RecolS[i] = dat.G2()
			lt.RecolD[i] = dat.G2()
		}
	case 60:
		lt.MapFunction = int(dat.G2())
	case 61:
		lt.Category = int(dat.G2())
	case 62:
		lt.Mirror = true
	case 64:
		lt.Shadow = false
	case 65:
		lt.ResizeX = int(dat.G2())
	case 66:
		lt.ResizeY = int(dat.G2())
	case 67:
		lt.ResizeZ = int(dat.G2())
	case 68:
		lt.MapScene = int(dat.G2())
	case 69:
		lt.ForceApproach = int(dat.G1())
	case 70:
		lt.OffsetX = dat.G2S()
	case 71:
		lt.OffsetY = dat.G2S()
	case 72:
		lt.OffsetZ = dat.G2S()
	case 73:
		lt.ForceDecor = true
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
		ConfigType:  ConfigType{ID: id},
		Width:       1,
		Length:      1,
		BlockWalk:   true,
		BlockRange:  true,
		Active:      -1,
		Anim:        -1,
		WallWidth:   16,
		Shadow:      true,
		ResizeX:     128,
		ResizeY:     128,
		ResizeZ:     128,
		MapFunction: -1,
		MapScene:    -1,
		Category:    -1,
		Params:      make(ParamMap),
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
	clientJag, err := jagfile.LoadJagfile(filepath.Join(dir, "client", "config"))
	if err != nil {
		return nil, err
	}
	return parseLocTypes(server, clientJag)
}

func parseLocTypes(server *packet2.Packet, clientJag *jagfile.Jagfile) (*LocTypeConfigs, error) {
	count := int(server.G2())

	client, err := clientJag.Read("loc.dat")
	if err != nil {
		return nil, fmt.Errorf("client/config loc.dat: %w", err)
	}
	client.Pos = 2 // skip the 2-byte count header on the client side

	configs := make([]*LocType, count)
	configNames := make(map[string]int, count)

	for id := range count {
		config := NewLocType(id)
		if err := DecodeType(server, config); err != nil {
			return nil, fmt.Errorf("loc id %d (server): %w", id, err)
		}
		if err := DecodeType(client, config); err != nil {
			return nil, fmt.Errorf("loc id %d (client): %w", id, err)
		}
		// PostDecode wired in T4; struct method does not yet exist.
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
