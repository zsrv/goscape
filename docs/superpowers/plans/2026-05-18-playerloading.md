# PlayerLoading Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Port the Engine-TS `PlayerLoading` codec (decode + verify) and
`Player.save()` (encode) to goscape's world module, and wire the codec
into the existing gRPC `LoginService` plumbing so that player state
flows end-to-end between the (out-of-process) login server and the
in-process `Player` struct.

**Architecture:** New twin files `modules/world/player_load.go` and
`modules/world/player_save.go` provide a byte codec over
`pkg/io/packet/*Packet`. Three wiring touchpoints: login decode in
`processLogins`, 15-min autosave in tick loop, logout/disconnect via
split helpers `removePlayerOnTick` (saves) and `removePlayerOnDisconnect`
(force-logout only). Byte-pin tests against TS-generated v1..v6
fixtures lock decode + v6 encode to TS bit-for-bit.

**Tech Stack:** Go 1.24, `pkg/io/packet` for byte sequencing, gRPC
client (already wired), `slog` for logging.

**Repo invocation prefix for `go` commands:** `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go ...`

**Git commit policy:** `git commit --no-gpg-sign` (per project CLAUDE.md).

---

## File map

```
modules/world/
├── player_load.go              (NEW) ← VerifySave + LoadSave + sentinel errors
├── player_save.go              (NEW) ← (*Player).Save()
├── player_save_test.go         (NEW) ← all codec + integration tests
├── client.go                   (MOD) ← add c.savePayload []byte
├── server.go                   (MOD) ← capture resp.Save; split removePlayer
├── tick.go                     (MOD) ← remove bootstrap at :157; add autosave;
│                                       LoadSave in processLogins; update call site at :305
├── testdata/playerloading/
│   ├── v1.sav                  (NEW, BINARY) ← TS-generated fixtures
│   ├── v2.sav                  (NEW, BINARY)
│   ├── v3.sav                  (NEW, BINARY)
│   ├── v4.sav                  (NEW, BINARY)
│   ├── v5.sav                  (NEW, BINARY)
│   ├── v6.sav                  (NEW, BINARY)
│   ├── fixture_values.go       (NEW) ← Go-side mirror of TS fixture field values
│   └── README.md               (NEW) ← tsx generator script + field values
```

---

## Task 0: Prepare fixture-generation infrastructure

**Files:**
- Create: `modules/world/testdata/playerloading/README.md`
- Create: `modules/world/testdata/playerloading/fixture_values.go`

This task documents the TS-side fixture-generator script and codifies
the exact field values used in fixtures so they're reproducible.
**Actual `.sav` binary generation is a user task** — they will run the
tsx generator script in their Engine-TS checkout and copy the resulting
bytes into the testdata dir. Tasks 4-10 depend on these binaries
existing.

- [ ] **Step 1: Write README.md with field values + tsx script**

Create `modules/world/testdata/playerloading/README.md`:

```markdown
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

\`\`\`typescript
// scripts/gen-playerloading-fixtures.ts
import 'dotenv/config';
import * as fs from 'node:fs';
import * as path from 'node:path';
import Player from '#/engine/entity/Player.js';
import { PlayerLoading } from '#/engine/entity/PlayerLoading.js';
import World from '#/engine/World.js';
import VarPlayerType from '#/cache/config/VarPlayerType.js';
import InvType from '#/cache/config/InvType.js';

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
        fs.writeFileSync(path.join(OUT_DIR, \`v\${version}.sav\`), bytes);
        console.log(\`wrote v\${version}.sav (\${bytes.length} bytes)\`);
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
\`\`\`

## Why this script and not just `player.save()`

(a) `Player.save()` always writes the current SAV_VERSION (6). We need
all six legacy formats to pin backward-compat decode.
(b) Real `player.invs` is iterated in Map-insertion-order; goscape
sorts ascending by typeId. The patched script enforces sort order so
the byte-identity test holds against goscape's encoder.

## Regenerating

If field values change, edit both this README and the tsx script in
lock-step, then run the script and commit the new binary fixtures.

\`\`\`
```

- [ ] **Step 2: Write fixture_values.go**

Create `modules/world/testdata/playerloading/fixture_values.go` —
exports the same field values as Go constants so `player_save_test.go`
can assert against them without hardcoding magic numbers in test
bodies. This file lives under `testdata/` so it isn't compiled into
the production binary — but Go tooling does scan `testdata/`-prefixed
dirs for `.go` files if you `package testdata`. To avoid that, put it
under a sibling test-only path: actually keep it inside `testdata/`
and reference via copy. **Simpler**: put `fixture_values.go` at
`modules/world/playerloading_fixture_values_test.go` (package world,
`_test.go` suffix excludes it from the production binary).

Final placement: `modules/world/playerloading_fixture_values_test.go`.

```go
package world

// Field values that match the tsx fixture generator (see
// testdata/playerloading/README.md). Used by player_save_test.go for
// per-version decode assertions.
//
// Keep in lock-step with testdata/playerloading/README.md and the tsx
// fixture-generation script.

var fixturePlayerValues = struct {
    X, Z, Level         int
    Body                [7]int
    Colors              [5]int
    Gender              int
    Runenergy           int
    PlaytimeV1          int // u16-fitting
    PlaytimeV2Plus      int // 4-byte
    Stats               [21]int32
    AfkZones            [2]int32
    LastAfkZone         int
    PublicChat          int
    PrivateChat         int
    TradeDuel           int
    PackedChatModes     uint8 // (1<<4)|(2<<2)|0
    LastLoginTime       int64
}{
    X: 3094, Z: 3106, Level: 0,
    Body:   [7]int{0, 10, 18, 26, 33, 36, 42},
    Colors: [5]int{3, 7, 11, 13, 17},
    Gender:    0,
    Runenergy: 10000,
    PlaytimeV1:     12345,
    PlaytimeV2Plus: 1234567,
    Stats: func() (s [21]int32) {
        for i := 0; i < 21; i++ {
            s[i] = int32(i) * 1000
        }
        return
    }(),
    AfkZones:        [2]int32{200, 300},
    LastAfkZone:     42,
    PublicChat:      1,
    PrivateChat:     2,
    TradeDuel:       0,
    PackedChatModes: 0x18,
    LastLoginTime:   1715200000000,
}
```

- [ ] **Step 3: Commit the fixture infrastructure (README + values file)**

```bash
cd $HOME/Code/github.com/zsrv/goscape
git add modules/world/testdata/playerloading/README.md \
        modules/world/playerloading_fixture_values_test.go
git commit --no-gpg-sign -m "test(world/playerloading): fixture infrastructure (README + values mirror)

T0 of NAI-PLAYERLOADING. Documents the tsx fixture-generator script for
Engine-TS and codifies fixture field values as Go test constants so
v1..v6 decode tests can assert against named values rather than magic
numbers. Binary v{1..6}.sav files generated separately by user (Engine-TS
side); tasks T4-T10 depend on them.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

- [ ] **Step 4: Note for executor — user supplies binary fixtures**

After this task, the executor MUST pause and request the user to:
1. Author and run the tsx generator script in their Engine-TS checkout.
2. Copy the resulting `v{1..6}.sav` files into
   `modules/world/testdata/playerloading/`.
3. Commit those binaries with message
   `test(world/playerloading): TS-generated v1..v6 fixtures`.

Without those binaries, tasks T4-T10 cannot land.

---

## Task 1: Codec skeleton — constants + sentinel errors

**Files:**
- Create: `modules/world/player_load.go`
- Create: `modules/world/player_save.go`

- [ ] **Step 1: Create player_load.go skeleton**

```go
// Package world — SAV codec for Player persistence.
// Mirrors Engine-TS PlayerLoading.ts. See
// docs/superpowers/specs/2026-05-18-playerloading-design.md.
package world

import (
    "errors"

    "github.com/zsrv/goscape/pkg/io/packet"
)

const (
    // SavMagic is the on-disk magic at byte 0..1 of every SAV file.
    // Matches TS PlayerLoading.SAV_MAGIC.
    SavMagic uint16 = 0x2004

    // SavVersion is the current SAV format version emitted by (*Player).Save().
    // Decoder supports v1..SavVersion. Matches TS PlayerLoading.SAV_VERSION.
    SavVersion uint16 = 6
)

var (
    // ErrSavInvalidMagic is returned by LoadSave when the leading 2 bytes
    // do not match SavMagic. Mirrors TS 'Invalid save file' throw.
    ErrSavInvalidMagic = errors.New("playerloading: invalid save magic")

    // ErrSavUnsupportedVer is returned by LoadSave when the version byte
    // is 0 or greater than SavVersion. Mirrors TS 'Unsupported save version'.
    ErrSavUnsupportedVer = errors.New("playerloading: unsupported save version")

    // ErrSavCorrupt is returned by LoadSave when the trailing CRC does not
    // match the recomputed CRC of the leading payload. Mirrors TS
    // 'Incorrect save checksum'.
    ErrSavCorrupt = errors.New("playerloading: incorrect save checksum")
)

// VerifySave reports whether sav has a valid magic, a supported version,
// and a matching trailing CRC. Mirrors PlayerLoading.verify
// (PlayerLoading.ts:16-29).
func VerifySave(sav []byte) bool {
    // TODO(T2): implement
    _ = packet.GetCRC
    return false
}

// LoadSave populates p from sav. If len(sav) < 2 it applies the
// empty-save bootstrap (21 stats=0, baseLevels=1, levels=1; hitpoints
// at level 10 with matching XP). Mirrors PlayerLoading.load
// (PlayerLoading.ts:31-159). Returns an error on magic mismatch,
// unsupported version, or CRC mismatch.
func LoadSave(p *Player, sav []byte) error {
    // TODO(T3+): implement
    return nil
}
```

- [ ] **Step 2: Create player_save.go skeleton**

```go
package world

