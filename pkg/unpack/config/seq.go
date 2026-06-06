package config

import (
	"fmt"
	"strings"

	"github.com/zsrv/goscape/pkg/pack"
)

// unpackSeq is the core implementation of SeqConfig unpacking.
// Called by Env.UnpackSeq; also directly by tests.
//
// TS source: tools/unpack/config/SeqConfig.ts:6-138.
func unpackSeq(cfg *ConfigIdx, id int, seqPack, animPack, objPack *pack.PackFile, warnf func(string, ...any)) []string {
	dat := cfg.Dat

	def := make([]string, 0, 8)
	// TS line 10: def.push(`[${SeqPack.getById(id)}]`)
	def = append(def, fmt.Sprintf("[%s]", getByID(seqPack, id)))

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
			// TS lines 19-53: frame table.
			// Read count g1, then for each i: frame g2, iframe g2 (0xffff→-1), delay g2.
			// Then emit frameN= + conditional delayN=, then conditional iframeN= (separate pass).
			count := int(dat.G1())

			frame := make([]int, count)
			iframe := make([]int, count)
			delay := make([]int, count)

			for i := range count {
				// TS line 27: frame[i] = dat.g2()
				frame[i] = int(dat.G2())
				// TS lines 28-31: iframe[i] = dat.g2(); if (iframe[i] === 65535) iframe[i] = -1
				iframe[i] = int(dat.G2())
				if iframe[i] == 65535 {
					iframe[i] = -1
				}
				// TS line 32: delay[i] = dat.g2()
				delay[i] = int(dat.G2())
			}

			// TS lines 35-44: emit frameN= and conditional delayN=
			for i := range count {
				index := i + 1
				frameName := getByID(animPack, frame[i])
				if frameName == "" {
					frameName = fmt.Sprintf("anim_%d", frame[i])
				}
				def = append(def, fmt.Sprintf("frame%d=%s", index, frameName))
				if delay[i] != 0 {
					def = append(def, fmt.Sprintf("delay%d=%d", index, delay[i]))
				}
			}

			// TS lines 46-53: emit conditional iframeN=
			for i := range count {
				index := i + 1
				if iframe[i] != -1 {
					iframeName := getByID(animPack, iframe[i])
					if iframeName == "" {
						iframeName = fmt.Sprintf("anim_%d", iframe[i])
					}
					def = append(def, fmt.Sprintf("iframe%d=%s", index, iframeName))
				}
			}

		case 2:
			// TS lines 54-56: const loops = dat.g2(); def.push(`loops=${loops}`)
			loops := dat.G2()
			def = append(def, fmt.Sprintf("loops=%d", loops))

		case 3:
			// TS lines 57-66: walkmerge label list
			count := int(dat.G1())
			labels := make([]string, count)
			for i := range count {
				walkmerge := int(dat.G1())
				labels[i] = fmt.Sprintf("label_%d", walkmerge)
			}
			def = append(def, fmt.Sprintf("walkmerge=%s", strings.Join(labels, ",")))

		case 4:
			// TS line 67-68
			def = append(def, "stretches=yes")

		case 5:
			// TS lines 69-71: const priority = dat.g1(); def.push(`priority=${priority}`)
			priority := dat.G1()
			def = append(def, fmt.Sprintf("priority=%d", priority))

		case 6:
			// TS lines 72-78: replaceheldleft
			// replaceheldleft = dat.g2(); if === 0 → "hide" else ObjPack.getById(replaceheldleft - 512)
			// NOTE: the value subtracted is the raw wire value, not id. This is what TS does.
			replaceheldleft := int(dat.G2())
			if replaceheldleft == 0 {
				def = append(def, "replaceheldleft=hide")
			} else {
				def = append(def, fmt.Sprintf("replaceheldleft=%s", getByID(objPack, replaceheldleft-512)))
			}

		case 7:
			// TS lines 79-85: replaceheldright
			replaceheldright := int(dat.G2())
			if replaceheldright == 0 {
				def = append(def, "replaceheldright=hide")
			} else {
				def = append(def, fmt.Sprintf("replaceheldright=%s", getByID(objPack, replaceheldright-512)))
			}

		case 8:
			// TS lines 86-88: const maxloops = dat.g1(); def.push(`maxloops=${maxloops}`)
			maxloops := dat.G1()
			def = append(def, fmt.Sprintf("maxloops=%d", maxloops))

		case 9:
			// TS lines 89-100: preanim_move enum
			preanim := int(dat.G1())
			op := seqMoveOp9(preanim)
			def = append(def, fmt.Sprintf("preanim_move=%s", op))

		case 10:
			// TS lines 101-112: postanim_move enum
			postanim := int(dat.G1())
			op := seqMoveOp10(postanim)
			def = append(def, fmt.Sprintf("postanim_move=%s", op))

		case 11:
			// TS lines 113-124: duplicatebehavior enum
			dup := int(dat.G1())
			op := seqDupBehavior(dup)
			def = append(def, fmt.Sprintf("duplicatebehavior=%s", op))

		case 12:
			// TS lines 125-127: const code12 = dat.g4s(); def.push(`code12=${code12}`)
			// TS g4s() is a signed 32-bit read; Go Packet only has G4() (uint32), so cast.
			code12 := int32(dat.G4())
			def = append(def, fmt.Sprintf("code12=%d", code12))

		default:
			// TS line 128-130: printWarning(`unknown seq code ${code}`)
			warnf("unknown seq code %d", code)
		}
	}

	// TS lines 133-135: position-mismatch warning
	if dat.Pos != cfg.Pos[id]+cfg.Len[id] {
		warnf("incomplete read: %d != %d", dat.Pos, cfg.Pos[id]+cfg.Len[id])
	}

	return def
}

// seqMoveOp9 maps preanim_move values to strings.
// TS source: SeqConfig.ts:92-99.
func seqMoveOp9(v int) string {
	switch v {
	case 0:
		return "delaymove"
	case 1:
		return "delayanim"
	case 2:
		return "merge"
	default:
		return fmt.Sprintf("%d", v)
	}
}

// seqMoveOp10 maps postanim_move values to strings.
// TS source: SeqConfig.ts:104-111.
func seqMoveOp10(v int) string {
	switch v {
	case 0:
		return "delaymove"
	case 1:
		return "abortanim"
	case 2:
		return "merge"
	default:
		return fmt.Sprintf("%d", v)
	}
}

// seqDupBehavior maps duplicatebehavior values to strings.
// TS source: SeqConfig.ts:115-123.
// NOTE: TS case 0 maps to the string "0" (not "delaymove" etc.), which is
// intentional — the default branch also calls .toString() giving numeric
// strings, and case 0 is explicitly set to "0" (overrides the default numeric path
// that would also produce "0", so it is a no-op but mirrors TS exactly).
func seqDupBehavior(v int) string {
	switch v {
	case 0:
		return "0"
	case 1:
		return "reset"
	case 2:
		return "reset_loop"
	default:
		return fmt.Sprintf("%d", v)
	}
}
