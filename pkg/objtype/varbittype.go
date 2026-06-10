package objtype

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"path/filepath"

	jagfile "github.com/zsrv/goscape/pkg/io/jagfile"
	packet2 "github.com/zsrv/goscape/pkg/io/packet"
)

// VarBitType is a named bit-range inside a base player varp. Introduced
// at revision 254. Mirrors TS Engine-TS/src/cache/config/VarBitType.ts
// @43e02957.
type VarBitType struct {
	ConfigType
	Basevar  int
	Startbit int
	Endbit   int
}

func (v *VarBitType) Decode(code uint8, dat *packet2.Packet) error {
	switch code {
	case 1:
		v.Basevar = int(dat.G2())
		v.Startbit = int(dat.G1())
		v.Endbit = int(dat.G1())
	case 250:
		v.DebugName = dat.GJStrLF()
	default:
		// TS VarBitType.ts:80 logs via printError (non-fatal) and the
		// decodeType loop continues. Mirror it: log and return nil so
		// DecodeType keeps reading; the unknown code's payload is not
		// consumed, matching TS. See varptype.go for the full rationale.
		slog.Warn("objtype: unrecognized varbit config code", "code", code)
	}
	return nil
}

// NewVarBitType returns a VarBitType with the TS field-initializer
// defaults (VarBitType.ts:68-70: basevar/startbit/endbit = -1).
func NewVarBitType(id int) *VarBitType {
	return &VarBitType{
		ConfigType: ConfigType{ID: id},
		Basevar:    -1,
		Startbit:   -1,
		Endbit:     -1,
	}
}

type VarBitTypeConfigs struct {
	ConfigNames map[string]int
	Configs     []*VarBitType
}

// Get returns the VarBitType for id, or nil when the registry is nil
// (server booted against a pre-254 cache without varbit.dat) or id is
// out of range. Mirrors TS VarBitType.get (VarBitType.ts:45-47); the
// nil/OOB tolerance is the Go analog of TS's undefined array read.
func (v *VarBitTypeConfigs) Get(id int) *VarBitType {
	if v == nil || id < 0 || id >= len(v.Configs) {
		return nil
	}
	return v.Configs[id]
}

// LoadVarBitTypes reads dir/server/varbit.dat plus the varbit.dat entry
// of the dir/client/config jagfile (dual-pass decode, mirroring TS
// VarBitType.load/parse at VarBitType.ts:13-43 and the varp loader's
// structure in varptype.go).
//
// Missing-file posture: TS load early-returns when server/varbit.dat
// does not exist (VarBitType.ts:14-16), leaving an empty registry.
// Mirror that here so a 245.2-era cache (which has no varbit.dat until
// the rev-254 pack pipeline regenerates it) still boots: return an
// empty, non-nil registry and no error.
func LoadVarBitTypes(dir string) (*VarBitTypeConfigs, error) {
	server, err := packet2.Load(filepath.Join(dir, "server", "varbit.dat"), false)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return &VarBitTypeConfigs{ConfigNames: map[string]int{}}, nil
		}
		return nil, err
	}
	clientJag, err := jagfile.LoadJagfile(filepath.Join(dir, "client", "config"))
	if err != nil {
		return nil, err
	}
	return parseVarBitTypes(server, clientJag)
}

func parseVarBitTypes(server *packet2.Packet, clientJag *jagfile.Jagfile) (*VarBitTypeConfigs, error) {
	count := int(server.G2())

	client, err := clientJag.Read("varbit.dat")
	if err != nil {
		return nil, fmt.Errorf("client/config varbit.dat: %w", err)
	}
	client.Pos = 2 // skip the 2-byte count header on the client side

	configs := make([]*VarBitType, count)
	configNames := make(map[string]int, count)

	for id := range count {
		config := NewVarBitType(id)
		if err := DecodeType(server, config); err != nil {
			return nil, fmt.Errorf("varbit id %d (server): %w", id, err)
		}
		if err := DecodeType(client, config); err != nil {
			return nil, fmt.Errorf("varbit id %d (client): %w", id, err)
		}
		configs[id] = config
		if config.DebugName != "" {
			configNames[config.DebugName] = id
		}
	}

	return &VarBitTypeConfigs{
		ConfigNames: configNames,
		Configs:     configs,
	}, nil
}
