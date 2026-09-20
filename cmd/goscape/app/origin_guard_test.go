package app

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// eventspbPath is the import path whose envelope literals this guard covers.
const eventspbPath = "github.com/zsrv/goscape/pkg/eventspb"

// originRequired maps each envelope type to the origin fields every literal
// of it must set. All five carry both: revision says which game revision
// produced the event (one world id can be served by several at once) and
// profile says which deployment did (account ids are global across profiles).
// ReplayEnvelope is no exception — its revision simply sits on an older tag.
var originRequired = map[string][]string{
	"AuthEnvelope":        {"Revision", "Profile"},
	"PlayerInputEnvelope": {"Revision", "Profile"},
	"WealthEnvelope":      {"Revision", "Profile"},
	"WorldEnvelope":       {"Revision", "Profile"},
	"ReplayEnvelope":      {"Revision", "Profile"},
}

var generatedHeader = regexp.MustCompile(`^// Code generated .* DO NOT EDIT\.$`)

// moduleRoot walks up from the test's working directory to the go.mod.
func moduleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found above the test's working directory")
		}
		dir = parent
	}
}

// TestEveryEventEnvelopeLiteralStampsItsOrigin walks every non-test,
// non-generated Go file in the module that imports pkg/eventspb and fails if
// any envelope composite literal omits an origin field. A new emit site that
// forgets them is a test failure here, not a silent hole in the data: the
// fields cannot be stamped centrally, because pkg/telemetry and
// modules/telemetry are revision-neutral and must not import
// pkg/io/protocol/revision or any module.
func TestEveryEventEnvelopeLiteralStampsItsOrigin(t *testing.T) {
	root := moduleRoot(t)
	fset := token.NewFileSet()

	var problems []string
	var checked int

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "vendor", "testdata", "node_modules":
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		// Parse imports first: only files that import pkg/eventspb can hold
		// an envelope literal, and that keeps this walk cheap.
		head, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly|parser.ParseComments)
		if err != nil {
			return err
		}
		if len(head.Comments) > 0 {
			for _, c := range head.Comments[0].List {
				if generatedHeader.MatchString(c.Text) {
					return nil
				}
			}
		}
		pkgName := ""
		for _, imp := range head.Imports {
			p, err := strconv.Unquote(imp.Path.Value)
			if err != nil || p != eventspbPath {
				continue
			}
			pkgName = "eventspb"
			if imp.Name != nil {
				pkgName = imp.Name.Name
			}
		}
		if pkgName == "" || pkgName == "_" {
			return nil
		}

		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return err
		}
		ast.Inspect(f, func(n ast.Node) bool {
			lit, ok := n.(*ast.CompositeLit)
			if !ok {
				return true
			}
			sel, ok := lit.Type.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			ident, ok := sel.X.(*ast.Ident)
			if !ok || ident.Name != pkgName {
				return true
			}
			want, ok := originRequired[sel.Sel.Name]
			if !ok {
				return true
			}
			checked++

			var set []string
			for _, elt := range lit.Elts {
				kv, ok := elt.(*ast.KeyValueExpr)
				if !ok {
					continue
				}
				if key, ok := kv.Key.(*ast.Ident); ok {
					set = append(set, key.Name)
				}
			}
			rel, relErr := filepath.Rel(root, path)
			if relErr != nil {
				rel = path
			}
			for _, field := range want {
				if !slices.Contains(set, field) {
					problems = append(problems, rel+":"+
						strconv.Itoa(fset.Position(lit.Pos()).Line)+
						": eventspb."+sel.Sel.Name+" literal does not set "+field)
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}

	if checked == 0 {
		t.Fatal("no envelope literals found — the guard is not looking where the emit sites are")
	}
	for _, p := range problems {
		t.Errorf("%s", p)
	}
	if len(problems) > 0 {
		t.Logf("every eventspb envelope literal must stamp Revision (revision.Expected) "+
			"and Profile (the emitting module's configured profile); %d literals checked", checked)
	}
}

// TestPacketCaptureConfigBorrowsWorldProfile pins the one origin value that
// is not read at its emit site: pkg/packetcapture builds its envelopes from
// CaptureOpts, so the profile has to arrive there the same borrowed way the
// world id already does — from world.node_profile, with no user-facing key of
// its own.
func TestPacketCaptureConfigBorrowsWorldProfile(t *testing.T) {
	c := &Config{}
	c.World.NodeID = 7
	c.World.NodeProfile = "beta"

	got := c.packetCaptureConfig()
	if got.WorldID != 7 {
		t.Errorf("WorldID = %d, want 7", got.WorldID)
	}
	if got.Profile != "beta" {
		t.Errorf("Profile = %q, want %q (world.node_profile)", got.Profile, "beta")
	}
}
