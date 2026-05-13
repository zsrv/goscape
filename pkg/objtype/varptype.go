package objtype

import (
	"fmt"
	"path/filepath"

	jagfile "github.com/zsrv/goscape/pkg/io/jagfile"
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
	// RunID is the varp id of the engine run-mode varp (the config whose
	// ClientCode==7). Defaults to 0 (TS placeholder default at
	// Engine-TS/src/cache/config/VarPlayerType.ts:18) when no clientcode-7
	// config exists in the cache. Set by dynamic discovery mirroring
	// VarPlayerType.ts:50-53.
	RunID int
}

func LoadVarpTypes(dir string) (*VarpTypeConfigs, error) {
	server, err := packet2.Load(filepath.Join(dir, "server", "varp.dat"), false)
	if err != nil {
		return nil, err
	}
	clientJag, err := jagfile.LoadJagfile(filepath.Join(dir, "client", "config"))
	if err != nil {
		return nil, err
	}
	return parseVarpTypes(server, clientJag)
}

func parseVarpTypes(server *packet2.Packet, clientJag *jagfile.Jagfile) (*VarpTypeConfigs, error) {
	count := int(server.G2())

	client, err := clientJag.Read("varp.dat")
	if err != nil {
		return nil, fmt.Errorf("client/config varp.dat: %w", err)
	}
	client.Pos = 2 // skip the 2-byte count header on the client side

	configs := make([]*VarPlayerType, count)
	configNames := make(map[string]int, count)
	runID := 0

	for id := range count {
		config := NewVarPlayerType(id)
		if err := DecodeType(server, config); err != nil {
			return nil, fmt.Errorf("varp id %d (server): %w", id, err)
		}
		if err := DecodeType(client, config); err != nil {
			return nil, fmt.Errorf("varp id %d (client): %w", id, err)
		}
		configs[id] = config
		if config.DebugName != "" {
			configNames[config.DebugName] = id
		}
		if config.ClientCode == 7 {
			runID = id
		}
	}

	return &VarpTypeConfigs{
		ConfigNames: configNames,
		Configs:     configs,
		RunID:       runID,
	}, nil
}

// ByName returns the VarPlayerType matching the given debugname, or nil
// if no match exists. Mirrors TS VarPlayerType.getByName. Uses the
// ConfigNames index built at load time — O(1) on name-indexed configs,
// O(N) only if ConfigNames is unpopulated (test fixtures) or stale.
// Consumed by ::setvar / ::setvarother / ::getvar / ::getvarother in
// modules/world/handlers_game.go (NAI-185).
func (vtc *VarpTypeConfigs) ByName(name string) *VarPlayerType {
	if vtc == nil {
		return nil
	}
	if id, ok := vtc.ConfigNames[name]; ok {
		if id >= 0 && id < len(vtc.Configs) {
			return vtc.Configs[id]
		}
	}
	for _, c := range vtc.Configs {
		if c != nil && c.DebugName == name {
			return c
		}
	}
	return nil
}
