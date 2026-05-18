# PlayerLoading SAV fixtures

These `v{1..6}.sav` files are TS-generated fixtures used to pin
goscape's SAV codec to Engine-TS byte-for-byte at every historical
SAV_VERSION. Used by `player_save_test.go`.

## Fixture field values

Every fixture encodes a player with these deterministic values (NB:
inv map iterated in ascending typeId order — see deviation
`NAI-PLAYERLOADING-D-INVS-SORTED-BY-TYPEID`):

| Field | Value |
|---|---|
| x | 3094 |
| z | 3106 |
| level | 0 |
| body[0..6] | [0, 10, 18, 26, 33, 36, 42] |
| colors[0..4] | [3, 7, 11, 13, 17] |
| gender | 0 |
| runenergy | 10000 |
| playtime | 1234567 (v2+); 12345 (v1 — fits in u16) |
| stats[0..20] | i*1000 (i.e., stats[0]=0, stats[1]=1000, ..., stats[20]=20000) |
| levels[0..20] | min(99, baseLevels[i]) where baseLevels are derived from stats |
| varpCount | 2000 (must match goscape's `len(varpTypes.Configs)` at fixture-gen time — adjust to match) |
| vars[i] for i in [0..varpCount-1] | i*7 for PERM-scoped INT slots; 0 elsewhere |
| invCount | 2 |
| inv[0] | typeId=0, size=28, slots: slot 0 → (id=995, count=1000000); slot 4 → (id=1, count=1) |
| inv[1] | typeId=1, size=14, slots: slot 0 → (id=1038, count=1) |
| afkZones[0..1] | [200, 300] (v3+) |
| lastAfkZone | 42 (v3+) |
| publicChat / privateChat / tradeDuel | 1 / 2 / 0 (v4+; packedChatModes = (1<<4)|(2<<2)|0 = 0x18) |
| lastLoginTime | 1715200000000 (v6+; unix-ms 2024-05-08) |

## tsx generator script

Run in your Engine-TS checkout:

```bash
cd ~/Code/github.com/LostCityRS/Engine-TS
bun run scripts/gen-playerloading-fixtures.ts
```

The script (commit to Engine-TS, not goscape). Important shape rules:

1. **Use dynamic imports for the entity classes.** `Player` ↔ `NetworkPlayer`
   form a circular import (NetworkPlayer extends Player). A static
   `import Player from '#/engine/entity/Player.js'` at the top of the
   script triggers `ReferenceError: Cannot access 'Player' before
   initialization` under Bun. `await World.start(...)` fully evaluates
   the entity class graph, so a dynamic `await import(...)` after that
   point resolves cleanly.
2. **Port the helper stubs.** `addInv`, `getLevelByExp`, and
   `savePatched` are NOT defined here — port them from Engine-TS source
   (`addInv` via `p.getInventory(typeId)` + slot setters from `Inv.ts`;
   `getLevelByExp` lives next to `getExpByLevel` in `Player.ts` /
   `PlayerStats.ts`; `savePatched` is `Player.save()` copied verbatim
   with each `v2+`/`v3+`/`v4+`/`v5+`/`v6+` section wrapped in
   `if (version >= N)` and `p.invs` sorted by typeId ascending).

```typescript
// scripts/gen-playerloading-fixtures.ts
import 'dotenv/config';
import * as fs from 'node:fs';
import * as path from 'node:path';
import World from '#/engine/World.js';

const OUT_DIR = '/path/to/goscape/modules/world/testdata/playerloading';

async function main() {
    // Initialise caches AND fully evaluate the entity class graph
    // (Player, NetworkPlayer, PlayerLoading). After this point the
    // dynamic imports below resolve without circular issues.
    await World.start({ skipMaps: true, startCycle: false });

    const { default: Player }      = await import('#/engine/entity/Player.js');
    const { PlayerLoading }        = await import('#/engine/entity/PlayerLoading.js');
    const { default: VarPlayerType } = await import('#/cache/config/VarPlayerType.js');
    // getLevelByExp lives near getExpByLevel — adjust the path if your
    // checkout exports it elsewhere (e.g., PlayerStats.js).
    const { getLevelByExp }        = await import('#/engine/entity/Player.js');

    for (let version = 1; version <= 6; version++) {
        const p = makeFixturePlayer(Player, VarPlayerType, getLevelByExp, version);
        (PlayerLoading as any).SAV_VERSION = version;
        const bytes = savePatched(p, version);
        fs.writeFileSync(path.join(OUT_DIR, `v${version}.sav`), bytes);
        console.log(`wrote v${version}.sav (${bytes.length} bytes)`);
    }
}

function makeFixturePlayer(
    Player: any, VarPlayerType: any, getLevelByExp: (xp: number) => number,
    version: number,
): any {
    const p = new Player('fixture', 0n, 0n);
    p.x = 3094;
    p.z = 3106;
    p.level = 0;
    p.body = [0, 10, 18, 26, 33, 36, 42];
    p.colors = [3, 7, 11, 13, 17];
    p.gender = 0;
    p.runenergy = 10000;
    p.playtime = version >= 2 ? 1234567 : 12345;

    for (let i = 0; i < 21; i++) {
        p.stats[i] = i * 1000;
        p.baseLevels[i] = getLevelByExp(p.stats[i]);
        p.levels[i] = p.baseLevels[i];
    }

    const varpCount = p.vars.length;  // = len(VarPlayerType.configs)
    for (let i = 0; i < varpCount; i++) {
        const type = VarPlayerType.get(i);
        if (type.scope === VarPlayerType.SCOPE_PERM && type.type !== /* STRING */ 22) {
            p.vars[i] = i * 7;
        }
    }

    // Two perm-scoped invs at typeIds 0 and 1 (verify these are SCOPE_PERM
    // in your varptype/invtype configs; pick alternative perm-scoped IDs
    // if not). Insert in ASCENDING typeId order to match Go's sort output.
    addInv(p, 0, 28, [{slot:0, id:995, count:1000000}, {slot:4, id:1, count:1}]);
    addInv(p, 1, 14, [{slot:0, id:1038, count:1}]);

    if (version >= 3) {
        p.afkZones[0] = 200;
        p.afkZones[1] = 300;
        p.lastAfkZone = 42;
    }
    if (version >= 4) {
        p.publicChat = 1;
        p.privateChat = 2;
        p.tradeDuel = 0;
    }
    if (version >= 6) {
        p.lastLoginTime = 1715200000000n;
    }

    return p;
}

// addInv lazily creates the inv via p.getInventory(typeId) and sets each
// requested slot. Replace `inv.set(...)` with the actual Inv-side
// setter in your checkout (e.g., `inv.add(slot, id, count)` or
// `inv.setSlot(slot, id, count)`).
function addInv(
    p: any, typeId: number, _size: number,
    slots: {slot: number, id: number, count: number}[],
): void {
    const inv = p.getInventory(typeId);
    for (const s of slots) {
        inv.set(s.slot, { id: s.id, count: s.count });
    }
}

// savePatched mirrors Player.save() but skips version-gated sections
// above N. Copy Player.save() body verbatim and wrap each version-gated
// section in `if (version >= N) { ... }`. CRITICAL: iterate p.invs
// sorted by typeId ascending (not Map insertion order) so the bytes
// match goscape's deterministic encoder.
function savePatched(p: any, version: number): Uint8Array {
    // ...
}

main().catch(e => { console.error(e); process.exit(1); });
```

### Known gotchas

- **`World.start` signature** — if your Engine-TS checkout doesn't accept
  `{ skipMaps, startCycle }`, grep `tests/` for the entry point your test
  suite uses (often a thin wrapper that calls `World.readyData()` /
  similar without entering the tick loop).
- **`getLevelByExp` import path** — exported from `Player.ts` in most
  recent revisions but historically lived in a `PlayerStats.ts` /
  `getStats.ts` sibling. Adjust the dynamic-import path if the named
  export isn't found.
- **`addInv` slot setter** — `Inv.ts` API has churned. Use whichever of
  `inv.set` / `inv.add` / `inv.setSlot` your checkout exposes.

## Why this script and not just `player.save()`

(a) `Player.save()` always writes the current SAV_VERSION (6). We need
all six legacy formats to pin backward-compat decode.
(b) Real `player.invs` is iterated in Map-insertion-order; goscape
sorts ascending by typeId. The patched script enforces sort order so
the byte-identity test holds against goscape's encoder.

## Regenerating

If field values change, edit both this README and the tsx script in
lock-step, then run the script and commit the new binary fixtures.