// Save serializes p to a fresh SAV byte slice at version SavVersion.
// Inventories iterate over typeIds in ascending order (deviation
// NAI-PLAYERLOADING-D-INVS-SORTED-BY-TYPEID). Mirrors Player.save()
// (Player.ts:190-270).
func (p *Player) Save() []byte {
    // TODO(T10): implement
    return nil
}
```

- [ ] **Step 3: Verify the package still builds**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./modules/world/...`
Expected: builds cleanly (no symbol collisions, no unused-import errors).

- [ ] **Step 4: Commit**

```bash
git add modules/world/player_load.go modules/world/player_save.go
git commit --no-gpg-sign -m "feat(world/playerloading): codec skeleton (constants + sentinel errors)

T1 of NAI-PLAYERLOADING. Adds SavMagic, SavVersion constants and the
three sentinel errors (ErrSavInvalidMagic, ErrSavUnsupportedVer,
ErrSavCorrupt). VerifySave / LoadSave / (*Player).Save are stubs with
TODO markers; subsequent tasks fill them.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 2: VerifySave implementation + tests

**Files:**
- Modify: `modules/world/player_load.go` (VerifySave body)
- Create: `modules/world/player_save_test.go`

- [ ] **Step 1: Write failing test for VerifySave_RejectsTooSmall**

Create `modules/world/player_save_test.go`:

```go
package world

import (
    "encoding/binary"
    "errors"
    "testing"

    "github.com/zsrv/goscape/pkg/io/packet"
)

func TestVerifySave_RejectsTooSmall(t *testing.T) {
    if VerifySave(nil) {
        t.Error("VerifySave(nil) should be false")
    }
    if VerifySave([]byte{0x20}) {
        t.Error("VerifySave([0x20]) should be false (too short for magic)")
    }
}
```

- [ ] **Step 2: Run test, verify it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestVerifySave_RejectsTooSmall -v`
Expected: FAIL — current stub returns false unconditionally, so this passes by accident. **Adjust** the assertion to also test a well-formed accept case so we see a real failure first:

```go
func TestVerifySave_AcceptsWellFormed(t *testing.T) {
    sav := buildValidSav(t, 6, []byte{0xAA, 0xBB})
    if !VerifySave(sav) {
        t.Error("VerifySave on well-formed v6 sav should be true")
    }
}

// buildValidSav constructs a minimal SAV with the given version and
// payload bytes, including a trailing valid CRC. Used by Verify tests.
func buildValidSav(t *testing.T, version uint16, payload []byte) []byte {
    t.Helper()
    p := packet.NewPacket(make([]byte, 0, 16))
    p.P2(SavMagic)
    p.P2(version)
    for _, b := range payload {
        p.P1(b)
    }
    body := append([]byte{}, p.Bytes()...)
    crc := packet.GetCRC(body, 0, len(body))
    out := append(body, 0, 0, 0, 0)
    binary.BigEndian.PutUint32(out[len(body):], crc)
    return out
}
```

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestVerifySave_AcceptsWellFormed -v`
Expected: FAIL — stub returns false.

- [ ] **Step 3: Implement VerifySave**

Replace the VerifySave stub in `player_load.go`:

```go
func VerifySave(sav []byte) bool {
    if len(sav) < 8 {
        return false
    }
    p := packet.NewPacketView(sav)
    if p.G2() != SavMagic {
        return false
    }
    version := p.G2()
    if version < 1 || version > SavVersion {
        return false
    }
    // CRC covers bytes [0, len-4); trailing 4 bytes are the CRC itself.
    bodyLen := len(sav) - 4
    if bodyLen < 4 {
        return false
    }
    expected := packet.GetCRC(sav, 0, bodyLen)
    p.SetPos(bodyLen)
    got := p.G4()
    return got == expected
}
```

**Check**: confirm `packet.NewPacketView` and `Packet.SetPos` exist. If
the API differs (e.g., `NewPacket(bytes)` is the constructor for a
read-from-bytes packet, and `Pos` is a field), adjust to the actual
API. Look at existing call sites in `pkg/cache/preloaded.go:60` and
`pkg/io/jagfile/jagfile.go:302` for the canonical pattern.

- [ ] **Step 4: Add the rest of the verify test suite**

Append to `player_save_test.go`:

```go
func TestVerifySave_RejectsBadMagic(t *testing.T) {
    sav := buildValidSav(t, 6, []byte{0x00})
    sav[0] = 0xFF // corrupt magic
    if VerifySave(sav) {
        t.Error("VerifySave with corrupted magic should be false")
    }
}

func TestVerifySave_RejectsUnsupportedVer(t *testing.T) {
    sav := buildValidSav(t, 7, []byte{0x00}) // version 7
    if VerifySave(sav) {
        t.Error("VerifySave with version=7 should be false")
    }
    sav = buildValidSav(t, 0, []byte{0x00}) // version 0
    if VerifySave(sav) {
        t.Error("VerifySave with version=0 should be false")
    }
}

func TestVerifySave_RejectsCorruptCRC(t *testing.T) {
    sav := buildValidSav(t, 6, []byte{0xAA})
    sav[len(sav)-1] ^= 0xFF // flip last CRC byte
    if VerifySave(sav) {
        t.Error("VerifySave with corrupted CRC should be false")
    }
}
```

- [ ] **Step 5: Run all VerifySave tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestVerifySave -v`
Expected: all PASS.

- [ ] **Step 6: Commit**

```bash
git add modules/world/player_load.go modules/world/player_save_test.go
git commit --no-gpg-sign -m "feat(world/playerloading): VerifySave + tests

T2 of NAI-PLAYERLOADING. Implements PlayerLoading.verify
(PlayerLoading.ts:16-29) with 4 tests: well-formed accept, too-small
reject, bad-magic reject, unsupported-version reject (0 and 7), and
CRC-mismatch reject.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 3: LoadSave — empty-save bootstrap + tests

**Files:**
- Modify: `modules/world/player_load.go` (LoadSave body, partial)
- Modify: `modules/world/player_save_test.go`

- [ ] **Step 1: Write failing test for empty-save bootstrap**

Append to `player_save_test.go`:

```go
import (
    // ...existing imports...
    "github.com/zsrv/goscape/pkg/objtype"
)

func TestLoadSave_EmptyByteSliceBootstraps(t *testing.T) {
    p := &Player{}
    if err := LoadSave(p, []byte{}); err != nil {
        t.Fatalf("LoadSave(empty) returned err: %v", err)
    }
    for i := 0; i < objtype.PlayerStatCount; i++ {
        if i == objtype.PlayerStatHitpoints {
            continue
        }
        if p.stats[i] != 0 || p.baseLevels[i] != 1 || p.levels[i] != 1 {
            t.Errorf("stat %d: got (stats=%d, base=%d, lvl=%d), want (0, 1, 1)",
                i, p.stats[i], p.baseLevels[i], p.levels[i])
        }
    }
    wantHpExp := int32(objtype.GetExpByLevel(10))
    if p.stats[objtype.PlayerStatHitpoints] != wantHpExp {
        t.Errorf("hp stats: got %d, want %d", p.stats[objtype.PlayerStatHitpoints], wantHpExp)
    }
    if p.baseLevels[objtype.PlayerStatHitpoints] != 10 || p.levels[objtype.PlayerStatHitpoints] != 10 {
        t.Errorf("hp levels: got (base=%d, lvl=%d), want (10, 10)",
            p.baseLevels[objtype.PlayerStatHitpoints], p.levels[objtype.PlayerStatHitpoints])
    }
}

func TestLoadSave_NilSliceBootstraps(t *testing.T) {
    p := &Player{}
    if err := LoadSave(p, nil); err != nil {
        t.Fatalf("LoadSave(nil) returned err: %v", err)
    }
    if p.stats[objtype.PlayerStatHitpoints] != int32(objtype.GetExpByLevel(10)) {
        t.Errorf("nil-slice path didn't bootstrap hp like empty-slice path")
    }
}
```

- [ ] **Step 2: Run test, verify it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestLoadSave_EmptyByteSliceBootstraps -v`
Expected: FAIL — stub returns nil without populating fields.

- [ ] **Step 3: Implement empty-save bootstrap in LoadSave**

Replace LoadSave body in `player_load.go`:

```go
func LoadSave(p *Player, sav []byte) error {
    if len(sav) < 2 {
        // Empty-save bootstrap. Mirrors PlayerLoading.ts:41-53.
        for i := 0; i < objtype.PlayerStatCount; i++ {
            p.stats[i] = 0
            p.baseLevels[i] = 1
            p.levels[i] = 1
        }
        // Hitpoints starts at level 10.
        p.stats[objtype.PlayerStatHitpoints] = int32(objtype.GetExpByLevel(10))
        p.baseLevels[objtype.PlayerStatHitpoints] = 10
        p.levels[objtype.PlayerStatHitpoints] = 10
        return nil
    }
    // TODO(T4+): full decode
    return nil
}
```

Add import: `"github.com/zsrv/goscape/pkg/objtype"`

- [ ] **Step 4: Run tests, verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestLoadSave_EmptyByteSliceBootstraps -v && GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestLoadSave_NilSliceBootstraps -v`
Expected: both PASS.

- [ ] **Step 5: Commit**

```bash
git add modules/world/player_load.go modules/world/player_save_test.go
git commit --no-gpg-sign -m "feat(world/playerloading): LoadSave empty-save bootstrap

T3 of NAI-PLAYERLOADING. Ports the PlayerLoading.ts:41-53 'no save data'
branch into LoadSave: 21 skills initialised to (stats=0, base=1, lvl=1)
with hitpoints overridden to level 10 + matching XP. Two pins for empty
and nil slice inputs.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 4: LoadSave — v1 header + body decode

**Prerequisite:** User has placed `testdata/playerloading/v1.sav`.

**Files:**
- Modify: `modules/world/player_load.go`
- Modify: `modules/world/player_save_test.go`

- [ ] **Step 1: Write failing test for V1 decode**

Append to `player_save_test.go`:

