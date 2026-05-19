package build

import (
	"runtime"
)

var (
	Version   string
	Revision  string
	Branch    string
	BuildUser string
	BuildDate string
	GoVersion string
)

func init() {
	Version = Version
	Revision = Revision
	Branch = Branch
	BuildUser = BuildUser
	BuildDate = BuildDate
	GoVersion = runtime.Version()
}
