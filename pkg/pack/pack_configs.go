package pack

import (
	"fmt"
	"path/filepath"

	"github.com/zsrv/goscape/pkg/io/jagfile"
	"github.com/zsrv/goscape/pkg/objtype"
)

// PackConfigs runs the per-config packing pipeline. NAI-191–195 wired
// .varp/.varn/.vars/.param/.enum/.inv/.mesanim/.struct. NAI-196 wires
// .loc/.npc/.obj and re-orders the pipeline to TS-canonical layout per
// tools/pack/config/PackShared.ts:261-669. NAI-197 wires
// .seq/.flo/.spotanim/.idk. NAI-198 wires .hunt/.dbtable/.dbrow and
// closes the per-config layer (18/18 TS configs ported).
//
// Server outputs land at <outDir>/server/<type>.{dat,idx}.
// Client outputs land in a fresh jagfile at <outDir>/client/config.
//
// The three var-domain PackFiles (varp/varn/vars) are constructed
// up-front so the cross-domain uniqueness check has all three name
// maps available. Each *.pack file is small (<1 KB); cost is fixed.
//
// NAI-193-D-PACKFILE-SINGLETONS-DEFERRED: TS uses module-level
// VarpPack/VarnPack/VarsPack singletons; goscape constructs *PackFile
// from srcDir per call (continuation of NAI-191 §2 / NAI-192).
//
// NAI-191-D-VALIDATE-FLAGS-DEFERRED: TS BUILD_VERIFY callback (.varp
// magic 705633567 at PackShared.ts:631-633) deferred — continuation
// of NAI-191 §2.
//
// NAI-194-D-PARAM-EMPTY-CLIENT-FAITHFUL: .param contributes empty
// param.dat/param.idx to client jagfile; TS callback is no-op (does
// not contribute to client jag). Preserved for client-jagfile entry
// completeness.
//
// NAI-196-D-UNCONDITIONAL-CLIENT-PACK: .param, .seq, .loc, .flo,
// .spotanim, .npc, .obj, .idk, .varp run on EVERY PackConfigs
// invocation regardless of source freshness, matching TS
// PackShared.ts:337 (`const rebuildClient = true`) which ungates
// shouldBuild on the eight configs that write to client jag and —
// per NAI-196 §"R5 resolution" — also on .param so that all
// client-jagfile entries are always present. NAI-197 extends the
// scope to the four additional client+server configs ported in that
// slice (.seq, .flo, .spotanim, .idk). The server-only six (.enum,
// .inv, .mesanim, .struct, .varn, .vars) retain their ShouldBuild +
// GetLatestModified freshness gates.
//
// NAI-192-D-NO-SRC-NO-OP: applies only to the nine server-only
// freshness-gated branches (.enum, .inv, .mesanim, .struct, .dbtable,
// .dbrow, .hunt, .varn, .vars). The nine unconditional branches always
// run; an empty source directory produces an empty .dat/.idx pair
// (matching TS shouldBuild-output-missing arm).
//
// NAI-195-D-DEADBRANCH-OMITTED: per-config parsers omit dead TS
// branches (empty stringKeys/numberKeys/booleanKeys arrays).
//
// TS source: tools/pack/config/PackShared.ts:261-669 (packConfigs).
func PackConfigs(srcDir, outDir string) error {
	constants, err := LoadConstants(srcDir)
	if err != nil {
		return err
	}

	// Construct all three var-domain PackFiles up-front for the
	// cross-domain uniqueness check across all three name maps.
	varpPack, err := NewPackFile(srcDir, "varp", nil)
	if err != nil {
		return err
	}
	varnPack, err := NewPackFile(srcDir, "varn", nil)
	if err != nil {
		return err
	}
	varsPack, err := NewPackFile(srcDir, "vars", nil)
	if err != nil {
		return err
	}

	if err := checkVarNameUniqueness(varpPack, varnPack, varsPack); err != nil {
		return err
	}

	scriptsDir := filepath.Join(srcDir, "scripts")
	serverOut := filepath.Join(outDir, "server")
	clientOut := filepath.Join(outDir, "client")

	// Fresh client jagfile; saved unconditionally at end of pipeline
	// per NAI-196-D-UNCONDITIONAL-CLIENT-PACK.
	clientJag, err := jagfile.NewJagfile(nil)
	if err != nil {
		return err
	}

	// Lazy registry helpers reused across multiple branches.
	var (
		lk           *paramLookups
		objPack      *PackFile
		seqPack      *PackFile
		locPack      *PackFile
		npcPack      *PackFile
		modelPack    *PackFile
		categoryPack *PackFile
		huntPack     *PackFile
		texturePack  *PackFile
		animPack     *PackFile
		floPack      *PackFile
		spotanimPack *PackFile
		idkPack      *PackFile
		paramPack    *PackFile // NAI-198: lazy-promoted from eager at top of fn
		invPack      *PackFile // NAI-198: lazy-promoted from inline inside .inv branch
		dbtablePack  *PackFile // NAI-198
		dbrowPack    *PackFile // NAI-198
	)
	ensureLk := func() error {
		if lk != nil {
			return nil
		}
		newLk, err := loadParamLookups(srcDir, varpPack)
		if err != nil {
			return err
		}
		lk = newLk
		return nil
	}
	ensureObjPack := func() error {
		if objPack != nil {
			return nil
		}
		pf, err := NewPackFile(srcDir, "obj", nil)
		if err != nil {
			return err
		}
		objPack = pf
		return nil
	}
	ensureSeqPack := func() error {
		if seqPack != nil {
			return nil
		}
		pf, err := NewPackFile(srcDir, "seq", nil)
		if err != nil {
			return err
		}
		seqPack = pf
		return nil
	}
	ensureLocPack := func() error {
		if locPack != nil {
			return nil
		}
		pf, err := NewPackFile(srcDir, "loc", nil)
		if err != nil {
			return err
		}
		locPack = pf
		return nil
	}
	ensureNpcPack := func() error {
		if npcPack != nil {
			return nil
		}
		pf, err := NewPackFile(srcDir, "npc", nil)
		if err != nil {
			return err
		}
		npcPack = pf
		return nil
	}
	ensureModelPack := func() error {
		if modelPack != nil {
			return nil
		}
		pf, err := NewPackFile(srcDir, "model", nil)
		if err != nil {
			return err
		}
		modelPack = pf
		return nil
	}
	ensureCategoryPack := func() error {
		if categoryPack != nil {
			return nil
		}
		pf, err := NewPackFile(srcDir, "category", nil)
		if err != nil {
			return err
		}
		categoryPack = pf
		return nil
	}
	ensureHuntPack := func() error {
		if huntPack != nil {
			return nil
		}
		pf, err := NewPackFile(srcDir, "hunt", nil)
		if err != nil {
			return err
		}
		huntPack = pf
		return nil
	}
	ensureTexturePack := func() error {
		if texturePack != nil {
			return nil
		}
		pf, err := NewPackFile(srcDir, "texture", nil)
		if err != nil {
			return err
		}
		texturePack = pf
		return nil
	}
	ensureAnimPack := func() error {
		if animPack != nil {
			return nil
		}
		pf, err := NewPackFile(srcDir, "anim", nil)
		if err != nil {
			return err
		}
		animPack = pf
		return nil
	}
	ensureFloPack := func() error {
		if floPack != nil {
			return nil
		}
		pf, err := NewPackFile(srcDir, "flo", nil)
		if err != nil {
			return err
		}
		floPack = pf
		return nil
	}
	ensureSpotAnimPack := func() error {
		if spotanimPack != nil {
			return nil
		}
		pf, err := NewPackFile(srcDir, "spotanim", nil)
		if err != nil {
			return err
		}
		spotanimPack = pf
		return nil
	}
	ensureIdkPack := func() error {
		if idkPack != nil {
			return nil
		}
		pf, err := NewPackFile(srcDir, "idk", nil)
		if err != nil {
			return err
		}
		idkPack = pf
		return nil
	}
	ensureParamPack := func() error {
		if paramPack != nil {
			return nil
		}
		pf, err := NewPackFile(srcDir, "param", nil)
		if err != nil {
			return err
		}
		paramPack = pf
		return nil
	}
	ensureInvPack := func() error {
		if invPack != nil {
			return nil
		}
		pf, err := NewPackFile(srcDir, "inv", nil)
		if err != nil {
			return err
		}
		invPack = pf
		return nil
	}
	ensureDbTablePack := func() error {
		if dbtablePack != nil {
			return nil
		}
		pf, err := NewPackFile(srcDir, "dbtable", nil)
		if err != nil {
			return err
		}
		dbtablePack = pf
		return nil
	}
	ensureDbRowPack := func() error {
		if dbrowPack != nil {
			return nil
		}
		pf, err := NewPackFile(srcDir, "dbrow", nil)
		if err != nil {
			return err
		}
		dbrowPack = pf
		return nil
	}
	// .param — unconditional (NAI-196-D-UNCONDITIONAL-CLIENT-PACK).
	// Matches TS PackShared.ts:315 "We have to pack params for other
	// configs to parse correctly" — must run before .struct/.loc/.npc/.obj.
	if err := ensureParamPack(); err != nil {
		return err
	}
	if err := ensureLk(); err != nil {
		return err
	}
	if err := packAndSaveParam(srcDir, serverOut, paramPack, lk, constants, clientJag); err != nil {
		return err
	}

	// Eager LoadParamTypes (replaces former lazy ensureParamTypes).
	// .param was just packed above; param.dat/idx now exist on disk
	// for downstream consumers (.struct, .loc, .npc, .obj).
	// TS source: PackShared.ts:334 (ParamType.load).
	paramTypes, err := objtype.LoadParamTypes(outDir)
	if err != nil {
		return fmt.Errorf("load param types: %w", err)
	}

	// .enum — server-only, freshness-gated.
	if GetLatestModified(scriptsDir, ".enum") > 0 &&
		ShouldBuild(scriptsDir, ".enum", filepath.Join(serverOut, "enum.dat")) {
		enumPack, err := NewPackFile(srcDir, "enum", nil)
		if err != nil {
			return err
		}
		if err := packAndSaveEnum(srcDir, serverOut, enumPack, lk, constants); err != nil {
			return err
		}
	}

	// .inv — server-only, freshness-gated.
	if GetLatestModified(scriptsDir, ".inv") > 0 &&
		ShouldBuild(scriptsDir, ".inv", filepath.Join(serverOut, "inv.dat")) {
		if err := ensureObjPack(); err != nil {
			return err
		}
		if err := ensureInvPack(); err != nil {
			return err
		}
		if err := packAndSaveInv(srcDir, serverOut, invPack, objPack, constants); err != nil {
			return err
		}
	}

	// .mesanim — server-only, freshness-gated.
	if GetLatestModified(scriptsDir, ".mesanim") > 0 &&
		ShouldBuild(scriptsDir, ".mesanim", filepath.Join(serverOut, "mesanim.dat")) {
		if err := ensureSeqPack(); err != nil {
			return err
		}
		mesPack, err := NewPackFile(srcDir, "mesanim", nil)
		if err != nil {
			return err
		}
		if err := packAndSaveMesAnim(srcDir, serverOut, mesPack, seqPack, constants); err != nil {
			return err
		}
	}

	// .struct — server-only, freshness-gated.
	if GetLatestModified(scriptsDir, ".struct") > 0 &&
		ShouldBuild(scriptsDir, ".struct", filepath.Join(serverOut, "struct.dat")) {
		structPack, err := NewPackFile(srcDir, "struct", nil)
		if err != nil {
			return err
		}
		if err := packAndSaveStruct(srcDir, serverOut, structPack, paramTypes, lk, constants); err != nil {
			return err
		}
	}

	// .dbtable + .dbrow — paired server-only joint freshness-gated.
	// TS PackShared.ts:393-414 — joint shouldBuild gate, DbTableType.load
	// between packers. Goscape mirrors via mid-pipeline objtype.LoadDbTableTypes.
	if GetLatestModified(scriptsDir, ".dbrow") > 0 || GetLatestModified(scriptsDir, ".dbtable") > 0 {
		if ShouldBuild(scriptsDir, ".dbrow", filepath.Join(serverOut, "dbrow.dat")) ||
			ShouldBuild(scriptsDir, ".dbtable", filepath.Join(serverOut, "dbtable.dat")) {
			if err := ensureDbTablePack(); err != nil {
				return err
			}
			if err := packAndSaveDbTable(srcDir, serverOut, dbtablePack, lk, constants); err != nil {
				return err
			}

			// Mid-pipeline DbTableType cache load — .dbrow needs to resolve
			// table=NAME → *DbTableType at pack time. Per
			// [[load_param_types_dir_arg]]: LoadDbTableTypes takes outDir
			// (parent of server/), NOT serverOut.
			dbtableTypes, err := objtype.LoadDbTableTypes(outDir)
			if err != nil {
				return fmt.Errorf("load dbtable types between dbtable/dbrow packers: %w", err)
			}

			if err := ensureDbRowPack(); err != nil {
				return err
			}
			if err := packAndSaveDbRow(srcDir, serverOut, dbrowPack, dbtablePack, dbtableTypes, lk, constants); err != nil {
				return err
			}
		}
	}

	// .seq — unconditional (NAI-196-D-UNCONDITIONAL-CLIENT-PACK).
	// TS PackShared.ts:454-475.
	if err := ensureSeqPack(); err != nil {
		return err
	}
	if err := ensureAnimPack(); err != nil {
		return err
	}
	if err := ensureObjPack(); err != nil {
		return err
	}
	if err := packAndSaveSeq(srcDir, serverOut, seqPack, animPack, objPack, constants, clientJag); err != nil {
		return err
	}

	// .loc — unconditional (NAI-196-D-UNCONDITIONAL-CLIENT-PACK).
	if err := ensureLocPack(); err != nil {
		return err
	}
	if err := ensureModelPack(); err != nil {
		return err
	}
	if err := ensureCategoryPack(); err != nil {
		return err
	}
	if err := ensureSeqPack(); err != nil {
		return err
	}
	if err := ensureTexturePack(); err != nil {
		return err
	}
	if err := packAndSaveLoc(srcDir, serverOut, locPack, modelPack, categoryPack, seqPack, texturePack, lk, paramTypes, constants, clientJag); err != nil {
		return err
	}

	// .flo — unconditional (NAI-196-D-UNCONDITIONAL-CLIENT-PACK).
	// TS PackShared.ts:500-521.
	if err := ensureFloPack(); err != nil {
		return err
	}
	if err := ensureTexturePack(); err != nil {
		return err
	}
	if err := packAndSaveFlo(srcDir, serverOut, floPack, texturePack, constants, clientJag); err != nil {
		return err
	}

	// .spotanim — unconditional (NAI-196-D-UNCONDITIONAL-CLIENT-PACK).
	// TS PackShared.ts:523-544.
	if err := ensureSpotAnimPack(); err != nil {
		return err
	}
	if err := ensureModelPack(); err != nil {
		return err
	}
	if err := ensureSeqPack(); err != nil {
		return err
	}
	if err := packAndSaveSpotAnim(srcDir, serverOut, spotanimPack, modelPack, seqPack, constants, clientJag); err != nil {
		return err
	}

	// .npc — unconditional (NAI-196-D-UNCONDITIONAL-CLIENT-PACK).
	if err := ensureNpcPack(); err != nil {
		return err
	}
	if err := ensureModelPack(); err != nil {
		return err
	}
	if err := ensureCategoryPack(); err != nil {
		return err
	}
	if err := ensureSeqPack(); err != nil {
		return err
	}
	if err := ensureHuntPack(); err != nil {
		return err
	}
	if err := packAndSaveNpc(srcDir, serverOut, npcPack, modelPack, categoryPack, seqPack, huntPack, lk, paramTypes, constants, clientJag); err != nil {
		return err
	}

	// .obj — unconditional (NAI-196-D-UNCONDITIONAL-CLIENT-PACK).
	if err := ensureObjPack(); err != nil {
		return err
	}
	if err := ensureModelPack(); err != nil {
		return err
	}
	if err := ensureCategoryPack(); err != nil {
		return err
	}
	if err := ensureSeqPack(); err != nil {
		return err
	}
	if err := packAndSaveObj(srcDir, serverOut, objPack, modelPack, categoryPack, seqPack, lk, paramTypes, constants, clientJag); err != nil {
		return err
	}

	// .idk — unconditional (NAI-196-D-UNCONDITIONAL-CLIENT-PACK).
	// TS PackShared.ts:592-613.
	if err := ensureIdkPack(); err != nil {
		return err
	}
	if err := ensureModelPack(); err != nil {
		return err
	}
	if err := packAndSaveIdk(srcDir, serverOut, idkPack, modelPack, constants, clientJag); err != nil {
		return err
	}

	// .varp — unconditional (NAI-196-D-UNCONDITIONAL-CLIENT-PACK).
	if err := packAndSaveVarp(srcDir, serverOut, varpPack, constants, clientJag); err != nil {
		return err
	}

	// .hunt — server-only, freshness-gated.
	// TS PackShared.ts:638-645. Eight reference registries — largest
	// fan-out of any single config.
	if GetLatestModified(scriptsDir, ".hunt") > 0 &&
		ShouldBuild(scriptsDir, ".hunt", filepath.Join(serverOut, "hunt.dat")) {
		if err := ensureCategoryPack(); err != nil {
			return err
		}
		if err := ensureHuntPack(); err != nil {
			return err
		}
		if err := ensureInvPack(); err != nil {
			return err
		}
		if err := ensureLocPack(); err != nil {
			return err
		}
		if err := ensureNpcPack(); err != nil {
			return err
		}
		if err := ensureObjPack(); err != nil {
			return err
		}
		if err := ensureParamPack(); err != nil {
			return err
		}
		if err := packAndSaveHunt(srcDir, serverOut, huntPack, categoryPack, invPack, locPack, npcPack, objPack, paramPack, varnPack, varpPack, constants); err != nil {
			return err
		}
	}

	// .varn — server-only, freshness-gated.
	if GetLatestModified(scriptsDir, ".varn") > 0 &&
		ShouldBuild(scriptsDir, ".varn", filepath.Join(serverOut, "varn.dat")) {
		if err := packAndSaveVarn(srcDir, serverOut, varnPack, constants); err != nil {
			return err
		}
	}

	// .vars — server-only, freshness-gated.
	if GetLatestModified(scriptsDir, ".vars") > 0 &&
		ShouldBuild(scriptsDir, ".vars", filepath.Join(serverOut, "vars.dat")) {
		if err := packAndSaveVars(srcDir, serverOut, varsPack, constants); err != nil {
			return err
		}
	}

	return clientJag.Save(filepath.Join(clientOut, "config"), false)
}

