package pack

import (
	"fmt"
	"path/filepath"
	"strings"
)

// Constants maps a constant name (without leading "^") to its raw
// textual value. Substitution into config values is done via
// substituteConstants.
//
// TS source: tools/pack/config/PackShared.ts:86 (CONSTANTS module-level
// map). NAI-192 keeps the map non-global — caller threads it through
// LoadConstants → ReadTypedConfigs.
type Constants map[string]string

// LoadConstants walks <srcDir>/scripts recursively for *.constant
// files, parses `name=value` lines, and returns the aggregated map.
// Blank lines (including whitespace-only) and lines starting with `//`
// are skipped. A leading `^` on a name is stripped. Duplicate names
// across all files error.
//
// Missing <srcDir>/scripts directory returns an empty map without
// error.
//
// TS source: tools/pack/config/PackShared.ts:262-289.
func LoadConstants(srcDir string) (Constants, error) {
	c := Constants{}
	scriptsDir := filepath.Join(srcDir, "scripts")
	if !FileExists(scriptsDir) {
		return c, nil
	}
	var outerErr error
	LoadDirExt(scriptsDir, ".constant", func(lines []string, file string) {
		if outerErr != nil {
			return
		}
		for _, raw := range lines {
			line := strings.TrimSpace(raw)
			if len(line) == 0 || strings.HasPrefix(line, "//") {
				continue
			}
			eq := strings.IndexByte(line, '=')
			if eq < 0 {
				outerErr = fmt.Errorf("bad constant declaration in %s: %s", file, line)
				return
			}
			name := strings.TrimSpace(line[:eq])
			value := strings.TrimSpace(line[eq+1:])
			if strings.HasPrefix(name, "^") {
				name = name[1:]
			}
			if name == "" {
				outerErr = fmt.Errorf("empty constant name in %s: %s", file, line)
				return
			}
			if _, dup := c[name]; dup {
				outerErr = fmt.Errorf("duplicate constant in %s: %s", file, name)
				return
			}
			c[name] = value
		}
	})
	if outerErr != nil {
		return nil, outerErr
	}
	return c, nil
}

// substituteConstants scans value for `^NAME` runs (terminators: '\r'
// '\n' ',' ' ' end-of-string) and replaces with c[NAME] when present.
// Absent names leave the literal `^NAME` in place — TS parity, no
// error.
//
// TS source: tools/pack/config/PackShared.ts:200-223.
func substituteConstants(value string, c Constants) string {
	if !strings.ContainsRune(value, '^') {
		return value
	}
	var b strings.Builder
	b.Grow(len(value))
	i := 0
	for i < len(value) {
		if value[i] != '^' {
			b.WriteByte(value[i])
			i++
			continue
		}
		// Scan to terminator.
		end := i + 1
		for end < len(value) {
			ch := value[end]
			if ch == '\r' || ch == '\n' || ch == ',' || ch == ' ' {
				break
			}
			end++
		}
		name := value[i+1 : end]
		if sub, ok := c[name]; ok {
			b.WriteString(sub)
		} else {
			b.WriteString(value[i:end])
		}
		i = end
	}
	return b.String()
}
