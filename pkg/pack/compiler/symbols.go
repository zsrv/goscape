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
// TS Compiler.ts:147 writes the corrupt2 arm to commandInfo.corrupt[opcode]
// (overwriting the corrupt arm assigned one line above) rather than to
// commandInfo.corrupt2[opcode]. Goscape preserves this behavior for
// TS parity — info.Corrupt2 is never populated.
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
				info.Corrupt[op] = strings.Join(ptrs.Corrupt2, ",")
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
	// pack-media-compiler-1: TS ScriptVarType.getType (Engine-TS/src/cache/
	// config/ScriptVarType.ts:64-79) covers these 7 cases too. goscape's
	// switch pre-fix omitted them, so any compiler symbol whose declared
	// type was autoint / varp / player_uid / npc_uid / npc_stat / idkit /
	// dbrow emitted the literal "unknown" string — affecting symbol-table
	// rendering and downstream tooling that round-trips the name.
	case objtype.ScriptVarTypeAutoInt:
		return "autoint"
	case objtype.ScriptVarTypeVarp:
		return "varp"
	case objtype.ScriptVarTypePlayerUid:
		return "player_uid"
	case objtype.ScriptVarTypeNpcUid:
		return "npc_uid"
	case objtype.ScriptVarTypeNpcStat:
		return "npc_stat"
	case objtype.ScriptVarTypeIdkit:
		return "idkit"
	case objtype.ScriptVarTypeDbrow:
		return "dbrow"
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

// enrichWriteinvInfo populates writeinv.Protect[id] from
// InvType.Protect for every id present in writeinv.Map. Mirrors TS
// Compiler.ts:203-212.
func enrichWriteinvInfo(info *TypeInfo, configs *objtype.InvTypeConfigs) {
	if configs == nil {
		return
	}
	for id := 0; id <= info.Max; id++ {
		if _, ok := info.Map[id]; !ok {
			continue
		}
		if id < 0 || id >= len(configs.Configs) {
			continue
		}
		inv := configs.Configs[id]
		if inv == nil {
			continue
		}
		info.Protect[id] = inv.Protect
	}
}

// enrichVarpInfo populates varp.VarType and varp.Protect from
// VarPlayerType fields. Mirrors TS Compiler.ts:234-243.
func enrichVarpInfo(info *TypeInfo, configs *objtype.VarpTypeConfigs) {
	if configs == nil {
		return
	}
	for id := 0; id <= info.Max; id++ {
		if _, ok := info.Map[id]; !ok {
			continue
		}
		if id < 0 || id >= len(configs.Configs) {
			continue
		}
		varp := configs.Configs[id]
		if varp == nil {
			continue
		}
		info.VarType[id] = scriptVarTypeName(varp.Type)
		info.Protect[id] = varp.Protect
	}
}

// enrichVarnInfo populates varn.VarType from VarNpcType.Type.
//
// NEW semantics (CompilerSymbols.ts:167-178 @ Engine-TS@9aadcec4):
// iterates info.Map (varn pack entries), enriching only ids present in
// varn.pack. This matches the NEW CompilerSymbols.ts which iterates
// `varns` (the varn.pack slice) directly.
//
// DEVIATION from OLD Compiler.ts:245-253 (Engine-TS@e1dea19f):
// OLD had a typo at line 247 — the loop guard read `varpInfo.map[id]`
// instead of `varnInfo.map[id]`, meaning varn ids without a varp at the
// same id were silently skipped, and varp-only ids could incorrectly
// receive a vartype write. NEW CompilerSymbols.ts fixes this by iterating
// varn entries directly. Go is updated to NEW semantics here so the
// in-memory compiler symbol table agrees with what the jar sees via
// varn.sym.
//
// rev244-b6-cs-delta3: enrichVarnInfo updated from OLD to NEW semantics.
func enrichVarnInfo(info *TypeInfo, configs *objtype.VarnTypeConfigs) {
	if configs == nil {
		return
	}
	for id := 0; id <= info.Max; id++ {
		if _, ok := info.Map[id]; !ok {
			continue
		}
		if id < 0 || id >= len(configs.Configs) {
			continue
		}
		varn := configs.Configs[id]
		if varn == nil {
			continue
		}
		info.VarType[id] = scriptVarTypeName(varn.Type)
	}
}

