// Package revision exposes the RuneScape wire-protocol revision that this
// goscape binary implements. Each goscape branch ships exactly one revision.
package revision

// Expected is the wire-protocol revision compiled into this binary.
// Bumping this constant is a branch-level decision: each goscape branch
// implements one revision and only one.
//
// Untyped on purpose: the world server compares it against a uint16 read off
// the wire (uint8 before rev-274's 0xff+u2 escape), while other consumers may
// carry the revision as a wider integer. An untyped constant fits all without
// forcing a cast.
//
// rev-245.2: TS Environment.ts:27 (3c16994c) defaults ENGINE_REVISION to 245
// and World.ts:2158 rejects any other client revision with login reply 6
// ("RuneScape has been updated!"). Origin story: found live in the rev-244
// B6 client smoke — the pinned 244 client was rejected while this constant
// still said 225. Goscape-only constants (no TS counterpart file) are
// invisible to TS-diff slicing; a repo-wide grep for the old value is
// required on every revision bump.
//
// rev-254: TS Environment.ts:27 (43e02957) defaults ENGINE_REVISION to 254.
//
// rev-274: TS WorldConfig.ts:91 (dee467c8) sets engine.revision to 274
// (Environment.ts was folded into WorldConfig.ts upstream; ENGINE_REVISION
// env override at WorldConfig.ts:231). 274 exceeds one byte, so the login
// wire carries it as the 0xff escape marker + u2 (World.ts:2136-2138) —
// see pkg/io/protocol/login/req.
const Expected = 274
