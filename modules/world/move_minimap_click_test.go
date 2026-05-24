package world

import (
	"testing"

	"github.com/zsrv/goscape/pkg/coordgrid"
)

// MOVE_MINIMAPCLICK (opcode 165) is a variable-length client packet that
// carries a minimap click target plus an optional list of dx/dz waypoints,
// terminated by a 14-byte trailer the client appends for this opcode only.
// See Engine-TS MoveClickDecoder.ts:16 — "const offset = prot ===
// MOVE_MINIMAPCLICK ? 14 : 0;" — the trailer is documented but its
// content is unused by both the TS server and goscape.
//
// Wire layout: ctrlHeld G1 (1) + startX G2 (2) + startZ G2 (2) +
// waypoints (N * 2 dx/dz bytes) + trailer (14 bytes). Minimum useful
// payload is therefore 5 + 14 = 19 bytes (zero waypoints).
//
// goscape handler: handlers_game.go:1284 handleMoveMinimapClick.

// TestHandleMoveMinimapClick_ClosesModal pins the reported bug: walking
// away via the minimap must close an open modal (e.g. the bank), exactly
// like MOVE_GAMECLICK does. The bank is a MAIN modal (modalMain /
// modalStateMain), so this opens one and asserts it closes.
// MOVE_MINIMAPCLICK decodes to opClick=false, so TS MoveClickHandler.ts:
// !opClick → clearPendingAction() → closeModal() fires. Go routes minimap
// through the same moveClickInner as game-click.
func TestHandleMoveMinimapClick_ClosesModal(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.x, p.z, p.level = 3094, 3106, 0
	p.client.server = newTestServer(t)

	// Open a main modal (the bank interface lives in modalMain).
	p.modalMain = 5292 // bank component id (arbitrary non-negative)
	p.modalState |= modalStateMain

	// Minimap payload: header (ctrlHeld=0, startX=3100, startZ=3110) + 14
	// content-irrelevant trailer bytes.
	payload := append(buildMovePayload(0, 3100, 3110), make([]byte, 14)...)

	if err := handleMoveMinimapClick(p, payload); err != nil {
		t.Fatalf("handleMoveMinimapClick: %v", err)
	}

	if p.modalMain != -1 {
		t.Errorf("modalMain: got %d, want -1 (minimap walk must close the bank modal via ClearPendingAction)", p.modalMain)
	}
	if p.modalState&modalStateMain != modalStateNone {
		t.Errorf("modalState main bit still set: got 0x%x (minimap walk must clear it)", p.modalState)
	}
}

// TestHandleMoveMinimapClick_MinimumPayloadQueuesDest pins the happy path
// for a payload with zero waypoints (5 header bytes + 14 trailer bytes =
// 19 bytes). The handler should queue a single waypoint at (startX,
// startZ) via pathToMoveClick's Smart-no-finding fallback.
func TestHandleMoveMinimapClick_MinimumPayloadQueuesDest(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.x, p.z, p.level = 3094, 3106, 0
	// moveClickInner needs a server (gamemap stays nil, so pathToMoveClick's
	// Smart branch falls through to queueWaypoints(packed) — same observable
	// queue as before the minimap/game-click unification).
	p.client.server = newTestServer(t)

	// Header: ctrlHeld=0, startX=3100, startZ=3110.
	// 14-byte trailer is content-irrelevant (zeroed here).
	payload := []byte{
		0,          // ctrlHeld
		0x0C, 0x1C, // startX = 3100
		0x0C, 0x26, // startZ = 3110
	}
	// Append 14 zero trailer bytes.
	payload = append(payload, make([]byte, 14)...)

	if err := handleMoveMinimapClick(p, payload); err != nil {
		t.Fatalf("handleMoveMinimapClick returned error: %v", err)
	}

	if p.waypointIndex != 0 {
		t.Fatalf("waypointIndex: got %d, want 0 (single dest queued)", p.waypointIndex)
	}
	got := coordgrid.UnpackCoord(p.waypoints[0])
	if got.X != 3100 || got.Z != 3110 {
		t.Errorf("queued dest: got (%d,%d), want (3100,3110)", got.X, got.Z)
	}
}

