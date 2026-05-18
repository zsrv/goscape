package worldmap

import (
	"errors"

	packet2 "github.com/zsrv/goscape/pkg/io/packet"
	"github.com/zsrv/goscape/pkg/objtype"
)

// Pack is the worldmap packer entry point. Implementation lands in
// the Pack-entry-point task; this stub exists so callers can compile
// against the public surface while the per-map loop is being ported.
func Pack(srcDir, outDir string) error {
	_ = srcDir
	_ = outDir
	return errors.New("worldmap.Pack: not implemented")
}

// packWater appends one "ocean" map square (mx, mz) to underlay
// and overlay. Mirrors TS Worldmap.ts:15-28.
//
// underlay grows by 2 + 4096 = 4098 bytes.
// overlay  grows by 2 + 4096*2 = 8194 bytes.
func packWater(flo *objtype.FloTypeConfigs, underlay, overlay *packet2.Packet, mx, mz int) {
	muddyId := uint8(1 + flo.GetId("muddygrass"))
	waterId := uint8(1 + flo.GetId("water"))

	underlay.P1(uint8(mx))
	underlay.P1(uint8(mz))
	overlay.P1(uint8(mx))
	overlay.P1(uint8(mz))

	for range 4096 {
		underlay.P1(muddyId)
		overlay.P1(waterId)
		overlay.P1(0)
	}
}

// unpackCoord extracts (level, x, z) from a packed local-coord
// int. x and z are LOCAL mapsquare coords (0..63). Mirrors TS
// Worldmap.ts:53-58.
func unpackCoord(packed int) (level, x, z int) {
	z = packed & 0x3f
	x = (packed >> 6) & 0x3f
	level = (packed >> 12) & 0x3
	return
}
