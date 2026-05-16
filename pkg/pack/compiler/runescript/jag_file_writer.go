package runescript

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/zsrv/goscape/pkg/pack/compiler/codegen"
	"github.com/zsrv/goscape/pkg/pack/compiler/writer"
)

// jagFileVersion is the .dat header version constant. Mirrors TS L18.
const jagFileVersion = 27

// JagFileScriptWriter buffers scripts in-memory and writes script.dat +
// script.idx at Close. Mirrors TS src/runescript/writer/JagFileScriptWriter.ts.
type JagFileScriptWriter struct {
	*BinaryScriptWriter
	output  string
	buffers map[int][]byte
}

// NewJagFileScriptWriter prepares output as a directory. Mirrors TS L20-30
// (TS stat-check folded into MkdirAll's ENOTDIR error return — Go idiom).
func NewJagFileScriptWriter(output string, ids writer.IdProvider) (*JagFileScriptWriter, error) {
	if err := os.MkdirAll(output, 0o755); err != nil {
		return nil, fmt.Errorf("JagFileScriptWriter: mkdir %s: %w", output, err)
	}
	w := &JagFileScriptWriter{
		output:  output,
		buffers: map[int][]byte{},
	}
	w.BinaryScriptWriter = NewBinaryScriptWriter(ids, w)
	return w, nil
}

// OutputScript stores a copy of data keyed by script id. Mirrors TS L33-38
// (Buffer.from(data) clones the bytes).
func (w *JagFileScriptWriter) OutputScript(script *codegen.RuneScript, data []byte) {
	id := w.IdProvider.Get(script.Symbol)
	w.buffers[id] = bytes.Clone(data)
}

// Close emits script.dat + script.idx. Mirrors TS L40-72.
func (w *JagFileScriptWriter) Close() error {
	datPath := filepath.Join(w.output, "script.dat")
	idxPath := filepath.Join(w.output, "script.idx")

	dat, err := os.Create(datPath)
	if err != nil {
		return fmt.Errorf("JagFileScriptWriter: create dat: %w", err)
	}
	defer dat.Close()
	idx, err := os.Create(idxPath)
	if err != nil {
		return fmt.Errorf("JagFileScriptWriter: create idx: %w", err)
	}
	defer idx.Close()

	keys := make([]int, 0, len(w.buffers))
	for k := range w.buffers {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	lastID := 0
	if len(keys) > 0 {
		lastID = keys[len(keys)-1]
	}

	if err := writeBE4(dat, int32(lastID+1)); err != nil {
		return err
	}
	if err := writeBE4(idx, int32(lastID+1)); err != nil {
		return err
	}
	if err := writeBE4(dat, int32(jagFileVersion)); err != nil {
		return err
	}

	for i := range lastID + 1 {
		buf, ok := w.buffers[i]
		if !ok {
			if err := writeBE4(idx, 0); err != nil {
				return err
			}
			continue
		}
		if err := writeBE4(idx, int32(len(buf))); err != nil {
			return err
		}
		if _, err := dat.Write(buf); err != nil {
			return err
		}
	}
	return nil
}

// writeBE4 writes a 32-bit big-endian value to f.
func writeBE4(f *os.File, v int32) error {
	var b [4]byte
	binary.BigEndian.PutUint32(b[:], uint32(v))
	_, err := f.Write(b[:])
	return err
}
