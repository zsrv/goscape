package pack

import (
	"fmt"
	"os"

	"github.com/zsrv/goscape/pkg/io/packet"
)

// BuildVerify checks that the CRC of the first length bytes of data
// matches expected. Used by clientinterface (active) and audio
// (commented-out in TS, magic number retained as a constant).
//
// expected is the int32 magic number from TS source (e.g. -2146838800
// for interface). Internally we convert to uint32 for packet.CheckCRC.
//
// TS source: PixPack.ts uses Packet.checkcrc(data, 0, pos, expected).
func BuildVerify(data []uint8, length int, expected int32) error {
	if !packet.CheckCRC(data, 0, length, uint32(expected)) {
		return fmt.Errorf("CRC mismatch (got=%d want=%d)", packet.GetCRC(data, 0, length), expected)
	}
	return nil
}

// Archive CRC magics for the packed cache archives, as pinned by Engine-TS
// 8139461a. Each is the CRC of the FINAL FILE BYTES — the saved jagfile read
// back off disk — not of the in-memory buffer before jag.save.
//
// That relocation is the substance of the upstream change: the checks used to
// run against the pre-save Packet (interface at PackClient.ts:36-38 and synth
// at sound/pack.ts:58-60 @4c95f87e), which is why the interface and synth
// constants both changed value. Title, textures and wordenc had no gate at all
// before. The superseded pre-save values are recorded in the history comment
// on TestBuildVerifyMagicNumbers_AppearExactlyOnce.
//
// All five values were confirmed empirically against goscape's own byte-parity
// output at Engine-TS 1d25566c / Content 2b62ae68d.
const (
	TitleCRCMagic     int32 = 410306098  // archive (0, 1) — TS sprite/title.ts:49
	InterfaceCRCMagic int32 = 2135735991 // archive (0, 3) — TS interface/PackClient.ts:45
	TextureCRCMagic   int32 = 915347346  // archive (0, 6) — TS sprite/textures.ts:39
	WordencCRCMagic   int32 = 1386621111 // archive (0, 7) — TS chat/pack.ts:10
	SoundCRCMagic     int32 = -759577225 // archive (0, 8) — TS sound/pack.ts:64
)

// VerifyArchive checks a packed archive's file bytes against its pinned CRC
// and reports a mismatch on stderr without failing the pack.
//
// TS throws here when BUILD_VERIFY is set. goscape logs instead, extending the
// established NAI-213-D-BUILDVERIFY-*-MAY-DIVERGE posture (see the long
// rationale at pkg/pack/clientinterface/pack.go and pkg/pack/audio/sound.go)
// to the three archives 8139461a newly gated. The reasoning is unchanged: the
// magic is a hash of one specific content tree, and goscape must stay able to
// pack custom trees and synthetic fixtures. The constants are still pinned so
// a real regression is visible in the log, and the byte-parity gate against
// the reference cache is what actually enforces correctness in CI.
func VerifyArchive(name string, data []uint8, expected int32) {
	if err := BuildVerify(data, len(data), expected); err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v (NAI-213-D-BUILDVERIFY-MAY-DIVERGE)\n", name, err)
	}
}
