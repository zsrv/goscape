package objtype

import (
	"path/filepath"
	"testing"

	jag "github.com/zsrv/goscape/pkg/io/jagfile"
	packet2 "github.com/zsrv/goscape/pkg/io/packet"
)

type locEntry struct {
	debugName string
	desc      string
	category  int
	width     int
	length    int
	intParams map[uint32]uint32
	op        []string // codes 30-34
}

// hashLocDat is genHash("loc.dat") — pre-computed via the algorithm in
// pkg/io/jagfile/jagfile.go:18-25 (uppercase + h*61+c-32 reduction).
const hashLocDat uint32 = 682978269

// buildLocServerDat assembles the server-side loc.dat blob:
//
//	u16 count
//	for each entry: codes 61 (category), 249 (params), 250 (debugname),
//	terminated by code 0.
func buildLocServerDat(entries []locEntry) []byte {
	pkt := packet2.NewPacket(nil)
	pkt.P2(uint16(len(entries)))
	for _, e := range entries {
		if e.category != 0 {
			pkt.P1(61)
			pkt.P2(uint16(e.category))
		}
		if len(e.intParams) > 0 {
			pkt.P1(249)
			pkt.P1(uint8(len(e.intParams)))
			for k, v := range e.intParams {
				pkt.P3(k)
				pkt.PBool(false)
				pkt.P4(v)
			}
		}
		if e.debugName != "" {
			pkt.P1(250)
			pkt.PJStrLF(e.debugName)
		}
		pkt.P1(0)
	}
	return pkt.Bytes()
}

// buildLocClientDat assembles the inner client-side loc.dat payload (the
// blob that lives inside client/config jagfile under entry name "loc.dat"):
//
//	u16 count
//	for each entry: codes 3 (desc), 14 (width), 15 (length), 30-34 (op),
//	250 (debugname), terminated by code 0.
func buildLocClientDat(entries []locEntry) []byte {
	pkt := packet2.NewPacket(nil)
	pkt.P2(uint16(len(entries)))
	for _, e := range entries {
		if e.desc != "" {
			pkt.P1(3)
			pkt.PJStrLF(e.desc)
		}
		if e.width != 0 {
			pkt.P1(14)
			pkt.P1(uint8(e.width))
		}
		if e.length != 0 {
			pkt.P1(15)
			pkt.P1(uint8(e.length))
		}
		for i, name := range e.op {
			if name == "" {
				continue
			}
			pkt.P1(uint8(30 + i))
			pkt.PJStrLF(name)
		}
		if e.debugName != "" {
			pkt.P1(250)
			pkt.PJStrLF(e.debugName)
		}
		pkt.P1(0)
	}
	return pkt.Bytes()
}

// buildClientJag wraps a single-entry jagfile around the given client
// loc.dat blob and returns a parsed *jag.Jagfile ready for parseLocTypes.
// Mirrors componenttype_test.go:751 buildMinimalJagfile pattern.
func buildClientJag(t *testing.T, locDatBytes []byte) *jag.Jagfile {
	t.Helper()
	compressed, err := jag.BZip2Compress(locDatBytes, false, true, 1, 0)
	if err != nil {
		t.Fatalf("BZip2Compress: %v", err)
	}
	p := packet2.NewPacket(nil)
	p.P3(1)                        // unpackedSize (== packedSize → CompressWhole=false outer path)
	p.P3(1)                        // packedSize
	p.P2(1)                        // fileCount = 1
	p.P4(hashLocDat)               // file hash
	p.P3(uint32(len(locDatBytes))) // unpacked size
	p.P3(uint32(len(compressed)))  // packed size
	p.Data = append(p.Data, compressed...)

	jf, err := jag.NewJagfile(packet2.NewPacket(p.Data))
	if err != nil {
		t.Fatalf("NewJagfile: %v", err)
	}
	return jf
}

// buildLocFixture is a convenience that builds both server bytes and
// client jagfile from a single entries list, ready for parseLocTypes.
func buildLocFixture(t *testing.T, entries []locEntry) (*packet2.Packet, *jag.Jagfile) {
	t.Helper()
	server := packet2.NewPacket(buildLocServerDat(entries))
	clientJag := buildClientJag(t, buildLocClientDat(entries))
	return server, clientJag
}

