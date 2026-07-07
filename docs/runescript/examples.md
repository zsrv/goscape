# Examples

Three complete scripts, copied verbatim from the
[`LostCityRS/Content`](https://github.com/LostCityRS/Content) repository that
goscape compiles. Each is real, shipping content — the file path above every
listing is where it lives in the tree. They are ordered from simplest to most
involved: a debug proc, an NPC interaction, and a zone-driven timer that
enqueues a delayed action. For the syntax behind any line, follow the links
into the [Language reference](language.md); to write and compile-check your own,
see [Writing scripts](writing-scripts.md).

## A simple debug proc

`scripts/_test/scripts/engine/debug_pos.rs2`

```rs2
[debugproc,pos]
def_int $x = coordx(coord);
def_int $z = coordz(coord);
def_int $y = coordy(coord);
mes("Position: <tostring($x)> <tostring($z)> <tostring($y)>");
```

`debugproc` is a code-only trigger: a script you invoke by name from a dev
command, not one the engine fires on a game event
([§1, trigger families](language.md#1-file-layout-triggers)). It relies on the
active player pointer being set by whoever calls it.

- `coord` is the active player's coordinate. `coordx`, `coordz`, and `coordy`
  unpack it into its x, z, and y (level) components — three `int`s stored in
  script-local `$x`, `$z`, `$y` (`def_int`, [§2](language.md#2-variables)).
- `mes` prints one line to the message box. Inside the string, `<tostring($x)>`
  runs the `tostring` command on the local and splices the result in — the same
  `<…>` string-interpolation form introduced on the
  [Writing scripts](writing-scripts.md) page.

## An NPC interaction

`scripts/areas/area_ardougne_west/scripts/priest.rs2`

```rs2
[opnpc1,w_ardoungepriest]
~chatplayer("<p,neutral>Hello there.");
~chatnpc("<p,sad>I wish there was more I could do for these people.");
```

This runs when a player clicks the first option on the `w_ardoungepriest` NPC.
An `opnpc1` trigger arrives with **two active pointers already set** — the
player who clicked and the npc they clicked — and this script uses both without
naming either ([§6, Pointers](language.md#6-pointers-the-implicit-active-register)).

- The leading `~` invokes a **proc**. `~chatplayer` and `~chatnpc` are content
  procs (defined in `scripts/interface_chat/scripts/chat.rs2`) that wrap the
  engine's raw dialogue commands; `~chatplayer` speaks as the active player and
  `~chatnpc` as the active npc, producing the two-sided conversation.
- `<p,neutral>` and `<p,sad>` are facial-expression tags the chat procs
  interpret to animate the speaker's dialogue portrait. (This is distinct from
  the inline text tags — `<col=…>`, `<br>` — described in
  [§8, Dialogue & string formatting](language.md#8-dialogue-string-formatting).)

## A zone timer that enqueues an action

`scripts/areas/area_draynor/scripts/manor_vines.rs2`

```rs2
[zone,0_48_52_40_8] settimer(draynor_manor_vines, 5);

[timer,draynor_manor_vines]
// will only start timer after you enter the zone, but will continue 1 tile n/e outside of the zone
if(inzone(0_48_52_40_8, movecoord(0_48_52_40_8, 9, 0, 9), coord) = false) {
    cleartimer(draynor_manor_vines);
    return;
}
mes("A vine whips out and attacks you!");
spotanim_map(whippingplant_attack, coord, 0, 0);
sound_synth(nasty_tree_attack, 0, 0);
queue(damage_player, 0, 1);
```

This file combines a **timer** and a **queue** — the two scheduling mechanisms
from [§7](language.md#7-queues-timers-and-engine-driven-scheduling) — to make
the Draynor Manor vines periodically lash out at whoever is standing among them.

- **`[zone,0_48_52_40_8] settimer(draynor_manor_vines, 5);`** — a zone trigger.
  When a player enters the zone anchored at that coord, `settimer` registers the
  `draynor_manor_vines` timer to fire every `5` ticks for that player
  ([§7, timers](language.md#7-queues-timers-and-engine-driven-scheduling)).
- **`[timer,draynor_manor_vines]`** — the timer body, run once per interval.
- The `if` guards against the player having left. `movecoord` offsets the
  zone's base coord to compute its far corner; `inzone(corner1, corner2, coord)`
  tests whether the active player's `coord` is still inside those bounds. If not
  (`= false`), `cleartimer` cancels the timer and `return` stops this run —
  a self-cancelling timer, the standard idiom for one that a `[zone,…]` trigger
  started.
- Otherwise the effect fires: `mes` warns the player, `spotanim_map` plays the
  attack animation at the player's `coord`, and `sound_synth` plays the sound.
- **`queue(damage_player, 0, 1);`** enqueues the `damage_player` queue script
  with delay `0` and argument `1` — the actual damage is applied through a
  separate `[queue,damage_player]` script rather than inline, keeping the timer
  body to scheduling and presentation.

---

Every script above is compiled by goscape as part of the content tree; see
[Writing scripts › Compile-checking it](writing-scripts.md#compile-checking-it)
for the exact `goscape-cli compile -check` command that type-checks them.
