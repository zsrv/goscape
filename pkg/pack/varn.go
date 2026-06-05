package pack

import (
	"fmt"

	"github.com/zsrv/goscape/pkg/objtype"
)

// parseVarnConfig is the per-key=value parser for .varn config blocks.
// Only `type` is accepted; all other keys are reported as invalid via
// ok=false.
//
// NAI-192-D-DEADBRANCH-OMITTED: TS parseVarnConfig contains empty
// stringKeys/numberKeys/booleanKeys arrays — dead code preserved by
// the TS author. Goscape omits the empty branches; they revive when a
// future schema addition needs them.
//
// TS source: tools/pack/config/VarnConfig.ts:5-51.
func parseVarnConfig(key, value string) (ConfigValue, bool, error) {
	if key == "type" {
		t, ok := objtype.ScriptVarTypeFromName(value)
		if !ok {
			return nil, true, fmt.Errorf("unknown script var type: %s: %w", value, ErrUnknownVarType)
		}
		return t, true, nil
	}
	return nil, false, nil
}

// packVarnConfigs walks every id in [0, pf.Max), pulls the debugname
// from the PackFile, emits the parsed config body (currently just the
// 1-byte `type` opcode), then writes the debugname trailer (opcode
// 250 + LF-terminated string) when the slot has a name. Each slot
// ends with PackedData.Next() — a single 0x00 terminator + idx offset.
//
// modelFlags is accepted for TS ConfigPackCallback parity
// (PackShared.ts:137-141); varn does not write any model flags.
//
// TS source: tools/pack/config/VarnConfig.ts:53-82.
func packVarnConfigs(configs map[string][]ConfigLine, pf *PackFile, modelFlags []int) *PackedData {
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