// checkVarNameUniqueness rejects when any debugname appears in more
// than one of the supplied PackFiles. Sparse slots (empty name) are
// ignored. Error message names the duplicated identifier and the
// pack-type ("varp", "varn", "vars") of the first declaration.
//
// First introduced in NAI-193 once varp landed as the third and final
// of the var-name trio, enabling the full cross-domain name check.
//
// TS source: tools/pack/config/PackShared.ts:292-310.
func checkVarNameUniqueness(pfs ...*PackFile) error {
	seen := map[string]string{} // name → pack type that first declared it
	for _, pf := range pfs {
		for id := range pf.Max {
			name := pf.GetByID(id)
			if name == "" {
				continue
			}
			if prior, dup := seen[name]; dup {
				return fmt.Errorf("non-unique var name %q (declared in %s and again in %s)", name, prior, pf.Type)
			}
			seen[name] = pf.Type
		}
	}
	return nil
}

func packAndSaveVarp(srcDir, serverOut string, pf *PackFile, c Constants, clientJag *jagfile.Jagfile) error {
	cfgs, err := ReadTypedConfigs(srcDir, ".varp", nil, parseVarpConfig, c)
	if err != nil {
		return err
	}
	server, client := packVarpConfigs(cfgs, pf)
	if err := server.Save(
		filepath.Join(serverOut, "varp.dat"),
		filepath.Join(serverOut, "varp.idx"),
	); err != nil {
		return err
	}
	clientJag.Write("varp.dat", client.Dat)
	clientJag.Write("varp.idx", client.Idx)
	return nil
}

