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
//
// rev-245.2: TS Environment.ts:27 (3c16994c) defaults ENGINE_REVISION to 245
// and World.ts:2158 rejects any other client revision with login reply 6
// ("RuneScape has been updated!"). Origin story: found live in the rev-244
// B6 client smoke — the pinned 244 client was rejected while this constant
// still said 225. Goscape-only constants (no TS counterpart file) are
// invisible to TS-diff slicing; a repo-wide grep for the old value is
// required on every revision bump.
const Expected = 245
