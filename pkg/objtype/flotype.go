package objtype

import (
	"fmt"
	"path/filepath"

	"github.com/zsrv/goscape/pkg/io/jagfile"
	packet2 "github.com/zsrv/goscape/pkg/io/packet"
)

// FloType is a minimal binary view of a floor-type entry. The full
// TS FloType has many more fields; goscape's worldmap packer only
// needs the debugname → id mapping and the total count.
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

// LoadFloTypes mirrors TS FloType.load: reads dir/server/flo.dat
// for the count + per-id server stream, then dir/client/config
// (jagfile) for the per-id client stream where debugname etc. live.
// All real Content data is in the client stream — the server stream
// contains only zero terminators (TS PackedData.next()), matching
// FloConfig.packFloConfigs which writes everything to client.
func LoadFloTypes(dir string) (*FloTypeConfigs, error) {
	server, err := packet2.Load(filepath.Join(dir, "server", "flo.dat"), false)
	if err != nil {
		return nil, fmt.Errorf("server/flo.dat: %w", err)
	}
	clientJag, err := jagfile.LoadJagfile(filepath.Join(dir, "client", "config"))
	if err != nil {
		return nil, fmt.Errorf("client/config: %w", err)
	}
	return parseFloTypes(server, clientJag)
}

func parseFloTypes(server *packet2.Packet, clientJag *jagfile.Jagfile) (*FloTypeConfigs, error) {
	count := int(server.G2())

	client, err := clientJag.Read("flo.dat")
	if err != nil {
		return nil, fmt.Errorf("client/config flo.dat: %w", err)
	}
	client.Pos = 2 // skip the 2-byte count header on the client side

	configs := make([]*FloType, count)
	names := make(map[string]int, count)

	for id := range count {
		ft := &FloType{Id: id}
		if err := decodeFloStream(server, ft, id); err != nil {
			return nil, fmt.Errorf("flo id %d (server): %w", id, err)
		}
		if err := decodeFloStream(client, ft, id); err != nil {
			return nil, fmt.Errorf("flo id %d (client): %w", id, err)
		}
		configs[id] = ft
		if ft.DebugName != "" {
			names[ft.DebugName] = id
		}
	}
	return &FloTypeConfigs{Configs: configs, ConfigNames: names}, nil
}

// decodeFloStream reads one entry's opcode stream (terminated by 0).
// Mirrors TS FloType.decode. Only opcodes 1, 2, 3, 5, 6 are valid;
// any other opcode is an error (TS throws).
func decodeFloStream(p *packet2.Packet, ft *FloType, id int) error {
	for {
		code := p.G1()
		if code == 0 {
			return nil
		}
		switch code {
		case 1:
			_ = p.G3() // rgb
		case 2:
			_ = p.G1() // texture
		case 3:
			// overlay = true; no payload
		case 5:
			// occlude = false; no payload
		case 6:
			ft.DebugName = p.GJStrLF()
		default:
			return fmt.Errorf("unknown flo opcode %d (id=%d)", code, id)
		}
	}
}