func packAndSaveVarn(srcDir, serverOut string, pf *PackFile, c Constants) error {
	cfgs, err := ReadTypedConfigs(srcDir, ".varn", nil, parseVarnConfig, c)
	if err != nil {
		return err
	}
	pd := packVarnConfigs(cfgs, pf)
	return pd.Save(filepath.Join(serverOut, "varn.dat"), filepath.Join(serverOut, "varn.idx"))
}

func packAndSaveVars(srcDir, serverOut string, pf *PackFile, c Constants) error {
	cfgs, err := ReadTypedConfigs(srcDir, ".vars", nil, parseVarsConfig, c)
	if err != nil {
		return err
	}
	pd := packVarsConfigs(cfgs, pf)
	return pd.Save(filepath.Join(serverOut, "vars.dat"), filepath.Join(serverOut, "vars.idx"))
}

// loadParamLookups constructs the 12 typed-id PackFiles needed by
// lookupParamValue (the 13th, varpPF, is reused from the up-front
// var-domain trio). Called only when .param source is present so the
// cost is amortized for the no-source case.
//
// NAI-194-D-PACKFILE-SINGLETONS-DEFERRED: TS uses module-level
// EnumPack/ObjPack/etc.; goscape constructs from srcDir per call.
func loadParamLookups(srcDir string, varpPF *PackFile) (*paramLookups, error) {
	lk := &paramLookups{varpPF: varpPF}
	for _, t := range []struct {
		name string
		dst  **PackFile
	}{
		{"enum", &lk.enumPF},
		{"obj", &lk.objPF},
		{"loc", &lk.locPF},
		{"interface", &lk.interfacePF},
		{"struct", &lk.structPF},
		{"category", &lk.categoryPF},
		{"spotanim", &lk.spotanimPF},
		{"npc", &lk.npcPF},
		{"inv", &lk.invPF},
		{"synth", &lk.synthPF},
		{"seq", &lk.seqPF},
		{"dbrow", &lk.dbrowPF},
	} {
		pf, err := NewPackFile(srcDir, t.name, nil)
		if err != nil {
			return nil, fmt.Errorf("load %s pack: %w", t.name, err)
		}
		*t.dst = pf
	}
	return lk, nil
}

