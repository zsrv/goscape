package pack

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/zsrv/goscape/pkg/io/filestream"
	"github.com/zsrv/goscape/pkg/io/jagfile"
	"github.com/zsrv/goscape/pkg/objtype"
)

// clientConfigCRC* are the rev-244 BUILD_VERIFY CRC magic numbers for
// the eight client-jagfile config sub-files. Updated from the 225 values
// at TS PackShared.ts:435,459,467,507,531,555,573,603 @ 9aadcec4.
//
// 225 values (for reference):
//
//	seqCRC      was 1638136604  → now 1405403166  (PackShared.ts:435)
//	locCRC      was 891497087   → now 1195428820  (PackShared.ts:459)
//	floCRC      new constant    → now 1976597026  (PackShared.ts:467)
//	spotanimCRC was -1279835623 → now 117013845   (PackShared.ts:507)
//	npcCRC      was -2140681882 → now -997428438  (PackShared.ts:531)
//	objCRC      was -840233510  → now 1589810970  (PackShared.ts:555)
//	idkCRC      new constant    → now -359342366  (PackShared.ts:573)
//	varpCRC     was 705633567   → now -1961744050 (PackShared.ts:603)
const (
	clientConfigCRCSeq      int32 = 1405403166
	clientConfigCRCLoc      int32 = 1195428820
	clientConfigCRCFlo      int32 = 1976597026
	clientConfigCRCSpotAnim int32 = 117013845
	clientConfigCRCNpc      int32 = -997428438
	clientConfigCRCObj      int32 = 1589810970
	clientConfigCRCIdk      int32 = -359342366
	clientConfigCRCVarp     int32 = -1961744050
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
// NAI-191-D-VALIDATE-FLAGS: formerly deferred; now wired in rev-244 B6
// via validatePackNamesAgainstCfgs at each packAndSave* call site. The
// eight CLIENT BUILD_VERIFY CRC checks (seq/loc/flo/spotanim/npc/obj/idk/varp)
// are now wired with log-not-throw posture per the established BuildVerify
// convention (see packAndSave* functions and pkg/pack/build_verify.go).
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
// in the NAI-192-D-NO-SRC-NO-OP paragraph below). NAI-199 added two
// server-only specials (category.dat, frame_del.dat); frame_del.dat
// was removed at rev-244 (TS PackShared.ts:355-388 deleted @ 9aadcec4).
// category.dat uses ShouldBuildFile gating and produces .dat without .idx.
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
	reg, _, err := PackConfigsForRegistryAndModelFlags(srcDir, outDir)
	return reg, err
}

// PackConfigsForRegistryAndModelFlags is like PackConfigsForRegistry but also
// returns the modelFlags slice so callers that need it (e.g. pkg/packall.PackAll
// threading flags into graphics.Pack) can observe the flag writes from the five
// consumers (idk/loc/npc/obj/spotanim). TS PackAll.ts:117-129 @ 9aadcec4.
//
// The returned slice is indexed by model id, length = reg.Model.Max.
// Callers must not modify the returned slice concurrently.
//
// pkg/packall.PackAll callers should use PackConfigsForPackAll instead, which
// takes caller-owned reg, modelFlags, and cache so PackAll controls the
// allocation order (TS PackAll.ts:38-40 initialises modelFlags before cache).
func PackConfigsForRegistryAndModelFlags(srcDir, outDir string) (*Registry, []int, error) {
	reg := &Registry{SrcDir: srcDir}
	// Allocate modelFlags sized by ModelPack.max (reg.Model.Max after EnsureModel).
	// EnsureModel is lazy; we call it here so the size is known before the
	// pipeline starts. Matches TS PackAll.ts:38-40:
	//   for (let i = 0; i < ModelPack.max; i++) { modelFlags[i] = 0; }
	// Zero-alloc satisfies that init.
	if _, err := reg.EnsureModel(); err != nil {
		return nil, nil, err
	}
	modelFlags := make([]int, reg.Model.Max)
	if err := packConfigsCoreWithModelFlags(srcDir, outDir, reg, modelFlags, nil); err != nil {
		return nil, nil, err
	}
	return reg, modelFlags, nil
}

