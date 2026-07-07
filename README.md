# goscape

A Go rewrite of the [Lost City](https://github.com/LostCityRS) TypeScript
RuneScape server ([Engine-TS](https://github.com/LostCityRS/Engine-TS)),
compatible with the Lost City Java client.

This repository hosts **one Go port per game revision, one branch per
revision**. `main` carries no engine code — only the cross-revision
documentation below and the tooling for the documentation site.

## Branch model

Every `rev-N` branch is a **complete, self-contained port** of that game
revision. The branches form a lineage: each is cut from the nearest prior
revision branch and carries the upstream delta forward.

| Branch | Contents |
|---|---|
| `main` | This docs hub: cross-revision references, porting lessons, and the docs-site tooling |
| `rev-225` | Complete revision-225 port — the original branch (RuneScriptTS compiler); every later branch descends from it |
| `rev-244` | Complete revision-244 port, cut from `rev-225` — swaps in the RuneScriptKt compiler jar; `@2004scape/rsbuf` bumps to `244.1.0` |
| `rev-245.2` | Complete revision-245.2 port, cut from `rev-244` — reverts two 244-only commits; no toolchain change |
| `rev-254` | Complete revision-254 port, cut from `rev-245.2` — RuneScriptTS returns (`0.9.6`); `@2004scape/rsbuf` `254.1.0` widens NPC ids to 14 bits |
| `rev-274` | Complete revision-274 port, cut from `rev-254` — rsbuf and rsmod-pathfinder are internalised into Engine-TS; the newest revision |

To port a further revision, branch `rev-N` from the nearest prior revision
branch and apply the upstream delta — see the "Future revisions" recipe in
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
`docs/PORTING-CLOSED.md` on any `rev-N` branch.

## Documentation site

A versioned documentation site is built from this branch. It publishes one
documentation set per game revision — all five, `rev-225` through `rev-274`
(the newest, served as `latest`) — behind a single **version switcher**. The
cross-revision files above (`REFERENCES.md`, `PORTING-LESSONS.md`) are injected
into it verbatim, and each revision's item/NPC/location/command tables are
generated from that revision's own content tree and packed cache.

Build and preview it locally with Python:

```bash
# one-time: create the virtualenv and install the toolchain (zensical, mike, pytest)
python -m venv .venv && .venv/bin/pip install -r requirements.txt
export PATH="$PWD/.venv/bin:$PATH"

# regenerate the per-revision generated pages — only when a pin changes.
# Needs each revision's content/cache checkouts, wired in tools/docsgen/revisions.toml.
python -m tools.docsgen

# assemble + publish every revision as a mike version, newest set as `latest`
python tools/build.py all

# preview the versioned site in a browser
mike serve
```

The full contributor walkthrough — engine build/test alongside this docs
workflow — is the site's own Contributor's Guide
(`docs/contributor/dev-setup.md`).
