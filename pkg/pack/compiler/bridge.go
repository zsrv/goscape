// Package compiler — NAI-212 bridge from compiler.TypeInfo (NAI-200
// dual-map shape) to runescript.CompilerTypeInfo (NAI-210 single-map
// shape). Pure conversion, no IO.
package compiler

import (
	"maps"
	"strconv"

	"github.com/zsrv/goscape/pkg/pack/compiler/runescript"
)

// ToCompilerTypeInfo bridges *compiler.TypeInfo → *runescript.CompilerTypeInfo.
//
// Shape divergence summary (per NAI-212 spec §4):
//   - compiler.TypeInfo splits TS `map: Record<string, string>` into
//     Map (map[int]string) + NameMap (map[string]string) because Go
//     forbids mixed-type keys in one map.
//   - runescript.CompilerTypeInfo carries Map as map[string]string,
//     mirroring the TS single-map shape used by the compiler driver.
//
// Conversion rules:
//  1. Int-keyed Map entries → dst.Map with strconv.Itoa(k) → v.
//  2. NameMap entries → dst.Map under their string keys.
//  3. On key collision (impossible in TS-canonical call sites since
//     loadRecords is only used for constantInfo, but defensively
//     enforced): numeric-id entries win.
//  4. Auxiliary int-keyed maps (VarType/Protect/Require/Require2/
//     Set/Set2/Corrupt/Corrupt2/Conditional) → dst with stringified
//     keys, values preserved.
//  5. Max copies as-is.
func ToCompilerTypeInfo(src *TypeInfo) *runescript.CompilerTypeInfo {
	if src == nil {
		return nil
	}
	dst := &runescript.CompilerTypeInfo{
		Max:         src.Max,
		Map:         make(map[string]string, len(src.Map)+len(src.NameMap)),
		Vartype:     make(map[string]string, len(src.VarType)),
		Protect:     make(map[string]bool, len(src.Protect)),
		Require:     make(map[string]string, len(src.Require)),
		Require2:    make(map[string]string, len(src.Require2)),
		Set:         make(map[string]string, len(src.Set)),
		Set2:        make(map[string]string, len(src.Set2)),
		Corrupt:     make(map[string]string, len(src.Corrupt)),
		Corrupt2:    make(map[string]string, len(src.Corrupt2)),
		Conditional: make(map[string]bool, len(src.Conditional)),
	}

	// Rule 2: NameMap first, so numeric-id entries overwrite on collision.
	maps.Copy(dst.Map, src.NameMap)
	// Rule 1 + 3: numeric-id entries (precedence).
	for k, v := range src.Map {
		dst.Map[strconv.Itoa(k)] = v
	}

	// Rule 4: auxiliary maps.
	for k, v := range src.VarType {
		dst.Vartype[strconv.Itoa(k)] = v
	}
	for k, v := range src.Protect {
		dst.Protect[strconv.Itoa(k)] = v
	}
	for k, v := range src.Require {
		dst.Require[strconv.Itoa(k)] = v
	}
	for k, v := range src.Require2 {
		dst.Require2[strconv.Itoa(k)] = v
	}
	for k, v := range src.Set {
		dst.Set[strconv.Itoa(k)] = v
	}
	for k, v := range src.Set2 {
		dst.Set2[strconv.Itoa(k)] = v
	}
	for k, v := range src.Corrupt {
		dst.Corrupt[strconv.Itoa(k)] = v
	}
	for k, v := range src.Corrupt2 {
		dst.Corrupt2[strconv.Itoa(k)] = v
	}
	for k, v := range src.Conditional {
		dst.Conditional[strconv.Itoa(k)] = v
	}

	return dst
}
