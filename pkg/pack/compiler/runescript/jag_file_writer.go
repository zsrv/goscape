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

// jagFileVersion is the .dat header version constant.
//
// 27 at the rev-254 pin-advance (the Task A15 flip, fronted by Task A1's
// reference-cache re-pin): the Arc-26 REFERENCES condition is met — the
// pinned engine depends on `@lostcityrs/runescript@0.9.6`, which carries
// upstream commit 750291c ("chore: Bumped compiler version", a pure marker
// bump with no layout change), and the rev-274 reference cache
// (Server274-ref/engine/data/pack/server/script.dat @4c95f87e, re-pinned by
// the 2026-07-16 pin-update plan Task 5) still reports 27 in its header
// (unchanged from the rev-254 @2e3bcf43 reference and the rev-274 @dee467c8
// reference before it). The Go
// engine reads this header via
// pkg/script/provider.go (CompilerVersion = 27) and rejects mismatches,
// so the packer must emit 27 for byte-parity and engine-loadable output.
//
// History: pinned to 26 from Arc-26 (NAI-221 companion) through rev-245.2
// because the then-pinned `@lostcityrs/runescript@^0.9.4` predated the
// 750291c bump. Keep this constant and CompilerVersion in lockstep.
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
