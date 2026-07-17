# Reference Sources

The upstream sources each Go revision was ported **from**. Branch names move,
so the **commit hash is the real pin** — treat this file like a lockfile for
the port. To port a new revision, diff the new reference commit against the
commit recorded here for the revision you branch from (see the "Porting
workflow" section of `PORTING-LESSONS.md`).

Local working-copy paths are machine-specific and do not belong here; only
the portable URL / branch / commit do.

## rev-225 — Go branch `rev-225`

| Repo | Role | URL | Branch | Pinned commit |
|---|---|---|---|---|
| Engine-TS | **primary** — authoritative translation source; every ported Go region maps to a TS function | https://github.com/LostCityRS/Engine-TS | `225` | `e1dea19f256c7ff1a89d47024c811c755ad2184d` |
| Content | game content (`.rs2` scripts, configs, maps) packed and served by the server | https://github.com/LostCityRS/Content | `225` | `9901aa27b60198afac49012f45f32e4eb4d5c012` |
| Client-Java | the client this server speaks to; wire-protocol cross-check | https://github.com/LostCityRS/Client-Java | `225-clean` | `cc3781de9e45265c52711dca850cd154f03c3a2c` |
| RuneScriptTS | RuneScript compiler reference for `pkg/script` + the pack pipeline (`@lostcityrs/runescript`; Engine-TS at the pin above depends on `^0.9.4`) | https://github.com/LostCityRS/RuneScriptTS | `main` | `750291cf59f55f64d8a9565d2607110b532dad94` |
| Engine | engine reference (Java) | https://github.com/LostCityRS/Engine | `main` | `5b5584280d910511ac5635e1025b9fd2912a8264` |
| Server | runnable meta-repo whose `engine/` checkout is Engine-TS at the pinned commit; the TS-packed-cache **byte-parity baseline** for the pack pipeline | https://github.com/LostCityRS/Server | `main` | `326bb4a3b24fbf7a1bf503ec598a4c2cab118ee1` |

(Commits captured 2026-06-03 from the goscape-client `REFERENCES.md` 225 pins
plus the local reference checkouts. The local working copies have since moved
to 244 branches — the pins above are what the rev-225 port corresponds to,
regardless of where those branches point now.)

Notes:

- The packer writes `jagFileVersion=26`; do **not** bump it to 27 unless the
  upstream Server meta-repo pins `@lostcityrs/runescript` past `750291c`
  (see `PORTING-LESSONS.md` §3, "Pack pipeline / byte parity").

## rev-244 — Go branch `rev-244`

| Repo | Role | URL | Branch | Pinned commit |
|---|---|---|---|---|
| Engine-TS | **primary** — authoritative translation source | https://github.com/LostCityRS/Engine-TS | `244` | `9aadcec4e9560b810b5e5eee31aadc67f3b206cd` |
| Content | game content packed and served by the server | https://github.com/LostCityRS/Content | `244` | `e5d0282e03b383efd3b2a81e63090e703ffb5399` |
| Client-Java | the client this server speaks to; wire-protocol cross-check | https://github.com/LostCityRS/Client-Java | `244` | `01f1608842acb12901f7e4f3df25553f641cc86e` |
| RuneScriptKt | RuneScript compiler — **replaces RuneScriptTS in 244** (see note) | https://github.com/LostCityRS/RuneScriptKt | release tag `26` | jar sha256 `38e16e2c375cfdb0179cce1cab9c06d279cc7c30b0cbc298c97a37c4dca1851a` (the release-26 `RuneScriptCompiler.jar` is the effective pin — captured 2026-06-05 from the upstream-auto-downloaded, checksum-verified jar; local source checkout sits at tag `22` and is not used) |
| cloudflare/zlib | gzip byte-parity reference — bun's `node:zlib.gzipSync` (the upstream pack toolchain's gzip) is this vendored fork; goscape's `pkg/io/gziputil` ports its level-6 deflate bit-exactly (B6) | https://github.com/cloudflare/zlib | `gcc.amd64` lineage | `886098f3f339617b4243b286f5ed364b9989e245` (bun 1.2.20 `process.versions.zlib`; verified to reproduce all 4,764 reference cache gzip members byte-identically — stock zlib/zlib-ng/libdeflate do NOT) |

(Commits captured 2026-06-03 from the local reference checkouts, matching the
goscape-client `REFERENCES.md` rev-244 pins. Go branch `rev-244` is cut from
`rev-225` at `21b66635`. Engine (Java) and Server — pinned at `main` for
rev-225 — have no 244-specific checkout yet; record them here if/when a
244-specific need arises, otherwise the rev-225 pins remain the last-known
reference.)

Notes:

- **The 225→244 diff is a clean work list.** Unlike the client's Java
  references (divergent deob lineages), the Engine-TS `225` and `244` pins
  share history (merge-base `de5fa4db`), so
  `git -C Engine-TS diff e1dea19f..9aadcec4` is the real server delta.
