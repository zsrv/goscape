package objtype

import (
	"fmt"
	"path/filepath"

	packet2 "github.com/zsrv/goscape/pkg/io/packet"
)

const (
	InvTypeScopeTemp   = 0
	InvTypeScopePerm   = 1
	InvTypeScopeShared = 2
)

type InvType struct {
	ConfigType
	Scope      int
	Size       int
	StackAll   bool
	Restock    bool
	AllStock   bool
	StockObj   []uint16
	StockCount []uint16
	StockRate  []int32
	Protect    bool
	RunWeight  bool // inv contributes to weight
	DummyInv   bool // inv only accepts objs with dummyitem=inv_only
}

func (t *InvType) Decode(code uint8, dat *packet2.Packet) error {
	switch code {
	case 1:
		t.Scope = int(dat.G1())
	case 2:
		t.Size = int(dat.G2())
	case 3:
		t.StackAll = true
	case 4:
		count := dat.G1()

		t.StockObj = make([]uint16, count)
		t.StockCount = make([]uint16, count)
		t.StockRate = make([]int32, count)

		for i := range count {
			t.StockObj[i] = dat.G2()
			t.StockCount[i] = dat.G2()
			t.StockRate[i] = int32(dat.G4())
		}
	case 5:
		t.Restock = true
	case 6:
		t.AllStock = true
	case 7:
		t.Protect = false
	case 8:
		t.RunWeight = true
	case 9:
		t.DummyInv = true
	case 250:
		t.DebugName = dat.GJStrLF()
	default:
		return fmt.Errorf("unrecognized inv config code %d", code)
	}

	return nil
}

func NewInvType(id int) *InvType {
	return &InvType{
		ConfigType: ConfigType{
			ID: id,
		},
		Scope:      0,
		Size:       1,
		StackAll:   false,
		Restock:    false,
		AllStock:   false,
		StockObj:   nil,
		StockCount: nil,
		StockRate:  nil,
		Protect:    true,
		RunWeight:  false,
		DummyInv:   false,
	}
}

type InvTypeConfigs struct {
	ConfigNames map[string]int
	Configs     []*InvType

	// commonly referenced in-engine
	Inv  int
	Worn int
}

func LoadInvTypes(dir string) (*InvTypeConfigs, error) {
	server, err := packet2.Load(filepath.Join(dir, "server", "inv.dat"), false)
	if err != nil {
		return nil, err
	}

	c, err := parseInvTypes(server)
	if err != nil {
		return nil, err
	}

	return c, nil
}

func parseInvTypes(server *packet2.Packet) (*InvTypeConfigs, error) {
	count := int(server.G2())

	configs := make([]*InvType, count)
	configNames := make(map[string]int, count)

	for id := range count {
		config := NewInvType(id)

		if err := DecodeType(server, config); err != nil {
			return nil, err
		}

		configs[id] = config

		if config.DebugName != "" {
			configNames[config.DebugName] = id
		}
	}

	inv, ok := configNames["inv"]
	if !ok {
		inv = -1
	}

	worn, ok := configNames["worn"]
	if !ok {
		worn = -1
	}

	c := &InvTypeConfigs{
		ConfigNames: configNames,
		Configs:     configs,
		Inv:         inv,
		Worn:        worn,
	}

	return c, nil
}

// ByName returns the InvType matching the given debugname, or nil
// if no match exists. Mirrors TS InvType.getByName. Uses the
// ConfigNames index built at load time — O(1) on name-indexed configs,
// O(N) linear-scan fallback for test fixtures or stale indices.
// Consumed by dispatchDebugproc in modules/world/handlers_game.go (NAI-189).
func (c *InvTypeConfigs) ByName(name string) *InvType {
	if c == nil {
		return nil
	}
	if id, ok := c.ConfigNames[name]; ok {
		if id >= 0 && id < len(c.Configs) {
			return c.Configs[id]
		}
	}
	for _, t := range c.Configs {
		if t != nil && t.DebugName == name {
			return t
		}
	}
	return nil
}
