package pack

import (
	"path/filepath"
	"testing"

	"github.com/zsrv/goscape/pkg/objtype"
)

// TestVarpPacker_LoaderRoundTrip exercises the full packer → loader path
// for a varp with non-default field values (scope=perm, transmit=yes,
// clientcode=7).  clientcode=7 also triggers RunID=0 discovery.
func TestVarpPacker_LoaderRoundTrip(t *testing.T) {
	srcDir := t.TempDir()
	outDir := t.TempDir()

	writeFile(t, filepath.Join(srcDir, "scripts", "test.varp"),
		"[run]\nscope=perm\ntype=int\ntransmit=yes\nclientcode=7\n")
	writeFile(t, filepath.Join(srcDir, "pack", "varp.pack"), "0=run\n")
	writeFile(t, filepath.Join(srcDir, "pack", "varn.pack"), "")
	writeFile(t, filepath.Join(srcDir, "pack", "vars.pack"), "")
	ClearFsCache()

	if err := PackConfigs(srcDir, outDir); err != nil {
		t.Fatal(err)
	}

	cfgs, err := objtype.LoadVarpTypes(outDir)
	if err != nil {
		t.Fatal(err)
	}

	if len(cfgs.Configs) != 1 {
		t.Fatalf("len(Configs)=%d, want 1", len(cfgs.Configs))
	}
	v := cfgs.Configs[0]
	if v.DebugName != "run" {
		t.Fatalf("DebugName=%q, want %q", v.DebugName, "run")
	}
	if v.Scope != objtype.VarpScopePerm {
		t.Fatalf("Scope=%d, want VarpScopePerm=%d", v.Scope, objtype.VarpScopePerm)
	}
	if v.Type != objtype.ScriptVarTypeInt {
		t.Fatalf("Type=%v, want ScriptVarTypeInt", v.Type)
	}
	if !v.Transmit {
		t.Fatal("Transmit=false, want true")
	}
	if v.ClientCode != 7 {
		t.Fatalf("ClientCode=%d, want 7", v.ClientCode)
	}
	if cfgs.RunID != 0 {
		t.Fatalf("RunID=%d, want 0", cfgs.RunID)
	}
}

// TestVarpPacker_LoaderRoundTrip_ProtectFalse verifies that protect=no
// causes opcode 4 to be emitted by the packer and the loader flips the
// default (Protect=true → false).
func TestVarpPacker_LoaderRoundTrip_ProtectFalse(t *testing.T) {
	srcDir := t.TempDir()
	outDir := t.TempDir()

	writeFile(t, filepath.Join(srcDir, "scripts", "test.varp"),
		"[unprotected]\nprotect=no\n")
	writeFile(t, filepath.Join(srcDir, "pack", "varp.pack"), "0=unprotected\n")
	writeFile(t, filepath.Join(srcDir, "pack", "varn.pack"), "")
	writeFile(t, filepath.Join(srcDir, "pack", "vars.pack"), "")
	ClearFsCache()

	if err := PackConfigs(srcDir, outDir); err != nil {
		t.Fatal(err)
	}

	cfgs, err := objtype.LoadVarpTypes(outDir)
	if err != nil {
		t.Fatal(err)
	}

	if len(cfgs.Configs) != 1 {
		t.Fatalf("len(Configs)=%d, want 1", len(cfgs.Configs))
	}
	if cfgs.Configs[0].Protect {
		t.Fatal("Protect=true, want false (opcode 4 should have been emitted)")
	}
}

// TestVarpPacker_LoaderRoundTrip_ProtectDefaultsTrue verifies that when
// protect= is absent from the source the packer omits opcode 4 and the
// loader preserves the NewVarPlayerType default (Protect=true).
func TestVarpPacker_LoaderRoundTrip_ProtectDefaultsTrue(t *testing.T) {
	srcDir := t.TempDir()
	outDir := t.TempDir()

	// protect= deliberately omitted.
	writeFile(t, filepath.Join(srcDir, "scripts", "test.varp"),
		"[defaultprotect]\nscope=temp\ntype=int\n")
	writeFile(t, filepath.Join(srcDir, "pack", "varp.pack"), "0=defaultprotect\n")
	writeFile(t, filepath.Join(srcDir, "pack", "varn.pack"), "")
	writeFile(t, filepath.Join(srcDir, "pack", "vars.pack"), "")
	ClearFsCache()

	if err := PackConfigs(srcDir, outDir); err != nil {
		t.Fatal(err)
	}

	cfgs, err := objtype.LoadVarpTypes(outDir)
	if err != nil {
		t.Fatal(err)
	}

	if len(cfgs.Configs) != 1 {
		t.Fatalf("len(Configs)=%d, want 1", len(cfgs.Configs))
	}
	if !cfgs.Configs[0].Protect {
		t.Fatal("Protect=false, want true (default; opcode 4 must NOT have been emitted)")
	}
}
