// Package compiler — extended in NAI-202 to host BuildSymbols and its
// supporting helpers. NAI-200 introduced TypeInfo + Load family.
// NAI-202 introduces the runServerCompiler driver port that consumes
// them.
package compiler

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/script"
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

// populateCommandInfo populates the command TypeInfo with one entry per
// ScriptOpcodeMap key plus pointer-flag enrichments from
// ScriptOpcodePointers. Wraps populateCommandInfoFrom with the package
// globals; the seam exists so tests can pass synthetic data.
func populateCommandInfo(info *TypeInfo) {
	populateCommandInfoFrom(info, script.ScriptOpcodeMap, script.ScriptOpcodePointers)
}

// populateCommandInfoFrom is the testable seam under populateCommandInfo.
// Mirrors TS Compiler.ts:110-150 (allCommands sort + commandInfo build).
//
// NAI-202-D-CORRUPT2-FIELD: TS Compiler.ts:146-147 has a typo — the
// corrupt2 arm assigns to commandInfo.corrupt[opcode] (overwriting
// commandInfo.corrupt[opcode] just-written one line above) instead of
// commandInfo.corrupt2[opcode]. Goscape writes to info.Corrupt2[op].
func populateCommandInfoFrom(
	info *TypeInfo,
	opmap map[string]script.Opcode,
	pointers map[script.Opcode]script.Pointers,
) {
	type entry struct {
		name   string
		opcode script.Opcode
	}
	entries := make([]entry, 0, len(opmap))
	for n, op := range opmap {
		entries = append(entries, entry{n, op})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].opcode < entries[j].opcode
	})

	for _, e := range entries {
		op := int(e.opcode)
		info.Add(op, strings.ToLower(e.name), true)

		ptrs, hasPtrs := pointers[e.opcode]
		if !hasPtrs {
			continue
		}

		if len(ptrs.Require) > 0 {
			info.Require[op] = strings.Join(ptrs.Require, ",")
			if len(ptrs.Require2) > 0 {
				info.Require2[op] = strings.Join(ptrs.Require2, ",")
			}
		}

		if len(ptrs.Set) > 0 {
			if ptrs.Conditional {
				info.Conditional[op] = true
			}
			info.Set[op] = strings.Join(ptrs.Set, ",")
			if len(ptrs.Set2) > 0 {
				info.Set2[op] = strings.Join(ptrs.Set2, ",")
			}
		}

		if len(ptrs.Corrupt) > 0 {
			info.Corrupt[op] = strings.Join(ptrs.Corrupt, ",")
			if len(ptrs.Corrupt2) > 0 {
				// NAI-202-D-CORRUPT2-FIELD: write to Corrupt2, not Corrupt.
				info.Corrupt2[op] = strings.Join(ptrs.Corrupt2, ",")
			}
		}
	}
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

// populateDbColumns synthesizes the dbcolumn TypeInfo from DbTableType
// column metadata. Mirrors TS Compiler.ts:275-297.
//
// Bitfield-encoded column ids:
//   - primary id  = (table.ID & 0xffff) << 12 | (column & 0x7f) << 4
//   - tuple id    = primary | ((tuple + 1) & 0xf)     // only if len(types) > 1
//
// .Add is called with updateMax=false on all entries — dbcolumn.Max
// stays at -1 (matching TS Compiler.ts:286,292 third arg).
//
// .VarType[primary] is the comma-joined list of all type names.
// .VarType[tuple_n] is the single tuple type name.
//
// Skips: nil table entries; columns whose Types[col] is nil.
func populateDbColumns(info *TypeInfo, tables *objtype.DbTableTypeConfigs) {
	if tables == nil {
		return
	}
	for _, table := range tables.Configs {
		if table == nil {
			continue
		}
		for column, types := range table.Types {
			if types == nil {
				continue
			}
			primary := int(((table.ID & 0xffff) << 12) | ((column & 0x7f) << 4))

			typeNames := make([]string, len(types))
			for i, t := range types {
				typeNames[i] = scriptVarTypeName(t)
			}
			columnName := ""
			if column < len(table.ColumnNames) {
				columnName = table.ColumnNames[column]
			}
			primaryLabel := fmt.Sprintf("%s:%s", table.DebugName, columnName)
			info.Add(primary, primaryLabel, false)
			info.VarType[primary] = strings.Join(typeNames, ",")

			if len(types) > 1 {
				for tuple := range types {
					tupleID := primary | ((tuple + 1) & 0xf)
					tupleLabel := fmt.Sprintf("%s:%s:%d", table.DebugName, columnName, tuple)
					info.Add(tupleID, tupleLabel, false)
					info.VarType[tupleID] = typeNames[tuple]
				}
			}
		}
	}
}

// populateInterfaceOverlay derives the `interface` and `overlayinterface`
// TypeInfos from componentInfo (loaded from interface.pack) enriched
// with Component.ComName / Component.Overlay from the cache loader.
// Mirrors TS Compiler.ts:214-232.
//
// `name` is com.ComName if non-empty, else componentInfo.Map[id]
// (TS `com.comName || componentInfo.map[id]`).
//
// Per TS Compiler.ts:215, the loop bound is `id <= componentInfo.Max`
// and the inner guards are the standard `Map[id]` presence check + a
// `Configs[id] != nil` check.
func populateInterfaceOverlay(
	componentInfo, interfaceInfo, overlayInfo *TypeInfo,
	components *objtype.ComponentTypeConfigs,
) {
	if components == nil {
		return
	}
	for id := 0; id <= componentInfo.Max; id++ {
		baseName, present := componentInfo.Map[id]
		if !present {
			continue
		}
		if id < 0 || id >= len(components.Configs) {
			continue
		}
		com := components.Configs[id]
		if com == nil {
			continue
		}
		name := com.ComName
		if name == "" {
			name = baseName
		}
		interfaceInfo.Add(id, name, true)
		if com.Overlay {
			overlayInfo.Add(id, name, true)
		}
	}
}
