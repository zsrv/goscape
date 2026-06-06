package worldmap

import "testing"

func TestRefColors_SpotCheck(t *testing.T) {
	t.Parallel()
	// Row 0: cliff      edge=0x00000038 fill=0x009c8f8e
	if refColors[0][0] != 0x00000038 || refColors[0][1] != 0x009c8f8e {
		t.Errorf("row 0 = (%#x, %#x), want (0x00000038, 0x009c8f8e)",
			refColors[0][0], refColors[0][1])
	}
	// Row 39: bluefloor2  edge=0x03808427 fill=0x004e4a82
	if refColors[39][0] != 0x03808427 || refColors[39][1] != 0x004e4a82 {
		t.Errorf("row 39 = (%#x, %#x), want (0x03808427, 0x004e4a82)",
			refColors[39][0], refColors[39][1])
	}
	// Row 78 (hive — TS Worldmap.ts:612): edge=0x00b06826 fill=0x0071673f
	if refColors[78][0] != 0x00b06826 || refColors[78][1] != 0x0071673f {
		t.Errorf("row 78 = (%#x, %#x), want (0x00b06826, 0x0071673f)",
			refColors[78][0], refColors[78][1])
	}
	// Row 88 (last, viking_mud_overlay — TS Worldmap.ts:622 @ 9aadcec4):
	// edge=0x0090b814 fill=0x00625416
	if refColors[88][0] != 0x0090b814 || refColors[88][1] != 0x00625416 {
		t.Errorf("row 88 = (%#x, %#x), want (0x0090b814, 0x00625416)",
			refColors[88][0], refColors[88][1])
	}
}
