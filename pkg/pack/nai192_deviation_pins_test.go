package pack

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// scanPackageDecls parses every non-test .go file in pkg/pack and
// returns all top-level identifier names declared as var/const/type/func.
func scanPackageDecls(t *testing.T) map[string]bool {
	t.Helper()
	names := map[string]bool{}
	// Resolve the package directory relative to this test file.
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(wd)
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, filepath.Join(wd, e.Name()), nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatal(err)
		}
		for _, decl := range f.Decls {
			switch d := decl.(type) {
			case *ast.GenDecl:
				for _, spec := range d.Specs {
					switch s := spec.(type) {
					case *ast.ValueSpec:
						for _, n := range s.Names {
							names[n.Name] = true
						}
					case *ast.TypeSpec:
						names[s.Name.Name] = true
					}
				}
			case *ast.FuncDecl:
				if d.Recv == nil {
					names[d.Name.Name] = true
				}
			}
		}
	}
	return names
}

// NAI-192-D-PACKFILE-SINGLETONS-DEFERRED: no module-level VarnPack /
// VarsPack *PackFile decls.
func TestNAI192_PackFileSingletonsDeferred_NoModuleLevelVarnPack(t *testing.T) {
	decls := scanPackageDecls(t)
	for _, name := range []string{"VarnPack", "VarsPack"} {
		if decls[name] {
			t.Errorf("found top-level decl %q in pkg/pack — violates NAI-192-D-PACKFILE-SINGLETONS-DEFERRED", name)
		}
	}
}

// NAI-192-D-VARP-UNIQUENESS-DEFERRED: retired by NAI-193 T4 — the
// checkVarNameUniqueness function now exists in pack_configs.go and is
// wired into PackConfigs in T5. The pin test is deleted here rather than
// T7 because the function definition (not yet called by the orchestrator)
// already trips the raw strings.Contains scan. Positive-retirement pin
// lands in nai193_deviation_pins_test.go (T7).

// NAI-192-D-DEADBRANCH-OMITTED: parseVarnConfig / parseVarsConfig
// source must NOT contain stringKeys / numberKeys / booleanKeys
// identifiers in non-comment code. (The empty TS branches are
// intentionally omitted; the deviation tag in doc-comments is allowed.)
func TestNAI192_DeadBranchOmitted_NoEmptyKeyArrays(t *testing.T) {
	for _, fn := range []string{"varn.go", "vars.go"} {
		body, err := os.ReadFile(fn)
		if err != nil {
			t.Fatal(err)
		}
		// Strip single-line comments before scanning so the deviation
		// tag itself (which names the banned identifiers) doesn't trigger.
		lines := strings.Split(string(body), "\n")
		var codeLines []string
		for _, l := range lines {
			trimmed := strings.TrimSpace(l)
			if strings.HasPrefix(trimmed, "//") {
				continue
			}
			codeLines = append(codeLines, l)
		}
		s := strings.Join(codeLines, "\n")
		for _, banned := range []string{"stringKeys", "numberKeys", "booleanKeys"} {
			if strings.Contains(s, banned) {
				t.Errorf("%s contains %q in non-comment code — violates NAI-192-D-DEADBRANCH-OMITTED", fn, banned)
			}
		}
	}
}

// NAI-192-D-PACKET-WRITE-CURSOR: PackedData.Next must use Dat.Length()
// for write-cursor arithmetic, NOT Dat.Pos. A regression to Dat.Pos
// would silently produce wrong idx offsets.
func TestNAI192_PacketWriteCursor_UsesLengthNotPos(t *testing.T) {
	body, err := os.ReadFile("packed_data.go")
	if err != nil {
		t.Fatal(err)
	}
	s := string(body)
	// Permissive: any read of "Pos" anywhere in Next() would be a bug.
	// Locate the Next() function body and inspect it.
	startMarker := "func (pd *PackedData) Next()"
	start := strings.Index(s, startMarker)
	if start < 0 {
		t.Fatal("Next() not found")
	}
	// Find the end of the function (first `}` at column 0 after start).
	end := strings.Index(s[start:], "\n}")
	if end < 0 {
		t.Fatal("Next() end not found")
	}
	body_ := s[start : start+end]
	if strings.Contains(body_, "Dat.Pos") || strings.Contains(body_, ".Pos") {
		t.Error("PackedData.Next() references Dat.Pos — violates NAI-192-D-PACKET-WRITE-CURSOR")
	}
	if !strings.Contains(body_, "Dat.Length()") {
		t.Error("PackedData.Next() must use Dat.Length() per NAI-192-D-PACKET-WRITE-CURSOR")
	}
}
