package pack

import (
	"fmt"

	"github.com/zsrv/goscape/pkg/io/packet"
)

// BuildVerify checks that the CRC of the first length bytes of data
// matches expected. Used by clientinterface (active) and audio
// (commented-out in TS, magic number retained as a constant).
//
// expected is the int32 magic number from TS source (e.g. -2146838800
// for interface). Internally we convert to uint32 for packet.CheckCRC.
//
// TS source: PixPack.ts uses Packet.checkcrc(data, 0, pos, expected).
func BuildVerify(data []uint8, length int, expected int32) error {
	if !packet.CheckCRC(data, 0, length, uint32(expected)) {
		return fmt.Errorf("CRC mismatch (got=%d want=%d)", packet.GetCRC(data, 0, length), expected)
	}
	return nil
}
