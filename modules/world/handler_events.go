package world

import (
	"uuid"

	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/zsrv/goscape/pkg/eventspb"
	"github.com/zsrv/goscape/pkg/io/packet"
	"github.com/zsrv/goscape/pkg/telemetry"
)

// 254 client event packets. The windowed EVENT_TRACKING upload was
// replaced upstream by four discrete event packets appended into the
// InputTracking accumulation buffer (see input_tracking.go). Each
// handler is a thin decode+dispatch mirroring its one-line TS handler
// (`player.input.<method>(message)`); all recording gates (Active,
// overflow flush) live in InputTracking. The login reply's third byte
// (=1) is what tells the 254 client to send these events.

// handleEventMouseClick handles EVENT_MOUSE_CLICK (opcode 234, 4
// bytes). Decode per TS EventMouseClickDecoder.ts @43e02957: info = g4.
// Dispatch per EventMouseClickHandler.ts: player.input.mouseClick.
func handleEventMouseClick(p *Player, payload []byte) error {
	if p.input == nil {
		return nil
	}
	buf := packet.NewPacket(payload)
	p.input.MouseClick(buf.G4())
	return nil
}

// handleEventMouseMove handles EVENT_MOUSE_MOVE (opcode 232, 1-byte
// length prefix). Decode per TS EventMouseMoveDecoder.ts @43e02957:
// data = the remaining payload bytes (gdata of len). Dispatch per
// EventMouseMoveHandler.ts: player.input.mouseMove.
func handleEventMouseMove(p *Player, payload []byte) error {
	if p.input == nil {
		return nil
	}
	p.input.MouseMove(payload)

	// NAI-Phase2 seam (carried from the retired EVENT_TRACKING handler):
	// emit MouseMoveEvent. Gated on input.Active to preserve the old
	// seam's posture (it fired only once the recording gates passed) and
	// to avoid per-packet work for untracked players. The payload is an
	// opaque packed format; the TS engine likewise treats the bytes as
	// opaque and never decodes mouse coords server-side, so X/Y stay zero
	// for TS-parity — the canonical mouse trail lives in the raw blob
	// accumulated by p.input.
	if p.input.Active && p.client != nil && p.client.server != nil {
		telemetry.Get().EmitPlayerInput(&eventspb.PlayerInputEnvelope{
			SchemaVersion: 1,
			EventId:       uuid.New().String(),
			Ts:            timestamppb.Now(),
			WorldId:       int32(p.client.server.cfg.NodeID),
			AccountId:     p.accountID,
			Payload: &eventspb.PlayerInputEnvelope_MouseMove{
				MouseMove: &eventspb.MouseMoveEvent{X: 0, Y: 0},
			},
		})
	}
	return nil
}

// handleEventAppletFocus handles EVENT_APPLET_FOCUS (opcode 8, 1 byte).
// Decode per TS EventAppletFocusDecoder.ts @43e02957: focus = g1.
// Dispatch per EventAppletFocusHandler.ts: player.input.appletFocus.
func handleEventAppletFocus(p *Player, payload []byte) error {
	if p.input == nil {
		return nil
	}
	buf := packet.NewPacket(payload)
	p.input.AppletFocus(buf.G1())
	return nil
}

// handleEventCameraPosition handles EVENT_CAMERA_POSITION (opcode 91,
// 4 bytes). Decode per TS EventCameraPositionDecoder.ts @43e02957:
// pitch = g2, yaw = g2. Dispatch per EventCameraPositionHandler.ts:
// player.input.cameraPosition.
func handleEventCameraPosition(p *Player, payload []byte) error {
	if p.input == nil {
		return nil
	}
	buf := packet.NewPacket(payload)
	pitch := buf.G2()
	yaw := buf.G2()
	p.input.CameraPosition(pitch, yaw)
	return nil
}
