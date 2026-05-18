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
	return p.Save(invTypes)
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
	if err := LoadSave(p, sav, invTypes); err != nil {
		if errors.Is(err, errCloseConn) {
			t.Fatal("LoadSave returned errCloseConn unexpectedly")
		}
		t.Fatalf("LoadSave: %v", err)
	}
}
