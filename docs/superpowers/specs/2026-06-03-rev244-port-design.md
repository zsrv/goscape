# rev-244 port — 225→244 server delta — umbrella design

**Date:** 2026-06-03
**Status:** COMPLETE (2026-06-06 — B1..B7 all shipped; definition-of-done
(a)-(d) met; see PORTING.md §rev-244 Bundle audit trail §B7 close-out)
**Branch:** all work lands on `rev-244` (cut from `rev-225` at `bf073fcc`)

## Goal

`rev-244` = `rev-225` + the translated Engine-TS delta. Work list = the
cross-pin diff `git -C Engine-TS diff e1dea19f..9aadcec4` (pins in
`main:REFERENCES.md` §rev-244; the pins share lineage — merge-base
`de5fa4db` — so the diff is a clean change-for-change work list, unlike the
client's deob-divergent Java references).

**Definition of done:**

- (a) The Go branch diff (`git diff rev-225 rev-244`) corresponds
  change-for-change to the TS cross-pin diff (PORTING-LESSONS §2 audit).
- (b) A 244 client (Client-Java `01f16088` / goscape-client `rev-244`) logs
  in and plays against goscape.
- (c) Pack output is byte-parity against a 244 reference cache produced by
  the upstream toolchain (Engine-TS 244 + Content 244 + RuneScriptKt-26 jar).
- (d) Suite green incl. `-race` on touched packages.

**Scope decisions (user-approved):** core server parity; `tools/unpack`
ports as a `goscape-cli unpack` command; the worker/multiworld architecture
gets a written evaluation — adopt what fits goscape, map the rest to dskit
modules or document as PORTING-EXCEPTION. The upstream delta is 332 files,
+9,984/−4,660, 97 commits; the engine surface in `src/` is 155 M / 12 A /
15 D / 13 R.

## Bundle decomposition (dependency order)

Each bundle is its own spec → plan → implementation cycle; this umbrella
spec governs the decomposition. Commits land on `rev-244` only — no shared
code across revision branches.

| # | Bundle | Upstream surface | Go targets |
|---|---|---|---|
| B1 | io/cache/util primitives | new `src/io/FileStream.ts`, `GZip.ts`, `PemUtil.ts`; `src/util/DoublyLinkList.ts`; `src/cache` loading rework (−435/+134) incl. config decoders (SeqType/AnimFrame, Component, NpcType, ObjType) + `wordenc/WordEnc.ts` raw-path | `pkg/io`, `pkg/cache`, `pkg/objtype`, `pkg/wordenc` |
| B2 | Wire protocol + rsbuf | `ClientGameProt.ts`/`ServerGameProt.ts` opcode renumber; the OpHeld*/OpHeldU/InvButton/OpNpc*/OpObj*/OpLoc*/OpPlayerU handler family; `@2004scape/rsbuf` `^225.1.7→^244.1.0` | `pkg/io/protocol`, `modules/world` handlers, `pkg/rsbuf` re-audit |
| B3 | Engine core | `World.ts` (262/232), entity family (`Player.ts` 73/73, `Npc.ts`, `NetworkPlayer.ts`, `EntityList.ts`), new engine `OnDemand.ts` (+123), `InputTrackingBlob.ts` | `modules/world`, `modules/ondemand` |
| B4 | Script runtime | `ScriptOpcode.ts` (226/206), `ServerOps.ts` +175, `DebugOps.ts` +55, `PlayerOps`/`InvOps`, `ScriptOpcodePointers` | `pkg/script` |
| B5 | Server/login/db + worker evaluation | `src/server/login/Messages.ts`, `Worker*`, prisma singleworld/multiworld schema deltas; **deliverable: written worker/multiworld evaluation** | `modules/login`, `modules/friends`, SQLite schemas |
| B6 | Pack pipeline re-baseline | `tools/pack` delta (~1.3k/1.2k); byte-diff loop vs the 244 reference cache | `pkg/pack` |
| B7 | `goscape-cli unpack` | new `tools/unpack` (+3,793) | `cmd/goscape-cli` |

Ordering rationale: B1 has no dependencies and unblocks types used
everywhere. The B5 worker **evaluation document** is a prerequisite read for
B2 (if the worker model changed the login/friends wire, that surface moves
into protocol work). B6 follows B4 (script-op changes affect compiler
output). B7 is leaf tooling.

## Method (fixed by PORTING-LESSONS §2/§3)

Per bundle: slice the cross-pin diff to the bundle's files → translate each
hunk into the corresponding Go region applying the §3 gotcha rules → pin
tests (RED→GREEN) → tracker rows in `rev-244`'s `PORTING.md` for any
deviation (closure shapes per Tracking conventions) → gates
(`CGO_ENABLED=0 go build -trimpath ./...`, `go vet`, `go test ./...`,
`-race` on touched packages). `// TS <File>.ts:<lines>` citations refer to
the **244 pin**; the branch is the revision context (no per-comment tags).

## Risks (ordered)

1. **Compiler swap (B6)** — 244 replaced the `@lostcityrs/runescript` npm
   compiler with the RuneScriptKt-26 jar. Output differences vs the TS
   lineage are the biggest unknown; the fix-determinism-first byte-diff
   rule applies. De-risk: generate the 244 reference cache EARLY (during
   B1) from the local Engine-TS 244 checkout, so its shape is known long
   before B6.
2. **rsbuf 244 crate delta** — `pkg/rsbuf` is ~6.4k LOC reimplementing the
   crate; the `^244.1.0` delta is unknown until the B2 audit.
3. **Worker entanglement** — mitigated by ordering (B5 evaluation before B2
   freeze).

## Testing

Per-bundle pins + suite + `-race`; end-to-end gates: after B2+B3 a 244
client login smoke; after B6 byte-parity; final = a delta-scoped
diff-correspondence audit (rev-225 fresh-audit style, scoped to changed
regions).