// packAndSaveParam reads .param sources, packs them, writes server
// .dat/.idx, and queues the empty-client param entries into clientJag.
//
// TS source: tools/pack/PackShared.ts (param branch of packConfigs).
func packAndSaveParam(srcDir, serverOut string, pf *PackFile, lk *paramLookups, c Constants, clientJag *jagfile.Jagfile) error {
	cfgs, err := ReadTypedConfigs(srcDir, ".param", nil, parseParamConfig, c)
	if err != nil {
		return err
	}
	server, client, err := packParamConfigs(cfgs, pf, lk)
	if err != nil {
		return err
	}
	if err := server.Save(
		filepath.Join(serverOut, "param.dat"),
		filepath.Join(serverOut, "param.idx"),
	); err != nil {
		return err
	}
	clientJag.Write("param.dat", client.Dat)
	clientJag.Write("param.idx", client.Idx)
	return nil
}

func packAndSaveEnum(srcDir, serverOut string, pf *PackFile, lk *paramLookups, c Constants) error {
	cfgs, err := ReadTypedConfigs(srcDir, ".enum", nil, parseEnumConfig, c)
	if err != nil {
		return err
	}
	pd, err := packEnumConfigs(cfgs, pf, lk)
	if err != nil {
		return err
	}
	return pd.Save(filepath.Join(serverOut, "enum.dat"), filepath.Join(serverOut, "enum.idx"))
}

