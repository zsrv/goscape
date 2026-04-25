package world

import (
	"testing"

	"github.com/zsrv/goscape/pkg/inventory"
	"github.com/zsrv/goscape/pkg/objtype"
)

func synthesizeTypes(t *testing.T) (*objtype.ObjTypeConfigs, *objtype.InvTypeConfigs) {
	t.Helper()
	objs := &objtype.ObjTypeConfigs{
		Configs:     make([]*objtype.ObjType, 10),
		ConfigNames: map[string]int{},
	}
	// id=1: platebody (wearPos=4, wearPos2=6, wearPos3=-1) - hides arms
	objs.Configs[1] = &objtype.ObjType{
		ConfigType: objtype.ConfigType{ID: 1, DebugName: "platebody"},
		WearPos:    4, WearPos2: 6, WearPos3: -1,
	}
	// id=2: full helm (wearPos=8, wearPos2=0, wearPos3=1)
	objs.Configs[2] = &objtype.ObjType{
		ConfigType: objtype.ConfigType{ID: 2, DebugName: "full_helm"},
		WearPos:    8, WearPos2: 0, WearPos3: 1,
	}

	invs := &objtype.InvTypeConfigs{
		Configs:     make([]*objtype.InvType, 2),
		ConfigNames: map[string]int{"worn": 0},
		Worn:        0,
	}
	invs.Configs[0] = &objtype.InvType{
		ConfigType: objtype.ConfigType{ID: 0, DebugName: "worn"},
		Size:       14,
	}
	return objs, invs
}

func TestGenerateAppearanceNakedPlayer(t *testing.T) {
	objs, invs := synthesizeTypes(t)
	p, _ := newTestPlayer(t)
	p.invs = map[int]*inventory.Inventory{
		invs.Worn: inventory.FromType(invs.Configs[invs.Worn]),
	}

	p.generateAppearance(objs, invs, 0)

	if len(p.appearanceBuf) == 0 {
		t.Fatal("appearanceBuf should be non-empty")
	}
	if p.lastAppearance != 0 {
		t.Errorf("lastAppearance: got %d, want 0", p.lastAppearance)
	}
	if p.appearanceBuf[0] != 0 {
		t.Errorf("gender byte: got %d, want 0", p.appearanceBuf[0])
	}
	if p.appearanceBuf[1] != 0 {
		t.Errorf("headicons byte: got %d, want 0", p.appearanceBuf[1])
	}
}

func TestGenerateAppearancePlatebodyEquipped(t *testing.T) {
	objs, invs := synthesizeTypes(t)
	p, _ := newTestPlayer(t)
	p.invs = map[int]*inventory.Inventory{
		invs.Worn: inventory.FromType(invs.Configs[invs.Worn]),
	}
	// Equip platebody at slot 4
	p.invs[invs.Worn].Items[4] = &inventory.Item{Id: 1, Count: 1}

	p.generateAppearance(objs, invs, 0)

	if len(p.appearanceBuf) == 0 {
		t.Fatal("appearanceBuf should be non-empty")
	}
}

