package tapper

import (
	"testing"
	"time"
)

func TestNoopTapperImplementsTapper(t *testing.T) {
	var tp Tapper = NoopTapper()
	tp.SessionStarted(1, "s", time.Unix(0, 0))
	tp.Tap(1, "s", DirOut, 0, nil, time.Unix(0, 0))
	tp.SessionEnded(1, "s", time.Unix(0, 0), CloseReasonDisconnect)
}

func TestImplSatisfiesTapper(t *testing.T) {
	var _ Tapper = NoopTapper() // compile-time assertion
}
