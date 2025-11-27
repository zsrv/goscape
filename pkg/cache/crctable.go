package cache

import (
	"os"

	"github.com/zsrv/goscape/pkg/io/packet"
)

var (
	CrcBuffer   = packet.NewPacket(make([]byte, 0, 4*9))
	CrcTable    []uint32
	CrcBuffer32 uint32
)

func makeCrc(path string) {
	if _, err := os.Stat(path); err != nil {
		return
	}

	p, err := packet.Load(path)
	if err != nil {
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
