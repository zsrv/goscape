package pack

import (
	"fmt"
	"os"
	"strings"
)

// LoadDirCallback is the per-file callback for LoadDir / LoadDirFull /
// LoadDirExt / LoadDirExtFull.
type LoadDirCallback func(lines []string, file string)

// LoadFile returns the file's lines with all \r stripped (split on \n),
// or nil if the path is missing.
//
// TS source: Parse.ts:17-23 (9aadcec4). Routes through readTextNormalize
// so mid-line \r bytes are stripped before splitting.
func LoadFile(path string) []string {
	if !FileExists(path) {
		return nil
	}
	return strings.Split(readTextNormalize(path), "\n")
}

// LoadFileFull returns LoadFile's output with single-line ("//") and
// multi-line ("/* */") comments stripped using TS Parse.ts:loadFileFull
// counter semantics. Returns an error if a /* is never closed.
//
// TS-parity quirk pinned in tests: the outer block (counter==0) strips
// ONLY the first /* */ pair on a line; trailing */ tokens survive into
// the output. See spec §3.6.
func LoadFileFull(path string) ([]string, error) {
	text := LoadFile(path)
	lines := make([]string, 0, len(text))
	multiCommentStart := 0
	multiLineComments := 0

	for i := range text {
		line := strings.TrimSpace(text[i])

		if multiLineComments > 0 {
			// Already inside multi-line comment: walk all /* and */ on
			// this line, incrementing / decrementing the counter.
			for {
				idx := strings.Index(line, "/*")
				if idx == -1 {
					break
				}
				line = strings.TrimLeft(line[idx+2:], " \t")
				multiLineComments++
			}
			for multiLineComments > 0 {
				idx := strings.Index(line, "*/")
				if idx == -1 {
					break
				}
				line = strings.TrimLeft(line[idx+2:], " \t")
				multiLineComments--
			}
			if multiLineComments > 0 {
				continue
			}
		}

		if len(line) == 0 {
			continue
		}

		// Single-line // comment.
		if c := strings.Index(line, "//"); c != -1 {
			line = strings.TrimRight(line[:c], " \t")
			if len(line) == 0 {
				continue
			}
		}

		// Multi-line /* */ first-entry handling.
		commentStart := strings.Index(line, "/*")
		commentEnd := strings.Index(line, "*/")
		if commentStart != -1 {
			if commentEnd != -1 {
				line = line[:commentStart] + line[commentEnd+2:]
			} else {
				line = line[:commentStart]
				multiLineComments++
				if multiCommentStart == 0 {
					multiCommentStart = i + 1
				}
			}
			if len(line) == 0 {
				continue
			}
		}

		lines = append(lines, line)
	}

	if multiLineComments > 0 {
		return nil, fmt.Errorf("%s has an unclosed multi-line comment starting at line %d", path, multiCommentStart)
	}
	return lines, nil
}

// LoadDir invokes cb(lines, basename) for every file under path
// (recursive). Subdirectory entries (suffixed "/") are skipped.
func LoadDir(path string, cb LoadDirCallback) {
	for _, f := range ListFiles(path) {
		if strings.HasSuffix(f, "/") {
			continue
		}
		base := f[strings.LastIndex(f, "/")+1:]
		cb(LoadFile(f), base)
	}
}

// LoadDirFull is LoadDir but with comment stripping. Returns the first
// LoadFileFull error and halts the walk (TS throws synchronously).
// NAI-191-D-LOADFILEFULL-ERRORS-PROPAGATE: TS throws from a callback;
// Go returns the error and halts the walk. Same observable outcome.
func LoadDirFull(path string, cb LoadDirCallback) error {
	for _, f := range ListFiles(path) {
		if strings.HasSuffix(f, "/") {
			continue
		}
		lines, err := LoadFileFull(f)
		if err != nil {
			return err
		}
		base := f[strings.LastIndex(f, "/")+1:]
		cb(lines, base)
	}
	return nil
}

// ListFilesExt returns all files (recursive) under path with the given
// extension. Returns nil for missing paths.
func ListFilesExt(path, ext string) []string {
	if !FileExists(path) {
		return nil
	}
	all := ListDir(path)
	out := make([]string, 0, len(all))
	for _, f := range all {
		if strings.HasSuffix(f, ext) {
			out = append(out, f)
		}
	}
	return out
}