```go
import (
    // ...existing...
    "os"
    "path/filepath"
)

func mustReadFixture(t *testing.T, name string) []byte {
    t.Helper()
    raw, err := os.ReadFile(filepath.Join("testdata", "playerloading", name))
    if err != nil {
        t.Fatalf("reading fixture %s: %v", name, err)
    }
    return raw
}

func TestLoadSave_V1_DecodesHeaderAndBody(t *testing.T) {
    raw := mustReadFixture(t, "v1.sav")
    p := &Player{}
    if err := LoadSave(p, raw); err != nil {
        t.Fatalf("LoadSave(v1): %v", err)
    }
    fv := fixturePlayerValues
    if p.x != fv.X { t.Errorf("x: got %d, want %d", p.x, fv.X) }
    if p.z != fv.Z { t.Errorf("z: got %d, want %d", p.z, fv.Z) }
    if p.level != fv.Level { t.Errorf("level: got %d, want %d", p.level, fv.Level) }
    if p.body != fv.Body { t.Errorf("body: got %v, want %v", p.body, fv.Body) }
    if p.colors != fv.Colors { t.Errorf("colors: got %v, want %v", p.colors, fv.Colors) }
    if p.gender != fv.Gender { t.Errorf("gender: got %d, want %d", p.gender, fv.Gender) }
    if p.runenergy != fv.Runenergy { t.Errorf("runenergy: got %d, want %d", p.runenergy, fv.Runenergy) }
    if p.playtime != fv.PlaytimeV1 { t.Errorf("playtime: got %d, want %d", p.playtime, fv.PlaytimeV1) }
    for i, want := range fv.Stats {
        if p.stats[i] != want {
            t.Errorf("stats[%d]: got %d, want %d", i, p.stats[i], want)
        }
    }
}
```

- [ ] **Step 2: Run test, verify it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestLoadSave_V1_DecodesHeaderAndBody -v`
Expected: FAIL — LoadSave currently no-ops on non-empty input.

- [ ] **Step 3: Implement header check + v1 body decode**

Replace the v1+ branch (the `TODO(T4+)` placeholder) in LoadSave:

```go
func LoadSave(p *Player, sav []byte) error {
    if len(sav) < 2 {
        // ... empty-save bootstrap (unchanged) ...
        return nil
    }

    // Header: magic + version.
    pkt := packet.NewPacketView(sav)
    if pkt.G2() != SavMagic {
        return ErrSavInvalidMagic
    }
    version := pkt.G2()
    if version < 1 || version > SavVersion {
        return ErrSavUnsupportedVer
    }

    // CRC check: last 4 bytes of sav are the CRC of bytes [0, len-4).
    bodyLen := len(sav) - 4
    if bodyLen < 4 {
        return ErrSavCorrupt
    }
    pkt.SetPos(bodyLen)
    if pkt.G4() != packet.GetCRC(sav, 0, bodyLen) {
        return ErrSavCorrupt
    }

    // Body starts at byte 4.
    pkt.SetPos(4)
    p.x = int(pkt.G2())
    p.z = int(pkt.G2())
    p.level = int(pkt.G1())
    for i := 0; i < 7; i++ {
        b := int(pkt.G1())
        if b == 255 {
            b = -1
        }
        p.body[i] = b
    }
    for i := 0; i < 5; i++ {
        p.colors[i] = int(pkt.G1())
    }
    p.gender = int(pkt.G1())
    p.runenergy = int(pkt.G2())
    // Playtime: v1 is u16, v2+ is i32.
    if version >= 2 {
        p.playtime = int(int32(pkt.G4()))
    } else {
        p.playtime = int(pkt.G2())
    }

    // 21 stats: each i32 exp + u8 current level. baseLevel derived from exp.
    for i := 0; i < objtype.PlayerStatCount; i++ {
        p.stats[i] = int32(pkt.G4())
        p.baseLevels[i] = uint8(objtype.GetLevelByExp(int(p.stats[i])))
        p.levels[i] = pkt.G1()
    }

    // TODO(T4 cont): varps, invs, v3+ afkZones, v4+ chat, v5+ inv size, v6+ lastLoginTime
    return nil
}
```

- [ ] **Step 4: Run test, verify it passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestLoadSave_V1_DecodesHeaderAndBody -v`
Expected: PASS through the asserted fields (stats[0..20], header fields, body, colors, etc.).

- [ ] **Step 5: Add varps + invs decode (still inside Task 4)**

Append below the stats loop in LoadSave:

```go
    // Varps. Count is u16, then `count` × i32. STRING-typed slots get 0
    // written into the int32 column (string state lives in varpsString).
    varpCount := int(pkt.G2())
    if cap(p.varps) < varpCount {
        p.varps = make([]int32, varpCount)
    } else {
        p.varps = p.varps[:varpCount]
    }
    for i := 0; i < varpCount; i++ {
        p.varps[i] = int32(pkt.G4())
    }

    // Inventories. Count is u1, then per-inv: typeId u2, size u2 (v5+
    // only; v1..v4 use invType.size), then `size` × slot (id u2 - 1; if
    // id == -1 skip; else count u1 with 255 meaning extended-u32).
    invCount := int(pkt.G1())
    for i := 0; i < invCount; i++ {
        typeID := int(pkt.G2())
        var size int
        if version >= 5 {
            size = int(pkt.G2())
        } else {
            // Look up invType.Size by typeID. If invTypes not loaded
            // (test path with stripped server), fall back to reading
            // exactly the slots that fit until... we can't; size is
            // required to advance the read cursor. Tests for v1..v4
            // must seed invTypes or accept that this section may panic.
            if p.invTypes == nil {
                return errors.New("playerloading: v" + itoa(int(version)) +
                    " decode requires invTypes; got nil")
            }
            if typeID < 0 || typeID >= len(p.invTypes.Configs) || p.invTypes.Configs[typeID] == nil {
                return errors.New("playerloading: unknown invType " + itoa(typeID))
            }
            size = p.invTypes.Configs[typeID].Size
        }
        for slot := 0; slot < size; slot++ {
            objID := int(pkt.G2()) - 1
            if objID == -1 {
                continue
            }
            count := int(pkt.G1())
            if count == 255 {
                count = int(int32(pkt.G4()))
            }
            // Only write to inventory if scope == SCOPE_PERM. Lookup
            // via invTypes; in tests without invTypes seeded, fall
            // back to assuming SCOPE_PERM (all fixture invs are perm).
            if p.invTypes != nil {
                if cfg := p.invTypes.Configs[typeID]; cfg != nil && cfg.Scope != objtype.InvTypeScopePerm {
                    continue
                }
            }
            inv, ok := p.invs[typeID]
            if !ok {
                // Inventory map not pre-seeded for this typeID. The
                // login flow seeds at least 'worn' before LoadSave runs;
                // others need create-on-demand here only if a test path
                // doesn't seed. For port parity with TS getInventory(),
                // skip silently if absent — TS does the same when
                // getInventory returns null.
                continue
            }
            inv.Set(slot, &inventory.Item{ID: objID, Count: count})
        }
    }
```

Notes for the executor:
- **`p.invTypes`** does not exist as a field on Player today. The varptype/invtype configs are owned by `Server`, not `Player`. Two design adjustments are needed:
  - Either pass a context to LoadSave (`LoadSave(p, sav, invTypes)`), or
  - Look invTypes up via `p.client.server.invTypes`.
- **Recommendation**: refactor `LoadSave` signature to take an explicit
  `invTypes *objtype.InvTypeConfigs` parameter (and similarly for
  varpTypes if needed by future fields). The internal helper used by
  tests can stub these with hand-built configs.
- **Helper `itoa`**: import `"strconv"` and use `strconv.Itoa`. (Don't
  invent a local `itoa`; it was shorthand.)

Adjusted LoadSave signature:

```go
func LoadSave(p *Player, sav []byte, invTypes *objtype.InvTypeConfigs) error {
    // ...
}
```

