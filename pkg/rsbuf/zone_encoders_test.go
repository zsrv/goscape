package rsbuf

import (
	"bytes"
	"testing"

	"github.com/zsrv/goscape/pkg/io/packet"
)

func TestEncodeLocAddChange(t *testing.T) {
	buf := packet.NewPacket(nil)
	// coord=0x62, shape=10(0b01010), angle=3 → packed=(10<<2)|3=0x2B
	// locID=5000=0x1388
	EncodeLocAddChange(buf, 0x62, 10, 3, 5000)
	want := []byte{0x62, 0x2B, 0x13, 0x88}
	if !bytes.Equal(buf.Data, want) {
		t.Errorf("got %v, want %v", buf.Data, want)
	}
}

func TestEncodeLocAnim(t *testing.T) {
	buf := packet.NewPacket(nil)
	EncodeLocAnim(buf, 0x00, 0, 0, 1)
	want := []byte{0, 0, 0, 1}
	if !bytes.Equal(buf.Data, want) {
		t.Errorf("got %v, want %v", buf.Data, want)
	}
}

func TestEncodeLocDel(t *testing.T) {
	buf := packet.NewPacket(nil)
	// shape=2, angle=1 → packed=(2<<2)|1=0x09
	EncodeLocDel(buf, 0x77, 2, 1)
	want := []byte{0x77, 0x09}
	if !bytes.Equal(buf.Data, want) {
		t.Errorf("got %v, want %v", buf.Data, want)
	}
}

func TestEncodeLocMerge(t *testing.T) {
	buf := packet.NewPacket(nil)
	// coord=0x42, shape=1, angle=0 → packed=0x04
	// locID=100=0x0064, startCycle=10=0x000A, endCycle=20=0x0014
	// playerSlot=3=0x0003, dxEast=2, dzSouth=2, dxWest=2, dzNorth=2
	EncodeLocMerge(buf, 0x42, 1, 0, 100, 10, 20, 3, 2, 2, 2, 2)
	want := []byte{
		0x42, 0x04,
		0x00, 0x64,
		0x00, 0x0A,
		0x00, 0x14,
		0x00, 0x03,
		2, 2, 2, 2,
	}
	if !bytes.Equal(buf.Data, want) {
		t.Errorf("got %v, want %v", buf.Data, want)
	}
}

func TestEncodeMapAnim(t *testing.T) {
	buf := packet.NewPacket(nil)
	// spotanim=200=0x00C8, delay=50=0x0032
	EncodeMapAnim(buf, 0x11, 200, 5, 50)
	want := []byte{0x11, 0x00, 0xC8, 0x05, 0x00, 0x32}
	if !bytes.Equal(buf.Data, want) {
		t.Errorf("got %v, want %v", buf.Data, want)
	}
}

func TestEncodeMapProjAnim(t *testing.T) {
	buf := packet.NewPacket(nil)
	// coord=0x00, dx=5, dz=10, target=0, spotanim=1,
	// srcHeight=2, dstHeight=3, startDelay=4, endDelay=5,
	// peak=6, arc=7
	EncodeMapProjAnim(buf, 0x00, 5, 10, 0, 1, 2, 3, 4, 5, 6, 7)
	want := []byte{
		0x00,
		5, 10,
		0x00, 0x00, // target
		0x00, 0x01, // spotanim
		2, 3,       // srcHeight, dstHeight
		0x00, 0x04, // startDelay
		0x00, 0x05, // endDelay
		6, 7,       // peak, arc
	}
	if !bytes.Equal(buf.Data, want) {
		t.Errorf("got %v, want %v", buf.Data, want)
	}
}

func TestEncodeMapProjAnimSignedDeltas(t *testing.T) {
	buf := packet.NewPacket(nil)
	// dx=-5 → byte 0xFB; dz=-10 → byte 0xF6
	EncodeMapProjAnim(buf, 0, -5, -10, 0, 0, 0, 0, 0, 0, 0, 0)
	if buf.Data[1] != 0xFB {
		t.Errorf("dx=-5 byte: got %#x, want 0xFB", buf.Data[1])
	}
	if buf.Data[2] != 0xF6 {
		t.Errorf("dz=-10 byte: got %#x, want 0xF6", buf.Data[2])
	}
}

func TestEncodeObjAdd(t *testing.T) {
	buf := packet.NewPacket(nil)
	// obj=4151=0x1037, count=1
	EncodeObjAdd(buf, 0x45, 4151, 1)
	want := []byte{0x45, 0x10, 0x37, 0x00, 0x01}
	if !bytes.Equal(buf.Data, want) {
		t.Errorf("got %v, want %v", buf.Data, want)
	}
}

func TestEncodeObjCount(t *testing.T) {
	buf := packet.NewPacket(nil)
	// Both counts within range.
	EncodeObjCount(buf, 0x10, 995, 100, 200)
	want := []byte{
		0x10,
		0x03, 0xE3, // 995
		0x00, 0x64, // 100
		0x00, 0xC8, // 200
	}
	if !bytes.Equal(buf.Data, want) {
		t.Errorf("got %v, want %v", buf.Data, want)
	}
}

