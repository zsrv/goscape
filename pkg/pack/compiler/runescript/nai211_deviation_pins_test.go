// pkg/pack/compiler/runescript/nai211_deviation_pins_test.go
package runescript

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestDeviationPinsLive_NAI211 grep-pins every NAI-211-introduced
// deviation tag to at least one production .go source. Mirrors the
// NAI-210 pin pattern.
func TestDeviationPinsLive_NAI211(t *testing.T) {
	tags := []struct{ Tag, Why string }{
		{"NAI-211-D-NO-PROCESS-EXIT", "BaseDiagnosticsHandler is print-only; Run() returns error instead of process.exit(1)"},
		{"NAI-211-D-MACRO-LOOKUP-DEFERRED", "TS BaseDiagnosticsHandler.macroLookup omitted; macros not yet ported"},
		{"NAI-211-D-PHASE-DIAGNOSTICS-FRESH", "Each phase allocates its own *Diagnostics; pre-NAI-211 shared accumulator retired"},
	}

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	root := cwd
	for {
		if _, err := os.Stat(filepath.Join(root, "go.mod")); err == nil {
			break
		}
		parent := filepath.Dir(root)
		if parent == root {
			t.Fatalf("go.mod not found walking up from %s", cwd)
		}
		root = parent
	}

	scanDirs := []string{"pkg", "modules", "cmd"}
	hits := map[string][]string{}
	for _, dir := range scanDirs {
		base := filepath.Join(root, dir)
		_ = filepath.WalkDir(base, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") {
				return nil
			}
			if strings.HasSuffix(path, "nai211_deviation_pins_test.go") {
				return nil // skip self
			}
			b, rerr := os.ReadFile(path)
			if rerr != nil {
				return nil
			}
			content := string(b)
			for _, tag := range tags {
				if strings.Contains(content, tag.Tag) {
					hits[tag.Tag] = append(hits[tag.Tag], path)
				}
			}
			return nil
		})
	}

	for _, tag := range tags {
		t.Run(tag.Tag, func(t *testing.T) {
			files := hits[tag.Tag]
			productionHit := false
			for _, f := range files {
				if !strings.HasSuffix(f, "_test.go") {
					productionHit = true
					break
				}
			}
			if !productionHit {
				t.Errorf("tag %s has no production touch point (%s); hits=%v", tag.Tag, tag.Why, files)
			}
		})
	}
}
