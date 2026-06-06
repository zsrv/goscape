package config

import (
	"fmt"

	"github.com/zsrv/goscape/pkg/pack"
)

// unpackFlo is the core implementation of FloConfig unpacking.
// Called by Env.UnpackFlo; also directly by tests.
//
// TS source: tools/unpack/config/FloConfig.ts:6-47.
func unpackFlo(cfg *ConfigIdx, id int, floPack, texturePack *pack.PackFile, warnf func(string, ...any)) []string {
	dat := cfg.Dat

	def := make([]string, 0, 4)
	// TS line 10: def.push(`[${FloPack.getById(id)}]`)
	def = append(def, fmt.Sprintf("[%s]", getByID(floPack, id)))

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
			// TS line 21-22: const colour = dat.g3(); def.push(`colour=0x${colour.toString(16).toUpperCase().padStart(6,'0')}`)
			colour := dat.G3()
			def = append(def, fmt.Sprintf("colour=0x%06X", colour))
		case 2:
			// TS line 23-27: const texture = dat.g1(); textureName = TexturePack.getById(texture) || `texture_${texture}`
			texture := int(dat.G1())
			textureName := getByID(texturePack, texture)
			if textureName == "" {
				textureName = fmt.Sprintf("texture_%d", texture)
			}
			def = append(def, fmt.Sprintf("texture=%s", textureName))
		case 3:
			// TS line 28-29
			def = append(def, "overlay=yes")
		case 5:
			// TS line 30-31
			def = append(def, "occlude=no")
		case 6:
			// TS line 32-36: read gjstr but do NOT emit it (commented out)
			_ = dat.GJStrLF()
		default:
			// TS line 37-39: printWarning(`unknown flo code ${code}`)
			warnf("unknown flo code %d", code)
		}
	}

	// TS line 42-44: position-mismatch warning
	if dat.Pos != cfg.Pos[id]+cfg.Len[id] {
		warnf("incomplete read: %d != %d", dat.Pos, cfg.Pos[id]+cfg.Len[id])
	}

	return def
}
