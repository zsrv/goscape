package world

import (
	"testing"

	"github.com/zsrv/goscape/pkg/rsbuf"
)

// TestPlayAnim_AnimProtectBlocksWrite — NAI-56. With animProtect set,
// PlayAnim must early-return and leave animID, animDelay, and the
// MaskAnim bit untouched. Mirrors TS Player.ts:1842 where the animProtect
// truthy check short-circuits playAnimation before any write.
func TestPlayAnim_AnimProtectBlocksWrite(t *testing.T) {
	p, _ := newTestPlayer(t)
	initialAnimID := p.animID
	initialAnimDelay := p.animDelay
	initialMasks := p.masks

	p.animProtect = 1
	p.PlayAnim(123, 5)

	if p.animID != initialAnimID {
		t.Errorf("animID: got %d, want %d (unchanged)", p.animID, initialAnimID)
	}
	if p.animDelay != initialAnimDelay {
		t.Errorf("animDelay: got %d, want %d (unchanged)", p.animDelay, initialAnimDelay)
	}
	if p.masks != initialMasks {
		t.Errorf("masks: got %d, want %d (unchanged)", p.masks, initialMasks)
	}
	if p.masks&rsbuf.MaskAnim != 0 {
		t.Error("MaskAnim bit should not be set when animProtect blocks the write")
	}
}

// TestPlayAnim_AnimProtectZeroAllowsWrite — NAI-56. Baseline regression
// guard: with animProtect=0 (default), PlayAnim writes through and sets
// MaskAnim. Pins that the new gate has no effect on the unprotected path.
func TestPlayAnim_AnimProtectZeroAllowsWrite(t *testing.T) {
	p, _ := newTestPlayer(t)
	if p.animProtect != 0 {
		t.Fatalf("animProtect: precondition got %d, want 0", p.animProtect)
	}

	p.PlayAnim(123, 5)

	if p.animID != 123 {
		t.Errorf("animID: got %d, want 123", p.animID)
	}
	if p.animDelay != 5 {
		t.Errorf("animDelay: got %d, want 5", p.animDelay)
	}
	if p.masks&rsbuf.MaskAnim == 0 {
		t.Error("MaskAnim bit should be set when animProtect=0")
	}
}
