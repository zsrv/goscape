package objtype

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	io "github.com/zsrv/goscape/pkg/io/jagfile"
	packet2 "github.com/zsrv/goscape/pkg/io/packet"
)

type ObjTypeConfigs struct {
	ConfigNames map[string]int
	Configs     []*ObjType
}

func LoadObjTypes(dir string, ptc *ParamTypeConfigs) (*ObjTypeConfigs, error) {
	server, err := packet2.Load(filepath.Join(dir, "server", "obj.dat"), false)
	if err != nil {
		return nil, err
	}

	jag, err := io.LoadJagfile(filepath.Join(dir, "client", "config"))
	if err != nil {
		return nil, err
	}

	otc, err := parseObjTypes(server, jag, ptc)
	if err != nil {
		return nil, err
	}
	return otc, nil
}

func parseObjTypes(server *packet2.Packet, jag *io.Jagfile, ptc *ParamTypeConfigs) (*ObjTypeConfigs, error) {
	count := int(server.G2())

	otc := &ObjTypeConfigs{
		ConfigNames: make(map[string]int, count),
		Configs:     make([]*ObjType, count),
	}

	client, err := jag.Read("obj.dat")
	if err != nil {
		return nil, err
	}
	client.Pos = 2

	for id := range count {
		config := NewObjType(id)
		if err := DecodeType(server, config); err != nil {
			return nil, err
		}
		if err := DecodeType(client, config); err != nil {
			return nil, err
		}

		otc.Configs[id] = config

		if config.DebugName != "" {
			otc.ConfigNames[config.DebugName] = id
		}
	}

	applyPostDecodeFixups(otc, ptc)

	return otc, nil
}

// ByName returns the ObjType matching the given debugname, or nil if no
// match exists. Mirrors TS ObjType.getByName. Uses the ConfigNames index
// built at load time — O(1) on name-indexed configs, O(N) only if
// ConfigNames is unpopulated.
func (otc *ObjTypeConfigs) ByName(name string) *ObjType {
	if otc == nil {
		return nil
	}
	if id, ok := otc.ConfigNames[name]; ok {
		if id >= 0 && id < len(otc.Configs) {
			return otc.Configs[id]
		}
	}
	// Fallback linear scan for configs loaded without a name index.
	for _, c := range otc.Configs {
		if c != nil && c.DebugName == name {
			return c
		}
	}
	return nil
}

func applyPostDecodeFixups(otc *ObjTypeConfigs, ptc *ParamTypeConfigs) {
	for id := range otc.Configs {
		config := otc.Configs[id]

		if config.CertTemplate != -1 {
			config.toCertificate(otc)
		}

		if config.DummyItem != 0 {
			config.Tradeable = false
		}

		if os.Getenv("NODE_MEMBERS") == "false" && config.Members {
			config.Tradeable = false
			config.Op = []string{"", "", "Take", "", ""}
			config.IOp = []string{"", "", "", "", "Drop"}
			config.Category = -1

			// TS ObjType.ts:73 uses ParamType.get(key)?.autodisable — the
			// optional-chain silently no-ops when the ParamType lookup
			// misses, leaving the param in place. goscape's pre-fix code
			// did a raw ptc.Configs[k] slice index, which panics when k is
			// out-of-range AND nil-derefs when the slot is nil. Mirror the
			// optional-chain by guarding both branches.
			// TS ObjType.ts:73 uses ParamType.get(key)?.autodisable — the
			// optional-chain silently no-ops when the ParamType lookup
			// misses, leaving the param in place. goscape's pre-fix code
			// did a raw ptc.Configs[k] slice index, which panics when k is
			// out-of-range AND nil-derefs when the slot is nil. Mirror the
			// optional-chain by guarding both branches.
			for k := range config.Params {
				if int(k) >= len(ptc.Configs) {
					continue
				}
				pt := ptc.Configs[k]
				if pt == nil {
					continue
				}
				if pt.AutoDisable {
					delete(config.Params, k)
				}
			}
		}
	}
}

type ObjType struct {
	ConfigType
	Model            int
	Name             string
	Desc             string
	RecolS           []uint16
	RecolD           []uint16
	Zoom2D           int
	Xan2D            int
	Yan2D            int
	Zan2D            int
	Xof2D            int
	Yof2D            int
	Code9            bool
	Code10           int
	Stackable        bool
	Cost             int
	Members          bool
	Op               []string
	IOp              []string
	ManWear          int
	ManWear2         int
	ManWearOffsetY   int
	WomanWear        int
	WomanWear2       int
	WomanWearOffsetY int
	ManWear3         int
	WomanWear3       int
	ManHead          int
	ManHead2         int
	WomanHead        int
	WomanHead2       int
	CountObj         []uint16
	CountCo          []uint16
	CertLink         int
	CertTemplate     int

	// server-side
	WearPos     int
	WearPos2    int
	WearPos3    int
	Weight      int // in grams
	Category    int
	DummyItem   int
	Tradeable   bool
	RespawnRate int // defaults to 1 minute
	Params      ParamMap
}

func (ot *ObjType) toCertificate(otc *ObjTypeConfigs) {
	template := otc.Configs[ot.CertTemplate]
	ot.Model = template.Model
	ot.Zoom2D = template.Zoom2D
	ot.Xan2D = template.Xan2D
	ot.Yan2D = template.Yan2D
	ot.Zan2D = template.Zan2D
	ot.Xof2D = template.Xof2D
	ot.Yof2D = template.Yof2D
	ot.RecolS = template.RecolS
	ot.RecolD = template.RecolD

	link := otc.Configs[ot.CertLink]
	ot.Name = link.Name
	ot.Members = link.Members
	ot.Cost = link.Cost
	ot.Tradeable = link.Tradeable

	article := "a"
	c, _ := utf8.DecodeRuneInString(strings.ToLower(link.Name))
	if c == 'a' || c == 'e' || c == 'i' || c == 'o' || c == 'u' {
		article = "an"
	}
	ot.Desc = fmt.Sprintf("Swap this note at any bank for %s %s.", article, link.Name)

	ot.Stackable = true
}

