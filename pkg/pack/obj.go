package pack

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/zsrv/goscape/pkg/colorconv"
	"github.com/zsrv/goscape/pkg/objtype"
)

// objStringKeys is the set of keys parsed as raw strings (subject to
// the 1000-char limit). TS source: ObjConfig.ts:12-16.
var objStringKeys = map[string]struct{}{
	"name": {},
	"desc": {},
	"op1":  {}, "op2": {}, "op3": {}, "op4": {}, "op5": {},
	"iop1": {}, "iop2": {}, "iop3": {}, "iop4": {}, "iop5": {},
}

// objNumberKeys is the set of keys parsed as signed/unsigned integers
// via TS parseInt (accepts 0x-prefixed hex). TS source: ObjConfig.ts:18-21.
var objNumberKeys = map[string]struct{}{
	"2dzoom":      {},
	"2dxan":       {},
	"2dyan":       {},
	"2dxof":       {},
	"2dyof":       {},
	"2dzan":       {},
	"cost":        {},
	"respawnrate": {},
}

// objBooleanKeys is the set of keys parsed as IsConfigBoolean-gated
// booleans. TS source: ObjConfig.ts:23-25.
var objBooleanKeys = map[string]struct{}{
	"code9":     {},
	"stackable": {},
	"members":   {},
	"tradeable": {},
}

// objModelKeys is the set of keys whose value is a model name resolved
// against modelPack (returns the integer id). TS source: ObjConfig.ts:63.
var objModelKeys = map[string]struct{}{
	"model":      {},
	"manwear2":   {},
	"womanwear2": {},
	"manwear3":   {},
	"womanwear3": {},
	"manhead":    {},
	"womanhead":  {},
	"manhead2":   {},
	"womanhead2": {},
}

// objWearPosKeys is the set of keys whose value is one of the wearpos
// names (resolved by objWearPosID). TS source: ObjConfig.ts:84.
var objWearPosKeys = map[string]struct{}{
	"wearpos":  {},
	"wearpos2": {},
	"wearpos3": {},
}

// objWearPosID mirrors ObjType.getWearPosId at
// src/cache/config/ObjType.ts:99-132. Returns -1 for unknown names.
func objWearPosID(name string) int {
	switch name {
	case "hat":
		return 0
	case "back":
		return 1
	case "front":
		return 2
	case "righthand":
		return 3
	case "torso":
		return 4
	case "lefthand":
		return 5
	case "arms":
		return 6
	case "legs":
		return 7
	case "head":
		return 8
	case "hands":
		return 9
	case "feet":
		return 10
	case "jaw":
		return 11
	case "ring":
		return 12
	case "quiver":
		return 13
	}
	return -1
}

// objManWomanWearPair is the parsed value of manwear=/womanwear= lines.
// TS returns the JS array [model, offset] (ObjConfig.ts:107). Goscape
// uses a small struct so the packer can type-assert one value cleanly.
type objManWomanWearPair struct {
	Model  int
	Offset int
}

// objCountPair is the parsed value of count{N}= lines.
// TS returns the JS array [obj, count] (ObjConfig.ts:157). Goscape uses
// a small struct so the packer can type-assert one value cleanly.
type objCountPair struct {
	Obj   int
	Count int
}

