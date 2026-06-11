package config

import (
	"fmt"

	"github.com/zsrv/goscape/pkg/pack"
	"github.com/zsrv/goscape/pkg/unpack/internal/model"
)

// idkPartTypeName maps IDK part-type indices to names.
// TS source: IdkConfig.ts:41-56 (IdkPartType enum).
var idkPartTypeName = [14]string{
	"man_hair",    // 0
	"man_jaw",     // 1
	"man_torso",   // 2
	"man_arms",    // 3
	"man_hands",   // 4
	"man_legs",    // 5
	"man_feet",    // 6
	"woman_hair",  // 7
	"woman_jaw",   // 8
	"woman_torso", // 9
	"woman_arms",  // 10
	"woman_hands", // 11
	"woman_legs",  // 12
	"woman_feet",  // 13
}

// unpackIdk is the core implementation of IdkConfig unpacking.
// Called by Env.UnpackIdk; also directly by tests.
//
// compare/modelRenameOffset feed the 254 model-rename guard at both model
// sites (see keepPackedName). Pass nil/0 for the pre-254 behavior.
//
// TS source: tools/unpack/config/IdkConfig.ts:58-159 @2e3bcf43.
func unpackIdk(
	cfg *ConfigIdx,
	id int,
	compare *ConfigIdx,
	modelRenameOffset int,
	idkPack, texturePack *pack.PackFile,
	modelPack *pack.PackFile,
	models *model.Store,
	srcDir string,
	warnf func(string, ...any),
	errorf func(string, ...any),
) []string {
	dat := cfg.Dat

	def := make([]string, 0, 8)
	// TS line 62: def.push(`[${IdkPack.getById(id)}]`)
	debugname := getByID(idkPack, id)
	def = append(def, fmt.Sprintf("[%s]", debugname))

	// resolveModel applies the 254 rename guard before each renameModel call
	// (TS IdkConfig.ts:87-93, 113-119 @2e3bcf43).
	resolveModel := func(modelID int, name string) string {
		if modelPack == nil {
			return ""
		}
		if keepPackedName(compare, id, modelID, modelRenameOffset) {
			return modelPack.GetByID(modelID)
		}
		return renameModelIdk(modelID, name, modelPack, srcDir, errorf)
	}

	var modelIDs []int
	// TS uses JS sparse arrays; we use maps to replicate undefined-skip semantics.
	// Iterating by index 0..max and skipping missing entries mirrors TS.
	recolSrc := make(map[int]int)
	recolDst := make(map[int]int)
	maxRecolIndex := -1

	// TS line 69: dat.pos = pos[id]
	dat.Pos = cfg.Pos[id]
	for {
		code := int(dat.G1())
		if code == 0 {
			break
		}

		switch {
		case code == 1:
			// TS lines 76-79: type = g1; def.push(`type=${IdkPartType[type]}`)
			// DIVERGENCE: for part-type >= 14 (out-of-bounds enum), TS emits `type=undefined`
			// (JS enum reverse-lookup returns undefined); Go emits the decimal number instead.
			// Unreachable with valid data — TS output would be degenerate anyway (IdkConfig.ts:79).
			typ := int(dat.G1())
			name := ""
			if typ < len(idkPartTypeName) {
				name = idkPartTypeName[typ]
			} else {
				name = fmt.Sprintf("%d", typ)
			}
			def = append(def, fmt.Sprintf("type=%s", name))

		case code == 2:
			// TS lines 80-95 @2e3bcf43: count = g1; for each: modelId = g2;
			// guarded renameModel; push model{i+1}=
			count := int(dat.G1())
			for i := range count {
				modelID := int(dat.G2())
				modelIDs = append(modelIDs, modelID)
				def = append(def, fmt.Sprintf("model%d=%s", i+1, resolveModel(modelID, debugname)))
			}

		case code == 3:
			// TS line 91: def.push('disable=yes')
			def = append(def, "disable=yes")

		case code >= 40 && code < 50:
			// TS lines 92-96: recolSrc[index] = g2
			index := code - 40
			recolSrc[index] = int(dat.G2())
			if index > maxRecolIndex {
				maxRecolIndex = index
			}

		case code >= 50 && code < 60:
			// TS lines 97-101: recolDst[index] = g2
			index := code - 50
			recolDst[index] = int(dat.G2())
			if index > maxRecolIndex {
				maxRecolIndex = index
			}

		case code >= 60 && code < 70:
			// TS lines 106-120 @2e3bcf43: head models, index = code-60+1, guarded rename
			index := code - 60 + 1
			modelID := int(dat.G2())
			modelIDs = append(modelIDs, modelID)
			def = append(def, fmt.Sprintf("head%d=%s", index, resolveModel(modelID, debugname+"_head")))

		default:
			// TS line 111: printWarning(`unknown idk code ${code}`)
			warnf("unknown idk code %d", code)
		}
	}

	// TS lines 115-117: incomplete read warning
	if dat.Pos != cfg.Pos[id]+cfg.Len[id] {
		warnf("incomplete read: %d != %d", dat.Pos, cfg.Pos[id]+cfg.Len[id])
	}

	// TS lines 119-146: recol/retex post-processing.
	// TS iterates recolSrc.length and skips undefined entries.
	// threshold = 100 (idk/npc/obj/loc).
	def = emitRecolSparse(def, recolSrc, recolDst, maxRecolIndex, modelIDs, texturePack, models, 100, false)

	return def
}