func packAndSaveInv(srcDir, serverOut string, pf, objPack *PackFile, c Constants) error {
	cfgs, err := ReadTypedConfigs(srcDir, ".inv", nil, parseInvConfigFor(objPack), c)
	if err != nil {
		return err
	}
	pd, err := packInvConfigs(cfgs, pf)
	if err != nil {
		return err
	}
	return pd.Save(filepath.Join(serverOut, "inv.dat"), filepath.Join(serverOut, "inv.idx"))
}

func packAndSaveMesAnim(srcDir, serverOut string, pf, seqPack *PackFile, c Constants) error {
	cfgs, err := ReadTypedConfigs(srcDir, ".mesanim", nil, parseMesAnimConfigFor(seqPack), c)
	if err != nil {
		return err
	}
	pd := packMesAnimConfigs(cfgs, pf)
	return pd.Save(filepath.Join(serverOut, "mesanim.dat"), filepath.Join(serverOut, "mesanim.idx"))
}

func packAndSaveStruct(srcDir, serverOut string, pf *PackFile, paramTypes *objtype.ParamTypeConfigs, lk *paramLookups, c Constants) error {
	cfgs, err := ReadTypedConfigs(srcDir, ".struct", nil, parseStructConfigFor(paramTypes, lk), c)
	if err != nil {
		return err
	}
	pd := packStructConfigs(cfgs, pf)
	return pd.Save(filepath.Join(serverOut, "struct.dat"), filepath.Join(serverOut, "struct.idx"))
}

