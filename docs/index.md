# goscape

goscape is a Go rewrite of the LostCityRS
[Engine-TS](https://github.com/LostCityRS/Engine-TS) RuneScape server, built to
speak to the Lost City Java game client. It is a faithful 1:1 translation of the
reference server, with one complete port per game revision.

You are reading the documentation for game revision **{{ revision }}**
(branch `{{ revision_branch }}`). Use the **version selector in the site
header** to switch to another revision.

## Guides

This site is organised into five guides. Start with the one that matches what
you are here to do.

### [Administrator's Guide](admin/index.md)

Run and operate a goscape server. goscape ships as a **single binary** that can
run the whole server or just one part of it, selected with the `--target` flag.
This guide covers the module layout and service lifecycle, then the practical
work of running a server: a quick start, the configuration reference, deployment
scenarios, Helm value examples, and an operations runbook.

### [Player's Guide](player/index.md)

Play on a goscape server. goscape is a server, not a client — you connect
through the LostCityRS Java client. This guide explains what you need to
connect, how accounts and logging in work, and gives a reference tour of the
game world for this revision: items, NPCs, locations, commands, music, and
places, all generated directly from the server's own game data.

### [RuneScript](runescript/index.md)

Write the content that drives the game. RuneScript is the domain-specific
language behind every NPC dialogue, item interaction, quest, and timed
behaviour. This guide covers the trigger model and the compile-then-run
workflow, with an authoritative language reference, worked examples, and the
`goscape-cli` toolchain that compiles and packs scripts into the game cache.

### [Contributor's Guide](contributor/index.md)

Change the engine itself. goscape's guiding rule is faithful translation: every
ported region maps to an identifiable Engine-TS function, and every deviation is
tracked, never silent. This guide covers the branch model (one branch per
revision), the codebase architecture, the dev environment, and how to port a
whole new game revision end to end.

### [Protocol](protocol/index.md)

Implement or debug a client, proxy, or packet capture. These pages document the
RS2 wire protocol **as goscape actually implements it** — read straight out of
the marshal/unmarshal code, not from community lore: the login handshake, packet
framing, the ISAAC stream cipher, and the OnDemand cache surface.

## Which revision am I reading?

goscape keeps a separate port for each game revision, and this site publishes
the documentation for every one of them together. The **version selector in the
header** switches between them:

- Each revision is published as its own version, from `rev-225` (the oldest)
  through `rev-274` (the newest).
- `latest` is an alias for the newest revision, **rev-274** — a fresh visit
  lands there.
- When you are reading an older revision, a banner at the top of the page says
  so and links you to the latest one. The revision this particular build
  documents is **{{ revision }}**.

Revisions differ in more than a version number: the game data, and sometimes the
wire protocol, change between them (the [Protocol](protocol/index.md) guide
flags where). Make sure the revision shown in the header matches the server or
client you are working with.

## How this site is built

This documentation lives in the goscape repository and is built from the same
cross-revision sources as the code. If you want to build or preview it locally,
or regenerate the per-revision reference pages, the
[Dev environment](contributor/dev-setup.md) page in the Contributor's Guide has
the full workflow under its "Docs site" section.