Update Task 3 tests accordingly: `LoadSave(p, []byte{}, nil)` (empty
path doesn't need invTypes).

- [ ] **Step 6: Update Task 3 tests + new V1 invs/varps assertions**

Modify the two existing bootstrap tests to pass `nil` for invTypes.

Extend `TestLoadSave_V1_DecodesHeaderAndBody`:

```go
    // varps
    if len(p.varps) == 0 {
        t.Error("varps not decoded")
    }
    // ...check varps[i] == i*7 for INT+PERM slots; harder to assert
    // without varp type config — skip detailed assertion if invTypes
    // path isn't seeded. Pin via byte-level round-trip in T10 instead.

    // invs (requires invTypes seeded in test fixture player)
    // Seed two inventories in p.invs (typeId 0 and 1) with the
    // expected capacities (28 and 14) before calling LoadSave.
```

Adjust the test helper `newTestPlayer(t)` (add it to the test file):

```go
func newTestPlayer(t *testing.T) (*Player, *objtype.InvTypeConfigs) {
    t.Helper()
    p := &Player{
        invs:   map[int]*inventory.Inventory{},
        varps:  []int32{},
    }
    cfgs := &objtype.InvTypeConfigs{
        Configs: []*objtype.InvType{
            {Size: 28, Scope: objtype.InvTypeScopePerm}, // typeId 0
            {Size: 14, Scope: objtype.InvTypeScopePerm}, // typeId 1
        },
    }
    p.invs[0] = inventory.FromType(cfgs.Configs[0])
    p.invs[1] = inventory.FromType(cfgs.Configs[1])
    return p, cfgs
}
```

**Important**: confirm `objtype.InvTypeConfigs` type name + exact path
via `grep -nR "type InvTypeConfigs" $HOME/Code/github.com/zsrv/goscape/pkg/objtype/`. Adjust as needed.

Modify the V1 decode test:

```go
func TestLoadSave_V1_DecodesHeaderAndBody(t *testing.T) {
    raw := mustReadFixture(t, "v1.sav")
    p, cfgs := newTestPlayer(t)
    if err := LoadSave(p, raw, cfgs); err != nil {
        t.Fatalf("LoadSave(v1): %v", err)
    }
    fv := fixturePlayerValues
    // ... existing field assertions ...

    // Inv 0 slot 0: id=995, count=1000000
    item0 := p.invs[0].Get(0)
    if item0 == nil || item0.ID != 995 || item0.Count != 1000000 {
        t.Errorf("inv[0][0]: got %+v, want {ID:995 Count:1000000}", item0)
    }
    item4 := p.invs[0].Get(4)
    if item4 == nil || item4.ID != 1 || item4.Count != 1 {
        t.Errorf("inv[0][4]: got %+v, want {ID:1 Count:1}", item4)
    }
}
```

- [ ] **Step 7: Run V1 test, fix until pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestLoadSave_V1 -v`
Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add modules/world/player_load.go modules/world/player_save_test.go modules/world/testdata/playerloading/v1.sav
git commit --no-gpg-sign -m "feat(world/playerloading): LoadSave v1 header + body + varps + invs

T4 of NAI-PLAYERLOADING. Ports PlayerLoading.ts:55-132 (header check,
v1 body, varps loop, invs loop with v1-style invType.Size lookup).
Reads body=255→-1 sentinel, obj.id-1 sentinel-skip, count=255 extended
read. invTypes passed explicitly via signature change
(LoadSave(p, sav, invTypes)). Pinned with v1.sav fixture decode.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 5: LoadSave — v2 delta (4-byte playtime)

**Prerequisite:** User has placed `testdata/playerloading/v2.sav`.

**Files:**
- Modify: `modules/world/player_save_test.go`

- [ ] **Step 1: Write failing test for V2 decode**

Append:

```go
func TestLoadSave_V2_DecodesPlaytimeAs4Byte(t *testing.T) {
    raw := mustReadFixture(t, "v2.sav")
    p, cfgs := newTestPlayer(t)
    if err := LoadSave(p, raw, cfgs); err != nil {
        t.Fatalf("LoadSave(v2): %v", err)
    }
    if p.playtime != fixturePlayerValues.PlaytimeV2Plus {
        t.Errorf("playtime: got %d, want %d (v2 must read 4 bytes, not 2)",
            p.playtime, fixturePlayerValues.PlaytimeV2Plus)
    }
}
```

- [ ] **Step 2: Run test**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestLoadSave_V2 -v`
Expected: PASS (the v2-branch is already implemented in T4). This is
purely a fixture-existence + version-branch coverage pin.

- [ ] **Step 3: Commit**

```bash
git add modules/world/player_save_test.go modules/world/testdata/playerloading/v2.sav
git commit --no-gpg-sign -m "test(world/playerloading): v2 fixture decode pin (4-byte playtime)

T5 of NAI-PLAYERLOADING. Pins the v2+ playtime branch (i32 vs v1's u16).
Encoder path of the branch is already covered by T4's implementation;
this test exists to lock fixture-existence and the version-aware read.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 6: LoadSave — v3 delta (afkZones + lastAfkZone)

**Prerequisite:** User has placed `testdata/playerloading/v3.sav`.

**Files:**
- Modify: `modules/world/player_load.go`
- Modify: `modules/world/player_save_test.go`

- [ ] **Step 1: Write failing test for V3 decode**

```go
func TestLoadSave_V3_DecodesAfkZones(t *testing.T) {
    raw := mustReadFixture(t, "v3.sav")
    p, cfgs := newTestPlayer(t)
    if err := LoadSave(p, raw, cfgs); err != nil {
        t.Fatalf("LoadSave(v3): %v", err)
    }
    if p.afkZones != fixturePlayerValues.AfkZones {
        t.Errorf("afkZones: got %v, want %v", p.afkZones, fixturePlayerValues.AfkZones)
    }
    if p.lastAfkZone != fixturePlayerValues.LastAfkZone {
        t.Errorf("lastAfkZone: got %d, want %d", p.lastAfkZone, fixturePlayerValues.LastAfkZone)
    }
}
```

- [ ] **Step 2: Run, verify it fails**

Expected: FAIL (afkZones decode not yet implemented).

- [ ] **Step 3: Add v3+ afkZones decode after invs loop**

In `player_load.go` LoadSave, after the inv-decode block:

```go
    // v3+: afk zones. Count is u1, then `count` × i32; then lastAfkZone u2.
    // Goscape's p.afkZones is fixed [2]int32 — bound the loop at 2.
    if version >= 3 {
        afkCount := int(pkt.G1())
        for i := 0; i < afkCount; i++ {
            v := int32(pkt.G4())
            if i < len(p.afkZones) {
                p.afkZones[i] = v
            }
            // else: silently drop excess (TS would OOB-write Int32Array).
        }
        p.lastAfkZone = int(pkt.G2())
    }
```

- [ ] **Step 4: Run V3 test**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestLoadSave_V3 -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add modules/world/player_load.go modules/world/player_save_test.go modules/world/testdata/playerloading/v3.sav
git commit --no-gpg-sign -m "feat(world/playerloading): LoadSave v3+ afkZones + lastAfkZone

T6 of NAI-PLAYERLOADING. Ports PlayerLoading.ts:134-141. Bound at
len(p.afkZones)=2 since goscape uses fixed-array storage.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 7: LoadSave — v4 delta (packed chat modes)

**Prerequisite:** `testdata/playerloading/v4.sav` placed.

**Files:**
- Modify: `modules/world/player_load.go`
- Modify: `modules/world/player_save_test.go`

- [ ] **Step 1: Write failing test**

```go
func TestLoadSave_V4_DecodesChatModes(t *testing.T) {
    raw := mustReadFixture(t, "v4.sav")
    p, cfgs := newTestPlayer(t)
    if err := LoadSave(p, raw, cfgs); err != nil {
        t.Fatalf("LoadSave(v4): %v", err)
    }
    fv := fixturePlayerValues
    if p.publicChat != fv.PublicChat || p.privateChat != fv.PrivateChat || p.tradeDuel != fv.TradeDuel {
        t.Errorf("chat modes: got (pub=%d, priv=%d, trade=%d), want (%d, %d, %d)",
            p.publicChat, p.privateChat, p.tradeDuel,
            fv.PublicChat, fv.PrivateChat, fv.TradeDuel)
    }
}
```

- [ ] **Step 2: Run, verify FAIL**

- [ ] **Step 3: Add v4+ chat-mode decode**

In LoadSave, after the v3+ block:

```go
    // v4+: chat modes packed into one u1 byte.
    if version >= 4 {
        packed := pkt.G1()
        p.publicChat = int((packed >> 4) & 0b11)
        p.privateChat = int((packed >> 2) & 0b11)
        p.tradeDuel = int(packed & 0b11)
    }
```

- [ ] **Step 4: Run V4 test → PASS**

- [ ] **Step 5: Commit**

```bash
git add modules/world/player_load.go modules/world/player_save_test.go modules/world/testdata/playerloading/v4.sav
git commit --no-gpg-sign -m "feat(world/playerloading): LoadSave v4+ packed chat modes

T7 of NAI-PLAYERLOADING. Ports PlayerLoading.ts:143-149. Packed byte
layout (publicChat<<4 | privateChat<<2 | tradeDuel), 2 bits each.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 8: LoadSave — v5 delta (per-inv size field)

**Prerequisite:** `testdata/playerloading/v5.sav` placed.

The v5 branch is **already implemented** in T4 (the inv-decode loop
reads `size = pkt.G2()` when `version >= 5`). This task only adds the
fixture pin test.

**Files:**
- Modify: `modules/world/player_save_test.go`

- [ ] **Step 1: Add test**

```go
func TestLoadSave_V5_DecodesPerInvSize(t *testing.T) {
    raw := mustReadFixture(t, "v5.sav")
    p, cfgs := newTestPlayer(t)
    if err := LoadSave(p, raw, cfgs); err != nil {
        t.Fatalf("LoadSave(v5): %v", err)
    }
    // Inv 0 size = 28 in fixture; inv 1 size = 14. Both should have
    // decoded items at the documented slots regardless of size source
    // (per-inv vs invType.Size).
    if item := p.invs[0].Get(0); item == nil || item.ID != 995 || item.Count != 1000000 {
        t.Errorf("v5 inv[0][0]: got %+v, want {ID:995 Count:1000000}", item)
    }
    if item := p.invs[1].Get(0); item == nil || item.ID != 1038 || item.Count != 1 {
        t.Errorf("v5 inv[1][0]: got %+v, want {ID:1038 Count:1}", item)
    }
}
```

- [ ] **Step 2: Run → PASS**

- [ ] **Step 3: Commit**

```bash
git add modules/world/player_save_test.go modules/world/testdata/playerloading/v5.sav
git commit --no-gpg-sign -m "test(world/playerloading): v5 per-inv-size pin

T8 of NAI-PLAYERLOADING. Locks the v5+ inv-section read of the size
field per-inv (vs deriving from invType.Size). Encoder of branch was
landed in T4; this test pins it.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 9: LoadSave — v6 delta (lastLoginTime)

**Prerequisite:** `testdata/playerloading/v6.sav` placed.

**Files:**
- Modify: `modules/world/player_load.go`
- Modify: `modules/world/player_save_test.go`

- [ ] **Step 1: Write failing test**

```go
func TestLoadSave_V6_DecodesLastLoginTime(t *testing.T) {
    raw := mustReadFixture(t, "v6.sav")
    p, cfgs := newTestPlayer(t)
    if err := LoadSave(p, raw, cfgs); err != nil {
        t.Fatalf("LoadSave(v6): %v", err)
    }
    if p.lastLoginTime != fixturePlayerValues.LastLoginTime {
        t.Errorf("lastLoginTime: got %d, want %d", p.lastLoginTime, fixturePlayerValues.LastLoginTime)
    }
}
```

- [ ] **Step 2: Run → FAIL**

- [ ] **Step 3: Add v6+ lastLoginTime decode + combat level recompute**

After the v4+ block:

```go
    // v6+: lastLoginTime is i64 unix-ms.
    if version >= 6 {
        p.lastLoginTime = int64(pkt.G8())
    }
    // Final: recompute derived combat level (mirrors TS PlayerLoading.ts:156).
    p.combatLevel = p.getCombatLevel()
```

**Note for executor:** verify the Go method is named `getCombatLevel`
or `GetCombatLevel` or similar — grep to confirm. If absent, leave a
TODO and tag `NAI-PLAYERLOADING-D-COMBAT-LEVEL-NOT-RECOMPUTED-ON-LOAD`.

- [ ] **Step 4: Run V6 test → PASS**

- [ ] **Step 5: Commit**

```bash
git add modules/world/player_load.go modules/world/player_save_test.go modules/world/testdata/playerloading/v6.sav
git commit --no-gpg-sign -m "feat(world/playerloading): LoadSave v6+ lastLoginTime + combatLevel recompute

T9 of NAI-PLAYERLOADING. Ports PlayerLoading.ts:151-156. v6 adds an i64
unix-ms field for last-login wall-clock. After all version-gated
sections, recompute combatLevel from the freshly-loaded base levels.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 10: (*Player).Save — v6 encode + byte-perfect round-trip

**Prerequisite:** `testdata/playerloading/v6.sav` placed (from T9).

**Files:**
- Modify: `modules/world/player_save.go`
- Modify: `modules/world/player_save_test.go`

- [ ] **Step 1: Write failing test for byte-perfect round-trip**

```go
import (
    // ...existing...
    "bytes"
)

func TestSave_V6_RoundTripsBytePerfect(t *testing.T) {
    raw := mustReadFixture(t, "v6.sav")
    p, cfgs := newTestPlayer(t)
    if err := LoadSave(p, raw, cfgs); err != nil {
        t.Fatalf("LoadSave(v6): %v", err)
    }
    got := p.Save(cfgs)
    if !bytes.Equal(got, raw) {
        t.Fatalf("Save() drift vs v6.sav (got %d bytes, want %d):\n"+
            "  first-diff at byte %d\n"+
            "  got=%x\n  want=%x",
            len(got), len(raw), firstDiff(got, raw), got, raw)
    }
}

func firstDiff(a, b []byte) int {
    n := len(a)
    if len(b) < n { n = len(b) }
    for i := 0; i < n; i++ {
        if a[i] != b[i] { return i }
    }
    if len(a) != len(b) { return n }
    return -1
}
```

- [ ] **Step 2: Run, verify FAIL**

Expected: FAIL — Save() returns nil.

- [ ] **Step 3: Implement (*Player).Save**

Replace player_save.go body. Note that we change the signature to take
invTypes (and possibly varpTypes) — mirroring LoadSave.

```go
package world

import (
    "sort"

    "github.com/zsrv/goscape/pkg/io/packet"
    "github.com/zsrv/goscape/pkg/objtype"
)

// Save serializes p to a fresh SAV byte slice at version SavVersion.
// Inventories iterate over typeIds in ascending order (deviation
// NAI-PLAYERLOADING-D-INVS-SORTED-BY-TYPEID). Mirrors Player.save()
// (Player.ts:190-270).
//
// varpTypes is needed to determine which varp slots are SCOPE_PERM
// (others are zeroed in the output). invTypes is needed to determine
// which inventories are SCOPE_PERM (others are skipped).
func (p *Player) Save(invTypes *objtype.InvTypeConfigs, varpTypes *objtype.VarpTypeConfigs) []byte {
    pkt := packet.NewPacket(make([]byte, 0, 256))
    pkt.P2(SavMagic)
    pkt.P2(SavVersion)
    pkt.P2(uint16(p.x))
    pkt.P2(uint16(p.z))
    pkt.P1(uint8(p.level))
    for i := 0; i < 7; i++ {
        pkt.P1(uint8(p.body[i])) // -1 → 255 via two's-complement
    }
    for i := 0; i < 5; i++ {
        pkt.P1(uint8(p.colors[i]))
    }
    pkt.P1(uint8(p.gender))
    pkt.P2(uint16(p.runenergy))
    pkt.P4(uint32(p.playtime))

    for i := 0; i < objtype.PlayerStatCount; i++ {
        pkt.P4(uint32(p.stats[i]))
        pkt.P1(p.levels[i])
    }

    // Varps: count, then PERM-INT slots written verbatim, others zeroed.
    pkt.P2(uint16(len(p.varps)))
    for i := 0; i < len(p.varps); i++ {
        if varpTypes != nil && i < len(varpTypes.Configs) {
            vt := varpTypes.Configs[i]
            if vt != nil && vt.Scope == objtype.VarpScopePerm && vt.Type != objtype.ScriptVarTypeString {
                pkt.P4(uint32(p.varps[i]))
                continue
            }
        }
        pkt.P4(0)
    }

    // Inventories: count (placeholder) then per-inv. Iterate in
    // ascending typeId order for deterministic, portable bytes.
    invStartPos := pkt.Pos()
    pkt.P1(0) // placeholder
    typeIDs := make([]int, 0, len(p.invs))
    for tid := range p.invs {
        typeIDs = append(typeIDs, tid)
    }
    sort.Ints(typeIDs)
    invCount := 0
    for _, tid := range typeIDs {
        if invTypes != nil && tid < len(invTypes.Configs) {
            if cfg := invTypes.Configs[tid]; cfg != nil && cfg.Scope != objtype.InvTypeScopePerm {
                continue
            }
        }
        inv := p.invs[tid]
        pkt.P2(uint16(tid))
        pkt.P2(uint16(inv.Capacity))
        for slot := 0; slot < inv.Capacity; slot++ {
            item := inv.Get(slot)
            if item == nil {
                pkt.P2(0)
                continue
            }
            pkt.P2(uint16(item.ID + 1))
            if item.Count >= 255 {
                pkt.P1(255)
                pkt.P4(uint32(item.Count))
            } else {
                pkt.P1(uint8(item.Count))
            }
        }
        invCount++
    }
    // Backfill the inv count placeholder.
    pkt.SetByteAt(invStartPos, byte(invCount))

    // v3+ afk zones — current SavVersion is 6, always written.
    pkt.P1(uint8(len(p.afkZones)))
    for i := 0; i < len(p.afkZones); i++ {
        pkt.P4(uint32(p.afkZones[i]))
    }
    pkt.P2(uint16(p.lastAfkZone))

    // v4+ packed chat modes.
    packed := uint8((p.publicChat&0b11)<<4 | (p.privateChat&0b11)<<2 | (p.tradeDuel & 0b11))
    pkt.P1(packed)

    // v6+ lastLoginTime.
    pkt.P8(uint64(p.lastLoginTime))

    // Trailing CRC over [0, pos).
    body := pkt.Bytes()
    crc := packet.GetCRC(body, 0, len(body))
    pkt.P4(crc)
    return pkt.Bytes()
}
```

**Important**: confirm `Packet.Pos()` and `Packet.SetByteAt(pos, b)` (or equivalent) exist. If not, restructure: write a placeholder byte and remember the index manually:

```go
invStartPos := len(pkt.Bytes())
pkt.P1(0)
...
out := pkt.Bytes()
out[invStartPos] = byte(invCount)
```

(Confirm `Bytes()` returns a writable slice header backing the same store.)

Also confirm the `Packet` API names — substitute with actual exports
from `pkg/io/packet/packet.go`.

- [ ] **Step 4: Update LoadSave signature to match new Save signature**

If the executor introduced `varpTypes` parameter to Save but not
LoadSave, add it to LoadSave too — keeps the API symmetric and lets
tests pass one set of configs:

```go
func LoadSave(p *Player, sav []byte, invTypes *objtype.InvTypeConfigs, varpTypes *objtype.VarpTypeConfigs) error
```

(`varpTypes` is currently unused by LoadSave but adding it now avoids a
later signature breaking change.)

Update all existing test invocations: `LoadSave(p, raw, cfgs, nil)`.

- [ ] **Step 5: Run round-trip test, fix any byte drift**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestSave_V6_RoundTripsBytePerfect -v`
Expected: PASS.

If it fails, the test output shows `first-diff at byte N` and the
hex dumps — chase per field. Common causes:
- Inv iteration order differs (Go map-iteration is randomized;
  the sort.Ints fixes this — verify the sort call ran).
- Body field with `-1` not encoded as `0xFF` (Go's uint8 conversion of
  `-1` produces `0xFF` via two's-complement; verify).
- Varps PERM check disagrees with TS — verify varpTypes is seeded.

- [ ] **Step 6: Add inv-order pin test**

```go
func TestSave_InvsWrittenInTypeIDAscOrder(t *testing.T) {
    // Player with invs inserted in non-sorted order (5, 2, 7, 1).
    cfgs := &objtype.InvTypeConfigs{
        Configs: make([]*objtype.InvType, 10),
    }
    for _, id := range []int{1, 2, 5, 7} {
        cfgs.Configs[id] = &objtype.InvType{Size: 4, Scope: objtype.InvTypeScopePerm}
    }
    p := &Player{invs: map[int]*inventory.Inventory{}, varps: []int32{}}
    // Insert in deliberately non-ascending order.
    for _, id := range []int{5, 2, 7, 1} {
        p.invs[id] = inventory.FromType(cfgs.Configs[id])
    }
    out := p.Save(cfgs, nil)

    // Parse out the inv section to assert ordering.
    // Section starts after the fixed-size header; locating it precisely
    // requires walking past stats + varps. Easier: re-load and assert
    // p.invs is structurally identical (load is sorted by parse-order
    // which == write order). Then re-decode the inv section manually
    // by walking the bytes.
    //
    // Walk: skip header(4) + body(2+2+1+7+5+1+2+4) +
    // 21*(4+1) stats + 2+0*4 varps (len=0) + 1 invCount byte. Then read
    // 4 sequential typeIDs at +1B each (P2 each).
    pos := 4 + (2 + 2 + 1 + 7 + 5 + 1 + 2 + 4) + 21*(4+1) + (2 + 0)
    if int(out[pos]) != 4 {
        t.Fatalf("invCount byte: got %d, want 4", out[pos])
    }
    // Following bytes: per-inv: typeID(2) + capacity(2) + 4 * slot-zero-bytes(2)
    // (all empty). Read 4 typeIDs.
    pos++
    var seen []int
    for i := 0; i < 4; i++ {
        tid := int(out[pos])<<8 | int(out[pos+1])
        seen = append(seen, tid)
        pos += 2 // typeID
        pos += 2 // capacity
        pos += 2 * 4 // 4 empty slots (each P2 = 0x00 0x00)
    }
    want := []int{1, 2, 5, 7}
    for i := range want {
        if seen[i] != want[i] {
            t.Errorf("inv order: got %v, want %v (pins NAI-PLAYERLOADING-D-INVS-SORTED-BY-TYPEID)",
                seen, want)
            break
        }
    }
}
```

- [ ] **Step 7: Run inv-order test → PASS**

- [ ] **Step 8: Commit**

```bash
git add modules/world/player_save.go modules/world/player_load.go modules/world/player_save_test.go
git commit --no-gpg-sign -m "feat(world/playerloading): (*Player).Save v6 encoder + byte-perfect round-trip

T10 of NAI-PLAYERLOADING. Ports Player.ts:190-270. Sorts invs by typeId
ascending per deviation NAI-PLAYERLOADING-D-INVS-SORTED-BY-TYPEID.
Round-trip vs v6.sav fixture pins TS byte-identity. Inv-order pin test
locks the deviation against accidental regression.

Signatures: LoadSave + Save take (invTypes, varpTypes) explicitly.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 11: Error-path coverage — magic / version / CRC

**Files:**
- Modify: `modules/world/player_save_test.go`

- [ ] **Step 1: Add error-path tests**

```go
func TestLoadSave_BadMagicReturnsErr(t *testing.T) {
    raw := mustReadFixture(t, "v6.sav")
    raw[0] = 0xFF
    p, cfgs := newTestPlayer(t)
    err := LoadSave(p, raw, cfgs, nil)
    if !errors.Is(err, ErrSavInvalidMagic) {
        t.Errorf("got err=%v, want ErrSavInvalidMagic", err)
    }
}

func TestLoadSave_VersionTooHigh_Err(t *testing.T) {
    raw := mustReadFixture(t, "v6.sav")
    raw[2] = 0x00
    raw[3] = 0x07 // version 7
    // After mutation, the CRC is now stale — load will report
    // ErrSavCorrupt before ErrSavUnsupportedVer if CRC check is first.
    // We need version-check ordering BEFORE CRC. Recompute CRC to
    // isolate the version-check arm.
    binary.BigEndian.PutUint32(raw[len(raw)-4:], packet.GetCRC(raw, 0, len(raw)-4))
    p, cfgs := newTestPlayer(t)
    err := LoadSave(p, raw, cfgs, nil)
    if !errors.Is(err, ErrSavUnsupportedVer) {
        t.Errorf("got err=%v, want ErrSavUnsupportedVer", err)
    }
}

func TestLoadSave_VersionZero_Err(t *testing.T) {
    raw := mustReadFixture(t, "v6.sav")
    raw[2] = 0x00
    raw[3] = 0x00 // version 0
    binary.BigEndian.PutUint32(raw[len(raw)-4:], packet.GetCRC(raw, 0, len(raw)-4))
    p, cfgs := newTestPlayer(t)
    err := LoadSave(p, raw, cfgs, nil)
    if !errors.Is(err, ErrSavUnsupportedVer) {
        t.Errorf("got err=%v, want ErrSavUnsupportedVer", err)
    }
}

func TestLoadSave_CRCMismatch_Err(t *testing.T) {
    raw := mustReadFixture(t, "v6.sav")
    raw[len(raw)-1] ^= 0x01 // flip last CRC byte
    p, cfgs := newTestPlayer(t)
    err := LoadSave(p, raw, cfgs, nil)
    if !errors.Is(err, ErrSavCorrupt) {
        t.Errorf("got err=%v, want ErrSavCorrupt", err)
    }
}
```

- [ ] **Step 2: Run all four tests → all PASS**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestLoadSave_(BadMagic|VersionTooHigh|VersionZero|CRCMismatch)' -v`
Expected: all PASS.

If `TestLoadSave_VersionTooHigh_Err` returns `ErrSavCorrupt` instead of
`ErrSavUnsupportedVer`, the version check must happen BEFORE the CRC
check in LoadSave. Inspect order; reorder if necessary (and update the
table in the spec if you flip the order).

- [ ] **Step 3: Commit**

```bash
git add modules/world/player_save_test.go
git commit --no-gpg-sign -m "test(world/playerloading): error-path pins (magic / version / CRC)

T11 of NAI-PLAYERLOADING. Pins all three sentinel-error arms via
fixture mutation. Tests reset the CRC where header mutations would
otherwise mask the intended error.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 12: CRC high-bit round-trip pin

**Files:**
- Modify: `modules/world/player_save_test.go`

- [ ] **Step 1: Write test**

```go
func TestSave_CRCHighBitSet_RoundTrips(t *testing.T) {
    // Construct players until one yields a CRC with the high bit set.
    // The CRC is over body bytes; mutating one body field (e.g. x) is
    // enough to cycle through CRC values.
    cfgs := &objtype.InvTypeConfigs{Configs: []*objtype.InvType{
        {Size: 4, Scope: objtype.InvTypeScopePerm},
    }}
    var bytesOut []byte
    for x := 0; x < 65535; x++ {
        p := &Player{
            invs:  map[int]*inventory.Inventory{0: inventory.FromType(cfgs.Configs[0])},
            varps: []int32{},
        }
        p.x = x
        out := p.Save(cfgs, nil)
        // Trailing 4 bytes are CRC big-endian.
        crc := binary.BigEndian.Uint32(out[len(out)-4:])
        if crc&0x80000000 != 0 {
            bytesOut = out
            break
        }
    }
    if bytesOut == nil {
        t.Fatal("could not find a CRC with high bit set across x=[0..65535)")
    }
    // Now round-trip-load.
    p := &Player{
        invs:  map[int]*inventory.Inventory{0: inventory.FromType(cfgs.Configs[0])},
        varps: []int32{},
    }
    if err := LoadSave(p, bytesOut, cfgs, nil); err != nil {
        t.Fatalf("LoadSave(CRC with high bit set): %v — pins TS signedness parity", err)
    }
}
```

- [ ] **Step 2: Run → PASS**

- [ ] **Step 3: Commit**

```bash
git add modules/world/player_save_test.go
git commit --no-gpg-sign -m "test(world/playerloading): CRC high-bit round-trip pin

T12 of NAI-PLAYERLOADING. TS reads CRC as g4s (signed i32) but compares
against an unsigned getcrc return. Pins that goscape's u32 read/write
round-trips identically for CRC values with the high bit set.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 13: Move tick.go empty-save bootstrap into LoadSave call

**Files:**
- Modify: `modules/world/tick.go`
- Modify: `modules/world/client.go`

This task migrates the existing skill-default init at `tick.go:157` to
call `LoadSave(p, c.savePayload, ...)` instead. After this task,
LoadSave owns the bootstrap path.

- [ ] **Step 1: Add `savePayload []byte` to client struct**

In `modules/world/client.go`, add to the client struct around line 70:

```go
    // savePayload is the optional SAV bytes returned by the login server
    // on PlayerLogin (resp.GetSave()). Read once by processLogins on the
    // tick goroutine to populate the freshly-constructed Player via
    // LoadSave. Nil for fresh accounts; arbitrary length for returning
    // players.
    savePayload []byte
```

- [ ] **Step 2: Capture resp.Save in server.go login flow**

In `modules/world/server.go`, inside the `if result == OK ...` block
(around line 723):

```go
            if result == loginpb.LoginResult_LOGIN_RESULT_OK ||
                result == loginpb.LoginResult_LOGIN_RESULT_NEW_PLAYER ||
                result == loginpb.LoginResult_LOGIN_RESULT_RECONNECT_OK {
                c.staffModLevel = resp.GetStaffModLevel()
                c.members = resp.GetMembers()
                c.username = safeName
                c.savePayload = resp.GetSave() // NEW
            }
```

- [ ] **Step 3: Replace tick.go bootstrap call with LoadSave**

In `modules/world/tick.go` around line 157:

Before (the existing block):
```go
            // Default-player skill init — 21 skills at level 1 with 0 XP, then
            // Hitpoints overridden to level 10 with the matching XP. ...
            for i := range objtype.PlayerStatCount {
                p.stats[i] = 0
                p.baseLevels[i] = 1
                p.levels[i] = 1
            }
            p.stats[objtype.PlayerStatHitpoints] = int32(objtype.GetExpByLevel(10))
            p.baseLevels[objtype.PlayerStatHitpoints] = 10
            p.levels[objtype.PlayerStatHitpoints] = 10
```

After:
```go
            // Player state load. Delegates to LoadSave which handles both
            // populated-save (full decode) and empty-save (default-skill
            // bootstrap) paths. NAI-PLAYERLOADING.
            if err := LoadSave(p, p.client.savePayload, s.invTypes, s.varpTypes); err != nil {
                s.log.Warn("LoadSave failed; falling back to empty bootstrap",
                    slog.String("username", p.username), slog.Any("err", err))
                _ = LoadSave(p, nil, s.invTypes, s.varpTypes)
            }
```

- [ ] **Step 4: Build and run all world tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/...`
Expected: all PASS. If any existing tick test breaks because the
bootstrap moved, the test was implicitly relying on the in-tick init.
Update those tests to call `LoadSave(p, nil, nil, nil)` directly
(the world-level integration tests still cover the wiring).

- [ ] **Step 5: Commit**

```bash
git add modules/world/client.go modules/world/server.go modules/world/tick.go
git commit --no-gpg-sign -m "feat(world/playerloading): wire LoadSave into processLogins

T13 of NAI-PLAYERLOADING. Adds c.savePayload to capture resp.GetSave()
from PlayerLogin. processLogins now calls LoadSave to populate the
fresh Player — single entry point owning both the save-bytes decode
path and the empty-save bootstrap (which previously lived inline at
tick.go:157, doc-commented as 'a future sub-spec'). On LoadSave
error, log a warning and fall back to the empty-save path so a
corrupt SAV doesn't deny login (deviation
NAI-PLAYERLOADING-D-DECODE-ERR-FALLS-BACK-TO-BOOTSTRAP).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 14: Login-flow integration tests

