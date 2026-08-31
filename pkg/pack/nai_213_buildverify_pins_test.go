// pkg/pack/nai_213_buildverify_pins_test.go
package pack

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zsrv/goscape/pkg/io/packet"
)

// TestBuildVerifyMagicNumbers_AppearExactlyOnce pins that each archive CRC
// magic is declared exactly once, in pkg/pack/build_verify.go. Guards against
// silent removal or a stray duplicate literal drifting out of sync.
//
// Rev-244 (9aadcec4): interface CRC updated from -2146838800 (225) →
// 316858560 (PackClient.ts:21). Sound CRC updated from -1570057128 (225
// placeholder) to the active value -1415586973.
// Rev-245.2 (3c16994c): interface CRC updated from 316858560 →
// 587792799 (PackClient.ts:19) to reflect swappable + activeovercolour.
// Sound CRC returns to pre-244 value: -1415586973 → -1570057128
// (sound/pack.ts:47 @ 3c16994c).
// Rev-254 (2e3bcf43): interface CRC updated from 587792799 →
// 1728499832 (PackClient.ts:19) alongside script ops 14-20. Sound CRC
// updated from -1570057128 → 831919863 (sound/pack.ts:47 @ 2e3bcf43).
// Rev-274 (dee467c8): interface CRC updated from 1728499832 → 2041671134
// (PackClient.ts:36) alongside the *_full font renames. Sound CRC
// updated from 831919863 → 2127412105 (sound/pack.ts:58 @ dee467c8).
// Rev-274 (8139461a): every gate moved off the pre-save packet and onto the
// saved FILE bytes, so interface went 2041671134 → 2135735991 and sound
// 2127412105 → -759577225. Title (410306098), textures (915347346) and
// wordenc (1386621111) gained gates they never had. All five constants moved
// into pkg/pack/build_verify.go so they sit together with the shared
// VerifyArchive helper.
func TestBuildVerifyMagicNumbers_AppearExactlyOnce(t *testing.T) {
	raw, err := os.ReadFile("build_verify.go")
	if err != nil {
		t.Fatalf("ReadFile build_verify.go: %v", err)
	}
	src := string(raw)

	for _, literal := range []string{
		"410306098",  // title
		"2135735991", // interface
		"915347346",  // textures
		"1386621111", // wordenc
		"-759577225", // sounds
	} {
		if count := strings.Count(src, literal); count != 1 {
			t.Errorf("%q in build_verify.go: count=%d, want 1", literal, count)
		}
	}
}

// TestArchiveCRCMagicsAreDistinct guards the copy-paste failure mode: two
// archives sharing a constant would make one of the gates vacuous.
func TestArchiveCRCMagicsAreDistinct(t *testing.T) {
	magics := map[string]int32{
		"title":     TitleCRCMagic,
		"interface": InterfaceCRCMagic,
		"textures":  TextureCRCMagic,
		"wordenc":   WordencCRCMagic,
		"sounds":    SoundCRCMagic,
	}
	seen := map[int32]string{}
	for name, v := range magics {
		if prev, dup := seen[v]; dup {
			t.Errorf("%s and %s share CRC magic %d", prev, name, v)
		}
		seen[v] = name
	}
}

// TestVerifyArchiveDetectsCorruption pins that the gate is not vacuous: a
// single flipped byte must be reported. BuildVerify is the failing half of
// VerifyArchive (which logs rather than returns), so it is asserted directly.
func TestVerifyArchiveDetectsCorruption(t *testing.T) {
	data := []uint8{1, 2, 3, 4, 5, 6, 7, 8}
	good := int32(packet.GetCRC(data, 0, len(data)))

	if err := BuildVerify(data, len(data), good); err != nil {
		t.Fatalf("BuildVerify on intact data: %v", err)
	}

	corrupt := append([]uint8(nil), data...)
	corrupt[3] ^= 0x01
	if err := BuildVerify(corrupt, len(corrupt), good); err == nil {
		t.Error("BuildVerify on corrupted data: got nil error, want CRC mismatch")
	}
}

// TestBuildVerify_BUILD_VERIFY_NotPresent ensures we don't leak the
// TS env-var name (BUILD_VERIFY) into any client-stage package; all
// CRC gating goes through pack.BuildVerify.
func TestBuildVerify_BUILD_VERIFY_NotPresent(t *testing.T) {
	for _, p := range []string{
		"clientinterface/pack.go",
		"audio/sound.go",
		"sprites/sprites.go",
		"graphics/pack.go",
	} {
		path := filepath.Join(p)
		raw, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		if strings.Contains(string(raw), "BUILD_VERIFY") {
			t.Errorf("%q contains forbidden identifier BUILD_VERIFY", path)
		}
	}
}
