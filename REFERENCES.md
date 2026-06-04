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

## Future revisions

When porting revision *N*:

1. Add a `## rev-N` section below recording the reference commits used.
2. Branch the Go code `rev-N` from `rev-225` (or the nearest prior revision).
3. Diff the primary reference across the gap —
   `git -C Engine-TS diff e1dea19f..<rev-N commit>` — and apply the
   corresponding Go deltas on the `rev-N` branch, so the Go branch diff
   mirrors the TS revision diff.
4. Bump the **Content** and **RuneScriptTS** pins in the same section — the
   pack pipeline is byte-parity-checked against the cache the upstream
   meta-repo packs, so engine, content, and compiler move together.
