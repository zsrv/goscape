package pack

import (
	"strings"
	"testing"
)

// TestNAI199_PresencePin_TSCodeStalenessGate asserts that the
// NAI-199-D-TS-CODE-STALENESS-GATE deviation tag appears in pkg/pack/
// production code at ≥2 sites: doc-comments on the category branch
// and the frame_del branch in pack_configs.go (plus the documentation
// on both packAndSave functions in pack_specials.go, for a total of
// ≥4 matches).
//
// The tag records the goscape decision to drop TS's second-arm
// shouldBuild('tools/pack/config', '.ts', dest) gate, which rebuilds
// when TS pipeline source files are newer than output — a semantic
// with no Go-binary equivalent at runtime.
//
// Per [[pin_test_self_trigger_production_doc]], this pin matches the
// tag identifier ONLY — not paraphrases like "TS source staleness" —
// to avoid self-triggering against adjacent prose.
func TestNAI199_PresencePin_TSCodeStalenessGate(t *testing.T) {
	src := scanPkgPack(t)
	const tag = "NAI-199-D-TS-CODE-STALENESS-GATE"
	count := strings.Count(src, tag)
	if count < 2 {
		t.Fatalf("NAI-199-D-TS-CODE-STALENESS-GATE should appear ≥2 times in pkg/pack production code (category branch + frame_del branch); got %d", count)
	}
}
