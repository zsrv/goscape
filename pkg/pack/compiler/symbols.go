// Package compiler — extended in NAI-202 to host BuildSymbols and its
// supporting helpers. NAI-200 introduced TypeInfo + Load family.
// NAI-202 introduces the runServerCompiler driver port that consumes
// them.
package compiler

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/zsrv/goscape/pkg/objtype"
)

// loadCompilerConstants walks scriptsDir recursively for *.constant
// files and returns a flat map of constant name → raw textual value.
// Mirrors TS Compiler.ts:152-173.
//
// NAI-202-D-CONSTANT-LOOSE-PARSER: this is goscape's second .constant
// parser. TS has two semantically different parsers — PackShared.ts:262
// is strict (dedup-error, no quote-strip, split-rest-after-first '=');
// Compiler.ts:152 is loose (last-writer-wins, surrounding-quote strip,
// drop-past-second '='). Goscape mirrors that two-parser shape rather
// than collapsing them.
//
// Per-line rules (TS-faithful, in order):
//   - empty line                         → skip
//   - line starts with "//"              → skip
//   - split on "=", take first segment as name, second as value;
//     anything past the second "=" is discarded (mirrors TS
//     `const [name, value] = line.split('=')`)
//   - trim name and value (whitespace)
//   - if name starts with "^", strip the leading "^"
//   - if value starts AND ends with `"`, strip both
//   - assign m[name] = value (last writer wins; no dedup error)
//
// A line with no "=" returns an error wrapping the file path and
// offending line — TS would throw on the parts[1] undefined access.
//
// Missing scriptsDir → empty map, nil error.
func loadCompilerConstants(scriptsDir string) (map[string]string, error) {
	m := map[string]string{}

	info, err := os.Stat(scriptsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return m, nil
		}
		return nil, fmt.Errorf("stat %s: %w", scriptsDir, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%s: not a directory", scriptsDir)
	}

	err = filepath.WalkDir(scriptsDir, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() || !strings.HasSuffix(path, ".constant") {
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return fmt.Errorf("read %s: %w", path, readErr)
		}
		// TS uses split(/\r?\n/); strings.Split("\n") + per-line \r-strip mirrors that.
		for lineNo, raw := range strings.Split(string(data), "\n") {
			line := strings.TrimSuffix(raw, "\r")
			if len(line) == 0 {
				continue
			}
			if strings.HasPrefix(line, "//") {
				continue
			}
			// TS unbounded split + destructure of [name, value] drops parts past second '='.
			// SplitN with n=3 captures parts[0], parts[1]; parts[2] (if present) is discarded.
			parts := strings.SplitN(line, "=", 3)
			if len(parts) < 2 {
				return fmt.Errorf("%s:%d: line missing '=': %q", path, lineNo+1, line)
			}
			name := strings.TrimPrefix(strings.TrimSpace(parts[0]), "^")
			value := strings.TrimSpace(parts[1])
			if len(value) >= 2 && strings.HasPrefix(value, "\"") && strings.HasSuffix(value, "\"") {
				value = value[1 : len(value)-1]
			}
			m[name] = value
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return m, nil
}

// scriptVarTypeName returns the TS-style name for a ScriptVarType code.
// Mirrors TS ScriptVarType.getType (ScriptVarType.ts:85-170). Unexported;
// used internally by the varp/varn/vars enrichment passes (NAI-202).
//
// Why not method on each VarPlayerType/VarNpcType/VarSharedType:
// goscape's existing objtype.ParamType.GetType() (paramtype.go:105) is
// the only entity that has this method. Adding three more parallel
// methods scatters identical switch statements across four files.
// Centralizing here is the cheaper maintenance posture.
func scriptVarTypeName(t objtype.ScriptVarType) string {
	switch t {
	case objtype.ScriptVarTypeInt:
		return "int"
	case objtype.ScriptVarTypeString:
		return "string"
	case objtype.ScriptVarTypeEnum:
		return "enum"
	case objtype.ScriptVarTypeObj:
		return "obj"
	case objtype.ScriptVarTypeLoc:
		return "loc"
	case objtype.ScriptVarTypeComponent:
		return "component"
	case objtype.ScriptVarTypeNamedObj:
		return "namedobj"
	case objtype.ScriptVarTypeStruct:
		return "struct"
	case objtype.ScriptVarTypeBoolean:
		return "boolean"
	case objtype.ScriptVarTypeCoord:
		return "coord"
	case objtype.ScriptVarTypeCategory:
		return "category"
	case objtype.ScriptVarTypeSpotanim:
		return "spotanim"
	case objtype.ScriptVarTypeNPC:
		return "npc"
	case objtype.ScriptVarTypeInv:
		return "inv"
	case objtype.ScriptVarTypeSynth:
		return "synth"
	case objtype.ScriptVarTypeSeq:
		return "seq"
	case objtype.ScriptVarTypeStat:
		return "stat"
	case objtype.ScriptVarTypeInterface:
		return "interface"
	default:
		return "unknown"
	}
}
