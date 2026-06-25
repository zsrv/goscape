package log

import (
	"fmt"
	"log/slog"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

// SourceFormat controls how the slog `source` attribute is rendered. It is a
// config-only type so YAML and flags accept the names "relative", "short", and
// "full"; logger construction reads it via WithSourceFormat.
type SourceFormat int

const (
	// SourceRelative renders the path relative to the module root, e.g.
	// "modules/world/npc_masks.go:42". This is the default: it stays short
	// while remaining clickable from the repository root in an IDE. It is the
	// zero value so an unconfigured logger uses it.
	SourceRelative SourceFormat = iota
	// SourceShort renders only the base filename, e.g. "npc_masks.go:42".
	SourceShort
	// SourceFull renders the unmodified path the compiler embedded: an absolute
	// path for a plain `go run`/`go test`, or an import-path-rooted path under
	// -trimpath (e.g. "github.com/zsrv/goscape/modules/world/npc_masks.go:42").
	SourceFull
)

// UnmarshalText parses "relative" (or empty), "short", and "full"
// case-insensitively. Any other value is an error, surfaced as a fatal config
// error by strict YAML decoding and by flag parsing.
func (s *SourceFormat) UnmarshalText(b []byte) error {
	switch strings.ToLower(strings.TrimSpace(string(b))) {
	case "relative", "":
		*s = SourceRelative
	case "short":
		*s = SourceShort
	case "full":
		*s = SourceFull
	default:
		return fmt.Errorf("invalid log source format %q: valid values are [relative, short, full]", b)
	}
	return nil
}

// MarshalText renders the format name; it round-trips with UnmarshalText.
func (s SourceFormat) MarshalText() ([]byte, error) {
	return []byte(s.String()), nil
}

// String renders the format name ("relative" for unknown values).
func (s SourceFormat) String() string {
	switch s {
	case SourceShort:
		return "short"
	case SourceFull:
		return "full"
	default:
		return "relative"
	}
}

// moduleRoot is the compile-path prefix up to (and including) the module root,
// derived once from this file's own runtime path. Stripping it from a source
// file yields a module-root-relative path. It is computed from this file rather
// than hard-coded so it tracks the module path, and it works for both plain
// `go run`/`go test` (absolute paths) and -trimpath builds (import-path-rooted
// paths), since both share the ".../pkg/util/log/source.go" suffix.
var moduleRoot = func() string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return ""
	}
	const self = "pkg/util/log/source.go"
	if i := strings.LastIndex(file, self); i >= 0 {
		return file[:i]
	}
	return ""
}()

// renderSource formats a slog.Source as "path:line" according to format.
func renderSource(src *slog.Source, format SourceFormat) string {
	line := ":" + strconv.Itoa(src.Line)
	switch format {
	case SourceShort:
		return filepath.Base(src.File) + line
	case SourceFull:
		return src.File + line
	default: // SourceRelative
		if moduleRoot != "" {
			if rel, ok := strings.CutPrefix(src.File, moduleRoot); ok {
				return rel + line
			}
		}
		// Prefix not found (e.g. stdlib or a vendored path): fall back to the
		// base name rather than leaking an unrelated absolute path.
		return filepath.Base(src.File) + line
	}
}
