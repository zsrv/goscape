package pack

import (
	"github.com/zsrv/goscape/pkg/io/packet"
)

// PackedData is a paired dat+idx Packet buffer used by per-config
// packers. The dat buffer holds per-entry bodies separated by a 0x00
// terminator; the idx buffer holds the per-entry byte length as a
// big-endian uint16.
//
// Not safe for concurrent use.
//
// TS source: tools/pack/config/PackShared.ts:39-84.
type PackedData struct {
	Dat    *packet.Packet
	Idx    *packet.Packet
	Size   int
	marker int
}

// NewPackedData allocates a fresh dat+idx pair, writes p2(size) into
// both as the count header, and sets marker=2 (past the header).
func NewPackedData(size int) *PackedData {
	pd := &PackedData{
		Dat:  packet.Alloc(5),
		Idx:  packet.Alloc(3),
		Size: size,
	}
	pd.Dat.P2(uint16(size))
	pd.Idx.P2(uint16(size))
	pd.marker = 2
	return pd
}

// Next writes one terminator (0x00) to dat, records the bytes-since-marker
// to idx as a p2, and advances marker to the new dat write cursor.
//
// NAI-192-D-PACKET-WRITE-CURSOR: TS uses dat.pos; goscape's Packet.Pos
// is the read pointer (memory packet_rw_pointer_gotcha). Use
// Dat.Length() — i.e. len(Dat.Data) — for the write cursor.
func (pd *PackedData) Next() {
	pd.Dat.P1(0)
	pd.Idx.P2(uint16(pd.Dat.Length() - pd.marker))
	pd.marker = pd.Dat.Length()
}

func (pd *PackedData) P1(v uint8)   { pd.Dat.P1(v) }
func (pd *PackedData) P2(v uint16)  { pd.Dat.P2(v) }
func (pd *PackedData) P3(v uint32)  { pd.Dat.P3(v) }
func (pd *PackedData) P4(v uint32)  { pd.Dat.P4(v) }
func (pd *PackedData) PBool(v bool) { pd.Dat.PBool(v) }

// PJStr writes a JagString with an LF (0x0a) terminator, matching
// TS Packet.pjstr at io/Packet.ts:336.
func (pd *PackedData) PJStr(s string) { pd.Dat.PJStrLF(s) }

// Save writes the full dat and idx buffers to disk. Parent directories
// are created via packet.Packet.Save's os.MkdirAll.
func (pd *PackedData) Save(dataPath, idxPath string) error {
	if err := pd.Dat.Save(dataPath, pd.Dat.Length(), 0); err != nil {
		return err
	}
	return pd.Idx.Save(idxPath, pd.Idx.Length(), 0)
}