// LoadDirExt is LoadDir filtered by extension. The callback receives
// the FULL path as the file argument (TS parity for this overload).
func LoadDirExt(path, ext string, cb LoadDirCallback) {
	for _, f := range ListFilesExt(path, ext) {
		cb(LoadFile(f), f)
	}
}

// LoadDirExtFull is LoadDirExt with comment stripping. Halts on first
// LoadFileFull error.
func LoadDirExtFull(path, ext string, cb LoadDirCallback) error {
	for _, f := range ListFilesExt(path, ext) {
		lines, err := LoadFileFull(f)
		if err != nil {
			return err
		}
		cb(lines, f)
	}
	return nil
}

// LoadFileConfigLines returns the config-relevant lines of path using the TS
// readConfigs line contract, NOT the loadFileFull one: lines are taken
// verbatim off the reader with no trimming and no comment stripping, and only
// fully-empty lines and lines whose first characters are "//" are skipped.
//
// TS PackShared.readConfigs (PackShared.ts:217-260 @1d25566c) drives config
// parsing straight from readline.createInterface — it never calls loadFileFull.
// That distinction is observable: a config value may legitimately end in
// whitespace, and loadFileFull's per-line .trim() would silently eat it.
//
// Scripts still go through LoadFileFull; only config families use this.
func LoadFileConfigLines(path string) []string {
	raw := LoadFile(path)
	lines := make([]string, 0, len(raw))
	for _, line := range raw {
		if len(line) == 0 || strings.HasPrefix(line, "//") {
			continue
		}
		lines = append(lines, line)
	}
	return lines
}

// LoadDirExtConfig is LoadDirExt over LoadFileConfigLines — the config-family
// counterpart of LoadDirExtFull.
func LoadDirExtConfig(path, ext string, cb LoadDirCallback) {
	for _, f := range ListFilesExt(path, ext) {
		cb(LoadFileConfigLines(f), f)
	}
}

// ReadConfigs walks <srcDir>/scripts/*.<ext>, splits each file into
// [header]-delimited config blocks, and returns the aggregated
// map[header] = lines. Returns an error on duplicate header keys or on
// any unclosed multi-line comment.
func ReadConfigs(srcDir, ext string) (map[string][]string, error) {
	configs := map[string][]string{}
	var outerErr error

	err := LoadDirExtFull(srcDir+"/scripts", ext, func(lines []string, file string) {
		if outerErr != nil {
			return
		}
		current := ""
		var block []string
		for _, line := range lines {
			if strings.HasPrefix(line, "[") {
				if current != "" {
					if _, dup := configs[current]; dup {
						outerErr = fmt.Errorf("duplicate config found in %s: %s", file, current)
						return
					}
					configs[current] = block
				}
				current = line[1 : len(line)-1]
				block = nil
				continue
			}
			block = append(block, line)
		}
		if current != "" {
			if _, dup := configs[current]; dup {
				outerErr = fmt.Errorf("duplicate config found in %s: %s", file, current)
				return
			}
			configs[current] = block
		}
	})
	if err != nil {
		return nil, err
	}
	if outerErr != nil {
		return nil, outerErr
	}
	return configs, nil
}

// readTextNormalize returns the file content with ALL \r stripped, or
// "" if the file does not exist.
//
// TS source: Parse.ts:6-12 (9aadcec4). Behavioural delta vs the 225
// contract: a stray \r NOT followed by \n (mid-line or trailing on
// the last line) is stripped rather than preserved in the line content.
func readTextNormalize(path string) string {
	if !FileExists(path) {
		return ""
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.ReplaceAll(string(data), "\r", "")
}

func splitLinesCRLF(s string) []string {
	// TS 244: .replace(/\r/g, '').split('\n') — strip ALL \r first,
	// then split on \n.  The previous contract (split /\r?\n/ via
	// TrimSuffix) only handled \r\n pairs; a mid-line \r was
	// preserved in the line content.
	//
	// TS source: Parse.ts:6-12 + NameMap.ts:11-12,26-27,44-45,59
	// (9aadcec4).
	return strings.Split(strings.ReplaceAll(s, "\r", ""), "\n")
}
