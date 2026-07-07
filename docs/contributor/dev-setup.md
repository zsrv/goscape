# Dev environment

This page covers two workflows: building and testing the **engine** (a `rev-N`
branch), and building this **documentation site** (the `docs-site` branch on the
docs worktree). They are independent — pick whichever you are working on.

## Engine: toolchain

The engine requires **Go 1.26 or newer** (`go.mod` declares `go 1.26`; the
release images pin `1.26.3`). No other toolchain is needed to build or test the
server — the binary is statically linkable and its dependencies are pure Go.

Builds set `CGO_ENABLED=0` so the result is a static binary with no libc
dependency:

```bash
# Run the server against the bundled example config
CGO_ENABLED=0 go run -trimpath ./cmd/goscape --config.file examples/bundled/goscape.yaml

# Build the server binary
CGO_ENABLED=0 go build -trimpath -o goscape ./cmd/goscape

# Build both binaries (goscape + goscape-cli) via the Makefile
make all
```

`make all` produces two executables: `goscape` (the server daemon) and
`goscape-cli` (the offline cache/RuneScript tooling — `pack`, `unpack`,
`compile`, `rsa`, `worldmap`, `jag`).

## Engine: tests

```bash
# Run the whole suite
go test ./...

# Run a single package / a single test
go test ./pkg/io/packet/... -run TestName

# Run the race detector on touched packages
go test -race ./...
```

The suite is green out of the box. Two things are worth knowing:

- **The race detector needs cgo.** `go test -race` requires `CGO_ENABLED=1`
  (the default), so it links against a C toolchain. The world tick loop is
  goroutine-heavy, so race coverage on packages you touch there is worth the
  cost.
- **Some tests need the packed cache.** Tests that exercise the world server's
  reload pipeline require `data/pack/` (produced by `goscape-cli pack`, below)
  and skip automatically when it is absent.

The full verification gate that porting work is held to —
`CGO_ENABLED=0 go build -trimpath ./...`, `go vet ./...`, `go test ./...`, and
`go test -race` on touched packages — is documented in the
[Porting lessons](porting-lessons.md) (§5, "Verification").

## Engine: packing the game cache

The server reads a **packed cache** (configs, compiled RuneScript, and maps)
that `goscape-cli` builds from a Content source tree. `make pack` runs the packer:

```bash
make pack   # packs CACHE_SRC_DIR (data/src) → CACHE_OUT_DIR (data/pack)
```

The packer reads source content from `data/src` by default (`CACHE_SRC_DIR`),
stages intermediate output under `data/raw` (`CACHE_RAW_DIR`), and writes the
finished cache to `data/pack` (`CACHE_OUT_DIR`). goscape does **not** vendor the
Content tree, so before your first `make pack` you must point `data/src` at a
checkout of the Content repo pinned for your revision — the simplest way is a
symlink:

```bash
# from the repo root; use the Content checkout pinned for your revision
[ -e data/src ] || ln -s /path/to/Content data/src
```

The exact Content commit each revision is pinned to is on the
[References & pins](references-pins.md) page. `data/pack/` is what the OnDemand
module serves to clients and what the world module reads maps from, so the same
`make pack` output feeds both a running server and the cache-dependent tests.

## Docs site: toolchain

The documentation site is a separate build that lives on the `docs-site` branch.
It assembles a per-revision docs tree, renders it with
[zensical](https://github.com/squidfunk/zensical), and publishes each revision as
a versioned site with [mike](https://github.com/squidfunk/mike). Everything runs
out of a Python virtual environment:

```bash
# from the docs worktree root, one time
python -m venv .venv
.venv/bin/pip install -r requirements.txt   # zensical, mike, pytest

# put the venv on PATH for the rest of your shell session
export PATH="$PWD/.venv/bin:$PATH"
```

### Regenerating the generated pages (when a pin changes)

The per-revision **overlay** pages (item/NPC/location/varp lists, the commands
reference, music, places, and the revision-comparison table) are generated from
each revision's Content tree and packed cache by `tools/docsgen`. You only need
to re-run it when those inputs change — for example when a Content pin moves or a
new revision is added:

```bash
python -m tools.docsgen --revision 274      # one revision
python -m tools.docsgen --revision all      # every revision + the comparison table
```

`docsgen` reads its per-revision inputs (branch, content directory, cache
directory) from `tools/docsgen/revisions.toml`. Its unpack/config/command steps
need the referenced Content and cache checkouts on disk, so a docs-only
contributor without those reference trees typically edits the hand-written pages
(under `docs/`) and leaves the generated overlays alone.

### Building and previewing the site

The build is a two-step loop: `tools/build.py assemble` stages a self-contained
docs tree for one revision (copying `docs/`, applying that revision's overlay,
injecting `REFERENCES.md`/`PORTING-LESSONS.md` as the pins and lessons pages, and
rendering `mkdocs.yml` from the template), and then zensical renders it:

```bash
python tools/build.py assemble --revision 274
zensical build --strict -f .build/rev-274/mkdocs.yml
```

`--strict` turns warnings (such as broken internal links) into errors, so a
clean strict build is the bar a docs change has to clear.

To build and publish **every** revision as a mike version — and set the newest
one as the default `latest` alias — use the `all` subcommand, then preview the
versioned set:

```bash
python tools/build.py all   # assemble + mike deploy every revision, set default
mike serve                  # preview the deployed versions locally
```

The revision set and which one is `latest` are defined by the `order` and
`latest` keys in `tools/docsgen/revisions.toml`; adding a revision there is the
first step of [Porting a new revision](porting.md)'s documentation task.
