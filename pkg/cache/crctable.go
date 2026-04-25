package cache

import (
	"log/slog"
	"os"

	"github.com/zsrv/goscape/pkg/io/packet"
)

var (
	CrcBuffer   = packet.NewPacket(make([]byte, 0, 4*9))
	CrcTable    []uint32
	CrcBuffer32 uint32
)

// ResetCRCState restores CrcBuffer, CrcTable, and CrcBuffer32 to their
// package-init shape. Test-only convenience to avoid drift between
// init expressions and inline test resets. Mirrors the var declarations
// at the top of this file.
func ResetCRCState() {
	CrcBuffer = packet.NewPacket(make([]byte, 0, 4*9))
	CrcTable = nil
	CrcBuffer32 = 0
}

func makeCrc(path string) {
	if _, err := os.Stat(path); err != nil {
		slog.Default().Warn("cache: makeCrc Stat failed",
			"path", path, "err", err)
		return
	}

	p, err := packet.Load(path, false)
	if err != nil {
		slog.Default().Warn("cache: makeCrc Load failed",
			"path", path, "err", err)
		return
	}

	crc := packet.GetCRC(p.Bytes(), 0, len(p.Bytes()))
	CrcTable = append(CrcTable, crc)
	CrcBuffer.P4(crc)
}

func MakeCRCs() {
	CrcTable = make([]uint32, 0)

	CrcBuffer.Pos = 0
	CrcBuffer.P4(0)
	CrcTable = append(CrcTable, 0)
	makeCrc("data/pack/client/title")
	makeCrc("data/pack/client/config")
	makeCrc("data/pack/client/interface")
	makeCrc("data/pack/client/media")
	makeCrc("data/pack/client/models")
	makeCrc("data/pack/client/textures")
	makeCrc("data/pack/client/wordenc")
	makeCrc("data/pack/client/sounds")

	CrcBuffer32 = packet.GetCRC(CrcBuffer.Bytes(), 0, len(CrcBuffer.Bytes()))
}
