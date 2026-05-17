package pack

import (
	"fmt"
	"strings"

	"github.com/zsrv/goscape/pkg/objtype"
)

// parseEnumConfig is the per-key=value parser for .enum config blocks.
//
// Accepted keys:
//   - inputtype, outputtype  (ScriptVarType name → ScriptVarType code)
//   - default, val           (raw string; resolved at pack time)
//
// NAI-192-D-DEADBRANCH-OMITTED: TS parseEnumConfig declares empty
// stringKeys/numberKeys/booleanKeys arrays — dead branches preserved
// by the TS author. Goscape omits the empty branches.
//
// TS source: tools/pack/config/EnumConfig.ts:6-57.
func parseEnumConfig(key, value string) (ConfigValue, bool, error) {
	switch key {
	case "inputtype", "outputtype":
		t, ok := objtype.ScriptVarTypeFromName(value)
		if !ok {
			return nil, true, fmt.Errorf("unknown script var type: %s", value)
		}
		return t, true, nil
	case "default", "val":
		return value, true, nil
	}
	return nil, false, nil
}

// packEnumConfigs walks every id, emits the enum body per
// EnumConfig.ts:60-156. Pre-scans for inputtype/outputtype because
// the val list trailer's opcode (5 vs 6) and the per-val emission
// shape depend on outputtype, and the val-key emission depends on
// inputtype. Returns an error when either is missing (TS '!' non-null
// assert ported to explicit error).
//
// AUTOINT collapse: TS writes INT byte when inputtype is AUTOINT.
// AUTOINT inputtype: val key is p4(loopIndex), no key-half lookup.
// AUTOINT outputtype: value = lookupParamValue(outputtype, valStr)
// over the WHOLE string (no comma split).
//
// TS source: tools/pack/config/EnumConfig.ts:60-156.
func packEnumConfigs(configs map[string][]ConfigLine, pf *PackFile, lk *paramLookups) (*PackedData, error) {
	pd := NewPackedData(pf.Max)
	for id := range pf.Max {
		name := pf.GetByID(id)
		if cfg, ok := configs[name]; ok {
			var (
				inputtype  objtype.ScriptVarType
				outputtype objtype.ScriptVarType
				gotIn      bool
				gotOut     bool
				vals       []string
			)
			for _, line := range cfg {
				switch line.Key {
				case "inputtype":
					inputtype = line.Value.(objtype.ScriptVarType)
					gotIn = true
				case "outputtype":
					outputtype = line.Value.(objtype.ScriptVarType)
					gotOut = true
				}
			}
			if !gotIn {
				return nil, fmt.Errorf("%s: missing inputtype", name)
			}
			if !gotOut {
				return nil, fmt.Errorf("%s: missing outputtype", name)
			}

			for _, line := range cfg {
				switch line.Key {
				case "inputtype":
					pd.P1(1)
					if inputtype == objtype.ScriptVarTypeAutoInt {
						pd.P1(uint8(objtype.ScriptVarTypeInt))
					} else {
						pd.P1(uint8(inputtype))
					}
				case "outputtype":
					pd.P1(2)
					pd.P1(uint8(outputtype))
				case "default":
					rawDefault := line.Value.(string)
					if outputtype == objtype.ScriptVarTypeString {
						pd.P1(3)
						v, err := lookupParamValue(outputtype, rawDefault, lk)
						if err != nil {
							return nil, fmt.Errorf("%s: default: %w", name, err)
						}
						pd.PJStr(v.(string))
					} else {
						pd.P1(4)
						// TS-D-ENUM-DEFAULT-NULL-TOLERANT: EnumConfig.ts:91-99
						// has no null-check on the default lookup (asymmetric
						// with packParamConfigs and with this fn's val-lookup
						// branches). `p4(null)` coerces to 0 via setInt32.
						// Real content relies on it: stat_members `default=stat`
						// with outputtype=int. Mirror TS silent-coerce.
						v, err := lookupParamValue(outputtype, rawDefault, lk)
						if err != nil {
							pd.P4(0)
						} else {
							pd.P4(uint32(v.(int)))
						}
					}
				case "val":
					vals = append(vals, line.Value.(string))
				}
			}

			if outputtype == objtype.ScriptVarTypeString {
				pd.P1(5)
			} else {
				pd.P1(6)
			}
			pd.P2(uint16(len(vals)))
			for i, raw := range vals {
				// key half
				if inputtype == objtype.ScriptVarTypeAutoInt {
					pd.P4(uint32(i))
				} else {
					keyPart, _, ok := strings.Cut(raw, ",")
					if !ok {
						return nil, fmt.Errorf("%s: val missing comma: %s", name, raw)
					}
					v, err := lookupParamValue(inputtype, keyPart, lk)
					if err != nil {
						return nil, fmt.Errorf("%s: val key %q: %w", name, raw, err)
					}
					pd.P4(uint32(v.(int)))
				}
				// value half
				if outputtype == objtype.ScriptVarTypeAutoInt {
					v, err := lookupParamValue(outputtype, raw, lk)
					if err != nil {
						return nil, fmt.Errorf("%s: val whole %q: %w", name, raw, err)
					}
					pd.P4(uint32(v.(int)))
				} else {
					// TS substring(indexOf(',') + 1): no comma → whole string
					// (JS indexOf returns -1, +1 = 0). Real content uses bare
					// `val=N` with AUTOINT inputtype, e.g. levelup_unlocks_attack.
					valuePart := raw
					if _, after, ok := strings.Cut(raw, ","); ok {
						valuePart = after
					}
					v, err := lookupParamValue(outputtype, valuePart, lk)
					if err != nil {
						return nil, fmt.Errorf("%s: val value %q: %w", name, raw, err)
					}
					if outputtype == objtype.ScriptVarTypeString {
						pd.PJStr(v.(string))
					} else {
						pd.P4(uint32(v.(int)))
					}
				}
			}
		}
		if len(name) > 0 {
			pd.P1(250)
			pd.PJStr(name)
		}
		pd.Next()
	}
	return pd, nil
}
