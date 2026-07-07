# Deploying scripts

A script only affects the game once it has been **compiled into the cache** and
the running world has **loaded that cache**. Source `.rs2` files are never read
by the server directly. This page covers the two paths from an edit to live
behaviour: the fast in-development loop, which hot-reloads a freshly packed cache
without restarting; and the production procedure, which repacks, verifies
byte-parity, and restarts. It builds on the [Toolchain](toolchain.md) page (the
`compile`, `pack`, and `smoke-pack` verbs) and the
[Operations runbook](../admin/operations.md#cache-packing) (the operational side
of packing and restarts).

## The development loop

On a development world the round trip is four steps:

1. **Edit** a `.rs2` file under `data/src/scripts`.
2. **Check** it with `goscape-cli compile -check` for fast diagnostics — a
   couple of seconds, no cache written
   ([Toolchain](toolchain.md#compile-check-the-script-tree),
   [Writing scripts](writing-scripts.md#compile-checking-it)).
3. **Pack** the cache with `make pack` — this is what compiles the scripts into
   `data/pack/server/script.dat` and rebuilds the rest of the cache
   ([Toolchain](toolchain.md#pack-build-the-cache)).
4. **Reload** in-game with the `::reload` admin command, which reloads the packed
   cache into the running world with no restart.

Step 2 is optional but cheap insurance: `compile -check` reports an unresolved
command or a type error in seconds, whereas discovering it during `make pack`
means waiting for the whole pipeline. Step 3 is not optional — `::reload` reads
the *packed cache*, never your source.

!!! warning "`::reload` loads the packed cache, not your source"
    `::reload` re-reads the cache at `world.cache_path` (default `./data/pack`).
    It does **not** compile: if you edit a script and reload without running
    `make pack` first, the world reloads the *old* compiled bytecode and your
    change does not appear. Always pack, then reload.

### What `::reload` reloads

`::reload` runs the world's reload routine on the tick goroutine — the tick
blocks while it works, so it is a brief pause, not a background job. Reading from
the packed cache, it reloads:

- **Every type/config registry** — varp, varbit, param, obj, loc, npc, idk, seq
  (with its animation frames), spotanim, category, enum, struct, inv, mesanim,
  dbtable and dbrow (and their index), hunt, varn, vars, and component types.
- **Inventories** — shared inventories (`scope=shared`) are rebuilt from their
  type; each player's temporary inventories (`scope=temp`) are cleared.
  Persistent inventories (`scope=perm`, written to save files) are left
  untouched.
- **Shared-variable storage** — the `varshared` backing store is resized if the
  varshared type count changed.
- **The compiled scripts** — reloaded from `<cache_path>/server`. On a debug
  world this broadcasts `Loaded N scripts.` (or an error message) so you can see
  it took.
- **The CRC tables** — regenerated so the values the world advertises to clients
  match the new cache.
- **The live map's type tables** — the fresh loc and obj type registries are
  re-injected into the running map, along with the members flag.

What `::reload` does **not** reload is the **map geometry itself**: the terrain,
zones, and placed-loc/ground-object data loaded at startup are not re-read. Only
the loc/obj *type* tables the map references are re-injected. A change to where
things are placed on the map — as opposed to what a loc or obj *is* — therefore
needs a server restart, not a reload.

!!! warning "`::reload` is developer-only: staff level 4 and non-production"
    `::reload` lives in the engine's developer command block, gated on
    **`world.node_production` being `false` and the caller's staff mod level
    being 4 or higher**. On a production world (`node_production: true`) the
    entire developer block is skipped, so `::reload` does nothing — production
    content updates go through the [restart procedure](#production) below. If a
    reload hits an error partway through, some registries may already be swapped
    and others not (the engine does not roll back); it logs the failure and
    tells you `Reload failed: see server log.`

## Production

A production world (`world.node_production: true`) does not hot-reload — the
developer `::reload` command is unavailable there. Content is updated by
repacking, verifying, and restarting:

1. **Repack from source** with the `goscape-cli` built from the **same
   revision** as the running server (`make pack`). The cache layout is coupled to
   the engine, so a mismatched tool can pack a cache the server reads
   incorrectly. See
   [Packing the cache](../admin/operations.md#packing-the-cache).
2. **Verify** the new cache with
   [`goscape-cli smoke-pack -reference-dir <ref>`](toolchain.md#byte-diffing-against-a-reference),
   pointing `-reference-dir` at the upstream reference cache. A run reporting
   `0` diffs across every stage is your byte-parity proof that the cache matches
   the reference the revision was built against.
3. **Restart the world** so it loads the new cache — it reads `world.cache_path`
   at boot. A clean OS-signal stop first runs a final save-all for every online
   player. See
   [Restarts & upgrades](../admin/operations.md#restarts-upgrades) for the full
   procedure, including config verification and database backup before a
   cross-revision upgrade.

!!! note "`::reboot` stops the whole process"
    The in-game `::reboot` / `::slowreboot` commands (staff level 3+, and only
    when `node_production` is `true`) schedule a graceful world shutdown that
    drains players with a save. When the world empties, that shutdown brings the
    **entire process** down cleanly — OnDemand, login, and friends stop alongside
    the world (exit status `0`), matching upstream Engine-TS. So `::reboot` is a
    valid way to trigger a binary swap: run the server under a process supervisor
    and it relaunches on the new binary once the reboot completes. A plain
    `SIGTERM` (also a clean save-all stop) remains the way to restart without the
    reboot semantic. The
    [In-game reboot commands](../admin/operations.md#in-game-reboot-commands)
    section of the Operations runbook covers this in full.

## Byte parity

The pack pipeline is not "close enough" — it is engineered to reproduce the
upstream cache **byte-for-byte**. From the project's porting lessons:

> **Byte-faithful at the boundaries.** […] the pack pipeline's cache output
> [is] byte-for-byte against the reference. The pack pipeline is verified by
> byte-diffing `script.dat` (and friends) against the cache the upstream TS
> meta-repo packs.

That is why [`smoke-pack -reference-dir`](toolchain.md#byte-diffing-against-a-reference)
exists and why a non-zero diff is treated as a bug in the port, not a build
choice you can shrug off. Two consequences worth knowing when you touch the
pipeline:

- **Determinism comes before diffing.** Go map iteration order is randomized, so
  any output derived from a map must impose an explicit order; the porting notes
  record a case where a single map-iteration-order bug masked four separate
  parity bugs until the output was made stable.
- **The cache version byte is pinned to the reference.** goscape writes the
  script-archive version byte the reference compiler for the revision emits (26
  through rev-245.2, 27 from rev-254 on — see the
  [Toolchain compiler table](toolchain.md#the-runescript-compiler)); the server
  rejects a cache whose version does not match.

For the durable version of this philosophy and the full set of pipeline gotchas,
see the [Porting lessons](../contributor/porting-lessons.md).

## Where to go next

- **[Toolchain](toolchain.md)** — the `compile`, `pack`, and `smoke-pack` verbs
  in detail.
- **[Operations runbook](../admin/operations.md#restarts-upgrades)** — restarts,
  upgrades, and cache packing from the operator's side.
- **[Writing scripts](writing-scripts.md)** — authoring and compile-checking a
  script.
