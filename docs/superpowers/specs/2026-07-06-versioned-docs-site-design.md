# Versioned goscape documentation site — design

**Date:** 2026-07-06
**Status:** approved by user (brainstorming session)
**Target branch for implementation:** `main` (docs-only hub) — rev branches are read, never modified

## Problem

goscape has no user-facing documentation site. Material exists but is scattered:
cross-revision docs on `main` (README, REFERENCES.md, PORTING-LESSONS.md), and
per-revision material on the five rev branches (`examples/full-config-reference.yaml`,
`examples/bundled/`, `production/helm/goscape/`, `docs/RUNESCRIPT.md`,
`docs/PORTING*.md`). We want one documentation site covering all five game
revisions (225, 244, 245.2, 254, 274), with a version switcher so readers pick
their revision, rendered with Zensical (zensical.org), sourced from `main`.

## Decisions made (user-confirmed)

1. **Item icons**: goscape has no model→2D rasterizer (items icons are rendered
   from `.ob2` models by the client). Docs ship **without icons**; item tables
   reserve an icon column. A model rasterizer is a **separate follow-up project**.
2. **Hosting**: **local/offline only for now.** `mike` maintains the versioned
   built site in a local `gh-pages` branch; any hosting (GitHub Pages etc.) can
   be added later without rework. No CI.
3. **Generated content is committed to `main`** (not built on the fly). A
   generator script is run manually when a revision's pins change; site builds
   are pure Zensical with no external inputs.
4. **Scope additions** (all confirmed): Engine Contributor's Guide, extended
   cache-generated references (NPC/loc/music/place names/varp), Ops runbook
   inside the Admin Guide, and a Protocol Reference.

## Key facts discovered

- **Zensical versioning**: no native support yet; official interim path is the
  squidfunk **fork of `mike`** (`pip install git+https://github.com/squidfunk/mike.git`;
  not on PyPI). Each version deploys to a subdirectory on a `gh-pages`-style
  branch. Version selector: `[project.extra.version] provider = "mike"`
  (+ `default`, `alias = true`). "Outdated version" banner via a `custom_dir`
  `main.html` overriding the `outdated` block, linking `'../' ~ base_url`.
  `site_url` must be set (placeholder until hosting is chosen). Zensical reads
  `MIKE_DOCS_VERSION` during mike-driven builds and rewrites `site_url`.
- Zensical: config `zensical.toml` (TOML, keys under `[project]`), or
  `mkdocs.yml` compat. `zensical build --strict`, `zensical serve`. Python
  >= 3.10. **No hooks, no plugin API** — external assembly scripting is
  required and acceptable. Macros extension (`zensical.extensions.macros`)
  provides Jinja2 in Markdown; `docs_dir` cannot be `.`. Mermaid diagrams,
  content tabs, admonitions, client-side search all built in. `uv` symlink
  link-mode unsupported → use pip venv (or uv copy mode).
- **goscape-cli extraction**: `unpack config -cache-dir … -revision N` dumps 9
  config families as RuneScript-text files (`all.obj`, `all.npc`, `all.loc`,
  `all.seq`, `all.idk`, `all.flo`, `all.spotanim`, `all.varp`, `all.varbit`)
  under `<src-dir>/scripts/_unpack/<revision>/`. `unpack sprite-*` → real PNGs
  (UI/textures/title only — **no per-item icons**). Obj fields available:
  name, desc, cost, members, stackable, op1-5/iop1-5, cert links, models,
  2d transforms, etc. (`pkg/unpack/config/obj.go`).
- **Commands** come from two places: the Go cheat ladder
  (`modules/world/handlers_game.go` `handleClientCheat` switch, ~35 commands,
  gated `staffModLevel>=4` + non-production; plus `player_inv_cheat.go`) and
  RuneScript `[debugproc,…]` scripts in the external Content repo (prefix
  `~` by default, `world.node-debugproc-char`).
- **Per-revision inputs exist locally**: `~/Code/github.com/LostCityRS/`
  `Server{244,245.2,254,274}-ref/` each have `content/` + a client cache at
  `unpack-ref/cache/main_file_cache.*`; rev-225 content at `Server225_2/content`
  and cache at `Server2/engine/data/pack/` (Server2 and Server225_2 share the
  same tip commit — same rev-225 lineage). Implementation must verify the 225
  cache matches the pins (e.g. spot-check via `unpack checksum`); fallback is
  self-packing from pinned content with the rev-225 branch's `make pack`.
- `main` today: 5 files only (README with a **stale branch table**, REFERENCES.md,
  PORTING-LESSONS.md, LICENSE, .gitignore). No site scaffolding anywhere.

## Architecture (Approach A — overlay assembly, chosen over
single-tree-macros-only and docs-on-rev-branches)

### Repository layout on `main`

```
zensical.toml.tmpl        # site config template (revision vars filled at build)
docs/                     # shared prose — written once, applies to all revisions
  index.md                # landing page
  admin/…                 # Administrator's Guide
  player/…                # Player's Guide
  runescript/…            # RuneScript Developer's Guide
  contributor/…           # Engine Contributor's Guide
  protocol/…              # Protocol Reference
overlays/rev-225/…        # per-revision pages, same tree shape as docs/
overlays/rev-244/…        #   (a file here replaces/adds to the shared tree):
overlays/rev-245.2/…      #   generated references, RUNESCRIPT.md snapshot,
overlays/rev-254/…        #   config-reference snapshot, any diverged page
overlays/rev-274/…
tools/docsgen/            # generator (Python) — cache/content/Go source → overlays
  revisions.toml          # per-revision: branch name, content dir, cache dir, pins
tools/build.py            # assembler + mike driver
requirements.txt          # zensical + mike fork (git URL)
README.md                 # updated: fix stale branch table, point at docs site
REFERENCES.md             # stays canonical; embedded into contributor guide at assembly
PORTING-LESSONS.md        # same
.gitignore                # + .build/, site/, venv
```

