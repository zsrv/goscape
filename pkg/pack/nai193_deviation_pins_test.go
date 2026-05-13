package pack

import (
	"os"
	"strings"
	"testing"
)

// NAI-193-D-PACKFILE-SINGLETONS-DEFERRED: no top-level VarpPack decl in
// pkg/pack (mirrors the NAI-192 absence-pin for VarnPack/VarsPack).
// scanPackageDecls helper lives in nai192_deviation_pins_test.go.
func TestNAI193_PackFileSingletonsDeferred_NoModuleLevelVarpPack(t *testing.T) {
	decls := scanPackageDecls(t)
	if decls["VarpPack"] {
		t.Errorf("found top-level decl \"VarpPack\" in pkg/pack — violates NAI-193-D-PACKFILE-SINGLETONS-DEFERRED")
	}
}

// NAI-193-D-VALIDATE-DEFERRED: pkg/pack/varp.go must NOT reference any
// BUILD_VERIFY-style validate callback identifiers.
func TestNAI193_ValidateDeferred_NoBuildVerifyInVarpSource(t *testing.T) {
	body, err := os.ReadFile("varp.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, banned := range []string{"BuildVerify", "BUILD_VERIFY", "validateVarp", "checkCRC", "checkcrc"} {
		if strings.Contains(string(body), banned) {
			t.Errorf("found %q in pkg/pack/varp.go — violates NAI-193-D-VALIDATE-DEFERRED", banned)
		}
	}
}

// NAI-193-D-FRESH-CLIENT-JAGFILE: PackConfigs must construct the client
// jagfile via NewJagfile(nil) and must NOT call LoadJagfile (which
// would indicate the deviation has flipped to "preserve existing
// entries").
func TestNAI193_FreshClientJagfile_NewNotLoad(t *testing.T) {
	body, err := os.ReadFile("pack_configs.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "NewJagfile(nil)") {
		t.Errorf("pkg/pack/pack_configs.go must call NewJagfile(nil) — NAI-193-D-FRESH-CLIENT-JAGFILE")
	}
	if strings.Contains(string(body), "LoadJagfile") {
		t.Errorf("pkg/pack/pack_configs.go must NOT call LoadJagfile — flipping NAI-193-D-FRESH-CLIENT-JAGFILE would require a new deviation tag")
	}
}

// Verifies that the cross-domain var-name uniqueness deferral (from NAI-192)
// has been retired: PackConfigs source must contain the uniqueness-check call
// (not the TODO placeholder comment that existed before NAI-193 T4).
func TestNAI193_UniquenessCheckRetiredVarpDeferral(t *testing.T) {
	body, err := os.ReadFile("pack_configs.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "checkVarNameUniqueness") {
		t.Errorf("pkg/pack/pack_configs.go must call checkVarNameUniqueness — cross-domain uniqueness deferral (NAI-192) is retired by NAI-193")
	}
}
