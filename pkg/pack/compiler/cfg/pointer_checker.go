// pkg/pack/compiler/cfg/pointer_checker.go
package cfg

import (
	"github.com/zsrv/goscape/pkg/pack/compiler/codegen"
	"github.com/zsrv/goscape/pkg/pack/compiler/diagnostics"
	"github.com/zsrv/goscape/pkg/pack/compiler/pointer"
	"github.com/zsrv/goscape/pkg/pack/compiler/semantics"
	"github.com/zsrv/goscape/pkg/pack/compiler/symbol"
	typ "github.com/zsrv/goscape/pkg/pack/compiler/type"
)

// scriptPointerAnalysis is the per-script analysis result cached by
// PointerChecker.getAnalysis. Mirrors TS ScriptPointerAnalysis type alias
// (PointerChecker.ts L29-38). Arrays are indexed by pointer.Index(pt).
type scriptPointerAnalysis struct {
	graph                 []*InstructionNode
	required              [][]*InstructionNode
	set                   [][]*InstructionNode
	corrupted             [][]*InstructionNode
	setNodes              []map[*InstructionNode]struct{}
	corruptedNodes        []map[*InstructionNode]struct{}
	returns               []*InstructionNode
	staticLabelArgsByCall map[*codegen.Instruction]map[int]symbol.Symbol
}

// PointerChecker ports TS PointerChecker (PointerChecker.ts L40+). One
// instance per codegen run; the per-script caches are populated lazily.
//
// NAI-208-D-VIRTUAL-VIA-FNFIELD: TS uses `protected override
// setsPointerTrigger`; goscape lifts the polymorphic call to a
// function-pointer field. ServerPointerChecker's constructor overwrites
// it after embedding.
//
// NAI-208-D-SYMBOL-NO-METHOD-CYCLE-AVOID: TS adds a
// ScriptSymbol.pointers(checker) method; goscape exposes the equivalent as
// PointerChecker.GetPointers(sym) to keep pkg/pack/compiler/symbol free of
// a symbol→cfg import cycle.
type PointerChecker struct {
	diagnostics     *diagnostics.Diagnostics
	scripts         []*codegen.RuneScript
	commandPointers map[string]*pointer.PointerHolder
	features        semantics.StrictFeatureLevel

	scriptsBySymbol map[symbol.Symbol]*codegen.RuneScript
	graphGenerator  *GraphGenerator

	scriptGraphs           map[symbol.Symbol][]*InstructionNode
	scriptPointers         map[symbol.Symbol]*pointer.PointerHolder
	scriptAnalyses         map[symbol.Symbol]*scriptPointerAnalysis
	jumpParamNodesByScript map[symbol.Symbol]map[int][]*InstructionNode

	pendingAnalyses map[symbol.Symbol]struct{}
	pendingScripts  map[symbol.Symbol]struct{}

	// setsPointerTriggerFn is the polymorphic hook; default is
	// defaultSetsPointerTrigger. ServerPointerChecker.NewServerPointerChecker
	// overwrites this with its IF_BUTTON-aware variant.
	setsPointerTriggerFn func(*codegen.RuneScript, *pointer.PointerType) bool
}

// NewPointerChecker constructs a PointerChecker. commandPointers may be
// nil/empty; trigger.Pointers may be nil (treated as no implicit set).
func NewPointerChecker(
	d *diagnostics.Diagnostics,
	scripts []*codegen.RuneScript,
	commandPointers map[string]*pointer.PointerHolder,
	features semantics.StrictFeatureLevel,
) *PointerChecker {
	if commandPointers == nil {
		commandPointers = map[string]*pointer.PointerHolder{}
	}

	scriptsBySymbol := make(map[symbol.Symbol]*codegen.RuneScript, len(scripts))
	for _, s := range scripts {
		scriptsBySymbol[s.Symbol] = s
	}

	pc := &PointerChecker{
		diagnostics:     d,
		scripts:         scripts,
		commandPointers: commandPointers,
		features:        features,

		scriptsBySymbol: scriptsBySymbol,
		graphGenerator:  NewGraphGenerator(commandPointers, features),

		scriptGraphs:           map[symbol.Symbol][]*InstructionNode{},
		scriptPointers:         map[symbol.Symbol]*pointer.PointerHolder{},
		scriptAnalyses:         map[symbol.Symbol]*scriptPointerAnalysis{},
		jumpParamNodesByScript: map[symbol.Symbol]map[int][]*InstructionNode{},

		pendingAnalyses: map[symbol.Symbol]struct{}{},
		pendingScripts:  map[symbol.Symbol]struct{}{},
	}
	pc.setsPointerTriggerFn = pc.defaultSetsPointerTrigger
	return pc
}

