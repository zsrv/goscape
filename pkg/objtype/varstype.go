package objtype

import (
	"log/slog"
	"path/filepath"

	packet2 "github.com/zsrv/goscape/pkg/io/packet"
)

type VarSharedType struct {
	ConfigType
	Type ScriptVarType
}

func (v *VarSharedType) Decode(code uint8, dat *packet2.Packet) error {
	switch code {
	case 1:
		v.Type = ScriptVarType(dat.G1())
	case 250:
		v.DebugName = dat.GJStrLF()
	default:
		// TS VarSharedType.ts:73 logs via printError (non-fatal) and the
		// decodeType loop continues. Mirror it: log and return nil so
		// DecodeType keeps reading; the unknown code's payload is not
		// consumed, matching TS. See varptype.go for the full rationale.
		slog.Warn("objtype: unrecognized vars config code", "code", code)
	}
	return nil
}

func NewVarSharedType(id int) *VarSharedType {
	return &VarSharedType{
		ID:   id,
		Type: ScriptVarTypeInt,
	}
}

type VarsTypeConfigs struct {
	ConfigNames map[string]int
	Configs     []*VarSharedType
}

func LoadVarsTypes(dir string) (*VarsTypeConfigs, error) {
	server, err := packet2.Load(filepath.Join(dir, "server", "vars.dat"), false)
	if err != nil {
		return nil, err
	}
	return parseVarsTypes(server)
}

func parseVarsTypes(server *packet2.Packet) (*VarsTypeConfigs, error) {
	count := int(server.G2())

	configs := make([]*VarSharedType, count)
	configNames := make(map[string]int, count)

	for id := range count {
		config := NewVarSharedType(id)
		if err := DecodeType(server, config); err != nil {
			return nil, err
		}
		configs[id] = config
		if config.DebugName != "" {
			configNames[config.DebugName] = id
		}
	}

	return &VarsTypeConfigs{
		ConfigNames: configNames,
		Configs:     configs,
	}, nil
}
