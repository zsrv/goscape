# Contributor's Guide

goscape is a Go rewrite of the [Lost City](https://github.com/LostCityRS)
TypeScript RuneScape server ([Engine-TS](https://github.com/LostCityRS/Engine-TS)),
built to speak to the Lost City Java client. This guide is for people who want to
change the engine itself: fix a parity bug, port a new game revision, or work on
this documentation site. It assumes you have read the operator-facing
[Administrator's Guide](../admin/index.md) for the runtime picture (modules,
targets, the service lifecycle) and does not repeat it.

## Philosophy: faithful translation is the default

goscape is not a reimagining of the Lost City server — it is a **1:1 translation**
of Engine-TS into Go. That single rule drives almost every convention in the
codebase:

- **Every ported game-logic region maps to an identifiable Engine-TS function**,
  cited inline in the Go source as `// TS <File>.ts:<lines>`. The TS file and
  symbol are the durable anchor; a bug is found by diff-checking the Go region
  against the cited TS source.
- **Infrastructure is adapted to Go idiom; game logic is not.** goscape rebuilds
  the service lifecycle, module system, and config layering in idiomatic Go
  (see [Architecture](architecture.md)), but inside ported game logic it does
  **not** refactor opportunistically — a "cleaner" rewrite is how behaviour
  bugs get introduced.
- **The boundaries are byte-faithful.** The wire protocol (ISAAC, RSA, opcodes),
  the codecs (bzip2, CRC, wordenc), collision, and the pack pipeline's cache
  output are byte-for-byte identical to the reference and pinned by tests.

These principles are stated in full, with the bug classes that motivate them, in
the [Porting lessons](porting-lessons.md) page (§1, "Philosophy").

### The fidelity gate: deviations are tracked, never silent

Because faithful translation is the default, **every behavioural divergence from
Engine-TS needs a justification, and that justification is recorded** — a
divergence is never left implicit. Each revision branch carries two tracker
files: `PORTING.md` (the active backlog of known gaps) and
`docs/PORTING-CLOSED.md` (closed rows, parity tables, and audit history). An
accepted in-code divergence carries a `PORTING-EXCEPTION (<row-id>, <short>)`
marker next to it, so `grep -rn "PORTING-EXCEPTION" modules pkg cmd internal`
lists every exception the port has agreed to live with. A comment that merely
*claims* a divergence is intentional is treated as the prime suspect, not as
evidence — see the [Porting lessons](porting-lessons.md) for why "by design"
comments are re-verified against the TS source before they are trusted.

## What lives where: `main` versus the revision branches

This repository holds **one Go port per game revision, one branch per
revision**. The split matters for anyone browsing the source:

- **`main` carries no engine code.** It is the cross-revision documentation
  hub — the `REFERENCES.md` lockfile (published here as
  [References & pins](references-pins.md)) and the durable `PORTING-LESSONS.md`
  (published as [Porting lessons](porting-lessons.md)). This versioned
  documentation site is built from the same cross-revision sources, which it
  injects verbatim so the pins and lessons never drift from their canonical
  files.
- **Each `rev-N` branch is one complete, self-contained port** of that game
  revision — its own engine code, tooling, and tests. Revision branches are not
  meant to share code packages; each is an independent faithful translation.
  Revision-specific docs (the branch's own `README.md`, `CLAUDE.md`,
  `PORTING.md`, and `docs/PORTING-CLOSED.md`) live on the branch, not on `main`.

The [Branch model](branches.md) page explains the lineage of those branches and
the "the pin is the commit hash" doctrine that ties each one to an exact upstream
commit.

## Where to start reading

| If you want to… | Start here |
|---|---|
| Understand how the branches relate and what each one is pinned to | [Branch model](branches.md) → [References & pins](references-pins.md) |
| Get a working build, run the tests, or preview these docs | [Dev environment](dev-setup.md) |
| Modify engine code (services, modules, packet I/O, the login/ISAAC path) | [Architecture](architecture.md) |
| Internalize the TS→Go pitfalls before touching ported logic | [Porting lessons](porting-lessons.md) |
| Port a whole new game revision end to end | [Porting a new revision](porting.md) |

If you are chasing a wire-level detail, the [Protocol](../protocol/index.md)
guide documents the login handshake, packet framing, ISAAC, and OnDemand from
the client's point of view.
