# Player's Guide

This guide is for people who want to *play* on a goscape server: what you need to
connect, how logging in works, and where to look up the items, NPCs, places, and
commands that exist in the game world.

goscape is a **server** — it does not include a game client. You play through the
LostCityRS Java game client, which talks to a goscape server over the network. A
**revision** is a numbered snapshot of the RuneScape game (its map, items, NPCs,
and rules) that the server and client must agree on. These pages describe game
revision **{{ revision }}**. If you are looking at a different server, use the
**version selector in the header** to switch this documentation to the matching
revision.

## What you need

To play you need the **LostCityRS Client-Java** build that matches the server's
revision. Client and server speak a wire protocol that changes between revisions,
so a rev-{{ revision }} client will not work against a server built for another
revision, and vice versa.

Each revision branch pins the exact upstream client commit it was built and
tested against. Rather than repeat commit hashes here (they change as the port
advances), the pins are recorded in one place and treated like a lockfile — see
the [References & pins](../contributor/references-pins.md) page in the
Contributor's Guide, which lists the pinned `Client-Java` commit for every
revision alongside the other upstream sources.

## Connecting to a server

You connect to a goscape server that someone else (the **server operator**) is
running. From them you need one thing: the **host address** of the machine the
server runs on. Point your client at that host and it reaches the server through
two network listeners:

- **World (TCP)** — the port your client talks to for live gameplay: movement,
  chat, combat, everything you do in the world. By default this is TCP port
  `43594`, the classic RuneScape game port.
- **OnDemand (HTTP)** — an HTTP service the client downloads the game cache from
  (the map, models, and other assets it needs to render the world). By default
  this is port `8080`.

Both ports are configurable by the operator, so the numbers above are only the
defaults. Everything about wiring a client to a running server — the full list of
listeners, and how a client that runs on a different machine reaches them — is
covered from the operator's side in the Administrator's Guide under
[Connecting a client](../admin/quickstart.md#connecting-a-client).

## Accounts and logging in

There is **no separate sign-up step**. You log in from the client with a username
and a password, and whether that creates an account depends on one server
setting:

- **When auto-registration is enabled** (`login.auto-register`, which is on by
  default), the *first* time you log in with a username, the server creates the
  account for that username and remembers the password you used. Every later
  login checks your password against that stored account.
- **When auto-registration is disabled**, logging in with a username the server
  has never seen is rejected as invalid credentials — the operator has to provide
  accounts some other way.

Passwords are **case-insensitive**: the server lowercases the password before
storing and before checking it, so `Hunter2` and `hunter2` are the same password.
Whether auto-registration is on is the operator's choice; it is one of the login
settings documented in the Administrator's Guide
[Config reference](../admin/config-reference.md).

## Reference tour

The rest of the Player's Guide is generated directly from the server's game data
for this revision, so the numbers and names match exactly what is live on a
rev-{{ revision }} server. Use these as a lookup when you want to know what
something is or where to find it. For a side-by-side of how the totals differ
between revisions, see [Revision comparison](revision-comparison.md).

- **[Commands](commands.md)** — the text commands you can type in chat. These come
  in two families: `::` **engine cheat commands** (built into the server engine)
  and `~` **debugproc commands** (defined in the game's RuneScript content; the
  `~` prefix is itself configurable by the operator). Most commands are gated
  behind **staff moderator levels** and exist for administration and testing — an
  ordinary player account cannot run the bulk of them.
- **[Items](items/index.md)** — every object in the game, from coins to equipment,
  with each one's description, shop value, and properties, spread across several
  pages.
- **[NPCs](npcs/index.md)** — the non-player characters that populate the world,
  such as townspeople, shopkeepers, and monsters.
- **[Locations](locs/index.md)** — the world objects you see and interact with,
  such as doors, ladders, trees, and furniture (often called "locs").
- **[Music tracks](music.md)** — the in-game music tracks and short jingles.
- **[Places](places.md)** — the named world-map labels (regions and landmarks) and
  their map coordinates.
