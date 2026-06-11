package config

import (
	"fmt"

	"github.com/zsrv/goscape/pkg/pack"
)

// unpackVarbit is the core implementation of VarbitConfig unpacking (NEW at
// rev-254). Called by Env.UnpackVarbit; also directly by tests.
//
// TS source: tools/unpack/config/VarbitConfig.ts:6-38 @2e3bcf43.
func unpackVarbit(cfg *ConfigIdx, id int, varbitPack, varpPack *pack.PackFile, warnf func(string, ...any)) []string {
	dat := cfg.Dat

	def := make([]string, 0, 4)
	// TS line 10: def.push(`[${VarbitPack.getById(id)}]`)
	def = append(def, fmt.Sprintf("[%s]", getByID(varbitPack, id)))

	// TS line 12: dat.pos = pos[id]
	dat.Pos = cfg.Pos[id]
	for {
		// TS line 14: const code = dat.g1()
		code := dat.G1()
		if code == 0 {
			break
		}

		switch code {
		case 1:
			// TS lines 19-27: varpId = g2; startbit = g1; endbit = g1;
			// basevar resolved through VarpPack with the `varp_<id>` fallback.
			varpID := int(dat.G2())
			startbit := dat.G1()
			endbit := dat.G1()

			varpName := getByID(varpPack, varpID)
			if varpName == "" {
				varpName = fmt.Sprintf("varp_%d", varpID)
			}
			def = append(def, fmt.Sprintf("basevar=%s", varpName))
			def = append(def, fmt.Sprintf("startbit=%d", startbit))
			def = append(def, fmt.Sprintf("endbit=%d", endbit))
		default:
			// TS lines 28-30: printWarning(`unknown varbit code ${code}`)
			warnf("unknown varbit code %d", code)
		}
	}

	// TS lines 33-35: position-mismatch warning
	if dat.Pos != cfg.Pos[id]+cfg.Len[id] {
		warnf("incomplete read: %d != %d", dat.Pos, cfg.Pos[id]+cfg.Len[id])
	}

	return def
}
