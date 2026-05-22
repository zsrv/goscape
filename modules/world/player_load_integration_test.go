package world

import (
	"errors"
	"io"
	"testing"

	io2 "github.com/zsrv/goscape/pkg/io/isaac"
	"github.com/zsrv/goscape/pkg/objtype"
)

// validSAVBytes returns a valid SAV byte slice produced by Player.Save
// for a freshly-seeded player (typeIds 0 & 1, sizes 28 & 14). The bytes
// are NOT pinned here — byte-identity vs the TS-generated fixtures lives
// in player_save_test.go. T5's goal is "valid SAV decodes successfully
// into LoadSave"; round-tripping through Save satisfies that contract.
func validSAVBytes(t *testing.T) []byte {
	t.Helper()
	p, invTypes := newTestPlayerForLoadSave(t)
	return p.Save(invTypes, nil)
}

// runProcessLogins wires p through a Server with the given savePayload
// and runs one processLogins cycle. Uses newTestPlayer (not the plan's
// manual newTestClient + newPlayer) so the encryptor and conn-drain
// pattern matches the existing TestProcessLogins_* tests in
// tick_logins_test.go.
func runProcessLogins(t *testing.T, savePayload []byte, invTypes *objtype.InvTypeConfigs) (*Server, *Player) {
	t.Helper()
	s := newTestServer(t)
	s.invTypes = invTypes

	p, conn := newTestPlayer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	p.client.savePayload = savePayload
	p.username = "test"
	go io.Copy(io.Discard, conn)

	s.playersMu.Lock()
	s.newPlayers = append(s.newPlayers, p)
	s.playersMu.Unlock()
	s.processLogins()
	return s, p
}

func TestProcessLogins_NilSavePayload_BootstrapsFresh(t *testing.T) {
	_, invTypes := newTestPlayerForLoadSave(t)
	s, p := runProcessLogins(t, nil, invTypes)

	if p.slot < 1 || p.slot > 2047 {
		t.Errorf("player not added: slot=%d", p.slot)
	}
	if s.players[p.slot] != p {
		t.Error("players[slot] not set")
	}
	if p.invs == nil {
		t.Error("invs map should be initialised by bootstrap")
	}
	// Worn inventory always seeded post-bootstrap when InvTypes.Worn is set.
	if invTypes.Worn >= 0 {
		if _, ok := p.invs[invTypes.Worn]; !ok {
			t.Error("worn inventory should be initialised on fresh login")
		}
	}
}

func TestProcessLogins_ValidSavePayload_LoadsSuccessfully(t *testing.T) {
	_, invTypes := newTestPlayerForLoadSave(t)
	sav := validSAVBytes(t)

	s, p := runProcessLogins(t, sav, invTypes)

	if p.slot < 1 || p.slot > 2047 {
		t.Errorf("player not added: slot=%d", p.slot)
	}
	if s.players[p.slot] != p {
		t.Error("players[slot] not set")
	}
	if p.invs == nil {
		t.Error("invs map should be initialised")
	}
	// Don't pin coords / varps here — that's the byte-pin tests' job in
	// player_save_test.go. This test pins only the "decode succeeded and
	// player joined" outcome.
}

func TestProcessLogins_CorruptSavePayload_FallsBackToBootstrap(t *testing.T) {
	_, invTypes := newTestPlayerForLoadSave(t)
	// 4-byte payload: bodyLen = len-4 = 0 < 4, so LoadSave returns
	// ErrSavCorrupt at the body-length guard before reaching the magic check.
	corrupt := []byte{0x00, 0x01, 0x02, 0x03}

	// Sanity: VerifySave rejects the corrupt bytes (size < 8 → false).
	if VerifySave(corrupt) {
		t.Fatal("VerifySave should reject 4-byte corrupt payload")
	}
	// processLogins should log + fall back to empty bootstrap (deviation
	// NAI-PLAYERLOADING-D-DECODE-ERR-FALLS-BACK-TO-BOOTSTRAP at tick.go:174).
	s, p := runProcessLogins(t, corrupt, invTypes)

	if p.slot < 1 || p.slot > 2047 {
		t.Errorf("player must still be added on corrupt SAV: slot=%d", p.slot)
	}
	if s.players[p.slot] != p {
		t.Error("players[slot] not set")
	}
	if p.invs == nil {
		t.Error("invs map should be bootstrapped after fallback")
	}
}