- **Compiler swap:** 244 drops the `@lostcityrs/runescript` npm dependency.
  `src/util/RuneScriptCompiler.ts` auto-downloads `RuneScriptCompiler.jar`
  from RuneScriptKt releases at `ScriptProvider.COMPILER_VERSION = 26` and
  verifies its sha256. Pack byte-parity work on rev-244 must track
  RuneScriptKt-26 output, not RuneScriptTS.
- **`@2004scape/rsbuf` bumps `^225.1.7` → `^244.1.0`** — `pkg/rsbuf` (the Go
  reimplementation of that crate, see `pkg/rsbuf/doc.go`) must be re-audited
  against the 244 crate. `@2004scape/rsmod-pathfinder` is unchanged (`^5.0.4`).

## rev-245.2 — Go branch `rev-245.2`

| Repo | Role | URL | Branch | Pinned commit |
|---|---|---|---|---|
| Engine-TS | **primary** — authoritative translation source | https://github.com/LostCityRS/Engine-TS | `245.2` | `3c16994ca4ba51b4e04f88316c1f7395b0c4bb8a` |
| Content | game content packed and served by the server | https://github.com/LostCityRS/Content | `245.2` | `cbcfe6706ef9f4093e5b8e4c9cfee93577346993` |
| Client-Java | the client this server speaks to; wire-protocol cross-check | https://github.com/LostCityRS/Client-Java | `245.2` | `176a85f7b423111c878a476e1ead048745e377c0` |
| RuneScriptKt | RuneScript compiler | — | — | unchanged from §rev-244 (release tag `26`; `ScriptProvider.COMPILER_VERSION = 26` at the 245.2 pin) |
| cloudflare/zlib | gzip byte-parity reference | — | — | unchanged from §rev-244 (`886098f3`) |

(Commits captured 2026-06-09, matching the goscape-client `REFERENCES.md`
rev-245.2 pins. Go branch `rev-245.2` is cut from `rev-244` at `2ecde050`.)

Notes:

- **Lineage:** the upstream `245.2` branch diverged from the `244` lineage at
  merge-base `4095da3b`; most 244-era fixes were cherry-picked onto both
  sides and cancel out, so the net cross-pin diff
  `git -C Engine-TS diff 9aadcec4..3c16994c` (26 files, +246/−178) is the
  real work list.
- **The diff reverts two 244-only commits** and goscape follows the 245.2
  pin: the hiscores banned-accounts gate (`ccc263c7`, absent from 245.2) and
  the friends staff threshold (`staffLvl > 1` → `> 0`).
- **No toolchain swap:** `@2004scape/rsbuf` stays `^244.1.0` (no `pkg/rsbuf`
  re-audit), `@2004scape/rsmod-pathfinder` stays `^5.0.4`, the compiler
  stays the RuneScriptKt release-26 jar.

## rev-254 — Go branch `rev-254`

| Repo | Role | URL | Branch | Pinned commit |
|---|---|---|---|---|
| Engine-TS | **primary** — authoritative translation source | https://github.com/LostCityRS/Engine-TS | `254` | `2e3bcf4392200e84dd15ce67008c5d41fa4537aa` |
| Content | game content packed and served by the server | https://github.com/LostCityRS/Content | `254` | `caee3f2eb3eb3df60126e2be88c436dc2dc98e43` |
| Client-Java | the client this server speaks to; wire-protocol cross-check | https://github.com/LostCityRS/Client-Java | `254` | `2e629784c3dcb671ee3aab134f9cb91d614d8094` |
| rsbuf | renderer/info reference crate — `@2004scape/rsbuf` bumps `^244.1.0` → `254.1.0` at the 254 pin; `pkg/rsbuf` re-audited against it | https://github.com/2004scape/rsbuf | `254` | `304955d5cd6896dbcd76fb2bb17736ea426cae3e` |
| RuneScriptTS | RuneScript compiler — **returns at the advanced 254 pin** (`@lostcityrs/runescript ^0.9.6`, `ScriptProvider.COMPILER_VERSION = 27`); replaces the RuneScriptKt release-26 jar used at 244/245.2 | https://github.com/LostCityRS/RuneScriptTS | `main` | record the resolved 0.9.x commit when the pack-parity work pins it |
| cloudflare/zlib | gzip byte-parity reference | — | — | unchanged from §rev-244 (`886098f3`) |

(Engine pin ADVANCED 2026-06-10 from the original capture `43e02957` — see
the pin-advance note below. Content/Client-Java/rsbuf match the
goscape-client `REFERENCES.md` rev-254 pins; their upstream tips have not
moved. Go branch `rev-254` is cut from `rev-245.2` at `4b4c6106`.)

Notes:

- **Pin advance (2026-06-10, user decision):** the original capture pinned
  Engine-TS at the then-tip `43e02957`, but that engine cannot pack the
  pinned Content — Content `caee3f2e` uses a `midi` dbtable column type
  that Engine-TS only gained in `2dc4a811` (post-pin). The pin was
  advanced to the current `254` tip `2e3bcf43` (57 commits, 200 files,
  +4506/−3655 over `43e02957`) rather than cherry-picking. Wire prot
  tables and `ENGINE_REVISION = 254` are unchanged across the advance;
  the compiler swaps back to RuneScriptTS (above) and the
  `tools/pack/CompilerSymbols.ts` .sym export pipeline is deleted
  upstream.
- **Lineage:** the upstream `254` branch shares history with `245.2`
  (merge-base `cc487e8c`); the net cross-pin work list is
  `git -C Engine-TS diff 3c16994c..2e3bcf43` — plus the rsbuf crate diff
  `origin/244..origin/254` (2 commits; `origin/244` tip `1defefb1` IS the
  merge-base).
- **Toolchain: rsbuf bumps to `254.1.0`** — NPC ids widen to 14 bits on the
  wire (terminator 8191 → 16383, capacity 8192 → 16384, `NODE_MAX_NPCS`
  8191 → 16383). `@2004scape/rsmod-pathfinder` stays `^5.0.4`.
- **The zone-op table is renumbered at 254** alongside both prot tables;
  `pkg/rsbuf`'s zone-op fork moves with it (cross-table pin test).
- **jagFileVersion:** the §rev-225 note ("do not bump to 27 unless the
  upstream pins `@lostcityrs/runescript` past `750291c`") now triggers —
  the advanced 254 pin depends on `^0.9.6`. The pack-parity work must
  re-verify the version byte against the new reference cache.

## rev-274 — Go branch `rev-274`

| Repo | Role | URL | Branch | Pinned commit |
|---|---|---|---|---|
| Engine-TS | **primary** — authoritative translation source | https://github.com/LostCityRS/Engine-TS | `274` | `4c95f87efe00b068cadbd229d94736626907bd1a` |
| Content | game content packed and served by the server | https://github.com/LostCityRS/Content | `274` | `376072662e78a314bf35bb18815be39521491a6b` |
| Client-Java | the client this server speaks to; wire-protocol cross-check | https://github.com/LostCityRS/Client-Java | `274` | `32f30626156783de9f142306eb73a2243909dacf` |
| rsbuf | cross-check reference only; the crate dependency is DROPPED at 274 (ported into Engine-TS `src/network/rsbuf/`) | https://github.com/2004scape/rsbuf | `274` | `669116109588ab5f5d9de8c24aace1d335da5399` |
| RuneScriptTS | RuneScript compiler | — | — | unchanged from §rev-254 (`@lostcityrs/runescript` `0.9.6`) |

(Commits captured 2026-06-12 from the Server274-ref reference worktrees;
Engine-TS/Content re-pinned 2026-07-16 (see note 5). Go branch `rev-274` is
cut from `rev-254` at `d5e3234f`.)

Notes:

1. Go branch cut from `rev-254` at `d5e3234f`.
2. Work list = `git -C Engine-TS diff 2e3bcf43..dee467c8` (188 files,
   +13,792/−3,694, merge-base `93b6e557`).
3. **Toolchain: bun → Node 24** at the 274 pin — `node:zlib` 1.3.1 replaces
   bun/cloudflare-zlib `886098f3` as the gzip byte-parity baseline
   (resolution recorded by the Phase-3 gzip-baseline task of the rev-274
   plan).
4. **rsbuf crate + rsmod-pathfinder WASM dependencies DROPPED upstream** —
   both internalized into TS (`src/network/rsbuf/`,
   `src/engine/routefinder/`); the TS files at the Engine-TS pin are the
   authoritative reference from 274 on.
5. **Re-pinned 2026-07-16**: Engine-TS advanced `dee467c8` → `4c95f87e`
   (4 engine commits); Content advanced `7f97b0a5` → `37607266`
   (31 content commits). Superseding work list =
   `git -C Engine-TS diff dee467c8..4c95f87e`. Note 2's work list above
   remains the historical record of the original 274 cut.

## Future revisions

When porting revision *N*:

1. Add a `## rev-N` section below recording the reference commits used.
2. Branch the Go code `rev-N` from `rev-225` (or the nearest prior revision).
3. Diff the primary reference across the gap —
   `git -C Engine-TS diff e1dea19f..<rev-N commit>` — and apply the
   corresponding Go deltas on the `rev-N` branch, so the Go branch diff
   mirrors the TS revision diff.
4. Bump the **Content** and **compiler** pins in the same section (the
   compiler is revision-dependent: RuneScriptTS for 225, RuneScriptKt for
   244+) — the pack pipeline is byte-parity-checked against the cache the
   upstream packs, so engine, content, and compiler move together.
