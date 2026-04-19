package packet

import "testing"

func TestP1Alt1(t *testing.T) {
	p := NewPacket(nil)
	p.P1Alt1(5)
	if got := p.Bytes()[0]; got != 133 {
		t.Errorf("P1Alt1(5) = %d, want 133 (5+128)", got)
	}
}

func TestP1Alt2(t *testing.T) {
	p := NewPacket(nil)
	p.P1Alt2(5)
	if got := p.Bytes()[0]; got != 123 {
		t.Errorf("P1Alt2(5) = %d, want 123 (128-5)", got)
	}
}

func TestP1Alt3(t *testing.T) {
	p := NewPacket(nil)
	p.P1Alt3(5)
	if got := p.Bytes()[0]; got != 251 {
		t.Errorf("P1Alt3(5) = %d, want 251 ((-5)&0xff)", got)
	}
}

func TestP2Alt2LittleEndian(t *testing.T) {
	p := NewPacket(nil)
	p.P2Alt2(0x1234)
	got := p.Bytes()
	if got[0] != 0x34 || got[1] != 0x12 {
		t.Errorf("P2Alt2(0x1234) = [%#x %#x], want [0x34 0x12]", got[0], got[1])
	}
}

func TestIP2IsP2Alt2(t *testing.T) {
	p := NewPacket(nil)
	p.IP2(0xABCD)
	got := p.Bytes()
	if got[0] != 0xCD || got[1] != 0xAB {
		t.Errorf("IP2(0xABCD) = [%#x %#x], want [0xCD 0xAB]", got[0], got[1])
	}
}

func TestP4Alt2MiddleEndian(t *testing.T) {
	p := NewPacket(nil)
	p.P4Alt2(0xAABBCCDD)
	got := p.Bytes()
	want := []byte{0xBB, 0xAA, 0xDD, 0xCC}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("P4Alt2[%d] = %#x, want %#x", i, got[i], want[i])
		}
	}
}

func TestPDataAlt1(t *testing.T) {
	p := NewPacket(nil)
	p.PDataAlt1([]byte{1, 2, 3})
	got := p.Bytes()
	for i, want := range []byte{129, 130, 131} {
		if got[i] != want {
			t.Errorf("PDataAlt1[%d] = %d, want %d", i, got[i], want)
		}
	}
}

func TestPDataAlt2(t *testing.T) {
	p := NewPacket(nil)
	p.PDataAlt2([]byte{1, 2, 3})
	got := p.Bytes()
	for i, want := range []byte{127, 126, 125} {
		if got[i] != want {
			t.Errorf("PDataAlt2[%d] = %d, want %d", i, got[i], want)
		}
	}
}
