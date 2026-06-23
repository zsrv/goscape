# RuneScript

RuneScript is the domain-specific scripting language used by the LostCity RuneScape
engine (and historically by Jagex's RS2/RuneScape revisions) to drive content:
NPC dialogue, item interactions, quests, combat AI, interface logic, and timed
behavior. Scripts are stored as `.rs2` source, compiled to a stack-based
bytecode, and executed by a per-engine ScriptRunner against a `ScriptState`.

This document captures the language as it appears in the `LostCityRS/Content`
corpus and as the goscape engine ports it from `LostCityRS/Engine-TS`.

---

## 1. File layout & triggers

A `.rs2` file contains one or more **scripts**, each introduced by a bracketed
**trigger header** of the form `[trigger,subject]`. The trigger names the event
that fires the script; the subject names the entity (or interface component, or
queue slot) it binds to.

```rs2
[opheld1,bronze_dagger]
mes("You inspect the dagger.");

[oploc1,oak_tree]
mes("You swing your axe at the oak.");

[opnpc1,man]
chatnpc("Hello there.");
```

Common trigger families:

| Family                | Examples                                       | Fires when…                               |
| --------------------- | ---------------------------------------------- | ----------------------------------------- |
| Inventory item ops    | `opheld1`..`opheld5`, `opheldu`, `opheldt`     | Player clicks an option on an inv item    |
| Loc (scenery) ops     | `oploc1`..`oploc5`, `oplocu`, `oploct`, `aploc1`..`aploc5` | Player clicks a scenery option (ap = approach/out-of-range) |
| NPC ops               | `opnpc1`..`opnpc5`, `opnpcu`, `opnpct`, `apnpc*` | Player clicks an NPC option              |
| Ground obj ops        | `opobj1`..`opobj5`, `opobju`, `opobjt`         | Player clicks a ground item               |
| Player ops            | `opplayer1`..`opplayer5`                       | Player right-clicks another player        |
| AI                    | `ai_queue1`..`ai_queue20`, `ai_timer`, `ai_spawn`, `ai_aprange` | NPC behaviour ticks    |
| Queues / timers       | `queue,name`, `weakqueue,name`, `timer,name`   | Engine drains the named queue / timer     |
| Lifecycle             | `login`, `logout`, `autocon`                   | Per-player session events                 |
| Interface             | `if_button,iface:comp`, `if_buttond`, `inv_button` | UI interactions on an open interface  |
| Code-only             | `proc,name`, `label,name`, `debugproc,name`    | Called from other scripts                 |
| Client-side           | `clientscript,name`                            | Compiled into the client cache            |

`[proc,...]` scripts return values and are invoked via `~name(args)`; `[label,...]`
scripts are invoked via `jump @name(args)` and never return.

---

## 2. Variables

RuneScript has four variable scopes, each disambiguated by sigil:

| Sigil  | Scope                                           | Lifetime            |
| ------ | ----------------------------------------------- | ------------------- |
| `$x`   | Script-local                                    | One script execution |
| `%x`   | Player varp / vartemp / varbit                  | Per-player (varp persisted, vartemp wiped on logout) |
| `^x`   | Constant                                        | Compile-time        |
| `~x`   | Procedure call (not a variable, but same family) | n/a                |

Locals are typed at declaration:

```rs2
def_int $count = 0;
def_string $name = "Bob";
def_obj $weapon = null;
def_coord $home = 0_50_50_25_25;
```

Varp writes go through engine setters; the apparent no-op `%score = %score`
still emits a real `OpVarpSmall` packet (PUSH_VARP + POP_VARP) so the client
re-runs cs1 listeners on that varp — useful for forcing a UI resync.

Constants live in `.constant` files and are inlined at compile time:

```
^max_hp_level    99
^bank_iface      12
```

---

## 3. Types

Primitive: `int`, `string`, `boolean` (encoded as int 0/1), `coord`.

Cache-typed references are first-class; the compiler verifies the symbol
exists in the corresponding config:

`obj`, `namedobj`, `loc`, `npc`, `seq` (anim), `spotanim`, `synth` (sound),
`stat`, `inv` (inventory id), `enum`, `struct`, `param`, `dbtable`, `dbrow`,
`dbcolumn`, `interface`, `component`, `category`, `mapelement`.

`null` is the universal absent-value, type-narrowed by context.

A coord literal packs `level_mx_mz_lx_lz`:

```rs2
def_coord $lumby = 0_50_50_25_25;   // level 0, map (50,50), local (25,25)
```

---

## 4. Expressions & control flow

Arithmetic uses **infix `calc(...)`**; outside `calc` only assignments and
calls are syntactically permitted on the right-hand side:

```rs2
$total = calc($base + $bonus * 2);
$dmg   = calc(random(^max_hit) + 1);
```

Conditionals use C-style `if` / `else if` / `else` with `=`, `!`, `<`, `>`,
`<=`, `>=`, `&` (and), `|` (or):

```rs2
if ($hp < calc(%maxhp / 4)) {
    mes("You are badly wounded.");
} else if ($hp < calc(%maxhp / 2)) {
    mes("You are wounded.");
}
```

Loops are `while (...)`:

```rs2
$i = 0;
while ($i < inv_size(inv)) {
    if (inv_getobj(inv, $i) = coins) {
        $found = true;
    }
    $i = calc($i + 1);
}
```

`switch_int`, `switch_obj`, `switch_npc`, etc. are typed jump tables:

```rs2
switch_int ($choice) {
    case 1 : mes("First.");
    case 2,3 : mes("Second or third.");
    case default : mes("Other.");
}
```

---

## 5. Procedures, labels, and the call stack

A `proc` is a typed function with declared params and return tuple:

```rs2
[proc,greet](string $who)(string)
return("Hello, " + $who + "!");

[opnpc1,man]
def_string $line = ~greet("traveller");
chatnpc($line);
```

A `label` is tail-called: control transfers and never returns. Labels are how
long-running NPC AI cycles are written without growing the call stack.

`gosub` is the dynamic dispatch form (`gosub("name", args)`). When a gosub'd
script suspends (e.g. on `delay`), the caller's frame parks; the engine
resumes it on a later tick. In the goscape port, callers are responsible for
flipping `Execution = Running` before resuming — TS does this in ScriptRunner;
goscape leaves it explicit, which is an easy invariant to miss.

---

## 6. Pointers (the implicit `active*` register)

RuneScript's runtime carries a small set of *active* pointers — a player, an
npc, a loc, an obj, etc. — that most commands implicitly consume. Triggers set
the relevant pointers on entry: `[opheld1,...]` arrives with active player and
active obj; `[opnpc1,...]` arrives with active player and active npc.

Many commands have a primary form (acts on the active player) and a `.npc`
suffixed form (acts on the active npc):

```
anim          // animates active player
npc_anim      // animates active npc
stat          // reads active player stat
npc_stat      // reads active npc stat
```

The bytecode verifier enforces that the requisite pointer is set; a script
that calls `npc_anim` without an active npc fails at compile time.

---

## 7. Queues, timers, and engine-driven scheduling

Four queue flavours, all triggered via `[queue,name]` / `[ai_queue,name]`:

| Command       | Survives logout? | Survives death?                  | Use case                       |
| ------------- | ---------------- | -------------------------------- | ------------------------------ |
| `queue`       | no               | no                               | Tick-delayed action            |
| `weakqueue`   | no               | no                               | Cancellable scenery animation  |
| `longqueue`   | yes              | yes                              | Persistent quest beat          |
| `strongqueue` | no               | yes (engine drops on death)      | Combat hits, must-fire effects |

`weakQueue` is the cancellable cohort; `queue.all()` and `weakQueue.all()` are
walked separately each tick (two distinct loops, ten lines apart in TS — easy
to miss when porting).

`settimer(timer_id, interval, args...)` registers a recurring trigger that
fires every `interval` ticks until `cleartimer` is called.

`STRONGQUEUE` and `LONGQUEUE` are **variadic** opcodes — they take a script ID
plus an arbitrary number of `int`/`string` arguments. They do not share helper
shape with fixed-arg siblings; porting them as fixed-arg silently breaks.

---

## 8. Dialogue & string formatting

`chatnpc(string)` and `chatplayer(string)` open a chat dialogue; `mes(string)`
prints to the message box; `objbox(obj, string)` shows an item dialogue.

The `chatnpc`/`chatplayer` family uses `|` (pipe) as the line-break delimiter
— the engine `SPLIT_INIT`s on `|` and renders one line per segment. `<br>`
works in `mes` but **not** in chat dialogue.

Inline tags:

```
<col=ff0000>red text</col>
<shad=000000>shadowed</shad>
<u>underline</u>
<str>strikethrough</str>
<img=N>            // sprite
<gt> <lt>          // literal > and <
```

`tostring(int)` converts; `string_length(s)`, `substring(s, start, end)`,
`uppercase(s)`, `lowercase(s)`, `parahelper(s, n)` manipulate.

---

## 9. Common command families (illustrative, not exhaustive)

```
// Movement / location
p_walk, p_telejump, p_teleport, p_arrivedelay
movecoord, distance, coord, coordx, coordy, coordz, map_members

// Inventory
inv_add, inv_del, inv_total, inv_getobj, inv_changeslot, inv_size,
inv_freespace, inv_moveitem, inv_setslot, inv_clear

// Stats
stat, stat_base, stat_add, stat_sub, stat_heal, stat_advance, stat_xp,
boostlevel, restorestat

// Animation / FX
anim, spotanim_pl, spotanim_map, spotanim_npc, sound_synth, midi_song

// NPCs
npc_add, npc_del, npc_anim, npc_say, npc_facesquare, npc_setmode,
npc_queue, npc_settimer, npc_findhero, npc_damage, npc_heal

// Locs / ground objs
loc_add, loc_del, loc_change, loc_anim,
obj_add, obj_del, obj_count, obj_takeitem

// Interfaces
if_openmain, if_opensub, if_close, if_settext, if_setanim, if_setmodel,
if_setobject, if_setcolour, if_setposition, if_setrecol

// Math / RNG
add, sub, multiply, divide, modulo, min, max, random, randominc, abs,
bitand, bitor, scale

// Queues / timers / control
queue, weakqueue, longqueue, strongqueue, settimer, cleartimer, delay,
gosub, jump, return
```

---

## 10. Engine-side notes (goscape porting)

A few places where RuneScript semantics constrain the engine implementation:

- **`%X = %X` is not a no-op.** It compiles to `PUSH_VARP` then `POP_VARP`,
  which writes through to the client packet stream and re-runs the client's
  cs1 listeners on that varp. Clients short-circuit on equal-value writes;
  the server cannot.
- **No `[varp,X]` content trigger exists** in the LostCity engine — varp
  changes drive client UI directly and cannot be hooked content-side. Bugs
  that look "varp-driven" must be fixed engine-side or via the writing site.
- **Suspended `gosub`s defer to later ticks.** A `npc_death` script that
  gosubs into a delay'd subroutine pauses; test fixtures must drive `n.turn`
  per tick to resolve the cascade.
- **Iterator commands** (`npc_findall`, `loc_findall`, `obj_findall`, etc.)
  use a single-tick template: a custom iterator struct, a state field on the
  ScriptState, a lazy snapshot built from `Lookup.ZoneFoo`, and a staleness
  check on access.
- **`ScriptArgument[]` is a sum type in TS.** goscape splits it into
  parallel `intArgs []int` + `stringArgs []string` slices; opcodes that take
  mixed types must thread both.

---

## 11. A worked example

A small NPC quest-giver: greet, branch on quest stage, deliver a reward.

```rs2
[opnpc1,quest_man]
if (%quest_stage = 0) {
    chatnpc("Will you help me find my missing cat?|"
          + "Last seen near the old well.");
    %quest_stage = 1;
} else if (%quest_stage = 1) {
    if (inv_total(inv, missing_cat) > 0) {
        chatnpc("My cat! Thank you, traveller.");
        inv_del(inv, missing_cat, 1);
        ~give_reward();
        %quest_stage = 2;
    } else {
        chatnpc("Have you found my cat yet?");
    }
} else {
    chatnpc("Thanks again for finding my cat.");
}

[proc,give_reward]()
inv_add(inv, coins, 100);
stat_advance(thieving, 250);
mes("<col=ff8800>You receive 100 coins and some XP.</col>");
```
