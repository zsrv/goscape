package world

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	"github.com/zsrv/goscape/pkg/inventory"
	"github.com/zsrv/goscape/pkg/io/packet"
	"github.com/zsrv/goscape/pkg/objtype"
)

func TestVerifySave_RejectsTooSmall(t *testing.T) {
	if VerifySave(nil) {
		t.Error("VerifySave(nil) should be false")
	}
	if VerifySave([]byte{0x20}) {
		t.Error("VerifySave([0x20]) should be false (too short for magic)")
	}
}

func TestVerifySave_AcceptsWellFormed(t *testing.T) {
	sav := buildValidSav(t, 6, []byte{0xAA, 0xBB})
	if !VerifySave(sav) {
		t.Error("VerifySave on well-formed v6 sav should be true")
	}
}

func TestVerifySave_RejectsBadMagic(t *testing.T) {
	sav := buildValidSav(t, 6, []byte{0x00})
	sav[0] = 0xFF
	if VerifySave(sav) {
		t.Error("VerifySave with corrupted magic should be false")
	}
}

func TestVerifySave_RejectsUnsupportedVer(t *testing.T) {
	sav := buildValidSav(t, 7, []byte{0x00})
	if VerifySave(sav) {
		t.Error("VerifySave with version=7 should be false")
	}
	sav = buildValidSav(t, 0, []byte{0x00})
	if VerifySave(sav) {
		t.Error("VerifySave with version=0 should be false")
	}
}

func TestVerifySave_RejectsCorruptCRC(t *testing.T) {
	sav := buildValidSav(t, 6, []byte{0xAA})
	sav[len(sav)-1] ^= 0xFF
	if VerifySave(sav) {
		t.Error("VerifySave with corrupted CRC should be false")
	}
}

func TestLoadSave_EmptyByteSliceBootstraps(t *testing.T) {
	p := &Player{}
	if err := LoadSave(p, []byte{}, nil); err != nil {
		t.Fatalf("LoadSave(empty) returned err: %v", err)
	}
	for i := range objtype.PlayerStatCount {
		if i == objtype.PlayerStatHitpoints {
			continue
		}
		if p.stats[i] != 0 || p.baseLevels[i] != 1 || p.levels[i] != 1 {
			t.Errorf("stat %d: got (stats=%d, base=%d, lvl=%d), want (0, 1, 1)",
				i, p.stats[i], p.baseLevels[i], p.levels[i])
		}
	}
	wantHpExp := int32(objtype.GetExpByLevel(10))
	if p.stats[objtype.PlayerStatHitpoints] != wantHpExp {
		t.Errorf("hp stats: got %d, want %d", p.stats[objtype.PlayerStatHitpoints], wantHpExp)
	}
	if p.baseLevels[objtype.PlayerStatHitpoints] != 10 || p.levels[objtype.PlayerStatHitpoints] != 10 {
		t.Errorf("hp levels: got (base=%d, lvl=%d), want (10, 10)",
			p.baseLevels[objtype.PlayerStatHitpoints], p.levels[objtype.PlayerStatHitpoints])
	}
}

func TestLoadSave_NilSliceBootstraps(t *testing.T) {
	p := &Player{}
	if err := LoadSave(p, nil, nil); err != nil {
		t.Fatalf("LoadSave(nil) returned err: %v", err)
	}
	if p.stats[objtype.PlayerStatHitpoints] != int32(objtype.GetExpByLevel(10)) {
		t.Errorf("nil-slice path didn't bootstrap hp like empty-slice path")
	}
}

func mustReadFixture(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "playerloading", name))
	if err != nil {
		t.Fatalf("reading fixture %s: %v", name, err)
	}
	return raw
}

