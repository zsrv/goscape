package pack

import (
	"fmt"
	"strconv"
	"strings"
)

// seqNumberKeys mirrors TS SeqConfig.ts:7-9 numberKeys[].
var seqNumberKeys = map[string]struct{}{
	"loops":    {},
	"priority": {},
	"maxloops": {},
}

// seqBooleanKeys mirrors TS SeqConfig.ts:11-13 booleanKeys[].
var seqBooleanKeys = map[string]struct{}{
	"stretches": {},
}

// parseSeqConfigFor returns the per-key=value parser for .seq config
// blocks. Closure-captures animPack (for frame{N}/iframe{N}) and
// objPack (for replaceheldleft/replaceheldright).
//
// NAI-195-D-DEADBRANCH-OMITTED: TS SeqConfig.ts:5 declares empty
// stringKeys[] — omitted here; revives if schema adds string keys.
//
// TS source: tools/pack/config/SeqConfig.ts:4-119.
func parseSeqConfigFor(animPack, objPack *PackFile) ParseFn {
	return func(key, value string) (ConfigValue, bool, error) {
		if _, ok := seqNumberKeys[key]; ok {
			n, err := strconv.ParseInt(value, 0, 64)
			if err != nil {
				return nil, true, fmt.Errorf("invalid number for %s: %s: %w", key, value, ErrInvalidNumber)
			}
			switch key {
			case "loops", "maxloops":
				if n < 0 || n > 1000 {
					return nil, true, fmt.Errorf("%s out of range [0,1000]: %d: %w", key, n, ErrOutOfRange)
				}
			case "priority":
				if n < 0 || n > 10 {
					return nil, true, fmt.Errorf("%s out of range [0,10]: %d: %w", key, n, ErrOutOfRange)
				}
			}
			return int(n), true, nil
		}
		if _, ok := seqBooleanKeys[key]; ok {
			if !IsConfigBoolean(value) {
				return nil, true, fmt.Errorf("invalid boolean for %s: %s: %w", key, value, ErrInvalidBoolean)
			}
			return GetConfigBoolean(value), true, nil
		}
		// TS SeqConfig.ts:63-69: frame{N} check comes before iframe{N}.
		// Both use animPack; since "iframe1".startsWith("frame") is false,
		// order is safe either way, but we mirror TS literally.
		if strings.HasPrefix(key, "frame") {
			idx := animPack.GetByName(value)
			if idx == -1 {
				return nil, true, fmt.Errorf("unknown anim: %s: %w", value, ErrUnknownAnim)
			}
			return idx, true, nil
		}
		if strings.HasPrefix(key, "iframe") {
			idx := animPack.GetByName(value)
			if idx == -1 {
				return nil, true, fmt.Errorf("unknown anim: %s: %w", value, ErrUnknownAnim)
			}
			return idx, true, nil
		}
		if strings.HasPrefix(key, "delay") {
			n, err := strconv.Atoi(value)
			if err != nil {
				return nil, true, fmt.Errorf("invalid delay: %s", value)
			}
			return n, true, nil
		}
		if key == "walkmerge" {
			parts := strings.Split(value, ",")
			labels := make([]int, 0, len(parts))
			for _, part := range parts {
				underscore := strings.Index(part, "_")
				// goscape defensive: TS SeqConfig.ts:88-91 uses indexOf('_')+1 without
				// validating presence; we reject malformed labels rather than emitting NaN.
				if underscore == -1 {
					return nil, true, fmt.Errorf("invalid walkmerge label: %s", part)
				}
				n, err := strconv.Atoi(part[underscore+1:])
				if err != nil {
					return nil, true, fmt.Errorf("invalid walkmerge label: %s", part)
				}
				labels = append(labels, n)
			}
			return labels, true, nil
		}
		if key == "replaceheldleft" || key == "replaceheldright" {
			if value == "hide" {
				return 0, true, nil
			}
			idx := objPack.GetByName(value)
			if idx == -1 {
				return nil, true, fmt.Errorf("unknown obj: %s: %w", value, ErrUnknownObj)
			}
			return idx + 512, true, nil
		}
		// TS SeqConfig.ts:116-148 (Engine-TS 9aadcec4): preanim_move / postanim_move / duplicatebehavior.
		// TS returns null for unrecognised enum values → Go: accepted=true, err!=nil.
		if key == "preanim_move" {
			switch value {
			case "delaymove":
				return 0, true, nil
			case "delayanim":
				return 1, true, nil
			case "merge":
				return 2, true, nil
			default:
				return nil, true, fmt.Errorf("invalid preanim_move value: %s", value)
			}
		}
		if key == "postanim_move" {
			switch value {
			case "delaymove":
				return 0, true, nil
			case "abortanim":
				return 1, true, nil
			case "merge":
				return 2, true, nil
			default:
				return nil, true, fmt.Errorf("invalid postanim_move value: %s", value)
			}
		}
		if key == "duplicatebehavior" {
			switch value {
			case "0":
				return 0, true, nil
			case "reset":
				return 1, true, nil
			case "reset_loop":
				return 2, true, nil
			default:
				return nil, true, fmt.Errorf("invalid duplicatebehavior value: %s", value)
			}
		}
		return nil, false, nil
	}
}

