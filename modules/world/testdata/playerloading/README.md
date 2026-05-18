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

The script (commit to Engine-TS, not goscape):

```typescript
// scripts/gen-playerloading-fixtures.ts
// NB: addInv, getLevelByExp, and savePatched are not implemented here.
// Port addInv + getLevelByExp from Engine-TS Player.ts; copy
// Player.save() into savePatched and wrap version-gated sections with
// `if (version >= N)` guards. The bottom of this file shows the
// savePatched contract.
import 'dotenv/config';
import * as fs from 'node:fs';
import * as path from 'node:path';
import Player from '#/engine/entity/Player.js';
import { PlayerLoading } from '#/engine/entity/PlayerLoading.js';
import World from '#/engine/World.js';
import VarPlayerType from '#/cache/config/VarPlayerType.js';

const OUT_DIR = '/path/to/goscape/modules/world/testdata/playerloading';

async function main() {
    // World init so VarPlayerType / InvType are populated.
    await World.start({ skipMaps: true, startCycle: false });

    for (let version = 1; version <= 6; version++) {
        const p = makeFixturePlayer(version);
        // Monkey-patch SAV_VERSION for this iteration.
        (PlayerLoading as any).SAV_VERSION = version;
        // Save body must skip version-gated sections above current version
        // via the version-aware patched save (see savePatched below).
        const bytes = savePatched(p, version);
        fs.writeFileSync(path.join(OUT_DIR, `v${version}.sav`), bytes);
        console.log(`wrote v${version}.sav (${bytes.length} bytes)`);
    }
}

function makeFixturePlayer(version: number): Player {
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

// savePatched mirrors Player.save() but skips version-gated sections above N.
function savePatched(p: Player, version: number): Uint8Array {
    // ... (copy Player.save() body verbatim, wrap v2+/v3+/v4+/v5+/v6+
    //     sections with `if (version >= N)` guards; iterate p.invs sorted
    //     by typeId ascending).
}

main().catch(e => { console.error(e); process.exit(1); });
```

## Why this script and not just `player.save()`

(a) `Player.save()` always writes the current SAV_VERSION (6). We need
all six legacy formats to pin backward-compat decode.
(b) Real `player.invs` is iterated in Map-insertion-order; goscape
sorts ascending by typeId. The patched script enforces sort order so
the byte-identity test holds against goscape's encoder.

## Regenerating

If field values change, edit both this README and the tsx script in
lock-step, then run the script and commit the new binary fixtures.