// packAndSaveLoc reads .loc sources, packs them, writes server
// .dat/.idx, and queues the client .dat/.idx into clientJag.
//
// NAI-196-D-UNCONDITIONAL-CLIENT-PACK: this branch runs every
// PackConfigs invocation regardless of source freshness, matching
// TS PackShared.ts:477 (rebuildClient=true ungates shouldBuild).
//
// TS source: tools/pack/config/LocConfig.ts:172-432.
func packAndSaveLoc(srcDir, serverOut string, locPack, modelPack, categoryPack, seqPack, texturePack *PackFile, lk *paramLookups, paramTypes *objtype.ParamTypeConfigs, c Constants, clientJag *jagfile.Jagfile) error {
	parse := parseLocConfigFor(modelPack, categoryPack, seqPack, texturePack, lk, paramTypes)
	cfgs, err := ReadTypedConfigs(srcDir, ".loc", nil, parse, c)
	if err != nil {
		return err
	}
	server, client, err := packLocConfigs(cfgs, locPack, modelPack)
	if err != nil {
		return err
	}
	if err := server.Save(
		filepath.Join(serverOut, "loc.dat"),
		filepath.Join(serverOut, "loc.idx"),
	); err != nil {
		return err
	}
	clientJag.Write("loc.dat", client.Dat)
	clientJag.Write("loc.idx", client.Idx)
	return nil
}

