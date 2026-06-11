package pack

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/zsrv/goscape/pkg/objtype"
)

// parseInvConfigFor returns the per-key=value parser for .inv config
// blocks. Returns a closure capturing the obj name-map for stockN
// resolution.
//
// Accepted keys:
//   - size       (number, bounded [0, 65535])
//   - scope      ("shared"|"perm"|"temp" → InvTypeScope*)
//   - stackall, restock, allstock, protect, runweight, dummyinv  (boolean)
//   - stockN     ("objName,count[,respawn]" → []int{objId, count[, respawn]})
//
// TS source: tools/pack/config/InvConfig.ts:5-92.
func parseInvConfigFor(objPack *PackFile) ParseFn {
	return func(key, value string) (ConfigValue, bool, error) {
		switch key {
		case "size":
			n, err := strconv.ParseInt(value, 0, 64)
			if err != nil {
				return nil, true, fmt.Errorf("invalid size: %s", value)
			}
			if n < 0 || n > 65535 {
				return nil, true, fmt.Errorf("size out of range [0, 65535]: %d: %w", n, ErrOutOfRange)
			}
			return int(n), true, nil
		case "scope":
			switch value {
			case "shared":
				return objtype.InvTypeScopeShared, true, nil
			case "perm":
				return objtype.InvTypeScopePerm, true, nil
			case "temp":
				return objtype.InvTypeScopeTemp, true, nil
			}
			return nil, true, fmt.Errorf("invalid scope: %s", value)
		case "stackall", "restock", "allstock", "protect", "runweight", "dummyinv":
			if !IsConfigBoolean(value) {
				return nil, true, fmt.Errorf("invalid boolean: %s", value)
			}
			return GetConfigBoolean(value), true, nil
		}
		if strings.HasPrefix(key, "stock") {
			parts := strings.Split(value, ",")
			if len(parts) < 2 {
				return nil, true, fmt.Errorf("stockN expects 'obj,count[,respawn]': %s", value)
			}
			objIdx := objPack.GetByName(parts[0])
			if objIdx == -1 {
				return nil, true, fmt.Errorf("unknown obj: %s: %w", parts[0], ErrUnknownObj)
			}
			count, err := strconv.Atoi(parts[1])
			if err != nil {
				return nil, true, fmt.Errorf("invalid stock count: %s", parts[1])
			}
			if len(parts) == 2 {
				return []int{objIdx, count}, true, nil
			}
			respawn, err := strconv.Atoi(parts[2])
			if err != nil {
				return nil, true, fmt.Errorf("invalid stock respawn: %s", parts[2])
			}
			return []int{objIdx, count, respawn}, true, nil
		}
		return nil, false, nil
	}
}

// packInvConfigs walks every id and emits opcodes 1/2/3/5/6/7/8/9 inline.
// Stock entries are collected into a SPARSE slice indexed by the N in
// stockN (stockN → slot N-1), then the stock block (opcode 4) is emitted
// after all other opcodes with 0xffff,0,0 filler rows for the holes.
//
// 254 changes vs 244 (TS InvConfig.ts:104-184 @ 2e3bcf43 — surfaced at
// the T23 full-tree gate via adventurershop, which skips stock2):
//   - size pre-scan pass restored (TS:106-113).
//   - stock[index] keyed by parseInt(key[5:]) - 1 (TS:125), not push order.
//   - Duplicate-stockN → packStepError (TS:126-128).
//   - stockN index >= size → packStepError (TS:130-132).
//   - Holes emit p2(-1) p2(0) p4(0) filler rows (TS:167-172).
//
// modelFlags is accepted for TS ConfigPackCallback parity
// (PackShared.ts:137-141); inv does not write any model flags.
//
// TS source: tools/pack/config/InvConfig.ts:94-197 (2e3bcf43).
func packInvConfigs(configs map[string][]ConfigLine, pf *PackFile, modelFlags []int) (*PackedData, error) {
	pd := NewPackedData(pf.Max)
	for id := range pf.Max {
		name := pf.GetByID(id)
		if cfg, ok := configs[name]; ok {
			// TS:106-113: size pre-scan (last size= line wins).
			size := 0
			for _, line := range cfg {
				if line.Key == "size" {
					size = line.Value.(int)
				}
			}

			// Sparse stock slots; nil = hole (TS sparse array, :125-134).
			var stock [][]int
			for _, line := range cfg {
				switch {
				case line.Key == "scope":
					pd.P1(1)
					pd.P1(uint8(line.Value.(int)))
				case line.Key == "size":
					pd.P1(2)
					pd.P2(uint16(line.Value.(int)))
				case strings.HasPrefix(line.Key, "stock"):
					// TS:125: index = parseInt(key.substring(5)) - 1.
					// A non-numeric/absent suffix gives NaN in TS; the
					// stock[NaN] write is a no-op on the array proper
					// (named property, length unchanged) → drop the line.
					n, err := strconv.Atoi(line.Key[len("stock"):])
					if err != nil || n-1 < 0 {
						break
					}
					index := n - 1
					if index < len(stock) && stock[index] != nil {
						return nil, packStepError(name, "Duplicate stock%d lines, one will overwrite the other.", index+1)
					}
					if index >= size {
						return nil, packStepError(name, "stock%d is larger than size=%d", index+1, size)
					}
					for len(stock) <= index {
						stock = append(stock, nil)
					}
					stock[index] = line.Value.([]int)
				case line.Key == "stackall":
					if line.Value.(bool) {
						pd.P1(3)
					}
				case line.Key == "restock":
					if line.Value.(bool) {
						pd.P1(5)
					}
				case line.Key == "allstock":
					if line.Value.(bool) {
						pd.P1(6)
					}
				case line.Key == "protect":
					if !line.Value.(bool) {
						pd.P1(7)
					}
				case line.Key == "runweight":
					if line.Value.(bool) {
						pd.P1(8)
					}
				case line.Key == "dummyinv":
					if line.Value.(bool) {
						pd.P1(9)
					}
				}
			}

			// TS:162-184: emit stock block only when non-empty; holes
			// (skipped stockN) become 0xffff,0,0 filler rows (TS:167-172).
			if len(stock) > 0 {
				pd.P1(4)
				pd.P1(uint8(len(stock)))
				for _, entry := range stock {
					if entry == nil {
						pd.P2(0xffff) // TS p2(-1)
						pd.P2(0)
						pd.P4(0)
						continue
					}
					pd.P2(uint16(entry[0]))
					pd.P2(uint16(entry[1]))
					if len(entry) == 3 {
						pd.P4(uint32(entry[2]))
					} else {
						pd.P4(0)
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
