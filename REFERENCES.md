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
`rev-225` at `bf073fcc`. Engine (Java) and Server — pinned at `main` for
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
