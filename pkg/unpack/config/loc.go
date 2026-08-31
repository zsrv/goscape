package config

import (
	"fmt"
	"slices"

	"github.com/zsrv/goscape/pkg/pack"
	"github.com/zsrv/goscape/pkg/unpack/internal/model"
)

// LocShapeSuffix maps a loc shape index (0-22) to the two-character model
// name suffix used by the TS renameModel helper.
//
// TS source: LocConfig.ts:131-155 (LocShapeSuffix enum, value=index).
var LocShapeSuffix = [23]string{
	"_1", // 0  wall_straight
	"_2", // 1  wall_diagonalcorner
	"_3", // 2  wall_l
	"_4", // 3  wall_squarecorner
	"_q", // 4  walldecor_straight_nooffset
	"_w", // 5  walldecor_straight_offset
	"_r", // 6  walldecor_diagonal_offset
	"_e", // 7  walldecor_diagonal_nooffset
	"_t", // 8  walldecor_diagonal_both
	"_5", // 9  wall_diagonal
	"_8", // 10 centrepiece_straight
	"_9", // 11 centrepiece_diagonal
	"_a", // 12 roof_straight
	"_s", // 13 roof_diagonal_with_roofedge
	"_d", // 14 roof_diagonal
	"_f", // 15 roof_l_concave
	"_g", // 16 roof_l_convex
	"_h", // 17 roof_flat
	"_z", // 18 roofedge_straight
	"_x", // 19 roofedge_diagonalcorner
	"_c", // 20 roofedge_l
	"_v", // 21 roofedge_squarecorner
	"_0", // 22 grounddecor
}

// LocModelShape holds a model id and its associated shape value.
// TS source: LocConfig.ts:22 (type LocModelShape).
type LocModelShape struct {
	Model int
	Shape int
}

// LocModels is the result of unpackLocModels: models discovered from opcodes
// 1 and 5. The 254 TS dropped the ldModels half entirely (code 77 gone).
// TS source: LocConfig.ts:23 (type LocModels @2e3bcf43).
type LocModels struct {
	Models []LocModelShape
}

// unpackLocModels performs a skip-read pass over a loc entry collecting only
// model-shape pairs from opcodes 1 and 5. All other opcodes are consumed and
// discarded. Called by Env.UnpackLocModels; also directly by tests.
//
// TS source: tools/unpack/config/LocConfig.ts:26-135 @2e3bcf43.
func unpackLocModels(cfg *ConfigIdx, id int, warnf func(string, ...any)) LocModels {
	dat := cfg.Dat
	dat.Pos = cfg.Pos[id]

	var models []LocModelShape

	for {
		code := int(dat.G1())
		if code == 0 {
			break
		}

		switch {
		case code == 1:
			// TS lines 37-50: 1 model per shape — count = g1; for each: model = g2; shape = g1; push
			count := int(dat.G1())
			for range count {
				m := int(dat.G2())
				shape := int(dat.G1())
				models = append(models, LocModelShape{Model: m, Shape: shape})
			}
		case code == 5:
			// TS lines 54-66: multiple models for the default shape — count = g1;
			// for each: model = g2 with the fixed centrepiece shape (_8 = 10).
			count := int(dat.G1())
			for range count {
				m := int(dat.G2())
				models = append(models, LocModelShape{Model: m, Shape: 10})
			}
		case code == 2:
			dat.GJStrLF() // skip name
		case code == 3:
			dat.GJStrLF() // skip desc
		case code == 14:
			dat.G1() // skip width
		case code == 15:
			dat.G1() // skip length
		case code == 17:
			// no-op
		case code == 18:
			// no-op
		case code == 19:
			dat.G1() // skip bool (gbool = g1 != 0)
		case code == 21:
			// no-op
		case code == 22:
			// no-op
		case code == 23:
			// no-op
		case code == 24:
			dat.G2() // skip seqId
		case code == 25:
			// no-op
		case code == 28:
			dat.G1() // skip wallwidth
		case code == 29:
			dat.G1() // skip ambient (g1b)
		case code == 39:
			dat.G1() // skip contrast (g1b)
		case code >= 30 && code < 35:
			dat.GJStrLF() // skip op string
		case code == 40:
			// TS lines 83-88: skip count pairs of g2
			count := int(dat.G1())
			for range count {
				dat.G2()
				dat.G2()
			}
		case code == 60:
			dat.G2() // skip mapfunction
		case code == 62:
			// no-op
		case code == 64:
			// no-op
		case code == 65:
			dat.G2() // skip resizex
		case code == 66:
			dat.G2() // skip resizey
		case code == 67:
			dat.G2() // skip resizez
		case code == 68:
			dat.G2() // skip mapscene
		case code == 69:
			dat.G1() // skip forceapproach flags
		case code == 70:
			dat.G2() // skip offsetx (g2s consumed as g2)
		case code == 71:
			dat.G2() // skip offsety (g2s consumed as g2)
		case code == 72:
			dat.G2() // skip offsetz (g2s consumed as g2)
		case code == 73:
			// no-op
		case code == 74:
			// no-op
		case code == 75:
			dat.G1() // skip bool (gbool)
		default:
			// Code 77 (the old multivariant/ldModels block) is GONE at 254
			// (TS LocConfig.ts @2e3bcf43 no longer handles it) — it lands here.
			// TS LocConfig.ts has no default either and would loop forever on unknown
			// opcodes here (unlike unpackLoc which calls printFatalError). Go bails
			// instead — cannot affect parity because no reference output exists for
			// data that hangs TS.
			warnf("unknown loc model code %d", code)
			return LocModels{Models: models}
		}
	}

	return LocModels{Models: models}
}

