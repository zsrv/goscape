package worldmap

import "testing"

func TestRefColors_SpotCheck(t *testing.T) {
	t.Parallel()
	// rev-274 (TS Worldmap.ts:520-622 @ dee467c8): every fill color
	// recomputed; two rows appended (slayer_tower, morytania_dark_green).
	if len(refColors) != 103 {
		t.Fatalf("len(refColors) = %d, want 103", len(refColors))
	}
	// Row 0: cliff      edge=0x00000038 fill=0x00847776
	if refColors[0][0] != 0x00000038 || refColors[0][1] != 0x00847776 {
		t.Errorf("row 0 = (%#x, %#x), want (0x00000038, 0x00847776)",
			refColors[0][0], refColors[0][1])
	}
	// Row 39: bluefloor2  edge=0x03808427 fill=0x00404995
	if refColors[39][0] != 0x03808427 || refColors[39][1] != 0x00404995 {
		t.Errorf("row 39 = (%#x, %#x), want (0x03808427, 0x00404995)",
			refColors[39][0], refColors[39][1])
	}
	// Row 78 (hive): edge=0x00b06826 fill=0x00817a38
	if refColors[78][0] != 0x00b06826 || refColors[78][1] != 0x00817a38 {
		t.Errorf("row 78 = (%#x, %#x), want (0x00b06826, 0x00817a38)",
			refColors[78][0], refColors[78][1])
	}
	// Row 100 (mm_town_overlay, last rev-254 entry):
	// edge=0x00a0c011 fill=0x0055431b
	if refColors[100][0] != 0x00a0c011 || refColors[100][1] != 0x0055431b {
		t.Errorf("row 100 = (%#x, %#x), want (0x00a0c011, 0x0055431b)",
			refColors[100][0], refColors[100][1])
	}
	// Row 101 (slayer_tower, new at rev-274):
	// edge=0x01203c0f fill=0x00303224
	if refColors[101][0] != 0x01203c0f || refColors[101][1] != 0x00303224 {
		t.Errorf("row 101 = (%#x, %#x), want (0x01203c0f, 0x00303224)",
			refColors[101][0], refColors[101][1])
	}
	// Row 102 (morytania_dark_green, new at rev-274, last):
	// edge=0x02605c07 fill=0x00030704
	if refColors[102][0] != 0x02605c07 || refColors[102][1] != 0x00030704 {
		t.Errorf("row 102 = (%#x, %#x), want (0x02605c07, 0x00030704)",
			refColors[102][0], refColors[102][1])
	}
}
