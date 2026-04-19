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
