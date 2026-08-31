package objtype

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	packet2 "github.com/zsrv/goscape/pkg/io/packet"
)

// MesanimType is a single mesanim.dat config record (message-animation
// frame-length table for chathead animations). Mirrors
// Engine-TS/src/cache/config/MesanimType.ts.
//
// TS-faithful: server-side .dat only (no client-jag side).
type MesanimType struct {
	ConfigType
	Len [4]int // init -1; code N (1..4) writes Len[N-1] = G2()
}

// NewMesanimType returns a MesanimType with TS-faithful defaults
// (Len init -1, matching TS Array(4).fill(-1)).
func NewMesanimType(id int) *MesanimType {
	return &MesanimType{
		ID:  id,
		Len: [4]int{-1, -1, -1, -1},
	}
}

// Decode dispatches on the mesanim config opcode, matching TS
// MesanimType.decode at Engine-TS/src/cache/config/MesanimType.ts:62-70.
func (t *MesanimType) Decode(code uint8, dat *packet2.Packet) error {
	switch {
	case code >= 1 && code <= 4:
		t.Len[code-1] = int(dat.G2())
	case code == 250:
		t.DebugName = dat.GJStrLF()
	default:
		return fmt.Errorf("unrecognized mesanim config code %d", code)
	}
	return nil
}

// MesanimTypeConfigs is the parsed registry of all mesanim.dat records.
type MesanimTypeConfigs struct {
	ConfigNames map[string]int
	Configs     []*MesanimType
}

// LoadMesanimTypes parses server/mesanim.dat into a MesanimTypeConfigs
// registry. Returns an empty registry with nil error when
// server/mesanim.dat is absent (silent-on-missing, matching TS
// MesanimType.load at Engine-TS/src/cache/config/MesanimType.ts:11-17).
func LoadMesanimTypes(dir string) (*MesanimTypeConfigs, error) {
	server, err := packet2.Load(filepath.Join(dir, "server", "mesanim.dat"), false)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &MesanimTypeConfigs{ConfigNames: map[string]int{}}, nil
		}
		return nil, err
	}

	count := int(server.G2())
	configs := make([]*MesanimType, count)
	configNames := make(map[string]int, count)
	for id := range count {
		c := NewMesanimType(id)
		if err := DecodeType(server, c); err != nil {
			return nil, err
		}
		configs[id] = c
		if c.DebugName != "" {
			configNames[c.DebugName] = id
		}
	}
	return &MesanimTypeConfigs{ConfigNames: configNames, Configs: configs}, nil
}
