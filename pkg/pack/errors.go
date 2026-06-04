package pack

import "errors"

// Package-level sentinel errors for recurring config-parsing failures.
// Promoted from inline fmt.Errorf literals per Arc 18 ERR-1 (continued
// from Arc 22 jagfile sentinel work in ccd0bf43) so callers can
// errors.Is against these without matching on formatted strings.
//
// Each sentinel covers a class of error that occurs in >2 distinct
// call sites across the pkg/pack root package's parse* functions for
// loc/npc/obj/seq/spotanim/idk/flo/inv/struct/enum/vars/varn/varp/
// param/mesanim. The human-readable formatted message (with %s key /
// %d value interpolation) is preserved at the call site via
// fmt.Errorf("...: %w", ErrXxx).
var (
	// ErrUnknownParam is returned when a `param=name,value` line
	// references a param name that isn't in the loaded param table.
	// Sites: loc.go (2), npc.go (2), obj.go (2), struct.go (1).
	ErrUnknownParam = errors.New("unknown param")

	// ErrUnknownModel is returned when an idk/spotanim/obj/npc config
	// references a model name that isn't in the loaded model table.
	// Sites: idk.go (2), spotanim.go (1), obj.go (2), npc.go (2).
	ErrUnknownModel = errors.New("unknown model")

	// ErrInvalidBoolean is returned when a boolean-typed config key
	// receives a value that isn't "yes" or "no".
	// Sites: loc.go, flo.go, idk.go, npc.go, obj.go, seq.go, spotanim.go.
	ErrInvalidBoolean = errors.New("invalid boolean")

	// ErrOutOfRange is returned when a numeric value falls outside the
	// type-specific allowed range for the config key.
	// Sites: seq.go (2), spotanim.go (3), npc.go (6), inv.go (1).
	ErrOutOfRange = errors.New("out of range")

	// ErrInvalidNumber is returned when a numeric-typed config key
	// receives a value that doesn't parse as an integer.
	// Sites: flo.go, loc.go, obj.go, seq.go, npc.go, spotanim.go.
	ErrInvalidNumber = errors.New("invalid number")

	// ErrUnknownSeq is returned when a config references a seq
	// (animation sequence) name that isn't in the loaded seq table.
	// Sites: npc.go (3), mesanim.go (1), obj.go (1).
	ErrUnknownSeq = errors.New("unknown seq")

	// ErrUnknownVarType is returned when an enum/vars/varn/varp/param
	// declaration uses a script var type that isn't recognised.
	// Sites: enum.go, vars.go, varn.go, varp.go, param.go.
	ErrUnknownVarType = errors.New("unknown script var type")

	// ErrInvalidRecol is returned when a recol_s/recol_d value can't
	// be parsed (expects a comma-separated integer pair / scalar).
	// Sites: spotanim.go, idk.go, loc.go, obj.go, npc.go.
	ErrInvalidRecol = errors.New("invalid recol value")

	// ErrUnknownObj is returned when a config references an obj name
	// that isn't in the loaded obj table.
	// Sites: inv.go, seq.go, obj.go (2).
	ErrUnknownObj = errors.New("unknown obj")

	// ErrUnknownAnim is returned when a config references an anim
	// (sequence) name that isn't in the loaded seq table; distinct
	// from ErrUnknownSeq in that the originating key is "anim" rather
	// than "seq" (TS parity preserves the user-facing distinction).
	// Sites: spotanim.go, loc.go, seq.go (2).
	ErrUnknownAnim = errors.New("unknown anim")
)
