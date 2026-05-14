package pack

import (
	"strings"
	"testing"
)

// TestNAI197_PresencePin_UnconditionalClientPackExtended re-asserts that
// the NAI-196-D-UNCONDITIONAL-CLIENT-PACK doc-comment lives in pkg/pack
// production code AND that its scope enumeration now includes the four
// configs ported in NAI-197.
//
// Guards against:
//
//	(a) accidental retirement of the tag identifier (the tag still applies);
//	(b) doc-comment refactors that drop one or more of the four NAI-197
//	    configs from the scope list.
func TestNAI197_PresencePin_UnconditionalClientPackExtended(t *testing.T) {
	src := scanPkgPack(t)
	if !strings.Contains(src, "NAI-196-D-UNCONDITIONAL-CLIENT-PACK") {
		t.Fatal("NAI-196-D-UNCONDITIONAL-CLIENT-PACK tag should be documented in pkg/pack production code but is absent")
	}
	idx := strings.Index(src, "NAI-196-D-UNCONDITIONAL-CLIENT-PACK")
	if idx == -1 {
		t.Fatal("tag identifier not found")
	}
	window := src[idx:]
	end := strings.Index(window, "\n//\n") // paragraph boundary in Go doc-comments
	if end == -1 || end > 2000 {
		end = 2000
	}
	if end > len(window) {
		end = len(window)
	}
	block := window[:end]

	for _, cfg := range []string{".seq", ".flo", ".spotanim", ".idk"} {
		if !strings.Contains(block, cfg) {
			t.Errorf("NAI-196-D-UNCONDITIONAL-CLIENT-PACK doc-comment scope is missing config %q (got block:\n%s)", cfg, block)
		}
	}
}
