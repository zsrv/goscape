package pack

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/zsrv/goscape/pkg/colorconv"
)

// spotanimNumberKeys mirrors TS SpotAnimConfig.ts:8-12 numberKeys[].
var spotanimNumberKeys = map[string]struct{}{
	"resizeh":  {},
	"resizev":  {},
	"angle":    {},
	"ambient":  {},
	"contrast": {},
}

// spotanimBooleanKeys mirrors TS SpotAnimConfig.ts:14-16 booleanKeys[].
var spotanimBooleanKeys = map[string]struct{}{}

// parseSpotAnimConfigFor returns the per-key=value parser for .spotanim
// config blocks. Closure-captures modelPack + seqPack.
//
// NAI-195-D-DEADBRANCH-OMITTED: empty TS stringKeys[] branch omitted.
//
// TS source: tools/pack/config/SpotAnimConfig.ts:5-90.
func parseSpotAnimConfigFor(modelPack, seqPack *PackFile) ParseFn {
	return func(key, value string) (ConfigValue, bool, error) {
		if _, ok := spotanimNumberKeys[key]; ok {
			n, err := strconv.ParseInt(value, 0, 64)
			if err != nil {
				return nil, true, fmt.Errorf("invalid number for %s: %s: %w", key, value, ErrInvalidNumber)
			}
			switch key {
			case "resizeh", "resizev":
				if n < 0 || n > 512 {
					return nil, true, fmt.Errorf("%s out of range [0,512]: %d: %w", key, n, ErrOutOfRange)
				}
			case "angle":
				if n < 0 || n > 360 {
					return nil, true, fmt.Errorf("%s out of range [0,360]: %d: %w", key, n, ErrOutOfRange)
				}
			case "ambient", "contrast":
				if n < -128 || n > 127 {
					return nil, true, fmt.Errorf("%s out of range [-128,127]: %d: %w", key, n, ErrOutOfRange)
				}
			}
			return int(n), true, nil
		}
		if _, ok := spotanimBooleanKeys[key]; ok {
			if !IsConfigBoolean(value) {
				return nil, true, fmt.Errorf("invalid boolean for %s: %s: %w", key, value, ErrInvalidBoolean)
			}
			return GetConfigBoolean(value), true, nil
		}
		if key == "model" {
			idx := modelPack.GetByName(value)
			if idx == -1 {
				return nil, true, fmt.Errorf("unknown model: %s: %w", value, ErrUnknownModel)
			}
			return idx, true, nil
		}
		if key == "anim" {
			idx := seqPack.GetByName(value)
			if idx == -1 {
				return nil, true, fmt.Errorf("unknown anim: %s: %w", value, ErrUnknownAnim)
			}
			return idx, true, nil
		}
		if strings.HasPrefix(key, "recol") && len(key) >= 6 {
			// TS SpotAnimConfig.ts:81-86: parseInt(key[5]) > 9 → null. Note NaN > 9
			// is false in TS, so non-digit key[5] slips through there; goscape
			// rejects non-digit key[5] defensively.
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
		return nil, false, nil
	}
}

// packSpotAnimConfigs emits the per-id body for .spotanim configs.
//
// Per TS, the recol opcode index is decoded from the key suffix as
// `parseInt(key[5:len-1]) - 1`; `*s` keys map to opcode 40+index,
// `*d` keys map to opcode 50+index. Note this is independent of the
// parser's single-char gate (`parseInt(key[5]) > 9`).
//
// Server-side: opcode 250 + pjstr(debugname) when debugname.length > 0.
//
// modelFlags is indexed by model id (size = Model PackFile max). The
// spotanim packer writes 0x2 flags for model references per
// TS SpotAnimConfig.ts:107 @ 9aadcec4.
//
// TS source: tools/pack/config/SpotAnimConfig.ts:92-152.
func packSpotAnimConfigs(configs map[string][]ConfigLine, spotanimPack *PackFile, modelFlags []int) (server, client *PackedData) {
	server = NewPackedData(spotanimPack.Max)
	client = NewPackedData(spotanimPack.Max)

	for id := range spotanimPack.Max {
		debugname := spotanimPack.GetByID(id)
		cfg := configs[debugname]

		for _, line := range cfg {
			key := line.Key
			switch {
			case key == "model":
				v := line.Value.(int)
				client.P1(1)
				client.P2(uint16(v))
				if modelFlags != nil {
					modelFlags[v] |= 0x2 // TS SpotAnimConfig.ts:107
				}
			case key == "anim":
				client.P1(2)
				client.P2(uint16(line.Value.(int)))
			case key == "resizeh":
				client.P1(4)
				client.P2(uint16(line.Value.(int)))
			case key == "resizev":
				client.P1(5)
				client.P2(uint16(line.Value.(int)))
			case key == "angle":
				client.P1(6)
				client.P2(uint16(line.Value.(int)))
			case key == "ambient":
				client.P1(7)
				client.P1(uint8(line.Value.(int)))
			case key == "contrast":
				client.P1(8)
				client.P1(uint8(line.Value.(int)))
			case strings.HasPrefix(key, "recol") && len(key) >= 7:
				// TS SpotAnimConfig.ts:130: parseInt(key.substring(5, len-1)) - 1.
				idx, err := strconv.Atoi(key[5 : len(key)-1])
				if err != nil {
					continue
				}
				idx--
				suffix := key[len(key)-1]
				switch suffix {
				case 's':
					client.P1(uint8(40 + idx))
					client.P2(uint16(line.Value.(int)))
				case 'd':
					client.P1(uint8(50 + idx))
					client.P2(uint16(line.Value.(int)))
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
