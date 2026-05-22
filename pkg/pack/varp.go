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
			return nil, true, fmt.Errorf("unknown script var type: %s: %w", value, ErrUnknownVarType)
		}
		return t, true, nil
	}
	return nil, false, nil
}

// packVarpConfigs walks every id in [0, pf.Max), pulls the debugname
// from the PackFile, emits per-config opcodes on the server buffer
// (scope=1, type=2, protect-when-false=4, transmit-when-true=6,
// debugname-trailer=250) and on the client buffer (clientcode=5+p2).
// Each slot ends with PackedData.Next() on both buffers — a single
// 0x00 terminator + idx entry-length.
//
// Returns server first to match parseVarpTypes' read order in
// pkg/objtype/varptype.go (server count first, then per-slot
// server-decode then client-decode).
//
// NAI-193-D-PACKFILE-SINGLETONS-DEFERRED: TS uses module-level
// VarpPack; goscape takes *PackFile as a parameter (continuation of
// NAI-191 §2 / NAI-192 deferral).
//
// TS source: tools/pack/config/VarpConfig.ts:69-110.
// TS author note at VarpConfig.ts:97 — "// todo: maybe this was
// opcode 10?" — preserved here as a TS-author uncertainty about the
// 250 trailer opcode, not a goscape deviation.
func packVarpConfigs(configs map[string][]ConfigLine, pf *PackFile) (server, client *PackedData) {
	server = NewPackedData(pf.Max)
	client = NewPackedData(pf.Max)

	for id := range pf.Max {
		name := pf.GetByID(id)
		if cfg, ok := configs[name]; ok {
			for _, line := range cfg {
				switch line.Key {
				case "scope":
					server.P1(1)
					server.P1(uint8(line.Value.(int)))
				case "type":
					server.P1(2)
					server.P1(uint8(line.Value.(objtype.ScriptVarType)))
				case "protect":
					if !line.Value.(bool) {
						server.P1(4)
					}
				case "clientcode":
					client.P1(5)
					client.P2(uint16(line.Value.(int)))
				case "transmit":
					if line.Value.(bool) {
						server.P1(6)
					}
				}
			}
		}
		if len(name) > 0 {
			server.P1(250)
			server.PJStr(name)
		}
		server.Next()
		client.Next()
	}
	return server, client
}
