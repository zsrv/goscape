package pack

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/zsrv/goscape/pkg/colorconv"
	"github.com/zsrv/goscape/pkg/objtype"
)

// npcStringKeys is the set of keys parsed as raw strings (subject to the
// 1000-char limit). TS source: NpcConfig.ts:13-16.
var npcStringKeys = map[string]struct{}{
	"name": {},
	"desc": {},
	"op1":  {}, "op2": {}, "op3": {}, "op4": {}, "op5": {},
}

// npcNumberKeys is the set of keys parsed as signed/unsigned integers via
// TS parseInt (accepts 0x-prefixed hex). turnspeed joined at rev-254.
// TS source: NpcConfig.ts:18-29 @ 2e3bcf43.
//
// Range constraints (TS L61-87) are honoured by the parser.
var npcNumberKeys = map[string]struct{}{
	"size":        {},
	"resizex":     {},
	"resizey":     {},
	"resizez":     {},
	"resizeh":     {},
	"resizev":     {},
	"wanderrange": {},
	"maxrange":    {},
	"huntrange":   {},
	"attackrange": {},
	"hitpoints":   {},
	"attack":      {},
	"strength":    {},
	"defence":     {},
	"magic":       {},
	"ranged":      {},
	"timer":       {},
	"respawnrate": {},
	"ambient":     {},
	"contrast":    {},
	"headicon":    {},
	"turnspeed":   {},
	"regenrate":   {},
}

// npcBooleanKeys is the set of keys parsed as IsConfigBoolean-gated
// booleans. TS source: NpcConfig.ts:28-33.
var npcBooleanKeys = map[string]struct{}{
	"minimap":     {},
	"members":     {},
	"givechase":   {},
	"alwaysontop": {},
}

// npcHeadKeyRE matches keys of the form headN or headNN (anchored: full key
// must be "head" + digits). More restrictive than TS L294 key.match(/head\d+/)
// (unanchored), but functionally equivalent for real content — the parser's
// strings.HasPrefix(key, "head") gate accepts the same key set in practice.
// Used only by the packer; the parser routes all `head`-prefixed keys through
// the model lookup (TS L103-109).
var npcHeadKeyRE = regexp.MustCompile(`^head\d+$`)

// npcPatrolEntry is the parsed value of a patrol{N}= line.
// TS returns `[coord, delay]` (NpcConfig.ts:259-261). Goscape uses a
// struct so the packer can type-assert one value cleanly.
type npcPatrolEntry struct {
	Coord int
	Delay int
}

