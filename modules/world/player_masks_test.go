package world

import (
	"testing"

	"github.com/zsrv/goscape/pkg/coordgrid"
	"github.com/zsrv/goscape/pkg/rsbuf"
)

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
	p.animID = 123
	p.animDelay = 5
	p.masks |= rsbuf.MaskAnim
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
	// animID/animDelay are per-tick: ResetMasks clears them to -1 (TS
	// PathingEntity.ts:598-601) so an animation can replay on a later tick.
	if p.animID != -1 {
		t.Errorf("animID should reset to -1: got %d", p.animID)
	}
	if p.animDelay != -1 {
		t.Errorf("animDelay should reset to -1: got %d", p.animDelay)
	}
	// Persistent (levels[3], baseLevels[3]) and conditionally-persistent
	// faceEntity (target=nil/faceEntity=-1 here, so trailing-clear no-ops) should stay.
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

// === NAI-108: Player.ResetMasks trailing-clear + chat metadata reset ===
//
// Mirrors NPC-side coverage at npc_masks_test.go:230-281.

// TestPlayerResetMasksTrailingClearFires — NAI-108 Task 1.
// When p.target is nil but p.faceEntity still holds a prior NPC slot,
// ResetMasks emits MaskFaceEntity and snaps faceEntity to -1, mirroring
// TS PathingEntity.ts:611-614. Closes the NAI-91 smoke-surfaced
// "player keeps facing NPC after walking away" symptom.
func TestPlayerResetMasksTrailingClearFires(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.target = nil
	p.faceEntity = 42
	p.masks = 0
	p.ResetMasks()
	if p.faceEntity != -1 {
		t.Errorf("faceEntity: got %d, want -1 (trailing clear should run)", p.faceEntity)
	}
	if p.masks&rsbuf.MaskFaceEntity == 0 {
		t.Error("masks & MaskFaceEntity: got 0, want nonzero (trailing clear should emit)")
	}
}

// TestPlayerResetMasksTrailingClearSkippedWhenTargetPresent — NAI-108 Task 1.
// Quirk guard: trailing clear must not fire when p.target is non-nil
// (the player is still facing someone, by design). Pattern mirrors NPC
// test at npc_masks_test.go:254-267.
func TestPlayerResetMasksTrailingClearSkippedWhenTargetPresent(t *testing.T) {
	p, _ := newTestPlayer(t)
	other, _ := newTestPlayer(t)
	p.target = other
	p.faceEntity = 42
	p.masks = 0
	p.ResetMasks()
	if p.faceEntity != 42 {
		t.Errorf("faceEntity: got %d, want 42 (trailing clear should be skipped — target present)", p.faceEntity)
	}
	if p.masks&rsbuf.MaskFaceEntity != 0 {
		t.Error("masks & MaskFaceEntity: got nonzero, want 0 (trailing clear should not emit — target present)")
	}
}

// TestPlayerResetMasksTrailingClearSkippedWhenFaceEntityAlreadyMinusOne — NAI-108 Task 1.
// Quirk guard: trailing clear must not fire when faceEntity is already
// -1 (no pending clear to sync). Pattern mirrors NPC test at
// npc_masks_test.go:272-281.
func TestPlayerResetMasksTrailingClearSkippedWhenFaceEntityAlreadyMinusOne(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.target = nil
	p.faceEntity = -1
	p.masks = 0
	p.ResetMasks()
	if p.masks != 0 {
		t.Errorf("masks: got 0x%x, want 0 (trailing clear should be skipped — faceEntity already -1)", p.masks)
	}
}

// TestPlayerResetMasksClearsChatMetadata — NAI-108 Task 1.
// TS Player.resetEntity at Player.ts:461-463 nulls chatColour/Effect/Rights
// each tick. Goscape resets to -1 (the sentinel used at newPlayer init,
// player.go:494-496) for TS-fidelity. The encoder gates on chatBytes != nil
// (tick.go:423), so this reset is observably-no-op; pinning it preserves
// future TS-faithfulness if a non-gated reader is added.
func TestPlayerResetMasksClearsChatMetadata(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.chatColour = 5
	p.chatEffect = 3
	p.chatRights = 2
	p.ResetMasks()
	if p.chatColour != -1 {
		t.Errorf("chatColour: got %d, want -1", p.chatColour)
	}
	if p.chatEffect != -1 {
		t.Errorf("chatEffect: got %d, want -1", p.chatEffect)
	}
	if p.chatRights != -1 {
		t.Errorf("chatRights: got %d, want -1", p.chatRights)
	}
}

