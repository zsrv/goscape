package gamemap

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
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
	mapsDir := filepath.Join(tmp, "maps")
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
	mapsDir := filepath.Join(tmp, "maps")
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