// parseNpcConfigFor returns the per-key=value parser for .npc config
// blocks. Closure-captures four name-map registries plus paramLookups +
// ParamTypeConfigs for `param=` resolution.
//
// TS source: tools/pack/config/NpcConfig.ts:11-265.
func parseNpcConfigFor(modelPack, categoryPack, seqPack, huntPack *PackFile, lk *paramLookups, paramTypes *objtype.ParamTypeConfigs) ParseFn {
	return func(key, value string) (ConfigValue, bool, error) {
		// Strict-named simple cases first.

		if _, ok := npcStringKeys[key]; ok {
			if len(value) > 1000 {
				return nil, true, fmt.Errorf("%s value exceeds 1000 chars", key)
			}
			return value, true, nil
		}

		if _, ok := npcNumberKeys[key]; ok {
			n, err := strconv.ParseInt(value, 0, 64)
			if err != nil {
				return nil, true, fmt.Errorf("invalid number for %s: %s: %w", key, value, ErrInvalidNumber)
			}
			ni := int(n)
			// Range guards (TS NpcConfig.ts:61-87).
			switch key {
			case "size":
				if ni < 0 || ni > 5 {
					return nil, true, fmt.Errorf("%s out of range: %d: %w", key, ni, ErrOutOfRange)
				}
			case "resizex", "resizey", "resizez", "resizeh", "resizev":
				if ni < 0 || ni > 512 {
					return nil, true, fmt.Errorf("%s out of range: %d: %w", key, ni, ErrOutOfRange)
				}
			case "wanderrange", "maxrange", "huntrange", "attackrange":
				if ni < 0 || ni > 32767 {
					return nil, true, fmt.Errorf("%s out of range: %d: %w", key, ni, ErrOutOfRange)
				}
			case "hitpoints", "attack", "strength", "defence", "magic", "ranged":
				if ni < 0 || ni > 5000 {
					return nil, true, fmt.Errorf("%s out of range: %d: %w", key, ni, ErrOutOfRange)
				}
			case "timer":
				if ni < 0 || ni > 12000 {
					return nil, true, fmt.Errorf("%s out of range: %d: %w", key, ni, ErrOutOfRange)
				}
			case "respawnrate", "regenrate":
				if ni < 0 || ni > 12000 {
					return nil, true, fmt.Errorf("%s out of range: %d: %w", key, ni, ErrOutOfRange)
				}
			}
			return ni, true, nil
		}

		if _, ok := npcBooleanKeys[key]; ok {
			if !IsConfigBoolean(value) {
				return nil, true, fmt.Errorf("invalid boolean for %s: %s: %w", key, value, ErrInvalidBoolean)
			}
			return GetConfigBoolean(value), true, nil
		}

		// `model`-prefixed and `head`-prefixed keys both resolve to model ids.
		// TS L96-109.
		if strings.HasPrefix(key, "model") {
			idx := modelPack.GetByName(value)
			if idx == -1 {
				return nil, true, fmt.Errorf("unknown model: %s: %w", value, ErrUnknownModel)
			}
			return idx, true, nil
		}
		if strings.HasPrefix(key, "head") {
			idx := modelPack.GetByName(value)
			if idx == -1 {
				return nil, true, fmt.Errorf("unknown model: %s: %w", value, ErrUnknownModel)
			}
			return idx, true, nil
		}

		// `recol{N}{s|d}` — TS L110-116 reads index from key[5] as digit
		// (rejects > 9) and parses value as a plain int.
		if strings.HasPrefix(key, "recol") && len(key) >= 6 {
			idxChar := key[5]
			if idxChar < '0' || idxChar > '9' {
				return nil, false, nil
			}
			n, err := strconv.ParseInt(value, 10, 64)
			if err != nil {
				return nil, true, fmt.Errorf("invalid recol value: %s: %w", value, ErrInvalidRecol)
			}
			return int(n), true, nil
		}

		switch key {
		case "readyanim":
			idx := seqPack.GetByName(value)
			if idx == -1 {
				return nil, true, fmt.Errorf("unknown seq: %s: %w", value, ErrUnknownSeq)
			}
			return idx, true, nil

		case "walkanim":
			// TS L124-150: 4-element comma list → []int, otherwise single id.
			if strings.Contains(value, ",") {
				parts := strings.Split(value, ",")
				indices := make([]int, 0, len(parts))
				for _, anim := range parts {
					idx := seqPack.GetByName(anim)
					if idx == -1 {
						return nil, true, fmt.Errorf("unknown seq: %s: %w", anim, ErrUnknownSeq)
					}
					indices = append(indices, idx)
				}
				if len(indices) != 4 {
					return nil, true, fmt.Errorf("walkanim list must have 4 entries, got %d", len(indices))
				}
				return indices, true, nil
			}
			idx := seqPack.GetByName(value)
			if idx == -1 {
				return nil, true, fmt.Errorf("unknown seq: %s: %w", value, ErrUnknownSeq)
			}
			return idx, true, nil

		case "vislevel":
			// TS L151-161: 'hide' → 0; otherwise parseInt.
			if value == "hide" {
				return 0, true, nil
			}
			n, err := strconv.ParseInt(value, 10, 64)
			if err != nil {
				return nil, true, fmt.Errorf("invalid vislevel: %s", value)
			}
			return int(n), true, nil

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

		case "moverestrict":
			switch value {
			case "normal":
				return objtype.MoveRestrictNormal, true, nil
			case "blocked":
				return objtype.MoveRestrictBlocked, true, nil
			case "blocked+normal":
				return objtype.MoveRestrictBlockedNormal, true, nil
			case "indoors":
				return objtype.MoveRestrictIndoors, true, nil
			case "outdoors":
				return objtype.MoveRestrictOutdoors, true, nil
			case "nomove":
				return objtype.MoveRestrictNoMove, true, nil
			case "passthru":
				return objtype.MoveRestrictPassthru, true, nil
			}
			return nil, true, fmt.Errorf("unknown moverestrict: %s", value)

		case "blockwalk":
			switch value {
			case "none":
				return objtype.BlockWalkNone, true, nil
			case "all":
				return objtype.BlockWalkAll, true, nil
			case "NPC":
				return objtype.BlockWalkNPC, true, nil
			}
			return nil, true, fmt.Errorf("unknown blockwalk: %s", value)

		case "huntmode":
			idx := huntPack.GetByName(value)
			if idx == -1 {
				return nil, true, fmt.Errorf("unknown hunt: %s", value)
			}
			return idx, true, nil

		case "defaultmode":
			switch value {
			case "none":
				return objtype.NPCModeNone, true, nil
			case "wander":
				return objtype.NPCModeWander, true, nil
			case "patrol":
				return objtype.NPCModePatrol, true, nil
			}
			return nil, true, fmt.Errorf("unknown defaultmode: %s", value)
		}

		// `patrol`-prefixed keys: "level_mX_mZ_lX_lZ,delay" → [packedCoord, delay].
		// TS L234-261.
		if strings.HasPrefix(key, "patrol") {
			parts := strings.Split(value, ",")
			coordStr := parts[0]
			var delayStr string
			if len(parts) > 1 {
				delayStr = parts[1]
			}
			coordParts := strings.Split(coordStr, "_")
			if len(coordParts) < 5 {
				return nil, true, fmt.Errorf("patrol coord requires 5 parts: %s", coordStr)
			}
			level, e1 := strconv.Atoi(coordParts[0])
			mX, e2 := strconv.Atoi(coordParts[1])
			mZ, e3 := strconv.Atoi(coordParts[2])
			lX, e4 := strconv.Atoi(coordParts[3])
			lZ, e5 := strconv.Atoi(coordParts[4])
			if e1 != nil || e2 != nil || e3 != nil || e4 != nil || e5 != nil {
				return nil, true, fmt.Errorf("patrol coord parse failed: %s", coordStr)
			}
			if lZ < 0 || lX < 0 || mZ < 0 || mX < 0 || level < 0 {
				return nil, true, fmt.Errorf("patrol coord negative: %s", coordStr)
			}
			if lZ > 63 || lX > 63 || mZ > 255 || mX > 255 || level > 3 {
				return nil, true, fmt.Errorf("patrol coord out of range: %s", coordStr)
			}
			x := (mX << 6) + lX
			z := (mZ << 6) + lZ
			coord := (z & 0x3fff) | ((x & 0x3fff) << 14) | ((level & 0x3) << 28)
			delay := 0
			if delayStr != "" {
				d, derr := strconv.Atoi(delayStr)
				// TS L258-260: NaN delay → 0 (comment: "maybe we return null instead?").
				if derr == nil {
					delay = d
				}
			}
			return npcPatrolEntry{Coord: coord, Delay: delay}, true, nil
		}

		return nil, false, nil
	}
}

