package pack

import (
	"fmt"

	"github.com/zsrv/goscape/pkg/objtype"
)

// parseVarsConfig is structurally identical to parseVarnConfig — same
// schema (`type=<scriptvartype-name>`), same accept/reject contract.
// Kept as a separate function to mirror TS one-per-config-domain shape;
// future schema additions are expected to diverge (e.g. var-shared
// scope vs var-npc scope).
//
// TS source: tools/pack/config/VarsConfig.ts:5-51.
func parseVarsConfig(key, value string) (ConfigValue, bool, error) {
	if key == "type" {
		t, ok := objtype.ScriptVarTypeFromName(value)
		if !ok {
			return nil, true, fmt.Errorf("unknown script var type: %s: %w", value, ErrUnknownVarType)
		}
		return t, true, nil
	}
	return nil, false, nil
}

// packVarsConfigs writes the .vars cache buffer using the same opcode
// shape as varn: 0x01 + 1-byte type code, then 0xfa + LF-terminated
// debugname, then terminator. See packVarnConfigs.
//
// modelFlags is accepted for TS ConfigPackCallback parity
// (PackShared.ts:137-141); vars does not write any model flags.
//
// TS source: tools/pack/config/VarsConfig.ts:53-82.
func packVarsConfigs(configs map[string][]ConfigLine, pf *PackFile, modelFlags []int) *PackedData {
	pd := NewPackedData(pf.Max)
	for id := range pf.Max {
		name := pf.GetByID(id)
		if cfg, ok := configs[name]; ok {
			for _, line := range cfg {
				if line.Key == "type" {
					pd.P1(1)
					pd.P1(uint8(line.Value.(objtype.ScriptVarType)))
				}
			}
		}
		if len(name) > 0 {
			pd.P1(250)
			pd.PJStr(name)
		}
		pd.Next()
	}
	return pd
}