// TestResetMasksResetsMoveSpeedToDefault pins that ResetMasks resets moveSpeed to
// defaultMoveSpeed() each tick, mirroring TS PathingEntity.resetPathingEntity at
// PathingEntity.ts:578.
func TestResetMasksResetsMoveSpeedToDefault(t *testing.T) {
	t.Run("Instant→Walk when run=0", func(t *testing.T) {
		p, _ := newTestPlayer(t)
		p.run = 0
		p.moveSpeed = MoveSpeedInstant

		p.ResetMasks()

		if p.moveSpeed != MoveSpeedWalk {
			t.Errorf("moveSpeed: got %v, want MoveSpeedWalk (run=0 → Walk)", p.moveSpeed)
		}
	})
	t.Run("Instant→Run when run=1", func(t *testing.T) {
		p, _ := newTestPlayer(t)
		p.run = 1
		p.moveSpeed = MoveSpeedInstant

		p.ResetMasks()

		if p.moveSpeed != MoveSpeedRun {
			t.Errorf("moveSpeed: got %v, want MoveSpeedRun (run=1 → Run)", p.moveSpeed)
		}
	})
	t.Run("Walk→Walk when run=0 (idempotent)", func(t *testing.T) {
		p, _ := newTestPlayer(t)
		p.run = 0
		p.moveSpeed = MoveSpeedWalk

		p.ResetMasks()

		if p.moveSpeed != MoveSpeedWalk {
			t.Errorf("moveSpeed: got %v, want MoveSpeedWalk (idempotent)", p.moveSpeed)
		}
	})
}

// TestPlayerResetMasksChatMetadataResetIsNoOpWithoutChatBytes — NAI-108 Task 1.
// Regression pin for the spec §3 (ε) "chat reset is functionally inert"
// claim. Asserts that with chatBytes nil (the encoder gate), arbitrary
// pre-reset chatColour/Effect/Rights values do not cause a chat packet
// to be emitted on the next tick. This is the structural reason the
// chat reset is TS-fidelity polish, not a behavior change.
//
// We assert via the in-place mask state: chatBytes nil → MaskChat must
// not fire from ResetMasks. ResetMasks itself never sets MaskChat (only
// Chat() does, in player_masks.go:18). After ResetMasks runs, p.chatBytes
// is still nil and p.masks must not carry MaskChat regardless of color
// pre-state. This pins the encoder-gate equivalence claim without
// requiring the full tick.go:423 chat-encode path.
func TestPlayerResetMasksChatMetadataResetIsNoOpWithoutChatBytes(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.chatBytes = nil
	p.chatColour = 99
	p.chatEffect = 99
	p.chatRights = 99
	p.masks = 0
	p.ResetMasks()
	if p.chatBytes != nil {
		t.Error("chatBytes: should remain nil after ResetMasks")
	}
	if p.masks&rsbuf.MaskChat != 0 {
		t.Errorf("masks & MaskChat: got nonzero, want 0 (chat reset must not synthesize MaskChat)")
	}
}

