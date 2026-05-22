package script

import "errors"

// Package-level sentinel errors used by script handlers and dispatch
// helpers. Each handler wraps these with its own opcode-prefixed
// fmt.Errorf("OP: %w", ErrXxx) so callers can errors.Is(err, ErrXxx)
// regardless of which opcode produced the failure, while the human-
// readable message keeps its handler tag for log triage.
//
// Threshold for promotion (Arc 18 ERR-1): a string is sentinelised when
// it recurs across multiple handlers or is a likely errors.Is target
// from tests/callers. Single-use bespoke errors stay as inline
// fmt.Errorf to avoid sentinel-soup.
var (
	// ErrNoActivePlayer is returned by requireActivePlayer when the
	// script has no PtrActivePlayer / s.Self bound. Mirrors TS
	// `checkedHandler(ActivePlayer, ...)`.
	ErrNoActivePlayer = errors.New("no active player")

	// ErrNoActivePlayer2 is the secondary-slot dual of ErrNoActivePlayer,
	// returned by requireActivePlayer2 when PtrActivePlayer2 / s.Self2 is
	// unbound. NAI-39.
	ErrNoActivePlayer2 = errors.New("no active player2")

	// ErrScriptNotProtected is returned by requireProtectedActivePlayer
	// when the script holds an ActivePlayer slot but not the protect
	// flag. Chained after ErrNoActivePlayer so the unprotected-but-bound
	// case reports this; the wholly-unbound case reports
	// ErrNoActivePlayer.
	ErrScriptNotProtected = errors.New("script not protected")

	// ErrNoActiveNpc is returned by requireActiveNpc when the script has
	// no ActiveNpc bound. Mirrors TS `checkedHandler(ActiveNpc, ...)`.
	ErrNoActiveNpc = errors.New("no active npc")

	// ErrNoWorld is returned by handlers that need a World reference
	// (tick counter, surface lookup, member-flag, etc.) but have
	// s.World == nil.
	ErrNoWorld = errors.New("no world")

	// ErrNoProvider is returned by control-flow handlers (GOSUB, JUMP,
	// JUMP_WITH_PARAMS, GOSUB_WITH_PARAMS) when s.Provider is unbound.
	// The same sentinel covers the variant "Provider not set on
	// ScriptState" used by GOSUB_WITH_PARAMS — wrappers preserve the
	// opcode tag.
	ErrNoProvider = errors.New("no provider")

	// ErrTriggerUnsafe is returned by LAST_* dialog opcodes when invoked
	// from a trigger whose allowlist does not include them. The handler
	// wrapper formats as "LAST_FOO: is not safe to use in this trigger"
	// for parity with TS's per-opcode error.
	ErrTriggerUnsafe = errors.New("is not safe to use in this trigger")
)
