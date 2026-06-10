package build

import (
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
