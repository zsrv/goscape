package encfilter

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

type fixturePair struct {
	Input    string `json:"input"`
	Filtered string `json:"filtered"`
}

// TestFilter_AgainstTSFixtures loads the JSON file produced by
// tools/wordenc/gen-fixtures.ts, loads the real client/wordenc jagfile from
// the canonical Engine-TS data path, runs goscape's Filter.Filter on each
// input, and asserts byte-identical match against the TS output.
//
// Skips if either the jagfile or the fixtures JSON is absent (matches the
// real-cache test convention; see modules/world/loctype_realcache_test.go).
func TestFilter_AgainstTSFixtures(t *testing.T) {
	ref := os.Getenv("GOSCAPE_REF225_DIR")
	if ref == "" {
		t.Skip("GOSCAPE_REF225_DIR not set; skipping (needs the reference engine's data/pack/client/wordenc)")
	}
	tsCache := filepath.Join(ref, "data", "pack")
	jagPath := filepath.Join(tsCache, "client", "wordenc")
	if _, err := os.Stat(jagPath); err != nil {
		t.Skipf("wordenc jagfile not present at %s; skipping (ensure data/pack/client/wordenc exists)", jagPath)
	}

	fixturesPath := filepath.Join("testdata", "wordenc-fixtures.json")
	raw, err := os.ReadFile(fixturesPath)
	if err != nil {
		t.Skipf("fixtures file %s not present; skipping (regenerate via tools/wordenc/gen-fixtures.ts)", fixturesPath)
	}
	var pairs []fixturePair
	if err := json.Unmarshal(raw, &pairs); err != nil {
		t.Fatalf("parse fixtures: %v", err)
	}
	if len(pairs) == 0 {
		t.Skip("fixtures file empty; regenerate via tools/wordenc/gen-fixtures.ts")
	}

	f, err := Load(tsCache)
	if err != nil {
		t.Fatalf("Load(%q): %v", tsCache, err)
	}

	for _, p := range pairs {
		t.Run(p.Input, func(t *testing.T) {
			got := f.Filter(p.Input)
			if got != p.Filtered {
				t.Errorf("Filter(%q):\n  got  %q\n  want %q", p.Input, got, p.Filtered)
			}
		})
	}
}
