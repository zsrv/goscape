// pkg/pack/compiler/runescript/server_pointer_checker_test.go
package runescript

import (
	"testing"

	"github.com/zsrv/goscape/pkg/pack/compiler/cfg"
	"github.com/zsrv/goscape/pkg/pack/compiler/codegen"
	"github.com/zsrv/goscape/pkg/pack/compiler/diagnostics"
	"github.com/zsrv/goscape/pkg/pack/compiler/pointer"
	"github.com/zsrv/goscape/pkg/pack/compiler/semantics"
	"github.com/zsrv/goscape/pkg/pack/compiler/symbol"
	"github.com/zsrv/goscape/pkg/pack/compiler/trigger"
	typ "github.com/zsrv/goscape/pkg/pack/compiler/type"
)

func newIfButtonScript(subjectInterface string, modIdent string) *codegen.RuneScript {
	// IF_BUTTON id pinned by the partial-port (T7's stub); use the same id
	// constant from server_pointer_checker.go.
	tr := &trigger.TriggerType{ID: IDIfButton, Identifier: modIdent}
	sym := &symbol.ServerScriptSymbol{Trigger: tr, Name: "b"}
	rs := codegen.NewRuneScript("test.rs2", sym, tr, "b", &symbol.BasicSymbol{Name: subjectInterface + ":btn", Type: typ.PrimitiveInt})
	b := codegen.NewBlock(&codegen.Label{Name: "entry"})
	b.Add(codegen.Instruction{Opcode: codegen.Return})
	rs.Blocks = []*codegen.Block{b}
	return rs
}

func TestServerPointerChecker_PActivePlayer_NonOverlay_ButtonTriggerSets(t *testing.T) {
	rs := newIfButtonScript("inv", "if_button")
	d := &diagnostics.Diagnostics{}
	spc := NewServerPointerChecker(d, []*codegen.RuneScript{rs}, map[string]*pointer.PointerHolder{}, semantics.StrictFeatureLevel{}, nil)
	if !spc.PointerChecker.SetsPointerTrigger(rs, pointer.PActivePlayer) {
		t.Error("non-overlay button trigger should set P_ACTIVE_PLAYER")
	}
}

func TestServerPointerChecker_PActivePlayer_Overlay_ButtonTriggerDoesNotSet(t *testing.T) {
	rs := newIfButtonScript("overlay_x", "if_button")
	d := &diagnostics.Diagnostics{}
	spc := NewServerPointerChecker(d, []*codegen.RuneScript{rs}, map[string]*pointer.PointerHolder{}, semantics.StrictFeatureLevel{}, []string{"Overlay X"})
	if spc.PointerChecker.SetsPointerTrigger(rs, pointer.PActivePlayer) {
		t.Error("overlay button trigger should NOT set P_ACTIVE_PLAYER (matched lowercase)")
	}
}

func TestServerPointerChecker_DisableOverlayInterfaceProtection_AlwaysSetsOnButtons(t *testing.T) {
	// With the flag set, P_ACTIVE_PLAYER is always set by any button
	// trigger regardless of subject-interface overlay status — matching
	// RuneScriptTS v0.9.4 bundled in Engine-TS, whose button triggers
	// listed P_ACTIVE_PLAYER directly in their `pointers` set. The
	// goscape trigger table (server_trigger_type.go) follows HEAD and
	// omits it; the gate restores v0.9.4-equivalent semantics.
	feats := semantics.StrictFeatureLevel{DisableOverlayInterfaceProtection: true}
	overlayed := newIfButtonScript("overlay_x", "if_button")
	d := &diagnostics.Diagnostics{}
	spc := NewServerPointerChecker(d, []*codegen.RuneScript{overlayed}, map[string]*pointer.PointerHolder{}, feats, []string{"Overlay X"})
	if !spc.PointerChecker.SetsPointerTrigger(overlayed, pointer.PActivePlayer) {
		t.Error("with DisableOverlayInterfaceProtection, P_ACTIVE_PLAYER must be set on overlay subjects too")
	}
	plain := newIfButtonScript("inv", "if_button")
	d2 := &diagnostics.Diagnostics{}
	spc2 := NewServerPointerChecker(d2, []*codegen.RuneScript{plain}, map[string]*pointer.PointerHolder{}, feats, nil)
	if !spc2.PointerChecker.SetsPointerTrigger(plain, pointer.PActivePlayer) {
		t.Error("with DisableOverlayInterfaceProtection, non-overlay buttons must still set P_ACTIVE_PLAYER")
	}
}

func TestServerPointerChecker_OtherPointers_DelegateToBase(t *testing.T) {
	rs := newIfButtonScript("inv", "if_button")
	rs.Trigger.Pointers = pointer.NewPointerSet(pointer.ActiveNpc)
	d := &diagnostics.Diagnostics{}
	spc := NewServerPointerChecker(d, []*codegen.RuneScript{rs}, map[string]*pointer.PointerHolder{}, semantics.StrictFeatureLevel{}, nil)

	// ACTIVE_NPC must delegate to base, which reads trigger.Pointers.
	if !spc.PointerChecker.SetsPointerTrigger(rs, pointer.ActiveNpc) {
		t.Error("non-P_ACTIVE_PLAYER pointer should delegate to base behaviour")
	}
}

// reference cfg to avoid "imported and not used"
var _ = cfg.PointerChecker{}
