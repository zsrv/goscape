package pack

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/zsrv/goscape/pkg/colorconv"
)

// idkBooleanKeys mirrors TS IdkConfig.ts:10-12 booleanKeys[].
var idkBooleanKeys = map[string]struct{}{
	"disable": {},
}

// idkTypeBodypart mirrors TS IdkConfig.ts:50-97 switch statement.
var idkTypeBodypart = map[string]int{
	"man_hair":    0,
	"man_jaw":     1,
	"man_torso":   2,
	"man_arms":    3,
	"man_hands":   4,
	"man_legs":    5,
	"man_feet":    6,
	"woman_hair":  7,
	"woman_jaw":   8,
	"woman_torso": 9,
	"woman_arms":  10,
	"woman_hands": 11,
	"woman_legs":  12,
	"woman_feet":  13,
}

// parseIdkConfigFor returns the per-key=value parser for .idk config
// blocks. Closure-captures modelPack (for model{N}/head{N}).
//
// NAI-195-D-DEADBRANCH-OMITTED: TS IdkConfig.ts:6-8 declares BOTH empty
// stringKeys[] AND empty numberKeys[] — both omitted here.
//
// TS source: tools/pack/config/IdkConfig.ts:5-124.
func parseIdkConfigFor(modelPack *PackFile) ParseFn {
	return func(key, value string) (ConfigValue, bool, error) {
		if _, ok := idkBooleanKeys[key]; ok {
			if !IsConfigBoolean(value) {
				return nil, true, fmt.Errorf("invalid boolean for %s: %s: %w", key, value, ErrInvalidBoolean)
			}
			return GetConfigBoolean(value), true, nil
		}
		if key == "type" {
			bp, ok := idkTypeBodypart[value]
			if !ok {
				return nil, true, fmt.Errorf("unknown idk type: %s", value)
			}
			return bp, true, nil
		}
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
		return nil, false, nil
	}
}

// packIdkConfigs emits the per-id body for .idk configs.
//
// Per-id accumulators (TS IdkConfig.ts:136-139): recol_s/recol_d/models/heads.
// In-loop loose-emit: `type` → opcode 1 + p1(bodypart); `disable=true` → opcode 3.
// End-of-id emission order (TS IdkConfig.ts:165-193):
//
//	recol_s[i] → opcode 40+i + p2
//	recol_d[i] → opcode 50+i + p2
//	heads[i]   → opcode 60+i + p2
//	models     → opcode 2 + p1(len) + per-entry p2
//
// Server side: opcode 250 + pjstr(debugname) when debugname.length > 0.
//
// TS source: tools/pack/config/IdkConfig.ts:126-205.
func packIdkConfigs(configs map[string][]ConfigLine, idkPack *PackFile) (server, client *PackedData) {
	server = NewPackedData(idkPack.Max)
	client = NewPackedData(idkPack.Max)

	for id := range idkPack.Max {
		debugname := idkPack.GetByID(id)
		cfg := configs[debugname]

		var recolS, recolD, models, heads []int

		for _, line := range cfg {
			key := line.Key
			switch {
			case strings.HasPrefix(key, "model"):
				idx, err := strconv.Atoi(key[len("model"):])
				if err != nil {
					continue
				}
				idx--
				for len(models) <= idx {
					models = append(models, 0)
				}
				models[idx] = line.Value.(int)
			case strings.HasPrefix(key, "head"):
				idx, err := strconv.Atoi(key[len("head"):])
				if err != nil {
					continue
				}
				idx--
				for len(heads) <= idx {
					heads = append(heads, 0)
				}
				heads[idx] = line.Value.(int)
			case strings.HasPrefix(key, "recol") && strings.HasSuffix(key, "s"):
				idx, err := strconv.Atoi(key[5 : len(key)-1])
				if err != nil {
					continue
				}
				idx--
				for len(recolS) <= idx {
					recolS = append(recolS, 0)
				}
				recolS[idx] = line.Value.(int)
			case strings.HasPrefix(key, "recol") && strings.HasSuffix(key, "d"):
				idx, err := strconv.Atoi(key[5 : len(key)-1])
				if err != nil {
					continue
				}
				idx--
				for len(recolD) <= idx {
					recolD = append(recolD, 0)
				}
				recolD[idx] = line.Value.(int)
			case key == "type":
				client.P1(1)
				client.P1(uint8(line.Value.(int)))
			case key == "disable":
				if line.Value.(bool) {
					client.P1(3)
				}
			}
		}

		for i, v := range recolS {
			client.P1(uint8(40 + i))
			client.P2(uint16(v))
		}
		for i, v := range recolD {
			client.P1(uint8(50 + i))
			client.P2(uint16(v))
		}
		for i, v := range heads {
			client.P1(uint8(60 + i))
			client.P2(uint16(v))
		}
		if len(models) > 0 {
			client.P1(2)
			client.P1(uint8(len(models)))
			for _, v := range models {
				client.P2(uint16(v))
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