// newTestPlayerForLoadSave returns a Player + InvTypeConfigs seeded
// with two perm-scoped inventories at typeId 0 (size 28) and typeId 1
// (size 14) matching the fixture-generation script. Tests reuse this
// for v1..v6 fixture decode.
func newTestPlayerForLoadSave(t *testing.T) (*Player, *objtype.InvTypeConfigs) {
	t.Helper()
	inv0 := &objtype.InvType{Scope: objtype.InvTypeScopePerm, Size: 28}
	inv0.ID = 0
	inv1 := &objtype.InvType{Scope: objtype.InvTypeScopePerm, Size: 14}
	inv1.ID = 1
	cfgs := &objtype.InvTypeConfigs{
		Configs: []*objtype.InvType{inv0, inv1},
	}
	p := &Player{
		invs: map[int]*inventory.Inventory{
			0: inventory.New(0, 28, inventory.StackNormal),
			1: inventory.New(1, 14, inventory.StackNormal),
		},
	}
	return p, cfgs
}

func TestLoadSave_V1_DecodesHeaderAndBody(t *testing.T) {
	raw := mustReadFixture(t, "v1.sav")
	p, cfgs := newTestPlayerForLoadSave(t)
	if err := LoadSave(p, raw, cfgs); err != nil {
		t.Fatalf("LoadSave(v1): %v", err)
	}
	fv := fixturePlayerValues
	if p.x != fv.X {
		t.Errorf("x: got %d, want %d", p.x, fv.X)
	}
	if p.z != fv.Z {
		t.Errorf("z: got %d, want %d", p.z, fv.Z)
	}
	if p.level != fv.Level {
		t.Errorf("level: got %d, want %d", p.level, fv.Level)
	}
	if p.body != fv.Body {
		t.Errorf("body: got %v, want %v", p.body, fv.Body)
	}
	if p.colors != fv.Colors {
		t.Errorf("colors: got %v, want %v", p.colors, fv.Colors)
	}
	if p.gender != fv.Gender {
		t.Errorf("gender: got %d, want %d", p.gender, fv.Gender)
	}
	if p.runenergy != fv.Runenergy {
		t.Errorf("runenergy: got %d, want %d", p.runenergy, fv.Runenergy)
	}
	if p.playtime != fv.PlaytimeV1 {
		t.Errorf("playtime: got %d, want %d", p.playtime, fv.PlaytimeV1)
	}
	for i, want := range fv.Stats {
		if p.stats[i] != want {
			t.Errorf("stats[%d]: got %d, want %d", i, p.stats[i], want)
		}
	}
	// baseLevels derive from stats[i] via GetLevelByExp(stats[i]).
	for i, xp := range fv.Stats {
		want := uint8(objtype.GetLevelByExp(int(xp)))
		if p.baseLevels[i] != want {
			t.Errorf("baseLevels[%d]: got %d, want %d", i, p.baseLevels[i], want)
		}
	}
	// NOTE: The committed v1.sav fixture has invCount=0 — the tsx
	// fixture-generation script's addInv helper did not populate
	// p.invs in Engine-TS's checkout (Inv API divergence; see
	// testdata/playerloading/README.md "Known gotchas"). Inv-decode
	// pins are covered by TestLoadSave_V1_DecodesInvsSyntheticBuffer.
}

