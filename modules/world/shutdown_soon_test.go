package world

import (
	"testing"

	"github.com/zsrv/goscape/pkg/io/protocol/login/resp"
)

// TestShutdownSoon_Predicate pins the TS World.shutdownSoon getter
// (Engine-TS/src/engine/World.ts:202-204):
//
//	shutdownTick != -1 && currentTick >= shutdownTick - 50
//
// The 50-tick (~30s at 600ms/tick) pre-shutdown window is what TS
// processLogins (World.ts:884-890) uses to reject new logins —
// world-tick-3 closure.
func TestShutdownSoon_Predicate(t *testing.T) {
	cases := []struct {
		name         string
		shutdownTick int
		currentTick  int
		want         bool
	}{
		{"no shutdown scheduled", -1, 0, false},
		{"no shutdown scheduled, late currentTick", -1, 10_000, false},
		{"scheduled in 100 ticks (outside 50-tick window)", 200, 100, false},
		{"scheduled exactly 51 ticks ahead (boundary, outside)", 151, 100, false},
		{"scheduled exactly 50 ticks ahead (boundary, inside)", 150, 100, true},
		{"scheduled in 1 tick (inside window)", 101, 100, true},
		{"at shutdown tick itself", 100, 100, true},
		{"past shutdown tick", 100, 105, true},
		{"shutdownTick=0, currentTick=0", 0, 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := newTestServer(t)
			s.shutdownTick = tc.shutdownTick
			s.currentTick = tc.currentTick
			if got := s.shutdownSoon(); got != tc.want {
				t.Errorf("shutdownSoon(): got %v, want %v (TS World.ts:202-204: shutdownTick=%d, currentTick=%d)",
					got, tc.want, tc.shutdownTick, tc.currentTick)
			}
		})
	}
}

// TestShutdownSoon_DoesNotFireWhenOnlyPastWindow guards against a
// regression where shutdownSoon would return false at the moment the
// shutdown has armed but currentTick equals shutdownTick (the boundary
// must be inclusive — TS uses `>= shutdownTick - 50`, not `> shutdownTick - 50`).
func TestShutdownSoon_BoundaryInclusive(t *testing.T) {
	s := newTestServer(t)
	s.shutdownTick = 200
	s.currentTick = 150 // shutdownTick - 50 exactly
	if !s.shutdownSoon() {
		t.Errorf("shutdownSoon() at currentTick == shutdownTick-50: got false, want true (boundary is inclusive)")
	}
}

// TestHandleLogin_ShutdownSoonGuardOpcode pins the wire-byte the
// shutdownSoon guard would emit. TS forceLogout(player, 14)
// (Engine-TS/src/engine/World.ts:884-887,1616-1631) writes a single
// raw byte 14 to the client; the Java client renders that as
// "The server is being updated. Please wait 1 minute and try
// again." goscape's OpUpdateInProgress (pkg/io/protocol/login/resp/
// resp.go) carries opcode 14 with the same client-rendered string.
// world-tick-3 closure.
func TestHandleLogin_ShutdownSoonGuardOpcode(t *testing.T) {
	if resp.OpUpdateInProgress.Opcode != 14 {
		t.Errorf("OpUpdateInProgress.Opcode drift: got %d, want 14 (TS forceLogout response byte)",
			resp.OpUpdateInProgress.Opcode)
	}
}
