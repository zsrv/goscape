// pkg/pack/nai212_deviation_pins_test.go
package pack

import (
	"os"
	"strings"
	"testing"
)

// TestNAI212DeviationPin_RevalidatePackInsidePackConfigs ensures the
// REVALIDATEPACK-INSIDE-PACKCONFIGS tag remains grep-discoverable in
// pkg/packall/packall.go. Permanent (no retirement plan unless
// PackConfigs is refactored to decouple namemap refresh from packing).
//
// NAI-213 moved PackAll out of pkg/pack into its own top-level
// package pkg/packall to break the import cycle that arose when
// client-stage subpackages (clientinterface, sprites, audio, graphics,
// wordenc, maps) started importing pkg/pack for the *Registry shared
// type.
func TestNAI212DeviationPin_RevalidatePackInsidePackConfigs(t *testing.T) {
	requireTagInFile(t, "../packall/packall.go", "NAI-212-D-REVALIDATEPACK-INSIDE-PACKCONFIGS")
}

// TestNAI212DeviationPin_ExplicitSourcePaths ensures the
// EXPLICIT-SOURCEPATHS tag remains grep-discoverable in
// compiler/run_server_compiler.go. Permanent.
func TestNAI212DeviationPin_ExplicitSourcePaths(t *testing.T) {
	requireTagInFile(t, "compiler/run_server_compiler.go", "NAI-212-D-EXPLICIT-SOURCEPATHS")
}

// TestNAI212DeviationPin_PointerNameTranslation pins the T2-emergent
// translation between pkg/script.Pointers "_2"-suffix names and
// pkg/pack/compiler/pointer.ForName dot-prefix Representation strings.
// Retires if pointer.ForName grows a secondary alias map covering the
// "_2"-suffix names directly, or if pkg/script.Pointers is rewritten
// to use dot-prefixed names.
func TestNAI212DeviationPin_PointerNameTranslation(t *testing.T) {
	requireTagInFile(t, "compiler/run_server_compiler.go", "NAI-212-D-POINTER-NAME-TRANSLATION")
}

// TestNAI212DeviationPin_MissingScriptsDirTolerant pins the T2-emergent
// tolerance contract: buildSymbolsCore returns nil for a missing
// scripts/ directory (TS errors). Pin lives in the test file because
// the deviation is most-visibly enforced by its asserting test.
func TestNAI212DeviationPin_MissingScriptsDirTolerant(t *testing.T) {
	requireTagInFile(t, "compiler/run_server_compiler_test.go", "NAI-212-D-MISSING-SCRIPTS-DIR-TOLERANT")
}

func requireTagInFile(t *testing.T, relPath, tag string) {
	t.Helper()
	data, err := os.ReadFile(relPath)
	if err != nil {
		t.Fatalf("read %s: %v", relPath, err)
	}
	if !strings.Contains(string(data), tag) {
		t.Errorf("%s missing tag %q", relPath, tag)
	}
}
