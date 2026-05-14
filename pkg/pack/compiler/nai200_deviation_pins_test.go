package compiler

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// scanCompilerPkg reads every .go file in pkg/pack/compiler/ (the
// current package directory, since tests run with cwd = package dir)
// excluding _test.go files, and returns concatenated content. Used by
// the NAI-200 deviation-tag pin.
//
// Distinct from sibling pkg/pack/scanPkgPack (in nai196_deviation_pins_test.go)
// because that one walks `..` rooted at pkg/pack/'s parent — wrong root
// for a sub-package pin.
func scanCompilerPkg(t *testing.T) string {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read pkg/pack/compiler: %v", err)
	}
	var sb strings.Builder
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(".", name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		sb.Write(data)
		sb.WriteString("\n")
	}
	return sb.String()
}

// TestNAI200_PresencePin_DualMap asserts the NAI-200-D-DUAL-MAP tag
// appears ≥1 times in pkg/pack/compiler/ production code (the
// doc-comment on TypeInfo at typeinfo.go).
//
// The tag records the goscape decision to split TS's
// `map: Record<string, string>` (mixed numeric/string keys) into two
// statically-typed maps: `Map map[int]string` (from Add) and
// `NameMap map[string]string` (from LoadRecords/LoadMap).
//
// Per [[pin_test_self_trigger_production_doc]], this pin matches the
// tag identifier ONLY — not paraphrases like "dual map" — to avoid
// self-triggering against adjacent prose.
func TestNAI200_PresencePin_DualMap(t *testing.T) {
	src := scanCompilerPkg(t)
	const tag = "NAI-200-D-DUAL-MAP"
	count := strings.Count(src, tag)
	if count < 1 {
		t.Fatalf("%s should appear ≥1 times in pkg/pack/compiler/ production code (TypeInfo doc-comment); got %d", tag, count)
	}
}