func TestParseLocTypes(t *testing.T) {
	entries := []locEntry{
		{
			debugName: "door_basic",
			desc:      "A wooden door.",
			category:  17,
			width:     1,
			length:    2,
			intParams: map[uint32]uint32{1: 100},
		},
		{
			debugName: "bush",
		},
	}

	server, clientJag := buildLocFixture(t, entries)
	cfgs, err := parseLocTypes(server, clientJag)
	if err != nil {
		t.Fatalf("parseLocTypes: %v", err)
	}
	if len(cfgs.Configs) != 2 {
		t.Fatalf("configs: got %d, want 2", len(cfgs.Configs))
	}

	door := cfgs.Configs[0]
	if door.DebugName != "door_basic" {
		t.Errorf("DebugName[0]: got %q", door.DebugName)
	}
	if door.Desc != "A wooden door." {
		t.Errorf("Desc[0]: got %q", door.Desc)
	}
	if door.Category != 17 {
		t.Errorf("Category[0]: got %d, want 17", door.Category)
	}
	if door.Width != 1 || door.Length != 2 {
		t.Errorf("Width/Length[0]: got %d/%d, want 1/2", door.Width, door.Length)
	}
	if got, _ := door.Params[1].(uint32); got != 100 {
		t.Errorf("Params[1]: got %v, want 100", door.Params[1])
	}

	bush := cfgs.Configs[1]
	if bush.Category != -1 {
		t.Errorf("Category default (bush): got %d, want -1", bush.Category)
	}
	if bush.Width != 1 || bush.Length != 1 {
		t.Errorf("Width/Length default (bush): got %d/%d, want 1/1", bush.Width, bush.Length)
	}

	if cfgs.ConfigNames["door_basic"] != 0 {
		t.Errorf("ConfigNames[door_basic]: got %d, want 0", cfgs.ConfigNames["door_basic"])
	}
}

func TestLocUnknownCode(t *testing.T) {
	server := packet2.NewPacket(nil)
	server.P2(1) // count = 1
	server.P1(0) // immediate terminator on server side

	clientInner := packet2.NewPacket(nil)
	clientInner.P2(1)   // count = 1
	clientInner.P1(200) // bogus code in client blob
	clientInner.P1(0)

	clientJag := buildClientJag(t, clientInner.Bytes())

	_, err := parseLocTypes(packet2.NewPacket(server.Bytes()), clientJag)
	if err == nil {
		t.Fatal("expected error on unknown loc code, got nil")
	}
}

func TestLocTypeDecodeOpSingleEntry(t *testing.T) {
	entries := []locEntry{
		{debugName: "tree", op: []string{"Chop", "", "", "", ""}},
	}
	server, clientJag := buildLocFixture(t, entries)

	cfgs, err := parseLocTypes(server, clientJag)
	if err != nil {
		t.Fatalf("parseLocTypes: %v", err)
	}
	if got := len(cfgs.Configs); got != 1 {
		t.Fatalf("Configs len: got %d, want 1", got)
	}

	tree := cfgs.Configs[0]
	if tree.Op == nil {
		t.Fatal("Op: got nil, want 5-slot slice")
	}
	if got := tree.Op[0]; got != "Chop" {
		t.Errorf("Op[0]: got %q, want \"Chop\"", got)
	}
	for i := 1; i < 5; i++ {
		if tree.Op[i] != "" {
			t.Errorf("Op[%d]: got %q, want \"\"", i, tree.Op[i])
		}
	}
	// Pins parseLocTypes → PostDecode wiring: Op != nil so Active=1.
	if tree.Active != 1 {
		t.Errorf("Active: got %d, want 1 (PostDecode wiring pin: Op != nil → Active=1)", tree.Active)
	}
}

func TestLocTypeDecodeOpAllFive(t *testing.T) {
	entries := []locEntry{
		{debugName: "multi", op: []string{"op0", "op1", "op2", "op3", "op4"}},
	}
	server, clientJag := buildLocFixture(t, entries)

	cfgs, err := parseLocTypes(server, clientJag)
	if err != nil {
		t.Fatalf("parseLocTypes: %v", err)
	}

	multi := cfgs.Configs[0]
	want := []string{"op0", "op1", "op2", "op3", "op4"}
	for i, w := range want {
		if got := multi.Op[i]; got != w {
			t.Errorf("Op[%d]: got %q, want %q", i, got, w)
		}
	}
}

