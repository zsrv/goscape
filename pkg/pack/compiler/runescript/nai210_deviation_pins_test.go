// pkg/pack/compiler/runescript/nai210_deviation_pins_test.go
package runescript

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestDeviationPinsLive_NAI210 grep-pins every NAI-210-introduced deviation
// tag to at least one production .go source. Mirrors the NAI-209 pin pattern.
func TestDeviationPinsLive_NAI210(t *testing.T) {
	tags := []struct{ Tag, Why string }{
		{"NAI-210-D-GZIP-OS-BYTE-ZEROED", "Go compress/gzip writes host OS byte; zeroed for TS-equivalent reproducibility"},
		{"NAI-210-D-LOADER-SORTED-ITERATION", "Go map iteration randomized; sort by id for byte-identical SymbolMapper"},
		{"NAI-210-D-EMPTYPOINTERS-RETURNS-FALSE", "TS-faithful early-return false on empty commandPointers"},
	}

	// Walk from this test's directory up to the repo root, then scan
	// pkg/ + modules/ + cmd/ for each tag in production .go files.
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
			if strings.HasSuffix(path, "nai210_deviation_pins_test.go") {
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
