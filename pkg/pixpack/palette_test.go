package pixpack

import "testing"

func TestGeneratePalette_SentinelFirst(t *testing.T) {
	bm := &Bitmap{Width: 2, Height: 1, Data: []uint8{0xff, 0x00, 0xff, 0xff, 1, 2, 3, 0xff}}
	colors := generatePalette(bm)
	if len(colors) != 2 {
		t.Fatalf("len=%d, want 2", len(colors))
	}
	if colors[0] != 0xff00ff {
		t.Errorf("colors[0]=%x, want ff00ff", colors[0])
	}
	if colors[1] != 0x010203 {
		t.Errorf("colors[1]=%x, want 010203", colors[1])
	}
}

func TestGeneratePalette_DedupNonSentinel(t *testing.T) {
	bm := &Bitmap{Width: 3, Height: 1, Data: []uint8{
		1, 2, 3, 0xff,
		1, 2, 3, 0xff,
		4, 5, 6, 0xff,
	}}
	colors := generatePalette(bm)
	if len(colors) != 3 {
		t.Errorf("len=%d, want 3 (sentinel + 2 unique)", len(colors))
	}
}