// enrichVarsInfo populates vars.VarType from VarSharedType.Type. Mirrors
// TS Compiler.ts:255-263.
func enrichVarsInfo(info *TypeInfo, configs *objtype.VarsTypeConfigs) {
	if configs == nil {
		return
	}
	for id := 0; id <= info.Max; id++ {
		if _, ok := info.Map[id]; !ok {
			continue
		}
		if id < 0 || id >= len(configs.Configs) {
			continue
		}
		vars := configs.Configs[id]
		if vars == nil {
			continue
		}
		info.VarType[id] = scriptVarTypeName(vars.Type)
	}
}

// enrichParamInfo populates param.VarType from ParamType.GetType(). Mirrors
// TS Compiler.ts:265-273. Uses ParamType's existing instance method
// rather than scriptVarTypeName — they share the same switch but the
// method is already exported.
func enrichParamInfo(info *TypeInfo, configs *objtype.ParamTypeConfigs) {
	if configs == nil {
		return
	}
	for id := 0; id <= info.Max; id++ {
		if _, ok := info.Map[id]; !ok {
			continue
		}
		if id < 0 || id >= len(configs.Configs) {
			continue
		}
		param := configs.Configs[id]
		if param == nil {
			continue
		}
		info.VarType[id] = param.GetType()
	}
}

// configLoaders bundles the entity-type configurations consumed by the
// enrichment passes. Unexported — internal seam for testability so
// buildSymbolsCore can be exercised with synthetic in-memory configs
// instead of binary .dat/.idx fixtures.
type configLoaders struct {
	inv     *objtype.InvTypeConfigs
	comp    *objtype.ComponentTypeConfigs
	varp    *objtype.VarpTypeConfigs
	varn    *objtype.VarnTypeConfigs
	varsCfg *objtype.VarsTypeConfigs
	param   *objtype.ParamTypeConfigs
	dbtable *objtype.DbTableTypeConfigs
}

// loadConfigs reads the 7 entity-type configurations from dataPackDir.
// Mirrors the cluster of TS .load() calls at Compiler.ts:203, 214, 234,
// 245, 255, 265, 275.
func loadConfigs(dataPackDir string) (*configLoaders, error) {
	inv, err := objtype.LoadInvTypes(dataPackDir)
	if err != nil {
		return nil, fmt.Errorf("LoadInvTypes: %w", err)
	}
	comp, err := objtype.LoadComponentTypes(dataPackDir)
	if err != nil {
		return nil, fmt.Errorf("LoadComponentTypes: %w", err)
	}
	varp, err := objtype.LoadVarpTypes(dataPackDir)
	if err != nil {
		return nil, fmt.Errorf("LoadVarpTypes: %w", err)
	}
	varn, err := objtype.LoadVarnTypes(dataPackDir)
	if err != nil {
		return nil, fmt.Errorf("LoadVarnTypes: %w", err)
	}
	varsCfg, err := objtype.LoadVarsTypes(dataPackDir)
	if err != nil {
		return nil, fmt.Errorf("LoadVarsTypes: %w", err)
	}
	param, err := objtype.LoadParamTypes(dataPackDir)
	if err != nil {
		return nil, fmt.Errorf("LoadParamTypes: %w", err)
	}
	dbtable, err := objtype.LoadDbTableTypes(dataPackDir)
	if err != nil {
		return nil, fmt.Errorf("LoadDbTableTypes: %w", err)
	}
	return &configLoaders{
		inv: inv, comp: comp, varp: varp, varn: varn,
		varsCfg: varsCfg, param: param, dbtable: dbtable,
	}, nil
}

// BuildSymbols ports TS runServerCompiler (Compiler.ts:109-329) up to —
// but not including — the final CompileServerScript({symbols}) call,
// which is deferred to NAI-203+.
//
// srcDir: path containing scripts/ and pack/ subdirs.
// dataPackDir: path containing client/ and server/ subdirs with cache
// .dat/.idx for InvType, Component, VarP, VarN, VarS, Param, DbTableType.
//
// Returns the 32-key symbol-category dict the bytecode compiler's
// typechecker consumes. Categories match TS Compiler.ts:330-365 exactly.
func BuildSymbols(srcDir, dataPackDir string) (map[string]*TypeInfo, error) {
	loaders, err := loadConfigs(dataPackDir)
	if err != nil {
		return nil, fmt.Errorf("BuildSymbols: %w", err)
	}
	return buildSymbolsCore(srcDir, loaders)
}