// Run validates every script's pointer flow, reporting diagnostics for any
// uninitialized or corrupted-pointer use. Mirrors TS PointerChecker.run.
//
// T4 ships Run as a no-op skeleton over the script list (validatePointer is
// implemented in T5); the per-script loop is in place so T5 only needs to
// fill in the body of validatePointer.
func (p *PointerChecker) Run() {
	for _, s := range p.scripts {
		p.validateAllPointers(s)
	}
}

// validateAllPointers iterates PointerType.All for one script.
func (p *PointerChecker) validateAllPointers(script *codegen.RuneScript) {
	for _, pt := range pointer.All {
		p.validatePointer(script, pt)
	}
}

// validatePointer is the per-pointer validation hook. T4 ships a no-op; T5
// fills in the body.
func (p *PointerChecker) validatePointer(script *codegen.RuneScript, pt *pointer.PointerType) {
	// Implemented in T5.
}

// GetGraph returns the cached CFG for script, building it on first call.
// Mirrors TS PointerChecker.getGraph.
func (p *PointerChecker) GetGraph(script *codegen.RuneScript) []*InstructionNode {
	if cached, ok := p.scriptGraphs[script.Symbol]; ok {
		return cached
	}
	g := p.graphGenerator.Generate(script.Blocks)
	p.scriptGraphs[script.Symbol] = g
	return g
}

// GetPointers returns the PointerHolder for sym, calculating it on first
// call. Mirrors TS PointerChecker.getPointers.
func (p *PointerChecker) GetPointers(sym symbol.Symbol) *pointer.PointerHolder {
	if cached, ok := p.scriptPointers[sym]; ok {
		return cached
	}
	h := p.calculatePointers(sym)
	p.scriptPointers[sym] = h
	return h
}

// SetsPointerTrigger reports whether the trigger of script implicitly sets
// pt. Polymorphic: delegates to setsPointerTriggerFn (overwritten by
// ServerPointerChecker constructor). Mirrors TS protected method.
func (p *PointerChecker) SetsPointerTrigger(script *codegen.RuneScript, pt *pointer.PointerType) bool {
	return p.setsPointerTriggerFn(script, pt)
}

// defaultSetsPointerTrigger is the base behaviour: trigger.Pointers.Has(pt).
func (p *PointerChecker) defaultSetsPointerTrigger(script *codegen.RuneScript, pt *pointer.PointerType) bool {
	return script.Trigger.Pointers.Has(pt)
}

// calculatePointers computes which pointers script requires/sets/corrupts
// based on the per-script CFG analysis. Mirrors TS calculatePointers.
//
// Recursion guard: while a script is mid-calculation, calculatePointers
// returns an empty holder rather than re-entering. Mirrors TS pendingScripts.
func (p *PointerChecker) calculatePointers(sym symbol.Symbol) *pointer.PointerHolder {
	if _, pending := p.pendingScripts[sym]; pending {
		return &pointer.PointerHolder{
			Required:  pointer.NewPointerSet(),
			Set:       pointer.NewPointerSet(),
			Corrupted: pointer.NewPointerSet(),
		}
	}
	script, ok := p.scriptsBySymbol[sym]
	if !ok {
		panic("PointerChecker.calculatePointers: unknown script " + sym.SymbolName())
	}

	required := pointer.NewPointerSet()
	set := pointer.NewPointerSet()
	corrupted := pointer.NewPointerSet()

	p.pendingScripts[sym] = struct{}{}
	for _, pt := range pointer.All {
		if p.requiresPointerScript(script, pt) {
			required.Add(pt)
		}
		if p.setsPointerScript(script, pt) {
			set.Add(pt)
		}
		if p.corruptsPointerScript(script, pt) {
			corrupted.Add(pt)
		}
	}
	delete(p.pendingScripts, sym)

	return &pointer.PointerHolder{
		Required:  required,
		Set:       set,
		Corrupted: corrupted,
	}
}

