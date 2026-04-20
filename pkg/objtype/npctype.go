package objtype

import (
	"fmt"
	"path/filepath"

	io "github.com/zsrv/goscape/pkg/io/jagfile"
	packet2 "github.com/zsrv/goscape/pkg/io/packet"
)

// NPC stat indices (attack, defence, strength, hitpoints, ranged, magic).
const (
	npcStatAttack    = 0
	npcStatDefence   = 1
	npcStatStrength  = 2
	npcStatHitpoints = 3
	npcStatRanged    = 4
	npcStatMagic     = 5
)

// MoveRestrict values (mirror of rs-server-225/entity.MoveRestrict).
const (
	MoveRestrictNormal        = 0
	MoveRestrictBlocked       = 1
	MoveRestrictBlockedNormal = 2
	MoveRestrictIndoors       = 3
	MoveRestrictOutdoors      = 4
	MoveRestrictNoMove        = 5
	MoveRestrictPassthru      = 6
)

// BlockWalk values (mirror of rs-server-225/entity.BlockWalk).
const (
	BlockWalkNone = 0
	BlockWalkNPC  = 1
	BlockWalkAll  = 2
)

// NPCMode values (subset of rs-server-225/entity.NPCMode constants relevant
// for config defaults).
const (
	NPCModeNull   = -1
	NPCModeNone   = 0
	NPCModeWander = 1
)

type NpcType struct {
	ConfigType
	Name      string
	Desc      string
	Size      uint8
	Models    []uint16
	Heads     []uint16
	HasAnim   bool
	ReadyAnim int
	WalkAnim  int
	WalkAnimB int
	WalkAnimR int
	WalkAnimL int
	HasAlpha  bool
	RecolS    []uint16
	RecolD    []uint16
	Op        []string
	ResizeX   int
	ResizeY   int
	ResizeZ   int
	Minimap   bool
	VisLevel  int
	ResizeH   uint16
	ResizeV   uint16

	// server-side
	RegenRate    int
	Category     int
	WanderRange  uint16
	MaxRange     uint16
	HuntRange    uint8
	Timer        int
	RespawnRate  uint16
	Stats        []uint16
	MoveRestrict int
	AttackRange  uint16
	HuntMode     int
	DefaultMode  int
	Members      bool
	BlockWalk    int
	Params       ParamMap
	PatrolCoord  []uint32
	PatrolDelay  []uint8
	GiveChase    bool
}

func (t *NpcType) Decode(code uint8, dat *packet2.Packet) error {
	switch code {
	case 1:
		count := dat.G1()
		t.Models = make([]uint16, count)

		for i := range count {
			t.Models[i] = dat.G2()
		}
	case 2:
		t.Name = dat.GJStrLF()
	case 3:
		t.Desc = dat.GJStrLF()
	case 12:
		t.Size = dat.G1()
	case 13:
		t.ReadyAnim = int(dat.G2())
	case 14:
		t.WalkAnim = int(dat.G2())
	case 16:
		t.HasAnim = true
	case 17:
		t.WalkAnim = int(dat.G2())
		t.WalkAnimB = int(dat.G2())
		t.WalkAnimR = int(dat.G2())
		t.WalkAnimL = int(dat.G2())
	case 18:
		t.Category = int(dat.G2())
	case 30, 31, 32, 33, 34, 35, 36, 37, 38, 39:
		if t.Op == nil {
			t.Op = make([]string, 5) // TODO: make []*string so it fills with nil?
		}

		t.Op[code-30] = dat.GJStrLF()
		if t.Op[code-30] == "hidden" {
			t.Op[code-30] = ""
		}
	case 40:
		count := dat.G1()

		t.RecolS = make([]uint16, count)
		t.RecolD = make([]uint16, count)

		for i := range count {
			t.RecolS[i] = dat.G2()
			t.RecolD[i] = dat.G2()
		}
	case 60:
		count := dat.G1()

		t.Heads = make([]uint16, count)

		for i := range count {
			t.Heads[i] = dat.G2()
		}
	case 74:
		t.Stats[npcStatAttack] = dat.G2()
	case 75:
		t.Stats[npcStatDefence] = dat.G2()
	case 76:
		t.Stats[npcStatStrength] = dat.G2()
	case 77:
		t.Stats[npcStatHitpoints] = dat.G2()
	case 78:
		t.Stats[npcStatRanged] = dat.G2()
	case 79:
		t.Stats[npcStatMagic] = dat.G2()
	case 90:
		t.ResizeX = int(dat.G2())
	case 91:
		t.ResizeY = int(dat.G2())
	case 92:
		t.ResizeZ = int(dat.G2())
	case 93:
		t.Minimap = false
	case 95:
		t.VisLevel = int(dat.G2())
	case 97:
		t.ResizeH = dat.G2()
	case 98:
		t.ResizeV = dat.G2()
	case 200:
		t.WanderRange = dat.G2()
	case 201:
		t.MaxRange = dat.G2()
	case 202:
		t.HuntRange = dat.G1()
	case 203:
		t.Timer = int(dat.G2())
	case 204:
		t.RespawnRate = dat.G2()
	case 206:
		t.MoveRestrict = int(dat.G1())
	case 207:
		t.AttackRange = dat.G2()
	case 208:
		t.BlockWalk = int(dat.G1())
	case 209:
		t.HuntMode = int(dat.G1())
	case 210:
		t.DefaultMode = int(dat.G1())
	case 211:
		t.Members = true
	case 212:
		count := dat.G1()

		t.PatrolCoord = make([]uint32, count)
		t.PatrolDelay = make([]uint8, count)

		for i := range count {
			t.PatrolCoord[i] = dat.G4()
			t.PatrolDelay[i] = dat.G1()
		}
	case 213:
		t.GiveChase = false
	case 249:
		t.Params = DecodeParams(dat)
	case 250:
		t.DebugName = dat.GJStrLF()
	default:
		return fmt.Errorf("unrecognized npc config code %d", code)
	}

	return nil
}

