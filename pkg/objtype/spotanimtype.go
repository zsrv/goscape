package objtype

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	io "github.com/zsrv/goscape/pkg/io/jagfile"
	"github.com/zsrv/goscape/pkg/io/packet"
)

// SpotanimType is a single spotanim.dat config record (graphic / spot animation).
// Mirrors Engine-TS/src/cache/config/SpotanimType.ts.
type SpotanimType struct {
	ConfigType
	Model       int
	Anim        int // -1 default
	RecolS      [6]uint16
	RecolD      [6]uint16
	Resizeh     int // 128 default
	Resizev     int // 128 default
	Angle int
	Ambient     int
	Contrast    int
}

// NewSpotanimType returns a SpotanimType with TS-faithful defaults.
// TS default: anim=-1, resizeh=128, resizev=128.
func NewSpotanimType(id int) *SpotanimType {
	return &SpotanimType{
		ID:      id,
		Anim:    -1,
		Resizeh: 128,
		Resizev: 128,
	}
}

// Decode dispatches on the spotanim config opcode, matching TS SpotanimType.decode
// at Engine-TS/src/cache/config/SpotanimType.ts:78-104.
func (t *SpotanimType) Decode(code uint8, dat *packet.Packet) error {
	switch code {
	case 1:
		t.Model = int(dat.G2())
	case 2:
		t.Anim = int(dat.G2())
	case 4:
		t.Resizeh = int(dat.G2())
	case 5:
		t.Resizev = int(dat.G2())
	case 6:
		t.Angle = int(dat.G2())
	case 7:
		t.Ambient = int(dat.G1())
	case 8:
		t.Contrast = int(dat.G1())
	case 40, 41, 42, 43, 44, 45, 46, 47, 48, 49:
		// TS recol_s is 6-element (Uint16Array(6)); codes 46-49 are out-of-range.
		// Guard matches TS silent-discard behavior (array index out of bounds = noop).
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
	case 250:
		t.DebugName = dat.GJStrLF()
	default:
		return fmt.Errorf("unrecognized spotanim config code %d", code)
	}
	return nil
}

// SpotanimTypeConfigs is the parsed registry of all spotanim config records.
type SpotanimTypeConfigs struct {
	ConfigNames map[string]int
	Configs     []*SpotanimType
}

// LoadSpotanimTypes parses server/spotanim.dat + client/config jag's
// spotanim.dat into a SpotanimTypeConfigs registry. Returns an empty
// registry with nil error when server/spotanim.dat is absent (silent-on-missing,
// matching TS SpotanimType.load at Engine-TS/src/cache/config/SpotanimType.ts:12-19).
func LoadSpotanimTypes(dir string) (*SpotanimTypeConfigs, error) {
	server, err := packet.Load(filepath.Join(dir, "server", "spotanim.dat"), false)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &SpotanimTypeConfigs{ConfigNames: map[string]int{}}, nil
		}
		return nil, err
	}

	clientJag, err := io.LoadJagfile(filepath.Join(dir, "client", "config"))
	if err != nil {
		return nil, err
	}

	return parseSpotanimTypes(server, clientJag)
}

func parseSpotanimTypes(server *packet.Packet, clientJag *io.Jagfile) (*SpotanimTypeConfigs, error) {
	count := int(server.G2())
	configs := make([]*SpotanimType, count)
	configNames := make(map[string]int, count)

	client, err := clientJag.Read("spotanim.dat")
	if err != nil {
		return nil, err
	}
	client.Pos = 2 // skip client-side count header (matches idktype.go pattern)

	for id := range count {
		config := NewSpotanimType(id)
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

	return &SpotanimTypeConfigs{ConfigNames: configNames, Configs: configs}, nil
}

// ByName returns the SpotanimType matching the given debugname, or nil
// if no match exists. Mirrors TS SpotanimType.getByName. Uses the
// ConfigNames index built at load time — O(1) on name-indexed configs,
// O(N) linear-scan fallback for test fixtures or stale indices.
// Consumed by dispatchDebugproc in modules/world/handlers_game.go (NAI-189).
func (c *SpotanimTypeConfigs) ByName(name string) *SpotanimType {
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