func (ot *ObjType) Decode(code uint8, dat *packet2.Packet) error {
	switch code {
	case 1:
		ot.Model = int(dat.G2())
	case 2:
		ot.Name = dat.GJStrLF()
	case 3:
		ot.Desc = dat.GJStrLF()
	case 4:
		ot.Zoom2D = int(dat.G2())
	case 5:
		ot.Xan2D = int(dat.G2())
	case 6:
		ot.Yan2D = int(dat.G2())
	case 7:
		ot.Xof2D = int(dat.G2S())
	case 8:
		ot.Yof2D = int(dat.G2S())
	case 9:
		ot.Code9 = true
	case 10:
		ot.Code10 = int(dat.G2())
	case 11:
		ot.Stackable = true
	case 12:
		ot.Cost = int(dat.G4())
	case 13:
		ot.WearPos = int(dat.G1())
	case 14:
		ot.WearPos2 = int(dat.G1())
	case 15:
		ot.Tradeable = false
	case 16:
		ot.Members = true
	case 23:
		ot.ManWear = int(dat.G2())
		ot.ManWearOffsetY = int(dat.G1B())
	case 24:
		ot.ManWear2 = int(dat.G2())
	case 25:
		ot.WomanWear = int(dat.G2())
		ot.WomanWearOffsetY = int(dat.G1B())
	case 26:
		ot.WomanWear2 = int(dat.G2())
	case 27:
		ot.WearPos3 = int(dat.G1())
	case 30, 31, 32, 33, 34:
		// Stored verbatim, including "hidden", matching TS ObjType.ts:226-227
		// (no decode-time coercion). The op-click handler blocks null/
		// "hidden", but OC_OP (`op[i] ?? ''`) and P_OPOBJ report it as a
		// present string, so coercing to "" here would diverge from TS.
		ot.Op[code-30] = dat.GJStrLF()
	case 35, 36, 37, 38, 39:
		ot.IOp[code-35] = dat.GJStrLF()
	case 40:
		count := dat.G1()
		ot.RecolS = make([]uint16, count)
		ot.RecolD = make([]uint16, count)

		for i := range count {
			ot.RecolS[i] = dat.G2()
			ot.RecolD[i] = dat.G2()
		}
	case 75:
		ot.Weight = int(dat.G2S())
	case 78:
		ot.ManWear3 = int(dat.G2())
	case 79:
		ot.WomanWear3 = int(dat.G2())
	case 90:
		ot.ManHead = int(dat.G2())
	case 91:
		ot.WomanHead = int(dat.G2())
	case 92:
		ot.ManHead2 = int(dat.G2())
	case 93:
		ot.WomanHead2 = int(dat.G2())
	case 94:
		ot.Category = int(dat.G2())
	case 95:
		ot.Zan2D = int(dat.G2())
	case 96:
		ot.DummyItem = int(dat.G1())
	case 97:
		ot.CertLink = int(dat.G2())
	case 98:
		ot.CertTemplate = int(dat.G2())
	case 100, 101, 102, 103, 104, 105, 106, 107, 108, 109:
		if ot.CountObj == nil || ot.CountCo == nil {
			ot.CountObj = make([]uint16, 10)
			ot.CountCo = make([]uint16, 10)
		}

		ot.CountObj[code-100] = dat.G2()
		ot.CountCo[code-100] = dat.G2()
	case 201:
		ot.RespawnRate = int(dat.G2())
	case 249:
		ot.Params = DecodeParams(dat)
	case 250:
		ot.DebugName = dat.GJStrLF()
	default:
		return fmt.Errorf("unrecognized obj config code %d", code)
	}

	return nil
}

func NewObjType(id int) *ObjType {
	return &ObjType{
		ConfigType: ConfigType{
			ID: id,
		},
		Zoom2D:       2000,
		Code10:       -1,
		Cost:         1,
		ManWear:      -1,
		ManWear2:     -1,
		WomanWear:    -1,
		WomanWear2:   -1,
		ManWear3:     -1,
		WomanWear3:   -1,
		ManHead:      -1,
		ManHead2:     -1,
		WomanHead:    -1,
		WomanHead2:   -1,
		CertLink:     -1,
		CertTemplate: -1,

		WearPos:     -1,
		WearPos2:    -1,
		WearPos3:    -1,
		Category:    -1,
		RespawnRate: 100,  // defaults to 1 minute
		Tradeable:   true, // TS ObjType.ts:177 class-field default
		Op:          []string{"", "", "Take", "", ""},
		IOp:         []string{"", "", "", "", "Drop"},
		Params:      make(ParamMap),
	}
}

func GetWearPosID(name string) int {
	switch name {
	case "hat":
		return 0
	case "back":
		return 1
	case "front":
		return 2
	case "righthand":
		return 3
	case "torso":
		return 4
	case "lefthand":
		return 5
	case "arms":
		return 6
	case "legs":
		return 7
	case "head":
		return 8
	case "hands":
		return 9
	case "feet":
		return 10
	case "jaw":
		return 11
	case "ring":
		return 12
	case "quiver":
		return 13
	default:
		return -1
	}
}
