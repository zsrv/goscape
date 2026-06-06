// Package clientinterface — unpack.go implements the top-level Unpack entry
// point and filesystem helpers used by export.go.
//
// TS source: tools/unpack/interface/Unpack.ts:859-876 (main flow).
package clientinterface

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/zsrv/goscape/pkg/io/filestream"
	"github.com/zsrv/goscape/pkg/io/jagfile"
	"github.com/zsrv/goscape/pkg/io/packet"
	"github.com/zsrv/goscape/pkg/pack"
)

// Options holds all inputs for a client-interface unpack run.
type Options struct {
	// CacheDir is the directory containing main_file_cache.dat/idx0-4.
	// TS: 'data/unpack'.
	CacheDir string

	// SrcDir is the content tree root (BUILD_SRC_DIR in TS). All output files
	// are written relative to this directory.
	SrcDir string

	// Out is the print channel for info messages. nil = discard.
	// TS printInfo → Out. (Interface family emits no printInfo in production;
	// kept for structural parity with config Options.)
	Out io.Writer

	// Errorf is the console.error sink; nil = no-op.
	Errorf func(format string, args ...any)
}

// Unpack is the top-level entry point for the client-interface unpack.
// It mirrors the TS Unpack.ts main script (lines 859-876):
//
//  1. Open FileStream(CacheDir), read archive 0 / file 3 (interface jagfile).
//  2. Decode the jagfile.
//  3. mkdir models/com (TS line 867-869).
//  4. ExportOrder → pack/interface.order.
//  5. ExportSrc → naming passes + .if files.
//  6. ModelPack.save().
//
// TS source: tools/unpack/interface/Unpack.ts:859-876.
func Unpack(opts Options) error {
	if opts.Errorf == nil {
		opts.Errorf = func(string, ...any) {}
	}

	// TS line 859: const cache = new FileStream('data/unpack')
	cache := filestream.New(opts.CacheDir, false, false)
	defer cache.Close()

	// TS line 860: const interfaceData = cache.read(0, 3)
	interfaceData := cache.Read(0, 3, false)
	if interfaceData == nil {
		return fmt.Errorf("No interface data in cache")
	}

	// TS line 867-869: mkdir models/com
	comDir := filepath.Join(opts.SrcDir, "models", "com")
	if err := mkdirAll(comDir); err != nil {
		return fmt.Errorf("mkdir models/com: %w", err)
	}

	// TS line 871: IfType.unpack(new Jagfile(new Packet(interfaceData)))
	jag, err := jagfile.NewJagfile(packet.NewPacket(interfaceData))
	if err != nil {
		return fmt.Errorf("parse interface jagfile: %w", err)
	}
	dec, err := Decode(jag)
	if err != nil {
		return fmt.Errorf("decode interface: %w", err)
	}

	// Build Registry with opts.SrcDir.
	reg := &pack.Registry{SrcDir: opts.SrcDir}

	interfacePack, err := reg.EnsureInterface()
	if err != nil {
		return fmt.Errorf("ensure interface pack: %w", err)
	}
	modelPack, err := reg.EnsureModel()
	if err != nil {
		return fmt.Errorf("ensure model pack: %w", err)
	}
	objPack, err := reg.EnsureObj()
	if err != nil {
		return fmt.Errorf("ensure obj pack: %w", err)
	}
	seqPack, err := reg.EnsureSeq()
	if err != nil {
		return fmt.Errorf("ensure seq pack: %w", err)
	}
	varpPack, err := reg.EnsureVarp()
	if err != nil {
		return fmt.Errorf("ensure varp pack: %w", err)
	}

	// TS line 872: IfType.exportOrder()
	orderPath := filepath.Join(opts.SrcDir, "pack", "interface.order")
	if err := ExportOrder(dec, orderPath); err != nil {
		return fmt.Errorf("exportOrder: %w", err)
	}

	// TS line 873: IfType.exportSrc()
	if err := ExportSrc(dec, interfacePack, modelPack, objPack, seqPack, varpPack, opts.SrcDir, opts.Errorf, nil); err != nil {
		return fmt.Errorf("exportSrc: %w", err)
	}

	// TS line 875: ModelPack.save()
	if err := modelPack.Save(); err != nil {
		return fmt.Errorf("save model pack: %w", err)
	}

	return nil
}

// defaultRename is the real os.Rename used when no hook is injected.
func defaultRename(src, dst string) error {
	return os.Rename(src, dst)
}

// writeFileMkdir writes content to path, creating parent directories as needed.
func writeFileMkdir(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", filepath.Dir(path), err)
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

// mkdirAll creates the directory and any necessary parents.
func mkdirAll(path string) error {
	return os.MkdirAll(path, 0o755)
}