// TestResetMasksResetsFaceSquare pins D1: faceSquareX/Z reset to -1 every tick
// (TS PathingEntity.ts:608-609). FACE_COORD is a one-tick event — the prior
// "persistent-by-design" behavior leaked a walked-away-from square to every
// newly-visible observer via the rsbuf low-def forced FACE_COORD. After the
// reset, effectiveFaceCoord falls back to the per-step-maintained faceAngle.
func TestResetMasksResetsFaceSquare(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.x, p.z, p.level = 3200, 3200, 0
	p.unfocus() // south baseline, as on spawn

	p.FaceSquare(3210, 3210) // script FACE_COORD this tick
	if p.faceSquareX == -1 {
		t.Fatal("setup: FaceSquare should set faceSquareX")
	}
	// effectiveFaceCoord reflects the active square this tick (what processInfo reads).
	if x, _ := p.effectiveFaceCoord(); x != p.faceSquareX {
		t.Fatalf("this-tick effectiveFaceCoord should be the square: got %d", x)
	}

	p.ResetMasks() // tick end

	if p.faceSquareX != -1 || p.faceSquareZ != -1 {
		t.Errorf("faceSquare not reset: got (%d,%d), want (-1,-1)", p.faceSquareX, p.faceSquareZ)
	}
	// After reset, effectiveFaceCoord must fall back to faceAngle (which TS
	// PathingEntity.focus(...,true) wrote to the focus coord too — see
	// player-core-1/pathing-4 fix in FaceSquare). The reset surface clears
	// faceSquare for the next tick but leaves faceAngle as the persistent
	// resting orientation.
	gotX, gotZ := p.effectiveFaceCoord()
	if gotX != p.faceAngleX || gotZ != p.faceAngleZ {
		t.Errorf("after reset effectiveFaceCoord should fall back to faceAngle (%d,%d); got (%d,%d)",
			p.faceAngleX, p.faceAngleZ, gotX, gotZ)
	}
}

// TestFaceSquare_WritesFaceAngle pins player-core-1 / pathing-4: TS
// Player.faceSquare (Player.ts:1898-1900) calls focus(fineX,fineZ,client=true).
// PathingEntity.focus (PathingEntity.ts:321-333) unconditionally writes
// faceAngleX/Z BEFORE the client gate, then writes faceSquareX/Z inside it.
// goscape's pre-fix FaceSquare only wrote faceSquareX/Z and the mask, so a
// P_FACESQUARE script without a follow-up walk step left faceAngle stuck at
// its prior value — typically south from unfocus(). After ResetMasks clears
// faceSquare, effectiveFaceCoord falls back to that stale southern faceAngle,
// silently re-orienting the entity for the next tick's forced FACE_COORD.
func TestFaceSquare_WritesFaceAngle(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.x, p.z, p.level = 3200, 3200, 0
	p.unfocus() // south baseline → faceAngleX/Z point to (Fine(3200,1), Fine(3199,1))

	// Capture the southern faceAngle to assert it actually changes.
	preX, preZ := p.faceAngleX, p.faceAngleZ

	p.FaceSquare(3210, 3210)

	wantX := 3210*2 + 1 // 6421 (TS CoordGrid.fine(3210, 1))
	wantZ := 3210*2 + 1 // 6421

	if p.faceAngleX != wantX || p.faceAngleZ != wantZ {
		t.Errorf("FaceSquare must overwrite faceAngleX/Z to the focus coord (TS PathingEntity.focus L325-326): "+
			"got (%d,%d), want (%d,%d) (pre-call was the southern (%d,%d))",
			p.faceAngleX, p.faceAngleZ, wantX, wantZ, preX, preZ)
	}
	if p.faceSquareX != wantX || p.faceSquareZ != wantZ {
		t.Errorf("FaceSquare must also write faceSquareX/Z (TS PathingEntity.focus L329-330): "+
			"got (%d,%d), want (%d,%d)", p.faceSquareX, p.faceSquareZ, wantX, wantZ)
	}
}

// === rev-244 B2 T13: damage2 + hitmarkSlot alternation (Player fork) ===
//
// TS contract: PathingEntity.ts:92-96 (244) adds hitmarkSlot=0, hitmark2Damage=-1,
// hitmark2Type=-1. Player.ts:1871-1890 (244) applyDamage: slot%2==0 → DAMAGE (first),
// slot%2==1 → DAMAGE2 (second); hitmarkSlot++ always. Reset: PathingEntity.ts:606-610
// resets hitmark2Damage/Type to -1 AND hitmarkSlot to 0.

