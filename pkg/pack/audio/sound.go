// Package audio ports the client-stage audio packers from
// tools/pack/sound/pack.ts and tools/pack/midi/pack.ts.
package audio

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/zsrv/goscape/pkg/io/jagfile"
	"github.com/zsrv/goscape/pkg/io/packet"
	"github.com/zsrv/goscape/pkg/pack"
)

// soundCRCMagic is the TS sound/pack.ts:46 BUILD_VERIFY constant.
//
// NAI-213-D-SOUND-CRC-DISABLED-MIRROR-TS: TS has the CRC check
// commented out; we mirror, retaining the constant for future activation.
const soundCRCMagic int32 = -1570057128

// PackSound ports TS sound/pack.ts:packClientSound.
//
// Reads <srcDir>/synth/*.synth, gates each by reg.Synth.GetByName(),
// emits in <srcDir>/pack/synth.order order as:
//
//	[id u16][synth-bytes...]
//	...
//	[0xffff terminator]
//
// Wraps in a Jagfile saved to <outDir>/client/sounds.
func PackSound(reg *pack.Registry, srcDir, outDir string) error {
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

	_ = soundCRCMagic // retain constant; see NAI-213-D-SOUND-CRC-DISABLED-MIRROR-TS

	jag.Write("sounds.dat", out)

	clientOut := filepath.Join(outDir, "client", "sounds")
	if err := os.MkdirAll(filepath.Dir(clientOut), 0o755); err != nil {
		return err
	}
	return jag.Save(clientOut)
}
