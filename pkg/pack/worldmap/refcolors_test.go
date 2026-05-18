package worldmap

import "testing"

func TestRefColors_Length(t *testing.T) {
	t.Parallel()
	// TS Worldmap.ts:533-613 declares 79 rows (lines 534-612 inclusive);
	// the plan's 80-entry assertion was incorrect — keep TS as the source
	// of truth.
	if got, want := len(refColors), 79; got != want {
		t.Errorf("len(refColors) = %d, want %d", got, want)
	}
}

func TestRefColors_SpotCheck(t *testing.T) {
	t.Parallel()
	// Row 0: cliff      edge=0x00000038 fill=0x009c8f8e
	if refColors[0][0] != 0x00000038 || refColors[0][1] != 0x009c8f8e {
		t.Errorf("row 0 = (%#x, %#x), want (0x00000038, 0x009c8f8e)",
			refColors[0][0], refColors[0][1])
	}
	// Row 4: woodenfloor edge=0x00000000 fill=0x003b1d0c
	if refColors[4][0] != 0x00000000 || refColors[4][1] != 0x003b1d0c {
		t.Errorf("row 4 = (%#x, %#x), want (0x00000000, 0x003b1d0c)",
			refColors[4][0], refColors[4][1])
	}
	// Row 78 (last, hive — TS Worldmap.ts:612): edge=0x00b06826 fill=0x0071673f
	if refColors[78][0] != 0x00b06826 || refColors[78][1] != 0x0071673f {
		t.Errorf("row 78 = (%#x, %#x), want (0x00b06826, 0x0071673f)",
			refColors[78][0], refColors[78][1])
	}
}
