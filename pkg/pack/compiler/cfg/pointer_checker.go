// pkg/pack/compiler/cfg/pointer_checker.go
package cfg

import (
	"sort"

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

// validatePointer verifies that pt is available everywhere it is required.
// If a path is found from a node that requires pt to a node that lacks it
// (the start, or a node that corrupts it), reports a diagnostic. Mirrors
// TS PointerChecker.validatePointer.
func (p *PointerChecker) validatePointer(script *codegen.RuneScript, pt *pointer.PointerType) {
	analysis := p.getAnalysis(script)
	pointerIndex := pointer.Index(pt)

	graph := analysis.graph
	required := analysis.required[pointerIndex]
	setNodes := analysis.setNodes[pointerIndex]
	corruptedSet := analysis.corruptedNodes[pointerIndex]

	if !p.SetsPointerTrigger(script, pt) {
		// Trigger does not implicitly set pt → mark start node corrupted.
		if len(graph) > 0 {
			if _, ok := corruptedSet[graph[0]]; !ok {
				corruptedSet = cloneNodeSet(corruptedSet)
				corruptedSet[graph[0]] = struct{}{}
			}
		}
	}

	path := p.findEdgePath(required, func(n *InstructionNode) bool {
		_, ok := corruptedSet[n]
		return ok
	}, setNodes)
	if path == nil {
		return
	}

	errorNode := path[0]
	if errorNode.Instruction == nil {
		return
	}

	corruptedNode := path[len(path)-1]
	isCorrupted := corruptedNode != graph[0] && corruptedNode != errorNode

	var msg string
	if isCorrupted {
		msg = diagnostics.MessagePointerCorrupted
	} else {
		msg = diagnostics.MessagePointerUninitialized
	}

	p.diagnostics.Report(diagnostics.NewDiagnostic(
		errorNode.Instruction.Source,
		diagnostics.DiagnosticError,
		msg,
		pt.Representation,
	))

	if isCorrupted && corruptedNode.Instruction != nil {
		p.diagnostics.Report(diagnostics.NewDiagnostic(
			corruptedNode.Instruction.Source,
			diagnostics.DiagnosticHint,
			diagnostics.MessagePointerCorruptedLoc,
			pt.Representation,
		))
	}
	p.logProcRequirement(errorNode, pt, analysis)
}

// cloneNodeSet returns a shallow copy of src so corrupted-set mutation does
// not leak back into the cached analysis.
func cloneNodeSet(src map[*InstructionNode]struct{}) map[*InstructionNode]struct{} {
	out := make(map[*InstructionNode]struct{}, len(src)+1)
	for k := range src {
		out[k] = struct{}{}
	}
	return out
}

// logProcRequirement walks the call graph downward from node, emitting
// MessagePointerRequiredLoc HINT diagnostics at every Gosub/Jump boundary
// where pt is required. When the called script directly requires pt
// (requiresPointerPathScript returns a path), emits a HINT at the path's
// first node and recurses into it. When the called script does NOT
// directly require pt, falls back to inspecting the call's
// staticLabelArgsByCall entry and emits a HINT at each label-typed
// parameter whose label requires pt and whose jump-param node confirms
// the requirement. Mirrors TS PointerChecker.logProcRequirement
// (RuneScriptTS src/compiler/codegen/script/config/PointerChecker.ts:243-301).
//
// TS throws on script-lookup miss / nil instruction source; goscape
// silently returns at those points — defensive fallthrough matching the
// no-panic posture documented at NAI-209-D-PUSHLONG-PANIC etc. The
// earlier error diagnostic already surfaces user-visible failure.
//
// Retires NAI-208-D-LOGPROCREQ-DEFERRED.
func (p *PointerChecker) logProcRequirement(
	node *InstructionNode,
	pt *pointer.PointerType,
	analysis *scriptPointerAnalysis,
) {
	inst := node.Instruction
	if inst == nil {
		return
	}
	if inst.Opcode != codegen.Gosub && inst.Opcode != codegen.Jump {
		return
	}

	sym, ok := inst.Operand.(symbol.Symbol)
	if !ok {
		return
	}
	calledScript, ok := p.scriptsBySymbol[sym]
	if !ok {
		return
	}

	scriptPath := p.requiresPointerPathScript(calledScript, pt)
	if scriptPath == nil {
		staticArgs, present := analysis.staticLabelArgsByCall[inst]
		if !present {
			return
		}
		jumpParamNodes := p.getJumpParamNodes(calledScript)
		// Sort param indexes for deterministic HINT emission order
		// (Go map iteration is unordered; mirrors NAI-210-D-LOADER-SORTED-ITERATION posture).
		indexes := make([]int, 0, len(staticArgs))
		for i := range staticArgs {
			indexes = append(indexes, i)
		}
		sort.Ints(indexes)
		for _, paramIndex := range indexes {
			labelSym := staticArgs[paramIndex]
			if !p.GetPointers(labelSym).Required.Has(pt) {
				continue
			}
			nodes := jumpParamNodes[paramIndex]
			if len(nodes) == 0 {
				continue
			}
			if !p.requiresPointerAtNodes(calledScript, pt, nodes) {
				continue
			}
			required := nodes[0]
			if required.Instruction == nil {
				continue
			}
			p.diagnostics.Report(diagnostics.NewDiagnostic(
				required.Instruction.Source,
				diagnostics.DiagnosticHint,
				diagnostics.MessagePointerRequiredLoc,
				pt.Representation,
			))
		}
		return
	}

	required := scriptPath[0]
	if required.Instruction == nil {
		return
	}
	p.diagnostics.Report(diagnostics.NewDiagnostic(
		required.Instruction.Source,
		diagnostics.DiagnosticHint,
		diagnostics.MessagePointerRequiredLoc,
		pt.Representation,
	))
	p.logProcRequirement(required, pt, analysis)
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

// SetSetsPointerTriggerFn overwrites the polymorphic setsPointerTrigger
// hook. Used by ServerPointerChecker to install its override. The default
// hook reads script.Trigger.Pointers.Has(pt).
func (p *PointerChecker) SetSetsPointerTriggerFn(fn func(*codegen.RuneScript, *pointer.PointerType) bool) {
	p.setsPointerTriggerFn = fn
}

// DefaultSetsPointerTrigger exposes the base implementation so overrides
// can call back into it for the non-overridden pointer kinds.
func (p *PointerChecker) DefaultSetsPointerTrigger(script *codegen.RuneScript, pt *pointer.PointerType) bool {
	return p.defaultSetsPointerTrigger(script, pt)
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
	// StrictFeatureLevel.DisableStaticLabelArgPropagation matches
	// RuneScriptTS v0.9.4 (pre-50c9bb1): no opportunistic label-ref pointer
	// checking. With propagation off, the static-arg map is left empty and
	// the per-instruction propagation in the Gosub/Jump arm below becomes
	// a no-op; the getJumpParamNodes cache is also unused.
	staticLabelArgsByCall := map[*codegen.Instruction]map[int]symbol.Symbol{}
	if !p.features.DisableStaticLabelArgPropagation {
		staticLabelArgsByCall = p.buildStaticLabelArgsByCall(script)
		p.getJumpParamNodes(script)
	}

	for _, node := range graph {
		// Synthetic pointer-set node (no instruction, has PointerSet).
		if node.Instruction == nil && node.PointerSet != nil {
			addPointersToArray(setArr, node.PointerSet, node)
			continue
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
			if staticArgs, present := staticLabelArgsByCall[inst]; present {
				p.addStaticLabelRequirements(required, node, sym, staticArgs)
			}

		case codegen.PushVar:
			if sym, ok := inst.Operand.(*symbol.BasicSymbol); ok {
				addBasicVarRequiredForSymbol(required, sym, node, false, false)
			}

		case codegen.PopVar:
			if sym, ok := inst.Operand.(*symbol.BasicSymbol); ok {
				addBasicVarRequiredForSymbol(required, sym, node, true, false)
			}

		case codegen.PushVar2:
			if sym, ok := inst.Operand.(*symbol.BasicSymbol); ok {
				addBasicVarRequiredForSymbol(required, sym, node, false, true)
			}

		case codegen.PopVar2:
			if sym, ok := inst.Operand.(*symbol.BasicSymbol); ok {
				addBasicVarRequiredForSymbol(required, sym, node, true, true)
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

// addBasicVarRequiredForSymbol is the T5 replacement for
// addBasicVarRequired. Takes the BasicSymbol so it can read IsProtected
// for the protected-pop branch. Retires NAI-208-D-PROTECTED-VAR-VIA-SYMBOL.
func addBasicVarRequiredForSymbol(target [][]*InstructionNode, sym *symbol.BasicSymbol, node *InstructionNode, pop, two bool) {
	switch sym.Type.(type) {
	case *typ.VarPlayerType, *typ.VarBitType:
		var pt *pointer.PointerType
		switch {
		case pop && two && sym.IsProtected:
			pt = pointer.PActivePlayer2
		case pop && !two && sym.IsProtected:
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