**Files:**
- Modify: `modules/world/player_save_test.go` (or a new
  `modules/world/playerloading_integration_test.go`)

These tests stub the LoginClient to deliver specific PlayerLoginResponse
payloads and assert the Player's resulting state.

- [ ] **Step 1: Survey existing test plumbing**

Look for existing fakes of `LoginClient` in
`modules/world/*_test.go` (`grep -nR "LoginClient" --include='*_test.go'`).
If a fake exists, reuse it; otherwise author a minimal one.

- [ ] **Step 2: Write the three tests**

```go
func TestLoginAcceptedWithSave_DecodesIntoPlayer(t *testing.T) {
    // Boot a Server with a fake LoginClient that returns LOGIN_RESULT_OK
    // and resp.Save = mustReadFixture(t, "v6.sav"). Drive a client
    // through login. After processLogins runs, assert the player's
    // x, z, level match fixturePlayerValues.
    // [Details depend on existing test scaffolding — see test
    // helpers in modules/world/server_test.go for the conventional pattern.]
}

func TestLoginAcceptedWithoutSave_BootstrapsDefaults(t *testing.T) {
    // Fake returns LOGIN_RESULT_OK with resp.Save = nil. Assert
    // p.stats[Hitpoints] == GetExpByLevel(10), p.x == 3094 (newPlayer default),
    // etc.
}

func TestLoginAcceptedWithCorruptSave_FallsBackToBootstrap(t *testing.T) {
    // Fake returns LOGIN_RESULT_OK with resp.Save = []byte{0x00, 0x00} (bad magic).
    // Assert bootstrap state applied AND a warning was logged.
    // Capture log via a slog.Handler test sink.
}
```

