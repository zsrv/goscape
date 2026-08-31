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
// The synth build-verify constant lives in pkg/pack as pack.SoundCRCMagic
// now. Engine-TS 8139461a moved the check from the pre-save out packet (magic
// 2127412105) to the saved file bytes (-759577225).

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
	// The synth CRC gate moved to the saved file bytes at the cache-write site
	// below (TS sound/pack.ts:63-68 @1d25566c). It used to run here against
	// the pre-save out packet with magic 2127412105.

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
		// TS sound/pack.ts:63-68 @1d25566c moved this gate off the pre-save
		// out packet and onto the saved file bytes, which is why the magic
		// changed 2127412105 -> -759577225.
		pack.VerifyArchive("packClientSound", data, pack.SoundCRCMagic)
		cache.Write(0, 8, data, 0)
	}
	return nil
}
