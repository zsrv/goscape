package config

import (
	"fmt"

	"github.com/zsrv/goscape/pkg/pack"
	"github.com/zsrv/goscape/pkg/unpack/internal/model"
)

// unpackObj is the core implementation of ObjConfig unpacking.
// Called by Env.UnpackObj; also directly by tests.
//
// compare/modelRenameOffset feed the 254 model-rename guard at every model
// site (see keepPackedName). Pass nil/0 for the pre-254 behavior.
//
// TS source: tools/unpack/config/ObjConfig.ts:41-308 @2e3bcf43.
func unpackObj(
	cfg *ConfigIdx,
	id int,
	compare *ConfigIdx,
	modelRenameOffset int,
	objPack, texturePack *pack.PackFile,
	seqPack *pack.PackFile,
	modelPack *pack.PackFile,
	models *model.Store,
	srcDir string,
	warnf func(string, ...any),
	errorf func(string, ...any),
) []string {
	dat := cfg.Dat

	def := make([]string, 0, 16)
	// TS line 45: debugname = ObjPack.getById(id)
	debugname := getByID(objPack, id)
	def = append(def, fmt.Sprintf("[%s]", debugname))

	// resolveModel applies the 254 rename guard before each renameModel call
	// (TS ObjConfig.ts:64-70 et al @2e3bcf43).
	resolveModel := func(modelID int, name string) string {
		if modelPack == nil {
			return ""
		}
		if keepPackedName(compare, id, modelID, modelRenameOffset) {
			return modelPack.GetByID(modelID)
		}
		return renameModelObj(modelID, name, modelPack, srcDir, errorf)
	}

	var modelIDs []int
	// ObjConfig opcode 40 reads pairs into parallel slices (dense).
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
			// TS lines 59-71: modelId = g2; guarded renameModel; push model=
			modelID := int(dat.G2())
			modelIDs = append(modelIDs, modelID)
			def = append(def, fmt.Sprintf("model=%s", resolveModel(modelID, debugname)))

		case code == 2:
			// TS line 66-68: name = gjstr
			name := dat.GJStrLF()
			def = append(def, fmt.Sprintf("name=%s", name))

		case code == 3:
			// TS line 69-71: desc = gjstr
			desc := dat.GJStrLF()
			def = append(def, fmt.Sprintf("desc=%s", desc))

		case code == 4:
			// TS line 72-73: zoom2d = g2; push 2dzoom=
			zoom2d := dat.G2()
			def = append(def, fmt.Sprintf("2dzoom=%d", zoom2d))

		case code == 5:
			// TS line 74-76: xan2d = g2; push 2dxan=
			xan2d := dat.G2()
			def = append(def, fmt.Sprintf("2dxan=%d", xan2d))

		case code == 6:
			// TS line 77-79: yan2d = g2; push 2dyan=
			yan2d := dat.G2()
			def = append(def, fmt.Sprintf("2dyan=%d", yan2d))

		case code == 7:
			// TS line 80-82: xof2d = g2s (signed); push 2dxof=
			xof2d := int(int16(dat.G2()))
			def = append(def, fmt.Sprintf("2dxof=%d", xof2d))

		case code == 8:
			// TS line 83-85: yof2d = g2s (signed); push 2dyof=
			yof2d := int(int16(dat.G2()))
			def = append(def, fmt.Sprintf("2dyof=%d", yof2d))

		case code == 9:
			// TS line 87-88: push code9=yes
			def = append(def, "code9=yes")

		case code == 10:
			// TS lines 89-93: seqId = g2; lookup or 'seq_' + seqId
			seqID := int(dat.G2())
			seqName := getByID(seqPack, seqID)
			if seqName == "" {
				seqName = fmt.Sprintf("seq_%d", seqID)
			}
			def = append(def, fmt.Sprintf("code10=%s", seqName))

		case code == 11:
			// TS line 94-95: push stackable=yes
			def = append(def, "stackable=yes")

		case code == 12:
			// TS line 96-98: cost = g4s (signed); push cost=
			cost := int(int32(dat.G4()))
			def = append(def, fmt.Sprintf("cost=%d", cost))

		case code == 16:
			// TS line 99-100: push members=yes
			def = append(def, "members=yes")

		case code == 23:
			// TS lines 106-119: modelId = g2; offset = g1b (signed); guarded rename; manwear=model,offset
			modelID := int(dat.G2())
			offset := int(int8(dat.G1()))
			modelIDs = append(modelIDs, modelID)
			def = append(def, fmt.Sprintf("manwear=%s,%d", resolveModel(modelID, debugname+"_manwear"), offset))

		case code == 24:
			// TS lines 120-131: modelId = g2; guarded rename; manwear2=model
			modelID := int(dat.G2())
			modelIDs = append(modelIDs, modelID)
			def = append(def, fmt.Sprintf("manwear2=%s", resolveModel(modelID, debugname+"_manwear2")))

		case code == 25:
			// TS lines 132-145: modelId = g2; offset = g1b (signed); guarded rename; womanwear=model,offset
			modelID := int(dat.G2())
			offset := int(int8(dat.G1()))
			modelIDs = append(modelIDs, modelID)
			def = append(def, fmt.Sprintf("womanwear=%s,%d", resolveModel(modelID, debugname+"_womanwear"), offset))

		case code == 26:
			// TS lines 146-157: modelId = g2; guarded rename; womanwear2=model
			modelID := int(dat.G2())
			modelIDs = append(modelIDs, modelID)
			def = append(def, fmt.Sprintf("womanwear2=%s", resolveModel(modelID, debugname+"_womanwear2")))

		case code >= 30 && code < 35:
			// TS lines 131-134: op index = (code-30)+1; gjstr
			index := (code - 30) + 1
			op := dat.GJStrLF()
			def = append(def, fmt.Sprintf("op%d=%s", index, op))

		case code >= 35 && code < 40:
			// TS lines 135-138: iop index = (code-35)+1; gjstr
			index := (code - 35) + 1
			op := dat.GJStrLF()
			def = append(def, fmt.Sprintf("iop%d=%s", index, op))

		case code == 40:
			// TS lines 139-145: count = g1; for each pair: recolSrc[i]=g2; recolDst[i]=g2
			count := int(dat.G1())
			recolSrc = make([]int, count)
			recolDst = make([]int, count)
			for i := range count {
				recolSrc[i] = int(dat.G2())
				recolDst[i] = int(dat.G2())
			}

		case code == 78:
			// TS lines 170-181: guarded rename; manwear3=model
			modelID := int(dat.G2())
			modelIDs = append(modelIDs, modelID)
			def = append(def, fmt.Sprintf("manwear3=%s", resolveModel(modelID, debugname+"_manwear3")))

		case code == 79:
			// TS lines 182-193: guarded rename; womanwear3=model
			modelID := int(dat.G2())
			modelIDs = append(modelIDs, modelID)
			def = append(def, fmt.Sprintf("womanwear3=%s", resolveModel(modelID, debugname+"_womanwear3")))

		case code == 90:
			// TS lines 194-205: guarded rename; manhead=model
			modelID := int(dat.G2())
			modelIDs = append(modelIDs, modelID)
			def = append(def, fmt.Sprintf("manhead=%s", resolveModel(modelID, debugname+"_manhead")))

		case code == 91:
			// TS lines 206-217: guarded rename; womanhead=model
			modelID := int(dat.G2())
			modelIDs = append(modelIDs, modelID)
			def = append(def, fmt.Sprintf("womanhead=%s", resolveModel(modelID, debugname+"_womanhead")))

		case code == 92:
			// TS lines 218-229: guarded rename; manhead2=model
			modelID := int(dat.G2())
			modelIDs = append(modelIDs, modelID)
			def = append(def, fmt.Sprintf("manhead2=%s", resolveModel(modelID, debugname+"_manhead2")))

		case code == 93:
			// TS lines 230-241: guarded rename; womanhead2=model
			modelID := int(dat.G2())
			modelIDs = append(modelIDs, modelID)
			def = append(def, fmt.Sprintf("womanhead2=%s", resolveModel(modelID, debugname+"_womanhead2")))

		case code == 95:
			// TS line 188-190: zan2d = g2; push 2dzan=
			zan2d := dat.G2()
			def = append(def, fmt.Sprintf("2dzan=%d", zan2d))

		case code == 97:
			// TS lines 191-195: certlink = ObjPack.getById(objId) || 'obj_' + objId
			objID := int(dat.G2())
			obj := getByID(objPack, objID)
			if obj == "" {
				obj = fmt.Sprintf("obj_%d", objID)
			}
			def = append(def, fmt.Sprintf("certlink=%s", obj))

		case code == 98:
			// TS lines 196-200: certtemplate = ObjPack.getById(objId) || 'obj_' + objId
			objID := int(dat.G2())
			obj := getByID(objPack, objID)
			if obj == "" {
				obj = fmt.Sprintf("obj_%d", objID)
			}
			def = append(def, fmt.Sprintf("certtemplate=%s", obj))

		case code >= 100 && code < 110:
			// TS lines 201-207: countN=objName,count
			index := (code - 100) + 1
			objID := int(dat.G2())
			count := int(dat.G2())
			objName := getByID(objPack, objID)
			if objName == "" {
				objName = fmt.Sprintf("obj_%d", objID)
			}
			def = append(def, fmt.Sprintf("count%d=%s,%d", index, objName, count))

		case code == 110:
			// TS line 208-210: resizex = g2
			resizex := dat.G2()
			def = append(def, fmt.Sprintf("resizex=%d", resizex))

		case code == 111:
			// TS line 211-213: resizey = g2
			resizey := dat.G2()
			def = append(def, fmt.Sprintf("resizey=%d", resizey))

		case code == 112:
			// TS line 214-216: resizez = g2
			resizez := dat.G2()
			def = append(def, fmt.Sprintf("resizez=%d", resizez))

		case code == 113:
			// TS line 217-219: ambient = g1b (signed)
			ambient := int(int8(dat.G1()))
			def = append(def, fmt.Sprintf("ambient=%d", ambient))

		case code == 114:
			// TS line 220-222: contrast = g1b (signed)
			contrast := int(int8(dat.G1()))
			def = append(def, fmt.Sprintf("contrast=%d", contrast))

		default:
			// TS line 223-225: printWarning(`unknown obj code ${code}`)
			warnf("unknown obj code %d", code)
		}
	}

	// TS lines 228-230: incomplete read warning
	if dat.Pos != cfg.Pos[id]+cfg.Len[id] {
		warnf("incomplete read: %d != %d", dat.Pos, cfg.Pos[id]+cfg.Len[id])
	}

	// TS lines 232-255: recol/retex post-processing (dense, threshold=100).
	def = emitRecolDense(def, recolSrc, recolDst, modelIDs, texturePack, models, 100, false)

	return def
}
