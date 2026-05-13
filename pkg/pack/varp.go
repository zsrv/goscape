package pack

import (
	"fmt"
	"strconv"

	"github.com/zsrv/goscape/pkg/objtype"
)

// parseVarpConfig is the per-key=value parser for .varp config blocks.
//
// Accepted keys:
//   - clientcode  (number; decimal or 0x-prefixed hex)
//   - protect     (boolean; yes/no/true/false/1/0)
//   - transmit    (boolean; same value set as protect)
//   - scope       ("perm" | "temp" → VarpScopePerm | VarpScopeTemp)
//   - type        (ScriptVarType name → ScriptVarType code)
//
// Return contract (matches NAI-192 ParseFn):
//   - (value, true, nil)  → accepted
//   - (nil, true, err)    → recognized key with invalid value
//   - (nil, false, nil)   → unrecognized key
//
// TS source: tools/pack/config/VarpConfig.ts:5-67.
func parseVarpConfig(key, value string) (ConfigValue, bool, error) {
	switch key {
	case "clientcode":
		// strconv.ParseInt(value, 0, 64) accepts decimal AND 0x-prefixed
		// hex with a single call — equivalent to the TS branch on
		// value.startsWith('0x') plus regex validation plus NaN check.
		n, err := strconv.ParseInt(value, 0, 64)
		if err != nil {
			return nil, true, fmt.Errorf("invalid clientcode: %s", value)
		}
		return int(n), true, nil
	case "protect", "transmit":
		if !IsConfigBoolean(value) {
			return nil, true, fmt.Errorf("invalid boolean: %s", value)
		}
		return GetConfigBoolean(value), true, nil
	case "scope":
		switch value {
		case "perm":
			return objtype.VarpScopePerm, true, nil
		case "temp":
			return objtype.VarpScopeTemp, true, nil
		default:
			return nil, true, fmt.Errorf("invalid scope: %s", value)
		}
	case "type":
		t, ok := objtype.ScriptVarTypeFromName(value)
		if !ok {
			return nil, true, fmt.Errorf("unknown script var type: %s", value)
		}
		return t, true, nil
	}
	return nil, false, nil
}
