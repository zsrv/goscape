package pack

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/zsrv/goscape/pkg/colorconv"
	"github.com/zsrv/goscape/pkg/objtype"
)

// LocShapeSuffix maps a shape number (the TS enum's *value*) to the
// 2-character suffix used in shape-specific model name synthesis.
//
// TS source: tools/pack/config/LocConfig.ts:9-32 (TS enum reverse-map
// lookup via numeric-string indexing).
var LocShapeSuffix = map[int]string{
	0:  "_1",
	1:  "_2",
	2:  "_3",
	3:  "_4",
	4:  "_q",
	5:  "_w",
	6:  "_r",
	7:  "_e",
	8:  "_t",
	9:  "_5",
	10: "_8",
	11: "_9",
	12: "_a",
	13: "_s",
	14: "_d",
	15: "_f",
	16: "_g",
	17: "_h",
	18: "_z",
	19: "_x",
	20: "_c",
	21: "_v",
	22: "_0",
}

// locShapeCentrepieceStraight is shape value 10 (suffix "_8"), the
// centrepiece-straight shape that gets first-pass priority and is the
// fallback for `directReference` resolution.
const locShapeCentrepieceStraight = 10

// locModelShape pairs a resolved model id with its shape value, used to
// stage opcode-1 emission after the model name resolution loop.
//
// TS source: tools/pack/config/PackShared.ts LocModelShape.
type locModelShape struct {
	model int
	shape int
}

// locBooleanKeys is the set of keys parsed as IsConfigBoolean-gated
// booleans. TS source: tools/pack/config/LocConfig.ts:54-60.
var locBooleanKeys = map[string]struct{}{
	"blockwalk":  {},
	"blockrange": {},
	"active":     {},
	"hillskew":   {},
	"sharelight": {},
	"occlude":    {},
	"hasalpha":   {},
	"mirror":     {},
	"shadow":     {},
	"forcedecor": {},
}

// locNumberKeys is the set of keys parsed as signed/unsigned integers
// via TS parseInt (accepts 0x-prefixed hex). TS source: LocConfig.ts:44-52.
var locNumberKeys = map[string]struct{}{
	"width":       {},
	"length":      {},
	"wallwidth":   {},
	"ambient":     {},
	"contrast":    {},
	"mapfunction": {},
	"resizex":     {},
	"resizey":     {},
	"resizez":     {},
	"mapscene":    {},
	"offsetx":     {},
	"offsety":     {},
	"offsetz":     {},
}

// locStringKeys is the set of keys whose raw value passes through as a
// string (subject to the 1000-char limit). TS source: LocConfig.ts:37-42.
var locStringKeys = map[string]struct{}{
	"name":  {},
	"desc":  {},
	"op1":   {},
	"op2":   {},
	"op3":   {},
	"op4":   {},
	"op5":   {},
	"model": {},
}

