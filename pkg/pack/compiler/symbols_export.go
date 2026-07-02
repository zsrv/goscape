// Package compiler — WriteCompilerSymbols generates the 32 .sym files that
// CompilerSymbols.ts produces at 9aadcec4. These are pack-pipeline OUTPUT
// artifacts: written after configs+interface are packed, before the RuneScript
// compiler jar is invoked.
//
// CompilerSymbols.ts reference: Engine-TS@9aadcec4 tools/pack/CompilerSymbols.ts
package compiler

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/script"
)

// WriteCompilerSymbols writes all 32 .sym files to symbolsDir. It mirrors
// CompilerSymbols.ts generateCompilerSymbols() at Engine-TS@9aadcec4.
//
// srcDir: content root — has scripts/ (*.constant files) and pack/
// (*.pack files). In packall: the content directory.
// outDir: the data/pack directory holding packed server/*.dat files
// consumed by InvType, Component, VarP, VarN, VarS, Param, DbTable
// loaders. In packall: outDir (same as packall.PackAll outDir).
// symbolsDir: destination directory; created if absent. In packall: sibling
// of outDir named "symbols" (e.g. data/pack → data/symbols).
//
// All writes are atomic at the file level: each .sym is written in full then
// saved. Missing pack files silently produce empty .sym output (mirrors TS
// loadPack behaviour on absent files).
//
// Ordering contract: caller must invoke WriteCompilerSymbols AFTER
// PackConfigsForPackAll and clientinterface.Pack so the InvType, Component,
// VarPlayerType etc. .dat files in outDir are current. In packall this means
// the call sits between clientinterface.Pack and RunServerCompiler.
func WriteCompilerSymbols(srcDir, outDir, symbolsDir string) error {
	if err := os.MkdirAll(symbolsDir, 0o755); err != nil {
		return fmt.Errorf("WriteCompilerSymbols: mkdir %s: %w", symbolsDir, err)
	}

	packDir := filepath.Join(srcDir, "pack")
	scriptsDir := filepath.Join(srcDir, "scripts")

	// Load the config types that need runtime data from outDir.
	loaders, err := loadConfigs(outDir)
	if err != nil {
		return fmt.Errorf("WriteCompilerSymbols: loadConfigs: %w", err)
	}

	// ── constant.sym ────────────────────────────────────────────────────────
	// CompilerSymbols.ts:22-48: loadDir scripts/**/*.constant, write name\tvalue\n.
	// Order follows filepath.WalkDir (lexicographic) + within-file line order,
	// matching TS loadDir (which uses fs.readdirSync alphabetical on Linux) +
	// JS for-in object insertion order.
	constPairs, err := loadCompilerConstantsOrdered(scriptsDir)
	if err != nil {
		return fmt.Errorf("WriteCompilerSymbols: constants: %w", err)
	}
	if err := writeConstantSym(symbolsDir, constPairs); err != nil {
		return err
	}

	// ── npc.sym ─────────────────────────────────────────────────────────────
	// CompilerSymbols.ts:50-55: NO empty-skip (every slot written).
	npcInfo, err := Load(filepath.Join(packDir, "npc.pack"))
	if err != nil {
		return fmt.Errorf("WriteCompilerSymbols: npc.pack: %w", err)
	}
	if err := writeSimpleSym(symbolsDir, "npc", npcInfo, false); err != nil {
		return err
	}

	// ── obj.sym ─────────────────────────────────────────────────────────────
	// CompilerSymbols.ts:57-62: NO empty-skip.
	objInfo, err := Load(filepath.Join(packDir, "obj.pack"))
	if err != nil {
		return fmt.Errorf("WriteCompilerSymbols: obj.pack: %w", err)
	}
	if err := writeSimpleSym(symbolsDir, "obj", objInfo, false); err != nil {
		return err
	}

	// ── inv.sym + writeinv.sym ───────────────────────────────────────────────
	// CompilerSymbols.ts:64-78: load InvType from data/pack; iterate inv.pack
	// with empty-skip. inv.sym = id\tdebugname\n; writeinv.sym =
	// id\tdebugname\tnone\t<protect bool>\n.
	invInfo, err := Load(filepath.Join(packDir, "inv.pack"))
	if err != nil {
		return fmt.Errorf("WriteCompilerSymbols: inv.pack: %w", err)
	}
	if err := writeInvSyms(symbolsDir, invInfo, loaders.inv); err != nil {
		return err
	}

	// ── seq.sym ─────────────────────────────────────────────────────────────
	// CompilerSymbols.ts:80-89: empty-skip.
	if err := writeSimplePackSym(symbolsDir, "seq", packDir, true); err != nil {
		return err
	}

	// ── idk.sym ─────────────────────────────────────────────────────────────
	// CompilerSymbols.ts:91-100: empty-skip.
	if err := writeSimplePackSym(symbolsDir, "idk", packDir, true); err != nil {
		return err
	}

	// ── spotanim.sym ─────────────────────────────────────────────────────────
	// CompilerSymbols.ts:102-111: empty-skip.
	if err := writeSimplePackSym(symbolsDir, "spotanim", packDir, true); err != nil {
		return err
	}

	// ── loc.sym ─────────────────────────────────────────────────────────────
	// CompilerSymbols.ts:113-122: empty-skip.
	if err := writeSimplePackSym(symbolsDir, "loc", packDir, true); err != nil {
		return err
	}

	// ── component.sym + interface.sym + overlayinterface.sym ─────────────────
	// CompilerSymbols.ts:124-152: three-way split on name.indexOf(':') and
	// com.overlay. Overlay entries are ALSO duplicated into interface.sym
	// ("temporary: until compiler updates", line 145-148).
	compInfo, err := Load(filepath.Join(packDir, "interface.pack"))
	if err != nil {
		return fmt.Errorf("WriteCompilerSymbols: interface.pack: %w", err)
	}
	if err := writeInterfaceSyms(symbolsDir, compInfo, loaders.comp); err != nil {
		return err
	}

	// ── varp.sym ─────────────────────────────────────────────────────────────
	// CompilerSymbols.ts:154-165: id\tdebugname\t<type>\t<protect>\n, empty-skip.
	varpPackInfo, err := Load(filepath.Join(packDir, "varp.pack"))
	if err != nil {
		return fmt.Errorf("WriteCompilerSymbols: varp.pack: %w", err)
	}
	if err := writeVarpSym(symbolsDir, varpPackInfo, loaders.varp); err != nil {
		return err
	}

	// ── varn.sym ─────────────────────────────────────────────────────────────
	// CompilerSymbols.ts:167-178: id\tdebugname\t<type>\n, empty-skip.
	// NEW semantics: iterates varn.pack entries (not varp as OLD Compiler.ts:246 did).
	varnPackInfo, err := Load(filepath.Join(packDir, "varn.pack"))
	if err != nil {
		return fmt.Errorf("WriteCompilerSymbols: varn.pack: %w", err)
	}
	if err := writeVarnSym(symbolsDir, varnPackInfo, loaders.varn); err != nil {
		return err
	}

	// ── vars.sym ─────────────────────────────────────────────────────────────
	// CompilerSymbols.ts:180-191: id\tdebugname\t<type>\n, empty-skip.
	varsPackInfo, err := Load(filepath.Join(packDir, "vars.pack"))
	if err != nil {
		return fmt.Errorf("WriteCompilerSymbols: vars.pack: %w", err)
	}
	if err := writeVarsSym(symbolsDir, varsPackInfo, loaders.varsCfg); err != nil {
		return err
	}

	// ── param.sym ────────────────────────────────────────────────────────────
	// CompilerSymbols.ts:193-204: id\tdebugname\t<paramtype>\n, empty-skip.
	paramPackInfo, err := Load(filepath.Join(packDir, "param.pack"))
	if err != nil {
		return fmt.Errorf("WriteCompilerSymbols: param.pack: %w", err)
	}
	if err := writeParamSym(symbolsDir, paramPackInfo, loaders.param); err != nil {
		return err
	}

	// ── struct.sym ───────────────────────────────────────────────────────────
	// CompilerSymbols.ts:206-211: NO empty-skip (raw iteration).
	structInfo, err := Load(filepath.Join(packDir, "struct.pack"))
	if err != nil {
		return fmt.Errorf("WriteCompilerSymbols: struct.pack: %w", err)
	}
	if err := writeSimpleSym(symbolsDir, "struct", structInfo, false); err != nil {
		return err
	}

	// ── enum.sym ─────────────────────────────────────────────────────────────
	// CompilerSymbols.ts:213-218: NO empty-skip (raw iteration).
	enumInfo, err := Load(filepath.Join(packDir, "enum.pack"))
	if err != nil {
		return fmt.Errorf("WriteCompilerSymbols: enum.pack: %w", err)
	}
	if err := writeSimpleSym(symbolsDir, "enum", enumInfo, false); err != nil {
		return err
	}

	// ── hunt.sym ─────────────────────────────────────────────────────────────
	// CompilerSymbols.ts:220-225: NO empty-skip (raw iteration).
	huntInfo, err := Load(filepath.Join(packDir, "hunt.pack"))
	if err != nil {
		return fmt.Errorf("WriteCompilerSymbols: hunt.pack: %w", err)
	}
	if err := writeSimpleSym(symbolsDir, "hunt", huntInfo, false); err != nil {
		return err
	}

	// ── mesanim.sym ──────────────────────────────────────────────────────────
	// CompilerSymbols.ts:227-232: NO empty-skip (raw iteration).
	mesanimInfo, err := Load(filepath.Join(packDir, "mesanim.pack"))
	if err != nil {
		return fmt.Errorf("WriteCompilerSymbols: mesanim.pack: %w", err)
	}
	if err := writeSimpleSym(symbolsDir, "mesanim", mesanimInfo, false); err != nil {
		return err
	}

	// ── synth.sym ────────────────────────────────────────────────────────────
	// CompilerSymbols.ts:234-239: NO empty-skip (raw iteration).
	synthInfo, err := Load(filepath.Join(packDir, "synth.pack"))
	if err != nil {
		return fmt.Errorf("WriteCompilerSymbols: synth.pack: %w", err)
	}
	if err := writeSimpleSym(symbolsDir, "synth", synthInfo, false); err != nil {
		return err
	}

	// ── category.sym ─────────────────────────────────────────────────────────
	// CompilerSymbols.ts:241-250: empty-skip.
	if err := writeSimplePackSym(symbolsDir, "category", packDir, true); err != nil {
		return err
	}

	// ── runescript.sym ───────────────────────────────────────────────────────
	// CompilerSymbols.ts:252-261: empty-skip; loaded from script.pack.
	if err := writeSimplePackSym(symbolsDir, "runescript", packDir, true); err != nil {
		return err
	}

	// ── commands.sym ─────────────────────────────────────────────────────────
	// CompilerSymbols.ts:263-324.
	if err := writeCommandsSym(symbolsDir); err != nil {
		return err
	}

	// ── dbtable.sym + dbcolumn.sym ───────────────────────────────────────────
	// CompilerSymbols.ts:326-353.
	dbtableInfo, err := Load(filepath.Join(packDir, "dbtable.pack"))
	if err != nil {
		return fmt.Errorf("WriteCompilerSymbols: dbtable.pack: %w", err)
	}
	if err := writeDbSyms(symbolsDir, dbtableInfo, loaders.dbtable); err != nil {
		return err
	}

	// ── dbrow.sym ────────────────────────────────────────────────────────────
	// CompilerSymbols.ts:355-364: empty-skip.
	if err := writeSimplePackSym(symbolsDir, "dbrow", packDir, true); err != nil {
		return err
	}

	// ── stat.sym ─────────────────────────────────────────────────────────────
	// CompilerSymbols.ts:366-369: PlayerStatMap sorted by value, value\tlowername\n.
	if err := writeMapSym(symbolsDir, "stat", objtype.PlayerStatMap); err != nil {
		return err
	}

	// ── npc_stat.sym ─────────────────────────────────────────────────────────
	// CompilerSymbols.ts:371-374.
	if err := writeMapSym(symbolsDir, "npc_stat", objtype.NpcStatMap); err != nil {
		return err
	}

	// ── locshape.sym ─────────────────────────────────────────────────────────
	// CompilerSymbols.ts:376-402.
	locshapes := []string{
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
	}
	if err := writeIndexedSym(symbolsDir, "locshape", locshapes); err != nil {
		return err
	}

	// ── fontmetrics.sym ──────────────────────────────────────────────────────
	// CompilerSymbols.ts:404-405.
	if err := writeIndexedSym(symbolsDir, "fontmetrics", []string{"p11", "p12", "b12", "q8"}); err != nil {
		return err
	}

	// ── npc_mode.sym ─────────────────────────────────────────────────────────
	// CompilerSymbols.ts:407-410.
	return writeMapSym(symbolsDir, "npc_mode", objtype.NpcModeMap)
}

