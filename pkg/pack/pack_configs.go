package pack

import (
	"fmt"
	"path/filepath"

	"github.com/zsrv/goscape/pkg/io/jagfile"
)

// PackConfigs runs the per-config packing pipeline. NAI-193 wires
// .varp (server + client jagfile), .varn (server), and .vars (server).
// NAI-194 adds .param (server + client jagfile).
// Subsequent NAI-194+ sub-specs add the remaining per-config branches.
//
// Each branch is freshness-gated via ShouldBuild against the relevant
// source extension. Server outputs land at <outDir>/server/<type>.{dat,idx}.
// Client outputs land in a fresh jagfile at <outDir>/client/config —
// saved only if at least one client-side branch fires.
//
// All three var-domain PackFiles are constructed up-front so the
// cross-domain uniqueness check has all three name maps
// available. Each *.pack file is small (<1 KB); cost is fixed.
//
// NAI-193-D-PACKFILE-SINGLETONS-DEFERRED: TS uses module-level
// VarpPack/VarnPack/VarsPack singletons; goscape constructs *PackFile
// from srcDir per call (continuation of NAI-191 §2 / NAI-192
// deferral of all 26 module-level pack singletons).
//
// NAI-193-D-FRESH-CLIENT-JAGFILE: client jagfile starts fresh
// (NewJagfile(nil)). Pre-existing entries in <outDir>/client/config
// are truncated if only a subset of client-side branches rebuild.
// Mirrors TS Jagfile.new() at PackShared.ts:336.
//
// NAI-193-D-VALIDATE-DEFERRED: TS BUILD_VERIFY callback (.varp magic
// 705633567 at PackShared.ts:631-633) deferred — continuation of
// NAI-191 §2.
//
// NAI-192-D-NO-SRC-NO-OP: goscape-only `GetLatestModified > 0`
// pre-guard suppresses output when no source files exist. TS would
// enter ShouldBuild's output-missing arm and write a zero-entry
// .dat/.idx pair; goscape elides that write.
//
// NAI-194-D-PARAM-AFTER-VARS: TS processes .param FIRST (before jag
// creation and before .varp/.varn/.vars) so other configs can resolve
// param defaults during packing — see PackShared.ts:315 "We have to
// pack params for other configs to parse correctly". Goscape runs
// .param after the var-domain trio because NAI-194 introduces .param
// in isolation (no other config packer yet exists that would consume
// param outputs). NAI-195+ may need to re-evaluate ordering when
// .loc/.obj/.npc packers land.
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

	// Fresh client jagfile per NAI-193-D-FRESH-CLIENT-JAGFILE. Saved
	// only when at least one client-side branch contributes a write.
	clientJag, err := jagfile.NewJagfile(nil)
	if err != nil {
		return err
	}
	clientJagDirty := false

	if GetLatestModified(scriptsDir, ".varp") > 0 &&
		ShouldBuild(scriptsDir, ".varp", filepath.Join(serverOut, "varp.dat")) {
		if err := packAndSaveVarp(srcDir, serverOut, varpPack, constants, clientJag); err != nil {
			return err
		}
		clientJagDirty = true
	}

	if GetLatestModified(scriptsDir, ".varn") > 0 &&
		ShouldBuild(scriptsDir, ".varn", filepath.Join(serverOut, "varn.dat")) {
		if err := packAndSaveVarn(srcDir, serverOut, varnPack, constants); err != nil {
			return err
		}
	}

	if GetLatestModified(scriptsDir, ".vars") > 0 &&
		ShouldBuild(scriptsDir, ".vars", filepath.Join(serverOut, "vars.dat")) {
		if err := packAndSaveVars(srcDir, serverOut, varsPack, constants); err != nil {
			return err
		}
	}

	// NAI-194-D-PARAM-AFTER-VARS: see PackConfigs doc-comment.
	if GetLatestModified(scriptsDir, ".param") > 0 &&
		ShouldBuild(scriptsDir, ".param", filepath.Join(serverOut, "param.dat")) {
		paramPack, err := NewPackFile(srcDir, "param", nil)
		if err != nil {
			return err
		}
		lk, err := loadParamLookups(srcDir, varpPack)
		if err != nil {
			return err
		}
		if err := packAndSaveParam(srcDir, serverOut, paramPack, lk, constants, clientJag); err != nil {
			return err
		}
		clientJagDirty = true
	}

	if clientJagDirty {
		if err := clientJag.Save(filepath.Join(clientOut, "config"), false); err != nil {
			return err
		}
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
