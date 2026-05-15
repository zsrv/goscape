package lexer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestNAI203Deviations_Pinned pins NAI-203's deviation tags — each
// must have at least one in-source reference. Mirrors the NAI-202 pin-
// test shape at pkg/pack/compiler/nai202_deviation_pins_test.go.
//
// Search root: this package (pkg/pack/compiler/lexer).
func TestNAI203Deviations_Pinned(t *testing.T) {
	wantTags := []string{
		"NAI-203-D-LEXER-ERROR-RECOVERY",
	}

	counts := make(map[string]int, len(wantTags))
	err := filepath.WalkDir(".", func(path string, d os.DirEntry, walkErr error) error {
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
		t.Fatalf("walk: %v", err)
	}

	for _, tag := range wantTags {
		if counts[tag] == 0 {
			t.Errorf("deviation tag %q: 0 references in package, want ≥1", tag)
		}
	}
}