// ─── helpers ───────────────────────────────────────────────────────────────

// loadCompilerConstantsOrdered walks scriptsDir recursively for *.constant
// files (in filepath.WalkDir order, which is lexicographic on Linux) and
// returns the constants as an ordered slice of [name, value] pairs, in the
// order they were inserted (per-file line order, last-writer-wins for
// duplicates: only the LAST entry for a given name appears, at its last
// position). The result follows TS FsCache.ts listDir alphabetical traversal
// order + within-file line order, matching the reference constant.sym output.
//
// This is intentionally separate from loadCompilerConstants (which returns
// a map) so the exporter can produce an ordered constant.sym.
func loadCompilerConstantsOrdered(scriptsDir string) ([][2]string, error) {
	// Two-pass: first collect all (name, value, seqNo) triples tracking
	// insertion position; last-writer-wins replaces value + position.
	type entry struct {
		value string
		seq   int
	}
	m := map[string]*entry{}
	seq := 0

	info, err := os.Stat(scriptsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
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
		for lineNo, raw := range strings.Split(string(data), "\n") {
			line := strings.TrimSuffix(raw, "\r")
			if len(line) == 0 || strings.HasPrefix(line, "//") {
				continue
			}
			parts := strings.SplitN(line, "=", 3)
			if len(parts) < 2 {
				return fmt.Errorf("%s:%d: line missing '=': %q", path, lineNo+1, line)
			}
			name := strings.TrimPrefix(strings.TrimSpace(parts[0]), "^")
			value := strings.TrimSpace(parts[1])
			if len(value) >= 2 && strings.HasPrefix(value, "\"") && strings.HasSuffix(value, "\"") {
				value = value[1 : len(value)-1]
			}
			if e, ok := m[name]; ok {
				// Last-writer-wins: update value + move to current position.
				e.value = value
				e.seq = seq
			} else {
				m[name] = &entry{value: value, seq: seq}
			}
			seq++
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	// Collect and sort by final seq to get insertion order.
	type kv struct {
		name  string
		value string
		seq   int
	}
	pairs := make([]kv, 0, len(m))
	for k, e := range m {
		pairs = append(pairs, kv{k, e.value, e.seq})
	}
	slices.SortFunc(pairs, func(a, b kv) int {
		return a.seq - b.seq
	})

	result := make([][2]string, len(pairs))
	for i, p := range pairs {
		result[i] = [2]string{p.name, p.value}
	}
	return result, nil
}

// writeConstantSym writes constant.sym from ordered name→value pairs.
// CompilerSymbols.ts:44-48: `for (const name in constants) { name\tvalue\n }`.
// Order follows file-walk + within-file line insertion order (TS-faithful).
func writeConstantSym(symbolsDir string, constants [][2]string) error {
	var sb strings.Builder
	for _, kv := range constants {
		sb.WriteString(kv[0])
		sb.WriteByte('\t')
		sb.WriteString(kv[1])
		sb.WriteByte('\n')
	}
	return writeSym(symbolsDir, "constant", sb.String())
}

// writeSimpleSym writes a sym file with format `id\tname\n` for each entry
// in info.Map. If skipEmpty, entries absent from info.Map (i.e. not assigned)
// between 0 and info.Max are omitted. If !skipEmpty, every id 0..lastId is
// written (raw-iteration semantics for struct/enum/hunt/mesanim/synth).
//
// Loop bound and bounds semantics:
//   - skipEmpty=true  (seq/idk/spotanim/loc/category/runescript/dbrow):
//     iterate 0..Max inclusive; absent ids are skipped. TypeInfo.Max may
//     equal lastId (not lastId+1) for sequentially-packed files, so we use
//     `<= Max` (same as all TypeInfo enrichment helpers in symbols.go).
//   - skipEmpty=false (npc/obj/struct/enum/hunt/mesanim/synth):
//     iterate 0..lastKey (the maximum id in Map) inclusive. TypeInfo.Max can
//     be lastId+1 (when the update condition `Max < id` fired for the last
//     entry), in which case iterating `<= Max` would write an extra empty-name
//     entry at id=Max. Using the actual max key avoids the spurious extra line.
//
// Note: for npc and obj, TS uses a simple slice (loadPack) with no skip;
// for those we iterate 0..len-1 writing every slot including empty string ""
// (they are never empty in practice for valid content). skipEmpty=false
// reproduces that.
func writeSimpleSym(symbolsDir, name string, info *TypeInfo, skipEmpty bool) error {
	if info.Max < 0 || len(info.Map) == 0 {
		return writeSym(symbolsDir, name, "")
	}
	var sb strings.Builder
	if skipEmpty {
		// Use <= Max (inclusive) — some files leave Max == lastId.
		for i := 0; i <= info.Max; i++ {
			v, ok := info.Map[i]
			if !ok {
				continue
			}
			fmt.Fprintf(&sb, "%d\t%s\n", i, v)
		}
	} else {
		// Find the actual maximum key to avoid writing a spurious empty entry
		// at id=Max when Max == lastId+1 (sequential even-id files).
		lastKey := 0
		for k := range info.Map {
			if k > lastKey {
				lastKey = k
			}
		}
		for i := 0; i <= lastKey; i++ {
			v := info.Map[i] // empty string if absent (TS: sparse array slot = undefined → "")
			fmt.Fprintf(&sb, "%d\t%s\n", i, v)
		}
	}
	return writeSym(symbolsDir, name, sb.String())
}

// writeSimplePackSym loads pack/<packType>.pack and writes <symName>.sym.
// Uses skipEmpty=true (empty-skip semantics for: seq, idk, spotanim, loc,
// category, runescript, dbrow).
// runescript.sym is written from script.pack but under the name "runescript".
func writeSimplePackSym(symbolsDir, symName, packDir string, skipEmpty bool) error {
	packName := symName
	if symName == "runescript" {
		packName = "script"
	}
	info, err := Load(filepath.Join(packDir, packName+".pack"))
	if err != nil {
		return fmt.Errorf("writeSimplePackSym %s: %w", symName, err)
	}
	return writeSimpleSym(symbolsDir, symName, info, skipEmpty)
}

// writeInvSyms writes inv.sym and writeinv.sym.
// CompilerSymbols.ts:64-78.
// inv.sym:       id\tdebugname\n
// writeinv.sym:  id\tdebugname\tnone\t<protect>\n
// Empty-skip: entries absent from inv.pack are skipped (invs[i] falsy in TS).
func writeInvSyms(symbolsDir string, invInfo *TypeInfo, configs *objtype.InvTypeConfigs) error {
	var invSB, writeSB strings.Builder
	for i := 0; i <= invInfo.Max; i++ {
		_, ok := invInfo.Map[i]
		if !ok {
			continue
		}
		debugname := ""
		protect := false
		if configs != nil && i < len(configs.Configs) && configs.Configs[i] != nil {
			debugname = configs.Configs[i].DebugName
			protect = configs.Configs[i].Protect
		}
		fmt.Fprintf(&invSB, "%d\t%s\n", i, debugname)
		fmt.Fprintf(&writeSB, "%d\t%s\tnone\t%v\n", i, debugname, protect)
	}
	if err := writeSym(symbolsDir, "inv", invSB.String()); err != nil {
		return err
	}
	return writeSym(symbolsDir, "writeinv", writeSB.String())
}

// writeInterfaceSyms writes component.sym, interface.sym, overlayinterface.sym.
// CompilerSymbols.ts:124-152.
//
// Three-way split on name (= com.comName || pack name):
//   - name.indexOf(':') != -1  → component.sym
//   - com.overlay              → overlayinterface.sym
//   - else                     → interface.sym
//
// Additionally, overlay entries are ALSO written to interface.sym
// ("temporary: until compiler updates", CompilerSymbols.ts:145-148).
//
// Skips: absent entries (empty-skip) and "null:null" (handled by Load).
func writeInterfaceSyms(
	symbolsDir string,
	compInfo *TypeInfo,
	components *objtype.ComponentTypeConfigs,
) error {
	var comSB, ifSB, overlaySB strings.Builder
	for i := 0; i <= compInfo.Max; i++ {
		baseName, ok := compInfo.Map[i]
		if !ok {
			continue
		}
		// Resolve name = com.comName || pack name.
		name := baseName
		overlay := false
		if components != nil && i >= 0 && i < len(components.Configs) && components.Configs[i] != nil {
			com := components.Configs[i]
			if com.ComName != "" {
				name = com.ComName
			}
			overlay = com.Overlay
		}

		if strings.Contains(name, ":") {
			fmt.Fprintf(&comSB, "%d\t%s\n", i, name)
		} else if overlay {
			fmt.Fprintf(&overlaySB, "%d\t%s\n", i, name)
			// Temporary: until compiler updates (CompilerSymbols.ts:145-148).
			fmt.Fprintf(&ifSB, "%d\t%s\n", i, name)
		} else {
			fmt.Fprintf(&ifSB, "%d\t%s\n", i, name)
		}
	}
	if err := writeSym(symbolsDir, "component", comSB.String()); err != nil {
		return err
	}
	if err := writeSym(symbolsDir, "interface", ifSB.String()); err != nil {
		return err
	}
	return writeSym(symbolsDir, "overlayinterface", overlaySB.String())
}

// writeVarpSym writes varp.sym.
// CompilerSymbols.ts:154-165: id\tdebugname\t<type>\t<protect>\n, empty-skip.
func writeVarpSym(symbolsDir string, varpInfo *TypeInfo, configs *objtype.VarpTypeConfigs) error {
	var sb strings.Builder
	for i := 0; i <= varpInfo.Max; i++ {
		_, ok := varpInfo.Map[i]
		if !ok {
			continue
		}
		debugname := ""
		typeName := "int"
		protect := false
		if configs != nil && i < len(configs.Configs) && configs.Configs[i] != nil {
			v := configs.Configs[i]
			debugname = v.DebugName
			typeName = scriptVarTypeName(v.Type)
			protect = v.Protect
		}
		fmt.Fprintf(&sb, "%d\t%s\t%s\t%v\n", i, debugname, typeName, protect)
	}
	return writeSym(symbolsDir, "varp", sb.String())
}

// writeVarnSym writes varn.sym.
// CompilerSymbols.ts:167-178: id\tdebugname\t<type>\n, empty-skip.
// NEW semantics: iterates varn.pack (not varpInfo as OLD Compiler.ts:246 did).
// See symbols.go enrichVarnInfo for the OLD bridge bug note.
func writeVarnSym(symbolsDir string, varnInfo *TypeInfo, configs *objtype.VarnTypeConfigs) error {
	var sb strings.Builder
	for i := 0; i <= varnInfo.Max; i++ {
		_, ok := varnInfo.Map[i]
		if !ok {
			continue
		}
		debugname := ""
		typeName := "int"
		if configs != nil && i < len(configs.Configs) && configs.Configs[i] != nil {
			v := configs.Configs[i]
			debugname = v.DebugName
			typeName = scriptVarTypeName(v.Type)
		}
		fmt.Fprintf(&sb, "%d\t%s\t%s\n", i, debugname, typeName)
	}
	return writeSym(symbolsDir, "varn", sb.String())
}

// writeVarsSym writes vars.sym.
// CompilerSymbols.ts:180-191: id\tdebugname\t<type>\n, empty-skip.
func writeVarsSym(symbolsDir string, varsInfo *TypeInfo, configs *objtype.VarsTypeConfigs) error {
	var sb strings.Builder
	for i := 0; i <= varsInfo.Max; i++ {
		_, ok := varsInfo.Map[i]
		if !ok {
			continue
		}
		debugname := ""
		typeName := "int"
		if configs != nil && i < len(configs.Configs) && configs.Configs[i] != nil {
			v := configs.Configs[i]
			debugname = v.DebugName
			typeName = scriptVarTypeName(v.Type)
		}
		fmt.Fprintf(&sb, "%d\t%s\t%s\n", i, debugname, typeName)
	}
	return writeSym(symbolsDir, "vars", sb.String())
}

// writeParamSym writes param.sym.
// CompilerSymbols.ts:193-204: id\tdebugname\t<paramtype>\n, empty-skip.
func writeParamSym(symbolsDir string, paramInfo *TypeInfo, configs *objtype.ParamTypeConfigs) error {
	var sb strings.Builder
	for i := 0; i <= paramInfo.Max; i++ {
		_, ok := paramInfo.Map[i]
		if !ok {
			continue
		}
		debugname := ""
		typeName := "int"
		if configs != nil && i < len(configs.Configs) && configs.Configs[i] != nil {
			p := configs.Configs[i]
			debugname = p.DebugName
			typeName = p.GetType()
		}
		fmt.Fprintf(&sb, "%d\t%s\t%s\n", i, debugname, typeName)
	}
	return writeSym(symbolsDir, "param", sb.String())
}

// writeCommandsSym writes commands.sym.
// CompilerSymbols.ts:263-324.
//
// Format for opcodes WITHOUT pointers: opcode\tname\n
// Format for opcodes WITH pointers:
//
//	opcode\tname\t<require>\t<set>\t<corrupt>\n
//
// where:
//   - require = pointers.require.join(',') or 'none';
//     if require2 → append ':' + require2.join(',')
//   - set = ['CONDITIONAL:' if conditional] + pointers.set.join(',') or 'none';
//     if set2 → append ':' + set2.join(',')
//   - corrupt = pointers.corrupt.join(',') or 'none';
//     if corrupt2 → append ':' + corrupt2.join(',')
//
// Entries are sorted by opcode value (ascending), mirroring the TS
// `Array.from(ScriptOpcodeMap.entries()).sort((a, b) => a[1] - b[1])`.
//
// NOTE: the raw pointer-name strings from script.ScriptOpcodePointers are
// used directly (e.g. "active_player2", not ".active_player"). The
// NAI-212-D-POINTER-NAME-TRANSLATION translation in run_server_compiler.go
// applies only to the in-memory compiler symbol table, not to .sym files.
func writeCommandsSym(symbolsDir string) error {
	type entry struct {
		name   string
		opcode script.Opcode
	}
	entries := make([]entry, 0, len(script.ScriptOpcodeMap))
	for n, op := range script.ScriptOpcodeMap {
		entries = append(entries, entry{n, op})
	}
	slices.SortFunc(entries, func(a, b entry) int {
		return int(a.opcode) - int(b.opcode)
	})

	var sb strings.Builder
	for _, e := range entries {
		ptrs, hasPointers := script.ScriptOpcodePointers[e.opcode]
		name := strings.ToLower(e.name)

		if !hasPointers {
			fmt.Fprintf(&sb, "%d\t%s\n", e.opcode, name)
			continue
		}

		// require column
		var require string
		if len(ptrs.Require) > 0 {
			require = strings.Join(ptrs.Require, ",")
			if len(ptrs.Require2) > 0 {
				require += ":" + strings.Join(ptrs.Require2, ",")
			}
		} else {
			require = "none"
		}

		// set column
		var set string
		if len(ptrs.Set) > 0 {
			if ptrs.Conditional {
				set = "CONDITIONAL:"
			}
			set += strings.Join(ptrs.Set, ",")
			if len(ptrs.Set2) > 0 {
				set += ":" + strings.Join(ptrs.Set2, ",")
			}
		} else {
			set = "none"
		}

		// corrupt column
		var corrupt string
		if len(ptrs.Corrupt) > 0 {
			corrupt = strings.Join(ptrs.Corrupt, ",")
			if len(ptrs.Corrupt2) > 0 {
				corrupt += ":" + strings.Join(ptrs.Corrupt2, ",")
			}
		} else {
			corrupt = "none"
		}

		fmt.Fprintf(&sb, "%d\t%s\t%s\t%s\t%s\n", e.opcode, name, require, set, corrupt)
	}
	return writeSym(symbolsDir, "commands", sb.String())
}

// writeDbSyms writes dbtable.sym and dbcolumn.sym.
// CompilerSymbols.ts:326-353.
//
// dbtable.sym: id\tname\n (empty-skip).
// dbcolumn.sym per table column:
//
//	primary_key\tdebugname:colname\ttypes_joined\n
//	tuple_key\tdebugname:colname:tupleN\ttypes[n]\n  (only if len(types) > 1)
//
// primary_key  = ((table.id & 0xffff) << 12) | ((col & 0x7f) << 4)
// tuple_key    = primary_key | ((tuple + 1) & 0xf)
func writeDbSyms(
	symbolsDir string,
	dbtableInfo *TypeInfo,
	tables *objtype.DbTableTypeConfigs,
) error {
	var tableSB, colSB strings.Builder
	for i := 0; i <= dbtableInfo.Max; i++ {
		name, ok := dbtableInfo.Map[i]
		if !ok {
			continue
		}
		fmt.Fprintf(&tableSB, "%d\t%s\n", i, name)

		if tables == nil || i >= len(tables.Configs) || tables.Configs[i] == nil {
			continue
		}
		table := tables.Configs[i]
		for col, types := range table.Types {
			if types == nil {
				continue
			}
			colName := ""
			if col < len(table.ColumnNames) {
				colName = table.ColumnNames[col]
			}
			typeNames := make([]string, len(types))
			for ti, t := range types {
				typeNames[ti] = scriptVarTypeName(t)
			}

			primary := ((table.ID & 0xffff) << 12) | ((col & 0x7f) << 4)
			fmt.Fprintf(&colSB, "%d\t%s:%s\t%s\n", primary, table.DebugName, colName, strings.Join(typeNames, ","))

			if len(types) > 1 {
				for tuple := range types {
					tupleID := primary | ((tuple + 1) & 0xf)
					fmt.Fprintf(&colSB, "%d\t%s:%s:%d\t%s\n", tupleID, table.DebugName, colName, tuple, typeNames[tuple])
				}
			}
		}
	}
	if err := writeSym(symbolsDir, "dbtable", tableSB.String()); err != nil {
		return err
	}
	return writeSym(symbolsDir, "dbcolumn", colSB.String())
}

// writeMapSym writes a sym file from a map[string]int sorted by value.
// CompilerSymbols.ts pattern for stat/npc_stat/npc_mode:
// entries sorted by value ascending; format: value\tlowername\n.
// Trailing newline appended (TS .join('\n') + '\n').
func writeMapSym(symbolsDir, name string, m map[string]int) error {
	type kv struct {
		k string
		v int
	}
	pairs := make([]kv, 0, len(m))
	for k, v := range m {
		pairs = append(pairs, kv{k, v})
	}
	slices.SortFunc(pairs, func(a, b kv) int {
		if a.v != b.v {
			return a.v - b.v
		}
		return strings.Compare(a.k, b.k)
	})

	var sb strings.Builder
	for _, p := range pairs {
		fmt.Fprintf(&sb, "%d\t%s\n", p.v, strings.ToLower(p.k))
	}
	return writeSym(symbolsDir, name, sb.String())
}

// writeIndexedSym writes a sym file from a slice: index\tname\n.
// CompilerSymbols.ts pattern for locshape/fontmetrics:
// `.map((name, index) => `${index}\t${name}`).join('\n') + '\n'`
func writeIndexedSym(symbolsDir, name string, values []string) error {
	var sb strings.Builder
	for i, v := range values {
		fmt.Fprintf(&sb, "%d\t%s\n", i, v)
	}
	return writeSym(symbolsDir, name, sb.String())
}

// writeSym atomically writes content to symbolsDir/<name>.sym.
func writeSym(symbolsDir, name, content string) error {
	path := filepath.Join(symbolsDir, name+".sym")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("writeSym %s: %w", name, err)
	}
	return nil
}
