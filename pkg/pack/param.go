package pack

import (
	"fmt"

	"github.com/zsrv/goscape/pkg/objtype"
)

// parseParamConfig is the per-key=value parser for .param config blocks.
//
// Accepted keys:
//   - autodisable  (boolean; yes/no/true/false/1/0)
//   - type         (ScriptVarType name → ScriptVarType code)
//   - default      (raw string; resolution deferred to packParamConfigs
//                   after `type` is known — mirrors TS comment
//                   "defer lookup to pack callback")
//
// Return contract (matches NAI-192 ParseFn):
//   - (value, true, nil)  → accepted
//   - (nil, true, err)    → recognized key with invalid value
//   - (nil, false, nil)   → unrecognized key
//
// TS source: tools/pack/config/ParamConfig.ts parseParamConfig (~190-240).
func parseParamConfig(key, value string) (ConfigValue, bool, error) {
	switch key {
	case "autodisable":
		if !IsConfigBoolean(value) {
			return nil, true, fmt.Errorf("invalid boolean: %s", value)
		}
		return GetConfigBoolean(value), true, nil
	case "type":
		t, ok := objtype.ScriptVarTypeFromName(value)
		if !ok {
			return nil, true, fmt.Errorf("unknown script var type: %s", value)
		}
		return t, true, nil
	case "default":
		return value, true, nil
	}
	return nil, false, nil
}
