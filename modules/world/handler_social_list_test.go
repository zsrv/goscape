package world

import (
	"testing"

	util "github.com/zsrv/goscape/pkg/util/jstring"
)

// commonSocialListSetup wires a player against a server with recording
// bridges and a known username. Returns p, recorder.
func commonSocialListSetup(t *testing.T) (*Player, *recordingBridges) {
	t.Helper()
	s := newTestServer(t)
	p, _ := newTestPlayer(t)
	p.client.server = s
	p.username = "alice"
	rec := installRecordingBridges(s)
	return p, rec
}

// payloadG8 returns an 8-byte big-endian payload encoding the username37
// value, matching what packet.G8 reads.
func payloadG8(v uint64) []byte {
	return []byte{
		byte(v >> 56), byte(v >> 48), byte(v >> 40), byte(v >> 32),
		byte(v >> 24), byte(v >> 16), byte(v >> 8), byte(v),
	}
}

func TestHandleFriendListAddCallsBridgeAndSetsProtect(t *testing.T) {
	p, rec := commonSocialListSetup(t)
	target := util.ToBase37("bob")

	if err := handleFriendListAdd(p, payloadG8(target)); err != nil {
		t.Fatalf("handleFriendListAdd: %v", err)
	}
	if len(rec.friends) != 1 {
		t.Fatalf("friends: got %d calls, want 1", len(rec.friends))
	}
	got := rec.friends[0]
	if got.method != "AddFriend" || got.playerUsername != "alice" || got.targetUsername37 != target {
		t.Errorf("AddFriend record: %+v (want method=AddFriend, user=alice, target=%d)", got, target)
	}
	if !p.socialProtect {
		t.Error("socialProtect: must be true after successful Friend/Ignore call")
	}
}

func TestHandleFriendListDelCallsRemoveFriend(t *testing.T) {
	p, rec := commonSocialListSetup(t)
	target := util.ToBase37("bob")
	if err := handleFriendListDel(p, payloadG8(target)); err != nil {
		t.Fatalf("handleFriendListDel: %v", err)
	}
	if len(rec.friends) != 1 || rec.friends[0].method != "RemoveFriend" {
		t.Errorf("expected one RemoveFriend call, got %+v", rec.friends)
	}
	if !p.socialProtect {
		t.Error("socialProtect not set")
	}
}

func TestHandleIgnoreListAddCallsAddIgnore(t *testing.T) {
	p, rec := commonSocialListSetup(t)
	target := util.ToBase37("bob")
	if err := handleIgnoreListAdd(p, payloadG8(target)); err != nil {
		t.Fatalf("handleIgnoreListAdd: %v", err)
	}
	if len(rec.friends) != 1 || rec.friends[0].method != "AddIgnore" {
		t.Errorf("expected one AddIgnore call, got %+v", rec.friends)
	}
	if !p.socialProtect {
		t.Error("socialProtect not set")
	}
}

func TestHandleIgnoreListDelCallsRemoveIgnore(t *testing.T) {
	p, rec := commonSocialListSetup(t)
	target := util.ToBase37("bob")
	if err := handleIgnoreListDel(p, payloadG8(target)); err != nil {
		t.Fatalf("handleIgnoreListDel: %v", err)
	}
	if len(rec.friends) != 1 || rec.friends[0].method != "RemoveIgnore" {
		t.Errorf("expected one RemoveIgnore call, got %+v", rec.friends)
	}
	if !p.socialProtect {
		t.Error("socialProtect not set")
	}
}

// TestHandleSocialListSocialProtectGate pins {Friend,Ignore}List handlers'
// early-return when socialProtect is already set: no bridge call, no
// re-set (no-op).
func TestHandleSocialListSocialProtectGate(t *testing.T) {
	p, rec := commonSocialListSetup(t)
	p.socialProtect = true
	target := util.ToBase37("bob")

	for _, fn := range []func(*Player, []byte) error{
		handleFriendListAdd, handleFriendListDel,
		handleIgnoreListAdd, handleIgnoreListDel,
	} {
		if err := fn(p, payloadG8(target)); err != nil {
			t.Fatalf("handler error: %v", err)
		}
	}
	if len(rec.friends) != 0 {
		t.Errorf("bridge: got %d calls, want 0 (gated by socialProtect)", len(rec.friends))
	}
}

// TestHandleSocialListInvalidNameGate pins the FromBase37 == "invalid_name"
// gate. Use 37 — multiple of 37 → invalid_name per the T1 jstring fix.
func TestHandleSocialListInvalidNameGate(t *testing.T) {
	p, rec := commonSocialListSetup(t)
	target := uint64(37) // % 37 == 0 → "invalid_name"

	if got := util.FromBase37(target); got != "invalid_name" {
		t.Fatalf("test premise broken: FromBase37(%d) = %q, want invalid_name", target, got)
	}

	if err := handleFriendListAdd(p, payloadG8(target)); err != nil {
		t.Fatalf("handleFriendListAdd: %v", err)
	}
	if len(rec.friends) != 0 {
		t.Errorf("bridge: got %d calls, want 0 (gated by invalid_name)", len(rec.friends))
	}
	if p.socialProtect {
		t.Error("socialProtect: must remain false on invalid_name early-return")
	}
}

// TestHandleSocialListInvalidNameUpperBoundGate pins the upper-bound
// "invalid_name" branch (>= 37**12).
func TestHandleSocialListInvalidNameUpperBoundGate(t *testing.T) {
	p, rec := commonSocialListSetup(t)
	target := uint64(6582952005840035281) // == 37**12

	if err := handleFriendListAdd(p, payloadG8(target)); err != nil {
		t.Fatalf("handleFriendListAdd: %v", err)
	}
	if len(rec.friends) != 0 {
		t.Errorf("bridge: got %d calls, want 0 (gated by upper-bound invalid_name)", len(rec.friends))
	}
}

// TestHandleSocialListNilServerNoOp pins the defensive guard.
func TestHandleSocialListNilServerNoOp(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.client.server = nil
	target := util.ToBase37("bob")

	if err := handleFriendListAdd(p, payloadG8(target)); err != nil {
		t.Errorf("handleFriendListAdd nil-server: got err %v", err)
	}
	if p.socialProtect {
		t.Error("socialProtect: must remain false on nil-server early-return")
	}
}