// TestPlayerDamage2AlternationSlot0SetsDamage pins that the first call to
// Damage sets damageAmt/damageType + MaskDamage (slot%2==0 → DAMAGE path).
// TS Player.ts:1884-1887 (244).
func TestPlayerDamage2AlternationSlot0SetsDamage(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.baseLevels[3] = 50
	p.levels[3] = 50
	// hitmarkSlot=0 on fresh player.
	p.Damage(10, 1)
	if p.masks&rsbuf.MaskDamage == 0 {
		t.Error("slot 0: MaskDamage should be set (TS Player.ts:1887)")
	}
	if p.masks&rsbuf.MaskDamage2 != 0 {
		t.Error("slot 0: MaskDamage2 must NOT be set (wrong slot)")
	}
	if p.damageAmt != 10 {
		t.Errorf("slot 0: damageAmt: got %d, want 10", p.damageAmt)
	}
	if p.damageType != 1 {
		t.Errorf("slot 0: damageType: got %d, want 1", p.damageType)
	}
	if p.damage2Amt != -1 {
		t.Errorf("slot 0: damage2Amt: got %d, want -1 (untouched)", p.damage2Amt)
	}
	if p.hitmarkSlot != 1 {
		t.Errorf("slot 0: hitmarkSlot: got %d, want 1 (incremented)", p.hitmarkSlot)
	}
}

// TestPlayerDamage2AlternationSlot1SetsDamage2 pins that the second call to
// Damage sets damage2Amt/damage2Type + MaskDamage2 (slot%2==1 → DAMAGE2 path).
// TS Player.ts:1880-1883 (244).
func TestPlayerDamage2AlternationSlot1SetsDamage2(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.baseLevels[3] = 50
	p.levels[3] = 50
	p.Damage(10, 1) // slot 0 → DAMAGE
	p.Damage(5, 2)  // slot 1 → DAMAGE2
	if p.masks&rsbuf.MaskDamage2 == 0 {
		t.Error("slot 1: MaskDamage2 should be set (TS Player.ts:1883)")
	}
	if p.damage2Amt != 5 {
		t.Errorf("slot 1: damage2Amt: got %d, want 5", p.damage2Amt)
	}
	if p.damage2Type != 2 {
		t.Errorf("slot 1: damage2Type: got %d, want 2", p.damage2Type)
	}
	// MaskDamage set on first call should still be on.
	if p.masks&rsbuf.MaskDamage == 0 {
		t.Error("slot 1: MaskDamage from slot-0 call should still be set")
	}
	if p.hitmarkSlot != 2 {
		t.Errorf("slot 1: hitmarkSlot: got %d, want 2 (incremented twice)", p.hitmarkSlot)
	}
}

// TestPlayerDamage2AlternationSlot2OverwritesDamage pins the slot-2 wrap-around:
// a third call overwrites damageAmt (same DAMAGE slot as slot 0). TS
// Player.ts:1880: `hitmarkSlot % 2 === 1` is false for slot 2, so DAMAGE is
// set again.
func TestPlayerDamage2AlternationSlot2OverwritesDamage(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.baseLevels[3] = 50
	p.levels[3] = 50
	p.Damage(10, 1) // slot 0 → DAMAGE=10
	p.Damage(5, 2)  // slot 1 → DAMAGE2=5
	p.Damage(3, 3)  // slot 2 → DAMAGE again (overwrite)
	if p.damageAmt != 3 {
		t.Errorf("slot 2: damageAmt should be overwritten to 3; got %d", p.damageAmt)
	}
	if p.damageType != 3 {
		t.Errorf("slot 2: damageType should be overwritten to 3; got %d", p.damageType)
	}
	if p.hitmarkSlot != 3 {
		t.Errorf("slot 2: hitmarkSlot: got %d, want 3", p.hitmarkSlot)
	}
}

// TestPlayerResetMasksClearsDamage2AndHitmarkSlot pins the per-tick reset:
// ResetMasks clears damage2Amt/damage2Type to -1 AND hitmarkSlot to 0.
// TS PathingEntity.ts:606-610 (244).
func TestPlayerResetMasksClearsDamage2AndHitmarkSlot(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.baseLevels[3] = 50
	p.levels[3] = 50
	p.Damage(10, 1) // slot 0
	p.Damage(5, 2)  // slot 1 → damage2 set
	p.ResetMasks()
	if p.damage2Amt != -1 {
		t.Errorf("damage2Amt after ResetMasks: got %d, want -1", p.damage2Amt)
	}
	if p.damage2Type != -1 {
		t.Errorf("damage2Type after ResetMasks: got %d, want -1", p.damage2Type)
	}
	if p.hitmarkSlot != 0 {
		t.Errorf("hitmarkSlot after ResetMasks: got %d, want 0 (TS PathingEntity.ts:610)", p.hitmarkSlot)
	}
}

