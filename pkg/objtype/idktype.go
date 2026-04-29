package objtype

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	io "github.com/zsrv/goscape/pkg/io/jagfile"
	packet2 "github.com/zsrv/goscape/pkg/io/packet"
)

// IdkType is a single idk.dat config record (identity-kit / character design slot).
// Mirrors Engine-TS/src/cache/config/IdkType.ts.
type IdkType struct {
	ConfigType
	Type    int       // body-part slot; -1 = unset
	Models  []uint16  // nil = no models
	Heads   [5]uint16 // 0xFFFF = unset (TS Uint16Array(5).fill(-1))
	RecolS  [6]uint16
	RecolD  [6]uint16
	Disable bool
}

// NewIdkType returns an IdkType with TS-faithful defaults.
// TS default: type=-1, heads=Uint16Array(5).fill(-1), recol_s/recol_d zeroed.
func NewIdkType(id int) *IdkType {
	return &IdkType{
		ConfigType: ConfigType{ID: id},
		Type:       -1,
		Heads:      [5]uint16{0xFFFF, 0xFFFF, 0xFFFF, 0xFFFF, 0xFFFF},
	}
}

// Decode dispatches on the idk config opcode, matching TS IdkType.decode
// at Engine-TS/src/cache/config/IdkType.ts:62-89.
func (t *IdkType) Decode(code uint8, dat *packet2.Packet) error {
	switch code {
	case 1:
		t.Type = int(dat.G1())
	case 2:
		count := dat.G1()
		t.Models = make([]uint16, count)
		for i := range count {
			t.Models[i] = dat.G2()
		}
	case 3:
		t.Disable = true
	case 40, 41, 42, 43, 44, 45, 46, 47, 48, 49:
		// TS recol_s is 6-element; codes 46-49 are out-of-range in Go.
		// Guard matches TS Uint16Array silent-discard behavior.
		slot := code - 40
		v := dat.G2()
		if slot < 6 {
			t.RecolS[slot] = v
		}
	case 50, 51, 52, 53, 54, 55, 56, 57, 58, 59:
		// Same guard for recol_d.
		slot := code - 50
		v := dat.G2()
		if slot < 6 {
			t.RecolD[slot] = v
		}
	case 60, 61, 62, 63, 64, 65, 66, 67, 68, 69:
		// TS heads[] is 5-element; codes 65-69 are out-of-range. Guard to
		// avoid panic; consume the G2 regardless so the packet cursor advances.
		slot := code - 60
		v := dat.G2()
		if slot < 5 {
			t.Heads[slot] = v
		}
	case 250:
		t.DebugName = dat.GJStrLF()
	default:
		return fmt.Errorf("unrecognized idk config code %d", code)
	}
	return nil
}

// IdkTypeConfigs is the parsed registry of all identity-kit config records.
type IdkTypeConfigs struct {
	ConfigNames map[string]int
	Configs     []*IdkType
}

// LoadIdkTypes parses server/idk.dat + client/config jag → idk.dat into
// an IdkTypeConfigs registry. Returns an empty registry with nil error when
// server/idk.dat is absent (silent-on-missing, matching TS IdkType.load at
// Engine-TS/src/cache/config/IdkType.ts:14-18).
func LoadIdkTypes(dir string) (*IdkTypeConfigs, error) {
	server, err := packet2.Load(filepath.Join(dir, "server", "idk.dat"), false)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &IdkTypeConfigs{ConfigNames: map[string]int{}}, nil
		}
		return nil, err
	}

	clientJag, err := io.LoadJagfile(filepath.Join(dir, "client", "config"))
	if err != nil {
		return nil, err
	}

	return parseIdkTypes(server, clientJag)
}

func parseIdkTypes(server *packet2.Packet, clientJag *io.Jagfile) (*IdkTypeConfigs, error) {
	count := int(server.G2())
	configs := make([]*IdkType, count)
	configNames := make(map[string]int, count)

	client, err := clientJag.Read("idk.dat")
	if err != nil {
		return nil, err
	}
	client.Pos = 2 // skip client-side count header (same as npctype.go:377)

	for id := range count {
		config := NewIdkType(id)
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

	return &IdkTypeConfigs{ConfigNames: configNames, Configs: configs}, nil
}
