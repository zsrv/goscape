package pack

import (
	"path/filepath"

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
