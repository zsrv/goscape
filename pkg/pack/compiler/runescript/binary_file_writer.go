package runescript

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/zsrv/goscape/pkg/pack/compiler/codegen"
	"github.com/zsrv/goscape/pkg/pack/compiler/writer"
)

// BinaryFileScriptWriter writes each script as a single file named after its
// numeric id under output. Mirrors TS
// src/runescript/writer/BinaryFileScriptWriter.ts.
type BinaryFileScriptWriter struct {
	*BinaryScriptWriter
	output string
}

// NewBinaryFileScriptWriter prepares output as a directory and returns a sink
// that satisfies BinaryOutput. Returns error if output cannot be created as a
// directory. Mirrors TS L13-24 (TS's stat-check is folded into MkdirAll's
// ENOTDIR error return — Go idiom).
func NewBinaryFileScriptWriter(output string, ids writer.IdProvider) (*BinaryFileScriptWriter, error) {
	if err := os.MkdirAll(output, 0o755); err != nil {
		return nil, fmt.Errorf("BinaryFileScriptWriter: mkdir %s: %w", output, err)
	}
	w := &BinaryFileScriptWriter{output: output}
	w.BinaryScriptWriter = NewBinaryScriptWriter(ids, w)
	return w, nil
}

// OutputScript writes data to <output>/<id>. Mirrors TS L26-30.
func (w *BinaryFileScriptWriter) OutputScript(script *codegen.RuneScript, data []byte) {
	id := w.IdProvider.Get(script.Symbol)
	path := filepath.Join(w.output, strconv.Itoa(id))
	if err := os.WriteFile(path, data, 0o644); err != nil {
		panic(fmt.Sprintf("BinaryFileScriptWriter: write %s: %v", path, err))
	}
}