If the existing test scaffolding does not support stubbing
`loginClient` cleanly, **leave a TODO** in this task and move the
tests to a follow-up — do not block on a major test-harness refactor.
Document the gap in the commit message.

- [ ] **Step 3: Run → PASS**

- [ ] **Step 4: Commit**

```bash
git add modules/world/player_save_test.go # or playerloading_integration_test.go
git commit --no-gpg-sign -m "test(world/playerloading): login-flow integration pins

T14 of NAI-PLAYERLOADING. Three pins exercising the full PlayerLogin →
processLogins → LoadSave chain:
- Accepted with SAV decodes fields from fixture
- Accepted without SAV applies bootstrap
- Accepted with corrupt SAV falls back to bootstrap + logs warning
(pins NAI-PLAYERLOADING-D-DECODE-ERR-FALLS-BACK-TO-BOOTSTRAP).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 15: Split removePlayer → removePlayerInternal + removePlayerOnTick + removePlayerOnDisconnect

**Files:**
- Modify: `modules/world/server.go`
- Modify: `modules/world/tick.go`

- [ ] **Step 1: Rename existing removePlayer to removePlayerInternal**

In `server.go` line 835:

```go
// removePlayerInternal performs the slot/zone/playerLoop cleanup for p.
// Must only be called from the tick goroutine.
//
// Callers should use removePlayerOnTick or removePlayerOnDisconnect,
// which add the appropriate gRPC-side cleanup before invoking this.
func (s *Server) removePlayerInternal(p *Player) {
    // ... existing body unchanged ...
}
```

- [ ] **Step 2: Add tick-goroutine wrapper**

After `removePlayerInternal`:

```go
// removePlayerOnTick handles graceful logout from the tick goroutine.
// Captures p.Save() while still on-tick (thread-safe) and fires a
// best-effort PlayerLogout RPC in a goroutine, then performs internal
// cleanup.
//
// Deviation NAI-PLAYERLOADING-D-LOGOUT-NO-FORCE-FALLBACK: on RPC
// failure, log only — no PlayerForceLogout belt-and-braces (TS parity).
func (s *Server) removePlayerOnTick(p *Player) {
    if s.loginClient != nil && p.username != "" {
        save := p.Save(s.invTypes, s.varpTypes)
        username := p.username
        go func() {
            ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
            defer cancel()
            _, err := s.loginClient.PlayerLogout(ctx, &loginpb.PlayerLogoutRequest{
                NodeId:   int32(s.cfg.NodeID),
                Profile:  s.cfg.NodeProfile,
                Username: username,
                Save:     save,
            })
            if err != nil {
                s.log.Warn("PlayerLogout RPC failed",
                    slog.String("username", username), slog.Any("err", err))
            }
        }()
    }
    s.removePlayerInternal(p)
}
```

- [ ] **Step 3: Add disconnect wrapper**

```go
// removePlayerOnDisconnect handles ungraceful disconnect from the
// per-conn goroutine. Cannot safely call p.Save() (would race tick
// goroutine), so calls PlayerForceLogout instead.
//
// Deviation NAI-PLAYERLOADING-D-DISCONNECT-NO-SAVE: state since the
// last autosave is lost on ungraceful disconnect. Autosave cadence
// (15 min) caps the loss window. TS has the same window.
func (s *Server) removePlayerOnDisconnect(p *Player) {
    if s.loginClient != nil && p.username != "" {
        go s.loginClient.PlayerForceLogout(context.Background(),
            &loginpb.PlayerForceLogoutRequest{
                NodeId:   int32(s.cfg.NodeID),
                Profile:  s.cfg.NodeProfile,
                Username: p.username,
            })
    }
    s.removePlayerInternal(p)
}
```

- [ ] **Step 4: Update call sites**

In `tick.go:305`:
```go
                s.removePlayer(p)
