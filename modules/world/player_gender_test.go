package world

import "testing"

// TestPlayerSetGender_MaleToFemale_RewritesAllSlotsViaMap pins TS
// PlayerOps.ts:1104-1118 male→female direction. Body slots are
// rewritten via maleFemaleMap; gender field set to 1.
func TestPlayerSetGender_MaleToFemale_RewritesAllSlotsViaMap(t *testing.T) {
	p := &Player{}
	p.body = [7]int{0, 1, 2, 3, 4, 5, 6}
	p.gender = 0

	p.SetGender(1)

	want := [7]int{45, 47, 48, 49, 50, 51, 52}
	if p.body != want {
		t.Errorf("body: got %v, want %v", p.body, want)
	}
	if p.gender != 1 {
		t.Errorf("gender: got %d, want 1", p.gender)
	}
}

// TestPlayerSetGender_FemaleToMale_Slot1HardcodedTo14 pins the
// TS slot-1 special case (PlayerOps.ts:1111-1113):
//
//	if (i === 1) { state.activePlayer.body[i] = 14; continue; }
//
// On female→male direction, body[1] is forced to 14 even when
// femaleMaleMap[body[1]] would yield a different value. Deliberate
// TS canon for the canonical male hair model — not a bug.
func TestPlayerSetGender_FemaleToMale_Slot1HardcodedTo14(t *testing.T) {
	p := &Player{}
	p.body = [7]int{45, 47, 48, 49, 50, 51, 52}
	p.gender = 1

	p.SetGender(0)

	// femaleMaleMap[47] == 1, but slot 1 is hardcoded to 14.
	want := [7]int{0, 14, 2, 3, 4, 5, 6}
	if p.body != want {
		t.Errorf("body: got %v, want %v (slot 1 must be 14 hardcode)", p.body, want)
	}
	if p.gender != 0 {
		t.Errorf("gender: got %d, want 0", p.gender)
	}
}

// TestPlayerSetGender_FemaleToMale_NonSlot1UsesMap pins that slots
// other than 1 go through femaleMaleMap on the female→male direction.
// Spot-checks a few keys including a lossy-collapse case.
func TestPlayerSetGender_FemaleToMale_NonSlot1UsesMap(t *testing.T) {
	p := &Player{}
	// Use female values that exercise both 1:1 keys and the {73, 74, 77}→36
	// lossy-collapse case in slot 0.
	p.body = [7]int{73, 47, 56, 65, 76, 77, 81}
	p.gender = 1

	p.SetGender(0)

	// slot 0: 73 → 36 (lossy, both 73 and 77 collapse to 36)
	// slot 1: hardcoded to 14
	// slot 2: 56 → 18
	// slot 3: 65 → 29
	// slot 4: 76 → 39
	// slot 5: 77 → 36 (lossy)
	// slot 6: 81 → 44
	want := [7]int{36, 14, 18, 29, 39, 36, 44}
	if p.body != want {
		t.Errorf("body: got %v, want %v", p.body, want)
	}
}

// TestPlayerSetGender_UnmappedKeysWriteMinusOne pins
// NAI-SETGENDER-D-TS-UNMAPPED-IDKIT-INHERITED: TS-literal
// `Map.get(k) ?? -1` writes -1 to the slot when the current
// body[i] is not present in the relevant lookup map.
//
// Real content cannot reach this case (the makeover_mage UI flow
// constrains body[] to mapped values), but the behavior is TS-literal
// and pinned for future TS sync.
func TestPlayerSetGender_UnmappedKeysWriteMinusOne(t *testing.T) {
	t.Run("male->female direction", func(t *testing.T) {
		p := &Player{}
		p.body = [7]int{999, 999, 999, 999, 999, 999, 999}
		p.gender = 0

		p.SetGender(1)

		want := [7]int{-1, -1, -1, -1, -1, -1, -1}
		if p.body != want {
			t.Errorf("body: got %v, want %v (unmapped keys must write -1)", p.body, want)
		}
	})
	t.Run("female->male direction (slot 1 still hardcoded)", func(t *testing.T) {
		p := &Player{}
		p.body = [7]int{999, 999, 999, 999, 999, 999, 999}
		p.gender = 1

		p.SetGender(0)

		// Slot 1 is hardcoded to 14 regardless of map lookup. Other slots → -1.
		want := [7]int{-1, 14, -1, -1, -1, -1, -1}
		if p.body != want {
			t.Errorf("body: got %v, want %v", p.body, want)
		}
	})
}

// TestPlayerSetGender_DoesNotFlipMaskAppearance pins the TS-faithful
// deferred-rebuild assertion. SETGENDER must NOT flip MaskAppearance —
// callers must invoke BUILDAPPEARANCE explicitly (per
// makeover_mage.rs2:58-64 content evidence).
func TestPlayerSetGender_DoesNotFlipMaskAppearance(t *testing.T) {
	t.Run("male->female", func(t *testing.T) {
		p := &Player{}
		p.body = [7]int{0, 1, 2, 3, 4, 5, 6}
		p.masks = 0

		p.SetGender(1)

		if p.masks != 0 {
			t.Errorf("masks: got %d, want 0 (SETGENDER must not flip MaskAppearance)", p.masks)
		}
	})
	t.Run("female->male", func(t *testing.T) {
		p := &Player{}
		p.body = [7]int{45, 47, 48, 49, 50, 51, 52}
		p.masks = 0

		p.SetGender(0)

		if p.masks != 0 {
			t.Errorf("masks: got %d, want 0", p.masks)
		}
	})
}

// TestPlayerSetGender_LossyCollapse documents that the TS map pair is
// intentionally NOT a full bijection. body[0]=19 (a male in the {18..25}
// cohort that all collapse to female 56) round-trips to canonical 18,
// not 19. Mirrors OSRS canon — the makeover-mage isn't fully reversible.
func TestPlayerSetGender_LossyCollapse(t *testing.T) {
	p := &Player{}
	p.body = [7]int{19, 0, 0, 0, 0, 0, 0}
	p.gender = 0

	p.SetGender(1) // body[0]: 19 → 56
	if p.body[0] != 56 {
		t.Fatalf("after M→F: body[0]=%d, want 56", p.body[0])
	}

	p.SetGender(0) // body[0]: 56 → 18 (canonical, NOT 19)
	if p.body[0] != 18 {
		t.Errorf("after F→M: body[0]=%d, want 18 (canonical lossy collapse, NOT 19)", p.body[0])
	}
}
