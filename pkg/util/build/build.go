package build

import (
	"cmp"
	"fmt"
	"runtime"
)

// Version, Revision, Branch, BuildUser, and BuildDate are injected at link
// time via -ldflags -X; they stay empty in plain `go build` binaries.
var (
	Version   string
	Revision  string
	Branch    string
	BuildUser string
	BuildDate string
	GoVersion string
)

func init() {
	GoVersion = runtime.Version()
}

// String renders the ldflags-injected build metadata for --version and
// the startup log. Empty fields (local `go run`) render as "unknown".
func String() string {
	v := cmp.Or(Version, "unknown")
	r := cmp.Or(Revision, "unknown")
	b := cmp.Or(Branch, "unknown")
	return fmt.Sprintf("goscape %s (revision %s, branch %s, %s)", v, r, b, GoVersion)
}