// exclusiveAdd appends value to collection only when it is not already present.
//
// TS source: LocConfig.ts:157-161 @2e3bcf43.
func exclusiveAdd(collection []string, value string) []string {
	if slices.Contains(collection, value) {
		return collection
	}
	return append(collection, value)
}

// unpackLoc is the core implementation of LocConfig unpacking.
// Returns a non-nil error when an unknown opcode is encountered.
// Called by Env.UnpackLoc; also directly by tests.
//
// TS printFatalError on unknown opcode → Go returns error.
// All other behaviours mirror TS faithfully.
//
// TS source: tools/unpack/config/LocConfig.ts:157-330.
func unpackLoc(
	cfg *ConfigIdx,
	id int,
	locPack, texturePack *pack.PackFile,
	seqPack *pack.PackFile,
	modelPack *pack.PackFile,
	models *model.Store,
	warnf func(string, ...any),
) ([]string, error) {
	dat := cfg.Dat

	def := make([]string, 0, 16)
	// TS line 161: debugname = LocPack.getById(id)
	debugname := getByID(locPack, id)
	def = append(def, fmt.Sprintf("[%s]", debugname))

	var lastCode int
	var modelIDs []int
	// LocConfig opcode 40 reads dense pairs.
	var recolSrc []int
	var recolDst []int

	// TS line 159: dat.pos = pos[id]
	dat.Pos = cfg.Pos[id]
	for {
		code := int(dat.G1())
		if code == 0 {
			break
		}

		switch {
		case code == 1:
			// TS lines 177-195: model list with shape suffixes + duplicate-name skip.
			// written tracks 1-based output model index; lastName deduplicates consecutive names.
			// DIVERGENCE: TS `let lastName` starts undefined, so an empty-string first name would
			// emit (TS `lastName !== name` is true when lastName=undefined, name=""). Go's
			// `var lastName string` (zero="") skips such an entry. Unreachable with well-formed
			// data — renameModel never returns "" for a real model (TS LocConfig.ts:181-193).
			count := int(dat.G1())
			written := 1
			var lastName string
			for range count {
				modelID := int(dat.G2())
				shape := int(dat.G1())
				modelIDs = append(modelIDs, modelID)

				var name string
				if modelPack != nil {
					name = renameModelLoc(modelID, shape, modelPack)
				}
				// TS lines 189-194: skip if same as lastName (consecutive duplicate).
				if lastName != name {
					if written > 1 {
						def = append(def, fmt.Sprintf("model%d=%s", written, name))
					} else {
						def = append(def, fmt.Sprintf("model=%s", name))
					}
					written++
					lastName = name
				}
			}

		case code == 2:
			// TS line 196-198: name = gjstr
			name := dat.GJStrLF()
			def = append(def, fmt.Sprintf("name=%s", name))

		case code == 3:
			// TS line 199-201: desc = gjstr
			desc := dat.GJStrLF()
			def = append(def, fmt.Sprintf("desc=%s", desc))

		case code == 5:
			// TS lines 213-226 @2e3bcf43: multiple models for the default
			// centrepiece shape — index-suffixed model lines via exclusiveAdd
			// (identical lines deduplicated; first index has no suffix).
			count := int(dat.G1())
			for i := range count {
				index := i + 1
				modelID := int(dat.G2())
				modelIDs = append(modelIDs, modelID)

				var name string
				if modelPack != nil {
					name = renameModelLoc(modelID, 10, modelPack) // LocShapeSuffix._8
				}
				line := fmt.Sprintf("model=%s", name)
				if index > 1 {
					line = fmt.Sprintf("model%d=%s", index, name)
				}
				def = exclusiveAdd(def, line)
			}

		case code == 14:
			// TS line 202-204: width = g1
			width := int(dat.G1())
			def = append(def, fmt.Sprintf("width=%d", width))

		case code == 15:
			// TS line 205-207: length = g1
			length := int(dat.G1())
			def = append(def, fmt.Sprintf("length=%d", length))

		case code == 17:
			// TS line 208-209: push blockwalk=no
			def = append(def, "blockwalk=no")

		case code == 18:
			// TS line 210-211: push blockrange=no
			def = append(def, "blockrange=no")

		case code == 19:
			// TS line 213: active = gbool (g1 === 1, not merely != 0)
			active := dat.GBool()
			if active {
				def = append(def, "active=yes")
			} else {
				def = append(def, "active=no")
			}

		case code == 21:
			// TS line 215-216: push hillskew=yes
			def = append(def, "hillskew=yes")

		case code == 22:
			// TS line 217-218: push sharelight=yes
			def = append(def, "sharelight=yes")

		case code == 23:
			// TS line 219-220: push occlude=yes
			def = append(def, "occlude=yes")

		case code == 24:
			// TS lines 221-224: seqId = g2; seq or 'seq_' + seqId
			seqID := int(dat.G2())
			seq := getByID(seqPack, seqID)
			if seq == "" {
				seq = fmt.Sprintf("seq_%d", seqID)
			}
			def = append(def, fmt.Sprintf("anim=%s", seq))

		case code == 25:
			// TS line 225-226: push hasalpha=yes
			def = append(def, "hasalpha=yes")

		case code == 28:
			// TS lines 229-231: wallwidth = g1
			wallwidth := int(dat.G1())
			def = append(def, fmt.Sprintf("wallwidth=%d", wallwidth))

		case code == 29:
			// TS lines 232-234: ambient = g1b (signed)
			ambient := int(int8(dat.G1()))
			def = append(def, fmt.Sprintf("ambient=%d", ambient))

		case code == 39:
			// TS lines 235-237: contrast = g1b (signed)
			contrast := int(int8(dat.G1()))
			def = append(def, fmt.Sprintf("contrast=%d", contrast))

		case code >= 30 && code < 35:
			// TS lines 237-240: op index = (code-30)+1; gjstr
			index := (code - 30) + 1
			op := dat.GJStrLF()
			def = append(def, fmt.Sprintf("op%d=%s", index, op))

		case code == 40:
			// TS lines 241-247: count = g1; dense pairs
			count := int(dat.G1())
			recolSrc = make([]int, count)
			recolDst = make([]int, count)
			for i := range count {
				recolSrc[i] = int(dat.G2())
				recolDst[i] = int(dat.G2())
			}

		case code == 60:
			// TS lines 248-250: mapfunction = g2
			mapfunction := dat.G2()
			def = append(def, fmt.Sprintf("mapfunction=%d", mapfunction))

		case code == 62:
			// TS line 251-252: push mirror=yes
			def = append(def, "mirror=yes")

		case code == 64:
			// TS line 253-254: push shadow=no
			def = append(def, "shadow=no")

		case code == 65:
			// TS line 255-257: resizex = g2
			resizex := dat.G2()
			def = append(def, fmt.Sprintf("resizex=%d", resizex))

		case code == 66:
			// TS line 258-260: resizey = g2
			resizey := dat.G2()
			def = append(def, fmt.Sprintf("resizey=%d", resizey))

		case code == 67:
			// TS line 261-263: resizez = g2
			resizez := dat.G2()
			def = append(def, fmt.Sprintf("resizez=%d", resizez))

		case code == 68:
			// TS line 264-266: mapscene = g2
			mapscene := dat.G2()
			def = append(def, fmt.Sprintf("mapscene=%d", mapscene))

		case code == 69:
			// TS lines 267-280: forceapproach bitfield decode.
			// Bit 0 clear → "north", bit 1 clear → "east", bit 2 clear → "south", bit 3 clear → "west".
			flags := int(dat.G1())
			forceapproach := ""
			if (flags & 0b0001) == 0 {
				forceapproach = "north"
			} else if (flags & 0b0010) == 0 {
				forceapproach = "east"
			} else if (flags & 0b0100) == 0 {
				forceapproach = "south"
			} else if (flags & 0b1000) == 0 {
				forceapproach = "west"
			}
			def = append(def, fmt.Sprintf("forceapproach=%s", forceapproach))

		case code == 70:
			// TS lines 282-284: offsetx = g2s (signed)
			offsetx := int(int16(dat.G2()))
			def = append(def, fmt.Sprintf("offsetx=%d", offsetx))

		case code == 71:
			// TS lines 285-287: offsety = g2s (signed)
			offsety := int(int16(dat.G2()))
			def = append(def, fmt.Sprintf("offsety=%d", offsety))

		case code == 72:
			// TS lines 288-290: offsetz = g2s (signed)
			offsetz := int(int16(dat.G2()))
			def = append(def, fmt.Sprintf("offsetz=%d", offsetz))

		case code == 73:
			// TS line 291-292: push forcedecor=yes
			def = append(def, "forcedecor=yes")

		case code == 74:
			// TS LocConfig.ts:317-318 (@2e3bcf43): push breakroutefinding=yes
			def = append(def, "breakroutefinding=yes")

		case code == 75:
			// TS LocConfig.ts:319-321 (@2e3bcf43): raiseobject = gbool
			raiseobject := dat.GBool()
			if raiseobject {
				def = append(def, "raiseobject=yes")
			} else {
				def = append(def, "raiseobject=no")
			}

		default:
			// TS line 293-294: printFatalError(`unknown loc code ${code}, last code ${lastCode}`)
			// Go: return error instead of fatal.
			return def, fmt.Errorf("unknown loc code %d, last code %d", code, lastCode)
		}

		lastCode = code
	}

	// TS lines 300-302: incomplete read warning
	if dat.Pos != cfg.Pos[id]+cfg.Len[id] {
		warnf("incomplete read: %d != %d", dat.Pos, cfg.Pos[id]+cfg.Len[id])
	}

	// TS lines 304-327: recol/retex post-processing (dense, threshold=100).
	// LocConfig has a DIFFERENT retex condition: retex when either reverseHsl is
	// undefined OR modelsHaveTexture. (locMode=true enables this path.)
	def = emitRecolDense(def, recolSrc, recolDst, modelIDs, texturePack, models, 100, true)

	return def, nil
}