// TestHandleMoveMinimapClick_WaypointsPreservedAcrossTrailer pins that
// the 14-byte trailer is correctly skipped: dx/dz pairs before the
// trailer are parsed into the waypoint queue, and trailer bytes are NOT
// mis-read as extra waypoints. Mirrors TS MoveClickDecoder.ts:17 —
// "waypoints = (length - buf.pos - offset) / 2".
//
// queueWaypoints stores the path REVERSED (waypoints.go:25-36): index 0
// holds the dest, index waypointIndex holds the first step. So with
// startX/Z + 2 dx/dz pairs, we expect waypointIndex == 2 and
// waypoints[0] == the LAST dx/dz applied.
func TestHandleMoveMinimapClick_WaypointsPreservedAcrossTrailer(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.x, p.z, p.level = 3094, 3106, 0
	p.client.server = newTestServer(t)
	// Full-path preservation is a routefinder-mode property: TS only copies
	// the whole client path into userPath when NODE_CLIENT_ROUTEFINDER is set
	// (MoveClickHandler.ts:22-32); the non-routefinder branch collapses it to
	// [dest]. The zero-value test cfg leaves routefinder off, so opt in here so
	// the start + 2 dx/dz steps survive into the waypoint queue (M24).
	p.client.server.cfg.NodeClientRoutefinder = true

	// Header: ctrlHeld=1, startX=3100, startZ=3110.
	// 2 dx/dz pairs: (+1,+0) → (3101,3110), (+2,+1) → (3102,3111).
	// 14-byte trailer of sentinel 0x7F (= +127): if the handler ever
	// confused these with waypoints, the resulting coords would walk
	// far off (3102+127, 3111+127) and the assertion would catch it.
	payload := []byte{
		1,          // ctrlHeld
		0x0C, 0x1C, // startX = 3100
		0x0C, 0x26, // startZ = 3110
		0x01, 0x00, // dx=+1, dz=0
		0x02, 0x01, // dx=+2, dz=+1
	}
	trailer := make([]byte, 14)
	for i := range trailer {
		trailer[i] = 0x7F
	}
	payload = append(payload, trailer...)

	if err := handleMoveMinimapClick(p, payload); err != nil {
		t.Fatalf("handleMoveMinimapClick returned error: %v", err)
	}

	// waypointIndex == 2 → 3 entries (start + 2 dx/dz steps).
	if p.waypointIndex != 2 {
		t.Fatalf("waypointIndex: got %d, want 2 (start + 2 dx/dz steps)", p.waypointIndex)
	}
	// waypoints[0] is the LAST coord (queueWaypoints reverses on copy).
	dest := coordgrid.UnpackCoord(p.waypoints[0])
	if dest.X != 3102 || dest.Z != 3111 {
		t.Errorf("dest (waypoints[0]): got (%d,%d), want (3102,3111) — trailer bytes must not be parsed as waypoints", dest.X, dest.Z)
	}
	// waypoints[2] is the start (first step in TS-faithful order).
	start := coordgrid.UnpackCoord(p.waypoints[2])
	if start.X != 3100 || start.Z != 3110 {
		t.Errorf("start (waypoints[2]): got (%d,%d), want (3100,3110)", start.X, start.Z)
	}
}

// TestHandleMoveMinimapClick_ShortPayloadIsNoop pins the 14-trailing-byte
// invariant: any payload shorter than 5 + 14 = 19 bytes is silently
// dropped. The handler does NOT panic on short payload (unlike
// handleResumeCountDialog's G4-EOF contract) — it returns nil and leaves
// waypointIndex at its sentinel -1 from newPlayer.
func TestHandleMoveMinimapClick_ShortPayloadIsNoop(t *testing.T) {
	cases := []struct {
		name string
		size int
	}{
		{"empty", 0},
		{"header_only_no_trailer", 5}, // 5 header bytes, no trailer
		{"one_byte_short", 18},        // one byte shy of minimum
		{"header_plus_partial_trailer", 5 + 13},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p, _ := newTestPlayer(t)
			p.x, p.z, p.level = 3094, 3106, 0
			p.client.server = newTestServer(t)
			// Sentinel: newPlayer leaves waypointIndex at -1.
			if p.waypointIndex != -1 {
				t.Fatalf("setup: waypointIndex pre-call: got %d, want -1", p.waypointIndex)
			}

			payload := make([]byte, tc.size)
			if err := handleMoveMinimapClick(p, payload); err != nil {
				t.Fatalf("handler returned error: %v", err)
			}

			if p.waypointIndex != -1 {
				t.Errorf("waypointIndex post short-payload: got %d, want -1 (handler must early-return on len(payload) < 19)", p.waypointIndex)
			}
		})
	}
}

// TestHandleMoveMinimapClick_PathLengthCappedAt24 pins the cap from
// handlers_game.go:1294 / 1297 — "min((len(payload)-5-trailingBytes)/2,
// 24)". Even if the client sends 50 dx/dz pairs, the handler must
// process at most 24 (plus the start coord = 25 packed entries in).
// queueWaypoints further bounds the storage at len(p.waypoints), so the
// observable cap is whichever is smaller. Mirrors TS MoveClickDecoder.ts:21
// — "index <= waypoints && index < 25".
func TestHandleMoveMinimapClick_PathLengthCappedAt24(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.x, p.z, p.level = 3094, 3106, 0
	p.client.server = newTestServer(t)

	// 50 dx/dz pairs (= 100 bytes of waypoint data). Handler should
	// consume only the first 24 from the wire (i.e. read 48 bytes) and
	// stop, regardless of how many remain before the trailer.
	header := []byte{
		0,          // ctrlHeld
		0x0C, 0x1C, // startX = 3100
		0x0C, 0x26, // startZ = 3110
	}
	waypoints := make([]byte, 50*2)
	// Fill so any over-read past 24 pairs would alter the stored dest.
	for i := range waypoints {
		waypoints[i] = 1
	}
	trailer := make([]byte, 14)

	payload := append(header, waypoints...)
	payload = append(payload, trailer...)

	if err := handleMoveMinimapClick(p, payload); err != nil {
		t.Fatalf("handleMoveMinimapClick returned error: %v", err)
	}

	// 24 dx/dz pairs + start = 25 packed entries → waypointIndex
	// bounded by len(p.waypoints). Whichever bound bites, it must be
	// > 0 (path queued) and <= 24.
	if p.waypointIndex < 0 {
		t.Fatalf("waypointIndex: got %d, want >= 0 (path must be queued)", p.waypointIndex)
	}
	if p.waypointIndex > 24 {
		t.Errorf("waypointIndex: got %d, want <= 24 (path length capped at 24 dx/dz pairs + start)", p.waypointIndex)
	}
}
