package gamemap

import (
	"bytes"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/zsrv/goscape/pkg/entity"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestInitHandlesMissingDir(t *testing.T) {
	gm := New(discardLogger())
	err := gm.Init(t.TempDir())
	if err != nil {
		t.Errorf("Init on empty dir: got %v, want nil", err)
	}
}

func TestInitHandlesMissingCsv(t *testing.T) {
	tmp := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmp, "maps"), 0755); err != nil {
		t.Fatal(err)
	}
	gm := New(discardLogger())
	err := gm.Init(tmp)
	if err != nil {
		t.Errorf("Init with missing CSVs: got %v, want nil", err)
	}
	if gm.IsMulti(1000, 2000, 0) {
		t.Error("IsMulti should default false when multimap CSV missing")
	}
	if gm.IsFreeToPlay(1000, 2000) {
		t.Error("IsFreeToPlay should default false when freemap CSV missing")
	}
}

func TestInitLoadsCsvMaps(t *testing.T) {
	tmp := t.TempDir()
	mapsDir := filepath.Join(tmp, "client", "maps")
	if err := os.MkdirAll(mapsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mapsDir, "multiway.csv"), []byte("0,1000,2000\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mapsDir, "free2play.csv"), []byte("0,1500,2500\n"), 0644); err != nil {
		t.Fatal(err)
	}

	gm := New(discardLogger())
	if err := gm.Init(tmp); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if !gm.IsMulti(1000, 2000, 0) {
		t.Error("expected (1000,2000,0) to be multi")
	}
	if !gm.IsFreeToPlay(1500, 2500) {
		t.Error("expected (1500,2500) to be F2P")
	}
}

func TestLoadNpcsParsesSpawns(t *testing.T) {
	tmp := t.TempDir()
	mapsDir := filepath.Join(tmp, "client", "maps") // matches Init's cacheDir/client/maps lookup
	if err := os.MkdirAll(mapsDir, 0755); err != nil {
		t.Fatal(err)
	}
	// Minimal m file (loadGround needs at least an opcode-0 byte).
	if err := os.WriteFile(filepath.Join(mapsDir, "m50_50"), []byte{0}, 0644); err != nil {
		t.Fatal(err)
	}

	// n50_50: one record at local (10, 20) level 0 with 2 spawns of types 100 and 200.
	// packed = (0<<12) | (10<<6) | 20 = 660 = 0x0294
	// Bytes: G2(packed)=0x02 0x94, G1(count)=0x02, G2(100)=0x00 0x64, G2(200)=0x00 0xC8
	nData := []byte{0x02, 0x94, 0x02, 0x00, 0x64, 0x00, 0xC8}
	if err := os.WriteFile(filepath.Join(mapsDir, "n50_50"), nData, 0644); err != nil {
		t.Fatal(err)
	}

	gm := New(discardLogger())
	if err := gm.Init(tmp); err != nil {
		t.Fatal(err)
	}

	spawns := gm.NpcSpawns()
	if len(spawns) != 2 {
		t.Fatalf("spawns: got %d, want 2 (got %+v)", len(spawns), spawns)
	}
	wantX, wantZ := 50*64+10, 50*64+20
	for i, want := range []int{100, 200} {
		if spawns[i].TypeID != want {
			t.Errorf("spawn[%d].TypeID: got %d, want %d", i, spawns[i].TypeID, want)
		}
		if spawns[i].X != wantX || spawns[i].Z != wantZ || spawns[i].Level != 0 {
			t.Errorf("spawn[%d] coords: got (%d,%d,%d), want (%d,%d,0)", i, spawns[i].X, spawns[i].Z, spawns[i].Level, wantX, wantZ)
		}
	}
}

func TestMapsquareCRCReturnsZeroForMissing(t *testing.T) {
	gm := New(discardLogger())
	mCRC, lCRC := gm.MapsquareCRC(0, 0)
	if mCRC != 0 || lCRC != 0 {
		t.Errorf("missing mapsquare: got (%d,%d), want (0,0)", mCRC, lCRC)
	}
}

func TestMapsquareCRCCachedFromInit(t *testing.T) {
	tmp := t.TempDir()
	mapsDir := filepath.Join(tmp, "client", "maps")
	if err := os.MkdirAll(mapsDir, 0755); err != nil {
		t.Fatal(err)
	}
	mData := []byte{0, 0, 0, 0}
	if err := os.WriteFile(filepath.Join(mapsDir, "m50_50"), mData, 0644); err != nil {
		t.Fatal(err)
	}
	gm := New(discardLogger())
	if err := gm.Init(tmp); err != nil {
		t.Fatal(err)
	}
	mCRC, _ := gm.MapsquareCRC(50, 50)
	if mCRC == 0 {
		t.Error("expected non-zero CRC after Init")
	}
}

