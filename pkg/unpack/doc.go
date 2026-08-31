// Package unpack is goscape's cache-unpacking toolchain: it decodes a packed
// RS2 cache back into the source-tree formats the packer consumes. It backs
// the `goscape-cli unpack` subcommand and covers 16 families (config, sprite,
// model, anim, midi, sound, map, interface, versionlist, worldmap, …).
//
// # PORTING-EXCEPTION (unpack-tools-go-only) — DEVIATION SYNC289-D2
//
// This package no longer has an upstream counterpart. Engine-TS 8139461a
// ("Synced engine with 289 improvements", the rev-274 sync landed 2026-08-31)
// deleted tools/unpack/** and tools/render/sound/** outright, ~4,900 lines.
// goscape deliberately KEEPS its equivalent, following the precedent set by
// symbols-export-go-only (see docs/PORTING.md).
//
// Rationale: the upstream deletion is repo housekeeping with no runtime,
// packed-byte or wire consequence — no server code path, no cache artifact and
// no protocol message depends on it. goscape's counterpart, by contrast, is a
// shipped capability with a documented CLI surface and a decoder-conformance
// fixture corpus (Server274-ref/unpack-ref). Mirroring the deletion would
// remove working functionality and its tests to track a change that means
// nothing on the Go side.
//
// Consequence for future parity work: from the 1d25566c pin forward this
// package is goscape-owned. An audit must NOT report its divergence from a
// (now absent) TS reference as unclosed drift. The last upstream unpack change
// goscape will ever port is Pix.ts's write-only-if-changed / .opt-cleanup
// behaviour from that same commit.
package unpack