func NewNpcType(id int) *NpcType {
	return &NpcType{
		ConfigType: ConfigType{
			ID: id,
		},
		Size:      1,
		ReadyAnim: -1,
		WalkAnim:  -1,
		WalkAnimB: -1,
		WalkAnimR: -1,
		WalkAnimL: -1,
		ResizeX:   -1,
		ResizeY:   -1,
		ResizeZ:   -1,
		Minimap:   true,
		VisLevel:  -1,
		ResizeH:   128,
		ResizeV:   128,

		// server-side
		RegenRate:    100,
		Category:     -1,
		WanderRange:  5,
		MaxRange:     7,
		Timer:        -1,
		RespawnRate:  100, // 1 minute
		Stats:        []uint16{1, 1, 1, 1, 1, 1},
		MoveRestrict: MoveRestrictNormal,
		HuntMode:     -1,
		DefaultMode:  NPCModeWander,
		BlockWalk:    BlockWalkNPC,
		Params:       make(ParamMap),
		PatrolCoord:  make([]uint32, 0),
		PatrolDelay:  make([]uint8, 0),
		GiveChase:    true,
	}
}

type NPCTypeConfigs struct {
	ConfigNames map[string]int
	Configs     []*NpcType
}

func LoadNPCTypes(dir string) (*NPCTypeConfigs, error) {
	server, err := packet2.Load(filepath.Join(dir, "server", "npc.dat"), false)
	if err != nil {
		return nil, err
	}

	clientJag, err := io.LoadJagfile(filepath.Join(dir, "client", "config"))
	if err != nil {
		return nil, err
	}

	ntc, err := parseNPCTypes(server, clientJag)
	if err != nil {
		return nil, err
	}

	return ntc, nil
}

func parseNPCTypes(server *packet2.Packet, clientJag *io.Jagfile) (*NPCTypeConfigs, error) {
	count := int(server.G2())

	configs := make([]*NpcType, count)
	configNames := make(map[string]int, count)

	client, err := clientJag.Read("npc.dat")
	if err != nil {
		return nil, err
	}
	client.Pos = 2

	for id := range count {
		config := NewNpcType(id)

		if err := DecodeType(server, config); err != nil {
			return nil, err
		}
		if err := DecodeType(client, config); err != nil {
			return nil, err
		}

		configs[id] = config

		if config.DebugName != "" {
			configNames[config.DebugName] = id
		}
	}

	ptc := &NPCTypeConfigs{
		ConfigNames: configNames,
		Configs:     configs,
	}

	return ptc, nil
}
