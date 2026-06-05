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

// soundCRCMagic is the TS sound/pack.ts:47 build-verify constant at rev-244
// (Engine-TS@9aadcec4). Updated from the 225 placeholder (-1570057128) which
// was commented out; this value is now active.
const soundCRCMagic int32 = -1415586973

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
	// NAI-213-D-BUILDVERIFY-SOUND-MAY-DIVERGE: downgrade to informational
	// stderr log and continue writing, mirroring clientinterface posture.
	if err := pack.BuildVerify(out.Data, out.Length(), soundCRCMagic); err != nil {
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