// packSeqConfigs emits the per-id body for .seq configs. For each id:
//   - loose opcodes (loops/walkmerge/stretches/priority/replaceheld*
//     /maxloops) are written in config-line order;
//   - the frames block (opcode 1) is appended after the full config scan
//     when at least one frame{N} key is present;
//   - the server 250-trailer fires when the slot has a non-empty debugname.
//
// modelFlags is accepted for TS ConfigPackCallback parity
// (PackShared.ts:137-141); seq does not write any model flags.
//
// TS source: tools/pack/config/SeqConfig.ts:121-208.
func packSeqConfigs(configs map[string][]ConfigLine, seqPack *PackFile, modelFlags []int) (server, client *PackedData) {
	server = NewPackedData(seqPack.Max)
	client = NewPackedData(seqPack.Max)

	for id := range seqPack.Max {
		debugname := seqPack.GetByID(id)
		cfg := configs[debugname]

		var frames, iframes, delays []int
		hasIframe := map[int]bool{}
		hasDelay := map[int]bool{}

		for _, line := range cfg {
			switch {
			case strings.HasPrefix(line.Key, "frame"):
				idx, err := strconv.Atoi(line.Key[len("frame"):])
				if err != nil {
					continue
				}
				idx--
				for len(frames) <= idx {
					frames = append(frames, 0)
				}
				frames[idx] = line.Value.(int)
			case strings.HasPrefix(line.Key, "iframe"):
				idx, err := strconv.Atoi(line.Key[len("iframe"):])
				if err != nil {
					continue
				}
				idx--
				for len(iframes) <= idx {
					iframes = append(iframes, 0)
				}
				iframes[idx] = line.Value.(int)
				hasIframe[idx] = true
			case strings.HasPrefix(line.Key, "delay"):
				idx, err := strconv.Atoi(line.Key[len("delay"):])
				if err != nil {
					continue
				}
				idx--
				for len(delays) <= idx {
					delays = append(delays, 0)
				}
				delays[idx] = line.Value.(int)
				hasDelay[idx] = true
			case line.Key == "loops":
				client.P1(2)
				client.P2(uint16(line.Value.(int)))
			case line.Key == "walkmerge":
				labels := line.Value.([]int)
				client.P1(3)
				client.P1(uint8(len(labels)))
				for _, lab := range labels {
					client.P1(uint8(lab))
				}
			case line.Key == "stretches":
				if line.Value.(bool) {
					client.P1(4)
				}
			case line.Key == "priority":
				client.P1(5)
				client.P1(uint8(line.Value.(int)))
			case line.Key == "replaceheldleft":
				client.P1(6)
				client.P2(uint16(line.Value.(int)))
			case line.Key == "replaceheldright":
				client.P1(7)
				client.P2(uint16(line.Value.(int)))
			case line.Key == "maxloops":
				client.P1(8)
				client.P1(uint8(line.Value.(int)))
			// TS SeqConfig.ts:203-211 (Engine-TS 9aadcec4): preanim_move / postanim_move / duplicatebehavior.
			case line.Key == "preanim_move":
				client.P1(9)
				client.P1(uint8(line.Value.(int)))
			case line.Key == "postanim_move":
				client.P1(10)
				client.P1(uint8(line.Value.(int)))
			case line.Key == "duplicatebehavior":
				client.P1(11)
				client.P1(uint8(line.Value.(int)))
			}
		}

		if len(frames) > 0 {
			client.P1(1)
			client.P1(uint8(len(frames)))
			for j := range frames {
				client.P2(uint16(frames[j]))
				if hasIframe[j] {
					client.P2(uint16(iframes[j]))
				} else {
					client.P2(uint16(0xFFFF))
				}
				if hasDelay[j] {
					client.P2(uint16(delays[j]))
				} else {
					client.P2(0)
				}
			}
		}

		if len(debugname) > 0 {
			server.P1(250)
			server.PJStr(debugname)
		}

		client.Next()
		server.Next()
	}
	return server, client
}
