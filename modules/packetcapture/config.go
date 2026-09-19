package packetcapture

import (
	"flag"

	pkgcapture "github.com/zsrv/goscape/pkg/packetcapture"
)

// Config re-exports pkg/packetcapture.Config under the module package so the
// dskit modules.go registration matches the convention used by other modules.
type Config = pkgcapture.Config

func RegisterFlagsAndApplyDefaults(cfg *Config, f *flag.FlagSet) {
	cfg.RegisterFlagsAndApplyDefaults(f)
}
