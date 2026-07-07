# Writing scripts

This page walks through the shape of a RuneScript file and ends with a
compile-checked first script. It is deliberately a tour, not a specification:
wherever a topic has depth, it links into the [Language reference](language.md)
rather than restating it. If you already know the model and just want working
code, jump to [Examples](examples.md).

## File layout & naming

Scripts are plain-text `.rs2` files under the content repository's `scripts/`
tree (goscape reads it as `data/src/scripts`). A single file may contain one
script or many; the directory structure is organisational only — the compiler
walks the whole tree, so a script in one file can call a proc defined in
another. Alongside the source live two other content inputs the compiler needs:

- **`.constant` files** under `scripts/` declare `^name value` constants that
  are inlined at compile time — see
  [§2 of the reference](language.md#2-variables).
- **`.pack` files** under `pack/` (`obj.pack`, `loc.pack`, `npc.pack`,
  `varp.pack`, …) define the entities scripts name. When you write `oak_tree`
  or `bronze_dagger`, the compiler resolves it against these files and fails if
  the symbol does not exist.

## Anatomy of a script

A script is a trigger header followed by an optional parameter/return
signature, then a body of statements:

```rs2
[opheld1,bronze_dagger]
def_int $count = inv_total(inv, bronze_dagger);
mes("You inspect the dagger.");
mes("You are carrying <tostring($count)>.");
```

The pieces:

- **Trigger header** `[opheld1,bronze_dagger]` — the event (`opheld1`, the
  first click option on a held item) bound to a subject (`bronze_dagger`).
- **Locals** — `def_int $count = …` declares a typed, script-local variable.
  Locals live for exactly one execution.
- **Commands** — `mes(…)`, `inv_total(…)` are engine commands. Statements end
  with `;`.

Procedures and labels add a signature after the header — `[proc,greet](string
$who)(string)` takes a `string` and returns a `string`. See
[§5, Procedures, labels, and the call stack](language.md#5-procedures-labels-and-the-call-stack).

## Trigger families

The header's trigger word selects one of several families. This is the
condensed map; the full table with exact opcodes is
[§1 of the reference](language.md#1-file-layout-triggers).

| Family | Representative triggers | Fires when… |
| --- | --- | --- |
| Inventory item ops | `opheld1`..`opheld5`, `opheldu`, `opheldt` | a player clicks an option on an inventory item |
| Loc (scenery) ops | `oploc1`..`oploc5`, `aploc1`..`aploc5` | a player clicks a scenery option (`ap*` = approach / out-of-range) |
| NPC ops | `opnpc1`..`opnpc5`, `apnpc*` | a player clicks an NPC option |
| Ground-obj ops | `opobj1`..`opobj5` | a player clicks a ground item |
| Player ops | `opplayer1`..`opplayer5` | a player right-clicks another player |
| NPC AI | `ai_queue1`..`ai_queue20`, `ai_timer`, `ai_spawn` | an NPC's behaviour ticks |
| Queues / timers | `queue,name`, `timer,name` | the engine drains that queue / timer |
| Lifecycle | `login`, `logout` | a per-player session event |
| Interface | `if_button,iface:comp`, `inv_button1`..`inv_button5` | a UI interaction on an open interface |
| Code-only | `proc,name`, `label,name`, `debugproc,name` | called from another script |

## Variables & types in brief

RuneScript has four variable scopes, each marked by a sigil. This is the short
version of [§2](language.md#2-variables):

| Sigil | Scope | Lifetime |
| --- | --- | --- |
| `$x` | script-local | one execution |
| `%x` | player varp / vartemp / varbit | per-player (varps persist; vartemp wiped on logout) |
| `^x` | constant | compile-time, inlined |
| `~x` | procedure call | — (not a variable) |

The `%`-scoped variables are catalogued on the [Varps & varbits](varps.md)
page. Locals are typed at declaration:

```rs2
def_int $count = 0;
def_string $name = "Bob";
def_coord $home = 0_50_50_25_25;
```

The primitive types are `int`, `string`, `boolean` (an int 0/1), and `coord`
(a packed `level_mx_mz_lx_lz` literal). On top of those, cache-typed references
such as `obj`, `loc`, `npc`, `seq`, and `inv` are first-class — the compiler
verifies each name exists in its config. The full list is in
[§3, Types](language.md#3-types).

One rule catches every newcomer: **arithmetic only happens inside `calc(…)`**.
Outside a `calc`, the right-hand side of a statement may only be an assignment
or a call, so `$total = calc($base + $bonus * 2);` is written with the
`calc(…)` wrapper. Conditionals (`if` / `else if` / `else`), `while` loops, and
typed `switch_int` / `switch_obj` jump tables are covered in
[§4, Expressions & control flow](language.md#4-expressions-control-flow).

## Queues & timers

Most "later" behaviour — a delayed hit, a repeating environmental effect, a
persistent quest beat — is scheduled rather than run inline. There are two
mechanisms, both fully described in
[§7, Queues, timers, and engine-driven scheduling](language.md#7-queues-timers-and-engine-driven-scheduling).

**Queues** run a `[queue,name]` script after a tick delay. Four flavours differ
only in whether the pending action survives logout and death:

| Command | Survives logout? | Survives death? | Use case |
| --- | --- | --- | --- |
| `queue` | no | no | ordinary tick-delayed action |
| `weakqueue` | no | no | cancellable scenery animation |
| `longqueue` | yes | yes | persistent quest beat |
| `strongqueue` | no | yes | combat hits, must-fire effects |

**Timers** repeat. `settimer(timer_id, interval)` registers a `[timer,name]`
script to fire every `interval` ticks until `cleartimer(timer_id)` cancels it.
The [Examples](examples.md) page includes a timer that starts on zone entry and
clears itself when the player leaves.

## Your first script

Here is a minimal, complete script. `debugproc` is a code-only trigger — a
script you call by name from a dev/cheat command rather than one the engine
fires on an event, so it needs no active pointers.

```rs2
[debugproc,hello]
def_string $name = "world";
mes("Hello, <$name>!");
```

Line by line: the header names the script `hello`; `def_string` declares a
local; `mes` prints to the player's message box, and `<$name>` interpolates the
local into the string (the `<…>` form also runs commands, as in
`<tostring($count)>`).

### Compile-checking it

`goscape-cli compile -check` type-checks source without writing bytecode. One
detail matters: the compiler resolves engine commands like `mes` from their
declarations in `scripts/engine.rs2`, and it resolves any procs you call from
wherever they are defined in the tree. It only sees files under the source path
you give it — so you check a script by compiling it **as part of the content
tree**, not as a lone file. In practice: save your script into the tree (say
`data/src/scripts/hello.rs2`) and point the compiler at the whole `scripts/`
directory:

```console
$ goscape-cli compile -check \
    -src-dir data/src \
    -datapack-dir data/pack \
    data/src/scripts
```

- `-check` is diagnostics-only; it discards the compiled output.
- `-src-dir` is the content root (the directory holding `scripts/` and
  `pack/`), from which the compiler loads entity symbols.
- `-datapack-dir` points at the packed entity-type cache — the
  `data/pack/server/*.dat` files produced by `goscape-cli pack`. The
  [Toolchain](toolchain.md) page covers packing.
- The final argument is the source path to compile: a directory (walked
  recursively) or a single file.

A clean run ends with `compile succeeded` and exit status `0`. An error prints
the offending file, line, and column with a caret under the token, and exits
non-zero — for example a mistyped command:

```text
data/src/scripts/hello.rs2:3:1: ERROR: 'messs' cannot be resolved to a command.
    > messs("Hello, <$name>!");
    > ^
```

With a working first script in hand, read [Examples](examples.md) for real,
annotated content, or the [Language reference](language.md) for the complete
spec.