// TestLoadSave_PopulatesCombatLevel pins that after LoadSave, the
// player's combatLevel is computed from the loaded baseLevels — NOT
// the constructor default of 3. Retires NAI-PLAYERLOADING-D-COMBAT-
// LEVEL-NOT-RECOMPUTED-ON-LOAD. NAI-184 T5.
//
// Uses a SAV produced by Player.Save with all combat stats at level 99
// (CL=126 per the formula). The load-time recompute is the no-rebuild
// variant: MaskAppearance must NOT be flipped (no client yet).
func TestLoadSave_PopulatesCombatLevel(t *testing.T) {
	src, invTypes := newTestPlayerForLoadSave(t)
	// Override all seven combat-stat XPs to level-99 thresholds; LoadSave
	// derives baseLevels from stats[i] via GetLevelByExp.
	for _, stat := range []int{
		objtype.PlayerStatAttack,
		objtype.PlayerStatDefence,
		objtype.PlayerStatStrength,
		objtype.PlayerStatHitpoints,
		objtype.PlayerStatRanged,
		objtype.PlayerStatPrayer,
		objtype.PlayerStatMagic,
	} {
		src.stats[stat] = int32(objtype.GetExpByLevel(99))
	}
	sav := src.Save(invTypes, nil)

	dst := &Player{}
	dst.combatLevel = 3 // constructor default; LoadSave must overwrite it
	dst.masks = 0
	if err := LoadSave(dst, sav, invTypes, nil); err != nil {
		t.Fatalf("LoadSave: %v", err)
	}
	if dst.baseLevels[objtype.PlayerStatStrength] != 99 {
		t.Fatalf("precondition: baseLevels[STR]: got %d, want 99",
			dst.baseLevels[objtype.PlayerStatStrength])
	}
	if dst.combatLevel != 126 {
		t.Errorf("combatLevel: got %d, want 126 (maxed combat stats)", dst.combatLevel)
	}
	if dst.masks&MaskAppearance != 0 {
		t.Errorf("masks: MaskAppearance unexpectedly set by LoadSave (load uses triggerRebuild=false)")
	}
}

// TestLoadSave_CombatLevelFlowsToHuntTooStrongGate is the consumer-side
// pin for NAI-184 T5: it verifies that the combatLevel populated by
// LoadSave is what npc_hunt.go's CheckNotTooStrong gate (TS Npc.ts:939-941
// → npc_hunt.go:172-179) actually reads. Pre-NAI-184, a freshly-loaded
// player's combatLevel would have been the constructor default (3) and a
// strong loaded player would have leaked into hunted; post-T5, LoadSave's
// recompute makes the gate fire on maxed-stat save data.
func TestLoadSave_CombatLevelFlowsToHuntTooStrongGate(t *testing.T) {
	// SAV with maxed combat stats (CL=126) at non-wilderness coords.
	// z=3500 sits south of the wilderness rect (which starts at z=3520),
	// so IsInWilderness()=false and the OutsideWilderness filter applies.
	src, invTypes := newTestPlayerForLoadSave(t)
	src.x = 3203
	src.z = 3500
	src.level = 0
	for _, stat := range []int{
		objtype.PlayerStatAttack,
		objtype.PlayerStatDefence,
		objtype.PlayerStatStrength,
		objtype.PlayerStatHitpoints,
		objtype.PlayerStatRanged,
		objtype.PlayerStatPrayer,
		objtype.PlayerStatMagic,
	} {
		src.stats[stat] = int32(objtype.GetExpByLevel(99))
	}
	sav := src.Save(invTypes, nil)

	dst, _ := newTestPlayerForLoadSave(t)
	dst.combatLevel = 3 // constructor default; LoadSave must overwrite it
	if err := LoadSave(dst, sav, invTypes, nil); err != nil {
		t.Fatalf("LoadSave: %v", err)
	}
	if dst.combatLevel != 126 {
		t.Fatalf("precondition: combatLevel after LoadSave: got %d, want 126", dst.combatLevel)
	}

	// Register the loaded player into a Server's zone map so huntPlayers
	// can find it via Zone subscription (post-NAI-28).
	s := newTestServer(t)
	dst.slot = 1
	dst.active = true
	s.players[dst.slot] = dst
	zn := s.zoneMap.Get(dst.level, dst.x, dst.z)
	dst.zoneListElement = zn.EnterPlayer(dst, nil)

	npcType := &objtype.NpcType{ConfigType: objtype.ConfigType{ID: 1}, Size: 1, Category: -1, VisLevel: 30}
	s.npcTypes = &objtype.NPCTypeConfigs{Configs: []*objtype.NpcType{nil, npcType}}
	n := NewNpc(1, 1, dst.x, dst.z, dst.level, npcType)
	n.server = s
	n.huntRange = 5

	hunt := &objtype.HuntType{
		CheckNotTooStrong:  objtype.HuntCheckNotTooStrongOutsideWilderness,
		CheckVis:           objtype.HuntVisOff,
		CheckInv:           -1,
		CheckNotCombat:     -1,
		CheckNotCombatSelf: -1,
	}
	hunted := n.huntPlayers(s, hunt)

	if len(hunted) != 0 {
		t.Errorf("hunted: got %d, want 0 (CL=126 from LoadSave > 2*VisLevel=60 must filter)", len(hunted))
	}
}

// TestValidSAVBytesRoundTrips ensures validSAVBytes itself round-trips
// cleanly so the valid-savePayload test above isn't testing a no-op.
func TestValidSAVBytesRoundTrips(t *testing.T) {
	_, invTypes := newTestPlayerForLoadSave(t)
	sav := validSAVBytes(t)
	if len(sav) == 0 {
		t.Fatal("validSAVBytes returned empty")
	}
	if !VerifySave(sav) {
		t.Fatalf("VerifySave(validSAVBytes()) = false; want true")
	}
	// LoadSave into a fresh player must succeed.
	p, _ := newTestPlayerForLoadSave(t)
	if err := LoadSave(p, sav, invTypes, nil); err != nil {
		if errors.Is(err, errCloseConn) {
			t.Fatal("LoadSave returned errCloseConn unexpectedly")
		}
		t.Fatalf("LoadSave: %v", err)
	}
}