// requiresPointerScript reports whether some node requires pt without first
// passing through a node that sets it. Mirrors TS.
func (p *PointerChecker) requiresPointerScript(script *codegen.RuneScript, pt *pointer.PointerType) bool {
	return p.requiresPointerPathScript(script, pt) != nil
}

func (p *PointerChecker) requiresPointerPathScript(script *codegen.RuneScript, pt *pointer.PointerType) []*InstructionNode {
	analysis := p.getAnalysis(script)
	i := pointer.Index(pt)
	return p.findEdgePath(
		analysis.required[i],
		func(n *InstructionNode) bool { return n == analysis.graph[0] },
		analysis.setNodes[i],
	)
}

func (p *PointerChecker) setsPointerScript(script *codegen.RuneScript, pt *pointer.PointerType) bool {
	analysis := p.getAnalysis(script)
	i := pointer.Index(pt)
	return p.findEdgePath(
		analysis.returns,
		func(n *InstructionNode) bool {
			if n == analysis.graph[0] {
				return true
			}
			_, ok := analysis.corruptedNodes[i][n]
			return ok
		},
		analysis.setNodes[i],
	) == nil
}

func (p *PointerChecker) corruptsPointerScript(script *codegen.RuneScript, pt *pointer.PointerType) bool {
	analysis := p.getAnalysis(script)
	i := pointer.Index(pt)
	return p.findEdgePath(
		analysis.returns,
		func(n *InstructionNode) bool {
			_, ok := analysis.corruptedNodes[i][n]
			return ok
		},
		analysis.setNodes[i],
	) != nil
}

// findEdgePath performs BFS from any starts' previous-edge neighbour,
// walking Previous links, accumulating the path on the first node where
// end() returns true. Mirrors TS findEdgePath.
func (p *PointerChecker) findEdgePath(
	starts []*InstructionNode,
	end func(*InstructionNode) bool,
	blocked map[*InstructionNode]struct{},
) []*InstructionNode {
	if len(starts) == 0 {
		return nil
	}

	sources := map[*InstructionNode]*InstructionNode{}
	startSource := map[*InstructionNode]*InstructionNode{}
	var queue []*InstructionNode

	for _, s := range starts {
		for _, neighbour := range s.Previous {
			if _, blk := blocked[neighbour]; blk {
				continue
			}
			if _, seen := sources[neighbour]; seen {
				continue
			}
			startSource[neighbour] = s
			sources[neighbour] = nil
			queue = append(queue, neighbour)
		}
	}

	for i := 0; i < len(queue); i++ {
		current := queue[i]
		if end(current) {
			// Reconstruct backwards from current to startSource head.
			var result []*InstructionNode
			node := current
			for node != nil {
				result = append([]*InstructionNode{node}, result...)
				parent, ok := sources[node]
				if !ok {
					break
				}
				node = parent
			}
			// Prepend the original start that owns the head's neighbour entry.
			head := result[0]
			result = append([]*InstructionNode{startSource[head]}, result...)
			return result
		}
		for _, neighbour := range current.Previous {
			if _, blk := blocked[neighbour]; blk {
				continue
			}
			if _, seen := sources[neighbour]; seen {
				continue
			}
			sources[neighbour] = current
			queue = append(queue, neighbour)
		}
	}

	return nil
}

