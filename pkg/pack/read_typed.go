package pack

import (
	"fmt"
	"path/filepath"
	"strings"
)

// ParseFn is the per-key=value callback used by ReadTypedConfigs.
//
// Return convention:
//   - ok=true, err=nil  → accepted; value goes into the ConfigLine
//   - ok=false, err=nil → invalid key  (TS parity for `undefined` →
//     "Invalid property key")
//   - err != nil        → invalid value (TS parity for `null`      →
//     "Invalid property value")
//
// TS source: tools/pack/config/PackShared.ts:135 (ConfigParseCallback).
type ParseFn func(key, value string) (ConfigValue, bool, error)

// ReadTypedConfigs walks <srcDir>/scripts/*.<ext>, splits each file
// into [name]-delimited blocks, applies constants substitution to
// every value, calls parseFn per key=value line, and enforces
// required-properties at block close. Returns map[debugname][]ConfigLine.
//
// NAI-192-D-COMMENT-STRIP-EAGER: CLOSED by the 289 sync. goscape used to
// route config files through LoadDirExtFull, which trims each line and strips
// // and /* */ comments; TS PackShared.readConfigs uses raw readline with only
// an empty-line and //-prefix skip. The old note called this harmless "for
// varn/vars whose values cannot contain comment markers" — Content 2b62ae68d
// falsified it: fishing_equipment.struct gained a param value ending in a
// space, and the trim cost one byte of server/struct.dat against the
// reference. Config families now use LoadDirExtConfig, which implements the
// readConfigs contract exactly. Scripts still use LoadDirExtFull.
//
// NAI-192-D-PARSE-ERROR-ENVELOPE: error messages use
// "<kind> in <file>: <detail>" rather than TS
// "\nError during parsing - see <file>:<n+1>\n<msg>". Matches existing
// pkg/pack/parse.go convention.
//
// TS source: tools/pack/config/PackShared.ts:141-247.
func ReadTypedConfigs(srcDir, ext string, required []string, parseFn ParseFn, c Constants) (map[string][]ConfigLine, error) {
	configs := map[string][]ConfigLine{}
	scriptsDir := filepath.Join(srcDir, "scripts")
	if !FileExists(scriptsDir) {
		return configs, nil
	}
	var outerErr error
	LoadDirExtConfig(scriptsDir, ext, func(lines []string, file string) {
		if outerErr != nil {
			return
		}
		current := ""
		var block []ConfigLine

		flush := func() bool {
			if current == "" {
				return true
			}
			if _, dup := configs[current]; dup {
				outerErr = fmt.Errorf("duplicate config in %s: %s", file, current)
				return false
			}
			for _, prop := range required {
				found := false
				for _, ln := range block {
					if ln.Key == prop {
						found = true
						break
					}
				}
				if !found {
					outerErr = fmt.Errorf("missing required property %q for %s in %s", prop, current, file)
					return false
				}
			}
			configs[current] = block
			return true
		}

		for _, line := range lines {
			if strings.HasPrefix(line, "[") {
				if !strings.HasSuffix(line, "]") {
					outerErr = fmt.Errorf("missing closing bracket in %s: %s", file, line)
					return
				}
				if !flush() {
					return
				}
				name := line[1 : len(line)-1]
				if name == "" {
					outerErr = fmt.Errorf("empty config name in %s", file)
					return
				}
				current = name
				block = nil
				continue
			}
			key, value, ok := strings.Cut(line, "=")
			if !ok {
				outerErr = fmt.Errorf("missing property separator in %s: %s", file, line)
				return
			}
			value = substituteConstants(value, c)

			parsed, parseOk, perr := parseFn(key, value)
			if perr != nil {
				outerErr = fmt.Errorf("invalid property value in %s: %s", file, line)
				return
			}
			if !parseOk {
				outerErr = fmt.Errorf("invalid property key in %s: %s", file, line)
				return
			}
			block = append(block, ConfigLine{Key: key, Value: parsed})
		}
		flush()
	})
	if outerErr != nil {
		return nil, outerErr
	}
	return configs, nil
}
