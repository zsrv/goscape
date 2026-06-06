package config

import (
	"fmt"

	"github.com/zsrv/goscape/pkg/pack"
)

// unpackVarp is the core implementation of VarpConfig unpacking.
// Called by Env.UnpackVarp; also directly by tests.
//
// TS source: tools/unpack/config/VarpConfig.ts:6-33.
func unpackVarp(cfg *ConfigIdx, id int, varpPack *pack.PackFile, warnf func(string, ...any)) []string {
	dat := cfg.Dat

	def := make([]string, 0, 4)
	// TS line 10: def.push(`[${VarpPack.getById(id)}]`)
	def = append(def, fmt.Sprintf("[%s]", getByID(varpPack, id)))

	// TS line 12: dat.pos = pos[id]
	dat.Pos = cfg.Pos[id]
	for {
		// TS line 14: const code = dat.g1()
		code := dat.G1()
		if code == 0 {
			break
		}

		switch code {
		case 5:
			// TS line 19-22: const clientcode = dat.g2(); def.push(`clientcode=${clientcode}`)
			clientcode := dat.G2()
			def = append(def, fmt.Sprintf("clientcode=%d", clientcode))
		default:
			// TS line 23-25: printWarning(`unknown varp code ${code}`)
			warnf("unknown varp code %d", code)
		}
	}

	// TS line 27-29: position-mismatch warning
	if dat.Pos != cfg.Pos[id]+cfg.Len[id] {
		warnf("incomplete read: %d != %d", dat.Pos, cfg.Pos[id]+cfg.Len[id])
	}

	return def
}