// emitRecolSparse handles TS sparse-array recol semantics (idk/spotanim style).
// Skips indices where recolSrc[i] is absent. 1-based output index = i+1.
//
// TS IdkConfig.ts:119-146, SpotAnimConfig.ts:112-139.
func emitRecolSparse(def []string, recolSrc, recolDst map[int]int, maxIndex int, modelIDs []int, texturePack *pack.PackFile, models *model.Store, threshold int, locMode bool) []string {
	for i := range maxIndex + 1 {
		src, hasSrc := recolSrc[i]
		if !hasSrc {
			// TS: if (typeof recolSrc[i] === 'undefined') continue
			continue
		}
		dst := recolDst[i]
		index := i + 1
		def = appendRecolLine(def, index, src, dst, modelIDs, texturePack, models, threshold, locMode)
	}
	return def
}

// emitRecolDense handles TS dense-array recol semantics (npc/obj/loc style).
// Iterates 0..len(recolSrc)-1, no skipping. 1-based output index = i+1.
//
// TS NpcConfig.ts:162-185, ObjConfig.ts:233-255, LocConfig.ts:304-327.
func emitRecolDense(def []string, recolSrc, recolDst []int, modelIDs []int, texturePack *pack.PackFile, models *model.Store, threshold int, locMode bool) []string {
	for i, src := range recolSrc {
		dst := recolDst[i]
		index := i + 1
		def = appendRecolLine(def, index, src, dst, modelIDs, texturePack, models, threshold, locMode)
	}
	return def
}

// appendRecolLine emits one recol or retex pair at the given 1-based index.
func appendRecolLine(def []string, index, srcRaw, dstRaw int, modelIDs []int, texturePack *pack.PackFile, models *model.Store, threshold int, locMode bool) []string {
	srcRgbSlice := colorconvReverseHsl(srcRaw)
	dstRgbSlice := colorconvReverseHsl(dstRaw)

	// TS: reverseHsl(...)[0] — undefined when empty. Go: raw fallback.
	srcRgb, srcRgbOk := firstOrZero(srcRgbSlice)
	dstRgb, dstRgbOk := firstOrZero(dstRgbSlice)

	srcVal := srcRaw
	if srcRgbOk {
		srcVal = srcRgb
	}
	dstVal := dstRaw
	if dstRgbOk {
		dstVal = dstRgb
	}

	if srcRaw >= threshold || dstRaw >= threshold {
		// Output as RGB.
		def = append(def, fmt.Sprintf("recol%ds=%d", index, srcVal))
		def = append(def, fmt.Sprintf("recol%dd=%d", index, dstVal))
	} else if locMode && (!srcRgbOk || !dstRgbOk) {
		// LocConfig.ts:318: retex when either reverseHsl result is undefined.
		def = append(def, fmt.Sprintf("retex%ds=%s", index, getByID(texturePack, srcRaw)))
		def = append(def, fmt.Sprintf("retex%dd=%s", index, getByID(texturePack, dstRaw)))
	} else if models != nil && models.ModelsHaveTexture(modelIDs, srcRaw) {
		// Model has source as texture — output as texture.
		def = append(def, fmt.Sprintf("retex%ds=%s", index, getByID(texturePack, srcRaw)))
		def = append(def, fmt.Sprintf("retex%dd=%s", index, getByID(texturePack, dstRaw)))
	} else {
		// Output as RGB.
		def = append(def, fmt.Sprintf("recol%ds=%d", index, srcVal))
		def = append(def, fmt.Sprintf("recol%dd=%d", index, dstVal))
	}
	return def
}
