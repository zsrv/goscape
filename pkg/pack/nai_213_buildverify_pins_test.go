// pkg/pack/nai_213_buildverify_pins_test.go
package pack

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestBuildVerifyMagicNumbers_AppearExactlyOnce pins that the two CRC
// magic numbers from TS PackClient.ts and sound/pack.ts appear exactly
// once each, in their expected locations. Guards against silent removal
// or duplication.
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
func TestBuildVerifyMagicNumbers_AppearExactlyOnce(t *testing.T) {
	tests := []struct {
		file    string
		literal string
	}{
		{"clientinterface/pack.go", "1728499832"},
		{"audio/sound.go", "831919863"},
	}
	for _, tc := range tests {
		path := filepath.Join(tc.file)
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("ReadFile %q: %v", path, err)
			continue
		}
		count := strings.Count(string(raw), tc.literal)
		if count != 1 {
			t.Errorf("%q in %q: count=%d, want 1", tc.literal, path, count)
		}
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
