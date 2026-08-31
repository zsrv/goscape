package pack

import (
	"fmt"
	"strings"

	"github.com/zsrv/goscape/pkg/objtype"
)

// parseStructConfigFor returns the per-key=value parser for .struct
// config blocks. Only `param=name,value` is accepted. Param names are
// resolved against the runtime ParamType registry (loaded between
// .param save and .struct parse in PackConfigs); values are resolved
// via lookupParamValue using the param's typed code.
//
// NAI-192-D-DEADBRANCH-OMITTED: TS parseStructConfig declares empty
// stringKeys/numberKeys/booleanKeys arrays — dead branches preserved
// by the TS author. Goscape omits the empty branches.
//
// TS source: tools/pack/config/StructConfig.ts:7-67.
func parseStructConfigFor(paramTypes *objtype.ParamTypeConfigs, lk *paramLookups) ParseFn {
	return func(key, value string) (ConfigValue, bool, error) {
		if key != "param" {
			return nil, false, nil
		}
		before, after, ok0 := strings.Cut(value, ",")
		if !ok0 {
			return nil, true, fmt.Errorf("param expects 'name,value': %s", value)
		}
		name := before
		raw := after
		id, ok := paramTypes.ConfigNames[name]
		if !ok {
			return nil, true, fmt.Errorf("unknown param: %s: %w", name, ErrUnknownParam)
		}
		pt := paramTypes.Configs[id]
		resolved, err := lookupParamValue(pt.Type, raw, lk)
		if err != nil {
			return nil, true, fmt.Errorf("param %s value: %w", name, err)
		}
		return ParamValue{ID: id, Type: pt.Type, Value: resolved}, true, nil
	}
}

// packStructConfigs walks every id, collects all `param=` lines, and
// emits opcode 249 + p1(count) + per-param p3(id) + pbool(isString)
// + pjstr(value)|p4(value) when at least one param is present. 250
// trailer + Next() always.
//
// TS source: tools/pack/config/StructConfig.ts:70-117.
func packStructConfigs(configs map[string][]ConfigLine, pf *PackFile) *PackedData {
	pd := NewPackedData(pf.Max)
	for id := range pf.Max {
		name := pf.GetByID(id)
		if cfg, ok := configs[name]; ok {
			var params []ParamValue
			for _, line := range cfg {
				if line.Key == "param" {
					params = append(params, line.Value.(ParamValue))
				}
			}
			if len(params) > 0 {
				pd.P1(249)
				pd.P1(uint8(len(params)))
				for _, p := range params {
					pd.P3(uint32(p.ID))
					isString := p.Type == objtype.ScriptVarTypeString
					pd.PBool(isString)
					if isString {
						pd.PJStr(p.Value.(string))
					} else {
						pd.P4(uint32(p.Value.(int)))
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
	return pd
}
