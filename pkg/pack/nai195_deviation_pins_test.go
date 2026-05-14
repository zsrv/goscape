package pack

import (
	"os"
	"strings"
	"testing"
)

// NAI-195-D-PACKFILE-SINGLETONS-DEFERRED: no top-level *Pack decls
// for enum/inv/mesanim/struct (continues NAI-193/194 deferral).
func TestNAI195_PackFileSingletonsDeferred_FourNewConfigs(t *testing.T) {
	decls := scanPackageDecls(t)
	for _, banned := range []string{"EnumPack", "InvPack", "MesAnimPack", "StructPack"} {
		if decls[banned] {
			t.Errorf("found top-level decl %q in pkg/pack — violates PACKFILE-SINGLETONS-DEFERRED", banned)
		}
	}
}

// NAI-195-D-VALIDATE-DEFERRED extension: no BUILD_VERIFY-style
// callback identifiers in any of the 4 new packer sources.
func TestNAI195_ValidateDeferred_NoBuildVerifyInNewSources(t *testing.T) {
	for _, src := range []string{"enum.go", "inv.go", "mesanim.go", "struct.go"} {
		body, err := os.ReadFile(src)
		if err != nil {
			t.Fatal(err)
		}
		for _, banned := range []string{"BuildVerify", "BUILD_VERIFY", "checkCRC", "checkcrc"} {
			if strings.Contains(string(body), banned) {
				t.Errorf("found %q in pkg/pack/%s — violates VALIDATE-DEFERRED", banned, src)
			}
		}
	}
}
