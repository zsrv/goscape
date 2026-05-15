// pkg/pack/compiler/semantics/nai205_deviation_pins_test.go
package semantics

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// readPackageFiles concatenates every non-_test.go file in dir.
func readPackageFiles(t *testing.T, dir string) string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("readdir %q: %v", dir, err)
	}
	var sb strings.Builder
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatalf("read %q: %v", filepath.Join(dir, e.Name()), err)
		}
		sb.Write(b)
		sb.WriteByte('\n')
	}
	return sb.String()
}

func pin(t *testing.T, dir, tag string) {
	t.Helper()
	src := readPackageFiles(t, dir)
	if !strings.Contains(src, tag) {
		t.Fatalf("%q missing deviation tag %q", dir, tag)
	}
}

func TestPin_NAI205D_NoNodeReportError(t *testing.T) {
	pin(t, "../diagnostics", "NAI-205-D-NO-NODE-REPORT-ERROR")
}

func TestPin_NAI205D_TypeOptionsFlat(t *testing.T) {
	pin(t, "../type", "NAI-205-D-TYPEOPTIONS-FLAT")
}

func TestPin_NAI205D_MetaTypeFlat(t *testing.T) {
	pin(t, "../type", "NAI-205-D-METATYPE-FLAT")
}

func TestPin_NAI205D_TypeNoIntern(t *testing.T) {
	pin(t, "../type", "NAI-205-D-TYPE-NO-INTERN")
}

func TestPin_NAI205D_ScriptSymbolNoPointers(t *testing.T) {
	pin(t, "../symbol", "NAI-205-D-SCRIPTSYMBOL-NO-POINTERS")
}

func TestPin_NAI205D_SymbolTypeStringKey(t *testing.T) {
	pin(t, "../symbol", "NAI-205-D-SYMBOLTYPE-STRING-KEY")
}

func TestPin_NAI205D_TriggerPointersDeferred(t *testing.T) {
	pin(t, "../trigger", "NAI-205-D-TRIGGER-POINTERS-DEFERRED")
}

func TestPin_NAI205D_StrictInvertedPolarity(t *testing.T) {
	pin(t, ".", "NAI-205-D-STRICT-INVERTED-POLARITY")
}

func TestPin_NAI205D_AstRefInterfaces(t *testing.T) {
	pin(t, "../ast", "NAI-205-D-AST-REF-INTERFACES")
}

func TestPin_NAI205D_HandlerRequiredMethods(t *testing.T) {
	pin(t, "../diagnostics", "NAI-205-D-HANDLER-REQUIRED-METHODS")
}

func TestPin_NAI205D_NoVisitBlock(t *testing.T) {
	pin(t, ".", "NAI-205-D-NO-VISIT-BLOCK")
}

func TestPin_NAI205D_NormalizeUnicodeSubset(t *testing.T) {
	pin(t, "../symbol", "NAI-205-D-NORMALIZE-UNICODE-SUBSET")
}

func TestPin_NAI205D_MapZoneAtoiStrict(t *testing.T) {
	pin(t, ".", "NAI-205-D-MAPZONE-ATOI-STRICT")
}