func TestGameMapRetainsRawBytes(t *testing.T) {
	dir := t.TempDir()
	mapsDir := filepath.Join(dir, "client", "maps")
	if err := os.MkdirAll(mapsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mapsDir, "m50_51"), []byte{0xDE, 0xAD}, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mapsDir, "l50_51"), []byte{0xBE, 0xEF}, 0644); err != nil {
		t.Fatal(err)
	}

	gm := New(discardLogger())
	if err := gm.Init(dir); err != nil {
		t.Fatal(err)
	}
	if got := gm.LandBytes(50, 51); !bytes.Equal(got, []byte{0xDE, 0xAD}) {
		t.Errorf("LandBytes: got %v, want [0xDE, 0xAD]", got)
	}
	if got := gm.LocBytes(50, 51); !bytes.Equal(got, []byte{0xBE, 0xEF}) {
		t.Errorf("LocBytes: got %v, want [0xBE, 0xEF]", got)
	}
	if gm.LandBytes(0, 0) != nil {
		t.Errorf("LandBytes(0,0) unloaded should return nil; got %v", gm.LandBytes(0, 0))
	}
}

func TestAddStaticLocPublicAPI(t *testing.T) {
	gm := New(discardLogger())
	loc := entity.NewLoc(0, 100, 200, 1, 1, entity.LifecycleRespawn, 42, 0, 0)
	gm.AddStaticLoc(loc)
	if got := gm.StaticLocs(); len(got) != 1 || got[0] != loc {
		t.Errorf("StaticLocs after Add: got %v, want [loc]", got)
	}
}

func TestLoadLocsParsesKnownFixture(t *testing.T) {
	// gsmart(101)=0x65; gsmart(200)=0x80 0xC8; info=(5<<2)|2=0x16.
	// coord packed = 199 = (level=0<<12) | (localX=3<<6) | (localZ=7).
	fixture := []byte{
		0x65,       // locID delta 101 -> locID = 100
		0x80, 0xC8, // coord delta 200 -> coord = 199
		0x16, // info: shape 5, angle 2
		0x00, // end inner
		0x00, // end outer
	}
	gm := New(discardLogger())
	gm.loadLocs(fixture, 50, 51)

	statics := gm.StaticLocs()
	if len(statics) != 1 {
		t.Fatalf("StaticLocs: got %d, want 1", len(statics))
	}
	loc := statics[0]
	if loc.Level != 0 {
		t.Errorf("Level: got %d, want 0", loc.Level)
	}
	if loc.X != 50*64+3 {
		t.Errorf("X: got %d, want %d", loc.X, 50*64+3)
	}
	if loc.Z != 51*64+7 {
		t.Errorf("Z: got %d, want %d", loc.Z, 51*64+7)
	}
	if loc.Type() != 100 {
		t.Errorf("Type: got %d, want 100", loc.Type())
	}
	if loc.Shape() != 5 {
		t.Errorf("Shape: got %d, want 5", loc.Shape())
	}
	if loc.Angle() != 2 {
		t.Errorf("Angle: got %d, want 2", loc.Angle())
	}
	if loc.Lifecycle != entity.LifecycleRespawn {
		t.Errorf("Lifecycle: got %v, want Respawn", loc.Lifecycle)
	}
}

func TestLoadLocsMultipleLocIDs(t *testing.T) {
	fixture := []byte{
		0x0B, // locID delta 11 -> locID = 10
		0x01, // coord delta 1 -> coord = 0
		0x00, // info
		0x00, // end inner
		0x0B, // locID delta 11 -> locID = 21
		0x01, // coord delta 1 -> coord = 0
		0x00, // info
		0x00, // end inner
		0x00, // end outer
	}
	gm := New(discardLogger())
	gm.loadLocs(fixture, 0, 0)
	if got := len(gm.StaticLocs()); got != 2 {
		t.Errorf("StaticLocs count: got %d, want 2", got)
	}
	if gm.StaticLocs()[0].Type() != 10 {
		t.Errorf("first loc type: got %d, want 10", gm.StaticLocs()[0].Type())
	}
	if gm.StaticLocs()[1].Type() != 21 {
		t.Errorf("second loc type: got %d, want 21", gm.StaticLocs()[1].Type())
	}
}

func TestLoadLocsEmptyFile(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("loadLocs panicked on empty input: %v", r)
		}
	}()
	gm := New(discardLogger())
	gm.loadLocs([]byte{}, 0, 0)
	if got := len(gm.StaticLocs()); got != 0 {
		t.Errorf("empty input should produce 0 locs; got %d", got)
	}
}
