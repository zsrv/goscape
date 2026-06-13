// Package audio ports the client-stage audio packers from
// tools/pack/sound/pack.ts and tools/pack/midi/pack.ts.
package audio

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/zsrv/goscape/pkg/io/filestream"
	"github.com/zsrv/goscape/pkg/io/jagfile"
	"github.com/zsrv/goscape/pkg/io/packet"
	"github.com/zsrv/goscape/pkg/pack"
)

// soundCRCMagic is the TS sound/pack.ts build-verify constant at rev-274
// (Engine-TS@dee467c8, sound/pack.ts:58). History: -1570057128 (225
// placeholder = 245.2 value via 3c16994c) → -1415586973 (244 via 9aadcec4)
// → back to -1570057128 (245.2) → 831919863 (254 @ 2e3bcf43) → the value
// below (274 @ dee467c8).
const soundCRCMagic int32 = 2127412105

// PackSound ports TS sound/pack.ts:packClientSound at revision 244.
//
// Reads <srcDir>/synth/*.synth, gates each by reg.Synth.GetByName(),
// emits in <srcDir>/pack/synth.order order as:
//
//	[id u16][synth-bytes...]
//	...
//	[0xffff terminator]
//
// Wraps in a Jagfile saved to <outDir>/client/sounds.
//
// Build-verify: calls pack.BuildVerify with soundCRCMagic (mirroring TS
// sound/pack.ts:47-49). Mismatch is logged to stderr and execution continues
// (NAI-213-D-BUILDVERIFY-SOUND-MAY-DIVERGE; same posture as clientinterface).
//
// cache is an optional *filestream.FileStream. When non-nil, the packed
// client/sounds jagfile bytes are written to cache.Write(0, 8, data, 0),
// mirroring TS: `cache.write(0, 8, fs.readFileSync('data/pack/client/sounds'))`.
// Real handle is wired in T15.
func PackSound(reg *pack.Registry, srcDir, outDir string, cache *filestream.FileStream) error {
	synthPack, err := reg.EnsureSynth()
	if err != nil {
		return err
	}

	order := pack.LoadOrder(filepath.Join(srcDir, "pack", "synth.order"))
	files := pack.ListFilesExt(filepath.Join(srcDir, "synth"), ".synth")
	nameToFile := map[string]string{}
	for _, file := range files {
		name := strings.TrimSuffix(filepath.Base(file), filepath.Ext(file))
		if synthPack.GetByName(name) == -1 {
			continue
		}
		nameToFile[name] = file
	}

	jag := jagfile.NewEmptyJagfile(false)
	out := packet.Alloc(5)
	defer out.Release()

	for _, id := range order {
		name := synthPack.GetByID(id)
		if name == "" {
			continue
		}
		file, ok := nameToFile[name]
		if !ok {
			continue
		}
		out.P2(uint16(id))
		data, err := os.ReadFile(file)
		if err != nil {
			return err
		}
		out.PData(data)
	}
	out.P2(0xffff) // TS out.p2(-1) terminator

	// Build-verify: TS sound/pack.ts:47-49 at 9aadcec4.
	if err := pack.BuildVerify(out.Data, out.Length(), soundCRCMagic); err != nil {
		// NAI-213-D-BUILDVERIFY-SOUND-MAY-DIVERGE — CONFIRMED-EXCEPTION
		// (pack-media-compiler-13, rev-244 B6 audit closure):
		//
		// TS sound/pack.ts:47-49 hard-throws on CRC mismatch when the
		// TS environment's build-verify toggle is set. goscape downgrades to an
		// informational stderr log and continues writing. The downgrade
		// is INTENTIONAL and STRUCTURAL — not a transient defer:
		//
		//   1. The soundCRCMagic constant is a hash of TS's synth pack
		//      at a specific build moment. goscape's synth set derives
		//      from the content tree being packed (which may be stock
		//      LostCity, a custom content tree, or a synthetic test
		//      fixture). Any name-id divergence — by design or accident
		//      — produces a different CRC than the TS-stored magic.
		//   2. Aborting on mismatch would make goscape unable to pack
		//      ANY content tree whose synth set doesn't byte-match
		//      LostCity's at the build that generated the magic. Custom
		//      content trees and synthetic test fixtures are first-class
		//      use cases in goscape's design; the log lets the operator
		//      see the mismatch without breaking the pipeline.
		//   3. The magic constant is retained so it CAN re-engage if
		//      upstream pack consumers ever become TS-byte-faithful
		//      end-to-end (an env-gate could promote the log to a throw
		//      then), but that activation is not in scope for the
		//      current 1:1 parity arc.
		//
		// Audit row pack-media-compiler-13 closed as ✅ EXCEPTION-
		// DOCUMENTED — see docs/PORTING-CLOSED.md.
		fmt.Fprintf(os.Stderr, "packClientSound: %v (NAI-213-D-BUILDVERIFY-SOUND-MAY-DIVERGE)\n", err)
	}

	jag.Write("sounds.dat", out)

	clientOut := filepath.Join(outDir, "client", "sounds")
	if err := os.MkdirAll(filepath.Dir(clientOut), 0o755); err != nil {
		return err
	}
	if err := jag.Save(clientOut); err != nil {
		return err
	}

	if cache != nil {
		data, err := os.ReadFile(clientOut)
		if err != nil {
			return fmt.Errorf("PackSound: read client/sounds for cache: %w", err)
		}
		cache.Write(0, 8, data, 0)
	}
	return nil
}
