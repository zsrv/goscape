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

func TestPlayerDamageDecrementsHitpointsAndSetsMask(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.baseLevels[3] = 50
	p.levels[3] = 50
	p.Damage(10, 1)
	if p.masks&rsbuf.MaskDamage == 0 {
		t.Error("MaskDamage bit should be set")
	}
	if p.damageAmt != 10 {
		t.Errorf("damageAmt: got %d, want 10", p.damageAmt)
	}
	if p.damageType != 1 {
		t.Errorf("damageType: got %d, want 1", p.damageType)
	}
	if p.levels[3] != 40 {
		t.Errorf("levels[3]: got %d, want 40", p.levels[3])
	}
	if p.baseLevels[3] != 50 {
		t.Errorf("baseLevels[3]: got %d, want 50 (unchanged)", p.baseLevels[3])
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
	p.baseLevels[3] = 50
	p.levels[3] = 50
	p.Say([]byte("hi"))
	p.Animate(123, 5)
	p.Damage(10, 1)
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
	// Persistent (animID, faceEntity, levels[3], baseLevels[3]) should stay.
	if p.animID != 123 {
		t.Errorf("animID should persist: got %d", p.animID)
	}
	if p.levels[3] != 40 {
		t.Errorf("levels[3] should persist after ResetMasks (S6e): got %d, want 40", p.levels[3])
	}
}

func TestPlayerDamageClampsAtZero(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.baseLevels[3] = 10
	p.levels[3] = 2
	p.Damage(5, 1)
	if p.levels[3] != 0 {
		t.Errorf("levels[3]: got %d, want 0 (clamped)", p.levels[3])
	}
	// damageAmt clamps to pre-hit current — matches TS Player.applyDamage
	// (Player.ts:1865-1867: hitmarkDamage = current on overkill).
	if p.damageAmt != 2 {
		t.Errorf("damageAmt: got %d, want 2 (clamped to pre-hit current)", p.damageAmt)
	}
}

func TestPlayerDamageNegativeAmountClampsToZero(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.baseLevels[3] = 50
	p.levels[3] = 50
	p.Damage(-3, 1)
	if p.levels[3] != 50 {
		t.Errorf("levels[3]: got %d, want 50 (negative amount must not heal)", p.levels[3])
	}
	if p.damageAmt != 0 {
		t.Errorf("damageAmt: got %d, want 0 (negative clamped)", p.damageAmt)
	}
	if p.masks&rsbuf.MaskDamage == 0 {
		t.Error("MaskDamage bit should still be set on zero damage (debug signal)")
	}
}

func TestPlayerHPPersistsAcrossResetMasks(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.baseLevels[3] = 50
	p.levels[3] = 50
	p.Damage(3, 1)
	if p.levels[3] != 47 {
		t.Fatalf("pre-reset levels[3]: got %d, want 47", p.levels[3])
	}
	p.ResetMasks()
	if p.levels[3] != 47 {
		t.Errorf("post-reset levels[3]: got %d, want 47 (persistent)", p.levels[3])
	}
	if p.damageAmt != -1 {
		t.Errorf("damageAmt: got %d, want -1 (per-tick cleared)", p.damageAmt)
	}
}

func TestPlayerResetHP(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.baseLevels[3] = 50
	p.levels[3] = 30
	p.ResetHP()
	if p.levels[3] != 50 {
		t.Errorf("levels[3] after ResetHP: got %d, want 50", p.levels[3])
	}
	if p.baseLevels[3] != 50 {
		t.Errorf("baseLevels[3]: got %d, want 50 (unchanged)", p.baseLevels[3])
	}
}

func TestPlayerCurHPAndBaseHPDeriveFromLevels(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.baseLevels[3] = 50
	p.levels[3] = 35
	if got := p.CurHP(); got != 35 {
		t.Errorf("CurHP(): got %d, want 35", got)
	}
	if got := p.BaseHP(); got != 50 {
		t.Errorf("BaseHP(): got %d, want 50", got)
	}
}

func TestPlayerDamageWithBoostedHitpoints(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.baseLevels[3] = 10
	p.levels[3] = 14 // boosted via brew/etc
	p.Damage(3, 1)
	if p.levels[3] != 11 {
		t.Errorf("boosted levels[3] after Damage: got %d, want 11", p.levels[3])
	}
	if p.damageAmt != 3 {
		t.Errorf("damageAmt: got %d, want 3", p.damageAmt)
	}
	if p.baseLevels[3] != 10 {
		t.Errorf("baseLevels[3]: got %d, want 10 (boost doesn't touch base)", p.baseLevels[3])
	}
	// ResetHP restores to base, not boosted-max — RS2 respawn semantics.
	p.ResetHP()
	if p.levels[3] != 10 {
		t.Errorf("levels[3] after ResetHP: got %d, want 10 (restored to base)", p.levels[3])
	}
}

func TestPlayerDamageOnZeroHP(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.baseLevels[3] = 10
	p.levels[3] = 0
	p.Damage(5, 1)
	if p.levels[3] != 0 {
		t.Errorf("levels[3]: got %d, want 0 (already dead, stays dead)", p.levels[3])
	}
	if p.damageAmt != 0 {
		t.Errorf("damageAmt: got %d, want 0 (clamped to current=0)", p.damageAmt)
	}
	if p.masks&rsbuf.MaskDamage == 0 {
		t.Error("MaskDamage bit should still flip on zero damage")
	}
}

// TestNewPlayerSetsEntityMaskToMaskFaceEntity — NAI-14 Task 3.
// Mirrors TS PathingEntity.ts:107 where `this.entitymask = entitymask`
// is set at construction. For Player, this is rsbuf.MaskFaceEntity.
// Parallel of NAI-13's TestNewNpcSetsEntityMaskToFaceEntity on the NPC
// side — closes the Player-side latent-no-op where p.entitymask was
// declared at player.go:115 but never assigned.
//
// No `p.masks |= p.entitymask` sites exist today on the Player side
// (grep-verified), so this assignment is structural future-proofing.
// Future Player-interaction port sub-specs that need the face-entity
// mask bit should use `p.entitymask` (not hardcode `rsbuf.MaskFaceEntity`).
func TestNewPlayerSetsEntityMaskToMaskFaceEntity(t *testing.T) {
	p, _ := newTestPlayer(t)
	if p.entitymask != rsbuf.MaskFaceEntity {
		t.Errorf("entitymask: got %d, want %d (MaskFaceEntity)", p.entitymask, rsbuf.MaskFaceEntity)
	}
}
