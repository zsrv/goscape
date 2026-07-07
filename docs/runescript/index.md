# RuneScript

RuneScript is the domain-specific language that LostCity content authors use to
drive everything a player touches: NPC dialogue, item and scenery interactions,
quests, combat AI, interface logic, and timed behaviour. Scripts are written as
`.rs2` source, compiled to a compact stack-based bytecode, and executed by the
engine's script runner. goscape runs the *same* scripts the reference server
does — it ports the compiler and the script runtime from
[`LostCityRS/Engine-TS`](https://github.com/LostCityRS/Engine-TS) so that
content authored for Engine-TS behaves identically when executed by goscape.

This page is the map. It explains what a script is, where scripts live, and how
goscape turns them into behaviour. For the authoritative language spec, follow
the [Language reference](language.md); this section's other pages —
[Writing scripts](writing-scripts.md) and [Examples](examples.md) — are the
tutorial layer that leads you into it.

## Where scripts live

Scripts and the config they reference live in a **content repository**
([`LostCityRS/Content`](https://github.com/LostCityRS/Content)), kept separate
from the engine. A checkout is organised roughly like this:

- `scripts/` — the `.rs2` source tree. A single `.rs2` file holds one or more
  scripts; folders group them by area, skill, quest, or subsystem.
- `scripts/**/*.constant` — compile-time constants written `^name value`, which
  the compiler inlines wherever `^name` appears.
- `pack/` — text config files (`obj.pack`, `loc.pack`, `npc.pack`,
  `varp.pack`, …) that define the objects, locations, NPCs, and variables a
  script refers to by name rather than by raw id.

goscape reads this tree through its `data/src` path (so `data/src/scripts` and
`data/src/pack`) and compiles it into the game cache the client downloads. The
[Toolchain](toolchain.md) page covers the compile and pack commands, and
[Deploying scripts](deploying.md) covers getting the result onto a running
server.

## The trigger model

Every script begins with a bracketed **trigger header** of the form
`[trigger,subject]`. The *trigger* names an event — a player clicking an
inventory item (`opheld1`), an NPC option (`opnpc1`), a scenery option
(`oploc1`), a login, a queued action, a timer tick — and the *subject* names
the entity, interface component, or queue slot the script binds to. When that
event fires for that subject, the engine runs the script. There is no `main()`:
content is a flat collection of event handlers, and the engine decides which
one to invoke.

Triggers also seed the script's **active pointers**. `[opnpc1,man]` arrives
with an active player *and* an active npc already set, and most commands act
implicitly on those pointers — `chatnpc` speaks as the active npc, `stat` reads
the active player. Scripts share logic by calling typed procedures with
`~name(args)` (which return values) or by tail-jumping to labels with
`@name(args)` (which never return). The [Language reference](language.md)
covers triggers, pointers, procedures, and labels in full.

## How goscape runs scripts

The workflow is compile-then-run. `goscape-cli compile` type-checks the `.rs2`
tree against the entity config, lowers each script to bytecode, and writes the
result into the cache; the server then loads that cache and its script runner
executes the bytecode a tick at a time. Because goscape's compiler and runtime
are ports of Engine-TS, the bytecode and its semantics match — the same source
produces the same in-game behaviour. When goscape *has* to deviate from
Engine-TS, the divergence is documented; the porting notes in
[the language reference](language.md#10-engine-side-notes-goscape-porting)
call out the places where RuneScript semantics constrain the engine.

## Where to go next

- **[Language reference](language.md)** — the authoritative spec: every trigger
  family, variable scope, type, control-flow form, and command family.
- **[Writing scripts](writing-scripts.md)** — file layout, the anatomy of a
  script, and a compile-checked "hello world".
- **[Examples](examples.md)** — real, annotated scripts lifted from the content
  repository.
- **[Toolchain](toolchain.md)** — the `goscape-cli` compile and pack commands.
- **[Deploying scripts](deploying.md)** — getting compiled content onto a
  server.
- **[Varps & varbits](varps.md)** — the per-player variable catalogue that
  `%`-scoped variables read and write.
