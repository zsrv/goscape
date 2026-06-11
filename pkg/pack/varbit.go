package pack

import (
	"fmt"
	"strconv"
)

// parseVarbitConfigFor returns the per-key=value parser for .varbit
// config blocks. The varbit family is NEW at rev-254 (introduced
// between 9aadcec4/3c16994c and the 254 pin; verified unchanged
// 43e02957 → 2e3bcf43).
//
// Accepted keys:
//   - startbit, endbit  (number; decimal or 0x-prefixed hex)
//   - basevar           (varp debugname → varp id via VarpPack.getByName;
//     unknown name is an invalid VALUE, TS returns null)
//
// Return contract (matches NAI-192 ParseFn):
//   - (value, true, nil)  → accepted
//   - (nil, true, err)    → recognized key with invalid value
//   - (nil, false, nil)   → unrecognized key
//
// NAI-193-D-PACKFILE-SINGLETONS-DEFERRED: TS reads the module-level
// VarpPack singleton; goscape takes the varp *PackFile as a parameter.
//
// TS source: tools/pack/config/VarbitConfig.ts:4-43 @ 2e3bcf43.
func parseVarbitConfigFor(varpPack *PackFile) ParseFn {
	return func(key, value string) (ConfigValue, bool, error) {
		switch key {
		case "startbit", "endbit":
			// strconv.ParseInt(value, 0, 64) accepts decimal AND
			// 0x-prefixed hex with a single call — equivalent to the TS
			// branch on value.startsWith('0x') plus regex validation plus
			// NaN check (VarbitConfig.ts:10-32, same shape as varp
			// clientcode).
			n, err := strconv.ParseInt(value, 0, 64)
			if err != nil {
				return nil, true, fmt.Errorf("invalid %s: %s", key, value)
			}
			return int(n), true, nil
		case "basevar":
			// TS VarbitConfig.ts:33-39: VarpPack.getByName; -1 → null
			// (invalid property value).
			id := varpPack.GetByName(value)
			if id == -1 {
				return nil, true, fmt.Errorf("unknown basevar: %s", value)
			}
			return id, true, nil
		}
		return nil, false, nil
	}
}

// packVarbitConfigs walks every id in [0, pf.Max), pulls the debugname
// from the PackFile, and emits:
//   - client buffer: code 1 + p2(basevar) + p1(startbit) + p1(endbit),
//     ONLY when all three of basevar/startbit/endbit were declared
//     (TS collects them across the block, last writer wins, then checks
//     `!== null` on all three);
//   - server buffer: debugname-trailer 250 + pjstr(debugname).
//
// Each slot ends with PackedData.Next() on both buffers.
//
// Returns server first to match parseVarBitTypes' read order in
// pkg/objtype/varbittype.go (same convention as packVarpConfigs).
//
// NAI-193-D-PACKFILE-SINGLETONS-DEFERRED: TS uses module-level
// VarbitPack; goscape takes *PackFile as a parameter.
// modelFlags is accepted for TS ConfigPackCallback parity
// (PackShared.ts:138); varbit does not write any model flags.
//
// TS source: tools/pack/config/VarbitConfig.ts:45-88 @ 2e3bcf43.
func packVarbitConfigs(configs map[string][]ConfigLine, pf *PackFile, modelFlags []int) (server, client *PackedData) {
	server = NewPackedData(pf.Max)
	client = NewPackedData(pf.Max)

	for id := range pf.Max {
		name := pf.GetByID(id)
		if cfg, ok := configs[name]; ok {
			// TS VarbitConfig.ts:54-68: collect across the block; a later
			// duplicate key overwrites an earlier one.
			basevar, startbit, endbit := -1, -1, -1
			haveBasevar, haveStartbit, haveEndbit := false, false, false
			for _, line := range cfg {
				switch line.Key {
				case "basevar":
					basevar = line.Value.(int)
					haveBasevar = true
				case "startbit":
					startbit = line.Value.(int)
					haveStartbit = true
				case "endbit":
					endbit = line.Value.(int)
					haveEndbit = true
				}
			}
			// TS VarbitConfig.ts:70-75: emit only when all three present.
			if haveBasevar && haveStartbit && haveEndbit {
				client.P1(1)
				client.P2(uint16(basevar))
				client.P1(uint8(startbit))
				client.P1(uint8(endbit))
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
