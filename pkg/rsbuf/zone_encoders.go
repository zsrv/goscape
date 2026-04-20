package rsbuf

import "github.com/zsrv/goscape/pkg/io/packet"

// Zone-nested opcode constants. Written by the zone subsystem (sub-spec 4b-3)
// as a single byte before each encoder's payload when composing the shared
// buffer delivered via UpdateZonePartialEnclosed.
const (
	ZoneOpLocMerge     = 23
	ZoneOpLocAnim      = 42
	ZoneOpObjDel       = 49
	ZoneOpObjReveal    = 50
	ZoneOpLocAddChange = 59
	ZoneOpMapProjAnim  = 69
	ZoneOpLocDel       = 76
	ZoneOpObjCount     = 151
	ZoneOpMapAnim      = 191
	ZoneOpObjAdd       = 223
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
