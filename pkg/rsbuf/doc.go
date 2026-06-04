// Package rsbuf is goscape's native Go reimplementation of the upstream
// @2004scape/rsbuf renderer (originally a Rust crate consumed by the
// LostCityRS Engine-TS server via WASM). It encodes the per-tick
// PlayerInfo and NpcInfo packets that describe the local player plus
// every other player and NPC visible from a given vantage point.
//
// NAI-30 / NAI-31 — FORMAL CLOSURE (Arc 24 #177 / NAI-201 pattern,
// "TS doesn't ship it either"). Catalogued historically as a deferred
// "High effort, multi-arc renderer port". A scoping pass in May 2026
// confirmed the work is done; the backlog row was stale.
//
// Upstream posture:
//   - TS PlayerInfo.ts (9 LOC) and NpcInfo.ts (9 LOC) are bare message
//     containers exposing only `readonly bytes: Uint8Array`.
//   - TS PlayerInfoEncoder.ts (16 LOC) and NpcInfoEncoder.ts (16 LOC)
//     just call `buf.pdata(message.bytes, 0, message.bytes.length)`.
//   - The actual rendering pipeline lives in the external Rust crate
//     `@2004scape/rsbuf ^225.1.7` (Engine-TS package.json:37), imported
//     into TS via `import * as rsbuf from '@2004scape/rsbuf'`
//     (Engine-TS src/engine/World.ts:6-7). TS only sets mask bits and
//     calls into rsbuf for the actual encoding; it does not ship a
//     renderer impl.
//
// Go posture:
//   - This package ports the Rust renderer logic natively (no WASM
//     boundary): mask payload encoding, visibility, zone discovery,
//     new-player/NPC detection, appearance dedup, byte budgeting,
//     8191-terminator emission, and per-mask encoders.
//   - Wired into the tick loop at modules/world/tick.go (ComputePlayer,
//     ComputeNpc, ComputePlayers, ComputeNpcs) and emitted via
//     modules/world/player_info.go and player_npc_info.go (Encode).
//   - Shipped across NAI-30 Bundles 1-4 (last: cd585ea9 polish),
//     NAI-31 Bundles 1-3 (last: 24fb1538 8191-terminator), NAI-32
//     Tasks 1-3 + Bundle 3 Rust-canonical port (last: 93c8f4d0 mask
//     payload reorder), and NAI-116 fix (1ddc5f83 EntityMask gate).
//
// Conclusion: NAI-30/31 is a Case-B TS-parity exception (Arc 24 #177),
// not an open implementation gap. Do NOT re-investigate as an
// "outstanding port" unless TS itself begins shipping a renderer impl
// outside the @2004scape/rsbuf crate.
package rsbuf
