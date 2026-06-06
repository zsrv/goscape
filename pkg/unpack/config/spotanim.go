package config

import (
	"fmt"

	"github.com/zsrv/goscape/pkg/pack"
	"github.com/zsrv/goscape/pkg/unpack/internal/model"
)

// unpackSpotAnim is the core implementation of SpotAnimConfig unpacking.
// Called by Env.UnpackSpotAnim; also directly by tests.
//
// TS source: tools/unpack/config/SpotAnimConfig.ts:41-142.
func unpackSpotAnim(
	cfg *ConfigIdx,
	id int,
	spotAnimPack, texturePack *pack.PackFile,
	seqPack *pack.PackFile,
	modelPack *pack.PackFile,
	models *model.Store,
	srcDir string,
	warnf func(string, ...any),
	errorf func(string, ...any),
) []string {
	dat := cfg.Dat

	def := make([]string, 0, 8)
	// TS line 44: debugname = SpotAnimPack.getById(id)
	debugname := getByID(spotAnimPack, id)
	def = append(def, fmt.Sprintf("[%s]", debugname))

	var modelIDs []int
	// SpotAnim uses sparse arrays (like idk): opcodes 40-49 set recolSrc[index],
	// opcodes 50-59 set recolDst[index], and the post-pass skips undefined entries.
	recolSrc := make(map[int]int)
	recolDst := make(map[int]int)
	maxRecolIndex := -1

	// TS line 52: dat.pos = pos[id]
	dat.Pos = cfg.Pos[id]
	for {
		code := int(dat.G1())
		if code == 0 {
			break
		}

		switch {
		case code == 1:
			// TS lines 59-65: modelId = g2; renameModel; push model=
			modelID := int(dat.G2())
			modelIDs = append(modelIDs, modelID)
			var modelName string
			if modelPack != nil {
				modelName = renameModelSpot(modelID, debugname, modelPack, srcDir, errorf)
			}
			def = append(def, fmt.Sprintf("model=%s", modelName))

		case code == 2:
			// TS lines 66-70: seqId = g2; SeqPack.getById or 'seq_' + seqId
			seqID := int(dat.G2())
			seqName := getByID(seqPack, seqID)
			if seqName == "" {
				seqName = fmt.Sprintf("seq_%d", seqID)
			}
			def = append(def, fmt.Sprintf("anim=%s", seqName))

		case code == 3:
			// TS line 71-73: push hasalpha=yes
			def = append(def, "hasalpha=yes")

		case code == 4:
			// TS lines 74-77: resizeh = g2
			resizeh := dat.G2()
			def = append(def, fmt.Sprintf("resizeh=%d", resizeh))

		case code == 5:
			// TS lines 78-81: resizev = g2
			resizev := dat.G2()
			def = append(def, fmt.Sprintf("resizev=%d", resizev))

		case code == 6:
			// TS lines 82-85: angle = g2
			angle := dat.G2()
			def = append(def, fmt.Sprintf("angle=%d", angle))

		case code == 7:
			// TS lines 86-89: ambient = g1b (signed)
			ambient := int(int8(dat.G1()))
			def = append(def, fmt.Sprintf("ambient=%d", ambient))

		case code == 8:
			// TS lines 90-93: contrast = g1b (signed)
			contrast := int(int8(dat.G1()))
			def = append(def, fmt.Sprintf("contrast=%d", contrast))

		case code >= 40 && code < 50:
			// TS lines 93-97: recolSrc[index] = g2
			index := code - 40
			recolSrc[index] = int(dat.G2())
			if index > maxRecolIndex {
				maxRecolIndex = index
			}

		case code >= 50 && code < 60:
			// TS lines 98-102: recolDst[index] = g2
			index := code - 50
			recolDst[index] = int(dat.G2())
			if index > maxRecolIndex {
				maxRecolIndex = index
			}

		default:
			// TS line 103-104: printWarning(`unknown spotanim code ${code}`)
			warnf("unknown spotanim code %d", code)
		}
	}

	// TS lines 108-110: incomplete read warning
	if dat.Pos != cfg.Pos[id]+cfg.Len[id] {
		warnf("incomplete read: %d != %d", dat.Pos, cfg.Pos[id]+cfg.Len[id])
	}

	// TS lines 112-139: recol/retex post-processing.
	// SpotAnim uses sparse arrays (skip undefined) AND threshold >= 50.
	// TS comment: "texture ids cap at 50, so we can save time knowing it's not a texture id"
	def = emitRecolSparse(def, recolSrc, recolDst, maxRecolIndex, modelIDs, texturePack, models, 50, false)

	return def
}
