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
// NAI-191-D-VALIDATE-FLAGS: the config-name presence check is wired
// via validatePackNamesAgainstCfgs in pack_configs.go (rev-244 B6).
// TS BUILD_VERIFY_FOLDER directory-structure enforcement remains
// out-of-scope (goscape does not model BUILD_VERIFY_FOLDER).
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

// CrawlConfigCategories collects category names from the `category=` lines of
// .loc, .npc and .obj sources, in that extension order, first-seen wins.
//
// Categories are the one config family with no source files of their own —
// Content ships no .category files — so pack/category.pack cannot be derived
// from a header crawl the way every other index is. Content gitignores it, so
// a fresh clone has none and every `category=` reference fails to resolve
// until it is rebuilt from these references.
//
// TS source: tools/pack/PackFile.ts:crawlConfigCategories.
func CrawlConfigCategories(srcDir string) ([]string, error) {
	scripts := filepath.Join(srcDir, "scripts")
	var names []string
	for _, ext := range []string{".loc", ".npc", ".obj"} {
		err := LoadDirExtFull(scripts, ext, func(lines []string, file string) {
			for _, line := range lines {
				after, ok := strings.CutPrefix(line, "category=")
				if !ok {
					continue
				}
				if !slices.Contains(names, after) {
					names = append(names, after)
				}
			}
		})
		if err != nil {
			return nil, err
		}
	}
	return names, nil
}
