package pack

import (
	"fmt"
	"path/filepath"

	"github.com/zsrv/goscape/pkg/io/jagfile"
	"github.com/zsrv/goscape/pkg/objtype"
)

// PackConfigsForRegistry runs the per-config packing pipeline,
// returning a *Registry whose fields (Obj, Seq, Loc, ...) are the
// *PackFile singletons constructed during the run. NAI-191-NAI-198
// wired the 18 TS configs ported to date; this entry point was added
// at NAI-213 T1 to expose those singletons to subsequent client-pack
// stages (clientinterface, sprites, audio, graphics).
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
// NAI-196-D-UNCONDITIONAL-CLIENT-PACK: .param, .seq, .loc, .flo,
// .spotanim, .npc, .obj, .idk, .varp run on EVERY PackConfigs
// invocation regardless of source freshness, matching TS
// PackShared.ts:337 (`const rebuildClient = true`) which ungates
// shouldBuild on the eight configs that write to client jag, plus
// .param whose server-side output (param.dat/.idx) other configs
// (.struct/.loc/.npc/.obj) depend on for parsing — TS gates .param
// separately at PackShared.ts:315-331 ("We have to pack params for
// other configs to parse correctly"). .param contributes nothing to
// the client jagfile (TS callback is `() => {}`). NAI-197 extends
// the scope to the four additional client+server configs ported in
// that slice (.seq, .flo, .spotanim, .idk). The server-only nine retain
// their ShouldBuild + GetLatestModified freshness gates (enumerated
// in the NAI-192-D-NO-SRC-NO-OP paragraph below). NAI-199 adds two
// more server-only outputs (category.dat, frame_del.dat) that sit
// outside the NAI-192 scope — both use distinct gate shapes
// (ShouldBuildFile and GetLatestModified+ShouldBuild, respectively)
// and produce .dat without .idx.
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
func PackConfigsForRegistry(srcDir, outDir string) (*Registry, error) {
	reg := &Registry{SrcDir: srcDir}
	if err := packConfigsCore(srcDir, outDir, reg); err != nil {
		return nil, err
	}
	return reg, nil
}

// PackConfigs is the original entry point (2-arg). Kept for backward
// compatibility with non-PackAll callers.
func PackConfigs(srcDir, outDir string) error {
	_, err := PackConfigsForRegistry(srcDir, outDir)
	return err
}

