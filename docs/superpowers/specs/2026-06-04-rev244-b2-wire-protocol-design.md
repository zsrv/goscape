# rev-244 Bundle 2: wire protocol + rsbuf — design

**Date:** 2026-06-04
**Status:** Approved
**Branch:** rev-244
**Umbrella:** `2026-06-03-rev244-port-design.md` (B2 row)
**Prerequisite:** the B5-early worker/multiworld evaluation
(`2026-06-04-rev244-worker-multiworld-eval.md`) concluded the worker delta has
no game-client wire impact — B2's handler shapes are safe to freeze.

## Goal

Port the B2 slice of the Engine-TS cross-pin diff
(`git -C ../Engine-TS diff e1dea19f..9aadcec4 -- src/network`; 115 files,
+620/−946) plus the `@2004scape/rsbuf` crate delta `225.1.7 → 244.1.0`, with
the damage2 entity feed pulled forward from B3 (user-approved scope decision).

**rsbuf reference:** local clone `/home/owner/Code/github.com/2004scape/rsbuf`.
The 225 baseline is branch `225` (Cargo.toml `225.1.7`, the prior audit pin);
the 244 target is `origin/244` tip `1defefb` ("feat: add support for damage2"),
**verified identical** to the published npm `244.1.0` dist consumed by
Engine-TS at the pin (`node_modules/@2004scape/rsbuf/dist/rsbuf.d.ts`
signature + enum comparison). The whole delta is that one commit: 6 files,
+64/−8.

**rsbuf audit depth (user-approved):** delta-port + targeted spot-check —
while in each touched Go file, diff-check the surrounding mask/write-order/
cache logic against the 244 Rust source; verdicts recorded in the audit
trail. Not a full 6.4k-LOC line re-audit.

## Verified scope facts

| Surface | 244 change | goscape target |
|---|---|---|
| `ClientGameProt.ts` (81/78) | all ~80 opcodes renumbered; ctor `(opcode,size)` → `(index,opcode,size)` (NXT packet index — **zero readers at the pin**, only written into `ClientGameProt.all`); removed `REBUILD_GETMAPS`, `EVENT_CAMERA_POSITION`; renamed `IDK_SAVEDESIGN→IF_PLAYERDESIGN` (op 8, size 13), `TUT_CLICKSIDE→TUTORIAL_CLICKSIDE` (op 233, size 1); new explicit `NoTimeout` model/decoder (+6/+11; packet existed at 225 as size-0) | `pkg/io/protocol/game/client/prot.go` + `modules/world/handlers_game.go` registration |
| `ServerGameProt.ts` (58/61) | all opcodes renumbered (ctor stays `(id,length)`); removed `DATA_LAND`, `DATA_LAND_DONE`, `DATA_LOC`, `DATA_LOC_DONE` (map delivery moves to engine OnDemand — **B3**); added `IF_OPENOVERLAY` (158, 2) + `IfOpenOverlayEncoder` (+12); `MidiSongEncoder`/`MidiJingleEncoder` deltas | `pkg/io/protocol/game/server/prot.go` named `Op` vars + their emitters in `modules/world` |
| `ServerGameZoneProt.ts` (10/10) | all 10 zone opcodes renumbered | same |
| Handler family (~25 files, each 5-50 lines) | behavioral re-shapes; sampled `OpHeldHandler`: validation reorder, `clearPendingAction()` added to every reject path, `delayed` check moved AFTER validation, explicit per-op trigger dispatch replaces `OPHELD1 + (op-1)` arithmetic, sessionlog only for ops 1-4; decoder read-order deltas in OpHeldU/T, OpNpcU/T, OpPlayerU/T, InvButtonD (3-4 lines each) | `modules/world/handler_*.go` (shared-impl convention preserved) |
| rsbuf `1defefb` | player `DAMAGE2 = 0x400` appended (write-ordinal 5; FACE_COORD→6, CHAT→7, SPOT_ANIM→8); **NPC mask bits ALL shift** (`DAMAGE2` inserted at `0x1`: ANIM 1→2 … FACE_COORD 64→128; write-ordinal 4, CHANGE_TYPE→5, SPOT_ANIM→6, FACE_COORD→7); caches 8→9 (player) / 7→8 (npc); `computePlayer`/`computeNpc` gain `damageTaken2`,`damageType2`; DAMAGE2 encoders reuse the Damage payload shape `(taken2, type2, currentHP, baseHP)` | `pkg/rsbuf` + `modules/world` bridge call sites |
| damage2 entity feed (pulled forward from B3) | `PathingEntity.ts:95-96` `hitmark2Damage/Type` fields + `:608-609` per-tick reset; `hitmarkSlot` counter; `applyDamage` even/odd alternation (`Player.ts:1870-1889`, `Npc.ts:475-493`) | `modules/world` Player + Npc forks (BOTH — shared-TS-base-class rule) |

## Structural decisions

1. **Opcode-keyed table stays.** goscape's `Ops [256]` lookup keyed by wire
   opcode is retained; the TS `index` field is recorded as a **NO-OP decision
   row** (no consumer at the pin; revisit only if later TS reads
   `ClientGameProt.all`).
