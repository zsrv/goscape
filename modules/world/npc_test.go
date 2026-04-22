package world

import (
	"testing"

	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/rsbuf"
)

func newTestNpc(nid int) *Npc {
	typ := &objtype.NpcType{
		ConfigType:  objtype.ConfigType{ID: 0, DebugName: "test"},
		WanderRange: 0,
		RespawnRate: 50,
	}
	return NewNpc(nid, 0, 3094, 3106, 0, typ)
}

func TestNpcAnimateSetsMask(t *testing.T) {
	n := newTestNpc(1)
	n.Animate(123, 5)
	if n.masks&rsbuf.NpcMaskAnim == 0 {
		t.Error("NpcMaskAnim should be set")
	}
	if n.animID != 123 || n.animDelay != 5 {
		t.Errorf("animID/Delay: got (%d,%d), want (123,5)", n.animID, n.animDelay)
	}
}

func TestNpcSaySetsMask(t *testing.T) {
	n := newTestNpc(1)
	n.Say([]byte("hi"))
	if n.masks&rsbuf.NpcMaskSay == 0 {
		t.Error("NpcMaskSay should be set")
	}
	if string(n.sayText) != "hi" {
		t.Errorf("sayText: got %q, want %q", n.sayText, "hi")
	}
}

func TestNpcChangeTypeSetsMask(t *testing.T) {
	n := newTestNpc(1)
	n.ChangeType(42)
	if n.masks&rsbuf.NpcMaskChangeType == 0 {
		t.Error("NpcMaskChangeType should be set")
	}
	if n.changeTypeID != 42 {
		t.Errorf("changeTypeID: got %d, want 42", n.changeTypeID)
	}
}

func TestNpcFaceCoord(t *testing.T) {
	n := newTestNpc(1)
	n.FaceCoord(100, 200)
	if n.faceSquareX != 201 || n.faceSquareZ != 401 {
		t.Errorf("faceSquareX/Z: got (%d,%d), want (201,401)", n.faceSquareX, n.faceSquareZ)
	}
}

func TestNpcResetMasksClearsEphemerals(t *testing.T) {
	n := newTestNpc(1)
	n.Animate(123, 5)
	n.Say([]byte("hi"))
	n.Damage(10, 1)
	n.ResetMasks()

	if n.masks != 0 {
		t.Errorf("masks: got %d, want 0", n.masks)
	}
	if n.sayText != nil {
		t.Error("sayText should be nil after reset")
	}
	if n.damageAmt != -1 {
		t.Errorf("damageAmt: got %d, want -1", n.damageAmt)
	}
	if n.animID != 123 {
		t.Errorf("animID should persist: got %d, want 123", n.animID)
	}
}