// TestLocTypeDecodeOpStoredVerbatim pins L32: the loc decoder stores the
// op string verbatim, including "hidden" (the former NAI-80-D1 coercion is
// removed). TS LocType stores it verbatim and gates "hidden" at the
// op-click handler (handler_oploc.go), while LOC iterators/gates use a
// truthy check that reads "hidden" as a present, operable slot.
func TestLocTypeDecodeOpStoredVerbatim(t *testing.T) {
	entries := []locEntry{
		{debugName: "hidden_test", op: []string{"visible", "hidden", "", "", ""}},
	}
	server, clientJag := buildLocFixture(t, entries)

	cfgs, err := parseLocTypes(server, clientJag)
	if err != nil {
		t.Fatalf("parseLocTypes: %v", err)
	}

	entry := cfgs.Configs[0]
	if got := entry.Op[0]; got != "visible" {
		t.Errorf("Op[0]: got %q, want \"visible\"", got)
	}
	if got := entry.Op[1]; got != "hidden" {
		t.Errorf("Op[1] (verbatim): got %q, want \"hidden\"", got)
	}
}

// TestLoadRealLocCache verifies the loader handles the repo's real server
// cache end-to-end. This is a regression guard in case goscape's packer ever
// writes a loc code this loader doesn't recognise.
func TestLoadRealLocCache(t *testing.T) {
	cacheDir := filepath.Join("..", "..", "data", "pack")
	cfgs, err := LoadLocTypes(cacheDir)
	if err != nil {
		t.Skipf("no cache data: %v", err)
	}
	if len(cfgs.Configs) == 0 {
		t.Fatal("expected at least one LocType, got 0")
	}
	t.Logf("loaded %d LocTypes from %s", len(cfgs.Configs), cacheDir)
}

func TestPostDecode_ActiveInference(t *testing.T) {
	t.Run("op_nonnil_sets_active_1", func(t *testing.T) {
		lt := NewLocType(0)
		lt.Op = []string{"Open", "", "", "", ""}
		lt.PostDecode()
		if lt.Active != 1 {
			t.Errorf("Active: got %d, want 1 (Op != nil branch)", lt.Active)
		}
	})

	t.Run("shapes_single_10_sets_active_1", func(t *testing.T) {
		lt := NewLocType(0)
		lt.Shapes = []uint8{10}
		lt.PostDecode()
		if lt.Active != 1 {
			t.Errorf("Active: got %d, want 1 (Shapes==[10] branch)", lt.Active)
		}
	})

	t.Run("neither_sets_active_0", func(t *testing.T) {
		lt := NewLocType(0)
		lt.PostDecode()
		if lt.Active != 0 {
			t.Errorf("Active: got %d, want 0 (default fallthrough)", lt.Active)
		}
	})

	t.Run("active_already_set_unchanged", func(t *testing.T) {
		lt := NewLocType(0)
		lt.Active = 5
		lt.Op = []string{"Open", "", "", "", ""}
		lt.PostDecode()
		if lt.Active != 5 {
			t.Errorf("Active: got %d, want 5 (already-set guard)", lt.Active)
		}
	})

	t.Run("shapes_multi_no_active_inference", func(t *testing.T) {
		lt := NewLocType(0)
		lt.Shapes = []uint8{10, 11}
		lt.PostDecode()
		if lt.Active != 0 {
			t.Errorf("Active: got %d, want 0 (Shapes len != 1)", lt.Active)
		}
	})

	t.Run("shapes_single_non10_no_active_inference", func(t *testing.T) {
		lt := NewLocType(0)
		lt.Shapes = []uint8{5}
		lt.PostDecode()
		if lt.Active != 0 {
			t.Errorf("Active: got %d, want 0 (Shapes[0] != 10)", lt.Active)
		}
	})
}

// buildClientBlobRaw assembles a 1-entry client loc.dat with the given
// raw code-payload bytes inserted between the count header and the 0
// terminator. Used by per-arm decode tests in TestLocTypeDecodeNewArms.
func buildClientBlobRaw(payload []byte) []byte {
	pkt := packet2.NewPacket(nil)
	pkt.P2(1) // count = 1
	pkt.Data = append(pkt.Data, payload...)
	pkt.P1(0) // terminator
	return pkt.Bytes()
}

