package pack

import (
	"path/filepath"
)

// PackConfigs runs the per-config packing pipeline. NAI-192 wires only
// .varn and .vars; subsequent NAI-193+ sub-specs add branches.
//
// Each branch is freshness-gated via ShouldBuild against the relevant
// source extension. Outputs land at <outDir>/server/<type>.{dat,idx}.
//
// NAI-192-D-VARP-UNIQUENESS-DEFERRED: TS PackShared.packConfigs runs
// a cross-domain var-name uniqueness check across {VarpPack, VarnPack,
// VarsPack}. Deferred — lands with whichever of {varp, varn, vars} is
// last to ship. No production callsite this slice, so fixture-driven
// duplicates cannot reach the orchestrator.
//
// NAI-192-D-PACKFILE-SINGLETONS-DEFERRED: TS uses module-level
// VarnPack/VarsPack singletons; goscape constructs *PackFile from
// srcDir per call (NAI-191 §2 deferred all 26 singletons).
//
// TS source: tools/pack/config/PackShared.ts:261-669 (packConfigs).
func PackConfigs(srcDir, outDir string) error {
	constants, err := LoadConstants(srcDir)
	if err != nil {
		return err
	}

	// TODO(NAI-VARP+): var-name uniqueness across {VarpPack, VarnPack, VarsPack}.

	scriptsDir := filepath.Join(srcDir, "scripts")
	serverOut := filepath.Join(outDir, "server")

	if GetLatestModified(scriptsDir, ".varn") > 0 &&
		ShouldBuild(scriptsDir, ".varn", filepath.Join(serverOut, "varn.dat")) {
		if err := packAndSaveVarn(srcDir, serverOut, constants); err != nil {
			return err
		}
	}

	if GetLatestModified(scriptsDir, ".vars") > 0 &&
		ShouldBuild(scriptsDir, ".vars", filepath.Join(serverOut, "vars.dat")) {
		if err := packAndSaveVars(srcDir, serverOut, constants); err != nil {
			return err
		}
	}

	return nil
}

func packAndSaveVarn(srcDir, serverOut string, c Constants) error {
	pf, err := NewPackFile(srcDir, "varn", nil)
	if err != nil {
		return err
	}
	cfgs, err := ReadTypedConfigs(srcDir, ".varn", nil, parseVarnConfig, c)
	if err != nil {
		return err
	}
	pd := packVarnConfigs(cfgs, pf)
	return pd.Save(filepath.Join(serverOut, "varn.dat"), filepath.Join(serverOut, "varn.idx"))
}

func packAndSaveVars(srcDir, serverOut string, c Constants) error {
	pf, err := NewPackFile(srcDir, "vars", nil)
	if err != nil {
		return err
	}
	cfgs, err := ReadTypedConfigs(srcDir, ".vars", nil, parseVarsConfig, c)
	if err != nil {
		return err
	}
	pd := packVarsConfigs(cfgs, pf)
	return pd.Save(filepath.Join(serverOut, "vars.dat"), filepath.Join(serverOut, "vars.idx"))
}