// TestPlayerDamage2AmtAccessorReturnsField pins that Damage2Amt() returns the
// real damage2Amt field (not the -1 placeholder).
func TestPlayerDamage2AmtAccessorReturnsField(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.baseLevels[3] = 50
	p.levels[3] = 50
	p.Damage(10, 1) // slot 0
	p.Damage(7, 2)  // slot 1 → damage2Amt=7
	if p.Damage2Amt() != 7 {
		t.Errorf("Damage2Amt(): got %d, want 7", p.Damage2Amt())
	}
	if p.Damage2Type() != 2 {
		t.Errorf("Damage2Type(): got %d, want 2", p.Damage2Type())
	}
}

// TestPlayerDamage2InitiallyMinusOne pins that a freshly constructed player
// has damage2Amt=-1, damage2Type=-1, hitmarkSlot=0.
func TestPlayerDamage2InitiallyMinusOne(t *testing.T) {
	p, _ := newTestPlayer(t)
	if p.damage2Amt != -1 {
		t.Errorf("damage2Amt initial: got %d, want -1", p.damage2Amt)
	}
	if p.damage2Type != -1 {
		t.Errorf("damage2Type initial: got %d, want -1", p.damage2Type)
	}
	if p.hitmarkSlot != 0 {
		t.Errorf("hitmarkSlot initial: got %d, want 0", p.hitmarkSlot)
	}
}

// TestMaskDamage2ConstantsMatchRsbuf pins that modules/world MaskDamage2 and
// NpcMaskDamage2 equal the rsbuf counterparts (both blocks must stay in sync).
func TestMaskDamage2ConstantsMatchRsbuf(t *testing.T) {
	if MaskDamage2 != rsbuf.MaskDamage2 {
		t.Errorf("world.MaskDamage2=%d != rsbuf.MaskDamage2=%d", MaskDamage2, rsbuf.MaskDamage2)
	}
	if NpcMaskDamage2 != rsbuf.NpcMaskDamage2 {
		t.Errorf("world.NpcMaskDamage2=%d != rsbuf.NpcMaskDamage2=%d", NpcMaskDamage2, rsbuf.NpcMaskDamage2)
	}
}

// TestApplyStepFocusesAhead pins M2: a walk step refreshes faceAngle to point
// one tile ahead in the travel direction (rev-254 TS PathingEntity.ts:237-242,
// focus client=false). Combined with D1 this is what makes a walking entity
// render facing where it walks for newly-visible observers.
func TestApplyStepFocusesAhead(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.x, p.z, p.level = 3200, 3200, 0
	p.unfocus() // south: faceAngleZ points to z-1

	// Walk one tile east (no gamemap → takeStep applies the delta freely).
	p.queueWaypoint(p.x+1, p.z)
	dir := p.validateAndAdvanceStep()
	if dir != int(coordgrid.DirectionEast) {
		t.Fatalf("validateAndAdvanceStep dir: got %d, want East", dir)
	}

	// faceAngle must now point one tile beyond the new position, eastward.
	wantX := coordgrid.Fine(p.x+1, 1)
	wantZ := coordgrid.Fine(p.z, 1)
	if p.faceAngleX != wantX || p.faceAngleZ != wantZ {
		t.Errorf("step faceAngle: got (%d,%d), want (%d,%d) — must focus one tile ahead (M2)",
			p.faceAngleX, p.faceAngleZ, wantX, wantZ)
	}
	// faceSquare untouched by a client=false focus.
	if p.faceSquareX != -1 {
		t.Errorf("step must not set faceSquare (client=false focus): got %d", p.faceSquareX)
	}
}
