package pack

import (
	"fmt"
	"strconv"
	"strings"
)

// parseMesAnimConfigFor returns the per-key=value parser for .mesanim
// config blocks. Only `len*` keys are accepted; the value is the
// debug name of a seq looked up via seqPack.
//
// NAI-192-D-DEADBRANCH-OMITTED: TS parseMesAnimConfig declares empty
// stringKeys/numberKeys/booleanKeys arrays — dead branches preserved
// by the TS author. Goscape omits the empty branches; they revive when
// a future schema addition needs them.
//
// TS source: tools/pack/config/MesAnimConfig.ts:4-55.
func parseMesAnimConfigFor(seqPack *PackFile) ParseFn {
	return func(key, value string) (ConfigValue, bool, error) {
		if strings.HasPrefix(key, "len") {
			idx := seqPack.GetByName(value)
			if idx == -1 {
				return nil, true, fmt.Errorf("unknown seq: %s: %w", value, ErrUnknownSeq)
			}
			return idx, true, nil
		}
		return nil, false, nil
	}
}

// packMesAnimConfigs emits the per-id body for .mesanim configs. Each
// id walks the config block once, emitting per-`lenN` opcodes via
// max(0, parsedLen-1)+1 followed by p2(seqIdx). The 250-trailer fires
// when the slot has a non-empty debugname. Each id ends with Next()
// (terminator + idx length).
//
// TS source: tools/pack/config/MesAnimConfig.ts:57-90.
func packMesAnimConfigs(configs map[string][]ConfigLine, pf *PackFile) *PackedData {
	pd := NewPackedData(pf.Max)
	for id := range pf.Max {
		name := pf.GetByID(id)
		if cfg, ok := configs[name]; ok {
			for _, line := range cfg {
				if !strings.HasPrefix(line.Key, "len") {
					continue
				}
				lenN, err := strconv.Atoi(line.Key[3:])
				if err != nil {
					// non-numeric `lenN` suffix → TS isNaN-continue
					continue
				}
				opcode := max(lenN-1, 0)
				opcode++
				pd.P1(uint8(opcode))
				pd.P2(uint16(line.Value.(int)))
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
