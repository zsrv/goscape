// Package checksum implements the cache checksum debug tool for the RS2 245.2 tool chain.
//
// TS source: tools/unpack/checksum.ts.
package checksum

import (
	"fmt"
	"io"
	"path/filepath"

	"github.com/zsrv/goscape/pkg/io/filestream"
	"github.com/zsrv/goscape/pkg/io/jagfile"
	"github.com/zsrv/goscape/pkg/io/packet"
)

// Run mirrors tools/unpack/checksum.ts: prints per-member CRCs of the config,
// interface, and synth jags and extracts each member to <cacheDir>/<jag>/<name>.
//
// TS checksum.ts:5: const cache = new FileStream('data/unpack')
// TS checksum.ts:16: printCrcs('config',    new Jagfile(new Packet(cache.read(0, 2))))
// TS checksum.ts:17: printCrcs('interface', new Jagfile(new Packet(cache.read(0, 3))))
// TS checksum.ts:18: printCrcs('synth',     new Jagfile(new Packet(cache.read(0, 8))))
//
// cacheDir is the FileStream root (contains main_file_cache.dat/idx*).
// out receives one line per member: "<jagName> <name> <signedInt32CRC>\n".
// Members are extracted to <cacheDir>/<jagName>/<name> (mkdir as needed).
//
// The CRC is printed as a signed int32 to match JavaScript's bitwise-NOT
// semantics: Packet.getcrc returns `~crc` which TypeScript/JS implicitly
// coerces to a signed 32-bit integer when printed via console.log.
func Run(cacheDir string, out io.Writer) error {
	// TS checksum.ts:5: const cache = new FileStream('data/unpack')
	// ctor signature: (dir, createNew=false, readOnly=false)
	cache := filestream.New(cacheDir, false, false)
	defer cache.Close()

	type jagSpec struct {
		name    string
		archive int
		file    int
	}

	// TS checksum.ts:16-18: three printCrcs calls
	specs := []jagSpec{
		{"config", 0, 2},
		{"interface", 0, 3},
		{"synth", 0, 8},
	}

	for _, s := range specs {
		raw := cache.Read(s.archive, s.file, false)
		if raw == nil {
			return fmt.Errorf("Run: cache.Read(%d, %d) returned nil for %q", s.archive, s.file, s.name)
		}
		jag, err := jagfile.NewJagfile(packet.NewPacket(raw))
		if err != nil {
			return fmt.Errorf("Run: parse %q jagfile: %w", s.name, err)
		}
		if err := printCrcs(s.name, jag, cacheDir, out); err != nil {
			return err
		}
	}
	return nil
}

// printCrcs mirrors the local printCrcs in checksum.ts (lines 7-13).
// For each member in jagfile directory order it prints
// "<jagName> <name> <signedInt32CRC>" and saves the member bytes to
// <cacheDir>/<jagName>/<name>.
//
// TS checksum.ts:7-13:
//
//	function printCrcs(jagName: string, jag: Jagfile)
//	    for (const name of jag.fileName)
//	        const file = jag.read(name)!
//	        console.log(jagName, name, Packet.getcrc(file.data, 0, file.length))
//	        file.save(`data/unpack/${jagName}/${name}`, file.length)
func printCrcs(jagName string, jag *jagfile.Jagfile, cacheDir string, out io.Writer) error {
	for i, name := range jag.FileName {
		if name == "" {
			// Skip entries whose name hash is not in knownNames (unresolved).
			// jag.FileName is sized to FileCount; unresolved entries are "".
			_ = i
			continue
		}

		pkt, err := jag.Read(name)
		if err != nil {
			return fmt.Errorf("printCrcs %q read %q: %w", jagName, name, err)
		}

		// CRC printed as signed int32 — mirrors JS console.log(Packet.getcrc(...))
		// which returns ~crc (bitwise NOT), yielding a signed int32.
		crc := int32(packet.GetCRC(pkt.Data, 0, len(pkt.Data)))

		// TS checksum.ts:10: console.log(jagName, name, Packet.getcrc(...))
		fmt.Fprintf(out, "%s %s %d\n", jagName, name, crc)

		// TS checksum.ts:12: file.save(`data/unpack/${jagName}/${name}`, file.length)
		savePath := filepath.Join(cacheDir, jagName, name)
		if err := pkt.Save(savePath, len(pkt.Data), 0); err != nil {
			return fmt.Errorf("printCrcs %q save %q: %w", jagName, name, err)
		}
	}
	return nil
}