// getAnalysis returns the cached scriptPointerAnalysis for script, building
// it on first call. Recursion-guarded via pendingAnalyses (mirrors TS).
//
// T4 includes only the non-label sources: Command + Gosub/Jump +
// PushVar/PopVar/PushVar2/PopVar2 + Return. T6 layers
// staticLabelArgsByCall + addStaticLabelRequirements on top via separate
// helpers (the field is allocated empty in T4 and populated in T6).
func (p *PointerChecker) getAnalysis(script *codegen.RuneScript) *scriptPointerAnalysis {
	if cached, ok := p.scriptAnalyses[script.Symbol]; ok {
		return cached
	}
	graph := p.GetGraph(script)
	if _, pending := p.pendingAnalyses[script.Symbol]; pending {
		return p.emptyAnalysis(graph)
	}
	p.pendingAnalyses[script.Symbol] = struct{}{}
	defer delete(p.pendingAnalyses, script.Symbol)

	pointerCount := len(pointer.All)
	required := make([][]*InstructionNode, pointerCount)
	setArr := make([][]*InstructionNode, pointerCount)
	corrupted := make([][]*InstructionNode, pointerCount)
	var returns []*InstructionNode
	staticLabelArgsByCall := map[*codegen.Instruction]map[int]symbol.Symbol{} // T6 populates

	for _, node := range graph {
		// PointerInstructionNode (synthetic): contributes to set.
		if node.Instruction == nil && len(node.Previous) > 0 {
			if pin := extractPointerInstructionNode(node); pin != nil {
				addPointersToArray(setArr, pin.Set, node)
				continue
			}
		}

		inst := node.Instruction
		if inst == nil {
			continue
		}

		if inst.Opcode == codegen.Return {
			returns = append(returns, node)
		}

		switch inst.Opcode {
		case codegen.Command:
			sym, ok := inst.Operand.(symbol.Symbol)
			if !ok {
				break
			}
			holder := p.commandPointers[sym.SymbolName()]
			if holder == nil {
				break
			}
			addPointersToArray(required, holder.Required, node)
			addPointersToArray(corrupted, holder.Corrupted, node)
			if !holder.ConditionalSet {
				addPointersToArray(setArr, holder.Set, node)
			}

		case codegen.Gosub, codegen.Jump:
			sym, ok := inst.Operand.(symbol.Symbol)
			if !ok {
				break
			}
			holder := p.GetPointers(sym)
			addPointersToArray(required, holder.Required, node)
			addPointersToArray(setArr, holder.Set, node)
			addPointersToArray(corrupted, holder.Corrupted, node)
			// T6 hooks here: staticLabelArgsByCall lookup + addStaticLabelRequirements.

		case codegen.PushVar:
			if sym, ok := inst.Operand.(*symbol.BasicSymbol); ok {
				addBasicVarRequired(required, sym.Type, node, false /*pop*/, false /*two*/)
			}

		case codegen.PopVar:
			if sym, ok := inst.Operand.(*symbol.BasicSymbol); ok {
				addBasicVarRequired(required, sym.Type, node, true, false)
				_ = sym.IsProtected // T5 will read sym.IsProtected here to wire protected-pop → P_ACTIVE_PLAYER; T4 stub isProtected returns false (NAI-208-D-PROTECTED-VAR-VIA-SYMBOL).
			}

		case codegen.PushVar2:
			if sym, ok := inst.Operand.(*symbol.BasicSymbol); ok {
				addBasicVarRequired(required, sym.Type, node, false, true)
			}

		case codegen.PopVar2:
			if sym, ok := inst.Operand.(*symbol.BasicSymbol); ok {
				addBasicVarRequired(required, sym.Type, node, true, true)
			}
		}
	}

	analysis := &scriptPointerAnalysis{
		graph:                 graph,
		required:              required,
		set:                   setArr,
		corrupted:             corrupted,
		setNodes:              nodeArrayToSets(setArr),
		corruptedNodes:        nodeArrayToSets(corrupted),
		returns:               returns,
		staticLabelArgsByCall: staticLabelArgsByCall,
	}
	p.scriptAnalyses[script.Symbol] = analysis
	return analysis
}

