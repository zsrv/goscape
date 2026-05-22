package script

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestNAI201Deviations_Pinned pins spec §7.12: each NAI-201 deviation
// tag has at least one in-source reference. Tracks the
// "grep ALL deviation-tag references when retiring" convention — if a
// tag is retired, the test fails and the implementer reviews scope
// before updating the wantTags map.
//
// Greps both pkg/script/ (Pointers / ScriptOpcodeMap-side deviations)
// and pkg/objtype/ (NpcMode-side deviation). Implementer adjusts the
// grep root if a new deviation lands in a different package.
//
// Status: NAI-201-D-NPCMODE-QUEUE-TODO is FORMALLY CLOSED as a
// TS-parity exception (Arc 23 #176 pattern). See the long-form
// closure note at pkg/objtype/npcmode.go above the NpcModeMap var.
// The QUEUE constants, dispatch (TriggerAiQueue1..20 via
// Server.consumeHuntTarget), and pack-parser support all ship in Go;
// only the string→mode entry is omitted from NpcModeMap to mirror
// TS NpcMode.ts:147-167's commented-out TODO block. Do not re-open
// this deviation unless TS uncomments those entries upstream.
func TestNAI201Deviations_Pinned(t *testing.T) {
	wantTags := []string{
		"NAI-201-D-NPCMODE-QUEUE-TODO",
		"NAI-201-D-POINTERS-SPREAD-HELPER",
	}

	roots := []string{".", "../objtype"}

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
			t.Errorf("deviation tag %q: 0 references found; want >=1 (search roots: pkg/script/, pkg/objtype/)", tag)
		}
	}
}