// parseLocConfigFor returns the per-key=value parser for .loc config
// blocks. Closure-captures three name-map registries plus paramLookups +
// ParamTypeConfigs for `param=` resolution. `model{N}` values pass
// through raw — model resolution is deferred to the packer
// (resolveLocModels), matching TS LocConfig.ts:41.
//
// TS source: tools/pack/config/LocConfig.ts:35-168.
func parseLocConfigFor(categoryPack, seqPack, texturePack *PackFile, lk *paramLookups, paramTypes *objtype.ParamTypeConfigs) ParseFn {
	return func(key, value string) (ConfigValue, bool, error) {
		// Strict-named simple cases first.

		if _, ok := locStringKeys[key]; ok {
			if len(value) > 1000 {
				return nil, true, fmt.Errorf("%s value exceeds 1000 chars", key)
			}
			return value, true, nil
		}

		if _, ok := locNumberKeys[key]; ok {
			n, err := strconv.ParseInt(value, 0, 64)
			if err != nil {
				return nil, true, fmt.Errorf("invalid number for %s: %s: %w", key, value, ErrInvalidNumber)
			}
			return int(n), true, nil
		}

		if _, ok := locBooleanKeys[key]; ok {
			if !IsConfigBoolean(value) {
				return nil, true, fmt.Errorf("invalid boolean for %s: %s: %w", key, value, ErrInvalidBoolean)
			}
			return GetConfigBoolean(value), true, nil
		}

		// Variadic-suffix and lookup cases.

		if strings.HasPrefix(key, "recol") && len(key) >= 6 {
			idxChar := key[5]
			if idxChar < '0' || idxChar > '9' {
				return nil, false, nil
			}
			n, err := strconv.ParseInt(value, 0, 64)
			if err != nil {
				return nil, true, fmt.Errorf("invalid recol value: %s: %w", value, ErrInvalidRecol)
			}
			return colorconv.Rgb15toHsl16(int(n)), true, nil
		}

		if strings.HasPrefix(key, "retex") && len(key) >= 6 {
			idxChar := key[5]
			if idxChar < '0' || idxChar > '9' {
				return nil, false, nil
			}
			texIdx := texturePack.GetByName(value)
			if texIdx == -1 {
				return nil, true, fmt.Errorf("unknown texture: %s", value)
			}
			return texIdx, true, nil
		}

		switch key {
		case "category":
			idx := categoryPack.GetByName(value)
			if idx == -1 {
				return nil, true, fmt.Errorf("unknown category: %s", value)
			}
			return idx, true, nil
		case "anim":
			idx := seqPack.GetByName(value)
			if idx == -1 {
				return nil, true, fmt.Errorf("unknown anim: %s: %w", value, ErrUnknownAnim)
			}
			return idx, true, nil
		case "param":
			name, vstr, ok := strings.Cut(value, ",")
			if !ok {
				return nil, true, fmt.Errorf("param missing comma: %s", value)
			}
			pid, found := paramTypes.ConfigNames[name]
			if !found {
				return nil, true, fmt.Errorf("unknown param: %s: %w", name, ErrUnknownParam)
			}
			pt := paramTypes.Configs[pid]
			if pt == nil {
				return nil, true, fmt.Errorf("unknown param: %s: %w", name, ErrUnknownParam)
			}
			v, err := lookupParamValue(pt.Type, vstr, lk)
			if err != nil {
				return nil, true, fmt.Errorf("param %s value: %w", name, err)
			}
			return ParamValue{ID: pt.ID, Type: pt.Type, Value: v}, true, nil
		case "forceapproach":
			flags := 0b1111
			switch value {
			case "north":
				flags &^= 0b0001
			case "east":
				flags &^= 0b0010
			case "south":
				flags &^= 0b0100
			case "west":
				flags &^= 0b1000
			}
			return flags, true, nil
		}

		// `model{N}` (e.g. model1, model2, ...) — TS uses startsWith('model').
		// Resolution deferred to packer; just return raw value.
		if strings.HasPrefix(key, "model") {
			if len(value) > 1000 {
				return nil, true, fmt.Errorf("model value exceeds 1000 chars")
			}
			return value, true, nil
		}

		// `op{N}` — handled by locStringKeys above for op1..op5. Anything
		// starting with "op" beyond op1..op5 falls through to unknown.

		return nil, false, nil
	}
}

// resolveLocModels mirrors the TS model-resolution loop at
// LocConfig.ts:319-360. For each raw model name in srcModels:
//   - probe modelPack for an exact match, then for each shape suffix
//   - if only the exact match exists (no shape-suffix variants),
//     force the model to shape _8 (centrepiece_straight)
//   - otherwise probe `_8` first and then all other shapes in order
//
// Returns the (model, shape) list to feed opcode 1, plus a flag
// indicating whether any non-_8 shape was synthesised (used for the
// "any _8 shape forces name transmit" downstream check).
func resolveLocModels(srcModels []string, modelPack *PackFile, debugname string) ([]locModelShape, error) {
	var models []locModelShape
	for _, raw := range srcModels {
		directReference := modelPack.GetByName(raw) != -1
		for shape := 0; shape <= 22; shape++ {
			if shape == locShapeCentrepieceStraight {
				continue
			}
			if modelPack.GetByName(raw+LocShapeSuffix[shape]) != -1 {
				directReference = false
				break
			}
		}

		if directReference {
			forceID := modelPack.GetByName(raw)
			if forceID != -1 {
				models = append(models, locModelShape{model: forceID, shape: locShapeCentrepieceStraight})
				continue
			}
		}

		// centrepiece_straight (`_8`) first.
		if id := modelPack.GetByName(raw + "_8"); id != -1 {
			models = append(models, locModelShape{model: id, shape: locShapeCentrepieceStraight})
		}
		for shape := 0; shape <= 22; shape++ {
			if shape == locShapeCentrepieceStraight {
				continue
			}
			if id := modelPack.GetByName(raw + LocShapeSuffix[shape]); id != -1 {
				models = append(models, locModelShape{model: id, shape: shape})
			}
		}
	}

	if len(srcModels) > 0 && len(models) == 0 {
		return nil, fmt.Errorf("%s: failed to find suitable loc models", debugname)
	}
	return models, nil
}

