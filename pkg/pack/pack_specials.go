package pack

import (
	"path/filepath"
	"strings"

	"github.com/zsrv/goscape/pkg/io/packet"
)

// packAndSaveCategoryDat writes <serverOut>/category.dat — a server-
// only enumeration of all registered category names from the in-memory
// categoryPack registry. No idx sibling; no client-jagfile contribution.
//
// Byte layout (per TS PackShared.ts:341-352):
//   - p2(categoryPack.Size())                        — registered name count
//   - for i in 0..Size()-1:                          — dense-id iteration
//     p1(1)                                        — record marker
//     pjstr(categoryPack.GetByID(i))               — LF-terminated name
//     p1(0)                                        — record terminator
//
// Empty registry → file contains just p2(0) (2 bytes).
//
// NAI-199-D-TS-CODE-STALENESS-GATE: TS gate's second arm
// `shouldBuild('tools/pack/config', '.ts', dest)` is dropped — that
// arm rebuilds when TS pipeline source files are newer than output;
// has no Go-binary equivalent at runtime. Goscape uses only the
// `ShouldBuildFile(<src>/pack/category.pack, dest)` arm. Gate logic
// lives in PackConfigs; this function is the unconditional writer.
//
// TS source: tools/pack/config/PackShared.ts:341-352.
func packAndSaveCategoryDat(serverOut string, categoryPack *PackFile) error {
	dat := packet.Alloc(1)
	size := categoryPack.Size()
	dat.P2(uint16(size))
	for i := range size {
		dat.P1(1)
		dat.PJStrLF(categoryPack.GetByID(i))
		dat.P1(0)
	}
	// NAI-192-D-PACKET-WRITE-CURSOR: writes append to len(Data); Pos is
	// the read pointer (memory [[packet_rw_pointer_gotcha]]). Use
	// Length() — i.e. len(Data) — for the byte count.
	return dat.Save(filepath.Join(serverOut, "category.dat"), dat.Length(), 0)
}

// packAndSaveFrameDel writes <serverOut>/frame_del.dat — for each
// registered AnimPack id (0..animPack.Max-1), one byte extracted from
// the corresponding <srcDir>/models/<name>.frame file's del segment.
// Server-only; no idx sibling; no client-jagfile contribution.
//
// Per-id byte:
//   - animPack.GetByID(i) == ""           → p1(0)
//   - no <name>.frame file on disk        → p1(0)
//   - else load .frame, read trailer at end-8 (3×g2 = head/tran1/tran2
//     lengths; 4th g2 implicit/discarded), seek to start+head+tran1+
//     tran2, emit g1() (first byte of del segment).
//
// Empty AnimPack (Max=0) → 0-byte output.
//
// File-match (TS PackShared.ts:365 uses files.find(f =>
// f.endsWith(name+'.frame'))): goscape mirrors via strings.HasSuffix.
// Both share the (latent) suffix-substring false-positive: a name "foo"
// matches files ending "bigfoo.frame". Acceptable per [[true_to_ts_gate]]
// — literal port; not promoted to a formal deviation tag.
//
// NAI-199-D-TS-CODE-STALENESS-GATE: TS gate's second arm
// `shouldBuild('tools/pack/config', '.ts', dest)` is dropped — that
// arm rebuilds when TS pipeline source files are newer than output;
// has no Go-binary equivalent at runtime. Goscape uses only the
// `ShouldBuild(<src>/models, '.frame', dest)` arm (plus
// `GetLatestModified > 0` no-src guard). Gate logic lives in
// PackConfigs; this function is the unconditional writer.
//
// TS source: tools/pack/config/PackShared.ts:355-388.
func packAndSaveFrameDel(srcDir, serverOut string, animPack *PackFile) error {
	modelsDir := filepath.Join(srcDir, "models")
	files := ListFilesExt(modelsDir, ".frame")
	out := packet.Alloc(3)
	for i := range animPack.Max {
		name := animPack.GetByID(i)
		if name == "" {
			out.P1(0)
			continue
		}
		suffix := name + ".frame"
		var match string
		for _, f := range files {
			if strings.HasSuffix(f, suffix) {
				match = f
				break
			}
		}
		if match == "" {
			out.P1(0)
			continue
		}
		data, err := packet.Load(match, false)
		if err != nil {
			return err
		}
		data.Pos = len(data.Data) - 8
		headLen := data.G2()
		tran1Len := data.G2()
		tran2Len := data.G2()
		data.Pos = int(headLen) + int(tran1Len) + int(tran2Len)
		out.P1(data.G1())
	}
	// NAI-192-D-PACKET-WRITE-CURSOR: out is write-only; use Length()
	// for the byte count (Pos remains 0 since writes don't advance it).
	return out.Save(filepath.Join(serverOut, "frame_del.dat"), out.Length(), 0)
}