// TestLoadSave_V1_DecodesInvsSyntheticBuffer pins the v1 inv-decode
// path against a hand-crafted SAV buffer because the TS-generated
// v1.sav fixture committed in T2 has invCount=0 (the tsx script's
// addInv helper failed to populate p.invs — see README "Known
// gotchas"). When the fixture is regenerated against a checkout where
// addInv works, this synthetic test can stay as a tighter unit pin
// alongside the (then-richer) fixture-based test.
func TestLoadSave_V1_DecodesInvsSyntheticBuffer(t *testing.T) {
	// Build a minimal v1 SAV with 2 invs at typeIds 0 and 1.
	pkt := packet.NewPacket(make([]byte, 0, 256))
	pkt.P2(SavMagic)
	pkt.P2(1) // version
	pkt.P2(0) // x
	pkt.P2(0) // z
	pkt.P1(0) // level
	for range 7 {
		pkt.P1(0) // body
	}
	for range 5 {
		pkt.P1(0) // colors
	}
	pkt.P1(0)   // gender
	pkt.P2(100) // runenergy
	pkt.P2(0)   // playtime (v1: u16)
	for range 21 {
		pkt.P4(0) // stat exp
		pkt.P1(1) // current level
	}
	pkt.P2(0) // varpCount = 0
	pkt.P1(2) // invCount = 2

	// inv[0]: typeId=0 (size=28 from config), slot 0 → (id=995, count=1000000), slot 4 → (id=1, count=1).
	pkt.P2(0) // typeId
	for slot := range 28 {
		switch slot {
		case 0:
			pkt.P2(996) // id+1
			pkt.P1(255) // extended-count sentinel
			pkt.P4(1000000)
		case 4:
			pkt.P2(2) // id+1 (id=1)
			pkt.P1(1) // count
		default:
			pkt.P2(0) // empty slot
		}
	}
	// inv[1]: typeId=1 (size=14), slot 0 → (id=1038, count=1).
	pkt.P2(1) // typeId
	for slot := range 14 {
		if slot == 0 {
			pkt.P2(1039) // id+1
			pkt.P1(1)    // count
		} else {
			pkt.P2(0)
		}
	}
	// CRC over [0, len).
	body := pkt.Data
	crc := packet.GetCRC(body, 0, len(body))
	pkt.P4(crc)
	sav := pkt.Data

	p, cfgs := newTestPlayerForLoadSave(t)
	if err := LoadSave(p, sav, cfgs); err != nil {
		t.Fatalf("LoadSave(synthetic v1): %v", err)
	}
	// Slot 0: id=995, count=1000000 (count >= 255 → extended-i32).
	item0 := p.invs[0].Items[0]
	if item0 == nil || item0.Id != 995 || item0.Count != 1000000 {
		t.Errorf("inv[0][0]: got %+v, want {Id:995 Count:1000000}", item0)
	}
	item4 := p.invs[0].Items[4]
	if item4 == nil || item4.Id != 1 || item4.Count != 1 {
		t.Errorf("inv[0][4]: got %+v, want {Id:1 Count:1}", item4)
	}
	// Inv 1, slot 0: id=1038, count=1.
	item10 := p.invs[1].Items[0]
	if item10 == nil || item10.Id != 1038 || item10.Count != 1 {
		t.Errorf("inv[1][0]: got %+v, want {Id:1038 Count:1}", item10)
	}
	// Empty slots remain nil.
	if p.invs[0].Items[1] != nil {
		t.Errorf("inv[0][1]: should be nil, got %+v", p.invs[0].Items[1])
	}
	if p.invs[1].Items[1] != nil {
		t.Errorf("inv[1][1]: should be nil, got %+v", p.invs[1].Items[1])
	}
}

func TestLoadSave_V2_DecodesPlaytimeAs4Byte(t *testing.T) {
	raw := mustReadFixture(t, "v2.sav")
	p, cfgs := newTestPlayerForLoadSave(t)
	if err := LoadSave(p, raw, cfgs); err != nil {
		t.Fatalf("LoadSave(v2): %v", err)
	}
	if p.playtime != fixturePlayerValues.PlaytimeV2Plus {
		t.Errorf("playtime: got %d, want %d (v2 must read 4 bytes, not 2)",
			p.playtime, fixturePlayerValues.PlaytimeV2Plus)
	}
}

func TestLoadSave_V3_DecodesAfkZones(t *testing.T) {
	raw := mustReadFixture(t, "v3.sav")
	p, cfgs := newTestPlayerForLoadSave(t)
	if err := LoadSave(p, raw, cfgs); err != nil {
		t.Fatalf("LoadSave(v3): %v", err)
	}
	if p.afkZones != fixturePlayerValues.AfkZones {
		t.Errorf("afkZones: got %v, want %v", p.afkZones, fixturePlayerValues.AfkZones)
	}
	if p.lastAfkZone != fixturePlayerValues.LastAfkZone {
		t.Errorf("lastAfkZone: got %d, want %d", p.lastAfkZone, fixturePlayerValues.LastAfkZone)
	}
}