// packAndSaveNpc reads .npc sources, packs them, writes server
// .dat/.idx, and queues the client .dat/.idx into clientJag.
//
// NAI-196-D-UNCONDITIONAL-CLIENT-PACK: this branch runs every
// PackConfigs invocation regardless of source freshness.
//
// TS source: tools/pack/config/NpcConfig.ts:265-509.
func packAndSaveNpc(srcDir, serverOut string, npcPack, modelPack, categoryPack, seqPack, huntPack *PackFile, lk *paramLookups, paramTypes *objtype.ParamTypeConfigs, c Constants, clientJag *jagfile.Jagfile) error {
	parse := parseNpcConfigFor(modelPack, categoryPack, seqPack, huntPack, lk, paramTypes)
	cfgs, err := ReadTypedConfigs(srcDir, ".npc", nil, parse, c)
	if err != nil {
		return err
	}
	server, client, err := packNpcConfigs(cfgs, npcPack)
	if err != nil {
		return err
	}
	if err := server.Save(
		filepath.Join(serverOut, "npc.dat"),
		filepath.Join(serverOut, "npc.idx"),
	); err != nil {
		return err
	}
	clientJag.Write("npc.dat", client.Dat)
	clientJag.Write("npc.idx", client.Idx)
	return nil
}

// packAndSaveObj reads .obj sources, packs them, writes server
// .dat/.idx, and queues the client .dat/.idx into clientJag.
//
// NAI-196-D-UNCONDITIONAL-CLIENT-PACK: this branch runs every
// PackConfigs invocation regardless of source freshness.
//
// TS source: tools/pack/config/ObjConfig.ts:196-440.
func packAndSaveObj(srcDir, serverOut string, objPack, modelPack, categoryPack, seqPack *PackFile, lk *paramLookups, paramTypes *objtype.ParamTypeConfigs, c Constants, clientJag *jagfile.Jagfile) error {
	parse := parseObjConfigFor(modelPack, categoryPack, seqPack, objPack, lk, paramTypes)
	cfgs, err := ReadTypedConfigs(srcDir, ".obj", nil, parse, c)
	if err != nil {
		return err
	}
	server, client, err := packObjConfigs(cfgs, objPack)
	if err != nil {
		return err
	}
	if err := server.Save(
		filepath.Join(serverOut, "obj.dat"),
		filepath.Join(serverOut, "obj.idx"),
	); err != nil {
		return err
	}
	clientJag.Write("obj.dat", client.Dat)
	clientJag.Write("obj.idx", client.Idx)
	return nil
}

// packAndSaveSeq reads .seq sources, packs them, writes server
// .dat/.idx, and queues the client .dat/.idx into clientJag.
//
// NAI-196-D-UNCONDITIONAL-CLIENT-PACK: this branch runs every
// PackConfigs invocation regardless of source freshness, matching
// TS PackShared.ts:460 (rebuildClient=true ungates shouldBuild).
//
// TS source: tools/pack/config/SeqConfig.ts:121-208.
func packAndSaveSeq(srcDir, serverOut string, seqPack, animPack, objPack *PackFile, c Constants, clientJag *jagfile.Jagfile) error {
	parse := parseSeqConfigFor(animPack, objPack)
	cfgs, err := ReadTypedConfigs(srcDir, ".seq", nil, parse, c)
	if err != nil {
		return err
	}
	server, client := packSeqConfigs(cfgs, seqPack)
	if err := server.Save(
		filepath.Join(serverOut, "seq.dat"),
		filepath.Join(serverOut, "seq.idx"),
	); err != nil {
		return err
	}
	clientJag.Write("seq.dat", client.Dat)
	clientJag.Write("seq.idx", client.Idx)
	return nil
}

// packAndSaveFlo reads .flo sources, packs them, writes server
// .dat/.idx (which contain only per-id Next() boundaries — no opcode
// bytes), and queues the client .dat/.idx into clientJag.
//
// NAI-196-D-UNCONDITIONAL-CLIENT-PACK: unconditional.
//
// TS source: tools/pack/config/FloConfig.ts:63-104.
func packAndSaveFlo(srcDir, serverOut string, floPack, texturePack *PackFile, c Constants, clientJag *jagfile.Jagfile) error {
	parse := parseFloConfigFor(texturePack)
	cfgs, err := ReadTypedConfigs(srcDir, ".flo", nil, parse, c)
	if err != nil {
		return err
	}
	server, client := packFloConfigs(cfgs, floPack)
	if err := server.Save(
		filepath.Join(serverOut, "flo.dat"),
		filepath.Join(serverOut, "flo.idx"),
	); err != nil {
		return err
	}
	clientJag.Write("flo.dat", client.Dat)
	clientJag.Write("flo.idx", client.Idx)
	return nil
}