// buildSymbolsCore is the testable seam under BuildSymbols. Takes
// pre-loaded *configLoaders so unit tests can construct synthetic
// configs without writing binary cache fixtures.
func buildSymbolsCore(srcDir string, loaders *configLoaders) (map[string]*TypeInfo, error) {
	packDir := filepath.Join(srcDir, "pack")
	scriptsDir := filepath.Join(srcDir, "scripts")

	// 1. commandInfo from ScriptOpcodeMap + ScriptOpcodePointers.
	commandInfo := newTypeInfo()
	populateCommandInfo(commandInfo)

	// 2. constantInfo from <srcDir>/scripts/**/*.constant.
	constants, err := loadCompilerConstants(scriptsDir)
	if err != nil {
		return nil, fmt.Errorf("BuildSymbols: constants: %w", err)
	}
	constantInfo := LoadRecords(constants, false)

	// 3. 22 .pack file Loads.
	var loadErr error
	loadOrFail := func(packType string) *TypeInfo {
		p := filepath.Join(packDir, packType+".pack")
		info, lerr := Load(p)
		if lerr != nil && loadErr == nil {
			loadErr = fmt.Errorf("Load(%s): %w", packType, lerr)
		}
		return info
	}
	npcInfo := loadOrFail("npc")
	objInfo := loadOrFail("obj")
	invInfo := loadOrFail("inv")
	seqInfo := loadOrFail("seq")
	idkInfo := loadOrFail("idk")
	spotanimInfo := loadOrFail("spotanim")
	locInfo := loadOrFail("loc")
	componentInfo := loadOrFail("interface")
	interfaceInfo := newTypeInfo() // synthesized below
	overlayInfo := newTypeInfo()   // synthesized below
	varpInfo := loadOrFail("varp")
	varnInfo := loadOrFail("varn")
	varsInfo := loadOrFail("vars")
	paramInfo := loadOrFail("param")
	structInfo := loadOrFail("struct")
	enumInfo := loadOrFail("enum")
	huntInfo := loadOrFail("hunt")
	mesanimInfo := loadOrFail("mesanim")
	synthInfo := loadOrFail("synth")
	categoryInfo := loadOrFail("category")
	runescriptInfo := loadOrFail("script") // script.pack → "runescript" symbol key
	dbtableInfo := loadOrFail("dbtable")
	dbcolumnInfo := newTypeInfo() // synthesized below
	dbrowInfo := loadOrFail("dbrow")
	// TS Compiler.ts:204 re-loads inv.pack for writeinv (separate
	// TypeInfo with its own .Protect map enriched below).
	writeinvInfo := loadOrFail("inv")
	if loadErr != nil {
		return nil, fmt.Errorf("BuildSymbols: %w", loadErr)
	}

	// 4. writeinv (InvType.Protect).
	enrichWriteinvInfo(writeinvInfo, loaders.inv)

	// 5. interface / overlayinterface (Component.ComName + Component.Overlay).
	populateInterfaceOverlay(componentInfo, interfaceInfo, overlayInfo, loaders.comp)

	// 6. varp/varn/vars/param vartype + protect enrichments.
	enrichVarpInfo(varpInfo, loaders.varp)
	enrichVarnInfo(varnInfo, loaders.varn)
	enrichVarsInfo(varsInfo, loaders.varsCfg)
	enrichParamInfo(paramInfo, loaders.param)

	// 7. dbcolumn synth.
	populateDbColumns(dbcolumnInfo, loaders.dbtable)

	// 8. stat / npc_stat / npc_mode via LoadMap valueAsKey=true.
	statInfo := LoadMap(objtype.PlayerStatMap, true)
	npcStatInfo := LoadMap(objtype.NpcStatMap, true)
	npcModeInfo := LoadMap(objtype.NpcModeMap, true)

	// 9. fontmetrics / locshape (static LoadArray).
	fontmetricsInfo := LoadArray([]string{"p11", "p12", "b12", "q8"})
	locshapeInfo := LoadArray([]string{
		"wall_straight", "wall_diagonalcorner", "wall_l", "wall_squarecorner",
		"walldecor_straight_nooffset", "walldecor_straight_offset",
		"walldecor_diagonal_offset", "walldecor_diagonal_nooffset",
		"walldecor_diagonal_both", "wall_diagonal",
		"centrepiece_straight", "centrepiece_diagonal",
		"roof_straight", "roof_diagonal_with_roofedge", "roof_diagonal",
		"roof_l_concave", "roof_l_convex", "roof_flat",
		"roofedge_straight", "roofedge_diagonalcorner", "roofedge_l",
		"roofedge_squarecorner",
		"grounddecor",
	})

	// 10. Assemble the 32-key dict, mirroring TS Compiler.ts:330-365 order.
	symbols := map[string]*TypeInfo{
		"command":          commandInfo,
		"constant":         constantInfo,
		"npc":              npcInfo,
		"obj":              objInfo,
		"inv":              invInfo,
		"writeinv":         writeinvInfo,
		"seq":              seqInfo,
		"idk":              idkInfo,
		"spotanim":         spotanimInfo,
		"loc":              locInfo,
		"component":        componentInfo,
		"interface":        interfaceInfo,
		"overlayinterface": overlayInfo,
		"varp":             varpInfo,
		"varn":             varnInfo,
		"vars":             varsInfo,
		"param":            paramInfo,
		"struct":           structInfo,
		"enum":             enumInfo,
		"hunt":             huntInfo,
		"mesanim":          mesanimInfo,
		"synth":            synthInfo,
		"category":         categoryInfo,
		"runescript":       runescriptInfo,
		"dbtable":          dbtableInfo,
		"dbcolumn":         dbcolumnInfo,
		"dbrow":            dbrowInfo,
		"stat":             statInfo,
		"npc_stat":         npcStatInfo,
		"npc_mode":         npcModeInfo,
		"fontmetrics":      fontmetricsInfo,
		"locshape":         locshapeInfo,
	}

	return symbols, nil
}

