package world

import (
	"testing"

	"github.com/zsrv/goscape/pkg/rsbuf"
)

func TestAnimateSetsMask(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.Animate(123, 5)
	if p.masks&rsbuf.MaskAnim == 0 {
		t.Error("MaskAnim bit should be set")
	}
	if p.animID != 123 || p.animDelay != 5 {
		t.Errorf("animID/Delay: got (%d,%d), want (123,5)", p.animID, p.animDelay)
	}
}

func TestSaySetsMask(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.Say([]byte("hi"))
	if p.masks&rsbuf.MaskSay == 0 {
		t.Error("MaskSay bit should be set")
	}
	if string(p.sayText) != "hi" {
		t.Errorf("sayText: got %q, want %q", p.sayText, "hi")
	}
}

func TestChatSetsMask(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.Chat(1, 2, 3, []byte("yo"))
	if p.masks&rsbuf.MaskChat == 0 {
		t.Error("MaskChat bit should be set")
	}
	if p.chatColour != 1 || p.chatEffect != 2 || p.chatRights != 3 {
		t.Errorf("chat flags: got (%d,%d,%d), want (1,2,3)", p.chatColour, p.chatEffect, p.chatRights)
	}
}

func TestShowHitSetsMask(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.ShowHit(10, 1, 40, 50)
	if p.masks&rsbuf.MaskDamage == 0 {
		t.Error("MaskDamage bit should be set")
	}
	if p.damageAmt != 10 || p.curHP != 40 || p.baseHP != 50 {
		t.Errorf("damage fields: %+v", p)
	}
}

func TestFaceCoordMultipliesBy2Plus1(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.FaceCoord(100, 200)
	if p.faceSquareX != 201 || p.faceSquareZ != 401 {
		t.Errorf("faceSquare: got (%d,%d), want (201,401) = (100*2+1, 200*2+1)", p.faceSquareX, p.faceSquareZ)
	}
}

func TestFaceEntity(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.SetFaceEntity(0x8005)
	if p.masks&rsbuf.MaskFaceEntity == 0 {
		t.Error("MaskFaceEntity bit should be set")
	}
	if p.faceEntity != 0x8005 {
		t.Errorf("faceEntity: got %d, want 0x8005", p.faceEntity)
	}
}

func TestResetMasksClearsEphemerals(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.Say([]byte("hi"))
	p.Animate(123, 5)
	p.ShowHit(10, 1, 40, 50)
	p.ResetMasks()
	if p.masks != 0 {
		t.Errorf("masks: got %d, want 0", p.masks)
	}
	if p.sayText != nil {
		t.Error("sayText should be nil after reset")
	}
	if p.damageAmt != -1 {
		t.Errorf("damageAmt: got %d, want -1", p.damageAmt)
	}
	// Persistent (animID, faceEntity) should stay.
	if p.animID != 123 {
		t.Errorf("animID should persist: got %d", p.animID)
	}
}