func TestLoadSave_V4_DecodesChatModes(t *testing.T) {
	raw := mustReadFixture(t, "v4.sav")
	p, cfgs := newTestPlayerForLoadSave(t)
	if err := LoadSave(p, raw, cfgs); err != nil {
		t.Fatalf("LoadSave(v4): %v", err)
	}
	fv := fixturePlayerValues
	if p.publicChat != fv.PublicChat || p.privateChat != fv.PrivateChat || p.tradeDuel != fv.TradeDuel {
		t.Errorf("chat modes: got (pub=%d, priv=%d, trade=%d), want (%d, %d, %d)",
			p.publicChat, p.privateChat, p.tradeDuel,
			fv.PublicChat, fv.PrivateChat, fv.TradeDuel)
	}
}

// TestLoadSave_V5_DecodesPerInvSizeSyntheticBuffer pins the v5+
// per-inv-size branch. T4's TestLoadSave_V1_DecodesInvsSyntheticBuffer
// covers v1-style invType.Size lookup; this test exercises the v5+
// inline u2 size read. Fixture-based pin omitted because v5.sav has
// invCount=0 (see T4 close commit message).
func TestLoadSave_V5_DecodesPerInvSizeSyntheticBuffer(t *testing.T) {
	pkt := packet.NewPacket(make([]byte, 0, 256))
	pkt.P2(SavMagic)
	pkt.P2(5) // version
	pkt.P2(0)
	pkt.P2(0)
	pkt.P1(0) // x, z, level
	for range 7 {
		pkt.P1(0)
	}
	for range 5 {
		pkt.P1(0)
	}
	pkt.P1(0)   // gender
	pkt.P2(100) // runenergy
	pkt.P4(0)   // playtime (v2+ = 4 bytes)
	for range 21 {
		pkt.P4(0)
		pkt.P1(1)
	}
	pkt.P2(0) // varpCount=0
	pkt.P1(1) // invCount=1
	pkt.P2(0) // typeId=0
	pkt.P2(4) // v5+ per-inv size = 4
	// 4 slots: [(id=995, count=10), empty, empty, (id=1, count=1)]
	pkt.P2(996)
	pkt.P1(10) // slot 0: id+1=996, count=10
	pkt.P2(0)  // slot 1: empty
	pkt.P2(0)  // slot 2: empty
	pkt.P2(2)
	pkt.P1(1) // slot 3: id+1=2, count=1

	// v3+ afk: count=0, lastAfkZone=0
	pkt.P1(0) // afkCount
	pkt.P2(0) // lastAfkZone
	// v4+ chat modes packed byte = 0
	pkt.P1(0)

	body := pkt.Data
	crc := packet.GetCRC(body, 0, len(body))
	pkt.P4(crc)
	sav := pkt.Data

	p, cfgs := newTestPlayerForLoadSave(t)
	if err := LoadSave(p, sav, cfgs); err != nil {
		t.Fatalf("LoadSave(synthetic v5): %v", err)
	}
	if item := p.invs[0].Items[0]; item == nil || item.Id != 995 || item.Count != 10 {
		t.Errorf("v5 inv[0][0]: got %+v, want {Id:995 Count:10}", item)
	}
	if item := p.invs[0].Items[3]; item == nil || item.Id != 1 || item.Count != 1 {
		t.Errorf("v5 inv[0][3]: got %+v, want {Id:1 Count:1}", item)
	}
	if p.invs[0].Items[1] != nil || p.invs[0].Items[2] != nil {
		t.Errorf("v5 inv[0] mid slots: should be nil")
	}
}

// buildValidSav constructs a minimal SAV with the given version and
// payload bytes, including a trailing valid CRC. Used by Verify tests.
func buildValidSav(t *testing.T, version uint16, payload []byte) []byte {
	t.Helper()
	p := packet.NewPacket(make([]byte, 0, 16))
	p.P2(SavMagic)
	p.P2(version)
	for _, b := range payload {
		p.P1(b)
	}
	body := append([]byte{}, p.Data...)
	crc := packet.GetCRC(body, 0, len(body))
	out := append(body, 0, 0, 0, 0)
	binary.BigEndian.PutUint32(out[len(body):], crc)
	return out
}
