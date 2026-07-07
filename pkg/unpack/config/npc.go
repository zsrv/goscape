package config

import (
	"fmt"

	"github.com/zsrv/goscape/pkg/pack"
	"github.com/zsrv/goscape/pkg/render/model"
)

// unpackNpc is the core implementation of NpcConfig unpacking.
// Called by Env.UnpackNpc; also directly by tests.
//
// compare/modelRenameOffset feed the 254 model-rename guard:
// `(compare && id < compare.size) || modelId < modelRenameOffset` keeps the
// packed model name instead of renaming (TS NpcConfig.ts:68-74, 129-135).
// Pass nil/0 for the pre-254 behavior (TS `modelId < undefined` is false).
//
// TS source: tools/unpack/config/NpcConfig.ts:41-198 @2e3bcf43.
func unpackNpc(
	cfg *ConfigIdx,
	id int,
	compare *ConfigIdx,
	modelRenameOffset int,
	npcPack, texturePack *pack.PackFile,
	seqPack *pack.PackFile,
	modelPack *pack.PackFile,
	models *model.Store,
	srcDir string,
	warnf func(string, ...any),
	errorf func(string, ...any),
) []string {
	dat := cfg.Dat

	def := make([]string, 0, 16)
	// TS line 45: debugname = NpcPack.getById(id)
	debugname := getByID(npcPack, id)
	def = append(def, fmt.Sprintf("[%s]", debugname))

	var modelIDs []int
	// NpcConfig opcode 40 reads pairs into parallel slices (dense).
	var recolSrc []int
	var recolDst []int

	// TS line 43: dat.pos = pos[id]
	dat.Pos = cfg.Pos[id]
	for {
		code := int(dat.G1())
		if code == 0 {
			break
		}

		switch {
		case code == 1:
			// TS lines 59-76 @2e3bcf43: count = g1; for each: index=i+1; modelId = g2;
			// guard keeps the packed name, otherwise renameModel; push model{index}=
			count := int(dat.G1())
			for i := range count {
				index := i + 1
				modelID := int(dat.G2())
				modelIDs = append(modelIDs, modelID)
				var modelName string
				if modelPack != nil {
					if keepPackedName(compare, id, modelID, modelRenameOffset) {
						modelName = modelPack.GetByID(modelID)
					} else {
						modelName = renameModelNpc(modelID, debugname, modelPack, srcDir, errorf)
					}
				}
				def = append(def, fmt.Sprintf("model%d=%s", index, modelName))
			}

		case code == 2:
			// TS line 72-73: name = gjstr; push name=
			name := dat.GJStrLF()
			def = append(def, fmt.Sprintf("name=%s", name))

		case code == 3:
			// TS line 74-75: desc = gjstr; push desc=
			desc := dat.GJStrLF()
			def = append(def, fmt.Sprintf("desc=%s", desc))

		case code == 12:
			// TS line 77-78: size = g1b; push size=
			size := int(int8(dat.G1()))
			def = append(def, fmt.Sprintf("size=%d", size))

		case code == 13:
			// TS lines 79-83: readyanimId = g2; fallback 'seq_ ' + id (with space before id)
			readyanimID := int(dat.G2())
			readyanim := getByID(seqPack, readyanimID)
			if readyanim == "" {
				// NOTE: TS NpcConfig.ts:83 uses 'seq_ ' + readyanimId (space before numeric id).
				// This is an odd quirk in the TS source — ported verbatim.
				readyanim = "seq_ " + fmt.Sprintf("%d", readyanimID)
			}
			def = append(def, fmt.Sprintf("readyanim=%s", readyanim))

		case code == 14:
			// TS lines 85-89: walkanimId = g2; fallback 'seq_ ' + id (with space before id)
			walkanimID := int(dat.G2())
			walkanim := getByID(seqPack, walkanimID)
			if walkanim == "" {
				// NOTE: TS NpcConfig.ts:88 uses 'seq_ ' + walkanimId (space before numeric id).
				walkanim = "seq_ " + fmt.Sprintf("%d", walkanimID)
			}
			def = append(def, fmt.Sprintf("walkanim=%s", walkanim))

		case code == 16:
			// TS line 90-91: push hasalpha=yes
			def = append(def, "hasalpha=yes")

		case code == 17:
			// TS lines 93-103: four walk anim ids, comma-joined.
			// walkanim fallback uses 'seq_' + id (NO space — NpcConfig.ts:98).
			walkanimID := int(dat.G2())
			walkanimBID := int(dat.G2())
			walkanimLID := int(dat.G2())
			walkanimRID := int(dat.G2())

			walkanim := getByID(seqPack, walkanimID)
			if walkanim == "" {
				walkanim = "seq_" + fmt.Sprintf("%d", walkanimID)
			}
			walkanimB := getByID(seqPack, walkanimBID)
			if walkanimB == "" {
				walkanimB = "seq_" + fmt.Sprintf("%d", walkanimBID)
			}
			walkanimL := getByID(seqPack, walkanimLID)
			if walkanimL == "" {
				walkanimL = "seq_" + fmt.Sprintf("%d", walkanimLID)
			}
			walkanimR := getByID(seqPack, walkanimRID)
			if walkanimR == "" {
				walkanimR = "seq_" + fmt.Sprintf("%d", walkanimRID)
			}

			def = append(def, fmt.Sprintf("walkanim=%s,%s,%s,%s", walkanim, walkanimB, walkanimL, walkanimR))

		case code >= 30 && code < 35:
			// TS lines 104-107: op index = (code-30)+1; gjstr
			index := (code - 30) + 1
			op := dat.GJStrLF()
			def = append(def, fmt.Sprintf("op%d=%s", index, op))

		case code == 40:
			// TS lines 108-114: count = g1; for each pair: recolSrc[i] = g2; recolDst[i] = g2
			count := int(dat.G1())
			recolSrc = make([]int, count)
			recolDst = make([]int, count)
			for i := range count {
				recolSrc[i] = int(dat.G2())
				recolDst[i] = int(dat.G2())
			}

		case code == 60:
			// TS lines 120-137 @2e3bcf43: count = g1; for each: index=i+1; modelId = g2;
			// head model with the same rename guard as code 1.
			count := int(dat.G1())
			for i := range count {
				index := i + 1
				modelID := int(dat.G2())
				modelIDs = append(modelIDs, modelID)
				headName := debugname + "_head"
				var modelName string
				if modelPack != nil {
					if keepPackedName(compare, id, modelID, modelRenameOffset) {
						modelName = modelPack.GetByID(modelID)
					} else {
						modelName = renameModelNpc(modelID, headName, modelPack, srcDir, errorf)
					}
				}
				def = append(def, fmt.Sprintf("head%d=%s", index, modelName))
			}

		case code == 93:
			// TS line 127-128: push minimap=no
			def = append(def, "minimap=no")

		case code == 95:
			// TS lines 129-134: vislevel = g2; 0 → "hide"
			vislevel := int(dat.G2())
			if vislevel == 0 {
				def = append(def, "vislevel=hide")
			} else {
				def = append(def, fmt.Sprintf("vislevel=%d", vislevel))
			}

		case code == 97:
			// TS line 135-137: resizeh = g2
			resizeh := dat.G2()
			def = append(def, fmt.Sprintf("resizeh=%d", resizeh))

		case code == 98:
			// TS line 138-140: resizev = g2
			resizev := dat.G2()
			def = append(def, fmt.Sprintf("resizev=%d", resizev))

		case code == 99:
			// TS line 141-142: push alwaysontop=yes
			def = append(def, "alwaysontop=yes")

		case code == 100:
			// TS line 143-145: ambient = g1b (signed byte)
			ambient := int(int8(dat.G1()))
			def = append(def, fmt.Sprintf("ambient=%d", ambient))

		case code == 101:
			// TS line 146-148: contrast = g1b (signed byte)
			contrast := int(int8(dat.G1()))
			def = append(def, fmt.Sprintf("contrast=%d", contrast))

		case code == 102:
			// TS line 149-151: headicon = g2
			headicon := dat.G2()
			def = append(def, fmt.Sprintf("headicon=%d", headicon))

		case code == 103:
			// TS lines 163-165 @2e3bcf43: turnspeed = g2
			turnspeed := dat.G2()
			def = append(def, fmt.Sprintf("turnspeed=%d", turnspeed))

		default:
			// TS line 153-154: printWarning(`unknown npc code ${code}`)
			warnf("unknown npc code %d", code)
		}
	}

	// TS lines 158-160: incomplete read warning
	if dat.Pos != cfg.Pos[id]+cfg.Len[id] {
		warnf("incomplete read: %d != %d", dat.Pos, cfg.Pos[id]+cfg.Len[id])
	}

	// TS lines 162-185: recol/retex post-processing (dense, threshold=100).
	def = emitRecolDense(def, recolSrc, recolDst, modelIDs, texturePack, models, 100, false)

	return def
}