// parseObjConfigFor returns the per-key=value parser for .obj config
// blocks. Closure-captures four name-map registries plus paramLookups +
// ParamTypeConfigs for `param=` resolution.
//
// TS source: tools/pack/config/ObjConfig.ts:10-193.
func parseObjConfigFor(modelPack, categoryPack, seqPack, objPack *PackFile, lk *paramLookups, paramTypes *objtype.ParamTypeConfigs) ParseFn {
	return func(key, value string) (ConfigValue, bool, error) {
		// Strict-named simple cases first.

		if _, ok := objStringKeys[key]; ok {
			if len(value) > 1000 {
				return nil, true, fmt.Errorf("%s value exceeds 1000 chars", key)
			}
			return value, true, nil
		}

		if _, ok := objNumberKeys[key]; ok {
			n, err := strconv.ParseInt(value, 0, 64)
			if err != nil {
				return nil, true, fmt.Errorf("invalid number for %s: %s: %w", key, value, ErrInvalidNumber)
			}
			return int(n), true, nil
		}

		if _, ok := objBooleanKeys[key]; ok {
			if !IsConfigBoolean(value) {
				return nil, true, fmt.Errorf("invalid boolean for %s: %s: %w", key, value, ErrInvalidBoolean)
			}
			return GetConfigBoolean(value), true, nil
		}

		if _, ok := objModelKeys[key]; ok {
			idx := modelPack.GetByName(value)
			if idx == -1 {
				return nil, true, fmt.Errorf("unknown model: %s: %w", value, ErrUnknownModel)
			}
			return idx, true, nil
		}

		if _, ok := objWearPosKeys[key]; ok {
			idx := objWearPosID(value)
			if idx == -1 {
				return nil, true, fmt.Errorf("unknown wearpos: %s", value)
			}
			return idx, true, nil
		}

		// Variadic-suffix cases.

		if strings.HasPrefix(key, "recol") && len(key) >= 6 {
			idxChar := key[5]
			if idxChar < '0' || idxChar > '9' {
				return nil, false, nil
			}
			// TS guards `index > 9` → reject (ObjConfig.ts:71-73). Index is
			// the raw char-as-digit, so any single digit 0..9 passes.
			n, err := strconv.ParseInt(value, 0, 64)
			if err != nil {
				return nil, true, fmt.Errorf("invalid recol value: %s: %w", value, ErrInvalidRecol)
			}
			return int(n), true, nil
		}

		if strings.HasPrefix(key, "count") && len(key) > len("count") {
			// count{N}=objname,count → [obj, count]
			parts := strings.SplitN(value, ",", 2)
			if len(parts) < 2 {
				return nil, true, fmt.Errorf("count expects 'obj,count': %s", value)
			}
			obj := objPack.GetByName(parts[0])
			if obj == -1 {
				return nil, true, fmt.Errorf("unknown obj: %s: %w", parts[0], ErrUnknownObj)
			}
			c, err := strconv.ParseInt(parts[1], 10, 64)
			if err != nil || c < 1 || c > 65535 {
				return nil, true, fmt.Errorf("invalid count: %s", parts[1])
			}
			return objCountPair{Obj: obj, Count: int(c)}, true, nil
		}

		switch key {
		case "code10":
			idx := seqPack.GetByName(value)
			if idx == -1 {
				return nil, true, fmt.Errorf("unknown seq: %s: %w", value, ErrUnknownSeq)
			}
			return idx, true, nil

		case "manwear", "womanwear":
			parts := strings.SplitN(value, ",", 2)
			if len(parts) < 2 {
				return nil, true, fmt.Errorf("%s expects 'model,offset': %s", key, value)
			}
			model := modelPack.GetByName(parts[0])
			if model == -1 {
				return nil, true, fmt.Errorf("unknown model: %s: %w", parts[0], ErrUnknownModel)
			}
			offset, err := strconv.ParseInt(parts[1], 10, 64)
			if err != nil {
				return nil, true, fmt.Errorf("invalid offset: %s", parts[1])
			}
			return objManWomanWearPair{Model: model, Offset: int(offset)}, true, nil

		case "weight":
			// Resolve unit suffix → grams. TS source: ObjConfig.ts:108-130.
			var grams float64
			switch {
			case strings.Contains(value, "kg"):
				numStr, _, _ := strings.Cut(value, "kg")
				f, err := strconv.ParseFloat(numStr, 64)
				if err != nil {
					return nil, true, fmt.Errorf("invalid weight: %s", value)
				}
				grams = f * 1000
			case strings.Contains(value, "oz"):
				numStr, _, _ := strings.Cut(value, "oz")
				f, err := strconv.ParseFloat(numStr, 64)
				if err != nil {
					return nil, true, fmt.Errorf("invalid weight: %s", value)
				}
				grams = f * 28.3495
			case strings.Contains(value, "lb"):
				numStr, _, _ := strings.Cut(value, "lb")
				f, err := strconv.ParseFloat(numStr, 64)
				if err != nil {
					return nil, true, fmt.Errorf("invalid weight: %s", value)
				}
				grams = f * 453.592
			case strings.Contains(value, "g"):
				numStr, _, _ := strings.Cut(value, "g")
				f, err := strconv.ParseFloat(numStr, 64)
				if err != nil {
					return nil, true, fmt.Errorf("invalid weight: %s", value)
				}
				grams = f
			default:
				return nil, true, fmt.Errorf("invalid weight (missing unit): %s", value)
			}
			ig := int(grams)
			if ig < -32768 || ig > 32767 {
				return nil, true, fmt.Errorf("weight out of range: %d", ig)
			}
			return ig, true, nil

		case "dummyitem":
			switch value {
			case "graphic_only":
				return 1, true, nil
			case "inv_only":
				return 2, true, nil
			}
			return nil, true, fmt.Errorf("invalid dummyitem: %s", value)

		case "category":
			idx := categoryPack.GetByName(value)
			if idx == -1 {
				return nil, true, fmt.Errorf("unknown category: %s", value)
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

		case "certlink", "certtemplate":
			idx := objPack.GetByName(value)
			if idx == -1 {
				return nil, true, fmt.Errorf("unknown obj: %s: %w", value, ErrUnknownObj)
			}
			return idx, true, nil
		}

		return nil, false, nil
	}
}

// packObjConfigs walks each id ∈ [0, objPack.Max), emitting per-id
// bodies into separate server + client PackedData buffers per
// ObjConfig.ts:195-444.
//
// Cert/uncert pairing: when debugname starts with "cert_", the config
// slice is REPLACED with [{certlink, uncertID}, {certtemplate,
// template_for_cert_ID}] before per-key emit. Reverse-lookup ("cert_"+
// debugname → opcode 97 on server) runs at the end of the per-id loop
// for non-cert names (TS:389-394).
//
// Server gets: opcodes 13/14/15/27 (wearpos/tradeable), 75 (weight),
// 94 (category), 96 (dummyitem), 97-reverse-lookup, 201 (respawnrate),
// 249 (params), 250 (debugname). Client gets all other client-facing
// fields including the certlink/certtemplate emits at 97/98.
//
// TS source: tools/pack/config/ObjConfig.ts:195-444.
func packObjConfigs(configs map[string][]ConfigLine, objPack *PackFile) (server, client *PackedData, err error) {
	server = NewPackedData(objPack.Max)
	client = NewPackedData(objPack.Max)

	templateForCert := objPack.GetByName("template_for_cert")
	// TS L200-202 prints a warning ("necessary template_for_cert does
	// not exist") but does not fail. Goscape silently tolerates -1; the
	// emit path only fires for cert_ debugnames, which require this id.

	for id := range objPack.Max {
		debugname := objPack.GetByID(id)

		var cfg []ConfigLine
		isCert := strings.HasPrefix(debugname, "cert_")
		if isCert {
			uncert := objPack.GetByName(debugname[len("cert_"):])
			if uncert == -1 {
				return nil, nil, fmt.Errorf("%s: Cert does not link to anything based on its name.", debugname)
			}
			cfg = []ConfigLine{
				{Key: "certlink", Value: uncert},
				{Key: "certtemplate", Value: templateForCert},
			}
		} else {
			cfg = configs[debugname]
			// TS L222-240: if config has model= but no name=, synthesise
			// a name from debugname ("first letter upper + underscores → spaces").
			if cfg != nil {
				hasName := false
				hasModel := false
				for _, line := range cfg {
					switch line.Key {
					case "name":
						hasName = true
					case "model":
						hasModel = true
					}
				}
				if !hasName && hasModel && len(debugname) > 0 {
					synthesised := strings.ToUpper(debugname[:1]) + strings.ReplaceAll(debugname[1:], "_", " ")
					cfg = append(cfg, ConfigLine{Key: "name", Value: synthesised})
				}
			}
		}

		if cfg != nil {
			// First-pass collectors (mirror TS L245-248).
			var (
				recolS []int
				recolD []int
				name   *string
				params []ParamValue
			)

			for _, line := range cfg {
				key := line.Key

				switch {
				case key == "name":
					s := line.Value.(string)
					name = &s
				case strings.HasPrefix(key, "recol"):
					// recol{N}s / recol{N}d — index = digit at key[5], minus 1
					// for the canonical 1-based numbering.
					indexCh := key[5]
					idx := int(indexCh-'0') - 1
					n := line.Value.(int)
					if strings.HasSuffix(key, "s") {
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
				case key == "param":
					params = append(params, line.Value.(ParamValue))
				case key == "model":
					client.P1(1)
					client.P2(uint16(line.Value.(int)))
				case key == "desc":
					client.P1(3)
					client.PJStr(line.Value.(string))
				case key == "2dzoom":
					client.P1(4)
					client.P2(uint16(line.Value.(int)))
				case key == "2dxan":
					client.P1(5)
					client.P2(uint16(line.Value.(int)))
				case key == "2dyan":
					client.P1(6)
					client.P2(uint16(line.Value.(int)))
				case key == "2dxof":
					client.P1(7)
					client.P2(uint16(line.Value.(int)))
				case key == "2dyof":
					client.P1(8)
					client.P2(uint16(line.Value.(int)))
				case key == "code9":
					if line.Value.(bool) {
						client.P1(9)
					}
				case key == "code10":
					client.P1(10)
					client.P2(uint16(line.Value.(int)))
				case key == "stackable":
					if line.Value.(bool) {
						client.P1(11)
					}
				case key == "cost":
					client.P1(12)
					client.P4(uint32(line.Value.(int)))
				case key == "wearpos":
					server.P1(13)
					server.P1(uint8(line.Value.(int)))
				case key == "wearpos2":
					server.P1(14)
					server.P1(uint8(line.Value.(int)))
				case key == "tradeable":
					if !line.Value.(bool) {
						server.P1(15)
					}
				case key == "members":
					if line.Value.(bool) {
						client.P1(16)
					}
				case key == "manwear":
					pair := line.Value.(objManWomanWearPair)
					client.P1(23)
					client.P2(uint16(pair.Model))
					client.P1(uint8(pair.Offset))
				case key == "manwear2":
					client.P1(24)
					client.P2(uint16(line.Value.(int)))
				case key == "womanwear":
					pair := line.Value.(objManWomanWearPair)
					client.P1(25)
					client.P2(uint16(pair.Model))
					client.P1(uint8(pair.Offset))
				case key == "womanwear2":
					client.P1(26)
					client.P2(uint16(line.Value.(int)))
				case key == "wearpos3":
					server.P1(27)
					server.P1(uint8(line.Value.(int)))
				case strings.HasPrefix(key, "iop"):
					n, perr := strconv.Atoi(key[3:])
					if perr != nil {
						return nil, nil, fmt.Errorf("%s: invalid iop key: %s", debugname, key)
					}
					client.P1(uint8(35 + n - 1))
					client.PJStr(line.Value.(string))
				case strings.HasPrefix(key, "op"):
					n, perr := strconv.Atoi(key[2:])
					if perr != nil {
						return nil, nil, fmt.Errorf("%s: invalid op key: %s", debugname, key)
					}
					client.P1(uint8(30 + n - 1))
					client.PJStr(line.Value.(string))
				case key == "weight":
					server.P1(75)
					server.P2(uint16(int16(line.Value.(int))))
				case key == "manwear3":
					client.P1(78)
					client.P2(uint16(line.Value.(int)))
				case key == "womanwear3":
					client.P1(79)
					client.P2(uint16(line.Value.(int)))
				case key == "manhead":
					client.P1(90)
					client.P2(uint16(line.Value.(int)))
				case key == "womanhead":
					client.P1(91)
					client.P2(uint16(line.Value.(int)))
				case key == "manhead2":
					client.P1(92)
					client.P2(uint16(line.Value.(int)))
				case key == "womanhead2":
					client.P1(93)
					client.P2(uint16(line.Value.(int)))
				case key == "category":
					server.P1(94)
					server.P2(uint16(line.Value.(int)))
				case key == "2dzan":
					client.P1(95)
					client.P2(uint16(line.Value.(int)))
				case key == "dummyitem":
					server.P1(96)
					server.P1(uint8(line.Value.(int)))
				case key == "certlink":
					client.P1(97)
					client.P2(uint16(line.Value.(int)))
				case key == "certtemplate":
					client.P1(98)
					client.P2(uint16(line.Value.(int)))
				case strings.HasPrefix(key, "count"):
					n, perr := strconv.Atoi(key[5:])
					if perr != nil {
						return nil, nil, fmt.Errorf("%s: invalid count key: %s", debugname, key)
					}
					pair := line.Value.(objCountPair)
					client.P1(uint8(100 + n - 1))
					client.P2(uint16(pair.Obj))
					client.P2(uint16(pair.Count))
				case key == "respawnrate":
					server.P1(201)
					server.P2(uint16(line.Value.(int)))
				}
			}

			// Reverse-lookup the certificate (TS L389-394). Only fires for
			// NON-cert names (cfg above is the synthesised pair for cert_).
			//
			// NAI-196-D-CERT-REVLOOKUP: TS enters the reverse-lookup block for
			// cert_ names (config is the synthesised pair = truthy) but gets -1
			// (no "cert_cert_X" names exist) and silently skips. Goscape guards
			// with !isCert for clarity; output is identical.
			if !isCert {
				cert := objPack.GetByName("cert_" + debugname)
				if cert != -1 {
					server.P1(97)
					server.P2(uint16(cert))
				}
			}

			// recol trailer (TS L396-409): opcode 40 + p1(count) + per
			// k: p2(s) + p2(d), with rgb15→hsl16 conversion when s>=100 or d>=100.
			if len(recolS) > 0 {
				client.P1(40)
				client.P1(uint8(len(recolS)))
				for k := range recolS {
					s := recolS[k]
					var d int
					if k < len(recolD) {
						d = recolD[k]
					}
					if s >= 100 || d >= 100 {
						client.P2(uint16(colorconv.Rgb15toHsl16(s)))
						client.P2(uint16(colorconv.Rgb15toHsl16(d)))
					} else {
						client.P2(uint16(s))
						client.P2(uint16(d))
					}
				}
			}

			// Name trailer (TS L411-414).
			if name != nil {
				client.P1(2)
				client.PJStr(*name)
			}

			// Params trailer (server, TS L416-431).
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

		// Debugname trailer on server when slot is named (TS L434-437).
		if len(debugname) > 0 {
			server.P1(250)
			server.PJStr(debugname)
		}

		client.Next()
		server.Next()
	}

	return server, client, nil
}
