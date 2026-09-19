package packetcapture

import (
	"testing"

	"github.com/zsrv/goscape/pkg/tapper"
)

func TestCaptureSatisfiesTapper(t *testing.T) {
	var _ tapper.Tapper = New(CaptureOpts{})
	var _ tapper.Tapper = NoopCapture()
}