// packLocConfigs walks each id ∈ [0, locPack.Max), emitting per-id
// bodies into separate server + client PackedData buffers per
// LocConfig.ts:170-434.
//
// Server gets: opcode 61 (category), opcode 249 (params), opcode 250
// (debugname). Client gets everything else (1, 2, 3, 14-73).
//
// modelFlags is indexed by model id (size = Model PackFile max). The loc
// packer writes 0x4 flags for model references (T6+). The parameter is
// accepted here for plumbing parity with TS PackShared.ts:137-141; no
// writes land in T5.
//
// TS source: tools/pack/config/LocConfig.ts:170-434.
func packLocConfigs(configs map[string][]ConfigLine, locPack, modelPack *PackFile, modelFlags []int) (server, client *PackedData, err error) {
	server = NewPackedData(locPack.Max)
	client = NewPackedData(locPack.Max)

	for id := range locPack.Max {
		debugname := locPack.GetByID(id)
		if cfg, ok := configs[debugname]; ok {
			// First pass collectors (mirror TS L180-186).
			var (
				recolS    []int
				recolD    []int
				srcModels []string
				name      *string
				active    = -1
				desc      *string
				params    []ParamValue
			)

			// Walk lines, emitting client-side opcodes inline where TS does,
			// and staging recol/model/param/name/active/desc for trailers.
			for _, line := range cfg {
				switch {
				case line.Key == "name":
					s := line.Value.(string)
					name = &s
				case line.Key == "desc":
					s := line.Value.(string)
					desc = &s
				case strings.HasPrefix(line.Key, "model"):
					srcModels = append(srcModels, line.Value.(string))
				case strings.HasPrefix(line.Key, "recol"):
					indexCh := line.Key[5]
					idx := int(indexCh-'0') - 1
					n := line.Value.(int)
					if strings.HasSuffix(line.Key, "s") {
						for len(recolS) <= idx {
							recolS = append(recolS, 0)
						}
						recolS[idx] = n
					} else {
						for len(recolD) <= idx {
							recolD = append(recolD, 0)
						}
						recolD[idx] = n
					}
				case strings.HasPrefix(line.Key, "retex"):
					// Retextures pre-rev-465 share the recol slot.
					indexCh := line.Key[5]
					idx := int(indexCh-'0') - 1
					n := line.Value.(int)
					if strings.HasSuffix(line.Key, "s") {
						for len(recolS) <= idx {
							recolS = append(recolS, 0)
						}
						recolS[idx] = n
					} else {
						for len(recolD) <= idx {
							recolD = append(recolD, 0)
						}
						recolD[idx] = n
					}
				case line.Key == "param":
					params = append(params, line.Value.(ParamValue))
				case line.Key == "width":
					client.P1(14)
					client.P1(uint8(line.Value.(int)))
				case line.Key == "length":
					client.P1(15)
					client.P1(uint8(line.Value.(int)))
				case line.Key == "blockwalk":
					if !line.Value.(bool) {
						client.P1(17)
					}
				case line.Key == "blockrange":
					if !line.Value.(bool) {
						client.P1(18)
					}
				case line.Key == "active":
					v := line.Value.(bool)
					client.P1(19)
					client.PBool(v)
					if v {
						active = 1
					} else {
						active = 0
					}
				case line.Key == "hillskew":
					if line.Value.(bool) {
						client.P1(21)
					}
				case line.Key == "sharelight":
					if line.Value.(bool) {
						client.P1(22)
					}
				case line.Key == "occlude":
					if line.Value.(bool) {
						client.P1(23)
					}
				case line.Key == "anim":
					client.P1(24)
					client.P2(uint16(line.Value.(int)))
				case line.Key == "hasalpha":
					if line.Value.(bool) {
						client.P1(25)
					}
				case line.Key == "wallwidth":
					client.P1(28)
					client.P1(uint8(line.Value.(int)))
				case line.Key == "ambient":
					client.P1(29)
					client.P1(uint8(line.Value.(int)))
				case line.Key == "contrast":
					client.P1(39)
					client.P1(uint8(line.Value.(int)))
				case strings.HasPrefix(line.Key, "op"):
					// op1..op5 → opcode 30 + (N-1)
					nStr := line.Key[2:]
					n, perr := strconv.Atoi(nStr)
					if perr != nil {
						return nil, nil, fmt.Errorf("%s: invalid op key: %s", debugname, line.Key)
					}
					client.P1(uint8(30 + n - 1))
					client.PJStr(line.Value.(string))
				case line.Key == "mapfunction":
					client.P1(60)
					client.P2(uint16(line.Value.(int)))
				case line.Key == "category":
					server.P1(61)
					server.P2(uint16(line.Value.(int)))
				case line.Key == "mirror":
					if line.Value.(bool) {
						client.P1(62)
					}
				case line.Key == "shadow":
					if !line.Value.(bool) {
						client.P1(64)
					}
				case line.Key == "resizex":
					client.P1(65)
					client.P2(uint16(line.Value.(int)))
				case line.Key == "resizey":
					client.P1(66)
					client.P2(uint16(line.Value.(int)))
				case line.Key == "resizez":
					client.P1(67)
					client.P2(uint16(line.Value.(int)))
				case line.Key == "mapscene":
					client.P1(68)
					client.P2(uint16(line.Value.(int)))
				case line.Key == "forceapproach":
					client.P1(69)
					client.P1(uint8(line.Value.(int)))
				case line.Key == "offsetx":
					client.P1(70)
					client.P2(uint16(line.Value.(int)))
				case line.Key == "offsety":
					client.P1(71)
					client.P2(uint16(line.Value.(int)))
				case line.Key == "offsetz":
					client.P1(72)
					client.P2(uint16(line.Value.(int)))
				case line.Key == "forcedecor":
					if line.Value.(bool) {
						client.P1(73)
					}
				}
			}

			// recol trailer: opcode 40 + p1(count) + per-entry p2 p2.
			if len(recolS) > 0 {
				client.P1(40)
				client.P1(uint8(len(recolS)))
				for k := range recolS {
					client.P2(uint16(recolS[k]))
					var d int
					if k < len(recolD) {
						d = recolD[k]
					}
					client.P2(uint16(d))
				}
			}

			// Model resolution and opcode 1 emission.
			models, mErr := resolveLocModels(srcModels, modelPack, debugname)
			if mErr != nil {
				return nil, nil, mErr
			}
			if len(models) > 0 {
				client.P1(1)
				client.P1(uint8(len(models)))
				for _, m := range models {
					client.P2(uint16(m.model))
					client.P1(uint8(m.shape))
				}
			}

			// Name forced-transmit edge case (TS L376-394).
			if name == nil && active != 0 {
				shouldTransmit := active == 1
				if active == -1 {
					for _, m := range models {
						if m.shape == locShapeCentrepieceStraight {
							shouldTransmit = true
							break
						}
					}
				}
				if shouldTransmit {
					n := debugname
					name = &n
				}
			}

			if name != nil {
				client.P1(2)
				client.PJStr(*name)
			}

			if desc != nil {
				client.P1(3)
				client.PJStr(*desc)
			}

			// Params trailer (server).
			if len(params) > 0 {
				server.P1(249)
				server.P1(uint8(len(params)))
				for _, p := range params {
					server.P3(uint32(p.ID))
					isString := p.Type == objtype.ScriptVarTypeString
					server.PBool(isString)
					if isString {
						server.PJStr(p.Value.(string))
					} else {
						server.P4(uint32(p.Value.(int)))
					}
				}
			}
		}

		// Debugname trailer always on server when slot is named.
		if len(debugname) > 0 {
			server.P1(250)
			server.PJStr(debugname)
		}

		client.Next()
		server.Next()
	}

	return server, client, nil
}