// populateInterfaceOverlay derives the `interface` and `overlayinterface`
// TypeInfos from componentInfo (loaded from interface.pack) enriched
// with Component.ComName / Component.Overlay from the cache loader.
//
// NEW semantics (CompilerSymbols.ts:124-152 @ Engine-TS@9aadcec4):
// three-way split on name:
//   - name contains ':' → component (NOT added to interface or overlay here;
//     componentInfo already holds all entries from interface.pack)
//   - com.Overlay       → overlayinterface; ALSO added to interface
//     ("temporary: until compiler updates", CompilerSymbols.ts:145-148)
//   - else              → interface
//
// `name` is com.ComName if non-empty, else componentInfo.Map[id]
// (TS `com.comName || componentInfo.map[id]`).
//
// DEVIATION from OLD Compiler.ts:214-232 (Engine-TS@e1dea19f):
// OLD fed ALL entries (including colon-names) to interfaceInfo regardless
// of whether they were component-style (containing ':') entries. NEW splits
// them so only pure-interface names (no ':') + overlay-duplicates resolve
// via the `interface` symbol type in the compiler. Colon-names resolve
// exclusively via `component`.
//
// rev244-b6-cs-delta1: populateInterfaceOverlay updated to NEW three-way
// split semantics.
//
// NAI-212-D-INTERFACE-FALLBACK-FROM-COMPONENTINFO: defensive fallback
// when loaders.comp is empty or missing entries. When compile runs
// inside packall.PackAll, packall calls clientinterface.Pack first
// (packall/packall.go:51) so loaders.comp is populated and the
// fallback is dormant. Standalone callers — primarily
// `goscape-cli compile` via compiler.LoadCompilerSymbols
// (cmd_compile.go:91) — do NOT run clientinterface.Pack first, and
// LoadComponentTypes returns empty configs when the client/interface
// jagfile is missing (componenttype.go:133-134). In that state the
// fallback uses the base name's colon-presence to classify (no ComName
// override, no overlay flag) so `interface`-typed identifier lookups
// still resolve to a BasicSymbol for non-component entries. Permanent —
// removing the fallback would break `goscape-cli compile` on a fresh
// dataPackDir.
func populateInterfaceOverlay(
	componentInfo, interfaceInfo, overlayInfo *TypeInfo,
	components *objtype.ComponentTypeConfigs,
) {
	for id := 0; id <= componentInfo.Max; id++ {
		baseName, present := componentInfo.Map[id]
		if !present {
			continue
		}
		var com *objtype.ComponentType
		if components != nil && id >= 0 && id < len(components.Configs) {
			com = components.Configs[id]
		}
		name := baseName
		if com != nil && com.ComName != "" {
			name = com.ComName
		}

		// Three-way split mirroring CompilerSymbols.ts:137-148.
		if strings.Contains(name, ":") {
			// component — componentInfo already holds these; skip interface/overlay.
			continue
		}
		if com != nil && com.Overlay {
			overlayInfo.Add(id, name, true)
			// Temporary duplication into interface (CompilerSymbols.ts:145-148).
			interfaceInfo.Add(id, name, true)
		} else {
			interfaceInfo.Add(id, name, true)
		}
	}
}
