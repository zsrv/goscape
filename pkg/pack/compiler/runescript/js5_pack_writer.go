package runescript

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/zsrv/goscape/pkg/pack/compiler/codegen"
	"github.com/zsrv/goscape/pkg/pack/compiler/writer"
)

// JS5 constants. Mirror TS L25-27.
const (
	js5IndexFormat  = 7
	js5IndexVersion = 1
	js5GroupVersion = 1
)

type js5CompressionType int

const (
	js5CompressionNone js5CompressionType = 0
	js5CompressionGzip js5CompressionType = 2
)

// Js5PackScriptWriter packs scripts into a complete sequential .js5 archive.
// Mirrors TS src/runescript/writer/Js5PackScriptWriter.ts.
type Js5PackScriptWriter struct {
	*BinaryScriptWriter
	output  string
	buffers map[int][]byte
}

// NewJs5PackScriptWriter prepares filepath.Dir(output) as a directory.
// Mirrors TS L33-37 (TS isDirectory() check folded into MkdirAll's ENOTDIR
// error return — Go idiom).
func NewJs5PackScriptWriter(output string, ids writer.IdProvider) (*Js5PackScriptWriter, error) {
	dir := filepath.Dir(output)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("Js5PackScriptWriter: mkdir %s: %w", dir, err)
	}
	w := &Js5PackScriptWriter{
		output:  output,
		buffers: map[int][]byte{},
	}
	w.BinaryScriptWriter = NewBinaryScriptWriter(ids, w)
	return w, nil
}

// OutputScript stores a copy of data keyed by script id. Mirrors TS L45-49.
func (w *Js5PackScriptWriter) OutputScript(script *codegen.RuneScript, data []byte) {
	id := w.IdProvider.Get(script.Symbol)
	w.buffers[id] = bytes.Clone(data)
}

type js5Group struct {
	groupID     int
	packedGroup []byte
	checksum    int32
	version     int32
}

// Close emits the JS5 archive. Mirrors TS L51-78.
func (w *Js5PackScriptWriter) Close() error {
	keys := make([]int, 0, len(w.buffers))
	for k := range w.buffers {
		keys = append(keys, k)
	}
	slices.Sort(keys)

	groups := make([]js5Group, 0, len(keys))
	for _, id := range keys {
		packed, err := w.packGroup(w.buffers[id], js5CompressionNone)
		if err != nil {
			return err
		}
		groups = append(groups, js5Group{
			groupID:     id,
			packedGroup: packed,
			checksum:    Crc32(packed),
			version:     js5GroupVersion,
		})
	}

	indexData := w.encodeIndex(groups)
	packedIndex, err := w.packGroup(indexData, js5CompressionGzip)
	if err != nil {
		return err
	}

	f, err := os.Create(w.output)
	if err != nil {
		return fmt.Errorf("Js5PackScriptWriter: create %s: %w", w.output, err)
	}
	defer f.Close()

	if _, err := f.Write(packedIndex); err != nil {
		return err
	}
	for _, g := range groups {
		if _, err := f.Write(g.packedGroup); err != nil {
			return err
		}
	}
	for _, g := range groups {
		if err := writeBE4(f, int32(len(g.packedGroup))); err != nil {
			return err
		}
	}
	return nil
}

// encodeIndex serialises the JS5 index group. Mirrors TS L80-112.
func (w *Js5PackScriptWriter) encodeIndex(groups []js5Group) []byte {
	bw := NewByteWriter(128)
	bw.P1(js5IndexFormat)
	bw.P4(js5IndexVersion)
	bw.P1(0) // flags: no names / digests / lengths / uncompressed checksums.
	bw.PSmart2or4(len(groups))

	previousGroupID := 0
	for _, g := range groups {
		bw.PSmart2or4(g.groupID - previousGroupID)
		previousGroupID = g.groupID
	}
	for _, g := range groups {
		bw.P4(g.checksum)
	}
	for _, g := range groups {
		bw.P4(g.version)
	}
	for range groups {
		bw.PSmart2or4(1) // one file per group
	}
	for range groups {
		bw.PSmart2or4(0) // single file id (0), delta-encoded
	}
	return bw.Bytes()
}

// packGroup wraps src with the JS5 compression prefix. Mirrors TS L114-138.
func (w *Js5PackScriptWriter) packGroup(src []byte, compression js5CompressionType) ([]byte, error) {
	bw := NewByteWriter(len(src) + 16)
	bw.P1(int(compression))

	switch compression {
	case js5CompressionNone:
		bw.P4(int32(len(src)))
		bw.PData(src)
	case js5CompressionGzip:
		var buf bytes.Buffer
		gz := gzip.NewWriter(&buf)
		if _, err := gz.Write(src); err != nil {
			return nil, err
		}
		if err := gz.Close(); err != nil {
			return nil, err
		}
		compressed := buf.Bytes()
		// NAI-210-D-GZIP-OS-BYTE-ZEROED: TS Js5PackScriptWriter.ts L125 sets
		// `compressed[9] = 0;`. Go compress/gzip writes the host OS byte at
		// offset 9 of the gzip stream; zero it for byte-identical
		// reproducibility with TS.
		if len(compressed) > 9 {
			compressed[9] = 0
		}
		bw.P4(int32(len(compressed)))
		bw.P4(int32(len(src)))
		bw.PData(compressed)
	default:
		return nil, fmt.Errorf("Js5PackScriptWriter: unsupported compression type %d", compression)
	}
	return bw.Bytes(), nil
}
