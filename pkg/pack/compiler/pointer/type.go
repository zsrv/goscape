// Package pointer ports TS src/compiler/pointer/ at HEAD
// b8c338801fbb72d294ff9576a58925a8d3f6de47. PointerType + PointerHolder
// describe the runtime entity-pointer state a script requires/sets/corrupts;
// PointerChecker (pkg/pack/compiler/cfg) consumes them for flow analysis.
//
// NAI-208-D-POINTERTYPE-PTR-SINGLETON: TS uses a class with a private
// constructor and static singletons (Object.values + reduce-keyed maps);
// goscape uses package-level *PointerType vars whose pointer identity is
// the equality key. Constructing new PointerType{} values bypasses the
// singletons and breaks PointerSet semantics — never do it outside this file.
package pointer

import "strings"

// PointerType identifies one entity-pointer kind. Representation is the
// canonical lowercase name used in user-facing diagnostic messages
// (e.g. "active_player"). Pointer identity (not Representation) is the
// equality key for sets and analysis arrays.
type PointerType struct {
	Representation string
}

var (
	ActivePlayer   = &PointerType{Representation: "active_player"}
	ActivePlayer2  = &PointerType{Representation: ".active_player"}
	PActivePlayer  = &PointerType{Representation: "p_active_player"}
	PActivePlayer2 = &PointerType{Representation: ".p_active_player"}
	ActiveNpc      = &PointerType{Representation: "active_npc"}
	ActiveNpc2     = &PointerType{Representation: ".active_npc"}
	ActiveLoc      = &PointerType{Representation: "active_loc"}
	ActiveLoc2     = &PointerType{Representation: ".active_loc"}
	ActiveObj      = &PointerType{Representation: "active_obj"}
	ActiveObj2     = &PointerType{Representation: ".active_obj"}
	FindPlayer     = &PointerType{Representation: "find_player"}
	FindNpc        = &PointerType{Representation: "find_npc"}
	FindLoc        = &PointerType{Representation: "find_loc"}
	FindObj        = &PointerType{Representation: "find_obj"}
	FindDb         = &PointerType{Representation: "find_db"}
	LastCom        = &PointerType{Representation: "last_com"}
	LastInt        = &PointerType{Representation: "last_int"}
	LastItem       = &PointerType{Representation: "last_item"}
	LastSlot       = &PointerType{Representation: "last_slot"}
	LastTargetSlot = &PointerType{Representation: "last_targetslot"}
	LastUseItem    = &PointerType{Representation: "last_useitem"}
	LastUseSlot    = &PointerType{Representation: "last_useslot"}
)

// All enumerates every PointerType singleton in declaration order. Mirrors
// TS PointerType.ALL (computed via Object.values+filter). Index returns the
// position within this slice; PointerChecker indexes analysis arrays with it.
var All = []*PointerType{
	ActivePlayer, ActivePlayer2, PActivePlayer, PActivePlayer2,
	ActiveNpc, ActiveNpc2,
	ActiveLoc, ActiveLoc2,
	ActiveObj, ActiveObj2,
	FindPlayer, FindNpc, FindLoc, FindObj, FindDb,
	LastCom, LastInt, LastItem, LastSlot, LastTargetSlot, LastUseItem, LastUseSlot,
}

// indexByPointer maps every singleton in All to its 0-based position.
// Populated by init() once per program; reads are lookup-only.
var indexByPointer = func() map[*PointerType]int {
	m := make(map[*PointerType]int, len(All))
	for i, p := range All {
		m[p] = i
	}
	return m
}()

// Index returns the position of pt within All. Panics if pt is not one of
// the package singletons — constructing fresh PointerType{} values is
// forbidden (see NAI-208-D-POINTERTYPE-PTR-SINGLETON).
func Index(pt *PointerType) int {
	i, ok := indexByPointer[pt]
	if !ok {
		panic("pointer.Index: unknown PointerType " + pt.Representation)
	}
	return i
}

// nameToType maps lowercase Representation → singleton. Populated once.
var nameToType = func() map[string]*PointerType {
	m := make(map[string]*PointerType, len(All))
	for _, p := range All {
		m[strings.ToLower(p.Representation)] = p
	}
	return m
}()

// ForName resolves the lowercase Representation back to its singleton, or
// nil if name does not match any pointer. Mirrors TS PointerType.forName.
func ForName(name string) *PointerType {
	return nameToType[strings.ToLower(name)]
}