// TestGenerateAppearanceSentinelDefaultReadsWorn pins NAI-21 Task (d) /
// NAI-21-D1: when p.appearanceInv == -1 (the default sentinel from
// newPlayer), the reader must fall back to invs.Worn — preserving
// pre-fix behavior for production callers that haven't yet invoked
// SetAppearanceInv (initial login, fresh players).
func TestGenerateAppearanceSentinelDefaultReadsWorn(t *testing.T) {
	objs, invs := synthesizeTypes(t)
	p, _ := newTestPlayer(t)
	if p.appearanceInv != -1 {
		t.Fatalf("setup: p.appearanceInv should default to -1, got %d", p.appearanceInv)
	}
	p.invs = map[int]*inventory.Inventory{
		invs.Worn: inventory.FromType(invs.Configs[invs.Worn]),
	}
	// Equip a platebody at slot 4 (the platebody synthesized in synthesizeTypes).
	p.invs[invs.Worn].Items[4] = &inventory.Item{Id: 1, Count: 1}

	p.generateAppearance(objs, invs, 0)

	// The platebody (id=1) at slot 4 must surface in the appearance buffer.
	// Equipped items use 2-byte form: 0x0200 | (id & 0x1FF). Search the buffer
	// for the encoded byte pair as a non-positional presence check.
	if len(p.appearanceBuf) == 0 {
		t.Fatal("appearanceBuf should be non-empty (sentinel must fall back to Worn)")
	}
	wantSlot4Hi := byte((0x200 | (1 & 0x1FF)) >> 8)
	wantSlot4Lo := byte((0x200 | (1 & 0x1FF)) & 0xFF)
	found := false
	for i := 0; i < len(p.appearanceBuf)-1; i++ {
		if p.appearanceBuf[i] == wantSlot4Hi && p.appearanceBuf[i+1] == wantSlot4Lo {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("appearanceBuf missing platebody encoded bytes (0x%02x 0x%02x); "+
			"sentinel mapping to Worn appears broken", wantSlot4Hi, wantSlot4Lo)
	}
}

// TestGenerateAppearanceExplicitWornIdMatchesSentinel pins NAI-21 Task (d):
// explicit p.appearanceInv = invs.Worn produces byte-identical output
// to the sentinel-default path. Proves the new explicit-set codepath
// matches the sentinel-mapped codepath for the common case.
func TestGenerateAppearanceExplicitWornIdMatchesSentinel(t *testing.T) {
	objs, invs := synthesizeTypes(t)

	// Player A: sentinel default (-1).
	pA, _ := newTestPlayer(t)
	pA.invs = map[int]*inventory.Inventory{
		invs.Worn: inventory.FromType(invs.Configs[invs.Worn]),
	}
	pA.invs[invs.Worn].Items[4] = &inventory.Item{Id: 1, Count: 1}
	pA.generateAppearance(objs, invs, 0)

	// Player B: explicit appearanceInv = invs.Worn.
	pB, _ := newTestPlayer(t)
	pB.appearanceInv = invs.Worn
	pB.invs = map[int]*inventory.Inventory{
		invs.Worn: inventory.FromType(invs.Configs[invs.Worn]),
	}
	pB.invs[invs.Worn].Items[4] = &inventory.Item{Id: 1, Count: 1}
	pB.generateAppearance(objs, invs, 0)

	if len(pA.appearanceBuf) != len(pB.appearanceBuf) {
		t.Fatalf("buffer length mismatch: sentinel=%d explicit=%d",
			len(pA.appearanceBuf), len(pB.appearanceBuf))
	}
	for i := range pA.appearanceBuf {
		if pA.appearanceBuf[i] != pB.appearanceBuf[i] {
			t.Errorf("byte %d differs: sentinel=0x%02x explicit=0x%02x",
				i, pA.appearanceBuf[i], pB.appearanceBuf[i])
		}
	}
}

// TestGenerateAppearanceCustomInvIdHonored pins NAI-21 Task (d) S7c-D1
// closure: when p.appearanceInv is set to a non-Worn inv id, the reader
// must read FROM that inv (not from invs.Worn). This is the actual S7c-D1
// bug-fix proof — the pre-fix reader at appearance.go:27 read p.invs[invs.Worn]
// regardless of p.appearanceInv, so custom-outfit scripts had no effect.
func TestGenerateAppearanceCustomInvIdHonored(t *testing.T) {
	objs, invs := synthesizeTypes(t)

	// Extend the synthesized invs to add a "custom" inv at id=1 (Worn is id=0).
	// synthesizeTypes pre-allocates Configs with len=2, slots [0]=Worn and [1]=nil.
	// Populate slot [1] in place (don't append, which would push to index 2).
	customInvId := 1
	invs.Configs[customInvId] = &objtype.InvType{
		ConfigType: objtype.ConfigType{ID: customInvId, DebugName: "custom"},
		Size:       14,
	}

	p, _ := newTestPlayer(t)
	p.invs = map[int]*inventory.Inventory{
		invs.Worn:   inventory.FromType(invs.Configs[invs.Worn]), // empty
		customInvId: inventory.FromType(invs.Configs[customInvId]),
	}
	// Worn is empty; custom has a platebody at slot 4.
	p.invs[customInvId].Items[4] = &inventory.Item{Id: 1, Count: 1}
	p.appearanceInv = customInvId

	p.generateAppearance(objs, invs, 0)

	// The platebody must surface in the appearance buffer because the
	// reader read from p.invs[customInvId], NOT from p.invs[invs.Worn].
	wantSlot4Hi := byte((0x200 | (1 & 0x1FF)) >> 8)
	wantSlot4Lo := byte((0x200 | (1 & 0x1FF)) & 0xFF)
	found := false
	for i := 0; i < len(p.appearanceBuf)-1; i++ {
		if p.appearanceBuf[i] == wantSlot4Hi && p.appearanceBuf[i+1] == wantSlot4Lo {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("appearanceBuf missing platebody from custom inv; reader is "+
			"still reading from invs.Worn (S7c-D1 NOT closed)")
	}
}
