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
				return nil, true, fmt.Errorf("size out of range [0, 65535]: %d", n)
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
				return nil, true, fmt.Errorf("unknown obj: %s", parts[0])
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

// packInvConfigs walks every id, pre-finds size, then walks config
// lines emitting opcodes 1/2/3/5/6/7/8/9 inline. Stock entries are
// collected into a sparse []*[]int slot map by stockN index, then the
// stock-list trailer (opcode 4) is emitted at end of config.
//
// Error paths (TS packStepError analogue):
//   - duplicate stockN line     → "%s: duplicate stockN"
//   - stockN index >= size      → "%s: stockN exceeds size"
//
// TS source: tools/pack/config/InvConfig.ts:94-197.
func packInvConfigs(configs map[string][]ConfigLine, pf *PackFile) (*PackedData, error) {
	pd := NewPackedData(pf.Max)
	for id := range pf.Max {
		name := pf.GetByID(id)
		if cfg, ok := configs[name]; ok {
			size := 0
			for _, line := range cfg {
				if line.Key == "size" {
					size = line.Value.(int)
				}
			}

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
					n, err := strconv.Atoi(line.Key[5:])
					if err != nil {
						return nil, fmt.Errorf("%s: invalid stock key: %s", name, line.Key)
					}
					index := n - 1
					if index >= size {
						return nil, fmt.Errorf("%s: stock%d exceeds size=%d", name, n, size)
					}
					for len(stock) <= index {
						stock = append(stock, nil)
					}
					if stock[index] != nil {
						return nil, fmt.Errorf("%s: duplicate stock%d", name, n)
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

			if len(stock) > 0 {
				pd.P1(4)
				pd.P1(uint8(len(stock)))
				for _, slot := range stock {
					if slot == nil {
						pd.P2(uint16(0xffff)) // -1 as uint16
						pd.P2(0)
						pd.P4(0)
						continue
					}
					pd.P2(uint16(slot[0]))
					pd.P2(uint16(slot[1]))
					if len(slot) == 3 {
						pd.P4(uint32(slot[2]))
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