// packAndSaveSpotAnim reads .spotanim sources, packs them, writes
// server .dat/.idx, and queues the client .dat/.idx into clientJag.
//
// NAI-196-D-UNCONDITIONAL-CLIENT-PACK: unconditional.
//
// TS source: tools/pack/config/SpotAnimConfig.ts:92-152.
func packAndSaveSpotAnim(srcDir, serverOut string, spotanimPack, modelPack, seqPack *PackFile, c Constants, clientJag *jagfile.Jagfile) error {
	parse := parseSpotAnimConfigFor(modelPack, seqPack)
	cfgs, err := ReadTypedConfigs(srcDir, ".spotanim", nil, parse, c)
	if err != nil {
		return err
	}
	server, client := packSpotAnimConfigs(cfgs, spotanimPack)
	if err := server.Save(
		filepath.Join(serverOut, "spotanim.dat"),
		filepath.Join(serverOut, "spotanim.idx"),
	); err != nil {
		return err
	}
	clientJag.Write("spotanim.dat", client.Dat)
	clientJag.Write("spotanim.idx", client.Idx)
	return nil
}

// packAndSaveDbTable reads .dbtable sources, packs them, and writes
// server .dat/.idx. Server-only — does NOT contribute to clientJag.
//
// TS source: tools/pack/config/DbTableConfig.ts:78-224.
func packAndSaveDbTable(srcDir, serverOut string, dbtablePack *PackFile, lk *paramLookups, c Constants) error {
	cfgs, err := ReadTypedConfigs(srcDir, ".dbtable", nil, parseDbTableConfig, c)
	if err != nil {
		return err
	}
	pd, err := packDbTableConfigs(cfgs, dbtablePack, lk)
	if err != nil {
		return err
	}
	return pd.Save(filepath.Join(serverOut, "dbtable.dat"), filepath.Join(serverOut, "dbtable.idx"))
}

// packAndSaveDbRow reads .dbrow sources, packs them, and writes server
// .dat/.idx. Server-only. Consumes the *DbTableTypeConfigs loaded from
// the just-written dbtable.dat for schema lookup.
//
// TS source: tools/pack/config/DbRowConfig.ts:84-185.
func packAndSaveDbRow(srcDir, serverOut string, dbrowPack, dbtablePack *PackFile, dbtableTypes *objtype.DbTableTypeConfigs, lk *paramLookups, c Constants) error {
	parse := parseDbRowConfigFor(dbtablePack)
	cfgs, err := ReadTypedConfigs(srcDir, ".dbrow", nil, parse, c)
	if err != nil {
		return err
	}
	pd, err := packDbRowConfigs(cfgs, dbrowPack, dbtableTypes, lk)
	if err != nil {
		return err
	}
	return pd.Save(filepath.Join(serverOut, "dbrow.dat"), filepath.Join(serverOut, "dbrow.idx"))
}

// packAndSaveHunt reads .hunt sources, packs them, and writes server
// .dat/.idx. Server-only — does NOT contribute to clientJag. Takes
// nine *PackFile parameters (eight reference registries + the Hunt
// pack itself); largest registry-dependency surface of any NAI-198
// config.
//
// TS source: tools/pack/config/HuntConfig.ts:383-545.
func packAndSaveHunt(srcDir, serverOut string, huntPack, categoryPack, invPack, locPack, npcPack, objPack, paramPack, varnPack, varpPack *PackFile, c Constants) error {
	parse := parseHuntConfigFor(categoryPack, invPack, locPack, npcPack, objPack, paramPack, varnPack, varpPack)
	cfgs, err := ReadTypedConfigs(srcDir, ".hunt", nil, parse, c)
	if err != nil {
		return err
	}
	pd, err := packHuntConfigs(cfgs, huntPack)
	if err != nil {
		return err
	}
	return pd.Save(filepath.Join(serverOut, "hunt.dat"), filepath.Join(serverOut, "hunt.idx"))
}

// packAndSaveIdk reads .idk sources, packs them, writes server
// .dat/.idx, and queues the client .dat/.idx into clientJag.
//
// NAI-196-D-UNCONDITIONAL-CLIENT-PACK: unconditional.
//
// TS source: tools/pack/config/IdkConfig.ts:126-205.
func packAndSaveIdk(srcDir, serverOut string, idkPack, modelPack *PackFile, c Constants, clientJag *jagfile.Jagfile) error {
	parse := parseIdkConfigFor(modelPack)
	cfgs, err := ReadTypedConfigs(srcDir, ".idk", nil, parse, c)
	if err != nil {
		return err
	}
	server, client := packIdkConfigs(cfgs, idkPack)
	if err := server.Save(
		filepath.Join(serverOut, "idk.dat"),
		filepath.Join(serverOut, "idk.idx"),
	); err != nil {
		return err
	}
	clientJag.Write("idk.dat", client.Dat)
	clientJag.Write("idk.idx", client.Idx)
	return nil
}