```
becomes:
```go
                s.removePlayerOnTick(p)
```

In `server.go:545`:
```go
            s.removePlayer(c.player)
```
becomes:
```go
            s.removePlayerOnDisconnect(c.player)
```

Delete the old `removePlayer` name (now `removePlayerInternal`)
references anywhere else — if any test calls `removePlayer` directly,
update to call `removePlayerInternal` or whichever variant is
appropriate. Run `grep -nR "\.removePlayer\b" modules/world/` to find
them all.

- [ ] **Step 5: Build + run all world tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/...`
Expected: all PASS.

- [ ] **Step 6: Commit**

```bash
git add modules/world/server.go modules/world/tick.go
git commit --no-gpg-sign -m "refactor(world): split removePlayer into tick + disconnect variants

T15 of NAI-PLAYERLOADING. p.Save() must run on the tick goroutine for
thread-safety; removePlayer was called from both tick and per-conn
goroutines. Splits into:
- removePlayerInternal — slot/zone/playerLoop cleanup (unchanged body)
- removePlayerOnTick — calls Save() + PlayerLogout, then Internal
- removePlayerOnDisconnect — calls PlayerForceLogout, then Internal

Two new deviation tags (LOGOUT-NO-FORCE-FALLBACK, DISCONNECT-NO-SAVE)
documented at the function sites.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 16: Logout-on-tick + integration test

**Files:**
- Modify: `modules/world/player_save_test.go` (or integration file)

The wiring landed in T15; this task adds the pin test.

- [ ] **Step 1: Write the pin test**

```go
func TestRemovePlayerOnTick_CallsLogoutWithSaveBytes(t *testing.T) {
    // Stand up a Server with a fake LoginClient that captures
    // PlayerLogout calls. Boot a player with known state (e.g., loaded
    // from v6.sav). Call s.removePlayerOnTick(p). Wait briefly for the
    // async goroutine. Assert:
    //  1. exactly one PlayerLogout call captured
    //  2. captured req.Save bytes pass VerifySave
    //  3. loading captured req.Save into a fresh player yields fields
    //     matching the source player
    //
    // [Test harness depends on existing fake — same caveat as T14.]
}
```

If T14's fake-LoginClient harness is incomplete, leave this as a TODO
and document the gap.

- [ ] **Step 2: Run → PASS**

- [ ] **Step 3: Commit**

```bash
git add modules/world/player_save_test.go
git commit --no-gpg-sign -m "test(world/playerloading): logout-on-tick captures save bytes

T16 of NAI-PLAYERLOADING. Pins that removePlayerOnTick fires
PlayerLogout with the player's Save() bytes and that those bytes
round-trip-load to the source state.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 17: Disconnect path test

**Files:**
- Modify: `modules/world/player_save_test.go` (or integration file)

- [ ] **Step 1: Write test**

```go
func TestRemovePlayerOnDisconnect_CallsForceLogoutOnly(t *testing.T) {
    // Fake LoginClient captures both PlayerLogout and PlayerForceLogout.
    // Call s.removePlayerOnDisconnect(p). Wait briefly. Assert:
    //  - PlayerForceLogout was called (with correct username/profile/nodeID)
    //  - PlayerLogout was NOT called
    // Pins NAI-PLAYERLOADING-D-DISCONNECT-NO-SAVE.
}
```

- [ ] **Step 2: Run → PASS**

- [ ] **Step 3: Commit**

```bash
git add modules/world/player_save_test.go
git commit --no-gpg-sign -m "test(world/playerloading): disconnect path calls ForceLogout only

T17 of NAI-PLAYERLOADING. Pins the deviation
NAI-PLAYERLOADING-D-DISCONNECT-NO-SAVE: ungraceful disconnect cleanup
must not call p.Save() (would race tick goroutine), so uses the
no-save PlayerForceLogout RPC.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 18: Autosave loop + integration test

**Files:**
- Modify: `modules/world/tick.go`
- Modify: `modules/world/server.go` (add autosavePlayers method)
- Modify: `modules/world/player_save_test.go`

- [ ] **Step 1: Add PlayerSaveRate constant + autosavePlayers method**

In `server.go` (or `tick.go` — wherever public constants/related
helpers live; check existing pattern):

```go
// PlayerSaveRate is the autosave cadence in ticks. 1500 ticks at ~600ms
// ≈ 15 minutes. Mirrors TS World.PLAYER_SAVERATE.
const PlayerSaveRate = 1500

