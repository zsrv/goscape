package pack

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestNAI198_PresencePin_NoSrcNoOpScopeExtended re-asserts that the
// NAI-192-D-NO-SRC-NO-OP doc-comment in pkg/pack/pack_configs.go now
// enumerates the nine server-only freshness-gated branches (six
// existing + .dbtable + .dbrow + .hunt added in NAI-198).
//
// Guards against:
//
//	(a) accidental retirement of the tag identifier;
//	(b) regression of the count phrasing back to "six";
//	(c) doc-comment refactors that drop one or more of the nine configs.
func TestNAI198_PresencePin_NoSrcNoOpScopeExtended(t *testing.T) {
	src := scanPkgPack(t)
	// Match the tag-heading line (with "// " prefix and ":" suffix) to
	// skip cross-reference mentions of the tag in neighbouring paragraphs.
	const tagHeading = "// NAI-192-D-NO-SRC-NO-OP:"
	if !strings.Contains(src, tagHeading) {
		t.Fatal("NAI-192-D-NO-SRC-NO-OP tag should be present in pkg/pack production code")
	}
	idx := strings.Index(src, tagHeading)
	window := src[idx:]
	end := strings.Index(window, "\n//\n")
	if end == -1 || end > 2000 {
		end = 2000
	}
	if end > len(window) {
		end = len(window)
	}
	block := window[:end]

	if !strings.Contains(block, "nine") {
		t.Errorf("NAI-192-D-NO-SRC-NO-OP scope phrasing should say 'nine' branches; block:\n%s", block)
	}
	for _, cfg := range []string{".enum", ".inv", ".mesanim", ".struct", ".dbtable", ".dbrow", ".hunt", ".varn", ".vars"} {
		if !strings.Contains(block, cfg) {
			t.Errorf("NAI-192-D-NO-SRC-NO-OP scope is missing config %q; block:\n%s", cfg, block)
		}
	}
}

// TestNAI198_PresencePin_HuntOpObj2TsBugTagged asserts the
// NAI-198-D-HUNT-OPOBJ2-TS-BUG deviation tag appears in pkg/pack/hunt.go
// as a comment. The tag flags goscape's literal port of the TS typo
// at HuntConfig.ts:201-202 ('opobj2' string maps to NpcMode.OPOBJ1,
// not OPOBJ2). Per [[pin_test_self_trigger_production_doc]], the pin
// matches against the tag identifier ONLY — not the bare string
// 'opobj2', which appears in the production case statement.
func TestNAI198_PresencePin_HuntOpObj2TsBugTagged(t *testing.T) {
	huntPath := filepath.Join("hunt.go")
	bytes, err := os.ReadFile(huntPath)
	if err != nil {
		t.Fatalf("read hunt.go: %v", err)
	}
	src := string(bytes)
	if !strings.Contains(src, "NAI-198-D-HUNT-OPOBJ2-TS-BUG") {
		t.Fatal("expected NAI-198-D-HUNT-OPOBJ2-TS-BUG tag-comment in pkg/pack/hunt.go")
	}
}

// TestNAI198_AbsencePin_UnconditionalClientCohortDoesNotGrow asserts
// the NAI-196-D-UNCONDITIONAL-CLIENT-PACK doc-comment scope does NOT
// list .hunt, .dbtable, or .dbrow. The three NAI-198 configs are
// server-only-freshness-gated, never unconditional-client. Guards
// against accidental scope expansion.
func TestNAI198_AbsencePin_UnconditionalClientCohortDoesNotGrow(t *testing.T) {
	src := scanPkgPack(t)
	idx := strings.Index(src, "NAI-196-D-UNCONDITIONAL-CLIENT-PACK")
	if idx == -1 {
		t.Fatal("NAI-196-D-UNCONDITIONAL-CLIENT-PACK tag not present in pkg/pack")
	}
	window := src[idx:]
	end := strings.Index(window, "\n//\n")
	if end == -1 || end > 2000 {
		end = 2000
	}
	if end > len(window) {
		end = len(window)
	}
	block := window[:end]
	for _, forbidden := range []string{".hunt", ".dbtable", ".dbrow"} {
		if strings.Contains(block, forbidden) {
			t.Errorf("NAI-196-D-UNCONDITIONAL-CLIENT-PACK scope MUST NOT contain %q (server-only config); block:\n%s",
				forbidden, block)
		}
	}
}
