package pack

import (
	"path/filepath"
	"slices"
	"strings"
)

// CrawlConfigNames walks <srcDir>/scripts recursively, parses every
// file with the given extension, and returns the unique set of
// [header] tokens found at the start of any line.
//
// If includeBrackets is true, returned names include the surrounding
// square brackets ("[name]"); otherwise just "name".
//
// The single TS-specific exclusion is <srcDir>/scripts/engine.rs2 —
// this file holds compiler type signatures for built-in commands and
// must NOT be treated as a packable config.
//
// NAI-191-D-VALIDATE-FLAGS-DEFERRED: TS BUILD_VERIFY_FOLDER also
// enforces directory-structure rules (configs must live under
// configs/, scripts under scripts/). This validator-side check defers
// to NAI-192+ alongside the env-flag plumbing.
//
// TS source: tools/pack/PackFile.ts:crawlConfigNames.
func CrawlConfigNames(srcDir, ext string, includeBrackets bool) ([]string, error) {
	enginePath := filepath.Join(srcDir, "scripts", "engine.rs2")
	var names []string
	err := LoadDirExtFull(filepath.Join(srcDir, "scripts"), ext, func(lines []string, file string) {
		if file == enginePath {
			return
		}
		for _, line := range lines {
			if !strings.HasPrefix(line, "[") {
				continue
			}
			end := strings.Index(line, "]")
			if end < 0 {
				continue
			}
			name := line[:end+1]
			if !includeBrackets {
				name = name[1 : len(name)-1]
			}
			if !slices.Contains(names, name) {
				names = append(names, name)
			}
		}
	})
	if err != nil {
		return nil, err
	}
	return names, nil
}
