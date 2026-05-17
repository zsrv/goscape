package compiler

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestNAI202Deviations_Pinned pins NAI-202's deviation tags — each must
// have at least one in-source reference across the touched packages.
// Mirrors the NAI-201 pin-test shape at pkg/script/nai201_deviation_pins_test.go.
//
// Search roots:
//   - pkg/pack/compiler/   — host of the driver + parser
//   - pkg/script/          — PointerGroupFind hardening
func TestNAI202Deviations_Pinned(t *testing.T) {
	wantTags := []string{
		"NAI-202-D-CONSTANT-LOOSE-PARSER",
		"NAI-202-D-POINTER-GROUP-FIND-HARDENED",
	}

	roots := []string{".", "../../script"}

	counts := make(map[string]int, len(wantTags))
	for _, root := range roots {
		err := filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if d.IsDir() {
				return nil
			}
			if !strings.HasSuffix(path, ".go") {
				return nil
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			s := string(data)
			for _, tag := range wantTags {
				counts[tag] += strings.Count(s, tag)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walking %s: %v", root, err)
		}
	}

	for _, tag := range wantTags {
		if counts[tag] < 1 {
			t.Errorf("deviation tag %q: 0 references found; want >=1 (search roots: pkg/pack/compiler/, pkg/script/)", tag)
		}
	}
}
