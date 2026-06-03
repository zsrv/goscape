// Package revision exposes the RuneScape wire-protocol revision that this
// goscape binary implements. Each goscape branch ships exactly one revision.
package revision

// Expected is the wire-protocol revision compiled into this binary.
// Bumping this constant is a branch-level decision: each goscape branch
// implements one revision and only one.
//
// Untyped on purpose: the world server compares it against a uint8 read off
// the wire, while other consumers may carry the revision as a wider integer
// (e.g. uint16). An untyped constant fits both without forcing a cast.
const Expected = 225
