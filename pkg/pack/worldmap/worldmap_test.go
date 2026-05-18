package worldmap

import (
	"testing"

	packet2 "github.com/zsrv/goscape/pkg/io/packet"
	"github.com/zsrv/goscape/pkg/objtype"
)

func TestUnpackCoord(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		level, x, z int
	}{
		{0, 0, 0},
		{0, 63, 63},
		{1, 30, 17},
		{3, 63, 63},
	} {
		packed := (tc.level << 12) | (tc.x << 6) | tc.z
		level, x, z := unpackCoord(packed)
		if level != tc.level || x != tc.x || z != tc.z {
			t.Errorf("unpackCoord(%#x) = (%d, %d, %d), want (%d, %d, %d)",
				packed, level, x, z, tc.level, tc.x, tc.z)
		}
	}
}

func TestPackWater_ByteLayout(t *testing.T) {
	t.Parallel()

	flo := &objtype.FloTypeConfigs{
		ConfigNames: map[string]int{
			"muddygrass": 7,
			"water":      11,
		},
	}

	underlay := packet2.Alloc(1)
	defer underlay.Release()
	overlay := packet2.Alloc(1)
	defer overlay.Release()

	packWater(flo, underlay, overlay, 42, 56)

	if got, want := underlay.Length(), 2+4096; got != want {
		t.Errorf("underlay length = %d, want %d", got, want)
	}
	if got, want := overlay.Length(), 2+4096*2; got != want {
		t.Errorf("overlay length = %d, want %d", got, want)
	}

	underlay.Pos = 0
	if underlay.G1() != 42 || underlay.G1() != 56 {
		t.Errorf("underlay header bytes wrong")
	}
	for i := range 4096 {
		if got := underlay.G1(); got != 8 {
			t.Fatalf("underlay body byte %d = %d, want 8", i, got)
		}
	}

	overlay.Pos = 0
	if overlay.G1() != 42 || overlay.G1() != 56 {
		t.Errorf("overlay header bytes wrong")
	}
	for i := range 4096 {
		v := overlay.G1()
		z := overlay.G1()
		if v != 12 || z != 0 {
			t.Fatalf("overlay body pair %d = (%d, %d), want (12, 0)", i, v, z)
		}
	}
}