func TestEncodeObjCountZero(t *testing.T) {
	buf := packet.NewPacket(nil)
	EncodeObjCount(buf, 0, 1, 0, 5)
	want := []byte{0, 0, 1, 0, 0, 0, 5}
	if !bytes.Equal(buf.Data, want) {
		t.Errorf("got %v, want %v", buf.Data, want)
	}
}

func TestEncodeObjDel(t *testing.T) {
	buf := packet.NewPacket(nil)
	EncodeObjDel(buf, 0x33, 1)
	want := []byte{0x33, 0x00, 0x01}
	if !bytes.Equal(buf.Data, want) {
		t.Errorf("got %v, want %v", buf.Data, want)
	}
}

func TestEncodeObjReveal(t *testing.T) {
	buf := packet.NewPacket(nil)
	// obj=995, count=100, receiverID=42
	EncodeObjReveal(buf, 0x20, 995, 100, 42)
	want := []byte{
		0x20,
		0x03, 0xE3, // obj=995
		0x00, 0x64, // count=100
		0x00, 0x2A, // receiverID=42
	}
	if !bytes.Equal(buf.Data, want) {
		t.Errorf("got %v, want %v", buf.Data, want)
	}
}

func TestCountClampingAtBoundary(t *testing.T) {
	// 65535 → 0xFF 0xFF unchanged
	buf := packet.NewPacket(nil)
	EncodeObjAdd(buf, 0, 1, 65535)
	if buf.Data[3] != 0xFF || buf.Data[4] != 0xFF {
		t.Errorf("count=65535: got %v, want 0xFF 0xFF tail", buf.Data)
	}

	// 65536 → clamped to 0xFF 0xFF
	buf2 := packet.NewPacket(nil)
	EncodeObjAdd(buf2, 0, 1, 65536)
	if buf2.Data[3] != 0xFF || buf2.Data[4] != 0xFF {
		t.Errorf("count=65536 should clamp; got %v", buf2.Data)
	}

	// ObjCount: both clamped
	buf3 := packet.NewPacket(nil)
	EncodeObjCount(buf3, 0, 1, 70000, 80000)
	want3 := []byte{0, 0, 1, 0xFF, 0xFF, 0xFF, 0xFF}
	if !bytes.Equal(buf3.Data, want3) {
		t.Errorf("oldCount+newCount clamp: got %v, want %v", buf3.Data, want3)
	}

	// ObjReveal: count clamped, receiverID not
	buf4 := packet.NewPacket(nil)
	EncodeObjReveal(buf4, 0, 1, 100000, 42)
	if buf4.Data[3] != 0xFF || buf4.Data[4] != 0xFF {
		t.Errorf("ObjReveal count clamp: got %v", buf4.Data)
	}
}

// Outer-encoder header math: dx = (zoneX<<3) - ZoneOrigin(originX).
// For originX=3094: Zone(3094)=386; ZoneCenter(386)=380; ZoneOrigin=380<<3=3040.
// For zoneX=386: dx = (386<<3) - 3040 = 3088 - 3040 = 48.
// (Previously the spec guessed 16 — recompute from the actual ZoneOrigin formula.)

func TestEncodeZoneFullFollowsHeader(t *testing.T) {
	buf := packet.NewPacket(nil)
	EncodeZoneFullFollows(buf, 386, 388, 3094, 3106)
	// originX=3094 → ZoneOrigin = (((3094>>3) - 6) << 3) = ((386-6)<<3) = 380<<3 = 3040
	// dx = (386<<3) - 3040 = 3088 - 3040 = 48
	// originZ=3106 → ZoneOrigin = (((3106>>3) - 6) << 3) = ((388-6)<<3) = 382<<3 = 3056
	// dz = (388<<3) - 3056 = 3104 - 3056 = 48
	want := []byte{48, 48}
	if !bytes.Equal(buf.Data, want) {
		t.Errorf("got %v, want %v", buf.Data, want)
	}
}

func TestEncodeZonePartialFollowsHeader(t *testing.T) {
	buf := packet.NewPacket(nil)
	EncodeZonePartialFollows(buf, 386, 388, 3094, 3106)
	if len(buf.Data) != 2 {
		t.Errorf("len: got %d, want 2", len(buf.Data))
	}
	if buf.Data[0] != 48 || buf.Data[1] != 48 {
		t.Errorf("bytes: got %v, want [48 48]", buf.Data)
	}
}

func TestEncodeZonePartialEnclosedAppendsData(t *testing.T) {
	buf := packet.NewPacket(nil)
	data := []byte{0xAA, 0xBB, 0xCC}
	EncodeZonePartialEnclosed(buf, 386, 388, 3094, 3106, data)
	want := []byte{48, 48, 0xAA, 0xBB, 0xCC}
	if !bytes.Equal(buf.Data, want) {
		t.Errorf("got %v, want %v", buf.Data, want)
	}
}