`main` remains free of server code; `tools/` is documentation tooling only.
Built versions live in a local `gh-pages` branch managed by `mike`.

### Build pipeline (`tools/build.py`)

Per revision:
1. Assemble `docs/` + `overlays/rev-N/` into `.build/rev-N/docs/`
   (overlay file wins on path collision).
2. Render `zensical.toml` from the template: `[project.extra] revision = "N"`
   plus pin metadata (macros expose these to pages as `{{ revision }}` etc.).
3. `mike deploy rev-N` (local `gh-pages`, never `--push`).

After all revisions: `mike deploy rev-274 latest --update-aliases`,
`mike set-default latest`. Preview: `mike serve` (full versioned site),
`zensical serve -f .build/rev-N/zensical.toml` (fast single-revision authoring).

Per-revision prose variance: small → Jinja conditionals in shared pages via the
macros extension; structural → full-page overlay copy.

### Generator (`tools/docsgen`)

Run manually when pins change. Output committed, **deterministic** (stable
sorts, no timestamps) so re-runs diff clean. Per revision:

1. Build that rev branch's `goscape-cli` in a temp git worktree
   (`GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache`, `CGO_ENABLED=0`); run
   `unpack config` against the pinned reference cache.
2. Parse `all.obj|npc|loc|varp|varbit|seq` → markdown reference pages.
   Item/NPC tables chunked (~500 rows/page) + index page. Item rows keep a
   reserved (empty) icon column for the future rasterizer.
3. Commands page: parse Go cheat-switch `case "…"` arms + inv cheats from the
   rev branch; grep pinned content `.rs2` for `[debugproc,…]`. Group
   player/mod/admin with staff-level and non-production gating notes.
4. Extras: music track list (versionlist/content midi names), worldmap
   place-name index (content `maps/labels.txt`).
5. Snapshots into overlays: `git show rev-N:docs/RUNESCRIPT.md` (language
   reference base), `git show rev-N:examples/full-config-reference.yaml`
   (embedded in config-reference page), per-rev Helm values examples.
6. Sanity floors: abort if e.g. item count < 1000, command count < 20.

## Content plan (page inventory)

**Administrator's Guide** — Overview & architecture (Mermaid module-dependency
graph, dskit service lifecycle); Quick start (bundled example); Configuration
(layered precedence defaults→file→env→flags, strict-decode warning, full
annotated reference from snapshot); Deployment scenarios with diagrams:
(a) single binary + sqlite, (b) split targets one host, (c) multi-host central
management (login+friends+postgres) + N world hosts, (d) Kubernetes/Helm —
three deploymentModes (SingleBinary/Management/World), (e) container images;
Ops runbook: backups (sqlite file / pg_dump), RSA key gen & rotation
(`goscape-cli rsa`), health endpoints, logging (levels, per-module, format),
upgrade/reboot procedures (`::reboot`, `::slowreboot`), cache packing
(`make pack`).

**Player's Guide** — Getting started (connecting with Client-Java, account
creation); Commands (generated); Item reference (generated); NPC / location /
music / place-name references (generated); revision-differences page (count
deltas derived from generated data).

**RuneScript Developer's Guide** — Introduction; Writing scripts (file layout,
triggers, types, variables, control flow — adapted per revision from the
RUNESCRIPT.md snapshot); Worked examples; Toolchain (`goscape-cli compile
-check`, `pack`, `smoke-pack`); Deploying scripts (pack → `::reload` on dev,
restart in production, byte-parity expectations); varp/varbit reference
(generated).

**Engine Contributor's Guide** — Branch model & upstream pins (wraps
REFERENCES.md); Porting methodology (wraps PORTING-LESSONS.md); Dev environment
setup (Go 1.26+, CGO_ENABLED=0, race detector, test commands, content
symlinks); Codebase architecture (dskit services, module system, binary I/O);
Porting-a-new-revision recipe.

**Protocol Reference** — Login handshake (Mermaid sequence diagram), ISAAC
cipher setup, RS2 packet framing (fixed/-1/-2 sizes), OnDemand HTTP protocol,
RSA login block. Written from `pkg/io/*` on rev-274 with per-revision notes
where the wire changed (e.g. NPC ids widen to 14 bits at rev-254).

## Verification

- `zensical build --strict` for every revision (broken links/nav fail).
- docsgen determinism: immediate re-run produces zero git diff.
- docsgen sanity floors (counts) abort on suspicious output.
- Manual `mike serve` smoke: version switcher across all five revisions,
  outdated banner on non-latest.

## Out of scope

- Item-icon model rasterizer (follow-up project; icon column reserved).
- Remote hosting, CI, publishing (`--push` never used yet).
- Docs for future revisions (the recipe is documented instead).
- Any change to rev branches (read-only inputs).

## Phasing note for the implementation plan

Large writing volume — phase it: (1) skeleton + build pipeline + versioning
working end-to-end with stub pages; (2) docsgen + generated references;
(3) Admin Guide; (4) RuneScript Developer's Guide + Player's Guide prose;
(5) Contributor Guide + Protocol Reference; (6) polish (landing page,
revision-differences page, README update on main).