func packConfigsCore(srcDir, outDir string, reg *Registry) error {
	constants, err := LoadConstants(srcDir)
	if err != nil {
		return err
	}

	// Construct all three var-domain PackFiles up-front for the
	// cross-domain uniqueness check across all three name maps.
	if _, err := reg.EnsureVarp(); err != nil {
		return err
	}
	if _, err := reg.EnsureVarn(); err != nil {
		return err
	}
	if _, err := reg.EnsureVars(); err != nil {
		return err
	}

	if err := checkVarNameUniqueness(reg.Varp, reg.Varn, reg.Vars); err != nil {
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

	// `lk` is NOT promoted to Registry — it carries *paramLookups, not
	// a *PackFile, and is consumed only within packConfigsCore.
	var lk *paramLookups
	ensureLk := func() error {
		if lk != nil {
			return nil
		}
		newLk, err := loadParamLookups(reg)
		if err != nil {
			return err
		}
		lk = newLk
		return nil
	}

	// Build every non-transmitted config index before anything packs — a
	// fresh Content clone ships none of them, and the families reference each
	// other across a fixed order. See generateConfigPackIndexes.
	if err := generateConfigPackIndexes(srcDir, reg); err != nil {
		return err
	}

	// .param — unconditional (NAI-196-D-UNCONDITIONAL-CLIENT-PACK).
	// Matches TS PackShared.ts:315 "We have to pack params for other
	// configs to parse correctly" — must run before .struct/.loc/.npc/.obj.
	if _, err := reg.EnsureParam(); err != nil {
		return err
	}
	if err := ensureLk(); err != nil {
		return err
	}
	if err := packAndSaveParam(srcDir, serverOut, reg.Param, lk, constants); err != nil {
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

	// category index — rebuilt from the `category=` references in .loc/.npc/
	// .obj, because categories have no source files of their own and Content
	// gitignores pack/category.pack. Ids are assigned from 0 in crawl order:
	// TS rebuilds the whole index rather than appending, so a removed category
	// renumbers the rest. Must run before .loc/.npc/.obj pack, which resolve
	// `category=` through this index.
	//
	// TS source: tools/pack/PackFile.ts:validateCategoryPack. TS's third gate
	// arm (didFileSetChange on a revalidate stamp) is dev-mode hot-reload,
	// which goscape does not model — same reasoning as the revalidatePack row
	// in docs/PORTING.md.
	{
		categoryPackPath := filepath.Join(srcDir, "pack", "category.pack")
		scriptsDir := filepath.Join(srcDir, "scripts")
		if ShouldBuild(scriptsDir, ".loc", categoryPackPath) ||
			ShouldBuild(scriptsDir, ".npc", categoryPackPath) ||
			ShouldBuild(scriptsDir, ".obj", categoryPackPath) {
			categories, err := CrawlConfigCategories(srcDir)
			if err != nil {
				return err
			}
			categoryPF, err := reg.EnsureCategory()
			if err != nil {
				return err
			}
			// No Clear(): TS registers over the loaded index from id 0 and
			// leaves anything beyond the crawl length in place, so an empty
			// crawl must not wipe a hand-authored index.
			for i, name := range categories {
				categoryPF.Register(i, name)
			}
			categoryPF.RefreshNames()
			if err := categoryPF.Save(); err != nil {
				return err
			}
			ClearFsCache()
		}
	}

	// category — server-only special. TS PackShared.ts:341-352.
	// Reads <srcDir>/pack/category.pack (already loaded by
	// EnsureCategory). NAI-199-D-TS-CODE-STALENESS-GATE drops TS's
	// second arm `shouldBuild('tools/pack/config', '.ts', dest)`.
	if ShouldBuildFile(
		filepath.Join(srcDir, "pack", "category.pack"),
		filepath.Join(serverOut, "category.dat"),
	) {
		if _, err := reg.EnsureCategory(); err != nil {
			return err
		}
		if err := packAndSaveCategoryDat(serverOut, reg.Category); err != nil {
			return err
		}
	}

	// frame_del — server-only special. TS PackShared.ts:355-388.
	// Reads AnimPack + <srcDir>/models/**/*.frame trailers.
	// Server-only; no idx. Empty models dir → branch skipped
	// (GetLatestModified guard); empty AnimPack registry inside the
	// branch → 0-byte frame_del.dat (per packAndSaveFrameDel docs).
	// NAI-199-D-TS-CODE-STALENESS-GATE drops TS's second arm
	// `shouldBuild('tools/pack/config', '.ts', dest)`.
	if GetLatestModified(filepath.Join(srcDir, "models"), ".frame") > 0 &&
		ShouldBuild(
			filepath.Join(srcDir, "models"),
			".frame",
			filepath.Join(serverOut, "frame_del.dat"),
		) {
		if _, err := reg.EnsureAnim(); err != nil {
			return err
		}
		if err := packAndSaveFrameDel(srcDir, serverOut, reg.Anim); err != nil {
			return err
		}
	}

	// .enum — server-only, freshness-gated.
	if GetLatestModified(scriptsDir, ".enum") > 0 &&
		ShouldBuild(scriptsDir, ".enum", filepath.Join(serverOut, "enum.dat")) {
		enumPack, err := reg.EnsureEnum()
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
		if _, err := reg.EnsureObj(); err != nil {
			return err
		}
		if _, err := reg.EnsureInv(); err != nil {
			return err
		}
		if err := packAndSaveInv(srcDir, serverOut, reg.Inv, reg.Obj, constants); err != nil {
			return err
		}
	}

	// .mesanim — server-only, freshness-gated.
	if GetLatestModified(scriptsDir, ".mesanim") > 0 &&
		ShouldBuild(scriptsDir, ".mesanim", filepath.Join(serverOut, "mesanim.dat")) {
		if _, err := reg.EnsureSeq(); err != nil {
			return err
		}
		if _, err := reg.EnsureMesAnim(); err != nil {
			return err
		}
		if err := packAndSaveMesAnim(srcDir, serverOut, reg.MesAnim, reg.Seq, constants); err != nil {
			return err
		}
	}

	// .struct — server-only, freshness-gated.
	if GetLatestModified(scriptsDir, ".struct") > 0 &&
		ShouldBuild(scriptsDir, ".struct", filepath.Join(serverOut, "struct.dat")) {
		if _, err := reg.EnsureStruct(); err != nil {
			return err
		}
		if err := packAndSaveStruct(srcDir, serverOut, reg.Struct, paramTypes, lk, constants); err != nil {
			return err
		}
	}

	// .dbtable + .dbrow — paired server-only joint freshness-gated.
	// TS PackShared.ts:393-414 — joint shouldBuild gate, DbTableType.load
	// between packers. Goscape mirrors via mid-pipeline objtype.LoadDbTableTypes.
	if GetLatestModified(scriptsDir, ".dbrow") > 0 || GetLatestModified(scriptsDir, ".dbtable") > 0 {
		if ShouldBuild(scriptsDir, ".dbrow", filepath.Join(serverOut, "dbrow.dat")) ||
			ShouldBuild(scriptsDir, ".dbtable", filepath.Join(serverOut, "dbtable.dat")) {
			if _, err := reg.EnsureDbTable(); err != nil {
				return err
			}
			if err := packAndSaveDbTable(srcDir, serverOut, reg.DbTable, lk, constants); err != nil {
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

			if _, err := reg.EnsureDbRow(); err != nil {
				return err
			}
			if err := packAndSaveDbRow(srcDir, serverOut, reg.DbRow, reg.DbTable, dbtableTypes, lk, constants); err != nil {
				return err
			}
		}
	}

	// .seq — unconditional (NAI-196-D-UNCONDITIONAL-CLIENT-PACK).
	// TS PackShared.ts:454-475.
	if _, err := reg.EnsureSeq(); err != nil {
		return err
	}
	if _, err := reg.EnsureAnim(); err != nil {
		return err
	}
	if _, err := reg.EnsureObj(); err != nil {
		return err
	}
	if err := packAndSaveSeq(srcDir, serverOut, reg.Seq, reg.Anim, reg.Obj, constants, clientJag); err != nil {
		return err
	}

	// .loc — unconditional (NAI-196-D-UNCONDITIONAL-CLIENT-PACK).
	if _, err := reg.EnsureLoc(); err != nil {
		return err
	}
	if _, err := reg.EnsureModel(); err != nil {
		return err
	}
	if _, err := reg.EnsureCategory(); err != nil {
		return err
	}
	if _, err := reg.EnsureSeq(); err != nil {
		return err
	}
	if _, err := reg.EnsureTexture(); err != nil {
		return err
	}
	if err := packAndSaveLoc(srcDir, serverOut, reg.Loc, reg.Model, reg.Category, reg.Seq, reg.Texture, lk, paramTypes, constants, clientJag); err != nil {
		return err
	}

	// .flo — unconditional (NAI-196-D-UNCONDITIONAL-CLIENT-PACK).
	// TS PackShared.ts:500-521.
	if _, err := reg.EnsureFlo(); err != nil {
		return err
	}
	if _, err := reg.EnsureTexture(); err != nil {
		return err
	}
	if err := packAndSaveFlo(srcDir, serverOut, reg.Flo, reg.Texture, constants, clientJag); err != nil {
		return err
	}

	// .spotanim — unconditional (NAI-196-D-UNCONDITIONAL-CLIENT-PACK).
	// TS PackShared.ts:523-544.
	if _, err := reg.EnsureSpotAnim(); err != nil {
		return err
	}
	if _, err := reg.EnsureModel(); err != nil {
		return err
	}
	if _, err := reg.EnsureSeq(); err != nil {
		return err
	}
	if err := packAndSaveSpotAnim(srcDir, serverOut, reg.SpotAnim, reg.Model, reg.Seq, constants, clientJag); err != nil {
		return err
	}

	// .npc — unconditional (NAI-196-D-UNCONDITIONAL-CLIENT-PACK).
	if _, err := reg.EnsureNpc(); err != nil {
		return err
	}
	if _, err := reg.EnsureModel(); err != nil {
		return err
	}
	if _, err := reg.EnsureCategory(); err != nil {
		return err
	}
	if _, err := reg.EnsureSeq(); err != nil {
		return err
	}
	if _, err := reg.EnsureHunt(); err != nil {
		return err
	}
	if err := packAndSaveNpc(srcDir, serverOut, reg.Npc, reg.Model, reg.Category, reg.Seq, reg.Hunt, lk, paramTypes, constants, clientJag); err != nil {
		return err
	}

	// .obj — unconditional (NAI-196-D-UNCONDITIONAL-CLIENT-PACK).
	if _, err := reg.EnsureObj(); err != nil {
		return err
	}
	if _, err := reg.EnsureModel(); err != nil {
		return err
	}
	if _, err := reg.EnsureCategory(); err != nil {
		return err
	}
	if _, err := reg.EnsureSeq(); err != nil {
		return err
	}
	if err := packAndSaveObj(srcDir, serverOut, reg.Obj, reg.Model, reg.Category, reg.Seq, lk, paramTypes, constants, clientJag); err != nil {
		return err
	}

	// .idk — unconditional (NAI-196-D-UNCONDITIONAL-CLIENT-PACK).
	// TS PackShared.ts:592-613.
	if _, err := reg.EnsureIdk(); err != nil {
		return err
	}
	if _, err := reg.EnsureModel(); err != nil {
		return err
	}
	if err := packAndSaveIdk(srcDir, serverOut, reg.Idk, reg.Model, constants, clientJag); err != nil {
		return err
	}

	// .varp — unconditional (NAI-196-D-UNCONDITIONAL-CLIENT-PACK).
	if err := packAndSaveVarp(srcDir, serverOut, reg.Varp, constants, clientJag); err != nil {
		return err
	}

	// .hunt — server-only, freshness-gated.
	// TS PackShared.ts:638-645. Eight reference registries — largest
	// fan-out of any single config.
	if GetLatestModified(scriptsDir, ".hunt") > 0 &&
		ShouldBuild(scriptsDir, ".hunt", filepath.Join(serverOut, "hunt.dat")) {
		if _, err := reg.EnsureCategory(); err != nil {
			return err
		}
		if _, err := reg.EnsureHunt(); err != nil {
			return err
		}
		if _, err := reg.EnsureInv(); err != nil {
			return err
		}
		if _, err := reg.EnsureLoc(); err != nil {
			return err
		}
		if _, err := reg.EnsureNpc(); err != nil {
			return err
		}
		if _, err := reg.EnsureObj(); err != nil {
			return err
		}
		if _, err := reg.EnsureParam(); err != nil {
			return err
		}
		if err := packAndSaveHunt(srcDir, serverOut, reg.Hunt, reg.Category, reg.Inv, reg.Loc, reg.Npc, reg.Obj, reg.Param, reg.Varn, reg.Varp, constants); err != nil {
			return err
		}
	}

	// .varn — server-only, freshness-gated.
	if GetLatestModified(scriptsDir, ".varn") > 0 &&
		ShouldBuild(scriptsDir, ".varn", filepath.Join(serverOut, "varn.dat")) {
		if err := packAndSaveVarn(srcDir, serverOut, reg.Varn, constants); err != nil {
			return err
		}
	}

	// .vars — server-only, freshness-gated.
	if GetLatestModified(scriptsDir, ".vars") > 0 &&
		ShouldBuild(scriptsDir, ".vars", filepath.Join(serverOut, "vars.dat")) {
		if err := packAndSaveVars(srcDir, serverOut, reg.Vars, constants); err != nil {
			return err
		}
	}

	return clientJag.Save(filepath.Join(clientOut, "config"))
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

// saveTransmittedConfig is the shared tail of the eight transmitted
// packAndSave* funcs (flo, idk, loc, npc, obj, seq, spotanim, varp —
// the configs that contribute to the client jagfile): save the server
// pack, then queue the client pack into clientJag. name is the
// lowercase file basename (e.g. "varp") used for both
// serverOut/<name>.{dat,idx} and the clientJag entry names.
//
// NAI-191-D-VALIDATE-FLAGS-DEFERRED: rev-225 has not yet wired
// BuildVerify CRC checks into these configs (see
// nai193_deviation_pins_test.go et al.), so this helper does not call
// BuildVerify -- only server.Save + the two clientJag.Write calls are
// duplicated across the eight callers.
func saveTransmittedConfig(serverOut, name string, server, client *PackedData, clientJag *jagfile.Jagfile) error {
	if err := server.Save(
		filepath.Join(serverOut, name+".dat"),
		filepath.Join(serverOut, name+".idx"),
	); err != nil {
		return err
	}
	clientJag.Write(name+".dat", client.Dat)
	clientJag.Write(name+".idx", client.Idx)
	return nil
}

func packAndSaveVarp(srcDir, serverOut string, pf *PackFile, c Constants, clientJag *jagfile.Jagfile) error {
	cfgs, err := ReadTypedConfigs(srcDir, ".varp", nil, parseVarpConfig, c)
	if err != nil {
		return err
	}
	server, client := packVarpConfigs(cfgs, pf)
	return saveTransmittedConfig(serverOut, "varp", server, client, clientJag)
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
// NAI-194-D-PACKFILE-SINGLETONS: CLOSED. TS uses module-level
// EnumPack/ObjPack/etc., so a name registered anywhere is visible to every
// consumer. goscape used to construct these from srcDir per call, which
// snapshots each .pack file at construction time — harmless while the indexes
// always existed before packing, but wrong once the packer generates them
// mid-run: a lookup built earlier stays empty and cross-family references fail.
// The lookups now share the Registry's instances, which varpPF already did.
func loadParamLookups(reg *Registry) (*paramLookups, error) {
	varpPF, err := reg.EnsureVarp()
	if err != nil {
		return nil, fmt.Errorf("load varp pack: %w", err)
	}
	lk := &paramLookups{varpPF: varpPF}
	for _, t := range []struct {
		name   string
		ensure func() (*PackFile, error)
		dst    **PackFile
	}{
		{"enum", reg.EnsureEnum, &lk.enumPF},
		{"obj", reg.EnsureObj, &lk.objPF},
		{"loc", reg.EnsureLoc, &lk.locPF},
		{"interface", reg.EnsureInterface, &lk.interfacePF},
		{"struct", reg.EnsureStruct, &lk.structPF},
		{"category", reg.EnsureCategory, &lk.categoryPF},
		{"spotanim", reg.EnsureSpotAnim, &lk.spotanimPF},
		{"npc", reg.EnsureNpc, &lk.npcPF},
		{"inv", reg.EnsureInv, &lk.invPF},
		{"synth", reg.EnsureSynth, &lk.synthPF},
		{"seq", reg.EnsureSeq, &lk.seqPF},
		{"dbrow", reg.EnsureDbRow, &lk.dbrowPF},
	} {
		pf, err := t.ensure()
		if err != nil {
			return nil, fmt.Errorf("load %s pack: %w", t.name, err)
		}
		*t.dst = pf
	}
	return lk, nil
}

// packAndSaveParam reads .param sources, packs them, and writes the
// server .dat/.idx. TS callback is server-only — the param branch is
// not added to the client jagfile (PackShared.ts:323 passes `() => {}`
// as the read-side callback).
//
// TS source: tools/pack/PackShared.ts (param branch of packConfigs).
func packAndSaveParam(srcDir, serverOut string, pf *PackFile, lk *paramLookups, c Constants) error {
	cfgs, err := ReadTypedConfigs(srcDir, ".param", nil, parseParamConfig, c)
	if err != nil {
		return err
	}
	server, _, err := packParamConfigs(cfgs, pf, lk)
	if err != nil {
		return err
	}
	return server.Save(
		filepath.Join(serverOut, "param.dat"),
		filepath.Join(serverOut, "param.idx"),
	)
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
	parse := parseLocConfigFor(categoryPack, seqPack, texturePack, lk, paramTypes)
	cfgs, err := ReadTypedConfigs(srcDir, ".loc", nil, parse, c)
	if err != nil {
		return err
	}
	server, client, err := packLocConfigs(cfgs, locPack, modelPack)
	if err != nil {
		return err
	}
	return saveTransmittedConfig(serverOut, "loc", server, client, clientJag)
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
	return saveTransmittedConfig(serverOut, "npc", server, client, clientJag)
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
	return saveTransmittedConfig(serverOut, "obj", server, client, clientJag)
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
	return saveTransmittedConfig(serverOut, "seq", server, client, clientJag)
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
	return saveTransmittedConfig(serverOut, "flo", server, client, clientJag)
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
	return saveTransmittedConfig(serverOut, "spotanim", server, client, clientJag)
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
	return saveTransmittedConfig(serverOut, "idk", server, client, clientJag)
}

// generateConfigPackIndexes rebuilds the ten NON-TRANSMITTED config indexes
// before any family is packed.
//
// Content gitignores these ("generated for the server only"), so a fresh clone
// has none of them. They cannot be filled in lazily as each family packs,
// because families reference each other across the fixed pack order: npc
// resolves `huntmode=` through the hunt index but packs before hunt, and struct
// resolves `param=` through the param index. TS registers each pack on first
// access, which makes its order irrelevant; goscape's order is fixed, so the
// indexes are built up front instead.
//
// Transmitted families are excluded: their ids are baked into the client cache
// and Content tracks those .pack files, so they are never auto-assigned.
//
// PIG-D3 (this pin only): rev-254 and later thread a transmitted flag through
// readAndValidate, which lets a per-family seam self-heal and lets transmitted
// packs raise TS's missing-id error. rev-225 has neither — it has no
// readAndValidate helper at all, and config-name validation is not wired into
// the live path (NAI-191-D-VALIDATE-FLAGS-DEFERRED) — so generation lives
// entirely in this function.
//
// TS source: tools/pack/PackFile.ts:validateConfigPack (register + save).
func generateConfigPackIndexes(srcDir string, reg *Registry) error {
	for _, f := range []struct {
		ext      string
		ensure   func() (*PackFile, error)
		brackets bool
	}{
		{".param", reg.EnsureParam, false},
		{".enum", reg.EnsureEnum, false},
		{".struct", reg.EnsureStruct, false},
		{".inv", reg.EnsureInv, false},
		{".mesanim", reg.EnsureMesAnim, false},
		{".dbtable", reg.EnsureDbTable, false},
		{".dbrow", reg.EnsureDbRow, false},
		{".hunt", reg.EnsureHunt, false},
		{".varn", reg.EnsureVarn, false},
		{".vars", reg.EnsureVars, false},
		// Scripts keep their brackets: the compiler loads this index as its
		// "runescript" symbol table, and without it nothing compiles.
		// TS source: tools/pack/PackFile.ts:regenScriptPack.
		{".rs2", reg.EnsureScript, true},
	} {
		pf, err := f.ensure()
		if err != nil {
			return err
		}
		if err := registerMissingNames(srcDir, pf, f.ext, f.brackets); err != nil {
			return err
		}
	}
	ClearFsCache()
	return nil
}

// registerMissingNames adds any config name absent from pf's index, assigning
// ids in crawl order, and persists the index when anything was added.
//
// Ordering matters: ids are written into the packed .dat files, so ranging a
// map here would make two packs of identical content differ.
func registerMissingNames(srcDir string, pf *PackFile, ext string, includeBrackets bool) error {
	names, err := CrawlConfigNames(srcDir, ext, includeBrackets)
	if err != nil {
		return err
	}
	added := false
	for _, name := range names {
		if _, ok := pf.Names[name]; ok {
			continue
		}
		pf.Register(pf.Max, name)
		pf.Max++
		added = true
	}
	if !added {
		return nil
	}
	pf.RefreshNames()
	return pf.Save()
}
