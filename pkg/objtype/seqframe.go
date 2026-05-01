package objtype

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/zsrv/goscape/pkg/io/packet"
)

// SeqFrame is the delay-only portion of a single seq frame record.
// Mirrors Engine-TS/src/cache/config/SeqFrame.ts. The TS class is
// intentionally partial (`// partial frame class - only delays, not loading
// transforms`); goscape preserves that shape.
type SeqFrame struct {
	Delay int
}

// SeqFrameConfigs holds all parsed frame records, indexed by frame id.
// TS exposes this as `SeqFrame.instances` static (Engine-TS/.../SeqFrame.ts:7).
type SeqFrameConfigs struct {
	Instances []*SeqFrame
}

// LoadSeqFrames reads data/server/frame_del.dat. Each byte in the file
// is one frame's delay (g1 per byte). Returns an empty registry with nil
// error when the file is absent (silent-on-missing, matching TS
// SeqFrame.load at Engine-TS/.../SeqFrame.ts:9-16).
func LoadSeqFrames(dir string) (*SeqFrameConfigs, error) {
	dat, err := packet.Load(filepath.Join(dir, "server", "frame_del.dat"), false)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &SeqFrameConfigs{}, nil
		}
		return nil, err
	}
	return parseSeqFrames(dat), nil
}

func parseSeqFrames(dat *packet.Packet) *SeqFrameConfigs {
	n := dat.Len()
	instances := make([]*SeqFrame, n)
	for i := range n {
		instances[i] = &SeqFrame{Delay: int(dat.G1())}
	}
	return &SeqFrameConfigs{Instances: instances}
}