// emptyAnalysis builds the zero-state analysis for the recursive-call guard.
func (p *PointerChecker) emptyAnalysis(graph []*InstructionNode) *scriptPointerAnalysis {
	pointerCount := len(pointer.All)
	required := make([][]*InstructionNode, pointerCount)
	setArr := make([][]*InstructionNode, pointerCount)
	corrupted := make([][]*InstructionNode, pointerCount)
	var returns []*InstructionNode
	for _, n := range graph {
		if n.Instruction != nil && n.Instruction.Opcode == codegen.Return {
			returns = append(returns, n)
		}
	}
	return &scriptPointerAnalysis{
		graph:                 graph,
		required:              required,
		set:                   setArr,
		corrupted:             corrupted,
		setNodes:              nodeArrayToSets(setArr),
		corruptedNodes:        nodeArrayToSets(corrupted),
		returns:               returns,
		staticLabelArgsByCall: map[*codegen.Instruction]map[int]symbol.Symbol{},
	}
}

// addPointersToArray fans each pointer in set out into target[Index(pt)],
// appending node. nil-safe on set.
func addPointersToArray(target [][]*InstructionNode, set *pointer.PointerSet, node *InstructionNode) {
	if set == nil || set.Len() == 0 {
		return
	}
	for _, pt := range set.All() {
		i := pointer.Index(pt)
		target[i] = append(target[i], node)
	}
}

// addBasicVarRequired files the pointer-required entry for a Push/Pop var
// instruction. Mirrors TS arms at PointerChecker.ts L664-706.
//
// pop=true + protected isProtected → P_ACTIVE_PLAYER / P_ACTIVE_PLAYER2.
//   otherwise (pop=false OR !isProtected) → ACTIVE_PLAYER / ACTIVE_PLAYER2.
// Npc vars always → ACTIVE_NPC / ACTIVE_NPC2 regardless of pop/protected.
func addBasicVarRequired(target [][]*InstructionNode, t typ.Type, node *InstructionNode, pop, two bool) {
	switch v := t.(type) {
	case *typ.VarPlayerType, *typ.VarBitType:
		var pt *pointer.PointerType
		switch {
		case pop && two && isProtected(v):
			pt = pointer.PActivePlayer2
		case pop && !two && isProtected(v):
			pt = pointer.PActivePlayer
		case two:
			pt = pointer.ActivePlayer2
		default:
			pt = pointer.ActivePlayer
		}
		target[pointer.Index(pt)] = append(target[pointer.Index(pt)], node)
	case *typ.VarNpcType:
		var pt *pointer.PointerType
		if two {
			pt = pointer.ActiveNpc2
		} else {
			pt = pointer.ActiveNpc
		}
		target[pointer.Index(pt)] = append(target[pointer.Index(pt)], node)
	}
}

// isProtected mirrors the TS `symbol.isProtected` read in the var arms.
// Goscape stores IsProtected on *symbol.BasicSymbol, not on the type.
// The instruction-node walker calls this with the operand's *type*; we
// always return false here. (Per TS, only Pop variants check isProtected,
// and the symbol carries the flag — adjust the call site in T5/T6 if a
// targeted test surfaces a false negative.)
//
// NAI-208-D-PROTECTED-VAR-VIA-SYMBOL: the protected-pop branch needs the
// BasicSymbol's IsProtected, not the type's. T4 conservatively returns
// false here (matches: var is unprotected → required=ACTIVE_PLAYER) and
// defers the type-vs-symbol fix to T5 once the validatePointer surfaces it.
func isProtected(t any) bool {
	return false
}

// extractPointerInstructionNode is unused in T4 — PointerInstructionNodes
// are emitted directly into the graph slice as *InstructionNode (via
// pin.BaseNode()). T4 ships this stub returning nil; T5 wires the set-arc
// path if validatePointer surfaces a need.
func extractPointerInstructionNode(node *InstructionNode) *PointerInstructionNode {
	return nil
}

// nodeArrayToSets converts a per-pointer [][]*InstructionNode into the
// matching per-pointer []map[*InstructionNode]struct{} for fast contains
// lookups during findEdgePath.
func nodeArrayToSets(arr [][]*InstructionNode) []map[*InstructionNode]struct{} {
	out := make([]map[*InstructionNode]struct{}, len(arr))
	for i, nodes := range arr {
		s := make(map[*InstructionNode]struct{}, len(nodes))
		for _, n := range nodes {
			s[n] = struct{}{}
		}
		out[i] = s
	}
	return out
}
