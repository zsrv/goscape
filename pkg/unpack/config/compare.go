// Package config implements the config-archive unpackers for the RS2 254 tool chain.
//
// This file ports tools/unpack/config/Compare.ts.
package config

import (
	"fmt"
	"io"
	"path/filepath"

	"github.com/zsrv/goscape/pkg/io/filestream"
	"github.com/zsrv/goscape/pkg/io/jagfile"
	"github.com/zsrv/goscape/pkg/io/packet"
)

// simpleConfigIdx mirrors the local readConfigIdx in Compare.ts (lines 6-18).
// It stores only positional metadata — no Dat pointer — because Compare reads
// entry bytes directly from the Packet it already holds.
//
// TS Compare.ts:6-18:
//
//	function readConfigIdx(idx: Packet): { pos: number[], len: number[] }
type simpleConfigIdx struct {
	pos []int
	len []int
}

// readSimpleConfigIdx parses a config .idx Packet into pos/len arrays.
// Mirrors Compare.ts:readConfigIdx exactly.
//
// TS Compare.ts:7-18:
//
//	const count = idx.g2()
//	let cur = 2
//	for i 0..count: pos[i]=cur; len[i]=idx.g2(); cur+=len[i]
func readSimpleConfigIdx(idx *packet.Packet) simpleConfigIdx {
	count := int(idx.G2())
	pos := make([]int, count)
	ln := make([]int, count)
	cur := 2
	for i := range count {
		pos[i] = cur
		ln[i] = int(idx.G2())
		cur += ln[i]
	}
	return simpleConfigIdx{pos: pos, len: ln}
}

// Compare mirrors tools/unpack/config/Compare.ts: per-entry length+CRC diff of
// one config type between the data/unpack cache and the packed client config jag.
//
// cacheDir is the directory containing main_file_cache.dat/idx* (FileStream root).
// packDir is the directory containing the packed client/config flat file
// (e.g. data/pack/client/config on disk — loaded directly, not via FileStream).
// typ is the config type to compare (e.g. "npc"); Compare.ts hardcodes "npc".
// out receives bare message lines mirroring TS printInfo / printWarning output.
//
// TS source: tools/unpack/config/Compare.ts.
func Compare(cacheDir, packDir, typ string, out io.Writer) error {
	printInfo := func(msg string) {
		if out != nil {
			fmt.Fprintf(out, "%s\n", msg)
		}
	}
	printWarning := func(msg string) {
		if out != nil {
			fmt.Fprintf(out, "%s\n", msg)
		}
	}

	// TS Compare.ts:54-57: cache1 = new FileStream('data/unpack')
	//   config1 = new Jagfile(new Packet(cache1.read(0, 2)!))
	cache1 := filestream.New(cacheDir, false, false)
	defer cache1.Close()
	raw1 := cache1.Read(0, 2, false)
	if raw1 == nil {
		return fmt.Errorf("Compare: cache1.Read(0, 2) returned nil")
	}
	config1, err := jagfile.NewJagfile(packet.NewPacket(raw1))
	if err != nil {
		return fmt.Errorf("Compare: parse config1 jagfile: %w", err)
	}

	// TS Compare.ts:56: const idx1 = readConfigIdx(config1.read(configType + '.idx')!)
	idxPkt1, err := config1.Read(typ + ".idx")
	if err != nil {
		return fmt.Errorf("Compare: config1 read %s.idx: %w", typ, err)
	}
	idx1 := readSimpleConfigIdx(idxPkt1)

	// TS Compare.ts:57: const dat1 = config1.read(configType + '.dat')!
	dat1, err := config1.Read(typ + ".dat")
	if err != nil {
		return fmt.Errorf("Compare: config1 read %s.dat: %w", typ, err)
	}

	// TS Compare.ts:60: Packet.load('data/pack/client/config')
	// — loads the flat file directly (not via FileStream).
	config2File := filepath.Join(packDir, "client", "config")
	pkt2, err := packet.Load(config2File, false)
	if err != nil {
		return fmt.Errorf("Compare: load %q: %w", config2File, err)
	}
	config2, err := jagfile.NewJagfile(pkt2)
	if err != nil {
		return fmt.Errorf("Compare: parse config2 jagfile: %w", err)
	}

	// TS Compare.ts:61: const idx2 = readConfigIdx(config2.read(configType + '.idx')!)
	idxPkt2, err := config2.Read(typ + ".idx")
	if err != nil {
		return fmt.Errorf("Compare: config2 read %s.idx: %w", typ, err)
	}
	idx2 := readSimpleConfigIdx(idxPkt2)

	// TS Compare.ts:62: const dat2 = config2.read(configType + '.dat')!
	dat2, err := config2.Read(typ + ".dat")
	if err != nil {
		return fmt.Errorf("Compare: config2 read %s.dat: %w", typ, err)
	}

	// TS Compare.ts:64: printInfo(configType)
	printInfo(typ)

	// TS Compare.ts:65-68: whole-dat CRC comparison
	crc1 := packet.GetCRC(dat1.Data, 0, len(dat1.Data))
	crc2 := packet.GetCRC(dat2.Data, 0, len(dat2.Data))
	if crc1 == crc2 {
		// TS Compare.ts:67: printInfo('exact match')
		printInfo("exact match")
		return nil
	}

	// TS Compare.ts:66: compareDat(idx1, idx2, dat1, dat2)
	compareDat(idx1, idx2, dat1, dat2, printWarning)
	return nil
}

// compareDat mirrors the local compareDat in Compare.ts (lines 21-49).
// It reports per-entry length and CRC mismatches between two config archives.
//
// TS Compare.ts:21-49:
//
//	function compareDat(idx1, idx2, dat1, dat2)
func compareDat(
	idx1, idx2 simpleConfigIdx,
	dat1, dat2 *packet.Packet,
	printWarning func(string),
) {
	// TS Compare.ts:22-24: size mismatch warning
	if len(idx1.pos) != len(idx2.pos) {
		printWarning(fmt.Sprintf("different config sizes, %d != %d", len(idx1.pos), len(idx2.pos)))
	}

	for i := range len(idx1.pos) {
		// TS Compare.ts:27-30: entry missing in idx2
		if i >= len(idx2.pos) {
			printWarning(fmt.Sprintf("%d: does not exist", i))
			continue
		}

		// TS Compare.ts:32-35: length mismatch → skip CRC check
		if idx1.len[i] != idx2.len[i] {
			printWarning(fmt.Sprintf("%d: length does not match, %d != %d", i, idx1.len[i], idx2.len[i]))
			continue
		}

		// TS Compare.ts:37-43: read entry bytes from each dat
		temp1 := dat1.Data[idx1.pos[i] : idx1.pos[i]+idx1.len[i]]
		temp2 := dat2.Data[idx2.pos[i] : idx2.pos[i]+idx2.len[i]]

		// TS Compare.ts:45-48: CRC mismatch warning
		if packet.GetCRC(temp1, 0, len(temp1)) != packet.GetCRC(temp2, 0, len(temp2)) {
			printWarning(fmt.Sprintf("%d: crc does not match", i))
		}
	}
}