// packNpcConfigs walks each id ∈ [0, npcPack.Max), emitting per-id bodies
// into separate server + client PackedData buffers per NpcConfig.ts:267-510.
//
// Server gets: opcodes 18 (category), 74-79 (attack/defence/strength/
// hitpoints/ranged/magic), 26-27 (wanderrange/maxrange), 202-204 (huntrange/timer/
// respawnrate), 206 (moverestrict), 207 (attackrange), 208 (blockwalk),
// 209 (huntmode), 210 (defaultmode), 211 (members), 212 (patrol trailer),
// 213 (givechase), 214 (regenrate), 249 (params), 250 (debugname).
// Client gets all other display-facing opcodes.
//
// Note TS L386 emits `huntrange` as p1 (not p2 like its siblings). Likely
// a TS oversight; mirrored here for byte-exact parity.
//
// TS source: tools/pack/config/NpcConfig.ts:267-510.
// modelFlags is indexed by model id (size = Model PackFile max). Every
// model1..N and head1..N value sets modelFlags[v]|=0x2 (TS NpcConfig.ts:296,300).
// May be nil (existing callers pass nil; nil is a safe no-op).
//
// TS source: tools/pack/config/NpcConfig.ts:267-510.
func packNpcConfigs(configs map[string][]ConfigLine, npcPack *PackFile, modelFlags []int) (server, client *PackedData, err error) {
	server = NewPackedData(npcPack.Max)
	client = NewPackedData(npcPack.Max)

	for id := range npcPack.Max {
		debugname := npcPack.GetByID(id)
		cfg, hasConfig := configs[debugname]

		if hasConfig {
			// First-pass collectors (mirror TS L277-284).
			var (
				recolS   []int
				recolD   []int
				name     *string
				models   []int
				heads    []int
				params   []ParamValue
				patrol   []npcPatrolEntry
				vislevel bool
			)

			for _, line := range cfg {
				key := line.Key

				switch {
				case key == "name":
					s := line.Value.(string)
					name = &s
				case strings.HasPrefix(key, "model"):
					// TS L291-294: index = parseInt(key after "model") - 1.
					idxStr := key[len("model"):]
					idx, perr := strconv.Atoi(idxStr)
					if perr != nil {
						return nil, nil, fmt.Errorf("%s: invalid model key: %s", debugname, key)
					}
					idx--
					for len(models) <= idx {
						models = append(models, 0)
					}
					v := line.Value.(int)
					models[idx] = v
					if modelFlags != nil {
						modelFlags[v] |= 0x2 // todo: use context from script compiler
					}
				case npcHeadKeyRE.MatchString(key):
					// TS L295-298: head\d+; index = parseInt(after "head") - 1.
					idxStr := key[len("head"):]
					idx, perr := strconv.Atoi(idxStr)
					if perr != nil {
						return nil, nil, fmt.Errorf("%s: invalid head key: %s", debugname, key)
					}
					idx--
					for len(heads) <= idx {
						heads = append(heads, 0)
					}
					v := line.Value.(int)
					heads[idx] = v
					if modelFlags != nil {
						modelFlags[v] |= 0x2 // todo: use context from script compiler
					}
				case strings.HasPrefix(key, "recol"):
					// TS L297-303: index = parseInt(key.substring("recol", len-1)) - 1;
					// suffix 's' → recol_s, otherwise → recol_d.
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
				case key == "desc":
					client.P1(3)
					client.PJStr(line.Value.(string))
				case key == "size":
					client.P1(12)
					client.P1(uint8(line.Value.(int)))
				case key == "readyanim":
					client.P1(13)
					client.P2(uint16(line.Value.(int)))
				case key == "walkanim":
					if arr, ok := line.Value.([]int); ok {
						client.P1(17)
						client.P2(uint16(arr[0]))
						client.P2(uint16(arr[1]))
						client.P2(uint16(arr[2]))
						client.P2(uint16(arr[3]))
					} else {
						client.P1(14)
						client.P2(uint16(line.Value.(int)))
					}
				case key == "category":
					server.P1(18)
					server.P2(uint16(line.Value.(int)))
				// Moved from server opcodes 200/201 by Engine-TS 8139461a
				// (TS NpcConfig.ts:334-341 @1d25566c).
				case key == "wanderrange":
					server.P1(26)
					server.P2(uint16(line.Value.(int)))
				case key == "maxrange":
					server.P1(27)
					server.P2(uint16(line.Value.(int)))
				case strings.HasPrefix(key, "op"):
					n, perr := strconv.Atoi(key[2:])
					if perr != nil {
						return nil, nil, fmt.Errorf("%s: invalid op key: %s", debugname, key)
					}
					client.P1(uint8(30 + n - 1))
					client.PJStr(line.Value.(string))
				case key == "attack":
					server.P1(74)
					server.P2(uint16(line.Value.(int)))
				case key == "defence":
					server.P1(75)
					server.P2(uint16(line.Value.(int)))
				case key == "strength":
					server.P1(76)
					server.P2(uint16(line.Value.(int)))
				case key == "hitpoints":
					server.P1(77)
					server.P2(uint16(line.Value.(int)))
				case key == "ranged":
					server.P1(78)
					server.P2(uint16(line.Value.(int)))
				case key == "magic":
					server.P1(79)
					server.P2(uint16(line.Value.(int)))
				case key == "resizex":
					client.P1(90)
					client.P2(uint16(line.Value.(int)))
				case key == "resizey":
					client.P1(91)
					client.P2(uint16(line.Value.(int)))
				case key == "resizez":
					client.P1(92)
					client.P2(uint16(line.Value.(int)))
				case key == "minimap":
					if !line.Value.(bool) {
						client.P1(93)
					}
				case key == "vislevel":
					client.P1(95)
					client.P2(uint16(line.Value.(int)))
					vislevel = true
				case key == "resizeh":
					client.P1(97)
					client.P2(uint16(line.Value.(int)))
				case key == "resizev":
					client.P1(98)
					client.P2(uint16(line.Value.(int)))
				case key == "alwaysontop":
					// TS NpcConfig.ts:382-383: presence-only opcode, no value byte,
					// no value guard — emits unconditionally when key is present.
					client.P1(99)
				case key == "ambient":
					// TS NpcConfig.ts:382-384.
					client.P1(100)
					client.P1(uint8(line.Value.(int)))
				case key == "contrast":
					// TS NpcConfig.ts:385-387.
					client.P1(101)
					client.P1(uint8(line.Value.(int)))
				case key == "headicon":
					// TS NpcConfig.ts:391-393 @ 2e3bcf43.
					client.P1(102)
					client.P2(uint16(line.Value.(int)))
				case key == "turnspeed":
					// NEW at rev-254. TS NpcConfig.ts:394-396 @ 2e3bcf43.
					client.P1(103)
					client.P2(uint16(line.Value.(int)))
				case key == "huntrange":
					// TS L384-386: emits p1, not p2 (mirrored verbatim).
					server.P1(202)
					server.P1(uint8(line.Value.(int)))
				case key == "timer":
					server.P1(203)
					server.P2(uint16(line.Value.(int)))
				case key == "respawnrate":
					server.P1(204)
					server.P2(uint16(line.Value.(int)))
				case key == "moverestrict":
					server.P1(206)
					server.P1(uint8(line.Value.(int)))
				case key == "attackrange":
					server.P1(207)
					server.P2(uint16(line.Value.(int)))
				case key == "blockwalk":
					server.P1(208)
					server.P1(uint8(line.Value.(int)))
				case key == "huntmode":
					server.P1(209)
					server.P1(uint8(line.Value.(int)))
				case key == "defaultmode":
					server.P1(210)
					server.P1(uint8(line.Value.(int)))
				case key == "members":
					if line.Value.(bool) {
						server.P1(211)
					}
				case strings.HasPrefix(key, "patrol"):
					patrol = append(patrol, line.Value.(npcPatrolEntry))
				case key == "givechase":
					if !line.Value.(bool) {
						server.P1(213)
					}
				case key == "regenrate":
					server.P1(214)
					server.P2(uint16(line.Value.(int)))
				}
			}

			// recol trailer (TS L424-437): CLIENT opcode 40 + p1(count)
			// + per-k p2(s) p2(d), with rgb15→hsl16 conversion when s>=100
			// or d>=100.
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

			// Name forced-transmit: TS L439-446 — if name==null, default to
			// debugname; then unconditionally emit opcode 2.
			if name == nil {
				n := debugname
				name = &n
			}
			client.P1(2)
			client.PJStr(*name)

			// Models trailer (TS L448-455): CLIENT opcode 1 + p1(len)
			// + per-entry p2.
			if len(models) > 0 {
				client.P1(1)
				client.P1(uint8(len(models)))
				for _, m := range models {
					client.P2(uint16(m))
				}
			}

			// Heads trailer (TS L457-464): CLIENT opcode 60 + p1(len)
			// + per-entry p2.
			if len(heads) > 0 {
				client.P1(60)
				client.P1(uint8(len(heads)))
				for _, h := range heads {
					client.P2(uint16(h))
				}
			}

			// vislevel default (TS L466-470): if no vislevel was emitted,
			// emit opcode 95 + p2(1).
			if !vislevel {
				client.P1(95)
				client.P2(1)
			}

			// Patrol trailer (TS L472-481): SERVER opcode 212 + p1(len)
			// + per-entry p4(packedCoord) p1(delay).
			if len(patrol) > 0 {
				server.P1(212)
				server.P1(uint8(len(patrol)))
				for _, p := range patrol {
					server.P4(uint32(p.Coord))
					server.P1(uint8(p.Delay))
				}
			}

			// Params trailer (TS L483-498): SERVER opcode 249 + p1(len)
			// + per-entry p3(id) pbool(isString) (pjstr|p4).
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

		// Debugname trailer (TS L501-504): SERVER opcode 250 + pjstr,
		// outside the `if (config)` block — always emitted when debugname
		// is nonempty.
		if len(debugname) > 0 {
			server.P1(250)
			server.PJStr(debugname)
		}

		client.Next()
		server.Next()
	}

	return server, client, nil
}
