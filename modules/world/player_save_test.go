package world

import (
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/zsrv/goscape/pkg/inventory"
	"github.com/zsrv/goscape/pkg/io/packet"
	"github.com/zsrv/goscape/pkg/objtype"
)

// seekToVarpBlock walks pkt past the SAV header (magic, version, coords,
// appearance, energy, playtime, 21 stats) so the next read is the u2
// varp count. Fails the test on magic/version mismatch.
func seekToVarpBlock(t *testing.T, pkt *packet.Packet, wantVersion uint16) {
	t.Helper()
	if got := pkt.G2(); got != SavMagic {
		t.Fatalf("magic: got 0x%04x, want 0x%04x", got, SavMagic)
	}
	if got := pkt.G2(); got != wantVersion {
		t.Fatalf("version: got %d, want %d", got, wantVersion)
	}
	pkt.G2() // x
	pkt.G2() // z
	pkt.G1() // level
	for range 7 {
		pkt.G1() // body
	}
	for range 5 {
		pkt.G1() // colors
	}
	pkt.G1() // gender
	pkt.G2() // runenergy
	pkt.G4() // playtime
	for range objtype.PlayerStatCount {
		pkt.G4() // stat xp
		pkt.G1() // level
	}
}

// TestSave_SparseVarps_SkipsTempScopeAndZeros pins the rev-254 A6
// sparse varp save block (TS Player.save @2e3bcf43 Player.ts:215-229):
// p2 count of non-zero SCOPE_PERM varps, then per varp p2(id) +
// pVarInt(value). Non-PERM varps are NEVER saved (subsumes NAI-220's
// v6-era zero-out, which had fixed the %lastcombat / %aggressive_npc
// stale-temp-varp combat bug) and zero-valued PERM varps are skipped.
//
// Test setup: four varps.
//   - Index 0: scope=PERM, value=42 → saved as (id=0, varint 42).
//   - Index 1: scope=TEMP, value=99 → never saved.
//   - Index 2: scope=PERM, value=0  → skipped (zero).
//   - Index 3: scope=PERM, value=-1 → saved as (id=3, varint -1)
//     (pins the negative pVarInt path: 5-byte encoding).
func TestSave_SparseVarps_SkipsTempScopeAndZeros(t *testing.T) {
	p, invTypes := newTestPlayerForLoadSave(t)
	p.varps = []int32{42, 99, 0, -1}

	v0 := &objtype.VarPlayerType{Scope: objtype.VarpScopePerm}
	v0.ID = 0
	v1 := &objtype.VarPlayerType{Scope: objtype.VarpScopeTemp}
	v1.ID = 1
	v2 := &objtype.VarPlayerType{Scope: objtype.VarpScopePerm}
	v2.ID = 2
	v3 := &objtype.VarPlayerType{Scope: objtype.VarpScopePerm}
	v3.ID = 3
	varpTypes := &objtype.VarpTypeConfigs{Configs: []*objtype.VarPlayerType{v0, v1, v2, v3}}

	sav := p.Save(invTypes, varpTypes)

	pkt := packet.NewPacket(sav)
	seekToVarpBlock(t, pkt, SavVersion)

	varpCount := int(pkt.G2())
	if varpCount != 2 {
		t.Fatalf("varpCount: got %d, want 2 (only non-zero PERM varps)", varpCount)
	}
	if id := pkt.G2(); id != 0 {
		t.Errorf("entry 0 id: got %d, want 0", id)
	}
	if v := pkt.GVarInt(); v != 42 {
		t.Errorf("entry 0 value: got %d, want 42", v)
	}
	if id := pkt.G2(); id != 3 {
		t.Errorf("entry 1 id: got %d, want 3 (TEMP id 1 and zero id 2 skipped)", id)
	}
	if v := pkt.GVarInt(); v != -1 {
		t.Errorf("entry 1 value: got %d, want -1 (negative pVarInt)", v)
	}
	// Stream alignment: the next byte is the inv count (2 seeded invs).
	if got := pkt.G1(); got != 2 {
		t.Errorf("byte after varp block: got %d, want invCount=2 — sparse block over/under-wrote", got)
	}
}

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
	// v7 (rev-254 A6 sparse-varp format) is the current version and must
	// verify too.
	sav = buildValidSav(t, 7, []byte{0xAA, 0xBB})
	if !VerifySave(sav) {
		t.Error("VerifySave on well-formed v7 sav should be true")
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
	sav := buildValidSav(t, 8, []byte{0x00})
	if VerifySave(sav) {
		t.Error("VerifySave with version=8 should be false (SavVersion=7 at rev-254 A6)")
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
	if err := LoadSave(p, []byte{}, nil, nil); err != nil {
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
	if err := LoadSave(p, nil, nil, nil); err != nil {
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
		// Production allocates p.varps registry-sized BEFORE LoadSave
		// (initPlayerVarps; TS Player.ts:423) and LoadSave only overlays
		// saved values in. Mirror that contract here: 295 slots = the
		// varp count of the committed *.sav fixtures, so the v6
		// byte-perfect round-trip sees registry count == saved count.
		varps: make([]int32, 295),
	}
	return p, cfgs
}

func TestLoadSave_V1_DecodesHeaderAndBody(t *testing.T) {
	raw := mustReadFixture(t, "v1.sav")
	p, cfgs := newTestPlayerForLoadSave(t)
	if err := LoadSave(p, raw, cfgs, nil); err != nil {
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
	if err := LoadSave(p, sav, cfgs, nil); err != nil {
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
	if err := LoadSave(p, raw, cfgs, nil); err != nil {
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
	if err := LoadSave(p, raw, cfgs, nil); err != nil {
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
	if err := LoadSave(p, raw, cfgs, nil); err != nil {
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
	if err := LoadSave(p, sav, cfgs, nil); err != nil {
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

func TestLoadSave_V6_DecodesLastLoginTime(t *testing.T) {
	raw := mustReadFixture(t, "v6.sav")
	p, cfgs := newTestPlayerForLoadSave(t)
	if err := LoadSave(p, raw, cfgs, nil); err != nil {
		t.Fatalf("LoadSave(v6): %v", err)
	}
	if p.lastLoginTime != fixturePlayerValues.LastLoginTime {
		t.Errorf("lastLoginTime: got %d, want %d", p.lastLoginTime, fixturePlayerValues.LastLoginTime)
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

// TestSave_V6Fixture_MigratesToV7AndRoundTrips replaces the v6
// byte-perfect pin (Save now emits v7 — the sparse varp block can't be
// byte-identical to the dense fixture): load the TS-generated v6
// fixture, re-save (must emit v7), and reload — every field must
// survive the dense→sparse migration. This is the upgrade path every
// pre-A6 save takes on its first post-A6 login.
func TestSave_V6Fixture_MigratesToV7AndRoundTrips(t *testing.T) {
	raw := mustReadFixture(t, "v6.sav")
	p, cfgs := newTestPlayerForLoadSave(t)
	if err := LoadSave(p, raw, cfgs, nil); err != nil {
		t.Fatalf("LoadSave(v6): %v", err)
	}
	// The committed v6.sav fixture has invCount=0 (see the fixture
	// README); clear the test-seeded invs so the re-save matches the
	// fixture player's empty map.
	p.invs = nil
	got := p.Save(cfgs, nil)

	// Header: same magic, NEW version.
	if magic := binary.BigEndian.Uint16(got[0:2]); magic != SavMagic {
		t.Fatalf("re-save magic: got 0x%04x, want 0x%04x", magic, SavMagic)
	}
	if ver := binary.BigEndian.Uint16(got[2:4]); ver != 7 {
		t.Fatalf("re-save version: got %d, want 7 (rev-254 A6)", ver)
	}

	// Reload the v7 bytes into a fresh player and compare all fields.
	q, _ := newTestPlayerForLoadSave(t)
	if err := LoadSave(q, got, cfgs, nil); err != nil {
		t.Fatalf("LoadSave(re-saved v7): %v", err)
	}
	if q.x != p.x || q.z != p.z || q.level != p.level {
		t.Errorf("coords: got (%d,%d,%d), want (%d,%d,%d)", q.x, q.z, q.level, p.x, p.z, p.level)
	}
	if q.body != p.body || q.colors != p.colors || q.gender != p.gender {
		t.Errorf("appearance drifted across v6→v7 migration")
	}
	if q.runenergy != p.runenergy || q.playtime != p.playtime {
		t.Errorf("energy/playtime: got (%d,%d), want (%d,%d)", q.runenergy, q.playtime, p.runenergy, p.playtime)
	}
	if q.stats != p.stats || q.levels != p.levels || q.baseLevels != p.baseLevels {
		t.Errorf("stats drifted across v6→v7 migration")
	}
	if !slices.Equal(q.varps, p.varps) {
		t.Errorf("varps drifted across v6→v7 migration:\n got %v\nwant %v", q.varps, p.varps)
	}
	if q.afkZones != p.afkZones || q.lastAfkZone != p.lastAfkZone {
		t.Errorf("afk zones drifted across v6→v7 migration")
	}
	if q.publicChat != p.publicChat || q.privateChat != p.privateChat || q.tradeDuel != p.tradeDuel {
		t.Errorf("chat modes drifted across v6→v7 migration")
	}
	if q.lastLoginTime != p.lastLoginTime {
		t.Errorf("lastLoginTime: got %d, want %d", q.lastLoginTime, p.lastLoginTime)
	}
}

// TestLoadSave_V6Dense_SyntheticVarps pins the legacy dense varp-block
// decode behind the version gate (PlayerLoading.ts:104-108 @2e3bcf43:
// version < 7 reads varpCount × g4s indexed by position). Old saves
// must keep loading with identical varps after the A6 format change.
func TestLoadSave_V6Dense_SyntheticVarps(t *testing.T) {
	pkt := packet.NewPacket(make([]byte, 0, 256))
	pkt.P2(SavMagic)
	pkt.P2(6) // OLD version: dense varp block
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
	pkt.P4(0)   // playtime (v2+)
	for range 21 {
		pkt.P4(0)
		pkt.P1(1)
	}
	// Dense varp block: count=3, i32 values by position (incl. negative).
	pkt.P2(3)
	pkt.P4(7)
	negThree := int32(-3)
	pkt.P4(uint32(negThree))
	pkt.P4(0)
	pkt.P1(0) // invCount=0
	pkt.P1(0) // v3+: afkCount=0
	pkt.P2(0) // lastAfkZone
	pkt.P1(0) // v4+: chat modes
	pkt.P8(uint64(1718000000000)) // v6+: lastLoginTime
	crc := packet.GetCRC(pkt.Data, 0, len(pkt.Data))
	pkt.P4(crc)
	sav := pkt.Data

	p, cfgs := newTestPlayerForLoadSave(t)
	p.varps = make([]int32, 3)
	if err := LoadSave(p, sav, cfgs, nil); err != nil {
		t.Fatalf("LoadSave(synthetic dense v6): %v", err)
	}
	if want := []int32{7, -3, 0}; !slices.Equal(p.varps, want) {
		t.Errorf("varps: got %v, want %v (dense v6 load must stay positional)", p.varps, want)
	}
	if p.lastLoginTime != 1718000000000 {
		t.Errorf("lastLoginTime: got %d, want 1718000000000 — dense varp decode desynced the stream", p.lastLoginTime)
	}
}

// TestLoadSave_V7_OutOfRangeVarpIDSkipped pins the f4334477 contract on
// the NEW sparse branch: a saved varp id >= the registry size is
// dropped WITHOUT panicking and WITHOUT desyncing the stream (the
// varint value is still consumed). TS parity: Int32Array out-of-range
// writes are silent no-ops (PlayerLoading.ts:100-103 @2e3bcf43).
func TestLoadSave_V7_OutOfRangeVarpIDSkipped(t *testing.T) {
	pkt := packet.NewPacket(make([]byte, 0, 256))
	pkt.P2(SavMagic)
	pkt.P2(7) // NEW version: sparse varp block
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
	pkt.P4(0)   // playtime
	for range 21 {
		pkt.P4(0)
		pkt.P1(1)
	}
	// Sparse varp block: count=3 — in-range id 1, OOB id 300 (registry
	// has 2 slots) with a 5-byte negative varint, then another in-range
	// id 0 AFTER the OOB entry to prove alignment survived.
	pkt.P2(3)
	pkt.P2(1)
	pkt.PVarInt(5)
	pkt.P2(300)
	pkt.PVarInt(-9)
	pkt.P2(0)
	pkt.PVarInt(11)
	pkt.P1(0)                     // invCount=0
	pkt.P1(0)                     // afkCount=0
	pkt.P2(0)                     // lastAfkZone
	pkt.P1(0)                     // chat modes
	pkt.P8(uint64(1718000000001)) // lastLoginTime
	crc := packet.GetCRC(pkt.Data, 0, len(pkt.Data))
	pkt.P4(crc)
	sav := pkt.Data

	p, cfgs := newTestPlayerForLoadSave(t)
	p.varps = make([]int32, 2) // registry smaller than saved id 300
	if err := LoadSave(p, sav, cfgs, nil); err != nil {
		t.Fatalf("LoadSave(v7 with OOB varp id): %v — must skip, not fail", err)
	}
	if want := []int32{11, 5}; !slices.Equal(p.varps, want) {
		t.Errorf("varps: got %v, want %v (OOB id dropped; later entries still applied)", p.varps, want)
	}
	if p.lastLoginTime != 1718000000001 {
		t.Errorf("lastLoginTime: got %d, want 1718000000001 — OOB varint not consumed, stream desynced", p.lastLoginTime)
	}
}

// TestSaveLoad_V7_RoundTripVarpsInclNegatives pins the full
// Save→LoadSave v7 round-trip through the sparse encoder: negative
// values (5-byte varints), max int32, and zero-skip all survive.
func TestSaveLoad_V7_RoundTripVarpsInclNegatives(t *testing.T) {
	src, cfgs := newTestPlayerForLoadSave(t)
	src.varps = []int32{-1000000, 0, 2147483647, -1, 128}
	sav := src.Save(cfgs, nil) // nil varpTypes: all non-zero varps saved

	dst, _ := newTestPlayerForLoadSave(t)
	dst.varps = make([]int32, 5)
	if err := LoadSave(dst, sav, cfgs, nil); err != nil {
		t.Fatalf("LoadSave(v7 round-trip): %v", err)
	}
	if !slices.Equal(dst.varps, src.varps) {
		t.Errorf("varps: got %v, want %v", dst.varps, src.varps)
	}
}

func firstDiff(a, b []byte) int {
	n := min(len(a), len(b))
	for i := range n {
		if a[i] != b[i] {
			return i
		}
	}
	if len(a) != len(b) {
		return n
	}
	return -1
}

func TestSave_InvsWrittenInTypeIDAscOrder(t *testing.T) {
	cfgs := &objtype.InvTypeConfigs{
		Configs: make([]*objtype.InvType, 10),
	}
	for _, id := range []int{1, 2, 5, 7} {
		cfg := &objtype.InvType{Scope: objtype.InvTypeScopePerm, Size: 4}
		cfg.ID = id
		cfgs.Configs[id] = cfg
	}
	p := &Player{
		invs:  map[int]*inventory.Inventory{},
		varps: []int32{},
	}
	// Insert in deliberately non-ascending order. Go map iteration is
	// randomized, so without sort.Ints in Save, this test would be
	// flaky-positive sometimes; sort.Ints makes it deterministic.
	for _, id := range []int{5, 2, 7, 1} {
		p.invs[id] = inventory.New(id, 4, inventory.StackNormal)
	}
	out := p.Save(cfgs, nil)

	// Walk past the fixed-size header to the inv section. Header layout
	// up through invCount byte:
	//   2 magic + 2 version  = 4
	//   2 x + 2 z + 1 level   = 5
	//   7 body + 5 colors     = 12
	//   1 gender + 2 runenergy = 3
	//   4 playtime            = 4
	//   21 * (4 exp + 1 level) = 105
	//   2 varpCount + 0 varps  = 2
	//   = 4 + 5 + 12 + 3 + 4 + 105 + 2 = 135. Next byte = invCount.
	pos := 4 + 5 + 12 + 3 + 4 + 21*(4+1) + 2
	if int(out[pos]) != 4 {
		t.Fatalf("invCount byte at offset %d: got %d, want 4", pos, out[pos])
	}
	pos++
	// Each inv: typeID(2) + capacity(2) + 4 * empty-slot(2 = 0x00 0x00)
	var seen []int
	for range 4 {
		tid := int(out[pos])<<8 | int(out[pos+1])
		seen = append(seen, tid)
		pos += 2     // typeID
		pos += 2     // capacity
		pos += 2 * 4 // 4 empty slots
	}
	want := []int{1, 2, 5, 7}
	for i := range want {
		if seen[i] != want[i] {
			t.Errorf("inv order: got %v, want %v (pins NAI-PLAYERLOADING-D-INVS-SORTED-BY-TYPEID)",
				seen, want)
			break
		}
	}
}

func TestLoadSave_BadMagicReturnsErr(t *testing.T) {
	raw := mustReadFixture(t, "v6.sav")
	raw[0] = 0xFF
	p, cfgs := newTestPlayerForLoadSave(t)
	err := LoadSave(p, raw, cfgs, nil)
	if !errors.Is(err, ErrSavInvalidMagic) {
		t.Errorf("got err=%v, want ErrSavInvalidMagic", err)
	}
}

func TestLoadSave_VersionTooHigh_Err(t *testing.T) {
	raw := mustReadFixture(t, "v6.sav")
	raw[2] = 0x00
	raw[3] = 0x08 // version 8 (one past SavVersion=7)
	// Header mutation invalidates the trailing CRC. Recompute so the
	// version-check arm fires, not the CRC arm.
	binary.BigEndian.PutUint32(raw[len(raw)-4:], packet.GetCRC(raw, 0, len(raw)-4))
	p, cfgs := newTestPlayerForLoadSave(t)
	err := LoadSave(p, raw, cfgs, nil)
	if !errors.Is(err, ErrSavUnsupportedVer) {
		t.Errorf("got err=%v, want ErrSavUnsupportedVer", err)
	}
}

func TestLoadSave_VersionZero_Err(t *testing.T) {
	raw := mustReadFixture(t, "v6.sav")
	raw[2] = 0x00
	raw[3] = 0x00 // version 0
	binary.BigEndian.PutUint32(raw[len(raw)-4:], packet.GetCRC(raw, 0, len(raw)-4))
	p, cfgs := newTestPlayerForLoadSave(t)
	err := LoadSave(p, raw, cfgs, nil)
	if !errors.Is(err, ErrSavUnsupportedVer) {
		t.Errorf("got err=%v, want ErrSavUnsupportedVer", err)
	}
}

func TestLoadSave_CRCMismatch_Err(t *testing.T) {
	raw := mustReadFixture(t, "v6.sav")
	raw[len(raw)-1] ^= 0x01 // flip last CRC byte
	p, cfgs := newTestPlayerForLoadSave(t)
	err := LoadSave(p, raw, cfgs, nil)
	if !errors.Is(err, ErrSavCorrupt) {
		t.Errorf("got err=%v, want ErrSavCorrupt", err)
	}
}

func TestSave_CRCHighBitSet_RoundTrips(t *testing.T) {
	// Construct players varying x until one yields a CRC with the high
	// bit set. Pins that goscape's u32 CRC read/write round-trips
	// identically vs TS's g4s (signed-i32) read.
	cfg := &objtype.InvType{Scope: objtype.InvTypeScopePerm, Size: 4}
	cfg.ID = 0
	cfgs := &objtype.InvTypeConfigs{Configs: []*objtype.InvType{cfg}}
	var savedBytes []byte
	for x := range 65535 {
		p := &Player{
			invs:  map[int]*inventory.Inventory{0: inventory.New(0, 4, inventory.StackNormal)},
			varps: []int32{},
		}
		p.x = x
		out := p.Save(cfgs, nil)
		crc := binary.BigEndian.Uint32(out[len(out)-4:])
		if crc&0x80000000 != 0 {
			savedBytes = out
			break
		}
	}
	if savedBytes == nil {
		t.Fatal("could not find a CRC with high bit set across x=[0..65535)")
	}
	p := &Player{
		invs:  map[int]*inventory.Inventory{0: inventory.New(0, 4, inventory.StackNormal)},
		varps: []int32{},
	}
	if err := LoadSave(p, savedBytes, cfgs, nil); err != nil {
		t.Fatalf("LoadSave(CRC with high bit set): %v — pins TS signedness parity", err)
	}
}

// TestLoadSave_LazyCreatesPermInvsMissingFromDestMap reproduces the
// production logout/login regression: a player's inventory and bank
// contents are dropped after relog because LoadSave silently skips any
// saved typeID not already present in p.invs. The production login flow
// at modules/world/tick.go:232-241 pre-creates only the Worn inventory;
// main inv (typeId 93) and bank (typeId 95) are lazy-created during
// gameplay via server_invs.go GetInventory. So saves contain those
// inventories (Save iterates p.invs verbatim), but loads drop them.
//
// TS Player.getInventory (Engine-TS Player.ts:1415-1439) returns null
// only for inv==-1 or unknown invType; for any valid PERM-scope typeID
// it lazy-creates via Inventory.fromType. Go's LoadSave at
// player_load.go:231-234 deviates with `if !ok { continue }` and a
// comment claiming TS parity — the comment is wrong.
func TestLoadSave_LazyCreatesPermInvsMissingFromDestMap(t *testing.T) {
	// Three perm-scoped inv types: worn-equivalent (0), main inv (93),
	// bank (95). Sizes match real cache.
	worn := &objtype.InvType{Scope: objtype.InvTypeScopePerm, Size: 14}
	worn.ID = 0
	mainInv := &objtype.InvType{Scope: objtype.InvTypeScopePerm, Size: 28}
	mainInv.ID = 93
	bank := &objtype.InvType{Scope: objtype.InvTypeScopePerm, Size: 800}
	bank.ID = 95
	cfgs := &objtype.InvTypeConfigs{Configs: make([]*objtype.InvType, 100)}
	cfgs.Configs[0] = worn
	cfgs.Configs[93] = mainInv
	cfgs.Configs[95] = bank

	// Source player: all three invs present (mirrors a logging-out
	// player whose scripts have lazy-created main inv + bank during
	// gameplay).
	src := &Player{
		invs: map[int]*inventory.Inventory{
			0:  inventory.New(0, 14, inventory.StackNormal),
			93: inventory.New(93, 28, inventory.StackNormal),
			95: inventory.New(95, 800, inventory.StackNormal),
		},
		varps: []int32{},
	}
	src.invs[93].Items[0] = &inventory.Item{Id: 1511, Count: 5}  // logs in slot 0
	src.invs[93].Items[14] = &inventory.Item{Id: 995, Count: 27} // coins
	src.invs[95].Items[0] = &inventory.Item{Id: 1163, Count: 1}  // rune plate
	src.invs[95].Items[42] = &inventory.Item{Id: 4151, Count: 1} // whip

	sav := src.Save(cfgs, nil)

	// Destination player mirrors production login state: empty invs
	// map with only Worn pre-created (matches tick.go:232-241).
	dst := &Player{
		invs:  map[int]*inventory.Inventory{0: inventory.FromType(worn)},
		varps: []int32{},
	}
	if err := LoadSave(dst, sav, cfgs, nil); err != nil {
		t.Fatalf("LoadSave: %v", err)
	}

	mainGot, ok := dst.invs[93]
	if !ok {
		t.Fatalf("main inv (typeId 93) missing from dst.invs after LoadSave — saved data dropped")
	}
	if it := mainGot.Get(0); it == nil || it.Id != 1511 || it.Count != 5 {
		t.Errorf("main inv slot 0: got %+v, want {Id:1511, Count:5}", it)
	}
	if it := mainGot.Get(14); it == nil || it.Id != 995 || it.Count != 27 {
		t.Errorf("main inv slot 14: got %+v, want {Id:995, Count:27}", it)
	}

	bankGot, ok := dst.invs[95]
	if !ok {
		t.Fatalf("bank (typeId 95) missing from dst.invs after LoadSave — saved data dropped")
	}
	if it := bankGot.Get(0); it == nil || it.Id != 1163 || it.Count != 1 {
		t.Errorf("bank slot 0: got %+v, want {Id:1163, Count:1}", it)
	}
	if it := bankGot.Get(42); it == nil || it.Id != 4151 || it.Count != 1 {
		t.Errorf("bank slot 42: got %+v, want {Id:4151, Count:1}", it)
	}
}

// TestLoadSave_SaveShorterThanRegistry_KeepsSeededTail pins the rev-245.2
// live-smoke crash: a save written when the varp registry was smaller (a
// rev-244-era save with 302 varps vs the 245.2 registry's 305) must load
// into the registry-sized, pre-seeded p.varps WITHOUT resizing it — the
// extra slots keep their initPlayerVarps seeds. TS contract: Player.ts:423
// allocates vars registry-sized with per-type seeds (:429-435);
// PlayerLoading.ts:98-101 only overlays saved values. The pre-fix goscape
// LoadSave resized p.varps to the SAVE count, so the post-login varp
// resync loop (tick.go processLogins, iterating the registry) indexed out
// of range and panicked the tick goroutine.
func TestLoadSave_SaveShorterThanRegistry_KeepsSeededTail(t *testing.T) {
	src, cfgs := newTestPlayerForLoadSave(t)
	src.varps = []int32{42, 99} // save carries exactly 2 varps
	sav := src.Save(cfgs, nil)

	dst, _ := newTestPlayerForLoadSave(t)
	// Registry of 4: slots 2-3 seeded like initPlayerVarps would
	// (slot 2: INT-typed → 0; slot 3: non-INT → -1).
	dst.varps = []int32{0, 0, 0, -1}
	if err := LoadSave(dst, sav, cfgs, nil); err != nil {
		t.Fatalf("LoadSave: %v", err)
	}
	want := []int32{42, 99, 0, -1}
	if len(dst.varps) != len(want) {
		t.Fatalf("varps resized to %d, want %d (LoadSave must overlay, not resize)", len(dst.varps), len(want))
	}
	for i := range want {
		if dst.varps[i] != want[i] {
			t.Errorf("varps[%d]: got %d, want %d", i, dst.varps[i], want[i])
		}
	}
}

// TestLoadSave_SaveLongerThanRegistry_DropsExtras pins the inverse
// direction: a save with MORE varps than the registry drops the extras
// (TS Int32Array out-of-range writes are silent no-ops) while keeping the
// stream aligned, so fields decoded after the varp block still land.
// Since rev-254 A6 this exercises the v7 sparse path: ids 2 and 3 are
// beyond the shrunken 2-slot registry and must be skipped.
func TestLoadSave_SaveLongerThanRegistry_DropsExtras(t *testing.T) {
	src, cfgs := newTestPlayerForLoadSave(t)
	src.varps = []int32{7, 8, 9, 10} // save carries 4 varps
	src.playtime = 1234
	sav := src.Save(cfgs, nil)

	dst, _ := newTestPlayerForLoadSave(t)
	dst.varps = make([]int32, 2) // registry shrank to 2
	if err := LoadSave(dst, sav, cfgs, nil); err != nil {
		t.Fatalf("LoadSave: %v", err)
	}
	if len(dst.varps) != 2 || dst.varps[0] != 7 || dst.varps[1] != 8 {
		t.Errorf("varps: got %v, want [7 8] (extras dropped, no resize)", dst.varps)
	}
	// Stream-alignment proof: playtime is decoded BEFORE the varp block
	// and the inventory block AFTER it; both must be intact.
	if dst.playtime != 1234 {
		t.Errorf("playtime: got %d, want 1234", dst.playtime)
	}
	if _, ok := dst.invs[0]; !ok {
		t.Errorf("inv 0 missing after LoadSave — varp-block over/under-read desynced the stream")
	}
}
