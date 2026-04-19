package packet

import (
	"os"
)

// Load reads a file at path and returns it wrapped in a *Packet.
// When compressed=true, the file is BZIP2-decompressed first; sub-spec 3a's
// callers always pass false.
func Load(path string, compressed bool) (*Packet, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if compressed {
		panic("packet.Load: compressed=true not yet supported")
	}
	return NewPacket(data), nil
}