2. **Named-constant registration.** `modules/world/handlers_game.go` currently
   registers handlers by raw opcode literal (`gameHandlers[194] = handleOpNpc1
   // OPNPC1`). The renumber forces editing every registration line anyway, so
   the registrations switch to named opcode constants exported by
   `pkg/io/protocol/game/client` — eliminating silent-drift risk for future
   revisions. Server-side emitters already use named `Op` vars; only values
   change.

## Slices (dependency order; one plan, B1 subagent-driven process)

### Slice 1 — opcode tables (foundation)

Regenerate both prot tables from the 244 TS source. Contract = exact
extraction commands:
`git -C ../Engine-TS show 9aadcec4:src/network/game/client/ClientGameProt.ts`
(and server / zone equivalents). Includes: removals' Go code (the
`REBUILD_GETMAPS` handler + `EVENT_CAMERA_POSITION` plumbing,
`DATA_*` emitters), renames with compile-cascade chase (adopt 244 names),
`IF_OPENOVERLAY` encoder, Midi encoder deltas, the named-constant
registration refactor, and regenerated table pin tests.

### Slice 2 — handler family (depends on slice 1)

Per-handler translation against each TS file's hunk of the cross-pin diff
(read the full diff per file before editing — B1 rule). Preserve goscape's
shared-impl convention: one Go impl per TS handler; wire deltas
parameterized, never behavior forks. The protected-access invariant
(`runScript` guard) is load-bearing — any handler change touching it is
verified against TS, never weakened.

### Slice 3 — rsbuf damage2 + entity pull-forward (independent; last)

- `pkg/rsbuf`: mask renumbers (both enums), write-order tables, cache bumps,
  DAMAGE2 encoders.
- Pre-change grep-audit: ALL `masks |=` / mask-constant sites in
  `modules/world` for raw-number usage (any literal silently breaks every
  NPC update after the shift).
- Bridge: `ComputePlayer`/`ComputeNpc` +2 params; call sites feed the new
  entity fields.
- Entity pull-forward: fields + reset in BOTH forks, `hitmarkSlot`,
  `applyDamage` alternation. **B3 must NOT double-apply** these hunks
  (decision rows cite the exact TS lines).
- Spot-check the surrounding logic of each touched file vs the 244 Rust
  source; record verdicts.

## Testing

- **Table pin tests** regenerated from the 244 source: every (name, opcode,
  size) tuple, client + server + zone. Existing 225-contract tests are
  UPDATED (the old contract is the wrong contract on this branch).
- **Handler tests**: each behavioral change pinned (reject-path
  `clearPendingAction`; delayed-check ordering; OPHELD5 sessionlog skip; …);
  existing tests updated to 244 after TS verification.
- **rsbuf tests**: mask-bit values pinned for both enums; write-order;
  DAMAGE2 payload encode; hitmarkSlot alternation (hit1→DAMAGE, hit2→DAMAGE2,
  per-tick reset).
- **Gates**: per slice — build + vet + touched-package tests. Bundle end —
  full `go test ./... -count=1` (capture real exit code), `-race` on
  `pkg/rsbuf` + `modules/world` + `pkg/io/protocol/...`,
  `CGO_ENABLED=0 go build -trimpath ./...`, correspondence-audit task, final
  whole-bundle integration review.

## Known windows (documented, by design)

- **Map-delivery window:** `DATA_*` removed in B2; engine OnDemand lands in
  B3; no client map delivery in between. End-to-end 244-client smoke is
  already gated "after B2+B3" (umbrella §Testing). PORTING.md row, expires
  at B3.
- The B1 format window (244 decoders vs 225 pack) continues unchanged;
  expires at B6.

## Risks (ordered)

1. **NPC mask renumber drift** — mitigated by the pre-change grep-audit +
   bit-value pin tests.
2. **Handler behavior coupling** — `clearPendingAction` on reject paths
   interacts with interaction/pathing state; per-file TS verification, no
   pattern-application across handlers.
3. **Registration renumber typos** — mitigated by named-constant refactor +
   table pin tests.
4. **Decoder read-order changes** are byte-level wire contract — verbatim
   translation with `// TS <File>.ts:<lines>` citations (244 pin).

## Tracker rows (PORTING.md §rev-244 Bundle audit trail, B2 subsection)

- Correspondence table: every TS file in the B2 scope diff → Go commit /
  decision (NO-OP rows for TS-infra-only files, e.g.
  `ClientGameProtRepository` import shuffles).
- Decision rows: TS `index` field NO-OP; map-delivery window; damage2 entity
  pull-forward (B3 must-not-double-apply, citing PathingEntity.ts:95-96,
  608-609 / Player.ts:1870-1889 / Npc.ts:475-493); rsbuf spot-check verdicts;
  `REBUILD_GETMAPS` removal — check coupling with the bundle-17 staff-only
  rebuild-broadcast PORTING-EXCEPTION and close/supersede it if the marker's
  code is deleted.

## Definition of done (B2)

- Every hunk of the B2 scope diff maps to a Go commit or decision row.
- All gates green; suite + `-race` on touched packages.
- PORTING.md B2 audit-trail subsection complete (B1 format).
- No end-to-end client gate here — that is the post-B3 smoke (umbrella).
