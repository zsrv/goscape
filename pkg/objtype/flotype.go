package objtype

import (
	"fmt"
	"path/filepath"

	"github.com/zsrv/goscape/pkg/io/jagfile"
	packet2 "github.com/zsrv/goscape/pkg/io/packet"
)

// FloType is a floor-type entry decoded from the flo.dat client stream.
// Mirrors the instance fields of TS FloType (src/cache/config/FloType.ts):
//
//	rgb: number = 0          (opcode 1 — G3)
//	texture: number = -1     (opcode 2 — G1; -1 = no texture)
//	overlay: bool = false    (opcode 3 — flag, no payload)
//	occlude: bool = true     (opcode 5 sets occlude = false; default true)
//	debugname: string = ""   (opcode 6 — GJStrLF)
type FloType struct {
	Id        int
	DebugName string
	RGB       int  // 24-bit colour; default 0
	Texture   int  // texture id; -1 = none (TS default)
	Overlay   bool // true when opcode 3 was present
	Occlude   bool // false when opcode 5 was present; TS default = true
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
		// TS FloType constructor defaults: texture = -1, occlude = true.
		ft := &FloType{Id: id, Texture: -1, Occlude: true}
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
			ft.RGB = int(p.G3()) // 24-bit colour
		case 2:
			ft.Texture = int(p.G1()) // texture id (0-based)
		case 3:
			ft.Overlay = true // flag; no payload
		case 5:
			ft.Occlude = false // flag; no payload (TS default is true, opcode 5 clears it)
		case 6:
			ft.DebugName = p.GJStrLF()
		default:
			return fmt.Errorf("unknown flo opcode %d (id=%d)", code, id)
		}
	}
}
