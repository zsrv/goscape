package build

import (
	"runtime"
	"strings"
	"testing"
)

func TestVersionString(t *testing.T) {
	Version, Revision, Branch = "v1", "abc1234", "rev-274"
	got := String()
	for _, want := range []string{"v1", "abc1234", "rev-274", runtime.Version()} {
		if !strings.Contains(got, want) {
			t.Errorf("String() = %q missing %q", got, want)
		}
	}
}