// withMinimalServer pairs a 1-entry server blob (no codes, just terminator)
// with the given client jagfile, returning both ready for parseLocTypes.
func withMinimalServer(t *testing.T, clientJag *jag.Jagfile) (*packet2.Packet, *jag.Jagfile) {
	t.Helper()
	srv := packet2.NewPacket(nil)
	srv.P2(1) // count = 1
	srv.P1(0) // terminator
	return packet2.NewPacket(srv.Bytes()), clientJag
}

func TestLocTypeDecodeNewArms(t *testing.T) {
	cases := []struct {
		name    string
		payload []byte
		assert  func(t *testing.T, lt *LocType)
	}{
		{
			name: "code1_models_shapes_pair",
			payload: func() []byte {
				p := packet2.NewPacket(nil)
				p.P1(1)
				p.P1(2)      // count = 2
				p.P2(0x1111) // models[0]
				p.P1(10)     // shapes[0]
				p.P2(0x2222) // models[1]
				p.P1(11)     // shapes[1]
				return p.Bytes()
			}(),
			assert: func(t *testing.T, lt *LocType) {
				if len(lt.Models) != 2 || lt.Models[0] != 0x1111 || lt.Models[1] != 0x2222 {
					t.Errorf("Models: got %v, want [0x1111 0x2222]", lt.Models)
				}
				if len(lt.Shapes) != 2 || lt.Shapes[0] != 10 || lt.Shapes[1] != 11 {
					t.Errorf("Shapes: got %v, want [10 11]", lt.Shapes)
				}
			},
		},
		{
			name: "code2_name",
			payload: func() []byte {
				p := packet2.NewPacket(nil)
				p.P1(2)
				p.PJStrLF("oak tree")
				return p.Bytes()
			}(),
			assert: func(t *testing.T, lt *LocType) {
				if lt.Name != "oak tree" {
					t.Errorf("Name: got %q, want \"oak tree\"", lt.Name)
				}
			},
		},
		{
			name:    "code17_blockwalk_false",
			payload: []byte{17},
			assert: func(t *testing.T, lt *LocType) {
				if lt.BlockWalk {
					t.Errorf("BlockWalk: got true, want false")
				}
			},
		},
		{
			name:    "code18_blockrange_false",
			payload: []byte{18},
			assert: func(t *testing.T, lt *LocType) {
				if lt.BlockRange {
					t.Errorf("BlockRange: got true, want false")
				}
			},
		},
		{
			name:    "code19_active_g1",
			payload: []byte{19, 7},
			assert: func(t *testing.T, lt *LocType) {
				if lt.Active != 7 {
					t.Errorf("Active: got %d, want 7", lt.Active)
				}
			},
		},
		{
			name:    "code21_hillskew_true",
			payload: []byte{21},
			assert: func(t *testing.T, lt *LocType) {
				if !lt.HillSkew {
					t.Errorf("HillSkew: got false, want true")
				}
			},
		},
		{
			name:    "code22_sharelight_true",
			payload: []byte{22},
			assert: func(t *testing.T, lt *LocType) {
				if !lt.ShareLight {
					t.Errorf("ShareLight: got false, want true")
				}
			},
		},
		{
			name:    "code23_occlude_true",
			payload: []byte{23},
			assert: func(t *testing.T, lt *LocType) {
				if !lt.Occlude {
					t.Errorf("Occlude: got false, want true")
				}
			},
		},
		{
			name:    "code24_anim_g2_normal",
			payload: []byte{24, 0x12, 0x34}, // 0x1234 = 4660
			assert: func(t *testing.T, lt *LocType) {
				if lt.Anim != 4660 {
					t.Errorf("Anim: got %d, want 4660", lt.Anim)
				}
			},
		},
		{
			name:    "code24_anim_65535_to_neg1",
			payload: []byte{24, 0xFF, 0xFF},
			assert: func(t *testing.T, lt *LocType) {
				if lt.Anim != -1 {
					t.Errorf("Anim: got %d, want -1 (65535 → -1)", lt.Anim)
				}
			},
		},
		{
			name:    "code25_hasalpha_true",
			payload: []byte{25},
			assert: func(t *testing.T, lt *LocType) {
				if !lt.HasAlpha {
					t.Errorf("HasAlpha: got false, want true")
				}
			},
		},
		{
			name:    "code28_wallwidth_g1",
			payload: []byte{28, 32},
			assert: func(t *testing.T, lt *LocType) {
				if lt.WallWidth != 32 {
					t.Errorf("WallWidth: got %d, want 32", lt.WallWidth)
				}
			},
		},
		{
			name:    "code29_ambient_g1b_negative",
			payload: []byte{29, 0xFF}, // -1 signed
			assert: func(t *testing.T, lt *LocType) {
				if lt.Ambient != -1 {
					t.Errorf("Ambient: got %d, want -1", lt.Ambient)
				}
			},
		},
		{
			name:    "code39_contrast_g1b_negative",
			payload: []byte{39, 0xFE}, // -2 signed
			assert: func(t *testing.T, lt *LocType) {
				if lt.Contrast != -2 {
					t.Errorf("Contrast: got %d, want -2", lt.Contrast)
				}
			},
		},
		{
			name: "code40_recol_pair",
			payload: func() []byte {
				p := packet2.NewPacket(nil)
				p.P1(40)
				p.P1(2) // count = 2
				p.P2(0xAAAA)
				p.P2(0xBBBB)
				p.P2(0xCCCC)
				p.P2(0xDDDD)
				return p.Bytes()
			}(),
			assert: func(t *testing.T, lt *LocType) {
				if len(lt.RecolS) != 2 || lt.RecolS[0] != 0xAAAA || lt.RecolS[1] != 0xCCCC {
					t.Errorf("RecolS: got %v", lt.RecolS)
				}
				if len(lt.RecolD) != 2 || lt.RecolD[0] != 0xBBBB || lt.RecolD[1] != 0xDDDD {
					t.Errorf("RecolD: got %v", lt.RecolD)
				}
			},
		},
		{
			name:    "code60_mapfunction_g2",
			payload: []byte{60, 0x01, 0x23},
			assert: func(t *testing.T, lt *LocType) {
				if lt.MapFunction != 0x0123 {
					t.Errorf("MapFunction: got %d, want 0x0123", lt.MapFunction)
				}
			},
		},
		{
			name:    "code62_mirror_true",
			payload: []byte{62},
			assert: func(t *testing.T, lt *LocType) {
				if !lt.Mirror {
					t.Errorf("Mirror: got false, want true")
				}
			},
		},
		{
			name:    "code64_shadow_false",
			payload: []byte{64},
			assert: func(t *testing.T, lt *LocType) {
				if lt.Shadow {
					t.Errorf("Shadow: got true, want false")
				}
			},
		},
		{
			name:    "code65_resizex_g2",
			payload: []byte{65, 0x00, 0x40}, // 64
			assert: func(t *testing.T, lt *LocType) {
				if lt.ResizeX != 64 {
					t.Errorf("ResizeX: got %d, want 64", lt.ResizeX)
				}
			},
		},
		{
			name:    "code66_resizey_g2",
			payload: []byte{66, 0x00, 0x50}, // 80
			assert: func(t *testing.T, lt *LocType) {
				if lt.ResizeY != 80 {
					t.Errorf("ResizeY: got %d, want 80", lt.ResizeY)
				}
			},
		},
		{
			name:    "code67_resizez_g2",
			payload: []byte{67, 0x00, 0x60}, // 96
			assert: func(t *testing.T, lt *LocType) {
				if lt.ResizeZ != 96 {
					t.Errorf("ResizeZ: got %d, want 96", lt.ResizeZ)
				}
			},
		},
		{
			name:    "code68_mapscene_g2",
			payload: []byte{68, 0x04, 0x56},
			assert: func(t *testing.T, lt *LocType) {
				if lt.MapScene != 0x0456 {
					t.Errorf("MapScene: got %d, want 0x0456", lt.MapScene)
				}
			},
		},
		{
			name:    "code69_forceapproach_g1",
			payload: []byte{69, 3},
			assert: func(t *testing.T, lt *LocType) {
				if lt.ForceApproach != 3 {
					t.Errorf("ForceApproach: got %d, want 3", lt.ForceApproach)
				}
			},
		},
		{
			name:    "code70_offsetx_g2s_negative",
			payload: []byte{70, 0xFF, 0xFE}, // -2 as int16
			assert: func(t *testing.T, lt *LocType) {
				if lt.OffsetX != -2 {
					t.Errorf("OffsetX: got %d, want -2", lt.OffsetX)
				}
			},
		},
		{
			name:    "code71_offsety_g2s_negative",
			payload: []byte{71, 0xFF, 0xFD}, // -3
			assert: func(t *testing.T, lt *LocType) {
				if lt.OffsetY != -3 {
					t.Errorf("OffsetY: got %d, want -3", lt.OffsetY)
				}
			},
		},
		{
			name:    "code72_offsetz_g2s_positive",
			payload: []byte{72, 0x00, 0x05},
			assert: func(t *testing.T, lt *LocType) {
				if lt.OffsetZ != 5 {
					t.Errorf("OffsetZ: got %d, want 5", lt.OffsetZ)
				}
			},
		},
		{
			name:    "code73_forcedecor_true",
			payload: []byte{73},
			assert: func(t *testing.T, lt *LocType) {
				if !lt.ForceDecor {
					t.Errorf("ForceDecor: got false, want true")
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			clientJag := buildClientJag(t, buildClientBlobRaw(tc.payload))
			server, clientJag := withMinimalServer(t, clientJag)
			cfgs, err := parseLocTypes(server, clientJag)
			if err != nil {
				t.Fatalf("parseLocTypes: %v", err)
			}
			if len(cfgs.Configs) != 1 {
				t.Fatalf("Configs len: got %d, want 1", len(cfgs.Configs))
			}
			tc.assert(t, cfgs.Configs[0])
		})
	}
}

func TestLocTypeConfigs_ByName_HitViaConfigNames(t *testing.T) {
	lc := &LocTypeConfigs{
		Configs:     []*LocType{{ID: 0, DebugName: "first"}, {ID: 1, DebugName: "second"}},
		ConfigNames: map[string]int{"first": 0, "second": 1},
	}
	got := lc.ByName("second")
	if got == nil {
		t.Fatalf("ByName(second) = nil, want non-nil")
	}
	if got.ID != 1 || got.DebugName != "second" {
		t.Errorf("ByName(second) = {ID:%d, DebugName:%q}, want {ID:1, DebugName:\"second\"}", got.ID, got.DebugName)
	}
}

func TestLocTypeConfigs_ByName_MissReturnsNil(t *testing.T) {
	lc := &LocTypeConfigs{
		Configs:     []*LocType{{ID: 0, DebugName: "only"}},
		ConfigNames: map[string]int{"only": 0},
	}
	if got := lc.ByName("absent"); got != nil {
		t.Errorf("ByName(absent) = %+v, want nil", got)
	}
}

func TestLocTypeConfigs_ByName_NilReceiverReturnsNil(t *testing.T) {
	var lc *LocTypeConfigs
	if got := lc.ByName("anything"); got != nil {
		t.Errorf("nil-receiver ByName = %+v, want nil", got)
	}
}

func TestLocTypeConfigs_ByName_StaleIndexFallsThroughToLinearScan(t *testing.T) {
	// ConfigNames points "fresh" at id=5 but Configs is only length 2.
	// Lookup must NOT panic and must fall through to the linear scan,
	// which finds "fresh" at id=1 by DebugName equality.
	lc := &LocTypeConfigs{
		Configs:     []*LocType{{ID: 0, DebugName: "other"}, {ID: 1, DebugName: "fresh"}},
		ConfigNames: map[string]int{"fresh": 5},
	}
	got := lc.ByName("fresh")
	if got == nil {
		t.Fatalf("stale-index ByName(fresh) = nil; want fallback hit at id=1")
	}
	if got.ID != 1 {
		t.Errorf("stale-index ByName(fresh).ID = %d, want 1", got.ID)
	}
}

func TestLocTypeConfigs_ByName_LinearScanWhenConfigNamesEmpty(t *testing.T) {
	// Some test fixtures construct Configs without populating ConfigNames.
	// ByName must still resolve by DebugName.
	lc := &LocTypeConfigs{
		Configs:     []*LocType{{ID: 0, DebugName: "scan_me"}},
		ConfigNames: nil,
	}
	got := lc.ByName("scan_me")
	if got == nil || got.ID != 0 {
		t.Errorf("ByName(scan_me) with nil ConfigNames = %+v, want non-nil id=0", got)
	}
}