// autosavePlayers fires a best-effort PlayerAutosave RPC for each
// active player. Must only be called from the tick goroutine.
//
// Deviation NAI-PLAYERLOADING-D-AUTOSAVE-FIRE-AND-FORGET: per-call
// failures log only; no automatic remediation.
func (s *Server) autosavePlayers() {
    if s.loginClient == nil {
        return
    }
    for _, p := range s.playerLoop {
        if p == nil || p.username == "" {
            continue
        }
        save := p.Save(s.invTypes, s.varpTypes)
        username := p.username
        req := &loginpb.PlayerAutosaveRequest{
            Profile:  s.cfg.NodeProfile,
            Username: username,
            Save:     save,
        }
        go s.loginClient.PlayerAutosave(context.Background(), req)
    }
}
```

- [ ] **Step 2: Add the tick-loop trigger**

In `tick.go`, near the top of the for-body (before `s.processClientsIn()`):

```go
        if s.currentTick%PlayerSaveRate == 0 && s.currentTick > 0 {
            s.autosavePlayers()
        }
```

Place it AFTER `s.currentTick++` if currentTick increments at the
**end** of the cycle (it does — see tick.go:90). For consistency,
read `s.currentTick` after the increment to test multiples cleanly,
i.e., put the autosave check at the **top** of the next for-iteration
(immediately at the start of the loop body, before any process step).

Audit the increment timing carefully:
- If tick.go:90 has `s.currentTick++` at the end of the body, then
  the autosave check at the top of the next body sees the incremented
  value. Tick 1500 fires when currentTick==1500 at the start of the
  iteration where the previous iteration completed tick 1499.
- Goal: autosave at the START of tick 1500, 3000, 4500, ... (not 0).

- [ ] **Step 3: Write autosave-cadence test**

```go
func TestTickAutosave_FiresAtRateMultiples(t *testing.T) {
    // Stand up Server with fake LoginClient counting PlayerAutosave
    // calls. Add one player. Run the tick loop for 4501 ticks
    // (or simulate by driving s.currentTick + calling the autosave
    // gate directly, bypassing the goroutine).
    //
    // Simpler: extract the gate check into a helper s.maybeAutosave()
    // that's called from the loop. Test invokes maybeAutosave at
    // currentTick=0, 1, ..., 4501 and asserts the count == 3.
    fake := &fakeLoginClient{}
    s := &Server{
        loginClient: fake,
        currentTick: 0,
        playerLoop:  []*Player{{username: "tester"}},
        invTypes:    &objtype.InvTypeConfigs{Configs: []*objtype.InvType{}},
        varpTypes:   &objtype.VarpTypeConfigs{Configs: []*objtype.VarpType{}},
    }
    s.playerLoop[0].invs = map[int]*inventory.Inventory{}
    for tick := 0; tick <= 4501; tick++ {
        s.currentTick = tick
        if s.currentTick%PlayerSaveRate == 0 && s.currentTick > 0 {
            s.autosavePlayers()
        }
    }
    // Three fires: tick 1500, 3000, 4500.
    if fake.autosaveCount != 3 {
        t.Errorf("autosaveCount: got %d, want 3 (ticks 1500, 3000, 4500)", fake.autosaveCount)
    }
}
```

(fakeLoginClient implementation: define a minimal stub that satisfies
the relevant interface or method-set used by Server. If LoginClient is
a concrete struct, refactor to an interface for this test:

```go
type loginClientIface interface {
    PlayerAutosave(ctx context.Context, req *loginpb.PlayerAutosaveRequest)
    // ... add others as needed
}
```

and change `Server.loginClient` to that interface type. Worth doing in
this task for testability.)

If the interface refactor is non-trivial, mark this test as `t.Skip`
and document the gap in the commit message — autosave wiring itself
still lands.

- [ ] **Step 4: Run → PASS**

- [ ] **Step 5: Commit**

```bash
git add modules/world/tick.go modules/world/server.go modules/world/player_save_test.go
git commit --no-gpg-sign -m "feat(world/playerloading): autosave every PlayerSaveRate ticks

T18 of NAI-PLAYERLOADING. Adds PlayerSaveRate=1500 (TS parity) and the
autosavePlayers helper that fires fire-and-forget PlayerAutosave RPCs
for each active player. Tick-loop gate at currentTick%1500==0 && >0.
Cadence pinned by test that drives 0..4501 ticks and asserts exactly
3 fires.

Deviation NAI-PLAYERLOADING-D-AUTOSAVE-FIRE-AND-FORGET documented at
the helper.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 19: Final verification — full test pass + smoke build

**Files:** none

- [ ] **Step 1: Run the full world-module test suite with race detector**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/world/...`
Expected: PASS with no race-detector reports.

If a race fires in the autosave goroutine path, audit
`autosavePlayers` — `s.playerLoop` is read on the tick goroutine which
is correct, but `p.Save(...)` mutating any fresh allocations should not
share state. If `p.username` is accessed inside the goroutine,
**capture it into a local** before the `go func()` (already done in
T18 example).

- [ ] **Step 2: Run the full repo test suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`
Expected: PASS.

- [ ] **Step 3: Build the binary**

Run: `CGO_ENABLED=0 GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build -trimpath -o /tmp/goscape ./cmd/goscape`
Expected: builds cleanly.

- [ ] **Step 4: Run go vet**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./modules/world/...`
Expected: no findings.

- [ ] **Step 5: Optional smoke-pack sanity (codec doesn't affect pack but run anyway)**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go run ./cmd/goscape-cli smoke-pack --content-dir $HOME/Code/github.com/LostCityRS/content`
Expected: same 12 OK / 0 ERR / 0 SKIP baseline as start-of-session. No regression on pack stages.

- [ ] **Step 6: Update memory + commit close**

Add a memory entry summarizing the slice. Update
`$HOME/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/MEMORY.md`
with a new line:

```markdown
- [NAI-PLAYERLOADING close](nai_playerloading_close.md) — SAV codec
  (Verify/LoadSave/Player.Save) + RPC wire-up (login decode / 1500-tick
  autosave / split removePlayer{OnTick,OnDisconnect}) shipped 2026-05-18
  at <FINAL-SHA>; 6 deviation tags; v1..v6 byte-pin via TS-gen fixtures.
```

And create the topic file `nai_playerloading_close.md` with the
standard frontmatter + body.

- [ ] **Step 7: Write the close commit (no code; memory + tag only)**

```bash
git add $HOME/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/
# Memory auto-commit hook will commit these in the memory repo; otherwise:
# git -C $HOME/.claude/projects/... commit ...

# In goscape repo:
git commit --no-gpg-sign --allow-empty -m "chore(close): NAI-PLAYERLOADING — SAV codec + RPC wire-up shipped

Closes the PlayerLoading port: world side now produces and consumes the
on-disk SAV format byte-for-byte against TS fixtures v1..v6 and wires
the codec into the existing gRPC LoginService plumbing for login
decode, 15-min autosave, and graceful logout (with no-save fallback on
ungraceful disconnect to avoid racing the tick goroutine).

Six deviation tags retired into the source as inline comments:
  NAI-PLAYERLOADING-D-INVS-SORTED-BY-TYPEID
  NAI-PLAYERLOADING-D-DECODE-ERR-FALLS-BACK-TO-BOOTSTRAP
  NAI-PLAYERLOADING-D-LOGOUT-NO-FORCE-FALLBACK
  NAI-PLAYERLOADING-D-DISCONNECT-NO-SAVE
  NAI-PLAYERLOADING-D-AUTOSAVE-FIRE-AND-FORGET
  NAI-PLAYERLOADING-D-HAS-SAVE-ALWAYS-FALSE

Out-of-scope follow-ups (open):
- Goscape-side LoginServer port (fs SAV reads/writes, hiscore export,
  ban table)
- wouldResetSaveFile server-side guard
- Reconnect HasSave optimization
- Historical SAV migration tool (currently load→save auto-migrates v1..v5)

Spec: docs/superpowers/specs/2026-05-18-playerloading-design.md
Plan: docs/superpowers/plans/2026-05-18-playerloading.md

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Notes for the executor

### Type / API drift to verify at first touch

- **Player struct field names**: `varps` (not `vars`), `varpsString`
  (not `varsString`), `afkZones [2]int32`, `lastAfkZone int`,
  `lastLoginTime int64`, `playtime int`. Confirm at task start.
- **Packet API**: confirm `NewPacketView`, `Pos()`, `SetPos(int)`,
  `Bytes()`, `SetByteAt(int, byte)`. If method names differ (e.g.,
  `Pos` is a field), adjust accordingly. Existing call sites:
  `pkg/cache/preloaded.go:60`, `pkg/io/jagfile/jagfile.go:302`.
- **Inventory.Item field names**: `ID`, `Count`. Confirm by reading
  `pkg/inventory/inventory.go:24` (the Item struct).
- **objtype.VarpTypeConfigs / objtype.InvTypeConfigs**: confirm
  exported type names by `grep -n "^type .*TypeConfigs" pkg/objtype/*.go`.
- **objtype.ScriptVarTypeString**: confirm constant exists.
- **Player.getCombatLevel**: confirm method name + receiver.

### TDD discipline

Each task follows: write failing test → run → implement → run → commit.
Do not skip the red-phase verification — it catches plan-codified
assertions that don't actually fail with the old code (a recurring
pattern per memory `[[plan_red_phase_prediction_old_sut]]`).

### Subagent-driven execution

This plan is sized for subagent-driven dispatch. Tasks 1, 2, 3, 11,
12 are smallest (single-file, single-concept) and good warm-ups.
Tasks 4 and 10 are the largest (full v1 body decode; full v6 encode)
and may need two-stage review.

### User-blocking step

After Task 0, **pause and ask the user to generate the v1..v6 binary
fixtures** before starting Task 4. Tasks 1, 2, 3 can run before the
fixtures arrive.