// PackConfigsForPackAll is the entry point for pkg/packall.PackAll. Unlike
// PackConfigsForRegistryAndModelFlags, the caller pre-allocates reg and
// modelFlags (after EnsureModel) and supplies a live FileStream cache so
// the packed client/config jagfile is written to cache.Write(0, 2, data, 0)
// in the same call.
//
// TS source: tools/pack/config/PackShared.ts:261-641 (packConfigs) +
// tools/pack/PackAll.ts:38-45 (modelFlags zeroing + cache construction).
func PackConfigsForPackAll(srcDir, outDir string, reg *Registry, modelFlags []int, cache *filestream.FileStream) error {
	return packConfigsCoreWithModelFlags(srcDir, outDir, reg, modelFlags, cache)
}

// PackConfigs is the original entry point (2-arg). Kept for backward
// compatibility with non-PackAll callers.
func PackConfigs(srcDir, outDir string) error {
	_, err := PackConfigsForRegistry(srcDir, outDir)
	return err
}

// packConfigsCoreWithModelFlags is the real implementation of the config
// pack pipeline. modelFlags is a caller-allocated []int indexed by model
// id (len = reg.Model.Max). Per TS PackShared.ts:137-141 each config packer
// receives the slice and the five consumers (idk/loc/npc/obj/spotanim) write
// bit flags back; the caller observes writes via shared backing array
// semantics after this function returns.
//
// PackConfigsForRegistry allocates modelFlags from reg.Model.Max and
// delegates here. pkg/packall.PackAll calls PackConfigsForPackAll which
// passes caller-allocated reg, modelFlags, and cache so the allocation order
// matches TS PackAll.ts:38-45 (modelFlags zeroed before cache open).
//
// cache is an optional *filestream.FileStream. When non-nil, the packed
// client/config jagfile bytes are written to cache.Write(0, 2, data, 0),
// mirroring TS PackShared.ts:641 @ 9aadcec4:
//
//	cache.write(0, 2, fs.readFileSync('data/pack/client/config'))
//
// Callers that do not yet have a FileStream pass nil (e.g.
// PackConfigsForRegistry).
//
// TS source: tools/pack/config/PackShared.ts:261-669 (packConfigs).
func packConfigsCoreWithModelFlags(srcDir, outDir string, reg *Registry, modelFlags []int, cache *filestream.FileStream) error {
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
	// a *PackFile, and is consumed only within packConfigsCoreWithModelFlags.
	var lk *paramLookups
	ensureLk := func() error {
		if lk != nil {
			return nil
		}
		newLk, err := loadParamLookups(srcDir, reg.Varp)
		if err != nil {
			return err
		}
		lk = newLk
		return nil
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
	if err := packAndSaveParam(srcDir, serverOut, reg.Param, lk, constants, modelFlags); err != nil {
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

	// frame_del — REMOVED at rev-244 (TS PackShared.ts:355-388 deleted at
	// 9aadcec4). The packer block was deleted from TS at commit 9aadcec4;
	// goscape mirrors that removal. Consumer check: no Go runtime code in
	// modules/ or cmd/ reads frame_del.dat; pkg/io/jagfile/jagfile.go:513
	// retains "frame_del.dat" in the known-name table for legacy decode
	// support (225-era caches), which is unrelated to the packer. The TS
	// runtime at 9aadcec4 likewise only retains it in Jagfile.ts:405 as a
	// decode-side name entry.

	// .enum — server-only, freshness-gated.
	if GetLatestModified(scriptsDir, ".enum") > 0 &&
		ShouldBuild(scriptsDir, ".enum", filepath.Join(serverOut, "enum.dat")) {
		enumPack, err := NewPackFile(srcDir, "enum", nil)
		if err != nil {
			return err
		}
		if err := packAndSaveEnum(srcDir, serverOut, enumPack, lk, constants, modelFlags); err != nil {
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
		if err := packAndSaveInv(srcDir, serverOut, reg.Inv, reg.Obj, constants, modelFlags); err != nil {
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
		if err := packAndSaveMesAnim(srcDir, serverOut, reg.MesAnim, reg.Seq, constants, modelFlags); err != nil {
			return err
		}
	}

	// .struct — server-only, freshness-gated.
	if GetLatestModified(scriptsDir, ".struct") > 0 &&
		ShouldBuild(scriptsDir, ".struct", filepath.Join(serverOut, "struct.dat")) {
		if _, err := reg.EnsureStruct(); err != nil {
			return err
		}
		if err := packAndSaveStruct(srcDir, serverOut, reg.Struct, paramTypes, lk, constants, modelFlags); err != nil {
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
			if err := packAndSaveDbTable(srcDir, serverOut, reg.DbTable, lk, constants, modelFlags); err != nil {
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
			if err := packAndSaveDbRow(srcDir, serverOut, reg.DbRow, reg.DbTable, dbtableTypes, lk, constants, modelFlags); err != nil {
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
	if err := packAndSaveSeq(srcDir, serverOut, reg.Seq, reg.Anim, reg.Obj, constants, clientJag, modelFlags); err != nil {
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
	if err := packAndSaveLoc(srcDir, serverOut, reg.Loc, reg.Model, reg.Category, reg.Seq, reg.Texture, lk, paramTypes, constants, clientJag, modelFlags); err != nil {
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
	if err := packAndSaveFlo(srcDir, serverOut, reg.Flo, reg.Texture, constants, clientJag, modelFlags); err != nil {
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
	if err := packAndSaveSpotAnim(srcDir, serverOut, reg.SpotAnim, reg.Model, reg.Seq, constants, clientJag, modelFlags); err != nil {
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
	if err := packAndSaveNpc(srcDir, serverOut, reg.Npc, reg.Model, reg.Category, reg.Seq, reg.Hunt, lk, paramTypes, constants, clientJag, modelFlags); err != nil {
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
	if err := packAndSaveObj(srcDir, serverOut, reg.Obj, reg.Model, reg.Category, reg.Seq, lk, paramTypes, constants, clientJag, modelFlags); err != nil {
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
	if err := packAndSaveIdk(srcDir, serverOut, reg.Idk, reg.Model, constants, clientJag, modelFlags); err != nil {
		return err
	}

	// .varp — unconditional (NAI-196-D-UNCONDITIONAL-CLIENT-PACK).
	if err := packAndSaveVarp(srcDir, serverOut, reg.Varp, constants, clientJag, modelFlags); err != nil {
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
		if err := packAndSaveHunt(srcDir, serverOut, reg.Hunt, reg.Category, reg.Inv, reg.Loc, reg.Npc, reg.Obj, reg.Param, reg.Varn, reg.Varp, constants, modelFlags); err != nil {
			return err
		}
	}

	// .varn — server-only, freshness-gated.
	if GetLatestModified(scriptsDir, ".varn") > 0 &&
		ShouldBuild(scriptsDir, ".varn", filepath.Join(serverOut, "varn.dat")) {
		if err := packAndSaveVarn(srcDir, serverOut, reg.Varn, constants, modelFlags); err != nil {
			return err
		}
	}

	// .vars — server-only, freshness-gated.
	if GetLatestModified(scriptsDir, ".vars") > 0 &&
		ShouldBuild(scriptsDir, ".vars", filepath.Join(serverOut, "vars.dat")) {
		if err := packAndSaveVars(srcDir, serverOut, reg.Vars, constants, modelFlags); err != nil {
			return err
		}
	}

	configPath := filepath.Join(clientOut, "config")
	if err := clientJag.Save(configPath); err != nil {
		return err
	}
	// TS PackShared.ts:641 @ 9aadcec4: cache.write(0, 2, fs.readFileSync('data/pack/client/config'))
	if cache != nil {
		data, err := os.ReadFile(configPath)
		if err != nil {
			return fmt.Errorf("read client/config for cache write: %w", err)
		}
		cache.Write(0, 2, data, 0)
	}
	return nil
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

// readAndValidate is the shared head of all 18 packAndSave* funcs:
// ReadTypedConfigs the .<ext> sources under srcDir, then
// validatePackNamesAgainstCfgs the result against pf. required and
// parse are forwarded verbatim to ReadTypedConfigs.
func readAndValidate(srcDir, ext string, required []string, parse ParseFn, c Constants, pf *PackFile) (map[string][]ConfigLine, error) {
	cfgs, err := ReadTypedConfigs(srcDir, ext, required, parse, c)
	if err != nil {
		return nil, err
	}
	if err := validatePackNamesAgainstCfgs(pf, cfgs, ext); err != nil {
		return nil, err
	}
	return cfgs, nil
}

// saveTransmittedConfig is the shared tail of the eight transmitted
// packAndSave* funcs (flo, idk, loc, npc, obj, seq, spotanim, varp —
// the eight client-jagfile configs enumerated on the clientConfigCRC*
// const block above): BuildVerify the client buffer against crc
// (log-not-throw), save the server pack, then queue the client pack
// into clientJag.
//
// label is the caller's func name (e.g. "packAndSaveVarp"), used
// verbatim in the BUILD_VERIFY stderr diagnostic. name is the
// lowercase file basename (e.g. "varp") used for both
// serverOut/<name>.{dat,idx} and the clientJag entry names. The exact
// CRC magic numbers/TS line references live in the clientConfigCRC*
// const block above.
func saveTransmittedConfig(label, name, serverOut string, server, client *PackedData, clientJag *jagfile.Jagfile, crc int32) error {
	if err := BuildVerify(client.Dat.Data, len(client.Dat.Data), crc); err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v (BUILD_VERIFY)\n", label, err)
	}
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

func packAndSaveVarp(srcDir, serverOut string, pf *PackFile, c Constants, clientJag *jagfile.Jagfile, modelFlags []int) error {
	cfgs, err := readAndValidate(srcDir, ".varp", nil, parseVarpConfig, c, pf)
	if err != nil {
		return err
	}
	server, client := packVarpConfigs(cfgs, pf, modelFlags)
	return saveTransmittedConfig("packAndSaveVarp", "varp", serverOut, server, client, clientJag, clientConfigCRCVarp)
}

func packAndSaveVarn(srcDir, serverOut string, pf *PackFile, c Constants, modelFlags []int) error {
	cfgs, err := readAndValidate(srcDir, ".varn", nil, parseVarnConfig, c, pf)
	if err != nil {
		return err
	}
	pd := packVarnConfigs(cfgs, pf, modelFlags)
	return pd.Save(filepath.Join(serverOut, "varn.dat"), filepath.Join(serverOut, "varn.idx"))
}

func packAndSaveVars(srcDir, serverOut string, pf *PackFile, c Constants, modelFlags []int) error {
	cfgs, err := readAndValidate(srcDir, ".vars", nil, parseVarsConfig, c, pf)
	if err != nil {
		return err
	}
	pd := packVarsConfigs(cfgs, pf, modelFlags)
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

// packAndSaveParam reads .param sources, packs them, and writes the
// server .dat/.idx. TS callback is server-only — the param branch is
// not added to the client jagfile (PackShared.ts:323 passes `() => {}`
// as the read-side callback).
//
// TS source: tools/pack/PackShared.ts (param branch of packConfigs).
func packAndSaveParam(srcDir, serverOut string, pf *PackFile, lk *paramLookups, c Constants, modelFlags []int) error {
	cfgs, err := readAndValidate(srcDir, ".param", nil, parseParamConfig, c, pf)
	if err != nil {
		return err
	}
	server, _, err := packParamConfigs(cfgs, pf, lk, modelFlags)
	if err != nil {
		return err
	}
	return server.Save(
		filepath.Join(serverOut, "param.dat"),
		filepath.Join(serverOut, "param.idx"),
	)
}

func packAndSaveEnum(srcDir, serverOut string, pf *PackFile, lk *paramLookups, c Constants, modelFlags []int) error {
	cfgs, err := readAndValidate(srcDir, ".enum", nil, parseEnumConfig, c, pf)
	if err != nil {
		return err
	}
	pd, err := packEnumConfigs(cfgs, pf, lk, modelFlags)
	if err != nil {
		return err
	}
	return pd.Save(filepath.Join(serverOut, "enum.dat"), filepath.Join(serverOut, "enum.idx"))
}

func packAndSaveInv(srcDir, serverOut string, pf, objPack *PackFile, c Constants, modelFlags []int) error {
	cfgs, err := readAndValidate(srcDir, ".inv", nil, parseInvConfigFor(objPack), c, pf)
	if err != nil {
		return err
	}
	pd, err := packInvConfigs(cfgs, pf, modelFlags)
	if err != nil {
		return err
	}
	return pd.Save(filepath.Join(serverOut, "inv.dat"), filepath.Join(serverOut, "inv.idx"))
}

func packAndSaveMesAnim(srcDir, serverOut string, pf, seqPack *PackFile, c Constants, modelFlags []int) error {
	cfgs, err := readAndValidate(srcDir, ".mesanim", nil, parseMesAnimConfigFor(seqPack), c, pf)
	if err != nil {
		return err
	}
	pd := packMesAnimConfigs(cfgs, pf, modelFlags)
	return pd.Save(filepath.Join(serverOut, "mesanim.dat"), filepath.Join(serverOut, "mesanim.idx"))
}

func packAndSaveStruct(srcDir, serverOut string, pf *PackFile, paramTypes *objtype.ParamTypeConfigs, lk *paramLookups, c Constants, modelFlags []int) error {
	cfgs, err := readAndValidate(srcDir, ".struct", nil, parseStructConfigFor(paramTypes, lk), c, pf)
	if err != nil {
		return err
	}
	pd := packStructConfigs(cfgs, pf, modelFlags)
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
func packAndSaveLoc(srcDir, serverOut string, locPack, modelPack, categoryPack, seqPack, texturePack *PackFile, lk *paramLookups, paramTypes *objtype.ParamTypeConfigs, c Constants, clientJag *jagfile.Jagfile, modelFlags []int) error {
	parse := parseLocConfigFor(categoryPack, seqPack, texturePack, lk, paramTypes)
	cfgs, err := readAndValidate(srcDir, ".loc", nil, parse, c, locPack)
	if err != nil {
		return err
	}
	server, client, err := packLocConfigs(cfgs, locPack, modelPack, modelFlags)
	if err != nil {
		return err
	}
	return saveTransmittedConfig("packAndSaveLoc", "loc", serverOut, server, client, clientJag, clientConfigCRCLoc)
}

// packAndSaveNpc reads .npc sources, packs them, writes server
// .dat/.idx, and queues the client .dat/.idx into clientJag.
//
// NAI-196-D-UNCONDITIONAL-CLIENT-PACK: this branch runs every
// PackConfigs invocation regardless of source freshness.
//
// TS source: tools/pack/config/NpcConfig.ts:265-509.
func packAndSaveNpc(srcDir, serverOut string, npcPack, modelPack, categoryPack, seqPack, huntPack *PackFile, lk *paramLookups, paramTypes *objtype.ParamTypeConfigs, c Constants, clientJag *jagfile.Jagfile, modelFlags []int) error {
	parse := parseNpcConfigFor(modelPack, categoryPack, seqPack, huntPack, lk, paramTypes)
	cfgs, err := readAndValidate(srcDir, ".npc", nil, parse, c, npcPack)
	if err != nil {
		return err
	}
	server, client, err := packNpcConfigs(cfgs, npcPack, modelFlags)
	if err != nil {
		return err
	}
	return saveTransmittedConfig("packAndSaveNpc", "npc", serverOut, server, client, clientJag, clientConfigCRCNpc)
}

// packAndSaveObj reads .obj sources, packs them, writes server
// .dat/.idx, and queues the client .dat/.idx into clientJag.
//
// NAI-196-D-UNCONDITIONAL-CLIENT-PACK: this branch runs every
// PackConfigs invocation regardless of source freshness.
//
// TS source: tools/pack/config/ObjConfig.ts:196-440.
func packAndSaveObj(srcDir, serverOut string, objPack, modelPack, categoryPack, seqPack *PackFile, lk *paramLookups, paramTypes *objtype.ParamTypeConfigs, c Constants, clientJag *jagfile.Jagfile, modelFlags []int) error {
	parse := parseObjConfigFor(modelPack, categoryPack, seqPack, objPack, lk, paramTypes)
	cfgs, err := readAndValidate(srcDir, ".obj", nil, parse, c, objPack)
	if err != nil {
		return err
	}
	server, client, err := packObjConfigs(cfgs, objPack, modelFlags)
	if err != nil {
		return err
	}
	return saveTransmittedConfig("packAndSaveObj", "obj", serverOut, server, client, clientJag, clientConfigCRCObj)
}

// packAndSaveSeq reads .seq sources, packs them, writes server
// .dat/.idx, and queues the client .dat/.idx into clientJag.
//
// NAI-196-D-UNCONDITIONAL-CLIENT-PACK: this branch runs every
// PackConfigs invocation regardless of source freshness, matching
// TS PackShared.ts:460 (rebuildClient=true ungates shouldBuild).
//
// TS source: tools/pack/config/SeqConfig.ts:121-208.
func packAndSaveSeq(srcDir, serverOut string, seqPack, animPack, objPack *PackFile, c Constants, clientJag *jagfile.Jagfile, modelFlags []int) error {
	parse := parseSeqConfigFor(animPack, objPack)
	cfgs, err := readAndValidate(srcDir, ".seq", nil, parse, c, seqPack)
	if err != nil {
		return err
	}
	server, client := packSeqConfigs(cfgs, seqPack, modelFlags)
	return saveTransmittedConfig("packAndSaveSeq", "seq", serverOut, server, client, clientJag, clientConfigCRCSeq)
}

// packAndSaveFlo reads .flo sources, packs them, writes server
// .dat/.idx (which contain only per-id Next() boundaries — no opcode
// bytes), and queues the client .dat/.idx into clientJag.
//
// NAI-196-D-UNCONDITIONAL-CLIENT-PACK: unconditional.
//
// TS source: tools/pack/config/FloConfig.ts:63-104.
func packAndSaveFlo(srcDir, serverOut string, floPack, texturePack *PackFile, c Constants, clientJag *jagfile.Jagfile, modelFlags []int) error {
	parse := parseFloConfigFor(texturePack)
	cfgs, err := readAndValidate(srcDir, ".flo", nil, parse, c, floPack)
	if err != nil {
		return err
	}
	server, client := packFloConfigs(cfgs, floPack, modelFlags)
	return saveTransmittedConfig("packAndSaveFlo", "flo", serverOut, server, client, clientJag, clientConfigCRCFlo)
}

// packAndSaveSpotAnim reads .spotanim sources, packs them, writes
// server .dat/.idx, and queues the client .dat/.idx into clientJag.
//
// NAI-196-D-UNCONDITIONAL-CLIENT-PACK: unconditional.
//
// TS source: tools/pack/config/SpotAnimConfig.ts:92-152.
func packAndSaveSpotAnim(srcDir, serverOut string, spotanimPack, modelPack, seqPack *PackFile, c Constants, clientJag *jagfile.Jagfile, modelFlags []int) error {
	parse := parseSpotAnimConfigFor(modelPack, seqPack)
	cfgs, err := readAndValidate(srcDir, ".spotanim", nil, parse, c, spotanimPack)
	if err != nil {
		return err
	}
	server, client := packSpotAnimConfigs(cfgs, spotanimPack, modelFlags)
	return saveTransmittedConfig("packAndSaveSpotAnim", "spotanim", serverOut, server, client, clientJag, clientConfigCRCSpotAnim)
}

// packAndSaveDbTable reads .dbtable sources, packs them, and writes
// server .dat/.idx. Server-only — does NOT contribute to clientJag.
//
// TS source: tools/pack/config/DbTableConfig.ts:78-224.
func packAndSaveDbTable(srcDir, serverOut string, dbtablePack *PackFile, lk *paramLookups, c Constants, modelFlags []int) error {
	cfgs, err := readAndValidate(srcDir, ".dbtable", nil, parseDbTableConfig, c, dbtablePack)
	if err != nil {
		return err
	}
	pd, err := packDbTableConfigs(cfgs, dbtablePack, lk, modelFlags)
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
func packAndSaveDbRow(srcDir, serverOut string, dbrowPack, dbtablePack *PackFile, dbtableTypes *objtype.DbTableTypeConfigs, lk *paramLookups, c Constants, modelFlags []int) error {
	parse := parseDbRowConfigFor(dbtablePack)
	cfgs, err := readAndValidate(srcDir, ".dbrow", nil, parse, c, dbrowPack)
	if err != nil {
		return err
	}
	pd, err := packDbRowConfigs(cfgs, dbrowPack, dbtableTypes, lk, modelFlags)
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
func packAndSaveHunt(srcDir, serverOut string, huntPack, categoryPack, invPack, locPack, npcPack, objPack, paramPack, varnPack, varpPack *PackFile, c Constants, modelFlags []int) error {
	parse := parseHuntConfigFor(categoryPack, invPack, locPack, npcPack, objPack, paramPack, varnPack, varpPack)
	cfgs, err := readAndValidate(srcDir, ".hunt", nil, parse, c, huntPack)
	if err != nil {
		return err
	}
	pd, err := packHuntConfigs(cfgs, huntPack, modelFlags)
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
func packAndSaveIdk(srcDir, serverOut string, idkPack, modelPack *PackFile, c Constants, clientJag *jagfile.Jagfile, modelFlags []int) error {
	parse := parseIdkConfigFor(modelPack)
	cfgs, err := readAndValidate(srcDir, ".idk", nil, parse, c, idkPack)
	if err != nil {
		return err
	}
	server, client := packIdkConfigs(cfgs, idkPack, modelFlags)
	return saveTransmittedConfig("packAndSaveIdk", "idk", serverOut, server, client, clientJag, clientConfigCRCIdk)
}

// validatePackNamesAgainstCfgs builds the configNames set from the keys of
// cfgs (the map returned by ReadTypedConfigs) and delegates to
// ValidateConfigPackNames. This is the live-path wiring for the rev-244
// universal config-name verification contract (TS PackFile.ts:117-121
// @ 9aadcec4).
//
// BEFORE this wiring (NAI-191-D-VALIDATE-FLAGS-DEFERRED), ValidateConfigPackNames
// and CrawlConfigNames existed as dead code introduced by commit 2cfec7ea.
// This function closes that gap: ReadTypedConfigs already walks the same
// scripts/ tree that CrawlConfigNames would walk, so its result map's
// keys are exactly the configNames set — no second crawl is needed.
//
// TS source: tools/pack/PackFile.ts:117-121 @ 9aadcec4 (rev-244 B6).
func validatePackNamesAgainstCfgs(pf *PackFile, cfgs map[string][]ConfigLine, ext string) error {
	configNames := make(map[string]struct{}, len(cfgs))
	for name := range cfgs {
		configNames[name] = struct{}{}
	}
	return ValidateConfigPackNames(pf, configNames, ext)
}
