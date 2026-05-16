// Package runescript ports TS src/runescript/ at HEAD
// b8c338801fbb72d294ff9576a58925a8d3f6de47. NAI-208 seeds the package with
// ServerPointerChecker (extends cfg.PointerChecker for the IF_BUTTON
// family's interface-overlay protection logic) + the seven trigger
// constants the override consults. NAI-210 (compiler slice 6c) will
// expand this into the full ServerScriptCompiler driver.
//
// NAI-208-D-TRIGGER-PARTIAL-PORT: runescript.ServerTriggerType ports only
// the 7 button triggers SetsPointerTrigger consults; the full enum +
// RegisterAll hook lands in NAI-210. Reviewer must catch any future
// ServerPointerChecker code that references additional ServerTriggerType
// constants and either add them to the partial port or escalate.
package runescript

import (
	"strings"

	"github.com/zsrv/goscape/pkg/pack/compiler/cfg"
	"github.com/zsrv/goscape/pkg/pack/compiler/codegen"
	"github.com/zsrv/goscape/pkg/pack/compiler/diagnostics"
	"github.com/zsrv/goscape/pkg/pack/compiler/pointer"
	"github.com/zsrv/goscape/pkg/pack/compiler/semantics"
)

// Trigger IDs for the IF_BUTTON family. Values verified against TS
// src/runescript/trigger/ServerTriggerType.ts at SHA
// b8c338801fbb72d294ff9576a58925a8d3f6de47. Per
// [[plan_constants_under_different_naming]]: grep'd TS for IF_BUTTON.id /
// INV_BUTTON1.id etc. and recorded actual TS values. The override only
// uses the IDs for set-membership, not for any arithmetic — so source
// values rather than computed offsets.
const (
	IDIfButton   = 147 // TS ServerTriggerType.IF_BUTTON.id
	IDInvButton1 = 149 // TS ServerTriggerType.INV_BUTTON1.id
	IDInvButton2 = 150 // TS ServerTriggerType.INV_BUTTON2.id
	IDInvButton3 = 151 // TS ServerTriggerType.INV_BUTTON3.id
	IDInvButton4 = 152 // TS ServerTriggerType.INV_BUTTON4.id
	IDInvButton5 = 153 // TS ServerTriggerType.INV_BUTTON5.id
	IDInvButtonD = 154 // TS ServerTriggerType.INV_BUTTOND.id
)

// buttonTriggerIDs is the set of trigger IDs for which ServerPointerChecker
// applies its overlay-aware P_ACTIVE_PLAYER override. Mirrors TS
// ServerPointerChecker.setsPointerTrigger.
var buttonTriggerIDs = map[int]struct{}{
	IDIfButton:   {},
	IDInvButton1: {},
	IDInvButton2: {},
	IDInvButton3: {},
	IDInvButton4: {},
	IDInvButton5: {},
	IDInvButtonD: {},
}

// ServerPointerChecker extends cfg.PointerChecker with the
// interface-button overlay-aware protection logic. For P_ACTIVE_PLAYER on
// a button trigger, returns true only when the script's subject interface
// is NOT an overlay.
//
// Embeds *cfg.PointerChecker — callers should construct via
// NewServerPointerChecker. The override is installed via the function-
// pointer field on the base (see NAI-208-D-VIRTUAL-VIA-FNFIELD).
type ServerPointerChecker struct {
	*cfg.PointerChecker
	overlayInterfaces map[string]struct{}
}

// NewServerPointerChecker constructs the override and wires the polymorphic
// hook on the embedded PointerChecker. overlayInterfaces is the list of
// interface names that are overlays (server "overlayinterface" symbols);
// names are normalised to lowercase + underscore-collapsed whitespace.
func NewServerPointerChecker(
	d *diagnostics.Diagnostics,
	scripts []*codegen.RuneScript,
	commandPointers map[string]*pointer.PointerHolder,
	features semantics.StrictFeatureLevel,
	overlayInterfaces []string,
) *ServerPointerChecker {
	base := cfg.NewPointerChecker(d, scripts, commandPointers, features)
	overlay := make(map[string]struct{}, len(overlayInterfaces))
	for _, name := range overlayInterfaces {
		overlay[normalizeName(name)] = struct{}{}
	}
	s := &ServerPointerChecker{
		PointerChecker:    base,
		overlayInterfaces: overlay,
	}
	// Install the polymorphic hook.
	base.SetSetsPointerTriggerFn(s.setsPointerTrigger)
	return s
}

func (s *ServerPointerChecker) setsPointerTrigger(script *codegen.RuneScript, pt *pointer.PointerType) bool {
	if pt != pointer.PActivePlayer {
		return s.PointerChecker.DefaultSetsPointerTrigger(script, pt)
	}
	if _, ok := buttonTriggerIDs[script.Trigger.ID]; !ok {
		return s.PointerChecker.DefaultSetsPointerTrigger(script, pt)
	}
	subj := script.SubjectReference
	if subj == nil {
		return false
	}
	name, ok := basicSymbolName(subj)
	if !ok {
		return false
	}
	// TS splits on ':' and takes the prefix.
	prefix := strings.SplitN(name, ":", 2)[0]
	if prefix == "" {
		return false
	}
	_, isOverlay := s.overlayInterfaces[normalizeName(prefix)]
	return !isOverlay
}

// basicSymbolName extracts the user-visible name from a SymbolRef. Only
// *symbol.BasicSymbol carries the dotted "interface:button" form.
func basicSymbolName(ref any) (string, bool) {
	type named interface {
		SymbolName() string
	}
	if n, ok := ref.(named); ok {
		return n.SymbolName(), true
	}
	return "", false
}

func normalizeName(name string) string {
	return strings.Join(strings.Fields(strings.ToLower(name)), "_")
}
