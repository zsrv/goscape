// pkg/pack/nai_213_buildverify_pins_test.go
package pack

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestBuildVerifyMagicNumbers_AppearExactlyOnce pins that the two CRC
// magic numbers from TS PackClient.ts:16 and sound/pack.ts:46 appear
// exactly once each, in their expected locations. Guards against
// silent removal or duplication.
func TestBuildVerifyMagicNumbers_AppearExactlyOnce(t *testing.T) {
	tests := []struct {
		file    string
		literal string
	}{
		{"clientinterface/pack.go", "-2146838800"},
		{"audio/sound.go", "-1570057128"},
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
