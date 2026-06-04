# goscape

A Go rewrite of the [Lost City](https://github.com/LostCityRS) TypeScript
RuneScape server ([Engine-TS](https://github.com/LostCityRS/Engine-TS)),
compatible with the Lost City Java client.

This repository hosts **one Go port per game revision, one branch per
revision**. `main` carries no code — only the cross-revision documentation
below.

## Branch model

| Branch | Contents |
|---|---|
| `main` | This docs hub: cross-revision references and porting lessons |
| `rev-225` | The complete revision-225 server port (full history) |

Future revisions: branch `rev-N` from the nearest prior revision branch and
apply the upstream delta — see the "Future revisions" recipe in
[`REFERENCES.md`](REFERENCES.md) and the porting workflow in
[`PORTING-LESSONS.md`](PORTING-LESSONS.md).

## Files on this branch

- [`REFERENCES.md`](REFERENCES.md) — the upstream reference repos + commits
  each revision was ported from. Treat it like a lockfile.
- [`PORTING-LESSONS.md`](PORTING-LESSONS.md) — durable TS→Go porting
  knowledge that applies across revisions. Read it before starting a new
  revision.

Revision-specific docs (architecture, build/run, the deviation tracker) live
on each revision branch: see `README.md`, `CLAUDE.md`, `PORTING.md`, and
`docs/PORTING-CLOSED.md` on `rev-225`.
