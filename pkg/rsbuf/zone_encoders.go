package rsbuf

import (
	"github.com/zsrv/goscape/pkg/coordgrid"
	"github.com/zsrv/goscape/pkg/io/packet"
)

// Zone-nested opcode constants. Written by the zone subsystem (sub-spec 4b-3)
// as a single byte before each encoder's payload when composing the shared
// buffer delivered via UpdateZonePartialEnclosed (opcode 233, -2).
// Values mirror ServerGameZoneProt.ts at Engine-TS rev 244 (9aadcec4).
// Must stay in sync with Op* vars in pkg/io/protocol/game/server (pinned by
// TestZoneOpConsistency244 in zone_encoders_test.go).
const (
	ZoneOpLocMerge     = 29
	ZoneOpLocAnim      = 155
	ZoneOpObjDel       = 39
	ZoneOpObjReveal    = 69
	ZoneOpLocAddChange = 232
	ZoneOpMapProjAnim  = 137
	ZoneOpLocDel       = 125
	ZoneOpObjCount     = 209
	ZoneOpMapAnim      = 198
	ZoneOpObjAdd       = 234
)

// packLocShapeAngle returns (shape<<2)|(angle&0x3), the common second byte
// of every LOC_* zone-nested packet.
func packLocShapeAngle(shape, angle int) byte {
	return byte((shape << 2) | (angle & 0x3))
}

// clampU16 clamps a non-negative int count to the [0, 65535] wire range.
func clampU16(n int) uint16 {
	if n < 0 {
		return 0
	}
	if n > 65535 {
		return 65535
	}
	return uint16(n)
}

// --- LOC_* ---

// EncodeLocAddChange writes the 4-byte LOC_ADD_CHANGE payload.
func EncodeLocAddChange(buf *packet.Packet, coord byte, shape, angle, locID int) {
	buf.P1(coord)
	buf.P1(packLocShapeAngle(shape, angle))
	buf.P2(uint16(locID))
}

// EncodeLocAnim writes the 4-byte LOC_ANIM payload.
func EncodeLocAnim(buf *packet.Packet, coord byte, shape, angle, seq int) {
	buf.P1(coord)
	buf.P1(packLocShapeAngle(shape, angle))
	buf.P2(uint16(seq))
}

// EncodeLocDel writes the 2-byte LOC_DEL payload.
func EncodeLocDel(buf *packet.Packet, coord byte, shape, angle int) {
	buf.P1(coord)
	buf.P1(packLocShapeAngle(shape, angle))
}

// EncodeLocMerge writes the 14-byte LOC_MERGE payload for a multi-tile
// NPC standing on a spatially-merged loc. Deltas are relative to srcX/srcZ.
func EncodeLocMerge(
	buf *packet.Packet,
	coord byte,
	shape, angle, locID int,
	startCycle, endCycle, playerSlot int,
	dxEast, dzSouth, dxWest, dzNorth int,
) {
	buf.P1(coord)
	buf.P1(packLocShapeAngle(shape, angle))
	buf.P2(uint16(locID))
	buf.P2(uint16(startCycle))
	buf.P2(uint16(endCycle))
	buf.P2(uint16(playerSlot))
	buf.P1(byte(dxEast))
	buf.P1(byte(dzSouth))
	buf.P1(byte(dxWest))
	buf.P1(byte(dzNorth))
}

// --- MAP_* ---

// EncodeMapAnim writes the 6-byte MAP_ANIM payload.
func EncodeMapAnim(buf *packet.Packet, coord byte, spotanim, height, delay int) {
	buf.P1(coord)
	buf.P2(uint16(spotanim))
	buf.P1(byte(height))
	buf.P2(uint16(delay))
}

// EncodeMapProjAnim writes the 15-byte MAP_PROJANIM payload. dx/dz are the
// signed tile delta (dst - src) and must each fit in a signed i8 (|delta|<=127).
// target: 0=coord; >0=npc+1; <0=-(player slot)-1.
func EncodeMapProjAnim(
	buf *packet.Packet,
	coord byte,
	dx, dz int,
	target, spotanim int,
	srcHeight, dstHeight int,
	startDelay, endDelay int,
	peak, arc int,
) {
	buf.P1(coord)
	buf.P1(byte(dx))
	buf.P1(byte(dz))
	buf.P2(uint16(target))
	buf.P2(uint16(spotanim))
	buf.P1(byte(srcHeight))
	buf.P1(byte(dstHeight))
	buf.P2(uint16(startDelay))
	buf.P2(uint16(endDelay))
	buf.P1(byte(peak))
	buf.P1(byte(arc))
}

// --- OBJ_* ---

// EncodeObjAdd writes the 5-byte OBJ_ADD payload. count is clamped to u16.
func EncodeObjAdd(buf *packet.Packet, coord byte, obj, count int) {
	buf.P1(coord)
	buf.P2(uint16(obj))
	buf.P2(clampU16(count))
}

// EncodeObjCount writes the 7-byte OBJ_COUNT payload. Both counts clamped.
func EncodeObjCount(buf *packet.Packet, coord byte, obj, oldCount, newCount int) {
	buf.P1(coord)
	buf.P2(uint16(obj))
	buf.P2(clampU16(oldCount))
	buf.P2(clampU16(newCount))
}

// EncodeObjDel writes the 3-byte OBJ_DEL payload.
func EncodeObjDel(buf *packet.Packet, coord byte, obj int) {
	buf.P1(coord)
	buf.P2(uint16(obj))
}

// EncodeObjReveal writes the 7-byte OBJ_REVEAL payload. count is clamped;
// receiverID is the original dropper's player slot (NOT clamped — it's a u16).
func EncodeObjReveal(buf *packet.Packet, coord byte, obj, count, receiverID int) {
	buf.P1(coord)
	buf.P2(uint16(obj))
	buf.P2(clampU16(count))
	buf.P2(uint16(receiverID))
}

// --- Outer zone packets ---

// zoneRelHeader writes the 2-byte zone-relative header: the first byte is
// (zoneX<<3) - ZoneOrigin(originX) and the second is the same for z.
// ZoneOrigin produces the build-area origin's mapsquare base, so the result
// is a small signed offset that fits in one byte.
func zoneRelHeader(buf *packet.Packet, zoneX, zoneZ, originX, originZ int) {
	buf.P1(byte((zoneX << 3) - coordgrid.ZoneOrigin(originX)))
	buf.P1(byte((zoneZ << 3) - coordgrid.ZoneOrigin(originZ)))
}

// EncodeZoneFullFollows writes the 2-byte header for the outer UpdateZoneFullFollows
// packet (opcode 131, fixed 2). The opcode is emitted by writeOut.
func EncodeZoneFullFollows(buf *packet.Packet, zoneX, zoneZ, originX, originZ int) {
	zoneRelHeader(buf, zoneX, zoneZ, originX, originZ)
}

// EncodeZonePartialFollows writes the 2-byte header for the outer
// UpdateZonePartialFollows packet (opcode 94, fixed 2).
func EncodeZonePartialFollows(buf *packet.Packet, zoneX, zoneZ, originX, originZ int) {
	zoneRelHeader(buf, zoneX, zoneZ, originX, originZ)
}

// EncodeZonePartialEnclosed writes the 2-byte header followed by the
// precomputed shared-data bytes for UpdateZonePartialEnclosed (opcode 233, -2).
func EncodeZonePartialEnclosed(buf *packet.Packet, zoneX, zoneZ, originX, originZ int, data []byte) {
	zoneRelHeader(buf, zoneX, zoneZ, originX, originZ)
	buf.PData(data)
}
