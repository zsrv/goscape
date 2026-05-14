package pack

import (
	"fmt"
	"strconv"
	"strings"
)

// floNumberKeys mirrors TS FloConfig.ts:7-9 numberKeys[].
var floNumberKeys = map[string]struct{}{
	"colour": {},
}

// floBooleanKeys mirrors TS FloConfig.ts:11-13 booleanKeys[].
var floBooleanKeys = map[string]struct{}{
	"overlay": {},
	"occlude": {},
}

// parseFloConfigFor returns the per-key=value parser for .flo config
// blocks. Closure-captures texturePack (for the `texture` key).
//
// NAI-195-D-DEADBRANCH-OMITTED: TS FloConfig.ts:5 declares empty
// stringKeys[] — omitted.
//
// TS source: tools/pack/config/FloConfig.ts:4-61.
func parseFloConfigFor(texturePack *PackFile) ParseFn {
	return func(key, value string) (ConfigValue, bool, error) {
		if _, ok := floNumberKeys[key]; ok {
			n, err := strconv.ParseInt(value, 0, 64)
			if err != nil {
				return nil, true, fmt.Errorf("invalid number for %s: %s", key, value)
			}
			return int(n), true, nil
		}
		if _, ok := floBooleanKeys[key]; ok {
			if !IsConfigBoolean(value) {
				return nil, true, fmt.Errorf("invalid boolean for %s: %s", key, value)
			}
			return GetConfigBoolean(value), true, nil
		}
		if key == "texture" {
			idx := texturePack.GetByName(value)
			if idx == -1 {
				return nil, true, fmt.Errorf("unknown texture: %s", value)
			}
			return idx, true, nil
		}
		return nil, false, nil
	}
}

// packFloConfigs emits the per-id body for .flo configs.
//
// CRITICAL: server-side has NO opcode emission anywhere in the body.
// TS FloConfig.ts:64-65 constructs `server: PackedData` and the only
// server call inside the per-id loop is `server.next()` (line 100).
// Do NOT add a 250-trailer "by analogy" with .seq/.spotanim/.idk.
//
// The debugname trailer for .flo lives on the CLIENT side as opcode 6,
// gated `debugname.length && !startsWith('flo_')` (TS FloConfig.ts:93-97).
//
// TS source: tools/pack/config/FloConfig.ts:63-104.
func packFloConfigs(configs map[string][]ConfigLine, floPack *PackFile) (server, client *PackedData) {
	server = NewPackedData(floPack.Max)
	client = NewPackedData(floPack.Max)

	for id := range floPack.Max {
		debugname := floPack.GetByID(id)
		cfg := configs[debugname]

		for _, line := range cfg {
			switch line.Key {
			case "colour":
				client.P1(1)
				client.P3(uint32(line.Value.(int)))
			case "texture":
				client.P1(2)
				client.P1(uint8(line.Value.(int)))
			case "overlay":
				if line.Value.(bool) {
					client.P1(3)
				}
			case "occlude":
				if !line.Value.(bool) {
					client.P1(5)
				}
			}
		}

		if len(debugname) > 0 && !strings.HasPrefix(debugname, "flo_") {
			client.P1(6)
			client.PJStr(debugname)
		}

		client.Next()
		server.Next()
	}
	return server, client
}
